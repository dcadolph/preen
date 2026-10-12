package run

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/dcadolph/preen/v2/plan"
	"github.com/dcadolph/preen/v2/repo"
)

// fileState is what a file on disk looks like, read without git.
type fileState struct {
	// Kind is "file", "symlink", or "dir".
	Kind string
	// Exec reports whether any execute bit is set on a regular file.
	Exec bool
	// Data is the file content, or the link target for a symlink.
	Data string
}

// diskSnapshot records every path under the working tree except .git. It reads
// the disk directly so it cannot share a bug with the tree hash preen checks.
func (h *harness) diskSnapshot() map[string]fileState {
	h.T.Helper()
	out := make(map[string]fileState)
	err := filepath.WalkDir(h.Dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(h.Dir, path)
		if err != nil {
			return err
		}
		if rel == "." || rel == ".git" {
			if rel == ".git" {
				return filepath.SkipDir
			}
			return nil
		}
		info, err := os.Lstat(path)
		if err != nil {
			return err
		}
		switch {
		case info.Mode()&os.ModeSymlink != 0:
			target, err := os.Readlink(path)
			if err != nil {
				return err
			}
			out[rel] = fileState{Kind: "symlink", Data: target}
		case info.IsDir():
			out[rel] = fileState{Kind: "dir"}
		default:
			data, err := os.ReadFile(path) //nolint:gosec // Test fixture path.
			if err != nil {
				return err
			}
			out[rel] = fileState{Kind: "file", Exec: info.Mode()&0o111 != 0, Data: string(data)}
		}
		return nil
	})
	if err != nil {
		h.T.Fatalf("snapshot: %v", err)
	}
	return out
}

// symlink creates a symbolic link in the working tree.
func (h *harness) symlink(target, path string) {
	h.T.Helper()
	if runtime.GOOS == "windows" {
		h.T.Skip("symlinks need privileges on Windows")
	}
	if err := os.Symlink(target, filepath.Join(h.Dir, path)); err != nil {
		h.T.Fatalf("symlink: %v", err)
	}
}

// commitAll commits everything currently in the working tree.
func (h *harness) commitAll(message string) {
	h.T.Helper()
	h.git("add", "-A")
	h.git("commit", "-q", "-m", message)
}

// requireConserved applies a plan and checks the promise from the outside: the
// hash preen reports matches, the disk is byte-identical to before, and the
// tree is clean because everything was committed.
func (h *harness) requireConserved(opts Options) *Result {
	h.T.Helper()
	ctx := context.Background()
	before := h.diskSnapshot()
	p, err := h.Plan(ctx, opts)
	if err != nil {
		h.T.Fatalf("Plan: %v", err)
	}
	result, err := h.Apply(ctx, p, opts)
	if err != nil {
		h.T.Fatalf("Apply: %v", err)
	}
	if result.TreeStart != result.TreeEnd {
		h.T.Errorf("tree hash moved: %s to %s", result.TreeStart, result.TreeEnd)
	}
	if diff := cmp.Diff(before, h.diskSnapshot()); diff != "" {
		h.T.Errorf("disk changed (-before +after):\n%s", diff)
	}
	if status := h.git("status", "--porcelain=v1"); strings.TrimSpace(status) != "" {
		h.T.Errorf("tree not clean after the run:\n%s", status)
	}
	return result
}

// requireRolledBack checks a failed run from the outside: HEAD is where it
// started, the disk is byte-identical, and every change is still uncommitted.
func (h *harness) requireRolledBack(headBefore string, diskBefore map[string]fileState) {
	h.T.Helper()
	if head := strings.TrimSpace(h.git("rev-parse", "HEAD")); head != headBefore {
		h.T.Errorf("HEAD moved: %s to %s", headBefore, head)
	}
	if diff := cmp.Diff(diskBefore, h.diskSnapshot()); diff != "" {
		h.T.Errorf("disk changed after rollback (-before +after):\n%s", diff)
	}
	if status := h.git("status", "--porcelain=v1"); strings.TrimSpace(status) == "" {
		h.T.Error("changes were committed or lost, want them uncommitted")
	}
}

