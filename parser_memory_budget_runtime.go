package gotreesitter

import "runtime"

const (
	// Arena and parser scratch enforce their budgets at every materialization
	// boundary. The runtime-wide guard covers untracked Go heap growth, but the
	// coarse iteration-count poll mask alone lets overshoot balloon 16-26x past
	// the configured budget on lanes that allocate multiple GB across only a
	// few poll-site calls (see the 2026-07-12 budget-containment RCA). Two
	// additional layers close that gap without tightening the mask globally
	// (some legitimate parses, e.g. the Poppler JS witness, rely on bounded
	// overshoot at the default budget to complete): a volume-triggered forced
	// poll (parseRuntimeMemoryVolumePollThresholdBytes) and an absolute,
	// decoupled hard ceiling (parseMemoryHardCeilingMB) checked every time a
	// real poll fires.
	parseRuntimeMemoryPollMask       = 255
	parseRuntimeMemoryTightPollMask  = 15
	parseRuntimeMemoryTightBudget    = 64 << 20
	parseRuntimeMemoryMinSourceBytes = 64 * 1024

	// parseRuntimeMemoryVolumePollThresholdBytes forces a real runtime memory
	// check (bypassing the iteration-count mask) once tracked allocation
	// volume has grown by at least this much since the last real check. This
	// catches lanes that allocate gigabytes across very few poll-site calls,
	// where the mask alone would let many multiples of the budget pass
	// unnoticed before the next sampled check.
	parseRuntimeMemoryVolumePollThresholdBytes = 64 << 20

	parseMemoryBudgetStopSourceArena       = "arena"
	parseMemoryBudgetStopSourceScratch     = "scratch"
	parseMemoryBudgetStopSourceRuntimeHeap = "runtime_heap"
	parseMemoryBudgetStopSourceRuntimeSys  = "runtime_sys"
	parseMemoryBudgetStopSourceHardCeiling = "hard_ceiling"
)

type parseMemoryBudgetDiagnostic struct {
	source                 string
	runtimeHeapGrowthBytes uint64
	runtimeSysGrowthBytes  uint64
}

type runtimeMemoryBudgetRestore struct {
	parser       *Parser
	budget       int64
	baseline     uint64
	baselineSys  uint64
	poll         uint64
	volumeAtPoll uint64
	hardCeiling  int64
}

func runtimeMemoryBudgetEnabled(p *Parser, bytes int64, sourceLen int) bool {
	return p != nil && bytes > 0 && sourceLen >= parseRuntimeMemoryMinSourceBytes
}

// runtimeMemoryHardCeilingEnabled reports whether the absolute hard ceiling
// should arm for this parse. It shares the same trivial-source floor as the
// soft budget (no point instrumenting parses too small to plausibly balloon
// to GB scale) but is otherwise independent of whether the soft budget itself
// is enabled, so it keeps working even if a caller explicitly disables the
// soft budget (GOT_PARSE_MEMORY_BUDGET_MB=0).
//
// A source-length-independent hard ceiling was tried and dropped
// (spore.2026-08-02.walnut-e.memory-exhaustion, consolidated into
// PR #641/rowan/recovery-memory-bounds): removing this floor made
// runtimeMemoryHardCeilingEnabled true at every length, which stopped
// enterRuntimeMemoryBudget from taking its early return (a real,
// stop-the-world-adjacent runtime.ReadMemStats call once per parse, even for
// a one-byte source) and left the soft budget armed at 0 bytes, which in turn
// selected the TIGHT poll mask instead of the loose one at every
// materialization-boundary poll site for the rest of the parse (see
// runtimeMemoryPollMask's git history). Adversarial review measured 3.6x-9x
// mean latency on ordinary small Go parses, a 2.2x wall-clock regression on a
// real 30 KB Swift file, and — the disqualifying finding — reopened the exact
// issue #454 symptom class this file's own determinism contract exists to
// prevent: the same 500-byte Go file returned seven distinct truncated trees
// (HasError()==false over a partial range) across repeated runs under
// concurrent heap churn, where main returns one accepted tree every time. It
// also caught none of the three witnesses tested (arena/scratch stopped all
// three; hard_ceiling stopped none), so it bought determinism risk and
// latency with no offsetting protection. The floor stays; the memory-
// exhaustion fix's real backstops are the always-armed arena/scratch soft
// budget (resultMaterializationStopReason, parser_result.go) and the two
// deterministic loop ceilings in parser_recover_c.go
// (cRecoverMaxReductionCandidateAttempts, cRecoverMaxMissingTokenTrials).
func runtimeMemoryHardCeilingEnabled(p *Parser, sourceLen int) bool {
	return p != nil && sourceLen >= parseRuntimeMemoryMinSourceBytes && parseMemoryHardCeilingBytes() > 0
}

