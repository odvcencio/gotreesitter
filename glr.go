package gotreesitter

import (
	"fmt"
	"os"
	"unsafe"
)

// glrStack is one version of the parse stack in a GLR parser.
// When the parse table has multiple actions for a (state, symbol) pair,
// the parser forks: one glrStack per alternative. Stacks that hit errors
// are dropped; surviving stacks are merged when their top states converge.
type glrStack struct {
	gss gssStack
	// entries is the fast-path contiguous stack used before any GLR forks.
	// Once a stack is promoted to GSS (shared-prefix), entries becomes an
	// optional cached materialization for indexed reduce/recover access.
	entries []stackEntry
	// cacheEntries keeps a materialized entries cache on this stack when true.
	// We generally keep this enabled only for the primary stack.
	cacheEntries bool
	// byteOffset tracks the end byte of the latest non-nil node on stack.
	// It avoids rescanning entries in merge/retention hot paths.
	byteOffset uint32
	// score tracks dynamic precedence accumulated through reduce actions.
	// It is used for tie-breaking when choosing a final parse.
	score int
	// dead marks a stack version that encountered an error and should be
	// removed at the next merge point.
	dead bool
	// accepted is set when the stack reaches a ParseActionAccept.
	accepted bool
	// shifted is set when this stack consumed the current token via a SHIFT
	// action in a GLR fork that also produced REDUCE actions. When the
	// reducing stacks cause the same token to be re-processed, shifted
	// stacks must be skipped since they already consumed it.
	shifted bool
	// recoverabilityKnown indicates whether mayRecover can be trusted as
	// a conservative "stack may contain recover-capable states" bit.
	recoverabilityKnown bool
	// mayRecover is true when the stack is known to contain at least one
	// state that can perform ParseActionRecover for some symbol.
	mayRecover bool
	// branchOrder preserves original GLR fork order for exact-tie selection.
	// Lower values correspond to earlier parse-table actions.
	branchOrder uint64
	// cRec marks the stack as being in tree-sitter C's ERROR_STATE under the
	// faithful recovery port (parser_recover_c.go). nil for every grammar not
	// gated by errorCostCompetitionLanguage, and for stacks not in error.
	cRec *cRecoverState
	// cRecoverMissingGroup marks a non-error stack created by C's
	// recover_with_missing for the given recovery group. C lexes per version,
	// so the missing sibling can lag behind the error-state version; the Go
	// lockstep token loop uses this to avoid letting an already-advanced
	// sibling suppress that group's Strategy 1 recovery.
	cRecoverMissingGroup *cRecGroup
	// cPaused mirrors C StackStatusPaused: the stack hit a no-action point
	// under the gated recovery port and waits for the condense step to either
	// resume it (ts_parser__handle_error) or remove it. Only ever set when
	// errorCostCompetitionLanguage gates the grammar.
	cPaused bool
	// cNodeBaseline mirrors C StackHead.node_count_at_last_error: the stack's
	// cumulative visible-node count when the error discontinuity was last
	// pushed. Zero for stacks that never entered the C error state.
	cNodeBaseline int
	// cEverErrored is a sticky per-stack bit: true once this lineage has entered
	// C's error handling at any point, even if it later recovered to a clean tree
	// with cRec cleared and cNodeBaseline reset (or clamped) back to zero. Unlike
	// cRec/cPaused/cNodeBaseline — every one of which a recovered fork can
	// legitimately reset to its pristine value — this bit never clears, so a gate
	// that must distinguish "provably never touched recovery" from "recovered
	// wreckage that now looks pristine" (the php comma-list gate in
	// conflict_policy.go) can rely on it where cNodeBaseline==0 is not airtight
	// (a baseline can be written or clamped back to 0). Set at every C-error
	// entry funnel (cHandleError entry, cApplyMergedErrorGroupBaseline,
	// cRecoverToState fork creation), propagated by clone()/cloneWithScratch(),
	// and OR-merged at the two GSS merge choke points (tryGSSMainMergeResult,
	// tryGSSMainMergeForParser) so a clean survivor cannot shed a merged-in
	// wreckage lineage's history.
	cEverErrored bool
	// cRecoveryUnvalidatedMarker is set on this lineage in exactly one place:
	// cRecoverToState (parser_recover_c.go), the moment it creates a real
	// ERROR node wrapping popped content (strategy 1 of ts_parser__recover),
	// AND only when the recovered span is small (see
	// crecoverySwallowedErrorMaxFallbackErrorBytes) and this dead end owned
	// the whole parse (crecoveryHandleErrorSingleStack). NOTE:
	// cAbsorbTokenIntoError (strategy 2) does NOT set it — a version
	// absorbing into an open error region already carries cRec != nil, which
	// cCondenseAndResume's own cost bookkeeping (cVersionStatus) already
	// accounts for without help from this field.
	//
	// A second trigger — tagging the surviving side of an ordinary
	// condense-step drop against a recovery-owning sibling, regardless of
	// span or stack count — was tried and rejected: an adversarial review's
	// corpus walk showed it firing thousands of times per large,
	// syntactically valid Go file (routine GLR disambiguation on the Go
	// grammar's LALR table), and because the tag propagates through every
	// subsequent clone() of the (usually eventually-selected) winning
	// lineage, the same-position sibling check below could not scope it back
	// down. The single-stack + small-span cRecoverToState trigger alone is
	// sufficient for the confirmed defect class (java/php/gomod).
	//
	// The marker is cleared as soon as the lineage re-enters cHandleError (at
	// which point cCondenseAndResume's cost competition properly accounts for
	// whatever content it carries, win or lose). If a lineage carries the
	// marker all the way to ACCEPT and is actually selected without ever
	// being re-validated by another competition, cStackErrorCost cannot have
	// legitimately dropped back to zero for it — see the check in
	// buildResultFromGLR (parser_result.go) that sets
	// Parser.crecoveryDroppedErrorForClean from this field, scoped to the
	// selected stack only, with a same-position sibling check.
	cRecoveryUnvalidatedMarker bool
}

const (
	defaultStackEntrySlabCap = 4 * 1024
	// Retain enough entry-scratch capacity to avoid re-allocating large
	// GLR stacks on every parse pass.
	// Benchmarked incremental workloads peak near ~256K entries; keep modest
	// headroom while avoiding very large retained scratch slabs.
	maxRetainedStackEntryCap = 512 * 1024
	// Hard cap on concurrently retained GLR stacks in parseInternal.
	// Kept intentionally tight for parse speed. Full parses that stop with no
	// live stacks can retry once at a higher cap.
	maxGLRStacks = 8
	// Per-merge-key survivor cap. Tuned below 8 to reduce full-parse GLR churn
	// while keeping corpus parity and correctness gates green.
	maxStacksPerMergeKey = 6
	// Retry parses can temporarily widen the merge fanout beyond the default
	// survivor cap without changing the steady-state parser behavior.
	maxStacksPerMergeKeyCeiling = 256
	// Hard emergency cap before allocating per-key merge slots. Normal parser
	// culling keeps live stacks far below this, so this only applies to
	// pathological GLR bursts that would otherwise allocate huge slot tables
	// before the next memory-budget check can run.
	maxMergeAliveStacks = 4096
	// Keep ordinary merge scratch hot while dropping pathological buffers after
	// the parse. glrMergeSlot is intentionally large because it owns fixed
	// per-key survivor arrays.
	maxRetainedMergeResultCap = 4096
	maxRetainedMergeSlotCap   = 1024
)

type glrMergeScratch struct {
	result            []glrStack
	slots             []glrMergeSlot
	largeSlots        []glrMergeLargeSlot
	perKeyCap         int
	language          *Language
	deferExactDedupe  bool
	frontierMergeHash bool
	trace             bool
	cRecoveryCost     bool
	audit             *runtimeAudit
	equivEpoch        uint32
	equivCache        []glrNodeEquivCacheEntry
	stackEquivCache   []glrStackEquivCacheEntry
	spineEquivCache   []glrSpineEquivCacheEntry
	frontierHashCache []glrStackFrontierHashCacheEntry
	cErrorCost        map[*Node]glrCErrorCostEntry
	// cPrefixPath is the descent scratch for merge-side GSS prefix-aggregate
	// fills (cStackPrefixCostForMerge, parser_recover_c.go); the aggregates
	// live on gssNode, validated against gssPrefixAggGen.
	cPrefixPath []*gssNode
	// spineVisit is the reusable descent scratch for the spine-equality memo
	// (gssSpineEqualMemo): the pairs walked before the deciding position, so the
	// single computed answer can be back-filled to every shallower spine pair.
	spineVisit       []spinePairKey
	shapePrefixCache []glrShapePrefixCacheEntry
	shapePrefixEpoch uint32
	shapePrefixBytes int64
	cleanZeroEpoch   uint32
	cleanZeroScan    uint32
	cleanZeroCache   map[*gssNode]gssCleanZeroErrorCacheEntry
	cleanZeroFront   []glrCleanZeroFrontCacheEntry
	cleanZeroBytes   int64
	cleanZeroStack   []*gssNode
	cleanZeroVisited []*gssNode
	// preflight + mergeSeen are pooled per-scratch so the GSS merge can-phase
	// stops allocating a preflight object plus several maps per merge attempt
	// (the dominant profile cost on low-stack GLR grammars like json — maps
	// were rebuilt every canMergeNodes call).
	preflight         *gssMainPreflight
	mergeSeen         map[gssMergePair]bool
	pythonShallow     bool
	budgetBytes       int64
	resultBytes       int64
	slotBytes         int64
	largeSlotBytes    int64
	equivCacheBytes   int64
	stackEquivBytes   int64
	spineEquivBytes   int64
	frontierHashBytes int64
}

type glrCErrorCostEntry struct {
	ver  uint32
	cost uint32
}

// glrMaterializingShapeHash is, per GSS node, the rolling materializing
// shape hash of the spine prefix root..node (inclusive) plus the number of
// materializing entries in that prefix. Cached in the set-associative
// glrMergeScratch.shapePrefixCache, keyed by (node pointer, shapePrefixEpoch).
// An entry stays valid until the epoch is bumped (bumpShapePrefixEpoch). The
// epoch is bumped on every mutation that can change a live node's root->head
// prefix hash:
//   - the start of each parse epoch (beginEquivEpoch);
//   - a successful GSS main merge that rewrites link 0 of surviving nodes, both
//     the collapse-phase merge (tryGSSMainMergeResult) and the dispatch-time
//     merge (tryGSSMainMergeForParser via gssMainMerge -> setGSSMainLink);
//   - retargetStackEntryPayload rewriting a spine node's parseState in place at
//     its reduce call sites (parseState feeds gssEntryHash, which the prefix
//     rolls in).
//
// Prefix caching makes the per-head shape hash O(new entries) instead of
// O(full spine), which is what made the merge attempt loop super-linear on
// large clean files (python cliff RCA 2026-07).
type glrMaterializingShapeHash struct {
	hash  uint64
	count uint32
}

// glrShapePrefixCacheEntry is one slot of the pointer-keyed 2-way
// set-associative materializing-shape prefix cache (same layout discipline as
// frontierHashCache — a Go map here was the top profile cost on wide-GLR
// python inputs).
type glrShapePrefixCacheEntry struct {
	node   uintptr
	epoch  uint32
	prefix glrMaterializingShapeHash
}

// glrCleanZeroFrontCacheEntry is one slot of the pointer-keyed 2-way
// set-associative front cache for gssNodeCleanZeroErrorAllLinksWithScratch
// results. The map cache (cleanZeroCache) stays authoritative — it also holds
// DFS scan bookkeeping — but the hot repeated lookups (cost fast path +
// gssMainCanMergeWithScratch, several per merge attempt) hit this array
// first. clean is only valid when epoch matches the scratch cleanZeroEpoch.
type glrCleanZeroFrontCacheEntry struct {
	node  uintptr
	epoch uint32
	clean bool
}

type glrMergeKey struct {
	state      StateID
	byteOffset uint32
}

type glrMergeSlot struct {
	key          glrMergeKey
	indices      [maxStacksPerMergeKey]int
	hashes       [maxStacksPerMergeKey]uint64
	extraIndices []int
	extraHashes  []uint64
	hashMask     uint64
	count        int
	worstIndex   int
}

type glrMergeLargeSlot struct {
	key        glrMergeKey
	indices    [maxStacksPerMergeKeyCeiling]int
	hashes     [maxStacksPerMergeKeyCeiling]uint64
	hashMask   uint64
	count      int
	worstIndex int
}

type glrNodeEquivCacheEntry struct {
	a        uintptr
	b        uintptr
	aVersion uint32
	bVersion uint32
	epoch    uint32
	depth    uint16
	result   bool
}

type glrStackEquivCacheEntry struct {
	a      uintptr
	b      uintptr
	epoch  uint32
	result bool
}

// glrSpineEquivCacheEntry is one slot of the pointer-keyed 2-way set-
// associative spine-equality memo. Key: the ordered gssNode pointer pair.
// Validity: epoch (per-parse) plus both prev-chain prefix fingerprints
// (gssNode.hash). A hit requires the pointers AND both fingerprints to match,
// so recycled slab pointers (fresh hash on allocNode) and in-place prefix
// mutations (hash reset to 0 then recomputed at the mutation sites) both force
// a miss rather than a stale answer.
type glrSpineEquivCacheEntry struct {
	a      uintptr
	b      uintptr
	aHash  uint64
	bHash  uint64
	epoch  uint32
	result bool
}

type glrStackFrontierHashCacheEntry struct {
	node  uintptr
	epoch uint32
	hash  uint64
}

type gssCleanZeroErrorCacheEntry struct {
	resultEpoch uint32
	scanEpoch   uint32
	clean       bool
}

type glrEntryScratch struct {
	slabs          []stackEntrySlab
	slabCursor     int
	usedTotal      int
	peakUsed       int
	allocatedBytes int64
}

type stackEntrySlab struct {
	data []stackEntry
	used int
}

func (s *glrEntryScratch) ensureInitialCap(minEntries int) {
	if minEntries <= 0 || len(s.slabs) != 0 {
		return
	}
	capacity := defaultStackEntrySlabCap
	if minEntries > capacity {
		capacity = minEntries
	}
	s.slabs = append(s.slabs, stackEntrySlab{data: make([]stackEntry, capacity)})
	s.allocatedBytes += stackEntryBytesForCap(capacity)
	s.slabCursor = 0
}

func newGLRStack(initial StateID) glrStack {
	return glrStack{
		entries:      []stackEntry{{state: initial}},
		cacheEntries: true,
	}
}

func newGLRStackWithScratch(initial StateID, scratch *glrEntryScratch) glrStack {
	return newGLRStackWithScratchCap(initial, scratch, 256*1024)
}

func newGLRStackWithScratchCap(initial StateID, scratch *glrEntryScratch, maxInitialCap int) glrStack {
	if scratch == nil {
		return newGLRStack(initial)
	}
	initialCap := 8
	if len(scratch.slabs) > 0 {
		// Reuse slab headroom for the primary stack to avoid repeated
		// grow/copy churn on deep parses.
		initialCap = len(scratch.slabs[0].data)
		if maxInitialCap <= 0 {
			maxInitialCap = defaultStackEntrySlabCap
		}
		if initialCap > maxInitialCap {
			initialCap = maxInitialCap
		}
	} else {
		initialCap = defaultStackEntrySlabCap
	}
	entries := scratch.allocWithCap(1, initialCap)
	entries[0] = stackEntry{state: initial}
	return glrStack{entries: entries, cacheEntries: true}
}

func (s *glrStack) ensureGSS(scratch *gssScratch) {
	if s.gss.head != nil || len(s.entries) == 0 {
		return
	}
	s.gss = buildGSSStack(s.entries, scratch)
}

func (s *glrStack) depth() int {
	if s.gss.head != nil {
		return s.gss.len()
	}
	return len(s.entries)
}

func (s *glrStack) top() stackEntry {
	if s.gss.head != nil {
		return s.gss.top()
	}
	if len(s.entries) == 0 {
		return stackEntry{}
	}
	return s.entries[len(s.entries)-1]
}

func (s *glrStack) clone() glrStack {
	if s.gss.head == nil && len(s.entries) > 0 {
		entries := make([]stackEntry, len(s.entries))
		copy(entries, s.entries)
		return glrStack{
			entries:                    entries,
			cacheEntries:               s.cacheEntries,
			byteOffset:                 s.byteOffset,
			score:                      s.score,
			recoverabilityKnown:        s.recoverabilityKnown,
			mayRecover:                 s.mayRecover,
			branchOrder:                s.branchOrder,
			cRec:                       s.cRec.clone(),
			cRecoverMissingGroup:       s.cRecoverMissingGroup,
			cNodeBaseline:              s.cNodeBaseline,
			cRecoveryUnvalidatedMarker: s.cRecoveryUnvalidatedMarker,
			cEverErrored:               s.cEverErrored,
		}
	}
	s.ensureGSS(nil)
	return glrStack{
		gss:                        s.gss.clone(),
		cacheEntries:               s.cacheEntries,
		byteOffset:                 s.byteOffset,
		score:                      s.score,
		recoverabilityKnown:        s.recoverabilityKnown,
		mayRecover:                 s.mayRecover,
		branchOrder:                s.branchOrder,
		cRec:                       s.cRec.clone(),
		cRecoverMissingGroup:       s.cRecoverMissingGroup,
		cNodeBaseline:              s.cNodeBaseline,
		cRecoveryUnvalidatedMarker: s.cRecoveryUnvalidatedMarker,
		cEverErrored:               s.cEverErrored,
	}
}

func (s *glrStack) cloneWithScratch(scratch *gssScratch) glrStack {
	s.ensureGSS(scratch)
	return glrStack{
		gss:                        s.gss.clone(),
		cacheEntries:               false,
		byteOffset:                 s.byteOffset,
		score:                      s.score,
		recoverabilityKnown:        s.recoverabilityKnown,
		mayRecover:                 s.mayRecover,
		branchOrder:                s.branchOrder,
		cRec:                       s.cRec.clone(),
		cRecoverMissingGroup:       s.cRecoverMissingGroup,
		cNodeBaseline:              s.cNodeBaseline,
		cRecoveryUnvalidatedMarker: s.cRecoveryUnvalidatedMarker,
		cEverErrored:               s.cEverErrored,
	}
}

func (s *glrStack) ensureEntries(entryScratch *glrEntryScratch) []stackEntry {
	if s.entries != nil {
		return s.entries
	}
	if s.gss.head == nil {
		return nil
	}
	depth := s.gss.len()
	if depth == 0 {
		return nil
	}
	if entryScratch != nil {
		dst := entryScratch.allocWithCap(depth, depth+1)
		s.entries = s.gss.materialize(dst)
		return s.entries
	}
	entries := make([]stackEntry, depth)
	s.entries = s.gss.materialize(entries)
	return s.entries
}

func (s *glrStack) entriesForRead(tmp []stackEntry) ([]stackEntry, bool) {
	if s.entries != nil {
		return s.entries, false
	}
	if s.gss.head == nil {
		return nil, false
	}
	return s.gss.materialize(tmp), true
}

func (s *glrStack) push(state StateID, node *Node, entryScratch *glrEntryScratch, gssScratch *gssScratch) {
	s.pushEntry(newStackEntryNode(state, node), entryScratch, gssScratch)
}

func (s *glrStack) pushEntry(entry stackEntry, entryScratch *glrEntryScratch, gssScratch *gssScratch) {
	if s.gss.head != nil {
		s.gss.pushEntry(entry, gssScratch)
	}
	if s.entries != nil {
		if entryScratch == nil {
			s.entries = append(s.entries, entry)
		} else {
			if len(s.entries) == cap(s.entries) {
				s.entries = entryScratch.grow(s.entries, len(s.entries)+1)
			}
			idx := len(s.entries)
			s.entries = s.entries[:idx+1]
			s.entries[idx] = entry
		}
	} else if s.gss.head == nil {
		s.entries = []stackEntry{entry}
	}
	if stackEntryHasNode(entry) {
		s.byteOffset = stackEntryNodeEndByte(entry)
	}
}

func (s *glrStack) truncate(depth int) bool {
	if s.gss.head != nil {
		if !s.gss.truncate(depth) {
			return false
		}
		if s.entries != nil {
			if depth <= len(s.entries) {
				s.entries = s.entries[:depth]
			} else {
				s.entries = s.gss.materialize(s.entries[:0])
			}
		}
		s.byteOffset = s.gss.byteOffset()
		return true
	}
	if depth < 0 || depth > len(s.entries) {
		return false
	}
	s.entries = s.entries[:depth]
	s.byteOffset = stackByteOffset(s.entries)
	return true
}

func (s *glrStack) truncateBeforePush(depth int) bool {
	if s.gss.head != nil {
		if !s.gss.truncate(depth) {
			return false
		}
		if s.entries != nil {
			if depth <= len(s.entries) {
				s.entries = s.entries[:depth]
			} else {
				s.entries = s.gss.materialize(s.entries[:0])
			}
		}
		return true
	}
	if depth < 0 || depth > len(s.entries) {
		return false
	}
	s.entries = s.entries[:depth]
	return true
}

// mergeStacks removes dead stacks and collapses only truly duplicate
// active stacks. Two stacks are considered merge-compatible only when
// they share the same top parser state and byte position (matching the
// C runtime's stack merge preconditions), and their stack entries are
// identical. Distinct parse paths are preserved.
func mergeStacks(stacks []glrStack) []glrStack {
	var scratch glrMergeScratch
	scratch.beginEquivEpoch()
	return mergeStacksWithScratch(stacks, &scratch)
}

func stackByteOffset(entries []stackEntry) uint32 {
	for i := len(entries) - 1; i >= 0; i-- {
		if stackEntryHasNode(entries[i]) {
			return stackEntryNodeEndByte(entries[i])
		}
		if i == 0 {
			break
		}
	}
	return 0
}

func mergeKeyForStack(s glrStack) glrMergeKey {
	if s.depth() == 0 {
		return glrMergeKey{}
	}
	top := s.top()
	return glrMergeKey{
		state:      top.state,
		byteOffset: s.byteOffset,
	}
}

func stackHash(s glrStack) uint64 {
	if s.gss.head != nil {
		return gssNodeHash(s.gss.head)
	}
	if len(s.entries) == 0 {
		if perfCountersEnabled {
			perfRecordMergeHashZero()
		}
		return 0
	}
	// Entries-only stack (pre-fork primary). Compute the same rolling hash
	// GSS nodes use so per-bucket hash prefiltering works before GSS materializes.
	h := gssHashSeed
	for i := range s.entries {
		h = gssEntryHash(h, s.entries[i])
	}
	return h
}

func stackHashForMerge(scratch *glrMergeScratch, lang *Language, s glrStack) uint64 {
	if scratch != nil && scratch.frontierMergeHash && languageUsesGenericFrontierMergeHash(lang) {
		return stackHashGenericFrontier(scratch, s)
	}
	return stackHash(s)
}

func languageUsesGenericFrontierMergeHash(lang *Language) bool {
	return lang != nil && lang.Name == "perl"
}

func stackHashGenericFrontier(scratch *glrMergeScratch, s glrStack) uint64 {
	if s.gss.head != nil {
		return gssNodeGenericFrontierHash(scratch, s.gss.head)
	}
	if len(s.entries) == 0 {
		if perfCountersEnabled {
			perfRecordMergeHashZero()
		}
		return 0
	}
	h := gssHashSeed
	for i := range s.entries {
		h = gssEntryGenericFrontierHash(h, s.entries[i])
	}
	return h
}

