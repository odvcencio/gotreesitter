package gotreesitter

import "bytes"

func rustRecoverWrappedFunctionBlockNodesFromRange(source []byte, start, end uint32, p *Parser, arena *nodeArena) ([]*Node, bool) {
	if p == nil || p.language == nil || start >= end || int(end) > len(source) {
		return nil, false
	}
	const prefix = "fn _() {\n"
	const suffix = "\n}\n"
	wrapped := make([]byte, 0, len(prefix)+int(end-start)+len(suffix))
	wrapped = append(wrapped, prefix...)
	wrapped = append(wrapped, source[start:end]...)
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
	return rustExtractRecoveredFunctionBlockNodes(offsetRoot, p.language, arena)
}

func rustExtractRecoveredFunctionBlockNodes(root *Node, lang *Language, arena *nodeArena) ([]*Node, bool) {
	if root == nil || lang == nil {
		return nil, false
	}
	var block *Node
	var walk func(*Node)
	walk = func(n *Node) {
		if n == nil || block != nil {
			return
		}
		if n.Type(lang) == "function_item" {
			for i := 0; i < n.NamedChildCount(); i++ {
				child := n.NamedChild(i)
				if child != nil && child.Type(lang) == "block" {
					block = child
					return
				}
			}
		}
		for i := 0; i < n.ChildCount(); i++ {
			walk(n.Child(i))
		}
	}
	walk(root)
	if block == nil {
		return nil, false
	}
	out := make([]*Node, 0, block.NamedChildCount())
	for i := 0; i < block.NamedChildCount(); i++ {
		child := block.NamedChild(i)
		if child == nil {
			continue
		}
		if arena != nil {
			out = append(out, cloneTreeNodesIntoArena(child, arena))
		} else {
			out = append(out, child)
		}
	}
	return out, true
}

func rustStatementLikeSpansInRange(source []byte, start, end uint32) [][2]uint32 {
	start = rustSkipSpaceBytes(source, start)
	if start >= end {
		return nil
	}
	var spans [][2]uint32
	stmtStart := start
	braceDepth := 0
	parenDepth := 0
	bracketDepth := 0
	inString := false
	escaped := false
	inLineComment := false
	inBlockComment := false
	for i := start; i < end; i++ {
		b := source[i]
		if inLineComment {
			if b == '\n' {
				inLineComment = false
			}
			continue
		}
		if inBlockComment {
			if b == '*' && i+1 < end && source[i+1] == '/' {
				inBlockComment = false
				i++
			}
			continue
		}
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
		if b == '/' && i+1 < end {
			if source[i+1] == '/' {
				inLineComment = true
				i++
				continue
			}
			if source[i+1] == '*' {
				inBlockComment = true
				i++
				continue
			}
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
					if next >= end || source[next] != ';' {
						spans = append(spans, [2]uint32{stmtStart, i + 1})
						stmtStart = next
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
				spans = append(spans, [2]uint32{stmtStart, i + 1})
				stmtStart = rustSkipSpaceBytes(source, i+1)
			}
		}
	}
	stmtStart = rustSkipSpaceBytes(source, stmtStart)
	if stmtStart < end {
		tailStart, tailEnd := rustTrimSpaceBounds(source, stmtStart, end)
		if tailStart < tailEnd {
			spans = append(spans, [2]uint32{tailStart, tailEnd})
		}
	}
	return spans
}

func rustRecoverPatternNodeFromRange(source []byte, start, end uint32, p *Parser, arena *nodeArena) (*Node, bool) {
	if p == nil || p.language == nil || start >= end || int(end) > len(source) {
		return nil, false
	}
	if node, ok := rustRecoverRustSpecialPatternNodeFromRange(source, start, end, p, arena); ok {
		return node, true
	}
	const prefix = "fn _("
	const suffix = ": u8) {}\n"
	pattern := bytes.TrimSpace(source[start:end])
	if len(pattern) == 0 {
		return nil, false
	}
	wrapped := make([]byte, 0, len(prefix)+len(pattern)+len(suffix))
	wrapped = append(wrapped, prefix...)
	wrapped = append(wrapped, pattern...)
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
	node := rustExtractRecoveredParameterPattern(offsetRoot, p.language, arena)
	recomputeNodePointsFromBytes(node, source)
	return node, node != nil
}

