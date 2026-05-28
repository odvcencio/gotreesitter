package gotreesitter

type typeScriptNormalizationContext struct {
	source []byte
	lang   *Language

	canRewriteGenericCalls      bool
	canRewriteInstantiatedCalls bool
	canRewriteAsExpressions     bool
	canRewriteGenericArrows     bool
	canRewriteClassDeclarations bool
	canClearEnumBodyFields      bool

	callSym                    Symbol
	callNamed                  bool
	instantiationExprSym       Symbol
	instantiationExprNamed     bool
	typeArgsSym                Symbol
	typeArgsNamed              bool
	argsSym                    Symbol
	argsNamed                  bool
	predefinedTypeSym          Symbol
	predefinedTypeNamed        bool
	asExpressionSym            Symbol
	asExpressionNamed          bool
	functionFieldID            FieldID
	typeArgsFieldID            FieldID
	argumentsFieldID           FieldID
	binaryExpressionSym        Symbol
	assignmentExprSym          Symbol
	assignmentExprNamed        bool
	ternaryExprSym             Symbol
	ternaryExprNamed           bool
	unionTypeSym               Symbol
	unionTypeNamed             bool
	intersectionTypeSym        Symbol
	intersectionTypeNamed      bool
	literalTypeSym             Symbol
	literalTypeNamed           bool
	hasLiteralTypeSym          bool
	objectTypeSym              Symbol
	objectTypeNamed            bool
	propertySignatureSym       Symbol
	propertySignatureNamed     bool
	publicFieldDefinitionSym   Symbol
	abstractMethodSignatureSym Symbol
	typeAnnotationSym          Symbol
	typeAnnotationNamed        bool
	methodDefinitionSym        Symbol
	methodSignatureSym         Symbol
	accessibilityModSym        Symbol
	accessibilityModNamed      bool
	objectSym                  Symbol
	pairSym                    Symbol
	propertyIdentifierSym      Symbol
	propertyIdentifierNamed    bool
	colonSym                   Symbol
	publicSym                  Symbol
	privateSym                 Symbol
	protectedSym               Symbol
	staticSym                  Symbol
	readonlySym                Symbol
	abstractSym                Symbol
	asyncSym                   Symbol
	getSym                     Symbol
	setSym                     Symbol
	numberSym                  Symbol
	stringSym                  Symbol
	stringFragmentSym          Symbol
	greaterThanSym             Symbol
	pipeSym                    Symbol
	ampersandSym               Symbol
	hasPipeSym                 bool
	hasAmpersandSym            bool
	parenthesizedExprSym       Symbol
	lessThanSym                Symbol
	identifierSym              Symbol
	memberExpressionSym        Symbol
	sequenceExpressionSym      Symbol
	typeIdentifierSym          Symbol
	typeIdentifierNamed        bool
	hasTypeIdentifierSym       bool
	typeAssertionSym           Symbol
	arrowFunctionSym           Symbol
	typeParametersSym          Symbol
	typeParametersNamed        bool
	typeParameterSym           Symbol
	typeParameterNamed         bool
	expressionStatementSym     Symbol
	classSym                   Symbol
	classDeclarationSym        Symbol
	classDeclarationNamed      bool
	enumBodySym                Symbol
	enumAssignmentSym          Symbol
	importSym                  Symbol
	hasImportSym               bool
	typeQuerySym               Symbol
	nameFieldID                FieldID
	typeParametersFieldID      FieldID
	parametersFieldID          FieldID
	returnTypeFieldID          FieldID
	typeFieldID                FieldID
	bodyFieldID                FieldID
	valueFieldID               FieldID
}

func normalizeTypeScriptCompatibility(root *Node, source []byte, lang *Language) {
	ctx, ok := newTypeScriptNormalizationContext(source, lang)
	if !ok || root == nil {
		return
	}

	walkResultTreeDenseFirst(root, func(n *Node) {
		switch n.symbol {
		case ctx.identifierSym:
			normalizeTypeScriptIdentifierKeywordAliases(n, &ctx)
		case ctx.importSym:
			if ctx.hasImportSym {
				normalizeTypeScriptImportKeywordNamedness(n, &ctx)
			}
		case ctx.methodDefinitionSym, ctx.methodSignatureSym, ctx.abstractMethodSignatureSym, ctx.propertySignatureSym, ctx.publicFieldDefinitionSym:
			if ctx.accessibilityModSym != 0 {
				normalizeTypeScriptRecoveredMemberModifiers(n, &ctx)
			}
		case ctx.enumBodySym:
			if ctx.canClearEnumBodyFields && len(n.fieldIDs) > 0 {
				normalizeTypeScriptEnumBodyCompatibility(n, &ctx)
			}
		}
		for i, child := range n.children {
			for {
				if !typeScriptCompatibilityChildCanRewrite(child, &ctx) {
					break
				}
				rewritten := rewriteTypeScriptCompatibilityChild(child, &ctx)
				if rewritten == nil {
					break
				}
				n.children[i] = rewritten
				rewritten.parent = n
				rewritten.childIndex = int32(i)
				child = rewritten
			}
		}
	})
}

type typeScriptCompatibilityStats struct {
	total                       normalizationPassCounters
	identifierAliases           normalizationPassCounters
	importKeywords              normalizationPassCounters
	memberModifiers             normalizationPassCounters
	enumBodies                  normalizationPassCounters
	binaryChildren              normalizationPassCounters
	binaryGenericChildren       normalizationPassCounters
	binaryAsTypeChildren        normalizationPassCounters
	binaryFastSkipped           normalizationPassCounters
	callChildren                normalizationPassCounters
	callInstantiatedChildren    normalizationPassCounters
	callFastSkipped             normalizationPassCounters
	asChildren                  normalizationPassCounters
	typeAssertionChildren       normalizationPassCounters
	expressionStatementChildren normalizationPassCounters
}

func normalizeTypeScriptCompatibilityWithStats(root *Node, source []byte, lang *Language) typeScriptCompatibilityStats {
	var stats typeScriptCompatibilityStats
	ctx, ok := newTypeScriptNormalizationContext(source, lang)
	if !ok || root == nil {
		return stats
	}

	walkResultTreeDenseFirst(root, func(n *Node) {
		stats.total.nodesVisited++
		switch n.symbol {
		case ctx.identifierSym:
			stats.identifierAliases.nodesVisited++
			before := len(n.children)
			normalizeTypeScriptIdentifierKeywordAliases(n, &ctx)
			if len(n.children) != before {
				stats.identifierAliases.nodesRewritten++
				stats.total.nodesRewritten++
			}
		case ctx.importSym:
			if ctx.hasImportSym {
				stats.importKeywords.nodesVisited++
				before := n.isNamed()
				normalizeTypeScriptImportKeywordNamedness(n, &ctx)
				if n.isNamed() != before {
					stats.importKeywords.nodesRewritten++
					stats.total.nodesRewritten++
				}
			}
		case ctx.methodDefinitionSym, ctx.methodSignatureSym, ctx.abstractMethodSignatureSym, ctx.propertySignatureSym, ctx.publicFieldDefinitionSym:
			if ctx.accessibilityModSym != 0 {
				stats.memberModifiers.nodesVisited++
				beforeSymbol := n.symbol
				beforeChildren := len(n.children)
				normalizeTypeScriptRecoveredMemberModifiers(n, &ctx)
				if n.symbol != beforeSymbol || len(n.children) != beforeChildren {
					stats.memberModifiers.nodesRewritten++
					stats.total.nodesRewritten++
				}
			}
		case ctx.enumBodySym:
			if ctx.canClearEnumBodyFields && len(n.fieldIDs) > 0 {
				stats.enumBodies.nodesVisited++
				beforeCleared := typeScriptEnumBodyClearedFieldCount(n, &ctx)
				normalizeTypeScriptEnumBodyCompatibility(n, &ctx)
				if typeScriptEnumBodyClearedFieldCount(n, &ctx) != beforeCleared {
					stats.enumBodies.nodesRewritten++
					stats.total.nodesRewritten++
				}
			}
		}
		for i, child := range n.children {
			for {
				bucket := typeScriptCompatibilityChildStatsBucket(child, &ctx, &stats)
				if bucket == nil {
					break
				}
				binaryGenericCandidate := false
				binaryAsTypeCandidate := false
				if child != nil && child.symbol == ctx.binaryExpressionSym {
					binaryGenericCandidate = ctx.canRewriteGenericCalls && typeScriptBinaryOperatorCouldBeGenericCall(child, &ctx)
					binaryAsTypeCandidate = ctx.canRewriteAsExpressions && typeScriptBinaryOperatorCouldBeAsTypeChain(child, &ctx)
					if binaryGenericCandidate {
						stats.binaryGenericChildren.nodesVisited++
					}
					if binaryAsTypeCandidate {
						stats.binaryAsTypeChildren.nodesVisited++
					}
					if !binaryGenericCandidate && !binaryAsTypeCandidate {
						stats.binaryFastSkipped.nodesVisited++
					}
				}
				callInstantiatedCandidate := false
				if child != nil && child.symbol == ctx.callSym && ctx.canRewriteInstantiatedCalls {
					callInstantiatedCandidate = typeScriptCallCouldBeInstantiated(child, &ctx)
					if callInstantiatedCandidate {
						stats.callInstantiatedChildren.nodesVisited++
					} else {
						stats.callFastSkipped.nodesVisited++
					}
				}
				bucket.nodesVisited++
				rewritten := rewriteTypeScriptCompatibilityChild(child, &ctx)
				if rewritten == nil {
					break
				}
				n.children[i] = rewritten
				rewritten.parent = n
				rewritten.childIndex = int32(i)
				if bucket != nil {
					bucket.nodesRewritten++
				}
				if binaryGenericCandidate {
					stats.binaryGenericChildren.nodesRewritten++
				} else if binaryAsTypeCandidate {
					stats.binaryAsTypeChildren.nodesRewritten++
				}
				if callInstantiatedCandidate {
					stats.callInstantiatedChildren.nodesRewritten++
				}
				stats.total.nodesRewritten++
				child = rewritten
			}
		}
	})
	return stats
}

