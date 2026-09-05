//go:build gts_no_parsercorephase0

package gotreesitter

func (p *Parser) attemptCompactIncrementalParse(_ []byte, _ *Tree, _ *incrementalParseTiming) (*Tree, string) {
	return nil, ""
}

// tryCompactFullParseRoute is the emergency-build stub for the compact
// candidate route. Phase-3 admission promoted the compact engine into the
// default build; the emergency opt-out tag gts_no_parsercorephase0 compiles the
// engine back out. In that build the candidate route is unavailable: every
// eligible full parse fails closed and falls back to production. The switch
// still resolves and its counters still move, so the routing seam is testable
// without the engine.
func (p *Parser) tryCompactFullParseRoute(_ []byte) (*Tree, bool, string) {
	return nil, false, "compact candidate route compiled out (built with -tags gts_no_parsercorephase0)"
}

// DiagnosticEnableDropCohortCertificateAdmissionForTest is inert when the
// compact parser build is disabled.
func (p *Parser) DiagnosticEnableDropCohortCertificateAdmissionForTest() func() {
	return func() {}
}

// admissionCandidateCompactStorageBytes is the emergency-build stub: the
// compact engine is compiled out, so no cached runner and no compact storage
// ever exist.
func admissionCandidateCompactStorageBytes(_ *Parser) uint64 {
	return 0
}

// admissionCandidateCompactFootprintBytes is the emergency-build stub,
// matching admissionCandidateCompactStorageBytes above.
func admissionCandidateCompactFootprintBytes(_ *Parser) uint64 {
	return 0
}
