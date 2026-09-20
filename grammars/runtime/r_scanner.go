//go:build !grammar_subset || grammar_subset_r

package grammarruntime

import (
	"unicode"

	gotreesitter "github.com/odvcencio/gotreesitter"
)

// External token indexes for the R grammar.
// Must match the order of external symbols in the generated R grammar.
//
// r-lib/tree-sitter-r@58a22794466c split `_raw_string_literal` into three
// externals (`_raw_string_open`, `_raw_string_content`, `_raw_string_close`),
// so every external after it shifted down by two positions from the previous
// port (which tracked r-lib/tree-sitter-r@0e6ef7741712).
const (
	rTokStart            = 0  // _start
	rTokNewline          = 1  // _newline
	rTokSemicolon        = 2  // _semicolon
	rTokRawStringOpen    = 3  // _raw_string_open
	rTokRawStringContent = 4  // _raw_string_content
	rTokRawStringClose   = 5  // _raw_string_close
	rTokElse             = 6  // _external_else
	rTokOpenParen        = 7  // _external_open_parenthesis
	rTokCloseParen       = 8  // _external_close_parenthesis
	rTokOpenBrace        = 9  // _external_open_brace
	rTokCloseBrace       = 10 // _external_close_brace
	rTokOpenBracket      = 11 // _external_open_bracket
	rTokCloseBracket     = 12 // _external_close_bracket
	rTokOpenBracket2     = 13 // _external_open_bracket2
	rTokCloseBracket2    = 14 // _external_close_bracket2
	rTokErrorSentinel    = 15 // _error_sentinel
)

// Concrete symbol IDs from the generated R grammar ExternalSymbols.
const (
	rSymStart            gotreesitter.Symbol = 66
	rSymNewline          gotreesitter.Symbol = 67
	rSymSemicolon        gotreesitter.Symbol = 68
	rSymRawStringOpen    gotreesitter.Symbol = 69
	rSymRawStringContent gotreesitter.Symbol = 70
	rSymRawStringClose   gotreesitter.Symbol = 71
	rSymElse             gotreesitter.Symbol = 72
	rSymOpenParen        gotreesitter.Symbol = 73
	rSymCloseParen       gotreesitter.Symbol = 74
	rSymOpenBrace        gotreesitter.Symbol = 75
	rSymCloseBrace       gotreesitter.Symbol = 76
	rSymOpenBracket      gotreesitter.Symbol = 77
	rSymCloseBracket     gotreesitter.Symbol = 78
	rSymOpenBracket2     gotreesitter.Symbol = 79
	rSymCloseBracket2    gotreesitter.Symbol = 80
	rSymErrorSentinel    gotreesitter.Symbol = 81
)

// rExternalScannerSpec records the source contract for this hand-written
// port, so updater tooling can tell a grammar-only upstream change apart
// from one that also touches the external scanner or its token list.
var rExternalScannerSpec = ExternalScannerSpec{
	Language:       "r",
	UpstreamRepo:   "https://github.com/r-lib/tree-sitter-r",
	UpstreamCommit: "58a22794466c0fc15b0d3b40531db751593721e8",
	SourceFiles: []ExternalScannerSourceFile{
		{Path: "src/grammar.json", SHA256: "90a92dc73949699d60c0dc29add88c8158411d36a11c0fadada07d96764a30b1"},
		{Path: "src/scanner.c", SHA256: "e99e003ab8b0463dee975432b6f9f7e39cd75eb0047989f064fdf57927fa8e9d"},
	},
	Externals: []string{
		"_start",
		"_newline",
		"_semicolon",
		"_raw_string_open",
		"_raw_string_content",
		"_raw_string_close",
		"_external_else",
		"_external_open_parenthesis",
		"_external_close_parenthesis",
		"_external_open_brace",
		"_external_close_brace",
		"_external_open_bracket",
		"_external_close_bracket",
		"_external_open_bracket2",
		"_external_close_bracket2",
		"_error_sentinel",
	},
}

