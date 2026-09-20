//go:build !grammar_subset || grammar_subset_doxygen

package grammarruntime

import gotreesitter "github.com/odvcencio/gotreesitter"

// External token indexes for the doxygen grammar. This is the external index
// (the position of the token in the grammar's `externals: [...]` list),
// which is exactly what tree-sitter's `valid_symbols` array and C's
// result_symbol enum are indexed by. The external index is stable across a
// blob regen as long as the externals list itself does not reorder;
// concrete numeric gotreesitter.Symbol IDs are NOT stable (they shift
// whenever the grammar's total symbol count changes), so this scanner never
// hardcodes them -- see doxygenDefaultSymTable and the symbols field on
// DoxygenExternalScanner below.
//
// NOTE on scope: upstream's code_block_start/code_block_end externals fire
// for a Markdown-style triple-backtick fence (an alternate spelling of a
// doxygen code block, alongside @code/@endcode); the upstream diff this port
// tracks (isalpha -> iswalpha/iswalnum, see doxygenExternalScannerSpec) fixes
// Unicode-lookahead handling on that backtick path only. This port instead
// recognizes @code/\code and @endcode/\endcode directly as the
// code_block_start/code_block_end tokens and never scans a backtick fence,
// so the upstream diff's isalpha/isalnum hunks have no Go counterpart to
// change; the two ports diverge on backtick-fenced code blocks, which is
// unchanged by this commit.
const (
	doxygenTokBriefText         = iota // "brief_text" — text after @brief until EOL
	doxygenTokCodeBlockStart           // "code_block_start" — @code or \code marker
	doxygenTokCodeBlockLanguage        // "code_block_language" — {.lang} after @code
	doxygenTokCodeBlockContent         // "code_block_content" — content until @endcode
	doxygenTokCodeBlockEnd             // "code_block_end" — @endcode or \endcode marker
	doxygenTokenCount
)

// doxygenDefaultSymTable records the concrete gotreesitter.Symbol IDs the
// currently shipped doxygen.bin assigns to each external, in doxygenTok*
// order. It exists only as a pre-bind fallback (and as an independent value
// to compare a real bind against in tests); ExternalScannerForLanguage below
// overwrites it with values read from the actual loaded Language at bind
// time, which is what the scanner must do to survive a future blob regen
// that renumbers absolute symbol IDs without touching the externals list
// order.
var doxygenDefaultSymTable = [doxygenTokenCount]gotreesitter.Symbol{
	doxygenTokBriefText:         42,
	doxygenTokCodeBlockStart:    43,
	doxygenTokCodeBlockLanguage: 44,
	doxygenTokCodeBlockContent:  45,
	doxygenTokCodeBlockEnd:      46,
}

// doxygenExternalScannerSpec records the source contract this port tracks.
// Its Externals list is also the binding source for
// ExternalScannerForLanguage: index i here is scanner token index i
// (doxygenTok* order), matching upstream tree-sitter-doxygen's
// `externals: [...]` order.
var doxygenExternalScannerSpec = ExternalScannerSpec{
	Language:       "doxygen",
	UpstreamRepo:   "https://github.com/amaanq/tree-sitter-doxygen",
	UpstreamCommit: "6069b1815b139080d6c562b5ff9ae2296cbc6602",
	SourceFiles: []ExternalScannerSourceFile{
		{Path: "src/grammar.json", SHA256: "fab2d4a8542769a35792ce8439ee8fd5aeadb3f795c2daf51c036c74a10af242"},
		{Path: "src/scanner.c", SHA256: "82da466d31ba874cd680fad143f60609ab7af46e7d20352dee9538c13761f66e"},
	},
	Externals: []string{
		"brief_text",
		"code_block_start",
		"code_block_language",
		"code_block_content",
		"code_block_end",
	},
}

func init() {
	RegisterExternalScannerSpec(doxygenExternalScannerSpec)
}

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
type DoxygenExternalScanner struct {
	symbols         [doxygenTokenCount]gotreesitter.Symbol
	externalToToken []int
}

// ExternalScannerForLanguage binds this scanner's token slots to lang's
// concrete external symbol IDs so Scan reports the IDs the parser table
// actually expects, instead of IDs frozen at some earlier grammar revision.
func (DoxygenExternalScanner) ExternalScannerForLanguage(lang *gotreesitter.Language) gotreesitter.ExternalScanner {
	s := DoxygenExternalScanner{symbols: doxygenDefaultSymTable}
	s.externalToToken = bindExternalScannerSpec(lang, doxygenExternalScannerSpec, func(tokenIdx int, sym gotreesitter.Symbol) {
		s.symbols[tokenIdx] = sym
	})
	return s
}

func (DoxygenExternalScanner) Create() any                           { return nil }
func (DoxygenExternalScanner) Destroy(payload any)                   {}
func (DoxygenExternalScanner) Serialize(payload any, buf []byte) int { return 0 }

// Deserialize is a no-op: this port carries no scanner payload (Create
// returns nil), so there is nothing to restore regardless of buffer length.
// Upstream amaanq/tree-sitter-doxygen@6069b1815b13 dropped the C scanner's
// invalid-length defensive branch (an fprintf+abort guarding a "should never
// happen" buffer length) in favor of a debug-only assert; this port already
// has no equivalent branch to remove, since it never inspects buf's length.
func (DoxygenExternalScanner) Deserialize(payload any, buf []byte) {}