func typeScriptCompatibilityChildCanRewrite(child *Node, ctx *typeScriptNormalizationContext) bool {
	if child == nil || ctx == nil {
		return false
	}
	switch child.symbol {
	case ctx.binaryExpressionSym:
		return ctx.canRewriteGenericCalls || ctx.canRewriteAsExpressions
	case ctx.callSym:
		return ctx.canRewriteInstantiatedCalls
	case ctx.asExpressionSym:
		return ctx.canRewriteAsExpressions
	case ctx.typeAssertionSym:
		return ctx.canRewriteGenericArrows
	case ctx.expressionStatementSym:
		return ctx.canRewriteClassDeclarations
	default:
		return false
	}
}

func typeScriptCompatibilityChildStatsBucket(child *Node, ctx *typeScriptNormalizationContext, stats *typeScriptCompatibilityStats) *normalizationPassCounters {
	if child == nil || ctx == nil || stats == nil {
		return nil
	}
	switch child.symbol {
	case ctx.binaryExpressionSym:
		if ctx.canRewriteGenericCalls || ctx.canRewriteAsExpressions {
			return &stats.binaryChildren
		}
	case ctx.callSym:
		if ctx.canRewriteInstantiatedCalls {
			return &stats.callChildren
		}
	case ctx.asExpressionSym:
		if ctx.canRewriteAsExpressions {
			return &stats.asChildren
		}
	case ctx.typeAssertionSym:
		if ctx.canRewriteGenericArrows {
			return &stats.typeAssertionChildren
		}
	case ctx.expressionStatementSym:
		if ctx.canRewriteClassDeclarations {
			return &stats.expressionStatementChildren
		}
	}
	return nil
}

func normalizeTypeScriptEnumBodyCompatibility(n *Node, ctx *typeScriptNormalizationContext) {
	if n == nil || ctx == nil || !ctx.canClearEnumBodyFields || n.symbol != ctx.enumBodySym || len(n.fieldIDs) == 0 {
		return
	}
	limit := len(n.children)
	if len(n.fieldIDs) < limit {
		limit = len(n.fieldIDs)
	}
	for i := 0; i < limit; i++ {
		child := n.children[i]
		if child == nil || child.symbol != ctx.enumAssignmentSym {
			continue
		}
		n.fieldIDs[i] = 0
		if len(n.fieldSources) > i {
			n.fieldSources[i] = fieldSourceNone
		}
	}
}

func typeScriptEnumBodyClearedFieldCount(n *Node, ctx *typeScriptNormalizationContext) int {
	if n == nil || ctx == nil || len(n.fieldIDs) == 0 {
		return 0
	}
	limit := len(n.children)
	if len(n.fieldIDs) < limit {
		limit = len(n.fieldIDs)
	}
	cleared := 0
	for i := 0; i < limit; i++ {
		child := n.children[i]
		if child != nil && child.symbol == ctx.enumAssignmentSym && n.fieldIDs[i] == 0 {
			cleared++
		}
	}
	return cleared
}

func rewriteTypeScriptCompatibilityChild(child *Node, ctx *typeScriptNormalizationContext) *Node {
	if child == nil || ctx == nil {
		return nil
	}
	switch child.symbol {
	case ctx.binaryExpressionSym:
		if ctx.canRewriteGenericCalls {
			if typeScriptBinaryOperatorCouldBeGenericCall(child, ctx) {
				if rewritten := rewriteTypeScriptPredefinedGenericCall(child, ctx); rewritten != nil {
					return rewritten
				}
			}
		}
		if ctx.canRewriteAsExpressions && typeScriptBinaryOperatorCouldBeAsTypeChain(child, ctx) {
			return rewriteTypeScriptAsTypeChain(child, ctx)
		}
	case ctx.callSym:
		if ctx.canRewriteInstantiatedCalls && typeScriptCallCouldBeInstantiated(child, ctx) {
			return rewriteTypeScriptInstantiatedCall(child, ctx)
		}
	case ctx.asExpressionSym:
		if ctx.canRewriteAsExpressions {
			return rewriteTypeScriptAsAssignmentOrTernary(child, ctx)
		}
	case ctx.typeAssertionSym:
		if ctx.canRewriteGenericArrows {
			return rewriteTypeScriptGenericArrowTypeAssertion(child, ctx)
		}
	case ctx.expressionStatementSym:
		if ctx.canRewriteClassDeclarations {
			return rewriteTypeScriptClassExpressionStatement(child, ctx)
		}
	}
	return nil
}

func typeScriptBinaryOperatorCouldBeGenericCall(node *Node, ctx *typeScriptNormalizationContext) bool {
	op, ok := typeScriptBinaryExpressionOperator(node, ctx)
	if !ok {
		return false
	}
	if op.symbol == ctx.greaterThanSym {
		return true
	}
	return typeScriptBinaryExpressionHasGenericCallClose(node, ctx)
}

func typeScriptBinaryOperatorCouldBeAsTypeChain(node *Node, ctx *typeScriptNormalizationContext) bool {
	if !ctx.hasPipeSym || !ctx.hasAmpersandSym {
		return true
	}
	op, ok := typeScriptBinaryExpressionOperator(node, ctx)
	return ok && (op.symbol == ctx.pipeSym || op.symbol == ctx.ampersandSym)
}

func typeScriptBinaryExpressionOperator(node *Node, ctx *typeScriptNormalizationContext) (*Node, bool) {
	if node == nil || ctx == nil || node.symbol != ctx.binaryExpressionSym || len(node.children) != 3 {
		return nil, false
	}
	op := node.children[1]
	if op == nil {
		return nil, false
	}
	return op, true
}

func typeScriptCallCouldBeInstantiated(node *Node, ctx *typeScriptNormalizationContext) bool {
	if node == nil || ctx == nil || node.symbol != ctx.callSym || len(node.children) != 2 {
		return false
	}
	function := node.children[0]
	arguments := node.children[1]
	return function != nil &&
		arguments != nil &&
		function.symbol == ctx.instantiationExprSym &&
		arguments.symbol == ctx.argsSym
}

func normalizeTypeScriptIdentifierKeywordAliases(node *Node, ctx *typeScriptNormalizationContext) {
	if node == nil || ctx == nil || ctx.lang == nil || node.symbol != ctx.identifierSym || len(node.children) != 1 {
		return
	}
	child := node.children[0]
	if child == nil || child.IsNamed() || child.IsExtra() {
		return
	}
	if child.startByte != node.startByte || child.endByte != node.endByte || child.startPoint != node.startPoint || child.endPoint != node.endPoint {
		return
	}
	node.children = nil
	node.fieldIDs = nil
	node.fieldSources = nil
}

func normalizeTypeScriptImportKeywordNamedness(node *Node, ctx *typeScriptNormalizationContext) {
	if node == nil || ctx == nil || !ctx.hasImportSym || node.symbol != ctx.importSym {
		return
	}
	if typeScriptNextNonspaceByte(ctx.source, node.endByte) == '(' {
		node.setNamed(true)
		return
	}
	node.setNamed(false)
}

func normalizeTypeScriptRecoveredNamespaceRoot(root *Node, source []byte, lang *Language) {
	if root == nil || lang == nil || len(root.children) < 4 {
		return
	}
	if lang.Name != "tsx" && lang.Name != "typescript" {
		return
	}
	rootType := root.Type(lang)
	if rootType != "ERROR" && rootType != "program" {
		return
	}
	stmtBlockSym, ok := lang.SymbolByName("statement_block")
	if !ok {
		return
	}
	internalModuleSym, ok := lang.SymbolByName("internal_module")
	if !ok {
		return
	}
	exprStmtSym, hasExprStmtSym := lang.SymbolByName("expression_statement")
	programSym, hasProgramSym := lang.SymbolByName("program")

	namespaceIdx := -1
	for i, child := range root.children {
		if child == nil || child.isExtra() {
			continue
		}
		if child.Type(lang) != "namespace" {
			if child.symbol == internalModuleSym {
				normalizeTypeScriptRecoveredInternalModuleRoot(root, source, lang, child, i, stmtBlockSym, exprStmtSym, hasExprStmtSym, programSym, hasProgramSym)
				return
			}
			if child.Type(lang) != "comment" {
				return
			}
			continue
		}
		namespaceIdx = i
		break
	}
	if namespaceIdx < 0 || namespaceIdx+2 >= len(root.children) {
		return
	}
	nameNode := root.children[namespaceIdx+1]
	openBrace := root.children[namespaceIdx+2]
	if nameNode == nil || openBrace == nil || nameNode.Type(lang) != "identifier" || openBrace.Type(lang) != "{" {
		return
	}

	bodyChildren := make([]*Node, 0, len(root.children)-(namespaceIdx+3))
	for i := namespaceIdx + 3; i < len(root.children); i++ {
		child := root.children[i]
		if child == nil {
			continue
		}
		if typeScriptWhitespaceOnlyRecoverySubtree(child, source) {
			continue
		}
		bodyChildren = append(bodyChildren, child)
	}
	if len(bodyChildren) == 0 {
		return
	}
	if root.ownerArena != nil {
		buf := root.ownerArena.allocNodeSlice(len(bodyChildren))
		copy(buf, bodyChildren)
		bodyChildren = buf
	}

	stmtBlockNamed := symbolIsNamed(lang, stmtBlockSym)
	internalModuleNamed := symbolIsNamed(lang, internalModuleSym)
	block := newParentNodeInArena(root.ownerArena, stmtBlockSym, stmtBlockNamed, bodyChildren, nil, 0)
	block.startByte = openBrace.startByte
	block.startPoint = openBrace.startPoint
	if len(bodyChildren) > 0 {
		last := bodyChildren[len(bodyChildren)-1]
		block.endByte = last.endByte
		block.endPoint = last.endPoint
	}

	moduleChildren := []*Node{nameNode, block}
	if root.ownerArena != nil {
		buf := root.ownerArena.allocNodeSlice(len(moduleChildren))
		copy(buf, moduleChildren)
		moduleChildren = buf
	}
	internalModule := newParentNodeInArena(root.ownerArena, internalModuleSym, internalModuleNamed, moduleChildren, nil, 0)
	internalModule.startByte = root.children[namespaceIdx].startByte
	internalModule.startPoint = root.children[namespaceIdx].startPoint
	internalModule.endByte = block.endByte
	internalModule.endPoint = block.endPoint

	wrapped := internalModule
	if hasExprStmtSym {
		exprStmtNamed := symbolIsNamed(lang, exprStmtSym)
		exprChildren := []*Node{internalModule}
		if root.ownerArena != nil {
			buf := root.ownerArena.allocNodeSlice(1)
			buf[0] = internalModule
			exprChildren = buf
		}
		exprStmt := newParentNodeInArena(root.ownerArena, exprStmtSym, exprStmtNamed, exprChildren, nil, 0)
		exprStmt.startByte = internalModule.startByte
		exprStmt.startPoint = internalModule.startPoint
		exprStmt.endByte = internalModule.endByte
		exprStmt.endPoint = internalModule.endPoint
		wrapped = exprStmt
	}

	newChildren := make([]*Node, 0, namespaceIdx+1)
	for i := 0; i < namespaceIdx; i++ {
		if root.children[i] != nil {
			newChildren = append(newChildren, root.children[i])
		}
	}
	newChildren = append(newChildren, wrapped)
	if root.ownerArena != nil {
		buf := root.ownerArena.allocNodeSlice(len(newChildren))
		copy(buf, newChildren)
		newChildren = buf
	}
	if hasProgramSym {
		retagResultRoot(root, programSym, symbolIsNamed(lang, programSym))
	}
	replaceNodeChildrenUnfielded(root, newChildren)
}