func init() {
	RegisterExternalScannerSpec(rExternalScannerSpec)
}

// Scope values for the R scanner's scope stack.
const (
	rScopeTopLevel byte = 0
	rScopeBrace    byte = 1
	rScopeParen    byte = 2
	rScopeBracket  byte = 3
	rScopeBracket2 byte = 4
)

// rMaxStackSize matches upstream's MAX_SCOPES_COUNT. The C scanner packs a
// 3-byte raw string state (closing bracket, hyphen count, closing quote) and
// a 4-byte scope count ahead of the scope array inside
// TREE_SITTER_SERIALIZATION_BUFFER_SIZE (1024 bytes), so the scope array
// gets (1024 - 3 - 4) / 1 = 1017 slots. Keep this in sync with the fields
// serialized below if that changes.
const rMaxStackSize = 1017

// rScannerState holds the R external scanner's persisted state: the scope
// stack that tracks nested (, ), {, }, [, ], [[, ]] scopes, and the raw
// string delimiter state captured by rScanRawStringOpen and consumed by
// rScanRawStringContentOrClose / rScanRawStringClose.
//
// SCOPE_TOP_LEVEL is never actually pushed; it is the implicit base returned
// by peek when the stack is empty.
type rScannerState struct {
	stack []byte

	// Raw string delimiter state, valid only between a _raw_string_open
	// token and the matching _raw_string_close token.
	closingBracket byte
	hyphenCount    uint8
	closingQuote   byte
}

func (s *rScannerState) push(scope byte) bool {
	if len(s.stack) >= rMaxStackSize {
		return false
	}
	s.stack = append(s.stack, scope)
	return true
}

func (s *rScannerState) peek() byte {
	if len(s.stack) == 0 {
		return rScopeTopLevel
	}
	return s.stack[len(s.stack)-1]
}

func (s *rScannerState) pop(expected byte) bool {
	if len(s.stack) == 0 {
		return false
	}
	actual := s.peek()
	s.stack = s.stack[:len(s.stack)-1]
	return actual == expected
}

// RExternalScanner implements gotreesitter.ExternalScanner for tree-sitter-r.
//
// This is a Go port of the C external scanner from tree-sitter-r
// (https://github.com/r-lib/tree-sitter-r). The scanner handles:
//   - _start: zero-width token emitted at the beginning of the file
//   - _newline: contextual newlines in top-level and brace scopes
//   - _semicolon: semicolons
//   - _raw_string_open/_raw_string_content/_raw_string_close: R raw string
//     literals (r"(...)", R'[...]', etc.), split into an opening delimiter,
//     an optional content body, and a closing delimiter
//   - else: the 'else' keyword with special newline handling in brace scopes
//     and a check that it is not the prefix of a longer identifier
//   - bracket/brace/paren: scope tracking for (, ), {, }, [, ], [[, ]]
//   - _error_sentinel: error recovery detection
type RExternalScanner struct{}

func (RExternalScanner) Create() any {
	return &rScannerState{}
}

func (RExternalScanner) Destroy(payload any) {}

// Serialize encodes the raw string state and scope stack. This wire format
// is private to this Go port (nothing decodes it in C), so it need not match
// tree_sitter_r_external_scanner_serialize()'s byte layout, only its
// behavior: it must round-trip through Deserialize and respect the same
// rMaxStackSize capacity.
func (RExternalScanner) Serialize(payload any, buf []byte) int {
	s := payload.(*rScannerState)
	needed := 3 + 2 + len(s.stack)
	if needed > len(buf) {
		// Should not happen: rMaxStackSize keeps the stack within capacity.
		return 0
	}
	n := 0
	buf[n] = s.closingBracket
	n++
	buf[n] = s.hyphenCount
	n++
	buf[n] = s.closingQuote
	n++
	count := len(s.stack)
	buf[n] = byte(count)
	buf[n+1] = byte(count >> 8)
	n += 2
	n += copy(buf[n:], s.stack)
	return n
}

