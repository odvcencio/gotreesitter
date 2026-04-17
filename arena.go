package gotreesitter

import (
	"sync"
	"sync/atomic"
	"unsafe"
)

const (
	// incrementalArenaSlab is sized for steady-state edits where only a small
	// frontier of nodes is rebuilt.
	incrementalArenaSlab = 16 * 1024
	// fullParseArenaSlab matches the current full-parse node footprint with
	// headroom, while remaining small enough to keep a warm pool.
	fullParseArenaSlab = 2 * 1024 * 1024
	minArenaNodeCap    = 64

	// Default capacities for slice backing storage used by reduce actions.
	// Full parses allocate many more parent-child edges than incremental edits.
	incrementalChildSliceCap = 2 * 1024
	fullChildSliceCap        = 64 * 1024
	incrementalFieldSliceCap = 2 * 1024
	fullFieldSliceCap        = 64 * 1024

	maxRetainedArenaFactor = 4
	// Full-parse node slabs are much larger; keep more headroom so capacity
	// growth does not thrash between parses.
	maxRetainedFullNodeArenaFactor  = 16
	maxRetainedFullSliceArenaFactor = 16

	// Absolute node-cap retention ceilings to avoid repeated large reallocation
	// on warm edit/full-parse workloads.
	maxRetainedIncrementalNodeCap  = 1 * 1024 * 1024
	maxRetainedFullNodeCap         = 2 * 1024 * 1024
	maxRetainedIncrementalSliceCap = 32 * 1024
	maxRetainedFullSliceCap        = 1 * 1024 * 1024
)

type arenaClass uint8

const (
	arenaClassIncremental arenaClass = iota
	arenaClassFull
)

// nodeArena is a slab-backed allocator for Node structs.
// It uses ref counting so trees that borrow reused subtrees can keep arena
// memory alive safely until all dependent trees are released.
type nodeArena struct {
	class arenaClass
	nodes []Node
	used  int
	refs  atomic.Int32
	// budgetBytes is a soft per-parse cap for retained arena backing storage.
	// A value of 0 disables budget checks.
	budgetBytes    int64
	allocatedBytes int64
	// skipChildClear allows reset() to skip child-slab pointer clearing when
	// a parse did not borrow any external nodes (full parse without reuse).
	skipChildClear bool
	audit          *runtimeAudit

	nodeSlabs      []nodeSlab
	nodeSlabCursor int

	childSlabs                         []childSliceSlab
	fieldSlabs                         []fieldSliceSlab
	fieldSourceSlabs                   []fieldSourceSliceSlab
	externalScannerNodeCheckpoints     []externalScannerCheckpointRef
	externalScannerNodeCheckpointSlabs []externalScannerCheckpointSlab
	childSlabCursor                    int
	fieldSlabCursor                    int
	fieldSourceSlabCursor              int
}

type nodeSlab struct {
	data []Node
	used int
}

type childSliceSlab struct {
	data []*Node
	used int
}

type fieldSliceSlab struct {
	data []FieldID
	used int
}

type fieldSourceSliceSlab struct {
	data []uint8
	used int
}

type externalScannerCheckpointSlab struct {
	data []externalScannerCheckpointRef
}

var (
	incrementalArenaPool = nodeArenaPool{
		class:   arenaClassIncremental,
		maxSize: 8,
	}
	fullArenaPool = nodeArenaPool{
		class:   arenaClassFull,
		maxSize: 4,
	}
)

type nodeArenaPool struct {
	mu      sync.Mutex
	class   arenaClass
	maxSize int
	free    []*nodeArena
}

// ArenaProfile captures node arena allocation statistics.
// Enable with SetArenaProfileEnabled(true) and retrieve with GetArenaProfile().
type ArenaProfile struct {
	IncrementalAcquire uint64
	IncrementalNew     uint64
	FullAcquire        uint64
	FullNew            uint64
}

var (
	arenaProfileEnabled bool
	arenaProfileData    ArenaProfile
)

