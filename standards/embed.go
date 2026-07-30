// Package standards exposes immutable, release-bundled normative snapshots.
package standards

import "embed"

// Snapshots contains only accepted, digest-bound standard sources.
//
//go:embed snapshots
var Snapshots embed.FS
