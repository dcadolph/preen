package cmd

import (
	"fmt"
	"runtime/debug"
	"testing"
)

// TestResolveVersion checks each source of the reported version in order of
// precedence.
func TestResolveVersion(t *testing.T) {
	t.Parallel()

	withModule := func(v string) *debug.BuildInfo {
		return &debug.BuildInfo{Main: debug.Module{Path: "github.com/dcadolph/preen/v2", Version: v}}
	}
	tests := []struct {
		Name        string
		Set         string
		Info        *debug.BuildInfo
		OK          bool
		WantVersion string
	}{
		{ // Test 0: The release build's version wins.
			Name: "release build", Set: "2.2.1", Info: withModule("v2.2.0"), OK: true, WantVersion: "2.2.1",
		},
		{ // Test 1: go install records the module version.
			Name: "go install", Info: withModule("v2.2.1"), OK: true, WantVersion: "2.2.1",
		},
		{ // Test 2: A build from a checkout has no module version.
			Name: "local build", Info: withModule("(devel)"), OK: true, WantVersion: "dev",
		},
		{ // Test 3: No build info at all.
			Name: "no build info", WantVersion: "dev",
		},
	}

	for testNum, test := range tests {
		t.Run(fmt.Sprintf("test %d %s", testNum, test.Name), func(t *testing.T) {
			t.Parallel()
			if got := resolveVersion(test.Set, test.Info, test.OK); got != test.WantVersion {
				t.Errorf("resolveVersion() = %q, want %q", got, test.WantVersion)
			}
		})
	}
}
