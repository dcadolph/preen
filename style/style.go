// Package style shapes and checks commit messages.
//
// The rules are the ones people actually argue about in review: banned
// punctuation, subject length, whether a subject ends in a period. preen
// applies them to the messages it generates and verifies the result, so a
// configured style is enforced rather than merely requested.
package style

import (
	"fmt"
	"strings"
	"unicode"

	"github.com/dcadolph/preen/plan"
)

// BodyMode controls when a commit gets a message body.
type BodyMode string

// The body modes.
const (
	// BodyAuto keeps a body only when one was written.
	BodyAuto BodyMode = "auto"
	// BodyAlways keeps a body and falls back to naming the files touched.
	BodyAlways BodyMode = "always"
	// BodyNever drops the body, leaving a subject-only message.
	BodyNever BodyMode = "never"
)

// Punctuation controls terminal punctuation on a subject.
type Punctuation string

// The punctuation modes.
const (
	// PunctAuto leaves terminal punctuation as written.
	PunctAuto Punctuation = "auto"
	// PunctAlways ends the subject with a period.
	PunctAlways Punctuation = "always"
	// PunctNever strips terminal punctuation from the subject.
	PunctNever Punctuation = "never"
)

// Style is a commit message convention.
type Style struct {
	// NoEmDash forbids em and en dashes anywhere in a message.
	NoEmDash bool
	// NoSemicolon forbids semicolons.
	NoSemicolon bool
	// NoHyphen forbids hyphens, for repositories that spell compounds out.
	NoHyphen bool
	// MaxSubject caps the subject length. Zero means the default of 72.
	MaxSubject int
	// Punctuation controls the terminal punctuation on a subject.
	Punctuation Punctuation
	// LowerSubject lowercases the first letter of the subject.
	LowerSubject bool
	// Conventional shapes subjects as Conventional Commits, "type: subject".
	Conventional bool
	// Prefix is prepended to every subject, such as a ticket identifier.
	Prefix string
	// Body controls whether a message keeps a body.
	Body BodyMode
	// SignOff adds a Signed-off-by trailer built from Signer.
	SignOff bool
	// Signer is the identity used for the sign-off trailer.
	Signer string
}

// DefaultMaxSubject is the subject cap when a style does not set one. It is
// the width git's own tooling assumes.
const DefaultMaxSubject = 72

// cap returns the effective subject cap.
func (s Style) cap() int {
	if s.MaxSubject > 0 {
		return s.MaxSubject
	}
	return DefaultMaxSubject
}

// Apply returns the commit with its message shaped to the style.
func (s Style) Apply(commit plan.Commit) plan.Commit {
	subject := commit.Subject
	if s.Conventional {
		subject = conventional(subject)
	}
	if s.Prefix != "" && !strings.HasPrefix(subject, s.Prefix) {
		subject = s.Prefix + " " + subject
	}
	subject = s.clean(subject)
	if s.LowerSubject {
		subject = lowerFirst(subject)
	}
	switch s.Punctuation {
	case PunctAlways:
		subject = ensurePeriod(subject)
	case PunctNever:
		subject = strings.TrimRight(subject, ".!?")
	}
	subject = s.truncate(subject)

	body := s.clean(commit.Body)
	if s.Body == BodyNever {
		body = ""
	}
	if s.SignOff && s.Signer != "" {
		body = addTrailer(body, "Signed-off-by: "+s.Signer)
	}
	commit.Subject = subject
	commit.Body = body
	return commit
}

// Verify reports the ways a message breaks the style, so a message can be
// checked before it is recorded rather than discovered in review.
func (s Style) Verify(commit plan.Commit) error {
	var problems []string
	if strings.TrimSpace(commit.Subject) == "" {
		problems = append(problems, "the subject is empty")
	}
	if n := len([]rune(commit.Subject)); n > s.cap() {
		problems = append(problems, fmt.Sprintf("the subject is %d characters, over the cap of %d", n, s.cap()))
	}
	full := commit.Subject + "\n" + commit.Body
	for _, banned := range s.banned() {
		if strings.Contains(full, banned.text) {
			problems = append(problems, "the message contains "+banned.name)
		}
	}
	switch s.Punctuation {
	case PunctAlways:
		if !strings.HasSuffix(strings.TrimSpace(commit.Subject), ".") {
			problems = append(problems, "the subject does not end in a period")
		}
	case PunctNever:
		if strings.HasSuffix(strings.TrimSpace(commit.Subject), ".") {
			problems = append(problems, "the subject ends in a period")
		}
	}
	if s.Prefix != "" && !strings.HasPrefix(commit.Subject, s.Prefix) {
		problems = append(problems, "the subject is missing the prefix "+s.Prefix)
	}
	if len(problems) == 0 {
		return nil
	}
	return fmt.Errorf("%w: %q: %s", ErrStyle, commit.Subject, strings.Join(problems, "; "))
}

