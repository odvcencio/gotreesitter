package gotreesitter

func csharpRecoverAttributedTopLevelTypeDeclarationFromChildren(children []*Node, startIdx int, source []byte, parser *Parser, lang *Language, arena *nodeArena) (*Node, int, bool) {
	if startIdx < 0 || startIdx >= len(children) || lang == nil || arena == nil || len(source) == 0 {
		return nil, startIdx, false
	}
	startNode := children[startIdx]
	if startNode == nil || int(startNode.startByte) >= len(source) {
		return nil, startIdx, false
	}
	start := csharpSkipSpaceBytes(source, startNode.startByte)
	if start >= uint32(len(source)) || source[start] != '[' {
		return nil, startIdx, false
	}
	attributeLists, declStart, ok := csharpBuildLeadingAttributeListsFromSource(source, start, uint32(len(source)), lang, arena)
	if !ok || len(attributeLists) == 0 {
		return nil, startIdx, false
	}
	spans := csharpTopLevelChunkSpans(source[declStart:])
	if len(spans) == 0 {
		return nil, startIdx, false
	}
	declEnd := declStart + spans[0][1]
	recovered, ok := csharpRecoverAttributedTopLevelTypeDeclarationFromRange(source, declStart, declEnd, attributeLists, parser, lang, arena)
	if !ok {
		return nil, startIdx, false
	}
	nextIdx := startIdx + 1
	for nextIdx < len(children) {
		child := children[nextIdx]
		if child == nil {
			nextIdx++
			continue
		}
		if child.startByte >= declEnd {
			break
		}
		nextIdx++
	}
	return recovered, nextIdx, true
}

func csharpRecoverAttributedTopLevelTypeDeclarationFromError(n *Node, source []byte, parser *Parser, lang *Language, arena *nodeArena) (*Node, bool) {
	if n == nil || lang == nil || arena == nil || n.Type(lang) != "ERROR" || len(source) == 0 {
		return nil, false
	}
	start, end := csharpTrimSpaceBounds(source, n.startByte, n.endByte)
	if start >= end || source[start] != '[' {
		return nil, false
	}
	attributeLists, declStart, ok := csharpBuildLeadingAttributeListsFromSource(source, start, end, lang, arena)
	if !ok || len(attributeLists) == 0 || declStart >= end {
		return nil, false
	}
	return csharpRecoverAttributedTopLevelTypeDeclarationFromRange(source, declStart, end, attributeLists, parser, lang, arena)
}

func csharpRecoverAttributedTopLevelTypeDeclarationFromRange(source []byte, declStart, declEnd uint32, attributeLists []*Node, parser *Parser, lang *Language, arena *nodeArena) (*Node, bool) {
	if lang == nil || arena == nil || len(attributeLists) == 0 || declStart >= declEnd || int(declEnd) > len(source) {
		return nil, false
	}
	tree, err := parseWithSnippetParserInheriting(lang, source[declStart:declEnd], parser)
	if err != nil || tree == nil || tree.RootNode() == nil {
		if tree != nil {
			tree.Release()
		}
		return nil, false
	}
	defer tree.Release()
	offsetRoot := tree.rootNodeWithRecoveryOffset(declStart, advancePointByBytes(Point{}, source[:declStart]))
	if offsetRoot == nil {
		return nil, false
	}
	if offsetRoot.HasError() {
		if recovered, ok := csharpRecoverEmptyTypeDeclarationFromError(offsetRoot, source, lang, arena); ok {
			return csharpPrependAttributeListsToDeclaration(recovered, attributeLists, arena), true
		}
		return nil, false
	}
	nodes := csharpExtractRecoveredTopLevelNodes(offsetRoot, lang, arena)
	if len(nodes) != 1 {
		return nil, false
	}
	switch nodes[0].Type(lang) {
	case "class_declaration", "struct_declaration", "record_declaration", "interface_declaration", "enum_declaration", "delegate_declaration":
	default:
		return nil, false
	}
	return csharpPrependAttributeListsToDeclaration(nodes[0], attributeLists, arena), true
}

