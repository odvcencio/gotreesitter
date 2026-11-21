//go:build !grammar_subset || grammar_subset_ocaml

package grammarruntime

import (
	gotreesitter "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/internal/scanmode"
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
	// Checked against the pinned C scanner (cgo_harness scandiff, 200,000 calls); no mode.
	scanmode.RegisterScanner(OcamlExternalScanner{}, (*ocamlScannerState)(nil), false, func(v any) (any, []byte, bool) {
		s := v.(OcamlExternalScanner)
		s.externalToToken = append([]int(nil), s.externalToToken...)
		return s, scannerKey(s.symbols[:], s.externalToToken), true
	}, func(left, right any) bool {
		a, b := left.(OcamlExternalScanner), right.(OcamlExternalScanner)
		return scannerConfigEqual(a.symbols[:], b.symbols[:], a.externalToToken, b.externalToToken)
	})
}

// ocamlScannerState mirrors upstream's Scanner struct in common/scanner.h:21-27. The delimiter
// identifier is kept the way the C code keeps it, because its saved form depends on that:
// id is the backing array (its length is the capacity, which only grows) and idLen is
// quoted_string_id_length. The array keeps its old contents when a shorter id is loaded, as
// the C array does.
type ocamlScannerState struct {
	inString bool
	idLen    int
	id       []int32
}

// quoted_string_id_resize (common/scanner.h:33-45): grow to at least minCapacity items. The new
// part holds zeros here; the C array holds whatever the allocator returned.
func (s *ocamlScannerState) idResize(minCapacity int) {
	capacity := len(s.id)
	if capacity >= minCapacity {
		return
	}
	if capacity < 16 {
		capacity = 16
	}
	for capacity < minCapacity {
		capacity <<= 1
	}
	grown := make([]int32, capacity)
	copy(grown, s.id)
	s.id = grown
}

// quoted_string_id_push (common/scanner.h:62-65).
func (s *ocamlScannerState) idPush(c int32) {
	s.idResize(s.idLen + 1)
	s.id[s.idLen] = c
	s.idLen++
}

// idByte and setIDByte address the array as bytes in little-endian order, which is what the C
// memcpy calls on lines 51 and 58 do.
func (s *ocamlScannerState) idByte(i int) byte {
	return byte(uint32(s.id[i/4]) >> (8 * uint(i%4)))
}

func (s *ocamlScannerState) setIDByte(i int, b byte) {
	shift := 8 * uint(i%4)
	s.id[i/4] = int32(uint32(s.id[i/4])&^(0xff<<shift) | uint32(b)<<shift)
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

// ocamlSerializationBufferSize is TREE_SITTER_SERIALIZATION_BUFFER_SIZE.
const ocamlSerializationBufferSize = 1024

// Serialize is common/scanner.h:486-494. quoted_string_id_copy (:56-60) copies
// quoted_string_id_length BYTES of the int32 array, not that many items, so the saved state is
// idLen bytes long and holds the low bytes of the first items.
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
	if s.idLen < ocamlSerializationBufferSize && 1+s.idLen <= len(buf) {
		for i := 0; i < s.idLen; i++ {
			buf[1+i] = s.idByte(i)
		}
		return s.idLen + 1
	}
	return 1
}

