//go:build !grammar_subset || grammar_subset_xml

package grammarruntime

import (
	"encoding/binary"
	"unicode"

	gotreesitter "github.com/odvcencio/gotreesitter"
)

// External token indexes for the XML grammar. This is the external index
// (the position of the token in the grammar's `externals: [...]` list),
// which is exactly what tree-sitter's `valid_symbols` array and C's
// result_symbol enum are indexed by. The external index is stable across a
// blob regen as long as the externals list itself does not reorder;
// concrete numeric gotreesitter.Symbol IDs are NOT stable (they shift
// whenever the grammar's total symbol count changes), so this scanner never
// hardcodes them -- see xmlDefaultSymTable below.
const (
	xmlTokPITarget       = iota // [0] PITarget
	xmlTokPIContent             // [1] _pi_content
	xmlTokComment               // [2] Comment
	xmlTokCharData              // [3] CharData
	xmlTokCData                 // [4] CData
	xmlTokXMLModel              // [5] xml-model
	xmlTokXMLStylesheet         // [6] xml-stylesheet
	xmlTokStartTagName          // [7] Name (start tag)
	xmlTokEndTagName            // [8] Name (end tag)
	xmlTokErrEndName            // [9] _erroneous_end_name
	xmlTokSelfClosingTag        // [10] />
	xmlTokenCount
)

// xmlDefaultSymTable records the concrete gotreesitter.Symbol IDs the
// currently shipped xml.bin assigns to each external, in xmlTok* order. It
// exists only as a pre-bind fallback (and as an independent value to
// compare a real bind against in tests); ExternalScannerForLanguage below
// overwrites it with values read from the actual loaded Language at bind
// time, which is what the scanner must do to survive a future blob regen
// that renumbers absolute symbol IDs without touching the externals list
// order. The two start/end tag name externals alias to a shared "Name"
// display node, and xml-model/xml-stylesheet/self-closing-tag each share a
// Symbol ID with every other occurrence of that literal elsewhere in the
// grammar.
var xmlDefaultSymTable = [xmlTokenCount]gotreesitter.Symbol{
	66, // PITarget
	67, // _pi_content
	68, // Comment
	69, // CharData
	70, // CData
	22, // "xml-model"
	21, // "xml-stylesheet"
	71, // _start_tag_name (display: Name)
	72, // _end_tag_name (display: Name)
	73, // _erroneous_end_name
	16, // "/>"
}

// xmlExternalScannerSpec records the source contract for this hand-written
// port, so updater tooling can tell a grammar-only upstream change apart
// from one that also touches the external scanner or its token list. Its
// Externals list is also the binding source for ExternalScannerForLanguage:
// index i here is scanner token index i (xmlTok* order).
var xmlExternalScannerSpec = ExternalScannerSpec{
	Language:       "xml",
	UpstreamRepo:   "https://github.com/tree-sitter-grammars/tree-sitter-xml",
	UpstreamCommit: "5000ae8f22d11fbe93939b05c1e37cf21117162d",
	SourceFiles: []ExternalScannerSourceFile{
		{Path: "xml/src/grammar.json", SHA256: "8154b75312606f2207e5a22eaefb69811836b22b53a9c4c96b5e417c409ec820"},
		{Path: "xml/src/scanner.c", SHA256: "c49be7a88f5bc0feee512ce704dc05b1fd53c20f2110a46fb987bd93509ef806"},
	},
	Externals: []string{
		"PITarget",
		"_pi_content",
		"Comment",
		"CharData",
		"CData",
		"xml-model",
		"xml-stylesheet",
		"_start_tag_name",
		"_end_tag_name",
		"_erroneous_end_name",
		"/>",
	},
}

func init() {
	RegisterExternalScannerSpec(xmlExternalScannerSpec)
}

// xmlScannerState holds a stack of tag name strings, mirroring the C
// scanner's Vector(String) structure.
type xmlScannerState struct {
	tags []string
}

