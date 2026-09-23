package gotreesitter

import (
	"bytes"
)

const (
	csharpMaxTopLevelChunkRecoverySourceBytes = 4096
	csharpMaxTopLevelChunkRecoverySpans       = 128

	// csharpMaxNamespaceRecoveries caps how many top-level error nodes we
	// re-parse via parseWithSnippetParser. Each recovery is itself a full
	// snippet parse — on pathological inputs (e.g. C# with CJK identifiers
	// or many nested partial classes) the recovery loop can be triggered
	// dozens of times per file, multiplying parse time/memory.
	csharpMaxNamespaceRecoveries = 32

	// csharpMaxTypeBodyRecoveryMembers bounds how many top-level members a
	// LENIENT, source-based type/namespace-body reconstruction will attempt
	// to recover (csharpRecoverNamespaceBodyMembersFromSource,
	// csharpRecoverSourceTypeMembersFromRange with lenient=true). This is
	// SHORT-TERM RELIEF for issue #136: large real-world files where the GLR
	// sub-parse dies partway through a type body, so the existing
	// children-based namespace/type recovery from #115/#116
	// (csharpRecoverNamespaceBodyMembersFromErrorRoot) only covers the
	// prefix that happened to parse before the failure. Unlike
	// csharpMaxTopLevelChunkRecoverySourceBytes (applied per FILE at the
	// csharpRecoverTopLevelChunks entry point — see its doc comment), this
	// bound is applied per BODY, so a namespace/class with a realistic
	// number of members is eligible regardless of the enclosing file's total
	// size. Each individual member is still capped at
	// csharpMaxTopLevelChunkRecoverySourceBytes bytes (skipped, not
	// reparsed, if larger). Worst case nests ONE level, not a single flat
	// bound: a namespace body can have up to csharpMaxTypeBodyRecoveryMembers
	// top-level members (csharpRecoverNamespaceBodyMembersFromSource), and
	// each of those that is itself a class/struct/interface body can in turn
	// have up to csharpMaxTypeBodyRecoveryMembers members
	// (csharpRecoverSourceTypeMembersFromRange) — so the worst-case bound is
	// csharpMaxTypeBodyRecoveryMembers^2 (65536) snippet reparses, each at
	// most csharpMaxTopLevelChunkRecoverySourceBytes bytes. Recursion stops
	// there: a type body's own members (fields/methods/properties) are leaf
	// reparses, not further namespace-body-scale recursions. Remove this
	// lenient path once the GLR engine gains real mid-parse error recovery
	// (see #136) and these files parse mostly clean without needing
	// source reconstruction.
	csharpMaxTypeBodyRecoveryMembers = 256

	// Bounds for the alternate lenient source-based member recovery driver in
	// parser_result_csharp_method_recovery.go (#136, upstream #138). This is a
	// second, independently-bounded pass used as a per-declaration fallback when
	// it recovers strictly more method_declaration nodes than the primary
	// csharpRecoverNamespaceBodyMembersFromSource pass above (see
	// csharpChooseRecoveredNamespaceMembers). Like the bounds above, these are
	// PER declaration, so the anti-OOM guarantees from #64/#98/#106 still hold.
	// csharpMaxMemberRecoverySourceBytes caps the size of a single member snippet
	// that will be reparsed; csharpMaxTypeMemberRecoveries caps how many members
	// one type recovery will reparse.
	csharpMaxMemberRecoverySourceBytes = 32768
	csharpMaxTypeMemberRecoveries      = 4096
)

func normalizeCSharpCompatibility(root *Node, source []byte, p *Parser, lang *Language) {
	missing := csharpMissingNativeResultCompatibility(lang)
	nativeRecoveredStructure := nativeRecoveredStructureIsAuthoritative(root, source, p, lang)
	if p != nil {
		p.normalizationStats.nativeRecoveredStructureAuthoritative = nativeRecoveredStructure
	}
	if p != nil && p.skipRecoveryReparse {
		if missing&ResultCompatibilityCSharpNativeUnicodeIdentifiers != 0 {
			normalizeCSharpUnicodeIdentifierSpans(root, source, lang)
		}
		normalizeCSharpQuotedStringContentIdentifiers(root, source, lang)
		normalizeCSharpSurfaceCompatibility(root, source, lang)
		normalizeCSharpMissingAttributedProperties(root, source, lang)
		if missing&ResultCompatibilityCSharpNativeScopedLambdaStatements != 0 {
			normalizeCSharpSplitScopedLambdaStatements(root, source, lang)
		}
		normalizeCSharpInvocationStatements(root, source, lang)
		normalizeCSharpConditionalIsPatternInitializers(root, source, lang)
		normalizeCSharpConditionalIsPatternExpressions(root, source, lang)
		normalizeCSharpIdentifierIsPatternExpressions(root, source, lang)
		normalizeCSharpConditionalExpressionTokens(root, source, lang)
		normalizeCSharpNullLiteralIdentifiers(root, source, lang)
		normalizeCSharpScopedRefTypes(root, source, lang)
		normalizeCSharpImplicitVarTypes(root, source, lang)
		normalizeCSharpParenthesizedVarPatterns(root, source, lang)
		normalizeCSharpGenericBaseLists(root, lang)
		if missing&ResultCompatibilityCSharpNativeNotNull != 0 {
			normalizeCSharpTypeConstraintKeywords(root, lang)
		}
		normalizeCSharpSwitchTupleCasePatterns(root, lang)
		return
	}
	if !nativeRecoveredStructure {
		normalizeCSharpRecoveredTopLevelChunks(root, source, p)
		normalizeCSharpRecoveredNamespaces(root, source, p, lang)
		normalizeCSharpRecoveredTypeDeclarations(root, source, p, lang)
	}
	if missing&ResultCompatibilityCSharpNativeUnicodeIdentifiers != 0 {
		normalizeCSharpUnicodeIdentifierSpans(root, source, lang)
	}
	normalizeCSharpQuotedStringContentIdentifiers(root, source, lang)
	normalizeCSharpSurfaceCompatibility(root, source, lang)
	normalizeCSharpMissingAttributedProperties(root, source, lang)
	if missing&ResultCompatibilityCSharpNativeQueryExpressions != 0 {
		normalizeCSharpQueryExpressions(root, source, p)
	}
	if missing&ResultCompatibilityCSharpNativeScopedLambdaStatements != 0 {
		normalizeCSharpSplitScopedLambdaStatements(root, source, lang)
	}
	if missing&ResultCompatibilityCSharpNativeScopedLambdaBlocks != 0 {
		normalizeCSharpRecoveredScopedLambdaBlocks(root, source, p)
	}
	if !nativeRecoveredStructure {
		normalizeCSharpRecoveredMethodBlocks(root, source, p)
	}
	normalizeCSharpInvocationStatements(root, source, lang)
	normalizeCSharpConditionalIsPatternInitializers(root, source, lang)
	normalizeCSharpConditionalIsPatternExpressions(root, source, lang)
	normalizeCSharpIdentifierIsPatternExpressions(root, source, lang)
	normalizeCSharpConditionalExpressionTokens(root, source, lang)
	normalizeCSharpNullLiteralIdentifiers(root, source, lang)
	normalizeCSharpScopedRefTypes(root, source, lang)
	normalizeCSharpImplicitVarTypes(root, source, lang)
	normalizeCSharpParenthesizedVarPatterns(root, source, lang)
	normalizeCSharpGenericBaseLists(root, lang)
	if missing&ResultCompatibilityCSharpNativeNotNull != 0 {
		normalizeCSharpTypeConstraintKeywords(root, lang)
	}
	normalizeCSharpSwitchTupleCasePatterns(root, lang)
}

