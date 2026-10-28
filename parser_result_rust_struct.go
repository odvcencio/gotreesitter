package gotreesitter

import "strings"

func normalizeRustRecoveredStructExpressionRoot(root *Node, source []byte, lang *Language) {
	if root == nil || lang == nil || lang.Name != "rust" || root.Type(lang) != "ERROR" || len(source) == 0 || len(root.children) == 0 {
		return
	}
	if !rustLooksLikeSplitStructExpressionRoot(root, lang) {
		return
	}
	stmts, ok := rustBuildRecoveredStructStatements(root.ownerArena, source, lang)
	if !ok || len(stmts) == 0 {
		return
	}
	sourceFileSym, ok := symbolByName(lang, "source_file")
	if !ok {
		return
	}
	retagResultRoot(root, sourceFileSym, rustNamedForSymbol(lang, sourceFileSym))
	replaceNodeChildrenUnfielded(root, cloneNodeSliceInArena(root.ownerArena, stmts))
	root.setHasError(false)
	if root.endByte < uint32(len(source)) && bytesAreTrivia(source[root.endByte:]) {
		extendNodeEndTo(root, uint32(len(source)), source)
	}
}

func rustLooksLikeSplitStructExpressionRoot(root *Node, lang *Language) bool {
	if root == nil || lang == nil || len(root.children) == 0 {
		return false
	}
	sawCandidate := false
	for _, child := range root.children {
		if child == nil {
			continue
		}
		switch child.Type(lang) {
		case "ERROR", "_expression", "field_expression", "expression_statement", "assignment_expression":
			sawCandidate = true
		default:
			return false
		}
	}
	return sawCandidate
}

func rustBuildRecoveredStructStatements(arena *nodeArena, source []byte, lang *Language) ([]*Node, bool) {
	if arena == nil || lang == nil || len(source) == 0 {
		return nil, false
	}
	spans := rustTopLevelStatementSpans(source)
	if len(spans) == 0 {
		return nil, false
	}
	stmts := make([]*Node, 0, len(spans))
	for _, span := range spans {
		stmt, ok := rustBuildRecoveredStructStatement(arena, source, lang, span[0], span[1])
		if !ok {
			return nil, false
		}
		stmts = append(stmts, stmt)
	}
	return stmts, true
}

func rustTopLevelStatementSpans(source []byte) [][2]uint32 {
	var spans [][2]uint32
	stmtStart := uint32(0)
	braceDepth := 0
	parenDepth := 0
	bracketDepth := 0
	inString := false
	escaped := false
	for i := 0; i < len(source); i++ {
		b := source[i]
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
		case ';':
			if braceDepth == 0 && parenDepth == 0 && bracketDepth == 0 {
				spans = append(spans, [2]uint32{stmtStart, uint32(i + 1)})
				stmtStart = uint32(i + 1)
			}
		}
	}
	for stmtStart < uint32(len(source)) && rustIsSpaceByte(source[stmtStart]) {
		stmtStart++
	}
	if stmtStart < uint32(len(source)) {
		return nil
	}
	return spans
}

func rustBuildRecoveredStructStatement(arena *nodeArena, source []byte, lang *Language, start, end uint32) (*Node, bool) {
	start, end = rustTrimSpaceBounds(source, start, end)
	if start >= end || end == 0 || source[end-1] != ';' {
		return nil, false
	}
	if rustHasPrefixAt(source, start, "let") && start+3 < end && rustIsSpaceByte(source[start+3]) {
		return rustBuildRecoveredLetStructStatement(arena, source, lang, start, end)
	}
	return rustBuildRecoveredExpressionStructStatement(arena, source, lang, start, end)
}

func rustBuildRecoveredLetStructStatement(arena *nodeArena, source []byte, lang *Language, start, end uint32) (*Node, bool) {
	letDeclSym, ok := symbolByName(lang, "let_declaration")
	if !ok {
		return nil, false
	}
	identifierSym, ok := symbolByName(lang, "identifier")
	if !ok {
		return nil, false
	}
	eqPos, ok := rustFindTopLevelByte(source, start, end, '=')
	if !ok {
		return nil, false
	}
	nameStart := rustSkipSpaceBytes(source, start+3)
	nameEnd := nameStart
	for nameEnd < eqPos && rustIsIdentByte(source[nameEnd]) {
		nameEnd++
	}
	nameStart, nameEnd = rustTrimSpaceBounds(source, nameStart, nameEnd)
	if nameStart >= nameEnd {
		return nil, false
	}
	name := newLeafNodeInArena(
		arena,
		identifierSym,
		rustNamedForSymbol(lang, identifierSym),
		nameStart,
		nameEnd,
		advancePointByBytes(Point{}, source[:nameStart]),
		advancePointByBytes(Point{}, source[:nameEnd]),
	)
	exprStart := rustSkipSpaceBytes(source, eqPos+1)
	structExpr, ok := rustBuildRecoveredStructExpression(arena, source, lang, exprStart, end-1)
	if !ok {
		return nil, false
	}
	letDecl := newParentNodeInArena(
		arena,
		letDeclSym,
		rustNamedForSymbol(lang, letDeclSym),
		[]*Node{name, structExpr},
		nil,
		0,
	)
	letDecl.startByte = start
	letDecl.startPoint = advancePointByBytes(Point{}, source[:start])
	letDecl.endByte = end
	letDecl.endPoint = advancePointByBytes(Point{}, source[:end])
	return letDecl, true
}

