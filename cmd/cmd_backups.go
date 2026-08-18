package cmd

import (
	"context"
	"errors"
	"flag"

	"github.com/dcadolph/preen/repo"
)

// backupsUsage is the help text for the backups command.
const backupsUsage = `usage: preen backups [flags]

List the recovery refs preen has left behind. Every run creates one, so they
accumulate; pruning removes only those the current branch already contains,
which are the ones holding nothing you could still need.

Flags:
  --prune     Delete the backups already contained in this branch.
  --all       With --prune, delete every backup, contained or not.
  --yes       Skip the confirmation prompt.
  -h, --help  Print this help and exit.
`

// runBackups lists and optionally prunes preen recovery refs.
func runBackups(ctx context.Context, env *environment, args []string) (int, error) {
	fs := flag.NewFlagSet("preen backups", flag.ContinueOnError)
	fs.SetOutput(env.Err)
	fs.Usage = func() { env.print(backupsUsage) }
	prune := fs.Bool("prune", false, "delete backups already contained in this branch")
	all := fs.Bool("all", false, "with --prune, delete every backup")
	yes := fs.Bool("yes", false, "skip the confirmation prompt")
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
		env.println("No preen backups in this repository.")
		return CodeOK, nil
	}
	printBackups(env, backups)
	if !*prune {
		return CodeOK, nil
	}

	doomed := prunable(backups, *all)
	if len(doomed) == 0 {
		env.println("\nNothing to prune: every backup holds work this branch does not have.")
		return CodeOK, nil
	}
	env.printf("\nWill delete %d backup%s:\n", len(doomed), plural(len(doomed)))
	for _, backup := range doomed {
		env.printf("  %s\n", backup.Name)
	}
	if !*yes {
		ok, err := confirm(env, "Delete them? [y/N]: ")
		if err != nil {
			return CodeErr, err
		}
		if !ok {
			env.println("Aborted. Nothing was deleted.")
			return CodeAborted, nil
		}
	}
	for _, backup := range doomed {
		if err := repository.DeleteBackup(ctx, backup.Name); err != nil {
			return CodeErr, err
		}
	}
	env.printf("Deleted %d backup%s.\n", len(doomed), plural(len(doomed)))
	return CodeOK, nil
}

// prunable selects the backups a prune should delete. Without --all only the
// refs the branch already contains are candidates, so pruning can never
// discard the only copy of something.
func prunable(backups []repo.Backup, all bool) []repo.Backup {
	doomed := make([]repo.Backup, 0, len(backups))
	for _, backup := range backups {
		if all || backup.Merged {
			doomed = append(doomed, backup)
		}
	}
	return doomed
}