func normalizeCSharpSurfaceCompatibility(root *Node, source []byte, lang *Language) {
	if root == nil || lang == nil || lang.Name != "c_sharp" || len(source) == 0 {
		return
	}
	modifierSym, hasModifier := symbolByName(lang, "modifier")
	identifierSym, hasIdentifier := symbolByName(lang, "identifier")
	nullSym, hasNull := symbolByName(lang, "null_literal")
	lambdaSym, ok := symbolByName(lang, "lambda_expression")
	hasLambda := ok
	argSym, hasArgument := symbolByName(lang, "argument")
	stringLiteralSym, hasStringLiteral := symbolByName(lang, "string_literal")
	modifierNamed := symbolIsNamed(lang, modifierSym)
	nullNamed := symbolIsNamed(lang, nullSym)
	walkResultTree(root, func(n *Node) {
		if n == nil {
			return
		}
		childCount := resultChildCount(n)
		if hasIdentifier && hasNull && n.Type(lang) == "identifier" &&
			n.startByte < n.endByte && int(n.endByte) <= len(source) &&
			string(source[n.startByte:n.endByte]) == "null" {
			retagResultRoot(n, nullSym, nullNamed)
			replaceNodeChildrenUnfielded(n, nil)
			return
		}
		if hasLambda && hasIdentifier && hasModifier && n.symbol == lambdaSym && childCount > 0 && len(n.fieldIDs()) > 0 {
			first := resultChildAt(n, 0)
			if first != nil && first.symbol == identifierSym && resultChildCount(first) == 0 &&
				first.startByte < first.endByte && int(first.endByte) <= len(source) &&
				string(source[first.startByte:first.endByte]) == "async" {
				retagResultRoot(first, modifierSym, modifierNamed)
				n.fieldIDs()[0] = 0
				if len(n.fieldSources()) > 0 {
					n.fieldSources()[0] = fieldSourceNone
				}
			}
			return
		}
		if hasArgument && n.symbol == argSym && childCount > 0 && n.startByte > 0 && int(n.startByte) < len(source) {
			first := resultChildAt(n, 0)
			if first != nil && first.startByte == n.startByte && source[n.startByte-1] == '$' {
				n.startByte--
				n.startPoint = advancePointByBytes(Point{}, source[:n.startByte])
			}
		}
		if hasStringLiteral && n.symbol == stringLiteralSym && n.startByte > 0 && int(n.endByte) <= len(source) && source[n.startByte-1] == '$' {
			csharpRewriteInterpolatedStringExpression(n, source, lang)
			return
		}
	})
}

func normalizeCSharpNullLiteralIdentifiers(root *Node, source []byte, lang *Language) {
	if root == nil || lang == nil || lang.Name != "c_sharp" || len(source) == 0 {
		return
	}
	nullSym, ok := symbolByName(lang, "null_literal")
	if !ok {
		return
	}
	nullNamed := symbolIsNamed(lang, nullSym)
	walkResultTree(root, func(n *Node) {
		if n == nil || n.Type(lang) != "identifier" || n.startByte >= n.endByte || int(n.endByte) > len(source) {
			return
		}
		if string(source[n.startByte:n.endByte]) != "null" {
			return
		}
		retagResultRoot(n, nullSym, nullNamed)
		replaceNodeChildrenUnfielded(n, nil)
	})
}

func normalizeCSharpImplicitVarTypes(root *Node, source []byte, lang *Language) {
	if root == nil || lang == nil || lang.Name != "c_sharp" || len(source) == 0 {
		return
	}
	implicitTypeSym, ok := symbolByName(lang, "implicit_type")
	if !ok {
		return
	}
	implicitTypeNamed := symbolIsNamed(lang, implicitTypeSym)
	walkResultTree(root, func(n *Node) {
		if n == nil || n.ownerArena == nil || !csharpNodeCanContainImplicitVarType(n, lang) {
			return
		}
		for i, child := range n.children {
			if child == nil || child.Type(lang) != "identifier" || child.startByte >= child.endByte || int(child.endByte) > len(source) {
				continue
			}
			if string(source[child.startByte:child.endByte]) != "var" {
				continue
			}
			varTok, ok := csharpBuildLeafNodeByName(n.ownerArena, source, lang, "var", child.startByte, child.endByte)
			if !ok {
				continue
			}
			n.children[i] = newParentNodeInArena(n.ownerArena, implicitTypeSym, implicitTypeNamed, []*Node{varTok}, nil, 0)
		}
	})
}

func csharpNodeCanContainImplicitVarType(n *Node, lang *Language) bool {
	switch n.Type(lang) {
	case "variable_declaration", "object_creation_expression", "declaration_pattern":
		return true
	default:
		return false
	}
}

func normalizeCSharpScopedRefTypes(root *Node, source []byte, lang *Language) {
	if root == nil || lang == nil || lang.Name != "c_sharp" || len(source) == 0 {
		return
	}
	walkResultTree(root, func(n *Node) {
		switch n.Type(lang) {
		case "parameter":
			csharpRewriteScopedRefParameter(n, source, lang)
		case "variable_declaration":
			csharpRewriteScopedRefVariableDeclaration(n, source, lang)
		}
	})
}

func csharpRewriteScopedRefParameter(param *Node, source []byte, lang *Language) bool {
	if param == nil || param.ownerArena == nil || resultChildCount(param) < 2 {
		return false
	}
	first := resultChildAt(param, 0)
	second := resultChildAt(param, 1)
	if !csharpNodeTextIs(source, first, "scoped") || !csharpNodeTextIs(source, second, "ref") {
		return false
	}
	typeStart := csharpSkipSpaceBytes(source, second.endByte)
	typeNameStart, typeNameEnd, ok := csharpScanIdentifierAt(source, typeStart)
	if !ok || typeNameStart != typeStart {
		return false
	}
	nameStart, nameEnd, ok := csharpScanIdentifierAt(source, csharpSkipSpaceBytes(source, typeNameEnd))
	if !ok {
		return false
	}
	if int(nameEnd) > len(source) {
		return false
	}
	trailing := csharpSkipSpaceBytes(source, nameEnd)
	if trailing >= uint32(len(source)) || source[trailing] != ')' && source[trailing] != ',' {
		return false
	}
	scopedMod, ok := csharpBuildModifierNodeFromSource(param.ownerArena, source, lang, first.startByte, first.endByte)
	if !ok {
		return false
	}
	refMod, ok := csharpBuildModifierNodeFromSource(param.ownerArena, source, lang, second.startByte, second.endByte)
	if !ok {
		return false
	}
	typeNode, ok := csharpBuildTypeNameNodeFromSource(param.ownerArena, source, lang, typeNameStart, typeNameEnd)
	if !ok {
		return false
	}
	nameNode, ok := csharpBuildIdentifierNodeFromSource(source, nameStart, nameEnd, lang, param.ownerArena)
	if !ok {
		return false
	}
	children := cloneNodeSliceIfArena(param.ownerArena, []*Node{scopedMod, refMod, typeNode, nameNode})
	replaceNodeChildrenUnfielded(param, children)
	param.startByte = first.startByte
	param.endByte = nameEnd
	recomputeNodePointsFromBytes(param, source)
	param.productionID = 0
	param.setHasError(false)
	return true
}