func rustBuildRecoveredExpressionStructStatement(arena *nodeArena, source []byte, lang *Language, start, end uint32) (*Node, bool) {
	exprStmtSym, ok := symbolByName(lang, "expression_statement")
	if !ok {
		return nil, false
	}
	structExpr, ok := rustBuildRecoveredStructExpression(arena, source, lang, start, end-1)
	if !ok {
		return nil, false
	}
	stmt := newParentNodeInArena(
		arena,
		exprStmtSym,
		rustNamedForSymbol(lang, exprStmtSym),
		[]*Node{structExpr},
		nil,
		0,
	)
	stmt.startByte = start
	stmt.startPoint = advancePointByBytes(Point{}, source[:start])
	stmt.endByte = end
	stmt.endPoint = advancePointByBytes(Point{}, source[:end])
	return stmt, true
}

func rustBuildRecoveredStructExpression(arena *nodeArena, source []byte, lang *Language, start, end uint32) (*Node, bool) {
	structExprSym, ok := symbolByName(lang, "struct_expression")
	if !ok {
		return nil, false
	}
	openBrace, ok := rustFindTopLevelByte(source, start, end, '{')
	if !ok {
		return nil, false
	}
	typeNode, ok := rustBuildRecoveredStructTypeNode(arena, source, lang, start, openBrace)
	if !ok {
		return nil, false
	}
	fieldList, ok := rustBuildRecoveredFieldInitializerList(arena, source, lang, openBrace, end)
	if !ok {
		return nil, false
	}
	structExpr := newParentNodeInArena(
		arena,
		structExprSym,
		rustNamedForSymbol(lang, structExprSym),
		[]*Node{typeNode, fieldList},
		nil,
		0,
	)
	structExpr.startByte = typeNode.startByte
	structExpr.startPoint = typeNode.startPoint
	structExpr.endByte = fieldList.endByte
	structExpr.endPoint = fieldList.endPoint
	return structExpr, true
}

func rustBuildRecoveredStructTypeNode(arena *nodeArena, source []byte, lang *Language, start, end uint32) (*Node, bool) {
	start, end = rustTrimSpaceBounds(source, start, end)
	if start >= end {
		return nil, false
	}
	if scopePos, ok := rustFindTopLevelDoubleColon(source, start, end); ok {
		scopedTypeSym, ok := symbolByName(lang, "scoped_type_identifier")
		if !ok {
			return nil, false
		}
		identifierSym, ok := symbolByName(lang, "identifier")
		if !ok {
			return nil, false
		}
		typeIdentifierSym, ok := symbolByName(lang, "type_identifier")
		if !ok {
			return nil, false
		}
		leftStart, leftEnd := rustTrimSpaceBounds(source, start, scopePos)
		rightStart, rightEnd := rustTrimSpaceBounds(source, scopePos+2, end)
		if leftStart >= leftEnd || rightStart >= rightEnd {
			return nil, false
		}
		left := newLeafNodeInArena(
			arena,
			identifierSym,
			rustNamedForSymbol(lang, identifierSym),
			leftStart,
			leftEnd,
			advancePointByBytes(Point{}, source[:leftStart]),
			advancePointByBytes(Point{}, source[:leftEnd]),
		)
		right := newLeafNodeInArena(
			arena,
			typeIdentifierSym,
			rustNamedForSymbol(lang, typeIdentifierSym),
			rightStart,
			rightEnd,
			advancePointByBytes(Point{}, source[:rightStart]),
			advancePointByBytes(Point{}, source[:rightEnd]),
		)
		scoped := newParentNodeInArena(
			arena,
			scopedTypeSym,
			rustNamedForSymbol(lang, scopedTypeSym),
			[]*Node{left, right},
			nil,
			0,
		)
		scoped.startByte = leftStart
		scoped.startPoint = left.startPoint
		scoped.endByte = rightEnd
		scoped.endPoint = right.endPoint
		return scoped, true
	}
	typeIdentifierSym, ok := symbolByName(lang, "type_identifier")
	if !ok {
		return nil, false
	}
	return newLeafNodeInArena(
		arena,
		typeIdentifierSym,
		rustNamedForSymbol(lang, typeIdentifierSym),
		start,
		end,
		advancePointByBytes(Point{}, source[:start]),
		advancePointByBytes(Point{}, source[:end]),
	), true
}

