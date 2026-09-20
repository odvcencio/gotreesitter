package gotreesitter

func normalizeCSharpConditionalIsPatternExpressions(root *Node, source []byte, lang *Language) {
	if root == nil || lang == nil || lang.Name != "c_sharp" {
		return
	}
	isPatternSym, ok := symbolByName(lang, "is_pattern_expression")
	if !ok {
		return
	}
	constantPatternSym, ok := symbolByName(lang, "constant_pattern")
	if !ok {
		return
	}
	nullSym, hasNull := symbolByName(lang, "null_literal")
	nullNamed := symbolIsNamed(lang, nullSym)
	isPatternNamed := symbolIsNamed(lang, isPatternSym)
	constantPatternNamed := symbolIsNamed(lang, constantPatternSym)
	expressionFieldID, _ := lang.FieldByName("expression")
	patternFieldID, _ := lang.FieldByName("pattern")

	walkResultTree(root, func(n *Node) {
		if n.Type(lang) == "conditional_expression" {
			for i, child := range n.children {
				if child == nil || n.FieldNameForChild(i, lang) != "condition" || child.Type(lang) != "is_expression" {
					continue
				}
				csharpRewriteConditionalIsPatternExpression(child, source, lang, isPatternSym, isPatternNamed, constantPatternSym, constantPatternNamed, nullSym, nullNamed, hasNull, expressionFieldID, patternFieldID)
			}
		}
	})
}

func normalizeCSharpIdentifierIsPatternExpressions(root *Node, source []byte, lang *Language) {
	if root == nil || lang == nil || lang.Name != "c_sharp" {
		return
	}
	isPatternSym, ok := symbolByName(lang, "is_pattern_expression")
	if !ok {
		return
	}
	constantPatternSym, ok := symbolByName(lang, "constant_pattern")
	if !ok {
		return
	}
	nullSym, hasNull := symbolByName(lang, "null_literal")
	nullNamed := symbolIsNamed(lang, nullSym)
	isPatternNamed := symbolIsNamed(lang, isPatternSym)
	constantPatternNamed := symbolIsNamed(lang, constantPatternSym)
	expressionFieldID, _ := lang.FieldByName("expression")
	patternFieldID, _ := lang.FieldByName("pattern")

	walkResultTree(root, func(n *Node) {
		if n.Type(lang) == "is_expression" {
			csharpRewriteConditionalIsPatternExpression(n, source, lang, isPatternSym, isPatternNamed, constantPatternSym, constantPatternNamed, nullSym, nullNamed, hasNull, expressionFieldID, patternFieldID)
		}
	})
}

func normalizeCSharpConditionalExpressionTokens(root *Node, source []byte, lang *Language) {
	if root == nil || lang == nil || lang.Name != "c_sharp" || len(source) == 0 {
		return
	}
	walkResultTree(root, func(n *Node) {
		if n.Type(lang) != "conditional_expression" {
			return
		}
		csharpRestoreConditionalExpressionTokens(n, source, lang)
	})
}

func csharpRestoreConditionalExpressionTokens(n *Node, source []byte, lang *Language) bool {
	if n == nil || resultChildCount(n) != 3 {
		return false
	}
	condition := resultChildAt(n, 0)
	consequence := resultChildAt(n, 1)
	alternative := resultChildAt(n, 2)
	if condition == nil || consequence == nil || alternative == nil {
		return false
	}
	if condition.endByte > consequence.startByte || consequence.endByte > alternative.startByte || int(alternative.endByte) > len(source) {
		return false
	}
	qPos, ok := csharpFindTopLevelOperator(source, condition.endByte, consequence.startByte, "?")
	if !ok {
		return false
	}
	colonPos, ok := csharpFindConditionalColon(source, qPos+1, alternative.startByte)
	if !ok || colonPos < consequence.endByte {
		return false
	}
	qTok, ok := csharpBuildConditionalQuestionToken(n.ownerArena, source, lang, qPos)
	if !ok {
		return false
	}
	colonTok, ok := csharpBuildLeafNodeByName(n.ownerArena, source, lang, ":", colonPos, colonPos+1)
	if !ok {
		return false
	}
	children := cloneNodeSliceIfArena(n.ownerArena, []*Node{condition, qTok, consequence, colonTok, alternative})
	conditionID, _ := lang.FieldByName("condition")
	consequenceID, _ := lang.FieldByName("consequence")
	alternativeID, _ := lang.FieldByName("alternative")
	fieldIDs := cloneFieldIDSliceInArena(n.ownerArena, []FieldID{conditionID, 0, consequenceID, 0, alternativeID})
	n.children = children
	n.setFieldMetadata(fieldIDs, defaultFieldSourcesInArena(n.ownerArena, fieldIDs))
	if n.ownerArena != nil {
		n.ownerArena.clearFinalChildRefs(n)
	}
	n.productionID = 0
	n.setHasError(false)
	populateParentNode(n, n.children)
	return true
}

