package gotreesitter

import "bytes"

func rustRecoverClosureExpressionStatementFromRange(source []byte, start, end uint32, p *Parser, arena *nodeArena) (*Node, bool) {
	if p == nil || p.language == nil || start >= end || int(end) > len(source) {
		return nil, false
	}
	start, end = rustTrimSpaceBounds(source, start, end)
	if start >= end || source[start] != '|' || source[end-1] != ';' {
		return nil, false
	}
	pipePos, ok := rustFindTopLevelByte(source, start+1, end-1, '|')
	if !ok || pipePos <= start+1 || pipePos+1 >= end {
		return nil, false
	}
	pattern, ok := rustRecoverPatternNodeFromRange(source, start+1, pipePos, p, arena)
	if !ok {
		return nil, false
	}
	body, ok := rustRecoverRustExpressionNodeFromRange(source, pipePos+1, end-1, p, arena)
	if !ok {
		return nil, false
	}
	paramsSym, ok := symbolByName(p.language, "closure_parameters")
	if !ok {
		return nil, false
	}
	closureSym, ok := symbolByName(p.language, "closure_expression")
	if !ok {
		return nil, false
	}
	exprStmtSym, ok := symbolByName(p.language, "expression_statement")
	if !ok {
		return nil, false
	}
	params := newParentNodeInArena(
		arena,
		paramsSym,
		rustNamedForSymbol(p.language, paramsSym),
		[]*Node{pattern},
		nil,
		0,
	)
	params.startByte = start
	params.startPoint = advancePointByBytes(Point{}, source[:start])
	params.endByte = pipePos + 1
	params.endPoint = advancePointByBytes(Point{}, source[:pipePos+1])

	closure := newParentNodeInArena(
		arena,
		closureSym,
		rustNamedForSymbol(p.language, closureSym),
		[]*Node{params, body},
		nil,
		0,
	)
	closure.startByte = start
	closure.startPoint = params.startPoint
	closure.endByte = body.endByte
	closure.endPoint = body.endPoint

	stmt := newParentNodeInArena(
		arena,
		exprStmtSym,
		rustNamedForSymbol(p.language, exprStmtSym),
		[]*Node{closure},
		nil,
		0,
	)
	stmt.startByte = start
	stmt.startPoint = params.startPoint
	stmt.endByte = end
	stmt.endPoint = advancePointByBytes(Point{}, source[:end])
	return stmt, true
}

func rustRecoverFunctionItemFromRange(source []byte, start, end uint32, p *Parser, arena *nodeArena) (*Node, bool) {
	if p == nil || p.language == nil || start >= end || int(end) > len(source) {
		return nil, false
	}
	start, end = rustTrimSpaceBounds(source, start, end)
	if start >= end {
		return nil, false
	}
	if _, ok := rustFunctionKeywordStart(source, start, end); !ok {
		return nil, false
	}
	openBrace, ok := rustFindTopLevelByte(source, start, end, '{')
	if !ok {
		return nil, false
	}
	closeBrace := rustFindMatchingDelimiter(source, int(openBrace), '{', '}')
	if closeBrace < 0 || uint32(closeBrace+1) != end {
		return nil, false
	}
	headerNodes, ok := rustRecoverFunctionHeaderNodesFromRange(source, start, openBrace, p, arena)
	if !ok || len(headerNodes) < 2 {
		return nil, false
	}
	bodyNodes, ok := rustRecoverRustBlockNodesFromRange(source, openBrace+1, uint32(closeBrace), p, arena)
	if !ok {
		return nil, false
	}
	blockSym, ok := symbolByName(p.language, "block")
	if !ok {
		return nil, false
	}
	functionItemSym, ok := symbolByName(p.language, "function_item")
	if !ok {
		return nil, false
	}
	block := newParentNodeInArena(
		arena,
		blockSym,
		rustNamedForSymbol(p.language, blockSym),
		bodyNodes,
		nil,
		0,
	)
	block.startByte = openBrace
	block.startPoint = advancePointByBytes(Point{}, source[:openBrace])
	block.endByte = uint32(closeBrace + 1)
	block.endPoint = advancePointByBytes(Point{}, source[:closeBrace+1])

	children := append(headerNodes, block)
	fnItem := newParentNodeInArena(
		arena,
		functionItemSym,
		rustNamedForSymbol(p.language, functionItemSym),
		children,
		nil,
		0,
	)
	fnItem.startByte = start
	fnItem.startPoint = advancePointByBytes(Point{}, source[:start])
	fnItem.endByte = end
	fnItem.endPoint = advancePointByBytes(Point{}, source[:end])
	return fnItem, true
}

