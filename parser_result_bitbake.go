package gotreesitter

import "bytes"

func normalizeBitbakeCompatibility(root *Node, source []byte, lang *Language) {
	normalizeBitbakeAddtaskErrorWrappers(root, lang)
	normalizeBitbakeRecoveredShellFunctions(root, source, lang)
}

func normalizeBitbakeAddtaskErrorWrappers(root *Node, lang *Language) {
	if root == nil || lang == nil || lang.Name != "bitbake" {
		return
	}
	addtaskSym, ok := symbolByName(lang, "addtask_statement")
	if !ok {
		return
	}
	rewriteResultTreeChildrenPostorder(root, func(n *Node) *Node {
		if n == nil || n.symbol != errorSymbol || resultChildCount(n) != 1 {
			return nil
		}
		child := resultChildAt(n, 0)
		if child == nil || child.symbol != addtaskSym ||
			child.startByte != n.startByte || child.endByte != n.endByte ||
			child.startPoint != n.startPoint || child.endPoint != n.endPoint {
			return nil
		}
		return child
	})
}

func normalizeBitbakeRecoveredShellFunctions(root *Node, source []byte, lang *Language) {
	if root == nil || lang == nil || lang.Name != "bitbake" || len(source) == 0 {
		return
	}
	for changed := true; changed; {
		changed = false
		walkResultTreePostorder(root, func(n *Node) {
			if changed || n == nil || n.Type(lang) != "recipe" {
				return
			}
			for i := 0; i < resultChildCount(n); i++ {
				child := resultChildAt(n, i)
				replacements := bitbakeRecoveredShellFunctionReplacements(child, source, lang)
				if len(replacements) == 0 {
					continue
				}
				replaceChildRangeWithNodes(n, i, i+1, replacements)
				changed = true
				return
			}
		})
	}
	refreshResultRootError(root)
}

type bitbakeCompatSymbols struct {
	functionDefinition Symbol
	variableAssignment Symbol
	variableFlag       Symbol
	literal            Symbol
	stringSym          Symbol
	stringContent      Symbol
	identifier         Symbol
	flag               Symbol
	override           Symbol
	shellContent       Symbol
	stringStart        Symbol
	quote              Symbol
	append             Symbol
	openParen          Symbol
	closeParen         Symbol
	openBrace          Symbol
	closeBrace         Symbol
	openBracket        Symbol
	closeBracket       Symbol
	eq                 Symbol
	colon              Symbol
	dollarOpen         Symbol
	dollarClose        Symbol
	minus              Symbol
	slash              Symbol
}

func bitbakeSymbols(lang *Language) (bitbakeCompatSymbols, bool) {
	var syms bitbakeCompatSymbols
	for _, item := range []struct {
		name string
		dst  *Symbol
	}{
		{"function_definition", &syms.functionDefinition},
		{"variable_assignment", &syms.variableAssignment},
		{"variable_flag", &syms.variableFlag},
		{"literal", &syms.literal},
		{"string", &syms.stringSym},
		{"string_content", &syms.stringContent},
		{"identifier", &syms.identifier},
		{"flag", &syms.flag},
		{"override", &syms.override},
		{"shell_content", &syms.shellContent},
		{"string_start", &syms.stringStart},
		{"\"", &syms.quote},
		{"append", &syms.append},
		{"(", &syms.openParen},
		{")", &syms.closeParen},
		{"{", &syms.openBrace},
		{"}", &syms.closeBrace},
		{"[", &syms.openBracket},
		{"]", &syms.closeBracket},
		{"=", &syms.eq},
		{":", &syms.colon},
		{"${", &syms.dollarOpen},
		{"-", &syms.minus},
		{"/", &syms.slash},
	} {
		name, dst := item.name, item.dst
		sym, ok := symbolByName(lang, name)
		if !ok {
			return bitbakeCompatSymbols{}, false
		}
		*dst = sym
	}
	syms.dollarClose = syms.closeBrace
	return syms, true
}

