package cmd

// Version is the CLI version. Keep in step with .claude-plugin/plugin.json,
// or override at build time with
// -ldflags "-X github.com/dcadolph/preen/cmd.Version=<v>".
//
//nolint:gochecknoglobals // Build-time override target.
var Version = "0.10.0"
