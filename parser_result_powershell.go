package gotreesitter

import "bytes"

func normalizePowerShellProgramShape(root *Node, source []byte, lang *Language) {
	if root == nil || lang == nil || lang.Name != "powershell" || root.Type(lang) != "ERROR" || len(root.children) < 4 || len(source) == 0 {
		return
	}
	programSym, ok := symbolByName(lang, "program")
	if !ok {
		return
	}
	statementListSym, ok := symbolByName(lang, "statement_list")
	if !ok {
		return
	}
	functionStatementSym, ok := symbolByName(lang, "function_statement")
	if !ok {
		return
	}
	functionSym, ok := symbolByName(lang, "function")
	if !ok {
		return
	}
	scriptBlockSym, ok := symbolByName(lang, "script_block")
	if !ok {
		return
	}
	scriptBlockBodySym, ok := symbolByName(lang, "script_block_body")
	if !ok {
		return
	}
	closeBraceSym, ok := symbolByName(lang, "}")
	if !ok {
		return
	}
	pipelineSym, ok := symbolByName(lang, "pipeline")
	if !ok {
		return
	}
	pipelineChainSym, ok := symbolByName(lang, "pipeline_chain")
	if !ok {
		return
	}
	commandSym, ok := symbolByName(lang, "command")
	if !ok {
		return
	}
	commandNameSym, ok := symbolByName(lang, "command_name")
	if !ok {
		return
	}
	commandElementsSym, ok := symbolByName(lang, "command_elements")
	if !ok {
		return
	}
	commandArgSepSym, ok := symbolByName(lang, "command_argument_sep")
	if !ok {
		return
	}
	commandParameterSym, ok := symbolByName(lang, "command_parameter")
	if !ok {
		return
	}
	arrayLiteralSym, ok := symbolByName(lang, "array_literal_expression")
	if !ok {
		return
	}
	unaryExprSym, ok := symbolByName(lang, "unary_expression")
	if !ok {
		return
	}
	variableSym, ok := symbolByName(lang, "variable")
	if !ok {
		return
	}
	stringLiteralSym, ok := symbolByName(lang, "string_literal")
	if !ok {
		return
	}
	expandableStringSym, ok := symbolByName(lang, "expandable_string_literal")
	if !ok {
		return
	}
	genericTokenSym, ok := symbolByName(lang, "generic_token")
	if !ok {
		return
	}
	spaceSym, ok := symbolByName(lang, " ")
	if !ok {
		return
	}

	statementListIdx := -1
	for i, child := range root.children {
		if child != nil && child.Type(lang) == "statement_list" {
			statementListIdx = i
			break
		}
	}
	if statementListIdx < 0 {
		return
	}
	statementList := cloneNodeInArena(root.ownerArena, root.children[statementListIdx])
	children := make([]*Node, 0, len(statementList.children)+len(root.children)-statementListIdx)
	children = append(children, statementList.children...)
	statementListEnd := statementList.endByte
	rebuiltTail := false
	for tailIdx := statementListIdx + 1; tailIdx < len(root.children); {
		child := root.children[tailIdx]
		if child == nil {
			tailIdx++
			continue
		}
		if child.IsExtra() {
			if child.endByte > statementListEnd {
				children = append(children, child)
				statementListEnd = child.endByte
			}
			tailIdx++
			continue
		}
		spill := root.children[tailIdx:]
		if !powerShellLooksLikeSpilledFunction(spill, lang) {
			pipelines := buildPowerShellTrailingPipelines(
				root.ownerArena, source, lang, child.startByte, root.endByte,
				pipelineSym, pipelineChainSym, commandSym, commandNameSym, commandElementsSym,
				commandArgSepSym, commandParameterSym, arrayLiteralSym, unaryExprSym,
				variableSym, stringLiteralSym, expandableStringSym, genericTokenSym, spaceSym,
			)
			if len(pipelines) == 0 {
				return
			}
			children = append(children, pipelines...)
			statementListEnd = pipelines[len(pipelines)-1].endByte
			rebuiltTail = true
			break
		}
		openBrace := spill[2]
		if openBrace == nil {
			return
		}
		closeBracePos := findMatchingBraceByte(source, int(openBrace.startByte), len(source))
		if closeBracePos < 0 {
			return
		}
		functionStatement := buildPowerShellSpilledFunctionStatement(
			root.ownerArena, source, lang, spill, closeBracePos,
			functionStatementSym, functionSym, scriptBlockSym, scriptBlockBodySym, statementListSym, closeBraceSym,
		)
		if functionStatement == nil {
			return
		}
		children = append(children, functionStatement)
		statementListEnd = functionStatement.endByte
		rebuiltTail = true
		for tailIdx < len(root.children) {
			next := root.children[tailIdx]
			if next == nil || next.startByte >= uint32(closeBracePos+1) {
				break
			}
			tailIdx++
		}
	}
	if !rebuiltTail {
		return
	}
	children = cloneNodeSliceIfArena(root.ownerArena, children)
	statementList.children = children
	statementList.clearFieldMetadata()
	statementList.symbol = statementListSym
	statementList.setNamed(symbolIsNamed(lang, statementListSym))
	statementList.setHasError(true)
	extendNodeEndTo(statementList, statementListEnd, source)

	out := make([]*Node, 0, statementListIdx+1)
	out = append(out, root.children[:statementListIdx]...)
	out = append(out, statementList)
	out = cloneNodeSliceIfArena(root.ownerArena, out)
	root.children = out
	root.clearFieldMetadata()
	retagResultRoot(root, programSym, symbolIsNamed(lang, programSym))
	root.setHasError(true)
	extendNodeEndTo(root, statementListEnd, source)
	if root.endByte < uint32(len(source)) && bytesAreTrivia(source[root.endByte:]) {
		extendNodeEndTo(root, uint32(len(source)), source)
	}
}

