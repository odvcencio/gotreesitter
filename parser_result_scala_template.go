package gotreesitter

import "bytes"

func normalizeScalaObjectTemplateBodyFragments(root *Node, source []byte, parser *Parser, lang *Language) {
	if root == nil || lang == nil || lang.Name != "scala" || len(root.children) < 3 || len(source) == 0 {
		return
	}
	templateBodySym, ok := symbolByName(lang, "template_body")
	if !ok {
		return
	}
	templateBodyNamed := symbolIsNamed(lang, templateBodySym)
	arena := root.ownerArena
	changed := false
	for i := 0; i+2 < len(root.children); i++ {
		obj := root.children[i]
		openBrace := scalaErrorTokenNode(root.children[i+1], "{", lang)
		if !scalaObjectNeedsTemplateBody(obj, lang) || openBrace == nil {
			continue
		}
		closeIdx := scalaFindTemplateBodyClose(root.children, i+2, lang)
		var closeByte uint32
		synthClose := false
		if closeIdx >= 0 {
			if closeNode := scalaErrorTokenNode(root.children[closeIdx], "}", lang); closeNode != nil {
				closeByte = closeNode.endByte
			}
		} else {
			matching := findMatchingBraceByte(source, int(openBrace.startByte), len(source))
			if matching < 0 {
				continue
			}
			closeByte = uint32(matching + 1)
			closeIdx = scalaFindTemplateBodyCloseByByte(root.children, i+2, closeByte)
			if closeIdx < 0 {
				continue
			}
			synthClose = true
		}
		bodyChildren, ok := scalaTemplateBodyFragmentChildren(root.children[i+1:closeIdx+1], arena, lang, source, closeByte, synthClose)
		if !ok {
			continue
		}
		replacementChildren := make([]*Node, 0, len(obj.children)+1)
		replacementChildren = append(replacementChildren, obj.children...)
		replacementChildren = append(replacementChildren, newParentNodeInArena(arena, templateBodySym, templateBodyNamed, bodyChildren, nil, 0))
		replacement := newParentNodeInArena(arena, obj.symbol, obj.isNamed(), replacementChildren, obj.fieldIDs(), obj.productionID)
		replaceChildRangeWithSingleNode(root, i, closeIdx+1, replacement)
		changed = true
	}
	if changed {
		populateParentNode(root, root.children)
	}
}

func normalizeScalaTemplateBodyObjectFragments(root *Node, source []byte, parser *Parser, lang *Language) {
	if root == nil || lang == nil || lang.Name != "scala" || len(source) == 0 {
		return
	}
	walkResultTree(root, func(n *Node) {
		if n.Type(lang) == "template_body" && len(n.children) >= 4 {
			for i := 0; i+2 < len(n.children); i++ {
				objTok := n.children[i]
				ident := n.children[i+1]
				open := n.children[i+2]
				if objTok == nil || ident == nil || open == nil || objTok.Type(lang) != "object" || ident.Type(lang) != "identifier" || open.Type(lang) != "{" {
					continue
				}
				startIdx := i
				if i > 0 {
					prev := n.children[i-1]
					if prev != nil && prev.Type(lang) == "_automatic_semicolon" && prev.startByte == objTok.startByte && prev.endByte == objTok.startByte {
						startIdx = i - 1
					}
				}
				closePos := scalaFindMatchingBraceByteWithTrivia(source, int(open.startByte), n.endByte)
				if closePos < 0 {
					continue
				}
				objectEnd := uint32(closePos + 1)
				recovered, ok := scalaRecoverTopLevelObjectNodeFromRange(source, objTok.startByte, objectEnd, parser, lang, n.ownerArena)
				if !ok || recovered == nil {
					continue
				}
				endIdx := len(n.children)
				for j := startIdx; j < len(n.children); j++ {
					child := n.children[j]
					if child == nil {
						continue
					}
					if child.startByte >= objectEnd {
						endIdx = j
						break
					}
				}
				if endIdx <= startIdx {
					continue
				}
				replaceChildRangeWithSingleNode(n, startIdx, endIdx, recovered)
				scalaRecoverTemplateBodyTailMembers(n, recovered.endByte, source, parser, lang)
				populateParentNode(n, n.children)
				i = startIdx
			}
		}
	})
}

type scalaTemplateMemberKind uint8

const (
	scalaTemplateMemberUnknown scalaTemplateMemberKind = iota
	scalaTemplateMemberPackage
	scalaTemplateMemberClass
	scalaTemplateMemberObject
	scalaTemplateMemberTrait
	scalaTemplateMemberEnum
	scalaTemplateMemberFunction
	scalaTemplateMemberImport
	scalaTemplateMemberVal
	scalaTemplateMemberComment
	scalaTemplateMemberBlockComment
)

type scalaTemplateMemberSpan struct {
	start uint32
	end   uint32
	kind  scalaTemplateMemberKind
}

func normalizeScalaTemplateBodyRecoveredMembers(root *Node, source []byte, parser *Parser, lang *Language) {
	if root == nil || lang == nil || lang.Name != "scala" || len(source) == 0 {
		return
	}
	walkResultTree(root, func(n *Node) {
		if n.Type(lang) == "template_body" && n.HasError() {
			scalaRecoverTemplateBodyMembers(n, source, parser, lang)
		}
	})
}

func normalizeScalaRecoveredObjectTemplateBodies(root *Node, source []byte, parser *Parser, lang *Language) {
	if root == nil || lang == nil || lang.Name != "scala" || len(source) == 0 {
		return
	}
	walkResultTree(root, func(n *Node) {
		if scalaDefinitionTemplateBodyNeedsRecovery(n, lang) {
			for i, child := range n.children {
				if child == nil || child.Type(lang) != "template_body" {
					continue
				}
				rebuilt, ok := scalaRebuildTemplateBodyFromSource(child, source, parser, lang, n.ownerArena)
				if !ok || rebuilt == nil {
					break
				}
				n.children[i] = rebuilt
				rebuilt.parent = n
				rebuilt.childIndex = int32(i)
				for cur := n; cur != nil; cur = cur.parent {
					cur.setHasError(false)
					populateParentNode(cur, cur.children)
				}
				break
			}
		}
	})
}