// Deserialize is common/scanner.h:496-504. quoted_string_id_assign (:47-54) copies length-1
// bytes into the int32 array and then takes length-1 as the number of items.
func (OcamlExternalScanner) Deserialize(payload any, buf []byte) {
	s := payload.(*ocamlScannerState)
	if len(buf) == 0 {
		s.inString = false
		s.idLen = 0
		return
	}
	s.inString = buf[0] != 0
	n := len(buf) - 1
	if n > 0 {
		s.idResize(n)
		for i := 0; i < n; i++ {
			s.setIDByte(i, buf[1+i])
		}
	}
	s.idLen = n
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

// cla is lexer->lookahead as C sees it: -1 for a byte that is not valid UTF-8.
func cla(lexer *gotreesitter.ExternalLexer) int32 { return scanmode.CLookahead(lexer) }

func (s OcamlExternalScanner) Scan(payload any, lexer *gotreesitter.ExternalLexer, validSymbols []bool) bool {
	state := payload.(*ocamlScannerState)
	var semanticValid [ocamlTokenCount]bool
	validSymbols = s.remapValidSymbols(validSymbols, &semanticValid)
	symbols := s.symbolTable()

	// common/scanner.h:507-512. The C scanner never calls mark_end, so the token ends at the
	// cursor when it returns.
	if !ocamlValid(validSymbols, ocamlTokErrorSentinel) &&
		ocamlValid(validSymbols, ocamlTokLeftQuotedStringDel) {
		ch := cla(lexer)
		if isOcamlLowercaseExt(ch) || ch == '|' {
			lexer.SetResultSymbol(symbols[ocamlTokLeftQuotedStringDel])
			return ocamlScanLeftQuotedStringDelim(state, lexer)
		}
	}

	// :514-519
	if !ocamlValid(validSymbols, ocamlTokErrorSentinel) &&
		ocamlValid(validSymbols, ocamlTokRightQuotedStringDel) &&
		lexer.Lookahead() == '|' {
		lexer.Advance(false)
		lexer.SetResultSymbol(symbols[ocamlTokRightQuotedStringDel])
		return ocamlScanRightQuotedStringDelim(state, lexer)
	}

	// :521-527
	if state.inString && ocamlValid(validSymbols, ocamlTokStringDelim) &&
		lexer.Lookahead() == '"' {
		lexer.Advance(false)
		state.inString = false
		lexer.SetResultSymbol(symbols[ocamlTokStringDelim])
		return true
	}

	// :529-531 iswspace in the C locale is ASCII only.
	for cIsSpace(cla(lexer)) {
		lexer.Advance(true)
	}

	// :533-539
	if !state.inString && ocamlValid(validSymbols, ocamlTokStringDelim) &&
		lexer.Lookahead() == '"' {
		lexer.Advance(false)
		state.inString = true
		lexer.SetResultSymbol(symbols[ocamlTokStringDelim])
		return true
	}

	// :541-574
	if !state.inString && ocamlValid(validSymbols, ocamlTokLineNumberDirective) &&
		lexer.Lookahead() == '#' && lexer.Column() == 0 {
		return ocamlScanLineNumberDirective(lexer, symbols[ocamlTokLineNumberDirective])
	}

	// :576-581
	if !state.inString && ocamlValid(validSymbols, ocamlTokComment) &&
		lexer.Lookahead() == '(' {
		lexer.Advance(false)
		lexer.SetResultSymbol(symbols[ocamlTokComment])
		return ocamlScanComment(state, lexer)
	}

	// :583-588 a NUL byte inside the input, not the end of input.
	if ocamlValid(validSymbols, ocamlTokNull) && lexer.Lookahead() == 0 && !scanmode.AtEOF(lexer) {
		lexer.Advance(false)
		lexer.SetResultSymbol(symbols[ocamlTokNull])
		return true
	}

	return false
}

// ---------------------------------------------------------------------------
// Quoted string delimiters (scan_left_quoted_string_delimiter,
// scan_right_quoted_string_delimiter, scan_quoted_string_delim_char)
// ---------------------------------------------------------------------------

// common/scanner.h:232-248. The identifier is not cleared first: the C code pushes onto
// whatever the saved state loaded.
func ocamlScanLeftQuotedStringDelim(s *ocamlScannerState, lexer *gotreesitter.ExternalLexer) bool {
	for {
		c := ocamlScanQuotedStringDelimChar(lexer)
		if c == 0 {
			break
		}
		s.idPush(c)
	}

	if lexer.Lookahead() == '|' {
		lexer.Advance(false)
		s.inString = true
		return true
	}

	s.idLen = 0
	return false
}

// common/scanner.h:250-265.
func ocamlScanRightQuotedStringDelim(s *ocamlScannerState, lexer *gotreesitter.ExternalLexer) bool {
	for i := 0; i < s.idLen; i++ {
		if ocamlScanQuotedStringDelimChar(lexer) != s.id[i] {
			return false
		}
	}

	if lexer.Lookahead() == '}' {
		s.inString = false
		s.idLen = 0
		return true
	}
	return false
}

// common/scanner.h:267-284. Only the end of input stops the loop, not a NUL byte.
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
			if scanmode.AtEOF(lexer) {
				return false
			}
			lexer.Advance(false)
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

// ocamlScanQuotedStringDelimChar is scan_quoted_string_delim_char (common/scanner.h:201-230).
// It returns the char, or 0 if the lookahead is not a valid delimiter char.
func ocamlScanQuotedStringDelimChar(lexer *gotreesitter.ExternalLexer) int32 {
	c := cla(lexer)

	if c == '|' {
		return 0
	}
	if c == '_' {
		lexer.Advance(false)
		return c
	}
	if c >= 'a' && c <= 'z' {
		lexer.Advance(false)
		if next := cla(lexer); next >= 0x300 && next <= 0x327 {
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

// ocamlScanString is scan_string (common/scanner.h:73-91). A NUL byte does not end it, only the
// end of input does.
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
			if scanmode.AtEOF(lexer) {
				return
			}
			lexer.Advance(false)
		default:
			lexer.Advance(false)
		}
	}
}

func isOcamlLowercaseExt(c int32) bool {
	return (c >= 'a' && c <= 'z') || c == '_' || c >= 192
}

func isOcamlIdentStart(c int32) bool {
	return isOcamlLowercaseExt(c) || (c >= 'A' && c <= 'Z')
}

func isOcamlIdentChar(c int32) bool {
	return isOcamlIdentStart(c) || (c >= '0' && c <= '9') || c == '\''
}

// ocamlScanIdentifier is scan_identifier (common/scanner.h:376-383). The test sees the C
// lookahead, so an invalid UTF-8 byte (-1) is not an identifier character.
func ocamlScanIdentifier(lexer *gotreesitter.ExternalLexer) bool {
	if isOcamlIdentStart(cla(lexer)) {
		lexer.Advance(false)
		for isOcamlIdentChar(cla(lexer)) {
			lexer.Advance(false)
		}
		return true
	}
	return false
}

// ocamlScanExtAttrIdent is scan_extattrident (common/scanner.h:385-391).
func ocamlScanExtAttrIdent(lexer *gotreesitter.ExternalLexer) bool {
	for ocamlScanIdentifier(lexer) {
		if lexer.Lookahead() != '.' {
			return true
		}
		lexer.Advance(false)
	}
	return false
}

// towupper in the C locale changes ASCII letters only.
func cToUpperWide(c int32) int32 {
	if c >= 'a' && c <= 'z' {
		return c - 'a' + 'A'
	}
	return c
}

// ocamlScanCharacter is scan_character (common/scanner.h:286-362). A non-zero result is a
// character that was read but not followed by a closing quote; the caller handles it again
// without advancing.
func ocamlScanCharacter(lexer *gotreesitter.ExternalLexer) int32 {
	var last int32

	switch c := cla(lexer); c {
	case '\\':
		lexer.Advance(false)
		if cIsDigit(cla(lexer)) {
			lexer.Advance(false)
			for i := 0; i < 2; i++ {
				if !cIsDigit(cla(lexer)) {
					return 0
				}
				lexer.Advance(false)
			}
		} else {
			switch cla(lexer) {
			case 'x':
				lexer.Advance(false)
				for i := 0; i < 2; i++ {
					la := cla(lexer)
					upper := cToUpperWide(la)
					if !cIsDigit(la) && (upper < 'A' || upper > 'F') {
						return 0
					}
					lexer.Advance(false)
				}
			case 'o':
				lexer.Advance(false)
				for i := 0; i < 3; i++ {
					la := cla(lexer)
					if !cIsDigit(la) || la > '7' {
						return 0
					}
					lexer.Advance(false)
				}
			case '\'', '"', '\\', 'n', 't', 'b', 'r', ' ':
				last = cla(lexer)
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
		// :344-347 only the end of input returns; a NUL byte is skipped.
		if scanmode.AtEOF(lexer) {
			return 0
		}
		lexer.Advance(false)
	default:
		// :348-354 an invalid UTF-8 byte (-1) is below 256.
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

// ocamlScanComment is scan_comment (common/scanner.h:393-474): an iterative (* *) comment scanner.
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
			dispatch = cla(lexer)
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
					for cIsSpace(cla(lexer)) {
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
			// :458-465 only the end of input returns; a NUL byte is skipped.
			if scanmode.AtEOF(lexer) {
				return true
			}
			if last != 0 {
				last = 0
			} else {
				lexer.Advance(false)
			}
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

// common/scanner.h:541-574. The loops test lexer->eof; a NUL byte does not stop them.
func ocamlScanLineNumberDirective(lexer *gotreesitter.ExternalLexer, resultSymbol gotreesitter.Symbol) bool {
	lexer.Advance(false) // consume '#'

	for lexer.Lookahead() == ' ' || lexer.Lookahead() == '\t' {
		lexer.Advance(false)
	}

	if !cIsDigit(cla(lexer)) {
		return false
	}
	for cIsDigit(cla(lexer)) {
		lexer.Advance(false)
	}

	for lexer.Lookahead() == ' ' || lexer.Lookahead() == '\t' {
		lexer.Advance(false)
	}

	if lexer.Lookahead() != '"' {
		return false
	}
	lexer.Advance(false)

	for lexer.Lookahead() != '\n' && lexer.Lookahead() != '\r' &&
		lexer.Lookahead() != '"' && !scanmode.AtEOF(lexer) {
		lexer.Advance(false)
	}

	if lexer.Lookahead() != '"' {
		return false
	}
	lexer.Advance(false)

	for lexer.Lookahead() != '\n' && lexer.Lookahead() != '\r' && !scanmode.AtEOF(lexer) {
		lexer.Advance(false)
	}

	lexer.SetResultSymbol(resultSymbol)
	return true
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func ocamlValid(validSymbols []bool, idx int) bool {
	return idx >= 0 && idx < len(validSymbols) && validSymbols[idx]
}
