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
	tree, ok, reason = p.tryCompactFullParseRoute(source)
	if runner, err := p.acquireAdmissionCandidateRunner(); err == nil && runner != nil {
		telemetry.RecoverEOFAccepts = runner.scheduler.work.RecoverEOFAccepts
	}
	if tree != nil {
		telemetry.Runtime = tree.ParseRuntime()
	}
	return tree, telemetry, ok, reason
}
