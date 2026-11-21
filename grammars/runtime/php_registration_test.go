//go:build !grammar_subset

package grammarruntime

import "testing"

// TestPhpScannerIsRegisteredAndBindsThroughTheCore: the port was compared with the pinned
// C scanner call by call (cgo_harness scandiff, mode on); this test pins the registration
// that follows from it.
func TestPhpScannerIsRegisteredAndBindsThroughTheCore(t *testing.T) {
	checkRegisteredScanner(t, "php", PhpExternalScanner{}, true, func(payload any) bool { return payload.(*phpState).cFaithful })
}
