//go:build !grammar_subset || grammar_subset_d

package grammarruntime

import (
	"unicode"

	gotreesitter "github.com/odvcencio/gotreesitter"
)

// External token indexes for the D grammar, in the same order as
// tree-sitter-d's grammar.json "externals" array.
const (
	dTokDirective     = iota // "directive"
	dTokIntLiteral           // "int_literal"
	dTokFloatLiteral         // "float_literal"
	dTokString               // "_string"
	dTokNotIn                // "not_in"
	dTokNotIs                // "not_is"
	dTokAfterEof             // "_after_eof"
	dTokErrorSentinel        // "error_sentinel"
	dTokenCount              // sentinel
)

// dDefaultSymTable holds the concrete symbol IDs for the d grammar blob
// pinned in grammars/languages.lock. It is a fallback default only: a
// scanner bound to a specific *gotreesitter.Language through
// ExternalScannerForLanguage always uses that Language's own ExternalSymbols,
// read positionally through bindExternalScannerSpec. Grammar symbol IDs shift
// whenever the pinned blob regenerates, so a hardcoded absolute ID used
// directly (instead of through this per-instance binding) silently mismatches
// the next time the grammar's rule set changes shape.
var dDefaultSymTable = [dTokenCount]gotreesitter.Symbol{
	221, // directive
	222, // int_literal
	223, // float_literal
	224, // _string
	225, // not_in
	226, // not_is
	227, // _after_eof
	228, // error_sentinel
}

// dExternalScannerSpec records the upstream scanner-source contract this port
// tracks.
var dExternalScannerSpec = ExternalScannerSpec{
	Language:       "d",
	UpstreamRepo:   "https://github.com/gdamore/tree-sitter-d",
	UpstreamCommit: "fb028c8f14f4188286c2eef143f105def6fbf24f",
	SourceFiles: []ExternalScannerSourceFile{
		{Path: "src/grammar.json", SHA256: "7d60a7685d759043b7e7d363e4a2afde7e65b5b9c21c31bd16446a1f3b25b47c"},
		{Path: "src/scanner.c", SHA256: "e4aaa550e8d646326fb89450b82b30ed1134fe55ece6f83a1f6245f6fa539a7c"},
	},
	Externals: []string{
		"directive",
		"int_literal",
		"float_literal",
		"_string",
		"not_in",
		"not_is",
		"_after_eof",
		"error_sentinel",
	},
}

func init() {
	RegisterExternalScannerSpec(dExternalScannerSpec)
}

// DExternalScanner handles external tokens for the D grammar.
// Ported from tree-sitter-d/src/scanner.c.
type DExternalScanner struct {
	symbols         [dTokenCount]gotreesitter.Symbol
	externalToToken []int
}

// ExternalScannerForLanguage binds this scanner's token slots to lang's
// concrete external symbol IDs so Scan reports the IDs the parser table
// actually expects, instead of IDs frozen at some earlier grammar revision.
func (DExternalScanner) ExternalScannerForLanguage(lang *gotreesitter.Language) gotreesitter.ExternalScanner {
	s := DExternalScanner{symbols: dDefaultSymTable}
	s.externalToToken = bindExternalScannerSpec(lang, dExternalScannerSpec, func(tokenIdx int, sym gotreesitter.Symbol) {
		s.symbols[tokenIdx] = sym
	})
	return s
}

func (DExternalScanner) Create() any                           { return nil }
func (DExternalScanner) Destroy(payload any)                   {}
func (DExternalScanner) Serialize(payload any, buf []byte) int { return 0 }
func (DExternalScanner) Deserialize(payload any, buf []byte)   {}

// The scanner carries no payload and derives every result from local
// lookahead plus validSymbols, so every incremental boundary is quiescent and
// failed scans cannot mutate persistent state.
func (DExternalScanner) SupportsIncrementalReuse() bool    { return true }
func (DExternalScanner) ExternalScannerIsStateless() bool  { return true }
func (DExternalScanner) PreservesStateOnScanFailure() bool { return true }

