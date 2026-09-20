//go:build !grammar_subset || grammar_subset_kotlin

package grammarruntime

import (
	"unicode"

	gotreesitter "github.com/odvcencio/gotreesitter"
)

// External token indexes for the kotlin grammar.
//
// fwcd/tree-sitter-kotlin@1852ea17b7f6 (2.1 multi-dollar string interpolation)
// dropped the SAFE_NAV and IMPORT_LIST_DELIMITER externals: `?.` and the
// import-list terminator are lexed in-grammar now. Three externals were
// added: INTERPOLATION_EXPRESSION_START and INTERPOLATION_IDENTIFIER_START
// (the scanner now emits the interpolation opener itself, since the number
// of leading '$' signs that trigger interpolation is per-string), and
// BY_DELEGATION_HINT (a zero-emit context flag the scanner never returns as
// a token; its only job is to appear in validSymbols so scan_automatic_semicolon
// knows a bare `by` continues a delegation).
const (
	kotlinTokAutoSemicolon    = 0 // "_automatic_semicolon"
	kotlinTokMultilineComment = 1 // "multiline_comment"
	kotlinTokStringStart      = 2 // "_string_start"
	kotlinTokStringEnd        = 3 // "_string_end"
	kotlinTokStringContent    = 4 // "string_content"
	kotlinTokPrimaryCtorKW    = 5 // "_primary_constructor_keyword"
	kotlinTokImportDot        = 6 // "_import_dot"
	kotlinTokInterpExprStart  = 7 // "_interpolation_expression_start"
	kotlinTokInterpIdentStart = 8 // "_interpolation_identifier_start"
	kotlinTokByDelegationHint = 9 // "_by_delegation_hint"
	kotlinTokenCount          = 10
)

// Concrete symbol IDs from the generated kotlin grammar ExternalSymbols.
const (
	kotlinSymAutoSemicolon    gotreesitter.Symbol = 144
	kotlinSymMultilineComment gotreesitter.Symbol = 145
	kotlinSymStringStart      gotreesitter.Symbol = 146
	kotlinSymStringEnd        gotreesitter.Symbol = 147
	kotlinSymStringContent    gotreesitter.Symbol = 148
	kotlinSymPrimaryCtorKW    gotreesitter.Symbol = 149
	kotlinSymImportDot        gotreesitter.Symbol = 150
	kotlinSymInterpExprStart  gotreesitter.Symbol = 151
	kotlinSymInterpIdentStart gotreesitter.Symbol = 152
	kotlinSymByDelegationHint gotreesitter.Symbol = 153
)

var kotlinDefaultSymTable = [kotlinTokenCount]gotreesitter.Symbol{
	kotlinSymAutoSemicolon,
	kotlinSymMultilineComment,
	kotlinSymStringStart,
	kotlinSymStringEnd,
	kotlinSymStringContent,
	kotlinSymPrimaryCtorKW,
	kotlinSymImportDot,
	kotlinSymInterpExprStart,
	kotlinSymInterpIdentStart,
	kotlinSymByDelegationHint,
}

var kotlinExternalScannerSpec = ExternalScannerSpec{
	Language:       "kotlin",
	UpstreamRepo:   "https://github.com/fwcd/tree-sitter-kotlin",
	UpstreamCommit: "1852ea17b7f60fb3f9d84e0b1555d56b46b39fb1",
	SourceFiles: []ExternalScannerSourceFile{
		{Path: "src/grammar.json", SHA256: "56082f33c3a77b410c37e351e6668cf354f87fa3f6ef47a070018b9e8a2681ed"},
		{Path: "src/scanner.c", SHA256: "8a300c7da25290d5de076605fb46cc6b53b188d99aa9e8f34e928dbb7191935f"},
	},
	Externals: []string{
		"_automatic_semicolon",
		"multiline_comment",
		"_string_start",
		"_string_end",
		"string_content",
		"_primary_constructor_keyword",
		"_import_dot",
		"_interpolation_expression_start",
		"_interpolation_identifier_start",
		"_by_delegation_hint",
	},
}

func init() {
	RegisterExternalScannerSpec(kotlinExternalScannerSpec)
}

