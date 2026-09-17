//go:build !grammar_subset || grammar_subset_disassembly

package grammarruntime

import (
	"unicode"

	gotreesitter "github.com/odvcencio/gotreesitter"
)

// External token indexes for the Disassembly grammar.
const (
	disasmTokCodeIdent     = 0
	disasmTokInstruction   = 1
	disasmTokMemoryDump    = 2
	disasmTokErrorSentinel = 3
)

const (
	disasmSymCodeIdent   gotreesitter.Symbol = 18
	disasmSymInstruction gotreesitter.Symbol = 19
	disasmSymMemoryDump  gotreesitter.Symbol = 20
)

type disasmState struct {
	expectedBytesCount uint32
	expectedBytesWidth uint32
}

// DisassemblyExternalScanner handles assembly instruction vs memory dump disambiguation.
type DisassemblyExternalScanner struct{}

func (DisassemblyExternalScanner) Create() any                           { return &disasmState{} }
func (DisassemblyExternalScanner) Destroy(payload any)                   {}
func (DisassemblyExternalScanner) Serialize(payload any, buf []byte) int { return 0 }
func (DisassemblyExternalScanner) Deserialize(payload any, buf []byte) {
	s := payload.(*disasmState)
	s.expectedBytesCount = 0
	s.expectedBytesWidth = 0
}

func (DisassemblyExternalScanner) Scan(payload any, lexer *gotreesitter.ExternalLexer, validSymbols []bool) bool {
	s := payload.(*disasmState)
	isValid := func(idx int) bool {
		return idx < len(validSymbols) && validSymbols[idx]
	}

	if isValid(disasmTokErrorSentinel) {
		return false
	}

	if isValid(disasmTokCodeIdent) {
		return disasmScanCodeIdent(lexer)
	}

	if isValid(disasmTokInstruction) {
		return disasmScanInstruction(s, lexer)
	}

	return false
}

func disasmIsHex(ch rune) bool {
	return (ch >= '0' && ch <= '9') || (ch >= 'a' && ch <= 'f') || (ch >= 'A' && ch <= 'F')
}

func disasmIsNumber(ch rune) bool {
	return (ch >= '0' && ch <= '9') || ch == '-'
}

func disasmLookAheadForBytes(lexer *gotreesitter.ExternalLexer, charsPerByte uint32) uint32 {
	inWS := false
	var currentCount, totalCount uint32

	for {
		if lexer.Lookahead() == '\n' || lexer.Lookahead() == 0 {
			break
		}
		if unicode.IsSpace(lexer.Lookahead()) {
			if !inWS {
				if currentCount != charsPerByte {
					break
				}
				totalCount++
				inWS = true
				currentCount = 0
			}
		} else if disasmIsHex(lexer.Lookahead()) {
			currentCount++
			inWS = false
		} else {
			break
		}
		lexer.Advance(false)
	}
	return totalCount
}

type disasmMemResult struct {
	timesIterated uint32
	isValid       bool
}

func disasmScanMemoryDump(lexer *gotreesitter.ExternalLexer, possiblyInJump bool) disasmMemResult {
	var timesIterated uint32
	var prevChar rune

	for {
		prevChar = lexer.Lookahead()
		lexer.Advance(false)

		if lexer.Lookahead() == '\n' || lexer.Lookahead() == 0 {
			if possiblyInJump && prevChar == '>' {
				lexer.MarkEnd()
				lexer.SetResultSymbol(disasmSymInstruction)
				return disasmMemResult{timesIterated, true}
			}
			lexer.MarkEnd()
			lexer.SetResultSymbol(disasmSymMemoryDump)
			return disasmMemResult{timesIterated, true}
		}
		timesIterated++
	}
}

