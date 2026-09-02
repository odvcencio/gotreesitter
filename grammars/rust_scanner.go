//go:build !grammar_subset || grammar_subset_rust

package grammars

import (
	"unicode"

	gotreesitter "github.com/odvcencio/gotreesitter"
)

// External token indexes for the rust grammar.
// NOTE: index 1 (string_close) was introduced in tree-sitter-rust 0.24.2
// (upstream commit b3e615d) as part of string literal error-recovery fix.
const (
	rustTokStringContent       = 0  // "string_content"
	rustTokStringClose         = 1  // "string_close" (closing `"` of string literal)
	rustTokRawStringStart      = 2  // "_raw_string_literal_start"
	rustTokRawStringContent    = 3  // "string_content" (raw variant)
	rustTokRawStringEnd        = 4  // "_raw_string_literal_end"
	rustTokFloatLiteral        = 5  // "float_literal"
	rustTokOuterDocMarker      = 6  // "outer_doc_comment_marker"
	rustTokInnerDocMarker      = 7  // "inner_doc_comment_marker"
	rustTokBlockCommentContent = 8  // "_block_comment_content"
	rustTokDocComment          = 9  // "doc_comment"
	rustTokErrorSentinel       = 10 // "_error_sentinel"
)

// Concrete symbol IDs from the generated rust grammar ExternalSymbols.
// (Symbol 147 = string_close alias `"`, inserted between string_content and
// _raw_string_literal_start; all downstream symbols shift by +1.)
const (
	rustSymStringContent       gotreesitter.Symbol = 146
	rustSymStringClose         gotreesitter.Symbol = 147
	rustSymRawStringStart      gotreesitter.Symbol = 148
	rustSymRawStringContent    gotreesitter.Symbol = 149
	rustSymRawStringEnd        gotreesitter.Symbol = 150
	rustSymFloatLiteral        gotreesitter.Symbol = 151
	rustSymOuterDocMarker      gotreesitter.Symbol = 152
	rustSymInnerDocMarker      gotreesitter.Symbol = 153
	rustSymBlockCommentContent gotreesitter.Symbol = 154
	rustSymDocComment          gotreesitter.Symbol = 155
	rustSymErrorSentinel       gotreesitter.Symbol = 156
)

const rustTokenCount = rustTokErrorSentinel + 1

var rustDefaultSymTable = [rustTokenCount]gotreesitter.Symbol{
	rustTokStringContent:       rustSymStringContent,
	rustTokStringClose:         rustSymStringClose,
	rustTokRawStringStart:      rustSymRawStringStart,
	rustTokRawStringContent:    rustSymRawStringContent,
	rustTokRawStringEnd:        rustSymRawStringEnd,
	rustTokFloatLiteral:        rustSymFloatLiteral,
	rustTokOuterDocMarker:      rustSymOuterDocMarker,
	rustTokInnerDocMarker:      rustSymInnerDocMarker,
	rustTokBlockCommentContent: rustSymBlockCommentContent,
	rustTokDocComment:          rustSymDocComment,
	rustTokErrorSentinel:       rustSymErrorSentinel,
}

var rustExternalSymbolNames = []string{
	"string_content",
	"\"",
	"_raw_string_literal_start",
	"string_content",
	"_raw_string_literal_end",
	"float_literal",
	"outer_doc_comment_marker",
	"inner_doc_comment_marker",
	"_block_comment_content",
	"doc_comment",
	"_error_sentinel",
}

// rustScannerState holds the opening hash count for raw string literals.
type rustScannerState struct {
	openingHashCount uint8
}

// RustExternalScanner implements gotreesitter.ExternalScanner for tree-sitter-rust.
//
// This is a Go port of the C external scanner from tree-sitter/tree-sitter-rust.
// The scanner handles 10 external tokens:
//   - String content (regular string escape sequences)
//   - Raw string literals (r#"..."# with hash counting for start/content/end)
//   - Float literals (disambiguation from integer + method call)
//   - Block comment content (nested /* */ comments)
//   - Doc comments (/// and //!)
//   - Outer/inner doc comment markers
//   - Error sentinel for error recovery detection
type RustExternalScanner struct {
	symbols         [rustTokenCount]gotreesitter.Symbol
	externalToToken []int
}

