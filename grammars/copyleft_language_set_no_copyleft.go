//go:build gotreesitter_no_copyleft

package grammars

// copyleftGrammarNames lists the grammars whose upstream tree-sitter
// repository (confirmed in licenses/grammars.json) carries a copyleft
// license -- GPL-3.0 for caddy, disassembly, and jq; MPL-2.0 for nim. See
// docs/licensing.md.
//
// The gotreesitter_no_copyleft build tag removes these four grammars from
// the public registry: Register (registry.go) checks
// compileTimeLanguageEnabled before adding any entry, so none of them ever
// appear in AllLanguages, DetectLanguage, or DetectLanguageByName under this
// tag, regardless of which registration path (the built-in ts2go registry,
// grammar_subset, or grammargen) tries to add them.
//
// This tag also excludes the hand-ported external scanner Go source for
// caddy, disassembly, and nim (grammars/runtime/{caddy,disassembly,nim}_scanner.go)
// from compilation entirely; jq has no hand-ported scanner to exclude (its
// grammar blob carries no externals).
//
// What this tag does NOT do (see docs/licensing.md for the full accounting):
// it does not strip the embedded grammar blob bytes for these four grammars
// out of the compiled binary, and it does not stop a caller who imports the
// standalone grammars/caddy, grammars/disassembly, grammars/jq, or
// grammars/nim package directly (bypassing the registry) from decoding that
// blob into a Language -- one with no external scanner attached, so caddy,
// disassembly, and nim then parse with every external token unhandled.
// Removing the blob bytes themselves requires relocating grammar_blobs/*.bin
// into a separately embedded directory, which this build tag deliberately
// does not do (see docs/licensing.md's effort estimate for that option).
var copyleftGrammarNames = map[string]struct{}{
	"caddy":       {},
	"disassembly": {},
	"jq":          {},
	"nim":         {},
}

func copyleftLanguageAllowed(name string) bool {
	_, blocked := copyleftGrammarNames[name]
	return !blocked
}
