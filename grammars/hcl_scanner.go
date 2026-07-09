//go:build !grammar_subset || grammar_subset_hcl

package grammars

import (
	"encoding/binary"
	"unicode"

	gotreesitter "github.com/odvcencio/gotreesitter"
)

// External token indexes for the HCL grammar.
const (
	hclTokQuotedTemplateStart        = 0
	hclTokQuotedTemplateEnd          = 1
	hclTokTemplateLiteralChunk       = 2
	hclTokTemplateInterpolationStart = 3
	hclTokTemplateInterpolationEnd   = 4
	hclTokTemplateDirectiveStart     = 5
	hclTokTemplateDirectiveEnd       = 6
	hclTokHeredocIdentifier          = 7
	hclTokenCount                    = hclTokHeredocIdentifier + 1
)

// Default/fallback symbol IDs, matching the checked-in ts2go HCL grammar blob's
// ExternalSymbols. These are used only for a zero-value HclExternalScanner (no
// bound Language). ExternalScannerForLanguage below binds the real per-Language
// symbols POSITIONALLY (by external index, via bindExternalScannerSymbolNames),
// not by these absolute IDs, so a future grammargen-generated hcl blob — which
// emits the same 8 externals in the same order but under different absolute
// Symbol numbering — still lexes correctly instead of silently mistyping every
// external token.
const (
	hclSymQuotedTemplateStart        gotreesitter.Symbol = 48
	hclSymQuotedTemplateEnd          gotreesitter.Symbol = 49
	hclSymTemplateLiteralChunk       gotreesitter.Symbol = 50
	hclSymTemplateInterpolationStart gotreesitter.Symbol = 51
	hclSymTemplateInterpolationEnd   gotreesitter.Symbol = 52
	hclSymTemplateDirectiveStart     gotreesitter.Symbol = 53
	hclSymTemplateDirectiveEnd       gotreesitter.Symbol = 54
	hclSymHeredocIdentifier          gotreesitter.Symbol = 55
)

var hclDefaultSymTable = [hclTokenCount]gotreesitter.Symbol{
	hclTokQuotedTemplateStart:        hclSymQuotedTemplateStart,
	hclTokQuotedTemplateEnd:          hclSymQuotedTemplateEnd,
	hclTokTemplateLiteralChunk:       hclSymTemplateLiteralChunk,
	hclTokTemplateInterpolationStart: hclSymTemplateInterpolationStart,
	hclTokTemplateInterpolationEnd:   hclSymTemplateInterpolationEnd,
	hclTokTemplateDirectiveStart:     hclSymTemplateDirectiveStart,
	hclTokTemplateDirectiveEnd:       hclSymTemplateDirectiveEnd,
	hclTokHeredocIdentifier:          hclSymHeredocIdentifier,
}

// hclExternalSymbolNames lists the HCL grammar's external tokens, in scanner
// token-index order (matching upstream tree-sitter-hcl's `externals: [...]`
// order). Used by ExternalScannerForLanguage to bind this scanner's token slots
// to a Language's ExternalSymbols positionally, the same pattern used by the
// kotlin/python/swift/dart/rust scanners. See bindExternalScannerSymbolNames for
// why positional (not absolute-ID or by-name) binding is required.
var hclExternalSymbolNames = []string{
	"quoted_template_start",
	"quoted_template_end",
	"_template_literal_chunk",
	"template_interpolation_start",
	"template_interpolation_end",
	"template_directive_start",
	"template_directive_end",
	"heredoc_identifier",
}

// hclContextType identifies the kind of template context being tracked.
type hclContextType uint32

const (
	hclCtxTemplateInterpolation hclContextType = iota
	hclCtxTemplateDirective
	hclCtxQuotedTemplate
	hclCtxHeredocTemplate
)

// hclContext represents one frame on the context stack.
type hclContext struct {
	ctxType           hclContextType
	heredocIdentifier string // non-empty only when ctxType == hclCtxHeredocTemplate
}

// hclState holds scanner state across parse calls.
type hclState struct {
	contextStack []hclContext
}

func (s *hclState) back() *hclContext {
	return &s.contextStack[len(s.contextStack)-1]
}

