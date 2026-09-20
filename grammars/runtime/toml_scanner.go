//go:build !grammar_subset || grammar_subset_toml

package grammarruntime

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
//
// External token indexes for the TOML grammar. This is the external index
// (the position of the token in the grammar's `externals: [...]` list),
// which is exactly what tree-sitter's `valid_symbols` array and C's
// result_symbol enum are indexed by. The external index is stable across a
// blob regen as long as the externals list itself does not reorder;
// concrete numeric gotreesitter.Symbol IDs are NOT stable (they shift
// whenever the grammar's total symbol count changes), so this scanner never
// hardcodes them -- see tomlDefaultSymTable below.
const (
	tomlTokLineEndingOrEOF            = 0
	tomlTokMultilineBasicStrContent   = 1
	tomlTokMultilineBasicStrEnd       = 2
	tomlTokMultilineLiteralStrContent = 3
	tomlTokMultilineLiteralStrEnd     = 4
	tomlTokenCount                    = 5
)

// tomlDefaultSymTable records the concrete gotreesitter.Symbol IDs the
// currently shipped toml.bin assigns to each external, in tomlTok* order. It
// exists only as a pre-bind fallback (and as an independent value to compare
// a real bind against in tests); ExternalScannerForLanguage below overwrites
// it with values read from the actual loaded Language at bind time, which is
// what the scanner must do to survive a future blob regen that renumbers
// absolute symbol IDs without touching the externals list order.
var tomlDefaultSymTable = [tomlTokenCount]gotreesitter.Symbol{
	35, // _line_ending_or_eof
	36, // _multiline_basic_string_content
	37, // _multiline_basic_string_end
	38, // _multiline_literal_string_content
	39, // _multiline_literal_string_end
}

// tomlExternalScannerSpec records the source contract for this hand-written
// port, so updater tooling can tell a grammar-only upstream change apart
// from one that also touches the external scanner or its token list. Its
// Externals list is also the binding source for ExternalScannerForLanguage:
// index i here is scanner token index i (tomlTok* order).
var tomlExternalScannerSpec = ExternalScannerSpec{
	Language:       "toml",
	UpstreamRepo:   "https://github.com/tree-sitter/tree-sitter-toml",
	UpstreamCommit: "342d9be207c2dba869b9967124c679b5e6fd0ebe",
	SourceFiles: []ExternalScannerSourceFile{
		{Path: "src/grammar.json", SHA256: "e88dd504146ca9726644f9bfe34d68d9a47b4d2ad54358670ca349dbf46958e1"},
		{Path: "src/scanner.c", SHA256: "59dcf6a51db3b53e4c33ac268221ce4e228c6855ca89d1e3f81786112120766f"},
	},
	Externals: []string{
		"_line_ending_or_eof",
		"_multiline_basic_string_content",
		"_multiline_basic_string_end",
		"_multiline_literal_string_content",
		"_multiline_literal_string_end",
	},
}

func init() {
	RegisterExternalScannerSpec(tomlExternalScannerSpec)
}

// TomlExternalScanner ports tree-sitter-toml's stateless external scanner.
//
// symbols holds the concrete gotreesitter.Symbol each external index maps to
// in the Language this instance was bound to (see ExternalScannerForLanguage).
// The scanner never hardcodes an absolute Symbol value: a blob regen can
// renumber the grammar's absolute symbol IDs without touching the externals
// list order, and a scanner that still called SetResultSymbol with a stale
// hardcoded ID would silently emit the wrong (but still structurally valid)
// node type instead of failing loudly.
type TomlExternalScanner struct {
	symbols         [tomlTokenCount]gotreesitter.Symbol
	externalToToken []int
}

// ExternalScannerForLanguage binds the scanner's token slots to the loaded
// Language's ExternalSymbols positionally. A hardcoded absolute
// gotreesitter.Symbol constant here would emit the wrong token whenever a
// grammar bump renumbers toml's external symbols.
func (TomlExternalScanner) ExternalScannerForLanguage(lang *gotreesitter.Language) gotreesitter.ExternalScanner {
	s := TomlExternalScanner{symbols: tomlDefaultSymTable}
	s.externalToToken = bindExternalScannerSpec(lang, tomlExternalScannerSpec, func(tokenIdx int, sym gotreesitter.Symbol) {
		s.symbols[tokenIdx] = sym
	})
	return s
}

func (TomlExternalScanner) Create() any                      { return nil }
func (TomlExternalScanner) Destroy(payload any)              {}
func (TomlExternalScanner) SupportsIncrementalReuse() bool   { return true }
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

func (s TomlExternalScanner) Scan(payload any, lexer *gotreesitter.ExternalLexer, validSymbols []bool) bool {
	if len(s.externalToToken) > 0 {
		var semanticValid [tomlTokenCount]bool
		for externalIdx, valid := range validSymbols {
			if !valid || externalIdx >= len(s.externalToToken) {
				continue
			}
			tokenIdx := s.externalToToken[externalIdx]
			if tokenIdx >= 0 && tokenIdx < tomlTokenCount {
				semanticValid[tokenIdx] = true
			}
		}
		validSymbols = semanticValid[:]
	}
	syms := s.symbolTable()

	if tomlScanMultilineStringEnd(lexer, validSymbols, '"',
		tomlTokMultilineBasicStrEnd, syms[tomlTokMultilineBasicStrContent], syms[tomlTokMultilineBasicStrEnd]) ||
		tomlScanMultilineStringEnd(lexer, validSymbols, '\'',
			tomlTokMultilineLiteralStrEnd, syms[tomlTokMultilineLiteralStrContent], syms[tomlTokMultilineLiteralStrEnd]) {
		return true
	}

	if tomlTokLineEndingOrEOF < len(validSymbols) && validSymbols[tomlTokLineEndingOrEOF] {
		lexer.SetResultSymbol(syms[tomlTokLineEndingOrEOF])

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

func (s TomlExternalScanner) symbolTable() *[tomlTokenCount]gotreesitter.Symbol {
	if s.symbols == ([tomlTokenCount]gotreesitter.Symbol{}) {
		return &tomlDefaultSymTable
	}
	return &s.symbols
}