func rustBuildRecoveredFieldInitializerList(arena *nodeArena, source []byte, lang *Language, start, end uint32) (*Node, bool) {
	fieldListSym, ok := symbolByName(lang, "field_initializer_list")
	if !ok {
		return nil, false
	}
	start = rustSkipSpaceBytes(source, start)
	if start >= end || source[start] != '{' {
		return nil, false
	}
	closeBrace := rustFindMatchingDelimiter(source, int(start), '{', '}')
	if closeBrace < 0 || uint32(closeBrace) > end {
		return nil, false
	}
	var children []*Node
	for _, span := range rustSplitTopLevelCommaSpans(source, start+1, uint32(closeBrace)) {
		entry, ok := rustBuildRecoveredFieldEntry(arena, source, lang, span[0], span[1])
		if !ok {
			return nil, false
		}
		children = append(children, entry)
	}
	fieldList := newParentNodeInArena(
		arena,
		fieldListSym,
		rustNamedForSymbol(lang, fieldListSym),
		children,
		nil,
		0,
	)
	fieldList.startByte = start
	fieldList.startPoint = advancePointByBytes(Point{}, source[:start])
	fieldList.endByte = uint32(closeBrace + 1)
	fieldList.endPoint = advancePointByBytes(Point{}, source[:closeBrace+1])
	return fieldList, true
}

func rustBuildRecoveredFieldEntry(arena *nodeArena, source []byte, lang *Language, start, end uint32) (*Node, bool) {
	start, end = rustTrimSpaceBounds(source, start, end)
	if start >= end {
		return nil, false
	}
	if colonPos, ok := rustFindTopLevelByte(source, start, end, ':'); ok {
		fieldInitSym, ok := symbolByName(lang, "field_initializer")
		if !ok {
			return nil, false
		}
		key, ok := rustBuildRecoveredFieldKey(arena, source, lang, start, colonPos)
		if !ok {
			return nil, false
		}
		value, ok := rustBuildRecoveredValueNode(arena, source, lang, colonPos+1, end)
		if !ok {
			return nil, false
		}
		fieldInit := newParentNodeInArena(
			arena,
			fieldInitSym,
			rustNamedForSymbol(lang, fieldInitSym),
			[]*Node{key, value},
			nil,
			0,
		)
		fieldInit.startByte = key.startByte
		fieldInit.startPoint = key.startPoint
		fieldInit.endByte = value.endByte
		fieldInit.endPoint = value.endPoint
		return fieldInit, true
	}
	shorthandSym, ok := symbolByName(lang, "shorthand_field_initializer")
	if !ok {
		return nil, false
	}
	identifierSym, ok := symbolByName(lang, "identifier")
	if !ok {
		return nil, false
	}
	ident := newLeafNodeInArena(
		arena,
		identifierSym,
		rustNamedForSymbol(lang, identifierSym),
		start,
		end,
		advancePointByBytes(Point{}, source[:start]),
		advancePointByBytes(Point{}, source[:end]),
	)
	shorthand := newParentNodeInArena(
		arena,
		shorthandSym,
		rustNamedForSymbol(lang, shorthandSym),
		[]*Node{ident},
		nil,
		0,
	)
	shorthand.startByte = start
	shorthand.startPoint = ident.startPoint
	shorthand.endByte = end
	shorthand.endPoint = ident.endPoint
	return shorthand, true
}

func rustBuildRecoveredFieldKey(arena *nodeArena, source []byte, lang *Language, start, end uint32) (*Node, bool) {
	start, end = rustTrimSpaceBounds(source, start, end)
	if start >= end {
		return nil, false
	}
	if rustBytesAreIntegerLiteral(source[start:end]) {
		intSym, ok := symbolByName(lang, "integer_literal")
		if !ok {
			return nil, false
		}
		return newLeafNodeInArena(
			arena,
			intSym,
			rustNamedForSymbol(lang, intSym),
			start,
			end,
			advancePointByBytes(Point{}, source[:start]),
			advancePointByBytes(Point{}, source[:end]),
		), true
	}
	fieldIdentifierSym, ok := symbolByName(lang, "field_identifier")
	if !ok {
		return nil, false
	}
	return newLeafNodeInArena(
		arena,
		fieldIdentifierSym,
		rustNamedForSymbol(lang, fieldIdentifierSym),
		start,
		end,
		advancePointByBytes(Point{}, source[:start]),
		advancePointByBytes(Point{}, source[:end]),
	), true
}