func normalizeTypeScriptRecoveredInternalModuleRoot(root *Node, source []byte, lang *Language, module *Node, moduleIdx int, stmtBlockSym, exprStmtSym Symbol, hasExprStmtSym bool, programSym Symbol, hasProgramSym bool) {
	if root == nil || module == nil || moduleIdx+1 >= len(root.children) {
		return
	}
	openBrace := root.children[moduleIdx+1]
	if openBrace == nil || openBrace.startByte >= openBrace.endByte || int(openBrace.endByte) > len(source) || string(source[openBrace.startByte:openBrace.endByte]) != "{" {
		return
	}

	bodyChildren := make([]*Node, 0, len(root.children)-(moduleIdx+2))
	for i := moduleIdx + 2; i < len(root.children); i++ {
		child := root.children[i]
		if child == nil || typeScriptWhitespaceOnlyRecoverySubtree(child, source) || typeScriptRecoveredNamespaceCloseBrace(child, source) {
			continue
		}
		if countFlattenedHiddenChildren(child, lang.SymbolMetadata) > 0 {
			count := countFlattenedHiddenChildren(child, lang.SymbolMetadata)
			start := len(bodyChildren)
			bodyChildren = append(bodyChildren, make([]*Node, count)...)
			appendFlattenedHiddenChildren(bodyChildren[start:], 0, child, lang.SymbolMetadata)
			continue
		}
		bodyChildren = append(bodyChildren, child)
	}
	if len(bodyChildren) == 0 {
		return
	}
	if root.ownerArena != nil {
		buf := root.ownerArena.allocNodeSlice(len(bodyChildren))
		copy(buf, bodyChildren)
		bodyChildren = buf
	}

	stmtBlockNamed := symbolIsNamed(lang, stmtBlockSym)
	block := newParentNodeInArena(root.ownerArena, stmtBlockSym, stmtBlockNamed, bodyChildren, nil, 0)
	block.startByte = openBrace.startByte
	block.startPoint = openBrace.startPoint
	last := bodyChildren[len(bodyChildren)-1]
	block.endByte = last.endByte
	block.endPoint = last.endPoint
	populateParentNode(block, block.children)

	moduleChildren := phpAllocChildren(root.ownerArena, len(module.children)+1)
	copy(moduleChildren, module.children)
	moduleChildren[len(module.children)] = block
	rewrittenModule := cloneNodeInArena(root.ownerArena, module)
	rewrittenModule.children = moduleChildren
	rewrittenModule.endByte = block.endByte
	rewrittenModule.endPoint = block.endPoint
	populateParentNode(rewrittenModule, rewrittenModule.children)

	wrapped := rewrittenModule
	if hasExprStmtSym {
		exprChildren := phpAllocChildren(root.ownerArena, 1)
		exprChildren[0] = rewrittenModule
		exprStmt := newParentNodeInArena(root.ownerArena, exprStmtSym, symbolIsNamed(lang, exprStmtSym), exprChildren, nil, 0)
		exprStmt.startByte = rewrittenModule.startByte
		exprStmt.startPoint = rewrittenModule.startPoint
		exprStmt.endByte = rewrittenModule.endByte
		exprStmt.endPoint = rewrittenModule.endPoint
		populateParentNode(exprStmt, exprStmt.children)
		wrapped = exprStmt
	}

	newChildren := make([]*Node, 0, moduleIdx+1)
	for i := 0; i < moduleIdx; i++ {
		if root.children[i] != nil {
			newChildren = append(newChildren, root.children[i])
		}
	}
	newChildren = append(newChildren, wrapped)
	if root.ownerArena != nil {
		buf := root.ownerArena.allocNodeSlice(len(newChildren))
		copy(buf, newChildren)
		newChildren = buf
	}
	if hasProgramSym {
		retagResultRoot(root, programSym, symbolIsNamed(lang, programSym))
	}
	replaceNodeChildrenUnfielded(root, newChildren)
}

func typeScriptRecoveredNamespaceCloseBrace(node *Node, source []byte) bool {
	if node == nil || node.startByte >= node.endByte || int(node.endByte) > len(source) {
		return false
	}
	return string(source[node.startByte:node.endByte]) == "}"
}

func typeScriptWhitespaceOnlyRecoverySubtree(node *Node, source []byte) bool {
	if node == nil || (!node.HasError() && node.symbol != errorSymbol) {
		return false
	}
	if int(node.endByte) > len(source) || node.startByte > node.endByte {
		return false
	}
	return bytesAreTrivia(source[node.startByte:node.endByte])
}

