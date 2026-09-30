package run

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"

	"github.com/dcadolph/preen/v2/plan"
)

// pycache is the path that exposed the gap: a Python bytecode cache the test
// suite rewrites on every run, reported as untracked because the repository's
// .gitignore never listed it.
const pycache = "internal/roundhouse/plugins/__pycache__/switchtender.cpython-313.pyc"

// dirtyWithGenerated writes real work alongside the build droppings that sit
// next to it in a working tree nobody has swept.
func (h *harness) dirtyWithGenerated() {
	h.write("internal/roundhouse/plugins/switchtender.py", "def run():\n    return 1\n")
	h.write(pycache, "\x00\x01compiled\n")
	h.write("internal/roundhouse/plugins/__pycache__/helper.cpython-313.pyc", "\x00\x01compiled\n")
	h.write(".DS_Store", "macos metadata")
}

// heldPaths returns the paths a plan held back, in listing order.
func heldPaths(p *plan.Plan) []string {
	paths := make([]string, 0, len(p.Held))
	for _, held := range p.Held {
		paths = append(paths, held.Part.Path)
	}
	return paths
}

// plannedPaths returns every path a plan's commits would record.
func plannedPaths(p *plan.Plan) []string {
	var paths []string
	for _, commit := range p.Commits {
		paths = append(paths, commit.Paths()...)
	}
	return paths
}

// TestHoldsGeneratedOutputBack is the regression test for the defect that
// started this: preen planned a commit called "Add __pycache__" holding a
// Python bytecode cache, because git reported it as untracked and nothing in
// preen knew what it was.
//
// The cache must not reach a commit, it must be reported with the reason, and
// it must still be in the working tree afterward, untracked and untouched.
func TestHoldsGeneratedOutputBack(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	h := newHarness(t)
	h.dirtyWithGenerated()

	p, err := h.Plan(ctx, Options{})
	if err != nil {
		t.Fatalf("Plan: %v", err)
	}
	want := []string{
		".DS_Store",
		"internal/roundhouse/plugins/__pycache__/helper.cpython-313.pyc",
		pycache,
	}
	sorted := cmpopts.SortSlices(func(a, b string) bool { return a < b })
	if diff := cmp.Diff(want, heldPaths(p), sorted, cmpopts.EquateEmpty()); diff != "" {
		t.Errorf("held paths mismatch (-want +got):\n%s", diff)
	}
	for _, path := range plannedPaths(p) {
		if strings.Contains(path, "__pycache__") || path == ".DS_Store" {
			t.Errorf("generated output reached a commit: %s", path)
		}
	}
	for _, held := range p.Held {
		if held.Pattern == "" || held.Why == "" {
			t.Errorf("held %s without saying why: %+v", held.Part.Path, held)
		}
	}

	result, err := h.Apply(ctx, p, Options{})
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if result.TreeStart != result.TreeEnd {
		t.Errorf("content moved: %s to %s", result.TreeStart, result.TreeEnd)
	}
	// The cache stays exactly where it was: still there, still untracked.
	status := h.git("status", "--porcelain=v1", "--untracked-files=all")
	if !strings.Contains(status, "?? "+pycache) {
		t.Errorf("the held path is not untracked in the working tree:\n%s", status)
	}
	if recorded := h.git("log", "--name-only", "--format="); strings.Contains(recorded, "__pycache__") {
		t.Errorf("a commit recorded the bytecode cache:\n%s", recorded)
	}
}

// TestGeneratedOverrides checks the ways a caller overrules the never-commit
// patterns, since a repository that vendors one of these paths on purpose has
// to be able to say so.
func TestGeneratedOverrides(t *testing.T) {
	t.Parallel()

	tests := []struct {
		Name          string
		Options       Options
		WantHeld      int
		WantCommitted bool
	}{{ // Test 0: Left alone, the defaults hold the cache back.
		Name:          "defaults",
		Options:       Options{},
		WantHeld:      3,
		WantCommitted: false,
	}, { // Test 1: Consent commits it like any other change.
		Name:          "allow generated",
		Options:       Options{AllowGenerated: true},
		WantHeld:      0,
		WantCommitted: true,
	}, { // Test 2: One exempted path is committed while the rest are held.
		Name:          "allow one path",
		Options:       Options{GeneratedAllow: []string{"internal/roundhouse/plugins/__pycache__/"}},
		WantHeld:      1,
		WantCommitted: true,
	}}

	for testNum, test := range tests {
		t.Run(fmt.Sprintf("test %d %s", testNum, test.Name), func(t *testing.T) {
			t.Parallel()
			ctx := context.Background()
			h := newHarness(t)
			h.dirtyWithGenerated()

			p, err := h.Plan(ctx, test.Options)
			if err != nil {
				t.Fatalf("Plan: %v", err)
			}
			if len(p.Held) != test.WantHeld {
				t.Errorf("held %d paths, want %d: %v", len(p.Held), test.WantHeld, heldPaths(p))
			}
			var committed bool
			for _, path := range plannedPaths(p) {
				committed = committed || strings.Contains(path, "__pycache__")
			}
			if committed != test.WantCommitted {
				t.Errorf("bytecode cache committed = %v, want %v", committed, test.WantCommitted)
			}
			result, err := h.Apply(ctx, p, test.Options)
			if err != nil {
				t.Fatalf("Apply: %v", err)
			}
			if result.TreeStart != result.TreeEnd {
				t.Errorf("content moved: %s to %s", result.TreeStart, result.TreeEnd)
			}
		})
	}
}

// TestTrackedGeneratedPathsAreStillCommitted checks that a path git already
// tracks is never held back.
//
// A tracked path was committed deliberately at some point, and a tool that
// refused to commit changes to it would break the one repository in a hundred
// that keeps such a directory on purpose.
func TestTrackedGeneratedPathsAreStillCommitted(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	h := newHarness(t)
	h.write("vendor/node_modules/react/index.js", "module.exports = 1;\n")
	h.git("add", "-A")
	h.git("commit", "-m", "Vendor the dependency")
	h.write("vendor/node_modules/react/index.js", "module.exports = 2;\n")

	p, err := h.Plan(ctx, Options{})
	if err != nil {
		t.Fatalf("Plan: %v", err)
	}
	if len(p.Held) != 0 {
		t.Errorf("held a tracked path: %v", heldPaths(p))
	}
	if len(plannedPaths(p)) != 1 {
		t.Errorf("planned %v, want the tracked change committed", plannedPaths(p))
	}
}

// TestNothingButGeneratedOutputStops checks that a tree holding only build
// droppings reports that rather than planning a commit for them.
func TestNothingButGeneratedOutputStops(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	h := newHarness(t)
	h.write(pycache, "\x00\x01compiled\n")

	_, err := h.Plan(ctx, Options{})
	if !errors.Is(err, ErrAllGenerated) {
		t.Fatalf("Plan = %v, want ErrAllGenerated", err)
	}
	if !strings.Contains(err.Error(), pycache) {
		t.Errorf("error does not name the path: %v", err)
	}
}
