package repo

import (
	"context"
	"fmt"
	"strings"
)

// Kind classifies what happened to a path in the working tree.
type Kind string

// The change kinds preen distinguishes when grouping.
const (
	// KindAdded is a path added to the index that HEAD does not have.
	KindAdded Kind = "added"
	// KindModified is a tracked path whose content changed.
	KindModified Kind = "modified"
	// KindDeleted is a tracked path that was removed.
	KindDeleted Kind = "deleted"
	// KindRenamed is a tracked path that moved, keeping its content.
	KindRenamed Kind = "renamed"
	// KindCopied is a path git recorded as a copy of another.
	KindCopied Kind = "copied"
	// KindTypeChanged is a path whose mode changed, like a file becoming a link.
	KindTypeChanged Kind = "typechanged"
	// KindUntracked is a path git is not tracking yet.
	KindUntracked Kind = "untracked"
)

// Change is one path's state in the working tree, merging the index and
// worktree sides of a porcelain entry into a single record.
type Change struct {
	// Path is the current path, relative to the repository root.
	Path string
	// From is the previous path for a rename or copy, empty otherwise.
	From string
	// Kind is what happened to the path.
	Kind Kind
	// Staged reports whether the index side of the entry carries a change,
	// which is a hint that the user drew a commit boundary by hand.
	Staged bool
	// Unstaged reports whether the worktree side carries a change.
	Unstaged bool
}

// IsRenamePair reports whether the change moves content between paths, which
// must stay together in one commit.
func (c Change) IsRenamePair() bool {
	return c.Kind == KindRenamed || c.Kind == KindCopied
}

// Status returns every changed path in the working tree, including untracked
// files. It reads the NUL-delimited porcelain format, so paths with spaces,
// quotes, or newlines survive intact.
func (r *Repo) Status(ctx context.Context) ([]Change, error) {
	out, err := r.raw(ctx, "status", "--porcelain=v1", "-z", "--untracked-files=all")
	if err != nil {
		return nil, err
	}
	return parseStatusZ(string(out))
}

// parseStatusZ parses NUL-delimited porcelain v1 output. Each entry is a
// two-letter code, a space, and the path; a rename or copy is followed by one
// more field holding the original path.
func parseStatusZ(out string) ([]Change, error) {
	fields := strings.Split(out, "\x00")
	changes := make([]Change, 0, len(fields))
	for i := 0; i < len(fields); i++ {
		entry := fields[i]
		if entry == "" {
			continue
		}
		if len(entry) < 4 {
			return nil, fmt.Errorf("%w: short status entry %q", ErrParse, entry)
		}
		index, worktree := entry[0], entry[1]
		change := Change{
			Path:     entry[3:],
			Kind:     classify(index, worktree),
			Staged:   index != ' ' && index != '?',
			Unstaged: worktree != ' ' && worktree != '?',
		}
		// A rename or copy consumes the following field as the original path.
		if index == 'R' || index == 'C' || worktree == 'R' || worktree == 'C' {
			if i+1 >= len(fields) {
				return nil, fmt.Errorf("%w: rename entry %q has no source", ErrParse, entry)
			}
			i++
			change.From = fields[i]
		}
		changes = append(changes, change)
	}
	return changes, nil
}

// classify maps the index and worktree status letters onto a Kind, preferring
// whichever side recorded a structural change over a plain modification.
func classify(index, worktree byte) Kind {
	if index == '?' && worktree == '?' {
		return KindUntracked
	}
	for _, code := range []byte{index, worktree} {
		switch code {
		case 'R':
			return KindRenamed
		case 'C':
			return KindCopied
		case 'A':
			return KindAdded
		case 'D':
			return KindDeleted
		case 'T':
			return KindTypeChanged
		}
	}
	return KindModified
}

// ChangedPaths returns the changes between two revisions, in the same shape as
// a working tree status. An absorb run uses it to learn what the commits it is
// about to redo actually touched.
func (r *Repo) ChangedPaths(ctx context.Context, from, to string) ([]Change, error) {
	out, err := r.raw(ctx, "-c", "core.quotePath=false", "diff", "--name-status", "-z", "--no-renames", from, to)
	if err != nil {
		return nil, err
	}
	fields := strings.Split(string(out), "\x00")
	changes := make([]Change, 0, len(fields)/2)
	for i := 0; i+1 < len(fields); i += 2 {
		code, path := fields[i], fields[i+1]
		if code == "" || path == "" {
			continue
		}
		changes = append(changes, Change{
			Path:     path,
			Kind:     classify(code[0], ' '),
			Unstaged: true,
		})
	}
	return changes, nil
}

// Paths returns the path of every change, which is the shape most callers want
// for pathspec arguments.
func Paths(changes []Change) []string {
	paths := make([]string, 0, len(changes))
	for _, c := range changes {
		paths = append(paths, c.Path)
	}
	return paths
}
