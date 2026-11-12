//go:build !grammar_subset || grammar_subset_tsx

package grammarruntime

import "testing"

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
		{"JSX text ends at & (scanner.h:271-310)", "&", 0, mask(tsxTokJsxText), false},
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
