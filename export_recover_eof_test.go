//go:build !gts_no_parsercorephase0

package gotreesitter

// CompactRecoverEOFAcceptTelemetryForTest captures the private runner receipt
// and public runtime for one direct compact recover_eof test route.
//
// This test-only seam keeps the scheduler work counter out of the production
// API while allowing external grammar tests to verify live publication.
type CompactRecoverEOFAcceptTelemetryForTest struct {
	RecoverEOFAccepts uint64
	Runtime           ParseRuntime
}

// TryCompactRecoverEOFAcceptTelemetryForTest runs the direct compact candidate
// route and returns its scheduler recover_eof count and tree runtime.
func TryCompactRecoverEOFAcceptTelemetryForTest(
	p *Parser,
	source []byte,
) (tree *Tree, telemetry CompactRecoverEOFAcceptTelemetryForTest, ok bool, reason string) {
	if p == nil {
		return nil, telemetry, false, "parser is nil"
	}
	runner, borrowed, err := p.borrowAdmissionCandidateRunner()
	if err != nil {
		return nil, telemetry, false, err.Error()
	}
	defer p.returnAdmissionCandidateRunner(runner, borrowed)
	// Hold the lease until the receipt is read; the nested route borrows the
	// same runtime and leaves its return to this outer owner.
	tree, ok, reason = p.tryCompactFullParseRoute(source)
	telemetry.RecoverEOFAccepts = runner.scheduler.work.RecoverEOFAccepts
	if tree != nil {
		telemetry.Runtime = tree.ParseRuntime()
	}
	return tree, telemetry, ok, reason
}
