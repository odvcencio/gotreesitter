package gotreesitter

import "bytes"

func rustRecoverRustSpecialCharactersExpressionNodeFromRange(source []byte, start, end uint32, p *Parser, arena *nodeArena) (*Node, bool) {
	if p == nil || p.language == nil || start >= end || int(end) > len(source) {
		return nil, false
	}
	start, end = rustTrimSpaceBounds(source, start, end)
	if start >= end || source[start] != '!' {
		return nil, false
	}
	innerStart := rustSkipSpaceBytes(source, start+1)
	if innerStart >= end || source[innerStart] != '(' || source[end-1] != ')' {
		return nil, false
	}
	innerClose := rustFindMatchingDelimiter(source, int(innerStart), '(', ')')
	if innerClose < 0 || uint32(innerClose+1) != end {
		return nil, false
	}
	binaryStart, binaryEnd := rustTrimSpaceBounds(source, innerStart+1, uint32(innerClose))
	eqPos, ok := rustFindTopLevelDoubleByte(source, binaryStart, binaryEnd, '=', '=')
	if !ok {
		return nil, false
	}
	left, ok := rustRecoverRustSpecialCharactersCallExpressionNodeFromRange(source, binaryStart, eqPos, p, arena)
	if !ok {
		return nil, false
	}
	right, ok := rustRecoverRustExpressionNodeFromRange(source, eqPos+2, binaryEnd, p, arena)
	if !ok {
		return nil, false
	}
	binarySym, ok := symbolByName(p.language, "binary_expression")
	if !ok {
		return nil, false
	}
	parenthesizedSym, ok := symbolByName(p.language, "parenthesized_expression")
	if !ok {
		return nil, false
	}
	unarySym, ok := symbolByName(p.language, "unary_expression")
	if !ok {
		return nil, false
	}
	binary := newParentNodeInArena(
		arena,
		binarySym,
		rustNamedForSymbol(p.language, binarySym),
		[]*Node{left, right},
		nil,
		0,
	)
	binary.startByte = binaryStart
	binary.startPoint = advancePointByBytes(Point{}, source[:binaryStart])
	binary.endByte = binaryEnd
	binary.endPoint = advancePointByBytes(Point{}, source[:binaryEnd])
	paren := newParentNodeInArena(
		arena,
		parenthesizedSym,
		rustNamedForSymbol(p.language, parenthesizedSym),
		[]*Node{binary},
		nil,
		0,
	)
	paren.startByte = innerStart
	paren.startPoint = advancePointByBytes(Point{}, source[:innerStart])
	paren.endByte = end
	paren.endPoint = advancePointByBytes(Point{}, source[:end])
	unary := newParentNodeInArena(
		arena,
		unarySym,
		rustNamedForSymbol(p.language, unarySym),
		[]*Node{paren},
		nil,
		0,
	)
	unary.startByte = start
	unary.startPoint = advancePointByBytes(Point{}, source[:start])
	unary.endByte = end
	unary.endPoint = advancePointByBytes(Point{}, source[:end])
	return unary, true
}

func rustRecoverRustSpecialCharactersCallExpressionNodeFromRange(source []byte, start, end uint32, p *Parser, arena *nodeArena) (*Node, bool) {
	if p == nil || p.language == nil || start >= end || int(end) > len(source) {
		return nil, false
	}
	start, end = rustTrimSpaceBounds(source, start, end)
	if start >= end || source[start] != '(' {
		return nil, false
	}
	calleeClose := rustFindMatchingDelimiter(source, int(start), '(', ')')
	if calleeClose < 0 || uint32(calleeClose+1) >= end {
		return nil, false
	}
	argsStart := rustSkipSpaceBytes(source, uint32(calleeClose+1))
	if argsStart >= end || source[argsStart] != '(' {
		return nil, false
	}
	argsClose := rustFindMatchingDelimiter(source, int(argsStart), '(', ')')
	if argsClose < 0 || uint32(argsClose+1) != end {
		return nil, false
	}
	callee, ok := rustRecoverRustSpecialCharactersCalleeNodeFromRange(source, start, uint32(calleeClose+1), p, arena)
	if !ok {
		return nil, false
	}
	args, ok := rustRecoverRustSpecialCharactersArgumentsNodeFromRange(source, argsStart, uint32(argsClose+1), p, arena)
	if !ok {
		return nil, false
	}
	callSym, ok := symbolByName(p.language, "call_expression")
	if !ok {
		return nil, false
	}
	call := newParentNodeInArena(
		arena,
		callSym,
		rustNamedForSymbol(p.language, callSym),
		[]*Node{callee, args},
		nil,
		0,
	)
	call.startByte = start
	call.startPoint = advancePointByBytes(Point{}, source[:start])
	call.endByte = end
	call.endPoint = advancePointByBytes(Point{}, source[:end])
	return call, true
}

