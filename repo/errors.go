package repo

import "errors"

// Sentinel errors for repository operations. Callers match with errors.Is.
var (
	// ErrNotRepo reports that the path is not inside a git repository.
	ErrNotRepo = errors.New("not inside a git repository")
	// ErrInProgress reports a repository mid-rebase, mid-merge, or mid-cherry-pick.
	ErrInProgress = errors.New("repository has an operation in progress")
	// ErrGit reports that a git command exited non-zero.
	ErrGit = errors.New("git command failed")
	// ErrParse reports output that did not match the expected git format.
	ErrParse = errors.New("git output parse failed")
	// ErrNoUpstream reports that the branch tracks no remote branch.
	ErrNoUpstream = errors.New("branch has no upstream")
	// ErrDetached reports that HEAD is not on a branch.
	ErrDetached = errors.New("head is detached")
	// ErrEmptyStage reports an attempt to commit with nothing staged, which
	// means the plan disagrees with the tree.
	ErrEmptyStage = errors.New("nothing staged to commit")
)
