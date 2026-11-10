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
type Parser struct{ p *ccore.Parser }

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
	c.p.NormNames = normNames
	c.p.SetLogger(log)
	defer c.p.SetLogger(nil)
	_, stop := c.p.Parse(src)
	return stop
}

// Digest is the visible-tree digest of the last parse.
func (c *Parser) Digest() string { c.p.NormNames = normNames; return c.p.Digest() }

// Walk counts the visible nodes of the last parse's tree.
func (c *Parser) Walk() int { return c.p.Walk() }

// Stats returns steps (shift or reduce), lexed tokens, the maximum version
// count, tokens lexed while more than one version was live, and arena bytes.
func (c *Parser) Stats() (steps, tokens, maxVersions, multiTokens, arenaBytes int) {
	s := c.p.Stats()
	return s.Shifts + s.Reductions, s.Tokens, s.MaxVersions, s.MultiTokens, s.ArenaBytes
}

// ArenaDigest hashes the subtree and children arenas (determinism check).
func (c *Parser) ArenaDigest() string { return c.p.ArenaDigest() }

// RootEnd is the finished tree's end byte.
func (c *Parser) RootEnd() uint32 { return c.p.RootEnd() }

// Tree is a root handle into a parser's arena; it stays valid until that
// parser runs a fresh (non-incremental) parse.
type Tree struct{ root ccore.Handle }

// Edit mirrors InputEdit.
type Edit struct {
	StartByte, OldEndByte, NewEndByte                              uint32
	StartRow, StartCol, OldEndRow, OldEndCol, NewEndRow, NewEndCol uint32
}

// Tree is the finished tree of the last parse.
func (c *Parser) Tree() Tree { return Tree{root: c.p.Finished()} }

// Edit is ts_tree_edit: it returns the edited tree; the old one is unchanged.
func (c *Parser) Edit(t Tree, e Edit) Tree {
	return Tree{root: c.p.Edit(t.root, ccore.Edit(e))}
}

// ParseIncremental parses src reusing the edited old tree (same arena).
func (c *Parser) ParseIncremental(src []byte, old Tree, log func(string)) (Tree, string) {
	c.p.NormNames = normNames
	c.p.SetLogger(log)
	defer c.p.SetLogger(nil)
	t, stop := c.p.ParseWith(src, old.root)
	return Tree{root: t.Root}, stop
}

// DigestOf is the visible-tree digest of a tree; changes adds has_changes.
func (c *Parser) DigestOf(t Tree, changes bool) string {
	c.p.NormNames = normNames
	return c.p.DigestOf(t.root, changes)
}

// ReuseStats returns nodes reused, their total bytes and the largest one.
func (c *Parser) ReuseStats() (nodes int, bytes uint64, largest uint32) {
	return c.p.ReuseStats()
}

// ArenaUse returns the arena bytes in use and the bytes reachable from t.
func (c *Parser) ArenaUse(t Tree) (used, reachable int) { return c.p.ArenaUse(t.root) }

// RecordsHash returns the FNV-1a 64 hash and the line count of the finished
// tree's subtree records; dump, when not empty, is a file to receive the lines.
// It panics when the file cannot be created (test support).
func (c *Parser) RecordsHash(dump string) (uint64, uint64) {
	if dump == "" {
		return c.p.Records(nil)
	}
	f, err := os.Create(dump)
	if err != nil {
		panic(err)
	}
	defer f.Close()
	return c.p.Records(f)
}
