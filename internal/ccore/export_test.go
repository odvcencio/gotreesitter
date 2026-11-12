package ccore

// Hooks for the tests in package ccore_test. They read or drive the internals
// of one store; none runs in production code.

// SetTestHook sets the function that the store of t calls at its named points
// ("drain", "sweep", "publish", "hold"); nil turns it off.
func SetTestHook(t *Tree, f func(point string)) {
	if f == nil {
		t.c.store.hook.Store(nil)
		return
	}
	h := hookFn(f)
	t.c.store.hook.Store(&h)
}

// EndStore ends the store of t by hand (a test that dropped trees after a
// panic in a drain, whose counts are short, uses it to free the pages).
func EndStore(t *Tree) {
	c := t.c.store
	c.lease.Lock()
	defer c.lease.Unlock()
	c.endLocked()
}

// PoisonUnused writes junk into the record slots that no allocator has handed
// out on the current pages of the parse and of the edit allocator.
func PoisonUnused(t *Tree) {
	c := t.c.store
	c.lease.Lock()
	defer c.lease.Unlock()
	c.emu.Lock()
	defer c.emu.Unlock()
	for _, cur := range []*arenaCur{&c.pc, &c.ec} {
		if cur.recPage < 0 {
			continue
		}
		for i := cur.recSlot; i < recPageLen; i++ {
			c.recs[cur.recPage][i] = rec{symbol: 777, size: Length{bytes: 12345}, childCount: 3}
		}
	}
}

// Leaked counts the record slots handed out that are neither on a free list nor
// reachable from the roots of the given trees (all of one store).
func Leaked(trees ...*Tree) int {
	c := trees[0].c.store
	c.lease.Lock()
	defer c.lease.Unlock()
	c.emu.Lock()
	defer c.emu.Unlock()
	v := c.view()
	reach := map[uint32]bool{}
	var stack []Handle
	for _, t := range trees {
		stack = append(stack, Handle(t.c.root.Load()))
	}
	for len(stack) > 0 {
		h := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		if h == 0 || h&1 != 0 || reach[uint32(h>>1)] {
			continue
		}
		reach[uint32(h>>1)] = true
		if r := v.rec(h); r.childCount > 0 {
			stack = append(stack, v.kidSpan(r.childrenStart, r.childCount)...)
		}
	}
	n := 0
	for pg := range c.recs {
		lim := recPageLen
		if pg == c.pc.recPage {
			lim = c.pc.recSlot
		} else if pg == c.ec.recPage {
			lim = c.ec.recSlot
		}
		for i := 0; i < lim; i++ {
			idx := uint32(pg)<<recShift | uint32(i)
			if idx == 0 || v.refs[pg][i]&flagFree != 0 || reach[idx] {
				continue
			}
			n++
		}
	}
	return n
}

// LockLease and UnlockLease hold the store's lease, as a running parse does.
func LockLease(t *Tree)   { t.c.store.lease.Lock() }
func UnlockLease(t *Tree) { t.c.store.lease.Unlock() }

// Finalize runs the body of the finalizer of t's cell, on the calling goroutine.
func Finalize(t *Tree) { t.c.finalize() }

// Cells is the number of live cells of t's store (a running parse counts as one).
func Cells(t *Tree) int64 { return t.c.store.cells.Load() }

// Ended reports whether t's store has ended.
func Ended(t *Tree) bool { return t.c.store.ended.Load() }

// EndedFunc returns a function that reports the same, holding the store but
// not the tree or its cell.
func EndedFunc(t *Tree) func() bool {
	c := t.c.store
	return func() bool { return c.ended.Load() }
}

// Pending is the number of queued operations of t's store.
func Pending(t *Tree) int {
	c := t.c.store
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.pending)
}

// Tables of pages held by t's store: record, refs, child and state pages in
// the tables, the counts the store claims, and the bytes summed from the tables.
func PagesHeld(t *Tree) (recs, refs, kids, state, claimedRec, claimedKid, claimedState, bytes int) {
	c := t.c.store
	c.mu.Lock()
	defer c.mu.Unlock()
	bytes = len(c.recs)*recPageBytes() + len(c.kids)*kidPageLen*8
	for _, pg := range c.state {
		bytes += len(pg)
	}
	for _, l := range c.large {
		bytes += len(l.h) * 8
	}
	return len(c.recs), len(c.refs), len(c.kids), len(c.state), c.nextRec, c.nextKid, c.nextState, bytes
}

// PoolBytes is the number of bytes of pages in the pool.
func PoolBytes() int {
	pool.mu.Lock()
	defer pool.mu.Unlock()
	return pool.bytes
}

// SetPoolMaxBytes sets the bound of the pool and returns the old one.
func SetPoolMaxBytes(n int) int {
	pool.mu.Lock()
	defer pool.mu.Unlock()
	old := poolMaxBytes
	poolMaxBytes = n
	return old
}

// TrimPool runs the idle trim as of time now (nanoseconds since the epoch).
func TrimPool(now int64) { trimPool(now) }

