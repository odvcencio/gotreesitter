//go:build gts_recovery_telemetry

package gotreesitter

import "runtime"
import "time"

const recoveryRuntimeDetailedBuildEnabled = true

func (p *Parser) detailedRecoveryRuntimeState(create bool) *recoveryRuntimeDetailedState {
	if p == nil || !recoveryRuntimeTelemetryEnabled {
		return nil
	}
	cold := p.forestDeclineMemo
	if cold == nil && create {
		cold = p.ensureParserColdState()
	}
	if cold == nil {
		return nil
	}
	state := cold.recoveryRuntimeDetailed
	if state == nil && create {
		state = &recoveryRuntimeDetailedState{}
		cold.recoveryRuntimeDetailed = state
	}
	return state
}

func (p *Parser) resetRecoveryRuntimeTelemetryDetailed() {
	if p == nil || !recoveryRuntimeTelemetryEnabled || p.forestDeclineMemo == nil {
		return
	}
	p.clearRecoveryRuntimeTelemetryDetailed()
}

// clearRecoveryRuntimeTelemetryDetailed scrubs diagnostic state at pool boundaries.
// Pool release must clear state even after the runtime toggle is disabled.
func (p *Parser) clearRecoveryRuntimeTelemetryDetailed() {
	if p == nil || p.forestDeclineMemo == nil {
		return
	}
	p.forestDeclineMemo.recoveryRuntime = recoveryRuntimeTelemetry{}
	p.forestDeclineMemo.recoveryRuntimeDetailed = nil
}

func (p *Parser) beginRecoveryRuntimeTelemetryDetailed() {
	if p == nil || !recoveryRuntimeTelemetryEnabled {
		return
	}
	state := p.detailedRecoveryRuntimeState(true)
	if state == nil {
		return
	}
	if p.fullParseRetryPassesTaken == 0 {
		state.attempts = nil
		state.byTree = nil
	}
	var mem runtime.MemStats
	runtime.ReadMemStats(&mem)
	state.activeStarted = time.Now()
	state.activeHeap = mem.HeapAlloc
	state.activeTotal = mem.TotalAlloc
	state.activeMallocs = mem.Mallocs
	state.activeCondense = 0
}

func (p *Parser) finishRecoveryRuntimeTelemetryDetailed(tree *Tree, parseRuntime *ParseRuntime) {
	if p == nil || tree == nil || !recoveryRuntimeTelemetryEnabled {
		return
	}
	state := p.detailedRecoveryRuntimeState(true)
	if state == nil {
		return
	}
	attempt := RecoveryRuntimeAttemptStats{
		Ordinal: uint32(len(state.attempts)),
		Rung:    "initial",
		Cause:   "initial_parse",
	}
	if stats := p.recoveryRuntimeTelemetryState(); stats != nil {
		attempt.RecoveryEntryCount = stats.stats.RecoveryEntryCount
		attempt.Strategy1ElectionCount = stats.stats.Strategy1ElectionCount
		attempt.RecoveryCostCompetitionCount = stats.stats.RecoveryCostCompetitionCount
		attempt.RecoveryCostWalkCount = stats.stats.RecoveryCostWalkCount
		attempt.RecoveryCostWalkNanos = stats.stats.RecoveryCostWalkNanos
		attempt.LiveVersions = stats.stats.LiveVersionCount
		attempt.PeakLiveVersions = stats.stats.PeakLiveVersionCount
	}
	if parseRuntime != nil {
		attempt.StopReason = parseRuntime.StopReason
		attempt.Truncated = parseRuntime.Truncated
		attempt.TokenSourceEOFEarly = parseRuntime.TokenSourceEOFEarly
		attempt.WallNanos = detailedNonNegativeUint64(parseRuntime.ParseWallNanos)
		attempt.ArenaBytesPeak = detailedNonNegativeUint64(parseRuntime.ArenaBytesAllocated)
		attempt.ScratchBytesPeak = detailedNonNegativeUint64(parseRuntime.ScratchBytesAllocated)
		attempt.EntryScratchBytesPeak = detailedNonNegativeUint64(parseRuntime.EntryScratchBytesAllocated)
		attempt.GSSBytesPeak = detailedNonNegativeUint64(parseRuntime.GSSBytesAllocated)
		attempt.GSSNodesPeak = detailedNonNegativeUint64(int64(parseRuntime.GSSNodesUsed))
		attempt.NodesAllocated = detailedNonNegativeUint64(int64(parseRuntime.NodesAllocated))
		attempt.MaxStacksSeen = detailedNonNegativeUint64(int64(parseRuntime.MaxStacksSeen))
		attempt.PeakStackDepth = detailedNonNegativeUint64(int64(parseRuntime.PeakStackDepth))
		attempt.ResultSelectionNanos = detailedNonNegativeUint64(parseRuntime.ResultSelectionNanos)
		attempt.TransientParentMaterializationNanos = detailedNonNegativeUint64(parseRuntime.TransientParentMaterializationNanos)
		attempt.ResultTreeBuildNanos = detailedNonNegativeUint64(parseRuntime.ResultTreeBuildNanos)
		attempt.TransientChildMaterializationNanos = detailedNonNegativeUint64(parseRuntime.TransientChildMaterializationNanos)
		attempt.MaterializationNanos = attempt.ResultSelectionNanos +
			attempt.TransientParentMaterializationNanos +
			attempt.ResultTreeBuildNanos +
			attempt.TransientChildMaterializationNanos
		expectedStart, expectedEnd := detailedRecoveryRuntimeExpectedRange(p, parseRuntime)
		root := rawRootOrNil(tree)
		attempt.AttemptFullSpan = root != nil && root.StartByte() == expectedStart && root.EndByte() == expectedEnd
	}
	root := rawRootOrNil(tree)
	attempt.AttemptHasError = root != nil && root.HasError()
	if attempt.WallNanos == 0 && !state.activeStarted.IsZero() {
		attempt.WallNanos = detailedNonNegativeUint64(time.Since(state.activeStarted).Nanoseconds())
	}
	var mem runtime.MemStats
	runtime.ReadMemStats(&mem)
	attempt.HeapAllocDeltaBytes = int64(mem.HeapAlloc) - int64(state.activeHeap)
	if mem.TotalAlloc >= state.activeTotal {
		attempt.TotalAllocDeltaBytes = mem.TotalAlloc - state.activeTotal
	}
	if mem.Mallocs >= state.activeMallocs {
		attempt.MallocsDelta = mem.Mallocs - state.activeMallocs
	}
	attempt.CondenseNanos = state.activeCondense
	state.attempts = append(state.attempts, attempt)
	if state.byTree == nil {
		state.byTree = make(map[*Tree]int)
	}
	state.byTree[tree] = len(state.attempts) - 1
	state.activeStarted = time.Time{}
	state.activeCondense = 0
}

