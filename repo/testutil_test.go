package repo

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// testRepo is a real git repository on disk for a single test. Operations run
// against the git binary, since preen's whole job is to match git's behavior.
type testRepo struct {
	// Repo is the opened repository.
	*Repo
	// Dir is the working tree root.
	Dir string
	// T is the owning test.
	T *testing.T
}

// newTestRepo initializes a repository in a temp directory with deterministic
// identity and no hooks, signing, or user configuration bleeding in.
func newTestRepo(t *testing.T) *testRepo {
	t.Helper()
	dir := t.TempDir()
	run := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		cmd.Env = append(os.Environ(),
			"GIT_CONFIG_GLOBAL=/dev/null",
			"GIT_CONFIG_SYSTEM=/dev/null",
			"GIT_AUTHOR_NAME=preen test",
			"GIT_AUTHOR_EMAIL=test@example.com",
			"GIT_COMMITTER_NAME=preen test",
			"GIT_COMMITTER_EMAIL=test@example.com",
		)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v: %s", args, err, out)
		}
	}
	run("init", "-b", "main")
	run("config", "user.name", "preen test")
	run("config", "user.email", "test@example.com")
	run("config", "commit.gpgsign", "false")
	run("config", "core.hooksPath", filepath.Join(dir, ".no-hooks"))

	r, err := Open(context.Background(), dir)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	return &testRepo{Repo: r, Dir: dir, T: t}
}

// write creates or replaces a file in the working tree, making parent
// directories as needed.
func (tr *testRepo) write(path, content string) {
	tr.T.Helper()
	full := filepath.Join(tr.Dir, path)
	if err := os.MkdirAll(filepath.Dir(full), 0o750); err != nil {
		tr.T.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(full, []byte(content), 0o600); err != nil {
		tr.T.Fatalf("write %s: %v", path, err)
	}
}

// remove deletes a file from the working tree.
func (tr *testRepo) remove(path string) {
	tr.T.Helper()
	if err := os.Remove(filepath.Join(tr.Dir, path)); err != nil {
		tr.T.Fatalf("remove %s: %v", path, err)
	}
}

// git runs a git command in the repository and fails the test on error.
func (tr *testRepo) git(args ...string) string {
	tr.T.Helper()
	out, err := tr.runner.Git(context.Background(), tr.Dir, nil, args...)
	if err != nil {
		tr.T.Fatalf("git %v: %v", args, err)
	}
	return string(out)
}

// commit stages everything and records a commit with the given message.
func (tr *testRepo) commit(message string) {
	tr.T.Helper()
	tr.git("add", "-A")
	tr.git("commit", "-m", message)
}
