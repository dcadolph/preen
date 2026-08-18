package repo

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// stateMarkers are paths under the git directory whose presence means an
// operation is already in progress and preen must not touch anything.
//
//nolint:gochecknoglobals // Immutable lookup.
var stateMarkers = []string{"rebase-merge", "rebase-apply", "MERGE_HEAD", "CHERRY_PICK_HEAD", "REVERT_HEAD"}

// Repo is an open git repository that preen operates on. It is a thin typed
// layer over a Runner, so every operation is testable and nothing reaches for
// global state.
type Repo struct {
	// runner executes git commands.
	runner Runner
	// root is the absolute path to the top of the working tree.
	root string
	// gitDir is the absolute path to the git directory backing the repository.
	gitDir string
}

// Open discovers the repository containing dir and returns it. It fails with
// ErrNotRepo when dir is not inside a working tree.
func Open(ctx context.Context, dir string) (*Repo, error) {
	return OpenWith(ctx, dir, NewRunner())
}

// OpenWith discovers the repository containing dir using the given Runner.
func OpenWith(ctx context.Context, dir string, runner Runner) (*Repo, error) {
	if runner == nil {
		panic("repo.OpenWith: Runner required")
	}
	if dir == "" {
		dir = "."
	}
	root, err := runner.Git(ctx, dir, nil, "rev-parse", "--show-toplevel")
	if err != nil {
		return nil, fmt.Errorf("%w: %s", ErrNotRepo, dir)
	}
	gitDir, err := runner.Git(ctx, dir, nil, "rev-parse", "--absolute-git-dir")
	if err != nil {
		return nil, fmt.Errorf("%w: %s", ErrNotRepo, dir)
	}
	return &Repo{
		runner: runner,
		root:   trimLine(root),
		gitDir: trimLine(gitDir),
	}, nil
}

// Root returns the absolute path to the top of the working tree.
func (r *Repo) Root() string { return r.root }

// run executes git in the repository root and returns trimmed stdout.
func (r *Repo) run(ctx context.Context, args ...string) (string, error) {
	out, err := r.runner.Git(ctx, r.root, nil, args...)
	if err != nil {
		return "", err
	}
	return strings.TrimRight(string(out), "\n"), nil
}

// runEnv executes git in the repository root with extra environment entries.
func (r *Repo) runEnv(ctx context.Context, env []string, args ...string) (string, error) {
	out, err := r.runner.Git(ctx, r.root, env, args...)
	if err != nil {
		return "", err
	}
	return strings.TrimRight(string(out), "\n"), nil
}

// raw executes git in the repository root and returns stdout unmodified, for
// output where trailing bytes are significant, like patches.
func (r *Repo) raw(ctx context.Context, args ...string) ([]byte, error) {
	return r.runner.Git(ctx, r.root, nil, args...)
}

// CheckReady reports whether the repository is safe to operate on. It returns
// ErrInProgress when a rebase, merge, cherry-pick, or revert is underway, and
// reports unmerged paths the same way, since regrouping a conflicted tree
// would discard a resolution in progress.
func (r *Repo) CheckReady(ctx context.Context) error {
	for _, marker := range stateMarkers {
		if _, err := os.Stat(filepath.Join(r.gitDir, marker)); err == nil {
			return fmt.Errorf("%w: %s exists: finish or abort it first", ErrInProgress, marker)
		}
	}
	out, err := r.run(ctx, "status", "--porcelain=v1")
	if err != nil {
		return err
	}
	for _, line := range strings.Split(out, "\n") {
		if len(line) < 2 {
			continue
		}
		if isUnmerged(line[:2]) {
			return fmt.Errorf("%w: unmerged paths: resolve the conflict first", ErrInProgress)
		}
	}
	return nil
}

// isUnmerged reports whether a porcelain status code marks a conflicted path.
// The conflict states are DD, AU, UD, UA, DU, AA, and UU.
func isUnmerged(code string) bool {
	switch code {
	case "DD", "AU", "UD", "UA", "DU", "AA", "UU":
		return true
	}
	return false
}

// Head returns the commit hash HEAD points at.
func (r *Repo) Head(ctx context.Context) (string, error) {
	return r.run(ctx, "rev-parse", "HEAD")
}

// CurrentBranch returns the checked-out branch name, or ErrDetached when HEAD
// is not on a branch.
func (r *Repo) CurrentBranch(ctx context.Context) (string, error) {
	name, err := r.run(ctx, "rev-parse", "--abbrev-ref", "HEAD")
	if err != nil {
		return "", err
	}
	if name == "HEAD" {
		return "", ErrDetached
	}
	return name, nil
}

// Resolve turns a revision the user named, like a branch or "HEAD~4", into a
// full commit hash, failing when it does not name a commit.
func (r *Repo) Resolve(ctx context.Context, rev string) (string, error) {
	hash, err := r.run(ctx, "rev-parse", "--verify", rev+"^{commit}")
	if err != nil {
		return "", fmt.Errorf("%w: %q does not name a commit", ErrParse, rev)
	}
	return hash, nil
}

// HasCommits reports whether the repository has at least one commit, which is
// false in a freshly initialized repository where HEAD is unborn.
func (r *Repo) HasCommits(ctx context.Context) bool {
	_, err := r.run(ctx, "rev-parse", "--verify", "HEAD")
	return err == nil
}

// trimLine returns the first line of git output with whitespace trimmed.
func trimLine(out []byte) string {
	return strings.TrimSpace(strings.SplitN(string(out), "\n", 2)[0])
}
