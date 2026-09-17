//go:build !grammar_subset || grammar_subset_elixir

package grammarruntime

import gotreesitter "github.com/odvcencio/gotreesitter"

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
)

const (
	elixirSymNewlineBeforeDo             gotreesitter.Symbol = 118
	elixirSymNewlineBeforeBinaryOperator gotreesitter.Symbol = 119
	elixirSymNewlineBeforeComment        gotreesitter.Symbol = 120
	elixirSymBeforeUnaryOperator         gotreesitter.Symbol = 121
	elixirSymNotIn                       gotreesitter.Symbol = 122
	elixirSymQuotedAtomStart             gotreesitter.Symbol = 123
)

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

type ElixirExternalScanner struct{}

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

func (ElixirExternalScanner) Scan(payload any, lexer *gotreesitter.ExternalLexer, validSymbols []bool) bool {
	return scanElixir(lexer, validSymbols)
}

func scanElixir(lexer *gotreesitter.ExternalLexer, validSymbols []bool) bool {
	quotedInfoIdx := findElixirQuotedTokenInfo(validSymbols)
	if quotedInfoIdx != -1 {
		info := elixirQuotedContentInfos[quotedInfoIdx]
		return scanElixirQuotedContent(lexer, info)
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
		return scanElixirNewline(lexer, validSymbols)
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
			lexer.SetResultSymbol(elixirSymBeforeUnaryOperator)
			return true
		}
	case '-':
		if skippedWhitespace && isElixirValid(validSymbols, elixirTokBeforeUnaryOperator) {
			lexer.MarkEnd()
			lexer.SetResultSymbol(elixirSymBeforeUnaryOperator)
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
								lexer.SetResultSymbol(elixirSymNotIn)
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
			lexer.SetResultSymbol(elixirSymQuotedAtomStart)
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

func elixirQuotedTokenSymbol(tokenType int) gotreesitter.Symbol {
	// quoted_content external symbols are contiguous [98..117]
	return gotreesitter.Symbol(98 + tokenType)
}

func scanElixirQuotedContent(lexer *gotreesitter.ExternalLexer, info elixirQuotedContentInfo) bool {
	lexer.SetResultSymbol(elixirQuotedTokenSymbol(info.tokenType))
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

func scanElixirNewline(lexer *gotreesitter.ExternalLexer, validSymbols []bool) bool {
	lexer.Advance(false)
	for isElixirWhitespace(lexer.Lookahead()) {
		lexer.Advance(false)
	}
	lexer.MarkEnd()

	if lexer.Lookahead() == '#' {
		lexer.SetResultSymbol(elixirSymNewlineBeforeComment)
		return true
	}

	if lexer.Lookahead() == 'd' && isElixirValid(validSymbols, elixirTokNewlineBeforeDo) {
		lexer.SetResultSymbol(elixirSymNewlineBeforeDo)
		lexer.Advance(false)
		if lexer.Lookahead() == 'o' {
			lexer.Advance(false)
			return isElixirTokenEnd(lexer.Lookahead())
		}
		return false
	}

	if isElixirValid(validSymbols, elixirTokNewlineBeforeBinaryOperator) {
		lexer.SetResultSymbol(elixirSymNewlineBeforeBinaryOperator)
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
