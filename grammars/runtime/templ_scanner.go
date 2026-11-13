//go:build !grammar_subset || grammar_subset_templ

package grammarruntime

import (
	"github.com/odvcencio/gotreesitter/internal/scanmode"
	"unicode"

	gotreesitter "github.com/odvcencio/gotreesitter"
)

// External token indexes for the templ grammar.
const (
	templTokCssPropertyValue = 0
	templTokScriptBlockText  = 1
	templTokSwitchElemText   = 2
	templTokElemText         = 3
	templTokenCount          = 4
)

// templDefaultSymTable records the concrete gotreesitter.Symbol IDs the
// currently shipped templ.bin assigns to each external, in templTok* order.
// It exists only as a pre-bind fallback (and as an independent value to
// compare a real bind against in tests); ExternalScannerForLanguage below
// overwrites it with values read from the actual loaded Language at bind
// time, which is what the scanner must do to survive a future blob regen
// that renumbers absolute symbol IDs without touching the externals list
// order.
var templDefaultSymTable = [templTokenCount]gotreesitter.Symbol{
	127, // css_property_value
	128, // script_block_text
	129, // switch_element_text
	130, // element_text
}

// templExternalScannerSpec records the source contract for this
// hand-written port, so updater tooling can tell a grammar-only upstream
// change apart from one that also touches the external scanner or its
// token list. Its Externals list is also the binding source for
// ExternalScannerForLanguage: index i here is scanner token index i
// (templTok* order).
var templExternalScannerSpec = ExternalScannerSpec{
	Language:       "templ",
	UpstreamRepo:   "https://github.com/vrischmann/tree-sitter-templ",
	UpstreamCommit: "1c6db04effbcd7773c826bded9783cbc3061bd55",
	SourceFiles: []ExternalScannerSourceFile{
		{Path: "src/grammar.json", SHA256: "0ea584a7e7a549bbc0640c12f317ceea0b509aa48f5e712ce476f69e3f58b357"},
		{Path: "src/scanner.c", SHA256: "e4ebb8e355486ef584518c87abcbea51ede7763393dc57d439ee00525acf3daf"},
	},
	Externals: []string{
		"css_property_value",
		"script_block_text",
		"switch_element_text",
		"element_text",
	},
}

func init() {
	RegisterExternalScannerSpec(templExternalScannerSpec)
}

var templStatementKeywords = []string{
	"//", "/*",
	"if ", "else ", "for ", "switch ",
}
var templSwitchKeywords = []string{
	"case ", "default:",
}

// templState tracks whether we've seen an @ symbol for component expressions.
type templState struct {
	sawAtSymbol   bool
	scanmode.Flag // the C-faithful mode, set by the core adapter
}

// TemplExternalScanner handles CSS property values, script blocks, and element text for templ.
//
// symbols holds the concrete gotreesitter.Symbol each external index maps to
// in the Language this instance was bound to (see ExternalScannerForLanguage).
// The scanner never hardcodes an absolute Symbol value: a blob regen can
// renumber the grammar's absolute symbol IDs without touching the externals
// list order, and a scanner that still called SetResultSymbol with a stale
// hardcoded ID would silently emit the wrong (but still structurally valid)
// node type instead of failing loudly.
type TemplExternalScanner struct {
	symbols         [templTokenCount]gotreesitter.Symbol
	externalToToken []int
}

// ExternalScannerForLanguage binds the scanner's token slots to the loaded
// Language's ExternalSymbols positionally. A hardcoded absolute
// gotreesitter.Symbol constant here would emit the wrong token whenever a
// grammar bump renumbers templ's external symbols.
func (TemplExternalScanner) ExternalScannerForLanguage(lang *gotreesitter.Language) gotreesitter.ExternalScanner {
	s := TemplExternalScanner{symbols: templDefaultSymTable}
	s.externalToToken = bindExternalScannerSpec(lang, templExternalScannerSpec, func(tokenIdx int, sym gotreesitter.Symbol) {
		s.symbols[tokenIdx] = sym
	})
	return s
}

func (s TemplExternalScanner) symbolTable() *[templTokenCount]gotreesitter.Symbol {
	if s.symbols == ([templTokenCount]gotreesitter.Symbol{}) {
		return &templDefaultSymTable
	}
	return &s.symbols
}

func (TemplExternalScanner) Create() any         { return &templState{} }
func (TemplExternalScanner) Destroy(payload any) {}

func (TemplExternalScanner) Serialize(payload any, buf []byte) int {
	return 0 // minimal state
}

func (TemplExternalScanner) Deserialize(payload any, buf []byte) {
	// The C scanner writes its state byte but returns a serialized length of zero,
	// so tree-sitter calls its deserializer with no state and leaves the existing
	// scanner state untouched. Clearing it here loses the component-import context
	// needed to stop element text at the import's argument-list delimiters.
	if len(buf) == 0 {
		return
	}
	s := payload.(*templState)
	s.sawAtSymbol = buf[0] == 1
}

