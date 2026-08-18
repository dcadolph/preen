package plan

import (
	"errors"
	"fmt"
	"testing"

	"github.com/dcadolph/preen/repo"
)

// TestParseAction checks that the approval prompt reads the commands the way
// they are written in the help, filler words and all.
func TestParseAction(t *testing.T) {
	t.Parallel()

	tests := []struct {
		Name     string
		In       string
		WantKind ActionKind
		Want     error
	}{
		{Name: "apply", In: "y", WantKind: ActionApply},
		{Name: "apply spelled out", In: "apply", WantKind: ActionApply},
		{Name: "abort", In: "n", WantKind: ActionAbort},
		{Name: "show", In: "show", WantKind: ActionShow},
		{Name: "help", In: "?", WantKind: ActionHelp},
		{Name: "merge with filler", In: "merge 2 into 1", WantKind: ActionEdit},
		{Name: "merge without filler", In: "merge 2 1", WantKind: ActionEdit},
		{Name: "split", In: "split 3", WantKind: ActionEdit},
		{Name: "split by file", In: "split 3 by file", WantKind: ActionEdit},
		{Name: "move", In: "move api/server.go to 2", WantKind: ActionEdit},
		{Name: "reword", In: "reword 1 Add the parser", WantKind: ActionEdit},
		{Name: "reword with colon", In: "reword 1: Add the parser", WantKind: ActionEdit},
		{Name: "drop", In: "drop notes.txt", WantKind: ActionEdit},
		{Name: "reorder commas", In: "reorder 3,1,2", WantKind: ActionEdit},
		{Name: "reorder spaces", In: "reorder 3 1 2", WantKind: ActionEdit},
		{Name: "empty", In: "   ", Want: ErrUsage},
		{Name: "unknown verb", In: "frobnicate 2", Want: ErrUsage},
		{Name: "merge missing target", In: "merge 2", Want: ErrUsage},
		{Name: "merge not a number", In: "merge two into one", Want: ErrUsage},
		{Name: "reword missing subject", In: "reword 1", Want: ErrUsage},
		{Name: "move missing number", In: "move api/server.go", Want: ErrUsage},
	}

	for testNum, test := range tests {
		t.Run(fmt.Sprintf("test %d %s", testNum, test.Name), func(t *testing.T) {
			t.Parallel()
			got, err := ParseAction(test.In)
			if test.Want != nil {
				if !errors.Is(err, test.Want) {
					t.Errorf("ParseAction(%q) error = %v, want %v", test.In, err, test.Want)
				}
				return
			}
			if err != nil {
				t.Fatalf("ParseAction(%q): %v", test.In, err)
			}
			if got.Kind != test.WantKind {
				t.Errorf("ParseAction(%q) kind = %v, want %v", test.In, got.Kind, test.WantKind)
			}
			if got.Kind == ActionEdit && got.Apply == nil {
				t.Error("edit action carries no change to apply")
			}
		})
	}
}

// TestParsedEditsChangeThePlan checks that a parsed command actually performs
// the move it names, so the grammar and the plan API cannot drift apart.
func TestParsedEditsChangeThePlan(t *testing.T) {
	t.Parallel()

	base := func() *Plan {
		return &Plan{
			Commits: []Commit{
				{Subject: "Add api", Parts: []Part{{Path: "api.go"}}},
				{Subject: "Add store", Parts: []Part{{Path: "store.go"}}},
				{Subject: "Add docs", Parts: []Part{{Path: "docs.md"}}},
			},
			Covers: []repo.Change{{Path: "api.go"}, {Path: "store.go"}, {Path: "docs.md"}},
		}
	}

	tests := []struct {
		Name      string
		In        string
		WantCount int
		WantFirst string
	}{
		{Name: "merge", In: "merge 2 into 1", WantCount: 2, WantFirst: "Add api"},
		{Name: "reword", In: "reword 1 Add the API server", WantCount: 3, WantFirst: "Add the API server"},
		{Name: "reorder", In: "reorder 3,2,1", WantCount: 3, WantFirst: "Add docs"},
		{Name: "drop", In: "drop docs.md", WantCount: 2, WantFirst: "Add api"},
		{Name: "move", In: "move store.go to 1", WantCount: 2, WantFirst: "Add api"},
	}

	for testNum, test := range tests {
		t.Run(fmt.Sprintf("test %d %s", testNum, test.Name), func(t *testing.T) {
			t.Parallel()
			p := base()
			action, err := ParseAction(test.In)
			if err != nil {
				t.Fatalf("ParseAction(%q): %v", test.In, err)
			}
			if err := action.Apply(p); err != nil {
				t.Fatalf("apply %q: %v", test.In, err)
			}
			if len(p.Commits) != test.WantCount {
				t.Errorf("got %d commits, want %d", len(p.Commits), test.WantCount)
			}
			if p.Commits[0].Subject != test.WantFirst {
				t.Errorf("first subject = %q, want %q", p.Commits[0].Subject, test.WantFirst)
			}
			// The tree must still be fully accounted for after any edit.
			if err := p.Revalidate(); err != nil {
				t.Errorf("plan stopped covering the tree: %v", err)
			}
			if action.Describe == "" {
				t.Error("edit has no description to echo back")
			}
		})
	}
}