func scalaDefinitionTemplateBodyNeedsRecovery(n *Node, lang *Language) bool {
	if n == nil || lang == nil {
		return false
	}
	switch n.Type(lang) {
	case "object_definition", "class_definition", "trait_definition":
	default:
		return false
	}
	var body *Node
	for _, child := range n.children {
		if child != nil && child.Type(lang) == "template_body" {
			body = child
			break
		}
	}
	if body == nil || len(body.children) < 3 {
		return false
	}
	sawRepeatComment := false
	sawOpenComment := false
	sawBlockComment := false
	for _, child := range body.children {
		if child == nil {
			continue
		}
		switch child.Type(lang) {
		case "{", "}":
			continue
		case "/*":
			sawOpenComment = true
			continue
		case "block_comment":
			sawBlockComment = true
			continue
		case "block_comment_repeat1":
			sawRepeatComment = true
			continue
		}
	}
	return sawRepeatComment && sawOpenComment && !sawBlockComment
}

func scalaRebuildTemplateBodyFromSource(body *Node, source []byte, parser *Parser, lang *Language, arena *nodeArena) (*Node, bool) {
	if body == nil || lang == nil || body.Type(lang) != "template_body" || len(body.children) < 2 {
		return nil, false
	}
	open := body.children[0]
	close := body.children[len(body.children)-1]
	if open == nil || close == nil || open.Type(lang) != "{" || close.Type(lang) != "}" {
		return nil, false
	}
	children := make([]*Node, 0, len(body.children))
	children = append(children, open)
	memberStart := open.endByte
	if comment, ok := scalaBuildTemplateBodyLeadingBlockComment(source, open.endByte, close.startByte, lang, arena); ok && comment != nil {
		children = append(children, comment)
		memberStart = comment.endByte
	}
	spans := scalaTemplateBodyMemberSpans(source, memberStart, close.startByte)
	for _, span := range spans {
		recovered, ok := scalaRecoverTemplateBodyMemberNode(source, span, parser, lang, arena)
		if !ok || recovered == nil {
			continue
		}
		children = append(children, recovered)
	}
	if len(children) < 2 {
		return nil, false
	}
	children = append(children, close)
	if arena != nil {
		buf := arena.allocNodeSlice(len(children))
		copy(buf, children)
		children = buf
	}
	return newParentNodeInArena(arena, body.symbol, body.isNamed(), children, nil, body.productionID), true
}

func scalaBuildTemplateBodyLeadingBlockComment(source []byte, start, limit uint32, lang *Language, arena *nodeArena) (*Node, bool) {
	if start >= limit || int(limit) > len(source) || lang == nil {
		return nil, false
	}
	pos := int(start)
	endLimit := int(limit)
	for pos < endLimit {
		switch source[pos] {
		case ' ', '\t', '\n', '\r':
			pos++
		default:
			goto triviaDone
		}
	}
triviaDone:
	if pos+1 >= endLimit || source[pos] != '/' || source[pos+1] != '*' {
		return nil, false
	}
	closeRel := bytes.Index(source[pos+2:endLimit], []byte("*/"))
	if closeRel < 0 {
		return nil, false
	}
	closeStart := pos + 2 + closeRel
	closeEnd := closeStart + 2
	closeLeafStart := closeStart
	for closeLeafStart > pos {
		switch source[closeLeafStart-1] {
		case ' ', '\t':
			closeLeafStart--
		default:
			goto closeLeafDone
		}
	}
closeLeafDone:
	commentSym, ok := symbolByName(lang, "block_comment")
	if !ok {
		return nil, false
	}
	openSym, ok := symbolByName(lang, "/*")
	if !ok {
		return nil, false
	}
	closeSym, ok := symbolByName(lang, "*/")
	if !ok {
		return nil, false
	}
	commentNamed := symbolIsNamed(lang, commentSym)
	openNamed := symbolIsNamed(lang, openSym)
	closeNamed := symbolIsNamed(lang, closeSym)
	openNode := newLeafNodeInArena(
		arena,
		openSym,
		openNamed,
		uint32(pos),
		uint32(pos+2),
		advancePointByBytes(Point{}, source[:pos]),
		advancePointByBytes(Point{}, source[:pos+2]),
	)
	closeNode := newLeafNodeInArena(
		arena,
		closeSym,
		closeNamed,
		uint32(closeLeafStart),
		uint32(closeEnd),
		advancePointByBytes(Point{}, source[:closeLeafStart]),
		advancePointByBytes(Point{}, source[:closeEnd]),
	)
	children := []*Node{openNode, closeNode}
	if arena != nil {
		buf := arena.allocNodeSlice(len(children))
		copy(buf, children)
		children = buf
	}
	comment := newParentNodeInArena(arena, commentSym, commentNamed, children, nil, 0)
	comment.setExtra(true)
	return comment, true
}

func normalizeScalaSplitFunctionDefinitions(root *Node, source []byte, parser *Parser, lang *Language) {
	if root == nil || lang == nil || lang.Name != "scala" || len(source) == 0 {
		return
	}
	walkResultTree(root, func(n *Node) {
		if n.Type(lang) == "template_body" && n.HasError() {
			scalaRecoverSplitFunctionDefinition(n, source, parser, lang)
		}
	})
}

