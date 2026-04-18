package gotreesitter

func normalizeRustCompatibility(root *Node, source []byte, p *Parser, lang *Language) {
	normalizeRustRecoveredPatternStatementsRoot(root, source, p)
	normalizeRustRecoveredFunctionItems(root, source, lang)
	normalizeRustRecoveredStructExpressionRoot(root, source, lang)
	normalizeRustDotRangeExpressions(root, source, lang)
	normalizeRustTokenBindingPatterns(root, source, lang)
	normalizeRustRecoveredTokenTrees(root, source, lang)
	normalizeRustSourceFileRoot(root, source, lang)
}

func normalizeRustTokenBindingPatterns(root *Node, source []byte, lang *Language) {
	if root == nil || lang == nil || lang.Name != "rust" || len(source) == 0 {
		return
	}
	tokenBindingPatternSym, ok := symbolByName(lang, "token_binding_pattern")
	if !ok {
		return
	}
	fragmentSpecifierSym, ok := symbolByName(lang, "fragment_specifier")
	if !ok {
		return
	}
	tokenBindingPatternNamed := true
	if int(tokenBindingPatternSym) < len(lang.SymbolMetadata) {
		tokenBindingPatternNamed = lang.SymbolMetadata[tokenBindingPatternSym].Named
	}
	fragmentSpecifierNamed := true
	if int(fragmentSpecifierSym) < len(lang.SymbolMetadata) {
		fragmentSpecifierNamed = lang.SymbolMetadata[fragmentSpecifierSym].Named
	}

	var walk func(*Node)
	walk = func(node *Node) {
		if node == nil {
			return
		}
		for _, child := range node.children {
			walk(child)
		}
		if node.Type(lang) != "token_tree_pattern" || len(node.children) < 3 {
			return
		}
		for i := 0; i+2 < len(node.children); i++ {
			meta := node.children[i]
			colon := node.children[i+1]
			frag := node.children[i+2]
			if meta == nil || colon == nil || frag == nil {
				continue
			}
			if meta.Type(lang) != "metavariable" || colon.Type(lang) != ":" || frag.Type(lang) != "identifier" {
				continue
			}
			if !rustFragmentSpecifierFollowsColon(meta, colon, frag, source) {
				continue
			}
			fragClone := cloneNodeInArena(frag.ownerArena, frag)
			fragClone.symbol = fragmentSpecifierSym
			fragClone.isNamed = fragmentSpecifierNamed
			fragClone.children = nil
			fragClone.fieldIDs = nil
			fragClone.fieldSources = nil

			binding := cloneNodeInArena(node.ownerArena, meta)
			binding.symbol = tokenBindingPatternSym
			binding.isNamed = tokenBindingPatternNamed
			binding.children = cloneNodeSliceInArena(binding.ownerArena, []*Node{meta, fragClone})
			binding.fieldIDs = nil
			binding.fieldSources = nil
			binding.productionID = 0
			populateParentNode(binding, binding.children)

			replaceChildRangeWithSingleNode(node, i, i+3, binding)
		}
	}
	walk(root)
}

func normalizeRustRecoveredTokenTrees(root *Node, source []byte, lang *Language) {
	if root == nil || lang == nil || lang.Name != "rust" || len(source) == 0 {
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
		if node.Type(lang) != "token_tree" || !node.HasError() {
			return
		}
		recovered, ok := rustBuildRecoveredTokenTree(node.ownerArena, source, lang, node.startByte, node.endByte)
		if !ok || recovered == nil {
			return
		}
		*node = *recovered
	}

	walk(root)
	rustRefreshRecoveredErrorFlags(root)
}

func normalizeRustDotRangeExpressions(root *Node, source []byte, lang *Language) {
	if root == nil || lang == nil || lang.Name != "rust" || len(source) == 0 {
		return
	}
	var walk func(*Node)
	walk = func(node *Node) {
		if node == nil {
			return
		}
		if node.Type(lang) == "range_expression" || node.Type(lang) == "assignment_expression" {
			if recovered, ok := rustBuildCanonicalDotRangeNode(node.ownerArena, source, lang, node.startByte, node.endByte); ok && recovered != nil {
				*node = *recovered
				return
			}
		}
		for _, child := range node.children {
			walk(child)
		}
	}
	walk(root)
	rustRefreshRecoveredErrorFlags(root)
}