// TestConservesStagedAndUnstagedEditsToOneFile checks a file with a staged edit
// and a further unstaged edit on top, the state `git add -p` leaves behind.
func TestConservesStagedAndUnstagedEditsToOneFile(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	h.write("api/server.go", "package api\n\nfunc A() {}\n")
	h.commitAll("Add server")
	h.write("api/server.go", "package api\n\nfunc A() {}\n\nfunc B() {}\n")
	h.git("add", "api/server.go")
	h.write("api/server.go", "package api\n\nfunc A() {}\n\nfunc B() {}\n\nfunc C() {}\n")

	h.requireConserved(Options{})
}

// TestConservesSymlinks checks a new symlink, a retargeted one, and a link to a
// path that does not exist. Content is the link target, never the file behind it.
func TestConservesSymlinks(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	h.write("docs/guide.md", "# guide\n")
	h.symlink("guide.md", "docs/current.md")
	h.commitAll("Add docs")
	if err := os.Remove(filepath.Join(h.Dir, "docs/current.md")); err != nil {
		t.Fatalf("remove: %v", err)
	}
	h.symlink("README.md", "docs/current.md")
	h.symlink("missing.md", "docs/dangling.md")
	h.write("api/server.go", "package api\n")

	h.requireConserved(Options{})
}

// TestConservesModeChanges checks that setting and clearing the execute bit,
// with no content change, survives the run.
func TestConservesModeChanges(t *testing.T) {
	t.Parallel()
	if runtime.GOOS == "windows" {
		t.Skip("execute bits are not tracked on Windows")
	}
	h := newHarness(t)
	h.write("scripts/build.sh", "#!/bin/sh\necho build\n")
	h.write("scripts/old.sh", "#!/bin/sh\necho old\n")
	if err := os.Chmod(filepath.Join(h.Dir, "scripts/old.sh"), 0o755); err != nil { //nolint:gosec // Test fixture.
		t.Fatalf("chmod: %v", err)
	}
	h.commitAll("Add scripts")
	if err := os.Chmod(filepath.Join(h.Dir, "scripts/build.sh"), 0o755); err != nil { //nolint:gosec // Test fixture.
		t.Fatalf("chmod: %v", err)
	}
	if err := os.Chmod(filepath.Join(h.Dir, "scripts/old.sh"), 0o644); err != nil { //nolint:gosec // Test fixture.
		t.Fatalf("chmod: %v", err)
	}
	h.write("api/server.go", "package api\n")

	h.requireConserved(Options{})
}

// TestConservesAwkwardBytes checks content a line-oriented tool tends to
// mangle: CRLF endings, a missing final newline, a NUL byte, and an empty file.
func TestConservesAwkwardBytes(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	h.write("docs/windows.md", "line one\r\nline two\r\n")
	h.write("docs/no-newline.md", "ends without a newline")
	h.write("store/blob.dat", "head\x00tail\x00\xff\xfe")
	h.write("store/empty.go", "")
	h.write("api/server.go", "package api\n")

	h.requireConserved(Options{})
}

// TestConservesIgnoredFilesUntouched checks that an ignored file is neither
// committed nor altered, since it is outside what preen answers for.
func TestConservesIgnoredFilesUntouched(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	h := newHarness(t)
	h.write(".gitignore", "secrets.env\n")
	h.write("secrets.env", "TOKEN=abc\n")
	h.write("api/server.go", "package api\n")
	before := h.diskSnapshot()

	p, err := h.Plan(ctx, Options{})
	if err != nil {
		t.Fatalf("Plan: %v", err)
	}
	if _, err := h.Apply(ctx, p, Options{}); err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if diff := cmp.Diff(before, h.diskSnapshot()); diff != "" {
		t.Errorf("disk changed (-before +after):\n%s", diff)
	}
	if tracked := h.git("ls-files", "secrets.env"); strings.TrimSpace(tracked) != "" {
		t.Errorf("ignored file was committed: %q", tracked)
	}
}

