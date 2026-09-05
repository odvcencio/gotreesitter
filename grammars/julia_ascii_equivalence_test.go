//go:build !grammar_subset || grammar_subset_julia

package grammars

import (
	"bytes"
	"reflect"
	"testing"
	_ "unsafe"

	gotreesitter "github.com/odvcencio/gotreesitter"
)

func TestJuliaScannerASCIIEquivalence(t *testing.T) {
	scanner := JuliaExternalScanner{}
	var _ gotreesitter.ASCIIEquivalenceExternalScanner = scanner
	for b := 0; b < 256; b++ {
		want := uint8(0)
		if b >= 'a' && b <= 'z' || b >= 'A' && b <= 'Z' || b >= '0' && b <= '9' || b == '_' {
			want = 1
		}
		if scanner.ExternalScannerASCIIEquivalenceClass(byte(b)) != want {
			t.Fatalf("unexpected equivalence class for byte %d", b)
		}
	}
	lang := &gotreesitter.Language{ExternalScanner: scanner}
	warmed := scanner.Create()
	defer scanner.Destroy(warmed)
	for _, template := range []string{"T", "#=T=#", "T\"\"\"", "\\T`", "T${x}", "(T"} {
		oldSource, newSource := []byte(template), []byte(template)
		at := bytes.IndexByte(oldSource, 'T')
		for mask := 0; mask < 1<<16; mask++ {
			var valid [16]bool
			for i := range valid {
				valid[i] = mask&(1<<i) != 0
			}
			for _, pair := range [][2]byte{{'T', 'U'}, {'0', '9'}, {'_', 'a'}} {
				oldSource[at], newSource[at] = pair[0], pair[1]
				origin := 0
				if mask&1 != 0 {
					origin = at
				}
				a := newJuliaASCIIExternalLexer(oldSource, origin, 2, 3)
				b := newJuliaASCIIExternalLexer(newSource, origin, 2, 3)
				fresh := scanner.Create()
				okA := gotreesitter.RunExternalScanner(lang, fresh, a, valid[:])
				okB := gotreesitter.RunExternalScanner(lang, warmed, b, valid[:])
				// Equalize only source bytes. Compare complete observations,
				// including failed scans and reads beyond the returned mark.
				oldSource[at] = pair[1]
				if okA != okB || !reflect.DeepEqual(a, b) {
					t.Fatalf("source=%q mask=%x origin=%d pair=%q: observations differ", template, mask, origin, pair)
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

//go:linkname newJuliaASCIIExternalLexer github.com/odvcencio/gotreesitter.newExternalLexer
func newJuliaASCIIExternalLexer(source []byte, pos int, row, col uint32) *gotreesitter.ExternalLexer
