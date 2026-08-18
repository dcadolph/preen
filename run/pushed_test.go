package run

import (
	"context"
	"errors"
	"strings"
	"testing"
)

// TestPushedRewriteRefusesProtectedBranch checks the second consent a rewrite
// of published history needs: the explicit ask is not enough on a branch that
// is shared by name.
func TestPushedRewriteRefusesProtectedBranch(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	h := newHarness(t)
	h.addRemote()

	h.write("api/server.go", "package api\n")
	h.git("add", "-A")
	h.git("commit", "-m", "sloppy")
	h.git("push", "origin", "main")

	// main is protected by default, so the ask alone must not be enough.
	_, err := h.Plan(ctx, Options{Pushed: true, PushedBase: "HEAD~1"})
	if !errors.Is(err, ErrProtectedBranch) {
		t.Errorf("Plan on main = %v, want a protected branch refusal", err)
	}

	// With the second consent it proceeds.
	built, err := h.Plan(ctx, Options{Pushed: true, PushedBase: "HEAD~1", AllowProtected: true})
	if err != nil {
		t.Fatalf("Plan with --allow-protected: %v", err)
	}
	if built.Push == "" {
		t.Error("a published rewrite carries no push preview")
	}
	if !strings.Contains(built.Push, "--force-with-lease") {
		t.Errorf("push preview = %q, want a lease rather than a plain force", built.Push)
	}
}

// TestPushedRewriteHonorsConfiguredProtection checks that a branch the project
// declared off limits is refused the same way the built-in names are.
func TestPushedRewriteHonorsConfiguredProtection(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	h := newHarness(t)
	h.addRemote()
	h.git("checkout", "-q", "-b", "release/2026.08")
	h.write("api/server.go", "package api\n")
	h.git("add", "-A")
	h.git("commit", "-m", "sloppy")
	h.git("push", "-q", "-u", "origin", "release/2026.08")

	_, err := h.Plan(ctx, Options{
		Pushed:     true,
		PushedBase: "HEAD~1",
		Protected:  []string{"release/*"},
	})
	if !errors.Is(err, ErrProtectedBranch) {
		t.Errorf("Plan on a configured protected branch = %v, want a refusal", err)
	}
}

// TestPushedRewriteNeedsBaseOnDefaultBranch checks that a rewrite with no
// boundary to work from asks for one instead of guessing how far back to go.
func TestPushedRewriteNeedsBaseOnDefaultBranch(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	h := newHarness(t)
	h.addRemote()
	h.write("api/server.go", "package api\n")
	h.git("add", "-A")
	h.git("commit", "-m", "sloppy")
	h.git("push", "origin", "main")

	_, err := h.Plan(ctx, Options{Pushed: true, AllowProtected: true})
	if !errors.Is(err, ErrNeedBase) {
		t.Errorf("Plan without a base on the default branch = %v, want ErrNeedBase", err)
	}
}

// TestPushedRewriteRedoesPublishedCommits checks the mode itself on a feature
// branch, where the whole point is that published commits are redone.
func TestPushedRewriteRedoesPublishedCommits(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	h := newHarness(t)
	h.addRemote()
	h.git("checkout", "-q", "-b", "feature")
	base := strings.TrimSpace(h.git("rev-parse", "HEAD"))

	h.write("api/server.go", "package api\n")
	h.write("docs/guide.md", "# guide\n")
	h.git("add", "-A")
	h.git("commit", "-m", "wip")
	h.git("push", "-q", "-u", "origin", "feature")

	built, err := h.Plan(ctx, Options{Pushed: true, PushedBase: base})
	if err != nil {
		t.Fatalf("Plan: %v", err)
	}
	if len(built.Absorbed) != 1 {
		t.Fatalf("redoing %d commits, want the one published commit", len(built.Absorbed))
	}
	result, err := h.Apply(ctx, built, Options{Pushed: true, PushedBase: base})
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if result.TreeStart != result.TreeEnd {
		t.Errorf("content moved during a published rewrite: %s to %s", result.TreeStart, result.TreeEnd)
	}
	if log := h.git("log", "--format=%s"); strings.Contains(log, "wip") {
		t.Errorf("the sloppy message survived:\n%s", log)
	}
	// The rewrite stays local until it is pushed, which is a separate consent.
	remote := h.git("log", "--format=%s", "origin/feature")
	if !strings.Contains(remote, "wip") {
		t.Errorf("the remote changed without a push:\n%s", remote)
	}
}