func csharpBuildConditionalQuestionToken(arena *nodeArena, source []byte, lang *Language, pos uint32) (*Node, bool) {
	if tok, ok := csharpBuildLeafNodeByName(arena, source, lang, "?", pos, pos+1); ok {
		return tok, true
	}
	sym, ok := symbolByName(lang, "\\?")
	if !ok {
		return nil, false
	}
	return newLeafNodeInArena(arena, sym, symbolIsNamed(lang, sym), pos, pos+1, advancePointByBytes(Point{}, source[:pos]), advancePointByBytes(Point{}, source[:pos+1])), true
}

func csharpRewriteConditionalIsPatternExpression(n *Node, source []byte, lang *Language, isPatternSym Symbol, isPatternNamed bool, constantPatternSym Symbol, constantPatternNamed bool, nullSym Symbol, nullNamed bool, hasNull bool, expressionFieldID, patternFieldID FieldID) bool {
	if n == nil || lang == nil || n.Type(lang) != "is_expression" || len(n.children) < 3 {
		return false
	}
	exprIdx := -1
	patternIdx := -1
	for i, child := range n.children {
		if child == nil || !child.IsNamed() {
			continue
		}
		if exprIdx == -1 {
			exprIdx = i
			continue
		}
		patternIdx = i
		break
	}
	if exprIdx < 0 || patternIdx < 0 {
		return false
	}
	patternValue := n.children[patternIdx]
	if patternValue == nil || patternValue.Type(lang) != "identifier" {
		return false
	}
	if hasNull && patternValue.startByte < patternValue.endByte && int(patternValue.endByte) <= len(source) &&
		string(source[patternValue.startByte:patternValue.endByte]) == "null" {
		retagResultRoot(patternValue, nullSym, nullNamed)
		replaceNodeChildrenUnfielded(patternValue, nil)
	}
	patternChildren := []*Node{patternValue}
	patternChildren = cloneNodeSliceIfArena(n.ownerArena, patternChildren)
	constantPattern := newParentNodeInArena(n.ownerArena, constantPatternSym, constantPatternNamed, patternChildren, nil, 0)
	constantPattern.setHasError(false)

	children := append([]*Node(nil), n.children...)
	children[patternIdx] = constantPattern
	children = cloneNodeSliceIfArena(n.ownerArena, children)
	fieldIDs := make([]FieldID, len(children))
	fieldIDs[exprIdx] = expressionFieldID
	fieldIDs[patternIdx] = patternFieldID
	fieldIDs = cloneFieldIDSliceInArena(n.ownerArena, fieldIDs)

	n.symbol = isPatternSym
	n.setNamed(isPatternNamed)
	n.children = children
	n.setFieldMetadata(fieldIDs, defaultFieldSourcesInArena(n.ownerArena, fieldIDs))
	n.productionID = 0
	n.setHasError(false)
	populateParentNode(n, n.children)
	return true
}

func normalizeCSharpConditionalIsPatternInitializers(root *Node, source []byte, lang *Language) {
	if root == nil || lang == nil || lang.Name != "c_sharp" || len(source) == 0 {
		return
	}
	walkResultTree(root, func(n *Node) {
		if n.Type(lang) == "local_declaration_statement" {
			csharpRewriteConditionalIsPatternInitializer(n, source, lang)
		}
	})
}

func csharpRewriteConditionalIsPatternInitializer(stmt *Node, source []byte, lang *Language) bool {
	if stmt == nil || lang == nil || stmt.ownerArena == nil || stmt.startByte >= stmt.endByte || int(stmt.endByte) > len(source) {
		return false
	}
	if source[stmt.endByte-1] != ';' {
		return false
	}
	stmtEnd := stmt.endByte - 1
	eqPos, ok := csharpFindTopLevelAssignment(source, stmt.startByte, stmtEnd)
	if !ok {
		return false
	}
	valueStart := csharpSkipSpaceBytes(source, eqPos+1)
	valueEnd := csharpTrimRightSpaceBytes(source, stmtEnd)
	if valueStart >= valueEnd {
		return false
	}
	qPos, ok := csharpFindTopLevelOperator(source, valueStart, valueEnd, "?")
	if !ok {
		return false
	}
	if _, ok := csharpFindConditionalColon(source, qPos+1, valueEnd); !ok {
		return false
	}
	if _, ok := csharpFindTopLevelKeyword(source, valueStart, qPos, "is"); !ok {
		return false
	}
	expr, ok := csharpRecoverQueryExpressionNodeFromRange(source, valueStart, valueEnd, lang, stmt.ownerArena)
	if !ok || expr == nil || expr.Type(lang) != "conditional_expression" {
		return false
	}
	if !csharpReplaceRecoveredVariableInitializer(stmt, lang, expr) {
		return false
	}
	recomputeNodePointsFromBytes(stmt, source)
	return true
}