func (s *hclState) push(ctx hclContext) {
	s.contextStack = append(s.contextStack, ctx)
}

func (s *hclState) pop() {
	s.contextStack = s.contextStack[:len(s.contextStack)-1]
}

func (s *hclState) inContextType(ct hclContextType) bool {
	if len(s.contextStack) == 0 {
		return false
	}
	return s.back().ctxType == ct
}

func (s *hclState) inQuotedContext() bool        { return s.inContextType(hclCtxQuotedTemplate) }
func (s *hclState) inHeredocContext() bool       { return s.inContextType(hclCtxHeredocTemplate) }
func (s *hclState) inTemplateContext() bool      { return s.inQuotedContext() || s.inHeredocContext() }
func (s *hclState) inInterpolationContext() bool { return s.inContextType(hclCtxTemplateInterpolation) }
func (s *hclState) inDirectiveContext() bool     { return s.inContextType(hclCtxTemplateDirective) }

// HclExternalScanner implements gotreesitter.ExternalScanner for the HCL grammar.
type HclExternalScanner struct {
	symbols         [hclTokenCount]gotreesitter.Symbol
	externalToToken []int
}

// ExternalScannerForLanguage binds this scanner's token slots to lang's external
// symbols positionally (external index i -> scanner token i), so Scan resolves
// result symbols through the bound table instead of hardcoded absolute IDs.
func (HclExternalScanner) ExternalScannerForLanguage(lang *gotreesitter.Language) gotreesitter.ExternalScanner {
	s := HclExternalScanner{symbols: hclDefaultSymTable}
	s.externalToToken = bindExternalScannerSymbolNames(lang, hclExternalSymbolNames, func(tokenIdx int, sym gotreesitter.Symbol) {
		s.symbols[tokenIdx] = sym
	})
	return s
}

func (HclExternalScanner) Create() any         { return &hclState{} }
func (HclExternalScanner) Destroy(payload any) {}

func (scanner HclExternalScanner) symbolTable() *[hclTokenCount]gotreesitter.Symbol {
	if scanner.symbols == ([hclTokenCount]gotreesitter.Symbol{}) {
		return &hclDefaultSymTable
	}
	return &scanner.symbols
}

func (scanner HclExternalScanner) remapValidSymbols(validSymbols []bool, semanticValid *[hclTokenCount]bool) []bool {
	if len(scanner.externalToToken) == 0 {
		return validSymbols
	}
	*semanticValid = [hclTokenCount]bool{}
	for externalIdx, valid := range validSymbols {
		if !valid || externalIdx >= len(scanner.externalToToken) {
			continue
		}
		tokenIdx := scanner.externalToToken[externalIdx]
		if tokenIdx >= 0 && tokenIdx < hclTokenCount {
			semanticValid[tokenIdx] = true
		}
	}
	return semanticValid[:]
}

func (HclExternalScanner) Serialize(payload any, buf []byte) int {
	s := payload.(*hclState)
	if len(s.contextStack) > 127 {
		return 0
	}

	size := 0
	// Write context stack length as uint32 (little-endian to match C memcpy on LE).
	if size+4 > len(buf) {
		return 0
	}
	binary.LittleEndian.PutUint32(buf[size:], uint32(len(s.contextStack)))
	size += 4

	for i := range s.contextStack {
		ctx := &s.contextStack[i]
		idLen := len(ctx.heredocIdentifier)
		if idLen > 127 {
			return 0
		}
		// Need space for: uint32 type + uint32 idLen + id bytes.
		needed := 4 + 4 + idLen
		if size+needed > len(buf) {
			return 0
		}
		binary.LittleEndian.PutUint32(buf[size:], uint32(ctx.ctxType))
		size += 4
		binary.LittleEndian.PutUint32(buf[size:], uint32(idLen))
		size += 4
		copy(buf[size:], ctx.heredocIdentifier)
		size += idLen
	}
	return size
}

