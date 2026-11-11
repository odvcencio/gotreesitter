package gotreesitter

import "strings"

func normalizeCCompatibilityWithParser(root *Node, source []byte, p *Parser, lang *Language) {
	recordPasses := p != nil && p.currentMaterializationTiming() != nil
	if recordPasses {
		run := func(name string, fn func()) {
			p.runNamedNormalizationPass(name, func() bool { return true }, func() normalizationPassCounters {
				fn()
				return normalizationPassCounters{}
			})
		}
		run("c_translation_unit_root", func() {
			normalizeCTranslationUnitRoot(root, lang)
		})
		run("c_recovered_top_level_chunks", func() {
			normalizeCRecoveredTopLevelChunks(root, source, p, lang)
		})
		run("cpp_malformed_class_function_definition", func() {
			normalizeCppMalformedClassFunctionDefinition(root, source, lang)
		})
		// Fused walk covers two preorder passes (builtin primitive identifiers +
		// preproc newline spans). It formerly also carried declaration-bounds
		// extension and variadic-ellipsis materialization, both cut as dead
		// (wave12/ccpp-dead-cuts census: 0 fires over the full c/cpp corpora,
		// clean and truncated). PreprocNewlineSpans only extends `\n` token
		// endByte, which downstream passes don't read — safe to move earlier
		// in the chain.
		run("c_builtin_primitive_and_newline_span", func() {
			normalizeCFusedDeclVariadicWalk(root, source, lang)
		})
		run("c_sizeof_unknown_type_identifiers", func() {
			normalizeCSizeofUnknownTypeIdentifiers(root, source, lang)
		})
		run("c_cast_unknown_type_identifiers", func() {
			normalizeCCastUnknownTypeIdentifiers(root, source, lang)
		})
		run("c_bare_type_identifier_expression_statements", func() {
			normalizeCBareTypeIdentifierExpressionStatements(root, source, lang)
		})
		return
	}
	normalizeCTranslationUnitRoot(root, lang)
	normalizeCRecoveredTopLevelChunks(root, source, p, lang)
	normalizeCppMalformedClassFunctionDefinition(root, source, lang)
	normalizeCFusedDeclVariadicWalk(root, source, lang)
	normalizeCSizeofUnknownTypeIdentifiers(root, source, lang)
	normalizeCCastUnknownTypeIdentifiers(root, source, lang)
	normalizeCBareTypeIdentifierExpressionStatements(root, source, lang)
}

// normalizeCFusedDeclVariadicWalk performs the work of two preorder walks in
// a single pass: builtin primitive type identifier promotion and
// preprocessor newline-span extension. (It formerly also carried
// declaration-bounds extension and variadic-parameter ellipsis
// materialization; both were removed as dead — wave12/ccpp-dead-cuts census
// found 0 fires over the full c/cpp corpora, clean and truncated-error
// variants — leaving the walk's name a historical artifact of the merge.)
// The two remaining handlers gate on disjoint node symbols (builtin) or
// operate on the child layer (newline spans), so a single visit can apply
// both without ordering concerns.
func normalizeCFusedDeclVariadicWalk(root *Node, source []byte, lang *Language) {
	if root == nil || lang == nil {
		return
	}
	isC := lang.Name == "c"
	isCpp := lang.Name == "cpp"
	if !isC && !isCpp {
		return
	}
	srcLen := uint32(len(source))

	// builtin primitive promotion is c only.
	var primitiveTypeSym Symbol
	var typeIdentifierSym Symbol
	var primitiveTypeNamed bool
	hasBuiltin := false
	if isC {
		ps, ok := lang.SymbolByName("primitive_type")
		if ok {
			ts, ok2 := lang.SymbolByName("type_identifier")
			if ok2 {
				primitiveTypeSym = ps
				typeIdentifierSym = ts
				primitiveTypeNamed = symbolIsNamed(lang, primitiveTypeSym)
				hasBuiltin = true
			}
		}
	}

	// preproc newline spans applies to c and cpp.
	nlSym, hasNl := symbolByName(lang, "\n")
	hasNewlineSpan := hasNl && srcLen > 0

	if !hasBuiltin && !hasNewlineSpan {
		return
	}

	walkResultTree(root, func(n *Node) {
		if hasBuiltin && n.symbol == typeIdentifierSym {
			if isCBuiltinPrimitiveTypeName(canonicalCTypeName(n.Text(source))) {
				n.symbol = primitiveTypeSym
				n.setNamed(primitiveTypeNamed)
			}
		}
		if hasNewlineSpan {
			for _, child := range n.children {
				if child != nil && child.symbol == nlSym && child.endByte < srcLen {
					end := child.endByte
					for end < srcLen && (source[end] == '\n' || source[end] == '\r') {
						end++
					}
					if end > child.endByte {
						child.endByte = end
						child.endPoint = advancePointByBytes(Point{}, source[:end])
					}
				}
			}
		}
	})
}

