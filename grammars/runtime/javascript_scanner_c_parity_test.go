//go:build !grammar_subset || grammar_subset_javascript

package grammarruntime

import (
	"bytes"
	"strings"
	"testing"

	"github.com/odvcencio/gotreesitter/internal/scanmode"
)

func jsCFaithful(scanner JavaScriptExternalScanner) any {
	p := scanner.Create().(interface{ SetCFaithful(bool) })
	p.SetCFaithful(true)
	return p
}

// TestJavaScriptTernaryQmarkMatchesC pins scan_ternary_qmark of
// tree-sitter-javascript (src/scanner.c:225-253) in the C-faithful mode. After
// "?" the scanner marks the token end and returns at once. It does not skip white
// space after the mark and does not reject ":", ")" or "," (those tests belong
// to the TypeScript scanner, common/scanner.h:195-236). "?." is a ternary mark
// only when a digit follows the dot (scanner.c:238-248).
func TestJavaScriptTernaryQmarkMatchesC(t *testing.T) {
	scanner := JavaScriptExternalScanner{}
	valid := make([]bool, jsTokenCount)
	valid[jsTokTernaryQmark] = true
	sym := jsSymTernaryQmark
	cases := []struct {
		name string
		src  string
		ok   bool
		end  uint32
	}{
		{"mark then space", "? ", true, 1},
		{"mark then colon", "? :", true, 1},
		{"mark then close paren", "?)", true, 1},
		{"mark then comma", "?,", true, 1},
		{"mark then end of input", "?", true, 1},
		{"leading space is skipped", "  ? x", true, 3},
		{"double mark", "??", false, 0},
		{"optional chain", "?.a", false, 0},
		{"mark then dot and digit", "?.5", true, 1},
		{"not a mark", "x", false, 0},
	}
	for _, c := range cases {
		got := portScan(scanner, jsCFaithful(scanner), c.src, valid)
		portWantScan(t, c.name, got, c.ok, sym, c.end)
	}
	// C returns at once after the mark (scanner.c:238-247): the cursor stays right
	// after "?" and does not move over the white space that follows. The marked
	// span alone would not show a loop that skips it, so the cursor is checked.
	if got := portScan(scanner, jsCFaithful(scanner), "? x", valid); got.col != 1 {
		t.Errorf("cursor after \"? x\" is at column %d, want 1", got.col)
	}
}

// TestJavaScriptTernaryDefaultModeKeepsTheLongerRule: without the mode the scan
// keeps the rule of the default engine: it skips white space after the mark,
// declines ":" ")" "," and declines "?." even when a digit follows.
func TestJavaScriptTernaryDefaultModeKeepsTheLongerRule(t *testing.T) {
	scanner := JavaScriptExternalScanner{}
	valid := make([]bool, jsTokenCount)
	valid[jsTokTernaryQmark] = true
	for _, src := range []string{"? :", "?)", "?,", "?.5"} {
		if got := portScan(scanner, scanner.Create(), src, valid); got.ok {
			t.Errorf("default mode %q: got %+v, want a decline", src, got)
		}
	}
	portWantScan(t, "default mode \"? x\"", portScan(scanner, scanner.Create(), "? x", valid), true, jsSymTernaryQmark, 1)
}

// TestJavaScriptTernaryWhitespaceIsCLocaleLikeC: the white space before "?" is
// iswspace of the C locale, so a no-break space ends the search (C declines
// " ?"). The default mode skips Unicode white space.
func TestJavaScriptTernaryWhitespaceIsCLocaleLikeC(t *testing.T) {
	scanner := JavaScriptExternalScanner{}
	valid := make([]bool, jsTokenCount)
	valid[jsTokTernaryQmark] = true
	if got := portScan(scanner, jsCFaithful(scanner), " ?", valid); got.ok {
		t.Errorf("C-faithful: a no-break space was skipped, got %+v", got)
	}
	if got := portScan(scanner, jsCFaithful(scanner), "\v?", valid); !got.ok {
		t.Errorf("C-faithful: a vertical tab must be skipped")
	}
	if got := portScan(scanner, scanner.Create(), " ?", valid); !got.ok {
		t.Errorf("default mode: Unicode white space is skipped, got %+v", got)
	}
}

