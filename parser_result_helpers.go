package gotreesitter

import "bytes"

func isCobolLanguage(lang *Language) bool {
	return lang != nil && (lang.Name == "cobol" || lang.Name == "COBOL")
}

func cloneNodeSliceInArena(arena *nodeArena, nodes []*Node) []*Node {
	if len(nodes) == 0 {
		return nil
	}
	if arena != nil {
		buf := arena.allocNodeSlice(len(nodes))
		copy(buf, nodes)
		return buf
	}
	buf := make([]*Node, len(nodes))
	copy(buf, nodes)
	return buf
}

func resultChildCount(n *Node) int {
	return nodeChildCountNoMaterialize(n)
}

func resultChildAt(n *Node, i int) *Node {
	return nodeChildAtForReason(n, i, materializeForNormalization)
}

func resultDenseChildrenFallbackForMutation(n *Node) []*Node {
	if perfCountersEnabled {
		perfRecordDenseMutationChildrenCall()
		if nodeHasFinalChildRefs(n) {
			perfRecordDenseMutationChildrenDrain()
		}
	}
	return nodeChildrenForReason(n, materializeForNormalization)
}

func resultChildSliceForMutation(n *Node) []*Node {
	if n == nil {
		return nil
	}
	if !nodeHasFinalChildRefs(n) {
		return n.children
	}
	childCount := resultChildCount(n)
	if childCount == 0 {
		return nil
	}
	return resultChildSliceRangeForMutation(n, 0, childCount)
}

func resultChildSliceRangeForMutation(n *Node, start, end int) []*Node {
	if n == nil {
		return nil
	}
	if !nodeHasFinalChildRefs(n) {
		childCount := len(n.children)
		if start < 0 {
			start = 0
		}
		if end > childCount {
			end = childCount
		}
		if start >= end {
			return nil
		}
		return n.children[start:end]
	}
	childCount := resultChildCount(n)
	if start < 0 {
		start = 0
	}
	if end > childCount {
		end = childCount
	}
	if start >= end {
		return nil
	}
	children := make([]*Node, end-start)
	for i := start; i < end; i++ {
		children[i-start] = resultChildAt(n, i)
	}
	return children
}

type resultMutableChildView struct {
	parent *Node
	arena  *nodeArena
	refs   []pendingChildEntry
}

func resultMutableChildrenForMutation(parent *Node) resultMutableChildView {
	if parent == nil || parent.ownerArena == nil {
		return resultMutableChildView{parent: parent}
	}
	arena := parent.ownerArena
	childRange, ok := arena.finalChildRange(parent)
	if !ok {
		return resultMutableChildView{parent: parent}
	}
	refs := childRange.refs(arena)
	count := childRange.count()
	if len(refs) > count {
		refs = refs[:count]
	}
	return resultMutableChildView{
		parent: parent,
		arena:  arena,
		refs:   refs,
	}
}

func (v resultMutableChildView) hasFinalChildRefs() bool {
	return v.parent != nil && v.arena != nil && v.refs != nil
}

func (v resultMutableChildView) Len() int {
	if v.hasFinalChildRefs() {
		return len(v.refs)
	}
	return resultChildCount(v.parent)
}

func (v resultMutableChildView) Entry(i int) (stackEntry, bool) {
	if i < 0 || i >= v.Len() {
		return stackEntry{}, false
	}
	if v.hasFinalChildRefs() {
		entry := v.refs[i].stackEntry()
		return entry, stackEntryHasNode(entry)
	}
	child := resultChildAt(v.parent, i)
	if child == nil {
		return stackEntry{}, false
	}
	return newStackEntryNode(child.parseState, child), true
}

func (v resultMutableChildView) Child(i int) *Node {
	return resultChildAt(v.parent, i)
}

func (v resultMutableChildView) FilterFinalRefs(keep func(i int, entry stackEntry) bool) bool {
	if !v.hasFinalChildRefs() || keep == nil {
		return false
	}
	kept := make([]stackEntry, 0, len(v.refs))
	changed := false
	for i, ref := range v.refs {
		entry := ref.stackEntry()
		if !stackEntryHasNode(entry) {
			changed = true
			continue
		}
		if keep(i, entry) {
			kept = append(kept, entry)
			continue
		}
		changed = true
	}
	if !changed {
		return false
	}
	v.parent.children = nil
	v.parent.fieldIDs = nil
	v.parent.fieldSources = nil
	if len(kept) == 0 {
		v.arena.clearFinalChildRefs(v.parent)
		return true
	}
	childRange, refs := v.arena.allocPendingChildEntries(len(kept))
	for i, entry := range kept {
		refs[i] = newPendingChildEntry(entry)
	}
	sidecar, ok := v.arena.finalChildSidecarForNode(v.parent)
	if !ok {
		return false
	}
	sidecar.childRange = childRange
	if perfCountersEnabled {
		perfRecordMutationChildRefCopyOnWrite(len(kept))
	}
	return true
}

