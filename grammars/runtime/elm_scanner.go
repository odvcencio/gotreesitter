//go:build !grammar_subset || grammar_subset_elm

package grammarruntime

import (
	"unicode"

	gotreesitter "github.com/odvcencio/gotreesitter"
)

// External token indexes for the Elm grammar. This is the external index
// (the position of the token in the grammar's `externals: [...]` list),
// which is exactly what tree-sitter's `valid_symbols` array and C's
// result_symbol enum are indexed by. The external index is stable across a
// blob regen as long as the externals list itself does not reorder;
// concrete numeric gotreesitter.Symbol IDs are NOT stable (they shift
// whenever the grammar's total symbol count changes), so this scanner never
// hardcodes them -- see elmDefaultSymTable below.
const (
	elmTokVirtualEndDecl   = 0
	elmTokVirtualOpenSect  = 1
	elmTokVirtualEndSect   = 2
	elmTokOperatorIdent    = 3 // minus without trailing whitespace
	elmTokGlslContent      = 4
	elmTokBlockCommentBody = 5
	elmTokStringMultiline  = 6
	elmTokenCount          = 7
)

// elmDefaultSymTable records the concrete gotreesitter.Symbol IDs the
// currently shipped elm.bin assigns to each external, in elmTok* order. It
// exists only as a pre-bind fallback (and as an independent value to compare
// a real bind against in tests); ExternalScannerForLanguage below overwrites
// it with values read from the actual loaded Language at bind time, which is
// what the scanner must do to survive a future blob regen that renumbers
// absolute symbol IDs without touching the externals list order.
var elmDefaultSymTable = [elmTokenCount]gotreesitter.Symbol{
	78, // _virtual_end_decl
	79, // _virtual_open_section
	80, // _virtual_end_section
	81, // minus_without_trailing_whitespace (display: operator_identifier)
	82, // glsl_content
	83, // _block_comment_content
	84, // _string_content_multiline (display: regular_string_part)
}

// elmExternalScannerSpec records the source contract for this hand-written
// port, so updater tooling can tell a grammar-only upstream change apart
// from one that also touches the external scanner or its token list. Its
// Externals list is also the binding source for ExternalScannerForLanguage:
// index i here is scanner token index i (elmTok* order).
var elmExternalScannerSpec = ExternalScannerSpec{
	Language:       "elm",
	UpstreamRepo:   "https://github.com/elm-tooling/tree-sitter-elm",
	UpstreamCommit: "e1e8fea161a1e66f3997855d316be2a43e4e956f",
	SourceFiles: []ExternalScannerSourceFile{
		{Path: "src/grammar.json", SHA256: "f7835295067daab21ff1754a7970fb54a1d245c2b0feaacd7989062b8779edc2"},
		{Path: "src/scanner.c", SHA256: "494e93226c0e366d2c1eaefcfd10fc4dfa0d95e87586bcb09c894003b6bb4b99"},
	},
	Externals: []string{
		"_virtual_end_decl",
		"_virtual_open_section",
		"_virtual_end_section",
		"minus_without_trailing_whitespace",
		"glsl_content",
		"_block_comment_content",
		"_string_content_multiline",
	},
}

func init() {
	RegisterExternalScannerSpec(elmExternalScannerSpec)
}

type elmState struct {
	indentLength uint32
	indents      []uint8
	runback      []uint8 // 0 = END_DECL, 1 = END_SECTION
}

// ElmExternalScanner handles indentation-based layout for Elm.
//
// symbols holds the concrete gotreesitter.Symbol each external index maps to
// in the Language this instance was bound to (see ExternalScannerForLanguage).
// The scanner never hardcodes an absolute Symbol value: a blob regen can
// renumber the grammar's absolute symbol IDs without touching the externals
// list order, and a scanner that still called SetResultSymbol with a stale
// hardcoded ID would silently emit the wrong (but still structurally valid)
// node type instead of failing loudly.
type ElmExternalScanner struct {
	symbols         [elmTokenCount]gotreesitter.Symbol
	externalToToken []int
}

// ExternalScannerForLanguage binds the scanner's token slots to the loaded
// Language's ExternalSymbols positionally. A hardcoded absolute
// gotreesitter.Symbol constant here would emit the wrong token whenever a
// grammar bump renumbers elm's external symbols.
func (ElmExternalScanner) ExternalScannerForLanguage(lang *gotreesitter.Language) gotreesitter.ExternalScanner {
	s := ElmExternalScanner{symbols: elmDefaultSymTable}
	s.externalToToken = bindExternalScannerSpec(lang, elmExternalScannerSpec, func(tokenIdx int, sym gotreesitter.Symbol) {
		s.symbols[tokenIdx] = sym
	})
	return s
}

