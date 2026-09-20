//go:build !grammar_subset || grammar_subset_agda

package grammarruntime

import gotreesitter "github.com/odvcencio/gotreesitter"

// External token indexes for the agda grammar. This is the external index
// (the position of the token in the grammar's `externals: [...]` list),
// which is exactly what tree-sitter's `valid_symbols` array and C's
// result_symbol enum are indexed by. The external index is stable across a
// blob regen as long as the externals list itself does not reorder;
// concrete numeric gotreesitter.Symbol IDs are NOT stable (they shift
// whenever the grammar's total symbol count changes), so this scanner never
// hardcodes them -- see agdaDefaultSymTable below.
const (
	agdaTokNewline = 0
	agdaTokIndent  = 1
	agdaTokDedent  = 2
	agdaTokenCount = 3
)

// agdaDefaultSymTable records the concrete gotreesitter.Symbol IDs the
// currently shipped agda.bin assigns to each external, in agdaTok* order.
// It exists only as a pre-bind fallback (and as an independent value to
// compare a real bind against in tests); ExternalScannerForLanguage below
// overwrites it with values read from the actual loaded Language at bind
// time, which is what the scanner must do to survive a future blob regen
// that renumbers absolute symbol IDs without touching the externals list
// order.
var agdaDefaultSymTable = [agdaTokenCount]gotreesitter.Symbol{
	87, // _newline
	88, // _indent
	89, // _dedent
}

// agdaExternalScannerSpec records the source contract for this
// hand-written port, so updater tooling can tell a grammar-only upstream
// change apart from one that also touches the external scanner or its
// token list. Its Externals list is also the binding source for
// ExternalScannerForLanguage: index i here is scanner token index i
// (agdaTok* order).
var agdaExternalScannerSpec = ExternalScannerSpec{
	Language:       "agda",
	UpstreamRepo:   "https://github.com/tree-sitter/tree-sitter-agda",
	UpstreamCommit: "e8d47a6987effe34d5595baf321d82d3519a8527",
	SourceFiles: []ExternalScannerSourceFile{
		{Path: "src/grammar.json", SHA256: "72933f8f5a8039b852b1f22e818514012cb435a454d24dffb0f80bcc62d643ec"},
		{Path: "src/scanner.c", SHA256: "91c9db32be80f74824ee785ba941ecd000f6450fd527098181e358ddb193ca62"},
	},
	Externals: []string{
		"_newline",
		"_indent",
		"_dedent",
	},
}

func init() {
	RegisterExternalScannerSpec(agdaExternalScannerSpec)
}

// agdaState tracks indent stack for Agda parsing.
type agdaState struct {
	indents []uint16
}

// AgdaExternalScanner handles newline/indent/dedent for Agda.
//
// symbols holds the concrete gotreesitter.Symbol each external index maps to
// in the Language this instance was bound to (see ExternalScannerForLanguage).
// The scanner never hardcodes an absolute Symbol value: a blob regen can
// renumber the grammar's absolute symbol IDs without touching the externals
// list order, and a scanner that still called SetResultSymbol with a stale
// hardcoded ID would silently emit the wrong (but still structurally valid)
// node type instead of failing loudly.
type AgdaExternalScanner struct {
	symbols         [agdaTokenCount]gotreesitter.Symbol
	externalToToken []int
}

// ExternalScannerForLanguage binds the scanner's token slots to the loaded
// Language's ExternalSymbols positionally. A hardcoded absolute
// gotreesitter.Symbol constant here would emit the wrong token whenever a
// grammar bump renumbers agda's external symbols.
func (AgdaExternalScanner) ExternalScannerForLanguage(lang *gotreesitter.Language) gotreesitter.ExternalScanner {
	s := AgdaExternalScanner{symbols: agdaDefaultSymTable}
	s.externalToToken = bindExternalScannerSpec(lang, agdaExternalScannerSpec, func(tokenIdx int, sym gotreesitter.Symbol) {
		s.symbols[tokenIdx] = sym
	})
	return s
}