func normalizePowerShellErrorProgramRoot(root *Node, lang *Language) {
	if root == nil || lang == nil || lang.Name != "powershell" || root.Type(lang) != "ERROR" || len(root.children) == 0 {
		return
	}
	programSym, ok := symbolByName(lang, "program")
	if !ok {
		return
	}
	sawStatementList := false
	for _, child := range root.children {
		if child == nil {
			return
		}
		switch child.Type(lang) {
		case "comment", "param_block":
		case "statement_list":
			sawStatementList = true
		default:
			return
		}
	}
	if !sawStatementList {
		return
	}
	retagResultRoot(root, programSym, symbolIsNamed(lang, programSym))
}

func normalizePowerShellPathCommandNameVariables(root *Node, source []byte, lang *Language) {
	if root == nil || lang == nil || lang.Name != "powershell" || bytes.IndexByte(source, '&') < 0 {
		return
	}
	pathCommandNameSym, pathCommandNameNamed, ok := symbolMeta(lang, "path_command_name")
	if !ok {
		return
	}
	var walk func(*Node)
	walk = func(n *Node) {
		if n == nil {
			return
		}
		if n.Type(lang) != "path_command_name" {
			for i, child := range n.children {
				if child != nil && child.Type(lang) == "variable" && powerShellVariableFollowsInvocationOperator(source, child) {
					pathChildren := []*Node{child}
					if root.ownerArena != nil {
						buf := root.ownerArena.allocNodeSlice(1)
						buf[0] = child
						pathChildren = buf
					}
					n.children[i] = newParentNodeInArena(root.ownerArena, pathCommandNameSym, pathCommandNameNamed, pathChildren, nil, 0)
				}
			}
		}
		for _, child := range n.children {
			walk(child)
		}
	}
	walk(root)
}

func normalizePowerShellEnumStatementKeywordSpans(root *Node, source []byte, lang *Language) {
	if root == nil || lang == nil || lang.Name != "powershell" || !bytes.Contains(source, []byte("enum ")) {
		return
	}
	walkResultTree(root, func(n *Node) {
		if n == nil || n.Type(lang) != "enum_statement" || n.startByte < 5 || int(n.startByte) > len(source) {
			return
		}
		keywordStart := int(n.startByte) - len("enum ")
		if keywordStart < 0 || string(source[keywordStart:n.startByte]) != "enum " {
			return
		}
		if keywordStart > 0 {
			switch source[keywordStart-1] {
			case ' ', '\t', '\r', '\n', ';', '{', '}':
			default:
				return
			}
		}
		n.startByte = uint32(keywordStart)
		n.startPoint = advancePointByBytes(Point{}, source[:keywordStart])
	})
}

func powerShellVariableFollowsInvocationOperator(source []byte, variable *Node) bool {
	if variable == nil || variable.startByte >= variable.endByte || int(variable.startByte) >= len(source) || source[variable.startByte] != '$' {
		return false
	}
	for i := int(variable.startByte) - 1; i >= 0; i-- {
		switch source[i] {
		case ' ', '\t', '\r', '\n':
			continue
		default:
			return source[i] == '&'
		}
	}
	return false
}

func powerShellLooksLikeSpilledFunction(nodes []*Node, lang *Language) bool {
	if len(nodes) < 4 || lang == nil {
		return false
	}
	head := nodes[0]
	if head == nil || head.Type(lang) != "ERROR" || resultChildCount(head) != 1 {
		return false
	}
	functionLeaf := resultChildAt(head, 0)
	if functionLeaf == nil || functionLeaf.Type(lang) != "function" {
		return false
	}
	return nodes[1] != nil && nodes[1].Type(lang) == "function_name" &&
		nodes[2] != nil && nodes[2].Type(lang) == "{"
}