func gssNodeGenericFrontierHash(scratch *glrMergeScratch, n *gssNode) uint64 {
	if n == nil {
		return gssHashSeed
	}
	if hash, ok := lookupStackFrontierHashCache(scratch, n); ok {
		return hash
	}

	var local [32]*gssNode
	pending := local[:0]
	prevHash := gssHashSeed
	for cur := n; cur != nil; cur = cur.prev {
		if hash, ok := lookupStackFrontierHashCache(scratch, cur); ok {
			prevHash = hash
			break
		}
		pending = append(pending, cur)
	}
	for i := len(pending) - 1; i >= 0; i-- {
		hash := gssEntryGenericFrontierHash(prevHash, pending[i].entry)
		if hash == 0 {
			hash = 1
		}
		storeStackFrontierHashCache(scratch, pending[i], hash)
		prevHash = hash
	}
	return prevHash
}

func gssEntryGenericFrontierHash(prev uint64, entry stackEntry) uint64 {
	h := prev ^ uint64(entry.state)
	h *= gssHashPrime
	if !stackEntryHasNode(entry) {
		h ^= gssNilNodeSentinel
		h *= gssHashPrime
		return h
	}
	if n := stackEntryNode(entry); n != nil {
		h ^= stackNodeGenericEquivSignature(n, stackEquivalentGenericFrontierDepthLimit)
		h *= gssHashPrime
		return h
	}
	h ^= stackEntryNonTreeEquivSignature(entry)
	h *= gssHashPrime
	return h
}

func stackEntryNonTreeEquivSignature(e stackEntry) uint64 {
	h := gssHashSeed
	h = mixStackEquivSignature(h, uint64(stackEntryNodeSymbol(e)))
	h = mixStackEquivSignature(h, (uint64(stackEntryNodeStartByte(e))<<32)|uint64(stackEntryNodeEndByte(e)))
	h = mixStackEquivSignature(h, uint64(stackEntryNodeChildCount(e)))
	h = mixStackEquivSignature(h, uint64(stackEntryNodeFieldIDCount(e)))
	h = mixStackEquivSignature(h, uint64(stackEntryNodeParseState(e)))
	h = mixStackEquivSignature(h, uint64(stackEntryNodePreGotoState(e)))
	h = mixStackEquivSignature(h, uint64(stackEntryNodeProductionID(e)))
	h = mixStackEquivSignature(h, uint64(uint32(stackEntryDynamicPrecedence(e))))
	h = mixStackEquivSignature(h, uint64(stackEntryNodeExactFlagBits(e)))
	return h
}

func stackNodeGenericEquivSignature(n *Node, depth int) uint64 {
	h := gssHashSeed
	if n == nil {
		return mixStackEquivSignature(h, gssNilNodeSentinel)
	}
	h = mixStackEquivSignature(h, uint64(n.symbol))
	h = mixStackEquivSignature(h, (uint64(n.startByte)<<32)|uint64(n.endByte))
	h = mixStackEquivSignature(h, uint64(len(n.children)))
	h = mixStackEquivSignature(h, uint64(n.flags&nodeStackEquivFlagMask))
	h = mixStackEquivSignature(h, uint64(n.parseState))
	h = mixStackEquivSignature(h, uint64(n.productionID))
	h = mixStackEquivSignature(h, uint64(uint32(n.dynamicPrecedence)))
	if n.flags&nodeFlagHasError != 0 {
		return h
	}
	if !stackNodeNeedsDeepEquivalent(n) {
		for i := range n.children {
			h = mixStackEquivSignature(h, stackNodeGenericShallowChildSignature(n.children[i]))
		}
		return h
	}

	h = mixStackEquivSignature(h, uint64(n.preGotoState))
	h = mixStackEquivSignature(h, uint64(len(n.fieldIDs)))
	for i := range n.fieldIDs {
		h = mixStackEquivSignature(h, uint64(n.fieldIDs[i]))
	}

	frontier := -1
	for i := range n.children {
		child := n.children[i]
		h = mixStackEquivSignature(h, stackNodeGenericFrontierChildSignature(child))
		if child != nil && child.flags&nodeFlagExtra == 0 && (child.flags&nodeFlagNamed != 0 || len(child.children) > 0) {
			frontier = i
		}
	}
	if depth == 0 {
		return h
	}

	candidates := [8]int{}
	candidateCount := 0
	addCandidate := func(idx int) {
		if idx < 0 {
			return
		}
		for i := 0; i < candidateCount; i++ {
			if candidates[i] == idx {
				return
			}
		}
		if candidateCount < len(candidates) {
			candidates[candidateCount] = idx
			candidateCount++
		}
	}
	if len(n.children) <= 3 {
		for i := range n.fieldIDs {
			if n.fieldIDs[i] == 0 || i >= len(n.children) {
				continue
			}
			child := n.children[i]
			if child == nil || child.flags&nodeFlagExtra != 0 || (child.flags&nodeFlagNamed == 0 && len(child.children) == 0) {
				continue
			}
			addCandidate(i)
		}
	}
	addCandidate(frontier)
	for i := 0; i < candidateCount; i++ {
		idx := candidates[i]
		h = mixStackEquivSignature(h, uint64(idx))
		h = mixStackEquivSignature(h, stackNodeGenericEquivSignature(n.children[idx], depth-1))
	}
	return h
}

func stackNodeGenericShallowChildSignature(n *Node) uint64 {
	h := gssHashSeed
	if n == nil {
		return mixStackEquivSignature(h, gssNilNodeSentinel)
	}
	h = mixStackEquivSignature(h, uint64(n.symbol))
	h = mixStackEquivSignature(h, (uint64(n.startByte)<<32)|uint64(n.endByte))
	h = mixStackEquivSignature(h, uint64(len(n.children)))
	h = mixStackEquivSignature(h, uint64(uint32(n.dynamicPrecedence)))
	h = mixStackEquivSignature(h, uint64(n.flags&nodeStackEquivNoMissingFlagMask))
	return h
}

func stackNodeGenericFrontierChildSignature(n *Node) uint64 {
	h := gssHashSeed
	if n == nil {
		return mixStackEquivSignature(h, gssNilNodeSentinel)
	}
	h = mixStackEquivSignature(h, uint64(n.symbol))
	h = mixStackEquivSignature(h, (uint64(n.startByte)<<32)|uint64(n.endByte))
	h = mixStackEquivSignature(h, uint64(len(n.children)))
	h = mixStackEquivSignature(h, uint64(len(n.fieldIDs)))
	h = mixStackEquivSignature(h, uint64(n.flags&nodeStackEquivFlagMask))
	h = mixStackEquivSignature(h, uint64(n.parseState))
	h = mixStackEquivSignature(h, uint64(n.preGotoState))
	h = mixStackEquivSignature(h, uint64(n.productionID))
	h = mixStackEquivSignature(h, uint64(uint32(n.dynamicPrecedence)))
	for i := range n.fieldIDs {
		h = mixStackEquivSignature(h, uint64(n.fieldIDs[i]))
	}
	return h
}

func stackMaterializingShapeHashWithScratch(scratch *glrMergeScratch, s glrStack) (uint64, bool) {
	if len(s.entries) == 0 {
		if s.gss.head == nil {
			return 0, false
		}
		prefix := gssMaterializingShapePrefix(scratch, s.gss.head)
		return finalizeMaterializingShapeHash(prefix)
	}

	h := gssHashSeed
	count := uint32(0)
	for i := range s.entries {
		if !stackEntryMaterializesForResult(s.entries[i]) {
			continue
		}
		h = gssEntryHash(h, s.entries[i])
		count++
	}
	return finalizeMaterializingShapeHash(glrMaterializingShapeHash{hash: h, count: count})
}

func finalizeMaterializingShapeHash(prefix glrMaterializingShapeHash) (uint64, bool) {
	if prefix.count == 0 {
		return 0, false
	}
	h := prefix.hash
	h ^= uint64(prefix.count)
	h *= gssHashPrime
	if h == 0 {
		h = 1
	}
	return h, true
}

// gssMaterializingShapePrefix returns the rolling materializing shape hash of
// the spine root..n along the primary (link 0) chain, memoizing every prefix
// in scratch.shapePrefixCache so a fresh head only pays for its own new
// entries. The rolling direction is root->head (matching the s.entries path
// and gssNodeHash) so shared prefixes are reusable across heads and tokens.
func gssMaterializingShapePrefix(scratch *glrMergeScratch, n *gssNode) glrMaterializingShapeHash {
	if n == nil {
		return glrMaterializingShapeHash{hash: gssHashSeed}
	}
	if cached, ok := lookupShapePrefixCache(scratch, n); ok {
		return cached
	}
	var local [32]*gssNode
	pending := local[:0]
	prefix := glrMaterializingShapeHash{hash: gssHashSeed}
	for cur := n; cur != nil; cur = cur.prev {
		if cached, ok := lookupShapePrefixCache(scratch, cur); ok {
			prefix = cached
			break
		}
		pending = append(pending, cur)
	}
	for i := len(pending) - 1; i >= 0; i-- {
		cur := pending[i]
		if stackEntryMaterializesForResult(cur.entry) {
			prefix = glrMaterializingShapeHash{
				hash:  gssEntryHash(prefix.hash, cur.entry),
				count: prefix.count + 1,
			}
		}
		storeShapePrefixCache(scratch, cur, prefix)
	}
	return prefix
}

func gssStacksHaveDistinctMaterializingShapesWithScratch(scratch *glrMergeScratch, a, b *glrStack) bool {
	if a == nil || b == nil {
		return false
	}
	aHash, aOK := stackMaterializingShapeHashWithScratch(scratch, *a)
	bHash, bOK := stackMaterializingShapeHashWithScratch(scratch, *b)
	return aOK && bOK && aHash != bHash
}

const (
	// glrNodeEquivCacheSize is sized to fit comfortably in L2 (16384 × 32 B = 512 KiB).
	// The previous 131072 entries (4 MiB) scattered random reads into L3/DRAM and made
	// lookupNodeEquivCache the #1 CPU hotspot (~23% flat on BenchmarkSelfParseWarmReuse).
	// 16K keeps the table cache-resident while reducing collision pressure on the
	// Java/C/Rust/TypeScript real-corpus matrix relative to 8K; 4K loses too many hits.
	//
	// LAYOUT: 2-way set associative. The 16K entries are grouped into 8K sets of
	// 2 slots each (primary + victim). Lookups check primary, then victim; on a
	// victim hit, the entry is promoted to primary (swap). On store, the previous
	// primary is evicted to the victim slot. This converts ~50% of direct-mapped
	// collision misses into victim hits on profiles where the working set fits
	// in ~2× the set count, which is the JS/Rust real-corpus shape.
	glrNodeEquivCacheSize     = 16384
	glrNodeEquivCacheSetCount = glrNodeEquivCacheSize / 2 // 8192 sets × 2 ways
	// glrStackEquivCacheSize memoizes GSS head-pair equivalence across merge
	// calls in a parse epoch. GSS nodes are immutable inside an epoch, so pointer
	// pairs are stable; this avoids repeatedly walking long shared stack tails in
	// GLR-heavy grammars such as Dart.
	glrStackEquivCacheSize     = 4096
	glrStackEquivCacheSetCount = glrStackEquivCacheSize / 2
	// glrSpineEquivCacheSize memoizes per-(node,node) GSS spine-equality — the
	// answer "are the two sub-stacks rooted at an and bn fully equivalent from
	// here to the root?" — checked at every spine position during a
	// gssStacksEqual walk (not just the head pair, which is new every token and
	// so rarely repeats). Unlike the epoch-only stackEquivCache, each entry
	// carries the prev-chain prefix fingerprint (gssNode.hash) of both nodes and
	// is valid only while both fingerprints are unchanged. gssNode.hash is a
	// Merkle-style prefix hash (hash(prev.hash, entry)), so it captures exactly
	// the data spine-equality depends on: an entry is the full-walk answer iff
	// both fingerprints match. This survives the ~2 gssPrefixAggGen bumps/token
	// (frontier merges) because deep spine nodes are not relinked and keep their
	// hash — the choke-point invalidation that resets the global gen leaves the
	// unchanged deep-spine fingerprints intact (wave-2b merge-equiv bucket).
	glrSpineEquivCacheSize     = 16384
	glrSpineEquivCacheSetCount = glrSpineEquivCacheSize / 2
	// glrStackFrontierHashCache memoizes the Perl-only frontier merge hash for
	// immutable GSS heads. It is intentionally smaller than the node-equivalence
	// cache: it only covers live stack heads encountered during merge bucketing.
	glrStackFrontierHashCacheSize     = 4096
	glrStackFrontierHashCacheSetCount = glrStackFrontierHashCacheSize / 2
	// glrShapePrefixCache covers every live spine node of every live stack (not
	// just heads): wide-GLR python inputs keep ~1.5K stacks × ~12 spine nodes
	// live at once, so this is sized like the node-equivalence cache.
	glrShapePrefixCacheSize     = 16384
	glrShapePrefixCacheSetCount = glrShapePrefixCacheSize / 2
	// glrCleanZeroFrontCache fronts the map-based cleanZeroCache for the
	// merge-attempt hot path (see glrCleanZeroFrontCacheEntry).
	glrCleanZeroFrontCacheSize     = 16384
	glrCleanZeroFrontCacheSetCount = glrCleanZeroFrontCacheSize / 2
	// Depth is part of the cache key. Keep it bounded so large recursive
	// comparisons cannot alias through a narrowing conversion.
	glrNodeEquivCacheMaxDepth = 1<<16 - 1
	// Exact TypeScript equivalence is independent of recursion depth. Use a
	// reserved depth key so exact entries do not fragment across ancestors or
	// collide with bounded frontier-equivalence entries.
	glrNodeEquivCacheExactDepth = glrNodeEquivCacheMaxDepth
)

func (s *glrMergeScratch) beginEquivEpoch() {
	if s == nil {
		return
	}
	s.beginCleanZeroEpoch()
	if s.equivEpoch == ^uint32(0) {
		clear(s.equivCache)
		clear(s.stackEquivCache)
		clear(s.spineEquivCache)
		clear(s.frontierHashCache)
		s.equivEpoch = 0
	}
	s.equivEpoch++
	if len(s.cErrorCost) > maxRetainedMergeResultCap {
		s.cErrorCost = nil
	} else if len(s.cErrorCost) > 0 {
		clear(s.cErrorCost)
	}
	s.bumpShapePrefixEpoch()
	if len(s.equivCache) == 0 {
		s.equivCache = make([]glrNodeEquivCacheEntry, glrNodeEquivCacheSize)
		s.equivCacheBytes = glrNodeEquivCacheBytesForCap(cap(s.equivCache))
	}
	// spineEquivCache is provisioned only for persistent parse scratches
	// (ensureMergeHotCaches), matching shapePrefixCache: one-shot local scratches
	// (standalone mergeStacks, diagnostics) skip spine memoization rather than
	// paying a 640KiB zeroed allocation per call. The lookup/store guards on an
	// empty backing array make that a correct no-memo fallback.
}

func stackFrontierHashCacheIndex(p uintptr) int {
	h := uint64(p)
	h ^= h >> 33
	h *= 0xff51afd7ed558ccd
	h ^= h >> 33
	h *= 0xc4ceb9fe1a85ec53
	h ^= h >> 33
	return int(h&uint64(glrStackFrontierHashCacheSetCount-1)) << 1
}

func lookupStackFrontierHashCache(scratch *glrMergeScratch, n *gssNode) (uint64, bool) {
	if scratch == nil || len(scratch.frontierHashCache) == 0 || scratch.equivEpoch == 0 || n == nil {
		return 0, false
	}
	p := uintptr(unsafe.Pointer(n))
	idx := stackFrontierHashCacheIndex(p)
	primary := &scratch.frontierHashCache[idx]
	if primary.epoch == scratch.equivEpoch && primary.node == p {
		return primary.hash, true
	}
	victim := &scratch.frontierHashCache[idx+1]
	if victim.epoch == scratch.equivEpoch && victim.node == p {
		scratch.frontierHashCache[idx], scratch.frontierHashCache[idx+1] = scratch.frontierHashCache[idx+1], scratch.frontierHashCache[idx]
		return scratch.frontierHashCache[idx].hash, true
	}
	return 0, false
}

func storeStackFrontierHashCache(scratch *glrMergeScratch, n *gssNode, hash uint64) {
	if scratch == nil || scratch.equivEpoch == 0 || n == nil {
		return
	}
	if len(scratch.frontierHashCache) == 0 {
		scratch.frontierHashCache = make([]glrStackFrontierHashCacheEntry, glrStackFrontierHashCacheSize)
		scratch.frontierHashBytes = glrStackFrontierHashCacheBytesForCap(cap(scratch.frontierHashCache))
	}
	p := uintptr(unsafe.Pointer(n))
	idx := stackFrontierHashCacheIndex(p)
	scratch.frontierHashCache[idx+1] = scratch.frontierHashCache[idx]
	scratch.frontierHashCache[idx] = glrStackFrontierHashCacheEntry{
		node:  p,
		epoch: scratch.equivEpoch,
		hash:  hash,
	}
}

func orderedGSSNodePair(a, b *gssNode) (uintptr, uintptr, bool) {
	if a == nil || b == nil {
		return 0, 0, false
	}
	ap := uintptr(unsafe.Pointer(a))
	bp := uintptr(unsafe.Pointer(b))
	if ap == 0 || bp == 0 {
		return 0, 0, false
	}
	if ap > bp {
		ap, bp = bp, ap
	}
	return ap, bp, true
}

func stackEquivCacheIndex(ap, bp uintptr) int {
	x := uint64(ap)
	y := uint64(bp)
	h := x ^ (y + 0x9e3779b97f4a7c15 + (x << 6) + (x >> 2))
	h ^= (x >> 4) * 0x85ebca6b
	h ^= (y >> 7) * 0xc2b2ae35
	return int(h&uint64(glrStackEquivCacheSetCount-1)) << 1
}

func lookupGSSStackEquivCache(scratch *glrMergeScratch, a, b *gssNode) (bool, bool) {
	if scratch == nil || len(scratch.stackEquivCache) == 0 || scratch.equivEpoch == 0 {
		return false, false
	}
	ap, bp, ok := orderedGSSNodePair(a, b)
	if !ok {
		return false, false
	}
	idx := stackEquivCacheIndex(ap, bp)
	primary := &scratch.stackEquivCache[idx]
	if primary.epoch == scratch.equivEpoch && primary.a == ap && primary.b == bp {
		return primary.result, true
	}
	victim := &scratch.stackEquivCache[idx+1]
	if victim.epoch == scratch.equivEpoch && victim.a == ap && victim.b == bp {
		scratch.stackEquivCache[idx], scratch.stackEquivCache[idx+1] = scratch.stackEquivCache[idx+1], scratch.stackEquivCache[idx]
		return scratch.stackEquivCache[idx].result, true
	}
	return false, false
}

func storeGSSStackEquivCache(scratch *glrMergeScratch, a, b *gssNode, result bool) {
	if scratch == nil || scratch.equivEpoch == 0 {
		return
	}
	ap, bp, ok := orderedGSSNodePair(a, b)
	if !ok {
		return
	}
	if len(scratch.stackEquivCache) == 0 {
		scratch.stackEquivCache = make([]glrStackEquivCacheEntry, glrStackEquivCacheSize)
		scratch.stackEquivBytes = glrStackEquivCacheBytesForCap(cap(scratch.stackEquivCache))
	}
	idx := stackEquivCacheIndex(ap, bp)
	scratch.stackEquivCache[idx+1] = scratch.stackEquivCache[idx]
	scratch.stackEquivCache[idx] = glrStackEquivCacheEntry{
		a:      ap,
		b:      bp,
		epoch:  scratch.equivEpoch,
		result: result,
	}
}

// debugMergeEquiv, when set (GOT_DEBUG_MERGE_EQUIV=1), makes gssStacksEqual
// recompute every answer with the spine memo bypassed and loudly report any
// divergence — the house env-gated correctness assertion pattern (mirrors
// GOT_DEBUG_RECOVERY_INCREMENTAL_COST / GOT_DEBUG_RECOVERY_CYCLES). Zero cost
// when unset.
var debugMergeEquiv = os.Getenv("GOT_DEBUG_MERGE_EQUIV") == "1"

// mergeEquivMemoEnabled gates the spine-equality memo. Default on; set
// GOT_DISABLE_MERGE_EQUIV_MEMO=1 to fall back to the original memo-free walk
// (A/B measurement only — the memo is answer-exact, verified by
// GOT_DEBUG_MERGE_EQUIV).
var mergeEquivMemoEnabled = os.Getenv("GOT_DISABLE_MERGE_EQUIV_MEMO") != "1"

var debugMergeEquivReportsLt = 20

func spineEquivCacheIndex(ap, bp uintptr) int {
	x := uint64(ap)
	y := uint64(bp)
	h := x ^ (y + 0x9e3779b97f4a7c15 + (x << 6) + (x >> 2))
	h ^= (x >> 5) * 0x85ebca6b
	h ^= (y >> 3) * 0xc2b2ae35
	return int(h&uint64(glrSpineEquivCacheSetCount-1)) << 1
}

// lookupSpineEquivCache returns the memoized "spine-from-(a) equals spine-from-
// (b)" answer when both nodes' prev-chain prefix fingerprints are unchanged
// since the entry was stored. A hit is exactly the full-walk answer (the
// fingerprint captures every field the walk compares), so it cannot change a
// merge decision.
func lookupSpineEquivCache(scratch *glrMergeScratch, a, b *gssNode) (bool, bool) {
	if scratch == nil || len(scratch.spineEquivCache) == 0 || scratch.equivEpoch == 0 {
		return false, false
	}
	if a == nil || b == nil || a.hash == 0 || b.hash == 0 {
		return false, false
	}
	ap, bp, ok := orderedGSSNodePair(a, b)
	if !ok {
		return false, false
	}
	aHash, bHash := a.hash, b.hash
	if ap != uintptr(unsafe.Pointer(a)) {
		// orderedGSSNodePair swapped: keep fingerprints paired with pointers.
		aHash, bHash = b.hash, a.hash
	}
	idx := spineEquivCacheIndex(ap, bp)
	primary := &scratch.spineEquivCache[idx]
	if primary.epoch == scratch.equivEpoch && primary.a == ap && primary.b == bp &&
		primary.aHash == aHash && primary.bHash == bHash {
		return primary.result, true
	}
	victim := &scratch.spineEquivCache[idx+1]
	if victim.epoch == scratch.equivEpoch && victim.a == ap && victim.b == bp &&
		victim.aHash == aHash && victim.bHash == bHash {
		*primary, *victim = *victim, *primary
		return primary.result, true
	}
	return false, false
}

// storeSpineEquivCache never allocates the backing array (see
// storeShapePrefixCache); one-shot scratches without provisioned caches simply
// skip memoization.
func storeSpineEquivCache(scratch *glrMergeScratch, a, b *gssNode, result bool) {
	if scratch == nil || scratch.equivEpoch == 0 || len(scratch.spineEquivCache) == 0 {
		return
	}
	if a == nil || b == nil || a.hash == 0 || b.hash == 0 {
		return
	}
	ap, bp, ok := orderedGSSNodePair(a, b)
	if !ok {
		return
	}
	aHash, bHash := a.hash, b.hash
	if ap != uintptr(unsafe.Pointer(a)) {
		aHash, bHash = b.hash, a.hash
	}
	idx := spineEquivCacheIndex(ap, bp)
	scratch.spineEquivCache[idx+1] = scratch.spineEquivCache[idx]
	scratch.spineEquivCache[idx] = glrSpineEquivCacheEntry{
		a:      ap,
		b:      bp,
		aHash:  aHash,
		bHash:  bHash,
		epoch:  scratch.equivEpoch,
		result: result,
	}
}

