//go:build !grammar_subset || grammar_subset_teal

package grammarruntime

import gotreesitter "github.com/odvcencio/gotreesitter"

// External token indexes for the teal grammar.
const (
	tealTokComment         = 0
	tealTokLongStringStart = 1
	tealTokLongStringChar  = 2
	tealTokLongStringEnd   = 3
	tealTokShortStrStart   = 4
	tealTokShortStrChar    = 5
	tealTokShortStrEnd     = 6
	tealTokenCount         = 7
)

// tealDefaultSymTable records the concrete gotreesitter.Symbol IDs the
// currently shipped teal.bin assigns to each external, in tealTok* order.
// It exists only as a pre-bind fallback (and as an independent value to
// compare a real bind against in tests); ExternalScannerForLanguage below
// overwrites it with values read from the actual loaded Language at bind
// time, which is what the scanner must do to survive a future blob regen
// that renumbers absolute symbol IDs without touching the externals list
// order.
var tealDefaultSymTable = [tealTokenCount]gotreesitter.Symbol{
	76, // comment
	77, // _long_string_start
	78, // _long_string_char
	79, // _long_string_end
	80, // _short_string_start
	81, // _short_string_char
	82, // _short_string_end
}

// tealExternalScannerSpec records the source contract for this
// hand-written port, so updater tooling can tell a grammar-only upstream
// change apart from one that also touches the external scanner or its
// token list. Its Externals list is also the binding source for
// ExternalScannerForLanguage: index i here is scanner token index i
// (tealTok* order).
var tealExternalScannerSpec = ExternalScannerSpec{
	Language:       "teal",
	UpstreamRepo:   "https://github.com/euclidianAce/tree-sitter-teal",
	UpstreamCommit: "05d276e737055e6f77a21335b7573c9d3c091e2f",
	SourceFiles: []ExternalScannerSourceFile{
		{Path: "src/grammar.json", SHA256: "82b4dcd6e6d14bf1afcf8feb12f36bff17d4ffde4e90d1c8997131b233e351cb"},
		{Path: "src/scanner.c", SHA256: "cd68fc9be970e004d038a5f1e2e6c119da64cfbc22ba2a72278565b4bc261c18"},
	},
	Externals: []string{
		"comment",
		"_long_string_start",
		"_long_string_char",
		"_long_string_end",
		"_short_string_start",
		"_short_string_char",
		"_short_string_end",
	},
}

func init() {
	RegisterExternalScannerSpec(tealExternalScannerSpec)
}

// tealState tracks Lua-style string parsing state.
type tealState struct {
	openingEqs   uint32
	inStr        bool
	openingQuote rune
}

// TealExternalScanner handles Teal/Lua string and comment scanning.
//
// symbols holds the concrete gotreesitter.Symbol each external index maps to
// in the Language this instance was bound to (see ExternalScannerForLanguage).
// The scanner never hardcodes an absolute Symbol value: a blob regen can
// renumber the grammar's absolute symbol IDs without touching the externals
// list order, and a scanner that still called SetResultSymbol with a stale
// hardcoded ID would silently emit the wrong (but still structurally valid)
// node type instead of failing loudly.
type TealExternalScanner struct {
	symbols         [tealTokenCount]gotreesitter.Symbol
	externalToToken []int
}

// ExternalScannerForLanguage binds the scanner's token slots to the loaded
// Language's ExternalSymbols positionally. A hardcoded absolute
// gotreesitter.Symbol constant here would emit the wrong token whenever a
// grammar bump renumbers teal's external symbols.
func (TealExternalScanner) ExternalScannerForLanguage(lang *gotreesitter.Language) gotreesitter.ExternalScanner {
	s := TealExternalScanner{symbols: tealDefaultSymTable}
	s.externalToToken = bindExternalScannerSpec(lang, tealExternalScannerSpec, func(tokenIdx int, sym gotreesitter.Symbol) {
		s.symbols[tokenIdx] = sym
	})
	return s
}

func (s TealExternalScanner) symbolTable() *[tealTokenCount]gotreesitter.Symbol {
	if s.symbols == ([tealTokenCount]gotreesitter.Symbol{}) {
		return &tealDefaultSymTable
	}
	return &s.symbols
}

func (TealExternalScanner) Create() any         { return &tealState{} }
func (TealExternalScanner) Destroy(payload any) {}

func (TealExternalScanner) Serialize(payload any, buf []byte) int {
	s := payload.(*tealState)
	if len(buf) < 6 {
		return 0
	}
	buf[0] = byte(s.openingEqs)
	buf[1] = byte(s.openingEqs >> 8)
	buf[2] = byte(s.openingEqs >> 16)
	buf[3] = byte(s.openingEqs >> 24)
	if s.inStr {
		buf[4] = 1
	} else {
		buf[4] = 0
	}
	buf[5] = byte(s.openingQuote)
	return 6
}

func (TealExternalScanner) Deserialize(payload any, buf []byte) {
	s := payload.(*tealState)
	*s = tealState{}
	if len(buf) >= 6 {
		s.openingEqs = uint32(buf[0]) | uint32(buf[1])<<8 | uint32(buf[2])<<16 | uint32(buf[3])<<24
		s.inStr = buf[4] != 0
		s.openingQuote = rune(buf[5])
	}
}