func scalaRecoverSplitFunctionDefinition(body *Node, source []byte, parser *Parser, lang *Language) {
	if body == nil || lang == nil || body.Type(lang) != "template_body" || len(body.children) < 4 {
		return
	}
	for i := 0; i+2 < len(body.children); i++ {
		header := body.children[i]
		if header == nil {
			continue
		}
		switch header.Type(lang) {
		case "function_declaration", "_function_declaration":
		default:
			continue
		}
		eqIdx := i + 1
		openIdx := i + 2
		if eqIdx >= len(body.children) || openIdx >= len(body.children) {
			continue
		}
		open := body.children[openIdx]
		if open == nil || open.Type(lang) != "{" {
			continue
		}
		eqLeaf := body.children[eqIdx]
		if eqLeaf == nil {
			continue
		}
		eqToken := eqLeaf
		if eqLeaf.Type(lang) == "ERROR" {
			eqToken = scalaErrorTokenNode(eqLeaf, "=", lang)
		}
		if eqToken == nil || eqToken.Type(lang) != "=" {
			continue
		}
		closePos := scalaFindMatchingBraceByteWithTrivia(source, int(open.startByte), body.endByte)
		if closePos < 0 {
			continue
		}
		recovered, ok := scalaRecoverSplitFunctionDefinitionFromRange(source, header.startByte, uint32(closePos+1), parser, lang, body.ownerArena)
		if !ok || recovered == nil {
			continue
		}
		startIdx, endIdx, ok := scalaTemplateBodyChildRange(body.children, header.startByte, uint32(closePos+1))
		if !ok {
			continue
		}
		replaceChildRangeWithSingleNode(body, startIdx, endIdx, recovered)
		for n := body; n != nil; n = n.parent {
			n.setHasError(false)
			populateParentNode(n, n.children)
		}
		return
	}
}

func scalaRecoverTemplateBodyMembers(body *Node, source []byte, parser *Parser, lang *Language) {
	if body == nil || lang == nil || body.Type(lang) != "template_body" || len(body.children) < 3 {
		return
	}
	open := body.children[0]
	close := body.children[len(body.children)-1]
	if open == nil || close == nil || open.Type(lang) != "{" || close.Type(lang) != "}" {
		return
	}
	spans := scalaTemplateBodyMemberSpans(source, open.endByte, close.startByte)
	if len(spans) == 0 {
		return
	}
	changed := false
	for _, span := range spans {
		recovered, ok := scalaRecoverTemplateBodyMemberNode(source, span, parser, lang, body.ownerArena)
		if !ok || recovered == nil {
			continue
		}
		startIdx, endIdx, ok := scalaTemplateBodyChildRange(body.children, span.start, span.end)
		if !ok {
			continue
		}
		replaceChildRangeWithSingleNode(body, startIdx, endIdx, recovered)
		changed = true
	}
	if !changed {
		return
	}
	for n := body; n != nil; n = n.parent {
		n.setHasError(false)
		populateParentNode(n, n.children)
	}
}

func scalaTemplateBodyChildRange(children []*Node, start, end uint32) (int, int, bool) {
	startIdx := -1
	endIdx := -1
	for i, child := range children {
		if child == nil {
			continue
		}
		if startIdx < 0 && (child.startByte >= start || child.endByte > start) {
			startIdx = i
		}
		if startIdx >= 0 && child.startByte >= end {
			endIdx = i
			break
		}
	}
	if startIdx < 0 {
		return 0, 0, false
	}
	if endIdx < 0 {
		endIdx = len(children)
	}
	if endIdx <= startIdx {
		return 0, 0, false
	}
	return startIdx, endIdx, true
}

func scalaRecoverTemplateBodyMemberNode(source []byte, span scalaTemplateMemberSpan, parser *Parser, lang *Language, arena *nodeArena) (*Node, bool) {
	if span.end <= span.start || int(span.end) > len(source) {
		return nil, false
	}
	switch span.kind {
	case scalaTemplateMemberClass:
		return scalaRecoverTopLevelClassNodeFromRange(source, span.start, span.end, parser, lang, arena)
	case scalaTemplateMemberObject:
		return scalaRecoverTopLevelObjectNodeFromRange(source, span.start, span.end, parser, lang, arena)
	case scalaTemplateMemberFunction:
		return scalaRecoverTopLevelFunctionNodeFromRange(source, span.start, span.end, parser, lang, arena)
	case scalaTemplateMemberImport:
		return scalaRecoverTopLevelNamedNodeFromRange(source, span.start, span.end, parser, lang, arena, "import_declaration")
	case scalaTemplateMemberVal:
		return scalaRecoverTopLevelNamedNodeFromRange(source, span.start, span.end, parser, lang, arena, "val_definition")
	case scalaTemplateMemberComment:
		return scalaRecoverTopLevelNamedNodeFromRange(source, span.start, span.end, parser, lang, arena, "comment")
	case scalaTemplateMemberBlockComment:
		if comment, ok := scalaBuildTemplateBodyLeadingBlockComment(source, span.start, span.end, lang, arena); ok && comment != nil {
			return comment, true
		}
		return scalaRecoverTopLevelNamedNodeFromRange(source, span.start, span.end, parser, lang, arena, "block_comment")
	default:
		return nil, false
	}
}

func scalaRecoverTemplateBodyTailMembers(body *Node, start uint32, source []byte, parser *Parser, lang *Language) {
	if body == nil || lang == nil || body.Type(lang) != "template_body" || len(body.children) < 2 {
		return
	}
	closeIdx := len(body.children) - 1
	close := body.children[closeIdx]
	if close == nil || close.Type(lang) != "}" || start >= close.startByte {
		return
	}
	for i := 0; i < closeIdx; i++ {
		child := body.children[i]
		if child != nil && child.startByte >= start && !child.IsExtra() {
			return
		}
	}
	spans := scalaTemplateBodyMemberSpans(source, start, close.startByte)
	if len(spans) == 0 {
		return
	}
	recovered := make([]*Node, 0, len(spans))
	for _, span := range spans {
		node, ok := scalaRecoverTemplateBodyMemberNode(source, span, parser, lang, body.ownerArena)
		if !ok || node == nil {
			continue
		}
		recovered = append(recovered, node)
	}
	if len(recovered) == 0 {
		return
	}
	newChildren := make([]*Node, 0, len(body.children)+len(recovered))
	newChildren = append(newChildren, body.children[:closeIdx]...)
	newChildren = append(newChildren, recovered...)
	newChildren = append(newChildren, body.children[closeIdx:]...)
	body.children = newChildren
	bodyFieldIDs := body.fieldIDs()
	bodyFieldSources := body.fieldSources()
	metadataChanged := false
	if len(bodyFieldIDs) > 0 {
		fieldIDs := make([]FieldID, 0, len(body.children))
		fieldIDs = append(fieldIDs, bodyFieldIDs[:closeIdx]...)
		for range recovered {
			fieldIDs = append(fieldIDs, 0)
		}
		fieldIDs = append(fieldIDs, bodyFieldIDs[closeIdx:]...)
		bodyFieldIDs = fieldIDs
		metadataChanged = true
	}
	if len(bodyFieldSources) > 0 {
		fieldSources := make([]uint8, 0, len(body.children))
		fieldSources = append(fieldSources, bodyFieldSources[:closeIdx]...)
		for range recovered {
			fieldSources = append(fieldSources, fieldSourceNone)
		}
		fieldSources = append(fieldSources, bodyFieldSources[closeIdx:]...)
		bodyFieldSources = fieldSources
		metadataChanged = true
	}
	if metadataChanged {
		body.setFieldMetadata(bodyFieldIDs, bodyFieldSources)
	}
	for i, child := range body.children {
		if child == nil {
			continue
		}
		child.parent = body
		child.childIndex = int32(i)
	}
}

