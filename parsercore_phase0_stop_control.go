//go:build !gts_no_parsercorephase0

package gotreesitter

import (
	"math"
	"slices"
	"unsafe"

	core "github.com/odvcencio/gotreesitter/internal/parsercorephase0"
)

// Compact stop control owns footprint measurement and scheduler stop checks.
// Keep memory accounting, overflow protection, and polling order together.

// diagnosticParserCoreStopControlTripped renders a poll-detected stop-control
// trip (spec.campaign.v7 tranche B8: memory budget, deadline, or
// cancellation) as the same kind of decline every other scheduler cap uses.
// Returning a real error here -- not the graceful s.finish(...) receipt path
// -- matters: run executes inside compact.RunFreshSchedulerSession for the
// admission-candidate route (options.freshSchedulerSession), whose deferred
// cleanup resets the whole core on any non-nil error. That is what releases
// the compact arenas' accumulated storage before the caller's production
// fallback engages, with no extra cleanup call needed here.
func diagnosticParserCoreStopControlTripped(reason ParseStopReason) error {
	return &diagnosticParserCoreDecline{
		boundary: DiagnosticParserCoreCap,
		detail:   "scheduler stop-control tripped: " + string(reason),
	}
}

// stopControlFootprintChurnRatio documents a measured, deliberately UNUSED
// lever (tranche B9 honest-accounting gate). FootprintBytes gauges retained
// structure; it is blind to per-token ephemeral allocation (temporary
// values the dispatch/election hot path creates and discards -- boxed
// action results, scanner-state capture buffers, and similar -- that a live
// GC reclaims continuously in normal operation but that accumulate
// unbounded in any measurement that holds GC off for the whole parse,
// including the RCA-era production replica test this poll is compared
// against). On the giant-table-literal witness, a scheduler run whose
// tracked footprint reached 103.6 MB at decline had allocated 516.5 MB
// cumulative by then: a ~5x ratio.
//
// A runtime-heap-based soft stop is not an option here. The determinism
// contract at parser_memory_budget_runtime.go:162-172 (issue #454) bars a
// runtime.MemStats reading (HeapAlloc, Sys) from stopping a parse at
// anything but the absolute hard ceiling: both readings are process-global
// and shift run to run with GC timing, not with the input, so using either
// one for the SOFT per-parse budget would make this poll's trip point
// non-deterministic. FootprintBytes is the deterministic, capacity-based
// gauge that keeps the soft budget reproducible instead.
//
// Discounting the comparison threshold by a fixed divisor (tripping the
// poll at budget/divisor instead of budget) was tried and reverted: at
// divisor 2, the giant-table-literal replica still exceeded the 6x
// cumulative-allocation contract (6.15-6.70x measured, still over), and a
// realistic, currently-passing witness (a 140KB clean Go source, budget 48
// MB) started declining before completion, because ITS OWN legitimate
// footprint at completion (measured (34,36] MB) already exceeds budget/2. At
// divisor 3 the replica came inside the contract (5.0-5.9x) but the same
// 140KB/48MB witness still regressed. No tested divisor cleared the
// pathological witness without cutting into ordinary coverage, because the
// two witnesses need materially different discounts: the giant literal's
// churn ratio is a property of ITS shape (dense, repeated struct-literal
// reduction), not a universal constant every input pays.
//
// stopControlMemoryBudgetReason therefore compares FootprintBytes against
// the configured budget with NO discount (ratio effectively 1): the honest,
// cap()-based, structure-complete gauge alone, with its own measured
// improvement (11.51x to 9.14-9.56x cumulative allocation on the same
// replica, down from the pre-B9-honest-accounting baseline). That
// improvement is not free at low budgets: FootprintBytes reads higher than
// the length-only StorageBytes gauge tranche B9 replaced, so the minimum
// budget a realistic witness needs to still route (rather than decline)
// rose too -- measured +25-29% on two witnesses (the same 140KB clean Go
// source referenced above: 28 MB to 36 MB; a 238KB one: 48 MB to 60 MB).
// The cost is nil at the shipped 512 MB default budget, which clears both
// thresholds with wide margin; it only reaches a caller who configured a
// budget close to a witness's pre-B9 threshold. Closing the remaining gap
// to the 6x contract needs either an owner decision to accept the
// divisor-discount coverage cost described above, or a deeper change to
// reduce the scheduler's own per-token ephemeral allocation rate (out of
// this tranche's scope). See the tranche's PR for the full witness table.
const stopControlFootprintChurnRatio = 1

