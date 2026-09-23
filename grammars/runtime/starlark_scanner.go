//go:build !grammar_subset || grammar_subset_starlark

package grammarruntime

import (
	gotreesitter "github.com/odvcencio/gotreesitter"
)

// External token indexes for starlark — same layout as Python.
const (
	slTokNewline = iota
	slTokIndent
	slTokDedent
	slTokStringStart
	slTokStringContent
	slTokEscapeInterpolation
	slTokStringEnd
	slTokComment
	slTokCloseBracket
	slTokCloseParen
	slTokCloseBrace
	slTokExcept
	slTokenCount = 12
)

// slDefaultSymTable records the concrete gotreesitter.Symbol IDs the
// currently shipped starlark.bin assigns to each external, in slTok* order.
// It exists only as a pre-bind fallback (and as an independent value to
// compare a real bind against in tests); ExternalScannerForLanguage below
// overwrites it with values read from the actual loaded Language at bind
// time, which is what the scanner must do to survive a future blob regen
// that renumbers absolute symbol IDs without touching the externals list
// order.
var slDefaultSymTable = [slTokenCount]gotreesitter.Symbol{
	99,  // _newline
	100, // _indent
	101, // _dedent
	102, // string_start
	103, // _string_content
	104, // escape_interpolation
	105, // string_end
	96,  // comment
	42,  // ]
	35,  // )
	49,  // }
	98,  // except
}

// slExternalScannerSpec records the source contract for this
// hand-written port, so updater tooling can tell a grammar-only upstream
// change apart from one that also touches the external scanner or its
// token list. Its Externals list is also the binding source for
// ExternalScannerForLanguage: index i here is scanner token index i
// (slTok* order).
var slExternalScannerSpec = ExternalScannerSpec{
	Language:       "starlark",
	UpstreamRepo:   "https://github.com/tree-sitter-grammars/tree-sitter-starlark",
	UpstreamCommit: "a453dbf3ba433db0e5ec621a38a7e59d72e4dc69",
	SourceFiles: []ExternalScannerSourceFile{
		{Path: "src/grammar.json", SHA256: "409fe5144492738e7841dc470e31281e6b6be7b57db04431f5144092db894c9e"},
		{Path: "src/scanner.c", SHA256: "c3032b6d5ef796cd7bfe518982c9ca1254fbe79b3b20d0f2eb070318e22d61be"},
	},
	Externals: []string{
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
		"except",
	},
}

func init() {
	RegisterExternalScannerSpec(slExternalScannerSpec)
}

// StarlarkExternalScanner handles indent/dedent and string literals for Starlark.
// Starlark is essentially Python syntax; this reuses the pythonScannerState type.
//
// symbols holds the concrete gotreesitter.Symbol each external index maps to
// in the Language this instance was bound to (see ExternalScannerForLanguage).
// The scanner never hardcodes an absolute Symbol value: a blob regen can
// renumber the grammar's absolute symbol IDs without touching the externals
// list order, and a scanner that still called SetResultSymbol with a stale
// hardcoded ID would silently emit the wrong (but still structurally valid)
// node type instead of failing loudly.
type StarlarkExternalScanner struct {
	symbols         [slTokenCount]gotreesitter.Symbol
	externalToToken []int
}

// ExternalScannerForLanguage binds the scanner's token slots to the loaded
// Language's ExternalSymbols positionally. A hardcoded absolute
// gotreesitter.Symbol constant here would emit the wrong token whenever a
// grammar bump renumbers starlark's external symbols.
func (StarlarkExternalScanner) ExternalScannerForLanguage(lang *gotreesitter.Language) gotreesitter.ExternalScanner {
	s := StarlarkExternalScanner{symbols: slDefaultSymTable}
	s.externalToToken = bindExternalScannerSpec(lang, slExternalScannerSpec, func(tokenIdx int, sym gotreesitter.Symbol) {
		s.symbols[tokenIdx] = sym
	})
	return s
}

func (s StarlarkExternalScanner) symbolTable() *[slTokenCount]gotreesitter.Symbol {
	if s.symbols == ([slTokenCount]gotreesitter.Symbol{}) {
		return &slDefaultSymTable
	}
	return &s.symbols
}

func (StarlarkExternalScanner) Create() any {
	return &pythonScannerState{Indents: []uint16{0}}
}

func (StarlarkExternalScanner) Destroy(payload any) {}

func (StarlarkExternalScanner) Serialize(payload any, buf []byte) int {
	return serializePythonScannerState(payload.(*pythonScannerState), buf)
}

func (StarlarkExternalScanner) Deserialize(payload any, buf []byte) {
	deserializePythonScannerState(payload.(*pythonScannerState), buf)
}

// SupportsIncrementalReuse remains disabled with Python's scanner: Starlark
// shares the same serialized indentation state and checkpoint restoration
// semantics. Re-enable only after Starlark's DEDENT behavior is independently
// certified.
func (StarlarkExternalScanner) SupportsIncrementalReuse() bool { return false }

func (StarlarkExternalScanner) UsesExternalScannerCheckpoints() bool { return true }

// ASCII digits share every character branch, including string and comment scans.
func (StarlarkExternalScanner) ExternalScannerASCIIEquivalenceClass(b byte) uint8 {
	if b >= '0' && b <= '9' {
		return 1
	}
	return 0
}

