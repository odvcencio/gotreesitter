package gotreesitter

import (
	"bytes"
	"unicode"
	"unicode/utf8"
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

	// Bounds for the lenient source-based type/member recovery (#136). Unlike the
	// whole-file gate above, these are PER declaration: a large real file is
	// recovered as many small, independently-bounded member reparses rather than
	// one giant reparse, so the anti-OOM guarantees from #64/#98/#106 still hold.
	// csharpMaxMemberRecoverySourceBytes caps the size of a single member snippet
	// that will be reparsed; csharpMaxTypeMemberRecoveries caps how many members
	// one type recovery will reparse.
	csharpMaxMemberRecoverySourceBytes = 32768
	csharpMaxTypeMemberRecoveries      = 4096
)

func normalizeCSharpCompatibility(root *Node, source []byte, p *Parser, lang *Language) {
	if p != nil && p.skipRecoveryReparse {
		normalizeCSharpUnicodeIdentifierSpans(root, source, lang)
		normalizeCSharpQuotedStringContentIdentifiers(root, source, lang)
		normalizeCSharpSurfaceCompatibility(root, source, lang)
		normalizeCSharpMissingAttributedProperties(root, source, lang)
		normalizeCSharpSplitScopedLambdaStatements(root, source, lang)
		normalizeCSharpInvocationStatements(root, source, lang)
		normalizeCSharpDereferenceLogicalAndCasts(root, source, lang)
		normalizeCSharpConditionalIsPatternInitializers(root, source, lang)
		normalizeCSharpConditionalIsPatternExpressions(root, lang)
		normalizeCSharpTypeConstraintKeywords(root, lang)
		normalizeCSharpSwitchTupleCasePatterns(root, lang)
		return
	}
	normalizeCSharpRecoveredTopLevelChunks(root, source, p)
	normalizeCSharpRecoveredNamespaces(root, source, p, lang)
	normalizeCSharpRecoveredTypeDeclarations(root, source, p, lang)
	normalizeCSharpUnicodeIdentifierSpans(root, source, lang)
	normalizeCSharpQuotedStringContentIdentifiers(root, source, lang)
	normalizeCSharpSurfaceCompatibility(root, source, lang)
	normalizeCSharpMissingAttributedProperties(root, source, lang)
	normalizeCSharpQueryExpressions(root, source, p)
	normalizeCSharpSplitScopedLambdaStatements(root, source, lang)
	normalizeCSharpRecoveredScopedLambdaBlocks(root, source, p)
	normalizeCSharpRecoveredMethodBlocks(root, source, p)
	normalizeCSharpInvocationStatements(root, source, lang)
	normalizeCSharpDereferenceLogicalAndCasts(root, source, lang)
	normalizeCSharpConditionalIsPatternInitializers(root, source, lang)
	normalizeCSharpConditionalIsPatternExpressions(root, lang)
	normalizeCSharpTypeConstraintKeywords(root, lang)
	normalizeCSharpSwitchTupleCasePatterns(root, lang)
}

