//go:build !grammar_subset || grammar_subset_ocaml

package grammarruntime

import (
	"unicode"

	gotreesitter "github.com/odvcencio/gotreesitter"
)

// External token indexes for the ocaml grammar. These are scanner-internal
// slot indexes, in the same order as tree-sitter-ocaml's grammar.json
// "externals" array.
const (
	ocamlTokComment              = iota // "comment"
	ocamlTokLeftQuotedStringDel         // "_left_quoted_string_delimiter"
	ocamlTokRightQuotedStringDel        // "_right_quoted_string_delimiter"
	ocamlTokStringDelim                 // "\""
	ocamlTokLineNumberDirective         // "line_number_directive"
	ocamlTokNull                        // "_null"
	ocamlTokErrorSentinel               // "_error_sentinel"
	ocamlTokenCount                     // sentinel
)

// ocamlDefaultSymTable holds the concrete symbol IDs for the ocaml grammar
// blob pinned in grammars/languages.lock. It is a fallback default only: a
// scanner bound to a specific *gotreesitter.Language through
// ExternalScannerForLanguage always uses that Language's own ExternalSymbols,
// read positionally through bindExternalScannerSpec. Grammar symbol IDs shift
// whenever the pinned blob regenerates, so a hardcoded absolute ID used
// directly (instead of through this per-instance binding) silently mismatches
// the next time the grammar's rule set changes shape.
var ocamlDefaultSymTable = [ocamlTokenCount]gotreesitter.Symbol{
	165, // comment
	166, // _left_quoted_string_delimiter
	167, // _right_quoted_string_delimiter
	116, // "\""
	168, // line_number_directive
	169, // _null
	170, // _error_sentinel
}

// ocamlExternalScannerSpec records the upstream scanner-source contract this
// port tracks. common/scanner.h holds the real scanner logic; each
// per-grammar scanner.c (including grammars/ocaml/src/scanner.c) is a thin
// shim that includes it.
var ocamlExternalScannerSpec = ExternalScannerSpec{
	Language:       "ocaml",
	UpstreamRepo:   "https://github.com/tree-sitter/tree-sitter-ocaml",
	UpstreamCommit: "3b2e14e0697d405c9aa0beddfa09b71f45abc504",
	SourceFiles: []ExternalScannerSourceFile{
		{Path: "grammars/ocaml/src/grammar.json", SHA256: "f6bc047d8cf03c469da4cadafdec88148034d06eecd698b2fa00f33bc301d97a"},
		{Path: "grammars/ocaml/src/scanner.c", SHA256: "fa23af8a5db2aaaf7041b60f7e762c23f1b5950388a592e2bc4dcffaffa75f5b"},
		{Path: "common/scanner.h", SHA256: "6fbc07429a755a9372cddd11b64754f6eaf8cca70ecd5d0cfe265d0149388244"},
	},
	Externals: []string{
		"comment",
		"_left_quoted_string_delimiter",
		"_right_quoted_string_delimiter",
		"\"",
		"line_number_directive",
		"_null",
		"_error_sentinel",
	},
}

func init() {
	RegisterExternalScannerSpec(ocamlExternalScannerSpec)
}

// ocamlScannerState tracks whether the scanner is inside a string and the
// current quoted string delimiter identifier, matching upstream's Scanner
// struct in common/scanner.h.
type ocamlScannerState struct {
	inString       bool
	quotedStringID []int32 // delimiter chars for {id|...|id} strings
}

// OcamlExternalScanner implements gotreesitter.ExternalScanner for tree-sitter-ocaml.
//
// This is a Go port of the C external scanner shared by every tree-sitter-ocaml
// sub-grammar (common/scanner.h). The scanner handles:
//   - Nestable (* *) comments, lexically aware of strings, quoted strings, and
//     character literals inside them
//   - Quoted string delimiters {id|...|id}
//   - String open/close with in_string state tracking
//   - Line number directives (# <num> "file")
//   - Literal null characters (\0 that isn't EOF)
type OcamlExternalScanner struct {
	symbols         [ocamlTokenCount]gotreesitter.Symbol
	externalToToken []int
}

