//go:build !grammar_subset || grammar_subset_bicep

package grammarruntime

import (
	"unicode"

	gotreesitter "github.com/odvcencio/gotreesitter"
)

// External token indexes for the bicep grammar. This is the external
// index (the position of the token in the grammar's `externals: [...]`
// list), which is exactly what tree-sitter's `valid_symbols` array and
// C's result_symbol enum are indexed by. The external index is stable
// across a blob regen as long as the externals list itself does not
// reorder; concrete numeric gotreesitter.Symbol IDs are NOT stable (they
// shift whenever the grammar's total symbol count changes), so this
// scanner never hardcodes them -- see bicepDefaultSymTable below.
const (
	bicepTokExternalAsterisk       = 0
	bicepTokMultilineStringContent = 1
	bicepTokenCount                = 2
)

// bicepDefaultSymTable records the concrete gotreesitter.Symbol IDs the
// currently shipped bicep.bin assigns to each external, in bicepTok*
// order. It exists only as a pre-bind fallback (and as an independent
// value to compare a real bind against in tests); ExternalScannerForLanguage
// below overwrites it with values read from the actual loaded Language at
// bind time, which is what the scanner must do to survive a future blob
// regen that renumbers absolute symbol IDs without touching the externals
// list order.
var bicepDefaultSymTable = [bicepTokenCount]gotreesitter.Symbol{
	77, // _external_asterisk
	78, // _multiline_string_content
}

// bicepExternalScannerSpec records the source contract for this
// hand-written port, so updater tooling can tell a grammar-only upstream
// change apart from one that also touches the external scanner or its
// token list. Its Externals list is also the binding source for
// ExternalScannerForLanguage: index i here is scanner token index i
// (bicepTok* order).
var bicepExternalScannerSpec = ExternalScannerSpec{
	Language:       "bicep",
	UpstreamRepo:   "https://github.com/amaanq/tree-sitter-bicep",
	UpstreamCommit: "bff59884307c0ab009bd5e81afd9324b46a6c0f9",
	SourceFiles: []ExternalScannerSourceFile{
		{Path: "src/grammar.json", SHA256: "3368c494203589f6c511a21590bb9aeb4ac2935eff12252daeca1650464ddf94"},
		{Path: "src/scanner.c", SHA256: "20eaded85dfb7b17a6fc6f2f5c690a3fecfbd47b2214d43008c9e2ba502e0d7f"},
	},
	Externals: []string{
		"_external_asterisk",
		"_multiline_string_content",
	},
}

func init() {
	RegisterExternalScannerSpec(bicepExternalScannerSpec)
}

// bicepState stores the number of quotes to skip before the next content token.
type bicepState struct {
	quoteBeforeEndCount uint8
}

// BicepExternalScanner handles wildcard resource type matching and
// triple-single-quote multiline strings for Bicep.
//
// symbols holds the concrete gotreesitter.Symbol each external index maps to
// in the Language this instance was bound to (see ExternalScannerForLanguage).
// The scanner never hardcodes an absolute Symbol value: a blob regen can
// renumber the grammar's absolute symbol IDs without touching the externals
// list order, and a scanner that still called SetResultSymbol with a stale
// hardcoded ID would silently emit the wrong (but still structurally valid)
// node type instead of failing loudly.
type BicepExternalScanner struct {
	symbols         [bicepTokenCount]gotreesitter.Symbol
	externalToToken []int
}

// ExternalScannerForLanguage binds the scanner's token slots to the loaded
// Language's ExternalSymbols positionally. A hardcoded absolute
// gotreesitter.Symbol constant here would emit the wrong token whenever a
// grammar bump renumbers bicep's external symbols.
func (BicepExternalScanner) ExternalScannerForLanguage(lang *gotreesitter.Language) gotreesitter.ExternalScanner {
	s := BicepExternalScanner{symbols: bicepDefaultSymTable}
	s.externalToToken = bindExternalScannerSpec(lang, bicepExternalScannerSpec, func(tokenIdx int, sym gotreesitter.Symbol) {
		s.symbols[tokenIdx] = sym
	})
	return s
}

func (s BicepExternalScanner) symbolTable() *[bicepTokenCount]gotreesitter.Symbol {
	if s.symbols == ([bicepTokenCount]gotreesitter.Symbol{}) {
		return &bicepDefaultSymTable
	}
	return &s.symbols
}

func (BicepExternalScanner) Create() any         { return &bicepState{} }
func (BicepExternalScanner) Destroy(payload any) {}
func (BicepExternalScanner) Serialize(payload any, buf []byte) int {
	s := payload.(*bicepState)
	buf[0] = s.quoteBeforeEndCount
	return 1
}
func (BicepExternalScanner) Deserialize(payload any, buf []byte) {
	if len(buf) >= 1 {
		s := payload.(*bicepState)
		s.quoteBeforeEndCount = buf[0]
	}
}

func (sc BicepExternalScanner) Scan(payload any, lexer *gotreesitter.ExternalLexer, validSymbols []bool) bool {
	s := payload.(*bicepState)

	if len(sc.externalToToken) > 0 {
		var semanticValid [bicepTokenCount]bool
		for externalIdx, valid := range validSymbols {
			if !valid || externalIdx >= len(sc.externalToToken) {
				continue
			}
			tokenIdx := sc.externalToToken[externalIdx]
			if tokenIdx >= 0 && tokenIdx < bicepTokenCount {
				semanticValid[tokenIdx] = true
			}
		}
		validSymbols = semanticValid[:]
	}
	syms := sc.symbolTable()

	if bicepValid(validSymbols, bicepTokExternalAsterisk) {
		for unicode.IsSpace(lexer.Lookahead()) {
			lexer.Advance(true)
		}
		if lexer.Lookahead() == '*' {
			lexer.Advance(false)
			lexer.MarkEnd()
			lexer.SetResultSymbol(syms[bicepTokExternalAsterisk])
			if lexer.Lookahead() == ':' {
				return true
			}
		}
	}

	if bicepValid(validSymbols, bicepTokMultilineStringContent) {
		advancedOnce := false
		for lexer.Lookahead() != 0 {
			if lexer.Lookahead() == '\'' {
				if s.quoteBeforeEndCount > 0 {
					for s.quoteBeforeEndCount > 0 {
						lexer.Advance(false)
						s.quoteBeforeEndCount--
					}
					lexer.SetResultSymbol(syms[bicepTokMultilineStringContent])
					return true
				}

				lexer.MarkEnd()
				lexer.Advance(false)
				if lexer.Lookahead() == '\'' {
					lexer.Advance(false)
					if lexer.Lookahead() == '\'' {
						lexer.Advance(false)
						// Count extra quotes beyond the closing '''
						for lexer.Lookahead() == '\'' {
							s.quoteBeforeEndCount++
							lexer.Advance(false)
						}
						lexer.SetResultSymbol(syms[bicepTokMultilineStringContent])
						return advancedOnce
					}
				}
			}
			lexer.Advance(false)
			advancedOnce = true
		}
	}

	return false
}

func bicepValid(vs []bool, i int) bool { return i < len(vs) && vs[i] }