// bannedText is a character the style forbids, with a name for the report.
type bannedText struct {
	// text is the literal that must not appear.
	text string
	// name is how the character is described in an error.
	name string
}

// banned returns the characters this style forbids.
func (s Style) banned() []bannedText {
	var out []bannedText
	if s.NoEmDash {
		out = append(out, bannedText{"—", "an em dash"}, bannedText{"–", "an en dash"})
	}
	if s.NoSemicolon {
		out = append(out, bannedText{";", "a semicolon"})
	}
	if s.NoHyphen {
		out = append(out, bannedText{"-", "a hyphen"})
	}
	return out
}

// clean rewrites banned characters into their plain equivalents rather than
// deleting them, so the sentence still reads.
func (s Style) clean(text string) string {
	if text == "" {
		return text
	}
	if s.NoEmDash {
		text = strings.ReplaceAll(text, "—", ", ")
		text = strings.ReplaceAll(text, "–", "-")
	}
	if s.NoSemicolon {
		text = strings.ReplaceAll(text, "; ", ". ")
		text = strings.ReplaceAll(text, ";", ".")
	}
	if s.NoHyphen {
		text = strings.ReplaceAll(text, "-", " ")
	}
	return collapseSpaces(text)
}

// truncate shortens a subject to the cap, cutting at a word boundary so the
// result still reads as words rather than a severed one.
func (s Style) truncate(subject string) string {
	limit := s.cap()
	runes := []rune(subject)
	if len(runes) <= limit {
		return subject
	}
	cut := string(runes[:limit])
	if space := strings.LastIndex(cut, " "); space > limit/2 {
		cut = cut[:space]
	}
	return strings.TrimRight(cut, " ,.;:")
}

// conventional reshapes a subject into "type: subject", inferring the type
// from the verb preen chose when the subject does not already carry one.
func conventional(subject string) string {
	if at := strings.Index(subject, ":"); at > 0 && at < 20 && !strings.Contains(subject[:at], " ") {
		return subject
	}
	kind := "chore"
	switch {
	case strings.HasPrefix(subject, "Add"):
		kind = "feat"
	case strings.HasPrefix(subject, "Fix"):
		kind = "fix"
	case strings.HasPrefix(subject, "Remove"):
		kind = "refactor"
	case strings.Contains(strings.ToLower(subject), "document"):
		kind = "docs"
	case strings.Contains(strings.ToLower(subject), "dependenc"):
		kind = "build"
	}
	return kind + ": " + lowerFirst(subject)
}

// addTrailer appends a trailer to a body, separated by a blank line when the
// body has other content.
func addTrailer(body, trailer string) string {
	if strings.Contains(body, trailer) {
		return body
	}
	if strings.TrimSpace(body) == "" {
		return trailer
	}
	return strings.TrimRight(body, "\n") + "\n\n" + trailer
}

// ensurePeriod adds a terminal period when the subject lacks one.
func ensurePeriod(subject string) string {
	trimmed := strings.TrimRight(subject, " ")
	if trimmed == "" || strings.HasSuffix(trimmed, ".") ||
		strings.HasSuffix(trimmed, "!") || strings.HasSuffix(trimmed, "?") {
		return trimmed
	}
	return trimmed + "."
}

// lowerFirst lowercases the first letter, leaving the rest alone so an
// identifier keeps its capitals.
func lowerFirst(text string) string {
	if text == "" {
		return text
	}
	runes := []rune(text)
	runes[0] = unicode.ToLower(runes[0])
	return string(runes)
}

// collapseSpaces squeezes runs of spaces left by a replacement and trims the
// ends.
func collapseSpaces(text string) string {
	var b strings.Builder
	var lastSpace bool
	for _, r := range text {
		if r == ' ' {
			if lastSpace {
				continue
			}
			lastSpace = true
		} else {
			lastSpace = false
		}
		b.WriteRune(r)
	}
	return strings.TrimSpace(b.String())
}