func diagnosticParserCoreSliceAliases[T any](items []T, inline []T) bool {
	if cap(items) == 0 || len(inline) == 0 {
		return false
	}
	return unsafe.Pointer(&items[:cap(items)][0]) == unsafe.Pointer(&inline[0])
}

type diagnosticParserCoreFootprintRef struct {
	pointer unsafe.Pointer
	kind    uint8
}

const (
	diagnosticParserCoreFootprintState uint8 = iota + 1
	diagnosticParserCoreFootprintRegion
	diagnosticParserCoreFootprintSnapshot
)

func appendDiagnosticParserCoreFootprintRef(
	refs []diagnosticParserCoreFootprintRef,
	kind uint8,
	pointer unsafe.Pointer,
) []diagnosticParserCoreFootprintRef {
	if pointer == nil {
		return refs
	}
	return append(refs, diagnosticParserCoreFootprintRef{pointer: pointer, kind: kind})
}

func appendDiagnosticParserCoreVersionFootprintRefs(
	refs []diagnosticParserCoreFootprintRef,
	state *diagnosticParserCoreVersionState,
) []diagnosticParserCoreFootprintRef {
	if state == nil {
		return refs
	}
	refs = appendDiagnosticParserCoreFootprintRef(refs, diagnosticParserCoreFootprintState, unsafe.Pointer(state))
	refs = appendDiagnosticParserCoreFootprintRef(refs, diagnosticParserCoreFootprintRegion, unsafe.Pointer(state.s3Region))
	return appendDiagnosticParserCoreFootprintRef(refs, diagnosticParserCoreFootprintSnapshot, unsafe.Pointer(state.relexSnapshot))
}

func appendDiagnosticParserCoreHeaderFootprintRefs(
	refs []diagnosticParserCoreFootprintRef,
	headers []diagnosticParserCoreHeader,
) []diagnosticParserCoreFootprintRef {
	for index := 0; index < cap(headers); index++ {
		refs = appendDiagnosticParserCoreVersionFootprintRefs(refs, headers[:cap(headers)][index].versionState)
	}
	return refs
}

func appendDiagnosticParserCoreCanonicalScratchFootprintRefs(
	refs []diagnosticParserCoreFootprintRef,
	scratch *diagnosticParserCoreCanonicalScratch,
) []diagnosticParserCoreFootprintRef {
	if scratch == nil {
		return refs
	}
	for _, key := range scratch.keys {
		refs = appendDiagnosticParserCoreVersionFootprintRefs(refs, key.versionState)
	}
	for key := range scratch.groups {
		refs = appendDiagnosticParserCoreVersionFootprintRefs(refs, key.versionState)
	}
	return refs
}

func appendDiagnosticParserCoreVersionLexerRequestFootprintRefs(
	refs []diagnosticParserCoreFootprintRef,
	requests []diagnosticParserCoreVersionLexerRequest,
) []diagnosticParserCoreFootprintRef {
	for index := range requests {
		refs = appendDiagnosticParserCoreFootprintRef(refs, diagnosticParserCoreFootprintSnapshot, unsafe.Pointer(requests[index].before))
		refs = appendDiagnosticParserCoreFootprintRef(refs, diagnosticParserCoreFootprintSnapshot, unsafe.Pointer(requests[index].after))
	}
	return refs
}

