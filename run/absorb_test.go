package run

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// addRemote gives the harness a bare origin and pushes main, so the engine has
// real remote tracking refs to decide what is published.
func (h *harness) addRemote() {
	h.T.Helper()
	bare := filepath.Join(h.T.TempDir(), "origin.git")
	cmd := exec.Command("git", "init", "--bare", "-b", "main", bare)
	cmd.Env = append(os.Environ(), "GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_SYSTEM=/dev/null")
	if out, err := cmd.CombinedOutput(); err != nil {
		h.T.Fatalf("init bare: %v: %s", err, out)
	}
	h.git("remote", "add", "origin", bare)
	h.git("push", "-u", "origin", "main")
}

// TestAbsorbRedoesUnpushedCommits checks the mode that fixes a run of sloppy
// local commits: they come back into the tree and are recorded again, cleanly,
// with the content unchanged.
func TestAbsorbRedoesUnpushedCommits(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	h := newHarness(t)
	h.addRemote()
	published := strings.TrimSpace(h.git("rev-parse", "HEAD"))

	// Three sloppy commits nobody has seen, mixing concerns as people do.
	h.write("api/server.go", "package api\n")
	h.write("docs/guide.md", "# guide\n")
	h.git("add", "-A")
	h.git("commit", "-m", "wip")
	h.write("store/db.go", "package store\n")
	h.git("add", "-A")
	h.git("commit", "-m", "more wip")
	h.write("go.mod", "module example.com/project\n")
	h.git("add", "-A")
	h.git("commit", "-m", "asdf")

	p, err := h.Plan(ctx, Options{Absorb: true})
	if err != nil {
		t.Fatalf("Plan: %v", err)
	}
	if !p.Resets() {
		t.Fatal("an absorb plan does not reset, so nothing would be redone")
	}
	if p.Base != published {
		t.Errorf("base = %s, want the last published commit %s", p.Base, published)
	}
	if len(p.Absorbed) != 3 {
		t.Errorf("absorbing %d commits, want the 3 unpushed ones", len(p.Absorbed))
	}
	if p.MergeSummary == "" {
		t.Error("a resetting plan carries no merge audit line")
	}

	result, err := h.Apply(ctx, p, Options{Absorb: true})
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if result.TreeStart != result.TreeEnd {
		t.Errorf("content moved during an absorb: %s to %s", result.TreeStart, result.TreeEnd)
	}
	// The sloppy messages are gone and the published commit is untouched.
	log := h.git("log", "--format=%s")
	for _, gone := range []string{"wip", "more wip", "asdf"} {
		for _, line := range strings.Split(strings.TrimSpace(log), "\n") {
			if line == gone {
				t.Errorf("original message %q survived the absorb:\n%s", gone, log)
			}
		}
	}
	if !strings.Contains(log, "Initial commit") {
		t.Errorf("the published commit was rewritten:\n%s", log)
	}
	if status := h.git("status", "--porcelain=v1"); strings.TrimSpace(status) != "" {
		t.Errorf("tree not clean after the absorb:\n%s", status)
	}
}

// TestAbsorbRefusesPushedCommits checks the rule that protects other people:
// commits a remote already has are never redone without an explicit ask.
func TestAbsorbRefusesPushedCommits(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	h := newHarness(t)
	h.addRemote()

	// A commit that is pushed, so redoing it would rewrite published history.
	h.write("api/server.go", "package api\n")
	h.git("add", "-A")
	h.git("commit", "-m", "Published work")
	h.git("push", "origin", "main")

	// A local commit on top, which on its own would be fine to redo.
	h.write("store/db.go", "package store\n")
	h.git("add", "-A")
	h.git("commit", "-m", "local wip")

	p, err := h.Plan(ctx, Options{Absorb: true})
	if err != nil {
		t.Fatalf("Plan: %v", err)
	}
	// The base must sit at the published tip, so only the local commit is redone.
	if len(p.Absorbed) != 1 {
		t.Errorf("absorbing %d commits, want only the unpushed one: %+v", len(p.Absorbed), p.Absorbed)
	}
	for _, commit := range p.Absorbed {
		if commit.Subject == "Published work" {
			t.Error("a pushed commit was included in the absorb range")
		}
	}
}

// TestAbsorbNothingUnpushed checks that a branch level with its remote and a
// clean tree reports nothing to do rather than inventing work.
func TestAbsorbNothingUnpushed(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	h := newHarness(t)
	h.addRemote()

	if _, err := h.Plan(ctx, Options{Absorb: true}); !errors.Is(err, ErrNothingToDo) {
		t.Errorf("Plan = %v, want ErrNothingToDo", err)
	}
}
