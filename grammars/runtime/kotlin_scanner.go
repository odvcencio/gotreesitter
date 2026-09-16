//go:build !grammar_subset || grammar_subset_kotlin

package grammarruntime

import (
	"unicode"

	gotreesitter "github.com/odvcencio/gotreesitter"
)

// External token indexes for the kotlin grammar.
const (
	kotlinTokAutoSemicolon    = 0 // "_automatic_semicolon"
	kotlinTokImportListDelim  = 1 // "_import_list_delimiter"
	kotlinTokSafeNav          = 2 // "\\?."
	kotlinTokMultilineComment = 3 // "multiline_comment"
	kotlinTokStringStart      = 4 // "_string_start"
	kotlinTokStringEnd        = 5 // "_string_end"
	kotlinTokStringContent    = 6 // "string_content"
	kotlinTokPrimaryCtorKW    = 7 // "_primary_constructor_keyword"
	kotlinTokImportDot        = 8 // "_import_dot"
	kotlinTokenCount          = 9
)

// Concrete symbol IDs from the generated kotlin grammar ExternalSymbols.
const (
	kotlinSymAutoSemicolon    gotreesitter.Symbol = 145
	kotlinSymImportListDelim  gotreesitter.Symbol = 146
	kotlinSymSafeNav          gotreesitter.Symbol = 147
	kotlinSymMultilineComment gotreesitter.Symbol = 148
	kotlinSymStringStart      gotreesitter.Symbol = 149
	kotlinSymStringEnd        gotreesitter.Symbol = 150
	kotlinSymStringContent    gotreesitter.Symbol = 151
	kotlinSymPrimaryCtorKW    gotreesitter.Symbol = 152
	kotlinSymImportDot        gotreesitter.Symbol = 153
)

var kotlinDefaultSymTable = [kotlinTokenCount]gotreesitter.Symbol{
	kotlinSymAutoSemicolon,
	kotlinSymImportListDelim,
	kotlinSymSafeNav,
	kotlinSymMultilineComment,
	kotlinSymStringStart,
	kotlinSymStringEnd,
	kotlinSymStringContent,
	kotlinSymPrimaryCtorKW,
	kotlinSymImportDot,
}

var kotlinExternalScannerSpec = ExternalScannerSpec{
	Language:       "kotlin",
	UpstreamRepo:   "https://github.com/fwcd/tree-sitter-kotlin",
	UpstreamCommit: "cbed96ab13dbc082eeeb2e8333c342a62829c29d",
	SourceFiles: []ExternalScannerSourceFile{
		{Path: "src/grammar.json", SHA256: "f29862b076453c9dca62e28cf0063345b90af0c3cb80535676b4960a60fe459a"},
		{Path: "src/scanner.c", SHA256: "cc0e641013fc19e9e37e4d7bc690abc573a6e10f54f0e8aabf816ac07f6324a4"},
	},
	Externals: []string{
		"_automatic_semicolon",
		"_import_list_delimiter",
		"safe_nav",
		"multiline_comment",
		"_string_start",
		"_string_end",
		"string_content",
		"_primary_constructor_keyword",
		"_import_dot",
	},
}

func init() {
	RegisterExternalScannerSpec(kotlinExternalScannerSpec)
}

// kotlinDelimiter stores a string delimiter on the stack. Exploits the
// fact that '"' (34) is even: triple-quoted delimiters are stored as
// the char value + 1 (odd), single-quoted as the char value (even).
type kotlinDelimiter byte

func (d kotlinDelimiter) isTriple() bool { return d&1 != 0 }
func (d kotlinDelimiter) endChar() byte  { return byte(d &^ 1) }

// kotlinScannerState holds a stack of active string delimiters.
type kotlinScannerState struct {
	delimiters []kotlinDelimiter
}

// KotlinExternalScanner implements gotreesitter.ExternalScanner for tree-sitter-kotlin.
//
// This is a Go port of the C external scanner from fwcd/tree-sitter-kotlin.
// The scanner handles 9 external tokens including automatic semicolon
// insertion (ASI), safe navigation (?.), nested multiline comments,
// string start/end/content with interpolation support, primary constructor
// keyword detection, and import path handling.
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

