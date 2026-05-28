package gotreesitter

import "unsafe"

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
	result           []glrStack
	slots            []glrMergeSlot
	largeSlots       []glrMergeLargeSlot
	perKeyCap        int
	language         *Language
	deferExactDedupe bool
	audit            *runtimeAudit
	equivEpoch       uint32
	equivCache       []glrNodeEquivCacheEntry
	pythonShallow    bool
	budgetBytes      int64
	resultBytes      int64
	slotBytes        int64
	largeSlotBytes   int64
	equivCacheBytes  int64
}

type glrMergeKey struct {
	state      StateID
	byteOffset uint32
}

type glrMergeSlot struct {
	key        glrMergeKey
	indices    [maxStacksPerMergeKey]int
	hashes     [maxStacksPerMergeKey]uint64
	hashMask   uint64
	count      int
	worstIndex int
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
			entries:      entries,
			cacheEntries: s.cacheEntries,
			byteOffset:   s.byteOffset,
			score:        s.score,
			branchOrder:  s.branchOrder,
		}
	}
	s.ensureGSS(nil)
	return glrStack{
		gss:          s.gss.clone(),
		cacheEntries: s.cacheEntries,
		byteOffset:   s.byteOffset,
		score:        s.score,
		branchOrder:  s.branchOrder,
	}
}

