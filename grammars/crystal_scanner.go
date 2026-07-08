//go:build !grammar_subset || grammar_subset_crystal

package grammars

import (
	"unicode"
	"unicode/utf8"

	gotreesitter "github.com/odvcencio/gotreesitter"
)

// ---------------------------------------------------------------------------
// Token indexes (validSymbols indices) — must match ExternalSymbols order from
// the compiled Crystal grammar binary.
//
// The compiled grammar binary has 55 external symbols. Some tokens present in
// the full grammar.js (START_OF_SYMBOL, UNQUOTED_SYMBOL_CONTENT,
// TYPE_FIELD_COLON, COMMAND_LITERAL_START, COMMAND_LITERAL_END, BINARY_STAR,
// all MACRO_* tokens, START_OF_MACRO_VAR_EXPS) are not present in this binary
// and are therefore omitted.
// ---------------------------------------------------------------------------
const (
	cryTokLineBreak             = iota // 0
	cryTokLineContinuation             // 1
	cryTokStartOfBraceBlock            // 2
	cryTokStartOfHashOrTuple           // 3
	cryTokStartOfNamedTuple            // 4
	cryTokStartOfTupleType             // 5
	cryTokStartOfNamedTupleType        // 6
	cryTokStartOfIndexOperator         // 7
	cryTokEndOfWithExpression          // 8
	cryTokUnaryPlus                    // 9
	cryTokUnaryMinus                   // 10
	cryTokBinaryPlus                   // 11
	cryTokBinaryMinus                  // 12
	cryTokUnaryWrappingPlus            // 13
	cryTokUnaryWrappingMinus           // 14
	cryTokBinaryWrappingPlus           // 15
	cryTokBinaryWrappingMinus          // 16
	cryTokPointerStar                  // 17
	cryTokUnaryStar                    // 18
	// NOTE: BINARY_STAR is not in the binary grammar (merged/removed)
	cryTokUnaryDoubleStar        // 19
	cryTokBinaryDoubleStar       // 20
	cryTokBlockAmpersand         // 21
	cryTokBinaryAmpersand        // 22
	cryTokBeginlessRangeOperator // 23
	cryTokRegexStart             // 24
	cryTokBinarySlash            // 25
	cryTokBinaryDoubleSlash      // 26
	cryTokRegularIfKeyword       // 27
	cryTokModifierIfKeyword      // 28
	cryTokRegularUnlessKeyword   // 29
	cryTokModifierUnlessKeyword  // 30
	cryTokRegularRescueKeyword   // 31
	cryTokModifierRescueKeyword  // 32
	cryTokRegularEnsureKeyword   // 33
	cryTokModifierEnsureKeyword  // 34
	cryTokModuloOperator         // 35
	// NOTE: START_OF_SYMBOL, UNQUOTED_SYMBOL_CONTENT, TYPE_FIELD_COLON not in binary
	cryTokStringLiteralStart      // 36
	cryTokDelimitedStringContents // 37
	cryTokStringLiteralEnd        // 38
	// NOTE: COMMAND_LITERAL_START, COMMAND_LITERAL_END not in binary
	cryTokStringPercentLiteralStart      // 39
	cryTokCommandPercentLiteralStart     // 40
	cryTokStringArrayPercentLiteralStart // 41
	cryTokSymbolArrayPercentLiteralStart // 42
	cryTokRegexPercentLiteralStart       // 43
	cryTokPercentLiteralEnd              // 44
	cryTokDelimitedArrayElementStart     // 45
	cryTokDelimitedArrayElementEnd       // 46
	cryTokHeredocStart                   // 47
	cryTokHeredocBodyStart               // 48
	cryTokHeredocContent                 // 49
	cryTokHeredocEnd                     // 50
	cryTokRegexModifier                  // 51
	// NOTE: MACRO_* tokens not in binary
	cryTokStartOfParenlessArgs // 52
	cryTokEndOfRange           // 53
	// NOTE: START_OF_MACRO_VAR_EXPS not in binary
	cryTokErrorRecovery // 54
	cryTokNone          // 55, sentinel, must be last
)

// Concrete Symbol IDs from the compiled Crystal grammar ExternalSymbols.
const (
	crySymLineBreak                      gotreesitter.Symbol = 151
	crySymLineContinuation               gotreesitter.Symbol = 152
	crySymStartOfBraceBlock              gotreesitter.Symbol = 153
	crySymStartOfHashOrTuple             gotreesitter.Symbol = 154
	crySymStartOfNamedTuple              gotreesitter.Symbol = 155
	crySymStartOfTupleType               gotreesitter.Symbol = 156
	crySymStartOfNamedTupleType          gotreesitter.Symbol = 157
	crySymStartOfIndexOperator           gotreesitter.Symbol = 158
	crySymEndOfWithExpression            gotreesitter.Symbol = 159
	crySymUnaryPlus                      gotreesitter.Symbol = 160
	crySymUnaryMinus                     gotreesitter.Symbol = 161
	crySymBinaryPlus                     gotreesitter.Symbol = 162
	crySymBinaryMinus                    gotreesitter.Symbol = 163
	crySymUnaryWrappingPlus              gotreesitter.Symbol = 164
	crySymUnaryWrappingMinus             gotreesitter.Symbol = 165
	crySymBinaryWrappingPlus             gotreesitter.Symbol = 166
	crySymBinaryWrappingMinus            gotreesitter.Symbol = 167
	crySymPointerStar                    gotreesitter.Symbol = 168
	crySymUnaryStar                      gotreesitter.Symbol = 169
	crySymUnaryDoubleStar                gotreesitter.Symbol = 170
	crySymBinaryDoubleStar               gotreesitter.Symbol = 171
	crySymBlockAmpersand                 gotreesitter.Symbol = 172
	crySymBinaryAmpersand                gotreesitter.Symbol = 173
	crySymBeginlessRangeOperator         gotreesitter.Symbol = 174
	crySymRegexStart                     gotreesitter.Symbol = 175
	crySymBinarySlash                    gotreesitter.Symbol = 176
	crySymBinaryDoubleSlash              gotreesitter.Symbol = 177
	crySymRegularIfKeyword               gotreesitter.Symbol = 178
	crySymModifierIfKeyword              gotreesitter.Symbol = 179
	crySymRegularUnlessKeyword           gotreesitter.Symbol = 180
	crySymModifierUnlessKeyword          gotreesitter.Symbol = 181
	crySymRegularRescueKeyword           gotreesitter.Symbol = 182
	crySymModifierRescueKeyword          gotreesitter.Symbol = 183
	crySymRegularEnsureKeyword           gotreesitter.Symbol = 184
	crySymModifierEnsureKeyword          gotreesitter.Symbol = 185
	crySymModuloOperator                 gotreesitter.Symbol = 186
	crySymStringLiteralStart             gotreesitter.Symbol = 187
	crySymDelimitedStringContents        gotreesitter.Symbol = 188
	crySymStringLiteralEnd               gotreesitter.Symbol = 189
	crySymStringPercentLiteralStart      gotreesitter.Symbol = 190
	crySymCommandPercentLiteralStart     gotreesitter.Symbol = 191
	crySymStringArrayPercentLiteralStart gotreesitter.Symbol = 192
	crySymSymbolArrayPercentLiteralStart gotreesitter.Symbol = 193
	crySymRegexPercentLiteralStart       gotreesitter.Symbol = 194
	crySymPercentLiteralEnd              gotreesitter.Symbol = 195
	crySymDelimitedArrayElementStart     gotreesitter.Symbol = 196
	crySymDelimitedArrayElementEnd       gotreesitter.Symbol = 197
	crySymHeredocStart                   gotreesitter.Symbol = 198
	crySymHeredocBodyStart               gotreesitter.Symbol = 199
	crySymHeredocContent                 gotreesitter.Symbol = 200
	crySymHeredocEnd                     gotreesitter.Symbol = 201
	crySymRegexModifier                  gotreesitter.Symbol = 202
	crySymStartOfParenlessArgs           gotreesitter.Symbol = 203
	crySymEndOfRange                     gotreesitter.Symbol = 204
	crySymErrorRecovery                  gotreesitter.Symbol = 205
)