func normalizeRustRecoveredPatternStatementsRoot(root *Node, source []byte, p *Parser) {
	if root == nil || p == nil || p.language == nil || p.language.Name != "rust" || p.skipRecoveryReparse || root.Type(p.language) != "ERROR" || len(source) == 0 {
		return
	}
	recovered, ok := rustRecoverTopLevelChunks(source, p, root.ownerArena)
	if !ok || len(recovered) == 0 {
		return
	}
	sourceFileSym, ok := symbolByName(p.language, "source_file")
	if !ok {
		return
	}
	root.children = cloneNodeSliceInArena(root.ownerArena, recovered)
	root.fieldIDs = nil
	root.fieldSources = nil
	root.symbol = sourceFileSym
	root.isNamed = rustNamedForSymbol(p.language, sourceFileSym)
	populateParentNode(root, root.children)
	root.hasError = false
	if root.endByte < uint32(len(source)) && bytesAreTrivia(source[root.endByte:]) {
		extendNodeEndTo(root, uint32(len(source)), source)
	}
}

func normalizeRustRecoveredFunctionItems(root *Node, source []byte, lang *Language) {
	if root == nil || lang == nil || lang.Name != "rust" || len(source) == 0 || len(root.children) == 0 {
		return
	}
	changed := false
	for i := 0; i < len(root.children); i++ {
		recovered, end, ok := rustRecoverFunctionItemFromChildren(root, i, source, lang)
		if !ok {
			continue
		}
		replaceChildRangeWithSingleNode(root, i, end, recovered)
		changed = true
	}
	if changed {
		populateParentNode(root, root.children)
	}
}

func rustRecoverFunctionItemFromChildren(parent *Node, start int, source []byte, lang *Language) (*Node, int, bool) {
	if parent == nil || lang == nil || len(source) == 0 || start < 0 || start+6 >= len(parent.children) {
		return nil, 0, false
	}
	fnErr := parent.children[start]
	name := parent.children[start+1]
	openParen := parent.children[start+2]
	pattern := parent.children[start+3]
	colon := parent.children[start+4]
	implLeaf := parent.children[start+5]
	typeNode := parent.children[start+6]
	if fnErr == nil || name == nil || openParen == nil || pattern == nil || colon == nil || implLeaf == nil || typeNode == nil {
		return nil, 0, false
	}
	if fnErr.Type(lang) != "ERROR" || name.Type(lang) != "identifier" || openParen.Type(lang) != "(" || colon.Type(lang) != ":" || implLeaf.Type(lang) != "impl" || typeNode.Type(lang) != "_type" {
		return nil, 0, false
	}
	if fnErr.startByte >= fnErr.endByte || int(fnErr.endByte) > len(source) || string(source[fnErr.startByte:fnErr.endByte]) != "fn" {
		return nil, 0, false
	}
	paramName := rustPatternIdentifier(pattern, lang)
	if paramName == nil {
		return nil, 0, false
	}
	closeParen := rustFindMatchingDelimiter(source, int(openParen.startByte), '(', ')')
	if closeParen < 0 {
		return nil, 0, false
	}
	if closeParen <= int(implLeaf.startByte) {
		return nil, 0, false
	}
	blockStart := rustSkipSpaceBytes(source, uint32(closeParen+1))
	if int(blockStart) >= len(source) || source[blockStart] != '{' {
		return nil, 0, false
	}
	blockEnd := rustFindMatchingDelimiter(source, int(blockStart), '{', '}')
	if blockEnd < 0 {
		return nil, 0, false
	}

	abstractType, ok := rustBuildRecoveredAbstractType(parent.ownerArena, source, lang, implLeaf.startByte, uint32(closeParen))
	if !ok {
		return nil, 0, false
	}

	functionItemSym, ok := symbolByName(lang, "function_item")
	if !ok {
		return nil, 0, false
	}
	parametersSym, ok := symbolByName(lang, "parameters")
	if !ok {
		return nil, 0, false
	}
	parameterSym, ok := symbolByName(lang, "parameter")
	if !ok {
		return nil, 0, false
	}
	blockSym, ok := symbolByName(lang, "block")
	if !ok {
		return nil, 0, false
	}

	paramClone := cloneNodeInArena(parent.ownerArena, paramName)
	param := newParentNodeInArena(
		parent.ownerArena,
		parameterSym,
		rustNamedForSymbol(lang, parameterSym),
		[]*Node{paramClone, abstractType},
		nil,
		0,
	)
	param.startByte = paramClone.startByte
	param.startPoint = paramClone.startPoint
	param.endByte = uint32(closeParen)
	param.endPoint = advancePointByBytes(Point{}, source[:closeParen])

	params := newParentNodeInArena(
		parent.ownerArena,
		parametersSym,
		rustNamedForSymbol(lang, parametersSym),
		[]*Node{param},
		nil,
		0,
	)
	params.startByte = openParen.startByte
	params.startPoint = openParen.startPoint
	params.endByte = uint32(closeParen + 1)
	params.endPoint = advancePointByBytes(Point{}, source[:closeParen+1])

	block := newParentNodeInArena(
		parent.ownerArena,
		blockSym,
		rustNamedForSymbol(lang, blockSym),
		nil,
		nil,
		0,
	)
	block.startByte = blockStart
	block.startPoint = advancePointByBytes(Point{}, source[:blockStart])
	block.endByte = uint32(blockEnd + 1)
	block.endPoint = advancePointByBytes(Point{}, source[:blockEnd+1])

	fnName := cloneNodeInArena(parent.ownerArena, name)
	functionItem := newParentNodeInArena(
		parent.ownerArena,
		functionItemSym,
		rustNamedForSymbol(lang, functionItemSym),
		[]*Node{fnName, params, block},
		nil,
		0,
	)
	functionItem.startByte = fnErr.startByte
	functionItem.startPoint = fnErr.startPoint
	functionItem.endByte = uint32(blockEnd + 1)
	functionItem.endPoint = advancePointByBytes(Point{}, source[:blockEnd+1])

	end := start + 7
	for end < len(parent.children) && parent.children[end] != nil && parent.children[end].startByte < functionItem.endByte {
		end++
	}
	return functionItem, end, true
}

