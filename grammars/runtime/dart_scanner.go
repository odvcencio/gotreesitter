//go:build !grammar_subset || grammar_subset_dart

package grammarruntime

import (
	"unicode"

	gotreesitter "github.com/odvcencio/gotreesitter"
)

// External token indexes for the dart grammar.
// NOTE: the upstream C enum uses a swapped ordering for DOUBLE/SINGLE
// compared to the grammar's externals array. We use the grammar order here,
// which matches the ExternalSymbols slice from the generated grammar.
const (
	dartTokTemplateCharsDouble       = 0 // "_template_chars_double"
	dartTokTemplateCharsSingle       = 1 // "_template_chars_single"
	dartTokTemplateCharsDoubleSingle = 2 // "_template_chars_double_single"
	dartTokTemplateCharsSingleSingle = 3 // "_template_chars_single_single"
	dartTokTemplateCharsRawSlash     = 4 // "_template_chars_raw_slash"
	dartTokBlockComment              = 5 // "_block_comment"
	dartTokDocBlockComment           = 6 // "_documentation_block_comment"
	dartTokenCount                   = 7
)

// Concrete symbol IDs from the generated dart grammar ExternalSymbols.
const (
	dartSymTemplateCharsDouble       gotreesitter.Symbol = 154
	dartSymTemplateCharsSingle       gotreesitter.Symbol = 155
	dartSymTemplateCharsDoubleSingle gotreesitter.Symbol = 156
	dartSymTemplateCharsSingleSingle gotreesitter.Symbol = 157
	dartSymTemplateCharsRawSlash     gotreesitter.Symbol = 158
	dartSymBlockComment              gotreesitter.Symbol = 159
	dartSymDocBlockComment           gotreesitter.Symbol = 160
)

var dartDefaultSymTable = [dartTokenCount]gotreesitter.Symbol{
	dartSymTemplateCharsDouble,
	dartSymTemplateCharsSingle,
	dartSymTemplateCharsDoubleSingle,
	dartSymTemplateCharsSingleSingle,
	dartSymTemplateCharsRawSlash,
	dartSymBlockComment,
	dartSymDocBlockComment,
}

var dartExternalSymbolNames = []string{
	"_template_chars_double",
	"_template_chars_single",
	"_template_chars_double_single",
	"_template_chars_single_single",
	"_template_chars_raw_slash",
	"_block_comment",
	"_documentation_block_comment",
}

// DartExternalScanner implements gotreesitter.ExternalScanner for tree-sitter-dart.
//
// This is a Go port of the C external scanner from UserNobody14/tree-sitter-dart.
// The scanner is stateless and handles:
//   - Template/string content tokens for 4 string variants (single/double x single/multi-line)
//   - Raw string backslash passthrough
//   - Nestable block comments (/* */ and /** */)
type DartExternalScanner struct {
	symbols         [dartTokenCount]gotreesitter.Symbol
	externalToToken []int
}

func (DartExternalScanner) ExternalScannerForLanguage(lang *gotreesitter.Language) gotreesitter.ExternalScanner {
	s := DartExternalScanner{symbols: dartDefaultSymTable}
	s.externalToToken = bindExternalScannerSymbolNames(lang, dartExternalSymbolNames, func(tokenIdx int, sym gotreesitter.Symbol) {
		s.symbols[tokenIdx] = sym
	})
	return s
}

func (DartExternalScanner) Create() any                           { return nil }
func (DartExternalScanner) Destroy(payload any)                   {}
func (DartExternalScanner) Serialize(payload any, buf []byte) int { return 0 }
func (DartExternalScanner) Deserialize(payload any, buf []byte)   {}
func (DartExternalScanner) SupportsIncrementalReuse() bool        { return true }

func (s DartExternalScanner) Scan(payload any, lexer *gotreesitter.ExternalLexer, validSymbols []bool) bool {
	var semanticValid [dartTokenCount]bool
	validSymbols = s.remapValidSymbols(validSymbols, &semanticValid)
	symbols := s.symbolTable()

	// Template/string content takes priority.
	if dartValid(validSymbols, dartTokTemplateCharsDouble) ||
		dartValid(validSymbols, dartTokTemplateCharsSingle) ||
		dartValid(validSymbols, dartTokTemplateCharsDoubleSingle) ||
		dartValid(validSymbols, dartTokTemplateCharsSingleSingle) {
		return dartScanTemplates(lexer, validSymbols, symbols)
	}

	// Skip whitespace before checking for block comments.
	for unicode.IsSpace(lexer.Lookahead()) {
		lexer.Advance(true)
	}

	if lexer.Lookahead() == '/' {
		return dartScanBlockComment(lexer, symbols)
	}

	return false
}

