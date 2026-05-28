package gotreesitter

import "bytes"

func normalizeJavaScriptCompatibility(root *Node, source []byte, lang *Language) {
	normalizeJavaScriptProgramStart(root, lang)
	// Fused walk handles empty_statement, statement keywords (if/while),
	// optional-chain leaves, call_expression precedence, and builds unary/
	// binary candidate indexes — six separate full-tree walks collapsed into
	// one walk + indexed rewrites.
	normalizeJavaScriptTypeScriptStatementKeywordsAndPrecedence(root, source, lang)
	normalizeJavaScriptTrailingContinueComments(root, source, lang)
	normalizeJavaScriptTopLevelExpressionStatementBounds(root, lang)
	normalizeJavaScriptTopLevelDeclarationBounds(root, lang)
	normalizeJavaScriptTopLevelObjectLiterals(root, lang)
	normalizeJavaScriptProgramEnd(root, source, lang)
}

func normalizeTypeScriptTreeCompatibility(root *Node, source []byte, lang *Language) {
	normalizeTypeScriptTreeCompatibilityWithParser(root, source, nil, lang)
}

func normalizeTypeScriptTreeCompatibilityWithParser(root *Node, source []byte, parser *Parser, lang *Language) {
	recordPasses := parser != nil && parser.currentMaterializationTiming() != nil
	if !recordPasses {
		// OptionalChainLeaves was fused into the statement-keyword walk; one less
		// tree traversal per file containing "?." (most modern JS/TS).
		normalizeJavaScriptTypeScriptStatementKeywordsAndPrecedence(root, source, lang)
		normalizeTypeScriptRecoveredNamespaceRoot(root, source, lang)
		normalizeJavaScriptTopLevelDeclarationBounds(root, lang)
		normalizeTypeScriptCompatibility(root, source, lang)
		normalizeJavaScriptTopLevelExpressionStatementBounds(root, lang)
		return
	}
	run := func(name string, fn func() normalizationPassCounters) {
		parser.runNamedNormalizationPass(name, func() bool { return true }, fn)
	}
	runVoid := func(name string, fn func()) {
		run(name, func() normalizationPassCounters {
			fn()
			return normalizationPassCounters{}
		})
	}
	recordTypeScriptCompatSourceFlagMetrics(parser, typeScriptCompatSourceFlagsFor(source))

	recordTypeScriptCompatCandidateMetrics(parser, root, lang)
	var syntaxStats javaScriptTypeScriptSyntaxNormalizationStats
	var haveSyntaxStats bool
	run("ts_statement_keyword_leaves", func() normalizationPassCounters {
		syntaxStats = normalizeJavaScriptTypeScriptStatementKeywordsAndPrecedenceWithDetailedStats(root, source, lang)
		haveSyntaxStats = true
		parser.recordNormalizationMetric("ts_syntax_precedence_index", 1, syntaxStats.indexBuilds, syntaxStats.indexNodesVisited, 0)
		parser.recordNormalizationMetric("ts_empty_statement_candidates", 1, 1, syntaxStats.emptyStatement.nodesVisited, syntaxStats.emptyStatement.nodesRewritten)
		parser.recordNormalizationMetric("ts_existential_type_candidates", 1, 1, syntaxStats.existentialType.nodesVisited, syntaxStats.existentialType.nodesRewritten)
		parser.recordNormalizationMetric("ts_statement_keyword_candidates", 1, 1, syntaxStats.statementKeyword.nodesVisited, syntaxStats.statementKeyword.nodesRewritten)
		parser.recordNormalizationMetric("ts_call_precedence_candidates", 1, 1, syntaxStats.call.nodesVisited, syntaxStats.call.nodesRewritten)
		parser.recordNormalizationMetric("ts_unary_precedence_candidates", 1, 1, syntaxStats.unary.nodesVisited, syntaxStats.unary.nodesRewritten)
		parser.recordNormalizationMetric("ts_binary_precedence_candidates", 1, 1, syntaxStats.binary.nodesVisited, syntaxStats.binary.nodesRewritten)
		return syntaxStats.statementKeyword
	})
	run("ts_empty_statement", func() normalizationPassCounters {
		if haveSyntaxStats {
			return syntaxStats.emptyStatement
		}
		return normalizeCollapsedNamedLeafChildrenBySourceWithStats(root, source, lang, "empty_statement", ";")
	})
	run("ts_call_precedence", func() normalizationPassCounters {
		if haveSyntaxStats {
			return syntaxStats.call
		}
		stats := normalizeJavaScriptTypeScriptCallPrecedenceWithDetailedStats(root, lang)
		parser.recordNormalizationMetric("ts_call_precedence_index", 1, stats.indexBuilds, stats.indexNodesVisited, 0)
		parser.recordNormalizationMetric("ts_call_precedence_candidates", 1, 1, stats.candidateCalls, stats.nodesRewritten)
		return stats.normalizationPassCounters
	})
	run("ts_unary_precedence", func() normalizationPassCounters {
		if haveSyntaxStats {
			return syntaxStats.unary
		}
		return normalizeJavaScriptTypeScriptUnaryPrecedenceWithStats(root, lang)
	})
	run("ts_binary_precedence", func() normalizationPassCounters {
		if haveSyntaxStats {
			return syntaxStats.binary
		}
		return normalizeJavaScriptTypeScriptBinaryPrecedenceWithStats(root, lang)
	})
	runVoid("ts_recovered_namespace_root", func() {
		normalizeTypeScriptRecoveredNamespaceRoot(root, source, lang)
	})
	runVoid("ts_top_level_declaration_bounds", func() {
		normalizeJavaScriptTopLevelDeclarationBounds(root, lang)
	})
	run("ts_type_compatibility", func() normalizationPassCounters {
		stats := normalizeTypeScriptCompatibilityWithStats(root, source, lang)
		parser.recordNormalizationMetric("ts_type_identifier_alias_candidates", 1, 1, stats.identifierAliases.nodesVisited, stats.identifierAliases.nodesRewritten)
		parser.recordNormalizationMetric("ts_type_import_keyword_candidates", 1, 1, stats.importKeywords.nodesVisited, stats.importKeywords.nodesRewritten)
		parser.recordNormalizationMetric("ts_type_member_modifier_candidates", 1, 1, stats.memberModifiers.nodesVisited, stats.memberModifiers.nodesRewritten)
		parser.recordNormalizationMetric("ts_type_enum_body_candidates", 1, 1, stats.enumBodies.nodesVisited, stats.enumBodies.nodesRewritten)
		parser.recordNormalizationMetric("ts_type_binary_child_candidates", 1, 1, stats.binaryChildren.nodesVisited, stats.binaryChildren.nodesRewritten)
		parser.recordNormalizationMetric("ts_type_binary_generic_candidates", 1, 1, stats.binaryGenericChildren.nodesVisited, stats.binaryGenericChildren.nodesRewritten)
		parser.recordNormalizationMetric("ts_type_binary_as_type_candidates", 1, 1, stats.binaryAsTypeChildren.nodesVisited, stats.binaryAsTypeChildren.nodesRewritten)
		parser.recordNormalizationMetric("ts_type_binary_fast_skip", 1, 1, stats.binaryFastSkipped.nodesVisited, 0)
		parser.recordNormalizationMetric("ts_type_call_child_candidates", 1, 1, stats.callChildren.nodesVisited, stats.callChildren.nodesRewritten)
		parser.recordNormalizationMetric("ts_type_call_instantiated_candidates", 1, 1, stats.callInstantiatedChildren.nodesVisited, stats.callInstantiatedChildren.nodesRewritten)
		parser.recordNormalizationMetric("ts_type_call_fast_skip", 1, 1, stats.callFastSkipped.nodesVisited, 0)
		parser.recordNormalizationMetric("ts_type_as_child_candidates", 1, 1, stats.asChildren.nodesVisited, stats.asChildren.nodesRewritten)
		parser.recordNormalizationMetric("ts_type_assertion_child_candidates", 1, 1, stats.typeAssertionChildren.nodesVisited, stats.typeAssertionChildren.nodesRewritten)
		parser.recordNormalizationMetric("ts_type_expression_statement_child_candidates", 1, 1, stats.expressionStatementChildren.nodesVisited, stats.expressionStatementChildren.nodesRewritten)
		return stats.total
	})
	runVoid("ts_top_level_expression_bounds", func() {
		normalizeJavaScriptTopLevelExpressionStatementBounds(root, lang)
	})
}

type typeScriptCompatSourceFlags struct {
	hasSemicolon        bool
	hasKeywordStatement bool
	hasCallAngle        bool
	hasUnaryCandidate   bool
	hasBinaryCandidate  bool
	hasTypeKeyword      bool
	hasImportType       bool
	hasMappedTypeSyntax bool
	hasIndexTypeSyntax  bool
	hasMemberModifier   bool
	hasNamespaceModule  bool
	hasOptionalChain    bool
}

func typeScriptCompatSourceFlagsFor(source []byte) typeScriptCompatSourceFlags {
	var flags typeScriptCompatSourceFlags
	for i := 0; i < len(source); {
		switch source[i] {
		case ';':
			flags.hasSemicolon = true
		case '?':
			if i+1 < len(source) && source[i+1] == '.' {
				flags.hasOptionalChain = true
			}
		case '<':
			flags.hasCallAngle = true
			flags.hasBinaryCandidate = true
		case '>', '=', '*', '/', '%', '&', '|', '^':
			flags.hasBinaryCandidate = true
		case '+', '-':
			flags.hasUnaryCandidate = true
			flags.hasBinaryCandidate = true
		case '!', '~':
			flags.hasUnaryCandidate = true
		case '[':
			flags.hasIndexTypeSyntax = true
			if typeScriptSourceRangeHasKeyword(source, i+1, typeScriptSourceBracketEnd(source, i+1), "in") {
				flags.hasMappedTypeSyntax = true
			}
		}
		if !isTypeScriptIdentifierStartByte(source[i]) {
			i++
			continue
		}
		start := i
		i++
		for i < len(source) && isTypeScriptIdentifierByte(source[i]) {
			i++
		}
		switch string(source[start:i]) {
		case "if", "while":
			flags.hasKeywordStatement = true
		case "type", "keyof", "as", "satisfies":
			flags.hasTypeKeyword = true
		case "import":
			if typeScriptImportSourceLooksTypeLike(source, i) {
				flags.hasImportType = true
			}
		case "public", "private", "protected", "readonly", "static", "abstract", "declare", "accessor", "override":
			flags.hasMemberModifier = true
		case "namespace", "module":
			flags.hasNamespaceModule = true
		case "delete", "typeof", "void", "await":
			flags.hasUnaryCandidate = true
		case "in", "instanceof":
			flags.hasBinaryCandidate = true
		}
		continue
	}
	return flags
}

