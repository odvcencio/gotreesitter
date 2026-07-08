//go:build !grammar_subset || grammar_subset_ruby

package grammars

import (
	"strings"
	"unicode"

	gotreesitter "github.com/odvcencio/gotreesitter"
)

// External token indexes for the Ruby grammar (order must match grammar.js externals).
const (
	rbyTokLineBreak                        = 0
	rbyTokNoLineBreak                      = 1
	rbyTokSimpleSymbol                     = 2
	rbyTokStringStart                      = 3
	rbyTokSymbolStart                      = 4
	rbyTokSubshellStart                    = 5
	rbyTokRegexStart                       = 6
	rbyTokStringArrayStart                 = 7
	rbyTokSymbolArrayStart                 = 8
	rbyTokHeredocBodyStart                 = 9
	rbyTokStringContent                    = 10
	rbyTokHeredocContent                   = 11
	rbyTokStringEnd                        = 12
	rbyTokHeredocBodyEnd                   = 13
	rbyTokHeredocStart                     = 14
	rbyTokForwardSlash                     = 15
	rbyTokBlockAmpersand                   = 16
	rbyTokSplatStar                        = 17
	rbyTokUnaryMinus                       = 18
	rbyTokUnaryMinusNum                    = 19
	rbyTokBinaryMinus                      = 20
	rbyTokBinaryStar                       = 21
	rbyTokSingletonClassLeftAngleLeftAngle = 22
	rbyTokHashKeySymbol                    = 23
	rbyTokIdentifierSuffix                 = 24
	rbyTokConstantSuffix                   = 25
	rbyTokHashSplatStarStar                = 26
	rbyTokBinaryStarStar                   = 27
	rbyTokElementReferenceBracket          = 28
	rbyTokShortInterpolation               = 29
	rbyTokNone                             = 30
)

// Concrete symbol IDs from the generated Ruby grammar ExternalSymbols.
const (
	rbySymLineBreak                        gotreesitter.Symbol = 128
	rbySymNoLineBreak                      gotreesitter.Symbol = 129
	rbySymSimpleSymbol                     gotreesitter.Symbol = 130
	rbySymStringStart                      gotreesitter.Symbol = 131
	rbySymSymbolStart                      gotreesitter.Symbol = 132
	rbySymSubshellStart                    gotreesitter.Symbol = 133
	rbySymRegexStart                       gotreesitter.Symbol = 134
	rbySymStringArrayStart                 gotreesitter.Symbol = 135
	rbySymSymbolArrayStart                 gotreesitter.Symbol = 136
	rbySymHeredocBodyStart                 gotreesitter.Symbol = 137
	rbySymStringContent                    gotreesitter.Symbol = 138
	rbySymHeredocContent                   gotreesitter.Symbol = 139
	rbySymStringEnd                        gotreesitter.Symbol = 140
	rbySymHeredocBodyEnd                   gotreesitter.Symbol = 141
	rbySymHeredocStart                     gotreesitter.Symbol = 142
	rbySymForwardSlash                     gotreesitter.Symbol = 85
	rbySymBlockAmpersand                   gotreesitter.Symbol = 143
	rbySymSplatStar                        gotreesitter.Symbol = 144
	rbySymUnaryMinus                       gotreesitter.Symbol = 145
	rbySymUnaryMinusNum                    gotreesitter.Symbol = 146
	rbySymBinaryMinus                      gotreesitter.Symbol = 147
	rbySymBinaryStar                       gotreesitter.Symbol = 148
	rbySymSingletonClassLeftAngleLeftAngle gotreesitter.Symbol = 149
	rbySymHashKeySymbol                    gotreesitter.Symbol = 150
	rbySymIdentifierSuffix                 gotreesitter.Symbol = 151
	rbySymConstantSuffix                   gotreesitter.Symbol = 152
	rbySymHashSplatStarStar                gotreesitter.Symbol = 153
	rbySymBinaryStarStar                   gotreesitter.Symbol = 154
	rbySymElementReferenceBracket          gotreesitter.Symbol = 155
	rbySymShortInterpolation               gotreesitter.Symbol = 156
)

// rbySymTable maps token indexes to concrete symbol IDs.
var rbySymTable = [rbyTokNone + 1]gotreesitter.Symbol{
	rbySymLineBreak,
	rbySymNoLineBreak,
	rbySymSimpleSymbol,
	rbySymStringStart,
	rbySymSymbolStart,
	rbySymSubshellStart,
	rbySymRegexStart,
	rbySymStringArrayStart,
	rbySymSymbolArrayStart,
	rbySymHeredocBodyStart,
	rbySymStringContent,
	rbySymHeredocContent,
	rbySymStringEnd,
	rbySymHeredocBodyEnd,
	rbySymHeredocStart,
	rbySymForwardSlash,
	rbySymBlockAmpersand,
	rbySymSplatStar,
	rbySymUnaryMinus,
	rbySymUnaryMinusNum,
	rbySymBinaryMinus,
	rbySymBinaryStar,
	rbySymSingletonClassLeftAngleLeftAngle,
	rbySymHashKeySymbol,
	rbySymIdentifierSuffix,
	rbySymConstantSuffix,
	rbySymHashSplatStarStar,
	rbySymBinaryStarStar,
	rbySymElementReferenceBracket,
	rbySymShortInterpolation,
	0, // NONE sentinel
}

