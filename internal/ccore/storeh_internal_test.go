package ccore

import (
	"strings"
	"sync"
	"testing"
)

// runAllocators runs two allocators of one store at the same time (the parse's
// and the edit allocator's, each with a view of its own, as a parse and an edit
// of separate trees have) until both are refused, and returns what they handed out.
func runAllocators(c *storeCore, parse, edit func(s *Store) (got uint32, refused *limitError)) (a, b []uint32, ea, eb *limitError) {
	var wg sync.WaitGroup
	run := func(f func(s *Store) (uint32, *limitError), out *[]uint32, le **limitError) {
		defer wg.Done()
		s := c.view()
		for {
			v, refused := f(&s)
			if refused != nil {
				*le = refused
				return
			}
			*out = append(*out, v)
		}
	}
	wg.Add(2)
	go run(parse, &a, &ea)
	go run(edit, &b, &eb)
	wg.Wait()
	return
}

// Child pages: the allocators of a parse and an edit, both running, claim
// exactly the permitted pages. Under -race the old unlocked read of the page
// table length is reported; without it the two could both pass the limit and
// take a page whose offset reaches largeBit.
func TestConcurrentChildPageLimit(t *testing.T) {
	for round := 0; round < 200; round++ {
		c := newStoreCore()
		c.kidPageLimit.Store(3)
		alloc := func(cur *arenaCur) func(s *Store) (uint32, *limitError) {
			return func(s *Store) (start uint32, le *limitError) {
				le = catchLimit(func() { start = s.allocKids(cur, 8, false) })
				return
			}
		}
		a, b, ea, eb := runAllocators(c, alloc(&c.pc), alloc(&c.ec))
		if ea == nil || eb == nil || !strings.Contains(ea.Error(), "child page limit") {
			t.Fatalf("round %d: not both refused: %v %v", round, ea, eb)
		}
		if len(c.kids) != 3 {
			t.Fatalf("round %d: %d child pages, want exactly the limit 3", round, len(c.kids))
		}
		seen := map[uint32]bool{}
		for _, st := range append(a, b...) {
			if st&largeBit != 0 || st>>kidShift >= 3 || seen[st] {
				t.Fatalf("round %d: list start %#x is outside the permitted pages or handed out twice", round, st)
			}
			seen[st] = true
		}
	}
}

// Scanner-state pages: the same, for the offset that would wrap into page zero.
func TestConcurrentStatePageLimit(t *testing.T) {
	for round := 0; round < 100; round++ {
		c := newStoreCore()
		c.statePageLimit.Store(3)
		alloc := func(cur *arenaCur) func(s *Store) (uint32, *limitError) {
			return func(s *Store) (off uint32, le *limitError) {
				le = catchLimit(func() { off = s.allocState(cur, 4096, false) })
				return
			}
		}
		a, b, ea, eb := runAllocators(c, alloc(&c.pc), alloc(&c.ec))
		if ea == nil || eb == nil || !strings.Contains(ea.Error(), "state page limit") {
			t.Fatalf("round %d: not both refused: %v %v", round, ea, eb)
		}
		if len(c.state) != 3 {
			t.Fatalf("round %d: %d state pages, want exactly the limit 3", round, len(c.state))
		}
		seen := map[uint32]bool{}
		for _, off := range append(a, b...) {
			if off>>stShift >= 3 || seen[off] {
				t.Fatalf("round %d: offset %#x is outside the permitted pages or handed out twice", round, off)
			}
			seen[off] = true
		}
	}
}

// Record pages: the parse allocator and the edit allocator share the pages
// that the limit permits; every index is handed out once and none passes the limit.
func TestConcurrentRecordLimit(t *testing.T) {
	const pages = 3
	for round := 0; round < 20; round++ {
		c := newStoreCore()
		c.recLimit.Store(pages<<recShift - 1)
		parse := func(s *Store) (idx uint32, le *limitError) {
			le = catchLimit(func() { idx = uint32(s.add(rec{}) >> 1) })
			return
		}
		edit := func(s *Store) (idx uint32, le *limitError) {
			le = catchLimit(func() { idx = uint32(s.claimEdit() >> 1) })
			return
		}
		a, b, ea, eb := runAllocators(c, parse, edit)
		if ea == nil || eb == nil {
			t.Fatalf("round %d: not both refused: %v %v", round, ea, eb)
		}
		if len(c.recs) != pages {
			t.Fatalf("round %d: %d record pages, want exactly the %d that the limit permits", round, len(c.recs), pages)
		}
		seen := map[uint32]bool{}
		for _, i := range append(a, b...) {
			if i == 0 || i > c.recLimit.Load() || seen[i] {
				t.Fatalf("round %d: record %d is over the limit, is the null record, or was handed out twice", round, i)
			}
			seen[i] = true
		}
		if len(seen) != pages*recPageLen-1 {
			t.Fatalf("round %d: %d records handed out, want %d", round, len(seen), pages*recPageLen-1)
		}
	}
}

