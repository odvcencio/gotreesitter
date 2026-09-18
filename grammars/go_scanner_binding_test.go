package grammars

import (
	"testing"

	gts "github.com/odvcencio/gotreesitter"
)

func TestGoScannerBindingRequiresExternalTokens(t *testing.T) {
	scanner := GoExternalScanner{}
	if scanner.ExternalScannerForLanguage(nil) != nil || scanner.ExternalScannerForLanguage(&gts.Language{}) != nil {
		t.Fatal("a grammar without external tokens received a scanner")
	}
	if scanner.ExternalScannerForLanguage(&gts.Language{ExternalSymbols: []gts.Symbol{1}}) == nil {
		t.Fatal("a legacy grammar lost its external scanner")
	}
}
