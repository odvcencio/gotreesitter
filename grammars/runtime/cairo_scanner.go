//go:build !grammar_subset || grammar_subset_cairo

package grammarruntime

import (
	"unicode"

	gotreesitter "github.com/odvcencio/gotreesitter"
)

// External token indexes for the cairo grammar. This is the external
// index (the position of the token in the grammar's `externals: [...]`
// list), which is exactly what tree-sitter's `valid_symbols` array and
// C's result_symbol enum are indexed by. The external index is stable
// across a blob regen as long as the externals list itself does not
// reorder; concrete numeric gotreesitter.Symbol IDs are NOT stable (they
// shift whenever the grammar's total symbol count changes), so this
// scanner never hardcodes them -- see cairoDefaultSymTable below.
//
// cairoTokHintStart never reaches SetResultSymbol (Scan returns false so
// the built-in lexer emits the literal "%{" token itself), but it still
// binds positionally like every other external.
const (
	cairoTokHintStart      = 0
	cairoTokPythonCodeLine = 1
	cairoTokFailure        = 2
	cairoTokenCount        = 3
)

// cairoDefaultSymTable records the concrete gotreesitter.Symbol IDs the
// currently shipped cairo.bin assigns to each external, in cairoTok*
// order. It exists only as a pre-bind fallback (and as an independent
// value to compare a real bind against in tests); ExternalScannerForLanguage
// below overwrites it with values read from the actual loaded Language at
// bind time, which is what the scanner must do to survive a future blob
// regen that renumbers absolute symbol IDs without touching the externals
// list order.
var cairoDefaultSymTable = [cairoTokenCount]gotreesitter.Symbol{
	56,  // "%{"
	136, // code_line
	137, // _failure
}

// cairoExternalScannerSpec records the source contract for this
// hand-written port, so updater tooling can tell a grammar-only upstream
// change apart from one that also touches the external scanner or its
// token list. Its Externals list is also the binding source for
// ExternalScannerForLanguage: index i here is scanner token index i
// (cairoTok* order).
var cairoExternalScannerSpec = ExternalScannerSpec{
	Language:       "cairo",
	UpstreamRepo:   "https://github.com/amaanq/tree-sitter-cairo",
	UpstreamCommit: "6238f609bea233040fe927858156dee5515a0745",
	SourceFiles: []ExternalScannerSourceFile{
		{Path: "src/grammar.json", SHA256: "165e9ee90fc1ee185d0b431eae7e2dd16c47b1551b42d1717deeba4aa768461c"},
		{Path: "src/scanner.c", SHA256: "db7a7afa9901800d8e2f98cf861ef84e8954b549d3757cae9b1482dab1a958fa"},
	},
	Externals: []string{
		"%{",
		"code_line",
		"_failure",
	},
}

func init() {
	RegisterExternalScannerSpec(cairoExternalScannerSpec)
}

// Cairo scanner context
const (
	cairoCtxNone          = 0
	cairoCtxPythonCode    = 1
	cairoCtxPythonString  = 2
	cairoCtxPythonComment = 3
)

// Python string type
const (
	cairoPstNone      = 0
	cairoPst1SqString = 1
	cairoPst3SqString = 2
	cairoPst1DqString = 3
	cairoPst3DqString = 4
)

// cairoState tracks the hint parsing context.
type cairoState struct {
	wsCount uint32
	context uint8
	pst     uint8
}

// CairoExternalScanner handles %{ %} hint blocks with embedded Python in Cairo.
//
// symbols holds the concrete gotreesitter.Symbol each external index maps to
// in the Language this instance was bound to (see ExternalScannerForLanguage).
// The scanner never hardcodes an absolute Symbol value: a blob regen can
// renumber the grammar's absolute symbol IDs without touching the externals
// list order, and a scanner that still called SetResultSymbol with a stale
// hardcoded ID would silently emit the wrong (but still structurally valid)
// node type instead of failing loudly.
type CairoExternalScanner struct {
	symbols         [cairoTokenCount]gotreesitter.Symbol
	externalToToken []int
}

// ExternalScannerForLanguage binds the scanner's token slots to the loaded
// Language's ExternalSymbols positionally. A hardcoded absolute
// gotreesitter.Symbol constant here would emit the wrong token whenever a
// grammar bump renumbers cairo's external symbols.
func (CairoExternalScanner) ExternalScannerForLanguage(lang *gotreesitter.Language) gotreesitter.ExternalScanner {
	s := CairoExternalScanner{symbols: cairoDefaultSymTable}
	s.externalToToken = bindExternalScannerSpec(lang, cairoExternalScannerSpec, func(tokenIdx int, sym gotreesitter.Symbol) {
		s.symbols[tokenIdx] = sym
	})
	return s
}

func (s CairoExternalScanner) symbolTable() *[cairoTokenCount]gotreesitter.Symbol {
	if s.symbols == ([cairoTokenCount]gotreesitter.Symbol{}) {
		return &cairoDefaultSymTable
	}
	return &s.symbols
}

func (CairoExternalScanner) Create() any         { return &cairoState{} }
func (CairoExternalScanner) Destroy(payload any) {}

func (CairoExternalScanner) Serialize(payload any, buf []byte) int {
	s := payload.(*cairoState)
	if len(buf) < 6 {
		return 0
	}
	buf[0] = byte(s.wsCount)
	buf[1] = byte(s.wsCount >> 8)
	buf[2] = byte(s.wsCount >> 16)
	buf[3] = byte(s.wsCount >> 24)
	buf[4] = s.context
	buf[5] = s.pst
	return 6
}