func (s DartExternalScanner) symbolTable() *[dartTokenCount]gotreesitter.Symbol {
	if s.symbols == ([dartTokenCount]gotreesitter.Symbol{}) {
		return &dartDefaultSymTable
	}
	return &s.symbols
}

func (s DartExternalScanner) remapValidSymbols(validSymbols []bool, semanticValid *[dartTokenCount]bool) []bool {
	if len(s.externalToToken) == 0 {
		return validSymbols
	}
	*semanticValid = [dartTokenCount]bool{}
	for externalIdx, valid := range validSymbols {
		if !valid || externalIdx >= len(s.externalToToken) {
			continue
		}
		tokenIdx := s.externalToToken[externalIdx]
		if tokenIdx >= 0 && tokenIdx < dartTokenCount {
			semanticValid[tokenIdx] = true
		}
	}
	return semanticValid[:]
}

// dartScanTemplates scans string literal content, stopping before
// characters that the grammar needs to handle (quotes, $, \).
func dartScanTemplates(lexer *gotreesitter.ExternalLexer, validSymbols []bool, symbols *[dartTokenCount]gotreesitter.Symbol) bool {
	// Determine which token to emit based on priority.
	lexer.SetResultSymbol(dartTemplateResultSymbol(validSymbols, symbols))

	isSingleLine := dartValid(validSymbols, dartTokTemplateCharsDoubleSingle) ||
		dartValid(validSymbols, dartTokTemplateCharsSingleSingle)

	hasContent := false
	for {
		lexer.MarkEnd()
		ch := lexer.Lookahead()
		switch ch {
		case '\'', '"':
			// Stop before closing quote — let the grammar match it.
			return hasContent
		case '\n':
			// Newlines are illegal in single-line strings.
			if isSingleLine {
				return false
			}
			lexer.Advance(false)
		case 0: // EOF
			return false
		case '$':
			// Stop before interpolation.
			return hasContent
		case '\\':
			if dartValid(validSymbols, dartTokTemplateCharsRawSlash) {
				// In raw strings, consume the backslash as literal content.
				lexer.SetResultSymbol(symbols[dartTokTemplateCharsRawSlash])
				lexer.Advance(false)
			} else {
				// Stop before escape sequence.
				return hasContent
			}
		default:
			lexer.Advance(false)
		}
		hasContent = true
	}
}

func dartTemplateResultSymbol(validSymbols []bool, symbols *[dartTokenCount]gotreesitter.Symbol) gotreesitter.Symbol {
	switch {
	case dartValid(validSymbols, dartTokTemplateCharsDouble):
		return symbols[dartTokTemplateCharsDouble]
	case dartValid(validSymbols, dartTokTemplateCharsSingle):
		return symbols[dartTokTemplateCharsSingle]
	case dartValid(validSymbols, dartTokTemplateCharsSingleSingle):
		return symbols[dartTokTemplateCharsSingleSingle]
	default:
		return symbols[dartTokTemplateCharsDoubleSingle]
	}
}

// dartScanBlockComment scans a nestable /* */ or /** */ comment.
func dartScanBlockComment(lexer *gotreesitter.ExternalLexer, symbols *[dartTokenCount]gotreesitter.Symbol) bool {
	// Expect '/'
	lexer.Advance(false)
	if lexer.Lookahead() != '*' {
		return false
	}
	lexer.Advance(false)

	// Check if this is a documentation comment (/** ...).
	isDoc := lexer.Lookahead() == '*'

	afterStar := false
	nestingDepth := 1

	for {
		ch := lexer.Lookahead()
		switch ch {
		case 0: // EOF — Dart does not accept unterminated comments.
			return false
		case '*':
			lexer.Advance(false)
			afterStar = true
		case '/':
			lexer.Advance(false)
			if afterStar {
				afterStar = false
				nestingDepth--
				if nestingDepth == 0 {
					lexer.MarkEnd()
					if isDoc {
						lexer.SetResultSymbol(symbols[dartTokDocBlockComment])
					} else {
						lexer.SetResultSymbol(symbols[dartTokBlockComment])
					}
					return true
				}
			} else {
				afterStar = false
				if lexer.Lookahead() == '*' {
					nestingDepth++
					lexer.Advance(false)
				}
			}
		default:
			lexer.Advance(false)
			afterStar = false
		}
	}
}

func dartValid(validSymbols []bool, idx int) bool {
	return idx >= 0 && idx < len(validSymbols) && validSymbols[idx]
}
