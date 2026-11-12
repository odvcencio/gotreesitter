package ccore

// The tree cell and the public tree object (lifetime.go, steps 4, 6, 7 and 9;
// the drain attempts, the drain and the end of the store of steps 8, 10 and 11
// are in lifetime.go and store.go).
//
// A Tree is a handle on one root in a store. The Tree points to a cell; the
// cell holds the store, the generation, the root and the released flag, and
// has no pointer back to the Tree. A finalizer on the cell therefore runs
// when the Tree and any views of it become unreachable, even when they form a
// cycle. Go 1.22 is the language version of the module, so runtime.AddCleanup
// is not available and the finalizer is runtime.SetFinalizer on the cell.

import (
	"runtime"
	"sync/atomic"
)

// cell is the owner of one root: one count in the store's cell count.
type cell struct {
	store    *storeCore
	gen      uint64
	root     atomic.Uint64
	released atomic.Bool
	editing  bool // an Edit is running on this cell (guarded by store.mu); a release then leaves the queueing of its entry to the Edit's commit
	deferred bool // drop met a running Edit and did not queue the release of the root; Edit queues it at its commit (guarded by store.mu)
}

// Tree is a parse result: a root in a store. Readers of a published tree take
// no lock: the Tree keeps its own copy of the page table headers, which it
// refreshes only in Edit, and a page table only grows.
type Tree struct {
	c        *cell
	rd       Store
	lastEdit int // records the last Edit created
}

// newCell counts a new cell of the store (the caller holds the lease or a live cell of the store).
func (c *storeCore) newCell(root Handle) *cell {
	c.cells.Add(1)
	cl := &cell{store: c, gen: c.gen}
	cl.root.Store(uint64(root))
	runtime.SetFinalizer(cl, (*cell).finalize)
	return cl
}

// drop is the first half of Release and of the finalizer: in one section of
// the short mutex it tests and sets released, reads the root and queues the
// release of that root, except while an Edit of the same tree is running: then
// it only marks the release as deferred and the Edit queues it at its commit.
// Edit and Copy test and change the root in the same mutex, so a release sees
// either the root before an edit or the root after it, never one that the edit
// has already given up. It reports whether this call released the tree.
func (cl *cell) drop() bool {
	c := cl.store
	c.mu.Lock()
	defer c.mu.Unlock()
	if cl.released.Load() {
		return false
	}
	cl.released.Store(true)
	if cl.editing {
		// A running Edit reads the records of this root. Queueing their release
		// now could let a drain on another goroutine free them under the edit, so
		// the Edit queues the release when it commits.
		cl.deferred = true
		return true
	}
	c.pending = append(c.pending, op{kind: opRelease, h: Handle(cl.root.Load()), gen: cl.gen})
	return true
}

// finalize is step 9 of lifetime.go. It runs on the one finalizer goroutine of the
// process, so it never waits for a lease and walks nothing: it appends its
// entry; if no cell is left it ends the store inline when the lease is free
// (which hands the store's pages to the pool, page by page); otherwise it starts
// one goroutine that tries the drain. A drain that fails there poisons the store
// and returns quietly.
func (cl *cell) finalize() {
	if !cl.drop() {
		return
	}
	c := cl.store
	if c.cells.Add(-1) == 0 {
		c.endIfIdle()
		return
	}
	go c.tryDrain()
}

// endIfIdle ends the store when the lease is free and no cell is left. A held
// lease means a parse or a drain is running; its holder ends the store (the
// exit of a parse and every round of tryDrain test the cell count again).
func (c *storeCore) endIfIdle() {
	if !c.lease.TryLock() {
		return
	}
	defer c.lease.Unlock()
	if !c.ended.Load() && c.cells.Load() == 0 {
		c.endLocked()
	}
}

// release is steps 8 and 9 of lifetime.go: Release does what the finalizer does,
// then drains on the caller's goroutine.
func (cl *cell) release() {
	if !cl.drop() {
		return
	}
	runtime.SetFinalizer(cl, nil)
	c := cl.store
	c.cells.Add(-1)
	c.tryDrain()
}

// Root is the root handle of the tree (the edited root after an Edit); 0 for a
// released tree, whose pages may be free or pooled.
func (t *Tree) Root() Handle {
	if t.c.released.Load() {
		return 0
	}
	return Handle(t.c.root.Load())
}

// emptyStore is what Reader returns for a released tree: it has no pages, and
// Root is 0, so a walker that starts at the root finds nothing.
var emptyStore Store

