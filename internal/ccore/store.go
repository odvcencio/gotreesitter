package ccore

// The store: fixed pages of records, child lists and scanner state bytes, and
// a side array of reference counts (the protocol is described at the head of
// lifetime.go). Pages never move, so a *rec or a []Handle taken from a page
// stays valid while the page lives. A page table only grows.
//
// Differences from C that this file introduces, semantics kept:
//   - C allocates every subtree with malloc/free and counts owners in each
//     subtree (C/subtree.c ts_subtree_retain, ts_subtree_release). Here the
//     parse loop counts nothing; counts are changed only when a tree is
//     published, edited, copied or released (see lifetime.go).

import (
	"fmt"
	"math/bits"
	"reflect"
	"sync"
	"sync/atomic"
	"time"
)

const (
	recShift   = 12 // records per page: 4,096
	recPageLen = 1 << recShift
	recMask    = recPageLen - 1
	kidShift   = 12 // handles per child page: 4,096
	kidPageLen = 1 << kidShift
	kidMask    = kidPageLen - 1
	stShift    = 16 // bytes per state page: 65,536
	stPageLen  = 1 << stShift
	stMask     = stPageLen - 1

	largeBit uint32 = 1 << 31 // childrenStart of a list that has a page of its own

	inlinePages = 4 // page-table slots kept inside the store (see storeCore.recsIn)

	numKidClasses = 17 // exact sizes 1 to 8, then 16 to 4,096
	numStClasses  = 10 // 8, 16, ... 4,096 bytes
)

// Bits of a word of the side array refs. The low bits count the owners of a
// record beyond the first (0 means one owner, as a fresh node in C has
// ref_count 1). While a record is free, the low bits hold the next free index.
//
// The flags new and mark could live in refs too. They live in the
// record's own flags word (heapNew, heapMark in handle.go) because the
// balance step tests new for every node it visits and the publication walk
// marks every new record, and a flag in the side array costs one more cache
// line per node (measured: the balance step ran 35 percent
// longer). Both are cleared in the sweep, before the tree is published, so a
// published record never carries them and is never written.
//
// flagEdit marks a record, child list or state block that the edit allocator
// made. They are freed to the edit allocator's own free lists (guarded by the
// short mutex), because Tree.Edit never takes the lease and so cannot use the
// lease's lists; without this, every edit would add its path records to the
// store for good (measured on the Python chain: 14 records a step, 3.6 times the
// fresh store after 10,000 edits).
const (
	flagFree  uint32 = 1 << 29 // on a free list
	flagEdit  uint32 = 1 << 28 // made by the edit allocator
	countMask uint32 = flagEdit - 1
)

var recBytes = int(reflect.TypeOf(rec{}).Size())

// arenaCur is one allocator's position: the page it fills and the next free
// position in it. The parse and the edit allocator each own one.
type arenaCur struct {
	recPage, recSlot int
	kidPage, kidOff  int
	stPage, stOff    int
}

func newArenaCur() arenaCur {
	return arenaCur{recPage: -1, recSlot: recPageLen, kidPage: -1, kidOff: kidPageLen, stPage: -1, stOff: stPageLen}
}

// span is a run of record slots a parse took by bumping.
type span struct{ page, first, last int }

// storeCore is the part of a store that all views share.
type storeCore struct {
	parseTables *Tables    // table identity, read and cleared under lease
	mu          sync.Mutex // short mutex: page tables, pending operations
	emu         sync.Mutex // edit allocator: serializes Tree.Edit and guards ec
	// page tables (guarded by mu for growth; views hold copies of the headers)
	recs      []*[recPageLen]rec
	refs      []*[recPageLen]uint32
	kids      []*[kidPageLen]Handle
	large     []*largeList
	state     [][]byte
	freeLarge []uint32

	// allocator state of the parse (guarded by the lease)
	pc      arenaCur
	ec      arenaCur // the edit allocator's position (guarded by emu)
	freeRec uint32
	freeKid [numKidClasses]uint32
	freeSt  [numStClasses]uint32

	// Records and child lists freed in one batch (a sweep or a drain) wait here
	// and join the free lists in ascending address order (flushFree), so that
	// the next parse allocates them in ascending order, as a bump allocator would.
	freedRecs []uint32
	sortTmp   []uint32

	// The same for what the edit allocator made. The free lists are guarded
	// by mu (Tree.Edit pops from them without the lease); the batches by the lease.
	editFreeRec   uint32
	editFreeKid   [numKidClasses]uint32
	editFreeSt    [numStClasses]uint32
	freedEditRecs []uint32
	freedEditSt   [numStClasses][]uint32

	// allocation log of the running parse (guarded by the lease)
	spans     []span
	spanFirst int
	taken     []uint32
	walk      []Handle
	lease     sync.Mutex // held by a parse from its start to its exit

	// ownership: operations queued away from the lease (guarded by mu)
	pending []op
	spare   []op
	gen     uint64
	rel     []Handle // release walk stack (lease)

	// The first page-table slots live in the store itself, so a small parse
	// allocates no table; a larger one takes a table sized by presize.
	recsIn [inlinePages]*[recPageLen]rec
	refsIn [inlinePages]*[recPageLen]uint32
	kidsIn [inlinePages]*[kidPageLen]Handle
	// pages claimed so far (tables may hold more pages, taken from the pool)
	nextRec, nextKid, nextState int

	// cells counts the live tree cells; a running parse counts as one more.
	cells    atomic.Int64
	ended    atomic.Bool // set under the lease when the store has ended
	poisoned atomic.Bool // a panic or a limit refusal stopped a publication walk, a sweep, a drain or an edit (see poisonOnPanic)
	failure  atomic.Pointer[string]

	recLimit, countLimit, kidPageLimit atomic.Uint32
	statePageLimit                     atomic.Uint64

	hook atomic.Pointer[hookFn] // test hook (see fire)
}

