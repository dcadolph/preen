// Package group turns a surveyed working tree into a set of planned commits.
//
// Grouping is the one part of preen that is judgment rather than mechanics, so
// it sits behind an interface. The built-in grouper is deterministic and needs
// no model, which keeps preen useful on its own; a model-backed grouper can be
// substituted where better judgment is worth the cost.
package group

import (
	"context"

	"github.com/dcadolph/preen/plan"
	"github.com/dcadolph/preen/repo"
)

// Input is everything a grouper sees about the working tree.
type Input struct {
	// Changes is every changed path, including untracked files.
	Changes []repo.Change
	// Diffs are the per-file patches, which carry the hunks a grouper may
	// split across commits.
	Diffs []repo.FileDiff
}

// DiffFor returns the patch for a path, and whether one exists. A deletion or
// a binary file may have no usable patch.
func (in Input) DiffFor(path string) (repo.FileDiff, bool) {
	for _, d := range in.Diffs {
		if d.Path == path {
			return d, true
		}
	}
	return repo.FileDiff{}, false
}

// Grouper clusters changes into ordered commits. It returns commits only; the
// caller validates them against the tree and builds the surrounding plan.
type Grouper interface {
	// Group clusters the input into commits, in the order they should be
	// recorded.
	Group(ctx context.Context, in Input) ([]plan.Commit, error)
}

// GrouperFunc adapts a function to the Grouper interface.
type GrouperFunc func(ctx context.Context, in Input) ([]plan.Commit, error)

// Group calls f.
func (f GrouperFunc) Group(ctx context.Context, in Input) ([]plan.Commit, error) {
	return f(ctx, in)
}

// Chain runs groupers in order and returns the first non-empty result, so a
// model-backed grouper can fall back to the deterministic one when it is
// unavailable or returns nothing usable.
func Chain(first Grouper, rest ...Grouper) Grouper {
	return GrouperFunc(func(ctx context.Context, in Input) ([]plan.Commit, error) {
		commits, err := first.Group(ctx, in)
		if err == nil && len(commits) > 0 {
			return commits, nil
		}
		var lastErr = err
		for _, g := range rest {
			commits, err := g.Group(ctx, in)
			if err == nil && len(commits) > 0 {
				return commits, nil
			}
			if err != nil {
				lastErr = err
			}
		}
		return nil, lastErr
	})
}
