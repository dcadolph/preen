package repo

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// Runner executes a git command in a working directory and returns its stdout.
// Every repository operation goes through this one method, so a test can
// substitute a fake git without a real repository on disk.
type Runner interface {
	// Git runs git with args in dir and returns stdout. The env entries are
	// appended to the process environment as "KEY=value" pairs.
	Git(ctx context.Context, dir string, env []string, args ...string) ([]byte, error)
}

// RunnerFunc adapts a function to the Runner interface, following the shape of
// http.HandlerFunc.
type RunnerFunc func(ctx context.Context, dir string, env []string, args ...string) ([]byte, error)

// Git calls f.
func (f RunnerFunc) Git(ctx context.Context, dir string, env []string, args ...string) ([]byte, error) {
	return f(ctx, dir, env, args...)
}

// execRunner runs the real git binary.
type execRunner struct {
	// Bin is the git executable to run.
	Bin string
}

// Git runs the git binary and returns stdout, wrapping a non-zero exit with the
// trimmed stderr so the caller sees what git actually complained about.
func (r execRunner) Git(ctx context.Context, dir string, env []string, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, r.Bin, args...)
	cmd.Dir = dir
	if len(env) > 0 {
		cmd.Env = append(os.Environ(), env...)
	}
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		detail := strings.TrimSpace(stderr.String())
		if detail == "" {
			detail = err.Error()
		}
		return nil, fmt.Errorf("%w: git %s: %s", ErrGit, strings.Join(args, " "), detail)
	}
	return stdout.Bytes(), nil
}

// NewRunner returns a Runner that shells out to the git binary on PATH.
//
// The real git is used rather than a pure Go implementation because preen
// rewrites history: matching git's own index, patch application, and rebase
// behavior exactly matters more here than avoiding a process boundary.
func NewRunner() Runner {
	return execRunner{Bin: "git"}
}