// hookFn is a test hook: it is called at named points inside a drain, a sweep,
// a publication walk and a lease-holding statistic ("drain", "sweep",
// "publish", "hold"). A test installs one on one store core, so a drain that a
// finalizer started on another store never sees it. It is nil in production.
type hookFn func(point string)

// A store is poisoned when a panic stops a publication walk, a sweep or a drain:
// those leave counts raised or records unfreed that nothing records. A poisoned
// store frees nothing piecemeal again; a parse that is given a tree of it
// parses afresh in a new store; its pages go back to the pool when its last
// tree is released or dropped.
//
// poisonOnPanic is deferred by publish, sweep and drain: when the function
// panics (an injected failure, or a limit refusal) it marks the store poisoned,
// records the reason and swallows the panic. A caller that has a result to give
// (ParseWith) reads the reason with failure; a drain that runs in the background
// has no caller and simply returns, so nothing in the store can terminate the
// process.
func (c *storeCore) poisonOnPanic() {
	if r := recover(); r != nil {
		c.poison(r)
	}
}

// poison marks the store poisoned and keeps the first reason.
func (c *storeCore) poison(r any) {
	reason := "store failure: "
	if le, ok := r.(*limitError); ok {
		reason += le.Error()
	} else {
		reason += fmt.Sprint("panic: ", r)
	}
	c.failure.CompareAndSwap(nil, &reason)
	c.poisoned.Store(true)
}

// failureText is the reason the store was poisoned.
func (c *storeCore) failureText() string {
	if p := c.failure.Load(); p != nil {
		return *p
	}
	return "store failure"
}

// limitError is the panic value of a store limit: the record index or the
// count of owners would not fit the 28 bits that the reference word gives them
// (a free-list link and a count share them with the flags). ParseWith turns it
// into a stop text; Tree.Edit passes it on.
type limitError struct{ what string }

func (e *limitError) Error() string { return "ccore: " + e.what }

// The limits of a store; they are fields of the store (the defaults are the
// largest values that the encodings can hold) so that a test can lower them for
// one store without touching any other.
const (
	defaultRecLimit       = countMask                   // the highest record index that may be handed out
	defaultCountLimit     = countMask                   // the highest number of owners beyond the first
	defaultKidPageLimit   = 1 << (31 - kidShift)        // child pages whose start offset does not reach largeBit
	defaultStatePageLimit = uint64(1) << (32 - stShift) // state pages whose offset fits 32 bits
)

// storeLimits are lowered limits that a Parser gives to the stores it creates
// (tests only; nil in production).
type storeLimits struct {
	rec, count, kidPages uint32
	statePages           uint64
}

func (p *Parser) applyTestLimits(c *storeCore) {
	if l := p.testLimits; l != nil {
		c.recLimit.Store(l.rec)
		c.countLimit.Store(l.count)
		c.kidPageLimit.Store(l.kidPages)
		c.statePageLimit.Store(l.statePages)
	}
}

func (c *storeCore) limitPanic(what string) { panic(&limitError{what}) }

// fire calls the store's test hook, if there is one.
func (c *storeCore) fire(point string) {
	if f := c.hook.Load(); f != nil {
		(*f)(point)
	}
}

// counters are the integrity counters of every store in the process.
var counters struct {
	shared    atomic.Int64 // new records met in two slots of one tree at publication
	double    atomic.Int64 // releases that reached a record already on the free list
	badRetain atomic.Int64 // retains that reached a record already on the free list
}

func newStoreCore() *storeCore {
	c := &storeCore{pc: newArenaCur(), ec: newArenaCur()}
	c.recs, c.refs, c.kids = c.recsIn[:0:inlinePages], c.refsIn[:0:inlinePages], c.kidsIn[:0:inlinePages]
	c.recLimit.Store(defaultRecLimit)
	c.countLimit.Store(defaultCountLimit)
	c.kidPageLimit.Store(defaultKidPageLimit)
	c.statePageLimit.Store(defaultStatePageLimit)
	return c
}

