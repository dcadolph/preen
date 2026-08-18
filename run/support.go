package run

import (
	"context"
	"os/exec"
	"path"
	"strings"

	"github.com/dcadolph/preen/repo"
)

// shellGate runs a gate command through the shell, so a configured check can
// use pipes and operators the way it would in a terminal.
type shellGate struct{}

// Check runs the command in dir and returns its combined output.
func (shellGate) Check(ctx context.Context, dir, command string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, "sh", "-c", command)
	cmd.Dir = dir
	return cmd.CombinedOutput()
}

// mergeChanges combines the working tree status with the paths an absorb run
// is bringing back, keeping one record per path. The working tree entry wins,
// since it carries the staged and unstaged detail.
func mergeChanges(tree, absorbed []repo.Change) []repo.Change {
	seen := make(map[string]bool, len(tree))
	merged := make([]repo.Change, 0, len(tree)+len(absorbed))
	for _, change := range tree {
		seen[change.Path] = true
		merged = append(merged, change)
	}
	for _, change := range absorbed {
		if !seen[change.Path] {
			merged = append(merged, change)
		}
	}
	return merged
}

// inScope filters changes to those matching any of the pathspecs. An empty
// scope keeps everything.
//
// A pathspec matches a path outright, by directory prefix, or by a glob on the
// base name, which covers the shapes a caller reaches for without pulling in
// git's full pathspec grammar.
func inScope(changes []repo.Change, scope []string) []repo.Change {
	if len(scope) == 0 {
		return changes
	}
	kept := make([]repo.Change, 0, len(changes))
	for _, change := range changes {
		if matchesAny(change.Path, scope) {
			kept = append(kept, change)
		}
	}
	return kept
}

// matchesAny reports whether a path matches any pathspec.
func matchesAny(p string, scope []string) bool {
	for _, spec := range scope {
		if matches(p, spec) {
			return true
		}
	}
	return false
}

// matches reports whether one pathspec covers a path.
func matches(p, spec string) bool {
	spec = strings.TrimSuffix(spec, "/")
	if spec == "" || spec == "." {
		return true
	}
	if p == spec || strings.HasPrefix(p, spec+"/") {
		return true
	}
	if ok, err := path.Match(spec, p); err == nil && ok {
		return true
	}
	if ok, err := path.Match(spec, path.Base(p)); err == nil && ok {
		return true
	}
	return false
}
