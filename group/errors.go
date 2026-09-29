package group

import "errors"

// Sentinel errors for grouping.
var (
	// ErrNoCommand reports a command grouper with no program to run.
	ErrNoCommand = errors.New("no grouper command configured")
	// ErrCommand reports a grouper program that failed to run.
	ErrCommand = errors.New("grouper command failed")
	// ErrRequest reports a request that could not be encoded.
	ErrRequest = errors.New("cannot build the grouper request")
	// ErrResponse reports an answer preen cannot trust: unparsable, empty, or
	// naming a path or hunk that is not in the tree.
	ErrResponse = errors.New("unusable grouper response")
	// ErrStale reports an answer written for a tree other than the one being
	// preened.
	ErrStale = errors.New("answer does not match this tree")
	// ErrAnswer reports an answer file that could not be read.
	ErrAnswer = errors.New("cannot read the grouping answer")
)