// Maximum serialization buffer size (matches TREE_SITTER_SERIALIZATION_BUFFER_SIZE).
const rbyMaxSerialize = 1024

// Characters that are not valid in Ruby identifiers.
const rbyNonIdentifierChars = "\x00\n\r\t :;`\"'@$#.,|^&<=>+-*/\\%?!~()[]{}"

// rbyLiteral mirrors the C Literal struct.
type rbyLiteral struct {
	tokenType           int // one of the rbyTok* constants
	openDelimiter       rune
	closeDelimiter      rune
	nestingDepth        int
	allowsInterpolation bool
}

// rbyHeredoc mirrors the C Heredoc struct.
type rbyHeredoc struct {
	word                      []byte
	endWordIndentationAllowed bool
	allowsInterpolation       bool
	started                   bool
}

// rbyScannerState holds the mutable state for the Ruby external scanner.
type rbyScannerState struct {
	hasLeadingWhitespace bool
	literalStack         []rbyLiteral
	openHeredocs         []rbyHeredoc
}

// RubyExternalScanner handles all external tokens for the Ruby grammar.
type RubyExternalScanner struct{}

func (RubyExternalScanner) Create() any {
	return &rbyScannerState{}
}

func (RubyExternalScanner) Destroy(payload any) {}

func (RubyExternalScanner) Serialize(payload any, buf []byte) int {
	s := payload.(*rbyScannerState)
	size := 0

	if len(s.literalStack)*5+2 >= rbyMaxSerialize {
		return 0
	}

	if size >= len(buf) {
		return 0
	}
	buf[size] = byte(len(s.literalStack))
	size++

	for i := range s.literalStack {
		lit := &s.literalStack[i]
		if size+5 > len(buf) {
			return 0
		}
		buf[size] = byte(lit.tokenType)
		size++
		buf[size] = byte(lit.openDelimiter)
		size++
		buf[size] = byte(lit.closeDelimiter)
		size++
		buf[size] = byte(lit.nestingDepth)
		size++
		if lit.allowsInterpolation {
			buf[size] = 1
		} else {
			buf[size] = 0
		}
		size++
	}

	if size >= len(buf) {
		return 0
	}
	buf[size] = byte(len(s.openHeredocs))
	size++

	for i := range s.openHeredocs {
		hd := &s.openHeredocs[i]
		if size+4+len(hd.word) >= rbyMaxSerialize {
			return 0
		}
		if size+4+len(hd.word) > len(buf) {
			return 0
		}
		if hd.endWordIndentationAllowed {
			buf[size] = 1
		} else {
			buf[size] = 0
		}
		size++
		if hd.allowsInterpolation {
			buf[size] = 1
		} else {
			buf[size] = 0
		}
		size++
		if hd.started {
			buf[size] = 1
		} else {
			buf[size] = 0
		}
		size++
		buf[size] = byte(len(hd.word))
		size++
		copy(buf[size:], hd.word)
		size += len(hd.word)
	}

	return size
}

func (RubyExternalScanner) Deserialize(payload any, buf []byte) {
	s := payload.(*rbyScannerState)
	s.hasLeadingWhitespace = false
	s.literalStack = s.literalStack[:0]
	s.openHeredocs = s.openHeredocs[:0]

	if len(buf) == 0 {
		return
	}

	size := 0
	literalDepth := int(buf[size])
	size++
	for j := 0; j < literalDepth && size+5 <= len(buf); j++ {
		lit := rbyLiteral{}
		lit.tokenType = int(buf[size])
		size++
		lit.openDelimiter = rune(buf[size])
		size++
		lit.closeDelimiter = rune(buf[size])
		size++
		lit.nestingDepth = int(buf[size])
		size++
		lit.allowsInterpolation = buf[size] != 0
		size++
		s.literalStack = append(s.literalStack, lit)
	}

	if size >= len(buf) {
		return
	}
	openHeredocCount := int(buf[size])
	size++
	for j := 0; j < openHeredocCount && size < len(buf); j++ {
		hd := rbyHeredoc{}
		if size+4 > len(buf) {
			break
		}
		hd.endWordIndentationAllowed = buf[size] != 0
		size++
		hd.allowsInterpolation = buf[size] != 0
		size++
		hd.started = buf[size] != 0
		size++
		wordLen := int(buf[size])
		size++
		if size+wordLen > len(buf) {
			break
		}
		hd.word = make([]byte, wordLen)
		copy(hd.word, buf[size:size+wordLen])
		size += wordLen
		s.openHeredocs = append(s.openHeredocs, hd)
	}
}

