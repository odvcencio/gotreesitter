//go:build !grammar_subset || grammar_subset_nushell

package grammarruntime

import (
	"unicode"

	gotreesitter "github.com/odvcencio/gotreesitter"
)

// External token indexes for the nushell grammar. This is the external
// index (the position of the token in the grammar's `externals: [...]`
// list), which is exactly what tree-sitter's `valid_symbols` array and
// C's result_symbol enum are indexed by. The external index is stable
// across a blob regen as long as the externals list itself does not
// reorder; concrete numeric gotreesitter.Symbol IDs are NOT stable (they
// shift whenever the grammar's total symbol count changes), so this
// scanner never hardcodes them -- see nushellDefaultSymTable below.
const (
	nushellTokRawStringBegin   = 0
	nushellTokRawStringContent = 1
	nushellTokRawStringEnd     = 2
	nushellTokenCount          = 3
)

// nushellDefaultSymTable records the concrete gotreesitter.Symbol IDs the
// currently shipped nushell.bin assigns to each external, in nushellTok*
// order. It exists only as a pre-bind fallback (and as an independent
// value to compare a real bind against in tests); ExternalScannerForLanguage
// below overwrites it with values read from the actual loaded Language at
// bind time, which is what the scanner must do to survive a future blob
// regen that renumbers absolute symbol IDs without touching the externals
// list order.
var nushellDefaultSymTable = [nushellTokenCount]gotreesitter.Symbol{
	269, // raw_string_begin
	270, // raw_string_content, displays as "string_content"
	271, // raw_string_end
}

// nushellExternalScannerSpec records the source contract for this
// hand-written port, so updater tooling can tell a grammar-only upstream
// change apart from one that also touches the external scanner or its
// token list. Its Externals list is also the binding source for
// ExternalScannerForLanguage: index i here is scanner token index i
// (nushellTok* order).
var nushellExternalScannerSpec = ExternalScannerSpec{
	Language:       "nushell",
	UpstreamRepo:   "https://github.com/nushell/tree-sitter-nu",
	UpstreamCommit: "bb3f533e5792260291945e1f329e1f0a779def6e",
	SourceFiles: []ExternalScannerSourceFile{
		{Path: "src/grammar.json", SHA256: "403c490bb985fc6b0b771d5c033a3b2ed89e64e88b39887cd1485c51000531cd"},
		{Path: "src/scanner.c", SHA256: "a010ea852dbea82325d84d9d0b29e815c1226df1732badaea30c007cd3742da8"},
	},
	Externals: []string{
		"raw_string_begin",
		"raw_string_content",
		"raw_string_end",
	},
}

func init() {
	RegisterExternalScannerSpec(nushellExternalScannerSpec)
}

// nushellScannerState holds the raw string hash level.
type nushellScannerState struct {
	level uint8
}

// NushellExternalScanner implements gotreesitter.ExternalScanner for tree-sitter-nu.
//
// This is a Go port of the C external scanner from tree-sitter-nu
// (https://github.com/nushell/tree-sitter-nu). The scanner handles:
//   - raw_string_begin: r#'  (with variable # count)
//   - string_content: content between raw string delimiters
//   - raw_string_end: '#  closing delimiter
//
// symbols holds the concrete gotreesitter.Symbol each external index maps to
// in the Language this instance was bound to (see ExternalScannerForLanguage).
// The scanner never hardcodes an absolute Symbol value: a blob regen can
// renumber the grammar's absolute symbol IDs without touching the externals
// list order, and a scanner that still called SetResultSymbol with a stale
// hardcoded ID would silently emit the wrong (but still structurally valid)
// node type instead of failing loudly.
type NushellExternalScanner struct {
	symbols         [nushellTokenCount]gotreesitter.Symbol
	externalToToken []int
}

// ExternalScannerForLanguage binds the scanner's token slots to the loaded
// Language's ExternalSymbols positionally. A hardcoded absolute
// gotreesitter.Symbol constant here would emit the wrong token whenever a
// grammar bump renumbers nushell's external symbols.
func (NushellExternalScanner) ExternalScannerForLanguage(lang *gotreesitter.Language) gotreesitter.ExternalScanner {
	s := NushellExternalScanner{symbols: nushellDefaultSymTable}
	s.externalToToken = bindExternalScannerSpec(lang, nushellExternalScannerSpec, func(tokenIdx int, sym gotreesitter.Symbol) {
		s.symbols[tokenIdx] = sym
	})
	return s
}

func (s NushellExternalScanner) symbolTable() *[nushellTokenCount]gotreesitter.Symbol {
	if s.symbols == ([nushellTokenCount]gotreesitter.Symbol{}) {
		return &nushellDefaultSymTable
	}
	return &s.symbols
}

func (NushellExternalScanner) Create() any {
	return &nushellScannerState{level: 0}
}

func (NushellExternalScanner) Destroy(payload any) {}

func (NushellExternalScanner) Serialize(payload any, buf []byte) int {
	s := payload.(*nushellScannerState)
	if len(buf) >= 1 {
		buf[0] = s.level
		return 1
	}
	return 0
}

func (NushellExternalScanner) Deserialize(payload any, buf []byte) {
	s := payload.(*nushellScannerState)
	s.level = 0
	if len(buf) == 1 {
		s.level = buf[0]
	}
}

func (sc NushellExternalScanner) Scan(payload any, lexer *gotreesitter.ExternalLexer, validSymbols []bool) bool {
	syms := sc.symbolTable()

	s := payload.(*nushellScannerState)

	// RAW_STRING_BEGIN: r#+'
	if nushellValid(validSymbols, nushellTokRawStringBegin) && s.level == 0 {
		// Skip whitespace
		for unicode.IsSpace(lexer.Lookahead()) && lexer.Lookahead() != 0 {
			lexer.Advance(true)
		}

		if lexer.Lookahead() != 'r' {
			return false
		}
		lexer.Advance(false)

		var level uint8
		for lexer.Lookahead() == '#' && lexer.Lookahead() != 0 {
			lexer.Advance(false)
			level++
		}

		if lexer.Lookahead() == '\'' {
			lexer.Advance(false)
			s.level = level
			lexer.MarkEnd()
			lexer.SetResultSymbol(syms[nushellTokRawStringBegin])
			return true
		}
		return false
	}

	// RAW_STRING_CONTENT: everything up to the closing '#+
	if nushellValid(validSymbols, nushellTokRawStringContent) && s.level != 0 {
		for lexer.Lookahead() != 0 {
			lexer.MarkEnd()
			lexer.Advance(false)
			// Count consecutive '#' after current char
			var hashCount uint8
			for lexer.Lookahead() == '#' && lexer.Lookahead() != 0 {
				lexer.Advance(false)
				hashCount++
			}
			if hashCount == s.level {
				lexer.SetResultSymbol(syms[nushellTokRawStringContent])
				return true
			}
		}
		return false
	}

	// RAW_STRING_END: '#+  (closing delimiter)
	if nushellValid(validSymbols, nushellTokRawStringEnd) && s.level != 0 && lexer.Lookahead() == '\'' {
		lexer.Advance(false) // consume '
		remaining := s.level
		for remaining > 0 {
			lexer.Advance(false) // consume #
			remaining--
		}
		s.level = 0
		lexer.MarkEnd()
		lexer.SetResultSymbol(syms[nushellTokRawStringEnd])
		return true
	}

	return false
}

func nushellValid(validSymbols []bool, idx int) bool {
	return idx >= 0 && idx < len(validSymbols) && validSymbols[idx]
}
