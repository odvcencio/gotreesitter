//go:build !grammar_subset || grammar_subset_disassembly

package grammarruntime

import (
	"unicode"

	gotreesitter "github.com/odvcencio/gotreesitter"
)

// External token indexes for the Disassembly grammar. This is the
// external index (the position of the token in the grammar's
// `externals: [...]` list), which is exactly what tree-sitter's
// `valid_symbols` array and C's result_symbol enum are indexed by. The
// external index is stable across a blob regen as long as the externals
// list itself does not reorder; concrete numeric gotreesitter.Symbol IDs
// are NOT stable (they shift whenever the grammar's total symbol count
// changes), so this scanner never hardcodes them -- see
// disasmDefaultSymTable below.
//
// disasmTokErrorSentinel never reaches SetResultSymbol (it only gates the
// early error-recovery decline check below), but it still binds
// positionally like every other external.
const (
	disasmTokCodeIdent     = 0
	disasmTokInstruction   = 1
	disasmTokMemoryDump    = 2
	disasmTokErrorSentinel = 3
	disasmTokenCount       = 4
)

// disasmDefaultSymTable records the concrete gotreesitter.Symbol IDs the
// currently shipped disassembly.bin assigns to each external, in
// disasmTok* order. It exists only as a pre-bind fallback (and as an
// independent value to compare a real bind against in tests);
// ExternalScannerForLanguage below overwrites it with values read from
// the actual loaded Language at bind time, which is what the scanner
// must do to survive a future blob regen that renumbers absolute symbol
// IDs without touching the externals list order. code_identifier
// displays as the grammar-collapsed node name "identifier".
var disasmDefaultSymTable = [disasmTokenCount]gotreesitter.Symbol{
	18, // code_identifier (display: identifier)
	19, // instruction
	20, // memory_dump
	21, // _error_sentinel
}

// disasmExternalScannerSpec records the source contract for this
// hand-written port, so updater tooling can tell a grammar-only upstream
// change apart from one that also touches the external scanner or its
// token list. Its Externals list is also the binding source for
// ExternalScannerForLanguage: index i here is scanner token index i
// (disasmTok* order).
var disasmExternalScannerSpec = ExternalScannerSpec{
	Language:       "disassembly",
	UpstreamRepo:   "https://github.com/ColinKennedy/tree-sitter-disassembly",
	UpstreamCommit: "0229c0211dba909c5d45129ac784a3f4d49c243a",
	SourceFiles: []ExternalScannerSourceFile{
		{Path: "src/grammar.json", SHA256: "f2a0cfffbedf27f29af5230977a8e083665ea931417ca8df25ac5d42138cac47"},
		{Path: "src/scanner.c", SHA256: "63d18cce13ed2a0a0bff6ca95b77b8dcdad3d7cd8456ff2e88af13cd52845066"},
	},
	Externals: []string{
		"code_identifier",
		"instruction",
		"memory_dump",
		"_error_sentinel",
	},
}

func init() {
	RegisterExternalScannerSpec(disasmExternalScannerSpec)
}

type disasmState struct {
	expectedBytesCount uint32
	expectedBytesWidth uint32
}

// DisassemblyExternalScanner handles assembly instruction vs memory dump disambiguation.
//
// symbols holds the concrete gotreesitter.Symbol each external index maps to
// in the Language this instance was bound to (see ExternalScannerForLanguage).
// The scanner never hardcodes an absolute Symbol value: a blob regen can
// renumber the grammar's absolute symbol IDs without touching the externals
// list order, and a scanner that still called SetResultSymbol with a stale
// hardcoded ID would silently emit the wrong (but still structurally valid)
// node type instead of failing loudly.
type DisassemblyExternalScanner struct {
	symbols         [disasmTokenCount]gotreesitter.Symbol
	externalToToken []int
}

// ExternalScannerForLanguage binds the scanner's token slots to the loaded
// Language's ExternalSymbols positionally. A hardcoded absolute
// gotreesitter.Symbol constant here would emit the wrong token whenever a
// grammar bump renumbers disassembly's external symbols.
func (DisassemblyExternalScanner) ExternalScannerForLanguage(lang *gotreesitter.Language) gotreesitter.ExternalScanner {
	s := DisassemblyExternalScanner{symbols: disasmDefaultSymTable}
	s.externalToToken = bindExternalScannerSpec(lang, disasmExternalScannerSpec, func(tokenIdx int, sym gotreesitter.Symbol) {
		s.symbols[tokenIdx] = sym
	})
	return s
}