func rustPatternIdentifier(node *Node, lang *Language) *Node {
	if node == nil || lang == nil {
		return nil
	}
	if node.Type(lang) == "identifier" {
		return node
	}
	for _, child := range node.children {
		if ident := rustPatternIdentifier(child, lang); ident != nil {
			return ident
		}
	}
	return nil
}

func rustBuildRecoveredAbstractType(arena *nodeArena, source []byte, lang *Language, start, end uint32) (*Node, bool) {
	if arena == nil || lang == nil || len(source) == 0 {
		return nil, false
	}
	start, end = rustTrimSpaceBounds(source, start, end)
	if start >= end {
		return nil, false
	}
	abstractTypeSym, ok := symbolByName(lang, "abstract_type")
	if !ok {
		return nil, false
	}
	typeParametersSym, ok := symbolByName(lang, "type_parameters")
	if !ok {
		return nil, false
	}
	lifetimeParameterSym, ok := symbolByName(lang, "lifetime_parameter")
	if !ok {
		return nil, false
	}
	lifetimeSym, ok := symbolByName(lang, "lifetime")
	if !ok {
		return nil, false
	}

	i := rustSkipSpaceBytes(source, start)
	if !rustHasPrefixAt(source, i, "impl") {
		return nil, false
	}
	i = rustSkipSpaceBytes(source, i+4)
	if !rustHasPrefixAt(source, i, "for") {
		return nil, false
	}
	i = rustSkipSpaceBytes(source, i+3)
	if int(i) >= len(source) || source[i] != '<' {
		return nil, false
	}
	typeParamsEnd := rustFindMatchingDelimiter(source, int(i), '<', '>')
	if typeParamsEnd < 0 {
		return nil, false
	}

	lifetimeStart := rustSkipSpaceBytes(source, i+1)
	lifetimeEnd := uint32(typeParamsEnd)
	lifetimeStart, lifetimeEnd = rustTrimSpaceBounds(source, lifetimeStart, lifetimeEnd)
	if lifetimeStart >= lifetimeEnd || source[lifetimeStart] != '\'' {
		return nil, false
	}
	identStart := lifetimeStart + 1
	if identStart >= lifetimeEnd {
		return nil, false
	}

	identifierSym, ok := symbolByName(lang, "identifier")
	if !ok {
		return nil, false
	}
	ident := newLeafNodeInArena(
		arena,
		identifierSym,
		rustNamedForSymbol(lang, identifierSym),
		identStart,
		lifetimeEnd,
		advancePointByBytes(Point{}, source[:identStart]),
		advancePointByBytes(Point{}, source[:lifetimeEnd]),
	)
	lifetime := newParentNodeInArena(
		arena,
		lifetimeSym,
		rustNamedForSymbol(lang, lifetimeSym),
		[]*Node{ident},
		nil,
		0,
	)
	lifetime.startByte = lifetimeStart
	lifetime.startPoint = advancePointByBytes(Point{}, source[:lifetimeStart])
	lifetime.endByte = lifetimeEnd
	lifetime.endPoint = advancePointByBytes(Point{}, source[:lifetimeEnd])

	lifetimeParam := newParentNodeInArena(
		arena,
		lifetimeParameterSym,
		rustNamedForSymbol(lang, lifetimeParameterSym),
		[]*Node{lifetime},
		nil,
		0,
	)
	lifetimeParam.startByte = lifetimeStart
	lifetimeParam.startPoint = lifetime.startPoint
	lifetimeParam.endByte = lifetimeEnd
	lifetimeParam.endPoint = lifetime.endPoint

	typeParams := newParentNodeInArena(
		arena,
		typeParametersSym,
		rustNamedForSymbol(lang, typeParametersSym),
		[]*Node{lifetimeParam},
		nil,
		0,
	)
	typeParams.startByte = i
	typeParams.startPoint = advancePointByBytes(Point{}, source[:i])
	typeParams.endByte = uint32(typeParamsEnd + 1)
	typeParams.endPoint = advancePointByBytes(Point{}, source[:typeParamsEnd+1])

	typeStart := rustSkipSpaceBytes(source, uint32(typeParamsEnd+1))
	traitType, ok := rustBuildRecoveredTypeNode(arena, source, lang, typeStart, end)
	if !ok {
		return nil, false
	}

	abstractType := newParentNodeInArena(
		arena,
		abstractTypeSym,
		rustNamedForSymbol(lang, abstractTypeSym),
		[]*Node{typeParams, traitType},
		nil,
		0,
	)
	abstractType.startByte = start
	abstractType.startPoint = advancePointByBytes(Point{}, source[:start])
	abstractType.endByte = end
	abstractType.endPoint = advancePointByBytes(Point{}, source[:end])
	return abstractType, true
}

