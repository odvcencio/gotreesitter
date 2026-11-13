package ccore

// Reference counts and the lifetime of a tree.
//
// The protocol as implemented, in the order it runs (other files refer to these
// steps as "lifetime.go, step N"). Failures are described after the list.
//  0. Start of a parse: ParseWith (api.go) rejects a released old tree, picks the
//     store (a new store when old is nil or its store is poisoned), takes the
//     lease and checks again that the store is not poisoned, then drains the
//     queue under the lease (step 10) and captures the old tree's released flag,
//     root and page-table headers together under the short mutex.
//  1. Allocation: a running parse allocates records with Store.add (subtree.go).
//     A record that owns storage (a child list, scanner-state bytes) is claimed
//     first (Store.claim: the slot is taken, checked against the record limit,
//     and left empty), then its storage is allocated, then the record is filled
//     (Store.fill). No other allocator can take the slot in between, and a
//     refusal of the storage leaves an empty record that the abort sweep frees,
//     never storage that no record owns. In an incremental parse each record gets the new flag; a
//     fresh parse sets no flag and runs neither a publication walk nor a sweep,
//     so what it allocated and the finished tree does not reach (a replaced
//     root, discarded error leaves) stays in the store until the store ends.
//     Nothing is counted during a parse.
//  2. Heap leaves: a cached heap token is handed out once and every later
//     hand-out is a copy made by Store.copyHeapLeaf (subtree.go; called from the
//     parse loop in parser.go). setSymbol (the keyword switch) and setExtra make
//     a mutable copy of every heap leaf they change. So a leaf sits in one slot
//     of a tree. Before the balance step, every parse that finished a tree walks it
//     for new records that it reaches more than once (findMulti, subtree.go; a fresh
//     parse keeps the marks of the walk in a bit set, one bit per record slot, and
//     an incremental parse in the flags word of the records it visits; both give
//     the same counts); the
//     balance step treats them as shared, and a fresh parse, which has no
//     publication walk, adds the extra owners to the reference words
//     (countFreshShared).
//  3. Publication (incremental parses): after the parse and the balance step,
//     Store.publish (store.go) walks the new tree, counts the owners it adds and
//     clears the flags; a sweep frees what the parse allocated and the tree does
//     not reach. This happens before the tree has a cell.
//  4. Trees: the parse then creates the tree's cell, one count in the store's
//     cell count (newCell, tree.go).
//  5. One exit: ParseWith registers endParse (api.go) directly after taking the
//     lease. Its defers run in this order: a parse that ends without a tree
//     sweeps what it allocated (incremental parses only); the parser's handles
//     and views are cleared; the parse's count is lowered (the store ends if it
//     reaches zero); the lease is released; afterParse (this file) then applies
//     what arrived meanwhile. When other trees are alive the exit also drains
//     once under the lease, before the handles are cleared.
//  6. Edit: Tree.Edit (tree.go) copies the records on the edit path into the edit
//     allocator's pages, queues the retains of what the new records share and the
//     release of the old root. A running Edit holds one count in the cell count
//     and marks its cell as editing. A Release of that tree meanwhile sets the
//     released flag and counts the cell down but queues nothing; the Edit, at its
//     commit, queues the release of the old root and, when the tree was released,
//     of its own new root too, and lowers its count (ending the store if it
//     reaches zero); if a Release ran meanwhile it also drains once. If the Edit
//     panics before it allocates, it clears the mark, queues the deferred
//     release of the old root and lowers its count. If it fails after its first
//     allocation and before its commit has published the new root and queued the
//     operations (the commit sets "committed" last, under a deferred unlock), the
//     store is poisoned as well, because the records it made are not tracked.
//  7. Copy: Tree.Copy (tree.go) creates and counts a new cell for the same root
//     (one more count in the cell count) and queues one retain of the root.
//  8. Drain attempt: tryDrain and drainRound (this file) take the lease without
//     waiting; they end the store when no tree and no parse is left, otherwise
//     they apply one snapshot of the queue.
//  9. Release and finalizer: Tree.Release (cell.release, tree.go) queues the
//     release of the root (unless an Edit of the same tree is running, step 6),
//     counts the cell down, drains on the caller's goroutine and clears the
//     Tree's view. The finalizer (cell.finalize) queues the same entry and
//     counts the cell down; if that was the last cell it ends the store itself
//     when the lease is free (the pages go to the pool, page by page), otherwise
//     it starts one goroutine that tries the drain.
// 10. Drain: Store.drain (this file) applies one snapshot of the queued operations
//     in list order under the lease.
// 11. Store end: endLocked (store.go) drops the pages whole when the cell count is
//     zero.
//
// Failures. A panic or a limit refusal inside a publication walk, a sweep or a
// drain poisons the store (poisonOnPanic, store.go) and is swallowed there: it is
// never re-thrown from a place without a caller. ParseWith turns it into
// (nil, stop) with the reason; a drain that runs in Release, in a finalizer's
// goroutine or in the exit of a parse returns quietly. A poisoned store frees
// nothing piecemeal again: its drains discard their queue, a parse of one of its
// trees parses afresh in a new store, and its pages go to the pool when its last
// tree is released or dropped (the end check runs after every drain, so a store
// whose last owner left during a failed drain still ends). A refusal inside a
// parse's own allocations (the record-index, child-page or state-page limit)
// stops that parse; the abort sweep frees its records. The shared page-growth
// limits (record pages, child pages, state pages) are checked and the page
// claimed in one section of the short mutex, so a parse and an edit of separate
// trees cannot both pass one limit. The cursor within a page and the parse's
// free lists belong to the lease holder; an edit's cursor belongs to the edited
// tree (one Edit at a time) and its pops from the shared free lists run under
// the short mutex.
//
// Memory. The records of a fresh parse that the finished tree does not reach (a
// replaced root, discarded error leaves) and the storage they own stay in the
// store until the whole store ends: releasing the first tree, editing it and
// reparsing does not reclaim them. A later incremental parse sweeps what it
// allocated and the tree does not reach, so a new set of such records does not
// build up on every reparse. The share of retained records can rise as the
// document shrinks.
//
// Counts are kept in the side array refs, one word per record: the number of
// owners beyond the first. They change only when a tree is published (store.go
// publish), edited, copied or released. A change of ownership that happens
// away from the lease is queued as an operation and applied in list order by
// whoever holds the lease.
//
// C functions this file ports in spirit:
//   C/subtree.c:578-605  ts_subtree_release  -> releaseWalk (count 0: free the record, its child list and its state bytes, then each heap child)
//   C/subtree.c:  ts_subtree_retain          -> opRetain

