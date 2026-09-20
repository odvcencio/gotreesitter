//go:build !grammar_subset || grammar_subset_scala

package grammarruntime

import (
	"crypto/sha256"
	"encoding/binary"

	gotreesitter "github.com/odvcencio/gotreesitter"
)

// scalaExternalScannerLocalPortSHA256 identifies the marked scanner port.
// A focused test requires an identity update when the implementation changes.
const scalaExternalScannerLocalPortSHA256 = "e383718c2b0a41d9f582d3a17fa0e0c160db62c73a478249cb930c27d0cbe784"

// SCALA_EXTERNAL_SCANNER_LOCAL_PORT_BEGIN

// External token indexes for the Scala grammar. Order must mirror the
// upstream externals array in src/grammar.json exactly: tree-sitter couples
// the scanner token index to the external's position.
const (
	scaTokAutoSemicolon               = 0
	scaTokIndent                      = 1
	scaTokOutdent                     = 2
	scaTokCommaOutdent                = 3
	scaTokSimpleStringStart           = 4
	scaTokSimpleStringMiddle          = 5
	scaTokSimpleMultiStringStart      = 6
	scaTokInterpStringMiddle          = 7
	scaTokInterpMultiStringMiddle     = 8
	scaTokRawStringStart              = 9
	scaTokRawStringMiddle             = 10
	scaTokRawMultiStringMiddle        = 11
	scaTokSingleLineStringEnd         = 12
	scaTokMultilineStringEnd          = 13
	scaTokElse                        = 14
	scaTokCatch                       = 15
	scaTokFinally                     = 16
	scaTokExtends                     = 17
	scaTokDerives                     = 18
	scaTokWith                        = 19
	scaTokBlockComment                = 20
	scaTokSuppressBlockComment        = 21
	scaTokErrorSentinel               = 22
	scaTokColonEol                    = 23
	scaTokPostfixOp                   = 24
	scaTokPostfixStar                 = 25
	scaTokFloatingPointWithSeparators = 26
	scaTokEndKeyword                  = 27
	scaTokControlTailGate             = 28
	scaTokXmlTagStart                 = 29
	scaTokErasedModifier              = 30
	scaTokOpenModifier                = 31
	scaTokOpaqueModifier              = 32
	scaTokInfixModifier               = 33
	scaTokTrackedModifier             = 34
	scaTokTransparentModifier         = 35
	scaTokInlineModifier              = 36
	scaTokIntoModifier                = 37
	scaTokUpdateModifier              = 38
	scaTokConsumeModifier             = 39
	scaTokUses                        = 40
	scaTokOpLeftOr                    = 41
	scaTokOpLeftXor                   = 42
	scaTokOpLeftAnd                   = 43
	scaTokOpLeftEq                    = 44
	scaTokOpLeftRel                   = 45
	scaTokOpLeftColon                 = 46
	scaTokOpLeftAdd                   = 47
	scaTokOpLeftMul                   = 48
	scaTokOpLeftOther                 = 49
	scaTokOpName                      = 50
	scaTokUsingDirectiveStart         = 51
	scaTokCaseDefinitionKeyword       = 52
	scaTokDefSemicolon                = 53
	scaTokWildcardBoundStart          = 54
	scaTokenCount                     = 55
)

// scaDefaultSymTable is the fallback symbol table used only when a scanner is
// constructed without a bound Language (scanner.symbols is the zero value).
// Every production path binds real symbols positionally through
// bindExternalScannerSpec, which overwrites every one of these entries, so
// the zero placeholders below are never observed by a bound scanner.
var scaDefaultSymTable = [scaTokenCount]gotreesitter.Symbol{}

var scalaExternalScannerSpec = ExternalScannerSpec{
	Language:       "scala",
	UpstreamRepo:   "https://github.com/tree-sitter/tree-sitter-scala",
	UpstreamCommit: "db390f312a54b04b13790e1767bfac32665c17ac",
	SourceFiles: []ExternalScannerSourceFile{
		{Path: "src/grammar.json", SHA256: "b5160fe983ae41f0d7e9adcd529e874cde0d9ea76cc906e072978d2f7b24a4c5"},
		{Path: "src/scanner.c", SHA256: "af12600dab23b81b263fa470b411a680755c119f221d5219e04f69b7883617df"},
	},
	Externals: []string{
		"automatic_semicolon",
		"indent",
		"outdent",
		"comma_outdent",
		"simple_string_start",
		"simple_string_middle",
		"simple_multiline_string_start",
		"interpolated_string_middle",
		"interpolated_multiline_string_middle",
		"raw_string_start",
		"raw_string_middle",
		"raw_string_multiline_middle",
		"single_line_string_end",
		"multiline_string_end",
		"else",
		"catch",
		"finally",
		"extends",
		"derives",
		"with",
		"block_comment",
		"suppress_block_comment",
		"error_sentinel",
		"colon_eol",
		"postfix_op",
		"postfix_star",
		"floating_point_with_separators",
		"end_keyword",
		"control_tail_gate",
		"xml_tag_start",
		"erased_modifier",
		"open_modifier",
		"opaque_modifier",
		"infix_modifier",
		"tracked_modifier",
		"transparent_modifier",
		"inline_modifier",
		"into_modifier",
		"update_modifier",
		"consume_modifier",
		"uses",
		"op_left_or",
		"op_left_xor",
		"op_left_and",
		"op_left_eq",
		"op_left_rel",
		"op_left_colon",
		"op_left_add",
		"op_left_mul",
		"op_left_other",
		"op_name",
		"using_directive_start",
		"case_definition_keyword",
		"def_semicolon",
		"wildcard_bound_start",
	},
}

func init() {
	RegisterExternalScannerSpec(scalaExternalScannerSpec)
}

// scaCaseIndentFlag marks an indent region whose `case` clauses align with
// their `match`/`catch` (Scala 3 same-width case). Such a region also closes
// on a same-width line that is not another `case` clause. The flag is packed
// into the int16 width.
const scaCaseIndentFlag int16 = 0x4000

type scalaState struct {
	indents             []int16
	lastIndentationSize int16
	lastNewlineCount    int16
	lastColumn          int16
	// lastChar is the lookahead at the position lastColumn names. Two lines
	// can share a column, so the character keeps the saved newline from
	// being recovered at an unrelated position further down.
	lastChar int16
	// afterColonEol tracks whether the last returned token was a
	// fewer-braces colon. Comments in between keep the flag.
	afterColonEol bool
}

// ScalaExternalScanner handles auto-semicolons, indent/outdent, layout
// gating, XML mode, operator precedence classification, and string scanning
// for Scala. The raw scanner remains conservative. The exact built-in runtime
// profile wraps it with incremental checkpoint capabilities after it
// verifies the blob.
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
	scalaScannerCheckpointHeader   = 14
	scalaExternalScannerABIVersion = "gotreesitter/scala-external-scanner/v1"
	scalaExternalScannerSemantics  = "state=indents,last-indentation-size,last-newline-count,last-column,last-char,after-colon-eol;codec=framed-le-v1;failure=eager-restore;error-tree=reject"
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
// Keep general subtree reuse closed. Interpolation and layout edits can
// invalidate reduction ownership even when every serialized scanner
// checkpoint matches.
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
	binary.LittleEndian.PutUint16(buf[10:12], uint16(s.lastChar))
	afterColonEol := uint16(0)
	if s.afterColonEol {
		afterColonEol = 1
	}
	binary.LittleEndian.PutUint16(buf[12:14], afterColonEol)
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
	s.lastChar = 0
	s.afterColonEol = false

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
	s.lastChar = int16(binary.LittleEndian.Uint16(buf[10:12]))
	s.afterColonEol = binary.LittleEndian.Uint16(buf[12:14]) != 0
	if count > cap(s.indents) {
		s.indents = make([]int16, count)
	} else {
		s.indents = s.indents[:count]
	}
	for index := range s.indents {
		offset := scalaScannerCheckpointHeader + index*2
		s.indents[index] = int16(binary.LittleEndian.Uint16(buf[offset : offset+2]))
	}
}

// Scan implements the tree_sitter_scala_external_scanner_scan wrapper: it
// runs the scan and then tracks whether the returned token was a
// fewer-braces colon. Comments in between keep the flag; a false return does
// not persist state anyway.
func (scanner ScalaExternalScanner) Scan(payload any, lexer *gotreesitter.ExternalLexer, validSymbols []bool) bool {
	s := payload.(*scalaState)
	symbols := scanner.symbolTable()
	if len(scanner.externalToToken) > 0 {
		var semanticValid [scaTokenCount]bool
		validSymbols = scanner.remapValidSymbols(validSymbols, &semanticValid)
	}
	isValid := func(idx int) bool {
		return idx >= 0 && idx < len(validSymbols) && validSymbols[idx]
	}

	ok, tok := scaScanImpl(s, lexer, isValid, symbols)
	if ok {
		switch tok {
		case scaTokColonEol:
			s.afterColonEol = true
		case scaTokBlockComment:
			// A comment between a fewer-braces colon and its body keeps the
			// flag; leave it as-is.
		default:
			s.afterColonEol = false
		}
	}
	return ok
}

