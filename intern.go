package gotreesitter

import (
	"os"
	"unsafe"
)

// internLeavesObserveEnabled gates the Phase 2 leaf-interning observation
// path. Read once at package init from GOT_PARSE_INTERN_LEAVES_OBSERVE=1
// so the per-leaf branch is free in the default case. When true,
// newLeafNodeInArena populates an arena-local internTable for hit-rate
// measurement BUT does not actually short-circuit allocation. This lets
// us learn the potential hit rate before committing to the behavior
// change in a future phase.
var internLeavesObserveEnabled = os.Getenv("GOT_PARSE_INTERN_LEAVES_OBSERVE") == "1"

// SetInternLeavesObserveEnabled toggles leaf-interning observation at
// runtime. Tests and benches that want to A/B observation without
// re-running the test binary set this directly. Not safe to flip while
// a parse is in flight on another goroutine. Phase 2 scaffolding; the
// API may change before becoming public.
func SetInternLeavesObserveEnabled(on bool) {
	internLeavesObserveEnabled = on
}

// internLeavesSubstituteEnabled is the Phase 3 behavior gate. When true,
// the shift path looks up the new leaf in the per-arena intern table
// after parseState is set; on hit, the canonical leaf is pushed onto
// the stack instead of the newly-allocated one (the new one stays in
// the arena slab but is unreferenced). Implies observation is on.
var internLeavesSubstituteEnabled = os.Getenv("GOT_PARSE_INTERN_LEAVES_SUBSTITUTE") == "1"

// SetInternLeavesSubstituteEnabled toggles canonical substitution at
// runtime. See internLeavesSubstituteEnabled.
func SetInternLeavesSubstituteEnabled(on bool) {
	internLeavesSubstituteEnabled = on
	if on {
		internLeavesObserveEnabled = true
	}
}

// InternStatsFor returns a snapshot of the leaf-interning observation
// counters for the arena that owns the given root node. Returns the
// zero value if observation is disabled or the root is not arena-backed.
// Exposed so external benches can read hit rates without grepping
// internal logs.
func InternStatsFor(root *Node) InternObservationStats {
	if root == nil || root.ownerArena == nil {
		return InternObservationStats{}
	}
	arena := root.ownerArena
	out := InternObservationStats{
		ShiftLeafObserved: arena.internShiftLeafObserved,
	}
	if arena.internLeaves != nil {
		s := arena.internLeaves.stats()
		out.LeafLookups = s.Lookups
		out.LeafHits = s.Hits
		out.LeafMisses = s.Misses
		out.LeafStores = s.Stores
		out.LeafGrowths = s.Growths
	}
	if arena.internLeavesFull != nil {
		s := arena.internLeavesFull.stats()
		out.FullLookups = s.Lookups
		out.FullHits = s.Hits
		out.FullMisses = s.Misses
	}
	return out
}

// InternObservationStats is the externally-visible snapshot of
// leaf-interning observation counters for a single parse. Returned
// from InternStatsFor.
type InternObservationStats struct {
	// Phase 2 counters (parseState-blind observation across ALL leaves).
	LeafLookups uint64
	LeafHits    uint64
	LeafMisses  uint64
	LeafStores  uint64
	LeafGrowths uint64
	// Phase 3 attribution. Shift-path leaves get parseState set per-fork
	// so they can't be canonically substituted via the parseState-blind
	// measurement; non-shift leaves can. "Safe to substitute" via blind
	// measurement = (LeafMisses+LeafHits) - ShiftLeafObserved.
	ShiftLeafObserved uint64
	// Phase 3 parseState-aware measurement. Same hook as LeafLookups
	// but with parseState/preGotoState included in the key. A hit here
	// means a truly dedup-safe duplicate; the difference between this
	// and the blind hit rate quantifies how much of the blind
	// observation was an artifact of ignoring state.
	FullLookups uint64
	FullHits    uint64
	FullMisses  uint64
}