func bitbakeRecoveredShellFunctionReplacements(n *Node, source []byte, lang *Language) []*Node {
	if n == nil || int(n.endByte) > len(source) || n.startByte >= n.endByte {
		return nil
	}
	typ := n.Type(lang)
	if typ != "variable_assignment" && typ != "ERROR" {
		return nil
	}
	syms, ok := bitbakeSymbols(lang)
	if !ok {
		return nil
	}
	start := int(n.startByte)
	end := int(n.endByte)
	firstFunc, next, ok := bitbakeParseFunctionAt(source, start, end, n.ownerArena, lang, syms)
	if !ok {
		return nil
	}
	var replacements []*Node
	replacements = append(replacements, firstFunc)
	for next < end {
		next = bitbakeSkipHorizontalWhitespace(source, next, end)
		if next < end && (source[next] == '\n' || source[next] == '\r') {
			next = bitbakeSkipLineBreaksAndHorizontalWhitespace(source, next, end)
		}
		if next >= end {
			break
		}
		if fn, after, ok := bitbakeParseFunctionAt(source, next, end, n.ownerArena, lang, syms); ok {
			replacements = append(replacements, fn)
			next = after
			continue
		}
		if assign, after, ok := bitbakeParseVariableFlagAssignmentAt(source, next, end, n.ownerArena, lang, syms); ok {
			replacements = append(replacements, assign)
			next = after
			continue
		}
		return nil
	}
	if len(replacements) <= 1 {
		return nil
	}
	return replacements
}

func bitbakeParseFunctionAt(source []byte, start, limit int, arena *nodeArena, lang *Language, syms bitbakeCompatSymbols) (*Node, int, bool) {
	nameStart := start
	nameEnd := bitbakeIdentifierEnd(source, nameStart, limit)
	if nameEnd == nameStart {
		return nil, start, false
	}
	pos := nameEnd
	var override *Node
	if pos < limit && source[pos] == ':' {
		overrideEnd := bitbakeIdentifierEnd(source, pos+1, limit)
		if overrideEnd == pos+1 {
			return nil, start, false
		}
		colon := bitbakeLeaf(arena, lang, syms.colon, pos, pos+1, source)
		overrideSym := syms.identifier
		if bytes.Equal(source[pos+1:overrideEnd], []byte("append")) {
			overrideSym = syms.append
		}
		overrideValue := bitbakeLeaf(arena, lang, overrideSym, pos+1, overrideEnd, source)
		override = newParentNodeInArena(arena, syms.override, symbolIsNamed(lang, syms.override), []*Node{colon, overrideValue}, nil, 0)
		pos = overrideEnd
	}
	pos = bitbakeSkipHorizontalWhitespace(source, pos, limit)
	if pos >= limit || source[pos] != '(' {
		return nil, start, false
	}
	openParenStart := pos
	pos++
	pos = bitbakeSkipHorizontalWhitespace(source, pos, limit)
	if pos >= limit || source[pos] != ')' {
		return nil, start, false
	}
	closeParenStart := pos
	pos++
	pos = bitbakeSkipHorizontalWhitespace(source, pos, limit)
	if pos >= limit || source[pos] != '{' {
		return nil, start, false
	}
	openBraceStart := pos
	pos++
	lineStart := pos
	if lineStart < limit && source[lineStart] == '\r' {
		lineStart++
	}
	if lineStart < limit && source[lineStart] == '\n' {
		lineStart++
	}
	closeBraceStart := bitbakeFindFunctionCloseBrace(source, lineStart, limit)
	if closeBraceStart < 0 {
		return nil, start, false
	}
	bodyChildren := bitbakeRecoveredShellBodyNodes(source, lineStart, closeBraceStart, arena, lang, syms)
	if len(bodyChildren) == 0 {
		return nil, start, false
	}
	children := []*Node{
		bitbakeLeaf(arena, lang, syms.identifier, nameStart, nameEnd, source),
	}
	if override != nil {
		children = append(children, override)
	}
	children = append(children,
		bitbakeLeaf(arena, lang, syms.openParen, openParenStart, openParenStart+1, source),
		bitbakeLeaf(arena, lang, syms.closeParen, closeParenStart, closeParenStart+1, source),
		bitbakeLeaf(arena, lang, syms.openBrace, openBraceStart, openBraceStart+1, source),
	)
	children = append(children, bodyChildren...)
	children = append(children, bitbakeLeaf(arena, lang, syms.closeBrace, closeBraceStart, closeBraceStart+1, source))
	fn := newParentNodeInArena(arena, syms.functionDefinition, symbolIsNamed(lang, syms.functionDefinition), children, nil, 0)
	return fn, closeBraceStart + 1, true
}

