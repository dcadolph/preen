package group

import (
	"context"
	"fmt"
	"path"
	"slices"
	"strings"

	"github.com/dcadolph/preen/plan"
	"github.com/dcadolph/preen/repo"
	"github.com/dcadolph/preen/style"
)

// category is a class of change that belongs in its own commit.
type category int

// The categories, in the order their commits are recorded. Dependencies land
// before the code that needs them and documentation lands last, so the history
// reads in the direction the work actually depends.
const (
	catDeps category = iota
	catStaged
	catSource
	catConfig
	catCI
	catDocs
)

// title is the fixed commit subject for categories that do not derive one from
// the paths they hold.
func (c category) title() string {
	switch c {
	case catDeps:
		return "Update dependencies"
	case catCI:
		return "Update CI configuration"
	default:
		return ""
	}
}

// lockFiles are dependency manifests and lock files, which are noise inside a
// feature commit and belong together in their own.
//
//nolint:gochecknoglobals // Immutable lookup.
var lockFiles = []string{
	"go.mod", "go.sum", "package-lock.json", "yarn.lock", "pnpm-lock.yaml",
	"Cargo.lock", "Gemfile.lock", "poetry.lock", "requirements.txt", "vendor/modules.txt",
}

// Heuristic groups changes without a model, using path conventions alone.
//
// It never splits a file across commits: a deterministic grouper cannot know
// whether two hunks in one file are one idea or two, and guessing wrong is
// worse than a slightly coarse commit. Splitting is left to a model-backed
// grouper or an explicit edit.
//
// Within a directory it works in units rather than files. A unit is a source
// file plus the tests that exercise it, which is the one pairing a file name
// states outright. Units that only add code are separated from units that
// change code already there, and tests written for code this run did not touch
// stand on their own. Nothing here reads a diff, so the shape of the plan
// follows from the file list and is the same every time.
type Heuristic struct {
	// RespectStaged treats already-staged paths as a boundary the user drew by
	// hand and keeps them in their own commit.
	RespectStaged bool
	// MaxSubject is the subject length generated subjects aim to fit. Zero means
	// the default the style layer applies.
	MaxSubject int
}

// NewHeuristic returns a Heuristic with the default behavior, which honors a
// hand-staged boundary.
func NewHeuristic() Heuristic {
	return Heuristic{RespectStaged: true}
}

// subjectCap returns the length a generated subject aims to fit.
func (h Heuristic) subjectCap() int {
	if h.MaxSubject > 0 {
		return h.MaxSubject
	}
	return style.DefaultMaxSubject
}

// shape is how a bucket's commit is named, which follows from why its changes
// were put together.
type shape int

// The bucket shapes.
const (
	// shapePlain names a commit after the files it holds.
	shapePlain shape = iota
	// shapeAdd holds units that only add code, which is separable work from
	// changing code that is already there.
	shapeAdd
	// shapeTests holds tests for code this run did not otherwise touch.
	shapeTests
	// shapeMove holds both halves of a file that moved between directories.
	shapeMove
)

// rank is where a shape falls in the recording order within one directory: a
// file that moved arrives first, then new code, then changes to code already
// there, then tests for code this run did not otherwise touch. Each step
// depends on the one before it, so nothing lands before what it needs.
func (s shape) rank() int {
	switch s {
	case shapeMove:
		return 0
	case shapeAdd:
		return 1
	case shapeTests:
		return 3
	default:
		return 2
	}
}

// bucket is one planned commit under construction: the changes it holds plus
// the facts its subject is written from.
type bucket struct {
	// key identifies the bucket while changes are being collected.
	key string
	// cat is the category, which sets where the commit lands in the order.
	cat category
	// dir is the directory the bucket covers, empty when its changes are not
	// tied to one place.
	dir string
	// shape is how the commit is named.
	shape shape
	// changes are the changes the commit will carry.
	changes []repo.Change
}