func isTypeScriptIdentifierByte(ch byte) bool {
	return isTypeScriptIdentifierStartByte(ch) || (ch >= '0' && ch <= '9')
}

func typeScriptSourceBracketEnd(source []byte, pos int) int {
	for pos < len(source) {
		if source[pos] == ']' || source[pos] == '\n' || source[pos] == '\r' {
			return pos
		}
		pos++
	}
	return len(source)
}

func typeScriptSourceRangeHasKeyword(source []byte, start, end int, keyword string) bool {
	if start < 0 {
		start = 0
	}
	if end > len(source) {
		end = len(source)
	}
	for i := start; i+len(keyword) <= end; i++ {
		if string(source[i:i+len(keyword)]) == keyword &&
			(i == 0 || !isTypeScriptIdentifierByte(source[i-1])) &&
			(i+len(keyword) >= len(source) || !isTypeScriptIdentifierByte(source[i+len(keyword)])) {
			return true
		}
	}
	return false
}

func typeScriptImportSourceLooksTypeLike(source []byte, pos int) bool {
	for pos < len(source) && isASCIIWhitespace(source[pos]) {
		pos++
	}
	if pos < len(source) && source[pos] == '(' {
		return true
	}
	return typeScriptSourceRangeHasKeyword(source, pos, pos+4, "type")
}

func recordTypeScriptCompatSourceFlagMetrics(parser *Parser, flags typeScriptCompatSourceFlags) {
	if parser == nil {
		return
	}
	record := func(name string, enabled bool) {
		parser.recordNormalizationMetric(name, 1, boolToUint64(enabled), 0, 0)
	}
	record("ts_source_has_semicolon", flags.hasSemicolon)
	record("ts_source_has_keyword_statement", flags.hasKeywordStatement)
	record("ts_source_has_call_angle", flags.hasCallAngle)
	record("ts_source_has_unary_candidate", flags.hasUnaryCandidate)
	record("ts_source_has_binary_candidate", flags.hasBinaryCandidate)
	record("ts_source_has_type_keyword", flags.hasTypeKeyword)
	record("ts_source_has_import_type", flags.hasImportType)
	record("ts_source_has_mapped_type_syntax", flags.hasMappedTypeSyntax)
	record("ts_source_has_index_type_syntax", flags.hasIndexTypeSyntax)
	record("ts_source_has_member_modifier", flags.hasMemberModifier)
	record("ts_source_has_namespace_module", flags.hasNamespaceModule)
	record("ts_source_has_optional_chain", flags.hasOptionalChain)
}

func boolToUint64(v bool) uint64 {
	if v {
		return 1
	}
	return 0
}

type typeScriptCompatCandidateMetrics struct {
	nodesVisited      uint64
	callExpressions   uint64
	unaryExpressions  uint64
	binaryExpressions uint64
	typeNodes         uint64
	importNodes       uint64
	memberNodes       uint64
	namespaceNodes    uint64
}

func recordTypeScriptCompatCandidateMetrics(parser *Parser, root *Node, lang *Language) {
	if parser == nil || root == nil || lang == nil {
		return
	}
	metrics := typeScriptCompatCandidateMetricsFor(root, lang)
	parser.recordNormalizationMetric("ts_candidates_index", 1, 1, metrics.nodesVisited, 0)
	parser.recordNormalizationMetric("ts_candidates_call_expression", 1, 1, metrics.callExpressions, 0)
	parser.recordNormalizationMetric("ts_candidates_unary_expression", 1, 1, metrics.unaryExpressions, 0)
	parser.recordNormalizationMetric("ts_candidates_binary_expression", 1, 1, metrics.binaryExpressions, 0)
	parser.recordNormalizationMetric("ts_candidates_type_nodes", 1, 1, metrics.typeNodes, 0)
	parser.recordNormalizationMetric("ts_candidates_import_nodes", 1, 1, metrics.importNodes, 0)
	parser.recordNormalizationMetric("ts_candidates_member_nodes", 1, 1, metrics.memberNodes, 0)
	parser.recordNormalizationMetric("ts_candidates_namespace_nodes", 1, 1, metrics.namespaceNodes, 0)
}

func typeScriptCompatCandidateMetricsFor(root *Node, lang *Language) typeScriptCompatCandidateMetrics {
	var metrics typeScriptCompatCandidateMetrics
	syms := typeScriptCompatCandidateSymbolsFor(lang)
	walkResultTreeDenseFirst(root, func(n *Node) {
		metrics.nodesVisited++
		switch {
		case syms.hasCallExpression && n.symbol == syms.callExpression:
			metrics.callExpressions++
		case syms.hasUnaryExpression && n.symbol == syms.unaryExpression:
			metrics.unaryExpressions++
		case syms.hasBinaryExpression && n.symbol == syms.binaryExpression:
			metrics.binaryExpressions++
		case (syms.hasImportStatement && n.symbol == syms.importStatement) ||
			(syms.hasImportType && n.symbol == syms.importType) ||
			(syms.hasImportKeyword && n.symbol == syms.importKeyword):
			metrics.importNodes++
		case syms.hasInternalModule && n.symbol == syms.internalModule:
			metrics.namespaceNodes++
		case (syms.hasMethodDefinition && n.symbol == syms.methodDefinition) ||
			(syms.hasMethodSignature && n.symbol == syms.methodSignature) ||
			(syms.hasPropertySignature && n.symbol == syms.propertySignature) ||
			(syms.hasPublicFieldDefinition && n.symbol == syms.publicFieldDefinition):
			metrics.memberNodes++
		case (syms.hasTypeAnnotation && n.symbol == syms.typeAnnotation) ||
			(syms.hasTypeIdentifier && n.symbol == syms.typeIdentifier) ||
			(syms.hasTypeQuery && n.symbol == syms.typeQuery) ||
			(syms.hasMappedType && n.symbol == syms.mappedType) ||
			(syms.hasIndexSignature && n.symbol == syms.indexSignature) ||
			(syms.hasIndexedAccessType && n.symbol == syms.indexedAccessType):
			metrics.typeNodes++
		}
	})
	return metrics
}

type typeScriptCompatCandidateSymbols struct {
	callExpression           Symbol
	hasCallExpression        bool
	unaryExpression          Symbol
	hasUnaryExpression       bool
	binaryExpression         Symbol
	hasBinaryExpression      bool
	importStatement          Symbol
	hasImportStatement       bool
	importType               Symbol
	hasImportType            bool
	importKeyword            Symbol
	hasImportKeyword         bool
	internalModule           Symbol
	hasInternalModule        bool
	methodDefinition         Symbol
	hasMethodDefinition      bool
	methodSignature          Symbol
	hasMethodSignature       bool
	propertySignature        Symbol
	hasPropertySignature     bool
	publicFieldDefinition    Symbol
	hasPublicFieldDefinition bool
	typeAnnotation           Symbol
	hasTypeAnnotation        bool
	typeIdentifier           Symbol
	hasTypeIdentifier        bool
	typeQuery                Symbol
	hasTypeQuery             bool
	mappedType               Symbol
	hasMappedType            bool
	indexSignature           Symbol
	hasIndexSignature        bool
	indexedAccessType        Symbol
	hasIndexedAccessType     bool
}

func typeScriptCompatCandidateSymbolsFor(lang *Language) typeScriptCompatCandidateSymbols {
	var syms typeScriptCompatCandidateSymbols
	syms.callExpression, syms.hasCallExpression = symbolByName(lang, "call_expression")
	syms.unaryExpression, syms.hasUnaryExpression = symbolByName(lang, "unary_expression")
	syms.binaryExpression, syms.hasBinaryExpression = symbolByName(lang, "binary_expression")
	syms.importStatement, syms.hasImportStatement = symbolByName(lang, "import_statement")
	syms.importType, syms.hasImportType = symbolByName(lang, "import_type")
	syms.importKeyword, syms.hasImportKeyword = symbolByName(lang, "import")
	syms.internalModule, syms.hasInternalModule = symbolByName(lang, "internal_module")
	syms.methodDefinition, syms.hasMethodDefinition = symbolByName(lang, "method_definition")
	syms.methodSignature, syms.hasMethodSignature = symbolByName(lang, "method_signature")
	syms.propertySignature, syms.hasPropertySignature = symbolByName(lang, "property_signature")
	syms.publicFieldDefinition, syms.hasPublicFieldDefinition = symbolByName(lang, "public_field_definition")
	syms.typeAnnotation, syms.hasTypeAnnotation = symbolByName(lang, "type_annotation")
	syms.typeIdentifier, syms.hasTypeIdentifier = symbolByName(lang, "type_identifier")
	syms.typeQuery, syms.hasTypeQuery = symbolByName(lang, "type_query")
	syms.mappedType, syms.hasMappedType = symbolByName(lang, "mapped_type")
	syms.indexSignature, syms.hasIndexSignature = symbolByName(lang, "index_signature")
	syms.indexedAccessType, syms.hasIndexedAccessType = symbolByName(lang, "indexed_access_type")
	return syms
}

