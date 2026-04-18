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
		n.fieldSources = fieldSources
	}
}
func replaceChildRangeWithSingleNode(parent *Node, start, end int, replacement *Node) {
	if parent == nil || replacement == nil || start < 0 || start >= end || end > len(parent.children) {
		return
	}
	oldLen := len(parent.children)
	newChildren := make([]*Node, 0, oldLen-(end-start)+1)
	newChildren = append(newChildren, parent.children[:start]...)
	newChildren = append(newChildren, replacement)
	newChildren = append(newChildren, parent.children[end:]...)
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
		child.childIndex = i
	}
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
