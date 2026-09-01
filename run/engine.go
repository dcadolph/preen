// Package run orchestrates a preen run: survey the tree, group it into a
// plan, and apply that plan under a conservation guarantee.
//
// The engine owns every mechanical step and every guardrail. Grouping is the
// only judgment it delegates, so a run behaves the same whether a model was
// involved or not.
package run

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/dcadolph/preen/group"
	"github.com/dcadolph/preen/plan"
	"github.com/dcadolph/preen/repo"
	"github.com/dcadolph/preen/style"
	"github.com/dcadolph/preen/sweep"
)

// Options control a single run.
type Options struct {
	// Scope limits the run to paths matching these pathspecs. Everything else
	// stays uncommitted and untouched.
	Scope []string
	// Gate is a command run after each commit. A failure stops the run.
	Gate string
	// Absorb brings unpushed commits back into the tree to be redone.
	Absorb bool
	// NoVerify skips commit hooks, which requires standing consent from the
	// caller and is never preen's own decision.
	NoVerify bool
	// AllowHookRewrites accepts content differences confined to paths the run
	// committed, which is what a reformatting commit hook produces.
	AllowHookRewrites bool
	// Style is the commit message convention. It is applied while planning, so
	// the messages shown for approval are the ones that get recorded.
	Style style.Style
	// Pushed grants the explicit consent that rewriting published commits
	// requires. It cannot come from a config file, only from the invocation.
	Pushed bool
	// PushedBase names the commit just before the range to redo. Without one,
	// the merge base with the default branch is used.
	PushedBase string
	// AllowProtected permits a rewrite on a protected branch, the second
	// consent needed when the branch is shared by name.
	AllowProtected bool
	// Protected are extra branch patterns the repository declares off limits.
	Protected []string
	// Fixup folds each change into the unpushed commit that introduced it
	// instead of building new commits.
	Fixup bool
	// IncludeFiles lists the touched paths in each commit body.
	IncludeFiles bool
	// IncludeLineNumbers cites each file's changed line ranges in the body,
	// read from the hunk headers of the real diff.
	IncludeLineNumbers bool
	// Sweep scans the added lines for debris and reports what it finds. It
	// never removes anything.
	Sweep bool
}

// Gate runs a check after a commit and reports failure.
type Gate interface {
	// Check runs the gate in dir and returns its combined output.
	Check(ctx context.Context, dir, command string) ([]byte, error)
}

// GateFunc adapts a function to the Gate interface.
type GateFunc func(ctx context.Context, dir, command string) ([]byte, error)

// Check calls f.
func (f GateFunc) Check(ctx context.Context, dir, command string) ([]byte, error) {
	return f(ctx, dir, command)
}

// Engine runs preen against one repository.
type Engine struct {
	// Repo is the repository being preened.
	Repo *repo.Repo
	// Grouper decides how changes cluster into commits.
	Grouper group.Grouper
	// Gate runs the configured check after each commit.
	Gate Gate
	// Now supplies the current time, injectable so runs are reproducible in
	// tests.
	Now func() time.Time
	// Out receives progress output.
	Out io.Writer
}

// New returns an Engine with the deterministic grouper and real clock, the
// configuration that needs no model and no network.
func New(r *repo.Repo) *Engine {
	if r == nil {
		panic("run.New: Repo required")
	}
	return &Engine{
		Repo:    r,
		Grouper: group.NewHeuristic(),
		Gate:    shellGate{},
		Now:     time.Now,
		Out:     os.Stdout,
	}
}

// Recorded is a commit the run created.
type Recorded struct {
	// Hash is the new commit.
	Hash string
	// Subject is its subject line.
	Subject string
	// Paths are the files it touched.
	Paths []string
}

// Result reports what a run did.
type Result struct {
	// Backup is the recovery ref covering the whole run.
	Backup string
	// Commits are the commits created, in order.
	Commits []Recorded
	// TreeStart is the content hash before the run.
	TreeStart string
	// TreeEnd is the content hash after the run.
	TreeEnd string
	// Reformatted lists paths a commit hook rewrote, accepted only when the
	// caller allowed hook rewrites.
	Reformatted []string
}