// enterRuntimeMemoryBudget arms the runtime-wide poll for the current parse.
func (p *Parser) enterRuntimeMemoryBudget(bytes int64, sourceLen int) runtimeMemoryBudgetRestore {
	softEnabled := runtimeMemoryBudgetEnabled(p, bytes, sourceLen)
	hardEnabled := runtimeMemoryHardCeilingEnabled(p, sourceLen)
	if p == nil || (!softEnabled && !hardEnabled) {
		return runtimeMemoryBudgetRestore{}
	}
	hardCeiling := int64(0)
	if hardEnabled {
		hardCeiling = parseMemoryHardCeilingBytes()
	}
	restore := runtimeMemoryBudgetRestore{
		parser:       p,
		budget:       p.parseRuntimeMemoryBudgetBytes,
		baseline:     p.parseRuntimeMemoryBaselineBytes,
		baselineSys:  p.parseRuntimeMemoryBaselineSys,
		poll:         p.parseRuntimeMemoryPoll,
		volumeAtPoll: p.parseRuntimeMemoryVolumeAtPoll,
		hardCeiling:  p.parseRuntimeMemoryHardCeilingBytes,
	}

	var stats runtime.MemStats
	runtime.ReadMemStats(&stats)
	if softEnabled {
		p.parseRuntimeMemoryBudgetBytes = bytes
	} else {
		p.parseRuntimeMemoryBudgetBytes = 0
	}
	p.parseRuntimeMemoryBaselineBytes = stats.HeapAlloc
	p.parseRuntimeMemoryBaselineSys = stats.Sys
	p.parseRuntimeMemoryPoll = 0
	p.parseRuntimeMemoryVolumeAtPoll = 0
	p.parseRuntimeMemoryHardCeilingBytes = hardCeiling

	return restore
}

func (r runtimeMemoryBudgetRestore) restore() {
	if r.parser == nil {
		return
	}
	r.parser.parseRuntimeMemoryBudgetBytes = r.budget
	r.parser.parseRuntimeMemoryBaselineBytes = r.baseline
	r.parser.parseRuntimeMemoryBaselineSys = r.baselineSys
	r.parser.parseRuntimeMemoryPoll = r.poll
	r.parser.parseRuntimeMemoryVolumeAtPoll = r.volumeAtPoll
	r.parser.parseRuntimeMemoryHardCeilingBytes = r.hardCeiling
}

// runtimeMemoryBudgetStopReason is the mask-gated poll called from the many
// materialization-boundary sites throughout the parse loop. volumeBytes is a
// cheap, already-tracked allocation-volume signal (e.g. arena.allocatedBytes)
// sampled at the call site with no extra syscalls. When that signal has grown
// by at least parseRuntimeMemoryVolumePollThresholdBytes since the last real
// check, the poll is forced through regardless of the iteration-count mask --
// this is what catches a lane that allocates gigabytes across only a handful
// of poll-site calls (mask undersampling), without lowering the mask (and
// therefore the tolerated overshoot) for every parse.
func (p *Parser) runtimeMemoryBudgetStopReason(volumeBytes uint64) ParseStopReason {
	if p == nil || (p.parseRuntimeMemoryBudgetBytes <= 0 && p.parseRuntimeMemoryHardCeilingBytes <= 0) {
		return ParseStopNone
	}
	p.parseRuntimeMemoryPoll++
	forced := runtimeMemoryVolumeForcesPoll(volumeBytes, p.parseRuntimeMemoryVolumeAtPoll)
	if !forced && p.parseRuntimeMemoryPoll&runtimeMemoryPollMask(p.parseRuntimeMemoryBudgetBytes) != 0 {
		return ParseStopNone
	}
	p.parseRuntimeMemoryVolumeAtPoll = volumeBytes
	return p.runtimeMemoryBudgetStopReasonNow()
}

