//go:build !grammar_subset || grammar_subset_just

package grammarruntime

import (
	"unicode"

	gotreesitter "github.com/odvcencio/gotreesitter"
)

// External token indexes for the just grammar. This is the external
// index (the position of the token in the grammar's `externals: [...]`
// list), which is exactly what tree-sitter's `valid_symbols` array and
// C's result_symbol enum are indexed by. The external index is stable
// across a blob regen as long as the externals list itself does not
// reorder; concrete numeric gotreesitter.Symbol IDs are NOT stable (they
// shift whenever the grammar's total symbol count changes), so this
// scanner never hardcodes them -- see justDefaultSymTable below.
const (
	justTokIndent        = 0
	justTokDedent        = 1
	justTokNewline       = 2
	justTokText          = 3
	justTokErrorRecovery = 4 // error_recovery (never emitted by this scanner)
	justTokenCount       = 5
)

// justDefaultSymTable records the concrete gotreesitter.Symbol IDs the
// currently shipped just.bin assigns to each external, in justTok*
// order. It exists only as a pre-bind fallback (and as an independent
// value to compare a real bind against in tests); ExternalScannerForLanguage
// below overwrites it with values read from the actual loaded Language at
// bind time, which is what the scanner must do to survive a future blob
// regen that renumbers absolute symbol IDs without touching the externals
// list order.
var justDefaultSymTable = [justTokenCount]gotreesitter.Symbol{
	55, // _indent
	56, // _dedent
	57, // _newline
	58, // text
	59, // error_recovery (never emitted by this scanner)
}

// justExternalScannerSpec records the source contract for this
// hand-written port, so updater tooling can tell a grammar-only upstream
// change apart from one that also touches the external scanner or its
// token list. Its Externals list is also the binding source for
// ExternalScannerForLanguage: index i here is scanner token index i
// (justTok* order).
var justExternalScannerSpec = ExternalScannerSpec{
	Language:       "just",
	UpstreamRepo:   "https://github.com/IndianBoy42/tree-sitter-just",
	UpstreamCommit: "60df3d5b3fda2a22fdb3621226cafab50b763663",
	SourceFiles: []ExternalScannerSourceFile{
		{Path: "src/grammar.json", SHA256: "8f9060dc5706ccfd6a6fe96e4eeffc85906f6f61b5984ea268cce397db6af44b"},
		{Path: "src/scanner.c", SHA256: "99a4ae2ff08e90a18d21fc42d324411aa0df57ebf486e06e70c0b5dd26d489e8"},
	},
	Externals: []string{
		"_indent",
		"_dedent",
		"_newline",
		"text",
		"error_recovery",
	},
}

func init() {
	RegisterExternalScannerSpec(justExternalScannerSpec)
}

// justState tracks indent level and brace advancement for just files.
type justState struct {
	prevIndent        uint32
	advanceBraceCount uint16
	hasSeenEof        bool
}

// JustExternalScanner handles indent/dedent/newline/text for justfiles.
//
// symbols holds the concrete gotreesitter.Symbol each external index maps to
// in the Language this instance was bound to (see ExternalScannerForLanguage).
// The scanner never hardcodes an absolute Symbol value: a blob regen can
// renumber the grammar's absolute symbol IDs without touching the externals
// list order, and a scanner that still called SetResultSymbol with a stale
// hardcoded ID would silently emit the wrong (but still structurally valid)
// node type instead of failing loudly.
type JustExternalScanner struct {
	symbols         [justTokenCount]gotreesitter.Symbol
	externalToToken []int
}

// ExternalScannerForLanguage binds the scanner's token slots to the loaded
// Language's ExternalSymbols positionally. A hardcoded absolute
// gotreesitter.Symbol constant here would emit the wrong token whenever a
// grammar bump renumbers just's external symbols.
func (JustExternalScanner) ExternalScannerForLanguage(lang *gotreesitter.Language) gotreesitter.ExternalScanner {
	s := JustExternalScanner{symbols: justDefaultSymTable}
	s.externalToToken = bindExternalScannerSpec(lang, justExternalScannerSpec, func(tokenIdx int, sym gotreesitter.Symbol) {
		s.symbols[tokenIdx] = sym
	})
	return s
}

func (s JustExternalScanner) symbolTable() *[justTokenCount]gotreesitter.Symbol {
	if s.symbols == ([justTokenCount]gotreesitter.Symbol{}) {
		return &justDefaultSymTable
	}
	return &s.symbols
}

func (JustExternalScanner) Create() any         { return &justState{} }
func (JustExternalScanner) Destroy(payload any) {}

func (JustExternalScanner) Serialize(payload any, buf []byte) int {
	s := payload.(*justState)
	if len(buf) < 7 {
		return 0
	}
	buf[0] = byte(s.prevIndent)
	buf[1] = byte(s.prevIndent >> 8)
	buf[2] = byte(s.prevIndent >> 16)
	buf[3] = byte(s.prevIndent >> 24)
	buf[4] = byte(s.advanceBraceCount)
	buf[5] = byte(s.advanceBraceCount >> 8)
	if s.hasSeenEof {
		buf[6] = 1
	} else {
		buf[6] = 0
	}
	return 7
}

func (JustExternalScanner) Deserialize(payload any, buf []byte) {
	s := payload.(*justState)
	s.prevIndent = 0
	s.advanceBraceCount = 0
	s.hasSeenEof = false
	if len(buf) >= 7 {
		s.prevIndent = uint32(buf[0]) | uint32(buf[1])<<8 | uint32(buf[2])<<16 | uint32(buf[3])<<24
		s.advanceBraceCount = uint16(buf[4]) | uint16(buf[5])<<8
		s.hasSeenEof = buf[6] != 0
	}
}