// ExternalScannerForLanguage binds this scanner's token slots to lang's
// concrete external symbol IDs so Scan reports the IDs the parser table
// actually expects, instead of IDs frozen at some earlier grammar revision.
func (OcamlExternalScanner) ExternalScannerForLanguage(lang *gotreesitter.Language) gotreesitter.ExternalScanner {
	s := OcamlExternalScanner{symbols: ocamlDefaultSymTable}
	s.externalToToken = bindExternalScannerSpec(lang, ocamlExternalScannerSpec, func(tokenIdx int, sym gotreesitter.Symbol) {
		s.symbols[tokenIdx] = sym
	})
	return s
}

func (OcamlExternalScanner) Create() any {
	return &ocamlScannerState{}
}

func (OcamlExternalScanner) Destroy(payload any) {}

func (OcamlExternalScanner) Serialize(payload any, buf []byte) int {
	s := payload.(*ocamlScannerState)
	if len(buf) == 0 {
		return 0
	}
	if s.inString {
		buf[0] = 1
	} else {
		buf[0] = 0
	}
	// Copy quoted string ID (stored as int32 bytes).
	idBytes := len(s.quotedStringID) * 4
	if 1+idBytes > len(buf) {
		return 1
	}
	pos := 1
	for _, c := range s.quotedStringID {
		buf[pos] = byte(c)
		buf[pos+1] = byte(c >> 8)
		buf[pos+2] = byte(c >> 16)
		buf[pos+3] = byte(c >> 24)
		pos += 4
	}
	return pos
}

func (OcamlExternalScanner) Deserialize(payload any, buf []byte) {
	s := payload.(*ocamlScannerState)
	s.inString = false
	s.quotedStringID = s.quotedStringID[:0]

	if len(buf) == 0 {
		return
	}
	s.inString = buf[0] != 0
	pos := 1
	for pos+4 <= len(buf) {
		c := int32(buf[pos]) | int32(buf[pos+1])<<8 | int32(buf[pos+2])<<16 | int32(buf[pos+3])<<24
		s.quotedStringID = append(s.quotedStringID, c)
		pos += 4
	}
}

func (s OcamlExternalScanner) symbolTable() *[ocamlTokenCount]gotreesitter.Symbol {
	if s.symbols == ([ocamlTokenCount]gotreesitter.Symbol{}) {
		return &ocamlDefaultSymTable
	}
	return &s.symbols
}

// remapValidSymbols translates the parser's external-index-space validSymbols
// slice into this scanner's token-index space via externalToToken, matching
// the pattern used by the other positionally bound scanners in this package
// (see dart_scanner.go, csharp_scanner.go).
func (s OcamlExternalScanner) remapValidSymbols(validSymbols []bool, semanticValid *[ocamlTokenCount]bool) []bool {
	if len(s.externalToToken) == 0 {
		return validSymbols
	}
	*semanticValid = [ocamlTokenCount]bool{}
	for externalIdx, valid := range validSymbols {
		if !valid || externalIdx >= len(s.externalToToken) {
			continue
		}
		tokenIdx := s.externalToToken[externalIdx]
		if tokenIdx >= 0 && tokenIdx < ocamlTokenCount {
			semanticValid[tokenIdx] = true
		}
	}
	return semanticValid[:]
}

