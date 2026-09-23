//go:build !grammar_subset || grammar_subset_move

package grammarruntime

import gotreesitter "github.com/odvcencio/gotreesitter"

// External token indexes for the move grammar
// (aptos-labs/tree-sitter-move-on-aptos src/scanner.c). The order must match
// the `externals` list in the grammar. This is the external index (the
// position of the token in the grammar's `externals: [...]` list), which is
// exactly what tree-sitter's `valid_symbols` array and C's result_symbol
// enum are indexed by. The external index is stable across a blob regen as
// long as the externals list itself does not reorder; concrete numeric
// gotreesitter.Symbol IDs are NOT stable (they shift whenever the grammar's
// total symbol count changes), so this scanner never hardcodes them -- see
// moveDefaultSymTable below.
const (
	moveTokBlockDocCommentMarker = 0
	moveTokBlockCommentContent   = 1
	moveTokDocLineComment        = 2
	moveTokErrorSentinel         = 3
	moveTokenCount               = 4
)

// moveDefaultSymTable records the concrete gotreesitter.Symbol IDs the
// currently shipped move.bin assigns to each external, in moveTok* order.
// It exists only as a pre-bind fallback (and as an independent value to
// compare a real bind against in tests); ExternalScannerForLanguage below
// overwrites it with values read from the actual loaded Language at bind
// time, which is what the scanner must do to survive a future blob regen
// that renumbers absolute symbol IDs without touching the externals list
// order.
var moveDefaultSymTable = [moveTokenCount]gotreesitter.Symbol{
	151, // _block_doc_comment_marker
	152, // _block_comment_content
	153, // _doc_line_comment
	154, // _error_sentinel (never emitted; error-recovery bail-out only)
}

// moveExternalScannerSpec records the source contract for this hand-written
// port, so updater tooling can tell a grammar-only upstream change apart
// from one that also touches the external scanner or its token list. Its
// Externals list is also the binding source for ExternalScannerForLanguage:
// index i here is scanner token index i (moveTok* order).
var moveExternalScannerSpec = ExternalScannerSpec{
	Language:       "move",
	UpstreamRepo:   "https://github.com/aptos-labs/tree-sitter-move-on-aptos",
	UpstreamCommit: "12906b341de7cef81cf03d7d91dae51d8a9299e7",
	SourceFiles: []ExternalScannerSourceFile{
		{Path: "src/grammar.json", SHA256: "b7595445df5ea81393a798739b9e08ab2b3ee5b213435384f58308f130925f86"},
		{Path: "src/scanner.c", SHA256: "943112fa94a2b15a19ed6b5d83397a410e669bbea456a53305dba944224c082e"},
	},
	Externals: []string{
		"_block_doc_comment_marker",
		"_block_comment_content",
		"_doc_line_comment",
		"_error_sentinel",
	},
}

func init() {
	RegisterExternalScannerSpec(moveExternalScannerSpec)
}

// MoveExternalScanner ports the stateless upstream scanner: block doc-comment
// markers (`/**` but not `/***` or `/**/`), nestable block comment content,
// and doc line comment bodies (`/// ...` up to and including EOL).
//
// symbols holds the concrete gotreesitter.Symbol each external index maps to
// in the Language this instance was bound to (see ExternalScannerForLanguage).
// The scanner never hardcodes an absolute Symbol value: a blob regen can
// renumber the grammar's absolute symbol IDs without touching the externals
// list order, and a scanner that still called SetResultSymbol with a stale
// hardcoded ID would silently emit the wrong (but still structurally valid)
// node type instead of failing loudly.
type MoveExternalScanner struct {
	symbols         [moveTokenCount]gotreesitter.Symbol
	externalToToken []int
}

// ExternalScannerForLanguage binds the scanner's token slots to the loaded
// Language's ExternalSymbols positionally. A hardcoded absolute
// gotreesitter.Symbol constant here would emit the wrong token whenever a
// grammar bump renumbers move's external symbols.
func (MoveExternalScanner) ExternalScannerForLanguage(lang *gotreesitter.Language) gotreesitter.ExternalScanner {
	s := MoveExternalScanner{symbols: moveDefaultSymTable}
	s.externalToToken = bindExternalScannerSpec(lang, moveExternalScannerSpec, func(tokenIdx int, sym gotreesitter.Symbol) {
		s.symbols[tokenIdx] = sym
	})
	return s
}