func (sc StarlarkExternalScanner) Scan(payload any, lexer *gotreesitter.ExternalLexer, validSymbols []bool) bool {
	if len(sc.externalToToken) > 0 {
		var semanticValid [slTokenCount]bool
		for externalIdx, valid := range validSymbols {
			if !valid || externalIdx >= len(sc.externalToToken) {
				continue
			}
			tokenIdx := sc.externalToToken[externalIdx]
			if tokenIdx >= 0 && tokenIdx < slTokenCount {
				semanticValid[tokenIdx] = true
			}
		}
		validSymbols = semanticValid[:]
	}
	syms := sc.symbolTable()

	s := payload.(*pythonScannerState)
	if len(s.Indents) == 0 {
		s.Indents = append(s.Indents, 0)
	}

	isValid := func(idx int) bool {
		return idx >= 0 && idx < len(validSymbols) && validSymbols[idx]
	}

	errorRecoveryMode := isValid(slTokStringContent) && isValid(slTokIndent)
	withinBrackets := isValid(slTokCloseBrace) || isValid(slTokCloseParen) || isValid(slTokCloseBracket)

	advancedOnce := false
	if isValid(slTokEscapeInterpolation) && len(s.Delimiters) > 0 &&
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
				lexer.SetResultSymbol(syms[slTokEscapeInterpolation])
				return true
			}
			return false
		}
	}

	if isValid(slTokStringContent) && len(s.Delimiters) > 0 && !errorRecoveryMode {
		delimiter := s.Delimiters[len(s.Delimiters)-1]
		EndChar := delimiter.EndChar()
		hasContent := advancedOnce

		for lexer.Lookahead() != 0 {
			if (advancedOnce || lexer.Lookahead() == '{' || lexer.Lookahead() == '}') && delimiter.IsFormat() {
				lexer.MarkEnd()
				lexer.SetResultSymbol(syms[slTokStringContent])
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
						lexer.SetResultSymbol(syms[slTokStringContent])
						return hasContent
					}
				} else {
					lexer.MarkEnd()
					lexer.SetResultSymbol(syms[slTokStringContent])
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
								lexer.SetResultSymbol(syms[slTokStringContent])
							} else {
								lexer.Advance(false)
								lexer.MarkEnd()
								s.Delimiters = s.Delimiters[:len(s.Delimiters)-1]
								lexer.SetResultSymbol(syms[slTokStringEnd])
								s.InsideInterpolatedString = false
							}
							return true
						}
						lexer.MarkEnd()
						lexer.SetResultSymbol(syms[slTokStringContent])
						return true
					}
					lexer.MarkEnd()
					lexer.SetResultSymbol(syms[slTokStringContent])
					return true
				}

				if hasContent {
					lexer.SetResultSymbol(syms[slTokStringContent])
				} else {
					lexer.Advance(false)
					s.Delimiters = s.Delimiters[:len(s.Delimiters)-1]
					lexer.SetResultSymbol(syms[slTokStringEnd])
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
			indentLength++
			lexer.Advance(true)
		case '\r', '\f':
			indentLength = 0
			lexer.Advance(true)
		case '\t':
			indentLength += 8
			lexer.Advance(true)
		case '#':
			if isValid(slTokIndent) || isValid(slTokDedent) || isValid(slTokNewline) || isValid(slTokExcept) {
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
			}
			goto slAfterIndentLoop
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
			goto slAfterIndentLoop
		default:
			goto slAfterIndentLoop
		}
	}

slAfterIndentLoop:
	if foundEndOfLine {
		currentIndent := s.Indents[len(s.Indents)-1]

		if isValid(slTokIndent) && indentLength > currentIndent {
			s.Indents = append(s.Indents, indentLength)
			lexer.SetResultSymbol(syms[slTokIndent])
			return true
		}

		nextTokIsStringStart := lexer.Lookahead() == '"' || lexer.Lookahead() == '\'' || lexer.Lookahead() == '`'
		if (isValid(slTokDedent) ||
			(!isValid(slTokNewline) && !(isValid(slTokStringStart) && nextTokIsStringStart) && !withinBrackets)) &&
			indentLength < currentIndent &&
			!s.InsideInterpolatedString &&
			firstCommentIndentLength < int32(currentIndent) {
			s.Indents = s.Indents[:len(s.Indents)-1]
			lexer.SetResultSymbol(syms[slTokDedent])
			return true
		}

		if isValid(slTokNewline) && !errorRecoveryMode {
			lexer.SetResultSymbol(syms[slTokNewline])
			return true
		}
	}

	if firstCommentIndentLength == -1 && isValid(slTokStringStart) {
		var delimiter pyDelimiter
		hasFlags := false

		for lexer.Lookahead() != 0 {
			switch lexer.Lookahead() {
			case 'f', 'F', 't', 'T':
				delimiter |= pyDelimFormat
			case 'r', 'R':
				delimiter |= pyDelimRaw
			case 'b', 'B':
				delimiter |= pyDelimBytes
			case 'u', 'U':
				// accepted prefix, no flag
			default:
				goto slAfterFlags
			}
			hasFlags = true
			lexer.Advance(false)
		}

	slAfterFlags:
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
			lexer.SetResultSymbol(syms[slTokStringStart])
			s.InsideInterpolatedString = delimiter.IsFormat()
			return true
		}
		if hasFlags {
			return false
		}
	}

	return false
}