// Reader returns a read-only view of the tree's store.
//
// Reader contract (the same as C's for a TSTree): one Tree value may be read by
// many goroutines at the same time (Reader, Root, the accessors of a view,
// Digest and the other readers of Parser). Edit and Release of a Tree value need
// exclusive use of that value: no goroutine may read it while another edits or
// releases it. A goroutine that wants to read while another edits takes its own
// Copy and reads that. The returned view keeps the tree's cell reachable, also
// when the caller copies the Store value, so the cell's finalizer cannot run
// while a view exists. A view, the handles read through it and every slice its
// accessors returned are valid until the owning tree is edited or released; a
// caller that needs them longer reads an unedited Copy. No lock is taken on the
// read path.
//
// For a released tree it returns an empty store: its accessors that index a
// page (Symbol, Size, Children of a heap handle) panic, so a walker must test
// the tree's Root first (Root is 0 for a released tree) and not walk from it;
// accessors that decode an inline handle, and ExtState(0), do not index pages.
func (t *Tree) Reader() *Store {
	if t.c.released.Load() {
		return &emptyStore
	}
	return &t.rd
}

// StoreFailure is the reason the store of the tree was poisoned (a panic or a
// limit refusal in a publication, a sweep, a drain or an edit), or "" if it was
// not. A poisoned store frees nothing piecemeal; it is a failure that no
// caller could receive (an exit drain, a finalizer), so tests and gates ask.
func (t *Tree) StoreFailure() string {
	if !t.c.store.poisoned.Load() {
		return ""
	}
	return t.c.store.failureText()
}

// Released reports whether the tree has been released.
func (t *Tree) Released() bool { return t.c.released.Load() }

// LastEditRecords is the number of records the last Edit of this tree created.
func (t *Tree) LastEditRecords() int { return t.lastEdit }

// Release is C ts_tree_delete (C/tree.c): it gives up the tree's ownership of
// its root, and clears the Tree's view of the store's pages (once no Edit of
// this Tree is running; a running Edit clears it at its commit), so that a
// released Tree that stays reachable holds no page. A second call does nothing.
//
// Release needs exclusive use of this Tree value, like Edit: no goroutine may
// read it (Reader, Root and the accessors of a view taken from it) while it is
// released. A goroutine that reads while another goroutine edits or releases
// takes its own Copy first and reads that.
func (t *Tree) Release() {
	c := t.c.store
	defer func() { // also when the release panics in a place that is not guarded
		c.mu.Lock()
		if t.c.released.Load() && !t.c.editing {
			t.rd = Store{}
		}
		c.mu.Unlock()
	}()
	t.c.release()
}

// Copy returns a tree with the same root (C ts_tree_copy). It queues one
// retain of the root. Copy may run while other goroutines read this Tree value
// (the reader contract is at Reader); it is the way to get a Tree of one's own
// for reading while another goroutine edits or releases this one. C also copies the included ranges because Edit rewrites
// them (C/tree.c:21-22, 98-100); this core keeps no included ranges.
func (t *Tree) Copy() *Tree {
	c := t.c.store
	c.mu.Lock() // the test of released, the read of the root and the queued retain are one section
	defer c.mu.Unlock()
	if t.c.released.Load() {
		return nil
	}
	root := Handle(t.c.root.Load())
	n := &Tree{c: c.newCell(root), rd: t.rd}
	n.rd.owner = n.c
	n.c.gen = t.c.gen
	c.pending = append(c.pending, op{kind: opRetain, h: root, gen: t.c.gen})
	runtime.KeepAlive(t) // t's finalizer must not run before the retain is queued
	return n
}

// Edit is C ts_tree_edit (lifetime.go, step 6). It copies the records on the edit
// path into the edit allocator's pages, queues the retains and the release of
// the old root, and sets the cell's root. It takes no lease and drains
// nothing (but see the last case below); its cost is the path length. Records published earlier are not
// written.
//
// Edit needs exclusive use of this Tree value (see Reader); the store-level
// handling below keeps the store consistent if a Release of the same Tree
// overlaps, but readers of this Tree value are not protected from the Edit.
//
// A running Edit holds one count in the store's cell count and marks the cell
// as editing, so that a Release of the same tree on another goroutine neither
// ends the store (and hands its pages to another document) nor queues the
// release of the root the edit is reading; the Edit does both at its commit.
// If a Release ran meanwhile, the Edit queues the release of the edit's new root
// and of the old root at its commit and drains them itself, once, after it has
// let go of its locks.
func (t *Tree) Edit(e Edit) {
	c := t.c.store
	var counted, releasedMeanwhile bool
	// The edit's count is lowered after emu is let go: ending the store or
	// draining takes the lease, which comes before emu in the lock order. The
	// defer also runs when the edit panics, so that a panic leaves no count behind.
	defer func() {
		if !counted {
			return
		}
		if c.cells.Add(-1) == 0 { // the edit held the last count of a store whose trees were all released
			c.endIfIdle()
		} else if releasedMeanwhile {
			c.tryDrain() // the only case in which an Edit drains: it queued entries that no other release or parse is sure to ask for
		}
	}()
	t.edit(e, &counted, &releasedMeanwhile)
}