func rustBuildRecoveredValueNode(arena *nodeArena, source []byte, lang *Language, start, end uint32) (*Node, bool) {
	start, end = rustTrimSpaceBounds(source, start, end)
	if start >= end {
		return nil, false
	}
	if source[start] == '"' && end > start+1 && source[end-1] == '"' {
		stringLiteralSym, ok := symbolByName(lang, "string_literal")
		if !ok {
			return nil, false
		}
		var children []*Node
		if stringContentSym, ok := symbolByName(lang, "string_content"); ok && end > start+2 {
			content := newLeafNodeInArena(
				arena,
				stringContentSym,
				rustNamedForSymbol(lang, stringContentSym),
				start+1,
				end-1,
				advancePointByBytes(Point{}, source[:start+1]),
				advancePointByBytes(Point{}, source[:end-1]),
			)
			children = []*Node{content}
		}
		lit := newParentNodeInArena(
			arena,
			stringLiteralSym,
			rustNamedForSymbol(lang, stringLiteralSym),
			children,
			nil,
			0,
		)
		lit.startByte = start
		lit.startPoint = advancePointByBytes(Point{}, source[:start])
		lit.endByte = end
		lit.endPoint = advancePointByBytes(Point{}, source[:end])
		return lit, true
	}
	if rustBytesAreFloatLiteral(source[start:end]) {
		floatSym, ok := symbolByName(lang, "float_literal")
		if !ok {
			return nil, false
		}
		return newLeafNodeInArena(
			arena,
			floatSym,
			rustNamedForSymbol(lang, floatSym),
			start,
			end,
			advancePointByBytes(Point{}, source[:start]),
			advancePointByBytes(Point{}, source[:end]),
		), true
	}
	if rustBytesAreIntegerLiteral(source[start:end]) {
		intSym, ok := symbolByName(lang, "integer_literal")
		if !ok {
			return nil, false
		}
		return newLeafNodeInArena(
			arena,
			intSym,
			rustNamedForSymbol(lang, intSym),
			start,
			end,
			advancePointByBytes(Point{}, source[:start]),
			advancePointByBytes(Point{}, source[:end]),
		), true
	}
	if openParen, ok := rustFindTopLevelByte(source, start, end, '('); ok {
		closeParen := rustFindMatchingDelimiter(source, int(openParen), '(', ')')
		if closeParen >= 0 && uint32(closeParen+1) == end {
			callSym, ok := symbolByName(lang, "call_expression")
			if !ok {
				return nil, false
			}
			argsSym, ok := symbolByName(lang, "arguments")
			if !ok {
				return nil, false
			}
			fnNode, ok := rustBuildRecoveredScopedIdentifierNode(arena, source, lang, start, openParen)
			if !ok {
				return nil, false
			}
			var argsChildren []*Node
			for _, span := range rustSplitTopLevelCommaSpans(source, openParen+1, uint32(closeParen)) {
				arg, ok := rustBuildRecoveredValueNode(arena, source, lang, span[0], span[1])
				if !ok {
					return nil, false
				}
				argsChildren = append(argsChildren, arg)
			}
			args := newParentNodeInArena(
				arena,
				argsSym,
				rustNamedForSymbol(lang, argsSym),
				argsChildren,
				nil,
				0,
			)
			args.startByte = openParen
			args.startPoint = advancePointByBytes(Point{}, source[:openParen])
			args.endByte = uint32(closeParen + 1)
			args.endPoint = advancePointByBytes(Point{}, source[:closeParen+1])

			call := newParentNodeInArena(
				arena,
				callSym,
				rustNamedForSymbol(lang, callSym),
				[]*Node{fnNode, args},
				nil,
				0,
			)
			call.startByte = start
			call.startPoint = advancePointByBytes(Point{}, source[:start])
			call.endByte = end
			call.endPoint = advancePointByBytes(Point{}, source[:end])
			return call, true
		}
	}
	identifierSym, ok := symbolByName(lang, "identifier")
	if !ok {
		return nil, false
	}
	return newLeafNodeInArena(
		arena,
		identifierSym,
		rustNamedForSymbol(lang, identifierSym),
		start,
		end,
		advancePointByBytes(Point{}, source[:start]),
		advancePointByBytes(Point{}, source[:end]),
	), true
}

func rustBuildRecoveredScopedIdentifierNode(arena *nodeArena, source []byte, lang *Language, start, end uint32) (*Node, bool) {
	start, end = rustTrimSpaceBounds(source, start, end)
	if start >= end {
		return nil, false
	}
	scopePos, ok := rustFindTopLevelDoubleColon(source, start, end)
	if !ok {
		identifierSym, ok := symbolByName(lang, "identifier")
		if !ok {
			return nil, false
		}
		return newLeafNodeInArena(
			arena,
			identifierSym,
			rustNamedForSymbol(lang, identifierSym),
			start,
			end,
			advancePointByBytes(Point{}, source[:start]),
			advancePointByBytes(Point{}, source[:end]),
		), true
	}
	scopedIdentifierSym, ok := symbolByName(lang, "scoped_identifier")
	if !ok {
		return nil, false
	}
	identifierSym, ok := symbolByName(lang, "identifier")
	if !ok {
		return nil, false
	}
	leftStart, leftEnd := rustTrimSpaceBounds(source, start, scopePos)
	rightStart, rightEnd := rustTrimSpaceBounds(source, scopePos+2, end)
	if leftStart >= leftEnd || rightStart >= rightEnd {
		return nil, false
	}
	left := newLeafNodeInArena(
		arena,
		identifierSym,
		rustNamedForSymbol(lang, identifierSym),
		leftStart,
		leftEnd,
		advancePointByBytes(Point{}, source[:leftStart]),
		advancePointByBytes(Point{}, source[:leftEnd]),
	)
	right := newLeafNodeInArena(
		arena,
		identifierSym,
		rustNamedForSymbol(lang, identifierSym),
		rightStart,
		rightEnd,
		advancePointByBytes(Point{}, source[:rightStart]),
		advancePointByBytes(Point{}, source[:rightEnd]),
	)
	scoped := newParentNodeInArena(
		arena,
		scopedIdentifierSym,
		rustNamedForSymbol(lang, scopedIdentifierSym),
		[]*Node{left, right},
		nil,
		0,
	)
	scoped.startByte = leftStart
	scoped.startPoint = left.startPoint
	scoped.endByte = rightEnd
	scoped.endPoint = right.endPoint
	return scoped, true
}