func (s OcamlExternalScanner) Scan(payload any, lexer *gotreesitter.ExternalLexer, validSymbols []bool) bool {
	state := payload.(*ocamlScannerState)
	var semanticValid [ocamlTokenCount]bool
	validSymbols = s.remapValidSymbols(validSymbols, &semanticValid)
	symbols := s.symbolTable()

	// Left quoted string delimiter: {id|
	if !ocamlValid(validSymbols, ocamlTokErrorSentinel) &&
		ocamlValid(validSymbols, ocamlTokLeftQuotedStringDel) {
		ch := lexer.Lookahead()
		if isOcamlLowercaseExt(ch) || ch == '|' {
			lexer.SetResultSymbol(symbols[ocamlTokLeftQuotedStringDel])
			return ocamlScanLeftQuotedStringDelim(state, lexer)
		}
	}

	// Right quoted string delimiter: |id}
	if !ocamlValid(validSymbols, ocamlTokErrorSentinel) &&
		ocamlValid(validSymbols, ocamlTokRightQuotedStringDel) &&
		lexer.Lookahead() == '|' {
		lexer.Advance(false)
		lexer.SetResultSymbol(symbols[ocamlTokRightQuotedStringDel])
		return ocamlScanRightQuotedStringDelim(state, lexer)
	}

	// Closing string delimiter (before whitespace skip).
	if state.inString && ocamlValid(validSymbols, ocamlTokStringDelim) &&
		lexer.Lookahead() == '"' {
		lexer.Advance(false)
		state.inString = false
		lexer.MarkEnd()
		lexer.SetResultSymbol(symbols[ocamlTokStringDelim])
		return true
	}

	// Skip whitespace.
	for unicode.IsSpace(lexer.Lookahead()) {
		lexer.Advance(true)
	}

	// Opening string delimiter.
	if !state.inString && ocamlValid(validSymbols, ocamlTokStringDelim) &&
		lexer.Lookahead() == '"' {
		lexer.Advance(false)
		state.inString = true
		lexer.MarkEnd()
		lexer.SetResultSymbol(symbols[ocamlTokStringDelim])
		return true
	}

	// Line number directive: # <digits> "filename"
	if !state.inString && ocamlValid(validSymbols, ocamlTokLineNumberDirective) &&
		lexer.Lookahead() == '#' && lexer.Column() == 0 {
		return ocamlScanLineNumberDirective(lexer, symbols[ocamlTokLineNumberDirective])
	}

	// Comment: (* ... *)
	if !state.inString && ocamlValid(validSymbols, ocamlTokComment) &&
		lexer.Lookahead() == '(' {
		lexer.Advance(false)
		lexer.SetResultSymbol(symbols[ocamlTokComment])
		return ocamlScanComment(state, lexer)
	}

	// Null character (literal \0 that isn't EOF). Our ExternalLexer cannot
	// tell a true embedded NUL byte apart from EOF: Lookahead returns 0 for
	// both. Every hand-written scanner in this runtime that needs an EOF
	// check treats Lookahead()==0 as EOF (see cryIsEOF in crystal_scanner.go),
	// so upstream's "!eof(lexer)" guard is always false here and this token
	// never fires.
	if ocamlValid(validSymbols, ocamlTokNull) && lexer.Lookahead() == 0 {
		return false
	}

	return false
}

// ---------------------------------------------------------------------------
// Quoted string delimiters (scan_left_quoted_string_delimiter,
// scan_right_quoted_string_delimiter, scan_quoted_string_delim_char)
// ---------------------------------------------------------------------------

func ocamlScanLeftQuotedStringDelim(s *ocamlScannerState, lexer *gotreesitter.ExternalLexer) bool {
	s.quotedStringID = s.quotedStringID[:0]

	for {
		c := ocamlScanQuotedStringDelimChar(lexer)
		if c == 0 {
			break
		}
		s.quotedStringID = append(s.quotedStringID, c)
	}

	if lexer.Lookahead() == '|' {
		lexer.Advance(false)
		s.inString = true
		return true
	}

	s.quotedStringID = s.quotedStringID[:0]
	return false
}

func ocamlScanRightQuotedStringDelim(s *ocamlScannerState, lexer *gotreesitter.ExternalLexer) bool {
	for _, expected := range s.quotedStringID {
		if ocamlScanQuotedStringDelimChar(lexer) != expected {
			return false
		}
	}

	if lexer.Lookahead() == '}' {
		s.inString = false
		s.quotedStringID = s.quotedStringID[:0]
		return true
	}
	return false
}

func ocamlScanQuotedString(s *ocamlScannerState, lexer *gotreesitter.ExternalLexer) bool {
	if !ocamlScanLeftQuotedStringDelim(s, lexer) {
		return false
	}
	for {
		switch lexer.Lookahead() {
		case '|':
			lexer.Advance(false)
			if ocamlScanRightQuotedStringDelim(s, lexer) {
				return true
			}
		case 0:
			return false
		default:
			lexer.Advance(false)
		}
	}
}

