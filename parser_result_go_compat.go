package gotreesitter

import "bytes"

const goSemicolonContainerSymbolCount = 11

type goCompatibilitySymbols struct {
	semicolon         Symbol
	expressionCase    Symbol
	defaultCase       Symbol
	typeCase          Symbol
	communicationCase Symbol
	statementList     Symbol
	statementListTail Symbol
	semiContainers    [goSemicolonContainerSymbolCount]Symbol
	semiContainerLen  int
}

type goCompatibilitySourceFlags struct {
	dot              bool
	siblingBoundary  bool
	trailingBoundary bool
}

func normalizeGoCompatibility(root *Node, source []byte, lang *Language) {
	_ = normalizeGoCompatibilityInRanges(root, source, lang, nil)
}

func normalizeGoCompatibilityInRanges(root *Node, source []byte, lang *Language, incrementalRanges []Range) ParseStopReason {
	return normalizeGoCompatibilityInRangesWithStop(root, source, lang, incrementalRanges, nil)
}

func normalizeGoCompatibilityWithStop(root *Node, source []byte, lang *Language, stopCheck parseStopCheck) ParseStopReason {
	return normalizeGoCompatibilityInRangesWithStop(root, source, lang, nil, stopCheck)
}

func normalizeGoCompatibilityInRangesWithStop(root *Node, source []byte, lang *Language, incrementalRanges []Range, stopCheck parseStopCheck) ParseStopReason {
	if root == nil || lang == nil || lang.Name != "go" || len(source) == 0 {
		return ParseStopNone
	}
	poller := parseStopPoller{check: stopCheck}
	if reason := poller.pollNow(); parseStopReasonIsActive(reason) {
		return reason
	}
	flags := goCompatibilitySourceFlagsFor(source)
	if flags.dot {
		if reason := normalizeGoDotLeafChildrenWithStop(root, source, lang, &poller); parseStopReasonIsActive(reason) {
			return reason
		}
	}
	syms, ok := goCompatibilitySymbolsForLanguage(lang)
	if !ok {
		return poller.pollNow()
	}
	if reason := normalizeGoCompatibilitySubtreeWithStop(root, source, syms, flags, incrementalRanges, &poller); parseStopReasonIsActive(reason) {
		return reason
	}
	return poller.pollNow()
}

func goCompatibilitySourceFlagsFor(source []byte) goCompatibilitySourceFlags {
	return goCompatibilitySourceFlags{
		dot:              bytes.IndexByte(source, '.') >= 0,
		siblingBoundary:  goSourceMayNeedSiblingBoundaryCompatibility(source),
		trailingBoundary: goSourceMayNeedTrailingBoundaryCompatibility(source),
	}
}

func goSourceMayNeedSiblingBoundaryCompatibility(source []byte) bool {
	return bytes.Contains(source, []byte("//")) ||
		bytes.Contains(source, []byte("/*")) ||
		bytes.Contains(source, []byte("case")) ||
		bytes.Contains(source, []byte("default")) ||
		bytes.Contains(source, []byte("switch")) ||
		bytes.Contains(source, []byte("select"))
}

func goSourceMayNeedTrailingBoundaryCompatibility(source []byte) bool {
	return bytes.Contains(source, []byte("//")) ||
		bytes.Contains(source, []byte("/*"))
}

func normalizeGoDotLeafChildren(root *Node, source []byte, lang *Language) {
	_ = normalizeGoDotLeafChildrenWithStop(root, source, lang, nil)
}

