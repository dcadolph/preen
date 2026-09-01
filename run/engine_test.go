package run

import (
	"context"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/dcadolph/preen/repo"
)

// harness is a real repository plus an engine wired to it.
type harness struct {
	// Engine is the engine under test.
	*Engine
	// Dir is the working tree root.
	Dir string
	// T is the owning test.
	T *testing.T
}

// newHarness builds a repository with one commit and an engine over it.
func newHarness(t *testing.T) *harness {
	t.Helper()
	dir := t.TempDir()
	env := append(os.Environ(),
		"GIT_CONFIG_GLOBAL=/dev/null",
		"GIT_CONFIG_SYSTEM=/dev/null",
		"GIT_AUTHOR_NAME=preen test",
		"GIT_AUTHOR_EMAIL=test@example.com",
		"GIT_COMMITTER_NAME=preen test",
		"GIT_COMMITTER_EMAIL=test@example.com",
	)
	run := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		cmd.Env = env
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v: %s", args, err, out)
		}
	}
	run("init", "-b", "main")
	run("config", "user.name", "preen test")
	run("config", "user.email", "test@example.com")
	run("config", "commit.gpgsign", "false")

	h := &harness{Dir: dir, T: t}
	h.write("README.md", "# project\n")
	run("add", "-A")
	run("commit", "-m", "Initial commit")

	r, err := repo.Open(context.Background(), dir)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	engine := New(r)
	engine.Out = io.Discard
	engine.Now = func() time.Time { return time.Date(2026, 8, 18, 12, 0, 0, 0, time.UTC) }
	h.Engine = engine
	return h
}

// write creates or replaces a file in the working tree.
func (h *harness) write(path, content string) {
	h.T.Helper()
	full := filepath.Join(h.Dir, path)
	if err := os.MkdirAll(filepath.Dir(full), 0o750); err != nil {
		h.T.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(full, []byte(content), 0o600); err != nil {
		h.T.Fatalf("write %s: %v", path, err)
	}
}

// git runs a git command in the repository and returns its output.
func (h *harness) git(args ...string) string {
	h.T.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = h.Dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		h.T.Fatalf("git %v: %v: %s", args, err, out)
	}
	return string(out)
}

// messyTree writes a spread of changes across categories, the state preen
// exists to clean up.
func (h *harness) messyTree() {
	h.write("go.mod", "module example.com/project\n\ngo 1.26\n")
	h.write("api/server.go", "package api\n\nfunc Serve() {}\n")
	h.write("api/server_test.go", "package api\n\nfunc TestServe(t *testing.T) {}\n")
	h.write("store/db.go", "package store\n\nfunc Open() {}\n")
	h.write("docs/guide.md", "# guide\n")
	h.write(".github/workflows/ci.yml", "name: ci\n")
}

// TestPlanGroupsByCategory checks that a messy tree becomes coherent commits
// in a dependency-respecting order, with every change accounted for.
func TestPlanGroupsByCategory(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	h := newHarness(t)
	h.messyTree()

	p, err := h.Plan(ctx, Options{})
	if err != nil {
		t.Fatalf("Plan: %v", err)
	}
	if len(p.Commits) < 4 {
		t.Fatalf("got %d commits, want the categories kept apart: %+v", len(p.Commits), p.Commits)
	}
	if got := p.Commits[0].Subject; got != "Update dependencies" {
		t.Errorf("first commit = %q, want dependencies first", got)
	}
	// Documentation is the least depended-upon change and lands last.
	if got := p.Commits[len(p.Commits)-1].Subject; !strings.Contains(strings.ToLower(got), "guide") {
		t.Errorf("last commit = %q, want the documentation change", got)
	}
	// The two source packages must not be mixed into one commit.
	for _, commit := range p.Commits {
		var api, store bool
		for _, part := range commit.Parts {
			api = api || strings.HasPrefix(part.Path, "api/")
			store = store || strings.HasPrefix(part.Path, "store/")
		}
		if api && store {
			t.Errorf("commit %q mixes two packages: %v", commit.Subject, commit.Paths())
		}
	}
}

