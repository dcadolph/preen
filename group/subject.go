package group

import (
	"fmt"
	"path"
	"slices"
	"strings"

	"github.com/dcadolph/preen/v2/repo"
)

// maxNamedFiles is how many files a subject names outright before it gives up
// and counts them instead. Past three the list is longer than the cap allows
// and reads as noise rather than information.
const maxNamedFiles = 3

// subjectFor writes an imperative subject for a bucket.
//
// Every subject is built from the file list and the kinds of change in it, both
// of which git states outright. Nothing here reads a diff or guesses at intent,
// so a subject preen writes is one the commit can be checked against.
func (h Heuristic) subjectFor(b bucket) string {
	if fixed := b.cat.title(); fixed != "" {
		return fixed
	}
	verb := verbFor(b.changes)
	switch b.shape {
	case shapeMove:
		return h.moveSubject(b, verb)
	case shapeTests:
		return h.testSubject(b, verb)
	default:
		return h.filesSubject(b, verb)
	}
}

// moveSubject names a file that changed directories, which is the one thing an
// add and a delete together say plainly.
func (h Heuristic) moveSubject(b bucket, verb string) string {
	var from, to string
	for _, change := range b.changes {
		if change.Kind == repo.KindDeleted {
			from = change.Path
			continue
		}
		to = change.Path
	}
	if from == "" || to == "" {
		return h.filesSubject(b, verb)
	}
	name := path.Base(to)
	return h.fit(
		fmt.Sprintf("Move %s from %s to %s", name, place(dirOf(from)), place(dirOf(to))),
		fmt.Sprintf("Move %s to %s", name, place(dirOf(to))),
		"Move "+name,
	)
}

// testSubject names a commit holding tests for code this run did not otherwise
// touch, which is what the tests are for and all that can honestly be said.
func (h Heuristic) testSubject(b bucket, verb string) string {
	sources := unitSources(b.changes)
	if len(sources) == 1 {
		stem := strings.TrimSuffix(path.Base(sources[0]), path.Ext(sources[0]))
		return h.fit(
			fmt.Sprintf("%s tests for %s", verb, sources[0]),
			fmt.Sprintf("%s tests for %s", verb, stem),
		)
	}
	if b.dir == "" {
		return verb + " tests"
	}
	where := path.Base(b.dir)
	return h.fit(
		fmt.Sprintf("%s tests for the %s package", verb, where),
		fmt.Sprintf("%s tests for %s", verb, where),
	)
}

// filesSubject names a commit after the files it holds, preferring the most
// specific form that fits the subject cap.
func (h Heuristic) filesSubject(b bucket, verb string) string {
	paths := make([]string, 0, len(b.changes))
	for _, change := range b.changes {
		paths = append(paths, change.Path)
	}
	var candidates []string
	switch sources := unitSources(b.changes); {
	case len(paths) == 1:
		candidates = append(candidates,
			fmt.Sprintf("%s %s", verb, paths[0]),
			fmt.Sprintf("%s %s", verb, path.Base(paths[0])))
	case len(sources) == 1:
		// One source and the tests that go with it, which is a single piece of
		// work however many files it took.
		with := testWord(len(paths) - 1)
		candidates = append(candidates,
			fmt.Sprintf("%s %s and its %s", verb, sources[0], with),
			fmt.Sprintf("%s %s and its %s", verb, path.Base(sources[0]), with))
	}
	if names := baseNames(paths); len(names) <= maxNamedFiles {
		if b.dir != "" {
			candidates = append(candidates,
				fmt.Sprintf("%s %s in %s", verb, andList(names), path.Base(b.dir)))
		}
		candidates = append(candidates, fmt.Sprintf("%s %s", verb, andList(names)))
	}
	if b.dir != "" {
		candidates = append(candidates,
			fmt.Sprintf("%s %d files in %s", verb, len(paths), path.Base(b.dir)))
	}
	return h.fit(append(candidates, b.fallback(verb))...)
}

// fallback is the least specific subject a bucket can carry, used when nothing
// more specific fits the cap. It names the place rather than the files, which
// is what a subject can always say.
func (b bucket) fallback(verb string) string {
	switch b.cat {
	case catDocs:
		return verb + " documentation"
	case catConfig:
		return verb + " configuration"
	case catStaged:
		return "Commit staged work"
	default:
		if b.dir != "" {
			return fmt.Sprintf("%s %s", verb, path.Base(b.dir))
		}
		return fmt.Sprintf("%s %d files", verb, len(b.changes))
	}
}

// fit returns the first candidate within the subject cap, or the last one when
// none fits.
//
// Candidates run from most specific to least, so a subject that will not fit
// gives up detail rather than being cut off mid word by the style layer.
func (h Heuristic) fit(candidates ...string) string {
	limit := h.subjectCap()
	for _, candidate := range candidates {
		if len([]rune(candidate)) <= limit {
			return candidate
		}
	}
	return candidates[len(candidates)-1]
}

// unitSources returns the distinct source files a bucket's changes belong to,
// with each test counted against the file it exercises.
func unitSources(changes []repo.Change) []string {
	seen := make(map[string]bool, len(changes))
	sources := make([]string, 0, len(changes))
	for _, change := range changes {
		source := change.Path
		if named, ok := testSource(change.Path); ok {
			source = named
		}
		if seen[source] {
			continue
		}
		seen[source] = true
		sources = append(sources, source)
	}
	slices.Sort(sources)
	return sources
}

// baseNames returns the file names of a path list, in path order.
func baseNames(paths []string) []string {
	names := make([]string, 0, len(paths))
	for _, p := range paths {
		names = append(names, path.Base(p))
	}
	return names
}

// andList renders names as an English list.
func andList(names []string) string {
	switch len(names) {
	case 0:
		return ""
	case 1:
		return names[0]
	case 2:
		return names[0] + " and " + names[1]
	default:
		return strings.Join(names[:len(names)-1], ", ") + ", and " + names[len(names)-1]
	}
}

// testWord returns the singular or plural of test for a count.
func testWord(n int) string {
	if n == 1 {
		return "test"
	}
	return "tests"
}

// place names a directory for a subject, calling the repository root by name
// rather than by an empty string.
func place(dir string) string {
	if dir == "" {
		return "the repository root"
	}
	return dir
}