func (RubyExternalScanner) Scan(payload any, lexer *gotreesitter.ExternalLexer, validSymbols []bool) bool {
	s := payload.(*rbyScannerState)
	return rbyScan(s, lexer, validSymbols)
}

// ---------- helpers ----------

func rbyIsValid(validSymbols []bool, idx int) bool {
	return idx >= 0 && idx < len(validSymbols) && validSymbols[idx]
}

func rbySkip(s *rbyScannerState, lexer *gotreesitter.ExternalLexer) {
	s.hasLeadingWhitespace = true
	lexer.Advance(true)
}

func rbyAdvance(lexer *gotreesitter.ExternalLexer) {
	lexer.Advance(false)
}

func rbyIsEOF(lexer *gotreesitter.ExternalLexer) bool {
	return lexer.Lookahead() == 0
}

func rbyIsIdenChar(c rune) bool {
	if c == 0 {
		return false
	}
	return !strings.ContainsRune(rbyNonIdentifierChars, c)
}

func rbySetResult(lexer *gotreesitter.ExternalLexer, tok int) {
	lexer.SetResultSymbol(rbySymTable[tok])
}

// ---------- scan_operator ----------

func rbyScanOperator(lexer *gotreesitter.ExternalLexer) bool {
	switch lexer.Lookahead() {
	case '<':
		rbyAdvance(lexer)
		if lexer.Lookahead() == '<' {
			rbyAdvance(lexer)
		} else if lexer.Lookahead() == '=' {
			rbyAdvance(lexer)
			if lexer.Lookahead() == '>' {
				rbyAdvance(lexer)
			}
		}
		return true

	case '>':
		rbyAdvance(lexer)
		if lexer.Lookahead() == '>' || lexer.Lookahead() == '=' {
			rbyAdvance(lexer)
		}
		return true

	case '=':
		rbyAdvance(lexer)
		if lexer.Lookahead() == '~' {
			rbyAdvance(lexer)
			return true
		}
		if lexer.Lookahead() == '=' {
			rbyAdvance(lexer)
			if lexer.Lookahead() == '=' {
				rbyAdvance(lexer)
			}
			return true
		}
		return false

	case '+', '-', '~':
		rbyAdvance(lexer)
		if lexer.Lookahead() == '@' {
			rbyAdvance(lexer)
		}
		return true

	case '.':
		rbyAdvance(lexer)
		if lexer.Lookahead() == '.' {
			rbyAdvance(lexer)
			return true
		}
		return false

	case '&', '^', '|', '/', '%', '`':
		rbyAdvance(lexer)
		return true

	case '!':
		rbyAdvance(lexer)
		if lexer.Lookahead() == '=' || lexer.Lookahead() == '~' {
			rbyAdvance(lexer)
		}
		return true

	case '*':
		rbyAdvance(lexer)
		if lexer.Lookahead() == '*' {
			rbyAdvance(lexer)
		}
		return true

	case '[':
		rbyAdvance(lexer)
		if lexer.Lookahead() == ']' {
			rbyAdvance(lexer)
		} else {
			return false
		}
		if lexer.Lookahead() == '=' {
			rbyAdvance(lexer)
		}
		return true

	default:
		return false
	}
}

// ---------- scan_symbol_identifier ----------

func rbyScanSymbolIdentifier(lexer *gotreesitter.ExternalLexer) bool {
	if lexer.Lookahead() == '@' {
		rbyAdvance(lexer)
		if lexer.Lookahead() == '@' {
			rbyAdvance(lexer)
		}
	} else if lexer.Lookahead() == '$' {
		rbyAdvance(lexer)
	}

	if rbyIsIdenChar(lexer.Lookahead()) {
		rbyAdvance(lexer)
	} else if !rbyScanOperator(lexer) {
		return false
	}

	for rbyIsIdenChar(lexer.Lookahead()) {
		rbyAdvance(lexer)
	}

	if lexer.Lookahead() == '?' || lexer.Lookahead() == '!' {
		rbyAdvance(lexer)
	}

	if lexer.Lookahead() == '=' {
		lexer.MarkEnd()
		rbyAdvance(lexer)
		if lexer.Lookahead() != '>' {
			lexer.MarkEnd()
		}
	}

	return true
}

// ---------- scan_open_delimiter ----------

