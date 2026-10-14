package gotreesitter

import "bytes"

func normalizeScalaTopLevelClassFragments(root *Node, source []byte, parser *Parser, lang *Language) {
	if root == nil || lang == nil || lang.Name != "scala" || root.Type(lang) != "ERROR" || len(root.children) == 0 || len(source) == 0 {
		return
	}
	for _, child := range root.children {
		if child != nil && child.Type(lang) == "class_definition" {
			return
		}
	}
	lastObjectEnd := uint32(0)
	for _, child := range root.children {
		if child != nil && child.Type(lang) == "object_definition" && child.endByte > lastObjectEnd {
			lastObjectEnd = child.endByte
		}
	}
	if lastObjectEnd == 0 || int(lastObjectEnd) >= len(source) {
		return
	}
	classStartRel := bytes.Index(source[lastObjectEnd:], []byte("\nfinal class "))
	if classStartRel < 0 {
		classStartRel = bytes.Index(source[lastObjectEnd:], []byte("\nclass "))
		if classStartRel < 0 {
			return
		}
	}
	classStart := int(lastObjectEnd) + classStartRel + 1
	classNode, ok := scalaRecoverTopLevelClassNode(source, uint32(classStart), parser, lang, root.ownerArena)
	if !ok || classNode == nil {
		return
	}
	startIdx := len(root.children)
	for i, child := range root.children {
		if child != nil && child.startByte >= uint32(classStart) {
			startIdx = i
			break
		}
	}
	if startIdx >= len(root.children) {
		return
	}
	replaceChildRangeWithSingleNode(root, startIdx, len(root.children), classNode)
	populateParentNode(root, root.children)
}

func scalaObjectNeedsTemplateBody(node *Node, lang *Language) bool {
	if node == nil || lang == nil || node.Type(lang) != "object_definition" || len(node.children) != 2 {
		return false
	}
	return node.children[0] != nil && node.children[0].Type(lang) == "object" &&
		node.children[1] != nil && node.children[1].Type(lang) == "identifier"
}

func scalaSingleTokenError(node *Node, token string, lang *Language) bool {
	return scalaErrorTokenNode(node, token, lang) != nil
}

func scalaErrorTokenNode(node *Node, token string, lang *Language) *Node {
	if node == nil || lang == nil || node.Type(lang) != "ERROR" || len(node.children) != 1 || node.children[0] == nil {
		return nil
	}
	if node.children[0].Type(lang) == token {
		return node.children[0]
	}
	return nil
}

func scalaFindTemplateBodyClose(nodes []*Node, start int, lang *Language) int {
	for i := start; i < len(nodes); i++ {
		if scalaSingleTokenError(nodes[i], "}", lang) {
			return i
		}
	}
	return -1
}

func scalaFindTemplateBodyCloseByByte(nodes []*Node, start int, closeByte uint32) int {
	last := -1
	for i := start; i < len(nodes); i++ {
		n := nodes[i]
		if n == nil {
			continue
		}
		if n.startByte >= closeByte {
			break
		}
		last = i
		if n.endByte >= closeByte {
			return i
		}
	}
	return last
}

func scalaTemplateBodyFragmentChildren(nodes []*Node, arena *nodeArena, lang *Language, source []byte, closeByte uint32, synthClose bool) ([]*Node, bool) {
	out := make([]*Node, 0, len(nodes))
	var appendNode func(*Node)
	appendNode = func(n *Node) {
		if n == nil {
			return
		}
		switch n.Type(lang) {
		case "_indent", "_outdent":
			return
		case "_block_repeat1":
			for _, child := range n.children {
				appendNode(child)
			}
			return
		case "ERROR":
			if len(n.children) == 1 && n.children[0] != nil {
				switch n.children[0].Type(lang) {
				case "{", "}":
					out = append(out, n.children[0])
					return
				}
			}
		}
		out = append(out, n)
	}
	for _, node := range nodes {
		appendNode(node)
	}
	if len(out) == 0 || out[0] == nil || out[0].Type(lang) != "{" {
		return nil, false
	}
	if synthClose && (len(out) == 1 || out[len(out)-1] == nil || out[len(out)-1].Type(lang) != "}") {
		closeSym, ok := symbolByName(lang, "}")
		if !ok {
			return nil, false
		}
		closeNamed := symbolIsNamed(lang, closeSym)
		start := closeByte - 1
		if int(closeByte) > len(source) || start >= closeByte {
			return nil, false
		}
		close := newLeafNodeInArena(
			arena,
			closeSym,
			closeNamed,
			start,
			closeByte,
			advancePointByBytes(Point{}, source[:start]),
			advancePointByBytes(Point{}, source[:closeByte]),
		)
		out = append(out, close)
	}
	if len(out) < 2 || out[len(out)-1] == nil || out[len(out)-1].Type(lang) != "}" {
		return nil, false
	}
	if arena != nil {
		buf := arena.allocNodeSlice(len(out))
		copy(buf, out)
		out = buf
	}
	return out, true
}

