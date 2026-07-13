package cmd

import (
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"slices"
	"strings"
)

// usage is the help text. Skill flags pass through untouched, so only the
// wrapper's own surface is documented here.
const usage = `usage: preen [skill flags] [-- claude flags]

Runs the preen skill through the Claude Code CLI. Every flag except the
wrapper flags below is passed to the skill unchanged: --scope, --gate,
--dry-run, --fixup, --yes, --spread, --pushed, message style flags, and
anything else the skill understands.

Wrapper flags:
  --headless        Run claude non-interactively (claude -p) and add --yes
                    to the skill invocation so the plan applies without an
                    approval prompt.
  --claude-bin BIN  Claude Code binary to run. Default: claude.
  --version         Print the version and exit.
  -h, --help        Print this help and exit.

Arguments after -- go to the claude CLI verbatim. In headless mode the
wrapper adds --permission-mode acceptEdits and --allowedTools "Bash(git:*)"
unless the claude flags already set a permission option.

Examples:
  preen
  preen --fixup --scope internal/
  preen --headless --gate 'go test ./...'
  preen --headless -- --model opus --max-turns 80
`

// options holds parsed wrapper flags plus the argument groups forwarded to
// the skill prompt and the claude command line.
type options struct {
	// ClaudeBin is the claude executable to run.
	ClaudeBin string
	// SkillArgs are forwarded inside the /preen prompt.
	SkillArgs []string
	// ClaudeArgs are appended to the claude command line verbatim.
	ClaudeArgs []string
	// Headless selects non-interactive claude with --yes appended.
	Headless bool
	// ShowVersion prints the version and exits.
	ShowVersion bool
	// ShowHelp prints usage and exits.
	ShowHelp bool
}

// runner executes the wrapped claude invocation with injectable dependencies
// so tests run without a claude binary or a real repository.
type runner struct {
	// Skill is the embedded skill text claude is told to follow.
	Skill string
	// LookPath resolves the claude binary path.
	LookPath func(bin string) (string, error)
	// CheckRepo verifies the working directory holds a usable repository.
	CheckRepo func() error
	// TempSkill writes the skill text to a temp file and returns its path
	// and a cleanup function.
	TempSkill func(content string) (string, func(), error)
	// Start launches claude and returns its exit code.
	Start func(bin string, argv []string) (int, error)
	// Out receives help and version output.
	Out io.Writer
}

// newRunner returns a runner wired to the real environment.
func newRunner(skill string) *runner {
	return &runner{
		Skill:     skill,
		LookPath:  exec.LookPath,
		CheckRepo: checkRepo,
		TempSkill: writeTempSkill,
		Start:     startClaude,
		Out:       os.Stdout,
	}
}

// Run parses args, runs preflight checks, and launches claude. It returns
// the process exit code and any error worth printing.
func (r *runner) Run(args []string) (int, error) {
	opts, err := splitArgs(args)
	if err != nil {
		return CodeErr, err
	}
	if opts.ShowHelp {
		_, _ = fmt.Fprint(r.Out, usage)
		return CodeOK, nil
	}
	if opts.ShowVersion {
		_, _ = fmt.Fprintf(r.Out, "preen %s\n", Version)
		return CodeOK, nil
	}
	bin, err := r.LookPath(opts.ClaudeBin)
	if err != nil {
		return CodeNoClaude, fmt.Errorf("%w: %q: install Claude Code or pass --claude-bin", ErrNoClaude, opts.ClaudeBin)
	}
	if err := r.CheckRepo(); err != nil {
		return repoCode(err), err
	}
	skillPath, cleanup, err := r.TempSkill(r.Skill)
	if err != nil {
		return CodeErr, fmt.Errorf("skill file write failed: %w", err)
	}
	defer cleanup()
	prompt := instruction(skillPath, composePrompt(opts.SkillArgs, opts.Headless))
	return r.Start(bin, claudeArgv(opts, prompt))
}

// instruction wraps the skill invocation in a directive to follow this
// build's pinned skill text, overriding any installed preen skill or plugin.
func instruction(skillPath, invocation string) string {
	return fmt.Sprintf("Read the file %s and follow it exactly as the preen skill for "+
		"this invocation. It overrides any other preen skill, plugin, or prior knowledge "+
		"of preen. Invocation: %s", skillPath, invocation)
}

