//go:build !grammar_subset || grammar_subset_doxygen

package grammarruntime

import gotreesitter "github.com/odvcencio/gotreesitter"

// External token indexes for the doxygen grammar.
// These must match the order in the grammar's externals array.
const (
	doxygenTokBriefText         = 0 // "brief_text" — text after @brief until EOL
	doxygenTokCodeBlockStart    = 1 // "code_block_start" — @code or \code marker
	doxygenTokCodeBlockLanguage = 2 // "code_block_language" — {.lang} after @code
	doxygenTokCodeBlockContent  = 3 // "code_block_content" — content until @endcode
	doxygenTokCodeBlockEnd      = 4 // "code_block_end" — @endcode or \endcode marker
)

// Concrete symbol IDs from the generated doxygen grammar ExternalSymbols.
const (
	doxygenSymBriefText         gotreesitter.Symbol = 42
	doxygenSymCodeBlockStart    gotreesitter.Symbol = 43
	doxygenSymCodeBlockLanguage gotreesitter.Symbol = 44
	doxygenSymCodeBlockContent  gotreesitter.Symbol = 45
	doxygenSymCodeBlockEnd      gotreesitter.Symbol = 46
)

// DoxygenExternalScanner implements gotreesitter.ExternalScanner for
// tree-sitter-doxygen. The doxygen grammar parses documentation comment
// body text (without the // or /* comment markers).
//
// Five external tokens are handled:
//   - brief_text: captures text after @brief/@short/\brief/\short until EOL
//   - code_block_start: matches @code or \code
//   - code_block_language: matches {.lang} immediately after code_block_start
//   - code_block_content: scans all text until @endcode or \endcode
//   - code_block_end: matches @endcode or \endcode
type DoxygenExternalScanner struct{}

func (DoxygenExternalScanner) Create() any                           { return nil }
func (DoxygenExternalScanner) Destroy(payload any)                   {}
func (DoxygenExternalScanner) Serialize(payload any, buf []byte) int { return 0 }
func (DoxygenExternalScanner) Deserialize(payload any, buf []byte)   {}

func (DoxygenExternalScanner) Scan(payload any, lexer *gotreesitter.ExternalLexer, validSymbols []bool) bool {
	isValid := func(idx int) bool {
		return idx >= 0 && idx < len(validSymbols) && validSymbols[idx]
	}

	// code_block_end: match @endcode or \endcode
	if isValid(doxygenTokCodeBlockEnd) {
		return scanDoxygenCodeBlockEnd(lexer)
	}

	// code_block_content: scan everything until @endcode/\endcode
	if isValid(doxygenTokCodeBlockContent) {
		return scanDoxygenCodeBlockContent(lexer)
	}

	// code_block_language: match {.lang}
	if isValid(doxygenTokCodeBlockLanguage) {
		return scanDoxygenCodeBlockLanguage(lexer)
	}

	// code_block_start: match @code or \code
	if isValid(doxygenTokCodeBlockStart) {
		return scanDoxygenCodeBlockStart(lexer)
	}

	// brief_text: scan text until end of line
	if isValid(doxygenTokBriefText) {
		return scanDoxygenBriefText(lexer)
	}

	return false
}

// scanDoxygenBriefText scans text until end of line or EOF.
// This captures the text content after @brief (the parser has already
// consumed the @brief tag itself).
func scanDoxygenBriefText(lexer *gotreesitter.ExternalLexer) bool {
	count := 0
	for {
		ch := lexer.Lookahead()
		if ch == 0 || ch == '\n' {
			break
		}
		lexer.Advance(false)
		count++
	}
	if count == 0 {
		return false
	}
	lexer.MarkEnd()
	lexer.SetResultSymbol(doxygenSymBriefText)
	return true
}

// scanDoxygenCodeBlockStart matches @code or \code at the current position.
func scanDoxygenCodeBlockStart(lexer *gotreesitter.ExternalLexer) bool {
	ch := lexer.Lookahead()
	if ch != '@' && ch != '\\' {
		return false
	}
	lexer.Advance(false)

	target := "code"
	for _, expected := range target {
		if lexer.Lookahead() != expected {
			return false
		}
		lexer.Advance(false)
	}

	// Make sure we're at a word boundary (not @codeword)
	next := lexer.Lookahead()
	if next != 0 && next != '{' && next != '\n' && next != '\r' && next != ' ' && next != '\t' {
		return false
	}

	lexer.MarkEnd()
	lexer.SetResultSymbol(doxygenSymCodeBlockStart)
	return true
}

// scanDoxygenCodeBlockLanguage matches {.lang} after @code.
func scanDoxygenCodeBlockLanguage(lexer *gotreesitter.ExternalLexer) bool {
	if lexer.Lookahead() != '{' {
		return false
	}
	lexer.Advance(false)

	if lexer.Lookahead() != '.' {
		return false
	}
	lexer.Advance(false)

	count := 0
	for {
		ch := lexer.Lookahead()
		if ch == '}' || ch == 0 || ch == '\n' {
			break
		}
		lexer.Advance(false)
		count++
	}
	if count == 0 {
		return false
	}

	if lexer.Lookahead() != '}' {
		return false
	}
	lexer.Advance(false)

	lexer.MarkEnd()
	lexer.SetResultSymbol(doxygenSymCodeBlockLanguage)
	return true
}

// scanDoxygenCodeBlockContent scans everything until @endcode or \endcode is found.
func scanDoxygenCodeBlockContent(lexer *gotreesitter.ExternalLexer) bool {
	count := 0
	for {
		ch := lexer.Lookahead()
		if ch == 0 {
			break
		}

		// Check for @endcode or \endcode
		if ch == '@' || ch == '\\' {
			// Try to match "endcode"
			lexer.MarkEnd()

			lexer.Advance(false)
			if matchWord(lexer, "endcode") {
				// Verify word boundary
				next := lexer.Lookahead()
				if next == 0 || next == '\n' || next == '\r' || next == ' ' || next == '\t' {
					// Found @endcode/\endcode, emit content up to here
					if count == 0 {
						return false
					}
					lexer.SetResultSymbol(doxygenSymCodeBlockContent)
					return true
				}
			}
			// Not @endcode, continue scanning
			count++
			continue
		}

		lexer.Advance(false)
		count++
	}

	// Reached EOF without finding @endcode
	if count == 0 {
		return false
	}
	lexer.MarkEnd()
	lexer.SetResultSymbol(doxygenSymCodeBlockContent)
	return true
}

// scanDoxygenCodeBlockEnd matches @endcode or \endcode at the current position.
func scanDoxygenCodeBlockEnd(lexer *gotreesitter.ExternalLexer) bool {
	ch := lexer.Lookahead()
	if ch != '@' && ch != '\\' {
		return false
	}
	lexer.Advance(false)

	if !matchWord(lexer, "endcode") {
		return false
	}

	// Verify word boundary
	next := lexer.Lookahead()
	if next != 0 && next != '\n' && next != '\r' && next != ' ' && next != '\t' {
		return false
	}

	lexer.MarkEnd()
	lexer.SetResultSymbol(doxygenSymCodeBlockEnd)
	return true
}

// matchWord checks if the lexer's current characters match the given word,
// advancing past each matching character. Returns false if any character
// doesn't match (leaving the lexer at the mismatching position).
func matchWord(lexer *gotreesitter.ExternalLexer, word string) bool {
	for _, expected := range word {
		if lexer.Lookahead() != expected {
			return false
		}
		lexer.Advance(false)
	}
	return true
}