func diagnosticParserCoreDFARelexSnapshotRetainedBytes(snapshot dfaRelexSnapshot) uint64 {
	total := uint64(0)
	add := func(count int, size uintptr) {
		if count <= 0 || size == 0 || total == math.MaxUint64 {
			return
		}
		if uint64(count) > math.MaxUint64/uint64(size) {
			total = math.MaxUint64
			return
		}
		bytes := uint64(count) * uint64(size)
		if math.MaxUint64-total < bytes {
			total = math.MaxUint64
			return
		}
		total += bytes
	}
	add(cap(snapshot.externalPayload), 1)
	add(cap(snapshot.externalTokenStart), 1)
	add(cap(snapshot.externalTokenEnd), 1)
	add(cap(snapshot.extZeroTried), unsafe.Sizeof(bool(false)))
	return total
}

func diagnosticParserCoreDFARelexSnapshotAndScratchRetainedBytes(
	snapshot dfaRelexSnapshot,
	scratch dfaRelexSnapshotScratch,
) uint64 {
	total := uint64(0)
	addPair := func(left, right unsafe.Pointer, leftCap, rightCap int, size uintptr) {
		if size == 0 || total == math.MaxUint64 {
			return
		}
		count := leftCap + rightCap
		if left != nil && left == right {
			count = max(leftCap, rightCap)
		}
		if count <= 0 || uint64(count) > math.MaxUint64/uint64(size) {
			if count > 0 {
				total = math.MaxUint64
			}
			return
		}
		bytes := uint64(count) * uint64(size)
		if math.MaxUint64-total < bytes {
			total = math.MaxUint64
			return
		}
		total += bytes
	}
	bytePointer := func(items []byte) unsafe.Pointer {
		if cap(items) == 0 {
			return nil
		}
		return unsafe.Pointer(&items[:cap(items)][0])
	}
	boolPointer := func(items []bool) unsafe.Pointer {
		if cap(items) == 0 {
			return nil
		}
		return unsafe.Pointer(&items[:cap(items)][0])
	}
	addPair(bytePointer(snapshot.externalPayload), bytePointer(scratch.externalPayload), cap(snapshot.externalPayload), cap(scratch.externalPayload), 1)
	addPair(bytePointer(snapshot.externalTokenStart), bytePointer(scratch.externalTokenStart), cap(snapshot.externalTokenStart), cap(scratch.externalTokenStart), 1)
	addPair(bytePointer(snapshot.externalTokenEnd), bytePointer(scratch.externalTokenEnd), cap(snapshot.externalTokenEnd), cap(scratch.externalTokenEnd), 1)
	addPair(boolPointer(snapshot.extZeroTried), boolPointer(scratch.extZeroTried), cap(snapshot.extZeroTried), cap(scratch.extZeroTried), unsafe.Sizeof(bool(false)))
	return total
}

func compareDiagnosticParserCoreFootprintRefs(
	left, right diagnosticParserCoreFootprintRef,
) int {
	if left.kind < right.kind {
		return -1
	}
	if left.kind > right.kind {
		return 1
	}
	leftPointer, rightPointer := uintptr(left.pointer), uintptr(right.pointer)
	if leftPointer < rightPointer {
		return -1
	}
	if leftPointer > rightPointer {
		return 1
	}
	return 0
}

