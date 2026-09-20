//go:build !grammar_subset || grammar_subset_julia

package grammarruntime

import gotreesitter "github.com/odvcencio/gotreesitter"

// External token indexes for the Julia grammar. This is the external index
// (the position of the token in the grammar's `externals: [...]` list),
// which is exactly what tree-sitter's `valid_symbols` array and C's
// result_symbol enum are indexed by. The external index is stable across a
// blob regen as long as the externals list itself does not reorder;
// concrete numeric gotreesitter.Symbol IDs are NOT stable (they shift
// whenever the grammar's total symbol count changes), so this scanner never
// hardcodes them -- see juliaDefaultSymTable below.
const (
	juliaTokBlockCommentRest      = 0
	juliaTokImmediateParen        = 1
	juliaTokImmediateBracket      = 2
	juliaTokImmediateBrace        = 3
	juliaTokImmediateStringStart  = 4
	juliaTokImmediateCommandStart = 5
	juliaTokContentCmd1           = 6
	juliaTokContentCmd1Raw        = 7
	juliaTokContentCmd3           = 8
	juliaTokContentCmd3Raw        = 9
	juliaTokContentStr1           = 10
	juliaTokContentStr1Raw        = 11
	juliaTokContentStr3           = 12
	juliaTokContentStr3Raw        = 13
	juliaTokEndCmd                = 14
	juliaTokEndStr                = 15
	juliaTokenCount               = 16
)

// juliaDefaultSymTable records the concrete gotreesitter.Symbol IDs the
// currently shipped julia.bin assigns to each external, in juliaTok* order.
// It exists only as a pre-bind fallback (and as an independent value to
// compare a real bind against in tests); ExternalScannerForLanguage below
// overwrites it with values read from the actual loaded Language at bind
// time, which is what the scanner must do to survive a future blob regen
// that renumbers absolute symbol IDs without touching the externals list
// order. The eight _content_* externals (indexes 6-13) alias to the same
// visible "content" node type.
var juliaDefaultSymTable = [juliaTokenCount]gotreesitter.Symbol{
	104, // _block_comment_rest
	105, // _immediate_paren
	106, // _immediate_bracket
	107, // _immediate_brace
	108, // _immediate_string_start
	109, // _immediate_command_start
	110, // _content_cmd_1 (display: content)
	111, // _content_cmd_1_raw (display: content)
	112, // _content_cmd_3 (display: content)
	113, // _content_cmd_3_raw (display: content)
	114, // _content_str_1 (display: content)
	115, // _content_str_1_raw (display: content)
	116, // _content_str_3 (display: content)
	117, // _content_str_3_raw (display: content)
	118, // _end_cmd
	119, // _end_str
}

// juliaExternalScannerSpec records the source contract for this
// hand-written port, so updater tooling can tell a grammar-only upstream
// change apart from one that also touches the external scanner or its token
// list. Its Externals list is also the binding source for
// ExternalScannerForLanguage: index i here is scanner token index i
// (juliaTok* order).
var juliaExternalScannerSpec = ExternalScannerSpec{
	Language:       "julia",
	UpstreamRepo:   "https://github.com/tree-sitter/tree-sitter-julia",
	UpstreamCommit: "e0f9dcd180fdcfcfa8d79a3531e11d99e79321d3",
	SourceFiles: []ExternalScannerSourceFile{
		{Path: "src/grammar.json", SHA256: "c04f5071a65141f3ee82289756c78ad8c2b22c7e6da1bcb62e246fb1cf3fa4d1"},
		{Path: "src/scanner.c", SHA256: "e6350a2f2725d966115d56142bf4e3e43dc13e3a3060744dd25f695363886e30"},
	},
	Externals: []string{
		"_block_comment_rest",
		"_immediate_paren",
		"_immediate_bracket",
		"_immediate_brace",
		"_immediate_string_start",
		"_immediate_command_start",
		"_content_cmd_1",
		"_content_cmd_1_raw",
		"_content_cmd_3",
		"_content_cmd_3_raw",
		"_content_str_1",
		"_content_str_1_raw",
		"_content_str_3",
		"_content_str_3_raw",
		"_end_cmd",
		"_end_str",
	},
}

func init() {
	RegisterExternalScannerSpec(juliaExternalScannerSpec)
}

