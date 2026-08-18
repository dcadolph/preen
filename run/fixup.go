package run

import (
	"context"
	"fmt"
	"sort"

	"github.com/dcadolph/preen/plan"
	"github.com/dcadolph/preen/repo"
)

// FixupPlan is a fixup run's intent: the plan to render and approve, plus the
// routing that says which commit each change folds into.
type FixupPlan struct {
	// Plan is the renderable, validatable plan.
	Plan *plan.Plan
	// Fixups are the routed changes.
	Fixups []Fixup
}

// Fixup is one change routed to the commit that introduced it.
type Fixup struct {
	// Part is the change being folded in.
	Part plan.Part
	// Target is the unpushed commit the change belongs to.
	Target repo.Commit
}

// PlanFixup routes each dirty change to the unpushed commit that last touched
// it, instead of building new commits.
//
// Targeting is per file rather than per line. A file-level answer is one git
// can state exactly, where attributing a hunk to a commit means guessing at
// which of several commits owns a region, and a wrong guess here silently
// rewrites the wrong commit.
func (e *Engine) PlanFixup(ctx context.Context, opts Options) (*FixupPlan, error) {
	if err := e.Repo.CheckReady(ctx); err != nil {
		return nil, err
	}
	base, err := e.Repo.UnpushedBase(ctx)
	if err != nil {
		return nil, fmt.Errorf("cannot tell which commits are unpushed: %w", err)
	}
	candidates, err := e.Repo.Log(ctx, base+"..HEAD")
	if err != nil {
		return nil, err
	}
	if len(candidates) == 0 {
		return nil, fmt.Errorf("%w: no unpushed commits to fold into", ErrNothingToDo)
	}
	if err := e.refusePushed(ctx, candidates); err != nil {
		return nil, err
	}

	changes, err := e.survey(ctx, opts, "")
	if err != nil {
		return nil, err
	}
	if len(changes) == 0 {
		return nil, ErrNothingToDo
	}

	built := &plan.Plan{Base: base, Covers: changes}
	check, err := e.Repo.CheckMerges(ctx, base)
	if err != nil {
		return nil, err
	}
	built.MergeSummary = check.Summary()

	fixups, leftover, err := e.routeChanges(ctx, base, changes)
	if err != nil {
		return nil, err
	}
	built.Leftover = leftover
	built.Commits = fixupCommits(fixups)
	if len(built.Commits) == 0 {
		return nil, fmt.Errorf("%w: nothing matched an unpushed commit", ErrFixupTarget)
	}
	if err := built.Revalidate(); err != nil {
		return nil, err
	}
	return &FixupPlan{Plan: built, Fixups: fixups}, nil
}

// routeChanges finds each change's target commit. A change no unpushed commit
// introduced becomes a leftover rather than being forced somewhere wrong.
func (e *Engine) routeChanges(ctx context.Context, base string, changes []repo.Change) ([]Fixup, []plan.Part, error) {
	var (
		fixups   []Fixup
		leftover []plan.Part
	)
	for _, change := range changes {
		part := plan.Part{Path: change.Path, From: change.From, Kind: change.Kind}
		target, err := e.Repo.LastCommitTouching(ctx, base, change.Path)
		if err != nil {
			return nil, nil, err
		}
		if target.Hash == "" {
			leftover = append(leftover, part)
			continue
		}
		fixups = append(fixups, Fixup{Part: part, Target: target})
	}
	return fixups, leftover, nil
}

// fixupCommits turns routed changes into planned commits, one per target, so
// the plan renders and validates like any other.
func fixupCommits(fixups []Fixup) []plan.Commit {
	byTarget := make(map[string][]plan.Part)
	subjects := make(map[string]string)
	var order []string
	for _, fixup := range fixups {
		if _, seen := byTarget[fixup.Target.Hash]; !seen {
			order = append(order, fixup.Target.Hash)
			subjects[fixup.Target.Hash] = fixup.Target.Subject
		}
		byTarget[fixup.Target.Hash] = append(byTarget[fixup.Target.Hash], fixup.Part)
	}
	sort.Strings(order)
	commits := make([]plan.Commit, 0, len(order))
	for _, hash := range order {
		parts := byTarget[hash]
		sort.Slice(parts, func(i, j int) bool { return parts[i].Path < parts[j].Path })
		commits = append(commits, plan.Commit{
			Subject: fmt.Sprintf("fixup! %s", subjects[hash]),
			Parts:   parts,
		})
	}
	return commits
}