// TestJavaScriptAutoSemicolonAtRangeStartLikeC: C returns an automatic semicolon
// at the first byte of an included range whatever follows (src/scanner.c:133-135,
// lexer->is_at_included_range_start). With only the automatic semicolon valid and
// the input "a" at byte 0, the C-faithful mode finds it (an empty token); the
// default mode does not ask that question and finds none.
func TestJavaScriptAutoSemicolonAtRangeStartLikeC(t *testing.T) {
	scanner := JavaScriptExternalScanner{}
	valid := make([]bool, jsTokenCount)
	valid[jsTokAutoSemicolon] = true
	portWantScan(t, "C-faithful", portScan(scanner, jsCFaithful(scanner), "a", valid), true, jsDefaultSymTable[jsTokAutoSemicolon], 0)
	if got := portScan(scanner, scanner.Create(), "a", valid); got.ok {
		t.Errorf("default mode: got %+v, want no token", got)
	}
}

// TestJavaScriptJsxTextKeepsLiteralAmpersandLikePatchedC: after
// grammars/patches/tree-sitter-javascript-jsx-text-ampersand.patch the C scanner
// keeps a bare "&" in JSX text (src/scanner.c:319-370), so "&" alone is text and
// " &" is text over both bytes. The default mode agrees.
func TestJavaScriptJsxTextKeepsLiteralAmpersandLikePatchedC(t *testing.T) {
	scanner := JavaScriptExternalScanner{}
	valid := make([]bool, jsTokenCount)
	valid[jsTokJsxText] = true
	sym := jsDefaultSymTable[jsTokJsxText]
	portWantScan(t, "C-faithful \"&\"", portScan(scanner, jsCFaithful(scanner), "&", valid), true, sym, 1)
	portWantScan(t, "C-faithful \" &\"", portScan(scanner, jsCFaithful(scanner), " &", valid), true, sym, 2)
	if got := portScan(scanner, scanner.Create(), "&", valid); !got.ok {
		t.Errorf("default mode: a bare \"&\" is text, got %+v", got)
	}
}

// TestJavaScriptCModeJSXCharacterReferenceBoundary pins the cursor of the patched
// scan_jsx_text: the C lexer uses the marked token end for the span and the final
// cursor for its read dependency. A leading reference is left to the internal
// lexer even when the character after its semicolon would satisfy another
// external rule (the patched dispatch returns at once, src/scanner.c:380-385).
func TestJavaScriptCModeJSXCharacterReferenceBoundary(t *testing.T) {
	scanner := JavaScriptExternalScanner{}
	cases := []struct {
		input       string
		found       bool
		end, cursor int
	}{
		{"A &amp;}", true, 2, 7},
		{"&amp;}", false, 0, 5},
		{"&amp;?x", false, 0, 5},
		{"&amp;", false, 0, 5},
		{"&#38;-->", false, 0, 5},
		{"&#x26;?", false, 0, 6},
		{"AT&T<", true, 4, 4},
		{"A & B &amp;?", true, 6, 11},
		{"A &amp B<", true, 8, 8},
		{"&#x;{", true, 4, 4},
		{"&#123456;{", true, 9, 9},
		{"&#x1234567;{", true, 11, 11},
		{"&" + strings.Repeat("a", 30) + ";?", false, 0, 32},
		{"&" + strings.Repeat("a", 31) + ";<", true, 33, 33},
	}
	for _, tc := range cases {
		for _, additional := range [][]int{nil, {jsTokAutoSemicolon, jsTokTernaryQmark}, {jsTokHtmlComment}} {
			valid := jsMask(append([]int{jsTokJsxText}, additional...)...)
			got := portScan(scanner, jsCFaithful(scanner), tc.input, valid)
			portWantScan(t, tc.input, got, tc.found, jsDefaultSymTable[jsTokJsxText], uint32(tc.end))
			if got.pos != tc.cursor || got.frontier != uint32(tc.cursor+1) {
				t.Errorf("%q mask %v: cursor/frontier = %d/%d, want %d/%d", tc.input, additional, got.pos, got.frontier, tc.cursor, tc.cursor+1)
			}
		}
	}
	// Empty input: scan_jsx_text sees the end at once and returns false without
	// advancing. With an automatic semicolon also valid, the dispatcher goes on to
	// scan_automatic_semicolon, which accepts at the end of input.
	for _, additional := range [][]int{nil, {jsTokHtmlComment}} {
		got := portScan(scanner, jsCFaithful(scanner), "", jsMask(append([]int{jsTokJsxText}, additional...)...))
		if got.ok || got.pos != 0 {
			t.Errorf("empty input mask %v: got %+v, want no token and no advance", additional, got)
		}
	}
	got := portScan(scanner, jsCFaithful(scanner), "", jsMask(jsTokJsxText, jsTokAutoSemicolon, jsTokTernaryQmark))
	portWantScan(t, "empty input with automatic semicolon", got, true, jsDefaultSymTable[jsTokAutoSemicolon], 0)
	if got.pos != 0 {
		t.Errorf("empty input with automatic semicolon advanced to %d", got.pos)
	}
	// Neither mask reaches the JSX scan, even when the source begins with '&'.
	for _, enabled := range [][]int{{jsTokTernaryQmark}, {jsTokJsxText, jsTokTemplateChars, jsTokAutoSemicolon}} {
		got := portScan(scanner, jsCFaithful(scanner), "&amp;?", jsMask(enabled...))
		if got.ok || got.pos != 0 || got.frontier != 1 {
			t.Errorf("mask %v unexpectedly examined the entity: %+v", enabled, got)
		}
	}
}

