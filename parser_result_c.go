package gotreesitter

import "strings"

func normalizeCCompatibility(root *Node, source []byte, lang *Language) {
	normalizeCCompatibilityWithParser(root, source, nil, lang)
}

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
		run("c_preprocessor_directive_shapes", func() {
			normalizeCPreprocessorDirectiveShapes(root, source, lang)
		})
		// Fused walk replaces four preorder passes (declaration bounds + builtin
		// primitive identifiers + variadic ellipsis + preproc newline spans).
		// PreprocNewlineSpans only extends `\n` token endByte, which downstream
		// passes don't read — safe to move earlier in the chain.
		run("c_declaration_bounds", func() {
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
		run("c_pointer_assignment_precedence", func() {
			normalizeCPointerAssignmentPrecedence(root, lang)
		})
		run("c_collapsed_keyword_children", func() {
			normalizeCCollapsedKeywordChildren(root, source, lang)
		})
		return
	}
	normalizeCTranslationUnitRoot(root, lang)
	normalizeCRecoveredTopLevelChunks(root, source, p, lang)
	normalizeCPreprocessorDirectiveShapes(root, source, lang)
	normalizeCFusedDeclVariadicWalk(root, source, lang)
	normalizeCSizeofUnknownTypeIdentifiers(root, source, lang)
	normalizeCCastUnknownTypeIdentifiers(root, source, lang)
	normalizeCBareTypeIdentifierExpressionStatements(root, source, lang)
	normalizeCPointerAssignmentPrecedence(root, lang)
	normalizeCCollapsedKeywordChildren(root, source, lang)
}

