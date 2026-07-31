// Package templates embeds the versioned Golden Path materialization bundle.
package templates

import "embed"

// Files contains every source template and the pre-resolved cross-platform
// mise lock catalog used by the deterministic generator.
//
//go:embed bundle.json all:common all:profiles tooling/mise.toml tooling/mise.lock
var Files embed.FS