func (RustExternalScanner) ExternalScannerForLanguage(lang *gotreesitter.Language) gotreesitter.ExternalScanner {
	s := RustExternalScanner{symbols: rustDefaultSymTable}
	s.externalToToken = bindExternalScannerSymbolNames(lang, rustExternalSymbolNames, func(tokenIdx int, sym gotreesitter.Symbol) {
		s.symbols[tokenIdx] = sym
	})
	return s
}

func (s RustExternalScanner) symbolTable() *[rustTokenCount]gotreesitter.Symbol {
	if s.symbols == ([rustTokenCount]gotreesitter.Symbol{}) {
		return &rustDefaultSymTable
	}
	return &s.symbols
}

func (s RustExternalScanner) remapValidSymbols(validSymbols []bool, semanticValid *[rustTokenCount]bool) []bool {
	if len(s.externalToToken) == 0 {
		return validSymbols
	}
	*semanticValid = [rustTokenCount]bool{}
	for externalIdx, valid := range validSymbols {
		if !valid || externalIdx >= len(s.externalToToken) {
			continue
		}
		tokenIdx := s.externalToToken[externalIdx]
		if tokenIdx >= 0 && tokenIdx < rustTokenCount {
			semanticValid[tokenIdx] = true
		}
	}
	return semanticValid[:]
}

func (RustExternalScanner) Create() any {
	return &rustScannerState{}
}

func (RustExternalScanner) Destroy(payload any) {}

func (RustExternalScanner) Serialize(payload any, buf []byte) int {
	state := payload.(*rustScannerState)
	if len(buf) < 1 {
		return 0
	}
	buf[0] = state.openingHashCount
	return 1
}

func (RustExternalScanner) Deserialize(payload any, buf []byte) {
	state := payload.(*rustScannerState)
	state.openingHashCount = 0
	if len(buf) == 1 {
		state.openingHashCount = buf[0]
	}
}

// SupportsIncrementalReuse reports that the Rust scanner can restore its
// complete serialized state at a reused subtree boundary.
func (RustExternalScanner) SupportsIncrementalReuse() bool { return true }

// UsesExternalScannerCheckpoints requires reuse to authenticate the raw-string
// hash count before it resumes scanning after a reused subtree.
func (RustExternalScanner) UsesExternalScannerCheckpoints() bool { return true }

func (scanner RustExternalScanner) Scan(payload any, lexer *gotreesitter.ExternalLexer, validSymbols []bool) bool {
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
		return rustProcessBlockComment(lexer, validSymbols, symbols)
	}

	// String content (but not when float literal is also valid).
	// When process_string returns false (lookahead already at '"' or '\' or EOF
	// and no content was consumed), fall through so STRING_CLOSE can consume
	// the closing quote. This mirrors the upstream C scanner's behaviour
	// introduced in tree-sitter-rust 0.24.2.
	if rustValid(validSymbols, rustTokStringContent) && !rustValid(validSymbols, rustTokFloatLiteral) {
		if rustProcessString(lexer, symbols) {
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
		return rustProcessLineDocContent(lexer, symbols)
	}

	// Skip whitespace before checking remaining tokens.
	for unicode.IsSpace(lexer.Lookahead()) {
		lexer.Advance(true)
	}

	// Raw string literal start.
	if rustValid(validSymbols, rustTokRawStringStart) {
		ch := lexer.Lookahead()
		if ch == 'r' || ch == 'b' || ch == 'c' {
			return rustScanRawStringStart(state, lexer, symbols)
		}
	}

	// Raw string literal content.
	if rustValid(validSymbols, rustTokRawStringContent) {
		return rustScanRawStringContent(state, lexer, symbols)
	}

	// Raw string literal end.
	if rustValid(validSymbols, rustTokRawStringEnd) && lexer.Lookahead() == '"' {
		return rustScanRawStringEnd(state, lexer, symbols)
	}

	// Float literal.
	if rustValid(validSymbols, rustTokFloatLiteral) && unicode.IsDigit(lexer.Lookahead()) {
		return rustProcessFloatLiteral(lexer, symbols)
	}

	return false
}

