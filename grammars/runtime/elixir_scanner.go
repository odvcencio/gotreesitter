//go:build !grammar_subset || grammar_subset_elixir

package grammarruntime

import gotreesitter "github.com/odvcencio/gotreesitter"

// External token indexes for the Elixir grammar. This is the external index
// (the position of the token in the grammar's `externals: [...]` list),
// which is exactly what tree-sitter's `valid_symbols` array and C's
// result_symbol enum are indexed by. The external index is stable across a
// blob regen as long as the externals list itself does not reorder;
// concrete numeric gotreesitter.Symbol IDs are NOT stable (they shift
// whenever the grammar's total symbol count changes), so this scanner never
// hardcodes them -- see elixirDefaultSymTable below.
const (
	elixirTokQuotedContentISingle = iota
	elixirTokQuotedContentIDouble
	elixirTokQuotedContentIHeredocSingle
	elixirTokQuotedContentIHeredocDouble
	elixirTokQuotedContentIParenthesis
	elixirTokQuotedContentICurly
	elixirTokQuotedContentISquare
	elixirTokQuotedContentIAngle
	elixirTokQuotedContentIBar
	elixirTokQuotedContentISlash
	elixirTokQuotedContentSingle
	elixirTokQuotedContentDouble
	elixirTokQuotedContentHeredocSingle
	elixirTokQuotedContentHeredocDouble
	elixirTokQuotedContentParenthesis
	elixirTokQuotedContentCurly
	elixirTokQuotedContentSquare
	elixirTokQuotedContentAngle
	elixirTokQuotedContentBar
	elixirTokQuotedContentSlash
	elixirTokNewlineBeforeDo
	elixirTokNewlineBeforeBinaryOperator
	elixirTokNewlineBeforeComment
	elixirTokBeforeUnaryOperator
	elixirTokNotIn
	elixirTokQuotedAtomStart
	elixirTokenCount
)

// elixirDefaultSymTable records the concrete gotreesitter.Symbol IDs the
// currently shipped elixir.bin assigns to each external, in elixirTok*
// order. It exists only as a pre-bind fallback (and as an independent value
// to compare a real bind against in tests); ExternalScannerForLanguage below
// overwrites it with values read from the actual loaded Language at bind
// time, which is what the scanner must do to survive a future blob regen
// that renumbers absolute symbol IDs without touching the externals list
// order.
//
// All 26 externals sit in one contiguous run (98-123); the 20
// _quoted_content_* variants alias to the same visible "quoted_content"
// node type.
var elixirDefaultSymTable = [elixirTokenCount]gotreesitter.Symbol{
	98,  // _quoted_content_i_single
	99,  // _quoted_content_i_double
	100, // _quoted_content_i_heredoc_single
	101, // _quoted_content_i_heredoc_double
	102, // _quoted_content_i_parenthesis
	103, // _quoted_content_i_curly
	104, // _quoted_content_i_square
	105, // _quoted_content_i_angle
	106, // _quoted_content_i_bar
	107, // _quoted_content_i_slash
	108, // _quoted_content_single
	109, // _quoted_content_double
	110, // _quoted_content_heredoc_single
	111, // _quoted_content_heredoc_double
	112, // _quoted_content_parenthesis
	113, // _quoted_content_curly
	114, // _quoted_content_square
	115, // _quoted_content_angle
	116, // _quoted_content_bar
	117, // _quoted_content_slash
	118, // _newline_before_do
	119, // _newline_before_binary_operator
	120, // _newline_before_comment
	121, // _before_unary_op
	122, // _not_in (display: "not in")
	123, // _quoted_atom_start (display: ":")
}