func buildPowerShellSpilledFunctionStatement(arena *nodeArena, source []byte, lang *Language, nodes []*Node, closeBracePos int, functionStatementSym, functionSym, scriptBlockSym, scriptBlockBodySym, statementListSym, closeBraceSym Symbol) *Node {
	if len(nodes) < 4 || nodes[0] == nil || nodes[1] == nil || nodes[2] == nil {
		return nil
	}
	functionLeaf := resultChildAt(nodes[0], 0)
	if functionLeaf == nil {
		return nil
	}
	functionName := nodes[1]
	openBrace := nodes[2]
	scriptEnd := closeBracePos
	for scriptEnd > int(openBrace.endByte) {
		switch source[scriptEnd-1] {
		case ' ', '\t', '\r', '\n':
			scriptEnd--
		default:
			goto trimmed
		}
	}
trimmed:
	scriptChildren := make([]*Node, 0, len(nodes))
	for _, child := range nodes[3:] {
		if child == nil {
			continue
		}
		if int(child.startByte) >= scriptEnd {
			break
		}
		if int(child.endByte) <= scriptEnd {
			scriptChildren = append(scriptChildren, child)
			continue
		}
		truncated := cloneNodeInArena(arena, child)
		truncated.children = nil
		truncated.clearFieldMetadata()
		truncated.endByte = uint32(scriptEnd)
		truncated.endPoint = advancePointByBytes(truncated.startPoint, source[truncated.startByte:uint32(scriptEnd)])
		scriptChildren = append(scriptChildren, truncated)
		break
	}
	if len(scriptChildren) == 0 {
		return nil
	}
	if len(scriptChildren) > 0 && scriptChildren[0] != nil && scriptChildren[0].Type(lang) == "param_block" {
		structured := make([]*Node, 0, len(scriptChildren))
		structured = append(structured, scriptChildren[0])
		idx := 1
		if idx < len(scriptChildren) && scriptChildren[idx] != nil && scriptChildren[idx].Type(lang) == "_statement_terminator" {
			idx++
		}
		for idx < len(scriptChildren) && scriptChildren[idx] != nil && scriptChildren[idx].Type(lang) == "comment" {
			structured = append(structured, scriptChildren[idx])
			idx++
		}
		if idx < len(scriptChildren) {
			statementListChildren := recoverPowerShellStatementListChildren(arena, source, lang, scriptChildren[idx:], scriptEnd)
			if arena != nil {
				buf := arena.allocNodeSlice(len(statementListChildren))
				copy(buf, statementListChildren)
				statementListChildren = buf
			}
			statementListNamed := symbolIsNamed(lang, statementListSym)
			stmtList := newParentNodeInArena(arena, statementListSym, statementListNamed, statementListChildren, nil, 0)
			stmtList.setHasError(true)
			stmtList.endByte = uint32(scriptEnd)
			stmtList.endPoint = advancePointByBytes(stmtList.startPoint, source[stmtList.startByte:uint32(scriptEnd)])
			bodyChildren := []*Node{stmtList}
			if arena != nil {
				buf := arena.allocNodeSlice(1)
				buf[0] = stmtList
				bodyChildren = buf
			}
			scriptBlockBodyNamed := symbolIsNamed(lang, scriptBlockBodySym)
			body := newParentNodeInArena(arena, scriptBlockBodySym, scriptBlockBodyNamed, bodyChildren, nil, 0)
			body.setHasError(true)
			for fieldIdx, fieldName := range lang.FieldNames {
				if fieldName != "statement_list" {
					continue
				}
				ensureNodeFieldStorage(body, len(body.children))
				body.fieldIDs()[0] = FieldID(fieldIdx)
				body.fieldSources()[0] = fieldSourceDirect
				break
			}
			structured = append(structured, body)
		}
		scriptChildren = structured
	}
	if arena != nil {
		buf := arena.allocNodeSlice(len(scriptChildren))
		copy(buf, scriptChildren)
		scriptChildren = buf
	}
	scriptBlockNamed := symbolIsNamed(lang, scriptBlockSym)
	scriptBlock := newParentNodeInArena(arena, scriptBlockSym, scriptBlockNamed, scriptChildren, nil, 0)
	scriptBlock.setHasError(true)
	for i, child := range scriptBlock.children {
		if child == nil || child.Type(lang) != "script_block_body" {
			continue
		}
		for fieldIdx, fieldName := range lang.FieldNames {
			if fieldName != "script_block_body" {
				continue
			}
			ensureNodeFieldStorage(scriptBlock, len(scriptBlock.children))
			scriptBlock.fieldIDs()[i] = FieldID(fieldIdx)
			scriptBlock.fieldSources()[i] = fieldSourceDirect
			break
		}
		break
	}
	functionStatementNamed := symbolIsNamed(lang, functionStatementSym)
	closeBraceStart := advancePointByBytes(Point{}, source[:closeBracePos])
	closeBraceLeaf := newLeafNodeInArena(arena, closeBraceSym, false, uint32(closeBracePos), uint32(closeBracePos+1), closeBraceStart, advancePointByBytes(closeBraceStart, source[closeBracePos:closeBracePos+1]))
	children := []*Node{functionLeaf, functionName, openBrace, scriptBlock, closeBraceLeaf}
	if arena != nil {
		buf := arena.allocNodeSlice(len(children))
		copy(buf, children)
		children = buf
	}
	fn := newParentNodeInArena(arena, functionStatementSym, functionStatementNamed, children, nil, 0)
	fn.setHasError(true)
	if functionLeaf.symbol != functionSym {
		functionLeaf = cloneNodeInArena(arena, functionLeaf)
		functionLeaf.symbol = functionSym
		functionLeaf.setNamed(symbolIsNamed(lang, functionSym))
		fn.children[0] = functionLeaf
	}
	extendNodeEndTo(fn, uint32(closeBracePos+1), source)
	return fn
}

func recoverPowerShellStatementListChildren(arena *nodeArena, source []byte, lang *Language, nodes []*Node, end int) []*Node {
	if len(nodes) == 0 || lang == nil || len(source) == 0 {
		return nodes
	}
	flattened := flattenPowerShellStatementListChildren(nodes, lang, nil)
	out := make([]*Node, 0, len(flattened))
	tailStart := -1
	for _, child := range flattened {
		if child == nil {
			continue
		}
		if powerShellIsStatementListChild(child, lang) {
			out = append(out, child)
			continue
		}
		tailStart = int(child.startByte)
		break
	}
	if tailStart < 0 {
		return flattened
	}
	rebuilt := buildPowerShellRecoveredStatements(arena, source, lang, tailStart, end, flattened)
	if len(rebuilt) == 0 {
		return flattened
	}
	out = append(out, rebuilt...)
	return out
}