func scalaRecoverTopLevelClassNode(source []byte, classStart uint32, parser *Parser, lang *Language, arena *nodeArena) (*Node, bool) {
	if lang == nil || int(classStart) >= len(source) {
		return nil, false
	}
	openRel := bytes.IndexByte(source[classStart:], '{')
	if openRel < 0 {
		return nil, false
	}
	openBrace := int(classStart) + openRel
	closeBrace := findMatchingBraceByte(source, openBrace, len(source))
	if closeBrace < 0 || closeBrace <= openBrace {
		return nil, false
	}
	return scalaRecoverTopLevelClassNodeFromRange(source, classStart, uint32(closeBrace+1), parser, lang, arena)
}

func scalaRecoverTopLevelClassNodeFromRange(source []byte, classStart, classEnd uint32, parser *Parser, lang *Language, arena *nodeArena) (*Node, bool) {
	if lang == nil || int(classStart) >= len(source) || classEnd <= classStart || int(classEnd) > len(source) {
		return nil, false
	}
	offsetRoot, ok := parseSnippetRootWithOffset(lang, source, classStart, classEnd, parser)
	if !ok {
		return nil, false
	}
	if recovered, ok := scalaCloneCompleteClassDefinition(offsetRoot, source, classEnd, lang, arena); ok {
		return recovered, true
	}
	symbols, ok := scalaClassRecoverySymbolsFor(lang)
	if !ok {
		return nil, false
	}
	indexes := scalaFindClassRecoveryIndexes(offsetRoot, lang)
	if recovered, ok := scalaBuildClassFromHeaderFragment(offsetRoot, indexes, symbols, source, classEnd, lang, arena); ok {
		return recovered, true
	}
	return scalaBuildClassFromTokenFragments(offsetRoot, indexes, symbols, source, classEnd, lang, arena)
}

type scalaClassRecoverySymbols struct {
	classSym          Symbol
	classNamed        bool
	templateBodySym   Symbol
	templateBodyNamed bool
}

type scalaClassRecoveryIndexes struct {
	headerIdx      int
	classIdx       int
	constructorIdx int
	openIdx        int
	extendsIdx     int
}

func scalaClassRecoverySymbolsFor(lang *Language) (scalaClassRecoverySymbols, bool) {
	classSym, ok := symbolByName(lang, "class_definition")
	if !ok {
		return scalaClassRecoverySymbols{}, false
	}
	templateBodySym, ok := symbolByName(lang, "template_body")
	if !ok {
		return scalaClassRecoverySymbols{}, false
	}
	return scalaClassRecoverySymbols{
		classSym:          classSym,
		classNamed:        symbolIsNamed(lang, classSym),
		templateBodySym:   templateBodySym,
		templateBodyNamed: symbolIsNamed(lang, templateBodySym),
	}, true
}

func scalaNewClassRecoveryIndexes() scalaClassRecoveryIndexes {
	return scalaClassRecoveryIndexes{
		headerIdx:      -1,
		classIdx:       -1,
		constructorIdx: -1,
		openIdx:        -1,
		extendsIdx:     -1,
	}
}

func scalaCloneCompleteClassDefinition(offsetRoot *Node, source []byte, classEnd uint32, lang *Language, arena *nodeArena) (*Node, bool) {
	for i := 0; i < offsetRoot.ChildCount(); i++ {
		child := offsetRoot.Child(i)
		if child == nil || child.Type(lang) != "class_definition" || child.HasError() {
			continue
		}
		recovered := cloneTreeNodesIntoArena(child, arena)
		if recovered.endByte < classEnd && bytesAreTrivia(source[recovered.endByte:classEnd]) {
			extendNodeEndTo(recovered, classEnd, source)
		}
		if recovered.endByte == classEnd {
			return recovered, true
		}
	}
	return nil, false
}

