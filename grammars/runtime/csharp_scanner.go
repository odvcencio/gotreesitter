//go:build !grammar_subset || grammar_subset_c_sharp

package grammarruntime

import (
	"unicode"

	gotreesitter "github.com/odvcencio/gotreesitter"
)

// External token indexes for the C# grammar (order must match grammar.json externals).
//
// tree-sitter/tree-sitter-c-sharp@9150f7d56bb4 appended one external,
// _lambda_paren_open, after raw_string_content. The scanner now looks ahead
// over a parenthesized parameter list and reports the opening "(" as a
// distinct token when the list carries a ref/out/in/readonly modifier and
// ends with "=>". Indexes 0 through 11 keep their previous positions.
const (
	csTokOptSemi             = iota // 0
	csTokInterpRegularStart         // 1
	csTokInterpVerbatimStart        // 2
	csTokInterpRawStart             // 3
	csTokInterpStartQuote           // 4
	csTokInterpEndQuote             // 5
	csTokInterpOpenBrace            // 6
	csTokInterpCloseBrace           // 7
	csTokInterpStringContent        // 8
	csTokRawStringStart             // 9
	csTokRawStringEnd               // 10
	csTokRawStringContent           // 11
	csTokLambdaParenOpen            // 12
	csTokenCount                    // 13 — sentinel
)

// Concrete symbol IDs from the generated C# grammar ExternalSymbols.
const (
	csSymOptSemi             gotreesitter.Symbol = 206
	csSymInterpRegularStart  gotreesitter.Symbol = 207
	csSymInterpVerbatimStart gotreesitter.Symbol = 208
	csSymInterpRawStart      gotreesitter.Symbol = 209
	csSymInterpStartQuote    gotreesitter.Symbol = 210
	csSymInterpEndQuote      gotreesitter.Symbol = 211
	csSymInterpOpenBrace     gotreesitter.Symbol = 212
	csSymInterpCloseBrace    gotreesitter.Symbol = 213
	csSymInterpStringContent gotreesitter.Symbol = 214
	csSymRawStringStart      gotreesitter.Symbol = 215
	csSymRawStringEnd        gotreesitter.Symbol = 216
	csSymRawStringContent    gotreesitter.Symbol = 217
	csSymLambdaParenOpen     gotreesitter.Symbol = 218
)

// csDefaultSymTable maps token indexes to concrete ts2go symbol IDs.
var csDefaultSymTable = [csTokenCount]gotreesitter.Symbol{
	csSymOptSemi,
	csSymInterpRegularStart,
	csSymInterpVerbatimStart,
	csSymInterpRawStart,
	csSymInterpStartQuote,
	csSymInterpEndQuote,
	csSymInterpOpenBrace,
	csSymInterpCloseBrace,
	csSymInterpStringContent,
	csSymRawStringStart,
	csSymRawStringEnd,
	csSymRawStringContent,
	csSymLambdaParenOpen,
}

var cSharpExternalScannerSpec = ExternalScannerSpec{
	Language:       "c_sharp",
	UpstreamRepo:   "https://github.com/tree-sitter/tree-sitter-c-sharp",
	UpstreamCommit: "9150f7d56bb47f1a809fa23623f1ba1413e93fa9",
	SourceFiles: []ExternalScannerSourceFile{
		{Path: "src/grammar.json", SHA256: "f63299656c0072ff9f2b18a54bfba31541e54bcbc20c0b091e0e19189c7ef593"},
		{Path: "src/scanner.c", SHA256: "2ee1241a6a275e72a06838f5df927700bd405c16b48f986e2c33d1264cae4818"},
	},
	Externals: []string{
		"_optional_semi",
		"interpolation_regular_start",
		"interpolation_verbatim_start",
		"interpolation_raw_start",
		"interpolation_start_quote",
		"interpolation_end_quote",
		"interpolation_open_brace",
		"interpolation_close_brace",
		"interpolation_string_content",
		"raw_string_start",
		"raw_string_end",
		"raw_string_content",
		"_lambda_paren_open",
	},
}

