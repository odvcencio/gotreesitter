package gotreesitter

import "bytes"

func normalizeCooklangCompatibilityWithCensus(
	root *Node,
	source []byte,
	lang *Language,
	census materializationSubpassCensus,
) {
	census.run("dispatch.cooklang.recovered-recipe", func() {
		normalizeCooklangRecoveredRecipe(root, source, lang)
	})
}

func normalizeCooklangRecoveredRecipe(root *Node, source []byte, lang *Language) {
	if root == nil || lang == nil || lang.Name != "cooklang" || root.Type(lang) != "recipe" {
		return
	}
	stepSym, okStep := lang.SymbolByName("step")
	metadataSym, okMetadata := lang.SymbolByName("metadata")
	commentSym, okComment := lang.SymbolByName("comment")
	if !okStep || !okMetadata || !okComment {
		return
	}
	cooklangNormalizeNestedPunctuationErrors(root, source, lang)
	children := resultChildSliceForMutation(root)
	if len(children) == 0 {
		return
	}
	out := make([]*Node, 0, len(children))
	for i := 0; i < len(children); i++ {
		child := children[i]
		if child == nil {
			continue
		}
		if cooklangNodeTextEquals(child, source, "---") && child.Type(lang) == "step" {
			child.symbol = commentSym
			child.setNamed(symbolIsNamed(lang, commentSym))
			cooklangNormalizeFenceComment(child, source)
		}
		if child.Type(lang) == "metadata" {
			if split := cooklangSplitMetadataLines(child, source, metadataSym); len(split) > 0 {
				out = append(out, split...)
				continue
			}
			if i+1 < len(children) && cooklangCanExtendMetadataToError(child, children[i+1], source, lang) {
				extendNodeEndTo(child, children[i+1].endByte, source)
				cooklangNormalizeMetadataChildren(child, lang)
				i++
				out = append(out, child)
				continue
			}
			cooklangNormalizeMetadataChildren(child, lang)
		}
		if child.Type(lang) == "ERROR" {
			if repl, consumed := cooklangRewriteTopLevelError(child, children, i, source, lang, metadataSym, stepSym); consumed > 0 {
				out = append(out, repl...)
				i += consumed - 1
				continue
			}
			if cooklangErrorIsStepText(child, source, lang) {
				child.symbol = stepSym
				child.setNamed(symbolIsNamed(lang, stepSym))
			}
		}
		if len(out) > 0 && cooklangIsStandalonePunctuationError(child, source, lang) {
			prev := out[len(out)-1]
			if prev != nil && (prev.Type(lang) == "step" || prev.Type(lang) == "metadata") && child.startByte == prev.endByte {
				extendNodeEndTo(prev, child.endByte, source)
				continue
			}
		}
		if child.Type(lang) == "step" && child.startByte > 0 && child.startByte <= uint32(len(source)) && source[child.startByte-1] == ' ' {
			if len(out) > 0 && out[len(out)-1] != nil && out[len(out)-1].endByte == child.startByte-1 {
				cooklangSetNodeStartTo(child, child.startByte-1, source)
			}
		}
		out = append(out, child)
	}
	replaceNodeChildrenUnfielded(root, cloneNodeSliceInArena(root.ownerArena, out))
}

func cooklangNormalizeNestedPunctuationErrors(n *Node, source []byte, lang *Language) {
	if n == nil {
		return
	}
	if n.Type(lang) == "ERROR" {
		n.setNamed(true)
	}
	for i := 0; i < resultChildCount(n); i++ {
		cooklangNormalizeNestedPunctuationErrors(resultChildAt(n, i), source, lang)
	}
	switch n.Type(lang) {
	case "step", "metadata":
	default:
		return
	}
	children := resultChildSliceForMutation(n)
	if len(children) == 0 {
		return
	}
	out := children[:0]
	changed := false
	for _, child := range children {
		if cooklangIsStandalonePunctuationError(child, source, lang) {
			changed = true
			continue
		}
		out = append(out, child)
	}
	if changed {
		cooklangReplaceChildrenPreserveSpan(n, cloneNodeSliceInArena(n.ownerArena, out), source)
	}
}

func cooklangSplitMetadataLines(n *Node, source []byte, metadataSym Symbol) []*Node {
	if n == nil || int(n.endByte) > len(source) || n.startByte >= n.endByte {
		return nil
	}
	text := source[n.startByte:n.endByte]
	rel := bytes.LastIndex(text, []byte("\n>> "))
	if rel <= 0 {
		return nil
	}
	firstEnd := n.startByte + uint32(rel)
	secondStart := firstEnd + 1
	if firstEnd <= n.startByte || secondStart >= n.endByte {
		return nil
	}
	first := cloneNodeInArena(n.ownerArena, n)
	second := cloneNodeInArena(n.ownerArena, n)
	first.symbol = metadataSym
	second.symbol = metadataSym
	cooklangSetNodeEndExact(first, firstEnd, source)
	cooklangSetNodeStartTo(second, secondStart, source)
	cooklangReplaceChildrenPreserveSpan(first, nil, source)
	cooklangReplaceChildrenPreserveSpan(second, nil, source)
	return []*Node{first, second}
}