func scalaTemplateBodyMemberSpans(source []byte, bodyStart, bodyEnd uint32) []scalaTemplateMemberSpan {
	if bodyStart >= bodyEnd || int(bodyEnd) > len(source) {
		return nil
	}
	var spans []scalaTemplateMemberSpan
	pos := int(bodyStart)
	limit := int(bodyEnd)
	for pos < limit {
		start, kind, ok := scalaFindNextTemplateBodyMemberStart(source, pos, limit)
		if !ok {
			break
		}
		end := scalaFindTemplateBodyMemberEnd(source, start, limit)
		if end <= start {
			pos = start + 1
			continue
		}
		spans = append(spans, scalaTemplateMemberSpan{
			start: uint32(start),
			end:   uint32(end),
			kind:  kind,
		})
		pos = end
	}
	return spans
}

func scalaFindNextTemplateBodyMemberStart(source []byte, pos, limit int) (int, scalaTemplateMemberKind, bool) {
	braceDepth := 0
	parenDepth := 0
	bracketDepth := 0
	inLineComment := false
	inBlockComment := false
	var stringQuote byte
	tripleQuote := false
	lineStart := true
	for i := pos; i < limit; i++ {
		ch := source[i]
		next := byte(0)
		if i+1 < limit {
			next = source[i+1]
		}
		if inLineComment {
			if ch == '\n' {
				inLineComment = false
				lineStart = true
			}
			continue
		}
		if inBlockComment {
			if ch == '*' && next == '/' {
				inBlockComment = false
				i++
				continue
			}
			if ch == '\n' {
				lineStart = true
			}
			continue
		}
		if stringQuote != 0 {
			if tripleQuote {
				if i+2 < limit && source[i] == stringQuote && source[i+1] == stringQuote && source[i+2] == stringQuote {
					stringQuote = 0
					tripleQuote = false
					i += 2
				}
				continue
			}
			if ch == '\\' {
				i++
				continue
			}
			if ch == stringQuote {
				stringQuote = 0
			}
			continue
		}
		if braceDepth == 0 && parenDepth == 0 && bracketDepth == 0 && ch == '/' {
			switch next {
			case '/':
				return i, scalaTemplateMemberComment, true
			case '*':
				return i, scalaTemplateMemberBlockComment, true
			}
		}
		if lineStart {
			j := skipHorizontalTrivia(source, i, limit)
			if braceDepth == 0 && parenDepth == 0 && bracketDepth == 0 {
				if kind, ok := scalaTemplateMemberKindAt(source, j, limit); ok {
					return j, kind, true
				}
			}
			lineStart = false
		}
		switch {
		case ch == '/' && next == '/':
			inLineComment = true
			i++
			continue
		case ch == '/' && next == '*':
			inBlockComment = true
			i++
			continue
		case ch == '"' || ch == '\'':
			if i+2 < limit && source[i+1] == ch && source[i+2] == ch {
				stringQuote = ch
				tripleQuote = true
				i += 2
				continue
			}
			stringQuote = ch
			tripleQuote = false
			continue
		case ch == '{':
			braceDepth++
		case ch == '}':
			if braceDepth > 0 {
				braceDepth--
			}
		case ch == '(':
			parenDepth++
		case ch == ')':
			if parenDepth > 0 {
				parenDepth--
			}
		case ch == '[':
			bracketDepth++
		case ch == ']':
			if bracketDepth > 0 {
				bracketDepth--
			}
		}
		if ch == '\n' {
			lineStart = true
		}
	}
	return 0, scalaTemplateMemberUnknown, false
}

func scalaFindTemplateBodyMemberEnd(source []byte, start, limit int) int {
	if end, ok := scalaTemplateBodyLeadingCommentEnd(source, start, limit); ok {
		return end
	}
	var scan scalaTemplateMemberEndScan
	for i := start + 1; i < limit; i++ {
		ch := source[i]
		next := byte(0)
		if i+1 < limit {
			next = source[i+1]
		}
		if scan.consumeTriviaOrString(source, &i, limit, ch, next) {
			continue
		}
		if scan.atTopLevel() && scalaStartsLineOrBlockComment(source, i, limit) {
			return trimTrailingHorizontalAndVerticalTrivia(source, start, i)
		}
		if scan.lineStart {
			if end, ok := scalaTemplateBodyMemberEndAtLineStart(source, start, i, limit, scan); ok {
				return end
			}
			scan.lineStart = false
		}
		if scan.startTriviaOrString(source, &i, limit, ch, next) {
			continue
		}
		scan.updateDepth(ch)
		if ch == '\n' {
			scan.lineStart = true
		}
	}
	return trimTrailingHorizontalAndVerticalTrivia(source, start, limit)
}

type scalaTemplateMemberEndScan struct {
	braceDepth     int
	parenDepth     int
	bracketDepth   int
	inLineComment  bool
	inBlockComment bool
	stringQuote    byte
	tripleQuote    bool
	lineStart      bool
}

func (scan scalaTemplateMemberEndScan) atTopLevel() bool {
	return scan.braceDepth == 0 && scan.parenDepth == 0 && scan.bracketDepth == 0
}