// Group clusters changes into commits by category, then by directory, then by
// the units within a directory.
func (h Heuristic) Group(_ context.Context, in Input) ([]plan.Commit, error) {
	moves := movePairs(in.Changes)
	buckets := make(map[string]*bucket, len(in.Changes))
	order := make([]*bucket, 0, len(in.Changes))
	for _, unit := range h.units(in.Changes, moves) {
		key := unit.key
		existing, seen := buckets[key]
		if !seen {
			existing = &bucket{key: key, cat: unit.cat, dir: unit.dir, shape: unit.shape}
			buckets[key] = existing
			order = append(order, existing)
		}
		existing.changes = append(existing.changes, unit.changes...)
	}
	slices.SortStableFunc(order, byOrder)

	commits := make([]plan.Commit, 0, len(order))
	for _, b := range order {
		slices.SortFunc(b.changes, func(a, c repo.Change) int { return strings.Compare(a.Path, c.Path) })
		parts := make([]plan.Part, 0, len(b.changes))
		for _, change := range b.changes {
			parts = append(parts, plan.Part{Path: change.Path, From: change.From, Kind: change.Kind})
		}
		commits = append(commits, plan.Commit{Subject: h.subjectFor(*b), Parts: parts})
	}
	return commits, nil
}

// byOrder sorts buckets into the order their commits are recorded: category
// first, then directory, then new code before changed code before tests, so
// what a later commit depends on has already landed.
func byOrder(a, b *bucket) int {
	if a.cat != b.cat {
		return int(a.cat) - int(b.cat)
	}
	if a.dir != b.dir {
		return strings.Compare(a.dir, b.dir)
	}
	if a.shape.rank() != b.shape.rank() {
		return a.shape.rank() - b.shape.rank()
	}
	return strings.Compare(a.key, b.key)
}

// unit is a source file and the tests that exercise it, collected with the
// bucket it belongs to. It is the smallest set of paths a file name proves
// belong in one commit.
type unit struct {
	// key is the bucket the unit joins.
	key string
	// cat is the bucket's category.
	cat category
	// dir is the bucket's directory.
	dir string
	// shape is how the bucket's commit is named.
	shape shape
	// changes are the unit's changes.
	changes []repo.Change
}

// units collects the changes into units and decides which bucket each joins.
//
// The unit is decided before the bucket so a test never parts company with the
// source it names, whatever kinds of change the two carry.
func (h Heuristic) units(changes []repo.Change, moves map[string]movePair) []unit {
	byKey := make(map[string][]repo.Change, len(changes))
	order := make([]string, 0, len(changes))
	for _, change := range changes {
		key := h.unitKey(change, moves)
		if _, seen := byKey[key]; !seen {
			order = append(order, key)
		}
		byKey[key] = append(byKey[key], change)
	}
	units := make([]unit, 0, len(order))
	for _, key := range order {
		units = append(units, h.assign(key, byKey[key], moves))
	}
	return units
}

// unitKey names the unit a change belongs to: the move it is half of, the
// staged boundary it sits behind, the source file it tests, or itself.
func (h Heuristic) unitKey(change repo.Change, moves map[string]movePair) string {
	if pair, ok := moves[change.Path]; ok {
		return pair.Key
	}
	if h.RespectStaged && change.Staged && !change.Unstaged {
		return "staged:" + change.Path
	}
	if source, ok := testSource(change.Path); ok {
		return source
	}
	return change.Path
}

// assign works out which bucket a unit joins and how that bucket is named.
func (h Heuristic) assign(key string, changes []repo.Change, moves map[string]movePair) unit {
	if pair, ok := moves[changes[0].Path]; ok {
		return unit{
			key: key, cat: classify(pair.To), dir: dirOf(pair.To),
			shape: shapeMove, changes: changes,
		}
	}
	if strings.HasPrefix(key, "staged:") {
		return unit{key: "staged", cat: catStaged, shape: shapePlain, changes: changes}
	}
	cat := classify(changes[0].Path)
	if cat != catSource {
		return unit{key: fmt.Sprintf("cat:%d", cat), cat: cat, shape: shapePlain, changes: changes}
	}
	dir := dirOf(changes[0].Path)
	shape := shapeFor(changes)
	return unit{
		key: fmt.Sprintf("%s|%d", dir, shape), cat: catSource,
		dir: dir, shape: shape, changes: changes,
	}
}

// shapeFor decides what kind of work a unit is, from what happened to its files.
//
// A unit whose files are all tests is work on the tests alone, since a unit
// takes its name from the source file and that file did not change. A unit that
// only adds files is new code, which stands apart from a change to code that
// was already there. Anything else is a plain change.
func shapeFor(changes []repo.Change) shape {
	tests, added := true, true
	for _, change := range changes {
		if _, ok := testSource(change.Path); !ok {
			tests = false
		}
		if change.Kind != repo.KindAdded && change.Kind != repo.KindUntracked {
			added = false
		}
	}
	switch {
	case tests:
		return shapeTests
	case added:
		return shapeAdd
	default:
		return shapePlain
	}
}