// ocamlLowerUTF8Chars mirrors upstream's LOWER_UTF8_CHARS: single lowercase
// Latin-1/Latin Extended-A code points OCaml treats as identifier-start
// characters, from ocaml/ocaml utils/misc.ml.
var ocamlLowerUTF8Chars = []int32{
	0xdf, 0xe0, 0xe1, 0xe2, 0xe3, 0xe4, 0xe5, 0xe6, 0xe7, 0xe8,
	0xe9, 0xea, 0xeb, 0xec, 0xed, 0xee, 0xef, 0xf0, 0xf1, 0xf2,
	0xf3, 0xf4, 0xf5, 0xf6, 0xf8, 0xf9, 0xfa, 0xfb, 0xfc, 0xfd,
	0xfe, 0xff, 0x153, 0x161, 0x17e,
}

// ocamlLowerUTF8Pair is one (base letter, combining diacritic) -> precomposed
// lowercase pair, mirroring upstream's LOWER_UTF8_PAIRS.
type ocamlLowerUTF8Pair struct {
	base      int32
	diacritic int32
	result    int32
}

var ocamlLowerUTF8Pairs = []ocamlLowerUTF8Pair{
	{'a', 0x300, 0xe0}, {'a', 0x301, 0xe1}, {'a', 0x302, 0xe2},
	{'a', 0x303, 0xe3}, {'a', 0x308, 0xe4}, {'a', 0x30a, 0xe5},
	{'c', 0x327, 0xe7}, {'e', 0x300, 0xe8}, {'e', 0x301, 0xe9},
	{'e', 0x302, 0xea}, {'e', 0x308, 0xeb}, {'i', 0x300, 0xec},
	{'i', 0x301, 0xed}, {'i', 0x302, 0xee}, {'i', 0x308, 0xef},
	{'n', 0x303, 0xf1}, {'o', 0x300, 0xf2}, {'o', 0x301, 0xf3},
	{'o', 0x302, 0xf4}, {'o', 0x303, 0xf5}, {'o', 0x308, 0xf6},
	{'s', 0x30c, 0x161}, {'u', 0x300, 0xf9}, {'u', 0x301, 0xfa},
	{'u', 0x302, 0xfb}, {'u', 0x308, 0xfc}, {'y', 0x301, 0xfd},
	{'y', 0x308, 0xff}, {'z', 0x30c, 0x17e},
}

func ocamlSearchLowerUTF8Char(c int32) int32 {
	for _, v := range ocamlLowerUTF8Chars {
		if v == c {
			return v
		}
	}
	return 0
}

func ocamlSearchLowerUTF8Pair(c, diacritic int32) int32 {
	for _, p := range ocamlLowerUTF8Pairs {
		if p.base == c && p.diacritic == diacritic {
			return p.result
		}
	}
	return 0
}

// ocamlScanQuotedStringDelimChar scans one character of a quoted string
// delimiter identifier, mirroring upstream's scan_quoted_string_delim_char.
// Returns the char, or 0 if the lookahead is not a valid delimiter char.
func ocamlScanQuotedStringDelimChar(lexer *gotreesitter.ExternalLexer) int32 {
	c := lexer.Lookahead()

	if c == '|' {
		return 0
	}
	if c == '_' {
		lexer.Advance(false)
		return c
	}
	if c >= 'a' && c <= 'z' {
		lexer.Advance(false)
		if next := lexer.Lookahead(); next >= 0x300 && next <= 0x327 {
			if r := ocamlSearchLowerUTF8Pair(c, next); r != 0 {
				lexer.Advance(false)
				return r
			}
		}
		return c
	}
	if r := ocamlSearchLowerUTF8Char(c); r != 0 {
		lexer.Advance(false)
		return r
	}
	return 0
}

// ---------------------------------------------------------------------------
// Comment scanning (scan_comment, scan_character, scan_string,
// scan_extattrident, scan_identifier)
// ---------------------------------------------------------------------------

// ocamlScanString mirrors upstream's scan_string: skips a regular "..."
// string, used both at top level and while skipping strings inside comments.
func ocamlScanString(lexer *gotreesitter.ExternalLexer) {
	for {
		switch lexer.Lookahead() {
		case '\\':
			lexer.Advance(false)
			lexer.Advance(false)
		case '"':
			lexer.Advance(false)
			return
		case 0:
			return
		default:
			lexer.Advance(false)
		}
	}
}

