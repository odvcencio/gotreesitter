//go:build !grammar_subset

package grammarruntime

import (
	"bytes"
	"os"
	"reflect"
	"testing"
	_ "unsafe"

	gts "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/internal/incr"
)

// Scan the complete valid-symbol mask space with empty and persisted payloads.
// The witnesses exercise content, failed delimiter probes, and keyword edges;
// punctuation edits are deliberately outside the certificate.
func TestLengthNeutralScannerCertificate(t *testing.T) {
	name := os.Getenv("GTS_ZERO_REUSE_LANGUAGE")
	var scanner gts.ExternalScanner
	var states []any
	var count int
	var witnesses []string
	switch name {
	case "lua":
		scanner, count = LuaExternalScanner{}, luaTokenCount
		states = []any{&luaScannerState{}, &luaScannerState{levelCount: 2}}
		witnesses = []string{"xx]]", "[==[xx]==]", "--xx\n", "--[=[xx]=]", "xx]=]"}
	case "nickel":
		scanner, count = NickelExternalScanner{}, nickelTokenCount
		states = []any{&nickelState{}, &nickelState{expectedPercentCount: []uint8{0, 2}}}
		witnesses = []string{"xx", "#xx\n", "xx-s%", "xx-m%\"", "m%\"xx\"%"}
	case "starlark":
		scanner, count = StarlarkExternalScanner{}, slTokenCount
		states = []any{&pythonScannerState{Indents: []uint16{0}}, &pythonScannerState{Indents: []uint16{0, 4}, Delimiters: []pyDelimiter{pyDelimDoubleQuote}}, &pythonScannerState{Indents: []uint16{0}, Delimiters: []pyDelimiter{pyDelimDoubleQuote | pyDelimFormat | pyDelimTriple}}}
		witnesses = []string{"xx", "#xx\n", "xx\"", "xx{value}\"", "xx\\\\N{NAME}\"", "f\"xx\""}
	case "properties":
		scanner, count = PropertiesExternalScanner{}, propertiesTokenCount
		states = []any{&propertiesScannerState{}, &propertiesScannerState{emittedEOF: true}}
		witnesses = []string{"xx", "#xx\n", "key=xx\n"}
	case "firrtl":
		scanner, count = FirrtlExternalScanner{}, firrtlTokenCount
		states = []any{&firrtlState{indents: []uint16{0}}, &firrtlState{indents: []uint16{0, 2, 4}}}
		witnesses = []string{"xx", "#xx\n  value", "\n  #xx\n    value", "\\xx\n", "\n  xx"}
	default:
		t.Fatal("set GTS_ZERO_REUSE_LANGUAGE to a certified grammar")
	}
	for _, state := range states {
		var wire [1024]byte
		n := scanner.Serialize(state, wire[:])
		for _, witness := range witnesses {
			old := []byte(witness)
			pos := bytes.Index(old, []byte("xx")) + 1
			if pos < 1 {
				t.Fatal("bad witness")
			}
			for _, growth := range []int{1, 3, -1} {
				oldEnd, added := pos, growth
				if growth < 0 {
					oldEnd, added = pos-growth, 0
				}
				next := append(append(append([]byte{}, old[:pos]...), bytes.Repeat([]byte{'x'}, added)...), old[oldEnd:]...)
				edit := incr.TokenEdit{Start: uint32(pos), OldEnd: uint32(oldEnd), NewEnd: uint32(pos + added), Row: uint32(bytes.Count(old[:pos], []byte{'\n'}))}
				if !incr.LengthNeutralScannerEdit(scanner, old, next, edit) {
					t.Fatal("witness has no certificate")
				}
				checkLengthNeutralScannerWitness(t, scanner, wire[:n], count, old, next, edit)
			}
		}
	}
}