// XMLExternalScanner implements gotreesitter.ExternalScanner for tree-sitter-xml.
//
// This is a Go port of the C external scanner from tree-sitter-xml
// (https://github.com/tree-sitter-grammars/tree-sitter-xml). The scanner manages
// a tag name stack and handles 11 external tokens: PITarget, PIContent, Comment,
// CharData, CData, xml-model, xml-stylesheet, StartTagName, EndTagName,
// ErroneousEndName, and SelfClosingTagDelimiter.
//
// symbols holds the concrete gotreesitter.Symbol each external index maps to
// in the Language this instance was bound to (see ExternalScannerForLanguage).
// The scanner never hardcodes an absolute Symbol value: a blob regen can
// renumber the grammar's absolute symbol IDs without touching the externals
// list order, and a scanner that still called SetResultSymbol with a stale
// hardcoded ID would silently emit the wrong (but still structurally valid)
// node type instead of failing loudly.
type XMLExternalScanner struct {
	symbols         [xmlTokenCount]gotreesitter.Symbol
	externalToToken []int
}

// ExternalScannerForLanguage binds the scanner's token slots to the loaded
// Language's ExternalSymbols positionally. A hardcoded absolute
// gotreesitter.Symbol constant here would emit the wrong token whenever a
// grammar bump renumbers xml's external symbols.
func (XMLExternalScanner) ExternalScannerForLanguage(lang *gotreesitter.Language) gotreesitter.ExternalScanner {
	s := XMLExternalScanner{symbols: xmlDefaultSymTable}
	s.externalToToken = bindExternalScannerSpec(lang, xmlExternalScannerSpec, func(tokenIdx int, sym gotreesitter.Symbol) {
		s.symbols[tokenIdx] = sym
	})
	return s
}

func (s XMLExternalScanner) symbolTable() *[xmlTokenCount]gotreesitter.Symbol {
	if s.symbols == ([xmlTokenCount]gotreesitter.Symbol{}) {
		return &xmlDefaultSymTable
	}
	return &s.symbols
}

func (XMLExternalScanner) Create() any {
	return &xmlScannerState{}
}

func (XMLExternalScanner) Destroy(payload any) {}

func (XMLExternalScanner) Serialize(payload any, buf []byte) int {
	s := payload.(*xmlScannerState)
	tagCount := len(s.tags)
	if tagCount > 0xFFFF {
		tagCount = 0xFFFF
	}

	// Format: 4 bytes serialized_tag_count + 4 bytes tag_count + per-tag data
	// We write tag_count first at offset 4, then fill in serialized_tag_count
	// at offset 0 after we know how many actually fit.
	if len(buf) < 8 {
		return 0
	}

	size := 4 // reserve space for serialized_tag_count
	binary.LittleEndian.PutUint32(buf[size:], uint32(tagCount))
	size += 4

	serializedTagCount := 0
	for i := 0; i < tagCount; i++ {
		nameLen := len(s.tags[i])
		if nameLen > 255 {
			nameLen = 255
		}
		// Need 1 byte for length + nameLen bytes for the name
		if size+1+nameLen > len(buf) {
			break
		}
		buf[size] = byte(nameLen)
		size++
		if nameLen > 0 {
			copy(buf[size:], s.tags[i][:nameLen])
			size += nameLen
		}
		serializedTagCount++
	}

	binary.LittleEndian.PutUint32(buf[0:], uint32(serializedTagCount))
	return size
}

