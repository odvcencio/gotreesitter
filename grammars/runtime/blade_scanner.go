//go:build !grammar_subset || grammar_subset_angular || grammar_subset_astro || grammar_subset_blade || grammar_subset_html || grammar_subset_svelte || grammar_subset_vue

package grammarruntime

import gotreesitter "github.com/odvcencio/gotreesitter"

// --- Shared HTML scanning helpers ---

type goLexerAdapter struct {
	l *gotreesitter.ExternalLexer
}

func (a *goLexerAdapter) lookahead() rune   { return a.l.Lookahead() }
func (a *goLexerAdapter) advance(skip bool) { a.l.Advance(skip) }
func (a *goLexerAdapter) markEnd()          { a.l.MarkEnd() }
func (a *goLexerAdapter) eof() bool         { return a.l.Lookahead() == 0 }

func htmlSerializeTags(tags []htmlTag, buf []byte) int {
	if len(buf) < 4 || len(tags) > 0xFFFF {
		return 0
	}
	size := 4
	for i := range tags {
		tag := &tags[i]
		if tag.tagType > htmlTagInterpolation {
			return 0
		}
		size++
		if tag.tagType == htmlTagCustom {
			if len(tag.customName) > 255 {
				return 0
			}
			size += 1 + len(tag.customName)
		}
		if size > len(buf) {
			return 0
		}
	}

	tagCount := len(tags)
	// A non-empty result is always the complete state. Keeping both counts in
	// the inherited wire format lets the decoder reject old partial snapshots.
	buf[0] = byte(tagCount)
	buf[1] = byte(tagCount >> 8)
	buf[2] = byte(tagCount)
	buf[3] = byte(tagCount >> 8)
	offset := 4
	for i := 0; i < tagCount; i++ {
		tag := tags[i]
		buf[offset] = byte(tag.tagType)
		offset++
		if tag.tagType == htmlTagCustom {
			nameLen := len(tag.customName)
			buf[offset] = byte(nameLen)
			offset++
			copy(buf[offset:], tag.customName)
			offset += nameLen
		}
	}
	return size
}

func htmlDeserializeTagsInto(dst []htmlTag, buf []byte) []htmlTag {
	if len(buf) < 4 {
		return nil
	}
	serializedCount := int(buf[0]) | int(buf[1])<<8
	totalCount := int(buf[2]) | int(buf[3])<<8
	if serializedCount != totalCount {
		return nil
	}
	offset := 4
	for i := 0; i < serializedCount; i++ {
		if offset >= len(buf) {
			return nil
		}
		tagType := htmlTagType(buf[offset])
		offset++
		if tagType > htmlTagInterpolation {
			return nil
		}
		if tagType == htmlTagCustom {
			if offset >= len(buf) {
				return nil
			}
			nameLen := int(buf[offset])
			offset++
			if offset+nameLen > len(buf) {
				return nil
			}
			offset += nameLen
		}
	}
	if offset != len(buf) {
		return nil
	}

	clear(dst)
	tags := dst[:0]
	offset = 4
	for i := 0; i < serializedCount; i++ {
		tagType := htmlTagType(buf[offset])
		offset++
		if tagType == htmlTagCustom {
			nameLen := int(buf[offset])
			offset++
			name := string(buf[offset : offset+nameLen])
			offset += nameLen
			tags = append(tags, htmlTag{tagType: htmlTagCustom, customName: name})
		} else {
			tags = append(tags, htmlTag{tagType: tagType})
		}
	}
	return tags
}

func htmlScanComment(lx htmlLexer, commentSym gotreesitter.Symbol, lexer *gotreesitter.ExternalLexer) bool {
	if lx.lookahead() != '-' {
		return false
	}
	lx.advance(false)
	if lx.lookahead() != '-' {
		return false
	}
	lx.advance(false)

	dashes := uint32(0)
	for lx.lookahead() != 0 {
		switch lx.lookahead() {
		case '-':
			dashes++
		case '>':
			if dashes >= 2 {
				lexer.SetResultSymbol(commentSym)
				lx.advance(false)
				lx.markEnd()
				return true
			}
			dashes = 0
		default:
			dashes = 0
		}
		lx.advance(false)
	}
	return false
}

