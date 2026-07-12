package cmd

import (
	"errors"
	"fmt"
	"os"
)

// Execute runs the wrapper with the embedded skill text and the given
// arguments, returning the process exit code and printing any error to
// stderr. It panics on empty skill text, which means a broken build.
func Execute(skill string, args []string) int {
	if skill == "" {
		panic("cmd.Execute: skill text required")
	}
	runner := newRunner(skill)
	code, err := runner.Run(args)
	if err != nil {
		fmt.Fprintf(os.Stderr, "preen: %v\n", err)
	}
	return code
}

// repoCode maps a preflight repository error to its exit code.
func repoCode(err error) int {
	switch {
	case errors.Is(err, ErrRepoState):
		return CodeRepoState
	case errors.Is(err, ErrNoRepo):
		return CodeNoRepo
	default:
		return CodeErr
	}
}
