package cmd

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	git "github.com/go-git/go-git/v5"
)

// newRepoDir initializes a git repository in a fresh temp directory.
func newRepoDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	if _, err := git.PlainInit(dir, false); err != nil {
		t.Fatalf("init repo: %v", err)
	}
	return dir
}

func TestCheckRepoAt(t *testing.T) {
	t.Parallel()
	tests := []struct {
		Setup func(t *testing.T) string
		Want  error
	}{{ // Test 0: Plain directory is not a repository.
		Setup: func(t *testing.T) string { t.Helper(); return t.TempDir() },
		Want:  ErrNoRepo,
	}, { // Test 1: Fresh repository passes.
		Setup: newRepoDir,
	}, { // Test 2: Merge in progress refuses.
		Setup: func(t *testing.T) string {
			t.Helper()
			dir := newRepoDir(t)
			if err := os.WriteFile(filepath.Join(dir, ".git", "MERGE_HEAD"), []byte("0000\n"), 0o644); err != nil {
				t.Fatalf("write MERGE_HEAD: %v", err)
			}
			return dir
		},
		Want: ErrRepoState,
	}, { // Test 3: Rebase in progress refuses.
		Setup: func(t *testing.T) string {
			t.Helper()
			dir := newRepoDir(t)
			if err := os.Mkdir(filepath.Join(dir, ".git", "rebase-merge"), 0o755); err != nil {
				t.Fatalf("mkdir rebase-merge: %v", err)
			}
			return dir
		},
		Want: ErrRepoState,
	}, { // Test 4: Subdirectory of a repository passes via dot-git detection.
		Setup: func(t *testing.T) string {
			t.Helper()
			dir := newRepoDir(t)
			sub := filepath.Join(dir, "internal", "deep")
			if err := os.MkdirAll(sub, 0o755); err != nil {
				t.Fatalf("mkdir sub: %v", err)
			}
			return sub
		},
	}}
	for testNum, test := range tests {
		t.Run(fmt.Sprintf("test %d", testNum), func(t *testing.T) {
			t.Parallel()
			got := checkRepoAt(test.Setup(t))
			if !errors.Is(got, test.Want) {
				t.Errorf("error mismatch: got %v, want %v", got, test.Want)
			}
		})
	}
}
