package cmd

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dcadolph/preen/plan"
	"github.com/dcadolph/preen/repo"
	"github.com/dcadolph/preen/run"
)

// cli is a repository plus the streams a command reads and writes, so a test
// drives the real command line without touching the terminal or the process
// working directory.
type cli struct {
	// Dir is the repository root.
	Dir string
	// Out captures normal output.
	Out strings.Builder
	// Err captures error output.
	Err strings.Builder
	// T is the owning test.
	T *testing.T
}

// newCLI builds a repository with one commit and a CLI harness over it.
func newCLI(t *testing.T) *cli {
	t.Helper()
	dir := t.TempDir()
	c := &cli{Dir: dir, T: t}
	c.git("init", "-b", "main", ".")
	c.git("config", "user.name", "preen test")
	c.git("config", "user.email", "test@example.com")
	c.git("config", "commit.gpgsign", "false")
	c.write("README.md", "# project\n")
	c.git("add", "-A")
	c.git("commit", "-m", "Initial commit")
	return c
}

// git runs a git command in the repository.
func (c *cli) git(args ...string) string {
	c.T.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = c.Dir
	cmd.Env = append(os.Environ(),
		"GIT_CONFIG_GLOBAL=/dev/null",
		"GIT_CONFIG_SYSTEM=/dev/null",
		"GIT_AUTHOR_NAME=preen test",
		"GIT_AUTHOR_EMAIL=test@example.com",
		"GIT_COMMITTER_NAME=preen test",
		"GIT_COMMITTER_EMAIL=test@example.com",
	)
	out, err := cmd.CombinedOutput()
	if err != nil {
		c.T.Fatalf("git %v: %v: %s", args, err, out)
	}
	return string(out)
}

