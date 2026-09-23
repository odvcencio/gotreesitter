//go:build !grammar_subset || grammar_subset_templ

package grammarruntime

import (
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
	sawAtSymbol bool
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
	s := payload.(*templState)
	s.sawAtSymbol = false
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

	// Skip whitespace
	for unicode.IsSpace(lexer.Lookahead()) {
		lexer.Advance(true)
	}

	if templValid(validSymbols, templTokCssPropertyValue) {
		return templScanCssPropertyValue(lexer, syms[templTokCssPropertyValue])
	}

	if templValid(validSymbols, templTokScriptBlockText) {
		return templScanScriptBlockText(lexer, syms[templTokScriptBlockText])
	}

	if templValid(validSymbols, templTokSwitchElemText) {
		return templScanElementText(s, lexer, syms[templTokSwitchElemText], true)
	}

	if templValid(validSymbols, templTokElemText) {
		return templScanElementText(s, lexer, syms[templTokElemText], false)
	}

	return false
}

func templScanCssPropertyValue(lexer *gotreesitter.ExternalLexer, sym gotreesitter.Symbol) bool {
	if lexer.Lookahead() == '{' {
		return false
	}
	lexer.SetResultSymbol(sym)
	for lexer.Lookahead() != 0 {
		if lexer.Lookahead() == ';' {
			return true
		}
		lexer.Advance(false)
	}
	return false
}

func templScanScriptBlockText(lexer *gotreesitter.ExternalLexer, sym gotreesitter.Symbol) bool {
	lexer.SetResultSymbol(sym)
	lexer.MarkEnd()

	if lexer.Lookahead() == 0 {
		return false
	}

	hasMarked := false
	braceCount := 1

	for lexer.Lookahead() != 0 {
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