func newTypeScriptNormalizationContext(source []byte, lang *Language) (typeScriptNormalizationContext, bool) {
	ctx := typeScriptNormalizationContext{
		source: source,
		lang:   lang,
	}
	if lang == nil {
		return ctx, false
	}
	switch lang.Name {
	case "tsx", "typescript":
	default:
		return ctx, false
	}

	if syms, ok := languageSymbols(lang,
		"call_expression",
		"type_arguments",
		"arguments",
		"predefined_type",
		"binary_expression",
		">",
		"parenthesized_expression",
		"<",
		"identifier",
		"member_expression",
		"sequence_expression",
	); ok {
		ctx.canRewriteGenericCalls = true
		ctx.callSym = syms[0]
		ctx.callNamed = symbolIsNamed(lang, ctx.callSym)
		ctx.typeArgsSym = syms[1]
		ctx.typeArgsNamed = symbolIsNamed(lang, ctx.typeArgsSym)
		ctx.argsSym = syms[2]
		ctx.argsNamed = symbolIsNamed(lang, ctx.argsSym)
		ctx.predefinedTypeSym = syms[3]
		ctx.predefinedTypeNamed = symbolIsNamed(lang, ctx.predefinedTypeSym)
		ctx.binaryExpressionSym = syms[4]
		ctx.greaterThanSym = syms[5]
		ctx.parenthesizedExprSym = syms[6]
		ctx.lessThanSym = syms[7]
		ctx.identifierSym = syms[8]
		ctx.memberExpressionSym = syms[9]
		ctx.sequenceExpressionSym = syms[10]
		ctx.functionFieldID, _ = lang.FieldByName("function")
		ctx.typeArgsFieldID, _ = lang.FieldByName("type_arguments")
		ctx.argumentsFieldID, _ = lang.FieldByName("arguments")
		ctx.typeIdentifierSym, ctx.hasTypeIdentifierSym = lang.SymbolByName("type_identifier")
		if ctx.hasTypeIdentifierSym {
			ctx.typeIdentifierNamed = symbolIsNamed(lang, ctx.typeIdentifierSym)
		}
		if syms, ok := languageSymbols(lang, "instantiation_expression"); ok {
			ctx.instantiationExprSym = syms[0]
			ctx.instantiationExprNamed = symbolIsNamed(lang, ctx.instantiationExprSym)
			ctx.canRewriteInstantiatedCalls = ctx.functionFieldID != 0 && ctx.typeArgsFieldID != 0 && ctx.argumentsFieldID != 0
		}
	}

	if syms, ok := languageSymbols(lang,
		"as_expression",
		"assignment_expression",
		"ternary_expression",
		"union_type",
		"intersection_type",
	); ok {
		ctx.canRewriteAsExpressions = true
		ctx.asExpressionSym = syms[0]
		ctx.asExpressionNamed = symbolIsNamed(lang, ctx.asExpressionSym)
		ctx.assignmentExprSym = syms[1]
		ctx.assignmentExprNamed = symbolIsNamed(lang, ctx.assignmentExprSym)
		ctx.ternaryExprSym = syms[2]
		ctx.ternaryExprNamed = symbolIsNamed(lang, ctx.ternaryExprSym)
		ctx.unionTypeSym = syms[3]
		ctx.unionTypeNamed = symbolIsNamed(lang, ctx.unionTypeSym)
		ctx.intersectionTypeSym = syms[4]
		ctx.intersectionTypeNamed = symbolIsNamed(lang, ctx.intersectionTypeSym)
		ctx.literalTypeSym, ctx.hasLiteralTypeSym = lang.SymbolByName("literal_type")
		if ctx.hasLiteralTypeSym {
			ctx.literalTypeNamed = symbolIsNamed(lang, ctx.literalTypeSym)
		}
		ctx.pipeSym, ctx.hasPipeSym = lang.SymbolByName("|")
		ctx.ampersandSym, ctx.hasAmpersandSym = lang.SymbolByName("&")
		if syms, ok := languageSymbols(lang,
			"object_type",
			"property_signature",
			"type_annotation",
			"object",
			"pair",
			"property_identifier",
			":",
		); ok {
			ctx.objectTypeSym = syms[0]
			ctx.objectTypeNamed = symbolIsNamed(lang, ctx.objectTypeSym)
			ctx.propertySignatureSym = syms[1]
			ctx.propertySignatureNamed = symbolIsNamed(lang, ctx.propertySignatureSym)
			ctx.typeAnnotationSym = syms[2]
			ctx.typeAnnotationNamed = symbolIsNamed(lang, ctx.typeAnnotationSym)
			ctx.objectSym = syms[3]
			ctx.pairSym = syms[4]
			ctx.propertyIdentifierSym = syms[5]
			ctx.propertyIdentifierNamed = symbolIsNamed(lang, ctx.propertyIdentifierSym)
			ctx.colonSym = syms[6]
		}
	}

	if syms, ok := languageSymbols(lang,
		"method_definition",
		"method_signature",
		"accessibility_modifier",
		"property_identifier",
		"public_field_definition",
		"abstract_method_signature",
		"number",
		"string",
		"string_fragment",
	); ok {
		ctx.methodDefinitionSym = syms[0]
		ctx.methodSignatureSym = syms[1]
		ctx.accessibilityModSym = syms[2]
		ctx.accessibilityModNamed = symbolIsNamed(lang, ctx.accessibilityModSym)
		if ctx.propertyIdentifierSym == 0 {
			ctx.propertyIdentifierSym = syms[3]
			ctx.propertyIdentifierNamed = symbolIsNamed(lang, ctx.propertyIdentifierSym)
		}
		ctx.publicFieldDefinitionSym = syms[4]
		ctx.abstractMethodSignatureSym = syms[5]
		ctx.numberSym = syms[6]
		ctx.stringSym = syms[7]
		ctx.stringFragmentSym = syms[8]
		ctx.publicSym, _ = lang.SymbolByName("public")
		ctx.privateSym, _ = lang.SymbolByName("private")
		ctx.protectedSym, _ = lang.SymbolByName("protected")
		ctx.staticSym, _ = lang.SymbolByName("static")
		ctx.readonlySym, _ = lang.SymbolByName("readonly")
		ctx.abstractSym, _ = lang.SymbolByName("abstract")
		ctx.asyncSym, _ = lang.SymbolByName("async")
		ctx.getSym, _ = lang.SymbolByName("get")
		ctx.setSym, _ = lang.SymbolByName("set")
		if ctx.nameFieldID == 0 {
			ctx.nameFieldID, _ = lang.FieldByName("name")
		}
		ctx.parametersFieldID, _ = lang.FieldByName("parameters")
		ctx.returnTypeFieldID, _ = lang.FieldByName("return_type")
		ctx.typeFieldID, _ = lang.FieldByName("type")
		ctx.bodyFieldID, _ = lang.FieldByName("body")
		ctx.valueFieldID, _ = lang.FieldByName("value")
	}

	if enumBodySym, ok := lang.SymbolByName("enum_body"); ok {
		if enumAssignmentSym, ok := lang.SymbolByName("enum_assignment"); ok {
			ctx.canClearEnumBodyFields = true
			ctx.enumBodySym = enumBodySym
			ctx.enumAssignmentSym = enumAssignmentSym
		}
	}
	ctx.importSym, ctx.hasImportSym = lang.SymbolByName("import")
	ctx.typeQuerySym, _ = lang.SymbolByName("type_query")

	if syms, ok := visibleLanguageSymbols(lang, true,
		"type_assertion",
		"arrow_function",
		"type_arguments",
		"type_parameters",
		"type_parameter",
		"type_identifier",
	); ok {
		ctx.canRewriteGenericArrows = true
		ctx.typeAssertionSym = syms[0]
		ctx.arrowFunctionSym = syms[1]
		ctx.typeArgsSym = syms[2]
		ctx.typeParametersSym = syms[3]
		ctx.typeParametersNamed = symbolIsNamed(lang, ctx.typeParametersSym)
		ctx.typeParameterSym = syms[4]
		ctx.typeParameterNamed = symbolIsNamed(lang, ctx.typeParameterSym)
		ctx.typeIdentifierSym = syms[5]
		ctx.typeIdentifierNamed = symbolIsNamed(lang, ctx.typeIdentifierSym)
		ctx.nameFieldID, _ = lang.FieldByName("name")
		ctx.typeParametersFieldID, _ = lang.FieldByName("type_parameters")
	}

	if syms, ok := visibleLanguageSymbols(lang, true,
		"expression_statement",
		"class",
		"class_declaration",
	); ok {
		ctx.canRewriteClassDeclarations = true
		ctx.expressionStatementSym = syms[0]
		ctx.classSym = syms[1]
		ctx.classDeclarationSym = syms[2]
		ctx.classDeclarationNamed = symbolIsNamed(lang, ctx.classDeclarationSym)
		if ctx.nameFieldID == 0 {
			ctx.nameFieldID, _ = lang.FieldByName("name")
		}
	}

	return ctx, ctx.canRewriteGenericCalls || ctx.canRewriteInstantiatedCalls || ctx.canRewriteAsExpressions || ctx.canRewriteGenericArrows || ctx.canRewriteClassDeclarations || ctx.canClearEnumBodyFields
}

type typeScriptMemberTokenKind uint8

const (
	typeScriptMemberTokenIdentifier typeScriptMemberTokenKind = iota
	typeScriptMemberTokenNumber
	typeScriptMemberTokenString
)

type typeScriptMemberToken struct {
	text       string
	startByte  uint32
	endByte    uint32
	startPoint Point
	endPoint   Point
	kind       typeScriptMemberTokenKind
}

func normalizeTypeScriptRecoveredMemberModifiers(node *Node, ctx *typeScriptNormalizationContext) {
	if node == nil || ctx == nil || ctx.lang == nil || ctx.accessibilityModSym == 0 || len(node.children) == 0 {
		return
	}
	switch node.symbol {
	case ctx.methodDefinitionSym, ctx.methodSignatureSym, ctx.abstractMethodSignatureSym, ctx.propertySignatureSym, ctx.publicFieldDefinitionSym:
	default:
		return
	}
	if !typeScriptMemberPrefixStartsWithModifier(ctx.source, node.startByte, node.endByte) {
		return
	}
	tokens, ok := scanTypeScriptMemberPrefixTokens(ctx.source, node.startByte, node.endByte, node.startPoint)
	if !ok || len(tokens) < 2 {
		return
	}
	nameTok := tokens[len(tokens)-1]
	for _, tok := range tokens[:len(tokens)-1] {
		if !typeScriptMemberModifierSymbol(ctx, tok.text).ok {
			return
		}
	}
	suffix := typeScriptMemberSuffixChildren(node, nameTok.endByte)
	if len(suffix) == 0 {
		return
	}

	arena := node.ownerArena
	newChildren := phpAllocChildren(arena, len(tokens)+len(suffix))
	out := 0
	nameIdx := -1
	hasAbstract := false
	for _, tok := range tokens[:len(tokens)-1] {
		mod := typeScriptMemberModifierSymbol(ctx, tok.text)
		if !mod.ok {
			return
		}
		if tok.text == "abstract" {
			hasAbstract = true
		}
		newChildren[out] = buildTypeScriptMemberModifierNode(arena, ctx, tok, mod)
		if newChildren[out] == nil {
			return
		}
		out++
	}
	nameNode := buildTypeScriptMemberNameNode(arena, ctx, nameTok)
	if nameNode == nil {
		return
	}
	nameIdx = out
	newChildren[out] = nameNode
	out++
	copy(newChildren[out:], suffix)

	node.children = newChildren
	if node.ownerArena != nil {
		node.ownerArena.clearFinalChildRefs(node)
	}
	if hasAbstract && node.symbol == ctx.methodSignatureSym && ctx.abstractMethodSignatureSym != 0 {
		node.symbol = ctx.abstractMethodSignatureSym
		node.setNamed(symbolIsNamed(ctx.lang, ctx.abstractMethodSignatureSym))
	}
	typeScriptAssignMemberFields(node, ctx, nameIdx)
	populateParentNode(node, node.children)
}

func typeScriptAccessibilityTokenSymbol(ctx *typeScriptNormalizationContext, text string) Symbol {
	if ctx == nil {
		return 0
	}
	switch text {
	case "public":
		return ctx.publicSym
	case "private":
		return ctx.privateSym
	case "protected":
		return ctx.protectedSym
	default:
		return 0
	}
}

type typeScriptMemberModifier struct {
	sym           Symbol
	accessibility bool
	ok            bool
}

func typeScriptMemberModifierSymbol(ctx *typeScriptNormalizationContext, text string) typeScriptMemberModifier {
	if ctx == nil {
		return typeScriptMemberModifier{}
	}
	if sym := typeScriptAccessibilityTokenSymbol(ctx, text); sym != 0 {
		return typeScriptMemberModifier{sym: sym, accessibility: true, ok: true}
	}
	var sym Symbol
	switch text {
	case "static":
		sym = ctx.staticSym
	case "readonly":
		sym = ctx.readonlySym
	case "abstract":
		sym = ctx.abstractSym
	case "async":
		sym = ctx.asyncSym
	case "get":
		sym = ctx.getSym
	case "set":
		sym = ctx.setSym
	default:
		return typeScriptMemberModifier{}
	}
	if sym == 0 {
		return typeScriptMemberModifier{}
	}
	return typeScriptMemberModifier{sym: sym, ok: true}
}