// kotlinDelimEntry stores one open string delimiter on the stack: the
// delimiter byte and the number of leading '$' signs required to trigger
// interpolation in that string (Kotlin 2.1 multi-dollar strings: 1 for an
// ordinary string or a single-'$'-prefixed string, 2 for $$"...", and so
// on). Exploits the fact that '"' (34) is even: triple-quoted delimiters
// are stored as the char value + 1 (odd).
type kotlinDelimEntry struct {
	ch        byte
	prefixLen byte
}

func (d kotlinDelimEntry) isTriple() bool { return d.ch&1 != 0 }
func (d kotlinDelimEntry) endChar() byte  { return d.ch &^ 1 }

// kotlinScannerState holds a stack of active string delimiters.
type kotlinScannerState struct {
	delimiters []kotlinDelimEntry
}

func (s *kotlinScannerState) pop() {
	if len(s.delimiters) == 0 {
		return
	}
	s.delimiters = s.delimiters[:len(s.delimiters)-1]
}

// KotlinExternalScanner implements gotreesitter.ExternalScanner for tree-sitter-kotlin.
//
// This is a Go port of the C external scanner from fwcd/tree-sitter-kotlin.
// The scanner handles 10 external tokens including automatic semicolon
// insertion (ASI), nested multiline comments, string start/end/content with
// multi-dollar interpolation support, primary constructor keyword detection,
// import path handling, and a by-delegation ASI hint.
type KotlinExternalScanner struct {
	symbols         [kotlinTokenCount]gotreesitter.Symbol
	externalToToken []int
}

func (KotlinExternalScanner) ExternalScannerForLanguage(lang *gotreesitter.Language) gotreesitter.ExternalScanner {
	s := KotlinExternalScanner{symbols: kotlinDefaultSymTable}
	s.externalToToken = bindExternalScannerSpec(lang, kotlinExternalScannerSpec, func(tokenIdx int, sym gotreesitter.Symbol) {
		s.symbols[tokenIdx] = sym
	})
	return s
}

func (KotlinExternalScanner) Create() any {
	return &kotlinScannerState{}
}

func (KotlinExternalScanner) Destroy(payload any) {}

// Serialize writes the delimiter stack as 2 bytes per entry: [delimiter,
// prefixLen]. Mirrors the C scanner's wire format after the 2.1 refresh.
func (KotlinExternalScanner) Serialize(payload any, buf []byte) int {
	s := payload.(*kotlinScannerState)
	n := len(s.delimiters) * 2
	if n > len(buf) {
		n = len(buf) - (len(buf) % 2)
	}
	pairs := n / 2
	for i := 0; i < pairs; i++ {
		buf[2*i] = s.delimiters[i].ch
		buf[2*i+1] = s.delimiters[i].prefixLen
	}
	return pairs * 2
}

// Deserialize restores the delimiter stack from 2-byte entries. An odd
// length is corrupted state; discard it and reset to empty, matching the C
// scanner's `length % 2 == 0` guard.
func (KotlinExternalScanner) Deserialize(payload any, buf []byte) {
	s := payload.(*kotlinScannerState)
	s.delimiters = s.delimiters[:0]
	if len(buf) == 0 || len(buf)%2 != 0 {
		return
	}
	for i := 0; i+1 < len(buf); i += 2 {
		s.delimiters = append(s.delimiters, kotlinDelimEntry{ch: buf[i], prefixLen: buf[i+1]})
	}
}