// EnableArenaProfile toggles arena pool counters.
// This debug hook is not concurrency-safe and is intended for single-threaded
// benchmark/profiling runs.
func EnableArenaProfile(enabled bool) {
	arenaProfileEnabled = enabled
}

// ResetArenaProfile resets arena pool counters.
// This debug hook is not concurrency-safe and is intended for single-threaded
// benchmark/profiling runs.
func ResetArenaProfile() {
	arenaProfileData = ArenaProfile{}
}

// ArenaProfileSnapshot returns current arena pool counters.
// This debug hook is not concurrency-safe and is intended for single-threaded
// benchmark/profiling runs.
func ArenaProfileSnapshot() ArenaProfile {
	return arenaProfileData
}

func (p *nodeArenaPool) acquire() *nodeArena {
	p.mu.Lock()
	n := len(p.free)
	if n == 0 {
		p.mu.Unlock()
		a := newNodeArena(p.class)
		if arenaProfileEnabled {
			switch p.class {
			case arenaClassIncremental:
				arenaProfileData.IncrementalAcquire++
				arenaProfileData.IncrementalNew++
			default:
				arenaProfileData.FullAcquire++
				arenaProfileData.FullNew++
			}
		}
		return a
	}
	a := p.free[n-1]
	p.free = p.free[:n-1]
	p.mu.Unlock()
	if arenaProfileEnabled {
		switch p.class {
		case arenaClassIncremental:
			arenaProfileData.IncrementalAcquire++
		default:
			arenaProfileData.FullAcquire++
		}
	}
	return a
}

func (p *nodeArenaPool) release(a *nodeArena) {
	if a == nil {
		return
	}
	p.mu.Lock()
	if len(p.free) < p.maxSize {
		p.free = append(p.free, a)
	}
	p.mu.Unlock()
}

func (p *nodeArenaPool) drain() {
	p.mu.Lock()
	clear(p.free[:cap(p.free)]) // nil all pointers so GC can collect the arenas
	p.free = p.free[:0]
	p.mu.Unlock()
}

// DrainArenaPools releases all cached arenas from both incremental and full-parse
// pools. Arenas held in the pool are strong Go references and are not collected
// by the GC until explicitly drained or the process exits.
//
// Call this after a large batch scan (e.g. after WalkAndParse returns) to allow
// the GC to reclaim the arena memory. The next parse will allocate a fresh arena.
func DrainArenaPools() {
	incrementalArenaPool.drain()
	fullArenaPool.drain()
}

func nodeCapacityForBytes(slabBytes int) int {
	nodeSize := int(unsafe.Sizeof(Node{}))
	if nodeSize <= 0 {
		return minArenaNodeCap
	}
	capacity := slabBytes / nodeSize
	if capacity < minArenaNodeCap {
		return minArenaNodeCap
	}
	return capacity
}

func newNodeArena(class arenaClass) *nodeArena {
	childCap := fullChildSliceCap
	fieldCap := fullFieldSliceCap
	fieldSourceCap := fullFieldSliceCap
	if class == arenaClassIncremental {
		childCap = incrementalChildSliceCap
		fieldCap = incrementalFieldSliceCap
		fieldSourceCap = incrementalFieldSliceCap
	}
	a := &nodeArena{
		class:            class,
		nodes:            make([]Node, nodeCapacityForClass(class)),
		childSlabs:       []childSliceSlab{{data: make([]*Node, childCap)}},
		fieldSlabs:       []fieldSliceSlab{{data: make([]FieldID, fieldCap)}},
		fieldSourceSlabs: []fieldSourceSliceSlab{{data: make([]uint8, fieldSourceCap)}},
	}
	a.recomputeAllocatedBytes()
	return a
}

func acquireNodeArena(class arenaClass) *nodeArena {
	var a *nodeArena
	switch class {
	case arenaClassIncremental:
		a = incrementalArenaPool.acquire()
	default:
		a = fullArenaPool.acquire()
	}
	a.refs.Store(1)
	a.clearBudget()
	a.audit = nil
	return a
}