func rustRecoverRustExpressionNodeFromRange(source []byte, start, end uint32, p *Parser, arena *nodeArena) (*Node, bool) {
	if p == nil || p.language == nil || start >= end || int(end) > len(source) {
		return nil, false
	}
	if node, ok := rustRecoverRustBlockValueExpressionNodeFromRange(source, start, end, p, arena); ok {
		return node, true
	}
	if node, ok := rustRecoverRustSpecialCharactersExpressionNodeFromRange(source, start, end, p, arena); ok {
		return node, true
	}
	if node, ok := rustRecoverRustReferenceCastClosureExpressionNodeFromRange(source, start, end, p, arena); ok {
		return node, true
	}
	if node, ok := rustRecoverRustClosureExpressionNodeFromRange(source, start, end, p, arena); ok {
		return node, true
	}
	if node, ok := rustRecoverRustMatchExpressionNodeFromRange(source, start, end, p, arena); ok {
		return node, true
	}
	const prefix = "fn _() { let _ = "
	const suffix = ";\n}\n"
	expr := bytes.TrimSpace(source[start:end])
	if len(expr) == 0 {
		return nil, false
	}
	wrapped := make([]byte, 0, len(prefix)+len(expr)+len(suffix))
	wrapped = append(wrapped, prefix...)
	wrapped = append(wrapped, expr...)
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
	node := rustExtractRecoveredLetInitializer(offsetRoot, p.language, arena)
	recomputeNodePointsFromBytes(node, source)
	return node, node != nil
}

func rustExtractRecoveredParameterPattern(root *Node, lang *Language, arena *nodeArena) *Node {
	if root == nil || lang == nil {
		return nil
	}
	var walk func(*Node) *Node
	walk = func(n *Node) *Node {
		if n == nil {
			return nil
		}
		if n.Type(lang) == "parameter" {
			for i := 0; i < n.ChildCount(); i++ {
				child := n.Child(i)
				if child != nil && child.IsNamed() {
					if arena != nil {
						return cloneTreeNodesIntoArena(child, arena)
					}
					return child
				}
			}
		}
		for i := 0; i < n.ChildCount(); i++ {
			if out := walk(n.Child(i)); out != nil {
				return out
			}
		}
		return nil
	}
	return walk(root)
}

func rustExtractRecoveredLetInitializer(root *Node, lang *Language, arena *nodeArena) *Node {
	if root == nil || lang == nil {
		return nil
	}
	var walk func(*Node) *Node
	walk = func(n *Node) *Node {
		if n == nil {
			return nil
		}
		if n.Type(lang) == "let_declaration" {
			var lastNamed *Node
			for i := 0; i < n.ChildCount(); i++ {
				child := n.Child(i)
				if child == nil || !child.IsNamed() {
					continue
				}
				lastNamed = child
			}
			if lastNamed != nil {
				if arena != nil {
					return cloneTreeNodesIntoArena(lastNamed, arena)
				}
				return lastNamed
			}
		}
		for i := 0; i < n.ChildCount(); i++ {
			if out := walk(n.Child(i)); out != nil {
				return out
			}
		}
		return nil
	}
	return walk(root)
}

