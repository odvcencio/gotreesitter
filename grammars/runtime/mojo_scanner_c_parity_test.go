//go:build !grammar_subset || grammar_subset_mojo

package grammarruntime

import "testing"

// TestMojoStringStartMatchesC pins the string-start rules of the pinned C
// scanner (tree-sitter-mojo src/scanner.c:285-360): a string starts with a
// single or double quote after optional prefix letters. A backtick does not
// start a string (the port used to accept it, the C scanner has no such
// case). The prefix letters are f F t T r R b B u U (scanner.c:324-336).
func TestMojoStringStartMatchesC(t *testing.T) {
	scanner := MojoExternalScanner{}
	valid := make([]bool, mojoTokenCount)
	valid[mojoTokStringStart] = true
	sym := mojoDefaultSymTable[mojoTokStringStart]
	cases := []struct {
		src string
		ok  bool
		end uint32
	}{
		{"'a'", true, 1},
		{`"a"`, true, 1},
		{"f'a'", true, 2},
		{"t\"a\"", true, 2},
		{"`a`", false, 0},
		{"f`a`", false, 0},
	}
	for _, c := range cases {
		got := portScan(scanner, cFaithfulPayload(scanner), c.src, valid)
		portWantScan(t, c.src, got, c.ok, sym, c.end)
	}
}

// TestMojoDedentBeforeBacktickFollowsValidSymbols covers this case.
// With indents [0,4], no delimiters, only STRING_START valid and the input
// "\n`x`", the C scanner returns DEDENT (a backtick is not a quote at
// scanner.c:298-300) although DEDENT is not valid. The legacy mode keeps the
// backtick in that test and returns no DEDENT; the C-faithful mode returns it.
func TestMojoDedentBeforeBacktickFollowsValidSymbols(t *testing.T) {
	scanner := MojoExternalScanner{}
	valid := make([]bool, mojoTokenCount)
	valid[mojoTokStringStart] = true
	src := "\n`x`"
	legacy := scanner.Create().(*pythonScannerState)
	legacy.Indents = []uint16{0, 4}
	// Main's scan (mode off) reads the backtick as a string start and returns STRING_START.
	if got := portScan(scanner, legacy, src, valid); got.ok && got.sym == mojoDefaultSymTable[mojoTokDedent] {
		t.Errorf("legacy mode returned DEDENT (%+v) although only STRING_START is valid", got)
	}
	cf := scanner.Create().(*pythonScannerState)
	cf.SetCFaithful(true)
	cf.Indents = []uint16{0, 4}
	got := portScan(scanner, cf, src, valid)
	portWantScan(t, "C-faithful dedent", got, true, mojoDefaultSymTable[mojoTokDedent], 0)
	if len(cf.Indents) != 1 {
		t.Errorf("C-faithful mode left indents %v, want the stack popped", cf.Indents)
	}
}

// TestMojoUsesTheExternalOrderMappingLikePython: the
// valid-symbol mask arrives in external-index order of the bound language and
// must be mapped to the scanner's token order. External index 0 maps to
// STRING_START here; enabling only that entry must make `"x"` a string start.
func TestMojoUsesTheExternalOrderMappingLikePython(t *testing.T) {
	m := make([]int, mojoTokenCount)
	for i := range m {
		m[i] = i
	}
	m[0], m[mojoTokStringStart] = mojoTokStringStart, m[0]
	scanner := MojoExternalScanner{externalToToken: m}
	valid := make([]bool, mojoTokenCount)
	valid[0] = true
	got := portScan(scanner, cFaithfulPayload(scanner), `"x"`, valid)
	portWantScan(t, "string start through external index 0", got, true, mojoDefaultSymTable[mojoTokStringStart], 1)
}
