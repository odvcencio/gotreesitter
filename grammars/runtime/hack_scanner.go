//go:build !grammar_subset || grammar_subset_hack

package grammarruntime

import (
	"unicode"

	gotreesitter "github.com/odvcencio/gotreesitter"
)

// External token indexes for the hack grammar. This is the external
// index (the position of the token in the grammar's `externals: [...]`
// list), which is exactly what tree-sitter's `valid_symbols` array and
// C's result_symbol enum are indexed by. The external index is stable
// across a blob regen as long as the externals list itself does not
// reorder; concrete numeric gotreesitter.Symbol IDs are NOT stable (they
// shift whenever the grammar's total symbol count changes), so this
// scanner never hardcodes them -- see hackDefaultSymTable below.
const (
	hackTokHeredocStart        = 0
	hackTokHeredocStartNewline = 1
	hackTokHeredocBody         = 2
	hackTokHeredocEndNewline   = 3
	hackTokHeredocEnd          = 4
	hackTokEmbeddedOpenBrace   = 5
	hackTokenCount             = 6
)

// hackDefaultSymTable records the concrete gotreesitter.Symbol IDs the
// currently shipped hack.bin assigns to each external, in hackTok*
// order. It exists only as a pre-bind fallback (and as an independent
// value to compare a real bind against in tests); ExternalScannerForLanguage
// below overwrites it with values read from the actual loaded Language at
// bind time, which is what the scanner must do to survive a future blob
// regen that renumbers absolute symbol IDs without touching the externals
// list order.
var hackDefaultSymTable = [hackTokenCount]gotreesitter.Symbol{
	182, // _heredoc_start
	183, // _heredoc_start_newline, displays as "\n"
	184, // _heredoc_body
	185, // _heredoc_end_newline, displays as "\n"
	186, // _heredoc_end
	187, // _embedded_opening_brace, displays as "{"
}

// hackExternalScannerSpec records the source contract for this
// hand-written port, so updater tooling can tell a grammar-only upstream
// change apart from one that also touches the external scanner or its
// token list. Its Externals list is also the binding source for
// ExternalScannerForLanguage: index i here is scanner token index i
// (hackTok* order).
var hackExternalScannerSpec = ExternalScannerSpec{
	Language:       "hack",
	UpstreamRepo:   "https://github.com/slackhq/tree-sitter-hack",
	UpstreamCommit: "1a7ded90288189746c54861ac144ede97df95081",
	SourceFiles: []ExternalScannerSourceFile{
		{Path: "src/grammar.json", SHA256: "0e445b649f150edb955ef94238645f5b81afd0531afb0c4998d283f8f0ffb823"},
		{Path: "src/scanner.c", SHA256: "6d003ff56371b4ee31252ecfab216dccd2ce22ddcbafb482ca167fca77d5b199"},
	},
	Externals: []string{
		"_heredoc_start",
		"_heredoc_start_newline",
		"_heredoc_body",
		"_heredoc_end_newline",
		"_heredoc_end",
		"_embedded_opening_brace",
	},
}

func init() {
	RegisterExternalScannerSpec(hackExternalScannerSpec)
}

// hackState tracks the heredoc delimiter and parsing phase.
type hackState struct {
	delimiter []byte
	isNowdoc  bool
	didStart  bool
	didEnd    bool
}

// HackExternalScanner handles heredoc/nowdoc strings for Hack.
//
// symbols holds the concrete gotreesitter.Symbol each external index maps to
// in the Language this instance was bound to (see ExternalScannerForLanguage).
// The scanner never hardcodes an absolute Symbol value: a blob regen can
// renumber the grammar's absolute symbol IDs without touching the externals
// list order, and a scanner that still called SetResultSymbol with a stale
// hardcoded ID would silently emit the wrong (but still structurally valid)
// node type instead of failing loudly.
type HackExternalScanner struct {
	symbols         [hackTokenCount]gotreesitter.Symbol
	externalToToken []int
}