type opKind uint8

const (
	opRetain opKind = iota
	opRelease
)

// op is one queued change of ownership.
type op struct {
	kind opKind
	h    Handle
	gen  uint64 // the generation of the store the operation was made for
}

// appendOps queues operations in order (guarded by the short mutex).
func (c *storeCore) appendOps(ops ...op) {
	c.mu.Lock()
	c.pending = append(c.pending, ops...)
	c.mu.Unlock()
}

func (c *storeCore) hasPending() bool {
	c.mu.Lock()
	n := len(c.pending)
	c.mu.Unlock()
	return n > 0
}

// drain applies one snapshot of the queued operations in list order
// (lifetime.go, step 10). The caller holds the lease. Operations that arrive
// while it runs stay queued: a drain holds the lease for a bounded amount of work, and the
// caller (tryDrain, the exit of a parse) tests again after it has released the
// lease. If a panic stops the drain before an operation starts, the unapplied
// tail goes back to the front of the queue, in order; the failure poisons the
// store (poisonOnPanic), so the next drain discards that tail instead of
// applying it, and the counts it would have changed stay as they are. An
// operation that panics while it runs is not put back: it may have freed
// records already, and applying it again could free one twice.
func (s *Store) drain() {
	c := s.core
	defer c.poisonOnPanic()
	if c.poisoned.Load() { // a poisoned store frees nothing piecemeal; its queue is dropped
		c.mu.Lock()
		c.pending = c.pending[:0]
		c.mu.Unlock()
		return
	}
	c.mu.Lock()
	ops := c.pending
	c.pending, c.spare = c.spare[:0], nil
	s.syncLocked()
	c.mu.Unlock()
	if len(ops) == 0 {
		c.mu.Lock()
		if c.spare == nil { // keep the capacity of the empty buffer: the next drain would otherwise leave the queue without a spare
			c.spare = ops[:0]
		}
		c.mu.Unlock()
		return
	}
	done := 0
	defer func() {
		if done < len(ops) {
			c.mu.Lock()
			c.pending = append(append([]op(nil), ops[done:]...), c.pending...)
			c.mu.Unlock()
		}
	}()
	for i, o := range ops {
		if o.gen != c.gen {
			done = i + 1
			continue
		}
		c.fire("drain")
		done = i + 1
		switch o.kind {
		case opRetain:
			s.retain(o.h)
		case opRelease:
			s.releaseWalk(o.h)
		}
	}
	s.flushFree()
	c.mu.Lock()
	c.spare = ops[:0]
	c.mu.Unlock()
}

