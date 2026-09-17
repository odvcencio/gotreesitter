//go:build !grammar_subset || grammar_subset_r

package grammarruntime

import (
	"unicode"

	gotreesitter "github.com/odvcencio/gotreesitter"
)

// External token indexes for the R grammar.
// Must match the order of external symbols in the generated R grammar.
const (
	rTokStart            = 0  // _start
	rTokNewline          = 1  // _newline
	rTokSemicolon        = 2  // _semicolon
	rTokRawStringLiteral = 3  // _raw_string_literal
	rTokElse             = 4  // else
	rTokOpenParen        = 5  // (
	rTokCloseParen       = 6  // )
	rTokOpenBrace        = 7  // {
	rTokCloseBrace       = 8  // }
	rTokOpenBracket      = 9  // [
	rTokCloseBracket     = 10 // ]
	rTokOpenBracket2     = 11 // [[
	rTokCloseBracket2    = 12 // ]]
	rTokErrorSentinel    = 13 // _error_sentinel
)

// Concrete symbol IDs from the generated R grammar ExternalSymbols.
const (
	rSymStart            gotreesitter.Symbol = 67
	rSymNewline          gotreesitter.Symbol = 68
	rSymSemicolon        gotreesitter.Symbol = 69
	rSymRawStringLiteral gotreesitter.Symbol = 70
	rSymElse             gotreesitter.Symbol = 71
	rSymOpenParen        gotreesitter.Symbol = 72
	rSymCloseParen       gotreesitter.Symbol = 73
	rSymOpenBrace        gotreesitter.Symbol = 74
	rSymCloseBrace       gotreesitter.Symbol = 75
	rSymOpenBracket      gotreesitter.Symbol = 76
	rSymCloseBracket     gotreesitter.Symbol = 77
	rSymOpenBracket2     gotreesitter.Symbol = 78
	rSymCloseBracket2    gotreesitter.Symbol = 79
	rSymErrorSentinel    gotreesitter.Symbol = 80
)

// Scope values for the R scanner's scope stack.
const (
	rScopeTopLevel byte = 0
	rScopeBrace    byte = 1
	rScopeParen    byte = 2
	rScopeBracket  byte = 3
	rScopeBracket2 byte = 4
)

// Maximum stack size matches TREE_SITTER_SERIALIZATION_BUFFER_SIZE.
const rMaxStackSize = 1024

// rScannerState holds the scope stack for the R external scanner.
// The stack tracks nested (, ), {, }, [, ], [[, ]] scopes.
// SCOPE_TOP_LEVEL is never actually pushed; it is the implicit base
// returned by peek when the stack is empty.
type rScannerState struct {
	stack []byte
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
//   - _raw_string_literal: R raw string literals (r"(...)", R"[...]", etc.)
//   - else: the 'else' keyword with special newline handling in brace scopes
//   - bracket/brace/paren: scope tracking for (, ), {, }, [, ], [[, ]]
//   - _error_sentinel: error recovery detection
type RExternalScanner struct{}

func (RExternalScanner) Create() any {
	return &rScannerState{}
}

func (RExternalScanner) Destroy(payload any) {}

func (RExternalScanner) Serialize(payload any, buf []byte) int {
	s := payload.(*rScannerState)
	n := len(s.stack)
	if n > len(buf) {
		n = len(buf)
	}
	copy(buf[:n], s.stack[:n])
	return n
}

func (RExternalScanner) Deserialize(payload any, buf []byte) {
	s := payload.(*rScannerState)
	s.stack = s.stack[:0]
	if len(buf) > 0 {
		s.stack = append(s.stack, buf...)
	}
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

	if rValid(validSymbols, rTokRawStringLiteral) && (ch == 'r' || ch == 'R') {
		return rScanRawStringLiteral(lexer)
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

// rScanElse checks for the keyword "else" starting at the current lookahead.
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

// rScanRawStringLiteral scans an R raw string literal:
// r"(...)", R'[...]', r"-{...}-", etc.
func rScanRawStringLiteral(lexer *gotreesitter.ExternalLexer) bool {
	lexer.MarkEnd()

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

	// Count hyphens.
	hyphenCount := 0
	for lexer.Lookahead() == '-' {
		lexer.Advance(false)
		hyphenCount++
	}

	// Check for opening bracket and determine closing bracket.
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

	// Scan the body of the raw string until we find the matching
	// closingBracket + hyphens + closingQuote sequence.
	for lexer.Lookahead() != 0 {
		if lexer.Lookahead() != closingBracket {
			// Consume an arbitrary string part.
			lexer.Advance(false)
			continue
		}

		// Consume the closing bracket.
		lexer.Advance(false)

		// Try to consume hyphenCount hyphens in a row.
		matchedHyphens := true
		for i := 0; i < hyphenCount; i++ {
			if lexer.Lookahead() != '-' {
				matchedHyphens = false
				break
			}
			lexer.Advance(false)
		}

		if !matchedHyphens {
			continue
		}

		if lexer.Lookahead() != closingQuote {
			continue
		}

		// Consume the closing quote.
		lexer.Advance(false)

		lexer.MarkEnd()
		lexer.SetResultSymbol(rSymRawStringLiteral)
		return true
	}

	// Hit EOF with unclosed raw string.
	return false
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