func (s *glrMergeScratch) beginCleanZeroEpoch() {
	if s == nil {
		return
	}
	if s.cleanZeroEpoch == ^uint32(0) {
		clear(s.cleanZeroCache)
		clear(s.cleanZeroFront)
		s.cleanZeroEpoch = 0
	}
	s.cleanZeroEpoch++
}

// ensureMergeHotCaches provisions the fixed-size merge-attempt caches. Called
// only for persistent (pooled, per-parse) scratches so their cost amortizes
// across the whole parse; one-shot local scratches never allocate these.
func (s *glrMergeScratch) ensureMergeHotCaches() {
	if s == nil {
		return
	}
	if len(s.shapePrefixCache) == 0 {
		s.shapePrefixCache = make([]glrShapePrefixCacheEntry, glrShapePrefixCacheSize)
		s.shapePrefixBytes = int64(cap(s.shapePrefixCache)) * int64(unsafe.Sizeof(glrShapePrefixCacheEntry{}))
	}
	if len(s.cleanZeroFront) == 0 {
		s.cleanZeroFront = make([]glrCleanZeroFrontCacheEntry, glrCleanZeroFrontCacheSize)
		s.cleanZeroBytes = int64(cap(s.cleanZeroFront)) * int64(unsafe.Sizeof(glrCleanZeroFrontCacheEntry{}))
	}
	if len(s.spineEquivCache) == 0 {
		s.spineEquivCache = make([]glrSpineEquivCacheEntry, glrSpineEquivCacheSize)
		s.spineEquivBytes = glrSpineEquivCacheBytesForCap(cap(s.spineEquivCache))
	}
}

// bumpShapePrefixEpoch invalidates every cached materializing-shape prefix in
// O(1). Called when a GSS main merge rewrites links (stale prefixes) and at
// the start of each parse epoch.
func (s *glrMergeScratch) bumpShapePrefixEpoch() {
	if s == nil {
		return
	}
	if s.shapePrefixEpoch == ^uint32(0) {
		clear(s.shapePrefixCache)
		s.shapePrefixEpoch = 0
	}
	s.shapePrefixEpoch++
}

func lookupShapePrefixCache(scratch *glrMergeScratch, n *gssNode) (glrMaterializingShapeHash, bool) {
	if scratch == nil || len(scratch.shapePrefixCache) == 0 || scratch.shapePrefixEpoch == 0 || n == nil {
		return glrMaterializingShapeHash{}, false
	}
	p := uintptr(unsafe.Pointer(n))
	idx := shapePrefixCacheIndex(p)
	primary := &scratch.shapePrefixCache[idx]
	if primary.epoch == scratch.shapePrefixEpoch && primary.node == p {
		return primary.prefix, true
	}
	victim := &scratch.shapePrefixCache[idx+1]
	if victim.epoch == scratch.shapePrefixEpoch && victim.node == p {
		*primary, *victim = *victim, *primary
		return primary.prefix, true
	}
	return glrMaterializingShapeHash{}, false
}

// storeShapePrefixCache never allocates: the backing array is provisioned only
// for persistent parse scratches (ensureMergeHotCaches). One-shot local
// scratches (mergeStacks, diagnostics) skip caching rather than paying a
// 384KiB zeroed allocation per call — that allocation pattern showed up as a
// GC/memclr storm on the C# per-member recovery path.
func storeShapePrefixCache(scratch *glrMergeScratch, n *gssNode, prefix glrMaterializingShapeHash) {
	if scratch == nil || scratch.shapePrefixEpoch == 0 || n == nil || len(scratch.shapePrefixCache) == 0 {
		return
	}
	p := uintptr(unsafe.Pointer(n))
	idx := shapePrefixCacheIndex(p)
	scratch.shapePrefixCache[idx+1] = scratch.shapePrefixCache[idx]
	scratch.shapePrefixCache[idx] = glrShapePrefixCacheEntry{
		node:   p,
		epoch:  scratch.shapePrefixEpoch,
		prefix: prefix,
	}
}

func shapePrefixCacheIndex(p uintptr) int {
	h := uint64(p)
	h ^= h >> 33
	h *= 0xff51afd7ed558ccd
	h ^= h >> 33
	h *= 0xc4ceb9fe1a85ec53
	h ^= h >> 33
	return int(h&uint64(glrShapePrefixCacheSetCount-1)) << 1
}

func lookupCleanZeroFrontCache(scratch *glrMergeScratch, n *gssNode) (bool, bool) {
	if scratch == nil || len(scratch.cleanZeroFront) == 0 || scratch.cleanZeroEpoch == 0 || n == nil {
		return false, false
	}
	p := uintptr(unsafe.Pointer(n))
	idx := cleanZeroFrontCacheIndex(p)
	primary := &scratch.cleanZeroFront[idx]
	if primary.epoch == scratch.cleanZeroEpoch && primary.node == p {
		return primary.clean, true
	}
	victim := &scratch.cleanZeroFront[idx+1]
	if victim.epoch == scratch.cleanZeroEpoch && victim.node == p {
		*primary, *victim = *victim, *primary
		return primary.clean, true
	}
	return false, false
}

// storeCleanZeroFrontCache never allocates (see storeShapePrefixCache).
func storeCleanZeroFrontCache(scratch *glrMergeScratch, n *gssNode, clean bool) {
	if scratch == nil || scratch.cleanZeroEpoch == 0 || n == nil || len(scratch.cleanZeroFront) == 0 {
		return
	}
	p := uintptr(unsafe.Pointer(n))
	idx := cleanZeroFrontCacheIndex(p)
	scratch.cleanZeroFront[idx+1] = scratch.cleanZeroFront[idx]
	scratch.cleanZeroFront[idx] = glrCleanZeroFrontCacheEntry{
		node:  p,
		epoch: scratch.cleanZeroEpoch,
		clean: clean,
	}
}

func cleanZeroFrontCacheIndex(p uintptr) int {
	h := uint64(p)
	h ^= h >> 33
	h *= 0xff51afd7ed558ccd
	h ^= h >> 33
	h *= 0xc4ceb9fe1a85ec53
	h ^= h >> 33
	return int(h&uint64(glrCleanZeroFrontCacheSetCount-1)) << 1
}

func lookupNodeEquivCache(scratch *glrMergeScratch, a, b *Node, depth int) (bool, bool) {
	if scratch == nil || len(scratch.equivCache) == 0 || scratch.equivEpoch == 0 {
		return false, false
	}
	if depth < 0 || depth > glrNodeEquivCacheMaxDepth {
		return false, false
	}
	depthKey := uint16(depth)
	ap := uintptr(unsafe.Pointer(a))
	bp := uintptr(unsafe.Pointer(b))
	if ap > bp {
		a, b = b, a
		ap, bp = bp, ap
	}
	primaryIdx := nodeEquivCacheIndex(a, b, depth)
	primary := &scratch.equivCache[primaryIdx]
	var audit *runtimeAudit
	if runtimeEquivAuditEnabled {
		if audit = scratch.audit; audit != nil {
			audit.recordEquivCacheLookup()
		}
	}
	// Try primary slot first.
	if primary.epoch == scratch.equivEpoch &&
		primary.a == ap && primary.b == bp && primary.depth == depthKey &&
		primary.aVersion == a.equivVersion && primary.bVersion == b.equivVersion {
		if audit != nil {
			audit.recordEquivCacheHit()
			audit.recordEquivCacheResultHit(primary.result)
		}
		return primary.result, true
	}
	// Primary missed — try victim slot (immediately following primary in the set).
	victim := &scratch.equivCache[primaryIdx+1]
	if victim.epoch == scratch.equivEpoch &&
		victim.a == ap && victim.b == bp && victim.depth == depthKey &&
		victim.aVersion == a.equivVersion && victim.bVersion == b.equivVersion {
		// Promote victim to primary so the freshest hit is always in slot 0.
		// The displaced primary moves to the victim slot to act as the next
		// fallback. This is a 32-byte swap, cheaper than re-computing the deep
		// equivalence walk on the alternative.
		*primary, *victim = *victim, *primary
		if audit != nil {
			audit.recordEquivCacheHit()
			audit.recordEquivCacheResultHit(primary.result)
		}
		return primary.result, true
	}
	// Real miss — record which kind for diagnostic attribution.
	if audit != nil {
		if primary.epoch != scratch.equivEpoch {
			audit.recordEquivCacheEpochMiss()
		} else if primary.a != ap || primary.b != bp || primary.depth != depthKey {
			audit.recordEquivCacheKeyMiss()
		} else {
			audit.recordEquivCacheVersionMiss()
		}
	}
	return false, false
}

func lookupNodeEquivCacheNoAudit(scratch *glrMergeScratch, a, b *Node, depth int) (bool, bool) {
	if scratch == nil || len(scratch.equivCache) == 0 || scratch.equivEpoch == 0 {
		return false, false
	}
	if depth < 0 || depth > glrNodeEquivCacheMaxDepth {
		return false, false
	}
	depthKey := uint16(depth)
	ap := uintptr(unsafe.Pointer(a))
	bp := uintptr(unsafe.Pointer(b))
	if ap > bp {
		a, b = b, a
		ap, bp = bp, ap
	}
	primaryIdx := nodeEquivCacheIndex(a, b, depth)
	primary := &scratch.equivCache[primaryIdx]
	if primary.epoch == scratch.equivEpoch &&
		primary.a == ap && primary.b == bp && primary.depth == depthKey &&
		primary.aVersion == a.equivVersion && primary.bVersion == b.equivVersion {
		return primary.result, true
	}
	victim := &scratch.equivCache[primaryIdx+1]
	if victim.epoch == scratch.equivEpoch &&
		victim.a == ap && victim.b == bp && victim.depth == depthKey &&
		victim.aVersion == a.equivVersion && victim.bVersion == b.equivVersion {
		*primary, *victim = *victim, *primary
		return primary.result, true
	}
	return false, false
}

func storeNodeEquivCache(scratch *glrMergeScratch, a, b *Node, depth int, result bool) {
	if scratch == nil || len(scratch.equivCache) == 0 || scratch.equivEpoch == 0 || a == nil || b == nil {
		return
	}
	if depth < 0 || depth > glrNodeEquivCacheMaxDepth {
		return
	}
	if runtimeEquivAuditEnabled {
		if audit := scratch.audit; audit != nil {
			audit.recordEquivCacheStore()
		}
	}
	depthKey := uint16(depth)
	ap := uintptr(unsafe.Pointer(a))
	bp := uintptr(unsafe.Pointer(b))
	if ap > bp {
		a, b = b, a
		ap, bp = bp, ap
	}
	primaryIdx := nodeEquivCacheIndex(a, b, depth)
	// 2-way set associative: evict the current primary to the victim slot,
	// then write the new entry into primary. Stale entries in the victim
	// (different epoch) are harmless — they fail epoch check on lookup.
	scratch.equivCache[primaryIdx+1] = scratch.equivCache[primaryIdx]
	scratch.equivCache[primaryIdx] = glrNodeEquivCacheEntry{
		a:        ap,
		b:        bp,
		aVersion: a.equivVersion,
		bVersion: b.equivVersion,
		epoch:    scratch.equivEpoch,
		depth:    depthKey,
		result:   result,
	}
}

func storeNodeEquivCacheNoAudit(scratch *glrMergeScratch, a, b *Node, depth int, result bool) {
	if scratch == nil || len(scratch.equivCache) == 0 || scratch.equivEpoch == 0 || a == nil || b == nil {
		return
	}
	if depth < 0 || depth > glrNodeEquivCacheMaxDepth {
		return
	}
	depthKey := uint16(depth)
	ap := uintptr(unsafe.Pointer(a))
	bp := uintptr(unsafe.Pointer(b))
	if ap > bp {
		a, b = b, a
		ap, bp = bp, ap
	}
	primaryIdx := nodeEquivCacheIndex(a, b, depth)
	scratch.equivCache[primaryIdx+1] = scratch.equivCache[primaryIdx]
	scratch.equivCache[primaryIdx] = glrNodeEquivCacheEntry{
		a:        ap,
		b:        bp,
		aVersion: a.equivVersion,
		bVersion: b.equivVersion,
		epoch:    scratch.equivEpoch,
		depth:    depthKey,
		result:   result,
	}
}

func lookupExactNodeEquivCache(scratch *glrMergeScratch, a, b *Node) (bool, bool) {
	return lookupNodeEquivCache(scratch, a, b, glrNodeEquivCacheExactDepth)
}

func lookupExactNodeEquivCacheNoAudit(scratch *glrMergeScratch, a, b *Node) (bool, bool) {
	return lookupNodeEquivCacheNoAudit(scratch, a, b, glrNodeEquivCacheExactDepth)
}

func storeExactNodeEquivCache(scratch *glrMergeScratch, a, b *Node, result bool) {
	storeNodeEquivCache(scratch, a, b, glrNodeEquivCacheExactDepth, result)
}

func storeExactNodeEquivCacheNoAudit(scratch *glrMergeScratch, a, b *Node, result bool) {
	storeNodeEquivCacheNoAudit(scratch, a, b, glrNodeEquivCacheExactDepth, result)
}

func activeEquivAudit(scratch *glrMergeScratch) *runtimeAudit {
	if !runtimeEquivAuditEnabled || scratch == nil {
		return nil
	}
	return scratch.audit
}

func stackEquivalentForMergeState(scratch *glrMergeScratch, lang *Language, state StateID, a, b glrStack) bool {
	if cRecoveryMergeCostsDiffer(scratch, &a, &b) {
		return false
	}
	audit := activeEquivAudit(scratch)
	if audit != nil {
		audit.setEquivState(state)
		defer audit.clearEquivState()
	}
	return stackEquivalentForLanguageWithScratch(scratch, lang, a, b)
}

// nodeEquivCacheIndex returns the primary slot index for the 2-way set-
// associative cache. The victim slot is at primary+1 (set base = primary &
// ~1). Hash widens uses both pointers, both symbols, and depth to maximize
// distribution across the 8K sets.
func nodeEquivCacheIndex(a, b *Node, depth int) int {
	x := uint64(uintptr(unsafe.Pointer(a)))
	y := uint64(uintptr(unsafe.Pointer(b)))
	h := x ^ (y + 0x9e3779b97f4a7c15 + (x << 6) + (x >> 2))
	// Mix in symbol to improve distribution for arena-sequential pointers.
	h ^= (uint64(a.symbol) | uint64(b.symbol)<<16) * 0x85ebca6b
	h ^= uint64(depth) * 0x517cc1b727220a95
	// Map to a set index in [0, glrNodeEquivCacheSetCount), then multiply by
	// 2 to land on the primary slot. The victim slot is at primary+1.
	return int(h&uint64(glrNodeEquivCacheSetCount-1)) << 1
}

func stackEntriesEqualForLanguageWithScratch(scratch *glrMergeScratch, lang *Language, a, b []stackEntry) bool {
	if len(a) != len(b) {
		if audit := activeEquivAudit(scratch); audit != nil {
			audit.recordStackEquivDepthMismatch()
		}
		return false
	}
	audit := activeEquivAudit(scratch)
	if audit == nil {
		for i := len(a) - 1; i >= 0; i-- {
			if a[i].state != b[i].state {
				return false
			}
			if !stackEntryPayloadsEquivalentForLanguageWithScratch(scratch, lang, a[i], b[i]) {
				return false
			}
		}
		return true
	}
	for i, depthFromTop := len(a)-1, 0; i >= 0; i, depthFromTop = i-1, depthFromTop+1 {
		audit.recordStackEquivEntryCompare()
		if a[i].state != b[i].state {
			audit.recordStackEquivStateMismatchAt(depthFromTop)
			return false
		}
		if !stackEntryPayloadsEquivalentForLanguageWithScratch(scratch, lang, a[i], b[i]) {
			audit.recordStackEquivPayloadMismatchAt(depthFromTop)
			audit.recordStackEquivPayloadMismatchSignatures(a[i], b[i])
			return false
		}
	}
	return true
}

func gssStacksEqual(a, b gssStack) bool {
	return gssStacksEqualForLanguage(nil, a, b)
}

func gssStacksEqualForLanguage(lang *Language, a, b gssStack) bool {
	return gssStacksEqualForLanguageWithScratch(nil, lang, a, b)
}

type spinePairKey struct {
	a *gssNode
	b *gssNode
}

// gssSpineEqualMemo walks the two spines top-down, consulting the spine-equality
// memo at every position. On the first memoized pair the whole remaining spine
// answer is known, so the walk stops. It is the iterative twin of the original
// gssStacksEqual loop (equal answer for equal inputs) plus back-fill: the walk
// visits pairs (p0,p1,..,pd) before it decides at depth d (convergence,
// state/payload mismatch, or a memo hit); every shallower pair shares that same
// answer because they all carry the identical deciding suffix, so one computed
// verdict is stored for all of them. Iterative (not recursive) to keep the Go
// stack O(1) on deep spines.
func gssSpineEqualMemo(scratch *glrMergeScratch, lang *Language, headA, headB *gssNode) bool {
	if scratch == nil {
		return gssSpineEqualRaw(scratch, lang, headA, headB)
	}
	visited := scratch.spineVisit[:0]
	result := true
	for an, bn := headA, headB; an != nil && bn != nil; an, bn = an.prev, bn.prev {
		if an == bn {
			result = true
			break
		}
		if res, ok := lookupSpineEquivCache(scratch, an, bn); ok {
			storeSpineEquivVisited(scratch, visited, res)
			scratch.spineVisit = visited
			return res
		}
		if an.entry.state != bn.entry.state {
			visited = append(visited, spinePairKey{an, bn})
			result = false
			break
		}
		if !stackEntryPayloadsEquivalentForLanguageWithScratch(scratch, lang, an.entry, bn.entry) {
			visited = append(visited, spinePairKey{an, bn})
			result = false
			break
		}
		visited = append(visited, spinePairKey{an, bn})
	}
	storeSpineEquivVisited(scratch, visited, result)
	scratch.spineVisit = visited
	return result
}

func storeSpineEquivVisited(scratch *glrMergeScratch, visited []spinePairKey, result bool) {
	for i := range visited {
		storeSpineEquivCache(scratch, visited[i].a, visited[i].b, result)
	}
}

// gssSpineEqualRaw reproduces the original memo-free spine walk. Used only by
// the GOT_DEBUG_MERGE_EQUIV assertion to confirm the memo answer is exact.
func gssSpineEqualRaw(scratch *glrMergeScratch, lang *Language, headA, headB *gssNode) bool {
	for an, bn := headA, headB; an != nil && bn != nil; an, bn = an.prev, bn.prev {
		if an == bn {
			return true
		}
		if an.entry.state != bn.entry.state {
			return false
		}
		if !stackEntryPayloadsEquivalentForLanguageWithScratch(scratch, lang, an.entry, bn.entry) {
			return false
		}
	}
	return true
}

func debugCheckSpineEquiv(scratch *glrMergeScratch, lang *Language, headA, headB *gssNode, got bool) {
	want := gssSpineEqualRaw(scratch, lang, headA, headB)
	if want == got {
		return
	}
	if debugMergeEquivReportsLt > 0 {
		debugMergeEquivReportsLt--
		fmt.Fprintf(os.Stderr,
			"MERGE-EQUIV divergence: headA=%p headB=%p memo=%v full=%v\n", headA, headB, got, want)
	}
}

func gssStacksEqualForLanguageWithScratch(scratch *glrMergeScratch, lang *Language, a, b gssStack) bool {
	if a.head == b.head {
		return true
	}
	if a.head == nil || b.head == nil {
		return false
	}
	if a.head.depth != b.head.depth {
		if audit := activeEquivAudit(scratch); audit != nil {
			audit.recordStackEquivDepthMismatch()
		}
		return false
	}
	if gssNodeHash(a.head) != gssNodeHash(b.head) {
		if audit := activeEquivAudit(scratch); audit != nil {
			audit.recordStackEquivHashMismatch()
		}
		return false
	}
	if hit, ok := lookupGSSStackEquivCache(scratch, a.head, b.head); ok {
		return hit
	}
	audit := activeEquivAudit(scratch)
	if audit == nil {
		if !mergeEquivMemoEnabled {
			res := gssSpineEqualRaw(scratch, lang, a.head, b.head)
			storeGSSStackEquivCache(scratch, a.head, b.head, res)
			return res
		}
		res := gssSpineEqualMemo(scratch, lang, a.head, b.head)
		if debugMergeEquiv {
			debugCheckSpineEquiv(scratch, lang, a.head, b.head, res)
		}
		storeGSSStackEquivCache(scratch, a.head, b.head, res)
		return res
	}
	for an, bn, depthFromTop := a.head, b.head, 0; an != nil && bn != nil; an, bn, depthFromTop = an.prev, bn.prev, depthFromTop+1 {
		if an == bn {
			storeGSSStackEquivCache(scratch, a.head, b.head, true)
			return true
		}
		audit.recordStackEquivEntryCompare()
		if an.entry.state != bn.entry.state {
			audit.recordStackEquivStateMismatchAt(depthFromTop)
			storeGSSStackEquivCache(scratch, a.head, b.head, false)
			return false
		}
		if !stackEntryPayloadsEquivalentForLanguageWithScratch(scratch, lang, an.entry, bn.entry) {
			audit.recordStackEquivPayloadMismatchAt(depthFromTop)
			audit.recordStackEquivPayloadMismatchSignatures(an.entry, bn.entry)
			storeGSSStackEquivCache(scratch, a.head, b.head, false)
			return false
		}
	}
	storeGSSStackEquivCache(scratch, a.head, b.head, true)
	return true
}

func stackEquivalent(a, b glrStack) bool {
	return stackEquivalentForLanguage(nil, a, b)
}

func stackEquivalentForLanguage(lang *Language, a, b glrStack) bool {
	return stackEquivalentForLanguageWithScratch(nil, lang, a, b)
}

