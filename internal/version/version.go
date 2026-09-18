// SPDX-License-Identifier: AGPL-3.0-only

// Package version exposes build metadata stamped in by the linker.
package version

import "fmt"

// Overwritten at build time with -ldflags -X.
var (
	Version string
	Commit  string
)

func String() string {
	v, c := Version, Commit
	if v == "" {
		v = "dev"
	}
	if c == "" {
		c = "unknown"
	}
	return fmt.Sprintf("%s (%s)", v, c)
}
