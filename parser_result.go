package gotreesitter

import (
	"fmt"
	"os"
)

func debugFinalResultNodes(lang *Language, nodes []*Node) {
	if os.Getenv("GTS_TMP_DEBUG_FINAL_NODES") != "1" || lang == nil {
		return
	}
	fmt.Printf("[final-nodes] lang=%s count=%d\n", lang.Name, len(nodes))
	for i, n := range nodes {
		if n == nil {
			fmt.Printf("  [%d] <nil>\n", i)
			continue
		}
		fmt.Printf("  [%d] sym=%d type=%s extra=%v named=%v error=%v missing=%v children=%d span=%d-%d pre=%d parse=%d\n",
			i, n.symbol, n.Type(lang), n.isExtra, n.isNamed, n.hasError, n.isMissing,
			len(n.children), n.startByte, n.endByte, n.preGotoState, n.parseState)
	}
}

// buildResultFromGLR picks the best stack and constructs the final tree.
// Prefers accepted stacks, then highest score, then most entries.
func (p *Parser) buildResultFromGLR(stacks []glrStack, source []byte, arena *nodeArena, oldTree *Tree, reuseState *parseReuseState, linkScratch *[]*Node) *Tree {
	if len(stacks) == 0 {
		arena.Release()
		return parseErrorTree(source, p.language)
	}

	best := 0
	for i := 1; i < len(stacks); i++ {
		if stackComparePtr(&stacks[i], &stacks[best]) > 0 {
			best = i
		}
	}

	selected := stacks[best]
	if len(selected.entries) > 0 {
		return p.buildResult(selected.entries, source, arena, oldTree, reuseState, linkScratch)
	}
	if selected.gss.head == nil {
		return p.buildResult(nil, source, arena, oldTree, reuseState, linkScratch)
	}
	return p.buildResultFromNodes(nodesFromGSS(selected.gss), source, arena, oldTree, reuseState, linkScratch)
}

// isNamedSymbol checks whether a symbol is a named symbol.
func (p *Parser) isNamedSymbol(sym Symbol) bool {
	if int(sym) < len(p.language.SymbolMetadata) {
		return p.language.SymbolMetadata[sym].Named
	}
	return false
}

func nodesFromGSS(stack gssStack) []*Node {
	if stack.head == nil {
		return nil
	}
	count := 0
	for n := stack.head; n != nil; n = n.prev {
		if n.entry.node != nil {
			count++
		}
	}
	if count == 0 {
		return nil
	}
	nodes := make([]*Node, count)
	i := count - 1
	for n := stack.head; n != nil; n = n.prev {
		if n.entry.node != nil {
			nodes[i] = n.entry.node
			i--
		}
	}
	return nodes
}

// buildResult constructs the final Tree from a stack of entries.
func (p *Parser) buildResult(stack []stackEntry, source []byte, arena *nodeArena, oldTree *Tree, reuseState *parseReuseState, linkScratch *[]*Node) *Tree {
	var nodes []*Node
	for _, entry := range stack {
		if entry.node != nil {
			nodes = append(nodes, entry.node)
		}
	}
	return p.buildResultFromNodes(nodes, source, arena, oldTree, reuseState, linkScratch)
}

