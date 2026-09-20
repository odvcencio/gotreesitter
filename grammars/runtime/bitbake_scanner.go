//go:build !grammar_subset || grammar_subset_bitbake

package grammarruntime

import (
	"unicode"

	gotreesitter "github.com/odvcencio/gotreesitter"
)

// External token indexes for the BitBake grammar. This is the external
// index (the position of the token in the grammar's `externals: [...]`
// list), which is exactly what tree-sitter's `valid_symbols` array and
// C's result_symbol enum are indexed by. The external index is stable
// across a blob regen as long as the externals list itself does not
// reorder; concrete numeric gotreesitter.Symbol IDs are NOT stable (they
// shift whenever the grammar's total symbol count changes), so this
// scanner never hardcodes them -- see bbDefaultSymTable below.
//
// bbTokCloseParen/bbTokCloseBracket/bbTokCloseBrace never individually
// reach SetResultSymbol (they only gate the withinBrackets OR-check
// below), so their exact upstream literal at each index does not affect
// behavior; see bbDefaultSymTable's comments for the true upstream name
// at each index.
const (
	bbTokConcat        = 0
	bbTokNewline       = 1
	bbTokIndent        = 2
	bbTokDedent        = 3
	bbTokStringStart   = 4
	bbTokStringContent = 5
	bbTokEscapeInterp  = 6
	bbTokStringEnd     = 7
	bbTokComment       = 8
	bbTokCloseParen    = 9
	bbTokCloseBracket  = 10
	bbTokCloseBrace    = 11
	bbTokShellContent  = 12
	bbTokenCount       = 13
)

// bbDefaultSymTable records the concrete gotreesitter.Symbol IDs the
// currently shipped bitbake.bin assigns to each external, in bbTok*
// order. It exists only as a pre-bind fallback (and as an independent
// value to compare a real bind against in tests); ExternalScannerForLanguage
// below overwrites it with values read from the actual loaded Language at
// bind time, which is what the scanner must do to survive a future blob
// regen that renumbers absolute symbol IDs without touching the externals
// list order.
var bbDefaultSymTable = [bbTokenCount]gotreesitter.Symbol{
	140, // _concat
	141, // _newline
	142, // _indent
	143, // _dedent
	144, // string_start
	145, // _string_content
	146, // escape_interpolation
	147, // string_end
	139, // comment
	14,  // "]" (bbTokCloseParen's actual upstream literal; never emitted)
	38,  // ")" (bbTokCloseBracket's actual upstream literal; never emitted)
	40,  // "}" (bbTokCloseBrace's actual upstream literal; never emitted)
	148, // shell_content
}

// bbExternalScannerSpec records the source contract for this hand-written
// port, so updater tooling can tell a grammar-only upstream change apart
// from one that also touches the external scanner or its token list. Its
// Externals list is also the binding source for ExternalScannerForLanguage:
// index i here is scanner token index i (bbTok* order), matching the
// upstream externals array exactly (including the "]"/")"/"}" literals at
// indices 9-11, which upstream's own C scanner also never emits by name).
var bbExternalScannerSpec = ExternalScannerSpec{
	Language:       "bitbake",
	UpstreamRepo:   "https://github.com/amaanq/tree-sitter-bitbake",
	UpstreamCommit: "a5d04fdb5a69a02b8fa8eb5525a60dfb5309b73b",
	SourceFiles: []ExternalScannerSourceFile{
		{Path: "src/grammar.json", SHA256: "02f522a7931f7a0882b5cc406f8e4e00bb4785912ec8ecfd2a484807d6bde714"},
		{Path: "src/scanner.c", SHA256: "61c10dfe3bf2b837b720451f72a2dac3b70590589850bcfef0addc7dd7405c13"},
	},
	Externals: []string{
		"_concat",
		"_newline",
		"_indent",
		"_dedent",
		"string_start",
		"_string_content",
		"escape_interpolation",
		"string_end",
		"comment",
		"]",
		")",
		"}",
		"shell_content",
	},
}

func init() {
	RegisterExternalScannerSpec(bbExternalScannerSpec)
}