func rbyScanOpenDelimiter(s *rbyScannerState, lexer *gotreesitter.ExternalLexer, literal *rbyLiteral, validSymbols []bool) bool {
	switch lexer.Lookahead() {
	case '"':
		literal.tokenType = rbyTokStringStart
		literal.openDelimiter = '"'
		literal.closeDelimiter = '"'
		literal.allowsInterpolation = true
		rbyAdvance(lexer)
		return true

	case '\'':
		literal.tokenType = rbyTokStringStart
		literal.openDelimiter = '\''
		literal.closeDelimiter = '\''
		literal.allowsInterpolation = false
		rbyAdvance(lexer)
		return true

	case '`':
		if !rbyIsValid(validSymbols, rbyTokSubshellStart) {
			return false
		}
		literal.tokenType = rbyTokSubshellStart
		literal.openDelimiter = '`'
		literal.closeDelimiter = '`'
		literal.allowsInterpolation = true
		rbyAdvance(lexer)
		return true

	case '/':
		if !rbyIsValid(validSymbols, rbyTokRegexStart) {
			return false
		}
		literal.tokenType = rbyTokRegexStart
		literal.openDelimiter = '/'
		literal.closeDelimiter = '/'
		literal.allowsInterpolation = true
		rbyAdvance(lexer)
		if rbyIsValid(validSymbols, rbyTokForwardSlash) {
			if !s.hasLeadingWhitespace {
				return false
			}
			la := lexer.Lookahead()
			if la == ' ' || la == '\t' || la == '\n' || la == '\r' {
				return false
			}
			if la == '=' {
				return false
			}
		}
		return true

	case '%':
		rbyAdvance(lexer)
		switch lexer.Lookahead() {
		case 's':
			if !rbyIsValid(validSymbols, rbyTokSimpleSymbol) {
				return false
			}
			literal.tokenType = rbyTokSymbolStart
			literal.allowsInterpolation = false
			rbyAdvance(lexer)

		case 'r':
			if !rbyIsValid(validSymbols, rbyTokRegexStart) {
				return false
			}
			literal.tokenType = rbyTokRegexStart
			literal.allowsInterpolation = true
			rbyAdvance(lexer)

		case 'x':
			if !rbyIsValid(validSymbols, rbyTokSubshellStart) {
				return false
			}
			literal.tokenType = rbyTokSubshellStart
			literal.allowsInterpolation = true
			rbyAdvance(lexer)

		case 'q':
			if !rbyIsValid(validSymbols, rbyTokStringStart) {
				return false
			}
			literal.tokenType = rbyTokStringStart
			literal.allowsInterpolation = false
			rbyAdvance(lexer)

		case 'Q':
			if !rbyIsValid(validSymbols, rbyTokStringStart) {
				return false
			}
			literal.tokenType = rbyTokStringStart
			literal.allowsInterpolation = true
			rbyAdvance(lexer)

		case 'w':
			if !rbyIsValid(validSymbols, rbyTokStringArrayStart) {
				return false
			}
			literal.tokenType = rbyTokStringArrayStart
			literal.allowsInterpolation = false
			rbyAdvance(lexer)

		case 'i':
			if !rbyIsValid(validSymbols, rbyTokSymbolArrayStart) {
				return false
			}
			literal.tokenType = rbyTokSymbolArrayStart
			literal.allowsInterpolation = false
			rbyAdvance(lexer)

		case 'W':
			if !rbyIsValid(validSymbols, rbyTokStringArrayStart) {
				return false
			}
			literal.tokenType = rbyTokStringArrayStart
			literal.allowsInterpolation = true
			rbyAdvance(lexer)

		case 'I':
			if !rbyIsValid(validSymbols, rbyTokSymbolArrayStart) {
				return false
			}
			literal.tokenType = rbyTokSymbolArrayStart
			literal.allowsInterpolation = true
			rbyAdvance(lexer)

		default:
			if !rbyIsValid(validSymbols, rbyTokStringStart) {
				return false
			}
			literal.tokenType = rbyTokStringStart
			literal.allowsInterpolation = true
		}

		switch lexer.Lookahead() {
		case '(':
			literal.openDelimiter = '('
			literal.closeDelimiter = ')'
		case '[':
			literal.openDelimiter = '['
			literal.closeDelimiter = ']'
		case '{':
			literal.openDelimiter = '{'
			literal.closeDelimiter = '}'
		case '<':
			literal.openDelimiter = '<'
			literal.closeDelimiter = '>'
		case '\r', '\n', ' ', '\t':
			if rbyIsValid(validSymbols, rbyTokForwardSlash) {
				return false
			}
		case '|', '!', '#', '/', '\\', '@', '$', '%', '^', '&', '*',
			')', ']', '}', '>', '+', '-', '~', '`', ',', '.', '?',
			':', ';', '_', '"', '\'':
			literal.openDelimiter = lexer.Lookahead()
			literal.closeDelimiter = lexer.Lookahead()
		default:
			return false
		}

		rbyAdvance(lexer)
		return true

	default:
		return false
	}
}

// ---------- scan_heredoc_word ----------

