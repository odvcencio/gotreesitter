//go:build !grammar_subset || grammar_subset_tsx

package grammarruntime

import (
	"strings"
	"testing"
)

// TestTsxCModeRulesLikeC pins the scanner the oracle compiles (tree-sitter-typescript
// common/scanner.h at the pinned commit, with grammars/patches/
// tree-sitter-typescript-import-type.patch applied) in the C-faithful mode, one
// rule per case.
func TestTsxCModeRulesLikeC(t *testing.T) {
	scanner := TsxExternalScanner{}
	asi := tsxDefaultSymTable[tsxTokAutoSemicolon]
	mask := func(tokens ...int) []bool {
		m := make([]bool, tsxTokenCount)
		for _, k := range tokens {
			m[k] = true
		}
		return m
	}
	cf := func() any {
		p := scanner.Create().(interface{ SetCFaithful(bool) })
		p.SetCFaithful(true)
		return p
	}
	cases := []struct {
		rule string
		src  string
		pos  int
		mask []bool
		ok   bool
	}{
		{"} returns at once unless a colon follows after white space (scanner.h:90-101): no JSX veto", "c}</", 1, mask(tsxTokAutoSemicolon), true},
		{"} then a colon returns the || flag", "x}:", 1, mask(tsxTokAutoSemicolon), false},
		{"} then a colon with || valid", "x}:", 1, mask(tsxTokAutoSemicolon, tsxTokLogicalOr), true},
		{"} on its own line", "\n}", 0, mask(tsxTokAutoSemicolon), true},
		{"white space is the C locale's: a no-break space is not skipped (scanner.h:105)", " ", 0, mask(tsxTokAutoSemicolon), false},
		{"U+2028 is not white space for the semicolon scan", "x y", 1, mask(tsxTokAutoSemicolon), false},
		{"the TSX scanner patch accepts a literal ampersand", "&", 0, mask(tsxTokJsxText), true},
		{"< after a newline gives a semicolon only when the function-signature flag is set (the oracle's patched scanner.h)", "x\n<", 1, mask(tsxTokAutoSemicolon, tsxTokFuncSigAutoSemi), true},
		{"< after a newline refuses without the function-signature flag", "x\n<", 1, mask(tsxTokAutoSemicolon), false},
		{"in with the || flag off gives a semicolon (the oracle's patched scanner.h)", "x\nin x", 1, mask(tsxTokAutoSemicolon), true},
		{"in with the || flag on, then x refuses the semicolon", "x\nin x", 1, mask(tsxTokAutoSemicolon, tsxTokLogicalOr), false},
		{"a name that starts with i gives a semicolon", "x\nix", 1, mask(tsxTokAutoSemicolon), true},
	}
	for _, c := range cases {
		got := portScanAt(scanner, cf(), c.src, c.pos, c.mask)
		if got.ok != c.ok {
			t.Errorf("%s: %q at %d: ok=%v, want %v", c.rule, c.src, c.pos, got.ok, c.ok)
		}
		if got.ok && c.mask[tsxTokAutoSemicolon] && got.sym != asi {
			t.Errorf("%s: symbol %d, want the automatic semicolon", c.rule, got.sym)
		}
	}
}

// TestTsxCModeReadsPastTheIOfAnIdentifierLikeC: for "x\nix" C skips the "i" and
// examines the next byte when the || flag is set (scanner.h:167-171), so the cursor ends at byte 3, which is
// column 1 of the second line; a scan that returns at the "i" ends at column 0.
func TestTsxCModeReadsPastTheIOfAnIdentifierLikeC(t *testing.T) {
	scanner := TsxExternalScanner{}
	p := scanner.Create().(interface{ SetCFaithful(bool) })
	p.SetCFaithful(true)
	m := make([]bool, tsxTokenCount)
	m[tsxTokAutoSemicolon] = true
	m[tsxTokLogicalOr] = true
	if got := portScanAt(scanner, p, "x\nix", 1, m); !got.ok || got.col != 1 {
		t.Fatalf("got %+v, want a semicolon with the cursor at column 1", got)
	}
}

// The C lexer uses the marked token end for the span and the final cursor for
// its read dependency. A leading entity is left to the internal DFA even if
// the character after its semicolon would satisfy another external rule.
func TestTsxCModeJSXCharacterReferenceBoundary(t *testing.T) {
	scanner := TsxExternalScanner{}
	payload := scanner.Create().(interface{ SetCFaithful(bool) })
	defer scanner.Destroy(payload)
	payload.SetCFaithful(true)
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
		for _, additional := range [][]int{nil, {tsxTokAutoSemicolon, tsxTokTernaryQmark}, {tsxTokHtmlComment}} {
			valid := make([]bool, tsxTokenCount)
			valid[tsxTokJsxText] = true
			for _, token := range additional {
				valid[token] = true
			}
			got := portScan(scanner, payload, tc.input, valid)
			portWantScan(t, tc.input, got, tc.found, tsxDefaultSymTable[tsxTokJsxText], uint32(tc.end))
			if got.pos != tc.cursor || got.frontier != uint32(tc.cursor+1) {
				t.Errorf("%q mask %v: cursor/frontier = %d/%d, want %d/%d", tc.input, additional, got.pos, got.frontier, tc.cursor, tc.cursor+1)
			}
		}
	}
	// Empty input: C's scan_jsx_text sees EOF at once and returns false without
	// advancing (patched common/scanner.h:313-365). When an automatic semicolon
	// is also valid, the dispatcher falls through to scan_automatic_semicolon,
	// which accepts at EOF (scanner.h:82-89, dispatcher at 367-400).
	for _, additional := range [][]int{nil, {tsxTokHtmlComment}} {
		valid := make([]bool, tsxTokenCount)
		valid[tsxTokJsxText] = true
		for _, token := range additional {
			valid[token] = true
		}
		got := portScan(scanner, payload, "", valid)
		if got.ok || got.pos != 0 {
			t.Errorf("empty input mask %v: got %+v, want no token and no advance", additional, got)
		}
	}
	valid := make([]bool, tsxTokenCount)
	valid[tsxTokJsxText], valid[tsxTokAutoSemicolon], valid[tsxTokTernaryQmark] = true, true, true
	got := portScan(scanner, payload, "", valid)
	portWantScan(t, "empty input with automatic semicolon", got, true, tsxDefaultSymTable[tsxTokAutoSemicolon], 0)
	if got.pos != 0 {
		t.Errorf("empty input with automatic semicolon advanced to %d", got.pos)
	}
	// Both masks leave the JSX scan untouched even when the source begins with '&'.
	for _, enabled := range [][]int{{tsxTokTernaryQmark}, {tsxTokJsxText, tsxTokTemplateChars, tsxTokAutoSemicolon}} {
		valid := make([]bool, tsxTokenCount)
		for _, token := range enabled {
			valid[token] = true
		}
		got := portScan(scanner, payload, "&amp;?", valid)
		if got.ok || got.pos != 0 || got.frontier != 1 {
			t.Errorf("mask %v unexpectedly examined the entity: %+v", enabled, got)
		}
	}
}