func stackEquivalentForLanguageWithScratch(scratch *glrMergeScratch, lang *Language, a, b glrStack) bool {
	if perfCountersEnabled {
		perfRecordStackEquivalentCall()
	}
	audit := activeEquivAudit(scratch)
	var pairKey runtimeAuditStackEquivPairKey
	var pairPrevious bool
	var pairHit bool
	pairKeyOK := false
	headerEq := false
	if audit != nil {
		audit.recordStackEquivCall()
		if key, ok := stackEquivPairKeyForAudit(a, b); ok {
			pairKey = key
			pairKeyOK = true
			pairPrevious, pairHit = audit.lookupStackEquivPair(key)
		} else {
			audit.recordStackEquivPairUnkeyed()
		}
		// Compute the header-only equivalence (C tree-sitter's
		// ts_stack_can_merge shape: top state + byte offset). We track
		// whether switching to header-only merge would over-merge — i.e.
		// cases where header-only accepts but deep-frontier rejects.
		headerEq = stacksHeaderEquivalent(a, b)
		if headerEq {
			audit.recordMergeHeaderEq()
		}
	}
	if a.depth() != b.depth() {
		if audit != nil {
			audit.recordStackEquivDepthMismatch()
			finishStackEquivalentForAudit(audit, pairKey, pairKeyOK, pairPrevious, pairHit, false)
			recordMergeHeaderDivergenceForAudit(audit, headerEq, false)
		}
		return false
	}
	if a.gss.head != nil && b.gss.head != nil {
		eq := gssStacksEqualForLanguageWithScratch(scratch, lang, a.gss, b.gss)
		if audit != nil {
			recordMergeHeaderDivergenceForAudit(audit, headerEq, eq)
		}
		return finishStackEquivalentResultForAudit(audit, pairKey, pairKeyOK, pairPrevious, pairHit, eq)
	}
	if a.gss.head != nil {
		eq := gssStackEntriesEqualForLanguageWithScratch(scratch, lang, a.gss, b.entries)
		if audit != nil {
			recordMergeHeaderDivergenceForAudit(audit, headerEq, eq)
		}
		return finishStackEquivalentResultForAudit(audit, pairKey, pairKeyOK, pairPrevious, pairHit, eq)
	}
	if b.gss.head != nil {
		eq := gssStackEntriesEqualForLanguageWithScratch(scratch, lang, b.gss, a.entries)
		if audit != nil {
			recordMergeHeaderDivergenceForAudit(audit, headerEq, eq)
		}
		return finishStackEquivalentResultForAudit(audit, pairKey, pairKeyOK, pairPrevious, pairHit, eq)
	}
	eq := stackEntriesEqualForLanguageWithScratch(scratch, lang, a.entries, b.entries)
	if audit != nil {
		recordMergeHeaderDivergenceForAudit(audit, headerEq, eq)
	}
	return finishStackEquivalentResultForAudit(audit, pairKey, pairKeyOK, pairPrevious, pairHit, eq)
}

// stacksHeaderEquivalent returns true when two stacks would be considered
// mergeable under C tree-sitter's ts_stack_can_merge semantics — i.e. when
// their top parser state and byte offset agree. This is the cheap shallow
// check we'd switch to if the divergence-from-deep-frontier rate is near
// zero across the ring matrix.
//
// External scanner state is intentionally NOT included here because our
// scanner is a parser-singleton (not per-stack), so the comparison would
// be tautologically true. If we ever per-stack the external scanner, this
// helper should grow that field too.
func stacksHeaderEquivalent(a, b glrStack) bool {
	aTop := a.top()
	bTop := b.top()
	if aTop.state != bTop.state {
		return false
	}
	return a.byteOffset == b.byteOffset
}

func cRecoverStackTraceKind(s glrStack) string {
	switch {
	case s.cRec != nil && s.cRec.group != nil:
		return "error-group"
	case s.cRecoverMissingGroup != nil:
		return "missing-group"
	case s.cRec != nil:
		return "error"
	default:
		return "ordinary"
	}
}

func cRecoverTraceInteresting(a, b glrStack) bool {
	return a.cRec != nil || b.cRec != nil || a.cRecoverMissingGroup != nil || b.cRecoverMissingGroup != nil
}

// cStackCleanZeroErrorCostForMerge is an exact fast path for
// cStackErrorCostForMergeWithScratch (parser_recover_c.go). When every subtree
// reachable from the stack head is provably error/missing-free — via the same
// epoch-managed all-links clean-zero cache gssMainCanMergeWithScratch already
// trusts for real merges — the walked per-subtree sum is exactly zero (every
// cost source in cNodeErrorCostLang requires an errorSymbol node, a missing
// node, or a hasError subtree, all of which force the entry's clean-zero check
// false). Only the per-stack aux terms remain; they MUST mirror the tail of
// cStackErrorCostForMergeWithScratch. ok=false means "not provably clean":
// callers fall back to the full walk, so this can never change a cost value.
//
// This matters because on clean parses of large real files the full walk is
// O(spine length) map lookups per call and runs ~100x per token from the merge
// attempt loop, which made the cost competition super-linear in file size
// (python cliff RCA 2026-07).
func cStackCleanZeroErrorCostForMerge(scratch *glrMergeScratch, s *glrStack) (uint32, bool) {
	if s == nil {
		return 0, true
	}
	if scratch == nil || len(s.entries) != 0 || s.gss.head == nil {
		return 0, false
	}
	if !gssNodeCleanZeroErrorAllLinksWithScratch(scratch, s.gss.head) {
		return 0, false
	}
	var cost uint32
	if s.cPaused || (s.cRec != nil && s.cRec.openErr == nil) {
		cost += cErrCostPerRecovery
	}
	if s.cRec != nil && s.cRec.extraRecoveries > 0 {
		cost += cErrCostPerRecovery * uint32(s.cRec.extraRecoveries)
	}
	return cost, true
}

// cStackErrorCostForMergeCached returns cStackErrorCostForMergeWithScratch's
// value, preferring the exact clean-zero fast path.
func cStackErrorCostForMergeCached(scratch *glrMergeScratch, lang *Language, s *glrStack) uint32 {
	if cost, ok := cStackCleanZeroErrorCostForMerge(scratch, s); ok {
		return cost
	}
	return cStackErrorCostForMergeWithScratch(scratch, lang, s)
}

func cRecoveryMergeCostsDiffer(scratch *glrMergeScratch, a, b *glrStack) bool {
	if scratch == nil || !scratch.cRecoveryCost || a == nil || b == nil {
		return false
	}
	if !stacksHeaderEquivalent(*a, *b) {
		return false
	}
	return cStackErrorCostForMergeCached(scratch, scratch.language, a) != cStackErrorCostForMergeCached(scratch, scratch.language, b)
}

func cRecoveryMergeCostsDifferForParser(p *Parser, a, b *glrStack) bool {
	if p == nil || !p.errorCostCompetitionEnabled() {
		return false
	}
	// Sticky per-parse gate: before anything cost-relevant has happened this
	// pass every stack's error cost is provably zero, so the costs cannot
	// differ (see crecoveryCostCompetitionRelevant). The pair-local recovery
	// state check keeps this function sound for callers that construct
	// paused/recovering stacks directly (unit tests, future call sites)
	// without having gone through the dispatch flip sites.
	if !p.crecoveryCostCompetitionRelevant &&
		!(a != nil && (a.cPaused || a.cRec != nil)) &&
		!(b != nil && (b.cPaused || b.cRec != nil)) {
		return false
	}
	scratch := glrMergeScratch{
		language:      p.language,
		trace:         p.glrTrace,
		cRecoveryCost: true,
	}
	return cRecoveryMergeCostsDiffer(&scratch, a, b)
}

func traceCRecoverMergeDecision(scratch *glrMergeScratch, phase, decision string, incumbent, candidate glrStack) {
	if scratch == nil || !scratch.trace || !cRecoverTraceInteresting(incumbent, candidate) {
		return
	}
	fmt.Printf("      -> C-RECOVER-MERGE phase=%s decision=%s key=(state:%d byte:%d) inc={%s depth:%d score:%d shifted:%v} cand={%s depth:%d score:%d shifted:%v}\n",
		phase,
		decision,
		candidate.top().state,
		candidate.byteOffset,
		cRecoverStackTraceKind(incumbent),
		incumbent.depth(),
		incumbent.score,
		incumbent.shifted,
		cRecoverStackTraceKind(candidate),
		candidate.depth(),
		candidate.score,
		candidate.shifted,
	)
}

// recordMergeHeaderDivergenceForAudit tallies the relationship between
// header-only equivalence and deep equivalence for a single merge-candidate
// pair. The interesting bucket is "header-only would accept, deep walk
// rejects" (mergeHeaderDeepDivergent) — that's how many merges would change
// behavior if we switched to header-only.
func recordMergeHeaderDivergenceForAudit(audit *runtimeAudit, headerEq, deepEq bool) {
	if audit == nil {
		return
	}
	audit.recordMergeDeepResult(headerEq, deepEq)
}

func stackEquivPairKeyForAudit(a, b glrStack) (runtimeAuditStackEquivPairKey, bool) {
	if a.gss.head == nil || b.gss.head == nil {
		return runtimeAuditStackEquivPairKey{}, false
	}
	ap := uintptr(unsafe.Pointer(a.gss.head))
	bp := uintptr(unsafe.Pointer(b.gss.head))
	if ap == 0 || bp == 0 {
		return runtimeAuditStackEquivPairKey{}, false
	}
	if ap > bp {
		ap, bp = bp, ap
	}
	depth := a.gss.head.depth
	if b.gss.head.depth > depth {
		depth = b.gss.head.depth
	}
	return runtimeAuditStackEquivPairKey{
		a:     ap,
		b:     bp,
		depth: uint32(depth),
	}, true
}

func finishStackEquivalentResultForAudit(audit *runtimeAudit, pairKey runtimeAuditStackEquivPairKey, pairKeyOK bool, pairPrevious bool, pairHit bool, result bool) bool {
	if result && perfCountersEnabled {
		perfRecordStackEquivalentTrue()
	}
	if audit != nil {
		if result {
			audit.recordStackEquivTrue()
		}
		finishStackEquivalentForAudit(audit, pairKey, pairKeyOK, pairPrevious, pairHit, result)
	}
	return result
}

func finishStackEquivalentForAudit(audit *runtimeAudit, pairKey runtimeAuditStackEquivPairKey, pairKeyOK bool, pairPrevious bool, pairHit bool, result bool) {
	if audit == nil || !pairKeyOK {
		return
	}
	audit.storeStackEquivPair(pairKey, pairPrevious, pairHit, result)
}

func gssStackEntriesEqualForLanguageWithScratch(scratch *glrMergeScratch, lang *Language, gss gssStack, entries []stackEntry) bool {
	if gss.head == nil {
		return len(entries) == 0
	}
	if len(entries) != gss.len() {
		if audit := activeEquivAudit(scratch); audit != nil {
			audit.recordStackEquivDepthMismatch()
		}
		return false
	}
	audit := activeEquivAudit(scratch)
	i := len(entries) - 1
	if audit == nil {
		for n := gss.head; n != nil; n = n.prev {
			if i < 0 {
				return false
			}
			e := entries[i]
			if n.entry.state != e.state {
				return false
			}
			if !stackEntryPayloadsEquivalentForLanguageWithScratch(scratch, lang, n.entry, e) {
				return false
			}
			i--
		}
		return i == -1
	}
	for n, depthFromTop := gss.head, 0; n != nil; n, depthFromTop = n.prev, depthFromTop+1 {
		if i < 0 {
			return false
		}
		e := entries[i]
		audit.recordStackEquivEntryCompare()
		if n.entry.state != e.state {
			audit.recordStackEquivStateMismatchAt(depthFromTop)
			return false
		}
		if !stackEntryPayloadsEquivalentForLanguageWithScratch(scratch, lang, n.entry, e) {
			audit.recordStackEquivPayloadMismatchAt(depthFromTop)
			audit.recordStackEquivPayloadMismatchSignatures(n.entry, e)
			return false
		}
		i--
	}
	return i == -1
}

const (
	stackEquivalentFrontierDepthLimit        = 8
	stackEquivalentGenericFrontierDepthLimit = 4
	nodeStackEquivFlagMask                   = nodeFlagNamed | nodeFlagExtra | nodeFlagMissing | nodeFlagHasError
	nodeStackEquivNoMissingFlagMask          = nodeFlagNamed | nodeFlagExtra | nodeFlagHasError
)

func stackEntryPayloadsEquivalentForLanguageWithScratch(scratch *glrMergeScratch, lang *Language, a, b stackEntry) bool {
	an := stackEntryNode(a)
	bn := stackEntryNode(b)
	if an != nil && bn != nil {
		return stackEntryNodesEquivalentForLanguageWithScratch(scratch, lang, an, bn)
	}
	if !stackEntryHasNode(a) || !stackEntryHasNode(b) {
		return !stackEntryHasNode(a) && !stackEntryHasNode(b)
	}
	if stackEntryNodeSymbol(a) != stackEntryNodeSymbol(b) ||
		stackEntryNodeStartByte(a) != stackEntryNodeStartByte(b) ||
		stackEntryNodeEndByte(a) != stackEntryNodeEndByte(b) ||
		stackEntryNodeChildCount(a) != stackEntryNodeChildCount(b) ||
		stackEntryNodeFieldIDCount(a) != stackEntryNodeFieldIDCount(b) ||
		stackEntryNodeIsExtra(a) != stackEntryNodeIsExtra(b) ||
		stackEntryNodeIsNamed(a) != stackEntryNodeIsNamed(b) ||
		stackEntryNodeIsMissing(a) != stackEntryNodeIsMissing(b) ||
		stackEntryNodeHasError(a) != stackEntryNodeHasError(b) ||
		stackEntryNodeParseState(a) != stackEntryNodeParseState(b) ||
		stackEntryNodePreGotoState(a) != stackEntryNodePreGotoState(b) ||
		stackEntryNodeProductionID(a) != stackEntryNodeProductionID(b) ||
		stackEntryDynamicPrecedence(a) != stackEntryDynamicPrecedence(b) {
		return false
	}
	return true
}

func stackEntryExactHeaderSignature(e stackEntry) uint64 {
	h := gssHashSeed
	h = mixStackEquivSignature(h, uint64(e.kind))
	h = mixStackEquivSignature(h, uint64(e.state))
	if !stackEntryHasNode(e) {
		return mixStackEquivSignature(h, gssNilNodeSentinel)
	}
	h = mixStackEquivSignature(h, uint64(stackEntryNodeSymbol(e)))
	h = mixStackEquivSignature(h, (uint64(stackEntryNodeStartByte(e))<<32)|uint64(stackEntryNodeEndByte(e)))
	h = mixStackEquivSignature(h, uint64(stackEntryNodeChildCount(e)))
	h = mixStackEquivSignature(h, uint64(stackEntryNodeFieldIDCount(e)))
	h = mixStackEquivSignature(h, uint64(stackEntryNodeParseState(e)))
	h = mixStackEquivSignature(h, uint64(stackEntryNodePreGotoState(e)))
	h = mixStackEquivSignature(h, uint64(stackEntryNodeProductionID(e)))
	h = mixStackEquivSignature(h, uint64(uint32(stackEntryDynamicPrecedence(e))))
	h = mixStackEquivSignature(h, uint64(stackEntryNodeExactFlagBits(e)))
	return h
}

func stackEntryExactShallowSignature(e stackEntry) uint64 {
	h := stackEntryExactHeaderSignature(e)
	n := stackEntryNode(e)
	if n == nil {
		return h
	}
	h = mixStackEquivSignature(h, uint64(len(n.fieldIDs)))
	for i := range n.fieldIDs {
		h = mixStackEquivSignature(h, uint64(n.fieldIDs[i]))
	}
	h = mixStackEquivSignature(h, uint64(len(n.children)))
	for i := range n.children {
		h = mixStackEquivSignature(h, uint64(i))
		h = mixStackEquivSignature(h, stackNodeExactHeaderSignature(n.children[i]))
	}
	return h
}

func stackNodeExactHeaderSignature(n *Node) uint64 {
	h := gssHashSeed
	if n == nil {
		return mixStackEquivSignature(h, gssNilNodeSentinel)
	}
	h = mixStackEquivSignature(h, uint64(n.symbol))
	h = mixStackEquivSignature(h, (uint64(n.startByte)<<32)|uint64(n.endByte))
	h = mixStackEquivSignature(h, uint64(len(n.children)))
	h = mixStackEquivSignature(h, uint64(len(n.fieldIDs)))
	h = mixStackEquivSignature(h, uint64(n.parseState))
	h = mixStackEquivSignature(h, uint64(n.preGotoState))
	h = mixStackEquivSignature(h, uint64(n.productionID))
	h = mixStackEquivSignature(h, uint64(uint32(n.dynamicPrecedence)))
	h = mixStackEquivSignature(h, uint64(n.flags&nodeStackEquivFlagMask))
	return h
}

func stackEntryNodeExactFlagBits(e stackEntry) nodeFlags {
	var flags nodeFlags
	if stackEntryNodeIsExtra(e) {
		flags |= nodeFlagExtra
	}
	if stackEntryNodeIsNamed(e) {
		flags |= nodeFlagNamed
	}
	if stackEntryNodeIsMissing(e) {
		flags |= nodeFlagMissing
	}
	if stackEntryNodeHasError(e) {
		flags |= nodeFlagHasError
	}
	return flags
}

func mixStackEquivSignature(h, v uint64) uint64 {
	h ^= v + 0x9e3779b97f4a7c15 + (h << 6) + (h >> 2)
	h *= gssHashPrime
	return h
}

func stackEntryNodesEquivalent(a, b *Node) bool {
	if a == b {
		return true
	}
	if a == nil || b == nil {
		return false
	}
	if a.symbol != b.symbol {
		return false
	}
	if a.startByte != b.startByte ||
		a.endByte != b.endByte ||
		((a.flags^b.flags)&nodeStackEquivFlagMask) != 0 ||
		a.parseState != b.parseState ||
		a.productionID != b.productionID ||
		a.dynamicPrecedence != b.dynamicPrecedence ||
		len(a.children) != len(b.children) {
		return false
	}
	if a.flags&nodeFlagHasError != 0 {
		return true
	}
	if stackNodeNeedsDeepEquivalent(a) || stackNodeNeedsDeepEquivalent(b) {
		return stackEntryNodesEquivalentFrontierWithScratch(nil, a, b, stackEquivalentGenericFrontierDepthLimit)
	}
	for i := range a.children {
		ca := a.children[i]
		cb := b.children[i]
		if ca == cb {
			continue
		}
		if ca == nil || cb == nil {
			return false
		}
		if ca.symbol != cb.symbol ||
			ca.startByte != cb.startByte ||
			ca.endByte != cb.endByte ||
			((ca.flags^cb.flags)&nodeStackEquivNoMissingFlagMask) != 0 ||
			ca.dynamicPrecedence != cb.dynamicPrecedence ||
			len(ca.children) != len(cb.children) {
			return false
		}
	}
	return true
}

func stackNodeNeedsDeepEquivalent(n *Node) bool {
	if n == nil {
		return false
	}
	if n.flags&nodeFlagExtra != 0 || n.preGotoState != 0 || len(n.fieldIDs) != 0 {
		return true
	}
	for i := range n.children {
		child := n.children[i]
		if child == nil {
			continue
		}
		if child.flags&nodeFlagExtra != 0 || child.preGotoState != 0 || len(child.fieldIDs) != 0 || len(child.children) > 0 {
			return true
		}
	}
	return false
}

func stackEntryNodesEquivalentForLanguageWithScratch(scratch *glrMergeScratch, lang *Language, a, b *Node) bool {
	if languageNeedsExactStackNodeEquivalence(lang) {
		if a == b {
			return true
		}
		if a == nil || b == nil {
			return false
		}
		if len(a.children) == 0 || len(b.children) == 0 ||
			a.flags&nodeFlagHasError != 0 || b.flags&nodeFlagHasError != 0 {
			if audit := activeEquivAudit(scratch); audit != nil {
				return stackEntryNodesExactlyEquivalentTerminal(audit, a, b)
			}
			return stackEntryNodesExactlyEquivalentTerminalNoAudit(a, b)
		}
		return stackEntryNodesExactlyEquivalentWithScratch(scratch, a, b, 0)
	}
	if lang != nil && lang.Name == "python" && scratch != nil && scratch.pythonShallow {
		return stackEntryNodesEquivalentPythonShallow(a, b)
	}
	if lang != nil && (lang.Name == "c_sharp" || lang.Name == "bash" || len(lang.AliasSequences) > 0) {
		depthLimit := stackEquivalentFrontierDepthLimit
		if lang.Name == "bash" {
			if depthLimit < 32 {
				depthLimit = 32
			}
		} else if depthLimit < 10 {
			depthLimit = 10
		}
		if !stackEntryNodesEquivalentFrontierWithScratch(scratch, a, b, depthLimit) {
			return false
		}
		if lang.Name == "bash" || lang.Name != "c_sharp" {
			return true
		}
		if a == nil || b == nil {
			return a == b
		}
		if a.Type(lang) == "block" && len(a.children) > 3 {
			compared := 0
			for i := len(a.children) - 1; i >= 0 && compared < 3; i-- {
				child := a.children[i]
				if child == nil || child.flags&nodeFlagExtra != 0 || (child.flags&nodeFlagNamed == 0 && len(child.children) == 0) {
					continue
				}
				if !stackEntryNodesEquivalentFrontierWithScratch(scratch, child, b.children[i], depthLimit-1) {
					return false
				}
				compared++
			}
		}
		if a.Type(lang) == "compilation_unit" && len(a.children) > 2 {
			compared := 0
			for i := len(a.children) - 1; i >= 0 && compared < 2; i-- {
				child := a.children[i]
				if child == nil || child.flags&nodeFlagExtra != 0 || (child.flags&nodeFlagNamed == 0 && len(child.children) == 0) {
					continue
				}
				if !stackEntryNodesEquivalentFrontierWithScratch(scratch, child, b.children[i], depthLimit-1) {
					return false
				}
				compared++
			}
		}
		return true
	}
	return stackEntryNodesEquivalent(a, b)
}

func stackEntryNodesEquivalentPythonShallow(a, b *Node) bool {
	if a == b {
		return true
	}
	if a == nil || b == nil {
		return false
	}
	if a.symbol != b.symbol ||
		a.startByte != b.startByte ||
		a.endByte != b.endByte ||
		len(a.children) != len(b.children) ||
		((a.flags^b.flags)&nodeStackEquivFlagMask) != 0 ||
		a.parseState != b.parseState ||
		a.preGotoState != b.preGotoState ||
		a.productionID != b.productionID ||
		a.dynamicPrecedence != b.dynamicPrecedence {
		return false
	}
	if a.flags&nodeFlagHasError != 0 {
		return true
	}
	if len(a.fieldIDs) != len(b.fieldIDs) {
		return false
	}
	for i := range a.fieldIDs {
		if a.fieldIDs[i] != b.fieldIDs[i] {
			return false
		}
	}
	for i := range a.children {
		ca := a.children[i]
		cb := b.children[i]
		if ca == cb {
			continue
		}
		if ca == nil || cb == nil {
			return false
		}
		if ca.symbol != cb.symbol ||
			ca.startByte != cb.startByte ||
			ca.endByte != cb.endByte ||
			((ca.flags^cb.flags)&nodeStackEquivFlagMask) != 0 ||
			ca.parseState != cb.parseState ||
			ca.preGotoState != cb.preGotoState ||
			ca.productionID != cb.productionID ||
			ca.dynamicPrecedence != cb.dynamicPrecedence ||
			len(ca.children) != len(cb.children) ||
			len(ca.fieldIDs) != len(cb.fieldIDs) {
			return false
		}
		for j := range ca.fieldIDs {
			if ca.fieldIDs[j] != cb.fieldIDs[j] {
				return false
			}
		}
	}
	return true
}

func languageNeedsExactStackNodeEquivalence(lang *Language) bool {
	if lang == nil {
		return false
	}
	switch lang.Name {
	case "typescript", "tsx":
		return true
	default:
		return false
	}
}

func stackEntryNodesExactlyEquivalentWithScratch(scratch *glrMergeScratch, a, b *Node, depth int) bool {
	audit := activeEquivAudit(scratch)
	if audit == nil {
		return stackEntryNodesExactlyEquivalentNoAudit(scratch, a, b, depth)
	}
	return stackEntryNodesExactlyEquivalentWithAudit(scratch, audit, a, b, depth)
}