func (XMLExternalScanner) Deserialize(payload any, buf []byte) {
	s := payload.(*xmlScannerState)
	s.tags = s.tags[:0]

	if len(buf) == 0 {
		return
	}
	if len(buf) < 8 {
		return
	}

	serializedTagCount := binary.LittleEndian.Uint32(buf[0:4])
	tagCount := binary.LittleEndian.Uint32(buf[4:8])
	pos := 8

	if tagCount == 0 {
		return
	}

	// Pre-allocate
	if cap(s.tags) < int(tagCount) {
		s.tags = make([]string, 0, tagCount)
	}

	var i uint32
	for i = 0; i < serializedTagCount; i++ {
		if pos >= len(buf) {
			break
		}
		nameLen := int(buf[pos])
		pos++
		name := ""
		if nameLen > 0 && pos+nameLen <= len(buf) {
			name = string(buf[pos : pos+nameLen])
			pos += nameLen
		}
		s.tags = append(s.tags, name)
	}

	// Pad with empty tags if the buffer ran out of room during serialization
	for ; i < tagCount; i++ {
		s.tags = append(s.tags, "")
	}
}

func (s XMLExternalScanner) Scan(payload any, lexer *gotreesitter.ExternalLexer, validSymbols []bool) bool {
	state := payload.(*xmlScannerState)

	if len(s.externalToToken) > 0 {
		var semanticValid [xmlTokenCount]bool
		for externalIdx, valid := range validSymbols {
			if !valid || externalIdx >= len(s.externalToToken) {
				continue
			}
			tokenIdx := s.externalToToken[externalIdx]
			if tokenIdx >= 0 && tokenIdx < xmlTokenCount {
				semanticValid[tokenIdx] = true
			}
		}
		validSymbols = semanticValid[:]
	}
	syms := s.symbolTable()

	// When all of these tokens are valid, we are in error recovery -- bail out.
	if xmlInErrorRecovery(validSymbols) {
		return false
	}

	if xmlValid(validSymbols, xmlTokPITarget) {
		return xmlScanPITarget(lexer, validSymbols, syms[xmlTokPITarget])
	}

	if xmlValid(validSymbols, xmlTokPIContent) {
		return xmlScanPIContent(lexer, syms[xmlTokPIContent])
	}

	if xmlValid(validSymbols, xmlTokCharData) && xmlScanCharData(lexer, syms[xmlTokCharData]) {
		return true
	}

	if xmlValid(validSymbols, xmlTokCData) && xmlScanCData(lexer, syms[xmlTokCData]) {
		return true
	}

	ch := lexer.Lookahead()
	switch ch {
	case '<':
		lexer.MarkEnd()
		lexer.Advance(false)
		if lexer.Lookahead() == '!' {
			lexer.Advance(false)
			return xmlScanComment(lexer, syms[xmlTokComment])
		}
	case '/':
		if xmlValid(validSymbols, xmlTokSelfClosingTag) {
			return xmlScanSelfClosingTagDelimiter(state, lexer, syms[xmlTokSelfClosingTag])
		}
	case 0:
		// EOF -- do nothing
	default:
		if xmlValid(validSymbols, xmlTokStartTagName) {
			return xmlScanStartTagName(state, lexer, syms[xmlTokStartTagName])
		}
		if xmlValid(validSymbols, xmlTokEndTagName) {
			return xmlScanEndTagName(state, lexer, syms[xmlTokEndTagName], syms[xmlTokErrEndName])
		}
	}

	return false
}

// ---------------------------------------------------------------------------
// Helper predicates
// ---------------------------------------------------------------------------

func xmlValid(validSymbols []bool, idx int) bool {
	return idx >= 0 && idx < len(validSymbols) && validSymbols[idx]
}

func xmlInErrorRecovery(validSymbols []bool) bool {
	return xmlValid(validSymbols, xmlTokPITarget) &&
		xmlValid(validSymbols, xmlTokPIContent) &&
		xmlValid(validSymbols, xmlTokComment) &&
		xmlValid(validSymbols, xmlTokCharData) &&
		xmlValid(validSymbols, xmlTokCData)
}

// isXMLNameStartChar matches iswalpha || '_' || ':'
func isXMLNameStartChar(ch rune) bool {
	return unicode.IsLetter(ch) || ch == '_' || ch == ':'
}

