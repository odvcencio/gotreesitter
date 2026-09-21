//go:build !grammar_subset || grammar_subset_nickel

package grammarruntime

import (
	"unicode"

	gotreesitter "github.com/odvcencio/gotreesitter"
)

// External token indexes for the nickel grammar. This is the external
// index (the position of the token in the grammar's `externals: [...]`
// list), which is exactly what tree-sitter's `valid_symbols` array and
// C's result_symbol enum are indexed by. The external index is stable
// across a blob regen as long as the externals list itself does not
// reorder; concrete numeric gotreesitter.Symbol IDs are NOT stable (they
// shift whenever the grammar's total symbol count changes), so this
// scanner never hardcodes them -- see nickelDefaultSymTable below.
const (
	nickelTokMultstrStart       = 0
	nickelTokMultstrEnd         = 1
	nickelTokStrStart           = 2
	nickelTokStrEnd             = 3
	nickelTokInterpStart        = 4
	nickelTokInterpEnd          = 5
	nickelTokQuotedEnumTagStart = 6
	nickelTokComment            = 7
	nickelTokenCount            = 8
)

// nickelDefaultSymTable records the concrete gotreesitter.Symbol IDs the
// currently shipped nickel.bin assigns to each external, in nickelTok*
// order. It exists only as a pre-bind fallback (and as an independent
// value to compare a real bind against in tests); ExternalScannerForLanguage
// below overwrites it with values read from the actual loaded Language at
// bind time, which is what the scanner must do to survive a future blob
// regen that renumbers absolute symbol IDs without touching the externals
// list order.
var nickelDefaultSymTable = [nickelTokenCount]gotreesitter.Symbol{
	74, // multstr_start
	75, // multstr_end
	76, // _str_start
	77, // _str_end
	78, // interpolation_start
	79, // interpolation_end
	80, // quoted_enum_tag_start
	81, // comment
}

// nickelExternalScannerSpec records the source contract for this
// hand-written port, so updater tooling can tell a grammar-only upstream
// change apart from one that also touches the external scanner or its
// token list. Its Externals list is also the binding source for
// ExternalScannerForLanguage: index i here is scanner token index i
// (nickelTok* order).
var nickelExternalScannerSpec = ExternalScannerSpec{
	Language:       "nickel",
	UpstreamRepo:   "https://github.com/nickel-lang/tree-sitter-nickel",
	UpstreamCommit: "b5b6cc3bc7b9ea19f78fed264190685419cd17a8",
	SourceFiles: []ExternalScannerSourceFile{
		{Path: "src/grammar.json", SHA256: "89954727957f261f4c4ded77774762da1a42cd8f03c80990347259eb141de959"},
		{Path: "src/scanner.c", SHA256: "0f9af5cf8137ee314372a307b7bd06057851d97530f7ea0831028d3f9bbd6348"},
	},
	Externals: []string{
		"multstr_start",
		"multstr_end",
		"_str_start",
		"_str_end",
		"interpolation_start",
		"interpolation_end",
		"quoted_enum_tag_start",
		"comment",
	},
}

func init() {
	RegisterExternalScannerSpec(nickelExternalScannerSpec)
}

// nickelState tracks the percent count stack for nested strings.
type nickelState struct {
	expectedPercentCount []uint8
}

// NickelExternalScanner handles Nickel's multi-line strings, interpolation, and comments.
//
// symbols holds the concrete gotreesitter.Symbol each external index maps to
// in the Language this instance was bound to (see ExternalScannerForLanguage).
// The scanner never hardcodes an absolute Symbol value: a blob regen can
// renumber the grammar's absolute symbol IDs without touching the externals
// list order, and a scanner that still called SetResultSymbol with a stale
// hardcoded ID would silently emit the wrong (but still structurally valid)
// node type instead of failing loudly.
type NickelExternalScanner struct {
	symbols         [nickelTokenCount]gotreesitter.Symbol
	externalToToken []int
}

// ExternalScannerForLanguage binds the scanner's token slots to the loaded
// Language's ExternalSymbols positionally. A hardcoded absolute
// gotreesitter.Symbol constant here would emit the wrong token whenever a
// grammar bump renumbers nickel's external symbols.
func (NickelExternalScanner) ExternalScannerForLanguage(lang *gotreesitter.Language) gotreesitter.ExternalScanner {
	s := NickelExternalScanner{symbols: nickelDefaultSymTable}
	s.externalToToken = bindExternalScannerSpec(lang, nickelExternalScannerSpec, func(tokenIdx int, sym gotreesitter.Symbol) {
		s.symbols[tokenIdx] = sym
	})
	return s
}

func (s NickelExternalScanner) symbolTable() *[nickelTokenCount]gotreesitter.Symbol {
	if s.symbols == ([nickelTokenCount]gotreesitter.Symbol{}) {
		return &nickelDefaultSymTable
	}
	return &s.symbols
}

