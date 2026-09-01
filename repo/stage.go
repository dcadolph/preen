package repo

import (
	"context"
	"fmt"
	"os"
)

// CommitOptions controls how a single commit is recorded.
type CommitOptions struct {
	// Message is the full commit message, subject and optional body.
	Message string
	// NoVerify skips commit hooks. It requires standing consent from the
	// caller, never preen's own judgment.
	NoVerify bool
}

// StagePaths stages whole paths, including deletions and untracked files.
func (r *Repo) StagePaths(ctx context.Context, paths ...string) error {
	if len(paths) == 0 {
		return nil
	}
	args := append([]string{"add", "--"}, paths...)
	_, err := r.run(ctx, args...)
	return err
}

// IntentToAdd records an untracked path in the index without its content, so
// its lines become visible to diff and can be split across commits.
func (r *Repo) IntentToAdd(ctx context.Context, paths ...string) error {
	if len(paths) == 0 {
		return nil
	}
	args := append([]string{"add", "-N", "--"}, paths...)
	_, err := r.run(ctx, args...)
	return err
}

// ApplyToIndex applies patch text to the index only, leaving the working tree
// untouched, which is how a subset of a file's hunks becomes one commit.
//
// The counts in the hunk headers describe the full diff rather than the
// selected subset, so recount is passed and git infers them from the body.
func (r *Repo) ApplyToIndex(ctx context.Context, patch string) error {
	if patch == "" {
		return nil
	}
	path, cleanup, err := writeTemp("preen-patch-*.diff", patch)
	if err != nil {
		return err
	}
	defer cleanup()
	_, err = r.run(ctx, "apply", "--cached", "--recount", "--whitespace=nowarn", path)
	return err
}

// CheckPatch reports whether a patch would apply to the index cleanly, without
// changing anything. It turns a doomed plan into an error before any commit is
// made.
func (r *Repo) CheckPatch(ctx context.Context, patch string) error {
	if patch == "" {
		return nil
	}
	path, cleanup, err := writeTemp("preen-patch-*.diff", patch)
	if err != nil {
		return err
	}
	defer cleanup()
	_, err = r.run(ctx, "apply", "--cached", "--check", "--recount", "--whitespace=nowarn", path)
	return err
}

// HasStagedChanges reports whether anything is staged for the next commit.
//
// The staged path list is read rather than the --quiet exit status, since git
// signals "differences exist" with a non-zero exit that is indistinguishable
// from a genuine failure once it has been wrapped in an error.
func (r *Repo) HasStagedChanges(ctx context.Context) (bool, error) {
	if !r.HasCommits(ctx) {
		out, err := r.run(ctx, "diff", "--cached", "--name-only", "--no-renames")
		if err != nil {
			return false, err
		}
		return len(splitLines(out)) > 0, nil
	}
	out, err := r.run(ctx, "diff", "--cached", "--name-only", "--no-renames", "HEAD")
	if err != nil {
		return false, err
	}
	return len(splitLines(out)) > 0, nil
}

// ClearIndex unstages everything without touching the working tree, so a run
// starts from a known index no matter what the user had staged.
func (r *Repo) ClearIndex(ctx context.Context) error {
	if !r.HasCommits(ctx) {
		_, err := r.run(ctx, "rm", "-r", "--cached", "-q", "--ignore-unmatch", ".")
		return err
	}
	_, err := r.run(ctx, "reset", "-q", "HEAD", "--")
	return err
}

// SoftReset moves the branch to base while keeping every change in the index
// and working tree, which is how committed work is absorbed for regrouping.
func (r *Repo) SoftReset(ctx context.Context, base string) error {
	_, err := r.run(ctx, "reset", "--soft", base)
	return err
}

// Commit records the staged content and returns the new commit hash. It
// refuses an empty stage, since that means the plan and the tree disagree.
func (r *Repo) Commit(ctx context.Context, opts CommitOptions) (string, error) {
	staged, err := r.HasStagedChanges(ctx)
	if err != nil {
		return "", err
	}
	if !staged {
		return "", ErrEmptyStage
	}
	path, cleanup, err := writeTemp("preen-message-*.txt", opts.Message)
	if err != nil {
		return "", err
	}
	defer cleanup()

	args := []string{"commit", "-F", path, "--cleanup=whitespace"}
	if opts.NoVerify {
		args = append(args, "--no-verify")
	}
	if _, err := r.run(ctx, args...); err != nil {
		return "", err
	}
	return r.Head(ctx)
}

// writeTemp writes content to a temp file and returns its path with a cleanup
// function. Patches and messages go through a file rather than stdin so the
// Runner interface stays a single method.
func writeTemp(pattern, content string) (string, func(), error) {
	file, err := os.CreateTemp("", pattern)
	if err != nil {
		return "", nil, fmt.Errorf("temp file: %w", err)
	}
	path := file.Name()
	cleanup := func() { _ = os.Remove(path) }
	if _, err := file.WriteString(content); err != nil {
		_ = file.Close()
		cleanup()
		return "", nil, fmt.Errorf("temp write: %w", err)
	}
	if err := file.Close(); err != nil {
		cleanup()
		return "", nil, fmt.Errorf("temp close: %w", err)
	}
	return path, cleanup, nil
}