func rustBuildRecoveredTypeNode(arena *nodeArena, source []byte, lang *Language, start, end uint32) (*Node, bool) {
	if arena == nil || lang == nil || len(source) == 0 {
		return nil, false
	}
	start, end = rustTrimSpaceBounds(source, start, end)
	if start >= end {
		return nil, false
	}
	lifetimeSym, hasLifetime := symbolByName(lang, "lifetime")
	if hasLifetime && source[start] == '\'' {
		identStart := start + 1
		if identStart >= end {
			return nil, false
		}
		identifierSym, ok := symbolByName(lang, "identifier")
		if !ok {
			return nil, false
		}
		ident := newLeafNodeInArena(
			arena,
			identifierSym,
			rustNamedForSymbol(lang, identifierSym),
			identStart,
			end,
			advancePointByBytes(Point{}, source[:identStart]),
			advancePointByBytes(Point{}, source[:end]),
		)
		lifetime := newParentNodeInArena(arena, lifetimeSym, rustNamedForSymbol(lang, lifetimeSym), []*Node{ident}, nil, 0)
		lifetime.startByte = start
		lifetime.startPoint = advancePointByBytes(Point{}, source[:start])
		lifetime.endByte = end
		lifetime.endPoint = advancePointByBytes(Point{}, source[:end])
		return lifetime, true
	}

	typeIdentifierSym, ok := symbolByName(lang, "type_identifier")
	if !ok {
		return nil, false
	}
	typeNameEnd := start
	for typeNameEnd < end && rustIsIdentByte(source[typeNameEnd]) {
		typeNameEnd++
	}
	if typeNameEnd == start {
		return nil, false
	}
	typeIdent := newLeafNodeInArena(
		arena,
		typeIdentifierSym,
		rustNamedForSymbol(lang, typeIdentifierSym),
		start,
		typeNameEnd,
		advancePointByBytes(Point{}, source[:start]),
		advancePointByBytes(Point{}, source[:typeNameEnd]),
	)
	next := rustSkipSpaceBytes(source, typeNameEnd)
	if next >= end || source[next] != '<' {
		return typeIdent, true
	}

	typeArgsEnd := rustFindMatchingDelimiter(source, int(next), '<', '>')
	if typeArgsEnd < 0 || uint32(typeArgsEnd+1) > end {
		return nil, false
	}

	var argChildren []*Node
	for _, span := range rustSplitTopLevelTypeArgSpans(source, next+1, uint32(typeArgsEnd)) {
		child, ok := rustBuildRecoveredTypeNode(arena, source, lang, span[0], span[1])
		if !ok {
			return nil, false
		}
		argChildren = append(argChildren, child)
	}
	typeArgumentsSym, ok := symbolByName(lang, "type_arguments")
	if !ok {
		return nil, false
	}
	typeArgs := newParentNodeInArena(
		arena,
		typeArgumentsSym,
		rustNamedForSymbol(lang, typeArgumentsSym),
		argChildren,
		nil,
		0,
	)
	typeArgs.startByte = next
	typeArgs.startPoint = advancePointByBytes(Point{}, source[:next])
	typeArgs.endByte = uint32(typeArgsEnd + 1)
	typeArgs.endPoint = advancePointByBytes(Point{}, source[:typeArgsEnd+1])

	genericTypeSym, ok := symbolByName(lang, "generic_type")
	if !ok {
		return nil, false
	}
	genericType := newParentNodeInArena(
		arena,
		genericTypeSym,
		rustNamedForSymbol(lang, genericTypeSym),
		[]*Node{typeIdent, typeArgs},
		nil,
		0,
	)
	genericType.startByte = start
	genericType.startPoint = advancePointByBytes(Point{}, source[:start])
	genericType.endByte = uint32(typeArgsEnd + 1)
	genericType.endPoint = advancePointByBytes(Point{}, source[:typeArgsEnd+1])
	return genericType, true
}

