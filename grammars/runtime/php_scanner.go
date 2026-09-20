//go:build !grammar_subset || grammar_subset_php

package grammarruntime

import (
	"encoding/binary"
	"unicode"

	gotreesitter "github.com/odvcencio/gotreesitter"
)

// External token indexes for the PHP grammar. This is the external index
// (the position of the token in the grammar's `externals: [...]` list),
// which is exactly what tree-sitter's `valid_symbols` array and C's
// result_symbol enum are indexed by. The external index is stable across a
// blob regen as long as the externals list itself does not reorder;
// concrete numeric gotreesitter.Symbol IDs are NOT stable (they shift
// whenever the grammar's total symbol count changes), so this scanner never
// hardcodes them -- see phpDefaultSymTable below.
const (
	phpTokAutoSemicolon                   = 0
	phpTokEncapsedStringChars             = 1
	phpTokEncapsedStringCharsAfterVar     = 2
	phpTokExecutionStringChars            = 3
	phpTokExecutionStringCharsAfterVar    = 4
	phpTokEncapsedStringCharsHeredoc      = 5
	phpTokEncapsedStringCharsAfterVarHdoc = 6
	phpTokEOF                             = 7
	phpTokHeredocStart                    = 8
	phpTokHeredocEnd                      = 9
	phpTokNowdocString                    = 10
	// phpTokSentinelError is a genuine external (upstream `sentinel_error`)
	// that the scanner only ever reads via isValid (an error-recovery probe
	// signaling the parser is in error recovery), never as a SetResultSymbol
	// argument -- see the "always decline" check at the top of Scan.
	phpTokSentinelError = 11
	phpTokenCount       = 12
)

// phpDefaultSymTable records the concrete gotreesitter.Symbol IDs the
// currently shipped php.bin assigns to each external, in phpTok* order. It
// exists only as a pre-bind fallback (and as an independent value to compare
// a real bind against in tests); ExternalScannerForLanguage below overwrites
// it with values read from the actual loaded Language at bind time, which is
// what the scanner must do to survive a future blob regen that renumbers
// absolute symbol IDs without touching the externals list order.
var phpDefaultSymTable = [phpTokenCount]gotreesitter.Symbol{
	185, // _automatic_semicolon
	186, // encapsed_string_chars (display: string_content)
	187, // encapsed_string_chars_after_variable (display: string_content)
	188, // execution_string_chars (display: string_content)
	189, // execution_string_chars_after_variable (display: string_content)
	190, // encapsed_string_chars_heredoc (display: string_content)
	191, // encapsed_string_chars_after_variable_heredoc (display: string_content)
	192, // _eof
	193, // heredoc_start
	194, // heredoc_end
	195, // nowdoc_string
	196, // sentinel_error
}

// phpExternalScannerSpec records the source contract for this hand-written
// port, so updater tooling can tell a grammar-only upstream change apart
// from one that also touches the external scanner or its token list. Its
// Externals list is also the binding source for ExternalScannerForLanguage:
// index i here is scanner token index i (phpTok* order).
var phpExternalScannerSpec = ExternalScannerSpec{
	Language:       "php",
	UpstreamRepo:   "https://github.com/tree-sitter/tree-sitter-php",
	UpstreamCommit: "3fda2fb9577166c6399834917f9844f30370beea",
	SourceFiles: []ExternalScannerSourceFile{
		{Path: "php/src/grammar.json", SHA256: "4dd996a822c6d9130db3541104169cc45db90f03ec10627dd1a1aa3197916e7a"},
		{Path: "php/src/scanner.c", SHA256: "58c92cafe4ebda509c3ad3864fa6fc0e9877bbac26e17a03d23ea2101c291ad5"},
	},
	Externals: []string{
		"_automatic_semicolon",
		"encapsed_string_chars",
		"encapsed_string_chars_after_variable",
		"execution_string_chars",
		"execution_string_chars_after_variable",
		"encapsed_string_chars_heredoc",
		"encapsed_string_chars_after_variable_heredoc",
		"_eof",
		"heredoc_start",
		"heredoc_end",
		"nowdoc_string",
		"sentinel_error",
	},
}

func init() {
	RegisterExternalScannerSpec(phpExternalScannerSpec)
}

type phpHeredoc struct {
	endWordIndentAllowed bool
	word                 []rune
}

type phpState struct {
	heredocs []phpHeredoc
}