func (a *nodeArena) Retain() {
	if a == nil {
		return
	}
	a.refs.Add(1)
}

func (a *nodeArena) Release() {
	if a == nil {
		return
	}
	if a.refs.Add(-1) != 0 {
		return
	}
	a.reset()
	switch a.class {
	case arenaClassIncremental:
		incrementalArenaPool.release(a)
	default:
		fullArenaPool.release(a)
	}
}

func (a *nodeArena) reset() {
	// Clear the full backing arrays, not just [:used], so Go's GC can collect
	// Node structs that contain pointer fields (children, parent, etc.).
	// Partial clear leaves stale *Node pointers in the unused tail which the
	// GC traces, preventing arena memory from being reclaimed.
	// clear() is a no-op on nil/empty slices, no guard needed.
	clear(a.nodes)
	a.used = 0
	// externalScannerCheckpointRef contains only integer fields (no pointers),
	// so clearing it is not required for GC correctness. We clear it anyway for
	// consistency and to avoid leaking stale slab+offset values.
	clear(a.externalScannerNodeCheckpoints)
	for i := range a.externalScannerNodeCheckpointSlabs {
		clear(a.externalScannerNodeCheckpointSlabs[i].data)
	}
	for i := range a.nodeSlabs {
		slab := &a.nodeSlabs[i]
		clear(slab.data)
		slab.used = 0
	}
	if len(a.nodeSlabs) > 0 {
		retained := 0
		keep := 0
		limit := maxRetainedOverflowNodeCapacityForClass(a.class)
		for i := 0; i < len(a.nodeSlabs); i++ {
			capacity := len(a.nodeSlabs[i].data)
			if capacity <= 0 {
				break
			}
			if retained+capacity > limit {
				break
			}
			retained += capacity
			keep = i + 1
		}
		for i := keep; i < len(a.nodeSlabs); i++ {
			a.nodeSlabs[i] = nodeSlab{}
		}
		a.nodeSlabs = a.nodeSlabs[:keep]
		if len(a.externalScannerNodeCheckpointSlabs) > keep {
			for i := keep; i < len(a.externalScannerNodeCheckpointSlabs); i++ {
				a.externalScannerNodeCheckpointSlabs[i] = externalScannerCheckpointSlab{}
			}
			a.externalScannerNodeCheckpointSlabs = a.externalScannerNodeCheckpointSlabs[:keep]
		}
	}
	a.nodeSlabCursor = 0

	if len(a.childSlabs) > 0 {
		retained := 0
		keep := 0
		limit := maxRetainedChildSliceCapacityForClass(a.class)
		for i := 0; i < len(a.childSlabs); i++ {
			capacity := len(a.childSlabs[i].data)
			if capacity <= 0 {
				break
			}
			if keep > 0 && retained+capacity > limit {
				break
			}
			retained += capacity
			keep = i + 1
		}
		if keep == 0 {
			keep = 1
		}
		for i := keep; i < len(a.childSlabs); i++ {
			a.childSlabs[i] = childSliceSlab{}
		}
		a.childSlabs = a.childSlabs[:keep]
	}
	for i := range a.childSlabs {
		slab := &a.childSlabs[i]
		if !a.skipChildClear {
			clear(slab.data[:slab.used])
		}
		slab.used = 0
	}
	a.skipChildClear = false
	a.audit = nil
	if len(a.fieldSlabs) > 0 {
		retained := 0
		keep := 0
		limit := maxRetainedFieldSliceCapacityForClass(a.class)
		for i := 0; i < len(a.fieldSlabs); i++ {
			capacity := len(a.fieldSlabs[i].data)
			if capacity <= 0 {
				break
			}
			if keep > 0 && retained+capacity > limit {
				break
			}
			retained += capacity
			keep = i + 1
		}
		if keep == 0 {
			keep = 1
		}
		for i := keep; i < len(a.fieldSlabs); i++ {
			a.fieldSlabs[i] = fieldSliceSlab{}
		}
		a.fieldSlabs = a.fieldSlabs[:keep]
	}
	for i := range a.fieldSlabs {
		a.fieldSlabs[i].used = 0
	}
	if len(a.fieldSourceSlabs) > 0 {
		retained := 0
		keep := 0
		limit := maxRetainedFieldSourceSliceCapacityForClass(a.class)
		for i := 0; i < len(a.fieldSourceSlabs); i++ {
			capacity := len(a.fieldSourceSlabs[i].data)
			if capacity <= 0 {
				break
			}
			if keep > 0 && retained+capacity > limit {
				break
			}
			retained += capacity
			keep = i + 1
		}
		if keep == 0 {
			keep = 1
		}
		for i := keep; i < len(a.fieldSourceSlabs); i++ {
			a.fieldSourceSlabs[i] = fieldSourceSliceSlab{}
		}
		a.fieldSourceSlabs = a.fieldSourceSlabs[:keep]
	}
	for i := range a.fieldSourceSlabs {
		a.fieldSourceSlabs[i].used = 0
	}
	a.childSlabCursor = 0
	a.fieldSlabCursor = 0
	a.fieldSourceSlabCursor = 0

	if limit := maxRetainedNodeCapacityForClass(a.class); len(a.nodes) > limit {
		// Drop the oversized primary array entirely. Setting to nil lets the GC
		// collect it and the next parse will allocate a fresh slab of default
		// size via allocNodeSlow -> ensureNodeCapacity.
		a.nodes = nil
		a.externalScannerNodeCheckpoints = nil
	}
	if len(a.childSlabs) == 0 {
		a.childSlabs = []childSliceSlab{{data: make([]*Node, defaultChildSliceCap(a.class))}}
	}
	if len(a.fieldSlabs) == 0 {
		a.fieldSlabs = []fieldSliceSlab{{data: make([]FieldID, defaultFieldSliceCap(a.class))}}
	}
	if len(a.fieldSourceSlabs) == 0 {
		a.fieldSourceSlabs = []fieldSourceSliceSlab{{data: make([]uint8, defaultFieldSliceCap(a.class))}}
	}
	a.clearBudget()
}

