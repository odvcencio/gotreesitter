//go:build !grammar_subset || grammar_subset_kdl

package grammarruntime

import gotreesitter "github.com/odvcencio/gotreesitter"

// External token indexes for the kdl grammar. This is the external
// index (the position of the token in the grammar's `externals: [...]`
// list), which is exactly what tree-sitter's `valid_symbols` array and
// C's result_symbol enum are indexed by. The external index is stable
// across a blob regen as long as the externals list itself does not
// reorder; concrete numeric gotreesitter.Symbol IDs are NOT stable (they
// shift whenever the grammar's total symbol count changes), so this
// scanner never hardcodes them -- see kdlDefaultSymTable below.
const (
	kdlTokEof              = 0
	kdlTokMultiLineComment = 1
	kdlTokRawString        = 2
	kdlTokenCount          = 3
)

// kdlDefaultSymTable records the concrete gotreesitter.Symbol IDs the
// currently shipped kdl.bin assigns to each external, in kdlTok* order.
// It exists only as a pre-bind fallback (and as an independent value to
// compare a real bind against in tests); ExternalScannerForLanguage below
// overwrites it with values read from the actual loaded Language at bind
// time, which is what the scanner must do to survive a future blob regen
// that renumbers absolute symbol IDs without touching the externals list
// order.
var kdlDefaultSymTable = [kdlTokenCount]gotreesitter.Symbol{
	81, // _eof
	82, // multi_line_comment
	83, // _raw_string
}

// kdlExternalScannerSpec records the source contract for this
// hand-written port, so updater tooling can tell a grammar-only upstream
// change apart from one that also touches the external scanner or its
// token list. Its Externals list is also the binding source for
// ExternalScannerForLanguage: index i here is scanner token index i
// (kdlTok* order).
var kdlExternalScannerSpec = ExternalScannerSpec{
	Language:       "kdl",
	UpstreamRepo:   "https://github.com/tree-sitter-grammars/tree-sitter-kdl",
	UpstreamCommit: "b37e3d58e5c5cf8d739b315d6114e02d42e66664",
	SourceFiles: []ExternalScannerSourceFile{
		{Path: "src/grammar.json", SHA256: "645a5f013c9b8b937aca22a793db9f3c5c5a547e41b54cb36cfde495e42e806c"},
		{Path: "src/scanner.c", SHA256: "a9b8515732df9ec83f8797db58523fccda77b5a40ec119f4448ce651e6753ee4"},
	},
	Externals: []string{
		"_eof",
		"multi_line_comment",
		"_raw_string",
	},
}

func init() {
	RegisterExternalScannerSpec(kdlExternalScannerSpec)
}

// KdlExternalScanner handles EOF detection, nestable /* */ comments,
// and r#"..."# raw strings for KDL.
//
// symbols holds the concrete gotreesitter.Symbol each external index maps to
// in the Language this instance was bound to (see ExternalScannerForLanguage).
// The scanner never hardcodes an absolute Symbol value: a blob regen can
// renumber the grammar's absolute symbol IDs without touching the externals
// list order, and a scanner that still called SetResultSymbol with a stale
// hardcoded ID would silently emit the wrong (but still structurally valid)
// node type instead of failing loudly.
type KdlExternalScanner struct {
	symbols         [kdlTokenCount]gotreesitter.Symbol
	externalToToken []int
}

// ExternalScannerForLanguage binds the scanner's token slots to the loaded
// Language's ExternalSymbols positionally. A hardcoded absolute
// gotreesitter.Symbol constant here would emit the wrong token whenever a
// grammar bump renumbers kdl's external symbols.
func (KdlExternalScanner) ExternalScannerForLanguage(lang *gotreesitter.Language) gotreesitter.ExternalScanner {
	s := KdlExternalScanner{symbols: kdlDefaultSymTable}
	s.externalToToken = bindExternalScannerSpec(lang, kdlExternalScannerSpec, func(tokenIdx int, sym gotreesitter.Symbol) {
		s.symbols[tokenIdx] = sym
	})
	return s
}

func (s KdlExternalScanner) symbolTable() *[kdlTokenCount]gotreesitter.Symbol {
	if s.symbols == ([kdlTokenCount]gotreesitter.Symbol{}) {
		return &kdlDefaultSymTable
	}
	return &s.symbols
}

func (KdlExternalScanner) Create() any                           { return nil }
func (KdlExternalScanner) Destroy(payload any)                   {}
func (KdlExternalScanner) Serialize(payload any, buf []byte) int { return 0 }
func (KdlExternalScanner) Deserialize(payload any, buf []byte)   {}

// Nesting depth and raw-string delimiters are local to one Scan call. The
// scanner retains no state between tokens or after a failed scan.
func (KdlExternalScanner) SupportsIncrementalReuse() bool    { return true }
func (KdlExternalScanner) ExternalScannerIsStateless() bool  { return true }
func (KdlExternalScanner) PreservesStateOnScanFailure() bool { return true }

func (sc KdlExternalScanner) Scan(payload any, lexer *gotreesitter.ExternalLexer, validSymbols []bool) bool {
	syms := sc.symbolTable()

	// EOF detection
	if kdlValid(validSymbols, kdlTokEof) && lexer.Lookahead() == 0 {
		lexer.Advance(false)
		lexer.SetResultSymbol(syms[kdlTokEof])
		return true
	}

	// Raw string: r#"..."#
	if kdlValid(validSymbols, kdlTokRawString) && lexer.Lookahead() == 'r' {
		lexer.Advance(false)
		numHashes := uint32(0)
		for lexer.Lookahead() == '#' {
			numHashes++
			lexer.Advance(false)
		}
		if lexer.Lookahead() != '"' {
			return false
		}
		lexer.Advance(false)

		for {
			if lexer.Lookahead() == 0 {
				return false
			}
			ch := lexer.Lookahead()
			lexer.Advance(false)
			if ch != '"' {
				continue
			}
			// Try to match closing hashes
			closingHashes := uint32(0)
			for closingHashes < numHashes && lexer.Lookahead() == '#' {
				closingHashes++
				lexer.Advance(false)
			}
			if closingHashes == numHashes {
				lexer.MarkEnd()
				lexer.SetResultSymbol(syms[kdlTokRawString])
				return true
			}
		}
	}

	// Nestable multi-line comment: /* ... */
	if lexer.Lookahead() == '/' {
		lexer.Advance(false)
		if lexer.Lookahead() != '*' {
			return false
		}
		lexer.Advance(false)

		afterStar := false
		depth := uint32(1)
		for depth > 0 {
			ch := lexer.Lookahead()
			switch {
			case ch == 0:
				return false
			case ch == '*':
				lexer.Advance(false)
				afterStar = true
			case ch == '/':
				if afterStar {
					lexer.Advance(false)
					afterStar = false
					depth--
				} else {
					lexer.Advance(false)
					afterStar = false
					if lexer.Lookahead() == '*' {
						depth++
						lexer.Advance(false)
					}
				}
			default:
				lexer.Advance(false)
				afterStar = false
			}
		}
		lexer.MarkEnd()
		lexer.SetResultSymbol(syms[kdlTokMultiLineComment])
		return true
	}

	return false
}

func kdlValid(vs []bool, i int) bool { return i < len(vs) && vs[i] }
