//go:build !grammar_subset || grammar_subset_dtd

package grammarruntime

import (
	"unicode"

	gotreesitter "github.com/odvcencio/gotreesitter"
)

// External token indexes for the dtd grammar. This is the external index
// (the position of the token in the grammar's `externals: [...]` list),
// which is exactly what tree-sitter's `valid_symbols` array and C's
// result_symbol enum are indexed by. The external index is stable across a
// blob regen as long as the externals list itself does not reorder;
// concrete numeric gotreesitter.Symbol IDs are NOT stable (they shift
// whenever the grammar's total symbol count changes), so this scanner never
// hardcodes them -- see dtdDefaultSymTable below.
const (
	dtdTokPITarget  = 0
	dtdTokPIContent = 1
	dtdTokComment   = 2
	dtdTokenCount   = 3
)

// dtdDefaultSymTable records the concrete gotreesitter.Symbol IDs the
// currently shipped dtd.bin assigns to each external, in dtdTok* order. It
// exists only as a pre-bind fallback (and as an independent value to
// compare a real bind against in tests); ExternalScannerForLanguage below
// overwrites it with values read from the actual loaded Language at bind
// time, which is what the scanner must do to survive a future blob regen
// that renumbers absolute symbol IDs without touching the externals list
// order.
var dtdDefaultSymTable = [dtdTokenCount]gotreesitter.Symbol{
	58, // PITarget
	59, // _pi_content
	60, // Comment
}

// dtdExternalScannerSpec records the source contract for this
// hand-written port, so updater tooling can tell a grammar-only upstream
// change apart from one that also touches the external scanner or its
// token list. Its Externals list is also the binding source for
// ExternalScannerForLanguage: index i here is scanner token index i
// (dtdTok* order). dtd shares its upstream repository with xml
// (tree-sitter-grammars/tree-sitter-xml), but as the dtd/src subdirectory
// grammar rather than xml/src.
var dtdExternalScannerSpec = ExternalScannerSpec{
	Language:       "dtd",
	UpstreamRepo:   "https://github.com/tree-sitter-grammars/tree-sitter-xml",
	UpstreamCommit: "5000ae8f22d11fbe93939b05c1e37cf21117162d",
	SourceFiles: []ExternalScannerSourceFile{
		{Path: "dtd/src/grammar.json", SHA256: "e3583be3ffd765964adfa91510b84e2749b1331792413f4aa7f5059eaa452bb0"},
		{Path: "dtd/src/scanner.c", SHA256: "cdb323ea613368705ade3c6904523848f262db38ee01f5722e2f16e0751bd71c"},
	},
	Externals: []string{
		"PITarget",
		"_pi_content",
		"Comment",
	},
}

func init() {
	RegisterExternalScannerSpec(dtdExternalScannerSpec)
}

// DtdExternalScanner handles processing instructions and <!-- --> comments for DTD.
//
// symbols holds the concrete gotreesitter.Symbol each external index maps to
// in the Language this instance was bound to (see ExternalScannerForLanguage).
// The scanner never hardcodes an absolute Symbol value: a blob regen can
// renumber the grammar's absolute symbol IDs without touching the externals
// list order, and a scanner that still called SetResultSymbol with a stale
// hardcoded ID would silently emit the wrong (but still structurally valid)
// node type instead of failing loudly.
type DtdExternalScanner struct {
	symbols         [dtdTokenCount]gotreesitter.Symbol
	externalToToken []int
}

// ExternalScannerForLanguage binds the scanner's token slots to the loaded
// Language's ExternalSymbols positionally. A hardcoded absolute
// gotreesitter.Symbol constant here would emit the wrong token whenever a
// grammar bump renumbers dtd's external symbols.
func (DtdExternalScanner) ExternalScannerForLanguage(lang *gotreesitter.Language) gotreesitter.ExternalScanner {
	s := DtdExternalScanner{symbols: dtdDefaultSymTable}
	s.externalToToken = bindExternalScannerSpec(lang, dtdExternalScannerSpec, func(tokenIdx int, sym gotreesitter.Symbol) {
		s.symbols[tokenIdx] = sym
	})
	return s
}

func (s DtdExternalScanner) symbolTable() *[dtdTokenCount]gotreesitter.Symbol {
	if s.symbols == ([dtdTokenCount]gotreesitter.Symbol{}) {
		return &dtdDefaultSymTable
	}
	return &s.symbols
}

func (DtdExternalScanner) Create() any                           { return nil }
func (DtdExternalScanner) Destroy(payload any)                   {}
func (DtdExternalScanner) Serialize(payload any, buf []byte) int { return 0 }
func (DtdExternalScanner) Deserialize(payload any, buf []byte)   {}
func (DtdExternalScanner) SupportsIncrementalReuse() bool        { return true }
func (DtdExternalScanner) ExternalScannerIsStateless() bool      { return true }