func (p *Parser) buildResultFromNodes(nodes []*Node, source []byte, arena *nodeArena, oldTree *Tree, reuseState *parseReuseState, linkScratch *[]*Node) *Tree {
	if len(nodes) == 0 {
		arena.Release()
		if isWhitespaceOnlySource(source) {
			return NewTree(nil, source, p.language)
		}
		return parseErrorTree(source, p.language)
	}

	if arena != nil && arena.used == 0 {
		arena.Release()
		arena = nil
	}

	if p != nil {
		debugFinalResultNodes(p.language, nodes)
	}

	expectedRootSymbol := Symbol(0)
	hasExpectedRoot := false
	shouldWireParentLinks := oldTree == nil
	if p != nil && p.hasRootSymbol {
		expectedRootSymbol = p.rootSymbol
		hasExpectedRoot = true
	}
	if oldTree != nil && oldTree.RootNode() != nil {
		expectedRootSymbol = oldTree.RootNode().symbol
		hasExpectedRoot = true
	}
	if p != nil && p.language != nil && p.language.Name == "python" {
		nodes = collapsePythonRootFragments(nodes, arena, p.language)
	}
	borrowedResolved := false
	var borrowed []*nodeArena
	getBorrowed := func() []*nodeArena {
		if borrowedResolved {
			return borrowed
		}
		borrowed = reuseState.retainBorrowed(arena)
		borrowedResolved = true
		return borrowed
	}

	if len(nodes) == 1 {
		candidate := nodes[0]
		candidate = repairPythonRootNode(candidate, arena, p.language)
		extendNodeToTrailingWhitespace(candidate, source)
		p.normalizeRootSourceStart(candidate, source)
		normalizeKnownSpanAttribution(candidate, source, p.language)
		if !hasExpectedRoot || candidate.symbol == expectedRootSymbol {
			if shouldWireParentLinks {
				wireParentLinksWithScratch(candidate, linkScratch)
			}
			return newTreeWithArenas(candidate, source, p.language, arena, getBorrowed())
		}

		// Incremental reuse guard: if the only stacked node doesn't match the
		// previous root symbol, synthesize an expected root wrapper instead of
		// returning a reused child as the new tree root.
		rootChildren := make([]*Node, 1)
		rootChildren[0] = candidate
		if arena != nil {
			rootChildren = arena.allocNodeSlice(1)
			rootChildren[0] = candidate
		}
		root := newParentNodeInArena(arena, expectedRootSymbol, true, rootChildren, nil, 0)
		extendNodeToTrailingWhitespace(root, source)
		p.normalizeRootSourceStart(root, source)
		normalizeKnownSpanAttribution(root, source, p.language)
		if shouldWireParentLinks {
			wireParentLinksWithScratch(root, linkScratch)
		}
		return newTreeWithArenas(root, source, p.language, arena, getBorrowed())
	}

	// When multiple nodes remain on the stack, check whether all but one
	// are extras (e.g. leading whitespace/comments). If so, fold the extras
	// into the real root rather than wrapping everything in an error node.
	var realRoot *Node
	var allExtras []*Node
	var extras []*Node
	for _, n := range nodes {
		if n.isExtra {
			allExtras = append(allExtras, n)
			// Ignore invisible extras in final-root recovery; they should not
			// force an error wrapper or inflate root child counts.
			if p != nil && p.language != nil && int(n.symbol) < len(p.language.SymbolMetadata) && p.language.SymbolMetadata[n.symbol].Visible {
				extras = append(extras, n)
			}
		} else {
			if realRoot != nil {
				realRoot = nil // more than one non-extra -> genuine error
				break
			}
			realRoot = n
		}
	}
	if realRoot != nil {
		if reuseState != nil && reuseState.reusedAny {
			realRoot = cloneNodeInArena(arena, realRoot)
			realRoot.parent = nil
			realRoot.childIndex = -1
		}
		if len(extras) > 0 {
			// Fold visible extras into the real root as leading/trailing children.
			merged := make([]*Node, 0, len(extras)+len(realRoot.children))
			leadingCount := 0
			for _, e := range extras {
				if e.startByte <= realRoot.startByte {
					merged = append(merged, e)
					leadingCount++
				}
			}
			merged = append(merged, realRoot.children...)
			for _, e := range extras {
				if e.startByte > realRoot.startByte {
					merged = append(merged, e)
				}
			}
			if arena != nil {
				out := arena.allocNodeSlice(len(merged))
				copy(out, merged)
				merged = out
			}
			realRoot.children = merged
			// Keep fieldIDs aligned with children: extras have no field (0).
			if len(realRoot.fieldIDs) > 0 {
				trailingCount := len(extras) - leadingCount
				padded := make([]FieldID, leadingCount+len(realRoot.fieldIDs)+trailingCount)
				copy(padded[leadingCount:], realRoot.fieldIDs)
				realRoot.fieldIDs = padded
				if len(realRoot.fieldSources) > 0 {
					paddedSources := make([]uint8, len(padded))
					copy(paddedSources[leadingCount:], realRoot.fieldSources)
					realRoot.fieldSources = paddedSources
				}
			}
			// Extend root range to cover the extras.
			for _, e := range extras {
				if e.startByte < realRoot.startByte {
					realRoot.startByte = e.startByte
					realRoot.startPoint = e.startPoint
				}
				if e.endByte > realRoot.endByte {
					realRoot.endByte = e.endByte
					realRoot.endPoint = e.endPoint
				}
			}
		}
		// Invisible extras should still contribute to the root byte/point range.
		for _, e := range allExtras {
			if e.startByte < realRoot.startByte {
				realRoot.startByte = e.startByte
				realRoot.startPoint = e.startPoint
			}
			if e.endByte > realRoot.endByte {
				realRoot.endByte = e.endByte
				realRoot.endPoint = e.endPoint
			}
		}
		realRoot = repairPythonRootNode(realRoot, arena, p.language)
		extendNodeToTrailingWhitespace(realRoot, source)
		p.normalizeRootSourceStart(realRoot, source)
		normalizeKnownSpanAttribution(realRoot, source, p.language)
		if !hasExpectedRoot || realRoot.symbol == expectedRootSymbol {
			if shouldWireParentLinks {
				wireParentLinksWithScratch(realRoot, linkScratch)
			}
			return newTreeWithArenas(realRoot, source, p.language, arena, getBorrowed())
		}
	}

	rootChildren := nodes
	rootSymbol := nodes[len(nodes)-1].symbol
	if hasExpectedRoot {
		rootSymbol = expectedRootSymbol
	}
	root := newParentNodeInArena(arena, rootSymbol, true, rootChildren, nil, 0)
	if !(p != nil && p.language != nil && p.language.Name == "python" && hasExpectedRoot && pythonModuleChildrenLookComplete(nodes, p.language)) {
		root.hasError = true
	}
	root = repairPythonRootNode(root, arena, p.language)
	extendNodeToTrailingWhitespace(root, source)
	p.normalizeRootSourceStart(root, source)
	normalizeKnownSpanAttribution(root, source, p.language)
	if shouldWireParentLinks {
		wireParentLinksWithScratch(root, linkScratch)
	}
	return newTreeWithArenas(root, source, p.language, arena, getBorrowed())
}

func (p *Parser) normalizeRootSourceStart(root *Node, source []byte) {
	if root == nil || root.startByte == 0 || len(source) == 0 {
		return
	}
	// Included-range parses intentionally preserve range-local root spans.
	if p != nil && len(p.included) > 0 {
		return
	}
	root.startByte = 0
	root.startPoint = Point{}
}

// normalizeKnownSpanAttribution applies narrow compatibility fixes where
// C tree-sitter attributes trailing trivia to a grouped node but this runtime
// currently drops it during child normalization.
func normalizeKnownSpanAttribution(root *Node, source []byte, lang *Language) {
	normalizeCobolLeadingAreaStart(root, source, lang)
	normalizeCooklangTrailingStepTail(root, source, lang)
	normalizeFortranStatementLineBreaks(root, source, lang)
	normalizeHaskellImportsSpan(root, source, lang)
	normalizeNginxAttributeLineBreaks(root, source, lang)
	normalizeTopLevelTrailingLineBreakSpan(root, source, lang)
	normalizeScalaTrailingCommentOwnership(root, source, lang)
	normalizeScalaFunctionModifierFields(root, lang)
	normalizeScalaInterpolatedStringTail(root, source, lang)
}

