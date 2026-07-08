//go:build !grammar_subset || grammar_subset_typst

package grammars

import (
	"unicode"

	gotreesitter "github.com/odvcencio/gotreesitter"
)

// External token indexes for the Typst grammar.
// Must match the order of external symbols in the generated Typst grammar.
const (
	typstTokIndent         = 0  // _indent
	typstTokDedent         = 1  // _dedent
	typstTokRedent         = 2  // _redent
	typstTokLineStartCheck = 3  // _line_start_check
	typstTokContent        = 4  // [ (content block open)
	typstTokStrong         = 5  // * (strong markup open)
	typstTokEmph           = 6  // _ (emph markup open)
	typstTokBarrier        = 7  // _barrier
	typstTokBracket        = 8  // text (bracket)
	typstTokSection        = 9  // _token_section
	typstTokTermination    = 10 // _termination
	typstTokInlinedItemEnd = 11 // end (inlined item end)
	typstTokInlinedStmtEnd = 12 // end (inlined stmt end)
	typstTokBlockedExprEnd = 13 // sep (blocked expr end)
	typstTokMathLetter     = 14 // letter
	typstTokMathIdent      = 15 // ident
	typstTokMathFrac       = 16 // /
	typstTokMathGroupEnd   = 17 // )
	typstTokElse           = 18 // else
	typstTokUnit           = 19 // _token_unit
	typstTokURL            = 20 // _token_url
	typstTokItem           = 21 // - (list item)
	typstTokTerm           = 22 // / (term)
	typstTokHead1          = 23 // =
	typstTokHead2          = 24 // ==
	typstTokHead3          = 25 // ===
	typstTokHead4          = 26 // ====
	typstTokHead5          = 27 // =====
	typstTokHeadP          = 28 // ====== (6+)
	typstTokStringBlob     = 29 // _token_string_blob
	typstTokRawSpanBlob    = 30 // blob (raw span)
	typstTokRawBlckLdlm    = 31 // ``` (raw block left delimiter)
	typstTokRawBlckRdlm    = 32 // ``` (raw block right delimiter)
	typstTokRawBlckBlob    = 33 // blob (raw block)
	typstTokRawLang        = 34 // ident (raw language)
	typstTokIdentifier     = 35 // _token_identifier
	typstTokLabel          = 36 // _token_label
	typstTokAntiMarkup     = 37 // _token_anti_markup
	typstTokComment        = 38 // comment
	typstTokSpace          = 39 // _sp
	typstTokImmediateSet   = 40 // _immediate
	typstTokImmediateParen = 41 // _immediate_paren
	typstTokImmediateBrack = 42 // _immediate_brack
	typstTokImmediateIdent = 43 // _immediate_ident
	typstTokImmMathCall    = 44 // _immediate_math_call
	typstTokImmMathApply   = 45 // _immediate_math_apply
	typstTokImmMathField   = 46 // _immediate_math_field
	typstTokImmMathPrime   = 47 // _immediate_math_prime
	typstTokRecovery       = 48 // _recovery
)

// Concrete symbol IDs from the generated Typst grammar ExternalSymbols.
const (
	typstSymIndent         gotreesitter.Symbol = 74
	typstSymDedent         gotreesitter.Symbol = 75
	typstSymRedent         gotreesitter.Symbol = 76
	typstSymLineStartCheck gotreesitter.Symbol = 77
	typstSymContent        gotreesitter.Symbol = 78
	typstSymStrong         gotreesitter.Symbol = 79
	typstSymEmph           gotreesitter.Symbol = 80
	typstSymBarrier        gotreesitter.Symbol = 81
	typstSymBracket        gotreesitter.Symbol = 82
	typstSymSection        gotreesitter.Symbol = 83
	typstSymTermination    gotreesitter.Symbol = 84
	typstSymInlinedItemEnd gotreesitter.Symbol = 85
	typstSymInlinedStmtEnd gotreesitter.Symbol = 86
	typstSymBlockedExprEnd gotreesitter.Symbol = 87
	typstSymMathLetter     gotreesitter.Symbol = 88
	typstSymMathIdent      gotreesitter.Symbol = 89
	typstSymMathFrac       gotreesitter.Symbol = 90
	typstSymMathGroupEnd   gotreesitter.Symbol = 91
	typstSymElse           gotreesitter.Symbol = 92
	typstSymUnit           gotreesitter.Symbol = 93
	typstSymURL            gotreesitter.Symbol = 94
	typstSymItem           gotreesitter.Symbol = 95
	typstSymTerm           gotreesitter.Symbol = 96
	typstSymHead1          gotreesitter.Symbol = 97
	typstSymHead2          gotreesitter.Symbol = 98
	typstSymHead3          gotreesitter.Symbol = 99
	typstSymHead4          gotreesitter.Symbol = 100
	typstSymHead5          gotreesitter.Symbol = 101
	typstSymHeadP          gotreesitter.Symbol = 102
	typstSymStringBlob     gotreesitter.Symbol = 103
	typstSymRawSpanBlob    gotreesitter.Symbol = 104
	typstSymRawBlckLdlm    gotreesitter.Symbol = 105
	typstSymRawBlckRdlm    gotreesitter.Symbol = 106
	typstSymRawBlckBlob    gotreesitter.Symbol = 107
	typstSymRawLang        gotreesitter.Symbol = 108
	typstSymIdentifier     gotreesitter.Symbol = 109
	typstSymLabel          gotreesitter.Symbol = 110
	typstSymAntiMarkup     gotreesitter.Symbol = 111
	typstSymComment        gotreesitter.Symbol = 112
	typstSymSpace          gotreesitter.Symbol = 113
	typstSymImmediateSet   gotreesitter.Symbol = 114
	typstSymImmediateParen gotreesitter.Symbol = 115
	typstSymImmediateBrack gotreesitter.Symbol = 116
	typstSymImmediateIdent gotreesitter.Symbol = 117
	typstSymImmMathCall    gotreesitter.Symbol = 118
	typstSymImmMathApply   gotreesitter.Symbol = 119
	typstSymImmMathField   gotreesitter.Symbol = 120
	typstSymImmMathPrime   gotreesitter.Symbol = 121
	typstSymRecovery       gotreesitter.Symbol = 122
)

