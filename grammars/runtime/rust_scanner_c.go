//go:build !grammar_subset || grammar_subset_rust

package grammarruntime

import (
	gotreesitter "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/internal/scanmode"
)

// scanC follows tree-sitter-rust src/scanner.c:335 at commit 77a3747266f4.
func (scanner RustExternalScanner) scanC(payload any, lexer *gotreesitter.ExternalLexer, validSymbols []bool) bool {
	var semanticValid [rustTokenCount]bool
	validSymbols = scanner.remapValidSymbols(validSymbols, &semanticValid)
	symbols := scanner.symbolTable()
	// If the error sentinel is valid, tree-sitter is in error recovery mode.
	// We cannot help recover, so bail out.
	if rustValid(validSymbols, rustTokErrorSentinel) {
		return false
	}

	state := payload.(*rustScannerState)

	// Block comment handling (content, inner/outer doc markers).
	if rustValid(validSymbols, rustTokBlockCommentContent) ||
		rustValid(validSymbols, rustTokInnerDocMarker) ||
		rustValid(validSymbols, rustTokOuterDocMarker) {
		return rustProcessBlockCommentC(lexer, validSymbols, symbols)
	}

	// String content (but not when float literal is also valid).
	// When process_string returns false (lookahead already at '"' or '\' or EOF
	// and no content was consumed), fall through so STRING_CLOSE can consume
	// the closing quote. This mirrors the upstream C scanner's behaviour
	// introduced in tree-sitter-rust 0.24.2.
	if rustValid(validSymbols, rustTokStringContent) && !rustValid(validSymbols, rustTokFloatLiteral) {
		if rustProcessStringC(lexer, symbols) {
			return true
		}
	}

	// String close: closing `"` of a string literal. Emitting as an external
	// token (instead of a plain '"') lets the parser recover from unterminated
	// strings more gracefully.
	if rustValid(validSymbols, rustTokStringClose) && lexer.Lookahead() == '"' {
		lexer.Advance(false)
		lexer.SetResultSymbol(symbols[rustTokStringClose])
		lexer.MarkEnd()
		return true
	}

	// Line doc content.
	if rustValid(validSymbols, rustTokDocComment) {
		return rustProcessLineDocContentC(lexer, symbols)
	}

	// Skip whitespace before checking remaining tokens.
	for cIsSpace(lexer.Lookahead()) {
		lexer.Advance(true)
	}

	// Raw string literal start.
	if rustValid(validSymbols, rustTokRawStringStart) {
		ch := lexer.Lookahead()
		if ch == 'r' || ch == 'b' || ch == 'c' {
			return rustScanRawStringStartC(state, lexer, symbols)
		}
	}

	// Raw string literal content.
	if rustValid(validSymbols, rustTokRawStringContent) {
		return rustScanRawStringContentC(state, lexer, symbols)
	}

	// Raw string literal end.
	if rustValid(validSymbols, rustTokRawStringEnd) && lexer.Lookahead() == '"' {
		return rustScanRawStringEndC(state, lexer, symbols)
	}

	// Float literal.
	if rustValid(validSymbols, rustTokFloatLiteral) && cIsDigit(lexer.Lookahead()) {
		return rustProcessFloatLiteralC(lexer, symbols)
	}

	return false
}

// ---------------------------------------------------------------------------
// String content
// ---------------------------------------------------------------------------

// rustProcessStringC follows tree-sitter-rust src/scanner.c:49.
func rustProcessStringC(lexer *gotreesitter.ExternalLexer, symbols *[rustTokenCount]gotreesitter.Symbol) bool {
	hasContent := false
	for {
		ch := lexer.Lookahead()
		if ch == '"' || ch == '\\' {
			break
		}
		if scanmode.AtEOF(lexer) { // C distinguishes embedded NUL from EOF
			return false
		}
		hasContent = true
		lexer.Advance(false)
	}
	lexer.SetResultSymbol(symbols[rustTokStringContent])
	lexer.MarkEnd()
	return hasContent
}

// ---------------------------------------------------------------------------
// Raw string literals
// ---------------------------------------------------------------------------