// The deterministic interleaving of the record-reservation gap: the parse has
// claimed the last permitted record page and, before its node's child list is
// allocated, the edit allocator asks for a record. The edit is refused; the
// parse's node is whole. (A check that only looks, and a claim made after the
// storage, strand a child list of the parse when the edit takes the page first.)
func TestRecordClaimIsNotOvertakenBeforeItsStorage(t *testing.T) {
	p := newTestParser(t)
	p.beginParse(false) // an incremental parse: it logs, so the sweep of an aborted parse would free what it claimed
	c := p.core
	a := p.newLeaf(1, Length{}, Length{bytes: 300}, 0, 0, false, false, false)
	b := p.newLeaf(2, Length{}, Length{bytes: 300}, 0, 0, false, false, false)
	for c.pc.recSlot < recPageLen { // the parse's page is full: its next record needs a new page
		p.add(rec{})
	}
	ed := c.view()
	for { // the edit allocator's page is full too
		ed.claimEdit()
		if c.ec.recSlot == recPageLen {
			break
		}
	}
	c.recLimit.Store(uint32(len(c.recs)+1)<<recShift - 1) // exactly one more page is permitted
	// the hook fires after the child list exists: the other allocator asks for a record page there
	var edErr *limitError
	fired := 0
	hook := hookFn(func(pt string) {
		if pt == "storage" {
			fired++
			edErr = catchLimit(func() { ed.claimEdit() }) // the other allocator asks for the page the parse just took
		}
	})
	c.hook.Store(&hook)
	var n Handle
	if le := catchLimit(func() { n = p.newNode(5, []Handle{a, b}, 0) }); le != nil {
		t.Fatalf("the parse's node was refused after its record was claimed: %v", le)
	}
	c.hook.Store(nil)
	if fired == 0 {
		t.Fatal("the hook did not run")
	}
	if edErr == nil || !strings.Contains(edErr.Error(), "record index limit") {
		t.Fatalf("the edit allocator was not refused: %v", edErr)
	}
	if got := p.children(n); len(got) != 2 || got[0] != a || got[1] != b {
		t.Fatalf("the node's child list is %v", got)
	}
}

// A record index from a free list is checked against the record limit too: a
// lowered limit stops it, and the list is not touched.
func TestFreeListIndexIsCheckedAgainstTheLimit(t *testing.T) {
	p := newTestParser(t)
	p.beginParse(false)
	c := p.core
	for i := 0; i < 3; i++ {
		p.add(rec{})
	}
	p.sweep(true) // all three go to the parse's free list
	head := c.freeRec
	if head == 0 {
		t.Fatal("nothing on the free list")
	}
	c.recLimit.Store(head - 1)
	le := catchLimit(func() { p.add(rec{}) })
	if le == nil || !strings.Contains(le.Error(), "record index limit") {
		t.Fatalf("limit error %v: the index from the free list was not checked", le)
	}
	if c.freeRec != head {
		t.Errorf("the refused claim popped the free list (head %d, was %d)", c.freeRec, head)
	}

	// the edit allocator's free list
	s := c.view()
	c.recLimit.Store(defaultRecLimit)
	x, y := uint32(s.claimEdit()>>1), uint32(s.claimEdit()>>1)
	c.freedEditRecs = append(c.freedEditRecs, x, y)
	s.refs[x>>recShift][x&recMask], s.refs[y>>recShift][y&recMask] = flagEdit, flagEdit
	s.flushFreeEdit()
	if c.editFreeRec != x {
		t.Fatalf("edit free list head %d, want %d", c.editFreeRec, x)
	}
	c.recLimit.Store(x - 1)
	le = catchLimit(func() { s.claimEdit() })
	if le == nil || !strings.Contains(le.Error(), "record index limit") {
		t.Fatalf("limit error %v: the edit free list index was not checked", le)
	}
	if c.editFreeRec != x {
		t.Errorf("the refused claim popped the edit free list")
	}
}

// The first child page is under the limit too: a limit of zero stops a parse
// that needs the first page (before this, an empty input took it unchecked).
func TestFirstChildPageIsUnderTheLimit(t *testing.T) {
	c := newStoreCore()
	c.kidPageLimit.Store(0)
	s := c.view()
	le := catchLimit(func() { s.growKids(&c.pc) })
	if le == nil || !strings.Contains(le.Error(), "child page limit") {
		t.Fatalf("limit error %v", le)
	}
	if len(c.kids) != 0 {
		t.Fatalf("%d child pages claimed under a limit of zero", len(c.kids))
	}
}