func (v resultMutableChildView) ReplaceFinalRefRangeWithNode(start, end int, replacement *Node) bool {
	if !v.hasFinalChildRefs() || replacement == nil || start < 0 || start >= end || end > len(v.refs) {
		return false
	}
	oldLen := len(v.refs)
	newLen := oldLen - (end - start) + 1
	childRange, refs := v.arena.allocPendingChildEntries(newLen)
	outIndex := 0
	copyEntry := func(entry stackEntry) {
		refs[outIndex] = newPendingChildEntry(entry)
		if child := stackEntryNode(entry); child != nil {
			setNodeParentLink(child, v.parent, outIndex)
		}
		outIndex++
	}
	for i := 0; i < start; i++ {
		copyEntry(v.refs[i].stackEntry())
	}
	copyEntry(newStackEntryNode(replacement.parseState, replacement))
	for i := end; i < oldLen; i++ {
		copyEntry(v.refs[i].stackEntry())
	}
	sidecar, ok := v.arena.finalChildSidecarForNode(v.parent)
	if !ok {
		return false
	}
	sidecar.childRange = childRange
	v.parent.children = nil

	if len(v.parent.fieldIDs) == oldLen {
		fieldIDs := make([]FieldID, 0, newLen)
		fieldIDs = append(fieldIDs, v.parent.fieldIDs[:start]...)
		mergedField := FieldID(0)
		for i := start; i < end; i++ {
			if v.parent.fieldIDs[i] != 0 {
				mergedField = v.parent.fieldIDs[i]
				break
			}
		}
		fieldIDs = append(fieldIDs, mergedField)
		fieldIDs = append(fieldIDs, v.parent.fieldIDs[end:]...)
		if v.parent.ownerArena != nil {
			buf := v.parent.ownerArena.allocFieldIDSlice(len(fieldIDs))
			copy(buf, fieldIDs)
			fieldIDs = buf
		}
		v.parent.fieldIDs = fieldIDs
	}
	if len(v.parent.fieldSources) == oldLen {
		fieldSources := make([]uint8, 0, newLen)
		fieldSources = append(fieldSources, v.parent.fieldSources[:start]...)
		mergedSource := uint8(fieldSourceNone)
		for i := start; i < end; i++ {
			if v.parent.fieldSources[i] != fieldSourceNone {
				mergedSource = v.parent.fieldSources[i]
				break
			}
		}
		fieldSources = append(fieldSources, mergedSource)
		fieldSources = append(fieldSources, v.parent.fieldSources[end:]...)
		if v.parent.ownerArena != nil {
			buf := v.parent.ownerArena.allocFieldSourceSlice(len(fieldSources))
			copy(buf, fieldSources)
			fieldSources = buf
		}
		v.parent.fieldSources = fieldSources
	}
	if perfCountersEnabled {
		perfRecordMutationChildRefCopyOnWrite(newLen)
	}
	return true
}

func (v resultMutableChildView) AppendFinalRefNode(child *Node) bool {
	if !v.hasFinalChildRefs() || child == nil {
		return false
	}
	oldLen := len(v.refs)
	newLen := oldLen + 1
	childRange, refs := v.arena.allocPendingChildEntries(newLen)
	for i, ref := range v.refs {
		entry := ref.stackEntry()
		refs[i] = newPendingChildEntry(entry)
		if existing := stackEntryNode(entry); existing != nil {
			setNodeParentLink(existing, v.parent, i)
		}
	}
	refs[oldLen] = newPendingChildEntry(newStackEntryNode(child.parseState, child))
	setNodeParentLink(child, v.parent, oldLen)
	sidecar, ok := v.arena.finalChildSidecarForNode(v.parent)
	if !ok {
		return false
	}
	sidecar.childRange = childRange
	v.parent.children = nil

	if len(v.parent.fieldIDs) == oldLen {
		fieldIDs := make([]FieldID, 0, newLen)
		fieldIDs = append(fieldIDs, v.parent.fieldIDs...)
		fieldIDs = append(fieldIDs, 0)
		if v.parent.ownerArena != nil {
			buf := v.parent.ownerArena.allocFieldIDSlice(len(fieldIDs))
			copy(buf, fieldIDs)
			fieldIDs = buf
		}
		v.parent.fieldIDs = fieldIDs
	}
	if len(v.parent.fieldSources) == oldLen {
		fieldSources := make([]uint8, 0, newLen)
		fieldSources = append(fieldSources, v.parent.fieldSources...)
		fieldSources = append(fieldSources, uint8(fieldSourceNone))
		if v.parent.ownerArena != nil {
			buf := v.parent.ownerArena.allocFieldSourceSlice(len(fieldSources))
			copy(buf, fieldSources)
			fieldSources = buf
		}
		v.parent.fieldSources = fieldSources
	}
	if perfCountersEnabled {
		perfRecordMutationChildRefCopyOnWrite(newLen)
	}
	return true
}