func typeScriptMemberPrefixStartsWithModifier(source []byte, startByte, endByte uint32) bool {
	if int(startByte) >= len(source) || startByte >= endByte || int(endByte) > len(source) {
		return false
	}
	pos := int(startByte)
	end := int(endByte)
	for pos < end {
		switch source[pos] {
		case ' ', '\t', '\n', '\r':
			pos++
			continue
		}
		break
	}
	if pos >= end || !isTypeScriptIdentifierStartByte(source[pos]) {
		return false
	}
	start := pos
	pos++
	for pos < end {
		ch := source[pos]
		if ch == '_' || ch == '$' || (ch >= 'a' && ch <= 'z') || (ch >= 'A' && ch <= 'Z') || (ch >= '0' && ch <= '9') {
			pos++
			continue
		}
		break
	}
	return typeScriptMemberPrefixBytesAreModifier(source[start:pos])
}

func typeScriptMemberPrefixBytesAreModifier(b []byte) bool {
	switch len(b) {
	case 3:
		return typeScriptBytesEqualString(b, "get") || typeScriptBytesEqualString(b, "set")
	case 5:
		return typeScriptBytesEqualString(b, "async")
	case 6:
		return typeScriptBytesEqualString(b, "public") || typeScriptBytesEqualString(b, "static")
	case 7:
		return typeScriptBytesEqualString(b, "private")
	case 8:
		return typeScriptBytesEqualString(b, "readonly") || typeScriptBytesEqualString(b, "abstract")
	case 9:
		return typeScriptBytesEqualString(b, "protected")
	default:
		return false
	}
}

func typeScriptBytesEqualString(b []byte, s string) bool {
	if len(b) != len(s) {
		return false
	}
	for i := range b {
		if b[i] != s[i] {
			return false
		}
	}
	return true
}

func buildTypeScriptMemberModifierNode(arena *nodeArena, ctx *typeScriptNormalizationContext, tok typeScriptMemberToken, mod typeScriptMemberModifier) *Node {
	if mod.sym == 0 {
		return nil
	}
	leaf := newLeafNodeInArena(arena, mod.sym, symbolIsNamed(ctx.lang, mod.sym), tok.startByte, tok.endByte, tok.startPoint, tok.endPoint)
	if !mod.accessibility {
		return leaf
	}
	children := phpAllocChildren(arena, 1)
	children[0] = leaf
	node := newParentNodeInArena(arena, ctx.accessibilityModSym, ctx.accessibilityModNamed, children, nil, 0)
	populateParentNode(node, node.children)
	return node
}

func buildTypeScriptMemberNameNode(arena *nodeArena, ctx *typeScriptNormalizationContext, tok typeScriptMemberToken) *Node {
	switch tok.kind {
	case typeScriptMemberTokenIdentifier:
		return newLeafNodeInArena(arena, ctx.propertyIdentifierSym, ctx.propertyIdentifierNamed, tok.startByte, tok.endByte, tok.startPoint, tok.endPoint)
	case typeScriptMemberTokenNumber:
		return newLeafNodeInArena(arena, ctx.numberSym, true, tok.startByte, tok.endByte, tok.startPoint, tok.endPoint)
	case typeScriptMemberTokenString:
		return buildTypeScriptStringNameNode(arena, ctx, tok)
	default:
		return nil
	}
}

func buildTypeScriptStringNameNode(arena *nodeArena, ctx *typeScriptNormalizationContext, tok typeScriptMemberToken) *Node {
	if ctx.stringSym == 0 || ctx.stringFragmentSym == 0 || tok.endByte <= tok.startByte+1 {
		return nil
	}
	fragmentStart := tok.startByte + 1
	fragmentEnd := tok.endByte - 1
	fragmentStartPoint := advancePointByBytes(tok.startPoint, ctx.source[tok.startByte:fragmentStart])
	fragmentEndPoint := advancePointByBytes(fragmentStartPoint, ctx.source[fragmentStart:fragmentEnd])
	fragment := newLeafNodeInArena(arena, ctx.stringFragmentSym, symbolIsNamed(ctx.lang, ctx.stringFragmentSym), fragmentStart, fragmentEnd, fragmentStartPoint, fragmentEndPoint)
	children := phpAllocChildren(arena, 1)
	children[0] = fragment
	node := newParentNodeInArena(arena, ctx.stringSym, symbolIsNamed(ctx.lang, ctx.stringSym), children, nil, 0)
	node.startByte = tok.startByte
	node.endByte = tok.endByte
	node.startPoint = tok.startPoint
	node.endPoint = tok.endPoint
	populateParentNode(node, node.children)
	return node
}

func typeScriptMemberSuffixChildren(node *Node, nameEnd uint32) []*Node {
	if node == nil {
		return nil
	}
	out := make([]*Node, 0, len(node.children))
	for _, child := range node.children {
		if child == nil {
			continue
		}
		if child.startByte >= nameEnd {
			out = append(out, child)
		}
	}
	if len(out) == 0 {
		return nil
	}
	if node.ownerArena != nil {
		buf := node.ownerArena.allocNodeSlice(len(out))
		copy(buf, out)
		out = buf
	}
	return out
}

func typeScriptAssignMemberFields(node *Node, ctx *typeScriptNormalizationContext, nameIdx int) {
	if node == nil || ctx == nil || nameIdx < 0 || nameIdx >= len(node.children) {
		return
	}
	if ctx.nameFieldID == 0 {
		node.fieldIDs = nil
		node.fieldSources = nil
		return
	}
	var fieldIDs []FieldID
	if node.ownerArena != nil {
		fieldIDs = node.ownerArena.allocFieldIDSlice(len(node.children))
	} else {
		fieldIDs = make([]FieldID, len(node.children))
	}
	fieldIDs[nameIdx] = ctx.nameFieldID
	for i, child := range node.children {
		if i == nameIdx || child == nil {
			continue
		}
		switch child.symbol {
		case ctx.typeParametersSym:
			fieldIDs[i] = ctx.typeParametersFieldID
		case ctx.argsSym:
			fieldIDs[i] = ctx.parametersFieldID
		case ctx.typeAnnotationSym:
			if node.symbol == ctx.methodDefinitionSym || node.symbol == ctx.methodSignatureSym || node.symbol == ctx.abstractMethodSignatureSym {
				fieldIDs[i] = ctx.returnTypeFieldID
			} else {
				fieldIDs[i] = ctx.typeFieldID
			}
		default:
			switch child.Type(ctx.lang) {
			case "formal_parameters":
				fieldIDs[i] = ctx.parametersFieldID
			case "statement_block":
				fieldIDs[i] = ctx.bodyFieldID
			default:
				if node.symbol == ctx.publicFieldDefinitionSym && child.startByte > node.children[nameIdx].endByte {
					fieldIDs[i] = ctx.valueFieldID
				}
			}
		}
	}
	node.fieldIDs = fieldIDs
	node.fieldSources = defaultFieldSourcesInArena(node.ownerArena, fieldIDs)
}

func scanTypeScriptIdentifierAfter(source []byte, after uint32) (uint32, uint32, bool) {
	pos := int(after)
	for pos < len(source) {
		switch source[pos] {
		case ' ', '\t', '\n', '\r':
			pos++
			continue
		}
		break
	}
	if pos >= len(source) || !isTypeScriptIdentifierStartByte(source[pos]) {
		return 0, 0, false
	}
	start := pos
	pos++
	for pos < len(source) {
		ch := source[pos]
		if ch == '_' || ch == '$' || (ch >= 'a' && ch <= 'z') || (ch >= 'A' && ch <= 'Z') || (ch >= '0' && ch <= '9') {
			pos++
			continue
		}
		break
	}
	return uint32(start), uint32(pos), true
}

func typeScriptNextNonspaceByte(source []byte, after uint32) byte {
	pos := int(after)
	for pos < len(source) {
		switch source[pos] {
		case ' ', '\t', '\n', '\r':
			pos++
			continue
		default:
			return source[pos]
		}
	}
	return 0
}

