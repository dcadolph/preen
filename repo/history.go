package repo

import (
	"context"
	"fmt"
	"strings"
	"time"
)

// Commit is a commit in the range preen is considering.
type Commit struct {
	// Hash is the full object name.
	Hash string
	// Subject is the first line of the message.
	Subject string
	// Body is the message after the subject, trimmed.
	Body string
	// AuthorDate is when the commit was authored.
	AuthorDate time.Time
	// Merge reports whether the commit has more than one parent.
	Merge bool
}

// Short returns the abbreviated hash used in reports.
func (c Commit) Short() string {
	if len(c.Hash) < 8 {
		return c.Hash
	}
	return c.Hash[:8]
}

// Upstream returns the tracking branch for the current branch, or
// ErrNoUpstream when the branch tracks nothing.
func (r *Repo) Upstream(ctx context.Context) (string, error) {
	name, err := r.run(ctx, "rev-parse", "--abbrev-ref", "--symbolic-full-name", "@{upstream}")
	if err != nil {
		return "", ErrNoUpstream
	}
	return name, nil
}

// DefaultBranch returns the remote's default branch, falling back to a local
// main or master when the remote does not advertise one.
func (r *Repo) DefaultBranch(ctx context.Context) (string, error) {
	if out, err := r.run(ctx, "symbolic-ref", "--short", "refs/remotes/origin/HEAD"); err == nil && out != "" {
		return out, nil
	}
	for _, candidate := range []string{"origin/main", "origin/master", "main", "master"} {
		if _, err := r.run(ctx, "rev-parse", "--verify", "--quiet", candidate); err == nil {
			return candidate, nil
		}
	}
	return "", fmt.Errorf("%w: no default branch found", ErrParse)
}

// MergeBase returns the best common ancestor of two revisions.
func (r *Repo) MergeBase(ctx context.Context, a, b string) (string, error) {
	return r.run(ctx, "merge-base", a, b)
}

// Log returns the commits in a revision range, newest first.
func (r *Repo) Log(ctx context.Context, revRange string) ([]Commit, error) {
	// Records are separated by a unit separator and fields by a record
	// separator, so subjects and bodies containing newlines survive parsing.
	const format = "%H%x1e%at%x1e%P%x1e%s%x1e%b%x1f"
	out, err := r.raw(ctx, "log", "--format="+format, revRange)
	if err != nil {
		return nil, err
	}
	var commits []Commit
	for _, record := range strings.Split(string(out), "\x1f") {
		record = strings.TrimLeft(record, "\n")
		if strings.TrimSpace(record) == "" {
			continue
		}
		fields := strings.Split(record, "\x1e")
		if len(fields) < 5 {
			return nil, fmt.Errorf("%w: log record %q", ErrParse, record)
		}
		when, err := parseUnix(fields[1])
		if err != nil {
			return nil, fmt.Errorf("%w: log date %q", ErrParse, fields[1])
		}
		commits = append(commits, Commit{
			Hash:       fields[0],
			AuthorDate: when,
			Merge:      len(strings.Fields(fields[2])) > 1,
			Subject:    fields[3],
			Body:       strings.TrimSpace(fields[4]),
		})
	}
	return commits, nil
}

// RemoteBranchesContaining returns the remote branches that contain a
// revision. A non-empty result means the commit is published.
func (r *Repo) RemoteBranchesContaining(ctx context.Context, rev string) ([]string, error) {
	out, err := r.run(ctx, "branch", "-r", "--contains", rev, "--format=%(refname:short)")
	if err != nil {
		// A revision no remote knows about makes git exit non-zero, which is
		// the answer rather than a failure.
		return nil, nil //nolint:nilerr // Unknown revision means not published.
	}
	return splitLines(out), nil
}

// IsPushed reports whether any remote branch contains the commit.
func (r *Repo) IsPushed(ctx context.Context, rev string) (bool, error) {
	branches, err := r.RemoteBranchesContaining(ctx, rev)
	if err != nil {
		return false, err
	}
	return len(branches) > 0, nil
}

