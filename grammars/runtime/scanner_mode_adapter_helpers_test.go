//go:build !grammar_subset

package grammarruntime

import (
	"strings"
	"testing"

	gotreesitter "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/internal/ccore"
	"github.com/odvcencio/gotreesitter/internal/ccore/bind"
)

// wrappedCoreScanner loads language name, wraps its scanner in `layers` order
// adapters with identity mappings (AdaptExternalScannerByExternalOrder over the
// previous layer) and returns the scanner the core would run, with the
// external index of each named external symbol.
func wrappedCoreScanner(t *testing.T, name string, layers int) (ccore.ExtScanner, *gotreesitter.Language) {
	t.Helper()
	prev, err := LoadLanguage(name, BlobByName(name))
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < layers; i++ {
		next, err := LoadLanguage(name, BlobByName(name))
		if err != nil {
			t.Fatal(err)
		}
		adapted, ok := gotreesitter.AdaptExternalScannerByExternalOrder(prev, next)
		if !ok {
			t.Fatal("scanner adaptation refused")
		}
		next.ExternalScanner = adapted
		prev = next
	}
	cfg, err := bind.Config(prev)
	if err != nil {
		t.Fatal(err)
	}
	return cfg.Scanner, prev
}

func externalIndex(t *testing.T, lang *gotreesitter.Language, symbolName string) int {
	t.Helper()
	for i, sym := range lang.ExternalSymbols {
		if lang.SymbolNames[sym] == symbolName {
			return i
		}
	}
	// Some grammars name an external with a leading underscore; match that too,
	// and a name that contains the wanted one without the other candidate "switch_".
	for i, sym := range lang.ExternalSymbols {
		name := lang.SymbolNames[sym]
		if strings.Contains(name, symbolName) && (strings.Contains(symbolName, "switch") || !strings.Contains(name, "switch")) {
			return i
		}
	}
	t.Fatalf("no external symbol %q", symbolName)
	return -1
}