func (scan *scalaTemplateMemberEndScan) consumeTriviaOrString(source []byte, i *int, limit int, ch, next byte) bool {
	if scan.inLineComment {
		if ch == '\n' {
			scan.inLineComment = false
			scan.lineStart = true
		}
		return true
	}
	if scan.inBlockComment {
		if ch == '*' && next == '/' {
			scan.inBlockComment = false
			*i = *i + 1
			return true
		}
		if ch == '\n' {
			scan.lineStart = true
		}
		return true
	}
	if scan.stringQuote == 0 {
		return false
	}
	if scan.tripleQuote {
		if *i+2 < limit && source[*i] == scan.stringQuote && source[*i+1] == scan.stringQuote && source[*i+2] == scan.stringQuote {
			scan.stringQuote = 0
			scan.tripleQuote = false
			*i += 2
		}
		return true
	}
	if ch == '\\' {
		*i = *i + 1
		return true
	}
	if ch == scan.stringQuote {
		scan.stringQuote = 0
	}
	return true
}

func (scan *scalaTemplateMemberEndScan) startTriviaOrString(source []byte, i *int, limit int, ch, next byte) bool {
	switch {
	case ch == '/' && next == '/':
		scan.inLineComment = true
		*i = *i + 1
		return true
	case ch == '/' && next == '*':
		scan.inBlockComment = true
		*i = *i + 1
		return true
	case ch == '"' || ch == '\'':
		scan.stringQuote = ch
		scan.tripleQuote = *i+2 < limit && source[*i+1] == ch && source[*i+2] == ch
		if scan.tripleQuote {
			*i += 2
		}
		return true
	default:
		return false
	}
}

func (scan *scalaTemplateMemberEndScan) updateDepth(ch byte) {
	switch ch {
	case '{':
		scan.braceDepth++
	case '}':
		if scan.braceDepth > 0 {
			scan.braceDepth--
		}
	case '(':
		scan.parenDepth++
	case ')':
		if scan.parenDepth > 0 {
			scan.parenDepth--
		}
	case '[':
		scan.bracketDepth++
	case ']':
		if scan.bracketDepth > 0 {
			scan.bracketDepth--
		}
	}
}

func scalaTemplateBodyLeadingCommentEnd(source []byte, start, limit int) (int, bool) {
	if !scalaStartsLineOrBlockComment(source, start, limit) {
		return 0, false
	}
	if source[start+1] == '/' {
		end := start + 2
		for end < limit && source[end] != '\n' && source[end] != '\r' {
			end++
		}
		return trimTrailingHorizontalAndVerticalTrivia(source, start, end), true
	}
	end := start + 2
	for end+1 < limit {
		if source[end] == '*' && source[end+1] == '/' {
			end += 2
			return trimTrailingHorizontalAndVerticalTrivia(source, start, end), true
		}
		end++
	}
	return trimTrailingHorizontalAndVerticalTrivia(source, start, limit), true
}

func scalaStartsLineOrBlockComment(source []byte, pos, limit int) bool {
	return pos+1 < limit && source[pos] == '/' && (source[pos+1] == '/' || source[pos+1] == '*')
}

func scalaTemplateBodyMemberEndAtLineStart(source []byte, start, i, limit int, scan scalaTemplateMemberEndScan) (int, bool) {
	j := skipHorizontalTrivia(source, i, limit)
	if !scan.atTopLevel() {
		return 0, false
	}
	switch {
	case j < limit && source[j] == '}':
		return j, true
	case scalaStartsLineOrBlockComment(source, j, limit):
		return trimTrailingHorizontalAndVerticalTrivia(source, start, i), true
	default:
		if _, ok := scalaTemplateMemberKindAt(source, j, limit); ok {
			return trimTrailingHorizontalAndVerticalTrivia(source, start, i), true
		}
		return 0, false
	}
}

func scalaTemplateMemberKindAt(source []byte, pos, limit int) (scalaTemplateMemberKind, bool) {
	if pos >= limit {
		return scalaTemplateMemberUnknown, false
	}
	switch {
	case bytes.HasPrefix(source[pos:limit], []byte("private lazy val ")):
		return scalaTemplateMemberVal, true
	case bytes.HasPrefix(source[pos:limit], []byte("lazy val ")):
		return scalaTemplateMemberVal, true
	case bytes.HasPrefix(source[pos:limit], []byte("private val ")):
		return scalaTemplateMemberVal, true
	case bytes.HasPrefix(source[pos:limit], []byte("override val ")):
		return scalaTemplateMemberVal, true
	case bytes.HasPrefix(source[pos:limit], []byte("val ")):
		return scalaTemplateMemberVal, true
	case bytes.HasPrefix(source[pos:limit], []byte("implicit class ")):
		return scalaTemplateMemberClass, true
	case bytes.HasPrefix(source[pos:limit], []byte("final class ")):
		return scalaTemplateMemberClass, true
	case bytes.HasPrefix(source[pos:limit], []byte("class ")):
		return scalaTemplateMemberClass, true
	case bytes.HasPrefix(source[pos:limit], []byte("object ")):
		return scalaTemplateMemberObject, true
	case bytes.HasPrefix(source[pos:limit], []byte("import ")):
		return scalaTemplateMemberImport, true
	case pos < limit && source[pos] == '@':
		return scalaTemplateMemberFunction, true
	case bytes.HasPrefix(source[pos:limit], []byte("private def ")):
		return scalaTemplateMemberFunction, true
	case bytes.HasPrefix(source[pos:limit], []byte("override def ")):
		return scalaTemplateMemberFunction, true
	case bytes.HasPrefix(source[pos:limit], []byte("def ")):
		return scalaTemplateMemberFunction, true
	default:
		return scalaTemplateMemberUnknown, false
	}
}

func skipHorizontalTrivia(source []byte, pos, limit int) int {
	for pos < limit {
		switch source[pos] {
		case ' ', '\t':
			pos++
		default:
			return pos
		}
	}
	return pos
}