func rustSplitTopLevelCommaSpans(source []byte, start, end uint32) [][2]uint32 {
	start, end = rustTrimSpaceBounds(source, start, end)
	if start >= end {
		return nil
	}
	var spans [][2]uint32
	partStart := start
	braceDepth := 0
	parenDepth := 0
	bracketDepth := 0
	inString := false
	escaped := false
	for i := start; i < end; i++ {
		b := source[i]
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
		case ',':
			if braceDepth == 0 && parenDepth == 0 && bracketDepth == 0 {
				a, b := rustTrimSpaceBounds(source, partStart, i)
				if a < b {
					spans = append(spans, [2]uint32{a, b})
				}
				partStart = i + 1
			}
		}
	}
	a, b := rustTrimSpaceBounds(source, partStart, end)
	if a < b {
		spans = append(spans, [2]uint32{a, b})
	}
	return spans
}

func rustBuildRecoveredTokenTree(arena *nodeArena, source []byte, lang *Language, start, end uint32) (*Node, bool) {
	if lang == nil || lang.Name != "rust" || start >= end || int(end) > len(source) || end-start < 2 {
		return nil, false
	}
	open := source[start]
	close := source[end-1]
	switch {
	case open == '(' && close == ')':
	case open == '[' && close == ']':
	case open == '{' && close == '}':
	default:
		return nil, false
	}

	tokenTreeSym, ok := symbolByName(lang, "token_tree")
	if !ok {
		return nil, false
	}
	children, ok := rustBuildRecoveredTokenTreeChildren(arena, source, lang, start+1, end-1)
	if !ok {
		return nil, false
	}
	node := newParentNodeInArena(arena, tokenTreeSym, rustNamedForSymbol(lang, tokenTreeSym), children, nil, 0)
	node.startByte = start
	node.endByte = end
	node.startPoint = advancePointByBytes(Point{}, source[:start])
	node.endPoint = advancePointByBytes(Point{}, source[:end])
	node.setHasError(false)
	populateParentNode(node, node.children)
	return node, true
}

