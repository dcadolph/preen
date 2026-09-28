package run

import (
	"context"
	"fmt"
	"os/exec"
	"path"
	"strings"

	"github.com/dcadolph/preen/generated"
	"github.com/dcadolph/preen/plan"
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

// holdGenerated splits surveyed changes into what a run may commit and what it
// holds back because the path holds generated output.
//
// Only an untracked path is ever held. A path git already tracks was committed
// deliberately at some point, and overruling that would break the repository
// that vendors one of these directories on purpose. Holding a path back does
// not drop it: it becomes a declared part of the plan, it is shown before
// approval, and it stays in the working tree, so the content the run conserves
// is unchanged either way.
func holdGenerated(changes []repo.Change, matcher generated.Matcher) ([]repo.Change, []plan.Held) {
	kept := make([]repo.Change, 0, len(changes))
	var held []plan.Held
	for _, change := range changes {
		rule, matched := matcher.Match(change.Path)
		if !matched || change.Kind != repo.KindUntracked {
			kept = append(kept, change)
			continue
		}
		held = append(held, plan.Held{
			Part:    plan.Part{Path: change.Path, From: change.From, Kind: change.Kind},
			Pattern: rule.Pattern,
			Why:     rule.Why,
		})
	}
	return kept, held
}

// generatedOnly reports a run with nothing left to commit once generated output
// was held back, which needs a .gitignore entry rather than a commit.
func generatedOnly(held []plan.Held) error {
	paths := make([]string, 0, len(held))
	for _, entry := range held {
		paths = append(paths, entry.Part.Path)
	}
	return fmt.Errorf("%w: %s", ErrAllGenerated, strings.Join(paths, ", "))
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