// MergeInfo describes one merge commit found in the range being rewritten.
type MergeInfo struct {
	// Hash is the merge commit.
	Hash string
	// Subject is the merge commit's subject.
	Subject string
	// SideBranches are the remote branches containing the merge's second
	// parent. Non-empty means the merged work is published.
	SideBranches []string
}

// Published reports whether the merge brought in already-pushed work, which
// makes the merge unsafe to flatten.
func (m MergeInfo) Published() bool { return len(m.SideBranches) > 0 }

// MergeCheck is the result of the mandatory merge audit for a range that is
// about to be absorbed or rewritten.
type MergeCheck struct {
	// Base is the base the check was run against.
	Base string
	// Merges are the merge commits in base..HEAD.
	Merges []MergeInfo
	// SafeBase is the base preen may actually reset to. It moves forward past
	// the newest merge whose side branch is published, so published work is
	// never redone as new commits.
	SafeBase string
	// Moved reports whether SafeBase differs from the requested Base.
	Moved bool
	// Flattens reports whether unpushed merges in range will be linearized.
	Flattens bool
}

// Summary renders the one-line audit result that every plan involving a reset
// must carry.
func (m MergeCheck) Summary() string {
	switch {
	case len(m.Merges) == 0:
		return fmt.Sprintf("no merges in %s..HEAD", shorten(m.Base))
	case m.Moved:
		var names []string
		for _, merge := range m.Merges {
			if merge.Published() {
				names = append(names, merge.SideBranches...)
			}
		}
		return fmt.Sprintf("published merge in range (%s), base moved forward to %s",
			strings.Join(names, ", "), shorten(m.SafeBase))
	default:
		return fmt.Sprintf("%d unpushed merge(s) in %s..HEAD will be flattened",
			len(m.Merges), shorten(m.Base))
	}
}

// CheckMerges audits the merges between base and HEAD and returns the base
// that is actually safe to reset to.
//
// The rule is mechanical and has no override: a merge whose second parent is
// reachable from any remote branch brought in published work, so absorbing it
// would re-commit that work as new commits. The base moves forward past the
// newest such merge instead.
func (r *Repo) CheckMerges(ctx context.Context, base string) (MergeCheck, error) {
	check := MergeCheck{Base: base, SafeBase: base}
	merges, err := r.Log(ctx, base+"..HEAD")
	if err != nil {
		return MergeCheck{}, err
	}
	for _, commit := range merges {
		if !commit.Merge {
			continue
		}
		side, err := r.RemoteBranchesContaining(ctx, commit.Hash+"^2")
		if err != nil {
			return MergeCheck{}, err
		}
		check.Merges = append(check.Merges, MergeInfo{
			Hash:         commit.Hash,
			Subject:      commit.Subject,
			SideBranches: side,
		})
	}
	// Log returns newest first, so the first published merge encountered is the
	// newest one and defines how far the base must move.
	for _, merge := range check.Merges {
		if merge.Published() {
			check.SafeBase = merge.Hash
			check.Moved = true
			break
		}
	}
	if !check.Moved {
		check.Flattens = len(check.Merges) > 0
	}
	return check, nil
}

// UnpushedBase returns the commit preen would absorb back to for the current
// branch: the upstream when the branch tracks one, otherwise the fork point
// from the default branch.
func (r *Repo) UnpushedBase(ctx context.Context) (string, error) {
	if upstream, err := r.Upstream(ctx); err == nil {
		return r.run(ctx, "rev-parse", upstream)
	}
	def, err := r.DefaultBranch(ctx)
	if err != nil {
		return "", ErrNoUpstream
	}
	base, err := r.MergeBase(ctx, "HEAD", def)
	if err != nil {
		return "", ErrNoUpstream
	}
	return base, nil
}

// shorten abbreviates a hash for display, leaving branch names alone.
func shorten(rev string) string {
	if len(rev) == 40 {
		return rev[:8]
	}
	return rev
}
