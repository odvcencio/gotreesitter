//go:build !grammar_subset || grammar_subset_xml

package grammarruntime

import (
	"encoding/binary"
	"unicode"

	gotreesitter "github.com/odvcencio/gotreesitter"
)

// External token indexes for the XML grammar.
// These must match the order in the grammar's externals array.
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
)

// Concrete symbol IDs from the generated XML grammar ExternalSymbols.
const (
	xmlSymPITarget       gotreesitter.Symbol = 66
	xmlSymPIContent      gotreesitter.Symbol = 67
	xmlSymComment        gotreesitter.Symbol = 68
	xmlSymCharData       gotreesitter.Symbol = 69
	xmlSymCData          gotreesitter.Symbol = 70
	xmlSymXMLModel       gotreesitter.Symbol = 22
	xmlSymXMLStylesheet  gotreesitter.Symbol = 21
	xmlSymStartTagName   gotreesitter.Symbol = 71
	xmlSymEndTagName     gotreesitter.Symbol = 72
	xmlSymErrEndName     gotreesitter.Symbol = 73
	xmlSymSelfClosingTag gotreesitter.Symbol = 16
)

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
type XMLExternalScanner struct{}

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

func (XMLExternalScanner) Scan(payload any, lexer *gotreesitter.ExternalLexer, validSymbols []bool) bool {
	s := payload.(*xmlScannerState)

	// When all of these tokens are valid, we are in error recovery -- bail out.
	if xmlInErrorRecovery(validSymbols) {
		return false
	}

	if xmlValid(validSymbols, xmlTokPITarget) {
		return xmlScanPITarget(lexer, validSymbols)
	}

	if xmlValid(validSymbols, xmlTokPIContent) {
		return xmlScanPIContent(lexer)
	}

	if xmlValid(validSymbols, xmlTokCharData) && xmlScanCharData(lexer) {
		return true
	}

	if xmlValid(validSymbols, xmlTokCData) && xmlScanCData(lexer) {
		return true
	}

	ch := lexer.Lookahead()
	switch ch {
	case '<':
		lexer.MarkEnd()
		lexer.Advance(false)
		if lexer.Lookahead() == '!' {
			lexer.Advance(false)
			return xmlScanComment(lexer)
		}
	case '/':
		if xmlValid(validSymbols, xmlTokSelfClosingTag) {
			return xmlScanSelfClosingTagDelimiter(s, lexer)
		}
	case 0:
		// EOF -- do nothing
	default:
		if xmlValid(validSymbols, xmlTokStartTagName) {
			return xmlScanStartTagName(s, lexer)
		}
		if xmlValid(validSymbols, xmlTokEndTagName) {
			return xmlScanEndTagName(s, lexer)
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

func xmlScanStartTagName(s *xmlScannerState, lexer *gotreesitter.ExternalLexer) bool {
	name := xmlScanTagName(lexer)
	if len(name) == 0 {
		return false
	}
	lexer.MarkEnd()
	lexer.SetResultSymbol(xmlSymStartTagName)
	s.tags = append(s.tags, name)
	return true
}

func xmlScanEndTagName(s *xmlScannerState, lexer *gotreesitter.ExternalLexer) bool {
	name := xmlScanTagName(lexer)
	if len(name) == 0 {
		return false
	}
	lexer.MarkEnd()

	if len(s.tags) > 0 && s.tags[len(s.tags)-1] == name {
		s.tags = s.tags[:len(s.tags)-1]
		lexer.SetResultSymbol(xmlSymEndTagName)
		return true
	}
	lexer.SetResultSymbol(xmlSymErrEndName)
	return false
}

func xmlScanSelfClosingTagDelimiter(s *xmlScannerState, lexer *gotreesitter.ExternalLexer) bool {
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
		lexer.SetResultSymbol(xmlSymSelfClosingTag)
	}
	return true
}

// ---------------------------------------------------------------------------
// CharData, CData, Comment, PI scanning
// ---------------------------------------------------------------------------

func xmlScanCharData(lexer *gotreesitter.ExternalLexer) bool {
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
						lexer.SetResultSymbol(xmlSymCharData)
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
		lexer.SetResultSymbol(xmlSymCharData)
		return true
	}
	return false
}

func xmlScanCData(lexer *gotreesitter.ExternalLexer) bool {
	advancedOnce := false

	for lexer.Lookahead() != 0 {
		if lexer.Lookahead() == ']' {
			lexer.MarkEnd()
			lexer.Advance(false)
			if lexer.Lookahead() == ']' {
				lexer.Advance(false)
				if lexer.Lookahead() == '>' && advancedOnce {
					lexer.SetResultSymbol(xmlSymCData)
					return true
				}
			}
		}
		advancedOnce = true
		lexer.Advance(false)
	}

	return false
}

func xmlScanComment(lexer *gotreesitter.ExternalLexer) bool {
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
		lexer.SetResultSymbol(xmlSymComment)
		return true
	}

	return false
}

func xmlScanPITarget(lexer *gotreesitter.ExternalLexer, validSymbols []bool) bool {
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
	lexer.SetResultSymbol(xmlSymPITarget)
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

func xmlScanPIContent(lexer *gotreesitter.ExternalLexer) bool {
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
		lexer.SetResultSymbol(xmlSymPIContent)
		return true
	}

	return false
}