// BitbakeExternalScanner handles Python-like indent/dedent, strings, concat, and shell content.
//
// symbols holds the concrete gotreesitter.Symbol each external index maps to
// in the Language this instance was bound to (see ExternalScannerForLanguage).
// The scanner never hardcodes an absolute Symbol value: a blob regen can
// renumber the grammar's absolute symbol IDs without touching the externals
// list order, and a scanner that still called SetResultSymbol with a stale
// hardcoded ID would silently emit the wrong (but still structurally valid)
// node type instead of failing loudly.
type BitbakeExternalScanner struct {
	symbols         [bbTokenCount]gotreesitter.Symbol
	externalToToken []int
}

// ExternalScannerForLanguage binds the scanner's token slots to the loaded
// Language's ExternalSymbols positionally. A hardcoded absolute
// gotreesitter.Symbol constant here would emit the wrong token whenever a
// grammar bump renumbers bitbake's external symbols.
func (BitbakeExternalScanner) ExternalScannerForLanguage(lang *gotreesitter.Language) gotreesitter.ExternalScanner {
	s := BitbakeExternalScanner{symbols: bbDefaultSymTable}
	s.externalToToken = bindExternalScannerSpec(lang, bbExternalScannerSpec, func(tokenIdx int, sym gotreesitter.Symbol) {
		s.symbols[tokenIdx] = sym
	})
	return s
}

func (s BitbakeExternalScanner) symbolTable() *[bbTokenCount]gotreesitter.Symbol {
	if s.symbols == ([bbTokenCount]gotreesitter.Symbol{}) {
		return &bbDefaultSymTable
	}
	return &s.symbols
}

// Reuse pythonScannerState — same internal structure.

func (BitbakeExternalScanner) Create() any {
	return &pythonScannerState{Indents: []uint16{0}}
}
func (BitbakeExternalScanner) Destroy(payload any) {}

func (BitbakeExternalScanner) Serialize(payload any, buf []byte) int {
	// Bitbake serialize format: 1 byte f-string flag, 1 byte delim count,
	// N bytes Delimiters, remaining bytes = Indents[1:] (1 byte each).
	s := payload.(*pythonScannerState)
	if len(buf) == 0 {
		return 0
	}
	size := 0
	if s.InsideInterpolatedString {
		buf[size] = 1
	} else {
		buf[size] = 0
	}
	size++

	delimCount := len(s.Delimiters)
	if delimCount > 255 {
		delimCount = 255
	}
	if size >= len(buf) {
		return size
	}
	buf[size] = byte(delimCount)
	size++
	for i := 0; i < delimCount && size < len(buf); i++ {
		buf[size] = byte(s.Delimiters[i])
		size++
	}
	for i := 1; i < len(s.Indents) && size < len(buf); i++ {
		buf[size] = byte(s.Indents[i])
		size++
	}
	return size
}

func (BitbakeExternalScanner) Deserialize(payload any, buf []byte) {
	s := payload.(*pythonScannerState)
	s.Delimiters = s.Delimiters[:0]
	s.Indents = s.Indents[:0]
	s.Indents = append(s.Indents, 0)
	s.InsideInterpolatedString = false

	if len(buf) == 0 {
		return
	}
	size := 0
	s.InsideInterpolatedString = buf[size] != 0
	size++
	if size >= len(buf) {
		return
	}
	delimCount := int(buf[size])
	size++
	for i := 0; i < delimCount && size < len(buf); i++ {
		s.Delimiters = append(s.Delimiters, pyDelimiter(buf[size]))
		size++
	}
	for ; size < len(buf); size++ {
		s.Indents = append(s.Indents, uint16(buf[size]))
	}
}

