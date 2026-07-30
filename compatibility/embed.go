// Package compatibility exposes the release-bundled compatibility manifest.
package compatibility

import _ "embed"

// Manifest is the exact compatibility contract embedded into the checker.
//
//go:embed manifest.json
var Manifest []byte
