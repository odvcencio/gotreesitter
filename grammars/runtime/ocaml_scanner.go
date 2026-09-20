//go:build !grammar_subset || grammar_subset_ocaml

package grammarruntime

import (
	"unicode"

	gotreesitter "github.com/odvcencio/gotreesitter"
)

// External token indexes for the ocaml grammar. These are scanner-internal
// slot indexes, in the same order as tree-sitter-ocaml's grammar.json
// "externals" array.
const (
	ocamlTokComment              = iota // "comment"
	ocamlTokLeftQuotedStringDel         // "_left_quoted_string_delimiter"
	ocamlTokRightQuotedStringDel        // "_right_quoted_string_delimiter"
	ocamlTokStringDelim                 // "\""
	ocamlTokLineNumberDirective         // "line_number_directive"
	ocamlTokNull                        // "_null"
	ocamlTokErrorSentinel               // "_error_sentinel"
	ocamlTokenCount                     // sentinel
)

// ocamlDefaultSymTable holds the concrete symbol IDs for the ocaml grammar
// blob pinned in grammars/languages.lock. It is a fallback default only: a
// scanner bound to a specific *gotreesitter.Language through
// ExternalScannerForLanguage always uses that Language's own ExternalSymbols,
// read positionally through bindExternalScannerSpec. Grammar symbol IDs shift
// whenever the pinned blob regenerates, so a hardcoded absolute ID used
// directly (instead of through this per-instance binding) silently mismatches
// the next time the grammar's rule set changes shape.
var ocamlDefaultSymTable = [ocamlTokenCount]gotreesitter.Symbol{
	147, // comment
	148, // _left_quoted_string_delimiter
	149, // _right_quoted_string_delimiter
	106, // "\""
	150, // line_number_directive
	151, // _null
	152, // _error_sentinel
}

// ocamlExternalScannerSpec records the upstream scanner-source contract this
// port tracks. common/scanner.h holds the real scanner logic; each
// per-grammar scanner.c (including grammars/ocaml/src/scanner.c) is a thin
// shim that includes it.
var ocamlExternalScannerSpec = ExternalScannerSpec{
	Language:     "ocaml",
	UpstreamRepo: "https://github.com/tree-sitter/tree-sitter-ocaml",
	Externals: []string{
		"comment",
		"_left_quoted_string_delimiter",
		"_right_quoted_string_delimiter",
		"\"",
		"line_number_directive",
		"_null",
		"_error_sentinel",
	},
}

func init() {
	RegisterExternalScannerSpec(ocamlExternalScannerSpec)
}

// ocamlScannerState tracks whether the scanner is inside a string and the
// current quoted string delimiter identifier, matching upstream's Scanner
// struct in common/scanner.h.
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
type OcamlExternalScanner struct {
	symbols         [ocamlTokenCount]gotreesitter.Symbol
	externalToToken []int
}

// ExternalScannerForLanguage binds this scanner's token slots to lang's
// concrete external symbol IDs so Scan reports the IDs the parser table
// actually expects, instead of IDs frozen at some earlier grammar revision.
func (OcamlExternalScanner) ExternalScannerForLanguage(lang *gotreesitter.Language) gotreesitter.ExternalScanner {
	s := OcamlExternalScanner{symbols: ocamlDefaultSymTable}
	s.externalToToken = bindExternalScannerSpec(lang, ocamlExternalScannerSpec, func(tokenIdx int, sym gotreesitter.Symbol) {
		s.symbols[tokenIdx] = sym
	})
	return s
}

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

func (s OcamlExternalScanner) symbolTable() *[ocamlTokenCount]gotreesitter.Symbol {
	if s.symbols == ([ocamlTokenCount]gotreesitter.Symbol{}) {
		return &ocamlDefaultSymTable
	}
	return &s.symbols
}

// remapValidSymbols translates the parser's external-index-space validSymbols
// slice into this scanner's token-index space via externalToToken, matching
// the pattern used by the other positionally bound scanners in this package
// (see dart_scanner.go, csharp_scanner.go).
func (s OcamlExternalScanner) remapValidSymbols(validSymbols []bool, semanticValid *[ocamlTokenCount]bool) []bool {
	if len(s.externalToToken) == 0 {
		return validSymbols
	}
	*semanticValid = [ocamlTokenCount]bool{}
	for externalIdx, valid := range validSymbols {
		if !valid || externalIdx >= len(s.externalToToken) {
			continue
		}
		tokenIdx := s.externalToToken[externalIdx]
		if tokenIdx >= 0 && tokenIdx < ocamlTokenCount {
			semanticValid[tokenIdx] = true
		}
	}
	return semanticValid[:]
}

func (s OcamlExternalScanner) Scan(payload any, lexer *gotreesitter.ExternalLexer, validSymbols []bool) bool {
	state := payload.(*ocamlScannerState)
	var semanticValid [ocamlTokenCount]bool
	validSymbols = s.remapValidSymbols(validSymbols, &semanticValid)
	symbols := s.symbolTable()

	// Left quoted string delimiter: {id|
	if !ocamlValid(validSymbols, ocamlTokErrorSentinel) &&
		ocamlValid(validSymbols, ocamlTokLeftQuotedStringDel) {
		ch := lexer.Lookahead()
		if isOcamlLowercaseExt(ch) || ch == '|' {
			lexer.SetResultSymbol(symbols[ocamlTokLeftQuotedStringDel])
			return ocamlScanLeftQuotedStringDelim(state, lexer)
		}
	}

	// Right quoted string delimiter: |id}
	if !ocamlValid(validSymbols, ocamlTokErrorSentinel) &&
		ocamlValid(validSymbols, ocamlTokRightQuotedStringDel) &&
		lexer.Lookahead() == '|' {
		lexer.Advance(false)
		lexer.SetResultSymbol(symbols[ocamlTokRightQuotedStringDel])
		return ocamlScanRightQuotedStringDelim(state, lexer)
	}

	// Closing string delimiter (before whitespace skip).
	if state.inString && ocamlValid(validSymbols, ocamlTokStringDelim) &&
		lexer.Lookahead() == '"' {
		lexer.Advance(false)
		state.inString = false
		lexer.MarkEnd()
		lexer.SetResultSymbol(symbols[ocamlTokStringDelim])
		return true
	}

	// Skip whitespace.
	for unicode.IsSpace(lexer.Lookahead()) {
		lexer.Advance(true)
	}

	// Opening string delimiter.
	if !state.inString && ocamlValid(validSymbols, ocamlTokStringDelim) &&
		lexer.Lookahead() == '"' {
		lexer.Advance(false)
		state.inString = true
		lexer.MarkEnd()
		lexer.SetResultSymbol(symbols[ocamlTokStringDelim])
		return true
	}

	// Line number directive: # <digits> "filename"
	if !state.inString && ocamlValid(validSymbols, ocamlTokLineNumberDirective) &&
		lexer.Lookahead() == '#' && lexer.Column() == 0 {
		return ocamlScanLineNumberDirective(lexer, symbols[ocamlTokLineNumberDirective])
	}

	// Comment: (* ... *)
	if !state.inString && ocamlValid(validSymbols, ocamlTokComment) &&
		lexer.Lookahead() == '(' {
		lexer.Advance(false)
		lexer.SetResultSymbol(symbols[ocamlTokComment])
		return ocamlScanComment(state, lexer)
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

func ocamlScanLineNumberDirective(lexer *gotreesitter.ExternalLexer, resultSymbol gotreesitter.Symbol) bool {
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
	lexer.SetResultSymbol(resultSymbol)
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
