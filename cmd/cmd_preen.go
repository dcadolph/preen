package cmd

import (
	"bufio"
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/dcadolph/preen/config"
	"github.com/dcadolph/preen/group"
	"github.com/dcadolph/preen/plan"
	"github.com/dcadolph/preen/run"
	"github.com/dcadolph/preen/style"
)

// runPreen is the default command: survey the tree, show a plan, and apply it
// after approval.
func runPreen(ctx context.Context, env *environment, args []string) (int, error) {
	opts, settings, err := parseRunFlags(env, args)
	if err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return CodeOK, nil
		}
		return CodeErr, err
	}

	repository, err := openRepo(ctx, env)
	if err != nil {
		return exitCode(err), err
	}
	cfg, err := config.Load(repository.Root())
	if err != nil {
		return CodeErr, err
	}
	applyConfig(&opts, cfg, args)

	engine := run.New(repository)
	engine.Out = env.Out
	if settings.Grouper != "" {
		// A grouper that fails or answers with something unusable falls back to
		// the built-in rules rather than taking the run down with it.
		engine.Grouper = group.Chain(
			group.Command{Name: settings.Grouper, Dir: repository.Root()},
			group.NewHeuristic(),
		)
	}
	if opts.Fixup {
		return runFixup(ctx, env, engine, opts, settings)
	}

	built, err := engine.Plan(ctx, opts)
	if err != nil {
		if errors.Is(err, run.ErrNothingToDo) {
			env.println("Nothing to preen: the tree is clean and no commits need redoing.")
			return CodeNothingToDo, nil
		}
		return exitCode(err), err
	}
	if err := built.Render(env.Out); err != nil {
		return CodeErr, err
	}
	if settings.DryRun {
		env.println("Dry run: nothing was staged or committed.")
		return CodeOK, nil
	}
	if !settings.Yes {
		approved, err := review(env, built)
		if err != nil {
			return CodeErr, err
		}
		if !approved {
			env.println("Aborted. Nothing was changed.")
			return CodeAborted, nil
		}
	}

	result, err := engine.Apply(ctx, built, opts)
	if err != nil {
		return exitCode(err), err
	}
	reportRun(env, result)

	if built.Push != "" {
		return publish(ctx, env, engine, built, settings)
	}
	return CodeOK, nil
}

// runFixup folds dirty changes into the unpushed commits that introduced them.
func runFixup(ctx context.Context, env *environment, engine *run.Engine,
	opts run.Options, settings promptSettings) (int, error) {
	built, err := engine.PlanFixup(ctx, opts)
	if err != nil {
		if errors.Is(err, run.ErrNothingToDo) {
			env.println("Nothing to fold in: no dirty changes over unpushed commits.")
			return CodeNothingToDo, nil
		}
		return exitCode(err), err
	}
	if err := built.Plan.Render(env.Out); err != nil {
		return CodeErr, err
	}
	if settings.DryRun {
		env.println("Dry run: nothing was staged or committed.")
		return CodeOK, nil
	}
	if !settings.Yes {
		ok, err := confirm(env, "Fold these changes in? [y/N]: ")
		if err != nil {
			return CodeErr, err
		}
		if !ok {
			env.println("Aborted. Nothing was changed.")
			return CodeAborted, nil
		}
	}
	result, err := engine.ApplyFixup(ctx, built, opts)
	if err != nil {
		return exitCode(err), err
	}
	reportRun(env, result)
	return CodeOK, nil
}

// publish force-pushes a rewritten branch, after a confirmation that shows the
// exact command. Publishing is always a separate consent from rewriting.
func publish(ctx context.Context, env *environment, engine *run.Engine,
	built *plan.Plan, settings promptSettings) (int, error) {
	env.printf("\nThis rewrote published history. To share it:\n  %s\n", built.Push)
	if !settings.Yes {
		ok, err := confirm(env, "Run that push now? [y/N]: ")
		if err != nil {
			return CodeErr, err
		}
		if !ok {
			env.println("Not pushed. The rewritten commits are local until you push them.")
			return CodeOK, nil
		}
	}
	if err := engine.Push(ctx); err != nil {
		return CodeErr, fmt.Errorf("push failed, the rewrite is still local: %w", err)
	}
	env.println("Pushed.")
	return CodeOK, nil
}