func stackEntryNodesExactlyEquivalentWithAudit(scratch *glrMergeScratch, audit *runtimeAudit, a, b *Node, depth int) bool {
	if audit != nil {
		audit.recordEquivExactCall()
	}
	if a == b {
		if audit != nil {
			audit.recordEquivExactPointerTrue()
			audit.recordEquivExactTrue()
		}
		return true
	}
	if a == nil || b == nil {
		if audit != nil {
			audit.recordEquivExactNilMismatch()
		}
		return false
	}
	if hit, ok := lookupExactNodeEquivCache(scratch, a, b); ok {
		if hit && audit != nil {
			audit.recordEquivExactTrue()
		}
		return hit
	}
	if a.symbol != b.symbol ||
		a.startByte != b.startByte ||
		a.endByte != b.endByte ||
		len(a.children) != len(b.children) ||
		((a.flags^b.flags)&nodeStackEquivFlagMask) != 0 ||
		a.parseState != b.parseState ||
		a.preGotoState != b.preGotoState ||
		a.productionID != b.productionID ||
		a.dynamicPrecedence != b.dynamicPrecedence {
		if audit != nil {
			audit.recordEquivExactHeaderMismatch()
		}
		return false
	}
	if len(a.fieldIDs) != len(b.fieldIDs) {
		if audit != nil {
			audit.recordEquivSkipFieldMismatch()
		}
		return false
	}
	if a.flags&nodeFlagHasError != 0 {
		if audit != nil {
			audit.recordEquivSkipError()
			audit.recordEquivExactTrue()
		}
		return true
	}
	for i := range a.fieldIDs {
		if a.fieldIDs[i] != b.fieldIDs[i] {
			if audit != nil {
				audit.recordEquivSkipFieldMismatch()
			}
			return false
		}
	}
	if len(a.children) == 0 {
		if audit != nil {
			audit.recordEquivSkipLeaf()
			audit.recordEquivExactTrue()
		}
		return true
	}
	for i := range a.children {
		if audit != nil {
			audit.recordEquivExactChildCompare()
		}
		ca := a.children[i]
		cb := b.children[i]
		if ca == cb {
			continue
		}
		if ca == nil || cb == nil {
			if audit != nil {
				audit.recordEquivExactNilMismatch()
				audit.recordEquivExactChildMismatch()
			}
			storeExactNodeEquivCache(scratch, a, b, false)
			return false
		}
		if len(ca.children) == 0 || len(cb.children) == 0 ||
			ca.flags&nodeFlagHasError != 0 || cb.flags&nodeFlagHasError != 0 {
			if !stackEntryNodesExactlyEquivalentTerminal(audit, ca, cb) {
				if audit != nil {
					audit.recordEquivExactChildMismatch()
				}
				storeExactNodeEquivCache(scratch, a, b, false)
				return false
			}
			continue
		}
		if !stackEntryNodesExactlyEquivalentWithScratch(scratch, ca, cb, depth+1) {
			if audit != nil {
				audit.recordEquivExactChildMismatch()
			}
			storeExactNodeEquivCache(scratch, a, b, false)
			return false
		}
	}
	storeExactNodeEquivCache(scratch, a, b, true)
	if audit != nil {
		audit.recordEquivExactTrue()
	}
	return true
}

func stackEntryNodesExactlyEquivalentNoAudit(scratch *glrMergeScratch, a, b *Node, depth int) bool {
	if a == b {
		return true
	}
	if a == nil || b == nil {
		return false
	}
	if hit, ok := lookupExactNodeEquivCacheNoAudit(scratch, a, b); ok {
		return hit
	}
	if a.symbol != b.symbol ||
		a.startByte != b.startByte ||
		a.endByte != b.endByte ||
		len(a.children) != len(b.children) ||
		((a.flags^b.flags)&nodeStackEquivFlagMask) != 0 ||
		a.parseState != b.parseState ||
		a.preGotoState != b.preGotoState ||
		a.productionID != b.productionID ||
		a.dynamicPrecedence != b.dynamicPrecedence ||
		len(a.fieldIDs) != len(b.fieldIDs) {
		return false
	}
	if a.flags&nodeFlagHasError != 0 {
		return true
	}
	for i := range a.fieldIDs {
		if a.fieldIDs[i] != b.fieldIDs[i] {
			return false
		}
	}
	if len(a.children) == 0 {
		return true
	}
	for i := range a.children {
		ca := a.children[i]
		cb := b.children[i]
		if ca == cb {
			continue
		}
		if ca == nil || cb == nil {
			storeExactNodeEquivCacheNoAudit(scratch, a, b, false)
			return false
		}
		if len(ca.children) == 0 || len(cb.children) == 0 ||
			ca.flags&nodeFlagHasError != 0 || cb.flags&nodeFlagHasError != 0 {
			if !stackEntryNodesExactlyEquivalentTerminalNoAudit(ca, cb) {
				storeExactNodeEquivCacheNoAudit(scratch, a, b, false)
				return false
			}
			continue
		}
		if !stackEntryNodesExactlyEquivalentNoAudit(scratch, ca, cb, depth+1) {
			storeExactNodeEquivCacheNoAudit(scratch, a, b, false)
			return false
		}
	}
	storeExactNodeEquivCacheNoAudit(scratch, a, b, true)
	return true
}

func stackEntryNodesExactlyEquivalentTerminal(audit *runtimeAudit, a, b *Node) bool {
	if audit != nil {
		audit.recordEquivExactTerminalCall()
	}
	if a == b {
		if audit != nil {
			audit.recordEquivExactPointerTrue()
			audit.recordEquivExactTerminalTrue()
		}
		return true
	}
	if a == nil || b == nil {
		if audit != nil {
			audit.recordEquivExactNilMismatch()
			audit.recordEquivExactTerminalFalse()
		}
		return false
	}
	if a.symbol != b.symbol ||
		a.startByte != b.startByte ||
		a.endByte != b.endByte ||
		len(a.children) != len(b.children) ||
		((a.flags^b.flags)&nodeStackEquivFlagMask) != 0 ||
		a.parseState != b.parseState ||
		a.preGotoState != b.preGotoState ||
		a.productionID != b.productionID ||
		a.dynamicPrecedence != b.dynamicPrecedence {
		if audit != nil {
			audit.recordEquivExactHeaderMismatch()
			audit.recordEquivExactTerminalFalse()
		}
		return false
	}
	if len(a.fieldIDs) != len(b.fieldIDs) {
		if audit != nil {
			audit.recordEquivSkipFieldMismatch()
			audit.recordEquivExactTerminalFalse()
		}
		return false
	}
	for i := range a.fieldIDs {
		if a.fieldIDs[i] != b.fieldIDs[i] {
			if audit != nil {
				audit.recordEquivSkipFieldMismatch()
				audit.recordEquivExactTerminalFalse()
			}
			return false
		}
	}
	if a.flags&nodeFlagHasError != 0 {
		if audit != nil {
			audit.recordEquivSkipError()
			audit.recordEquivExactTerminalTrue()
		}
		return true
	}
	if len(a.children) == 0 {
		if audit != nil {
			audit.recordEquivSkipLeaf()
			audit.recordEquivExactTerminalTrue()
		}
		return true
	}
	if audit != nil {
		audit.recordEquivExactTerminalFalse()
	}
	return false
}

func stackEntryNodesExactlyEquivalentTerminalNoAudit(a, b *Node) bool {
	if a == b {
		return true
	}
	if a == nil || b == nil {
		return false
	}
	if a.symbol != b.symbol ||
		a.startByte != b.startByte ||
		a.endByte != b.endByte ||
		len(a.children) != len(b.children) ||
		((a.flags^b.flags)&nodeStackEquivFlagMask) != 0 ||
		a.parseState != b.parseState ||
		a.preGotoState != b.preGotoState ||
		a.productionID != b.productionID ||
		a.dynamicPrecedence != b.dynamicPrecedence ||
		len(a.fieldIDs) != len(b.fieldIDs) {
		return false
	}
	for i := range a.fieldIDs {
		if a.fieldIDs[i] != b.fieldIDs[i] {
			return false
		}
	}
	return a.flags&nodeFlagHasError != 0 || len(a.children) == 0
}

func stackEntryNodesEquivalentFrontierWithScratch(scratch *glrMergeScratch, a, b *Node, depth int) bool {
	audit := activeEquivAudit(scratch)
	if audit != nil {
		audit.recordEquivFrontierCall()
	}
	// Cheap checks first — skip cache for trivial cases.
	if a == b {
		if audit != nil {
			audit.recordEquivFrontierTrue()
		}
		return true
	}
	if a == nil || b == nil {
		return false
	}
	if a.symbol != b.symbol ||
		a.startByte != b.startByte ||
		a.endByte != b.endByte ||
		len(a.children) != len(b.children) ||
		((a.flags^b.flags)&nodeStackEquivFlagMask) != 0 ||
		a.parseState != b.parseState ||
		a.preGotoState != b.preGotoState ||
		a.productionID != b.productionID ||
		a.dynamicPrecedence != b.dynamicPrecedence {
		return false
	}
	// Cache lookup only for recursive children comparison.
	if hit, ok := lookupNodeEquivCache(scratch, a, b, depth); ok {
		if hit && audit != nil {
			audit.recordEquivFrontierTrue()
		}
		return hit
	}
	if a.flags&nodeFlagHasError != 0 {
		storeNodeEquivCache(scratch, a, b, depth, true)
		if audit != nil {
			audit.recordEquivFrontierTrue()
		}
		return true
	}
	if len(a.fieldIDs) != len(b.fieldIDs) {
		storeNodeEquivCache(scratch, a, b, depth, false)
		return false
	}
	for i := range a.fieldIDs {
		if a.fieldIDs[i] != b.fieldIDs[i] {
			storeNodeEquivCache(scratch, a, b, depth, false)
			return false
		}
	}

	frontier := -1
	for i := range a.children {
		if audit != nil {
			audit.recordEquivFrontierChildScan()
		}
		ca := a.children[i]
		cb := b.children[i]
		if ca == cb {
			if ca != nil && ca.flags&nodeFlagExtra == 0 && (ca.flags&nodeFlagNamed != 0 || len(ca.children) > 0) {
				frontier = i
			}
			continue
		}
		if ca == nil || cb == nil {
			storeNodeEquivCache(scratch, a, b, depth, false)
			return false
		}
		if ca.symbol != cb.symbol ||
			ca.startByte != cb.startByte ||
			ca.endByte != cb.endByte ||
			((ca.flags^cb.flags)&nodeStackEquivFlagMask) != 0 ||
			ca.parseState != cb.parseState ||
			ca.preGotoState != cb.preGotoState ||
			ca.productionID != cb.productionID ||
			ca.dynamicPrecedence != cb.dynamicPrecedence ||
			len(ca.children) != len(cb.children) ||
			len(ca.fieldIDs) != len(cb.fieldIDs) {
			storeNodeEquivCache(scratch, a, b, depth, false)
			return false
		}
		for j := range ca.fieldIDs {
			if ca.fieldIDs[j] != cb.fieldIDs[j] {
				storeNodeEquivCache(scratch, a, b, depth, false)
				return false
			}
		}
		if ca.flags&nodeFlagExtra == 0 && (ca.flags&nodeFlagNamed != 0 || len(ca.children) > 0) {
			frontier = i
		}
	}
	if depth == 0 {
		storeNodeEquivCache(scratch, a, b, depth, true)
		if audit != nil {
			audit.recordEquivFrontierTrue()
		}
		return true
	}

	candidates := [8]int{}
	candidateCount := 0
	addCandidate := func(idx int) {
		if idx < 0 {
			return
		}
		for i := 0; i < candidateCount; i++ {
			if candidates[i] == idx {
				return
			}
		}
		if candidateCount < len(candidates) {
			candidates[candidateCount] = idx
			candidateCount++
		}
	}
	if len(a.children) <= 3 {
		for i := range a.children {
			fielded := i < len(a.fieldIDs) && a.fieldIDs[i] != 0
			child := a.children[i]
			if child == nil || child.flags&nodeFlagExtra != 0 {
				continue
			}
			semantic := child.flags&nodeFlagNamed != 0 || len(child.children) > 0
			if fielded || semantic {
				addCandidate(i)
			}
		}
	}
	addCandidate(frontier)
	if candidateCount == 0 {
		storeNodeEquivCache(scratch, a, b, depth, true)
		if audit != nil {
			audit.recordEquivFrontierTrue()
		}
		return true
	}
	for i := 0; i < candidateCount; i++ {
		idx := candidates[i]
		if audit != nil {
			audit.recordEquivFrontierCandidateCompare()
		}
		if !stackEntryNodesEquivalentFrontierWithScratch(scratch, a.children[idx], b.children[idx], depth-1) {
			storeNodeEquivCache(scratch, a, b, depth, false)
			return false
		}
	}
	storeNodeEquivCache(scratch, a, b, depth, true)
	if audit != nil {
		audit.recordEquivFrontierTrue()
	}
	return true
}

func stackComparePtr(a, b *glrStack) int {
	if perfCountersEnabled {
		perfRecordStackCompare()
	}
	if a.dead != b.dead {
		if a.dead {
			return -1
		}
		return 1
	}
	if a.accepted != b.accepted {
		if a.accepted {
			return 1
		}
		return -1
	}
	if a.score != b.score {
		if a.score > b.score {
			return 1
		}
		return -1
	}
	// When re-processing the current token after GLR reductions, unshifted
	// stacks are the only branches that can still make progress on that
	// lookahead. Prefer keeping them before depth/offset tie-breakers.
	if a.shifted != b.shifted {
		if !a.shifted {
			return 1
		}
		return -1
	}
	aDepth := a.depth()
	bDepth := b.depth()
	if aDepth != bDepth {
		if aDepth > bDepth {
			return 1
		}
		return -1
	}
	if a.byteOffset != b.byteOffset {
		if a.byteOffset > b.byteOffset {
			return 1
		}
		return -1
	}
	if a.branchOrder != b.branchOrder {
		if a.branchOrder < b.branchOrder {
			return 1
		}
		return -1
	}
	return 0
}

func stackCompareMerge(a, b *glrStack) int {
	if perfCountersEnabled {
		perfRecordStackCompare()
	}
	// mergeStacksWithScratch prunes dead stacks before comparing.
	if a.accepted != b.accepted {
		if a.accepted {
			return 1
		}
		return -1
	}
	if a.score != b.score {
		if a.score > b.score {
			return 1
		}
		return -1
	}
	// See stackComparePtr: keep current-token work alive before preferring
	// deeper stacks that already shifted the lookahead.
	if a.shifted != b.shifted {
		if !a.shifted {
			return 1
		}
		return -1
	}
	aDepth := a.depth()
	bDepth := b.depth()
	if aDepth != bDepth {
		if aDepth > bDepth {
			return 1
		}
		return -1
	}
	if a.byteOffset != b.byteOffset {
		if a.byteOffset > b.byteOffset {
			return 1
		}
		return -1
	}
	if a.branchOrder != b.branchOrder {
		if a.branchOrder < b.branchOrder {
			return 1
		}
		return -1
	}
	return 0
}

func stackCompareMergeSmallCapOne(a, b *glrStack) int {
	if perfCountersEnabled {
		perfRecordStackCompare()
	}
	// Small merges normally preserve distinct same-key parse paths. When the
	// caller explicitly caps a key to one survivor, prune only on parser-rank
	// signals and avoid branch-order/hash tie-breakers that can discard the
	// still-correct Java branch on large corpus files.
	if a.accepted != b.accepted {
		if a.accepted {
			return 1
		}
		return -1
	}
	if a.score != b.score {
		if a.score > b.score {
			return 1
		}
		return -1
	}
	if a.shifted != b.shifted {
		if !a.shifted {
			return 1
		}
		return -1
	}
	if glrFaithfulCapOneMerge {
		return 0
	}
	aDepth := a.depth()
	bDepth := b.depth()
	if aDepth != bDepth {
		if aDepth > bDepth {
			return 1
		}
		return -1
	}
	return 0
}

func gssMainCanMerge(a, b *glrStack) bool {
	return gssMainCanMergeWithScratch(nil, a, b)
}

func gssMainCanMergeForParser(p *Parser, a, b *glrStack) bool {
	if cRecoveryMergeCostsDifferForParser(p, a, b) {
		if p != nil && p.glrTrace {
			scratch := glrMergeScratch{language: p.language, trace: true, cRecoveryCost: true}
			traceCRecoverMergeDecision(&scratch, "gss-direct", "reject-cost", *a, *b)
		}
		return false
	}
	return gssMainCanMergeWithScratch(nil, a, b)
}

func tryGSSMainMergeForParser(p *Parser, a, b *glrStack) bool {
	if !gssMainCanMergeForParser(p, a, b) {
		return false
	}
	merged := gssMainMerge(a, b)
	if merged {
		// a survives and absorbs b, so OR the sticky wreckage bit: cEverErrored
		// is lineage history, not current shape, and a clean survivor must not
		// shed a merged-in recovered-wreckage lineage's error history (see
		// glrStack.cEverErrored / tryGSSMainMergeResult).
		a.cEverErrored = a.cEverErrored || b.cEverErrored
		if p != nil {
			// Mirror tryGSSMainMergeResult (bumpShapePrefixEpoch above): a successful
			// main merge rewrites link 0 (setGSSMainLink) of surviving nodes during
			// dispatch, so every root->head shape prefix cached in the active merge
			// scratch may now be stale. p.mergeScratch is nil outside a parse and
			// bumpShapePrefixEpoch is nil-safe.
			p.mergeScratch.bumpShapePrefixEpoch()
		}
	}
	return merged
}

func gssMainCanMergeWithScratch(scratch *glrMergeScratch, a, b *glrStack) bool {
	if a.gss.head == nil || b.gss.head == nil {
		return false
	}
	if a.dead || b.dead || a.accepted != b.accepted {
		return false
	}
	if a.score != b.score || a.shifted != b.shifted {
		return false
	}
	if a.top().state != b.top().state || a.byteOffset != b.byteOffset {
		return false
	}
	return gssNodeCleanZeroErrorAllLinksWithScratch(scratch, a.gss.head) &&
		gssNodeCleanZeroErrorAllLinksWithScratch(scratch, b.gss.head)
}

func gssNodeByteOffset(n *gssNode) uint32 {
	for cur := n; cur != nil; cur = cur.prev {
		if stackEntryHasNode(cur.entry) {
			return stackEntryNodeEndByte(cur.entry)
		}
	}
	return 0
}

func gssNodesCanMerge(a, b *gssNode) bool {
	return gssNodesCanMergeWithScratch(nil, a, b)
}

func gssNodesCanMergeWithScratch(scratch *glrMergeScratch, a, b *gssNode) bool {
	if a == b {
		return true
	}
	if a == nil || b == nil {
		return false
	}
	if gssNodeCanReach(a, b) || gssNodeCanReach(b, a) {
		return false
	}
	if a.entry.state != b.entry.state {
		return false
	}
	if !gssNodeCleanZeroErrorAllLinksWithScratch(scratch, a) ||
		!gssNodeCleanZeroErrorAllLinksWithScratch(scratch, b) {
		return false
	}
	aOffset, aOK := gssNodeUniformByteOffset(a, make(map[*gssNode]bool))
	bOffset, bOK := gssNodeUniformByteOffset(b, make(map[*gssNode]bool))
	return aOK && bOK && aOffset == bOffset
}

func gssNodeCanReach(from, target *gssNode) bool {
	if from == nil || target == nil {
		return false
	}
	if from == target {
		return true
	}
	// Iterative DFS with a small linear visited set: these walks are almost
	// always tiny, and the per-call map this used to allocate was a top
	// profile cost on merge-heavy grammars (rust). Falls back to a map only
	// for pathologically large link graphs.
	var stackLocal [64]*gssNode
	var visitedLocal [64]*gssNode
	stack := stackLocal[:0]
	visited := visitedLocal[:0]
	var visitedMap map[*gssNode]bool
	isVisited := func(n *gssNode) bool {
		if visitedMap != nil {
			return visitedMap[n]
		}
		for _, v := range visited {
			if v == n {
				return true
			}
		}
		return false
	}
	markVisited := func(n *gssNode) {
		if visitedMap != nil {
			visitedMap[n] = true
			return
		}
		if len(visited) == cap(visited) && len(visited) >= len(visitedLocal) {
			visitedMap = make(map[*gssNode]bool, 2*len(visited))
			for _, v := range visited {
				visitedMap[v] = true
			}
			visitedMap[n] = true
			return
		}
		visited = append(visited, n)
	}
	stack = append(stack, from)
	for len(stack) > 0 {
		cur := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		if cur == nil || isVisited(cur) {
			continue
		}
		if cur == target {
			return true
		}
		markVisited(cur)
		for i := 0; i < cur.linkCount(); i++ {
			prev, _ := cur.link(i)
			stack = append(stack, prev)
		}
	}
	return false
}

func gssNodeUniformByteOffset(n *gssNode, seen map[*gssNode]bool) (uint32, bool) {
	if n == nil {
		return 0, true
	}
	if seen[n] {
		return gssNodeByteOffset(n), true
	}
	seen[n] = true
	var offset uint32
	haveOffset := false
	for i := 0; i < n.linkCount(); i++ {
		prev, entry := n.link(i)
		linkOffset, ok := gssLinkByteOffset(prev, entry, seen)
		if !ok {
			return 0, false
		}
		if !haveOffset {
			offset = linkOffset
			haveOffset = true
			continue
		}
		if offset != linkOffset {
			return 0, false
		}
	}
	return offset, true
}

func gssLinkByteOffset(prev *gssNode, entry stackEntry, seen map[*gssNode]bool) (uint32, bool) {
	if stackEntryHasNode(entry) {
		return stackEntryNodeEndByte(entry), true
	}
	return gssNodeUniformByteOffset(prev, seen)
}

func gssNodeCleanZeroErrorAllLinksWithScratch(scratch *glrMergeScratch, n *gssNode) bool {
	if n == nil {
		return true
	}
	var local glrMergeScratch
	if scratch == nil {
		scratch = &local
	}
	if scratch.cleanZeroEpoch == 0 {
		scratch.beginCleanZeroEpoch()
	}
	if clean, ok := lookupCleanZeroFrontCache(scratch, n); ok {
		return clean
	}
	if entry, ok := scratch.cleanZeroCache[n]; ok && entry.resultEpoch == scratch.cleanZeroEpoch {
		storeCleanZeroFrontCache(scratch, n, entry.clean)
		return entry.clean
	}
	if scratch.cleanZeroCache == nil {
		scratch.cleanZeroCache = make(map[*gssNode]gssCleanZeroErrorCacheEntry, 64)
	}
	if scratch.cleanZeroScan == ^uint32(0) {
		for node, entry := range scratch.cleanZeroCache {
			entry.scanEpoch = 0
			scratch.cleanZeroCache[node] = entry
		}
		scratch.cleanZeroScan = 0
	}
	scratch.cleanZeroScan++
	scanEpoch := scratch.cleanZeroScan
	stack := scratch.cleanZeroStack[:0]
	visited := scratch.cleanZeroVisited[:0]
	stack = append(stack, n)
	for len(stack) > 0 {
		last := len(stack) - 1
		cur := stack[last]
		stack = stack[:last]
		if cur == nil {
			continue
		}
		entry, ok := scratch.cleanZeroCache[cur]
		if ok && entry.resultEpoch == scratch.cleanZeroEpoch {
			if !entry.clean {
				scratch.cleanZeroCache[n] = gssCleanZeroErrorCacheEntry{resultEpoch: scratch.cleanZeroEpoch, clean: false}
				storeCleanZeroFrontCache(scratch, n, false)
				scratch.cleanZeroStack = stack[:0]
				scratch.cleanZeroVisited = visited[:0]
				return false
			}
			continue
		}
		if ok && entry.scanEpoch == scanEpoch {
			continue
		}
		entry.scanEpoch = scanEpoch
		scratch.cleanZeroCache[cur] = entry
		visited = append(visited, cur)
		for i := 0; i < cur.linkCount(); i++ {
			prev, linkEntry := cur.link(i)
			if stackEntryHasNode(linkEntry) &&
				(stackEntryNodeHasError(linkEntry) || stackEntryNodeIsMissing(linkEntry) || stackEntryNodeSymbol(linkEntry) == errorSymbol) {
				scratch.cleanZeroCache[cur] = gssCleanZeroErrorCacheEntry{resultEpoch: scratch.cleanZeroEpoch, clean: false}
				scratch.cleanZeroCache[n] = gssCleanZeroErrorCacheEntry{resultEpoch: scratch.cleanZeroEpoch, clean: false}
				storeCleanZeroFrontCache(scratch, n, false)
				scratch.cleanZeroStack = stack[:0]
				scratch.cleanZeroVisited = visited[:0]
				return false
			}
			stack = append(stack, prev)
		}
	}
	for _, node := range visited {
		scratch.cleanZeroCache[node] = gssCleanZeroErrorCacheEntry{resultEpoch: scratch.cleanZeroEpoch, clean: true}
	}
	storeCleanZeroFrontCache(scratch, n, true)
	scratch.cleanZeroStack = stack[:0]
	scratch.cleanZeroVisited = visited[:0]
	return true
}

