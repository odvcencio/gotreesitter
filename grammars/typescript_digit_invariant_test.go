//go:build !grammar_subset || grammar_subset_typescript

package grammars

import (
	"bytes"
	"reflect"
	"testing"
	_ "unsafe"

	gotreesitter "github.com/odvcencio/gotreesitter"
)

func TestTypeScriptScannerASCIIDigitInvariant(t *testing.T) {
	var _ gotreesitter.ASCIIEquivalenceExternalScanner = TypeScriptExternalScanner{}
	remapped := TypeScriptExternalScanner{externalToToken: make([]int, tsTokenCount)}
	for i := 0; i < tsTokenCount; i++ {
		remapped.symbols[i] = gotreesitter.Symbol(300 + i)
		remapped.externalToToken[i] = tsTokenCount - 1 - i
	}
	for _, scanner := range []TypeScriptExternalScanner{{}, remapped} {
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
		warmed := scanner.Create()
		for _, template := range []string{
			"0", "\n0", "\n/*0*/ in x", "? .0", "a0 =", "0>>x", "0${x}`", "\\0", "<!--0\n", "} 0",
		} {
			oldSource, newSource := []byte(template), []byte(template)
			digit := bytes.IndexByte(oldSource, '0')
			for mask := 0; mask < 1<<tsTokenCount; mask++ {
				var valid [tsTokenCount]bool
				for i := range valid {
					valid[i] = mask&(1<<i) != 0
				}
				for oldDigit := byte('0'); oldDigit <= '9'; oldDigit++ {
					for newDigit := byte('0'); newDigit <= '9'; newDigit++ {
						oldSource[digit], newSource[digit] = oldDigit, newDigit
						// Alternate origins to cover contextual and interior scans.
						origin := 0
						if (oldDigit+newDigit)&1 != 0 {
							origin = digit
						}
						a := newTypeScriptDigitExternalLexer(oldSource, origin, 2, 3)
						b := newTypeScriptDigitExternalLexer(newSource, origin, 2, 3)
						fresh := scanner.Create()
						okA := gotreesitter.RunExternalScanner(lang, fresh, a, valid[:])
						okB := gotreesitter.RunExternalScanner(lang, warmed, b, valid[:])
						// Equalize source bytes only. Compare all cursors, marks,
						// result symbols, and examined frontiers, including failures.
						oldSource[digit] = newDigit
						if okA != okB || !reflect.DeepEqual(a, b) {
							t.Fatalf("source=%q mask=%x origin=%d digits=%c/%c: observations differ", template, mask, origin, oldDigit, newDigit)
						}
						var wire [8]byte
						if scanner.Serialize(fresh, wire[:]) != 0 || scanner.Serialize(warmed, wire[:]) != 0 {
							t.Fatal("scanner persisted source state")
						}
						scanner.Deserialize(warmed, nil)
						scanner.Destroy(fresh)
					}
				}
			}
		}
		scanner.Destroy(warmed)
	}
}

//go:linkname newTypeScriptDigitExternalLexer github.com/odvcencio/gotreesitter.newExternalLexer
func newTypeScriptDigitExternalLexer(source []byte, pos int, row, col uint32) *gotreesitter.ExternalLexer