func normalizeJavaScriptTypeScriptStatementKeywordLeaves(root *Node, source []byte, lang *Language) {
	if root == nil || lang == nil || len(source) == 0 {
		return
	}
	switch lang.Name {
	case "javascript", "typescript", "tsx":
	default:
		return
	}
	ifStmtSym, hasIfStmt := symbolByName(lang, "if_statement")
	whileStmtSym, hasWhileStmt := symbolByName(lang, "while_statement")
	ifSym, ifNamed, hasIf := symbolMeta(lang, "if")
	whileSym, whileNamed, hasWhile := symbolMeta(lang, "while")
	closeBraceSym, hasCloseBrace := symbolByName(lang, "}")
	if (!hasIfStmt || !hasIf) && (!hasWhileStmt || !hasWhile) {
		return
	}

	walkResultTreeDenseFirst(root, func(n *Node) {
		if hasIfStmt && hasIf && n.symbol == ifStmtSym {
			normalizeJavaScriptTypeScriptStatementKeywordLeafWithSymbol(n, source, "if", ifSym, ifNamed, closeBraceSym, hasCloseBrace)
			return
		}
		if hasWhileStmt && hasWhile && n.symbol == whileStmtSym {
			normalizeJavaScriptTypeScriptStatementKeywordLeafWithSymbol(n, source, "while", whileSym, whileNamed, closeBraceSym, hasCloseBrace)
		}
	})
}

func normalizeJavaScriptTypeScriptStatementKeywordLeavesWithStats(root *Node, source []byte, lang *Language) normalizationPassCounters {
	var counters normalizationPassCounters
	if root == nil || lang == nil || len(source) == 0 {
		return counters
	}
	switch lang.Name {
	case "javascript", "typescript", "tsx":
	default:
		return counters
	}
	ifStmtSym, hasIfStmt := symbolByName(lang, "if_statement")
	whileStmtSym, hasWhileStmt := symbolByName(lang, "while_statement")
	ifSym, ifNamed, hasIf := symbolMeta(lang, "if")
	whileSym, whileNamed, hasWhile := symbolMeta(lang, "while")
	closeBraceSym, hasCloseBrace := symbolByName(lang, "}")
	if (!hasIfStmt || !hasIf) && (!hasWhileStmt || !hasWhile) {
		return counters
	}

	walkResultTreeDenseFirst(root, func(n *Node) {
		counters.nodesVisited++
		if hasIfStmt && hasIf && n.symbol == ifStmtSym {
			if normalizeJavaScriptTypeScriptStatementKeywordLeafWithSymbolChanged(n, source, "if", ifSym, ifNamed, closeBraceSym, hasCloseBrace) {
				counters.nodesRewritten++
			}
			return
		}
		if hasWhileStmt && hasWhile && n.symbol == whileStmtSym {
			if normalizeJavaScriptTypeScriptStatementKeywordLeafWithSymbolChanged(n, source, "while", whileSym, whileNamed, closeBraceSym, hasCloseBrace) {
				counters.nodesRewritten++
			}
		}
	})
	return counters
}

func normalizeJavaScriptTypeScriptStatementKeywordLeaf(n *Node, source []byte, lang *Language, keyword string) {
	keywordSym, ok := symbolByName(lang, keyword)
	if !ok {
		return
	}
	normalizeJavaScriptTypeScriptStatementKeywordLeafWithSymbol(n, source, keyword, keywordSym, symbolIsNamed(lang, keywordSym), 0, false)
}

func normalizeJavaScriptTypeScriptStatementKeywordLeafWithSymbol(n *Node, source []byte, keyword string, keywordSym Symbol, keywordNamed bool, closeBraceSym Symbol, hasCloseBrace bool) {
	_ = normalizeJavaScriptTypeScriptStatementKeywordLeafWithSymbolChanged(n, source, keyword, keywordSym, keywordNamed, closeBraceSym, hasCloseBrace)
}

func normalizeJavaScriptTypeScriptStatementKeywordLeafWithSymbolChanged(n *Node, source []byte, keyword string, keywordSym Symbol, keywordNamed bool, closeBraceSym Symbol, hasCloseBrace bool) bool {
	end := n.startByte + uint32(len(keyword))
	if int(end) > len(source) || !bytes.Equal(source[n.startByte:end], []byte(keyword)) {
		return false
	}
	childCount := resultChildCount(n)
	if childCount == 0 {
		keywordNode := newLeafNodeInArena(n.ownerArena, keywordSym, keywordNamed, n.startByte, end, n.startPoint, advancePointByBytes(n.startPoint, source[n.startByte:end]))
		replaceNodeChildrenUnfielded(n, cloneNodeSliceInArena(n.ownerArena, []*Node{keywordNode}))
		return true
	}
	first := resultChildAt(n, 0)
	if first != nil && first.symbol == keywordSym && first.startByte == n.startByte && first.endByte == end {
		return false
	}
	keywordNode := newLeafNodeInArena(n.ownerArena, keywordSym, keywordNamed, n.startByte, end, n.startPoint, advancePointByBytes(n.startPoint, source[n.startByte:end]))

	children := make([]*Node, 0, childCount+1)
	for i := 0; i < childCount; i++ {
		children = append(children, resultChildAt(n, i))
	}
	if first != nil && hasCloseBrace && first.symbol == closeBraceSym {
		children[0] = keywordNode
		if len(n.fieldIDs) == childCount {
			n.fieldIDs[0] = 0
		}
		if len(n.fieldSources) == childCount {
			n.fieldSources[0] = fieldSourceNone
		}
	} else if first == nil || first.startByte > n.startByte {
		children = append([]*Node{keywordNode}, children...)
		n.fieldIDs = prependFieldID(n.ownerArena, n.fieldIDs, childCount)
		n.fieldSources = prependFieldSource(n.ownerArena, n.fieldSources, childCount)
	} else {
		children[0] = keywordNode
		if len(n.fieldIDs) == childCount {
			n.fieldIDs[0] = 0
		}
		if len(n.fieldSources) == childCount {
			n.fieldSources[0] = fieldSourceNone
		}
	}
	n.children = cloneNodeSliceInArena(n.ownerArena, children)
	if n.ownerArena != nil {
		n.ownerArena.clearFinalChildRefs(n)
	}
	populateParentNode(n, n.children)
	return true
}

func prependFieldID(arena *nodeArena, fieldIDs []FieldID, oldLen int) []FieldID {
	if len(fieldIDs) != oldLen {
		return nil
	}
	out := make([]FieldID, oldLen+1)
	copy(out[1:], fieldIDs)
	return cloneFieldIDSliceInArena(arena, out)
}

func prependFieldSource(arena *nodeArena, fieldSources []uint8, oldLen int) []uint8 {
	if len(fieldSources) != oldLen {
		return nil
	}
	out := make([]uint8, oldLen+1)
	copy(out[1:], fieldSources)
	if arena != nil {
		buf := arena.allocFieldSourceSlice(len(out))
		copy(buf, out)
		return buf
	}
	return out
}

func normalizeJavaScriptTopLevelObjectLiterals(root *Node, lang *Language) {
	if root == nil || lang == nil || lang.Name != "javascript" || root.Type(lang) != "program" {
		return
	}
	exprSym, exprNamed, ok := symbolMeta(lang, "expression_statement")
	if !ok {
		return
	}
	objectSym, objectNamed, ok := symbolMeta(lang, "object")
	if !ok {
		return
	}
	pairSym, pairNamed, ok := symbolMeta(lang, "pair")
	if !ok {
		return
	}
	propSym, _, ok := symbolMeta(lang, "property_identifier")
	if !ok {
		return
	}
	for i, child := range root.children {
		repl, ok := rewriteJavaScriptTopLevelObjectLiteral(child, lang, root.ownerArena, exprSym, exprNamed, objectSym, objectNamed, pairSym, pairNamed, propSym)
		if ok {
			root.children[i] = repl
		}
	}
}

func normalizeJavaScriptProgramStart(root *Node, lang *Language) {
	if root == nil || lang == nil || lang.Name != "javascript" || root.Type(lang) != "program" {
		return
	}
	first, _ := firstAndLastNonNilChild(root.children)
	if first == nil {
		return
	}
	root.startByte = first.startByte
	root.startPoint = first.startPoint
}

func normalizeJavaScriptProgramEnd(root *Node, source []byte, lang *Language) {
	if root == nil || lang == nil || lang.Name != "javascript" || root.endByte >= uint32(len(source)) {
		return
	}
	switch root.Type(lang) {
	case "program", "ERROR":
	default:
		return
	}
	tail := source[root.endByte:]
	if !bytesAreTrivia(tail) && !bytesAreJavaScriptStatementTerminatorTail(tail) {
		return
	}
	extendNodeEndTo(root, uint32(len(source)), source)
}

func bytesAreJavaScriptStatementTerminatorTail(b []byte) bool {
	seenSemicolon := false
	for _, c := range b {
		switch c {
		case ';':
			seenSemicolon = true
		case ' ', '\t', '\n', '\r':
		default:
			return false
		}
	}
	return seenSemicolon
}

func normalizeJavaScriptTopLevelExpressionStatementBounds(root *Node, lang *Language) {
	if root == nil || lang == nil || root.Type(lang) != "program" {
		return
	}
	switch lang.Name {
	case "javascript", "typescript", "tsx":
	default:
		return
	}
	if normalizeJavaScriptTopLevelBoundsFinalRefs(root, lang, func(name string) bool {
		return name == "expression_statement"
	}) {
		return
	}
	for _, child := range root.children {
		if child == nil || child.Type(lang) != "expression_statement" || len(child.children) == 0 {
			continue
		}
		first, last := firstAndLastNonNilChild(child.children)
		if first == nil || last == nil {
			continue
		}
		child.startByte = first.startByte
		child.startPoint = first.startPoint
		child.endByte = last.endByte
		child.endPoint = last.endPoint
	}
}

func normalizeJavaScriptTopLevelDeclarationBounds(root *Node, lang *Language) {
	if root == nil || lang == nil || root.Type(lang) != "program" {
		return
	}
	switch lang.Name {
	case "javascript", "typescript", "tsx":
	default:
		return
	}
	if normalizeJavaScriptTopLevelBoundsFinalRefs(root, lang, func(name string) bool {
		switch name {
		case "lexical_declaration",
			"variable_declaration",
			"function_declaration",
			"generator_function_declaration",
			"class_declaration",
			"import_statement",
			"export_statement":
			return true
		default:
			return false
		}
	}) {
		return
	}
	for _, child := range root.children {
		if child == nil || len(child.children) == 0 {
			continue
		}
		switch child.Type(lang) {
		case "lexical_declaration",
			"variable_declaration",
			"function_declaration",
			"generator_function_declaration",
			"class_declaration",
			"import_statement",
			"export_statement":
		default:
			continue
		}
		first, last := firstAndLastNonNilChild(child.children)
		if first == nil || last == nil {
			continue
		}
		child.startByte = first.startByte
		child.startPoint = first.startPoint
		child.endByte = last.endByte
		child.endPoint = last.endPoint
	}
}

