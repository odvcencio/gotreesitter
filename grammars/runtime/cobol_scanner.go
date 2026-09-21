//go:build !grammar_subset || grammar_subset_cobol

package grammarruntime

import (
	"unicode"

	gotreesitter "github.com/odvcencio/gotreesitter"
)

// External token indexes for the COBOL grammar. This is the external index
// (the position of the token in the grammar's `externals: [...]` list),
// which is exactly what tree-sitter's `valid_symbols` array and C's
// result_symbol enum are indexed by. The external index is stable across a
// blob regen as long as the externals list itself does not reorder;
// concrete numeric gotreesitter.Symbol IDs are NOT stable (they shift
// whenever the grammar's total symbol count changes), so this scanner never
// hardcodes them -- see cobolDefaultSymTable below.
const (
	cobolTokWhiteSpaces       = 0
	cobolTokLinePrefixComment = 1
	cobolTokLineSuffixComment = 2
	cobolTokLineComment       = 3
	cobolTokCommentEntry      = 4
	cobolTokMultilineString   = 5
	cobolTokenCount           = 6
)

// cobolDefaultSymTable records the concrete gotreesitter.Symbol IDs the
// currently shipped cobol.bin assigns to each external, in cobolTok* order.
// It exists only as a pre-bind fallback (and as an independent value to
// compare a real bind against in tests); ExternalScannerForLanguage below
// overwrites it with values read from the actual loaded Language at bind
// time, which is what the scanner must do to survive a future blob regen
// that renumbers absolute symbol IDs without touching the externals list
// order.
var cobolDefaultSymTable = [cobolTokenCount]gotreesitter.Symbol{
	579, // _WHITE_SPACES
	580, // _LINE_PREFIX_COMMENT
	581, // _LINE_SUFFIX_COMMENT
	582, // _LINE_COMMENT
	583, // comment_entry
	584, // _multiline_string
}

// cobolExternalScannerSpec records the source contract for this
// hand-written port, so updater tooling can tell a grammar-only upstream
// change apart from one that also touches the external scanner or its
// token list. Its Externals list is also the binding source for
// ExternalScannerForLanguage: index i here is scanner token index i
// (cobolTok* order).
var cobolExternalScannerSpec = ExternalScannerSpec{
	Language:       "cobol",
	UpstreamRepo:   "https://github.com/yutaro-sakamoto/tree-sitter-cobol",
	UpstreamCommit: "e99dbdc3d800d5fa2796476efd60af91f6b43d93",
	SourceFiles: []ExternalScannerSourceFile{
		{Path: "src/grammar.json", SHA256: "5ddb58fbfefca22dbbcae0f6b5a8d2ad406dc507b392106e26f2b5fd711010af"},
		{Path: "src/scanner.c", SHA256: "7d9e24b1ba0c134997a58dbfaf4441bad68a3eaaec83dd5ba84bb7a13b5caa72"},
	},
	Externals: []string{
		"_WHITE_SPACES",
		"_LINE_PREFIX_COMMENT",
		"_LINE_SUFFIX_COMMENT",
		"_LINE_COMMENT",
		"comment_entry",
		"_multiline_string",
	},
}

func init() {
	RegisterExternalScannerSpec(cobolExternalScannerSpec)
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
//
// symbols holds the concrete gotreesitter.Symbol each external index maps to
// in the Language this instance was bound to (see ExternalScannerForLanguage).
// The scanner never hardcodes an absolute Symbol value: a blob regen can
// renumber the grammar's absolute symbol IDs without touching the externals
// list order, and a scanner that still called SetResultSymbol with a stale
// hardcoded ID would silently emit the wrong (but still structurally valid)
// node type instead of failing loudly.
type CobolExternalScanner struct {
	symbols         [cobolTokenCount]gotreesitter.Symbol
	externalToToken []int
}

// ExternalScannerForLanguage binds the scanner's token slots to the loaded
// Language's ExternalSymbols positionally. A hardcoded absolute
// gotreesitter.Symbol constant here would emit the wrong token whenever a
// grammar bump renumbers cobol's external symbols.
func (CobolExternalScanner) ExternalScannerForLanguage(lang *gotreesitter.Language) gotreesitter.ExternalScanner {
	s := CobolExternalScanner{symbols: cobolDefaultSymTable}
	s.externalToToken = bindExternalScannerSpec(lang, cobolExternalScannerSpec, func(tokenIdx int, sym gotreesitter.Symbol) {
		s.symbols[tokenIdx] = sym
	})
	return s
}

func (s CobolExternalScanner) symbolTable() *[cobolTokenCount]gotreesitter.Symbol {
	if s.symbols == ([cobolTokenCount]gotreesitter.Symbol{}) {
		return &cobolDefaultSymTable
	}
	return &s.symbols
}

func (CobolExternalScanner) Create() any                           { return nil }
func (CobolExternalScanner) Destroy(payload any)                   {}
func (CobolExternalScanner) Serialize(payload any, buf []byte) int { return 0 }
func (CobolExternalScanner) Deserialize(payload any, buf []byte)   {}

func (sc CobolExternalScanner) Scan(payload any, lexer *gotreesitter.ExternalLexer, validSymbols []bool) bool {
	if len(sc.externalToToken) > 0 {
		var semanticValid [cobolTokenCount]bool
		for externalIdx, valid := range validSymbols {
			if !valid || externalIdx >= len(sc.externalToToken) {
				continue
			}
			tokenIdx := sc.externalToToken[externalIdx]
			if tokenIdx >= 0 && tokenIdx < cobolTokenCount {
				semanticValid[tokenIdx] = true
			}
		}
		validSymbols = semanticValid[:]
	}
	syms := sc.symbolTable()

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
			lexer.SetResultSymbol(syms[cobolTokWhiteSpaces])
			return true
		}
	}

	// LINE_PREFIX_COMMENT: columns 1-6 (0-5)
	if cobolValid(validSymbols, cobolTokLinePrefixComment) && lexer.Column() <= 5 {
		for lexer.Column() <= 5 && lexer.Lookahead() != 0 && lexer.Lookahead() != '\n' {
			lexer.Advance(true)
		}
		lexer.MarkEnd()
		lexer.SetResultSymbol(syms[cobolTokLinePrefixComment])
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
				lexer.SetResultSymbol(syms[cobolTokLineComment])
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
			lexer.SetResultSymbol(syms[cobolTokLineSuffixComment])
			return true
		}
	}

	// COMMENT_ENTRY: content that doesn't start with a known keyword
	if cobolValid(validSymbols, cobolTokCommentEntry) {
		if !cobolStartsWithKeyword(lexer) {
			lexer.MarkEnd()
			lexer.SetResultSymbol(syms[cobolTokCommentEntry])
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
				lexer.SetResultSymbol(syms[cobolTokMultilineString])
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