func (k KotlinExternalScanner) Scan(payload any, lexer *gotreesitter.ExternalLexer, validSymbols []bool) bool {
	s := payload.(*kotlinScannerState)
	symbols := k.symbolTable()
	if len(k.externalToToken) > 0 {
		var semanticValid [kotlinTokenCount]bool
		for externalIdx, valid := range validSymbols {
			if !valid || externalIdx >= len(k.externalToToken) {
				continue
			}
			tokenIdx := k.externalToToken[externalIdx]
			if tokenIdx >= 0 && tokenIdx < kotlinTokenCount {
				semanticValid[tokenIdx] = true
			}
		}
		validSymbols = semanticValid[:]
	}

	// ASI (automatic semicolon insertion).
	if kotlinValid(validSymbols, kotlinTokAutoSemicolon) {
		ret := kotlinScanAutoSemicolon(lexer, validSymbols, symbols)
		if ret {
			return ret
		}
	}

	// Import dot.
	if kotlinValid(validSymbols, kotlinTokImportDot) {
		if kotlinScanImportDot(lexer, symbols) {
			return true
		}
	}

	// Primary constructor keyword (outside strings), same-line case. The
	// cross-newline case is handled inside scan_automatic_semicolon.
	if kotlinValid(validSymbols, kotlinTokPrimaryCtorKW) &&
		!kotlinValid(validSymbols, kotlinTokStringContent) {
		for unicode.IsSpace(lexer.Lookahead()) {
			lexer.Advance(true)
		}
		if lexer.Lookahead() == 'c' && kotlinAdvanceMatchWord(lexer, "constructor") {
			lexer.MarkEnd()
			lexer.SetResultSymbol(symbols[kotlinTokPrimaryCtorKW])
			return true
		}
	}

	// String content, end, or interpolation start.
	if kotlinValid(validSymbols, kotlinTokStringContent) ||
		kotlinValid(validSymbols, kotlinTokInterpExprStart) ||
		kotlinValid(validSymbols, kotlinTokInterpIdentStart) {
		if kotlinScanStringContent(s, lexer, validSymbols, symbols) {
			return true
		}
	}

	// A string might follow after some whitespace, so we can't lookahead
	// until we get rid of it.
	for unicode.IsSpace(lexer.Lookahead()) {
		lexer.Advance(true)
	}

	// String start.
	if kotlinValid(validSymbols, kotlinTokStringStart) {
		if kotlinScanStringStart(s, lexer) {
			lexer.SetResultSymbol(symbols[kotlinTokStringStart])
			return true
		}
	}

	// Multiline comment.
	if kotlinValid(validSymbols, kotlinTokMultilineComment) {
		if kotlinScanMultilineComment(lexer, symbols) {
			return true
		}
	}

	return false
}

func (k KotlinExternalScanner) symbolTable() *[kotlinTokenCount]gotreesitter.Symbol {
	if k.symbols == ([kotlinTokenCount]gotreesitter.Symbol{}) {
		return &kotlinDefaultSymTable
	}
	return &k.symbols
}

// ---------------------------------------------------------------------------
// String scanning
// ---------------------------------------------------------------------------

func kotlinScanStringStart(s *kotlinScannerState, lexer *gotreesitter.ExternalLexer) bool {
	// Count leading '$' signs (the interpolation prefix). Capped at 255.
	// Regular strings with no prefix still use a single '$' as the trigger.
	var prefixLen byte
	for lexer.Lookahead() == '$' {
		lexer.Advance(false)
		if prefixLen < 255 {
			prefixLen++
		}
	}
	if prefixLen == 0 {
		prefixLen = 1
	}

	if lexer.Lookahead() != '"' {
		return false
	}
	lexer.Advance(false)
	lexer.MarkEnd()

	// Check for triple quote.
	count := 1
	for count < 3 && lexer.Lookahead() == '"' {
		lexer.Advance(false)
		count++
	}

	if count == 3 {
		lexer.MarkEnd()
		s.delimiters = append(s.delimiters, kotlinDelimEntry{ch: '"' + 1, prefixLen: prefixLen})
	} else {
		s.delimiters = append(s.delimiters, kotlinDelimEntry{ch: '"', prefixLen: prefixLen})
	}
	return true
}

