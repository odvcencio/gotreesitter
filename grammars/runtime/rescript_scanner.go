//go:build !grammar_subset || grammar_subset_rescript

package grammarruntime

import (
	"unicode"

	gotreesitter "github.com/odvcencio/gotreesitter"
)

// External token indexes for the rescript grammar.
const (
	rescriptTokNewline           = 0  // _newline
	rescriptTokComment           = 1  // comment (line comment)
	rescriptTokNewlineAndComment = 2  // comment (block comment / newline+comment)
	rescriptTokQuote             = 3  // "
	rescriptTokBacktick          = 4  // `
	rescriptTokTemplateChars     = 5  // _template_chars
	rescriptTokLParen            = 6  // _lparen
	rescriptTokRParen            = 7  // _rparen
	rescriptTokListConstructor   = 8  // _list_constructor
	rescriptTokDictConstructor   = 9  // dict
	rescriptTokDecorator         = 10 // decorator_identifier (with parens)
	rescriptTokDecoratorInline   = 11 // decorator_identifier (inline)
)

// Concrete symbol IDs from the generated rescript grammar ExternalSymbols.
const (
	rescriptSymNewline           gotreesitter.Symbol = 105
	rescriptSymComment           gotreesitter.Symbol = 106
	rescriptSymNewlineAndComment gotreesitter.Symbol = 107
	rescriptSymQuote             gotreesitter.Symbol = 94
	rescriptSymBacktick          gotreesitter.Symbol = 98
	rescriptSymTemplateChars     gotreesitter.Symbol = 108
	rescriptSymLParen            gotreesitter.Symbol = 109
	rescriptSymRParen            gotreesitter.Symbol = 110
	rescriptSymListConstructor   gotreesitter.Symbol = 111
	rescriptSymDictConstructor   gotreesitter.Symbol = 112
	rescriptSymDecorator         gotreesitter.Symbol = 113
	rescriptSymDecoratorInline   gotreesitter.Symbol = 114
)

// rescriptState holds the mutable scanner state that persists across calls
// via serialize/deserialize.
type rescriptState struct {
	parensNesting int32
	inQuotes      bool
	inBackticks   bool
	eofReported   bool
}

// RescriptExternalScanner implements gotreesitter.ExternalScanner for tree-sitter-rescript.
//
// This is a Go port of the C external scanner from rescript-lang/tree-sitter-rescript.
// The scanner handles 12 external tokens: newlines (with statement-termination
// semantics), line and block comments, template string characters, string
// delimiters (" and `), parenthesis nesting, list/dict constructors, and
// decorator identifiers.
type RescriptExternalScanner struct{}

func (RescriptExternalScanner) Create() any {
	return &rescriptState{}
}

func (RescriptExternalScanner) Destroy(payload any) {}

func (RescriptExternalScanner) Serialize(payload any, buf []byte) int {
	s := payload.(*rescriptState)
	if len(buf) < 7 {
		return 0
	}
	// Encode parensNesting as 4 bytes (little-endian).
	buf[0] = byte(s.parensNesting)
	buf[1] = byte(s.parensNesting >> 8)
	buf[2] = byte(s.parensNesting >> 16)
	buf[3] = byte(s.parensNesting >> 24)
	buf[4] = rescriptBoolByte(s.inQuotes)
	buf[5] = rescriptBoolByte(s.inBackticks)
	buf[6] = rescriptBoolByte(s.eofReported)
	return 7
}

func (RescriptExternalScanner) Deserialize(payload any, buf []byte) {
	s := payload.(*rescriptState)
	if len(buf) < 7 {
		*s = rescriptState{}
		return
	}
	s.parensNesting = int32(buf[0]) | int32(buf[1])<<8 | int32(buf[2])<<16 | int32(buf[3])<<24
	s.inQuotes = buf[4] != 0
	s.inBackticks = buf[5] != 0
	s.eofReported = buf[6] != 0
}

