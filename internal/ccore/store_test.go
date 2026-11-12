package ccore

import (
	"math/rand"
	"runtime"
	"sort"
	"sync"
	"testing"
	"time"
)

func TestSortU32(t *testing.T) {
	rng := rand.New(rand.NewSource(1))
	var tmp []uint32
	for _, n := range []int{0, 1, 2, 5, 47, 48, 100, 5000, 70000} {
		for _, max := range []uint32{10, 4000, 1 << 22, 1 << 31} {
			a := make([]uint32, n)
			for i := range a {
				a[i] = uint32(rng.Int63n(int64(max)))
			}
			want := append([]uint32(nil), a...)
			sort.Slice(want, func(i, j int) bool { return want[i] < want[j] })
			sortU32(a, &tmp)
			for i := range a {
				if a[i] != want[i] {
					t.Fatalf("n=%d max=%d: differs at %d", n, max, i)
				}
			}
		}
	}
}

func TestSizeClasses(t *testing.T) {
	prevClass, prevSize := -1, uint32(0)
	for n := uint32(1); n <= kidPageLen; n++ {
		class, size := kidClass(n)
		if size < n || class < prevClass || size < prevSize || class >= numKidClasses || kidClassSize(class) != int(size) {
			t.Fatalf("kidClass(%d) = %d, %d", n, class, size)
		}
		prevClass, prevSize = class, size
	}
	for n := uint32(1); n <= 4096; n++ {
		class, size := stClass(n)
		if size < n || size < 8 || class < 0 || class >= numStClasses || size != 8<<class {
			t.Fatalf("stClass(%d) = %d, %d", n, class, size)
		}
	}
}

// buildTree makes a complete tree of the given depth and fan-out out of
// internal records and heap leaves (no parser: the records are set directly).
func buildTree(s *Store, depth, fan int) Handle {
	if depth == 0 {
		return s.add(rec{symbol: 3, size: Length{bytes: 300}})
	}
	kids := make([]Handle, fan)
	for i := range kids {
		kids[i] = buildTree(s, depth-1, fan)
	}
	return s.add(rec{symbol: 4, childCount: uint32(fan), childrenStart: s.addKids(kids)})
}

func TestReleaseWalkAndFreeOrder(t *testing.T) {
	c := newStoreCore()
	s := c.view()
	c.lease.Lock()
	s.beginParse(false) // a logging parse: records get the new bit
	root := buildTree(&s, 4, 3)
	total := 1 + 3 + 9 + 27 + 81
	if got := s.allocatedRecords(); got != total {
		t.Fatalf("allocated %d records, want %d", got, total)
	}
	s.publish(root) // everything is reachable: nothing is swept
	if c.freeRec != 0 {
		t.Fatal("publication freed a reachable record")
	}
	before := counters.double.Load()
	s.releaseWalk(root)
	s.flushFree()
	n, prev := 0, uint32(0)
	for i := c.freeRec; i != 0; i = s.refs[i>>recShift][i&recMask] & countMask {
		if i <= prev {
			t.Fatalf("free list not ascending: %d after %d", i, prev)
		}
		prev = i
		n++
	}
	if n != total {
		t.Fatalf("free list holds %d records, want %d", n, total)
	}
	s.releaseWalk(root) // a second release of the root reaches a free record
	if counters.double.Load() != before+1 {
		t.Fatal("double free not counted")
	}
	// The next parse takes the records in ascending order.
	s.beginParse(false)
	a := s.add(rec{})
	b := s.add(rec{})
	if !(a>>1 < b>>1) || a>>1 != 1 {
		t.Fatalf("records taken from the free list: %d, %d", a>>1, b>>1)
	}
	c.lease.Unlock()
}

func TestSharedInTreeCountsOwners(t *testing.T) {
	c := newStoreCore()
	s := c.view()
	c.lease.Lock()
	s.beginParse(false)
	leaf := s.add(rec{symbol: 3}) // one record in two slots of one tree
	root := s.add(rec{symbol: 4, childCount: 2, childrenStart: s.addKids([]Handle{leaf, leaf})})
	before, beforeDouble := counters.shared.Load(), counters.double.Load()
	s.publish(root)
	if counters.shared.Load() != before+1 || *(&s.refs[leaf>>(recShift+1)][(leaf>>1)&recMask])&countMask != 1 {
		t.Fatal("a record in two slots must have one owner beyond the first")
	}
	s.releaseWalk(root)
	s.flushFree()
	if c.freeRec == 0 || counters.double.Load() != beforeDouble {
		t.Fatal("release of a shared record must free it once and report no double free")
	}
	c.lease.Unlock()
}

