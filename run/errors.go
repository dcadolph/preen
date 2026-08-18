package run

import "errors"

// Sentinel errors for a preen run.
var (
	// ErrNothingToDo reports a clean tree with no commits to redo.
	ErrNothingToDo = errors.New("nothing to preen")
	// ErrContentChanged reports that the content tree moved during a run,
	// which means work was lost or invented. The run is rolled back.
	ErrContentChanged = errors.New("content changed during the run")
	// ErrGateFailed reports that the configured gate command failed after a
	// commit.
	ErrGateFailed = errors.New("gate command failed")
	// ErrHunkMissing reports a planned hunk that no longer appears in the
	// regenerated diff, so the plan no longer matches the tree.
	ErrHunkMissing = errors.New("planned hunk not found in the current diff")
	// ErrPushedRewrite reports an attempt to redo published commits without
	// the explicit consent that requires.
	ErrPushedRewrite = errors.New("range contains pushed commits")
	// ErrProtectedBranch reports a rewrite refused because the branch is shared
	// by name, like main.
	ErrProtectedBranch = errors.New("refusing to rewrite a protected branch")
	// ErrNeedBase reports a rewrite that cannot work out which commits to redo.
	ErrNeedBase = errors.New("a rewrite needs an explicit base")
	// ErrFixupTarget reports a change no unpushed commit introduced, so there is
	// nothing to fold it into.
	ErrFixupTarget = errors.New("no unpushed commit introduced these lines")
)
