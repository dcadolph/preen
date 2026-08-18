package repo

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// addRemote creates a bare repository next to the working tree, wires it as
// origin, and pushes the current branch so the repository has real remote
// tracking refs to reason about.
func (tr *testRepo) addRemote() string {
	tr.T.Helper()
	bare := filepath.Join(tr.T.TempDir(), "origin.git")
	cmd := exec.Command("git", "init", "--bare", "-b", "main", bare)
	cmd.Env = append(os.Environ(), "GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_SYSTEM=/dev/null")
	if out, err := cmd.CombinedOutput(); err != nil {
		tr.T.Fatalf("init bare: %v: %s", err, out)
	}
	tr.git("remote", "add", "origin", bare)
	tr.git("push", "-u", "origin", "main")
	return bare
}

// TestCheckMergesNoMerges checks the clean case, where a linear range reports
// no merges and leaves the base alone.
func TestCheckMergesNoMerges(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	tr := newTestRepo(t)
	tr.write("a.txt", "one\n")
	tr.commit("First")
	base, err := tr.Head(ctx)
	if err != nil {
		t.Fatalf("Head: %v", err)
	}
	tr.write("b.txt", "two\n")
	tr.commit("Second")

	check, err := tr.CheckMerges(ctx, base)
	if err != nil {
		t.Fatalf("CheckMerges: %v", err)
	}
	if len(check.Merges) != 0 || check.Moved || check.Flattens {
		t.Errorf("linear range reported merges: %+v", check)
	}
	if check.SafeBase != base {
		t.Errorf("SafeBase = %s, want the requested base %s", check.SafeBase, base)
	}
}

// TestCheckMergesPublishedSideBranch checks the rule that protects other
// people's history: a merge whose second parent is on a remote must never be
// absorbed, so the base moves forward past it.
func TestCheckMergesPublishedSideBranch(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	tr := newTestRepo(t)
	tr.write("a.txt", "one\n")
	tr.commit("First")
	tr.addRemote()

	base, err := tr.Head(ctx)
	if err != nil {
		t.Fatalf("Head: %v", err)
	}

	// A feature branch that gets pushed, so its commits are published.
	tr.git("checkout", "-b", "feature")
	tr.write("feature.txt", "feature work\n")
	tr.commit("Add feature")
	tr.git("push", "-u", "origin", "feature")

	// Merge it back into main, creating a merge whose second parent is pushed.
	tr.git("checkout", "main")
	tr.git("merge", "--no-ff", "-m", "Merge feature", "feature")
	mergeHash, err := tr.Head(ctx)
	if err != nil {
		t.Fatalf("Head: %v", err)
	}
	tr.write("local.txt", "local work\n")
	tr.commit("Local work after the merge")

	check, err := tr.CheckMerges(ctx, base)
	if err != nil {
		t.Fatalf("CheckMerges: %v", err)
	}
	if len(check.Merges) != 1 {
		t.Fatalf("got %d merges, want 1: %+v", len(check.Merges), check.Merges)
	}
	if !check.Merges[0].Published() {
		t.Errorf("merge with a pushed side branch reported unpublished: %+v", check.Merges[0])
	}
	if !check.Moved {
		t.Error("base did not move past a published merge")
	}
	if check.SafeBase != mergeHash {
		t.Errorf("SafeBase = %s, want the merge commit %s", check.SafeBase, mergeHash)
	}
	if check.Flattens {
		t.Error("Flattens = true, want false when the base moved instead")
	}
}

// TestCheckMergesUnpushedMerge checks that a merge nobody has published is
// reported as safe to linearize rather than blocking the run.
func TestCheckMergesUnpushedMerge(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	tr := newTestRepo(t)
	tr.write("a.txt", "one\n")
	tr.commit("First")
	tr.addRemote()
	base, err := tr.Head(ctx)
	if err != nil {
		t.Fatalf("Head: %v", err)
	}

	// A local-only side branch, never pushed.
	tr.git("checkout", "-b", "scratch")
	tr.write("scratch.txt", "scratch work\n")
	tr.commit("Scratch work")
	tr.git("checkout", "main")
	tr.git("merge", "--no-ff", "-m", "Merge scratch", "scratch")

	check, err := tr.CheckMerges(ctx, base)
	if err != nil {
		t.Fatalf("CheckMerges: %v", err)
	}
	if len(check.Merges) != 1 {
		t.Fatalf("got %d merges, want 1", len(check.Merges))
	}
	if check.Merges[0].Published() {
		t.Errorf("unpushed merge reported as published: %+v", check.Merges[0])
	}
	if check.Moved {
		t.Error("base moved for an unpushed merge, want it left alone")
	}
	if !check.Flattens {
		t.Error("Flattens = false, want true for an unpushed merge in range")
	}
}

// TestIsPushed checks published detection for individual commits, which
// decides which commits may be absorbed.
func TestIsPushed(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	tr := newTestRepo(t)
	tr.write("a.txt", "one\n")
	tr.commit("Published")
	tr.addRemote()
	published, err := tr.Head(ctx)
	if err != nil {
		t.Fatalf("Head: %v", err)
	}
	tr.write("b.txt", "two\n")
	tr.commit("Local only")
	local, err := tr.Head(ctx)
	if err != nil {
		t.Fatalf("Head: %v", err)
	}

	tests := []struct {
		Name string
		Rev  string
		Want bool
	}{
		{Name: "pushed commit", Rev: published, Want: true},
		{Name: "local commit", Rev: local, Want: false},
	}
	for testNum, test := range tests {
		t.Run(test.Name, func(t *testing.T) {
			t.Parallel()
			got, err := tr.IsPushed(ctx, test.Rev)
			if err != nil {
				t.Fatalf("test %d IsPushed: %v", testNum, err)
			}
			if got != test.Want {
				t.Errorf("test %d IsPushed(%s) = %v, want %v", testNum, test.Rev[:8], got, test.Want)
			}
		})
	}
}

// TestUnpushedBase checks that the absorb base is the upstream tip when the
// branch tracks a remote.
func TestUnpushedBase(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	tr := newTestRepo(t)
	tr.write("a.txt", "one\n")
	tr.commit("Published")
	tr.addRemote()
	want, err := tr.Head(ctx)
	if err != nil {
		t.Fatalf("Head: %v", err)
	}
	tr.write("b.txt", "two\n")
	tr.commit("Unpushed one")
	tr.write("c.txt", "three\n")
	tr.commit("Unpushed two")

	got, err := tr.UnpushedBase(ctx)
	if err != nil {
		t.Fatalf("UnpushedBase: %v", err)
	}
	if got != want {
		t.Errorf("UnpushedBase = %s, want the upstream tip %s", got, want)
	}
	commits, err := tr.Log(ctx, got+"..HEAD")
	if err != nil {
		t.Fatalf("Log: %v", err)
	}
	if len(commits) != 2 {
		t.Errorf("got %d unpushed commits, want 2", len(commits))
	}
}