func (sc TemplExternalScanner) Scan(payload any, lexer *gotreesitter.ExternalLexer, validSymbols []bool) bool {
	if len(sc.externalToToken) > 0 {
		var semanticValid [templTokenCount]bool
		for externalIdx, valid := range validSymbols {
			if !valid || externalIdx >= len(sc.externalToToken) {
				continue
			}
			tokenIdx := sc.externalToToken[externalIdx]
			if tokenIdx >= 0 && tokenIdx < templTokenCount {
				semanticValid[tokenIdx] = true
			}
		}
		validSymbols = semanticValid[:]
	}
	syms := sc.symbolTable()

	s := payload.(*templState)
	if s.On() {
		return templScanC(s, lexer, validSymbols, syms)
	}

	// Legacy mode: skip white space, then return the result of the first valid
	// token's scan (the behaviour of the base).
	for unicode.IsSpace(lexer.Lookahead()) {
		lexer.Advance(true)
	}

	if templValid(validSymbols, templTokCssPropertyValue) {
		return templScanCssPropertyValue(lexer, syms[templTokCssPropertyValue], false)
	}

	if templValid(validSymbols, templTokScriptBlockText) {
		return templScanScriptBlockText(lexer, syms[templTokScriptBlockText], false)
	}

	if templValid(validSymbols, templTokSwitchElemText) {
		return templScanElementText(s, lexer, syms[templTokSwitchElemText], true)
	}

	if templValid(validSymbols, templTokElemText) {
		return templScanElementText(s, lexer, syms[templTokElemText], false)
	}

	return false
}

// templScanC is Scan in the C-faithful mode (tree-sitter-templ src/scanner.c:317-345
// at the pinned commit): C tries each valid token in turn and falls through when
// one fails; a failed attempt may have consumed input and the next one continues
// from there. White space is the six ASCII characters of the C locale and the end
// of the input is asked from the lexer, so a NUL byte is not the end.
func templScanC(s *templState, lexer *gotreesitter.ExternalLexer, validSymbols []bool, syms *[templTokenCount]gotreesitter.Symbol) bool {
	for !scanmode.AtEOF(lexer) && cIsSpace(lexer.Lookahead()) {
		lexer.Advance(true)
	}
	if templValid(validSymbols, templTokCssPropertyValue) &&
		templScanCssPropertyValue(lexer, syms[templTokCssPropertyValue], true) {
		return true
	}
	if templValid(validSymbols, templTokScriptBlockText) &&
		templScanScriptBlockText(lexer, syms[templTokScriptBlockText], true) {
		return true
	}
	if templValid(validSymbols, templTokSwitchElemText) &&
		templScanElementTextC(s, lexer, syms[templTokSwitchElemText], true) {
		return true
	}
	if templValid(validSymbols, templTokElemText) &&
		templScanElementTextC(s, lexer, syms[templTokElemText], false) {
		return true
	}
	return false
}

// templEOF is the end-of-input test: C's lexer->eof in the C-faithful mode, a
// zero lookahead otherwise.
func templEOF(cf bool, lexer *gotreesitter.ExternalLexer) bool {
	if cf {
		return scanmode.AtEOF(lexer)
	}
	return lexer.Lookahead() == 0
}

func templScanCssPropertyValue(lexer *gotreesitter.ExternalLexer, sym gotreesitter.Symbol, cf bool) bool {
	if lexer.Lookahead() == '{' {
		return false
	}
	lexer.SetResultSymbol(sym)
	for !templEOF(cf, lexer) {
		if lexer.Lookahead() == ';' {
			return true
		}
		lexer.Advance(false)
	}
	return false
}

func templScanScriptBlockText(lexer *gotreesitter.ExternalLexer, sym gotreesitter.Symbol, cf bool) bool {
	lexer.SetResultSymbol(sym)
	lexer.MarkEnd()

	if templEOF(cf, lexer) {
		return false
	}

	hasMarked := false
	braceCount := 1

	for !templEOF(cf, lexer) {
		switch lexer.Lookahead() {
		case '{':
			braceCount++
		case '}':
			braceCount--
			if braceCount == 0 {
				return hasMarked
			}
		}
		lexer.Advance(false)
		lexer.MarkEnd()
		hasMarked = true
	}

	return hasMarked
}