func csharpBuildLeadingAttributeListsFromSource(source []byte, start, end uint32, lang *Language, arena *nodeArena) ([]*Node, uint32, bool) {
	cursor := csharpSkipSpaceBytes(source, start)
	lists := make([]*Node, 0, 2)
	for cursor < end && source[cursor] == '[' {
		closePos, ok := csharpFindMatchingBracketByte(source, cursor, end)
		if !ok {
			return nil, start, false
		}
		list, ok := csharpBuildAttributeListNodeFromSource(source, cursor, closePos+1, lang, arena)
		if !ok {
			return nil, start, false
		}
		lists = append(lists, list)
		cursor = csharpSkipSpaceBytes(source, closePos+1)
	}
	if len(lists) == 0 {
		return nil, start, false
	}
	if arena != nil {
		buf := arena.allocNodeSlice(len(lists))
		copy(buf, lists)
		lists = buf
	}
	return lists, cursor, true
}

func csharpBuildAttributeListNodeFromSource(source []byte, start, end uint32, lang *Language, arena *nodeArena) (*Node, bool) {
	if lang == nil || arena == nil || start >= end || end-start < 2 || source[start] != '[' || source[end-1] != ']' {
		return nil, false
	}
	attrListSym, ok := symbolByName(lang, "attribute_list")
	if !ok {
		return nil, false
	}
	attrListNamed := symbolIsNamed(lang, attrListSym)
	openTok, ok := csharpBuildLeafNodeByName(arena, source, lang, "[", start, start+1)
	if !ok {
		return nil, false
	}
	closeTok, ok := csharpBuildLeafNodeByName(arena, source, lang, "]", end-1, end)
	if !ok {
		return nil, false
	}
	contentStart, contentEnd := csharpTrimSpaceBounds(source, start+1, end-1)
	if contentStart > contentEnd {
		return nil, false
	}
	var targetSpec *Node
	if target, attrStart, ok := csharpBuildAttributeTargetSpecifierFromSource(source, contentStart, contentEnd, lang, arena); ok {
		targetSpec = target
		contentStart = attrStart
	}
	itemSpans := csharpSplitTopLevelByComma(source, contentStart, contentEnd)
	if len(itemSpans) == 0 {
		return nil, false
	}
	commaPos := make([]uint32, 0, len(itemSpans)-1)
	cursor := contentStart
	for i, span := range itemSpans {
		if i == 0 {
			cursor = span[1]
			continue
		}
		pos, ok := csharpFindTopLevelOperator(source, cursor, span[0], ",")
		if !ok {
			return nil, false
		}
		commaPos = append(commaPos, pos)
		cursor = span[1]
	}
	children := make([]*Node, 0, len(itemSpans)*2+3)
	children = append(children, openTok)
	if targetSpec != nil {
		children = append(children, targetSpec)
	}
	for i, span := range itemSpans {
		itemStart, itemEnd := csharpTrimSpaceBounds(source, span[0], span[1])
		attr, ok := csharpBuildAttributeNodeFromSource(source, itemStart, itemEnd, lang, arena)
		if !ok {
			return nil, false
		}
		children = append(children, attr)
		if i < len(commaPos) {
			commaTok, ok := csharpBuildLeafNodeByName(arena, source, lang, ",", commaPos[i], commaPos[i]+1)
			if !ok {
				return nil, false
			}
			children = append(children, commaTok)
		}
	}
	children = append(children, closeTok)
	if arena != nil {
		buf := arena.allocNodeSlice(len(children))
		copy(buf, children)
		children = buf
	}
	return newParentNodeInArena(arena, attrListSym, attrListNamed, children, nil, 0), true
}