func csharpRewriteScopedRefVariableDeclaration(decl *Node, source []byte, lang *Language) bool {
	if decl == nil || decl.ownerArena == nil || resultChildCount(decl) != 2 {
		return false
	}
	typeCandidate := resultChildAt(decl, 0)
	declarator := resultChildAt(decl, 1)
	if !csharpNodeTextIs(source, typeCandidate, "scoped") || declarator == nil || declarator.Type(lang) != "variable_declarator" {
		return false
	}
	refStart := csharpSkipSpaceBytes(source, typeCandidate.endByte)
	if !csharpHasKeywordAt(source, refStart, "ref") {
		return false
	}
	typeStart := csharpSkipSpaceBytes(source, refStart+uint32(len("ref")))
	typeNameStart, typeNameEnd, ok := csharpScanIdentifierAt(source, typeStart)
	if !ok || typeNameStart != typeStart {
		return false
	}
	nameStart, nameEnd, ok := csharpScanIdentifierAt(source, csharpSkipSpaceBytes(source, typeNameEnd))
	if !ok {
		return false
	}
	eqPos := csharpSkipSpaceBytes(source, nameEnd)
	if eqPos >= uint32(len(source)) || source[eqPos] != '=' {
		return false
	}
	value := csharpVariableDeclaratorInitializerValue(declarator, lang)
	if value == nil {
		return false
	}
	scopedType, ok := csharpBuildScopedRefTypeNode(decl.ownerArena, source, lang, typeCandidate.startByte, refStart, typeNameStart, typeNameEnd)
	if !ok {
		return false
	}
	newDeclarator, ok := csharpBuildVariableDeclaratorNode(source, lang, decl.ownerArena, nameStart, nameEnd, eqPos, value)
	if !ok {
		return false
	}
	typeID, _ := lang.FieldByName("type")
	children := cloneNodeSliceIfArena(decl.ownerArena, []*Node{scopedType, newDeclarator})
	fields := cloneFieldIDSliceInArena(decl.ownerArena, []FieldID{typeID, 0})
	decl.children = children
	decl.setFieldMetadata(fields, defaultFieldSourcesInArena(decl.ownerArena, fields))
	decl.startByte = typeCandidate.startByte
	decl.endByte = newDeclarator.endByte
	recomputeNodePointsFromBytes(decl, source)
	decl.productionID = 0
	decl.setHasError(false)
	populateParentNode(decl, decl.children)
	return true
}

func csharpVariableDeclaratorInitializerValue(declarator *Node, lang *Language) *Node {
	if declarator == nil || lang == nil {
		return nil
	}
	for i := resultChildCount(declarator) - 1; i >= 0; i-- {
		child := resultChildAt(declarator, i)
		if child != nil && child.IsNamed() && child.Type(lang) != "identifier" {
			return child
		}
	}
	return nil
}

func csharpBuildModifierNodeFromSource(arena *nodeArena, source []byte, lang *Language, start, end uint32) (*Node, bool) {
	if arena == nil || lang == nil || start >= end || int(end) > len(source) {
		return nil, false
	}
	modifierSym, ok := symbolByName(lang, "modifier")
	if !ok {
		return nil, false
	}
	return newLeafNodeInArena(arena, modifierSym, symbolIsNamed(lang, modifierSym), start, end, advancePointByBytes(Point{}, source[:start]), advancePointByBytes(Point{}, source[:end])), true
}

func csharpBuildScopedRefTypeNode(arena *nodeArena, source []byte, lang *Language, scopedStart, refStart, typeStart, typeEnd uint32) (*Node, bool) {
	if arena == nil || lang == nil || scopedStart >= refStart || typeStart >= typeEnd || int(typeEnd) > len(source) {
		return nil, false
	}
	scopedTypeSym, ok := symbolByName(lang, "scoped_type")
	if !ok {
		return nil, false
	}
	refTypeSym, ok := symbolByName(lang, "ref_type")
	if !ok {
		return nil, false
	}
	refTok, ok := csharpBuildLeafNodeByName(arena, source, lang, "ref", refStart, refStart+uint32(len("ref")))
	if !ok {
		return nil, false
	}
	typeNode, ok := csharpBuildTypeNameNodeFromSource(arena, source, lang, typeStart, typeEnd)
	if !ok {
		return nil, false
	}
	refType := newParentNodeInArena(arena, refTypeSym, symbolIsNamed(lang, refTypeSym), cloneNodeSliceIfArena(arena, []*Node{refTok, typeNode}), nil, 0)
	refType.setHasError(false)
	scopedType := newParentNodeInArena(arena, scopedTypeSym, symbolIsNamed(lang, scopedTypeSym), cloneNodeSliceIfArena(arena, []*Node{refType}), nil, 0)
	scopedType.startByte = scopedStart
	scopedType.endByte = typeNode.endByte
	scopedType.startPoint = advancePointByBytes(Point{}, source[:scopedStart])
	scopedType.endPoint = typeNode.endPoint
	scopedType.setHasError(false)
	return scopedType, true
}

func csharpNodeTextIs(source []byte, n *Node, text string) bool {
	if n == nil || n.startByte >= n.endByte || int(n.endByte) > len(source) {
		return false
	}
	return string(source[n.startByte:n.endByte]) == text
}

func normalizeCSharpParenthesizedVarPatterns(root *Node, source []byte, lang *Language) {
	if root == nil || lang == nil || lang.Name != "c_sharp" || len(source) == 0 {
		return
	}
	parenthesizedPatternSym, ok := symbolByName(lang, "parenthesized_pattern")
	if !ok {
		return
	}
	parenthesizedPatternNamed := symbolIsNamed(lang, parenthesizedPatternSym)
	walkResultTree(root, func(n *Node) {
		if n == nil || n.ownerArena == nil || n.Type(lang) != "recursive_pattern" || resultChildCount(n) != 1 {
			return
		}
		if n.startByte >= n.endByte || int(n.endByte) > len(source) || source[n.startByte] != '(' || source[n.endByte-1] != ')' {
			return
		}
		decl := csharpFindResultDescendantOfType(n, lang, "declaration_pattern")
		if decl == nil || decl.Type(lang) != "declaration_pattern" || resultChildCount(decl) != 2 {
			return
		}
		typeNode := resultChildAt(decl, 0)
		if typeNode == nil || typeNode.Type(lang) != "implicit_type" {
			return
		}
		openTok, ok := csharpBuildLeafNodeByName(n.ownerArena, source, lang, "(", n.startByte, n.startByte+1)
		if !ok {
			return
		}
		closeTok, ok := csharpBuildLeafNodeByName(n.ownerArena, source, lang, ")", n.endByte-1, n.endByte)
		if !ok {
			return
		}
		retagResultRoot(n, parenthesizedPatternSym, parenthesizedPatternNamed)
		replaceNodeChildrenUnfielded(n, []*Node{openTok, decl, closeTok})
	})
}

func csharpFindResultDescendantOfType(root *Node, lang *Language, want string) *Node {
	if root == nil || lang == nil {
		return nil
	}
	for i := 0; i < resultChildCount(root); i++ {
		child := resultChildAt(root, i)
		if child == nil {
			continue
		}
		if child.Type(lang) == want {
			return child
		}
		if got := csharpFindResultDescendantOfType(child, lang, want); got != nil {
			return got
		}
	}
	return nil
}

func normalizeCSharpGenericBaseLists(root *Node, lang *Language) {
	if root == nil || lang == nil || lang.Name != "c_sharp" {
		return
	}
	genericNameSym, hasGenericName := symbolByName(lang, "generic_name")
	typeArgumentListSym, hasTypeArgumentList := symbolByName(lang, "type_argument_list")
	if !hasGenericName || !hasTypeArgumentList {
		return
	}
	genericNameNamed := symbolIsNamed(lang, genericNameSym)
	typeArgumentListNamed := symbolIsNamed(lang, typeArgumentListSym)
	walkResultTree(root, func(n *Node) {
		if n == nil || n.Type(lang) != "class_declaration" || resultChildCount(n) < 5 {
			return
		}
		csharpMergeClassGenericBaseList(n, lang, genericNameSym, genericNameNamed, typeArgumentListSym, typeArgumentListNamed)
	})
}

