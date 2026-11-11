//go:build !grammar_subset

package grammarruntime

import (
	"testing"

	"github.com/odvcencio/gotreesitter/internal/ccore/bind"
	"github.com/odvcencio/gotreesitter/internal/scanmode"
)

// TestStarlarkIsRefusedByTheCoreUnlessItsPortIsRegistered: starlark shares the python
// state type, but its scanner is served by the core only when its package registers
// the scanner and its payload type. Without that registration the core refuses to
// bind it with a named reason and the present engine parses the language.
func TestStarlarkIsRefusedByTheCoreUnlessItsPortIsRegistered(t *testing.T) {
	if _, ok := scanmode.Lookup(StarlarkExternalScanner{}); ok {
		t.Skip("the starlark port is registered here")
	}
	lang, err := LoadLanguage("starlark", BlobByName("starlark"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := bind.Config(lang); err == nil {
		t.Fatal("the core bound a scanner that no package registered")
	}
}