func flattenPowerShellStatementListChildren(nodes []*Node, lang *Language, out []*Node) []*Node {
	for _, node := range nodes {
		out = flattenPowerShellStatementListChild(node, lang, out)
	}
	return out
}

func flattenPowerShellStatementListChild(node *Node, lang *Language, out []*Node) []*Node {
	if node == nil || lang == nil {
		return out
	}
	switch node.Type(lang) {
	case "_statement":
		if len(node.children) == 1 && node.children[0] != nil {
			return flattenPowerShellStatementListChild(node.children[0], lang, out)
		}
	case "statement_list_repeat1":
		for _, child := range node.children {
			out = flattenPowerShellStatementListChild(child, lang, out)
		}
		return out
	}
	return append(out, node)
}

func powerShellIsStatementListChild(node *Node, lang *Language) bool {
	if node == nil || lang == nil {
		return false
	}
	switch node.Type(lang) {
	case "comment", "pipeline", "if_statement", "try_statement", "flow_control_statement":
		return true
	default:
		return false
	}
}

func buildPowerShellRecoveredStatements(arena *nodeArena, source []byte, lang *Language, start, end int, existing []*Node) []*Node {
	if lang == nil || len(source) == 0 || start >= end {
		return nil
	}
	commentSym, commentNamed, ok := symbolMeta(lang, "comment")
	if !ok {
		return nil
	}
	out := make([]*Node, 0, 16)
	i := powerShellSkipTrivia(source, start, end)
	for i < end {
		switch {
		case source[i] == '#':
			lineEnd := powerShellLineEnd(source, i, end)
			startPoint := advancePointByBytes(Point{}, source[:i])
			comment := newLeafNodeInArena(arena, commentSym, commentNamed, uint32(i), uint32(lineEnd), startPoint, advancePointByBytes(startPoint, source[i:lineEnd]))
			comment.setExtra(true)
			out = append(out, comment)
			i = powerShellSkipTrivia(source, lineEnd, end)
		case powerShellKeywordAt(source, i, "if"):
			stmt, next := buildPowerShellRecoveredIfStatement(arena, source, lang, i, end, existing)
			if stmt == nil || next <= i {
				return out
			}
			out = append(out, stmt)
			i = powerShellSkipTrivia(source, next, end)
		case powerShellKeywordAt(source, i, "try"):
			stmt, next := buildPowerShellRecoveredTryStatement(arena, source, lang, i, end)
			if stmt == nil || next <= i {
				return out
			}
			out = append(out, stmt)
			i = powerShellSkipTrivia(source, next, end)
		case powerShellKeywordAt(source, i, "throw"):
			lineEnd := powerShellLineEnd(source, i, end)
			if stmt := buildPowerShellRecoveredFlowControlStatement(arena, source, lang, i, lineEnd); stmt != nil {
				out = append(out, stmt)
			}
			i = powerShellSkipTrivia(source, lineEnd, end)
		default:
			lineEnd := powerShellLineEnd(source, i, end)
			if stmt := buildPowerShellRecoveredPipeline(arena, source, lang, i, lineEnd); stmt != nil {
				out = append(out, stmt)
			}
			i = powerShellSkipTrivia(source, lineEnd, end)
		}
	}
	return out
}