func normalizeGoDotLeafChildrenWithStop(root *Node, source []byte, lang *Language, poller *parseStopPoller) ParseStopReason {
	if root == nil || lang == nil || len(source) == 0 {
		return ParseStopNone
	}
	if reason := poller.pollNow(); parseStopReasonIsActive(reason) {
		return reason
	}
	parentSym, ok := lang.symbolByNameAndNamed("dot", true)
	if !ok {
		parentSym, ok = symbolByName(lang, "dot")
	}
	if !ok {
		return ParseStopNone
	}
	childSym, ok := lang.symbolByNameAndNamed(".", false)
	if !ok {
		childSym, ok = symbolByName(lang, ".")
	}
	if !ok {
		return ParseStopNone
	}
	childNamed := symbolIsNamed(lang, childSym)
	// Iterative DFS with an explicit stack: source trees can nest or chain
	// deeply enough that callback recursion overflows the goroutine stack
	// (fatal, unrecoverable) — see issue #110.
	stack := []*Node{root}
	push := func(n *Node) {
		if n != nil {
			stack = append(stack, n)
		}
	}
	for len(stack) > 0 {
		if reason := poller.poll(); parseStopReasonIsActive(reason) {
			return reason
		}
		n := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		childCount := resultChildCount(n)
		if n.symbol == parentSym && childCount == 0 {
			normalizeGoDotLeafNode(n, source, childSym, childNamed)
			continue
		}
		if n.ownerArena == nil || n.childIndex > finalChildSidecarIndexBase {
			for _, child := range n.children {
				push(child)
			}
			continue
		}
		view := resultMutableChildrenForMutation(n)
		if !view.hasFinalChildRefs() {
			for i := 0; i < childCount; i++ {
				push(resultChildAt(n, i))
			}
			continue
		}
		for i := 0; i < view.Len(); i++ {
			entry, ok := view.Entry(i)
			if !ok {
				continue
			}
			if stackEntryNodeSymbol(entry) != parentSym && stackEntryNodeChildCount(entry) == 0 {
				continue
			}
			push(resultChildAt(n, i))
		}
	}
	return poller.pollNow()
}

func normalizeGoDotLeafNode(n *Node, source []byte, childSym Symbol, childNamed bool) {
	if n == nil || int(n.startByte) > len(source) || int(n.endByte) > len(source) || n.startByte > n.endByte {
		return
	}
	if !bytes.Equal(source[n.startByte:n.endByte], []byte(".")) {
		return
	}
	child := newLeafNodeInArena(n.ownerArena, childSym, childNamed, n.startByte, n.endByte, n.startPoint, n.endPoint)
	child.parent = n
	child.childIndex = 0
	n.children = cloneNodeSliceInArena(n.ownerArena, []*Node{child})
}

func goCompatibilitySymbolsForLanguage(lang *Language) (goCompatibilitySymbols, bool) {
	var syms goCompatibilitySymbols
	syms.semicolon, _ = symbolByName(lang, ";")
	syms.expressionCase, _ = symbolByName(lang, "expression_case")
	syms.defaultCase, _ = symbolByName(lang, "default_case")
	syms.typeCase, _ = symbolByName(lang, "type_case")
	syms.communicationCase, _ = symbolByName(lang, "communication_case")
	syms.statementList, _ = symbolByName(lang, "statement_list")
	syms.statementListTail, _ = symbolByName(lang, "statement_list_repeat1")
	syms.addSemicolonContainer(lang, "source_file")
	syms.addSemicolonContainer(lang, "statement_list")
	syms.addSemicolonContainer(lang, "statement_list_repeat1")
	syms.addSemicolonContainer(lang, "import_declaration")
	syms.addSemicolonContainer(lang, "var_declaration")
	syms.addSemicolonContainer(lang, "const_declaration")
	syms.addSemicolonContainer(lang, "type_declaration")
	syms.addSemicolonContainer(lang, "import_spec_list")
	syms.addSemicolonContainer(lang, "var_spec_list")
	syms.addSemicolonContainer(lang, "const_spec_list")
	syms.addSemicolonContainer(lang, "field_declaration_list")
	return syms, true
}

func (s *goCompatibilitySymbols) addSemicolonContainer(lang *Language, name string) {
	if s.semiContainerLen >= len(s.semiContainers) {
		return
	}
	sym, ok := symbolByName(lang, name)
	if !ok {
		return
	}
	s.semiContainers[s.semiContainerLen] = sym
	s.semiContainerLen++
}

func (s goCompatibilitySymbols) isSemicolonContainer(sym Symbol) bool {
	for _, candidate := range s.semiContainers[:s.semiContainerLen] {
		if candidate == sym {
			return true
		}
	}
	return false
}

func (s goCompatibilitySymbols) isCase(sym Symbol) bool {
	switch sym {
	case s.expressionCase, s.defaultCase, s.typeCase, s.communicationCase:
		return sym != 0
	default:
		return false
	}
}