func trimTrailingHorizontalAndVerticalTrivia(source []byte, start, end int) int {
	if end > len(source) {
		end = len(source)
	}
	for end > start {
		switch source[end-1] {
		case ' ', '\t', '\n', '\r', '\f':
			end--
		default:
			return end
		}
	}
	return end
}

type scalaStatementSpan struct {
	start uint32
	end   uint32
}

func scalaRecoverSplitFunctionDefinitionFromRange(source []byte, fnStart, fnEnd uint32, parser *Parser, lang *Language, arena *nodeArena) (*Node, bool) {
	if lang == nil || int(fnStart) >= len(source) || fnEnd <= fnStart || int(fnEnd) > len(source) {
		return nil, false
	}
	offsetRoot, ok := parseSnippetRootWithOffset(lang, source, fnStart, fnEnd, parser)
	if !ok || offsetRoot.ChildCount() < 3 {
		return nil, false
	}
	header := offsetRoot.Child(0)
	eqLeaf := offsetRoot.Child(1)
	open := offsetRoot.Child(2)
	if header == nil || open == nil || open.Type(lang) != "{" {
		return nil, false
	}
	switch header.Type(lang) {
	case "function_declaration", "_function_declaration":
	default:
		return nil, false
	}
	if eqLeaf == nil || eqLeaf.Type(lang) == "ERROR" {
		if eqLeaf == nil {
			return nil, false
		}
		eqLeaf = scalaErrorTokenNode(eqLeaf, "=", lang)
	}
	if eqLeaf == nil || eqLeaf.Type(lang) != "=" {
		return nil, false
	}
	closePos := scalaFindMatchingBraceByteWithTrivia(source, int(open.startByte), fnEnd)
	if closePos < 0 {
		return nil, false
	}
	block, ok := scalaRecoverFunctionBlockFromRange(source, open.startByte, uint32(closePos+1), parser, lang, arena)
	if !ok || block == nil {
		return nil, false
	}
	functionSym, ok := symbolByName(lang, "function_definition")
	if !ok {
		return nil, false
	}
	functionNamed := symbolIsNamed(lang, functionSym)
	children := make([]*Node, 0, len(header.children)+2)
	for _, child := range header.children {
		if child == nil {
			continue
		}
		children = append(children, cloneTreeNodesIntoArena(child, arena))
	}
	children = append(children, cloneTreeNodesIntoArena(eqLeaf, arena))
	children = append(children, block)
	if arena != nil {
		buf := arena.allocNodeSlice(len(children))
		copy(buf, children)
		children = buf
	}
	return newParentNodeInArena(arena, functionSym, functionNamed, children, nil, 0), true
}

func scalaRecoverFunctionBlockFromRange(source []byte, blockStart, blockEnd uint32, parser *Parser, lang *Language, arena *nodeArena) (*Node, bool) {
	if lang == nil || blockEnd <= blockStart || int(blockEnd) > len(source) {
		return nil, false
	}
	blockSym, ok := symbolByName(lang, "block")
	if !ok {
		return nil, false
	}
	blockNamed := symbolIsNamed(lang, blockSym)
	openSym, ok := symbolByName(lang, "{")
	if !ok {
		return nil, false
	}
	openNamed := symbolIsNamed(lang, openSym)
	closeSym, ok := symbolByName(lang, "}")
	if !ok {
		return nil, false
	}
	closeNamed := symbolIsNamed(lang, closeSym)
	open := newLeafNodeInArena(arena, openSym, openNamed, blockStart, blockStart+1, advancePointByBytes(Point{}, source[:blockStart]), advancePointByBytes(Point{}, source[:blockStart+1]))
	close := newLeafNodeInArena(arena, closeSym, closeNamed, blockEnd-1, blockEnd, advancePointByBytes(Point{}, source[:blockEnd-1]), advancePointByBytes(Point{}, source[:blockEnd]))
	statementSpans := scalaBlockStatementSpans(source, blockStart+1, blockEnd-1)
	if len(statementSpans) == 0 {
		return nil, false
	}
	children := make([]*Node, 0, len(statementSpans)+2)
	children = append(children, open)
	for _, span := range statementSpans {
		stmt, ok := scalaRecoverBlockStatementNode(source, span.start, span.end, parser, lang, arena)
		if !ok || stmt == nil {
			return nil, false
		}
		children = append(children, stmt)
	}
	children = append(children, close)
	if arena != nil {
		buf := arena.allocNodeSlice(len(children))
		copy(buf, children)
		children = buf
	}
	return newParentNodeInArena(arena, blockSym, blockNamed, children, nil, 0), true
}

func scalaBlockStatementSpans(source []byte, blockStart, blockEnd uint32) []scalaStatementSpan {
	if blockStart >= blockEnd || int(blockEnd) > len(source) {
		return nil
	}
	var spans []scalaStatementSpan
	pos := int(blockStart)
	limit := int(blockEnd)
	for pos < limit {
		start, ok := scalaFindNextBlockStatementStart(source, pos, limit)
		if !ok {
			break
		}
		end := scalaFindNextBlockStatementBoundary(source, start, limit)
		if end <= start {
			pos = start + 1
			continue
		}
		spans = append(spans, scalaStatementSpan{start: uint32(start), end: uint32(end)})
		pos = end
	}
	return spans
}

func scalaFindNextBlockStatementStart(source []byte, pos, limit int) (int, bool) {
	lineStart := true
	for i := pos; i < limit; i++ {
		if lineStart {
			j := skipHorizontalTrivia(source, i, limit)
			if j < limit && source[j] != '\n' && source[j] != '\r' && source[j] != '}' {
				return j, true
			}
			lineStart = false
		}
		if source[i] == '\n' {
			lineStart = true
		}
	}
	return 0, false
}

