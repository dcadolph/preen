package plan

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/dcadolph/preen/repo"
)

// changes builds a change set from paths, the shape Validate checks against.
func changes(paths ...string) []repo.Change {
	out := make([]repo.Change, 0, len(paths))
	for _, p := range paths {
		out = append(out, repo.Change{Path: p, Kind: repo.KindModified})
	}
	return out
}

// whole builds a commit taking whole files.
func whole(subject string, paths ...string) Commit {
	parts := make([]Part, 0, len(paths))
	for _, p := range paths {
		parts = append(parts, Part{Path: p, Kind: repo.KindModified})
	}
	return Commit{Subject: subject, Parts: parts}
}

// TestValidate checks the rule that keeps a grouping mistake from losing work:
// the plan must account for every change exactly once.
func TestValidate(t *testing.T) {
	t.Parallel()

	tests := []struct {
		Name    string
		Plan    Plan
		Changes []repo.Change
		Want    error
	}{
		{ // Test 0: Every change covered once.
			Name:    "complete plan",
			Plan:    Plan{Commits: []Commit{whole("Add api", "api.go"), whole("Add docs", "docs.md")}},
			Changes: changes("api.go", "docs.md"),
			Want:    nil,
		},
		{ // Test 1: A change no commit claims would be silently abandoned.
			Name:    "unaccounted change",
			Plan:    Plan{Commits: []Commit{whole("Add api", "api.go")}},
			Changes: changes("api.go", "forgotten.go"),
			Want:    ErrInvalid,
		},
		{ // Test 2: The same file in two commits cannot both be staged whole.
			Name:    "file claimed twice",
			Plan:    Plan{Commits: []Commit{whole("First", "api.go"), whole("Second", "api.go")}},
			Changes: changes("api.go"),
			Want:    ErrInvalid,
		},
		{ // Test 3: An empty commit has nothing to record.
			Name:    "empty commit",
			Plan:    Plan{Commits: []Commit{{Subject: "Nothing"}}},
			Changes: nil,
			Want:    ErrInvalid,
		},
		{ // Test 4: A commit with no subject has no message.
			Name:    "missing subject",
			Plan:    Plan{Commits: []Commit{whole("", "api.go")}},
			Changes: changes("api.go"),
			Want:    ErrInvalid,
		},
		{ // Test 5: A dropped change is accounted for as a leftover.
			Name: "leftover covers the change",
			Plan: Plan{
				Commits:  []Commit{whole("Add api", "api.go")},
				Leftover: []Part{{Path: "scratch.txt"}},
			},
			Changes: changes("api.go", "scratch.txt"),
			Want:    nil,
		},
		{ // Test 6: Distinct hunks of one file may go to different commits.
			Name: "file split by hunk",
			Plan: Plan{Commits: []Commit{
				{Subject: "First half", Parts: []Part{{Path: "api.go", Hunks: []Hunk{{Index: 0, Body: "a"}}}}},
				{Subject: "Second half", Parts: []Part{{Path: "api.go", Hunks: []Hunk{{Index: 1, Body: "b"}}}}},
			}},
			Changes: changes("api.go"),
			Want:    nil,
		},
		{ // Test 7: The same hunk cannot land in two commits.
			Name: "hunk claimed twice",
			Plan: Plan{Commits: []Commit{
				{Subject: "First", Parts: []Part{{Path: "api.go", Hunks: []Hunk{{Index: 0, Body: "a"}}}}},
				{Subject: "Second", Parts: []Part{{Path: "api.go", Hunks: []Hunk{{Index: 0, Body: "a"}}}}},
			}},
			Changes: changes("api.go"),
			Want:    ErrInvalid,
		},
		{ // Test 8: A file cannot be taken whole and by hunk at once.
			Name: "whole and partial",
			Plan: Plan{Commits: []Commit{
				whole("Whole", "api.go"),
				{Subject: "Partial", Parts: []Part{{Path: "api.go", Hunks: []Hunk{{Index: 0, Body: "a"}}}}},
			}},
			Changes: changes("api.go"),
			Want:    ErrInvalid,
		},
		{ // Test 9: A plan with no commits does nothing.
			Name:    "no commits",
			Plan:    Plan{},
			Changes: changes("api.go"),
			Want:    ErrInvalid,
		},
	}

	for testNum, test := range tests {
		t.Run(fmt.Sprintf("test %d %s", testNum, test.Name), func(t *testing.T) {
			t.Parallel()
			err := test.Plan.Validate(test.Changes)
			if !errors.Is(err, test.Want) {
				t.Errorf("Validate() = %v, want %v", err, test.Want)
			}
		})
	}
}