func (sc JustExternalScanner) Scan(payload any, lexer *gotreesitter.ExternalLexer, validSymbols []bool) bool {
	s := payload.(*justState)

	if len(sc.externalToToken) > 0 {
		var semanticValid [justTokenCount]bool
		for externalIdx, valid := range validSymbols {
			if !valid || externalIdx >= len(sc.externalToToken) {
				continue
			}
			tokenIdx := sc.externalToToken[externalIdx]
			if tokenIdx >= 0 && tokenIdx < justTokenCount {
				semanticValid[tokenIdx] = true
			}
		}
		validSymbols = semanticValid[:]
	}
	syms := sc.symbolTable()

	if lexer.Lookahead() == 0 {
		return justHandleEof(lexer, s, validSymbols, syms)
	}

	// NEWLINE
	if justValid(validSymbols, justTokNewline) {
		escape := false
		if lexer.Lookahead() == '\\' {
			escape = true
			lexer.Advance(true)
		}

		eolFound := false
		for unicode.IsSpace(lexer.Lookahead()) {
			if lexer.Lookahead() == '\n' {
				lexer.Advance(true)
				eolFound = true
				break
			}
			lexer.Advance(true)
		}

		if eolFound && !escape {
			lexer.MarkEnd()
			lexer.SetResultSymbol(syms[justTokNewline])
			return true
		}
	}

	// INDENT / DEDENT
	if justValid(validSymbols, justTokIndent) || justValid(validSymbols, justTokDedent) {
		for lexer.Lookahead() != 0 && justIsSpace(lexer.Lookahead()) {
			switch lexer.Lookahead() {
			case '\n':
				if justValid(validSymbols, justTokIndent) {
					return false
				}
				lexer.Advance(true)
			case '\t', ' ':
				lexer.Advance(true)
			default:
				return false
			}
		}

		if lexer.Lookahead() == 0 {
			return justHandleEof(lexer, s, validSymbols, syms)
		}

		indent := lexer.Column()

		if indent > s.prevIndent && justValid(validSymbols, justTokIndent) && s.prevIndent == 0 {
			lexer.MarkEnd()
			lexer.SetResultSymbol(syms[justTokIndent])
			s.prevIndent = indent
			return true
		}
		if indent < s.prevIndent && justValid(validSymbols, justTokDedent) && indent == 0 {
			lexer.MarkEnd()
			lexer.SetResultSymbol(syms[justTokDedent])
			s.prevIndent = indent
			return true
		}
	}

	// TEXT
	if justValid(validSymbols, justTokText) {
		// Don't start text at column == prevIndent for certain chars
		if lexer.Column() == s.prevIndent &&
			(lexer.Lookahead() == '\n' || lexer.Lookahead() == '@' || lexer.Lookahead() == '-') {
			return false
		}

		advancedOnce := false

		// Advance past braces tracked from previous interpolation
		for lexer.Lookahead() == '{' && s.advanceBraceCount > 0 && lexer.Lookahead() != 0 {
			s.advanceBraceCount--
			lexer.Advance(false)
			advancedOnce = true
		}

		for {
			if lexer.Lookahead() == 0 {
				return justHandleEof(lexer, s, validSymbols, syms)
			}

			// Consume until newline or '{'
			for lexer.Lookahead() != 0 && lexer.Lookahead() != '\n' && lexer.Lookahead() != '{' {
				if lexer.Lookahead() == '#' && !advancedOnce {
					lexer.Advance(false)
					if lexer.Lookahead() == '!' {
						return false
					}
				}
				lexer.Advance(false)
				advancedOnce = true
			}

			if lexer.Lookahead() == '\n' || lexer.Lookahead() == 0 {
				lexer.MarkEnd()
				lexer.SetResultSymbol(syms[justTokText])
				if advancedOnce {
					return true
				}
				if lexer.Lookahead() == 0 {
					return justHandleEof(lexer, s, validSymbols, syms)
				}
				lexer.Advance(false)
			} else if lexer.Lookahead() == '{' {
				lexer.MarkEnd()
				lexer.Advance(false)

				if lexer.Lookahead() == 0 || lexer.Lookahead() == '\n' {
					lexer.MarkEnd()
					lexer.SetResultSymbol(syms[justTokText])
					return advancedOnce
				}

				if lexer.Lookahead() == '{' {
					lexer.Advance(false)

					for lexer.Lookahead() == '{' {
						s.advanceBraceCount++
						lexer.Advance(false)
					}

					// Scan till balanced }}
					for lexer.Lookahead() != 0 && lexer.Lookahead() != '\n' {
						lexer.Advance(false)
						if lexer.Lookahead() == '}' {
							lexer.Advance(false)
							if lexer.Lookahead() == '}' {
								lexer.SetResultSymbol(syms[justTokText])
								return advancedOnce
							}
						}
					}

					if !advancedOnce {
						return false
					}
				}
			}
		}
	}

	return false
}

func justHandleEof(lexer *gotreesitter.ExternalLexer, s *justState, validSymbols []bool, syms *[justTokenCount]gotreesitter.Symbol) bool {
	lexer.MarkEnd()

	if justValid(validSymbols, justTokDedent) {
		lexer.SetResultSymbol(syms[justTokDedent])
		return true
	}

	if justValid(validSymbols, justTokNewline) {
		if s.hasSeenEof {
			return false
		}
		lexer.SetResultSymbol(syms[justTokNewline])
		s.hasSeenEof = true
		return true
	}
	return false
}

func justIsSpace(ch rune) bool {
	return ch == ' ' || ch == '\t' || ch == '\n' || ch == '\r' || ch == '\f'
}

func justValid(vs []bool, i int) bool { return i < len(vs) && vs[i] }