func scalaFindClassRecoveryIndexes(offsetRoot *Node, lang *Language) scalaClassRecoveryIndexes {
	indexes := scalaNewClassRecoveryIndexes()
	for i := 0; i < offsetRoot.ChildCount(); i++ {
		child := offsetRoot.Child(i)
		if child == nil {
			continue
		}
		switch child.Type(lang) {
		case "class_definition":
			if indexes.headerIdx < 0 {
				indexes.headerIdx = i
			}
		case "class":
			indexes.classIdx = i
		case "_class_constructor":
			if indexes.classIdx >= 0 && indexes.constructorIdx < 0 {
				indexes.constructorIdx = i
			}
		case "extends_clause":
			if indexes.constructorIdx >= 0 && indexes.extendsIdx < 0 {
				indexes.extendsIdx = i
			}
		case "{":
			if indexes.constructorIdx >= 0 || indexes.headerIdx >= 0 {
				indexes.openIdx = i
			}
		}
		if indexes.openIdx < 0 && indexes.headerIdx >= 0 {
			if brace := scalaErrorTokenNode(child, "{", lang); brace != nil {
				indexes.openIdx = i
			}
		}
		if indexes.openIdx >= 0 {
			break
		}
	}
	return indexes
}

func scalaBuildClassFromHeaderFragment(offsetRoot *Node, indexes scalaClassRecoveryIndexes, symbols scalaClassRecoverySymbols, source []byte, classEnd uint32, lang *Language, arena *nodeArena) (*Node, bool) {
	if indexes.headerIdx < 0 || indexes.openIdx < 0 {
		return nil, false
	}
	header := offsetRoot.Child(indexes.headerIdx)
	if header == nil {
		return nil, false
	}
	closeIdx := scalaTemplateBodyCloseIndex(offsetRoot.children, indexes.openIdx, classEnd)
	bodyChildren, ok := scalaTemplateBodyFragmentChildren(offsetRoot.children[indexes.openIdx:closeIdx+1], arena, lang, source, classEnd, true)
	if !ok {
		return nil, false
	}
	templateBody := newParentNodeInArena(arena, symbols.templateBodySym, symbols.templateBodyNamed, bodyChildren, nil, 0)
	children := make([]*Node, 0, len(header.children)+1)
	for _, child := range header.children {
		if child == nil {
			continue
		}
		children = append(children, cloneTreeNodesIntoArena(child, arena))
	}
	children = append(children, templateBody)
	children = scalaNodesInArena(children, arena)
	recovered := newParentNodeInArena(arena, symbols.classSym, symbols.classNamed, children, nil, header.productionID)
	extendScalaRecoveredNodeEnd(recovered, classEnd, source)
	return recovered, true
}

func scalaBuildClassFromTokenFragments(offsetRoot *Node, indexes scalaClassRecoveryIndexes, symbols scalaClassRecoverySymbols, source []byte, classEnd uint32, lang *Language, arena *nodeArena) (*Node, bool) {
	if indexes.classIdx < 0 || indexes.constructorIdx < 0 || indexes.openIdx < 0 {
		return nil, false
	}
	constructor := offsetRoot.Child(indexes.constructorIdx)
	if constructor == nil || constructor.ChildCount() < 2 {
		return nil, false
	}
	nameNode := constructor.Child(0)
	paramsNode := constructor.Child(1)
	if nameNode == nil || paramsNode == nil || nameNode.Type(lang) != "identifier" || paramsNode.Type(lang) != "class_parameters" {
		return nil, false
	}
	closeByte := classEnd
	closeIdx, synthClose := scalaTemplateBodyCloseIndexAndSynth(offsetRoot.children, indexes.openIdx, closeByte, lang)
	bodyChildren, ok := scalaTemplateBodyFragmentChildren(offsetRoot.children[indexes.openIdx:closeIdx+1], arena, lang, source, closeByte, synthClose)
	if !ok {
		return nil, false
	}
	templateBody := newParentNodeInArena(arena, symbols.templateBodySym, symbols.templateBodyNamed, bodyChildren, nil, 0)
	children := make([]*Node, 0, 6)
	if indexes.classIdx > 0 {
		if modifiers := offsetRoot.Child(indexes.classIdx - 1); modifiers != nil && modifiers.Type(lang) == "modifiers" {
			children = append(children, cloneTreeNodesIntoArena(modifiers, arena))
		}
	}
	children = append(children, cloneTreeNodesIntoArena(offsetRoot.Child(indexes.classIdx), arena))
	children = append(children, cloneTreeNodesIntoArena(nameNode, arena))
	children = append(children, cloneTreeNodesIntoArena(paramsNode, arena))
	if indexes.extendsIdx >= 0 {
		if extendsClause := offsetRoot.Child(indexes.extendsIdx); extendsClause != nil && extendsClause.Type(lang) == "extends_clause" {
			children = append(children, cloneTreeNodesIntoArena(extendsClause, arena))
		}
	}
	children = append(children, templateBody)
	children = scalaNodesInArena(children, arena)
	recovered := newParentNodeInArena(arena, symbols.classSym, symbols.classNamed, children, nil, 0)
	extendScalaRecoveredNodeEnd(recovered, classEnd, source)
	return recovered, true
}