func normalizeJavaScriptTopLevelBoundsFinalRefs(root *Node, lang *Language, match func(string) bool) bool {
	view := resultMutableChildrenForMutation(root)
	if !view.hasFinalChildRefs() {
		return false
	}
	for i := 0; i < view.Len(); i++ {
		entry, ok := view.Entry(i)
		if !ok || !match(symbolTypeName(lang, stackEntryNodeSymbol(entry))) {
			continue
		}
		first, last, ok := firstAndLastStackEntryChild(root.ownerArena, entry)
		if !ok {
			continue
		}
		setStackEntryStart(entry, stackEntryNodeStartByte(first), stackEntryNodeStartPoint(first))
		setStackEntryEnd(entry, stackEntryNodeEndByte(last), stackEntryNodeEndPoint(last))
	}
	return true
}

func firstAndLastStackEntryChild(arena *nodeArena, entry stackEntry) (stackEntry, stackEntry, bool) {
	childCount := stackEntryNodeChildCount(entry)
	if childCount == 0 {
		return stackEntry{}, stackEntry{}, false
	}
	childAt := func(i int) (stackEntry, bool) {
		if parent := stackEntryPendingParent(entry); parent != nil {
			child := parent.childEntry(arena, i)
			return child, stackEntryHasNode(child)
		}
		if node := stackEntryNode(entry); node != nil {
			return nodeChildEntryAtNoMaterialize(node, i)
		}
		return stackEntry{}, false
	}
	firstIdx := 0
	first, ok := childAt(firstIdx)
	for !ok && firstIdx+1 < childCount {
		firstIdx++
		first, ok = childAt(firstIdx)
	}
	if !ok {
		return stackEntry{}, stackEntry{}, false
	}
	lastIdx := childCount - 1
	last, ok := childAt(lastIdx)
	for !ok && lastIdx > firstIdx {
		lastIdx--
		last, ok = childAt(lastIdx)
	}
	if !ok {
		return stackEntry{}, stackEntry{}, false
	}
	return first, last, true
}

func setStackEntryStart(entry stackEntry, startByte uint32, startPoint Point) {
	if node := stackEntryNode(entry); node != nil {
		node.startByte = startByte
		node.startPoint = startPoint
		return
	}
	if node := stackEntryNoTreeNode(entry); node != nil {
		node.startByte = startByte
		return
	}
	if leaf := stackEntryCompactFullLeaf(entry); leaf != nil {
		leaf.startByte = startByte
		leaf.startPoint = startPoint
		return
	}
	if parent := stackEntryPendingParent(entry); parent != nil {
		parent.startByte = startByte
		parent.startPoint = startPoint
	}
}

func normalizeJavaScriptTrailingContinueComments(root *Node, source []byte, lang *Language) {
	if root == nil || lang == nil || lang.Name != "javascript" || len(source) == 0 {
		return
	}
	// Source-level gate: this pass only acts on continue_statement nodes
	// with trailing comments. If source has neither "continue" nor "//",
	// no candidate exists and the walk is wasted on large files like jquery.
	if !bytes.Contains(source, []byte("continue")) || !bytes.Contains(source, []byte("//")) {
		return
	}
	walkResultTreeDenseFirst(root, func(n *Node) {
		normalizeJavaScriptTrailingContinueCommentSiblings(n, source, lang)
	})
}

func normalizeJavaScriptTrailingContinueCommentSiblings(parent *Node, source []byte, lang *Language) {
	if parent == nil || len(parent.children) < 3 || parent.Type(lang) != "statement_block" {
		return
	}
	for i := 1; i+1 < len(parent.children); i++ {
		if comment, ok := extractJavaScriptTrailingContinueComment(parent.children[i], source, lang); ok {
			insertJavaScriptStatementBlockComment(parent, i, comment)
			i++
			continue
		}
		stmt := parent.children[i]
		if stmt == nil || stmt.Type(lang) != "if_statement" || len(stmt.children) < 3 {
			continue
		}
		branch := stmt.children[len(stmt.children)-1]
		comment, ok := extractJavaScriptTrailingContinueComment(branch, source, lang)
		if !ok {
			continue
		}
		stmt.endByte = branch.endByte
		stmt.endPoint = branch.endPoint
		insertJavaScriptStatementBlockComment(parent, i, comment)
		i++
	}
}

func extractJavaScriptTrailingContinueComment(node *Node, source []byte, lang *Language) (*Node, bool) {
	if node == nil || lang == nil || node.Type(lang) != "continue_statement" || len(node.children) < 3 {
		return nil, false
	}
	comment := node.children[len(node.children)-1]
	if comment == nil || comment.Type(lang) != "comment" || comment.startByte >= comment.endByte {
		return nil, false
	}
	if int(comment.endByte) > len(source) || !bytes.HasPrefix(source[comment.startByte:comment.endByte], []byte("//")) {
		return nil, false
	}
	prev := node.children[len(node.children)-2]
	if prev == nil || prev.endByte > comment.startByte || bytesContainLineBreak(source[prev.endByte:comment.startByte]) {
		return nil, false
	}
	node.children = node.children[:len(node.children)-1]
	if len(node.fieldIDs) > len(node.children) {
		node.fieldIDs = node.fieldIDs[:len(node.children)]
		if len(node.fieldSources) > len(node.children) {
			node.fieldSources = node.fieldSources[:len(node.children)]
		}
	}
	node.endByte = prev.endByte
	node.endPoint = prev.endPoint
	return comment, true
}

func insertJavaScriptStatementBlockComment(parent *Node, childIdx int, comment *Node) {
	if parent == nil || comment == nil || childIdx < 0 || childIdx >= len(parent.children) {
		return
	}
	parent.children = append(parent.children[:childIdx+1], append([]*Node{comment}, parent.children[childIdx+1:]...)...)
	if len(parent.fieldIDs) > 0 {
		fieldIDs := append([]FieldID(nil), parent.fieldIDs[:childIdx+1]...)
		fieldIDs = append(fieldIDs, 0)
		fieldIDs = append(fieldIDs, parent.fieldIDs[childIdx+1:]...)
		parent.fieldIDs = fieldIDs
		if len(parent.fieldSources) > 0 {
			fieldSources := append([]uint8(nil), parent.fieldSources[:childIdx+1]...)
			fieldSources = append(fieldSources, fieldSourceNone)
			fieldSources = append(fieldSources, parent.fieldSources[childIdx+1:]...)
			parent.fieldSources = fieldSources
		}
	}
	populateParentNode(parent, parent.children)
}

func normalizeJavaScriptTypeScriptOptionalChainLeaves(root *Node, source []byte, lang *Language) {
	if root == nil || lang == nil || !bytes.Contains(source, []byte("?.")) {
		return
	}
	switch lang.Name {
	case "javascript", "typescript", "tsx":
	default:
		return
	}
	optionalChainSym, ok := symbolByName(lang, "optional_chain")
	if !ok {
		return
	}
	optionalChainTokenSym, ok := symbolByName(lang, "?.")
	if !ok {
		return
	}

	walkResultTreeDenseFirst(root, func(n *Node) {
		if n.symbol != optionalChainSym || len(n.children) != 0 {
			return
		}
		if n.endByte <= n.startByte || int(n.endByte) > len(source) || !bytes.Equal(source[n.startByte:n.endByte], []byte("?.")) {
			return
		}
		child := newLeafNodeInArena(n.ownerArena, optionalChainTokenSym, symbolIsNamed(lang, optionalChainTokenSym), n.startByte, n.endByte, n.startPoint, n.endPoint)
		children := phpAllocChildren(n.ownerArena, 1)
		children[0] = child
		n.children = children
		n.fieldIDs = nil
		n.fieldSources = nil
		if n.ownerArena != nil {
			n.ownerArena.clearFinalChildRefs(n)
		}
		populateParentNode(n, n.children)
	})
}

func normalizeJavaScriptTypeScriptCallPrecedence(root *Node, lang *Language) {
	if root == nil || lang == nil {
		return
	}
	switch lang.Name {
	case "javascript", "typescript", "tsx":
	default:
		return
	}
	callSym, ok := symbolByName(lang, "call_expression")
	if !ok {
		return
	}
	if lang.Name == "javascript" {
		_ = normalizeJavaScriptTypeScriptCallPrecedenceFullWalk(root, lang, callSym)
		return
	}

	index := buildJavaScriptTypeScriptCallPrecedenceIndex(root, callSym)
	rewriteJavaScriptTypeScriptCallPrecedenceCandidates(index.candidates, lang, callSym)
}

func normalizeJavaScriptTypeScriptCallPrecedenceWithStats(root *Node, lang *Language) normalizationPassCounters {
	return normalizeJavaScriptTypeScriptCallPrecedenceWithDetailedStats(root, lang).normalizationPassCounters
}

type javaScriptTypeScriptCallPrecedenceStats struct {
	normalizationPassCounters
	indexBuilds       uint64
	indexNodesVisited uint64
	candidateCalls    uint64
}

func normalizeJavaScriptTypeScriptCallPrecedenceWithDetailedStats(root *Node, lang *Language) javaScriptTypeScriptCallPrecedenceStats {
	var stats javaScriptTypeScriptCallPrecedenceStats
	if root == nil || lang == nil {
		return stats
	}
	switch lang.Name {
	case "javascript", "typescript", "tsx":
	default:
		return stats
	}
	callSym, ok := symbolByName(lang, "call_expression")
	if !ok {
		return stats
	}
	if lang.Name == "javascript" {
		stats.normalizationPassCounters = normalizeJavaScriptTypeScriptCallPrecedenceFullWalk(root, lang, callSym)
		return stats
	}

	index := buildJavaScriptTypeScriptCallPrecedenceIndex(root, callSym)
	stats.indexBuilds = index.builds
	stats.indexNodesVisited = index.nodesVisited
	stats.candidateCalls = uint64(len(index.candidates))
	stats.normalizationPassCounters = rewriteJavaScriptTypeScriptCallPrecedenceCandidates(index.candidates, lang, callSym)
	return stats
}

