package repo

import (
	"context"
	"strings"
	"testing"
)

// baseFile is a file with widely separated regions, so edits at the top and
// bottom produce two distinct hunks rather than merging into one.
const baseFile = `package main

func alpha() int {
	return 1
}

// filler so the hunks stay apart
// filler
// filler
// filler
// filler
// filler
// filler
// filler

func omega() int {
	return 2
}
`

// TestSplitFileAcrossCommits checks the capability preen is built on: one
// file's hunks divided across two commits, with the working tree unchanged and
// every line accounted for.
func TestSplitFileAcrossCommits(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	tr := newTestRepo(t)
	tr.write("main.go", baseFile)
	tr.commit("Add main")

	edited := strings.Replace(baseFile, "return 1", "return 100", 1)
	edited = strings.Replace(edited, "return 2", "return 200", 1)
	tr.write("main.go", edited)

	before, err := tr.ContentTree(ctx)
	if err != nil {
		t.Fatalf("ContentTree: %v", err)
	}

	files, err := tr.Diff(ctx)
	if err != nil {
		t.Fatalf("Diff: %v", err)
	}
	if len(files) != 1 {
		t.Fatalf("Diff returned %d files, want 1", len(files))
	}
	if got := len(files[0].Hunks); got != 2 {
		t.Fatalf("got %d hunks, want 2 (diff:\n%s)", got, files[0].Text())
	}
	if !files[0].Splittable() {
		t.Error("Splittable() = false for a two-hunk text file")
	}

	// First commit takes only the alpha change.
	first := files[0].TextWith([]int{0})
	if err := tr.CheckPatch(ctx, first); err != nil {
		t.Fatalf("CheckPatch on first hunk: %v", err)
	}
	if err := tr.ApplyToIndex(ctx, first); err != nil {
		t.Fatalf("ApplyToIndex: %v", err)
	}
	if _, err := tr.Commit(ctx, CommitOptions{Message: "Raise alpha"}); err != nil {
		t.Fatalf("Commit: %v", err)
	}

	// The first commit must carry the alpha change and not the omega one.
	firstDiff := tr.git("show", "--no-color", "--format=", "HEAD")
	if !strings.Contains(firstDiff, "return 100") {
		t.Errorf("first commit missing the alpha change:\n%s", firstDiff)
	}
	if strings.Contains(firstDiff, "return 200") {
		t.Errorf("first commit leaked the omega change:\n%s", firstDiff)
	}

	// Second commit takes what is left.
	rest, err := tr.Diff(ctx)
	if err != nil {
		t.Fatalf("Diff after first commit: %v", err)
	}
	if len(rest) != 1 || len(rest[0].Hunks) != 1 {
		t.Fatalf("remaining diff should hold exactly one hunk, got %+v", rest)
	}
	if err := tr.ApplyToIndex(ctx, rest[0].Text()); err != nil {
		t.Fatalf("ApplyToIndex remainder: %v", err)
	}
	if _, err := tr.Commit(ctx, CommitOptions{Message: "Raise omega"}); err != nil {
		t.Fatalf("Commit remainder: %v", err)
	}

	after, err := tr.ContentTree(ctx)
	if err != nil {
		t.Fatalf("ContentTree: %v", err)
	}
	if before != after {
		paths, _ := tr.TreeDiffPaths(ctx, before, after)
		t.Errorf("content changed across the split: %v", paths)
	}
	if got := tr.git("status", "--porcelain=v1"); strings.TrimSpace(got) != "" {
		t.Errorf("working tree not clean after committing everything:\n%s", got)
	}
}

// TestCommitRefusesEmptyStage checks that a plan disagreeing with the tree
// fails loudly instead of creating an empty commit.
func TestCommitRefusesEmptyStage(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	tr := newTestRepo(t)
	tr.write("a.txt", "one\n")
	tr.commit("Add a")

	_, err := tr.Commit(ctx, CommitOptions{Message: "Nothing to say"})
	if err == nil {
		t.Fatal("Commit with an empty stage returned no error")
	}
	if !strings.Contains(err.Error(), "nothing staged") {
		t.Errorf("unexpected error: %v", err)
	}
}

// TestSplitUntrackedFile checks that a new file can be divided across commits,
// which requires recording it in the index first so its lines are visible to
// diff.
func TestSplitUntrackedFile(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	tr := newTestRepo(t)
	tr.write("seed.txt", "seed\n")
	tr.commit("Seed")

	tr.write("fresh.go", baseFile)
	if err := tr.IntentToAdd(ctx, "fresh.go"); err != nil {
		t.Fatalf("IntentToAdd: %v", err)
	}
	files, err := tr.Diff(ctx)
	if err != nil {
		t.Fatalf("Diff: %v", err)
	}
	if len(files) != 1 {
		t.Fatalf("got %d files, want 1", len(files))
	}
	if len(files[0].Hunks) == 0 {
		t.Fatal("intent-to-add file produced no hunks")
	}
}

// TestStatusParsesRenames checks that a rename is reported as one change
// carrying both paths, so the pair is never split across commits.
func TestStatusParsesRenames(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	tr := newTestRepo(t)
	tr.write("old.txt", "content that stays identical\n")
	tr.commit("Add old")
	tr.git("mv", "old.txt", "new.txt")

	changes, err := tr.Status(ctx)
	if err != nil {
		t.Fatalf("Status: %v", err)
	}
	var found bool
	for _, c := range changes {
		if c.Kind == KindRenamed {
			found = true
			if c.Path != "new.txt" || c.From != "old.txt" {
				t.Errorf("rename recorded as %q from %q, want new.txt from old.txt", c.Path, c.From)
			}
			if !c.IsRenamePair() {
				t.Error("IsRenamePair() = false for a rename")
			}
		}
	}
	if !found {
		t.Errorf("no rename in status: %+v", changes)
	}
}