func (NickelExternalScanner) Create() any         { return &nickelState{} }
func (NickelExternalScanner) Destroy(payload any) {}

func (NickelExternalScanner) Serialize(payload any, buf []byte) int {
	s := payload.(*nickelState)
	if len(s.expectedPercentCount)+1 > len(buf) {
		return 0
	}
	l := len(s.expectedPercentCount)
	if l > 255 {
		l = 255
	}
	n := 0
	buf[n] = byte(l)
	n++
	for i := 0; i < l; i++ {
		buf[n] = s.expectedPercentCount[i]
		n++
	}
	return n
}

func (NickelExternalScanner) Deserialize(payload any, buf []byte) {
	s := payload.(*nickelState)
	s.expectedPercentCount = s.expectedPercentCount[:0]
	if len(buf) > 0 {
		vecLen := int(buf[0])
		for i := 1; i <= vecLen && i < len(buf); i++ {
			s.expectedPercentCount = append(s.expectedPercentCount, buf[i])
		}
	}
}

func (sc NickelExternalScanner) Scan(payload any, lexer *gotreesitter.ExternalLexer, validSymbols []bool) bool {
	if len(sc.externalToToken) > 0 {
		var semanticValid [nickelTokenCount]bool
		for externalIdx, valid := range validSymbols {
			if !valid || externalIdx >= len(sc.externalToToken) {
				continue
			}
			tokenIdx := sc.externalToToken[externalIdx]
			if tokenIdx >= 0 && tokenIdx < nickelTokenCount {
				semanticValid[tokenIdx] = true
			}
		}
		validSymbols = semanticValid[:]
	}
	syms := sc.symbolTable()

	s := payload.(*nickelState)

	// Error recovery guard
	if nickelValid(validSymbols, nickelTokMultstrStart) && nickelValid(validSymbols, nickelTokMultstrEnd) &&
		nickelValid(validSymbols, nickelTokStrStart) && nickelValid(validSymbols, nickelTokStrEnd) &&
		nickelValid(validSymbols, nickelTokInterpStart) && nickelValid(validSymbols, nickelTokInterpEnd) &&
		nickelValid(validSymbols, nickelTokComment) && nickelValid(validSymbols, nickelTokQuotedEnumTagStart) {
		return false
	}

	// Skip whitespace
	for unicode.IsSpace(lexer.Lookahead()) {
		lexer.Advance(true)
	}

	switch lexer.Lookahead() {
	case '"':
		if nickelValid(validSymbols, nickelTokMultstrEnd) {
			lexer.Advance(false)
			if lexer.Lookahead() == '%' {
				return nickelScanMultstrEnd(s, lexer, syms)
			}
		} else if nickelValid(validSymbols, nickelTokStrStart) {
			return nickelScanStrStart(s, lexer, syms)
		} else if nickelValid(validSymbols, nickelTokStrEnd) {
			return nickelScanStrEnd(s, lexer, syms)
		}
	case '%':
		if nickelValid(validSymbols, nickelTokInterpStart) {
			return nickelScanInterpStart(s, lexer, syms)
		}
	case '}':
		if nickelValid(validSymbols, nickelTokInterpEnd) {
			lexer.Advance(false)
			lexer.SetResultSymbol(syms[nickelTokInterpEnd])
			return true
		}
	case '\'':
		if nickelValid(validSymbols, nickelTokQuotedEnumTagStart) {
			lexer.Advance(false)
			if lexer.Lookahead() == '"' {
				return nickelScanQuotedEnumTagStart(s, lexer, syms)
			}
		}
	case '#':
		if nickelValid(validSymbols, nickelTokComment) {
			return nickelScanComment(s, lexer, syms)
		}
	default:
		if nickelValid(validSymbols, nickelTokMultstrStart) {
			return nickelScanMultstrStart(s, lexer, syms)
		}
	}

	return false
}

func nickelScanMultstrStart(s *nickelState, lexer *gotreesitter.ExternalLexer, syms *[nickelTokenCount]gotreesitter.Symbol) bool {
	lexer.SetResultSymbol(syms[nickelTokMultstrStart])
	mScanned := false

	if lexer.Lookahead() == 'm' {
		lexer.Advance(false)
		mScanned = true
	}

	if mScanned && lexer.Lookahead() == '%' {
		lexer.Advance(false)
	} else if !nickelScanUntilSstrStartEnd(lexer, mScanned) {
		return false
	}

	// Count % signs (starting at 1 since we already consumed one)
	count := uint8(1)
	for lexer.Lookahead() == '%' {
		count++
		lexer.Advance(false)
	}

	quote := false
	if lexer.Lookahead() == '"' {
		quote = true
		lexer.Advance(false)
	}

	s.expectedPercentCount = append(s.expectedPercentCount, count)
	return quote
}