func normalizeJavaScriptTypeScriptCallPrecedenceFullWalk(root *Node, lang *Language, callSym Symbol) normalizationPassCounters {
	var counters normalizationPassCounters
	walkResultTreeDenseFirst(root, func(n *Node) {
		counters.nodesVisited++
		for i, child := range n.children {
			if rewritten := rewriteJavaScriptTypeScriptCallPrecedenceWithSymbol(child, lang, callSym); rewritten != nil {
				counters.nodesRewritten++
				n.children[i] = rewritten
				rewritten.parent = n
				rewritten.childIndex = int32(i)
			}
		}
	})
	return counters
}

func normalizeJavaScriptTypeScriptPrecedence(root *Node, lang *Language) {
	if root == nil || lang == nil {
		return
	}
	if lang.Name == "javascript" {
		normalizeJavaScriptTypeScriptCallPrecedence(root, lang)
		normalizeJavaScriptTypeScriptUnaryPrecedence(root, lang)
		normalizeJavaScriptTypeScriptBinaryPrecedence(root, lang)
		return
	}
	_ = normalizeJavaScriptTypeScriptPrecedenceWithDetailedStats(root, lang)
}

type javaScriptTypeScriptPrecedenceStats struct {
	call              normalizationPassCounters
	unary             normalizationPassCounters
	binary            normalizationPassCounters
	indexBuilds       uint64
	indexNodesVisited uint64
}

type javaScriptTypeScriptSyntaxNormalizationStats struct {
	emptyStatement    normalizationPassCounters
	existentialType   normalizationPassCounters
	statementKeyword  normalizationPassCounters
	call              normalizationPassCounters
	unary             normalizationPassCounters
	binary            normalizationPassCounters
	indexBuilds       uint64
	indexNodesVisited uint64
}

func normalizeJavaScriptTypeScriptStatementKeywordsAndPrecedence(root *Node, source []byte, lang *Language) {
	_ = normalizeJavaScriptTypeScriptStatementKeywordsAndPrecedenceWithDetailedStats(root, source, lang)
}

func normalizeJavaScriptTypeScriptStatementKeywordsAndPrecedenceWithDetailedStats(root *Node, source []byte, lang *Language) javaScriptTypeScriptSyntaxNormalizationStats {
	var stats javaScriptTypeScriptSyntaxNormalizationStats
	if root == nil || lang == nil {
		return stats
	}
	switch lang.Name {
	case "javascript", "typescript", "tsx":
	default:
		stats.statementKeyword = normalizeJavaScriptTypeScriptStatementKeywordLeavesWithStats(root, source, lang)
		precedence := normalizeJavaScriptTypeScriptPrecedenceWithDetailedStats(root, lang)
		stats.call = precedence.call
		stats.unary = precedence.unary
		stats.binary = precedence.binary
		stats.indexBuilds = precedence.indexBuilds
		stats.indexNodesVisited = precedence.indexNodesVisited
		return stats
	}
	callSym, hasCallSym := symbolByName(lang, "call_expression")
	unarySym, hasUnarySym := symbolByName(lang, "unary_expression")
	binarySym, hasBinarySym := symbolByName(lang, "binary_expression")
	emptyStatementSym, hasEmptyStatement := symbolByName(lang, "empty_statement")
	semicolonSym, semicolonNamed, hasSemicolon := symbolMeta(lang, ";")
	existentialTypeSym, hasExistentialType := symbolByName(lang, "existential_type")
	starSym, starNamed, hasStar := symbolMeta(lang, "*")
	ifStmtSym, hasIfStmt := symbolByName(lang, "if_statement")
	whileStmtSym, hasWhileStmt := symbolByName(lang, "while_statement")
	ifSym, ifNamed, hasIf := symbolMeta(lang, "if")
	whileSym, whileNamed, hasWhile := symbolMeta(lang, "while")
	closeBraceSym, hasCloseBrace := symbolByName(lang, "}")

	// Source-level gate: only enable the optional-chain leaf handler when "?."
	// actually appears in the file, mirroring the standalone pass's gate so we
	// don't slow down files that have no chance of matching.
	optionalChainSym, hasOptionalChainSym := symbolByName(lang, "optional_chain")
	optionalChainTokenSym, hasOptionalChainTokenSym := symbolByName(lang, "?.")
	optionalChainTokenNamed := hasOptionalChainTokenSym && symbolIsNamed(lang, optionalChainTokenSym)
	enableOptionalChain := hasOptionalChainSym && hasOptionalChainTokenSym && bytes.Contains(source, []byte("?."))

	index := rewriteJavaScriptTypeScriptStatementKeywordsCallPrecedenceAndBuildUnaryBinaryIndex(
		root, source, lang,
		callSym, hasCallSym, unarySym, hasUnarySym, binarySym, hasBinarySym,
		emptyStatementSym, hasEmptyStatement, semicolonSym, semicolonNamed, hasSemicolon,
		existentialTypeSym, hasExistentialType, starSym, starNamed, hasStar,
		ifStmtSym, hasIfStmt, ifSym, ifNamed, hasIf,
		whileStmtSym, hasWhileStmt, whileSym, whileNamed, hasWhile,
		closeBraceSym, hasCloseBrace,
		optionalChainSym, optionalChainTokenSym, optionalChainTokenNamed, enableOptionalChain,
	)
	stats.emptyStatement = index.emptyStatement
	stats.existentialType = index.existentialType
	stats.statementKeyword = index.statementKeyword
	stats.call = index.call
	stats.indexBuilds += index.builds
	stats.indexNodesVisited += index.nodesVisited
	if hasUnarySym {
		stats.unary = rewriteJavaScriptTypeScriptPrecedenceCandidates(index.unaryCandidates, func(n *Node) *Node {
			return rewriteJavaScriptTypeScriptUnaryPrecedenceWithSymbol(n, lang, unarySym)
		})
		if stats.unary.nodesRewritten != 0 && hasBinarySym {
			rebuild := buildJavaScriptTypeScriptUnaryBinaryPrecedenceIndex(root, unarySym, binarySym)
			stats.indexBuilds += rebuild.builds
			stats.indexNodesVisited += rebuild.nodesVisited
			index.binaryCandidates = rebuild.binaryCandidates
		}
	}
	if hasBinarySym {
		stats.binary = rewriteJavaScriptTypeScriptPrecedenceCandidates(index.binaryCandidates, func(n *Node) *Node {
			return rewriteJavaScriptTypeScriptBinaryPrecedenceWithSymbol(n, lang, binarySym)
		})
	}
	return stats
}

func normalizeJavaScriptTypeScriptPrecedenceWithDetailedStats(root *Node, lang *Language) javaScriptTypeScriptPrecedenceStats {
	var stats javaScriptTypeScriptPrecedenceStats
	if root == nil || lang == nil {
		return stats
	}
	switch lang.Name {
	case "typescript", "tsx":
	default:
		stats.call = normalizeJavaScriptTypeScriptCallPrecedenceWithStats(root, lang)
		stats.unary = normalizeJavaScriptTypeScriptUnaryPrecedenceWithStats(root, lang)
		stats.binary = normalizeJavaScriptTypeScriptBinaryPrecedenceWithStats(root, lang)
		return stats
	}
	callSym, ok := symbolByName(lang, "call_expression")
	if !ok {
		return stats
	}
	unarySym, ok := symbolByName(lang, "unary_expression")
	if !ok {
		return stats
	}
	binarySym, ok := symbolByName(lang, "binary_expression")
	if !ok {
		return stats
	}

	index := rewriteJavaScriptTypeScriptCallPrecedenceAndBuildUnaryBinaryIndex(root, lang, callSym, unarySym, binarySym)
	stats.call = index.call
	stats.indexBuilds += index.builds
	stats.indexNodesVisited += index.nodesVisited
	stats.unary = rewriteJavaScriptTypeScriptPrecedenceCandidates(index.unaryCandidates, func(n *Node) *Node {
		return rewriteJavaScriptTypeScriptUnaryPrecedenceWithSymbol(n, lang, unarySym)
	})
	if stats.unary.nodesRewritten != 0 {
		rebuild := buildJavaScriptTypeScriptUnaryBinaryPrecedenceIndex(root, unarySym, binarySym)
		stats.indexBuilds += rebuild.builds
		stats.indexNodesVisited += rebuild.nodesVisited
		index.binaryCandidates = rebuild.binaryCandidates
	}
	stats.binary = rewriteJavaScriptTypeScriptPrecedenceCandidates(index.binaryCandidates, func(n *Node) *Node {
		return rewriteJavaScriptTypeScriptBinaryPrecedenceWithSymbol(n, lang, binarySym)
	})
	return stats
}