func (RescriptExternalScanner) Scan(payload any, lexer *gotreesitter.ExternalLexer, validSymbols []bool) bool {
	s := payload.(*rescriptState)
	inString := s.inQuotes || s.inBackticks

	// Skip inline whitespace (space/tab) unless inside a string.
	for !inString && rescriptIsInlineWhitespace(lexer.Lookahead()) {
		lexer.Advance(true)
	}

	// Template characters: consume content inside backtick strings.
	if rescriptValid(validSymbols, rescriptTokTemplateChars) {
		lexer.SetResultSymbol(rescriptSymTemplateChars)
		hasContent := false
		for {
			lexer.MarkEnd()
			switch lexer.Lookahead() {
			case '`':
				s.inBackticks = false
				return hasContent
			case 0: // EOF
				return false
			case '$':
				lexer.Advance(false)
				ch := lexer.Lookahead()
				if ch == '{' || rescriptIsIdentifierStart(ch) {
					return hasContent
				}
			case '\\':
				return hasContent
			default:
				lexer.Advance(false)
			}
			hasContent = true
		}
	}

	// EOF newline: if a source file is missing EOL at EOF, report a synthetic
	// newline once so the last statement can terminate.
	if rescriptValid(validSymbols, rescriptTokNewline) && lexer.Lookahead() == 0 && !s.eofReported {
		lexer.SetResultSymbol(rescriptSymNewline)
		s.eofReported = true
		return true
	}

	// Newline handling with statement-termination semantics.
	if rescriptValid(validSymbols, rescriptTokNewline) && lexer.Lookahead() == '\n' {
		lexer.SetResultSymbol(rescriptSymNewline)
		lexer.Advance(true)
		lexer.MarkEnd()

		hasComment := rescriptScanWhitespaceAndComments(lexer)
		if hasComment && rescriptValid(validSymbols, rescriptTokNewlineAndComment) {
			lexer.SetResultSymbol(rescriptSymNewlineAndComment)
			lexer.MarkEnd()
		}

		inMultilineStatement := false
		switch lexer.Lookahead() {
		case '-':
			// Ignore new lines before pipe operator (->)
			lexer.Advance(false)
			if lexer.Lookahead() == '>' {
				inMultilineStatement = true
			}
		case '|':
			// Ignore new lines before variant declarations and switch matches
			inMultilineStatement = true
		case '?', ':':
			// Ignore new lines before potential ternaries
			inMultilineStatement = true
		case '}':
			// Do not report new lines before block/switch closings
			inMultilineStatement = true
		case 'a':
			// Check for 'and' keyword
			lexer.Advance(false)
			if lexer.Lookahead() == 'n' {
				lexer.Advance(false)
				if lexer.Lookahead() == 'd' {
					inMultilineStatement = true
				}
			}
		case 'e':
			// Check for 'else' keyword
			lexer.Advance(false)
			if lexer.Lookahead() == 'l' {
				lexer.Advance(false)
				if lexer.Lookahead() == 's' {
					lexer.Advance(false)
					if lexer.Lookahead() == 'e' {
						inMultilineStatement = true
					}
				}
			}
		case 'w':
			// Check for 'with' keyword
			lexer.Advance(false)
			if lexer.Lookahead() == 'i' {
				lexer.Advance(false)
				if lexer.Lookahead() == 't' {
					lexer.Advance(false)
					if lexer.Lookahead() == 'h' {
						inMultilineStatement = true
					}
				}
			}
		}

		if inMultilineStatement {
			if hasComment && rescriptValid(validSymbols, rescriptTokComment) {
				lexer.SetResultSymbol(rescriptSymComment)
				return true
			}
		} else {
			return true
		}
	}

	// Skip whitespace outside strings before remaining checks.
	if !inString {
		rescriptScanWhitespace(lexer, true)
	}

	// Comment: line or block.
	if rescriptValid(validSymbols, rescriptTokComment) && lexer.Lookahead() == '/' && !inString {
		lexer.SetResultSymbol(rescriptSymComment)
		if rescriptScanComment(lexer) {
			lexer.MarkEnd()
			return true
		}
		return false
	}

	// Double-quote: toggle in_quotes state.
	if rescriptValid(validSymbols, rescriptTokQuote) && lexer.Lookahead() == '"' {
		s.inQuotes = !s.inQuotes
		lexer.SetResultSymbol(rescriptSymQuote)
		lexer.Advance(false)
		lexer.MarkEnd()
		return true
	}

	// Backtick: toggle in_backticks state.
	if rescriptValid(validSymbols, rescriptTokBacktick) && lexer.Lookahead() == '`' {
		s.inBackticks = !s.inBackticks
		lexer.SetResultSymbol(rescriptSymBacktick)
		lexer.Advance(false)
		lexer.MarkEnd()
		return true
	}

	// Left parenthesis.
	if rescriptValid(validSymbols, rescriptTokLParen) && lexer.Lookahead() == '(' {
		s.parensNesting++
		lexer.SetResultSymbol(rescriptSymLParen)
		lexer.Advance(false)
		lexer.MarkEnd()
		return true
	}

	// Right parenthesis.
	if rescriptValid(validSymbols, rescriptTokRParen) && lexer.Lookahead() == ')' {
		s.parensNesting--
		lexer.SetResultSymbol(rescriptSymRParen)
		lexer.Advance(false)
		lexer.MarkEnd()
		return true
	}

	// List constructor: "list{".
	if rescriptValid(validSymbols, rescriptTokListConstructor) {
		lexer.SetResultSymbol(rescriptSymListConstructor)
		if lexer.Lookahead() == 'l' {
			lexer.Advance(false)
			if lexer.Lookahead() == 'i' {
				lexer.Advance(false)
				if lexer.Lookahead() == 's' {
					lexer.Advance(false)
					if lexer.Lookahead() == 't' {
						lexer.Advance(false)
						if lexer.Lookahead() == '{' {
							lexer.MarkEnd()
							return true
						}
					}
				}
			}
		}
	}

	// Dict constructor: "dict{".
	if rescriptValid(validSymbols, rescriptTokDictConstructor) {
		lexer.SetResultSymbol(rescriptSymDictConstructor)
		if lexer.Lookahead() == 'd' {
			lexer.Advance(false)
			if lexer.Lookahead() == 'i' {
				lexer.Advance(false)
				if lexer.Lookahead() == 'c' {
					lexer.Advance(false)
					if lexer.Lookahead() == 't' {
						lexer.Advance(false)
						if lexer.Lookahead() == '{' {
							lexer.MarkEnd()
							return true
						}
					}
				}
			}
		}
	}

	// Decorator identifiers: @identifier or @@identifier.
	if rescriptValid(validSymbols, rescriptTokDecorator) &&
		rescriptValid(validSymbols, rescriptTokDecoratorInline) &&
		lexer.Lookahead() == '@' {
		lexer.Advance(false)
		if lexer.Lookahead() == '@' {
			lexer.Advance(false)
		}

		if rescriptIsDecoratorStart(lexer.Lookahead()) {
			lexer.Advance(false)

			// Check for quoted decorator: @foo"..."
			if lexer.Lookahead() == '"' {
				lexer.Advance(false)
				for lexer.Lookahead() != '"' {
					lexer.Advance(false)
					if lexer.Lookahead() == 0 {
						return false
					}
				}
				lexer.Advance(false)
				if rescriptIsWhitespace(lexer.Lookahead()) {
					lexer.SetResultSymbol(rescriptSymDecoratorInline)
					lexer.MarkEnd()
					return true
				}
				if lexer.Lookahead() == '(' {
					lexer.SetResultSymbol(rescriptSymDecorator)
					lexer.MarkEnd()
					return true
				}
				return false
			}

			// Non-quoted decorator identifier.
			for rescriptIsDecoratorIdentifier(lexer.Lookahead()) {
				lexer.Advance(false)
				if lexer.Lookahead() == 0 {
					return false
				}
			}

			if rescriptIsWhitespace(lexer.Lookahead()) {
				lexer.SetResultSymbol(rescriptSymDecoratorInline)
				lexer.MarkEnd()
				return true
			}

			if lexer.Lookahead() == '(' {
				lexer.SetResultSymbol(rescriptSymDecorator)
				lexer.MarkEnd()
				return true
			}
		}
		return false
	}

	// Fall-through: advance one character, skipping if whitespace.
	lexer.Advance(unicode.IsSpace(lexer.Lookahead()))
	return false
}