func csharpMergeClassGenericBaseList(classNode *Node, lang *Language, genericNameSym Symbol, genericNameNamed bool, typeArgumentListSym Symbol, typeArgumentListNamed bool) bool {
	childCount := resultChildCount(classNode)
	for i := 0; i+1 < childCount; i++ {
		baseList := resultChildAt(classNode, i)
		typeParams := resultChildAt(classNode, i+1)
		if baseList == nil || typeParams == nil || baseList.Type(lang) != "base_list" || typeParams.Type(lang) != "type_parameter_list" {
			continue
		}
		if baseList.endByte != typeParams.startByte || resultChildCount(baseList) != 2 {
			continue
		}
		colon := resultChildAt(baseList, 0)
		baseName := resultChildAt(baseList, 1)
		if colon == nil || baseName == nil || colon.Type(lang) != ":" || baseName.Type(lang) != "identifier" {
			continue
		}
		retagResultRoot(typeParams, typeArgumentListSym, typeArgumentListNamed)
		csharpUnwrapTypeArgumentListParameters(typeParams, lang)
		genericChildren := cloneNodeSliceIfArena(classNode.ownerArena, []*Node{baseName, typeParams})
		genericName := newParentNodeInArena(classNode.ownerArena, genericNameSym, genericNameNamed, genericChildren, nil, 0)
		genericName.startByte = baseName.startByte
		genericName.endByte = typeParams.endByte
		genericName.startPoint = baseName.startPoint
		genericName.endPoint = typeParams.endPoint
		genericName.setHasError(false)
		baseChildren := cloneNodeSliceIfArena(classNode.ownerArena, []*Node{colon, genericName})
		baseList.children = baseChildren
		baseList.clearFieldMetadata()
		if baseList.ownerArena != nil {
			baseList.ownerArena.clearFinalChildRefs(baseList)
		}
		baseList.endByte = typeParams.endByte
		baseList.endPoint = typeParams.endPoint
		baseList.setHasError(false)
		populateParentNode(baseList, baseList.children)

		classChildren := resultChildSliceForMutation(classNode)
		if len(classChildren) != childCount {
			return false
		}
		rebuilt := make([]*Node, 0, childCount-1)
		rebuilt = append(rebuilt, classChildren[:i+1]...)
		rebuilt = append(rebuilt, classChildren[i+2:]...)
		classNode.children = cloneNodeSliceIfArena(classNode.ownerArena, rebuilt)
		if classNode.ownerArena != nil {
			classNode.ownerArena.clearFinalChildRefs(classNode)
		}
		classNode.clearFieldMetadata()
		classNode.setHasError(false)
		populateParentNode(classNode, classNode.children)
		return true
	}
	return false
}

func csharpUnwrapTypeArgumentListParameters(typeArgs *Node, lang *Language) bool {
	if typeArgs == nil || lang == nil || resultChildCount(typeArgs) == 0 {
		return false
	}
	changed := false
	children := resultChildSliceForMutation(typeArgs)
	if len(children) == 0 {
		return false
	}
	for i, child := range children {
		if child == nil || child.Type(lang) != "type_parameter" || resultChildCount(child) != 1 {
			continue
		}
		inner := resultChildAt(child, 0)
		if inner == nil || inner.Type(lang) != "identifier" {
			continue
		}
		children[i] = inner
		changed = true
	}
	if !changed {
		return false
	}
	typeArgs.children = cloneNodeSliceIfArena(typeArgs.ownerArena, children)
	typeArgs.clearFieldMetadata()
	if typeArgs.ownerArena != nil {
		typeArgs.ownerArena.clearFinalChildRefs(typeArgs)
	}
	populateParentNode(typeArgs, typeArgs.children)
	return true
}

func csharpRewriteInterpolatedStringExpression(n *Node, source []byte, lang *Language) bool {
	if n == nil || lang == nil || n.ownerArena == nil || n.startByte == 0 || n.startByte >= n.endByte || int(n.endByte) > len(source) {
		return false
	}
	dollar := n.startByte - 1
	if source[dollar] != '$' || source[n.startByte] != '"' || source[n.endByte-1] != '"' {
		return false
	}
	exprSym, ok := symbolByName(lang, "interpolated_string_expression")
	if !ok {
		return false
	}
	startTok, ok := csharpBuildLeafNodeByName(n.ownerArena, source, lang, "interpolation_start", dollar, n.startByte)
	if !ok {
		return false
	}
	openQuote, ok := csharpBuildLeafNodeByName(n.ownerArena, source, lang, "\"", n.startByte, n.startByte+1)
	if !ok {
		return false
	}
	closeQuote, ok := csharpBuildLeafNodeByName(n.ownerArena, source, lang, "\"", n.endByte-1, n.endByte)
	if !ok {
		return false
	}
	children := []*Node{startTok, openQuote}
	cursor := n.startByte + 1
	contentStart := cursor
	for cursor < n.endByte-1 {
		if source[cursor] != '{' {
			cursor++
			continue
		}
		if contentStart < cursor {
			content, ok := csharpBuildInterpolatedStringContentNode(n.ownerArena, source, lang, contentStart, cursor)
			if !ok {
				return false
			}
			children = append(children, content)
		}
		closeBrace, ok := csharpFindSimpleInterpolationCloseBrace(source, cursor+1, n.endByte-1)
		if !ok {
			return false
		}
		interp, ok := csharpBuildInterpolationNode(n.ownerArena, source, lang, cursor, closeBrace)
		if !ok {
			return false
		}
		children = append(children, interp)
		cursor = closeBrace + 1
		contentStart = cursor
	}
	if contentStart < n.endByte-1 {
		content, ok := csharpBuildInterpolatedStringContentNode(n.ownerArena, source, lang, contentStart, n.endByte-1)
		if !ok {
			return false
		}
		children = append(children, content)
	}
	children = append(children, closeQuote)
	retagResultRoot(n, exprSym, symbolIsNamed(lang, exprSym))
	n.startByte = dollar
	n.startPoint = advancePointByBytes(Point{}, source[:dollar])
	replaceNodeChildrenUnfielded(n, cloneNodeSliceInArena(n.ownerArena, children))
	n.productionID = 0
	n.setHasError(false)
	return true
}

func csharpBuildInterpolatedStringContentNode(arena *nodeArena, source []byte, lang *Language, start, end uint32) (*Node, bool) {
	if start >= end {
		return nil, false
	}
	sym, ok := symbolByName(lang, "string_content")
	if !ok {
		return nil, false
	}
	return newLeafNodeInArena(arena, sym, symbolIsNamed(lang, sym), start, end, advancePointByBytes(Point{}, source[:start]), advancePointByBytes(Point{}, source[:end])), true
}

func csharpFindSimpleInterpolationCloseBrace(source []byte, start, limit uint32) (uint32, bool) {
	for i := start; i < limit; i++ {
		if source[i] == '}' {
			return i, true
		}
	}
	return 0, false
}