func (s DExternalScanner) symbolTable() *[dTokenCount]gotreesitter.Symbol {
	if s.symbols == ([dTokenCount]gotreesitter.Symbol{}) {
		return &dDefaultSymTable
	}
	return &s.symbols
}

// remapValidSymbols translates the parser's external-index-space validSymbols
// slice into this scanner's token-index space via externalToToken, matching
// the pattern used by the other positionally bound scanners in this package
// (see ocaml_scanner.go, csharp_scanner.go).
func (s DExternalScanner) remapValidSymbols(validSymbols []bool, semanticValid *[dTokenCount]bool) []bool {
	if len(s.externalToToken) == 0 {
		return validSymbols
	}
	*semanticValid = [dTokenCount]bool{}
	for externalIdx, valid := range validSymbols {
		if !valid || externalIdx >= len(s.externalToToken) {
			continue
		}
		tokenIdx := s.externalToToken[externalIdx]
		if tokenIdx >= 0 && tokenIdx < dTokenCount {
			semanticValid[tokenIdx] = true
		}
	}
	return semanticValid[:]
}

func (s DExternalScanner) Scan(payload any, lexer *gotreesitter.ExternalLexer, validSymbols []bool) bool {
	var semanticValid [dTokenCount]bool
	validSymbols = s.remapValidSymbols(validSymbols, &semanticValid)
	symbols := s.symbolTable()

	c := lexer.Lookahead()
	startOfLine := lexer.Column() == 0

	// After-EOF token: consume all remaining input.
	if dValid(validSymbols, dTokAfterEof) && !dValid(validSymbols, dTokErrorSentinel) {
		for lexer.Lookahead() != 0 {
			lexer.Advance(true)
		}
		lexer.MarkEnd()
		lexer.SetResultSymbol(symbols[dTokAfterEof])
		return true
	}

	// Skip whitespace.
	for (unicode.IsSpace(c) || dIsEOL(c)) && c != 0 {
		if dIsEOL(c) {
			startOfLine = true
		}
		lexer.Advance(true)
		c = lexer.Lookahead()
	}

	// Directive: # at start of line.
	if c == '#' && startOfLine {
		return dMatchDirective(lexer, validSymbols, symbols)
	}

	if lexer.Lookahead() == 0 { // EOF after whitespace
		return false
	}

	// Number literals.
	if c == '.' || (c >= '0' && c <= '9') {
		return dMatchNumber(lexer, validSymbols, symbols)
	}

	// !in and !is operators.
	if c == '!' {
		return dMatchNotInIs(lexer, validSymbols, symbols)
	}

	// Delimited string: q"..."
	if c == 'q' && dValid(validSymbols, dTokString) {
		return dMatchQString(lexer, symbols)
	}

	return false
}

func dIsEOL(c rune) bool {
	return c == '\n' || c == '\r' || c == 0x2028 || c == 0x2029
}

func dMatchDirective(lexer *gotreesitter.ExternalLexer, valid []bool, symbols *[dTokenCount]gotreesitter.Symbol) bool {
	if !dValid(valid, dTokDirective) {
		return false
	}
	// Consume '#'
	lexer.Advance(false)
	c := lexer.Lookahead()
	if c == '!' {
		return false
	}
	// Skip spaces (not newlines)
	for (unicode.IsSpace(c) || dIsEOL(c)) && c != 0 {
		if dIsEOL(c) {
			return false
		}
		lexer.Advance(false)
		c = lexer.Lookahead()
	}
	// Consume to end of line
	for !dIsEOL(c) && c != 0 {
		lexer.Advance(false)
		c = lexer.Lookahead()
	}
	// Consume newline
	lexer.Advance(false)
	lexer.MarkEnd()
	lexer.SetResultSymbol(symbols[dTokDirective])
	return true
}