func (v resultMutableChildView) SurroundFinalRefs(prefix, suffix []*Node) bool {
	if !v.hasFinalChildRefs() || (len(prefix) == 0 && len(suffix) == 0) {
		return false
	}
	oldLen := len(v.refs)
	newLen := oldLen + len(prefix) + len(suffix)
	childRange, refs := v.arena.allocPendingChildEntries(newLen)
	outIndex := 0
	copyNode := func(node *Node) {
		refs[outIndex] = newPendingChildEntry(newStackEntryNode(node.parseState, node))
		setNodeParentLink(node, v.parent, outIndex)
		outIndex++
	}
	for _, node := range prefix {
		if node != nil {
			copyNode(node)
		}
	}
	leadingCount := outIndex
	for _, ref := range v.refs {
		entry := ref.stackEntry()
		refs[outIndex] = newPendingChildEntry(entry)
		if child := stackEntryNode(entry); child != nil {
			setNodeParentLink(child, v.parent, outIndex)
		}
		outIndex++
	}
	for _, node := range suffix {
		if node != nil {
			copyNode(node)
		}
	}
	if outIndex != newLen {
		childRange, refs = v.arena.allocPendingChildEntries(outIndex)
		outIndex = 0
		for _, node := range prefix {
			if node != nil {
				refs[outIndex] = newPendingChildEntry(newStackEntryNode(node.parseState, node))
				setNodeParentLink(node, v.parent, outIndex)
				outIndex++
			}
		}
		leadingCount = outIndex
		for _, ref := range v.refs {
			entry := ref.stackEntry()
			refs[outIndex] = newPendingChildEntry(entry)
			if child := stackEntryNode(entry); child != nil {
				setNodeParentLink(child, v.parent, outIndex)
			}
			outIndex++
		}
		for _, node := range suffix {
			if node != nil {
				refs[outIndex] = newPendingChildEntry(newStackEntryNode(node.parseState, node))
				setNodeParentLink(node, v.parent, outIndex)
				outIndex++
			}
		}
		newLen = outIndex
	}
	sidecar, ok := v.arena.finalChildSidecarForNode(v.parent)
	if !ok {
		return false
	}
	sidecar.childRange = childRange
	v.parent.children = nil

	if len(v.parent.fieldIDs) > 0 {
		fieldIDs := make([]FieldID, newLen)
		copy(fieldIDs[leadingCount:], v.parent.fieldIDs)
		if v.parent.ownerArena != nil {
			buf := v.parent.ownerArena.allocFieldIDSlice(len(fieldIDs))
			copy(buf, fieldIDs)
			fieldIDs = buf
		}
		v.parent.fieldIDs = fieldIDs
	}
	if len(v.parent.fieldSources) > 0 {
		fieldSources := make([]uint8, newLen)
		copy(fieldSources[leadingCount:], v.parent.fieldSources)
		if v.parent.ownerArena != nil {
			buf := v.parent.ownerArena.allocFieldSourceSlice(len(fieldSources))
			copy(buf, fieldSources)
			fieldSources = buf
		}
		v.parent.fieldSources = fieldSources
	}
	if perfCountersEnabled {
		perfRecordMutationChildRefCopyOnWrite(newLen)
	}
	return true
}

func cloneNodeInArenaWithFinalRefsForMutation(arena *nodeArena, node *Node) (*Node, bool) {
	if arena == nil || node == nil || !nodeHasFinalChildRefs(node) {
		return nil, false
	}
	cloned := arena.allocNode()
	cloneNodeHeaderInto(cloned, node, arena, nil)
	cloneNodeFieldMetadataInto(cloned, node, arena)
	if !cloneFinalChildRefsIntoArenaForMutation(node, cloned, arena) {
		return nil, false
	}
	return cloned, true
}

func cloneNodeInArenaPreservingFinalRefsForMutation(arena *nodeArena, node *Node) *Node {
	if cloned, ok := cloneNodeInArenaWithFinalRefsForMutation(arena, node); ok {
		return cloned
	}
	return cloneNodeInArena(arena, node)
}

func cloneNodeInArenaReplacingChildForMutation(arena *nodeArena, node *Node, childIndex int, replacement *Node) *Node {
	if cloned, ok := cloneNodeInArenaWithFinalRefsForMutation(arena, node); ok {
		if resultMutableChildrenForMutation(cloned).ReplaceFinalRefRangeWithNode(childIndex, childIndex+1, replacement) {
			return cloned
		}
	}

	cloned := cloneNodeInArena(arena, node)
	if cloned == nil {
		return nil
	}
	children := cloneNodeSliceInArena(arena, resultChildSliceForMutation(node))
	if childIndex >= 0 && childIndex < len(children) {
		children[childIndex] = replacement
	}
	cloned.children = children
	populateParentNode(cloned, cloned.children)
	return cloned
}