func (HclExternalScanner) Deserialize(payload any, buf []byte) {
	s := payload.(*hclState)
	s.contextStack = s.contextStack[:0]

	if len(buf) == 0 {
		return
	}

	size := 0
	if size+4 > len(buf) {
		return
	}
	stackLen := binary.LittleEndian.Uint32(buf[size:])
	size += 4

	for j := uint32(0); j < stackLen; j++ {
		if size+4 > len(buf) {
			return
		}
		ctxType := hclContextType(binary.LittleEndian.Uint32(buf[size:]))
		size += 4

		if size+4 > len(buf) {
			return
		}
		idLen := binary.LittleEndian.Uint32(buf[size:])
		size += 4

		var id string
		if idLen > 0 {
			if size+int(idLen) > len(buf) {
				return
			}
			id = string(buf[size : size+int(idLen)])
			size += int(idLen)
		}
		s.contextStack = append(s.contextStack, hclContext{
			ctxType:           ctxType,
			heredocIdentifier: id,
		})
	}
}

func (scanner HclExternalScanner) Scan(payload any, lexer *gotreesitter.ExternalLexer, validSymbols []bool) bool {
	s := payload.(*hclState)

	var semanticValid [hclTokenCount]bool
	validSymbols = scanner.remapValidSymbols(validSymbols, &semanticValid)
	symbols := scanner.symbolTable()

	// Skip whitespace, tracking whether a newline was seen.
	hasLeadingNewline := false
	for unicode.IsSpace(lexer.Lookahead()) {
		if lexer.Lookahead() == '\n' {
			hasLeadingNewline = true
		}
		lexer.Advance(true)
	}

	if lexer.Lookahead() == 0 {
		return false
	}

	// ---- Quoted template context ----
	if hclValid(validSymbols, hclTokQuotedTemplateStart) && !s.inQuotedContext() && lexer.Lookahead() == '"' {
		s.push(hclContext{ctxType: hclCtxQuotedTemplate})
		lexer.Advance(false)
		lexer.SetResultSymbol(symbols[hclTokQuotedTemplateStart])
		return true
	}
	if hclValid(validSymbols, hclTokQuotedTemplateEnd) && s.inQuotedContext() && lexer.Lookahead() == '"' {
		s.pop()
		lexer.Advance(false)
		lexer.SetResultSymbol(symbols[hclTokQuotedTemplateEnd])
		return true
	}

	// ---- Template interpolation ----
	if hclValid(validSymbols, hclTokTemplateInterpolationStart) &&
		hclValid(validSymbols, hclTokTemplateLiteralChunk) &&
		!s.inInterpolationContext() && lexer.Lookahead() == '$' {

		lexer.Advance(false)
		if lexer.Lookahead() == '{' {
			s.push(hclContext{ctxType: hclCtxTemplateInterpolation})
			lexer.Advance(false)
			lexer.SetResultSymbol(symbols[hclTokTemplateInterpolationStart])
			return true
		}
		// Escape sequence: $${ becomes literal chunk
		if lexer.Lookahead() == '$' {
			lexer.Advance(false)
			if lexer.Lookahead() == '{' {
				lexer.Advance(false)
				lexer.SetResultSymbol(symbols[hclTokTemplateLiteralChunk])
				return true
			}
		}
		lexer.SetResultSymbol(symbols[hclTokTemplateLiteralChunk])
		return true
	}
	if hclValid(validSymbols, hclTokTemplateInterpolationEnd) && s.inInterpolationContext() && lexer.Lookahead() == '}' {
		s.pop()
		lexer.Advance(false)
		lexer.SetResultSymbol(symbols[hclTokTemplateInterpolationEnd])
		return true
	}

	// ---- Template directive ----
	if hclValid(validSymbols, hclTokTemplateDirectiveStart) &&
		hclValid(validSymbols, hclTokTemplateLiteralChunk) &&
		!s.inDirectiveContext() && lexer.Lookahead() == '%' {

		lexer.Advance(false)
		if lexer.Lookahead() == '{' {
			s.push(hclContext{ctxType: hclCtxTemplateDirective})
			lexer.Advance(false)
			lexer.SetResultSymbol(symbols[hclTokTemplateDirectiveStart])
			return true
		}
		// Escape sequence: %%{ becomes literal chunk
		if lexer.Lookahead() == '%' {
			lexer.Advance(false)
			if lexer.Lookahead() == '{' {
				lexer.Advance(false)
				lexer.SetResultSymbol(symbols[hclTokTemplateLiteralChunk])
				return true
			}
		}
		lexer.SetResultSymbol(symbols[hclTokTemplateLiteralChunk])
		return true
	}
	if hclValid(validSymbols, hclTokTemplateDirectiveEnd) && s.inDirectiveContext() && lexer.Lookahead() == '}' {
		s.pop()
		lexer.Advance(false)
		lexer.SetResultSymbol(symbols[hclTokTemplateDirectiveEnd])
		return true
	}

	// ---- Heredoc identifier ----
	if hclValid(validSymbols, hclTokHeredocIdentifier) && !s.inHeredocContext() {
		// Scan a new heredoc identifier.
		var ident []byte
		for hclIsIdentChar(lexer.Lookahead()) {
			ident = append(ident, byte(lexer.Lookahead()))
			lexer.Advance(false)
		}
		s.push(hclContext{
			ctxType:           hclCtxHeredocTemplate,
			heredocIdentifier: string(ident),
		})
		lexer.SetResultSymbol(symbols[hclTokHeredocIdentifier])
		return true
	}
	if hclValid(validSymbols, hclTokHeredocIdentifier) && s.inHeredocContext() && hasLeadingNewline {
		expected := s.back().heredocIdentifier
		for i := 0; i < len(expected); i++ {
			if lexer.Lookahead() == rune(expected[i]) {
				lexer.Advance(false)
			} else {
				lexer.SetResultSymbol(symbols[hclTokTemplateLiteralChunk])
				return true
			}
		}
		// Check if the identifier is on a line of its own.
		lexer.MarkEnd()
		for unicode.IsSpace(lexer.Lookahead()) && lexer.Lookahead() != '\n' {
			lexer.Advance(false)
		}
		if lexer.Lookahead() == '\n' {
			s.pop()
			lexer.SetResultSymbol(symbols[hclTokHeredocIdentifier])
			return true
		}
		lexer.Advance(false)
		lexer.MarkEnd()
		lexer.SetResultSymbol(symbols[hclTokTemplateLiteralChunk])
		return true
	}

	// ---- Template literal chunks ----

	// Handle escape sequences in quoted template context.
	if hclValid(validSymbols, hclTokTemplateLiteralChunk) && s.inQuotedContext() {
		if lexer.Lookahead() == '\\' {
			lexer.Advance(false)
			switch lexer.Lookahead() {
			case '"', 'n', 'r', 't', '\\':
				lexer.Advance(false)
				lexer.SetResultSymbol(symbols[hclTokTemplateLiteralChunk])
				return true
			case 'u':
				for i := 0; i < 4; i++ {
					lexer.Advance(false)
					if !hclIsHexDigit(lexer.Lookahead()) {
						return false
					}
				}
				lexer.Advance(false)
				lexer.SetResultSymbol(symbols[hclTokTemplateLiteralChunk])
				return true
			case 'U':
				for i := 0; i < 8; i++ {
					lexer.Advance(false)
					if !hclIsHexDigit(lexer.Lookahead()) {
						return false
					}
				}
				lexer.Advance(false)
				lexer.SetResultSymbol(symbols[hclTokTemplateLiteralChunk])
				return true
			default:
				return false
			}
		}
	}

	// Handle all other characters in template contexts.
	if hclValid(validSymbols, hclTokTemplateLiteralChunk) && s.inTemplateContext() {
		lexer.Advance(false)
		lexer.SetResultSymbol(symbols[hclTokTemplateLiteralChunk])
		return true
	}

	return false
}

// hclValid checks whether a token index is valid in the current parse state.
func hclValid(vs []bool, i int) bool { return i < len(vs) && vs[i] }

// hclIsIdentChar returns true for characters allowed in HCL heredoc identifiers.
func hclIsIdentChar(ch rune) bool {
	return unicode.IsLetter(ch) || unicode.IsDigit(ch) || ch == '_' || ch == '-'
}

// hclIsHexDigit returns true for hexadecimal digit characters.
func hclIsHexDigit(ch rune) bool {
	return (ch >= '0' && ch <= '9') || (ch >= 'a' && ch <= 'f') || (ch >= 'A' && ch <= 'F')
}