func csharpBuildInterpolationNode(arena *nodeArena, source []byte, lang *Language, openBrace, closeBrace uint32) (*Node, bool) {
	if openBrace+1 >= closeBrace || int(closeBrace) >= len(source) {
		return nil, false
	}
	sym, ok := symbolByName(lang, "interpolation")
	if !ok {
		return nil, false
	}
	openTok, ok := csharpBuildLeafNodeByName(arena, source, lang, "interpolation_brace", openBrace, openBrace+1)
	if !ok {
		return nil, false
	}
	expr, ok := csharpRecoverQueryExpressionNodeFromRange(source, openBrace+1, closeBrace, lang, arena)
	if !ok {
		return nil, false
	}
	closeTok, ok := csharpBuildLeafNodeByName(arena, source, lang, "interpolation_brace", closeBrace, closeBrace+1)
	if !ok {
		return nil, false
	}
	children := cloneNodeSliceInArena(arena, []*Node{openTok, expr, closeTok})
	return newParentNodeInArena(arena, sym, symbolIsNamed(lang, sym), children, nil, 0), true
}

func normalizeCSharpRecoveredTopLevelChunks(root *Node, source []byte, p *Parser) {
	if root == nil || p == nil || p.language == nil || p.language.Name != "c_sharp" || p.skipRecoveryReparse || len(source) == 0 || root.ownerArena == nil {
		return
	}
	rootType := root.Type(p.language)
	if rootType != "ERROR" && rootType != "compilation_unit" {
		return
	}
	if rootType == "compilation_unit" && !root.HasError() {
		_, sourceEnd := csharpTrimSpaceBounds(source, 0, uint32(len(source)))
		if root.endByte >= sourceEnd {
			return
		}
	}
	recovered, ok := csharpRecoverTopLevelChunks(source, p, root.ownerArena)
	if !ok || len(recovered) == 0 {
		return
	}
	compilationUnitSym, ok := p.language.SymbolByName("compilation_unit")
	if !ok {
		return
	}
	compilationUnitNamed := symbolIsNamed(p.language, compilationUnitSym)
	recovered = cloneNodeSliceIfArena(root.ownerArena, recovered)
	retagResultRoot(root, compilationUnitSym, compilationUnitNamed)
	replaceNodeChildrenUnfielded(root, recovered)
	root.productionID = 0
	// A recovered chunk may still carry an error: the statement fallback in
	// csharpRecoverTopLevelChunkNodesFromRange assembles nodes from a partial
	// re-parse whose interior flags never saw the ERROR deep inside. Re-derive
	// the flags over each recovered subtree, then derive the root's flag from
	// them, instead of clearing it: clearing left a root that reported
	// HasError()=false above descendants that reported true.
	for _, child := range recovered {
		resultRecomputeHasErrorSubtree(child)
	}
	resultRefreshHasErrorFromChildren(root)
	extendNodeToTrailingWhitespace(root, source)
}

func csharpRecoverTopLevelChunks(source []byte, p *Parser, arena *nodeArena) ([]*Node, bool) {
	if p == nil || p.language == nil || len(source) == 0 || len(source) > csharpMaxTopLevelChunkRecoverySourceBytes {
		return nil, false
	}
	spans := csharpTopLevelChunkSpans(source)
	if len(spans) == 0 || len(spans) > csharpMaxTopLevelChunkRecoverySpans {
		return nil, false
	}
	out := make([]*Node, 0, len(spans))
	for _, span := range spans {
		for _, part := range csharpSplitLeadingTopLevelCommentSpans(source, span[0], span[1]) {
			nodes, ok := csharpRecoverTopLevelChunkNodesFromRange(source, part[0], part[1], p, arena, false)
			if !ok || len(nodes) == 0 {
				return nil, false
			}
			out = append(out, nodes...)
		}
	}
	return out, true
}

func csharpTopLevelChunkSpans(source []byte) [][2]uint32 {
	start := csharpSkipSpaceBytes(source, 0)
	if start >= uint32(len(source)) {
		return nil
	}
	var spans [][2]uint32
	braceDepth := 0
	parenDepth := 0
	bracketDepth := 0
	inLineComment := false
	inBlockComment := false
	inString := false
	inChar := false
	verbatimString := false
	escape := false
	for i := start; i < uint32(len(source)); i++ {
		b := source[i]
		if inLineComment {
			if b == '\n' {
				inLineComment = false
			}
			continue
		}
		if inBlockComment {
			if i > 0 && source[i-1] == '*' && b == '/' {
				inBlockComment = false
			}
			continue
		}
		if inString {
			if verbatimString {
				if b == '"' {
					if i+1 < uint32(len(source)) && source[i+1] == '"' {
						i++
						continue
					}
					inString = false
					verbatimString = false
				}
				continue
			}
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
		if inChar {
			if escape {
				escape = false
				continue
			}
			if b == '\\' {
				escape = true
				continue
			}
			if b == '\'' {
				inChar = false
			}
			continue
		}
		if b == '/' && i+1 < uint32(len(source)) {
			switch source[i+1] {
			case '/':
				inLineComment = true
				i++
				continue
			case '*':
				inBlockComment = true
				i++
				continue
			}
		}
		switch b {
		case '"':
			inString = true
			// Verbatim strings (@"...") and verbatim interpolated strings in
			// either order (@$"... or $@"...) use "" as the escaped quote rather
			// than backslash escapes.
			verbatimString = (i > 0 && source[i-1] == '@') ||
				(i > 1 && source[i-1] == '$' && source[i-2] == '@')
			escape = false
		case '\'':
			inChar = true
			escape = false
		case '{':
			braceDepth++
		case '}':
			if braceDepth > 0 {
				braceDepth--
				if braceDepth == 0 && parenDepth == 0 && bracketDepth == 0 {
					if next := csharpSkipSpaceBytes(source, i+1); next < uint32(len(source)) && source[next] == ';' {
						continue
					}
					spans = append(spans, [2]uint32{start, i + 1})
					start = csharpSkipSpaceBytes(source, i+1)
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
				start = csharpSkipSpaceBytes(source, i+1)
			}
		}
	}
	start, end := csharpTrimSpaceBounds(source, start, uint32(len(source)))
	if start < end {
		spans = append(spans, [2]uint32{start, end})
	}
	return spans
}

func csharpSplitLeadingTopLevelCommentSpans(source []byte, start, end uint32) [][2]uint32 {
	start, end = csharpTrimSpaceBounds(source, start, end)
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
			cursor = csharpSkipSpaceBytes(source, commentEnd)
		case cursor+1 < end && source[cursor] == '/' && source[cursor+1] == '*':
			commentEnd := csharpFindBlockCommentEnd(source, cursor+2, end)
			if commentEnd <= cursor+1 {
				return [][2]uint32{{start, end}}
			}
			spans = append(spans, [2]uint32{cursor, commentEnd})
			cursor = csharpSkipSpaceBytes(source, commentEnd)
		default:
			spans = append(spans, [2]uint32{cursor, end})
			return spans
		}
	}
	if len(spans) == 0 {
		spans = append(spans, [2]uint32{start, end})
	}
	return spans
}