func normalizeCRecoveredTopLevelChunks(root *Node, source []byte, p *Parser, lang *Language) {
	if root == nil || p == nil || lang == nil || p.skipRecoveryReparse || root.ownerArena == nil || len(source) == 0 {
		return
	}
	if lang.Name != "c" || root.Type(lang) != "ERROR" {
		return
	}
	spans := cTopLevelChunkSpans(source)
	if len(spans) == 0 {
		return
	}
	children := make([]*Node, 0, len(spans))
	for _, span := range spans {
		for _, part := range cSplitLeadingTopLevelCommentSpans(source, span[0], span[1]) {
			nodes, ok := cRecoverTopLevelChunkNodesFromRange(source, part[0], part[1], p, root.ownerArena)
			if !ok || len(nodes) == 0 {
				return
			}
			children = append(children, nodes...)
		}
	}
	if len(children) == 0 {
		return
	}
	sym, ok := symbolByName(lang, "translation_unit")
	if !ok {
		return
	}
	retagResultRoot(root, sym, symbolIsNamed(lang, sym))
	replaceNodeChildrenUnfielded(root, cloneNodeSliceIfArena(root.ownerArena, children))
	root.productionID = 0
	root.setHasError(false)
	for _, child := range root.children {
		if child != nil && child.HasError() {
			root.setHasError(true)
			break
		}
	}
	extendNodeToTrailingWhitespace(root, source)
}

func cRecoverTopLevelChunkNodesFromRange(source []byte, start, end uint32, p *Parser, arena *nodeArena) ([]*Node, bool) {
	start, end = rustTrimSpaceBounds(source, start, end)
	if p == nil || p.language == nil || start >= end || int(end) > len(source) {
		return nil, false
	}
	if node, ok := cRecoverCommentNodeFromRange(source, start, end, p.language, arena); ok {
		return []*Node{node}, true
	}
	tree, err := p.parseForRecovery(source[start:end])
	if err != nil || tree == nil || tree.RootNode() == nil {
		if tree != nil {
			tree.Release()
		}
		return nil, false
	}
	defer tree.Release()

	parsedRoot := tree.RootNode()
	if parsedRoot == nil {
		return nil, false
	}
	if parsedRoot.Type(p.language) != "translation_unit" {
		errorRoot := parsedRoot
		if start != 0 {
			startPoint := advancePointByBytes(Point{}, source[:start])
			errorRoot = tree.rootNodeWithRecoveryOffset(start, startPoint)
		}
		if errorRoot != nil && errorRoot.Type(p.language) == "ERROR" {
			if node, ok := cRecoverTypedefStructDefinitionFromErrorRoot(errorRoot, source, start, end, p.language, arena); ok {
				return []*Node{node}, true
			}
		}
		return nil, false
	}
	childCount := parsedRoot.ChildCount()
	out := make([]*Node, 0, childCount)
	var offset *cloneOffset
	if start != 0 {
		offset = &cloneOffset{
			byteDelta: start,
			point:     advancePointByBytes(Point{}, source[:start]),
			baseRow:   parsedRoot.startPoint.Row,
		}
	}
	offsetRoot := parsedRoot
	if arena == nil && offset != nil {
		offsetRoot = tree.rootNodeWithRecoveryOffset(start, offset.point)
		if offsetRoot == nil {
			return nil, false
		}
	}
	for i := 0; i < childCount; i++ {
		child := parsedRoot.Child(i)
		if child == nil {
			continue
		}
		if arena != nil {
			out = append(out, cloneTreeNodesIntoArenaWithOffset(child, arena, offset))
		} else {
			if offset != nil {
				if i >= offsetRoot.ChildCount() {
					continue
				}
				child = offsetRoot.Child(i)
			}
			out = append(out, child)
		}
	}
	return out, len(out) > 0
}

