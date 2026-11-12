package ccore

// The exported surface of a Parser: parse, edit, statistics and the read-side
// hooks adapters and tests use.

import (
	"crypto/sha256"
	"encoding/hex"
	"io"
	"runtime"
)

// Parse parses src from scratch into a new store (C ts_parser_parse with no
// old tree). The returned string is empty on success, or the reason the parse
// stopped; a parse that stopped returns no tree.
func (p *Parser) Parse(src []byte) (*Tree, string) { return p.ParseWith(src, nil) }

// ParseWith parses src, reusing the edited old tree when old is not nil (C
// ts_parser_parse with an old tree). An incremental parse appends to the
// store of old, so old stays valid. The parse holds the store's lease from
// its start to its exit; a second parse of the same store waits for it.
// A released old tree is rejected, also when it was released while this call
// waited for the lease. The parser keeps no reference to the returned tree.
func (p *Parser) ParseWith(src []byte, old *Tree) (*Tree, string) {
	var c *storeCore
	for {
		// A released tree is rejected first, whatever the state of its store, and
		// again on every round: it may be released while this call waits for the
		// lease, and a poisoned store must not turn that rejection into a fresh parse.
		if old != nil && old.c.released.Load() {
			return nil, "old tree released"
		}
		if old != nil && !old.c.store.poisoned.Load() {
			c = old.c.store
		} else {
			old = nil // the store of old is poisoned: parse afresh in a new store
			c = newStoreCore()
			c.cells.Store(1) // the running parse counts as one cell
			c.presize(p.pageHint(len(src)))
			p.applyTestLimits(c)
		}
		c.fire("parse-wait") // test hook: the released check is behind this parse, the lease is not yet taken
		c.lease.Lock()
		if old != nil {
			if old.c.released.Load() { // released while this parse waited for the lease
				c.lease.Unlock()
				c.afterParse() // the release that came meanwhile may have left the end of the store to the lease holder
				return nil, "old tree released"
			}
			if c.poisoned.Load() { // poisoned while this parse waited for the lease: round again, which tests the release first
				c.lease.Unlock()
				c.afterParse()
				continue
			}
		}
		break
	}
	if old != nil {
		c.cells.Add(1) // an ended store is handled below: its old tree is released, and the exit lowers the count again
	}
	p.Store = c.view()
	p.fresh, p.logging = false, false
	published := false
	defer func() { p.endParse(published) }() // the one exit path (lifetime.go, step 5), directly after the lease is taken
	if stop := p.drainAtStart(); stop != "" {
		return nil, stop
	}
	c.fire("parse-start")
	var oldRoot Handle
	if old != nil {
		// The released flag, the root and the page-table headers are captured
		// together: an Edit of old on another goroutine may add a page and publish
		// a new root at any time before this point (it needs no lease), and the
		// root must be read through headers that include its pages.
		c.mu.Lock()
		released := old.c.released.Load()
		oldRoot = Handle(old.c.root.Load())
		p.Store.syncLocked()
		c.mu.Unlock()
		if released { // released after the first test: the drain above may have freed its root
			return nil, "old tree released"
		}
	}
	p.beginParse(old == nil)
	p.ops = 0
	p.zeroNodes, p.balanceChecks, p.balanceRefusals, p.multi = 0, 0, 0, nil
	stop, done := p.parseAndPublish(src, oldRoot)
	if !done {
		return nil, stop
	}
	if old == nil { // remember how many pages a parse of this size took, for the next fresh store
		c.mu.Lock()
		p.hintRec, p.hintKid = len(c.recs), len(c.kids)
		c.mu.Unlock()
	}
	t := &Tree{c: c.newCell(p.finished), rd: p.Store}
	t.rd.fresh, t.rd.logging = false, false
	t.rd.owner = t.c
	published = true
	runtime.KeepAlive(old)
	return t, stop
}

// pageHint is the number of record pages and child pages to reserve in the
// tables of a new store: what the last fresh parse of this parser took, or a
// guess from the source length (a page per 4 KiB of source; a count that is too
// high costs 8 bytes a slot, one that is too low costs the doubling it saves).
// Both are capped.
func (p *Parser) pageHint(srcLen int) (recPages, kidPages int) {
	const capPages = 1 << 14
	recPages, kidPages = p.hintRec, p.hintKid
	if g := srcLen >> 12; recPages < g {
		recPages = g
	}
	if g := srcLen >> 12; kidPages < g {
		kidPages = g
	}
	return min(recPages, capPages), min(kidPages, capPages)
}

// drainAtStart is the drain at the start of a parse. A drain that fails (a limit
// refusal, a panic) poisons the store and swallows the failure; the parse then
// stops with the reason and returns no tree.
func (p *Parser) drainAtStart() (stop string) {
	p.drain()
	if p.core.poisoned.Load() {
		return p.core.failureText()
	}
	return ""
}