// Deserialize restores the raw string state and scope stack. A zero-length
// buffer is the "reset" signal issued at the start of every parse, matching
// tree_sitter_r_external_scanner_deserialize()'s length == 0 case. A buffer
// too short to hold the header is treated the same way, matching upstream's
// fail-safe payload_reset() on a failed payload_deserialize().
func (RExternalScanner) Deserialize(payload any, buf []byte) {
	s := payload.(*rScannerState)
	s.closingBracket = 0
	s.hyphenCount = 0
	s.closingQuote = 0
	s.stack = s.stack[:0]
	if len(buf) == 0 {
		return
	}
	if len(buf) < 5 {
		return
	}
	s.closingBracket = buf[0]
	s.hyphenCount = buf[1]
	s.closingQuote = buf[2]
	count := int(buf[3]) | int(buf[4])<<8
	rest := buf[5:]
	if count > len(rest) {
		count = len(rest)
	}
	s.stack = append(s.stack, rest[:count]...)
}

func (RExternalScanner) Scan(payload any, lexer *gotreesitter.ExternalLexer, validSymbols []bool) bool {
	s := payload.(*rScannerState)

	// Decline to handle when in "error recovery" mode. When a syntax error
	// occurs, tree-sitter calls the external scanner with all valid_symbols
	// marked as valid.
	if rValid(validSymbols, rTokErrorSentinel) {
		return false
	}

	// START: emit zero-width token at the very beginning of a file before any
	// tokens have been seen. Forces the program node to open at (0,0).
	if rValid(validSymbols, rTokStart) {
		lexer.SetResultSymbol(rSymStart)
		return true
	}

	// These cases are only valid after rScanRawStringOpen accepted a raw
	// string opening sequence. They must run before whitespace and newlines
	// are consumed, otherwise `r"(  hello)"` would not capture the leading
	// whitespace in the string content.
	if rValid(validSymbols, rTokRawStringContent) {
		return rScanRawStringContentOrClose(lexer, s)
	}
	if rValid(validSymbols, rTokRawStringClose) {
		return rScanRawStringClose(lexer, s)
	}

	// Consume whitespace and newlines that have no syntactic meaning.
	rConsumeWhitespaceAndIgnoredNewlines(lexer, s)

	ch := lexer.Lookahead()

	// Purposefully structured as exclusive branches because each scan_*
	// function calls Advance internally, meaning lookahead will no longer
	// be accurate for checking other branches.

	if rValid(validSymbols, rTokSemicolon) && ch == ';' {
		return rScanSemicolon(lexer)
	}

	if rValid(validSymbols, rTokOpenParen) && ch == '(' {
		return rScanOpenBlock(lexer, s, rScopeParen, rSymOpenParen)
	}

	if rValid(validSymbols, rTokCloseParen) && ch == ')' {
		return rScanCloseBlock(lexer, s, rScopeParen, rSymCloseParen)
	}

	if rValid(validSymbols, rTokOpenBrace) && ch == '{' {
		return rScanOpenBlock(lexer, s, rScopeBrace, rSymOpenBrace)
	}

	if rValid(validSymbols, rTokCloseBrace) && ch == '}' {
		return rScanCloseBlock(lexer, s, rScopeBrace, rSymCloseBrace)
	}

	if (rValid(validSymbols, rTokOpenBracket) || rValid(validSymbols, rTokOpenBracket2)) && ch == '[' {
		return rScanOpenBracketOrBracket2(lexer, s, validSymbols)
	}

	// For close bracket vs close bracket2, the scope breaks the tie.
	if rValid(validSymbols, rTokCloseBracket) && ch == ']' && s.peek() == rScopeBracket {
		return rScanCloseBlock(lexer, s, rScopeBracket, rSymCloseBracket)
	}

	if rValid(validSymbols, rTokCloseBracket2) && ch == ']' && s.peek() == rScopeBracket2 {
		return rScanCloseBracket2(lexer, s)
	}

	if rValid(validSymbols, rTokRawStringOpen) && (ch == 'r' || ch == 'R') {
		return rScanRawStringOpen(lexer, s)
	}

	if rValid(validSymbols, rTokElse) && ch == 'e' {
		return rScanElse(lexer)
	}

	if rValid(validSymbols, rTokElse) && s.peek() == rScopeBrace && ch == '\n' {
		// Inside a brace scope, 'else' can follow any number of newlines/whitespace.
		return rScanElseWithLeadingNewlines(lexer)
	}

	if rValid(validSymbols, rTokNewline) && ch == '\n' {
		// Due to rConsumeWhitespaceAndIgnoredNewlines, we are either in top-level
		// or brace scope when we see a newline at this point.
		return rScanNewline(lexer)
	}

	return false
}

