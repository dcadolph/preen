package cmd

// Exit codes. Each failure mode gets its own code so a script can tell a
// rolled-back run from a rejected plan without parsing output.
const (
	// CodeOK means the run completed, or help or version was served.
	CodeOK = 0
	// CodeErr covers failures with no more specific code.
	CodeErr = 1
	// CodeNoRepo means the working directory is not inside a git repository.
	CodeNoRepo = 2
	// CodeRepoState means a rebase, merge, or cherry-pick is in progress.
	CodeRepoState = 3
	// CodeNothingToDo means the tree was clean and nothing needed redoing.
	CodeNothingToDo = 4
	// CodeInvalidPlan means the plan did not account for the tree exactly once.
	CodeInvalidPlan = 5
	// CodeGateFailed means the gate command failed and the run was rolled back.
	CodeGateFailed = 6
	// CodeContentChanged means the conservation check failed and the run was
	// rolled back. It is the code that should never appear.
	CodeContentChanged = 7
	// CodeAborted means the user declined the plan.
	CodeAborted = 8
)
