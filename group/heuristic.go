package group

import (
	"context"
	"fmt"
	"path"
	"slices"
	"sort"
	"strings"

	"github.com/dcadolph/preen/plan"
	"github.com/dcadolph/preen/repo"
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
	case catStaged:
		return "Commit staged work"
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
type Heuristic struct {
	// RespectStaged treats already-staged paths as a boundary the user drew by
	// hand and keeps them in their own commit.
	RespectStaged bool
}

// NewHeuristic returns a Heuristic with the default behavior, which honors a
// hand-staged boundary.
func NewHeuristic() Heuristic {
	return Heuristic{RespectStaged: true}
}

// Group clusters changes by category and, for source, by directory.
func (h Heuristic) Group(_ context.Context, in Input) ([]plan.Commit, error) {
	buckets := make(map[string][]repo.Change)
	order := make([]string, 0, len(in.Changes))
	for _, change := range in.Changes {
		key := h.bucket(change)
		if _, seen := buckets[key]; !seen {
			order = append(order, key)
		}
		buckets[key] = append(buckets[key], change)
	}
	sort.SliceStable(order, func(i, j int) bool {
		ci, cj := categoryOf(order[i]), categoryOf(order[j])
		if ci != cj {
			return ci < cj
		}
		return order[i] < order[j]
	})

	commits := make([]plan.Commit, 0, len(order))
	for _, key := range order {
		changes := buckets[key]
		sort.Slice(changes, func(i, j int) bool { return changes[i].Path < changes[j].Path })
		parts := make([]plan.Part, 0, len(changes))
		for _, change := range changes {
			parts = append(parts, plan.Part{Path: change.Path, From: change.From, Kind: change.Kind})
		}
		commits = append(commits, plan.Commit{Subject: subjectFor(key, changes), Parts: parts})
	}
	return commits, nil
}

// bucket returns the grouping key for a change. The key encodes the category
// and, for source, the directory, so ordering can be derived from it later.
func (h Heuristic) bucket(change repo.Change) string {
	if h.RespectStaged && change.Staged && !change.Unstaged {
		return fmt.Sprintf("%d:", catStaged)
	}
	cat := classify(change.Path)
	if cat != catSource {
		return fmt.Sprintf("%d:", cat)
	}
	dir := path.Dir(change.Path)
	if dir == "." {
		dir = ""
	}
	return fmt.Sprintf("%d:%s", catSource, dir)
}

// categoryOf recovers the category from a bucket key.
func categoryOf(key string) category {
	var cat int
	_, _ = fmt.Sscanf(key, "%d:", &cat)
	return category(cat)
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

// subjectFor writes an imperative subject for a bucket, naming what the group
// touches rather than restating the file list.
func subjectFor(key string, changes []repo.Change) string {
	cat := categoryOf(key)
	if fixed := cat.title(); fixed != "" {
		return fixed
	}
	verb := verbFor(changes)
	switch cat {
	case catDocs:
		if len(changes) == 1 {
			return fmt.Sprintf("%s %s", verb, path.Base(changes[0].Path))
		}
		return verb + " documentation"
	case catConfig:
		if len(changes) == 1 {
			return fmt.Sprintf("%s %s", verb, path.Base(changes[0].Path))
		}
		return verb + " configuration"
	default:
		return fmt.Sprintf("%s %s", verb, scopeName(key, changes))
	}
}

// scopeName names what a source bucket covers: its directory when it has one,
// otherwise the single file it holds or the count of files it does not.
func scopeName(key string, changes []repo.Change) string {
	dir := strings.TrimPrefix(key, fmt.Sprintf("%d:", catSource))
	if dir != "" {
		return path.Base(dir)
	}
	if len(changes) == 1 {
		return path.Base(changes[0].Path)
	}
	return fmt.Sprintf("%d files", len(changes))
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