// PhpExternalScanner handles heredocs, encapsed strings, auto-semicolons, and
// EOF for PHP.
//
// symbols holds the concrete gotreesitter.Symbol each external index maps to
// in the Language this instance was bound to (see ExternalScannerForLanguage).
// The scanner never hardcodes an absolute Symbol value: a blob regen can
// renumber the grammar's absolute symbol IDs without touching the externals
// list order, and a scanner that still called SetResultSymbol with a stale
// hardcoded ID would silently emit the wrong (but still structurally valid)
// node type instead of failing loudly.
type PhpExternalScanner struct {
	symbols         [phpTokenCount]gotreesitter.Symbol
	externalToToken []int
}

// ExternalScannerForLanguage binds the scanner's token slots to the loaded
// Language's ExternalSymbols positionally. A hardcoded absolute
// gotreesitter.Symbol constant here would emit the wrong token whenever a
// grammar bump renumbers php's external symbols.
func (PhpExternalScanner) ExternalScannerForLanguage(lang *gotreesitter.Language) gotreesitter.ExternalScanner {
	s := PhpExternalScanner{symbols: phpDefaultSymTable}
	s.externalToToken = bindExternalScannerSpec(lang, phpExternalScannerSpec, func(tokenIdx int, sym gotreesitter.Symbol) {
		s.symbols[tokenIdx] = sym
	})
	return s
}

func (s PhpExternalScanner) symbolTable() *[phpTokenCount]gotreesitter.Symbol {
	if s.symbols == ([phpTokenCount]gotreesitter.Symbol{}) {
		return &phpDefaultSymTable
	}
	return &s.symbols
}

func (PhpExternalScanner) Create() any {
	return &phpState{}
}
func (PhpExternalScanner) Destroy(payload any) {}

func (PhpExternalScanner) Serialize(payload any, buf []byte) int {
	s := payload.(*phpState)
	size := 0
	if len(buf) == 0 {
		return 0
	}
	buf[size] = byte(len(s.heredocs))
	size++
	for j := range s.heredocs {
		hd := &s.heredocs[j]
		wordBytes := len(hd.word) * 4
		if size+5+wordBytes >= len(buf) {
			return 0
		}
		if hd.endWordIndentAllowed {
			buf[size] = 1
		} else {
			buf[size] = 0
		}
		size++
		binary.LittleEndian.PutUint32(buf[size:], uint32(len(hd.word)))
		size += 4
		for _, ch := range hd.word {
			binary.LittleEndian.PutUint32(buf[size:], uint32(ch))
			size += 4
		}
	}
	return size
}

func (PhpExternalScanner) Deserialize(payload any, buf []byte) {
	s := payload.(*phpState)
	s.heredocs = s.heredocs[:0]

	if len(buf) == 0 {
		return
	}
	size := 0
	hdCount := int(buf[size])
	size++
	for i := 0; i < hdCount && size < len(buf); i++ {
		hd := phpHeredoc{}
		hd.endWordIndentAllowed = buf[size] != 0
		size++
		if size+4 > len(buf) {
			break
		}
		wordLen := int(binary.LittleEndian.Uint32(buf[size:]))
		size += 4
		hd.word = make([]rune, 0, wordLen)
		for j := 0; j < wordLen && size+4 <= len(buf); j++ {
			ch := int32(binary.LittleEndian.Uint32(buf[size:]))
			hd.word = append(hd.word, rune(ch))
			size += 4
		}
		s.heredocs = append(s.heredocs, hd)
	}
}

