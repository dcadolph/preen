package repo

import (
	"context"
	"fmt"
	"strings"
	"time"
)

// BackupPrefix is the ref namespace every preen recovery branch lives under.
// Only branches beneath it are ever pruning candidates.
const BackupPrefix = "preen-backup/"

// Backup is a recovery branch left behind by a preen run.
type Backup struct {
	// Name is the branch name, including the preen-backup/ prefix.
	Name string
	// Hash is the commit the branch points at.
	Hash string
	// Created is the branch's commit timestamp, used to report its age.
	Created time.Time
	// Merged reports whether the current branch already contains the tip, which
	// means the backup holds nothing that would be lost by deleting it.
	Merged bool
}

// CreateBackup points a new preen-backup branch at HEAD and returns its name.
// It is the undo anchor for a run and must be created before any history moves.
func (r *Repo) CreateBackup(ctx context.Context, now time.Time) (string, error) {
	name := BackupPrefix + now.Format("20060102-150405")
	// A second run inside the same second would collide, so widen the name
	// rather than fail or overwrite an existing recovery point.
	if r.refExists(ctx, name) {
		name = fmt.Sprintf("%s-%09d", name, now.Nanosecond())
	}
	if _, err := r.run(ctx, "branch", name); err != nil {
		return "", fmt.Errorf("backup branch: %w", err)
	}
	return name, nil
}

// refExists reports whether a ref resolves.
func (r *Repo) refExists(ctx context.Context, name string) bool {
	_, err := r.run(ctx, "rev-parse", "--verify", "--quiet", "refs/heads/"+name)
	return err == nil
}

// ListBackups returns every preen-backup branch, newest first.
func (r *Repo) ListBackups(ctx context.Context) ([]Backup, error) {
	const format = "%(refname:short)%09%(objectname)%09%(committerdate:unix)"
	out, err := r.run(ctx, "for-each-ref", "--sort=-committerdate",
		"--format="+format, "refs/heads/"+BackupPrefix+"*")
	if err != nil {
		return nil, err
	}
	lines := splitLines(out)
	backups := make([]Backup, 0, len(lines))
	for _, line := range lines {
		fields := strings.Split(line, "\t")
		if len(fields) != 3 {
			return nil, fmt.Errorf("%w: for-each-ref line %q", ErrParse, line)
		}
		created, err := parseUnix(fields[2])
		if err != nil {
			return nil, fmt.Errorf("%w: for-each-ref date %q", ErrParse, fields[2])
		}
		merged, err := r.contains(ctx, fields[1])
		if err != nil {
			return nil, err
		}
		backups = append(backups, Backup{
			Name:    fields[0],
			Hash:    fields[1],
			Created: created,
			Merged:  merged,
		})
	}
	return backups, nil
}

// contains reports whether HEAD already contains the commit, which marks a
// backup as safe to delete.
func (r *Repo) contains(ctx context.Context, hash string) (bool, error) {
	out, err := r.run(ctx, "branch", "--contains", hash, "--format=%(refname:short)")
	if err != nil {
		// A commit unreachable from any branch is not contained, which is not
		// an error worth failing the listing over.
		return false, nil //nolint:nilerr // Unreachable commit means not contained.
	}
	head, err := r.CurrentBranch(ctx)
	if err != nil {
		return false, nil //nolint:nilerr // Detached head cannot contain by name.
	}
	for _, name := range splitLines(out) {
		if strings.TrimSpace(name) == head {
			return true, nil
		}
	}
	return false, nil
}

// DeleteBackup removes a preen-backup branch. It refuses any ref outside the
// backup namespace, so a mistaken argument can never delete real work.
func (r *Repo) DeleteBackup(ctx context.Context, name string) error {
	if !strings.HasPrefix(name, BackupPrefix) {
		return fmt.Errorf("refusing to delete %q: not a %s ref", name, BackupPrefix)
	}
	_, err := r.run(ctx, "branch", "-D", name)
	return err
}

// RestoreBackup moves the current branch back to a backup ref, undoing a run
// and leaving the work as uncommitted changes exactly as it was beforehand.
//
// The reset is deliberately mixed. A preen run only reshapes history, so undo
// must move HEAD and the index while leaving the working tree untouched, which
// puts the content back in the messy state it started in. Neither --hard nor
// --keep is safe here: both update tracked files to the target commit, which
// deletes from disk every file the undone commits had added.
func (r *Repo) RestoreBackup(ctx context.Context, name string) error {
	if !strings.HasPrefix(name, BackupPrefix) {
		return fmt.Errorf("refusing to restore from %q: not a %s ref", name, BackupPrefix)
	}
	_, err := r.run(ctx, "reset", "--mixed", name)
	return err
}

// parseUnix converts a unix timestamp string into a time.
func parseUnix(s string) (time.Time, error) {
	var secs int64
	if _, err := fmt.Sscanf(s, "%d", &secs); err != nil {
		return time.Time{}, err
	}
	return time.Unix(secs, 0), nil
}