// isXMLNameChar matches iswalnum || '_' || ':' || '.' || '-' || 0xB7
func isXMLNameChar(ch rune) bool {
	return unicode.IsLetter(ch) || unicode.IsDigit(ch) ||
		ch == '_' || ch == ':' || ch == '.' || ch == '-' || ch == 0xB7
}

// ---------------------------------------------------------------------------
// Tag name scanning
// ---------------------------------------------------------------------------

func xmlScanTagName(lexer *gotreesitter.ExternalLexer) string {
	var name []byte
	ch := lexer.Lookahead()
	if isXMLNameStartChar(ch) {
		name = append(name, byte(ch))
		lexer.Advance(false)
	}
	for {
		ch = lexer.Lookahead()
		if !isXMLNameChar(ch) {
			break
		}
		name = append(name, byte(ch))
		lexer.Advance(false)
	}
	return string(name)
}

func xmlScanStartTagName(s *xmlScannerState, lexer *gotreesitter.ExternalLexer, startTagNameSym gotreesitter.Symbol) bool {
	name := xmlScanTagName(lexer)
	if len(name) == 0 {
		return false
	}
	lexer.MarkEnd()
	lexer.SetResultSymbol(startTagNameSym)
	s.tags = append(s.tags, name)
	return true
}

func xmlScanEndTagName(s *xmlScannerState, lexer *gotreesitter.ExternalLexer, endTagNameSym, errEndNameSym gotreesitter.Symbol) bool {
	name := xmlScanTagName(lexer)
	if len(name) == 0 {
		return false
	}
	lexer.MarkEnd()

	if len(s.tags) > 0 && s.tags[len(s.tags)-1] == name {
		s.tags = s.tags[:len(s.tags)-1]
		lexer.SetResultSymbol(endTagNameSym)
		return true
	}
	lexer.SetResultSymbol(errEndNameSym)
	return false
}

func xmlScanSelfClosingTagDelimiter(s *xmlScannerState, lexer *gotreesitter.ExternalLexer, selfClosingTagSym gotreesitter.Symbol) bool {
	// Consume '/'
	lexer.Advance(false)
	// Expect '>'
	if lexer.Lookahead() == 0 || lexer.Lookahead() != '>' {
		return false
	}
	lexer.Advance(false)
	lexer.MarkEnd()
	if len(s.tags) > 0 {
		s.tags = s.tags[:len(s.tags)-1]
		lexer.SetResultSymbol(selfClosingTagSym)
	}
	return true
}

// ---------------------------------------------------------------------------
// CharData, CData, Comment, PI scanning
// ---------------------------------------------------------------------------

func xmlScanCharData(lexer *gotreesitter.ExternalLexer, charDataSym gotreesitter.Symbol) bool {
	advancedOnce := false

	for lexer.Lookahead() != 0 && lexer.Lookahead() != '<' && lexer.Lookahead() != '&' {
		if lexer.Lookahead() == ']' {
			lexer.MarkEnd()
			lexer.Advance(false)
			if lexer.Lookahead() == ']' {
				lexer.Advance(false)
				if lexer.Lookahead() == '>' {
					lexer.Advance(false)
					if advancedOnce {
						lexer.SetResultSymbol(charDataSym)
						return false
					}
				}
			}
		}
		advancedOnce = true
		// Re-check in_char_data condition before advancing
		if lexer.Lookahead() != 0 && lexer.Lookahead() != '<' && lexer.Lookahead() != '&' {
			lexer.Advance(false)
		}
	}

	if advancedOnce {
		lexer.MarkEnd()
		lexer.SetResultSymbol(charDataSym)
		return true
	}
	return false
}