func rustSplitTopLevelTypeArgSpans(source []byte, start, end uint32) [][2]uint32 {
	start, end = rustTrimSpaceBounds(source, start, end)
	if start >= end {
		return nil
	}
	var spans [][2]uint32
	depth := 0
	partStart := start
	for i := start; i < end; i++ {
		switch source[i] {
		case '<':
			depth++
		case '>':
			if depth > 0 {
				depth--
			}
		case ',':
			if depth == 0 {
				a, b := rustTrimSpaceBounds(source, partStart, i)
				if a < b {
					spans = append(spans, [2]uint32{a, b})
				}
				partStart = i + 1
			}
		}
	}
	a, b := rustTrimSpaceBounds(source, partStart, end)
	if a < b {
		spans = append(spans, [2]uint32{a, b})
	}
	return spans
}

func rustRecoverTopLevelChunks(source []byte, p *Parser, arena *nodeArena) ([]*Node, bool) {
	if p == nil || p.language == nil || len(source) == 0 {
		return nil, false
	}
	spans := rustTopLevelChunkSpans(source)
	if len(spans) == 0 {
		return nil, false
	}
	out := make([]*Node, 0, len(spans))
	for _, span := range spans {
		for _, part := range rustSplitLeadingTopLevelCommentSpans(source, span[0], span[1]) {
			nodes, ok := rustRecoverTopLevelChunkNodesFromRange(source, part[0], part[1], p, arena)
			if !ok || len(nodes) == 0 {
				return nil, false
			}
			out = append(out, nodes...)
		}
	}
	return out, true
}