func bytesAreTrivia(b []byte) bool {
	for _, c := range b {
		switch c {
		case ' ', '\t', '\n', '\r':
			continue
		default:
			return false
		}
	}
	return true
}

func bytesContainLineBreak(b []byte) bool {
	for _, c := range b {
		if c == '\n' || c == '\r' {
			return true
		}
	}
	return false
}

func firstNonWhitespaceByte(source []byte) uint32 {
	for i, c := range source {
		switch c {
		case ' ', '\t', '\n', '\r':
			continue
		default:
			return uint32(i)
		}
	}
	return 0
}

func normalizeCobolLeadingAreaStart(root *Node, source []byte, lang *Language) {
	if root == nil || lang == nil || lang.Name != "cobol" || len(source) == 0 {
		return
	}
	start := firstNonWhitespaceByte(source)
	if start == 0 {
		return
	}
	startPoint := advancePointByBytes(Point{}, source[:start])
	setNodeStartTo := func(n *Node) {
		if n == nil || n.startByte == start {
			return
		}
		n.startByte = start
		n.startPoint = startPoint
	}
	setNodeStartTo(root)
	if len(root.children) != 1 {
		return
	}
	def := root.children[0]
	if def == nil || def.Type(lang) != "program_definition" {
		return
	}
	setNodeStartTo(def)
	if len(def.children) != 1 {
		return
	}
	div := def.children[0]
	if div == nil || div.Type(lang) != "identification_division" {
		return
	}
	setNodeStartTo(div)
}

func bytesAreCooklangStepTail(b []byte) bool {
	sawPunctuation := false
	for _, c := range b {
		switch c {
		case '.', '!', '?':
			if sawPunctuation {
				return false
			}
			sawPunctuation = true
		case ' ', '\t', '\n', '\r':
		default:
			return false
		}
	}
	return sawPunctuation
}

func normalizeCooklangTrailingStepTail(root *Node, source []byte, lang *Language) {
	if root == nil || lang == nil || lang.Name != "cooklang" || len(source) == 0 {
		return
	}
	if root.Type(lang) != "recipe" || len(root.children) != 1 {
		return
	}
	step := root.children[0]
	if step == nil || step.Type(lang) != "step" || step.endByte >= uint32(len(source)) {
		return
	}
	tail := source[step.endByte:]
	if !bytesAreCooklangStepTail(tail) {
		return
	}
	stepEnd := step.endByte
	for i := int(step.endByte); i < len(source); i++ {
		switch source[i] {
		case '.', '!', '?':
			stepEnd = uint32(i + 1)
		}
	}
	if stepEnd > step.endByte {
		extendNodeEndTo(step, stepEnd, source)
	}
	if root.endByte < uint32(len(source)) {
		extendNodeEndTo(root, uint32(len(source)), source)
	}
}

func lineBreakEndAt(source []byte, start, limit uint32) uint32 {
	if start >= limit || start >= uint32(len(source)) {
		return 0
	}
	switch source[start] {
	case '\n':
		return start + 1
	case '\r':
		if start+1 < limit && start+1 < uint32(len(source)) && source[start+1] == '\n' {
			return start + 2
		}
		return start + 1
	default:
		return 0
	}
}

func normalizeFortranStatementLineBreaks(root *Node, source []byte, lang *Language) {
	if root == nil || lang == nil || lang.Name != "fortran" || len(source) == 0 {
		return
	}
	var walk func(*Node)
	walk = func(n *Node) {
		if n == nil {
			return
		}
		if n.Type(lang) == "program" {
			for i := 0; i+1 < len(n.children); i++ {
				cur := n.children[i]
				next := n.children[i+1]
				if cur == nil || next == nil || cur.endByte >= next.startByte {
					continue
				}
				if cur.Type(lang) != "program_statement" {
					continue
				}
				if end := lineBreakEndAt(source, cur.endByte, next.startByte); end > cur.endByte {
					extendNodeEndTo(cur, end, source)
				}
			}
		}
		for _, child := range n.children {
			walk(child)
		}
	}
	walk(root)
}

func normalizeNginxAttributeLineBreaks(root *Node, source []byte, lang *Language) {
	if root == nil || lang == nil || lang.Name != "nginx" || len(source) == 0 {
		return
	}
	var walk func(*Node)
	walk = func(n *Node) {
		if n == nil {
			return
		}
		if n.Type(lang) == "attribute" {
			if end := lineBreakEndAt(source, n.endByte, uint32(len(source))); end > n.endByte {
				extendNodeEndTo(n, end, source)
			}
		}
		for _, child := range n.children {
			walk(child)
		}
	}
	walk(root)
}

func normalizeTopLevelTrailingLineBreakSpan(root *Node, source []byte, lang *Language) {
	if root == nil || lang == nil || len(source) == 0 {
		return
	}
	switch lang.Name {
	case "caddy", "fortran", "pug":
	default:
		return
	}
	if len(root.children) != 1 {
		return
	}
	child := root.children[0]
	if child == nil || child.endByte >= root.endByte || root.endByte > uint32(len(source)) {
		return
	}
	gap := source[child.endByte:root.endByte]
	if !bytesAreTrivia(gap) || !bytesContainLineBreak(gap) {
		return
	}
	extendNodeEndTo(child, root.endByte, source)
}

func normalizeHaskellImportsSpan(root *Node, source []byte, lang *Language) {
	if root == nil || len(root.children) < 2 || len(source) == 0 || lang == nil || lang.Name != "haskell" {
		return
	}
	for i := 0; i+1 < len(root.children); i++ {
		left := root.children[i]
		right := root.children[i+1]
		if left == nil || right == nil {
			continue
		}
		if left.Type(lang) != "imports" {
			continue
		}
		if left.endByte >= right.startByte {
			continue
		}
		if left.endByte > uint32(len(source)) || right.startByte > uint32(len(source)) {
			continue
		}
		gap := source[left.endByte:right.startByte]
		if !bytesAreTrivia(gap) {
			continue
		}
		left.endByte = right.startByte
		left.endPoint = advancePointByBytes(left.endPoint, gap)
	}
}

