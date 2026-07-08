//go:build !grammar_subset || grammar_subset_ocaml

package grammars

import (
	"unicode"

	gotreesitter "github.com/odvcencio/gotreesitter"
)

// External token indexes for the ocaml grammar.
const (
	ocamlTokComment              = 0 // "comment"
	ocamlTokLeftQuotedStringDel  = 1 // "_left_quoted_string_delimiter"
	ocamlTokRightQuotedStringDel = 2 // "_right_quoted_string_delimiter"
	ocamlTokStringDelim          = 3 // "\""
	ocamlTokLineNumberDirective  = 4 // "line_number_directive"
	ocamlTokNull                 = 5 // "_null"
	ocamlTokErrorSentinel        = 6 // "_error_sentinel"
)

// Concrete symbol IDs from the generated ocaml grammar ExternalSymbols.
const (
	ocamlSymComment              gotreesitter.Symbol = 147
	ocamlSymLeftQuotedStringDel  gotreesitter.Symbol = 148
	ocamlSymRightQuotedStringDel gotreesitter.Symbol = 149
	ocamlSymStringDelim          gotreesitter.Symbol = 106
	ocamlSymLineNumberDirective  gotreesitter.Symbol = 150
	ocamlSymNull                 gotreesitter.Symbol = 151
	ocamlSymErrorSentinel        gotreesitter.Symbol = 152
)

// ocamlScannerState tracks whether we're inside a string and the current
// quoted string delimiter identifier.
type ocamlScannerState struct {
	inString       bool
	quotedStringID []int32 // delimiter chars for {id|...|id} strings
}

// OcamlExternalScanner implements gotreesitter.ExternalScanner for tree-sitter-ocaml.
//
// This is a Go port of the C external scanner from tree-sitter/tree-sitter-ocaml.
// The scanner handles:
//   - Nestable (* *) comments (lexically aware of strings/chars inside)
//   - Quoted string delimiters {id|...|id}
//   - String open/close with in_string state tracking
//   - Line number directives (# <num> "file")
//   - Literal null characters (\0 that isn't EOF)
type OcamlExternalScanner struct{}

func (OcamlExternalScanner) Create() any {
	return &ocamlScannerState{}
}

func (OcamlExternalScanner) Destroy(payload any) {}

func (OcamlExternalScanner) Serialize(payload any, buf []byte) int {
	s := payload.(*ocamlScannerState)
	if len(buf) == 0 {
		return 0
	}
	if s.inString {
		buf[0] = 1
	} else {
		buf[0] = 0
	}
	// Copy quoted string ID (stored as int32 bytes).
	idBytes := len(s.quotedStringID) * 4
	if 1+idBytes > len(buf) {
		return 1
	}
	pos := 1
	for _, c := range s.quotedStringID {
		buf[pos] = byte(c)
		buf[pos+1] = byte(c >> 8)
		buf[pos+2] = byte(c >> 16)
		buf[pos+3] = byte(c >> 24)
		pos += 4
	}
	return pos
}

func (OcamlExternalScanner) Deserialize(payload any, buf []byte) {
	s := payload.(*ocamlScannerState)
	s.inString = false
	s.quotedStringID = s.quotedStringID[:0]

	if len(buf) == 0 {
		return
	}
	s.inString = buf[0] != 0
	pos := 1
	for pos+4 <= len(buf) {
		c := int32(buf[pos]) | int32(buf[pos+1])<<8 | int32(buf[pos+2])<<16 | int32(buf[pos+3])<<24
		s.quotedStringID = append(s.quotedStringID, c)
		pos += 4
	}
}