// Container types for the Typst scanner's container stack.
const (
	typstContainerContent = 0
	typstContainerStrong  = 1
	typstContainerEmph    = 2
	typstContainerBarrier = 3
	typstContainerBracket = 4
	typstContainerSection = 5
	// Any value >= typstContainerSection is a section; the difference from
	// typstContainerSection is the heading level.
)

// Termination result from scanner_termination.
const (
	typstTermNone      = 0
	typstTermInclusive = 1
	typstTermExclusive = 2
)

// typstScannerState holds scanner state between tokens.
type typstScannerState struct {
	indentation  []uint32
	containers   []uint32
	worker       []uint32 // transient: used during URL parsing only
	immediate    bool
	headingLevel uint8
	lineStart    bool
	rawLevel     uint8
}

func (s *typstScannerState) redent(col uint32) {
	if len(s.indentation) == 0 {
		return
	}
	s.indentation[len(s.indentation)-1] = col
}

func (s *typstScannerState) dedent() {
	if len(s.indentation) == 0 {
		return
	}
	s.indentation = s.indentation[:len(s.indentation)-1]
}

func (s *typstScannerState) indent(col uint32) {
	s.indentation = append(s.indentation, col)
}

func (s *typstScannerState) containerAt(at int) uint32 {
	idx := len(s.containers) - 1 - at
	if idx < 0 || idx >= len(s.containers) {
		return 0
	}
	return s.containers[idx]
}

func (s *typstScannerState) containerPush(c uint32) {
	s.containers = append(s.containers, c)
}

func (s *typstScannerState) containerPop() {
	if len(s.containers) == 0 {
		return
	}
	s.containers = s.containers[:len(s.containers)-1]
}

func (s *typstScannerState) termination(lexer *gotreesitter.ExternalLexer, valid []bool, at int) int {
	if len(s.containers) == at {
		// no container
		if lexer.Lookahead() == 0 {
			return typstTermExclusive
		}
		return typstTermNone
	}
	container := s.containerAt(at)

	// For non-bracket and non-content containers, ] is exclusive termination.
	switch container {
	case typstContainerBracket, typstContainerContent:
		// no early ] check
	default:
		if lexer.Lookahead() == ']' {
			return typstTermExclusive
		}
	}

	switch container {
	case typstContainerBracket:
		if lexer.Lookahead() == 0 {
			return typstTermExclusive
		}
		if lexer.Lookahead() == ']' {
			return typstTermInclusive
		}
		if len(s.containers) > 1 && s.termination(lexer, valid, at+1) != typstTermNone {
			return typstTermExclusive
		}
		return typstTermNone

	case typstContainerContent:
		if lexer.Lookahead() == ']' {
			return typstTermInclusive
		}
		return typstTermNone

	case typstContainerStrong:
		if lexer.Lookahead() == '*' {
			return typstTermInclusive
		}
		return typstTermNone

	case typstContainerEmph:
		if lexer.Lookahead() == '_' {
			return typstTermInclusive
		}
		return typstTermNone

	case typstContainerBarrier:
		if lexer.Lookahead() == ']' {
			return typstTermExclusive
		}
		if typstIsLB(lexer.Lookahead()) {
			return typstTermExclusive
		}
		if lexer.Lookahead() == 0 {
			return typstTermExclusive
		}
		if len(s.containers) > 1+at {
			parentIdx := len(s.containers) - 2 - at
			if parentIdx >= 0 && parentIdx < len(s.containers) {
				parent := s.containers[parentIdx]
				switch parent {
				case typstContainerEmph, typstContainerStrong:
					// inside emph or strong, a new line is mandatory
					return typstTermNone
				case typstContainerBarrier:
					return typstTermNone
				case typstContainerContent:
					if lexer.Lookahead() == ']' {
						return typstTermExclusive
					}
					return typstTermNone
				}
			}
		}
		return typstTermNone

	default:
		// CONTAINER_SECTION or above
		if len(s.containers) > 1 && s.termination(lexer, valid, at+1) != typstTermNone {
			return typstTermExclusive
		}
		if lexer.Lookahead() == 0 {
			return typstTermExclusive
		}
		if lexer.Lookahead() == ']' {
			return typstTermExclusive
		}
		return typstTermNone
	}
}

// TypstExternalScanner implements gotreesitter.ExternalScanner for tree-sitter-typst.
//
// This is a Go port of the C external scanner from tree-sitter-typst
// (https://github.com/Enter-tainer/tree-sitter-typst). The scanner handles 49
// external tokens including indentation tracking, content/strong/emph containers,
// raw blocks, comments, headings, list items, math mode, and more.
type TypstExternalScanner struct{}

func (TypstExternalScanner) Create() any {
	return &typstScannerState{}
}

func (TypstExternalScanner) Destroy(payload any) {}