// TestFailureOnLaterCommitRestoresEverything checks a run that fails after
// several commits already landed: the disk and HEAD return to the start and a
// backup branch exists to prove it.
func TestFailureOnLaterCommitRestoresEverything(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	h := newHarness(t)
	h.messyTree()
	h.symlink("guide.md", "docs/current.md")
	head := strings.TrimSpace(h.git("rev-parse", "HEAD"))
	disk := h.diskSnapshot()

	calls := 0
	h.Gate = GateFunc(func(context.Context, string, string) ([]byte, error) {
		calls++
		if calls == 3 {
			return []byte("boom"), errors.New("check failed")
		}
		return nil, nil
	})
	p, err := h.Plan(ctx, Options{})
	if err != nil {
		t.Fatalf("Plan: %v", err)
	}
	if len(p.Commits) < 4 {
		t.Fatalf("plan has %d commits, want at least 4 so the failure lands mid-run", len(p.Commits))
	}
	if _, err := h.Apply(ctx, p, Options{Gate: "check"}); !errors.Is(err, ErrGateFailed) {
		t.Fatalf("Apply = %v, want a gate failure", err)
	}
	h.requireRolledBack(head, disk)
	if branches := h.git("branch", "--list", repo.BackupPrefix+"*"); strings.TrimSpace(branches) == "" {
		t.Error("no backup branch left behind")
	}
}

// TestRejectingHookRestoresEverything checks that a pre-commit hook refusing
// the third commit takes the run back with the disk untouched.
func TestRejectingHookRestoresEverything(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	h := newHarness(t)
	h.messyTree()
	head := strings.TrimSpace(h.git("rev-parse", "HEAD"))
	disk := h.diskSnapshot()
	counter := filepath.Join(t.TempDir(), "count")
	h.installHook("pre-commit", "#!/bin/sh\necho x >> "+counter+"\n[ \"$(wc -l < "+counter+")\" -lt 3 ]\n")

	p, err := h.Plan(ctx, Options{})
	if err != nil {
		t.Fatalf("Plan: %v", err)
	}
	if _, err := h.Apply(ctx, p, Options{}); err == nil {
		t.Fatal("Apply succeeded despite a hook that rejects")
	}
	h.requireRolledBack(head, disk)
}

// TestRefusesWhileMergeIsInProgress checks that a conflicted merge is left
// alone: preen refuses, and nothing about the tree or the merge state changes.
func TestRefusesWhileMergeIsInProgress(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	h := newHarness(t)
	h.write("api/server.go", "package api\n\nfunc A() {}\n")
	h.commitAll("Add server")
	h.git("checkout", "-q", "-b", "side")
	h.write("api/server.go", "package api\n\nfunc Side() {}\n")
	h.commitAll("Side change")
	h.git("checkout", "-q", "main")
	h.write("api/server.go", "package api\n\nfunc Main() {}\n")
	h.commitAll("Main change")
	merge := exec.Command("git", "merge", "side")
	merge.Dir = h.Dir
	if err := merge.Run(); err == nil {
		t.Fatal("merge did not conflict")
	}
	disk := h.diskSnapshot()
	status := h.git("status", "--porcelain=v1")

	if _, err := h.Plan(ctx, Options{}); !errors.Is(err, repo.ErrInProgress) {
		t.Fatalf("Plan = %v, want ErrInProgress", err)
	}
	if _, err := h.Apply(ctx, &plan.Plan{}, Options{}); !errors.Is(err, repo.ErrInProgress) {
		t.Errorf("Apply = %v, want ErrInProgress", err)
	}
	if diff := cmp.Diff(disk, h.diskSnapshot()); diff != "" {
		t.Errorf("disk changed (-before +after):\n%s", diff)
	}
	if after := h.git("status", "--porcelain=v1"); after != status {
		t.Errorf("status changed:\n%s\nto\n%s", status, after)
	}
}

// TestCanceledRunRestoresEverything checks a run interrupted partway, as by a
// signal that cancels the context. The rollback must still complete, since
// leaving half the commits made is the failure this tool exists to prevent.
func TestCanceledRunRestoresEverything(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	h.messyTree()
	head := strings.TrimSpace(h.git("rev-parse", "HEAD"))
	disk := h.diskSnapshot()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	calls := 0
	h.Gate = GateFunc(func(context.Context, string, string) ([]byte, error) {
		calls++
		if calls == 2 {
			cancel()
		}
		return nil, nil
	})
	p, err := h.Plan(context.Background(), Options{})
	if err != nil {
		t.Fatalf("Plan: %v", err)
	}
	if _, err := h.Apply(ctx, p, Options{Gate: "check"}); err == nil {
		t.Fatal("Apply succeeded despite cancellation")
	}
	h.requireRolledBack(head, disk)
}

