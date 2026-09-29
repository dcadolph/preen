package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dcadolph/preen/group"
)

// agentMess commits a file with two functions far apart, then edits both ends
// for different reasons and leaves a scratch note, which is the tree an agent
// session leaves and the built-in rules cannot split.
func agentMess(c *cli) {
	c.T.Helper()
	body := "package api\n\nfunc Alpha() {}\n" + strings.Repeat("\n// filler\n", 20) + "\nfunc Omega() {}\n"
	c.write("api/server.go", body)
	c.git("add", "-A")
	c.git("commit", "-m", "Add the api server")
	edited := strings.Replace(body, "func Alpha() {}", "func Alpha() { retry() }", 1)
	edited = strings.Replace(edited, "func Omega() {}", "func Omega() { logRequest() }", 1)
	c.write("api/server.go", edited)
	c.write("NOTES.md", "scratch\n")
}

// request runs preen request and decodes what it printed.
func (c *cli) request() group.Request {
	c.T.Helper()
	code, out, err := c.run("", "request")
	if err != nil || code != CodeOK {
		c.T.Fatalf("request: code %d: %v", code, err)
	}
	var request group.Request
	if err := json.Unmarshal([]byte(out), &request); err != nil {
		c.T.Fatalf("request is not JSON: %v\n%s", err, out)
	}
	return request
}

// answer writes an answer file outside the repository, so it is not itself a
// change the answer has to account for.
func (c *cli) answer(tree string) string {
	c.T.Helper()
	path := filepath.Join(c.T.TempDir(), "answer.json")
	content := fmt.Sprintf(`{"tree":%q,"commits":[
	  {"subject":"Retry in Alpha","parts":[{"path":"api/server.go","hunks":[0]}]},
	  {"subject":"Log requests in Omega","parts":[{"path":"api/server.go","hunks":[1]}]}
	]}`, tree)
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		c.T.Fatalf("write answer: %v", err)
	}
	return path
}

// TestRequestDescribesTheTree checks that the request holds the tree hash and
// every change with its hunks, and changes nothing.
func TestRequestDescribesTheTree(t *testing.T) {
	t.Parallel()
	c := newCLI(t)
	agentMess(c)
	before := c.git("status", "--porcelain=v1")

	request := c.request()
	if request.Tree == "" {
		t.Error("request carries no tree")
	}
	hunks := map[string]int{}
	for _, file := range request.Files {
		hunks[file.Path] = len(file.Hunks)
	}
	if hunks["api/server.go"] != 2 {
		t.Errorf("api/server.go offered %d hunks, want 2: %+v", hunks["api/server.go"], request.Files)
	}
	if _, ok := hunks["NOTES.md"]; !ok {
		t.Errorf("untracked NOTES.md missing from the request: %+v", request.Files)
	}
	if after := c.git("status", "--porcelain=v1"); after != before {
		t.Errorf("request changed the tree:\n%s\nwant\n%s", after, before)
	}
}

// TestGroupingAppliesAnAgentsAnswer runs the whole loop: request, answer,
// apply. One file's two hunks land in two commits and the scratch note stays
// uncommitted.
func TestGroupingAppliesAnAgentsAnswer(t *testing.T) {
	t.Parallel()
	c := newCLI(t)
	agentMess(c)
	answer := c.answer(c.request().Tree)

	code, out, err := c.run("y\n", "--grouping", answer, "--leave", "NOTES.md")
	if err != nil {
		t.Fatalf("run: %v\n%s", err, out)
	}
	if code != CodeOK {
		t.Errorf("code = %d, want %d", code, CodeOK)
	}
	subjects := strings.TrimSpace(c.git("log", "--format=%s", "-2"))
	if subjects != "Log requests in Omega\nRetry in Alpha" {
		t.Errorf("recorded subjects:\n%s", subjects)
	}
	if status := strings.TrimSpace(c.git("status", "--porcelain=v1")); status != "?? NOTES.md" {
		t.Errorf("status after the run = %q, want only the note left", status)
	}
}

// TestGroupingRefusals checks every way an answer or its flags can be wrong.
// Each must fail before anything is staged.
func TestGroupingRefusals(t *testing.T) {
	t.Parallel()

	tests := []struct {
		Name     string
		Args     func(answer string) []string
		Stale    bool
		InRepo   bool
		WantCode int
	}{
		{ // Test 0: The tree moved after the request was answered.
			Name:     "stale answer",
			Args:     func(a string) []string { return []string{"--grouping", a, "--leave", "NOTES.md", "--yes"} },
			Stale:    true,
			WantCode: CodeInvalidPlan,
		},
		{ // Test 1: The answer leaves a change unaccounted for.
			Name:     "note neither committed nor left",
			Args:     func(a string) []string { return []string{"--grouping", a, "--yes"} },
			WantCode: CodeInvalidPlan,
		},
		{ // Test 2: Two answers to one question.
			Name:     "grouping and grouper",
			Args:     func(a string) []string { return []string{"--grouping", a, "--grouper", "/bin/cat", "--yes"} },
			WantCode: CodeErr,
		},
		{ // Test 3: Fixup takes no grouping.
			Name:     "grouping and fixup",
			Args:     func(a string) []string { return []string{"--grouping", a, "--fixup", "--yes"} },
			WantCode: CodeErr,
		},
		{ // Test 4: An answer inside the repository changes the tree it answers.
			// Without the check it would be refused as stale, with a
			// different code.
			Name:     "answer inside the repository",
			Args:     func(a string) []string { return []string{"--grouping", a, "--leave", "NOTES.md", "--yes"} },
			InRepo:   true,
			WantCode: CodeErr,
		},
		{ // Test 5: Leaving a path that is not a change.
			Name:     "leave a typo",
			Args:     func(a string) []string { return []string{"--grouping", a, "--leave", "NOTES.nd", "--yes"} },
			WantCode: CodeErr,
		},
	}

	for testNum, test := range tests {
		t.Run(fmt.Sprintf("test %d %s", testNum, test.Name), func(t *testing.T) {
			t.Parallel()
			c := newCLI(t)
			agentMess(c)
			answer := c.answer(c.request().Tree)
			if test.InRepo {
				content, err := os.ReadFile(answer)
				if err != nil {
					t.Fatalf("read answer: %v", err)
				}
				c.write("answer.json", string(content))
				answer = "answer.json"
			}
			if test.Stale {
				// Rewriting a path the answer already accounts for leaves its
				// coverage intact, so only the tree check can catch it.
				c.write("NOTES.md", "rewritten after the request\n")
			}
			head := strings.TrimSpace(c.git("rev-parse", "HEAD"))

			code, _, err := c.run("", test.Args(answer)...)
			if err == nil {
				t.Error("the run was accepted")
			}
			if code != test.WantCode {
				t.Errorf("code = %d, want %d: %v", code, test.WantCode, err)
			}
			if after := strings.TrimSpace(c.git("rev-parse", "HEAD")); after != head {
				t.Error("a refused run moved HEAD")
			}
		})
	}
}