// write creates a file in the working tree.
func (c *cli) write(path, content string) {
	c.T.Helper()
	full := filepath.Join(c.Dir, path)
	if err := os.MkdirAll(filepath.Dir(full), 0o750); err != nil {
		c.T.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(full, []byte(content), 0o600); err != nil {
		c.T.Fatalf("write: %v", err)
	}
}

// mess writes a spread of changes across categories.
func (c *cli) mess() {
	c.write("go.mod", "module example.com/project\n")
	c.write("api/server.go", "package api\n")
	c.write("docs/guide.md", "# guide\n")
}

// run drives dispatch with the given stdin and returns the exit code, the
// captured output, and any error.
func (c *cli) run(stdin string, args ...string) (int, string, error) {
	c.T.Helper()
	c.Out.Reset()
	c.Err.Reset()
	env := &environment{Out: &c.Out, Err: &c.Err, In: strings.NewReader(stdin), Dir: c.Dir}
	code, err := dispatch(context.Background(), env, args)
	return code, c.Out.String(), err
}

// TestDispatchServesHelpAndVersion checks the routes that must never touch a
// repository.
func TestDispatchServesHelpAndVersion(t *testing.T) {
	t.Parallel()

	tests := []struct {
		Name string
		Args []string
		Want string
	}{
		{Name: "help", Args: []string{"help"}, Want: "usage: preen"},
		{Name: "short help", Args: []string{"-h"}, Want: "usage: preen"},
		{Name: "long help", Args: []string{"--help"}, Want: "usage: preen"},
		{Name: "version", Args: []string{"--version"}, Want: "preen "},
	}

	for testNum, test := range tests {
		t.Run(fmt.Sprintf("test %d %s", testNum, test.Name), func(t *testing.T) {
			t.Parallel()
			c := newCLI(t)
			code, out, err := c.run("", test.Args...)
			if err != nil {
				t.Fatalf("dispatch: %v", err)
			}
			if code != CodeOK {
				t.Errorf("code = %d, want %d", code, CodeOK)
			}
			if !strings.Contains(out, test.Want) {
				t.Errorf("output missing %q:\n%s", test.Want, out)
			}
		})
	}
}

// TestRunDryRunChangesNothing checks that a dry run shows the plan and leaves
// the repository exactly as it was.
func TestRunDryRunChangesNothing(t *testing.T) {
	t.Parallel()
	c := newCLI(t)
	c.mess()
	before := strings.TrimSpace(c.git("rev-parse", "HEAD"))

	code, out, err := c.run("", "--dry-run")
	if err != nil {
		t.Fatalf("dispatch: %v", err)
	}
	if code != CodeOK {
		t.Errorf("code = %d, want %d", code, CodeOK)
	}
	if !strings.Contains(out, "Planned commits") || !strings.Contains(out, "Dry run") {
		t.Errorf("dry run output looks wrong:\n%s", out)
	}
	if after := strings.TrimSpace(c.git("rev-parse", "HEAD")); after != before {
		t.Error("a dry run moved HEAD")
	}
}

// TestRunDeclinedAtPrompt checks that answering no leaves everything alone and
// reports the declined code.
func TestRunDeclinedAtPrompt(t *testing.T) {
	t.Parallel()
	c := newCLI(t)
	c.mess()
	before := strings.TrimSpace(c.git("rev-parse", "HEAD"))

	code, out, err := c.run("n\n")
	if err != nil {
		t.Fatalf("dispatch: %v", err)
	}
	if code != CodeAborted {
		t.Errorf("code = %d, want %d for a declined plan", code, CodeAborted)
	}
	if !strings.Contains(out, "Aborted") {
		t.Errorf("output does not say it aborted:\n%s", out)
	}
	if after := strings.TrimSpace(c.git("rev-parse", "HEAD")); after != before {
		t.Error("declining the plan still moved HEAD")
	}
}

// TestRunApprovedAtPrompt checks the whole path: plan, approve, commit, and a
// report that says how to undo it.
func TestRunApprovedAtPrompt(t *testing.T) {
	t.Parallel()
	c := newCLI(t)
	c.mess()

	code, out, err := c.run("y\n")
	if err != nil {
		t.Fatalf("dispatch: %v", err)
	}
	if code != CodeOK {
		t.Errorf("code = %d, want %d", code, CodeOK)
	}
	for _, want := range []string{"Created", "Content verified unchanged", "preen restore"} {
		if !strings.Contains(out, want) {
			t.Errorf("report missing %q:\n%s", want, out)
		}
	}
	if status := c.git("status", "--porcelain=v1"); strings.TrimSpace(status) != "" {
		t.Errorf("tree not clean:\n%s", status)
	}
}

// TestRunEditsPlanAtPrompt checks that an edit typed at the prompt reaches the
// commits that get recorded.
func TestRunEditsPlanAtPrompt(t *testing.T) {
	t.Parallel()
	c := newCLI(t)
	c.mess()

	code, out, err := c.run("reword 1 Bump the module\ny\n")
	if err != nil {
		t.Fatalf("dispatch: %v", err)
	}
	if code != CodeOK {
		t.Errorf("code = %d, want %d", code, CodeOK)
	}
	if !strings.Contains(out, "Edit applied") {
		t.Errorf("the edit was not acknowledged:\n%s", out)
	}
	if log := c.git("log", "--format=%s"); !strings.Contains(log, "Bump the module") {
		t.Errorf("the reworded subject was not recorded:\n%s", log)
	}
}

// TestRunNothingToDo checks the clean-tree case reports its own code rather
// than an error.
func TestRunNothingToDo(t *testing.T) {
	t.Parallel()
	c := newCLI(t)

	code, out, err := c.run("")
	if err != nil {
		t.Fatalf("dispatch: %v", err)
	}
	if code != CodeNothingToDo {
		t.Errorf("code = %d, want %d", code, CodeNothingToDo)
	}
	if !strings.Contains(out, "Nothing to preen") {
		t.Errorf("output:\n%s", out)
	}
}

// TestRestoreRoundTrip checks that the restore command undoes a run and puts
// the work back in the working tree.
func TestRestoreRoundTrip(t *testing.T) {
	t.Parallel()
	c := newCLI(t)
	c.mess()
	before := strings.TrimSpace(c.git("rev-parse", "HEAD"))

	if code, _, err := c.run("", "--yes"); err != nil || code != CodeOK {
		t.Fatalf("run: code %d: %v", code, err)
	}
	code, out, err := c.run("", "restore", "--yes")
	if err != nil {
		t.Fatalf("restore: %v", err)
	}
	if code != CodeOK {
		t.Errorf("code = %d, want %d", code, CodeOK)
	}
	if !strings.Contains(out, "Restored to preen-backup/") {
		t.Errorf("restore output:\n%s", out)
	}
	if after := strings.TrimSpace(c.git("rev-parse", "HEAD")); after != before {
		t.Errorf("HEAD is %s after restore, want %s", after, before)
	}
	status := c.git("status", "--porcelain=v1", "--untracked-files=all")
	for _, path := range []string{"go.mod", "api/server.go", "docs/guide.md"} {
		if !strings.Contains(status, path) {
			t.Errorf("%s did not come back to the tree:\n%s", path, status)
		}
	}
}

// TestRestoreWithNoBackups checks the empty case reports a clear error rather
// than panicking on an empty list.
func TestRestoreWithNoBackups(t *testing.T) {
	t.Parallel()
	c := newCLI(t)
	_, _, err := c.run("", "restore", "--yes")
	if !errors.Is(err, ErrNoBackups) {
		t.Errorf("restore with no backups = %v, want ErrNoBackups", err)
	}
}

// TestBackupsListAndPrune checks the listing and that pruning only removes
// refs the branch already contains.
func TestBackupsListAndPrune(t *testing.T) {
	t.Parallel()
	c := newCLI(t)
	c.mess()
	if code, _, err := c.run("", "--yes"); err != nil || code != CodeOK {
		t.Fatalf("run: code %d: %v", code, err)
	}

	code, out, err := c.run("", "backups")
	if err != nil {
		t.Fatalf("backups: %v", err)
	}
	if code != CodeOK || !strings.Contains(out, "preen-backup/") {
		t.Errorf("backups listing looks wrong (code %d):\n%s", code, out)
	}
	// The backup is behind the branch now, so it is prunable.
	code, out, err = c.run("", "backups", "--prune", "--yes")
	if err != nil {
		t.Fatalf("prune: %v", err)
	}
	if code != CodeOK || !strings.Contains(out, "Deleted") {
		t.Errorf("prune output (code %d):\n%s", code, out)
	}
	if refs := c.git("for-each-ref", "refs/heads/preen-backup/"); strings.TrimSpace(refs) != "" {
		t.Errorf("a backup survived the prune:\n%s", refs)
	}
}

// TestScopeFlagLimitsTheRun checks the flag reaches the engine.
func TestScopeFlagLimitsTheRun(t *testing.T) {
	t.Parallel()
	c := newCLI(t)
	c.mess()

	if code, _, err := c.run("", "--yes", "--scope", "api"); err != nil || code != CodeOK {
		t.Fatalf("run: code %d: %v", code, err)
	}
	committed := c.git("show", "--name-only", "--format=", "HEAD")
	if !strings.Contains(committed, "api/server.go") {
		t.Errorf("the in-scope file was not committed:\n%s", committed)
	}
	status := c.git("status", "--porcelain=v1", "--untracked-files=all")
	if !strings.Contains(status, "docs/guide.md") {
		t.Errorf("out-of-scope work was committed:\n%s", status)
	}
}

// TestConfigFileAppliesStyle checks that a repository's .preen.toml reaches
// the messages, and that a flag overrides it.
func TestConfigFileAppliesStyle(t *testing.T) {
	t.Parallel()
	c := newCLI(t)
	c.write(".preen.toml", "[commit]\nconventional = true\n")
	c.mess()

	if _, out, err := c.run("", "--dry-run"); err != nil {
		t.Fatalf("dry run: %v", err)
	} else if !strings.Contains(out, "build: update dependencies") {
		t.Errorf("the config style was not applied:\n%s", out)
	}
	if _, out, err := c.run("", "--dry-run", "--conventional=false", "--prefix", "ABC-1"); err != nil {
		t.Fatalf("dry run: %v", err)
	} else if !strings.Contains(out, "ABC-1 Update dependencies") {
		t.Errorf("the flag did not override the config:\n%s", out)
	}
}

// TestBadFlagsAreRejected checks that unusable values fail before anything is
// touched.
func TestBadFlagsAreRejected(t *testing.T) {
	t.Parallel()

	tests := []struct {
		Name string
		Args []string
	}{
		{Name: "bad punctuation", Args: []string{"--punctuation", "sometimes"}},
		{Name: "bad spread", Args: []string{"--spread", "soon"}},
		{Name: "negative spread", Args: []string{"--spread", "-2h"}},
		{Name: "unknown flag", Args: []string{"--nope"}},
	}

	for testNum, test := range tests {
		t.Run(fmt.Sprintf("test %d %s", testNum, test.Name), func(t *testing.T) {
			t.Parallel()
			c := newCLI(t)
			c.mess()
			code, _, err := c.run("", test.Args...)
			if err == nil {
				t.Fatalf("args %v were accepted", test.Args)
			}
			if code == CodeOK {
				t.Errorf("code = %d, want a failure", code)
			}
			if status := c.git("status", "--porcelain=v1"); !strings.Contains(status, "go.mod") {
				t.Error("a rejected invocation still touched the tree")
			}
		})
	}
}

// TestNotARepository checks the error and code outside a git repository.
func TestNotARepository(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	var out, errOut strings.Builder
	env := &environment{Out: &out, Err: &errOut, In: strings.NewReader(""), Dir: dir}

	code, err := dispatch(context.Background(), env, nil)
	if !errors.Is(err, repo.ErrNotRepo) {
		t.Errorf("error = %v, want ErrNotRepo", err)
	}
	if code != CodeNoRepo {
		t.Errorf("code = %d, want %d", code, CodeNoRepo)
	}
}

// TestExitCode checks that each failure maps onto its own code, since scripts
// tell the outcomes apart by number.
func TestExitCode(t *testing.T) {
	t.Parallel()

	tests := []struct {
		Name string
		In   error
		Want int
	}{
		{Name: "nil", In: nil, Want: CodeOK},
		{Name: "not a repo", In: repo.ErrNotRepo, Want: CodeNoRepo},
		{Name: "mid operation", In: repo.ErrInProgress, Want: CodeRepoState},
		{Name: "nothing to do", In: run.ErrNothingToDo, Want: CodeNothingToDo},
		{Name: "content changed", In: run.ErrContentChanged, Want: CodeContentChanged},
		{Name: "gate failed", In: run.ErrGateFailed, Want: CodeGateFailed},
		{Name: "invalid plan", In: plan.ErrInvalid, Want: CodeInvalidPlan},
		{Name: "aborted", In: ErrAborted, Want: CodeAborted},
		{Name: "wrapped", In: fmt.Errorf("context: %w", run.ErrGateFailed), Want: CodeGateFailed},
		{Name: "unknown", In: errors.New("something else"), Want: CodeErr},
	}

	for testNum, test := range tests {
		t.Run(fmt.Sprintf("test %d %s", testNum, test.Name), func(t *testing.T) {
			t.Parallel()
			if got := exitCode(test.In); got != test.Want {
				t.Errorf("exitCode(%v) = %d, want %d", test.In, got, test.Want)
			}
		})
	}
}

// TestFlagGiven checks the precedence helper that keeps a config value from
// being overwritten by a flag's zero value.
func TestFlagGiven(t *testing.T) {
	t.Parallel()

	tests := []struct {
		Name string
		Args []string
		Flag string
		Want bool
	}{
		{Name: "long form", Args: []string{"--conventional"}, Flag: "conventional", Want: true},
		{Name: "with value", Args: []string{"--prefix", "ABC"}, Flag: "prefix", Want: true},
		{Name: "equals form", Args: []string{"--prefix=ABC"}, Flag: "prefix", Want: true},
		{Name: "single dash", Args: []string{"-prefix", "ABC"}, Flag: "prefix", Want: true},
		{Name: "absent", Args: []string{"--yes"}, Flag: "prefix", Want: false},
		{Name: "not a prefix match", Args: []string{"--prefixed"}, Flag: "prefix", Want: false},
		{Name: "after separator", Args: []string{"--", "--prefix"}, Flag: "prefix", Want: false},
	}

	for testNum, test := range tests {
		t.Run(fmt.Sprintf("test %d %s", testNum, test.Name), func(t *testing.T) {
			t.Parallel()
			if got := flagGiven(test.Args, test.Flag); got != test.Want {
				t.Errorf("flagGiven(%v, %q) = %v, want %v", test.Args, test.Flag, got, test.Want)
			}
		})
	}
}

// addRemote gives the CLI harness a bare origin and pushes main, so unpushed
// commits are distinguishable from published ones.
func (c *cli) addRemote() {
	c.T.Helper()
	bare := filepath.Join(c.T.TempDir(), "origin.git")
	cmd := exec.Command("git", "init", "--bare", "-b", "main", bare)
	cmd.Env = append(os.Environ(), "GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_SYSTEM=/dev/null")
	if out, err := cmd.CombinedOutput(); err != nil {
		c.T.Fatalf("init bare: %v: %s", err, out)
	}
	c.git("remote", "add", "origin", bare)
	c.git("push", "-u", "origin", "main")
}

// TestFixupCommandFoldsChangesIn checks the --fixup route end to end through
// the command line, including its own confirmation prompt.
func TestFixupCommandFoldsChangesIn(t *testing.T) {
	t.Parallel()
	c := newCLI(t)
	c.addRemote()
	c.write("api/server.go", "package api\n")
	c.git("add", "-A")
	c.git("commit", "-m", "Add the api server")
	c.write("api/server.go", "package api\n\nfunc Serve() {}\n")

	before := len(strings.Split(strings.TrimSpace(c.git("log", "--format=%H")), "\n"))
	code, out, err := c.run("y\n", "--fixup")
	if err != nil {
		t.Fatalf("fixup: %v", err)
	}
	if code != CodeOK {
		t.Errorf("code = %d, want %d", code, CodeOK)
	}
	if !strings.Contains(out, "fixup!") {
		t.Errorf("the plan did not name its targets:\n%s", out)
	}
	after := len(strings.Split(strings.TrimSpace(c.git("log", "--format=%H")), "\n"))
	if after != before {
		t.Errorf("history went from %d commits to %d, want the fixups squashed away", before, after)
	}
	if status := c.git("status", "--porcelain=v1"); strings.TrimSpace(status) != "" {
		t.Errorf("tree not clean after the fixup:\n%s", status)
	}
}

// TestFixupCommandDeclined checks that declining the fixup prompt changes
// nothing.
func TestFixupCommandDeclined(t *testing.T) {
	t.Parallel()
	c := newCLI(t)
	c.addRemote()
	c.write("api/server.go", "package api\n")
	c.git("add", "-A")
	c.git("commit", "-m", "Add the api server")
	c.write("api/server.go", "package api\n\nfunc Serve() {}\n")
	before := strings.TrimSpace(c.git("rev-parse", "HEAD"))

	code, _, err := c.run("n\n", "--fixup")
	if err != nil {
		t.Fatalf("fixup: %v", err)
	}
	if code != CodeAborted {
		t.Errorf("code = %d, want %d", code, CodeAborted)
	}
	if after := strings.TrimSpace(c.git("rev-parse", "HEAD")); after != before {
		t.Error("declining the fixup still moved HEAD")
	}
}

// TestFixupCommandNothingToDo checks the clean-tree case for --fixup.
func TestFixupCommandNothingToDo(t *testing.T) {
	t.Parallel()
	c := newCLI(t)
	c.addRemote()

	code, out, err := c.run("", "--fixup")
	if err != nil {
		t.Fatalf("fixup: %v", err)
	}
	if code != CodeNothingToDo {
		t.Errorf("code = %d, want %d", code, CodeNothingToDo)
	}
	if !strings.Contains(out, "Nothing to fold in") {
		t.Errorf("output:\n%s", out)
	}
}