// ExternalScannerForLanguage binds the scanner's token slots to the loaded
// Language's ExternalSymbols positionally. A hardcoded absolute
// gotreesitter.Symbol constant here would emit the wrong token whenever a
// grammar bump renumbers hack's external symbols.
func (HackExternalScanner) ExternalScannerForLanguage(lang *gotreesitter.Language) gotreesitter.ExternalScanner {
	s := HackExternalScanner{symbols: hackDefaultSymTable}
	s.externalToToken = bindExternalScannerSpec(lang, hackExternalScannerSpec, func(tokenIdx int, sym gotreesitter.Symbol) {
		s.symbols[tokenIdx] = sym
	})
	return s
}

func (s HackExternalScanner) symbolTable() *[hackTokenCount]gotreesitter.Symbol {
	if s.symbols == ([hackTokenCount]gotreesitter.Symbol{}) {
		return &hackDefaultSymTable
	}
	return &s.symbols
}

func (HackExternalScanner) Create() any {
	return &hackState{}
}

func (HackExternalScanner) Destroy(payload any) {}

func (HackExternalScanner) Serialize(payload any, buf []byte) int {
	s := payload.(*hackState)
	if len(s.delimiter)+3 >= len(buf) {
		return 0
	}
	n := 0
	if s.isNowdoc {
		buf[n] = 1
	} else {
		buf[n] = 0
	}
	n++
	if s.didStart {
		buf[n] = 1
	} else {
		buf[n] = 0
	}
	n++
	if s.didEnd {
		buf[n] = 1
	} else {
		buf[n] = 0
	}
	n++
	copy(buf[n:], s.delimiter)
	n += len(s.delimiter)
	return n
}

func (HackExternalScanner) Deserialize(payload any, buf []byte) {
	s := payload.(*hackState)
	if len(buf) == 0 {
		s.isNowdoc = false
		s.didStart = false
		s.didEnd = false
		s.delimiter = s.delimiter[:0]
	} else {
		s.isNowdoc = buf[0] != 0
		s.didStart = buf[1] != 0
		s.didEnd = buf[2] != 0
		s.delimiter = append(s.delimiter[:0], buf[3:]...)
	}
}

func (sc HackExternalScanner) Scan(payload any, lexer *gotreesitter.ExternalLexer, validSymbols []bool) bool {
	s := payload.(*hackState)

	if len(sc.externalToToken) > 0 {
		var semanticValid [hackTokenCount]bool
		for externalIdx, valid := range validSymbols {
			if !valid || externalIdx >= len(sc.externalToToken) {
				continue
			}
			tokenIdx := sc.externalToToken[externalIdx]
			if tokenIdx >= 0 && tokenIdx < hackTokenCount {
				semanticValid[tokenIdx] = true
			}
		}
		validSymbols = semanticValid[:]
	}
	syms := sc.symbolTable()

	if (hackValid(validSymbols, hackTokHeredocBody) || hackValid(validSymbols, hackTokHeredocEnd) ||
		hackValid(validSymbols, hackTokEmbeddedOpenBrace)) && len(s.delimiter) > 0 {
		return hackScanBody(s, lexer, validSymbols, syms)
	}

	if hackValid(validSymbols, hackTokHeredocStart) {
		return hackScanStart(s, lexer, syms)
	}

	return false
}

func hackScanStart(s *hackState, lexer *gotreesitter.ExternalLexer, syms *[hackTokenCount]gotreesitter.Symbol) bool {
	for unicode.IsSpace(lexer.Lookahead()) {
		lexer.Advance(true)
	}

	s.isNowdoc = lexer.Lookahead() == '\''
	s.delimiter = s.delimiter[:0]

	var quote rune
	if s.isNowdoc || lexer.Lookahead() == '"' {
		quote = lexer.Lookahead()
		lexer.Advance(false)
	}

	if hackIsIdentStart(lexer.Lookahead()) {
		s.delimiter = append(s.delimiter, byte(lexer.Lookahead()))
		lexer.Advance(false)
		for hackIsIdentPart(lexer.Lookahead()) {
			s.delimiter = append(s.delimiter, byte(lexer.Lookahead()))
			lexer.Advance(false)
		}
	}

	if lexer.Lookahead() == quote {
		lexer.Advance(false)
	} else if quote != 0 {
		return false
	}

	if lexer.Lookahead() != '\n' || len(s.delimiter) == 0 {
		return false
	}

	lexer.SetResultSymbol(syms[hackTokHeredocStart])
	lexer.MarkEnd()
	lexer.Advance(false) // consume \n

	// Check if the delimiter immediately follows
	if hackMatchDelimiter(s, lexer) {
		if lexer.Lookahead() == ';' {
			lexer.Advance(false)
		}
		if lexer.Lookahead() == '\n' {
			s.didEnd = true
		}
	}

	return true
}