func cRecoverCommentNodeFromRange(source []byte, start, end uint32, lang *Language, arena *nodeArena) (*Node, bool) {
	start, end = rustTrimSpaceBounds(source, start, end)
	if start >= end || int(end) > len(source) {
		return nil, false
	}
	switch {
	case start+1 < end && source[start] == '/' && source[start+1] == '/':
		commentEnd := start + 2
		for commentEnd < end && source[commentEnd] != '\n' {
			commentEnd++
		}
		if commentEnd != end {
			return nil, false
		}
	case start+1 < end && source[start] == '/' && source[start+1] == '*':
		if rustFindBlockCommentEnd(source, start+2, end) != end {
			return nil, false
		}
	default:
		return nil, false
	}
	commentSym, ok := symbolByName(lang, "comment")
	if !ok {
		return nil, false
	}
	node := cLeaf(arena, lang, commentSym, start, end, source)
	node.setExtra(true)
	return node, true
}

func cSplitLeadingTopLevelCommentSpans(source []byte, start, end uint32) [][2]uint32 {
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

func cRecoverTypedefStructDefinitionFromErrorRoot(root *Node, source []byte, start, end uint32, lang *Language, arena *nodeArena) (*Node, bool) {
	if root == nil || lang == nil || start >= end || int(end) > len(source) {
		return nil, false
	}
	start, end = rustTrimSpaceBounds(source, start, end)
	if !rustKeywordAt(source, start, end, "typedef") {
		return nil, false
	}
	var structSpec *Node
	for i := 0; i < root.ChildCount(); i++ {
		child := root.Child(i)
		if child != nil && child.Type(lang) == "struct_specifier" {
			structSpec = child
			break
		}
	}
	if structSpec == nil || structSpec.endByte >= end {
		return nil, false
	}
	semicolon := cFindByteAtTopLevel(source, structSpec.endByte, end, ';')
	if semicolon == 0 {
		return nil, false
	}

	typeDefSym, ok := symbolByName(lang, "type_definition")
	if !ok {
		return nil, false
	}
	typedefSym, ok := symbolByName(lang, "typedef")
	if !ok {
		return nil, false
	}
	commaSym, ok := symbolByName(lang, ",")
	if !ok {
		return nil, false
	}
	typeIdentSym, ok := symbolByName(lang, "type_identifier")
	if !ok {
		return nil, false
	}
	semiSym, ok := symbolByName(lang, ";")
	if !ok {
		return nil, false
	}

	children := make([]*Node, 0, 8)
	children = append(children, cLeaf(arena, lang, typedefSym, start, start+7, source))
	children = append(children, cloneTreeNodesIntoArena(structSpec, arena))

	cursor := rustSkipSpaceBytes(source, structSpec.endByte)
	if cursor >= semicolon || source[cursor] != ',' {
		return nil, false
	}
	errNode := newParentNodeInArena(
		arena,
		errorSymbol,
		true,
		[]*Node{cLeaf(arena, lang, commaSym, cursor, cursor+1, source)},
		nil,
		0,
	)
	errNode.startByte = cursor
	errNode.startPoint = advancePointByBytes(Point{}, source[:cursor])
	errNode.endByte = cursor + 1
	errNode.endPoint = advancePointByBytes(Point{}, source[:cursor+1])
	errNode.setExtra(true)
	errNode.setHasError(true)
	children = append(children, errNode)
	cursor = rustSkipSpaceBytes(source, cursor+1)

	for cursor < semicolon {
		nameStart := cursor
		for cursor < semicolon && rustIsIdentByte(source[cursor]) {
			cursor++
		}
		if nameStart == cursor {
			return nil, false
		}
		children = append(children, cLeaf(arena, lang, typeIdentSym, nameStart, cursor, source))
		cursor = rustSkipSpaceBytes(source, cursor)
		if cursor < semicolon {
			if source[cursor] != ',' {
				return nil, false
			}
			children = append(children, cLeaf(arena, lang, commaSym, cursor, cursor+1, source))
			cursor = rustSkipSpaceBytes(source, cursor+1)
		}
	}
	children = append(children, cLeaf(arena, lang, semiSym, semicolon, semicolon+1, source))

	typeDef := newParentNodeInArena(
		arena,
		typeDefSym,
		symbolIsNamed(lang, typeDefSym),
		children,
		cTypeDefinitionFieldIDs(arena, lang, children),
		0,
	)
	typeDef.startByte = start
	typeDef.startPoint = advancePointByBytes(Point{}, source[:start])
	typeDef.endByte = semicolon + 1
	typeDef.endPoint = advancePointByBytes(Point{}, source[:semicolon+1])
	typeDef.setHasError(true)
	return typeDef, true
}

func cTypeDefinitionFieldIDs(arena *nodeArena, lang *Language, children []*Node) []FieldID {
	typeFID, ok := lang.FieldByName("type")
	if !ok || len(children) < 2 {
		return nil
	}
	fieldIDs := make([]FieldID, len(children))
	fieldIDs[1] = typeFID
	if declaratorFID, ok := lang.FieldByName("declarator"); ok {
		for i, child := range children {
			if child != nil && child.Type(lang) == "type_identifier" {
				fieldIDs[i] = declaratorFID
			}
		}
	}
	return cloneFieldIDSliceInArena(arena, fieldIDs)
}

func cLeaf(arena *nodeArena, lang *Language, sym Symbol, start, end uint32, source []byte) *Node {
	return newLeafNodeInArena(
		arena,
		sym,
		symbolIsNamed(lang, sym),
		start,
		end,
		advancePointByBytes(Point{}, source[:start]),
		advancePointByBytes(Point{}, source[:end]),
	)
}

func cFindByteAtTopLevel(source []byte, start, end uint32, want byte) uint32 {
	parenDepth := 0
	bracketDepth := 0
	inString := byte(0)
	escaped := false
	for i := start; i < end; i++ {
		b := source[i]
		if inString != 0 {
			if escaped {
				escaped = false
				continue
			}
			if b == '\\' {
				escaped = true
				continue
			}
			if b == inString {
				inString = 0
			}
			continue
		}
		if next, ok := rustSkipCommentAt(source, i, end); ok {
			i = next - 1
			continue
		}
		switch b {
		case '"', '\'':
			inString = b
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
		default:
			if b == want && parenDepth == 0 && bracketDepth == 0 {
				return i
			}
		}
	}
	return 0
}

func cTopLevelChunkSpans(source []byte) [][2]uint32 {
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
	inString := byte(0)
	escaped := false
	for i := start; i < uint32(len(source)); i++ {
		b := source[i]
		if inString != 0 {
			if escaped {
				escaped = false
				continue
			}
			if b == '\\' {
				escaped = true
				continue
			}
			if b == inString {
				inString = 0
			}
			continue
		}
		if next, ok := rustSkipCommentAt(source, i, uint32(len(source))); ok {
			i = next - 1
			continue
		}
		switch b {
		case '"', '\'':
			inString = b
		case '{':
			braceDepth++
		case '}':
			if braceDepth > 0 {
				braceDepth--
				if braceDepth == 0 && parenDepth == 0 && bracketDepth == 0 {
					next := rustSkipSpaceBytes(source, i+1)
					if next >= uint32(len(source)) || source[next] != ';' && source[next] != ',' {
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

func normalizeCTranslationUnitRoot(root *Node, lang *Language) {
	if root == nil || lang == nil || root.Type(lang) != "ERROR" {
		return
	}
	if lang.Name != "c" && lang.Name != "cpp" {
		return
	}
	sym, ok := symbolByName(lang, "translation_unit")
	if !ok || !rootLooksLikeCTopLevel(root, lang) {
		return
	}
	retagResultRoot(root, sym, symbolIsNamed(lang, sym))
}

func rootLooksLikeCTopLevel(root *Node, lang *Language) bool {
	if root == nil || lang == nil || len(root.children) == 0 {
		return false
	}
	sawTopLevel := false
	for _, child := range root.children {
		if child == nil {
			continue
		}
		switch child.Type(lang) {
		case "preproc_if",
			"preproc_ifdef",
			"preproc_include",
			"preproc_def",
			"preproc_function_def",
			"preproc_call",
			"declaration",
			"function_definition",
			"linkage_specification",
			"type_definition",
			"struct_specifier",
			"union_specifier",
			"enum_specifier",
			"class_specifier",
			"namespace_definition",
			"template_declaration",
			"comment":
			sawTopLevel = true
		default:
			return false
		}
	}
	return sawTopLevel
}

func normalizeCSizeofUnknownTypeIdentifiers(root *Node, source []byte, lang *Language) {
	if root == nil || lang == nil || lang.Name != "c" {
		return
	}
	typeDescriptorSym, ok := lang.SymbolByName("type_descriptor")
	if !ok {
		return
	}
	typeIdentifierSym, ok := lang.SymbolByName("type_identifier")
	if !ok {
		return
	}
	identifierSym, ok := lang.SymbolByName("identifier")
	if !ok {
		return
	}
	parenthesizedSym, ok := lang.SymbolByName("parenthesized_expression")
	if !ok {
		return
	}
	identifierNamed := symbolIsNamed(lang, identifierSym)
	parenthesizedNamed := symbolIsNamed(lang, parenthesizedSym)
	valueFieldID, hasValueField := lang.FieldByName("value")
	sizeofSym, ok := lang.SymbolByName("sizeof_expression")
	if !ok {
		return
	}
	localTypes := make(map[string]struct{})
	var sizeofNodes []*Node
	collectCLocalTypeNamesAndCandidates(root, source, lang, localTypes, func(n *Node) {
		if cResultSymbolMatches(lang, n, sizeofSym) {
			sizeofNodes = append(sizeofNodes, n)
		}
	})

	for _, n := range sizeofNodes {
		if len(n.children) == 4 {
			typeDescriptor := n.children[2]
			if typeDescriptor != nil && typeDescriptor.symbol == typeDescriptorSym && len(typeDescriptor.children) == 1 {
				typeIdent := typeDescriptor.children[0]
				if typeIdent != nil && typeIdent.symbol == typeIdentifierSym {
					name := canonicalCTypeName(typeIdent.Text(source))
					if _, ok := localTypes[name]; !ok {
						ident := newLeafNodeInArena(n.ownerArena, identifierSym, identifierNamed, typeIdent.startByte, typeIdent.endByte, typeIdent.startPoint, typeIdent.endPoint)
						paren := newParentNodeInArena(n.ownerArena, parenthesizedSym, parenthesizedNamed, []*Node{n.children[1], ident, n.children[3]}, nil, 0)
						replaceChildRangeWithSingleNode(n, 1, 4, paren)
						if hasValueField && len(n.children) > 1 {
							ensureNodeFieldStorage(n, len(n.children))
							n.fieldIDs()[1] = valueFieldID
							n.fieldSources()[1] = fieldSourceDirect
						}
					}
				}
			}
		}
	}
}

func normalizeCBareTypeIdentifierExpressionStatements(root *Node, source []byte, lang *Language) {
	if root == nil || lang == nil || lang.Name != "c" {
		return
	}
	compoundSym, ok1 := symbolByName(lang, "compound_statement")
	typeIdSym, ok2 := symbolByName(lang, "type_identifier")
	semiSym, ok3 := symbolByName(lang, ";")
	exprStmtSym, ok4 := symbolByName(lang, "expression_statement")
	identSym, ok5 := symbolByName(lang, "identifier")
	if !ok1 || !ok2 || !ok3 || !ok4 || !ok5 {
		return
	}
	exprStmtNamed := symbolIsNamed(lang, exprStmtSym)
	identNamed := symbolIsNamed(lang, identSym)
	walkResultTree(root, func(n *Node) {
		if n.symbol == compoundSym {
			// Look for bare type_identifier ; pairs that should be expression_statement(identifier ;)
			newChildren := make([]*Node, 0, len(n.children))
			for i := 0; i < len(n.children); i++ {
				child := n.children[i]
				if child != nil && child.symbol == typeIdSym && i+1 < len(n.children) && n.children[i+1] != nil && n.children[i+1].symbol == semiSym {
					semi := n.children[i+1]
					ident := newLeafNodeInArena(n.ownerArena, identSym, identNamed, child.startByte, child.endByte, child.startPoint, child.endPoint)
					exprStmt := newParentNodeInArena(n.ownerArena, exprStmtSym, exprStmtNamed, []*Node{ident, semi}, nil, 0)
					exprStmt.startByte = child.startByte
					exprStmt.startPoint = child.startPoint
					exprStmt.endByte = semi.endByte
					exprStmt.endPoint = semi.endPoint
					newChildren = append(newChildren, exprStmt)
					i++ // skip the semicolon
					continue
				}
				newChildren = append(newChildren, child)
			}
			if len(newChildren) != len(n.children) {
				n.children = newChildren
			}
		}
	})
}

func normalizeCCastUnknownTypeIdentifiers(root *Node, source []byte, lang *Language) {
	if root == nil || lang == nil || lang.Name != "c" {
		return
	}
	syms, ok := cCastRewriteSymbolsForLanguage(lang)
	if !ok {
		return
	}
	localTypes := make(map[string]struct{})
	var castNodes []*Node
	var callNodes []*Node
	collectCLocalTypeNamesAndCandidates(root, source, lang, localTypes, func(n *Node) {
		switch {
		case cResultSymbolMatches(lang, n, syms.cast):
			castNodes = append(castNodes, n)
		case cResultSymbolMatches(lang, n, syms.call):
			callNodes = append(callNodes, n)
		}
	})

	for _, n := range castNodes {
		rewriteUnknownCCastAsCall(n, source, lang, syms, localTypes)
	}

	for _, n := range callNodes {
		rewriteKnownCCallAsCast(n, source, lang, syms, localTypes)
	}
}

type cCastRewriteSymbols struct {
	typeDescriptor     Symbol
	typeIdentifier     Symbol
	identifier         Symbol
	parenthesized      Symbol
	call               Symbol
	cast               Symbol
	argumentList       Symbol
	functionField      FieldID
	argumentsField     FieldID
	typeField          FieldID
	valueField         FieldID
	identifierNamed    bool
	typeDescNamed      bool
	typeIdentNamed     bool
	parenthesizedNamed bool
	callNamed          bool
	castNamed          bool
	argumentListNamed  bool
}

func cCastRewriteSymbolsForLanguage(lang *Language) (cCastRewriteSymbols, bool) {
	var syms cCastRewriteSymbols
	var ok bool
	if syms.typeDescriptor, ok = lang.symbolByNamePreferNamed("type_descriptor"); !ok {
		return syms, false
	}
	if syms.typeIdentifier, ok = lang.symbolByNamePreferNamed("type_identifier"); !ok {
		return syms, false
	}
	if syms.identifier, ok = lang.symbolByNamePreferNamed("identifier"); !ok {
		return syms, false
	}
	if syms.parenthesized, ok = lang.symbolByNamePreferNamed("parenthesized_expression"); !ok {
		return syms, false
	}
	if syms.call, ok = lang.symbolByNamePreferNamed("call_expression"); !ok {
		return syms, false
	}
	if syms.cast, ok = lang.symbolByNamePreferNamed("cast_expression"); !ok {
		return syms, false
	}
	if syms.argumentList, ok = lang.symbolByNamePreferNamed("argument_list"); !ok {
		return syms, false
	}
	if syms.functionField, ok = lang.FieldByName("function"); !ok {
		return syms, false
	}
	if syms.argumentsField, ok = lang.FieldByName("arguments"); !ok {
		return syms, false
	}
	if syms.typeField, ok = lang.FieldByName("type"); !ok {
		return syms, false
	}
	if syms.valueField, ok = lang.FieldByName("value"); !ok {
		return syms, false
	}
	syms.identifierNamed = symbolIsNamed(lang, syms.identifier)
	syms.typeDescNamed = symbolIsNamed(lang, syms.typeDescriptor)
	syms.typeIdentNamed = symbolIsNamed(lang, syms.typeIdentifier)
	syms.parenthesizedNamed = symbolIsNamed(lang, syms.parenthesized)
	syms.callNamed = symbolIsNamed(lang, syms.call)
	syms.castNamed = symbolIsNamed(lang, syms.cast)
	syms.argumentListNamed = symbolIsNamed(lang, syms.argumentList)
	return syms, true
}

func rewriteUnknownCCastAsCall(n *Node, source []byte, lang *Language, syms cCastRewriteSymbols, localTypes map[string]struct{}) {
	if n == nil || !cResultSymbolMatches(lang, n, syms.cast) || resultChildCount(n) != 4 {
		return
	}
	openParen := resultChildAt(n, 0)
	typeDescriptor := resultChildAt(n, 1)
	closeParen := resultChildAt(n, 2)
	value := resultChildAt(n, 3)
	if typeDescriptor == nil || value == nil || !cResultSymbolMatches(lang, typeDescriptor, syms.typeDescriptor) || resultChildCount(typeDescriptor) != 1 {
		return
	}
	typeIdent := resultChildAt(typeDescriptor, 0)
	if typeIdent == nil || !cResultSymbolMatches(lang, typeIdent, syms.typeIdentifier) || !cResultSymbolMatches(lang, value, syms.parenthesized) {
		return
	}
	if _, ok := localTypes[canonicalCTypeName(typeIdent.Text(source))]; ok {
		return
	}

	ident := newLeafNodeInArena(n.ownerArena, syms.identifier, syms.identifierNamed, typeIdent.startByte, typeIdent.endByte, typeIdent.startPoint, typeIdent.endPoint)
	function := newParentNodeInArena(n.ownerArena, syms.parenthesized, syms.parenthesizedNamed, []*Node{openParen, ident, closeParen}, nil, 0)
	argsChildren := resultChildSliceRangeForMutation(value, 0, resultChildCount(value))
	argsChildren = cloneNodeSliceIfArena(n.ownerArena, append([]*Node(nil), argsChildren...))
	arguments := newParentNodeInArena(n.ownerArena, syms.argumentList, syms.argumentListNamed, argsChildren, nil, 0)
	children := cloneNodeSliceIfArena(n.ownerArena, []*Node{function, arguments})
	fieldIDs := cloneFieldIDSliceInArena(n.ownerArena, []FieldID{syms.functionField, syms.argumentsField})
	setCRewriteChildren(n, syms.call, syms.callNamed, children, fieldIDs, []int{0, 1})
}

func rewriteKnownCCallAsCast(n *Node, source []byte, lang *Language, syms cCastRewriteSymbols, localTypes map[string]struct{}) {
	if n == nil || !cResultSymbolMatches(lang, n, syms.call) || resultChildCount(n) != 2 {
		return
	}
	function := resultChildAt(n, 0)
	arguments := resultChildAt(n, 1)
	if function == nil || arguments == nil ||
		!cResultSymbolMatches(lang, function, syms.parenthesized) ||
		!cResultSymbolMatches(lang, arguments, syms.argumentList) ||
		resultChildCount(function) < 3 {
		return
	}
	ident := firstChildWithSymbol(function, lang, syms.identifier)
	if ident == nil {
		return
	}
	if _, ok := localTypes[canonicalCTypeName(ident.Text(source))]; !ok {
		return
	}
	valueNode := firstNamedChild(arguments)
	if valueNode == nil {
		return
	}

	typeIdent := newLeafNodeInArena(n.ownerArena, syms.typeIdentifier, syms.typeIdentNamed, ident.startByte, ident.endByte, ident.startPoint, ident.endPoint)
	typeDescriptor := newParentNodeInArena(n.ownerArena, syms.typeDescriptor, syms.typeDescNamed, []*Node{typeIdent}, nil, 0)
	children := cloneNodeSliceIfArena(n.ownerArena, []*Node{
		resultChildAt(function, 0),
		typeDescriptor,
		resultChildAt(function, resultChildCount(function)-1),
		valueNode,
	})
	fieldIDs := make([]FieldID, len(children))
	fieldIDs[1] = syms.typeField
	fieldIDs[3] = syms.valueField
	fieldIDs = cloneFieldIDSliceInArena(n.ownerArena, fieldIDs)
	setCRewriteChildren(n, syms.cast, syms.castNamed, children, fieldIDs, []int{1, 3})
}

func cResultSymbolMatches(lang *Language, n *Node, sym Symbol) bool {
	if n == nil {
		return false
	}
	if n.symbol == sym {
		return true
	}
	return lang != nil && lang.PublicSymbolForNamedness(n.symbol, n.isNamed()) == sym
}

func firstChildWithSymbol(n *Node, lang *Language, sym Symbol) *Node {
	for i := 0; i < resultChildCount(n); i++ {
		child := resultChildAt(n, i)
		if cResultSymbolMatches(lang, child, sym) {
			return child
		}
	}
	return nil
}

func firstNamedChild(n *Node) *Node {
	for i := 0; i < resultChildCount(n); i++ {
		child := resultChildAt(n, i)
		if child != nil && child.isNamed() {
			return child
		}
	}
	return nil
}

func setCRewriteChildren(n *Node, symbol Symbol, named bool, children []*Node, fieldIDs []FieldID, directFieldIndexes []int) {
	n.symbol = symbol
	n.setNamed(named)
	n.children = children
	fieldSources := make([]uint8, len(children))
	for _, idx := range directFieldIndexes {
		if idx >= 0 && idx < len(fieldSources) {
			fieldSources[idx] = fieldSourceDirect
		}
	}
	n.setFieldMetadata(fieldIDs, fieldSources)
	n.productionID = 0
	populateParentNode(n, n.children)
}

func isCBuiltinPrimitiveTypeName(name string) bool {
	switch name {
	case "char", "int", "float", "double", "void", "_Bool", "_Complex", "bool", "__int128",
		"size_t", "ssize_t", "ptrdiff_t", "intptr_t", "uintptr_t",
		"int8_t", "int16_t", "int32_t", "int64_t",
		"uint8_t", "uint16_t", "uint32_t", "uint64_t",
		"wchar_t", "char16_t", "char32_t":
		return true
	default:
		return false
	}
}

func canonicalCTypeName(name string) string {
	name = strings.TrimSpace(name)
	start, end := 0, len(name)
	for start < end && !isCTypeNameChar(name[start]) {
		start++
	}
	for end > start && !isCTypeNameChar(name[end-1]) {
		end--
	}
	return name[start:end]
}

func isCTypeNameChar(b byte) bool {
	return b == '_' ||
		(b >= 'a' && b <= 'z') ||
		(b >= 'A' && b <= 'Z') ||
		(b >= '0' && b <= '9')
}

func collectCLocalTypeNamesAndCandidates(root *Node, source []byte, lang *Language, localTypes map[string]struct{}, visit func(*Node)) {
	if root == nil || lang == nil || lang.Name != "c" {
		return
	}
	typeDefinitionSym, hasTypeDefinition := lang.symbolByNamePreferNamed("type_definition")
	typeIdentifierSym, hasTypeIdentifier := lang.symbolByNamePreferNamed("type_identifier")
	walkResultTree(root, func(n *Node) {
		if hasTypeDefinition && hasTypeIdentifier && cResultSymbolMatches(lang, n, typeDefinitionSym) {
			collectCLocalTypeNamesFromDefinition(n, source, lang, typeIdentifierSym, localTypes)
		}
		if visit != nil {
			visit(n)
		}
	})
}

func collectCLocalTypeNamesFromDefinition(n *Node, source []byte, lang *Language, typeIdentifierSym Symbol, localTypes map[string]struct{}) {
	if n == nil || localTypes == nil {
		return
	}
	for i := 0; i < resultChildCount(n); i++ {
		child := resultChildAt(n, i)
		if child == nil || !cResultSymbolMatches(lang, child, typeIdentifierSym) {
			continue
		}
		if name := canonicalCTypeName(child.Text(source)); name != "" {
			localTypes[name] = struct{}{}
		}
	}
}