func diagnosticParserCoreSchedulerFootprintBytes(s *diagnosticParserCoreGenericScheduler) uint64 {
	if s == nil {
		return 0
	}
	total := uint64(cap(s.reuseDependencies.ends)) * 4
	addBytes := func(bytes uint64) {
		if total == math.MaxUint64 || bytes == 0 {
			return
		}
		if math.MaxUint64-total < bytes {
			total = math.MaxUint64
			return
		}
		total += bytes
	}
	add := func(count int, size uintptr) {
		if count <= 0 || size == 0 || total == math.MaxUint64 {
			return
		}
		countBytes := uint64(count)
		sizeBytes := uint64(size)
		if countBytes > math.MaxUint64/sizeBytes {
			total = math.MaxUint64
			return
		}
		addBytes(countBytes * sizeBytes)
	}
	addBytes(s.options.compactIncrementalReuse.footprintBytes())
	// Header copies share immutable version-state pointers. Count each owned
	// wrapper, region, and lexer snapshot once across the active frontier,
	// canonical keys/groups, and retained header scratch. The scheduler-owned
	// buffer keeps repeated polls allocation-free while growing exactly for a
	// wider frontier.
	refs := s.footprintRefs[:0]
	defer func() {
		clear(refs)
		s.footprintRefs = refs[:0]
	}()
	refs = appendDiagnosticParserCoreHeaderFootprintRefs(refs, s.headers)
	refs = appendDiagnosticParserCoreHeaderFootprintRefs(refs, s.headerRollbackScratch.headers)
	refs = appendDiagnosticParserCoreHeaderFootprintRefs(refs, s.headerRollbackScratch.inline[:])
	for index := range s.canonicalScratch.inlineHeaders {
		refs = appendDiagnosticParserCoreHeaderFootprintRefs(refs, s.canonicalScratch.inlineHeaders[index][:])
		refs = appendDiagnosticParserCoreHeaderFootprintRefs(refs, s.canonicalScratch.headerBuffers[index])
	}
	refs = appendDiagnosticParserCoreHeaderFootprintRefs(refs, s.conflictScratch.outputs)
	refs = appendDiagnosticParserCoreHeaderFootprintRefs(refs, s.conflictScratch.headerAssembly)
	refs = appendDiagnosticParserCoreHeaderFootprintRefs(refs, s.reductionReplacements)
	refs = appendDiagnosticParserCoreHeaderFootprintRefs(refs, s.seedHeaders[:])
	refs = appendDiagnosticParserCoreCanonicalScratchFootprintRefs(refs, &s.canonicalScratch)
	refs = appendDiagnosticParserCoreVersionLexerRequestFootprintRefs(refs, s.versionLexerRequests)
	slices.SortFunc(refs, compareDiagnosticParserCoreFootprintRefs)
	for index := 0; index < len(refs); {
		ref := refs[index]
		next := index + 1
		for next < len(refs) && compareDiagnosticParserCoreFootprintRefs(ref, refs[next]) == 0 {
			next++
		}
		switch ref.kind {
		case diagnosticParserCoreFootprintState:
			addBytes(uint64(unsafe.Sizeof(diagnosticParserCoreVersionState{})))
		case diagnosticParserCoreFootprintRegion:
			addBytes(diagnosticParserCoreVersionS3RegionFootprintBytes((*diagnosticParserCoreS3Region)(ref.pointer)))
		case diagnosticParserCoreFootprintSnapshot:
			addBytes(diagnosticParserCoreVersionLexerSnapshotFootprintBytes((*diagnosticParserCoreVersionLexerSnapshot)(ref.pointer)))
		}
		index = next
	}
	add(cap(refs), unsafe.Sizeof(diagnosticParserCoreFootprintRef{}))
	add(cap(s.headers), unsafe.Sizeof(diagnosticParserCoreHeader{}))
	add(cap(s.summaryHeaderScratch), unsafe.Sizeof(DiagnosticParserCoreHeaderReceipt{}))
	add(len(s.headerRollbackScratch.inline), unsafe.Sizeof(diagnosticParserCoreHeader{}))
	if !diagnosticParserCoreSliceAliases(s.headerRollbackScratch.headers, s.headerRollbackScratch.inline[:]) {
		add(cap(s.headerRollbackScratch.headers), unsafe.Sizeof(diagnosticParserCoreHeader{}))
	}
	add(len(s.canonicalScratch.inlineHeaders[0]), unsafe.Sizeof(diagnosticParserCoreHeader{}))
	add(len(s.canonicalScratch.inlineHeaders[1]), unsafe.Sizeof(diagnosticParserCoreHeader{}))
	for index := range s.canonicalScratch.headerBuffers {
		aliasesInline := diagnosticParserCoreSliceAliases(s.canonicalScratch.headerBuffers[index], s.canonicalScratch.inlineHeaders[0][:]) ||
			diagnosticParserCoreSliceAliases(s.canonicalScratch.headerBuffers[index], s.canonicalScratch.inlineHeaders[1][:])
		if !aliasesInline {
			add(cap(s.canonicalScratch.headerBuffers[index]), unsafe.Sizeof(diagnosticParserCoreHeader{}))
		}
	}
	if !diagnosticParserCoreSliceAliases(s.canonicalScratch.keys, s.canonicalScratch.inlineKeys[:]) {
		add(cap(s.canonicalScratch.keys), unsafe.Sizeof(diagnosticParserCorePhaseHead{}))
	}
	add(len(s.canonicalScratch.inlineKeys), unsafe.Sizeof(diagnosticParserCorePhaseHead{}))
	addBytes(s.canonicalScratch.groupsRetainedBytes)
	add(cap(s.dispatchScratch.cells), unsafe.Sizeof(diagnosticParserCoreGenericCell{}))
	add(cap(s.dispatchScratch.noActionIndices), unsafe.Sizeof(int(0)))
	add(cap(s.conflictScratch.actionOutputs), unsafe.Sizeof(diagnosticParserCoreActionOutput{}))
	add(cap(s.conflictScratch.reductionOutputs), unsafe.Sizeof(core.ReductionOutput{}))
	add(cap(s.conflictScratch.outputs), unsafe.Sizeof(diagnosticParserCoreHeader{}))
	add(cap(s.conflictScratch.armRanges), unsafe.Sizeof(diagnosticParserCoreConflictArmRange{}))
	add(cap(s.conflictScratch.adopted), unsafe.Sizeof(int(0)))
	add(cap(s.conflictScratch.headerAssembly), unsafe.Sizeof(diagnosticParserCoreHeader{}))
	add(cap(s.reductionOutputs), unsafe.Sizeof(core.ReductionOutput{}))
	add(cap(s.reductionReplacements), unsafe.Sizeof(diagnosticParserCoreHeader{}))
	add(cap(s.recoverySymbols), unsafe.Sizeof(core.SelectedSymbolPolicy{}))
	add(cap(s.recoveryCondenseScratch), unsafe.Sizeof(diagnosticParserCoreRecoveryCondenseEntry{}))
	add(cap(s.recoveryCondenseOrderScratch), unsafe.Sizeof(int(0)))
	add(cap(s.classifiedBoundaries), unsafe.Sizeof(core.ClassifiedBoundary{}))
	add(cap(s.condenseCandidates), unsafe.Sizeof(core.CondenseCandidate{}))
	add(cap(s.electStates), unsafe.Sizeof(StateID(0)))
	add(cap(s.electGLRStates), unsafe.Sizeof(StateID(0)))
	add(cap(s.acceptedPayloads), unsafe.Sizeof(core.SubtreeID(0)))
	add(cap(s.versionLexerRequests), unsafe.Sizeof(diagnosticParserCoreVersionLexerRequest{}))
	addBytes(diagnosticParserCoreDFARelexSnapshotAndScratchRetainedBytes(
		s.versionLexerBefore, s.versionLexerBeforeScratch,
	))
	add(len(s.seedHeaders), unsafe.Sizeof(diagnosticParserCoreHeader{}))
	add(len(s.corridorCells), unsafe.Sizeof(diagnosticParserCoreGenericCell{}))
	add(1, unsafe.Sizeof(s.recoveryTurns))
	if s.compact != nil {
		coreBytes := s.compact.FootprintBytes()
		if math.MaxUint64-total < coreBytes {
			total = math.MaxUint64
		} else {
			total += coreBytes
		}
	}
	return total
}

