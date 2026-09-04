//go:build !grammar_subset || grammar_subset_scala

package grammars

import (
	"crypto/sha256"
	"encoding/binary"
	"unicode"

	gotreesitter "github.com/odvcencio/gotreesitter"
)

// scalaExternalScannerLocalPortSHA256 identifies the marked scanner port.
// A focused test requires an identity update when the implementation changes.
const scalaExternalScannerLocalPortSHA256 = "d0e7b147cdcfc7a28b30da296ea8d510ea85aa6262739fa9375e474e6ef1f782"

// SCALA_EXTERNAL_SCANNER_LOCAL_PORT_BEGIN

// External token indexes for the Scala grammar.
const (
	scaTokAutoSemicolon           = 0
	scaTokIndent                  = 1
	scaTokOutdent                 = 2
	scaTokSimpleStringStart       = 3
	scaTokSimpleStringMiddle      = 4
	scaTokSimpleMultiStringStart  = 5
	scaTokInterpStringMiddle      = 6
	scaTokInterpMultiStringMiddle = 7
	scaTokRawStringStart          = 8
	scaTokRawStringMiddle         = 9
	scaTokRawMultiStringMiddle    = 10
	scaTokSingleLineStringEnd     = 11
	scaTokMultilineStringEnd      = 12
	scaTokElse                    = 13
	scaTokCatch                   = 14
	scaTokFinally                 = 15
	scaTokExtends                 = 16
	scaTokDerives                 = 17
	scaTokWith                    = 18
	scaTokErrorSentinel           = 19
	scaTokenCount                 = 20
)

const (
	scaSymAutoSemicolon           gotreesitter.Symbol = 104
	scaSymIndent                  gotreesitter.Symbol = 105
	scaSymOutdent                 gotreesitter.Symbol = 106
	scaSymSimpleStringStart       gotreesitter.Symbol = 107
	scaSymSimpleStringMiddle      gotreesitter.Symbol = 108
	scaSymSimpleMultiStringStart  gotreesitter.Symbol = 109
	scaSymInterpStringMiddle      gotreesitter.Symbol = 110
	scaSymInterpMultiStringMiddle gotreesitter.Symbol = 111
	scaSymRawStringStart          gotreesitter.Symbol = 112
	scaSymRawStringMiddle         gotreesitter.Symbol = 113
	scaSymRawMultiStringMiddle    gotreesitter.Symbol = 114
	scaSymSingleLineStringEnd     gotreesitter.Symbol = 115
	scaSymMultilineStringEnd      gotreesitter.Symbol = 116
)

var scaDefaultSymTable = [scaTokenCount]gotreesitter.Symbol{
	scaSymAutoSemicolon,
	scaSymIndent,
	scaSymOutdent,
	scaSymSimpleStringStart,
	scaSymSimpleStringMiddle,
	scaSymSimpleMultiStringStart,
	scaSymInterpStringMiddle,
	scaSymInterpMultiStringMiddle,
	scaSymRawStringStart,
	scaSymRawStringMiddle,
	scaSymRawMultiStringMiddle,
	scaSymSingleLineStringEnd,
	scaSymMultilineStringEnd,
}

var scalaExternalScannerSpec = ExternalScannerSpec{
	Language:       "scala",
	UpstreamRepo:   "https://github.com/tree-sitter/tree-sitter-scala",
	UpstreamCommit: "97aead18d97708190a51d4f551ea9b05b60641c9",
	Externals: []string{
		"automatic_semicolon",
		"indent",
		"outdent",
		"simple_string_start",
		"simple_string_middle",
		"simple_multiline_string_start",
		"interpolated_string_middle",
		"interpolated_multiline_string_middle",
		"raw_string_start",
		"raw_string_middle",
		"raw_multiline_string_middle",
		"single_line_string_end",
		"multiline_string_end",
		"else",
		"catch",
		"finally",
		"extends",
		"derives",
		"with",
		"error_sentinel",
	},
}

func init() {
	RegisterExternalScannerSpec(scalaExternalScannerSpec)
}

type scalaState struct {
	indents             []int16
	lastIndentationSize int16
	lastNewlineCount    int16
	lastColumn          int16
}