// Phase 1 scaffolding for the GLR node interning initiative
// (initiative.glr-node-interning in hyphae://m31labs/gotreesitter).
//
// Scope of this file:
//   - InternTable type + reset/teardown lifecycle hooks
//   - Hash function for (symbol, child pointers, span, flags, productionID)
//   - Lookup/store API, designed for the reduce path but NOT YET wired in
//   - Counters for hit/miss/store so future phases have a baseline
//
// What this is NOT (yet):
//   - Not integrated into the reduce path. Calling parser.Parse does not
//     populate or query this table. Phase 2 will wire leaf interning;
//     Phase 3 extends to parents; Phase 4 plugs into glr_merge.
//   - Not a replacement for the 2-way set-associative equiv cache in glr.go.
//     Those primitives stay in place until Phase 4 demonstrates the intern
//     table subsumes them.
//
// Concurrency:
//   - One table per parser instance. Not safe for concurrent Parse() calls
//     on the same Parser. (The runtime already isolates parsers via
//     ParserPool for concurrent use.)
//
// Lifecycle:
//   - Phase 1 invariant: the table is created at parse start and dropped
//     before post-parse normalizers run. This sidesteps the mutation
//     barrier — normalizers mutate Node.children freely; the intern table
//     is gone by then.

// internKey is the lookup key for a single node shape. Keep the struct
// tight: every reduce traverses this struct hot.
type internKey struct {
	// symbol identifies the node's grammar symbol. uint16 in the runtime;
	// widened to uint32 here so the struct lays out cleanly without
	// padding holes between symbol and the pointer-equivalent fields.
	symbol uint32
	// productionID disambiguates two reductions for the same symbol
	// that produced different shapes (e.g. different rule alternatives).
	productionID uint16
	// flags captures isNamed/isExtra/hasError/isMissing in the same byte
	// layout as Node.flags. Two nodes with the same shape but different
	// flags MUST hash to different buckets.
	flags uint8
	// childCount is duplicated from len(childrenHash) to allow rejection
	// without indexing the slice.
	childCount uint8
	// startByte and endByte pin the source span. Identical shapes at
	// different file positions are not interchangeable — consumers use
	// startByte for position queries.
	startByte uint32
	endByte   uint32
	// parseState and preGotoState capture the per-GLR-stack state the
	// node was created in. Without these, two leaves from different
	// forks would erroneously dedup even though consumers (e.g. the
	// incremental-leaf fastpath) read these fields. Phase 3 promoted
	// them from "tracked separately" to "part of the key" after Phase 2
	// observation showed shift-path dominance.
	parseState   StateID
	preGotoState StateID
	// childrenHash is a Bob Jenkins-style mix of the child pointer
	// values. The pointers themselves live in the table's separate
	// children-pointer arena; we compare them on hash collision.
	childrenHash uint64
}

// internEntry holds one intern table entry. The pointer comparison on
// lookup is cheap; the canonical *Node lives in the parser's main arena
// so this struct does not need to own it.
type internEntry struct {
	key  internKey
	node *Node
}

// internTable is the per-parse intern table. Phase 1 uses a flat
// open-addressed hash table; the table is cleared (length zeroed, slots
// not freed) at parse start and discarded post-parse.
type internTable struct {
	// entries is the open-addressed bucket array. Capacity is power-of-2;
	// occupancy is bounded by maxLoadFactor before growing.
	entries []internEntry
	// occupied is the count of non-empty slots, used to trigger growth.
	occupied int
	// lookups/hits/misses/stores are observability counters. Future
	// phases will wire these into runtimeAudit; Phase 1 keeps them
	// local so the file builds standalone.
	lookups uint64
	hits    uint64
	misses  uint64
	stores  uint64
	// growths counts table resizes — a non-zero value during Phase 2
	// real-corpus runs is a signal to bump initial capacity.
	growths uint64
}

// internTableInitialCap is the starting bucket count. 4096 is enough
// for the JS bench (~700k nodes across 3 files, hit rate target 30%+
// means ~200k canonical shapes; one parse iteration covers ~140k of
// those, fits well above 50% load in a 4K table after one growth).
// Tuned with Phase 2 measurements.
const internTableInitialCap = 4096

// internTableMaxLoadFactor governs when to grow. 0.7 trades memory for
// lookup speed; tighter than the 0.75 default to keep probe sequences
// short on a hot inner loop.
const internTableMaxLoadFactor = 0.7

// newInternTable allocates a fresh table with the initial capacity.
// Caller is responsible for calling reset() between parses.
func newInternTable() *internTable {
	return &internTable{
		entries: make([]internEntry, internTableInitialCap),
	}
}

// reset clears entries for reuse on the next parse. Counters are kept;
// callers can read or zero them as needed.
func (t *internTable) reset() {
	if t == nil {
		return
	}
	for i := range t.entries {
		t.entries[i] = internEntry{}
	}
	t.occupied = 0
}