func csharpBuildAttributeTargetSpecifierFromSource(source []byte, start, end uint32, lang *Language, arena *nodeArena) (*Node, uint32, bool) {
	colonPos, ok := csharpFindTopLevelOperator(source, start, end, ":")
	if !ok {
		return nil, start, false
	}
	targetStart, targetEnd := csharpTrimSpaceBounds(source, start, colonPos)
	if targetStart >= targetEnd {
		return nil, start, false
	}
	identStart, identEnd, ok := csharpScanIdentifierAt(source, targetStart)
	if !ok || identStart != targetStart || identEnd != targetEnd {
		return nil, start, false
	}
	sym, ok := symbolByName(lang, "attribute_target_specifier")
	if !ok {
		return nil, start, false
	}
	named := symbolIsNamed(lang, sym)
	target := newLeafNodeInArena(arena, sym, named, targetStart, colonPos+1, advancePointByBytes(Point{}, source[:targetStart]), advancePointByBytes(Point{}, source[:colonPos+1]))
	return target, csharpSkipSpaceBytes(source, colonPos+1), true
}

func csharpBuildAttributeNodeFromSource(source []byte, start, end uint32, lang *Language, arena *nodeArena) (*Node, bool) {
	if lang == nil || arena == nil {
		return nil, false
	}
	attrSym, ok := symbolByName(lang, "attribute")
	if !ok {
		return nil, false
	}
	attrNamed := symbolIsNamed(lang, attrSym)
	nameFieldID, _ := lang.FieldByName("name")
	nameStart, nameEnd := csharpTrimSpaceBounds(source, start, end)
	if nameStart >= nameEnd {
		return nil, false
	}
	var argList *Node
	if openPos, ok := csharpFindInvocationOpenParen(source, nameStart, nameEnd); ok {
		closePos, ok := csharpFindMatchingParenByte(source, openPos, nameEnd)
		if !ok || closePos+1 != nameEnd {
			return nil, false
		}
		argList, ok = csharpBuildAttributeArgumentListNodeFromSource(source, openPos, closePos+1, lang, arena)
		if !ok {
			return nil, false
		}
		nameEnd = csharpTrimRightSpaceBytes(source, openPos)
	}
	nameNode, ok := csharpBuildQualifiedNameNode(source, nameStart, nameEnd, lang, arena)
	if !ok {
		return nil, false
	}
	children := []*Node{nameNode}
	fieldIDs := []FieldID{nameFieldID}
	if argList != nil {
		children = append(children, argList)
		fieldIDs = append(fieldIDs, 0)
	}
	if arena != nil {
		buf := arena.allocNodeSlice(len(children))
		copy(buf, children)
		children = buf
	}
	fields := cloneFieldIDSliceInArena(arena, fieldIDs)
	return newParentNodeInArena(arena, attrSym, attrNamed, children, fields, 0), true
}