// ScalaExternalScanner handles auto-semicolons, indent/outdent, and string scanning for Scala.
// The raw scanner remains conservative. The exact built-in runtime profile
// wraps it with incremental checkpoint capabilities after it verifies the blob.
type ScalaExternalScanner struct {
	symbols                [scaTokenCount]gotreesitter.Symbol
	externalToToken        []int
	grammarBlobSHA256      [32]byte
	grammarIdentityPresent bool
}

func (ScalaExternalScanner) ExternalScannerForLanguage(lang *gotreesitter.Language) gotreesitter.ExternalScanner {
	scanner := ScalaExternalScanner{symbols: scaDefaultSymTable}
	scanner.externalToToken = bindExternalScannerSpec(lang, scalaExternalScannerSpec, func(tokenIdx int, sym gotreesitter.Symbol) {
		scanner.symbols[tokenIdx] = sym
	})
	if lang != nil {
		if sum, ok := lang.GrammarBlobSHA256(); ok {
			scanner.grammarBlobSHA256 = sum
			scanner.grammarIdentityPresent = true
		}
	}
	return scanner
}

func (s ScalaExternalScanner) externalScannerForExactRuntimeProfile() gotreesitter.ExternalScanner {
	return scalaCertifiedExternalScanner{ScalaExternalScanner: s}
}

type scalaCertifiedExternalScanner struct {
	ScalaExternalScanner
}

const (
	scalaScannerCheckpointMagic    = 0x53
	scalaScannerCheckpointVersion  = 1
	scalaScannerCheckpointHeader   = 10
	scalaExternalScannerABIVersion = "gotreesitter/scala-external-scanner/v1"
	scalaExternalScannerSemantics  = "state=indents,last-indentation-size,last-newline-count,last-column;codec=framed-le-v1;failure=eager-restore;error-tree=reject"
)

func (s scalaCertifiedExternalScanner) CheckpointIdentity() (gotreesitter.ExternalScannerCheckpointIdentity, bool) {
	if !s.grammarIdentityPresent {
		return gotreesitter.ExternalScannerCheckpointIdentity{}, false
	}
	return gotreesitter.ExternalScannerCheckpointIdentity{
		Scanner: append([]byte(nil), scalaExternalScannerIdentity[:]...),
		Grammar: append([]byte(nil), s.grammarBlobSHA256[:]...),
	}, true
}

// Scala permits the checkpoint-authenticated token-invariant leaf fast path.
// Keep general subtree reuse closed. Interpolation edits can invalidate
// reduction ownership even when every serialized scanner checkpoint matches.
func (scalaCertifiedExternalScanner) SupportsIncrementalReuse() bool { return false }

func (scalaCertifiedExternalScanner) SupportsIncrementalReuseFromErrorTree() bool { return false }

func (scalaCertifiedExternalScanner) UsesExternalScannerCheckpoints() bool { return true }

func (s ScalaExternalScanner) symbolTable() *[scaTokenCount]gotreesitter.Symbol {
	if s.symbols == ([scaTokenCount]gotreesitter.Symbol{}) {
		return &scaDefaultSymTable
	}
	return &s.symbols
}

func (s ScalaExternalScanner) remapValidSymbols(
	validSymbols []bool,
	semanticValid *[scaTokenCount]bool,
) []bool {
	if len(s.externalToToken) == 0 {
		return validSymbols
	}
	*semanticValid = [scaTokenCount]bool{}
	for externalIndex, valid := range validSymbols {
		if !valid || externalIndex >= len(s.externalToToken) {
			continue
		}
		tokenIndex := s.externalToToken[externalIndex]
		if tokenIndex >= 0 && tokenIndex < scaTokenCount {
			semanticValid[tokenIndex] = true
		}
	}
	return semanticValid[:]
}

func (ScalaExternalScanner) Create() any {
	return &scalaState{
		lastIndentationSize: -1,
		lastColumn:          -1,
	}
}
func (ScalaExternalScanner) Destroy(payload any) {}