func init() {
	RegisterExternalScannerSpec(cSharpExternalScannerSpec)
}

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

// CSharpExternalScanner handles auto-semicolons, interpolated strings, raw
// strings, and simple-lambda parameter lists for C#.
type CSharpExternalScanner struct {
	symbols         [csTokenCount]gotreesitter.Symbol
	externalToToken []int
}

func (CSharpExternalScanner) ExternalScannerForLanguage(lang *gotreesitter.Language) gotreesitter.ExternalScanner {
	s := CSharpExternalScanner{symbols: csDefaultSymTable}
	s.externalToToken = bindExternalScannerSpec(lang, cSharpExternalScannerSpec, func(tokenIdx int, sym gotreesitter.Symbol) {
		s.symbols[tokenIdx] = sym
	})
	return s
}

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

func (s CSharpExternalScanner) Scan(payload any, lexer *gotreesitter.ExternalLexer, validSymbols []bool) bool {
	state := payload.(*csState)
	if len(s.externalToToken) > 0 {
		var semanticValid [csTokenCount]bool
		for externalIdx, valid := range validSymbols {
			if !valid || externalIdx >= len(s.externalToToken) {
				continue
			}
			tokenIdx := s.externalToToken[externalIdx]
			if tokenIdx >= 0 && tokenIdx < csTokenCount {
				semanticValid[tokenIdx] = true
			}
		}
		validSymbols = semanticValid[:]
	}
	return csScan(state, lexer, validSymbols, s.symbolTable())
}

func (s CSharpExternalScanner) symbolTable() *[csTokenCount]gotreesitter.Symbol {
	if s.symbols == ([csTokenCount]gotreesitter.Symbol{}) {
		return &csDefaultSymTable
	}
	return &s.symbols
}

// ---------- helpers for _lambda_paren_open scanning ----------

// csLambdaScanResult reports the outcome of csScanLambdaParenOpen. It tells
// "did not start" (the lexer is untouched, so the caller can fall through to
// the other token handlers) apart from "started and failed" (the lexer cursor
// moved past characters that the other handlers must not consume, because
// they would emit tokens with wrong spans).
type csLambdaScanResult int

const (
	csLambdaScanNoParen csLambdaScanResult = iota
	csLambdaScanFailedAfterParen
	csLambdaScanSuccess
)

func csIsIDStart(c rune) bool {
	return c == '_' || unicode.IsLetter(c)
}

func csIsIDContinue(c rune) bool {
	return c == '_' || unicode.IsLetter(c) || unicode.IsDigit(c)
}

// csSkipWsAndComments advances over whitespace, line comments, and block
// comments. It does not advance over preprocessor directives, because a
// simple-lambda parameter list cannot contain one.
func csSkipWsAndComments(lexer *gotreesitter.ExternalLexer) {
	for {
		c := lexer.Lookahead()
		switch {
		case c == ' ' || c == '\t' || c == '\r' || c == '\n':
			lexer.Advance(false)
		case c == '/':
			lexer.Advance(false)
			switch lexer.Lookahead() {
			case '/':
				for lexer.Lookahead() != 0 && lexer.Lookahead() != '\n' {
					lexer.Advance(false)
				}
			case '*':
				lexer.Advance(false)
				var prev rune
				for lexer.Lookahead() != 0 && !(prev == '*' && lexer.Lookahead() == '/') {
					prev = lexer.Lookahead()
					lexer.Advance(false)
				}
				if lexer.Lookahead() == '/' {
					lexer.Advance(false)
				}
			default:
				// A stray '/' is not valid in a parameter list. Return and let
				// the caller see the unexpected character.
				return
			}
		default:
			return
		}
	}
}

// csConsumeIdentifierInto copies an identifier into a fixed-size buffer. It
// returns the consumed length, which can exceed len(buf). In that case buf
// holds only the first len(buf) bytes, which is enough to reject a name that
// is too long to be a keyword.
func csConsumeIdentifierInto(lexer *gotreesitter.ExternalLexer, buf []byte) int {
	n := 0
	for csIsIDContinue(lexer.Lookahead()) {
		if n < len(buf) {
			buf[n] = byte(lexer.Lookahead())
		}
		n++
		lexer.Advance(false)
	}
	return n
}