func rustBuildRecoveredTokenTreeChildren(arena *nodeArena, source []byte, lang *Language, start, end uint32) ([]*Node, bool) {
	if start > end || int(end) > len(source) {
		return nil, false
	}
	identifierSym, ok := symbolByName(lang, "identifier")
	if !ok {
		return nil, false
	}
	charLiteralSym, hasCharLiteral := symbolByName(lang, "char_literal")

	var children []*Node
	for i := start; i < end; {
		switch c := source[i]; {
		case rustIsSpaceByte(c):
			i++
		case c == '/' && i+1 < end && source[i+1] == '/':
			commentEnd := i + 2
			for commentEnd < end && source[commentEnd] != '\n' {
				commentEnd++
			}
			if comment, ok := rustBuildRecoveredTriviaNode(arena, source, lang, i, commentEnd, "line_comment"); ok {
				children = append(children, comment)
			}
			i = commentEnd
		case c == '/' && i+1 < end && source[i+1] == '*':
			commentEnd := rustFindBlockCommentEnd(source, i+2, end)
			if commentEnd <= i+1 {
				return nil, false
			}
			if comment, ok := rustBuildRecoveredTriviaNode(arena, source, lang, i, commentEnd, "block_comment"); ok {
				children = append(children, comment)
			}
			i = commentEnd
		case c == '\'':
			if litEnd, ok := rustFindCharLiteralEnd(source, i, end); ok {
				if hasCharLiteral {
					children = append(children, newLeafNodeInArena(
						arena,
						charLiteralSym,
						rustNamedForSymbol(lang, charLiteralSym),
						i,
						litEnd,
						advancePointByBytes(Point{}, source[:i]),
						advancePointByBytes(Point{}, source[:litEnd]),
					))
				}
				i = litEnd
				continue
			}
			nameStart := i + 1
			nameEnd := nameStart
			for nameEnd < end && rustIsIdentByte(source[nameEnd]) {
				nameEnd++
			}
			if nameEnd > nameStart {
				children = append(children, newLeafNodeInArena(
					arena,
					identifierSym,
					rustNamedForSymbol(lang, identifierSym),
					nameStart,
					nameEnd,
					advancePointByBytes(Point{}, source[:nameStart]),
					advancePointByBytes(Point{}, source[:nameEnd]),
				))
				i = nameEnd
				continue
			}
			i++
		case c == '$':
			next := i + 1
			if next < end {
				switch source[next] {
				case '(', '[', '{':
					closePos := rustFindMatchingDelimiter(source, int(next), source[next], rustMatchingDelimiter(source[next]))
					if closePos >= 0 && uint32(closePos) < end {
						nested, ok := rustBuildRecoveredTokenTree(arena, source, lang, next, uint32(closePos+1))
						if !ok {
							return nil, false
						}
						children = append(children, nested)
						i = uint32(closePos + 1)
						continue
					}
				}
			}
			nameStart := next
			nameEnd := nameStart
			for nameEnd < end && rustIsIdentByte(source[nameEnd]) {
				nameEnd++
			}
			if nameEnd > nameStart {
				children = append(children, newLeafNodeInArena(
					arena,
					identifierSym,
					rustNamedForSymbol(lang, identifierSym),
					nameStart,
					nameEnd,
					advancePointByBytes(Point{}, source[:nameStart]),
					advancePointByBytes(Point{}, source[:nameEnd]),
				))
				i = nameEnd
				if i+1 < end && source[i] == ':' && rustIsIdentByte(source[i+1]) {
					fragStart := i + 1
					fragEnd := fragStart
					for fragEnd < end && rustIsIdentByte(source[fragEnd]) {
						fragEnd++
					}
					children = append(children, newLeafNodeInArena(
						arena,
						identifierSym,
						rustNamedForSymbol(lang, identifierSym),
						fragStart,
						fragEnd,
						advancePointByBytes(Point{}, source[:fragStart]),
						advancePointByBytes(Point{}, source[:fragEnd]),
					))
					i = fragEnd
				}
				continue
			}
			i++
		case rustIsIdentByte(c):
			nameStart := i
			nameEnd := i + 1
			for nameEnd < end && rustIsIdentByte(source[nameEnd]) {
				nameEnd++
			}
			children = append(children, newLeafNodeInArena(
				arena,
				identifierSym,
				rustNamedForSymbol(lang, identifierSym),
				nameStart,
				nameEnd,
				advancePointByBytes(Point{}, source[:nameStart]),
				advancePointByBytes(Point{}, source[:nameEnd]),
			))
			i = nameEnd
		case c == '(' || c == '[' || c == '{':
			closePos := rustFindMatchingDelimiter(source, int(i), c, rustMatchingDelimiter(c))
			if closePos < 0 || uint32(closePos) >= end {
				return nil, false
			}
			nested, ok := rustBuildRecoveredTokenTree(arena, source, lang, i, uint32(closePos+1))
			if !ok {
				return nil, false
			}
			children = append(children, nested)
			i = uint32(closePos + 1)
		default:
			i++
		}
	}
	return children, true
}

func rustBuildRecoveredTriviaNode(arena *nodeArena, source []byte, lang *Language, start, end uint32, typeName string) (*Node, bool) {
	sym, ok := symbolByName(lang, typeName)
	if !ok {
		return nil, false
	}
	tokenName := ""
	switch typeName {
	case "line_comment":
		if node, ok := rustBuildRecoveredDocLineCommentNode(arena, source, lang, start, end); ok {
			return node, true
		}
		tokenName = "//"
	case "block_comment":
		tokenName = "/*"
	}
	if tokenName != "" && start+uint32(len(tokenName)) <= end {
		if tokenSym, ok := symbolByName(lang, tokenName); ok {
			tokenEnd := start + uint32(len(tokenName))
			startPoint := advancePointByBytes(Point{}, source[:start])
			tokenEndPoint := advancePointByBytes(startPoint, source[start:tokenEnd])
			endPoint := advancePointByBytes(tokenEndPoint, source[tokenEnd:end])
			token := newLeafNodeInArena(
				arena,
				tokenSym,
				rustNamedForSymbol(lang, tokenSym),
				start,
				tokenEnd,
				startPoint,
				tokenEndPoint,
			)
			node := newParentNodeInArena(
				arena,
				sym,
				rustNamedForSymbol(lang, sym),
				[]*Node{token},
				nil,
				0,
			)
			node.startByte = start
			node.startPoint = token.startPoint
			node.endByte = end
			node.endPoint = endPoint
			node.setExtra(true)
			return node, true
		}
	}
	startPoint := advancePointByBytes(Point{}, source[:start])
	endPoint := advancePointByBytes(startPoint, source[start:end])
	node := newLeafNodeInArena(
		arena,
		sym,
		rustNamedForSymbol(lang, sym),
		start,
		end,
		startPoint,
		endPoint,
	)
	node.setExtra(true)
	return node, true
}

