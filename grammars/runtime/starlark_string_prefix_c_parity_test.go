//go:build !grammar_subset || grammar_subset_starlark

package grammarruntime

import (
	"strings"
	"testing"
)

// TestStarlarkStringPrefixMatchesC pins the prefix letters of the pinned C
// scanner (tree-sitter-starlark src/scanner.c:306-314): f F r R b B u U only.
// The port also took t and T, which the C scanner treats as the end of the
// prefix, so T" is not a string start there.
func TestStarlarkStringPrefixMatchesC(t *testing.T) {
	scanner := StarlarkExternalScanner{}
	valid := make([]bool, slTokenCount)
	valid[slTokStringStart] = true
	sym := slDefaultSymTable[slTokStringStart]
	cases := []struct {
		src string
		ok  bool
		end uint32
	}{
		{`"a"`, true, 1},
		{`f"a"`, true, 2},
		{`rb"a"`, true, 3},
		{`U"a"`, true, 2},
		{`t"a"`, false, 0},
		{`T"a"`, false, 0},
		{"`a`", true, 1},
	}
	for _, c := range cases {
		got := portScan(scanner, cFaithfulPayload(scanner), c.src, valid)
		portWantScan(t, c.src, got, c.ok, sym, c.end)
	}
}

// TestStarlarkIndentWidthIs32BitLikeC covers this case: C counts the
// indent in a uint32_t (tree-sitter-starlark scanner.c:214), so 65,536 spaces
// give an indent that is larger than the sentinel 0; a 16-bit counter wraps to 0
// and the INDENT is refused.
func TestStarlarkIndentWidthIs32BitLikeC(t *testing.T) {
	scanner := StarlarkExternalScanner{}
	valid := make([]bool, slTokenCount)
	valid[slTokIndent] = true
	got := portScan(scanner, cFaithfulPayload(scanner), "\n"+strings.Repeat(" ", 65536)+"x", valid)
	if !got.ok || got.sym != slDefaultSymTable[slTokIndent] {
		t.Fatalf("65,536 spaces: Scan = %+v, want INDENT", got)
	}
}