func templScanElementText(s *templState, lexer *gotreesitter.ExternalLexer, sym gotreesitter.Symbol, inSwitch bool) bool {
	lexer.SetResultSymbol(sym)
	lexer.MarkEnd()

	if lexer.Lookahead() == 0 {
		return false
	}

	// Buffer for keyword lookahead
	var buf []rune
	count := 0

	// Check statement keywords
	keywords := templStatementKeywords
	if inSwitch {
		keywords = append(keywords, templSwitchKeywords...)
	}

	for _, kw := range keywords {
		if templMatchesKeyword(lexer, &buf, kw) {
			return false
		}
	}

	// Check for @ symbol (component expression)
	if templMatchesKeyword(lexer, &buf, "@") {
		s.sawAtSymbol = true
		return false
	}

	// Check buffer for terminators
	for _, ch := range buf {
		if templIsElemTextTerminator(ch) {
			return false
		}
		if s.sawAtSymbol && templIsImportExprTerminator(ch) {
			return false
		}
	}

	count += len(buf)

	// Continue scanning
	for lexer.Lookahead() != 0 {
		if templIsElemTextTerminator(lexer.Lookahead()) {
			break
		}
		if s.sawAtSymbol && templIsImportExprTerminator(lexer.Lookahead()) {
			break
		}
		lexer.Advance(false)
		lexer.MarkEnd()
		count++
	}

	if count > 0 {
		lexer.MarkEnd()
		s.sawAtSymbol = false
		return true
	}
	return false
}

// templScanElementTextC is scan_element_text of the pinned C scanner
// (src/scanner.c:164-266) with its lookahead buffer (lookahead_buffer_find_keyword,
// :42-73). The helper compares before it consumes, so a character that does
// not match stays unread, and its loop bound shrinks as the buffer is matched,
// both as in C.
func templScanElementTextC(s *templState, lexer *gotreesitter.ExternalLexer, sym gotreesitter.Symbol, inSwitch bool) bool {
	lexer.SetResultSymbol(sym)
	keywords := templStatementKeywords
	if inSwitch {
		keywords = append(append([]string{}, templStatementKeywords...), templSwitchKeywords...)
	}
	lexer.MarkEnd()
	var buf []rune
	hasMarked := false
	count := 0
	if scanmode.AtEOF(lexer) {
		return false
	}
	done := false
	for _, kw := range keywords {
		if templFindKeywordC(&buf, lexer, kw) {
			done = true
			break
		}
	}
	if !done && templFindKeywordC(&buf, lexer, "@") {
		s.sawAtSymbol = true
		done = true
	}
	if !done {
		for _, ch := range buf {
			if templIsElemTextTerminator(ch) {
				done = true
				break
			}
		}
	}
	if !done && s.sawAtSymbol {
		for _, ch := range buf {
			if templIsImportExprTerminator(ch) {
				done = true
				break
			}
		}
	}
	if !done {
		count += len(buf)
		for !scanmode.AtEOF(lexer) {
			if templIsElemTextTerminator(lexer.Lookahead()) {
				break
			}
			if s.sawAtSymbol && templIsImportExprTerminator(lexer.Lookahead()) {
				break
			}
			lexer.Advance(false)
			lexer.MarkEnd()
			count++
		}
	}
	if count > 0 {
		lexer.MarkEnd()
		hasMarked = true
	}
	if hasMarked {
		s.sawAtSymbol = false
	}
	return hasMarked
}

// templFindKeywordC is lookahead_buffer_find_keyword (scanner.c:42-73).
func templFindKeywordC(buf *[]rune, lexer *gotreesitter.ExternalLexer, str string) bool {
	runes := []rune(str)
	length := len(runes)
	for i := 0; i < len(*buf) && i < length; i++ {
		if (*buf)[i] != runes[i] {
			return false
		}
		length--
	}
	// C reads str[write_pos]; past the end of the keyword that is the NUL
	// terminator, which no lookahead of a normal character equals.
	start := len(*buf) // C: str_remaining = &str[write_pos], taken before the loop
	for i := 0; i < length; i++ {
		var want rune
		if idx := start + i; idx < len(runes) {
			want = runes[idx]
		}
		if scanmode.AtEOF(lexer) || lexer.Lookahead() != want {
			return false
		}
		*buf = append(*buf, lexer.Lookahead())
		lexer.Advance(false)
	}
	return true
}

func templMatchesKeyword(lexer *gotreesitter.ExternalLexer, buf *[]rune, kw string) bool {
	runes := []rune(kw)
	for i, r := range runes {
		var ch rune
		if i < len(*buf) {
			ch = (*buf)[i]
		} else {
			if lexer.Lookahead() == 0 {
				return false
			}
			ch = lexer.Lookahead()
			*buf = append(*buf, ch)
			lexer.Advance(false)
		}
		if ch != r {
			return false
		}
	}
	return true
}

func templIsElemTextTerminator(ch rune) bool {
	return ch == '<' || ch == '{' || ch == '}' || ch == '\n'
}

func templIsImportExprTerminator(ch rune) bool {
	return ch == '.' || ch == '(' || ch == ')' || ch == '['
}

func templValid(vs []bool, i int) bool { return i < len(vs) && vs[i] }

func init() {
	// A port with a behavioural mode (internal/scanmode); registered with the rest of its set.
	scanmode.RegisterScanner(TemplExternalScanner{}, (*templState)(nil), true)
}