// rConsumeWhitespaceAndIgnoredNewlines skips non-newline whitespace and
// newlines inside (, [, [[ scopes. It stops at newlines in top-level or {} scope.
func rConsumeWhitespaceAndIgnoredNewlines(lexer *gotreesitter.ExternalLexer, s *rScannerState) {
	for unicode.IsSpace(lexer.Lookahead()) {
		if lexer.Lookahead() != '\n' {
			// Whitespace that is not a newline: skip it.
			lexer.Advance(true)
			continue
		}

		scope := s.peek()
		if scope == rScopeParen || scope == rScopeBracket || scope == rScopeBracket2 {
			// Newline in (, [, or [[ scope: skip it.
			lexer.Advance(true)
			continue
		}

		// Contextual newline in top-level or brace scope: stop and let scan() handle it.
		break
	}
}

// rIsIdentifierContinuation reports whether ch can continue an R identifier,
// matching upstream's is_identifier_continuation(). It approximates
// XID_Continue: ASCII letters and digits, '_', '.', and any non-ASCII rune
// are treated as continuation characters; every other ASCII value (like '>'
// or '#' or '"') is not.
func rIsIdentifierContinuation(ch rune) bool {
	if ch >= 128 {
		return true
	}
	return unicode.IsLetter(ch) || unicode.IsDigit(ch) || ch == '_' || ch == '.'
}

// rScanElse checks for the keyword "else" starting at the current lookahead.
// It declines if "else" is actually the prefix of a longer identifier, like
// "else_idx" (upstream #200).
func rScanElse(lexer *gotreesitter.ExternalLexer) bool {
	if lexer.Lookahead() != 'e' {
		return false
	}
	lexer.Advance(false)

	if lexer.Lookahead() != 'l' {
		return false
	}
	lexer.Advance(false)

	if lexer.Lookahead() != 's' {
		return false
	}
	lexer.Advance(false)

	if lexer.Lookahead() != 'e' {
		return false
	}
	lexer.Advance(false)

	// Check that this 'else' isn't part of a larger identifier, like 'else_idx'.
	if rIsIdentifierContinuation(lexer.Lookahead()) {
		return false
	}

	lexer.MarkEnd()
	lexer.SetResultSymbol(rSymElse)

	return true
}

// rScanElseWithLeadingNewlines advances past newlines/whitespace in a brace
// scope, then tries to find 'else'. If a comment (#) follows the newlines,
// returns false to let the internal scanner handle it.
func rScanElseWithLeadingNewlines(lexer *gotreesitter.ExternalLexer) bool {
	// Advance past all whitespace (including newlines).
	// We know we have at least 1 newline because this function was called.
	for unicode.IsSpace(lexer.Lookahead()) {
		if lexer.Lookahead() != '\n' {
			lexer.Advance(true)
			continue
		}
		lexer.Advance(true)
		lexer.MarkEnd()
		lexer.SetResultSymbol(rSymNewline)
	}

	// If the next symbol is a comment, allow the internal scanner to pick it up.
	// The mark_end() above ensures we've skipped past interfering newlines.
	// Returning false makes the result_symbol = NEWLINE ignored.
	if lexer.Lookahead() == '#' {
		return false
	}

	// Give the ELSE scanner a chance to run; otherwise return the NEWLINE.
	// Either way we return true because we have found a token.
	rScanElse(lexer)

	return true
}