func csharpRecoverTopLevelChunkNodesFromRange(source []byte, start, end uint32, p *Parser, arena *nodeArena, lenient bool) ([]*Node, bool) {
	if p == nil || p.language == nil || start >= end || int(end) > len(source) {
		return nil, false
	}
	start, end = csharpTrimSpaceBounds(source, start, end)
	if start >= end {
		return nil, false
	}
	if comment, ok := csharpRecoverTopLevelCommentNodeFromRange(source, start, end, p.language, arena); ok {
		return []*Node{comment}, true
	}
	if source[start] == '[' {
		if attributeLists, declStart, ok := csharpBuildLeadingAttributeListsFromSource(source, start, end, p.language, arena); ok && declStart < end {
			// Bounded (issue #136): csharpRecoverAttributedTopLevelTypeDeclarationFromRange
			// reparses source[declStart:end] as ONE unbounded whole-span GLR
			// parse with no size cap — safe under the strict (non-lenient)
			// caller (csharpRecoverTopLevelChunks gates the whole FILE at
			// csharpMaxTopLevelChunkRecoverySourceBytes, so declStart:end is
			// always small there), but namespace-body-scale lenient recovery
			// can hand this an attributed real-world class spanning tens of
			// KB (e.g. "[RequiresDynamicCode(...)] public sealed class Foo {
			// ...100 methods... }"), which must NOT get an unbounded reparse.
			// Route those through the per-member-bounded source declaration
			// path instead, prepending the already-recovered attribute lists.
			if lenient && end-declStart > csharpMaxTopLevelChunkRecoverySourceBytes {
				if recovered, ok := csharpRecoverSourceTopLevelTypeDeclarationFromRange(source, declStart, end, p, arena, lenient); ok {
					return []*Node{csharpPrependAttributeListsToDeclaration(recovered, attributeLists, arena)}, true
				}
			} else if recovered, ok := csharpRecoverAttributedTopLevelTypeDeclarationFromRange(source, declStart, end, attributeLists, p, p.language, arena); ok {
				return []*Node{recovered}, true
			}
		}
	}
	if recovered, ok := csharpRecoverSourceTopLevelTypeDeclarationFromRange(source, start, end, p, arena, lenient); ok {
		return []*Node{recovered}, true
	}
	// Bounded (issue #136): in lenient mode (namespace-body-scale recovery,
	// see csharpRecoverNamespaceBodyMembersFromSource), a single top-level
	// chunk can legitimately be as large as an entire real-world class body
	// — csharpRecoverSourceTopLevelTypeDeclarationFromRange above already
	// bounds THAT cost per-member (csharpMaxTypeBodyRecoveryMembers /
	// csharpMaxTopLevelChunkRecoverySourceBytes). But if the chunk was NOT
	// recognized as a class/record declaration (e.g. an enum straddling an
	// unbalanced #if/#endif), don't fall through to an unbounded whole-chunk
	// reparse here — skip it instead, so a single oversized, unrecognized
	// chunk cannot blow up worst-case recovery cost.
	if lenient && end-start > csharpMaxTopLevelChunkRecoverySourceBytes {
		return nil, false
	}
	chunk := source[start:end]
	tree, err := p.parseForRecovery(chunk)
	if err == nil && tree != nil && tree.RootNode() != nil {
		startPoint := advancePointByBytes(Point{}, source[:start])
		offsetRoot := tree.RootNodeWithOffset(start, startPoint)
		if offsetRoot != nil && !offsetRoot.HasError() {
			nodes := csharpExtractRecoveredTopLevelNodes(offsetRoot, p.language, arena)
			tree.Release()
			if len(nodes) > 0 {
				return nodes, true
			}
		}
		tree.Release()
	}
	if invocation, ok := csharpRecoverTopLevelInvocationStatementFromRange(source, start, end, p.language, arena); ok {
		return []*Node{invocation}, true
	}
	if stmt, ok := csharpRecoverTopLevelStatementFromRange(source, start, end, p, arena); ok {
		return []*Node{stmt}, true
	}
	return nil, false
}

func csharpRecoverTopLevelCommentNodeFromRange(source []byte, start, end uint32, lang *Language, arena *nodeArena) (*Node, bool) {
	if lang == nil || start >= end || int(end) > len(source) {
		return nil, false
	}
	switch {
	case start+1 < end && source[start] == '/' && source[start+1] == '/':
		lineEnd := start + 2
		for lineEnd < end && source[lineEnd] != '\n' {
			lineEnd++
		}
		if trimmedStart, trimmedEnd := csharpTrimSpaceBounds(source, lineEnd, end); trimmedStart != trimmedEnd {
			return nil, false
		}
		comment, ok := csharpBuildLeafNodeByName(arena, source, lang, "comment", start, lineEnd)
		if !ok {
			return nil, false
		}
		comment.setExtra(true)
		return comment, true
	case start+1 < end && source[start] == '/' && source[start+1] == '*':
		commentEnd := csharpFindBlockCommentEnd(source, start+2, end)
		if commentEnd <= start+1 {
			return nil, false
		}
		if trimmedStart, trimmedEnd := csharpTrimSpaceBounds(source, commentEnd, end); trimmedStart != trimmedEnd {
			return nil, false
		}
		comment, ok := csharpBuildLeafNodeByName(arena, source, lang, "comment", start, commentEnd)
		if !ok {
			return nil, false
		}
		comment.setExtra(true)
		return comment, true
	default:
		return nil, false
	}
}

func csharpExtractRecoveredTopLevelNodes(root *Node, lang *Language, arena *nodeArena) []*Node {
	if root == nil || lang == nil {
		return nil
	}
	if root.Type(lang) != "compilation_unit" {
		if !csharpIsRecoveredTopLevelDeclaration(root, lang) {
			return nil
		}
		if arena != nil {
			return []*Node{cloneTreeNodesIntoArena(root, arena)}
		}
		return []*Node{root}
	}
	out := make([]*Node, 0, root.NamedChildCount())
	for _, child := range root.children {
		if child == nil {
			continue
		}
		cur := child
		if cur.Type(lang) == "declaration" && len(cur.children) == 1 && cur.children[0] != nil {
			cur = cur.children[0]
		}
		if !csharpIsRecoveredTopLevelDeclaration(cur, lang) {
			continue
		}
		if arena != nil {
			out = append(out, cloneTreeNodesIntoArena(cur, arena))
		} else {
			out = append(out, cur)
		}
	}
	return out
}

func csharpFindBlockCommentEnd(source []byte, start, end uint32) uint32 {
	for i := start; i+1 < end && i+1 < uint32(len(source)); i++ {
		if source[i] == '*' && source[i+1] == '/' {
			return i + 2
		}
	}
	return 0
}

func normalizeCSharpRecoveredNamespaces(root *Node, source []byte, p *Parser, lang *Language) {
	if root == nil || p == nil || lang == nil || lang.Name != "c_sharp" || len(source) == 0 || root.ownerArena == nil {
		return
	}
	rootType := root.Type(lang)
	if rootType != "ERROR" && rootType != "compilation_unit" {
		return
	}
	recoveredChildren := make([]*Node, 0, len(root.children))
	changed := false
	recoveryCount := 0
	for i := 0; i < len(root.children); {
		if recoveryCount < csharpMaxNamespaceRecoveries {
			if recovered, ok := csharpRecoverErroredNamespaceDeclaration(root.children[i], source, p, lang, root.ownerArena); ok {
				recoveredChildren = append(recoveredChildren, recovered)
				i++
				changed = true
				recoveryCount++
				continue
			}
			if recovered, next, ok := csharpRecoverNamespaceFromChildren(root.children, i, source, p, lang, root.ownerArena); ok {
				recoveredChildren = append(recoveredChildren, recovered)
				i = next
				changed = true
				recoveryCount++
				continue
			}
		}
		if child := root.children[i]; child != nil {
			if recovered, ok := csharpRecoverWrappedTopLevelDeclaration(child, lang, root.ownerArena); ok {
				recoveredChildren = append(recoveredChildren, recovered)
				changed = true
			} else {
				recoveredChildren = append(recoveredChildren, child)
			}
		}
		i++
	}
	if !changed {
		return
	}
	recoveredChildren = cloneNodeSliceIfArena(root.ownerArena, recoveredChildren)
	root.children = recoveredChildren
	root.setHasError(false)
	populateParentNode(root, root.children)
	if root.Type(lang) == "ERROR" && csharpCanRecoverCompilationUnitRoot(root, lang) {
		if sym, ok := lang.SymbolByName("compilation_unit"); ok {
			retagResultRoot(root, sym, symbolIsNamed(lang, sym))
			root.setHasError(false)
			populateParentNode(root, root.children)
		}
	}
}