// presize gives the page tables of a store that has no page yet room for
// recPages record pages and kidPages child pages, so that the tables do not
// grow by doubling (one allocation each instead of one per doubling). The
// tables are the same slices append would have made; only their capacity
// differs. The caller holds the lease of a new store.
func (c *storeCore) presize(recPages, kidPages int) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if len(c.recs) != 0 || len(c.kids) != 0 {
		return
	}
	if recPages > inlinePages {
		c.recs = make([]*[recPageLen]rec, 0, recPages)
		c.refs = make([]*[recPageLen]uint32, 0, recPages)
	}
	if kidPages > inlinePages {
		c.kids = make([]*[kidPageLen]Handle, 0, kidPages)
	}
}

// Store is a reader's and a writer's view of a storeCore: copies of the page
// table headers. Every read and write of a record or a child list goes
// through its methods.
// largeList is a child list longer than a page.
type largeList struct{ h []Handle }

type Store struct {
	recs  []*[recPageLen]rec
	refs  []*[recPageLen]uint32
	kids  []*[kidPageLen]Handle
	large []*largeList // lists longer than a page; the pointer is stable, so freeing a list clears what every snapshot of the table sees
	state [][]byte
	core  *storeCore
	// owner is the cell of the tree this view belongs to (nil for the parser's
	// own view). A copy of the view keeps the cell reachable, so the cell's
	// finalizer cannot run while a view of its tree exists.
	owner *cell

	fresh   bool // the running parse is the first of a new store: every record is new, nothing is logged
	logging bool // the running parse logs its allocations and sets the new bit

	nLeaf, nNode, nCopy int // records the running parse allocated, by source (statistics)
}

func (c *storeCore) view() Store {
	c.mu.Lock()
	defer c.mu.Unlock()
	return Store{recs: c.recs, refs: c.refs, kids: c.kids, large: c.large, state: c.state, core: c}
}

// syncLocked refreshes the headers of s (c.mu held).
func (s *Store) syncLocked() {
	c := s.core
	s.recs, s.refs, s.kids, s.large, s.state = c.recs, c.refs, c.kids, c.large, c.state
}

func (s *Store) recAt(idx uint32) *rec { return &s.recs[idx>>recShift][idx&recMask] }

func (s *Store) rec(h Handle) *rec { return &s.recs[h>>(recShift+1)][(h>>1)&recMask] }