func (a *nodeArena) allocNode() *Node {
	if a == nil {
		return &Node{}
	}
	return a.allocNodeFast()
}

func (a *nodeArena) allocNodeFast() *Node {
	if a.used < len(a.nodes) {
		n := &a.nodes[a.used]
		a.used++
		// Node is already zeroed: fresh slabs by make(), reused slabs by reset().
		return n
	}
	return a.allocNodeSlow()
}

func (a *nodeArena) allocNodeSlow() *Node {
	if len(a.nodeSlabs) == 0 {
		capacity := max(nodeCapacityForClass(a.class), minArenaNodeCap)
		a.nodeSlabs = append(a.nodeSlabs, nodeSlab{data: make([]Node, capacity)})
		a.allocatedBytes += nodeBytesForCap(capacity)
		a.nodeSlabCursor = 0
	}
	if a.nodeSlabCursor < 0 || a.nodeSlabCursor >= len(a.nodeSlabs) {
		a.nodeSlabCursor = 0
	}
	for i := a.nodeSlabCursor; ; i++ {
		if i >= len(a.nodeSlabs) {
			lastCap := len(a.nodeSlabs[len(a.nodeSlabs)-1].data)
			capacity := max(lastCap*2, minArenaNodeCap)
			a.nodeSlabs = append(a.nodeSlabs, nodeSlab{data: make([]Node, capacity)})
			a.allocatedBytes += nodeBytesForCap(capacity)
		}

		slab := &a.nodeSlabs[i]
		if slab.used >= len(slab.data) {
			continue
		}
		idx := slab.used
		slab.used++
		a.nodeSlabCursor = i
		a.used++
		n := &slab.data[idx]
		// Node is already zeroed: fresh slabs by make(), reused slabs by reset().
		return n
	}
}