func csharpAcceptedErrorTreeCanUseNamespaceRecovery(tree *Tree, source []byte) bool {
	if tree == nil || len(source) == 0 || tree.language == nil || tree.language.Name != "c_sharp" {
		return false
	}
	rt := *tree.rawParseRuntime()
	if rt.StopReason != ParseStopAccepted || rt.Truncated || rt.TokenSourceEOFEarly {
		return false
	}
	if !retryTreeHasError(tree) {
		return false
	}
	root := rawRootOrNil(tree)
	if root == nil || root.ownerArena == nil {
		return false
	}
	rootType := root.Type(tree.language)
	if rootType != "ERROR" && rootType != "compilation_unit" {
		return false
	}
	for _, child := range root.children {
		if _, _, ok := csharpRecoverableErroredNamespaceDeclarationSpan(child, source, tree.language); ok {
			return true
		}
	}
	return false
}

func csharpRecoverErroredNamespaceDeclaration(n *Node, source []byte, p *Parser, lang *Language, arena *nodeArena) (*Node, bool) {
	start, end, ok := csharpRecoverableErroredNamespaceDeclarationSpan(n, source, lang)
	if !ok {
		return nil, false
	}
	return csharpBuildRecoveredNamespaceDeclarationFromErrorRoot(n, source, start, end, p, lang, arena)
}

func csharpRecoverableErroredNamespaceDeclarationSpan(n *Node, source []byte, lang *Language) (uint32, uint32, bool) {
	if n == nil || lang == nil || lang.Name != "c_sharp" || !n.HasError() || n.Type(lang) != "namespace_declaration" {
		return 0, 0, false
	}
	if n.startByte >= n.endByte || int(n.endByte) > len(source) {
		return 0, 0, false
	}
	nsStart := csharpSkipSpaceBytes(source, n.startByte)
	if nsStart >= n.endByte || !csharpHasKeywordAt(source, nsStart, "namespace") {
		return 0, 0, false
	}
	nameStart := csharpSkipSpaceBytes(source, nsStart+uint32(len("namespace")))
	if nameStart >= n.endByte {
		return 0, 0, false
	}
	openBrace := csharpFindTopLevelByte(source, nameStart, n.endByte, '{')
	if openBrace >= n.endByte {
		return 0, 0, false
	}
	closeBrace := csharpFindMatchingBraceByte(source, int(openBrace), int(n.endByte))
	if closeBrace < 0 {
		return 0, 0, false
	}
	nsEnd := uint32(closeBrace + 1)
	if nsEnd > n.endByte || !bytesAreTrivia(source[nsEnd:n.endByte]) {
		return 0, 0, false
	}
	return nsStart, nsEnd, true
}

func csharpRecoverNamespaceFromChildren(children []*Node, startIdx int, source []byte, p *Parser, lang *Language, arena *nodeArena) (*Node, int, bool) {
	if startIdx < 0 || startIdx >= len(children) || p == nil || lang == nil || arena == nil {
		return nil, startIdx, false
	}
	startNode := children[startIdx]
	if startNode == nil || int(startNode.startByte) >= len(source) {
		return nil, startIdx, false
	}
	switch startNode.Type(lang) {
	case "ERROR", "global_statement", "statement":
	default:
		return nil, startIdx, false
	}
	nsStart := csharpSkipSpaceBytes(source, startNode.startByte)
	// SHORT-TERM RELIEF for issue #136: for a large real-world file the
	// leading boilerplate (copyright header comments, #region/#endregion,
	// using directives) can collapse into the SAME opaque ERROR span as the
	// namespace keyword itself, so "namespace" is not at this child's own
	// start byte even though it does appear within its span. Search forward
	// within the ERROR span for the namespace keyword instead of giving up
	// outright — but skip csharpRecoverNamespaceNodeFromRange's initial
	// full-body reparse in that case (foundViaSearch below): that reparse
	// re-runs the GLR parser over the SAME content that just failed to
	// produce this child in the first place, so it is both unlikely to help
	// (the reparse dies at essentially the same relative point — verified on
	// the #136 repro files) and, on some file shapes, catastrophically
	// slower than the bounded, purely source-based recovery in
	// csharpBuildRecoveredNamespaceDeclarationFromErrorRoot. Go straight to
	// that (with startNode itself as the best-effort "already parsed
	// fragment" source, since it may still have some usable children).
	foundViaSearch := false
	if int(nsStart)+len("namespace") > len(source) || !bytes.HasPrefix(source[nsStart:], []byte("namespace")) {
		if startNode.Type(lang) != "ERROR" {
			return nil, startIdx, false
		}
		found, ok := csharpFindTopLevelNamespaceKeyword(source, startNode.startByte, startNode.endByte)
		if !ok {
			return nil, startIdx, false
		}
		nsStart = found
		foundViaSearch = true
	}
	openRel := bytes.IndexByte(source[nsStart:], '{')
	if openRel < 0 {
		return nil, startIdx, false
	}
	openBrace := int(nsStart) + openRel
	closeBrace := csharpFindMatchingBraceByte(source, openBrace, len(source))
	if closeBrace < 0 {
		return nil, startIdx, false
	}
	nsEnd := uint32(closeBrace + 1)
	var recovered *Node
	var ok bool
	if foundViaSearch {
		recovered, ok = csharpBuildRecoveredNamespaceDeclarationFromErrorRoot(startNode, source, nsStart, nsEnd, p, lang, arena)
	} else {
		recovered, ok = csharpRecoverNamespaceNodeFromRange(source, nsStart, nsEnd, p, lang, arena)
	}
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
		if child.startByte >= nsEnd {
			break
		}
		nextIdx++
	}
	return recovered, nextIdx, true
}

func csharpRecoverNamespaceNodeFromRange(source []byte, start, end uint32, p *Parser, lang *Language, arena *nodeArena) (*Node, bool) {
	if p == nil || lang == nil || arena == nil || start >= end || int(end) > len(source) {
		return nil, false
	}
	tree, err := p.parseForRecovery(source[start:end])
	if err != nil || tree == nil || tree.RootNode() == nil {
		if tree != nil {
			tree.Release()
		}
		return nil, false
	}
	defer tree.Release()
	startPoint := advancePointByBytes(Point{}, source[:start])
	offsetRoot := tree.RootNodeWithOffset(start, startPoint)
	if offsetRoot == nil {
		return nil, false
	}
	if node, ok := csharpExtractRecoveredTopLevelNode(offsetRoot, lang, arena, end, "namespace_declaration"); ok {
		return node, true
	}
	// The namespace body did not parse cleanly. Reuse the same sub-parse to
	// recover a best-effort namespace_declaration containing the declarations
	// that did parse (issue #115).
	return csharpBuildRecoveredNamespaceDeclarationFromErrorRoot(offsetRoot, source, start, end, p, lang, arena)
}

func csharpRecoverWrappedTopLevelDeclaration(n *Node, lang *Language, arena *nodeArena) (*Node, bool) {
	if n == nil || lang == nil || arena == nil || n.Type(lang) != "ERROR" {
		return nil, false
	}
	var candidate *Node
	for _, child := range n.children {
		if child == nil {
			continue
		}
		cur := child
		if cur.Type(lang) == "declaration" && len(cur.children) == 1 && cur.children[0] != nil {
			cur = cur.children[0]
		}
		if !csharpIsRecoveredTopLevelDeclaration(cur, lang) {
			continue
		}
		if candidate != nil {
			return nil, false
		}
		candidate = cur
	}
	if candidate == nil {
		return nil, false
	}
	return cloneTreeNodesIntoArena(candidate, arena), true
}