func normalizeScalaTrailingCommentOwnership(root *Node, source []byte, lang *Language) {
	if root == nil || len(source) == 0 || lang == nil || lang.Name != "scala" {
		return
	}
	var walk func(*Node)
	walk = func(n *Node) {
		if n == nil {
			return
		}
		normalizeScalaTrailingCommentSiblings(n, source, lang)
		for _, child := range n.children {
			walk(child)
		}
	}
	walk(root)
}

func normalizeScalaTrailingCommentSiblings(parent *Node, source []byte, lang *Language) {
	if parent == nil || len(parent.children) < 3 {
		return
	}
	for i := 1; i+1 < len(parent.children); {
		firstComment := parent.children[i]
		if !isScalaCommentNode(firstComment, lang) {
			i++
			continue
		}
		prev := parent.children[i-1]
		body := scalaIndentedCommentTarget(prev, lang)
		if body == nil || body.endByte != firstComment.startByte {
			i++
			continue
		}
		j := i
		for j < len(parent.children) && isScalaCommentNode(parent.children[j], lang) {
			j++
		}
		if j >= len(parent.children) {
			i++
			continue
		}
		next := parent.children[j]
		if next == nil || next.isExtra {
			i++
			continue
		}
		lastComment := parent.children[j-1]

		targetEndByte := lastComment.endByte
		targetEndPoint := lastComment.endPoint
		if lastComment.endByte <= uint32(len(source)) && next.startByte >= lastComment.endByte && next.startByte <= uint32(len(source)) {
			gap := source[lastComment.endByte:next.startByte]
			if bytesAreTrivia(gap) {
				targetEndByte = next.startByte
				targetEndPoint = advancePointByBytes(lastComment.endPoint, gap)
			}
		}

		added := parent.children[i:j]
		rebuiltChildren := make([]*Node, 0, len(body.children)+len(added))
		rebuiltChildren = append(rebuiltChildren, body.children...)
		rebuiltChildren = append(rebuiltChildren, added...)
		body.children = rebuiltChildren

		if len(body.fieldIDs) > 0 {
			rebuiltFieldIDs := make([]FieldID, 0, len(body.fieldIDs)+len(added))
			rebuiltFieldIDs = append(rebuiltFieldIDs, body.fieldIDs...)
			for range added {
				rebuiltFieldIDs = append(rebuiltFieldIDs, 0)
			}
			body.fieldIDs = rebuiltFieldIDs
		}
		if len(body.fieldSources) > 0 {
			rebuiltFieldSources := make([]uint8, 0, len(body.fieldSources)+len(added))
			rebuiltFieldSources = append(rebuiltFieldSources, body.fieldSources...)
			for range added {
				rebuiltFieldSources = append(rebuiltFieldSources, 0)
			}
			body.fieldSources = rebuiltFieldSources
		}
		if targetEndByte > body.endByte {
			body.endByte = targetEndByte
			body.endPoint = targetEndPoint
		}
		if targetEndByte > prev.endByte {
			prev.endByte = targetEndByte
			prev.endPoint = targetEndPoint
		}

		parent.children = append(parent.children[:i], parent.children[j:]...)
		if len(parent.fieldIDs) > 0 {
			parent.fieldIDs = append(parent.fieldIDs[:i], parent.fieldIDs[j:]...)
			if len(parent.fieldSources) > 0 {
				parent.fieldSources = append(parent.fieldSources[:i], parent.fieldSources[j:]...)
			}
		}
	}
}

func isScalaCommentNode(n *Node, lang *Language) bool {
	if n == nil {
		return false
	}
	switch n.Type(lang) {
	case "comment", "block_comment":
		return true
	default:
		return false
	}
}

func scalaIndentedCommentTarget(prev *Node, lang *Language) *Node {
	if prev == nil || lang == nil || prev.Type(lang) != "function_definition" || len(prev.children) == 0 {
		return nil
	}
	last := prev.children[len(prev.children)-1]
	if last == nil || last.Type(lang) != "indented_block" {
		return nil
	}
	return last
}

func normalizeScalaFunctionModifierFields(root *Node, lang *Language) {
	if root == nil || lang == nil || lang.Name != "scala" {
		return
	}
	returnTypeID, ok := lang.FieldByName("return_type")
	if !ok {
		return
	}
	var walk func(*Node)
	walk = func(n *Node) {
		if n == nil {
			return
		}
		if n.Type(lang) == "function_definition" {
			for i, child := range n.children {
				if child == nil || child.Type(lang) != "modifiers" {
					continue
				}
				if i < len(n.fieldIDs) && n.fieldIDs[i] == returnTypeID {
					n.fieldIDs[i] = 0
					if i < len(n.fieldSources) {
						n.fieldSources[i] = fieldSourceNone
					}
				}
			}
		}
		for _, child := range n.children {
			walk(child)
		}
	}
	walk(root)
}