func stackEntryPayloadsEquivalentIgnoringDynamic(a, b stackEntry) bool {
	an := stackEntryNode(a)
	bn := stackEntryNode(b)
	if an != nil && bn != nil {
		return stackEntryNodesEquivalentIgnoringDynamic(an, bn)
	}
	if !stackEntryHasNode(a) || !stackEntryHasNode(b) {
		return !stackEntryHasNode(a) && !stackEntryHasNode(b)
	}
	return stackEntryNodeSymbol(a) == stackEntryNodeSymbol(b) &&
		stackEntryNodeStartByte(a) == stackEntryNodeStartByte(b) &&
		stackEntryNodeEndByte(a) == stackEntryNodeEndByte(b) &&
		stackEntryNodeChildCount(a) == stackEntryNodeChildCount(b) &&
		stackEntryNodeFieldIDCount(a) == stackEntryNodeFieldIDCount(b) &&
		stackEntryNodeIsExtra(a) == stackEntryNodeIsExtra(b) &&
		stackEntryNodeIsNamed(a) == stackEntryNodeIsNamed(b) &&
		stackEntryNodeIsMissing(a) == stackEntryNodeIsMissing(b) &&
		stackEntryNodeHasError(a) == stackEntryNodeHasError(b) &&
		stackEntryNodeParseState(a) == stackEntryNodeParseState(b) &&
		stackEntryNodePreGotoState(a) == stackEntryNodePreGotoState(b) &&
		stackEntryNodeProductionID(a) == stackEntryNodeProductionID(b)
}

func stackEntryNodesEquivalentIgnoringDynamic(a, b *Node) bool {
	if a == b {
		return true
	}
	if a == nil || b == nil {
		return false
	}
	if a.symbol != b.symbol ||
		a.startByte != b.startByte ||
		a.endByte != b.endByte ||
		((a.flags^b.flags)&nodeStackEquivFlagMask) != 0 ||
		a.parseState != b.parseState ||
		a.preGotoState != b.preGotoState ||
		a.productionID != b.productionID ||
		len(a.fieldIDs) != len(b.fieldIDs) ||
		len(a.children) != len(b.children) {
		return false
	}
	for i := range a.fieldIDs {
		if a.fieldIDs[i] != b.fieldIDs[i] {
			return false
		}
	}
	if a.flags&nodeFlagHasError != 0 {
		return true
	}
	for i := range a.children {
		ca := a.children[i]
		cb := b.children[i]
		if ca == cb {
			continue
		}
		if ca == nil || cb == nil {
			return false
		}
		if ca.symbol != cb.symbol ||
			ca.startByte != cb.startByte ||
			ca.endByte != cb.endByte ||
			((ca.flags^cb.flags)&nodeStackEquivNoMissingFlagMask) != 0 ||
			ca.parseState != cb.parseState ||
			ca.preGotoState != cb.preGotoState ||
			ca.productionID != cb.productionID ||
			len(ca.fieldIDs) != len(cb.fieldIDs) ||
			len(ca.children) != len(cb.children) {
			return false
		}
	}
	return true
}

func setGSSMainLink(n *gssNode, i int, prev *gssNode, entry stackEntry) {
	if i == 0 {
		// Rewriting link 0 changes n's prev chain and payload, so every
		// cached GSS prefix aggregate at or above n is stale (see
		// gssPrefixAggGen, parser_recover_c.go).
		gssPrefixAggGen.Add(1)
		n.prev = prev
		n.entry = entry
		return
	}
	n.extraLinks[i-1] = gssMainLink{prev: prev, entry: entry}
}

func gssMainAddLink(n *gssNode, prev *gssNode, entry stackEntry) bool {
	return gssMainAddLinkSeen(n, prev, entry, make(map[gssMergePair]bool))
}

func cloneGSSMergeSeen(seen map[gssMergePair]bool) map[gssMergePair]bool {
	cloned := make(map[gssMergePair]bool, len(seen))
	for pair, ok := range seen {
		cloned[pair] = ok
	}
	return cloned
}

type gssMainPreflight struct {
	seen        map[gssMergePair]bool
	virtualLink map[*gssNode][]gssMainLink
	reachStrict bool
	reachEpoch  uint32
	reachCache  map[gssReachPair]gssReachCacheEntry
	reachSeen   map[*gssNode]bool
	reachStack  []*gssNode
	reachVisit  []*gssNode
	cleanCache  map[*gssNode]gssPreflightCleanCacheEntry
	cleanSeen   map[*gssNode]bool
	cleanStack  []*gssNode
	cleanVisit  []*gssNode
	// scratch, when non-nil, lets the preflight consult the parse-long
	// clean-zero caches instead of rebuilding a private verdict map per
	// preflight (valid only while no virtual links exist — virtual links can
	// change a node's all-links cleanliness mid-simulation).
	scratch *glrMergeScratch
	// offsetSeen is a reusable scratch map for uniformByteOffset walks
	// (cleared per use) so pooled preflights stop allocating two maps per
	// nodesCanMerge call.
	offsetSeen map[*gssNode]bool
}

const maxGSSPreflightReachCacheEntries = 32768

type gssReachPair struct {
	from   *gssNode
	target *gssNode
}

type gssReachCacheEntry struct {
	epoch     uint32
	reachable bool
}

type gssPreflightCleanCacheEntry struct {
	epoch uint32
	clean bool
}

func newGSSMainPreflight(seen map[gssMergePair]bool) *gssMainPreflight {
	return &gssMainPreflight{
		seen:        cloneGSSMergeSeen(seen),
		virtualLink: make(map[*gssNode][]gssMainLink),
		reachStrict: true,
		reachEpoch:  1,
	}
}

// acquirePreflightForScratch returns the scratch's pooled preflight, reset to
// the same observable state a fresh newGSSMainPreflight(seen) would have for
// an EMPTY seen map: all simulation caches invalidated (cleared), no virtual
// links, strict reachability. The pooled instance additionally carries
// scratch so cleanZeroErrorAllLinks can use the parse-long clean-zero caches
// while no virtual links exist. Callers must not retain the returned
// preflight beyond the current can-phase.
func acquirePreflightForScratch(scratch *glrMergeScratch) *gssMainPreflight {
	if scratch == nil {
		return newGSSMainPreflight(nil)
	}
	pf := scratch.preflight
	if pf == nil {
		pf = &gssMainPreflight{
			virtualLink: make(map[*gssNode][]gssMainLink),
			reachStrict: true,
			reachEpoch:  1,
		}
		scratch.preflight = pf
	}
	if pf.seen == nil {
		pf.seen = make(map[gssMergePair]bool, 16)
	} else if len(pf.seen) > 0 {
		clear(pf.seen)
	}
	if len(pf.virtualLink) > 0 {
		clear(pf.virtualLink)
	}
	if len(pf.reachCache) > 0 {
		clear(pf.reachCache)
	}
	if len(pf.reachSeen) > 0 {
		clear(pf.reachSeen)
	}
	if len(pf.cleanCache) > 0 {
		clear(pf.cleanCache)
	}
	if len(pf.cleanSeen) > 0 {
		clear(pf.cleanSeen)
	}
	pf.reachStrict = true
	pf.reachEpoch = 1
	pf.scratch = scratch
	return pf
}

// acquireMergeSeenForScratch returns the scratch's pooled (cleared) seen map
// for the GSS merge mutate phase, matching a fresh make(map[gssMergePair]bool).
func acquireMergeSeenForScratch(scratch *glrMergeScratch) map[gssMergePair]bool {
	if scratch == nil {
		return make(map[gssMergePair]bool)
	}
	if scratch.mergeSeen == nil {
		scratch.mergeSeen = make(map[gssMergePair]bool, 16)
	} else if len(scratch.mergeSeen) > 0 {
		clear(scratch.mergeSeen)
	}
	return scratch.mergeSeen
}

func (p *gssMainPreflight) linkCount(n *gssNode) int {
	if len(p.virtualLink) == 0 {
		// Fast path: no virtual links anywhere, skip the per-node map lookup
		// (this runs once per node visit in every preflight DFS).
		return n.linkCount()
	}
	return n.linkCount() + len(p.virtualLink[n])
}

func (p *gssMainPreflight) linkAt(n *gssNode, i int) (prev *gssNode, entry stackEntry) {
	realCount := n.linkCount()
	if i < realCount {
		return n.link(i)
	}
	l := p.virtualLink[n][i-realCount]
	return l.prev, l.entry
}

func (p *gssMainPreflight) addVirtualLink(n *gssNode, prev *gssNode, entry stackEntry) {
	p.virtualLink[n] = append(p.virtualLink[n], gssMainLink{prev: prev, entry: entry})
	if n != nil && prev != nil && prev.depth >= n.depth {
		p.reachStrict = false
	}
	p.bumpReachEpoch()
}

func (p *gssMainPreflight) bumpReachEpoch() {
	p.reachEpoch++
	if p.reachEpoch != 0 {
		return
	}
	clear(p.reachCache)
	p.reachEpoch = 1
}

func (p *gssMainPreflight) cachedReach(from, target *gssNode) (bool, bool) {
	entry, ok := p.reachCache[gssReachPair{from: from, target: target}]
	if !ok {
		return false, false
	}
	if entry.reachable {
		return true, true
	}
	if entry.epoch == p.reachEpoch {
		return false, true
	}
	return false, false
}

func (p *gssMainPreflight) cacheReach(from, target *gssNode, reachable bool) {
	if p.reachCache == nil {
		p.reachCache = make(map[gssReachPair]gssReachCacheEntry, 64)
	}
	if len(p.reachCache) >= maxGSSPreflightReachCacheEntries {
		return
	}
	entry := gssReachCacheEntry{reachable: reachable}
	if !reachable {
		entry.epoch = p.reachEpoch
	}
	p.reachCache[gssReachPair{from: from, target: target}] = entry
}

func (p *gssMainPreflight) canReach(from, target *gssNode) bool {
	if from == nil || target == nil {
		return false
	}
	if from == target {
		return true
	}
	if p.reachCache != nil {
		if reachable, ok := p.cachedReach(from, target); ok {
			return reachable
		}
	}
	if p.reachStrict && from.depth <= target.depth {
		return false
	}
	if p.reachSeen == nil {
		p.reachSeen = make(map[*gssNode]bool, 64)
	}
	stack := p.reachStack[:0]
	visited := p.reachVisit[:0]
	stack = append(stack, from)
	for len(stack) > 0 {
		last := len(stack) - 1
		cur := stack[last]
		stack = stack[:last]
		if cur == nil || p.reachSeen[cur] {
			continue
		}
		if cur == target {
			p.cacheReach(from, target, true)
			for _, node := range visited {
				delete(p.reachSeen, node)
			}
			p.reachStack = stack[:0]
			p.reachVisit = visited[:0]
			return true
		}
		if p.reachCache != nil {
			if reachable, ok := p.cachedReach(cur, target); ok {
				if reachable {
					p.cacheReach(from, target, true)
					for _, node := range visited {
						delete(p.reachSeen, node)
					}
					p.reachStack = stack[:0]
					p.reachVisit = visited[:0]
					return true
				}
				continue
			}
		}
		p.reachSeen[cur] = true
		visited = append(visited, cur)
		for i := 0; i < p.linkCount(cur); i++ {
			prev, _ := p.linkAt(cur, i)
			stack = append(stack, prev)
		}
	}
	p.cacheReach(from, target, false)
	for _, node := range visited {
		delete(p.reachSeen, node)
	}
	p.reachStack = stack[:0]
	p.reachVisit = visited[:0]
	return false
}

func (p *gssMainPreflight) cleanZeroErrorAllLinks(n *gssNode) bool {
	if n == nil {
		return true
	}
	if p.scratch != nil && len(p.virtualLink) == 0 {
		// With no virtual links the preflight's link view is exactly the real
		// graph, so the parse-long clean-zero caches give the same verdict the
		// private DFS below would compute — without rebuilding a per-preflight
		// verdict map every merge attempt.
		return gssNodeCleanZeroErrorAllLinksWithScratch(p.scratch, n)
	}
	if p.cleanCache != nil {
		if entry, ok := p.cleanCache[n]; ok {
			if !entry.clean {
				return false
			}
			if entry.epoch == p.reachEpoch {
				return true
			}
		}
	}
	if p.cleanCache == nil {
		p.cleanCache = make(map[*gssNode]gssPreflightCleanCacheEntry, 64)
	}
	if p.cleanSeen == nil {
		p.cleanSeen = make(map[*gssNode]bool, 64)
	}
	stack := p.cleanStack[:0]
	visited := p.cleanVisit[:0]
	stack = append(stack, n)
	for len(stack) > 0 {
		last := len(stack) - 1
		cur := stack[last]
		stack = stack[:last]
		if cur == nil || p.cleanSeen[cur] {
			continue
		}
		if entry, ok := p.cleanCache[cur]; ok {
			if !entry.clean {
				p.cleanCache[n] = gssPreflightCleanCacheEntry{clean: false}
				for _, node := range visited {
					delete(p.cleanSeen, node)
				}
				p.cleanStack = stack[:0]
				p.cleanVisit = visited[:0]
				return false
			}
			if entry.epoch == p.reachEpoch {
				continue
			}
		}
		p.cleanSeen[cur] = true
		visited = append(visited, cur)
		for i := 0; i < p.linkCount(cur); i++ {
			prev, entry := p.linkAt(cur, i)
			if stackEntryHasNode(entry) &&
				(stackEntryNodeHasError(entry) || stackEntryNodeIsMissing(entry) || stackEntryNodeSymbol(entry) == errorSymbol) {
				p.cleanCache[cur] = gssPreflightCleanCacheEntry{clean: false}
				p.cleanCache[n] = gssPreflightCleanCacheEntry{clean: false}
				for _, node := range visited {
					delete(p.cleanSeen, node)
				}
				p.cleanStack = stack[:0]
				p.cleanVisit = visited[:0]
				return false
			}
			stack = append(stack, prev)
		}
	}
	for _, node := range visited {
		p.cleanCache[node] = gssPreflightCleanCacheEntry{epoch: p.reachEpoch, clean: true}
		delete(p.cleanSeen, node)
	}
	p.cleanStack = stack[:0]
	p.cleanVisit = visited[:0]
	return true
}

func (p *gssMainPreflight) uniformByteOffset(n *gssNode, seen map[*gssNode]bool) (uint32, bool) {
	if n == nil {
		return 0, true
	}
	if seen[n] {
		return gssNodeByteOffset(n), true
	}
	seen[n] = true
	var offset uint32
	haveOffset := false
	for i := 0; i < p.linkCount(n); i++ {
		prev, entry := p.linkAt(n, i)
		linkOffset, ok := p.linkByteOffset(prev, entry, seen)
		if !ok {
			return 0, false
		}
		if !haveOffset {
			offset = linkOffset
			haveOffset = true
			continue
		}
		if offset != linkOffset {
			return 0, false
		}
	}
	return offset, true
}

func (p *gssMainPreflight) linkByteOffset(prev *gssNode, entry stackEntry, seen map[*gssNode]bool) (uint32, bool) {
	if stackEntryHasNode(entry) {
		return stackEntryNodeEndByte(entry), true
	}
	return p.uniformByteOffset(prev, seen)
}

func (p *gssMainPreflight) nodesCanMerge(a, b *gssNode) bool {
	if a == b {
		return true
	}
	if a == nil || b == nil {
		return false
	}
	if p.canReach(a, b) || p.canReach(b, a) {
		return false
	}
	if a.entry.state != b.entry.state {
		return false
	}
	if !p.cleanZeroErrorAllLinks(a) || !p.cleanZeroErrorAllLinks(b) {
		return false
	}
	aOffset, aOK := p.uniformByteOffset(a, p.acquireOffsetSeen())
	bOffset, bOK := p.uniformByteOffset(b, p.acquireOffsetSeen())
	return aOK && bOK && aOffset == bOffset
}

// acquireOffsetSeen returns the preflight's reusable (cleared) cycle-guard map
// for a single uniformByteOffset walk. Both walks in nodesCanMerge complete
// before any re-entrant preflight call, so one map suffices.
func (p *gssMainPreflight) acquireOffsetSeen() map[*gssNode]bool {
	if p.offsetSeen == nil {
		p.offsetSeen = make(map[*gssNode]bool, 16)
	} else if len(p.offsetSeen) > 0 {
		clear(p.offsetSeen)
	}
	return p.offsetSeen
}

func gssMainCanAddLinkSeen(n *gssNode, prev *gssNode, entry stackEntry, seen map[gssMergePair]bool) bool {
	return newGSSMainPreflight(seen).canAddLink(n, prev, entry)
}

func (p *gssMainPreflight) canAddLink(n *gssNode, prev *gssNode, entry stackEntry) bool {
	if n == nil {
		return false
	}
	if prev == n || p.canReach(prev, n) {
		return false
	}
	for i := 0; i < p.linkCount(n); i++ {
		existingPrev, existingEntry := p.linkAt(n, i)
		if !stackEntryPayloadsEquivalentIgnoringDynamic(existingEntry, entry) {
			continue
		}
		if existingPrev == prev {
			return true
		}
		if p.nodesCanMerge(existingPrev, prev) {
			return p.canMergeNodes(existingPrev, prev)
		}
	}
	if p.linkCount(n) >= maxMainLinkCount {
		return p.canReplaceWorstEquivalentLinkIfBetter(n, prev, entry)
	}
	p.addVirtualLink(n, prev, entry)
	return true
}

func gssMainAddLinkSeen(n *gssNode, prev *gssNode, entry stackEntry, seen map[gssMergePair]bool) bool {
	if !gssMainCanAddLinkSeen(n, prev, entry, seen) {
		return false
	}
	return gssMainAddLinkSeenMutate(n, prev, entry, seen)
}

func gssMainAddLinkSeenMutate(n *gssNode, prev *gssNode, entry stackEntry, seen map[gssMergePair]bool) bool {
	if n == nil {
		return false
	}
	if prev == n || gssNodeCanReach(prev, n) {
		return false
	}
	for i := 0; i < n.linkCount(); i++ {
		existingPrev, existingEntry := n.link(i)
		if !stackEntryPayloadsEquivalentIgnoringDynamic(existingEntry, entry) {
			continue
		}
		if existingPrev == prev {
			if stackEntryDynamicPrecedence(entry) > stackEntryDynamicPrecedence(existingEntry) {
				setGSSMainLink(n, i, prev, entry)
			}
			n.hash = 0
			return true
		}
		if gssNodesCanMerge(existingPrev, prev) {
			merged := gssMainMergeNodesSeenMutate(existingPrev, prev, seen)
			if merged && stackEntryDynamicPrecedence(entry) > stackEntryDynamicPrecedence(existingEntry) {
				setGSSMainLink(n, i, existingPrev, entry)
			}
			n.hash = 0
			return merged
		}
	}
	if n.linkCount() >= maxMainLinkCount {
		if gssMainReplaceWorstEquivalentLinkIfBetterMutate(n, prev, entry, seen) {
			n.hash = 0
			return true
		}
		return false
	}
	n.extraLinks = append(n.extraLinks, gssMainLink{prev: prev, entry: entry})
	n.hash = 0
	return true
}

func gssMainCanReplaceWorstEquivalentLinkIfBetter(n *gssNode, prev *gssNode, entry stackEntry, seen map[gssMergePair]bool) bool {
	return newGSSMainPreflight(seen).canReplaceWorstEquivalentLinkIfBetter(n, prev, entry)
}

func (p *gssMainPreflight) canReplaceWorstEquivalentLinkIfBetter(n *gssNode, prev *gssNode, entry stackEntry) bool {
	worst := -1
	worstPrecedence := stackEntryDynamicPrecedence(entry)
	var worstPrev *gssNode
	for i := 0; i < p.linkCount(n); i++ {
		existingPrev, existingEntry := p.linkAt(n, i)
		if !stackEntryPayloadsEquivalentIgnoringDynamic(existingEntry, entry) {
			continue
		}
		if existingPrev != prev && !p.nodesCanMerge(existingPrev, prev) {
			continue
		}
		existingPrecedence := stackEntryDynamicPrecedence(existingEntry)
		if worst == -1 || existingPrecedence < worstPrecedence {
			worst = i
			worstPrecedence = existingPrecedence
			worstPrev = existingPrev
		}
	}
	if worst == -1 || stackEntryDynamicPrecedence(entry) <= worstPrecedence {
		return false
	}
	if worstPrev != prev {
		return p.canMergeNodes(worstPrev, prev)
	}
	return true
}

func gssMainReplaceWorstEquivalentLinkIfBetter(n *gssNode, prev *gssNode, entry stackEntry) bool {
	if !gssMainCanReplaceWorstEquivalentLinkIfBetter(n, prev, entry, make(map[gssMergePair]bool)) {
		return false
	}
	return gssMainReplaceWorstEquivalentLinkIfBetterMutate(n, prev, entry, make(map[gssMergePair]bool))
}

func gssMainReplaceWorstEquivalentLinkIfBetterMutate(n *gssNode, prev *gssNode, entry stackEntry, seen map[gssMergePair]bool) bool {
	worst := -1
	worstPrecedence := stackEntryDynamicPrecedence(entry)
	var worstPrev *gssNode
	for i := 0; i < n.linkCount(); i++ {
		existingPrev, existingEntry := n.link(i)
		if !stackEntryPayloadsEquivalentIgnoringDynamic(existingEntry, entry) {
			continue
		}
		if existingPrev != prev && !gssNodesCanMerge(existingPrev, prev) {
			continue
		}
		existingPrecedence := stackEntryDynamicPrecedence(existingEntry)
		if worst == -1 || existingPrecedence < worstPrecedence {
			worst = i
			worstPrecedence = existingPrecedence
			worstPrev = existingPrev
		}
	}
	if worst == -1 || stackEntryDynamicPrecedence(entry) <= worstPrecedence {
		return false
	}
	if worstPrev != prev {
		if !gssMainMergeNodesSeenMutate(worstPrev, prev, seen) {
			return false
		}
		prev = worstPrev
	}
	setGSSMainLink(n, worst, prev, entry)
	return true
}