func (s PhpExternalScanner) Scan(payload any, lexer *gotreesitter.ExternalLexer, validSymbols []bool) bool {
	state := payload.(*phpState)

	if len(s.externalToToken) > 0 {
		var semanticValid [phpTokenCount]bool
		for externalIdx, valid := range validSymbols {
			if !valid || externalIdx >= len(s.externalToToken) {
				continue
			}
			tokenIdx := s.externalToToken[externalIdx]
			if tokenIdx >= 0 && tokenIdx < phpTokenCount {
				semanticValid[tokenIdx] = true
			}
		}
		validSymbols = semanticValid[:]
	}
	syms := s.symbolTable()

	isValid := func(idx int) bool {
		return idx < len(validSymbols) && validSymbols[idx]
	}

	if isValid(phpTokSentinelError) {
		return false
	}

	lexer.MarkEnd()

	if isValid(phpTokEncapsedStringCharsAfterVar) {
		lexer.SetResultSymbol(syms[phpTokEncapsedStringCharsAfterVar])
		return phpScanEncapsed(state, lexer, true, false, false)
	}

	if isValid(phpTokEncapsedStringChars) {
		lexer.SetResultSymbol(syms[phpTokEncapsedStringChars])
		return phpScanEncapsed(state, lexer, false, false, false)
	}

	if isValid(phpTokExecutionStringCharsAfterVar) {
		lexer.SetResultSymbol(syms[phpTokExecutionStringCharsAfterVar])
		return phpScanEncapsed(state, lexer, true, false, true)
	}

	if isValid(phpTokExecutionStringChars) {
		lexer.SetResultSymbol(syms[phpTokExecutionStringChars])
		return phpScanEncapsed(state, lexer, false, false, true)
	}

	if isValid(phpTokEncapsedStringCharsAfterVarHdoc) {
		lexer.SetResultSymbol(syms[phpTokEncapsedStringCharsAfterVarHdoc])
		return phpScanEncapsed(state, lexer, true, true, false)
	}

	if isValid(phpTokEncapsedStringCharsHeredoc) {
		lexer.SetResultSymbol(syms[phpTokEncapsedStringCharsHeredoc])
		return phpScanEncapsed(state, lexer, false, true, false)
	}

	if isValid(phpTokNowdocString) {
		lexer.SetResultSymbol(syms[phpTokNowdocString])
		return phpScanNowdoc(state, lexer)
	}

	if isValid(phpTokHeredocEnd) {
		lexer.SetResultSymbol(syms[phpTokHeredocEnd])
		if len(state.heredocs) == 0 {
			return false
		}
		hd := state.heredocs[len(state.heredocs)-1]

		for unicode.IsSpace(lexer.Lookahead()) {
			lexer.Advance(true)
		}

		word := phpScanHeredocWord(lexer)
		if !phpRuneSliceEqual(word, hd.word) {
			return false
		}

		lexer.MarkEnd()
		state.heredocs = state.heredocs[:len(state.heredocs)-1]
		return true
	}

	if !phpSkipWhitespace(lexer) {
		return false
	}

	if isValid(phpTokEOF) && lexer.Lookahead() == 0 {
		lexer.SetResultSymbol(syms[phpTokEOF])
		return true
	}

	if isValid(phpTokHeredocStart) {
		lexer.SetResultSymbol(syms[phpTokHeredocStart])
		hd := phpHeredoc{}

		for unicode.IsSpace(lexer.Lookahead()) {
			lexer.Advance(true)
		}

		hd.word = phpScanHeredocWord(lexer)
		if len(hd.word) == 0 {
			return false
		}
		lexer.MarkEnd()

		state.heredocs = append(state.heredocs, hd)
		return true
	}

	if isValid(phpTokAutoSemicolon) {
		lexer.SetResultSymbol(syms[phpTokAutoSemicolon])
		if lexer.Lookahead() != '?' {
			return false
		}
		lexer.Advance(false)
		return lexer.Lookahead() == '>'
	}

	return false
}

func phpIsValidNameChar(lexer *gotreesitter.ExternalLexer) bool {
	ch := lexer.Lookahead()
	return unicode.IsLetter(ch) || unicode.IsDigit(ch) || ch == '_' || ch >= 0x80
}

func phpIsEscapableSequence(lexer *gotreesitter.ExternalLexer) bool {
	ch := lexer.Lookahead()
	if ch == 'n' || ch == 'r' || ch == 't' || ch == 'v' || ch == 'e' || ch == 'f' ||
		ch == '\\' || ch == '$' || ch == '"' {
		return true
	}
	if ch == 'x' {
		lexer.Advance(false)
		return phpIsHexDigit(lexer.Lookahead())
	}
	if ch == 'u' {
		return true
	}
	return ch >= '0' && ch <= '7'
}

func phpIsHexDigit(ch rune) bool {
	return (ch >= '0' && ch <= '9') || (ch >= 'a' && ch <= 'f') || (ch >= 'A' && ch <= 'F')
}

func phpSkipWhitespace(lexer *gotreesitter.ExternalLexer) bool {
	for {
		for unicode.IsSpace(lexer.Lookahead()) {
			lexer.Advance(false)
		}
		if lexer.Lookahead() == '/' {
			lexer.Advance(false)
			if lexer.Lookahead() == '/' {
				lexer.Advance(false)
				for lexer.Lookahead() != 0 && lexer.Lookahead() != '\n' {
					lexer.Advance(false)
				}
			} else {
				return false
			}
		} else {
			return true
		}
	}
}

func phpScanHeredocWord(lexer *gotreesitter.ExternalLexer) []rune {
	var result []rune
	for phpIsValidNameChar(lexer) {
		result = append(result, lexer.Lookahead())
		lexer.Advance(false)
	}
	return result
}