func cooklangCanExtendMetadataToError(meta, err *Node, source []byte, lang *Language) bool {
	if meta == nil || err == nil || err.Type(lang) != "ERROR" || err.startByte != meta.endByte || int(err.endByte) > len(source) {
		return false
	}
	if bytes.IndexByte(source[err.startByte:err.endByte], '\n') >= 0 {
		return false
	}
	return true
}

func cooklangRewriteTopLevelError(err *Node, siblings []*Node, idx int, source []byte, lang *Language, metadataSym, stepSym Symbol) ([]*Node, int) {
	if err == nil || int(err.endByte) > len(source) {
		return nil, 0
	}
	text := source[err.startByte:err.endByte]
	if bytes.HasPrefix(text, []byte(">> ")) && bytes.Contains(text, []byte("\n>> ")) && idx+1 < len(siblings) {
		next := siblings[idx+1]
		if next != nil && next.Type(lang) == "step" && next.startByte == err.endByte {
			rel := bytes.LastIndex(text, []byte("\n>> "))
			if rel > 0 {
				firstEnd := err.startByte + uint32(rel)
				secondStart := firstEnd + 1
				first := cloneNodeInArena(err.ownerArena, err)
				first.symbol = metadataSym
				first.setNamed(symbolIsNamed(lang, metadataSym))
				first.setExtra(false)
				cooklangSetNodeEndExact(first, firstEnd, source)
				second := cloneNodeInArena(next.ownerArena, next)
				second.symbol = metadataSym
				second.setNamed(symbolIsNamed(lang, metadataSym))
				cooklangSetNodeStartTo(second, secondStart, source)
				cooklangReplaceChildrenPreserveSpan(first, nil, source)
				cooklangNormalizeMetadataChildren(second, lang)
				return []*Node{first, second}, 2
			}
		}
	}
	if resultChildCount(err) == 1 {
		child := resultChildAt(err, 0)
		if child != nil && child.Type(lang) == "step" && child.startByte == err.startByte && child.endByte == err.endByte {
			child.symbol = stepSym
			child.setNamed(symbolIsNamed(lang, stepSym))
			return []*Node{child}, 1
		}
	}
	return nil, 0
}

func cooklangNormalizeMetadataChildren(n *Node, lang *Language) {
	if n == nil {
		return
	}
	children := resultChildSliceForMutation(n)
	if len(children) == 0 {
		return
	}
	out := children[:0]
	changed := false
	for _, child := range children {
		if child == nil {
			changed = true
			continue
		}
		switch child.Type(lang) {
		case ">>", ":", "ERROR":
			out = append(out, child)
		default:
			changed = true
		}
	}
	if changed {
		cooklangReplaceChildrenPreserveSpan(n, cloneNodeSliceInArena(n.ownerArena, out), nil)
	}
}

func cooklangNormalizeFenceComment(n *Node, source []byte) {
	children := resultChildSliceForMutation(n)
	if len(children) <= 2 {
		return
	}
	cooklangReplaceChildrenPreserveSpan(n, cloneNodeSliceInArena(n.ownerArena, children[:2]), source)
}

func cooklangIsStandalonePunctuationError(n *Node, source []byte, lang *Language) bool {
	if n == nil || n.Type(lang) != "ERROR" || n.endByte <= n.startByte || int(n.endByte) > len(source) {
		return false
	}
	text := source[n.startByte:n.endByte]
	if len(text) == 0 || len(text) > 2 {
		return false
	}
	for _, c := range text {
		switch c {
		case '.', ',', '!', '?', '/':
		default:
			return false
		}
	}
	return true
}

func cooklangErrorIsStepText(n *Node, source []byte, lang *Language) bool {
	if n == nil || n.Type(lang) != "ERROR" || resultChildCount(n) != 1 {
		return false
	}
	child := resultChildAt(n, 0)
	return child != nil && child.Type(lang) == "step" && child.startByte == n.startByte && child.endByte == n.endByte
}

func cooklangNodeTextEquals(n *Node, source []byte, want string) bool {
	return n != nil && int(n.endByte) <= len(source) && string(source[n.startByte:n.endByte]) == want
}

func cooklangSetNodeStartTo(n *Node, start uint32, source []byte) {
	if n == nil || start > n.endByte || int(start) > len(source) || start == n.startByte {
		return
	}
	n.startByte = start
	n.startPoint = advancePointByBytes(Point{}, source[:start])
}

func cooklangSetNodeEndExact(n *Node, end uint32, source []byte) {
	if n == nil || end < n.startByte || int(end) > len(source) || end == n.endByte {
		return
	}
	n.endByte = end
	n.endPoint = advancePointByBytes(Point{}, source[:end])
}

func cooklangReplaceChildrenPreserveSpan(n *Node, children []*Node, source []byte) {
	if n == nil {
		return
	}
	startByte, endByte := n.startByte, n.endByte
	startPoint, endPoint := n.startPoint, n.endPoint
	replaceNodeChildrenUnfielded(n, children)
	n.startByte, n.endByte = startByte, endByte
	n.startPoint, n.endPoint = startPoint, endPoint
	if source != nil && int(endByte) <= len(source) {
		n.startPoint = advancePointByBytes(Point{}, source[:startByte])
		n.endPoint = advancePointByBytes(Point{}, source[:endByte])
	}
}
