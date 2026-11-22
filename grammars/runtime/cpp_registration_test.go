//go:build !grammar_subset

package grammarruntime

import "testing"

// TestCppScannerIsRegisteredAndBindsThroughTheCore: the port was compared with the pinned
// C scanner call by call (cgo_harness scandiff, mode on, safe-state runs); this test pins
// the registration that follows from it.
func TestCppScannerIsRegisteredAndBindsThroughTheCore(t *testing.T) {
	checkRegisteredScanner(t, "cpp", CppExternalScanner{}, true, func(payload any) bool { return payload.(*cppState).On() })
}