func rustRecoverFunctionHeaderNodesFromRange(source []byte, start, blockStart uint32, p *Parser, arena *nodeArena) ([]*Node, bool) {
	if p == nil || p.language == nil || start >= blockStart || int(blockStart) > len(source) {
		return nil, false
	}
	wrapped := make([]byte, 0, int(blockStart-start)+2)
	wrapped = append(wrapped, source[start:blockStart]...)
	wrapped = append(wrapped, '{', '}')
	tree, err := p.parseForRecovery(wrapped)
	if err != nil || tree == nil || tree.RootNode() == nil {
		if tree != nil {
			tree.Release()
		}
		return nil, false
	}
	defer tree.Release()
	startPoint := advancePointByBytes(Point{}, source[:start])
	offsetRoot := tree.rootNodeWithRecoveryOffset(start, startPoint)
	if offsetRoot == nil || offsetRoot.HasError() {
		return nil, false
	}
	return rustExtractRecoveredFunctionHeaderNodes(offsetRoot, p.language, arena)
}

func rustExtractRecoveredFunctionHeaderNodes(root *Node, lang *Language, arena *nodeArena) ([]*Node, bool) {
	if root == nil || lang == nil {
		return nil, false
	}
	var fnItem *Node
	var walk func(*Node)
	walk = func(n *Node) {
		if n == nil || fnItem != nil {
			return
		}
		if n.Type(lang) == "function_item" {
			fnItem = n
			return
		}
		for i := 0; i < n.ChildCount(); i++ {
			walk(n.Child(i))
		}
	}
	walk(root)
	if fnItem == nil {
		return nil, false
	}
	out := make([]*Node, 0, fnItem.ChildCount())
	for i := 0; i < fnItem.ChildCount(); i++ {
		child := fnItem.Child(i)
		if child == nil || child.Type(lang) == "block" {
			continue
		}
		if arena != nil {
			out = append(out, cloneTreeNodesIntoArena(child, arena))
		} else {
			out = append(out, child)
		}
	}
	return out, len(out) >= 2
}

func rustRecoverRustBlockNodesFromRange(source []byte, start, end uint32, p *Parser, arena *nodeArena) ([]*Node, bool) {
	spans := rustStatementLikeSpansInRange(source, start, end)
	if len(spans) == 0 && bytes.TrimSpace(source[start:end]) != nil && len(bytes.TrimSpace(source[start:end])) > 0 {
		return nil, false
	}
	out := make([]*Node, 0, len(spans))
	for _, span := range spans {
		nodes, ok := rustRecoverRustBlockChunkNodesFromRange(source, span[0], span[1], p, arena)
		if !ok || len(nodes) == 0 {
			return nil, false
		}
		out = append(out, nodes...)
	}
	return out, true
}

func rustRecoverRustBlockChunkNodesFromRange(source []byte, start, end uint32, p *Parser, arena *nodeArena) ([]*Node, bool) {
	if p == nil || p.language == nil || start >= end || int(end) > len(source) {
		return nil, false
	}
	if nodes, ok := rustRecoverWrappedFunctionBlockNodesFromRange(source, start, end, p, arena); ok {
		if !rustRecoveredNodesNeedFunctionFallback(source, start, end, p.language, nodes) {
			return nodes, true
		}
	}
	trimmedStart, trimmedEnd := rustTrimSpaceBounds(source, start, end)
	if trimmedStart >= trimmedEnd {
		return nil, false
	}
	switch {
	case rustHasPrefixAt(source, trimmedStart, "let"):
		if node, ok := rustRecoverLetStatementFromRange(source, trimmedStart, trimmedEnd, p, arena); ok {
			return []*Node{node}, true
		}
	case rustHasPrefixAt(source, trimmedStart, "impl"):
		if node, ok := rustRecoverImplItemFromRange(source, trimmedStart, trimmedEnd, p, arena); ok {
			return []*Node{node}, true
		}
	case rustHasPrefixAt(source, trimmedStart, "loop"):
		if node, ok := rustRecoverLoopStatementFromRange(source, trimmedStart, trimmedEnd, p, arena); ok {
			return []*Node{node}, true
		}
	case rustRangeStartsFunctionItem(source, trimmedStart, trimmedEnd):
		if node, ok := rustRecoverFunctionItemFromRange(source, trimmedStart, trimmedEnd, p, arena); ok {
			return []*Node{node}, true
		}
	}
	if node, ok := rustRecoverRustBlockExpressionNodeFromRange(source, trimmedStart, trimmedEnd, p, arena); ok {
		return []*Node{node}, true
	}
	return nil, false
}