func checkLengthNeutralScannerWitness(t *testing.T, scanner gts.ExternalScanner, wire []byte, count int, old, next []byte, edit incr.TokenEdit) {
	t.Helper()
	for bits := 0; bits < 1<<uint(count); bits++ {
		valid := make([]bool, count)
		for i := range valid {
			valid[i] = bits&(1<<uint(i)) != 0
		}
		a, b := scanner.Create(), scanner.Create()
		scanner.Deserialize(a, wire)
		scanner.Deserialize(b, wire)
		la, lb := newLengthNeutralLexer(old, 0, 0, 0), newLengthNeutralLexer(next, 0, 0, 0)
		oka, okb := scanner.Scan(a, la, valid), scanner.Scan(b, lb, valid)
		var wa, wb [1024]byte
		na, nb := scanner.Serialize(a, wa[:]), scanner.Serialize(b, wb[:])
		if oka != okb || !bytes.Equal(wa[:na], wb[:nb]) {
			t.Fatalf("%q edit=%+v mask=%x outcomes/payloads differ", old, edit, bits)
		}
		cursorA, pointA := lengthNeutralLexerCursor(la)
		cursorB, pointB := lengthNeutralLexerCursor(lb)
		cursor, cok := edit.Byte(cursorA)
		row, column, pok := edit.Point(cursorA, pointA.Row, pointA.Column)
		if !cok || !pok || cursor != cursorB || (gts.Point{Row: row, Column: column}) != pointB || la.Lookahead() != lb.Lookahead() {
			t.Fatalf("%q edit=%+v mask=%x cursor differs: %d/%+v -> %d/%+v", old, edit, bits, cursorA, pointA, cursorB, pointB)
		}
		ta, hasa := lengthNeutralLexerToken(la)
		tb, hasb := lengthNeutralLexerToken(lb)
		if hasa != hasb {
			t.Fatalf("%q edit=%+v mask=%x mark differs", old, edit, bits)
		}
		if hasa {
			start, sok := edit.Byte(ta.StartByte)
			end, eok := edit.Byte(ta.EndByte)
			srow, scol, spok := edit.Point(ta.StartByte, ta.StartPoint.Row, ta.StartPoint.Column)
			erow, ecol, epok := edit.Point(ta.EndByte, ta.EndPoint.Row, ta.EndPoint.Column)
			if !sok || !eok || !spok || !epok || start != tb.StartByte || end != tb.EndByte || ta.Symbol != tb.Symbol ||
				(gts.Point{Row: srow, Column: scol}) != tb.StartPoint || (gts.Point{Row: erow, Column: ecol}) != tb.EndPoint {
				t.Fatalf("%q edit=%+v mask=%x token differs: %+v -> %+v", old, edit, bits, ta, tb)
			}
		}
		scanner.Destroy(a)
		scanner.Destroy(b)
	}
}

// Read cursor coordinates only in tests, including unsuccessful scans that
// have no token. Column alone cannot authenticate a multiline witness.
func lengthNeutralLexerCursor(lexer *gts.ExternalLexer) (uint32, gts.Point) {
	v := reflect.ValueOf(lexer).Elem()
	point := v.FieldByName("point")
	return uint32(v.FieldByName("pos").Int()), gts.Point{
		Row: uint32(point.FieldByName("Row").Uint()), Column: uint32(point.FieldByName("Column").Uint()),
	}
}

//go:linkname newLengthNeutralLexer github.com/odvcencio/gotreesitter.newExternalLexer
func newLengthNeutralLexer([]byte, int, uint32, uint32) *gts.ExternalLexer

//go:linkname lengthNeutralLexerToken github.com/odvcencio/gotreesitter.(*ExternalLexer).token
func lengthNeutralLexerToken(*gts.ExternalLexer) (gts.Token, bool)

// The R raw-string closer trusts parser context and consumes a fixed width.
// It cannot satisfy an unconditional scanner-owned run-length certificate.
func TestRLengthNeutralFixedCloseCounterexample(t *testing.T) {
	scanner := RExternalScanner{}
	valid := make([]bool, rTokenCount)
	valid[rTokRawStringClose] = true
	a, b := scanner.Create(), scanner.Create()
	defer scanner.Destroy(a)
	defer scanner.Destroy(b)
	old, next := newLengthNeutralLexer([]byte("xx"), 0, 0, 0), newLengthNeutralLexer([]byte("xxx"), 0, 0, 0)
	if !scanner.Scan(a, old, valid) || !scanner.Scan(b, next, valid) || old.Column() != 2 || next.Column() != 2 {
		t.Fatal("fixed-close witness changed")
	}
	projected, ok := (incr.TokenEdit{Start: 1, OldEnd: 1, NewEnd: 2}).Byte(old.Column())
	if !ok || projected == next.Column() {
		t.Fatal("fixed-close witness is length neutral")
	}
	if _, ok := any(scanner).(interface{ ExternalScannerLengthNeutralASCIIClass(byte) uint8 }); ok {
		t.Fatal("R advertises the refuted certificate")
	}
}