// rScanRawStringOpen scans the opening delimiter of an R raw string literal:
// r"(, R'[, r---{, etc. It records the matching closing bracket, hyphen
// count, and closing quote in s for rScanRawStringContentOrClose and
// rScanRawStringClose to consume later.
func rScanRawStringOpen(lexer *gotreesitter.ExternalLexer, s *rScannerState) bool {
	// Raw string literals can start with either 'r' or 'R'.
	prefix := lexer.Lookahead()
	if prefix != 'r' && prefix != 'R' {
		return false
	}
	lexer.Advance(false)

	// Check for quote character.
	closingQuote := lexer.Lookahead()
	if closingQuote != '"' && closingQuote != '\'' {
		return false
	}
	lexer.Advance(false)

	// Count hyphens. Bail on the pathological case of 256 hyphens.
	hyphenCount := 0
	for lexer.Lookahead() == '-' {
		if hyphenCount == 255 {
			return false
		}
		lexer.Advance(false)
		hyphenCount++
	}

	// Check for opening bracket and determine the matching closing bracket.
	openingBracket := lexer.Lookahead()
	var closingBracket rune
	switch openingBracket {
	case '(':
		closingBracket = ')'
	case '[':
		closingBracket = ']'
	case '{':
		closingBracket = '}'
	default:
		return false
	}
	lexer.Advance(false)

	lexer.MarkEnd()
	lexer.SetResultSymbol(rSymRawStringOpen)
	s.closingBracket = byte(closingBracket)
	s.hyphenCount = uint8(hyphenCount)
	s.closingQuote = byte(closingQuote)
	return true
}

// rScanRawStringContentOrClose scans the body of a raw string until it finds
// the matching closingBracket -> hyphens -> closingQuote sequence.
//
// It purposefully only advances on known non-closing sequence elements at
// the very beginning of the `!= closingBracket` check (upstream #162):
// consider `r"(())"`, where advancing unconditionally past a tentative `)`
// that turns out not to be the real close would skip over the true closing
// `)`. The same reasoning applies to a tentative hyphen or quote mismatch.
//
// In the case of `r"()"`, where there is no string content, this avoids
// emitting a zero-width content node and instead closes the raw string
// immediately, consistent with single- and double-quoted strings. This must
// happen here, rather than as a lookahead in rScanRawStringOpen, because the
// lexer cannot rewind.
func rScanRawStringContentOrClose(lexer *gotreesitter.ExternalLexer, s *rScannerState) bool {
	closingBracket := rune(s.closingBracket)
	hyphenCount := s.hyphenCount
	closingQuote := rune(s.closingQuote)

	anyContent := false

	for lexer.Lookahead() != 0 {
		if lexer.Lookahead() != closingBracket {
			// Consume an arbitrary string part.
			lexer.Advance(false)
			anyContent = true
			continue
		}

		// Assume we've captured all string content, and that we are about to
		// match the closing sequence. If right, this marker is the content
		// cutoff. If wrong, loop around again and the marker gets reset.
		lexer.MarkEnd()

		// Consume the closing bracket.
		lexer.Advance(false)

		// Try to consume hyphenCount hyphens in a row (0 hyphens "matches"
		// trivially).
		matchedHyphens := true
		for i := uint8(0); i < hyphenCount; i++ {
			if lexer.Lookahead() != '-' {
				matchedHyphens = false
				break
			}
			lexer.Advance(false)
		}

		if !matchedHyphens {
			anyContent = true
			continue
		}

		if lexer.Lookahead() != closingQuote {
			anyContent = true
			continue
		}

		// Consume the closing quote.
		lexer.Advance(false)

		if anyContent {
			// Everything up to MarkEnd() above is content. The closing
			// sequence gets reconsumed next, in rScanRawStringClose.
			lexer.SetResultSymbol(rSymRawStringContent)
		} else {
			lexer.MarkEnd()
			lexer.SetResultSymbol(rSymRawStringClose)
		}
		return true
	}

	// Hit EOF with an unclosed raw string.
	return false
}