func scalaFindNextBlockStatementBoundary(source []byte, start, limit int) int {
	braceDepth := 0
	parenDepth := 0
	bracketDepth := 0
	inLineComment := false
	inBlockComment := false
	var stringQuote byte
	tripleQuote := false
	lineStart := false
	for i := start + 1; i < limit; i++ {
		ch := source[i]
		next := byte(0)
		if i+1 < limit {
			next = source[i+1]
		}
		if inLineComment {
			if ch == '\n' {
				inLineComment = false
				lineStart = true
			}
			continue
		}
		if inBlockComment {
			if ch == '*' && next == '/' {
				inBlockComment = false
				i++
				continue
			}
			if ch == '\n' {
				lineStart = true
			}
			continue
		}
		if stringQuote != 0 {
			if tripleQuote {
				if i+2 < limit && source[i] == stringQuote && source[i+1] == stringQuote && source[i+2] == stringQuote {
					stringQuote = 0
					tripleQuote = false
					i += 2
				}
				continue
			}
			if ch == '\\' {
				i++
				continue
			}
			if ch == stringQuote {
				stringQuote = 0
			}
			continue
		}
		if lineStart {
			j := skipHorizontalTrivia(source, i, limit)
			if braceDepth == 0 && parenDepth == 0 && bracketDepth == 0 && j < limit {
				switch source[j] {
				case '}', '\n', '\r':
					return trimTrailingHorizontalAndVerticalTrivia(source, start, i)
				}
				return trimTrailingHorizontalAndVerticalTrivia(source, start, i)
			}
			lineStart = false
		}
		switch {
		case ch == '/' && next == '/':
			inLineComment = true
			i++
			continue
		case ch == '/' && next == '*':
			inBlockComment = true
			i++
			continue
		case ch == '"' || ch == '\'':
			if i+2 < limit && source[i+1] == ch && source[i+2] == ch {
				stringQuote = ch
				tripleQuote = true
				i += 2
				continue
			}
			stringQuote = ch
			tripleQuote = false
			continue
		case ch == '{':
			braceDepth++
		case ch == '}':
			if braceDepth > 0 {
				braceDepth--
			}
		case ch == '(':
			parenDepth++
		case ch == ')':
			if parenDepth > 0 {
				parenDepth--
			}
		case ch == '[':
			bracketDepth++
		case ch == ']':
			if bracketDepth > 0 {
				bracketDepth--
			}
		}
		if ch == '\n' {
			lineStart = true
		}
	}
	return trimTrailingHorizontalAndVerticalTrivia(source, start, limit)
}

func scalaRecoverBlockStatementNode(source []byte, start, end uint32, parser *Parser, lang *Language, arena *nodeArena) (*Node, bool) {
	if end <= start || int(end) > len(source) {
		return nil, false
	}
	if offsetRoot, ok := parseSnippetRootWithOffset(lang, source, start, end, parser); ok {
		for i := 0; i < offsetRoot.ChildCount(); i++ {
			child := offsetRoot.Child(i)
			if child == nil || child.HasError() {
				continue
			}
			switch child.Type(lang) {
			case "val_definition", "call_expression":
				return cloneTreeNodesIntoArena(child, arena), true
			}
		}
	}
	if bytes.HasPrefix(source[start:end], []byte("val ")) {
		return scalaRecoverValDefinitionIfExpressionFromRange(source, start, end, parser, lang, arena)
	}
	return nil, false
}

func scalaRecoverValDefinitionIfExpressionFromRange(source []byte, start, end uint32, parser *Parser, lang *Language, arena *nodeArena) (*Node, bool) {
	if end <= start || int(end) > len(source) || lang == nil {
		return nil, false
	}
	valSym, ok := symbolByName(lang, "val")
	if !ok {
		return nil, false
	}
	identifierSym, ok := symbolByName(lang, "identifier")
	if !ok {
		return nil, false
	}
	eqSym, ok := symbolByName(lang, "=")
	if !ok {
		return nil, false
	}
	valDefSym, ok := symbolByName(lang, "val_definition")
	if !ok {
		return nil, false
	}
	ifExprSym, ok := symbolByName(lang, "if_expression")
	if !ok {
		return nil, false
	}
	ifSym, ok := symbolByName(lang, "if")
	if !ok {
		return nil, false
	}
	elseSym, ok := symbolByName(lang, "else")
	if !ok {
		return nil, false
	}
	valNamed := symbolIsNamed(lang, valSym)
	identifierNamed := symbolIsNamed(lang, identifierSym)
	eqNamed := symbolIsNamed(lang, eqSym)
	valDefNamed := symbolIsNamed(lang, valDefSym)
	ifExprNamed := symbolIsNamed(lang, ifExprSym)
	ifNamed := symbolIsNamed(lang, ifSym)
	elseNamed := symbolIsNamed(lang, elseSym)

	ifPos := bytes.Index(source[start:end], []byte("if "))
	elsePos := bytes.Index(source[start:end], []byte(" else "))
	if ifPos < 0 || elsePos < 0 {
		return nil, false
	}
	ifPos += int(start)
	elsePos += int(start) + 1
	condStart := ifPos + len("if ")
	condEnd := scalaFindMatchingParenByteWithTrivia(source, condStart, int(end))
	if condEnd < condStart {
		return nil, false
	}
	consequenceStart := skipHorizontalTrivia(source, condEnd+1, int(end))
	if consequenceStart >= elsePos {
		return nil, false
	}
	alternativeStart := skipHorizontalTrivia(source, elsePos+len("else"), int(end))
	if alternativeStart >= int(end) {
		return nil, false
	}
	condition, ok := scalaRecoverSingleExpressionNode(source, uint32(condStart), uint32(condEnd+1), parser, lang, arena, "parenthesized_expression")
	if !ok {
		return nil, false
	}
	consequence, ok := scalaRecoverSingleExpressionNode(source, uint32(consequenceStart), uint32(elsePos), parser, lang, arena, "infix_expression")
	if !ok {
		return nil, false
	}
	alternative, ok := scalaRecoverSingleExpressionNode(source, uint32(alternativeStart), end, parser, lang, arena, "identifier")
	if !ok {
		return nil, false
	}
	valLeaf := newLeafNodeInArena(arena, valSym, valNamed, start, start+3, advancePointByBytes(Point{}, source[:start]), advancePointByBytes(Point{}, source[:start+3]))
	nameStart := start + 4
	nameEnd := nameStart + 3
	nameLeaf := newLeafNodeInArena(arena, identifierSym, identifierNamed, nameStart, nameEnd, advancePointByBytes(Point{}, source[:nameStart]), advancePointByBytes(Point{}, source[:nameEnd]))
	eqStart := start + 8
	eqLeaf := newLeafNodeInArena(arena, eqSym, eqNamed, eqStart, eqStart+1, advancePointByBytes(Point{}, source[:eqStart]), advancePointByBytes(Point{}, source[:eqStart+1]))
	ifLeaf := newLeafNodeInArena(arena, ifSym, ifNamed, uint32(ifPos), uint32(ifPos+2), advancePointByBytes(Point{}, source[:ifPos]), advancePointByBytes(Point{}, source[:ifPos+2]))
	elseLeaf := newLeafNodeInArena(arena, elseSym, elseNamed, uint32(elsePos), uint32(elsePos+4), advancePointByBytes(Point{}, source[:elsePos]), advancePointByBytes(Point{}, source[:elsePos+4]))
	ifChildren := []*Node{ifLeaf, condition, consequence, elseLeaf, alternative}
	if arena != nil {
		buf := arena.allocNodeSlice(len(ifChildren))
		copy(buf, ifChildren)
		ifChildren = buf
	}
	ifNode := newParentNodeInArena(arena, ifExprSym, ifExprNamed, ifChildren, nil, 0)
	valChildren := []*Node{valLeaf, nameLeaf, eqLeaf, ifNode}
	if arena != nil {
		buf := arena.allocNodeSlice(len(valChildren))
		copy(buf, valChildren)
		valChildren = buf
	}
	return newParentNodeInArena(arena, valDefSym, valDefNamed, valChildren, nil, 0), true
}

