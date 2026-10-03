package grammarruntime

import (
	"crypto/sha256"
	"testing"

	gotreesitter "github.com/odvcencio/gotreesitter"
)

func TestGoStringTerminalsRemainVisibleBeforeReduction(t *testing.T) {
	lang := GoLanguage()
	for _, name := range []string{"raw_string_literal_content", "interpreted_string_literal_content"} {
		found := false
		for symbol, symbolName := range lang.SymbolNames {
			if uint32(symbol) >= lang.TokenCount || symbolName != name {
				continue
			}
			found = true
			if !lang.SymbolMetadata[symbol].Visible || !lang.SymbolMetadata[symbol].Named {
				t.Fatalf("terminal %s is hidden before its enclosing string reduces", name)
			}
		}
		if !found {
			t.Fatalf("missing visible terminal %s", name)
		}
	}
	uncertified := &gotreesitter.Language{
		TokenCount:     2,
		SymbolNames:    []string{"end", "_interpreted_string_literal_token1"},
		SymbolMetadata: make([]gotreesitter.SymbolMetadata, 2),
	}
	if attachBuiltinLanguageRuntimeProfile("go", sha256.Sum256([]byte("stale grammar")), uncertified) || uncertified.SymbolMetadata[1].Visible {
		t.Fatal("an uncertified grammar acquired the visible terminal profile")
	}
}
