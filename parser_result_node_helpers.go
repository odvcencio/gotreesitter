package gotreesitter

func ensureNodeFieldStorage(n *Node, childCount int) {
	if n == nil || childCount <= 0 {
		return
	}
	if len(n.fieldIDs) != childCount {
		fieldIDs := make([]FieldID, childCount)
		copy(fieldIDs, n.fieldIDs)
		if n.ownerArena != nil {
			buf := n.ownerArena.allocFieldIDSlice(childCount)
			copy(buf, fieldIDs)
			fieldIDs = buf
		}
		n.fieldIDs = fieldIDs
	}
	if len(n.fieldSources) != childCount {
		fieldSources := make([]uint8, childCount)
		copy(fieldSources, n.fieldSources)
		if n.ownerArena != nil {
			buf := n.ownerArena.allocFieldSourceSlice(childCount)
			copy(buf, fieldSources)
			fieldSources = buf
		}
		n.fieldSources = fieldSources
	}
}

func setNodeChildField(n *Node, childIndex int, fid FieldID, source uint8, overwrite bool) bool {
	if n == nil || childIndex < 0 || childIndex >= len(n.children) || fid == 0 {
		return false
	}
	ensureNodeFieldStorage(n, len(n.children))
	if !overwrite && n.fieldIDs[childIndex] != 0 {
		return false
	}
	n.fieldIDs[childIndex] = fid
	n.fieldSources[childIndex] = source
	return true
}

func setNodeChildFieldDirect(n *Node, childIndex int, fid FieldID) bool {
	return setNodeChildField(n, childIndex, fid, fieldSourceDirect, true)
}

func setNodeChildFieldInheritedIfEmpty(n *Node, childIndex int, fid FieldID) bool {
	return setNodeChildField(n, childIndex, fid, fieldSourceInherited, false)
}

func clearNodeChildField(n *Node, childIndex int) bool {
	if n == nil || childIndex < 0 || childIndex >= len(n.children) {
		return false
	}
	if len(n.fieldIDs) == len(n.children) {
		n.fieldIDs[childIndex] = 0
	}
	if len(n.fieldSources) == len(n.children) {
		n.fieldSources[childIndex] = fieldSourceNone
	}
	return true
}

func replaceNodeChildrenUnfielded(n *Node, children []*Node) {
	if n == nil {
		return
	}
	n.children = children
	n.fieldIDs = nil
	n.fieldSources = nil
	if n.ownerArena != nil {
		n.ownerArena.clearFinalChildRefs(n)
	}
	populateParentNode(n, n.children)
}

func walkResultTree(root *Node, visit func(*Node)) {
	if visit == nil {
		return
	}
	var walk func(*Node)
	walk = func(n *Node) {
		if n == nil {
			return
		}
		visit(n)
		if n.ownerArena == nil || n.childIndex > finalChildSidecarIndexBase {
			for _, child := range n.children {
				walk(child)
			}
			return
		}
		for i := 0; i < resultChildCount(n); i++ {
			walk(resultChildAt(n, i))
		}
	}
	walk(root)
}

func walkResultTreeDenseFirst(root *Node, visit func(*Node)) {
	if visit == nil {
		return
	}
	var walk func(*Node)
	walk = func(n *Node) {
		if n == nil {
			return
		}
		visit(n)
		if n.ownerArena == nil || n.childIndex > finalChildSidecarIndexBase {
			for _, child := range n.children {
				walk(child)
			}
			return
		}
		for i := 0; i < resultChildCount(n); i++ {
			walk(resultChildAt(n, i))
		}
	}
	walk(root)
}

func walkResultTreePostorder(root *Node, visit func(*Node)) {
	if visit == nil {
		return
	}
	var walk func(*Node)
	walk = func(n *Node) {
		if n == nil {
			return
		}
		if n.ownerArena == nil || n.childIndex > finalChildSidecarIndexBase {
			for _, child := range n.children {
				walk(child)
			}
			visit(n)
			return
		}
		for i := 0; i < resultChildCount(n); i++ {
			walk(resultChildAt(n, i))
		}
		visit(n)
	}
	walk(root)
}

func walkResultTreeBounded(root *Node, visit func(*Node)) {
	if visit == nil {
		return
	}
	var walk func(*Node, int)
	walk = func(n *Node, depth int) {
		if n == nil || depth > maxTreeWalkDepth {
			return
		}
		visit(n)
		for i := 0; i < resultChildCount(n); i++ {
			walk(resultChildAt(n, i), depth+1)
		}
	}
	walk(root, 0)
}

