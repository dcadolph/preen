package repo

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// ContentTree returns the hash of a tree holding HEAD plus every staged,
// unstaged, and untracked change, which is the total content preen is
// responsible for conserving.
//
// preen only reshapes history, never content, so this hash must be identical
// before and after a run. It is computed on a scratch index so the real index
// is never disturbed, which means it is safe to call at any point in a run.
// Ignored files are excluded, matching what a commit would ever capture.
func (r *Repo) ContentTree(ctx context.Context) (string, error) {
	dir, err := os.MkdirTemp("", "preen-index-")
	if err != nil {
		return "", fmt.Errorf("scratch index: %w", err)
	}
	defer func() { _ = os.RemoveAll(dir) }()

	env := []string{"GIT_INDEX_FILE=" + filepath.Join(dir, "index")}
	// An unborn HEAD has no tree to seed the index with, and the empty scratch
	// index is already the right starting point there.
	if r.HasCommits(ctx) {
		if _, err := r.runEnv(ctx, env, "read-tree", "HEAD"); err != nil {
			return "", fmt.Errorf("scratch read-tree: %w", err)
		}
	}
	if _, err := r.runEnv(ctx, env, "add", "-A"); err != nil {
		return "", fmt.Errorf("scratch add: %w", err)
	}
	hash, err := r.runEnv(ctx, env, "write-tree")
	if err != nil {
		return "", fmt.Errorf("scratch write-tree: %w", err)
	}
	if hash == "" {
		return "", fmt.Errorf("%w: write-tree returned nothing", ErrParse)
	}
	return hash, nil
}

// TreeDiffPaths returns the paths whose content differs between two trees. An
// empty result means the trees are identical.
func (r *Repo) TreeDiffPaths(ctx context.Context, a, b string) ([]string, error) {
	if a == b {
		return nil, nil
	}
	out, err := r.run(ctx, "diff", "--name-only", a, b)
	if err != nil {
		return nil, err
	}
	return splitLines(out), nil
}

// splitLines splits git output into non-empty lines.
func splitLines(out string) []string {
	if strings.TrimSpace(out) == "" {
		return nil
	}
	lines := strings.Split(strings.TrimRight(out, "\n"), "\n")
	kept := make([]string, 0, len(lines))
	for _, line := range lines {
		if line != "" {
			kept = append(kept, line)
		}
	}
	return kept
}
