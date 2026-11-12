//go:build !grammar_subset || grammar_subset_scss

package grammarruntime

import "testing"

// TestScssAlnumIsAsciiLikeC pins the iswalnum calls of the pinned C scanner
// (tree-sitter-scss src/scanner.c:32 and :53). The oracle runs in the C
// locale, where only ASCII letters and digits are alphanumeric, so "é" is not.
// The port used unicode.IsLetter and unicode.IsDigit.
func TestScssAlnumIsAsciiLikeC(t *testing.T) {
	scanner := ScssExternalScanner{}
	concat := make([]bool, scssTokenCount)
	concat[scssTokConcat] = true
	descendant := make([]bool, scssTokenCount)
	descendant[scssTokDescendantOp] = true
	cases := []struct {
		name  string
		src   string
		valid []bool
		sym   int
		ok    bool
		end   uint32
	}{
		{"concat before ascii letter", "a", concat, scssTokConcat, true, 0},
		{"concat before digit", "7", concat, scssTokConcat, true, 0},
		{"concat before e acute", "é", concat, scssTokConcat, false, 0},
		{"concat before arabic-indic digit", "٣", concat, scssTokConcat, false, 0},
		{"descendant before ascii letter", " a", descendant, scssTokDescendantOp, true, 1},
		{"descendant before e acute", " é", descendant, scssTokDescendantOp, false, 0},
	}
	for _, c := range cases {
		got := portScan(scanner, cFaithfulPayload(scanner), c.src, c.valid)
		portWantScan(t, c.name, got, c.ok, scssDefaultSymTable[c.sym], c.end)
	}
}

// TestScssVerticalTabAndNulLikeC covers two cases. The C locale's
// iswspace is true for the vertical tab, so "\va" gives a descendant operator.
// A NUL byte is not the end of the input (lexer->eof is false), so the colon
// lookahead of the descendant operator goes on over it and finds the "{"
// (scanner.c, the loop after the colon in the descendant branch).
func TestScssVerticalTabAndNulLikeC(t *testing.T) {
	scanner := ScssExternalScanner{}
	valid := make([]bool, scssTokenCount)
	valid[scssTokDescendantOp] = true
	sym := scssDefaultSymTable[scssTokDescendantOp]
	portWantScan(t, "vertical tab", portScan(scanner, cFaithfulPayload(scanner), "\va", valid), true, sym, 1)
	portWantScan(t, "NUL in the lookahead", portScan(scanner, cFaithfulPayload(scanner), " :a\x00{", valid), true, sym, 1)
}
