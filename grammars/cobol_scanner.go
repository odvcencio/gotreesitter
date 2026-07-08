//go:build !grammar_subset || grammar_subset_cobol

package grammars

import (
	"sync"
	"unicode"

	gotreesitter "github.com/odvcencio/gotreesitter"
)

// External token indexes for the COBOL grammar.
const (
	cobolTokWhiteSpaces       = 0
	cobolTokLinePrefixComment = 1
	cobolTokLineSuffixComment = 2
	cobolTokLineComment       = 3
	cobolTokCommentEntry      = 4
	cobolTokMultilineString   = 5
)

// cobolSyms caches resolved external symbol IDs for the COBOL grammar.
var cobolSyms struct {
	once              sync.Once
	whiteSpaces       gotreesitter.Symbol
	linePrefixComment gotreesitter.Symbol
	lineSuffixComment gotreesitter.Symbol
	lineComment       gotreesitter.Symbol
	commentEntry      gotreesitter.Symbol
	multilineString   gotreesitter.Symbol
}

func resolveCobolSyms() {
	cobolSyms.once.Do(func() {
		lang := CobolLanguage()
		cobolSyms.whiteSpaces = lang.ExternalSymbols[cobolTokWhiteSpaces]
		cobolSyms.linePrefixComment = lang.ExternalSymbols[cobolTokLinePrefixComment]
		cobolSyms.lineSuffixComment = lang.ExternalSymbols[cobolTokLineSuffixComment]
		cobolSyms.lineComment = lang.ExternalSymbols[cobolTokLineComment]
		cobolSyms.commentEntry = lang.ExternalSymbols[cobolTokCommentEntry]
		cobolSyms.multilineString = lang.ExternalSymbols[cobolTokMultilineString]
	})
}

// COBOL comment entry keywords (case-insensitive prefixes).
var cobolCommentEntryKeywords = []string{
	"author",
	"installlation",
	"date-written",
	"date-compiled",
	"security",
	"identification division",
	"environment division",
	"data division",
	"procedure division",
}

// CobolExternalScanner handles COBOL's column-based formatting.
type CobolExternalScanner struct{}

func (CobolExternalScanner) Create() any                           { return nil }
func (CobolExternalScanner) Destroy(payload any)                   {}
func (CobolExternalScanner) Serialize(payload any, buf []byte) int { return 0 }
func (CobolExternalScanner) Deserialize(payload any, buf []byte)   {}

