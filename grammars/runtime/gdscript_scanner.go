//go:build !grammar_subset || grammar_subset_gdscript

package grammarruntime

import (
	gotreesitter "github.com/odvcencio/gotreesitter"
)

// External token indexes for the GDScript grammar. This is the external
// index (the position of the token in the grammar's `externals: [...]`
// list), which is exactly what tree-sitter's `valid_symbols` array and
// C's result_symbol enum are indexed by. The external index is stable
// across a blob regen as long as the externals list itself does not
// reorder; concrete numeric gotreesitter.Symbol IDs are NOT stable (they
// shift whenever the grammar's total symbol count changes), so this
// scanner never hardcodes them -- see gdsDefaultSymTable below.
//
// gdsTokCloseParen and gdsTokCloseBracket disagree with the upstream
// literal at the same index (index 8 is "]", index 9 is ")", the names
// here are swapped); this is a pre-existing naming quirk in this hand
// port, harmless because both are only used in an OR/AND gate together
// with gdsTokCloseBrace and gdsTokComma, never individually emitted.
const (
	gdsTokNewline = iota
	gdsTokIndent
	gdsTokDedent
	gdsTokStringStart
	gdsTokStringContent
	gdsTokStringEnd
	gdsTokStringNameStart
	gdsTokNodePathStart
	gdsTokCloseParen
	gdsTokCloseBracket
	gdsTokCloseBrace
	gdsTokComma
	gdsTokBodyEnd
	gdsTokenCount
)

// gdsDefaultSymTable records the concrete gotreesitter.Symbol IDs the
// currently shipped gdscript.bin assigns to each external, in gdsTok*
// order. It exists only as a pre-bind fallback (and as an independent
// value to compare a real bind against in tests); ExternalScannerForLanguage
// below overwrites it with values read from the actual loaded Language at
// bind time, which is what the scanner must do to survive a future blob
// regen that renumbers absolute symbol IDs without touching the externals
// list order.
var gdsDefaultSymTable = [gdsTokenCount]gotreesitter.Symbol{
	102, // _newline
	103, // _indent
	104, // _dedent
	105, // _string_start, displays as "\""
	106, // _string_content
	107, // _string_end, displays as "\""
	108, // _string_name_start, displays as "&\""
	109, // _node_path_start, displays as "^\""
	81,  // "]", shared literal symbol
	84,  // ")", shared literal symbol
	52,  // "}", shared literal symbol
	29,  // ",", shared literal symbol
	110, // _body_end
}

// gdsExternalScannerSpec records the source contract for this
// hand-written port, so updater tooling can tell a grammar-only upstream
// change apart from one that also touches the external scanner or its
// token list. Its Externals list is also the binding source for
// ExternalScannerForLanguage: index i here is scanner token index i
// (gdsTok* order).
var gdsExternalScannerSpec = ExternalScannerSpec{
	Language:       "gdscript",
	UpstreamRepo:   "https://github.com/PrestonKnopp/tree-sitter-gdscript",
	UpstreamCommit: "89e66b6bdc002ab976283f277cbb48b780c5d0e9",
	SourceFiles: []ExternalScannerSourceFile{
		{Path: "src/grammar.json", SHA256: "e4ead30de233febe95fc133981efa9a013d4f1f54c251ead2101ce8fd7ced263"},
		{Path: "src/scanner.c", SHA256: "175188924474f3265c49ac364695036136eb69ced21ac5e6a23e3a9d9c4a2e75"},
	},
	Externals: []string{
		"_newline",
		"_indent",
		"_dedent",
		"_string_start",
		"_string_content",
		"_string_end",
		"_string_name_start",
		"_node_path_start",
		"]",
		")",
		"}",
		",",
		"_body_end",
	},
}

func init() {
	RegisterExternalScannerSpec(gdsExternalScannerSpec)
}

type gdsDelimiter byte

const (
	gdsDelimSingleQuote gdsDelimiter = 1 << 0
	gdsDelimDoubleQuote gdsDelimiter = 1 << 1
	gdsDelimTriple      gdsDelimiter = 1 << 2
	gdsDelimRaw         gdsDelimiter = 1 << 3
	gdsDelimName        gdsDelimiter = 1 << 4
	gdsDelimNodePath    gdsDelimiter = 1 << 5
)