func dMatchNumber(lexer *gotreesitter.ExternalLexer, valid []bool, symbols *[dTokenCount]gotreesitter.Symbol) bool {
	c := lexer.Lookahead()
	isHex := false
	isBin := false
	hasDot := false
	hasDigit := false
	inExp := false

	if c == '.' {
		lexer.Advance(false)
		c = lexer.Lookahead()
		if c < '0' || c > '9' {
			return false
		}
		hasDot = true
	} else if c == '0' {
		lexer.Advance(false)
		c = lexer.Lookahead()
		switch c {
		case 'b', 'B':
			isBin = true
			lexer.Advance(false)
		case 'x', 'X':
			isHex = true
			lexer.Advance(false)
		default:
			hasDigit = true
		}
	}

	if !dValid(valid, dTokIntLiteral) && !dValid(valid, dTokFloatLiteral) {
		return false
	}

	done := false
	for lexer.Lookahead() != 0 && !done {
		c = lexer.Lookahead()
		if c > 0x7f || unicode.IsSpace(c) || c == ';' {
			break
		}
		if isBin && (c == '0' || c == '1') {
			lexer.Advance(false)
			lexer.MarkEnd()
			hasDigit = true
			continue
		}
		if (c >= '0' && c <= '9') || (isHex && !inExp && dIsXDigit(c)) {
			lexer.Advance(false)
			lexer.MarkEnd()
			hasDigit = true
			continue
		}

		switch c {
		case '.':
			if !hasDigit || hasDot || inExp || isBin {
				lexer.MarkEnd()
				done = true
				break
			}
			lexer.MarkEnd()
			lexer.Advance(false)
			c = lexer.Lookahead()
			if (c >= '0' && c <= '9') || (isHex && dIsXDigit(c)) {
				hasDot = true
				continue
			}
			if dIsAlphaNum(c) || c == '_' || c == '.' || (c > 0x7f && !dIsEOL(c)) {
				lexer.SetResultSymbol(symbols[dTokIntLiteral])
				return dValid(valid, dTokIntLiteral)
			}
			lexer.SetResultSymbol(symbols[dTokFloatLiteral])
			lexer.MarkEnd()
			return dValid(valid, dTokFloatLiteral)

		case '_':
			lexer.Advance(false)
			continue

		case 'e', 'E', 'p', 'P':
			if inExp || isBin {
				return false
			}
			if isHex && (c == 'e' || c == 'E') {
				return false
			}
			if !isHex && (c == 'p' || c == 'P') {
				return false
			}
			lexer.Advance(false)
			c = lexer.Lookahead()
			if c == '+' || c == '-' {
				lexer.Advance(false)
			}
			hasDigit = false
			inExp = true
			continue

		default:
			done = true
		}
	}

	if !hasDigit {
		return false
	}
	return dMatchNumberSuffix(lexer, valid, hasDot || inExp, symbols)
}

func dMatchNumberSuffix(lexer *gotreesitter.ExternalLexer, valid []bool, isFloat bool, symbols *[dTokenCount]gotreesitter.Symbol) bool {
	seenL := false
	seenI := false
	seenU := false
	seenF := false
	tok := 0 // 0=unset, dTokIntLiteral or dTokFloatLiteral
	done := false

	for lexer.Lookahead() != 0 && !done {
		c := lexer.Lookahead()
		switch c {
		case 'u', 'U':
			if seenU || seenI || seenF || isFloat {
				return false
			}
			seenU = true
			tok = dTokIntLiteral
		case 'f', 'F':
			if seenU || seenF || seenI {
				return false
			}
			seenF = true
			tok = dTokFloatLiteral
		case 'i':
			if seenI || seenU {
				return false
			}
			tok = dTokFloatLiteral
			seenI = true
		case 'L':
			if seenL || seenF || seenI {
				return false
			}
			seenL = true
		default:
			done = true
		}
		if !done {
			lexer.Advance(false)
		}
	}

	c := lexer.Lookahead()
	if dIsAlphaNum(c) || (c > 0x7f && !dIsEOL(c)) {
		return false
	}
	if isFloat {
		tok = dTokFloatLiteral
	}
	if dValid(valid, dTokIntLiteral) && tok != dTokFloatLiteral {
		lexer.SetResultSymbol(symbols[dTokIntLiteral])
		lexer.MarkEnd()
		return true
	}
	if dValid(valid, dTokFloatLiteral) && tok != dTokIntLiteral {
		lexer.SetResultSymbol(symbols[dTokFloatLiteral])
		lexer.MarkEnd()
		return true
	}
	return false
}

