//go:build !grammar_subset || (grammar_subset_css && grammar_subset_scss)

package grammars

import (
	"reflect"
	"testing"
	_ "unsafe"

	gotreesitter "github.com/odvcencio/gotreesitter"
)

func TestCSSFamilyScannerStatelessContract(t *testing.T) {
	for _, test := range []struct {
		name    string
		scanner gotreesitter.StatelessExternalScanner
		count   int
	}{
		{"css", CssExternalScanner{}, 3},
		{"scss", ScssExternalScanner{}, 4},
	} {
		t.Run(test.name, func(t *testing.T) {
			if !test.scanner.ExternalScannerIsStateless() {
				t.Fatal("scanner did not certify statelessness")
			}
			lang := &gotreesitter.Language{ExternalScanner: test.scanner}
			warmed := test.scanner.Create()
			defer test.scanner.Destroy(warmed)
			// Include successful probes and failed probes beyond the token mark.
			for _, source := range []string{"", " div", " :hover {", " :hover;", ":hover {", ":hover;", ":/* {} */hover {", "::before", "#{name}", "#name", "\n\t.é", ":😁"} {
				for origin := 0; origin <= len(source); origin++ {
					for mask := 0; mask < 1<<test.count; mask++ {
						valid := make([]bool, test.count)
						for i := range valid {
							valid[i] = mask&(1<<i) != 0
						}
						fresh := test.scanner.Create()
						a := newCSSContractExternalLexer([]byte(source), origin, 2, 3)
						b := newCSSContractExternalLexer([]byte(source), origin, 2, 3)
						okA := gotreesitter.RunExternalScanner(lang, fresh, a, valid)
						okB := gotreesitter.RunExternalScanner(lang, warmed, b, valid)
						// Compare cursors, marks, symbols, and read observers, including failures.
						if okA != okB || !reflect.DeepEqual(a, b) {
							t.Fatalf("source=%q origin=%d mask=%x: scanner history changed observation", source, origin, mask)
						}
						var wire [16]byte
						if test.scanner.Serialize(fresh, wire[:]) != 0 || test.scanner.Serialize(warmed, wire[:]) != 0 {
							t.Fatal("scanner persisted token state")
						}
						test.scanner.Deserialize(warmed, nil)
						test.scanner.Destroy(fresh)
					}
				}
			}
		})
	}
}

//go:linkname newCSSContractExternalLexer github.com/odvcencio/gotreesitter.newExternalLexer
func newCSSContractExternalLexer(source []byte, pos int, row, col uint32) *gotreesitter.ExternalLexer
