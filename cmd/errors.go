package cmd

import "errors"

// Sentinel errors for the command line surface.
var (
	// ErrUsage reports invalid arguments.
	ErrUsage = errors.New("invalid arguments")
	// ErrAborted reports that the user declined the plan, which is a normal
	// outcome rather than a failure worth a message.
	ErrAborted = errors.New("aborted")
	// ErrNoBackups reports that no recovery ref exists to restore from.
	ErrNoBackups = errors.New("no preen backups in this repository")
)
