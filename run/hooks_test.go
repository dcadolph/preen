package run

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// installHook writes an executable git hook into the repository.
func (h *harness) installHook(name, script string) {
	h.T.Helper()
	dir := filepath.Join(h.Dir, ".git", "hooks")
	if err := os.MkdirAll(dir, 0o750); err != nil {
		h.T.Fatalf("mkdir hooks: %v", err)
	}
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(script), 0o700); err != nil { //nolint:gosec // Test fixture.
		h.T.Fatalf("write hook: %v", err)
	}
}

// TestRealGateRuns checks that the shell gate actually executes the configured
// command, since every other test substitutes a fake.
func TestRealGateRuns(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	h := newHarness(t)
	h.messyTree()
	marker := filepath.Join(t.TempDir(), "gate-ran")

	p, err := h.Plan(ctx, Options{})
	if err != nil {
		t.Fatalf("Plan: %v", err)
	}
	if _, err := h.Apply(ctx, p, Options{Gate: "echo ran >> " + marker}); err != nil {
		t.Fatalf("Apply: %v", err)
	}
	data, err := os.ReadFile(marker) //nolint:gosec // Test fixture path.
	if err != nil {
		t.Fatalf("the gate never ran: %v", err)
	}
	if got := strings.Count(string(data), "ran"); got != len(p.Commits) {
		t.Errorf("the gate ran %d times, want once per commit (%d)", got, len(p.Commits))
	}
}

// TestRealGateFailureRollsBack checks that a genuinely failing shell command,
// rather than an injected error, takes the run back.
func TestRealGateFailureRollsBack(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	h := newHarness(t)
	h.messyTree()
	before := strings.TrimSpace(h.git("rev-parse", "HEAD"))

	p, err := h.Plan(ctx, Options{})
	if err != nil {
		t.Fatalf("Plan: %v", err)
	}
	_, err = h.Apply(ctx, p, Options{Gate: "exit 3"})
	if !errors.Is(err, ErrGateFailed) {
		t.Fatalf("Apply = %v, want a gate failure", err)
	}
	if after := strings.TrimSpace(h.git("rev-parse", "HEAD")); after != before {
		t.Error("HEAD moved despite the rollback")
	}
}

// TestHookRewriteRejectedByDefault checks that a commit hook editing files is
// treated as content changing, which rolls the run back unless the caller
// accepted it.
func TestHookRewriteRejectedByDefault(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	h := newHarness(t)
	h.write("api/server.go", "package api\n")
	// A hook that reformats what it is given, the way a formatter would.
	h.installHook("pre-commit", "#!/bin/sh\nprintf '// formatted\\n' >> api/server.go\ngit add api/server.go\n")

	p, err := h.Plan(ctx, Options{})
	if err != nil {
		t.Fatalf("Plan: %v", err)
	}
	before := strings.TrimSpace(h.git("rev-parse", "HEAD"))
	_, err = h.Apply(ctx, p, Options{})
	if !errors.Is(err, ErrContentChanged) {
		t.Fatalf("Apply = %v, want the conservation check to catch the hook", err)
	}
	if after := strings.TrimSpace(h.git("rev-parse", "HEAD")); after != before {
		t.Error("HEAD moved despite the rollback")
	}
}

// TestHookRewriteAcceptedWhenAllowed checks that the same rewrite is accepted
// when the caller opted in, and is reported rather than passing silently.
func TestHookRewriteAcceptedWhenAllowed(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	h := newHarness(t)
	h.write("api/server.go", "package api\n")
	h.installHook("pre-commit", "#!/bin/sh\nprintf '// formatted\\n' >> api/server.go\ngit add api/server.go\n")

	p, err := h.Plan(ctx, Options{})
	if err != nil {
		t.Fatalf("Plan: %v", err)
	}
	result, err := h.Apply(ctx, p, Options{AllowHookRewrites: true})
	if err != nil {
		t.Fatalf("Apply with hook rewrites allowed: %v", err)
	}
	if len(result.Reformatted) == 0 {
		t.Error("the hook's edit was accepted without being reported")
	}
	for _, path := range result.Reformatted {
		if path != "api/server.go" {
			t.Errorf("reported %s as reformatted, want only the file the hook touched", path)
		}
	}
}

// TestNoVerifySkipsARejectingHook checks that standing consent lets a run
// finish in a repository whose hook refuses automated commits.
func TestNoVerifySkipsARejectingHook(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	h := newHarness(t)
	h.messyTree()
	h.installHook("pre-commit", "#!/bin/sh\necho blocked >&2\nexit 1\n")

	p, err := h.Plan(ctx, Options{})
	if err != nil {
		t.Fatalf("Plan: %v", err)
	}
	// Without consent the hook stops the run.
	if _, err := h.Apply(ctx, p, Options{}); err == nil {
		t.Fatal("a rejecting hook did not stop the run")
	}
	// With consent it proceeds.
	p, err = h.Plan(ctx, Options{})
	if err != nil {
		t.Fatalf("Plan: %v", err)
	}
	if _, err := h.Apply(ctx, p, Options{NoVerify: true}); err != nil {
		t.Fatalf("Apply with standing consent: %v", err)
	}
	if status := h.git("status", "--porcelain=v1"); strings.TrimSpace(status) != "" {
		t.Errorf("tree not clean:\n%s", status)
	}
}

// TestPushPublishesARewrite checks the force push itself, which is the one
// step that changes something other people can see.
func TestPushPublishesARewrite(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	h := newHarness(t)
	h.addRemote()
	h.git("checkout", "-q", "-b", "feature")
	base := strings.TrimSpace(h.git("rev-parse", "HEAD"))
	h.write("api/server.go", "package api\n")
	h.git("add", "-A")
	h.git("commit", "-m", "wip")
	h.git("push", "-q", "-u", "origin", "feature")

	opts := Options{Pushed: true, PushedBase: base}
	p, err := h.Plan(ctx, opts)
	if err != nil {
		t.Fatalf("Plan: %v", err)
	}
	if _, err := h.Apply(ctx, p, opts); err != nil {
		t.Fatalf("Apply: %v", err)
	}
	// The remote still holds the old commit until the push runs.
	if remote := h.git("log", "--format=%s", "origin/feature"); !strings.Contains(remote, "wip") {
		t.Errorf("the remote moved before the push:\n%s", remote)
	}
	if err := h.Push(ctx); err != nil {
		t.Fatalf("Push: %v", err)
	}
	h.git("fetch", "-q", "origin")
	if remote := h.git("log", "--format=%s", "origin/feature"); strings.Contains(remote, "wip") {
		t.Errorf("the push did not update the remote:\n%s", remote)
	}
}
