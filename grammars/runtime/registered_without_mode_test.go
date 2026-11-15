//go:build !grammar_subset

package grammarruntime

import (
	"testing"

	gotreesitter "github.com/odvcencio/gotreesitter"
)

// TestScannersRegisteredWithoutAModeBindThroughTheCore: these ports equal their pinned C
// scanners call by call as they are (cgo_harness scandiff), so they are registered without
// a mode; this test pins each registration: it exists, its payload type is the type Create
// returns, the payload has no mode, and the scanner binds through the core directly and
// behind one and two order adapters.
func TestScannersRegisteredWithoutAModeBindThroughTheCore(t *testing.T) {
	for _, c := range []struct {
		lang    string
		scanner gotreesitter.ExternalScanner
	}{
		{"beancount", BeancountExternalScanner{}},
		{"cooklang", CooklangExternalScanner{}},
		{"dhall", DhallExternalScanner{}},
		{"elixir", ElixirExternalScanner{}},
		{"gn", GnExternalScanner{}},
		{"haxe", HaxeExternalScanner{}},
		{"jsdoc", JsdocExternalScanner{}},
		{"kdl", KdlExternalScanner{}},
		{"move", MoveExternalScanner{}},
		{"nix", NixExternalScanner{}},
		{"toml", TomlExternalScanner{}},
		{"yuck", YuckExternalScanner{}},
	} {
		t.Run(c.lang, func(t *testing.T) {
			checkRegisteredScanner(t, c.lang, c.scanner, false, nil)
		})
	}
}