func (sc BitbakeExternalScanner) Scan(payload any, lexer *gotreesitter.ExternalLexer, validSymbols []bool) bool {
	s := payload.(*pythonScannerState)
	if len(s.Indents) == 0 {
		s.Indents = append(s.Indents, 0)
	}

	if len(sc.externalToToken) > 0 {
		var semanticValid [bbTokenCount]bool
		for externalIdx, valid := range validSymbols {
			if !valid || externalIdx >= len(sc.externalToToken) {
				continue
			}
			tokenIdx := sc.externalToToken[externalIdx]
			if tokenIdx >= 0 && tokenIdx < bbTokenCount {
				semanticValid[tokenIdx] = true
			}
		}
		validSymbols = semanticValid[:]
	}
	syms := sc.symbolTable()

	isValid := func(idx int) bool {
		return idx < len(validSymbols) && validSymbols[idx]
	}

	errorRecoveryMode := isValid(bbTokStringContent) && isValid(bbTokIndent)
	withinBrackets := isValid(bbTokCloseBrace) || isValid(bbTokCloseParen) || isValid(bbTokCloseBracket)

	// ---- CONCAT ----
	if isValid(bbTokConcat) && !errorRecoveryMode {
		ch := lexer.Lookahead()
		if ch != 0 && !unicode.IsSpace(ch) && ch != '(' && ch != ':' && ch != '[' && ch != '=' {
			lexer.SetResultSymbol(syms[bbTokConcat])
			return true
		}
	}

	// ---- Escape interpolation ----
	advancedOnce := false
	if isValid(bbTokEscapeInterp) && len(s.Delimiters) > 0 &&
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
				lexer.SetResultSymbol(syms[bbTokEscapeInterp])
				return true
			}
			return false
		}
	}

	// ---- String content ----
	if isValid(bbTokStringContent) && len(s.Delimiters) > 0 && !errorRecoveryMode {
		delimiter := s.Delimiters[len(s.Delimiters)-1]
		EndChar := delimiter.EndChar()
		hasContent := advancedOnce

		for lexer.Lookahead() != 0 {
			if (advancedOnce || lexer.Lookahead() == '{' || lexer.Lookahead() == '}') && delimiter.IsFormat() {
				lexer.MarkEnd()
				lexer.SetResultSymbol(syms[bbTokStringContent])
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
						lexer.SetResultSymbol(syms[bbTokStringContent])
						return hasContent
					}
				} else {
					lexer.MarkEnd()
					lexer.SetResultSymbol(syms[bbTokStringContent])
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
								lexer.SetResultSymbol(syms[bbTokStringContent])
							} else {
								lexer.Advance(false)
								lexer.MarkEnd()
								s.Delimiters = s.Delimiters[:len(s.Delimiters)-1]
								lexer.SetResultSymbol(syms[bbTokStringEnd])
								s.InsideInterpolatedString = false
							}
							return true
						}
						lexer.MarkEnd()
						lexer.SetResultSymbol(syms[bbTokStringContent])
						return true
					}
					lexer.MarkEnd()
					lexer.SetResultSymbol(syms[bbTokStringContent])
					return true
				}
				if hasContent {
					lexer.SetResultSymbol(syms[bbTokStringContent])
				} else {
					lexer.Advance(false)
					s.Delimiters = s.Delimiters[:len(s.Delimiters)-1]
					lexer.SetResultSymbol(syms[bbTokStringEnd])
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

	// ---- Indent/dedent scanning ----
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
			indentLength++
			lexer.Advance(true)
		case '\r', '\f':
			indentLength = 0
			lexer.Advance(true)
		case '\t':
			indentLength += 8
			lexer.Advance(true)
		case '#':
			if !foundEndOfLine {
				return false
			}
			if firstCommentIndentLength == -1 {
				firstCommentIndentLength = int32(indentLength)
			}
			for lexer.Lookahead() != 0 && lexer.Lookahead() != '\n' {
				lexer.Advance(true)
			}
			lexer.Advance(true)
			indentLength = 0
			continue
		case '\\':
			if isValid(bbTokStringContent) {
				lexer.Advance(true)
				if lexer.Lookahead() == '\r' {
					lexer.Advance(true)
				}
				if lexer.Lookahead() == '\n' || lexer.Lookahead() == 0 {
					lexer.Advance(true)
				} else {
					return false
				}
				continue
			}
			goto bbAfterIndentLoop
		case 0:
			indentLength = 0
			foundEndOfLine = true
			goto bbAfterIndentLoop
		default:
			goto bbAfterIndentLoop
		}
	}