func rustRecoverRustBlockExpressionNodeFromRange(source []byte, start, end uint32, p *Parser, arena *nodeArena) (*Node, bool) {
	if p == nil || p.language == nil || start >= end || int(end) > len(source) {
		return nil, false
	}
	start, end = rustTrimSpaceBounds(source, start, end)
	if start >= end {
		return nil, false
	}
	exprEnd := end
	withSemicolon := false
	if source[end-1] == ';' {
		withSemicolon = true
		exprEnd = rustSkipBackwardSpaceBytes(source, end-1)
		if exprEnd <= start {
			return nil, false
		}
	}
	expr, ok := rustRecoverRustExpressionNodeFromRange(source, start, exprEnd, p, arena)
	if !ok {
		return nil, false
	}
	if !withSemicolon {
		return expr, true
	}
	exprStmtSym, ok := symbolByName(p.language, "expression_statement")
	if !ok {
		return nil, false
	}
	stmt := newParentNodeInArena(
		arena,
		exprStmtSym,
		rustNamedForSymbol(p.language, exprStmtSym),
		[]*Node{expr},
		nil,
		0,
	)
	stmt.startByte = start
	stmt.startPoint = advancePointByBytes(Point{}, source[:start])
	stmt.endByte = end
	stmt.endPoint = advancePointByBytes(Point{}, source[:end])
	return stmt, true
}

func rustRecoverRustBlockValueExpressionNodeFromRange(source []byte, start, end uint32, p *Parser, arena *nodeArena) (*Node, bool) {
	if p == nil || p.language == nil || start >= end || int(end) > len(source) {
		return nil, false
	}
	start, end = rustTrimSpaceBounds(source, start, end)
	if start >= end || source[start] != '{' || source[end-1] != '}' {
		return nil, false
	}
	closeBrace := rustFindMatchingDelimiter(source, int(start), '{', '}')
	if closeBrace < 0 || uint32(closeBrace+1) != end {
		return nil, false
	}
	children, ok := rustRecoverRustBlockNodesFromRange(source, start+1, uint32(closeBrace), p, arena)
	if !ok {
		return nil, false
	}
	blockSym, ok := symbolByName(p.language, "block")
	if !ok {
		return nil, false
	}
	block := newParentNodeInArena(
		arena,
		blockSym,
		rustNamedForSymbol(p.language, blockSym),
		children,
		nil,
		0,
	)
	block.startByte = start
	block.startPoint = advancePointByBytes(Point{}, source[:start])
	block.endByte = end
	block.endPoint = advancePointByBytes(Point{}, source[:end])
	return block, true
}

func rustRecoveredNodesNeedFunctionFallback(source []byte, start, end uint32, lang *Language, nodes []*Node) bool {
	if lang == nil || len(nodes) == 0 {
		return false
	}
	start, end = rustTrimSpaceBounds(source, start, end)
	if start >= end {
		return false
	}
	if _, ok := rustFunctionKeywordStart(source, start, end); !ok {
		return false
	}
	return len(nodes) != 1 || nodes[0] == nil || nodes[0].Type(lang) != "function_item"
}

func rustFunctionKeywordStart(source []byte, start, end uint32) (uint32, bool) {
	start, end = rustTrimSpaceBounds(source, start, end)
	if start >= end {
		return 0, false
	}
	if rustKeywordAt(source, start, end, "fn") {
		return start, true
	}
	if !rustKeywordAt(source, start, end, "pub") {
		return 0, false
	}
	cursor := start + 3
	if cursor < end && source[cursor] == '(' {
		closeParen := rustFindMatchingDelimiter(source, int(cursor), '(', ')')
		if closeParen < 0 || uint32(closeParen+1) > end {
			return 0, false
		}
		cursor = uint32(closeParen + 1)
	}
	cursor = rustSkipSpaceBytes(source, cursor)
	if rustKeywordAt(source, cursor, end, "fn") {
		return cursor, true
	}
	return 0, false
}

func rustRangeStartsFunctionItem(source []byte, start, end uint32) bool {
	_, ok := rustFunctionKeywordStart(source, start, end)
	return ok
}