func TestSweepFreesUnreachable(t *testing.T) {
	c := newStoreCore()
	s := c.view()
	c.lease.Lock()
	s.beginParse(false)
	keep := s.add(rec{symbol: 3})
	drop := s.add(rec{symbol: 4, childCount: 2, childrenStart: s.addKids([]Handle{keep, s.add(rec{symbol: 5})})})
	_ = drop
	root := s.add(rec{symbol: 6, childCount: 1, childrenStart: s.addKids([]Handle{keep})})
	s.publish(root)
	n := 0
	for i := c.freeRec; i != 0; i = s.refs[i>>recShift][i&recMask] & countMask {
		n++
	}
	if n != 2 { // drop and its second child
		t.Fatalf("sweep freed %d records, want 2", n)
	}
	if s.rec(keep).flags&(heapNew|heapMark) != 0 || s.refs[keep>>(recShift+1)][(keep>>1)&recMask]&countMask != 0 {
		t.Fatal("a kept record must have new and mark cleared and count 0")
	}
	c.lease.Unlock()
}

func TestCellCountsAndEnd(t *testing.T) {
	c := newStoreCore()
	c.cells.Store(1) // the parse
	s := c.view()
	c.lease.Lock()
	s.beginParse(true)
	root := s.add(rec{symbol: 3})
	cl := c.newCell(root)
	tr := &Tree{c: cl, rd: s}
	c.cells.Add(-1) // the parse ends
	c.lease.Unlock()
	cp := tr.Copy()
	if c.cells.Load() != 2 {
		t.Fatalf("cells = %d, want 2", c.cells.Load())
	}
	tr.Release()
	tr.Release() // a second release does nothing
	if c.cells.Load() != 1 || c.ended.Load() {
		t.Fatalf("after one release: cells %d ended %t", c.cells.Load(), c.ended.Load())
	}
	cp.Release()
	if c.cells.Load() != 0 || !c.ended.Load() || c.gen != 1 {
		t.Fatalf("after both releases: cells %d ended %t gen %d", c.cells.Load(), c.ended.Load(), c.gen)
	}
}

func TestFinalizerReleases(t *testing.T) {
	c := newStoreCore()
	c.cells.Store(1)
	s := c.view()
	c.lease.Lock()
	s.beginParse(true)
	root := s.add(rec{symbol: 3})
	func() {
		_ = &Tree{c: c.newCell(root), rd: s} // dropped without Release
	}()
	c.cells.Add(-1)
	c.lease.Unlock()
	for i := 0; i < 50 && !c.ended.Load(); i++ {
		runtime.GC()
		time.Sleep(10 * time.Millisecond)
	}
	if !c.ended.Load() {
		t.Fatal("the finalizer did not release the dropped tree")
	}
}

// buildSized makes a complete tree whose leaves are heap leaves of 300 bytes
// and whose internal nodes have the summed size, so that edits find a path.
func buildSized(s *Store, depth, fan int) (Handle, uint32) {
	if depth == 0 {
		return s.add(rec{symbol: 3, size: Length{bytes: 300, col: 300}}), 300
	}
	kids := make([]Handle, fan)
	var total uint32
	for i := range kids {
		var n uint32
		kids[i], n = buildSized(s, depth-1, fan)
		total += n
	}
	return s.add(rec{symbol: 4, childCount: uint32(fan), childrenStart: s.addKids(kids), size: Length{bytes: total, col: total}}), total
}

