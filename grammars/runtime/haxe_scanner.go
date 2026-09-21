//go:build !grammar_subset || grammar_subset_haxe

package grammarruntime

import gotreesitter "github.com/odvcencio/gotreesitter"

// External token indexes for the haxe grammar. This is the external
// index (the position of the token in the grammar's `externals: [...]`
// list), which is exactly what tree-sitter's `valid_symbols` array and
// C's result_symbol enum are indexed by. The external index is stable
// across a blob regen as long as the externals list itself does not
// reorder; concrete numeric gotreesitter.Symbol IDs are NOT stable (they
// shift whenever the grammar's total symbol count changes), so this
// scanner never hardcodes them -- see haxeDefaultSymTable below.
const (
	haxeTokLookbackSemicolon    = 0
	haxeTokClosingBraceMarker   = 1
	haxeTokClosingBraceUnmarker = 2
	haxeTokenCount              = 3
)

// haxeDefaultSymTable records the concrete gotreesitter.Symbol IDs the
// currently shipped haxe.bin assigns to each external, in haxeTok*
// order. It exists only as a pre-bind fallback (and as an independent
// value to compare a real bind against in tests); ExternalScannerForLanguage
// below overwrites it with values read from the actual loaded Language at
// bind time, which is what the scanner must do to survive a future blob
// regen that renumbers absolute symbol IDs without touching the externals
// list order.
var haxeDefaultSymTable = [haxeTokenCount]gotreesitter.Symbol{
	116, // _lookback_semicolon
	117, // _closing_brace_marker
	118, // _closing_brace_unmarker
}

// haxeExternalScannerSpec records the source contract for this
// hand-written port, so updater tooling can tell a grammar-only upstream
// change apart from one that also touches the external scanner or its
// token list. Its Externals list is also the binding source for
// ExternalScannerForLanguage: index i here is scanner token index i
// (haxeTok* order).
var haxeExternalScannerSpec = ExternalScannerSpec{
	Language:       "haxe",
	UpstreamRepo:   "https://github.com/vantreeseba/tree-sitter-haxe",
	UpstreamCommit: "f2a2394d9ca7a6099f78d8b0d178530e7c9a8e26",
	SourceFiles: []ExternalScannerSourceFile{
		{Path: "src/grammar.json", SHA256: "05ddc6f044288d47b8df60c2dc3aa3868c799b6f55e1c0609107643c830e50b7"},
		{Path: "src/scanner.c", SHA256: "2eee9efa0cfbcd6cf8c69e638a059ddf3f1abf47540a1e9460f1b9c805374277"},
	},
	Externals: []string{
		"_lookback_semicolon",
		"_closing_brace_marker",
		"_closing_brace_unmarker",
	},
}

func init() {
	RegisterExternalScannerSpec(haxeExternalScannerSpec)
}

// haxeState tracks whether a closing brace was just seen.
type haxeState struct {
	justSawBrace bool
}

// HaxeExternalScanner handles lookback semicolons and closing brace
// detection for Haxe automatic semicolon insertion.
//
// symbols holds the concrete gotreesitter.Symbol each external index maps to
// in the Language this instance was bound to (see ExternalScannerForLanguage).
// The scanner never hardcodes an absolute Symbol value: a blob regen can
// renumber the grammar's absolute symbol IDs without touching the externals
// list order, and a scanner that still called SetResultSymbol with a stale
// hardcoded ID would silently emit the wrong (but still structurally valid)
// node type instead of failing loudly.
type HaxeExternalScanner struct {
	symbols         [haxeTokenCount]gotreesitter.Symbol
	externalToToken []int
}

// ExternalScannerForLanguage binds the scanner's token slots to the loaded
// Language's ExternalSymbols positionally. A hardcoded absolute
// gotreesitter.Symbol constant here would emit the wrong token whenever a
// grammar bump renumbers haxe's external symbols.
func (HaxeExternalScanner) ExternalScannerForLanguage(lang *gotreesitter.Language) gotreesitter.ExternalScanner {
	s := HaxeExternalScanner{symbols: haxeDefaultSymTable}
	s.externalToToken = bindExternalScannerSpec(lang, haxeExternalScannerSpec, func(tokenIdx int, sym gotreesitter.Symbol) {
		s.symbols[tokenIdx] = sym
	})
	return s
}

func (s HaxeExternalScanner) symbolTable() *[haxeTokenCount]gotreesitter.Symbol {
	if s.symbols == ([haxeTokenCount]gotreesitter.Symbol{}) {
		return &haxeDefaultSymTable
	}
	return &s.symbols
}

func (HaxeExternalScanner) Create() any         { return &haxeState{} }
func (HaxeExternalScanner) Destroy(payload any) {}
func (HaxeExternalScanner) Serialize(payload any, buf []byte) int {
	s := payload.(*haxeState)
	if s.justSawBrace {
		buf[0] = 1
	} else {
		buf[0] = 0
	}
	return 1
}
func (HaxeExternalScanner) Deserialize(payload any, buf []byte) {
	s := payload.(*haxeState)
	if len(buf) > 0 {
		s.justSawBrace = buf[0] != 0
	} else {
		s.justSawBrace = false
	}
}

func (sc HaxeExternalScanner) Scan(payload any, lexer *gotreesitter.ExternalLexer, validSymbols []bool) bool {
	s := payload.(*haxeState)

	if len(sc.externalToToken) > 0 {
		var semanticValid [haxeTokenCount]bool
		for externalIdx, valid := range validSymbols {
			if !valid || externalIdx >= len(sc.externalToToken) {
				continue
			}
			tokenIdx := sc.externalToToken[externalIdx]
			if tokenIdx >= 0 && tokenIdx < haxeTokenCount {
				semanticValid[tokenIdx] = true
			}
		}
		validSymbols = semanticValid[:]
	}
	syms := sc.symbolTable()

	if haxeValid(validSymbols, haxeTokLookbackSemicolon) {
		if lexer.Lookahead() == ';' {
			s.justSawBrace = false
			lexer.SetResultSymbol(syms[haxeTokLookbackSemicolon])
			lexer.Advance(false)
			return true
		}
		if s.justSawBrace {
			s.justSawBrace = false
			lexer.SetResultSymbol(syms[haxeTokLookbackSemicolon])
			return true
		}
		return false
	}

	if haxeValid(validSymbols, haxeTokClosingBraceMarker) {
		lexer.MarkEnd()
		for isHaxeWhitespace(lexer.Lookahead()) {
			lexer.Advance(true)
		}
		if lexer.Lookahead() == '}' {
			s.justSawBrace = true
			lexer.SetResultSymbol(syms[haxeTokClosingBraceMarker])
			return true
		}
	}

	if haxeValid(validSymbols, haxeTokClosingBraceUnmarker) &&
		s.justSawBrace &&
		lexer.Lookahead() != '}' &&
		!isHaxeWhitespace(lexer.Lookahead()) {
		s.justSawBrace = false
		lexer.SetResultSymbol(syms[haxeTokClosingBraceUnmarker])
		return true
	}

	return false
}

func isHaxeWhitespace(ch rune) bool {
	return ch == ' ' || ch == '\n' || ch == '\t' || ch == '\r'
}

func haxeValid(vs []bool, i int) bool { return i < len(vs) && vs[i] }