// movePair is a file that moved between directories without git recording a
// rename, seen as one addition and one deletion of the same file name.
type movePair struct {
	// Key is the bucket both halves join.
	Key string
	// From is the path the file left.
	From string
	// To is the path it arrived at.
	To string
}

// movePairs finds the moves hiding in a change set.
//
// Both halves of a move belong in one commit: split apart they read as a
// deletion of one file and an unrelated addition of another, and a bisect
// landing between them gets a tree with the file missing. The match is only
// made when exactly one addition and one deletion claim a file name, so an
// ambiguous name is left alone rather than paired on a guess.
func movePairs(changes []repo.Change) map[string]movePair {
	added, deleted := make(map[string][]string), make(map[string][]string)
	for _, change := range changes {
		base := path.Base(change.Path)
		switch change.Kind {
		case repo.KindAdded, repo.KindUntracked:
			added[base] = append(added[base], change.Path)
		case repo.KindDeleted:
			deleted[base] = append(deleted[base], change.Path)
		}
	}
	pairs := make(map[string]movePair, len(added))
	for base, adds := range added {
		dels := deleted[base]
		if len(adds) != 1 || len(dels) != 1 || dirOf(adds[0]) == dirOf(dels[0]) {
			continue
		}
		pair := movePair{Key: "move:" + base, From: dels[0], To: adds[0]}
		pairs[adds[0]] = pair
		pairs[dels[0]] = pair
	}
	return pairs
}

// testSource returns the path of the source file a test exercises, and whether
// the path reads as a test at all.
//
// The answer comes from the file name and nothing else, following the
// conventions that are near universal in each language: a _test or _spec
// suffix, a .test or .spec infix, or a test_ prefix. The source path it derives
// need not exist, and a caller that cares checks it against the change set.
func testSource(p string) (string, bool) {
	dir, base := path.Split(p)
	ext := path.Ext(base)
	stem := strings.TrimSuffix(base, ext)
	switch {
	case strings.HasSuffix(stem, "_test"), strings.HasSuffix(stem, "_spec"),
		strings.HasSuffix(stem, ".test"), strings.HasSuffix(stem, ".spec"):
		stem = stem[:len(stem)-len("_test")]
	case strings.HasPrefix(stem, "test_"), strings.HasPrefix(stem, "spec_"):
		stem = stem[len("test_"):]
	default:
		return "", false
	}
	if stem == "" {
		return "", false
	}
	return dir + stem + ext, true
}

// classify maps a path onto a category by convention.
func classify(p string) category {
	base := path.Base(p)
	if slices.Contains(lockFiles, base) || slices.Contains(lockFiles, p) {
		return catDeps
	}
	if strings.HasPrefix(p, ".github/") || strings.HasPrefix(p, ".circleci/") ||
		base == ".gitlab-ci.yml" || base == "Jenkinsfile" {
		return catCI
	}
	switch strings.ToLower(path.Ext(p)) {
	case ".md", ".rst", ".adoc":
		return catDocs
	case ".yaml", ".yml", ".toml", ".ini", ".cfg":
		return catConfig
	}
	if strings.HasPrefix(p, "docs/") || base == "LICENSE" || base == "CHANGELOG" {
		return catDocs
	}
	if strings.HasPrefix(base, ".") && !strings.Contains(base[1:], ".") {
		// Dotfiles like .gitignore and .editorconfig are configuration.
		return catConfig
	}
	return catSource
}

// dirOf returns a path's directory, with the repository root as an empty
// string rather than a dot.
func dirOf(p string) string {
	dir := path.Dir(p)
	if dir == "." {
		return ""
	}
	return dir
}

// verbFor picks the imperative verb that matches what happened: a group that
// only adds reads as an addition, one that only deletes as a removal, and
// anything mixed as an update.
func verbFor(changes []repo.Change) string {
	var added, deleted, other int
	for _, change := range changes {
		switch change.Kind {
		case repo.KindAdded, repo.KindUntracked:
			added++
		case repo.KindDeleted:
			deleted++
		default:
			other++
		}
	}
	switch {
	case other == 0 && deleted == 0 && added > 0:
		return "Add"
	case other == 0 && added == 0 && deleted > 0:
		return "Remove"
	default:
		return "Update"
	}
}