type gssMergePair struct {
	a *gssNode
	b *gssNode
}

func gssMainMergeNodes(a, b *gssNode) bool {
	return gssMainMergeNodesSeen(a, b, make(map[gssMergePair]bool))
}

func gssMainCanMergeNodesSeen(a, b *gssNode, seen map[gssMergePair]bool) bool {
	return newGSSMainPreflight(seen).canMergeNodes(a, b)
}

func (p *gssMainPreflight) canMergeNodes(a, b *gssNode) bool {
	if a == nil || b == nil || a == b {
		return true
	}
	if p.canReach(b, a) {
		return false
	}
	pair := gssMergePair{a: a, b: b}
	if p.seen[pair] {
		return true
	}
	p.seen[pair] = true
	count := p.linkCount(b)
	for i := 0; i < count; i++ {
		prev, entry := p.linkAt(b, i)
		if !p.canAddLink(a, prev, entry) {
			return false
		}
	}
	return true
}

func gssMainMergeNodesSeen(a, b *gssNode, seen map[gssMergePair]bool) bool {
	if !gssMainCanMergeNodesSeen(a, b, seen) {
		return false
	}
	return gssMainMergeNodesSeenMutate(a, b, seen)
}

func gssMainMergeNodesSeenMutate(a, b *gssNode, seen map[gssMergePair]bool) bool {
	if a == nil || b == nil || a == b {
		return true
	}
	if gssNodeCanReach(b, a) {
		return false
	}
	pair := gssMergePair{a: a, b: b}
	if seen[pair] {
		return true
	}
	seen[pair] = true
	mergedAll := true
	count := b.linkCount()
	for i := 0; i < count; i++ {
		prev, entry := b.link(i)
		if !gssMainAddLinkSeenMutate(a, prev, entry, seen) {
			mergedAll = false
		}
	}
	return mergedAll
}

func gssMainMerge(a, b *glrStack) bool {
	return gssMainMergeWithScratch(nil, a, b)
}

// gssMainMergeWithScratch is gssMainMerge with a pooled preflight + seen map
// when scratch is non-nil: the can-phase and mutate-phase each get a cleared
// reusable map instead of freshly allocated ones (identical observable
// semantics — gssMainMergeNodes always started both phases with empty maps).
func gssMainMergeWithScratch(scratch *glrMergeScratch, a, b *glrStack) bool {
	ah, bh := a.gss.head, b.gss.head
	if ah == nil || bh == nil {
		return false
	}
	if ah == bh {
		return true
	}
	if scratch == nil {
		return gssMainMergeNodes(ah, bh)
	}
	pf := acquirePreflightForScratch(scratch)
	if !pf.canMergeNodes(ah, bh) {
		return false
	}
	return gssMainMergeNodesSeenMutate(ah, bh, acquireMergeSeenForScratch(scratch))
}

func tryGSSMainMergeResult(scratch *glrMergeScratch, result []glrStack, idx int, stack *glrStack) (merged bool, attempted bool) {
	if idx < 0 || idx >= len(result) || stack == nil {
		return false, false
	}
	if cRecoveryMergeCostsDiffer(scratch, &result[idx], stack) {
		traceCRecoverMergeDecision(scratch, "gss", "reject-cost", result[idx], *stack)
		return false, false
	}
	if !gssMainCanMergeWithScratch(scratch, &result[idx], stack) {
		return false, false
	}
	if (scratch == nil || scratch.perKeyCap != 1) &&
		gssStacksHaveDistinctMaterializingShapesWithScratch(scratch, &result[idx], stack) {
		return false, true
	}
	merged = gssMainMergeWithScratch(scratch, &result[idx], stack)
	if merged {
		// result[idx] survives and absorbs stack, so OR the sticky wreckage bit:
		// a clean survivor that merges a recovered-wreckage lineage must inherit
		// its error history (see glrStack.cEverErrored / tryGSSMainMergeForParser).
		result[idx].cEverErrored = result[idx].cEverErrored || stack.cEverErrored
		if scratch != nil {
			// A successful main merge can rewrite link 0 (prev/entry) of surviving
			// nodes (setGSSMainLink), so every cached spine prefix may be stale.
			scratch.bumpShapePrefixEpoch()
		}
	}
	return merged, true
}

func preserveCapOneStackInSlot(result *[]glrStack, slot *glrMergeSlot, stack glrStack, hash uint64) bool {
	if result == nil || slot == nil {
		return false
	}
	idx := len(*result)
	*result = append(*result, stack)
	if slot.count >= len(slot.indices) {
		slot.extraIndices = append(slot.extraIndices, idx)
		slot.extraHashes = append(slot.extraHashes, hash)
		slot.hashMask |= mergeHashBit(hash)
		if slot.worstIndex < 0 || stackCompareMerge(&(*result)[idx], &(*result)[slot.worstIndex]) < 0 {
			slot.worstIndex = idx
		}
		return true
	}
	slot.indices[slot.count] = idx
	slot.hashes[slot.count] = hash
	slot.hashMask |= mergeHashBit(hash)
	slot.count++
	if slot.worstIndex < 0 || stackCompareMerge(&(*result)[idx], &(*result)[slot.worstIndex]) < 0 {
		slot.worstIndex = idx
	}
	return true
}

func mergeSlotTrackedCount(slot *glrMergeSlot) int {
	if slot == nil {
		return 0
	}
	return slot.count + len(slot.extraIndices)
}

func mergeSlotIndexAt(slot *glrMergeSlot, pos int) int {
	if pos < slot.count {
		return slot.indices[pos]
	}
	return slot.extraIndices[pos-slot.count]
}

func mergeSlotHashAt(slot *glrMergeSlot, pos int) uint64 {
	if pos < slot.count {
		return slot.hashes[pos]
	}
	return slot.extraHashes[pos-slot.count]
}

func mergeSlotSetHashAt(slot *glrMergeSlot, pos int, hash uint64) {
	if pos < slot.count {
		slot.hashes[pos] = hash
		return
	}
	slot.extraHashes[pos-slot.count] = hash
}

func mergeSlotPositionForIndex(slot *glrMergeSlot, idx int) int {
	if slot == nil {
		return -1
	}
	for j := 0; j < slot.count; j++ {
		if slot.indices[j] == idx {
			return j
		}
	}
	for j := range slot.extraIndices {
		if slot.extraIndices[j] == idx {
			return slot.count + j
		}
	}
	return -1
}

func cRecoveryCostClassForSlot(scratch *glrMergeScratch, result []glrStack, slot *glrMergeSlot, stack *glrStack) (sameCostIndex int, preserveNewCost bool) {
	if scratch == nil || !scratch.cRecoveryCost || slot == nil || stack == nil || mergeSlotTrackedCount(slot) == 0 {
		return -1, false
	}
	candidateCost := cStackErrorCostForMergeCached(scratch, scratch.language, stack)
	sawDifferentCost := false
	for j, n := 0, mergeSlotTrackedCount(slot); j < n; j++ {
		idx := mergeSlotIndexAt(slot, j)
		if idx < 0 || idx >= len(result) || !stacksHeaderEquivalent(result[idx], *stack) {
			continue
		}
		if cStackErrorCostForMergeCached(scratch, scratch.language, &result[idx]) == candidateCost {
			return idx, false
		}
		sawDifferentCost = true
	}
	return -1, sawDifferentCost
}

func cRecoveryCostClassForSlice(scratch *glrMergeScratch, result []glrStack, key glrMergeKey, stack *glrStack) (sameCostIndex int, preserveNewCost bool) {
	if scratch == nil || !scratch.cRecoveryCost || stack == nil {
		return -1, false
	}
	candidateCost := cStackErrorCostForMergeCached(scratch, scratch.language, stack)
	sawDifferentCost := false
	for j := range result {
		if mergeKeyForStack(result[j]) != key || !stacksHeaderEquivalent(result[j], *stack) {
			continue
		}
		if cStackErrorCostForMergeCached(scratch, scratch.language, &result[j]) == candidateCost {
			return j, false
		}
		sawDifferentCost = true
	}
	return -1, sawDifferentCost
}

func preferOverflowCandidate(candidate, incumbent *glrStack, candidateHash, incumbentHash uint64) bool {
	cmp := stackCompareMerge(candidate, incumbent)
	if cmp != 0 {
		return cmp > 0
	}
	// Equal-ranked candidates should not depend on insertion order.
	// Deterministically keep the higher hash to preserve diversity.
	return candidateHash > incumbentHash
}

func mergeStacksSmallForLanguage(alive []glrStack, scratch *glrMergeScratch, lang *Language) []glrStack {
	if len(alive) <= 1 {
		return alive
	}
	if scratch != nil && scratch.deferExactDedupe {
		return mergeStacksSmallDeferExact(alive, scratch, lang)
	}
	result := alive[:0]
	for i := range alive {
		stack := alive[i]
		key := mergeKeyForStack(stack)
		duplicateIndex := -1
		mergedByGSS := false
		preserveByGSS := false
		cRecoverySameCostIndex := -1
		cRecoveryPreserveNewCost := false
		if scratch != nil && scratch.perKeyCap == 1 {
			cRecoverySameCostIndex, cRecoveryPreserveNewCost = cRecoveryCostClassForSlice(scratch, result, key, &stack)
		}
		for j := range result {
			if mergeKeyForStack(result[j]) != key {
				continue
			}
			if merged, attempted := tryGSSMainMergeResult(scratch, result, j, &stack); attempted {
				if merged {
					traceCRecoverMergeDecision(scratch, "small", "gss-merged", result[j], stack)
					duplicateIndex = j
					mergedByGSS = true
				} else {
					traceCRecoverMergeDecision(scratch, "small", "gss-preserve", result[j], stack)
					preserveByGSS = true
				}
				break
			}
			if scratch != nil && scratch.perKeyCap == 1 {
				if cRecoveryPreserveNewCost {
					traceCRecoverMergeDecision(scratch, "small", "preserve-cost", result[j], stack)
					preserveByGSS = true
					break
				}
				if cRecoverySameCostIndex >= 0 && j != cRecoverySameCostIndex {
					continue
				}
				cmp := stackCompareMergeSmallCapOne(&stack, &result[j])
				if cmp > 0 {
					result[j] = stack
					duplicateIndex = j
					break
				}
				if cmp < 0 {
					duplicateIndex = j
					break
				}
			}
			if stackEquivalentForMergeState(scratch, lang, key.state, result[j], stack) {
				traceCRecoverMergeDecision(scratch, "small", "equivalent", result[j], stack)
				duplicateIndex = j
				break
			}
		}
		if duplicateIndex < 0 || preserveByGSS {
			result = append(result, stack)
			continue
		}
		if mergedByGSS {
			continue
		}
		if stackCompareMerge(&stack, &result[duplicateIndex]) >= 0 {
			traceCRecoverMergeDecision(scratch, "small", "replace-duplicate", result[duplicateIndex], stack)
			result[duplicateIndex] = stack
		} else {
			traceCRecoverMergeDecision(scratch, "small", "drop-duplicate", result[duplicateIndex], stack)
		}
	}
	return result
}

func mergeStacksSmallDeferExact(alive []glrStack, scratch *glrMergeScratch, lang *Language) []glrStack {
	perKeyCap := maxStacksPerMergeKey
	if scratch != nil && scratch.perKeyCap > 0 {
		perKeyCap = scratch.perKeyCap
	}
	result := alive[:0]
	var resultKeys [maxGLRStacks]glrMergeKey
	for i := range alive {
		stack := alive[i]
		key := mergeKeyForStack(stack)
		duplicateIndex := -1
		mergedByGSS := false
		sameKeyCount := 0
		cRecoverySameCostIndex := -1
		cRecoveryPreserveNewCost := false
		if scratch != nil && scratch.perKeyCap == 1 {
			cRecoverySameCostIndex, cRecoveryPreserveNewCost = cRecoveryCostClassForSlice(scratch, result, key, &stack)
		}
		for j := range result {
			if resultKeys[j] != key {
				continue
			}
			sameKeyCount++
			if merged, attempted := tryGSSMainMergeResult(scratch, result, j, &stack); attempted {
				if merged {
					traceCRecoverMergeDecision(scratch, "small-defer", "gss-merged", result[j], stack)
					duplicateIndex = j
					mergedByGSS = true
				} else {
					traceCRecoverMergeDecision(scratch, "small-defer", "gss-preserve", result[j], stack)
					sameKeyCount = perKeyCap
				}
				break
			}
			if scratch != nil && scratch.perKeyCap == 1 {
				if cRecoveryPreserveNewCost {
					traceCRecoverMergeDecision(scratch, "small-defer", "preserve-cost", result[j], stack)
					sameKeyCount = perKeyCap
					break
				}
				if cRecoverySameCostIndex >= 0 && j != cRecoverySameCostIndex {
					continue
				}
				cmp := stackCompareMergeSmallCapOne(&stack, &result[j])
				if cmp > 0 {
					result[j] = stack
					duplicateIndex = j
					break
				}
				if cmp < 0 {
					duplicateIndex = j
					break
				}
			}
			if sameKeyCount < perKeyCap {
				continue
			}
			if stackEquivalentForMergeState(scratch, lang, key.state, result[j], stack) {
				traceCRecoverMergeDecision(scratch, "small-defer", "equivalent", result[j], stack)
				duplicateIndex = j
				break
			}
		}
		if duplicateIndex < 0 {
			resultKeys[len(result)] = key
			result = append(result, stack)
			continue
		}
		if mergedByGSS {
			continue
		}
		if stackCompareMerge(&stack, &result[duplicateIndex]) >= 0 {
			traceCRecoverMergeDecision(scratch, "small-defer", "replace-duplicate", result[duplicateIndex], stack)
			result[duplicateIndex] = stack
		} else {
			traceCRecoverMergeDecision(scratch, "small-defer", "drop-duplicate", result[duplicateIndex], stack)
		}
	}
	return result
}

// mergeStacksWithScratch performs bounded merge/pruning in three phases:
//  1. drop dead stacks
//  2. group by (state, byteOffset) merge key
//  3. within each key keep exact-equivalent dedupes plus at most N survivors
//     chosen by stackCompareMerge (with hash prefilter before deep equivalence)
func mergeStacksWithScratch(stacks []glrStack, scratch *glrMergeScratch) []glrStack {
	if len(stacks) == 0 {
		return stacks
	}
	if perfCountersEnabled {
		perfRecordMergeCall(len(stacks))
	}

	// Remove dead stacks first. Most merge calls have no dead stacks; avoid
	// copying the full live slice in that case.
	alive := stacks
	deadCount := 0
	firstDead := -1
	for i := range stacks {
		if stacks[i].dead {
			firstDead = i
			deadCount = 1
			break
		}
	}
	if firstDead >= 0 {
		alive = stacks[:firstDead]
		for i := firstDead + 1; i < len(stacks); i++ {
			if stacks[i].dead {
				deadCount++
				continue
			}
			alive = append(alive, stacks[i])
		}
	}
	if perfCountersEnabled {
		perfRecordMergeAlive(len(alive), deadCount)
	}
	if len(alive) <= 1 {
		return alive
	}
	if scratch == nil {
		local := glrMergeScratch{}
		local.beginEquivEpoch()
		scratch = &local
	}
	if limit := mergeAliveLimitForScratch(scratch, len(alive)); limit > 0 && len(alive) > limit {
		alive = retainTopStacksForLanguage(alive, limit, scratch.language)
	}
	if len(alive) <= 4 {
		result := mergeStacksSmallForLanguage(alive, scratch, scratch.language)
		if perfCountersEnabled {
			perfRecordMergeOut(len(result))
		}
		return result
	}

	perKeyCap := maxStacksPerMergeKey
	if scratch.perKeyCap > 0 {
		perKeyCap = scratch.perKeyCap
	}
	if perKeyCap < 1 {
		perKeyCap = 1
	}
	if perKeyCap > maxStacksPerMergeKeyCeiling {
		perKeyCap = maxStacksPerMergeKeyCeiling
	}
	if perKeyCap > maxStacksPerMergeKey {
		return mergeStacksWithScratchLargeCap(alive, scratch, perKeyCap)
	}
	if scratch.deferExactDedupe {
		return mergeStacksWithScratchDeferExact(alive, scratch, perKeyCap)
	}

	// Merge exact duplicates and keep a bounded number of distinct
	// alternatives per merge key. This approximates the C runtime's
	// graph-stack link fanout while keeping memory bounded.
	result := ensureMergeResultCap(scratch, len(alive))
	slots := ensureMergeSlotCap(scratch, len(alive))
	slotCount := 0
	for i := range alive {
		stack := alive[i]
		hash := stackHashForMerge(scratch, scratch.language, stack)
		key := mergeKeyForStack(stack)

		slotIndex := -1
		for si := 0; si < slotCount; si++ {
			if slots[si].key == key {
				slotIndex = si
				break
			}
		}
		if slotIndex < 0 {
			slotIndex = slotCount
			slotCount++
			slots[slotIndex].key = key
			slots[slotIndex].count = 0
			slots[slotIndex].worstIndex = -1
			slots[slotIndex].hashMask = 0
			slots[slotIndex].extraIndices = slots[slotIndex].extraIndices[:0]
			slots[slotIndex].extraHashes = slots[slotIndex].extraHashes[:0]
		}
		slot := &slots[slotIndex]

		if mergeSlotTrackedCount(slot) > 0 {
			mergedByGSS := false
			for j, n := 0, mergeSlotTrackedCount(slot); j < n; j++ {
				idx := mergeSlotIndexAt(slot, j)
				merged, attempted := tryGSSMainMergeResult(scratch, result, idx, &stack)
				if attempted && merged {
					mergedByGSS = true
					break
				}
			}
			if mergedByGSS {
				continue
			}
		}

		if perKeyCap == 1 && mergeSlotTrackedCount(slot) > 0 {
			idx, preserveCost := cRecoveryCostClassForSlot(scratch, result, slot, &stack)
			if preserveCost {
				idx = mergeSlotIndexAt(slot, 0)
				traceCRecoverMergeDecision(scratch, "default", "preserve-cost", result[idx], stack)
				_ = preserveCapOneStackInSlot(&result, slot, stack, hash)
				continue
			}
			if idx < 0 {
				idx = mergeSlotIndexAt(slot, 0)
			}
			cmp := stackCompareMergeSmallCapOne(&stack, &result[idx])
			if cmp > 0 {
				result[idx] = stack
				if pos := mergeSlotPositionForIndex(slot, idx); pos >= 0 {
					mergeSlotSetHashAt(slot, pos, hash)
				}
				slot.hashMask = recomputeMergeSlotHashMask(slot)
				slot.worstIndex = recomputeMergeSlotWorst(slot, result)
				if perfCountersEnabled {
					perfRecordMergeReplacement()
				}
				continue
			}
			if cmp < 0 {
				continue
			}
			if merged, attempted := tryGSSMainMergeResult(scratch, result, idx, &stack); attempted {
				if !merged {
					_ = preserveCapOneStackInSlot(&result, slot, stack, hash)
				}
				continue
			}
			if scratch != nil && scratch.cRecoveryCost {
				continue
			}
		}

		duplicateIndex := -1
		hashMatched := false
		if mergeSlotTrackedCount(slot) > 0 && (slot.hashMask&mergeHashBit(hash)) != 0 {
			for j, n := 0, mergeSlotTrackedCount(slot); j < n; j++ {
				if mergeSlotHashAt(slot, j) != hash {
					continue
				}
				hashMatched = true
				idx := mergeSlotIndexAt(slot, j)
				existing := &result[idx]
				if stackEquivalentForMergeState(scratch, scratch.language, key.state, *existing, stack) {
					duplicateIndex = idx
					break
				}
			}
		}
		if !hashMatched && mergeSlotTrackedCount(slot) > 0 && perfCountersEnabled {
			perfRecordStackEquivalentHashMissSkip()
		}
		if duplicateIndex >= 0 {
			// Equal-ranked duplicates should not preserve the first-inserted
			// branch by accident. Let later survivors replace ties so
			// post-reduce reprocessing can keep the branch that stayed viable.
			if merged, attempted := tryGSSMainMergeResult(scratch, result, duplicateIndex, &stack); attempted {
				if !merged {
					_ = preserveCapOneStackInSlot(&result, slot, stack, hash)
				}
				continue
			}
			if stackCompareMerge(&stack, &result[duplicateIndex]) >= 0 {
				result[duplicateIndex] = stack
				if pos := mergeSlotPositionForIndex(slot, duplicateIndex); pos >= 0 {
					mergeSlotSetHashAt(slot, pos, hash)
				}
				if slot.worstIndex == duplicateIndex {
					slot.worstIndex = recomputeMergeSlotWorst(slot, result)
				}
			}
			continue
		}

		if slot.count < perKeyCap {
			idx := len(result)
			result = append(result, stack)
			slot.indices[slot.count] = idx
			slot.hashes[slot.count] = hash
			slot.hashMask |= mergeHashBit(hash)
			slot.count++
			if slot.worstIndex < 0 || stackCompareMerge(&result[idx], &result[slot.worstIndex]) < 0 {
				slot.worstIndex = idx
			}
			continue
		}
		if perfCountersEnabled {
			perfRecordMergePerKeyOverflow()
		}
		if perKeyCap == 1 && glrFaithfulCapOneMerge {
			merged := false
			attempted := false
			for j, n := 0, mergeSlotTrackedCount(slot); j < n; j++ {
				idx := mergeSlotIndexAt(slot, j)
				m, a := tryGSSMainMergeResult(scratch, result, idx, &stack)
				if a {
					attempted = true
					if m {
						merged = true
						break
					}
				}
			}
			if merged || (attempted && preserveCapOneStackInSlot(&result, slot, stack, hash)) {
				continue
			}
		}

		// Per-key alternative budget reached: replace the weakest
		// retained candidate only if this stack is better.
		if slot.worstIndex >= 0 {
			replacedSlot := mergeSlotPositionForIndex(slot, slot.worstIndex)
			incumbentHash := uint64(0)
			if replacedSlot >= 0 {
				incumbentHash = mergeSlotHashAt(slot, replacedSlot)
			}
			if !preferOverflowCandidate(&stack, &result[slot.worstIndex], hash, incumbentHash) {
				continue
			}
			if perfCountersEnabled {
				perfRecordMergeReplacement()
			}
			result[slot.worstIndex] = stack
			if replacedSlot >= 0 {
				mergeSlotSetHashAt(slot, replacedSlot, hash)
				slot.hashMask = recomputeMergeSlotHashMask(slot)
			}
			slot.worstIndex = recomputeMergeSlotWorst(slot, result)
		}
	}
	if perfCountersEnabled {
		perfRecordMergeOut(len(result))
	}
	if scratch.audit != nil {
		scratch.audit.recordMerge(len(alive), len(result), slotCount)
	}
	scratch.result = result
	scratch.slots = slots[:slotCount]
	return result
}