func normalizeCSharpSurfaceCompatibility(root *Node, source []byte, lang *Language) {
	if root == nil || lang == nil || lang.Name != "c_sharp" || len(source) == 0 {
		return
	}
	booleanSym, hasBoolean := symbolByName(lang, "boolean_literal")
	modifierSym, hasModifier := symbolByName(lang, "modifier")
	aliasSym, hasAlias := symbolByName(lang, "alias_qualified_name")
	identifierSym, hasIdentifier := symbolByName(lang, "identifier")
	globalSym, hasGlobal := symbolByName(lang, "global")
	lambdaSym, ok := symbolByName(lang, "lambda_expression")
	hasLambda := ok
	argSym, hasArgument := symbolByName(lang, "argument")
	stringLiteralSym, hasStringLiteral := symbolByName(lang, "string_literal")
	globalNamed := symbolIsNamed(lang, globalSym)
	modifierNamed := symbolIsNamed(lang, modifierSym)
	walkResultTree(root, func(n *Node) {
		if n == nil {
			return
		}
		childCount := resultChildCount(n)
		if childCount == 0 {
			if hasBoolean && n.symbol == booleanSym {
				if childSym, ok := csharpCollapsedBooleanTokenSymbol(lang, source, n); ok {
					csharpInstallCollapsedChild(n, childSym, symbolIsNamed(lang, childSym))
				}
				return
			}
			if hasModifier && n.symbol == modifierSym {
				if childSym, ok := csharpCollapsedModifierTokenSymbol(lang, source, n); ok {
					csharpInstallCollapsedChild(n, childSym, symbolIsNamed(lang, childSym))
				}
				return
			}
		}
		if hasAlias && hasIdentifier && hasGlobal && n.symbol == aliasSym && childCount > 0 {
			first := resultChildAt(n, 0)
			if first != nil && first.symbol == identifierSym && resultChildCount(first) == 0 &&
				first.startByte < first.endByte && int(first.endByte) <= len(source) &&
				string(source[first.startByte:first.endByte]) == "global" {
				csharpInstallCollapsedChild(first, globalSym, globalNamed)
			}
			return
		}
		if hasLambda && hasIdentifier && hasModifier && n.symbol == lambdaSym && childCount > 0 && len(n.fieldIDs) > 0 {
			first := resultChildAt(n, 0)
			if first != nil && first.symbol == identifierSym && resultChildCount(first) == 0 &&
				first.startByte < first.endByte && int(first.endByte) <= len(source) &&
				string(source[first.startByte:first.endByte]) == "async" {
				retagResultRoot(first, modifierSym, modifierNamed)
				n.fieldIDs[0] = 0
				if len(n.fieldSources) > 0 {
					n.fieldSources[0] = fieldSourceNone
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

func csharpCollapsedBooleanTokenSymbol(lang *Language, source []byte, n *Node) (Symbol, bool) {
	if n == nil || n.startByte >= n.endByte || int(n.endByte) > len(source) {
		return 0, false
	}
	text := string(source[n.startByte:n.endByte])
	switch text {
	case "true", "false":
		return symbolByName(lang, text)
	default:
		return 0, false
	}
}

func csharpCollapsedModifierTokenSymbol(lang *Language, source []byte, n *Node) (Symbol, bool) {
	if n == nil || n.startByte >= n.endByte || int(n.endByte) > len(source) {
		return 0, false
	}
	text := string(source[n.startByte:n.endByte])
	switch text {
	case "abstract",
		"const",
		"extern",
		"file",
		"fixed",
		"internal",
		"new",
		"override",
		"partial",
		"private",
		"protected",
		"public",
		"readonly",
		"ref",
		"required",
		"sealed",
		"static",
		"unsafe",
		"virtual",
		"volatile":
		return symbolByName(lang, text)
	default:
		return 0, false
	}
}

func csharpInstallCollapsedChild(n *Node, childSym Symbol, childNamed bool) {
	if n == nil || resultChildCount(n) != 0 {
		return
	}
	child := newLeafNodeInArena(n.ownerArena, childSym, childNamed, n.startByte, n.endByte, n.startPoint, n.endPoint)
	child.parent = n
	child.childIndex = 0
	n.children = cloneNodeSliceInArena(n.ownerArena, []*Node{child})
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
	root.setHasError(false)
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
			nodes, ok := csharpRecoverTopLevelChunkNodesFromRange(source, part[0], part[1], p, arena)
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

func csharpRecoverTopLevelChunkNodesFromRange(source []byte, start, end uint32, p *Parser, arena *nodeArena) ([]*Node, bool) {
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
			if recovered, ok := csharpRecoverAttributedTopLevelTypeDeclarationFromRange(source, declStart, end, attributeLists, p.language, arena); ok {
				return []*Node{recovered}, true
			}
		}
	}
	if recovered, ok := csharpRecoverSourceTopLevelTypeDeclarationFromRange(source, start, end, p, arena); ok {
		return []*Node{recovered}, true
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
	if int(nsStart)+len("namespace") > len(source) || !bytes.HasPrefix(source[nsStart:], []byte("namespace")) {
		return nil, startIdx, false
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
	recovered, ok := csharpRecoverNamespaceNodeFromRange(source, nsStart, nsEnd, p, lang, arena)
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

func normalizeCSharpUnicodeIdentifierSpans(root *Node, source []byte, lang *Language) {
	if root == nil || lang == nil || lang.Name != "c_sharp" || len(source) == 0 {
		return
	}
	walkResultTree(root, func(n *Node) {
		if n.Type(lang) == "identifier" && len(n.children) == 0 {
			if end := csharpUnicodeIdentifierEnd(source, n.startByte); end > n.endByte && csharpCanExtendLeafNodeTo(n, end) {
				n.endByte = end
				n.endPoint = advancePointByBytes(Point{}, source[:end])
			}
		}
	})
}

func csharpUnicodeIdentifierEnd(source []byte, start uint32) uint32 {
	if int(start) >= len(source) {
		return start
	}
	r, size := utf8.DecodeRune(source[start:])
	if size == 0 || r == utf8.RuneError && size == 1 || !csharpIdentifierStartRune(r) {
		return start
	}
	pos := start + uint32(size)
	for int(pos) < len(source) {
		r, size = utf8.DecodeRune(source[pos:])
		if size == 0 || r == utf8.RuneError && size == 1 || !csharpIdentifierContinueRune(r) {
			break
		}
		pos += uint32(size)
	}
	return pos
}

func csharpIdentifierStartRune(r rune) bool {
	return r == '_' || unicode.IsLetter(r) || unicode.In(r, unicode.Nl)
}

func csharpIdentifierContinueRune(r rune) bool {
	return csharpIdentifierStartRune(r) ||
		unicode.IsDigit(r) ||
		unicode.In(r, unicode.Mn, unicode.Mc, unicode.Pc, unicode.Cf)
}

func csharpCanExtendLeafNodeTo(n *Node, end uint32) bool {
	if n == nil || end <= n.endByte {
		return false
	}
	if n.parent == nil {
		return true
	}
	for _, sibling := range n.parent.children {
		if sibling == nil || sibling == n {
			continue
		}
		if sibling.startByte >= n.endByte && sibling.startByte < end {
			return false
		}
	}
	return true
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
		if recovered, next, ok := csharpRecoverAttributedTopLevelTypeDeclarationFromChildren(root.children, i, source, lang, root.ownerArena); ok {
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
		attributed, ok := csharpRecoverAttributedTopLevelTypeDeclarationFromError(child, source, lang, root.ownerArena)
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

func normalizeCSharpTypeConstraintKeywords(root *Node, lang *Language) {
	if root == nil || lang == nil || lang.Name != "c_sharp" {
		return
	}
	walkResultTree(root, func(n *Node) {
		if n.Type(lang) == "type_parameter_constraint" && len(n.children) == 1 {
			child := n.children[0]
			if child != nil && child.Type(lang) == "identifier" && len(child.children) == 1 {
				inner := child.children[0]
				if inner != nil && inner.Type(lang) == "notnull" && !inner.isNamed() &&
					child.startByte == inner.startByte && child.endByte == inner.endByte {
					n.children[0] = inner
					inner.parent = n
					inner.childIndex = 0
					if len(n.fieldIDs) > 0 {
						n.fieldIDs[0] = 0
					}
					if len(n.fieldSources) > 0 {
						n.fieldSources[0] = fieldSourceNone
					}
				}
			}
		}
	})
}

type csharpSimpleJoinQuerySpec struct {
	queryStart uint32
	queryEnd   uint32
	semiPos    uint32

	fromStart uint32
	fromEnd   uint32
	rangeName [2]uint32
	in1Start  uint32
	in1End    uint32
	source1   [2]uint32

	joinStart uint32
	joinEnd   uint32
	joinName  [2]uint32
	in2Start  uint32
	in2End    uint32
	source2   [2]uint32

	onStart    uint32
	onEnd      uint32
	leftObj    [2]uint32
	leftDotPos uint32
	leftProp   [2]uint32

	equalsStart uint32
	equalsEnd   uint32
	rightObj    [2]uint32
	rightDotPos uint32
	rightProp   [2]uint32

	selectStart uint32
	selectEnd   uint32
	selectName  [2]uint32
}
