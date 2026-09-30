package group

import (
	"context"
	"fmt"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"

	"github.com/dcadolph/preen/v2/repo"
)

// grouped is one planned commit flattened to the two things a caller judges it
// by: what it is called and what it carries.
type grouped struct {
	// Subject is the commit's subject line.
	Subject string
	// Paths are the files the commit carries, in listing order.
	Paths []string
}

// groupingOf runs the built-in grouper over a change set and flattens the
// result, so a test states the whole plan rather than probing at it.
func groupingOf(t *testing.T, changes []repo.Change) []grouped {
	t.Helper()
	commits, err := NewHeuristic().Group(context.Background(), Input{Changes: changes})
	if err != nil {
		t.Fatalf("Group: %v", err)
	}
	out := make([]grouped, 0, len(commits))
	for _, commit := range commits {
		out = append(out, grouped{Subject: commit.Subject, Paths: commit.Paths()})
	}
	return out
}

// added returns an untracked change, the shape a brand new file arrives in.
func added(path string) repo.Change {
	return repo.Change{Path: path, Kind: repo.KindUntracked}
}

// modified returns a change to a file already tracked.
func modified(path string) repo.Change {
	return repo.Change{Path: path, Kind: repo.KindModified}
}

// deleted returns a removed file.
func deleted(path string) repo.Change {
	return repo.Change{Path: path, Kind: repo.KindDeleted}
}