func scanTypeScriptMemberPrefixTokens(source []byte, startByte, endByte uint32, startPoint Point) ([]typeScriptMemberToken, bool) {
	if int(startByte) >= len(source) || startByte >= endByte || int(endByte) > len(source) {
		return nil, false
	}
	pos := int(startByte)
	end := int(endByte)
	point := startPoint
	var tokens []typeScriptMemberToken
	for pos < end {
		spaceStart := pos
		for pos < end {
			switch source[pos] {
			case ' ', '\t', '\n', '\r':
				pos++
				continue
			}
			break
		}
		if pos > spaceStart {
			point = advancePointByBytes(point, source[spaceStart:pos])
		}
		if pos >= end {
			break
		}
		switch ch := source[pos]; {
		case isTypeScriptIdentifierStartByte(ch):
			tokStart := pos
			tokStartPoint := point
			pos++
			for pos < end {
				next := source[pos]
				if next == '_' || next == '$' || (next >= 'a' && next <= 'z') || (next >= 'A' && next <= 'Z') || (next >= '0' && next <= '9') {
					pos++
					continue
				}
				break
			}
			tokEndPoint := advancePointByBytes(tokStartPoint, source[tokStart:pos])
			tokens = append(tokens, typeScriptMemberToken{
				text:       string(source[tokStart:pos]),
				startByte:  uint32(tokStart),
				endByte:    uint32(pos),
				startPoint: tokStartPoint,
				endPoint:   tokEndPoint,
				kind:       typeScriptMemberTokenIdentifier,
			})
			point = tokEndPoint
		case ch >= '0' && ch <= '9':
			tokStart := pos
			tokStartPoint := point
			pos++
			for pos < end && source[pos] >= '0' && source[pos] <= '9' {
				pos++
			}
			tokEndPoint := advancePointByBytes(tokStartPoint, source[tokStart:pos])
			tokens = append(tokens, typeScriptMemberToken{
				text:       string(source[tokStart:pos]),
				startByte:  uint32(tokStart),
				endByte:    uint32(pos),
				startPoint: tokStartPoint,
				endPoint:   tokEndPoint,
				kind:       typeScriptMemberTokenNumber,
			})
			point = tokEndPoint
		case ch == '\'' || ch == '"':
			quote := ch
			tokStart := pos
			tokStartPoint := point
			pos++
			for pos < end {
				if source[pos] == '\\' {
					pos += 2
					continue
				}
				if source[pos] == quote {
					pos++
					break
				}
				pos++
			}
			if pos > end || source[pos-1] != quote {
				return nil, false
			}
			tokEndPoint := advancePointByBytes(tokStartPoint, source[tokStart:pos])
			tokens = append(tokens, typeScriptMemberToken{
				text:       string(source[tokStart:pos]),
				startByte:  uint32(tokStart),
				endByte:    uint32(pos),
				startPoint: tokStartPoint,
				endPoint:   tokEndPoint,
				kind:       typeScriptMemberTokenString,
			})
			point = tokEndPoint
		default:
			return tokens, len(tokens) > 1
		}
		if pos >= end {
			break
		}
		switch source[pos] {
		case ' ', '\t', '\n', '\r':
			continue
		case '(', ':', '=', ';', '{', '}', '<', '?', '!':
			return tokens, len(tokens) > 1
		default:
			return nil, false
		}
	}
	return tokens, len(tokens) > 1
}

func rewriteTypeScriptGenericArrowTypeAssertion(node *Node, ctx *typeScriptNormalizationContext) *Node {
	if node == nil || ctx == nil || ctx.lang == nil || node.symbol != ctx.typeAssertionSym || len(node.children) < 2 {
		return nil
	}
	typeArgs := node.children[0]
	arrow := node.children[len(node.children)-1]
	if typeArgs == nil || arrow == nil || typeArgs.symbol != ctx.typeArgsSym || arrow.symbol != ctx.arrowFunctionSym {
		return nil
	}

	typeParams := convertTypeScriptTypeArgumentsToParameters(typeArgs, ctx)
	if typeParams == nil {
		return nil
	}

	arena := node.ownerArena
	children := phpAllocChildren(arena, len(arrow.children)+1)
	children[0] = typeParams
	copy(children[1:], arrow.children)

	var fieldIDs []FieldID
	if ctx.typeParametersFieldID != 0 || len(arrow.fieldIDs) > 0 {
		if arena != nil {
			fieldIDs = arena.allocFieldIDSlice(len(children))
		} else {
			fieldIDs = make([]FieldID, len(children))
		}
		fieldIDs[0] = ctx.typeParametersFieldID
		copy(fieldIDs[1:], arrow.fieldIDs)
	}

	rewritten := cloneNodeInArena(arena, arrow)
	rewritten.children = children
	rewritten.fieldIDs = fieldIDs
	rewritten.fieldSources = defaultFieldSourcesInArena(arena, fieldIDs)
	populateParentNode(rewritten, rewritten.children)
	return rewritten
}

func convertTypeScriptTypeArgumentsToParameters(typeArgs *Node, ctx *typeScriptNormalizationContext) *Node {
	if typeArgs == nil || ctx == nil || ctx.lang == nil || typeArgs.symbol != ctx.typeArgsSym || len(typeArgs.children) == 0 {
		return nil
	}
	arena := typeArgs.ownerArena
	children := phpAllocChildren(arena, len(typeArgs.children))
	convertedNamed := 0
	for i, child := range typeArgs.children {
		if child == nil || !child.isNamed() {
			children[i] = child
			continue
		}
		if child.symbol != ctx.typeIdentifierSym {
			return nil
		}
		paramChildren := phpAllocChildren(arena, 1)
		paramChildren[0] = child
		var fieldIDs []FieldID
		if ctx.nameFieldID != 0 {
			if arena != nil {
				fieldIDs = arena.allocFieldIDSlice(1)
			} else {
				fieldIDs = make([]FieldID, 1)
			}
			fieldIDs[0] = ctx.nameFieldID
		}
		param := newParentNodeInArena(arena, ctx.typeParameterSym, ctx.typeParameterNamed, paramChildren, fieldIDs, child.productionID)
		param.fieldSources = defaultFieldSourcesInArena(arena, fieldIDs)
		children[i] = param
		convertedNamed++
	}
	if convertedNamed == 0 {
		return nil
	}
	return newParentNodeInArena(arena, ctx.typeParametersSym, ctx.typeParametersNamed, children, nil, typeArgs.productionID)
}

func rewriteTypeScriptClassExpressionStatement(node *Node, ctx *typeScriptNormalizationContext) *Node {
	if node == nil || ctx == nil || ctx.lang == nil || node.symbol != ctx.expressionStatementSym {
		return nil
	}
	var classNode *Node
	for _, child := range node.children {
		if child == nil || child.isExtra() {
			continue
		}
		if child.symbol == ctx.classSym {
			if classNode != nil {
				return nil
			}
			classNode = child
			continue
		}
		if child.isNamed() {
			return nil
		}
	}
	if classNode == nil || !typeScriptClassExpressionHasName(classNode, ctx) {
		return nil
	}
	children := cloneNodeSliceInArena(classNode.ownerArena, classNode.children)
	fieldIDs := cloneFieldIDSliceInArena(classNode.ownerArena, classNode.fieldIDs)
	decl := newParentNodeInArena(classNode.ownerArena, ctx.classDeclarationSym, ctx.classDeclarationNamed, children, fieldIDs, classNode.productionID)
	decl.fieldSources = cloneFieldSourceSliceInArena(classNode.ownerArena, classNode.fieldSources)
	if len(decl.fieldSources) == 0 {
		decl.fieldSources = defaultFieldSourcesInArena(classNode.ownerArena, fieldIDs)
	}
	return decl
}

func typeScriptClassExpressionHasName(node *Node, ctx *typeScriptNormalizationContext) bool {
	if node == nil || ctx == nil || ctx.lang == nil {
		return false
	}
	for i, child := range node.children {
		if child == nil {
			continue
		}
		if ctx.nameFieldID != 0 && i < len(node.fieldIDs) && node.fieldIDs[i] == ctx.nameFieldID {
			return child.symbol == ctx.typeIdentifierSym && child.endByte > child.startByte
		}
		if child.symbol == ctx.typeIdentifierSym && child.endByte > child.startByte {
			return true
		}
	}
	return false
}

func cloneFieldSourceSliceInArena(arena *nodeArena, fieldSources []uint8) []uint8 {
	if len(fieldSources) == 0 {
		return nil
	}
	if arena != nil {
		out := arena.allocFieldSourceSlice(len(fieldSources))
		copy(out, fieldSources)
		return out
	}
	out := make([]uint8, len(fieldSources))
	copy(out, fieldSources)
	return out
}

func rewriteTypeScriptPredefinedGenericCall(node *Node, ctx *typeScriptNormalizationContext) *Node {
	if node == nil || ctx == nil || ctx.lang == nil || node.symbol != ctx.binaryExpressionSym || len(node.children) != 3 {
		return nil
	}
	typeExpr, gt, paren, ok := splitTypeScriptGenericCallClose(node, ctx)
	if !ok || typeExpr == nil || gt == nil || paren == nil || len(paren.children) != 3 {
		return nil
	}
	callee, lt, typeArgExpr, ok := splitTypeScriptGenericCallOpen(typeExpr, ctx)
	if !ok || callee == nil || lt == nil || typeArgExpr == nil {
		return nil
	}
	switch callee.Type(ctx.lang) {
	case "identifier", "member_expression":
	default:
		return nil
	}
	typeArg := normalizeTypeScriptGenericCallTypeArgument(typeArgExpr, ctx)
	if typeArg == nil {
		return nil
	}
	arena := node.ownerArena
	if typeArg.ownerArena != arena {
		typeArg = cloneNodeInArena(arena, typeArg)
	}
	typeArgs := newParentNodeInArena(arena, ctx.typeArgsSym, ctx.typeArgsNamed, []*Node{lt, typeArg, gt}, nil, 0)
	argsChildren := typeScriptGenericCallArgumentChildren(paren, ctx.sequenceExpressionSym)
	if arena != nil && len(argsChildren) > 0 {
		buf := arena.allocNodeSlice(len(argsChildren))
		copy(buf, argsChildren)
		argsChildren = buf
	}
	args := newParentNodeInArena(arena, ctx.argsSym, ctx.argsNamed, argsChildren, nil, paren.productionID)

	callChildren := phpAllocChildren(arena, 3)
	callChildren[0] = callee
	callChildren[1] = typeArgs
	callChildren[2] = args
	var fieldIDs []FieldID
	if ctx.functionFieldID != 0 || ctx.typeArgsFieldID != 0 || ctx.argumentsFieldID != 0 {
		if arena != nil {
			fieldIDs = arena.allocFieldIDSlice(3)
		} else {
			fieldIDs = make([]FieldID, 3)
		}
		fieldIDs[0] = ctx.functionFieldID
		fieldIDs[1] = ctx.typeArgsFieldID
		fieldIDs[2] = ctx.argumentsFieldID
	}
	call := newParentNodeInArena(arena, ctx.callSym, ctx.callNamed, callChildren, fieldIDs, node.productionID)
	call.fieldSources = defaultFieldSourcesInArena(arena, fieldIDs)
	return call
}