func TestEditAllocatorReusesFreedRecords(t *testing.T) {
	beforeDouble, beforeBad := counters.double.Load(), counters.badRetain.Load()
	c := newStoreCore()
	c.cells.Store(1)
	s := c.view()
	c.lease.Lock()
	s.beginParse(true)
	root, _ := buildSized(&s, 3, 3)
	tr := &Tree{c: c.newCell(root), rd: s}
	c.cells.Add(-1)
	c.lease.Unlock()
	e := Edit{StartByte: 5, OldEndByte: 6, NewEndByte: 6, StartCol: 5, OldEndCol: 6, NewEndCol: 6}
	var warm, kidWarm int
	for i := 0; i < 400; i++ {
		n := tr.Copy()
		n.Edit(e)
		if n.LastEditRecords() == 0 {
			t.Fatal("the edit copied no record")
		}
		tr.Release()
		tr = n
		if i == 20 {
			warm, kidWarm = c.ec.recPage*recPageLen+c.ec.recSlot, c.ec.kidPage*kidPageLen+c.ec.kidOff
		}
	}
	end, kidEnd := c.ec.recPage*recPageLen+c.ec.recSlot, c.ec.kidPage*kidPageLen+c.ec.kidOff
	if end != warm || kidEnd != kidWarm {
		t.Fatalf("the edit allocator kept growing: records %d to %d, child handles %d to %d", warm, end, kidWarm, kidEnd)
	}
	if counters.double.Load() != beforeDouble || counters.badRetain.Load() != beforeBad {
		t.Fatal("double free or bad retain")
	}
	tr.Release()
	if !c.ended.Load() {
		t.Fatal("the store did not end")
	}
}

// ---- tests of the store's edge cases ----

// newTestTree makes a store with one tree of buildSized(depth, fan) records
// and returns the store core, the tree and the view the tree was built with.
func newTestTree(depth, fan int) (*storeCore, *Tree) {
	c := newStoreCore()
	c.cells.Store(1)
	s := c.view()
	c.lease.Lock()
	s.beginParse(true)
	root, _ := buildSized(&s, depth, fan)
	tr := &Tree{c: c.newCell(root), rd: s}
	tr.rd.fresh = false
	c.cells.Add(-1)
	c.lease.Unlock()
	return c, tr
}

// reach walks the records under root and returns how many it met and whether
// any of them is on a free list.
func reach(s *Store, root Handle) (n int, anyFree bool) {
	stack := []Handle{root}
	for len(stack) > 0 {
		h := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		if h == 0 || h&1 != 0 {
			continue
		}
		n++
		if s.refs[h>>(recShift+1)][(h>>1)&recMask]&flagFree != 0 {
			anyFree = true
		}
		if r := s.rec(h); r.childCount > 0 {
			stack = append(stack, s.kidSpan(r.childrenStart, r.childCount)...)
		}
	}
	return n, anyFree
}

// A released tree cannot be edited. Without the guard the edit queues a release of a root that other trees own, and the
// next drains free the root of a live tree.
func TestEditOfReleasedTreeIsRejected(t *testing.T) {
	beforeDouble, beforeBad := counters.double.Load(), counters.badRetain.Load()
	c, x := newTestTree(3, 3)
	other := x.Copy()
	gone := x.Copy()
	gone.Release()
	e := Edit{StartByte: 5, OldEndByte: 6, NewEndByte: 6, StartCol: 5, OldEndCol: 6, NewEndCol: 6}
	gone.Edit(e)
	if gone.LastEditRecords() != 0 || gone.Root() != 0 {
		t.Fatal("an edit of a released tree changed the tree")
	}
	nBefore, _ := reach(&x.rd, x.Root())
	other.Release() // drains: x and gone were released or edited, so only x owns the root now
	cp := x.Copy()
	cp.Release()
	n, anyFree := reach(&x.rd, x.Root())
	if anyFree || n != nBefore {
		t.Fatalf("the live tree lost records: %d reachable before, %d after, free record met: %t", nBefore, n, anyFree)
	}
	if counters.double.Load() != beforeDouble || counters.badRetain.Load() != beforeBad {
		t.Fatal("double free or bad retain")
	}
	x.Release()
	if !c.ended.Load() {
		t.Fatal("the store did not end")
	}
}