// rustScanRawStringStartC follows tree-sitter-rust src/scanner.c:66.
func rustScanRawStringStartC(s *rustScannerState, lexer *gotreesitter.ExternalLexer, symbols *[rustTokenCount]gotreesitter.Symbol) bool {
	ch := lexer.Lookahead()
	if ch == 'b' || ch == 'c' {
		lexer.Advance(false)
	}
	if lexer.Lookahead() != 'r' {
		return false
	}
	lexer.Advance(false)

	var openingHashCount uint8
	for lexer.Lookahead() == '#' {
		lexer.Advance(false)
		openingHashCount++
	}

	if lexer.Lookahead() != '"' {
		return false
	}
	lexer.Advance(false)
	s.openingHashCount = openingHashCount

	lexer.SetResultSymbol(symbols[rustTokRawStringStart])
	return true
}

// rustScanRawStringContentC follows tree-sitter-rust src/scanner.c:91.
func rustScanRawStringContentC(s *rustScannerState, lexer *gotreesitter.ExternalLexer, symbols *[rustTokenCount]gotreesitter.Symbol) bool {
	for {
		if scanmode.AtEOF(lexer) { // C distinguishes embedded NUL from EOF
			return false
		}
		if lexer.Lookahead() == '"' {
			lexer.MarkEnd()
			lexer.Advance(false)
			var hashCount uint8
			for lexer.Lookahead() == '#' && hashCount < s.openingHashCount {
				lexer.Advance(false)
				hashCount++
			}
			if hashCount == s.openingHashCount {
				lexer.SetResultSymbol(symbols[rustTokRawStringContent])
				return true
			}
		} else {
			lexer.Advance(false)
		}
	}
}

// rustScanRawStringEndC follows tree-sitter-rust src/scanner.c:114.
func rustScanRawStringEndC(s *rustScannerState, lexer *gotreesitter.ExternalLexer, symbols *[rustTokenCount]gotreesitter.Symbol) bool {
	lexer.Advance(false) // consume the '"'
	for i := uint8(0); i < s.openingHashCount; i++ {
		lexer.Advance(false)
	}
	lexer.SetResultSymbol(symbols[rustTokRawStringEnd])
	return true
}

// ---------------------------------------------------------------------------
// Float literal
// ---------------------------------------------------------------------------

func rustIsNumCharC(ch rune) bool {
	return ch == '_' || cIsDigit(ch)
}

// rustProcessFloatLiteralC follows tree-sitter-rust src/scanner.c:123.
func rustProcessFloatLiteralC(lexer *gotreesitter.ExternalLexer, symbols *[rustTokenCount]gotreesitter.Symbol) bool {
	lexer.SetResultSymbol(symbols[rustTokFloatLiteral])

	lexer.Advance(false)
	for rustIsNumCharC(lexer.Lookahead()) {
		lexer.Advance(false)
	}

	hasFraction := false
	hasExponent := false

	if lexer.Lookahead() == '.' {
		hasFraction = true
		lexer.Advance(false)
		if cIsAlpha(lexer.Lookahead()) {
			// The dot is followed by a letter: 1.max(2) => not a float but an integer.
			return false
		}
		if lexer.Lookahead() == '.' {
			return false
		}
		for rustIsNumCharC(lexer.Lookahead()) {
			lexer.Advance(false)
		}
	}

	lexer.MarkEnd()

	if lexer.Lookahead() == 'e' || lexer.Lookahead() == 'E' {
		hasExponent = true
		lexer.Advance(false)
		if lexer.Lookahead() == '+' || lexer.Lookahead() == '-' {
			lexer.Advance(false)
		}
		if !rustIsNumCharC(lexer.Lookahead()) {
			return true
		}
		lexer.Advance(false)
		for rustIsNumCharC(lexer.Lookahead()) {
			lexer.Advance(false)
		}
		lexer.MarkEnd()
	}

	if !hasExponent && !hasFraction {
		return false
	}

	ch := lexer.Lookahead()
	if ch != 'u' && ch != 'i' && ch != 'f' {
		return true
	}
	lexer.Advance(false)
	if !cIsDigit(lexer.Lookahead()) {
		return true
	}
	for cIsDigit(lexer.Lookahead()) {
		lexer.Advance(false)
	}

	lexer.MarkEnd()
	return true
}

