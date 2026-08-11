package gotreesitter

import "time"

// recoveryRuntimeTelemetryEnabled gates the recovery facts that are absent
// from the normal parser runtime. Keep the default path free of these writes.
var recoveryRuntimeTelemetryEnabled bool

type recoveryRuntimeTelemetry struct {
	stats              RecoveryRuntimeStats
	pendingRetryReason string
}

// EnableRecoveryRuntimeTelemetry enables diagnostic recovery counters.
//
// Use this function for single-threaded profiling and witness collection.
// Disable it after the diagnostic run to restore the default path.
func EnableRecoveryRuntimeTelemetry(enabled bool) {
	recoveryRuntimeTelemetryEnabled = enabled
}

// DebugRecoveryRuntimeStats returns the latest recovery facts for parser p.
// The method returns zero values when telemetry is disabled or no parse ran.
func (p *Parser) DebugRecoveryRuntimeStats() RecoveryRuntimeStats {
	if p == nil || !recoveryRuntimeTelemetryEnabled || p.forestDeclineMemo == nil {
		return RecoveryRuntimeStats{}
	}
	return p.forestDeclineMemo.recoveryRuntime.stats
}

func (p *Parser) beginRecoveryRuntimeTelemetry() {
	if p == nil || !recoveryRuntimeTelemetryEnabled {
		return
	}
	cold := p.ensureParserColdState()
	state := &cold.recoveryRuntime
	retryReason := state.pendingRetryReason
	if p.fullParseRetryPassesTaken == 0 {
		retryReason = ""
	}
	state.stats = RecoveryRuntimeStats{
		Enabled:        true,
		RetryPassCount: uint64(p.fullParseRetryPassesTaken),
		RetryReason:    retryReason,
	}
	state.pendingRetryReason = ""
}

func (p *Parser) recoveryRuntimeTelemetryState() *recoveryRuntimeTelemetry {
	if p == nil || !recoveryRuntimeTelemetryEnabled || p.forestDeclineMemo == nil {
		return nil
	}
	return &p.forestDeclineMemo.recoveryRuntime
}

func (p *Parser) recordRecoveryRuntimeRetry(reason string) {
	if p == nil || !recoveryRuntimeTelemetryEnabled || reason == "" {
		return
	}
	cold := p.ensureParserColdState()
	cold.recoveryRuntime.pendingRetryReason = reason
}

func (p *Parser) recordRecoveryEntry() {
	if state := p.recoveryRuntimeTelemetryState(); state != nil {
		state.stats.RecoveryEntryCount++
	}
}

func (p *Parser) recordStrategy1Election() {
	if state := p.recoveryRuntimeTelemetryState(); state != nil {
		state.stats.Strategy1ElectionCount++
	}
}

func (p *Parser) recordRecoveryErrorModeToken() {
	if state := p.recoveryRuntimeTelemetryState(); state != nil {
		state.stats.ErrorModeTokenCount++
	}
}

func (p *Parser) recordRecoveryScannerResync() {
	if state := p.recoveryRuntimeTelemetryState(); state != nil {
		state.stats.ScannerResyncCount++
	}
}

func (p *Parser) recordRecoveryLiveVersions(stacks []glrStack) {
	state := p.recoveryRuntimeTelemetryState()
	if state == nil {
		return
	}
	var live uint64
	for i := range stacks {
		if !stacks[i].dead && !stacks[i].accepted {
			live++
		}
	}
	state.stats.LiveVersionCount = live
	if live > state.stats.PeakLiveVersionCount {
		state.stats.PeakLiveVersionCount = live
	}
}

func (p *Parser) finishRecoveryRuntimeTelemetry(tree *Tree, stacks []glrStack) {
	state := p.recoveryRuntimeTelemetryState()
	if state == nil {
		return
	}
	state.stats.Completed = true
	p.recordRecoveryLiveVersions(stacks)
	state.stats.ErrorNodeCount, state.stats.ErrorSpanBytes = recoveryRuntimeErrorStats(rawRootOrNil(tree))
}

func recoveryRuntimeErrorStats(root *Node) (count uint64, span uint32) {
	if root == nil {
		return 0, 0
	}
	var local [64]*Node
	stack := local[:0]
	stack = append(stack, root)
	minStart := ^uint32(0)
	var maxEnd uint32
	for len(stack) > 0 {
		n := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		if n == nil {
			continue
		}
		if n.IsError() || n.IsMissing() {
			count++
			if n.StartByte() < minStart {
				minStart = n.StartByte()
			}
			if n.EndByte() > maxEnd {
				maxEnd = n.EndByte()
			}
		}
		for i := n.ChildCount() - 1; i >= 0; i-- {
			stack = append(stack, n.Child(i))
		}
	}
	if count == 0 {
		return 0, 0
	}
	return count, maxEnd - minStart
}

func recoveryRuntimeParser(scratch *glrMergeScratch) *Parser {
	if !recoveryRuntimeTelemetryEnabled || scratch == nil {
		return nil
	}
	return scratch.cErrorCostParser
}

func recordRecoveryCostCompetition(scratch *glrMergeScratch) {
	if p := recoveryRuntimeParser(scratch); p != nil {
		if state := p.recoveryRuntimeTelemetryState(); state != nil {
			state.stats.RecoveryCostCompetitionCount++
		}
	}
}

func recoveryRuntimeCostWalk(scratch *glrMergeScratch, lang *Language, stack *glrStack) uint32 {
	p := recoveryRuntimeParser(scratch)
	if p == nil {
		return cStackErrorCostForMergeWithScratch(scratch, lang, stack)
	}
	start := time.Now()
	cost := cStackErrorCostForMergeWithScratch(scratch, lang, stack)
	if state := p.recoveryRuntimeTelemetryState(); state != nil {
		state.stats.RecoveryCostWalkCount++
		state.stats.RecoveryCostWalkNanos += uint64(time.Since(start).Nanoseconds())
	}
	return cost
}
