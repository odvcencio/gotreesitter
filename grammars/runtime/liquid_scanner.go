//go:build !grammar_subset || grammar_subset_liquid

package grammarruntime

import (
	"unicode"

	gotreesitter "github.com/odvcencio/gotreesitter"
)

// External token indexes for the liquid grammar.
const (
	liquidTokInlineCommentContent    = 0
	liquidTokPairedCommentContent    = 1
	liquidTokPairedCommentContentLiq = 2
	liquidTokRawContent              = 3
	liquidTokFrontMatter             = 4
	liquidTokErrorSentinel           = 5
	liquidTokenCount                 = 6
)

// liquidDefaultSymTable seeds a scanner value that is used without going
// through ExternalScannerForLanguage first. Production attachment always
// calls ExternalScannerForLanguage, which rebinds these slots positionally
// against the loaded Language's ExternalSymbols (see
// liquidExternalScannerSpec and bindExternalScannerSpec). These six values
// match the liquid.bin blob shipped on 2026-09-20; keep them in step with
// ExternalSymbols[0:6] if that ever changes.
var liquidDefaultSymTable = [liquidTokenCount]gotreesitter.Symbol{
	98,  // _inline_comment_content
	99,  // _paired_comment_content
	100, // _paired_comment_content_liq
	101, // raw_content
	102, // front_matter
	103, // error_sentinel
}

var liquidExternalScannerSpec = ExternalScannerSpec{
	Language:       "liquid",
	UpstreamRepo:   "https://github.com/hankthetank27/tree-sitter-liquid",
	UpstreamCommit: "fa11c7ba45038b61e03a8a00ad667fb5f3d72088",
	SourceFiles: []ExternalScannerSourceFile{
		{Path: "src/grammar.json", SHA256: "7bc8ec55fe9112ffdfd3e2ee6a13b02e9b4b0c3986b73c7151555d898052f5e4"},
		{Path: "src/scanner.c", SHA256: "6e5aefa2487f346a86a752986bbbd7813ea9f988d5e1571ceb6d563f62b6b70e"},
	},
	Externals: []string{
		"_inline_comment_content",
		"_paired_comment_content",
		"_paired_comment_content_liq",
		"raw_content",
		"front_matter",
		"error_sentinel",
	},
}

func init() {
	RegisterExternalScannerSpec(liquidExternalScannerSpec)
}

// LiquidExternalScanner handles comment content, raw blocks, and front matter for Liquid templates.
type LiquidExternalScanner struct {
	symbols         [liquidTokenCount]gotreesitter.Symbol
	externalToToken []int
}

// ExternalScannerForLanguage binds the scanner's six token slots to the
// loaded Language's ExternalSymbols positionally. A hardcoded absolute
// gotreesitter.Symbol constant here would emit the wrong token whenever a
// grammar bump renumbers liquid's external symbols.
func (LiquidExternalScanner) ExternalScannerForLanguage(lang *gotreesitter.Language) gotreesitter.ExternalScanner {
	s := LiquidExternalScanner{symbols: liquidDefaultSymTable}
	s.externalToToken = bindExternalScannerSpec(lang, liquidExternalScannerSpec, func(tokenIdx int, sym gotreesitter.Symbol) {
		s.symbols[tokenIdx] = sym
	})
	return s
}

func (LiquidExternalScanner) Create() any                           { return nil }
func (LiquidExternalScanner) Destroy(payload any)                   {}
func (LiquidExternalScanner) Serialize(payload any, buf []byte) int { return 0 }
func (LiquidExternalScanner) Deserialize(payload any, buf []byte)   {}

// The scanner carries no payload and derives every result from local
// lookahead plus validSymbols, so every incremental boundary is quiescent and
// failed scans cannot mutate persistent state.
func (LiquidExternalScanner) SupportsIncrementalReuse() bool    { return true }
func (LiquidExternalScanner) ExternalScannerIsStateless() bool  { return true }
func (LiquidExternalScanner) PreservesStateOnScanFailure() bool { return true }

func (s LiquidExternalScanner) Scan(payload any, lexer *gotreesitter.ExternalLexer, validSymbols []bool) bool {
	if len(s.externalToToken) > 0 {
		var semanticValid [liquidTokenCount]bool
		for externalIdx, valid := range validSymbols {
			if !valid || externalIdx >= len(s.externalToToken) {
				continue
			}
			tokenIdx := s.externalToToken[externalIdx]
			if tokenIdx >= 0 && tokenIdx < liquidTokenCount {
				semanticValid[tokenIdx] = true
			}
		}
		validSymbols = semanticValid[:]
	}
	symbols := s.symbolTable()

	// Error recovery
	if liquidValid(validSymbols, liquidTokErrorSentinel) {
		return false
	}

	// Front matter: ---\n...\n---\n
	if liquidValid(validSymbols, liquidTokFrontMatter) {
		return liquidScanFrontMatter(lexer, symbols)
	}

	// Skip whitespace
	for unicode.IsSpace(lexer.Lookahead()) {
		lexer.Advance(true)
	}

	// Inline comment: # ... (until %} or newline)
	if liquidValid(validSymbols, liquidTokInlineCommentContent) {
		if lexer.Lookahead() == '#' {
			lexer.SetResultSymbol(symbols[liquidTokInlineCommentContent])
			lexer.Advance(false)
			for lexer.Lookahead() != 0 {
				lexer.MarkEnd()
				if lexer.Lookahead() == '\n' {
					lexer.Advance(false)
					lexer.MarkEnd()
					return true
				}
				if lexer.Lookahead() == '%' {
					lexer.Advance(false)
					if lexer.Lookahead() == '}' {
						lexer.Advance(false)
						return true
					}
				}
				lexer.Advance(false) // consume other chars
			}
		}
	}

	// Paired comment or raw content
	if liquidValid(validSymbols, liquidTokPairedCommentContent) ||
		liquidValid(validSymbols, liquidTokPairedCommentContentLiq) ||
		liquidValid(validSymbols, liquidTokRawContent) {
		return liquidScanPairedContent(lexer, validSymbols, symbols)
	}

	return false
}

