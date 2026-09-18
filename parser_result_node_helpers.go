package gotreesitter

func ensureNodeFieldStorage(n *Node, childCount int) {
	if n == nil || childCount <= 0 {
		return
	}
	fieldIDs := n.fieldIDs()
	fieldSources := n.fieldSources()
	changed := false
	if len(fieldIDs) != childCount {
		rebuilt := make([]FieldID, childCount)
		copy(rebuilt, fieldIDs)
		if n.ownerArena != nil {
			buf := n.ownerArena.allocFieldIDSlice(childCount)
			copy(buf, rebuilt)
			rebuilt = buf
		}
		fieldIDs = rebuilt
		changed = true
	}
	if len(fieldSources) != childCount {
		rebuilt := make([]uint8, childCount)
		copy(rebuilt, fieldSources)
		if n.ownerArena != nil {
			buf := n.ownerArena.allocFieldSourceSlice(childCount)
			copy(buf, rebuilt)
			rebuilt = buf
		}
		fieldSources = rebuilt
		changed = true
	}
	if changed {
		n.setFieldMetadata(fieldIDs, fieldSources)
	}
}

func setNodeChildField(n *Node, childIndex int, fid FieldID, source uint8, overwrite bool) bool {
	if n == nil || childIndex < 0 || childIndex >= len(n.children) || fid == 0 {
		return false
	}
	ensureNodeFieldStorage(n, len(n.children))
	fieldIDs := n.fieldIDs()
	fieldSources := n.fieldSources()
	if !overwrite && fieldIDs[childIndex] != 0 {
		return false
	}
	fieldIDs[childIndex] = fid
	fieldSources[childIndex] = source
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
	fieldIDs := n.fieldIDs()
	fieldSources := n.fieldSources()
	if len(fieldIDs) == len(n.children) {
		fieldIDs[childIndex] = 0
	}
	if len(fieldSources) == len(n.children) {
		fieldSources[childIndex] = fieldSourceNone
	}
	return true
}

func replaceNodeChildrenUnfielded(n *Node, children []*Node) {
	if n == nil {
		return
	}
	n.children = children
	n.clearFieldMetadata()
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

func walkResultTreeUntil(root *Node, visit func(*Node) bool) bool {
	if visit == nil {
		return true
	}
	var walk func(*Node) bool
	walk = func(n *Node) bool {
		if n == nil {
			return true
		}
		if !visit(n) {
			return false
		}
		if n.ownerArena == nil || n.childIndex > finalChildSidecarIndexBase {
			for _, child := range n.children {
				if !walk(child) {
					return false
				}
			}
			return true
		}
		for i := 0; i < resultChildCount(n); i++ {
			if !walk(resultChildAt(n, i)) {
				return false
			}
		}
		return true
	}
	return walk(root)
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

func walkResultTreeSidecarFirst(root *Node, visit func(*Node)) {
	if visit == nil {
		return
	}
	var walk func(*Node)
	walk = func(n *Node) {
		if n == nil {
			return
		}
		visit(n)
		if n.childIndex > finalChildSidecarIndexBase || n.ownerArena == nil {
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

func walkResultTreePostorderSidecarFirst(root *Node, visit func(*Node)) {
	if visit == nil {
		return
	}
	var walk func(*Node)
	walk = func(n *Node) {
		if n == nil {
			return
		}
		if n.childIndex > finalChildSidecarIndexBase || n.ownerArena == nil {
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
		if n == nil {
			return
		}
		children := n.children
		if n.ownerArena != nil && n.childIndex <= finalChildSidecarIndexBase {
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

	fieldIDs := parent.fieldIDs()
	fieldSources := parent.fieldSources()
	metadataChanged := false
	if len(fieldIDs) == oldLen {
		newFieldIDs := make([]FieldID, 0, len(newChildren))
		newFieldIDs = append(newFieldIDs, fieldIDs[:start]...)
		mergedField := FieldID(0)
		for i := start; i < end; i++ {
			if fieldIDs[i] != 0 {
				mergedField = fieldIDs[i]
				break
			}
		}
		newFieldIDs = append(newFieldIDs, mergedField)
		newFieldIDs = append(newFieldIDs, fieldIDs[end:]...)
		fieldIDs = newFieldIDs
		metadataChanged = true
	}
	if len(fieldSources) == oldLen {
		newFieldSources := make([]uint8, 0, len(newChildren))
		newFieldSources = append(newFieldSources, fieldSources[:start]...)
		mergedSource := uint8(fieldSourceNone)
		for i := start; i < end; i++ {
			if fieldSources[i] != fieldSourceNone {
				mergedSource = fieldSources[i]
				break
			}
		}
		newFieldSources = append(newFieldSources, mergedSource)
		newFieldSources = append(newFieldSources, fieldSources[end:]...)
		fieldSources = newFieldSources
		metadataChanged = true
	}
	if metadataChanged {
		parent.setFieldMetadata(fieldIDs, fieldSources)
	}
	for i, child := range parent.children {
		if child == nil {
			continue
		}
		child.parent = parent
		child.childIndex = int32(i)
	}
}

func replaceChildRangeWithNodes(parent *Node, start, end int, replacements []*Node) {
	if parent == nil || len(replacements) == 0 {
		return
	}
	childCount := resultChildCount(parent)
	if start < 0 || start >= end || end > childCount {
		return
	}
	children := resultDenseChildrenFallbackForMutation(parent)
	oldLen := len(children)
	newChildren := make([]*Node, 0, oldLen-(end-start)+len(replacements))
	newChildren = append(newChildren, children[:start]...)
	newChildren = append(newChildren, replacements...)
	newChildren = append(newChildren, children[end:]...)
	parent.children = newChildren
	if parent.ownerArena != nil {
		parent.ownerArena.clearFinalChildRefs(parent)
	}

	fieldIDs := parent.fieldIDs()
	fieldSources := parent.fieldSources()
	metadataChanged := false
	if len(fieldIDs) == oldLen {
		newFieldIDs := make([]FieldID, 0, len(newChildren))
		newFieldIDs = append(newFieldIDs, fieldIDs[:start]...)
		for range replacements {
			newFieldIDs = append(newFieldIDs, 0)
		}
		newFieldIDs = append(newFieldIDs, fieldIDs[end:]...)
		fieldIDs = newFieldIDs
		metadataChanged = true
	}
	if len(fieldSources) == oldLen {
		newFieldSources := make([]uint8, 0, len(newChildren))
		newFieldSources = append(newFieldSources, fieldSources[:start]...)
		for range replacements {
			newFieldSources = append(newFieldSources, fieldSourceNone)
		}
		newFieldSources = append(newFieldSources, fieldSources[end:]...)
		fieldSources = newFieldSources
		metadataChanged = true
	}
	if metadataChanged {
		parent.setFieldMetadata(fieldIDs, fieldSources)
	}
	populateParentNode(parent, parent.children)
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
		if lang != nil && (last.symbol == errorSymbol || (int(last.symbol) < len(lang.SymbolNames) && lang.SymbolNames[last.symbol] != "")) {
			break
		}
		nodes = nodes[:len(nodes)-1]
	}
	return nodes
}