// crySymTable maps token index -> concrete Symbol ID.
var crySymTable = [cryTokNone + 1]gotreesitter.Symbol{
	crySymLineBreak,                      // 0
	crySymLineContinuation,               // 1
	crySymStartOfBraceBlock,              // 2
	crySymStartOfHashOrTuple,             // 3
	crySymStartOfNamedTuple,              // 4
	crySymStartOfTupleType,               // 5
	crySymStartOfNamedTupleType,          // 6
	crySymStartOfIndexOperator,           // 7
	crySymEndOfWithExpression,            // 8
	crySymUnaryPlus,                      // 9
	crySymUnaryMinus,                     // 10
	crySymBinaryPlus,                     // 11
	crySymBinaryMinus,                    // 12
	crySymUnaryWrappingPlus,              // 13
	crySymUnaryWrappingMinus,             // 14
	crySymBinaryWrappingPlus,             // 15
	crySymBinaryWrappingMinus,            // 16
	crySymPointerStar,                    // 17
	crySymUnaryStar,                      // 18
	crySymUnaryDoubleStar,                // 19
	crySymBinaryDoubleStar,               // 20
	crySymBlockAmpersand,                 // 21
	crySymBinaryAmpersand,                // 22
	crySymBeginlessRangeOperator,         // 23
	crySymRegexStart,                     // 24
	crySymBinarySlash,                    // 25
	crySymBinaryDoubleSlash,              // 26
	crySymRegularIfKeyword,               // 27
	crySymModifierIfKeyword,              // 28
	crySymRegularUnlessKeyword,           // 29
	crySymModifierUnlessKeyword,          // 30
	crySymRegularRescueKeyword,           // 31
	crySymModifierRescueKeyword,          // 32
	crySymRegularEnsureKeyword,           // 33
	crySymModifierEnsureKeyword,          // 34
	crySymModuloOperator,                 // 35
	crySymStringLiteralStart,             // 36
	crySymDelimitedStringContents,        // 37
	crySymStringLiteralEnd,               // 38
	crySymStringPercentLiteralStart,      // 39
	crySymCommandPercentLiteralStart,     // 40
	crySymStringArrayPercentLiteralStart, // 41
	crySymSymbolArrayPercentLiteralStart, // 42
	crySymRegexPercentLiteralStart,       // 43
	crySymPercentLiteralEnd,              // 44
	crySymDelimitedArrayElementStart,     // 45
	crySymDelimitedArrayElementEnd,       // 46
	crySymHeredocStart,                   // 47
	crySymHeredocBodyStart,               // 48
	crySymHeredocContent,                 // 49
	crySymHeredocEnd,                     // 50
	crySymRegexModifier,                  // 51
	crySymStartOfParenlessArgs,           // 52
	crySymEndOfRange,                     // 53
	crySymErrorRecovery,                  // 54
	0,                                    // 55 = NONE sentinel
}

// ---------------------------------------------------------------------------
// Literal types (for percent / string / heredoc literals)
// ---------------------------------------------------------------------------
const (
	cryLitString      = 0
	cryLitStringNoEsc = 1
	cryLitCommand     = 2
	cryLitStringArray = 3
	cryLitSymbolArray = 4
	cryLitRegex       = 5
)

// ---------------------------------------------------------------------------
// State types
// ---------------------------------------------------------------------------

const cryMaxLiteralCount = 16
const cryMaxHeredocCount = 16
const cryHeredocBufferSize = 512
const cryMaxHeredocWordSize = 255

type cryPercentLiteral struct {
	openingChar  byte
	closingChar  byte
	nestingLevel byte
	litType      byte
}

type cryHeredoc struct {
	allowEscapes bool
	started      bool
	identifier   []byte // UTF-8 encoded
}

type cryScannerState struct {
	hasLeadingWhitespace  bool
	previousLineContinued bool

	// Nested delimited literals (percent strings, strings, etc.)
	literals []cryPercentLiteral

	// Queue of heredocs
	heredocs []cryHeredoc
}

// ---------------------------------------------------------------------------
// Scan result type (mirrors C ScanResult enum)
// ---------------------------------------------------------------------------
const (
	crySRContinue      = 0
	crySRStop          = 1
	crySRStopNoContent = 2
)

// ---------------------------------------------------------------------------
// CrystalExternalScanner
// ---------------------------------------------------------------------------

type CrystalExternalScanner struct{}

func (CrystalExternalScanner) Create() any {
	return &cryScannerState{}
}

func (CrystalExternalScanner) Destroy(payload any) {}

func (CrystalExternalScanner) Serialize(payload any, buf []byte) int {
	s := payload.(*cryScannerState)
	offset := 0

	if offset+6 > len(buf) {
		return 0
	}

	buf[offset] = boolToByte(s.hasLeadingWhitespace)
	offset++
	buf[offset] = boolToByte(s.previousLineContinued)
	offset++

	// Macro state fields removed since they're not in the binary grammar.
	// We still need 2 placeholder bytes for compatibility: always false.
	buf[offset] = 0 // in_comment placeholder
	offset++
	buf[offset] = 1 // non_modifier_keyword_can_begin placeholder
	offset++

	// Literals
	litCount := len(s.literals)
	if litCount > cryMaxLiteralCount {
		litCount = cryMaxLiteralCount
	}
	buf[offset] = byte(litCount)
	offset++

	for i := 0; i < litCount; i++ {
		if offset+4 > len(buf) {
			return 0
		}
		lit := &s.literals[i]
		buf[offset] = lit.openingChar
		offset++
		buf[offset] = lit.closingChar
		offset++
		buf[offset] = lit.nestingLevel
		offset++
		buf[offset] = lit.litType
		offset++
	}

	// Heredocs
	heredocCount := len(s.heredocs)
	if heredocCount > cryMaxHeredocCount {
		heredocCount = cryMaxHeredocCount
	}
	if offset >= len(buf) {
		return 0
	}
	buf[offset] = byte(heredocCount)
	offset++

	for i := 0; i < heredocCount; i++ {
		hd := &s.heredocs[i]
		if offset+3+len(hd.identifier) > len(buf) {
			return 0
		}
		buf[offset] = boolToByte(hd.allowEscapes)
		offset++
		buf[offset] = boolToByte(hd.started)
		offset++
		buf[offset] = byte(len(hd.identifier))
		offset++
		copy(buf[offset:], hd.identifier)
		offset += len(hd.identifier)
	}

	return offset
}

func (CrystalExternalScanner) Deserialize(payload any, buf []byte) {
	s := payload.(*cryScannerState)

	s.hasLeadingWhitespace = false
	s.previousLineContinued = false
	s.literals = s.literals[:0]
	s.heredocs = s.heredocs[:0]

	if len(buf) == 0 {
		return
	}

	offset := 0
	if offset >= len(buf) {
		return
	}
	s.hasLeadingWhitespace = buf[offset] != 0
	offset++
	if offset >= len(buf) {
		return
	}
	s.previousLineContinued = buf[offset] != 0
	offset++

	// Skip macro state fields (2 bytes)
	if offset+2 > len(buf) {
		return
	}
	offset += 2

	if offset >= len(buf) {
		return
	}
	litCount := int(buf[offset])
	offset++

	for i := 0; i < litCount; i++ {
		if offset+4 > len(buf) {
			break
		}
		lit := cryPercentLiteral{}
		lit.openingChar = buf[offset]
		offset++
		lit.closingChar = buf[offset]
		offset++
		lit.nestingLevel = buf[offset]
		offset++
		lit.litType = buf[offset]
		offset++
		s.literals = append(s.literals, lit)
	}

	if offset >= len(buf) {
		return
	}
	heredocCount := int(buf[offset])
	offset++

	for i := 0; i < heredocCount; i++ {
		if offset+3 > len(buf) {
			break
		}
		hd := cryHeredoc{}
		hd.allowEscapes = buf[offset] != 0
		offset++
		hd.started = buf[offset] != 0
		offset++
		idLen := int(buf[offset])
		offset++
		if offset+idLen > len(buf) {
			break
		}
		hd.identifier = make([]byte, idLen)
		copy(hd.identifier, buf[offset:offset+idLen])
		offset += idLen
		s.heredocs = append(s.heredocs, hd)
	}
}