func (s LiquidExternalScanner) symbolTable() *[liquidTokenCount]gotreesitter.Symbol {
	if s.symbols == ([liquidTokenCount]gotreesitter.Symbol{}) {
		return &liquidDefaultSymTable
	}
	return &s.symbols
}

func liquidScanFrontMatter(lexer *gotreesitter.ExternalLexer, symbols *[liquidTokenCount]gotreesitter.Symbol) bool {
	lexer.Advance(false)
	if lexer.Lookahead() != '-' {
		return false
	}
	lexer.Advance(false)
	if lexer.Lookahead() != '-' {
		return false
	}
	lexer.Advance(false)
	// Skip trailing spaces/tabs
	for lexer.Lookahead() == ' ' || lexer.Lookahead() == '\t' {
		lexer.Advance(false)
	}
	if lexer.Lookahead() != '\n' && lexer.Lookahead() != '\r' {
		return false
	}

	for {
		// Advance over newline
		if lexer.Lookahead() == '\r' {
			lexer.Advance(false)
			if lexer.Lookahead() == '\n' {
				lexer.Advance(false)
			}
		} else {
			lexer.Advance(false)
		}
		// Check for dashes
		dashCount := 0
		for lexer.Lookahead() == '-' {
			dashCount++
			lexer.Advance(false)
		}
		if dashCount == 3 {
			for lexer.Lookahead() == ' ' || lexer.Lookahead() == '\t' {
				lexer.Advance(false)
			}
			if lexer.Lookahead() == '\n' || lexer.Lookahead() == '\r' || lexer.Lookahead() == 0 {
				if lexer.Lookahead() == '\r' {
					lexer.Advance(false)
					if lexer.Lookahead() == '\n' {
						lexer.Advance(false)
					}
				} else if lexer.Lookahead() != 0 {
					lexer.Advance(false)
				}
				lexer.MarkEnd()
				lexer.SetResultSymbol(symbols[liquidTokFrontMatter])
				return true
			}
		}
		// Consume rest of line
		for lexer.Lookahead() != '\n' && lexer.Lookahead() != '\r' && lexer.Lookahead() != 0 {
			lexer.Advance(false)
		}
		if lexer.Lookahead() == 0 {
			return false
		}
	}
}

func liquidScanStr(lexer *gotreesitter.ExternalLexer, s string) bool {
	for _, ch := range s {
		if lexer.Lookahead() != ch {
			return false
		}
		lexer.Advance(false)
	}
	return true
}

func liquidScanPairedContent(lexer *gotreesitter.ExternalLexer, validSymbols []bool, symbols *[liquidTokenCount]gotreesitter.Symbol) bool {
	for lexer.Lookahead() != 0 {
		for unicode.IsSpace(lexer.Lookahead()) {
			lexer.Advance(true)
		}
		lexer.MarkEnd()

		if !liquidValid(validSymbols, liquidTokPairedCommentContentLiq) {
			if lexer.Lookahead() != '{' {
				lexer.Advance(false)
				continue
			}
			lexer.Advance(false)
			if lexer.Lookahead() != '%' {
				continue
			}
			lexer.Advance(false)
			if lexer.Lookahead() == '-' {
				lexer.Advance(false)
			}
			for unicode.IsSpace(lexer.Lookahead()) {
				lexer.Advance(true)
			}
		}

		// Try "end"
		if lexer.Lookahead() == 'e' {
			if !liquidScanStr(lexer, "end") {
				lexer.Advance(false)
				continue
			}
		}

		isRaw := liquidScanStr(lexer, "raw")
		isComment := liquidScanStr(lexer, "comment")

		if isComment && liquidValid(validSymbols, liquidTokPairedCommentContent) {
			lexer.SetResultSymbol(symbols[liquidTokPairedCommentContent])
		} else if isComment && liquidValid(validSymbols, liquidTokPairedCommentContentLiq) {
			lexer.SetResultSymbol(symbols[liquidTokPairedCommentContentLiq])
			return true
		} else if isRaw && liquidValid(validSymbols, liquidTokRawContent) {
			lexer.SetResultSymbol(symbols[liquidTokRawContent])
		} else {
			lexer.Advance(false)
			continue
		}

		for unicode.IsSpace(lexer.Lookahead()) {
			lexer.Advance(true)
		}
		if lexer.Lookahead() == '-' {
			lexer.Advance(false)
		}
		if lexer.Lookahead() != '%' {
			continue
		}
		lexer.Advance(false)
		if lexer.Lookahead() == '}' {
			lexer.Advance(false)
			return true
		}
	}
	return false
}

func liquidValid(vs []bool, i int) bool { return i < len(vs) && vs[i] }