// normalizeCFusedDeclVariadicWalk performs the work of four previously
// separate preorder walks in a single pass: declaration-bounds extension,
// builtin primitive type identifier promotion, variadic-parameter ellipsis
// materialization, and preprocessor newline-span extension. The handlers
// either gate on disjoint node symbols (decl/builtin/variadic) or operate on
// the child layer (newline spans), so a single visit can apply all four
// without ordering concerns.
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

	// declaration bounds applies to c and cpp.
	declarationSym, hasDecl := symbolByName(lang, "declaration")

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

	// variadic ellipsis is c only.
	var variadicSym Symbol
	var ellipsisSym Symbol
	var ellipsisNamed bool
	hasVariadic := false
	if isC {
		vs, ok := lang.SymbolByName("variadic_parameter")
		if ok {
			es, ok2 := lang.SymbolByName("...")
			if ok2 {
				variadicSym = vs
				ellipsisSym = es
				ellipsisNamed = symbolIsNamed(lang, ellipsisSym)
				hasVariadic = true
			}
		}
	}

	// preproc newline spans applies to c and cpp.
	nlSym, hasNl := symbolByName(lang, "\n")
	hasNewlineSpan := hasNl && srcLen > 0

	if !hasDecl && !hasBuiltin && !hasVariadic && !hasNewlineSpan {
		return
	}

	walkResultTree(root, func(n *Node) {
		if hasDecl && n.symbol == declarationSym {
			first, last := firstAndLastNonNilChild(n.children)
			if first != nil && n.startByte < first.startByte &&
				first.startByte <= srcLen &&
				cBytesAreCommentOrTrivia(source[n.startByte:first.startByte]) {
				n.startByte = first.startByte
				n.startPoint = first.startPoint
			}
			if last != nil && n.endByte > last.endByte &&
				n.endByte <= srcLen &&
				bytesAreTrivia(source[last.endByte:n.endByte]) {
				setNodeEndTo(n, last.endByte, source)
			}
		}
		if hasBuiltin && n.symbol == typeIdentifierSym {
			if isCBuiltinPrimitiveTypeName(canonicalCTypeName(n.Text(source))) {
				n.symbol = primitiveTypeSym
				n.setNamed(primitiveTypeNamed)
			}
		}
		if hasVariadic && n.symbol == variadicSym && len(n.children) == 0 {
			child := newLeafNodeInArena(n.ownerArena, ellipsisSym, ellipsisNamed, n.startByte, n.endByte, n.startPoint, n.endPoint)
			n.children = cloneNodeSliceInArena(n.ownerArena, []*Node{child})
			populateParentNode(n, n.children)
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
			errorRoot = tree.RootNodeWithOffset(start, startPoint)
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
		offsetRoot = tree.RootNodeWithOffset(start, offset.point)
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

func normalizeCCollapsedKeywordChildren(root *Node, source []byte, lang *Language) {
	if root == nil || lang == nil || len(source) == 0 {
		return
	}
	if lang.Name != "c" && lang.Name != "cpp" {
		return
	}
	rules := make([]cCollapsedKeywordRule, 0, 3)
	if rule, ok := newCCollapsedKeywordRule(lang, "null", "NULL"); ok {
		rules = append(rules, rule)
	}
	if rule, ok := newCCollapsedKeywordRule(lang, "type_qualifier", "const", "restrict", "volatile", "_Atomic"); ok {
		rules = append(rules, rule)
	}
	if rule, ok := newCCollapsedKeywordRule(
		lang,
		"storage_class_specifier",
		"auto",
		"extern",
		"inline",
		"register",
		"static",
		"_Thread_local",
	); ok {
		rules = append(rules, rule)
	}
	if len(rules) == 0 {
		return
	}
	var walk func(*Node)
	walk = func(n *Node) {
		if n == nil {
			return
		}
		childCount := resultChildCount(n)
		if childCount == 0 {
			if rule, ok := cCollapsedKeywordRuleForParent(rules, n.symbol); ok {
				normalizeCCollapsedKeywordLeaf(n, source, rule)
			}
			return
		}
		if n.ownerArena == nil || n.childIndex > finalChildSidecarIndexBase {
			for _, child := range n.children {
				walk(child)
			}
			return
		}
		view := resultMutableChildrenForMutation(n)
		if !view.hasFinalChildRefs() {
			for i := 0; i < childCount; i++ {
				walk(resultChildAt(n, i))
			}
			return
		}
		for i := 0; i < view.Len(); i++ {
			entry, ok := view.Entry(i)
			if !ok {
				continue
			}
			sym := stackEntryNodeSymbol(entry)
			if !cCollapsedKeywordRulesContainParent(rules, sym) && stackEntryNodeChildCount(entry) == 0 {
				continue
			}
			walk(resultChildAt(n, i))
		}
	}
	walk(root)
}

type cCollapsedKeywordRule struct {
	parentSym Symbol
	children  []cCollapsedKeywordChild
}

type cCollapsedKeywordChild struct {
	text  string
	sym   Symbol
	named bool
}

func newCCollapsedKeywordRule(lang *Language, parentName string, childNames ...string) (cCollapsedKeywordRule, bool) {
	var rule cCollapsedKeywordRule
	if lang == nil || parentName == "" || len(childNames) == 0 {
		return rule, false
	}
	parentSym, ok := lang.symbolByNameAndNamed(parentName, true)
	if !ok {
		parentSym, ok = symbolByName(lang, parentName)
	}
	if !ok {
		return rule, false
	}
	rule.parentSym = parentSym
	rule.children = make([]cCollapsedKeywordChild, 0, len(childNames))
	for _, childName := range childNames {
		childSym, ok := lang.symbolByNameAndNamed(childName, false)
		if !ok {
			childSym, ok = symbolByName(lang, childName)
			if !ok {
				continue
			}
		}
		rule.children = append(rule.children, cCollapsedKeywordChild{
			text:  childName,
			sym:   childSym,
			named: symbolIsNamed(lang, childSym),
		})
	}
	return rule, len(rule.children) > 0
}

func cCollapsedKeywordRuleForParent(rules []cCollapsedKeywordRule, sym Symbol) (cCollapsedKeywordRule, bool) {
	for _, rule := range rules {
		if rule.parentSym == sym {
			return rule, true
		}
	}
	return cCollapsedKeywordRule{}, false
}

func cCollapsedKeywordRulesContainParent(rules []cCollapsedKeywordRule, sym Symbol) bool {
	for _, rule := range rules {
		if rule.parentSym == sym {
			return true
		}
	}
	return false
}

func normalizeCCollapsedKeywordLeaf(n *Node, source []byte, rule cCollapsedKeywordRule) {
	if n == nil || int(n.startByte) > len(source) || int(n.endByte) > len(source) || n.startByte > n.endByte {
		return
	}
	for _, child := range rule.children {
		if !cSourceRangeEquals(source, n.startByte, n.endByte, child.text) {
			continue
		}
		leaf := newLeafNodeInArena(n.ownerArena, child.sym, child.named, n.startByte, n.endByte, n.startPoint, n.endPoint)
		leaf.parent = n
		leaf.childIndex = 0
		n.children = cloneNodeSliceInArena(n.ownerArena, []*Node{leaf})
		return
	}
}

func cSourceRangeEquals(source []byte, start, end uint32, text string) bool {
	if start > end || int(end) > len(source) || int(end-start) != len(text) {
		return false
	}
	offset := int(start)
	for i := 0; i < len(text); i++ {
		if source[offset+i] != text[i] {
			return false
		}
	}
	return true
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

func normalizeCPreprocessorDirectiveShapes(root *Node, source []byte, lang *Language) {
	if root == nil || lang == nil || len(root.children) == 0 {
		return
	}
	if lang.Name != "c" && lang.Name != "cpp" {
		return
	}
	if root.Type(lang) != "translation_unit" {
		return
	}
	preprocDefSym, hasPreprocDef := symbolByName(lang, "preproc_def")
	preprocArgSym, hasPreprocArg := symbolByName(lang, "preproc_arg")
	nameFieldID, hasNameField := lang.FieldByName("name")
	valueFieldID, hasValueField := lang.FieldByName("value")
	preprocArgNamed := hasPreprocArg && symbolIsNamed(lang, preprocArgSym)

	out := make([]*Node, 0, len(root.children))
	changed := false
	for i := 0; i < len(root.children); i++ {
		child := root.children[i]
		if child == nil {
			continue
		}
		if hasPreprocDef && hasPreprocArg && hasNameField && hasValueField {
			if normalizeCWhitespaceSeparatedFunctionMacro(child, source, lang, preprocDefSym, preprocArgSym, preprocArgNamed, nameFieldID, valueFieldID) {
				changed = true
			}
		}
		if consumed, ok := normalizeCPreprocessorDirectiveRange(child, source, lang); ok {
			changed = true
			for i+1 < len(root.children) && root.children[i+1] != nil && root.children[i+1].startByte < consumed && root.children[i+1].endByte <= consumed {
				i++
			}
		}
		out = append(out, child)
	}
	if !changed {
		return
	}
	out = cloneNodeSliceIfArena(root.ownerArena, out)
	replaceNodeChildrenUnfielded(root, out)
	extendNodeToTrailingWhitespace(root, source)
}

func normalizeCWhitespaceSeparatedFunctionMacro(node *Node, source []byte, lang *Language, preprocDefSym, preprocArgSym Symbol, preprocArgNamed bool, nameFieldID, valueFieldID FieldID) bool {
	if node == nil || lang == nil || node.Type(lang) != "preproc_function_def" || len(node.children) < 3 || len(node.children) > 4 {
		return false
	}
	name := node.children[1]
	params := node.children[2]
	if name == nil || params == nil || name.Type(lang) != "identifier" || params.Type(lang) != "preproc_params" {
		return false
	}
	value := (*Node)(nil)
	if len(node.children) == 4 {
		value = node.children[3]
		if value == nil || value.Type(lang) != "preproc_arg" {
			return false
		}
	} else {
		value = newParentNodeInArena(node.ownerArena, preprocArgSym, preprocArgNamed, nil, nil, 0)
		value.startByte = params.startByte
		value.startPoint = params.startPoint
		value.endByte = params.endByte
		value.endPoint = params.endPoint
	}
	if name.endByte >= params.startByte || params.startByte > uint32(len(source)) {
		return false
	}
	if !bytesAreTrivia(source[name.endByte:params.startByte]) {
		return false
	}

	value.startByte = params.startByte
	value.startPoint = advancePointByBytes(Point{}, source[:params.startByte])
	if value.endByte < value.startByte {
		value.endByte = value.startByte
		value.endPoint = value.startPoint
	}

	children := []*Node{node.children[0], name, value}
	children = cloneNodeSliceIfArena(node.ownerArena, children)
	node.symbol = preprocDefSym
	node.setNamed(symbolIsNamed(lang, preprocDefSym))
	node.children = children
	ensureNodeFieldStorage(node, len(children))
	for i := range node.fieldIDs {
		node.fieldIDs[i] = 0
	}
	for i := range node.fieldSources {
		node.fieldSources[i] = fieldSourceNone
	}
	node.fieldIDs[1] = nameFieldID
	node.fieldIDs[2] = valueFieldID
	node.fieldSources[1] = fieldSourceDirect
	node.fieldSources[2] = fieldSourceDirect
	populateParentNode(node, node.children)
	return true
}

func normalizeCPreprocessorDirectiveRange(node *Node, source []byte, lang *Language) (uint32, bool) {
	if node == nil || lang == nil || len(node.children) == 0 {
		return 0, false
	}
	switch node.Type(lang) {
	case "preproc_def", "preproc_function_def", "preproc_call":
	default:
		return 0, false
	}
	arg := node.children[len(node.children)-1]
	if arg == nil || arg.Type(lang) != "preproc_arg" || node.startByte >= uint32(len(source)) {
		return 0, false
	}
	directiveEnd, valueEnd, ok := cScanPreprocessorDirectiveExtent(source, node.startByte)
	if !ok || directiveEnd <= node.endByte {
		return 0, false
	}
	valueStart := cScanPreprocessorValueStart(source, arg.startByte, valueEnd)
	if valueStart < arg.startByte || valueStart > valueEnd {
		valueStart = arg.startByte
	}
	arg.startByte = valueStart
	arg.startPoint = advancePointByBytes(Point{}, source[:valueStart])
	setNodeEndTo(arg, valueEnd, source)
	populateParentNode(node, node.children)
	extendNodeEndTo(node, directiveEnd, source)
	return directiveEnd, true
}

func cScanPreprocessorDirectiveExtent(source []byte, start uint32) (directiveEnd uint32, valueEnd uint32, ok bool) {
	if start >= uint32(len(source)) {
		return 0, 0, false
	}
	lineStart := int(start)
	lastValueEnd := lineStart
	for lineStart < len(source) {
		lineEnd := lineStart
		for lineEnd < len(source) && source[lineEnd] != '\n' {
			lineEnd++
		}
		lastValueEnd = lineEnd
		if lineEnd > lineStart && source[lineEnd-1] == '\r' {
			lastValueEnd--
		}
		directiveEnd = uint32(lineEnd)
		if lineEnd < len(source) && source[lineEnd] == '\n' {
			directiveEnd++
		}
		if !cLineEndsWithContinuation(source[lineStart:lineEnd]) {
			return directiveEnd, uint32(lastValueEnd), true
		}
		lineStart = lineEnd + 1
	}
	return uint32(len(source)), uint32(lastValueEnd), true
}

func cScanPreprocessorValueStart(source []byte, start, end uint32) uint32 {
	if start > end || end > uint32(len(source)) {
		return start
	}
	i := start
	for i < end {
		switch source[i] {
		case ' ', '\t', '\n', '\r', '\f', '\\':
			i++
			continue
		default:
			return i
		}
	}
	return end
}

func cLineEndsWithContinuation(line []byte) bool {
	end := len(line)
	for end > 0 && (line[end-1] == ' ' || line[end-1] == '\t' || line[end-1] == '\f' || line[end-1] == '\r') {
		end--
	}
	if end == 0 || line[end-1] != '\\' {
		return false
	}
	backslashes := 0
	for i := end - 1; i >= 0 && line[i] == '\\'; i-- {
		backslashes++
	}
	return backslashes%2 == 1
}

func cBytesAreCommentOrTrivia(b []byte) bool {
	for i := 0; i < len(b); {
		switch b[i] {
		case ' ', '\t', '\n', '\r', '\f':
			i++
		case '/':
			if i+1 >= len(b) {
				return false
			}
			switch b[i+1] {
			case '/':
				end, ok := cScanLineCommentEnd(b, i)
				if !ok {
					return false
				}
				i = end
			case '*':
				end, ok := cScanBlockCommentEnd(b, i)
				if !ok {
					return false
				}
				i = end
			default:
				return false
			}
		default:
			return false
		}
	}
	return true
}

func cScanLineCommentEnd(b []byte, start int) (int, bool) {
	if start+1 >= len(b) || b[start] != '/' || b[start+1] != '/' {
		return 0, false
	}
	i := start + 2
	for i < len(b) {
		if b[i] == '\n' {
			lineEnd := i
			if cLineEndsWithContinuation(b[start:lineEnd]) {
				i++
				continue
			}
			return i + 1, true
		}
		i++
	}
	return len(b), true
}

func cScanBlockCommentEnd(b []byte, start int) (int, bool) {
	if start+1 >= len(b) || b[start] != '/' || b[start+1] != '*' {
		return 0, false
	}
	for i := start + 2; i+1 < len(b); i++ {
		if b[i] == '*' && b[i+1] == '/' {
			return i + 2, true
		}
	}
	return 0, false
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
							n.fieldIDs[1] = valueFieldID
							n.fieldSources[1] = fieldSourceDirect
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
	n.fieldIDs = fieldIDs
	n.fieldSources = make([]uint8, len(children))
	for _, idx := range directFieldIndexes {
		if idx >= 0 && idx < len(n.fieldSources) {
			n.fieldSources[idx] = fieldSourceDirect
		}
	}
	n.productionID = 0
	populateParentNode(n, n.children)
}

func normalizeCPointerAssignmentPrecedence(root *Node, lang *Language) {
	if root == nil || lang == nil {
		return
	}
	if lang.Name != "c" && lang.Name != "cpp" {
		return
	}

	rewriteResultTreeChildrenPostorder(root, func(n *Node) *Node {
		return rewriteCPointerAssignmentPrecedence(n, lang)
	})
}

func rewriteCPointerAssignmentPrecedence(node *Node, lang *Language) *Node {
	if node == nil || lang == nil || node.Type(lang) != "pointer_expression" || len(node.children) != 2 {
		return nil
	}
	operator := node.children[0]
	assignment := node.children[1]
	if operator == nil || assignment == nil || operator.Type(lang) != "*" || assignment.Type(lang) != "assignment_expression" || len(assignment.children) != 3 {
		return nil
	}
	left := assignment.children[0]
	assignOp := assignment.children[1]
	right := assignment.children[2]
	if left == nil || assignOp == nil || right == nil || !isCAssignmentOperatorToken(assignOp.Type(lang)) {
		return nil
	}

	rewrittenPointer := cloneNodeInArena(node.ownerArena, node)
	rewrittenPointer.children = cloneNodeSliceInArena(node.ownerArena, []*Node{operator, left})
	populateParentNode(rewrittenPointer, rewrittenPointer.children)

	rewrittenAssign := cloneNodeInArena(node.ownerArena, assignment)
	rewrittenAssign.children = cloneNodeSliceInArena(node.ownerArena, []*Node{rewrittenPointer, assignOp, right})
	populateParentNode(rewrittenAssign, rewrittenAssign.children)
	return rewrittenAssign
}

func isCAssignmentOperatorToken(tok string) bool {
	if tok == "=" {
		return true
	}
	if !strings.HasSuffix(tok, "=") {
		return false
	}
	switch tok {
	case "==", "!=", "<=", ">=", "=>", "===", "!==":
		return false
	default:
		return true
	}
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
