package plan

import (
	"fmt"
	"slices"
	"strings"
)

// Commit numbers in every edit are the 1-based positions shown in the
// rendered plan, so what the user types matches what they read.

// MergeInto folds commit number from into commit number into, keeping the
// destination's message and appending the source's parts.
func (p *Plan) MergeInto(from, into int) error {
	src, err := p.index(from)
	if err != nil {
		return err
	}
	dst, err := p.index(into)
	if err != nil {
		return err
	}
	if src == dst {
		return fmt.Errorf("%w: cannot merge commit %d into itself", ErrInvalid, from)
	}
	p.Commits[dst].Parts = append(p.Commits[dst].Parts, p.Commits[src].Parts...)
	p.Commits = slices.Delete(p.Commits, src, src+1)
	return nil
}

// SplitByFile breaks a commit into one commit per file it touches, keeping the
// original subject with the path appended so each is still named.
func (p *Plan) SplitByFile(number int) error {
	at, err := p.index(number)
	if err != nil {
		return err
	}
	commit := p.Commits[at]
	if len(commit.Parts) < 2 {
		return fmt.Errorf("%w: commit %d holds one file and cannot be split by file", ErrInvalid, number)
	}
	split := make([]Commit, 0, len(commit.Parts))
	for _, part := range commit.Parts {
		split = append(split, Commit{
			Subject: fmt.Sprintf("%s (%s)", commit.Subject, part.Path),
			Body:    commit.Body,
			Parts:   []Part{part},
		})
	}
	p.Commits = slices.Concat(p.Commits[:at], split, p.Commits[at+1:])
	return nil
}

// MovePath reassigns every part for a path to another commit.
func (p *Plan) MovePath(path string, to int) error {
	dst, err := p.index(to)
	if err != nil {
		return err
	}
	var moved []Part
	for i := range p.Commits {
		if i == dst {
			continue
		}
		kept := p.Commits[i].Parts[:0]
		for _, part := range p.Commits[i].Parts {
			if part.Path == path {
				moved = append(moved, part)
				continue
			}
			kept = append(kept, part)
		}
		p.Commits[i].Parts = kept
	}
	if len(moved) == 0 {
		return fmt.Errorf("%w: %s", ErrNoSuchPath, path)
	}
	p.Commits[dst].Parts = append(p.Commits[dst].Parts, moved...)
	p.dropEmpty()
	return nil
}

// Reword replaces a commit's subject, leaving its body and parts alone.
func (p *Plan) Reword(number int, subject string) error {
	at, err := p.index(number)
	if err != nil {
		return err
	}
	if strings.TrimSpace(subject) == "" {
		return fmt.Errorf("%w: an empty subject is not a message", ErrInvalid)
	}
	p.Commits[at].Subject = subject
	return nil
}

// DropPath removes a path from every commit and records it as deliberately
// left uncommitted, which keeps the plan accounting for the whole tree.
func (p *Plan) DropPath(path string) error {
	var dropped []Part
	for i := range p.Commits {
		kept := p.Commits[i].Parts[:0]
		for _, part := range p.Commits[i].Parts {
			if part.Path == path {
				dropped = append(dropped, part)
				continue
			}
			kept = append(kept, part)
		}
		p.Commits[i].Parts = kept
	}
	if len(dropped) == 0 {
		return fmt.Errorf("%w: %s", ErrNoSuchPath, path)
	}
	// A path split across commits becomes one whole-file leftover, since
	// nothing will be staged for it at all.
	p.Leftover = append(p.Leftover, Part{Path: path, From: dropped[0].From, Kind: dropped[0].Kind})
	p.dropEmpty()
	return nil
}

// Reorder rearranges the commits into the given 1-based order, which must be a
// permutation of the existing numbers.
func (p *Plan) Reorder(order []int) error {
	if len(order) != len(p.Commits) {
		return fmt.Errorf("%w: order lists %d commits, plan holds %d", ErrInvalid, len(order), len(p.Commits))
	}
	seen := make(map[int]bool, len(order))
	reordered := make([]Commit, 0, len(order))
	for _, number := range order {
		at, err := p.index(number)
		if err != nil {
			return err
		}
		if seen[at] {
			return fmt.Errorf("%w: commit %d listed twice", ErrInvalid, number)
		}
		seen[at] = true
		reordered = append(reordered, p.Commits[at])
	}
	p.Commits = reordered
	return nil
}

// index converts a 1-based commit number into a slice index.
func (p *Plan) index(number int) (int, error) {
	if number < 1 || number > len(p.Commits) {
		return 0, fmt.Errorf("%w: %d", ErrNoSuchCommit, number)
	}
	return number - 1, nil
}

// dropEmpty removes commits left with no parts by an edit.
func (p *Plan) dropEmpty() {
	kept := p.Commits[:0]
	for _, commit := range p.Commits {
		if len(commit.Parts) > 0 {
			kept = append(kept, commit)
		}
	}
	p.Commits = kept
}