func rustBuildRecoveredDocLineCommentNode(arena *nodeArena, source []byte, lang *Language, start, end uint32) (*Node, bool) {
	if !rustLineCommentIsDoc(source, start, end) {
		return nil, false
	}
	lineCommentSym, ok := symbolByName(lang, "line_comment")
	if !ok {
		return nil, false
	}
	slashSlashSym, ok := symbolByName(lang, "//")
	if !ok {
		return nil, false
	}
	markerName := "outer_doc_comment_marker"
	markerToken := "/"
	if source[start+2] == '!' {
		markerName = "inner_doc_comment_marker"
		markerToken = "!"
	}
	markerSym, ok := symbolByName(lang, markerName)
	if !ok {
		return nil, false
	}
	markerTokenSym, ok := symbolByName(lang, markerToken)
	if !ok {
		return nil, false
	}
	docCommentSym, ok := symbolByName(lang, "doc_comment")
	if !ok {
		return nil, false
	}

	slashSlashEnd := start + 2
	markerEnd := start + 3
	startPoint := advancePointByBytes(Point{}, source[:start])
	slashSlashEndPoint := advancePointByBytes(startPoint, source[start:slashSlashEnd])
	markerEndPoint := advancePointByBytes(slashSlashEndPoint, source[slashSlashEnd:markerEnd])
	endPoint := advancePointByBytes(markerEndPoint, source[markerEnd:end])
	slashSlash := newLeafNodeInArena(
		arena,
		slashSlashSym,
		rustNamedForSymbol(lang, slashSlashSym),
		start,
		slashSlashEnd,
		startPoint,
		slashSlashEndPoint,
	)
	markerTokenNode := newLeafNodeInArena(
		arena,
		markerTokenSym,
		rustNamedForSymbol(lang, markerTokenSym),
		slashSlashEnd,
		markerEnd,
		slashSlashEndPoint,
		markerEndPoint,
	)
	marker := newParentNodeInArena(
		arena,
		markerSym,
		rustNamedForSymbol(lang, markerSym),
		[]*Node{markerTokenNode},
		nil,
		0,
	)
	marker.startByte = slashSlashEnd
	marker.startPoint = markerTokenNode.startPoint
	marker.endByte = markerEnd
	marker.endPoint = markerTokenNode.endPoint

	doc := newLeafNodeInArena(
		arena,
		docCommentSym,
		rustNamedForSymbol(lang, docCommentSym),
		markerEnd,
		end,
		markerEndPoint,
		endPoint,
	)
	node := newParentNodeInArena(
		arena,
		lineCommentSym,
		rustNamedForSymbol(lang, lineCommentSym),
		[]*Node{slashSlash, marker, doc},
		rustDocCommentFieldIDs(arena, lang, markerName),
		0,
	)
	node.startByte = start
	node.startPoint = slashSlash.startPoint
	node.endByte = end
	node.endPoint = doc.endPoint
	node.setExtra(true)
	return node, true
}

func rustDocCommentFieldIDs(arena *nodeArena, lang *Language, markerName string) []FieldID {
	fieldName := "outer"
	if markerName == "inner_doc_comment_marker" {
		fieldName = "inner"
	}
	fid, ok := lang.FieldByName(fieldName)
	if !ok {
		return nil
	}
	docFID, _ := lang.FieldByName("doc")
	return cloneFieldIDSliceInArena(arena, []FieldID{0, fid, docFID})
}

func rustFindBlockCommentEnd(source []byte, start, end uint32) uint32 {
	depth := 1
	for i := start; i+1 < end; i++ {
		switch {
		case source[i] == '/' && source[i+1] == '*':
			depth++
			i++
		case source[i] == '*' && source[i+1] == '/':
			depth--
			i++
			if depth == 0 {
				return i + 1
			}
		}
	}
	return 0
}

func rustFindCharLiteralEnd(source []byte, start, end uint32) (uint32, bool) {
	if start >= end || source[start] != '\'' {
		return 0, false
	}
	for i := start + 1; i < end; i++ {
		switch source[i] {
		case '\\':
			i++
		case '\'':
			return i + 1, true
		case '\n', '\r':
			return 0, false
		}
	}
	return 0, false
}

func rustMatchingDelimiter(open byte) byte {
	switch open {
	case '(':
		return ')'
	case '[':
		return ']'
	case '{':
		return '}'
	default:
		return 0
	}
}

func rustFindTopLevelDoubleColon(source []byte, start, end uint32) (uint32, bool) {
	if end <= start+1 {
		return 0, false
	}
	for i := start; i+1 < end; i++ {
		if source[i] == ':' && source[i+1] == ':' {
			return i, true
		}
	}
	return 0, false
}