// parseRunFlags reads the flags for a run. It returns the engine options plus
// the two decisions the CLI itself owns: whether to stop after the plan and
// whether to skip the prompt.
func parseRunFlags(env *environment, args []string) (opts run.Options, settings promptSettings, err error) {
	fs := flag.NewFlagSet("preen", flag.ContinueOnError)
	fs.SetOutput(env.Err)
	fs.Usage = func() { env.print(usage) }

	var scope stringList
	var spread, grouperCmd string
	fs.Var(&scope, "scope", "limit the run to paths under this prefix (repeatable)")
	fs.StringVar(&opts.Gate, "gate", "", "command to run after each commit")
	fs.BoolVar(&opts.Absorb, "absorb", false, "bring unpushed commits back and redo them")
	fs.BoolVar(&opts.Fixup, "fixup", false, "fold changes into the unpushed commits that introduced them")
	fs.BoolVar(&opts.Pushed, "pushed", false, "consent to rewriting commits a remote already has")
	fs.StringVar(&opts.PushedBase, "pushed-base", "", "the commit just before the range to redo")
	fs.BoolVar(&opts.AllowProtected, "allow-protected", false,
		"permit a rewrite on a protected branch, when the branch is yours alone")
	fs.StringVar(&grouperCmd, "grouper", "",
		"program that groups the changes, reading JSON on stdin and writing JSON on stdout")
	fs.BoolVar(&opts.NoVerify, "no-verify", false, "skip commit hooks")
	fs.BoolVar(&opts.AllowHookRewrites, "allow-hook-rewrites", false,
		"accept content changes a commit hook made to files the run committed")
	fs.StringVar(&spread, "spread", "", "spread commit timestamps across a window, like 2h, or auto")
	fs.BoolVar(&opts.Sweep, "sweep", false, "report debug prints and other leftovers in the diff")
	fs.BoolVar(&settings.DryRun, "dry-run", false, "show the plan and stop")
	fs.BoolVar(&settings.Yes, "yes", false, "skip the approval prompt")
	showVersion := fs.Bool("version", false, "print the version and exit")

	// Message style. Each of these overrides the repository's .preen.toml.
	var punctuation, body string
	fs.BoolVar(&opts.Style.NoEmDash, "no-emdash", false, "forbid em and en dashes in messages")
	fs.BoolVar(&opts.Style.NoSemicolon, "no-semicolon", false, "forbid semicolons in messages")
	fs.BoolVar(&opts.Style.NoHyphen, "no-hyphen", false, "forbid hyphens in messages")
	fs.IntVar(&opts.Style.MaxSubject, "max-subject", 0, "cap the subject length")
	fs.StringVar(&punctuation, "punctuation", "", "terminal punctuation: auto, always, or never")
	fs.BoolVar(&opts.Style.LowerSubject, "lower-subject", false, "lowercase the subject's first letter")
	fs.BoolVar(&opts.Style.Conventional, "conventional", false, "shape subjects as Conventional Commits")
	fs.StringVar(&opts.Style.Prefix, "prefix", "", "prepend this text to every subject")
	fs.BoolVar(&opts.Style.SignOff, "sign-off", false, "add a Signed-off-by trailer")
	fs.StringVar(&body, "body", "", "when to include a message body: auto, always, or never")
	fs.BoolVar(&opts.IncludeFiles, "include-files", false, "list the touched paths in each body")
	fs.BoolVar(&opts.IncludeLineNumbers, "include-line-numbers", false,
		"cite each file's changed line ranges in the body")
	fs.StringVar(&opts.Style.Signer, "signer", "", "identity for the sign-off trailer")

	if err := fs.Parse(args); err != nil {
		return run.Options{}, promptSettings{}, err
	}
	if *showVersion {
		env.printf("preen %s\n", Version)
		return run.Options{}, promptSettings{}, flag.ErrHelp
	}
	opts.Scope = scope
	if punctuation != "" {
		switch style.Punctuation(punctuation) {
		case style.PunctAuto, style.PunctAlways, style.PunctNever:
			opts.Style.Punctuation = style.Punctuation(punctuation)
		default:
			return run.Options{}, promptSettings{},
				fmt.Errorf("%w: --punctuation must be auto, always, or never", ErrUsage)
		}
	}
	if body != "" {
		switch style.BodyMode(body) {
		case style.BodyAuto, style.BodyAlways, style.BodyNever:
			opts.Style.Body = style.BodyMode(body)
		default:
			return run.Options{}, promptSettings{},
				fmt.Errorf("%w: --body must be auto, always, or never", ErrUsage)
		}
	}
	if spread == "auto" {
		opts.SpreadAuto = true
	} else if spread != "" {
		window, err := time.ParseDuration(spread)
		if err != nil {
			return run.Options{}, promptSettings{}, fmt.Errorf("%w: --spread %q: %w", ErrUsage, spread, err)
		}
		if window <= 0 {
			return run.Options{}, promptSettings{}, fmt.Errorf("%w: --spread must be positive", ErrUsage)
		}
		opts.Spread = window
	}
	settings.Grouper = grouperCmd
	return opts, settings, nil
}

// promptSettings are the decisions the CLI owns rather than the engine.
type promptSettings struct {
	// DryRun stops after the plan is shown.
	DryRun bool
	// Yes skips the approval prompt.
	Yes bool
	// Grouper is a program to group the changes instead of the built-in rules.
	Grouper string
}

// reportRun prints what a completed run did and how to undo it. The undo line
// is always last, since it is what a surprised reader reaches for.
func reportRun(env *environment, result *run.Result) {
	env.printf("\nCreated %d commit%s:\n", len(result.Commits), plural(len(result.Commits)))
	for i, commit := range result.Commits {
		env.printf("  %d. %s  %s\n", i+1, short(commit.Hash), commit.Subject)
	}
	if len(result.Reformatted) > 0 {
		env.printf("\nA commit hook reformatted: %s\n", strings.Join(result.Reformatted, ", "))
	}
	env.printf("\nContent verified unchanged (%s).\n", short(result.TreeStart))
	env.printf("Undo with: preen restore %s\n", result.Backup)
}

// confirm asks a yes or no question, defaulting to no on anything else.
func confirm(env *environment, prompt string) (bool, error) {
	env.print(prompt)
	reader := bufio.NewReader(env.In)
	answer, err := reader.ReadString('\n')
	if err != nil && !errors.Is(err, io.EOF) {
		return false, err
	}
	switch strings.ToLower(strings.TrimSpace(answer)) {
	case "y", "yes":
		return true, nil
	default:
		return false, nil
	}
}

// stringList collects a repeatable string flag.
type stringList []string

// String renders the collected values.
func (s *stringList) String() string { return strings.Join(*s, ",") }

// Set appends a value.
func (s *stringList) Set(value string) error {
	*s = append(*s, value)
	return nil
}

// short abbreviates a hash for display.
func short(hash string) string {
	if len(hash) < 8 {
		return hash
	}
	return hash[:8]
}

// plural returns the plural suffix for a count.
func plural(n int) string {
	if n == 1 {
		return ""
	}
	return "s"
}