func rustKeywordAt(source []byte, pos, end uint32, keyword string) bool {
	kwEnd := pos + uint32(len(keyword))
	if pos >= end || kwEnd > end || !rustHasPrefixAt(source, pos, keyword) {
		return false
	}
	if pos > 0 && rustIsIdentByte(source[pos-1]) {
		return false
	}
	return kwEnd >= end || !rustIsIdentByte(source[kwEnd])
}

func rustRecoverRustSpecialPatternNodeFromRange(source []byte, start, end uint32, p *Parser, arena *nodeArena) (*Node, bool) {
	start, end = rustTrimSpaceBounds(source, start, end)
	if start >= end || source[start] != '(' || source[end-1] != ')' {
		return nil, false
	}
	return rustRecoverRustTupleClosurePatternNodeFromRange(source, start, end, p, arena)
}

func rustRecoverRustTupleClosurePatternNodeFromRange(source []byte, start, end uint32, p *Parser, arena *nodeArena) (*Node, bool) {
	if p == nil || p.language == nil || start >= end || int(end) > len(source) {
		return nil, false
	}
	start, end = rustTrimSpaceBounds(source, start, end)
	if start >= end || source[start] != '(' || source[end-1] != ')' {
		return nil, false
	}
	innerStart, innerEnd := rustTrimSpaceBounds(source, start+1, end-1)
	if innerStart >= innerEnd || source[innerStart] != '|' {
		return nil, false
	}
	closure, ok := rustRecoverRustClosureExpressionNodeFromRange(source, innerStart, innerEnd, p, arena)
	if !ok {
		return nil, false
	}
	tuplePatternSym, ok := symbolByName(p.language, "tuple_pattern")
	if !ok {
		return nil, false
	}
	pattern := newParentNodeInArena(
		arena,
		tuplePatternSym,
		rustNamedForSymbol(p.language, tuplePatternSym),
		[]*Node{closure},
		nil,
		0,
	)
	pattern.startByte = start
	pattern.startPoint = advancePointByBytes(Point{}, source[:start])
	pattern.endByte = end
	pattern.endPoint = advancePointByBytes(Point{}, source[:end])
	return pattern, true
}

func rustRecoverRustClosureExpressionNodeFromRange(source []byte, start, end uint32, p *Parser, arena *nodeArena) (*Node, bool) {
	if p == nil || p.language == nil || start >= end || int(end) > len(source) {
		return nil, false
	}
	start, end = rustTrimSpaceBounds(source, start, end)
	if start >= end || source[start] != '|' {
		return nil, false
	}
	pipePos, ok := rustFindTopLevelByte(source, start+1, end, '|')
	if !ok || pipePos >= end {
		return nil, false
	}
	bodyStart := rustSkipSpaceBytes(source, pipePos+1)
	if bodyStart >= end {
		return nil, false
	}
	paramNodes, ok := rustRecoverRustClosureParameterNodesFromRange(source, start+1, pipePos, p, arena)
	if !ok {
		return nil, false
	}
	body, ok := rustRecoverRustExpressionNodeFromRange(source, bodyStart, end, p, arena)
	if !ok {
		return nil, false
	}
	paramsSym, ok := symbolByName(p.language, "closure_parameters")
	if !ok {
		return nil, false
	}
	closureSym, ok := symbolByName(p.language, "closure_expression")
	if !ok {
		return nil, false
	}
	params := newParentNodeInArena(
		arena,
		paramsSym,
		rustNamedForSymbol(p.language, paramsSym),
		paramNodes,
		nil,
		0,
	)
	params.startByte = start
	params.startPoint = advancePointByBytes(Point{}, source[:start])
	params.endByte = pipePos + 1
	params.endPoint = advancePointByBytes(Point{}, source[:pipePos+1])
	closure := newParentNodeInArena(
		arena,
		closureSym,
		rustNamedForSymbol(p.language, closureSym),
		[]*Node{params, body},
		nil,
		0,
	)
	closure.startByte = start
	closure.startPoint = params.startPoint
	closure.endByte = end
	closure.endPoint = advancePointByBytes(Point{}, source[:end])
	return closure, true
}