func (s ElmExternalScanner) symbolTable() *[elmTokenCount]gotreesitter.Symbol {
	if s.symbols == ([elmTokenCount]gotreesitter.Symbol{}) {
		return &elmDefaultSymTable
	}
	return &s.symbols
}

func (ElmExternalScanner) Create() any {
	return &elmState{indents: []uint8{0}}
}
func (ElmExternalScanner) Destroy(payload any) {}

func (ElmExternalScanner) Serialize(payload any, buf []byte) int {
	s := payload.(*elmState)
	size := 0

	runbackLen := len(s.runback)
	if runbackLen > 255 {
		runbackLen = 255
	}
	if 3+len(s.indents)+runbackLen >= len(buf) {
		return 0
	}

	buf[size] = byte(runbackLen)
	size++
	for i := 0; i < runbackLen; i++ {
		buf[size] = s.runback[i]
		size++
	}

	// indent_length as 4 bytes little-endian
	buf[size] = 4
	size++
	buf[size] = byte(s.indentLength)
	buf[size+1] = byte(s.indentLength >> 8)
	buf[size+2] = byte(s.indentLength >> 16)
	buf[size+3] = byte(s.indentLength >> 24)
	size += 4

	for i := 1; i < len(s.indents) && size < len(buf); i++ {
		buf[size] = s.indents[i]
		size++
	}

	return size
}

func (ElmExternalScanner) Deserialize(payload any, buf []byte) {
	s := payload.(*elmState)
	s.runback = s.runback[:0]
	s.indents = s.indents[:0]
	s.indents = append(s.indents, 0)
	s.indentLength = 0

	if len(buf) == 0 {
		return
	}

	size := 0
	runbackLen := int(buf[size])
	size++
	for i := 0; i < runbackLen && size < len(buf); i++ {
		s.runback = append(s.runback, buf[size])
		size++
	}

	if size >= len(buf) {
		return
	}
	indentLenLen := int(buf[size])
	size++
	if indentLenLen > 0 && size+indentLenLen <= len(buf) {
		s.indentLength = uint32(buf[size]) |
			uint32(buf[size+1])<<8 |
			uint32(buf[size+2])<<16 |
			uint32(buf[size+3])<<24
		size += indentLenLen
	}

	for ; size < len(buf); size++ {
		s.indents = append(s.indents, buf[size])
	}
}