func kotlinScanStringContent(s *kotlinScannerState, lexer *gotreesitter.ExternalLexer, validSymbols []bool, symbols *[kotlinTokenCount]gotreesitter.Symbol) bool {
	if len(s.delimiters) == 0 {
		return false
	}

	top := s.delimiters[len(s.delimiters)-1]
	endCh := rune(top.endChar())
	isTriple := top.isTriple()
	prefixLen := top.prefixLen
	hasContent := false

	for {
		la := lexer.Lookahead()

		// Go's ExternalLexer has no eof() primitive distinct from Lookahead
		// returning 0: a literal NUL byte in the source is indistinguishable
		// from real end-of-input here. Treat lookahead==0 as terminal, same
		// as this port already does at every other EOF check in this file.
		// This cannot infinite-loop (the C fix's DoS concern): we break
		// immediately instead of returning false-without-advancing.
		if la == 0 {
			break
		}

		if la == '$' {
			if hasContent {
				lexer.SetResultSymbol(symbols[kotlinTokStringContent])
				return true
			}
			// Kotlin 2.1 multi-dollar interpolation: in a string with
			// prefixLen N, exactly N consecutive '$' followed by
			// alpha/'_'/'{' triggers interpolation. Excess leading '$'
			// signs are literal string content.
			lexer.Advance(false)
			lexer.MarkEnd()
			var additionalDollars uint16
			for lexer.Lookahead() == '$' {
				lexer.Advance(false)
				additionalDollars++
			}
			totalDollars := 1 + additionalDollars
			next := lexer.Lookahead()
			if totalDollars >= uint16(prefixLen) && (unicode.IsLetter(next) || next == '_' || next == '{') {
				if totalDollars > uint16(prefixLen) {
					// Excess: emit first '$' as literal content; the mark
					// from right after it makes tree-sitter rewind there.
					lexer.SetResultSymbol(symbols[kotlinTokStringContent])
					return true
				}
				// Exact match: emit the interpolation start token.
				if additionalDollars > 0 {
					lexer.MarkEnd()
				}
				if kotlinValid(validSymbols, kotlinTokInterpExprStart) && lexer.Lookahead() == '{' {
					lexer.Advance(false)
					// Empty interpolation "${}" is invalid Kotlin. Refuse to
					// emit the token so the parser produces an ERROR node
					// instead of matching a zero-width expression.
					if lexer.Lookahead() == '}' {
						return false
					}
					lexer.MarkEnd()
					lexer.SetResultSymbol(symbols[kotlinTokInterpExprStart])
					return true
				}
				if kotlinValid(validSymbols, kotlinTokInterpIdentStart) &&
					(unicode.IsLetter(lexer.Lookahead()) || lexer.Lookahead() == '_') {
					lexer.SetResultSymbol(symbols[kotlinTokInterpIdentStart])
					return true
				}
				return false
			}
			// Not enough '$' signs or not followed by alpha/'{': all
			// consumed dollars are literal string content.
			if additionalDollars > 0 {
				lexer.MarkEnd()
			}
			lexer.SetResultSymbol(symbols[kotlinTokStringContent])
			return true
		}

		if la == '\\' {
			lexer.Advance(false)
			if lexer.Lookahead() == '$' {
				lexer.Advance(false)
				// Edge case: escaped $ at end of string.
				if lexer.Lookahead() == endCh {
					s.pop()
					lexer.Advance(false)
					lexer.MarkEnd()
					lexer.SetResultSymbol(symbols[kotlinTokStringEnd])
					return true
				}
			} else if isTriple && lexer.Lookahead() == endCh {
				// In triple-quoted strings, `\` is NOT an escape character.
				// Don't advance past the quote; let the next iteration
				// handle it as a potential closing delimiter.
				hasContent = true
				continue
			}
		} else if la == endCh {
			if isTriple {
				lexer.MarkEnd()
				for count := 1; count < 3; count++ {
					lexer.Advance(false)
					if lexer.Lookahead() != endCh {
						lexer.MarkEnd()
						lexer.SetResultSymbol(symbols[kotlinTokStringContent])
						return true
					}
				}
				if hasContent && lexer.Lookahead() == endCh {
					lexer.SetResultSymbol(symbols[kotlinTokStringContent])
					return true
				}
				lexer.SetResultSymbol(symbols[kotlinTokStringEnd])
				lexer.MarkEnd()
				for lexer.Lookahead() == endCh {
					lexer.Advance(false)
					lexer.MarkEnd()
				}
				s.pop()
				return true
			}

			if hasContent {
				lexer.MarkEnd()
				lexer.SetResultSymbol(symbols[kotlinTokStringContent])
				return true
			}
			s.pop()
			lexer.Advance(false)
			lexer.MarkEnd()
			lexer.SetResultSymbol(symbols[kotlinTokStringEnd])
			return true
		}

		lexer.Advance(false)
		hasContent = true
	}

	return false
}

// ---------------------------------------------------------------------------
// Multiline comment
// ---------------------------------------------------------------------------

