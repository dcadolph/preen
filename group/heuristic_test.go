package group

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/dcadolph/preen/plan"
	"github.com/dcadolph/preen/repo"
)

// changesOf builds a change set of modified paths.
func changesOf(paths ...string) []repo.Change {
	out := make([]repo.Change, 0, len(paths))
	for _, p := range paths {
		out = append(out, repo.Change{Path: p, Kind: repo.KindModified})
	}
	return out
}

// commitSubjectFor returns the subject of the commit holding a path.
func commitSubjectFor(t *testing.T, in Input, path string) string {
	t.Helper()
	commits, err := NewHeuristic().Group(context.Background(), in)
	if err != nil {
		t.Fatalf("Group: %v", err)
	}
	for _, commit := range commits {
		for _, part := range commit.Parts {
			if part.Path == path {
				return commit.Subject
			}
		}
	}
	t.Fatalf("no commit carries %s", path)
	return ""
}

// TestHeuristicSeparatesCategories checks that unrelated kinds of change never
// share a commit, which is the whole point of grouping.
func TestHeuristicSeparatesCategories(t *testing.T) {
	t.Parallel()
	in := Input{Changes: changesOf(
		"go.mod", "go.sum",
		"api/server.go", "api/server_test.go",
		"store/db.go",
		"docs/guide.md",
		".github/workflows/ci.yml",
		"config.yaml",
	)}
	commits, err := NewHeuristic().Group(context.Background(), in)
	if err != nil {
		t.Fatalf("Group: %v", err)
	}

	// Each commit must hold exactly one category, checked by the subject the
	// grouper chose for it.
	for _, commit := range commits {
		var dirs []string
		for _, part := range commit.Parts {
			if strings.Contains(part.Path, "/") {
				dirs = append(dirs, strings.SplitN(part.Path, "/", 2)[0])
			}
		}
		for _, dir := range dirs {
			if dir != dirs[0] {
				t.Errorf("commit %q mixes directories %v", commit.Subject, dirs)
				break
			}
		}
	}
	if got := commits[0].Subject; got != "Update dependencies" {
		t.Errorf("first commit = %q, want dependencies first so code that needs them follows", got)
	}
	// Both dependency files belong to one commit rather than two.
	if got := len(commits[0].Parts); got != 2 {
		t.Errorf("dependency commit holds %d files, want go.mod and go.sum together", got)
	}
}

// TestHeuristicKeepsTestsWithSource checks that a test file rides along with
// the package it exercises, so a commit stays buildable and reviewable.
func TestHeuristicKeepsTestsWithSource(t *testing.T) {
	t.Parallel()
	in := Input{Changes: changesOf("api/server.go", "api/server_test.go")}
	if src, test := commitSubjectFor(t, in, "api/server.go"), commitSubjectFor(t, in, "api/server_test.go"); src != test {
		t.Errorf("source and test split across commits: %q and %q", src, test)
	}
}

// TestHeuristicVerbMatchesTheChange checks that the generated subject uses the
// verb the change actually performs.
func TestHeuristicVerbMatchesTheChange(t *testing.T) {
	t.Parallel()

	tests := []struct {
		Name       string
		Changes    []repo.Change
		WantPrefix string
	}{
		{
			Name:       "all new files",
			Changes:    []repo.Change{{Path: "api/server.go", Kind: repo.KindUntracked}},
			WantPrefix: "Add",
		},
		{
			Name:       "all deletions",
			Changes:    []repo.Change{{Path: "api/old.go", Kind: repo.KindDeleted}},
			WantPrefix: "Remove",
		},
		{
			Name:       "edits",
			Changes:    []repo.Change{{Path: "api/server.go", Kind: repo.KindModified}},
			WantPrefix: "Update",
		},
		{
			Name: "mixed becomes an update",
			Changes: []repo.Change{
				{Path: "api/server.go", Kind: repo.KindUntracked},
				{Path: "api/old.go", Kind: repo.KindDeleted},
			},
			WantPrefix: "Update",
		},
	}

	for testNum, test := range tests {
		t.Run(fmt.Sprintf("test %d %s", testNum, test.Name), func(t *testing.T) {
			t.Parallel()
			commits, err := NewHeuristic().Group(context.Background(), Input{Changes: test.Changes})
			if err != nil {
				t.Fatalf("Group: %v", err)
			}
			if len(commits) != 1 {
				t.Fatalf("got %d commits, want 1", len(commits))
			}
			if !strings.HasPrefix(commits[0].Subject, test.WantPrefix) {
				t.Errorf("subject = %q, want it to start with %q", commits[0].Subject, test.WantPrefix)
			}
		})
	}
}

// TestHeuristicRespectsStagedBoundary checks that work the user staged by hand
// is treated as a commit boundary they drew deliberately.
func TestHeuristicRespectsStagedBoundary(t *testing.T) {
	t.Parallel()
	in := Input{Changes: []repo.Change{
		{Path: "api/server.go", Kind: repo.KindModified, Staged: true},
		{Path: "api/client.go", Kind: repo.KindModified, Unstaged: true},
	}}
	staged := commitSubjectFor(t, in, "api/server.go")
	unstaged := commitSubjectFor(t, in, "api/client.go")
	if staged == unstaged {
		t.Errorf("staged and unstaged work landed in the same commit (%q)", staged)
	}

	// With the hint off, the same files group together by directory.
	commits, err := Heuristic{}.Group(context.Background(), in)
	if err != nil {
		t.Fatalf("Group: %v", err)
	}
	if len(commits) != 1 {
		t.Errorf("got %d commits with the staged hint off, want 1", len(commits))
	}
}

// TestHeuristicNeverSplitsFiles checks that the deterministic grouper only
// ever takes whole files, since it cannot know whether two hunks are one idea.
func TestHeuristicNeverSplitsFiles(t *testing.T) {
	t.Parallel()
	in := Input{Changes: changesOf("api/server.go", "store/db.go", "docs/guide.md")}
	commits, err := NewHeuristic().Group(context.Background(), in)
	if err != nil {
		t.Fatalf("Group: %v", err)
	}
	for _, commit := range commits {
		for _, part := range commit.Parts {
			if !part.Whole() {
				t.Errorf("commit %q takes %s by hunk, want whole files only", commit.Subject, part.Path)
			}
		}
	}
}

// TestChainFallsBack checks that a grouper returning nothing hands off to the
// next one, which is how a model-backed grouper degrades to the deterministic
// one when it is unavailable.
func TestChainFallsBack(t *testing.T) {
	t.Parallel()
	empty := GrouperFunc(func(context.Context, Input) ([]plan.Commit, error) { return nil, nil })
	in := Input{Changes: changesOf("api/server.go")}

	commits, err := Chain(empty, NewHeuristic()).Group(context.Background(), in)
	if err != nil {
		t.Fatalf("Chain: %v", err)
	}
	if len(commits) == 0 {
		t.Error("chain returned nothing, want the fallback grouper's result")
	}
}