func rustRecoverRustClosureParameterNodesFromRange(source []byte, start, end uint32, p *Parser, arena *nodeArena) ([]*Node, bool) {
	if p == nil || p.language == nil {
		return nil, false
	}
	start, end = rustTrimSpaceBounds(source, start, end)
	if start >= end {
		return nil, true
	}
	spans := rustSplitTopLevelCommaSpans(source, start, end)
	if len(spans) == 0 {
		spans = append(spans, [2]uint32{start, end})
	}
	children := make([]*Node, 0, len(spans))
	for _, span := range spans {
		partStart, partEnd := rustTrimSpaceBounds(source, span[0], span[1])
		if partStart >= partEnd {
			continue
		}
		part := source[partStart:partEnd]
		if len(part) == 1 && part[0] == '_' {
			continue
		}
		if rustBytesAreIdentifier(part) {
			identifierSym, ok := symbolByName(p.language, "identifier")
			if !ok {
				return nil, false
			}
			children = append(children, newLeafNodeInArena(
				arena,
				identifierSym,
				rustNamedForSymbol(p.language, identifierSym),
				partStart,
				partEnd,
				advancePointByBytes(Point{}, source[:partStart]),
				advancePointByBytes(Point{}, source[:partEnd]),
			))
			continue
		}
		if node, ok := rustRecoverRustCapturedPatternNodeFromRange(source, partStart, partEnd, p, arena); ok {
			children = append(children, node)
			continue
		}
		if node, ok := rustRecoverRustTupleClosurePatternNodeFromRange(source, partStart, partEnd, p, arena); ok {
			children = append(children, node)
			continue
		}
		if node, ok := rustRecoverRustClosureParameterNodeFromRange(source, partStart, partEnd, p, arena); ok {
			children = append(children, node)
			continue
		}
		if node, ok := rustRecoverPatternNodeFromRange(source, partStart, partEnd, p, arena); ok {
			children = append(children, node)
			continue
		}
		return nil, false
	}
	return children, true
}

func rustRecoverRustClosureParameterNodeFromRange(source []byte, start, end uint32, p *Parser, arena *nodeArena) (*Node, bool) {
	if p == nil || p.language == nil || start >= end || int(end) > len(source) {
		return nil, false
	}
	const prefix = "fn _("
	const suffix = ") {}\n"
	part := bytes.TrimSpace(source[start:end])
	if len(part) == 0 {
		return nil, false
	}
	wrapped := make([]byte, 0, len(prefix)+len(part)+len(suffix))
	wrapped = append(wrapped, prefix...)
	wrapped = append(wrapped, part...)
	wrapped = append(wrapped, suffix...)
	tree, err := p.parseForRecovery(wrapped)
	if err != nil || tree == nil || tree.RootNode() == nil {
		if tree != nil {
			tree.Release()
		}
		return nil, false
	}
	defer tree.Release()
	startPoint := advancePointByBytes(Point{}, source[:start])
	prefixPoint := advancePointByBytes(Point{}, []byte(prefix))
	if startPoint.Row < prefixPoint.Row {
		return nil, false
	}
	offsetRoot := tree.rootNodeWithRecoveryOffset(start-uint32(len(prefix)), Point{Row: startPoint.Row - prefixPoint.Row, Column: startPoint.Column})
	if offsetRoot == nil || offsetRoot.HasError() {
		return nil, false
	}
	var out *Node
	var walk func(*Node)
	walk = func(n *Node) {
		if n == nil || out != nil {
			return
		}
		if n.Type(p.language) == "parameters" {
			for i := 0; i < n.NamedChildCount(); i++ {
				child := n.NamedChild(i)
				if child == nil {
					continue
				}
				if arena != nil {
					out = cloneTreeNodesIntoArena(child, arena)
				} else {
					out = child
				}
				return
			}
		}
		for i := 0; i < n.ChildCount(); i++ {
			walk(n.Child(i))
		}
	}
	walk(offsetRoot)
	return out, out != nil
}

func rustRecoverRustCapturedPatternNodeFromRange(source []byte, start, end uint32, p *Parser, arena *nodeArena) (*Node, bool) {
	if p == nil || p.language == nil || start >= end || int(end) > len(source) {
		return nil, false
	}
	start, end = rustTrimSpaceBounds(source, start, end)
	atPos, ok := rustFindTopLevelByte(source, start, end, '@')
	if !ok {
		return nil, false
	}
	nameStart, nameEnd := rustTrimSpaceBounds(source, start, atPos)
	restStart, restEnd := rustTrimSpaceBounds(source, atPos+1, end)
	if nameStart >= nameEnd || restStart >= restEnd || restEnd-restStart != 1 || source[restStart] != '_' || !rustBytesAreIdentifier(source[nameStart:nameEnd]) {
		return nil, false
	}
	identifierSym, ok := symbolByName(p.language, "identifier")
	if !ok {
		return nil, false
	}
	capturedSym, ok := symbolByName(p.language, "captured_pattern")
	if !ok {
		return nil, false
	}
	ident := newLeafNodeInArena(
		arena,
		identifierSym,
		rustNamedForSymbol(p.language, identifierSym),
		nameStart,
		nameEnd,
		advancePointByBytes(Point{}, source[:nameStart]),
		advancePointByBytes(Point{}, source[:nameEnd]),
	)
	captured := newParentNodeInArena(
		arena,
		capturedSym,
		rustNamedForSymbol(p.language, capturedSym),
		[]*Node{ident},
		nil,
		0,
	)
	captured.startByte = start
	captured.startPoint = advancePointByBytes(Point{}, source[:start])
	captured.endByte = end
	captured.endPoint = advancePointByBytes(Point{}, source[:end])
	return captured, true
}

