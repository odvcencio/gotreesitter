//go:build !grammar_subset

package grammarruntime

import (
	"bytes"
	"os"
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
	default:
		t.Fatal("set GTS_ZERO_REUSE_LANGUAGE to lua, nickel, or starlark")
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
			next := append(append(append([]byte{}, old[:pos]...), 'x'), old[pos:]...)
			edit := incr.TokenEdit{Start: uint32(pos), OldEnd: uint32(pos), NewEnd: uint32(pos + 1)}
			if !incr.LengthNeutralScannerEdit(scanner, old, next, edit) {
				t.Fatal("witness has no certificate")
			}
			for bits := 0; bits < 1<<uint(count); bits++ {
				valid := make([]bool, count)
				for i := range valid {
					valid[i] = bits&(1<<uint(i)) != 0
				}
				a, b := scanner.Create(), scanner.Create()
				scanner.Deserialize(a, wire[:n])
				scanner.Deserialize(b, wire[:n])
				la, lb := newLengthNeutralLexer(old, 0, 0, 0), newLengthNeutralLexer(next, 0, 0, 0)
				oka, okb := scanner.Scan(a, la, valid), scanner.Scan(b, lb, valid)
				var wa, wb [1024]byte
				na, nb := scanner.Serialize(a, wa[:]), scanner.Serialize(b, wb[:])
				if oka != okb || !bytes.Equal(wa[:na], wb[:nb]) {
					t.Fatalf("%q mask=%x outcomes/payloads differ", witness, bits)
				}
				col, projected := edit.Byte(la.Column())
				if !projected || col != lb.Column() || la.Lookahead() != lb.Lookahead() {
					t.Fatalf("%q mask=%x cursor differs: %d -> %d", witness, bits, la.Column(), lb.Column())
				}
				ta, hasa := lengthNeutralLexerToken(la)
				tb, hasb := lengthNeutralLexerToken(lb)
				if hasa != hasb {
					t.Fatalf("%q mask=%x mark differs", witness, bits)
				}
				if hasa {
					start, sok := edit.Byte(ta.StartByte)
					end, eok := edit.Byte(ta.EndByte)
					if !sok || !eok || start != tb.StartByte || end != tb.EndByte || ta.Symbol != tb.Symbol {
						t.Fatalf("%q mask=%x token differs: %+v -> %+v", witness, bits, ta, tb)
					}
				}
				scanner.Destroy(a)
				scanner.Destroy(b)
			}
		}
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
