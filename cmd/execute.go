// Package cmd is preen's command line surface. It parses arguments, wires the
// engine, and renders results; every decision and guardrail lives in the
// engine packages so the CLI stays a thin shell over them.
package cmd

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/dcadolph/preen/plan"
	"github.com/dcadolph/preen/repo"
	"github.com/dcadolph/preen/run"
)

// usage is the top-level help text.
const usage = `usage: preen [command] [flags]

Turn a messy working tree into clean, atomic commits. preen shows a plan and
changes nothing until it is approved, backs up before it moves anything, and
verifies that your content came out exactly as it went in.

Commands:
  (none)      Group the working tree into commits.
  restore     Undo a preen run from its backup ref.
  backups     List the recovery refs preen has left behind.

Run flags:
  --scope PATH    Preen only paths under PATH. Repeatable.
  --gate CMD      Run CMD after each commit; a failure rolls the run back.
  --absorb        Bring unpushed commits back and redo them.
  --fixup         Fold changes into the unpushed commits that introduced them.
  --sweep         Report debug prints and other leftovers. Never removes them.
  --grouper PROG  Group with an external program instead of the built-in rules.
                  PROG reads JSON on stdin and writes JSON on stdout, and can
                  split one file's hunks across commits. It falls back to the
                  built-in rules if it fails or answers with anything unusable.
  --dry-run       Show the plan and stop.
  --yes           Skip the approval prompt.
  --no-verify     Skip commit hooks. Requires your explicit consent.
  --version       Print the version and exit.
  -h, --help      Print this help and exit.

Rewriting published history (both consents are required):
  --pushed             Consent to redoing commits a remote already has.
  --pushed-base REV    The commit just before the range to redo. Required on
                       the default branch, where there is no other boundary.
  --allow-protected    Permit it on a protected branch, when it is yours alone.
                       main, master, trunk, develop, release, and production
                       are protected, plus anything in [protect] in the config.
  The push is a separate confirmation and always uses --force-with-lease.

Message style (each overrides .preen.toml):
  --conventional     Shape subjects as Conventional Commits.
  --prefix TEXT      Prepend TEXT to every subject, such as a ticket id.
  --max-subject N    Cap the subject length. Default 72.
  --punctuation MODE Terminal punctuation: auto, always, or never.
  --no-emdash        Forbid em and en dashes.
  --no-semicolon     Forbid semicolons.
  --no-hyphen        Forbid hyphens.
  --lower-subject    Lowercase the subject's first letter.
  --sign-off         Add a Signed-off-by trailer.
  --body MODE        When to include a body: auto, always, or never.
  --include-files    List the touched paths in each body.
  --include-line-numbers  Cite each file's changed line ranges in the body.

At the approval prompt you can edit the plan before anything moves: merge,
split, move, reword, drop, and reorder. Type ? there for the list.

Examples:
  preen
  preen --scope internal --gate 'go test ./...'
  preen --absorb --dry-run
  preen --conventional --prefix ABC-123
  preen restore
  preen backups --prune
`

// Execute runs preen with the given arguments and returns a process exit code.
func Execute(args []string) int {
	ctx := context.Background()
	env := &environment{Out: os.Stdout, Err: os.Stderr, In: os.Stdin}
	code, err := dispatch(ctx, env, args)
	if err != nil {
		_, _ = fmt.Fprintf(env.Err, "preen: %v\n", err)
	}
	return code
}

// environment holds the streams a command reads and writes, so tests drive the
// CLI without touching the real terminal.
type environment struct {
	// Out receives normal output.
	Out io.Writer
	// Err receives errors.
	Err io.Writer
	// In supplies the approval answer.
	In io.Reader
	// Dir is the directory the repository is discovered from. An empty value
	// means the process working directory.
	Dir string
}

// printf writes formatted output. A failed write to the terminal leaves
// nothing worth doing, so the error is dropped here rather than threaded
// through every call site.
func (e *environment) printf(format string, args ...any) {
	_, _ = fmt.Fprintf(e.Out, format, args...)
}

// print writes output verbatim.
func (e *environment) print(args ...any) {
	_, _ = fmt.Fprint(e.Out, args...)
}

// println writes output followed by a newline.
func (e *environment) println(args ...any) {
	_, _ = fmt.Fprintln(e.Out, args...)
}

// dispatch routes to a subcommand, treating anything that is not a known
// command as flags for the default run.
func dispatch(ctx context.Context, env *environment, args []string) (int, error) {
	if len(args) > 0 {
		switch args[0] {
		case "restore":
			return runRestore(ctx, env, args[1:])
		case "backups":
			return runBackups(ctx, env, args[1:])
		case "help", "-h", "--help":
			env.print(usage)
			return CodeOK, nil
		case "--version":
			env.printf("preen %s\n", Version)
			return CodeOK, nil
		}
	}
	return runPreen(ctx, env, args)
}

// openRepo opens the repository holding the environment's directory, which is
// the process working directory unless a caller set one.
func openRepo(ctx context.Context, env *environment) (*repo.Repo, error) {
	dir := env.Dir
	if dir == "" {
		var err error
		if dir, err = os.Getwd(); err != nil {
			return nil, err
		}
	}
	return repo.Open(ctx, dir)
}

// exitCode maps an error onto the process exit code that describes it.
func exitCode(err error) int {
	switch {
	case err == nil:
		return CodeOK
	case errors.Is(err, repo.ErrNotRepo):
		return CodeNoRepo
	case errors.Is(err, repo.ErrInProgress):
		return CodeRepoState
	case errors.Is(err, run.ErrNothingToDo):
		return CodeNothingToDo
	case errors.Is(err, run.ErrContentChanged):
		return CodeContentChanged
	case errors.Is(err, run.ErrGateFailed):
		return CodeGateFailed
	case errors.Is(err, plan.ErrInvalid):
		return CodeInvalidPlan
	case errors.Is(err, ErrAborted):
		return CodeAborted
	default:
		return CodeErr
	}
}
