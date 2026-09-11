package gotreesitter

import "testing"

func TestNodeTypeCStringNamesPreserveIdentity(t *testing.T) {
	for _, tc := range []struct{ name, want string }{{"\x00", ""}, {"prefix\x00suffix", "prefix"}, {`\0`, `\0`}, {`\?`, "?"}} {
		lang := &Language{SymbolNames: []string{"end", tc.name}}
		node := &Node{symbol: 1}
		if got := node.Type(lang); got != tc.want {
			t.Errorf("Type(%q)=%q want %q", tc.name, got, tc.want)
		}
		if node.symbol != 1 || lang.SymbolNames[1] != tc.name {
			t.Fatal("display changed symbol identity")
		}
		if got := dropZeroWidthUnnamedTail([]*Node{node}, lang); len(got) != 1 || got[0] != node {
			t.Fatalf("dropped named symbol %q", tc.name)
		}
	}
	lang := &Language{SymbolNames: []string{""}}
	if got := dropZeroWidthUnnamedTail([]*Node{{symbol: 0}}, lang); len(got) != 0 {
		t.Fatal("retained empty placeholder")
	}
	if got := dropZeroWidthUnnamedTail([]*Node{{symbol: errorSymbol}}, lang); len(got) != 1 {
		t.Fatal("dropped ERROR identity")
	}
}