func rbyScanHeredocWord(lexer *gotreesitter.ExternalLexer, heredoc *rbyHeredoc) {
	var word []byte
	var quote rune

	switch lexer.Lookahead() {
	case '\'', '"', '`':
		quote = lexer.Lookahead()
		rbyAdvance(lexer)
		for lexer.Lookahead() != quote && !rbyIsEOF(lexer) {
			word = append(word, byte(lexer.Lookahead()))
			rbyAdvance(lexer)
		}
		rbyAdvance(lexer)
	default:
		la := lexer.Lookahead()
		if unicode.IsLetter(la) || unicode.IsDigit(la) || la == '_' {
			word = append(word, byte(la))
			rbyAdvance(lexer)
			for {
				la = lexer.Lookahead()
				if unicode.IsLetter(la) || unicode.IsDigit(la) || la == '_' {
					word = append(word, byte(la))
					rbyAdvance(lexer)
				} else {
					break
				}
			}
		}
	}

	heredoc.word = word
	heredoc.allowsInterpolation = quote != '\''
}

// ---------- scan_short_interpolation ----------

func rbyScanShortInterpolation(lexer *gotreesitter.ExternalLexer, hasContent bool, contentTok int) bool {
	start := lexer.Lookahead()
	if start == '@' || start == '$' {
		if hasContent {
			rbySetResult(lexer, contentTok)
			return true
		}
		lexer.MarkEnd()
		rbyAdvance(lexer)
		isShortInterpolation := false
		if start == '$' {
			if strings.ContainsRune("!@&`'+~=/\\,;.<>*$?:\"", lexer.Lookahead()) {
				isShortInterpolation = true
			} else if lexer.Lookahead() == '-' {
				rbyAdvance(lexer)
				la := lexer.Lookahead()
				isShortInterpolation = unicode.IsLetter(la) || la == '_'
			} else {
				la := lexer.Lookahead()
				isShortInterpolation = unicode.IsLetter(la) || unicode.IsDigit(la) || la == '_'
			}
		}
		if start == '@' {
			if lexer.Lookahead() == '@' {
				rbyAdvance(lexer)
			}
			la := lexer.Lookahead()
			isShortInterpolation = rbyIsIdenChar(la) && !unicode.IsDigit(la)
		}
		if isShortInterpolation {
			rbySetResult(lexer, rbyTokShortInterpolation)
			return true
		}
	}
	return false
}

// ---------- scan_whitespace ----------

// rbyScanWhitespace skips whitespace and handles LINE_BREAK / HEREDOC_BODY_START.
// Returns (ok, producedToken): ok=false means scan should return false;
// producedToken=true means a real result symbol was set and scan should return true.
func rbyScanWhitespace(s *rbyScannerState, lexer *gotreesitter.ExternalLexer, validSymbols []bool) (ok bool, producedToken bool) {
	heredocBodyStartIsValid := len(s.openHeredocs) > 0 &&
		!s.openHeredocs[0].started &&
		rbyIsValid(validSymbols, rbyTokHeredocBodyStart)
	crossedNewline := false

	for {
		// NOTE: is_at_included_range_start is not available in the Go ExternalLexer API,
		// so we skip that check. This may cause minor differences in multi-language
		// injection scenarios but is correct for standalone Ruby parsing.

		switch lexer.Lookahead() {
		case ' ', '\t':
			rbySkip(s, lexer)

		case '\r':
			if heredocBodyStartIsValid {
				rbySetResult(lexer, rbyTokHeredocBodyStart)
				s.openHeredocs[0].started = true
				return true, true
			}
			rbySkip(s, lexer)

		case '\n':
			if heredocBodyStartIsValid {
				rbySetResult(lexer, rbyTokHeredocBodyStart)
				s.openHeredocs[0].started = true
				return true, true
			} else if !rbyIsValid(validSymbols, rbyTokNoLineBreak) &&
				rbyIsValid(validSymbols, rbyTokLineBreak) && !crossedNewline {
				lexer.MarkEnd()
				rbyAdvance(lexer)
				crossedNewline = true
			} else {
				rbySkip(s, lexer)
			}

		case '\\':
			rbyAdvance(lexer)
			if lexer.Lookahead() == '\r' {
				rbySkip(s, lexer)
			}
			if unicode.IsSpace(lexer.Lookahead()) {
				rbySkip(s, lexer)
			} else {
				return false, false
			}

		default:
			if crossedNewline {
				la := lexer.Lookahead()
				if la != '.' && la != '&' && la != '#' {
					rbySetResult(lexer, rbyTokLineBreak)
					return true, true
				} else if la == '.' {
					rbyAdvance(lexer)
					if !rbyIsEOF(lexer) && lexer.Lookahead() == '.' {
						rbySetResult(lexer, rbyTokLineBreak)
						return true, true
					}
					return false, false
				}
			}
			return true, false
		}
	}
}

// ---------- scan_heredoc_content ----------

