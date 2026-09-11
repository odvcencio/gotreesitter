//go:build cgo && treesitter_c_parity && !gts_no_parsercorephase0

package cgoharness

import "testing"

func TestErlangAdmissionFuzzSeedsNativeLockedCParity(t *testing.T) {
	runErlangAdmissionFuzzSeedsLockedCParity(t, true)
}