func dMatchNotInIs(lexer *gotreesitter.ExternalLexer, valid []bool, symbols *[dTokenCount]gotreesitter.Symbol) bool {
	if !dValid(valid, dTokNotIn) && !dValid(valid, dTokNotIs) {
		return false
	}
	// Consume '!'
	lexer.Advance(false)
	// Skip whitespace
	for c := lexer.Lookahead(); c != 0; c = lexer.Lookahead() {
		if !unicode.IsSpace(c) && !dIsEOL(c) {
			break
		}
		lexer.Advance(false)
	}

	if lexer.Lookahead() != 'i' {
		return false
	}
	lexer.Advance(false)
	var token int
	switch lexer.Lookahead() {
	case 'n':
		token = dTokNotIn
	case 's':
		token = dTokNotIs
	default:
		return false
	}
	lexer.Advance(false)
	// Must not be followed by alphanumeric
	c := lexer.Lookahead()
	if dIsAlphaNum(c) || c == '_' || (c > 0x7f && !dIsEOL(c)) {
		return false
	}
	if !dValid(valid, token) {
		return false
	}
	lexer.MarkEnd()
	lexer.SetResultSymbol(symbols[token])
	return true
}

func dMatchQString(lexer *gotreesitter.ExternalLexer, symbols *[dTokenCount]gotreesitter.Symbol) bool {
	// Consume 'q'
	lexer.Advance(false)
	if lexer.Lookahead() != '"' {
		return false
	}
	lexer.Advance(false)
	lexer.SetResultSymbol(symbols[dTokString])

	opener := lexer.Lookahead()
	var closer rune
	switch opener {
	case '(':
		closer = ')'
	case '[':
		closer = ']'
	case '{':
		closer = '}'
	case '<':
		closer = '>'
	default:
		// Identifier-delimited string
		var delim []rune
		delim = append(delim, '\n')
		for lexer.Lookahead() != '\n' {
			ch := lexer.Lookahead()
			if !dIsIdentChar(ch) {
				return false
			}
			delim = append(delim, ch)
			lexer.Advance(false)
		}
		delim = append(delim, '"')

		delimPos := 0
		for {
			if lexer.Lookahead() == 0 {
				return false
			}
			if delimPos == len(delim) {
				return true
			}
			if lexer.Lookahead() == delim[delimPos] {
				delimPos++
			} else if lexer.Lookahead() == delim[0] {
				delimPos = 1
			} else {
				delimPos = 0
			}
			lexer.Advance(false)
		}
	}

	// Punctuation-delimited string
	depth := 1
	for depth > 0 {
		lexer.Advance(false)
		if lexer.Lookahead() == opener {
			depth++
		} else if lexer.Lookahead() == closer {
			depth--
		} else if lexer.Lookahead() == 0 {
			return false
		}
	}
	lexer.Advance(false)
	if lexer.Lookahead() != '"' {
		return false
	}
	lexer.Advance(false)
	return true
}

func dIsIdentChar(c rune) bool {
	return (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') || c == '_'
}

func dIsXDigit(c rune) bool {
	return (c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F')
}

func dIsAlphaNum(c rune) bool {
	return (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9')
}

func dValid(vs []bool, i int) bool { return i < len(vs) && vs[i] }