func isOcamlLowercaseExt(c rune) bool {
	return (c >= 'a' && c <= 'z') || c == '_' || c >= 192
}

func isOcamlIdentStart(c rune) bool {
	return isOcamlLowercaseExt(c) || (c >= 'A' && c <= 'Z')
}

func isOcamlIdentChar(c rune) bool {
	return isOcamlIdentStart(c) || (c >= '0' && c <= '9') || c == '\''
}

// ocamlScanIdentifier mirrors upstream's scan_identifier.
func ocamlScanIdentifier(lexer *gotreesitter.ExternalLexer) bool {
	if isOcamlIdentStart(lexer.Lookahead()) {
		lexer.Advance(false)
		for isOcamlIdentChar(lexer.Lookahead()) {
			lexer.Advance(false)
		}
		return true
	}
	return false
}

// ocamlScanExtAttrIdent mirrors upstream's scan_extattrident: a dotted chain
// of identifiers, used by the "{%attr ..." extension-node syntax inside
// comments.
func ocamlScanExtAttrIdent(lexer *gotreesitter.ExternalLexer) bool {
	for ocamlScanIdentifier(lexer) {
		if lexer.Lookahead() != '.' {
			return true
		}
		lexer.Advance(false)
	}
	return false
}

// ocamlScanCharacter mirrors upstream's scan_character: scans one OCaml
// character-literal payload (after the opening quote character), including
// escape sequences, and reports whether a closing quote character
// immediately follows.
//
// The return value distinguishes two cases the caller (ocamlScanComment)
// must handle differently:
//   - 0: either the payload closed on a following quote character (a real
//     character literal, fully consumed), or the payload was empty/invalid
//     at EOF.
//   - non-zero: the payload was NOT followed by a closing quote character.
//     The scanner already advanced past one lookahead character while
//     probing for the literal (OCaml's polymorphic type-variable syntax,
//     e.g. 'a, looks identical up to this point), and that already-consumed
//     character must be reprocessed by the caller without calling Advance
//     again.
func ocamlScanCharacter(lexer *gotreesitter.ExternalLexer) int32 {
	var last int32

	switch c := lexer.Lookahead(); c {
	case '\\':
		lexer.Advance(false)
		if unicode.IsDigit(lexer.Lookahead()) {
			lexer.Advance(false)
			for i := 0; i < 2; i++ {
				if !unicode.IsDigit(lexer.Lookahead()) {
					return 0
				}
				lexer.Advance(false)
			}
		} else {
			switch lexer.Lookahead() {
			case 'x':
				lexer.Advance(false)
				for i := 0; i < 2; i++ {
					la := lexer.Lookahead()
					upper := unicode.ToUpper(la)
					if !unicode.IsDigit(la) && (upper < 'A' || upper > 'F') {
						return 0
					}
					lexer.Advance(false)
				}
			case 'o':
				lexer.Advance(false)
				for i := 0; i < 3; i++ {
					la := lexer.Lookahead()
					if !unicode.IsDigit(la) || la > '7' {
						return 0
					}
					lexer.Advance(false)
				}
			case '\'', '"', '\\', 'n', 't', 'b', 'r', ' ':
				last = lexer.Lookahead()
				lexer.Advance(false)
			default:
				return 0
			}
		}
	case '\'':
		// Empty: leaves last == 0 and does not advance, matching upstream.
	case '\r':
		lexer.Advance(false)
		for lexer.Lookahead() == '\r' {
			lexer.Advance(false)
		}
		if lexer.Lookahead() != '\n' {
			return 0
		}
		lexer.Advance(false)
	case 0:
		return 0
	default:
		if c < 256 {
			last = c
			lexer.Advance(false)
		} else {
			return 0
		}
	}

	if lexer.Lookahead() == '\'' {
		lexer.Advance(false)
		return 0
	}
	return last
}

