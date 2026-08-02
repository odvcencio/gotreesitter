//go:build !gts_no_parsercorephase0 && !gts_derivation_set_census

package gotreesitter

import "testing"

// TestDerivationSetCensusIsAbsentFromTheDefaultBuild is the inertness ratchet
// for the stage D0 differential instrument
// (spec.derivation-set-equivalence.v1).
//
// The instrument records the whole live derivation set at every accept and
// materializes every candidate to do it. That cost must never reach a shipped
// parse. A build tag, not an env var, is what keeps it out: the default build
// compiles parsercore_phase0_derivation_set_census_disabled.go, whose two
// call-site methods have empty bodies, so the accept path keeps the exact
// instruction stream it had before the instrument existed.
//
// This test fails the moment the default build starts carrying the census.
func TestDerivationSetCensusIsAbsentFromTheDefaultBuild(t *testing.T) {
	if DerivationSetCensusBuilt() {
		t.Fatal("the derivation-set census is compiled into the default build; it must stay behind the gts_derivation_set_census build tag")
	}
}