// ---------------------------------------------------------------------------
// String content
// ---------------------------------------------------------------------------

func rustProcessString(lexer *gotreesitter.ExternalLexer, symbols *[rustTokenCount]gotreesitter.Symbol) bool {
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
	lexer.SetResultSymbol(symbols[rustTokStringContent])
	lexer.MarkEnd()
	return hasContent
}

// ---------------------------------------------------------------------------
// Raw string literals
// ---------------------------------------------------------------------------

func rustScanRawStringStart(s *rustScannerState, lexer *gotreesitter.ExternalLexer, symbols *[rustTokenCount]gotreesitter.Symbol) bool {
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

func rustScanRawStringContent(s *rustScannerState, lexer *gotreesitter.ExternalLexer, symbols *[rustTokenCount]gotreesitter.Symbol) bool {
	for {
		if lexer.Lookahead() == 0 { // EOF
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

func rustScanRawStringEnd(s *rustScannerState, lexer *gotreesitter.ExternalLexer, symbols *[rustTokenCount]gotreesitter.Symbol) bool {
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

func rustIsNumChar(ch rune) bool {
	return ch == '_' || unicode.IsDigit(ch)
}

func rustProcessFloatLiteral(lexer *gotreesitter.ExternalLexer, symbols *[rustTokenCount]gotreesitter.Symbol) bool {
	lexer.SetResultSymbol(symbols[rustTokFloatLiteral])

	lexer.Advance(false)
	for rustIsNumChar(lexer.Lookahead()) {
		lexer.Advance(false)
	}

	hasFraction := false
	hasExponent := false

	if lexer.Lookahead() == '.' {
		hasFraction = true
		lexer.Advance(false)
		if unicode.IsLetter(lexer.Lookahead()) {
			// The dot is followed by a letter: 1.max(2) => not a float but an integer.
			return false
		}
		if lexer.Lookahead() == '.' {
			return false
		}
		for rustIsNumChar(lexer.Lookahead()) {
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
		if !rustIsNumChar(lexer.Lookahead()) {
			return true
		}
		lexer.Advance(false)
		for rustIsNumChar(lexer.Lookahead()) {
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
	if !unicode.IsDigit(lexer.Lookahead()) {
		return true
	}
	for unicode.IsDigit(lexer.Lookahead()) {
		lexer.Advance(false)
	}

	lexer.MarkEnd()
	return true
}

// ---------------------------------------------------------------------------
// Line doc content
// ---------------------------------------------------------------------------

func rustProcessLineDocContent(lexer *gotreesitter.ExternalLexer, symbols *[rustTokenCount]gotreesitter.Symbol) bool {
	if !lexer.HasPreviousBytes("///") && !lexer.HasPreviousBytes("//!") {
		return false
	}
	lexer.SetResultSymbol(symbols[rustTokDocComment])
	for {
		if lexer.Lookahead() == 0 { // EOF
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

// blockCommentState tracks the state machine for nested block comment parsing.
type blockCommentState int

const (
	bcLeftForwardSlash blockCommentState = iota
	bcLeftAsterisk
	bcContinuing
)

func rustProcessBlockComment(lexer *gotreesitter.ExternalLexer, validSymbols []bool, symbols *[rustTokenCount]gotreesitter.Symbol) bool {
	first := lexer.Lookahead()

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
		for lexer.Lookahead() != 0 && nestingDepth != 0 {
			// Set first to the current lookahead as that is the second character
			// as we force an advance in the above code when we are checking if we
			// need to handle a block comment inner or outer doc comment signifier node.
			current := lexer.Lookahead()
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

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func rustValid(validSymbols []bool, idx int) bool {
	return idx >= 0 && idx < len(validSymbols) && validSymbols[idx]
}