// parseAndPublish runs the parse and, for an incremental parse, the publication
// walk. It reports done = false with a stop text when the parse stopped or hit a
// store limit (a limitError panic becomes the stop text; any other panic goes
// on).
func (p *Parser) parseAndPublish(src []byte, oldRoot Handle) (stop string, done bool) {
	defer func() {
		if r := recover(); r != nil {
			if le, ok := r.(*limitError); ok {
				stop, done = le.Error(), false
				return
			}
			panic(r)
		}
	}()
	if p.fresh && p.core.nextKid == 0 { // an empty input builds a node with no child list; the kid table must have a page 0 to slice (this takes the first page under the child-page limit)
		p.growKids(&p.core.pc)
	}
	stop = p.parseWith(src, oldRoot)
	if stop != "" || p.finished == 0 {
		return stop, false
	}
	if p.fresh {
		p.countFreshShared() // a fresh tree has no publication walk: it counts the owners that findMulti found
	} else {
		p.publish(p.finished) // a failure here poisons the store and is swallowed
		if p.core.poisoned.Load() {
			return p.core.failureText(), false
		}
	}
	return "", true
}

// endParse is the exit of every parse, with a tree or without one. A parse
// that ends without a tree sweeps everything it allocated; both clear the
// parser's handles and release the lease. When the parse was the last user of
// the store, the store ends. Everything after the sweep and the drain runs
// under defer, so a panic in either leaves the lease free and the count right.
func (p *Parser) endParse(published bool) {
	c := p.core
	defer c.afterParse()
	defer c.lease.Unlock()
	defer func() {
		if c.cells.Add(-1) == 0 {
			c.endLocked()
		}
	}()
	defer p.clearHandles()
	if !published && p.logging {
		p.sweep(true)
	}
	if c.cells.Load() > 1 {
		p.drain()
	}
}

// clearHandles drops what the parser holds after a parse: the token cache, the
// stack, the finished root, the reuse cursor, and its view of the page tables
// (a tree keeps its own view; the parser must not keep the pages of an ended
// store alive).
func (p *Parser) clearHandles() {
	p.fresh, p.logging = false, false
	p.finished = 0
	p.cacheTok, p.cacheAt, p.ext.cacheLast = 0, 0, 0
	p.nodes, p.free, p.heads = p.nodes[:0], p.free[:0], p.heads[:0]
	p.rnClear()
	p.recs, p.refs, p.kids, p.large, p.state = nil, nil, nil, nil, nil
}

// LeaseFree reports whether no parse holds the lease of the parser's store.
func (p *Parser) LeaseFree() bool {
	if p.core == nil || !p.core.lease.TryLock() {
		return p.core == nil
	}
	p.core.lease.Unlock()
	p.core.afterParse() // a release that failed TryLock against this probe is looked at again
	return true
}

// SharedInTree counts, over every store of the process, the new records the publication walk met in two slots of one tree.
func (p *Parser) SharedInTree() int64 { return counters.shared.Load() }

// DoubleFree counts, over every store of the process, the releases that reached a record already on the free list.
func (p *Parser) DoubleFree() int64 { return counters.double.Load() }

// BadRetain counts the retains that reached a record already on the free list.
func (p *Parser) BadRetain() int64 { return counters.badRetain.Load() }

// StoreInfo describes the pages and free lists of the parser's store.
func (p *Parser) StoreInfo() string { return p.Store.Info() }

// SetStopAfter makes the next parses stop after n steps of the parse loop (a
// test hook for parses that end without a tree); n <= 0 turns it off. With
// panicking set, the parse panics at that step instead of stopping.
func (p *Parser) SetStopAfter(n int, panicking bool) { p.stopAt, p.stopPanics = n, panicking }

// Edit mirrors InputEdit.
type Edit struct {
	StartByte, OldEndByte, NewEndByte                              uint32
	StartRow, StartCol, OldEndRow, OldEndCol, NewEndRow, NewEndCol uint32
}

// Stats are the counters of the last parse. The first five are always on.
type Stats struct {
	LexerCalls  int // calls of the lexer, C ts_parser__lex
	Shifts      int
	Reductions  int
	HeapRecords int // record slots the store has handed out
	RecLeaf     int // heap leaves the last parse allocated (lexed tokens, error leaves)
	RecNode     int // internal nodes the last parse allocated (reductions)
	RecCopy     int // heap leaf copies the last parse made (set symbol, set extra)
	MaxVersions int // maximum live stack versions
	Tokens      int // tokens lexed
	MultiTokens int // tokens lexed while more than one version was live
	ArenaBytes  int // capacity of the record, child and stack arenas, in bytes
}