func (d gdsDelimiter) endChar() rune {
	if d&gdsDelimSingleQuote != 0 {
		return '\''
	}
	if d&gdsDelimDoubleQuote != 0 {
		return '"'
	}
	return 0
}
func (d gdsDelimiter) isTriple() bool   { return d&gdsDelimTriple != 0 }
func (d gdsDelimiter) isRaw() bool      { return d&gdsDelimRaw != 0 }
func (d gdsDelimiter) isName() bool     { return d&gdsDelimName != 0 }
func (d gdsDelimiter) isNodePath() bool { return d&gdsDelimNodePath != 0 }

type gdscriptState struct {
	indents    []uint16
	delimiters []gdsDelimiter
}

// GdscriptExternalScanner handles indent/dedent, strings, and body_end for GDScript.
//
// symbols holds the concrete gotreesitter.Symbol each external index maps to
// in the Language this instance was bound to (see ExternalScannerForLanguage).
// The scanner never hardcodes an absolute Symbol value: a blob regen can
// renumber the grammar's absolute symbol IDs without touching the externals
// list order, and a scanner that still called SetResultSymbol with a stale
// hardcoded ID would silently emit the wrong (but still structurally valid)
// node type instead of failing loudly.
type GdscriptExternalScanner struct {
	symbols         [gdsTokenCount]gotreesitter.Symbol
	externalToToken []int
}

// ExternalScannerForLanguage binds the scanner's token slots to the loaded
// Language's ExternalSymbols positionally. A hardcoded absolute
// gotreesitter.Symbol constant here would emit the wrong token whenever a
// grammar bump renumbers gdscript's external symbols.
func (GdscriptExternalScanner) ExternalScannerForLanguage(lang *gotreesitter.Language) gotreesitter.ExternalScanner {
	s := GdscriptExternalScanner{symbols: gdsDefaultSymTable}
	s.externalToToken = bindExternalScannerSpec(lang, gdsExternalScannerSpec, func(tokenIdx int, sym gotreesitter.Symbol) {
		s.symbols[tokenIdx] = sym
	})
	return s
}

func (s GdscriptExternalScanner) symbolTable() *[gdsTokenCount]gotreesitter.Symbol {
	if s.symbols == ([gdsTokenCount]gotreesitter.Symbol{}) {
		return &gdsDefaultSymTable
	}
	return &s.symbols
}

func (GdscriptExternalScanner) Create() any {
	return &gdscriptState{indents: []uint16{0}}
}
func (GdscriptExternalScanner) Destroy(payload any) {}

func (GdscriptExternalScanner) Serialize(payload any, buf []byte) int {
	s := payload.(*gdscriptState)
	if len(buf) == 0 {
		return 0
	}
	size := 0
	delimCount := len(s.delimiters)
	if delimCount > 255 {
		delimCount = 255
	}
	buf[size] = byte(delimCount)
	size++
	for i := 0; i < delimCount && size < len(buf); i++ {
		buf[size] = byte(s.delimiters[i])
		size++
	}
	for i := 1; i < len(s.indents) && size < len(buf); i++ {
		buf[size] = byte(s.indents[i])
		size++
	}
	return size
}

func (GdscriptExternalScanner) Deserialize(payload any, buf []byte) {
	s := payload.(*gdscriptState)
	s.delimiters = s.delimiters[:0]
	s.indents = s.indents[:0]
	s.indents = append(s.indents, 0)
	if len(buf) == 0 {
		return
	}
	size := 0
	delimCount := int(buf[size])
	size++
	for i := 0; i < delimCount && size < len(buf); i++ {
		s.delimiters = append(s.delimiters, gdsDelimiter(buf[size]))
		size++
	}
	for ; size < len(buf); size++ {
		s.indents = append(s.indents, uint16(buf[size]))
	}
}

