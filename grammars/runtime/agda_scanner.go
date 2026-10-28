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

// agdaState mirrors the C scanner's payload: the indent stack, the count of
// dedents still owed, and a queue of tokens to emit on the next scans. The
// queue is payload only, as in C: Serialize does not write it and Deserialize
// does not clear it.
type agdaState struct {
	indents      []uint16
	queuedDedent uint8
	tokens       []int
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

// Serialize writes the owed dedent count, then every indent as a
// little-endian uint16, including the base indent of zero. This matches the
// C scanner's buffer.
func (AgdaExternalScanner) Serialize(payload any, buf []byte) int {
	s := payload.(*agdaState)
	if len(s.indents)*2+1 > len(buf) {
		return 0
	}
	buf[0] = s.queuedDedent
	n := 1
	for _, indent := range s.indents {
		buf[n] = byte(indent)
		buf[n+1] = byte(indent >> 8)
		n += 2
	}
	return n
}

// Deserialize restores the owed dedent count and the indent stack. An empty
// buffer is the initial state, an indent stack of one zero.
func (AgdaExternalScanner) Deserialize(payload any, buf []byte) {
	s := payload.(*agdaState)
	s.queuedDedent = 0
	s.indents = s.indents[:0]
	if len(buf) == 0 {
		s.indents = append(s.indents, 0)
		return
	}
	s.queuedDedent = buf[0]
	for i := 1; i+1 < len(buf); i += 2 {
		s.indents = append(s.indents, uint16(buf[i])|uint16(buf[i+1])<<8)
	}
	if len(s.indents) == 0 {
		s.indents = append(s.indents, 0)
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

	if len(s.tokens) == 0 {
		if agdaValid(validSymbols, agdaTokDedent) && s.queuedDedent > 0 {
			s.queuedDedent--
			s.tokens = append(s.tokens, agdaTokDedent, agdaTokNewline)
		} else {
			skippedNewline := false
			for ch := lexer.Lookahead(); ch == ' ' || ch == '\t' || ch == '\r' || ch == '\n'; ch = lexer.Lookahead() {
				if ch == '\n' {
					skippedNewline = true
				}
				lexer.Advance(true)
			}

			if lexer.Lookahead() == 0 {
				if agdaValid(validSymbols, agdaTokDedent) && len(s.indents) > 1 {
					s.indents = s.indents[:len(s.indents)-1]
					s.tokens = append(s.tokens, agdaTokDedent, agdaTokNewline)
				} else if agdaValid(validSymbols, agdaTokNewline) {
					s.tokens = append(s.tokens, agdaTokNewline)
				}
			} else {
				indentLength := uint16(lexer.Column())
				top := s.indents[len(s.indents)-1]
				indent := indentLength > top
				dedent := indentLength < top

				if skippedNewline {
					if indent {
						if agdaValid(validSymbols, agdaTokIndent) {
							s.indents = append(s.indents, indentLength)
							s.tokens = append(s.tokens, agdaTokIndent)
						}
					} else if agdaValid(validSymbols, agdaTokNewline) {
						s.tokens = append(s.tokens, agdaTokNewline)
					}
				} else {
					if indent {
						if agdaValid(validSymbols, agdaTokIndent) {
							s.indents = append(s.indents, indentLength)
							s.tokens = append(s.tokens, agdaTokIndent)
						}
					} else if dedent {
						s.indents = s.indents[:len(s.indents)-1]
						for indentLength < s.indents[len(s.indents)-1] {
							s.indents = s.indents[:len(s.indents)-1]
							s.queuedDedent++
						}
						if agdaValid(validSymbols, agdaTokDedent) {
							s.tokens = append(s.tokens, agdaTokDedent, agdaTokNewline)
						} else {
							s.queuedDedent++
						}
					}
				}
			}
		}
	}

	if len(s.tokens) == 0 {
		return false
	}
	lexer.SetResultSymbol(syms[s.tokens[0]])
	s.tokens = s.tokens[1:]
	return true
}

func agdaValid(vs []bool, i int) bool { return i < len(vs) && vs[i] }