func rustSplitLeadingTopLevelCommentSpans(source []byte, start, end uint32) [][2]uint32 {
	start, end = rustTrimSpaceBounds(source, start, end)
	if start >= end {
		return nil
	}
	var spans [][2]uint32
	cursor := start
	for cursor < end {
		switch {
		case cursor+1 < end && source[cursor] == '/' && source[cursor+1] == '/':
			commentEnd := cursor + 2
			for commentEnd < end && source[commentEnd] != '\n' {
				commentEnd++
			}
			spans = append(spans, [2]uint32{cursor, commentEnd})
			cursor = rustSkipSpaceBytes(source, commentEnd)
		case cursor+1 < end && source[cursor] == '/' && source[cursor+1] == '*':
			commentEnd := rustFindBlockCommentEnd(source, cursor+2, end)
			if commentEnd <= cursor+1 {
				return [][2]uint32{{start, end}}
			}
			spans = append(spans, [2]uint32{cursor, commentEnd})
			cursor = rustSkipSpaceBytes(source, commentEnd)
		default:
			if cursor < end {
				spans = append(spans, [2]uint32{cursor, end})
			}
			return spans
		}
	}
	if len(spans) == 0 {
		spans = append(spans, [2]uint32{start, end})
	}
	return spans
}

func rustTopLevelChunkSpans(source []byte) [][2]uint32 {
	var spans [][2]uint32
	start := uint32(0)
	for start < uint32(len(source)) && rustIsSpaceByte(source[start]) {
		start++
	}
	if start >= uint32(len(source)) {
		return nil
	}
	braceDepth := 0
	parenDepth := 0
	bracketDepth := 0
	inString := false
	escaped := false
	for i := start; i < uint32(len(source)); i++ {
		b := source[i]
		if inString {
			if escaped {
				escaped = false
				continue
			}
			if b == '\\' {
				escaped = true
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
		case '{':
			braceDepth++
		case '}':
			if braceDepth > 0 {
				braceDepth--
				if braceDepth == 0 && parenDepth == 0 && bracketDepth == 0 {
					next := rustSkipSpaceBytes(source, i+1)
					if next >= uint32(len(source)) || source[next] != ';' {
						spans = append(spans, [2]uint32{start, i + 1})
						start = next
					}
				}
			}
		case '(':
			parenDepth++
		case ')':
			if parenDepth > 0 {
				parenDepth--
			}
		case '[':
			bracketDepth++
		case ']':
			if bracketDepth > 0 {
				bracketDepth--
			}
		case ';':
			if braceDepth == 0 && parenDepth == 0 && bracketDepth == 0 {
				spans = append(spans, [2]uint32{start, i + 1})
				start = rustSkipSpaceBytes(source, i+1)
			}
		}
	}
	if start < uint32(len(source)) {
		start = rustSkipSpaceBytes(source, start)
		if start < uint32(len(source)) {
			return nil
		}
	}
	return spans
}

func rustRecoverTopLevelChunkNodesFromRange(source []byte, start, end uint32, p *Parser, arena *nodeArena) ([]*Node, bool) {
	if p == nil || p.language == nil || start >= end || int(end) > len(source) {
		return nil, false
	}
	chunk := source[start:end]
	tree, err := p.parseForRecovery(chunk)
	if err == nil && tree != nil && tree.RootNode() != nil {
		startPoint := advancePointByBytes(Point{}, source[:start])
		offsetRoot := tree.RootNodeWithOffset(start, startPoint)
		if offsetRoot != nil && !offsetRoot.HasError() {
			nodes := rustExtractRecoveredTopLevelNodes(offsetRoot, p.language, arena)
			tree.Release()
			if len(nodes) > 0 && !rustRecoveredNodesNeedFunctionFallback(source, start, end, p.language, nodes) {
				return nodes, true
			}
		}
		tree.Release()
	}
	if node, ok := rustRecoverClosureExpressionStatementFromRange(source, start, end, p, arena); ok {
		return []*Node{node}, true
	}
	if node, ok := rustRecoverFunctionItemFromRange(source, start, end, p, arena); ok {
		return []*Node{node}, true
	}
	return nil, false
}

func rustExtractRecoveredTopLevelNodes(root *Node, lang *Language, arena *nodeArena) []*Node {
	if root == nil || lang == nil {
		return nil
	}
	if root.Type(lang) != "source_file" {
		if root.IsNamed() {
			if arena != nil {
				return []*Node{cloneTreeNodesIntoArena(root, arena)}
			}
			return []*Node{root}
		}
		return nil
	}
	out := make([]*Node, 0, root.NamedChildCount())
	for i := 0; i < root.NamedChildCount(); i++ {
		child := root.NamedChild(i)
		if child == nil {
			continue
		}
		if arena != nil {
			out = append(out, cloneTreeNodesIntoArena(child, arena))
		} else {
			out = append(out, child)
		}
	}
	return out
}