func (CobolExternalScanner) Scan(payload any, lexer *gotreesitter.ExternalLexer, validSymbols []bool) bool {
	resolveCobolSyms()

	if lexer.Lookahead() == 0 {
		return false
	}

	// WHITE_SPACES: consume whitespace (including ; and ,)
	if cobolValid(validSymbols, cobolTokWhiteSpaces) {
		if cobolIsWhiteSpace(lexer.Lookahead()) {
			for cobolIsWhiteSpace(lexer.Lookahead()) {
				lexer.Advance(true)
			}
			lexer.MarkEnd()
			lexer.SetResultSymbol(cobolSyms.whiteSpaces)
			return true
		}
	}

	// LINE_PREFIX_COMMENT: columns 1-6 (0-5)
	if cobolValid(validSymbols, cobolTokLinePrefixComment) && lexer.Column() <= 5 {
		for lexer.Column() <= 5 && lexer.Lookahead() != 0 && lexer.Lookahead() != '\n' {
			lexer.Advance(true)
		}
		lexer.MarkEnd()
		lexer.SetResultSymbol(cobolSyms.linePrefixComment)
		return true
	}

	// LINE_COMMENT: column 7 (index 6) with * or /
	if cobolValid(validSymbols, cobolTokLineComment) {
		if lexer.Column() == 6 {
			if lexer.Lookahead() == '*' || lexer.Lookahead() == '/' {
				for lexer.Lookahead() != '\n' && lexer.Lookahead() != 0 {
					lexer.Advance(true)
				}
				lexer.MarkEnd()
				lexer.SetResultSymbol(cobolSyms.lineComment)
				return true
			}
			lexer.Advance(true)
			lexer.MarkEnd()
			return false
		}
	}

	// LINE_SUFFIX_COMMENT: column 73+ (index 72+)
	if cobolValid(validSymbols, cobolTokLineSuffixComment) {
		if lexer.Column() >= 72 {
			for lexer.Lookahead() != '\n' && lexer.Lookahead() != 0 {
				lexer.Advance(true)
			}
			lexer.MarkEnd()
			lexer.SetResultSymbol(cobolSyms.lineSuffixComment)
			return true
		}
	}

	// COMMENT_ENTRY: content that doesn't start with a known keyword
	if cobolValid(validSymbols, cobolTokCommentEntry) {
		if !cobolStartsWithKeyword(lexer) {
			// Never emit zero-width external tokens: they can cause scanner
			// spin loops under repeated external-token probes.
			if lexer.Lookahead() == 0 || lexer.Lookahead() == '\n' {
				return false
			}
			for lexer.Lookahead() != '\n' && lexer.Lookahead() != 0 && lexer.Column() < 72 {
				lexer.Advance(true)
			}
			lexer.MarkEnd()
			lexer.SetResultSymbol(cobolSyms.commentEntry)
			return true
		}
		return false
	}

	// MULTILINE_STRING: "..."  with continuation lines
	if cobolValid(validSymbols, cobolTokMultilineString) {
		for {
			if lexer.Lookahead() != '"' {
				return false
			}
			lexer.Advance(false)
			for lexer.Lookahead() != '"' && lexer.Lookahead() != 0 && lexer.Column() < 72 {
				lexer.Advance(false)
			}
			if lexer.Lookahead() == '"' {
				lexer.Advance(false)
				lexer.MarkEnd()
				lexer.SetResultSymbol(cobolSyms.multilineString)
				return true
			}
			// Skip to end of line
			for lexer.Lookahead() != 0 && lexer.Lookahead() != '\n' {
				lexer.Advance(true)
			}
			if lexer.Lookahead() == 0 {
				return false
			}
			lexer.Advance(true) // skip \n
			// Skip columns 1-6
			for i := 0; i <= 5; i++ {
				if lexer.Lookahead() == 0 || lexer.Lookahead() == '\n' {
					return false
				}
				lexer.Advance(true)
			}
			// Column 7 must be '-' for continuation
			if lexer.Lookahead() != '-' {
				return false
			}
			lexer.Advance(true)
			// Skip spaces to the continuation quote
			for lexer.Lookahead() == ' ' && lexer.Column() < 72 {
				lexer.Advance(true)
			}
		}
	}

	return false
}

func cobolIsWhiteSpace(c rune) bool {
	return unicode.IsSpace(c) || c == ';' || c == ','
}

// cobolStartsWithKeyword checks if the current line starts with any comment entry keyword.
func cobolStartsWithKeyword(lexer *gotreesitter.ExternalLexer) bool {
	// Skip leading whitespace
	for lexer.Lookahead() == ' ' || lexer.Lookahead() == '\t' {
		lexer.Advance(true)
	}

	// Try to match each keyword
	type tracker struct {
		keyword string
		pos     int
		active  bool
	}
	trackers := make([]tracker, len(cobolCommentEntryKeywords))
	for i, kw := range cobolCommentEntryKeywords {
		trackers[i] = tracker{keyword: kw, pos: 0, active: true}
	}

	for {
		if lexer.Column() > 71 || lexer.Lookahead() == '\n' || lexer.Lookahead() == 0 {
			return false
		}

		// Check if all matching has failed
		anyActive := false
		for i := range trackers {
			if trackers[i].active {
				anyActive = true
			}
		}
		if !anyActive {
			// Skip rest of line
			for lexer.Column() < 71 && lexer.Lookahead() != '\n' && lexer.Lookahead() != 0 {
				lexer.Advance(true)
			}
			return false
		}

		ch := lexer.Lookahead()

		// Check if any keyword completed
		for i := range trackers {
			if trackers[i].active && trackers[i].pos >= len(trackers[i].keyword) {
				return true
			}
		}

		// Advance matching
		for i := range trackers {
			if trackers[i].active {
				k := rune(trackers[i].keyword[trackers[i].pos])
				trackers[i].active = cobolCIMatch(ch, k)
				trackers[i].pos++
			}
		}

		lexer.Advance(true)
	}
}

func cobolCIMatch(a, b rune) bool {
	return unicode.ToUpper(a) == unicode.ToUpper(b)
}

func cobolValid(vs []bool, i int) bool { return i < len(vs) && vs[i] }