func (ScalaExternalScanner) Serialize(payload any, buf []byte) int {
	s := payload.(*scalaState)
	if len(buf) < scalaScannerCheckpointHeader ||
		len(s.indents) > (len(buf)-scalaScannerCheckpointHeader)/2 {
		return 0
	}
	needed := scalaScannerCheckpointHeader + len(s.indents)*2
	buf[0] = scalaScannerCheckpointMagic
	buf[1] = scalaScannerCheckpointVersion
	binary.LittleEndian.PutUint16(buf[2:4], uint16(len(s.indents)))
	binary.LittleEndian.PutUint16(buf[4:6], uint16(s.lastIndentationSize))
	binary.LittleEndian.PutUint16(buf[6:8], uint16(s.lastNewlineCount))
	binary.LittleEndian.PutUint16(buf[8:10], uint16(s.lastColumn))
	size := scalaScannerCheckpointHeader
	for _, v := range s.indents {
		binary.LittleEndian.PutUint16(buf[size:size+2], uint16(v))
		size += 2
	}
	return needed
}

func (ScalaExternalScanner) Deserialize(payload any, buf []byte) {
	s := payload.(*scalaState)
	s.indents = s.indents[:0]
	s.lastIndentationSize = -1
	s.lastColumn = -1
	s.lastNewlineCount = 0

	if len(buf) < scalaScannerCheckpointHeader ||
		buf[0] != scalaScannerCheckpointMagic ||
		buf[1] != scalaScannerCheckpointVersion {
		return
	}
	count := int(binary.LittleEndian.Uint16(buf[2:4]))
	if scalaScannerCheckpointHeader+count*2 != len(buf) {
		return
	}
	s.lastIndentationSize = int16(binary.LittleEndian.Uint16(buf[4:6]))
	s.lastNewlineCount = int16(binary.LittleEndian.Uint16(buf[6:8]))
	s.lastColumn = int16(binary.LittleEndian.Uint16(buf[8:10]))
	s.indents = make([]int16, count)
	for index := range s.indents {
		offset := scalaScannerCheckpointHeader + index*2
		s.indents[index] = int16(binary.LittleEndian.Uint16(buf[offset : offset+2]))
	}
}

