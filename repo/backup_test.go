package repo

import (
	"context"
	"strings"
	"testing"
	"time"
)

// TestRestoreConservesContent checks the property that undo must have: moving
// the branch back to a backup restores the pre-run state without deleting a
// single byte from the working tree.
//
// A reset that updates tracked files, --hard or --keep alike, removes from
// disk every file the undone commits added. That silently destroys work, which
// is the one failure preen must never have, so the invariant is asserted here.
func TestRestoreConservesContent(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	tr := newTestRepo(t)
	tr.write("kept.txt", "already committed\n")
	tr.commit("Initial commit")

	// The messy state a run starts from: new files and an edit, none committed.
	tr.write("kept.txt", "already committed\nplus an edit\n")
	tr.write("added.go", "package main\n")
	tr.write("docs/notes.md", "# notes\n")

	before, err := tr.ContentTree(ctx)
	if err != nil {
		t.Fatalf("ContentTree: %v", err)
	}
	backup, err := tr.CreateBackup(ctx, time.Now())
	if err != nil {
		t.Fatalf("CreateBackup: %v", err)
	}

	// A run commits the work in pieces.
	tr.git("add", "added.go")
	tr.git("commit", "-m", "Add main")
	tr.git("add", "docs/notes.md")
	tr.git("commit", "-m", "Add notes")
	tr.git("add", "kept.txt")
	tr.git("commit", "-m", "Edit kept")

	if err := tr.RestoreBackup(ctx, backup); err != nil {
		t.Fatalf("RestoreBackup: %v", err)
	}

	after, err := tr.ContentTree(ctx)
	if err != nil {
		t.Fatalf("ContentTree after restore: %v", err)
	}
	if before != after {
		paths, _ := tr.TreeDiffPaths(ctx, before, after)
		t.Errorf("restore changed content, diverged at %v", paths)
	}
	// The files must still be on disk, as uncommitted work.
	status := tr.git("status", "--porcelain=v1", "--untracked-files=all")
	for _, path := range []string{"added.go", "docs/notes.md", "kept.txt"} {
		if !strings.Contains(status, path) {
			t.Errorf("%s is not in the working tree after restore, status:\n%s", path, status)
		}
	}
	// HEAD must be back where the run started.
	if got := strings.TrimSpace(tr.git("log", "--oneline")); strings.Count(got, "\n") != 0 {
		t.Errorf("history not rewound to a single commit:\n%s", got)
	}
}

// TestRestoreRefusesForeignRefs checks that only preen's own recovery refs can
// ever be a restore target, so a mistaken argument cannot move the branch to
// something unrelated.
func TestRestoreRefusesForeignRefs(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	tr := newTestRepo(t)
	tr.write("a.txt", "one\n")
	tr.commit("Initial")

	for _, name := range []string{"main", "refs/heads/main", "some-branch"} {
		if err := tr.RestoreBackup(ctx, name); err == nil {
			t.Errorf("RestoreBackup(%q) was allowed, want a refusal", name)
		}
		if err := tr.DeleteBackup(ctx, name); err == nil {
			t.Errorf("DeleteBackup(%q) was allowed, want a refusal", name)
		}
	}
}

// TestListBackupsReportsContainment checks that a backup already contained in
// the branch is reported as safe to prune, since that is what decides which
// refs a prune may delete.
func TestListBackupsReportsContainment(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	tr := newTestRepo(t)
	tr.write("a.txt", "one\n")
	tr.commit("Initial")

	contained, err := tr.CreateBackup(ctx, time.Now())
	if err != nil {
		t.Fatalf("CreateBackup: %v", err)
	}
	// Move the branch forward so the backup is behind it.
	tr.write("b.txt", "two\n")
	tr.commit("Second")

	backups, err := tr.ListBackups(ctx)
	if err != nil {
		t.Fatalf("ListBackups: %v", err)
	}
	if len(backups) != 1 {
		t.Fatalf("got %d backups, want 1", len(backups))
	}
	if backups[0].Name != contained {
		t.Errorf("backup name = %q, want %q", backups[0].Name, contained)
	}
	if !backups[0].Merged {
		t.Error("a backup the branch already contains was not marked as prunable")
	}
}