func disasmScanInstruction(s *disasmState, lexer *gotreesitter.ExternalLexer) bool {
	hasText := false
	hasSpace := false
	hasPeriod := false
	var timesIterated uint32
	isMaybeBad := true
	isMaybeByte := true
	var hexCount uint32
	possiblyNeedExit := false
	possiblyInJump := false

	var offsetCounter uint32
	badInstr := "(bad)"

	if lexer.Lookahead() == ':' {
		return false
	}

	for {
		if hasText {
			timesIterated++
		}

		if lexer.Lookahead() == '.' {
			hasPeriod = true
			result := disasmScanMemoryDump(lexer, possiblyInJump)
			if !result.isValid {
				lexer.MarkEnd()
				lexer.SetResultSymbol(disasmSymInstruction)
				s.expectedBytesCount = 0
				s.expectedBytesWidth = 0
				return false
			}
			matches := (timesIterated + result.timesIterated + 1) == s.expectedBytesCount
			s.expectedBytesCount = 0
			s.expectedBytesWidth = 0
			if matches {
				return true
			}
			lexer.MarkEnd()
			lexer.SetResultSymbol(disasmSymInstruction)
			return true
		} else if possiblyInJump {
			possiblyInJump = false
		}

		if lexer.Lookahead() == '<' {
			if !hasText {
				result := disasmScanMemoryDump(lexer, possiblyInJump)
				if !result.isValid {
					s.expectedBytesCount = 0
					s.expectedBytesWidth = 0
					return false
				}
				matches := (timesIterated + result.timesIterated + 1) == s.expectedBytesCount
				s.expectedBytesCount = 0
				s.expectedBytesWidth = 0
				return matches
			}
			possiblyInJump = true
		}

		if lexer.Lookahead() == '\n' || lexer.Lookahead() == 0 {
			if (hasPeriod || !hasSpace) && timesIterated == s.expectedBytesCount {
				s.expectedBytesWidth = 0
				lexer.MarkEnd()
				lexer.SetResultSymbol(disasmSymMemoryDump)
				return true
			}
			s.expectedBytesCount = 0
			s.expectedBytesWidth = 0
			if possiblyNeedExit {
				return hasText
			}
			lexer.MarkEnd()
			lexer.SetResultSymbol(disasmSymInstruction)
			return hasText
		}

		if possiblyNeedExit {
			if !disasmIsNumber(lexer.Lookahead()) || lexer.Lookahead() == 0 {
				s.expectedBytesCount = 0
				s.expectedBytesWidth = 0
				return hasText
			}
			possiblyNeedExit = false
		}

		if lexer.Lookahead() == '#' {
			lexer.MarkEnd()
			lexer.SetResultSymbol(disasmSymInstruction)
			possiblyNeedExit = true
		}

		if lexer.Lookahead() == ';' {
			lexer.MarkEnd()
			lexer.SetResultSymbol(disasmSymInstruction)
			s.expectedBytesCount = 0
			s.expectedBytesWidth = 0
			return hasText
		}

		isWS := unicode.IsSpace(lexer.Lookahead())

		if isWS {
			if hasText {
				hasSpace = true
			}
			if isMaybeByte && s.expectedBytesWidth == 0 {
				s.expectedBytesWidth = timesIterated
			}
		}

		if !isWS {
			if isMaybeBad && offsetCounter < uint32(len(badInstr)) &&
				lexer.Lookahead() == rune(badInstr[offsetCounter]) {
				offsetCounter++
				if offsetCounter == uint32(len(badInstr)) {
					s.expectedBytesCount = 0
					return false
				}
			} else {
				isMaybeBad = false
				offsetCounter = 0
			}

			if hexCount >= 8 {
				isMaybeByte = false
			} else if isMaybeByte {
				if disasmIsHex(lexer.Lookahead()) {
					hexCount++
				} else {
					hexCount = 0
					isMaybeByte = false
				}
			}
			hasText = true
		} else if isMaybeByte && s.expectedBytesWidth != 0 && hexCount == s.expectedBytesWidth {
			lexer.Advance(true)
			found := disasmLookAheadForBytes(lexer, hexCount) + 1
			if found > s.expectedBytesCount {
				s.expectedBytesCount = found
			}
			return false
		}

		lexer.Advance(false)
	}
}

func disasmScanCodeIdent(lexer *gotreesitter.ExternalLexer) bool {
	hasText := false
	hasNumberData := false
	isMaybeAtEnd := false
	possiblyInNextNum := false

	for {
		if lexer.Lookahead() == '\n' || lexer.Lookahead() == 0 {
			lexer.SetResultSymbol(disasmSymCodeIdent)
			return hasText
		}

		if possiblyInNextNum {
			if disasmIsNumber(lexer.Lookahead()) {
				hasNumberData = true
			} else {
				possiblyInNextNum = false
			}
		}

		if isMaybeAtEnd && lexer.Lookahead() != '\n' && unicode.IsSpace(lexer.Lookahead()) {
			lexer.SetResultSymbol(disasmSymCodeIdent)
			return hasText
		}

		switch lexer.Lookahead() {
		case ';', '#':
			lexer.SetResultSymbol(disasmSymCodeIdent)
			return hasText
		case '+':
			lexer.MarkEnd()
			possiblyInNextNum = true
			isMaybeAtEnd = true
		case '>':
			if !hasNumberData && !possiblyInNextNum {
				lexer.MarkEnd()
			}
			isMaybeAtEnd = true
		default:
			isMaybeAtEnd = false
		}

		lexer.Advance(false)
		hasText = true
	}
}
