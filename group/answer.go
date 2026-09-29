package group

import (
	"context"
	"fmt"
	"os"

	"github.com/dcadolph/preen/plan"
)

// Answer groups changes as a response written ahead of time says. It exists
// for the agent that made the changes: that agent knows why each hunk exists,
// which no rule and no model reading the diff cold can recover, so it answers
// the request from `preen request` itself and preen verifies the answer like
// any other.
//
// Unlike Command, an Answer is never paired with a fallback. Whoever wrote it
// is waiting on the result, so a stale or malformed answer is an error to fix,
// not a reason to quietly plan something else.
type Answer struct {
	// Path is the file holding the response.
	Path string
}

// Group reads the answer and converts it into commits. The answer must carry
// the tree hash of the request it answers, and that hash must match the tree
// being preened, because hunk indexes read from any other tree may name
// different hunks.
func (a Answer) Group(_ context.Context, in Input) ([]plan.Commit, error) {
	out, err := os.ReadFile(a.Path)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrAnswer, err)
	}
	response, err := decode(out)
	if err != nil {
		return nil, err
	}
	if response.Tree == "" {
		return nil, fmt.Errorf("%w: the answer carries no tree; copy it from preen request", ErrStale)
	}
	if response.Tree != in.Tree {
		return nil, fmt.Errorf("%w: request again, and keep both files outside the repository", ErrStale)
	}
	return convert(response, in)
}