// csBufEquals reports whether the first n bytes of buf equal kw.
func csBufEquals(buf []byte, n int, kw string) bool {
	return n == len(kw) && n <= len(buf) && string(buf[:n]) == kw
}

// csScanLambdaParenOpen recognizes a C# 14 simple-lambda parameter list at the
// current position. On success lexer.SetResultSymbol records the
// _lambda_paren_open symbol and MarkEnd ends the token at the opening "(".
// On csLambdaScanNoParen the lexer only skipped leading whitespace, which does
// not take part in token boundaries. On csLambdaScanFailedAfterParen the lexer
// moved past at least the opening "(", so the caller must return false and let
// tree-sitter rewind.
//
// The pattern after the opening "(" is:
//
//	element (',' element)* ')' '=>'
//
//	element  := modifier+ identifier
//	          | identifier
//
//	modifier is one of scoped, ref, out, in, readonly
//
// At least one element must carry a hard modifier. Without that rule the
// scanner fires on a plain "(x, y) => ..." list, which the _lambda_parameters
// choice already covers.
//
// The scanner reads a whole identifier into a small buffer before it decides
// between a modifier and a name. A character-by-character probe cannot work,
// because "ref" is a prefix of "readonly" and the lexer has no rewind
// operation. The buffer removes the prefix conflict.
func csScanLambdaParenOpen(lexer *gotreesitter.ExternalLexer, symbols *[csTokenCount]gotreesitter.Symbol) csLambdaScanResult {
	// An external scanner runs before tree-sitter skips the whitespace extras,
	// so skip the leading whitespace here before the test for "(". Advance(true)
	// consumes the character as an extra, so the caller can still fall through
	// to the other handlers after csLambdaScanNoParen. Those handlers skip
	// whitespace on their own.
	for unicode.IsSpace(lexer.Lookahead()) {
		lexer.Advance(true)
	}
	if lexer.Lookahead() != '(' {
		return csLambdaScanNoParen
	}
	lexer.Advance(false)
	lexer.MarkEnd()

	// From here the lexer cursor sits past "(". Every return must report
	// csLambdaScanFailedAfterParen until the scan reaches success.

	// The list must carry at least one hard modifier (ref, out, in, or
	// readonly) before the scanner commits. "scoped" alone is not a valid C#
	// parameter modifier, because it must always join a hard modifier.
	// "scoped" is also a legal type name, so a "(scoped x) =>" match would
	// collide with the parameter_list path on input that means type plus
	// identifier.
	sawHardModifier := false
	expectingElement := true

	for {
		csSkipWsAndComments(lexer)
		c := lexer.Lookahead()

		if c == 0 {
			// End of input inside the list.
			return csLambdaScanFailedAfterParen
		}

		if c == ')' {
			lexer.Advance(false)
			csSkipWsAndComments(lexer)
			if !sawHardModifier {
				return csLambdaScanFailedAfterParen
			}
			if lexer.Lookahead() != '=' {
				return csLambdaScanFailedAfterParen
			}
			lexer.Advance(false)
			if lexer.Lookahead() != '>' {
				return csLambdaScanFailedAfterParen
			}
			lexer.SetResultSymbol(symbols[csTokLambdaParenOpen])
			return csLambdaScanSuccess
		}

		if !expectingElement {
			if c != ',' {
				return csLambdaScanFailedAfterParen
			}
			lexer.Advance(false)
			expectingElement = true
			continue
		}

		// Read the identifier tokens of this element. Each one is either a
		// parameter modifier, and the scan continues, or the parameter name,
		// which ends the element. The scan reads the whole token before it
		// classifies the token, so it never needs to rewind the lexer on a
		// prefix conflict such as "ref" against "readonly".
		consumedName := false
		for !consumedName {
			csSkipWsAndComments(lexer)
			if !csIsIDStart(lexer.Lookahead()) {
				return csLambdaScanFailedAfterParen
			}

			var buf [9]byte // "readonly" is 8 characters long
			n := csConsumeIdentifierInto(lexer, buf[:])

			isHardModifier := n <= 8 && (csBufEquals(buf[:], n, "ref") ||
				csBufEquals(buf[:], n, "out") ||
				csBufEquals(buf[:], n, "in") ||
				csBufEquals(buf[:], n, "readonly"))
			isSoftModifier := n == 6 && csBufEquals(buf[:], n, "scoped")

			switch {
			case isHardModifier:
				sawHardModifier = true
			case isSoftModifier:
				// "scoped" is a modifier only in front of a hard modifier, so
				// keep scanning. If the element ends with "scoped <identifier>"
				// and no hard modifier appears, the final sawHardModifier test
				// fails and the parse falls back to the parameter_list path.
			default:
				consumedName = true
			}
		}
		expectingElement = false
	}
}