func buildPowerShellRecoveredIfStatement(arena *nodeArena, source []byte, lang *Language, start, end int, existing []*Node) (*Node, int) {
	ifStatementSym, ifStatementNamed, ok := symbolMeta(lang, "if_statement")
	if !ok {
		return nil, 0
	}
	ifSym, ifNamed, ok := symbolMeta(lang, "if")
	if !ok {
		return nil, 0
	}
	openParenSym, _, ok := symbolMeta(lang, "(")
	if !ok {
		return nil, 0
	}
	closeParenSym, _, ok := symbolMeta(lang, ")")
	if !ok {
		return nil, 0
	}
	elseClauseSym, elseClauseNamed, ok := symbolMeta(lang, "else_clause")
	if !ok {
		return nil, 0
	}
	elseSym, elseNamed, ok := symbolMeta(lang, "else")
	if !ok {
		return nil, 0
	}
	openParen := powerShellSkipInlineSpace(source, start+len("if"), end)
	if openParen >= end || source[openParen] != '(' {
		return nil, 0
	}
	closeParen := findMatchingDelimitedByte(source, openParen, end, '(', ')')
	if closeParen < 0 {
		return nil, 0
	}
	blockOpen := powerShellSkipTrivia(source, closeParen+1, end)
	if blockOpen >= end || source[blockOpen] != '{' {
		return nil, 0
	}
	blockClose := findMatchingBraceByte(source, blockOpen, end)
	if blockClose < 0 {
		return nil, 0
	}
	condPipeline := powerShellReuseExactNode(existing, lang, "pipeline", uint32(openParen+1), uint32(closeParen))
	reusedCond := condPipeline != nil
	if condPipeline == nil {
		condPipeline = buildPowerShellRecoveredConditionPipeline(arena, source, lang, openParen+1, closeParen)
	}
	if condPipeline == nil {
		return nil, 0
	}
	thenBlock := powerShellReuseExactNode(existing, lang, "statement_block", uint32(blockOpen), uint32(blockClose+1))
	reusedThenBlock := thenBlock != nil
	if thenBlock == nil {
		thenBlock = buildPowerShellRecoveredStatementBlock(arena, source, lang, blockOpen, blockClose)
	}
	if thenBlock == nil {
		return nil, 0
	}
	children := make([]*Node, 0, 6)
	children = append(children,
		newLeafNodeInArena(arena, ifSym, ifNamed, uint32(start), uint32(start+len("if")), advancePointByBytes(Point{}, source[:start]), advancePointByBytes(advancePointByBytes(Point{}, source[:start]), source[start:start+len("if")])),
		newLeafNodeInArena(arena, openParenSym, false, uint32(openParen), uint32(openParen+1), advancePointByBytes(Point{}, source[:openParen]), advancePointByBytes(advancePointByBytes(Point{}, source[:openParen]), source[openParen:openParen+1])),
		condPipeline,
		newLeafNodeInArena(arena, closeParenSym, false, uint32(closeParen), uint32(closeParen+1), advancePointByBytes(Point{}, source[:closeParen]), advancePointByBytes(advancePointByBytes(Point{}, source[:closeParen]), source[closeParen:closeParen+1])),
		thenBlock,
	)
	next := powerShellSkipTrivia(source, blockClose+1, end)
	if powerShellKeywordAt(source, next, "else") {
		elseStart := next
		elseBlockOpen := powerShellSkipTrivia(source, elseStart+len("else"), end)
		if elseBlockOpen >= end || source[elseBlockOpen] != '{' {
			return nil, 0
		}
		elseBlockClose := findMatchingBraceByte(source, elseBlockOpen, end)
		if elseBlockClose < 0 {
			return nil, 0
		}
		elseBlock := buildPowerShellRecoveredStatementBlock(arena, source, lang, elseBlockOpen, elseBlockClose)
		if elseBlock == nil {
			return nil, 0
		}
		elseChildren := []*Node{
			newLeafNodeInArena(arena, elseSym, elseNamed, uint32(elseStart), uint32(elseStart+len("else")), advancePointByBytes(Point{}, source[:elseStart]), advancePointByBytes(advancePointByBytes(Point{}, source[:elseStart]), source[elseStart:elseStart+len("else")])),
			elseBlock,
		}
		if arena != nil {
			buf := arena.allocNodeSlice(len(elseChildren))
			copy(buf, elseChildren)
			elseChildren = buf
		}
		children = append(children, newParentNodeInArena(arena, elseClauseSym, elseClauseNamed, elseChildren, nil, 0))
		next = elseBlockClose + 1
	}
	if arena != nil {
		buf := arena.allocNodeSlice(len(children))
		copy(buf, children)
		children = buf
	}
	stmt := newParentNodeInArena(arena, ifStatementSym, ifStatementNamed, children, nil, 0)
	for fieldIdx, fieldName := range lang.FieldNames {
		switch fieldName {
		case "condition":
			ensureNodeFieldStorage(stmt, len(stmt.children))
			stmt.fieldIDs()[2] = FieldID(fieldIdx)
			stmt.fieldSources()[2] = fieldSourceDirect
		case "else_clause":
			if len(stmt.children) > 5 && stmt.children[5] != nil && stmt.children[5].Type(lang) == "else_clause" {
				ensureNodeFieldStorage(stmt, len(stmt.children))
				stmt.fieldIDs()[5] = FieldID(fieldIdx)
				stmt.fieldSources()[5] = fieldSourceDirect
			}
		}
	}
	if reusedCond || reusedThenBlock {
		stmt.setHasError(true)
	}
	return stmt, next
}

func buildPowerShellRecoveredStatementBlock(arena *nodeArena, source []byte, lang *Language, openBracePos, closeBracePos int) *Node {
	statementBlockSym, statementBlockNamed, ok := symbolMeta(lang, "statement_block")
	if !ok {
		return nil
	}
	openBraceSym, _, ok := symbolMeta(lang, "{")
	if !ok {
		return nil
	}
	closeBraceSym, _, ok := symbolMeta(lang, "}")
	if !ok {
		return nil
	}
	statementListSym, statementListNamed, ok := symbolMeta(lang, "statement_list")
	if !ok {
		return nil
	}
	inner := buildPowerShellRecoveredStatements(arena, source, lang, openBracePos+1, closeBracePos, nil)
	blockChildren := make([]*Node, 0, len(inner)+2)
	blockChildren = append(blockChildren, newLeafNodeInArena(arena, openBraceSym, false, uint32(openBracePos), uint32(openBracePos+1), advancePointByBytes(Point{}, source[:openBracePos]), advancePointByBytes(advancePointByBytes(Point{}, source[:openBracePos]), source[openBracePos:openBracePos+1])))
	leadingComments := 0
	for leadingComments < len(inner) && inner[leadingComments] != nil && inner[leadingComments].Type(lang) == "comment" {
		blockChildren = append(blockChildren, inner[leadingComments])
		leadingComments++
	}
	if leadingComments < len(inner) {
		stmtChildren := inner[leadingComments:]
		if arena != nil {
			buf := arena.allocNodeSlice(len(stmtChildren))
			copy(buf, stmtChildren)
			stmtChildren = buf
		}
		blockChildren = append(blockChildren, newParentNodeInArena(arena, statementListSym, statementListNamed, stmtChildren, nil, 0))
	}
	blockChildren = append(blockChildren, newLeafNodeInArena(arena, closeBraceSym, false, uint32(closeBracePos), uint32(closeBracePos+1), advancePointByBytes(Point{}, source[:closeBracePos]), advancePointByBytes(advancePointByBytes(Point{}, source[:closeBracePos]), source[closeBracePos:closeBracePos+1])))
	if arena != nil {
		buf := arena.allocNodeSlice(len(blockChildren))
		copy(buf, blockChildren)
		blockChildren = buf
	}
	block := newParentNodeInArena(arena, statementBlockSym, statementBlockNamed, blockChildren, nil, 0)
	for i, child := range block.children {
		if child == nil || child.Type(lang) != "statement_list" {
			continue
		}
		for fieldIdx, fieldName := range lang.FieldNames {
			if fieldName != "statement_list" {
				continue
			}
			ensureNodeFieldStorage(block, len(block.children))
			block.fieldIDs()[i] = FieldID(fieldIdx)
			block.fieldSources()[i] = fieldSourceDirect
			break
		}
		break
	}
	return block
}

