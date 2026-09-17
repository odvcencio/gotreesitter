//go:build !grammar_subset || grammar_subset_gleam

package grammarruntime

import gotreesitter "github.com/odvcencio/gotreesitter"

// External token indexes for the gleam grammar.
const (
	gleamTokQuotedContent     = 0 // "quoted_content" — string literal interior
	gleamTokDocCommentContent = 1 // "doc_comment_content" — doc comment line
)

// Concrete symbol IDs from the generated gleam grammar ExternalSymbols.
const (
	gleamSymQuotedContent     gotreesitter.Symbol = 97
	gleamSymDocCommentContent gotreesitter.Symbol = 98
)

// GleamExternalScanner implements gotreesitter.ExternalScanner for tree-sitter-gleam.
//
// The gleam grammar uses an external scanner to produce two tokens:
//   - quoted_content: the interior of a string literal, consuming characters
//     until a closing " or escape \ is encountered.
//   - doc_comment_content: a single line of a doc comment, consuming
//     characters until end-of-line or EOF.
type GleamExternalScanner struct{}

func (GleamExternalScanner) Create() any                           { return nil }
func (GleamExternalScanner) Destroy(payload any)                   {}
func (GleamExternalScanner) Serialize(payload any, buf []byte) int { return 0 }
func (GleamExternalScanner) Deserialize(payload any, buf []byte)   {}

// The scanner carries no payload and derives every result from local
// lookahead plus validSymbols, so every incremental boundary is quiescent and
// failed scans cannot mutate persistent state.
func (GleamExternalScanner) SupportsIncrementalReuse() bool    { return true }
func (GleamExternalScanner) ExternalScannerIsStateless() bool  { return true }
func (GleamExternalScanner) PreservesStateOnScanFailure() bool { return true }

func (GleamExternalScanner) Scan(payload any, lexer *gotreesitter.ExternalLexer, validSymbols []bool) bool {
	if gleamValid(validSymbols, gleamTokQuotedContent) {
		return scanGleamQuotedContent(lexer)
	}
	if gleamValid(validSymbols, gleamTokDocCommentContent) {
		return scanGleamDocCommentContent(lexer)
	}
	return false
}

// scanGleamQuotedContent consumes string content until " or \ is hit.
// Returns true only if at least one character was consumed.
func scanGleamQuotedContent(lexer *gotreesitter.ExternalLexer) bool {
	hasContent := false
	for {
		ch := lexer.Lookahead()
		if ch == '"' || ch == '\\' {
			break
		}
		if ch == 0 { // EOF
			return false
		}
		hasContent = true
		lexer.Advance(false)
	}
	if !hasContent {
		return false
	}
	lexer.MarkEnd()
	lexer.SetResultSymbol(gleamSymQuotedContent)
	return true
}

// scanGleamDocCommentContent consumes a single doc comment line until
// newline (inclusive) or EOF.
func scanGleamDocCommentContent(lexer *gotreesitter.ExternalLexer) bool {
	for {
		ch := lexer.Lookahead()
		if ch == 0 { // EOF
			lexer.MarkEnd()
			lexer.SetResultSymbol(gleamSymDocCommentContent)
			return true
		}
		if ch == '\n' {
			lexer.Advance(false)
			lexer.MarkEnd()
			lexer.SetResultSymbol(gleamSymDocCommentContent)
			return true
		}
		lexer.Advance(false)
	}
}

func gleamValid(validSymbols []bool, idx int) bool {
	return idx >= 0 && idx < len(validSymbols) && validSymbols[idx]
}
