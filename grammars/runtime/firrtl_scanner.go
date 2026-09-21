//go:build !grammar_subset || grammar_subset_firrtl

package grammarruntime

import gotreesitter "github.com/odvcencio/gotreesitter"

// External token indexes for the firrtl grammar. This is the external
// index (the position of the token in the grammar's `externals: [...]`
// list), which is exactly what tree-sitter's `valid_symbols` array and
// C's result_symbol enum are indexed by. The external index is stable
// across a blob regen as long as the externals list itself does not
// reorder; concrete numeric gotreesitter.Symbol IDs are NOT stable (they
// shift whenever the grammar's total symbol count changes), so this
// scanner never hardcodes them -- see firrtlDefaultSymTable below.
const (
	firrtlTokNewline = 0
	firrtlTokIndent  = 1
	firrtlTokDedent  = 2
	firrtlTokenCount = 3
)

// firrtlDefaultSymTable records the concrete gotreesitter.Symbol IDs the
// currently shipped firrtl.bin assigns to each external, in firrtlTok*
// order. It exists only as a pre-bind fallback (and as an independent
// value to compare a real bind against in tests); ExternalScannerForLanguage
// below overwrites it with values read from the actual loaded Language at
// bind time, which is what the scanner must do to survive a future blob
// regen that renumbers absolute symbol IDs without touching the externals
// list order.
var firrtlDefaultSymTable = [firrtlTokenCount]gotreesitter.Symbol{
	126, // _newline
	127, // _indent
	128, // _dedent
}

// firrtlExternalScannerSpec records the source contract for this
// hand-written port, so updater tooling can tell a grammar-only upstream
// change apart from one that also touches the external scanner or its
// token list. Its Externals list is also the binding source for
// ExternalScannerForLanguage: index i here is scanner token index i
// (firrtlTok* order).
var firrtlExternalScannerSpec = ExternalScannerSpec{
	Language:       "firrtl",
	UpstreamRepo:   "https://github.com/amaanq/tree-sitter-firrtl",
	UpstreamCommit: "8503d3a0fe0f9e427863cb0055699ff2d29ae5f5",
	SourceFiles: []ExternalScannerSourceFile{
		{Path: "src/grammar.json", SHA256: "e7985f016cab5f7f77be23711d99a243d3519cc1c3f6aeba1c0196042b559823"},
		{Path: "src/scanner.c", SHA256: "e42e5ea30b8ff7533f5f4cfaf92c0d309a4ea1b546421d504f99b993b1f759a6"},
	},
	Externals: []string{
		"_newline",
		"_indent",
		"_dedent",
	},
}

func init() {
	RegisterExternalScannerSpec(firrtlExternalScannerSpec)
}

// firrtlState tracks indent stack for FIRRTL parsing.
type firrtlState struct {
	indents []uint16
}

// FirrtlExternalScanner handles newline/indent/dedent for FIRRTL.
//
// symbols holds the concrete gotreesitter.Symbol each external index maps to
// in the Language this instance was bound to (see ExternalScannerForLanguage).
// The scanner never hardcodes an absolute Symbol value: a blob regen can
// renumber the grammar's absolute symbol IDs without touching the externals
// list order, and a scanner that still called SetResultSymbol with a stale
// hardcoded ID would silently emit the wrong (but still structurally valid)
// node type instead of failing loudly.
type FirrtlExternalScanner struct {
	symbols         [firrtlTokenCount]gotreesitter.Symbol
	externalToToken []int
}

// ExternalScannerForLanguage binds the scanner's token slots to the loaded
// Language's ExternalSymbols positionally. A hardcoded absolute
// gotreesitter.Symbol constant here would emit the wrong token whenever a
// grammar bump renumbers firrtl's external symbols.
func (FirrtlExternalScanner) ExternalScannerForLanguage(lang *gotreesitter.Language) gotreesitter.ExternalScanner {
	s := FirrtlExternalScanner{symbols: firrtlDefaultSymTable}
	s.externalToToken = bindExternalScannerSpec(lang, firrtlExternalScannerSpec, func(tokenIdx int, sym gotreesitter.Symbol) {
		s.symbols[tokenIdx] = sym
	})
	return s
}