// elixirExternalScannerSpec records the source contract for this
// hand-written port, so updater tooling can tell a grammar-only upstream
// change apart from one that also touches the external scanner or its token
// list. Its Externals list is also the binding source for
// ExternalScannerForLanguage: index i here is scanner token index i
// (elixirTok* order).
var elixirExternalScannerSpec = ExternalScannerSpec{
	Language:       "elixir",
	UpstreamRepo:   "https://github.com/elixir-lang/tree-sitter-elixir",
	UpstreamCommit: "4b0c7118760af58a2e7081bbc8396e136f820b37",
	SourceFiles: []ExternalScannerSourceFile{
		{Path: "src/grammar.json", SHA256: "320c4af03d6d34085c744ac9923d9b963848bd639a97c5f04401472931b059c4"},
		{Path: "src/scanner.c", SHA256: "2d49d8974f7fdf0d38bdb7ed792f7a60aa87dd087b0c0bb469cd5b138b08ba57"},
	},
	Externals: []string{
		"_quoted_content_i_single",
		"_quoted_content_i_double",
		"_quoted_content_i_heredoc_single",
		"_quoted_content_i_heredoc_double",
		"_quoted_content_i_parenthesis",
		"_quoted_content_i_curly",
		"_quoted_content_i_square",
		"_quoted_content_i_angle",
		"_quoted_content_i_bar",
		"_quoted_content_i_slash",
		"_quoted_content_single",
		"_quoted_content_double",
		"_quoted_content_heredoc_single",
		"_quoted_content_heredoc_double",
		"_quoted_content_parenthesis",
		"_quoted_content_curly",
		"_quoted_content_square",
		"_quoted_content_angle",
		"_quoted_content_bar",
		"_quoted_content_slash",
		"_newline_before_do",
		"_newline_before_binary_operator",
		"_newline_before_comment",
		"_before_unary_op",
		"_not_in",
		"_quoted_atom_start",
	},
}

func init() {
	RegisterExternalScannerSpec(elixirExternalScannerSpec)
}

type elixirQuotedContentInfo struct {
	tokenType        int
	supportsInterpol bool
	endDelimiter     rune
	delimiterLength  int
}

var elixirQuotedContentInfos = []elixirQuotedContentInfo{
	{elixirTokQuotedContentISingle, true, '\'', 1},
	{elixirTokQuotedContentIDouble, true, '"', 1},
	{elixirTokQuotedContentIHeredocSingle, true, '\'', 3},
	{elixirTokQuotedContentIHeredocDouble, true, '"', 3},
	{elixirTokQuotedContentIParenthesis, true, ')', 1},
	{elixirTokQuotedContentICurly, true, '}', 1},
	{elixirTokQuotedContentISquare, true, ']', 1},
	{elixirTokQuotedContentIAngle, true, '>', 1},
	{elixirTokQuotedContentIBar, true, '|', 1},
	{elixirTokQuotedContentISlash, true, '/', 1},
	{elixirTokQuotedContentSingle, false, '\'', 1},
	{elixirTokQuotedContentDouble, false, '"', 1},
	{elixirTokQuotedContentHeredocSingle, false, '\'', 3},
	{elixirTokQuotedContentHeredocDouble, false, '"', 3},
	{elixirTokQuotedContentParenthesis, false, ')', 1},
	{elixirTokQuotedContentCurly, false, '}', 1},
	{elixirTokQuotedContentSquare, false, ']', 1},
	{elixirTokQuotedContentAngle, false, '>', 1},
	{elixirTokQuotedContentBar, false, '|', 1},
	{elixirTokQuotedContentSlash, false, '/', 1},
}

// ElixirExternalScanner implements gotreesitter.ExternalScanner for
// tree-sitter-elixir.
//
// symbols holds the concrete gotreesitter.Symbol each external index maps to
// in the Language this instance was bound to (see ExternalScannerForLanguage).
// The scanner never hardcodes an absolute Symbol value: a blob regen can
// renumber the grammar's absolute symbol IDs without touching the externals
// list order, and a scanner that still called SetResultSymbol with a stale
// hardcoded ID would silently emit the wrong (but still structurally valid)
// node type instead of failing loudly.
type ElixirExternalScanner struct {
	symbols         [elixirTokenCount]gotreesitter.Symbol
	externalToToken []int
}

// ExternalScannerForLanguage binds the scanner's token slots to the loaded
// Language's ExternalSymbols positionally. A hardcoded absolute
// gotreesitter.Symbol constant here would emit the wrong token whenever a
// grammar bump renumbers elixir's external symbols.
func (ElixirExternalScanner) ExternalScannerForLanguage(lang *gotreesitter.Language) gotreesitter.ExternalScanner {
	s := ElixirExternalScanner{symbols: elixirDefaultSymTable}
	s.externalToToken = bindExternalScannerSpec(lang, elixirExternalScannerSpec, func(tokenIdx int, sym gotreesitter.Symbol) {
		s.symbols[tokenIdx] = sym
	})
	return s
}

