package cmd

// Exit codes returned by Execute. Once claude starts, its own exit code is
// mirrored instead.
const (
	// CodeOK means the run completed or a help or version request was served.
	CodeOK = 0
	// CodeErr covers failures with no more specific code.
	CodeErr = 1
	// CodeNoClaude means the claude binary was not found.
	CodeNoClaude = 2
	// CodeNoRepo means the working directory is not inside a git repository.
	CodeNoRepo = 3
	// CodeRepoState means the repository is mid-rebase, mid-merge, or mid-cherry-pick.
	CodeRepoState = 4
)