// TestHardStopIsRecoverableFromTheBackup checks the case no rollback code can
// handle, a process that dies mid-run. A panic stands in for the kill: the
// commits made so far stay, and restoring the backup branch puts HEAD and the
// disk back exactly as they were.
func TestHardStopIsRecoverableFromTheBackup(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	h := newHarness(t)
	h.messyTree()
	head := strings.TrimSpace(h.git("rev-parse", "HEAD"))
	disk := h.diskSnapshot()

	calls := 0
	h.Gate = GateFunc(func(context.Context, string, string) ([]byte, error) {
		calls++
		if calls == 2 {
			panic("process died")
		}
		return nil, nil
	})
	p, err := h.Plan(ctx, Options{})
	if err != nil {
		t.Fatalf("Plan: %v", err)
	}
	func() {
		defer func() { _ = recover() }()
		_, _ = h.Apply(ctx, p, Options{Gate: "check"})
	}()
	if strings.TrimSpace(h.git("rev-parse", "HEAD")) == head {
		t.Fatal("the simulated death left no partial commits, so the test proves nothing")
	}
	backups, err := h.Repo.ListBackups(ctx)
	if err != nil || len(backups) != 1 {
		t.Fatalf("ListBackups = %v, %v, want exactly one backup", backups, err)
	}
	if err := h.Repo.RestoreBackup(ctx, backups[0].Name); err != nil {
		t.Fatalf("RestoreBackup: %v", err)
	}
	h.requireRolledBack(head, disk)
}

// TestRollbackRestoresWhatWasStaged checks that a failed run puts the index
// back as it was: the file staged by hand is staged again, and the edit made on
// top of it stays unstaged.
func TestRollbackRestoresWhatWasStaged(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	h := newHarness(t)
	h.messyTree()
	h.write("api/server.go", "package api\n\nfunc Serve() {}\n\nfunc More() {}\n")
	h.git("add", "api/server.go", "docs/guide.md")
	h.write("api/server.go", "package api\n\nfunc Serve() {}\n\nfunc More() {}\n\nfunc Extra() {}\n")
	statusBefore := h.git("status", "--porcelain=v1")

	h.Gate = GateFunc(func(context.Context, string, string) ([]byte, error) {
		return []byte("boom"), errors.New("check failed")
	})
	p, err := h.Plan(ctx, Options{})
	if err != nil {
		t.Fatalf("Plan: %v", err)
	}
	if _, err := h.Apply(ctx, p, Options{Gate: "check"}); !errors.Is(err, ErrGateFailed) {
		t.Fatalf("Apply = %v, want a gate failure", err)
	}
	if diff := cmp.Diff(statusBefore, h.git("status", "--porcelain=v1")); diff != "" {
		t.Errorf("staging not restored (-before +after):\n%s", diff)
	}
}

// TestRestoreBringsBackWhatWasStaged checks the same for the manual path,
// restoring from the backup branch after a run that died.
func TestRestoreBringsBackWhatWasStaged(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	h := newHarness(t)
	h.messyTree()
	h.git("add", "docs/guide.md")
	statusBefore := h.git("status", "--porcelain=v1")

	h.Gate = GateFunc(func(context.Context, string, string) ([]byte, error) { panic("process died") })
	p, err := h.Plan(ctx, Options{})
	if err != nil {
		t.Fatalf("Plan: %v", err)
	}
	func() {
		defer func() { _ = recover() }()
		_, _ = h.Apply(ctx, p, Options{Gate: "check"})
	}()
	backups, err := h.Repo.ListBackups(ctx)
	if err != nil || len(backups) != 1 {
		t.Fatalf("ListBackups = %v, %v, want exactly one backup", backups, err)
	}
	if err := h.Repo.RestoreBackup(ctx, backups[0].Name); err != nil {
		t.Fatalf("RestoreBackup: %v", err)
	}
	if diff := cmp.Diff(statusBefore, h.git("status", "--porcelain=v1")); diff != "" {
		t.Errorf("staging not restored (-before +after):\n%s", diff)
	}
	if err := h.Repo.DeleteBackup(ctx, backups[0].Name); err != nil {
		t.Fatalf("DeleteBackup: %v", err)
	}
	if refs := h.git("for-each-ref", "refs/preen-index/"); strings.TrimSpace(refs) != "" {
		t.Errorf("saved index outlived its backup:\n%s", refs)
	}
}