func (s FirrtlExternalScanner) symbolTable() *[firrtlTokenCount]gotreesitter.Symbol {
	if s.symbols == ([firrtlTokenCount]gotreesitter.Symbol{}) {
		return &firrtlDefaultSymTable
	}
	return &s.symbols
}

func (FirrtlExternalScanner) Create() any {
	return &firrtlState{indents: []uint16{0}}
}

func (FirrtlExternalScanner) Destroy(payload any) {}

func (FirrtlExternalScanner) Serialize(payload any, buf []byte) int {
	s := payload.(*firrtlState)
	n := 0
	// Skip index 0 (always 0), serialize rest as bytes
	for i := 1; i < len(s.indents) && n < len(buf); i++ {
		buf[n] = byte(s.indents[i])
		n++
	}
	return n
}

func (FirrtlExternalScanner) Deserialize(payload any, buf []byte) {
	s := payload.(*firrtlState)
	s.indents = s.indents[:0]
	s.indents = append(s.indents, 0)
	for i := 0; i < len(buf); i++ {
		s.indents = append(s.indents, uint16(buf[i]))
	}
}

func (sc FirrtlExternalScanner) Scan(payload any, lexer *gotreesitter.ExternalLexer, validSymbols []bool) bool {
	s := payload.(*firrtlState)

	if len(sc.externalToToken) > 0 {
		var semanticValid [firrtlTokenCount]bool
		for externalIdx, valid := range validSymbols {
			if !valid || externalIdx >= len(sc.externalToToken) {
				continue
			}
			tokenIdx := sc.externalToToken[externalIdx]
			if tokenIdx >= 0 && tokenIdx < firrtlTokenCount {
				semanticValid[tokenIdx] = true
			}
		}
		validSymbols = semanticValid[:]
	}
	syms := sc.symbolTable()

	lexer.MarkEnd()

	foundEol := false
	indentLen := uint32(0)

	for {
		ch := lexer.Lookahead()
		switch {
		case ch == '\n':
			foundEol = true
			indentLen = 0
			lexer.Advance(true)
		case ch == ' ':
			indentLen++
			lexer.Advance(true)
		case ch == '\r' || ch == '\f':
			indentLen = 0
			lexer.Advance(true)
		case ch == '\t':
			indentLen += 8
			lexer.Advance(true)
		case ch == '#':
			// Skip comment
			for lexer.Lookahead() != 0 && lexer.Lookahead() != '\n' {
				lexer.Advance(true)
			}
			lexer.Advance(true)
			indentLen = 0
		case ch == '\\':
			lexer.Advance(true)
			if lexer.Lookahead() == '\r' {
				lexer.Advance(true)
			}
			if lexer.Lookahead() == '\n' || lexer.Lookahead() == 0 {
				lexer.Advance(true)
			} else {
				return false
			}
		case ch == 0:
			indentLen = 0
			foundEol = true
			goto done
		default:
			goto done
		}
	}

done:
	if foundEol {
		if len(s.indents) > 0 {
			currentIndent := s.indents[len(s.indents)-1]

			if firrtlValid(validSymbols, firrtlTokIndent) && indentLen > uint32(currentIndent) {
				s.indents = append(s.indents, uint16(indentLen))
				lexer.SetResultSymbol(syms[firrtlTokIndent])
				return true
			}

			if (firrtlValid(validSymbols, firrtlTokDedent) || !firrtlValid(validSymbols, firrtlTokNewline)) &&
				indentLen < uint32(currentIndent) {
				s.indents = s.indents[:len(s.indents)-1]
				lexer.SetResultSymbol(syms[firrtlTokDedent])
				return true
			}
		}

		if firrtlValid(validSymbols, firrtlTokNewline) {
			lexer.SetResultSymbol(syms[firrtlTokNewline])
			return true
		}
	}

	return false
}

func firrtlValid(vs []bool, i int) bool { return i < len(vs) && vs[i] }