func splitTypeScriptGenericCallClose(node *Node, ctx *typeScriptNormalizationContext) (*Node, *Node, *Node, bool) {
	if node == nil || ctx == nil || node.symbol != ctx.binaryExpressionSym || len(node.children) != 3 {
		return nil, nil, nil, false
	}
	left := node.children[0]
	op := node.children[1]
	right := node.children[2]
	if left == nil || op == nil || right == nil {
		return nil, nil, nil, false
	}
	if op.symbol == ctx.greaterThanSym && right.symbol == ctx.parenthesizedExprSym {
		return left, op, right, true
	}
	if !typeScriptBinaryOperatorIsTypeUnionOrIntersection(op, ctx) || right.symbol != ctx.binaryExpressionSym {
		return nil, nil, nil, false
	}
	strippedRight, gt, paren, ok := splitTypeScriptGenericCallClose(right, ctx)
	if !ok || strippedRight == nil {
		return nil, nil, nil, false
	}
	rewritten := cloneNodeInArena(node.ownerArena, node)
	children := cloneNodeSliceInArena(node.ownerArena, node.children)
	children[2] = strippedRight
	rewritten.children = children
	populateParentNode(rewritten, rewritten.children)
	return rewritten, gt, paren, true
}

func splitTypeScriptGenericCallOpen(node *Node, ctx *typeScriptNormalizationContext) (*Node, *Node, *Node, bool) {
	if node == nil || ctx == nil || node.symbol != ctx.binaryExpressionSym || len(node.children) != 3 {
		return nil, nil, nil, false
	}
	left := node.children[0]
	op := node.children[1]
	right := node.children[2]
	if left == nil || op == nil || right == nil {
		return nil, nil, nil, false
	}
	if op.symbol == ctx.lessThanSym {
		return left, op, right, true
	}
	if !typeScriptBinaryOperatorIsTypeUnionOrIntersection(op, ctx) || left.symbol != ctx.binaryExpressionSym {
		return nil, nil, nil, false
	}
	callee, lt, strippedLeft, ok := splitTypeScriptGenericCallOpen(left, ctx)
	if !ok || strippedLeft == nil {
		return nil, nil, nil, false
	}
	rewritten := cloneNodeInArena(node.ownerArena, node)
	children := cloneNodeSliceInArena(node.ownerArena, node.children)
	children[0] = strippedLeft
	rewritten.children = children
	populateParentNode(rewritten, rewritten.children)
	return callee, lt, rewritten, true
}

func typeScriptBinaryExpressionHasGenericCallClose(node *Node, ctx *typeScriptNormalizationContext) bool {
	typeExpr, ok := typeScriptGenericCallCloseTypeExpressionNoAlloc(node, ctx)
	if !ok || typeExpr == nil {
		return false
	}
	return typeScriptGenericCallOpenNoAlloc(typeExpr, ctx)
}

func typeScriptGenericCallCloseTypeExpressionNoAlloc(node *Node, ctx *typeScriptNormalizationContext) (*Node, bool) {
	root := node
	for {
		left, op, right, ok := typeScriptBinaryParts(node, ctx)
		if !ok {
			return nil, false
		}
		if op.symbol == ctx.greaterThanSym && right.symbol == ctx.parenthesizedExprSym {
			if node == root {
				return left, true
			}
			return root, true
		}
		if !typeScriptBinaryOperatorIsTypeUnionOrIntersection(op, ctx) || right.symbol != ctx.binaryExpressionSym {
			return nil, false
		}
		node = right
	}
}

func typeScriptGenericCallOpenNoAlloc(node *Node, ctx *typeScriptNormalizationContext) bool {
	for {
		left, op, _, ok := typeScriptBinaryParts(node, ctx)
		if !ok {
			return false
		}
		if op.symbol == ctx.lessThanSym {
			return true
		}
		if !typeScriptBinaryOperatorIsTypeUnionOrIntersection(op, ctx) || left.symbol != ctx.binaryExpressionSym {
			return false
		}
		node = left
	}
}

func typeScriptBinaryParts(node *Node, ctx *typeScriptNormalizationContext) (*Node, *Node, *Node, bool) {
	if node == nil || ctx == nil || node.symbol != ctx.binaryExpressionSym || len(node.children) != 3 {
		return nil, nil, nil, false
	}
	left := node.children[0]
	op := node.children[1]
	right := node.children[2]
	if left == nil || op == nil || right == nil {
		return nil, nil, nil, false
	}
	return left, op, right, true
}

func typeScriptBinaryOperatorIsTypeUnionOrIntersection(op *Node, ctx *typeScriptNormalizationContext) bool {
	if op == nil || ctx == nil {
		return false
	}
	return (ctx.hasPipeSym && op.symbol == ctx.pipeSym) ||
		(ctx.hasAmpersandSym && op.symbol == ctx.ampersandSym)
}

func rewriteTypeScriptInstantiatedCall(node *Node, ctx *typeScriptNormalizationContext) *Node {
	if node == nil || ctx == nil || ctx.lang == nil || node.symbol != ctx.callSym || len(node.children) != 2 {
		return nil
	}
	function := node.children[0]
	arguments := node.children[1]
	if function == nil || arguments == nil || function.symbol != ctx.instantiationExprSym || arguments.symbol != ctx.argsSym || len(function.children) != 2 {
		return nil
	}
	callee := function.children[0]
	typeArgs := function.children[1]
	if callee == nil || typeArgs == nil || typeArgs.symbol != ctx.typeArgsSym {
		return nil
	}
	children := phpAllocChildren(node.ownerArena, 3)
	children[0] = callee
	children[1] = typeArgs
	children[2] = arguments
	var fieldIDs []FieldID
	if ctx.functionFieldID != 0 || ctx.typeArgsFieldID != 0 || ctx.argumentsFieldID != 0 {
		if node.ownerArena != nil {
			fieldIDs = node.ownerArena.allocFieldIDSlice(3)
		} else {
			fieldIDs = make([]FieldID, 3)
		}
		fieldIDs[0] = ctx.functionFieldID
		fieldIDs[1] = ctx.typeArgsFieldID
		fieldIDs[2] = ctx.argumentsFieldID
	}
	call := newParentNodeInArena(node.ownerArena, ctx.callSym, ctx.callNamed, children, fieldIDs, node.productionID)
	call.fieldSources = defaultFieldSourcesInArena(node.ownerArena, fieldIDs)
	return call
}

func rewriteTypeScriptAsExpressionCompatibility(node *Node, ctx *typeScriptNormalizationContext) *Node {
	if node == nil || ctx == nil || ctx.lang == nil {
		return nil
	}
	if rewritten := rewriteTypeScriptAsAssignmentOrTernary(node, ctx); rewritten != nil {
		return rewritten
	}
	return rewriteTypeScriptAsTypeChain(node, ctx)
}

func rewriteTypeScriptAsAssignmentOrTernary(node *Node, ctx *typeScriptNormalizationContext) *Node {
	if node == nil || ctx == nil || ctx.lang == nil || node.symbol != ctx.asExpressionSym || len(node.children) < 2 {
		return nil
	}
	valueIdx, typeIdx := 0, len(node.children)-1
	value := node.children[valueIdx]
	if value == nil {
		return nil
	}

	switch value.symbol {
	case ctx.assignmentExprSym:
		if len(value.children) < 2 {
			return nil
		}
		rightIdx := len(value.children) - 1
		rewrittenAs := cloneNodeInArena(node.ownerArena, node)
		asChildren := cloneNodeSliceInArena(node.ownerArena, node.children)
		asChildren[valueIdx] = value.children[rightIdx]
		rewrittenAs.children = asChildren
		populateParentNode(rewrittenAs, rewrittenAs.children)

		rewrittenAssign := cloneNodeInArena(node.ownerArena, value)
		assignChildren := cloneNodeSliceInArena(node.ownerArena, value.children)
		assignChildren[rightIdx] = rewrittenAs
		rewrittenAssign.children = assignChildren
		populateParentNode(rewrittenAssign, rewrittenAssign.children)
		return rewrittenAssign
	case ctx.ternaryExprSym:
		if len(value.children) < 3 {
			return nil
		}
		falseIdx := len(value.children) - 1
		rewrittenAs := cloneNodeInArena(node.ownerArena, node)
		asChildren := cloneNodeSliceInArena(node.ownerArena, node.children)
		asChildren[valueIdx] = value.children[falseIdx]
		rewrittenAs.children = asChildren
		populateParentNode(rewrittenAs, rewrittenAs.children)

		rewrittenTernary := cloneNodeInArena(node.ownerArena, value)
		ternaryChildren := cloneNodeSliceInArena(node.ownerArena, value.children)
		ternaryChildren[falseIdx] = rewrittenAs
		rewrittenTernary.children = ternaryChildren
		populateParentNode(rewrittenTernary, rewrittenTernary.children)
		return rewrittenTernary
	default:
		_ = typeIdx
		return nil
	}
}

func rewriteTypeScriptAsTypeChain(node *Node, ctx *typeScriptNormalizationContext) *Node {
	if node == nil || ctx == nil || ctx.lang == nil || node.symbol != ctx.binaryExpressionSym || len(node.children) != 3 {
		return nil
	}
	baseAs, rewrittenType, ok := collapseTypeScriptAsTypeChain(node, ctx)
	if !ok || baseAs == nil || rewrittenType == nil || len(baseAs.children) < 2 {
		return nil
	}
	rewrittenAs := cloneNodeInArena(node.ownerArena, baseAs)
	asChildren := cloneNodeSliceInArena(node.ownerArena, baseAs.children)
	asChildren[len(asChildren)-1] = rewrittenType
	rewrittenAs.children = asChildren
	populateParentNode(rewrittenAs, rewrittenAs.children)
	return rewrittenAs
}

