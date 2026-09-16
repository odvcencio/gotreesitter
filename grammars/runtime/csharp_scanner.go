//go:build !grammar_subset || grammar_subset_c_sharp

package grammarruntime

import (
	"unicode"

	gotreesitter "github.com/odvcencio/gotreesitter"
)

// External token indexes for the C# grammar.
const (
	csTokOptSemi             = 0
	csTokInterpRegularStart  = 1
	csTokInterpVerbatimStart = 2
	csTokInterpRawStart      = 3
	csTokInterpStartQuote    = 4
	csTokInterpEndQuote      = 5
	csTokInterpOpenBrace     = 6
	csTokInterpCloseBrace    = 7
	csTokInterpStringContent = 8
	csTokRawStringStart      = 9
	csTokRawStringEnd        = 10
	csTokRawStringContent    = 11
)

const (
	csSymOptSemi             gotreesitter.Symbol = 205
	csSymInterpRegularStart  gotreesitter.Symbol = 206
	csSymInterpVerbatimStart gotreesitter.Symbol = 207
	csSymInterpRawStart      gotreesitter.Symbol = 208
	csSymInterpStartQuote    gotreesitter.Symbol = 209
	csSymInterpEndQuote      gotreesitter.Symbol = 210
	csSymInterpOpenBrace     gotreesitter.Symbol = 211
	csSymInterpCloseBrace    gotreesitter.Symbol = 212
	csSymInterpStringContent gotreesitter.Symbol = 213
	csSymRawStringStart      gotreesitter.Symbol = 214
	csSymRawStringEnd        gotreesitter.Symbol = 215
	csSymRawStringContent    gotreesitter.Symbol = 216
)

// String type flags for C# interpolated strings.
const (
	csStrRegular  = 1 << 0
	csStrVerbatim = 1 << 1
	csStrRaw      = 1 << 2
)

type csInterpolation struct {
	dollarCount    uint8
	openBraceCount uint8
	quoteCount     uint8
	stringType     uint8
}

type csState struct {
	quoteCount         uint8
	interpolationStack []csInterpolation
}

// CSharpExternalScanner handles auto-semicolons, interpolated strings, and raw strings for C#.
type CSharpExternalScanner struct{}

func (CSharpExternalScanner) Create() any {
	return &csState{}
}

func (CSharpExternalScanner) Destroy(payload any) {}

func (CSharpExternalScanner) Serialize(payload any, buf []byte) int {
	s := payload.(*csState)
	needed := 2 + len(s.interpolationStack)*4
	if needed > len(buf) {
		return 0
	}
	n := 0
	buf[n] = s.quoteCount
	n++
	buf[n] = byte(len(s.interpolationStack))
	n++
	for _, interp := range s.interpolationStack {
		buf[n] = interp.dollarCount
		n++
		buf[n] = interp.openBraceCount
		n++
		buf[n] = interp.quoteCount
		n++
		buf[n] = interp.stringType
		n++
	}
	return n
}

func (CSharpExternalScanner) Deserialize(payload any, buf []byte) {
	s := payload.(*csState)
	s.quoteCount = 0
	s.interpolationStack = s.interpolationStack[:0]
	if len(buf) == 0 {
		return
	}
	n := 0
	s.quoteCount = buf[n]
	n++
	stackLen := int(buf[n])
	n++
	for i := 0; i < stackLen && n+3 < len(buf); i++ {
		interp := csInterpolation{
			dollarCount:    buf[n],
			openBraceCount: buf[n+1],
			quoteCount:     buf[n+2],
			stringType:     buf[n+3],
		}
		n += 4
		s.interpolationStack = append(s.interpolationStack, interp)
	}
}

