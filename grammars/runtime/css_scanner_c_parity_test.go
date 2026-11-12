//go:build !grammar_subset || grammar_subset_css

package grammarruntime

import "testing"

// TestCssPseudoClassColonScanMatchesC pins the lookahead loop that decides
// whether a ":" starts a pseudo-class (tree-sitter-css src/scanner.c:59-95).
// C advances first and then reads the new lookahead, so a "{" directly after
// the colon is consumed and does not end the scan, and a "{" counts only when
// it is the lookahead after an advance. At the end of input the token is still
// returned (scanner.c:95). The port tested the character it had just
// passed, which differs for ":{" and reads one byte further than C.
func TestCssPseudoClassColonScanMatchesC(t *testing.T) {
	scanner := CssExternalScanner{}
	valid := make([]bool, cssTokenCount)
	valid[cssTokColon] = true
	sym := cssDefaultSymTable[cssTokColon]
	cases := []struct {
		name string
		src  string
		ok   bool
	}{
		{"pseudo-class before block", ":hover {", true},
		{"block right after the colon", ":{}", false},
		{"property", ":a;", false},
		{"end of input", ":a", true},
		{"pseudo-element", "::a {", false},
		{"brace inside a comment", ":a/*{*/b{", true},
		{"only a brace in a comment", ":a/*{*/;", false},
	}
	for _, c := range cases {
		got := portScan(scanner, cFaithfulPayload(scanner), c.src, valid)
		portWantScan(t, c.name, got, c.ok, sym, 1)
	}
}

// TestCssDescendantOpAlnumIsAsciiLikeC pins the iswalnum call of the pinned C
// scanner (tree-sitter-css src/scanner.c:38): ASCII letters and digits only.
func TestCssDescendantOpAlnumIsAsciiLikeC(t *testing.T) {
	scanner := CssExternalScanner{}
	valid := make([]bool, cssTokenCount)
	valid[cssTokDescendantOp] = true
	sym := cssDefaultSymTable[cssTokDescendantOp]
	got := portScan(scanner, cFaithfulPayload(scanner), " a", valid)
	portWantScan(t, "space then ascii letter", got, true, sym, 1)
	got = portScan(scanner, cFaithfulPayload(scanner), " é", valid)
	portWantScan(t, "space then e acute", got, false, sym, 0)
}

// TestCssNulWhitespaceAndResultLikeC covers three cases.
// a NUL byte is not the end of the input (lexer->eof is false), so
// ":a\x00;" does not reach the end; the colon scan stops at the ";" and is a
// property, which C declines (scanner.c, the colon loop). the vertical
// tab is white space in the C locale, so "\vfoo" gives a descendant operator.
// C assigns DESCENDANT_OP before it skips (scanner.c:29), so the scan
// of " $" fails but leaves a result symbol.
func TestCssNulWhitespaceAndResultLikeC(t *testing.T) {
	scanner := CssExternalScanner{}
	colon := make([]bool, cssTokenCount)
	colon[cssTokColon] = true
	if got := portScan(scanner, cFaithfulPayload(scanner), ":a\x00;", colon); got.ok {
		t.Errorf("colon over NUL: got %+v, want a decline", got)
	}
	desc := make([]bool, cssTokenCount)
	desc[cssTokDescendantOp] = true
	sym := cssDefaultSymTable[cssTokDescendantOp]
	// The colon lookahead of the descendant operator also goes on over a NUL byte
	// (scanner.c:48-57): " :a\x00{" is a descendant operator that ends after the space.
	portWantScan(t, "NUL in the descendant lookahead", portScan(scanner, cFaithfulPayload(scanner), " :a\x00{", desc), true, cssDefaultSymTable[cssTokDescendantOp], 1)
	portWantScan(t, "vertical tab", portScan(scanner, cFaithfulPayload(scanner), "\vfoo", desc), true, sym, 1)
	got := portScan(scanner, cFaithfulPayload(scanner), " $", desc)
	if got.ok || !got.hasTok {
		t.Errorf("\" $\": ok=%v hasTok=%v, want a failed scan that keeps its result symbol", got.ok, got.hasTok)
	}
}
