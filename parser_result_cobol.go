package gotreesitter

import "strings"

func normalizeCobolCompatibility(root *Node, source []byte, lang *Language) {
	normalizeCobolLeadingAreaStart(root, source, lang)
	normalizeCobolTopLevelDefinitionEnd(root, source, lang)
	normalizeCobolDivisionSiblingEnds(root, source, lang)
	normalizeCobolRecoveredParagraphHeader(root, source, lang)
}
func normalizeCobolLeadingAreaStart(root *Node, source []byte, lang *Language) {
	if root == nil || !isCobolLanguage(lang) || len(source) == 0 {
		return
	}
	rootStart, defStart, ok := cobolLeadingStarts(source)
	if !ok {
		return
	}
	setNodeStartTo := func(n *Node, start uint32) {
		if n == nil || n.startByte == start {
			return
		}
		startPoint := advancePointByBytes(Point{}, source[:start])
		n.startByte = start
		n.startPoint = startPoint
	}
	setNodeStartTo(root, rootStart)
	if resultChildCount(root) == 0 {
		return
	}
	def := (*Node)(nil)
	for i := 0; i < resultChildCount(root); i++ {
		child := resultChildAt(root, i)
		if child != nil && !child.IsExtra() && child.Type(lang) == "program_definition" {
			def = child
			break
		}
	}
	if def == nil {
		return
	}
	setNodeStartTo(def, defStart)
	if resultChildCount(def) == 0 {
		return
	}
	for i := 0; i < resultChildCount(def); i++ {
		child := resultChildAt(def, i)
		if child != nil && !child.IsExtra() && child.Type(lang) == "identification_division" {
			setNodeStartTo(child, defStart)
			break
		}
	}
}

func cobolLeadingStarts(source []byte) (uint32, uint32, bool) {
	start := firstNonWhitespaceByte(source)
	if start != 0 {
		return start, start, true
	}
	if len(source) < 7 {
		return 0, 0, false
	}
	switch source[6] {
	case ' ':
		contentStart, ok := firstNonWhitespaceByteFrom(source, 7)
		if !ok {
			return 0, 0, false
		}
		return 6, contentStart, true
	case '*', '-', '/':
		return 6, 6, true
	default:
		return 0, 0, false
	}
}

func firstNonWhitespaceByteFrom(source []byte, start int) (uint32, bool) {
	if start < 0 {
		start = 0
	}
	for i := start; i < len(source); i++ {
		switch source[i] {
		case ' ', '\t', '\n', '\r':
			continue
		default:
			return uint32(i), true
		}
	}
	return 0, false
}

func normalizeCobolTopLevelDefinitionEnd(root *Node, source []byte, lang *Language) {
	if root == nil || !isCobolLanguage(lang) || root.Type(lang) != "start" || resultChildCount(root) == 0 {
		return
	}
	def := (*Node)(nil)
	for i := 0; i < resultChildCount(root); i++ {
		child := resultChildAt(root, i)
		if child != nil && !child.IsExtra() && child.Type(lang) == "program_definition" {
			def = child
			break
		}
	}
	if def == nil {
		return
	}
	if idDiv := cobolSoleIdentificationDivision(def, lang); idDiv != nil {
		end := cobolProgramIDStatementEnd(source, def.startByte, def.endByte)
		if end != 0 && end < def.endByte {
			setCobolNodeEnd(def, source, end)
			if end < idDiv.endByte {
				setCobolNodeEnd(idDiv, source, end)
			}
			return
		}
	}
	end := lastNonTriviaByteEnd(source)
	if end == 0 || end >= def.endByte {
		return
	}
	setCobolNodeEnd(def, source, end)
}

func cobolSoleIdentificationDivision(def *Node, lang *Language) *Node {
	var idDiv *Node
	for i := 0; i < resultChildCount(def); i++ {
		child := resultChildAt(def, i)
		if child == nil || child.IsExtra() {
			continue
		}
		if child.Type(lang) != "identification_division" {
			return nil
		}
		if idDiv != nil {
			return nil
		}
		idDiv = child
	}
	return idDiv
}

func cobolProgramIDStatementEnd(source []byte, start, end uint32) uint32 {
	if start >= end || int(end) > len(source) {
		return 0
	}
	idx := cobolIndexFoldASCII(source, int(start), int(end), "program-id.")
	if idx < 0 {
		return 0
	}
	for i := idx + len("program-id."); i < int(end); i++ {
		switch source[i] {
		case '.':
			return uint32(i + 1)
		case '\n', '\r':
			return 0
		}
	}
	return 0
}

func cobolIndexFoldASCII(source []byte, start, end int, needle string) int {
	if start < 0 {
		start = 0
	}
	if end > len(source) {
		end = len(source)
	}
	if len(needle) == 0 || start >= end || end-start < len(needle) {
		return -1
	}
	for i := start; i+len(needle) <= end; i++ {
		if cobolEqualFoldASCII(source[i:i+len(needle)], needle) {
			return i
		}
	}
	return -1
}

func cobolEqualFoldASCII(b []byte, s string) bool {
	if len(b) != len(s) {
		return false
	}
	for i := range b {
		if asciiLower(b[i]) != asciiLower(s[i]) {
			return false
		}
	}
	return true
}

func setCobolNodeEnd(n *Node, source []byte, end uint32) {
	if n == nil || n.endByte == end {
		return
	}
	n.endByte = end
	n.endPoint = advancePointByBytes(Point{}, source[:end])
}