func kotlinScanMultilineComment(lexer *gotreesitter.ExternalLexer, symbols *[kotlinTokenCount]gotreesitter.Symbol) bool {
	if lexer.Lookahead() != '/' {
		return false
	}
	lexer.Advance(false)
	if lexer.Lookahead() != '*' {
		return false
	}
	lexer.Advance(false)

	afterStar := false
	depth := 1

	for {
		ch := lexer.Lookahead()
		switch ch {
		case '*':
			lexer.Advance(false)
			afterStar = true
		case '/':
			lexer.Advance(false)
			if afterStar {
				afterStar = false
				depth--
				if depth == 0 {
					lexer.MarkEnd()
					lexer.SetResultSymbol(symbols[kotlinTokMultilineComment])
					return true
				}
			} else {
				afterStar = false
				if lexer.Lookahead() == '*' {
					depth++
					lexer.Advance(false)
				}
			}
		case 0:
			// EOF (see the NUL-vs-EOF note in kotlinScanStringContent: Go
			// cannot tell a literal NUL byte from real end-of-input here).
			// Accept unterminated comments rather than rejecting them.
			lexer.MarkEnd()
			lexer.SetResultSymbol(symbols[kotlinTokMultilineComment])
			return true
		default:
			lexer.Advance(false)
			afterStar = false
		}
	}
}

// kotlinSkipWhitespaceAndComments skips whitespace and comments (// and
// nested /* */) using skip, so the consumed bytes never extend the current
// token's span. Returns false when a bare '/' is found (not a comment).
func kotlinSkipWhitespaceAndComments(lexer *gotreesitter.ExternalLexer) bool {
	for {
		for unicode.IsSpace(lexer.Lookahead()) {
			lexer.Advance(true)
		}
		if lexer.Lookahead() != '/' {
			return true
		}
		lexer.Advance(true)
		switch lexer.Lookahead() {
		case '/':
			lexer.Advance(true)
			for {
				la := lexer.Lookahead()
				if la == '\n' || la == '\r' || la == 0 {
					break
				}
				lexer.Advance(true)
			}
		case '*':
			lexer.Advance(true)
			depth := 1
			for depth > 0 {
				la := lexer.Lookahead()
				if la == 0 {
					break
				}
				switch la {
				case '*':
					lexer.Advance(true)
					if lexer.Lookahead() == '/' {
						lexer.Advance(true)
						depth--
					}
				case '/':
					lexer.Advance(true)
					if lexer.Lookahead() == '*' {
						lexer.Advance(true)
						depth++
					}
				default:
					lexer.Advance(true)
				}
			}
		default:
			return false
		}
	}
}

// kotlinFollowedByArrow peeks past optional whitespace and comments for
// "->", after scan_for_word matched "else". If found, this is a when-entry's
// `else ->`, not an if-else.
func kotlinFollowedByArrow(lexer *gotreesitter.ExternalLexer) bool {
	if !kotlinSkipWhitespaceAndComments(lexer) {
		return false
	}
	if lexer.Lookahead() != '-' {
		return false
	}
	lexer.Advance(true)
	return lexer.Lookahead() == '>'
}

// ---------------------------------------------------------------------------
// Automatic semicolon insertion
// ---------------------------------------------------------------------------

