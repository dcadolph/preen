package repo

import (
	"context"
	"fmt"
	"path"
	"strings"
)

// DefaultProtected are the branch names preen refuses to rewrite without an
// explicit override, whatever the repository configures.
//
//nolint:gochecknoglobals // Immutable lookup.
var DefaultProtected = []string{"main", "master", "trunk", "develop", "release", "production"}

// IsProtected reports whether a branch is one preen must not rewrite. The
// built-in names always apply, and a pattern may use shell globbing so a
// project can protect a whole namespace like "release/*".
func IsProtected(branch string, extra []string) bool {
	for _, name := range DefaultProtected {
		if strings.EqualFold(branch, name) {
			return true
		}
	}
	for _, pattern := range extra {
		if strings.EqualFold(branch, pattern) {
			return true
		}
		if ok, err := path.Match(pattern, branch); err == nil && ok {
			return true
		}
	}
	return false
}

// Remote returns the remote the current branch tracks, defaulting to origin
// when the branch tracks nothing.
func (r *Repo) Remote(ctx context.Context) (string, error) {
	branch, err := r.CurrentBranch(ctx)
	if err != nil {
		return "", err
	}
	name, err := r.run(ctx, "config", "--get", "branch."+branch+".remote")
	if err != nil || name == "" {
		return "origin", nil //nolint:nilerr // An untracked branch still pushes to origin.
	}
	return name, nil
}

// ForcePushWithLease republishes a rewritten branch, refusing if the remote
// moved since it was last fetched.
//
// The lease is what makes a force push survivable: if someone else pushed in
// the meantime, the push aborts instead of destroying their work. A plain
// --force is never used.
func (r *Repo) ForcePushWithLease(ctx context.Context, remote, branch string) error {
	if remote == "" || branch == "" {
		return fmt.Errorf("%w: push needs a remote and a branch", ErrGit)
	}
	_, err := r.run(ctx, "push", "--force-with-lease", remote, branch)
	return err
}

// PushPreview renders the exact push a rewrite will run, so it can be shown
// before consent is given rather than described in the abstract.
func PushPreview(remote, branch string) string {
	return fmt.Sprintf("git push --force-with-lease %s %s", remote, branch)
}