// This port intentionally does not declare SupportsIncrementalReuse,
// ExternalScannerIsStateless, or PreservesStateOnScanFailure. The scanner
// looks stateless by inspection (Create returns nil; Scan reads only
// lookahead and validSymbols), but certifying incremental-reuse and
// quiescence properties is a separate, dedicated audit from binding
// conversion; declaring them here would silently flip doxygen's
// previously-unaudited default (incremental reuse unsupported) without that
// audit. See d_scanner.go/powershell_scanner.go for the audited version of
// this contract.
func (s DoxygenExternalScanner) symbolTable() *[doxygenTokenCount]gotreesitter.Symbol {
	if s.symbols == ([doxygenTokenCount]gotreesitter.Symbol{}) {
		return &doxygenDefaultSymTable
	}
	return &s.symbols
}

// remapValidSymbols translates the parser's external-index-space validSymbols
// slice into this scanner's token-index space via externalToToken, matching
// the pattern used by the other positionally bound scanners in this package
// (see d_scanner.go, ocaml_scanner.go).
func (s DoxygenExternalScanner) remapValidSymbols(validSymbols []bool, semanticValid *[doxygenTokenCount]bool) []bool {
	if len(s.externalToToken) == 0 {
		return validSymbols
	}
	*semanticValid = [doxygenTokenCount]bool{}
	for externalIdx, valid := range validSymbols {
		if !valid || externalIdx >= len(s.externalToToken) {
			continue
		}
		tokenIdx := s.externalToToken[externalIdx]
		if tokenIdx >= 0 && tokenIdx < doxygenTokenCount {
			semanticValid[tokenIdx] = true
		}
	}
	return semanticValid[:]
}

func (s DoxygenExternalScanner) Scan(payload any, lexer *gotreesitter.ExternalLexer, validSymbols []bool) bool {
	var semanticValid [doxygenTokenCount]bool
	validSymbols = s.remapValidSymbols(validSymbols, &semanticValid)
	symbols := s.symbolTable()

	isValid := func(idx int) bool {
		return idx >= 0 && idx < len(validSymbols) && validSymbols[idx]
	}

	// code_block_end: match @endcode or \endcode
	if isValid(doxygenTokCodeBlockEnd) {
		return scanDoxygenCodeBlockEnd(lexer, symbols)
	}

	// code_block_content: scan everything until @endcode/\endcode
	if isValid(doxygenTokCodeBlockContent) {
		return scanDoxygenCodeBlockContent(lexer, symbols)
	}

	// code_block_language: match {.lang}
	if isValid(doxygenTokCodeBlockLanguage) {
		return scanDoxygenCodeBlockLanguage(lexer, symbols)
	}

	// code_block_start: match @code or \code
	if isValid(doxygenTokCodeBlockStart) {
		return scanDoxygenCodeBlockStart(lexer, symbols)
	}

	// brief_text: scan text until end of line
	if isValid(doxygenTokBriefText) {
		return scanDoxygenBriefText(lexer, symbols)
	}

	return false
}

// scanDoxygenBriefText scans text until end of line or EOF.
// This captures the text content after @brief (the parser has already
// consumed the @brief tag itself).
func scanDoxygenBriefText(lexer *gotreesitter.ExternalLexer, symbols *[doxygenTokenCount]gotreesitter.Symbol) bool {
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
	lexer.SetResultSymbol(symbols[doxygenTokBriefText])
	return true
}

// scanDoxygenCodeBlockStart matches @code or \code at the current position.
func scanDoxygenCodeBlockStart(lexer *gotreesitter.ExternalLexer, symbols *[doxygenTokenCount]gotreesitter.Symbol) bool {
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
	lexer.SetResultSymbol(symbols[doxygenTokCodeBlockStart])
	return true
}

// scanDoxygenCodeBlockLanguage matches {.lang} after @code.
func scanDoxygenCodeBlockLanguage(lexer *gotreesitter.ExternalLexer, symbols *[doxygenTokenCount]gotreesitter.Symbol) bool {
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
	lexer.SetResultSymbol(symbols[doxygenTokCodeBlockLanguage])
	return true
}

// scanDoxygenCodeBlockContent scans everything until @endcode or \endcode is found.
func scanDoxygenCodeBlockContent(lexer *gotreesitter.ExternalLexer, symbols *[doxygenTokenCount]gotreesitter.Symbol) bool {
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
					lexer.SetResultSymbol(symbols[doxygenTokCodeBlockContent])
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
	lexer.SetResultSymbol(symbols[doxygenTokCodeBlockContent])
	return true
}

// scanDoxygenCodeBlockEnd matches @endcode or \endcode at the current position.
func scanDoxygenCodeBlockEnd(lexer *gotreesitter.ExternalLexer, symbols *[doxygenTokenCount]gotreesitter.Symbol) bool {
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
	lexer.SetResultSymbol(symbols[doxygenTokCodeBlockEnd])
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
