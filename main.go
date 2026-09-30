// Command preen at this path is a pointer. Releases from v2 on live at
// github.com/dcadolph/preen/v2, and Go does not install them from here.
package main

import (
	"fmt"
	"os"
)

// main says where preen moved and exits nonzero, so a script that installed
// from this path fails loudly instead of running a stale release.
func main() {
	fmt.Fprintln(os.Stderr, "preen moved: go install github.com/dcadolph/preen/v2@latest")
	os.Exit(1)
}
