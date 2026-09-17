//go:build !grammar_subset || grammar_subset_comment

package grammarruntime

import gotreesitter "github.com/odvcencio/gotreesitter"

// External token indexes for the comment grammar.
const (
	commentTokName         = 0 // "name" — tag keyword like TODO, FIXME, NOTE
	commentTokInvalidToken = 1 // "invalid_token" — error recovery
)

// Concrete symbol IDs from the generated comment grammar ExternalSymbols.
const (
	commentSymName         gotreesitter.Symbol = 25
	commentSymInvalidToken gotreesitter.Symbol = 26
)

// CommentExternalScanner implements gotreesitter.ExternalScanner for tree-sitter-comment.
//
// The comment grammar (tree-sitter-comment) parses structured comment text
// such as "TODO: fix this" or "FIXME(user): description". The external
// scanner is responsible for producing the "name" token which represents
// a tag keyword (TODO, FIXME, NOTE, HACK, etc.).
//
// The scanner must be careful not to match arbitrary text as a "name",
// since the DFA handles regular text via _text_token1. A name is only
// returned when the scanned word is immediately followed by ':' or '(',
// indicating it forms part of a tag construct.
type CommentExternalScanner struct{}

func (CommentExternalScanner) Create() any                           { return nil }
func (CommentExternalScanner) Destroy(payload any)                   {}
func (CommentExternalScanner) Serialize(payload any, buf []byte) int { return 0 }
func (CommentExternalScanner) Deserialize(payload any, buf []byte)   {}
func (CommentExternalScanner) SupportsIncrementalReuse() bool        { return true }
func (CommentExternalScanner) ExternalScannerIsStateless() bool      { return true }

func (CommentExternalScanner) Scan(payload any, lexer *gotreesitter.ExternalLexer, validSymbols []bool) bool {
	// Upstream treats invalid_token as correction mode. The Go-generated
	// external valid set can conservatively include invalid_token beside name,
	// so only decline when name is not also explicitly valid.
	if len(validSymbols) > commentTokInvalidToken && validSymbols[commentTokInvalidToken] &&
		!(len(validSymbols) > commentTokName && validSymbols[commentTokName]) {
		return false
	}

	// name: scan a tag keyword like TODO, FIXME, NOTE, etc.
	if len(validSymbols) > commentTokName && validSymbols[commentTokName] {
		return scanCommentName(lexer)
	}

	return false
}

// scanCommentName scans a name token (tag keyword), mirroring
// tree-sitter-comment's scanner:
//   - the name starts with an uppercase ASCII letter,
//   - continues with uppercase ASCII, digits, '-' or '_',
//   - cannot end with '-' or '_',
//   - may be followed by optional non-newline space plus a non-empty user
//     component in parentheses,
//   - and must end with ':' followed by whitespace.
func scanCommentName(lexer *gotreesitter.ExternalLexer) bool {
	if !isCommentUpper(lexer.Lookahead()) {
		return false
	}

	previous := lexer.Lookahead()
	lexer.Advance(false)
	for isCommentUpper(lexer.Lookahead()) ||
		isCommentDigit(lexer.Lookahead()) ||
		isCommentInternal(lexer.Lookahead()) {
		previous = lexer.Lookahead()
		lexer.Advance(false)
	}
	lexer.MarkEnd()

	if isCommentInternal(previous) {
		return false
	}

	if (isCommentSpace(lexer.Lookahead()) && !isCommentNewline(lexer.Lookahead())) ||
		lexer.Lookahead() == '(' {
		for isCommentSpace(lexer.Lookahead()) && !isCommentNewline(lexer.Lookahead()) {
			lexer.Advance(false)
		}
		if lexer.Lookahead() != '(' {
			return false
		}
		lexer.Advance(false)

		userLength := 0
		for lexer.Lookahead() != ')' {
			if isCommentNewline(lexer.Lookahead()) {
				return false
			}
			lexer.Advance(false)
			userLength++
		}
		if userLength <= 0 {
			return false
		}
		lexer.Advance(false)
	}

	if lexer.Lookahead() != ':' {
		return false
	}
	lexer.Advance(false)
	if !isCommentSpace(lexer.Lookahead()) {
		return false
	}

	lexer.SetResultSymbol(commentSymName)
	return true
}

func isCommentUpper(ch rune) bool {
	if ch >= 'A' && ch <= 'Z' {
		return true
	}
	return false
}

func isCommentDigit(ch rune) bool {
	if ch >= '0' && ch <= '9' {
		return true
	}
	return false
}

func isCommentInternal(ch rune) bool {
	return ch == '-' || ch == '_'
}

func isCommentNewline(ch rune) bool {
	return ch == 0 || ch == '\n' || ch == '\r'
}

func isCommentSpace(ch rune) bool {
	switch ch {
	case ' ', '\f', '\t', '\v':
		return true
	default:
		return isCommentNewline(ch)
	}
}