func (s AgdaExternalScanner) symbolTable() *[agdaTokenCount]gotreesitter.Symbol {
	if s.symbols == ([agdaTokenCount]gotreesitter.Symbol{}) {
		return &agdaDefaultSymTable
	}
	return &s.symbols
}

func (AgdaExternalScanner) Create() any {
	return &agdaState{indents: []uint16{0}}
}

func (AgdaExternalScanner) Destroy(payload any) {}

func (AgdaExternalScanner) Serialize(payload any, buf []byte) int {
	s := payload.(*agdaState)
	n := 0
	for i := 1; i < len(s.indents) && n < len(buf); i++ {
		buf[n] = byte(s.indents[i])
		n++
	}
	return n
}

func (AgdaExternalScanner) Deserialize(payload any, buf []byte) {
	s := payload.(*agdaState)
	s.indents = s.indents[:0]
	s.indents = append(s.indents, 0)
	for i := 0; i < len(buf); i++ {
		s.indents = append(s.indents, uint16(buf[i]))
	}
}

func (sc AgdaExternalScanner) Scan(payload any, lexer *gotreesitter.ExternalLexer, validSymbols []bool) bool {
	s := payload.(*agdaState)

	if len(sc.externalToToken) > 0 {
		var semanticValid [agdaTokenCount]bool
		for externalIdx, valid := range validSymbols {
			if !valid || externalIdx >= len(sc.externalToToken) {
				continue
			}
			tokenIdx := sc.externalToToken[externalIdx]
			if tokenIdx >= 0 && tokenIdx < agdaTokenCount {
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
		case ch == '-':
			// Skip -- line comments
			lexer.Advance(true)
			if lexer.Lookahead() == '-' {
				for lexer.Lookahead() != 0 && lexer.Lookahead() != '\n' {
					lexer.Advance(true)
				}
				indentLen = 0
			} else {
				// Not a comment, put us back
				if foundEol {
					break
				}
				return false
			}
		case ch == '{':
			// Skip {- -} block comments
			lexer.Advance(true)
			if lexer.Lookahead() == '-' {
				lexer.Advance(true)
				depth := 1
				for depth > 0 && lexer.Lookahead() != 0 {
					if lexer.Lookahead() == '{' {
						lexer.Advance(true)
						if lexer.Lookahead() == '-' {
							depth++
							lexer.Advance(true)
						}
					} else if lexer.Lookahead() == '-' {
						lexer.Advance(true)
						if lexer.Lookahead() == '}' {
							depth--
							lexer.Advance(true)
						}
					} else {
						lexer.Advance(true)
					}
				}
				indentLen = 0
			} else {
				if foundEol {
					break
				}
				return false
			}
		case ch == 0:
			indentLen = 0
			foundEol = true
			goto done
		default:
			goto done
		}
		continue
	done:
		break
	}

	if foundEol {
		if len(s.indents) > 0 {
			currentIndent := s.indents[len(s.indents)-1]

			if agdaValid(validSymbols, agdaTokIndent) && indentLen > uint32(currentIndent) {
				s.indents = append(s.indents, uint16(indentLen))
				lexer.SetResultSymbol(syms[agdaTokIndent])
				return true
			}

			if (agdaValid(validSymbols, agdaTokDedent) || !agdaValid(validSymbols, agdaTokNewline)) &&
				indentLen < uint32(currentIndent) {
				s.indents = s.indents[:len(s.indents)-1]
				lexer.SetResultSymbol(syms[agdaTokDedent])
				return true
			}
		}

		if agdaValid(validSymbols, agdaTokNewline) {
			lexer.SetResultSymbol(syms[agdaTokNewline])
			return true
		}
	}

	return false
}

func agdaValid(vs []bool, i int) bool { return i < len(vs) && vs[i] }