func rewriteJavaScriptTypeScriptStatementKeywordsCallPrecedenceAndBuildUnaryBinaryIndex(
	root *Node,
	source []byte,
	lang *Language,
	callSym Symbol,
	hasCallSym bool,
	unarySym Symbol,
	hasUnarySym bool,
	binarySym Symbol,
	hasBinarySym bool,
	emptyStatementSym Symbol,
	hasEmptyStatement bool,
	semicolonSym Symbol,
	semicolonNamed bool,
	hasSemicolon bool,
	existentialTypeSym Symbol,
	hasExistentialType bool,
	starSym Symbol,
	starNamed bool,
	hasStar bool,
	ifStmtSym Symbol,
	hasIfStmt bool,
	ifSym Symbol,
	ifNamed bool,
	hasIf bool,
	whileStmtSym Symbol,
	hasWhileStmt bool,
	whileSym Symbol,
	whileNamed bool,
	hasWhile bool,
	closeBraceSym Symbol,
	hasCloseBrace bool,
	optionalChainSym Symbol,
	optionalChainTokenSym Symbol,
	optionalChainTokenNamed bool,
	enableOptionalChain bool,
) javaScriptTypeScriptUnaryBinaryPrecedenceIndex {
	var index javaScriptTypeScriptUnaryBinaryPrecedenceIndex
	if root == nil {
		return index
	}
	// If none of the symbols this walk targets exists in the language, there
	// is nothing to do; returning early avoids materializing final-ref
	// children for callers (e.g. mock-lang unit tests) that exercise only
	// other compat passes.
	if !hasCallSym && !hasUnarySym && !hasBinarySym &&
		!hasEmptyStatement && !hasExistentialType &&
		!hasIfStmt && !hasWhileStmt && !enableOptionalChain {
		return index
	}
	index.builds = 1
	var walk func(*Node)
	walk = func(n *Node) {
		if n == nil {
			return
		}
		index.nodesVisited++
		if hasEmptyStatement && hasSemicolon && n.symbol == emptyStatementSym {
			index.emptyStatement.nodesVisited++
			if normalizeJavaScriptTypeScriptEmptyStatementLeafWithSymbolChanged(n, source, semicolonSym, semicolonNamed) {
				index.emptyStatement.nodesRewritten++
			}
		}
		if hasExistentialType && hasStar && n.symbol == existentialTypeSym {
			index.existentialType.nodesVisited++
			if normalizeJavaScriptTypeScriptCollapsedLeafWithSymbolChanged(n, starSym, starNamed) {
				index.existentialType.nodesRewritten++
			}
		}
		if hasIfStmt && hasIf && n.symbol == ifStmtSym {
			index.statementKeyword.nodesVisited++
			if normalizeJavaScriptTypeScriptStatementKeywordLeafWithSymbolChanged(n, source, "if", ifSym, ifNamed, closeBraceSym, hasCloseBrace) {
				index.statementKeyword.nodesRewritten++
			}
		} else if hasWhileStmt && hasWhile && n.symbol == whileStmtSym {
			index.statementKeyword.nodesVisited++
			if normalizeJavaScriptTypeScriptStatementKeywordLeafWithSymbolChanged(n, source, "while", whileSym, whileNamed, closeBraceSym, hasCloseBrace) {
				index.statementKeyword.nodesRewritten++
			}
		} else if enableOptionalChain && n.symbol == optionalChainSym && len(n.children) == 0 {
			if n.endByte > n.startByte && int(n.endByte) <= len(source) && bytes.Equal(source[n.startByte:n.endByte], []byte("?.")) {
				child := newLeafNodeInArena(n.ownerArena, optionalChainTokenSym, optionalChainTokenNamed, n.startByte, n.endByte, n.startPoint, n.endPoint)
				children := phpAllocChildren(n.ownerArena, 1)
				children[0] = child
				n.children = children
				n.fieldIDs = nil
				n.fieldSources = nil
				if n.ownerArena != nil {
					n.ownerArena.clearFinalChildRefs(n)
				}
				populateParentNode(n, n.children)
			}
		}

		if nodeHasFinalChildRefs(n) {
			childCount := resultChildCount(n)
			for i := 0; i < childCount; i++ {
				child := resultChildAt(n, i)
				if child == nil {
					continue
				}
				if hasCallSym && child.symbol == callSym {
					index.call.nodesVisited++
					if rewritten := rewriteJavaScriptTypeScriptCallPrecedenceWithSymbol(child, lang, callSym); rewritten != nil {
						if replaceJavaScriptTypeScriptPrecedenceCandidate(javaScriptTypeScriptPrecedenceCandidate{parent: n, childIndex: i}, rewritten) {
							child = rewritten
							index.call.nodesRewritten++
						}
					}
				}
				walk(child)
				if hasUnarySym && child.symbol == unarySym {
					index.unaryCandidates = append(index.unaryCandidates, javaScriptTypeScriptPrecedenceCandidate{
						parent:     n,
						childIndex: i,
					})
				} else if hasBinarySym && child.symbol == binarySym {
					index.binaryCandidates = append(index.binaryCandidates, javaScriptTypeScriptPrecedenceCandidate{
						parent:     n,
						childIndex: i,
					})
				}
			}
			return
		}

		children := n.children
		if hasCallSym {
			for i, child := range children {
				if child == nil || child.symbol != callSym {
					continue
				}
				index.call.nodesVisited++
				rewritten := rewriteJavaScriptTypeScriptCallPrecedenceWithSymbol(child, lang, callSym)
				if rewritten == nil {
					continue
				}
				children[i] = rewritten
				setNodeParentLink(rewritten, n, i)
				index.call.nodesRewritten++
			}
		}
		for _, child := range children {
			walk(child)
		}
		for i, child := range children {
			if child == nil {
				continue
			}
			if hasUnarySym && child.symbol == unarySym {
				index.unaryCandidates = append(index.unaryCandidates, javaScriptTypeScriptPrecedenceCandidate{
					parent:     n,
					childIndex: i,
				})
			} else if hasBinarySym && child.symbol == binarySym {
				index.binaryCandidates = append(index.binaryCandidates, javaScriptTypeScriptPrecedenceCandidate{
					parent:     n,
					childIndex: i,
				})
			}
		}
	}
	walk(root)
	return index
}

func normalizeJavaScriptTypeScriptEmptyStatementLeafWithSymbolChanged(node *Node, source []byte, semicolonSym Symbol, semicolonNamed bool) bool {
	if node == nil || resultChildCount(node) != 0 || len(source) == 0 {
		return false
	}
	if node.startByte > node.endByte || int(node.endByte) > len(source) {
		return false
	}
	if node.endByte-node.startByte != 1 || source[node.startByte] != ';' {
		return false
	}
	leaf := newLeafNodeInArena(node.ownerArena, semicolonSym, semicolonNamed, node.startByte, node.endByte, node.startPoint, node.endPoint)
	replaceNodeChildrenUnfielded(node, cloneNodeSliceInArena(node.ownerArena, []*Node{leaf}))
	return true
}

func normalizeJavaScriptTypeScriptCollapsedLeafWithSymbolChanged(node *Node, childSym Symbol, childNamed bool) bool {
	if node == nil || resultChildCount(node) != 0 {
		return false
	}
	child := newLeafNodeInArena(node.ownerArena, childSym, childNamed, node.startByte, node.endByte, node.startPoint, node.endPoint)
	replaceNodeChildrenUnfielded(node, cloneNodeSliceInArena(node.ownerArena, []*Node{child}))
	return true
}

func rewriteJavaScriptTypeScriptCallPrecedenceAndBuildUnaryBinaryIndex(root *Node, lang *Language, callSym, unarySym, binarySym Symbol) javaScriptTypeScriptUnaryBinaryPrecedenceIndex {
	var index javaScriptTypeScriptUnaryBinaryPrecedenceIndex
	if root == nil {
		return index
	}
	index.builds = 1
	var walk func(*Node)
	walk = func(n *Node) {
		if n == nil {
			return
		}
		index.nodesVisited++
		if nodeHasFinalChildRefs(n) {
			childCount := resultChildCount(n)
			for i := 0; i < childCount; i++ {
				child := resultChildAt(n, i)
				if child == nil {
					continue
				}
				if child.symbol == callSym {
					index.call.nodesVisited++
					if rewritten := rewriteJavaScriptTypeScriptCallPrecedenceWithSymbol(child, lang, callSym); rewritten != nil {
						if replaceJavaScriptTypeScriptPrecedenceCandidate(javaScriptTypeScriptPrecedenceCandidate{parent: n, childIndex: i}, rewritten) {
							child = rewritten
							index.call.nodesRewritten++
						}
					}
				}
				walk(child)
				switch child.symbol {
				case unarySym:
					index.unaryCandidates = append(index.unaryCandidates, javaScriptTypeScriptPrecedenceCandidate{
						parent:     n,
						childIndex: i,
					})
				case binarySym:
					index.binaryCandidates = append(index.binaryCandidates, javaScriptTypeScriptPrecedenceCandidate{
						parent:     n,
						childIndex: i,
					})
				}
			}
			return
		}

		children := n.children
		for i, child := range children {
			if child == nil || child.symbol != callSym {
				continue
			}
			index.call.nodesVisited++
			rewritten := rewriteJavaScriptTypeScriptCallPrecedenceWithSymbol(child, lang, callSym)
			if rewritten == nil {
				continue
			}
			children[i] = rewritten
			setNodeParentLink(rewritten, n, i)
			index.call.nodesRewritten++
		}
		for _, child := range children {
			walk(child)
		}
		for i, child := range children {
			if child == nil {
				continue
			}
			switch child.symbol {
			case unarySym:
				index.unaryCandidates = append(index.unaryCandidates, javaScriptTypeScriptPrecedenceCandidate{
					parent:     n,
					childIndex: i,
				})
			case binarySym:
				index.binaryCandidates = append(index.binaryCandidates, javaScriptTypeScriptPrecedenceCandidate{
					parent:     n,
					childIndex: i,
				})
			}
		}
	}
	walk(root)
	return index
}

type javaScriptTypeScriptCallPrecedenceIndex struct {
	candidates   []javaScriptTypeScriptPrecedenceCandidate
	builds       uint64
	nodesVisited uint64
}

type javaScriptTypeScriptPrecedenceCandidate struct {
	parent     *Node
	childIndex int
}

func buildJavaScriptTypeScriptCallPrecedenceIndex(root *Node, callSym Symbol) javaScriptTypeScriptCallPrecedenceIndex {
	var index javaScriptTypeScriptCallPrecedenceIndex
	if root == nil {
		return index
	}
	index.builds = 1
	walkResultTreeDenseFirst(root, func(n *Node) {
		index.nodesVisited++
		if n == nil {
			return
		}
		for i, child := range n.children {
			if child == nil || child.symbol != callSym {
				continue
			}
			index.candidates = append(index.candidates, javaScriptTypeScriptPrecedenceCandidate{
				parent:     n,
				childIndex: i,
			})
		}
	})
	return index
}

