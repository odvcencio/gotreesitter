//go:build !grammar_subset || grammar_subset_awk

package grammarruntime

import gotreesitter "github.com/odvcencio/gotreesitter"

// External token indexes for the awk grammar.
const (
	awkTokConcatenatingSpace = 0
	awkTokIfElseSeparator    = 1
	awkTokNoSpace            = 2
	awkTokFuncCall           = 3
)

const (
	awkSymConcatenatingSpace gotreesitter.Symbol = 138
	awkSymIfElseSeparator    gotreesitter.Symbol = 139
	awkSymNoSpace            gotreesitter.Symbol = 140
	awkSymFuncCall           gotreesitter.Symbol = 141
)

// AwkExternalScanner handles spacing-sensitive tokens for AWK.
type AwkExternalScanner struct{}

func (AwkExternalScanner) Create() any                           { return nil }
func (AwkExternalScanner) Destroy(payload any)                   {}
func (AwkExternalScanner) Serialize(payload any, buf []byte) int { return 0 }
func (AwkExternalScanner) Deserialize(payload any, buf []byte)   {}

// The scanner carries no payload and derives every result from local
// lookahead plus validSymbols. Failed scans cannot mutate persistent state.
func (AwkExternalScanner) SupportsIncrementalReuse() bool    { return true }
func (AwkExternalScanner) ExternalScannerIsStateless() bool  { return true }
func (AwkExternalScanner) PreservesStateOnScanFailure() bool { return true }

func (AwkExternalScanner) Scan(payload any, lexer *gotreesitter.ExternalLexer, validSymbols []bool) bool {
	stmtTermFound := false

	// NO_SPACE: zero-width token when next char is not whitespace
	if awkValid(validSymbols, awkTokNoSpace) {
		if !awkIsWhitespace(lexer.Lookahead()) {
			lexer.SetResultSymbol(awkSymNoSpace)
			return true
		}
	}

	// FUNC_CALL: zero-width token when next char is '(' with no whitespace
	if awkValid(validSymbols, awkTokFuncCall) {
		if !awkIsWhitespace(lexer.Lookahead()) && lexer.Lookahead() == '(' {
			lexer.SetResultSymbol(awkSymFuncCall)
			return true
		}
	}

	// IF_ELSE_SEPARATOR: zero-width, checks if "else" follows after whitespace/newlines
	if awkValid(validSymbols, awkTokIfElseSeparator) {
		awkSkipWS(lexer, false)

		if awkIsStatementTerminator(lexer.Lookahead()) || lexer.Lookahead() == '#' {
			stmtTermFound = true
		}

		if awkIsIfElseSep(lexer) {
			lexer.SetResultSymbol(awkSymIfElseSeparator)
			return true
		}
	}

	// CONCATENATING_SPACE: whitespace that acts as string concatenation
	if awkValid(validSymbols, awkTokConcatenatingSpace) && !stmtTermFound {
		if awkIsConcatenatingSpace(lexer) {
			lexer.SetResultSymbol(awkSymConcatenatingSpace)
			return true
		}
	}

	return false
}

func awkIsWhitespace(ch rune) bool {
	return ch == ' ' || ch == '\t'
}

func awkIsStatementTerminator(ch rune) bool {
	return ch == '\n' || ch == ';'
}

func awkIsLineContinuation(lexer *gotreesitter.ExternalLexer) bool {
	if lexer.Lookahead() == '\\' {
		lexer.Advance(true)
		if lexer.Lookahead() == '\r' {
			lexer.Advance(true)
		}
		if lexer.Lookahead() == '\n' {
			return true
		}
	}
	return false
}

func awkSkipWS(lexer *gotreesitter.ExternalLexer, skipNewlines bool) {
	for awkIsWhitespace(lexer.Lookahead()) || awkIsLineContinuation(lexer) || lexer.Lookahead() == '\r' || (skipNewlines && lexer.Lookahead() == '\n') {
		lexer.Advance(true)
	}
}

func awkSkipComment(lexer *gotreesitter.ExternalLexer) {
	if lexer.Lookahead() != '#' {
		return
	}
	for lexer.Lookahead() != '\n' && lexer.Lookahead() != 0 {
		lexer.Advance(true)
	}
	lexer.Advance(false)
	awkSkipWS(lexer, true)
	if lexer.Lookahead() == '#' {
		awkSkipComment(lexer)
	}
}

func awkNextCharsEq(lexer *gotreesitter.ExternalLexer, word string) bool {
	for _, ch := range word {
		if lexer.Lookahead() != ch {
			return false
		}
		lexer.Advance(true)
	}
	return true
}

func awkIsIfElseSep(lexer *gotreesitter.ExternalLexer) bool {
	// Skip whitespace, newlines, semicolons
	for awkIsWhitespace(lexer.Lookahead()) || awkIsStatementTerminator(lexer.Lookahead()) || lexer.Lookahead() == '\r' {
		lexer.Advance(true)
	}
	lexer.MarkEnd()

	if lexer.Lookahead() == '#' {
		awkSkipComment(lexer)
		awkSkipWS(lexer, false)
	}

	return awkNextCharsEq(lexer, "else")
}

func awkIsConcatenatingSpace(lexer *gotreesitter.ExternalLexer) bool {
	hadWS := false
	for awkIsWhitespace(lexer.Lookahead()) || awkIsLineContinuation(lexer) || lexer.Lookahead() == '\r' {
		lexer.Advance(false)
		hadWS = true
	}
	_ = hadWS
	lexer.MarkEnd()

	switch lexer.Lookahead() {
	case '^', '*', '/', '%', '+', '-', '<', '>', '=', '!', '~',
		'&', '|', ',', '?', ':', ')', '[', ']', '{', '}', '#', ';', '\n':
		return false
	case 'i':
		lexer.Advance(true)
		ch := lexer.Lookahead()
		if ch == 'n' || ch == 'f' {
			lexer.Advance(true)
			return lexer.Lookahead() != ' '
		}
		return lexer.Lookahead() != 0
	default:
		return lexer.Lookahead() != 0
	}
}

func awkValid(vs []bool, i int) bool { return i < len(vs) && vs[i] }