// hashKey returns a 64-bit hash of the key. The mixing constants are
// xxHash-style; this is not cryptographic — only collision quality
// matters. Stable across runs (no per-process salt yet; revisit if a
// future spore proposes salting to harden against adversarial input).
func hashKey(k internKey) uint64 {
	const (
		prime1 = 0x9e3779b185ebca87
		prime2 = 0xc2b2ae3d27d4eb4f
		prime3 = 0x165667b19e3779f9
	)
	h := uint64(k.symbol) * prime1
	h ^= uint64(k.productionID)<<16 | uint64(k.flags)<<8 | uint64(k.childCount)
	h *= prime2
	h ^= uint64(k.startByte)<<32 | uint64(k.endByte)
	h *= prime3
	h ^= uint64(k.parseState)<<32 | uint64(k.preGotoState)
	h *= prime1
	h ^= k.childrenHash
	// Final avalanche.
	h ^= h >> 33
	h *= prime2
	h ^= h >> 29
	h *= prime3
	h ^= h >> 32
	return h
}

// hashChildren combines a child pointer slice into the childrenHash
// component of internKey. Position-sensitive: (a, b) and (b, a) hash
// differently. Uses pointer addresses, so cross-parse hashes are NOT
// stable (intentional — the intern table is per-parse).
func hashChildren(children []*Node) uint64 {
	if len(children) == 0 {
		return 0
	}
	var h uint64 = 0xcbf29ce484222325
	for _, c := range children {
		p := uintptr(unsafe.Pointer(c))
		h ^= uint64(p)
		h *= 0x100000001b3
	}
	return h
}

// buildKey constructs an internKey from a node's identifying fields.
// Helper for callers; safe to inline at hot sites if profile demands.
func buildKey(symbol Symbol, productionID uint16, flags nodeFlags, startByte, endByte uint32, children []*Node) internKey {
	return internKey{
		symbol:       uint32(symbol),
		productionID: productionID,
		flags:        uint8(flags),
		childCount:   uint8(len(children)),
		startByte:    startByte,
		endByte:      endByte,
		childrenHash: hashChildren(children),
	}
}

// buildKeyFromNode extracts the full key from an in-place node, including
// per-stack state. Used by the post-shift observation hook where the
// caller has already set parseState/preGotoState; lookup against this
// key tells us how many leaves are duplicates AT THE STATE LEVEL — the
// signal that matters for canonical substitution.
func buildKeyFromNode(n *Node) internKey {
	return internKey{
		symbol:       uint32(n.symbol),
		productionID: n.productionID,
		flags:        uint8(n.flags),
		childCount:   uint8(len(n.children)),
		startByte:    n.startByte,
		endByte:      n.endByte,
		parseState:   n.parseState,
		preGotoState: n.preGotoState,
		childrenHash: hashChildren(n.children),
	}
}

// observeLeafInternFull is the post-state observation hook called by the
// shift path AFTER parseState/preGotoState are set. Distinct from the
// observeLeafIntern helper called by newLeafNodeInArena (which lacks
// state info). The two co-exist so we can compare parseState-blind vs
// parseState-aware hit rates and quantify how many "duplicates" are
// only artifacts of the blind measurement.
func observeLeafInternFull(arena *nodeArena, n *Node) {
	if arena.internLeavesFull == nil {
		arena.internLeavesFull = newInternTable()
	}
	key := buildKeyFromNode(n)
	if hit := arena.internLeavesFull.lookup(key, n.children); hit == nil {
		arena.internLeavesFull.store(key, n)
	}
}

// lookupCanonicalLeafKey is the pre-allocation lookup used by the shift
// loop. The caller has computed the full intern key from primitives
// (token, act, state) without allocating a Node. On hit, the canonical
// leaf is returned and the caller can skip newLeafNodeInArena entirely.
// On miss, returns nil; the caller allocates and calls storeCanonicalLeaf
// afterward.
func lookupCanonicalLeafKey(arena *nodeArena, key internKey) *Node {
	if arena.internLeavesFull == nil {
		arena.internLeavesFull = newInternTable()
		return nil
	}
	// Pre-allocation lookup has no children slice to dedup against; the
	// table is leaves-only, so any hit at this key is a true match
	// without further collision verification beyond the key equality
	// the table already enforces.
	if hit := arena.internLeavesFull.lookup(key, nil); hit != nil {
		// Update hit/lookup counters in the lookup() call. arena.audit
		// integration happens in Phase 4 if/when the runtime audit
		// surface gets intern fields.
		return hit
	}
	return nil
}