// A count word with the free flag is counted and skipped by the publication
// walk, so the free-list link survives.
func TestPublishSkipsFreedRecord(t *testing.T) {
	c := newStoreCore()
	s := c.view()
	c.lease.Lock()
	defer c.lease.Unlock()
	s.beginParse(true)
	old := s.add(rec{symbol: 3, size: Length{bytes: 4}})
	s.beginParse(false)
	s.refs[old>>(recShift+1)][(old>>1)&recMask] = flagFree | 77 // as if released: free flag and a link
	root := s.add(rec{symbol: 4, childCount: 1, childrenStart: s.addKids([]Handle{old})})
	before := counters.badRetain.Load()
	s.publish(root)
	if counters.badRetain.Load() != before+1 {
		t.Fatal("a retain of a freed record was not counted")
	}
	if w := s.refs[old>>(recShift+1)][(old>>1)&recMask]; w != flagFree|77 {
		t.Fatalf("the free-list link changed: %#x", w)
	}
}

// The pool of pages is bounded in bytes, and pages that stay idle are trimmed.
func TestPoolBoundAndTrim(t *testing.T) {
	TrimPool(1 << 62) // empty the pool whatever earlier tests left in it
	defer SetPoolMaxBytes(SetPoolMaxBytes(2 << 20))
	if PoolBytes() != 0 {
		t.Fatalf("pool not empty after a trim: %d bytes", PoolBytes())
	}
	const bound = 2 << 20
	pages := bound/(kidPageLen*8) + 50 // more kid pages than the bound allows
	kids := make([]*[kidPageLen]Handle, pages)
	for i := range kids {
		kids[i] = new([kidPageLen]Handle)
	}
	putPages(nil, nil, kids, nil)
	if got := PoolBytes(); got > bound || got < bound-kidPageLen*8 {
		t.Fatalf("pool holds %d bytes, bound %d", got, bound)
	}
	TrimPool(time.Now().UnixNano()) // nothing has waited poolIdle yet
	if PoolBytes() == 0 {
		t.Fatal("a trim dropped pages that had not been idle")
	}
	TrimPool(time.Now().Add(2 * poolIdle).UnixNano())
	if PoolBytes() != 0 {
		t.Fatalf("idle pages stayed: %d bytes", PoolBytes())
	}
	if takeKidPage() != nil {
		t.Fatal("a trimmed page came back")
	}
}

// The store must end when the last two trees are released together (made
// deterministic): the second tree's entry is queued before the
// first release, so the first release's snapshot holds it; the second release
// then lowers the cell count to zero from inside the drain and fails TryLock.
// Only the test of the cell count after Unlock ends the store.
func TestStoreEndsWhenLastTwoTreesReleaseTogether(t *testing.T) {
	c := newStoreCore()
	c.cells.Store(1)
	s := c.view()
	c.lease.Lock()
	s.beginParse(true)
	root := s.add(rec{symbol: 3})
	a := &Tree{c: c.newCell(root), rd: s}
	c.cells.Add(-1)
	c.lease.Unlock()
	b := a.Copy()
	b.c.drop() // the second release up to its append
	runtime.SetFinalizer(b.c, nil)
	fired := false
	h := hookFn(func(point string) {
		if point == "drain" && !fired {
			fired = true
			c.cells.Add(-1) // the second release lowers the count ...
			c.tryDrain()    // ... and fails TryLock, because this drain holds the lease
		}
	})
	c.hook.Store(&h)
	a.Release()
	c.hook.Store(nil)
	if !fired || !c.ended.Load() || c.cells.Load() != 0 {
		t.Fatalf("store not ended (fired %t, cells %d)", fired, c.cells.Load())
	}
}

// A panic in a drain poisons the store: the lease is free afterwards, the next
// drain terminates and applies nothing (the queue is dropped, nothing is freed
// piecemeal, so nothing is freed twice), and the live tree still reads every
// record.
func TestDrainPanicPoisonsTheStore(t *testing.T) {
	beforeDouble := counters.double.Load()
	c, x := newTestTree(2, 3)
	cp := x.Copy()    // entry 1: retain
	other := x.Copy() // entry 2: retain
	calls := 0
	h := hookFn(func(point string) {
		if point == "drain" {
			calls++
			if calls == 2 {
				panic("injected at the second entry")
			}
		}
	})
	c.hook.Store(&h)
	func() {
		defer func() { _ = recover() }()
		c.lease.Lock()
		defer c.lease.Unlock()
		v := c.view()
		v.drain()
	}()
	c.hook.Store(nil)
	if !c.poisoned.Load() {
		t.Fatal("the store is not poisoned after a panic in a drain")
	}
	if !c.lease.TryLock() {
		t.Fatal("the lease is held after the panic")
	}
	c.lease.Unlock()
	c.tryDrain()
	if c.hasPending() {
		t.Fatal("a drain of a poisoned store left entries queued")
	}
	cp.Release()
	other.Release()
	if counters.double.Load() != beforeDouble {
		t.Fatal("a record was freed twice")
	}
	n, anyFree := reach(&x.rd, x.Root())
	if anyFree || n == 0 {
		t.Fatalf("the live tree lost records after the copies were released (%d reachable, free met: %t)", n, anyFree)
	}
	if counters.double.Load() != beforeDouble {
		t.Fatal("double free")
	}
	x.Release()
	if !c.ended.Load() {
		t.Fatal("the store did not end")
	}
}