func (s *glrStack) cloneWithScratch(scratch *gssScratch) glrStack {
	s.ensureGSS(scratch)
	return glrStack{
		gss:          s.gss.clone(),
		cacheEntries: false,
		byteOffset:   s.byteOffset,
		score:        s.score,
		branchOrder:  s.branchOrder,
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
	if s.equivEpoch == ^uint32(0) {
		clear(s.equivCache)
		s.equivEpoch = 0
	}
	s.equivEpoch++
	if len(s.equivCache) == 0 {
		s.equivCache = make([]glrNodeEquivCacheEntry, glrNodeEquivCacheSize)
		s.equivCacheBytes = glrNodeEquivCacheBytesForCap(cap(s.equivCache))
	}
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
	audit := activeEquivAudit(scratch)
	if audit == nil {
		for an, bn := a.head, b.head; an != nil && bn != nil; an, bn = an.prev, bn.prev {
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
	for an, bn, depthFromTop := a.head, b.head, 0; an != nil && bn != nil; an, bn, depthFromTop = an.prev, bn.prev, depthFromTop+1 {
		if an == bn {
			return true
		}
		audit.recordStackEquivEntryCompare()
		if an.entry.state != bn.entry.state {
			audit.recordStackEquivStateMismatchAt(depthFromTop)
			return false
		}
		if !stackEntryPayloadsEquivalentForLanguageWithScratch(scratch, lang, an.entry, bn.entry) {
			audit.recordStackEquivPayloadMismatchAt(depthFromTop)
			audit.recordStackEquivPayloadMismatchSignatures(an.entry, bn.entry)
			return false
		}
	}
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
			audit.mergeHeaderEqTotal++
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

// recordMergeHeaderDivergenceForAudit tallies the relationship between
// header-only equivalence and deep equivalence for a single merge-candidate
// pair. The interesting bucket is "header-only would accept, deep walk
// rejects" (mergeHeaderDeepDivergent) — that's how many merges would change
// behavior if we switched to header-only.
func recordMergeHeaderDivergenceForAudit(audit *runtimeAudit, headerEq, deepEq bool) {
	if audit == nil {
		return
	}
	if deepEq {
		audit.mergeDeepTrue++
	} else {
		audit.mergeDeepFalse++
		if headerEq {
			audit.mergeHeaderDeepDivergent++
		}
	}
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
		stackEntryNodeProductionID(a) != stackEntryNodeProductionID(b) {
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
		a.productionID != b.productionID {
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
		a.productionID != b.productionID {
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
		a.productionID != b.productionID {
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
		a.productionID != b.productionID {
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
		for i := range a.fieldIDs {
			if a.fieldIDs[i] == 0 {
				continue
			}
			child := a.children[i]
			if child == nil || child.flags&nodeFlagExtra != 0 || (child.flags&nodeFlagNamed == 0 && len(child.children) == 0) {
				continue
			}
			addCandidate(i)
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
	if aErr, bErr := stackErrorRank(a), stackErrorRank(b); aErr != bErr {
		if aErr < bErr {
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
	if aErr, bErr := stackErrorRank(a), stackErrorRank(b); aErr != bErr {
		if aErr < bErr {
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
	if aErr, bErr := stackErrorRank(a), stackErrorRank(b); aErr != bErr {
		if aErr < bErr {
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

func stackErrorRank(s *glrStack) int {
	if s == nil {
		return 2
	}
	top := s.top()
	if !stackEntryHasNode(top) {
		return 0
	}
	if stackEntryNodeHasError(top) {
		return 1
	}
	return 0
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
		for j := range result {
			if mergeKeyForStack(result[j]) != key {
				continue
			}
			if scratch != nil && scratch.perKeyCap == 1 {
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
				duplicateIndex = j
				break
			}
		}
		if duplicateIndex < 0 {
			result = append(result, stack)
			continue
		}
		if stackCompareMerge(&stack, &result[duplicateIndex]) >= 0 {
			result[duplicateIndex] = stack
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
	for i := range alive {
		stack := alive[i]
		key := mergeKeyForStack(stack)
		duplicateIndex := -1
		sameKeyCount := 0
		for j := range result {
			if mergeKeyForStack(result[j]) != key {
				continue
			}
			sameKeyCount++
			if scratch != nil && scratch.perKeyCap == 1 {
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
				duplicateIndex = j
				break
			}
		}
		if duplicateIndex < 0 {
			result = append(result, stack)
			continue
		}
		if stackCompareMerge(&stack, &result[duplicateIndex]) >= 0 {
			result[duplicateIndex] = stack
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

	// Remove dead stacks first.
	alive := stacks[:0]
	deadCount := 0
	for i := range stacks {
		if !stacks[i].dead {
			alive = append(alive, stacks[i])
		} else {
			deadCount++
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
		hash := stackHash(stack)
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

		if perKeyCap == 1 && slot.count == 1 {
			idx := slot.indices[0]
			cmp := stackCompareMerge(&stack, &result[idx])
			if cmp > 0 {
				result[idx] = stack
				slot.hashes[0] = hash
				slot.hashMask = mergeHashBit(hash)
				slot.worstIndex = idx
				if perfCountersEnabled {
					perfRecordMergeReplacement()
				}
				continue
			}
			if cmp < 0 {
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
		hash := stackHash(stack)
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

		if perKeyCap == 1 && slot.count == 1 {
			idx := slot.indices[0]
			cmp := stackCompareMerge(&stack, &result[idx])
			if cmp > 0 {
				result[idx] = stack
				slot.hashes[0] = hash
				slot.hashMask = mergeHashBit(hash)
				slot.worstIndex = idx
				if perfCountersEnabled {
					perfRecordMergeReplacement()
				}
				continue
			}
			if cmp < 0 {
				continue
			}
		}

		duplicateIndex := -1
		hashMatched := false
		if slot.count >= perKeyCap && (slot.hashMask&mergeHashBit(hash)) != 0 {
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
		if !hashMatched && slot.count >= perKeyCap && perfCountersEnabled {
			perfRecordStackEquivalentHashMissSkip()
		}
		if duplicateIndex >= 0 {
			if stackCompareMerge(&stack, &result[duplicateIndex]) >= 0 {
				result[duplicateIndex] = stack
				for j := 0; j < slot.count; j++ {
					if slot.indices[j] == duplicateIndex {
						slot.hashes[j] = hash
						break
					}
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
		hash := stackHash(stack)
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
	if slot == nil || slot.count == 0 {
		return 0
	}
	mask := uint64(0)
	for j := 0; j < slot.count; j++ {
		mask |= mergeHashBit(slot.hashes[j])
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
	return s.resultBytes + s.slotBytes + s.largeSlotBytes + s.equivCacheBytes
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
	s.perKeyCap = 0
	s.language = nil
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
