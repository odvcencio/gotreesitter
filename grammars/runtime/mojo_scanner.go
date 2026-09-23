//go:build !grammar_subset || grammar_subset_mojo

package grammarruntime

import (
	gotreesitter "github.com/odvcencio/gotreesitter"
)

// External token indexes for the Mojo grammar
// (whistlebee/tree-sitter-mojo, a tree-sitter-python derivative). The order
// matches tree-sitter-python's externals minus the trailing `except`, so the
// token indexes line up one-to-one with python for the first eleven tokens.
// This is the external index (the position of the token in the grammar's
// `externals: [...]` list), which is exactly what tree-sitter's
// `valid_symbols` array and C's result_symbol enum are indexed by. The
// external index is stable across a blob regen as long as the externals
// list itself does not reorder; concrete numeric gotreesitter.Symbol IDs
// are NOT stable (they shift whenever the grammar's total symbol count
// changes), so this scanner never hardcodes them -- see
// mojoDefaultSymTable below.
const (
	mojoTokNewline = iota
	mojoTokIndent
	mojoTokDedent
	mojoTokStringStart
	mojoTokStringContent
	mojoTokEscapeInterpolation
	mojoTokStringEnd
	mojoTokComment
	mojoTokCloseBracket
	mojoTokCloseParen
	mojoTokCloseBrace
	// mojoTokExcept does not exist in the mojo grammar (python's 12th
	// external). validSymbols has only 11 entries, so isValid(mojoTokExcept)
	// is always false and the shared except-handling branch is dead code.
	// It deliberately sits one past mojoTokenCount rather than being folded
	// into it, so it stays out of range of both the raw and the semantic
	// (remapped) validSymbols arrays.
	mojoTokExcept
)

// mojoTokenCount is the number of real externals mojo declares (indices
// 0-10 above); mojoTokExcept (11) is a deliberate always-out-of-range
// sentinel, not a 12th real token.
const mojoTokenCount = 11

// mojoDefaultSymTable records the concrete gotreesitter.Symbol IDs the
// currently shipped mojo.bin assigns to each external, in mojoTok* order.
// It exists only as a pre-bind fallback (and as an independent value to
// compare a real bind against in tests); ExternalScannerForLanguage below
// overwrites it with values read from the actual loaded Language at bind
// time, which is what the scanner must do to survive a future blob regen
// that renumbers absolute symbol IDs without touching the externals list
// order.
var mojoDefaultSymTable = [mojoTokenCount]gotreesitter.Symbol{
	108, // _newline
	109, // _indent
	110, // _dedent
	111, // string_start
	112, // _string_content
	113, // escape_interpolation
	114, // string_end
	105, // comment (never emitted by SetResultSymbol; checked only via isValid)
	12,  // "]" (never emitted by SetResultSymbol; checked only via isValid)
	20,  // ")" (never emitted by SetResultSymbol; checked only via isValid)
	28,  // "}" (never emitted by SetResultSymbol; checked only via isValid)
}

// mojoExternalScannerSpec records the source contract for this hand-written
// port, so updater tooling can tell a grammar-only upstream change apart
// from one that also touches the external scanner or its token list. Its
// Externals list is also the binding source for ExternalScannerForLanguage:
// index i here is scanner token index i (mojoTok* order).
var mojoExternalScannerSpec = ExternalScannerSpec{
	Language:       "mojo",
	UpstreamRepo:   "https://github.com/whistlebee/tree-sitter-mojo",
	UpstreamCommit: "c307dab71a43add26b4715f14e2d6de2a42e6007",
	SourceFiles: []ExternalScannerSourceFile{
		{Path: "src/grammar.json", SHA256: "dfea61526de2ace53d138e452d7394a2913572ce14d11cb58af7a1177c9054e7"},
		{Path: "src/scanner.c", SHA256: "371b36238e083035b0fa9bc7645597996dfb3d7ed98d16f0e5799dbed2006262"},
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
	},
}

func init() {
	RegisterExternalScannerSpec(mojoExternalScannerSpec)
}

// MojoExternalScanner is the tree-sitter-python external scanner retargeted
// at the mojo grammar's symbol IDs. Mojo's scanner.c is byte-for-byte
// python's scanner minus the `except` external, so the state shape and the
// scan logic are shared with PythonExternalScanner.
//
// symbols holds the concrete gotreesitter.Symbol each external index maps to
// in the Language this instance was bound to (see ExternalScannerForLanguage).
// The scanner never hardcodes an absolute Symbol value: a blob regen can
// renumber the grammar's absolute symbol IDs without touching the externals
// list order, and a scanner that still called SetResultSymbol with a stale
// hardcoded ID would silently emit the wrong (but still structurally valid)
// node type instead of failing loudly.
type MojoExternalScanner struct {
	symbols         [mojoTokenCount]gotreesitter.Symbol
	externalToToken []int
}

// ExternalScannerForLanguage binds the scanner's token slots to the loaded
// Language's ExternalSymbols positionally. A hardcoded absolute
// gotreesitter.Symbol constant here would emit the wrong token whenever a
// grammar bump renumbers mojo's external symbols.
func (MojoExternalScanner) ExternalScannerForLanguage(lang *gotreesitter.Language) gotreesitter.ExternalScanner {
	s := MojoExternalScanner{symbols: mojoDefaultSymTable}
	s.externalToToken = bindExternalScannerSpec(lang, mojoExternalScannerSpec, func(tokenIdx int, sym gotreesitter.Symbol) {
		s.symbols[tokenIdx] = sym
	})
	return s
}