func buildPowerShellRecoveredTryStatement(arena *nodeArena, source []byte, lang *Language, start, end int) (*Node, int) {
	tryStatementSym, tryStatementNamed, ok := symbolMeta(lang, "try_statement")
	if !ok {
		return nil, 0
	}
	trySym, tryNamed, ok := symbolMeta(lang, "try")
	if !ok {
		return nil, 0
	}
	catchClausesSym, catchClausesNamed, ok := symbolMeta(lang, "catch_clauses")
	if !ok {
		return nil, 0
	}
	blockOpen := powerShellSkipTrivia(source, start+len("try"), end)
	if blockOpen >= end || source[blockOpen] != '{' {
		return nil, 0
	}
	blockClose := findMatchingBraceByte(source, blockOpen, end)
	if blockClose < 0 {
		return nil, 0
	}
	tryBlock := buildPowerShellRecoveredStatementBlock(arena, source, lang, blockOpen, blockClose)
	if tryBlock == nil {
		return nil, 0
	}
	catchStart := powerShellSkipTrivia(source, blockClose+1, end)
	if !powerShellKeywordAt(source, catchStart, "catch") {
		return nil, 0
	}
	catchClause, next := buildPowerShellRecoveredCatchClause(arena, source, lang, catchStart, end)
	if catchClause == nil || next <= catchStart {
		return nil, 0
	}
	catchChildren := []*Node{catchClause}
	if arena != nil {
		buf := arena.allocNodeSlice(1)
		buf[0] = catchClause
		catchChildren = buf
	}
	children := []*Node{
		newLeafNodeInArena(arena, trySym, tryNamed, uint32(start), uint32(start+len("try")), advancePointByBytes(Point{}, source[:start]), advancePointByBytes(advancePointByBytes(Point{}, source[:start]), source[start:start+len("try")])),
		tryBlock,
		newParentNodeInArena(arena, catchClausesSym, catchClausesNamed, catchChildren, nil, 0),
	}
	if arena != nil {
		buf := arena.allocNodeSlice(len(children))
		copy(buf, children)
		children = buf
	}
	return newParentNodeInArena(arena, tryStatementSym, tryStatementNamed, children, nil, 0), next
}