func cloneNodeInArenaAppendingChildForMutation(arena *nodeArena, node *Node, child *Node) *Node {
	if cloned, ok := cloneNodeInArenaWithFinalRefsForMutation(arena, node); ok {
		if resultMutableChildrenForMutation(cloned).AppendFinalRefNode(child) {
			return cloned
		}
	}

	cloned := cloneNodeInArena(arena, node)
	if cloned == nil {
		return nil
	}
	children := resultChildSliceForMutation(node)
	out := make([]*Node, 0, len(children)+1)
	out = append(out, children...)
	out = append(out, child)
	cloned.children = cloneNodeSliceInArena(arena, out)
	populateParentNode(cloned, cloned.children)
	return cloned
}

func symbolTypeName(lang *Language, sym Symbol) string {
	if lang == nil || int(sym) >= len(lang.SymbolNames) {
		return ""
	}
	return unescapePunctuationSymbolName(lang.SymbolNames[sym])
}

func cloneNodeSliceIfArena(arena *nodeArena, nodes []*Node) []*Node {
	if arena == nil {
		return nodes
	}
	return cloneNodeSliceInArena(arena, nodes)
}

func cloneFieldIDSliceInArena(arena *nodeArena, fieldIDs []FieldID) []FieldID {
	if len(fieldIDs) == 0 {
		return nil
	}
	if arena != nil {
		out := arena.allocFieldIDSlice(len(fieldIDs))
		copy(out, fieldIDs)
		return out
	}
	out := make([]FieldID, len(fieldIDs))
	copy(out, fieldIDs)
	return out
}

func symbolByName(lang *Language, name string) (Symbol, bool) {
	if lang == nil {
		return 0, false
	}
	return lang.SymbolByName(name)
}

func symbolIsNamed(lang *Language, sym Symbol) bool {
	return lang != nil && int(sym) < len(lang.SymbolMetadata) && lang.SymbolMetadata[sym].Named
}

func symbolHasMetadata(lang *Language, sym Symbol) bool {
	return lang != nil && int(sym) < len(lang.SymbolMetadata)
}

func symbolIsVisible(lang *Language, sym Symbol) bool {
	return lang != nil && int(sym) < len(lang.SymbolMetadata) && lang.SymbolMetadata[sym].Visible
}

func symbolMeta(lang *Language, name string) (Symbol, bool, bool) {
	sym, ok := symbolByName(lang, name)
	if !ok {
		return 0, false, false
	}
	return sym, symbolIsNamed(lang, sym), true
}

func languageSymbols(lang *Language, names ...string) ([]Symbol, bool) {
	if lang == nil {
		return nil, false
	}
	syms := make([]Symbol, len(names))
	for i, name := range names {
		sym, ok := lang.SymbolByName(name)
		if !ok {
			return nil, false
		}
		syms[i] = sym
	}
	return syms, true
}

func visibleLanguageSymbols(lang *Language, named bool, names ...string) ([]Symbol, bool) {
	if lang == nil {
		return nil, false
	}
	syms := make([]Symbol, len(names))
	for i, name := range names {
		sym, ok := findVisibleSymbolByName(lang, name, named)
		if !ok {
			return nil, false
		}
		syms[i] = sym
	}
	return syms, true
}

func extendNodeEndTo(n *Node, end uint32, source []byte) {
	if n == nil || end <= n.endByte || end > uint32(len(source)) {
		return
	}
	gap := source[n.endByte:end]
	n.endByte = end
	n.endPoint = advancePointByBytes(n.endPoint, gap)
}

func setNodeEndTo(n *Node, end uint32, source []byte) {
	if n == nil || end > uint32(len(source)) || end < n.startByte || end == n.endByte {
		return
	}
	if end > n.endByte {
		extendNodeEndTo(n, end, source)
		return
	}
	n.endByte = end
	n.endPoint = advancePointByBytes(Point{}, source[:end])
}

func advancePointByBytes(start Point, b []byte) Point {
	p := start
	if len(b) < 256 {
		for _, c := range b {
			if c == '\n' {
				p.Row++
				p.Column = 0
				continue
			}
			p.Column++
		}
		return p
	}
	lastNewline := bytes.LastIndexByte(b, '\n')
	if lastNewline < 0 {
		p.Column += uint32(len(b))
		return p
	}
	p.Row += uint32(bytes.Count(b, []byte{'\n'}))
	p.Column = uint32(len(b) - lastNewline - 1)
	return p
}