func (scanner ScalaExternalScanner) Scan(payload any, lexer *gotreesitter.ExternalLexer, validSymbols []bool) bool {
	s := payload.(*scalaState)
	symbols := scanner.symbolTable()
	if len(scanner.externalToToken) > 0 {
		var semanticValid [scaTokenCount]bool
		validSymbols = scanner.remapValidSymbols(validSymbols, &semanticValid)
	}

	isValid := func(idx int) bool {
		return idx < len(validSymbols) && validSymbols[idx]
	}

	scaBack := func() int16 {
		if len(s.indents) > 0 {
			return s.indents[len(s.indents)-1]
		}
		return -1
	}

	prev := scaBack()
	var newlineCount int16
	var indentSize int16

	// Skip whitespace, count newlines
	for unicode.IsSpace(lexer.Lookahead()) {
		if lexer.Lookahead() == '\n' {
			newlineCount++
			indentSize = 0
		} else {
			indentSize++
		}
		lexer.Advance(true)
	}

	// Double outdent check
	if isValid(scaTokOutdent) &&
		(lexer.Lookahead() == 0 ||
			(prev != -1 && (lexer.Lookahead() == ')' || lexer.Lookahead() == ']' || lexer.Lookahead() == '}')) ||
			(s.lastIndentationSize != -1 && prev != -1 && s.lastIndentationSize < prev)) {
		if len(s.indents) > 0 {
			s.indents = s.indents[:len(s.indents)-1]
		}
		lexer.SetResultSymbol(symbols[scaTokOutdent])
		return true
	}
	s.lastIndentationSize = -1

	// Indent
	if isValid(scaTokIndent) && newlineCount > 0 &&
		(len(s.indents) == 0 || indentSize > scaBack()) {
		if scaDetectCommentStart(lexer) {
			return false
		}
		s.indents = append(s.indents, indentSize)
		lexer.SetResultSymbol(symbols[scaTokIndent])
		return true
	}

	// Single outdent
	if isValid(scaTokOutdent) &&
		(lexer.Lookahead() == 0 ||
			(newlineCount > 0 && prev != -1 && indentSize < prev)) {
		if len(s.indents) > 0 {
			s.indents = s.indents[:len(s.indents)-1]
		}
		lexer.SetResultSymbol(symbols[scaTokOutdent])
		s.lastIndentationSize = indentSize
		s.lastNewlineCount = newlineCount
		if lexer.Lookahead() == 0 {
			s.lastColumn = -1
		} else {
			s.lastColumn = int16(lexer.Column())
		}
		return true
	}

	// Recover newline_count from outdent reset
	isEOF := lexer.Lookahead() == 0
	if (s.lastNewlineCount > 0 && isEOF && s.lastColumn == -1) ||
		(!isEOF && int16(lexer.Column()) == s.lastColumn) {
		newlineCount += s.lastNewlineCount
	}
	s.lastNewlineCount = 0

	// Auto-semicolon
	if isValid(scaTokAutoSemicolon) && newlineCount > 0 {
		lexer.MarkEnd()
		lexer.SetResultSymbol(symbols[scaTokAutoSemicolon])

		if lexer.Lookahead() == '.' {
			return false
		}

		// Comments
		if lexer.Lookahead() == '/' {
			lexer.Advance(false)
			if lexer.Lookahead() == '/' {
				return false
			}
			if lexer.Lookahead() == '*' {
				lexer.Advance(false)
				for lexer.Lookahead() != 0 {
					if lexer.Lookahead() == '*' {
						lexer.Advance(false)
						if lexer.Lookahead() == '/' {
							lexer.Advance(false)
							break
						}
					} else {
						lexer.Advance(false)
					}
				}
				for unicode.IsSpace(lexer.Lookahead()) {
					if lexer.Lookahead() == '\n' || lexer.Lookahead() == '\r' {
						return false
					}
					lexer.Advance(true)
				}
				return true
			}
		}

		if isValid(scaTokElse) {
			return !scaScanWord(lexer, "else")
		}
		if isValid(scaTokCatch) && scaScanWord(lexer, "catch") {
			return false
		}
		if isValid(scaTokFinally) && scaScanWord(lexer, "finally") {
			return false
		}
		if isValid(scaTokExtends) && scaScanWord(lexer, "extends") {
			return false
		}
		if isValid(scaTokWith) && scaScanWord(lexer, "with") {
			return false
		}
		if isValid(scaTokDerives) && scaScanWord(lexer, "derives") {
			return false
		}

		return true
	}

	// Additional whitespace skip for string scanning
	for unicode.IsSpace(lexer.Lookahead()) {
		if lexer.Lookahead() == '\n' {
			newlineCount++
		}
		lexer.Advance(true)
	}

	// Simple string start
	if isValid(scaTokSimpleStringStart) && lexer.Lookahead() == '"' {
		lexer.Advance(false)
		lexer.MarkEnd()
		if lexer.Lookahead() == '"' {
			lexer.Advance(false)
			if lexer.Lookahead() == '"' {
				lexer.Advance(false)
				lexer.SetResultSymbol(symbols[scaTokSimpleMultiStringStart])
				lexer.MarkEnd()
				return true
			}
		}
		lexer.SetResultSymbol(symbols[scaTokSimpleStringStart])
		return true
	}

	// Raw string start: raw"
	if isValid(scaTokRawStringStart) && lexer.Lookahead() == 'r' {
		lexer.Advance(false)
		if lexer.Lookahead() == 'a' {
			lexer.Advance(false)
			if lexer.Lookahead() == 'w' {
				lexer.Advance(false)
				if lexer.Lookahead() == '"' {
					lexer.MarkEnd()
					lexer.SetResultSymbol(symbols[scaTokRawStringStart])
					return true
				}
			}
		}
	}

	// String content scanning
	if isValid(scaTokSimpleStringMiddle) {
		return scaScanStringContent(lexer, false, scaStringSimple, symbols)
	}
	if isValid(scaTokInterpStringMiddle) {
		return scaScanStringContent(lexer, false, scaStringInterp, symbols)
	}
	if isValid(scaTokRawStringMiddle) {
		return scaScanStringContent(lexer, false, scaStringRaw, symbols)
	}
	if isValid(scaTokRawMultiStringMiddle) {
		return scaScanStringContent(lexer, true, scaStringRaw, symbols)
	}
	if isValid(scaTokInterpMultiStringMiddle) {
		return scaScanStringContent(lexer, true, scaStringInterp, symbols)
	}
	if isValid(scaTokMultilineStringEnd) {
		return scaScanStringContent(lexer, true, scaStringSimple, symbols)
	}

	return false
}