// TestJavaScriptJsxTextBeforeAutoSemicolonLikeC: with both valid, C tries JSX
// text first (src/scanner.c:339-343), so "\nx" gives JSX text over both bytes; the
// default mode prefers the automatic semicolon at a line end.
func TestJavaScriptJsxTextBeforeAutoSemicolonLikeC(t *testing.T) {
	scanner := JavaScriptExternalScanner{}
	valid := make([]bool, jsTokenCount)
	valid[jsTokJsxText] = true
	valid[jsTokAutoSemicolon] = true
	portWantScan(t, "C-faithful", portScan(scanner, jsCFaithful(scanner), "\nx", valid), true, jsDefaultSymTable[jsTokJsxText], 2)
	got := portScan(scanner, scanner.Create(), "\nx", valid)
	if !got.ok || got.sym != jsDefaultSymTable[jsTokAutoSemicolon] {
		t.Errorf("default mode: got %+v, want the automatic semicolon", got)
	}
}

// jsMask returns a valid-symbol mask with the given external tokens set.
func jsMask(tokens ...int) []bool {
	m := make([]bool, jsTokenCount)
	for _, t := range tokens {
		m[t] = true
	}
	return m
}

// TestJavaScriptAutoSemicolonRulesLikeC pins scan_automatic_semicolon and
// scan_whitespace_and_comments of the pinned C scanner (src/scanner.c:61-221) in the
// C-faithful mode, one rule per case. Every scan starts at byte pos with an empty state.
func TestJavaScriptAutoSemicolonRulesLikeC(t *testing.T) {
	scanner := JavaScriptExternalScanner{}
	asi := jsDefaultSymTable[jsTokAutoSemicolon]
	A, T, H := jsTokAutoSemicolon, jsTokTernaryQmark, jsTokHtmlComment
	cases := []struct {
		rule string // the C rule
		src  string
		pos  int
		mask []bool
		ok   bool
		end  uint32 // token end when ok
		col  int    // expected cursor column after the scan; -1 not checked
	}{
		{"white space is iswspace of the C locale (:65, :138): a no-break space is not skipped", "a ? b : c;\n", 1, jsMask(A, T), false, 0, -1},
		{"U+2028 is a line terminator (:149)", "x y", 1, jsMask(A), true, 1, -1},
		{"U+2029 is a line terminator (:149)", "x y", 1, jsMask(A), true, 1, -1},
		{"after a newline a no-break space is not white space (:65)", "x\n +", 1, jsMask(A), true, 1, -1},
		{"html comment: leading no-break space is not skipped (:256)", " <!--x", 0, jsMask(H), false, 0, -1},
		{"html comment: a vertical tab and U+2028 are skipped (:256)", "\v <!--x", 0, jsMask(H), true, 0, -1},
		{"before } C returns at once (:130): no read past the brace", "x}:", 1, jsMask(A), true, 1, 1},
		{"before } C returns at once, followed by a letter", "x}x", 1, jsMask(A), true, 1, 1},
		{"a dot before a digit gives a semicolon (:174-177)", "x\n.5\n", 1, jsMask(A), true, 1, -1},
		{"a dot before a letter does not", "x\n.a", 1, jsMask(A), false, 0, -1},
		{"( after a newline never gives a semicolon (:155-171)", "x\n(", 1, jsMask(A), false, 0, -1},
		{"[ after a newline never gives a semicolon", "x\n[", 1, jsMask(A), false, 0, -1},
		{"in followed by a non-ASCII letter: iswalpha is ASCII only (:194-214)", "x\niné", 1, jsMask(A), false, 0, -1},
		{"instanceof followed by a non-ASCII letter", "x\ninstanceofé", 1, jsMask(A), false, 0, -1},
		{"in followed by a space declines (:203-205)", "x\nin x", 1, jsMask(A), false, 0, -1},
		{"a completed block comment sets scanned_comment, so no ternary fallback (:74-96, :348)", "x\n/**/?", 1, jsMask(A, T), false, 0, -1},
		{"a line comment stops at U+2028 (:74-96)", "x\n// \n,", 1, jsMask(A), true, 1, -1},
		{"a line comment stops at U+2029", "x\n// \n,", 1, jsMask(A), true, 1, -1},
	}
	for _, c := range cases {
		got := portScanAt(scanner, jsCFaithful(scanner), c.src, c.pos, c.mask)
		if got.ok != c.ok {
			t.Errorf("%s: %q at %d: ok=%v, want %v", c.rule, c.src, c.pos, got.ok, c.ok)
			continue
		}
		if c.ok && !got.hasTok {
			t.Errorf("%s: the scan succeeded without producing a token", c.rule)
			continue
		}
		if c.ok && c.mask[A] && (got.sym != asi || got.end != c.end) {
			t.Errorf("%s: token symbol %d end %d, want the automatic semicolon ending at %d", c.rule, got.sym, got.end, c.end)
		}
		if c.ok && c.mask[H] && (got.sym != jsDefaultSymTable[jsTokHtmlComment] || got.start != 4 || got.end != 9) {
			t.Errorf("%s: html comment token symbol %d span %d-%d, want span 4-9", c.rule, got.sym, got.start, got.end)
		}
		if c.col >= 0 && int(got.col) != c.col {
			t.Errorf("%s: cursor column %d, want %d", c.rule, got.col, c.col)
		}
	}
}

func TestScannerSnapshotPrivateConfiguration(t *testing.T) {
	s := JavaScriptExternalScanner{externalToToken: []int{1, -1, 3}}
	s.symbols[0] = 12
	reg, ok := scanmode.Lookup(s)
	if !ok || reg.Snapshot == nil {
		t.Fatal("missing snapshot")
	}
	v, key, ok := reg.Snapshot(s)
	if !ok {
		t.Fatal("snapshot failed")
	}
	private := v.(JavaScriptExternalScanner)
	if reg.SameConfig == nil || !reg.SameConfig(s, private) {
		t.Fatal("snapshot configuration did not match")
	}
	saved := append([]byte(nil), key...)
	s.externalToToken[0] = 9
	s.symbols[0] = 15
	if reg.SameConfig(s, private) {
		t.Fatal("changed configuration still matched the snapshot")
	}
	if private.externalToToken[0] != 1 || private.symbols[0] != 12 || !bytes.Equal(key, saved) {
		t.Fatal("snapshot shares mutable state")
	}
	_, changed, ok := reg.Snapshot(s)
	if !ok || bytes.Equal(key, changed) {
		t.Fatal("changed configuration has same key")
	}
}