// ---- character classes and small lexer helpers ----

// scaIsSpace mirrors the C locale set of iswspace, spelled out. No locale is
// ever installed, and library calls showed up in the parse profile.
func scaIsSpace(c rune) bool {
	return c == ' ' || (c >= '\t' && c <= '\r')
}

func scaIsAlpha(c rune) bool {
	return (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z')
}

func scaIsAlnum(c rune) bool {
	return scaIsAlpha(c) || (c >= '0' && c <= '9')
}

func scaIsWordStart(c rune) bool {
	return scaIsAlpha(c) || c == '_' || c == '$'
}

func scaIsEOF(lexer *gotreesitter.ExternalLexer) bool {
	return lexer.Lookahead() == 0
}

func scaAdvancePastBlanks(lexer *gotreesitter.ExternalLexer) bool {
	found := false
	for lexer.Lookahead() == ' ' || lexer.Lookahead() == '\t' {
		lexer.Advance(false)
		found = true
	}
	return found
}

func scaIndentWidth(entry int16) int16 {
	if entry == -1 {
		return -1
	}
	return entry & 0x3FFF
}

// scaAtCaseRegionWidth reports whether a line at `width` sits exactly at the
// width of a flagged same-width case region `prev`. Shared by every close
// site of such regions.
func scaAtCaseRegionWidth(prev, width int16) bool {
	return prev != -1 && (prev&scaCaseIndentFlag) != 0 && width == scaIndentWidth(prev)
}

// scaIsOpChar reports the ASCII opchars, deliberately without `/`, which
// could open a comment. An operator starting with `/` or a Unicode opchar
// never takes the external layout paths gated on this; the internal
// operator tokens handle it.
func scaIsOpChar(c rune) bool {
	switch c {
	case '!', '#', '%', '&', '*', '+', '-', '<', '=', '>', '?', '@', '\\', '^', '|', '~', ':':
		return true
	default:
		return false
	}
}

// scaOpLeftClass gives an operator its precedence by its first character
// (SLS 6.12.3). An operator ending in `/` is never right-associative and
// never an assignment, so the left-associative class is the whole answer.
func scaOpLeftClass(first rune) int {
	switch first {
	case '|':
		return scaTokOpLeftOr
	case '^':
		return scaTokOpLeftXor
	case '&':
		return scaTokOpLeftAnd
	case '=', '!':
		return scaTokOpLeftEq
	case '<', '>':
		return scaTokOpLeftRel
	case ':':
		return scaTokOpLeftColon
	case '+', '-':
		return scaTokOpLeftAdd
	case '*', '%':
		return scaTokOpLeftMul
	default:
		return scaTokOpLeftOther
	}
}

// scaIsXMLNameStart mirrors the start class of XML_NAME in grammar.json.
// Every non-ASCII character is accepted, since the letter test is
// ASCII-only and the internal lexer rejects the name if it is not a letter
// after all.
func scaIsXMLNameStart(c rune) bool {
	return scaIsAlpha(c) || c == '_' || c > 127
}

// scaIsCloseOrSeparator reports whether an operator directly before `c` ends
// its expression, leaving no right operand, so the operator is postfix.
func scaIsCloseOrSeparator(c rune) bool {
	return c == ')' || c == ']' || c == '}' || c == ',' || c == ';'
}

// ---- string content scanning ----

type scaStringMode int

const (
	scaStringSimple scaStringMode = iota
	scaStringInterp
	scaStringRaw
)

// scaScanStringContent enumerates the 3 types of strings scanned
// differently: simple strings, interpolated strings, and raw strings.
func scaScanStringContent(
	lexer *gotreesitter.ExternalLexer,
	isMultiline bool,
	mode scaStringMode,
	symbols *[scaTokenCount]gotreesitter.Symbol,
) (bool, int) {
	closingQuoteCount := 0
	for {
		switch {
		case lexer.Lookahead() == '"':
			lexer.Advance(false)
			closingQuoteCount++
			if !isMultiline {
				lexer.SetResultSymbol(symbols[scaTokSingleLineStringEnd])
				lexer.MarkEnd()
				return true, scaTokSingleLineStringEnd
			}
			if closingQuoteCount >= 3 && lexer.Lookahead() != '"' {
				lexer.SetResultSymbol(symbols[scaTokMultilineStringEnd])
				lexer.MarkEnd()
				return true, scaTokMultilineStringEnd
			}
		case lexer.Lookahead() == '$' && mode != scaStringSimple:
			tok := scaTokInterpStringMiddle
			switch mode {
			case scaStringInterp:
				if isMultiline {
					tok = scaTokInterpMultiStringMiddle
				} else {
					tok = scaTokInterpStringMiddle
				}
			case scaStringRaw:
				if isMultiline {
					tok = scaTokRawMultiStringMiddle
				} else {
					tok = scaTokRawStringMiddle
				}
			}
			lexer.SetResultSymbol(symbols[tok])
			lexer.MarkEnd()
			return true, tok
		default:
			closingQuoteCount = 0
			switch {
			case lexer.Lookahead() == '\\':
				// Multiline strings ignore escape sequences.
				if isMultiline || mode == scaStringRaw {
					lexer.Advance(false)
					// In single-line raw strings, `\"` is not translated to
					// `"`, but it also does not close the string. Likewise,
					// `\\` is not translated to `\`, but it does stop the
					// second `\` from stopping a double-quote from closing
					// the string.
					if !isMultiline && mode == scaStringRaw &&
						(lexer.Lookahead() == '"' || lexer.Lookahead() == '\\') {
						lexer.Advance(false)
					}
				} else {
					tok := scaTokSimpleStringMiddle
					if mode != scaStringSimple {
						tok = scaTokInterpStringMiddle
					}
					lexer.SetResultSymbol(symbols[tok])
					lexer.MarkEnd()
					return true, tok
				}
			case lexer.Lookahead() == '\n' && !isMultiline:
				return false, -1
			case scaIsEOF(lexer):
				return false, -1
			default:
				lexer.Advance(false)
			}
		}
	}
}

// ---- block comments ----

func scaConsumeBlockCommentBodyEx(lexer *gotreesitter.ExternalLexer, stopAtNewline bool) bool {
	depth := 1
	for depth > 0 {
		if scaIsEOF(lexer) || (stopAtNewline && lexer.Lookahead() == '\n') {
			return false
		}
		switch lexer.Lookahead() {
		case '/':
			lexer.Advance(false)
			if lexer.Lookahead() == '*' {
				lexer.Advance(false)
				depth++
			}
		case '*':
			lexer.Advance(false)
			if lexer.Lookahead() == '/' {
				lexer.Advance(false)
				depth--
			}
		default:
			lexer.Advance(false)
		}
	}
	return true
}

// scaConsumeBlockCommentBody stops just past the matching "*/" (or at EOF).
func scaConsumeBlockCommentBody(lexer *gotreesitter.ExternalLexer) {
	scaConsumeBlockCommentBodyEx(lexer, false)
}

// scaConsumeBlockCommentBodyOnLine stops at a newline. It reports whether
// the comment ended on the line it started on.
func scaConsumeBlockCommentBodyOnLine(lexer *gotreesitter.ExternalLexer) bool {
	return scaConsumeBlockCommentBodyEx(lexer, true)
}

func scaFinishBlockComment(lexer *gotreesitter.ExternalLexer, symbols *[scaTokenCount]gotreesitter.Symbol) bool {
	lexer.MarkEnd()
	lexer.SetResultSymbol(symbols[scaTokBlockComment])
	return true
}

// scaLexBlockComment is called with the leading "/*" already consumed.
func scaLexBlockComment(lexer *gotreesitter.ExternalLexer, symbols *[scaTokenCount]gotreesitter.Symbol) bool {
	scaConsumeBlockCommentBody(lexer)
	return scaFinishBlockComment(lexer, symbols)
}

// ---- word and keyword helpers ----

func scaScanWord(lexer *gotreesitter.ExternalLexer, word string) bool {
	for i := 0; i < len(word); i++ {
		if lexer.Lookahead() != rune(word[i]) {
			return false
		}
		lexer.Advance(false)
	}
	// `match_` must not match the keyword `match`.
	la := lexer.Lookahead()
	return !(scaIsAlnum(la) || la == '_' || la == '$')
}

// scaReadWord reads one identifier-like word. It returns length -1 when the
// word cannot be an ASCII keyword (too long for capacity, or non-ASCII). The
// whole word is always consumed, so a failed keyword check never leaves the
// lexer mid-identifier. underscoreTail reports a final `_` that is not the
// whole word.
func scaReadWord(lexer *gotreesitter.ExternalLexer, capacity int) (word string, length int, underscoreTail bool) {
	var buf []byte
	count := 0
	notKeyword := false
	var last rune
	for scaIsAlnum(lexer.Lookahead()) || lexer.Lookahead() == '_' || lexer.Lookahead() == '$' {
		c := lexer.Lookahead()
		if c > 127 || len(buf) >= capacity-1 {
			notKeyword = true
		} else {
			buf = append(buf, byte(c))
		}
		last = c
		count++
		lexer.Advance(false)
	}
	underscoreTail = count > 1 && last == '_'
	if notKeyword {
		return string(buf), -1, underscoreTail
	}
	return string(buf), len(buf), underscoreTail
}

func scaWordIn(word string, words []string) bool {
	for _, w := range words {
		if word == w {
			return true
		}
	}
	return false
}

// scaSkipBlanksAndBlockComments skips blanks and any block comments between
// them; block comments are transparent, as they are to the parser.
func scaSkipBlanksAndBlockComments(lexer *gotreesitter.ExternalLexer) {
	for {
		scaAdvancePastBlanks(lexer)
		if lexer.Lookahead() != '/' {
			return
		}
		lexer.Advance(false)
		if lexer.Lookahead() != '*' {
			return
		}
		lexer.Advance(false)
		scaConsumeBlockCommentBody(lexer)
	}
}

// scaNameFollowsWord reports whether a name follows the word just read,
// which is what makes an `end` tag a marker and an `open` a modifier.
func scaNameFollowsWord(lexer *gotreesitter.ExternalLexer) bool {
	scaSkipBlanksAndBlockComments(lexer)
	la := lexer.Lookahead()
	return scaIsAlpha(la) || la == '_' || la == '$' || la == '`' || la > 127
}

// scaExpressionTails are the words that continue the expression before them
// instead of starting one. `if` is missing on purpose, since it does start
// one.
var scaExpressionTails = []string{
	"match", "catch", "finally", "else", "then",
	"do", "yield", "while", "with", "extends",
}

func scaWordIsExpressionTail(lexer *gotreesitter.ExternalLexer) bool {
	word, length, _ := scaReadWord(lexer, 8) // sizeof "finally"
	return length > 0 && scaWordIn(word, scaExpressionTails)
}

// scaModifierWordAllowed reports that no modifier precedes such a word, so
// `update match` stays a plain name.
func scaModifierWordAllowed(lexer *gotreesitter.ExternalLexer) bool {
	return !scaWordIsExpressionTail(lexer)
}

// scaOperandWordAllowed reports that the right operand of an operator has to
// start an expression, so `??? match` on its own line is a statement rather
// than the tail of the line above.
func scaOperandWordAllowed(lexer *gotreesitter.ExternalLexer) bool {
	return !scaWordIsExpressionTail(lexer)
}

func scaModifierNameFollows(lexer *gotreesitter.ExternalLexer) bool {
	return scaNameFollowsWord(lexer) && scaModifierWordAllowed(lexer)
}

// scaDefinitionStarts are read across a line break after `inline`, where
// only a reserved word counts as continuing a modifier list.
var scaDefinitionStarts = []string{
	"def", "val", "var", "type", "given", "class",
	"object", "trait", "enum", "final", "lazy", "override",
	"private", "protected", "sealed", "abstract", "implicit",
}

// scaInlineModifierFollows reports whether `inline` is a modifier here.
// `inline` also prefixes the scrutinee of `inline 1 match`, and it can end a
// modifier line whose definition is on the next one.
func scaInlineModifierFollows(lexer *gotreesitter.ExternalLexer) bool {
	if scaNameFollowsWord(lexer) {
		return scaModifierWordAllowed(lexer)
	}
	// A scrutinee starts an expression. See canStartExprTokens in the
	// reference parser.
	la := lexer.Lookahead()
	if (la >= '0' && la <= '9') || la == '"' || la == '\'' || la == '(' || la == '{' || la == '-' {
		return true
	}
	if la != '\n' && la != '\r' {
		return false
	}
	for scaIsSpace(lexer.Lookahead()) {
		lexer.Advance(false)
	}
	word, length, _ := scaReadWord(lexer, 12) // sizeof "transparent"
	return length > 0 && scaWordIn(word, scaDefinitionStarts)
}

// scaIsCaseDefinitionWord reports whether `class` or `object` follows
// `case`, marking a definition not a clause. Advances the lexer.
func scaIsCaseDefinitionWord(lexer *gotreesitter.ExternalLexer) bool {
	scaAdvancePastBlanks(lexer)
	word, length, _ := scaReadWord(lexer, 7) // sizeof "object"
	return length > 0 && (word == "class" || word == "object")
}

// scaIsCaseClauseIntro reports whether the line starts a `case` clause and
// not a `case class`/`case object` definition. Advances the lexer.
func scaIsCaseClauseIntro(lexer *gotreesitter.ExternalLexer) bool {
	return scaScanWord(lexer, "case") && !scaIsCaseDefinitionWord(lexer)
}

// ---- comment-at-layout probing ----

type scaCommentAtLayout int

const (
	scaCommentNone         scaCommentAtLayout = iota // not a comment
	scaCommentLexed                                  // a BLOCK_COMMENT was lexed: return true
	scaCommentAbort                                  // a comment starts here: give up on the layout token
	scaCommentSameLineCode                           // comments skipped, code follows on the same line
)

// scaCheckCommentAtLayout probes for a comment at an INDENT/OUTDENT
// boundary. Comments must not affect indentation. Block comments have to be
// lexed here because the internal lexer no longer knows them.
func scaCheckCommentAtLayout(
	lexer *gotreesitter.ExternalLexer,
	isValid func(int) bool,
	symbols *[scaTokenCount]gotreesitter.Symbol,
) scaCommentAtLayout {
	if lexer.Lookahead() != '/' {
		return scaCommentNone
	}
	lexer.Advance(false)
	if lexer.Lookahead() == '*' && isValid(scaTokBlockComment) {
		lexer.Advance(false)
		for {
			scaConsumeBlockCommentBody(lexer)
			scaAdvancePastBlanks(lexer)
			if scaIsEOF(lexer) || lexer.Lookahead() == '\n' || lexer.Lookahead() == '\r' {
				scaFinishBlockComment(lexer, symbols)
				return scaCommentLexed
			}
			if lexer.Lookahead() == '/' {
				lexer.Advance(false)
				if lexer.Lookahead() == '*' {
					lexer.Advance(false)
					continue // another block comment on the same line
				}
				if lexer.Lookahead() == '/' {
					// The extent cannot be re-marked back, so consume the
					// trailing line comment into the comment token.
					for !scaIsEOF(lexer) && lexer.Lookahead() != '\n' {
						lexer.Advance(false)
					}
					scaFinishBlockComment(lexer, symbols)
					return scaCommentLexed
				}
				// A lone '/' is code. The off-by-one column is harmless.
				return scaCommentSameLineCode
			}
			return scaCommentSameLineCode
		}
	}
	if lexer.Lookahead() == '/' || lexer.Lookahead() == '*' {
		return scaCommentAbort
	}
	// A lone '/' is not a comment. The lexer stays advanced past it.
	return scaCommentNone
}

// ---- delimited-body skipping ----

// scaSkipDelimited skips a delimited body (a string or a back-ticked
// identifier) whose opening delimiter the caller has consumed. Stops after
// the closing `close` or at EOL/EOF.
func scaSkipDelimited(lexer *gotreesitter.ExternalLexer, close rune, escapes bool) {
	for !scaIsEOF(lexer) && lexer.Lookahead() != close && lexer.Lookahead() != '\n' {
		if escapes && lexer.Lookahead() == '\\' {
			lexer.Advance(false)
		}
		lexer.Advance(false)
	}
	if lexer.Lookahead() == close {
		lexer.Advance(false)
	}
}

// scaSkipCharOrQuoteTail runs after an opening `'`. A character literal
// reveals no code and returns 0. A quote reveals its first character, which
// the caller still needs for bracket depth, so that character is returned
// instead.
func scaSkipCharOrQuoteTail(lexer *gotreesitter.ExternalLexer) rune {
	if lexer.Lookahead() == '\\' {
		lexer.Advance(false)
		if !scaIsEOF(lexer) && lexer.Lookahead() != '\n' {
			lexer.Advance(false)
		}
		if lexer.Lookahead() == '\'' {
			lexer.Advance(false)
		}
		return 0
	}
	if scaIsEOF(lexer) || lexer.Lookahead() == '\n' {
		return 0
	}
	quoted := lexer.Lookahead()
	lexer.Advance(false)
	if lexer.Lookahead() == '\'' {
		lexer.Advance(false) // a character literal: `quoted` was not code
		return 0
	}
	return quoted
}

// scaRestOfLineIsBlankOrComments reports whether the rest of the line holds
// only blanks and comments.
func scaRestOfLineIsBlankOrComments(lexer *gotreesitter.ExternalLexer) bool {
	for {
		scaAdvancePastBlanks(lexer)
		if scaIsEOF(lexer) || lexer.Lookahead() == '\n' || lexer.Lookahead() == '\r' {
			return true
		}
		if lexer.Lookahead() != '/' {
			return false
		}
		lexer.Advance(false)
		if lexer.Lookahead() == '/' {
			for !scaIsEOF(lexer) && lexer.Lookahead() != '\n' {
				lexer.Advance(false)
			}
			return true
		}
		if lexer.Lookahead() != '*' {
			return false // a lone '/' is code
		}
		lexer.Advance(false)
		if !scaConsumeBlockCommentBodyOnLine(lexer) {
			return true // the comment runs past the line end
		}
	}
}

// scaHasOperand reports whether a real operand follows the operator.
// Comments and line breaks are transparent.
func scaHasOperand(lexer *gotreesitter.ExternalLexer) bool {
	for {
		if scaIsEOF(lexer) {
			return false
		}
		if scaIsSpace(lexer.Lookahead()) {
			lexer.Advance(false)
			continue
		}
		if lexer.Lookahead() != '/' {
			return !scaIsCloseOrSeparator(lexer.Lookahead())
		}
		lexer.Advance(false)
		if lexer.Lookahead() == '/' {
			for !scaIsEOF(lexer) && lexer.Lookahead() != '\n' {
				lexer.Advance(false)
			}
			continue
		}
		if lexer.Lookahead() != '*' {
			return true // a lone '/' is code
		}
		lexer.Advance(false)
		scaConsumeBlockCommentBody(lexer)
	}
}

// scaReadOpChars reads the run of opchars at the lookahead. Only the first 3
// are kept, since a longer run is never a symbolic keyword. Returns the full
// length and advances the lexer past the run.
func scaReadOpChars(lexer *gotreesitter.ExternalLexer) (string, int) {
	var buf [3]byte
	length := 0
	for scaIsOpChar(lexer.Lookahead()) {
		if length < 3 {
			buf[length] = byte(lexer.Lookahead())
		}
		length++
		lexer.Advance(false)
	}
	capLen := length
	if capLen > 3 {
		capLen = 3
	}
	return string(buf[:capLen]), length
}

// scaIsSymbolicKeyword reports the keywords made of opchars, `<%` aside,
// which is Scala 2 only. Each lexes as its own token and never as an
// identifier.
func scaIsSymbolicKeyword(op string, length int) bool {
	switch length {
	case 1:
		return op[0] == '=' || op[0] == ':' || op[0] == '#' || op[0] == '@'
	case 2:
		return (op[0] == '=' && op[1] == '>') ||
			(op[0] == '<' && (op[1] == '-' || op[1] == ':' || op[1] == '%')) ||
			(op[0] == '>' && op[1] == ':')
	case 3:
		return (op[0] == '?' && op[1] == '=' && op[2] == '>') ||
			(op[0] == '=' && op[1] == '>' && op[2] == '>')
	default:
		return false
	}
}

// scaIsUnaryOp reports the operator names that can start an expression.
// Every other one is binary, so it leaves no operand for the operator ahead
// of it. See isUnary in the reference compiler.
func scaIsUnaryOp(op string, length int) bool {
	return length == 1 && (op[0] == '-' || op[0] == '+' || op[0] == '~' || op[0] == '!')
}

// scaOperandFollows reports blanks and then something that can be the right
// operand.
func scaOperandFollows(lexer *gotreesitter.ExternalLexer) bool {
	if !scaAdvancePastBlanks(lexer) || !scaHasOperand(lexer) {
		return false
	}
	if scaIsOpChar(lexer.Lookahead()) {
		op, length := scaReadOpChars(lexer)
		// This also covers the symbolic keywords, none of which is unary.
		return scaIsUnaryOp(op, length)
	}
	return scaOperandWordAllowed(lexer)
}

// scaIsLeadingInfixContinuation reports whether the lookahead starts a
// leading infix operator — a symbolic operator or back-ticked identifier
// followed by whitespace and then an operand. Such a line is a continuation
// of the previous expression, so neither AUTOMATIC_SEMICOLON nor OUTDENT
// should fire ahead of it. Advances the lexer; the caller must not rely on
// position.
func scaIsLeadingInfixContinuation(lexer *gotreesitter.ExternalLexer) bool {
	if scaIsOpChar(lexer.Lookahead()) {
		op, length := scaReadOpChars(lexer)
		if scaIsSymbolicKeyword(op, length) {
			return false
		}
		return scaOperandFollows(lexer)
	}
	if lexer.Lookahead() == '`' {
		lexer.Advance(false)
		for lexer.Lookahead() != '`' && !scaIsEOF(lexer) {
			lexer.Advance(false)
		}
		if lexer.Lookahead() != '`' {
			return false
		}
		lexer.Advance(false)
		return scaOperandFollows(lexer)
	}
	return false
}

// scaLineScan is the result of walking the rest of the line, tracking
// bracket depth and skipping strings, characters, back-ticks and comments.
type scaLineScan struct {
	endsConditional bool
	hasCaseArrow    bool
	closesBracket   bool
}

// scaScanRestOfLine reports whether the line ends in a depth-0 `then`/`do`
// and whether it carries a depth-0 `=>` (Scala 2 `⇒`).
func scaScanRestOfLine(lexer *gotreesitter.ExternalLexer) scaLineScan {
	depth := 0
	var r scaLineScan
	for !scaIsEOF(lexer) && lexer.Lookahead() != '\n' && lexer.Lookahead() != '\r' {
		c := lexer.Lookahead()
		switch {
		case c == ' ' || c == '\t':
			lexer.Advance(false)
		case c == '(' || c == '[' || c == '{':
			depth++
			r.endsConditional = false
			lexer.Advance(false)
		case c == ')' || c == ']' || c == '}':
			depth--
			if depth < 0 {
				r.closesBracket = true
			}
			r.endsConditional = false
			lexer.Advance(false)
		case c == '"' || c == '`':
			// A string or a back-ticked identifier; only a string honours
			// escapes.
			r.endsConditional = false
			lexer.Advance(false)
			scaSkipDelimited(lexer, c, c == '"')
		case c == '\'':
			r.endsConditional = false
			lexer.Advance(false)
			quoted := scaSkipCharOrQuoteTail(lexer)
			if quoted == '(' || quoted == '[' || quoted == '{' {
				depth++
			} else if quoted == ')' || quoted == ']' || quoted == '}' {
				depth--
			}
		case c == '/':
			lexer.Advance(false)
			if lexer.Lookahead() == '/' {
				return r // rest of the line is a comment
			}
			if lexer.Lookahead() == '*' {
				lexer.Advance(false)
				if !scaConsumeBlockCommentBodyOnLine(lexer) {
					return r // the comment runs past the line end
				}
			} else {
				r.endsConditional = false // a lone '/' is code
			}
		case c == 0x21D2: // `⇒`, the Scala 2 spelling of `=>`
			if depth == 0 {
				r.hasCaseArrow = true
			}
			r.endsConditional = false
			lexer.Advance(false)
		case scaIsOpChar(c):
			startsEq := c == '='
			lexer.Advance(false)
			secondGT := lexer.Lookahead() == '>'
			extra := 0
			for scaIsOpChar(lexer.Lookahead()) {
				lexer.Advance(false)
				extra++
			}
			// A plain `=>` at depth 0, not `==>` or `=>>`.
			if depth == 0 && startsEq && secondGT && extra == 1 {
				r.hasCaseArrow = true
			}
			r.endsConditional = false
		case scaIsWordStart(c):
			word, length, _ := scaReadWord(lexer, 5) // sizeof "then"
			r.endsConditional = depth == 0 && length > 0 && (word == "then" || word == "do")
		default:
			r.endsConditional = false
			lexer.Advance(false)
		}
	}
	return r
}

// scaConsumeDigitGroup consumes `digit (digit | '_' digit)*`. It sets
// *sawSep if a separator appears.
func scaConsumeDigitGroup(lexer *gotreesitter.ExternalLexer, sawSep *bool) bool {
	if lexer.Lookahead() < '0' || lexer.Lookahead() > '9' {
		return false
	}
	lexer.Advance(false)
	for {
		if lexer.Lookahead() >= '0' && lexer.Lookahead() <= '9' {
			lexer.Advance(false)
		} else if lexer.Lookahead() == '_' {
			lexer.Advance(false)
			if lexer.Lookahead() < '0' || lexer.Lookahead() > '9' {
				return false
			}
			if sawSep != nil {
				*sawSep = true
			}
			lexer.Advance(false)
		} else {
			return true
		}
	}
}

// scaScanFloatWithSeparator lexes a floating point literal whose integer
// part uses digit separators, e.g. `1_000.5`. The internal regex cannot:
// after an underscore-containing group its DFA loses the transition to the
// following `.`.
func scaScanFloatWithSeparator(lexer *gotreesitter.ExternalLexer, symbols *[scaTokenCount]gotreesitter.Symbol) bool {
	intSep := false
	if !scaConsumeDigitGroup(lexer, &intSep) {
		return false
	}
	// Only the integer-part separator is ours. Without one the internal
	// lexer lexes the number, so bail before walking the fraction and
	// exponent.
	if !intSep {
		return false
	}
	isFloat := false
	if lexer.Lookahead() == '.' {
		lexer.Advance(false)
		if !scaConsumeDigitGroup(lexer, nil) {
			return false
		}
		isFloat = true
	}
	if lexer.Lookahead() == 'e' || lexer.Lookahead() == 'E' {
		lexer.Advance(false)
		if lexer.Lookahead() == '+' || lexer.Lookahead() == '-' {
			lexer.Advance(false)
		}
		if !scaConsumeDigitGroup(lexer, nil) {
			return false
		}
		isFloat = true
	}
	if lexer.Lookahead() == 'd' || lexer.Lookahead() == 'D' ||
		lexer.Lookahead() == 'f' || lexer.Lookahead() == 'F' {
		lexer.Advance(false)
		isFloat = true
	}
	// A bare integer with separators (`1_000`) is the internal lexer's job.
	if !isFloat {
		return false
	}
	lexer.MarkEnd()
	lexer.SetResultSymbol(symbols[scaTokFloatingPointWithSeparators])
	return true
}

// scaSoftModifier is one soft-keyword modifier: lexing it here means the
// parser only sees one alternative where a modifier can stand, instead of
// one per name position.
type scaSoftModifier struct {
	first byte
	word  string
	tok   int
}

var scaSoftModifiers = []scaSoftModifier{
	{'e', "erased", scaTokErasedModifier},
	{'o', "open", scaTokOpenModifier},
	{'o', "opaque", scaTokOpaqueModifier},
	{'i', "infix", scaTokInfixModifier},
	{'t', "tracked", scaTokTrackedModifier},
	{'t', "transparent", scaTokTransparentModifier},
	{'i', "inline", scaTokInlineModifier},
	{'i', "into", scaTokIntoModifier},
	{'u', "update", scaTokUpdateModifier},
	{'c', "consume", scaTokConsumeModifier},
}

var scaBlockClosingWords = []string{"else", "catch", "finally"}

var scaBlockOpeningStoppers = []string{"else", "catch", "finally", "yield", "do"}

var scaGatedStoppers = []string{"else", "catch", "finally", "yield"}

var scaContinuingWords = []struct {
	word  string
	tok   int
	gated bool
}{
	{"else", scaTokElse, true},
	{"catch", scaTokCatch, true},
	{"finally", scaTokFinally, true},
	{"extends", scaTokExtends, false},
	{"with", scaTokWith, false},
	{"derives", scaTokDerives, false},
	{"uses", scaTokUses, false},
}

var scaReservedOps = []string{"=", "#", "@", "=>", "<-", "<:", ">:", "<%", "?=>", "=>>"}

var scaDefinitionWords = []string{
	"abstract", "class", "def", "enum", "export", "final",
	"given", "import", "implicit", "lazy", "object", "override",
	"package", "private", "protected", "sealed", "trait", "type",
	"val", "var",
}

var scaTailWords = []string{"catch", "else", "finally", "then", "yield"}

// scaScanImpl is the direct port of tree-sitter-scala's scan_impl. It
// returns whether a token was produced and, when true, which scanner token
// index was set as the result symbol (used by Scan to track the
// fewer-braces colon flag).
func scaScanImpl(
	scanner *scalaState,
	lexer *gotreesitter.ExternalLexer,
	isValid func(int) bool,
	symbols *[scaTokenCount]gotreesitter.Symbol,
) (bool, int) {
	// The `>` of a using directive. It is immediate after the `//`, so this
	// has to run before the blanks are skipped below.
	if isValid(scaTokUsingDirectiveStart) && !isValid(scaTokErrorSentinel) && lexer.Lookahead() == '>' {
		lexer.Advance(false)
		lexer.MarkEnd()
		scaAdvancePastBlanks(lexer)
		if !scaScanWord(lexer, "using") {
			return false, -1
		}
		lexer.SetResultSymbol(symbols[scaTokUsingDirectiveStart])
		return true, scaTokUsingDirectiveStart
	}

	// The grammar takes this `?` only in front of a type lambda, so the
	// bracket has to be here before the word is handed over. The blanks are
	// skipped rather than advanced, which keeps them out of the token.
	if isValid(scaTokWildcardBoundStart) && !isValid(scaTokErrorSentinel) {
		for lexer.Lookahead() == ' ' || lexer.Lookahead() == '\t' {
			lexer.Advance(true)
		}
	}
	if isValid(scaTokWildcardBoundStart) && !isValid(scaTokErrorSentinel) && lexer.Lookahead() == '?' {
		lexer.Advance(false)
		lexer.MarkEnd()
		scaAdvancePastBlanks(lexer)
		if lexer.Lookahead() != '<' && lexer.Lookahead() != '>' {
			return false, -1
		}
		lexer.Advance(false)
		if lexer.Lookahead() != ':' {
			return false, -1
		}
		lexer.Advance(false)
		scaAdvancePastBlanks(lexer)
		if lexer.Lookahead() != '[' {
			return false, -1
		}
		lexer.SetResultSymbol(symbols[scaTokWildcardBoundStart])
		return true, scaTokWildcardBoundStart
	}

	prev := int16(-1)
	if len(scanner.indents) > 0 {
		prev = scanner.indents[len(scanner.indents)-1]
	}
	prevWidth := scaIndentWidth(prev)
	var newlineCount int16
	var indentationSize int16

	for scaIsSpace(lexer.Lookahead()) {
		if lexer.Lookahead() == '\n' {
			newlineCount++
			indentationSize = 0
		} else {
			indentationSize++
		}
		lexer.Advance(true)
	}

	// COMMA_OUTDENT is a distinct token so tree-sitter only makes it valid
	// where comma termination is expected (colon_argument,
	// _indentable_expression).
	if isValid(scaTokCommaOutdent) && lexer.Lookahead() == ',' && prev != -1 {
		lexer.MarkEnd()
		// Error recovery makes every symbol valid, so keep the eager pop
		// there.
		if !isValid(scaTokErrorSentinel) {
			lexer.Advance(false)
			endsLine := false
			for {
				scaAdvancePastBlanks(lexer)
				if scaIsEOF(lexer) || lexer.Lookahead() == '\n' || lexer.Lookahead() == '\r' {
					endsLine = true
					break
				}
				if lexer.Lookahead() != '/' {
					break
				}
				lexer.Advance(false)
				if lexer.Lookahead() == '/' {
					endsLine = true // line comment runs to the line end
					break
				}
				if lexer.Lookahead() != '*' {
					break // a lone '/' is code
				}
				lexer.Advance(false)
				scaConsumeBlockCommentBody(lexer)
			}
			// POSTFIX_OP is offered only where an operand just ended, so
			// the comma separates arguments rather than continuing a list
			// of names. The closer catches what is left, where no operand
			// ended before the comma.
			if !endsLine && !isValid(scaTokPostfixOp) && !scaScanRestOfLine(lexer).closesBracket {
				return false, -1
			}
		}
		if len(scanner.indents) > 0 {
			scanner.indents = scanner.indents[:len(scanner.indents)-1]
		}
		lexer.SetResultSymbol(symbols[scaTokCommaOutdent])
		return true, scaTokCommaOutdent
	}

	// Closes a same-width case region mid-cascade after a deeper region
	// popped and the line sits at this region's width. It stays open only
	// for another `case` clause.
	if isValid(scaTokOutdent) && !isValid(scaTokErrorSentinel) &&
		scanner.lastIndentationSize != -1 &&
		scaAtCaseRegionWidth(prev, scanner.lastIndentationSize) &&
		func() bool {
			if scaIsEOF(lexer) {
				return scanner.lastColumn == -1
			}
			return int16(lexer.Column()) == scanner.lastColumn
		}() {
		lexer.MarkEnd()
		if scaIsCaseClauseIntro(lexer) {
			return false, -1
		}
		if len(scanner.indents) > 0 {
			scanner.indents = scanner.indents[:len(scanner.indents)-1]
		}
		lexer.SetResultSymbol(symbols[scaTokOutdent])
		return true, scaTokOutdent
	}

	// Before advancing the lexer, check if we can double outdent.
	if isValid(scaTokOutdent) &&
		(lexer.Lookahead() == 0 ||
			(prev != -1 && (lexer.Lookahead() == ')' || lexer.Lookahead() == ']' || lexer.Lookahead() == '}')) ||
			(scanner.lastIndentationSize != -1 && prev != -1 && scanner.lastIndentationSize < prevWidth)) {
		if len(scanner.indents) > 0 {
			scanner.indents = scanner.indents[:len(scanner.indents)-1]
		}
		lexer.SetResultSymbol(symbols[scaTokOutdent])
		return true, scaTokOutdent
	}
	scanner.lastIndentationSize = -1

	// True when the line is deeper than the current region. An empty stack
	// has prevWidth == -1, so any width counts as deeper. A same-width line
	// right after a fewer-braces colon also opens a block.
	indentGeometry := indentationSize > prevWidth ||
		(scanner.afterColonEol && indentationSize == prevWidth)
	if isValid(scaTokIndent) && newlineCount > 0 &&
		// An indented block cannot start with a closing delimiter.
		lexer.Lookahead() != '}' && lexer.Lookahead() != ')' && lexer.Lookahead() != ']' &&
		(indentGeometry ||
			// A comment at exactly the region width can hide a deeper line
			// behind it. Probe it and let the code's column decide below.
			(indentationSize == prevWidth && lexer.Lookahead() == '/')) {
		lexer.MarkEnd()
		switch scaCheckCommentAtLayout(lexer, isValid, symbols) {
		case scaCommentLexed:
			return true, scaTokBlockComment
		case scaCommentAbort:
			return false, -1
		case scaCommentSameLineCode:
			// The line's content starts after the comment; its column is
			// the effective indentation.
			effective := int16(lexer.Column())
			if effective > prevWidth {
				indentationSize = effective
			} else {
				// The code does not open a block after all, so lex the
				// comment.
				return scaFinishBlockComment(lexer, symbols), scaTokBlockComment
			}
		case scaCommentNone:
			if !indentGeometry {
				return false, -1 // entered only to probe a comment; this is code
			}
		}
		// An indented block cannot start with else/catch/finally, nor with
		// yield/do, which belong to an enclosing `for (...)` whose body is
		// also indentable.
		entry := indentationSize
		switch lexer.Lookahead() {
		case 'e', 'c', 'f', 'y', 'd':
			word, length, _ := scaReadWord(lexer, 8) // sizeof "finally"
			if length > 0 && scaWordIn(word, scaBlockOpeningStoppers) {
				// The keyword belongs to an enclosing construct. Where its
				// gate is valid and the word is a gated one, emit it
				// (zero width) so the keyword can shift; otherwise no
				// block opens here.
				if isValid(scaTokControlTailGate) && scaWordIn(word, scaGatedStoppers) {
					lexer.SetResultSymbol(symbols[scaTokControlTailGate])
					return true, scaTokControlTailGate
				}
				return false, -1
			}
			// At top level the stack is empty, so the same-width `case`
			// close at the bottom never sees a width-0 `match`/`case`.
			// Flag the region here instead.
			if len(scanner.indents) == 0 && length == 4 && word == "case" &&
				!scaIsCaseDefinitionWord(lexer) && scaScanRestOfLine(lexer).hasCaseArrow {
				entry |= scaCaseIndentFlag
			}
		case '|', '&':
			// Nor with a leading `|`/`&` infix operator, which continues
			// the previous expression. Only these two run here, and not
			// after a colon, where the line can only be the body.
			if !scanner.afterColonEol && scaIsLeadingInfixContinuation(lexer) {
				return false, -1
			}
		}
		scanner.indents = append(scanner.indents, entry)
		lexer.SetResultSymbol(symbols[scaTokIndent])
		return true, scaTokIndent
	}

	// This saves the indentation_size and newline_count so it can be used
	// in subsequent calls for multiple outdent or auto-semicolon. A
	// same-width case region also closes on a line at its own width when
	// that line does not start another `case` clause.
	caseRegionClose := newlineCount > 0 && scaAtCaseRegionWidth(prev, indentationSize)
	if isValid(scaTokOutdent) &&
		(lexer.Lookahead() == 0 ||
			(newlineCount > 0 && prev != -1 && indentationSize < prevWidth) ||
			caseRegionClose) {
		lexer.MarkEnd()
		switch scaCheckCommentAtLayout(lexer, isValid, symbols) {
		case scaCommentLexed:
			return true, scaTokBlockComment
		case scaCommentAbort:
			return false, -1
		case scaCommentSameLineCode:
			// The line's content starts after the comment; its column is
			// the effective indentation.
			effective := int16(lexer.Column())
			if effective < prevWidth {
				indentationSize = effective
			} else {
				// Not an outdent after all. The automatic semicolon has
				// its own suppression rules, so lex the comment and let
				// the next scan decide, carrying the pending newline
				// through the recovery above.
				scanner.lastNewlineCount = newlineCount
				scanner.lastColumn = effective
				scanner.lastChar = int16(lexer.Lookahead() & 0x7FFF)
				return scaFinishBlockComment(lexer, symbols), scaTokBlockComment
			}
		case scaCommentNone:
			// nothing to do
		}
		scanner.lastIndentationSize = indentationSize
		scanner.lastNewlineCount = newlineCount
		if scaIsEOF(lexer) {
			scanner.lastColumn = -1
			scanner.lastChar = 0
		} else {
			scanner.lastColumn = int16(lexer.Column())
			scanner.lastChar = int16(lexer.Lookahead() & 0x7FFF)
		}
		// Keep the indented block open when the next line starts with a
		// leading infix operator, which continues the previous expression.
		// But a line ending in depth-0 `then`/`do` closes the regions up to
		// its conditional.
		if lexer.Lookahead() != 0 && scaIsLeadingInfixContinuation(lexer) && !scaScanRestOfLine(lexer).endsConditional {
			return false, -1
		}
		// A same-width `case` line continues the case region instead.
		if caseRegionClose && scaIsCaseClauseIntro(lexer) {
			return false, -1
		}
		if len(scanner.indents) > 0 {
			scanner.indents = scanner.indents[:len(scanner.indents)-1]
		}
		lexer.SetResultSymbol(symbols[scaTokOutdent])
		return true, scaTokOutdent
	}

	// Recover newline_count from the outdent reset. Skipped when this scan
	// crossed a newline itself, because the saved count belongs to an
	// earlier line at the same column.
	if scanner.lastNewlineCount > 0 {
		isEOF := scaIsEOF(lexer)
		if (isEOF && scanner.lastColumn == -1) ||
			(!isEOF && newlineCount == 0 &&
				int16(lexer.Lookahead()&0x7FFF) == scanner.lastChar &&
				lexer.Column() == uint32(scanner.lastColumn)) {
			newlineCount += scanner.lastNewlineCount
		}
	}
	scanner.lastNewlineCount = 0

	// END_KEYWORD is only valid right after the semicolon the grammar puts
	// in front of a marker. Emit it when the tag word confirms the marker
	// shape.
	if isValid(scaTokEndKeyword) && !isValid(scaTokErrorSentinel) && lexer.Lookahead() == 'e' {
		if scaScanWord(lexer, "end") && scaNameFollowsWord(lexer) {
			lexer.MarkEnd()
			lexer.SetResultSymbol(symbols[scaTokEndKeyword])
			return true, scaTokEndKeyword
		}
		return false, -1
	}

	// Runs before the plain semicolon below, so the header break wins where
	// the parser offers it.
	if isValid(scaTokDefSemicolon) && !isValid(scaTokErrorSentinel) &&
		newlineCount > 0 &&
		(lexer.Lookahead() == '(' || lexer.Lookahead() == '[' || lexer.Lookahead() == ':') {
		lexer.MarkEnd()
		lexer.SetResultSymbol(symbols[scaTokDefSemicolon])
		return true, scaTokDefSemicolon
	}

	if isValid(scaTokAutoSemicolon) && newlineCount > 0 {
		lexer.MarkEnd()
		resultTok := scaTokAutoSemicolon

		// Probably, a multi-line field expression, e.g. `a` + `.b` + `.c`.
		if lexer.Lookahead() == '.' {
			return false, -1
		}

		// A statement never ends right before a closing bracket.
		if lexer.Lookahead() == ')' || lexer.Lookahead() == ']' || lexer.Lookahead() == ',' {
			return false, -1
		}

		// Keeps a braced else-if chain from forking one marker head per
		// nested if. A `}` line that continues with more code keeps the
		// semicolon and the old reading of its tail.
		if lexer.Lookahead() == '}' {
			lexer.Advance(false)
			for {
				scaAdvancePastBlanks(lexer)
				if lexer.Lookahead() == '}' || lexer.Lookahead() == ')' || lexer.Lookahead() == ']' {
					lexer.Advance(false)
					continue
				}
				if lexer.Lookahead() == '/' {
					lexer.Advance(false)
					if lexer.Lookahead() == '/' {
						return false, -1
					}
					if lexer.Lookahead() == '*' {
						lexer.Advance(false)
						scaConsumeBlockCommentBody(lexer)
						continue
					}
				}
				break
			}
			if lexer.Lookahead() == '\n' || lexer.Lookahead() == '\r' || scaIsEOF(lexer) {
				return false, -1
			}
			lexer.SetResultSymbol(symbols[resultTok])
			return true, resultTok
		}

		// Single-line and multi-line comments.
		if lexer.Lookahead() == '/' {
			lexer.Advance(false)
			if lexer.Lookahead() == '/' {
				return false, -1
			}
			if lexer.Lookahead() == '*' && isValid(scaTokBlockComment) {
				lexer.Advance(false)
				// The suppression rules must also see code after the
				// comment. Lex the comment and carry the pending newline
				// to the next scan through the recovery above.
				scaConsumeBlockCommentBody(lexer)
				scaAdvancePastBlanks(lexer)
				if !(lexer.Lookahead() == '\n' || lexer.Lookahead() == '\r' || scaIsEOF(lexer)) {
					scanner.lastNewlineCount = newlineCount
					scanner.lastColumn = int16(lexer.Column())
					scanner.lastChar = int16(lexer.Lookahead() & 0x7FFF)
				}
				return scaFinishBlockComment(lexer, symbols), scaTokBlockComment
			}
			// A lone '/' falls through with the lexer advanced past it,
			// matching the old flow.
		}

		// A blank line still separates the statements, here and in the
		// word branch below.
		if scaIsOpChar(lexer.Lookahead()) {
			op, opLen := scaReadOpChars(lexer)
			// No statement break before a symbolic keyword, which cannot
			// start a statement. `@` is the one that can, since it opens
			// an annotation.
			if scaIsSymbolicKeyword(op, opLen) {
				if opLen == 1 && op[0] == '@' {
					lexer.SetResultSymbol(symbols[resultTok])
					return true, resultTok
				}
				return false, -1
			}
			if newlineCount == 1 && scaOperandFollows(lexer) {
				return false, -1
			}
			lexer.SetResultSymbol(symbols[resultTok])
			return true, resultTok
		}
		if lexer.Lookahead() == '`' {
			if newlineCount == 1 && scaIsLeadingInfixContinuation(lexer) {
				return false, -1
			}
			lexer.SetResultSymbol(symbols[resultTok])
			return true, resultTok
		}

		// A keyword that continues the enclosing expression suppresses the
		// semicolon, even when several are valid at once.
		if scaIsWordStart(lexer.Lookahead()) {
			word, length, underscoreTail := scaReadWord(lexer, 8) // sizeof "finally"
			// A name whose last characters are operator ones is an
			// operator too, so a line starting with it continues the line
			// above.
			if underscoreTail && scaIsOpChar(lexer.Lookahead()) {
				for scaIsOpChar(lexer.Lookahead()) {
					lexer.Advance(false)
				}
				if newlineCount == 1 && scaOperandFollows(lexer) {
					return false, -1
				}
				lexer.SetResultSymbol(symbols[resultTok])
				return true, resultTok
			}
			if length <= 0 {
				lexer.SetResultSymbol(symbols[resultTok])
				return true, resultTok
			}
			for _, cw := range scaContinuingWords {
				gate := cw.gated && isValid(scaTokControlTailGate)
				if (!gate && !isValid(cw.tok)) || word != cw.word {
					continue
				}
				if gate {
					lexer.SetResultSymbol(symbols[scaTokControlTailGate])
					return true, scaTokControlTailGate
				}
				return false, -1
			}
			// A case clause line needs no separator; a case definition
			// keeps its separator, as does a clause line that closes an
			// enclosing bracket.
			if word == "case" && !scaIsCaseDefinitionWord(lexer) {
				line := scaScanRestOfLine(lexer)
				if line.hasCaseArrow && !line.closesBracket {
					return false, -1
				}
				lexer.SetResultSymbol(symbols[resultTok])
				return true, resultTok
			}
			// `match` is a reserved word that never starts a statement, so
			// it always continues the previous expression.
			if word == "match" {
				return false, -1
			}
		}

		lexer.SetResultSymbol(symbols[resultTok])
		return true, resultTok
	}

	// An else/catch/finally that is not directly shiftable must first close
	// the open indented region, and a soft modifier is the modifier when a
	// name follows. Both read the word, so it is read once here. Skipped
	// during error recovery, where every symbol looks valid.
	outdentArm := isValid(scaTokOutdent) &&
		!isValid(scaTokControlTailGate) &&
		newlineCount == 0 &&
		prev != -1 &&
		((lexer.Lookahead() == 'e' && !isValid(scaTokElse)) ||
			(lexer.Lookahead() == 'c' && !isValid(scaTokCatch)) ||
			(lexer.Lookahead() == 'f' && !isValid(scaTokFinally)))
	// Character first: it rules out most positions without the table load.
	modifierArm := false
	for _, m := range scaSoftModifiers {
		if lexer.Lookahead() == rune(m.first) && isValid(m.tok) {
			modifierArm = true
			break
		}
	}
	// The reader below cannot rewind, so `case` compares there rather than
	// scanning on its own and eating the start of another word.
	caseDefinitionArm := isValid(scaTokCaseDefinitionKeyword) && lexer.Lookahead() == 'c'
	if !isValid(scaTokErrorSentinel) && (outdentArm || modifierArm || caseDefinitionArm) {
		if outdentArm {
			// OUTDENT is zero-width at the word, and the lexer cannot
			// rewind once ReadWord has consumed it.
			lexer.MarkEnd()
		}
		// Sized for the longest word above.
		word, length, _ := scaReadWord(lexer, 12) // sizeof "transparent"
		if length > 0 {
			if caseDefinitionArm && word == "case" {
				lexer.MarkEnd()
				if scaIsCaseDefinitionWord(lexer) {
					lexer.SetResultSymbol(symbols[scaTokCaseDefinitionKeyword])
					return true, scaTokCaseDefinitionKeyword
				}
				return false, -1
			}
			modifier := -1
			for _, m := range scaSoftModifiers {
				if isValid(m.tok) && word == m.word {
					modifier = m.tok
					break
				}
			}
			if modifier != -1 {
				// The modifier spans the word, unlike the zero-width
				// OUTDENT.
				lexer.MarkEnd()
				// A name follows a modifier, so `def open(p)` and
				// `a.infix` keep reading as plain identifiers.
				var follows bool
				if modifier == scaTokInlineModifier {
					follows = scaInlineModifierFollows(lexer)
				} else {
					follows = scaModifierNameFollows(lexer)
				}
				if follows {
					lexer.SetResultSymbol(symbols[modifier])
					return true, modifier
				}
				return false, -1
			}
			if outdentArm && scaWordIn(word, scaBlockClosingWords) {
				if len(scanner.indents) > 0 {
					scanner.indents = scanner.indents[:len(scanner.indents)-1]
				}
				lexer.SetResultSymbol(symbols[scaTokOutdent])
				return true, scaTokOutdent
			}
		}
		// The lexer has advanced past the word; nothing else can match it.
		return false, -1
	}

	for scaIsSpace(lexer.Lookahead()) {
		if lexer.Lookahead() == '\n' {
			newlineCount++
		}
		lexer.Advance(true)
	}

	// XML mode (SLS §10) starts only where the grammar makes XML_TAG_START
	// valid and the `<` is immediately followed by a name-start character.
	if isValid(scaTokXmlTagStart) && !isValid(scaTokErrorSentinel) && lexer.Lookahead() == '<' {
		lexer.Advance(false)
		if scaIsXMLNameStart(lexer.Lookahead()) {
			lexer.MarkEnd()
			lexer.SetResultSymbol(symbols[scaTokXmlTagStart])
			return true, scaTokXmlTagStart
		}
		return false, -1
	}

	// A floating point literal whose integer part uses digit separators.
	if isValid(scaTokFloatingPointWithSeparators) && !isValid(scaTokErrorSentinel) &&
		lexer.Lookahead() >= '0' && lexer.Lookahead() <= '9' {
		if scaScanFloatWithSeparator(lexer, symbols) {
			return true, scaTokFloatingPointWithSeparators
		}
		return false, -1
	}

	// A symbolic operator in postfix position is lexed here, as is the
	// fewer-braces colon. An operator that continues its expression stays
	// with the internal per-class tokens.
	if !isValid(scaTokErrorSentinel) && scaIsOpChar(lexer.Lookahead()) &&
		((isValid(scaTokColonEol) && lexer.Lookahead() == ':') ||
			isValid(scaTokPostfixOp) || isValid(scaTokPostfixStar) ||
			isValid(scaOpLeftClass(lexer.Lookahead())) || isValid(scaTokOpName)) {
		op, opLen := scaReadOpChars(lexer)
		lexer.MarkEnd()
		// A `/` next carries the operator on, so the colon is not a lone
		// one.
		if opLen == 1 && op[0] == ':' && lexer.Lookahead() != '/' {
			// A lone `:` ending its line is the fewer-braces colon.
			if isValid(scaTokColonEol) && scaRestOfLineIsBlankOrComments(lexer) {
				lexer.SetResultSymbol(symbols[scaTokColonEol])
				return true, scaTokColonEol
			}
			return false, -1
		}
		// `#!` opens a script header, whose path is not part of an
		// operator.
		shebang := opLen >= 2 && op[0] == '#' && op[1] == '!'
		opClass := scaOpLeftClass(rune(op[0]))
		if lexer.Lookahead() == '/' && !shebang &&
			(isValid(opClass) || isValid(scaTokOpName)) {
			// One step per OP_STEP in grammar.js.
			endsInSlash := false
			atComment := false
			for !atComment {
				c := lexer.Lookahead()
				if c == '/' {
					lexer.Advance(false)
					atComment = lexer.Lookahead() == '/' || lexer.Lookahead() == '*'
				} else if scaIsOpChar(c) {
					lexer.Advance(false)
				} else {
					break
				}
				endsInSlash = !atComment && c == '/'
			}
			// A Unicode opchar would carry the operator past what this
			// reads, so hand the whole token back to the internal lexer.
			if endsInSlash && lexer.Lookahead() < 0x80 {
				lexer.MarkEnd()
				tok := scaTokOpName
				if isValid(opClass) {
					tok = opClass
				}
				lexer.SetResultSymbol(symbols[tok])
				return true, tok
			}
			return false, -1
		}
		// The branch is also entered for COLON_EOL-only states. Everything
		// from here on emits a postfix token, so bail out early where none
		// is valid.
		postfixSym := scaTokPostfixOp
		if opLen == 1 && op[0] == '*' {
			postfixSym = scaTokPostfixStar
		}
		if !isValid(postfixSym) {
			return false, -1
		}
		// These sequences are not infix operators, so the line does not
		// continue.
		if op[0] == '=' || op[0] == '<' || op[0] == '>' || op[0] == '#' ||
			op[0] == '@' || op[0] == '?' {
			if opLen <= 3 && scaWordIn(op, scaReservedOps) {
				return false, -1
			}
		}
		// A trailing comment counts as the line end because scalac treats
		// it as transparent.
		if !scaRestOfLineIsBlankOrComments(lexer) {
			// Mid-line, the call left the lookahead at the next real
			// character (or just past a lone `/`). An operator directly
			// before a closing delimiter or a list separator is postfix.
			if scaIsCloseOrSeparator(lexer.Lookahead()) {
				lexer.SetResultSymbol(symbols[postfixSym])
				return true, postfixSym
			}
			// The assignment `=` starts no expression either, so the
			// operator is the postfix one an update calls.
			if lexer.Lookahead() == '=' {
				lexer.Advance(false)
				if !scaIsOpChar(lexer.Lookahead()) {
					lexer.SetResultSymbol(symbols[postfixSym])
					return true, postfixSym
				}
			}
			return false, -1
		}
		// The right operand must be able to start an expression.
		if !scaHasOperand(lexer) {
			lexer.SetResultSymbol(symbols[postfixSym])
			return true, postfixSym
		}
		// A `.` or a `=` next also ends the infix reading.
		if lexer.Lookahead() == '.' || lexer.Lookahead() == '=' {
			return false, -1
		}
		// No expression starts with `@` either.
		if lexer.Lookahead() == '@' {
			lexer.SetResultSymbol(symbols[postfixSym])
			return true, postfixSym
		}
		// A next line starting with a definition or modifier keyword
		// begins a new statement, so the operator was postfix.
		if lexer.Lookahead() >= 'a' && lexer.Lookahead() <= 'z' {
			word, length, _ := scaReadWord(lexer, 10) // sizeof "protected"
			if length > 0 && scaWordIn(word, scaDefinitionWords) {
				lexer.SetResultSymbol(symbols[postfixSym])
				return true, postfixSym
			}
		}
		// Any other operand continues the expression.
		return false, -1
	}

	// Mid-line block comments with no layout decision pending. `/*` is
	// plain text where SUPPRESS_BLOCK_COMMENT or a string-content state is
	// valid. In error recovery all symbols look valid and lexing the
	// comment is safe.
	if isValid(scaTokBlockComment) && lexer.Lookahead() == '/' &&
		(isValid(scaTokErrorSentinel) ||
			!(isValid(scaTokSuppressBlockComment) ||
				isValid(scaTokSimpleStringMiddle) ||
				isValid(scaTokInterpStringMiddle) ||
				isValid(scaTokRawStringMiddle) ||
				isValid(scaTokRawMultiStringMiddle) ||
				isValid(scaTokInterpMultiStringMiddle) ||
				isValid(scaTokMultilineStringEnd))) {
		lexer.Advance(false)
		if lexer.Lookahead() == '*' {
			lexer.Advance(false)
			return scaLexBlockComment(lexer, symbols), scaTokBlockComment
		}
		// A lone '/' or a line comment. Nothing else external can match
		// here.
		return false, -1
	}

	if isValid(scaTokSimpleStringStart) && lexer.Lookahead() == '"' {
		lexer.Advance(false)
		lexer.MarkEnd()

		if lexer.Lookahead() == '"' {
			lexer.Advance(false)
			if lexer.Lookahead() == '"' {
				lexer.Advance(false)
				lexer.SetResultSymbol(symbols[scaTokSimpleMultiStringStart])
				lexer.MarkEnd()
				return true, scaTokSimpleMultiStringStart
			}
		}

		lexer.SetResultSymbol(symbols[scaTokSimpleStringStart])
		return true, scaTokSimpleStringStart
	}

	// Two tokens of lookahead determine a raw string: the `raw` and the
	// `"`, which is why this needs the external scanner.
	if isValid(scaTokRawStringStart) && lexer.Lookahead() == 'r' {
		lexer.Advance(false)
		if lexer.Lookahead() == 'a' {
			lexer.Advance(false)
			if lexer.Lookahead() == 'w' {
				lexer.Advance(false)
				if lexer.Lookahead() == '"' {
					lexer.MarkEnd()
					lexer.SetResultSymbol(symbols[scaTokRawStringStart])
					return true, scaTokRawStringStart
				}
			}
		}
	}

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

	// The simple multiline string case has no MULTILINE_STRING_MIDDLE
	// token, and MULTILINE_STRING_END is shared by simple, raw, and
	// interpolated multiline strings, so this check comes after those.
	if isValid(scaTokMultilineStringEnd) {
		return scaScanStringContent(lexer, true, scaStringSimple, symbols)
	}

	// Scala 3 lets a `match`/`catch`'s `case` clauses align with the
	// enclosing region instead of indenting deeper. Open a flagged region
	// so the OUTDENT logic above closes it on a same-width non-case line.
	if isValid(scaTokIndent) && !isValid(scaTokErrorSentinel) &&
		newlineCount > 0 && lexer.Lookahead() == 'c' &&
		prev != -1 && indentationSize == prevWidth {
		lexer.MarkEnd()
		if scaIsCaseClauseIntro(lexer) {
			scanner.indents = append(scanner.indents, indentationSize|scaCaseIndentFlag)
			lexer.SetResultSymbol(symbols[scaTokIndent])
			return true, scaTokIndent
		}
		// The word was not `case`. A `catch` at this width belongs to an
		// enclosing try. The `case` probe above stopped at its `t`, so the
		// remainder is `tch`.
		if isValid(scaTokControlTailGate) && scaScanWord(lexer, "tch") {
			lexer.SetResultSymbol(symbols[scaTokControlTailGate])
			return true, scaTokControlTailGate
		}
		return false, -1
	}

	// Zero-width gate before a control-tail keyword. Tails on the same
	// line, and tails the semicolon machinery never sees, arrive here.
	if isValid(scaTokControlTailGate) && !isValid(scaTokErrorSentinel) {
		switch lexer.Lookahead() {
		case 'c', 'e', 'f', 't', 'y':
			lexer.MarkEnd()
			word, length, _ := scaReadWord(lexer, 8) // sizeof "finally"
			if length > 0 && scaWordIn(word, scaTailWords) {
				lexer.SetResultSymbol(symbols[scaTokControlTailGate])
				return true, scaTokControlTailGate
			}
			return false, -1
		}
	}

	return false, -1
}

// SCALA_EXTERNAL_SCANNER_LOCAL_PORT_END

var scalaExternalScannerIdentity = sha256.Sum256([]byte(
	scalaExternalScannerABIVersion + "\x00" +
		"local-port=" + scalaExternalScannerLocalPortSHA256 + "\x00" +
		scalaExternalScannerSpec.UpstreamRepo + "\x00" +
		scalaExternalScannerSpec.UpstreamCommit + "\x00" +
		scalaExternalScannerSemantics,
))