func rbyScanHeredocContent(s *rbyScannerState, lexer *gotreesitter.ExternalLexer) bool {
	heredoc := &s.openHeredocs[0]
	positionInWord := 0
	lookForHeredocEnd := true
	hasContent := false

	for {
		if positionInWord == len(heredoc.word) {
			if !hasContent {
				lexer.MarkEnd()
			}
			for lexer.Lookahead() == ' ' || lexer.Lookahead() == '\t' {
				rbyAdvance(lexer)
			}
			if lexer.Lookahead() == '\n' || lexer.Lookahead() == '\r' {
				if hasContent {
					rbySetResult(lexer, rbyTokHeredocContent)
				} else {
					s.openHeredocs = s.openHeredocs[1:]
					rbySetResult(lexer, rbyTokHeredocBodyEnd)
				}
				return true
			}
			hasContent = true
			positionInWord = 0
		}

		if rbyIsEOF(lexer) {
			lexer.MarkEnd()
			if hasContent {
				rbySetResult(lexer, rbyTokHeredocContent)
			} else {
				s.openHeredocs = s.openHeredocs[1:]
				rbySetResult(lexer, rbyTokHeredocBodyEnd)
			}
			return true
		}

		if lookForHeredocEnd && lexer.Lookahead() == rune(heredoc.word[positionInWord]) {
			rbyAdvance(lexer)
			positionInWord++
		} else {
			positionInWord = 0
			lookForHeredocEnd = false

			if heredoc.allowsInterpolation && lexer.Lookahead() == '\\' {
				if hasContent {
					rbySetResult(lexer, rbyTokHeredocContent)
					return true
				}
				return false
			}

			if heredoc.allowsInterpolation && lexer.Lookahead() == '#' {
				lexer.MarkEnd()
				rbyAdvance(lexer)
				if lexer.Lookahead() == '{' {
					if hasContent {
						rbySetResult(lexer, rbyTokHeredocContent)
						return true
					}
					return false
				}
				if rbyScanShortInterpolation(lexer, hasContent, rbyTokHeredocContent) {
					return true
				}
			} else if lexer.Lookahead() == '\r' || lexer.Lookahead() == '\n' {
				if lexer.Lookahead() == '\r' {
					rbyAdvance(lexer)
					if lexer.Lookahead() == '\n' {
						rbyAdvance(lexer)
					}
				} else {
					rbyAdvance(lexer)
				}
				hasContent = true
				lookForHeredocEnd = true
				for lexer.Lookahead() == ' ' || lexer.Lookahead() == '\t' {
					rbyAdvance(lexer)
					if !heredoc.endWordIndentationAllowed {
						lookForHeredocEnd = false
					}
				}
				lexer.MarkEnd()
			} else {
				hasContent = true
				rbyAdvance(lexer)
				lexer.MarkEnd()
			}
		}
	}
}

// ---------- scan_literal_content ----------

func rbyScanLiteralContent(s *rbyScannerState, lexer *gotreesitter.ExternalLexer) bool {
	literal := &s.literalStack[len(s.literalStack)-1]
	hasContent := false
	stopOnSpace := literal.tokenType == rbyTokSymbolArrayStart || literal.tokenType == rbyTokStringArrayStart

	for {
		if stopOnSpace && unicode.IsSpace(lexer.Lookahead()) {
			if hasContent {
				lexer.MarkEnd()
				rbySetResult(lexer, rbyTokStringContent)
				return true
			}
			return false
		}

		if lexer.Lookahead() == literal.closeDelimiter {
			lexer.MarkEnd()
			if literal.nestingDepth == 1 {
				if hasContent {
					rbySetResult(lexer, rbyTokStringContent)
				} else {
					rbyAdvance(lexer)
					if literal.tokenType == rbyTokRegexStart {
						for unicode.IsLower(lexer.Lookahead()) {
							rbyAdvance(lexer)
						}
					}
					s.literalStack = s.literalStack[:len(s.literalStack)-1]
					rbySetResult(lexer, rbyTokStringEnd)
					lexer.MarkEnd()
				}
				return true
			}
			literal.nestingDepth--
			rbyAdvance(lexer)
		} else if lexer.Lookahead() == literal.openDelimiter {
			literal.nestingDepth++
			rbyAdvance(lexer)
		} else if literal.allowsInterpolation && lexer.Lookahead() == '#' {
			lexer.MarkEnd()
			rbyAdvance(lexer)
			if lexer.Lookahead() == '{' {
				if hasContent {
					rbySetResult(lexer, rbyTokStringContent)
					return true
				}
				return false
			}
			if rbyScanShortInterpolation(lexer, hasContent, rbyTokStringContent) {
				return true
			}
		} else if lexer.Lookahead() == '\\' {
			if literal.allowsInterpolation {
				if hasContent {
					lexer.MarkEnd()
					rbySetResult(lexer, rbyTokStringContent)
					return true
				}
				return false
			}
			rbyAdvance(lexer)
			rbyAdvance(lexer)
		} else if rbyIsEOF(lexer) {
			rbyAdvance(lexer)
			lexer.MarkEnd()
			return false
		} else {
			rbyAdvance(lexer)
		}

		hasContent = true
	}
}