func rewriteJavaScriptTypeScriptCallPrecedenceCandidates(candidates []javaScriptTypeScriptPrecedenceCandidate, lang *Language, callSym Symbol) normalizationPassCounters {
	var counters normalizationPassCounters
	if len(candidates) == 0 || lang == nil {
		return counters
	}
	for _, candidate := range candidates {
		counters.nodesVisited++
		node := javaScriptTypeScriptPrecedenceCandidateNode(candidate)
		rewritten := rewriteJavaScriptTypeScriptCallPrecedenceWithSymbol(node, lang, callSym)
		if rewritten == nil {
			continue
		}
		if replaceJavaScriptTypeScriptPrecedenceCandidate(candidate, rewritten) {
			counters.nodesRewritten++
		}
	}
	return counters
}

func javaScriptTypeScriptPrecedenceCandidateNode(candidate javaScriptTypeScriptPrecedenceCandidate) *Node {
	if candidate.parent == nil || candidate.childIndex < 0 {
		return nil
	}
	if candidate.childIndex >= resultChildCount(candidate.parent) {
		return nil
	}
	return resultChildAt(candidate.parent, candidate.childIndex)
}

func replaceJavaScriptTypeScriptPrecedenceCandidate(candidate javaScriptTypeScriptPrecedenceCandidate, replacement *Node) bool {
	if candidate.parent == nil || candidate.childIndex < 0 || replacement == nil {
		return false
	}
	view := resultMutableChildrenForMutation(candidate.parent)
	if view.hasFinalChildRefs() {
		if candidate.childIndex >= view.Len() {
			return false
		}
		return view.ReplaceFinalRefRangeWithNode(candidate.childIndex, candidate.childIndex+1, replacement)
	}
	if candidate.childIndex >= len(candidate.parent.children) {
		return false
	}
	candidate.parent.children[candidate.childIndex] = replacement
	setNodeParentLink(replacement, candidate.parent, candidate.childIndex)
	return true
}

func rewriteJavaScriptTypeScriptCallPrecedence(node *Node, lang *Language) *Node {
	callSym, ok := symbolByName(lang, "call_expression")
	if !ok {
		return nil
	}
	return rewriteJavaScriptTypeScriptCallPrecedenceWithSymbol(node, lang, callSym)
}

func rewriteJavaScriptTypeScriptCallPrecedenceWithSymbol(node *Node, lang *Language, callSym Symbol) *Node {
	if node == nil || lang == nil || node.symbol != callSym || len(node.children) != 2 {
		return nil
	}
	function := node.children[0]
	arguments := node.children[1]
	if function == nil || arguments == nil {
		return nil
	}
	return rewriteJavaScriptTypeScriptCallTarget(function, arguments, node, lang)
}

func rewriteJavaScriptTypeScriptCallTarget(target, arguments, callNode *Node, lang *Language) *Node {
	if target == nil || arguments == nil || callNode == nil || lang == nil {
		return nil
	}
	if isJavaScriptTypeScriptCallableShape(target, lang) {
		if len(callNode.children) == 2 && target == callNode.children[0] && arguments == callNode.children[1] {
			return nil
		}
		rewrittenCall := cloneNodeInArena(callNode.ownerArena, callNode)
		rewrittenCall.children = cloneNodeSliceInArena(callNode.ownerArena, []*Node{target, arguments})
		populateParentNode(rewrittenCall, rewrittenCall.children)
		return rewrittenCall
	}

	switch target.Type(lang) {
	case "unary_expression":
		if len(target.children) < 2 {
			return nil
		}
		operandIdx := len(target.children) - 1
		rewrittenOperand := rewriteJavaScriptTypeScriptCallTarget(target.children[operandIdx], arguments, callNode, lang)
		if rewrittenOperand == nil {
			return nil
		}
		rewrittenUnary := cloneNodeInArena(callNode.ownerArena, target)
		unaryChildren := cloneNodeSliceInArena(callNode.ownerArena, target.children)
		unaryChildren[operandIdx] = rewrittenOperand
		rewrittenUnary.children = unaryChildren
		populateParentNode(rewrittenUnary, rewrittenUnary.children)
		return rewrittenUnary
	case "binary_expression":
		operator, rightIdx, ok := javaScriptTypeScriptBinaryOperatorAndRight(target, lang)
		if !ok || rightIdx < 0 || rightIdx >= len(target.children) {
			return nil
		}
		if operator == nil {
			return nil
		}
		if _, ok := javaScriptTypeScriptBinaryOperatorPrecedence(operator.Type(lang)); !ok {
			return nil
		}
		rewrittenRight := rewriteJavaScriptTypeScriptCallTarget(target.children[rightIdx], arguments, callNode, lang)
		if rewrittenRight == nil {
			return nil
		}
		rewrittenBinary := cloneNodeInArena(callNode.ownerArena, target)
		binaryChildren := cloneNodeSliceInArena(callNode.ownerArena, target.children)
		binaryChildren[rightIdx] = rewrittenRight
		rewrittenBinary.children = binaryChildren
		populateParentNode(rewrittenBinary, rewrittenBinary.children)
		return rewrittenBinary
	default:
		return nil
	}
}

func javaScriptTypeScriptBinaryOperatorAndRight(node *Node, lang *Language) (*Node, int, bool) {
	if node == nil || lang == nil || node.Type(lang) != "binary_expression" || len(node.children) < 3 {
		return nil, -1, false
	}
	operatorIdx := -1
	rightIdx := -1
	for i := 0; i < len(node.children); i++ {
		switch node.FieldNameForChild(i, lang) {
		case "operator":
			operatorIdx = i
		case "right":
			rightIdx = i
		}
	}
	if operatorIdx < 0 && len(node.children) >= 2 {
		operatorIdx = 1
	}
	if rightIdx < 0 {
		for i := len(node.children) - 1; i >= 0; i-- {
			child := node.children[i]
			if child == nil || child.isExtra() {
				continue
			}
			if i != operatorIdx {
				rightIdx = i
				break
			}
		}
	}
	if operatorIdx < 0 || rightIdx < 0 || operatorIdx >= len(node.children) {
		return nil, -1, false
	}
	return node.children[operatorIdx], rightIdx, true
}

func isJavaScriptTypeScriptCallableShape(node *Node, lang *Language) bool {
	if node == nil || lang == nil {
		return false
	}
	switch node.Type(lang) {
	case "identifier", "member_expression", "subscript_expression", "call_expression", "parenthesized_expression":
		return true
	default:
		return false
	}
}

func normalizeJavaScriptTypeScriptUnaryPrecedence(root *Node, lang *Language) {
	if root == nil || lang == nil {
		return
	}
	switch lang.Name {
	case "javascript", "typescript", "tsx":
	default:
		return
	}
	unarySym, ok := symbolByName(lang, "unary_expression")
	if !ok {
		return
	}

	rewriteResultTreeChildrenPostorder(root, func(n *Node) *Node {
		return rewriteJavaScriptTypeScriptUnaryPrecedenceWithSymbol(n, lang, unarySym)
	})
}

func normalizeJavaScriptTypeScriptUnaryPrecedenceWithStats(root *Node, lang *Language) normalizationPassCounters {
	var counters normalizationPassCounters
	if root == nil || lang == nil {
		return counters
	}
	switch lang.Name {
	case "javascript", "typescript", "tsx":
	default:
		return counters
	}
	unarySym, ok := symbolByName(lang, "unary_expression")
	if !ok {
		return counters
	}

	return rewriteResultTreeChildrenPostorderWithStats(root, func(n *Node) *Node {
		return rewriteJavaScriptTypeScriptUnaryPrecedenceWithSymbol(n, lang, unarySym)
	})
}

func normalizeJavaScriptTypeScriptUnaryBinaryPrecedence(root *Node, lang *Language) {
	_ = normalizeJavaScriptTypeScriptUnaryBinaryPrecedenceWithDetailedStats(root, lang)
}

type javaScriptTypeScriptUnaryBinaryPrecedenceStats struct {
	unary             normalizationPassCounters
	binary            normalizationPassCounters
	indexBuilds       uint64
	indexNodesVisited uint64
}

func normalizeJavaScriptTypeScriptUnaryBinaryPrecedenceWithDetailedStats(root *Node, lang *Language) javaScriptTypeScriptUnaryBinaryPrecedenceStats {
	var stats javaScriptTypeScriptUnaryBinaryPrecedenceStats
	if root == nil || lang == nil {
		return stats
	}
	switch lang.Name {
	case "javascript", "typescript", "tsx":
	default:
		return stats
	}
	unarySym, ok := symbolByName(lang, "unary_expression")
	if !ok {
		return stats
	}
	binarySym, ok := symbolByName(lang, "binary_expression")
	if !ok {
		return stats
	}

	index := buildJavaScriptTypeScriptUnaryBinaryPrecedenceIndex(root, unarySym, binarySym)
	stats.indexBuilds += index.builds
	stats.indexNodesVisited += index.nodesVisited
	stats.unary = rewriteJavaScriptTypeScriptPrecedenceCandidates(index.unaryCandidates, func(n *Node) *Node {
		return rewriteJavaScriptTypeScriptUnaryPrecedenceWithSymbol(n, lang, unarySym)
	})
	if stats.unary.nodesRewritten != 0 {
		index = buildJavaScriptTypeScriptUnaryBinaryPrecedenceIndex(root, unarySym, binarySym)
		stats.indexBuilds += index.builds
		stats.indexNodesVisited += index.nodesVisited
	}
	stats.binary = rewriteJavaScriptTypeScriptPrecedenceCandidates(index.binaryCandidates, func(n *Node) *Node {
		return rewriteJavaScriptTypeScriptBinaryPrecedenceWithSymbol(n, lang, binarySym)
	})
	return stats
}

type javaScriptTypeScriptUnaryBinaryPrecedenceIndex struct {
	emptyStatement   normalizationPassCounters
	existentialType  normalizationPassCounters
	statementKeyword normalizationPassCounters
	call             normalizationPassCounters
	unaryCandidates  []javaScriptTypeScriptPrecedenceCandidate
	binaryCandidates []javaScriptTypeScriptPrecedenceCandidate
	builds           uint64
	nodesVisited     uint64
}