func normalizeScalaInterpolatedStringTail(root *Node, source []byte, lang *Language) {
	if root == nil || len(source) == 0 || lang == nil || lang.Name != "scala" {
		return
	}
	var walk func(*Node)
	walk = func(n *Node) {
		if n == nil {
			return
		}
		if n.Type(lang) == "interpolated_string_expression" && len(n.children) >= 2 {
			inner := n.children[1]
			if inner != nil && inner.Type(lang) == "interpolated_string" {
				normalizeScalaSingleLineInterpolatedStringTail(n, inner, source)
			}
		}
		if n.Type(lang) == "field_expression" && len(n.children) >= 2 {
			left := n.children[0]
			right := n.children[1]
			if left != nil && right != nil &&
				left.Type(lang) == "interpolated_string_expression" &&
				right.Type(lang) == "." &&
				left.endByte < right.startByte &&
				right.startByte <= uint32(len(source)) &&
				scalaInterpolatedStringTail(source[left.endByte:right.startByte]) {
				extendNodeEndTo(left, right.startByte, source)
				if len(left.children) >= 2 {
					inner := left.children[1]
					if inner != nil && inner.Type(lang) == "interpolated_string" {
						extendNodeEndTo(inner, right.startByte, source)
					}
				}
			}
		}
		for _, child := range n.children {
			walk(child)
		}
		if n.Type(lang) == "infix_expression" && len(n.children) > 0 {
			last := n.children[len(n.children)-1]
			if last != nil && last.Type(lang) == "interpolated_string_expression" && n.endByte < last.endByte {
				extendNodeEndTo(n, last.endByte, source)
			}
		}
	}
	walk(root)
}

func normalizeScalaSingleLineInterpolatedStringTail(expr *Node, inner *Node, source []byte) {
	if expr == nil || inner == nil || inner.startByte >= uint32(len(source)) {
		return
	}
	if source[inner.startByte] != '"' {
		return
	}
	if inner.startByte+2 < uint32(len(source)) &&
		source[inner.startByte+1] == '"' &&
		source[inner.startByte+2] == '"' {
		return
	}
	end, ok := scanScalaSingleLineStringTail(source, inner.endByte)
	if !ok || end <= inner.endByte {
		return
	}
	extendNodeEndTo(inner, end, source)
	extendNodeEndTo(expr, end, source)
}

func scalaInterpolatedStringTail(gap []byte) bool {
	if len(gap) == 0 {
		return false
	}
	hasQuote := false
	for _, c := range gap {
		switch c {
		case ' ', '\t', '\n', '\r', '\f', '|', '}', '"':
			if c == '"' {
				hasQuote = true
			}
		default:
			return false
		}
	}
	return hasQuote
}

func scanScalaSingleLineStringTail(source []byte, start uint32) (uint32, bool) {
	if start >= uint32(len(source)) {
		return 0, false
	}
	for i := start; i < uint32(len(source)); i++ {
		switch source[i] {
		case '\n', '\r':
			return 0, false
		case '"':
			if !isEscapedQuote(source, i) {
				return i + 1, true
			}
		}
	}
	return 0, false
}

func isEscapedQuote(source []byte, idx uint32) bool {
	if idx == 0 || idx > uint32(len(source)) {
		return false
	}
	backslashes := 0
	for i := int(idx) - 1; i >= 0 && source[i] == '\\'; i-- {
		backslashes++
	}
	return backslashes%2 == 1
}

func extendNodeEndTo(n *Node, end uint32, source []byte) {
	if n == nil || end <= n.endByte || end > uint32(len(source)) {
		return
	}
	gap := source[n.endByte:end]
	n.endByte = end
	n.endPoint = advancePointByBytes(n.endPoint, gap)
}

func advancePointByBytes(start Point, b []byte) Point {
	p := start
	for _, c := range b {
		if c == '\n' {
			p.Row++
			p.Column = 0
			continue
		}
		p.Column++
	}
	return p
}

func collapsePythonRootFragments(nodes []*Node, arena *nodeArena, lang *Language) []*Node {
	if len(nodes) == 0 || lang == nil || lang.Name != "python" {
		return nodes
	}
	nodes = dropZeroWidthUnnamedTail(nodes, lang)
	for {
		next, changed := collapsePythonClassFragments(nodes, arena, lang)
		if changed {
			nodes = next
			nodes = dropZeroWidthUnnamedTail(nodes, lang)
			continue
		}
		next, changed = collapsePythonTerminalIfSuffix(nodes, arena, lang)
		if changed {
			nodes = next
			nodes = dropZeroWidthUnnamedTail(nodes, lang)
			continue
		}
		return normalizePythonModuleChildren(nodes, arena, lang)
	}
}

func collapsePythonClassFragments(nodes []*Node, arena *nodeArena, lang *Language) ([]*Node, bool) {
	if len(nodes) < 5 {
		return nodes, false
	}
	classDefSym, ok := symbolByName(lang, "class_definition")
	if !ok {
		return nodes, false
	}
	blockSym, ok := symbolByName(lang, "block")
	if !ok {
		return nodes, false
	}
	for i := 0; i < len(nodes)-4; i++ {
		j := i
		classNode := nodes[j]
		nameNode := nodes[j+1]
		if classNode == nil || nameNode == nil {
			continue
		}
		if classNode.Type(lang) != "class" || nameNode.Type(lang) != "identifier" {
			continue
		}
		var argNode *Node
		if j+5 < len(nodes) && nodes[j+2] != nil && nodes[j+2].Type(lang) == "argument_list" {
			argNode = nodes[j+2]
			j++
		}
		colonNode := nodes[j+2]
		indentNode := nodes[j+3]
		bodyNode := nodes[j+4]
		if colonNode == nil || indentNode == nil || bodyNode == nil {
			continue
		}
		if colonNode.Type(lang) != ":" || indentNode.Type(lang) != "_indent" || bodyNode.Type(lang) != "module_repeat1" {
			continue
		}

		bodyChildren := flattenPythonModuleRepeat(bodyNode, nil, lang)
		if len(bodyChildren) == 0 {
			continue
		}
		if arena != nil {
			buf := arena.allocNodeSlice(len(bodyChildren))
			copy(buf, bodyChildren)
			bodyChildren = buf
		}
		blockNode := newParentNodeInArena(arena, blockSym, true, bodyChildren, nil, 0)
		blockNode.hasError = false

		classChildren := make([]*Node, 0, 5)
		classChildren = append(classChildren, classNode, nameNode)
		if argNode != nil {
			classChildren = append(classChildren, argNode)
		}
		classChildren = append(classChildren, colonNode, blockNode)
		if arena != nil {
			buf := arena.allocNodeSlice(len(classChildren))
			copy(buf, classChildren)
			classChildren = buf
		}
		classFieldIDs := pythonSyntheticClassFieldIDs(arena, len(classChildren), argNode != nil, lang)
		classDef := newParentNodeInArena(arena, classDefSym, true, classChildren, classFieldIDs, 0)
		classDef.hasError = false

		out := make([]*Node, 0, len(nodes)-(j+5-i)+1)
		out = append(out, nodes[:i]...)
		out = append(out, classDef)
		out = append(out, nodes[j+5:]...)
		if arena != nil {
			buf := arena.allocNodeSlice(len(out))
			copy(buf, out)
			out = buf
		}
		return out, true
	}
	return nodes, false
}

