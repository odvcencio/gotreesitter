// Package grammarblobs exposes exact embedded blobs to standalone grammar packages.
package grammarblobs

import _ "embed"

//go:embed python.bin
var python []byte

// Python returns a copy of the embedded Python grammar blob.
func Python() []byte {
	return append([]byte(nil), python...)
}
