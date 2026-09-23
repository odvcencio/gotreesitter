//go:build !grammar_subset || grammar_subset_pug

package grammarruntime

import gotreesitter "github.com/odvcencio/gotreesitter"

// External token indexes for the pug grammar. This is the external
// index (the position of the token in the grammar's `externals: [...]`
// list), which is exactly what tree-sitter's `valid_symbols` array and
// C's result_symbol enum are indexed by. The external index is stable
// across a blob regen as long as the externals list itself does not
// reorder; concrete numeric gotreesitter.Symbol IDs are NOT stable (they
// shift whenever the grammar's total symbol count changes), so this
// scanner never hardcodes them -- see pugDefaultSymTable below.
const (
	pugTokNewline = 0
	pugTokIndent  = 1
	pugTokDedent  = 2
	pugTokenCount = 3
)

// pugDefaultSymTable records the concrete gotreesitter.Symbol IDs the
// currently shipped pug.bin assigns to each external, in pugTok* order.
// It exists only as a pre-bind fallback (and as an independent value to
// compare a real bind against in tests); ExternalScannerForLanguage below
// overwrites it with values read from the actual loaded Language at bind
// time, which is what the scanner must do to survive a future blob regen
// that renumbers absolute symbol IDs without touching the externals list
// order.
var pugDefaultSymTable = [pugTokenCount]gotreesitter.Symbol{
	78, // _newline
	79, // _indent
	80, // _dedent
}

// pugExternalScannerSpec records the source contract for this
// hand-written port, so updater tooling can tell a grammar-only upstream
// change apart from one that also touches the external scanner or its
// token list. Its Externals list is also the binding source for
// ExternalScannerForLanguage: index i here is scanner token index i
// (pugTok* order).
var pugExternalScannerSpec = ExternalScannerSpec{
	Language:       "pug",
	UpstreamRepo:   "https://github.com/zealot128/tree-sitter-pug",
	UpstreamCommit: "13e9195370172c86a8b88184cc358b23b677cc46",
	SourceFiles: []ExternalScannerSourceFile{
		{Path: "src/grammar.json", SHA256: "a9659eb4b4897de58691cb0da1b758cdd48408e23d9352aa712f295676ca1b57"},
		{Path: "src/scanner.c", SHA256: "88f11aa66335973f7fc8ac293048b72fcc893d752acc18d7a1145ab422af9716"},
	},
	Externals: []string{
		"_newline",
		"_indent",
		"_dedent",
	},
}

func init() {
	RegisterExternalScannerSpec(pugExternalScannerSpec)
}

// pugState tracks indent stack for Pug parsing.
type pugState struct {
	indents []uint16
}

// PugExternalScanner handles newline/indent/dedent for Pug templates.
//
// symbols holds the concrete gotreesitter.Symbol each external index maps to
// in the Language this instance was bound to (see ExternalScannerForLanguage).
// The scanner never hardcodes an absolute Symbol value: a blob regen can
// renumber the grammar's absolute symbol IDs without touching the externals
// list order, and a scanner that still called SetResultSymbol with a stale
// hardcoded ID would silently emit the wrong (but still structurally valid)
// node type instead of failing loudly.
type PugExternalScanner struct {
	symbols         [pugTokenCount]gotreesitter.Symbol
	externalToToken []int
}

// ExternalScannerForLanguage binds the scanner's token slots to the loaded
// Language's ExternalSymbols positionally. A hardcoded absolute
// gotreesitter.Symbol constant here would emit the wrong token whenever a
// grammar bump renumbers pug's external symbols.
func (PugExternalScanner) ExternalScannerForLanguage(lang *gotreesitter.Language) gotreesitter.ExternalScanner {
	s := PugExternalScanner{symbols: pugDefaultSymTable}
	s.externalToToken = bindExternalScannerSpec(lang, pugExternalScannerSpec, func(tokenIdx int, sym gotreesitter.Symbol) {
		s.symbols[tokenIdx] = sym
	})
	return s
}

func (s PugExternalScanner) symbolTable() *[pugTokenCount]gotreesitter.Symbol {
	if s.symbols == ([pugTokenCount]gotreesitter.Symbol{}) {
		return &pugDefaultSymTable
	}
	return &s.symbols
}

func (PugExternalScanner) Create() any {
	return &pugState{indents: []uint16{0}}
}

func (PugExternalScanner) Destroy(payload any) {}

func (PugExternalScanner) Serialize(payload any, buf []byte) int {
	s := payload.(*pugState)
	n := 0
	for i := 1; i < len(s.indents) && n < len(buf); i++ {
		buf[n] = byte(s.indents[i])
		n++
	}
	return n
}

func (PugExternalScanner) Deserialize(payload any, buf []byte) {
	s := payload.(*pugState)
	s.indents = s.indents[:0]
	s.indents = append(s.indents, 0)
	for i := 0; i < len(buf); i++ {
		s.indents = append(s.indents, uint16(buf[i]))
	}
}

func (sc PugExternalScanner) Scan(payload any, lexer *gotreesitter.ExternalLexer, validSymbols []bool) bool {
	if len(sc.externalToToken) > 0 {
		var semanticValid [pugTokenCount]bool
		for externalIdx, valid := range validSymbols {
			if !valid || externalIdx >= len(sc.externalToToken) {
				continue
			}
			tokenIdx := sc.externalToToken[externalIdx]
			if tokenIdx >= 0 && tokenIdx < pugTokenCount {
				semanticValid[tokenIdx] = true
			}
		}
		validSymbols = semanticValid[:]
	}
	syms := sc.symbolTable()

	s := payload.(*pugState)
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
			indentLen += 2
			lexer.Advance(true)
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

			if pugValid(validSymbols, pugTokIndent) && indentLen > uint32(currentIndent) {
				s.indents = append(s.indents, uint16(indentLen))
				lexer.MarkEnd()
				lexer.SetResultSymbol(syms[pugTokIndent])
				return true
			}

			if (pugValid(validSymbols, pugTokDedent) || !pugValid(validSymbols, pugTokNewline)) &&
				indentLen < uint32(currentIndent) {
				s.indents = s.indents[:len(s.indents)-1]
				lexer.MarkEnd()
				lexer.SetResultSymbol(syms[pugTokDedent])
				return true
			}
		}

		if pugValid(validSymbols, pugTokNewline) {
			lexer.MarkEnd()
			lexer.SetResultSymbol(syms[pugTokNewline])
			return true
		}
	}

	return false
}

func pugValid(vs []bool, i int) bool { return i < len(vs) && vs[i] }