// ---------- main scan ----------

func rbyScan(s *rbyScannerState, lexer *gotreesitter.ExternalLexer, validSymbols []bool) bool {
	s.hasLeadingWhitespace = false

	// Contents of literals, which match any character except for some close delimiter.
	if !rbyIsValid(validSymbols, rbyTokStringStart) {
		if (rbyIsValid(validSymbols, rbyTokStringContent) || rbyIsValid(validSymbols, rbyTokStringEnd)) &&
			len(s.literalStack) > 0 {
			return rbyScanLiteralContent(s, lexer)
		}
		if (rbyIsValid(validSymbols, rbyTokHeredocContent) || rbyIsValid(validSymbols, rbyTokHeredocBodyEnd)) &&
			len(s.openHeredocs) > 0 {
			return rbyScanHeredocContent(s, lexer)
		}
	}

	// Whitespace handling. The C code sets result_symbol = NONE, calls scan_whitespace,
	// then checks result_symbol != NONE. We replicate this by having rbyScanWhitespace
	// return whether it produced a token.
	ok, producedToken := rbyScanWhitespace(s, lexer, validSymbols)
	if !ok {
		return false
	}
	if producedToken {
		return true
	}

	switch lexer.Lookahead() {
	case '&':
		if rbyIsValid(validSymbols, rbyTokBlockAmpersand) {
			rbyAdvance(lexer)
			la := lexer.Lookahead()
			if la != '&' && la != '.' && la != '=' && !unicode.IsSpace(la) {
				rbySetResult(lexer, rbyTokBlockAmpersand)
				return true
			}
			return false
		}

	case '<':
		if rbyIsValid(validSymbols, rbyTokSingletonClassLeftAngleLeftAngle) {
			rbyAdvance(lexer)
			if lexer.Lookahead() == '<' {
				rbyAdvance(lexer)
				rbySetResult(lexer, rbyTokSingletonClassLeftAngleLeftAngle)
				return true
			}
			return false
		}

	case '*':
		if rbyIsValid(validSymbols, rbyTokSplatStar) || rbyIsValid(validSymbols, rbyTokBinaryStar) ||
			rbyIsValid(validSymbols, rbyTokHashSplatStarStar) || rbyIsValid(validSymbols, rbyTokBinaryStarStar) {
			rbyAdvance(lexer)
			if lexer.Lookahead() == '=' {
				return false
			}
			if lexer.Lookahead() == '*' {
				if rbyIsValid(validSymbols, rbyTokHashSplatStarStar) || rbyIsValid(validSymbols, rbyTokBinaryStarStar) {
					rbyAdvance(lexer)
					if lexer.Lookahead() == '=' {
						return false
					}
					if rbyIsValid(validSymbols, rbyTokBinaryStarStar) && !s.hasLeadingWhitespace {
						rbySetResult(lexer, rbyTokBinaryStarStar)
						return true
					}
					if rbyIsValid(validSymbols, rbyTokHashSplatStarStar) && !unicode.IsSpace(lexer.Lookahead()) {
						rbySetResult(lexer, rbyTokHashSplatStarStar)
						return true
					}
					if rbyIsValid(validSymbols, rbyTokBinaryStarStar) {
						rbySetResult(lexer, rbyTokBinaryStarStar)
						return true
					}
					if rbyIsValid(validSymbols, rbyTokHashSplatStarStar) {
						rbySetResult(lexer, rbyTokHashSplatStarStar)
						return true
					}
					return false
				}
				return false
			}
			if rbyIsValid(validSymbols, rbyTokBinaryStar) && !s.hasLeadingWhitespace {
				rbySetResult(lexer, rbyTokBinaryStar)
				return true
			}
			if rbyIsValid(validSymbols, rbyTokSplatStar) && !unicode.IsSpace(lexer.Lookahead()) {
				rbySetResult(lexer, rbyTokSplatStar)
				return true
			}
			if rbyIsValid(validSymbols, rbyTokBinaryStar) {
				rbySetResult(lexer, rbyTokBinaryStar)
				return true
			}
			if rbyIsValid(validSymbols, rbyTokSplatStar) {
				rbySetResult(lexer, rbyTokSplatStar)
				return true
			}
			return false
		}

	case '-':
		if rbyIsValid(validSymbols, rbyTokUnaryMinus) || rbyIsValid(validSymbols, rbyTokUnaryMinusNum) ||
			rbyIsValid(validSymbols, rbyTokBinaryMinus) {
			rbyAdvance(lexer)
			la := lexer.Lookahead()
			if la != '=' && la != '>' {
				if rbyIsValid(validSymbols, rbyTokUnaryMinusNum) &&
					(!rbyIsValid(validSymbols, rbyTokBinaryStar) || s.hasLeadingWhitespace) &&
					unicode.IsDigit(la) {
					rbySetResult(lexer, rbyTokUnaryMinusNum)
					return true
				}
				if rbyIsValid(validSymbols, rbyTokUnaryMinus) && s.hasLeadingWhitespace && !unicode.IsSpace(la) {
					rbySetResult(lexer, rbyTokUnaryMinus)
				} else if rbyIsValid(validSymbols, rbyTokBinaryMinus) {
					rbySetResult(lexer, rbyTokBinaryMinus)
				} else {
					rbySetResult(lexer, rbyTokUnaryMinus)
				}
				return true
			}
			return false
		}

	case ':':
		if rbyIsValid(validSymbols, rbyTokSymbolStart) {
			lit := rbyLiteral{
				tokenType:    rbyTokSymbolStart,
				nestingDepth: 1,
			}
			rbyAdvance(lexer)

			switch lexer.Lookahead() {
			case '"':
				rbyAdvance(lexer)
				lit.openDelimiter = '"'
				lit.closeDelimiter = '"'
				lit.allowsInterpolation = true
				s.literalStack = append(s.literalStack, lit)
				rbySetResult(lexer, rbyTokSymbolStart)
				return true

			case '\'':
				rbyAdvance(lexer)
				lit.openDelimiter = '\''
				lit.closeDelimiter = '\''
				lit.allowsInterpolation = false
				s.literalStack = append(s.literalStack, lit)
				rbySetResult(lexer, rbyTokSymbolStart)
				return true

			default:
				if rbyScanSymbolIdentifier(lexer) {
					rbySetResult(lexer, rbyTokSimpleSymbol)
					return true
				}
			}

			return false
		}

	case '[':
		if rbyIsValid(validSymbols, rbyTokElementReferenceBracket) &&
			(!s.hasLeadingWhitespace || !rbyIsValid(validSymbols, rbyTokStringStart)) {
			rbyAdvance(lexer)
			rbySetResult(lexer, rbyTokElementReferenceBracket)
			return true
		}
	}

	// Identifier/constant suffix and hash key symbol scanning.
	if ((rbyIsValid(validSymbols, rbyTokHashKeySymbol) || rbyIsValid(validSymbols, rbyTokIdentifierSuffix)) &&
		(unicode.IsLetter(lexer.Lookahead()) || lexer.Lookahead() == '_')) ||
		(rbyIsValid(validSymbols, rbyTokConstantSuffix) && unicode.IsUpper(lexer.Lookahead())) {

		validIdentifierSymbol := rbyTokIdentifierSuffix
		if unicode.IsUpper(lexer.Lookahead()) {
			validIdentifierSymbol = rbyTokConstantSuffix
		}

		for unicode.IsLetter(lexer.Lookahead()) || unicode.IsDigit(lexer.Lookahead()) || lexer.Lookahead() == '_' {
			rbyAdvance(lexer)
		}

		if rbyIsValid(validSymbols, rbyTokHashKeySymbol) && lexer.Lookahead() == ':' {
			lexer.MarkEnd()
			rbyAdvance(lexer)
			if lexer.Lookahead() != ':' {
				rbySetResult(lexer, rbyTokHashKeySymbol)
				return true
			}
		} else if rbyIsValid(validSymbols, validIdentifierSymbol) && lexer.Lookahead() == '!' {
			rbyAdvance(lexer)
			if lexer.Lookahead() != '=' {
				rbySetResult(lexer, validIdentifierSymbol)
				return true
			}
		}

		return false
	}

	// Open delimiters for literals.
	if rbyIsValid(validSymbols, rbyTokStringStart) {
		lit := rbyLiteral{
			nestingDepth: 1,
		}

		if lexer.Lookahead() == '<' {
			rbyAdvance(lexer)
			if lexer.Lookahead() != '<' {
				return false
			}
			rbyAdvance(lexer)

			heredoc := rbyHeredoc{}
			if lexer.Lookahead() == '-' || lexer.Lookahead() == '~' {
				rbyAdvance(lexer)
				heredoc.endWordIndentationAllowed = true
			}

			rbyScanHeredocWord(lexer, &heredoc)
			if len(heredoc.word) == 0 {
				return false
			}
			s.openHeredocs = append(s.openHeredocs, heredoc)
			rbySetResult(lexer, rbyTokHeredocStart)
			return true
		}

		if rbyScanOpenDelimiter(s, lexer, &lit, validSymbols) {
			s.literalStack = append(s.literalStack, lit)
			rbySetResult(lexer, lit.tokenType)
			return true
		}
		return false
	}

	return false
}
