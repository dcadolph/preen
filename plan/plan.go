// Package plan models what a preen run intends to do before anything moves.
// A plan is data: it can be rendered for approval, edited by the user, and
// validated against the working tree without touching the repository.
package plan

import (
	"fmt"
	"sort"
	"strings"

	"github.com/dcadolph/preen/repo"
)

// Hunk identifies one hunk a commit takes from a file.
//
// The hunk is identified by its body rather than its position, because
// committing an earlier hunk shifts the line numbers of every hunk after it.
// The body is stable across that shift, so it still finds the right hunk in a
// diff regenerated mid-run.
type Hunk struct {
	// Index is the hunk's position when the plan was built, used for display
	// and for reporting a hunk that later goes missing.
	Index int
	// Body is the hunk's lines without its header, the identity used to find
	// the hunk again at apply time.
	Body string
}

// Part is one file's contribution to a commit, either the whole file or a
// chosen subset of its hunks.
type Part struct {
	// Path is the file, relative to the repository root.
	Path string
	// From is the rename source, empty unless the change moves a file.
	From string
	// Kind is what happened to the path.
	Kind repo.Kind
	// Hunks are the file's hunks this commit takes. An empty selection means
	// the whole file.
	Hunks []Hunk
}

// Whole reports whether the part carries the file's entire change.
func (p Part) Whole() bool { return len(p.Hunks) == 0 }

// HunkAt returns a Hunk identifying the hunk at index i of a file diff.
func HunkAt(i int, body string) Hunk { return Hunk{Index: i, Body: body} }

// String renders the part for a plan listing.
func (p Part) String() string {
	switch {
	case p.From != "":
		return fmt.Sprintf("%s (renamed from %s)", p.Path, p.From)
	case p.Whole():
		return p.Path
	default:
		return fmt.Sprintf("%s (%d hunk%s)", p.Path, len(p.Hunks), plural(len(p.Hunks)))
	}
}

// Commit is one planned commit: a message and the parts it will contain.
type Commit struct {
	// Subject is the first line of the message.
	Subject string
	// Body is the optional message body.
	Body string
	// Parts are the file contributions this commit takes, in listing order.
	Parts []Part
}

// Message renders the full commit message.
func (c Commit) Message() string {
	if strings.TrimSpace(c.Body) == "" {
		return c.Subject
	}
	return c.Subject + "\n\n" + c.Body
}

// Paths returns the paths this commit touches.
func (c Commit) Paths() []string {
	paths := make([]string, 0, len(c.Parts))
	for _, p := range c.Parts {
		paths = append(paths, p.Path)
	}
	return paths
}

// Plan is the full intent of a run.
type Plan struct {
	// Base is the commit the run resets to, empty for a working-tree-only run.
	Base string
	// Backup is the recovery ref created before anything moved.
	Backup string
	// MergeSummary is the mandatory merge audit line for any run that resets.
	MergeSummary string
	// Absorbed are the existing commits the run will redo.
	Absorbed []repo.Commit
	// Commits are the planned commits, in the order they will be recorded.
	Commits []Commit
	// Leftover are changes deliberately left uncommitted.
	Leftover []Part
	// Push is the exact force push a published rewrite will run, empty for any
	// run that does not publish.
	Push string
	// Debris are lines that look like leftovers from the session, reported so
	// they can be dealt with before they are committed. They are never removed
	// automatically.
	Debris []string
	// Covers is the change set the plan was built against, kept so an edit can
	// be revalidated without surveying the repository again.
	Covers []repo.Change
}

// Revalidate checks the plan against the change set it was built for, which is
// how an edit is rejected before it can lose work.
func (p Plan) Revalidate() error { return p.Validate(p.Covers) }

// Resets reports whether the plan moves the branch, which is what makes the
// merge audit and the backup ref mandatory.
func (p Plan) Resets() bool { return p.Base != "" }

// Validate checks that the plan is internally coherent and covers exactly the
// supplied set of changes: every change lands somewhere, nothing lands twice,
// and no commit is empty.
//
// This is the check that keeps a grouping mistake from silently dropping work,
// and it runs before the repository is touched.
func (p Plan) Validate(changes []repo.Change) error {
	if len(p.Commits) == 0 {
		return fmt.Errorf("%w: no commits planned", ErrInvalid)
	}
	seen := make(map[string][]int, len(changes))
	for i, commit := range p.Commits {
		if strings.TrimSpace(commit.Subject) == "" {
			return fmt.Errorf("%w: commit %d has no subject", ErrInvalid, i+1)
		}
		if len(commit.Parts) == 0 {
			return fmt.Errorf("%w: commit %d (%q) is empty", ErrInvalid, i+1, commit.Subject)
		}
		for _, part := range commit.Parts {
			if err := recordPart(seen, part); err != nil {
				return err
			}
		}
	}
	for _, part := range p.Leftover {
		if err := recordPart(seen, part); err != nil {
			return err
		}
	}
	return coversAll(seen, changes)
}

// recordPart adds a part to the coverage map, rejecting a file claimed twice
// in whole and the same hunk claimed by two commits.
func recordPart(seen map[string][]int, part Part) error {
	prior, taken := seen[part.Path]
	if part.Whole() {
		if taken {
			return fmt.Errorf("%w: %s is claimed more than once", ErrInvalid, part.Path)
		}
		seen[part.Path] = nil
		return nil
	}
	if taken && prior == nil {
		return fmt.Errorf("%w: %s is claimed both whole and by hunk", ErrInvalid, part.Path)
	}
	for _, hunk := range part.Hunks {
		for _, already := range prior {
			if already == hunk.Index {
				return fmt.Errorf("%w: %s hunk %d is claimed twice", ErrInvalid, part.Path, hunk.Index)
			}
		}
		prior = append(prior, hunk.Index)
	}
	seen[part.Path] = prior
	return nil
}

// coversAll reports paths in the working tree that no commit or leftover
// claimed, which would mean the run silently abandons work.
func coversAll(seen map[string][]int, changes []repo.Change) error {
	var missing []string
	for _, change := range changes {
		if _, ok := seen[change.Path]; !ok {
			missing = append(missing, change.Path)
		}
	}
	if len(missing) == 0 {
		return nil
	}
	sort.Strings(missing)
	return fmt.Errorf("%w: unaccounted changes: %s", ErrInvalid, strings.Join(missing, ", "))
}

// plural returns the plural suffix for a count.
func plural(n int) string {
	if n == 1 {
		return ""
	}
	return "s"
}