// stopControlMemoryBudgetReason compares the compact core's and scheduler's real
// retained-memory footprint against the production engine's soft per-parse
// byte budget (stopControlMemoryBudgetBytes, sourced from
// parseMemoryBudgetForParser so the same GOT_PARSE_MEMORY_BUDGET_MB
// configuration governs both engines) and, independently, against
// production's own absolute hard ceiling (stopControlHardCeilingBytes,
// armed even when the soft budget is disabled). Every input is
// Core.FootprintBytes(): already-tracked slice/map length and capacity
// reads times compile-time-constant record sizes, so this is pure
// deterministic integer arithmetic -- no wall clock, no GC-timing
// dependence, and (same input, same budget) the same trip point on every
// run, unlike the runtime heap/sys signal production's own hard ceiling
// poll uses (parser_memory_budget_runtime.go). The scheduler contribution
// counts retained and ephemeral slice capacities, while Core.FootprintBytes
// counts Core-owned spill only once. FootprintBytes, not
// StorageBytes, is deliberate here: StorageBytes counts live length only,
// so it reads near zero for a core whose arenas hold retained capacity from
// an earlier declined attempt on the same cached runner, and it never
// counted scratch, the boundary index, or checkpoint interning at all --
// either gap let a pathological input's real footprint clear the configured
// budget well before this poll noticed (tranche B9 honest-accounting gate).
// See stopControlFootprintChurnRatio's doc comment for the ephemeral-churn
// gap this gauge still has, and why closing it further is left as an owner
// decision rather than a silent default change.
func (s *diagnosticParserCoreGenericScheduler) stopControlMemoryBudgetReason() ParseStopReason {
	return s.stopControlMemoryBudgetReasonWithAdditionalBytes(0)
}

