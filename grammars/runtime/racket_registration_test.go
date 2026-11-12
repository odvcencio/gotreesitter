//go:build !grammar_subset

package grammarruntime

import (
	"testing"

	"github.com/odvcencio/gotreesitter/internal/scanmode"
)

// TestRacketScannerIsRegisteredAndBindsThroughTheCore: the port was compared with the pinned
// C scanner call by call (cgo_harness scandiff, mode on, directly and behind one and two order
// adapters); this test pins the registration that follows from it.
func TestRacketScannerIsRegisteredAndBindsThroughTheCore(t *testing.T) {
	checkRegisteredScanner(t, "racket", RacketExternalScanner{}, true, func(payload any) bool { return scanmode.Of(payload) })
}
