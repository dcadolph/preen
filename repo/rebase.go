package repo

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// LastCommitTouching returns the newest commit in base..HEAD that changed the
// path, which is the commit a fixup for that path belongs to. The zero Commit
// means no commit in range touched it.
func (r *Repo) LastCommitTouching(ctx context.Context, base, path string) (Commit, error) {
	const format = "%H%x1e%at%x1e%P%x1e%s"
	out, err := r.raw(ctx, "log", "-1", "--format="+format, base+"..HEAD", "--", path)
	if err != nil {
		return Commit{}, err
	}
	record := strings.TrimRight(string(out), "\n")
	if strings.TrimSpace(record) == "" {
		return Commit{}, nil
	}
	fields := strings.Split(record, "\x1e")
	if len(fields) < 4 {
		return Commit{}, fmt.Errorf("%w: log record %q", ErrParse, record)
	}
	when, err := parseUnix(fields[1])
	if err != nil {
		return Commit{}, fmt.Errorf("%w: log date %q", ErrParse, fields[1])
	}
	return Commit{
		Hash:       fields[0],
		AuthorDate: when,
		Merge:      len(strings.Fields(fields[2])) > 1,
		Subject:    fields[3],
	}, nil
}

// CommitFixup records the staged content as a fixup of the target commit, the
// marker an autosquash rebase later folds in.
func (r *Repo) CommitFixup(ctx context.Context, target string, noVerify bool) (string, error) {
	staged, err := r.HasStagedChanges(ctx)
	if err != nil {
		return "", err
	}
	if !staged {
		return "", ErrEmptyStage
	}
	args := []string{"commit", "--fixup=" + target}
	if noVerify {
		args = append(args, "--no-verify")
	}
	if _, err := r.run(ctx, args...); err != nil {
		return "", err
	}
	return r.Head(ctx)
}

// AutosquashOnto folds every fixup commit into its target.
//
// The sequence editor is forced to a no-op so the rebase runs unattended: git
// builds the todo list with the fixups already positioned, and accepting it
// unedited is exactly the intent. The editor for messages is silenced the same
// way so a reworded target cannot block on a prompt.
func (r *Repo) AutosquashOnto(ctx context.Context, base string) error {
	env := []string{
		"GIT_SEQUENCE_EDITOR=true",
		"GIT_EDITOR=true",
	}
	_, err := r.runEnv(ctx, env, "rebase", "--interactive", "--autosquash", base)
	return err
}

// AbortRebase abandons a rebase in progress, returning the branch to where it
// was before the rebase started. It is safe to call when no rebase is running.
func (r *Repo) AbortRebase(ctx context.Context) error {
	if !r.rebaseInProgress() {
		return nil
	}
	_, err := r.run(ctx, "rebase", "--abort")
	return err
}

// rebaseInProgress reports whether a rebase is underway, which decides whether
// an abort is meaningful.
func (r *Repo) rebaseInProgress() bool {
	for _, marker := range []string{"rebase-merge", "rebase-apply"} {
		if _, err := os.Stat(filepath.Join(r.gitDir, marker)); err == nil {
			return true
		}
	}
	return false
}