func mergeStacksWithScratchDeferExact(alive []glrStack, scratch *glrMergeScratch, perKeyCap int) []glrStack {
	result := ensureMergeResultCap(scratch, len(alive))
	slots := ensureMergeSlotCap(scratch, len(alive))
	slotCount := 0
	for i := range alive {
		stack := alive[i]
		hash := stackHashForMerge(scratch, scratch.language, stack)
		key := mergeKeyForStack(stack)

		slotIndex := -1
		for si := 0; si < slotCount; si++ {
			if slots[si].key == key {
				slotIndex = si
				break
			}
		}
		if slotIndex < 0 {
			slotIndex = slotCount
			slotCount++
			slots[slotIndex].key = key
			slots[slotIndex].count = 0
			slots[slotIndex].worstIndex = -1
			slots[slotIndex].hashMask = 0
			slots[slotIndex].extraIndices = slots[slotIndex].extraIndices[:0]
			slots[slotIndex].extraHashes = slots[slotIndex].extraHashes[:0]
		}
		slot := &slots[slotIndex]

		if mergeSlotTrackedCount(slot) > 0 {
			mergedByGSS := false
			for j, n := 0, mergeSlotTrackedCount(slot); j < n; j++ {
				idx := mergeSlotIndexAt(slot, j)
				merged, attempted := tryGSSMainMergeResult(scratch, result, idx, &stack)
				if attempted && merged {
					mergedByGSS = true
					break
				}
			}
			if mergedByGSS {
				continue
			}
		}

		if perKeyCap == 1 && mergeSlotTrackedCount(slot) > 0 {
			idx, preserveCost := cRecoveryCostClassForSlot(scratch, result, slot, &stack)
			if preserveCost {
				idx = mergeSlotIndexAt(slot, 0)
				traceCRecoverMergeDecision(scratch, "defer", "preserve-cost", result[idx], stack)
				_ = preserveCapOneStackInSlot(&result, slot, stack, hash)
				continue
			}
			if idx < 0 {
				idx = mergeSlotIndexAt(slot, 0)
			}
			cmp := stackCompareMergeSmallCapOne(&stack, &result[idx])
			if cmp > 0 {
				result[idx] = stack
				if pos := mergeSlotPositionForIndex(slot, idx); pos >= 0 {
					mergeSlotSetHashAt(slot, pos, hash)
				}
				slot.hashMask = recomputeMergeSlotHashMask(slot)
				slot.worstIndex = recomputeMergeSlotWorst(slot, result)
				if perfCountersEnabled {
					perfRecordMergeReplacement()
				}
				continue
			}
			if cmp < 0 {
				continue
			}
			if merged, attempted := tryGSSMainMergeResult(scratch, result, idx, &stack); attempted {
				if !merged {
					_ = preserveCapOneStackInSlot(&result, slot, stack, hash)
				}
				continue
			}
			if scratch != nil && scratch.cRecoveryCost {
				continue
			}
		}

		duplicateIndex := -1
		hashMatched := false
		if mergeSlotTrackedCount(slot) >= perKeyCap && (slot.hashMask&mergeHashBit(hash)) != 0 {
			for j, n := 0, mergeSlotTrackedCount(slot); j < n; j++ {
				if mergeSlotHashAt(slot, j) != hash {
					continue
				}
				hashMatched = true
				idx := mergeSlotIndexAt(slot, j)
				existing := &result[idx]
				if stackEquivalentForMergeState(scratch, scratch.language, key.state, *existing, stack) {
					duplicateIndex = idx
					break
				}
			}
		}
		if !hashMatched && mergeSlotTrackedCount(slot) >= perKeyCap && perfCountersEnabled {
			perfRecordStackEquivalentHashMissSkip()
		}
		if duplicateIndex >= 0 {
			if merged, attempted := tryGSSMainMergeResult(scratch, result, duplicateIndex, &stack); attempted {
				if !merged {
					_ = preserveCapOneStackInSlot(&result, slot, stack, hash)
				}
				continue
			}
			if stackCompareMerge(&stack, &result[duplicateIndex]) >= 0 {
				result[duplicateIndex] = stack
				if pos := mergeSlotPositionForIndex(slot, duplicateIndex); pos >= 0 {
					mergeSlotSetHashAt(slot, pos, hash)
				}
				if slot.worstIndex == duplicateIndex {
					slot.worstIndex = recomputeMergeSlotWorst(slot, result)
				}
			}
			continue
		}

		if slot.count < perKeyCap {
			idx := len(result)
			result = append(result, stack)
			slot.indices[slot.count] = idx
			slot.hashes[slot.count] = hash
			slot.hashMask |= mergeHashBit(hash)
			slot.count++
			if slot.worstIndex < 0 || stackCompareMerge(&result[idx], &result[slot.worstIndex]) < 0 {
				slot.worstIndex = idx
			}
			continue
		}
		if perfCountersEnabled {
			perfRecordMergePerKeyOverflow()
		}
		if perKeyCap == 1 && glrFaithfulCapOneMerge {
			merged := false
			attempted := false
			for j, n := 0, mergeSlotTrackedCount(slot); j < n; j++ {
				idx := mergeSlotIndexAt(slot, j)
				m, a := tryGSSMainMergeResult(scratch, result, idx, &stack)
				if a {
					attempted = true
					if m {
						merged = true
						break
					}
				}
			}
			if merged || (attempted && preserveCapOneStackInSlot(&result, slot, stack, hash)) {
				continue
			}
		}

		if slot.worstIndex >= 0 {
			replacedSlot := mergeSlotPositionForIndex(slot, slot.worstIndex)
			incumbentHash := uint64(0)
			if replacedSlot >= 0 {
				incumbentHash = mergeSlotHashAt(slot, replacedSlot)
			}
			if !preferOverflowCandidate(&stack, &result[slot.worstIndex], hash, incumbentHash) {
				continue
			}
			if perfCountersEnabled {
				perfRecordMergeReplacement()
			}
			result[slot.worstIndex] = stack
			if replacedSlot >= 0 {
				mergeSlotSetHashAt(slot, replacedSlot, hash)
				slot.hashMask = recomputeMergeSlotHashMask(slot)
			}
			slot.worstIndex = recomputeMergeSlotWorst(slot, result)
		}
	}
	if perfCountersEnabled {
		perfRecordMergeOut(len(result))
	}
	if scratch.audit != nil {
		scratch.audit.recordMerge(len(alive), len(result), slotCount)
	}
	scratch.result = result
	scratch.slots = slots[:slotCount]
	return result
}

func mergeStacksWithScratchLargeCap(alive []glrStack, scratch *glrMergeScratch, perKeyCap int) []glrStack {
	result := ensureMergeResultCap(scratch, len(alive))
	slots := ensureMergeLargeSlotCap(scratch, len(alive))
	slotCount := 0
	for i := range alive {
		stack := alive[i]
		hash := stackHashForMerge(scratch, scratch.language, stack)
		key := mergeKeyForStack(stack)

		slotIndex := -1
		for si := 0; si < slotCount; si++ {
			if slots[si].key == key {
				slotIndex = si
				break
			}
		}
		if slotIndex < 0 {
			slotIndex = slotCount
			slotCount++
			slots[slotIndex].key = key
			slots[slotIndex].count = 0
			slots[slotIndex].worstIndex = -1
			slots[slotIndex].hashMask = 0
		}
		slot := &slots[slotIndex]

		if slot.count > 0 {
			mergedByGSS := false
			for j := 0; j < slot.count; j++ {
				idx := slot.indices[j]
				merged, attempted := tryGSSMainMergeResult(scratch, result, idx, &stack)
				if attempted && merged {
					mergedByGSS = true
					break
				}
			}
			if mergedByGSS {
				continue
			}
		}

		duplicateIndex := -1
		hashMatched := false
		if slot.count > 0 && (slot.hashMask&mergeHashBit(hash)) != 0 {
			for j := 0; j < slot.count; j++ {
				if slot.hashes[j] != hash {
					continue
				}
				hashMatched = true
				idx := slot.indices[j]
				existing := &result[idx]
				if stackEquivalentForMergeState(scratch, scratch.language, key.state, *existing, stack) {
					duplicateIndex = idx
					break
				}
			}
		}
		if !hashMatched && slot.count > 0 && perfCountersEnabled {
			perfRecordStackEquivalentHashMissSkip()
		}
		if duplicateIndex >= 0 {
			// Equal-ranked duplicates should not preserve the first-inserted
			// branch by accident. Let later survivors replace ties so
			// post-reduce reprocessing can keep the branch that stayed viable.
			if stackCompareMerge(&stack, &result[duplicateIndex]) >= 0 {
				result[duplicateIndex] = stack
				for j := 0; j < slot.count; j++ {
					if slot.indices[j] == duplicateIndex {
						slot.hashes[j] = hash
						break
					}
				}
				if slot.worstIndex == duplicateIndex {
					slot.worstIndex = recomputeMergeLargeSlotWorst(slot, result)
				}
			}
			continue
		}

		if slot.count < perKeyCap {
			idx := len(result)
			result = append(result, stack)
			slot.indices[slot.count] = idx
			slot.hashes[slot.count] = hash
			slot.hashMask |= mergeHashBit(hash)
			slot.count++
			if slot.worstIndex < 0 || stackCompareMerge(&result[idx], &result[slot.worstIndex]) < 0 {
				slot.worstIndex = idx
			}
			continue
		}
		if perfCountersEnabled {
			perfRecordMergePerKeyOverflow()
		}

		// Per-key alternative budget reached: replace the weakest
		// retained candidate only if this stack is better.
		if slot.worstIndex >= 0 {
			replacedSlot := -1
			for j := 0; j < slot.count; j++ {
				if slot.indices[j] == slot.worstIndex {
					replacedSlot = j
					break
				}
			}
			incumbentHash := uint64(0)
			if replacedSlot >= 0 {
				incumbentHash = slot.hashes[replacedSlot]
			}
			if !preferOverflowCandidate(&stack, &result[slot.worstIndex], hash, incumbentHash) {
				continue
			}
			if perfCountersEnabled {
				perfRecordMergeReplacement()
			}
			result[slot.worstIndex] = stack
			if replacedSlot >= 0 {
				slot.hashes[replacedSlot] = hash
				slot.hashMask = recomputeMergeLargeSlotHashMask(slot)
			}
			slot.worstIndex = recomputeMergeLargeSlotWorst(slot, result)
		}
	}
	if perfCountersEnabled {
		perfRecordMergeOut(len(result))
	}
	if scratch.audit != nil {
		scratch.audit.recordMerge(len(alive), len(result), slotCount)
	}
	scratch.result = result
	scratch.largeSlots = slots[:slotCount]
	return result
}

func recomputeMergeSlotWorst(slot *glrMergeSlot, result []glrStack) int {
	if slot == nil || mergeSlotTrackedCount(slot) == 0 {
		return -1
	}
	worst := mergeSlotIndexAt(slot, 0)
	for j, n := 1, mergeSlotTrackedCount(slot); j < n; j++ {
		idx := mergeSlotIndexAt(slot, j)
		if stackCompareMerge(&result[idx], &result[worst]) < 0 {
			worst = idx
		}
	}
	return worst
}

func recomputeMergeLargeSlotWorst(slot *glrMergeLargeSlot, result []glrStack) int {
	if slot == nil || slot.count == 0 {
		return -1
	}
	worst := slot.indices[0]
	for j := 1; j < slot.count; j++ {
		idx := slot.indices[j]
		if stackCompareMerge(&result[idx], &result[worst]) < 0 {
			worst = idx
		}
	}
	return worst
}

func mergeHashBit(hash uint64) uint64 {
	return uint64(1) << (hash & 63)
}

func recomputeMergeSlotHashMask(slot *glrMergeSlot) uint64 {
	if slot == nil || mergeSlotTrackedCount(slot) == 0 {
		return 0
	}
	mask := uint64(0)
	for j, n := 0, mergeSlotTrackedCount(slot); j < n; j++ {
		mask |= mergeHashBit(mergeSlotHashAt(slot, j))
	}
	return mask
}

func recomputeMergeLargeSlotHashMask(slot *glrMergeLargeSlot) uint64 {
	if slot == nil || slot.count == 0 {
		return 0
	}
	mask := uint64(0)
	for j := 0; j < slot.count; j++ {
		mask |= mergeHashBit(slot.hashes[j])
	}
	return mask
}

func ensureMergeResultCap(scratch *glrMergeScratch, n int) []glrStack {
	if cap(scratch.result) < n {
		scratch.result = make([]glrStack, 0, n)
		scratch.resultBytes = glrStackBytesForCap(cap(scratch.result))
	}
	return scratch.result[:0]
}

func ensureMergeSlotCap(scratch *glrMergeScratch, n int) []glrMergeSlot {
	if cap(scratch.slots) < n {
		scratch.slots = make([]glrMergeSlot, n)
		scratch.slotBytes = glrMergeSlotBytesForCap(cap(scratch.slots))
		return scratch.slots
	}
	return scratch.slots[:n]
}

func ensureMergeLargeSlotCap(scratch *glrMergeScratch, n int) []glrMergeLargeSlot {
	if cap(scratch.largeSlots) < n {
		scratch.largeSlots = make([]glrMergeLargeSlot, n)
		scratch.largeSlotBytes = glrMergeLargeSlotBytesForCap(cap(scratch.largeSlots))
		return scratch.largeSlots
	}
	return scratch.largeSlots[:n]
}

func mergeAliveLimitForScratch(scratch *glrMergeScratch, n int) int {
	limit := n
	if limit > maxMergeAliveStacks {
		limit = maxMergeAliveStacks
	}
	if scratch != nil && scratch.budgetBytes > 0 {
		slotSize := unsafe.Sizeof(glrMergeSlot{})
		if scratch.perKeyCap > maxStacksPerMergeKey {
			slotSize = unsafe.Sizeof(glrMergeLargeSlot{})
		}
		perStack := int64(unsafe.Sizeof(glrStack{}) + slotSize)
		if perStack > 0 {
			allowed := int(scratch.budgetBytes / perStack)
			if allowed < 1 {
				allowed = 1
			}
			if allowed < limit {
				limit = allowed
			}
		}
	}
	return limit
}

func (s *glrMergeScratch) allocatedBytes() int64 {
	if s == nil {
		return 0
	}
	return s.resultBytes + s.slotBytes + s.largeSlotBytes + s.equivCacheBytes + s.stackEquivBytes + s.spineEquivBytes + s.frontierHashBytes + s.shapePrefixBytes + s.cleanZeroBytes
}

func (s *glrMergeScratch) reset() {
	if s == nil {
		return
	}
	if cap(s.result) > maxRetainedMergeResultCap {
		s.result = nil
		s.resultBytes = 0
	} else {
		if len(s.result) > 0 {
			clear(s.result)
		}
		s.result = s.result[:0]
		s.resultBytes = glrStackBytesForCap(cap(s.result))
	}
	if cap(s.slots) > maxRetainedMergeSlotCap {
		s.slots = nil
		s.slotBytes = 0
	} else {
		s.slots = s.slots[:0]
		s.slotBytes = glrMergeSlotBytesForCap(cap(s.slots))
	}
	if cap(s.largeSlots) > maxRetainedMergeSlotCap {
		s.largeSlots = nil
		s.largeSlotBytes = 0
	} else {
		s.largeSlots = s.largeSlots[:0]
		s.largeSlotBytes = glrMergeLargeSlotBytesForCap(cap(s.largeSlots))
	}
	s.equivCacheBytes = glrNodeEquivCacheBytesForCap(cap(s.equivCache))
	s.stackEquivBytes = glrStackEquivCacheBytesForCap(cap(s.stackEquivCache))
	s.spineEquivBytes = glrSpineEquivCacheBytesForCap(cap(s.spineEquivCache))
	s.frontierHashBytes = glrStackFrontierHashCacheBytesForCap(cap(s.frontierHashCache))
	s.frontierMergeHash = false
	if len(s.cErrorCost) > maxRetainedMergeResultCap {
		s.cErrorCost = nil
	} else if len(s.cErrorCost) > 0 {
		clear(s.cErrorCost)
	}
	// shapePrefixEpoch is intentionally NOT reset here: like equivEpoch it
	// increases monotonically across parses so retained array entries can never
	// alias into a fresh parse (reset would restart the epoch at 1 and collide
	// with surviving epoch-1 entries once gss slab pointers get reused).
	s.shapePrefixBytes = int64(cap(s.shapePrefixCache)) * int64(unsafe.Sizeof(glrShapePrefixCacheEntry{}))
	// cleanZeroEpoch (below) restarts at 0 per reset, so the epoch-validated
	// front array MUST be cleared or stale entries could match a future epoch.
	if len(s.cleanZeroFront) > 0 {
		clear(s.cleanZeroFront)
	}
	s.cleanZeroBytes = int64(cap(s.cleanZeroFront)) * int64(unsafe.Sizeof(glrCleanZeroFrontCacheEntry{}))
	if len(s.cleanZeroCache) > 0 {
		clear(s.cleanZeroCache)
	}
	if cap(s.cleanZeroStack) > maxRetainedMergeResultCap {
		s.cleanZeroStack = nil
	} else if cap(s.cleanZeroStack) > 0 {
		clear(s.cleanZeroStack[:cap(s.cleanZeroStack)])
		s.cleanZeroStack = s.cleanZeroStack[:0]
	}
	if cap(s.cleanZeroVisited) > maxRetainedMergeResultCap {
		s.cleanZeroVisited = nil
	} else if cap(s.cleanZeroVisited) > 0 {
		clear(s.cleanZeroVisited[:cap(s.cleanZeroVisited)])
		s.cleanZeroVisited = s.cleanZeroVisited[:0]
	}
	s.cleanZeroEpoch = 0
	s.cleanZeroScan = 0
	s.perKeyCap = 0
	s.language = nil
	s.trace = false
	s.cRecoveryCost = false
	s.audit = nil
	s.budgetBytes = 0
}

func glrStackBytesForCap(n int) int64 {
	if n <= 0 {
		return 0
	}
	return int64(n) * int64(unsafe.Sizeof(glrStack{}))
}

func glrMergeSlotBytesForCap(n int) int64 {
	if n <= 0 {
		return 0
	}
	return int64(n) * int64(unsafe.Sizeof(glrMergeSlot{}))
}

func glrMergeLargeSlotBytesForCap(n int) int64 {
	if n <= 0 {
		return 0
	}
	return int64(n) * int64(unsafe.Sizeof(glrMergeLargeSlot{}))
}

func glrNodeEquivCacheBytesForCap(n int) int64 {
	if n <= 0 {
		return 0
	}
	return int64(n) * int64(unsafe.Sizeof(glrNodeEquivCacheEntry{}))
}

func glrStackEquivCacheBytesForCap(n int) int64 {
	if n <= 0 {
		return 0
	}
	return int64(n) * int64(unsafe.Sizeof(glrStackEquivCacheEntry{}))
}

func glrSpineEquivCacheBytesForCap(n int) int64 {
	if n <= 0 {
		return 0
	}
	return int64(n) * int64(unsafe.Sizeof(glrSpineEquivCacheEntry{}))
}

func glrStackFrontierHashCacheBytesForCap(n int) int64 {
	if n <= 0 {
		return 0
	}
	return int64(n) * int64(unsafe.Sizeof(glrStackFrontierHashCacheEntry{}))
}

func (s *glrEntryScratch) alloc(n int) []stackEntry {
	return s.allocWithCap(n, n)
}

func (s *glrEntryScratch) allocWithCap(length, capacity int) []stackEntry {
	if length <= 0 {
		return nil
	}
	if capacity < length {
		capacity = length
	}
	if capacity <= 0 {
		capacity = length
	}

	n := capacity
	if n <= 0 {
		return nil
	}
	if len(s.slabs) == 0 {
		capacity := defaultStackEntrySlabCap
		if n > capacity {
			capacity = n
		}
		s.slabs = append(s.slabs, stackEntrySlab{data: make([]stackEntry, capacity)})
		s.allocatedBytes += stackEntryBytesForCap(capacity)
		s.slabCursor = 0
	}
	if s.slabCursor < 0 || s.slabCursor >= len(s.slabs) {
		s.slabCursor = 0
	}
	for i := s.slabCursor; ; i++ {
		if i >= len(s.slabs) {
			lastCap := defaultStackEntrySlabCap
			if len(s.slabs) > 0 {
				lastCap = len(s.slabs[len(s.slabs)-1].data)
			}
			capacity := lastCap * 2
			if capacity < defaultStackEntrySlabCap {
				capacity = defaultStackEntrySlabCap
			}
			if n > capacity {
				capacity = n
			}
			s.slabs = append(s.slabs, stackEntrySlab{data: make([]stackEntry, capacity)})
			s.allocatedBytes += stackEntryBytesForCap(capacity)
		}
		slab := &s.slabs[i]
		if len(slab.data)-slab.used < n {
			continue
		}
		start := slab.used
		slab.used += n
		s.usedTotal += n
		if s.usedTotal > s.peakUsed {
			s.peakUsed = s.usedTotal
		}
		s.slabCursor = i
		end := start + length
		return slab.data[start : end : start+capacity]
	}
}

func (s *glrEntryScratch) grow(entries []stackEntry, minCap int) []stackEntry {
	newCap := cap(entries) * 2
	if newCap < 1 {
		newCap = 1
	}
	if newCap < minCap {
		newCap = minCap
	}
	out := s.alloc(newCap)
	copy(out, entries)
	return out[:len(entries)]
}

func (s *glrEntryScratch) reset() {
	if len(s.slabs) == 0 {
		s.usedTotal = 0
		s.peakUsed = 0
		s.allocatedBytes = 0
		return
	}

	totalCap := 0
	for i := range s.slabs {
		totalCap += len(s.slabs[i].data)
	}

	if totalCap > maxRetainedStackEntryCap {
		// Keep the newest/largest slabs up to the retention budget.
		keepFrom := len(s.slabs) - 1
		retained := len(s.slabs[keepFrom].data)
		for keepFrom > 0 {
			next := retained + len(s.slabs[keepFrom-1].data)
			if next > maxRetainedStackEntryCap {
				break
			}
			keepFrom--
			retained = next
		}
		if keepFrom > 0 {
			oldLen := len(s.slabs)
			copy(s.slabs, s.slabs[keepFrom:])
			newLen := oldLen - keepFrom
			for i := newLen; i < oldLen; i++ {
				s.slabs[i] = stackEntrySlab{}
			}
			s.slabs = s.slabs[:newLen]
		}
		for i := range s.slabs {
			used := s.slabs[i].used
			if used > len(s.slabs[i].data) {
				used = len(s.slabs[i].data)
			}
			clear(s.slabs[i].data[:used])
			s.slabs[i].used = 0
		}
		s.slabCursor = 0
		s.usedTotal = 0
		s.peakUsed = 0
		s.recomputeAllocatedBytes()
		return
	}

	for i := range s.slabs {
		used := s.slabs[i].used
		if used > len(s.slabs[i].data) {
			used = len(s.slabs[i].data)
		}
		clear(s.slabs[i].data[:used])
		s.slabs[i].used = 0
	}
	s.slabCursor = 0
	s.usedTotal = 0
	s.peakUsed = 0
	s.recomputeAllocatedBytes()
}

func (s *glrEntryScratch) peakEntriesUsed() int {
	if s == nil {
		return 0
	}
	return s.peakUsed
}

func stackEntryBytesForCap(n int) int64 {
	if n <= 0 {
		return 0
	}
	return int64(n) * int64(unsafe.Sizeof(stackEntry{}))
}

func (s *glrEntryScratch) recomputeAllocatedBytes() {
	if s == nil {
		return
	}
	var total int64
	for i := range s.slabs {
		total += stackEntryBytesForCap(len(s.slabs[i].data))
	}
	s.allocatedBytes = total
}
