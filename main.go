// Command preen turns a messy working tree into a clean, ordered set of
// atomic commits. It plans before it moves, backs up before it changes
// history, and verifies that content came out exactly as it went in.
package main

import (
	"os"

	"github.com/dcadolph/preen/cmd"
)

// main delegates to cmd.Execute and exits with its code.
func main() {
	os.Exit(cmd.Execute(os.Args[1:]))
}