// JuliaExternalScanner handles block comments, immediate tokens, and
// string/command content for Julia.
//
// symbols holds the concrete gotreesitter.Symbol each external index maps to
// in the Language this instance was bound to (see ExternalScannerForLanguage).
// The scanner never hardcodes an absolute Symbol value: a blob regen can
// renumber the grammar's absolute symbol IDs without touching the externals
// list order, and a scanner that still called SetResultSymbol with a stale
// hardcoded ID would silently emit the wrong (but still structurally valid)
// node type instead of failing loudly.
type JuliaExternalScanner struct {
	symbols         [juliaTokenCount]gotreesitter.Symbol
	externalToToken []int
}

// ExternalScannerForLanguage binds the scanner's token slots to the loaded
// Language's ExternalSymbols positionally. A hardcoded absolute
// gotreesitter.Symbol constant here would emit the wrong token whenever a
// grammar bump renumbers julia's external symbols.
func (JuliaExternalScanner) ExternalScannerForLanguage(lang *gotreesitter.Language) gotreesitter.ExternalScanner {
	s := JuliaExternalScanner{symbols: juliaDefaultSymTable}
	s.externalToToken = bindExternalScannerSpec(lang, juliaExternalScannerSpec, func(tokenIdx int, sym gotreesitter.Symbol) {
		s.symbols[tokenIdx] = sym
	})
	return s
}

func (s JuliaExternalScanner) symbolTable() *[juliaTokenCount]gotreesitter.Symbol {
	if s.symbols == ([juliaTokenCount]gotreesitter.Symbol{}) {
		return &juliaDefaultSymTable
	}
	return &s.symbols
}

func (JuliaExternalScanner) Create() any                           { return nil }
func (JuliaExternalScanner) Destroy(payload any)                   {}
func (JuliaExternalScanner) Serialize(payload any, buf []byte) int { return 0 }
func (JuliaExternalScanner) Deserialize(payload any, buf []byte)   {}

// The scanner carries no payload and derives every result from local
// lookahead plus validSymbols, so every incremental boundary is quiescent and
// failed scans cannot mutate persistent state.
func (JuliaExternalScanner) SupportsIncrementalReuse() bool   { return true }
func (JuliaExternalScanner) ExternalScannerIsStateless() bool { return true }

// These bytes follow the same branch in every scanner, including failed scans.
func (JuliaExternalScanner) ExternalScannerASCIIEquivalenceClass(b byte) uint8 {
	if b >= 'a' && b <= 'z' || b >= 'A' && b <= 'Z' || b >= '0' && b <= '9' || b == '_' {
		return 1
	}
	return 0
}
func (JuliaExternalScanner) PreservesStateOnScanFailure() bool { return true }