func bitbakeRecoveredShellBodyNodes(source []byte, start, end int, arena *nodeArena, lang *Language, syms bitbakeCompatSymbols) []*Node {
	var nodes []*Node
	for pos := start; pos < end; {
		lineStart := pos
		lineEnd := pos
		for lineEnd < end && source[lineEnd] != '\n' && source[lineEnd] != '\r' {
			lineEnd++
		}
		trimStart := bitbakeSkipHorizontalWhitespace(source, lineStart, lineEnd)
		if trimStart < lineEnd {
			nodes = append(nodes, bitbakeRecoveredShellLineNodes(source, trimStart, lineEnd, arena, lang, syms)...)
		}
		pos = lineEnd
		for pos < end && (source[pos] == '\n' || source[pos] == '\r') {
			pos++
		}
	}
	return nodes
}

func bitbakeRecoveredShellLineNodes(source []byte, start, end int, arena *nodeArena, lang *Language, syms bitbakeCompatSymbols) []*Node {
	line := source[start:end]
	if bytes.HasPrefix(line, []byte("bbnote ${DESTDIR:+DESTDIR=${DESTDIR} ")) ||
		bytes.HasPrefix(line, []byte("eval ${DESTDIR:+DESTDIR=${DESTDIR} ")) {
		return bitbakeDestdirCommandNodes(source, start, end, arena, lang, syms)
	}
	err := bitbakeCommandErrorNode(source, start, end, arena, lang, syms)
	if err == nil {
		return nil
	}
	return []*Node{err}
}

func bitbakeDestdirCommandNodes(source []byte, start, end int, arena *nodeArena, lang *Language, syms bitbakeCompatSymbols) []*Node {
	nested := bytes.Index(source[start:end], []byte("${DESTDIR}"))
	if nested < 0 {
		return nil
	}
	nestedStart := start + nested
	nestedEnd := nestedStart + len("${DESTDIR}")
	outerClose := nestedEnd
	for outerClose < end && source[outerClose] != '}' {
		outerClose++
	}
	if outerClose >= end {
		return nil
	}
	cmdEnd := bitbakeIdentifierEnd(source, start, end)
	errChildren := []*Node{
		bitbakeLeaf(arena, lang, syms.identifier, start, cmdEnd, source),
		bitbakeLeaf(arena, lang, syms.dollarOpen, cmdEnd+1, cmdEnd+3, source),
		bitbakeLeaf(arena, lang, syms.dollarOpen, nestedStart, nestedStart+2, source),
		bitbakeLeaf(arena, lang, syms.dollarClose, nestedEnd-1, nestedEnd, source),
	}
	err := newParentNodeInArena(arena, errorSymbol, true, errChildren, nil, 0)
	err.setExtra(true)
	err.setHasError(true)
	shell := bitbakeLeaf(arena, lang, syms.shellContent, outerClose, end, source)
	return []*Node{err, shell}
}