func rustRecoverLetStatementFromRange(source []byte, start, end uint32, p *Parser, arena *nodeArena) (*Node, bool) {
	if p == nil || p.language == nil || start >= end || int(end) > len(source) {
		return nil, false
	}
	start, end = rustTrimSpaceBounds(source, start, end)
	if start >= end || !rustHasPrefixAt(source, start, "let") {
		return nil, false
	}
	bodyEnd := end
	if source[bodyEnd-1] == ';' {
		bodyEnd--
	}
	bodyEnd = rustSkipBackwardSpaceBytes(source, bodyEnd)
	commentStart, commentEnd, hasComment := rustFindTrailingLineCommentBounds(source, start, bodyEnd)
	if hasComment {
		bodyEnd = rustSkipBackwardSpaceBytes(source, commentStart)
	}
	eqPos, ok := rustFindTopLevelByte(source, start, bodyEnd, '=')
	if !ok {
		return nil, false
	}
	patternStart := rustSkipSpaceBytes(source, start+3)
	patternEnd := rustSkipBackwardSpaceBytes(source, eqPos)
	if patternStart >= patternEnd {
		return nil, false
	}
	valueStart := rustSkipSpaceBytes(source, eqPos+1)
	valueEnd := rustSkipBackwardSpaceBytes(source, bodyEnd)
	if valueStart >= valueEnd {
		return nil, false
	}

	pattern, ok := rustRecoverPatternNodeFromRange(source, patternStart, patternEnd, p, arena)
	if !ok {
		identifierSym, hasIdentifier := symbolByName(p.language, "identifier")
		if !hasIdentifier {
			return nil, false
		}
		pattern = newLeafNodeInArena(
			arena,
			identifierSym,
			rustNamedForSymbol(p.language, identifierSym),
			patternStart,
			patternEnd,
			advancePointByBytes(Point{}, source[:patternStart]),
			advancePointByBytes(Point{}, source[:patternEnd]),
		)
	}
	value, ok := rustRecoverRustExpressionNodeFromRange(source, valueStart, valueEnd, p, arena)
	if !ok {
		return nil, false
	}
	letDeclSym, ok := symbolByName(p.language, "let_declaration")
	if !ok {
		return nil, false
	}
	children := []*Node{pattern, value}
	if hasComment {
		if comment, ok := rustBuildRecoveredTriviaNode(arena, source, p.language, commentStart, commentEnd, "line_comment"); ok {
			children = append(children, comment)
		}
	}
	letDecl := newParentNodeInArena(
		arena,
		letDeclSym,
		rustNamedForSymbol(p.language, letDeclSym),
		children,
		nil,
		0,
	)
	letDecl.startByte = start
	letDecl.startPoint = advancePointByBytes(Point{}, source[:start])
	letDecl.endByte = end
	letDecl.endPoint = advancePointByBytes(Point{}, source[:end])
	return letDecl, true
}