// edit does the work of Edit under emu. It sets *counted once the edit holds
// its count in the cell count (the caller lowers it, also after a panic), and
// *releasedMeanwhile when a Release of the tree ran during the edit. If the
// edit stops before its commit (a panic in the hook or in subtreeEdit), it
// clears the editing mark and, when a Release was deferred, queues the release
// of the old root, which the tree still holds. Records that a panicking
// subtreeEdit had already made are not freed.
func (t *Tree) edit(e Edit, counted, releasedMeanwhile *bool) {
	c := t.c.store
	cl := t.c
	c.emu.Lock()
	defer c.emu.Unlock()
	c.mu.Lock()
	if cl.released.Load() { // a released tree owns nothing: editing it would queue a release of a root that others own
		c.mu.Unlock()
		return
	}
	cl.editing = true
	c.cells.Add(1)
	*counted = true
	c.mu.Unlock()
	committed, armed := false, false
	defer func() {
		if committed {
			return
		}
		r := recover()
		if armed && r != nil {
			// The edit failed after it began to allocate and before its new records
			// were owned and published: the records it made (and the child lists or
			// state bytes they own) are not tracked, so the store is poisoned; its
			// pages return when its last tree goes. The failure goes on to the
			// caller of Edit (a panic of its own is not turned into a result: Edit
			// has none).
			c.poison(r)
		}
		c.mu.Lock()
		cl.editing = false
		if cl.deferred {
			cl.deferred = false
			c.pending = append(c.pending, op{kind: opRelease, h: Handle(cl.root.Load()), gen: cl.gen})
			*releasedMeanwhile = true
		}
		if cl.released.Load() {
			t.rd = Store{}
		}
		c.mu.Unlock()
		if r != nil {
			panic(r)
		}
	}()
	old := Handle(cl.root.Load()) // only Edit changes the root, and emu serializes Edits
	s := &t.rd
	c.mu.Lock() // the edit allocator's current pages may have been added after this tree's headers were taken
	s.syncLocked()
	c.mu.Unlock()
	c.fire("edit")
	armed = true // from the first allocation until the commit has published the ownership, a failure poisons the store
	root, created := s.subtreeEdit(&editor{s: s, edit: true}, old, editRange{
		start:  Length{bytes: e.StartByte, row: e.StartRow, col: e.StartCol},
		oldEnd: Length{bytes: e.OldEndByte, row: e.OldEndRow, col: e.OldEndCol},
		newEnd: Length{bytes: e.NewEndByte, row: e.NewEndRow, col: e.NewEndCol},
	})
	t.lastEdit = len(created)
	var ops []op
	if root != old {
		c.fire("edit-retain")
		ops = s.retainEdited(created, cl.gen)
	}
	t.commitEdit(old, root, ops, releasedMeanwhile, &committed)
}

// commitEdit is the end of an edit: in one section of the short mutex it queues
// the operations, publishes the new root and clears the editing mark. The unlock
// is deferred, so a failure inside leaves the mutex free; *committed is set last,
// after everything is published, so a failure before that point is handled as a
// failed edit (poisoned store, editing mark cleared) by the caller's defer.
func (t *Tree) commitEdit(old, root Handle, ops []op, releasedMeanwhile, committed *bool) {
	c, cl, s := t.c.store, t.c, &t.rd
	c.mu.Lock()
	defer c.mu.Unlock()
	c.fire("edit-commit")
	meanwhile := cl.released.Load()
	switch {
	case root == old: // nothing changed
		if cl.deferred { // released while the edit ran: the release of the root that drop deferred
			ops = append(ops, op{kind: opRelease, h: old, gen: cl.gen})
		}
	case meanwhile: // the edit's new root is thrown away: release it (this frees its path again), and the old one
		ops = append(ops, op{kind: opRelease, h: root, gen: cl.gen}, op{kind: opRelease, h: old, gen: cl.gen})
	default:
		ops = append(ops, op{kind: opRelease, h: old, gen: cl.gen})
	}
	c.pending = append(c.pending, ops...)
	s.syncLocked()
	if root != old && !meanwhile {
		cl.root.Store(uint64(root))
	}
	if meanwhile {
		t.rd = Store{} // the Release that ran meanwhile left the view to the edit
	}
	cl.editing = false
	cl.deferred = false
	*releasedMeanwhile = meanwhile
	*committed = true
}
