//go:build !grammar_subset || grammar_subset_python

package grammars

import (
	"bytes"
	"reflect"
	"strings"
	"testing"
	_ "unsafe"

	gotreesitter "github.com/odvcencio/gotreesitter"
)

func TestPythonScannerASCIIDigitInvariant(t *testing.T) {
	scanner := PythonExternalScanner{}
	var _ gotreesitter.ASCIIEquivalenceExternalScanner = scanner
	for b := 0; b < 256; b++ {
		want := uint8(0)
		if b >= '0' && b <= '9' {
			want = 1
		}
		if scanner.ExternalScannerASCIIEquivalenceClass(byte(b)) != want {
			t.Fatalf("unexpected equivalence class for byte %d", b)
		}
	}
	lang := &gotreesitter.Language{ExternalScanner: scanner}
	states := []pythonScannerState{
		{},
		{indents: []uint16{0, 4, 8}},
		{indents: []uint16{0, 8}, delimiters: []pyDelimiter{pyDelimSingleQuote}},
		{indents: []uint16{0}, delimiters: []pyDelimiter{pyDelimDoubleQuote | pyDelimRaw}},
		{indents: []uint16{0}, delimiters: []pyDelimiter{pyDelimSingleQuote | pyDelimBytes}},
		{indents: []uint16{0}, delimiters: []pyDelimiter{pyDelimDoubleQuote | pyDelimTriple}},
		{indents: []uint16{0, 4}, delimiters: []pyDelimiter{pyDelimDoubleQuote | pyDelimFormat}, insideInterpolatedString: true},
		{indents: []uint16{0}, delimiters: []pyDelimiter{pyDelimBackQuote, pyDelimSingleQuote | pyDelimRaw | pyDelimTriple}},
	}
	sources := []string{
		"0", "\n    0", "\n  # 0\n0", "0'", "0\"\"\"", "\\0'", "{{0}", "f0\"", "0\n", "\r\n\t0",
	}
	masks := []uint16{0, (1 << pyTokenCount) - 1}
	for i := 0; i < pyTokenCount; i++ {
		masks = append(masks, 1<<i, ((1<<pyTokenCount)-1)^(1<<i))
	}
	masks = append(masks, 1<<pyTokStringContent|1<<pyTokIndent, 1<<pyTokStringContent|1<<pyTokStringEnd,
		1<<pyTokNewline|1<<pyTokDedent|1<<pyTokStringStart)
	for stateIndex, initial := range states {
		for _, template := range sources {
			for oldDigit := byte('0'); oldDigit <= '9'; oldDigit++ {
				for newDigit := byte('0'); newDigit <= '9'; newDigit++ {
					for _, mask := range masks {
						oldSource := []byte(strings.ReplaceAll(template, "0", string(oldDigit)))
						newSource := []byte(strings.ReplaceAll(template, "0", string(newDigit)))
						oldState, newState := initial, initial
						oldState.indents = append([]uint16(nil), initial.indents...)
						newState.indents = append([]uint16(nil), initial.indents...)
						oldState.delimiters = append([]pyDelimiter(nil), initial.delimiters...)
						newState.delimiters = append([]pyDelimiter(nil), initial.delimiters...)
						var valid [pyTokenCount]bool
						for i := range valid {
							valid[i] = mask&(1<<i) != 0
						}
						oldLexer := newPythonDigitExternalLexer(oldSource, 0, 2, 3)
						newLexer := newPythonDigitExternalLexer(newSource, 0, 2, 3)
						oldOK := gotreesitter.RunExternalScanner(lang, &oldState, oldLexer, valid[:])
						newOK := gotreesitter.RunExternalScanner(lang, &newState, newLexer, valid[:])
						// Equalize only source storage after both scans. Compare every
						// cursor, mark, symbol, and observer field without a layout copy.
						copy(oldSource, newSource)
						if oldOK != newOK || !reflect.DeepEqual(oldLexer, newLexer) || !reflect.DeepEqual(oldState, newState) {
							t.Fatalf("state=%d source=%q digits=%c/%c mask=%x: scan outcomes differ", stateIndex, template, oldDigit, newDigit, mask)
						}
						var oldWire, newWire [128]byte
						oldN := scanner.Serialize(&oldState, oldWire[:])
						newN := scanner.Serialize(&newState, newWire[:])
						if oldN != newN || !bytes.Equal(oldWire[:oldN], newWire[:newN]) {
							t.Fatal("serialized scanner states differ")
						}
					}
				}
			}
		}
	}
}

//go:linkname newPythonDigitExternalLexer github.com/odvcencio/gotreesitter.newExternalLexer
func newPythonDigitExternalLexer(source []byte, pos int, row, col uint32) *gotreesitter.ExternalLexer