func (s goCompatibilitySymbols) isStatementList(sym Symbol) bool {
	return (s.statementList != 0 && sym == s.statementList) || (s.statementListTail != 0 && sym == s.statementListTail)
}

func normalizeGoCompatibilitySubtree(n *Node, source []byte, syms goCompatibilitySymbols, flags goCompatibilitySourceFlags, incrementalRanges []Range) {
	_ = normalizeGoCompatibilitySubtreeWithStop(n, source, syms, flags, incrementalRanges, nil)
}

func normalizeGoCompatibilitySubtreeWithStop(n *Node, source []byte, syms goCompatibilitySymbols, flags goCompatibilitySourceFlags, incrementalRanges []Range, poller *parseStopPoller) ParseStopReason {
	if reason := poller.poll(); parseStopReasonIsActive(reason) {
		return reason
	}
	if n == nil || !goNodeOverlapsAnyRange(n, incrementalRanges) {
		return ParseStopNone
	}
	childCount := resultChildCount(n)
	if childCount > 0 {
		normalizeGoSemicolonContainer(n, source, syms)
		if flags.siblingBoundary {
			normalizeGoAdjacentSiblingBoundaries(n, source, syms)
		}
	}
	if n.ownerArena == nil || n.childIndex > finalChildSidecarIndexBase {
		for _, child := range n.children {
			if reason := normalizeGoCompatibilitySubtreeWithStop(child, source, syms, flags, incrementalRanges, poller); parseStopReasonIsActive(reason) {
				return reason
			}
		}
	} else {
		view := resultMutableChildrenForMutation(n)
		if view.hasFinalChildRefs() {
			for i := 0; i < view.Len(); i++ {
				entry, ok := view.Entry(i)
				if !ok || stackEntryNodeChildCount(entry) == 0 {
					continue
				}
				if reason := normalizeGoCompatibilitySubtreeWithStop(resultChildAt(n, i), source, syms, flags, incrementalRanges, poller); parseStopReasonIsActive(reason) {
					return reason
				}
			}
		} else {
			for i := 0; i < childCount; i++ {
				if reason := normalizeGoCompatibilitySubtreeWithStop(resultChildAt(n, i), source, syms, flags, incrementalRanges, poller); parseStopReasonIsActive(reason) {
					return reason
				}
			}
		}
	}
	if flags.trailingBoundary {
		normalizeGoStatementListTrailingExtras(n, source, syms)
	}
	return ParseStopNone
}

func goNodeOverlapsAnyRange(n *Node, ranges []Range) bool {
	if n == nil || len(ranges) == 0 {
		return true
	}
	for _, r := range ranges {
		if !(n.endByte < r.StartByte || r.EndByte < n.startByte) {
			return true
		}
	}
	return false
}

func normalizeGoSemicolonContainer(n *Node, source []byte, syms goCompatibilitySymbols) {
	if syms.semicolon == 0 || !syms.isSemicolonContainer(n.symbol) {
		return
	}
	view := resultMutableChildrenForMutation(n)
	if view.hasFinalChildRefs() {
		normalizeGoSemicolonFinalRefs(view, source, syms.semicolon)
		return
	}
	if !goHasDroppableSemicolonChild(n, source, syms.semicolon) {
		return
	}
	children := resultChildSliceForMutation(n)
	kept := make([]*Node, 0, len(children))
	for _, child := range children {
		if goIsDroppableSemicolonNode(child, source, syms.semicolon) {
			continue
		}
		kept = append(kept, child)
	}
	replaceNodeChildrenUnfielded(n, cloneNodeSliceIfArena(n.ownerArena, kept))
}

func normalizeGoSemicolonFinalRefs(view resultMutableChildView, source []byte, semicolon Symbol) {
	if !goHasDroppableSemicolonFinalRef(view, source, semicolon) {
		return
	}
	view.FilterFinalRefs(func(_ int, entry stackEntry) bool {
		return !goIsDroppableSemicolonEntry(entry, source, semicolon)
	})
}