func (s DtdExternalScanner) Scan(payload any, lexer *gotreesitter.ExternalLexer, validSymbols []bool) bool {
	if len(s.externalToToken) > 0 {
		var semanticValid [dtdTokenCount]bool
		for externalIdx, valid := range validSymbols {
			if !valid || externalIdx >= len(s.externalToToken) {
				continue
			}
			tokenIdx := s.externalToToken[externalIdx]
			if tokenIdx >= 0 && tokenIdx < dtdTokenCount {
				semanticValid[tokenIdx] = true
			}
		}
		validSymbols = semanticValid[:]
	}
	syms := s.symbolTable()

	// Error recovery
	if dtdValid(validSymbols, dtdTokPITarget) &&
		dtdValid(validSymbols, dtdTokPIContent) &&
		dtdValid(validSymbols, dtdTokComment) {
		return false
	}

	if dtdValid(validSymbols, dtdTokPITarget) {
		return dtdScanPITarget(lexer, syms[dtdTokPITarget])
	}

	if dtdValid(validSymbols, dtdTokPIContent) {
		return dtdScanPIContent(lexer, syms[dtdTokPIContent])
	}

	if dtdValid(validSymbols, dtdTokComment) {
		return dtdScanComment(lexer, syms[dtdTokComment])
	}

	return false
}

func dtdIsValidNameStartChar(ch rune) bool {
	return unicode.IsLetter(ch) || ch == '_' || ch == ':'
}

func dtdIsValidNameChar(ch rune) bool {
	return unicode.IsLetter(ch) || unicode.IsDigit(ch) ||
		ch == '_' || ch == ':' || ch == '.' || ch == '-' || ch == 0xB7
}

func dtdScanPITarget(lexer *gotreesitter.ExternalLexer, piTargetSym gotreesitter.Symbol) bool {
	if !dtdIsValidNameStartChar(lexer.Lookahead()) {
		return false
	}
	foundXFirst := (lexer.Lookahead() == 'x' || lexer.Lookahead() == 'X')
	if foundXFirst {
		lexer.MarkEnd()
	}
	lexer.Advance(false)

	for dtdIsValidNameChar(lexer.Lookahead()) {
		if foundXFirst && (lexer.Lookahead() == 'm' || lexer.Lookahead() == 'M') {
			lexer.Advance(false)
			if lexer.Lookahead() == 'l' || lexer.Lookahead() == 'L' {
				lexer.Advance(false)
				if dtdIsValidNameChar(lexer.Lookahead()) {
					// Not "xml" exactly, continue
					foundXFirst = false
					lexer.Advance(false)
					continue
				}
				// This is exactly "xml" — not a valid PI target
				return false
			}
		}
		foundXFirst = false
		lexer.Advance(false)
	}

	lexer.MarkEnd()
	lexer.SetResultSymbol(piTargetSym)
	return true
}

func dtdScanPIContent(lexer *gotreesitter.ExternalLexer, piContentSym gotreesitter.Symbol) bool {
	for lexer.Lookahead() != 0 && lexer.Lookahead() != '\n' && lexer.Lookahead() != '?' {
		lexer.Advance(false)
	}
	if lexer.Lookahead() != '?' {
		return false
	}
	lexer.MarkEnd()
	lexer.Advance(false)
	if lexer.Lookahead() == '>' {
		lexer.Advance(false)
		for lexer.Lookahead() == ' ' {
			lexer.Advance(false)
		}
		if lexer.Lookahead() == '\n' {
			lexer.Advance(false)
		} else if lexer.Lookahead() == 0 {
			return false
		} else {
			return false
		}
		lexer.SetResultSymbol(piContentSym)
		return true
	}
	return false
}

func dtdScanComment(lexer *gotreesitter.ExternalLexer, commentSym gotreesitter.Symbol) bool {
	// Expect <!-- (< and ! already consumed by grammar)
	if lexer.Lookahead() != '<' {
		return false
	}
	lexer.Advance(false)
	if lexer.Lookahead() != '!' {
		return false
	}
	lexer.Advance(false)
	if lexer.Lookahead() != '-' {
		return false
	}
	lexer.Advance(false)
	if lexer.Lookahead() != '-' {
		return false
	}
	lexer.Advance(false)

	for lexer.Lookahead() != 0 {
		if lexer.Lookahead() == '-' {
			lexer.Advance(false)
			if lexer.Lookahead() == '-' {
				lexer.Advance(false)
				break
			}
		} else {
			lexer.Advance(false)
		}
	}
	if lexer.Lookahead() == '>' {
		lexer.Advance(false)
		lexer.MarkEnd()
		lexer.SetResultSymbol(commentSym)
		return true
	}
	return false
}

func dtdValid(vs []bool, i int) bool { return i < len(vs) && vs[i] }