// Plan surveys the repository and builds a plan without changing anything.
func (e *Engine) Plan(ctx context.Context, opts Options) (*plan.Plan, error) {
	if err := e.Repo.CheckReady(ctx); err != nil {
		return nil, err
	}
	built := &plan.Plan{}

	if opts.Absorb || opts.Pushed {
		base, err := e.rewriteBase(ctx, opts)
		if err != nil {
			return nil, err
		}
		check, err := e.Repo.CheckMerges(ctx, base)
		if err != nil {
			return nil, err
		}
		absorbed, err := e.Repo.Log(ctx, check.SafeBase+"..HEAD")
		if err != nil {
			return nil, err
		}
		if opts.Pushed {
			if err := e.allowRewrite(ctx, opts); err != nil {
				return nil, err
			}
			built.Push, err = e.pushPlan(ctx)
			if err != nil {
				return nil, err
			}
		} else if err := e.refusePushed(ctx, absorbed); err != nil {
			return nil, err
		}
		built.Base = check.SafeBase
		built.MergeSummary = check.Summary()
		built.Absorbed = absorbed
	}

	changes, err := e.survey(ctx, opts, built.Base)
	if err != nil {
		return nil, err
	}
	if len(changes) == 0 && len(built.Absorbed) == 0 {
		return nil, ErrNothingToDo
	}

	diffs, err := e.Repo.Diff(ctx, pathsOf(changes)...)
	if err != nil {
		return nil, err
	}
	commits, err := e.Grouper.Group(ctx, group.Input{Changes: changes, Diffs: diffs})
	if err != nil {
		return nil, fmt.Errorf("grouping failed: %w", err)
	}
	// The style is applied here rather than at commit time so the plan shown
	// for approval holds the exact messages that will be recorded. Auto
	// punctuation is resolved first, since it depends on the repository.
	shapedStyle := opts.Style
	shapedStyle.Punctuation = e.resolvePunctuation(ctx, opts.Style.Punctuation)
	for i, commit := range commits {
		shaped := shapedStyle.Apply(annotate(commit, diffs, opts))
		if err := shapedStyle.Verify(shaped); err != nil {
			return nil, fmt.Errorf("commit %d: %w", i+1, err)
		}
		commits[i] = shaped
	}
	if opts.Sweep {
		for _, finding := range e.sweepAll(diffs, changes) {
			built.Debris = append(built.Debris, finding.String())
		}
	}
	built.Commits = commits
	built.Covers = changes
	if err := built.Revalidate(); err != nil {
		return nil, err
	}
	return built, nil
}

// sweepAll scans both the diffs and any untracked file, which has no diff of
// its own until it is staged.
func (e *Engine) sweepAll(diffs []repo.FileDiff, changes []repo.Change) []sweep.Finding {
	findings := sweep.Scan(diffs)
	for _, change := range changes {
		if change.Kind != repo.KindUntracked {
			continue
		}
		content, err := os.ReadFile(filepath.Join(e.Repo.Root(), change.Path))
		if err != nil {
			// An unreadable file is not worth failing a run over, and the
			// commit itself will surface any real problem with it.
			continue
		}
		findings = append(findings, sweep.ScanFile(change.Path, string(content))...)
	}
	return findings
}

// rewriteBase returns the commit a resetting run goes back to: the last
// published commit for an absorb, or the caller's chosen base for a rewrite of
// published history.
func (e *Engine) rewriteBase(ctx context.Context, opts Options) (string, error) {
	if !opts.Pushed {
		base, err := e.Repo.UnpushedBase(ctx)
		if err != nil {
			return "", fmt.Errorf("cannot tell which commits are unpushed: %w", err)
		}
		return base, nil
	}
	if opts.PushedBase != "" {
		return e.Repo.Resolve(ctx, opts.PushedBase)
	}
	// Without a base, the merge base with the default branch bounds the range,
	// which is the work this branch added. On the default branch itself there
	// is no such boundary, so a base has to be named.
	branch, err := e.Repo.CurrentBranch(ctx)
	if err != nil {
		return "", err
	}
	def, err := e.Repo.DefaultBranch(ctx)
	if err != nil {
		return "", fmt.Errorf("%w: no default branch to bound the range, name a base", ErrNeedBase)
	}
	if strings.EqualFold(branch, strings.TrimPrefix(def, "origin/")) {
		return "", fmt.Errorf("%w: on %s a rewrite needs an explicit base", ErrNeedBase, branch)
	}
	return e.Repo.MergeBase(ctx, "HEAD", def)
}

