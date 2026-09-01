package cmd

import (
	"strings"

	"github.com/dcadolph/preen/config"
	"github.com/dcadolph/preen/run"
	"github.com/dcadolph/preen/style"
)

// applyConfig fills in the options a repository's config file supplies,
// leaving anything the invocation set alone.
//
// Precedence runs flag, then file, then default. A flag is treated as set only
// when it actually appears in the arguments, so a config value is not silently
// overwritten by a flag's zero value.
func applyConfig(opts *run.Options, cfg config.Config, args []string) {
	fileStyle := cfg.Style()
	if !flagGiven(args, "gate") && cfg.Run.Gate != "" {
		opts.Gate = cfg.Run.Gate
	}
	if !flagGiven(args, "sweep") && cfg.Run.Sweep {
		opts.Sweep = true
	}
	if !flagGiven(args, "include-files") && cfg.Commit.IncludeFiles {
		opts.IncludeFiles = true
	}
	if !flagGiven(args, "include-line-numbers") && cfg.Commit.IncludeLineNumbers {
		opts.IncludeLineNumbers = true
	}
	// Standing consent in the repository satisfies the requirement for an
	// explicit grant, which is the point of writing it down ahead of time.
	if cfg.Run.AllowNoVerify {
		opts.NoVerify = true
	}
	// Protected branches always come from the repository, never from a flag, so
	// a project can declare them once and no invocation can quietly drop them.
	opts.Protected = cfg.Protect.Branches
	opts.Style = mergeStyle(fileStyle, opts.Style, args)
}

// mergeStyle layers the invocation's style over the file's, keeping a file
// value wherever the invocation did not name that flag.
func mergeStyle(file, flags style.Style, args []string) style.Style {
	merged := file
	if flagGiven(args, "no-emdash") {
		merged.NoEmDash = flags.NoEmDash
	}
	if flagGiven(args, "no-semicolon") {
		merged.NoSemicolon = flags.NoSemicolon
	}
	if flagGiven(args, "no-hyphen") {
		merged.NoHyphen = flags.NoHyphen
	}
	if flagGiven(args, "max-subject") {
		merged.MaxSubject = flags.MaxSubject
	}
	if flagGiven(args, "punctuation") {
		merged.Punctuation = flags.Punctuation
	}
	if flagGiven(args, "lower-subject") {
		merged.LowerSubject = flags.LowerSubject
	}
	if flagGiven(args, "conventional") {
		merged.Conventional = flags.Conventional
	}
	if flagGiven(args, "prefix") {
		merged.Prefix = flags.Prefix
	}
	if flagGiven(args, "sign-off") {
		merged.SignOff = flags.SignOff
	}
	if flagGiven(args, "body") {
		merged.Body = flags.Body
	}
	if flags.Signer != "" {
		merged.Signer = flags.Signer
	}
	return merged
}

// flagGiven reports whether a flag name appears in the arguments, in either
// the "--name value" or "--name=value" form, and with one dash or two.
func flagGiven(args []string, name string) bool {
	for _, arg := range args {
		if arg == "--" {
			return false
		}
		trimmed := strings.TrimLeft(arg, "-")
		if trimmed == name || strings.HasPrefix(trimmed, name+"=") {
			return true
		}
	}
	return false
}