// growRec adds a record page and its refs page, and points cur at it. The
// pages come from the pool one at a time, so a store holds exactly the pages
// it has claimed.
func (s *Store) growRec(cur *arenaCur) {
	c := s.core
	pg, rf := takeRecPage()
	if pg == nil {
		pg, rf = new([recPageLen]rec), new([recPageLen]uint32)
	} else {
		clear(rf[:]) // a page of the pool: records are overwritten on use, flags must start at zero
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	// The limit is checked and the page claimed in this one section: the
	// allocators of a parse and of an edit both grow the tables, so a check
	// outside it could pass for both and the second page would pass the limit.
	// A page taken from the pool above and refused here is left to the collector.
	if uint64(len(c.recs))<<recShift > uint64(c.recLimit.Load()) { // the first index of the new page is over the limit
		c.limitPanic("record index limit reached")
	}
	first := len(c.recs) == 0
	c.recs = append(c.recs, pg)
	c.refs = append(c.refs, rf)
	cur.recPage, cur.recSlot = len(c.recs)-1, 0
	c.nextRec = len(c.recs)
	if first {
		c.recs[0][0] = rec{} // index 0 is Handle 0, C's NULL_SUBTREE: an empty record
		cur.recSlot = 1
	}
	s.syncLocked()
}

func (s *Store) growKids(cur *arenaCur) {
	c := s.core
	pg := takeKidPage() // a page of the pool: list contents are written before they are read
	if pg == nil {
		pg = new([kidPageLen]Handle)
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if uint32(len(c.kids)) >= c.kidPageLimit.Load() { // check and claim in one section (see growRec): the start offset would reach largeBit and be read as a large-list index
		c.limitPanic("child page limit reached")
	}
	first := len(c.kids) == 0
	c.kids = append(c.kids, pg)
	cur.kidPage, cur.kidOff = len(c.kids)-1, 0
	c.nextKid = len(c.kids)
	if first {
		cur.kidOff = 1 // start 0 means no list
	}
	s.syncLocked()
}

func (s *Store) growState(cur *arenaCur) {
	c := s.core
	pg := takeStatePage() // a page of the pool: state bytes are written before they are read
	if pg == nil {
		pg = make([]byte, stPageLen)
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if uint64(len(c.state)) >= c.statePageLimit.Load() { // check and claim in one section: the offset would wrap past 32 bits
		c.limitPanic("scanner state page limit reached")
	}
	first := len(c.state) == 0
	c.state = append(c.state, pg)
	cur.stPage, cur.stOff = len(c.state)-1, 0
	c.nextState = len(c.state)
	if first {
		cur.stOff = 8 // offset 0 means no bytes
	}
	s.syncLocked()
}

// kidClass returns the size class of a child list of n handles (n >= 1, n <= kidPageLen).
func kidClass(n uint32) (class int, size uint32) {
	if n <= 8 {
		return int(n - 1), n
	}
	b := bits.Len32(n - 1)
	return 8 + b - 4, 1 << b
}

// kidClassSize is the size of a list of the class, in handles.
func kidClassSize(class int) int {
	if class < 8 {
		return class + 1
	}
	return 16 << (class - 8)
}

func stClass(n uint32) (class int, size uint32) {
	if n <= 8 {
		return 0, 8
	}
	b := bits.Len32(n - 1)
	return b - 3, 1 << b
}

// allocKids returns the start of a list of n handles (n >= 1). The list's
// contents are not cleared.
func (s *Store) allocKids(cur *arenaCur, n uint32, takeFree bool) uint32 {
	c := s.core
	if n > kidPageLen {
		c.mu.Lock()
		var idx uint32
		if k := len(c.freeLarge); k > 0 {
			idx = c.freeLarge[k-1]
			c.freeLarge = c.freeLarge[:k-1]
			c.large[idx].h = make([]Handle, n)
		} else {
			idx = uint32(len(c.large))
			c.large = append(c.large, &largeList{h: make([]Handle, n)})
		}
		s.syncLocked()
		c.mu.Unlock()
		return largeBit | idx
	}
	class, size := kidClass(n)
	if takeFree && c.freeKid[class] != 0 {
		start := c.freeKid[class]
		c.freeKid[class] = uint32(s.kids[start>>kidShift][start&kidMask])
		return start
	}
	if cur.kidPage < 0 || cur.kidOff+int(size) > kidPageLen {
		s.growKids(cur)
	}
	start := uint32(cur.kidPage)<<kidShift | uint32(cur.kidOff)
	cur.kidOff += int(size)
	return start
}

// kidSpan returns the list [start, start+n).
func (s *Store) kidSpan(start, n uint32) []Handle {
	if n == 0 { // a node without children has no list: no child page need exist yet
		return nil
	}
	if start&largeBit != 0 {
		return s.large[start&^largeBit].h[:n]
	}
	o := start & kidMask
	return s.kids[start>>kidShift][o : o+n]
}

// allocState returns the offset of n bytes of scanner state (n >= 1).
func (s *Store) allocState(cur *arenaCur, n uint32, takeFree bool) uint32 {
	c := s.core
	class, size := stClass(n)
	if takeFree && c.freeSt[class] != 0 {
		off := c.freeSt[class]
		pg := s.state[off>>stShift]
		o := off & stMask
		c.freeSt[class] = uint32(pg[o]) | uint32(pg[o+1])<<8 | uint32(pg[o+2])<<16 | uint32(pg[o+3])<<24
		return off
	}
	if cur.stPage < 0 || cur.stOff+int(size) > stPageLen {
		s.growState(cur)
	}
	off := uint32(cur.stPage)<<stShift | uint32(cur.stOff)
	cur.stOff += int(size)
	return off
}

// stateBytes returns the n scanner-state bytes at off.
func (s *Store) stateBytes(off, n uint32) []byte {
	if n == 0 {
		return nil
	}
	o := off & stMask
	return s.state[off>>stShift][o : o+n]
}

// pageBytes is the memory the pages of the store hold, in bytes: every record,
// refs, child and state page in the tables and every large list.
func (c *storeCore) pageBytes() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	n := len(c.recs)*recPageLen*(recBytes+4) + len(c.kids)*kidPageLen*8
	for _, pg := range c.state {
		n += len(pg)
	}
	for _, l := range c.large {
		n += len(l.h) * 8
	}
	return n
}

// claimEdit takes the next record slot of the edit allocator (the caller holds
// c.emu) and returns its handle; the record is empty until fill. It never takes
// from the parse's free list: that list belongs to the lease. An index from the
// edit free list is checked against the limit like a fresh one. The record
// exists before the storage it will own is allocated, so no other allocator can
// take its place in between.
func (s *Store) claimEdit() Handle {
	idx := s.claimEditIdx()
	*s.recAt(idx) = rec{}
	return Handle(idx) << 1
}

func (s *Store) claimEditIdx() uint32 {
	c := s.core
	if idx, ok := s.popEditFree(); ok {
		return idx
	}
	cur := &c.ec // the edit allocator's page belongs to the holder of emu
	if cur.recSlot == recPageLen {
		s.growRec(cur)
	}
	idx := uint32(cur.recPage)<<recShift | uint32(cur.recSlot)
	if idx > c.recLimit.Load() {
		c.limitPanic("record index limit reached")
	}
	cur.recSlot++
	s.refs[idx>>recShift][idx&recMask] = flagEdit
	return idx
}

// popEditFree takes a record from the edit free list, if there is one and its
// index is within the record limit.
func (s *Store) popEditFree() (uint32, bool) {
	c := s.core
	c.mu.Lock()
	defer c.mu.Unlock()
	idx := c.editFreeRec
	if idx == 0 {
		return 0, false
	}
	if idx > c.recLimit.Load() {
		c.limitPanic("record index limit reached")
	}
	w := &s.refs[idx>>recShift][idx&recMask]
	c.editFreeRec = *w & countMask
	*w = flagEdit
	return idx, true
}

// fillEdit sets the record that claimEdit gave out.
func (s *Store) fillEdit(h Handle, r rec) { *s.recAt(uint32(h >> 1)) = r }

// addEdit is claimEdit and fillEdit for a record that owns no storage.
func (s *Store) addEdit(r rec) Handle {
	h := s.claimEdit()
	s.fillEdit(h, r)
	return h
}

// allocKidsEdit is allocKids for the edit allocator: a list of the class from
// the edit free list if there is one, else the next of the edit allocator's page.
func (s *Store) allocKidsEdit(n uint32) uint32 {
	c := s.core
	if n > kidPageLen {
		return s.allocKids(&c.ec, n, false)
	}
	class, _ := kidClass(n)
	c.mu.Lock()
	if start := c.editFreeKid[class]; start != 0 {
		c.editFreeKid[class] = uint32(s.kids[start>>kidShift][start&kidMask])
		c.mu.Unlock()
		return start
	}
	c.mu.Unlock()
	return s.allocKids(&c.ec, n, false)
}

// allocStateEdit is allocState for the edit allocator.
func (s *Store) allocStateEdit(n uint32) uint32 {
	c := s.core
	class, _ := stClass(n)
	c.mu.Lock()
	if off := c.editFreeSt[class]; off != 0 {
		pg := s.state[off>>stShift]
		o := off & stMask
		c.editFreeSt[class] = uint32(pg[o]) | uint32(pg[o+1])<<8 | uint32(pg[o+2])<<16 | uint32(pg[o+3])<<24
		c.mu.Unlock()
		return off
	}
	c.mu.Unlock()
	return s.allocState(&c.ec, n, false)
}

// isNew reports whether the running parse allocated h (the one
// test that C decides by ref_count).
func (s *Store) isNew(h Handle) bool {
	if s.fresh {
		return true
	}
	return s.rec(h).flags&heapNew != 0
}

// closeSpan logs the slots the parse has bumped on its current page.
func (s *Store) closeSpan() {
	c := s.core
	if c.pc.recPage >= 0 && c.pc.recSlot > c.spanFirst {
		c.spans = append(c.spans, span{page: c.pc.recPage, first: c.spanFirst, last: c.pc.recSlot})
	}
	c.spanFirst = c.pc.recSlot
}

// beginParse sets up the allocation log (the caller holds the lease).
func (s *Store) beginParse(fresh bool) {
	c := s.core
	s.fresh = fresh
	s.logging = !fresh
	s.nLeaf, s.nNode, s.nCopy = 0, 0, 0
	c.spans, c.taken = c.spans[:0], c.taken[:0]
	c.spanFirst = c.pc.recSlot
}

// publish is step 3 of lifetime.go (the caller holds the lease): walk the new tree
// from root, count the owners the tree adds, then sweep what the parse
// allocated and the tree does not reach.
func (s *Store) publish(root Handle) {
	c := s.core
	defer c.poisonOnPanic()
	hk := c.hook.Load()
	stack := append(c.walk[:0], root)
	for len(stack) > 0 {
		h := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		if h == 0 || h&1 != 0 {
			continue
		}
		r := s.rec(h)
		switch {
		case r.flags&heapNew == 0: // a record of an older tree: one more owner
			w := &s.refs[h>>(recShift+1)][(h>>1)&recMask]
			if *w&flagFree != 0 { // the old tree was released under the caller: count it and leave the free-list link alone
				counters.badRetain.Add(1)
				continue
			}
			if *w&countMask >= c.countLimit.Load() {
				c.limitPanic("reference count limit reached")
			}
			*w++
		case r.flags&heapMark == 0:
			r.flags |= heapMark
			if hk != nil {
				(*hk)("publish") // after the first mark: a panic here leaves marks, which sweep(true) must free
			}
			if r.childCount > 0 {
				stack = append(stack, s.kidSpan(r.childrenStart, r.childCount)...)
			}
		default: // the same new record in two slots of the tree
			w := &s.refs[h>>(recShift+1)][(h>>1)&recMask]
			if *w&countMask >= c.countLimit.Load() {
				c.limitPanic("reference count limit reached")
			}
			*w++
			counters.shared.Add(1)
		}
	}
	c.walk = stack[:0]
	s.sweep(false)
}

// sweep frees every record the parse allocated that the publication walk did
// not mark, and clears new and mark on the rest. A parse that ends without a
// tree passes all: it frees every record it allocated, marked or not, so that
// a panic inside the publication walk leaks nothing it allocated (the counts
// the walk had already added to older records stay raised, so those records
// are kept, never freed twice).
func (s *Store) sweep(all bool) {
	c := s.core
	defer c.poisonOnPanic()
	if c.poisoned.Load() { // a poisoned store frees nothing piecemeal
		c.spans, c.taken = c.spans[:0], c.taken[:0]
		return
	}
	c.fire("sweep")
	hk := c.hook.Load()
	s.closeSpan()
	for _, sp := range c.spans {
		base := uint32(sp.page) << recShift
		for slot := sp.first; slot < sp.last; slot++ {
			if hk != nil {
				(*hk)("sweep-record")
			}
			s.sweepOne(base|uint32(slot), all)
		}
	}
	for _, idx := range c.taken {
		if hk != nil {
			(*hk)("sweep-record")
		}
		s.sweepOne(idx, all)
	}
	c.spans, c.taken = c.spans[:0], c.taken[:0]
	s.flushFree()
}

func (s *Store) sweepOne(idx uint32, all bool) {
	r := s.recAt(idx)
	if r.flags&heapNew == 0 {
		return
	}
	freed := all || r.flags&heapMark == 0
	r.flags &^= heapNew | heapMark // a freed or kept record carries neither bit; only here are they cleared
	if freed {
		s.freeRecord(idx, r, &s.refs[idx>>recShift][idx&recMask])
	}
}

// freeRecord is the free step of C/subtree.c:601-605 ts_subtree_release: the
// record, its child list and its state bytes go to the free lists (the caller
// holds the lease). The caller has fetched the record r and its count word w,
// and the record carries neither new nor mark: a published record never does,
// and sweepOne clears them on a record it frees.
func (s *Store) freeRecord(idx uint32, r *rec, w *uint32) {
	c := s.core
	edit := *w&flagEdit != 0
	switch {
	case r.childCount > 0:
		s.freeKidList(r.childrenStart, r.childCount, edit)
	case r.flags&heapHasExternalTokens != 0 && r.namedChildCount > 0:
		s.freeStateBytes(r.visibleDescendantCount, r.namedChildCount, edit)
	}
	if edit {
		*w = flagFree | flagEdit
		c.freedEditRecs = append(c.freedEditRecs, idx)
		return
	}
	*w = flagFree
	c.freedRecs = append(c.freedRecs, idx)
}

// freeKidList frees a child list that has a page of its own at once. The lists
// of the size classes wait in their record: flushFree and flushFreeEdit link
// them, in the order of the sorted records.
func (s *Store) freeKidList(start, n uint32, edit bool) {
	if start&largeBit != 0 {
		c := s.core
		idx := start &^ largeBit
		c.mu.Lock()
		c.large[idx].h = nil
		c.freeLarge = append(c.freeLarge, idx)
		c.mu.Unlock()
	}
}

// pushKidList puts the small child list of the freed record r on the free list of its class.
func (s *Store) pushKidList(r *rec, head *[numKidClasses]uint32) {
	if r.childCount == 0 || r.childrenStart&largeBit != 0 {
		return
	}
	class, _ := kidClass(r.childCount)
	start := r.childrenStart
	s.kids[start>>kidShift][start&kidMask] = Handle(head[class])
	head[class] = start
}

// flushFree puts the batch of freed records and their child lists on the free
// lists, in ascending record order: the lowest record is allocated first, and
// the lists are linked in the same order. One sort serves both.
func (s *Store) flushFree() {
	c := s.core
	if n := len(c.freedRecs); n > 0 {
		sortU32(c.freedRecs, &c.sortTmp)
		for i := n - 1; i >= 0; i-- {
			idx := c.freedRecs[i]
			s.refs[idx>>recShift][idx&recMask] = flagFree | c.freeRec
			c.freeRec = idx
			s.pushKidList(s.recAt(idx), &c.freeKid)
		}
		c.freedRecs = c.freedRecs[:0]
	}
	s.flushFreeEdit()
}

// flushFreeEdit is flushFree for what the edit allocator made; the free lists
// it feeds are guarded by mu.
func (s *Store) flushFreeEdit() {
	c := s.core
	empty := len(c.freedEditRecs) == 0
	for cl := range c.freedEditSt {
		empty = empty && len(c.freedEditSt[cl]) == 0
	}
	if empty {
		return
	}
	sortU32(c.freedEditRecs, &c.sortTmp)
	for cl := range c.freedEditSt {
		sortU32(c.freedEditSt[cl], &c.sortTmp)
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	for i := len(c.freedEditRecs) - 1; i >= 0; i-- {
		idx := c.freedEditRecs[i]
		s.refs[idx>>recShift][idx&recMask] = flagFree | flagEdit | c.editFreeRec
		c.editFreeRec = idx
		s.pushKidList(s.recAt(idx), &c.editFreeKid)
	}
	c.freedEditRecs = c.freedEditRecs[:0]
	for cl := range c.freedEditSt {
		l := c.freedEditSt[cl]
		for i := len(l) - 1; i >= 0; i-- {
			off := l[i]
			pg := s.state[off>>stShift]
			o := off & stMask
			next := c.editFreeSt[cl]
			pg[o], pg[o+1], pg[o+2], pg[o+3] = byte(next), byte(next>>8), byte(next>>16), byte(next>>24)
			c.editFreeSt[cl] = off
		}
		c.freedEditSt[cl] = l[:0]
	}
}

// sortU32 sorts a in ascending order (least-significant-digit radix sort,
// 11 bits a pass; tmp is scratch).
func sortU32(a []uint32, tmp *[]uint32) {
	n := len(a)
	if n < 2 {
		return
	}
	if n < 48 {
		for i := 1; i < n; i++ {
			v := a[i]
			j := i
			for ; j > 0 && a[j-1] > v; j-- {
				a[j] = a[j-1]
			}
			a[j] = v
		}
		return
	}
	var max uint32
	for _, v := range a {
		if v > max {
			max = v
		}
	}
	if cap(*tmp) < n {
		*tmp = make([]uint32, n)
	}
	src, dst := a, (*tmp)[:n]
	for shift := uint(0); shift < 32 && max>>shift != 0; shift += 11 {
		var cnt [2048]int
		for _, v := range src {
			cnt[(v>>shift)&2047]++
		}
		pos := 0
		for i := range cnt {
			c := cnt[i]
			cnt[i] = pos
			pos += c
		}
		for _, v := range src {
			d := (v >> shift) & 2047
			dst[cnt[d]] = v
			cnt[d]++
		}
		src, dst = dst, src
	}
	if &src[0] != &a[0] {
		copy(a, src)
	}
}

func (s *Store) freeStateBytes(off, n uint32, edit bool) {
	c := s.core
	class, _ := stClass(n)
	if edit {
		c.freedEditSt[class] = append(c.freedEditSt[class], off)
		return
	}
	pg := s.state[off>>stShift]
	o := off & stMask
	next := c.freeSt[class]
	pg[o], pg[o+1], pg[o+2], pg[o+3] = byte(next), byte(next>>8), byte(next>>16), byte(next>>24)
	c.freeSt[class] = off
}

// Info describes the pages and the free lists of the store (a statistic for
// the memory gates; it walks the free lists, so it is not for hot paths). It
// takes the lease, so it waits for a running parse and must not be called from
// a callback of one.
func (s *Store) Info() string {
	c := s.core
	c.lease.Lock()
	defer c.afterParse() // a release that failed TryLock while this held the lease is looked at again
	defer c.lease.Unlock()
	c.fire("hold")
	if c.ended.Load() {
		return "store ended"
	}
	v := c.view()
	c.mu.Lock()
	rp, kp, sp, lp := len(c.recs), len(c.kids), len(c.state), len(c.large)
	largeBytes := 0
	for _, l := range c.large {
		largeBytes += len(l.h) * 8
	}
	c.mu.Unlock()
	recs := 0
	for i := c.freeRec; i != 0; i = v.refs[i>>recShift][i&recMask] & countMask {
		recs++
	}
	var kids [numKidClasses]int
	kidHandles := 0
	for cl := range kids {
		for st := c.freeKid[cl]; st != 0; st = uint32(v.kids[st>>kidShift][st&kidMask]) {
			kids[cl]++
			kidHandles += kidClassSize(cl)
		}
	}
	return fmt.Sprintf("rec_pages=%d kid_pages=%d state_pages=%d large_lists=%d large_bytes=%d free_records=%d free_kid_lists=%v free_kid_handles=%d",
		rp, kp, sp, lp, largeBytes, recs, kids, kidHandles)
}

// The pool: the pages of a store that has ended wait here for the next
// parse, so that a long session of fresh parses does not allocate and zero new
// pages. A store takes one page at a time, when it claims it; it never holds a
// page it does not use. The rule that bounds the pool: it holds at most
// poolMaxBytes, pages beyond that go to the garbage collector, and a page that
// has waited more than poolIdle is dropped by a timer that runs only while the
// pool is not empty.
const poolIdle = 30 * time.Second

// poolMaxBytes is a variable so that a test can lower it.
var poolMaxBytes = 128 << 20

type recPagePair struct {
	recs *[recPageLen]rec
	refs *[recPageLen]uint32
	at   int64
}

type kidPageEnt struct {
	pg *[kidPageLen]Handle
	at int64
}

type statePageEnt struct {
	pg []byte
	at int64
}

func recPageBytes() int { return recPageLen * (recBytes + 4) }

var pool struct {
	mu    sync.Mutex
	recs  []recPagePair
	kids  []kidPageEnt
	state []statePageEnt
	bytes int
	timer *time.Timer
}

func takeRecPage() (*[recPageLen]rec, *[recPageLen]uint32) {
	pool.mu.Lock()
	defer pool.mu.Unlock()
	n := len(pool.recs)
	if n == 0 {
		return nil, nil
	}
	e := pool.recs[n-1]
	pool.recs[n-1] = recPagePair{}
	pool.recs = pool.recs[:n-1]
	pool.bytes -= recPageBytes()
	return e.recs, e.refs
}

func takeKidPage() *[kidPageLen]Handle {
	pool.mu.Lock()
	defer pool.mu.Unlock()
	n := len(pool.kids)
	if n == 0 {
		return nil
	}
	e := pool.kids[n-1]
	pool.kids[n-1] = kidPageEnt{}
	pool.kids = pool.kids[:n-1]
	pool.bytes -= kidPageLen * 8
	return e.pg
}

func takeStatePage() []byte {
	pool.mu.Lock()
	defer pool.mu.Unlock()
	n := len(pool.state)
	if n == 0 {
		return nil
	}
	e := pool.state[n-1]
	pool.state[n-1] = statePageEnt{}
	pool.state = pool.state[:n-1]
	pool.bytes -= stPageLen
	return e.pg
}

// putPages gives the pages of an ended store to the pool, up to the byte bound.
func putPages(recs []*[recPageLen]rec, refs []*[recPageLen]uint32, kids []*[kidPageLen]Handle, state [][]byte) {
	now := time.Now().UnixNano()
	pool.mu.Lock()
	defer pool.mu.Unlock()
	for i := range recs {
		if pool.bytes+recPageBytes() > poolMaxBytes {
			break
		}
		pool.recs = append(pool.recs, recPagePair{recs: recs[i], refs: refs[i], at: now})
		pool.bytes += recPageBytes()
	}
	for _, pg := range kids {
		if pool.bytes+kidPageLen*8 > poolMaxBytes {
			break
		}
		pool.kids = append(pool.kids, kidPageEnt{pg: pg, at: now})
		pool.bytes += kidPageLen * 8
	}
	for _, pg := range state {
		if len(pg) != stPageLen || pool.bytes+stPageLen > poolMaxBytes {
			continue
		}
		pool.state = append(pool.state, statePageEnt{pg: pg, at: now})
		pool.bytes += stPageLen
	}
	if pool.bytes > 0 && pool.timer == nil {
		pool.timer = time.AfterFunc(poolIdle, func() { trimPool(time.Now().UnixNano()) })
	}
}

// trimPool drops every page that has waited in the pool for more than poolIdle
// at time now (nanoseconds), and arms the timer again while pages remain.
func trimPool(now int64) {
	cutoff := now - int64(poolIdle)
	pool.mu.Lock()
	defer pool.mu.Unlock()
	k := 0
	for k < len(pool.recs) && pool.recs[k].at < cutoff {
		pool.recs[k] = recPagePair{}
		k++
	}
	n0 := len(pool.recs)
	pool.recs = append(pool.recs[:0], pool.recs[k:]...)
	clear(pool.recs[len(pool.recs):n0])
	pool.bytes -= k * recPageBytes()
	k = 0
	for k < len(pool.kids) && pool.kids[k].at < cutoff {
		pool.kids[k] = kidPageEnt{}
		k++
	}
	n1 := len(pool.kids)
	pool.kids = append(pool.kids[:0], pool.kids[k:]...)
	clear(pool.kids[len(pool.kids):n1])
	pool.bytes -= k * kidPageLen * 8
	k = 0
	for k < len(pool.state) && pool.state[k].at < cutoff {
		pool.state[k] = statePageEnt{}
		k++
	}
	n2 := len(pool.state)
	pool.state = append(pool.state[:0], pool.state[k:]...)
	clear(pool.state[len(pool.state):n2])
	pool.bytes -= k * stPageLen
	pool.timer = nil
	if pool.bytes > 0 {
		pool.timer = time.AfterFunc(poolIdle, func() { trimPool(time.Now().UnixNano()) })
	}
}

// endLocked ends the store (lifetime.go, step 11). The caller holds the lease and
// has seen the cell count at zero: no tree and no parse uses the pages any
// more, so they are dropped whole, without a walk. The generation rises, so an
// operation made for the old store is never applied to a new one.
func (c *storeCore) endLocked() {
	if c.ended.Load() {
		return
	}
	c.mu.Lock()
	recs, refs, kids, state := c.recs, c.refs, c.kids, c.state
	c.recs, c.refs, c.kids, c.large, c.state = nil, nil, nil, nil, nil
	c.pending, c.spare, c.freeLarge = nil, nil, nil
	c.editFreeRec = 0
	c.editFreeKid = [numKidClasses]uint32{}
	c.editFreeSt = [numStClasses]uint32{}
	c.nextRec, c.nextKid, c.nextState = 0, 0, 0
	c.gen++
	c.ended.Store(true)
	c.parseTables = nil
	c.mu.Unlock()
	putPages(recs, refs, kids, state)
	// The first page-table slots live in the core (recsIn, refsIn, kidsIn). The
	// tables above alias them, and putPages has now copied the pointers, so the
	// arrays are cleared here and not before. A parser or a released tree that
	// stays reachable keeps the core; without this it would keep every page of a
	// small store alive after the pool dropped them.
	c.mu.Lock()
	clear(c.recsIn[:])
	clear(c.refsIn[:])
	clear(c.kidsIn[:])
	c.mu.Unlock()
	// The lease holder's lists of an ended store are dropped too: they can be as
	// long as the store was large, and a reachable core would keep them.
	c.rel, c.walk, c.taken, c.sortTmp, c.freedRecs, c.freedEditRecs, c.spans = nil, nil, nil, nil, nil, nil, nil
	c.freedEditSt = [numStClasses][]uint32{}
}