func csScan(s *csState, lexer *gotreesitter.ExternalLexer, validSymbols []bool, symbols *[csTokenCount]gotreesitter.Symbol) bool {
	var braceAdvanced uint8
	var quoteCount uint8
	didAdvance := false

	// The lambda-paren scan moves forward past the opening "(" on speculation.
	// If it consumes input and then fails, the lexer cursor is in the wrong
	// place and the handlers below must not reuse it. They would emit tokens
	// that start at the original scan position and end at the moved cursor, for
	// example an interpolation_regular_start that swallows "(false, $" in
	// "return (false, $\"...\");". When the scan consumed "(" but did not
	// confirm a lambda, return false here so tree-sitter rewinds and the
	// built-in "(" token matches.
	if csValid(validSymbols, csTokLambdaParenOpen) {
		switch csScanLambdaParenOpen(lexer, symbols) {
		case csLambdaScanSuccess:
			return true
		case csLambdaScanFailedAfterParen:
			return false
		case csLambdaScanNoParen:
			// The lexer is untouched. Fall through.
		}
	}

	// Error recovery guard
	if csValid(validSymbols, csTokOptSemi) && csValid(validSymbols, csTokInterpRegularStart) {
		return false
	}

	// Optional semicolon
	if csValid(validSymbols, csTokOptSemi) {
		lexer.SetResultSymbol(symbols[csTokOptSemi])
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
				lexer.SetResultSymbol(symbols[csTokRawStringStart])
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
			lexer.SetResultSymbol(symbols[csTokRawStringEnd])
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
					lexer.SetResultSymbol(symbols[csTokRawStringContent])
					return true
				}
			}
			lexer.Advance(false)
			didAdvance = true
		}
		lexer.MarkEnd()
		lexer.SetResultSymbol(symbols[csTokRawStringContent])
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
			lexer.SetResultSymbol(symbols[csTokInterpRegularStart])
			interp := csInterpolation{
				dollarCount: dollarAdvanced,
			}

			if isVerbatim || lexer.Lookahead() == '@' {
				if lexer.Lookahead() == '@' {
					lexer.Advance(false)
					isVerbatim = true
				}
				lexer.SetResultSymbol(symbols[csTokInterpVerbatimStart])
				interp.stringType = csStrVerbatim
			}

			lexer.MarkEnd()
			lexer.Advance(false) // consume opening "

			if lexer.Lookahead() == '"' && !isVerbatim {
				lexer.Advance(false)
				if lexer.Lookahead() == '"' {
					lexer.SetResultSymbol(symbols[csTokInterpRawStart])
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
		lexer.SetResultSymbol(symbols[csTokInterpStartQuote])
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
			lexer.SetResultSymbol(symbols[csTokInterpEndQuote])
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
			lexer.SetResultSymbol(symbols[csTokInterpOpenBrace])
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
				lexer.SetResultSymbol(symbols[csTokInterpCloseBrace])
				return true
			}
		}
		return false
	}

	// Interpolation string content
	if csValid(validSymbols, csTokInterpStringContent) && len(s.interpolationStack) > 0 {
		lexer.SetResultSymbol(symbols[csTokInterpStringContent])
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
