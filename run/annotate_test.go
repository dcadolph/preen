package run

import (
	"context"
	"strings"
	"testing"

	"github.com/dcadolph/preen/style"
)

// TestIncludeFilesListsPaths checks that the body names what the commit
// touched when the run asks for it.
func TestIncludeFilesListsPaths(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	h := newHarness(t)
	h.write("api/server.go", "package api\n")
	h.write("api/client.go", "package api\n")

	p, err := h.Plan(ctx, Options{IncludeFiles: true})
	if err != nil {
		t.Fatalf("Plan: %v", err)
	}
	body := p.Commits[0].Body
	for _, want := range []string{"- api/client.go", "- api/server.go"} {
		if !strings.Contains(body, want) {
			t.Errorf("body missing %q:\n%s", want, body)
		}
	}
}

// TestIncludeLineNumbersCitesRanges checks that the body cites the ranges the
// commit actually changes, read from the real hunk headers.
func TestIncludeLineNumbersCitesRanges(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	h := newHarness(t)
	h.write("api.go", spreadFile)
	h.git("add", "-A")
	h.git("commit", "-m", "Add api")
	h.write("api.go", strings.Replace(spreadFile, "return 2", "return 200", 1))

	p, err := h.Plan(ctx, Options{IncludeLineNumbers: true})
	if err != nil {
		t.Fatalf("Plan: %v", err)
	}
	body := p.Commits[0].Body
	if !strings.Contains(body, "api.go:") {
		t.Fatalf("body does not cite the file:\n%s", body)
	}
	// The edit is near the bottom of the file, so the cited range must not
	// start at line one.
	if strings.Contains(body, "api.go:1-") || strings.Contains(body, "api.go:1,") {
		t.Errorf("cited range looks wrong for a change at the bottom:\n%s", body)
	}
}

// TestBodyNeverStripsTheBody checks the body mode reaches the recorded message.
func TestBodyNeverStripsTheBody(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	h := newHarness(t)
	h.write("api/server.go", "package api\n")

	p, err := h.Plan(ctx, Options{
		IncludeFiles: true,
		Style:        style.Style{Body: style.BodyNever},
	})
	if err != nil {
		t.Fatalf("Plan: %v", err)
	}
	if p.Commits[0].Body != "" {
		t.Errorf("body survived the never mode:\n%s", p.Commits[0].Body)
	}
}

// TestPunctuationAutoFollowsTheRepository checks that auto reads the project's
// own convention rather than imposing one.
func TestPunctuationAutoFollowsTheRepository(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	tests := []struct {
		Name     string
		Subjects []string
		WantDot  bool
	}{
		{
			Name:     "repository ends subjects with periods",
			Subjects: []string{"Add the parser.", "Fix the lexer.", "Update the docs."},
			WantDot:  true,
		},
		{
			Name:     "repository does not",
			Subjects: []string{"Add the parser", "Fix the lexer", "Update the docs"},
			WantDot:  false,
		},
	}

	for testNum, test := range tests {
		t.Run(test.Name, func(t *testing.T) {
			t.Parallel()
			h := newHarness(t)
			for i, subject := range test.Subjects {
				h.write("seed"+string(rune('a'+i))+".txt", subject+"\n")
				h.git("add", "-A")
				h.git("commit", "-m", subject)
			}
			h.write("api/server.go", "package api\n")

			p, err := h.Plan(ctx, Options{Style: style.Style{Punctuation: style.PunctAuto}})
			if err != nil {
				t.Fatalf("test %d Plan: %v", testNum, err)
			}
			got := strings.HasSuffix(p.Commits[0].Subject, ".")
			if got != test.WantDot {
				t.Errorf("subject %q ends with a period = %v, want %v",
					p.Commits[0].Subject, got, test.WantDot)
			}
		})
	}
}

// TestSweepReportsDebrisWithoutRemovingIt checks that a run flags leftovers and
// still commits the content untouched, since removing is never automatic.
func TestSweepReportsDebrisWithoutRemovingIt(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	h := newHarness(t)
	h.write("api/server.go", "package api\n\nfunc Serve() {\n\tfmt.Println(\"debugging\")\n}\n")

	p, err := h.Plan(ctx, Options{Sweep: true})
	if err != nil {
		t.Fatalf("Plan: %v", err)
	}
	if len(p.Debris) == 0 {
		t.Fatal("the sweep found nothing, want the debug print reported")
	}
	if !strings.Contains(p.Debris[0], "api/server.go") {
		t.Errorf("finding does not name the file: %s", p.Debris[0])
	}

	result, err := h.Apply(ctx, p, Options{Sweep: true})
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if result.TreeStart != result.TreeEnd {
		t.Error("the sweep removed something, want it only to report")
	}
	committed := h.git("show", "--format=", "HEAD")
	if !strings.Contains(committed, "debugging") {
		t.Errorf("the reported line was dropped from the commit:\n%s", committed)
	}
}