func collapsePythonTerminalIfSuffix(nodes []*Node, arena *nodeArena, lang *Language) ([]*Node, bool) {
	if len(nodes) < 6 {
		return nodes, false
	}
	ifSym, ok := symbolByName(lang, "if_statement")
	if !ok {
		return nodes, false
	}
	blockSym, ok := symbolByName(lang, "block")
	if !ok {
		return nodes, false
	}
	n := len(nodes)
	ifNode := nodes[n-6]
	condNode := nodes[n-5]
	colonNode := nodes[n-4]
	indentNode := nodes[n-3]
	bodyNode := nodes[n-2]
	dedentNode := nodes[n-1]
	if ifNode == nil || condNode == nil || colonNode == nil || indentNode == nil || bodyNode == nil || dedentNode == nil {
		return nodes, false
	}
	if ifNode.Type(lang) != "if" || colonNode.Type(lang) != ":" || indentNode.Type(lang) != "_indent" || bodyNode.Type(lang) != "_simple_statements" || dedentNode.Type(lang) != "_dedent" {
		return nodes, false
	}
	if !condNode.IsNamed() {
		return nodes, false
	}

	blockChildren := []*Node{indentNode, bodyNode, dedentNode}
	if arena != nil {
		buf := arena.allocNodeSlice(len(blockChildren))
		copy(buf, blockChildren)
		blockChildren = buf
	}
	blockNode := newParentNodeInArena(arena, blockSym, true, blockChildren, nil, 0)
	blockNode.hasError = false

	ifChildren := []*Node{ifNode, condNode, colonNode, blockNode}
	if arena != nil {
		buf := arena.allocNodeSlice(len(ifChildren))
		copy(buf, ifChildren)
		ifChildren = buf
	}
	ifFieldIDs := pythonSyntheticIfFieldIDs(arena, len(ifChildren), lang)
	ifStmt := newParentNodeInArena(arena, ifSym, true, ifChildren, ifFieldIDs, 0)
	ifStmt.hasError = false

	out := make([]*Node, 0, n-5)
	out = append(out, nodes[:n-6]...)
	out = append(out, ifStmt)
	if arena != nil {
		buf := arena.allocNodeSlice(len(out))
		copy(buf, out)
		return buf, true
	}
	return out, true
}

func flattenPythonModuleRepeat(node *Node, out []*Node, lang *Language) []*Node {
	if node == nil {
		return out
	}
	if node.Type(lang) == "module_repeat1" {
		for _, child := range node.children {
			out = flattenPythonModuleRepeat(child, out, lang)
		}
		return out
	}
	if node.IsNamed() {
		out = append(out, node)
	}
	return out
}

func normalizePythonModuleChildren(nodes []*Node, arena *nodeArena, lang *Language) []*Node {
	if len(nodes) == 0 || lang == nil || lang.Name != "python" {
		return nodes
	}
	out := make([]*Node, 0, len(nodes))
	changed := false
	for _, node := range nodes {
		if node == nil {
			continue
		}
		normalized, nodeChanged := normalizePythonModuleNode(node, lang)
		if nodeChanged {
			out = append(out, normalized)
			changed = true
			continue
		}
		out = append(out, node)
	}
	if !changed {
		return nodes
	}
	if arena != nil {
		buf := arena.allocNodeSlice(len(out))
		copy(buf, out)
		return buf
	}
	return out
}

func normalizePythonModuleNode(node *Node, lang *Language) (*Node, bool) {
	changed := false
	for node != nil {
		if node.Type(lang) == "_simple_statements" && len(node.children) == 1 {
			child := node.children[0]
			if child != nil && child.IsNamed() {
				node = child
				changed = true
				continue
			}
		}
		if node.Type(lang) == "expression_statement" && len(node.children) == 1 {
			child := node.children[0]
			if child != nil && child.IsNamed() {
				node = child
				changed = true
				continue
			}
		}
		if (node.Type(lang) == "expression" || node.Type(lang) == "primary_expression") && len(node.children) == 1 {
			child := node.children[0]
			if child != nil && child.IsNamed() {
				node = child
				changed = true
				continue
			}
		}
		break
	}
	return node, changed
}

func repairPythonRootNode(root *Node, arena *nodeArena, lang *Language) *Node {
	if root == nil || lang == nil || lang.Name != "python" || root.Type(lang) != "module" {
		return root
	}
	children := collapsePythonRootFragments(root.children, arena, lang)
	changed := len(children) != len(root.children)
	if !changed {
		for i := range children {
			if children[i] != root.children[i] {
				changed = true
				break
			}
		}
	}

	repaired := make([]*Node, 0, len(children))
	for _, child := range children {
		fixed := repairPythonTopLevelNode(child, arena, lang)
		if fixed != child {
			changed = true
		}
		repaired = append(repaired, fixed)
	}

	if !changed {
		if root.hasError && pythonModuleChildrenLookComplete(repaired, lang) {
			cloned := cloneNodeInArena(arena, root)
			cloned.hasError = false
			return cloned
		}
		return root
	}

	cloned := cloneNodeInArena(arena, root)
	if arena != nil {
		buf := arena.allocNodeSlice(len(repaired))
		copy(buf, repaired)
		repaired = buf
	}
	cloned.children = repaired
	cloned.fieldIDs = nil
	cloned.fieldSources = nil
	if pythonModuleChildrenLookComplete(repaired, lang) {
		cloned.hasError = false
	}
	return cloned
}