// Release against Edit and against Copy of the same tree: the old root must be
// released once, and the sibling tree stays valid.
func TestReleaseRacesEditAndCopy(t *testing.T) {
	beforeDouble, beforeBad := counters.double.Load(), counters.badRetain.Load()
	c, x := newTestTree(3, 3)
	e := Edit{StartByte: 5, OldEndByte: 6, NewEndByte: 6, StartCol: 5, OldEndCol: 6, NewEndCol: 6}
	for round := 0; round < 400; round++ {
		tr := x.Copy()
		var wg sync.WaitGroup
		wg.Add(2)
		go func() { defer wg.Done(); tr.Release() }()
		if round%2 == 0 {
			go func() { defer wg.Done(); tr.Edit(e) }()
		} else {
			go func() {
				defer wg.Done()
				if n := tr.Copy(); n != nil {
					n.Release()
				}
			}()
		}
		wg.Wait()
	}
	x.Copy().Release() // a drain
	n, anyFree := reach(&x.rd, x.Root())
	if anyFree || n == 0 {
		t.Fatalf("the sibling tree lost records (%d reachable, free met: %t)", n, anyFree)
	}
	if counters.double.Load() != beforeDouble || counters.badRetain.Load() != beforeBad {
		t.Fatalf("double free %d to %d, bad retain %d to %d", beforeDouble, counters.double.Load(), beforeBad, counters.badRetain.Load())
	}
	x.Release()
	if !c.ended.Load() {
		t.Fatal("the store did not end")
	}
}

// A drain applies one snapshot per lease hold. An entry that arrives
// while it runs stays queued for the next hold. This test detects a drain that
// loops over new entries; TestParseStartsUnderCopyEditReleaseTraffic does not
// (a looping drain would only slow it down).
func TestDrainAppliesOneSnapshot(t *testing.T) {
	c, x := newTestTree(2, 3)
	cp := x.Copy() // one retain queued
	c.lease.Lock()
	fired := false
	h := hookFn(func(point string) {
		if point == "drain" && !fired {
			fired = true
			c.appendOps(op{kind: opRetain, h: x.Root(), gen: c.gen}) // arrives during the drain
		}
	})
	c.hook.Store(&h)
	v := c.view()
	v.drain()
	c.hook.Store(nil)
	c.lease.Unlock()
	if !fired || !c.hasPending() {
		t.Fatalf("the late entry was applied in the same hold (fired %t)", fired)
	}
	c.tryDrain()
	if c.hasPending() {
		t.Fatal("tryDrain left an entry queued")
	}
	cp.Release()
	x.Release() // two retains were applied, two releases drop them: the root goes with the second release plus one more
	if c.ended.Load() {
		return
	}
	// the late retain was never matched by a release; end the store by hand
	c.lease.Lock()
	c.endLocked()
	c.lease.Unlock()
}

// Scanner-state bytes of two sizes survive 400 rounds of copy,
// edit and release, then the same through the parse side (copyHeapLeaf, sweep).
func addStateLeaf(s *Store, sym Symbol, n uint32) Handle {
	off := s.allocState(&s.core.pc, n, false)
	b := s.stateBytes(off, n)
	for i := range b {
		b[i] = byte(sym)
	}
	return s.add(rec{symbol: sym, size: Length{bytes: 300, col: 300}, flags: heapHasExternalTokens, visibleDescendantCount: off, namedChildCount: n})
}

