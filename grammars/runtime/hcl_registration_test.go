//go:build !grammar_subset

package grammarruntime

import "testing"

// TestHclScannerIsRegisteredAndBindsThroughTheCore: the port was compared with the pinned
// C scanner call by call (cgo_harness scandiff, mode on, directly and behind one and two order
// adapters); this test pins the registration that follows from it.
func TestHclScannerIsRegisteredAndBindsThroughTheCore(t *testing.T) {
	checkRegisteredScanner(t, "hcl", HclExternalScanner{}, true, func(payload any) bool { return payload.(*hclState).On() })
}