func (KotlinExternalScanner) Serialize(payload any, buf []byte) int {
	s := payload.(*kotlinScannerState)
	n := len(s.delimiters)
	if n > len(buf) {
		n = len(buf)
	}
	for i := 0; i < n; i++ {
		buf[i] = byte(s.delimiters[i])
	}
	return n
}

func (KotlinExternalScanner) Deserialize(payload any, buf []byte) {
	s := payload.(*kotlinScannerState)
	s.delimiters = s.delimiters[:0]
	for _, b := range buf {
		s.delimiters = append(s.delimiters, kotlinDelimiter(b))
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
		if !ret && kotlinValid(validSymbols, kotlinTokSafeNav) && lexer.Lookahead() == '?' {
			return kotlinScanSafeNav(lexer, symbols)
		}
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

	// Primary constructor keyword (outside strings).
	if kotlinValid(validSymbols, kotlinTokPrimaryCtorKW) &&
		!kotlinValid(validSymbols, kotlinTokStringContent) {
		for unicode.IsSpace(lexer.Lookahead()) {
			lexer.Advance(true)
		}
		if lexer.Lookahead() == 'c' && kotlinCheckWord(lexer, "constructor") {
			lexer.MarkEnd()
			lexer.SetResultSymbol(symbols[kotlinTokPrimaryCtorKW])
			return true
		}
	}

	// Import list delimiter.
	if kotlinValid(validSymbols, kotlinTokImportListDelim) {
		return kotlinScanImportListDelim(lexer, symbols)
	}

	// String content.
	if kotlinValid(validSymbols, kotlinTokStringContent) {
		if kotlinScanStringContent(s, lexer, symbols) {
			return true
		}
	}

	// Skip whitespace before remaining checks.
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

	// Safe navigation.
	if kotlinValid(validSymbols, kotlinTokSafeNav) {
		return kotlinScanSafeNav(lexer, symbols)
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
		s.delimiters = append(s.delimiters, kotlinDelimiter('"'+1))
	} else {
		s.delimiters = append(s.delimiters, kotlinDelimiter('"'))
	}
	return true
}

func kotlinScanStringContent(s *kotlinScannerState, lexer *gotreesitter.ExternalLexer, symbols *[kotlinTokenCount]gotreesitter.Symbol) bool {
	if len(s.delimiters) == 0 {
		return false
	}

	top := s.delimiters[len(s.delimiters)-1]
	endCh := rune(top.endChar())
	isTriple := top.isTriple()
	hasContent := false

	for lexer.Lookahead() != 0 {
		ch := lexer.Lookahead()

		if ch == '$' {
			if hasContent {
				lexer.MarkEnd()
				lexer.SetResultSymbol(symbols[kotlinTokStringContent])
				return true
			}
			// Check if this starts an interpolation.
			lexer.Advance(false)
			next := lexer.Lookahead()
			if unicode.IsLetter(next) || next == '{' {
				// It's an interpolation — decline so the grammar handles it.
				return false
			}
			// Just a literal $ in the string.
			lexer.MarkEnd()
			lexer.SetResultSymbol(symbols[kotlinTokStringContent])
			return true
		}

		if ch == '\\' {
			lexer.Advance(false)
			// Escaped $ — consume it as content to avoid the interpolation check.
			if lexer.Lookahead() == '$' {
				lexer.Advance(false)
				// Edge case: escaped $ at end of string.
				if lexer.Lookahead() == endCh {
					s.delimiters = s.delimiters[:len(s.delimiters)-1]
					lexer.Advance(false)
					lexer.MarkEnd()
					lexer.SetResultSymbol(symbols[kotlinTokStringEnd])
					return true
				}
			}
			hasContent = true
			continue
		}

		if ch == endCh {
			if isTriple {
				lexer.MarkEnd()
				// Count consecutive quotes.
				count := 0
				for count < 3 && lexer.Lookahead() == endCh {
					lexer.Advance(false)
					count++
				}
				if count < 3 {
					// Not enough quotes for closing triple — it's content.
					lexer.MarkEnd()
					lexer.SetResultSymbol(symbols[kotlinTokStringContent])
					return true
				}
				// If we had content before the quotes, emit it first.
				if hasContent && lexer.Lookahead() == endCh {
					lexer.SetResultSymbol(symbols[kotlinTokStringContent])
					return true
				}
				// Consume any trailing extra quotes.
				lexer.MarkEnd()
				for lexer.Lookahead() == endCh {
					lexer.Advance(false)
					lexer.MarkEnd()
				}
				s.delimiters = s.delimiters[:len(s.delimiters)-1]
				lexer.SetResultSymbol(symbols[kotlinTokStringEnd])
				return true
			}

			// Single-quoted string.
			if hasContent {
				lexer.MarkEnd()
				lexer.SetResultSymbol(symbols[kotlinTokStringContent])
				return true
			}
			s.delimiters = s.delimiters[:len(s.delimiters)-1]
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
		case 0: // EOF — accept unterminated comments (matches C behavior).
			lexer.MarkEnd()
			lexer.SetResultSymbol(symbols[kotlinTokMultilineComment])
			return true
		default:
			lexer.Advance(false)
			afterStar = false
		}
	}
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

	// Skip remaining whitespace and comments.
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

	case '/':
		lexer.Advance(true)
		// Line or block comment after newline = ASI.
		return lexer.Lookahead() == '/' || lexer.Lookahead() == '*'

	case '+', '-':
		return true

	case '!':
		lexer.Advance(true)
		return lexer.Lookahead() != '='

	case 'e':
		return !kotlinScanWord(lexer, "else")

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
			!kotlinValid(validSymbols, kotlinTokStringContent) &&
			kotlinCheckWord(lexer, "constructor") {
			lexer.MarkEnd()
			lexer.SetResultSymbol(symbols[kotlinTokPrimaryCtorKW])
			return true
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
// Safe navigation (?.)
// ---------------------------------------------------------------------------

func kotlinScanSafeNav(lexer *gotreesitter.ExternalLexer, symbols *[kotlinTokenCount]gotreesitter.Symbol) bool {
	lexer.SetResultSymbol(symbols[kotlinTokSafeNav])
	lexer.MarkEnd()

	for unicode.IsSpace(lexer.Lookahead()) {
		lexer.Advance(true)
	}

	if lexer.Lookahead() != '?' {
		return false
	}
	lexer.Advance(false)

	for unicode.IsSpace(lexer.Lookahead()) {
		lexer.Advance(true)
	}

	if lexer.Lookahead() != '.' {
		return false
	}
	lexer.Advance(false)
	lexer.MarkEnd()
	return true
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

func kotlinScanImportListDelim(lexer *gotreesitter.ExternalLexer, symbols *[kotlinTokenCount]gotreesitter.Symbol) bool {
	lexer.SetResultSymbol(symbols[kotlinTokImportListDelim])
	lexer.MarkEnd()

	if lexer.Lookahead() == 0 {
		return true
	}

	if !kotlinScanLineSep(lexer) {
		return false
	}

	if kotlinScanLineSep(lexer) {
		lexer.MarkEnd()
		return true
	}

	for {
		ch := lexer.Lookahead()
		if ch == ' ' || ch == '\t' || ch == '\v' {
			lexer.Advance(false)
			continue
		}
		if ch == 'i' {
			return !kotlinScanWord(lexer, "import")
		}
		return true
	}
}

func kotlinScanLineSep(lexer *gotreesitter.ExternalLexer) bool {
	state := 0
	for {
		ch := lexer.Lookahead()
		switch ch {
		case ' ', '\t', '\v':
			lexer.Advance(false)
		case '\n':
			lexer.Advance(false)
			return true
		case '\r':
			if state == 1 {
				return true
			}
			state = 1
			lexer.Advance(false)
		default:
			return state == 1
		}
	}
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func kotlinIsWordChar(ch rune) bool {
	return unicode.IsLetter(ch) || unicode.IsDigit(ch) || ch == '_'
}

// kotlinScanWord checks if the input starts with the given word followed
// by a non-word character. Consumes characters via skip.
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

// kotlinCheckWord checks if the input starts with the given word followed
// by a non-word character. Consumes characters via advance (non-skip).
func kotlinCheckWord(lexer *gotreesitter.ExternalLexer, word string) bool {
	for i := 0; i < len(word); i++ {
		if lexer.Lookahead() != rune(word[i]) {
			return false
		}
		lexer.Advance(false)
	}
	return !kotlinIsWordChar(lexer.Lookahead())
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

func kotlinValid(validSymbols []bool, idx int) bool {
	return idx >= 0 && idx < len(validSymbols) && validSymbols[idx]
}