func (OcamlExternalScanner) Scan(payload any, lexer *gotreesitter.ExternalLexer, validSymbols []bool) bool {
	s := payload.(*ocamlScannerState)

	// Left quoted string delimiter: {id|
	if !ocamlValid(validSymbols, ocamlTokErrorSentinel) &&
		ocamlValid(validSymbols, ocamlTokLeftQuotedStringDel) {
		ch := lexer.Lookahead()
		if isOcamlLowercaseExt(ch) || ch == '|' {
			lexer.SetResultSymbol(ocamlSymLeftQuotedStringDel)
			return ocamlScanLeftQuotedStringDelim(s, lexer)
		}
	}

	// Right quoted string delimiter: |id}
	if !ocamlValid(validSymbols, ocamlTokErrorSentinel) &&
		ocamlValid(validSymbols, ocamlTokRightQuotedStringDel) &&
		lexer.Lookahead() == '|' {
		lexer.Advance(false)
		lexer.SetResultSymbol(ocamlSymRightQuotedStringDel)
		return ocamlScanRightQuotedStringDelim(s, lexer)
	}

	// Closing string delimiter (before whitespace skip).
	if s.inString && ocamlValid(validSymbols, ocamlTokStringDelim) &&
		lexer.Lookahead() == '"' {
		lexer.Advance(false)
		s.inString = false
		lexer.MarkEnd()
		lexer.SetResultSymbol(ocamlSymStringDelim)
		return true
	}

	// Skip whitespace.
	for unicode.IsSpace(lexer.Lookahead()) {
		lexer.Advance(true)
	}

	// Opening string delimiter.
	if !s.inString && ocamlValid(validSymbols, ocamlTokStringDelim) &&
		lexer.Lookahead() == '"' {
		lexer.Advance(false)
		s.inString = true
		lexer.MarkEnd()
		lexer.SetResultSymbol(ocamlSymStringDelim)
		return true
	}

	// Line number directive: # <digits> "filename"
	if !s.inString && ocamlValid(validSymbols, ocamlTokLineNumberDirective) &&
		lexer.Lookahead() == '#' && lexer.Column() == 0 {
		return ocamlScanLineNumberDirective(lexer)
	}

	// Comment: (* ... *)
	if !s.inString && ocamlValid(validSymbols, ocamlTokComment) &&
		lexer.Lookahead() == '(' {
		lexer.Advance(false)
		lexer.SetResultSymbol(ocamlSymComment)
		return ocamlScanComment(s, lexer)
	}

	// Null character (literal \0 that isn't EOF).
	if ocamlValid(validSymbols, ocamlTokNull) &&
		lexer.Lookahead() == 0 {
		// We can't distinguish true null from EOF via Lookahead() alone.
		// The C scanner checks !eof(lexer), but our lexer returns 0 for both.
		// In practice, this token is rarely needed. We decline to avoid
		// false positives at EOF.
		return false
	}

	return false
}

// ---------------------------------------------------------------------------
// Quoted string delimiters
// ---------------------------------------------------------------------------

func ocamlScanLeftQuotedStringDelim(s *ocamlScannerState, lexer *gotreesitter.ExternalLexer) bool {
	s.quotedStringID = s.quotedStringID[:0]

	for {
		c := ocamlScanQuotedStringDelimChar(lexer)
		if c == 0 {
			break
		}
		s.quotedStringID = append(s.quotedStringID, c)
	}

	if lexer.Lookahead() == '|' {
		lexer.Advance(false)
		lexer.MarkEnd()
		s.inString = true
		return true
	}

	s.quotedStringID = s.quotedStringID[:0]
	return false
}

func ocamlScanRightQuotedStringDelim(s *ocamlScannerState, lexer *gotreesitter.ExternalLexer) bool {
	for i, expected := range s.quotedStringID {
		_ = i
		c := ocamlScanQuotedStringDelimChar(lexer)
		if c != expected {
			return false
		}
	}

	if lexer.Lookahead() == '}' {
		lexer.MarkEnd()
		s.inString = false
		s.quotedStringID = s.quotedStringID[:0]
		return true
	}
	return false
}

// ocamlScanQuotedStringDelimChar scans one character of a quoted string
// delimiter identifier. Returns the char or 0 if not a valid delimiter char.
// Valid chars: lowercase letters, '_', '|' stops scanning (returns 0).
func ocamlScanQuotedStringDelimChar(lexer *gotreesitter.ExternalLexer) int32 {
	ch := lexer.Lookahead()
	if ch == '|' {
		return 0
	}
	if ch == '_' || (ch >= 'a' && ch <= 'z') {
		lexer.Advance(false)
		return ch
	}
	// Extended lowercase Unicode characters.
	if ch >= 192 && unicode.IsLower(ch) {
		lexer.Advance(false)
		return ch
	}
	return 0
}

// ---------------------------------------------------------------------------
// Comment scanning (recursive, lexically aware)
// ---------------------------------------------------------------------------

func ocamlScanComment(s *ocamlScannerState, lexer *gotreesitter.ExternalLexer) bool {
	// Expect '*' after '('.
	if lexer.Lookahead() != '*' {
		return false
	}
	lexer.Advance(false)

	for {
		ch := lexer.Lookahead()
		switch ch {
		case '(':
			// Possible nested comment.
			lexer.Advance(false)
			if lexer.Lookahead() == '*' {
				// Recursive nested comment.
				lexer.Advance(false)
				if !ocamlScanCommentBody(s, lexer) {
					return false
				}
			}
		case '*':
			lexer.Advance(false)
			if lexer.Lookahead() == ')' {
				lexer.Advance(false)
				lexer.MarkEnd()
				return true
			}
		case '"':
			// String inside comment — skip it.
			lexer.Advance(false)
			ocamlSkipString(lexer)
		case '{':
			// Possible quoted string inside comment.
			lexer.Advance(false)
			ocamlSkipQuotedString(s, lexer)
		case '\'':
			// Character literal inside comment.
			lexer.Advance(false)
			ocamlSkipCharLiteral(lexer)
		case 0: // EOF
			return false
		default:
			lexer.Advance(false)
		}
	}
}

