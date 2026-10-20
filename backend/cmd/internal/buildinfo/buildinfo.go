// Package buildinfo memuat metadata build yang diisi lewat -ldflags.
package buildinfo

import "fmt"

// Diisi lewat -ldflags "-X grimoire/cmd/internal/buildinfo.Version=..." dst.
var (
	Version   = "dev"
	Commit    = "none"
	BuildDate = "unknown"
)

// String mengembalikan "grimoire <Version> (commit <Commit>, built <BuildDate>)".
func String() string {
	return fmt.Sprintf("grimoire %s (commit %s, built %s)", Version, Commit, BuildDate)
}