// stateListOffsets returns the offsets on a state free list, failing on a cycle.
func stateListOffsets(t *testing.T, s *Store, head uint32) []uint32 {
	var out []uint32
	seen := map[uint32]bool{}
	for off := head; off != 0; {
		if seen[off] {
			t.Fatalf("state offset %d twice on a free list", off)
		}
		seen[off] = true
		out = append(out, off)
		pg := s.state[off>>stShift]
		o := off & stMask
		off = uint32(pg[o]) | uint32(pg[o+1])<<8 | uint32(pg[o+2])<<16 | uint32(pg[o+3])<<24
	}
	return out
}

// checkLeafStates walks root and checks that every leaf with state reads the
// bytes of its own symbol.
func checkLeafStates(t *testing.T, s *Store, root Handle, wantLeaves int) {
	t.Helper()
	leaves := 0
	stack := []Handle{root}
	for len(stack) > 0 {
		h := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		if h == 0 || h&1 != 0 {
			continue
		}
		r := s.rec(h)
		if r.childCount > 0 {
			stack = append(stack, s.kidSpan(r.childrenStart, r.childCount)...)
			continue
		}
		if r.flags&heapHasExternalTokens == 0 {
			continue
		}
		leaves++
		st := s.ExtState(h)
		if uint32(len(st)) != r.namedChildCount || len(st) == 0 {
			t.Fatalf("leaf %d: %d state bytes, want %d", r.symbol, len(st), r.namedChildCount)
		}
		for _, b := range st {
			if b != byte(r.symbol) {
				t.Fatalf("leaf %d reads state byte %d: a state block was reused or overwritten", r.symbol, b)
			}
		}
	}
	if leaves != wantLeaves {
		t.Fatalf("%d leaves with state, want %d", leaves, wantLeaves)
	}
}

func TestScannerStateThroughEditRounds(t *testing.T) {
	beforeDouble, beforeBad := counters.double.Load(), counters.badRetain.Load()
	c := newStoreCore()
	c.cells.Store(1)
	s := c.view()
	c.lease.Lock()
	s.beginParse(true)
	var mids []Handle
	var total uint32
	nLeaves := 0
	for g := 0; g < 3; g++ {
		kids := make([]Handle, 3)
		for i := range kids {
			n := uint32(12)
			if (g+i)%2 == 1 {
				n = 40
			}
			kids[i] = addStateLeaf(&s, Symbol(10+g*3+i), n)
			nLeaves++
		}
		mids = append(mids, s.add(rec{symbol: 4, childCount: 3, childrenStart: s.addKids(kids), size: Length{bytes: 900, col: 900}}))
		total += 900
	}
	root := s.add(rec{symbol: 5, childCount: 3, childrenStart: s.addKids(mids), size: Length{bytes: total, col: total}})
	tr := &Tree{c: c.newCell(root), rd: s}
	tr.rd.fresh = false
	c.cells.Add(-1)
	c.lease.Unlock()
	// Edit rounds: copy, edit inside a leaf (a different leaf each round), release the older tree.
	var warmSt, warmNext int
	stPos := func() int { return c.ec.stPage*stPageLen + c.ec.stOff }
	for i := 0; i < 400; i++ {
		at := uint32((i % 9) * 300)
		e := Edit{StartByte: at + 5, OldEndByte: at + 6, NewEndByte: at + 6, StartCol: at + 5, OldEndCol: at + 6, NewEndCol: at + 6}
		n := tr.Copy()
		n.Edit(e)
		if n.LastEditRecords() == 0 {
			t.Fatal("the edit copied no record")
		}
		tr.Release()
		tr = n
		if i == 20 {
			warmSt, warmNext = stPos(), c.nextState
		}
		if i%50 == 49 {
			checkLeafStates(t, &tr.rd, tr.Root(), nLeaves)
		}
	}
	if stPos() != warmSt || c.nextState != warmNext {
		t.Fatalf("state pages kept growing: position %d to %d, pages %d to %d", warmSt, stPos(), warmNext, c.nextState)
	}
	c.lease.Lock()
	v := c.view()
	for cl := range c.freeSt {
		stateListOffsets(t, &v, c.freeSt[cl])
	}
	c.mu.Lock()
	for cl := range c.editFreeSt {
		stateListOffsets(t, &v, c.editFreeSt[cl])
	}
	c.mu.Unlock()
	c.lease.Unlock()
	checkLeafStates(t, &tr.rd, tr.Root(), nLeaves)
	if counters.double.Load() != beforeDouble || counters.badRetain.Load() != beforeBad {
		t.Fatal("double free or bad retain")
	}
	tr.Release()
	if !c.ended.Load() {
		t.Fatal("the store did not end")
	}
}