// allocatedRecords is exact at full-page boundaries: the page number and the
// slot are added (a full page has slot 4096, which or-ed in would collide with
// the page bit).
func TestAllocatedRecordsAtPageBoundaries(t *testing.T) {
	p := newTestParser(t)
	n := 0
	for _, target := range []int{1, recPageLen - 2, recPageLen - 1, recPageLen, recPageLen + 1, 2*recPageLen - 2, 2*recPageLen - 1, 2 * recPageLen, 2*recPageLen + 1, 3*recPageLen - 1} {
		for n < target {
			p.add(rec{})
			n++
		}
		if got := p.allocatedRecords(); got != n {
			t.Fatalf("after %d records allocatedRecords is %d (page %d slot %d)", n, got, p.core.pc.recPage, p.core.pc.recSlot)
		}
	}
}

// The balance step through its real entry: a node that the finished tree
// reaches twice is not rewritten, whatever the sizes of the nodes (zero-width
// or not), and a fresh tree counts the owner that the second path adds.
// H=S[a,b]; G=S[H,c]; X=S[G,d]; R=S[X,e]; W=T[R,X]. Compressing R through X
// would change the X under W too.
func TestBalanceRefusesANodeReachedTwiceThroughTheRealEntry(t *testing.T) {
	for _, size := range []uint32{0, 1} {
		build := func() (p *Parser, x, g, d, w Handle) {
			p = newTestParser(t)
			leaf := func() Handle {
				return p.newLeaf(1, Length{}, Length{bytes: size, col: size}, 0, 0, false, false, false)
			}
			a, b, c, d1, e := leaf(), leaf(), leaf(), leaf(), leaf()
			h := p.newNode(5, []Handle{a, b}, 0)
			g = p.newNode(5, []Handle{h, c}, 0)
			x = p.newNode(5, []Handle{g, d1}, 0)
			r := p.newNode(5, []Handle{x, e}, 0)
			w = p.newNode(6, []Handle{r, x}, 0)
			p.finished = w
			return p, x, g, d1, w
		}
		p, x, g, d, _ := build()
		p.balance()
		if p.balanceChecks != 1 {
			t.Fatalf("size %d: the walk for records reached twice did not run", size)
		}
		if k := p.children(x); k[0] != g || k[1] != d {
			t.Fatalf("size %d: X was rewritten to %v although the tree reaches it twice", size, k)
		}
		if p.balanceRefusals == 0 {
			t.Fatalf("size %d: the refusal was not counted", size)
		}
		if n := p.multi[x]; n != 1 || len(p.multi) != 1 {
			t.Fatalf("size %d: multi = %v, want X once", size, p.multi)
		}
		// Control: without the walk the same balance rewrites X.
		p, x, g, _, _ = build()
		p.multi = map[Handle]uint32{}
		saved := p.finished
		p.compress(p.children(saved)[0], 1, &[]Handle{})
		if k := p.children(x); k[0] == g {
			t.Fatalf("size %d: the control did not rewrite X: the graph does not show the difference", size)
		}
	}
}

// Ownership of a fresh tree that reaches a record twice is counted, so that a
// later tree that also owns the record keeps it when the first tree is
// released: W owns X twice (fresh, count 0 before); V, a later tree, owns X once.
func TestFreshTreeCountsARecordItOwnsTwice(t *testing.T) {
	p := newTestParser(t)
	leaf := func() Handle { return p.newLeaf(1, Length{}, Length{}, 0, 0, false, false, false) }
	a, b, cc, d, e := leaf(), leaf(), leaf(), leaf(), leaf()
	h := p.newNode(5, []Handle{a, b}, 0)
	g := p.newNode(5, []Handle{h, cc}, 0)
	x := p.newNode(5, []Handle{g, d}, 0)
	r := p.newNode(5, []Handle{x, e}, 0)
	w := p.newNode(6, []Handle{r, x}, 0)
	p.finished = w
	p.balance()
	p.countFreshShared()
	ref := func(h Handle) uint32 { return p.refs[h>>(recShift+1)][(h>>1)&recMask] }
	if got := ref(x); got != 1 {
		t.Fatalf("the reference word of X is %d, want 1 (one owner beyond the first)", got)
	}
	// A later (incremental) parse makes V over X and publishes it: X belongs to an older tree.
	p.beginParse(false)
	f := leaf()
	v := p.newNode(7, []Handle{x, f}, 0)
	p.publish(v)
	if got := ref(x); got != 2 {
		t.Fatalf("after V is published the reference word of X is %d, want 2", got)
	}
	double0 := counters.double.Load()
	p.releaseWalk(w) // the fresh tree goes: X loses its two owners from W
	p.flushFree()
	if ref(x)&flagFree != 0 {
		t.Fatal("X was freed while the later tree still owns it")
	}
	if got := ref(x) & countMask; got != 0 {
		t.Fatalf("X has count %d after W was released, want 0 (V's single owner)", got)
	}
	p.releaseWalk(v)
	p.flushFree()
	if counters.double.Load() != double0 {
		t.Error("a record was released twice")
	}
	if ref(x)&flagFree == 0 {
		t.Error("X was not freed when its last owner went")
	}
}
