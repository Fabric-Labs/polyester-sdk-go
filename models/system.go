package models

import "strings"

// ServerVersion is the exact build an environment serves, as
// Client.GetVersion reads it.
type ServerVersion struct {
	// Version of the build: a release reads MAJOR.MINOR.PATCH, such as
	// "0.10.0"; a build made after a release, which devnet may run, appends
	// the number of source revisions since that release and "g" with the
	// first 12 hex digits of its revision, such as "0.10.0-12-g4e21a2f09670".
	// It is not a semantic version: do not compare the part after "-" as a
	// pre-release.
	Version string `json:"version"`
	// Commit is the full source revision of the build.
	Commit string `json:"commit"`
	// Environment that answered: "devnet", "testnet", "mainnet", or "local"
	// for a development stack. It stays a plain string, so a value this SDK
	// does not know yet still reads.
	Environment string `json:"environment"`
}

// Release returns the release the build follows: Version up to its first
// "-", such as "0.10.0" for "0.10.0-12-g4e21a2f09670".
func (v ServerVersion) Release() string {
	release, _, _ := strings.Cut(v.Version, "-")
	return release
}
