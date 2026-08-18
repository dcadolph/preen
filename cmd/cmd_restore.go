package cmd

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"time"

	"github.com/dcadolph/preen/repo"
)

// restoreUsage is the help text for the restore command.
const restoreUsage = `usage: preen restore [ref] [flags]

Undo a preen run by moving the branch back to the recovery ref it left behind.
With no ref, the most recent backup is used. Your commits since the run are
kept: the move refuses rather than discarding work it would overwrite.

Flags:
  --yes       Skip the confirmation prompt.
  --list      List the available backups and exit.
  -h, --help  Print this help and exit.
`

// runRestore moves the branch back to a preen backup ref, undoing a run.
func runRestore(ctx context.Context, env *environment, args []string) (int, error) {
	fs := flag.NewFlagSet("preen restore", flag.ContinueOnError)
	fs.SetOutput(env.Err)
	fs.Usage = func() { env.print(restoreUsage) }
	yes := fs.Bool("yes", false, "skip the confirmation prompt")
	list := fs.Bool("list", false, "list the available backups and exit")
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return CodeOK, nil
		}
		return CodeErr, err
	}

	repository, err := openRepo(ctx, env)
	if err != nil {
		return exitCode(err), err
	}
	backups, err := repository.ListBackups(ctx)
	if err != nil {
		return CodeErr, err
	}
	if len(backups) == 0 {
		return CodeErr, ErrNoBackups
	}
	if *list {
		printBackups(env, backups)
		return CodeOK, nil
	}

	target, err := pickBackup(fs.Arg(0), backups)
	if err != nil {
		return CodeErr, err
	}
	head, err := repository.Head(ctx)
	if err != nil {
		return CodeErr, err
	}
	env.printf("Restore %s\n", target.Name)
	env.printf("  branch moves from %s to %s\n", short(head), short(target.Hash))
	env.printf("  saved %s\n", target.Created.Format(time.RFC1123))

	if !*yes {
		ok, err := confirm(env, "Move the branch back? [y/N]: ")
		if err != nil {
			return CodeErr, err
		}
		if !ok {
			env.println("Aborted. Nothing was changed.")
			return CodeAborted, nil
		}
	}
	if err := repository.RestoreBackup(ctx, target.Name); err != nil {
		return CodeErr, fmt.Errorf("restore failed, the repository was not moved: %w", err)
	}
	env.printf("Restored to %s. The backup ref is still there if you need it again.\n", target.Name)
	return CodeOK, nil
}

// pickBackup resolves the requested ref, defaulting to the newest backup. A
// bare timestamp is accepted as well as the full ref name.
func pickBackup(want string, backups []repo.Backup) (repo.Backup, error) {
	if want == "" {
		return backups[0], nil
	}
	for _, backup := range backups {
		if backup.Name == want || backup.Name == repo.BackupPrefix+want {
			return backup, nil
		}
	}
	return repo.Backup{}, fmt.Errorf("%w: %s", ErrNoBackups, want)
}

// printBackups lists recovery refs with their age and whether the branch
// already contains them.
func printBackups(env *environment, backups []repo.Backup) {
	env.printf("Backups (%d):\n", len(backups))
	for _, backup := range backups {
		state := "holds work not on this branch"
		if backup.Merged {
			state = "already contained, safe to prune"
		}
		env.printf("  %-34s %s  %s\n", backup.Name, short(backup.Hash), state)
	}
}