// BalanceStats returns, for the last parse, the internal nodes of byte size
// zero that it made, the number of times the pre-balance walk for records reached
// twice ran (1 for a parse that finished a tree), and the number of rewrites the
// balance step refused.
func (p *Parser) BalanceStats() (zeroNodes, checks, refusals int) {
	return p.zeroNodes, p.balanceChecks, p.balanceRefusals
}

// Stats returns the counters of the last parse. It reads the parser's own
// counters and the allocation position of its store without the lease, so it is
// for the goroutine that parsed, when no other parser parses in the same store.
func (p *Parser) Stats() Stats {
	return Stats{
		LexerCalls: p.lexCalls, Shifts: p.shifts, Reductions: p.reductions,
		HeapRecords: p.allocatedRecords(), MaxVersions: p.maxVersions,
		RecLeaf: p.nLeaf, RecNode: p.nNode, RecCopy: p.nCopy,
		Tokens: p.tokens, MultiTokens: p.multiTokens,
		ArenaBytes: p.capBytes() + cap(p.nodes)*168,
	}
}

// reader returns a read context for t: a Parser value of its own that holds
// the tree's store view and the language tables, which are immutable. The
// readers below use it so that they never write the parser's state (a parse may
// be running in p); the only parser fields they read are the tables and
// NormNames.
func (p *Parser) reader(t *Tree) *Parser {
	return &Parser{t: p.t, NormNames: p.NormNames, Store: t.rd}
}

// Digest is the visible-tree digest of t (nil or released: empty).
func (p *Parser) Digest(t *Tree) string { return p.DigestOf(t, false) }

// DigestOf is the visible-tree digest of a tree; changes adds has_changes.
// It reads through the tree's own view of the store and keeps the tree alive
// while it reads. The readers Digest, DigestOf, Walk, RootEnd and Records
// must not run in a logger or scanner callback of this parser.
func (p *Parser) DigestOf(t *Tree, changes bool) string {
	if t == nil || t.c.released.Load() {
		return ""
	}
	defer runtime.KeepAlive(t)
	return p.reader(t).digestOf(t.Root(), changes)
}

// Walk counts the visible nodes of t.
func (p *Parser) Walk(t *Tree) int {
	if t == nil || t.c.released.Load() {
		return 0
	}
	defer runtime.KeepAlive(t)
	return p.reader(t).walkCount(t.Root())
}

// Records writes one line per subtree of t to w (when w is not nil) and
// returns the FNV-1a 64 hash and the number of lines (records.go).
func (p *Parser) Records(t *Tree, w io.Writer) (hash uint64, lines uint64) {
	if t == nil || t.c.released.Load() {
		return fnvOffset, 0
	}
	defer runtime.KeepAlive(t)
	return p.reader(t).records(t.Root(), w)
}

// ArenaDigest hashes the record and child pages and the scanner-state pages of
// the store of t (determinism check: two parses of one input give equal
// pages). It takes the lease, so it waits for a running parse and must not be
// called from a callback of one.
func (p *Parser) ArenaDigest(t *Tree) string {
	if t == nil {
		return ""
	}
	defer runtime.KeepAlive(t)
	c := t.c.store
	c.lease.Lock()
	defer c.afterParse()
	defer c.lease.Unlock()
	c.emu.Lock() // Edit writes the edit allocator's page under emu
	defer c.emu.Unlock()
	c.fire("hold")
	if c.ended.Load() {
		return ""
	}
	v := c.view()
	h := sha256.New()
	v.hashArena(h)
	return hex.EncodeToString(h.Sum(nil))
}

// RootEnd is the end byte of t.
func (p *Parser) RootEnd(t *Tree) uint32 {
	if t == nil || t.c.released.Load() || t.Root() == 0 {
		return 0
	}
	defer runtime.KeepAlive(t)
	return p.reader(t).totalSize(t.Root()).bytes
}

// ReuseStats returns nodes reused, their total bytes and the largest one.
func (p *Parser) ReuseStats() (nodes int, bytes uint64, largest uint32) {
	return p.reusedNodes, p.reusedBytes, p.largestReuse
}

// StoreBytes is the memory the pages of the store of the parser's last parse
// hold, in bytes: every record, refs, child and state page and every large
// list. It is 0 once the store has ended.
func (p *Parser) StoreBytes() int { return p.capBytes() }

// ArenaUse returns the page bytes the store holds and the bytes reachable from t.
func (p *Parser) ArenaUse(t *Tree) (used, reachable int) {
	if t.c.released.Load() {
		return 0, 0
	}
	defer runtime.KeepAlive(t)
	return t.rd.arenaUse(t.Root())
}