// runtimeMemoryVolumeForcesPoll reports whether tracked allocation volume has
// grown enough since the last real check to force one now, bypassing the
// iteration-count mask.
func runtimeMemoryVolumeForcesPoll(current, last uint64) bool {
	if current <= last {
		return false
	}
	return current-last >= parseRuntimeMemoryVolumePollThresholdBytes
}

func runtimeMemoryPollMask(budgetBytes int64) uint64 {
	if budgetBytes <= parseRuntimeMemoryTightBudget {
		return parseRuntimeMemoryTightPollMask
	}
	return parseRuntimeMemoryPollMask
}

// runtimeMemoryBudgetStopReasonNow performs the actual runtime.ReadMemStats
// sample and comparison, unconditionally (no mask, no volume gating). It is
// used both by the masked poll above once it decides to fire, and directly by
// the GLR merge survivor loop's coarse-stride poll (mergeStacksWithScratchLargeCap),
// which has no other route back to a materialization-boundary poll site
// during its O(survivors^2) grind.
//
// DETERMINISM CONTRACT (issue #454): only the absolute hard ceiling may stop a
// parse from a runtime.MemStats reading. The soft per-parse budget must not,
// because HeapAlloc and Sys are process-global quantities:
//
//   - HeapAlloc is live heap. A GC cycle mid-parse makes growth appear to
//     recede, so the same input crosses the budget at a different token on
//     every run.
//   - Sys is total memory the runtime has obtained from the OS. It moves in
//     large steps driven by GC pacing and by allocation from other goroutines
//     in the host process, which a parse does not control at all.
//
// Letting either decide where a parse stops makes the returned tree a function
// of GC timing rather than of the input. A downstream editor measured the
// consequence directly: five parses of identical 137 KiB C# bytes returned five
// different trees (1, 11048, 19178, 22928 and 31928 nodes), each reporting
// stopReason=memory_budget with HasError()==false over a partial byte range.
// This repo's own parser_memory_budget_runtime_test.go had already recorded the
// same instability, accepting either runtime_heap or runtime_sys as the stop
// source because "pinning runtime_heap alone made this test race-mode-flaky".
//
// Footprint is still bounded, and bounded deterministically, by the layers that
// measure what the parse itself allocated: the arena budget
// (nodeArena.budgetExhausted), the scratch budget
// (parserScratch.budgetExhausted), and the node and stack-depth limits. Those
// stop the same input at the same place every time. The hard ceiling below
// stays on real memory on purpose: it is an out-of-memory backstop of last
// resort, not a shaping decision, and a parse that reaches it is already
// pathological.
func (p *Parser) runtimeMemoryBudgetStopReasonNow() ParseStopReason {
	if p == nil || p.parseRuntimeMemoryHardCeilingBytes <= 0 {
		return ParseStopNone
	}
	var stats runtime.MemStats
	runtime.ReadMemStats(&stats)
	heapGrowth := runtimeMemoryGrowth(stats.HeapAlloc, p.parseRuntimeMemoryBaselineBytes)
	sysGrowth := runtimeMemoryGrowth(stats.Sys, p.parseRuntimeMemoryBaselineSys)
	if ceiling := p.parseRuntimeMemoryHardCeilingBytes; ceiling > 0 && heapGrowth >= uint64(ceiling) {
		return p.noteRuntimeMemoryBudgetStop(parseMemoryBudgetStopSourceHardCeiling, heapGrowth, sysGrowth)
	}
	return ParseStopNone
}

func runtimeMemoryGrowth(current, baseline uint64) uint64 {
	if current <= baseline {
		return 0
	}
	return current - baseline
}

func (p *Parser) noteMemoryBudgetStop(source string) ParseStopReason {
	return p.noteRuntimeMemoryBudgetStop(source, 0, 0)
}

func (p *Parser) noteRuntimeMemoryBudgetStop(source string, heapGrowth, sysGrowth uint64) ParseStopReason {
	if p == nil || !p.parseMemoryBudgetDiagActive || p.parseMemoryBudgetDiag.source != "" {
		return ParseStopMemoryBudget
	}
	p.parseMemoryBudgetDiag.source = source
	p.parseMemoryBudgetDiag.runtimeHeapGrowthBytes = heapGrowth
	p.parseMemoryBudgetDiag.runtimeSysGrowthBytes = sysGrowth
	return ParseStopMemoryBudget
}
