package style

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/dcadolph/preen/plan"
)

// TestApply checks that each style rule reshapes a message the way it says,
// and that applying a style always leaves a message its own Verify accepts.
func TestApply(t *testing.T) {
	t.Parallel()

	tests := []struct {
		Name        string
		Style       Style
		In          plan.Commit
		WantSubject string
		WantBody    string
	}{
		{ // Test 0: An em dash becomes a comma rather than vanishing.
			Name:        "no em dash",
			Style:       Style{NoEmDash: true},
			In:          plan.Commit{Subject: "Add the parser—finally"},
			WantSubject: "Add the parser, finally",
		},
		{ // Test 1: A semicolon becomes a period.
			Name:        "no semicolon",
			Style:       Style{NoSemicolon: true},
			In:          plan.Commit{Subject: "Add the parser; it works"},
			WantSubject: "Add the parser. it works",
		},
		{ // Test 2: A hyphen becomes a space.
			Name:        "no hyphen",
			Style:       Style{NoHyphen: true},
			In:          plan.Commit{Subject: "Add the well-known parser"},
			WantSubject: "Add the well known parser",
		},
		{ // Test 3: A long subject is cut at a word boundary.
			Name:        "subject capped",
			Style:       Style{MaxSubject: 20},
			In:          plan.Commit{Subject: "Add the parser and the lexer and the evaluator"},
			WantSubject: "Add the parser and",
		},
		{ // Test 4: A terminal period is added when the style demands one.
			Name:        "punctuation always",
			Style:       Style{Punctuation: PunctAlways},
			In:          plan.Commit{Subject: "Add the parser"},
			WantSubject: "Add the parser.",
		},
		{ // Test 5: A terminal period is removed when the style forbids one.
			Name:        "punctuation never",
			Style:       Style{Punctuation: PunctNever},
			In:          plan.Commit{Subject: "Add the parser."},
			WantSubject: "Add the parser",
		},
		{ // Test 6: A prefix is prepended once.
			Name:        "prefix",
			Style:       Style{Prefix: "ABC-123"},
			In:          plan.Commit{Subject: "Add the parser"},
			WantSubject: "ABC-123 Add the parser",
		},
		{ // Test 7: Conventional Commits infer a type from the verb.
			Name:        "conventional add",
			Style:       Style{Conventional: true},
			In:          plan.Commit{Subject: "Add the parser"},
			WantSubject: "feat: add the parser",
		},
		{ // Test 8: A dependency change becomes a build type.
			Name:        "conventional dependencies",
			Style:       Style{Conventional: true},
			In:          plan.Commit{Subject: "Update dependencies"},
			WantSubject: "build: update dependencies",
		},
		{ // Test 9: An existing type is left alone.
			Name:        "conventional already typed",
			Style:       Style{Conventional: true},
			In:          plan.Commit{Subject: "fix: handle nil"},
			WantSubject: "fix: handle nil",
		},
		{ // Test 10: The subject can be lowercased.
			Name:        "lower subject",
			Style:       Style{LowerSubject: true},
			In:          plan.Commit{Subject: "Add the parser"},
			WantSubject: "add the parser",
		},
		{ // Test 11: A sign-off lands as a trailer.
			Name:        "sign off",
			Style:       Style{SignOff: true, Signer: "Dev <dev@example.com>"},
			In:          plan.Commit{Subject: "Add the parser", Body: "Why it matters."},
			WantSubject: "Add the parser",
			WantBody:    "Why it matters.\n\nSigned-off-by: Dev <dev@example.com>",
		},
	}

	for testNum, test := range tests {
		t.Run(fmt.Sprintf("test %d %s", testNum, test.Name), func(t *testing.T) {
			t.Parallel()
			got := test.Style.Apply(test.In)
			if got.Subject != test.WantSubject {
				t.Errorf("subject = %q, want %q", got.Subject, test.WantSubject)
			}
			if test.WantBody != "" && got.Body != test.WantBody {
				t.Errorf("body = %q, want %q", got.Body, test.WantBody)
			}
			// Applying a style must always satisfy that same style.
			if err := test.Style.Verify(got); err != nil {
				t.Errorf("Apply produced a message its own Verify rejects: %v", err)
			}
		})
	}
}

// TestVerify checks that the style catches the violations it is configured to
// care about, and stays quiet otherwise.
func TestVerify(t *testing.T) {
	t.Parallel()

	tests := []struct {
		Name  string
		Style Style
		In    plan.Commit
		Want  error
	}{
		{Name: "clean", Style: Style{}, In: plan.Commit{Subject: "Add the parser"}, Want: nil},
		{Name: "empty subject", Style: Style{}, In: plan.Commit{Subject: "  "}, Want: ErrStyle},
		{
			Name:  "over the cap",
			Style: Style{MaxSubject: 10},
			In:    plan.Commit{Subject: "Add the parser and the lexer"},
			Want:  ErrStyle,
		},
		{
			Name:  "banned em dash in body",
			Style: Style{NoEmDash: true},
			In:    plan.Commit{Subject: "Add the parser", Body: "It works—mostly."},
			Want:  ErrStyle,
		},
		{
			Name:  "missing prefix",
			Style: Style{Prefix: "ABC-123"},
			In:    plan.Commit{Subject: "Add the parser"},
			Want:  ErrStyle,
		},
		{
			Name:  "missing period",
			Style: Style{Punctuation: PunctAlways},
			In:    plan.Commit{Subject: "Add the parser"},
			Want:  ErrStyle,
		},
	}

	for testNum, test := range tests {
		t.Run(fmt.Sprintf("test %d %s", testNum, test.Name), func(t *testing.T) {
			t.Parallel()
			err := test.Style.Verify(test.In)
			if !errors.Is(err, test.Want) {
				t.Errorf("test %d Verify() = %v, want %v", testNum, err, test.Want)
			}
		})
	}
}

// TestDefaultCapIsSeventyTwo checks the cap a style inherits when it sets
// none, since that is the width git tooling assumes.
func TestDefaultCapIsSeventyTwo(t *testing.T) {
	t.Parallel()
	long := plan.Commit{Subject: strings.Repeat("word ", 30)}
	got := Style{}.Apply(long)
	if n := len([]rune(got.Subject)); n > DefaultMaxSubject {
		t.Errorf("subject is %d characters, want it capped at %d", n, DefaultMaxSubject)
	}
}