func (s MoveExternalScanner) symbolTable() *[moveTokenCount]gotreesitter.Symbol {
	if s.symbols == ([moveTokenCount]gotreesitter.Symbol{}) {
		return &moveDefaultSymTable
	}
	return &s.symbols
}

func (MoveExternalScanner) Create() any                           { return nil }
func (MoveExternalScanner) Destroy(payload any)                   {}
func (MoveExternalScanner) Serialize(payload any, buf []byte) int { return 0 }
func (MoveExternalScanner) Deserialize(payload any, buf []byte)   {}

// The scanner carries no payload and derives every result from local
// lookahead plus validSymbols, so every incremental boundary is quiescent and
// failed scans cannot mutate persistent state.
func (MoveExternalScanner) SupportsIncrementalReuse() bool    { return true }
func (MoveExternalScanner) ExternalScannerIsStateless() bool  { return true }
func (MoveExternalScanner) PreservesStateOnScanFailure() bool { return true }

func (sc MoveExternalScanner) Scan(payload any, lexer *gotreesitter.ExternalLexer, validSymbols []bool) bool {
	syms := sc.symbolTable()

	// Error recovery state: bail out, exactly like the C scanner.
	if moveValid(validSymbols, moveTokErrorSentinel) {
		return false
	}

	if moveValid(validSymbols, moveTokDocLineComment) {
		return moveScanLineDocContent(lexer, syms)
	}

	matched := false
	if moveValid(validSymbols, moveTokBlockDocCommentMarker) {
		matched = moveScanBlockDocCommentMarker(lexer, syms)
	}
	if !matched && moveValid(validSymbols, moveTokBlockCommentContent) {
		matched = moveScanBlockCommentContent(lexer, syms)
	}
	return matched
}

// moveScanBlockDocCommentMarker matches the `*` of `/**` provided it is not
// followed by `/` (empty comment `/**/`) or another `*`.
func moveScanBlockDocCommentMarker(lexer *gotreesitter.ExternalLexer, syms *[moveTokenCount]gotreesitter.Symbol) bool {
	if lexer.Lookahead() != '*' {
		return false
	}
	lexer.Advance(false)
	lexer.MarkEnd()
	if lexer.Lookahead() == '/' || lexer.Lookahead() == '*' {
		return false
	}
	lexer.SetResultSymbol(syms[moveTokBlockDocCommentMarker])
	return true
}

// moveScanBlockCommentContent munches nestable block comment content. The
// outermost closing `*/` is excluded (MarkEnd before consuming it) so
// tree-sitter can recognise it as its own token.
func moveScanBlockCommentContent(lexer *gotreesitter.ExternalLexer, syms *[moveTokenCount]gotreesitter.Symbol) bool {
	depth := 1
	for lexer.Lookahead() != 0 && depth > 0 {
		switch lexer.Lookahead() {
		case '*':
			if depth == 1 {
				lexer.MarkEnd()
			}
			lexer.Advance(false)
			if lexer.Lookahead() == '/' {
				depth--
				lexer.Advance(false)
			}
		case '/':
			lexer.Advance(false)
			if lexer.Lookahead() == '*' {
				lexer.Advance(false)
				depth++
			}
		default:
			lexer.Advance(false)
		}
	}
	if depth > 0 {
		// Unterminated comment: everything scanned is content.
		lexer.MarkEnd()
		return false
	}
	lexer.SetResultSymbol(syms[moveTokBlockCommentContent])
	return true
}

// moveScanLineDocContent consumes a doc line comment body up to and including
// the EOL character (always matches).
func moveScanLineDocContent(lexer *gotreesitter.ExternalLexer, syms *[moveTokenCount]gotreesitter.Symbol) bool {
	lexer.SetResultSymbol(syms[moveTokDocLineComment])
	for lexer.Lookahead() != 0 {
		if moveIsEOL(lexer.Lookahead()) {
			lexer.Advance(false)
			break
		}
		lexer.Advance(false)
	}
	return true
}

// moveIsEOL reports end-of-line characters; EOF is not EOL.
func moveIsEOL(ch rune) bool { return ch == '\n' || ch == 0x2028 || ch == 0x2029 }

func moveValid(vs []bool, i int) bool { return i < len(vs) && vs[i] }
