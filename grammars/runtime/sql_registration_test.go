//go:build !grammar_subset

package grammarruntime

import "testing"

// TestSqlScannerIsRegisteredAndBindsThroughTheCore: the port was compared with the pinned
// C scanner call by call (cgo_harness scandiff, mode on); this test pins the registration
// that follows from it.
func TestSqlScannerIsRegisteredAndBindsThroughTheCore(t *testing.T) {
	checkRegisteredScanner(t, "sql", SqlExternalScanner{}, true, func(payload any) bool { return payload.(*sqlScannerState).cFaithful })
}