func (s ElmExternalScanner) Scan(payload any, lexer *gotreesitter.ExternalLexer, validSymbols []bool) bool {
	state := payload.(*elmState)

	if len(s.externalToToken) > 0 {
		var semanticValid [elmTokenCount]bool
		for externalIdx, valid := range validSymbols {
			if !valid || externalIdx >= len(s.externalToToken) {
				continue
			}
			tokenIdx := s.externalToToken[externalIdx]
			if tokenIdx >= 0 && tokenIdx < elmTokenCount {
				semanticValid[tokenIdx] = true
			}
		}
		validSymbols = semanticValid[:]
	}
	syms := s.symbolTable()

	isValid := func(idx int) bool {
		return idx < len(validSymbols) && validSymbols[idx]
	}

	// Error recovery: all tokens valid at once
	if isValid(elmTokVirtualEndDecl) && isValid(elmTokVirtualOpenSect) &&
		isValid(elmTokVirtualEndSect) && isValid(elmTokOperatorIdent) &&
		isValid(elmTokGlslContent) && isValid(elmTokBlockCommentBody) &&
		isValid(elmTokStringMultiline) {
		return false
	}

	// Handle deferred runback tokens
	if len(state.runback) > 0 && state.runback[len(state.runback)-1] == 0 && isValid(elmTokVirtualEndDecl) {
		state.runback = state.runback[:len(state.runback)-1]
		lexer.SetResultSymbol(syms[elmTokVirtualEndDecl])
		return true
	}
	if len(state.runback) > 0 && state.runback[len(state.runback)-1] == 1 && isValid(elmTokVirtualEndSect) {
		state.runback = state.runback[:len(state.runback)-1]
		lexer.SetResultSymbol(syms[elmTokVirtualEndSect])
		return true
	}
	state.runback = state.runback[:0]

	// Multiline string content (triple-quoted)
	if isValid(elmTokStringMultiline) {
		lexer.SetResultSymbol(syms[elmTokStringMultiline])
		hasContent := false
		for lexer.Lookahead() != 0 {
			switch lexer.Lookahead() {
			case '"':
				lexer.MarkEnd()
				lexer.Advance(false)
				if lexer.Lookahead() == '"' {
					lexer.Advance(false)
					if lexer.Lookahead() == '"' {
						return hasContent
					}
					hasContent = true
				} else {
					hasContent = true
				}
			case '\\':
				lexer.MarkEnd()
				return hasContent
			default:
				hasContent = true
				lexer.Advance(false)
			}
		}
		lexer.MarkEnd()
		return hasContent
	}

	// Whitespace/newline/comment scanning
	hasNewline := false
	foundIn := false
	canCallMarkEnd := true
	lexer.MarkEnd()

	for {
		ch := lexer.Lookahead()
		if ch == ' ' || ch == '\r' {
			lexer.Advance(true)
		} else if ch == '\n' {
			lexer.Advance(true)
			hasNewline = true
			for lexer.Lookahead() == ' ' {
				lexer.Advance(true)
			}
			state.indentLength = lexer.Column()
		} else if !isValid(elmTokBlockCommentBody) && ch == '-' {
			lexer.Advance(false)
			la := lexer.Lookahead()

			// Minus without trailing whitespace (negation)
			if isValid(elmTokOperatorIdent) &&
				((la >= 'a' && la <= 'z') || (la >= 'A' && la <= 'Z') || la == '(' || la > 127) {
				if canCallMarkEnd {
					lexer.SetResultSymbol(syms[elmTokOperatorIdent])
					lexer.MarkEnd()
					return true
				}
				return false
			}

			// Line comment: --
			if la == '-' && hasNewline {
				canCallMarkEnd = false
				lexer.Advance(false)
				for lexer.Lookahead() != 0 && lexer.Lookahead() != '\n' {
					lexer.Advance(false)
				}
			} else if isValid(elmTokBlockCommentBody) && la == '}' {
				lexer.SetResultSymbol(syms[elmTokBlockCommentBody])
				return true
			} else {
				return false
			}
		} else if lexer.Lookahead() == 0 { // EOF
			if isValid(elmTokVirtualEndSect) {
				lexer.SetResultSymbol(syms[elmTokVirtualEndSect])
				return true
			}
			if isValid(elmTokVirtualEndDecl) {
				lexer.SetResultSymbol(syms[elmTokVirtualEndDecl])
				return true
			}
			break
		} else {
			break
		}
	}

	// Check for `in` keyword (ends let section)
	if isValid(elmTokVirtualEndSect) && lexer.Lookahead() == 'i' {
		lexer.Advance(true)
		if lexer.Lookahead() == 'n' {
			lexer.Advance(true)
			if elmIsSpace(lexer) || lexer.Lookahead() == 0 {
				if hasNewline {
					foundIn = true
				} else {
					lexer.SetResultSymbol(syms[elmTokVirtualEndSect])
					if len(state.indents) > 0 {
						state.indents = state.indents[:len(state.indents)-1]
					}
					return true
				}
			}
		}
	}

	// Check for section-ending tokens: ), comma, }
	if isValid(elmTokVirtualEndSect) &&
		(lexer.Lookahead() == ')' || lexer.Lookahead() == ',' || lexer.Lookahead() == '}') {
		lexer.SetResultSymbol(syms[elmTokVirtualEndSect])
		if len(state.indents) > 0 {
			state.indents = state.indents[:len(state.indents)-1]
		}
		return true
	}

	// Virtual open section
	if isValid(elmTokVirtualOpenSect) && lexer.Lookahead() != 0 {
		if len(state.indents) >= 256 {
			return false
		}
		state.indents = append(state.indents, uint8(lexer.Column()))
		lexer.SetResultSymbol(syms[elmTokVirtualOpenSect])
		return true
	}

	// Block comment content
	if isValid(elmTokBlockCommentBody) {
		if !canCallMarkEnd {
			return false
		}
		lexer.MarkEnd()
		for lexer.Lookahead() != 0 {
			if lexer.Lookahead() != '{' && lexer.Lookahead() != '-' {
				lexer.Advance(false)
			} else if lexer.Lookahead() == '-' {
				lexer.MarkEnd()
				lexer.Advance(false)
				if lexer.Lookahead() == '}' {
					break
				}
			} else {
				// '{' — might be nested block comment
				if elmScanBlockComment(lexer) {
					lexer.MarkEnd()
				}
			}
		}
		lexer.SetResultSymbol(syms[elmTokBlockCommentBody])
		return true
	}

	// Newline indent handling
	if hasNewline {
		state.runback = state.runback[:0]

		// Skip past block comments that could distort indent measurement
		if lexer.Lookahead() == '{' && !isValid(elmTokBlockCommentBody) &&
			len(state.indents) > 0 && state.indentLength < uint32(state.indents[len(state.indents)-1]) {
			lexer.Advance(false)
			if lexer.Lookahead() == '-' {
				canCallMarkEnd = false
				lexer.Advance(false)
				elmSkipBlockComment(lexer)
				elmSkipWhitespaceAndRemeasure(lexer, state)
				// Check for additional block comments
				for lexer.Lookahead() == '{' {
					lexer.Advance(false)
					if lexer.Lookahead() == '-' {
						lexer.Advance(false)
						elmSkipBlockComment(lexer)
						elmSkipWhitespaceAndRemeasure(lexer, state)
					} else {
						break
					}
				}
			}
		}

		for len(state.indents) > 0 && state.indentLength <= uint32(state.indents[len(state.indents)-1]) {
			if state.indentLength == uint32(state.indents[len(state.indents)-1]) {
				if foundIn {
					state.indents = state.indents[:len(state.indents)-1]
					state.runback = append(state.runback, 1)
					foundIn = false
					break
				}
				// Don't insert END_DECL before line or block comment
				if lexer.Lookahead() == '-' {
					lexer.Advance(true)
					if lexer.Lookahead() == '-' {
						break
					}
				}
				if lexer.Lookahead() == '{' {
					lexer.Advance(true)
					if lexer.Lookahead() == '-' {
						break
					}
				}
				state.runback = append(state.runback, 0)
				break
			}
			if state.indentLength < uint32(state.indents[len(state.indents)-1]) {
				state.indents = state.indents[:len(state.indents)-1]
				state.runback = append(state.runback, 1)
				if foundIn && (len(state.indents) == 0 ||
					state.indentLength > uint32(state.indents[len(state.indents)-1])) {
					foundIn = false
				}
			}
		}

		if foundIn {
			if len(state.indents) > 0 {
				state.indents = state.indents[:len(state.indents)-1]
			}
			state.runback = append(state.runback, 1)
		}

		// Reverse runback so we pop from the end
		for i, j := 0, len(state.runback)-1; i < j; i, j = i+1, j-1 {
			state.runback[i], state.runback[j] = state.runback[j], state.runback[i]
		}

		if len(state.runback) > 0 && state.runback[len(state.runback)-1] == 0 && isValid(elmTokVirtualEndDecl) {
			state.runback = state.runback[:len(state.runback)-1]
			lexer.SetResultSymbol(syms[elmTokVirtualEndDecl])
			return true
		}
		if len(state.runback) > 0 && state.runback[len(state.runback)-1] == 1 && isValid(elmTokVirtualEndSect) {
			state.runback = state.runback[:len(state.runback)-1]
			lexer.SetResultSymbol(syms[elmTokVirtualEndSect])
			return true
		}
		if lexer.Lookahead() == 0 && isValid(elmTokVirtualEndSect) {
			lexer.SetResultSymbol(syms[elmTokVirtualEndSect])
			return true
		}
	}

	// GLSL content: scan until |]
	if isValid(elmTokGlslContent) {
		if !canCallMarkEnd {
			return false
		}
		lexer.SetResultSymbol(syms[elmTokGlslContent])
		for lexer.Lookahead() != 0 {
			if lexer.Lookahead() == '|' {
				lexer.MarkEnd()
				lexer.Advance(false)
				if lexer.Lookahead() == ']' {
					lexer.Advance(false)
					return true
				}
			} else {
				lexer.Advance(false)
			}
		}
		lexer.MarkEnd()
		return true
	}

	return false
}

