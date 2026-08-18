package style

import "errors"

// ErrStyle reports a commit message that breaks the configured convention.
var ErrStyle = errors.New("message does not match the configured style")