func (TypstExternalScanner) Serialize(payload any, buf []byte) int {
	s := payload.(*typstScannerState)
	written := 0

	// Serialize indentation vec: 8 bytes for length, then 4 bytes per entry
	n := typstSerializeVecU32(s.indentation, buf[written:])
	written += n

	// Serialize containers vec
	n = typstSerializeVecU32(s.containers, buf[written:])
	written += n

	// Serialize scalar fields (4 bytes)
	if written+4 > len(buf) {
		return written
	}
	if s.immediate {
		buf[written] = 1
	} else {
		buf[written] = 0
	}
	written++
	buf[written] = s.headingLevel
	written++
	if s.lineStart {
		buf[written] = 1
	} else {
		buf[written] = 0
	}
	written++
	buf[written] = s.rawLevel
	written++

	return written
}

func (TypstExternalScanner) Deserialize(payload any, buf []byte) {
	s := payload.(*typstScannerState)
	s.indentation = s.indentation[:0]
	s.containers = s.containers[:0]
	s.worker = s.worker[:0]
	s.immediate = false
	s.headingLevel = 0
	s.lineStart = false
	s.rawLevel = 0

	if len(buf) == 0 {
		s.indentation = append(s.indentation, 0)
		return
	}

	read := 0
	var n int

	s.indentation, n = typstDeserializeVecU32(buf[read:])
	read += n

	if read < len(buf) {
		s.containers, n = typstDeserializeVecU32(buf[read:])
		read += n
	}

	if read+4 <= len(buf) {
		s.immediate = buf[read] != 0
		read++
		s.headingLevel = buf[read]
		read++
		s.lineStart = buf[read] != 0
		read++
		s.rawLevel = buf[read]
		read++
	}
}

// typstSerializeVecU32 writes a vec<u32> to buf using the same format as the C scanner:
// size_t (8 bytes on 64-bit, but we use a fixed 8-byte size_t for compat) followed by
// len * 4 bytes of uint32 data.
//
// Actually the C scanner uses memcpy of size_t which is platform-dependent.
// For our Go implementation we use a simpler format: 4-byte length + data.
func typstSerializeVecU32(vec []uint32, buf []byte) int {
	// We need at least 4 bytes for the length.
	if len(buf) < 4 {
		return 0
	}
	l := uint32(len(vec))
	buf[0] = byte(l)
	buf[1] = byte(l >> 8)
	buf[2] = byte(l >> 16)
	buf[3] = byte(l >> 24)
	written := 4

	for i := 0; i < int(l); i++ {
		if written+4 > len(buf) {
			break
		}
		v := vec[i]
		buf[written] = byte(v)
		buf[written+1] = byte(v >> 8)
		buf[written+2] = byte(v >> 16)
		buf[written+3] = byte(v >> 24)
		written += 4
	}
	return written
}

func typstDeserializeVecU32(buf []byte) ([]uint32, int) {
	if len(buf) < 4 {
		return nil, 0
	}
	l := uint32(buf[0]) | uint32(buf[1])<<8 | uint32(buf[2])<<16 | uint32(buf[3])<<24
	read := 4

	vec := make([]uint32, 0, l)
	for i := uint32(0); i < l; i++ {
		if read+4 > len(buf) {
			break
		}
		v := uint32(buf[read]) | uint32(buf[read+1])<<8 | uint32(buf[read+2])<<16 | uint32(buf[read+3])<<24
		vec = append(vec, v)
		read += 4
	}
	return vec, read
}

// ---------------------------------------------------------------------------
// Unicode helpers
// ---------------------------------------------------------------------------

// typstIsSP matches the C scanner's is_sp function: space, tab, and Unicode
// space characters (excluding line breaks).
func typstIsSP(c rune) bool {
	return c == ' ' || c == '\t' ||
		c == 0x1680 ||
		(c >= 0x2000 && c <= 0x200a) ||
		c == 0x202f ||
		c == 0x205f ||
		c == 0x3000
}

// typstIsLB matches the C scanner's is_lb function: newline and Unicode line
// break characters.
func typstIsLB(c rune) bool {
	return c == '\n' || c == '\r' || c == '\v' || c == '\f' ||
		c == 0x85 || c == 0x2028 || c == 0x2029
}

// typstIsIDStart approximates XID_Start. Uses Go's unicode.IsLetter which
// covers the vast majority of XID_Start code points.
func typstIsIDStart(c rune) bool {
	return unicode.IsLetter(c)
}

// typstIsIDContinue approximates XID_Continue.
func typstIsIDContinue(c rune) bool {
	return unicode.IsLetter(c) || unicode.IsDigit(c) ||
		unicode.Is(unicode.Mn, c) || // Nonspacing_Mark
		unicode.Is(unicode.Mc, c) || // Spacing_Mark
		unicode.Is(unicode.Pc, c) || // Connector_Punctuation (includes _)
		c == 0xB7 || c == 0x0387 // middle dot
}

// typstIsWordPart approximates the in_word table: "Alphanumeric except Han,
// Hiragana, Katakana, Hangul". We approximate with IsLetter || IsDigit since
// the anti-markup check is a heuristic.
func typstIsWordPart(c rune) bool {
	return unicode.IsLetter(c) || unicode.IsDigit(c)
}

// ---------------------------------------------------------------------------
// Helper
// ---------------------------------------------------------------------------

func typstValid(valid []bool, idx int) bool {
	return idx >= 0 && idx < len(valid) && valid[idx]
}