func normalizeCobolDivisionSiblingEnds(root *Node, source []byte, lang *Language) {
	if root == nil || !isCobolLanguage(lang) || root.Type(lang) != "start" || resultChildCount(root) == 0 {
		return
	}
	def := (*Node)(nil)
	for i := 0; i < resultChildCount(root); i++ {
		child := resultChildAt(root, i)
		if child != nil && !child.IsExtra() && child.Type(lang) == "program_definition" {
			def = child
			break
		}
	}
	if def == nil {
		return
	}
	childCount := resultChildCount(def)
	for i := 0; i+1 < childCount; i++ {
		cur := resultChildAt(def, i)
		next := resultChildAt(def, i+1)
		if cur == nil || next == nil || cur.IsExtra() || next.IsExtra() {
			continue
		}
		if !strings.HasSuffix(cur.Type(lang), "_division") {
			continue
		}
		end := lastNonTriviaByteEnd(source[:next.startByte])
		if end == 0 || end <= cur.startByte || end >= cur.endByte {
			continue
		}
		cur.endByte = end
		cur.endPoint = advancePointByBytes(Point{}, source[:end])
	}
}

func normalizeCobolRecoveredParagraphHeader(root *Node, source []byte, lang *Language) {
	if root == nil || !isCobolLanguage(lang) || !lang.GeneratedByGrammargen || root.Type(lang) != "start" || !root.hasError() {
		return
	}
	rootChildren := resultChildSliceForMutation(root)
	if len(rootChildren) < 2 {
		return
	}
	errNode := rootChildren[len(rootChildren)-1]
	if errNode == nil || errNode.symbol != errorSymbol || !errNode.hasError() || errNode.startByte >= errNode.endByte || int(errNode.endByte) > len(source) {
		return
	}
	dot := cobolTrailingParagraphErrorDot(errNode, lang)
	if dot == nil || dot.endByte != errNode.endByte || dot.startByte <= errNode.startByte || int(dot.endByte) > len(source) {
		return
	}
	if !cobolBytesAreParagraphLabel(source[errNode.startByte:dot.startByte]) {
		return
	}
	program := cobolFirstProgramDefinition(root, lang)
	if program == nil {
		return
	}
	procedure := cobolFirstChildOfType(program, lang, "procedure_division")
	if procedure == nil {
		return
	}
	paragraphSym, ok := symbolByName(lang, "paragraph_header")
	if !ok {
		return
	}

	rootStart, rootStartPoint := root.startByte, root.startPoint
	rootEnd, rootEndPoint := root.endByte, root.endPoint
	procStart, procStartPoint := procedure.startByte, procedure.startPoint

	cobolClearErrorFlags(dot)
	header := newParentNodeInArena(root.ownerArena, paragraphSym, symbolIsNamed(lang, paragraphSym), []*Node{dot}, nil, 0)
	header.startByte = errNode.startByte
	header.startPoint = errNode.startPoint
	header.endByte = errNode.endByte
	header.endPoint = errNode.endPoint
	header.setHasError(false)

	procChildren := append(resultChildSliceForMutation(procedure), header)
	replaceNodeChildrenUnfielded(procedure, cloneNodeSliceInArena(procedure.ownerArena, procChildren))
	procedure.startByte = procStart
	procedure.startPoint = procStartPoint
	procedure.endByte = rootEnd
	procedure.endPoint = rootEndPoint
	cobolRefreshHasErrorFromChildren(procedure)

	keptRootChildren := cloneNodeSliceInArena(root.ownerArena, rootChildren[:len(rootChildren)-1])
	replaceNodeChildrenUnfielded(root, keptRootChildren)
	root.startByte = rootStart
	root.startPoint = rootStartPoint
	root.endByte = rootEnd
	root.endPoint = rootEndPoint
	cobolRefreshHasErrorFromChildren(root)
}

func cobolFirstProgramDefinition(root *Node, lang *Language) *Node {
	for i := 0; i < resultChildCount(root); i++ {
		child := resultChildAt(root, i)
		if child != nil && !child.IsExtra() && child.Type(lang) == "program_definition" {
			return child
		}
	}
	return nil
}

func cobolFirstChildOfType(parent *Node, lang *Language, typ string) *Node {
	for i := 0; i < resultChildCount(parent); i++ {
		child := resultChildAt(parent, i)
		if child != nil && !child.IsExtra() && child.Type(lang) == typ {
			return child
		}
	}
	return nil
}

func cobolTrailingParagraphErrorDot(errNode *Node, lang *Language) *Node {
	for i := resultChildCount(errNode) - 1; i >= 0; i-- {
		child := resultChildAt(errNode, i)
		if child != nil && child.Type(lang) == "." {
			return child
		}
	}
	return nil
}

func cobolBytesAreParagraphLabel(b []byte) bool {
	seen := false
	for _, c := range b {
		switch {
		case c == ' ' || c == '\t':
			if seen {
				return false
			}
		case c >= 'a' && c <= 'z':
			seen = true
		case c >= 'A' && c <= 'Z':
			seen = true
		case c >= '0' && c <= '9':
			seen = true
		case c == '-' || c == '_':
			seen = true
		default:
			return false
		}
	}
	return seen
}

func cobolClearErrorFlags(n *Node) {
	if n == nil {
		return
	}
	n.setHasError(false)
	for i := 0; i < resultChildCount(n); i++ {
		cobolClearErrorFlags(resultChildAt(n, i))
	}
}

func cobolRefreshHasErrorFromChildren(n *Node) {
	if n == nil {
		return
	}
	n.setHasError(false)
	for i := 0; i < resultChildCount(n); i++ {
		child := resultChildAt(n, i)
		if child != nil && (child.IsError() || child.HasError()) {
			n.setHasError(true)
			return
		}
	}
}