func rustRecoverRustSpecialCharactersCalleeNodeFromRange(source []byte, start, end uint32, p *Parser, arena *nodeArena) (*Node, bool) {
	if p == nil || p.language == nil || start >= end || int(end) > len(source) {
		return nil, false
	}
	start, end = rustTrimSpaceBounds(source, start, end)
	if start >= end || source[start] != '(' || source[end-1] != ')' {
		return nil, false
	}
	closure, ok := rustRecoverRustClosureExpressionNodeFromRange(source, start+1, end-1, p, arena)
	if !ok {
		return nil, false
	}
	parenthesizedSym, ok := symbolByName(p.language, "parenthesized_expression")
	if !ok {
		return nil, false
	}
	paren := newParentNodeInArena(
		arena,
		parenthesizedSym,
		rustNamedForSymbol(p.language, parenthesizedSym),
		[]*Node{closure},
		nil,
		0,
	)
	paren.startByte = start
	paren.startPoint = advancePointByBytes(Point{}, source[:start])
	paren.endByte = end
	paren.endPoint = advancePointByBytes(Point{}, source[:end])
	return paren, true
}

func rustRecoverRustSpecialCharactersArgumentsNodeFromRange(source []byte, start, end uint32, p *Parser, arena *nodeArena) (*Node, bool) {
	if p == nil || p.language == nil || start >= end || int(end) > len(source) {
		return nil, false
	}
	start, end = rustTrimSpaceBounds(source, start, end)
	if start >= end || source[start] != '(' || source[end-1] != ')' {
		return nil, false
	}
	contentStart, contentEnd := rustTrimSpaceBounds(source, start+1, end-1)
	if contentStart >= contentEnd || source[contentStart] != '(' {
		return nil, false
	}
	tupleClose := rustFindMatchingDelimiter(source, int(contentStart), '(', ')')
	if tupleClose < 0 {
		return nil, false
	}
	tupleExpr, ok := rustRecoverRustExpressionNodeFromRange(source, contentStart, uint32(tupleClose+1), p, arena)
	if !ok {
		return nil, false
	}
	cursor := rustSkipSpaceBytes(source, uint32(tupleClose+1))
	var comment *Node
	if cursor+1 < contentEnd && source[cursor] == '/' && source[cursor+1] == '*' {
		commentEnd := rustFindBlockCommentEnd(source, cursor+2, contentEnd)
		if commentEnd <= cursor+1 {
			return nil, false
		}
		comment, ok = rustBuildRecoveredTriviaNode(arena, source, p.language, cursor, commentEnd, "block_comment")
		if !ok {
			return nil, false
		}
		cursor = rustSkipSpaceBytes(source, commentEnd)
	}
	if cursor >= contentEnd || source[cursor] != ',' {
		return nil, false
	}
	blockExpr, ok := rustRecoverRustExpressionNodeFromRange(source, cursor+1, contentEnd, p, arena)
	if !ok {
		return nil, false
	}
	argsSym, ok := symbolByName(p.language, "arguments")
	if !ok {
		return nil, false
	}
	children := []*Node{tupleExpr}
	if comment != nil {
		children = append(children, comment)
	}
	children = append(children, blockExpr)
	args := newParentNodeInArena(
		arena,
		argsSym,
		rustNamedForSymbol(p.language, argsSym),
		children,
		nil,
		0,
	)
	args.startByte = start
	args.startPoint = advancePointByBytes(Point{}, source[:start])
	args.endByte = end
	args.endPoint = advancePointByBytes(Point{}, source[:end])
	return args, true
}

func rustSplitRustMatchArmSpans(source []byte, start, end uint32) [][2]uint32 {
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

func rustFindTopLevelFatArrow(source []byte, start, end uint32) (uint32, bool) {
	braceDepth := 0
	parenDepth := 0
	bracketDepth := 0
	inString := false
	escaped := false
	inLineComment := false
	inBlockComment := false
	for i := start; i+1 < end; i++ {
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
		if b == '=' && source[i+1] == '>' && braceDepth == 0 && parenDepth == 0 && bracketDepth == 0 {
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

func rustFindTopLevelDoubleByte(source []byte, start, end uint32, left, right byte) (uint32, bool) {
	braceDepth := 0
	parenDepth := 0
	bracketDepth := 0
	inString := false
	escaped := false
	inLineComment := false
	inBlockComment := false
	for i := start; i+1 < end; i++ {
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
		if b == left && source[i+1] == right && braceDepth == 0 && parenDepth == 0 && bracketDepth == 0 {
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

func rustFindTopLevelKeyword(source []byte, start, end uint32, kw string) (uint32, bool) {
	if len(kw) == 0 || start >= end {
		return 0, false
	}
	braceDepth := 0
	parenDepth := 0
	bracketDepth := 0
	inString := false
	escaped := false
	for i := start; i+uint32(len(kw)) <= end; i++ {
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
		if braceDepth == 0 && parenDepth == 0 && bracketDepth == 0 && bytes.HasPrefix(source[i:end], []byte(kw)) {
			beforeOK := i == start || !rustIsIdentByte(source[i-1])
			after := i + uint32(len(kw))
			afterOK := after >= end || !rustIsIdentByte(source[after])
			if beforeOK && afterOK {
				return i, true
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
	return 0, false
}

func rustBytesAreIdentifier(b []byte) bool {
	if len(b) == 0 {
		return false
	}
	for _, c := range b {
		if !rustIsIdentByte(c) {
			return false
		}
	}
	return true
}