func phpRuneSliceEqual(a, b []rune) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func phpScanNowdoc(s *phpState, lexer *gotreesitter.ExternalLexer) bool {
	hasConsumed := false
	if len(s.heredocs) == 0 {
		return false
	}

	for unicode.IsSpace(lexer.Lookahead()) {
		lexer.Advance(false)
		hasConsumed = true
	}

	tag := s.heredocs[len(s.heredocs)-1].word
	endTagMatched := false
	for i := 0; i < len(tag); i++ {
		if lexer.Lookahead() != tag[i] {
			break
		}
		lexer.Advance(false)
		hasConsumed = true
		if i == len(tag)-1 {
			la := lexer.Lookahead()
			if unicode.IsSpace(la) || la == ';' || la == ',' || la == ')' {
				endTagMatched = true
			}
		}
	}

	if endTagMatched {
		for unicode.IsSpace(lexer.Lookahead()) && lexer.Lookahead() != '\r' && lexer.Lookahead() != '\n' {
			lexer.Advance(false)
			hasConsumed = true
		}
		la := lexer.Lookahead()
		if la == ';' || la == ',' || la == ')' || la == '\n' || la == '\r' {
			return false
		}
	}

	hasContent := hasConsumed
	for {
		lexer.MarkEnd()
		switch lexer.Lookahead() {
		case '\n', '\r':
			return hasContent
		default:
			if lexer.Lookahead() == 0 {
				return false
			}
			lexer.Advance(false)
		}
		hasContent = true
	}
}

func phpScanEncapsed(s *phpState, lexer *gotreesitter.ExternalLexer, isAfterVariable, isHeredoc, isExecution bool) bool {
	hasConsumed := false

	if isHeredoc && len(s.heredocs) > 0 {
		for unicode.IsSpace(lexer.Lookahead()) && lexer.Lookahead() != '\r' && lexer.Lookahead() != '\n' {
			lexer.Advance(false)
			hasConsumed = true
		}

		tag := s.heredocs[len(s.heredocs)-1].word
		endTagMatched := false
		for i := 0; i < len(tag); i++ {
			if lexer.Lookahead() != tag[i] {
				break
			}
			hasConsumed = true
			lexer.Advance(false)
			if i == len(tag)-1 {
				la := lexer.Lookahead()
				if unicode.IsSpace(la) || la == ';' || la == ',' || la == ')' {
					endTagMatched = true
				}
			}
		}

		if endTagMatched {
			for unicode.IsSpace(lexer.Lookahead()) && lexer.Lookahead() != '\r' && lexer.Lookahead() != '\n' {
				lexer.Advance(false)
				hasConsumed = true
			}
			la := lexer.Lookahead()
			if la == ';' || la == ',' || la == ')' || la == '\n' || la == '\r' {
				return false
			}
		}
	}

	hasContent := hasConsumed
	afterVar := isAfterVariable
	for {
		lexer.MarkEnd()

		switch lexer.Lookahead() {
		case '"':
			if !isHeredoc && !isExecution {
				return hasContent
			}
			lexer.Advance(false)
		case '`':
			if isExecution {
				return hasContent
			}
			lexer.Advance(false)
		case '\n', '\r':
			if isHeredoc {
				return hasContent
			}
			lexer.Advance(false)
		case '\\':
			lexer.Advance(false)
			if lexer.Lookahead() == '{' {
				lexer.Advance(false)
			} else if isExecution && lexer.Lookahead() == '`' {
				return hasContent
			} else if isHeredoc && lexer.Lookahead() == '\\' {
				lexer.Advance(false)
			} else if phpIsEscapableSequence(lexer) {
				return hasContent
			}
		case '$':
			lexer.Advance(false)
			if (phpIsValidNameChar(lexer) && !unicode.IsDigit(lexer.Lookahead())) || lexer.Lookahead() == '{' {
				return hasContent
			}
		case '-':
			if afterVar {
				lexer.Advance(false)
				if lexer.Lookahead() == '>' {
					lexer.Advance(false)
					if phpIsValidNameChar(lexer) {
						return hasContent
					}
				}
			} else {
				// C fallthrough to '[' case: when not afterVar, just advance
				lexer.Advance(false)
			}
		case '[':
			if afterVar {
				return hasContent
			}
			lexer.Advance(false)
		case '{':
			lexer.Advance(false)
			if lexer.Lookahead() == '$' {
				return hasContent
			}
		default:
			if lexer.Lookahead() == 0 {
				return false
			}
			lexer.Advance(false)
		}

		afterVar = false
		hasContent = true
	}
}
