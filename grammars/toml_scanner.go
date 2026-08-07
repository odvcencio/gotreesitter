//go:build !grammar_subset || grammar_subset_toml

package grammars

import (
	gotreesitter "github.com/odvcencio/gotreesitter"
)

// TomlExternalScanner is a faithful port of tree-sitter-toml's src/scanner.c
// (pinned commit 342d9be207c2dba869b9967124c679b5e6fd0ebe).
//
// The C scanner is stateless. It handles two things:
//
//  1. Disambiguating quote runs inside multiline strings: a lone delimiter (or
//     a pair) inside `”'…”'` / `"""…"""` is string content, exactly three
//     delimiters end the string, and four-plus emit one content delimiter so
//     the closing triple still terminates the string.
//  2. The zero-width `_line_ending_or_eof` token emitted before a newline,
//     CRLF, or EOF (after skipping spaces/tabs).
const (
	tomlTokLineEndingOrEOF            = 0
	tomlTokMultilineBasicStrContent   = 1
	tomlTokMultilineBasicStrEnd       = 2
	tomlTokMultilineLiteralStrContent = 3
	tomlTokMultilineLiteralStrEnd     = 4
)

// Language symbol ids for the external tokens (see toml grammar symbol table;
// asserted against symbol names in toml_scanner_test.go).
const (
	tomlSymLineEndingOrEOF            gotreesitter.Symbol = 35
	tomlSymMultilineBasicStrContent   gotreesitter.Symbol = 36
	tomlSymMultilineBasicStrEnd       gotreesitter.Symbol = 37
	tomlSymMultilineLiteralStrContent gotreesitter.Symbol = 38
	tomlSymMultilineLiteralStrEnd     gotreesitter.Symbol = 39
)

// TomlExternalScanner ports tree-sitter-toml's stateless external scanner.
type TomlExternalScanner struct{}

func (TomlExternalScanner) Create() any                    { return nil }
func (TomlExternalScanner) Destroy(payload any)            {}
func (TomlExternalScanner) SupportsIncrementalReuse() bool { return true }

// ExternalScannerIsStateless proves that TOML scanning depends only on
// lookahead and the parser-provided valid-symbol set.
func (TomlExternalScanner) ExternalScannerIsStateless() bool { return true }

func (TomlExternalScanner) Serialize(payload any, buf []byte) int { return 0 }
func (TomlExternalScanner) Deserialize(payload any, buf []byte)   {}

// tomlScanMultilineStringEnd mirrors
// tree_sitter_toml_external_scanner_scan_multiline_string_end in scanner.c.
func tomlScanMultilineStringEnd(lexer *gotreesitter.ExternalLexer, validSymbols []bool, delimiter rune, endTok int, contentSym, endSym gotreesitter.Symbol) bool {
	if endTok >= len(validSymbols) || !validSymbols[endTok] || lexer.Lookahead() != delimiter {
		return false
	}

	lexer.Advance(false)
	lexer.MarkEnd()

	if lexer.Lookahead() != delimiter {
		lexer.SetResultSymbol(contentSym)
		return true
	}

	lexer.Advance(false)

	if lexer.Lookahead() != delimiter {
		lexer.MarkEnd()
		lexer.SetResultSymbol(contentSym)
		return true
	}

	lexer.Advance(false)

	if lexer.Lookahead() != delimiter {
		lexer.MarkEnd()
		lexer.SetResultSymbol(endSym)
		return true
	}

	lexer.SetResultSymbol(contentSym)
	return true
}

func (TomlExternalScanner) Scan(payload any, lexer *gotreesitter.ExternalLexer, validSymbols []bool) bool {
	if tomlScanMultilineStringEnd(lexer, validSymbols, '"',
		tomlTokMultilineBasicStrEnd, tomlSymMultilineBasicStrContent, tomlSymMultilineBasicStrEnd) ||
		tomlScanMultilineStringEnd(lexer, validSymbols, '\'',
			tomlTokMultilineLiteralStrEnd, tomlSymMultilineLiteralStrContent, tomlSymMultilineLiteralStrEnd) {
		return true
	}

	if tomlTokLineEndingOrEOF < len(validSymbols) && validSymbols[tomlTokLineEndingOrEOF] {
		lexer.SetResultSymbol(tomlSymLineEndingOrEOF)

		for lexer.Lookahead() == ' ' || lexer.Lookahead() == '\t' {
			lexer.Advance(true)
		}

		if lexer.Lookahead() == 0 || lexer.Lookahead() == '\n' {
			return true
		}

		if lexer.Lookahead() == '\r' {
			lexer.Advance(true)
			if lexer.Lookahead() == '\n' {
				return true
			}
		}
	}

	return false
}
