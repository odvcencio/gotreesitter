//go:build !grammar_subset

package grammarruntime

import (
	"testing"

	gotreesitter "github.com/odvcencio/gotreesitter"
)

type registeredWithoutModeCase struct {
	lang    string
	scanner gotreesitter.ExternalScanner
}

var extraRegisteredWithoutModeCases []registeredWithoutModeCase

// TestScannersRegisteredWithoutAModeBindThroughTheCore: these ports equal their pinned C
// scanners call by call as they are (cgo_harness scandiff), so they are registered without
// a mode; this test pins each registration: it exists, its payload type is the type Create
// returns, the payload has no mode, and the scanner binds through the core directly and
// behind one and two order adapters.
func TestScannersRegisteredWithoutAModeBindThroughTheCore(t *testing.T) {
	for _, c := range append([]registeredWithoutModeCase{
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
		{"tablegen", TablegenExternalScanner{}},
		{"nushell", NushellExternalScanner{}},
		{"godot_resource", GodotResourceExternalScanner{}},
		{"julia", JuliaExternalScanner{}},
		{"ron", RonExternalScanner{}},
		{"lua", LuaExternalScanner{}},
		{"fennel", FennelExternalScanner{}},
		{"dart", DartExternalScanner{}},
		{"dockerfile", DockerfileExternalScanner{}},
		{"erlang", ErlangExternalScanner{}},
		{"kotlin", KotlinExternalScanner{}},
		{"r", RExternalScanner{}},
		{"markdown_inline", MarkdownInlineExternalScanner{}},
		{"gitcommit", GitcommitExternalScanner{}},
		{"powershell", PowershellExternalScanner{}},
		{"luau", LuauExternalScanner{}},
		{"odin", OdinExternalScanner{}},
		{"fish", FishExternalScanner{}},
		{"tcl", TclExternalScanner{}},
		{"liquid", LiquidExternalScanner{}},
		{"janet", JanetExternalScanner{}},
		{"pkl", PklExternalScanner{}},
		{"editorconfig", EditorconfigExternalScanner{}},
		{"dtd", DtdExternalScanner{}},
		{"org", OrgExternalScanner{}},
		{"ocaml", OcamlExternalScanner{}},
		{"comment", CommentExternalScanner{}},
		{"cue", CueExternalScanner{}},
		{"kconfig", KconfigExternalScanner{}},
		{"xml", XMLExternalScanner{}},
		{"fortran", FortranExternalScanner{}},
		{"fsharp", FsharpExternalScanner{}},
		{"markdown", MarkdownExternalScanner{}},
		{"c_sharp", CSharpExternalScanner{}},
	}, extraRegisteredWithoutModeCases...) {
		t.Run(c.lang, func(t *testing.T) {
			checkRegisteredScanner(t, c.lang, c.scanner, false, nil)
		})
	}
}