func scalaTemplateBodyCloseIndex(nodes []*Node, openIdx int, closeByte uint32) int {
	closeIdx := scalaFindTemplateBodyCloseByByte(nodes, openIdx+1, closeByte)
	if closeIdx < openIdx {
		return len(nodes) - 1
	}
	return closeIdx
}

func scalaTemplateBodyCloseIndexAndSynth(nodes []*Node, openIdx int, closeByte uint32, lang *Language) (int, bool) {
	closeIdx := scalaTemplateBodyCloseIndex(nodes, openIdx, closeByte)
	synthClose := true
	if closeIdx >= 0 && closeIdx < len(nodes) {
		if closeNode := scalaErrorTokenNode(nodes[closeIdx], "}", lang); closeNode != nil && closeNode.endByte == closeByte {
			synthClose = false
		} else if nodes[closeIdx] != nil && nodes[closeIdx].Type(lang) == "}" && nodes[closeIdx].endByte == closeByte {
			synthClose = false
		}
	}
	return closeIdx, synthClose
}

func scalaNodesInArena(children []*Node, arena *nodeArena) []*Node {
	if arena != nil {
		buf := arena.allocNodeSlice(len(children))
		copy(buf, children)
		return buf
	}
	return children
}

func extendScalaRecoveredNodeEnd(recovered *Node, classEnd uint32, source []byte) {
	if recovered != nil && recovered.endByte < classEnd && bytesAreTrivia(source[recovered.endByte:classEnd]) {
		extendNodeEndTo(recovered, classEnd, source)
	}
}