func repairPythonTopLevelNode(node *Node, arena *nodeArena, lang *Language) *Node {
	if node == nil || lang == nil || lang.Name != "python" {
		return node
	}
	return repairPythonNode(node, arena, lang)
}

func repairPythonNode(node *Node, arena *nodeArena, lang *Language) *Node {
	if node == nil || lang == nil || lang.Name != "python" {
		return node
	}
	normalized, changed := normalizePythonModuleNode(node, lang)
	if changed {
		node = normalized
	}
	switch node.Type(lang) {
	case "class_definition":
		return repairPythonClassDefinition(node, arena, lang)
	case "function_definition":
		return repairPythonFunctionDefinition(node, arena, lang)
	case "if_statement":
		return repairPythonIfStatement(node, arena, lang)
	case "block":
		repaired, _ := repairPythonBlock(node, arena, lang, false)
		return repaired
	default:
		return node
	}
}

func repairPythonClassDefinition(node *Node, arena *nodeArena, lang *Language) *Node {
	if node == nil || node.Type(lang) != "class_definition" || len(node.children) == 0 {
		return node
	}
	bodyIndex := -1
	for i, child := range node.children {
		if child != nil && child.Type(lang) == "block" {
			bodyIndex = i
		}
	}
	if bodyIndex < 0 {
		return node
	}
	body := node.children[bodyIndex]
	repairedBody, changed := repairPythonBlock(body, arena, lang, true)
	if !changed {
		return node
	}

	cloned := cloneNodeInArena(arena, node)
	children := make([]*Node, len(node.children))
	copy(children, node.children)
	children[bodyIndex] = repairedBody
	if arena != nil {
		buf := arena.allocNodeSlice(len(children))
		copy(buf, children)
		children = buf
	}
	cloned.children = children
	if repairedBody != nil {
		cloned.endByte = repairedBody.endByte
		cloned.endPoint = repairedBody.endPoint
	}
	return cloned
}

func repairPythonFunctionDefinition(node *Node, arena *nodeArena, lang *Language) *Node {
	if node == nil || node.Type(lang) != "function_definition" || len(node.children) == 0 {
		return node
	}
	bodyIndex := -1
	for i, child := range node.children {
		if child != nil && child.Type(lang) == "block" {
			bodyIndex = i
		}
	}
	if bodyIndex < 0 {
		return node
	}
	body := node.children[bodyIndex]
	repairedBody, changed := repairPythonBlock(body, arena, lang, false)
	if !changed {
		return node
	}

	cloned := cloneNodeInArena(arena, node)
	children := make([]*Node, len(node.children))
	copy(children, node.children)
	children[bodyIndex] = repairedBody
	if arena != nil {
		buf := arena.allocNodeSlice(len(children))
		copy(buf, children)
		children = buf
	}
	cloned.children = children
	if repairedBody != nil {
		cloned.endByte = repairedBody.endByte
		cloned.endPoint = repairedBody.endPoint
	}
	return cloned
}

func repairPythonIfStatement(node *Node, arena *nodeArena, lang *Language) *Node {
	if node == nil || node.Type(lang) != "if_statement" || len(node.children) == 0 {
		return node
	}
	children := make([]*Node, len(node.children))
	changed := false
	for i, child := range node.children {
		repaired := repairPythonNode(child, arena, lang)
		if repaired != child {
			changed = true
		}
		children[i] = repaired
	}
	if !changed {
		return node
	}

	cloned := cloneNodeInArena(arena, node)
	if arena != nil {
		buf := arena.allocNodeSlice(len(children))
		copy(buf, children)
		children = buf
	}
	cloned.children = children
	last := children[len(children)-1]
	if last != nil {
		cloned.endByte = last.endByte
		cloned.endPoint = last.endPoint
	}
	return cloned
}

func repairPythonBlock(node *Node, arena *nodeArena, lang *Language, allowHoist bool) (*Node, bool) {
	if node == nil || node.Type(lang) != "block" {
		return node, false
	}
	pending := append([]*Node(nil), node.children...)
	out := make([]*Node, 0, len(node.children))
	changed := false

	for len(pending) > 0 {
		cur := pending[0]
		pending = pending[1:]
		if cur == nil {
			continue
		}
		norm, normChanged := normalizePythonModuleNode(cur, lang)
		if normChanged {
			changed = true
		}
		cur = norm
		if cur != nil {
			switch cur.Type(lang) {
			case "_indent", "_dedent":
				changed = true
				continue
			}
		}

		if allowHoist && cur != nil && cur.Type(lang) == "function_definition" {
			repairedFn, hoisted, split := splitPythonOvernestedFunction(cur, arena, lang)
			if split {
				changed = true
				repairedFn = repairPythonNode(repairedFn, arena, lang)
				out = append(out, repairedFn)
				if len(hoisted) > 0 {
					pending = append(append([]*Node{}, hoisted...), pending...)
				}
				continue
			}
		}

		repaired := repairPythonNode(cur, arena, lang)
		if repaired != cur {
			changed = true
		}
		out = append(out, repaired)
	}

	if !changed {
		firstNamed := pythonBlockStartAnchor(out, lang)
		lastSpan := pythonBlockEndAnchor(out)
		if firstNamed == nil || lastSpan == nil {
			return node, false
		}
		if node.startByte == firstNamed.startByte &&
			node.startPoint == firstNamed.startPoint &&
			node.endByte == lastSpan.endByte &&
			node.endPoint == lastSpan.endPoint {
			return node, false
		}
		changed = true
	}

	cloned := cloneNodeInArena(arena, node)
	if arena != nil {
		buf := arena.allocNodeSlice(len(out))
		copy(buf, out)
		out = buf
	}
	cloned.children = out
	cloned.fieldIDs = nil
	cloned.fieldSources = nil
	firstNamed := pythonBlockStartAnchor(out, lang)
	lastSpan := pythonBlockEndAnchor(out)
	if firstNamed != nil {
		cloned.startByte = firstNamed.startByte
		cloned.startPoint = firstNamed.startPoint
	}
	if lastSpan != nil {
		cloned.endByte = lastSpan.endByte
		cloned.endPoint = lastSpan.endPoint
	}
	return cloned, true
}

