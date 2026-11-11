package pythonruntime

// Code generated from the pre-change scanner source as a test oracle; DO NOT EDIT.

import (
	gotreesitter "github.com/odvcencio/gotreesitter"
)

// This state and encoding serve Python-derived external scanners. Keep this
// file separate from Python registration so a subset does not publish Python
// metadata when it only includes a derivative grammar.
type oracleDelimiter byte

const (
	oracleDelimiterSingleQuote oracleDelimiter = 1 << 0
	oracleDelimiterDoubleQuote oracleDelimiter = 1 << 1
	oracleDelimiterBackQuote   oracleDelimiter = 1 << 2
	oracleDelimiterRaw         oracleDelimiter = 1 << 3
	oracleDelimiterFormat      oracleDelimiter = 1 << 4
	oracleDelimiterTriple      oracleDelimiter = 1 << 5
	oracleDelimiterBytes       oracleDelimiter = 1 << 6
)

func (d oracleDelimiter) IsFormat() bool { return d&oracleDelimiterFormat != 0 }
func (d oracleDelimiter) IsRaw() bool    { return d&oracleDelimiterRaw != 0 }
func (d oracleDelimiter) IsTriple() bool { return d&oracleDelimiterTriple != 0 }
func (d oracleDelimiter) IsBytes() bool  { return d&oracleDelimiterBytes != 0 }

func (d oracleDelimiter) EndChar() rune {
	switch {
	case d&oracleDelimiterSingleQuote != 0:
		return '\''
	case d&oracleDelimiterDoubleQuote != 0:
		return '"'
	case d&oracleDelimiterBackQuote != 0:
		return '`'
	default:
		return 0
	}
}

type oracleState struct {
	Indents                  []uint16
	Delimiters               []oracleDelimiter
	InsideInterpolatedString bool
}

func (s *oracleState) SyncInsideInterpolatedString() {
	s.InsideInterpolatedString = false
	for _, d := range s.Delimiters {
		if d.IsFormat() {
			s.InsideInterpolatedString = true
			return
		}
	}
}

// Python scanner checkpoints use a compact, self-delimiting wire format:
// one flag byte, one delimiter-count byte, the delimiter stack, one
// little-endian indent-count word, and the indent stack as little-endian
// words. Keep the delimiter count byte as part of the current prefix shape,
// but treat the payload as ephemeral and version-local, not backward-compatible.
const (
	oraclePythonScannerCheckpointHeaderBytes = 1 + 1 + 2
	oracleMaxPythonScannerDelimiterCount     = int(^uint8(0))
	oracleMaxPythonScannerIndentCount        = int(^uint16(0))
)

func oraclePythonScannerCheckpointSize(delimiterCount, indentCount int) (int, bool) {
	if delimiterCount < 0 || delimiterCount > oracleMaxPythonScannerDelimiterCount ||
		indentCount < 0 || indentCount > oracleMaxPythonScannerIndentCount {
		return 0, false
	}
	size := oraclePythonScannerCheckpointHeaderBytes + delimiterCount
	maxInt := int(^uint(0) >> 1)
	if indentCount > (maxInt-size)/2 {
		return 0, false
	}
	return size + indentCount*2, true
}

func oracleSerializeState(s *oracleState, buf []byte) int {
	if s == nil {
		return 0
	}
	s.SyncInsideInterpolatedString()

	indentCount := 0
	if len(s.Indents) > 0 {
		// Indents[0] is the scanner's root sentinel and is not serialized.
		indentCount = len(s.Indents) - 1
	}
	size, ok := oraclePythonScannerCheckpointSize(len(s.Delimiters), indentCount)
	if !ok || len(buf) < size {
		// Never publish a prefix. A zero return tells the parser that this
		// boundary has no usable checkpoint and forces the safe fallback.
		return 0
	}

	write := 0
	// Always write the flag. Scanner checkpoint buffers are reused between
	// tokens, so leaving a false flag untouched would leak a prior f-string
	// state into the next checkpoint.
	buf[write] = 0
	if s.InsideInterpolatedString {
		buf[write] = 1
	}
	write++
	buf[write] = byte(len(s.Delimiters))
	write++
	for _, delimiter := range s.Delimiters {
		buf[write] = byte(delimiter)
		write++
	}
	buf[write] = byte(indentCount)
	buf[write+1] = byte(indentCount >> 8)
	write += 2

	// Skip Indents[0] (sentinel), serialize from index 1.
	for i := 1; i < len(s.Indents); i++ {
		v := s.Indents[i]
		buf[write] = byte(v)
		buf[write+1] = byte(v >> 8)
		write += 2
	}

	return write
}