// RecordFree reports whether the record of handle h is on the free list of the
// tree's store (the flag in its reference word).
func RecordFree(t *Tree, h Handle) bool {
	v := t.c.store.view() // takes the short mutex only: it can run while an Edit holds emu
	idx := uint32(h >> 1)
	return v.refs[idx>>recShift][idx&(recPageLen-1)]&flagFree != 0
}

// DuplicateAChildBeforeBalance makes the parser, before the balance step of
// its next parse, put one internal node of the finished tree into a second slot
// of its parent's child list (a tree that reaches a record twice). The handle
// of that node is stored where the returned pointer points. The hook turns
// itself off after one use.
func (p *Parser) DuplicateAChildBeforeBalance() *Handle {
	var target Handle
	p.preBalance = func() {
		p.preBalance = nil
		stack := []Handle{p.finished}
		for len(stack) > 0 {
			h := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			if h == 0 || h&1 != 0 {
				continue
			}
			kids := p.children(h)
			if len(kids) >= 2 {
				for i, k := range kids {
					if k&1 == 0 && k != 0 && p.childCount(k) > 0 {
						j := (i + 1) % len(kids)
						kids[j] = k
						target = k
						return
					}
				}
			}
			stack = append(stack, kids...)
		}
	}
	return &target
}

// RefCount is the number of owners beyond the first of the record h (a handle) in the store of t.
func RefCount(t *Tree, h Handle) uint32 {
	v := t.c.store.view()
	return v.refs[h>>(recShift+1)][(h>>1)&(recPageLen-1)] & countMask
}

// Poisoned reports whether the store of t is poisoned.
func Poisoned(t *Tree) bool { return t.c.store.poisoned.Load() }

// Exact setters of the limits of the store of t.
func SetRecLimit(t *Tree, n uint32)       { t.c.store.recLimit.Store(n) }
func SetCountLimit(t *Tree, n uint32)     { t.c.store.countLimit.Store(n) }
func SetKidPageLimit(t *Tree, n uint32)   { t.c.store.kidPageLimit.Store(n) }
func SetStatePageLimit(t *Tree, n uint64) { t.c.store.statePageLimit.Store(n) }

// SetTestLimits makes the stores this parser creates start with these limits.
func (p *Parser) SetTestLimits(rec, count, kidPages uint32, statePages uint64) {
	p.testLimits = &storeLimits{rec: rec, count: count, kidPages: kidPages, statePages: statePages}
}

// DrainNow applies the queued operations of the store of t (as a Release does).
func DrainNow(t *Tree) { t.c.store.tryDrain() }

// SameStore reports whether two trees live in one store.
func SameStore(a, b *Tree) bool { return a.c.store == b.c.store }

// Poison marks the store of t poisoned (as a failed drain does).
func Poison(t *Tree) { t.c.store.poison("injected") }

// NextRecIndex is the index the parse allocator would hand out next without a free list.
func NextRecIndex(t *Tree) uint32 {
	c := t.c.store
	return uint32(c.pc.recPage)<<recShift | uint32(c.pc.recSlot)
}

// EditRecIndex is the index the edit allocator would hand out next (when its
// free list is empty).
func EditRecIndex(t *Tree) uint32 {
	c := t.c.store
	c.mu.Lock()
	defer c.mu.Unlock()
	page, slot := c.ec.recPage, c.ec.recSlot
	if page < 0 || slot >= recPageLen {
		page, slot = len(c.recs), 0
	}
	return uint32(page)<<recShift | uint32(slot)
}

// FailureText is the reason the store of t was poisoned.
func FailureText(t *Tree) string { return t.c.store.failureText() }

// ViewPages is the number of page pointers (record, child, state pages and large
// lists) that the Tree object itself still holds.
func ViewPages(t *Tree) int {
	return len(t.rd.recs) + len(t.rd.refs) + len(t.rd.kids) + len(t.rd.state) + len(t.rd.large)
}

// StoreInlinePages is the number of page pointers that the store core of t still
// holds in its inline page-table arrays.
func StoreInlinePages(t *Tree) int {
	c := t.c.store
	c.mu.Lock()
	defer c.mu.Unlock()
	n := 0
	for i := range c.recsIn {
		if c.recsIn[i] != nil {
			n++
		}
		if c.refsIn[i] != nil {
			n++
		}
		if c.kidsIn[i] != nil {
			n++
		}
	}
	return n
}

// StoreRetained is the capacity of what the store core of t holds after the store ended and is not a page
// table: the pending and spare queues, the free-list state and the release stack.
func StoreRetained(t *Tree) (pending, spare, rel, walk, taken, freedRecs int) {
	c := t.c.store
	c.mu.Lock()
	defer c.mu.Unlock()
	return cap(c.pending), cap(c.spare), cap(c.rel), cap(c.walk), cap(c.taken), cap(c.freedRecs)
}

// ParserRetained counts the page pointers and record slices that the parser p holds
// between parses: the view of its last store, its handle and node slices, and its scratch.
func ParserRetained(p *Parser) (pages, handles int) {
	pages = len(p.recs) + len(p.refs) + len(p.kids) + len(p.state) + len(p.large)
	handles = len(p.nodes) + len(p.free) + len(p.heads) + len(p.scratch) + len(p.seen) + len(p.compressTmp)
	return
}