bbAfterIndentLoop:
	if foundEndOfLine {
		currentIndent := s.Indents[len(s.Indents)-1]

		if isValid(bbTokIndent) && indentLength > currentIndent {
			s.Indents = append(s.Indents, indentLength)
			lexer.SetResultSymbol(syms[bbTokIndent])
			return true
		}

		nextTokIsStringStart := lexer.Lookahead() == '"' || lexer.Lookahead() == '\'' || lexer.Lookahead() == '`'

		if (isValid(bbTokDedent) ||
			(!isValid(bbTokNewline) && !(isValid(bbTokStringStart) && nextTokIsStringStart) && !withinBrackets)) &&
			indentLength < currentIndent &&
			!s.InsideInterpolatedString &&
			firstCommentIndentLength < int32(currentIndent) {
			s.Indents = s.Indents[:len(s.Indents)-1]
			lexer.SetResultSymbol(syms[bbTokDedent])
			return true
		}

		if isValid(bbTokNewline) && !errorRecoveryMode {
			lexer.SetResultSymbol(syms[bbTokNewline])
			return true
		}
	}

	// ---- String start ----
	if firstCommentIndentLength == -1 && isValid(bbTokStringStart) {
		var delimiter pyDelimiter
		hasFlags := false

		for lexer.Lookahead() != 0 {
			switch lexer.Lookahead() {
			case 'f', 'F':
				delimiter |= pyDelimFormat
			case 'r', 'R':
				delimiter |= pyDelimRaw
			case 'b', 'B':
				delimiter |= pyDelimBytes
			case 'u', 'U':
				// accepted prefix, no flag
			default:
				goto bbAfterFlags
			}
			hasFlags = true
			lexer.Advance(false)
		}

	bbAfterFlags:
		switch lexer.Lookahead() {
		case '`':
			delimiter |= pyDelimBackQuote
			lexer.Advance(false)
			lexer.MarkEnd()
		case '\'':
			delimiter |= pyDelimSingleQuote
			lexer.Advance(false)
			lexer.MarkEnd()
			if lexer.Lookahead() == '\'' {
				lexer.Advance(false)
				if lexer.Lookahead() == '\'' {
					lexer.Advance(false)
					lexer.MarkEnd()
					delimiter |= pyDelimTriple
				}
			}
		case '"':
			delimiter |= pyDelimDoubleQuote
			lexer.Advance(false)
			lexer.MarkEnd()
			if lexer.Lookahead() == '"' {
				lexer.Advance(false)
				if lexer.Lookahead() == '"' {
					lexer.Advance(false)
					lexer.MarkEnd()
					delimiter |= pyDelimTriple
				}
			}
		}

		if delimiter.EndChar() != 0 {
			s.Delimiters = append(s.Delimiters, delimiter)
			lexer.SetResultSymbol(syms[bbTokStringStart])
			s.InsideInterpolatedString = delimiter.IsFormat()
			return true
		}
		if hasFlags {
			return false
		}
	}

	// ---- Shell content ----
	if isValid(bbTokShellContent) && !errorRecoveryMode {
		// Skip whitespace until newline
		for unicode.IsSpace(lexer.Lookahead()) {
			lexer.Advance(true)
			if lexer.Lookahead() == '\n' {
				lexer.Advance(true)
				break
			}
		}

		advOnce := false
		var braceDepth uint8
		var startQuote rune

		for lexer.Lookahead() != 0 && lexer.Lookahead() != '\n' {
			switch lexer.Lookahead() {
			case '\'', '"':
				if startQuote == 0 {
					startQuote = lexer.Lookahead()
				} else if lexer.Lookahead() == startQuote {
					startQuote = 0
				}
				lexer.Advance(false)
				advOnce = true
			case '$':
				lexer.MarkEnd()
				lexer.Advance(false)
				if lexer.Lookahead() == '{' {
					lexer.Advance(false)
					braceDepth++
					if lexer.Lookahead() == '@' {
						lexer.Advance(false)
						lexer.SetResultSymbol(syms[bbTokShellContent])
						return advOnce
					}
				}
				advOnce = true
			case '{':
				lexer.Advance(false)
				if startQuote == 0 {
					braceDepth++
				}
			case '}':
				lexer.Advance(false)
				if startQuote == 0 {
					braceDepth--
				}
			case '\r', '\t', '\f', '\v', ' ':
				lexer.Advance(false)
			default:
				lexer.Advance(false)
				advOnce = true
			}
		}
		lexer.MarkEnd()
		lexer.SetResultSymbol(syms[bbTokShellContent])
		return advOnce && braceDepth == 0
	}

	return false
}