func nickelScanMultstrEnd(s *nickelState, lexer *gotreesitter.ExternalLexer, syms *[nickelTokenCount]gotreesitter.Symbol) bool {
	lexer.SetResultSymbol(syms[nickelTokMultstrEnd])
	if len(s.expectedPercentCount) == 0 {
		return false
	}
	count := s.expectedPercentCount[len(s.expectedPercentCount)-1]

	for lexer.Lookahead() == '%' && count > 0 {
		count--
		lexer.Advance(false)
	}

	s.expectedPercentCount = s.expectedPercentCount[:len(s.expectedPercentCount)-1]
	return count == 0 && lexer.Lookahead() != '{'
}

func nickelScanStrStart(s *nickelState, lexer *gotreesitter.ExternalLexer, syms *[nickelTokenCount]gotreesitter.Symbol) bool {
	lexer.SetResultSymbol(syms[nickelTokStrStart])
	s.expectedPercentCount = append(s.expectedPercentCount, 1)
	lexer.Advance(false)
	return true
}

func nickelScanStrEnd(s *nickelState, lexer *gotreesitter.ExternalLexer, syms *[nickelTokenCount]gotreesitter.Symbol) bool {
	lexer.SetResultSymbol(syms[nickelTokStrEnd])
	lexer.Advance(false)
	if len(s.expectedPercentCount) > 0 {
		s.expectedPercentCount = s.expectedPercentCount[:len(s.expectedPercentCount)-1]
	}
	return true
}

func nickelScanInterpStart(s *nickelState, lexer *gotreesitter.ExternalLexer, syms *[nickelTokenCount]gotreesitter.Symbol) bool {
	lexer.SetResultSymbol(syms[nickelTokInterpStart])
	if len(s.expectedPercentCount) == 0 {
		return false
	}
	count := s.expectedPercentCount[len(s.expectedPercentCount)-1]
	if count == 0 {
		return false // no interpolation allowed
	}

	for lexer.Lookahead() == '%' {
		count--
		lexer.Advance(false)
	}

	brace := false
	if lexer.Lookahead() == '{' {
		brace = true
		lexer.Advance(false)
	}

	return brace && count == 0
}

func nickelScanQuotedEnumTagStart(s *nickelState, lexer *gotreesitter.ExternalLexer, syms *[nickelTokenCount]gotreesitter.Symbol) bool {
	lexer.SetResultSymbol(syms[nickelTokQuotedEnumTagStart])
	// 0 = no interpolation allowed
	s.expectedPercentCount = append(s.expectedPercentCount, 0)
	lexer.Advance(false)
	return true
}

func nickelScanComment(s *nickelState, lexer *gotreesitter.ExternalLexer, syms *[nickelTokenCount]gotreesitter.Symbol) bool {
	lexer.SetResultSymbol(syms[nickelTokComment])
	// Only allow comments outside strings
	if len(s.expectedPercentCount) > 0 {
		return false
	}
	lexer.Advance(false)
	for lexer.Lookahead() != '\n' && lexer.Lookahead() != 0 {
		lexer.Advance(false)
	}
	return true
}

// nickelScanUntilSstrStartEnd recognizes a symbolic string prefix like "tag-s%".
func nickelScanUntilSstrStartEnd(lexer *gotreesitter.ExternalLexer, mScanned bool) bool {
	const (
		stStart   = 0
		stMiddle  = 1
		stDash    = 2
		stS       = 3
		stPercent = 4
	)
	state := stStart
	if mScanned {
		state = stMiddle
	}
	for lexer.Lookahead() != 0 {
		ch := lexer.Lookahead()
		switch state {
		case stStart:
			if nickelIsSymtagStart(ch) {
				lexer.Advance(false)
				state = stMiddle
			} else {
				return false
			}
		case stMiddle:
			if !nickelIsSymtagMiddle(ch) {
				return false
			}
			if ch == '-' {
				state = stDash
			}
			lexer.Advance(false)
		case stDash:
			if ch == 's' {
				state = stS
				lexer.Advance(false)
			} else {
				state = stMiddle
			}
		case stS:
			if ch == '%' {
				state = stPercent
				lexer.Advance(false)
			} else {
				state = stMiddle
			}
		case stPercent:
			return true
		}
	}
	return false
}

func nickelIsSymtagStart(ch rune) bool {
	return (ch >= 'a' && ch <= 'z') || (ch >= 'A' && ch <= 'Z')
}

func nickelIsSymtagMiddle(ch rune) bool {
	return nickelIsSymtagStart(ch) || (ch >= '0' && ch <= '9') || ch == '-' || ch == '\'' || ch == '_'
}

func nickelValid(vs []bool, i int) bool { return i < len(vs) && vs[i] }