// stopControlMemoryBudgetReasonWithAdditionalBytes includes the materialization
// arena and coverage scratch during fresh and incremental execution.
func (s *diagnosticParserCoreGenericScheduler) stopControlMemoryBudgetReasonWithAdditionalBytes(additional uint64) ParseStopReason {
	if s == nil {
		return ParseStopNone
	}
	budget := s.options.stopControlMemoryBudgetBytes
	ceiling := s.options.stopControlHardCeilingBytes
	if budget <= 0 && ceiling <= 0 {
		return ParseStopNone
	}
	ratio := uint64(stopControlFootprintChurnRatio)
	scaledFootprint := func(footprint uint64) uint64 {
		if additional > math.MaxUint64-footprint {
			footprint = math.MaxUint64
		} else {
			footprint += additional
		}
		if ratio != 0 && footprint > math.MaxUint64/ratio {
			return math.MaxUint64
		}
		return footprint * ratio
	}
	// Recompute after every scheduler operation. A prior small footprint does
	// not bound capacity growth before the next poll.
	exact := diagnosticParserCoreSchedulerFootprintBytes(s)
	scaled := scaledFootprint(exact)
	if budget > 0 && scaled >= uint64(budget) {
		return ParseStopMemoryBudget
	}
	if ceiling > 0 && scaled >= uint64(ceiling) {
		return ParseStopMemoryBudget
	}
	return ParseStopNone
}

// pollStopControl is the bounded scheduler-boundary poll (spec.campaign.v7
// tranche B8): the memory-budget check above, then the exact production
// deadline and cancellation check. Admission candidates also run the node-cap
// predictor after those controls. The poll runs before the first election,
// once per dispatch loop, and during an S5 terminal scan. Diagnostic callers
// bind no Parser, so they do not run the predictor.
func (s *diagnosticParserCoreGenericScheduler) pollStopControl() error {
	// The eager materializer's arena is live storage of this run, so the
	// memory budget charges it the way the accepted-tree pass does.
	additional := uint64(0)
	eager := s.eagerMaterializerActive()
	if eager != nil {
		additional = arenaAllocatedVolume(eager.arena)
	}
	if reason := s.stopControlMemoryBudgetReasonWithAdditionalBytes(additional); reason != ParseStopNone {
		return diagnosticParserCoreStopControlTripped(reason)
	}
	parser := s.options.stopControlParser
	if parser == nil {
		return nil
	}
	if eager != nil {
		// The accepted-tree pass polls the arena every 256 subtrees. Keep
		// that cadence here instead of one poll per dispatch loop.
		s.eagerPolls++
		if s.eagerPolls&255 == 0 {
			if reason := parser.resultMaterializationStopReason(eager.arena); resultMaterializationShouldStop(reason) {
				return diagnosticParserCoreStopControlTripped(reason)
			}
		}
	}
	if reason := parser.activeParseStopReason(); parseStopReasonIsActive(reason) {
		return diagnosticParserCoreStopControlTripped(reason)
	}
	return s.observeCapPressure()
}