func (a *nodeArena) ensureNodeCapacity(min int) {
	if a == nil || min <= len(a.nodes) {
		return
	}
	if a.used > 0 {
		// Pre-sizing is only valid before the arena starts serving allocations.
		// Calling this after allocation begins is an internal usage bug.
		panic("ensureNodeCapacity called after arena allocations started")
	}
	newCap := max(len(a.nodes), minArenaNodeCap)
	for newCap < min {
		newCap *= 2
	}
	a.nodes = make([]Node, newCap)
	a.used = 0
	a.nodeSlabs = nil
	a.nodeSlabCursor = 0
	a.externalScannerNodeCheckpoints = nil
	a.externalScannerNodeCheckpointSlabs = nil
	a.recomputeAllocatedBytes()
}

func (a *nodeArena) allocNodeSlice(n int) []*Node {
	if n <= 0 {
		return nil
	}
	if a == nil {
		return make([]*Node, n)
	}

	if len(a.childSlabs) == 0 {
		a.childSlabs = append(a.childSlabs, childSliceSlab{data: make([]*Node, defaultChildSliceCap(a.class))})
		a.childSlabCursor = 0
	}
	if a.childSlabCursor < 0 || a.childSlabCursor >= len(a.childSlabs) {
		a.childSlabCursor = 0
	}

	for i := a.childSlabCursor; ; i++ {
		if i >= len(a.childSlabs) {
			capacity := max(defaultChildSliceCap(a.class), n)
			a.childSlabs = append(a.childSlabs, childSliceSlab{data: make([]*Node, capacity)})
			a.allocatedBytes += childSliceBytesForCap(capacity)
		}

		slab := &a.childSlabs[i]
		if len(slab.data)-slab.used < n {
			continue
		}
		start := slab.used
		slab.used += n
		a.childSlabCursor = i
		out := slab.data[start:slab.used]
		// Full-parse arena reset can skip bulk child-slab clearing to avoid
		// large memclr work on release. Zero the slice on allocation so reused
		// child slabs never leak stale child pointers into later parses.
		clear(out)
		return out
	}
}

func (a *nodeArena) allocFieldIDSlice(n int) []FieldID {
	if n <= 0 {
		return nil
	}
	if a == nil {
		return make([]FieldID, n)
	}

	if len(a.fieldSlabs) == 0 {
		a.fieldSlabs = append(a.fieldSlabs, fieldSliceSlab{data: make([]FieldID, defaultFieldSliceCap(a.class))})
		a.fieldSlabCursor = 0
	}
	if a.fieldSlabCursor < 0 || a.fieldSlabCursor >= len(a.fieldSlabs) {
		a.fieldSlabCursor = 0
	}

	for i := a.fieldSlabCursor; ; i++ {
		if i >= len(a.fieldSlabs) {
			capacity := max(defaultFieldSliceCap(a.class), n)
			a.fieldSlabs = append(a.fieldSlabs, fieldSliceSlab{data: make([]FieldID, capacity)})
			a.allocatedBytes += fieldSliceBytesForCap(capacity)
		}

		slab := &a.fieldSlabs[i]
		if len(slab.data)-slab.used < n {
			continue
		}
		start := slab.used
		slab.used += n
		a.fieldSlabCursor = i
		out := slab.data[start:slab.used]
		clear(out)
		return out
	}
}

