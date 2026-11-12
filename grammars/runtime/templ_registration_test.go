//go:build !grammar_subset

package grammarruntime

import "testing"

// TestTemplScannerIsRegisteredAndBindsThroughTheCore: the port was compared with the pinned
// C scanner call by call (cgo_harness scandiff, mode on, directly and behind one and two order
// adapters); this test pins the registration that follows from it.
func TestTemplScannerIsRegisteredAndBindsThroughTheCore(t *testing.T) {
	checkRegisteredScanner(t, "templ", TemplExternalScanner{}, true, func(payload any) bool { return payload.(*templState).On() })
}