type scaStringMode int

const (
	scaStringSimple scaStringMode = iota
	scaStringInterp
	scaStringRaw
)

func scaScanStringContent(
	lexer *gotreesitter.ExternalLexer,
	isMultiline bool,
	mode scaStringMode,
	symbols *[scaTokenCount]gotreesitter.Symbol,
) bool {
	closingQuotes := uint32(0)
	for {
		if lexer.Lookahead() == '"' {
			lexer.Advance(false)
			closingQuotes++
			if !isMultiline {
				lexer.SetResultSymbol(symbols[scaTokSingleLineStringEnd])
				lexer.MarkEnd()
				return true
			}
			if closingQuotes >= 3 && lexer.Lookahead() != '"' {
				lexer.SetResultSymbol(symbols[scaTokMultilineStringEnd])
				lexer.MarkEnd()
				return true
			}
		} else if lexer.Lookahead() == '$' && mode != scaStringSimple {
			switch mode {
			case scaStringInterp:
				if isMultiline {
					lexer.SetResultSymbol(symbols[scaTokInterpMultiStringMiddle])
				} else {
					lexer.SetResultSymbol(symbols[scaTokInterpStringMiddle])
				}
			case scaStringRaw:
				if isMultiline {
					lexer.SetResultSymbol(symbols[scaTokRawMultiStringMiddle])
				} else {
					lexer.SetResultSymbol(symbols[scaTokRawStringMiddle])
				}
			}
			lexer.MarkEnd()
			return true
		} else {
			closingQuotes = 0
			if lexer.Lookahead() == '\\' {
				if isMultiline || mode == scaStringRaw {
					lexer.Advance(false)
					if !isMultiline && mode == scaStringRaw &&
						(lexer.Lookahead() == '"' || lexer.Lookahead() == '\\') {
						lexer.Advance(false)
					}
				} else {
					if mode == scaStringSimple {
						lexer.SetResultSymbol(symbols[scaTokSimpleStringMiddle])
					} else {
						lexer.SetResultSymbol(symbols[scaTokInterpStringMiddle])
					}
					lexer.MarkEnd()
					return true
				}
			} else if lexer.Lookahead() == '\n' && !isMultiline {
				return false
			} else if lexer.Lookahead() == 0 {
				return false
			} else {
				lexer.Advance(false)
			}
		}
	}
}

func scaDetectCommentStart(lexer *gotreesitter.ExternalLexer) bool {
	lexer.MarkEnd()
	if lexer.Lookahead() == '/' {
		lexer.Advance(false)
		if lexer.Lookahead() == '/' || lexer.Lookahead() == '*' {
			return true
		}
	}
	return false
}

func scaScanWord(lexer *gotreesitter.ExternalLexer, word string) bool {
	for i := 0; i < len(word); i++ {
		if lexer.Lookahead() != rune(word[i]) {
			return false
		}
		lexer.Advance(false)
	}
	return !unicode.IsLetter(lexer.Lookahead()) && !unicode.IsDigit(lexer.Lookahead())
}

// SCALA_EXTERNAL_SCANNER_LOCAL_PORT_END

var scalaExternalScannerIdentity = sha256.Sum256([]byte(
	scalaExternalScannerABIVersion + "\x00" +
		"local-port=" + scalaExternalScannerLocalPortSHA256 + "\x00" +
		scalaExternalScannerSpec.UpstreamRepo + "\x00" +
		scalaExternalScannerSpec.UpstreamCommit + "\x00" +
		scalaExternalScannerSemantics,
))