func csharpExtractRecoveredTopLevelNode(root *Node, lang *Language, arena *nodeArena, wantEnd uint32, wantType string) (*Node, bool) {
	if root == nil || lang == nil || arena == nil {
		return nil, false
	}
	var walk func(*Node) *Node
	walk = func(n *Node) *Node {
		if n == nil {
			return nil
		}
		if n.Type(lang) == wantType && !n.HasError() && n.endByte == wantEnd {
			return n
		}
		for i := 0; i < n.ChildCount(); i++ {
			if got := walk(n.Child(i)); got != nil {
				return got
			}
		}
		return nil
	}
	node := walk(root)
	if node == nil {
		return nil, false
	}
	return cloneTreeNodesIntoArena(node, arena), true
}

func normalizeCSharpRecoveredTypeDeclarations(root *Node, source []byte, p *Parser, lang *Language) {
	if root == nil || lang == nil || lang.Name != "c_sharp" || root.Type(lang) != "ERROR" || len(source) == 0 || root.ownerArena == nil {
		return
	}
	compilationUnitSym, ok := lang.SymbolByName("compilation_unit")
	if !ok {
		return
	}
	compilationUnitNamed := symbolIsNamed(lang, compilationUnitSym)
	recoveredChildren := make([]*Node, 0, len(root.children))
	for i := 0; i < len(root.children); {
		child := root.children[i]
		if child == nil {
			i++
			continue
		}
		if recovered, next, ok := csharpRecoverAttributedTopLevelTypeDeclarationFromChildren(root.children, i, source, p, lang, root.ownerArena); ok {
			recoveredChildren = append(recoveredChildren, recovered)
			i = next
			continue
		}
		if recovered, next, ok := csharpRecoverNonEmptyTopLevelTypeDeclarationFromChildren(root.children, i, source, p, lang, root.ownerArena, false); ok {
			recoveredChildren = append(recoveredChildren, recovered)
			i = next
			continue
		}
		if csharpIsRecoveredTopLevelDeclaration(child, lang) {
			recoveredChildren = append(recoveredChildren, child)
			i++
			continue
		}
		attributed, ok := csharpRecoverAttributedTopLevelTypeDeclarationFromError(child, source, p, lang, root.ownerArena)
		if ok {
			recoveredChildren = append(recoveredChildren, attributed)
			i++
			continue
		}
		nonEmpty, ok := csharpRecoverNonEmptyTypeDeclarationFromError(child, source, p, lang, root.ownerArena, false)
		if ok {
			recoveredChildren = append(recoveredChildren, nonEmpty)
			i++
			continue
		}
		recovered, ok := csharpRecoverEmptyTypeDeclarationFromError(child, source, lang, root.ownerArena)
		if !ok {
			return
		}
		recoveredChildren = append(recoveredChildren, recovered)
		i++
	}
	if len(recoveredChildren) == 0 {
		return
	}
	recoveredChildren = cloneNodeSliceIfArena(root.ownerArena, recoveredChildren)
	retagResultRoot(root, compilationUnitSym, compilationUnitNamed)
	replaceNodeChildrenUnfielded(root, recoveredChildren)
	root.productionID = 0
	root.setHasError(false)
}

func csharpCanRecoverCompilationUnitRoot(root *Node, lang *Language) bool {
	if root == nil || lang == nil {
		return false
	}
	sawTopLevel := false
	for _, child := range root.children {
		if child == nil {
			continue
		}
		if !csharpIsRecoveredTopLevelDeclaration(child, lang) {
			return false
		}
		sawTopLevel = true
	}
	return sawTopLevel
}

func csharpIsRecoveredTopLevelDeclaration(n *Node, lang *Language) bool {
	if n == nil || lang == nil {
		return false
	}
	switch n.Type(lang) {
	case "class_declaration", "struct_declaration", "record_declaration", "interface_declaration", "enum_declaration", "delegate_declaration", "namespace_declaration", "file_scoped_namespace_declaration", "using_directive", "extern_alias_directive", "global_statement", "comment",
		"preproc_region", "preproc_endregion", "preproc_if", "preproc_define", "preproc_undef", "preproc_pragma", "preproc_nullable":
		return true
	default:
		return false
	}
}

func csharpRecoverEmptyTypeDeclarationFromError(n *Node, source []byte, lang *Language, arena *nodeArena) (*Node, bool) {
	if n == nil || lang == nil || arena == nil || n.Type(lang) != "ERROR" || len(n.children) == 0 {
		return nil, false
	}
	type recoverySpec struct {
		initName string
		declName string
	}
	specs := []recoverySpec{
		{initName: "_class_declaration_initializer", declName: "class_declaration"},
		{initName: "_struct_declaration_initializer", declName: "struct_declaration"},
		{initName: "_record_declaration_initializer", declName: "record_declaration"},
	}
	for _, spec := range specs {
		for _, child := range n.children {
			if child == nil || child.Type(lang) != spec.initName {
				continue
			}
			return csharpBuildRecoveredEmptyTypeDeclaration(n, child, source, lang, arena, spec.declName)
		}
	}
	return nil, false
}

func csharpBuildRecoveredEmptyTypeDeclaration(errNode, initNode *Node, source []byte, lang *Language, arena *nodeArena, declName string) (*Node, bool) {
	if errNode == nil || initNode == nil || lang == nil || arena == nil || int(errNode.endByte) > len(source) {
		return nil, false
	}
	openRel := bytes.IndexByte(source[initNode.endByte:errNode.endByte], '{')
	if openRel < 0 {
		return nil, false
	}
	openBrace := int(initNode.endByte) + openRel
	closeBrace := csharpFindMatchingBraceByte(source, openBrace, int(errNode.endByte))
	if closeBrace < 0 || closeBrace <= openBrace || !bytesAreTrivia(source[openBrace+1:closeBrace]) {
		return nil, false
	}
	declSym, ok := lang.SymbolByName(declName)
	if !ok {
		return nil, false
	}
	declNamed := symbolIsNamed(lang, declSym)
	declList, ok := csharpBuildEmptyDeclarationListNode(arena, source, lang, uint32(openBrace), uint32(closeBrace))
	if !ok {
		return nil, false
	}
	children := make([]*Node, 0, len(initNode.children)+1)
	for _, child := range initNode.children {
		if child != nil {
			children = append(children, child)
		}
	}
	children = append(children, declList)
	if arena != nil {
		buf := arena.allocNodeSlice(len(children))
		copy(buf, children)
		children = buf
	}
	recovered := newParentNodeInArena(arena, declSym, declNamed, children, nil, 0)
	recovered.setHasError(false)
	return recovered, true
}

func csharpBuildEmptyDeclarationListNode(arena *nodeArena, source []byte, lang *Language, openBrace, closeBrace uint32) (*Node, bool) {
	sym, ok := lang.SymbolByName("declaration_list")
	if !ok {
		return nil, false
	}
	openTok, ok := csharpBuildLeafNodeByName(arena, source, lang, "{", openBrace, openBrace+1)
	if !ok {
		return nil, false
	}
	closeTok, ok := csharpBuildLeafNodeByName(arena, source, lang, "}", closeBrace, closeBrace+1)
	if !ok {
		return nil, false
	}
	named := symbolIsNamed(lang, sym)
	return newParentNodeInArena(arena, sym, named, []*Node{openTok, closeTok}, nil, 0), true
}