func (s ElixirExternalScanner) symbolTable() *[elixirTokenCount]gotreesitter.Symbol {
	if s.symbols == ([elixirTokenCount]gotreesitter.Symbol{}) {
		return &elixirDefaultSymTable
	}
	return &s.symbols
}

func (ElixirExternalScanner) Create() any                           { return nil }
func (ElixirExternalScanner) Destroy(payload any)                   {}
func (ElixirExternalScanner) Serialize(payload any, buf []byte) int { return 0 }
func (ElixirExternalScanner) Deserialize(payload any, buf []byte)   {}

// The scanner carries no payload and derives every result from local
// lookahead plus validSymbols, so every incremental boundary is quiescent and
// failed scans cannot mutate persistent state.
func (ElixirExternalScanner) SupportsIncrementalReuse() bool    { return true }
func (ElixirExternalScanner) ExternalScannerIsStateless() bool  { return true }
func (ElixirExternalScanner) PreservesStateOnScanFailure() bool { return true }

func (s ElixirExternalScanner) Scan(payload any, lexer *gotreesitter.ExternalLexer, validSymbols []bool) bool {
	if len(s.externalToToken) > 0 {
		var semanticValid [elixirTokenCount]bool
		for externalIdx, valid := range validSymbols {
			if !valid || externalIdx >= len(s.externalToToken) {
				continue
			}
			tokenIdx := s.externalToToken[externalIdx]
			if tokenIdx >= 0 && tokenIdx < elixirTokenCount {
				semanticValid[tokenIdx] = true
			}
		}
		validSymbols = semanticValid[:]
	}
	return scanElixir(lexer, validSymbols, s.symbolTable())
}

func scanElixir(lexer *gotreesitter.ExternalLexer, validSymbols []bool, syms *[elixirTokenCount]gotreesitter.Symbol) bool {
	quotedInfoIdx := findElixirQuotedTokenInfo(validSymbols)
	if quotedInfoIdx != -1 {
		info := elixirQuotedContentInfos[quotedInfoIdx]
		return scanElixirQuotedContent(lexer, info, syms[info.tokenType])
	}

	skippedWhitespace := false
	for isElixirInlineWhitespace(lexer.Lookahead()) {
		skippedWhitespace = true
		lexer.Advance(true)
	}

	if isElixirNewline(lexer.Lookahead()) &&
		(isElixirValid(validSymbols, elixirTokNewlineBeforeDo) ||
			isElixirValid(validSymbols, elixirTokNewlineBeforeBinaryOperator) ||
			isElixirValid(validSymbols, elixirTokNewlineBeforeComment)) {
		return scanElixirNewline(lexer, validSymbols, syms)
	}

	switch lexer.Lookahead() {
	case '+':
		if skippedWhitespace && isElixirValid(validSymbols, elixirTokBeforeUnaryOperator) {
			lexer.MarkEnd()
			lexer.Advance(false)
			if lexer.Lookahead() == '+' || lexer.Lookahead() == ':' || lexer.Lookahead() == '/' {
				return false
			}
			if isElixirWhitespace(lexer.Lookahead()) {
				return false
			}
			lexer.SetResultSymbol(syms[elixirTokBeforeUnaryOperator])
			return true
		}
	case '-':
		if skippedWhitespace && isElixirValid(validSymbols, elixirTokBeforeUnaryOperator) {
			lexer.MarkEnd()
			lexer.SetResultSymbol(syms[elixirTokBeforeUnaryOperator])
			lexer.Advance(false)
			if lexer.Lookahead() == '-' || lexer.Lookahead() == '>' || lexer.Lookahead() == ':' || lexer.Lookahead() == '/' {
				return false
			}
			if isElixirWhitespace(lexer.Lookahead()) {
				return false
			}
			return true
		}
	case 'n':
		if isElixirValid(validSymbols, elixirTokNotIn) {
			lexer.Advance(false)
			if lexer.Lookahead() == 'o' {
				lexer.Advance(false)
				if lexer.Lookahead() == 't' {
					lexer.Advance(false)
					for isElixirInlineWhitespace(lexer.Lookahead()) {
						lexer.Advance(false)
					}
					if lexer.Lookahead() == 'i' {
						lexer.Advance(false)
						if lexer.Lookahead() == 'n' {
							lexer.Advance(false)
							if isElixirTokenEnd(lexer.Lookahead()) {
								lexer.MarkEnd()
								lexer.SetResultSymbol(syms[elixirTokNotIn])
								return true
							}
						}
					}
				}
			}
		}
	case ':':
		if isElixirValid(validSymbols, elixirTokQuotedAtomStart) {
			lexer.Advance(false)
			lexer.MarkEnd()
			lexer.SetResultSymbol(syms[elixirTokQuotedAtomStart])
			if lexer.Lookahead() == '"' || lexer.Lookahead() == '\'' {
				return true
			}
		}
	}

	return false
}