func rustRecoverRustReferenceCastClosureExpressionNodeFromRange(source []byte, start, end uint32, p *Parser, arena *nodeArena) (*Node, bool) {
	if p == nil || p.language == nil || start >= end || int(end) > len(source) {
		return nil, false
	}
	start, end = rustTrimSpaceBounds(source, start, end)
	if start >= end || source[start] != '&' {
		return nil, false
	}
	outerStart := rustSkipSpaceBytes(source, start+1)
	if outerStart >= end || source[outerStart] != '(' {
		return nil, false
	}
	outerClose := rustFindMatchingDelimiter(source, int(outerStart), '(', ')')
	if outerClose < 0 || uint32(outerClose+1) != end {
		return nil, false
	}
	innerStart, innerEnd := rustTrimSpaceBounds(source, outerStart+1, uint32(outerClose))
	asPos, ok := rustFindTopLevelKeyword(source, innerStart, innerEnd, "as")
	if !ok {
		return nil, false
	}
	closureRangeStart, closureRangeEnd := rustTrimSpaceBounds(source, innerStart, asPos)
	if closureRangeStart >= closureRangeEnd || source[closureRangeStart] != '(' || source[closureRangeEnd-1] != ')' {
		return nil, false
	}
	closure, ok := rustRecoverRustClosureExpressionNodeFromRange(source, closureRangeStart+1, closureRangeEnd-1, p, arena)
	if !ok {
		return nil, false
	}
	typeNode, ok := rustBuildRecoveredTypeNode(arena, source, p.language, asPos+2, innerEnd)
	if !ok {
		return nil, false
	}
	parenthesizedSym, ok := symbolByName(p.language, "parenthesized_expression")
	if !ok {
		return nil, false
	}
	typeCastSym, ok := symbolByName(p.language, "type_cast_expression")
	if !ok {
		return nil, false
	}
	referenceSym, ok := symbolByName(p.language, "reference_expression")
	if !ok {
		return nil, false
	}
	innerParen := newParentNodeInArena(
		arena,
		parenthesizedSym,
		rustNamedForSymbol(p.language, parenthesizedSym),
		[]*Node{closure},
		nil,
		0,
	)
	innerParen.startByte = closureRangeStart
	innerParen.startPoint = advancePointByBytes(Point{}, source[:closureRangeStart])
	innerParen.endByte = closureRangeEnd
	innerParen.endPoint = advancePointByBytes(Point{}, source[:closureRangeEnd])
	typeCast := newParentNodeInArena(
		arena,
		typeCastSym,
		rustNamedForSymbol(p.language, typeCastSym),
		[]*Node{innerParen, typeNode},
		nil,
		0,
	)
	typeCast.startByte = innerParen.startByte
	typeCast.startPoint = innerParen.startPoint
	typeCast.endByte = innerEnd
	typeCast.endPoint = advancePointByBytes(Point{}, source[:innerEnd])
	outerParen := newParentNodeInArena(
		arena,
		parenthesizedSym,
		rustNamedForSymbol(p.language, parenthesizedSym),
		[]*Node{typeCast},
		nil,
		0,
	)
	outerParen.startByte = outerStart
	outerParen.startPoint = advancePointByBytes(Point{}, source[:outerStart])
	outerParen.endByte = end
	outerParen.endPoint = advancePointByBytes(Point{}, source[:end])
	ref := newParentNodeInArena(
		arena,
		referenceSym,
		rustNamedForSymbol(p.language, referenceSym),
		[]*Node{outerParen},
		nil,
		0,
	)
	ref.startByte = start
	ref.startPoint = advancePointByBytes(Point{}, source[:start])
	ref.endByte = end
	ref.endPoint = advancePointByBytes(Point{}, source[:end])
	return ref, true
}