func (sc TealExternalScanner) Scan(payload any, lexer *gotreesitter.ExternalLexer, validSymbols []bool) bool {
	if len(sc.externalToToken) > 0 {
		var semanticValid [tealTokenCount]bool
		for externalIdx, valid := range validSymbols {
			if !valid || externalIdx >= len(sc.externalToToken) {
				continue
			}
			tokenIdx := sc.externalToToken[externalIdx]
			if tokenIdx >= 0 && tokenIdx < tealTokenCount {
				semanticValid[tokenIdx] = true
			}
		}
		validSymbols = semanticValid[:]
	}
	syms := sc.symbolTable()

	s := payload.(*tealState)

	if lexer.Lookahead() == 0 {
		return false
	}

	// Inside a string, handle end/char
	if s.inStr {
		if s.openingQuote > 0 {
			// Short string mode
			if tealValid(validSymbols, tealTokShortStrEnd) && lexer.Lookahead() == s.openingQuote {
				lexer.Advance(false)
				lexer.SetResultSymbol(syms[tealTokShortStrEnd])
				*s = tealState{}
				return true
			}
			if tealValid(validSymbols, tealTokShortStrChar) {
				ch := lexer.Lookahead()
				if ch != s.openingQuote && ch != '\n' && ch != '\r' && ch != '\\' && ch != '%' {
					lexer.Advance(false)
					lexer.SetResultSymbol(syms[tealTokShortStrChar])
					return true
				}
			}
			return false
		}

		// Long string mode
		if lexer.Lookahead() == ']' {
			lexer.Advance(false)
			eqs := tealConsumeEqs(lexer)
			if s.openingEqs == eqs && lexer.Lookahead() == ']' {
				lexer.Advance(false)
				lexer.SetResultSymbol(syms[tealTokLongStringEnd])
				*s = tealState{}
				return true
			}
		}
		// Long string char (not %)
		if lexer.Lookahead() == '%' {
			return false
		}
		lexer.Advance(false)
		lexer.SetResultSymbol(syms[tealTokLongStringChar])
		return true
	}

	// Skip whitespace
	for tealIsASCIIWhitespace(lexer.Lookahead()) {
		lexer.Advance(true)
	}

	// Short string start
	if tealValid(validSymbols, tealTokShortStrStart) {
		if lexer.Lookahead() == '"' || lexer.Lookahead() == '\'' {
			s.openingQuote = lexer.Lookahead()
			s.inStr = true
			lexer.Advance(false)
			lexer.SetResultSymbol(syms[tealTokShortStrStart])
			return true
		}
	}

	// Long string start: [=*[
	if tealValid(validSymbols, tealTokLongStringStart) {
		if lexer.Lookahead() == '[' {
			lexer.Advance(false)
			*s = tealState{}
			eqs := tealConsumeEqs(lexer)
			if lexer.Lookahead() == '[' {
				lexer.Advance(false)
				s.inStr = true
				s.openingEqs = eqs
				lexer.SetResultSymbol(syms[tealTokLongStringStart])
				return true
			}
			return false
		}
	}

	// Comment: -- followed by optional [=*[ for long comment
	if tealValid(validSymbols, tealTokComment) {
		return tealScanComment(lexer, syms[tealTokComment])
	}

	return false
}

func tealScanComment(lexer *gotreesitter.ExternalLexer, sym gotreesitter.Symbol) bool {
	if lexer.Lookahead() != '-' {
		return false
	}
	lexer.Advance(false)
	if lexer.Lookahead() != '-' {
		return false
	}
	lexer.Advance(false)

	lexer.SetResultSymbol(sym)

	// Check for long comment --[=*[
	if lexer.Lookahead() != '[' {
		tealConsumeRestOfLine(lexer)
		return true
	}
	lexer.Advance(false)
	eqs := tealConsumeEqs(lexer)

	if lexer.Lookahead() != '[' {
		tealConsumeRestOfLine(lexer)
		return true
	}

	// Long comment: consume until ]=*]
	for lexer.Lookahead() != 0 {
		for lexer.Lookahead() != 0 && lexer.Lookahead() != ']' {
			lexer.Advance(false)
		}
		if lexer.Lookahead() != ']' {
			return true
		}
		lexer.Advance(false)
		testEqs := tealConsumeEqs(lexer)
		if lexer.Lookahead() == ']' {
			lexer.Advance(false)
			if testEqs == eqs {
				return true
			}
		} else if lexer.Lookahead() != 0 {
			lexer.Advance(false)
		}
	}

	return true
}

func tealConsumeEqs(lexer *gotreesitter.ExternalLexer) uint32 {
	var count uint32
	for lexer.Lookahead() == '=' {
		lexer.Advance(false)
		count++
	}
	return count
}

func tealConsumeRestOfLine(lexer *gotreesitter.ExternalLexer) {
	for lexer.Lookahead() != 0 && lexer.Lookahead() != '\n' && lexer.Lookahead() != '\r' {
		lexer.Advance(false)
	}
}

func tealIsASCIIWhitespace(ch rune) bool {
	return ch == '\n' || ch == '\r' || ch == ' ' || ch == '\t'
}

func tealValid(vs []bool, i int) bool { return i < len(vs) && vs[i] }