// ---------------------------------------------------------------------------
// Comment and space parsing
// ---------------------------------------------------------------------------

// typstParseComment attempts to parse a // or /* */ comment.
// Returns true if a comment token was produced.
func typstParseComment(s *typstScannerState, lexer *gotreesitter.ExternalLexer) bool {
	if lexer.Lookahead() != '/' {
		return false
	}
	lexer.Advance(false)

	if lexer.Lookahead() == '/' {
		// Line comment
		lexer.Advance(false)
		for lexer.Lookahead() != 0 && !typstIsLB(lexer.Lookahead()) {
			lexer.Advance(false)
		}
		s.immediate = false
		lexer.MarkEnd()
		lexer.SetResultSymbol(typstSymComment)
		return true
	}

	if lexer.Lookahead() == '*' {
		// Block comment (nestable)
		lexer.Advance(false)
		level := 0
		for lexer.Lookahead() != 0 {
			if lexer.Lookahead() == '*' {
				lexer.Advance(false)
				if lexer.Lookahead() == '/' {
					lexer.Advance(false)
					if level == 0 {
						break
					}
					level--
				}
			} else if lexer.Lookahead() == '/' {
				lexer.Advance(false)
				if lexer.Lookahead() == '*' {
					lexer.Advance(false)
					level++
				}
			} else {
				lexer.Advance(false)
			}
		}
		s.immediate = false
		lexer.MarkEnd()
		lexer.SetResultSymbol(typstSymComment)
		return true
	}

	return false
}

// typstParseSpace attempts to parse whitespace (non-line-break).
// Returns true if a space token was produced.
func typstParseSpace(s *typstScannerState, lexer *gotreesitter.ExternalLexer) bool {
	if !typstIsSP(lexer.Lookahead()) {
		return false
	}
	lexer.Advance(false)
	for typstIsSP(lexer.Lookahead()) {
		lexer.Advance(false)
	}
	s.immediate = false
	lexer.MarkEnd()
	lexer.SetResultSymbol(typstSymSpace)
	return true
}

// ---------------------------------------------------------------------------
// Scan
// ---------------------------------------------------------------------------

