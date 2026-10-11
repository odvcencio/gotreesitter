//go:build !gts_no_parsercorephase0

package gotreesitter

// AdmissionCandidateConvergedSplitWorkForTest exposes the compact scheduler's
// converged-reduction-split-drop counters recorded by p's last candidate-route
// parse attempt, whether that attempt routed or declined. ConvergedReduction
// SplitDrops counts every no-action head dropped that descended from a
// converged-path reduction split; ConvergedCoverageDrops counts the subset
// whose dropped header's recorded alternative set was contained in one
// surviving header's recorded set (spec.b4b-alternative-set.v2 section 5,
// renamed from SelectedLineageDrops at stage 2b). A routed parse always has
// drops == proved (the accept gate requires it); a mid-run decline can leave
// the two unequal at the point of the last completed drop.
//
// ok is false when p never attempted the candidate route (no cached runner) or
// the run never reached an acceptance receipt. The runner is per-Parser and
// reused across calls, so callers must read this immediately after Parse and
// before any other parse on the same Parser.
func AdmissionCandidateConvergedSplitWorkForTest(p *Parser) (drops, proved uint64, ok bool) {
	if p == nil {
		return 0, 0, false
	}
	runner, isRunner := p.admissionCandidateRunner.(*parserCoreFreshFullRunner)
	if !isRunner || runner == nil {
		cold := p.forestDeclineMemo
		if cold == nil {
			return 0, 0, false
		}
		work := cold.admissionConvergedWork
		return work[0], work[1], work[2] != 0
	}
	if runner.scheduler.receipt == nil || runner.scheduler.receipt.Acceptance == nil {
		return 0, 0, false
	}
	work := runner.scheduler.receipt.Acceptance.Work
	return work.ConvergedReductionSplitDrops, work.ConvergedCoverageDrops, true
}
