package config

import "errors"

// Sentinel errors for reading the repository config.
var (
	// ErrRead reports a config file that exists but could not be read.
	ErrRead = errors.New("cannot read the config file")
	// ErrParse reports a config file that is not valid TOML, or that holds a
	// value preen cannot act on.
	ErrParse = errors.New("cannot parse the config file")
)