func (TypstExternalScanner) Scan(payload any, lexer *gotreesitter.ExternalLexer, validSymbols []bool) bool {
	s := payload.(*typstScannerState)
	lexer.MarkEnd()

	if typstValid(validSymbols, typstTokRecovery) {
		// The external scanner doesn't try any recovery.
		lexer.SetResultSymbol(typstSymRecovery)
		return true
	}

	// IMMEDIATE_SET must be before SPACE and COMMENT
	if typstValid(validSymbols, typstTokImmediateSet) {
		s.immediate = true
		lexer.SetResultSymbol(typstSymImmediateSet)
		return true
	}

	// IDENTIFIER
	if typstValid(validSymbols, typstTokIdentifier) && (typstIsIDStart(lexer.Lookahead()) || lexer.Lookahead() == '_') {
		lexer.Advance(false)
		for typstIsIDContinue(lexer.Lookahead()) || lexer.Lookahead() == '-' {
			lexer.Advance(false)
		}
		s.immediate = true
		lexer.MarkEnd()
		lexer.SetResultSymbol(typstSymIdentifier)
		return true
	}

	// LABEL
	if typstValid(validSymbols, typstTokLabel) && (typstIsIDContinue(lexer.Lookahead()) || lexer.Lookahead() == '-') {
		lexer.Advance(false)
		lexer.MarkEnd()
		for typstIsIDContinue(lexer.Lookahead()) || lexer.Lookahead() == '-' || lexer.Lookahead() == '.' || lexer.Lookahead() == ':' {
			upTo := lexer.Lookahead() != '.' && lexer.Lookahead() != ':'
			lexer.Advance(false)
			if upTo {
				lexer.MarkEnd()
			}
		}
		s.immediate = true
		lexer.SetResultSymbol(typstSymLabel)
		return true
	}

	// RAW_SPAN_BLOB
	if typstValid(validSymbols, typstTokRawSpanBlob) {
		for lexer.Lookahead() != '`' && lexer.Lookahead() != 0 {
			lexer.Advance(false)
		}
		lexer.MarkEnd()
		lexer.SetResultSymbol(typstSymRawSpanBlob)
		return true
	}

	// RAW_LANG
	if typstValid(validSymbols, typstTokRawLang) && (lexer.Lookahead() == '_' || typstIsIDStart(lexer.Lookahead())) {
		lexer.Advance(false)
		for lexer.Lookahead() == '-' || typstIsIDContinue(lexer.Lookahead()) {
			lexer.Advance(false)
		}
		lexer.MarkEnd()
		lexer.SetResultSymbol(typstSymRawLang)
		return true
	}

	// RAW_BLCK_LDLM (raw block left delimiter: ``` or more)
	if typstValid(validSymbols, typstTokRawBlckLdlm) && lexer.Lookahead() == '`' {
		lexer.Advance(false)
		if lexer.Lookahead() != '`' {
			return false
		}
		lexer.Advance(false)
		if lexer.Lookahead() != '`' {
			return false
		}
		lexer.Advance(false)
		level := uint8(3)
		for lexer.Lookahead() == '`' {
			lexer.Advance(false)
			level++
		}
		s.rawLevel = level
		lexer.MarkEnd()
		lexer.SetResultSymbol(typstSymRawBlckLdlm)
		return true
	}

	// RAW_BLCK_BLOB
	if typstValid(validSymbols, typstTokRawBlckBlob) {
		for lexer.Lookahead() != 0 {
			level := uint8(0)
			for lexer.Lookahead() == '`' {
				lexer.Advance(false)
				level++
				if level == s.rawLevel {
					lexer.SetResultSymbol(typstSymRawBlckBlob)
					return true
				}
			}
			lexer.Advance(false)
			lexer.MarkEnd()
		}
		return false
	}

	// RAW_BLCK_RDLM (raw block right delimiter)
	if typstValid(validSymbols, typstTokRawBlckRdlm) {
		for i := uint8(0); i < s.rawLevel; i++ {
			if lexer.Lookahead() != '`' {
				return false
			}
			lexer.Advance(false)
		}
		s.rawLevel = 0
		lexer.MarkEnd()
		lexer.SetResultSymbol(typstSymRawBlckRdlm)
		return true
	}

	// URL
	if typstValid(validSymbols, typstTokURL) {
		s.worker = s.worker[:0]
		const urlBrack = 0
		const urlParen = 1
		for {
			c := lexer.Lookahead()
			wLen := len(s.worker)
			if (c >= 'a' && c <= 'z') ||
				(c >= 'A' && c <= 'Z') ||
				(c >= '0' && c <= '9') ||
				c == '!' || c == '#' || c == '$' || c == '%' ||
				c == '&' || c == '*' || c == '+' || c == ',' ||
				c == '-' || c == '.' || c == '/' || c == ':' ||
				c == ';' || c == '=' || c == '?' || c == '@' ||
				c == '_' || c == '~' || c == '\'' {
				lexer.Advance(false)
				continue
			} else if c == '[' {
				s.worker = append(s.worker, urlBrack)
				lexer.Advance(false)
				continue
			} else if c == '(' {
				s.worker = append(s.worker, urlParen)
				lexer.Advance(false)
				continue
			} else if c == ']' && wLen > 0 && s.worker[wLen-1] == urlBrack {
				s.worker = s.worker[:wLen-1]
				lexer.Advance(false)
				continue
			} else if c == ')' && wLen > 0 && s.worker[wLen-1] == urlParen {
				s.worker = s.worker[:wLen-1]
				lexer.Advance(false)
				continue
			} else {
				lexer.MarkEnd()
				lexer.SetResultSymbol(typstSymURL)
				return true
			}
		}
	}

	// TERMINATION: end of strong, emph, content, item line, term line, heading line
	if typstValid(validSymbols, typstTokTermination) {
		term := s.termination(lexer, validSymbols, 0)
		switch term {
		case typstTermInclusive:
			lexer.Advance(false)
			lexer.MarkEnd()
			if s.containerAt(0) != typstContainerBracket {
				s.dedent()
			}
			s.containerPop()
			lexer.SetResultSymbol(typstSymTermination)
			return true
		case typstTermExclusive:
			s.containerPop()
			lexer.SetResultSymbol(typstSymTermination)
			return true
		}
	}

	// STRING_BLOB (must be before SPACE and COMMENT)
	if typstValid(validSymbols, typstTokStringBlob) {
		if lexer.Lookahead() != 0 && lexer.Lookahead() != '\\' && lexer.Lookahead() != '"' {
			lexer.Advance(false)
			lexer.MarkEnd()
			lexer.SetResultSymbol(typstSymStringBlob)
			return true
		}
	}

	// UNIT (suffix to number literal)
	if typstValid(validSymbols, typstTokUnit) && s.immediate {
		if lexer.Lookahead() == '%' {
			lexer.Advance(false)
			lexer.MarkEnd()
			lexer.SetResultSymbol(typstSymUnit)
			return true
		}
		if lexer.Lookahead() >= 'a' && lexer.Lookahead() <= 'z' {
			lexer.Advance(false)
			for lexer.Lookahead() >= 'a' && lexer.Lookahead() <= 'z' {
				lexer.Advance(false)
			}
			lexer.MarkEnd()
			lexer.SetResultSymbol(typstSymUnit)
			return true
		}
	}

	// INLINED_ITEM_END (complex multi-branch token)
	if typstValid(validSymbols, typstTokInlinedItemEnd) {
		if s.immediate {
			if typstValid(validSymbols, typstTokImmediateBrack) && lexer.Lookahead() == '[' {
				lexer.MarkEnd()
				lexer.SetResultSymbol(typstSymImmediateBrack)
				return true
			}
			if typstValid(validSymbols, typstTokImmediateParen) && lexer.Lookahead() == '(' {
				lexer.MarkEnd()
				lexer.SetResultSymbol(typstSymImmediateParen)
				return true
			}
		}
		if typstValid(validSymbols, typstTokElse) {
			if typstParseSpace(s, lexer) {
				return true
			}
			if typstParseComment(s, lexer) {
				return true
			}
			if lexer.Lookahead() != 'e' {
				lexer.SetResultSymbol(typstSymInlinedItemEnd)
				return true
			}
			lexer.Advance(false)
			if lexer.Lookahead() != 'l' {
				lexer.SetResultSymbol(typstSymInlinedItemEnd)
				return true
			}
			lexer.Advance(false)
			if lexer.Lookahead() != 's' {
				lexer.SetResultSymbol(typstSymInlinedItemEnd)
				return true
			}
			lexer.Advance(false)
			if lexer.Lookahead() != 'e' {
				lexer.SetResultSymbol(typstSymInlinedItemEnd)
				return true
			}
			lexer.Advance(false)
			if !typstIsIDContinue(lexer.Lookahead()) && lexer.Lookahead() != '-' {
				lexer.MarkEnd()
				lexer.SetResultSymbol(typstSymElse)
				return true
			}
			lexer.SetResultSymbol(typstSymInlinedItemEnd)
			return true
		}

		if lexer.Lookahead() == '.' {
			lexer.Advance(false)
			if lexer.Lookahead() == '_' {
				// Test 361 and 362
				lexer.Advance(false)
				if typstIsIDContinue(lexer.Lookahead()) || lexer.Lookahead() == '-' {
					return false
				}
			} else if typstIsIDStart(lexer.Lookahead()) {
				return false
			}
			lexer.SetResultSymbol(typstSymInlinedItemEnd)
			return true
		}

		if lexer.Lookahead() == ';' {
			lexer.Advance(false)
			lexer.MarkEnd()
		}

		lexer.SetResultSymbol(typstSymInlinedItemEnd)
		return true
	}

	// SPACE
	if typstParseSpace(s, lexer) {
		return true
	}

	// COMMENT via '/' prefix
	if lexer.Lookahead() == '/' {
		column := lexer.Column()

		// COMMENT
		if typstParseComment(s, lexer) {
			return true
		}

		// MATH_FRAC
		if typstValid(validSymbols, typstTokMathFrac) {
			lexer.MarkEnd()
			lexer.SetResultSymbol(typstSymMathFrac)
			return true
		}

		if typstIsSP(lexer.Lookahead()) || typstIsLB(lexer.Lookahead()) {
			if typstValid(validSymbols, typstTokLineStartCheck) {
				lexer.SetResultSymbol(typstSymLineStartCheck)
				s.lineStart = true
				return true
			}
			if typstValid(validSymbols, typstTokTerm) && s.lineStart {
				s.lineStart = false
				s.redent(column)
				lexer.MarkEnd()
				lexer.SetResultSymbol(typstSymTerm)
				return true
			}
		}
	}

	// LINE_START_CHECK
	if typstValid(validSymbols, typstTokLineStartCheck) {
		if lexer.Lookahead() == '=' {
			for lexer.Lookahead() == '=' {
				lexer.Advance(false)
			}
			if typstIsSP(lexer.Lookahead()) || typstIsLB(lexer.Lookahead()) || lexer.Lookahead() == 0 {
				s.lineStart = true
			}
		} else if lexer.Lookahead() == '-' || lexer.Lookahead() == '+' {
			lexer.Advance(false)
			if typstIsSP(lexer.Lookahead()) || typstIsLB(lexer.Lookahead()) || lexer.Lookahead() == 0 {
				s.lineStart = true
			}
		} else if lexer.Lookahead() >= '0' && lexer.Lookahead() <= '9' {
			for lexer.Lookahead() >= '0' && lexer.Lookahead() <= '9' {
				lexer.Advance(false)
			}
			if lexer.Lookahead() == '.' {
				lexer.Advance(false)
				if typstIsSP(lexer.Lookahead()) || typstIsLB(lexer.Lookahead()) || lexer.Lookahead() == 0 {
					s.lineStart = true
				}
			}
		}
		lexer.SetResultSymbol(typstSymLineStartCheck)
		return true
	}

	// IMMEDIATE tokens (must be after SPACE and COMMENT)
	if s.immediate {
		if typstValid(validSymbols, typstTokImmediateBrack) && lexer.Lookahead() == '[' {
			lexer.MarkEnd()
			lexer.SetResultSymbol(typstSymImmediateBrack)
			return true
		}
		if typstValid(validSymbols, typstTokImmediateParen) && lexer.Lookahead() == '(' {
			lexer.MarkEnd()
			lexer.SetResultSymbol(typstSymImmediateParen)
			return true
		}
		if typstValid(validSymbols, typstTokImmediateIdent) && (typstIsIDStart(lexer.Lookahead()) || lexer.Lookahead() == '_') {
			lexer.MarkEnd()
			lexer.SetResultSymbol(typstSymImmediateIdent)
			return true
		}
		if typstValid(validSymbols, typstTokImmMathCall) && lexer.Lookahead() == '(' {
			lexer.MarkEnd()
			lexer.SetResultSymbol(typstSymImmMathCall)
			return true
		}
		if typstValid(validSymbols, typstTokImmMathApply) && (lexer.Lookahead() == '(' || lexer.Lookahead() == '[' || lexer.Lookahead() == '{') {
			lexer.MarkEnd()
			lexer.SetResultSymbol(typstSymImmMathApply)
			return true
		}
		if typstValid(validSymbols, typstTokImmMathPrime) && lexer.Lookahead() == '\'' {
			lexer.Advance(false)
			for lexer.Lookahead() == '\'' {
				lexer.Advance(false)
			}
			lexer.MarkEnd()
			lexer.SetResultSymbol(typstSymImmMathPrime)
			return true
		}
		if typstValid(validSymbols, typstTokImmMathField) && lexer.Lookahead() == '.' {
			lexer.Advance(false)
			if typstIsIDStart(lexer.Lookahead()) {
				lexer.MarkEnd()
				lexer.SetResultSymbol(typstSymImmMathField)
				return true
			}
			return false
		}
	}

	// SECTION
	if typstValid(validSymbols, typstTokSection) {
		s.containerPush(uint32(typstContainerSection) + uint32(s.headingLevel))
		lexer.MarkEnd()
		lexer.SetResultSymbol(typstSymSection)
		return true
	}

	// BARRIER
	if typstValid(validSymbols, typstTokBarrier) {
		s.containerPush(typstContainerBarrier)
		lexer.MarkEnd()
		lexer.SetResultSymbol(typstSymBarrier)
		return true
	}

	// CONTENT [
	if typstValid(validSymbols, typstTokContent) && lexer.Lookahead() == '[' {
		lexer.Advance(false)
		s.indent(0)
		s.containerPush(typstContainerContent)
		lexer.MarkEnd()
		lexer.SetResultSymbol(typstSymContent)
		return true
	}

	// STRONG *
	if typstValid(validSymbols, typstTokStrong) && lexer.Lookahead() == '*' {
		lexer.Advance(false)
		s.indent(0)
		s.containerPush(typstContainerStrong)
		lexer.MarkEnd()
		lexer.SetResultSymbol(typstSymStrong)
		return true
	}

	// EMPH _
	if typstValid(validSymbols, typstTokEmph) && lexer.Lookahead() == '_' {
		lexer.Advance(false)
		s.indent(0)
		s.containerPush(typstContainerEmph)
		lexer.MarkEnd()
		lexer.SetResultSymbol(typstSymEmph)
		return true
	}

	// BRACKET [
	if typstValid(validSymbols, typstTokBracket) && lexer.Lookahead() == '[' {
		lexer.Advance(false)
		s.containerPush(typstContainerBracket)
		lexer.MarkEnd()
		lexer.SetResultSymbol(typstSymBracket)
		return true
	}

	// HEADINGS (=, ==, ===, etc.)
	if s.lineStart && typstValid(validSymbols, typstTokHead1) && lexer.Lookahead() == '=' {
		lexer.Advance(false)
		count := 1
		for lexer.Lookahead() == '=' {
			lexer.Advance(false)
			count++
		}
		if typstIsSP(lexer.Lookahead()) || typstIsLB(lexer.Lookahead()) || lexer.Lookahead() == 0 {
			// Check if we need to terminate a section first.
			if len(s.containers) > 0 && s.containerAt(0) >= typstContainerSection {
				level := int(s.containerAt(0)) - typstContainerSection
				if count <= level {
					s.containerPop()
					lexer.SetResultSymbol(typstSymTermination)
					return true
				}
			}
			s.headingLevel = uint8(count)
			s.lineStart = false
			var sym gotreesitter.Symbol
			switch count {
			case 1:
				sym = typstSymHead1
			case 2:
				sym = typstSymHead2
			case 3:
				sym = typstSymHead3
			case 4:
				sym = typstSymHead4
			case 5:
				sym = typstSymHead5
			default:
				sym = typstSymHeadP
			}
			lexer.MarkEnd()
			lexer.SetResultSymbol(sym)
			return true
		}
		return false
	}

	// ITEM (- or + or 1.)
	if s.lineStart && typstValid(validSymbols, typstTokItem) {
		if lexer.Lookahead() == '-' || lexer.Lookahead() == '+' {
			column := lexer.Column()
			lexer.Advance(false)
			if typstIsSP(lexer.Lookahead()) || typstIsLB(lexer.Lookahead()) || lexer.Lookahead() == 0 {
				s.lineStart = false
				s.redent(column)
				lexer.MarkEnd()
				lexer.SetResultSymbol(typstSymItem)
				return true
			}
			return false
		}
		if lexer.Lookahead() >= '0' && lexer.Lookahead() <= '9' {
			column := lexer.Column()
			lexer.Advance(false)
			for lexer.Lookahead() >= '0' && lexer.Lookahead() <= '9' {
				lexer.Advance(false)
			}
			if lexer.Lookahead() == '.' {
				lexer.Advance(false)
				if typstIsSP(lexer.Lookahead()) || typstIsLB(lexer.Lookahead()) || lexer.Lookahead() == 0 {
					s.lineStart = false
					s.redent(column)
					lexer.MarkEnd()
					lexer.SetResultSymbol(typstSymItem)
					return true
				}
			}
			return false
		}
	}

	// ANTI_MARKUP
	if typstValid(validSymbols, typstTokAntiMarkup) && typstIsWordPart(lexer.Lookahead()) {
		lexer.Advance(false)
		if lexer.Lookahead() != '_' && lexer.Lookahead() != '*' {
			return false
		}
		lexer.Advance(false)
		if !typstIsWordPart(lexer.Lookahead()) {
			return false
		}
		lexer.Advance(false)
		lexer.MarkEnd()
		lexer.SetResultSymbol(typstSymAntiMarkup)
		return true
	}

	// MATH_IDENT / MATH_LETTER
	if typstValid(validSymbols, typstTokMathIdent) && typstIsIDStart(lexer.Lookahead()) {
		lexer.Advance(false)
		if typstValid(validSymbols, typstTokMathLetter) && (lexer.Lookahead() == '_' || !typstIsIDContinue(lexer.Lookahead())) {
			s.immediate = true
			lexer.MarkEnd()
			lexer.SetResultSymbol(typstSymMathLetter)
			return true
		}
		for lexer.Lookahead() != '_' && typstIsIDContinue(lexer.Lookahead()) {
			lexer.Advance(false)
		}
		s.immediate = true
		lexer.MarkEnd()
		lexer.SetResultSymbol(typstSymMathIdent)
		return true
	}

	// MATH_GROUP_END
	if typstValid(validSymbols, typstTokMathGroupEnd) {
		if lexer.Lookahead() == ')' || lexer.Lookahead() == ']' || lexer.Lookahead() == '}' {
			lexer.Advance(false)
			lexer.MarkEnd()
			lexer.SetResultSymbol(typstSymMathGroupEnd)
			return true
		}
		if lexer.Lookahead() == '$' {
			lexer.MarkEnd()
			lexer.SetResultSymbol(typstSymMathGroupEnd)
			return true
		}
		if lexer.Lookahead() == '|' {
			lexer.Advance(false)
			if lexer.Lookahead() != ']' {
				return false
			}
			lexer.Advance(false)
			lexer.MarkEnd()
			lexer.SetResultSymbol(typstSymMathGroupEnd)
			return true
		}
	}

	// ELSE (standalone, not in INLINED_ITEM_END context)
	if typstValid(validSymbols, typstTokElse) {
		if typstValid(validSymbols, typstTokBlockedExprEnd) {
			lexer.SetResultSymbol(typstSymBlockedExprEnd)
			for typstIsSP(lexer.Lookahead()) || typstIsLB(lexer.Lookahead()) {
				lexer.Advance(false)
			}
			if lexer.Lookahead() != 'e' {
				return true
			}
			lexer.Advance(false)
			if lexer.Lookahead() != 'l' {
				return true
			}
			lexer.Advance(false)
			if lexer.Lookahead() != 's' {
				return true
			}
			lexer.Advance(false)
			if lexer.Lookahead() != 'e' {
				return true
			}
			lexer.Advance(false)
			if lexer.Lookahead() == '[' || lexer.Lookahead() == '{' || lexer.Lookahead() == '/' || typstIsSP(lexer.Lookahead()) {
				lexer.MarkEnd()
				lexer.SetResultSymbol(typstSymElse)
				return true
			}
			return true
		}
		// standalone else
		if lexer.Lookahead() != 'e' {
			return false
		}
		lexer.Advance(false)
		if lexer.Lookahead() != 'l' {
			return false
		}
		lexer.Advance(false)
		if lexer.Lookahead() != 's' {
			return false
		}
		lexer.Advance(false)
		if lexer.Lookahead() != 'e' {
			return false
		}
		lexer.Advance(false)
		if lexer.Lookahead() == '[' || lexer.Lookahead() == '{' || lexer.Lookahead() == '/' || typstIsSP(lexer.Lookahead()) {
			lexer.MarkEnd()
			lexer.SetResultSymbol(typstSymElse)
			return true
		}
		return false
	}

	// BLOCKED_EXPR_END (without ELSE)
	if typstValid(validSymbols, typstTokBlockedExprEnd) {
		if lexer.Lookahead() == '}' {
			lexer.SetResultSymbol(typstSymBlockedExprEnd)
			return true
		}
		if lexer.Lookahead() == ';' {
			lexer.Advance(false)
			lexer.MarkEnd()
			lexer.SetResultSymbol(typstSymBlockedExprEnd)
			return true
		}
		if typstIsLB(lexer.Lookahead()) {
			lexer.MarkEnd()
			lexer.Advance(false)
			for typstIsLB(lexer.Lookahead()) || typstIsSP(lexer.Lookahead()) {
				lexer.Advance(false)
			}
			if lexer.Lookahead() == '.' {
				return false
			}
			lexer.SetResultSymbol(typstSymBlockedExprEnd)
			return true
		}
		return false
	}

	// INLINED_STMT_END
	if typstValid(validSymbols, typstTokInlinedStmtEnd) {
		for typstIsSP(lexer.Lookahead()) {
			lexer.Advance(false)
		}
		if lexer.Lookahead() == ';' {
			lexer.Advance(false)
			lexer.MarkEnd()
			lexer.SetResultSymbol(typstSymInlinedStmtEnd)
			return true
		}
		if lexer.Lookahead() == 0 || lexer.Lookahead() == ']' {
			lexer.SetResultSymbol(typstSymInlinedStmtEnd)
			return true
		}
		if typstIsLB(lexer.Lookahead()) {
			lexer.SetResultSymbol(typstSymInlinedStmtEnd)
			return true
		}
		return false
	}

	// INDENT / DEDENT
	if typstValid(validSymbols, typstTokIndent) || typstValid(validSymbols, typstTokDedent) {
		for typstIsLB(lexer.Lookahead()) || typstIsSP(lexer.Lookahead()) {
			lexer.Advance(false)
		}

		// when a container terminates
		if s.termination(lexer, validSymbols, 0) != typstTermNone {
			if typstValid(validSymbols, typstTokDedent) {
				s.dedent()
				lexer.SetResultSymbol(typstSymDedent)
				return true
			}
			return false
		}

		column := lexer.Column()
		if len(s.indentation) == 0 {
			return false
		}

		if lexer.Lookahead() == ']' {
			if typstValid(validSymbols, typstTokDedent) {
				s.dedent()
				lexer.SetResultSymbol(typstSymDedent)
				return true
			}
			return false
		}

		indentation := s.indentation[len(s.indentation)-1]

		// indent
		if column > indentation {
			if typstValid(validSymbols, typstTokIndent) {
				s.indent(column)
				lexer.SetResultSymbol(typstSymIndent)
				return true
			}
			return false
		}

		// dedent or redent
		if column < indentation {
			// redent check
			if len(s.indentation) > 1 && typstValid(validSymbols, typstTokRedent) {
				if column > s.indentation[len(s.indentation)-2] {
					s.redent(column)
					lexer.SetResultSymbol(typstSymRedent)
					return true
				}
			}
			// dedent
			if typstValid(validSymbols, typstTokDedent) {
				s.dedent()
				lexer.SetResultSymbol(typstSymDedent)
				return true
			}
			return false
		}

		return false
	}

	return false
}
