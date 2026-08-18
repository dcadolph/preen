package repo

import (
	"context"
	"fmt"
	"strconv"
	"strings"
)

// Hunk is one @@ block of a unified diff, kept as raw lines so it can be
// written back out byte for byte.
type Hunk struct {
	// Header is the @@ line, including any trailing section heading.
	Header string
	// Lines are the body lines, each keeping its leading marker: a space for
	// context, plus or minus for a change, or a backslash for the no-newline
	// marker.
	Lines []string
	// OldStart is the first line the hunk covers in the pre-image.
	OldStart int
	// OldLines is how many pre-image lines the hunk covers.
	OldLines int
	// NewStart is the first line the hunk covers in the post-image.
	NewStart int
	// NewLines is how many post-image lines the hunk covers.
	NewLines int
}

// Text renders the hunk as patch text, header and body.
func (h Hunk) Text() string {
	var b strings.Builder
	b.WriteString(h.Header)
	b.WriteByte('\n')
	for _, line := range h.Lines {
		b.WriteString(line)
		b.WriteByte('\n')
	}
	return b.String()
}

// FileDiff is one file's patch: the git headers that identify it plus its
// hunks.
type FileDiff struct {
	// Path is the post-image path, relative to the repository root.
	Path string
	// From is the pre-image path, which differs only for a rename or copy.
	From string
	// Header holds every line from "diff --git" through the "+++" line, kept
	// verbatim so mode changes and rename metadata survive a rewrite.
	Header []string
	// Hunks are the file's @@ blocks, empty for a binary or metadata-only diff.
	Hunks []Hunk
	// Binary reports a patch git rendered as binary, which can never be split.
	Binary bool
}

// Splittable reports whether the file's changes can be divided across commits.
// Binary files and pure metadata changes go whole into one commit.
func (f FileDiff) Splittable() bool {
	return !f.Binary && len(f.Hunks) > 1
}

// Text renders the file's full patch with every hunk.
func (f FileDiff) Text() string {
	return f.TextWith(nil)
}

// TextWith renders the file's patch containing only the hunks at the given
// indexes, in their original order. A nil selection keeps every hunk.
//
// Hunk headers are emitted unchanged, so the counts describe the full diff
// rather than the subset. Apply the result with the recount option, which
// tells git to infer the counts from the body instead of trusting them.
func (f FileDiff) TextWith(selected []int) string {
	keep := make(map[int]bool, len(selected))
	for _, i := range selected {
		keep[i] = true
	}
	var b strings.Builder
	for _, line := range f.Header {
		b.WriteString(line)
		b.WriteByte('\n')
	}
	for i, hunk := range f.Hunks {
		if selected != nil && !keep[i] {
			continue
		}
		b.WriteString(hunk.Text())
	}
	return b.String()
}

// Diff returns the unstaged patch for the given paths, or for the whole tree
// when no paths are given.
func (r *Repo) Diff(ctx context.Context, paths ...string) ([]FileDiff, error) {
	return r.diff(ctx, false, paths)
}

// diff runs git diff and parses the result. Rename detection is off so every
// file carries its own complete patch, and path quoting is off so paths come
// back as raw bytes.
func (r *Repo) diff(ctx context.Context, staged bool, paths []string) ([]FileDiff, error) {
	args := []string{"-c", "core.quotePath=false", "diff", "--no-color", "--no-ext-diff", "--no-renames"}
	if staged {
		args = append(args, "--cached")
	}
	if len(paths) > 0 {
		args = append(args, "--")
		args = append(args, paths...)
	}
	out, err := r.raw(ctx, args...)
	if err != nil {
		return nil, err
	}
	return ParsePatch(string(out))
}

// ParsePatch splits unified diff text into per-file patches and their hunks.
func ParsePatch(text string) ([]FileDiff, error) {
	if strings.TrimSpace(text) == "" {
		return nil, nil
	}
	var (
		files   []FileDiff
		current *FileDiff
		inBody  bool
	)
	flush := func() {
		if current != nil {
			files = append(files, *current)
			current = nil
		}
	}
	for _, line := range strings.Split(strings.TrimRight(text, "\n"), "\n") {
		switch {
		case strings.HasPrefix(line, "diff --git "):
			flush()
			from, to, err := parseDiffGit(line)
			if err != nil {
				return nil, err
			}
			current = &FileDiff{Path: to, From: from, Header: []string{line}}
			inBody = false
		case current == nil:
			// Leading noise before the first file header is not a patch.
			continue
		case strings.HasPrefix(line, "@@"):
			hunk, err := parseHunkHeader(line)
			if err != nil {
				return nil, err
			}
			current.Hunks = append(current.Hunks, hunk)
			inBody = true
		case inBody:
			last := &current.Hunks[len(current.Hunks)-1]
			last.Lines = append(last.Lines, line)
		default:
			if strings.HasPrefix(line, "Binary files ") || strings.HasPrefix(line, "GIT binary patch") {
				current.Binary = true
			}
			current.Header = append(current.Header, line)
		}
	}
	flush()
	return files, nil
}

// parseDiffGit extracts the pre-image and post-image paths from a "diff --git"
// line, trimming the a/ and b/ prefixes git adds.
func parseDiffGit(line string) (from, to string, err error) {
	rest := strings.TrimPrefix(line, "diff --git ")
	// Paths are space separated, and a path containing a space makes the split
	// ambiguous, so the halves are recovered from the a/ and b/ markers.
	sep := strings.Index(rest, " b/")
	if !strings.HasPrefix(rest, "a/") || sep < 0 {
		return "", "", fmt.Errorf("%w: diff header %q", ErrParse, line)
	}
	return rest[len("a/"):sep], rest[sep+len(" b/"):], nil
}

// parseHunkHeader reads the line ranges out of an @@ line.
func parseHunkHeader(line string) (Hunk, error) {
	end := strings.Index(line[2:], "@@")
	if end < 0 {
		return Hunk{}, fmt.Errorf("%w: hunk header %q", ErrParse, line)
	}
	ranges := strings.Fields(line[2 : 2+end])
	if len(ranges) != 2 || !strings.HasPrefix(ranges[0], "-") || !strings.HasPrefix(ranges[1], "+") {
		return Hunk{}, fmt.Errorf("%w: hunk ranges %q", ErrParse, line)
	}
	oldStart, oldLines, err := parseRange(ranges[0][1:])
	if err != nil {
		return Hunk{}, fmt.Errorf("%w: hunk header %q: %w", ErrParse, line, err)
	}
	newStart, newLines, err := parseRange(ranges[1][1:])
	if err != nil {
		return Hunk{}, fmt.Errorf("%w: hunk header %q: %w", ErrParse, line, err)
	}
	return Hunk{
		Header:   line,
		OldStart: oldStart,
		OldLines: oldLines,
		NewStart: newStart,
		NewLines: newLines,
	}, nil
}

// parseRange reads a "start,count" pair, where a missing count means one line.
func parseRange(s string) (start, count int, err error) {
	count = 1
	if comma := strings.Index(s, ","); comma >= 0 {
		if count, err = strconv.Atoi(s[comma+1:]); err != nil {
			return 0, 0, err
		}
		s = s[:comma]
	}
	if start, err = strconv.Atoi(s); err != nil {
		return 0, 0, err
	}
	return start, count, nil
}