func csharpBuildAttributeArgumentListNodeFromSource(source []byte, start, end uint32, lang *Language, arena *nodeArena) (*Node, bool) {
	if lang == nil || arena == nil || start >= end || source[start] != '(' || source[end-1] != ')' {
		return nil, false
	}
	argListSym, ok := symbolByName(lang, "attribute_argument_list")
	if !ok {
		return nil, false
	}
	argListNamed := symbolIsNamed(lang, argListSym)
	openTok, ok := csharpBuildLeafNodeByName(arena, source, lang, "(", start, start+1)
	if !ok {
		return nil, false
	}
	closeTok, ok := csharpBuildLeafNodeByName(arena, source, lang, ")", end-1, end)
	if !ok {
		return nil, false
	}
	children := []*Node{openTok}
	contentStart, contentEnd := csharpTrimSpaceBounds(source, start+1, end-1)
	if contentStart < contentEnd {
		itemSpans := csharpSplitTopLevelByComma(source, contentStart, contentEnd)
		commaPos := make([]uint32, 0, len(itemSpans)-1)
		cursor := contentStart
		for i, span := range itemSpans {
			if i == 0 {
				cursor = span[1]
				continue
			}
			pos, ok := csharpFindTopLevelOperator(source, cursor, span[0], ",")
			if !ok {
				return nil, false
			}
			commaPos = append(commaPos, pos)
			cursor = span[1]
		}
		for i, span := range itemSpans {
			itemStart, itemEnd := csharpTrimSpaceBounds(source, span[0], span[1])
			arg, ok := csharpBuildAttributeArgumentNodeFromSource(source, itemStart, itemEnd, lang, arena)
			if !ok {
				return nil, false
			}
			children = append(children, arg)
			if i < len(commaPos) {
				commaTok, ok := csharpBuildLeafNodeByName(arena, source, lang, ",", commaPos[i], commaPos[i]+1)
				if !ok {
					return nil, false
				}
				children = append(children, commaTok)
			}
		}
	}
	children = append(children, closeTok)
	if arena != nil {
		buf := arena.allocNodeSlice(len(children))
		copy(buf, children)
		children = buf
	}
	return newParentNodeInArena(arena, argListSym, argListNamed, children, nil, 0), true
}

func csharpBuildAttributeArgumentNodeFromSource(source []byte, start, end uint32, lang *Language, arena *nodeArena) (*Node, bool) {
	if lang == nil || arena == nil {
		return nil, false
	}
	argSym, ok := symbolByName(lang, "attribute_argument")
	if !ok {
		return nil, false
	}
	argNamed := symbolIsNamed(lang, argSym)
	if opPos, opName, ok := csharpFindAttributeNamedArgumentOperator(source, start, end); ok {
		nameStart, nameEnd := csharpTrimSpaceBounds(source, start, opPos)
		valueStart, valueEnd := csharpTrimSpaceBounds(source, opPos+uint32(len(opName)), end)
		identStart, identEnd, ok := csharpScanIdentifierAt(source, nameStart)
		if !ok || identStart != nameStart || identEnd != nameEnd || valueStart >= valueEnd {
			return nil, false
		}
		nameNode, ok := csharpBuildIdentifierNodeFromSource(source, nameStart, nameEnd, lang, arena)
		if !ok {
			return nil, false
		}
		opTok, ok := csharpBuildLeafNodeByName(arena, source, lang, opName, opPos, opPos+uint32(len(opName)))
		if !ok {
			return nil, false
		}
		valueNode, ok := csharpBuildAttributeArgumentValueNode(source, valueStart, valueEnd, lang, arena)
		if !ok {
			return nil, false
		}
		nameFieldID, _ := lang.FieldByName("name")
		fields := cloneFieldIDSliceInArena(arena, []FieldID{nameFieldID, 0, 0})
		children := []*Node{nameNode, opTok, valueNode}
		if arena != nil {
			buf := arena.allocNodeSlice(len(children))
			copy(buf, children)
			children = buf
		}
		return newParentNodeInArena(arena, argSym, argNamed, children, fields, 0), true
	}
	valueNode, ok := csharpBuildAttributeArgumentValueNode(source, start, end, lang, arena)
	if !ok {
		return nil, false
	}
	return newParentNodeInArena(arena, argSym, argNamed, []*Node{valueNode}, nil, 0), true
}

func csharpFindAttributeNamedArgumentOperator(source []byte, start, end uint32) (uint32, string, bool) {
	colonPos, colonOK := csharpFindTopLevelOperator(source, start, end, ":")
	eqPos, eqOK := csharpFindTopLevelAssignment(source, start, end)
	switch {
	case colonOK && (!eqOK || colonPos < eqPos):
		return colonPos, ":", true
	case eqOK:
		return eqPos, "=", true
	default:
		return 0, "", false
	}
}

