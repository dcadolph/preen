package cmd

import "errors"

// Sentinel errors for preflight failures. Execute maps them to exit codes.
var (
	// ErrNoClaude reports that the claude binary could not be resolved.
	ErrNoClaude = errors.New("claude cli not found")
	// ErrNoRepo reports that the working directory is not in a git repository.
	ErrNoRepo = errors.New("not inside a git repository")
	// ErrRepoState reports a repository mid-rebase, mid-merge, or mid-cherry-pick.
	ErrRepoState = errors.New("repository has an operation in progress")
	// ErrUsage reports invalid wrapper arguments.
	ErrUsage = errors.New("invalid arguments")
)