func oracleDeserializeState(s *oracleState, buf []byte) {
	if s == nil {
		return
	}
	s.Delimiters = s.Delimiters[:0]
	s.Indents = s.Indents[:0]
	s.Indents = append(s.Indents, 0)
	s.InsideInterpolatedString = false

	if len(buf) == 0 {
		return
	}
	if len(buf) < oraclePythonScannerCheckpointHeaderBytes {
		return
	}

	encodedInside := buf[0] != 0
	delimCount := int(buf[1])
	indentCountOffset := 2 + delimCount
	if indentCountOffset+2 > len(buf) {
		return
	}
	indentCount := int(buf[indentCountOffset]) | int(buf[indentCountOffset+1])<<8
	required, ok := oraclePythonScannerCheckpointSize(delimCount, indentCount)
	if !ok || len(buf) != required {
		// Reject both truncated and trailing data. Leave the reset sentinel in
		// place so a malformed checkpoint cannot partially restore scanner state.
		return
	}

	inside := false
	for i := 0; i < delimCount; i++ {
		delimiter := oracleDelimiter(buf[2+i])
		s.Delimiters = append(s.Delimiters, delimiter)
		inside = inside || delimiter.IsFormat()
	}
	if encodedInside != inside {
		// The flag is redundant, but an inconsistent checkpoint is not a
		// complete state representation. Fail closed instead of guessing.
		s.Delimiters = s.Delimiters[:0]
		return
	}
	size := indentCountOffset + 2
	for i := 0; i < indentCount; i++ {
		v := uint16(buf[size]) | uint16(buf[size+1])<<8
		s.Indents = append(s.Indents, v)
		size += 2
	}
	s.InsideInterpolatedString = inside
}

// External token indexes must match the generated Python grammar ExternalSymbols order.
const (
	oraclePyTokNewline = iota
	oraclePyTokIndent
	oraclePyTokDedent
	oraclePyTokStringStart
	oraclePyTokStringContent
	oraclePyTokEscapeInterpolation
	oraclePyTokStringEnd
	oraclePyTokComment
	oraclePyTokCloseBracket
	oraclePyTokCloseParen
	oraclePyTokCloseBrace
	oraclePyTokExcept
	oraclePyTokenCount
)

// Concrete symbol IDs from the checked-in Python grammar ExternalSymbols.
const (
	oraclePySymNewline             gotreesitter.Symbol = 101
	oraclePySymIndent              gotreesitter.Symbol = 102
	oraclePySymDedent              gotreesitter.Symbol = 103
	oraclePySymStringStart         gotreesitter.Symbol = 104
	oraclePySymStringContent       gotreesitter.Symbol = 105
	oraclePySymEscapeInterpolation gotreesitter.Symbol = 106
	oraclePySymStringEnd           gotreesitter.Symbol = 107
	oraclePySymComment             gotreesitter.Symbol = 99
	oraclePySymCloseBracket        gotreesitter.Symbol = 47
	oraclePySymCloseParen          gotreesitter.Symbol = 8
	oraclePySymCloseBrace          gotreesitter.Symbol = 53
	oraclePySymExcept              gotreesitter.Symbol = 33
)

var oraclePyDefaultSymTable = [oraclePyTokenCount]gotreesitter.Symbol{
	oraclePySymNewline,
	oraclePySymIndent,
	oraclePySymDedent,
	oraclePySymStringStart,
	oraclePySymStringContent,
	oraclePySymEscapeInterpolation,
	oraclePySymStringEnd,
	oraclePySymComment,
	oraclePySymCloseBracket,
	oraclePySymCloseParen,
	oraclePySymCloseBrace,
	oraclePySymExcept,
}

type oraclePythonExternalScanner struct {
	symbols         [oraclePyTokenCount]gotreesitter.Symbol
	externalToToken []int
}

func (oraclePythonExternalScanner) ExternalScannerForLanguage(lang *gotreesitter.Language) gotreesitter.ExternalScanner {
	return oracleBind(lang, oracleBindExternalSymbols)
}