func kotlinScanAutoSemicolon(lexer *gotreesitter.ExternalLexer, validSymbols []bool, symbols *[kotlinTokenCount]gotreesitter.Symbol) bool {
	lexer.MarkEnd()
	lexer.SetResultSymbol(symbols[kotlinTokAutoSemicolon])

	// Check for explicit semicolons and newlines.
	sameLine := true
	for {
		ch := lexer.Lookahead()
		if ch == 0 { // EOF — always insert ASI.
			return true
		}
		if ch == ';' {
			lexer.Advance(false)
			lexer.MarkEnd()
			return true
		}
		if !unicode.IsSpace(ch) {
			break
		}
		if ch == '\n' || ch == '\r' {
			lexer.Advance(true)
			if ch == '\r' && lexer.Lookahead() == '\n' {
				lexer.Advance(true)
			}
			sameLine = false
			break
		}
		lexer.Advance(true)
	}

	// Skip remaining whitespace.
	for unicode.IsSpace(lexer.Lookahead()) {
		lexer.Advance(true)
	}

	if sameLine {
		ch := lexer.Lookahead()
		if ch == 'i' && kotlinScanWord(lexer, "import") {
			return true
		}
		if ch == ';' {
			lexer.Advance(false)
			lexer.MarkEnd()
			return true
		}
		return false
	}

	// After a newline: check if the next token is a continuation.
	ch := lexer.Lookahead()
	switch ch {
	case ',', '.', ':', '*', '%', '>', '<', '=',
		'{', '[', '(', '?', '|', '&':
		return false

	// Handle `/` — could be division, a line comment, or a block comment.
	// Division suppresses ASI. A line comment is skipped and the following
	// real token decides ASI. A block comment is read through, then either
	// produced as MULTILINE_COMMENT (if what follows continues the
	// statement) or the ASI is inserted at the pre-comment position.
	case '/':
		lexer.Advance(false)
		if lexer.Lookahead() == '/' {
			lexer.Advance(true)
			for {
				la := lexer.Lookahead()
				if la == '\n' || la == '\r' || la == 0 {
					break
				}
				lexer.Advance(true)
			}
			if !kotlinSkipWhitespaceAndComments(lexer) {
				return false
			}
			switch lexer.Lookahead() {
			case '.', ',', ':', '*', '%', '>', '<', '=', '{', '[', '(', '?', '|', '&', '/':
				return false
			case '!':
				lexer.Advance(true)
				return lexer.Lookahead() != '='
			case 'e':
				if kotlinScanWord(lexer, "else") {
					return kotlinFollowedByArrow(lexer)
				}
				return true
			case 'a':
				return !kotlinScanWord(lexer, "as")
			case 'w':
				return !kotlinScanWord(lexer, "where")
			case 'c':
				return !kotlinScanWord(lexer, "catch")
			case 'b':
				return !(kotlinValid(validSymbols, kotlinTokByDelegationHint) && kotlinScanWord(lexer, "by"))
			case 'f':
				return !kotlinScanWord(lexer, "finally")
			default:
				return true
			}
		} else if lexer.Lookahead() == '*' {
			lexer.Advance(false)
			nestingDepth := 1
			afterStar := false
			for nestingDepth > 0 {
				la := lexer.Lookahead()
				if la == 0 {
					// Unterminated block comment at EOF — produce it.
					lexer.MarkEnd()
					lexer.SetResultSymbol(symbols[kotlinTokMultilineComment])
					return true
				}
				switch la {
				case '*':
					lexer.Advance(false)
					afterStar = true
				case '/':
					lexer.Advance(false)
					if afterStar {
						afterStar = false
						nestingDepth--
					} else {
						if lexer.Lookahead() == '*' {
							nestingDepth++
							lexer.Advance(false)
						}
						afterStar = false
					}
				default:
					lexer.Advance(false)
					afterStar = false
				}
			}
			for unicode.IsSpace(lexer.Lookahead()) {
				lexer.Advance(true)
			}
			// For keyword checks, mark_end must happen BEFORE
			// scan_for_word/skip, because those advance past the keyword;
			// marking after would swallow the keyword into the comment span.
			switch lexer.Lookahead() {
			case '.', ',', ':', '%', '>', '<', '=', '{', '[', '(', '?', '|', '&', '/', '*':
				lexer.MarkEnd()
				lexer.SetResultSymbol(symbols[kotlinTokMultilineComment])
				return true
			case '!':
				lexer.MarkEnd()
				lexer.Advance(true)
				if lexer.Lookahead() == '=' {
					lexer.SetResultSymbol(symbols[kotlinTokMultilineComment])
					return true
				}
				return true
			case 'e':
				lexer.MarkEnd()
				if kotlinScanWord(lexer, "else") {
					if kotlinFollowedByArrow(lexer) {
						return true
					}
					lexer.SetResultSymbol(symbols[kotlinTokMultilineComment])
					return true
				}
				return true
			case 'a':
				lexer.MarkEnd()
				if kotlinScanWord(lexer, "as") {
					lexer.SetResultSymbol(symbols[kotlinTokMultilineComment])
					return true
				}
				return true
			case 'w':
				lexer.MarkEnd()
				if kotlinScanWord(lexer, "where") {
					lexer.SetResultSymbol(symbols[kotlinTokMultilineComment])
					return true
				}
				return true
			case 'b':
				if kotlinValid(validSymbols, kotlinTokByDelegationHint) {
					lexer.MarkEnd()
					if kotlinScanWord(lexer, "by") {
						lexer.SetResultSymbol(symbols[kotlinTokMultilineComment])
						return true
					}
				}
				return true
			default:
				return true
			}
		}
		// Bare `/` (not `//` or `/*`) — division. No ASI.
		return false

	case '+', '-':
		return true

	case '!':
		lexer.Advance(true)
		return lexer.Lookahead() != '='

	// Don't insert a semicolon before 'by' in delegation contexts. Gated on
	// kotlinTokByDelegationHint so `by` remains a usable soft-keyword
	// identifier in non-delegation positions.
	case 'b':
		return !(kotlinValid(validSymbols, kotlinTokByDelegationHint) && kotlinScanWord(lexer, "by"))

	// Don't insert a semicolon before an else, unless it's followed by
	// "->" (a when-entry's else, not an if-else).
	case 'e':
		if !kotlinScanWord(lexer, "else") {
			return true
		}
		return kotlinFollowedByArrow(lexer)

	case 'a':
		return !kotlinScanWord(lexer, "as")

	case 'w':
		return !kotlinScanWord(lexer, "where")

	case 'i':
		if kotlinValid(validSymbols, kotlinTokPrimaryCtorKW) &&
			!kotlinValid(validSymbols, kotlinTokStringContent) &&
			kotlinCheckModifierThenConstructor(lexer) {
			return false
		}
		return true

	case 'p':
		if kotlinValid(validSymbols, kotlinTokPrimaryCtorKW) &&
			!kotlinValid(validSymbols, kotlinTokStringContent) &&
			kotlinCheckModifierThenConstructor(lexer) {
			return false
		}
		return true

	case 'c':
		if kotlinValid(validSymbols, kotlinTokPrimaryCtorKW) &&
			!kotlinValid(validSymbols, kotlinTokStringContent) {
			if kotlinAdvanceMatchWord(lexer, "constructor") {
				lexer.SetResultSymbol(symbols[kotlinTokPrimaryCtorKW])
				lexer.MarkEnd()
				return true
			}
			// If constructor didn't match, we've advanced past some chars.
			// Can't reliably check 'catch' now. Just insert ASI.
			return true
		}
		// Not in constructor context — check for 'catch'.
		return !kotlinScanWord(lexer, "catch")

	// Don't insert a semicolon before finally (continues try_expression).
	case 'f':
		return !kotlinScanWord(lexer, "finally")

	// Don't insert a semicolon before an annotation that precedes
	// 'constructor', e.g. `class Foo\n@Bar\nconstructor(...)`.
	case '@':
		if kotlinValid(validSymbols, kotlinTokPrimaryCtorKW) &&
			!kotlinValid(validSymbols, kotlinTokStringContent) &&
			kotlinCheckAnnotationThenConstructor(lexer) {
			return false
		}
		return true

	case ';':
		lexer.Advance(false)
		lexer.MarkEnd()
		return true

	default:
		return true
	}
}