func (a *nodeArena) allocFieldSourceSlice(n int) []uint8 {
	if n <= 0 {
		return nil
	}
	if a == nil {
		return make([]uint8, n)
	}

	if len(a.fieldSourceSlabs) == 0 {
		a.fieldSourceSlabs = append(a.fieldSourceSlabs, fieldSourceSliceSlab{data: make([]uint8, defaultFieldSliceCap(a.class))})
		a.allocatedBytes += fieldSourceSliceBytesForCap(defaultFieldSliceCap(a.class))
		a.fieldSourceSlabCursor = 0
	}
	if a.fieldSourceSlabCursor < 0 || a.fieldSourceSlabCursor >= len(a.fieldSourceSlabs) {
		a.fieldSourceSlabCursor = 0
	}

	for i := a.fieldSourceSlabCursor; ; i++ {
		if i >= len(a.fieldSourceSlabs) {
			capacity := max(defaultFieldSliceCap(a.class), n)
			a.fieldSourceSlabs = append(a.fieldSourceSlabs, fieldSourceSliceSlab{data: make([]uint8, capacity)})
			a.allocatedBytes += fieldSourceSliceBytesForCap(capacity)
		}

		slab := &a.fieldSourceSlabs[i]
		if len(slab.data)-slab.used < n {
			continue
		}
		start := slab.used
		slab.used += n
		a.fieldSourceSlabCursor = i
		out := slab.data[start:slab.used]
		clear(out)
		return out
	}
}

func defaultChildSliceCap(class arenaClass) int {
	if class == arenaClassIncremental {
		return incrementalChildSliceCap
	}
	return fullChildSliceCap
}

func defaultFieldSliceCap(class arenaClass) int {
	if class == arenaClassIncremental {
		return incrementalFieldSliceCap
	}
	return fullFieldSliceCap
}

func nodeCapacityForClass(class arenaClass) int {
	if class == arenaClassIncremental {
		return nodeCapacityForBytes(incrementalArenaSlab)
	}
	return nodeCapacityForBytes(fullParseArenaSlab)
}

func nodeBytesForCap(n int) int64 {
	if n <= 0 {
		return 0
	}
	return int64(n) * int64(unsafe.Sizeof(Node{}))
}

func childSliceBytesForCap(n int) int64 {
	if n <= 0 {
		return 0
	}
	return int64(n) * int64(unsafe.Sizeof((*Node)(nil)))
}

func fieldSliceBytesForCap(n int) int64 {
	if n <= 0 {
		return 0
	}
	return int64(n) * int64(unsafe.Sizeof(FieldID(0)))
}

func fieldSourceSliceBytesForCap(n int) int64 {
	if n <= 0 {
		return 0
	}
	return int64(n)
}

func externalScannerCheckpointBytesForCap(n int) int64 {
	if n <= 0 {
		return 0
	}
	return int64(n) * int64(unsafe.Sizeof(externalScannerCheckpointRef{}))
}

func (a *nodeArena) recomputeAllocatedBytes() {
	if a == nil {
		return
	}
	total := nodeBytesForCap(len(a.nodes))
	for i := range a.nodeSlabs {
		total += nodeBytesForCap(len(a.nodeSlabs[i].data))
	}
	for i := range a.childSlabs {
		total += childSliceBytesForCap(len(a.childSlabs[i].data))
	}
	for i := range a.fieldSlabs {
		total += fieldSliceBytesForCap(len(a.fieldSlabs[i].data))
	}
	for i := range a.fieldSourceSlabs {
		total += fieldSourceSliceBytesForCap(len(a.fieldSourceSlabs[i].data))
	}
	total += externalScannerCheckpointBytesForCap(len(a.externalScannerNodeCheckpoints))
	for i := range a.externalScannerNodeCheckpointSlabs {
		total += externalScannerCheckpointBytesForCap(len(a.externalScannerNodeCheckpointSlabs[i].data))
	}
	a.allocatedBytes = total
}