// oracleBind uses the caller's positional binder and preserves its diagnostics.
func oracleBind(lang *gotreesitter.Language, bind func(*gotreesitter.Language, func(int, gotreesitter.Symbol)) []int) oraclePythonExternalScanner {
	s := oraclePythonExternalScanner{symbols: oraclePyDefaultSymTable}
	s.externalToToken = bind(lang, func(tokenIdx int, sym gotreesitter.Symbol) {
		s.symbols[tokenIdx] = sym
	})
	return s
}

func oracleBindExternalSymbols(lang *gotreesitter.Language, setSymbol func(int, gotreesitter.Symbol)) []int {
	if lang == nil {
		return nil
	}
	externalToToken := make([]int, len(lang.ExternalSymbols))
	for i := range externalToToken {
		externalToToken[i] = -1
	}
	bindCount := len(externalToToken)
	if oraclePyTokenCount < bindCount {
		bindCount = oraclePyTokenCount
	}
	for i := 0; i < bindCount; i++ {
		setSymbol(i, lang.ExternalSymbols[i])
		externalToToken[i] = i
	}
	return externalToToken
}

func (oraclePythonExternalScanner) Create() any {
	return &oracleState{
		Indents: []uint16{0},
	}
}

func (oraclePythonExternalScanner) Destroy(payload any) {}

func (oraclePythonExternalScanner) Serialize(payload any, buf []byte) int {
	return oracleSerializeState(payload.(*oracleState), buf)
}

func (oraclePythonExternalScanner) Deserialize(payload any, buf []byte) {
	oracleDeserializeState(payload.(*oracleState), buf)
}

// SupportsIncrementalReuse enables checkpoint-authenticated subtree reuse.
// Python serializes its complete indentation and string-delimiter state, so a
// reused boundary is accepted only after the live scanner matches its start
// checkpoint and restores its end checkpoint.
func (oraclePythonExternalScanner) SupportsIncrementalReuse() bool { return true }

func (oraclePythonExternalScanner) UsesExternalScannerCheckpoints() bool { return true }

// Checkpoints retain indentation and string state; all source dependencies
// pass through the observed lexer operations, including failed scans.
func (oraclePythonExternalScanner) SupportsLargeCheckpointReadDependencies() bool { return true }

// ASCII digits share every character branch, including string and comment scans.
func (oraclePythonExternalScanner) ExternalScannerASCIIEquivalenceClass(b byte) uint8 {
	if b >= '0' && b <= '9' {
		return 1
	}
	return 0
}

// RequiresIncrementalPrefixFrontierProof prevents a changed indentation
// prefix from transferring a reduction that belongs to the old source.
func (oraclePythonExternalScanner) RequiresIncrementalPrefixFrontierProof() bool { return true }

func (oraclePythonExternalScanner) PreservesStateOnScanFailure() bool { return true }

func (p oraclePythonExternalScanner) symbolTable() *[oraclePyTokenCount]gotreesitter.Symbol {
	if p.symbols == ([oraclePyTokenCount]gotreesitter.Symbol{}) {
		return &oraclePyDefaultSymTable
	}
	return &p.symbols
}

func (p oraclePythonExternalScanner) remapValidSymbols(validSymbols []bool, semanticValid *[oraclePyTokenCount]bool) []bool {
	if len(p.externalToToken) == 0 {
		return validSymbols
	}
	*semanticValid = [oraclePyTokenCount]bool{}
	for externalIdx, valid := range validSymbols {
		if !valid || externalIdx >= len(p.externalToToken) {
			continue
		}
		tokenIdx := p.externalToToken[externalIdx]
		if tokenIdx >= 0 && tokenIdx < oraclePyTokenCount {
			semanticValid[tokenIdx] = true
		}
	}
	return semanticValid[:]
}