func elmIsSpace(lexer *gotreesitter.ExternalLexer) bool {
	ch := lexer.Lookahead()
	return ch == ' ' || ch == '\r' || ch == '\n'
}

func elmScanBlockComment(lexer *gotreesitter.ExternalLexer) bool {
	lexer.MarkEnd()
	if lexer.Lookahead() != '{' {
		return false
	}
	lexer.Advance(false)
	if lexer.Lookahead() != '-' {
		return false
	}
	lexer.Advance(false)
	for lexer.Lookahead() != 0 {
		switch lexer.Lookahead() {
		case '{':
			elmScanBlockComment(lexer)
		case '-':
			lexer.Advance(false)
			if lexer.Lookahead() == '}' {
				lexer.Advance(false)
				return true
			}
		default:
			lexer.Advance(false)
		}
	}
	return true
}

func elmSkipBlockComment(lexer *gotreesitter.ExternalLexer) {
	nesting := 1
	for nesting > 0 && lexer.Lookahead() != 0 {
		if lexer.Lookahead() == '{' {
			lexer.Advance(false)
			if lexer.Lookahead() == '-' {
				lexer.Advance(false)
				nesting++
			}
		} else if lexer.Lookahead() == '-' {
			lexer.Advance(false)
			if lexer.Lookahead() == '}' {
				lexer.Advance(false)
				nesting--
			}
		} else {
			lexer.Advance(false)
		}
	}
}

func elmSkipWhitespaceAndRemeasure(lexer *gotreesitter.ExternalLexer, s *elmState) {
	for unicode.IsSpace(lexer.Lookahead()) {
		if lexer.Lookahead() == '\n' {
			lexer.Advance(false)
			for lexer.Lookahead() == ' ' {
				lexer.Advance(false)
			}
			s.indentLength = lexer.Column()
		} else {
			lexer.Advance(false)
		}
	}
}