func pythonBlockStartAnchor(children []*Node, lang *Language) *Node {
	for _, child := range children {
		if child == nil {
			continue
		}
		typ := child.Type(lang)
		if typ == "_indent" || typ == "_dedent" {
			continue
		}
		if child.endByte > child.startByte || child.IsNamed() {
			return child
		}
	}
	return nil
}

func pythonBlockEndAnchor(children []*Node) *Node {
	for i := len(children) - 1; i >= 0; i-- {
		child := children[i]
		if child != nil && child.endByte > child.startByte {
			return child
		}
	}
	return nil
}

func splitPythonOvernestedFunction(node *Node, arena *nodeArena, lang *Language) (*Node, []*Node, bool) {
	if node == nil || node.Type(lang) != "function_definition" {
		return node, nil, false
	}
	bodyIndex := -1
	for i, child := range node.children {
		if child != nil && child.Type(lang) == "block" {
			bodyIndex = i
		}
	}
	if bodyIndex < 0 {
		return node, nil, false
	}
	body := node.children[bodyIndex]
	if body == nil || len(body.children) == 0 {
		return node, nil, false
	}
	fnColumn := node.startPoint.Column
	hoistStart := -1
	for i, child := range body.children {
		if child == nil || !child.IsNamed() {
			continue
		}
		if child.startPoint.Column <= fnColumn {
			hoistStart = i
			break
		}
	}
	if hoistStart <= 0 {
		return node, nil, false
	}

	kept := append([]*Node(nil), body.children[:hoistStart]...)
	hoisted := append([]*Node(nil), body.children[hoistStart:]...)
	if len(kept) == 0 {
		return node, nil, false
	}

	newBody := cloneNodeInArena(arena, body)
	if arena != nil {
		buf := arena.allocNodeSlice(len(kept))
		copy(buf, kept)
		kept = buf
	}
	newBody.children = kept
	newBody.fieldIDs = nil
	newBody.fieldSources = nil
	lastKept := kept[len(kept)-1]
	newBody.endByte = lastKept.endByte
	newBody.endPoint = lastKept.endPoint

	newFn := cloneNodeInArena(arena, node)
	fnChildren := make([]*Node, len(node.children))
	copy(fnChildren, node.children)
	fnChildren[bodyIndex] = newBody
	if arena != nil {
		buf := arena.allocNodeSlice(len(fnChildren))
		copy(buf, fnChildren)
		fnChildren = buf
	}
	newFn.children = fnChildren
	newFn.endByte = newBody.endByte
	newFn.endPoint = newBody.endPoint
	return newFn, hoisted, true
}

func pythonSyntheticClassFieldIDs(arena *nodeArena, childCount int, hasArgList bool, lang *Language) []FieldID {
	fieldIDs := make([]FieldID, childCount)
	if arena != nil {
		fieldIDs = arena.allocFieldIDSlice(childCount)
	}
	if fid, ok := lang.FieldByName("name"); ok && childCount > 1 {
		fieldIDs[1] = fid
	}
	if hasArgList {
		if fid, ok := lang.FieldByName("superclasses"); ok && childCount > 2 {
			fieldIDs[2] = fid
		}
		if fid, ok := lang.FieldByName("body"); ok && childCount > 4 {
			fieldIDs[4] = fid
		}
		return fieldIDs
	}
	if fid, ok := lang.FieldByName("body"); ok && childCount > 3 {
		fieldIDs[3] = fid
	}
	return fieldIDs
}

func pythonSyntheticIfFieldIDs(arena *nodeArena, childCount int, lang *Language) []FieldID {
	fieldIDs := make([]FieldID, childCount)
	if arena != nil {
		fieldIDs = arena.allocFieldIDSlice(childCount)
	}
	if fid, ok := lang.FieldByName("condition"); ok && childCount > 1 {
		fieldIDs[1] = fid
	}
	if fid, ok := lang.FieldByName("consequence"); ok && childCount > 3 {
		fieldIDs[3] = fid
	}
	return fieldIDs
}

func pythonModuleChildrenLookComplete(nodes []*Node, lang *Language) bool {
	if len(nodes) == 0 {
		return false
	}
	seen := 0
	for _, n := range nodes {
		if n == nil || n.isExtra {
			continue
		}
		if n.IsNamed() {
			seen++
			continue
		}
		switch n.Type(lang) {
		case "_simple_statements":
			seen++
		default:
			return false
		}
	}
	return seen > 0
}

func dropZeroWidthUnnamedTail(nodes []*Node, lang *Language) []*Node {
	for len(nodes) > 0 {
		last := nodes[len(nodes)-1]
		if last == nil {
			nodes = nodes[:len(nodes)-1]
			continue
		}
		if last.IsNamed() || last.startByte != last.endByte || len(last.children) > 0 {
			break
		}
		if lang != nil && last.Type(lang) != "" {
			break
		}
		nodes = nodes[:len(nodes)-1]
	}
	return nodes
}

func symbolByName(lang *Language, name string) (Symbol, bool) {
	if lang == nil {
		return 0, false
	}
	for i, symName := range lang.SymbolNames {
		if symName == name {
			return Symbol(i), true
		}
	}
	return 0, false
}