func hackScanBody(s *hackState, lexer *gotreesitter.ExternalLexer, validSymbols []bool, syms *[hackTokenCount]gotreesitter.Symbol) bool {
	didAdvance := false

	for {
		if lexer.Lookahead() == 0 {
			return false
		}

		// Handle escape sequences in heredocs
		if lexer.Lookahead() == '\\' {
			lexer.Advance(false)
			lexer.Advance(false)
			didAdvance = true
			continue
		}

		// Handle embedded variables/expressions in heredocs (not nowdocs)
		if (lexer.Lookahead() == '{' || lexer.Lookahead() == '$') && !s.isNowdoc {
			lexer.MarkEnd()

			if lexer.Lookahead() == '{' {
				lexer.Advance(false)
				if lexer.Lookahead() == '$' && !didAdvance {
					lexer.MarkEnd()
					lexer.Advance(false)
					if hackIsIdentStart(lexer.Lookahead()) {
						lexer.SetResultSymbol(syms[hackTokEmbeddedOpenBrace])
						return true
					}
				}
			}

			if lexer.Lookahead() == '$' {
				lexer.Advance(false)
				if hackIsIdentStart(lexer.Lookahead()) {
					lexer.SetResultSymbol(syms[hackTokHeredocBody])
					return didAdvance
				}
			}

			didAdvance = true
			continue
		}

		if s.didEnd || lexer.Lookahead() == '\n' {
			if didAdvance {
				lexer.MarkEnd()
				lexer.Advance(false)
			} else if lexer.Lookahead() == '\n' {
				if s.didEnd {
					lexer.Advance(true)
				} else {
					lexer.Advance(false)
					lexer.MarkEnd()
				}
			}

			if hackMatchDelimiter(s, lexer) {
				if !didAdvance && s.didEnd {
					lexer.MarkEnd()
				}

				if lexer.Lookahead() == ';' {
					lexer.Advance(false)
				}
				if lexer.Lookahead() == '\n' {
					if didAdvance {
						lexer.SetResultSymbol(syms[hackTokHeredocBody])
						s.didStart = true
						s.didEnd = true
					} else if s.didEnd {
						lexer.SetResultSymbol(syms[hackTokHeredocEnd])
						s.delimiter = s.delimiter[:0]
						s.isNowdoc = false
						s.didStart = false
						s.didEnd = false
					} else {
						lexer.SetResultSymbol(syms[hackTokHeredocEndNewline])
						s.didStart = true
						s.didEnd = true
					}
					return true
				}
			} else if !s.didStart && !didAdvance {
				s.didStart = true
				lexer.SetResultSymbol(syms[hackTokHeredocStartNewline])
				return true
			}

			didAdvance = true
			continue
		}

		lexer.Advance(false)
		didAdvance = true
	}
}

func hackMatchDelimiter(s *hackState, lexer *gotreesitter.ExternalLexer) bool {
	for i := 0; i < len(s.delimiter); i++ {
		if lexer.Lookahead() == rune(s.delimiter[i]) {
			lexer.Advance(false)
		} else {
			return false
		}
	}
	return true
}

func hackIsIdentStart(ch rune) bool {
	return ch == '_' || (ch >= 'a' && ch <= 'z') || (ch >= 'A' && ch <= 'Z') || (ch >= 128 && ch <= 255)
}

func hackIsIdentPart(ch rune) bool {
	return hackIsIdentStart(ch) || (ch >= '0' && ch <= '9')
}

func hackValid(vs []bool, i int) bool { return i < len(vs) && vs[i] }
