package plan

import (
	"fmt"
	"io"
	"strings"
)

// Render writes the plan for approval. It leads with what will move, since the
// reset and the backup ref are what a reader needs to judge the risk, then
// lists the commits in the order they will be recorded.
func (p Plan) Render(w io.Writer) error {
	var b strings.Builder

	if p.Resets() {
		fmt.Fprintf(&b, "Reset to:     %s\n", shorten(p.Base))
		fmt.Fprintf(&b, "Backup ref:   %s\n", p.Backup)
		fmt.Fprintf(&b, "Merge check:  %s\n", p.MergeSummary)
		if len(p.Absorbed) > 0 {
			fmt.Fprintf(&b, "Redoing %d commit%s:\n", len(p.Absorbed), plural(len(p.Absorbed)))
			for _, commit := range p.Absorbed {
				fmt.Fprintf(&b, "  %s  %s\n", commit.Short(), commit.Subject)
			}
		}
		b.WriteByte('\n')
	}

	fmt.Fprintf(&b, "Planned commits (%d):\n\n", len(p.Commits))
	for i, commit := range p.Commits {
		fmt.Fprintf(&b, "%d. %s\n", i+1, commit.Subject)
		for _, line := range bodyLines(commit.Body) {
			fmt.Fprintf(&b, "   %s\n", line)
		}
		for _, part := range commit.Parts {
			fmt.Fprintf(&b, "     %s\n", part)
		}
		b.WriteByte('\n')
	}

	if len(p.Debris) > 0 {
		fmt.Fprintf(&b, "Possible leftovers (%d), not removed:\n", len(p.Debris))
		for _, line := range p.Debris {
			fmt.Fprintf(&b, "     %s\n", line)
		}
		b.WriteByte('\n')
	}

	if len(p.Leftover) > 0 {
		fmt.Fprintf(&b, "Left uncommitted (%d):\n", len(p.Leftover))
		for _, part := range p.Leftover {
			fmt.Fprintf(&b, "     %s\n", part)
		}
		b.WriteByte('\n')
	}

	_, err := io.WriteString(w, b.String())
	return err
}

// bodyLines splits a message body into display lines, dropping a blank body.
func bodyLines(body string) []string {
	if strings.TrimSpace(body) == "" {
		return nil
	}
	return strings.Split(strings.TrimRight(body, "\n"), "\n")
}

// shorten abbreviates a full hash for display and leaves anything else alone.
func shorten(rev string) string {
	if len(rev) == 40 {
		return rev[:8]
	}
	return rev
}