func detailedRecoveryRuntimeExpectedRange(p *Parser, parseRuntime *ParseRuntime) (uint32, uint32) {
	if p != nil && len(p.included) != 0 {
		return p.included[0].StartByte, p.included[len(p.included)-1].EndByte
	}
	if parseRuntime == nil {
		return 0, 0
	}
	return 0, parseRuntime.ExpectedEOFByte
}

func detailedNonNegativeUint64(value int64) uint64 {
	if value <= 0 {
		return 0
	}
	return uint64(value)
}

func (p *Parser) recordRecoveryRuntimeRetryTreeDetailed(tree *Tree, rung, cause string) {
	if p == nil || tree == nil || !recoveryRuntimeTelemetryEnabled {
		return
	}
	state := p.detailedRecoveryRuntimeState(false)
	if state == nil || state.byTree == nil {
		return
	}
	index, ok := state.byTree[tree]
	if !ok || index < 0 || index >= len(state.attempts) {
		return
	}
	attempt := state.attempts[index]
	if rung != "" {
		attempt.Rung = rung
	}
	if cause != "" {
		attempt.Cause = cause
	}
	state.attempts[index] = attempt
}

func (p *Parser) recordRecoveryRuntimeSelectedTreeDetailed(tree *Tree) {
	if p == nil || tree == nil || !recoveryRuntimeTelemetryEnabled {
		return
	}
	state := p.detailedRecoveryRuntimeState(false)
	if state == nil || state.byTree == nil {
		return
	}
	index, ok := state.byTree[tree]
	if !ok || index < 0 || index >= len(state.attempts) {
		return
	}
	attempt := state.attempts[index]
	attempt.CandidateSelected = true
	state.attempts[index] = attempt
}

func (p *Parser) recordRecoveryRuntimeCandidateReplacedDetailed(tree *Tree) {
	if p == nil || tree == nil || !recoveryRuntimeTelemetryEnabled {
		return
	}
	state := p.detailedRecoveryRuntimeState(false)
	if state == nil || state.byTree == nil {
		return
	}
	index, ok := state.byTree[tree]
	if !ok || index < 0 || index >= len(state.attempts) {
		return
	}
	attempt := state.attempts[index]
	attempt.CandidateSelected = true
	attempt.CandidateReplacedIncumbent = true
	state.attempts[index] = attempt
}

func (p *Parser) clearRecoveryRuntimeRetryTreesDetailed() {
	if p == nil || p.forestDeclineMemo == nil {
		return
	}
	state := p.forestDeclineMemo.recoveryRuntimeDetailed
	if state == nil {
		return
	}
	state.byTree = nil
}

func (p *Parser) cCondenseAndResumeDetailed(
	stacks []glrStack,
	source []byte,
	ts TokenSource,
	tok Token,
	nodeCount *int,
	arena *nodeArena,
	entryScratch *glrEntryScratch,
	gssScratch *gssScratch,
	tmpEntries *[]stackEntry,
	parseScratch *parserScratch,
	trackChildErrors *bool,
	condenseClean ...bool,
) ([]glrStack, bool, Token, ParseStopReason) {
	if !recoveryRuntimeTelemetryEnabled {
		return p.cCondenseAndResume(stacks, source, ts, tok, nodeCount, arena, entryScratch, gssScratch, tmpEntries, parseScratch, trackChildErrors, condenseClean...)
	}
	started := time.Now()
	resultStacks, resumed, resultToken, reason := p.cCondenseAndResume(stacks, source, ts, tok, nodeCount, arena, entryScratch, gssScratch, tmpEntries, parseScratch, trackChildErrors, condenseClean...)
	if state := p.detailedRecoveryRuntimeState(false); state != nil {
		state.activeCondense += detailedNonNegativeUint64(time.Since(started).Nanoseconds())
	}
	return resultStacks, resumed, resultToken, reason
}

// DebugRecoveryRuntimeAttempts returns a copy of the tagged attempt receipt.
func (p *Parser) DebugRecoveryRuntimeAttempts() RecoveryRuntimeAttempts {
	if p == nil || !recoveryRuntimeTelemetryEnabled || p.forestDeclineMemo == nil || p.forestDeclineMemo.recoveryRuntimeDetailed == nil {
		return nil
	}
	attempts := p.forestDeclineMemo.recoveryRuntimeDetailed.attempts
	if len(attempts) == 0 {
		return nil
	}
	return append(RecoveryRuntimeAttempts(nil), attempts...)
}