func csharpBuildAttributeArgumentValueNode(source []byte, start, end uint32, lang *Language, arena *nodeArena) (*Node, bool) {
	start, end = csharpTrimSpaceBounds(source, start, end)
	if start >= end {
		return nil, false
	}
	if source[start] == '"' && end-start >= 2 && source[end-1] == '"' {
		return csharpBuildStringLiteralNode(arena, source, lang, start, end)
	}
	if csharpIsDecimalLiteral(source[start:end]) {
		return csharpBuildLeafNodeByName(arena, source, lang, "integer_literal", start, end)
	}
	if string(source[start:end]) == "true" || string(source[start:end]) == "false" {
		return csharpBuildLeafNodeByName(arena, source, lang, "boolean_literal", start, end)
	}
	if node, ok := csharpBuildMemberAccessNodeFromSource(source, start, end, lang, arena); ok {
		return node, true
	}
	return csharpBuildIdentifierNodeFromSource(source, start, end, lang, arena)
}

func csharpBuildQualifiedNameNode(source []byte, start, end uint32, lang *Language, arena *nodeArena) (*Node, bool) {
	start, end = csharpTrimSpaceBounds(source, start, end)
	if start >= end || lang == nil || arena == nil {
		return nil, false
	}
	qualifierFieldID, _ := lang.FieldByName("qualifier")
	nameFieldID, _ := lang.FieldByName("name")
	qualifiedNameSym, ok := symbolByName(lang, "qualified_name")
	if !ok {
		return nil, false
	}
	qualifiedNameNamed := symbolIsNamed(lang, qualifiedNameSym)
	segStart, segEnd, ok := csharpScanIdentifierAt(source, start)
	if !ok || segStart != start {
		return nil, false
	}
	current, ok := csharpBuildIdentifierNodeFromSource(source, segStart, segEnd, lang, arena)
	if !ok {
		return nil, false
	}
	cursor := segEnd
	for {
		// Bounded: csharpSkipSpaceBytes skips whitespace against the whole
		// source, ignoring end, so if the caller already trimmed end to
		// exclude the trailing whitespace right after this segment (e.g. the
		// "\n" between a namespace name and its opening "{"), it would
		// otherwise walk past end into whatever non-whitespace content
		// follows and the cursor!=end check below would spuriously fail.
		if skipped := csharpSkipSpaceBytes(source, cursor); skipped <= end {
			cursor = skipped
		} else {
			cursor = end
		}
		if cursor >= end {
			break
		}
		if source[cursor] != '.' {
			return nil, false
		}
		dotTok, ok := csharpBuildLeafNodeByName(arena, source, lang, ".", cursor, cursor+1)
		if !ok {
			return nil, false
		}
		segStart, segEnd, ok = csharpScanIdentifierAt(source, csharpSkipSpaceBytes(source, cursor+1))
		if !ok {
			return nil, false
		}
		ident, ok := csharpBuildIdentifierNodeFromSource(source, segStart, segEnd, lang, arena)
		if !ok {
			return nil, false
		}
		fields := cloneFieldIDSliceInArena(arena, []FieldID{qualifierFieldID, 0, nameFieldID})
		current = newParentNodeInArena(arena, qualifiedNameSym, qualifiedNameNamed, []*Node{current, dotTok, ident}, fields, 0)
		cursor = segEnd
	}
	if cursor != end {
		return nil, false
	}
	return current, true
}

