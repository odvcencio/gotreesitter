package ccore_test

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/odvcencio/gotreesitter/internal/ccore"
	"github.com/odvcencio/gotreesitter/internal/ccore/ccoretest"
)

func loadGoldens(t *testing.T) []*ccoretest.Golden {
	t.Helper()
	files, err := filepath.Glob(filepath.Join("testdata", "golden", "*.json"))
	if err != nil {
		t.Fatal(err)
	}
	if len(files) == 0 {
		t.Fatal("no golden files under testdata/golden")
	}
	var out []*ccoretest.Golden
	for _, f := range files {
		g, err := ccoretest.Read(filepath.Join("testdata", "golden"), filepath.Join("testdata", "tables"), filepath.Base(f))
		if err != nil {
			t.Fatal(err)
		}
		out = append(out, g)
	}
	return out
}

// TestGoldenTrace replays each golden and compares the trace with the one
// pinned C's logger gave, line by line, the digest, and the hash and count of
// the subtree records (which hold the lookahead bytes of every leaf, so a lost
// lookahead end fails here). The tables carry the ASCII fast-path tables, so
// the run loop of the walk is part of the replay.
func TestGoldenTrace(t *testing.T) {
	for _, g := range loadGoldens(t) {
		trace, digest, stop, p := ccoretest.Replay(g)
		if g.RecordCount == 0 {
			t.Errorf("%s: the golden has no record count", g.Name)
		}
		if hash, lines := p.Records(nil); hash != g.RecordHash || lines != g.RecordCount {
			t.Errorf("%s: records hash %016x over %d lines, want %016x over %d", g.Name, hash, lines, g.RecordHash, g.RecordCount)
		}
		if stop != g.Stop {
			t.Errorf("%s: stop %q, want %q", g.Name, stop, g.Stop)
		}
		if digest != g.Digest {
			t.Errorf("%s: digest %s, want %s", g.Name, digest, g.Digest)
		}
		if len(trace) != len(g.Trace) {
			t.Errorf("%s: %d trace lines, want %d", g.Name, len(trace), len(g.Trace))
			continue
		}
		for i := range trace {
			if trace[i] != g.Trace[i] {
				t.Errorf("%s: line %d is %q, want %q", g.Name, i+1, trace[i], g.Trace[i])
				break
			}
		}
	}
}

// TestAsciiTable replays each golden with and without the ASCII fast-path
// tables: the trace, the digest, the records and the arenas must be equal.
func TestAsciiTable(t *testing.T) {
	for _, g := range loadGoldens(t) {
		with, digestWith, _, pWith := ccoretest.Replay(g)
		plain := *g
		tables := *g.Tables
		tables.LexAscii, tables.KeywordLexAscii = nil, nil
		plain.Tables = &tables
		without, digestWithout, _, pWithout := ccoretest.Replay(&plain)
		if digestWith != digestWithout || strings.Join(with, "\n") != strings.Join(without, "\n") {
			t.Errorf("%s: the ASCII table changes the trace or the digest", g.Name)
		}
		if pWith.ArenaDigest() != pWithout.ArenaDigest() {
			t.Errorf("%s: the ASCII table changes the arena", g.Name)
		}
		h1, n1 := pWith.Records(nil)
		h2, n2 := pWithout.Records(nil)
		if h1 != h2 || n1 != n2 {
			t.Errorf("%s: the ASCII table changes the records", g.Name)
		}
	}
}

// TestDeterminism parses each golden twice with one parser and once with a
// fresh one: the arenas must be equal.
func TestDeterminism(t *testing.T) {
	for _, g := range loadGoldens(t) {
		_, _, _, p1 := ccoretest.Replay(g)
		_, _, _, p2 := ccoretest.Replay(g)
		if p1.ArenaDigest() != p2.ArenaDigest() {
			t.Errorf("%s: two parsers give different arenas", g.Name)
		}
		before := p1.ArenaDigest()
		if _, stop := p1.Parse(g.Src); stop != g.Stop {
			t.Errorf("%s: second parse stop %q, want %q", g.Name, stop, g.Stop)
		}
		if p1.ArenaDigest() != before {
			t.Errorf("%s: a second parse on one parser changes the arena", g.Name)
		}
	}
}

// TestEditSelf edits each byte before the final newline to itself and
// reparses with the old tree: the incremental parse must stop with no reason
// and give the digest of the fresh parse. The text is unchanged, so no new
// lexer answer is needed. At least one node must be reused over all goldens.
func TestEditSelf(t *testing.T) {
	totalReused := 0
	for _, g := range loadGoldens(t) {
		p, err := ccore.NewParser(ccore.Config{Tables: g.Tables})
		if err != nil {
			t.Fatalf("%s: %v", g.Name, err)
		}
		if _, stop := p.Parse(g.Src); stop != "" {
			t.Errorf("%s: fresh parse stop %q", g.Name, stop)
			continue
		}
		root := p.Finished()
		fresh := p.Digest()
		reused := 0
		end := len(g.Src)
		if end > 0 && g.Src[end-1] == '\n' {
			end--
		}
		row, col := uint32(0), uint32(0)
		for i := 0; i < end; i++ {
			e := ccore.Edit{StartByte: uint32(i), OldEndByte: uint32(i + 1), NewEndByte: uint32(i + 1), StartRow: row, StartCol: col}
			if g.Src[i] == '\n' {
				e.OldEndRow, e.OldEndCol, e.NewEndRow, e.NewEndCol = row+1, 0, row+1, 0
				row, col = row+1, 0
			} else {
				e.OldEndRow, e.OldEndCol, e.NewEndRow, e.NewEndCol = row, col+1, row, col+1
				col++
			}
			edited := p.Edit(root, e)
			if _, stop := p.ParseWith(g.Src, edited); stop != "" {
				t.Errorf("%s: edit at byte %d: stop %q", g.Name, i, stop)
				break
			}
			if d := p.Digest(); d != fresh {
				t.Errorf("%s: edit at byte %d: digest %s, want %s", g.Name, i, d, fresh)
				break
			}
			n, _, _ := p.ReuseStats()
			reused += n
		}
		totalReused += reused
	}
	if totalReused == 0 {
		t.Error("no node was reused in any self edit")
	}
}
