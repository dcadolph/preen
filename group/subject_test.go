package group

import (
	"context"
	"fmt"
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/dcadolph/preen/repo"
)

// subjectsOf runs the grouper at a given subject cap and returns the subjects
// it wrote, in plan order.
func subjectsOf(t *testing.T, maxSubject int, changes []repo.Change) []string {
	t.Helper()
	h := Heuristic{RespectStaged: true, MaxSubject: maxSubject}
	commits, err := h.Group(context.Background(), Input{Changes: changes})
	if err != nil {
		t.Fatalf("Group: %v", err)
	}
	subjects := make([]string, 0, len(commits))
	for _, commit := range commits {
		subjects = append(subjects, commit.Subject)
	}
	return subjects
}

// TestSubjectsNameWhatChanged checks that a subject says which files a commit
// holds and what happened to them, rather than naming the directory and
// stopping there.
func TestSubjectsNameWhatChanged(t *testing.T) {
	t.Parallel()

	tests := []struct {
		Name    string
		Changes []repo.Change
		Want    []string
	}{{ // Test 0: One file is named outright, path and all.
		Name:    "single file",
		Changes: []repo.Change{modified("internal/backup/store.go")},
		Want:    []string{"Update internal/backup/store.go"},
	}, { // Test 1: A handful of files are listed rather than counted.
		Name: "a few files",
		Changes: []repo.Change{
			modified("cmd/tool.go"), modified("cmd/token.go"), modified("cmd/root.go"),
		},
		Want: []string{"Update root.go, token.go, and tool.go in cmd"},
	}, { // Test 2: Past a handful the count is the honest answer.
		Name: "many files",
		Changes: []repo.Change{
			modified("cmd/a.go"), modified("cmd/b.go"),
			modified("cmd/c.go"), modified("cmd/d.go"),
		},
		Want: []string{"Update 4 files in cmd"},
	}, { // Test 3: Documentation is one commit, named by its files rather than
		// by the word documentation.
		Name:    "documentation",
		Changes: []repo.Change{modified("README.md"), modified("docs/guide.md")},
		Want:    []string{"Update README.md and guide.md"},
	}, { // Test 4: New tests for a package say which package.
		Name: "tests for a package",
		Changes: []repo.Change{
			added("internal/backup/store_test.go"), added("internal/backup/prune_test.go"),
		},
		Want: []string{"Add tests for the backup package"},
	}, { // Test 5: A deletion reads as a removal and names the file.
		Name:    "deletion",
		Changes: []repo.Change{deleted("api/legacy.go")},
		Want:    []string{"Remove api/legacy.go"},
	}, { // Test 6: Dependency manifests keep their fixed subject.
		Name:    "dependencies",
		Changes: []repo.Change{modified("go.mod"), modified("go.sum")},
		Want:    []string{"Update dependencies"},
	}, { // Test 7: Hand-staged work spanning the tree keeps the subject that
		// says why it is one commit.
		Name: "staged boundary",
		Changes: []repo.Change{
			{Path: "api/a.go", Kind: repo.KindModified, Staged: true},
			{Path: "store/b.go", Kind: repo.KindModified, Staged: true},
			{Path: "web/c.go", Kind: repo.KindModified, Staged: true},
			{Path: "docs/d.go", Kind: repo.KindModified, Staged: true},
		},
		Want: []string{"Commit staged work"},
	}}

	for testNum, test := range tests {
		t.Run(fmt.Sprintf("test %d %s", testNum, test.Name), func(t *testing.T) {
			t.Parallel()
			got := subjectsOf(t, 0, test.Changes)
			if diff := cmp.Diff(test.Want, got); diff != "" {
				t.Errorf("subject mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

// TestSubjectsGiveUpDetailToFit checks that a tight cap costs the subject
// detail rather than leaving it severed mid word.
//
// The style layer truncates whatever it is handed, so a subject that will not
// fit has to be written shorter in the first place.
func TestSubjectsGiveUpDetailToFit(t *testing.T) {
	t.Parallel()

	changes := []repo.Change{modified("internal/roundhouse/plugins/switchtender.py")}
	tests := []struct {
		Name       string
		MaxSubject int
		Want       []string
	}{ // Test 0: With room, the full path is named.
		{Name: "roomy", MaxSubject: 72, Want: []string{"Update internal/roundhouse/plugins/switchtender.py"}},
		// Test 1: With less room, the file name alone survives.
		{Name: "tight", MaxSubject: 30, Want: []string{"Update switchtender.py"}},
		// Test 2: With almost none, the directory is all that is left.
		{Name: "cramped", MaxSubject: 16, Want: []string{"Update plugins"}},
	}

	for testNum, test := range tests {
		t.Run(fmt.Sprintf("test %d %s", testNum, test.Name), func(t *testing.T) {
			t.Parallel()
			got := subjectsOf(t, test.MaxSubject, changes)
			if diff := cmp.Diff(test.Want, got); diff != "" {
				t.Errorf("subject mismatch (-want +got):\n%s", diff)
			}
			for _, subject := range got {
				if len([]rune(subject)) > test.MaxSubject {
					t.Errorf("subject %q is %d characters, over the cap of %d",
						subject, len([]rune(subject)), test.MaxSubject)
				}
			}
		})
	}
}
