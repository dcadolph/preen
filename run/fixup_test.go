package run

import (
	"context"
	"errors"
	"strings"
	"testing"
)

// TestFixupFoldsIntoTheRightCommit checks the mode that fixes work already
// committed: each dirty change lands in the unpushed commit that introduced
// its file, and the history keeps its original shape.
func TestFixupFoldsIntoTheRightCommit(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	h := newHarness(t)
	h.addRemote()

	// Two unpushed commits, each owning a different file.
	h.write("api/server.go", "package api\n\nfunc Serve() {}\n")
	h.git("add", "-A")
	h.git("commit", "-m", "Add the api server")
	h.write("store/db.go", "package store\n\nfunc Open() {}\n")
	h.git("add", "-A")
	h.git("commit", "-m", "Add the store")

	// Dirty follow-up edits to both files.
	h.write("api/server.go", "package api\n\nfunc Serve() error { return nil }\n")
	h.write("store/db.go", "package store\n\nfunc Open() error { return nil }\n")

	built, err := h.PlanFixup(ctx, Options{})
	if err != nil {
		t.Fatalf("PlanFixup: %v", err)
	}
	if len(built.Fixups) != 2 {
		t.Fatalf("routed %d changes, want 2: %+v", len(built.Fixups), built.Fixups)
	}
	for _, fixup := range built.Fixups {
		switch fixup.Part.Path {
		case "api/server.go":
			if fixup.Target.Subject != "Add the api server" {
				t.Errorf("api change routed to %q, want the api commit", fixup.Target.Subject)
			}
		case "store/db.go":
			if fixup.Target.Subject != "Add the store" {
				t.Errorf("store change routed to %q, want the store commit", fixup.Target.Subject)
			}
		default:
			t.Errorf("unexpected routed path %s", fixup.Part.Path)
		}
	}

	before := len(strings.Split(strings.TrimSpace(h.git("log", "--format=%H")), "\n"))
	result, err := h.ApplyFixup(ctx, built, Options{})
	if err != nil {
		t.Fatalf("ApplyFixup: %v", err)
	}
	if result.TreeStart != result.TreeEnd {
		t.Errorf("content moved during a fixup: %s to %s", result.TreeStart, result.TreeEnd)
	}
	// The squash must leave the history the same length, not add commits.
	after := len(strings.Split(strings.TrimSpace(h.git("log", "--format=%H")), "\n"))
	if after != before {
		t.Errorf("history went from %d commits to %d, want the fixups squashed away", before, after)
	}
	if log := h.git("log", "--format=%s"); strings.Contains(log, "fixup!") {
		t.Errorf("a fixup marker survived the autosquash:\n%s", log)
	}
	// The edits must now live inside the original commits.
	apiCommit := h.git("show", "--format=", "--name-only", ":/Add the api server")
	if !strings.Contains(apiCommit, "api/server.go") {
		t.Errorf("the api commit does not carry its file:\n%s", apiCommit)
	}
	if status := h.git("status", "--porcelain=v1"); strings.TrimSpace(status) != "" {
		t.Errorf("tree not clean after the fixup:\n%s", status)
	}
}

// TestFixupLeavesUnroutableChanges checks that a change no unpushed commit
// introduced is reported as a leftover rather than forced into the wrong
// commit.
func TestFixupLeavesUnroutableChanges(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	h := newHarness(t)
	h.addRemote()

	h.write("api/server.go", "package api\n")
	h.git("add", "-A")
	h.git("commit", "-m", "Add the api server")

	// One edit that routes, and one brand new file that cannot.
	h.write("api/server.go", "package api\n\nfunc Serve() {}\n")
	h.write("brand/new.go", "package brand\n")

	built, err := h.PlanFixup(ctx, Options{})
	if err != nil {
		t.Fatalf("PlanFixup: %v", err)
	}
	if len(built.Fixups) != 1 {
		t.Errorf("routed %d changes, want only the one with a home", len(built.Fixups))
	}
	if len(built.Plan.Leftover) != 1 || built.Plan.Leftover[0].Path != "brand/new.go" {
		t.Errorf("leftovers = %+v, want the unroutable new file", built.Plan.Leftover)
	}
}

// TestFixupRefusesPushedTargets checks that a fixup never rewrites a commit a
// remote already has.
func TestFixupRefusesPushedTargets(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	h := newHarness(t)
	h.addRemote()

	h.write("api/server.go", "package api\n")
	h.git("add", "-A")
	h.git("commit", "-m", "Published api")
	h.git("push", "origin", "main")
	h.write("api/server.go", "package api\n\nfunc Serve() {}\n")

	_, err := h.PlanFixup(ctx, Options{})
	if err == nil {
		t.Fatal("PlanFixup targeted a pushed commit without complaint")
	}
	if !errors.Is(err, ErrNothingToDo) && !errors.Is(err, ErrFixupTarget) && !errors.Is(err, ErrPushedRewrite) {
		t.Errorf("error = %v, want a refusal to touch published work", err)
	}
}