// TestEdits checks the plan editing moves a user applies at the approval
// prompt, including that an edit leaves the plan still covering the tree.
func TestEdits(t *testing.T) {
	t.Parallel()

	base := func() Plan {
		return Plan{Commits: []Commit{
			whole("Add api", "api.go", "api_test.go"),
			whole("Add store", "store.go"),
			whole("Add docs", "docs.md"),
		}}
	}
	all := changes("api.go", "api_test.go", "store.go", "docs.md")

	tests := []struct {
		Name      string
		Apply     func(p *Plan) error
		WantCount int
		WantFirst string
		Want      error
	}{
		{ // Test 0: Merging folds one commit into another.
			Name:      "merge",
			Apply:     func(p *Plan) error { return p.MergeInto(2, 1) },
			WantCount: 2,
			WantFirst: "Add api",
		},
		{ // Test 1: Rewording replaces a subject.
			Name:      "reword",
			Apply:     func(p *Plan) error { return p.Reword(1, "Add the API server") },
			WantCount: 3,
			WantFirst: "Add the API server",
		},
		{ // Test 2: Reordering resequences without losing a commit.
			Name:      "reorder",
			Apply:     func(p *Plan) error { return p.Reorder([]int{3, 1, 2}) },
			WantCount: 3,
			WantFirst: "Add docs",
		},
		{ // Test 3: Splitting by file makes one commit per file it held.
			Name:      "split by file",
			Apply:     func(p *Plan) error { return p.SplitByFile(1) },
			WantCount: 4,
			WantFirst: "Add api (api.go)",
		},
		{ // Test 4: A single-file commit has nothing to split.
			Name:  "split a single file commit",
			Apply: func(p *Plan) error { return p.SplitByFile(2) },
			Want:  ErrInvalid,
		},
		{ // Test 5: Moving a path empties its commit, which is then dropped.
			Name:      "move path",
			Apply:     func(p *Plan) error { return p.MovePath("store.go", 1) },
			WantCount: 2,
			WantFirst: "Add api",
		},
		{ // Test 6: Dropping records the path as deliberately uncommitted.
			Name:      "drop path",
			Apply:     func(p *Plan) error { return p.DropPath("docs.md") },
			WantCount: 2,
			WantFirst: "Add api",
		},
		{ // Test 7: A commit number the plan lacks is rejected.
			Name:  "unknown commit",
			Apply: func(p *Plan) error { return p.Reword(9, "nope") },
			Want:  ErrNoSuchCommit,
		},
		{ // Test 8: A path the plan lacks is rejected.
			Name:  "unknown path",
			Apply: func(p *Plan) error { return p.DropPath("missing.go") },
			Want:  ErrNoSuchPath,
		},
		{ // Test 9: A partial order is not a permutation.
			Name:  "short reorder",
			Apply: func(p *Plan) error { return p.Reorder([]int{1, 2}) },
			Want:  ErrInvalid,
		},
	}

	for testNum, test := range tests {
		t.Run(fmt.Sprintf("test %d %s", testNum, test.Name), func(t *testing.T) {
			t.Parallel()
			p := base()
			err := test.Apply(&p)
			if test.Want != nil {
				if !errors.Is(err, test.Want) {
					t.Errorf("edit error = %v, want %v", err, test.Want)
				}
				return
			}
			if err != nil {
				t.Fatalf("edit failed: %v", err)
			}
			if len(p.Commits) != test.WantCount {
				t.Errorf("got %d commits, want %d: %+v", len(p.Commits), test.WantCount, p.Commits)
			}
			if p.Commits[0].Subject != test.WantFirst {
				t.Errorf("first subject = %q, want %q", p.Commits[0].Subject, test.WantFirst)
			}
			// An edit must never make the plan stop covering the tree.
			if err := p.Validate(all); err != nil {
				t.Errorf("plan invalid after edit: %v", err)
			}
		})
	}
}

// TestRenderShowsSafetyContext checks that a plan which resets always shows
// the reset target, the backup ref, and the merge audit, since those are what
// a reader needs to judge the risk before approving.
func TestRenderShowsSafetyContext(t *testing.T) {
	t.Parallel()
	p := Plan{
		Base:         "0123456789012345678901234567890123456789",
		Backup:       "preen-backup/20260818-120000",
		MergeSummary: "no merges in 01234567..HEAD",
		Absorbed:     []repo.Commit{{Hash: "abcdef1234567890", Subject: "wip"}},
		Commits:      []Commit{whole("Add api", "api.go")},
	}
	var out strings.Builder
	if err := p.Render(&out); err != nil {
		t.Fatalf("Render: %v", err)
	}
	for _, want := range []string{"Reset to:", "01234567", "preen-backup/20260818-120000", "Merge check:", "wip", "Add api"} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("rendered plan is missing %q:\n%s", want, out.String())
		}
	}
}

// TestMessageJoinsBody checks that a body is separated from the subject by a
// blank line, the shape git expects.
func TestMessageJoinsBody(t *testing.T) {
	t.Parallel()
	tests := []struct {
		Name string
		In   Commit
		Want string
	}{
		{Name: "subject only", In: Commit{Subject: "Add api"}, Want: "Add api"},
		{Name: "with body", In: Commit{Subject: "Add api", Body: "Why it matters."}, Want: "Add api\n\nWhy it matters."},
		{Name: "blank body", In: Commit{Subject: "Add api", Body: "   "}, Want: "Add api"},
	}
	for testNum, test := range tests {
		t.Run(fmt.Sprintf("test %d %s", testNum, test.Name), func(t *testing.T) {
			t.Parallel()
			if got := test.In.Message(); got != test.Want {
				t.Errorf("Message() = %q, want %q", got, test.Want)
			}
		})
	}
}