func rustRecoverImplItemFromRange(source []byte, start, end uint32, p *Parser, arena *nodeArena) (*Node, bool) {
	if p == nil || p.language == nil || start >= end || int(end) > len(source) {
		return nil, false
	}
	start, end = rustTrimSpaceBounds(source, start, end)
	if start >= end || !rustHasPrefixAt(source, start, "impl") {
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
	headerNodes, ok := rustRecoverImplHeaderNodesFromRange(source, start, openBrace, p, arena)
	if !ok || len(headerNodes) < 2 {
		return nil, false
	}
	bodyNodes, ok := rustRecoverImplBodyNodesFromRange(source, openBrace+1, uint32(closeBrace), p, arena)
	if !ok {
		return nil, false
	}
	declListSym, ok := symbolByName(p.language, "declaration_list")
	if !ok {
		return nil, false
	}
	implItemSym, ok := symbolByName(p.language, "impl_item")
	if !ok {
		return nil, false
	}
	declList := newParentNodeInArena(
		arena,
		declListSym,
		rustNamedForSymbol(p.language, declListSym),
		bodyNodes,
		nil,
		0,
	)
	declList.startByte = openBrace
	declList.startPoint = advancePointByBytes(Point{}, source[:openBrace])
	declList.endByte = uint32(closeBrace + 1)
	declList.endPoint = advancePointByBytes(Point{}, source[:closeBrace+1])

	children := append(headerNodes, declList)
	implItem := newParentNodeInArena(
		arena,
		implItemSym,
		rustNamedForSymbol(p.language, implItemSym),
		children,
		nil,
		0,
	)
	implItem.startByte = start
	implItem.startPoint = advancePointByBytes(Point{}, source[:start])
	implItem.endByte = end
	implItem.endPoint = advancePointByBytes(Point{}, source[:end])
	return implItem, true
}

func rustRecoverImplHeaderNodesFromRange(source []byte, start, blockStart uint32, p *Parser, arena *nodeArena) ([]*Node, bool) {
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
	return rustExtractRecoveredImplHeaderNodes(offsetRoot, p.language, arena)
}

func rustExtractRecoveredImplHeaderNodes(root *Node, lang *Language, arena *nodeArena) ([]*Node, bool) {
	if root == nil || lang == nil {
		return nil, false
	}
	var implItem *Node
	var walk func(*Node)
	walk = func(n *Node) {
		if n == nil || implItem != nil {
			return
		}
		if n.Type(lang) == "impl_item" {
			implItem = n
			return
		}
		for i := 0; i < n.ChildCount(); i++ {
			walk(n.Child(i))
		}
	}
	walk(root)
	if implItem == nil {
		return nil, false
	}
	out := make([]*Node, 0, implItem.ChildCount())
	for i := 0; i < implItem.ChildCount(); i++ {
		child := implItem.Child(i)
		if child == nil || child.Type(lang) == "declaration_list" {
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

func rustRecoverImplBodyNodesFromRange(source []byte, start, end uint32, p *Parser, arena *nodeArena) ([]*Node, bool) {
	spans := rustStatementLikeSpansInRange(source, start, end)
	if len(spans) == 0 && bytes.TrimSpace(source[start:end]) != nil && len(bytes.TrimSpace(source[start:end])) > 0 {
		return nil, false
	}
	out := make([]*Node, 0, len(spans))
	for _, span := range spans {
		nodes, ok := rustRecoverImplBodyChunkNodesFromRange(source, span[0], span[1], p, arena)
		if !ok || len(nodes) == 0 {
			return nil, false
		}
		out = append(out, nodes...)
	}
	return out, true
}

func rustRecoverImplBodyChunkNodesFromRange(source []byte, start, end uint32, p *Parser, arena *nodeArena) ([]*Node, bool) {
	if p == nil || p.language == nil || start >= end || int(end) > len(source) {
		return nil, false
	}
	if nodes, ok := rustRecoverWrappedImplBodyNodesFromRange(source, start, end, p, arena); ok {
		return nodes, true
	}
	trimmedStart, trimmedEnd := rustTrimSpaceBounds(source, start, end)
	if trimmedStart >= trimmedEnd {
		return nil, false
	}
	if _, ok := rustFunctionKeywordStart(source, trimmedStart, trimmedEnd); ok {
		if node, ok := rustRecoverFunctionItemFromRange(source, trimmedStart, trimmedEnd, p, arena); ok {
			return []*Node{node}, true
		}
	}
	return nil, false
}

func rustRecoverWrappedImplBodyNodesFromRange(source []byte, start, end uint32, p *Parser, arena *nodeArena) ([]*Node, bool) {
	if p == nil || p.language == nil || start >= end || int(end) > len(source) {
		return nil, false
	}
	const prefix = "impl __Trait for __Type {\n"
	const suffix = "\n}\n"
	wrapped := make([]byte, 0, len(prefix)+int(end-start)+len(suffix))
	wrapped = append(wrapped, prefix...)
	wrapped = append(wrapped, source[start:end]...)
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
	return rustExtractRecoveredImplBodyNodes(offsetRoot, p.language, arena)
}

func rustExtractRecoveredImplBodyNodes(root *Node, lang *Language, arena *nodeArena) ([]*Node, bool) {
	if root == nil || lang == nil {
		return nil, false
	}
	var declList *Node
	var walk func(*Node)
	walk = func(n *Node) {
		if n == nil || declList != nil {
			return
		}
		if n.Type(lang) == "impl_item" {
			for i := 0; i < n.NamedChildCount(); i++ {
				child := n.NamedChild(i)
				if child != nil && child.Type(lang) == "declaration_list" {
					declList = child
					return
				}
			}
		}
		for i := 0; i < n.ChildCount(); i++ {
			walk(n.Child(i))
		}
	}
	walk(root)
	if declList == nil {
		return nil, false
	}
	out := make([]*Node, 0, declList.NamedChildCount())
	for i := 0; i < declList.NamedChildCount(); i++ {
		child := declList.NamedChild(i)
		if child == nil {
			continue
		}
		if arena != nil {
			out = append(out, cloneTreeNodesIntoArena(child, arena))
		} else {
			out = append(out, child)
		}
	}
	return out, true
}

func rustRecoverLoopStatementFromRange(source []byte, start, end uint32, p *Parser, arena *nodeArena) (*Node, bool) {
	if p == nil || p.language == nil || start >= end || int(end) > len(source) {
		return nil, false
	}
	start, end = rustTrimSpaceBounds(source, start, end)
	if start >= end || !rustHasPrefixAt(source, start, "loop") {
		return nil, false
	}
	openBrace := rustSkipSpaceBytes(source, start+4)
	if openBrace >= end || source[openBrace] != '{' {
		return nil, false
	}
	closeBrace := rustFindMatchingDelimiter(source, int(openBrace), '{', '}')
	if closeBrace < 0 || uint32(closeBrace+1) != end {
		return nil, false
	}
	bodyStart := rustSkipSpaceBytes(source, openBrace+1)
	if !rustHasPrefixAt(source, bodyStart, "if") {
		return nil, false
	}
	condStart := rustSkipSpaceBytes(source, bodyStart+2)
	if !rustHasPrefixAt(source, condStart, "break") {
		return nil, false
	}
	breakEnd := condStart + 5
	ifBlockStart := rustSkipSpaceBytes(source, breakEnd)
	if ifBlockStart >= end || source[ifBlockStart] != '{' {
		return nil, false
	}
	ifBlockEnd := rustFindMatchingDelimiter(source, int(ifBlockStart), '{', '}')
	if ifBlockEnd < 0 {
		return nil, false
	}
	remainderStart := rustSkipSpaceBytes(source, uint32(ifBlockEnd+1))
	if remainderStart != uint32(closeBrace) {
		return nil, false
	}
	exprStmtSym, ok := symbolByName(p.language, "expression_statement")
	if !ok {
		return nil, false
	}
	loopExprSym, ok := symbolByName(p.language, "loop_expression")
	if !ok {
		return nil, false
	}
	blockSym, ok := symbolByName(p.language, "block")
	if !ok {
		return nil, false
	}
	ifExprSym, ok := symbolByName(p.language, "if_expression")
	if !ok {
		return nil, false
	}
	breakExprSym, ok := symbolByName(p.language, "break_expression")
	if !ok {
		return nil, false
	}
	breakExpr := newParentNodeInArena(
		arena,
		breakExprSym,
		rustNamedForSymbol(p.language, breakExprSym),
		nil,
		nil,
		0,
	)
	breakExpr.startByte = condStart
	breakExpr.startPoint = advancePointByBytes(Point{}, source[:condStart])
	breakExpr.endByte = breakEnd
	breakExpr.endPoint = advancePointByBytes(Point{}, source[:breakEnd])

	ifBlock := newParentNodeInArena(
		arena,
		blockSym,
		rustNamedForSymbol(p.language, blockSym),
		nil,
		nil,
		0,
	)
	ifBlock.startByte = ifBlockStart
	ifBlock.startPoint = advancePointByBytes(Point{}, source[:ifBlockStart])
	ifBlock.endByte = uint32(ifBlockEnd + 1)
	ifBlock.endPoint = advancePointByBytes(Point{}, source[:ifBlockEnd+1])

	ifExpr := newParentNodeInArena(
		arena,
		ifExprSym,
		rustNamedForSymbol(p.language, ifExprSym),
		[]*Node{breakExpr, ifBlock},
		nil,
		0,
	)
	ifExpr.startByte = bodyStart
	ifExpr.startPoint = advancePointByBytes(Point{}, source[:bodyStart])
	ifExpr.endByte = uint32(ifBlockEnd + 1)
	ifExpr.endPoint = advancePointByBytes(Point{}, source[:ifBlockEnd+1])

	innerStmt := newParentNodeInArena(
		arena,
		exprStmtSym,
		rustNamedForSymbol(p.language, exprStmtSym),
		[]*Node{ifExpr},
		nil,
		0,
	)
	innerStmt.startByte = bodyStart
	innerStmt.startPoint = ifExpr.startPoint
	innerStmt.endByte = uint32(ifBlockEnd + 1)
	innerStmt.endPoint = ifExpr.endPoint

	loopBlock := newParentNodeInArena(
		arena,
		blockSym,
		rustNamedForSymbol(p.language, blockSym),
		[]*Node{innerStmt},
		nil,
		0,
	)
	loopBlock.startByte = openBrace
	loopBlock.startPoint = advancePointByBytes(Point{}, source[:openBrace])
	loopBlock.endByte = uint32(closeBrace + 1)
	loopBlock.endPoint = advancePointByBytes(Point{}, source[:closeBrace+1])

	loopExpr := newParentNodeInArena(
		arena,
		loopExprSym,
		rustNamedForSymbol(p.language, loopExprSym),
		[]*Node{loopBlock},
		nil,
		0,
	)
	loopExpr.startByte = start
	loopExpr.startPoint = advancePointByBytes(Point{}, source[:start])
	loopExpr.endByte = uint32(closeBrace + 1)
	loopExpr.endPoint = advancePointByBytes(Point{}, source[:closeBrace+1])

	stmt := newParentNodeInArena(
		arena,
		exprStmtSym,
		rustNamedForSymbol(p.language, exprStmtSym),
		[]*Node{loopExpr},
		nil,
		0,
	)
	stmt.startByte = start
	stmt.startPoint = loopExpr.startPoint
	stmt.endByte = end
	stmt.endPoint = loopExpr.endPoint
	return stmt, true
}

func rustFindTrailingLineCommentBounds(source []byte, start, end uint32) (uint32, uint32, bool) {
	if start >= end || int(end) > len(source) {
		return 0, 0, false
	}
	var commentStart, commentEnd uint32
	braceDepth := 0
	parenDepth := 0
	bracketDepth := 0
	inString := false
	escaped := false
	inBlockComment := false
	for i := start; i < end; i++ {
		b := source[i]
		if inBlockComment {
			if b == '*' && i+1 < end && source[i+1] == '/' {
				inBlockComment = false
				i++
			}
			continue
		}
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
		if b == '/' && i+1 < end {
			if source[i+1] == '*' {
				inBlockComment = true
				i++
				continue
			}
			if source[i+1] == '/' && braceDepth == 0 && parenDepth == 0 && bracketDepth == 0 {
				commentStart = i
				commentEnd = end
				for j := i + 2; j < end; j++ {
					if source[j] == '\n' || source[j] == '\r' {
						commentEnd = j
						break
					}
				}
				break
			}
		}
		switch b {
		case '"':
			inString = true
		case '{':
			braceDepth++
		case '}':
			if braceDepth > 0 {
				braceDepth--
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
		}
	}
	if commentStart == 0 && !rustHasPrefixAt(source, start, "//") {
		return 0, 0, false
	}
	return commentStart, commentEnd, commentEnd > commentStart
}

func rustSkipBackwardSpaceBytes(source []byte, pos uint32) uint32 {
	for pos > 0 && rustIsSpaceByte(source[pos-1]) {
		pos--
	}
	return pos
}