func goHasDroppableSemicolonFinalRef(view resultMutableChildView, source []byte, semicolon Symbol) bool {
	for i := 0; i < view.Len(); i++ {
		entry, ok := view.Entry(i)
		if ok && goIsDroppableSemicolonEntry(entry, source, semicolon) {
			return true
		}
	}
	return false
}

func goHasDroppableSemicolonChild(n *Node, source []byte, semicolon Symbol) bool {
	if n != nil && (n.ownerArena == nil || n.childIndex > finalChildSidecarIndexBase) {
		for _, child := range n.children {
			if goIsDroppableSemicolonNode(child, source, semicolon) {
				return true
			}
		}
		return false
	}
	for i := 0; i < resultChildCount(n); i++ {
		if goIsDroppableSemicolonNode(resultChildAt(n, i), source, semicolon) {
			return true
		}
	}
	return false
}

func goIsDroppableSemicolonNode(n *Node, source []byte, semicolon Symbol) bool {
	return n != nil && n.symbol == semicolon && goShouldDropSemicolonSpan(n.startByte, n.endByte, source)
}

func goIsDroppableSemicolonEntry(entry stackEntry, source []byte, semicolon Symbol) bool {
	return stackEntryHasNode(entry) &&
		stackEntryNodeSymbol(entry) == semicolon &&
		goShouldDropSemicolonSpan(stackEntryNodeStartByte(entry), stackEntryNodeEndByte(entry), source)
}

func goShouldDropSemicolonSpan(startByte, endByte uint32, source []byte) bool {
	if startByte >= endByte || int(endByte) > len(source) {
		return true
	}
	text := source[startByte:endByte]
	if bytes.IndexByte(text, ';') >= 0 {
		return false
	}
	return bytes.IndexByte(text, '\n') >= 0 || bytes.IndexByte(text, '\r') >= 0
}

func normalizeGoAdjacentSiblingBoundaries(n *Node, source []byte, syms goCompatibilitySymbols) {
	if n != nil && (n.ownerArena == nil || n.childIndex > finalChildSidecarIndexBase) {
		for i := 0; i+1 < len(n.children); i++ {
			curr := n.children[i]
			next := n.children[i+1]
			if curr == nil || next == nil {
				continue
			}
			normalizeGoStatementListBoundary(curr, next, source, syms)
			normalizeGoCaseSiblingBoundary(curr, next, source, syms)
		}
		return
	}
	view := resultMutableChildrenForMutation(n)
	if view.hasFinalChildRefs() {
		for i := 0; i+1 < view.Len(); i++ {
			currEntry, ok := view.Entry(i)
			if !ok {
				continue
			}
			currSym := stackEntryNodeSymbol(currEntry)
			if !syms.isStatementList(currSym) && !syms.isCase(currSym) {
				continue
			}
			nextEntry, ok := view.Entry(i + 1)
			if !ok {
				continue
			}
			curr := resultChildAt(n, i)
			if curr == nil {
				continue
			}
			nextStart := stackEntryNodeStartByte(nextEntry)
			normalizeGoStatementListBoundaryBefore(curr, nextStart, source, syms)
			normalizeGoCaseSiblingBoundaryBefore(curr, nextStart, source, syms)
		}
		return
	}
	childCount := resultChildCount(n)
	for i := 0; i+1 < childCount; i++ {
		curr := resultChildAt(n, i)
		next := resultChildAt(n, i+1)
		if curr == nil || next == nil {
			continue
		}
		normalizeGoStatementListBoundary(curr, next, source, syms)
		normalizeGoCaseSiblingBoundary(curr, next, source, syms)
	}
}

func normalizeGoStatementListBoundary(curr, next *Node, source []byte, syms goCompatibilitySymbols) {
	if next == nil {
		return
	}
	normalizeGoStatementListBoundaryBefore(curr, next.startByte, source, syms)
}

func normalizeGoStatementListBoundaryBefore(curr *Node, nextStart uint32, source []byte, syms goCompatibilitySymbols) {
	if curr == nil || !syms.isStatementList(curr.symbol) || curr.endByte >= nextStart || int(nextStart) > len(source) {
		return
	}
	gap := source[curr.endByte:nextStart]
	if !bytesAreTrivia(gap) {
		return
	}
	target := goTrailingNewlineBoundary(curr.endByte, nextStart, source)
	if target > curr.endByte {
		extendNodeEndTo(curr, target, source)
	}
}