func xmlScanCData(lexer *gotreesitter.ExternalLexer, cDataSym gotreesitter.Symbol) bool {
	advancedOnce := false

	for lexer.Lookahead() != 0 {
		if lexer.Lookahead() == ']' {
			lexer.MarkEnd()
			lexer.Advance(false)
			if lexer.Lookahead() == ']' {
				lexer.Advance(false)
				if lexer.Lookahead() == '>' && advancedOnce {
					lexer.SetResultSymbol(cDataSym)
					return true
				}
			}
		}
		advancedOnce = true
		lexer.Advance(false)
	}

	return false
}

func xmlScanComment(lexer *gotreesitter.ExternalLexer, commentSym gotreesitter.Symbol) bool {
	// Expect '--' after '<!'
	if lexer.Lookahead() == 0 || lexer.Lookahead() != '-' {
		return false
	}
	lexer.Advance(false)
	if lexer.Lookahead() == 0 || lexer.Lookahead() != '-' {
		return false
	}
	lexer.Advance(false)

	for lexer.Lookahead() != 0 {
		if lexer.Lookahead() == '-' {
			lexer.Advance(false)
			if lexer.Lookahead() == '-' {
				lexer.Advance(false)
				break
			}
		} else {
			lexer.Advance(false)
		}
	}

	if lexer.Lookahead() == '>' {
		lexer.Advance(false)
		lexer.MarkEnd()
		lexer.SetResultSymbol(commentSym)
		return true
	}

	return false
}

func xmlScanPITarget(lexer *gotreesitter.ExternalLexer, validSymbols []bool, piTargetSym gotreesitter.Symbol) bool {
	advancedOnce := false
	foundXFirst := false

	ch := lexer.Lookahead()
	if isXMLNameStartChar(ch) {
		if ch == 'x' || ch == 'X' {
			foundXFirst = true
			lexer.MarkEnd()
		}
		advancedOnce = true
		lexer.Advance(false)
	}

	if !advancedOnce {
		return false
	}

	for isXMLNameChar(lexer.Lookahead()) {
		if foundXFirst && (lexer.Lookahead() == 'm' || lexer.Lookahead() == 'M') {
			lexer.Advance(false)
			if lexer.Lookahead() == 'l' || lexer.Lookahead() == 'L' {
				lexer.Advance(false)
				if isXMLNameChar(lexer.Lookahead()) {
					foundXFirst = false
					lastCharHyphen := lexer.Lookahead() == '-'
					lexer.Advance(false)
					if lastCharHyphen {
						if xmlValid(validSymbols, xmlTokXMLModel) && xmlCheckWord(lexer, "model") {
							return false
						}
						if xmlValid(validSymbols, xmlTokXMLStylesheet) && xmlCheckWord(lexer, "stylesheet") {
							return false
						}
					}
				} else {
					return false
				}
			}
		}

		foundXFirst = false
		lexer.Advance(false)
	}

	lexer.MarkEnd()
	lexer.SetResultSymbol(piTargetSym)
	return true
}

func xmlCheckWord(lexer *gotreesitter.ExternalLexer, word string) bool {
	for i := 0; i < len(word); i++ {
		if lexer.Lookahead() == 0 || lexer.Lookahead() != rune(word[i]) {
			return false
		}
		lexer.Advance(false)
	}
	return true
}

func xmlScanPIContent(lexer *gotreesitter.ExternalLexer, piContentSym gotreesitter.Symbol) bool {
	for lexer.Lookahead() != 0 && lexer.Lookahead() != '\n' && lexer.Lookahead() != '?' {
		lexer.Advance(false)
	}

	if lexer.Lookahead() != '?' {
		return false
	}

	lexer.MarkEnd()
	lexer.Advance(false)

	if lexer.Lookahead() == '>' {
		lexer.Advance(false)
		for lexer.Lookahead() == ' ' {
			lexer.Advance(false)
		}
		// advance_if_eq(lexer, '\n')
		if lexer.Lookahead() == 0 || lexer.Lookahead() != '\n' {
			return false
		}
		lexer.Advance(false)
		lexer.SetResultSymbol(piContentSym)
		return true
	}

	return false
}
