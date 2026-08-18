package run

import (
	"context"
	"strings"
	"testing"

	"github.com/dcadolph/preen/group"
	"github.com/dcadolph/preen/plan"
	"github.com/dcadolph/preen/repo"
)

// spreadFile has widely separated regions, so edits at the top and bottom
// produce two distinct hunks rather than merging into one.
const spreadFile = `package api

func alpha() int {
	return 1
}

// filler
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

// splitGrouper returns a grouper that puts each of a file's hunks in its own
// commit, in the given order of hunk indexes.
func splitGrouper(path string, order []int) group.Grouper {
	return group.GrouperFunc(func(_ context.Context, in group.Input) ([]plan.Commit, error) {
		diff, ok := in.DiffFor(path)
		if !ok {
			return nil, nil
		}
		commits := make([]plan.Commit, 0, len(order))
		for _, index := range order {
			body := strings.Join(diff.Hunks[index].Lines, "\n")
			commits = append(commits, plan.Commit{
				Subject: "Take hunk " + string(rune('0'+index)),
				Parts: []plan.Part{{
					Path:  path,
					Kind:  repo.KindModified,
					Hunks: []plan.Hunk{plan.HunkAt(index, body)},
				}},
			})
		}
		return commits, nil
	})
}

// TestSplitsFileAcrossCommits checks the engine can divide one file's hunks
// into separate commits, with each commit carrying only its own change.
func TestSplitsFileAcrossCommits(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	h := newHarness(t)
	h.write("api.go", spreadFile)
	h.git("add", "-A")
	h.git("commit", "-m", "Add api")

	edited := strings.Replace(spreadFile, "return 1", "return 100", 1)
	edited = strings.Replace(edited, "return 2", "return 200", 1)
	h.write("api.go", edited)
	h.Grouper = splitGrouper("api.go", []int{0, 1})

	p, err := h.Plan(ctx, Options{})
	if err != nil {
		t.Fatalf("Plan: %v", err)
	}
	if len(p.Commits) != 2 {
		t.Fatalf("got %d commits, want the file split in two", len(p.Commits))
	}
	result, err := h.Apply(ctx, p, Options{})
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if result.TreeStart != result.TreeEnd {
		t.Errorf("content moved during a split: %s to %s", result.TreeStart, result.TreeEnd)
	}

	first := h.git("show", "--format=", "HEAD~1")
	second := h.git("show", "--format=", "HEAD")
	if !strings.Contains(first, "return 100") || strings.Contains(first, "return 200") {
		t.Errorf("the first commit does not hold exactly the alpha change:\n%s", first)
	}
	if !strings.Contains(second, "return 200") || strings.Contains(second, "return 100") {
		t.Errorf("the second commit does not hold exactly the omega change:\n%s", second)
	}
	if status := h.git("status", "--porcelain=v1"); strings.TrimSpace(status) != "" {
		t.Errorf("tree not clean after the split:\n%s", status)
	}
}

// TestSplitSurvivesHunkShift checks the reason a planned hunk is identified by
// its body rather than its position.
//
// Committing the later hunk first renumbers what remains, so a plan that
// referred to positions would then stage the wrong lines. Taking hunk 1 before
// hunk 0 is exactly that case.
func TestSplitSurvivesHunkShift(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	h := newHarness(t)
	h.write("api.go", spreadFile)
	h.git("add", "-A")
	h.git("commit", "-m", "Add api")

	edited := strings.Replace(spreadFile, "return 1", "return 100", 1)
	edited = strings.Replace(edited, "return 2", "return 200", 1)
	h.write("api.go", edited)
	// The second hunk is committed first, which shifts the first one.
	h.Grouper = splitGrouper("api.go", []int{1, 0})

	p, err := h.Plan(ctx, Options{})
	if err != nil {
		t.Fatalf("Plan: %v", err)
	}
	result, err := h.Apply(ctx, p, Options{})
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if result.TreeStart != result.TreeEnd {
		t.Errorf("content moved when hunks were taken out of order: %s to %s", result.TreeStart, result.TreeEnd)
	}
	// The commit recorded first must hold the omega change, not the alpha one.
	first := h.git("show", "--format=", "HEAD~1")
	if !strings.Contains(first, "return 200") || strings.Contains(first, "return 100") {
		t.Errorf("out-of-order split staged the wrong lines:\n%s", first)
	}
	if status := h.git("status", "--porcelain=v1"); strings.TrimSpace(status) != "" {
		t.Errorf("tree not clean:\n%s", status)
	}
}

// TestSplitRejectsAVanishedHunk checks that a plan naming a hunk the tree no
// longer has fails loudly and rolls back, rather than staging something else.
func TestSplitRejectsAVanishedHunk(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	h := newHarness(t)
	h.write("api.go", spreadFile)
	h.git("add", "-A")
	h.git("commit", "-m", "Add api")
	h.write("api.go", strings.Replace(spreadFile, "return 1", "return 100", 1))

	p := &plan.Plan{
		Commits: []plan.Commit{{
			Subject: "Take a hunk that is not there",
			Parts: []plan.Part{{
				Path:  "api.go",
				Kind:  repo.KindModified,
				Hunks: []plan.Hunk{plan.HunkAt(0, "this body does not appear in any diff")},
			}},
		}},
		Covers: []repo.Change{{Path: "api.go", Kind: repo.KindModified}},
	}
	before := strings.TrimSpace(h.git("rev-parse", "HEAD"))

	if _, err := h.Apply(ctx, p, Options{}); err == nil {
		t.Fatal("Apply accepted a hunk that is not in the diff")
	}
	if after := strings.TrimSpace(h.git("rev-parse", "HEAD")); after != before {
		t.Errorf("HEAD moved despite the failure: %s to %s", before, after)
	}
	if status := h.git("status", "--porcelain=v1"); strings.TrimSpace(status) == "" {
		t.Error("the edit vanished after the rollback")
	}
}

// TestSplitUntrackedFile checks that a brand new file can be divided too,
// which needs it recorded in the index before its lines are visible to diff.
func TestSplitUntrackedFile(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	h := newHarness(t)
	h.write("fresh.go", spreadFile)

	h.Grouper = group.GrouperFunc(func(_ context.Context, in group.Input) ([]plan.Commit, error) {
		// An untracked file has no diff until it is recorded, so the grouper
		// takes it whole and the engine handles the rest.
		return []plan.Commit{{
			Subject: "Add fresh",
			Parts:   []plan.Part{{Path: "fresh.go", Kind: repo.KindUntracked}},
		}}, nil
	})
	p, err := h.Plan(ctx, Options{})
	if err != nil {
		t.Fatalf("Plan: %v", err)
	}
	result, err := h.Apply(ctx, p, Options{})
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if result.TreeStart != result.TreeEnd {
		t.Errorf("content moved: %s to %s", result.TreeStart, result.TreeEnd)
	}
	if status := h.git("status", "--porcelain=v1"); strings.TrimSpace(status) != "" {
		t.Errorf("tree not clean:\n%s", status)
	}
}
