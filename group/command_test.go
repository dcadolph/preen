package group

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/dcadolph/preen/repo"
)

// fakeGrouper writes a shell script that echoes a fixed response, standing in
// for whatever program a user points preen at.
func fakeGrouper(t *testing.T, response string) Command {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "grouper.sh")
	script := "#!/bin/sh\ncat > /dev/null\ncat <<'JSON'\n" + response + "\nJSON\n"
	if err := os.WriteFile(path, []byte(script), 0o700); err != nil { //nolint:gosec // Test fixture.
		t.Fatalf("write script: %v", err)
	}
	return Command{Name: path, Dir: dir}
}

// splitInput is a two-hunk file, the case only a smarter grouper can divide.
func splitInput() Input {
	diff, err := repo.ParsePatch(`diff --git a/api/server.go b/api/server.go
--- a/api/server.go
+++ b/api/server.go
@@ -1,3 +1,4 @@ func alpha()
 package api

+// added near the top
 func alpha() {}
@@ -20,3 +21,4 @@ func omega()

 func omega() {}
+// added near the bottom

`)
	if err != nil {
		panic(err)
	}
	return Input{
		Changes: []repo.Change{{Path: "api/server.go", Kind: repo.KindModified}},
		Diffs:   diff,
	}
}

// TestCommandSplitsByHunk checks the capability the deterministic grouper
// deliberately lacks: dividing one file's hunks across separate commits.
func TestCommandSplitsByHunk(t *testing.T) {
	t.Parallel()
	grouper := fakeGrouper(t, `{"commits":[
	  {"subject":"Note the top","parts":[{"path":"api/server.go","hunks":[0]}]},
	  {"subject":"Note the bottom","parts":[{"path":"api/server.go","hunks":[1]}]}
	]}`)

	commits, err := grouper.Group(context.Background(), splitInput())
	if err != nil {
		t.Fatalf("Group: %v", err)
	}
	if len(commits) != 2 {
		t.Fatalf("got %d commits, want the file split in two", len(commits))
	}
	for i, commit := range commits {
		if len(commit.Parts) != 1 || commit.Parts[0].Whole() {
			t.Errorf("commit %d did not take a hunk subset: %+v", i+1, commit.Parts)
		}
		if commit.Parts[0].Hunks[0].Body == "" {
			t.Errorf("commit %d carries no hunk body to find the hunk again with", i+1)
		}
	}
	if commits[0].Parts[0].Hunks[0].Index == commits[1].Parts[0].Hunks[0].Index {
		t.Error("both commits claim the same hunk")
	}
}

// TestCommandRejectsUntrustworthyAnswers checks that anything the grouper
// invents is refused rather than acted on, since it cannot be verified against
// the tree.
func TestCommandRejectsUntrustworthyAnswers(t *testing.T) {
	t.Parallel()

	tests := []struct {
		Name string
		In   string
		Want error
	}{
		{
			Name: "path that does not exist",
			In:   `{"commits":[{"subject":"Invented","parts":[{"path":"nope.go"}]}]}`,
			Want: ErrResponse,
		},
		{
			Name: "hunk out of range",
			In:   `{"commits":[{"subject":"Too far","parts":[{"path":"api/server.go","hunks":[9]}]}]}`,
			Want: ErrResponse,
		},
		{
			Name: "no subject",
			In:   `{"commits":[{"subject":"","parts":[{"path":"api/server.go"}]}]}`,
			Want: ErrResponse,
		},
		{
			Name: "commit with no files",
			In:   `{"commits":[{"subject":"Empty","parts":[]}]}`,
			Want: ErrResponse,
		},
		{
			Name: "no commits at all",
			In:   `{"commits":[]}`,
			Want: ErrResponse,
		},
		{
			Name: "not json",
			In:   `sorry, I could not do that`,
			Want: ErrResponse,
		},
	}

	for testNum, test := range tests {
		t.Run(fmt.Sprintf("test %d %s", testNum, test.Name), func(t *testing.T) {
			t.Parallel()
			_, err := fakeGrouper(t, test.In).Group(context.Background(), splitInput())
			if !errors.Is(err, test.Want) {
				t.Errorf("Group() = %v, want %v", err, test.Want)
			}
		})
	}
}

// TestCommandToleratesSurroundingProse checks that a chat-oriented tool
// wrapping its JSON in explanation still works, since that is what they do.
func TestCommandToleratesSurroundingProse(t *testing.T) {
	t.Parallel()
	grouper := fakeGrouper(t, `Sure! Here is the grouping you asked for:
{"commits":[{"subject":"Update the server","parts":[{"path":"api/server.go"}]}]}
Let me know if you want it split differently.`)

	commits, err := grouper.Group(context.Background(), splitInput())
	if err != nil {
		t.Fatalf("Group: %v", err)
	}
	if len(commits) != 1 || commits[0].Subject != "Update the server" {
		t.Errorf("got %+v, want the wrapped JSON to be read", commits)
	}
}

// TestChainFallsBackFromFailingCommand checks that a broken grouper degrades
// to the built-in rules instead of taking the run down.
func TestChainFallsBackFromFailingCommand(t *testing.T) {
	t.Parallel()
	broken := Command{Name: filepath.Join(t.TempDir(), "does-not-exist")}
	commits, err := Chain(broken, NewHeuristic()).Group(context.Background(), splitInput())
	if err != nil {
		t.Fatalf("Chain: %v", err)
	}
	if len(commits) == 0 {
		t.Fatal("chain returned nothing, want the deterministic fallback")
	}
	if !commits[0].Parts[0].Whole() {
		t.Error("the fallback split a file, which the deterministic grouper never does")
	}
}

// TestCommandNeedsAProgram checks the empty configuration is refused rather
// than silently doing nothing.
func TestCommandNeedsAProgram(t *testing.T) {
	t.Parallel()
	if _, err := (Command{}).Group(context.Background(), splitInput()); !errors.Is(err, ErrNoCommand) {
		t.Errorf("Group with no command = %v, want ErrNoCommand", err)
	}
}