// ApplyFixup stages each routed change, records it as a fixup commit, and then
// squashes the lot with an autosquash rebase.
//
// The same conservation guarantee applies: the content going in must equal the
// content coming out, and a rebase that conflicts is aborted and rolled back
// rather than left half done.
func (e *Engine) ApplyFixup(ctx context.Context, fp *FixupPlan, opts Options) (*Result, error) {
	if err := e.Repo.CheckReady(ctx); err != nil {
		return nil, err
	}
	treeStart, err := e.Repo.ContentTree(ctx)
	if err != nil {
		return nil, err
	}
	backup, err := e.Repo.CreateBackup(ctx, e.Now())
	if err != nil {
		return nil, err
	}
	fp.Plan.Backup = backup
	result := &Result{Backup: backup, TreeStart: treeStart}

	if err := e.recordFixups(ctx, fp, opts, result); err != nil {
		return result, e.rollback(ctx, backup, err)
	}
	if err := e.Repo.AutosquashOnto(ctx, fp.Plan.Base); err != nil {
		// A conflict leaves a rebase in progress, which must be abandoned
		// before the branch can be moved back.
		_ = e.Repo.AbortRebase(ctx)
		return result, e.rollback(ctx, backup, fmt.Errorf("autosquash failed: %w", err))
	}

	treeEnd, err := e.Repo.ContentTree(ctx)
	if err != nil {
		return result, e.rollback(ctx, backup, err)
	}
	result.TreeEnd = treeEnd
	if err := e.verify(ctx, result, opts); err != nil {
		return result, e.rollback(ctx, backup, err)
	}
	return result, nil
}

// recordFixups stages and commits each group as a fixup of its target.
func (e *Engine) recordFixups(ctx context.Context, fp *FixupPlan, opts Options, result *Result) error {
	if err := e.Repo.ClearIndex(ctx); err != nil {
		return err
	}
	for i, fixup := range groupedFixups(fp.Fixups) {
		if err := e.stage(ctx, plan.Commit{Subject: fixup.Subject, Parts: fixup.Parts}); err != nil {
			return fmt.Errorf("fixup %d (%s): %w", i+1, fixup.Target, err)
		}
		hash, err := e.Repo.CommitFixup(ctx, fixup.Target, opts.NoVerify)
		if err != nil {
			return fmt.Errorf("fixup %d (%s): %w", i+1, fixup.Target, err)
		}
		result.Commits = append(result.Commits, Recorded{
			Hash:    hash,
			Subject: fixup.Subject,
			Paths:   plan.Commit{Parts: fixup.Parts}.Paths(),
		})
	}
	return nil
}

// grouped is one target commit and everything being folded into it.
type grouped struct {
	// Target is the commit being amended.
	Target string
	// Subject is the fixup commit's subject, for reporting.
	Subject string
	// Parts are the changes going in.
	Parts []plan.Part
}

// groupedFixups collects a plan's routed changes by target commit.
func groupedFixups(fixups []Fixup) []grouped {
	byTarget := make(map[string]*grouped)
	var order []string
	for _, fixup := range fixups {
		entry, seen := byTarget[fixup.Target.Hash]
		if !seen {
			entry = &grouped{
				Target:  fixup.Target.Hash,
				Subject: "fixup! " + fixup.Target.Subject,
			}
			byTarget[fixup.Target.Hash] = entry
			order = append(order, fixup.Target.Hash)
		}
		entry.Parts = append(entry.Parts, fixup.Part)
	}
	out := make([]grouped, 0, len(order))
	for _, hash := range order {
		out = append(out, *byTarget[hash])
	}
	return out
}