func scalaRecoverSingleExpressionNode(source []byte, start, end uint32, parser *Parser, lang *Language, arena *nodeArena, want string) (*Node, bool) {
	if end <= start || int(end) > len(source) {
		return nil, false
	}
	offsetRoot, ok := parseSnippetRootWithOffset(lang, source, start, end, parser)
	if !ok {
		return nil, false
	}
	for i := 0; i < offsetRoot.ChildCount(); i++ {
		child := offsetRoot.Child(i)
		if child == nil || child.HasError() {
			continue
		}
		if child.Type(lang) == want {
			return cloneTreeNodesIntoArena(child, arena), true
		}
	}
	if want == "identifier" {
		sym, ok := symbolByName(lang, "identifier")
		if !ok {
			return nil, false
		}
		named := symbolIsNamed(lang, sym)
		return newLeafNodeInArena(arena, sym, named, start, end, advancePointByBytes(Point{}, source[:start]), advancePointByBytes(Point{}, source[:end])), true
	}
	return nil, false
}

func scalaRecoverLeadingAnnotations(source []byte, start, fnStart, fnEnd uint32, parser *Parser, lang *Language, arena *nodeArena) []*Node {
	if lang == nil || fnStart <= start || fnEnd <= fnStart || int(fnEnd) > len(source) {
		return nil
	}
	pos := int(start)
	limit := int(fnStart)
	for pos < limit {
		switch source[pos] {
		case ' ', '\t', '\n', '\r':
			pos++
		default:
			goto found
		}
	}
found:
	if pos >= limit || source[pos] != '@' {
		return nil
	}
	offsetRoot, ok := parseSnippetRootWithOffset(lang, source, uint32(pos), fnEnd, parser)
	if !ok {
		return nil
	}
	for i := 0; i < offsetRoot.ChildCount(); i++ {
		child := offsetRoot.Child(i)
		if child == nil || child.Type(lang) != "function_definition" || child.HasError() {
			continue
		}
		var annotations []*Node
		for _, fnChild := range child.children {
			if fnChild == nil || fnChild.Type(lang) != "annotation" {
				break
			}
			annotations = append(annotations, cloneTreeNodesIntoArena(fnChild, arena))
		}
		if len(annotations) > 0 {
			return annotations
		}
	}
	return nil
}

func scalaFindMatchingBraceByteWithTrivia(source []byte, openPos int, limit uint32) int {
	return scalaFindMatchingDelimiterByteWithTrivia(source, openPos, int(limit), '{', '}')
}

func scalaFindMatchingParenByteWithTrivia(source []byte, openPos int, limit int) int {
	return scalaFindMatchingDelimiterByteWithTrivia(source, openPos, limit, '(', ')')
}

func scalaFindMatchingDelimiterByteWithTrivia(source []byte, openPos, limit int, openDelim, closeDelim byte) int {
	if openPos < 0 || openPos >= len(source) {
		return -1
	}
	if limit > len(source) {
		limit = len(source)
	}
	depth := 0
	inLineComment := false
	inBlockComment := false
	var stringQuote byte
	tripleQuote := false
	for i := openPos; i < limit; i++ {
		ch := source[i]
		next := byte(0)
		if i+1 < limit {
			next = source[i+1]
		}
		if inLineComment {
			if ch == '\n' {
				inLineComment = false
			}
			continue
		}
		if inBlockComment {
			if ch == '*' && next == '/' {
				inBlockComment = false
				i++
			}
			continue
		}
		if stringQuote != 0 {
			if tripleQuote {
				if i+2 < limit && source[i] == stringQuote && source[i+1] == stringQuote && source[i+2] == stringQuote {
					stringQuote = 0
					tripleQuote = false
					i += 2
				}
				continue
			}
			if ch == '\\' {
				i++
				continue
			}
			if ch == stringQuote {
				stringQuote = 0
			}
			continue
		}
		switch {
		case ch == '/' && next == '/':
			inLineComment = true
			i++
			continue
		case ch == '/' && next == '*':
			inBlockComment = true
			i++
			continue
		case ch == '"' || ch == '\'':
			if i+2 < limit && source[i+1] == ch && source[i+2] == ch {
				stringQuote = ch
				tripleQuote = true
				i += 2
				continue
			}
			stringQuote = ch
			tripleQuote = false
			continue
		case ch == openDelim:
			depth++
		case ch == closeDelim:
			depth--
			if depth == 0 {
				return i
			}
		}
	}
	return -1
}
