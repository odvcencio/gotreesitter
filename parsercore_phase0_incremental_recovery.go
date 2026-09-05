//go:build !gts_no_parsercorephase0

package gotreesitter

import "time"

type compactFreshAttemptWork struct {
	allocatedNodes uint64
	tokens         uint64
}

// attemptCompactIncrementalRecoveryFullParse runs after reuse cleanup.
// Recovery requires complete descendants, so it starts a fresh compact graph.
func (p *Parser) attemptCompactIncrementalRecoveryFullParse(source []byte, reason string, recoveryDeclined bool, timing *incrementalParseTiming) (result *Tree) {
	if !recoveryDeclined || reason == "" || !p.admissionCandidateFullParseEligible(nil, true) {
		return nil
	}
	runner, ok := p.admissionCandidateRunner.(*parserCoreFreshFullRunner)
	if !ok || runner == nil || runner.lang != p.language ||
		!runner.options.allowCompactRecoveryVersionTurns || !runner.options.Recovery ||
		runner.options.compactIncrementalReuse != nil || runner.scheduler.options.compactIncrementalReuse != nil ||
		runner.scratch.incrementalReuse != nil || runner.scratch.freshAttemptWork != nil {
		return nil
	}
	started := time.Now()
	var work compactFreshAttemptWork
	if timing != nil {
		runner.scratch.freshAttemptWork = &work
	}
	defer func() {
		runner.scratch.freshAttemptWork = nil
		if timing != nil {
			// The successful tree reports its own allocations. Charge only
			// discarded attempts here, including materialization declines.
			if result != nil {
				work.allocatedNodes -= uint64(result.parseRuntime.NodesAllocated)
			}
			timing.newNodes += work.allocatedNodes
			timing.tokensConsumed += work.tokens
		}
		// Successful work comes from profileFreshParseFallback. A declined
		// fresh attempt must still be charged before the legacy fallback.
		if result == nil && timing != nil {
			timing.totalNanos += time.Since(started).Nanoseconds()
			timing.tokensConsumed += runner.scheduler.tokens
		}
	}()
	// Do not add an incremental entry to the fresh-parse admission counters.
	tree, accepted, _ := p.tryCompactFullParseRoute(source)
	if !accepted || tree == nil {
		return nil
	}
	tree.parseRuntime.CompactIncrementalFullRecoveryRoute = true
	tree.parseRuntime.CompactIncrementalFallbackReason = reason
	return tree
}
