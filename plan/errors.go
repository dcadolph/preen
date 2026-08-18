package plan

import "errors"

// Sentinel errors for plan construction and editing.
var (
	// ErrInvalid reports a plan that does not account for the working tree
	// exactly once, or that holds an empty or unnamed commit.
	ErrInvalid = errors.New("invalid plan")
	// ErrNoSuchCommit reports an edit naming a commit number the plan lacks.
	ErrNoSuchCommit = errors.New("no such commit in the plan")
	// ErrNoSuchPath reports an edit naming a path the plan does not carry.
	ErrNoSuchPath = errors.New("no such path in the plan")
	// ErrUsage reports a command typed at the approval prompt that could not
	// be understood.
	ErrUsage = errors.New("cannot read that command")
)
