package gotreesitter

func normalizeHTMLCompatibility(root *Node, source []byte, lang *Language) {
	normalizeHTMLRecoveredNestedCustomTags(root, lang)
	normalizeHTMLRecoveredNestedCustomTagRanges(root, source, lang)
}

func normalizeHTMLRecoveredNestedCustomTags(root *Node, lang *Language) {
	if root == nil || lang == nil || lang.Name != "html" || root.Type(lang) != "ERROR" || len(root.children) < 5 {
		return
	}
	documentSym, ok := symbolByName(lang, "document")
	if !ok {
		return
	}
	elementSym, ok := symbolByName(lang, "element")
	if !ok {
		return
	}
	endTagSym, ok := symbolByName(lang, "end_tag")
	if !ok {
		return
	}
	startTags, nextIdx, ok := collectHTMLRecoveredStartTags(root.children, lang)
	if !ok || nextIdx+4 != len(root.children) {
		return
	}
	continuation := root.children[nextIdx]
	closeTok := root.children[nextIdx+1]
	tagName := root.children[nextIdx+2]
	closeAngle := root.children[nextIdx+3]
	if continuation == nil || continuation.Type(lang) != "element" || closeTok == nil || closeTok.Type(lang) != "</" || tagName == nil || tagName.Type(lang) != "tag_name" || closeAngle == nil || closeAngle.Type(lang) != ">" {
		return
	}
	htmlExtendOpenElementChain(continuation, closeTok.startByte, closeTok.startPoint, lang)
	endTagChildren := []*Node{closeTok, tagName, closeAngle}
	if root.ownerArena != nil {
		buf := root.ownerArena.allocNodeSlice(len(endTagChildren))
		copy(buf, endTagChildren)
		endTagChildren = buf
	}
	endTag := newParentNodeInArena(root.ownerArena, endTagSym, lang.SymbolMetadata[endTagSym].Named, endTagChildren, nil, 0)
	inner := continuation
	for i := len(startTags) - 1; i >= 1; i-- {
		children := []*Node{startTags[i], inner}
		if root.ownerArena != nil {
			buf := root.ownerArena.allocNodeSlice(len(children))
			copy(buf, children)
			children = buf
		}
		wrapper := newParentNodeInArena(root.ownerArena, elementSym, lang.SymbolMetadata[elementSym].Named, children, nil, 0)
		wrapper.endByte = closeTok.startByte
		wrapper.endPoint = closeTok.startPoint
		inner = wrapper
	}
	htmlExtendLeadingElementChain(inner, closeTok.startByte, closeTok.startPoint, lang)
	outerChildren := []*Node{startTags[0], inner, endTag}
	if root.ownerArena != nil {
		buf := root.ownerArena.allocNodeSlice(len(outerChildren))
		copy(buf, outerChildren)
		outerChildren = buf
	}
	outer := newParentNodeInArena(root.ownerArena, elementSym, lang.SymbolMetadata[elementSym].Named, outerChildren, nil, 0)
	root.children = []*Node{outer}
	if root.ownerArena != nil {
		buf := root.ownerArena.allocNodeSlice(1)
		buf[0] = outer
		root.children = buf
	}
	root.fieldIDs = nil
	root.fieldSources = nil
	root.symbol = documentSym
	root.isNamed = lang.SymbolMetadata[documentSym].Named
	root.hasError = outer.HasError()
}

func collectHTMLRecoveredStartTags(children []*Node, lang *Language) ([]*Node, int, bool) {
	startTags := make([]*Node, 0, len(children))
	for i, child := range children {
		if child == nil {
			continue
		}
		if startTag := htmlRecoveredStartTag(child, lang); startTag != nil {
			startTags = append(startTags, startTag)
			continue
		}
		if len(startTags) == 0 {
			return nil, 0, false
		}
		return startTags, i, true
	}
	return nil, 0, false
}

func htmlRecoveredStartTag(node *Node, lang *Language) *Node {
	if node == nil || lang == nil {
		return nil
	}
	if node.Type(lang) == "start_tag" {
		return node
	}
	if node.Type(lang) == "ERROR" && len(node.children) == 1 && node.children[0] != nil && node.children[0].Type(lang) == "start_tag" {
		return node.children[0]
	}
	return nil
}

func htmlExtendOpenElementChain(node *Node, endByte uint32, endPoint Point, lang *Language) {
	if node == nil || lang == nil || node.Type(lang) != "element" {
		return
	}
	hasEndTag := false
	for _, child := range node.children {
		if child == nil {
			continue
		}
		if child.Type(lang) == "end_tag" {
			hasEndTag = true
		}
		htmlExtendOpenElementChain(child, endByte, endPoint, lang)
	}
	if !hasEndTag {
		node.endByte = endByte
		node.endPoint = endPoint
	}
}

func htmlExtendLeadingElementChain(node *Node, endByte uint32, endPoint Point, lang *Language) {
	for cur := node; cur != nil && lang != nil && cur.Type(lang) == "element"; {
		cur.endByte = endByte
		cur.endPoint = endPoint
		if len(cur.children) < 2 || cur.children[1] == nil || cur.children[1].Type(lang) != "element" {
			return
		}
		cur = cur.children[1]
	}
}

func normalizeHTMLRecoveredNestedCustomTagRanges(root *Node, source []byte, lang *Language) {
	if root == nil || lang == nil || lang.Name != "html" || len(source) == 0 {
		return
	}
	var walk func(*Node)
	walk = func(node *Node) {
		if node == nil {
			return
		}
		for _, child := range node.children {
			walk(child)
		}
		if node.Type(lang) != "element" || len(node.children) < 2 {
			return
		}
		for i := 0; i+1 < len(node.children); i++ {
			left := node.children[i]
			right := node.children[i+1]
			if left == nil || right == nil || left.Type(lang) != "element" || right.Type(lang) != "end_tag" || len(right.children) == 0 {
				continue
			}
			closeTok := right.children[0]
			if closeTok == nil || closeTok.Type(lang) != "</" || left.endByte >= closeTok.startByte || closeTok.startByte > uint32(len(source)) {
				continue
			}
			if !bytesAreTrivia(source[left.endByte:closeTok.startByte]) {
				continue
			}
			htmlExtendLeadingElementChain(left, closeTok.startByte, closeTok.startPoint, lang)
		}
	}
	walk(root)
}