func rustFindTopLevelByte(source []byte, start, end uint32, target byte) (uint32, bool) {
	braceDepth := 0
	parenDepth := 0
	bracketDepth := 0
	inString := false
	escaped := false
	for i := start; i < end; i++ {
		b := source[i]
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
		if b == target && braceDepth == 0 && parenDepth == 0 && bracketDepth == 0 {
			return i, true
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
	return 0, false
}

func rustBytesAreIntegerLiteral(b []byte) bool {
	if len(b) == 0 {
		return false
	}
	for _, c := range b {
		if (c < '0' || c > '9') && c != '_' {
			return false
		}
	}
	return true
}

func rustBytesAreFloatLiteral(b []byte) bool {
	if len(b) == 0 {
		return false
	}
	sawDot := false
	for _, c := range b {
		switch {
		case c >= '0' && c <= '9':
		case c == '_':
		case c == '.':
			if sawDot {
				return false
			}
			sawDot = true
		default:
			return false
		}
	}
	return sawDot
}

func normalizeRustSourceFileRoot(root *Node, source []byte, lang *Language) {
	if root == nil || lang == nil || lang.Name != "rust" || root.Type(lang) != "ERROR" {
		return
	}
	sourceFileSym, ok := symbolByName(lang, "source_file")
	if !ok || !rustRootLooksLikeTopLevel(root, lang) {
		return
	}
	retagResultRootAndRefreshError(root, sourceFileSym, rustNamedForSymbol(lang, sourceFileSym))
	if !root.hasError() && root.endByte < uint32(len(source)) && bytesAreTrivia(source[root.endByte:]) {
		extendNodeEndTo(root, uint32(len(source)), source)
	}
}

func rustRootLooksLikeTopLevel(root *Node, lang *Language) bool {
	if root == nil || lang == nil || len(root.children) == 0 {
		return false
	}
	sawTopLevel := false
	for _, child := range root.children {
		if child == nil {
			continue
		}
		switch child.Type(lang) {
		case "line_comment",
			"block_comment",
			"inner_attribute_item",
			"attribute_item",
			"extern_crate_declaration",
			"use_declaration",
			"expression_statement",
			"let_declaration",
			"function_item",
			"struct_item",
			"enum_item",
			"const_item",
			"static_item",
			"trait_item",
			"impl_item",
			"type_item",
			"union_item",
			"macro_definition",
			"macro_invocation",
			"mod_item",
			"foreign_mod_item":
			sawTopLevel = true
		default:
			return false
		}
	}
	return sawTopLevel
}

func rustNamedForSymbol(lang *Language, sym Symbol) bool {
	if lang != nil && int(sym) < len(lang.SymbolNames) {
		switch lang.SymbolNames[sym] {
		case "source_file",
			"identifier",
			"expression_statement",
			"let_declaration",
			"closure_expression",
			"closure_parameters",
			"struct_expression",
			"field_initializer_list",
			"field_initializer",
			"shorthand_field_initializer",
			"field_identifier",
			"float_literal",
			"integer_literal",
			"string_literal",
			"string_content",
			"call_expression",
			"arguments",
			"scoped_identifier",
			"scoped_type_identifier",
			"function_item",
			"parameters",
			"parameter",
			"abstract_type",
			"type_parameters",
			"lifetime_parameter",
			"lifetime",
			"generic_type",
			"type_identifier",
			"type_arguments",
			"block":
			return true
		}
	}
	return symbolIsNamed(lang, sym)
}

func rustTrimSpaceBounds(source []byte, start, end uint32) (uint32, uint32) {
	for start < end && rustIsSpaceByte(source[start]) {
		start++
	}
	for end > start && rustIsSpaceByte(source[end-1]) {
		end--
	}
	return start, end
}

func rustSkipSpaceBytes(source []byte, pos uint32) uint32 {
	for pos < uint32(len(source)) && rustIsSpaceByte(source[pos]) {
		pos++
	}
	return pos
}

func rustIsSpaceByte(b byte) bool {
	switch b {
	case ' ', '\t', '\n', '\r':
		return true
	default:
		return false
	}
}

func rustHasPrefixAt(source []byte, pos uint32, prefix string) bool {
	if int(pos)+len(prefix) > len(source) {
		return false
	}
	return string(source[pos:uint32(int(pos)+len(prefix))]) == prefix
}

func rustFindMatchingDelimiter(source []byte, start int, open, close byte) int {
	if start < 0 || start >= len(source) || source[start] != open {
		return -1
	}
	depth := 0
	for i := start; i < len(source); i++ {
		switch source[i] {
		case open:
			depth++
		case close:
			depth--
			if depth == 0 {
				return i
			}
		}
	}
	return -1
}

func rustIsIdentByte(b byte) bool {
	return b == '_' || b >= 'a' && b <= 'z' || b >= 'A' && b <= 'Z' || b >= '0' && b <= '9'
}

func rustFragmentSpecifierFollowsColon(meta, colon, frag *Node, source []byte) bool {
	if meta == nil || colon == nil || frag == nil || len(source) == 0 {
		return false
	}
	if int(meta.endByte) > len(source) || int(frag.endByte) > len(source) {
		return false
	}
	if meta.endByte > frag.startByte || colon.startByte > colon.endByte {
		return false
	}
	betweenMetaAndFrag := strings.TrimSpace(string(source[meta.endByte:frag.startByte]))
	return strings.Contains(betweenMetaAndFrag, ":")
}
