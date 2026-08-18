// Package sweep spots debris a working session leaves behind: stray debug
// prints, commented-out code, and scratch markers.
//
// It only ever reports. Deciding that a line is debris is a judgment about
// intent, and a tool that guesses wrong and deletes is worse than one that
// points and asks, so nothing here removes anything.
package sweep

import (
	"path"
	"regexp"
	"strings"

	"github.com/dcadolph/preen/repo"
)

// Finding is one suspected piece of debris in the diff.
type Finding struct {
	// Path is the file it was found in.
	Path string
	// Line is the line number in the post-image.
	Line int
	// Text is the offending line, trimmed.
	Text string
	// Rule names what matched, so the report says why.
	Rule string
}

// String renders a finding for a report.
func (f Finding) String() string {
	return f.Path + ":" + itoa(f.Line) + "  " + f.Rule + "  " + f.Text
}

// rule is a debris pattern and the languages it applies to.
type rule struct {
	// name describes what the pattern catches.
	name string
	// re matches the offending line.
	re *regexp.Regexp
	// exts limits the rule to certain file extensions, empty for all.
	exts []string
}

// rules are the debris patterns. They deliberately match only obvious cases:
// a false positive costs the user attention on every run, so the bar is a line
// nobody would defend in review.
//
//nolint:gochecknoglobals // Compiled once, never modified.
var rules = []rule{
	{
		name: "debug print",
		re:   regexp.MustCompile(`^\s*(fmt\.Print(f|ln)?|println|spew\.Dump)\s*\(`),
		exts: []string{".go"},
	},
	{
		name: "debug print",
		re:   regexp.MustCompile(`^\s*(console\.(log|debug|dir)|debugger)\b`),
		exts: []string{".js", ".ts", ".tsx", ".jsx"},
	},
	{
		name: "debug print",
		re:   regexp.MustCompile(`^\s*(print|pprint)\s*\(|^\s*breakpoint\s*\(`),
		exts: []string{".py"},
	},
	{
		name: "scratch marker",
		re:   regexp.MustCompile(`(?i)\b(XXX|HACK|FIXME|DO NOT (COMMIT|MERGE)|REMOVE ME)\b`),
	},
	{
		name: "commented-out code",
		re:   regexp.MustCompile(`^\s*(//|#)\s*(if|for|func|return|import|def|class|var|let|const)\b`),
	},
	{
		name: "skipped test",
		re:   regexp.MustCompile(`^\s*(t\.Skip\(|it\.only\(|describe\.only\(|@pytest\.mark\.skip)`),
	},
}

// Scan reports the debris among the lines a diff adds. Only added lines are
// considered, since preen is looking at what this session introduced rather
// than auditing the whole file.
func Scan(diffs []repo.FileDiff) []Finding {
	var findings []Finding
	for _, diff := range diffs {
		if diff.Binary {
			continue
		}
		ext := strings.ToLower(path.Ext(diff.Path))
		for _, hunk := range diff.Hunks {
			line := hunk.NewStart
			for _, text := range hunk.Lines {
				switch {
				case strings.HasPrefix(text, "+"):
					if found, name := match(text[1:], ext); found {
						findings = append(findings, Finding{
							Path: diff.Path,
							Line: line,
							Text: strings.TrimSpace(text[1:]),
							Rule: name,
						})
					}
					line++
				case strings.HasPrefix(text, "-"), strings.HasPrefix(text, `\`):
					// A removed line is not in the post-image and the no-newline
					// marker is not a line at all, so neither advances the count.
				default:
					line++
				}
			}
		}
	}
	return findings
}

// ScanFile reports the debris in a whole file's contents.
//
// An untracked file has no diff to read, so its every line is new and the file
// is scanned directly. Without this the most common case, debris in a file
// written this session, would go unseen.
func ScanFile(path, content string) []Finding {
	ext := strings.ToLower(pathExt(path))
	var findings []Finding
	for i, text := range strings.Split(content, "\n") {
		if found, name := match(text, ext); found {
			findings = append(findings, Finding{
				Path: path,
				Line: i + 1,
				Text: strings.TrimSpace(text),
				Rule: name,
			})
		}
	}
	return findings
}

// pathExt returns a path's extension, isolated so the scan of a whole file and
// the scan of a diff agree on how a file is classified.
func pathExt(p string) string { return path.Ext(p) }

// match reports whether a line is debris, and which rule caught it.
func match(line, ext string) (bool, string) {
	if strings.TrimSpace(line) == "" {
		return false, ""
	}
	for _, r := range rules {
		if len(r.exts) > 0 && !contains(r.exts, ext) {
			continue
		}
		if r.re.MatchString(line) {
			return true, r.name
		}
	}
	return false, ""
}

// contains reports whether the list holds the value.
func contains(list []string, value string) bool {
	for _, item := range list {
		if item == value {
			return true
		}
	}
	return false
}

// Paths returns the distinct files findings were found in, in the order they
// first appear.
func Paths(findings []Finding) []string {
	seen := make(map[string]bool, len(findings))
	var paths []string
	for _, finding := range findings {
		if seen[finding.Path] {
			continue
		}
		seen[finding.Path] = true
		paths = append(paths, finding.Path)
	}
	return paths
}

// itoa renders a line number without pulling in a formatting dependency for
// one integer.
func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var digits []byte
	for n > 0 {
		digits = append([]byte{byte('0' + n%10)}, digits...)
		n /= 10
	}
	return string(digits)
}