func (p oraclePythonExternalScanner) Scan(payload any, lexer *gotreesitter.ExternalLexer, validSymbols []bool) bool {
	s := payload.(*oracleState)
	if len(s.Indents) == 0 {
		s.Indents = append(s.Indents, 0)
	}
	s.SyncInsideInterpolatedString()
	symbols := p.symbolTable()
	if len(p.externalToToken) > 0 {
		var semanticValid [oraclePyTokenCount]bool
		validSymbols = p.remapValidSymbols(validSymbols, &semanticValid)
	}

	isValid := func(idx int) bool {
		return idx >= 0 && idx < len(validSymbols) && validSymbols[idx]
	}

	errorRecoveryMode := isValid(oraclePyTokStringContent) && isValid(oraclePyTokIndent)
	withinBrackets := isValid(oraclePyTokCloseBrace) || isValid(oraclePyTokCloseParen) || isValid(oraclePyTokCloseBracket)

	advancedOnce := false
	if isValid(oraclePyTokEscapeInterpolation) && len(s.Delimiters) > 0 &&
		(lexer.Lookahead() == '{' || lexer.Lookahead() == '}') && !errorRecoveryMode {
		delimiter := s.Delimiters[len(s.Delimiters)-1]
		if delimiter.IsFormat() {
			lexer.MarkEnd()
			isLeftBrace := lexer.Lookahead() == '{'
			lexer.Advance(false)
			advancedOnce = true
			if (lexer.Lookahead() == '{' && isLeftBrace) || (lexer.Lookahead() == '}' && !isLeftBrace) {
				lexer.Advance(false)
				lexer.MarkEnd()
				lexer.SetResultSymbol(symbols[oraclePyTokEscapeInterpolation])
				return true
			}
			return false
		}
	}

	if isValid(oraclePyTokStringContent) && len(s.Delimiters) > 0 && !errorRecoveryMode {
		delimiter := s.Delimiters[len(s.Delimiters)-1]
		EndChar := delimiter.EndChar()
		hasContent := advancedOnce

		for lexer.Lookahead() != 0 {
			if (advancedOnce || lexer.Lookahead() == '{' || lexer.Lookahead() == '}') && delimiter.IsFormat() {
				lexer.MarkEnd()
				lexer.SetResultSymbol(symbols[oraclePyTokStringContent])
				return hasContent
			}

			if lexer.Lookahead() == '\\' {
				if delimiter.IsRaw() {
					lexer.Advance(false)
					if lexer.Lookahead() == EndChar || lexer.Lookahead() == '\\' {
						lexer.Advance(false)
					}
					if lexer.Lookahead() == '\r' {
						lexer.Advance(false)
						if lexer.Lookahead() == '\n' {
							lexer.Advance(false)
						}
					} else if lexer.Lookahead() == '\n' {
						lexer.Advance(false)
					}
					continue
				}

				if delimiter.IsBytes() {
					lexer.MarkEnd()
					lexer.Advance(false)
					if lexer.Lookahead() == 'N' || lexer.Lookahead() == 'u' || lexer.Lookahead() == 'U' {
						lexer.Advance(false)
					} else {
						lexer.SetResultSymbol(symbols[oraclePyTokStringContent])
						return hasContent
					}
				} else {
					lexer.MarkEnd()
					lexer.SetResultSymbol(symbols[oraclePyTokStringContent])
					return hasContent
				}
			} else if lexer.Lookahead() == EndChar {
				if delimiter.IsTriple() {
					lexer.MarkEnd()
					lexer.Advance(false)
					if lexer.Lookahead() == EndChar {
						lexer.Advance(false)
						if lexer.Lookahead() == EndChar {
							if hasContent {
								lexer.SetResultSymbol(symbols[oraclePyTokStringContent])
							} else {
								lexer.Advance(false)
								lexer.MarkEnd()
								s.Delimiters = s.Delimiters[:len(s.Delimiters)-1]
								lexer.SetResultSymbol(symbols[oraclePyTokStringEnd])
								s.InsideInterpolatedString = false
							}
							return true
						}
						lexer.MarkEnd()
						lexer.SetResultSymbol(symbols[oraclePyTokStringContent])
						return true
					}
					lexer.MarkEnd()
					lexer.SetResultSymbol(symbols[oraclePyTokStringContent])
					return true
				}

				if hasContent {
					lexer.SetResultSymbol(symbols[oraclePyTokStringContent])
				} else {
					lexer.Advance(false)
					s.Delimiters = s.Delimiters[:len(s.Delimiters)-1]
					lexer.SetResultSymbol(symbols[oraclePyTokStringEnd])
					s.InsideInterpolatedString = false
				}
				lexer.MarkEnd()
				return true
			} else if lexer.Lookahead() == '\n' && hasContent && !delimiter.IsTriple() {
				return false
			}

			lexer.Advance(false)
			hasContent = true
		}
	}

	lexer.MarkEnd()

	foundEndOfLine := false
	var indentLength uint16
	firstCommentIndentLength := int32(-1)

	for {
		switch lexer.Lookahead() {
		case '\n':
			foundEndOfLine = true
			indentLength = 0
			lexer.Advance(true)
		case ' ':
			indentLength += uint16(lexer.AdvanceSpaces(true))
		case '\r', '\f':
			indentLength = 0
			lexer.Advance(true)
		case '\t':
			indentLength += 8
			lexer.Advance(true)
		case '#':
			if isValid(oraclePyTokIndent) || isValid(oraclePyTokDedent) || isValid(oraclePyTokNewline) || isValid(oraclePyTokExcept) {
				if !foundEndOfLine {
					return false
				}
				if firstCommentIndentLength == -1 {
					firstCommentIndentLength = int32(indentLength)
				}
				lexer.AdvanceUntilNewline(true)
				lexer.Advance(true)
				indentLength = 0
				continue
			}
			goto afterIndentLoop
		case '\\':
			lexer.Advance(true)
			if lexer.Lookahead() == '\r' {
				lexer.Advance(true)
			}
			if lexer.Lookahead() == '\n' || lexer.Lookahead() == 0 {
				lexer.Advance(true)
			} else {
				return false
			}
		case 0:
			indentLength = 0
			foundEndOfLine = true
			goto afterIndentLoop
		default:
			goto afterIndentLoop
		}
	}

afterIndentLoop:
	if foundEndOfLine {
		currentIndent := s.Indents[len(s.Indents)-1]

		if isValid(oraclePyTokIndent) && indentLength > currentIndent {
			s.Indents = append(s.Indents, indentLength)
			lexer.SetResultSymbol(symbols[oraclePyTokIndent])
			return true
		}

		nextTokIsStringStart := lexer.Lookahead() == '"' || lexer.Lookahead() == '\'' || lexer.Lookahead() == '`'
		if (isValid(oraclePyTokDedent) ||
			(!isValid(oraclePyTokNewline) && !(isValid(oraclePyTokStringStart) && nextTokIsStringStart) && !withinBrackets)) &&
			indentLength < currentIndent &&
			!s.InsideInterpolatedString &&
			firstCommentIndentLength < int32(currentIndent) {
			s.Indents = s.Indents[:len(s.Indents)-1]
			lexer.SetResultSymbol(symbols[oraclePyTokDedent])
			return true
		}

		if isValid(oraclePyTokNewline) && !errorRecoveryMode {
			lexer.SetResultSymbol(symbols[oraclePyTokNewline])
			return true
		}
	}

	if firstCommentIndentLength == -1 && isValid(oraclePyTokStringStart) {
		var delimiter oracleDelimiter
		hasFlags := false

		for lexer.Lookahead() != 0 {
			switch lexer.Lookahead() {
			case 'f', 'F', 't', 'T':
				delimiter |= oracleDelimiterFormat
			case 'r', 'R':
				delimiter |= oracleDelimiterRaw
			case 'b', 'B':
				delimiter |= oracleDelimiterBytes
			case 'u', 'U':
				// accepted prefix, no scanner flag
			default:
				goto afterFlags
			}
			hasFlags = true
			lexer.Advance(false)
		}

	afterFlags:
		switch lexer.Lookahead() {
		case '`':
			delimiter |= oracleDelimiterBackQuote
			lexer.Advance(false)
			lexer.MarkEnd()
		case '\'':
			delimiter |= oracleDelimiterSingleQuote
			lexer.Advance(false)
			lexer.MarkEnd()
			if lexer.Lookahead() == '\'' {
				lexer.Advance(false)
				if lexer.Lookahead() == '\'' {
					lexer.Advance(false)
					lexer.MarkEnd()
					delimiter |= oracleDelimiterTriple
				}
			}
		case '"':
			delimiter |= oracleDelimiterDoubleQuote
			lexer.Advance(false)
			lexer.MarkEnd()
			if lexer.Lookahead() == '"' {
				lexer.Advance(false)
				if lexer.Lookahead() == '"' {
					lexer.Advance(false)
					lexer.MarkEnd()
					delimiter |= oracleDelimiterTriple
				}
			}
		}

		if delimiter.EndChar() != 0 {
			s.Delimiters = append(s.Delimiters, delimiter)
			lexer.SetResultSymbol(symbols[oraclePyTokStringStart])
			s.InsideInterpolatedString = delimiter.IsFormat()
			return true
		}
		if hasFlags {
			return false
		}
	}

	return false
}