func normalizeGoStatementListTrailingExtras(n *Node, source []byte, syms goCompatibilitySymbols) {
	childCount := resultChildCount(n)
	if !syms.isStatementList(n.symbol) || childCount == 0 || int(n.endByte) > len(source) {
		return
	}
	var last *Node
	if n.ownerArena == nil || n.childIndex > finalChildSidecarIndexBase {
		last = n.children[childCount-1]
	} else {
		last = resultChildAt(n, childCount-1)
	}
	if last == nil || last.endByte >= n.endByte {
		return
	}
	target := goTrailingTriviaBeforeExtra(last.endByte, n.endByte, source)
	if target > last.endByte && target < n.endByte {
		setNodeEndTo(n, target, source)
	}
}

func goTrailingTriviaBeforeExtra(start, end uint32, source []byte) uint32 {
	if start >= end || int(end) > len(source) {
		return start
	}
	for cursor := start; cursor < end; cursor++ {
		switch source[cursor] {
		case ' ', '\t', '\n', '\r':
			continue
		default:
			for newline := start; newline < cursor; newline++ {
				if source[newline] == '\n' {
					return newline + 1
				}
			}
			return cursor
		}
	}
	return end
}

func normalizeGoCaseSiblingBoundary(curr, next *Node, source []byte, syms goCompatibilitySymbols) {
	if next == nil {
		return
	}
	normalizeGoCaseSiblingBoundaryBefore(curr, next.startByte, source, syms)
}

func normalizeGoCaseSiblingBoundaryBefore(curr *Node, nextStart uint32, source []byte, syms goCompatibilitySymbols) {
	if curr == nil || !syms.isCase(curr.symbol) || int(nextStart) > len(source) {
		return
	}
	tail := goTrailingCaseStatementList(curr, syms)
	if tail == nil {
		return
	}
	target, hasNewline := goTrailingTriviaBoundaryBefore(nextStart, source)
	if hasNewline {
		normalizeGoCaseBoundaryToTrivia(curr, tail, target, source)
		return
	}
	if curr.endByte > nextStart {
		setNodeEndTo(curr, nextStart, source)
	}
	if tail.endByte > nextStart {
		setNodeEndTo(tail, nextStart, source)
	}
}

func normalizeGoCaseBoundaryToTrivia(curr, tail *Node, target uint32, source []byte) {
	if curr.endByte != target {
		setNodeEndTo(curr, target, source)
	}
	switch {
	case tail.endByte > target:
		setNodeEndTo(tail, target, source)
	case tail.endByte < target && bytesAreTrivia(source[tail.endByte:target]):
		setNodeEndTo(tail, target, source)
	}
}

func goTrailingNewlineBoundary(start, end uint32, source []byte) uint32 {
	if start >= end || int(end) > len(source) || !bytesAreTrivia(source[start:end]) {
		return start
	}
	gap := source[start:end]
	if newline := bytes.LastIndexByte(gap, '\n'); newline >= 0 {
		return start + uint32(newline+1)
	}
	return start
}

func goTrailingTriviaBoundaryBefore(end uint32, source []byte) (uint32, bool) {
	if end == 0 || int(end) > len(source) {
		return end, false
	}
	start := int(end)
	for start > 0 {
		switch source[start-1] {
		case ' ', '\t', '\r', '\n':
			start--
		default:
			goto gapReady
		}
	}
gapReady:
	gap := source[start:int(end)]
	if newline := bytes.LastIndexByte(gap, '\n'); newline >= 0 {
		return uint32(start + newline + 1), true
	}
	return end, false
}

func goTrailingCaseStatementList(n *Node, syms goCompatibilitySymbols) *Node {
	childCount := resultChildCount(n)
	if n == nil || childCount == 0 {
		return nil
	}
	var last *Node
	if n.ownerArena == nil || n.childIndex > finalChildSidecarIndexBase {
		last = n.children[childCount-1]
	} else {
		last = resultChildAt(n, childCount-1)
	}
	if last == nil || !syms.isStatementList(last.symbol) {
		return nil
	}
	return last
}