// ---------------------------------------------------------------------------
// Line doc content
// ---------------------------------------------------------------------------

// rustProcessLineDocContentC follows tree-sitter-rust src/scanner.c:188.
func rustProcessLineDocContentC(lexer *gotreesitter.ExternalLexer, symbols *[rustTokenCount]gotreesitter.Symbol) bool {
	lexer.SetResultSymbol(symbols[rustTokDocComment])
	for {
		if scanmode.AtEOF(lexer) { // C distinguishes embedded NUL from EOF
			return true
		}
		if lexer.Lookahead() == '\n' {
			// Include the newline in the doc content node.
			// Line endings are useful for markdown injection.
			lexer.Advance(false)
			return true
		}
		lexer.Advance(false)
	}
}

// ---------------------------------------------------------------------------
// Block comment
// ---------------------------------------------------------------------------

// rustProcessBlockCommentC follows tree-sitter-rust src/scanner.c:247.
func rustProcessBlockCommentC(lexer *gotreesitter.ExternalLexer, validSymbols []bool, symbols *[rustTokenCount]gotreesitter.Symbol) bool {
	first := rune(byte(scanmode.CLookahead(lexer)))

	// The first character is stored so we can safely advance inside
	// these if blocks. However, because we only store one, we can only
	// safely advance 1 time. Since there's a chance that an advance could
	// happen in one state, we must advance in all states to ensure that
	// the program ends up in a sane state prior to processing the block
	// comment if need be.
	if rustValid(validSymbols, rustTokInnerDocMarker) && first == '!' {
		lexer.SetResultSymbol(symbols[rustTokInnerDocMarker])
		lexer.Advance(false)
		return true
	}
	if rustValid(validSymbols, rustTokOuterDocMarker) && first == '*' {
		lexer.Advance(false)
		lexer.MarkEnd()
		// If the next token is a / that means that it's an empty block comment.
		if lexer.Lookahead() == '/' {
			return false
		}
		// If the next token is a * that means that this isn't a BLOCK_OUTER_DOC_MARKER
		// as BLOCK_OUTER_DOC_MARKER's only have 2 * not 3 or more.
		if lexer.Lookahead() != '*' {
			lexer.SetResultSymbol(symbols[rustTokOuterDocMarker])
			return true
		}
	} else {
		lexer.Advance(false)
	}

	if rustValid(validSymbols, rustTokBlockCommentContent) {
		state := bcContinuing
		nestingDepth := uint32(1)

		// Manually set the current state based on the first character.
		switch first {
		case '*':
			state = bcLeftAsterisk
			if lexer.Lookahead() == '/' {
				// This case can happen in an empty doc block comment
				// like /*!*/. The comment has no contents, so bail.
				return false
			}
		case '/':
			state = bcLeftForwardSlash
		default:
			state = bcContinuing
		}

		// For the purposes of actually parsing rust code, this
		// is incorrect as it considers an unterminated block comment
		// to be an error. However, for the purposes of syntax highlighting
		// this should be considered successful as otherwise you are not able
		// to syntax highlight a block of code prior to closing the
		// block comment.
		for !scanmode.AtEOF(lexer) && nestingDepth != 0 {
			// Set first to the current lookahead as that is the second character
			// as we force an advance in the above code when we are checking if we
			// need to handle a block comment inner or outer doc comment signifier node.
			current := rune(byte(scanmode.CLookahead(lexer)))
			switch state {
			case bcLeftForwardSlash:
				if current == '*' {
					nestingDepth++
				}
				state = bcContinuing
			case bcLeftAsterisk:
				if current == '*' {
					lexer.MarkEnd()
					// Stay in bcLeftAsterisk state.
				} else {
					if current == '/' {
						nestingDepth--
					}
					state = bcContinuing
				}
			case bcContinuing:
				lexer.MarkEnd()
				switch current {
				case '/':
					state = bcLeftForwardSlash
				case '*':
					state = bcLeftAsterisk
				}
			}
			lexer.Advance(false)
			if current == '/' && nestingDepth != 0 {
				lexer.MarkEnd()
			}
		}

		lexer.SetResultSymbol(symbols[rustTokBlockCommentContent])
		return true
	}

	return false
}
