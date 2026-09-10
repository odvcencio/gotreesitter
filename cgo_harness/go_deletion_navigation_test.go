//go:build cgo && treesitter_c_parity && gts_parsercorephase0 && !gts_no_parsercorephase0

package cgoharness

import (
	"testing"

	gts "github.com/odvcencio/gotreesitter"
)

// Recovery still uses legacy. Both routes must publish correct navigation.
func TestGoDeletionHistoryNavigationLockedC(t *testing.T) {
	t.Setenv("GOT_GLR_MAX_MERGE_PER_KEY", "")
	gts.ResetParseEnvConfigCacheForTests()
	t.Cleanup(gts.ResetParseEnvConfigCacheForTests)
	for _, tc := range loadCanonicalGoIncrementalCases(t) {
		if tc.spec.Name == "recovery_deletion" {
			runGoCompactEditHistory(t, tc, false)
			return
		}
	}
	t.Fatal("missing recovery deletion fixture")
}
