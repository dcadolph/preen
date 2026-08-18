package sweep

import (
	"fmt"
	"strings"
	"testing"

	"github.com/dcadolph/preen/repo"
)

// patchOf builds a one-file diff whose hunk adds the given lines, starting at
// the given post-image line number.
func patchOf(t *testing.T, path string, start int, added ...string) []repo.FileDiff {
	t.Helper()
	var b strings.Builder
	fmt.Fprintf(&b, "diff --git a/%s b/%s\n--- a/%s\n+++ b/%s\n", path, path, path, path)
	fmt.Fprintf(&b, "@@ -%d,1 +%d,%d @@\n context line\n", start, start, len(added)+1)
	for _, line := range added {
		fmt.Fprintf(&b, "+%s\n", line)
	}
	diffs, err := repo.ParsePatch(b.String())
	if err != nil {
		t.Fatalf("ParsePatch: %v", err)
	}
	return diffs
}

// TestScanFindsDebris checks the patterns that matter, per language.
func TestScanFindsDebris(t *testing.T) {
	t.Parallel()

	tests := []struct {
		Name string
		Path string
		Line string
		Want string
	}{
		{Name: "go print", Path: "api.go", Line: `	fmt.Println("here")`, Want: "debug print"},
		{Name: "go printf", Path: "api.go", Line: `	fmt.Printf("%v", x)`, Want: "debug print"},
		{Name: "js console", Path: "app.js", Line: `  console.log(value)`, Want: "debug print"},
		{Name: "js debugger", Path: "app.ts", Line: `  debugger`, Want: "debug print"},
		{Name: "python print", Path: "app.py", Line: `    print(value)`, Want: "debug print"},
		{Name: "scratch marker", Path: "api.go", Line: `	// XXX do not ship`, Want: "scratch marker"},
		{Name: "do not commit", Path: "notes.md", Line: `DO NOT COMMIT`, Want: "scratch marker"},
		{Name: "commented code", Path: "api.go", Line: `	// return nil`, Want: "commented-out code"},
		{Name: "skipped test", Path: "api_test.go", Line: `	t.Skip("later")`, Want: "skipped test"},
	}

	for testNum, test := range tests {
		t.Run(fmt.Sprintf("test %d %s", testNum, test.Name), func(t *testing.T) {
			t.Parallel()
			findings := Scan(patchOf(t, test.Path, 10, test.Line))
			if len(findings) != 1 {
				t.Fatalf("got %d findings, want 1: %+v", len(findings), findings)
			}
			if findings[0].Rule != test.Want {
				t.Errorf("rule = %q, want %q", findings[0].Rule, test.Want)
			}
			if findings[0].Path != test.Path {
				t.Errorf("path = %q, want %q", findings[0].Path, test.Path)
			}
		})
	}
}

// TestScanLeavesOrdinaryCodeAlone checks the bar for a finding is high, since
// a false positive costs attention on every run.
func TestScanLeavesOrdinaryCodeAlone(t *testing.T) {
	t.Parallel()

	clean := []string{
		`func Serve() error {`,
		`	return http.ListenAndServe(addr, nil)`,
		`// Serve starts the server.`,
		`	log.Info("listening", "addr", addr)`,
		`	t.Run("subtest", func(t *testing.T) {`,
		`const printWidth = 80`,
		`// This explains why the code does what it does.`,
	}
	for testNum, line := range clean {
		t.Run(fmt.Sprintf("test %d", testNum), func(t *testing.T) {
			t.Parallel()
			if findings := Scan(patchOf(t, "api.go", 1, line)); len(findings) != 0 {
				t.Errorf("ordinary line %q was flagged as %s", line, findings[0].Rule)
			}
		})
	}
}

// TestScanIgnoresRemovedLines checks that deleting a debug print is not itself
// reported as debris.
func TestScanIgnoresRemovedLines(t *testing.T) {
	t.Parallel()
	patch := `diff --git a/api.go b/api.go
--- a/api.go
+++ b/api.go
@@ -1,2 +1,1 @@
 package api
-	fmt.Println("gone")
`
	diffs, err := repo.ParsePatch(patch)
	if err != nil {
		t.Fatalf("ParsePatch: %v", err)
	}
	if findings := Scan(diffs); len(findings) != 0 {
		t.Errorf("a removed debug print was reported: %+v", findings)
	}
}

// TestScanReportsTheRightLineNumber checks that the reported line matches the
// post-image, since a wrong number sends the reader to the wrong place.
func TestScanReportsTheRightLineNumber(t *testing.T) {
	t.Parallel()
	findings := Scan(patchOf(t, "api.go", 10, "	ok := true", `	fmt.Println("here")`))
	if len(findings) != 1 {
		t.Fatalf("got %d findings, want 1", len(findings))
	}
	// The hunk starts at 10 with one context line, then two added lines, so
	// the print is the third line of the hunk.
	if findings[0].Line != 12 {
		t.Errorf("line = %d, want 12", findings[0].Line)
	}
}

// TestScanSkipsBinaries checks that a binary patch is never scanned.
func TestScanSkipsBinaries(t *testing.T) {
	t.Parallel()
	diffs := []repo.FileDiff{{Path: "logo.png", Binary: true}}
	if findings := Scan(diffs); len(findings) != 0 {
		t.Errorf("a binary file produced findings: %+v", findings)
	}
}

// TestPathsDeduplicates checks the helper that names the affected files.
func TestPathsDeduplicates(t *testing.T) {
	t.Parallel()
	findings := []Finding{
		{Path: "a.go"}, {Path: "a.go"}, {Path: "b.go"},
	}
	got := Paths(findings)
	if len(got) != 2 || got[0] != "a.go" || got[1] != "b.go" {
		t.Errorf("Paths() = %v, want [a.go b.go]", got)
	}
}
