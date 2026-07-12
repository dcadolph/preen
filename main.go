// Command preen wraps the Claude Code CLI so the preen skill can be run from
// any terminal. The release's skill text ships inside the binary, so no
// plugin or user-level skill install is required.
package main

import (
	_ "embed"
	"os"

	"github.com/dcadolph/preen/cmd"
)

// skillText is the skill definition pinned into this build.
//
//go:embed skills/preen/SKILL.md
var skillText string

// main delegates to cmd.Execute and exits with its code.
func main() {
	os.Exit(cmd.Execute(skillText, os.Args[1:]))
}
