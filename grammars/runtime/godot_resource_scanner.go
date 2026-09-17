//go:build !grammar_subset || grammar_subset_godot_resource

package grammarruntime

import (
	"unicode"

	gotreesitter "github.com/odvcencio/gotreesitter"
)

// External token indexes for the godot_resource grammar.
const (
	godotResourceTokString = 0
)

const (
	godotResourceSymString gotreesitter.Symbol = 19
)

// GodotResourceExternalScanner handles multiline string literals in
// Godot .tres/.tscn resource files. Strings are "..." with \" escapes.
type GodotResourceExternalScanner struct{}

func (GodotResourceExternalScanner) Create() any                           { return nil }
func (GodotResourceExternalScanner) Destroy(payload any)                   {}
func (GodotResourceExternalScanner) Serialize(payload any, buf []byte) int { return 0 }
func (GodotResourceExternalScanner) Deserialize(payload any, buf []byte)   {}
func (GodotResourceExternalScanner) SupportsIncrementalReuse() bool        { return true }
func (GodotResourceExternalScanner) ExternalScannerIsStateless() bool      { return true }

// Scan is a line-faithful port of the pinned upstream src/scanner.c
// (PrestonKnopp/tree-sitter-godot-resource @ 302c1895).
//
// Note the upstream escape handling: a quote terminates the string only when
// the PREVIOUS character was not a backslash. That means `\\"` does NOT
// terminate (the quote follows a backslash) — upstream does not treat `\\`
// as a completed escape pair. Parity requires reproducing that exactly.
func (GodotResourceExternalScanner) Scan(payload any, lexer *gotreesitter.ExternalLexer, validSymbols []bool) bool {
	if !godotResourceValid(validSymbols, godotResourceTokString) {
		return false
	}

	for unicode.IsSpace(lexer.Lookahead()) {
		lexer.Advance(true)
	}

	if lexer.Lookahead() != '"' {
		return false
	}

	lastChar := rune('"')
	lexer.Advance(false)

	for lexer.Lookahead() != 0 {
		if lastChar != '\\' && lexer.Lookahead() == '"' {
			lexer.Advance(false)
			lexer.SetResultSymbol(godotResourceSymString)
			return true
		}
		lastChar = lexer.Lookahead()
		lexer.Advance(false)
	}

	return false
}

func godotResourceValid(vs []bool, i int) bool { return i < len(vs) && vs[i] }