func buildJavaScriptTypeScriptUnaryBinaryPrecedenceIndex(root *Node, unarySym, binarySym Symbol) javaScriptTypeScriptUnaryBinaryPrecedenceIndex {
	var index javaScriptTypeScriptUnaryBinaryPrecedenceIndex
	if root == nil {
		return index
	}
	index.builds = 1
	walkResultTreePostorder(root, func(n *Node) {
		index.nodesVisited++
		if n == nil {
			return
		}
		children := n.children
		if n.ownerArena != nil && n.childIndex <= finalChildSidecarIndexBase {
			children = resultDenseChildrenFallbackForMutation(n)
		}
		for i, child := range children {
			if child == nil {
				continue
			}
			switch child.symbol {
			case unarySym:
				index.unaryCandidates = append(index.unaryCandidates, javaScriptTypeScriptPrecedenceCandidate{
					parent:     n,
					childIndex: i,
				})
			case binarySym:
				index.binaryCandidates = append(index.binaryCandidates, javaScriptTypeScriptPrecedenceCandidate{
					parent:     n,
					childIndex: i,
				})
			}
		}
	})
	return index
}

func rewriteJavaScriptTypeScriptPrecedenceCandidates(candidates []javaScriptTypeScriptPrecedenceCandidate, rewrite func(*Node) *Node) normalizationPassCounters {
	var counters normalizationPassCounters
	if len(candidates) == 0 || rewrite == nil {
		return counters
	}
	for _, candidate := range candidates {
		counters.nodesVisited++
		for {
			node := javaScriptTypeScriptPrecedenceCandidateNode(candidate)
			rewritten := rewrite(node)
			if rewritten == nil {
				break
			}
			if !replaceJavaScriptTypeScriptPrecedenceCandidate(candidate, rewritten) {
				break
			}
			counters.nodesRewritten++
		}
	}
	return counters
}

func rewriteJavaScriptTypeScriptUnaryPrecedence(node *Node, lang *Language) *Node {
	unarySym, ok := symbolByName(lang, "unary_expression")
	if !ok {
		return nil
	}
	return rewriteJavaScriptTypeScriptUnaryPrecedenceWithSymbol(node, lang, unarySym)
}

func rewriteJavaScriptTypeScriptUnaryPrecedenceWithSymbol(node *Node, lang *Language, unarySym Symbol) *Node {
	if node == nil || lang == nil || node.symbol != unarySym || len(node.children) < 2 {
		return nil
	}
	operandIdx := len(node.children) - 1
	operand := node.children[operandIdx]
	if operand == nil || operand.Type(lang) != "binary_expression" || len(operand.children) != 3 {
		return nil
	}
	if _, ok := javaScriptTypeScriptBinaryOperatorPrecedence(operand.children[1].Type(lang)); !ok {
		return nil
	}

	rewrittenUnary := cloneNodeInArena(node.ownerArena, node)
	unaryChildren := cloneNodeSliceInArena(node.ownerArena, node.children)
	unaryChildren[operandIdx] = operand.children[0]
	rewrittenUnary.children = unaryChildren
	populateParentNode(rewrittenUnary, rewrittenUnary.children)

	rewrittenBinary := cloneNodeInArena(node.ownerArena, operand)
	binaryChildren := cloneNodeSliceInArena(node.ownerArena, operand.children)
	binaryChildren[0] = rewrittenUnary
	rewrittenBinary.children = binaryChildren
	populateParentNode(rewrittenBinary, rewrittenBinary.children)
	return rewrittenBinary
}

func normalizeJavaScriptTypeScriptBinaryPrecedence(root *Node, lang *Language) {
	if root == nil || lang == nil {
		return
	}
	switch lang.Name {
	case "javascript", "typescript", "tsx":
	default:
		return
	}
	binarySym, ok := symbolByName(lang, "binary_expression")
	if !ok {
		return
	}

	rewriteResultTreeChildrenPostorder(root, func(n *Node) *Node {
		return rewriteJavaScriptTypeScriptBinaryPrecedenceWithSymbol(n, lang, binarySym)
	})
}

func normalizeJavaScriptTypeScriptBinaryPrecedenceWithStats(root *Node, lang *Language) normalizationPassCounters {
	var counters normalizationPassCounters
	if root == nil || lang == nil {
		return counters
	}
	switch lang.Name {
	case "javascript", "typescript", "tsx":
	default:
		return counters
	}
	binarySym, ok := symbolByName(lang, "binary_expression")
	if !ok {
		return counters
	}

	return rewriteResultTreeChildrenPostorderWithStats(root, func(n *Node) *Node {
		return rewriteJavaScriptTypeScriptBinaryPrecedenceWithSymbol(n, lang, binarySym)
	})
}

func rewriteJavaScriptTypeScriptBinaryPrecedence(node *Node, lang *Language) *Node {
	binarySym, ok := symbolByName(lang, "binary_expression")
	if !ok {
		return nil
	}
	return rewriteJavaScriptTypeScriptBinaryPrecedenceWithSymbol(node, lang, binarySym)
}

func rewriteJavaScriptTypeScriptBinaryPrecedenceWithSymbol(node *Node, lang *Language, binarySym Symbol) *Node {
	if node == nil || lang == nil || node.symbol != binarySym || len(node.children) != 3 {
		return nil
	}
	left := node.children[0]
	op := node.children[1]
	right := node.children[2]
	if left == nil || op == nil || right == nil {
		return nil
	}
	parentPrec, ok := javaScriptTypeScriptBinaryOperatorPrecedence(op.Type(lang))
	if !ok {
		return nil
	}

	if left.Type(lang) == "binary_expression" && len(left.children) == 3 {
		leftOp := left.children[1]
		if leftOp != nil {
			leftPrec, ok := javaScriptTypeScriptBinaryOperatorPrecedence(leftOp.Type(lang))
			if ok && parentPrec > leftPrec {
				rotatedInner := cloneNodeInArena(node.ownerArena, node)
				rotatedInner.children = cloneNodeSliceInArena(node.ownerArena, []*Node{left.children[2], op, right})
				populateParentNode(rotatedInner, rotatedInner.children)

				rotatedOuter := cloneNodeInArena(node.ownerArena, left)
				rotatedOuter.children = cloneNodeSliceInArena(node.ownerArena, []*Node{left.children[0], leftOp, rotatedInner})
				populateParentNode(rotatedOuter, rotatedOuter.children)
				return rotatedOuter
			}
		}
	}

	if right.Type(lang) == "binary_expression" && len(right.children) == 3 {
		rightOp := right.children[1]
		if rightOp != nil {
			rightPrec, ok := javaScriptTypeScriptBinaryOperatorPrecedence(rightOp.Type(lang))
			if ok && parentPrec >= rightPrec && !javaScriptTypeScriptBinaryOperatorRightAssociative(op.Type(lang)) {
				rotatedInner := cloneNodeInArena(node.ownerArena, node)
				rotatedInner.children = cloneNodeSliceInArena(node.ownerArena, []*Node{left, op, right.children[0]})
				populateParentNode(rotatedInner, rotatedInner.children)

				rotatedOuter := cloneNodeInArena(node.ownerArena, right)
				rotatedOuter.children = cloneNodeSliceInArena(node.ownerArena, []*Node{rotatedInner, rightOp, right.children[2]})
				populateParentNode(rotatedOuter, rotatedOuter.children)
				return rotatedOuter
			}
		}
	}

	return nil
}

func javaScriptTypeScriptBinaryOperatorPrecedence(op string) (int, bool) {
	switch op {
	case "??":
		return 1, true
	case "||":
		return 2, true
	case "&&":
		return 3, true
	case "|":
		return 4, true
	case "^":
		return 5, true
	case "&":
		return 6, true
	case "==", "!=", "===", "!==":
		return 7, true
	case "<", "<=", ">", ">=", "instanceof", "in":
		return 8, true
	case "<<", ">>", ">>>":
		return 9, true
	case "+", "-":
		return 10, true
	case "*", "/", "%":
		return 11, true
	case "**":
		return 12, true
	default:
		return 0, false
	}
}

func javaScriptTypeScriptBinaryOperatorRightAssociative(op string) bool {
	return op == "**"
}

func rewriteJavaScriptTopLevelObjectLiteral(node *Node, lang *Language, arena *nodeArena, exprSym Symbol, exprNamed bool, objectSym Symbol, objectNamed bool, pairSym Symbol, pairNamed bool, propSym Symbol) (*Node, bool) {
	if node == nil || lang == nil || node.Type(lang) != "statement_block" || len(node.children) != 3 {
		return nil, false
	}
	if node.children[0] == nil || node.children[0].Type(lang) != "{" || node.children[2] == nil || node.children[2].Type(lang) != "}" {
		return nil, false
	}
	label := node.children[1]
	if label == nil || label.Type(lang) != "labeled_statement" || len(label.children) != 3 {
		return nil, false
	}
	key := label.children[0]
	colon := label.children[1]
	valueStmt := label.children[2]
	if key == nil || key.Type(lang) != "statement_identifier" || colon == nil || colon.Type(lang) != ":" || valueStmt == nil || valueStmt.Type(lang) != "expression_statement" || len(valueStmt.children) != 1 || valueStmt.children[0] == nil {
		return nil, false
	}
	pair := newParentNodeInArena(arena, pairSym, pairNamed, []*Node{
		aliasedNodeInArena(arena, lang, key, propSym),
		colon,
		valueStmt.children[0],
	}, nil, 0)
	for fieldIdx, fieldName := range lang.FieldNames {
		switch fieldName {
		case "key":
			ensureNodeFieldStorage(pair, len(pair.children))
			pair.fieldIDs[0] = FieldID(fieldIdx)
			pair.fieldSources[0] = fieldSourceDirect
		case "value":
			ensureNodeFieldStorage(pair, len(pair.children))
			pair.fieldIDs[2] = FieldID(fieldIdx)
			pair.fieldSources[2] = fieldSourceDirect
		}
	}
	object := newParentNodeInArena(arena, objectSym, objectNamed, []*Node{
		node.children[0],
		pair,
		node.children[2],
	}, nil, 0)
	return newParentNodeInArena(arena, exprSym, exprNamed, []*Node{object}, nil, 0), true
}