// ---------------------------------------------------------------------------
// Comment scanning
// ---------------------------------------------------------------------------

// rescriptScanMultilineComment consumes a nested block comment (/* ... */).
// Called after the opening '/' has been consumed and '*' is the current lookahead.
func rescriptScanMultilineComment(lexer *gotreesitter.ExternalLexer) {
	level := 1
	lexer.Advance(false) // consume '*'
	for level > 0 && lexer.Lookahead() != 0 {
		switch lexer.Lookahead() {
		case '/':
			lexer.Advance(false)
			if lexer.Lookahead() == '*' {
				level++
			} else {
				continue
			}
		case '*':
			lexer.Advance(false)
			if lexer.Lookahead() == '/' {
				level--
			} else {
				continue
			}
		default:
			// default case not in C; we just fall through to the advance below
		}
		lexer.Advance(false)
	}
}

// rescriptScanComment attempts to consume a line (//) or block (/* */) comment.
// Returns true if a comment was consumed.
func rescriptScanComment(lexer *gotreesitter.ExternalLexer) bool {
	if lexer.Lookahead() != '/' {
		return false
	}
	lexer.Advance(false) // consume '/'
	switch lexer.Lookahead() {
	case '/':
		// Single-line comment.
		for {
			lexer.Advance(false)
			if lexer.Lookahead() == '\n' || lexer.Lookahead() == 0 {
				break
			}
		}
		return true
	case '*':
		// Multi-line comment.
		rescriptScanMultilineComment(lexer)
		return true
	default:
		return false
	}
}