func isElixirValid(validSymbols []bool, idx int) bool {
	return idx >= 0 && idx < len(validSymbols) && validSymbols[idx]
}

func isElixirWhitespace(c rune) bool {
	return c == ' ' || c == '\t' || c == '\n' || c == '\r'
}

func isElixirInlineWhitespace(c rune) bool {
	return c == ' ' || c == '\t'
}

func isElixirNewline(c rune) bool {
	return c == '\n' || c == '\r'
}

func isElixirDigit(c rune) bool {
	return c >= '0' && c <= '9'
}

func checkElixirKeywordEnd(lexer *gotreesitter.ExternalLexer) bool {
	if lexer.Lookahead() == ':' {
		lexer.Advance(false)
		return isElixirWhitespace(lexer.Lookahead())
	}
	return false
}

func checkElixirOperatorEnd(lexer *gotreesitter.ExternalLexer) bool {
	if lexer.Lookahead() == ':' {
		return !checkElixirKeywordEnd(lexer)
	}
	for isElixirInlineWhitespace(lexer.Lookahead()) {
		lexer.Advance(false)
	}
	if lexer.Lookahead() == '/' {
		lexer.Advance(false)
		for isElixirWhitespace(lexer.Lookahead()) {
			lexer.Advance(false)
		}
		if isElixirDigit(lexer.Lookahead()) {
			return false
		}
	}
	return true
}

var elixirTokenTerminators = []rune{
	'@', '.', '+', '-', '^', '-', '*', '/', '<', '>', '|', '~', '=', '&', '\\', '%',
	'{', '}', '[', ']', '(', ')', '"', '\'',
	',', ';',
	'#',
}

func isElixirTokenEnd(c rune) bool {
	for _, t := range elixirTokenTerminators {
		if c == t {
			return true
		}
	}
	return isElixirWhitespace(c)
}

func findElixirQuotedTokenInfo(validSymbols []bool) int {
	if isElixirValid(validSymbols, elixirTokQuotedContentISingle) &&
		isElixirValid(validSymbols, elixirTokQuotedContentIDouble) {
		return -1
	}
	for i, info := range elixirQuotedContentInfos {
		if isElixirValid(validSymbols, info.tokenType) {
			return i
		}
	}
	return -1
}

func scanElixirQuotedContent(lexer *gotreesitter.ExternalLexer, info elixirQuotedContentInfo, quotedSym gotreesitter.Symbol) bool {
	lexer.SetResultSymbol(quotedSym)
	isHeredoc := info.delimiterLength == 3

	hasContent := false
	for {
		newline := false
		if isElixirNewline(lexer.Lookahead()) {
			lexer.Advance(false)
			hasContent = true
			newline = true
			for isElixirWhitespace(lexer.Lookahead()) {
				lexer.Advance(false)
			}
		}

		lexer.MarkEnd()

		if lexer.Lookahead() == info.endDelimiter {
			length := 1
			for length < info.delimiterLength {
				lexer.Advance(false)
				if lexer.Lookahead() == info.endDelimiter {
					length++
				} else {
					break
				}
			}
			if length == info.delimiterLength && (!isHeredoc || newline) {
				return hasContent
			}
		} else {
			switch lexer.Lookahead() {
			case '#':
				lexer.Advance(false)
				if info.supportsInterpol && lexer.Lookahead() == '{' {
					return hasContent
				}
			case '\\':
				lexer.Advance(false)
				if isHeredoc && lexer.Lookahead() == '\n' {
					// keep scanning; newline is needed for heredoc delimiter checks
				} else if info.supportsInterpol || lexer.Lookahead() == info.endDelimiter {
					return hasContent
				}
			case 0:
				return hasContent
			default:
				lexer.Advance(false)
			}
		}

		hasContent = true
	}
}