// TestUnitsShapeTheCommits checks that one directory's changes are divided by
// what they are rather than only by where they live.
//
// Grouping by directory alone is what produced a commit called "Update cmd"
// holding two unrelated fixes and a new test file. The unit is the smallest
// group of paths a file name proves belong together, and new code, changed
// code, and standalone tests are separated from each other.
func TestUnitsShapeTheCommits(t *testing.T) {
	t.Parallel()

	tests := []struct {
		Name    string
		Changes []repo.Change
		Want    []grouped
	}{{ // Test 0: The case that exposed the gap: two unrelated edits plus a new
		// file and its test, all in one directory.
		Name: "new work separated from edits",
		Changes: []repo.Change{
			modified("cmd/tool.go"),
			modified("cmd/token.go"),
			added("cmd/parser.go"),
			added("cmd/parser_test.go"),
		},
		Want: []grouped{
			{
				Subject: "Add cmd/parser.go and its test",
				Paths:   []string{"cmd/parser.go", "cmd/parser_test.go"},
			},
			{
				Subject: "Update token.go and tool.go in cmd",
				Paths:   []string{"cmd/token.go", "cmd/tool.go"},
			},
		},
	}, { // Test 1: A test for code this run did not touch is its own work and
		// lands after the code it depends on.
		Name: "standalone tests stand alone",
		Changes: []repo.Change{
			modified("cmd/token.go"),
			added("cmd/tool_test.go"),
		},
		Want: []grouped{
			{Subject: "Update cmd/token.go", Paths: []string{"cmd/token.go"}},
			{Subject: "Add tests for cmd/tool.go", Paths: []string{"cmd/tool_test.go"}},
		},
	}, { // Test 2: A test whose source changed rides with it, whatever kinds of
		// change the two carry.
		Name: "a test rides with its source",
		Changes: []repo.Change{
			modified("cmd/token.go"),
			added("cmd/token_test.go"),
		},
		Want: []grouped{{
			Subject: "Update cmd/token.go and its test",
			Paths:   []string{"cmd/token.go", "cmd/token_test.go"},
		}},
	}, { // Test 3: Python conventions are read the same way as Go ones.
		Name: "python test naming",
		Changes: []repo.Change{
			modified("plugins/runner.py"),
			added("plugins/test_runner.py"),
		},
		Want: []grouped{{
			Subject: "Update plugins/runner.py and its test",
			Paths:   []string{"plugins/runner.py", "plugins/test_runner.py"},
		}},
	}, { // Test 4: A file that moved between directories is one commit that says
		// so, not a deletion and an unrelated addition.
		Name: "a move stays together",
		Changes: []repo.Change{
			added("internal/util/text.go"),
			deleted("util/text.go"),
		},
		Want: []grouped{{
			Subject: "Move text.go from util to internal/util",
			Paths:   []string{"internal/util/text.go", "util/text.go"},
		}},
	}, { // Test 5: An addition and a deletion with different names are not a
		// move, and are not forced into one commit either.
		Name: "unrelated add and delete",
		Changes: []repo.Change{
			added("api/server.go"),
			deleted("api/old.go"),
		},
		Want: []grouped{
			{Subject: "Add api/server.go", Paths: []string{"api/server.go"}},
			{Subject: "Remove api/old.go", Paths: []string{"api/old.go"}},
		},
	}, { // Test 6: Two packages never share a commit, whatever their changes.
		Name: "packages stay apart",
		Changes: []repo.Change{
			added("api/server.go"),
			added("store/db.go"),
		},
		Want: []grouped{
			{Subject: "Add api/server.go", Paths: []string{"api/server.go"}},
			{Subject: "Add store/db.go", Paths: []string{"store/db.go"}},
		},
	}}

	for testNum, test := range tests {
		t.Run(fmt.Sprintf("test %d %s", testNum, test.Name), func(t *testing.T) {
			t.Parallel()
			got := groupingOf(t, test.Changes)
			if diff := cmp.Diff(test.Want, got, cmpopts.EquateEmpty()); diff != "" {
				t.Errorf("grouping mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

// TestMovePairsNeedAnUnambiguousName checks that a move is only claimed when
// exactly one addition and one deletion answer to a file name, since pairing on
// a guess would put two unrelated files in one commit.
func TestMovePairsNeedAnUnambiguousName(t *testing.T) {
	t.Parallel()

	tests := []struct {
		Name       string
		Changes    []repo.Change
		WantCommit int
	}{{ // Test 0: One addition and one deletion of the same name is a move.
		Name:       "unambiguous move",
		Changes:    []repo.Change{added("new/x.go"), deleted("old/x.go")},
		WantCommit: 1,
	}, { // Test 1: Two additions of the same name leave the pairing ambiguous,
		// so no move is claimed.
		Name: "two candidates",
		Changes: []repo.Change{
			added("new/x.go"), added("other/x.go"), deleted("old/x.go"),
		},
		WantCommit: 3,
	}, { // Test 2: A name that stays in its directory is an edit, not a move.
		Name:       "same directory",
		Changes:    []repo.Change{added("pkg/x.go"), deleted("pkg/x.go")},
		WantCommit: 1,
	}}

	for testNum, test := range tests {
		t.Run(fmt.Sprintf("test %d %s", testNum, test.Name), func(t *testing.T) {
			t.Parallel()
			got := groupingOf(t, test.Changes)
			if len(got) != test.WantCommit {
				t.Errorf("got %d commits, want %d: %+v", len(got), test.WantCommit, got)
			}
		})
	}
}

// TestTestSourceReadsNamingConventions checks the one pairing a file name
// states outright, since every unit is built on it.
func TestTestSourceReadsNamingConventions(t *testing.T) {
	t.Parallel()

	tests := []struct {
		Path       string
		WantSource string
		WantTest   bool
	}{ // Test 0: The Go convention.
		{Path: "cmd/parser_test.go", WantSource: "cmd/parser.go", WantTest: true},
		// Test 1: The Python convention.
		{Path: "plugins/test_runner.py", WantSource: "plugins/runner.py", WantTest: true},
		// Test 2: The JavaScript convention.
		{Path: "web/button.test.ts", WantSource: "web/button.ts", WantTest: true},
		// Test 3: The Ruby and JavaScript spec convention.
		{Path: "app/user_spec.rb", WantSource: "app/user.rb", WantTest: true},
		// Test 4: A file at the repository root.
		{Path: "main_test.go", WantSource: "main.go", WantTest: true},
		// Test 5: Ordinary source is not a test.
		{Path: "cmd/parser.go", WantTest: false},
		// Test 6: A name that is only the marker has no source to pair with.
		{Path: "cmd/_test.go", WantTest: false},
		// Test 7: A word ending in test is not the marker.
		{Path: "cmd/latest.go", WantTest: false},
	}

	for testNum, test := range tests {
		t.Run(fmt.Sprintf("test %d %s", testNum, test.Path), func(t *testing.T) {
			t.Parallel()
			source, isTest := testSource(test.Path)
			if isTest != test.WantTest {
				t.Fatalf("testSource(%q) reported %v, want %v", test.Path, isTest, test.WantTest)
			}
			if diff := cmp.Diff(test.WantSource, source, cmpopts.EquateEmpty()); diff != "" {
				t.Errorf("source mismatch (-want +got):\n%s", diff)
			}
		})
	}
}