func (CSharpExternalScanner) Scan(payload any, lexer *gotreesitter.ExternalLexer, validSymbols []bool) bool {
	s := payload.(*csState)

	var braceAdvanced uint8
	var quoteCount uint8
	didAdvance := false

	// Error recovery guard
	if csValid(validSymbols, csTokOptSemi) && csValid(validSymbols, csTokInterpRegularStart) {
		return false
	}

	// Optional semicolon
	if csValid(validSymbols, csTokOptSemi) {
		lexer.SetResultSymbol(csSymOptSemi)
		if lexer.Lookahead() == ';' {
			lexer.Advance(false)
		}
		return true
	}

	// Raw string start: """+ (3 or more quotes)
	if csValid(validSymbols, csTokRawStringStart) {
		for unicode.IsSpace(lexer.Lookahead()) {
			lexer.Advance(true)
		}
		if lexer.Lookahead() == '"' {
			for lexer.Lookahead() == '"' {
				lexer.Advance(false)
				quoteCount++
			}
			if quoteCount >= 3 {
				lexer.SetResultSymbol(csSymRawStringStart)
				s.quoteCount = quoteCount
				return true
			}
		}
	}

	// Raw string end: matching quote count
	if csValid(validSymbols, csTokRawStringEnd) && lexer.Lookahead() == '"' {
		for lexer.Lookahead() == '"' {
			lexer.Advance(false)
			quoteCount++
		}
		if quoteCount == s.quoteCount {
			lexer.SetResultSymbol(csSymRawStringEnd)
			s.quoteCount = 0
			return true
		}
		didAdvance = quoteCount > 0
	}

	// Raw string content
	if csValid(validSymbols, csTokRawStringContent) {
		for lexer.Lookahead() != 0 {
			if lexer.Lookahead() == '"' {
				lexer.MarkEnd()
				quoteCount = 0
				for lexer.Lookahead() == '"' {
					lexer.Advance(false)
					quoteCount++
				}
				if quoteCount == s.quoteCount {
					lexer.SetResultSymbol(csSymRawStringContent)
					return true
				}
			}
			lexer.Advance(false)
			didAdvance = true
		}
		lexer.MarkEnd()
		lexer.SetResultSymbol(csSymRawStringContent)
		return true
	}

	// Interpolation start: $"...", @$"...", $@"...", $$"...", etc.
	if csValid(validSymbols, csTokInterpRegularStart) || csValid(validSymbols, csTokInterpVerbatimStart) ||
		csValid(validSymbols, csTokInterpRawStart) {
		for unicode.IsSpace(lexer.Lookahead()) {
			lexer.Advance(true)
		}

		var dollarAdvanced uint8
		isVerbatim := false

		if lexer.Lookahead() == '@' {
			isVerbatim = true
			lexer.Advance(false)
		}

		for lexer.Lookahead() == '$' && quoteCount == 0 {
			lexer.Advance(false)
			dollarAdvanced++
		}

		if dollarAdvanced > 0 && (lexer.Lookahead() == '"' || lexer.Lookahead() == '@') {
			lexer.SetResultSymbol(csSymInterpRegularStart)
			interp := csInterpolation{
				dollarCount: dollarAdvanced,
			}

			if isVerbatim || lexer.Lookahead() == '@' {
				if lexer.Lookahead() == '@' {
					lexer.Advance(false)
					isVerbatim = true
				}
				lexer.SetResultSymbol(csSymInterpVerbatimStart)
				interp.stringType = csStrVerbatim
			}

			lexer.MarkEnd()
			lexer.Advance(false) // consume opening "

			if lexer.Lookahead() == '"' && !isVerbatim {
				lexer.Advance(false)
				if lexer.Lookahead() == '"' {
					lexer.SetResultSymbol(csSymInterpRawStart)
					interp.stringType |= csStrRaw
					s.interpolationStack = append(s.interpolationStack, interp)
				}
				// 1 or 3 quotes: push. 2 quotes: empty string, don't push.
			} else {
				interp.stringType |= csStrRegular
				s.interpolationStack = append(s.interpolationStack, interp)
			}

			return true
		}
	}

	// Interpolation start quote
	if csValid(validSymbols, csTokInterpStartQuote) && len(s.interpolationStack) > 0 {
		cur := &s.interpolationStack[len(s.interpolationStack)-1]
		if cur.stringType&csStrVerbatim != 0 || cur.stringType&csStrRegular != 0 {
			if lexer.Lookahead() == '"' {
				lexer.Advance(false)
				cur.quoteCount++
			}
		} else {
			for lexer.Lookahead() == '"' {
				lexer.Advance(false)
				cur.quoteCount++
			}
		}
		lexer.SetResultSymbol(csSymInterpStartQuote)
		return cur.quoteCount > 0
	}

	// Interpolation end quote
	if csValid(validSymbols, csTokInterpEndQuote) && len(s.interpolationStack) > 0 {
		cur := &s.interpolationStack[len(s.interpolationStack)-1]
		for lexer.Lookahead() == '"' {
			lexer.Advance(false)
			quoteCount++
		}
		if quoteCount == cur.quoteCount {
			lexer.SetResultSymbol(csSymInterpEndQuote)
			s.interpolationStack = s.interpolationStack[:len(s.interpolationStack)-1]
			return true
		}
		didAdvance = quoteCount > 0
	}

	// Interpolation open brace
	if csValid(validSymbols, csTokInterpOpenBrace) && len(s.interpolationStack) > 0 {
		cur := &s.interpolationStack[len(s.interpolationStack)-1]
		for lexer.Lookahead() == '{' && braceAdvanced < cur.dollarCount {
			lexer.Advance(false)
			braceAdvanced++
		}
		if braceAdvanced > 0 && braceAdvanced == cur.dollarCount &&
			(braceAdvanced == 0 || lexer.Lookahead() != '{') {
			cur.openBraceCount = braceAdvanced
			lexer.SetResultSymbol(csSymInterpOpenBrace)
			return true
		}
	}

	// Interpolation close brace
	if csValid(validSymbols, csTokInterpCloseBrace) && len(s.interpolationStack) > 0 {
		cur := &s.interpolationStack[len(s.interpolationStack)-1]
		var closeBraceAdvanced uint8
		for unicode.IsSpace(lexer.Lookahead()) {
			lexer.Advance(false)
		}
		for lexer.Lookahead() == '}' {
			lexer.Advance(false)
			closeBraceAdvanced++
			if closeBraceAdvanced == cur.openBraceCount {
				cur.openBraceCount = 0
				lexer.SetResultSymbol(csSymInterpCloseBrace)
				return true
			}
		}
		return false
	}

	// Interpolation string content
	if csValid(validSymbols, csTokInterpStringContent) && len(s.interpolationStack) > 0 {
		lexer.SetResultSymbol(csSymInterpStringContent)
		cur := &s.interpolationStack[len(s.interpolationStack)-1]
		braceAdvanced = 0

		for lexer.Lookahead() != 0 {
			if cur.stringType&csStrRaw != 0 {
				// Raw string content
				if lexer.Lookahead() == '"' {
					lexer.MarkEnd()
					lexer.Advance(false)
					if lexer.Lookahead() == '"' {
						lexer.Advance(false)
						var qa uint8 = 2
						for lexer.Lookahead() == '"' {
							qa++
							lexer.Advance(false)
						}
						if qa == cur.quoteCount {
							return didAdvance
						}
					}
				}
				if lexer.Lookahead() == '{' {
					lexer.MarkEnd()
					braceAdvanced = 0
					for lexer.Lookahead() == '{' && braceAdvanced < cur.openBraceCount {
						lexer.Advance(false)
						braceAdvanced++
					}
					if braceAdvanced == cur.openBraceCount &&
						(braceAdvanced == 0 || lexer.Lookahead() != '{') {
						return didAdvance
					}
				}
			} else if cur.stringType&csStrVerbatim != 0 {
				// Verbatim string content
				if lexer.Lookahead() == '"' {
					lexer.MarkEnd()
					lexer.Advance(false)
					if lexer.Lookahead() == '"' {
						lexer.Advance(false)
						continue
					}
					return didAdvance
				}
				if lexer.Lookahead() == '{' {
					lexer.MarkEnd()
					braceAdvanced = 0
					for lexer.Lookahead() == '{' && braceAdvanced < cur.openBraceCount {
						lexer.Advance(false)
						braceAdvanced++
					}
					if braceAdvanced == cur.openBraceCount &&
						(braceAdvanced == 0 || lexer.Lookahead() != '{') {
						return didAdvance
					}
				}
			} else if cur.stringType&csStrRegular != 0 {
				// Regular string content
				if lexer.Lookahead() == '\\' || lexer.Lookahead() == '\n' || lexer.Lookahead() == '"' {
					lexer.MarkEnd()
					return didAdvance
				}
				if lexer.Lookahead() == '{' {
					lexer.MarkEnd()
					braceAdvanced = 0
					for lexer.Lookahead() == '{' && braceAdvanced < cur.openBraceCount {
						lexer.Advance(false)
						braceAdvanced++
					}
					if braceAdvanced == cur.openBraceCount &&
						(braceAdvanced == 0 || lexer.Lookahead() != '{') {
						return didAdvance
					}
				}
			}

			if lexer.Lookahead() != '{' {
				braceAdvanced = 0
			}
			lexer.Advance(false)
			didAdvance = true
		}

		lexer.MarkEnd()
		return didAdvance
	}

	return false
}

func csValid(vs []bool, i int) bool { return i < len(vs) && vs[i] }
