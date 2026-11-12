//go:build !grammar_subset

package grammarruntime

import "testing"

// TestStarlarkScannerIsRegisteredAndBindsThroughTheCore: the port was compared with the pinned
// C scanner call by call (cgo_harness scandiff, mode on, directly and behind one and two order
// adapters); this test pins the registration that follows from it.
func TestStarlarkScannerIsRegisteredAndBindsThroughTheCore(t *testing.T) {
	checkRegisteredScanner(t, "starlark", StarlarkExternalScanner{}, true, func(payload any) bool { return payload.(*pythonScannerState).IsCFaithful() })
}
