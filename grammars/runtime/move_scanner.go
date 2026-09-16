//go:build !grammar_subset || grammar_subset_move

package grammarruntime

import gotreesitter "github.com/odvcencio/gotreesitter"

// External token indexes for the move grammar
// (aptos-labs/tree-sitter-move-on-aptos src/scanner.c). The order must match
// the `externals` list in the grammar.
const (
	moveTokBlockDocCommentMarker = 0
	moveTokBlockCommentContent   = 1
	moveTokDocLineComment        = 2
	moveTokErrorSentinel         = 3
)

const (
	moveSymBlockDocCommentMarker gotreesitter.Symbol = 151
	moveSymBlockCommentContent   gotreesitter.Symbol = 152
	moveSymDocLineComment        gotreesitter.Symbol = 153
)

// MoveExternalScanner ports the stateless upstream scanner: block doc-comment
// markers (`/**` but not `/***` or `/**/`), nestable block comment content,
// and doc line comment bodies (`/// ...` up to and including EOL).
type MoveExternalScanner struct{}

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

func (MoveExternalScanner) Scan(payload any, lexer *gotreesitter.ExternalLexer, validSymbols []bool) bool {
	// Error recovery state: bail out, exactly like the C scanner.
	if moveValid(validSymbols, moveTokErrorSentinel) {
		return false
	}

	if moveValid(validSymbols, moveTokDocLineComment) {
		return moveScanLineDocContent(lexer)
	}

	matched := false
	if moveValid(validSymbols, moveTokBlockDocCommentMarker) {
		matched = moveScanBlockDocCommentMarker(lexer)
	}
	if !matched && moveValid(validSymbols, moveTokBlockCommentContent) {
		matched = moveScanBlockCommentContent(lexer)
	}
	return matched
}

// moveScanBlockDocCommentMarker matches the `*` of `/**` provided it is not
// followed by `/` (empty comment `/**/`) or another `*`.
func moveScanBlockDocCommentMarker(lexer *gotreesitter.ExternalLexer) bool {
	if lexer.Lookahead() != '*' {
		return false
	}
	lexer.Advance(false)
	lexer.MarkEnd()
	if lexer.Lookahead() == '/' || lexer.Lookahead() == '*' {
		return false
	}
	lexer.SetResultSymbol(moveSymBlockDocCommentMarker)
	return true
}

// moveScanBlockCommentContent munches nestable block comment content. The
// outermost closing `*/` is excluded (MarkEnd before consuming it) so
// tree-sitter can recognise it as its own token.
func moveScanBlockCommentContent(lexer *gotreesitter.ExternalLexer) bool {
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
	lexer.SetResultSymbol(moveSymBlockCommentContent)
	return true
}

// moveScanLineDocContent consumes a doc line comment body up to and including
// the EOL character (always matches).
func moveScanLineDocContent(lexer *gotreesitter.ExternalLexer) bool {
	lexer.SetResultSymbol(moveSymDocLineComment)
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