func rustRecoverRustMatchExpressionNodeFromRange(source []byte, start, end uint32, p *Parser, arena *nodeArena) (*Node, bool) {
	if p == nil || p.language == nil || start >= end || int(end) > len(source) {
		return nil, false
	}
	start, end = rustTrimSpaceBounds(source, start, end)
	if start >= end || !rustHasPrefixAt(source, start, "match") {
		return nil, false
	}
	scrutineeStart := rustSkipSpaceBytes(source, start+5)
	openBrace, ok := rustFindTopLevelByte(source, scrutineeStart, end, '{')
	if !ok {
		return nil, false
	}
	closeBrace := rustFindMatchingDelimiter(source, int(openBrace), '{', '}')
	if closeBrace < 0 || uint32(closeBrace+1) != end {
		return nil, false
	}
	scrutinee, ok := rustRecoverRustExpressionNodeFromRange(source, scrutineeStart, openBrace, p, arena)
	if !ok {
		return nil, false
	}
	armSpans := rustSplitRustMatchArmSpans(source, openBrace+1, uint32(closeBrace))
	if len(armSpans) == 0 {
		return nil, false
	}
	matchArmNodes := make([]*Node, 0, len(armSpans))
	for _, span := range armSpans {
		arm, ok := rustRecoverRustMatchArmNodeFromRange(source, span[0], span[1], p, arena)
		if !ok {
			return nil, false
		}
		matchArmNodes = append(matchArmNodes, arm)
	}
	matchBlockSym, ok := symbolByName(p.language, "match_block")
	if !ok {
		return nil, false
	}
	matchExprSym, ok := symbolByName(p.language, "match_expression")
	if !ok {
		return nil, false
	}
	matchBlock := newParentNodeInArena(
		arena,
		matchBlockSym,
		rustNamedForSymbol(p.language, matchBlockSym),
		matchArmNodes,
		nil,
		0,
	)
	matchBlock.startByte = openBrace
	matchBlock.startPoint = advancePointByBytes(Point{}, source[:openBrace])
	matchBlock.endByte = uint32(closeBrace + 1)
	matchBlock.endPoint = advancePointByBytes(Point{}, source[:closeBrace+1])
	matchExpr := newParentNodeInArena(
		arena,
		matchExprSym,
		rustNamedForSymbol(p.language, matchExprSym),
		[]*Node{scrutinee, matchBlock},
		nil,
		0,
	)
	matchExpr.startByte = start
	matchExpr.startPoint = advancePointByBytes(Point{}, source[:start])
	matchExpr.endByte = end
	matchExpr.endPoint = advancePointByBytes(Point{}, source[:end])
	return matchExpr, true
}

func rustRecoverRustMatchArmNodeFromRange(source []byte, start, end uint32, p *Parser, arena *nodeArena) (*Node, bool) {
	if p == nil || p.language == nil || start >= end || int(end) > len(source) {
		return nil, false
	}
	start, end = rustTrimSpaceBounds(source, start, end)
	arrowPos, ok := rustFindTopLevelFatArrow(source, start, end)
	if !ok {
		return nil, false
	}
	pattern, ok := rustRecoverRustMatchPatternNodeFromRange(source, start, arrowPos, p, arena)
	if !ok {
		return nil, false
	}
	value, ok := rustRecoverRustExpressionNodeFromRange(source, arrowPos+2, end, p, arena)
	if !ok {
		return nil, false
	}
	matchArmSym, ok := symbolByName(p.language, "match_arm")
	if !ok {
		return nil, false
	}
	arm := newParentNodeInArena(
		arena,
		matchArmSym,
		rustNamedForSymbol(p.language, matchArmSym),
		[]*Node{pattern, value},
		nil,
		0,
	)
	arm.startByte = start
	arm.startPoint = advancePointByBytes(Point{}, source[:start])
	arm.endByte = end
	arm.endPoint = advancePointByBytes(Point{}, source[:end])
	return arm, true
}

