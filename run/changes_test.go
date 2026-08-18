package run

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestHandlesDeletions checks that a removed file is committed as a deletion
// rather than being skipped, which would leave the tree dirty forever.
func TestHandlesDeletions(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	h := newHarness(t)
	h.write("api/old.go", "package api\n")
	h.write("api/keep.go", "package api\n")
	h.git("add", "-A")
	h.git("commit", "-m", "Add api files")

	if err := os.Remove(filepath.Join(h.Dir, "api/old.go")); err != nil {
		t.Fatalf("remove: %v", err)
	}

	p, err := h.Plan(ctx, Options{})
	if err != nil {
		t.Fatalf("Plan: %v", err)
	}
	if got := p.Commits[0].Subject; !strings.HasPrefix(got, "Remove") {
		t.Errorf("subject = %q, want it to read as a removal", got)
	}
	result, err := h.Apply(ctx, p, Options{})
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if result.TreeStart != result.TreeEnd {
		t.Errorf("content moved while committing a deletion: %s to %s", result.TreeStart, result.TreeEnd)
	}
	if status := h.git("status", "--porcelain=v1"); strings.TrimSpace(status) != "" {
		t.Errorf("deletion left the tree dirty:\n%s", status)
	}
	if files := h.git("show", "--name-status", "--format=", "HEAD"); !strings.Contains(files, "D\tapi/old.go") {
		t.Errorf("the commit does not record the deletion:\n%s", files)
	}
}

// TestHandlesRenames checks that a moved file's two halves land in the same
// commit, since splitting them would produce a commit that deletes a file and
// another that adds it back.
func TestHandlesRenames(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	h := newHarness(t)
	h.write("api/old_name.go", "package api\n\nfunc Serve() {}\n")
	h.git("add", "-A")
	h.git("commit", "-m", "Add the api")

	h.git("mv", "api/old_name.go", "api/new_name.go")

	p, err := h.Plan(ctx, Options{})
	if err != nil {
		t.Fatalf("Plan: %v", err)
	}
	// Both halves of the move must be claimed by one commit.
	for _, commit := range p.Commits {
		var sawOld, sawNew bool
		for _, part := range commit.Parts {
			sawNew = sawNew || part.Path == "api/new_name.go"
			sawOld = sawOld || part.Path == "api/old_name.go" || part.From == "api/old_name.go"
		}
		if sawNew && !sawOld {
			t.Errorf("commit %q takes the new path without the old one: %v", commit.Subject, commit.Paths())
		}
	}
	result, err := h.Apply(ctx, p, Options{})
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if result.TreeStart != result.TreeEnd {
		t.Errorf("content moved while committing a rename: %s to %s", result.TreeStart, result.TreeEnd)
	}
	if status := h.git("status", "--porcelain=v1"); strings.TrimSpace(status) != "" {
		t.Errorf("rename left the tree dirty:\n%s", status)
	}
}

// TestHandlesBinaryFiles checks that a file git cannot diff as text is
// committed whole, since a binary can never be split by hunk.
func TestHandlesBinaryFiles(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	h := newHarness(t)

	// A byte sequence with a NUL, which git treats as binary.
	blob := []byte{0x00, 0x01, 0x02, 0xff, 0xfe, 0x00, 0x42}
	if err := os.WriteFile(filepath.Join(h.Dir, "logo.png"), blob, 0o600); err != nil {
		t.Fatalf("write binary: %v", err)
	}
	h.write("api/server.go", "package api\n")

	p, err := h.Plan(ctx, Options{})
	if err != nil {
		t.Fatalf("Plan: %v", err)
	}
	for _, commit := range p.Commits {
		for _, part := range commit.Parts {
			if part.Path == "logo.png" && !part.Whole() {
				t.Error("a binary file was planned as a hunk subset")
			}
		}
	}
	result, err := h.Apply(ctx, p, Options{})
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if result.TreeStart != result.TreeEnd {
		t.Errorf("content moved while committing a binary: %s to %s", result.TreeStart, result.TreeEnd)
	}
	if status := h.git("status", "--porcelain=v1"); strings.TrimSpace(status) != "" {
		t.Errorf("binary file left the tree dirty:\n%s", status)
	}
	// The bytes must survive exactly.
	got, err := os.ReadFile(filepath.Join(h.Dir, "logo.png"))
	if err != nil {
		t.Fatalf("read back: %v", err)
	}
	if string(got) != string(blob) {
		t.Errorf("binary content changed: %v, want %v", got, blob)
	}
}

// TestHandlesPathsWithSpaces checks that a path git would quote survives the
// survey, the plan, and staging intact.
func TestHandlesPathsWithSpaces(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	h := newHarness(t)
	h.write("docs/my notes.md", "# notes\n")
	h.write("docs/other file.md", "# other\n")

	p, err := h.Plan(ctx, Options{})
	if err != nil {
		t.Fatalf("Plan: %v", err)
	}
	var found bool
	for _, commit := range p.Commits {
		for _, part := range commit.Parts {
			if part.Path == "docs/my notes.md" {
				found = true
			}
		}
	}
	if !found {
		t.Errorf("a path with a space did not survive the survey: %+v", p.Commits)
	}
	result, err := h.Apply(ctx, p, Options{})
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if result.TreeStart != result.TreeEnd {
		t.Errorf("content moved on a quoted path: %s to %s", result.TreeStart, result.TreeEnd)
	}
	if status := h.git("status", "--porcelain=v1"); strings.TrimSpace(status) != "" {
		t.Errorf("quoted path left the tree dirty:\n%s", status)
	}
}

// TestPreVerifiedStagedWorkIsSeparated checks that work the user staged by
// hand becomes its own commit, since that boundary was drawn deliberately.
func TestPreVerifiedStagedWorkIsSeparated(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	h := newHarness(t)
	h.write("api/a.go", "package api\n")
	h.write("api/b.go", "package api\n")
	h.git("add", "api/a.go")

	p, err := h.Plan(ctx, Options{})
	if err != nil {
		t.Fatalf("Plan: %v", err)
	}
	if len(p.Commits) < 2 {
		t.Fatalf("staged and unstaged work landed together: %+v", p.Commits)
	}
	if _, err := h.Apply(ctx, p, Options{}); err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if status := h.git("status", "--porcelain=v1"); strings.TrimSpace(status) != "" {
		t.Errorf("tree not clean:\n%s", status)
	}
}