// retain is C/subtree.c ts_subtree_retain: one more owner of a heap record.
func (s *Store) retain(h Handle) {
	if h == 0 || h&1 != 0 {
		return
	}
	w := &s.refs[h>>(recShift+1)][(h>>1)&recMask]
	if *w&flagFree != 0 {
		counters.badRetain.Add(1)
		return
	}
	if *w&countMask >= s.core.countLimit.Load() {
		s.core.limitPanic("reference count limit reached")
	}
	*w++
}

// releaseWalk is C/subtree.c:578-605 ts_subtree_release: a record with count 0
// is freed with its child list and state bytes, and the walk goes on to each
// heap child; a record with a count above 0 loses one and the walk stops
// there. A walk that reaches a record already on the free list stops there
// and counts a double free.
func (s *Store) releaseWalk(root Handle) {
	c := s.core
	hk := c.hook.Load()
	stack := append(c.rel[:0], root)
	for len(stack) > 0 {
		h := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		if h == 0 || h&1 != 0 {
			continue
		}
		if hk != nil {
			(*hk)("release-record")
		}
		w := &s.refs[h>>(recShift+1)][(h>>1)&recMask]
		if *w&flagFree != 0 {
			counters.double.Add(1)
			continue
		}
		if *w&countMask != 0 {
			*w--
			continue
		}
		r := s.rec(h)
		if r.childCount > 0 {
			stack = append(stack, s.kidSpan(r.childrenStart, r.childCount)...)
		}
		s.freeRecord(uint32(h>>1), r, w)
	}
	c.rel = stack[:0]
}

// drainRound is one hold of the lease without waiting (lifetime.go, step 8): it
// ends the store if no tree and no parse is left, else it applies one snapshot
// of the queue. It reports whether it got the lease. The lease is released
// under defer, so a panic inside the drain does not keep it.
func (c *storeCore) drainRound() bool {
	if !c.lease.TryLock() {
		return false
	}
	defer c.lease.Unlock()
	if c.ended.Load() {
		return true
	}
	if c.cells.Load() == 0 { // no tree and no parse: the pages go whole, the queued operations are moot
		c.endLocked()
		return true
	}
	v := c.view()
	v.drain()
	return true
}

// more reports whether someone must look at the store again after the lease
// was released: an entry is queued, or the last tree is gone and the store has
// not ended (a release that failed TryLock while another drain held the lease
// left the end to that drain).
func (c *storeCore) more() bool {
	if c.ended.Load() {
		return false
	}
	return c.cells.Load() == 0 || c.hasPending()
}

// tryDrain applies the queued operations if no parse holds the lease; if a
// parse holds it, the entries stay for that parse. After each round it tests
// again with the lease released (a round that fails to get the lease, or an entry that arrives during a round, is not lost).
func (c *storeCore) tryDrain() {
	for c.drainRound() && c.more() {
	}
}

// afterParse runs once the exit of a parse has released the lease: an entry
// that arrived during the parse, or the end of the store, is dealt with here.
func (c *storeCore) afterParse() {
	if c.more() {
		c.tryDrain()
	}
}

// retainEdited queues a retain for every child of a record the edit created
// that the edit did not create itself (lifetime.go, step 6).
func (s *Store) retainEdited(created []Handle, gen uint64) []op {
	made := make(map[Handle]struct{}, len(created))
	for _, h := range created {
		made[h] = struct{}{}
	}
	var ops []op
	for _, h := range created {
		for _, k := range s.children(h) {
			if k == 0 || k&1 != 0 {
				continue
			}
			if _, ok := made[k]; !ok {
				ops = append(ops, op{kind: opRetain, h: k, gen: gen})
			}
		}
	}
	return ops
}