func (sc GdscriptExternalScanner) Scan(payload any, lexer *gotreesitter.ExternalLexer, validSymbols []bool) bool {
	s := payload.(*gdscriptState)
	if len(s.indents) == 0 {
		s.indents = append(s.indents, 0)
	}

	if len(sc.externalToToken) > 0 {
		var semanticValid [gdsTokenCount]bool
		for externalIdx, valid := range validSymbols {
			if !valid || externalIdx >= len(sc.externalToToken) {
				continue
			}
			tokenIdx := sc.externalToToken[externalIdx]
			if tokenIdx >= 0 && tokenIdx < gdsTokenCount {
				semanticValid[tokenIdx] = true
			}
		}
		validSymbols = semanticValid[:]
	}
	syms := sc.symbolTable()

	isValid := func(idx int) bool {
		return idx < len(validSymbols) && validSymbols[idx]
	}

	errorRecoveryMode := isValid(gdsTokStringContent) && isValid(gdsTokIndent)

	// ---- String content scanning ----
	if isValid(gdsTokStringContent) && len(s.delimiters) > 0 && !errorRecoveryMode {
		delimiter := s.delimiters[len(s.delimiters)-1]
		endCh := delimiter.endChar()
		hasContent := false
		for lexer.Lookahead() != 0 {
			if lexer.Lookahead() == '\\' {
				if delimiter.isRaw() {
					lexer.Advance(false)
					if lexer.Lookahead() == endCh || lexer.Lookahead() == '\\' {
						lexer.Advance(false)
					}
					continue
				}
				lexer.MarkEnd()
				lexer.SetResultSymbol(syms[gdsTokStringContent])
				return hasContent
			} else if lexer.Lookahead() == endCh {
				if delimiter.isTriple() {
					lexer.MarkEnd()
					lexer.Advance(false)
					if lexer.Lookahead() == endCh {
						lexer.Advance(false)
						if lexer.Lookahead() == endCh {
							if hasContent {
								lexer.SetResultSymbol(syms[gdsTokStringContent])
							} else {
								lexer.Advance(false)
								lexer.MarkEnd()
								s.delimiters = s.delimiters[:len(s.delimiters)-1]
								lexer.SetResultSymbol(syms[gdsTokStringEnd])
							}
							return true
						}
						lexer.MarkEnd()
						lexer.SetResultSymbol(syms[gdsTokStringContent])
						return true
					}
					lexer.MarkEnd()
					lexer.SetResultSymbol(syms[gdsTokStringContent])
					return true
				}
				if hasContent {
					lexer.SetResultSymbol(syms[gdsTokStringContent])
				} else {
					lexer.Advance(false)
					s.delimiters = s.delimiters[:len(s.delimiters)-1]
					lexer.SetResultSymbol(syms[gdsTokStringEnd])
				}
				lexer.MarkEnd()
				return true
			}
			lexer.Advance(false)
			hasContent = true
		}
	}

	lexer.MarkEnd()

	foundEndOfLine := false
	var indentLength uint32
	var lastNonEmptyIndent uint32

	for {
		ch := lexer.Lookahead()
		switch {
		case ch == '\n':
			foundEndOfLine = true
			indentLength = 0
			lexer.Advance(true)
		case ch == ' ':
			indentLength++
			lexer.Advance(true)
		case ch == '\r' || ch == '\f':
			indentLength = 0
			lexer.Advance(true)
		case ch == '\t':
			indentLength += 8
			lexer.Advance(true)
		case ch == '#':
			if !foundEndOfLine {
				goto afterIndentLoop
			}
			lastNonEmptyIndent = indentLength

			commentIndent := indentLength
			isRegion := false
			if commentIndent == 0 {
				lexer.Advance(true) // skip #
				if lexer.Lookahead() == 'r' {
					isRegion = gdsLookaheadString(lexer, "region")
				} else if lexer.Lookahead() == 'e' {
					isRegion = gdsLookaheadString(lexer, "endregion")
				}
			}

			if !isRegion && len(s.indents) > 1 && commentIndent == 0 {
				funcIndent := s.indents[1]
				if funcIndent > 0 {
					for lexer.Lookahead() != 0 && lexer.Lookahead() != '\n' {
						lexer.Advance(true)
					}
					if lexer.Lookahead() == '\n' {
						lexer.Advance(true)
					}
					var nextIndent uint32
					for gdsSkipWS(lexer, &nextIndent) {
					}
					if nextIndent > 0 {
						indentLength = uint32(funcIndent)
					}
				}
			}

			goto afterIndentLoop

		case ch == '\\':
			lexer.Advance(true)
			if lexer.Lookahead() == '\r' {
				lexer.Advance(true)
			}
			if lexer.Lookahead() == '\n' || lexer.Lookahead() == 0 {
				lexer.Advance(true)
			} else {
				return false
			}
		case ch == 0: // EOF
			if lastNonEmptyIndent > 0 {
				indentLength = lastNonEmptyIndent
			}
			if len(s.indents) > 0 {
				if indentLength != uint32(s.indents[len(s.indents)-1]) {
					indentLength = 0
				}
			} else {
				indentLength = 0
			}
			foundEndOfLine = true
			goto afterIndentLoop
		default:
			if indentLength == 0 && lastNonEmptyIndent > 0 && len(s.indents) > 0 {
				if lastNonEmptyIndent == uint32(s.indents[len(s.indents)-1]) {
					return false
				}
			}
			goto afterIndentLoop
		}
	}

afterIndentLoop:
	if foundEndOfLine {
		if len(s.indents) > 0 {
			current := s.indents[len(s.indents)-1]
			if isValid(gdsTokIndent) && indentLength > uint32(current) {
				s.indents = append(s.indents, uint16(indentLength))
				lexer.SetResultSymbol(syms[gdsTokIndent])
				return true
			}
			if isValid(gdsTokDedent) && indentLength < uint32(current) {
				s.indents = s.indents[:len(s.indents)-1]
				lexer.SetResultSymbol(syms[gdsTokDedent])
				return true
			}
		}
		if isValid(gdsTokNewline) && !errorRecoveryMode {
			lexer.SetResultSymbol(syms[gdsTokNewline])
			return true
		}
	}

	// BODY_END — fires when a closing bracket/paren/brace/comma is seen
	// but the grammar doesn't expect the specific bracket token.
	if !isValid(gdsTokComma) &&
		!isValid(gdsTokCloseParen) && !isValid(gdsTokCloseBrace) &&
		!isValid(gdsTokCloseBracket) &&
		(errorRecoveryMode || isValid(gdsTokBodyEnd)) {
		ch := lexer.Lookahead()
		if ch == ',' || ch == ')' || ch == '}' || ch == ']' {
			if isValid(gdsTokDedent) && len(s.indents) > 0 {
				s.indents = s.indents[:len(s.indents)-1]
			}
			lexer.SetResultSymbol(syms[gdsTokBodyEnd])
			return true
		}
	}

	// String / name / node-path start
	if isValid(gdsTokStringStart) || isValid(gdsTokStringNameStart) || isValid(gdsTokNodePathStart) {
		var delimiter gdsDelimiter
		hasFlags := true

		switch lexer.Lookahead() {
		case 'r':
			delimiter |= gdsDelimRaw
		case '&':
			delimiter |= gdsDelimName
		case '^', '@':
			delimiter |= gdsDelimNodePath
		default:
			hasFlags = false
		}

		if hasFlags {
			lexer.Advance(false)
		}

		if lexer.Lookahead() == '\'' || lexer.Lookahead() == '"' {
			gdsHandleQuote(lexer, &delimiter)
		}

		if delimiter.endChar() != 0 {
			s.delimiters = append(s.delimiters, delimiter)
			if delimiter.isNodePath() {
				lexer.SetResultSymbol(syms[gdsTokNodePathStart])
			} else if delimiter.isName() {
				lexer.SetResultSymbol(syms[gdsTokStringNameStart])
			} else {
				lexer.SetResultSymbol(syms[gdsTokStringStart])
			}
			return true
		}

		if hasFlags {
			return false
		}
	}

	return false
}