func TestScannerStateThroughParseSideCopies(t *testing.T) {
	beforeDouble := counters.double.Load()
	c := newStoreCore()
	c.cells.Store(1)
	s := c.view()
	c.lease.Lock()
	s.beginParse(true)
	leaves := []Handle{addStateLeaf(&s, 20, 12), addStateLeaf(&s, 21, 40)}
	root := s.add(rec{symbol: 5, childCount: 2, childrenStart: s.addKids(leaves), size: Length{bytes: 600, col: 600}})
	tr := &Tree{c: c.newCell(root), rd: s}
	tr.rd.fresh = false
	c.lease.Unlock()
	pos := func() int { return c.pc.stPage*stPageLen + c.pc.stOff }
	var warm int
	for i := 0; i < 400; i++ {
		c.lease.Lock()
		s.beginParse(false)
		a, b := s.copyHeapLeaf(leaves[0]), s.copyHeapLeaf(leaves[1])
		if string(s.ExtState(a)) != string(s.ExtState(leaves[0])) || string(s.ExtState(b)) != string(s.ExtState(leaves[1])) {
			t.Fatal("a copy does not read the bytes of its source")
		}
		if s.rec(a).visibleDescendantCount == s.rec(leaves[0]).visibleDescendantCount {
			t.Fatal("a copy shares the state block of its source")
		}
		s.sweep(false) // nothing is marked: both copies are freed with their state blocks
		c.lease.Unlock()
		if i == 20 {
			warm = pos()
		}
	}
	if pos() != warm {
		t.Fatalf("parse-side state kept growing: %d to %d", warm, pos())
	}
	c.lease.Lock()
	v := c.view()
	for cl := range c.freeSt {
		stateListOffsets(t, &v, c.freeSt[cl])
	}
	c.lease.Unlock()
	checkLeafStates(t, &tr.rd, tr.Root(), 2)
	if counters.double.Load() != beforeDouble {
		t.Fatal("double free")
	}
	c.cells.Add(-1) // the running parse of this test
	tr.Release()
	if !c.ended.Load() {
		t.Fatal("the store did not end")
	}
}

// setExtra on a record with children copies the child list.
func TestSetExtraCopiesChildList(t *testing.T) {
	c := newStoreCore()
	s := c.view()
	c.lease.Lock()
	defer c.lease.Unlock()
	s.beginParse(true)
	kids := []Handle{s.add(rec{symbol: 3}), s.add(rec{symbol: 3})}
	n := s.add(rec{symbol: 4, childCount: 2, childrenStart: s.addKids(kids)})
	cp := s.setExtra(n, true)
	if s.rec(cp).childrenStart == s.rec(n).childrenStart {
		t.Fatal("two records own one child list")
	}
	if got := s.children(cp); len(got) != 2 || got[0] != kids[0] || got[1] != kids[1] {
		t.Fatal("the copied list differs")
	}
}

// Freeing a list longer than a page clears the entry that every snapshot of the
// table sees, also a snapshot taken before the table grew (the entries are
// stable pointers).
func TestFreedLargeListIsClearedInOlderSnapshots(t *testing.T) {
	c := newStoreCore()
	s := c.view()
	start := s.allocKids(&c.pc, kidPageLen+10, false)
	if start&largeBit == 0 {
		t.Fatal("the list is not a large list")
	}
	old := c.view() // a snapshot with one entry
	for i := 0; i < 20; i++ {
		s.allocKids(&c.pc, kidPageLen+10, false) // the table grows and its backing array is replaced
	}
	idx := start &^ largeBit
	if len(old.large[idx].h) == 0 {
		t.Fatal("the old snapshot does not see the list")
	}
	s.freeKidList(start, kidPageLen+10, false)
	if old.large[idx].h != nil {
		t.Fatalf("a snapshot taken before the table grew still holds the freed list (%d handles)", len(old.large[idx].h))
	}
}
