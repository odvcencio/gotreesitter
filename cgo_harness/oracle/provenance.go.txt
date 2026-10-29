//go:build cgo

package tree_sitter

import _ "embed"

//go:embed upstream.json
var sourceManifest []byte

// SourceManifest records the exact runtime sources and local binding patch
// compiled by this harness module. It does not depend on an upstream tag.
func SourceManifest() []byte {
	return append([]byte(nil), sourceManifest...)
}
