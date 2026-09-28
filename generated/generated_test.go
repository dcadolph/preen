package generated

import (
	"fmt"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
)

// TestMatchRecognizesGeneratedOutput checks that the default patterns catch the
// paths a tool writes and leave real work alone.
//
// The __pycache__ case is the one that bit: a repository whose .gitignore
// missed it had the directory reported as untracked, and a commit named after a
// bytecode cache was planned as if it were work.
func TestMatchRecognizesGeneratedOutput(t *testing.T) {
	t.Parallel()

	tests := []struct {
		Name        string
		Path        string
		WantPattern string
		WantMatch   bool
	}{{ // Test 0: The bytecode cache that started this, nested deep in a tree.
		Name:        "nested pycache",
		Path:        "internal/roundhouse/plugins/__pycache__/switchtender.cpython-313.pyc",
		WantPattern: "__pycache__/",
		WantMatch:   true,
	}, { // Test 1: A stray .pyc outside a cache directory.
		Name:        "loose bytecode",
		Path:        "plugins/helper.pyc",
		WantPattern: "*.pyc",
		WantMatch:   true,
	}, { // Test 2: Operating system metadata at the repository root.
		Name:        "ds store",
		Path:        ".DS_Store",
		WantPattern: ".DS_Store",
		WantMatch:   true,
	}, { // Test 3: Installed dependencies, however deep the path runs.
		Name:        "node modules",
		Path:        "web/node_modules/react/index.js",
		WantPattern: "node_modules/",
		WantMatch:   true,
	}, { // Test 4: A directory pattern that is itself a glob.
		Name:        "egg info",
		Path:        "src/preen.egg-info/PKG-INFO",
		WantPattern: "*.egg-info/",
		WantMatch:   true,
	}, { // Test 5: Coverage output written by a test run.
		Name:        "coverage profile",
		Path:        "coverage.out",
		WantPattern: "coverage.out",
		WantMatch:   true,
	}, { // Test 6: An editor swap file left open in a directory.
		Name:        "swap file",
		Path:        "cmd/.server.go.swp",
		WantPattern: "*.swp",
		WantMatch:   true,
	}, { // Test 7: Ordinary source is never held back.
		Name:      "source",
		Path:      "internal/roundhouse/plugins/switchtender.py",
		WantMatch: false,
	}, { // Test 8: A file whose name merely contains a pattern is not a match.
		Name:      "lookalike",
		Path:      "docs/node_modules_guide.md",
		WantMatch: false,
	}, { // Test 9: A directory named target only matches as a directory.
		Name:      "file named target",
		Path:      "target",
		WantMatch: false,
	}, { // Test 10: Build output under a target directory does match.
		Name:        "target directory",
		Path:        "rust/target/debug/app",
		WantPattern: "target/",
		WantMatch:   true,
	}}

	matcher := New(nil, nil)
	for testNum, test := range tests {
		t.Run(fmt.Sprintf("test %d %s", testNum, test.Name), func(t *testing.T) {
			t.Parallel()
			rule, matched := matcher.Match(test.Path)
			if matched != test.WantMatch {
				t.Fatalf("Match(%q) = %v, want %v", test.Path, matched, test.WantMatch)
			}
			if diff := cmp.Diff(test.WantPattern, rule.Pattern, cmpopts.EquateEmpty()); diff != "" {
				t.Errorf("pattern mismatch (-want +got):\n%s", diff)
			}
			if matched && rule.Why == "" {
				t.Errorf("Match(%q) gave no reason, want one to report", test.Path)
			}
		})
	}
}

// TestMatcherOverrides checks the ways a repository overrules the defaults,
// since somewhere a project vendors one of these directories on purpose.
func TestMatcherOverrides(t *testing.T) {
	t.Parallel()

	tests := []struct {
		Name      string
		Extra     []string
		Allow     []string
		Path      string
		WantMatch bool
	}{{ // Test 0: An allowed directory is committed like anything else.
		Name:      "allowed directory",
		Allow:     []string{"third_party/node_modules"},
		Path:      "third_party/node_modules/react/index.js",
		WantMatch: false,
	}, { // Test 1: The allowance is scoped to the path it names.
		Name:      "allowance does not leak",
		Allow:     []string{"third_party/node_modules"},
		Path:      "web/node_modules/react/index.js",
		WantMatch: true,
	}, { // Test 2: A repository can add a pattern of its own.
		Name:      "extra pattern",
		Extra:     []string{"*.snap"},
		Path:      "ui/button.snap",
		WantMatch: true,
	}, { // Test 3: An allowed glob exempts a file name anywhere.
		Name:      "allowed glob",
		Allow:     []string{".DS_Store"},
		Path:      "assets/.DS_Store",
		WantMatch: false,
	}, { // Test 4: Blank patterns are ignored rather than matching everything.
		Name:      "blank pattern",
		Extra:     []string{"  "},
		Path:      "cmd/server.go",
		WantMatch: false,
	}, { // Test 5: A rooted directory pattern exempts that directory alone.
		Name:      "rooted directory",
		Allow:     []string{"internal/plugins/__pycache__/"},
		Path:      "internal/plugins/__pycache__/runner.cpython-313.pyc",
		WantMatch: false,
	}, { // Test 6: The same rooted pattern leaves another copy held back.
		Name:      "rooted directory elsewhere",
		Allow:     []string{"internal/plugins/__pycache__/"},
		Path:      "tools/__pycache__/runner.cpython-313.pyc",
		WantMatch: true,
	}}

	for testNum, test := range tests {
		t.Run(fmt.Sprintf("test %d %s", testNum, test.Name), func(t *testing.T) {
			t.Parallel()
			_, matched := New(test.Extra, test.Allow).Match(test.Path)
			if matched != test.WantMatch {
				t.Errorf("Match(%q) = %v, want %v", test.Path, matched, test.WantMatch)
			}
		})
	}
}

// TestOffMatchesNothing checks that consent to commit generated output turns
// the whole check off rather than narrowing it.
func TestOffMatchesNothing(t *testing.T) {
	t.Parallel()

	paths := []string{
		"internal/plugins/__pycache__/runner.cpython-313.pyc",
		".DS_Store",
		"web/node_modules/react/index.js",
	}
	for testNum, path := range paths {
		t.Run(fmt.Sprintf("test %d %s", testNum, path), func(t *testing.T) {
			t.Parallel()
			if _, matched := Off().Match(path); matched {
				t.Errorf("Off().Match(%q) held a path back, want nothing held", path)
			}
		})
	}
}