// ---------------------------------------------------------------------------
// Import handling
// ---------------------------------------------------------------------------

func kotlinScanImportDot(lexer *gotreesitter.ExternalLexer, symbols *[kotlinTokenCount]gotreesitter.Symbol) bool {
	if lexer.Lookahead() != '.' {
		return false
	}
	lexer.MarkEnd()
	lexer.Advance(false)

	foundNewline := false
	for unicode.IsSpace(lexer.Lookahead()) {
		if lexer.Lookahead() == '\n' || lexer.Lookahead() == '\r' {
			foundNewline = true
		}
		lexer.Advance(true)
	}

	if foundNewline && lexer.Lookahead() == 'i' && kotlinScanWord(lexer, "import") {
		lexer.SetResultSymbol(symbols[kotlinTokAutoSemicolon])
		return true
	}

	lexer.SetResultSymbol(symbols[kotlinTokImportDot])
	lexer.MarkEnd()
	return true
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func kotlinIsWordChar(ch rune) bool {
	return unicode.IsLetter(ch) || unicode.IsDigit(ch) || ch == '_'
}

// kotlinScanWord checks if the input starts with the given word followed
// by a non-word character. The first character is assumed already verified
// by the caller (a switch on Lookahead) and is skipped unchecked; the rest
// of the word is verified. Consumes characters via skip.
func kotlinScanWord(lexer *gotreesitter.ExternalLexer, word string) bool {
	lexer.Advance(true) // skip the first char (already verified by caller)
	for i := 1; i < len(word); i++ {
		if lexer.Lookahead() != rune(word[i]) {
			return false
		}
		lexer.Advance(true)
	}
	return !kotlinIsWordChar(lexer.Lookahead())
}

// kotlinCheckWord checks if the input starts with the given word (from the
// first character) followed by a non-word character. Consumes characters
// via skip, so a speculative check never extends the current token span.
// Mirrors the C scanner's check_word.
func kotlinCheckWord(lexer *gotreesitter.ExternalLexer, word string) bool {
	for i := 0; i < len(word); i++ {
		if lexer.Lookahead() != rune(word[i]) {
			return false
		}
		lexer.Advance(true)
	}
	return !kotlinIsWordChar(lexer.Lookahead())
}

// kotlinAdvanceMatchWord checks if the input starts with the given word
// followed by a non-word character, like kotlinCheckWord, but consumes
// matched characters via non-skip advance so the match becomes part of the
// emitted token when the caller marks the end afterward. Mirrors the C
// scanner's inline "constructor" matching loops.
func kotlinAdvanceMatchWord(lexer *gotreesitter.ExternalLexer, word string) bool {
	matched := true
	for i := 0; i < len(word); i++ {
		if lexer.Lookahead() != rune(word[i]) {
			matched = false
			break
		}
		lexer.Advance(false)
	}
	return matched && !kotlinIsWordChar(lexer.Lookahead())
}

// kotlinCheckModifierThenConstructor checks if the input is a visibility
// modifier followed by whitespace and "constructor".
func kotlinCheckModifierThenConstructor(lexer *gotreesitter.ExternalLexer) bool {
	var word []byte
	for kotlinIsWordChar(lexer.Lookahead()) && len(word) < 20 {
		word = append(word, byte(lexer.Lookahead()))
		lexer.Advance(true)
	}

	w := string(word)
	if w != "public" && w != "private" && w != "protected" && w != "internal" {
		return false
	}

	for lexer.Lookahead() == ' ' || lexer.Lookahead() == '\t' {
		lexer.Advance(true)
	}

	return kotlinCheckWord(lexer, "constructor")
}

// kotlinCheckAnnotationThenConstructor looks ahead past one or more
// annotations (e.g. @Bar, @com.example.Bar, @Bar(x=1)) and an optional
// visibility modifier, then checks for 'constructor'. All characters are
// consumed via skip so nothing affects token boundaries.
func kotlinCheckAnnotationThenConstructor(lexer *gotreesitter.ExternalLexer) bool {
	for lexer.Lookahead() == '@' {
		lexer.Advance(true)
		if !kotlinIsWordChar(lexer.Lookahead()) {
			return false
		}
		for kotlinIsWordChar(lexer.Lookahead()) {
			lexer.Advance(true)
		}
		for lexer.Lookahead() == '.' {
			lexer.Advance(true)
			if !kotlinIsWordChar(lexer.Lookahead()) {
				break
			}
			for kotlinIsWordChar(lexer.Lookahead()) {
				lexer.Advance(true)
			}
		}
		if lexer.Lookahead() == '(' {
			depth := 1
			lexer.Advance(true)
			for depth > 0 && lexer.Lookahead() != 0 {
				if lexer.Lookahead() == '"' {
					lexer.Advance(true)
					for lexer.Lookahead() != '"' && lexer.Lookahead() != 0 {
						if lexer.Lookahead() == '\\' {
							lexer.Advance(true)
						}
						lexer.Advance(true)
					}
					if lexer.Lookahead() == '"' {
						lexer.Advance(true)
					}
				} else {
					if lexer.Lookahead() == '(' {
						depth++
					} else if lexer.Lookahead() == ')' {
						depth--
					}
					lexer.Advance(true)
				}
			}
		}
		for unicode.IsSpace(lexer.Lookahead()) {
			lexer.Advance(true)
		}
	}
	if kotlinIsWordChar(lexer.Lookahead()) && lexer.Lookahead() != 'c' {
		return kotlinCheckModifierThenConstructor(lexer)
	}
	return kotlinCheckWord(lexer, "constructor")
}

func kotlinValid(validSymbols []bool, idx int) bool {
	return idx >= 0 && idx < len(validSymbols) && validSymbols[idx]
}