func (CairoExternalScanner) Deserialize(payload any, buf []byte) {
	s := payload.(*cairoState)
	*s = cairoState{}
	if len(buf) >= 6 {
		s.wsCount = uint32(buf[0]) | uint32(buf[1])<<8 | uint32(buf[2])<<16 | uint32(buf[3])<<24
		s.context = buf[4]
		s.pst = buf[5]
	}
}

func (sc CairoExternalScanner) Scan(payload any, lexer *gotreesitter.ExternalLexer, validSymbols []bool) bool {
	s := payload.(*cairoState)

	if len(sc.externalToToken) > 0 {
		var semanticValid [cairoTokenCount]bool
		for externalIdx, valid := range validSymbols {
			if !valid || externalIdx >= len(sc.externalToToken) {
				continue
			}
			tokenIdx := sc.externalToToken[externalIdx]
			if tokenIdx >= 0 && tokenIdx < cairoTokenCount {
				semanticValid[tokenIdx] = true
			}
		}
		validSymbols = semanticValid[:]
	}
	syms := sc.symbolTable()

	if cairoValid(validSymbols, cairoTokFailure) {
		return false
	}

	// HINT_START: detect %{ to enter hint mode
	if cairoValid(validSymbols, cairoTokHintStart) {
		if lexer.Lookahead() == '%' {
			lexer.Advance(true)
			if lexer.Lookahead() == '{' {
				s.context = cairoCtxPythonCode
				return false // let built-in lexer handle the actual token
			}
		}
	}

	// PYTHON_CODE_LINE: scan lines of Python code inside %{ %}
	if cairoValid(validSymbols, cairoTokPythonCodeLine) {
		// Skip leading newline after %{
		if lexer.Lookahead() == '\n' {
			lexer.Advance(true)
		}

		// Check for standalone %} close
		if lexer.Lookahead() == '%' {
			lexer.MarkEnd()
			lexer.Advance(false)
			if lexer.Lookahead() == '}' {
				if s.context == cairoCtxPythonString {
					lexer.SetResultSymbol(syms[cairoTokFailure])
					return true
				}
				s.context = cairoCtxNone
				return false
			}
		}

		// Skip leading whitespace, count it
		var wsCount uint32
		for lexer.Lookahead() != 0 {
			if lexer.Lookahead() == '\n' {
				lexer.Advance(false)
				lexer.MarkEnd()
				lexer.SetResultSymbol(syms[cairoTokPythonCodeLine])
				return true
			}
			if unicode.IsSpace(lexer.Lookahead()) {
				if lexer.Lookahead() == '\t' {
					wsCount += 8
				} else {
					wsCount++
				}
				lexer.Advance(true)
				if s.wsCount > 0 && wsCount == s.wsCount {
					break
				}
			} else {
				if s.wsCount == 0 || wsCount < s.wsCount {
					s.wsCount = wsCount
				}
				break
			}
		}

		// Scan content of line
		contentLen := uint32(0)
		for lexer.Lookahead() != 0 {
			switch lexer.Lookahead() {
			case '\'', '"':
				chr := lexer.Lookahead()
				lexer.Advance(false)
				contentLen++
				if s.context == cairoCtxPythonString {
					iter := 0
					if s.pst == cairoPst3DqString || s.pst == cairoPst3SqString {
						iter = 2
					}
					for iter > 0 {
						if lexer.Lookahead() != chr {
							s.context = cairoCtxPythonCode
							s.pst = cairoPstNone
							return false
						}
						lexer.Advance(false)
						contentLen++
						iter--
					}
					s.context = cairoCtxPythonCode
					s.pst = cairoPstNone
					continue
				}
				if lexer.Lookahead() == chr {
					lexer.Advance(false)
					contentLen++
					if lexer.Lookahead() == chr {
						lexer.Advance(false)
						contentLen++
						s.context = cairoCtxPythonString
						if chr == '"' {
							s.pst = cairoPst3DqString
						} else {
							s.pst = cairoPst3SqString
						}
					} else {
						s.context = cairoCtxPythonCode
						s.pst = cairoPstNone
					}
				} else {
					s.context = cairoCtxPythonString
					if chr == '"' {
						s.pst = cairoPst1DqString
					} else {
						s.pst = cairoPst1SqString
					}
				}
				continue

			case '%':
				if s.context == cairoCtxPythonString {
					lexer.Advance(false)
					contentLen++
					continue
				}
				lexer.MarkEnd()
				lexer.Advance(false)
				if lexer.Lookahead() == '}' {
					if s.context == cairoCtxPythonString {
						lexer.SetResultSymbol(syms[cairoTokFailure])
						return true
					}
					s.context = cairoCtxNone
					if contentLen > 0 {
						lexer.SetResultSymbol(syms[cairoTokPythonCodeLine])
						return true
					}
					return false
				}

			case '\n':
				lexer.Advance(false)
				lexer.MarkEnd()
				lexer.SetResultSymbol(syms[cairoTokPythonCodeLine])
				return true

			case '#':
				if s.context == cairoCtxPythonString {
					lexer.Advance(false)
					contentLen++
					continue
				}
				s.context = cairoCtxPythonComment
				for lexer.Lookahead() != '\n' && lexer.Lookahead() != 0 {
					lexer.Advance(false)
					contentLen++
				}
				s.context = cairoCtxNone
				continue

			default:
				lexer.Advance(false)
				contentLen++
			}
		}
	}

	return false
}

func cairoValid(vs []bool, i int) bool { return i < len(vs) && vs[i] }