func scalaRecoverTopLevelObjectNodeFromRange(source []byte, objectStart, objectEnd uint32, parser *Parser, lang *Language, arena *nodeArena) (*Node, bool) {
	if lang == nil || int(objectStart) >= len(source) || objectEnd <= objectStart || int(objectEnd) > len(source) {
		return nil, false
	}
	offsetRoot, ok := parseSnippetRootWithOffset(lang, source, objectStart, objectEnd, parser)
	if !ok {
		return nil, false
	}
	for i := 0; i < offsetRoot.ChildCount(); i++ {
		child := offsetRoot.Child(i)
		if child == nil || child.Type(lang) != "object_definition" || child.HasError() {
			continue
		}
		recovered := cloneTreeNodesIntoArena(child, arena)
		if recovered.endByte < objectEnd && bytesAreTrivia(source[recovered.endByte:objectEnd]) {
			extendNodeEndTo(recovered, objectEnd, source)
		}
		if recovered.endByte == objectEnd {
			return recovered, true
		}
	}
	objectSym, ok := symbolByName(lang, "object_definition")
	if !ok {
		return nil, false
	}
	objectNamed := symbolIsNamed(lang, objectSym)
	templateBodySym, ok := symbolByName(lang, "template_body")
	if !ok {
		return nil, false
	}
	templateBodyNamed := symbolIsNamed(lang, templateBodySym)
	objectIdx := -1
	identifierIdx := -1
	openIdx := -1
	for i := 0; i < offsetRoot.ChildCount(); i++ {
		child := offsetRoot.Child(i)
		if child == nil {
			continue
		}
		switch child.Type(lang) {
		case "object":
			if objectIdx < 0 {
				objectIdx = i
			}
		case "identifier":
			if objectIdx >= 0 && identifierIdx < 0 {
				identifierIdx = i
			}
		case "{":
			if identifierIdx >= 0 {
				openIdx = i
				i = offsetRoot.ChildCount()
			}
		}
	}
	if objectIdx < 0 || identifierIdx < 0 || openIdx < 0 {
		return nil, false
	}
	closeIdx := scalaFindTemplateBodyCloseByByte(offsetRoot.children, openIdx+1, objectEnd)
	if closeIdx < openIdx {
		closeIdx = len(offsetRoot.children) - 1
	}
	synthClose := true
	if closeIdx >= 0 && closeIdx < len(offsetRoot.children) {
		if closeNode := scalaErrorTokenNode(offsetRoot.children[closeIdx], "}", lang); closeNode != nil && closeNode.endByte == objectEnd {
			synthClose = false
		} else if offsetRoot.children[closeIdx] != nil && offsetRoot.children[closeIdx].Type(lang) == "}" && offsetRoot.children[closeIdx].endByte == objectEnd {
			synthClose = false
		}
	}
	bodyChildren, ok := scalaTemplateBodyFragmentChildren(offsetRoot.children[openIdx:closeIdx+1], arena, lang, source, objectEnd, synthClose)
	if !ok {
		return nil, false
	}
	templateBody := newParentNodeInArena(arena, templateBodySym, templateBodyNamed, bodyChildren, nil, 0)
	children := []*Node{
		cloneTreeNodesIntoArena(offsetRoot.Child(objectIdx), arena),
		cloneTreeNodesIntoArena(offsetRoot.Child(identifierIdx), arena),
		templateBody,
	}
	if arena != nil {
		buf := arena.allocNodeSlice(len(children))
		copy(buf, children)
		children = buf
	}
	recovered := newParentNodeInArena(arena, objectSym, objectNamed, children, nil, 0)
	if recovered.endByte < objectEnd && bytesAreTrivia(source[recovered.endByte:objectEnd]) {
		extendNodeEndTo(recovered, objectEnd, source)
	}
	return recovered, true
}

func scalaRecoverTopLevelNamedNodeFromRange(source []byte, start, end uint32, parser *Parser, lang *Language, arena *nodeArena, want string) (*Node, bool) {
	if lang == nil || int(start) >= len(source) || end <= start || int(end) > len(source) {
		return nil, false
	}
	offsetRoot, ok := parseSnippetRootWithOffset(lang, source, start, end, parser)
	if !ok {
		return nil, false
	}
	for i := 0; i < offsetRoot.ChildCount(); i++ {
		child := offsetRoot.Child(i)
		if child == nil || child.Type(lang) != want || child.HasError() {
			continue
		}
		recovered := cloneTreeNodesIntoArena(child, arena)
		if recovered.endByte < end && bytesAreTrivia(source[recovered.endByte:end]) {
			extendNodeEndTo(recovered, end, source)
		}
		if recovered.endByte == end {
			return recovered, true
		}
	}
	return nil, false
}

func scalaRecoverTopLevelFunctionNodeFromRange(source []byte, fnStart, fnEnd uint32, parser *Parser, lang *Language, arena *nodeArena) (*Node, bool) {
	if lang == nil || int(fnStart) >= len(source) || fnEnd <= fnStart || int(fnEnd) > len(source) {
		return nil, false
	}
	offsetRoot, ok := parseSnippetRootWithOffset(lang, source, fnStart, fnEnd, parser)
	if !ok {
		return nil, false
	}
	for i := 0; i < offsetRoot.ChildCount(); i++ {
		child := offsetRoot.Child(i)
		if child == nil || child.Type(lang) != "function_definition" {
			continue
		}
		recovered := cloneTreeNodesIntoArena(child, arena)
		if recovered.endByte < fnEnd && bytesAreTrivia(source[recovered.endByte:fnEnd]) {
			extendNodeEndTo(recovered, fnEnd, source)
		}
		return recovered, true
	}
	return nil, false
}