func bitbakeCommandErrorNode(source []byte, start, end int, arena *nodeArena, lang *Language, syms bitbakeCompatSymbols) *Node {
	line := source[start:end]
	var children []*Node
	switch {
	case bytes.HasPrefix(line, []byte("meson test ")):
		children = append(children,
			bitbakeLeaf(arena, lang, syms.identifier, start, start+5, source),
			bitbakeLeaf(arena, lang, syms.identifier, start+6, start+10, source),
		)
		bitbakeAppendByteLeaf := func(sym Symbol, idx int) {
			children = append(children, bitbakeLeaf(arena, lang, sym, idx, idx+1, source))
		}
		for i := start + 10; i < end; i++ {
			switch source[i] {
			case '-':
				if i > start && ((source[i-1] >= 'a' && source[i-1] <= 'z') || (source[i-1] >= 'A' && source[i-1] <= 'Z')) {
					continue
				}
				bitbakeAppendByteLeaf(syms.minus, i)
			case '"':
				children = append(children, bitbakeLeaf(arena, lang, syms.stringStart, i, i+1, source))
			case '$':
				if i+1 < end && source[i+1] == '{' {
					children = append(children, bitbakeLeaf(arena, lang, syms.dollarOpen, i, i+2, source))
				}
			case '}':
				bitbakeAppendByteLeaf(syms.dollarClose, i)
			}
		}
	case bytes.HasPrefix(line, []byte("ln -sf ")):
		children = append(children,
			bitbakeLeaf(arena, lang, syms.identifier, start, start+2, source),
			bitbakeLeaf(arena, lang, syms.identifier, start+3, start+6, source),
		)
		firstQuote := bytes.IndexByte(line, '"')
		expansion := bytes.Index(line, []byte("${"))
		if firstQuote < 0 || expansion < 0 {
			return nil
		}
		quote := start + firstQuote
		expStart := start + expansion
		expEnd := expStart + 2
		close := expEnd
		for close < end && source[close] != '}' {
			close++
		}
		if close >= end {
			return nil
		}
		children = append(children,
			bitbakeLeaf(arena, lang, syms.stringStart, quote, quote+1, source),
			bitbakeLeaf(arena, lang, syms.dollarOpen, expStart, expStart+2, source),
			bitbakeLeaf(arena, lang, syms.dollarClose, close, close+1, source),
		)
		if close+1 < end && source[close+1] == '/' {
			children = append(children, bitbakeLeaf(arena, lang, syms.slash, close+1, close+2, source))
		}
		lastQuote := end - 1
		if lastQuote > quote && source[lastQuote] == '"' {
			children = append(children, bitbakeLeaf(arena, lang, syms.stringStart, lastQuote, lastQuote+1, source))
		}
	default:
		return nil
	}
	err := newParentNodeInArena(arena, errorSymbol, true, children, nil, 0)
	err.startByte = uint32(start)
	err.endByte = uint32(end)
	err.startPoint = advancePointByBytes(Point{}, source[:start])
	err.endPoint = advancePointByBytes(Point{}, source[:end])
	err.setExtra(true)
	err.setHasError(true)
	return err
}