func collapseTypeScriptAsTypeChain(node *Node, ctx *typeScriptNormalizationContext) (*Node, *Node, bool) {
	if node == nil || ctx == nil || ctx.lang == nil || node.symbol != ctx.binaryExpressionSym || len(node.children) != 3 {
		return nil, nil, false
	}
	left := node.children[0]
	op := node.children[1]
	right := node.children[2]
	if left == nil || op == nil || right == nil {
		return nil, nil, false
	}
	var typeSym Symbol
	var typeNamed bool
	switch op.Type(ctx.lang) {
	case "|":
		typeSym = ctx.unionTypeSym
		typeNamed = ctx.unionTypeNamed
	case "&":
		typeSym = ctx.intersectionTypeSym
		typeNamed = ctx.intersectionTypeNamed
	default:
		return nil, nil, false
	}

	rightType := normalizeTypeScriptTypeExpression(right, ctx)
	if rightType == nil {
		return nil, nil, false
	}

	if left.symbol == ctx.asExpressionSym && len(left.children) >= 2 {
		leftType := normalizeTypeScriptTypeExpression(left.children[len(left.children)-1], ctx)
		if leftType == nil {
			return nil, nil, false
		}
		children := cloneNodeSliceInArena(node.ownerArena, []*Node{leftType, op, rightType})
		return left, newParentNodeInArena(node.ownerArena, typeSym, typeNamed, children, nil, node.productionID), true
	}

	leftAs, leftType, ok := collapseTypeScriptAsTypeChain(left, ctx)
	if !ok || leftAs == nil || leftType == nil {
		return nil, nil, false
	}
	children := cloneNodeSliceInArena(node.ownerArena, []*Node{leftType, op, rightType})
	return leftAs, newParentNodeInArena(node.ownerArena, typeSym, typeNamed, children, nil, node.productionID), true
}

func normalizeTypeScriptTypeExpression(node *Node, ctx *typeScriptNormalizationContext) *Node {
	if node == nil || ctx == nil || ctx.lang == nil {
		return nil
	}
	switch node.Type(ctx.lang) {
	case "type_identifier", "predefined_type", "union_type", "intersection_type", "object_type", "literal_type", "generic_type", "lookup_type", "template_literal_type", "conditional_type", "tuple_type", "array_type", "function_type", "constructor_type", "readonly_type", "type_query", "infer_type", "index_type_query", "nested_type_identifier":
		return node
	case "identifier":
		if ctx.hasTypeIdentifierSym {
			return newLeafNodeInArena(node.ownerArena, ctx.typeIdentifierSym, ctx.typeIdentifierNamed, node.startByte, node.endByte, node.startPoint, node.endPoint)
		}
		return node
	case "binary_expression":
		if len(node.children) != 3 || node.children[1] == nil {
			return nil
		}
		var typeSym Symbol
		var typeNamed bool
		switch node.children[1].Type(ctx.lang) {
		case "|":
			typeSym = ctx.unionTypeSym
			typeNamed = ctx.unionTypeNamed
		case "&":
			typeSym = ctx.intersectionTypeSym
			typeNamed = ctx.intersectionTypeNamed
		default:
			return nil
		}
		leftType := normalizeTypeScriptTypeExpression(node.children[0], ctx)
		rightType := normalizeTypeScriptTypeExpression(node.children[2], ctx)
		if leftType == nil || rightType == nil {
			return nil
		}
		children := cloneNodeSliceInArena(node.ownerArena, []*Node{leftType, node.children[1], rightType})
		return newParentNodeInArena(node.ownerArena, typeSym, typeNamed, children, nil, node.productionID)
	case "object":
		return rewriteTypeScriptObjectExpressionAsType(node, ctx)
	default:
		return nil
	}
}

func rewriteTypeScriptObjectExpressionAsType(node *Node, ctx *typeScriptNormalizationContext) *Node {
	if node == nil || ctx == nil || ctx.lang == nil || node.Type(ctx.lang) != "object" {
		return nil
	}
	children := cloneNodeSliceInArena(node.ownerArena, node.children)
	changed := false
	for i, child := range children {
		if child == nil || child.Type(ctx.lang) != "pair" {
			continue
		}
		propSig := rewriteTypeScriptObjectPairAsPropertySignature(child, ctx)
		if propSig == nil {
			return nil
		}
		children[i] = propSig
		changed = true
	}
	if !changed && len(children) != 2 {
		return nil
	}
	return newParentNodeInArena(node.ownerArena, ctx.objectTypeSym, ctx.objectTypeNamed, children, nil, node.productionID)
}

func rewriteTypeScriptObjectPairAsPropertySignature(node *Node, ctx *typeScriptNormalizationContext) *Node {
	if node == nil || ctx == nil || ctx.lang == nil || node.Type(ctx.lang) != "pair" || len(node.children) < 3 {
		return nil
	}
	key := node.children[0]
	colon := node.children[1]
	value := node.children[len(node.children)-1]
	if key == nil || colon == nil || value == nil || key.Type(ctx.lang) != "property_identifier" || colon.Type(ctx.lang) != ":" {
		return nil
	}
	valueType := normalizeTypeScriptTypeExpression(value, ctx)
	if valueType == nil {
		return nil
	}
	typeAnnChildren := cloneNodeSliceInArena(node.ownerArena, []*Node{colon, valueType})
	typeAnnotation := newParentNodeInArena(node.ownerArena, ctx.typeAnnotationSym, ctx.typeAnnotationNamed, typeAnnChildren, nil, 0)
	propChildren := cloneNodeSliceInArena(node.ownerArena, []*Node{key, typeAnnotation})
	return newParentNodeInArena(node.ownerArena, ctx.propertySignatureSym, ctx.propertySignatureNamed, propChildren, nil, node.productionID)
}

func typeScriptGenericCallArgumentChildren(paren *Node, sequenceExpressionSym Symbol) []*Node {
	if paren == nil {
		return nil
	}
	if len(paren.children) != 3 || paren.children[1] == nil || paren.children[1].symbol != sequenceExpressionSym {
		return append([]*Node(nil), paren.children...)
	}
	seq := paren.children[1]
	out := make([]*Node, 0, len(seq.children)+2)
	out = append(out, paren.children[0])
	out = append(out, seq.children...)
	out = append(out, paren.children[2])
	return out
}

func normalizeTypeScriptGenericCallTypeArgument(node *Node, ctx *typeScriptNormalizationContext) *Node {
	if node == nil || ctx == nil || ctx.lang == nil {
		return nil
	}
	switch node.Type(ctx.lang) {
	case "predefined_type", "type_identifier", "union_type", "intersection_type", "object_type", "generic_type", "lookup_type", "template_literal_type", "conditional_type", "tuple_type", "array_type", "function_type", "constructor_type", "readonly_type", "type_query", "infer_type", "index_type_query", "nested_type_identifier":
		return node
	case "literal_type":
		if ctx.hasLiteralTypeSym {
			return node
		}
	case "identifier":
		if typeKeywordSym, ok := typeScriptPredefinedTypeSymbol(ctx.lang, node.Text(ctx.source)); ok {
			typeKeywordNamed := symbolIsNamed(ctx.lang, typeKeywordSym)
			typeLeaf := newLeafNodeInArena(node.ownerArena, typeKeywordSym, typeKeywordNamed, node.startByte, node.endByte, node.startPoint, node.endPoint)
			return newParentNodeInArena(node.ownerArena, ctx.predefinedTypeSym, ctx.predefinedTypeNamed, []*Node{typeLeaf}, nil, 0)
		}
		if ctx.hasTypeIdentifierSym {
			typeIdentifierNamed := symbolIsNamed(ctx.lang, ctx.typeIdentifierSym)
			return newLeafNodeInArena(node.ownerArena, ctx.typeIdentifierSym, typeIdentifierNamed, node.startByte, node.endByte, node.startPoint, node.endPoint)
		}
	case "null", "true", "false", "undefined", "number", "string":
		return wrapTypeScriptGenericCallLiteralType(node, ctx)
	case "binary_expression":
		if len(node.children) != 3 || node.children[1] == nil {
			return nil
		}
		var typeSym Symbol
		var typeNamed bool
		switch node.children[1].symbol {
		case ctx.pipeSym:
			if !ctx.hasPipeSym {
				return nil
			}
			typeSym = ctx.unionTypeSym
			typeNamed = ctx.unionTypeNamed
		case ctx.ampersandSym:
			if !ctx.hasAmpersandSym {
				return nil
			}
			typeSym = ctx.intersectionTypeSym
			typeNamed = ctx.intersectionTypeNamed
		default:
			return nil
		}
		leftType := normalizeTypeScriptGenericCallTypeArgument(node.children[0], ctx)
		rightType := normalizeTypeScriptGenericCallTypeArgument(node.children[2], ctx)
		if leftType == nil || rightType == nil {
			return nil
		}
		children := cloneNodeSliceInArena(node.ownerArena, []*Node{leftType, node.children[1], rightType})
		return newParentNodeInArena(node.ownerArena, typeSym, typeNamed, children, nil, node.productionID)
	case "object":
		return rewriteTypeScriptObjectExpressionAsType(node, ctx)
	}
	return nil
}

func wrapTypeScriptGenericCallLiteralType(node *Node, ctx *typeScriptNormalizationContext) *Node {
	if node == nil || ctx == nil || !ctx.hasLiteralTypeSym {
		return nil
	}
	children := cloneNodeSliceInArena(node.ownerArena, []*Node{node})
	return newParentNodeInArena(node.ownerArena, ctx.literalTypeSym, ctx.literalTypeNamed, children, nil, node.productionID)
}

func typeScriptPredefinedTypeSymbol(lang *Language, text string) (Symbol, bool) {
	if lang == nil {
		return 0, false
	}
	switch text {
	case "any", "bigint", "boolean", "never", "number", "object", "string", "symbol", "undefined", "unknown", "void":
		return lang.SymbolByName(text)
	default:
		return 0, false
	}
}
