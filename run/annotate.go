package run

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/dcadolph/preen/plan"
	"github.com/dcadolph/preen/repo"
	"github.com/dcadolph/preen/style"
)

// annotate adds the body detail a run is configured to include, before the
// style is applied so the result is still capped and cleaned like any message.
func annotate(commit plan.Commit, diffs []repo.FileDiff, opts Options) plan.Commit {
	var sections []string
	if opts.IncludeLineNumbers {
		if cited := citeLines(commit, diffs); cited != "" {
			sections = append(sections, cited)
		}
	} else if opts.IncludeFiles || opts.Style.Body == style.BodyAlways {
		if listed := listFiles(commit); listed != "" {
			sections = append(sections, listed)
		}
	}
	if len(sections) == 0 {
		return commit
	}
	addition := strings.Join(sections, "\n")
	if strings.TrimSpace(commit.Body) == "" {
		commit.Body = addition
		return commit
	}
	commit.Body = strings.TrimRight(commit.Body, "\n") + "\n\n" + addition
	return commit
}

// listFiles renders the commit's paths, one per line.
func listFiles(commit plan.Commit) string {
	paths := commit.Paths()
	if len(paths) == 0 {
		return ""
	}
	sort.Strings(paths)
	lines := make([]string, 0, len(paths))
	for _, path := range paths {
		lines = append(lines, "- "+path)
	}
	return strings.Join(lines, "\n")
}

// citeLines renders each path with the line ranges the commit changes, read
// from the hunk headers of the real diff.
//
// A whole-file part cites every hunk of that file, since the commit takes them
// all; a partial part cites only the hunks it claims.
func citeLines(commit plan.Commit, diffs []repo.FileDiff) string {
	byPath := make(map[string]repo.FileDiff, len(diffs))
	for _, diff := range diffs {
		byPath[diff.Path] = diff
	}
	lines := make([]string, 0, len(commit.Parts))
	for _, part := range commit.Parts {
		diff, ok := byPath[part.Path]
		if !ok || len(diff.Hunks) == 0 {
			lines = append(lines, "- "+part.Path)
			continue
		}
		ranges := rangesFor(part, diff)
		if len(ranges) == 0 {
			lines = append(lines, "- "+part.Path)
			continue
		}
		lines = append(lines, fmt.Sprintf("- %s:%s", part.Path, strings.Join(ranges, ", ")))
	}
	sort.Strings(lines)
	return strings.Join(lines, "\n")
}

// rangesFor renders the post-image line ranges a part covers.
func rangesFor(part plan.Part, diff repo.FileDiff) []string {
	wanted := make(map[int]bool, len(part.Hunks))
	for _, hunk := range part.Hunks {
		wanted[hunk.Index] = true
	}
	ranges := make([]string, 0, len(diff.Hunks))
	for i, hunk := range diff.Hunks {
		if !part.Whole() && !wanted[i] {
			continue
		}
		if hunk.NewLines <= 1 {
			ranges = append(ranges, fmt.Sprintf("%d", hunk.NewStart))
			continue
		}
		ranges = append(ranges, fmt.Sprintf("%d-%d", hunk.NewStart, hunk.NewStart+hunk.NewLines-1))
	}
	return ranges
}

// resolvePunctuation turns the auto punctuation mode into a decision, by
// reading what the repository's own recent subjects do.
//
// Matching the existing convention is the point of auto: a project whose
// subjects end in periods should keep getting periods without anyone
// configuring it.
func (e *Engine) resolvePunctuation(ctx context.Context, mode style.Punctuation) style.Punctuation {
	if mode != style.PunctAuto {
		return mode
	}
	commits, err := e.Repo.Log(ctx, "-n50")
	if err != nil || len(commits) == 0 {
		return ""
	}
	var ended, counted int
	for _, commit := range commits {
		subject := strings.TrimSpace(commit.Subject)
		if subject == "" {
			continue
		}
		counted++
		if strings.HasSuffix(subject, ".") {
			ended++
		}
	}
	if counted == 0 {
		return ""
	}
	// A clear majority is treated as the convention; anything mixed is left
	// alone rather than imposing a rule the project does not actually follow.
	if ended*2 > counted {
		return style.PunctAlways
	}
	return style.PunctNever
}
