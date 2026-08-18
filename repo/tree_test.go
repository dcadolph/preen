package repo

import (
	"context"
	"testing"
)

// TestContentTreeConserved checks the invariant preen depends on: the content
// tree covers HEAD plus every staged, unstaged, and untracked change, so
// rearranging which commit holds what never changes the hash.
func TestContentTreeConserved(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	tr := newTestRepo(t)
	tr.write("a.txt", "one\n")
	tr.commit("Add a")

	tr.write("a.txt", "one\ntwo\n")
	tr.write("b.txt", "new file\n")
	before, err := tr.ContentTree(ctx)
	if err != nil {
		t.Fatalf("ContentTree: %v", err)
	}

	// Committing the same content in a different shape must not move the hash.
	tr.git("add", "b.txt")
	tr.git("commit", "-m", "Add b")
	after, err := tr.ContentTree(ctx)
	if err != nil {
		t.Fatalf("ContentTree: %v", err)
	}
	if before != after {
		t.Errorf("content tree changed after committing existing work:\n before %s\n after  %s", before, after)
	}
}

// TestContentTreeDetectsLoss checks that losing or inventing content moves the
// hash, which is what makes the conservation check a real guard rather than a
// formality.
func TestContentTreeDetectsLoss(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	tests := []struct {
		Name   string
		Mutate func(tr *testRepo)
	}{
		{
			Name:   "content dropped",
			Mutate: func(tr *testRepo) { tr.remove("b.txt") },
		},
		{
			Name:   "content invented",
			Mutate: func(tr *testRepo) { tr.write("c.txt", "surprise\n") },
		},
		{
			Name:   "content edited",
			Mutate: func(tr *testRepo) { tr.write("a.txt", "one\nthree\n") },
		},
	}

	for testNum, test := range tests {
		t.Run(test.Name, func(t *testing.T) {
			t.Parallel()
			tr := newTestRepo(t)
			tr.write("a.txt", "one\n")
			tr.commit("Add a")
			tr.write("a.txt", "one\ntwo\n")
			tr.write("b.txt", "new file\n")

			before, err := tr.ContentTree(ctx)
			if err != nil {
				t.Fatalf("test %d ContentTree: %v", testNum, err)
			}
			test.Mutate(tr)
			after, err := tr.ContentTree(ctx)
			if err != nil {
				t.Fatalf("test %d ContentTree: %v", testNum, err)
			}
			if before == after {
				t.Errorf("test %d: hash unchanged after %s, want a difference", testNum, test.Name)
			}
			paths, err := tr.TreeDiffPaths(ctx, before, after)
			if err != nil {
				t.Fatalf("test %d TreeDiffPaths: %v", testNum, err)
			}
			if len(paths) == 0 {
				t.Errorf("test %d: TreeDiffPaths reported no paths for %s", testNum, test.Name)
			}
		})
	}
}

// TestContentTreeIgnoresUntrackedIgnores checks that ignored files stay out of
// the baseline, since they are never committed and would otherwise make the
// check fail on unrelated build output.
func TestContentTreeIgnoresUntrackedIgnores(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	tr := newTestRepo(t)
	tr.write(".gitignore", "build/\n")
	tr.commit("Add ignore rules")

	before, err := tr.ContentTree(ctx)
	if err != nil {
		t.Fatalf("ContentTree: %v", err)
	}
	tr.write("build/artifact.bin", "junk\n")
	after, err := tr.ContentTree(ctx)
	if err != nil {
		t.Fatalf("ContentTree: %v", err)
	}
	if before != after {
		t.Errorf("ignored file changed the content tree:\n before %s\n after  %s", before, after)
	}
}

// TestContentTreeUnbornHead checks that the baseline works before the first
// commit, where there is no HEAD tree to seed the scratch index with.
func TestContentTreeUnbornHead(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	tr := newTestRepo(t)
	tr.write("first.txt", "hello\n")

	hash, err := tr.ContentTree(ctx)
	if err != nil {
		t.Fatalf("ContentTree on unborn head: %v", err)
	}
	if hash == "" {
		t.Error("ContentTree returned an empty hash on unborn head")
	}
}
