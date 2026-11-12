// Package bindtest is test support for the equality tests of cgo_harness: a
// wrapper around a parser of internal/ccore that keeps the call shapes the
// checks use, the digest record both sides write, and the golden recorder.
// It must not be imported by non-test code of the root package or of
// internal/ccore/bind: it holds a mutable package variable and test-only
// helpers, and everything that package imports is linked into every user's
// binary.
package bindtest

import (
	"os"

	"github.com/odvcencio/gotreesitter/internal/ccore"
	"github.com/odvcencio/gotreesitter/internal/ccore/bind"
)

var normNames bool

// NormalizeNames makes digest records compare node names with backslashes
// removed, for grammars whose Go table spells a symbol name with a backslash
// that C's table does not.
func NormalizeNames(on bool) { normNames = on }

// DigestRecord writes one node the way both digest sides do.
func DigestRecord(buf []byte, kind string, startByte, endByte, sr, sc, er, ec uint32, named, extra, missing, changes bool, children uint32) []byte {
	return ccore.DigestRecord(buf, kind, startByte, endByte, sr, sc, er, ec, named, extra, missing, changes, children, normNames)
}

// Parser drives internal/ccore for one language.
//
// The core's Parser keeps no tree; this wrapper keeps the tree of its last
// successful parse (last) so that Digest, Walk, RootEnd, ArenaDigest and
// RecordsHash can read it, and drops it at ReleaseLast. Release it before the
// next parse to end its store; a read after ReleaseLast gives the empty answer.
type Parser struct {
	p    *ccore.Parser
	last *ccore.Tree
}

// NewParser returns a parser for a language (passed as any).
func NewParser(lang any) (*Parser, error) {
	p, err := bind.New(lang)
	if err != nil {
		return nil, err
	}
	return &Parser{p: p}, nil
}

// ForceDerivedRows derives the external valid-symbol rows from the action
// table even when the blob has them.
func (c *Parser) ForceDerivedRows(on bool) { c.p.SetForceDerivedRows(on) }

// Parse parses src; log, when not nil, receives C's parse log lines.
func (c *Parser) Parse(src []byte, log func(string)) string {
	c.syncNorm()
	c.p.SetLogger(log)
	defer c.p.SetLogger(nil)
	t, stop := c.p.Parse(src)
	c.last = t
	return stop
}

// Digest is the visible-tree digest of the last parse.
func (c *Parser) Digest() string { c.syncNorm(); return c.p.Digest(c.last) }

// Walk counts the visible nodes of the last parse's tree.
func (c *Parser) Walk() int { return c.p.Walk(c.last) }

// Stats returns steps (shift or reduce), lexed tokens, the maximum version
// count, tokens lexed while more than one version was live, and arena bytes.
func (c *Parser) Stats() (steps, tokens, maxVersions, multiTokens, arenaBytes int) {
	s := c.p.Stats()
	return s.Shifts + s.Reductions, s.Tokens, s.MaxVersions, s.MultiTokens, s.ArenaBytes
}

// ArenaDigest hashes the subtree and children arenas (determinism check).
func (c *Parser) ArenaDigest() string { return c.p.ArenaDigest(c.last) }

// RootEnd is the finished tree's end byte.
func (c *Parser) RootEnd() uint32 { return c.p.RootEnd(c.last) }

// Tree is a tree of the core: a root in a store, owned by the caller until
// Release (or until the garbage collector finds it unreachable).
type Tree struct{ t *ccore.Tree }

// Edit mirrors InputEdit.
type Edit struct {
	StartByte, OldEndByte, NewEndByte                              uint32
	StartRow, StartCol, OldEndRow, OldEndCol, NewEndRow, NewEndCol uint32
}

// Tree is the tree of the last successful parse (the parser keeps it reachable).
func (c *Parser) Tree() Tree { return Tree{t: c.last} }

// ReleaseLast releases the tree of the last parse and forgets it (a second call does nothing).
func (c *Parser) ReleaseLast() {
	if t := c.last; t != nil {
		t.Release()
	}
	c.last = nil
}

// Edit is ts_tree_edit on a copy: it returns the edited tree; t is unchanged.
func (c *Parser) Edit(t Tree, e Edit) Tree {
	n := t.t.Copy()
	n.Edit(ccore.Edit(e))
	return Tree{t: n}
}

// EditInPlace is ts_tree_edit: t becomes the edited tree.
func (c *Parser) EditInPlace(t Tree, e Edit) { t.t.Edit(ccore.Edit(e)) }

// Copy is ts_tree_copy.
func (c *Parser) Copy(t Tree) Tree { return Tree{t: t.t.Copy()} }

// Release gives up the caller's ownership of t (nil trees and second calls do nothing).
func (c *Parser) Release(t Tree) {
	if t.t != nil {
		t.t.Release()
	}
}

// ParseIncremental parses src reusing the edited old tree (same store). On a
// stop it returns an empty tree.
func (c *Parser) ParseIncremental(src []byte, old Tree, log func(string)) (Tree, string) {
	c.syncNorm()
	c.p.SetLogger(log)
	defer c.p.SetLogger(nil)
	t, stop := c.p.ParseWith(src, old.t)
	c.last = t
	return Tree{t: t}, stop
}

