//go:build !grammar_subset || grammar_subset_typescript

package grammarruntime

import "testing"

// TestTypeScriptCModeRulesLikeC pins the scanner the oracle compiles (tree-sitter-typescript
// common/scanner.h at the pinned commit, with grammars/patches/
// tree-sitter-typescript-import-type.patch applied) in the C-faithful mode, one
// rule per case.
func TestTypeScriptCModeRulesLikeC(t *testing.T) {
	scanner := TypeScriptExternalScanner{}
	asi := tsDefaultSymTable[tsTokAutoSemicolon]
	mask := func(tokens ...int) []bool {
		m := make([]bool, tsTokenCount)
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
		{"} returns at once unless a colon follows after white space (scanner.h:90-101): no JSX veto", "c}</", 1, mask(tsTokAutoSemicolon), true},
		{"} then a colon returns the || flag", "x}:", 1, mask(tsTokAutoSemicolon), false},
		{"} then a colon with || valid", "x}:", 1, mask(tsTokAutoSemicolon, tsTokLogicalOr), true},
		{"} on its own line", "\n}", 0, mask(tsTokAutoSemicolon), true},
		{"white space is the C locale's: a no-break space is not skipped (scanner.h:105)", " ", 0, mask(tsTokAutoSemicolon), false},
		{"U+2028 is not white space for the semicolon scan", "x y", 1, mask(tsTokAutoSemicolon), false},
		{"JSX text ends at & (scanner.h:271-310)", "&", 0, mask(tsTokJsxText), false},
		{"< after a newline gives a semicolon only when the function-signature flag is set (the oracle's patched scanner.h)", "x\n<", 1, mask(tsTokAutoSemicolon, tsTokFuncSigAutoSemi), true},
		{"< after a newline refuses without the function-signature flag", "x\n<", 1, mask(tsTokAutoSemicolon), false},
		{"in with the || flag off gives a semicolon (the oracle's patched scanner.h)", "x\nin x", 1, mask(tsTokAutoSemicolon), true},
		{"in with the || flag on, then x refuses the semicolon", "x\nin x", 1, mask(tsTokAutoSemicolon, tsTokLogicalOr), false},
		{"a name that starts with i gives a semicolon", "x\nix", 1, mask(tsTokAutoSemicolon), true},
	}
	for _, c := range cases {
		got := portScanAt(scanner, cf(), c.src, c.pos, c.mask)
		if got.ok != c.ok {
			t.Errorf("%s: %q at %d: ok=%v, want %v", c.rule, c.src, c.pos, got.ok, c.ok)
		}
		if got.ok && c.mask[tsTokAutoSemicolon] && got.sym != asi {
			t.Errorf("%s: symbol %d, want the automatic semicolon", c.rule, got.sym)
		}
	}
}

// TestTypeScriptCModeReadsPastTheIOfAnIdentifierLikeC: for "x\nix" C skips the "i" and
// examines the next byte when the || flag is set (scanner.h:167-171), so the cursor ends at byte 3, which is
// column 1 of the second line; a scan that returns at the "i" ends at column 0.
func TestTypeScriptCModeReadsPastTheIOfAnIdentifierLikeC(t *testing.T) {
	scanner := TypeScriptExternalScanner{}
	p := scanner.Create().(interface{ SetCFaithful(bool) })
	p.SetCFaithful(true)
	m := make([]bool, tsTokenCount)
	m[tsTokAutoSemicolon] = true
	m[tsTokLogicalOr] = true
	if got := portScanAt(scanner, p, "x\nix", 1, m); !got.ok || got.col != 1 {
		t.Fatalf("got %+v, want a semicolon with the cursor at column 1", got)
	}
}
