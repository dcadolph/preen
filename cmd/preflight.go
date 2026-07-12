package cmd

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/storage/filesystem"
)

// stateMarkers are .git entries that mean an operation is in progress.
var stateMarkers = []string{"rebase-merge", "rebase-apply", "MERGE_HEAD", "CHERRY_PICK_HEAD"}

// checkRepo confirms the working directory is inside a git repository with
// no operation in progress. The skill refuses those states too; checking
// here fails fast before a claude session spins up.
func checkRepo() error {
	return checkRepoAt(".")
}

// checkRepoAt runs the repository preflight for the given directory.
func checkRepoAt(dir string) error {
	repo, err := git.PlainOpenWithOptions(dir, &git.PlainOpenOptions{DetectDotGit: true})
	if err != nil {
		if errors.Is(err, git.ErrRepositoryNotExists) {
			return ErrNoRepo
		}
		return fmt.Errorf("%w: %w", ErrNoRepo, err)
	}
	gitDir, err := repoDir(repo)
	if err != nil {
		// The state check is best-effort; the skill re-checks and refuses cleanly.
		return nil
	}
	for _, marker := range stateMarkers {
		if _, err := os.Stat(filepath.Join(gitDir, marker)); err == nil {
			return fmt.Errorf("%w: %s exists: finish or abort it first", ErrRepoState, marker)
		}
	}
	return nil
}

// repoDir returns the on-disk .git directory backing the repository.
func repoDir(repo *git.Repository) (string, error) {
	storage, ok := repo.Storer.(*filesystem.Storage)
	if !ok {
		return "", errors.New("repository storage is not filesystem-backed")
	}
	return storage.Filesystem().Root(), nil
}