// writeTempSkill writes the skill text to a temp file for claude to read.
func writeTempSkill(content string) (string, func(), error) {
	file, err := os.CreateTemp("", "preen-skill-*.md")
	if err != nil {
		return "", nil, err
	}
	path := file.Name()
	cleanup := func() { _ = os.Remove(path) }
	if _, err := file.WriteString(content); err != nil {
		_ = file.Close()
		cleanup()
		return "", nil, err
	}
	if err := file.Close(); err != nil {
		cleanup()
		return "", nil, err
	}
	return path, cleanup, nil
}

// splitArgs separates wrapper flags from skill and claude arguments.
// Everything after a bare -- goes to claude verbatim; unrecognized flags
// before it go to the skill in their original order.
func splitArgs(args []string) (options, error) {
	opts := options{ClaudeBin: "claude"}
	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch {
		case arg == "--":
			opts.ClaudeArgs = append(opts.ClaudeArgs, args[i+1:]...)
			return opts, nil
		case arg == "--headless":
			opts.Headless = true
		case arg == "--version":
			opts.ShowVersion = true
		case arg == "-h", arg == "--help":
			opts.ShowHelp = true
		case arg == "--claude-bin":
			if i+1 >= len(args) {
				return options{}, fmt.Errorf("%w: --claude-bin needs a value", ErrUsage)
			}
			i++
			opts.ClaudeBin = args[i]
		case strings.HasPrefix(arg, "--claude-bin="):
			opts.ClaudeBin = strings.TrimPrefix(arg, "--claude-bin=")
		default:
			opts.SkillArgs = append(opts.SkillArgs, arg)
		}
	}
	return opts, nil
}

// composePrompt builds the /preen invocation text. Headless runs get --yes
// appended when absent, since nobody is present to approve the plan.
func composePrompt(skillArgs []string, headless bool) string {
	parts := make([]string, 0, len(skillArgs)+2)
	parts = append(parts, "/preen")
	for _, arg := range skillArgs {
		parts = append(parts, quoteArg(arg))
	}
	if headless && !slices.Contains(skillArgs, "--yes") {
		parts = append(parts, "--yes")
	}
	return strings.Join(parts, " ")
}

// quoteArg single-quotes an argument for the prompt when it contains
// characters that would split or mangle it, matching what a user would type
// into a Claude session by hand.
func quoteArg(arg string) string {
	if arg == "" {
		return "''"
	}
	if !strings.ContainsAny(arg, " \t\n'\"\\$`") {
		return arg
	}
	return "'" + strings.ReplaceAll(arg, "'", `'\''`) + "'"
}

// claudeArgv assembles the claude command line. Interactive mode preloads
// the prompt into a session; headless mode uses -p and permissive-enough
// defaults for the skill's git work unless the caller set permission flags.
func claudeArgv(opts options, prompt string) []string {
	if !opts.Headless {
		return append([]string{prompt}, opts.ClaudeArgs...)
	}
	argv := []string{"-p", prompt}
	if !hasPermissionFlag(opts.ClaudeArgs) {
		argv = append(argv, "--permission-mode", "acceptEdits", "--allowedTools", "Bash(git:*)")
	}
	return append(argv, opts.ClaudeArgs...)
}

// hasPermissionFlag reports whether the claude arguments already configure
// permissions, in which case the headless defaults are omitted.
func hasPermissionFlag(args []string) bool {
	for _, arg := range args {
		for _, flag := range []string{"--permission-mode", "--allowedTools", "--dangerously-skip-permissions"} {
			if arg == flag || strings.HasPrefix(arg, flag+"=") {
				return true
			}
		}
	}
	return false
}

// startClaude runs claude with inherited stdio and mirrors its exit code.
// SIGINT is ignored in the wrapper while claude runs so Ctrl+C reaches the
// child and the wrapper survives to report its exit code.
func startClaude(bin string, argv []string) (int, error) {
	cmd := exec.Command(bin, argv...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Start(); err != nil {
		return CodeErr, fmt.Errorf("claude start failed: %w", err)
	}
	signal.Ignore(os.Interrupt)
	defer signal.Reset(os.Interrupt)
	if err := cmd.Wait(); err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			return exitErr.ExitCode(), nil
		}
		return CodeErr, fmt.Errorf("claude wait failed: %w", err)
	}
	return CodeOK, nil
}