func rustRecoverRustMatchPatternNodeFromRange(source []byte, start, end uint32, p *Parser, arena *nodeArena) (*Node, bool) {
	if p == nil || p.language == nil || start >= end || int(end) > len(source) {
		return nil, false
	}
	start, end = rustTrimSpaceBounds(source, start, end)
	if start >= end {
		return nil, false
	}
	var inner *Node
	if pipePos, ok := rustFindTopLevelByte(source, start, end, '|'); ok {
		leftStart, leftEnd := rustTrimSpaceBounds(source, start, pipePos)
		rightStart, rightEnd := rustTrimSpaceBounds(source, pipePos+1, end)
		if leftStart < leftEnd && rightStart < rightEnd && leftEnd-leftStart == 1 && source[leftStart] == '_' {
			left := rustBuildRustEmptyOrPatternNode(arena, source, p.language, leftStart, leftEnd)
			children := []*Node{left}
			if !(rightEnd-rightStart == 1 && source[rightStart] == '_') {
				right, ok := rustBuildRustTupleStructPatternNodeFromRange(source, rightStart, rightEnd, p, arena)
				if !ok {
					return nil, false
				}
				children = append(children, right)
			}
			orPatternSym, ok := symbolByName(p.language, "or_pattern")
			if !ok {
				return nil, false
			}
			inner = newParentNodeInArena(
				arena,
				orPatternSym,
				rustNamedForSymbol(p.language, orPatternSym),
				children,
				nil,
				0,
			)
			inner.startByte = start
			inner.startPoint = advancePointByBytes(Point{}, source[:start])
			inner.endByte = end
			inner.endPoint = advancePointByBytes(Point{}, source[:end])
		}
	}
	if inner == nil {
		node, ok := rustRecoverPatternNodeFromRange(source, start, end, p, arena)
		if !ok {
			return nil, false
		}
		inner = node
	}
	matchPatternSym, ok := symbolByName(p.language, "match_pattern")
	if !ok {
		return nil, false
	}
	pattern := newParentNodeInArena(
		arena,
		matchPatternSym,
		rustNamedForSymbol(p.language, matchPatternSym),
		[]*Node{inner},
		nil,
		0,
	)
	pattern.startByte = start
	pattern.startPoint = advancePointByBytes(Point{}, source[:start])
	pattern.endByte = end
	pattern.endPoint = advancePointByBytes(Point{}, source[:end])
	return pattern, true
}

func rustBuildRustTupleStructPatternNodeFromRange(source []byte, start, end uint32, p *Parser, arena *nodeArena) (*Node, bool) {
	if p == nil || p.language == nil || start >= end || int(end) > len(source) {
		return nil, false
	}
	start, end = rustTrimSpaceBounds(source, start, end)
	openParen, ok := rustFindTopLevelByte(source, start, end, '(')
	if !ok {
		return nil, false
	}
	closeParen := rustFindMatchingDelimiter(source, int(openParen), '(', ')')
	if closeParen < 0 || uint32(closeParen+1) != end {
		return nil, false
	}
	nameStart, nameEnd := rustTrimSpaceBounds(source, start, openParen)
	argStart, argEnd := rustTrimSpaceBounds(source, openParen+1, uint32(closeParen))
	if nameStart >= nameEnd || argStart >= argEnd {
		return nil, false
	}
	identifierSym, ok := symbolByName(p.language, "identifier")
	if !ok {
		return nil, false
	}
	tupleStructSym, ok := symbolByName(p.language, "tuple_struct_pattern")
	if !ok {
		return nil, false
	}
	name := newLeafNodeInArena(
		arena,
		identifierSym,
		rustNamedForSymbol(p.language, identifierSym),
		nameStart,
		nameEnd,
		advancePointByBytes(Point{}, source[:nameStart]),
		advancePointByBytes(Point{}, source[:nameEnd]),
	)
	arg, ok := rustBuildRecoveredValueNode(arena, source, p.language, argStart, argEnd)
	if !ok {
		return nil, false
	}
	pattern := newParentNodeInArena(
		arena,
		tupleStructSym,
		rustNamedForSymbol(p.language, tupleStructSym),
		[]*Node{name, arg},
		nil,
		0,
	)
	pattern.startByte = start
	pattern.startPoint = advancePointByBytes(Point{}, source[:start])
	pattern.endByte = end
	pattern.endPoint = advancePointByBytes(Point{}, source[:end])
	return pattern, true
}

func rustBuildRustEmptyOrPatternNode(arena *nodeArena, source []byte, lang *Language, start, end uint32) *Node {
	orPatternSym, ok := symbolByName(lang, "or_pattern")
	if !ok {
		return nil
	}
	node := newParentNodeInArena(
		arena,
		orPatternSym,
		rustNamedForSymbol(lang, orPatternSym),
		nil,
		nil,
		0,
	)
	node.startByte = start
	node.startPoint = advancePointByBytes(Point{}, source[:start])
	node.endByte = end
	node.endPoint = advancePointByBytes(Point{}, source[:end])
	return node
}