func (CrystalExternalScanner) Scan(payload any, lexer *gotreesitter.ExternalLexer, validSymbols []bool) bool {
	s := payload.(*cryScannerState)
	return cryInnerScan(s, lexer, validSymbols)
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func boolToByte(b bool) byte {
	if b {
		return 1
	}
	return 0
}

func cryIsValid(validSymbols []bool, idx int) bool {
	return idx >= 0 && idx < len(validSymbols) && validSymbols[idx]
}

func crySkip(s *cryScannerState, lexer *gotreesitter.ExternalLexer) {
	s.hasLeadingWhitespace = true
	lexer.Advance(true)
}

func cryAdvance(lexer *gotreesitter.ExternalLexer) {
	lexer.Advance(false)
}

func cryIsEOF(lexer *gotreesitter.ExternalLexer) bool {
	return lexer.Lookahead() == 0
}

func crySetResult(lexer *gotreesitter.ExternalLexer, tok int) {
	lexer.SetResultSymbol(crySymTable[tok])
}

func cryHasActiveLiteral(s *cryScannerState) bool {
	return len(s.literals) > 0
}

func cryActiveLiteral(s *cryScannerState) *cryPercentLiteral {
	return &s.literals[len(s.literals)-1]
}

func cryPushLiteral(s *cryScannerState, lit cryPercentLiteral) {
	s.literals = append(s.literals, lit)
}

func cryPopLiteral(s *cryScannerState) {
	if len(s.literals) > 0 {
		s.literals = s.literals[:len(s.literals)-1]
	}
}

func cryHasActiveHeredoc(s *cryScannerState) bool {
	for i := range s.heredocs {
		if s.heredocs[i].started {
			return true
		}
	}
	return false
}

func cryHasUnstartedHeredoc(s *cryScannerState) bool {
	if len(s.heredocs) == 0 {
		return false
	}
	return !s.heredocs[0].started
}

func cryPopHeredoc(s *cryScannerState) {
	if len(s.heredocs) > 0 {
		s.heredocs = s.heredocs[1:]
	}
}

func cryHeredocCurrentBufferSize(s *cryScannerState) int {
	total := 0
	for i := range s.heredocs {
		total += len(s.heredocs[i].identifier)
	}
	return total
}

func cryHasRoomForHeredoc(s *cryScannerState, identLen int) bool {
	if len(s.heredocs) >= cryMaxHeredocCount {
		return false
	}
	return (cryHeredocCurrentBufferSize(s) + identLen) <= cryHeredocBufferSize
}

func cryPushHeredoc(s *cryScannerState, hd cryHeredoc) {
	if cryHasActiveHeredoc(s) {
		// Insert before the currently-active heredoc (nested heredoc)
		insertIdx := 0
		for i := range s.heredocs {
			if s.heredocs[i].started {
				insertIdx = i
				break
			}
		}
		// Insert at insertIdx
		s.heredocs = append(s.heredocs, cryHeredoc{})
		copy(s.heredocs[insertIdx+1:], s.heredocs[insertIdx:])
		s.heredocs[insertIdx] = hd
	} else {
		s.heredocs = append(s.heredocs, hd)
	}
}

func cryNextCharIsIdentifier(lexer *gotreesitter.ExternalLexer) bool {
	la := lexer.Lookahead()
	return cryIsAlphaNumeric(la) || la == '_' || la == '?' || la == '!' || la >= 0xa0
}

func cryIsIdentPart(cp rune) bool {
	return ('0' <= cp && cp <= '9') ||
		('A' <= cp && cp <= 'Z') ||
		('a' <= cp && cp <= 'z') ||
		cp == '_' ||
		(0x00a0 <= cp && cp <= 0x10ffff)
}

func cryIsAlphaNumeric(cp rune) bool {
	return ('0' <= cp && cp <= '9') ||
		('A' <= cp && cp <= 'Z') ||
		('a' <= cp && cp <= 'z')
}

// cryCodepointToUTF8 encodes a rune as UTF-8 into dst and returns the number
// of bytes written (0 if invalid).
func cryCodepointToUTF8(cp rune, dst []byte) int {
	if !utf8.ValidRune(cp) {
		return 0
	}
	return utf8.EncodeRune(dst, cp)
}

// cryUTF8ToCodepoints decodes a UTF-8 byte slice into a slice of runes.
func cryUTF8ToCodepoints(b []byte) []rune {
	runes := make([]rune, 0, len(b))
	for len(b) > 0 {
		r, size := utf8.DecodeRune(b)
		if r == utf8.RuneError && size <= 1 {
			break
		}
		runes = append(runes, r)
		b = b[size:]
	}
	return runes
}

// ---------------------------------------------------------------------------
// check_for_heredoc_start
// ---------------------------------------------------------------------------

func cryCheckForHeredocStart(s *cryScannerState, lexer *gotreesitter.ExternalLexer, validSymbols []bool) bool {
	if cryIsValid(validSymbols, cryTokHeredocBodyStart) &&
		cryHasUnstartedHeredoc(s) &&
		!s.previousLineContinued &&
		!cryIsEOF(lexer) &&
		lexer.Column() == 0 {

		s.heredocs[0].started = true
		crySetResult(lexer, cryTokHeredocBodyStart)
		return true
	}
	return false
}

// ---------------------------------------------------------------------------
// scan_whitespace
// ---------------------------------------------------------------------------

func cryScanWhitespace(s *cryScannerState, lexer *gotreesitter.ExternalLexer, validSymbols []bool) (ok bool, resultToken int) {
	crossedNewline := false

	for {
		la := lexer.Lookahead()
		switch la {
		case ' ', '\t', '\r':
			if la == '\r' {
				// Skip \r
			}
			crySkip(s, lexer)

		case '\n':
			if cryIsValid(validSymbols, cryTokHeredocBodyStart) && cryHasUnstartedHeredoc(s) {
				s.heredocs[0].started = true
				crySkip(s, lexer)
				crySetResult(lexer, cryTokHeredocBodyStart)
				return true, cryTokHeredocBodyStart
			} else if cryIsValid(validSymbols, cryTokLineBreak) && !crossedNewline {
				cryAdvance(lexer)
				lexer.MarkEnd()
				crossedNewline = true
				s.hasLeadingWhitespace = true
			} else {
				crySkip(s, lexer)
			}

		case '\v', '\f':
			// In regular code, these characters are not allowed. But they
			// may be used in between strings in a %w array.
			if cryHasActiveLiteral(s) {
				crySkip(s, lexer)
			} else {
				return false, cryTokNone
			}

		default:
			if crossedNewline {
				if la == '.' {
					// Check if this is the continuation of a method call,
					// or the start of a beginless range literal.
					cryAdvance(lexer)
					if lexer.Lookahead() == '.' {
						return true, cryTokLineBreak
					}
					// Not a beginless range, treat as method chain continuation
				} else if la == '#' {
					// Comments don't interrupt line continuations
				} else {
					return true, cryTokLineBreak
				}
			}
			return true, cryTokNone
		}
	}
}

// ---------------------------------------------------------------------------
// scan_string_contents
// ---------------------------------------------------------------------------

func cryScanStringContents(s *cryScannerState, lexer *gotreesitter.ExternalLexer, validSymbols []bool) int {
	foundContent := false
	lexer.SetResultSymbol(crySymDelimitedStringContents)

	for {
		if cryIsEOF(lexer) {
			if foundContent {
				return crySRStop
			}
			return crySRStopNoContent
		}

		activeType := cryActiveLiteral(s).litType

		switch lexer.Lookahead() {
		case '\\':
			switch activeType {
			case cryLitString, cryLitCommand:
				if foundContent {
					return crySRStop
				}
				// do the regular check for LINE_CONTINUATION
				return crySRContinue

			case cryLitRegex:
				// No special regex escapes in content scanning
				cryAdvance(lexer)

			case cryLitStringNoEsc:
				// No action, just fall through to advance below

			case cryLitStringArray, cryLitSymbolArray:
				// %w and %i allow only '\<whitespace>' or the closing
				// delimiter as an escape sequence.
				lexer.MarkEnd()
				cryAdvance(lexer)
				if unicode.IsSpace(lexer.Lookahead()) || rune(cryActiveLiteral(s).closingChar) == lexer.Lookahead() {
					if foundContent {
						return crySRStop
					}
					return crySRStopNoContent
				}
				// The backslash must be part of the word contents.
				foundContent = true
				lexer.MarkEnd()
				continue
			}

		case '#':
			if activeType == cryLitStringNoEsc || activeType == cryLitStringArray || activeType == cryLitSymbolArray {
				// These types don't allow interpolation
				// fall through
			} else {
				lexer.MarkEnd()
				cryAdvance(lexer)
				if lexer.Lookahead() == '{' {
					if foundContent {
						return crySRStop
					}
					return crySRStopNoContent
				}
				foundContent = true
				lexer.MarkEnd()
				continue
			}

		case ' ', '\t', '\n', '\r', '\v', '\f':
			if activeType == cryLitStringArray || activeType == cryLitSymbolArray {
				if foundContent {
					return crySRStop
				} else if cryIsValid(validSymbols, cryTokDelimitedArrayElementEnd) {
					lexer.SetResultSymbol(crySymDelimitedArrayElementEnd)
					return crySRStop
				}
			}

		case '"', '|', '`':
			// These delimiters can't nest
			if rune(cryActiveLiteral(s).closingChar) == lexer.Lookahead() {
				if foundContent {
					return crySRStop
				} else if cryIsValid(validSymbols, cryTokDelimitedArrayElementEnd) {
					lexer.SetResultSymbol(crySymDelimitedArrayElementEnd)
					return crySRStop
				}
				return crySRContinue
			}

		case '(', '[', '{', '<':
			if rune(cryActiveLiteral(s).openingChar) == lexer.Lookahead() {
				cryActiveLiteral(s).nestingLevel++
			}

		case ')', ']', '}', '>':
			if rune(cryActiveLiteral(s).closingChar) == lexer.Lookahead() {
				if cryActiveLiteral(s).nestingLevel == 0 {
					if foundContent {
						return crySRStop
					} else if cryIsValid(validSymbols, cryTokDelimitedArrayElementEnd) {
						lexer.SetResultSymbol(crySymDelimitedArrayElementEnd)
						return crySRStop
					}
					return crySRContinue
				}
				cryActiveLiteral(s).nestingLevel--
			}
		}

		cryAdvance(lexer)
		lexer.MarkEnd()
		foundContent = true
	}
}

// ---------------------------------------------------------------------------
// scan_heredoc_contents
// ---------------------------------------------------------------------------

func cryScanHeredocContents(s *cryScannerState, lexer *gotreesitter.ExternalLexer, validSymbols []bool) bool {
	if cryIsValid(validSymbols, cryTokErrorRecovery) && !cryHasActiveHeredoc(s) {
		return false
	}

	foundContent := false

	// Find the active heredoc (first started one)
	activeHeredoc := &s.heredocs[0]
	heredocPendingStart := false

	if !activeHeredoc.started {
		// The first heredoc in the queue isn't started; find the started one
		heredocPendingStart = true
		for i := 1; i < len(s.heredocs); i++ {
			if s.heredocs[i].started {
				activeHeredoc = &s.heredocs[i]
				break
			}
		}
	}

	for {
		// start_of_line:
		if foundContent && heredocPendingStart {
			return true
		}

		if cryIsValid(validSymbols, cryTokHeredocEnd) && !cryIsEOF(lexer) && lexer.Column() == 0 {
			if foundContent {
				lexer.MarkEnd()
				for lexer.Lookahead() == '\t' || lexer.Lookahead() == ' ' {
					cryAdvance(lexer)
				}
			} else {
				for lexer.Lookahead() == '\t' || lexer.Lookahead() == ' ' {
					crySkip(s, lexer)
				}
				lexer.MarkEnd()
			}

			codepoints := cryUTF8ToCodepoints(activeHeredoc.identifier)
			matchedCount := 0

			for matchedCount < len(codepoints) {
				if lexer.Lookahead() == codepoints[matchedCount] {
					cryAdvance(lexer)
					matchedCount++
				} else {
					break
				}
			}

			endOfLine := lexer.Lookahead() == '\n' || lexer.Lookahead() == '\r' || cryIsEOF(lexer)

			if matchedCount == len(codepoints) && endOfLine {
				if foundContent {
					// Return content; next call will match the heredoc end
					crySetResult(lexer, cryTokHeredocContent)
					return true
				}
				cryPopHeredoc(s)
				lexer.MarkEnd()
				crySetResult(lexer, cryTokHeredocEnd)
				return true
			}

			if matchedCount > 0 {
				foundContent = true
				lexer.MarkEnd()
			}
		}

		// Scan for string contents within the heredoc
		crySetResult(lexer, cryTokHeredocContent)

		for {
			if cryIsEOF(lexer) {
				return foundContent
			}

			switch lexer.Lookahead() {
			case '\\':
				if activeHeredoc.allowEscapes {
					return foundContent
				}

			case '#':
				if !activeHeredoc.allowEscapes {
					// fall through
				} else {
					lexer.MarkEnd()
					cryAdvance(lexer)
					if lexer.Lookahead() == '{' {
						return foundContent
					}
					foundContent = true
					lexer.MarkEnd()
					continue
				}

			case '\r':
				cryAdvance(lexer)
				lexer.MarkEnd()
				foundContent = true
				if lexer.Lookahead() != '\n' {
					continue
				}
				// fall through to \n
				cryAdvance(lexer)
				lexer.MarkEnd()
				foundContent = true
				goto startOfLine

			case '\n':
				cryAdvance(lexer)
				lexer.MarkEnd()
				foundContent = true
				goto startOfLine
			}

			cryAdvance(lexer)
			lexer.MarkEnd()
			foundContent = true
		}

	startOfLine:
	}
}

// ---------------------------------------------------------------------------
// scan_regex_modifier
// ---------------------------------------------------------------------------

func cryScanRegexModifier(s *cryScannerState, lexer *gotreesitter.ExternalLexer) bool {
	if !s.hasLeadingWhitespace {
		foundModifier := false
		for {
			switch lexer.Lookahead() {
			case 'i', 'm', 'x':
				foundModifier = true
				cryAdvance(lexer)
				continue
			}
			if foundModifier {
				crySetResult(lexer, cryTokRegexModifier)
				return true
			}
			break
		}
	}
	return false
}

// ---------------------------------------------------------------------------
// Lookahead helpers for type / named tuple disambiguation
// ---------------------------------------------------------------------------

const (
	cryLookaheadUnknown    = 0
	cryLookaheadType       = 1
	cryLookaheadNamedTuple = 2
)

func cryAdvanceSpace(lexer *gotreesitter.ExternalLexer) {
	for lexer.Lookahead() == ' ' || lexer.Lookahead() == '\t' {
		cryAdvance(lexer)
	}
}

func cryAdvanceSpaceAndNewline(lexer *gotreesitter.ExternalLexer) {
	for {
		la := lexer.Lookahead()
		if la == ' ' || la == '\t' || la == '\r' || la == '\n' {
			cryAdvance(lexer)
		} else {
			break
		}
	}
}

func cryConsumeConst(lexer *gotreesitter.ExternalLexer) {
	la := lexer.Lookahead()
	if 'A' <= la && la <= 'Z' {
		cryAdvance(lexer)
		for cryIsIdentPart(lexer.Lookahead()) {
			cryAdvance(lexer)
		}
	}
}

func cryConsumeStringLiteral(lexer *gotreesitter.ExternalLexer) {
	canEscape := true
	canNest := false
	var openingChar, closingChar rune
	nestingLevel := 0

	if lexer.Lookahead() == '"' {
		openingChar = '"'
		closingChar = '"'
		canNest = false
	} else if lexer.Lookahead() == '%' {
		cryAdvance(lexer)
		if lexer.Lookahead() == 'q' {
			canEscape = false
			cryAdvance(lexer)
		} else if lexer.Lookahead() == 'Q' {
			cryAdvance(lexer)
		}

		switch lexer.Lookahead() {
		case '{':
			openingChar = '{'
			closingChar = '}'
			canNest = true
		case '(':
			openingChar = '('
			closingChar = ')'
			canNest = true
		case '[':
			openingChar = '['
			closingChar = ']'
			canNest = true
		case '<':
			openingChar = '<'
			closingChar = '>'
			canNest = true
		case '|':
			openingChar = '|'
			closingChar = '|'
			canNest = false
		}
	}

	if openingChar == 0 {
		return
	}

	// advance past opening char
	cryAdvance(lexer)

	for {
		if cryIsEOF(lexer) {
			return
		}
		if lexer.Lookahead() == '\\' && canEscape {
			cryAdvance(lexer)
			cryAdvance(lexer)
			continue
		}
		if lexer.Lookahead() == closingChar {
			cryAdvance(lexer)
			if nestingLevel == 0 {
				return
			}
			nestingLevel--
			continue
		}
		if lexer.Lookahead() == openingChar && canNest {
			cryAdvance(lexer)
			nestingLevel++
			continue
		}
		cryAdvance(lexer)
	}
}

func cryLookaheadDelimiterOrTypeSuffix(lexer *gotreesitter.ExternalLexer) int {
	if cryIsEOF(lexer) {
		return cryLookaheadType
	}

	switch lexer.Lookahead() {
	case '.':
		cryAdvance(lexer)
		cryAdvanceSpaceAndNewline(lexer)
		if lexer.Lookahead() != 'c' {
			return cryLookaheadUnknown
		}
		cryAdvance(lexer)
		if lexer.Lookahead() != 'l' {
			return cryLookaheadUnknown
		}
		cryAdvance(lexer)
		if lexer.Lookahead() != 'a' {
			return cryLookaheadUnknown
		}
		cryAdvance(lexer)
		if lexer.Lookahead() != 's' {
			return cryLookaheadUnknown
		}
		cryAdvance(lexer)
		if lexer.Lookahead() != 's' {
			return cryLookaheadUnknown
		}
		cryAdvance(lexer)
		if cryIsIdentPart(lexer.Lookahead()) {
			return cryLookaheadUnknown
		}
		return cryLookaheadType

	case '?', '*':
		cryAdvance(lexer)
		return cryLookaheadDelimiterOrTypeSuffix(lexer)

	case '-':
		cryAdvance(lexer)
		if lexer.Lookahead() == '>' {
			return cryLookaheadType
		}
		return cryLookaheadUnknown

	case '=':
		cryAdvance(lexer)
		switch lexer.Lookahead() {
		case '>':
			return cryLookaheadType
		case '=', '~':
			return cryLookaheadUnknown
		default:
			return cryLookaheadType
		}

	case '|', ',', ';', '\n', '(', ')', '[', ']':
		return cryLookaheadType

	default:
		return cryLookaheadUnknown
	}
}

func cryLookaheadStartOfNamedTupleEntry(lexer *gotreesitter.ExternalLexer, started bool) int {
	la := lexer.Lookahead()
	if started ||
		('a' <= la && la <= 'z') ||
		('A' <= la && la <= 'Z') ||
		la == '_' ||
		(0x00a0 <= la && la <= 0x10ffff) {

		for {
			la = lexer.Lookahead()
			if ('0' <= la && la <= '9') ||
				('A' <= la && la <= 'Z') ||
				('a' <= la && la <= 'z') ||
				la == '_' ||
				(0x00a0 <= la && la <= 0x10ffff) {
				cryAdvance(lexer)
			} else {
				break
			}
		}

		if lexer.Lookahead() == '!' || lexer.Lookahead() == '?' {
			cryAdvance(lexer)
		}

		if lexer.Lookahead() == ':' {
			cryAdvance(lexer)
			if lexer.Lookahead() == ':' {
				return cryLookaheadUnknown
			}
			return cryLookaheadNamedTuple
		}
	}

	if lexer.Lookahead() == '"' || lexer.Lookahead() == '%' {
		cryConsumeStringLiteral(lexer)
		if lexer.Lookahead() == ':' {
			cryAdvance(lexer)
			if lexer.Lookahead() == ':' {
				return cryLookaheadUnknown
			}
			return cryLookaheadNamedTuple
		}
	}

	return cryLookaheadUnknown
}

func cryLookaheadStartOfType(s *cryScannerState, lexer *gotreesitter.ExternalLexer) int {
	cryAdvanceSpace(lexer)

	if cryIsEOF(lexer) {
		return cryLookaheadUnknown
	}

	for lexer.Lookahead() == '{' || lexer.Lookahead() == '(' {
		cryAdvance(lexer)
		cryAdvanceSpaceAndNewline(lexer)
	}

	// Check for 'typeof' identifier
	if lexer.Lookahead() == 't' {
		cryAdvance(lexer)
		if lexer.Lookahead() == 'y' {
			cryAdvance(lexer)
			if lexer.Lookahead() == 'p' {
				cryAdvance(lexer)
				if lexer.Lookahead() == 'e' {
					cryAdvance(lexer)
					if lexer.Lookahead() == 'o' {
						cryAdvance(lexer)
						if lexer.Lookahead() == 'f' {
							cryAdvance(lexer)
							if lexer.Lookahead() == ':' {
								return cryLookaheadNamedTuple
							}
							if cryIsIdentPart(lexer.Lookahead()) || lexer.Lookahead() == '!' || lexer.Lookahead() == '?' {
								return cryLookaheadStartOfNamedTupleEntry(lexer, true)
							}
							return cryLookaheadType
						}
					}
				}
			}
		}
	} else if lexer.Lookahead() == 's' {
		cryAdvance(lexer)
		if lexer.Lookahead() == 'e' {
			cryAdvance(lexer)
			if lexer.Lookahead() == 'l' {
				cryAdvance(lexer)
				if lexer.Lookahead() == 'f' {
					cryAdvance(lexer)
					if lexer.Lookahead() == ':' {
						return cryLookaheadNamedTuple
					}
					if cryIsIdentPart(lexer.Lookahead()) || lexer.Lookahead() == '!' {
						return cryLookaheadStartOfNamedTupleEntry(lexer, true)
					}
					cryAdvanceSpace(lexer)
					return cryLookaheadDelimiterOrTypeSuffix(lexer)
				}
			}
		}
	} else if ('a' <= lexer.Lookahead() && lexer.Lookahead() <= 'z') ||
		(0x00a0 <= lexer.Lookahead() && lexer.Lookahead() <= 0x10ffff) {
		// other identifiers are not part of the type grammar
		return cryLookaheadStartOfNamedTupleEntry(lexer, false)
	}

	// Check for constant
	for 'A' <= lexer.Lookahead() && lexer.Lookahead() <= 'Z' {
		cryConsumeConst(lexer)

		if lexer.Lookahead() == ':' {
			cryAdvance(lexer)
			if lexer.Lookahead() == ':' {
				cryAdvance(lexer)
				cryAdvanceSpaceAndNewline(lexer)
				// continue consuming const segments
			} else {
				return cryLookaheadNamedTuple
			}
		} else {
			cryAdvanceSpace(lexer)
			return cryLookaheadDelimiterOrTypeSuffix(lexer)
		}
	}

	switch lexer.Lookahead() {
	case '_':
		cryAdvance(lexer)
		if !cryIsAlphaNumeric(lexer.Lookahead()) {
			return cryLookaheadType
		}
	case '-':
		cryAdvance(lexer)
		if lexer.Lookahead() == '>' {
			return cryLookaheadType
		}
	case '*':
		cryAdvance(lexer)
		cryAdvanceSpaceAndNewline(lexer)
		if lexer.Lookahead() == '*' {
			return cryLookaheadUnknown
		}
		result := cryLookaheadStartOfType(s, lexer)
		if result == cryLookaheadType {
			return cryLookaheadType
		}
		return cryLookaheadUnknown
	case '"', '%':
		return cryLookaheadStartOfNamedTupleEntry(lexer, false)
	}

	return cryLookaheadUnknown
}

// ---------------------------------------------------------------------------
// inner_scan — main scanner dispatch
// ---------------------------------------------------------------------------

func cryInnerScan(s *cryScannerState, lexer *gotreesitter.ExternalLexer, validSymbols []bool) bool {
	s.hasLeadingWhitespace = false

	if cryCheckForHeredocStart(s, lexer, validSymbols) {
		return true
	}

	// The previousLineContinued flag only matters for check_for_heredoc_start,
	// so it can now be cleared.
	if s.previousLineContinued {
		s.previousLineContinued = false
	}

	// Delimited string contents
	if cryIsValid(validSymbols, cryTokDelimitedStringContents) && cryHasActiveLiteral(s) {
		switch cryScanStringContents(s, lexer, validSymbols) {
		case crySRStop:
			return true
		case crySRStopNoContent:
			return false
		case crySRContinue:
			// continue to the rest of the scanner
		}
	}

	// Heredoc contents
	if cryIsValid(validSymbols, cryTokHeredocContent) && len(s.heredocs) > 0 && cryScanHeredocContents(s, lexer, validSymbols) {
		return true
	}

	// Whitespace handling
	resultTok := cryTokNone
	ok := false
	ok, resultTok = cryScanWhitespace(s, lexer, validSymbols)
	if !ok {
		return false
	}
	if resultTok != cryTokNone {
		crySetResult(lexer, resultTok)
		return true
	}

	// Percent literal end
	if cryIsValid(validSymbols, cryTokPercentLiteralEnd) && cryHasActiveLiteral(s) {
		if lexer.Lookahead() == rune(cryActiveLiteral(s).closingChar) {
			cryAdvance(lexer)
			cryPopLiteral(s)
			crySetResult(lexer, cryTokPercentLiteralEnd)
			return true
		}
	}

	// String literal end
	if cryIsValid(validSymbols, cryTokStringLiteralEnd) && cryHasActiveLiteral(s) {
		if lexer.Lookahead() == rune(cryActiveLiteral(s).closingChar) {
			cryAdvance(lexer)
			cryPopLiteral(s)
			crySetResult(lexer, cryTokStringLiteralEnd)
			return true
		}
	}

	// Delimited array element start
	if cryIsValid(validSymbols, cryTokDelimitedArrayElementStart) && cryHasActiveLiteral(s) {
		crySetResult(lexer, cryTokDelimitedArrayElementStart)
		return true
	}

	// Regex modifier
	if cryIsValid(validSymbols, cryTokRegexModifier) && cryScanRegexModifier(s, lexer) {
		return true
	}

	switch lexer.Lookahead() {
	case '{':
		cryAdvance(lexer)

		// Start of a macro expression
		if lexer.Lookahead() == '{' || lexer.Lookahead() == '%' {
			return false
		}

		braceBlock := cryIsValid(validSymbols, cryTokStartOfBraceBlock)
		braceExpr := cryIsValid(validSymbols, cryTokStartOfHashOrTuple) || cryIsValid(validSymbols, cryTokStartOfNamedTuple)
		braceType := cryIsValid(validSymbols, cryTokStartOfTupleType) || cryIsValid(validSymbols, cryTokStartOfNamedTupleType)

		if braceBlock || braceExpr || braceType {
			if braceBlock && braceExpr && braceType {
				if cryIsValid(validSymbols, cryTokErrorRecovery) {
					return false
				}
				return false
			} else if braceBlock && braceExpr {
				if cryIsValid(validSymbols, cryTokStartOfParenlessArgs) {
					crySetResult(lexer, cryTokStartOfBraceBlock)
					return true
				}

				if cryIsValid(validSymbols, cryTokEndOfRange) {
					lexer.MarkEnd()
					cryAdvanceSpaceAndNewline(lexer)

					switch cryLookaheadStartOfNamedTupleEntry(lexer, false) {
					case cryLookaheadNamedTuple:
						crySetResult(lexer, cryTokStartOfNamedTuple)
						return true
					default:
						crySetResult(lexer, cryTokStartOfHashOrTuple)
						return true
					}
				}

				// Array-like or hash-like constructor
				if cryIsValid(validSymbols, cryTokStartOfHashOrTuple) && !cryIsValid(validSymbols, cryTokStartOfNamedTuple) {
					crySetResult(lexer, cryTokStartOfHashOrTuple)
					return true
				}

				return false

			} else if braceBlock && braceType {
				lexer.MarkEnd()

				switch cryLookaheadStartOfType(s, lexer) {
				case cryLookaheadType:
					crySetResult(lexer, cryTokStartOfTupleType)
					return true
				case cryLookaheadNamedTuple:
					crySetResult(lexer, cryTokStartOfBraceBlock)
					return true
				default:
					crySetResult(lexer, cryTokStartOfBraceBlock)
					return true
				}

			} else if braceExpr && braceType {
				lexer.MarkEnd()

				switch cryLookaheadStartOfType(s, lexer) {
				case cryLookaheadType:
					crySetResult(lexer, cryTokStartOfTupleType)
					return true
				case cryLookaheadNamedTuple:
					crySetResult(lexer, cryTokStartOfNamedTuple)
					return true
				default:
					crySetResult(lexer, cryTokStartOfHashOrTuple)
					return true
				}

			} else if braceExpr {
				lexer.MarkEnd()
				cryAdvanceSpaceAndNewline(lexer)

				if cryIsValid(validSymbols, cryTokStartOfHashOrTuple) && !cryIsValid(validSymbols, cryTokStartOfNamedTuple) {
					crySetResult(lexer, cryTokStartOfHashOrTuple)
					return true
				} else if cryIsValid(validSymbols, cryTokStartOfNamedTuple) && !cryIsValid(validSymbols, cryTokStartOfHashOrTuple) {
					crySetResult(lexer, cryTokStartOfNamedTuple)
					return true
				}

				switch cryLookaheadStartOfNamedTupleEntry(lexer, false) {
				case cryLookaheadNamedTuple:
					crySetResult(lexer, cryTokStartOfNamedTuple)
					return true
				default:
					crySetResult(lexer, cryTokStartOfHashOrTuple)
					return true
				}

			} else if braceType {
				lexer.MarkEnd()
				cryAdvanceSpaceAndNewline(lexer)

				switch cryLookaheadStartOfNamedTupleEntry(lexer, false) {
				case cryLookaheadNamedTuple:
					crySetResult(lexer, cryTokStartOfNamedTupleType)
					return true
				default:
					crySetResult(lexer, cryTokStartOfTupleType)
					return true
				}

			} else if braceBlock {
				crySetResult(lexer, cryTokStartOfBraceBlock)
				return true
			}
		}

	case '[':
		if cryIsValid(validSymbols, cryTokStartOfIndexOperator) {
			if s.hasLeadingWhitespace && cryIsValid(validSymbols, cryTokStartOfParenlessArgs) {
				return false
			}
			cryAdvance(lexer)
			crySetResult(lexer, cryTokStartOfIndexOperator)
			return true
		}

	case '<':
		if cryIsValid(validSymbols, cryTokHeredocStart) {
			cryAdvance(lexer)
			if lexer.Lookahead() == '<' {
				cryAdvance(lexer)
				if lexer.Lookahead() == '-' {
					cryAdvance(lexer)
					quoted := false
					gotEndQuote := false

					if lexer.Lookahead() == '\'' {
						quoted = true
						cryAdvance(lexer)
					}

					maxWordSize := cryHeredocBufferSize - cryHeredocCurrentBufferSize(s)
					if maxWordSize < 1 {
						return false
					}
					if maxWordSize > cryMaxHeredocWordSize {
						maxWordSize = cryMaxHeredocWordSize
					}

					var word []byte
					wordBuf := [cryHeredocBufferSize + 4]byte{}

					// First character must be valid in an identifier
					if cryIsIdentPart(lexer.Lookahead()) {
						n := cryCodepointToUTF8(lexer.Lookahead(), wordBuf[:])
						if n == 0 {
							return false
						}
						word = append(word, wordBuf[:n]...)
						cryAdvance(lexer)
					} else {
						return false
					}

					for len(word) <= maxWordSize {
						la := lexer.Lookahead()
						if la == '\r' || la == '\n' || cryIsEOF(lexer) {
							break
						}
						if la == '\'' && quoted {
							gotEndQuote = true
							cryAdvance(lexer)
							break
						}
						if quoted || cryIsIdentPart(la) {
							n := cryCodepointToUTF8(la, wordBuf[:])
							if n == 0 {
								return false
							}
							word = append(word, wordBuf[:n]...)
							cryAdvance(lexer)
						} else {
							break
						}
					}

					if len(word) == 0 {
						return false
					} else if len(word) > maxWordSize || (len(word) == maxWordSize && cryIsIdentPart(lexer.Lookahead())) {
						return false
					} else if quoted && !gotEndQuote {
						return false
					}

					// Store the heredoc
					hd := cryHeredoc{
						allowEscapes: !quoted,
						started:      false,
						identifier:   make([]byte, len(word)),
					}
					copy(hd.identifier, word)

					if !cryHasRoomForHeredoc(s, len(hd.identifier)) {
						return false
					}

					cryPushHeredoc(s, hd)
					crySetResult(lexer, cryTokHeredocStart)
					return true
				}
			}
		}

	case '+':
		if cryIsValid(validSymbols, cryTokUnaryPlus) || cryIsValid(validSymbols, cryTokBinaryPlus) {
			cryAdvance(lexer)
			if lexer.Lookahead() == '=' {
				return false
			}

			unaryPriority := (s.hasLeadingWhitespace && !unicode.IsSpace(lexer.Lookahead())) ||
				cryIsValid(validSymbols, cryTokEndOfRange)

			if cryIsValid(validSymbols, cryTokUnaryPlus) && unaryPriority {
				crySetResult(lexer, cryTokUnaryPlus)
			} else if cryIsValid(validSymbols, cryTokBinaryPlus) {
				crySetResult(lexer, cryTokBinaryPlus)
			} else {
				crySetResult(lexer, cryTokUnaryPlus)
			}
			return true
		}

	case '-':
		if cryIsValid(validSymbols, cryTokUnaryMinus) || cryIsValid(validSymbols, cryTokBinaryMinus) {
			cryAdvance(lexer)
			if lexer.Lookahead() == '=' || lexer.Lookahead() == '>' {
				return false
			}

			unaryPriority := (s.hasLeadingWhitespace && !unicode.IsSpace(lexer.Lookahead())) ||
				cryIsValid(validSymbols, cryTokEndOfRange)

			if cryIsValid(validSymbols, cryTokUnaryMinus) && unaryPriority {
				crySetResult(lexer, cryTokUnaryMinus)
			} else if cryIsValid(validSymbols, cryTokBinaryMinus) {
				crySetResult(lexer, cryTokBinaryMinus)
			} else {
				crySetResult(lexer, cryTokUnaryMinus)
			}
			return true
		}

	case '*':
		if cryIsValid(validSymbols, cryTokPointerStar) || cryIsValid(validSymbols, cryTokUnaryStar) ||
			cryIsValid(validSymbols, cryTokUnaryDoubleStar) ||
			cryIsValid(validSymbols, cryTokBinaryDoubleStar) {

			cryAdvance(lexer)

			if cryIsValid(validSymbols, cryTokPointerStar) && !cryIsValid(validSymbols, cryTokErrorRecovery) {
				crySetResult(lexer, cryTokPointerStar)
				return true
			}

			if lexer.Lookahead() == '=' {
				return false
			}

			if lexer.Lookahead() == '*' {
				cryAdvance(lexer)
				if lexer.Lookahead() == '=' {
					return false
				}

				unaryPriority := s.hasLeadingWhitespace && !unicode.IsSpace(lexer.Lookahead())

				if cryIsValid(validSymbols, cryTokUnaryDoubleStar) && unaryPriority {
					crySetResult(lexer, cryTokUnaryDoubleStar)
					return true
				} else if cryIsValid(validSymbols, cryTokBinaryDoubleStar) {
					crySetResult(lexer, cryTokBinaryDoubleStar)
					return true
				} else if cryIsValid(validSymbols, cryTokUnaryDoubleStar) && !unicode.IsSpace(lexer.Lookahead()) {
					crySetResult(lexer, cryTokUnaryDoubleStar)
					return true
				}
				return false
			}

			unaryPriority := s.hasLeadingWhitespace && !unicode.IsSpace(lexer.Lookahead())

			if cryIsValid(validSymbols, cryTokUnaryStar) && unaryPriority {
				crySetResult(lexer, cryTokUnaryStar)
				return true
			} else if cryIsValid(validSymbols, cryTokUnaryStar) && !unicode.IsSpace(lexer.Lookahead()) {
				// A splat _cannot_ have whitespace after the *
				crySetResult(lexer, cryTokUnaryStar)
				return true
			}
		}

	case '&':
		if cryIsValid(validSymbols, cryTokUnaryWrappingPlus) ||
			cryIsValid(validSymbols, cryTokUnaryWrappingMinus) ||
			cryIsValid(validSymbols, cryTokBinaryWrappingPlus) ||
			cryIsValid(validSymbols, cryTokBinaryWrappingMinus) ||
			cryIsValid(validSymbols, cryTokBlockAmpersand) ||
			cryIsValid(validSymbols, cryTokBinaryAmpersand) {

			cryAdvance(lexer)
			lexer.MarkEnd()

			if lexer.Lookahead() == '+' {
				cryAdvance(lexer)
				if lexer.Lookahead() == '=' {
					return false
				}
				if cryIsValid(validSymbols, cryTokBinaryWrappingPlus) {
					lexer.MarkEnd()
					crySetResult(lexer, cryTokBinaryWrappingPlus)
					return true
				} else if cryIsValid(validSymbols, cryTokUnaryWrappingPlus) {
					lexer.MarkEnd()
					crySetResult(lexer, cryTokUnaryWrappingPlus)
					return true
				}
				return false
			}

			if lexer.Lookahead() == '-' {
				cryAdvance(lexer)
				if lexer.Lookahead() == '=' {
					return false
				}
				if lexer.Lookahead() == '>' {
					// '&->' case: always return just the '&'
					unaryPriority := s.hasLeadingWhitespace
					if unaryPriority && cryIsValid(validSymbols, cryTokBlockAmpersand) {
						crySetResult(lexer, cryTokBlockAmpersand)
						return true
					} else if cryIsValid(validSymbols, cryTokBinaryAmpersand) {
						crySetResult(lexer, cryTokBinaryAmpersand)
						return true
					} else if cryIsValid(validSymbols, cryTokBlockAmpersand) {
						crySetResult(lexer, cryTokBlockAmpersand)
						return true
					}
					return false
				}

				if cryIsValid(validSymbols, cryTokBinaryWrappingMinus) {
					lexer.MarkEnd()
					crySetResult(lexer, cryTokBinaryWrappingMinus)
					return true
				} else if cryIsValid(validSymbols, cryTokUnaryWrappingMinus) {
					lexer.MarkEnd()
					crySetResult(lexer, cryTokUnaryWrappingMinus)
					return true
				}
				return false
			}

			if lexer.Lookahead() == '*' || lexer.Lookahead() == '&' || lexer.Lookahead() == '=' {
				return false
			}

			if lexer.Lookahead() == '.' {
				if cryIsValid(validSymbols, cryTokBlockAmpersand) {
					crySetResult(lexer, cryTokBlockAmpersand)
					return true
				}
				return false
			}

			unaryPriority := s.hasLeadingWhitespace && !unicode.IsSpace(lexer.Lookahead())
			if unaryPriority && cryIsValid(validSymbols, cryTokBlockAmpersand) {
				crySetResult(lexer, cryTokBlockAmpersand)
				return true
			} else if cryIsValid(validSymbols, cryTokBinaryAmpersand) {
				crySetResult(lexer, cryTokBinaryAmpersand)
				return true
			} else if cryIsValid(validSymbols, cryTokBlockAmpersand) {
				crySetResult(lexer, cryTokBlockAmpersand)
				return true
			}
		}

	case '/':
		if cryIsValid(validSymbols, cryTokRegexStart) ||
			cryIsValid(validSymbols, cryTokBinarySlash) ||
			cryIsValid(validSymbols, cryTokBinaryDoubleSlash) {

			cryAdvance(lexer)

			if lexer.Lookahead() == '=' {
				if cryIsValid(validSymbols, cryTokRegexStart) || cryIsValid(validSymbols, cryTokBinarySlash) {
					if cryIsValid(validSymbols, cryTokRegexStart) && !cryIsValid(validSymbols, cryTokBinarySlash) {
						crySetResult(lexer, cryTokRegexStart)
						return true
					} else if cryIsValid(validSymbols, cryTokBinarySlash) && !cryIsValid(validSymbols, cryTokRegexStart) {
						return false
					} else {
						if cryIsValid(validSymbols, cryTokStartOfParenlessArgs) {
							return false
						}
						return false
					}
				}
				return false
			}

			if lexer.Lookahead() == '/' && cryIsValid(validSymbols, cryTokBinaryDoubleSlash) {
				cryAdvance(lexer)
				if lexer.Lookahead() == '=' {
					return false
				}
				crySetResult(lexer, cryTokBinaryDoubleSlash)
				return true
			}

			if cryIsValid(validSymbols, cryTokBinarySlash) && !cryIsValid(validSymbols, cryTokRegexStart) {
				crySetResult(lexer, cryTokBinarySlash)
				return true
			} else if cryIsValid(validSymbols, cryTokRegexStart) && !cryIsValid(validSymbols, cryTokBinarySlash) {
				crySetResult(lexer, cryTokRegexStart)
				return true
			} else {
				// Both are valid
				if cryIsValid(validSymbols, cryTokStartOfParenlessArgs) {
					if s.hasLeadingWhitespace &&
						!(lexer.Lookahead() == ' ' || lexer.Lookahead() == '\t' || lexer.Lookahead() == '\n' || lexer.Lookahead() == '\r') {
						crySetResult(lexer, cryTokRegexStart)
						return true
					}
					crySetResult(lexer, cryTokBinarySlash)
					return true
				} else if cryIsValid(validSymbols, cryTokEndOfRange) {
					crySetResult(lexer, cryTokRegexStart)
					return true
				}
			}
		}

	case '%':
		cryAdvance(lexer)

		// End of a macro expression
		if lexer.Lookahead() == '}' {
			return false
		}

		// `%=` is not an external token
		if lexer.Lookahead() == '=' {
			return false
		}

		if cryIsValid(validSymbols, cryTokStringPercentLiteralStart) ||
			cryIsValid(validSymbols, cryTokCommandPercentLiteralStart) ||
			cryIsValid(validSymbols, cryTokStringArrayPercentLiteralStart) ||
			cryIsValid(validSymbols, cryTokSymbolArrayPercentLiteralStart) ||
			cryIsValid(validSymbols, cryTokRegexPercentLiteralStart) {

			litType := byte(cryLitString)
			returnSymbol := cryTokStringPercentLiteralStart

			switch lexer.Lookahead() {
			case 'Q':
				cryAdvance(lexer)
				// type is already STRING
			case 'q':
				cryAdvance(lexer)
				litType = cryLitStringNoEsc
			case 'x':
				cryAdvance(lexer)
				litType = cryLitCommand
				returnSymbol = cryTokCommandPercentLiteralStart
			case 'w':
				cryAdvance(lexer)
				litType = cryLitStringArray
				returnSymbol = cryTokStringArrayPercentLiteralStart
			case 'i':
				cryAdvance(lexer)
				litType = cryLitSymbolArray
				returnSymbol = cryTokSymbolArrayPercentLiteralStart
			case 'r':
				cryAdvance(lexer)
				litType = cryLitRegex
				returnSymbol = cryTokRegexPercentLiteralStart
			}

			var openingChar, closingChar byte

			switch lexer.Lookahead() {
			case '{':
				openingChar = '{'
				closingChar = '}'
			case '(':
				openingChar = '('
				closingChar = ')'
			case '[':
				openingChar = '['
				closingChar = ']'
			case '<':
				openingChar = '<'
				closingChar = '>'
			case '|':
				openingChar = '|'
				closingChar = '|'
			default:
				if cryIsValid(validSymbols, cryTokModuloOperator) {
					crySetResult(lexer, cryTokModuloOperator)
					return true
				}
			}

			if openingChar != 0 {
				cryAdvance(lexer)

				if !cryIsValid(validSymbols, returnSymbol) {
					return false
				}

				crySetResult(lexer, returnSymbol)

				if len(s.literals) >= cryMaxLiteralCount {
					return false
				}

				cryPushLiteral(s, cryPercentLiteral{
					openingChar:  openingChar,
					closingChar:  closingChar,
					litType:      litType,
					nestingLevel: 0,
				})

				return true
			}

		} else if cryIsValid(validSymbols, cryTokModuloOperator) {
			crySetResult(lexer, cryTokModuloOperator)
			return true
		}

	case '"':
		if cryIsValid(validSymbols, cryTokStringLiteralStart) {
			cryAdvance(lexer)
			cryPushLiteral(s, cryPercentLiteral{
				openingChar:  '"',
				closingChar:  '"',
				litType:      cryLitString,
				nestingLevel: 0,
			})
			crySetResult(lexer, cryTokStringLiteralStart)
			return true
		} else if cryIsValid(validSymbols, cryTokStringLiteralEnd) {
			cryAdvance(lexer)
			crySetResult(lexer, cryTokStringLiteralEnd)
			return true
		}

	case '`':
		// The compiled grammar may not have command literal tokens, but handle
		// the backtick as string literal if the grammar supports it.
		// Since our grammar binary doesn't have separate COMMAND_LITERAL_START/END,
		// skip this case.

	case '\\':
		if cryIsValid(validSymbols, cryTokLineContinuation) {
			// Don't allow line continuation in a quoted heredoc
			if cryHasActiveHeredoc(s) && len(s.heredocs) > 0 && !s.heredocs[0].allowEscapes {
				return false
			}

			// Line continuations may be allowed in some literals
			if cryHasActiveLiteral(s) {
				switch cryActiveLiteral(s).litType {
				case cryLitStringNoEsc, cryLitRegex, cryLitStringArray, cryLitSymbolArray:
					return false
				case cryLitString, cryLitCommand:
					// Continue checking for line continuation
				}
			}

			cryAdvance(lexer)
			if lexer.Lookahead() == '\r' {
				cryAdvance(lexer)
			}
			if lexer.Lookahead() == '\n' {
				cryAdvance(lexer)
				crySetResult(lexer, cryTokLineContinuation)
				s.previousLineContinued = true
				return true
			}
		}

	case ':':
		// START_OF_SYMBOL and TYPE_FIELD_COLON are not in the binary grammar.
		// Skip this case.

	case '.':
		if cryIsValid(validSymbols, cryTokBeginlessRangeOperator) && !cryIsValid(validSymbols, cryTokStartOfParenlessArgs) {
			cryAdvance(lexer)
			if lexer.Lookahead() != '.' {
				return false
			}
			cryAdvance(lexer)
			if lexer.Lookahead() == '.' {
				cryAdvance(lexer)
			}
			crySetResult(lexer, cryTokBeginlessRangeOperator)
			return true
		}

	case 'e':
		if cryIsValid(validSymbols, cryTokRegularEnsureKeyword) || cryIsValid(validSymbols, cryTokModifierEnsureKeyword) {
			cryAdvance(lexer)
			if lexer.Lookahead() != 'n' {
				return false
			}
			cryAdvance(lexer)
			if lexer.Lookahead() != 's' {
				return false
			}
			cryAdvance(lexer)
			if lexer.Lookahead() != 'u' {
				return false
			}
			cryAdvance(lexer)
			if lexer.Lookahead() != 'r' {
				return false
			}
			cryAdvance(lexer)
			if lexer.Lookahead() != 'e' {
				return false
			}
			cryAdvance(lexer)
			if cryNextCharIsIdentifier(lexer) {
				return false
			}

			if cryIsValid(validSymbols, cryTokModifierEnsureKeyword) && !cryIsValid(validSymbols, cryTokRegularEnsureKeyword) {
				crySetResult(lexer, cryTokModifierEnsureKeyword)
				return true
			} else if cryIsValid(validSymbols, cryTokRegularEnsureKeyword) && !cryIsValid(validSymbols, cryTokModifierEnsureKeyword) {
				crySetResult(lexer, cryTokRegularEnsureKeyword)
				return true
			} else {
				crySetResult(lexer, cryTokModifierEnsureKeyword)
				return true
			}
		}

	case 'i':
		if cryIsValid(validSymbols, cryTokRegularIfKeyword) || cryIsValid(validSymbols, cryTokModifierIfKeyword) {
			cryAdvance(lexer)
			if lexer.Lookahead() != 'f' {
				return false
			}
			cryAdvance(lexer)
			if cryNextCharIsIdentifier(lexer) {
				return false
			}

			if cryIsValid(validSymbols, cryTokModifierIfKeyword) && !cryIsValid(validSymbols, cryTokRegularIfKeyword) {
				crySetResult(lexer, cryTokModifierIfKeyword)
				return true
			} else if cryIsValid(validSymbols, cryTokRegularIfKeyword) && !cryIsValid(validSymbols, cryTokModifierIfKeyword) {
				crySetResult(lexer, cryTokRegularIfKeyword)
				return true
			} else {
				crySetResult(lexer, cryTokModifierIfKeyword)
				return true
			}
		}

	case 'r':
		if cryIsValid(validSymbols, cryTokRegularRescueKeyword) || cryIsValid(validSymbols, cryTokModifierRescueKeyword) {
			cryAdvance(lexer)
			if lexer.Lookahead() != 'e' {
				return false
			}
			cryAdvance(lexer)
			if lexer.Lookahead() != 's' {
				return false
			}
			cryAdvance(lexer)
			if lexer.Lookahead() != 'c' {
				return false
			}
			cryAdvance(lexer)
			if lexer.Lookahead() != 'u' {
				return false
			}
			cryAdvance(lexer)
			if lexer.Lookahead() != 'e' {
				return false
			}
			cryAdvance(lexer)
			if cryNextCharIsIdentifier(lexer) {
				return false
			}

			if cryIsValid(validSymbols, cryTokModifierRescueKeyword) && !cryIsValid(validSymbols, cryTokRegularRescueKeyword) {
				crySetResult(lexer, cryTokModifierRescueKeyword)
				return true
			} else if cryIsValid(validSymbols, cryTokRegularRescueKeyword) && !cryIsValid(validSymbols, cryTokModifierRescueKeyword) {
				crySetResult(lexer, cryTokRegularRescueKeyword)
				return true
			} else {
				crySetResult(lexer, cryTokModifierRescueKeyword)
				return true
			}
		}

	case 'u':
		if cryIsValid(validSymbols, cryTokRegularUnlessKeyword) || cryIsValid(validSymbols, cryTokModifierUnlessKeyword) {
			cryAdvance(lexer)
			if lexer.Lookahead() != 'n' {
				return false
			}
			cryAdvance(lexer)
			if lexer.Lookahead() != 'l' {
				return false
			}
			cryAdvance(lexer)
			if lexer.Lookahead() != 'e' {
				return false
			}
			cryAdvance(lexer)
			if lexer.Lookahead() != 's' {
				return false
			}
			cryAdvance(lexer)
			if lexer.Lookahead() != 's' {
				return false
			}
			cryAdvance(lexer)
			if cryNextCharIsIdentifier(lexer) {
				return false
			}

			if cryIsValid(validSymbols, cryTokModifierUnlessKeyword) && !cryIsValid(validSymbols, cryTokRegularUnlessKeyword) {
				crySetResult(lexer, cryTokModifierUnlessKeyword)
				return true
			} else if cryIsValid(validSymbols, cryTokRegularUnlessKeyword) && !cryIsValid(validSymbols, cryTokModifierUnlessKeyword) {
				crySetResult(lexer, cryTokRegularUnlessKeyword)
				return true
			} else {
				crySetResult(lexer, cryTokModifierUnlessKeyword)
				return true
			}
		}

	case 'y':
		if cryIsValid(validSymbols, cryTokEndOfWithExpression) {
			// We don't want to consume the yield keyword
			lexer.MarkEnd()

			cryAdvance(lexer)
			if lexer.Lookahead() != 'i' {
				return false
			}
			cryAdvance(lexer)
			if lexer.Lookahead() != 'e' {
				return false
			}
			cryAdvance(lexer)
			if lexer.Lookahead() != 'l' {
				return false
			}
			cryAdvance(lexer)
			if lexer.Lookahead() != 'd' {
				return false
			}
			cryAdvance(lexer)
			if cryNextCharIsIdentifier(lexer) {
				return false
			}

			crySetResult(lexer, cryTokEndOfWithExpression)
			return true
		}

	case '}':
		cryAdvance(lexer)
		// End of a macro expression
		if lexer.Lookahead() == '}' {
			return false
		}
	}

	return false
}
