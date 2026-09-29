package group

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

// writeAnswer puts an answer where an Answer grouper can read it.
func writeAnswer(t *testing.T, content string) Answer {
	t.Helper()
	path := filepath.Join(t.TempDir(), "answer.json")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write answer: %v", err)
	}
	return Answer{Path: path}
}

// TestAnswerSplitsByHunk checks that an answer for the right tree is converted
// like any grouper's, including a split of one file's hunks.
func TestAnswerSplitsByHunk(t *testing.T) {
	t.Parallel()
	in := splitInput()
	in.Tree = "abc123"
	answer := writeAnswer(t, `{"tree":"abc123","commits":[
	  {"subject":"Note the top","parts":[{"path":"api/server.go","hunks":[0]}]},
	  {"subject":"Note the bottom","parts":[{"path":"api/server.go","hunks":[1]}]}
	]}`)

	commits, err := answer.Group(context.Background(), in)
	if err != nil {
		t.Fatalf("Group: %v", err)
	}
	if len(commits) != 2 {
		t.Fatalf("got %d commits, want the file split in two", len(commits))
	}
	if commits[0].Parts[0].Hunks[0].Index == commits[1].Parts[0].Hunks[0].Index {
		t.Error("both commits claim the same hunk")
	}
}

// TestAnswerRejects checks that an answer is refused whenever it cannot be
// shown to describe this tree, since its hunk indexes would then point at
// whatever sits there now.
func TestAnswerRejects(t *testing.T) {
	t.Parallel()

	tests := []struct {
		Name    string
		Content string
		Missing bool
		Want    error
	}{
		{ // Test 0: An answer for an earlier tree.
			Name:    "stale tree",
			Content: `{"tree":"old","commits":[{"subject":"Top","parts":[{"path":"api/server.go"}]}]}`,
			Want:    ErrStale,
		},
		{ // Test 1: An answer that never says which tree it answers.
			Name:    "no tree",
			Content: `{"commits":[{"subject":"Top","parts":[{"path":"api/server.go"}]}]}`,
			Want:    ErrStale,
		},
		{ // Test 2: The right tree does not excuse an invented path.
			Name:    "invented path",
			Content: `{"tree":"abc123","commits":[{"subject":"Nope","parts":[{"path":"nope.go"}]}]}`,
			Want:    ErrResponse,
		},
		{ // Test 3: Not JSON at all.
			Name:    "not json",
			Content: `here is my grouping`,
			Want:    ErrResponse,
		},
		{ // Test 4: No file to read.
			Name:    "missing file",
			Missing: true,
			Want:    ErrAnswer,
		},
	}

	for testNum, test := range tests {
		t.Run(fmt.Sprintf("test %d %s", testNum, test.Name), func(t *testing.T) {
			t.Parallel()
			in := splitInput()
			in.Tree = "abc123"
			answer := Answer{Path: filepath.Join(t.TempDir(), "absent.json")}
			if !test.Missing {
				answer = writeAnswer(t, test.Content)
			}
			if _, err := answer.Group(context.Background(), in); !errors.Is(err, test.Want) {
				t.Errorf("Group() = %v, want %v", err, test.Want)
			}
		})
	}
}

// TestNewRequestCarriesTree checks that the request names its tree, which is
// what an answer must echo back.
func TestNewRequestCarriesTree(t *testing.T) {
	t.Parallel()
	in := splitInput()
	in.Tree = "abc123"
	request := NewRequest(in)
	if request.Tree != "abc123" {
		t.Errorf("request tree = %q, want %q", request.Tree, "abc123")
	}
	if len(request.Files) != 1 || len(request.Files[0].Hunks) != 2 {
		t.Errorf("request lost the file or its hunks: %+v", request.Files)
	}
}