func (s JuliaExternalScanner) Scan(payload any, lexer *gotreesitter.ExternalLexer, validSymbols []bool) bool {
	if len(s.externalToToken) > 0 {
		var semanticValid [juliaTokenCount]bool
		for externalIdx, valid := range validSymbols {
			if !valid || externalIdx >= len(s.externalToToken) {
				continue
			}
			tokenIdx := s.externalToToken[externalIdx]
			if tokenIdx >= 0 && tokenIdx < juliaTokenCount {
				semanticValid[tokenIdx] = true
			}
		}
		validSymbols = semanticValid[:]
	}
	syms := s.symbolTable()

	// Immediate tokens: no whitespace consumed, just check current char.
	if juliaValid(validSymbols, juliaTokImmediateParen) && lexer.Lookahead() == '(' {
		lexer.SetResultSymbol(syms[juliaTokImmediateParen])
		return true
	}
	if juliaValid(validSymbols, juliaTokImmediateBracket) && lexer.Lookahead() == '[' {
		lexer.SetResultSymbol(syms[juliaTokImmediateBracket])
		return true
	}
	if juliaValid(validSymbols, juliaTokImmediateBrace) && lexer.Lookahead() == '{' {
		lexer.SetResultSymbol(syms[juliaTokImmediateBrace])
		return true
	}
	if juliaValid(validSymbols, juliaTokImmediateStringStart) && lexer.Lookahead() == '"' {
		lexer.SetResultSymbol(syms[juliaTokImmediateStringStart])
		return true
	}
	if juliaValid(validSymbols, juliaTokImmediateCommandStart) && lexer.Lookahead() == '`' {
		lexer.SetResultSymbol(syms[juliaTokImmediateCommandStart])
		return true
	}

	// Block comment: #= ... =#  (nested)
	if juliaValid(validSymbols, juliaTokBlockCommentRest) {
		if juliaScanBlockComment(lexer, syms[juliaTokBlockCommentRest]) {
			return true
		}
	}

	// String and command content scanning
	if juliaValid(validSymbols, juliaTokContentStr1) {
		return juliaScanContent(lexer, syms[juliaTokContentStr1], syms[juliaTokEndStr], '"', 1, true)
	}
	if juliaValid(validSymbols, juliaTokContentStr3) {
		return juliaScanContent(lexer, syms[juliaTokContentStr3], syms[juliaTokEndStr], '"', 3, true)
	}
	if juliaValid(validSymbols, juliaTokContentCmd1) {
		return juliaScanContent(lexer, syms[juliaTokContentCmd1], syms[juliaTokEndCmd], '`', 1, true)
	}
	if juliaValid(validSymbols, juliaTokContentCmd3) {
		return juliaScanContent(lexer, syms[juliaTokContentCmd3], syms[juliaTokEndCmd], '`', 3, true)
	}
	if juliaValid(validSymbols, juliaTokContentStr1Raw) {
		return juliaScanContent(lexer, syms[juliaTokContentStr1Raw], syms[juliaTokEndStr], '"', 1, false)
	}
	if juliaValid(validSymbols, juliaTokContentStr3Raw) {
		return juliaScanContent(lexer, syms[juliaTokContentStr3Raw], syms[juliaTokEndStr], '"', 3, false)
	}
	if juliaValid(validSymbols, juliaTokContentCmd1Raw) {
		return juliaScanContent(lexer, syms[juliaTokContentCmd1Raw], syms[juliaTokEndCmd], '`', 1, false)
	}
	if juliaValid(validSymbols, juliaTokContentCmd3Raw) {
		return juliaScanContent(lexer, syms[juliaTokContentCmd3Raw], syms[juliaTokEndCmd], '`', 3, false)
	}

	return false
}

func juliaScanContent(lexer *gotreesitter.ExternalLexer, contentSym, endSym gotreesitter.Symbol, endChar rune, nDelim uint32, interp bool) bool {
	hasContent := false

	for lexer.Lookahead() != 0 {
		lexer.MarkEnd()
		if interp && (lexer.Lookahead() == '$' || lexer.Lookahead() == '\\') {
			lexer.SetResultSymbol(contentSym)
			return hasContent
		} else if lexer.Lookahead() == '\\' {
			// Raw string: check escaped delimiters and '\\'
			lexer.Advance(false)
			if lexer.Lookahead() == endChar || lexer.Lookahead() == '\\' {
				lexer.SetResultSymbol(contentSym)
				return hasContent
			}
		} else {
			// Check for end delimiter sequence
			isEndDelimiter := true
			for i := uint32(1); i <= nDelim; i++ {
				if lexer.Lookahead() == endChar {
					lexer.Advance(false)
				} else {
					isEndDelimiter = false
					break
				}
			}
			if isEndDelimiter {
				if hasContent {
					lexer.SetResultSymbol(contentSym)
					return true
				}
				lexer.MarkEnd()
				lexer.SetResultSymbol(endSym)
				return true
			}
		}
		lexer.Advance(false)
		hasContent = true
	}
	return false
}

func juliaScanBlockComment(lexer *gotreesitter.ExternalLexer, blockCommentRestSym gotreesitter.Symbol) bool {
	// The first #= was already consumed by tree-sitter.
	afterEq := false
	nestingDepth := uint32(1)

	for {
		switch lexer.Lookahead() {
		case '=':
			lexer.Advance(false)
			afterEq = true
		case '#':
			lexer.Advance(false)
			if afterEq {
				afterEq = false
				nestingDepth--
				if nestingDepth == 0 {
					lexer.SetResultSymbol(blockCommentRestSym)
					return true
				}
			} else {
				afterEq = false
				if lexer.Lookahead() == '=' {
					lexer.Advance(false)
					nestingDepth++
				}
			}
		case 0:
			return false
		default:
			lexer.Advance(false)
			afterEq = false
		}
	}
}

func juliaValid(vs []bool, i int) bool { return i < len(vs) && vs[i] }
