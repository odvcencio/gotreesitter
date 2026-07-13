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
	endTagChildren = cloneNodeSliceIfArena(root.ownerArena, endTagChildren)
	endTag := newParentNodeInArena(root.ownerArena, endTagSym, symbolIsNamed(lang, endTagSym), endTagChildren, nil, 0)
	inner := continuation
	for i := len(startTags) - 1; i >= 1; i-- {
		children := []*Node{startTags[i], inner}
		children = cloneNodeSliceIfArena(root.ownerArena, children)
		wrapper := newParentNodeInArena(root.ownerArena, elementSym, symbolIsNamed(lang, elementSym), children, nil, 0)
		wrapper.endByte = closeTok.startByte
		wrapper.endPoint = closeTok.startPoint
		inner = wrapper
	}
	htmlExtendLeadingElementChain(inner, closeTok.startByte, closeTok.startPoint, lang)
	outerChildren := []*Node{startTags[0], inner, endTag}
	outerChildren = cloneNodeSliceIfArena(root.ownerArena, outerChildren)
	outer := newParentNodeInArena(root.ownerArena, elementSym, symbolIsNamed(lang, elementSym), outerChildren, nil, 0)
	root.children = cloneNodeSliceIfArena(root.ownerArena, []*Node{outer})
	root.clearFieldMetadata()
	retagResultRoot(root, documentSym, symbolIsNamed(lang, documentSym))
	root.setHasError(outer.HasError())
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
	if node.Type(lang) == "ERROR" && resultChildCount(node) == 1 {
		child := resultChildAt(node, 0)
		if child != nil && child.Type(lang) == "start_tag" {
			return child
		}
	}
	return nil
}

func htmlExtendOpenElementChain(node *Node, endByte uint32, endPoint Point, lang *Language) {
	if node == nil || lang == nil || node.Type(lang) != "element" {
		return
	}
	hasEndTag := false
	for i := 0; i < resultChildCount(node); i++ {
		child := resultChildAt(node, i)
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
	// Validate the complete chain before changing any range. Doing this once
	// also keeps deeply recovered HTML chains linear rather than re-walking the
	// remaining suffix for every element.
	if !htmlElementIsUnclosedChain(node, lang) {
		return
	}
	for cur := node; cur != nil && lang != nil && cur.Type(lang) == "element"; {
		// Only recovered chains of unclosed start tags should grow to the
		// enclosing close token. Closed or self-closing elements and elements
		// ending in concrete content already have an authoritative range.
		cur.endByte = endByte
		cur.endPoint = endPoint
		child := resultChildAt(cur, 1)
		if resultChildCount(cur) < 2 || child == nil || child.Type(lang) != "element" {
			return
		}
		cur = child
	}
}

func htmlElementIsUnclosedChain(node *Node, lang *Language) bool {
	for cur := node; cur != nil && lang != nil && cur.Type(lang) == "element"; {
		childCount := resultChildCount(cur)
		if childCount == 0 {
			return false
		}
		last := resultChildAt(cur, childCount-1)
		if last == nil {
			return false
		}
		switch last.Type(lang) {
		case "start_tag":
			return true
		case "element":
			cur = last
		default:
			return false
		}
	}
	return false
}

func normalizeHTMLRecoveredNestedCustomTagRanges(root *Node, source []byte, lang *Language) {
	if root == nil || lang == nil || lang.Name != "html" || len(source) == 0 {
		return
	}
	walkResultTreePostorder(root, func(node *Node) {
		childCount := resultChildCount(node)
		if node.Type(lang) != "element" || childCount < 2 {
			return
		}
		for i := 0; i+1 < childCount; i++ {
			left := resultChildAt(node, i)
			right := resultChildAt(node, i+1)
			if left == nil || right == nil || left.Type(lang) != "element" || right.Type(lang) != "end_tag" || resultChildCount(right) == 0 {
				continue
			}
			closeTok := resultChildAt(right, 0)
			if closeTok == nil || closeTok.Type(lang) != "</" || left.endByte >= closeTok.startByte || closeTok.startByte > uint32(len(source)) {
				continue
			}
			if !bytesAreTrivia(source[left.endByte:closeTok.startByte]) {
				continue
			}
			htmlExtendLeadingElementChain(left, closeTok.startByte, closeTok.startPoint, lang)
		}
	})
}