func (a *nodeArena) allocExternalScannerSnapshotRef(src []byte) externalScannerSnapshotRef {
	n := len(src)
	if a == nil || n == 0 {
		return externalScannerSnapshotRef{}
	}

	if len(a.fieldSourceSlabs) == 0 {
		a.fieldSourceSlabs = append(a.fieldSourceSlabs, fieldSourceSliceSlab{data: make([]uint8, defaultFieldSliceCap(a.class))})
		a.allocatedBytes += fieldSourceSliceBytesForCap(defaultFieldSliceCap(a.class))
		a.fieldSourceSlabCursor = 0
	}
	if a.fieldSourceSlabCursor < 0 || a.fieldSourceSlabCursor >= len(a.fieldSourceSlabs) {
		a.fieldSourceSlabCursor = 0
	}

	for i := a.fieldSourceSlabCursor; ; i++ {
		if i >= len(a.fieldSourceSlabs) {
			capacity := max(defaultFieldSliceCap(a.class), n)
			a.fieldSourceSlabs = append(a.fieldSourceSlabs, fieldSourceSliceSlab{data: make([]uint8, capacity)})
			a.allocatedBytes += fieldSourceSliceBytesForCap(capacity)
		}

		slab := &a.fieldSourceSlabs[i]
		if len(slab.data)-slab.used < n {
			continue
		}
		start := slab.used
		slab.used += n
		a.fieldSourceSlabCursor = i
		copy(slab.data[start:slab.used], src)
		return externalScannerSnapshotRef{
			slab: uint16(i),
			off:  uint32(start),
			len:  uint16(n),
		}
	}
}

func (a *nodeArena) externalScannerSnapshotBytes(ref externalScannerSnapshotRef) []byte {
	if a == nil || ref.len == 0 {
		return nil
	}
	if int(ref.slab) >= len(a.fieldSourceSlabs) {
		return nil
	}
	slab := a.fieldSourceSlabs[ref.slab].data
	start := int(ref.off)
	end := start + int(ref.len)
	if start < 0 || end > len(slab) || start > end {
		return nil
	}
	return slab[start:end]
}

func (a *nodeArena) clearBudget() {
	if a == nil {
		return
	}
	a.budgetBytes = 0
	a.recomputeAllocatedBytes()
}

func (a *nodeArena) setBudget(bytes int64) {
	if a == nil {
		return
	}
	a.budgetBytes = bytes
}

func (a *nodeArena) budgetExhausted() bool {
	if a == nil || a.budgetBytes <= 0 {
		return false
	}
	return a.allocatedBytes >= a.budgetBytes
}

func maxRetainedNodeCapacityForClass(class arenaClass) int {
	factor := maxRetainedArenaFactor
	floor := maxRetainedIncrementalNodeCap
	if class == arenaClassFull {
		factor = maxRetainedFullNodeArenaFactor
		floor = maxRetainedFullNodeCap
	}
	return max(nodeCapacityForClass(class)*factor, floor)
}

func maxRetainedOverflowNodeCapacityForClass(class arenaClass) int {
	return max(maxRetainedNodeCapacityForClass(class)/2, nodeCapacityForClass(class))
}

func maxRetainedChildSliceCapacityForClass(class arenaClass) int {
	factor := maxRetainedArenaFactor
	floor := maxRetainedIncrementalSliceCap
	if class == arenaClassFull {
		factor = maxRetainedFullSliceArenaFactor
		floor = maxRetainedFullSliceCap
	}
	return max(defaultChildSliceCap(class)*factor, floor)
}

func maxRetainedFieldSliceCapacityForClass(class arenaClass) int {
	factor := maxRetainedArenaFactor
	floor := maxRetainedIncrementalSliceCap
	if class == arenaClassFull {
		factor = maxRetainedFullSliceArenaFactor
		floor = maxRetainedFullSliceCap
	}
	return max(defaultFieldSliceCap(class)*factor, floor)
}

func maxRetainedFieldSourceSliceCapacityForClass(class arenaClass) int {
	factor := maxRetainedArenaFactor
	floor := maxRetainedIncrementalSliceCap
	if class == arenaClassFull {
		factor = maxRetainedFullSliceArenaFactor
		floor = maxRetainedFullSliceCap
	}
	return max(defaultFieldSliceCap(class)*factor, floor)
}