func scanElixirNewline(lexer *gotreesitter.ExternalLexer, validSymbols []bool, syms *[elixirTokenCount]gotreesitter.Symbol) bool {
	lexer.Advance(false)
	for isElixirWhitespace(lexer.Lookahead()) {
		lexer.Advance(false)
	}
	lexer.MarkEnd()

	if lexer.Lookahead() == '#' {
		lexer.SetResultSymbol(syms[elixirTokNewlineBeforeComment])
		return true
	}

	if lexer.Lookahead() == 'd' && isElixirValid(validSymbols, elixirTokNewlineBeforeDo) {
		lexer.SetResultSymbol(syms[elixirTokNewlineBeforeDo])
		lexer.Advance(false)
		if lexer.Lookahead() == 'o' {
			lexer.Advance(false)
			return isElixirTokenEnd(lexer.Lookahead())
		}
		return false
	}

	if isElixirValid(validSymbols, elixirTokNewlineBeforeBinaryOperator) {
		lexer.SetResultSymbol(syms[elixirTokNewlineBeforeBinaryOperator])
		return scanElixirBinaryOperatorAfterNewline(lexer)
	}

	return false
}

func scanElixirBinaryOperatorAfterNewline(lexer *gotreesitter.ExternalLexer) bool {
	switch lexer.Lookahead() {
	case '&':
		lexer.Advance(false)
		if lexer.Lookahead() == '&' {
			lexer.Advance(false)
			if lexer.Lookahead() == '&' {
				lexer.Advance(false)
			}
			return checkElixirOperatorEnd(lexer)
		}
	case '=':
		lexer.Advance(false)
		if lexer.Lookahead() == '=' {
			lexer.Advance(false)
			if lexer.Lookahead() == '=' {
				lexer.Advance(false)
			}
			return checkElixirOperatorEnd(lexer)
		}
		if lexer.Lookahead() == '~' || lexer.Lookahead() == '>' {
			lexer.Advance(false)
		}
		return checkElixirOperatorEnd(lexer)
	case ':':
		lexer.Advance(false)
		if lexer.Lookahead() == ':' {
			lexer.Advance(false)
			if lexer.Lookahead() == ':' {
				return false
			}
			return checkElixirOperatorEnd(lexer)
		}
	case '+':
		lexer.Advance(false)
		if lexer.Lookahead() == '+' {
			lexer.Advance(false)
			if lexer.Lookahead() == '+' {
				lexer.Advance(false)
			}
			return checkElixirOperatorEnd(lexer)
		}
	case '-':
		lexer.Advance(false)
		if lexer.Lookahead() == '-' {
			lexer.Advance(false)
			if lexer.Lookahead() == '-' {
				lexer.Advance(false)
			}
			return checkElixirOperatorEnd(lexer)
		}
		if lexer.Lookahead() == '>' {
			lexer.Advance(false)
			return checkElixirOperatorEnd(lexer)
		}
	case '<':
		lexer.Advance(false)
		if lexer.Lookahead() == '=' || lexer.Lookahead() == '-' || lexer.Lookahead() == '>' {
			lexer.Advance(false)
			return checkElixirOperatorEnd(lexer)
		}
		if lexer.Lookahead() == '~' {
			lexer.Advance(false)
			if lexer.Lookahead() == '>' {
				lexer.Advance(false)
			}
			return checkElixirOperatorEnd(lexer)
		}
		if lexer.Lookahead() == '|' {
			lexer.Advance(false)
			if lexer.Lookahead() == '>' {
				lexer.Advance(false)
				return checkElixirOperatorEnd(lexer)
			}
		}
		if lexer.Lookahead() == '<' {
			lexer.Advance(false)
			if lexer.Lookahead() == '<' || lexer.Lookahead() == '~' {
				lexer.Advance(false)
				return checkElixirOperatorEnd(lexer)
			}
			return false
		}
		return checkElixirOperatorEnd(lexer)
	case '>':
		lexer.Advance(false)
		if lexer.Lookahead() == '=' {
			lexer.Advance(false)
			return checkElixirOperatorEnd(lexer)
		}
		if lexer.Lookahead() == '>' {
			lexer.Advance(false)
			if lexer.Lookahead() == '>' {
				lexer.Advance(false)
				return checkElixirOperatorEnd(lexer)
			}
		}
		return checkElixirOperatorEnd(lexer)
	case '^':
		lexer.Advance(false)
		if lexer.Lookahead() == '^' {
			lexer.Advance(false)
			if lexer.Lookahead() == '^' {
				lexer.Advance(false)
				return checkElixirOperatorEnd(lexer)
			}
		}
	case '!':
		lexer.Advance(false)
		if lexer.Lookahead() == '=' {
			lexer.Advance(false)
			if lexer.Lookahead() == '=' {
				lexer.Advance(false)
			}
			return checkElixirOperatorEnd(lexer)
		}
	case '~':
		lexer.Advance(false)
		if lexer.Lookahead() == '>' {
			lexer.Advance(false)
			if lexer.Lookahead() == '>' {
				lexer.Advance(false)
			}
			return checkElixirOperatorEnd(lexer)
		}
	case '|':
		lexer.Advance(false)
		if lexer.Lookahead() == '|' {
			lexer.Advance(false)
			if lexer.Lookahead() == '|' {
				lexer.Advance(false)
			}
			return checkElixirOperatorEnd(lexer)
		}
		if lexer.Lookahead() == '>' {
			lexer.Advance(false)
			return checkElixirOperatorEnd(lexer)
		}
		return checkElixirOperatorEnd(lexer)
	case '*':
		lexer.Advance(false)
		if lexer.Lookahead() == '*' {
			lexer.Advance(false)
		}
		return checkElixirOperatorEnd(lexer)
	case '/':
		lexer.Advance(false)
		if lexer.Lookahead() == '/' {
			lexer.Advance(false)
		}
		return checkElixirOperatorEnd(lexer)
	case '.':
		lexer.Advance(false)
		if lexer.Lookahead() == '.' {
			lexer.Advance(false)
			if lexer.Lookahead() == '.' {
				return false
			}
			return checkElixirOperatorEnd(lexer)
		}
		return checkElixirOperatorEnd(lexer)
	case '\\':
		lexer.Advance(false)
		if lexer.Lookahead() == '\\' {
			lexer.Advance(false)
			return checkElixirOperatorEnd(lexer)
		}
	case 'w':
		lexer.Advance(false)
		if lexer.Lookahead() == 'h' {
			lexer.Advance(false)
			if lexer.Lookahead() == 'e' {
				lexer.Advance(false)
				if lexer.Lookahead() == 'n' {
					lexer.Advance(false)
					return isElixirTokenEnd(lexer.Lookahead()) && checkElixirOperatorEnd(lexer)
				}
			}
		}
	case 'a':
		lexer.Advance(false)
		if lexer.Lookahead() == 'n' {
			lexer.Advance(false)
			if lexer.Lookahead() == 'd' {
				lexer.Advance(false)
				return isElixirTokenEnd(lexer.Lookahead()) && checkElixirOperatorEnd(lexer)
			}
		}
	case 'o':
		lexer.Advance(false)
		if lexer.Lookahead() == 'r' {
			lexer.Advance(false)
			return isElixirTokenEnd(lexer.Lookahead()) && checkElixirOperatorEnd(lexer)
		}
	case 'i':
		lexer.Advance(false)
		if lexer.Lookahead() == 'n' {
			lexer.Advance(false)
			return isElixirTokenEnd(lexer.Lookahead()) && checkElixirOperatorEnd(lexer)
		}
	case 'n':
		lexer.Advance(false)
		if lexer.Lookahead() == 'o' {
			lexer.Advance(false)
			if lexer.Lookahead() == 't' {
				lexer.Advance(false)
				for isElixirInlineWhitespace(lexer.Lookahead()) {
					lexer.Advance(false)
				}
				if lexer.Lookahead() == 'i' {
					lexer.Advance(false)
					if lexer.Lookahead() == 'n' {
						lexer.Advance(false)
						return isElixirTokenEnd(lexer.Lookahead()) && checkElixirOperatorEnd(lexer)
					}
				}
			}
		}
	}
	return false
}
