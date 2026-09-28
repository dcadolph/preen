package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"

	"github.com/dcadolph/preen/style"
)

// writeConfig puts a .preen.toml in a temp directory and returns the root.
func writeConfig(t *testing.T, content string) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, FileName), []byte(content), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}
	return dir
}

// TestLoadMissingFile checks that a repository without a config is the normal
// case rather than an error.
func TestLoadMissingFile(t *testing.T) {
	t.Parallel()
	cfg, err := Load(t.TempDir())
	if err != nil {
		t.Fatalf("Load with no file: %v", err)
	}
	if cfg.Style() != (style.Style{}) {
		t.Errorf("missing config produced a non-zero style: %+v", cfg.Style())
	}
}

// TestLoadStyle checks that the file's names map onto the style the engine
// uses, since the file and the flags have to mean the same thing.
func TestLoadStyle(t *testing.T) {
	t.Parallel()
	root := writeConfig(t, `
[commit]
no-emdash = true
no-semicolon = true
max-subject = 50
punctuation = "never"
conventional = true
prefix = "ABC-123"
sign-off = true
`)
	cfg, err := Load(root)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	got := cfg.Style()
	want := style.Style{
		NoEmDash:     true,
		NoSemicolon:  true,
		MaxSubject:   50,
		Punctuation:  style.PunctNever,
		Conventional: true,
		Prefix:       "ABC-123",
		SignOff:      true,
	}
	if got != want {
		t.Errorf("Style() = %+v, want %+v", got, want)
	}
}

// TestLoadRunSection checks the behavior defaults, including the standing
// consent that lets a hook-blocked repository still be preened.
func TestLoadRunSection(t *testing.T) {
	t.Parallel()
	root := writeConfig(t, `
[run]
gate = "go test ./..."
allow-no-verify = true

[protect]
branches = ["develop", "release/*"]
`)
	cfg, err := Load(root)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Run.Gate != "go test ./..." {
		t.Errorf("gate = %q, want the configured command", cfg.Run.Gate)
	}
	if !cfg.Run.AllowNoVerify {
		t.Error("allow-no-verify did not survive the round trip")
	}
	if len(cfg.Protect.Branches) != 2 {
		t.Errorf("protected branches = %v, want two entries", cfg.Protect.Branches)
	}
}

// TestLoadGeneratedSection checks that a repository can extend and exempt the
// never-commit patterns, which is how a project that vendors one of those
// directories on purpose keeps committing it.
func TestLoadGeneratedSection(t *testing.T) {
	t.Parallel()
	root := writeConfig(t, `
[generated]
allow-all = false
patterns = ["*.snap"]
allow = ["third_party/node_modules"]
`)
	cfg, err := Load(root)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	want := GeneratedSection{
		Patterns: []string{"*.snap"},
		Allow:    []string{"third_party/node_modules"},
	}
	if diff := cmp.Diff(want, cfg.Generated, cmpopts.EquateEmpty()); diff != "" {
		t.Errorf("generated section mismatch (-want +got):\n%s", diff)
	}
}

// TestLoadRejectsBadValues checks that a typo surfaces at load time rather
// than as surprising behavior in the middle of a run.
func TestLoadRejectsBadValues(t *testing.T) {
	t.Parallel()

	tests := []struct {
		Name string
		In   string
		Want error
	}{
		{Name: "not toml", In: "this is not toml {{{", Want: ErrParse},
		{Name: "bad punctuation", In: "[commit]\npunctuation = \"sometimes\"\n", Want: ErrParse},
		{Name: "negative cap", In: "[commit]\nmax-subject = -1\n", Want: ErrParse},
	}

	for testNum, test := range tests {
		t.Run(fmt.Sprintf("test %d %s", testNum, test.Name), func(t *testing.T) {
			t.Parallel()
			_, err := Load(writeConfig(t, test.In))
			if !errors.Is(err, test.Want) {
				t.Errorf("Load() = %v, want %v", err, test.Want)
			}
		})
	}
}