// allowRewrite enforces the two consents rewriting published history needs:
// the explicit ask, which the caller already gave to reach here, and a branch
// that is not shared by name.
func (e *Engine) allowRewrite(ctx context.Context, opts Options) error {
	branch, err := e.Repo.CurrentBranch(ctx)
	if err != nil {
		return err
	}
	if repo.IsProtected(branch, opts.Protected) && !opts.AllowProtected {
		return fmt.Errorf("%w: %s is a protected branch", ErrProtectedBranch, branch)
	}
	return nil
}

// pushPlan describes the force push a rewrite will run, so it can be shown
// before consent rather than described after.
func (e *Engine) pushPlan(ctx context.Context) (string, error) {
	branch, err := e.Repo.CurrentBranch(ctx)
	if err != nil {
		return "", err
	}
	remote, err := e.Repo.Remote(ctx)
	if err != nil {
		return "", err
	}
	return repo.PushPreview(remote, branch), nil
}

// Push republishes a rewritten branch. It is separate from Apply so the push
// happens only after the commits are recorded and verified.
func (e *Engine) Push(ctx context.Context) error {
	branch, err := e.Repo.CurrentBranch(ctx)
	if err != nil {
		return err
	}
	remote, err := e.Repo.Remote(ctx)
	if err != nil {
		return err
	}
	return e.Repo.ForcePushWithLease(ctx, remote, branch)
}

// refusePushed rejects a range holding commits a remote already has, which is
// the one thing preen never does without an explicit ask.
func (e *Engine) refusePushed(ctx context.Context, commits []repo.Commit) error {
	for _, commit := range commits {
		pushed, err := e.Repo.IsPushed(ctx, commit.Hash)
		if err != nil {
			return err
		}
		if pushed {
			return fmt.Errorf("%w: %s %s", ErrPushedRewrite, commit.Short(), commit.Subject)
		}
	}
	return nil
}

// survey reads the changes a run will regroup. For an absorb run the commits
// being redone are not yet in the tree, so the survey is taken against the
// base rather than HEAD.
func (e *Engine) survey(ctx context.Context, opts Options, base string) ([]repo.Change, error) {
	changes, err := e.Repo.Status(ctx)
	if err != nil {
		return nil, err
	}
	if base != "" {
		absorbed, err := e.Repo.ChangedPaths(ctx, base, "HEAD")
		if err != nil {
			return nil, err
		}
		changes = mergeChanges(changes, absorbed)
	}
	return inScope(changes, opts.Scope), nil
}