func (s DisassemblyExternalScanner) symbolTable() *[disasmTokenCount]gotreesitter.Symbol {
	if s.symbols == ([disasmTokenCount]gotreesitter.Symbol{}) {
		return &disasmDefaultSymTable
	}
	return &s.symbols
}

func (DisassemblyExternalScanner) Create() any                           { return &disasmState{} }
func (DisassemblyExternalScanner) Destroy(payload any)                   {}
func (DisassemblyExternalScanner) Serialize(payload any, buf []byte) int { return 0 }
func (DisassemblyExternalScanner) Deserialize(payload any, buf []byte) {
	s := payload.(*disasmState)
	s.expectedBytesCount = 0
	s.expectedBytesWidth = 0
}

func (sc DisassemblyExternalScanner) Scan(payload any, lexer *gotreesitter.ExternalLexer, validSymbols []bool) bool {
	s := payload.(*disasmState)

	if len(sc.externalToToken) > 0 {
		var semanticValid [disasmTokenCount]bool
		for externalIdx, valid := range validSymbols {
			if !valid || externalIdx >= len(sc.externalToToken) {
				continue
			}
			tokenIdx := sc.externalToToken[externalIdx]
			if tokenIdx >= 0 && tokenIdx < disasmTokenCount {
				semanticValid[tokenIdx] = true
			}
		}
		validSymbols = semanticValid[:]
	}
	syms := sc.symbolTable()

	isValid := func(idx int) bool {
		return idx < len(validSymbols) && validSymbols[idx]
	}

	if isValid(disasmTokErrorSentinel) {
		return false
	}

	if isValid(disasmTokCodeIdent) {
		return disasmScanCodeIdent(lexer, syms[disasmTokCodeIdent])
	}

	if isValid(disasmTokInstruction) {
		return disasmScanInstruction(s, lexer, syms[disasmTokInstruction], syms[disasmTokMemoryDump])
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

func disasmScanMemoryDump(lexer *gotreesitter.ExternalLexer, possiblyInJump bool, instructionSym, memoryDumpSym gotreesitter.Symbol) disasmMemResult {
	var timesIterated uint32
	var prevChar rune

	for {
		prevChar = lexer.Lookahead()
		lexer.Advance(false)

		if lexer.Lookahead() == '\n' || lexer.Lookahead() == 0 {
			if possiblyInJump && prevChar == '>' {
				lexer.MarkEnd()
				lexer.SetResultSymbol(instructionSym)
				return disasmMemResult{timesIterated, true}
			}
			lexer.MarkEnd()
			lexer.SetResultSymbol(memoryDumpSym)
			return disasmMemResult{timesIterated, true}
		}
		timesIterated++
	}
}

func disasmScanInstruction(s *disasmState, lexer *gotreesitter.ExternalLexer, instructionSym, memoryDumpSym gotreesitter.Symbol) bool {
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
			result := disasmScanMemoryDump(lexer, possiblyInJump, instructionSym, memoryDumpSym)
			if !result.isValid {
				lexer.MarkEnd()
				lexer.SetResultSymbol(instructionSym)
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
			lexer.SetResultSymbol(instructionSym)
			return true
		} else if possiblyInJump {
			possiblyInJump = false
		}

		if lexer.Lookahead() == '<' {
			if !hasText {
				result := disasmScanMemoryDump(lexer, possiblyInJump, instructionSym, memoryDumpSym)
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
				lexer.SetResultSymbol(memoryDumpSym)
				return true
			}
			s.expectedBytesCount = 0
			s.expectedBytesWidth = 0
			if possiblyNeedExit {
				return hasText
			}
			lexer.MarkEnd()
			lexer.SetResultSymbol(instructionSym)
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
			lexer.SetResultSymbol(instructionSym)
			possiblyNeedExit = true
		}

		if lexer.Lookahead() == ';' {
			lexer.MarkEnd()
			lexer.SetResultSymbol(instructionSym)
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

func disasmScanCodeIdent(lexer *gotreesitter.ExternalLexer, codeIdentSym gotreesitter.Symbol) bool {
	hasText := false
	hasNumberData := false
	isMaybeAtEnd := false
	possiblyInNextNum := false

	for {
		if lexer.Lookahead() == '\n' || lexer.Lookahead() == 0 {
			lexer.SetResultSymbol(codeIdentSym)
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
			lexer.SetResultSymbol(codeIdentSym)
			return hasText
		}

		switch lexer.Lookahead() {
		case ';', '#':
			lexer.SetResultSymbol(codeIdentSym)
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
