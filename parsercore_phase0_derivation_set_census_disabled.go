//go:build !gts_no_parsercorephase0 && !gts_derivation_set_census

package gotreesitter

import (
	core "github.com/odvcencio/gotreesitter/internal/parsercorephase0"
)

// Shipped build of the derivation-set differential instrument (stage D0 of
// spec.derivation-set-equivalence.v1): the instrument is absent.
//
// parsercore_phase0_derivation_set_census.go carries the whole census behind
// the gts_derivation_set_census build tag. This file supplies the same two
// call-site names as empty functions so completeAcceptance compiles
// unchanged in the default build. Both bodies are empty, so the inliner
// removes the calls and the shipped binary keeps the exact instruction
// stream it had before the instrument existed.

// DerivationSetCensusBuilt reports whether this binary carries the
// derivation-set census. The shipped build never does.
func DerivationSetCensusBuilt() bool { return false }

func (s *diagnosticParserCoreGenericScheduler) censusAcceptanceDerivationSet(
	_ []core.Derivation, _ core.Derivation, _ bool, _ bool,
) {
}

func (s *diagnosticParserCoreGenericScheduler) censusAcceptanceDerivationSetTruncated() {}