// TestApplyConservesContent checks the guarantee that matters most: after a
// run every change is committed and the content is byte for byte what it was.
func TestApplyConservesContent(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	h := newHarness(t)
	h.messyTree()

	p, err := h.Plan(ctx, Options{})
	if err != nil {
		t.Fatalf("Plan: %v", err)
	}
	result, err := h.Apply(ctx, p, Options{})
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if result.TreeStart != result.TreeEnd {
		t.Errorf("content tree moved: %s to %s", result.TreeStart, result.TreeEnd)
	}
	if len(result.Commits) != len(p.Commits) {
		t.Errorf("recorded %d commits, planned %d", len(result.Commits), len(p.Commits))
	}
	if result.Backup == "" {
		t.Error("no backup ref recorded")
	}
	if status := h.git("status", "--porcelain=v1"); strings.TrimSpace(status) != "" {
		t.Errorf("tree not clean after the run:\n%s", status)
	}
	// The backup must still point at the pre-run state so undo works.
	if out := h.git("rev-parse", "--verify", result.Backup); strings.TrimSpace(out) == "" {
		t.Error("backup ref does not resolve")
	}
}

// TestApplyRollsBackOnGateFailure checks that a failing gate leaves the
// repository exactly as it was, which is what makes the gate safe to use.
func TestApplyRollsBackOnGateFailure(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	h := newHarness(t)
	h.messyTree()
	before := strings.TrimSpace(h.git("rev-parse", "HEAD"))

	h.Gate = GateFunc(func(context.Context, string, string) ([]byte, error) {
		return []byte("boom"), errors.New("check failed")
	})
	p, err := h.Plan(ctx, Options{})
	if err != nil {
		t.Fatalf("Plan: %v", err)
	}
	_, err = h.Apply(ctx, p, Options{Gate: "false"})
	if err == nil {
		t.Fatal("Apply with a failing gate returned no error")
	}
	if !errors.Is(err, ErrGateFailed) {
		t.Errorf("error = %v, want a gate failure", err)
	}
	if after := strings.TrimSpace(h.git("rev-parse", "HEAD")); after != before {
		t.Errorf("HEAD moved despite the rollback: %s to %s", before, after)
	}
	if status := h.git("status", "--porcelain=v1"); strings.TrimSpace(status) == "" {
		t.Error("changes vanished after the rollback, want them back in the tree")
	}
}

// TestPlanNothingToDo checks that a clean tree stops rather than inventing a
// commit.
func TestPlanNothingToDo(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	h := newHarness(t)

	if _, err := h.Plan(ctx, Options{}); !errors.Is(err, ErrNothingToDo) {
		t.Errorf("Plan on a clean tree = %v, want ErrNothingToDo", err)
	}
}

// TestScopeLimitsTheRun checks that out-of-scope work stays uncommitted.
func TestScopeLimitsTheRun(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	h := newHarness(t)
	h.messyTree()

	p, err := h.Plan(ctx, Options{Scope: []string{"api"}})
	if err != nil {
		t.Fatalf("Plan: %v", err)
	}
	for _, commit := range p.Commits {
		for _, part := range commit.Parts {
			if !strings.HasPrefix(part.Path, "api/") {
				t.Errorf("out-of-scope path planned: %s", part.Path)
			}
		}
	}
	if _, err := h.Apply(ctx, p, Options{Scope: []string{"api"}}); err != nil {
		t.Fatalf("Apply: %v", err)
	}
	// Untracked directories collapse in the default listing, so the file-level
	// form is requested to assert on the exact path.
	status := h.git("status", "--porcelain=v1", "--untracked-files=all")
	for _, path := range []string{"store/db.go", "docs/guide.md", "go.mod"} {
		if !strings.Contains(status, path) {
			t.Errorf("out-of-scope path %s was committed, status:\n%s", path, status)
		}
	}
	committed := h.git("show", "--name-only", "--format=", "HEAD")
	if strings.Contains(committed, "store/") {
		t.Errorf("out-of-scope path landed in a commit:\n%s", committed)
	}
}