func buildPowerShellRecoveredCatchClause(arena *nodeArena, source []byte, lang *Language, start, end int) (*Node, int) {
	catchClauseSym, catchClauseNamed, ok := symbolMeta(lang, "catch_clause")
	if !ok {
		return nil, 0
	}
	catchSym, catchNamed, ok := symbolMeta(lang, "catch")
	if !ok {
		return nil, 0
	}
	catchTypeListSym, catchTypeListNamed, ok := symbolMeta(lang, "catch_type_list")
	if !ok {
		return nil, 0
	}
	typeLiteralSym, typeLiteralNamed, ok := symbolMeta(lang, "type_literal")
	if !ok {
		return nil, 0
	}
	openBracketSym, _, ok := symbolMeta(lang, "[")
	if !ok {
		return nil, 0
	}
	closeBracketSym, _, ok := symbolMeta(lang, "]")
	if !ok {
		return nil, 0
	}
	typeOpen := powerShellSkipInlineSpace(source, start+len("catch"), end)
	if typeOpen >= end || source[typeOpen] != '[' {
		return nil, 0
	}
	typeClose := findMatchingDelimitedByte(source, typeOpen, end, '[', ']')
	if typeClose < 0 {
		return nil, 0
	}
	typeCoreStart, typeCoreEnd := powerShellTrimInlineSpace(source, typeOpen+1, typeClose)
	if typeCoreStart >= typeCoreEnd {
		return nil, 0
	}
	typeSpec := buildPowerShellTypeSpec(arena, source, lang, typeCoreStart, typeCoreEnd)
	if typeSpec == nil {
		return nil, 0
	}
	typeLiteralChildren := []*Node{
		newLeafNodeInArena(arena, openBracketSym, false, uint32(typeOpen), uint32(typeOpen+1), advancePointByBytes(Point{}, source[:typeOpen]), advancePointByBytes(advancePointByBytes(Point{}, source[:typeOpen]), source[typeOpen:typeOpen+1])),
		typeSpec,
		newLeafNodeInArena(arena, closeBracketSym, false, uint32(typeClose), uint32(typeClose+1), advancePointByBytes(Point{}, source[:typeClose]), advancePointByBytes(advancePointByBytes(Point{}, source[:typeClose]), source[typeClose:typeClose+1])),
	}
	if arena != nil {
		buf := arena.allocNodeSlice(len(typeLiteralChildren))
		copy(buf, typeLiteralChildren)
		typeLiteralChildren = buf
	}
	typeListChildren := []*Node{newParentNodeInArena(arena, typeLiteralSym, typeLiteralNamed, typeLiteralChildren, nil, 0)}
	if arena != nil {
		buf := arena.allocNodeSlice(1)
		buf[0] = typeListChildren[0]
		typeListChildren = buf
	}
	blockOpen := powerShellSkipTrivia(source, typeClose+1, end)
	if blockOpen >= end || source[blockOpen] != '{' {
		return nil, 0
	}
	blockClose := findMatchingBraceByte(source, blockOpen, end)
	if blockClose < 0 {
		return nil, 0
	}
	block := buildPowerShellRecoveredStatementBlock(arena, source, lang, blockOpen, blockClose)
	if block == nil {
		return nil, 0
	}
	children := []*Node{
		newLeafNodeInArena(arena, catchSym, catchNamed, uint32(start), uint32(start+len("catch")), advancePointByBytes(Point{}, source[:start]), advancePointByBytes(advancePointByBytes(Point{}, source[:start]), source[start:start+len("catch")])),
		newParentNodeInArena(arena, catchTypeListSym, catchTypeListNamed, typeListChildren, nil, 0),
		block,
	}
	if arena != nil {
		buf := arena.allocNodeSlice(len(children))
		copy(buf, children)
		children = buf
	}
	return newParentNodeInArena(arena, catchClauseSym, catchClauseNamed, children, nil, 0), blockClose + 1
}

func buildPowerShellRecoveredFlowControlStatement(arena *nodeArena, source []byte, lang *Language, start, end int) *Node {
	flowControlSym, flowControlNamed, ok := symbolMeta(lang, "flow_control_statement")
	if !ok {
		return nil
	}
	throwSym, throwNamed, ok := symbolMeta(lang, "throw")
	if !ok {
		return nil
	}
	valueStart := powerShellSkipInlineSpace(source, start+len("throw"), end)
	valueEnd := powerShellTrimInlineSpaceRight(source, valueStart, end)
	if valueStart >= valueEnd {
		return nil
	}
	pipeline := buildPowerShellRecoveredConditionPipeline(arena, source, lang, valueStart, valueEnd)
	if pipeline == nil {
		return nil
	}
	children := []*Node{
		newLeafNodeInArena(arena, throwSym, throwNamed, uint32(start), uint32(start+len("throw")), advancePointByBytes(Point{}, source[:start]), advancePointByBytes(advancePointByBytes(Point{}, source[:start]), source[start:start+len("throw")])),
		pipeline,
	}
	if arena != nil {
		buf := arena.allocNodeSlice(len(children))
		copy(buf, children)
		children = buf
	}
	return newParentNodeInArena(arena, flowControlSym, flowControlNamed, children, nil, 0)
}

func buildPowerShellRecoveredPipeline(arena *nodeArena, source []byte, lang *Language, start, end int) *Node {
	if lang == nil || start >= end {
		return nil
	}
	if powerShellFindAssignmentByte(source, start, end) >= 0 {
		return buildPowerShellRecoveredAssignmentPipeline(arena, source, lang, start, end)
	}
	pipelineSym, ok := symbolByName(lang, "pipeline")
	if !ok {
		return nil
	}
	pipelineChainSym, ok := symbolByName(lang, "pipeline_chain")
	if !ok {
		return nil
	}
	commandSym, ok := symbolByName(lang, "command")
	if !ok {
		return nil
	}
	commandNameSym, ok := symbolByName(lang, "command_name")
	if !ok {
		return nil
	}
	commandElementsSym, ok := symbolByName(lang, "command_elements")
	if !ok {
		return nil
	}
	commandArgSepSym, ok := symbolByName(lang, "command_argument_sep")
	if !ok {
		return nil
	}
	commandParameterSym, ok := symbolByName(lang, "command_parameter")
	if !ok {
		return nil
	}
	arrayLiteralSym, ok := symbolByName(lang, "array_literal_expression")
	if !ok {
		return nil
	}
	unaryExprSym, ok := symbolByName(lang, "unary_expression")
	if !ok {
		return nil
	}
	variableSym, ok := symbolByName(lang, "variable")
	if !ok {
		return nil
	}
	stringLiteralSym, ok := symbolByName(lang, "string_literal")
	if !ok {
		return nil
	}
	expandableStringSym, ok := symbolByName(lang, "expandable_string_literal")
	if !ok {
		return nil
	}
	genericTokenSym, ok := symbolByName(lang, "generic_token")
	if !ok {
		return nil
	}
	spaceSym, ok := symbolByName(lang, " ")
	if !ok {
		return nil
	}
	return buildPowerShellPipelineFromLine(arena, source, lang, start, end, pipelineSym, pipelineChainSym, commandSym, commandNameSym, commandElementsSym, commandArgSepSym, commandParameterSym, arrayLiteralSym, unaryExprSym, variableSym, stringLiteralSym, expandableStringSym, genericTokenSym, spaceSym)
}