// Apply carries out a plan and verifies that content was conserved. On any
// divergence it restores from the backup ref and reports what differed.
func (e *Engine) Apply(ctx context.Context, p *plan.Plan, opts Options) (*Result, error) {
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
	p.Backup = backup
	result := &Result{Backup: backup, TreeStart: treeStart}

	if err := e.execute(ctx, p, opts, result); err != nil {
		return result, e.rollback(ctx, backup, err)
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

// execute performs the reset and records every planned commit.
func (e *Engine) execute(ctx context.Context, p *plan.Plan, opts Options, result *Result) error {
	if p.Resets() {
		if err := e.Repo.SoftReset(ctx, p.Base); err != nil {
			return err
		}
	}
	if err := e.Repo.ClearIndex(ctx); err != nil {
		return err
	}
	for i, commit := range p.Commits {
		if err := e.stage(ctx, commit); err != nil {
			return fmt.Errorf("commit %d (%q): %w", i+1, commit.Subject, err)
		}
		hash, err := e.Repo.Commit(ctx, repo.CommitOptions{
			Message:  commit.Message(),
			NoVerify: opts.NoVerify,
		})
		if err != nil {
			return fmt.Errorf("commit %d (%q): %w", i+1, commit.Subject, err)
		}
		result.Commits = append(result.Commits, Recorded{
			Hash:    hash,
			Subject: commit.Subject,
			Paths:   commit.Paths(),
		})
		if err := e.runGate(ctx, opts, i+1, commit.Subject); err != nil {
			return err
		}
	}
	return nil
}

// stage puts exactly one planned commit's content into the index. Whole files
// are staged by path; a partial file has its hunks located in a freshly
// regenerated diff, since earlier commits shift every later hunk.
func (e *Engine) stage(ctx context.Context, commit plan.Commit) error {
	var whole []string
	for _, part := range commit.Parts {
		if part.Whole() {
			whole = append(whole, part.Path)
			if part.From != "" {
				whole = append(whole, part.From)
			}
			continue
		}
		if err := e.stagePartial(ctx, part); err != nil {
			return err
		}
	}
	if err := e.Repo.StagePaths(ctx, whole...); err != nil {
		return err
	}
	staged, err := e.Repo.HasStagedChanges(ctx)
	if err != nil {
		return err
	}
	if !staged {
		return repo.ErrEmptyStage
	}
	return nil
}

// stagePartial stages a subset of one file's hunks by regenerating its diff
// and matching the planned hunks by body.
func (e *Engine) stagePartial(ctx context.Context, part plan.Part) error {
	if part.Kind == repo.KindUntracked {
		// An untracked file has no diff until git knows the path exists.
		if err := e.Repo.IntentToAdd(ctx, part.Path); err != nil {
			return err
		}
	}
	diffs, err := e.Repo.Diff(ctx, part.Path)
	if err != nil {
		return err
	}
	if len(diffs) == 0 {
		return fmt.Errorf("%w: %s has no pending diff", ErrHunkMissing, part.Path)
	}
	file := diffs[0]
	selected, err := selectHunks(file, part.Hunks)
	if err != nil {
		return err
	}
	patch := file.TextWith(selected)
	if err := e.Repo.CheckPatch(ctx, patch); err != nil {
		return fmt.Errorf("%s: planned hunks no longer apply: %w", part.Path, err)
	}
	return e.Repo.ApplyToIndex(ctx, patch)
}

// selectHunks maps planned hunks onto positions in a regenerated diff by
// matching bodies, so a shifted hunk is still found.
func selectHunks(file repo.FileDiff, wanted []plan.Hunk) ([]int, error) {
	taken := make(map[int]bool, len(wanted))
	selected := make([]int, 0, len(wanted))
	for _, want := range wanted {
		found := -1
		for i, hunk := range file.Hunks {
			if taken[i] {
				continue
			}
			if bodyOf(hunk) == want.Body {
				found = i
				break
			}
		}
		if found < 0 {
			return nil, fmt.Errorf("%w: %s hunk %d", ErrHunkMissing, file.Path, want.Index)
		}
		taken[found] = true
		selected = append(selected, found)
	}
	return selected, nil
}

// bodyOf renders a hunk's lines without its header, the identity used to match
// a planned hunk against a regenerated diff.
func bodyOf(h repo.Hunk) string { return strings.Join(h.Lines, "\n") }

// runGate runs the configured check after a commit.
func (e *Engine) runGate(ctx context.Context, opts Options, number int, subject string) error {
	if opts.Gate == "" {
		return nil
	}
	out, err := e.Gate.Check(ctx, e.Repo.Root(), opts.Gate)
	if err == nil {
		return nil
	}
	return fmt.Errorf("%w after commit %d (%q): %w\n%s", ErrGateFailed, number, subject, err, out)
}

// verify enforces the conservation invariant: content in equals content out.
func (e *Engine) verify(ctx context.Context, result *Result, opts Options) error {
	if result.TreeEnd == result.TreeStart {
		return nil
	}
	diverged, err := e.Repo.TreeDiffPaths(ctx, result.TreeStart, result.TreeEnd)
	if err != nil {
		return err
	}
	if opts.AllowHookRewrites && withinCommitted(diverged, result.Commits) {
		result.Reformatted = diverged
		return nil
	}
	return fmt.Errorf("%w: %s", ErrContentChanged, strings.Join(diverged, ", "))
}

// withinCommitted reports whether every diverging path was touched by a commit
// this run made, which is the signature of a reformatting hook rather than
// lost work.
func withinCommitted(diverged []string, commits []Recorded) bool {
	touched := make(map[string]bool)
	for _, commit := range commits {
		for _, path := range commit.Paths {
			touched[path] = true
		}
	}
	for _, path := range diverged {
		if !touched[path] {
			return false
		}
	}
	return len(diverged) > 0
}

// rollback restores from the backup ref and returns an error describing both
// the original failure and the state the repository was left in.
func (e *Engine) rollback(ctx context.Context, backup string, cause error) error {
	if err := e.Repo.RestoreBackup(ctx, backup); err != nil {
		return fmt.Errorf("%w (restore from %s also failed: %w)", cause, backup, err)
	}
	return fmt.Errorf("%w (restored from %s)", cause, backup)
}

// pathsOf returns the paths of a change set.
func pathsOf(changes []repo.Change) []string {
	return repo.Paths(changes)
}