// rescriptScanWhitespace consumes all whitespace characters (Unicode-aware).
func rescriptScanWhitespace(lexer *gotreesitter.ExternalLexer, skip bool) {
	for unicode.IsSpace(lexer.Lookahead()) && lexer.Lookahead() != 0 {
		lexer.Advance(skip)
	}
}

// rescriptScanWhitespaceAndComments skips whitespace and comments.
// Returns true if at least one comment was found.
func rescriptScanWhitespaceAndComments(lexer *gotreesitter.ExternalLexer) bool {
	hasComments := false
	for lexer.Lookahead() != 0 {
		// Once a comment is found, the subsequent whitespace should not be
		// marked as skipped to keep the correct range of the comment node.
		skipWhitespace := !hasComments
		rescriptScanWhitespace(lexer, skipWhitespace)
		if rescriptScanComment(lexer) {
			hasComments = true
		} else {
			break
		}
	}
	return hasComments
}

// ---------------------------------------------------------------------------
// Character classification helpers
// ---------------------------------------------------------------------------

func rescriptIsInlineWhitespace(ch rune) bool {
	return ch == ' ' || ch == '\t'
}

func rescriptIsIdentifierStart(ch rune) bool {
	return ch == '_' || (ch >= 'a' && ch <= 'z')
}

func rescriptIsDecoratorStart(ch rune) bool {
	return ch == '_' || ch == '\\' || (ch >= 'a' && ch <= 'z') || (ch >= 'A' && ch <= 'Z')
}

func rescriptIsDecoratorIdentifier(ch rune) bool {
	return ch == '_' || ch == '.' || ch == '\'' ||
		(ch >= 'a' && ch <= 'z') || (ch >= 'A' && ch <= 'Z') ||
		(ch >= '0' && ch <= '9')
}

func rescriptIsWhitespace(ch rune) bool {
	return ch == ' ' || ch == '\t' || ch == '\n' || ch == '\r'
}

func rescriptBoolByte(b bool) byte {
	if b {
		return 1
	}
	return 0
}

func rescriptValid(validSymbols []bool, idx int) bool {
	return idx >= 0 && idx < len(validSymbols) && validSymbols[idx]
}