// ocamlScanComment mirrors upstream's scan_comment: an iterative (* *)
// comment scanner that tracks nesting depth with a counter instead of
// recursion, so a deeply nested comment cannot grow the Go call stack.
//
// The `last` local reproduces upstream's reprocess-without-advancing idiom:
// scan_character's caller sometimes discovers a lookahead character that
// scan_character already advanced past while probing for a character
// literal. That character must be dispatched through the same switch below
// without a further Advance call, exactly as upstream re-dispatches on
// `last ? last : lexer->lookahead`.
func ocamlScanComment(s *ocamlScannerState, lexer *gotreesitter.ExternalLexer) bool {
	if lexer.Lookahead() != '*' {
		return false
	}
	lexer.Advance(false)

	var last int32
	var depth uint32

	for {
		dispatch := last
		if dispatch == 0 {
			dispatch = int32(lexer.Lookahead())
		}
		switch dispatch {
		case '(':
			if last != 0 {
				last = 0
			} else {
				lexer.Advance(false)
			}
			if lexer.Lookahead() == '*' {
				lexer.Advance(false)
				depth++
			}
		case '*':
			if last != 0 {
				last = 0
			} else {
				lexer.Advance(false)
			}
			if lexer.Lookahead() == ')' {
				lexer.Advance(false)
				if depth == 0 {
					lexer.MarkEnd()
					return true
				}
				depth--
			}
		case '\'':
			if last != 0 {
				last = 0
			} else {
				lexer.Advance(false)
			}
			last = ocamlScanCharacter(lexer)
		case '"':
			if last != 0 {
				last = 0
			} else {
				lexer.Advance(false)
			}
			ocamlScanString(lexer)
		case '{':
			if last != 0 {
				last = 0
			} else {
				lexer.Advance(false)
			}
			if lexer.Lookahead() == '%' {
				lexer.Advance(false)
				if lexer.Lookahead() == '%' {
					lexer.Advance(false)
				}
				if ocamlScanExtAttrIdent(lexer) {
					for unicode.IsSpace(lexer.Lookahead()) {
						lexer.Advance(false)
					}
				} else {
					break
				}
			}
			if ocamlScanQuotedString(s, lexer) {
				lexer.Advance(false)
			}
		case 0:
			// Our ExternalLexer cannot tell a true embedded NUL byte apart
			// from EOF (see the ocamlTokNull comment in Scan), so treat this
			// as always-EOF, matching upstream's `if (eof(lexer)) return
			// true;` under that convention: an unterminated comment still
			// closes as a comment at EOF instead of misparsing as other
			// grammar rules.
			lexer.MarkEnd()
			return true
		default:
			if ocamlScanIdentifier(lexer) || last != 0 {
				last = 0
			} else {
				lexer.Advance(false)
			}
		}
	}
}

// ---------------------------------------------------------------------------
// Line number directive
// ---------------------------------------------------------------------------

func ocamlScanLineNumberDirective(lexer *gotreesitter.ExternalLexer, resultSymbol gotreesitter.Symbol) bool {
	lexer.Advance(false) // consume '#'

	// Skip spaces/tabs.
	for lexer.Lookahead() == ' ' || lexer.Lookahead() == '\t' {
		lexer.Advance(false)
	}

	// Expect digits.
	if !unicode.IsDigit(lexer.Lookahead()) {
		return false
	}
	for unicode.IsDigit(lexer.Lookahead()) {
		lexer.Advance(false)
	}

	// Skip spaces/tabs.
	for lexer.Lookahead() == ' ' || lexer.Lookahead() == '\t' {
		lexer.Advance(false)
	}

	// Expect opening quote.
	if lexer.Lookahead() != '"' {
		return false
	}
	lexer.Advance(false)

	// Filename: everything until closing quote, newline, or EOF.
	for {
		ch := lexer.Lookahead()
		if ch == '\n' || ch == '\r' || ch == '"' || ch == 0 {
			break
		}
		lexer.Advance(false)
	}

	if lexer.Lookahead() != '"' {
		return false
	}
	lexer.Advance(false)

	// Consume rest of line.
	for {
		ch := lexer.Lookahead()
		if ch == '\n' || ch == '\r' || ch == 0 {
			break
		}
		lexer.Advance(false)
	}

	lexer.MarkEnd()
	lexer.SetResultSymbol(resultSymbol)
	return true
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func ocamlValid(validSymbols []bool, idx int) bool {
	return idx >= 0 && idx < len(validSymbols) && validSymbols[idx]
}