func gdsHandleQuote(lexer *gotreesitter.ExternalLexer, delimiter *gdsDelimiter) {
	quote := lexer.Lookahead()
	if quote == '\'' {
		*delimiter |= gdsDelimSingleQuote
	} else {
		*delimiter |= gdsDelimDoubleQuote
	}
	lexer.Advance(false)
	lexer.MarkEnd()
	if lexer.Lookahead() == quote {
		lexer.Advance(false)
		if lexer.Lookahead() == quote {
			lexer.Advance(false)
			lexer.MarkEnd()
			*delimiter |= gdsDelimTriple
		}
	}
}

func gdsSkipWS(lexer *gotreesitter.ExternalLexer, indent *uint32) bool {
	switch lexer.Lookahead() {
	case '\n':
		*indent = 0
		lexer.Advance(true)
		return true
	case ' ':
		*indent++
		lexer.Advance(true)
		return true
	case '\r', '\f':
		*indent = 0
		lexer.Advance(true)
		return true
	case '\t':
		*indent += 8
		lexer.Advance(true)
		return true
	}
	return false
}

func gdsLookaheadString(lexer *gotreesitter.ExternalLexer, word string) bool {
	for i := 0; i < len(word); i++ {
		if lexer.Lookahead() != rune(word[i]) {
			return false
		}
		lexer.Advance(true)
	}
	return true
}
