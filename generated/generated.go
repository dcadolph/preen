// Package generated recognizes paths that hold generated output rather than
// work: bytecode caches, installed dependencies, coverage reports, and the
// files an editor or an operating system leaves lying around.
//
// A repository whose .gitignore misses one of these has it reported as
// untracked, which git status cannot tell apart from new work. preen holds such
// a path back from the plan and says why, rather than helping commit something
// that regenerates itself on the next test run.
//
// Every pattern is overridable. A repository that vendors one of these
// directories on purpose is a real thing, and the tool has no business
// overruling it.
package generated

import (
	"path"
	"strings"
)

// Rule is one never-commit pattern and what it catches.
type Rule struct {
	// Pattern is the path pattern, in the syntax matches documents.
	Pattern string
	// Why names what the pattern catches, so a report can explain the refusal.
	Why string
}

// Configured is the reason given for a pattern the repository added itself,
// where preen has no description of its own to offer.
const Configured = "matched a configured pattern"

// Defaults are the patterns preen holds back without being asked.
//
// The list is deliberately short and boring: every entry is output some tool
// writes and rewrites, and none of them is a file anyone edits by hand. Broad
// build directories like build/ and dist/ are left out, because plenty of
// projects keep real source under those names.
//
//nolint:gochecknoglobals // Immutable lookup.
var Defaults = []Rule{
	{Pattern: "__pycache__/", Why: "Python bytecode cache"},
	{Pattern: "*.pyc", Why: "compiled Python"},
	{Pattern: "*.pyo", Why: "optimized Python bytecode"},
	{Pattern: ".pytest_cache/", Why: "pytest cache"},
	{Pattern: ".mypy_cache/", Why: "mypy cache"},
	{Pattern: ".ruff_cache/", Why: "ruff cache"},
	{Pattern: "*.egg-info/", Why: "Python packaging metadata"},
	{Pattern: ".venv/", Why: "Python virtual environment"},
	{Pattern: "venv/", Why: "Python virtual environment"},
	{Pattern: "node_modules/", Why: "installed JavaScript dependencies"},
	{Pattern: ".nyc_output/", Why: "JavaScript coverage output"},
	{Pattern: "htmlcov/", Why: "rendered coverage report"},
	{Pattern: "lcov.info", Why: "coverage report"},
	{Pattern: "coverage.out", Why: "Go coverage profile"},
	{Pattern: "coverage.html", Why: "rendered coverage report"},
	{Pattern: "coverage.xml", Why: "coverage report"},
	{Pattern: ".coverage", Why: "Python coverage data"},
	{Pattern: "target/", Why: "build output"},
	{Pattern: ".gradle/", Why: "Gradle cache"},
	{Pattern: "*.class", Why: "compiled Java"},
	{Pattern: ".DS_Store", Why: "macOS directory metadata"},
	{Pattern: "Thumbs.db", Why: "Windows thumbnail cache"},
	{Pattern: "*.swp", Why: "editor swap file"},
	{Pattern: "*.swo", Why: "editor swap file"},
	{Pattern: "*~", Why: "editor backup file"},
	{Pattern: ".idea/", Why: "JetBrains project settings"},
}

// Matcher decides whether a path holds generated output. The zero Matcher
// matches nothing, which is the shape callers want when the check is off.
type Matcher struct {
	// rules are the patterns a path is tested against.
	rules []Rule
	// allow are patterns that override the rules, for paths a repository
	// commits deliberately.
	allow []string
}

// New returns a Matcher over the default patterns plus extra, with anything
// allow covers exempted. An extra pattern uses the same syntax as a default.
func New(extra, allow []string) Matcher {
	rules := make([]Rule, 0, len(Defaults)+len(extra))
	rules = append(rules, Defaults...)
	for _, pattern := range extra {
		if pattern = strings.TrimSpace(pattern); pattern != "" {
			rules = append(rules, Rule{Pattern: pattern, Why: Configured})
		}
	}
	return Matcher{rules: rules, allow: allow}
}

// Off returns a Matcher that holds nothing back, which is what the caller's
// consent to commit generated output amounts to.
func Off() Matcher { return Matcher{} }

// Match returns the rule that catches a path, and whether one did. An allowed
// path never matches, so a repository can vendor a directory the defaults would
// otherwise refuse.
func (m Matcher) Match(p string) (Rule, bool) {
	for _, pattern := range m.allow {
		if matches(pattern, p) {
			return Rule{}, false
		}
	}
	for _, rule := range m.rules {
		if matches(rule.Pattern, p) {
			return rule, true
		}
	}
	return Rule{}, false
}

// matches reports whether one pattern covers a path.
//
// Three forms are supported, chosen so a pattern reads the way it would in a
// .gitignore without dragging in that file's full grammar. A pattern holding a
// slash is a path from the repository root and matches that path and everything
// under it. A bare name with a trailing slash is a directory and matches
// wherever in the tree it appears. Anything else is a glob on the file's own
// name, wherever in the tree it sits.
func matches(pattern, p string) bool {
	pattern = strings.TrimPrefix(strings.TrimSpace(pattern), "./")
	if p == "" {
		return false
	}
	dirOnly := strings.HasSuffix(pattern, "/")
	pattern = strings.TrimSuffix(pattern, "/")
	if pattern == "" {
		return false
	}
	if strings.Contains(pattern, "/") {
		return p == pattern || strings.HasPrefix(p, pattern+"/")
	}
	if dirOnly {
		for _, part := range strings.Split(path.Dir(p), "/") {
			if ok, err := path.Match(pattern, part); err == nil && ok {
				return true
			}
		}
		return false
	}
	ok, err := path.Match(pattern, path.Base(p))
	return err == nil && ok
}