func csharpBuildMemberAccessNodeFromSource(source []byte, start, end uint32, lang *Language, arena *nodeArena) (*Node, bool) {
	start, end = csharpTrimSpaceBounds(source, start, end)
	if start >= end || lang == nil || arena == nil {
		return nil, false
	}
	expressionFieldID, hasExpressionField := lang.FieldByName("expression")
	nameFieldID, hasNameField := lang.FieldByName("name")
	memberAccessSym, hasMemberAccess := symbolByName(lang, "member_access_expression")
	if !hasExpressionField || !hasNameField || !hasMemberAccess {
		return nil, false
	}
	memberAccessNamed := symbolIsNamed(lang, memberAccessSym)
	leftStart, leftEnd, ok := csharpScanIdentifierAt(source, start)
	if !ok || leftStart != start {
		return nil, false
	}
	current, ok := csharpBuildIdentifierNodeFromSource(source, leftStart, leftEnd, lang, arena)
	if !ok {
		return nil, false
	}
	cursor := leftEnd
	sawDot := false
	for {
		cursor = csharpSkipSpaceBytes(source, cursor)
		if cursor >= end {
			break
		}
		if source[cursor] != '.' {
			return nil, false
		}
		sawDot = true
		dotTok, ok := csharpBuildLeafNodeByName(arena, source, lang, ".", cursor, cursor+1)
		if !ok {
			return nil, false
		}
		nameStart, nameEnd, ok := csharpScanIdentifierAt(source, csharpSkipSpaceBytes(source, cursor+1))
		if !ok {
			return nil, false
		}
		nameNode, ok := csharpBuildIdentifierNodeFromSource(source, nameStart, nameEnd, lang, arena)
		if !ok {
			return nil, false
		}
		fields := cloneFieldIDSliceInArena(arena, []FieldID{expressionFieldID, 0, nameFieldID})
		current = newParentNodeInArena(arena, memberAccessSym, memberAccessNamed, []*Node{current, dotTok, nameNode}, fields, 0)
		cursor = nameEnd
	}
	if !sawDot || cursor != end {
		return nil, false
	}
	return current, true
}

func csharpPrependAttributeListsToDeclaration(decl *Node, attributeLists []*Node, arena *nodeArena) *Node {
	if decl == nil || len(attributeLists) == 0 {
		return decl
	}
	children := make([]*Node, 0, len(attributeLists)+len(decl.children))
	children = append(children, attributeLists...)
	children = append(children, decl.children...)
	if arena != nil {
		buf := arena.allocNodeSlice(len(children))
		copy(buf, children)
		children = buf
	}
	decl.children = children
	declFieldIDs := decl.fieldIDs()
	if len(declFieldIDs) > 0 {
		fieldIDs := make([]FieldID, len(children))
		copy(fieldIDs[len(attributeLists):], declFieldIDs)
		fieldIDs = cloneFieldIDSliceInArena(arena, fieldIDs)
		decl.setFieldMetadata(fieldIDs, defaultFieldSourcesInArena(arena, fieldIDs))
	}
	populateParentNode(decl, decl.children)
	return decl
}

func csharpFindMatchingBracketByte(source []byte, openPos, limit uint32) (uint32, bool) {
	return csharpFindMatchingDelimitedByte(source, openPos, limit, '[', ']')
}

func csharpFindMatchingParenByte(source []byte, openPos, limit uint32) (uint32, bool) {
	return csharpFindMatchingDelimitedByte(source, openPos, limit, '(', ')')
}

func csharpFindMatchingDelimitedByte(source []byte, openPos, limit uint32, openCh, closeCh byte) (uint32, bool) {
	if openPos >= limit || int(limit) > len(source) || source[openPos] != openCh {
		return 0, false
	}
	depth := 0
	inString := false
	escape := false
	for i := openPos; i < limit; i++ {
		b := source[i]
		if inString {
			if escape {
				escape = false
				continue
			}
			if b == '\\' {
				escape = true
				continue
			}
			if b == '"' {
				inString = false
			}
			continue
		}
		switch b {
		case '"':
			inString = true
		case openCh:
			depth++
		case closeCh:
			depth--
			if depth == 0 {
				return i, true
			}
		}
	}
	return 0, false
}

func csharpIsDecimalLiteral(source []byte) bool {
	if len(source) == 0 {
		return false
	}
	for _, b := range source {
		if b < '0' || b > '9' {
			return false
		}
	}
	return true
}