func rewriteResultTreeChildrenPostorder(root *Node, rewrite func(*Node) *Node) {
	if rewrite == nil {
		return
	}
	walkResultTreePostorder(root, func(n *Node) {
		children := n.children
		if n != nil && n.ownerArena != nil && n.childIndex <= finalChildSidecarIndexBase {
			children = resultDenseChildrenFallbackForMutation(n)
		}
		for i, child := range children {
			for {
				rewritten := rewrite(child)
				if rewritten == nil {
					break
				}
				children[i] = rewritten
				rewritten.parent = n
				rewritten.childIndex = int32(i)
				child = rewritten
			}
		}
	})
}

func rewriteResultTreeChildrenPostorderWithStats(root *Node, rewrite func(*Node) *Node) normalizationPassCounters {
	var counters normalizationPassCounters
	if rewrite == nil {
		return counters
	}
	walkResultTreePostorder(root, func(n *Node) {
		counters.nodesVisited++
		children := n.children
		if n != nil && n.ownerArena != nil && n.childIndex <= finalChildSidecarIndexBase {
			children = resultDenseChildrenFallbackForMutation(n)
		}
		for i, child := range children {
			for {
				rewritten := rewrite(child)
				if rewritten == nil {
					break
				}
				counters.nodesRewritten++
				children[i] = rewritten
				rewritten.parent = n
				rewritten.childIndex = int32(i)
				child = rewritten
			}
		}
	})
	return counters
}

func replaceChildRangeWithSingleNode(parent *Node, start, end int, replacement *Node) {
	if parent == nil || replacement == nil {
		return
	}
	childCount := resultChildCount(parent)
	if start < 0 || start >= end || end > childCount {
		return
	}
	if resultMutableChildrenForMutation(parent).ReplaceFinalRefRangeWithNode(start, end, replacement) {
		return
	}
	children := resultDenseChildrenFallbackForMutation(parent)
	oldLen := len(children)
	newChildren := make([]*Node, 0, oldLen-(end-start)+1)
	newChildren = append(newChildren, children[:start]...)
	newChildren = append(newChildren, replacement)
	newChildren = append(newChildren, children[end:]...)
	parent.children = newChildren

	if len(parent.fieldIDs) == oldLen {
		newFieldIDs := make([]FieldID, 0, len(newChildren))
		newFieldIDs = append(newFieldIDs, parent.fieldIDs[:start]...)
		mergedField := FieldID(0)
		for i := start; i < end; i++ {
			if parent.fieldIDs[i] != 0 {
				mergedField = parent.fieldIDs[i]
				break
			}
		}
		newFieldIDs = append(newFieldIDs, mergedField)
		newFieldIDs = append(newFieldIDs, parent.fieldIDs[end:]...)
		parent.fieldIDs = newFieldIDs
	}
	if len(parent.fieldSources) == oldLen {
		newFieldSources := make([]uint8, 0, len(newChildren))
		newFieldSources = append(newFieldSources, parent.fieldSources[:start]...)
		mergedSource := uint8(fieldSourceNone)
		for i := start; i < end; i++ {
			if parent.fieldSources[i] != fieldSourceNone {
				mergedSource = parent.fieldSources[i]
				break
			}
		}
		newFieldSources = append(newFieldSources, mergedSource)
		newFieldSources = append(newFieldSources, parent.fieldSources[end:]...)
		parent.fieldSources = newFieldSources
	}
	for i, child := range parent.children {
		if child == nil {
			continue
		}
		child.parent = parent
		child.childIndex = int32(i)
	}
}

func firstAndLastNonNilChild(children []*Node) (*Node, *Node) {
	var first *Node
	for _, child := range children {
		if child != nil {
			first = child
			break
		}
	}
	if first == nil {
		return nil, nil
	}
	for i := len(children) - 1; i >= 0; i-- {
		if children[i] != nil {
			return first, children[i]
		}
	}
	return first, first
}

func bytesContainLineBreak(b []byte) bool {
	for _, c := range b {
		if c == '\n' || c == '\r' {
			return true
		}
	}
	return false
}

func firstNonWhitespaceByte(source []byte) uint32 {
	for i, c := range source {
		switch c {
		case ' ', '\t', '\n', '\r':
			continue
		default:
			return uint32(i)
		}
	}
	return 0
}
func dropZeroWidthUnnamedTail(nodes []*Node, lang *Language) []*Node {
	for len(nodes) > 0 {
		last := nodes[len(nodes)-1]
		if last == nil {
			nodes = nodes[:len(nodes)-1]
			continue
		}
		if last.IsNamed() || last.startByte != last.endByte || len(last.children) > 0 {
			break
		}
		if lang != nil && last.Type(lang) != "" {
			break
		}
		nodes = nodes[:len(nodes)-1]
	}
	return nodes
}