// storeCanonicalLeaf is the pre-allocation companion to
// lookupCanonicalLeafKey. After the shift loop has allocated and fully
// configured a leaf following a lookup miss, it stores the leaf as
// the canonical entry for its key so subsequent shifts with the same
// shape collapse to this pointer.
func storeCanonicalLeaf(arena *nodeArena, leaf *Node) {
	if arena.internLeavesFull == nil {
		arena.internLeavesFull = newInternTable()
	}
	arena.internLeavesFull.store(buildKeyFromNode(leaf), leaf)
}

// lookup returns the canonical node for the given key, or nil if absent.
// Caller must also pass the child slice for collision verification (two
// distinct child slices could in principle hash equal).
func (t *internTable) lookup(key internKey, children []*Node) *Node {
	if t == nil || len(t.entries) == 0 {
		return nil
	}
	t.lookups++
	mask := uint64(len(t.entries) - 1)
	h := hashKey(key)
	for probe := uint64(0); probe < uint64(len(t.entries)); probe++ {
		idx := (h + probe) & mask
		entry := &t.entries[idx]
		if entry.node == nil {
			t.misses++
			return nil
		}
		if entry.key == key && childrenSliceEq(entry.node, children) {
			t.hits++
			return entry.node
		}
	}
	t.misses++
	return nil
}

// store inserts node under the given key. If a slot is occupied with a
// matching key, the existing node is preserved (first-wins) — callers
// should look up before allocating.
func (t *internTable) store(key internKey, node *Node) {
	if t == nil || node == nil {
		return
	}
	if float64(t.occupied+1)/float64(len(t.entries)) > internTableMaxLoadFactor {
		t.grow()
	}
	mask := uint64(len(t.entries) - 1)
	h := hashKey(key)
	for probe := uint64(0); probe < uint64(len(t.entries)); probe++ {
		idx := (h + probe) & mask
		entry := &t.entries[idx]
		if entry.node == nil {
			entry.key = key
			entry.node = node
			t.occupied++
			t.stores++
			return
		}
		if entry.key == key {
			// First-wins. Phase 2 may revisit to assert the equal-key
			// node is also pointer-equal to the candidate; for now we
			// silently dedup.
			return
		}
	}
}

// grow doubles the table capacity and re-inserts existing entries.
// Called from store() when the load factor exceeds the bound.
func (t *internTable) grow() {
	oldEntries := t.entries
	t.entries = make([]internEntry, len(oldEntries)*2)
	t.occupied = 0
	t.growths++
	for _, e := range oldEntries {
		if e.node == nil {
			continue
		}
		// Re-insert without going through store() (which would re-check
		// the load factor; we already grew). Open-addressed probe.
		mask := uint64(len(t.entries) - 1)
		h := hashKey(e.key)
		for probe := uint64(0); probe < uint64(len(t.entries)); probe++ {
			idx := (h + probe) & mask
			if t.entries[idx].node == nil {
				t.entries[idx] = e
				t.occupied++
				break
			}
		}
	}
}

// childrenSliceEq verifies that node n has exactly the same child
// pointers as the candidate slice. Used to resolve hash collisions on
// lookup. Compares pointer identity, not deep equality — that's the
// whole point of interning.
func childrenSliceEq(n *Node, candidate []*Node) bool {
	if n == nil {
		return len(candidate) == 0
	}
	if len(n.children) != len(candidate) {
		return false
	}
	for i, c := range candidate {
		if n.children[i] != c {
			return false
		}
	}
	return true
}

// internStats is the observability snapshot exported for tests and
// future runtime_audit integration.
type internStats struct {
	Lookups  uint64
	Hits     uint64
	Misses   uint64
	Stores   uint64
	Growths  uint64
	Occupied int
	Capacity int
}

// stats returns the current counters. Safe to call between parses.
func (t *internTable) stats() internStats {
	if t == nil {
		return internStats{}
	}
	return internStats{
		Lookups:  t.lookups,
		Hits:     t.hits,
		Misses:   t.misses,
		Stores:   t.stores,
		Growths:  t.growths,
		Occupied: t.occupied,
		Capacity: len(t.entries),
	}
}