// SetStopAfter makes later parses stop (or panic) after n steps of the parse loop; n <= 0 turns it off.
func (c *Parser) SetStopAfter(n int, panicking bool) { c.p.SetStopAfter(n, panicking) }

// StoreInfo describes the pages and free lists of the store.
func (c *Parser) StoreInfo() string { return c.p.StoreInfo() }

// SharedInTree and DoubleFree are the store's integrity counters; BadRetain counts retains of freed records.
func (c *Parser) SharedInTree() int64 { return c.p.SharedInTree() }
func (c *Parser) DoubleFree() int64   { return c.p.DoubleFree() }
func (c *Parser) BadRetain() int64    { return c.p.BadRetain() }

// RecordSources are the records the last parse allocated: leaves, nodes, leaf copies.
func (c *Parser) RecordSources() (leaves, nodes, copies int) {
	s := c.p.Stats()
	return s.RecLeaf, s.RecNode, s.RecCopy
}

// StoreBytes is the memory the pages of the current store hold, in bytes.
func (c *Parser) StoreBytes() int { return c.p.StoreBytes() }

// LeaseFree reports whether no parse holds the lease of the current store.
func (c *Parser) LeaseFree() bool { return c.p.LeaseFree() }

// Valid reports whether t holds a tree (a stopped parse returns none).
func (t Tree) Valid() bool { return t.t != nil }

// Handle is the root handle of t (for the determinism check of handles).
func (t Tree) Handle() uint64 { return uint64(t.t.Root()) }

// EditRecords is the number of records the last in-place edit of t created.
func (t Tree) EditRecords() int { return t.t.LastEditRecords() }

// CountNodes walks t through its own view of the store (no lock, any goroutine)
// and returns the number of nodes and a fold of their symbols.
func (t Tree) CountNodes() (n int, fold uint64) {
	s := t.t.Reader()
	stack := []ccore.Handle{t.t.Root()}
	for len(stack) > 0 {
		h := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		if h == 0 {
			continue
		}
		n++
		fold = fold*1099511628211 + uint64(s.Symbol(h)) + 1
		for _, b := range s.ExtState(h) {
			fold = fold*1099511628211 + uint64(b)
		}
		if s.ChildCount(h) > 0 {
			stack = append(stack, s.Children(h)...)
		}
	}
	return n, fold
}

// DigestOf is the visible-tree digest of a tree (read through the tree's own view); changes adds has_changes.
func (c *Parser) DigestOf(t Tree, changes bool) string {
	c.syncNorm()
	return c.p.DigestOf(t.t, changes)
}

// ReuseStats returns nodes reused, their total bytes and the largest one.
func (c *Parser) ReuseStats() (nodes int, bytes uint64, largest uint32) {
	return c.p.ReuseStats()
}

// ArenaUse returns the page bytes the store holds and the bytes reachable from t.
func (c *Parser) ArenaUse(t Tree) (used, reachable int) { return c.p.ArenaUse(t.t) }

// StoreFailure is the reason the store of t was poisoned, or "" if it was not.
func (c *Parser) StoreFailure(t Tree) string { return t.t.StoreFailure() }

// BalanceStats returns, for the last parse, the internal nodes of byte size zero
// it made, the pre-balance walks it ran (0 or 1) and the rewrites that the
// balance step refused because the tree reaches the node twice.
func (c *Parser) BalanceStats() (zeroNodes, checks, refusals int) { return c.p.BalanceStats() }

// DoubleReach walks the tree and returns the number of heap records that are
// reached more than once (a record in two slots, zero-width ones included), and
// the number of heap records visited. It reads through the tree's own view.
func (c *Parser) DoubleReach(t Tree) (twice, visited int) {
	if t.t == nil {
		return 0, 0
	}
	s := t.t.Reader()
	seen := map[ccore.Handle]bool{}
	stack := []ccore.Handle{t.t.Root()}
	for len(stack) > 0 {
		h := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		if h == 0 || h&1 != 0 {
			continue
		}
		visited++
		if seen[h] {
			twice++
			continue
		}
		seen[h] = true
		stack = append(stack, s.Children(h)...)
	}
	return twice, visited
}

// RecordsHash returns the FNV-1a 64 hash and the line count of the finished
// tree's subtree records; dump, when not empty, is a file to receive the lines.
// It panics when the file cannot be created (test support).
func (c *Parser) RecordsHash(dump string) (uint64, uint64) {
	if dump == "" {
		return c.p.Records(c.last, nil)
	}
	f, err := os.Create(dump)
	if err != nil {
		panic(err)
	}
	defer f.Close()
	return c.p.Records(c.last, f)
}

// syncNorm sets the core parser's NormNames from the package setting. It writes
// only when the value changes, so that concurrent readers of the parser (which
// read NormNames) do not race with a call in steady state.
func (c *Parser) syncNorm() {
	if c.p.NormNames != normNames {
		c.p.NormNames = normNames
	}
}

// NodeReader returns a reader of value nodes for t (read through the tree's own
// view of the store) and the handle of its root.
func (c *Parser) NodeReader(t Tree) (r ccore.NodeReader, root ccore.Handle) {
	return ccore.NewNodeReader(t.t.Reader(), c.p.NodeTables()), t.t.Root()
}