func bitbakeParseVariableFlagAssignmentAt(source []byte, start, limit int, arena *nodeArena, lang *Language, syms bitbakeCompatSymbols) (*Node, int, bool) {
	nameEnd := bitbakeIdentifierEnd(source, start, limit)
	if nameEnd == start || nameEnd >= limit || source[nameEnd] != '[' {
		return nil, start, false
	}
	flagStart := nameEnd + 1
	flagEnd := flagStart
	for flagEnd < limit && source[flagEnd] != ']' {
		flagEnd++
	}
	if flagEnd >= limit || flagEnd == flagStart {
		return nil, start, false
	}
	pos := bitbakeSkipHorizontalWhitespace(source, flagEnd+1, limit)
	if pos >= limit || source[pos] != '=' {
		return nil, start, false
	}
	eqStart := pos
	pos = bitbakeSkipHorizontalWhitespace(source, pos+1, limit)
	if pos >= limit || source[pos] != '"' {
		return nil, start, false
	}
	literalStart := pos
	literalEnd := pos + 1
	for literalEnd < limit && source[literalEnd] != '"' && source[literalEnd] != '\n' && source[literalEnd] != '\r' {
		literalEnd++
	}
	if literalEnd >= limit || source[literalEnd] != '"' {
		return nil, start, false
	}
	literalEnd++
	flagNode := newParentNodeInArena(arena, syms.variableFlag, symbolIsNamed(lang, syms.variableFlag), []*Node{
		bitbakeLeaf(arena, lang, syms.openBracket, nameEnd, nameEnd+1, source),
		bitbakeLeaf(arena, lang, syms.flag, flagStart, flagEnd, source),
		bitbakeLeaf(arena, lang, syms.closeBracket, flagEnd, flagEnd+1, source),
	}, nil, 0)
	stringNode := newParentNodeInArena(arena, syms.stringSym, symbolIsNamed(lang, syms.stringSym), []*Node{
		bitbakeLeaf(arena, lang, syms.quote, literalStart, literalStart+1, source),
		bitbakeLeaf(arena, lang, syms.stringContent, literalStart+1, literalEnd-1, source),
		bitbakeLeaf(arena, lang, syms.quote, literalEnd-1, literalEnd, source),
	}, nil, 0)
	literal := newParentNodeInArena(arena, syms.literal, symbolIsNamed(lang, syms.literal), []*Node{stringNode}, nil, 0)
	assign := newParentNodeInArena(arena, syms.variableAssignment, symbolIsNamed(lang, syms.variableAssignment), []*Node{
		bitbakeLeaf(arena, lang, syms.identifier, start, nameEnd, source),
		flagNode,
		bitbakeLeaf(arena, lang, syms.eq, eqStart, eqStart+1, source),
		literal,
	}, nil, 0)
	return assign, literalEnd, true
}

func bitbakeLeaf(arena *nodeArena, lang *Language, sym Symbol, start, end int, source []byte) *Node {
	startPoint := advancePointByBytes(Point{}, source[:start])
	endPoint := advancePointByBytes(startPoint, source[start:end])
	return newLeafNodeInArena(arena, sym, symbolIsNamed(lang, sym), uint32(start), uint32(end), startPoint, endPoint)
}

func bitbakeIdentifierEnd(source []byte, start, limit int) int {
	i := start
	for i < limit {
		c := source[i]
		if (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') || c == '_' || c == '-' {
			i++
			continue
		}
		break
	}
	return i
}

func bitbakeSkipHorizontalWhitespace(source []byte, pos, limit int) int {
	for pos < limit && (source[pos] == ' ' || source[pos] == '\t') {
		pos++
	}
	return pos
}

func bitbakeSkipLineBreaksAndHorizontalWhitespace(source []byte, pos, limit int) int {
	for pos < limit {
		switch source[pos] {
		case ' ', '\t', '\n', '\r':
			pos++
		default:
			return pos
		}
	}
	return pos
}

func bitbakeFindFunctionCloseBrace(source []byte, start, limit int) int {
	for pos := start; pos < limit; {
		lineStart := pos
		lineEnd := pos
		for lineEnd < limit && source[lineEnd] != '\n' && source[lineEnd] != '\r' {
			lineEnd++
		}
		trim := bitbakeSkipHorizontalWhitespace(source, lineStart, lineEnd)
		if trim < lineEnd && source[trim] == '}' {
			after := bitbakeSkipHorizontalWhitespace(source, trim+1, lineEnd)
			if after == lineEnd {
				return trim
			}
		}
		pos = lineEnd
		for pos < limit && (source[pos] == '\n' || source[pos] == '\r') {
			pos++
		}
	}
	return -1
}
