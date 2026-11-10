package ccore

// The exported surface of a Parser: parse, edit, statistics and the read-side
// hooks adapters and tests use.

import (
	"crypto/sha256"
	"encoding/hex"
)

// Parse parses src from scratch (C ts_parser_parse with no old tree). The
// returned string is empty on success, or the reason the parse stopped.
func (p *Parser) Parse(src []byte) (Tree, string) { return p.ParseWith(src, 0) }

// ParseWith parses src, reusing the edited old tree rooted at old when old
// is not 0 (C ts_parser_parse with an old tree). A fresh parse clears the
// Store; an incremental parse appends to it, so old stays valid.
func (p *Parser) ParseWith(src []byte, old Handle) (Tree, string) {
	stop := p.parseWith(src, old)
	return Tree{Root: p.finished, Store: &p.Store}, stop
}

// Finished is the root of the last parse (0 when it found no tree).
func (p *Parser) Finished() Handle { return p.finished }

// Edit mirrors InputEdit.
type Edit struct {
	StartByte, OldEndByte, NewEndByte                              uint32
	StartRow, StartCol, OldEndRow, OldEndCol, NewEndRow, NewEndCol uint32
}

// Edit is ts_tree_edit: it returns the edited tree; the old one is unchanged.
func (p *Parser) Edit(root Handle, e Edit) Handle {
	return p.treeEdit(root, editRange{
		start:  Length{bytes: e.StartByte, row: e.StartRow, col: e.StartCol},
		oldEnd: Length{bytes: e.OldEndByte, row: e.OldEndRow, col: e.OldEndCol},
		newEnd: Length{bytes: e.NewEndByte, row: e.NewEndRow, col: e.NewEndCol},
	})
}

// Stats are the counters of the last parse. The first five are always on.
type Stats struct {
	LexerCalls  int // calls of the lexer, C ts_parser__lex
	Shifts      int
	Reductions  int
	HeapRecords int // records in the Store after the parse
	MaxVersions int // maximum live stack versions
	Tokens      int // tokens lexed
	MultiTokens int // tokens lexed while more than one version was live
	ArenaBytes  int // capacity of the record, child and stack arenas, in bytes
}

// Stats returns the counters of the last parse.
func (p *Parser) Stats() Stats {
	return Stats{
		LexerCalls: p.lexCalls, Shifts: p.shifts, Reductions: p.reductions,
		HeapRecords: len(p.recs) - 1, MaxVersions: p.maxVersions,
		Tokens: p.tokens, MultiTokens: p.multiTokens,
		ArenaBytes: p.capBytes() + cap(p.nodes)*168,
	}
}

// Digest is the visible-tree digest of the last parse.
func (p *Parser) Digest() string { return p.digest() }

// DigestOf is the visible-tree digest of a tree; changes adds has_changes.
func (p *Parser) DigestOf(root Handle, changes bool) string { return p.digestOf(root, changes) }

// Walk counts the visible nodes of the last parse's tree.
func (p *Parser) Walk() int { return p.walk() }

// ArenaDigest hashes the record and child arenas and the scanner-state arena
// (determinism check: two parses of one input give equal arenas).
func (p *Parser) ArenaDigest() string {
	h := sha256.New()
	p.hashArena(h)
	h.Write(p.ext.bytes)
	return hex.EncodeToString(h.Sum(nil))
}

// RootEnd is the finished tree's end byte.
func (p *Parser) RootEnd() uint32 {
	if p.finished == 0 {
		return 0
	}
	return p.totalSize(p.finished).bytes
}

// ReuseStats returns nodes reused, their total bytes and the largest one.
func (p *Parser) ReuseStats() (nodes int, bytes uint64, largest uint32) {
	return p.reusedNodes, p.reusedBytes, p.largestReuse
}

// ArenaUse returns the arena bytes in use and the bytes reachable from root.
func (p *Parser) ArenaUse(root Handle) (used, reachable int) { return p.arenaUse(root) }