func (s MojoExternalScanner) symbolTable() *[mojoTokenCount]gotreesitter.Symbol {
	if s.symbols == ([mojoTokenCount]gotreesitter.Symbol{}) {
		return &mojoDefaultSymTable
	}
	return &s.symbols
}

func (MojoExternalScanner) Create() any {
	return &pythonScannerState{Indents: []uint16{0}}
}

func (MojoExternalScanner) Destroy(payload any) {}

func (MojoExternalScanner) Serialize(payload any, buf []byte) int {
	return serializePythonScannerState(payload.(*pythonScannerState), buf)
}

func (MojoExternalScanner) Deserialize(payload any, buf []byte) {
	deserializePythonScannerState(payload.(*pythonScannerState), buf)
}

// SupportsIncrementalReuse remains disabled with Python's scanner: Mojo shares
// the same serialized indentation state and checkpoint restoration semantics.
// Re-enable only after Mojo's DEDENT behavior is independently certified.
func (MojoExternalScanner) SupportsIncrementalReuse() bool { return false }

func (MojoExternalScanner) UsesExternalScannerCheckpoints() bool { return true }

// ASCII digits share every character branch, including string and comment scans.
func (MojoExternalScanner) ExternalScannerASCIIEquivalenceClass(b byte) uint8 {
	if b >= '0' && b <= '9' {
		return 1
	}
	return 0
}

func (sc MojoExternalScanner) Scan(payload any, lexer *gotreesitter.ExternalLexer, validSymbols []bool) bool {
	syms := sc.symbolTable()

	s := payload.(*pythonScannerState)
	if len(s.Indents) == 0 {
		s.Indents = append(s.Indents, 0)
	}
	s.SyncInsideInterpolatedString()

	isValid := func(idx int) bool {
		return idx >= 0 && idx < len(validSymbols) && validSymbols[idx]
	}

	errorRecoveryMode := isValid(mojoTokStringContent) && isValid(mojoTokIndent)
	withinBrackets := isValid(mojoTokCloseBrace) || isValid(mojoTokCloseParen) || isValid(mojoTokCloseBracket)

	advancedOnce := false
	if isValid(mojoTokEscapeInterpolation) && len(s.Delimiters) > 0 &&
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
				lexer.SetResultSymbol(syms[mojoTokEscapeInterpolation])
				return true
			}
			return false
		}
	}

	if isValid(mojoTokStringContent) && len(s.Delimiters) > 0 && !errorRecoveryMode {
		delimiter := s.Delimiters[len(s.Delimiters)-1]
		EndChar := delimiter.EndChar()
		hasContent := advancedOnce

		for lexer.Lookahead() != 0 {
			if (advancedOnce || lexer.Lookahead() == '{' || lexer.Lookahead() == '}') && delimiter.IsFormat() {
				lexer.MarkEnd()
				lexer.SetResultSymbol(syms[mojoTokStringContent])
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
						lexer.SetResultSymbol(syms[mojoTokStringContent])
						return hasContent
					}
				} else {
					lexer.MarkEnd()
					lexer.SetResultSymbol(syms[mojoTokStringContent])
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
								lexer.SetResultSymbol(syms[mojoTokStringContent])
							} else {
								lexer.Advance(false)
								lexer.MarkEnd()
								s.Delimiters = s.Delimiters[:len(s.Delimiters)-1]
								lexer.SetResultSymbol(syms[mojoTokStringEnd])
								s.InsideInterpolatedString = false
							}
							return true
						}
						lexer.MarkEnd()
						lexer.SetResultSymbol(syms[mojoTokStringContent])
						return true
					}
					lexer.MarkEnd()
					lexer.SetResultSymbol(syms[mojoTokStringContent])
					return true
				}

				if hasContent {
					lexer.SetResultSymbol(syms[mojoTokStringContent])
				} else {
					lexer.Advance(false)
					s.Delimiters = s.Delimiters[:len(s.Delimiters)-1]
					lexer.SetResultSymbol(syms[mojoTokStringEnd])
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
			if isValid(mojoTokIndent) || isValid(mojoTokDedent) || isValid(mojoTokNewline) || isValid(mojoTokExcept) {
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

		if isValid(mojoTokIndent) && indentLength > currentIndent {
			s.Indents = append(s.Indents, indentLength)
			lexer.SetResultSymbol(syms[mojoTokIndent])
			return true
		}

		nextTokIsStringStart := lexer.Lookahead() == '"' || lexer.Lookahead() == '\'' || lexer.Lookahead() == '`'
		if (isValid(mojoTokDedent) ||
			(!isValid(mojoTokNewline) && !(isValid(mojoTokStringStart) && nextTokIsStringStart) && !withinBrackets)) &&
			indentLength < currentIndent &&
			!s.InsideInterpolatedString &&
			firstCommentIndentLength < int32(currentIndent) {
			s.Indents = s.Indents[:len(s.Indents)-1]
			lexer.SetResultSymbol(syms[mojoTokDedent])
			return true
		}

		if isValid(mojoTokNewline) && !errorRecoveryMode {
			lexer.SetResultSymbol(syms[mojoTokNewline])
			return true
		}
	}

	if firstCommentIndentLength == -1 && isValid(mojoTokStringStart) {
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
			lexer.SetResultSymbol(syms[mojoTokStringStart])
			s.InsideInterpolatedString = delimiter.IsFormat()
			return true
		}
		if hasFlags {
			return false
		}
	}

	return false
}
