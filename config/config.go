// Package config reads a repository's .preen.toml, which holds the defaults a
// project wants every preen run to use.
//
// Precedence is the usual one: a flag on the invocation beats the file, and
// the file beats the built-in defaults. Consent to rewrite published history
// is deliberately not settable here, since that has to be granted per run.
package config

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"time"

	"github.com/BurntSushi/toml"
	"github.com/dcadolph/preen/style"
)

// FileName is the config file preen looks for at the repository root.
const FileName = ".preen.toml"

// Config is a repository's preen defaults.
type Config struct {
	// Commit holds the message style defaults.
	Commit CommitSection `toml:"commit"`
	// Run holds the behavior defaults.
	Run RunSection `toml:"run"`
	// Protect names branches that must never be rewritten.
	Protect ProtectSection `toml:"protect"`
}

// CommitSection is the message style expressed in file form, using the same
// names as the flags without their leading dashes.
type CommitSection struct {
	// NoEmDash forbids em and en dashes.
	NoEmDash bool `toml:"no-emdash"`
	// NoSemicolon forbids semicolons.
	NoSemicolon bool `toml:"no-semicolon"`
	// NoHyphen forbids hyphens.
	NoHyphen bool `toml:"no-hyphen"`
	// MaxSubject caps the subject length.
	MaxSubject int `toml:"max-subject"`
	// Punctuation is auto, always, or never.
	Punctuation string `toml:"punctuation"`
	// LowerSubject lowercases the subject's first letter.
	LowerSubject bool `toml:"lower-subject"`
	// Conventional shapes subjects as Conventional Commits.
	Conventional bool `toml:"conventional"`
	// Prefix is prepended to every subject.
	Prefix string `toml:"prefix"`
	// SignOff adds a Signed-off-by trailer.
	SignOff bool `toml:"sign-off"`
	// Body is auto, always, or never.
	Body string `toml:"body"`
	// IncludeFiles lists the touched paths in each body.
	IncludeFiles bool `toml:"include-files"`
	// IncludeLineNumbers cites changed line ranges in each body.
	IncludeLineNumbers bool `toml:"include-line-numbers"`
}

// RunSection is the behavior defaults.
type RunSection struct {
	// Gate is a command run after each commit.
	Gate string `toml:"gate"`
	// Spread is a duration like "2h" that spaces commit timestamps.
	Spread string `toml:"spread"`
	// AllowNoVerify is standing consent to bypass commit hooks, written into
	// the repository ahead of time so a hook-blocked project can still preen.
	AllowNoVerify bool `toml:"allow-no-verify"`
	// Sweep reports debug prints and other leftovers on every run.
	Sweep bool `toml:"sweep"`
}

// ProtectSection names branches preen must refuse to rewrite.
type ProtectSection struct {
	// Branches are branch names or glob patterns.
	Branches []string `toml:"branches"`
}

// Load reads the config from the repository root. A missing file is not an
// error: it returns the zero config, which is the built-in default.
func Load(root string) (Config, error) {
	path := filepath.Join(root, FileName)
	data, err := os.ReadFile(path) //nolint:gosec // The path is the repository's own config.
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return Config{}, nil
		}
		return Config{}, fmt.Errorf("%w: %s: %w", ErrRead, path, err)
	}
	var cfg Config
	if err := toml.Unmarshal(data, &cfg); err != nil {
		return Config{}, fmt.Errorf("%w: %s: %w", ErrParse, path, err)
	}
	if err := cfg.validate(); err != nil {
		return Config{}, fmt.Errorf("%s: %w", path, err)
	}
	return cfg, nil
}

// validate rejects values the file can hold but preen cannot act on, so a typo
// surfaces at startup rather than as surprising behavior later.
func (c Config) validate() error {
	switch style.Punctuation(c.Commit.Punctuation) {
	case "", style.PunctAuto, style.PunctAlways, style.PunctNever:
	default:
		return fmt.Errorf("%w: punctuation %q must be auto, always, or never",
			ErrParse, c.Commit.Punctuation)
	}
	if c.Commit.MaxSubject < 0 {
		return fmt.Errorf("%w: max-subject cannot be negative", ErrParse)
	}
	switch style.BodyMode(c.Commit.Body) {
	case "", style.BodyAuto, style.BodyAlways, style.BodyNever:
	default:
		return fmt.Errorf("%w: body %q must be auto, always, or never", ErrParse, c.Commit.Body)
	}
	if c.Run.Spread != "" && c.Run.Spread != "auto" {
		if _, err := time.ParseDuration(c.Run.Spread); err != nil {
			return fmt.Errorf("%w: spread %q: %w", ErrParse, c.Run.Spread, err)
		}
	}
	return nil
}

// Style returns the message style the file configures.
func (c Config) Style() style.Style {
	return style.Style{
		NoEmDash:     c.Commit.NoEmDash,
		NoSemicolon:  c.Commit.NoSemicolon,
		NoHyphen:     c.Commit.NoHyphen,
		MaxSubject:   c.Commit.MaxSubject,
		Punctuation:  style.Punctuation(c.Commit.Punctuation),
		LowerSubject: c.Commit.LowerSubject,
		Conventional: c.Commit.Conventional,
		Prefix:       c.Commit.Prefix,
		SignOff:      c.Commit.SignOff,
		Body:         style.BodyMode(c.Commit.Body),
	}
}

// SpreadAuto reports whether the config asks for an automatically sized spread
// window rather than a fixed one.
func (c Config) SpreadAuto() bool { return c.Run.Spread == "auto" }

// SpreadWindow returns the configured spread duration, or zero when unset.
func (c Config) SpreadWindow() time.Duration {
	if c.Run.Spread == "" || c.Run.Spread == "auto" {
		return 0
	}
	// The value was validated at load time, so a parse failure here cannot
	// happen and a zero window simply means no spreading.
	window, err := time.ParseDuration(c.Run.Spread)
	if err != nil {
		return 0
	}
	return window
}