// rScanRawStringClose trusts that rScanRawStringContentOrClose already
// validated that the closing sequence comes next, so it consumes it without
// checking a second time.
func rScanRawStringClose(lexer *gotreesitter.ExternalLexer, s *rScannerState) bool {
	// Consume the closing bracket.
	lexer.Advance(false)

	// Consume hyphens.
	for i := uint8(0); i < s.hyphenCount; i++ {
		lexer.Advance(false)
	}

	// Consume the closing quote.
	lexer.Advance(false)

	lexer.MarkEnd()
	lexer.SetResultSymbol(rSymRawStringClose)
	return true
}

// rScanSemicolon consumes a semicolon.
func rScanSemicolon(lexer *gotreesitter.ExternalLexer) bool {
	lexer.Advance(false)
	lexer.MarkEnd()
	lexer.SetResultSymbol(rSymSemicolon)
	return true
}

// rScanNewline consumes a newline character.
func rScanNewline(lexer *gotreesitter.ExternalLexer) bool {
	lexer.Advance(false)
	lexer.MarkEnd()
	lexer.SetResultSymbol(rSymNewline)
	return true
}

// rScanOpenBlock pushes a scope and consumes the opening delimiter.
func rScanOpenBlock(lexer *gotreesitter.ExternalLexer, s *rScannerState, scope byte, sym gotreesitter.Symbol) bool {
	if !s.push(scope) {
		return false
	}
	lexer.Advance(false)
	lexer.MarkEnd()
	lexer.SetResultSymbol(sym)
	return true
}

// rScanCloseBlock pops a scope and consumes the closing delimiter.
func rScanCloseBlock(lexer *gotreesitter.ExternalLexer, s *rScannerState, scope byte, sym gotreesitter.Symbol) bool {
	if !s.pop(scope) {
		return false
	}
	lexer.Advance(false)
	lexer.MarkEnd()
	lexer.SetResultSymbol(sym)
	return true
}

// rScanOpenBracketOrBracket2 handles [ and [[ disambiguation.
// If [[ is valid and the next char is [, greedily accept [[.
// Otherwise accept a single [.
func rScanOpenBracketOrBracket2(lexer *gotreesitter.ExternalLexer, s *rScannerState, validSymbols []bool) bool {
	// We know lookahead is the first [.
	lexer.Advance(false)

	// If [[ is valid and we see another [, greedily accept [[.
	if rValid(validSymbols, rTokOpenBracket2) && lexer.Lookahead() == '[' {
		if !s.push(rScopeBracket2) {
			return false
		}
		lexer.Advance(false)
		lexer.MarkEnd()
		lexer.SetResultSymbol(rSymOpenBracket2)
		return true
	}

	// Otherwise accept a single [.
	if rValid(validSymbols, rTokOpenBracket) {
		if !s.push(rScopeBracket) {
			return false
		}
		lexer.MarkEnd()
		lexer.SetResultSymbol(rSymOpenBracket)
		return true
	}

	return false
}

// rScanCloseBracket2 handles ]] by consuming the first ] and checking for a second.
func rScanCloseBracket2(lexer *gotreesitter.ExternalLexer, s *rScannerState) bool {
	// We know lookahead is the first ].
	lexer.Advance(false)

	if lexer.Lookahead() != ']' {
		// Like x[[1] where we want an unmatched ].
		return false
	}

	return rScanCloseBlock(lexer, s, rScopeBracket2, rSymCloseBracket2)
}

// rValid checks if the external token at the given index is valid.
func rValid(validSymbols []bool, idx int) bool {
	return idx >= 0 && idx < len(validSymbols) && validSymbols[idx]
}