func buildPowerShellRecoveredAssignmentPipeline(arena *nodeArena, source []byte, lang *Language, start, end int) *Node {
	pipelineSym, pipelineNamed, ok := symbolMeta(lang, "pipeline")
	if !ok {
		return nil
	}
	assignmentExprSym, assignmentExprNamed, ok := symbolMeta(lang, "assignment_expression")
	if !ok {
		return nil
	}
	assignOpSym, assignOpNamed, ok := symbolMeta(lang, "assignement_operator")
	if !ok {
		return nil
	}
	assignLeafSym, assignLeafNamed, ok := symbolMeta(lang, "=")
	if !ok {
		assignLeafSym = assignOpSym
		assignLeafNamed = assignOpNamed
	}
	eq := powerShellFindAssignmentByte(source, start, end)
	if eq < 0 {
		return nil
	}
	lhsStart, lhsEnd := powerShellTrimInlineSpace(source, start, eq)
	rhsStart, rhsEnd := powerShellTrimInlineSpace(source, eq+1, end)
	if lhsStart >= lhsEnd || rhsStart >= rhsEnd {
		return nil
	}
	lhs := buildPowerShellLeftAssignmentExpression(arena, source, lang, lhsStart, lhsEnd)
	if lhs == nil {
		return nil
	}
	assignChildren := []*Node{newLeafNodeInArena(arena, assignLeafSym, assignLeafNamed, uint32(eq), uint32(eq+1), advancePointByBytes(Point{}, source[:eq]), advancePointByBytes(advancePointByBytes(Point{}, source[:eq]), source[eq:eq+1]))}
	if arena != nil {
		buf := arena.allocNodeSlice(1)
		buf[0] = assignChildren[0]
		assignChildren = buf
	}
	assignOp := newParentNodeInArena(arena, assignOpSym, assignOpNamed, assignChildren, nil, 0)
	rhs := buildPowerShellRecoveredConditionPipeline(arena, source, lang, rhsStart, rhsEnd)
	if rhs == nil {
		rhs = buildPowerShellRecoveredPipeline(arena, source, lang, rhsStart, rhsEnd)
	}
	if rhs == nil {
		return nil
	}
	children := []*Node{lhs, assignOp, rhs}
	if arena != nil {
		buf := arena.allocNodeSlice(len(children))
		copy(buf, children)
		children = buf
	}
	assignExpr := newParentNodeInArena(arena, assignmentExprSym, assignmentExprNamed, children, nil, 0)
	for fieldIdx, fieldName := range lang.FieldNames {
		if fieldName != "value" {
			continue
		}
		ensureNodeFieldStorage(assignExpr, len(assignExpr.children))
		assignExpr.fieldIDs()[2] = FieldID(fieldIdx)
		assignExpr.fieldSources()[2] = fieldSourceDirect
		break
	}
	pipelineChildren := []*Node{assignExpr}
	if arena != nil {
		buf := arena.allocNodeSlice(1)
		buf[0] = assignExpr
		pipelineChildren = buf
	}
	return newParentNodeInArena(arena, pipelineSym, pipelineNamed, pipelineChildren, nil, 0)
}

func buildPowerShellLeftAssignmentExpression(arena *nodeArena, source []byte, lang *Language, start, end int) *Node {
	if _, _, ok := symbolMeta(lang, "left_assignment_expression"); !ok {
		return nil
	}
	core := buildPowerShellExpressionCore(arena, source, lang, start, end)
	if core == nil {
		return nil
	}
	return wrapPowerShellExpression(arena, lang, core, "unary_expression", "array_literal_expression", "range_expression", "format_expression", "multiplicative_expression", "additive_expression", "comparison_expression", "bitwise_expression", "logical_expression", "left_assignment_expression")
}

func buildPowerShellRecoveredConditionPipeline(arena *nodeArena, source []byte, lang *Language, start, end int) *Node {
	pipelineSym, pipelineNamed, ok := symbolMeta(lang, "pipeline")
	if !ok {
		return nil
	}
	pipelineChainSym, pipelineChainNamed, ok := symbolMeta(lang, "pipeline_chain")
	if !ok {
		return nil
	}
	if powerShellLooksLikeCommandText(source, start, end) {
		if pipeline := buildPowerShellRecoveredPipeline(arena, source, lang, start, end); pipeline != nil {
			return pipeline
		}
	}
	logical := buildPowerShellLogicalExpression(arena, source, lang, start, end)
	if logical == nil {
		return nil
	}
	chainChildren := []*Node{logical}
	if arena != nil {
		buf := arena.allocNodeSlice(1)
		buf[0] = logical
		chainChildren = buf
	}
	chain := newParentNodeInArena(arena, pipelineChainSym, pipelineChainNamed, chainChildren, nil, 0)
	pipelineChildren := []*Node{chain}
	if arena != nil {
		buf := arena.allocNodeSlice(1)
		buf[0] = chain
		pipelineChildren = buf
	}
	return newParentNodeInArena(arena, pipelineSym, pipelineNamed, pipelineChildren, nil, 0)
}