func htmlScanRawText(lx htmlLexer, tags []htmlTag, rawTextSym gotreesitter.Symbol, lexer *gotreesitter.ExternalLexer) bool {
	if len(tags) == 0 {
		return false
	}
	lx.markEnd()

	lastTag := tags[len(tags)-1]
	var endDelimiter string
	if lastTag.tagType == htmlTagScript {
		endDelimiter = "</SCRIPT"
	} else {
		endDelimiter = "</STYLE"
	}

	delimIdx := 0
	for lx.lookahead() != 0 {
		ch := lx.lookahead()
		if ch >= 'a' && ch <= 'z' {
			ch = ch - 'a' + 'A'
		}
		if byte(ch) == endDelimiter[delimIdx] {
			delimIdx++
			if delimIdx == len(endDelimiter) {
				break
			}
			lx.advance(false)
		} else {
			delimIdx = 0
			lx.advance(false)
			lx.markEnd()
		}
	}

	lexer.SetResultSymbol(rawTextSym)
	return true
}

func htmlScanImplicitEndTag(lx htmlLexer, tags *[]htmlTag, implicitEndTagSym gotreesitter.Symbol, lexer *gotreesitter.ExternalLexer) bool {
	var parent *htmlTag
	if len(*tags) > 0 {
		parent = &(*tags)[len(*tags)-1]
	}

	isClosingTag := false
	if lx.lookahead() == '/' {
		isClosingTag = true
		lx.advance(false)
	} else {
		if parent != nil && htmlTagIsVoid(parent) {
			*tags = (*tags)[:len(*tags)-1]
			lexer.SetResultSymbol(implicitEndTagSym)
			return true
		}
	}

	tagName := htmlScanTagName(lx)
	if len(tagName) == 0 && !lx.eof() {
		return false
	}

	nextTag := htmlTagForName(tagName)

	if isClosingTag {
		if len(*tags) > 0 && htmlTagEq(&(*tags)[len(*tags)-1], &nextTag) {
			return false
		}
		for i := len(*tags); i > 0; i-- {
			if (*tags)[i-1].tagType == nextTag.tagType {
				*tags = (*tags)[:len(*tags)-1]
				lexer.SetResultSymbol(implicitEndTagSym)
				return true
			}
		}
	} else if parent != nil &&
		(!htmlTagCanContain(parent, &nextTag) ||
			((parent.tagType == htmlTagHtml || parent.tagType == htmlTagHead || parent.tagType == htmlTagBody) && lx.eof())) {
		*tags = (*tags)[:len(*tags)-1]
		lexer.SetResultSymbol(implicitEndTagSym)
		return true
	}

	return false
}

func htmlScanSelfClosingDelim(lx htmlLexer, tags *[]htmlTag, selfClosingSym gotreesitter.Symbol, lexer *gotreesitter.ExternalLexer) bool {
	lx.advance(false)
	if lx.lookahead() == '>' {
		lx.advance(false)
		if len(*tags) > 0 {
			*tags = (*tags)[:len(*tags)-1]
			lexer.SetResultSymbol(selfClosingSym)
		}
		return true
	}
	return false
}

func htmlScanStartTagName(lx htmlLexer, tags *[]htmlTag, startSym, scriptSym, styleSym, templateSym gotreesitter.Symbol, lexer *gotreesitter.ExternalLexer) bool {
	tagName := htmlScanTagName(lx)
	if len(tagName) == 0 {
		return false
	}

	tag := htmlTagForName(tagName)
	*tags = append(*tags, tag)

	lx.markEnd()
	switch tag.tagType {
	case htmlTagScript:
		lexer.SetResultSymbol(scriptSym)
	case htmlTagStyle:
		lexer.SetResultSymbol(styleSym)
	case htmlTagTemplate:
		if templateSym != 0 {
			lexer.SetResultSymbol(templateSym)
		} else {
			lexer.SetResultSymbol(startSym)
		}
	default:
		lexer.SetResultSymbol(startSym)
	}
	return true
}

func htmlScanEndTagName(lx htmlLexer, tags *[]htmlTag, endSym, errEndSym gotreesitter.Symbol, lexer *gotreesitter.ExternalLexer) bool {
	tagName := htmlScanTagName(lx)
	if len(tagName) == 0 {
		return false
	}

	tag := htmlTagForName(tagName)
	lx.markEnd()
	if len(*tags) > 0 && htmlTagEq(&(*tags)[len(*tags)-1], &tag) {
		*tags = (*tags)[:len(*tags)-1]
		lexer.SetResultSymbol(endSym)
	} else {
		lexer.SetResultSymbol(errEndSym)
	}
	return true
}