// ocamlScanCommentBody is the recursive helper for nested comments.
func ocamlScanCommentBody(s *ocamlScannerState, lexer *gotreesitter.ExternalLexer) bool {
	for {
		ch := lexer.Lookahead()
		switch ch {
		case '(':
			lexer.Advance(false)
			if lexer.Lookahead() == '*' {
				lexer.Advance(false)
				if !ocamlScanCommentBody(s, lexer) {
					return false
				}
			}
		case '*':
			lexer.Advance(false)
			if lexer.Lookahead() == ')' {
				lexer.Advance(false)
				return true
			}
		case '"':
			lexer.Advance(false)
			ocamlSkipString(lexer)
		case '{':
			lexer.Advance(false)
			ocamlSkipQuotedString(s, lexer)
		case '\'':
			lexer.Advance(false)
			ocamlSkipCharLiteral(lexer)
		case 0:
			return false
		default:
			lexer.Advance(false)
		}
	}
}

// ocamlSkipString skips a regular "..." string inside a comment.
func ocamlSkipString(lexer *gotreesitter.ExternalLexer) {
	for {
		ch := lexer.Lookahead()
		switch ch {
		case '\\':
			lexer.Advance(false)
			lexer.Advance(false) // skip escaped char
		case '"':
			lexer.Advance(false)
			return
		case 0:
			return
		default:
			lexer.Advance(false)
		}
	}
}

// ocamlSkipQuotedString skips a {id|...|id} quoted string inside a comment.
func ocamlSkipQuotedString(s *ocamlScannerState, lexer *gotreesitter.ExternalLexer) {
	// Save and restore quoted string ID since we might be inside one.
	savedID := make([]int32, len(s.quotedStringID))
	copy(savedID, s.quotedStringID)
	savedInString := s.inString

	if !ocamlScanLeftQuotedStringDelim(s, lexer) {
		s.quotedStringID = savedID
		s.inString = savedInString
		return
	}

	for {
		ch := lexer.Lookahead()
		switch ch {
		case '|':
			lexer.Advance(false)
			if ocamlScanRightQuotedStringDelim(s, lexer) {
				s.quotedStringID = savedID
				s.inString = savedInString
				return
			}
		case 0:
			s.quotedStringID = savedID
			s.inString = savedInString
			return
		default:
			lexer.Advance(false)
		}
	}
}

// ocamlSkipCharLiteral skips a character literal inside a comment.
func ocamlSkipCharLiteral(lexer *gotreesitter.ExternalLexer) {
	ch := lexer.Lookahead()
	if ch == '\\' {
		lexer.Advance(false)
		lexer.Advance(false)
	} else if ch != '\'' && ch != 0 {
		lexer.Advance(false)
	}
	// Expect closing quote.
	if lexer.Lookahead() == '\'' {
		lexer.Advance(false)
	}
}

// ---------------------------------------------------------------------------
// Line number directive
// ---------------------------------------------------------------------------

func ocamlScanLineNumberDirective(lexer *gotreesitter.ExternalLexer) bool {
	lexer.Advance(false) // consume '#'

	// Skip spaces/tabs.
	for lexer.Lookahead() == ' ' || lexer.Lookahead() == '\t' {
		lexer.Advance(false)
	}

	// Expect digits.
	if !unicode.IsDigit(lexer.Lookahead()) {
		return false
	}
	for unicode.IsDigit(lexer.Lookahead()) {
		lexer.Advance(false)
	}

	// Skip spaces/tabs.
	for lexer.Lookahead() == ' ' || lexer.Lookahead() == '\t' {
		lexer.Advance(false)
	}

	// Expect opening quote.
	if lexer.Lookahead() != '"' {
		return false
	}
	lexer.Advance(false)

	// Filename: everything until closing quote, newline, or EOF.
	for {
		ch := lexer.Lookahead()
		if ch == '\n' || ch == '\r' || ch == '"' || ch == 0 {
			break
		}
		lexer.Advance(false)
	}

	if lexer.Lookahead() != '"' {
		return false
	}
	lexer.Advance(false)

	// Consume rest of line.
	for {
		ch := lexer.Lookahead()
		if ch == '\n' || ch == '\r' || ch == 0 {
			break
		}
		lexer.Advance(false)
	}

	lexer.MarkEnd()
	lexer.SetResultSymbol(ocamlSymLineNumberDirective)
	return true
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func isOcamlLowercaseExt(ch rune) bool {
	return (ch >= 'a' && ch <= 'z') || ch == '_' || (ch >= 192 && unicode.IsLower(ch))
}

func ocamlValid(validSymbols []bool, idx int) bool {
	return idx >= 0 && idx < len(validSymbols) && validSymbols[idx]
}
