package gotreesitter

type resultRootBuild struct {
	parser                *Parser
	source                []byte
	arena                 *nodeArena
	reuseState            *parseReuseState
	linkScratch           *[]*Node
	lang                  *Language
	expectedRootSymbol    Symbol
	hasExpectedRoot       bool
	shouldWireParentLinks bool
	borrowedResolved      bool
	borrowed              []*nodeArena
}

func newResultRootBuild(p *Parser, source []byte, arena *nodeArena, oldTree *Tree, reuseState *parseReuseState, linkScratch *[]*Node) resultRootBuild {
	build := resultRootBuild{
		parser:                p,
		source:                source,
		arena:                 arena,
		reuseState:            reuseState,
		linkScratch:           linkScratch,
		shouldWireParentLinks: oldTree == nil,
	}
	if p != nil {
		build.lang = p.language
		if p.hasRootSymbol {
			build.expectedRootSymbol = p.rootSymbol
			build.hasExpectedRoot = true
		}
	}
	if oldTree != nil && oldTree.RootNode() != nil {
		build.expectedRootSymbol = oldTree.RootNode().symbol
		build.hasExpectedRoot = true
	}
	return build
}

func (b *resultRootBuild) prepareRootNodes(nodes []*Node) []*Node {
	if b.isLanguage("python") {
		nodes = b.repairPythonKeywordNodes(nodes)
		nodes = collapsePythonRootFragments(nodes, b.arena, b.lang)
	}
	if b.hasExpectedRoot && len(nodes) > 1 {
		nodes = flattenRootSelfFragments(nodes, b.arena, b.expectedRootSymbol)
	}
	return nodes
}

func (b *resultRootBuild) buildSingleRootTree(candidate *Node) *Tree {
	candidate = flattenInvisibleRootChildren(candidate, b.arena, b.lang)
	candidate = b.repairPythonKeywordNode(candidate)
	candidate = b.repairPythonRoot(candidate)
	if !b.hasExpectedRoot || candidate.symbol == b.expectedRootSymbol {
		return b.finishTree(candidate, b.shouldWireParentLinks, true)
	}
	return b.buildExpectedRootWrapperTree(candidate)
}

func (b *resultRootBuild) buildExpectedRootWrapperTree(child *Node) *Tree {
	root := newParentNodeInArena(b.arena, b.expectedRootSymbol, true, b.singleChildSlice(child), nil, 0)
	return b.finishTree(root, b.shouldWireParentLinks, true)
}

func (b *resultRootBuild) tryBuildRealRootTree(nodes []*Node) *Tree {
	extraSplit := splitResultRootExtras(nodes, b.lang)
	realRoot := extraSplit.realRoot
	if realRoot == nil {
		return nil
	}
	returnRealRoot := !b.hasExpectedRoot || realRoot.symbol == b.expectedRootSymbol
	if b.reuseState != nil && b.reuseState.reusedAny {
		realRoot = cloneNodeInArena(b.arena, realRoot)
		realRoot.parent = nil
		realRoot.childIndex = -1
	}
	if returnRealRoot && extraSplit.canFoldVisibleExtras() {
		foldResultRootExtras(realRoot, extraSplit.visibleExtras, b.arena)
	}
	if returnRealRoot {
		extendResultRootRangeToExtras(realRoot, extraSplit.allExtras)
	}
	realRoot = b.repairPythonRoot(realRoot)
	extendTrailing := returnRealRoot || !realRoot.hasError()
	if !returnRealRoot {
		b.finalizeRoot(realRoot, false, extendTrailing)
		return nil
	}
	return b.finishTree(realRoot, b.shouldWireParentLinks, extendTrailing)
}

func (b *resultRootBuild) buildSyntheticRootTree(nodes []*Node) *Tree {
	rootChildren := filterZeroWidthExtras(nodes, b.arena)
	rootChildren = b.repairPythonKeywordNodes(rootChildren)
	rootHasError := resultNodesHaveError(rootChildren)
	rootSymbol := b.syntheticRootSymbol(nodes, rootChildren, rootHasError)
	root := newParentNodeInArena(b.arena, rootSymbol, true, rootChildren, nil, 0)
	if rootHasError && !b.syntheticRootCanDropError(rootChildren) {
		root.setHasError(true)
	}
	root = b.repairPythonRoot(root)
	return b.finishTree(root, b.shouldWireParentLinks, true)
}

func (b *resultRootBuild) syntheticRootSymbol(originalNodes, rootChildren []*Node, rootHasError bool) Symbol {
	rootSymbol := rootChildren[len(rootChildren)-1].symbol
	if !b.hasExpectedRoot {
		return rootSymbol
	}
	if !rootHasError {
		return b.expectedRootSymbol
	}
	if b.isLanguage("dart") && dartProgramChildrenLookComplete(originalNodes, b.lang) {
		return b.expectedRootSymbol
	}
	return errorSymbol
}

func (b *resultRootBuild) syntheticRootCanDropError(rootChildren []*Node) bool {
	return b.isLanguage("python") && b.hasExpectedRoot && pythonModuleChildrenLookComplete(rootChildren, b.lang)
}

func (b *resultRootBuild) repairPythonKeywordNode(node *Node) *Node {
	timing := b.parser.currentMaterializationTiming()
	start := materializationTimingStart(timing)
	node = repairPythonKeywordErrorNode(node, b.source, b.arena, b.lang)
	timing.addPythonKeywordRepair(start)
	return node
}

func (b *resultRootBuild) repairPythonKeywordNodes(nodes []*Node) []*Node {
	timing := b.parser.currentMaterializationTiming()
	start := materializationTimingStart(timing)
	nodes, _ = repairPythonKeywordErrorNodes(nodes, b.source, b.arena, b.lang)
	timing.addPythonKeywordRepair(start)
	return nodes
}

func (b *resultRootBuild) repairPythonRoot(root *Node) *Node {
	timing := b.parser.currentMaterializationTiming()
	start := materializationTimingStart(timing)
	root = repairPythonRootNode(root, b.arena, b.lang)
	timing.addPythonRootRepair(start)
	return root
}

func (b *resultRootBuild) singleChildSlice(child *Node) []*Node {
	if b.arena != nil {
		children := b.arena.allocNodeSlice(1)
		children[0] = child
		return children
	}
	return []*Node{child}
}

func (b *resultRootBuild) finishTree(root *Node, wireParentLinks, extendTrailing bool) *Tree {
	b.finalizeRoot(root, wireParentLinks, extendTrailing)
	tree := newTreeWithArenas(root, b.source, b.lang, b.arena, b.borrowedArenas())
	if b.parser.shouldDeferResultCompatibility(root) {
		tree.deferResultCompatibility()
	}
	return tree
}

func (b *resultRootBuild) finalizeRoot(root *Node, wireParentLinks, extendTrailing bool) {
	b.parser.finalizeResultRoot(root, b.source, b.linkScratch, wireParentLinks, extendTrailing)
}

func (b *resultRootBuild) borrowedArenas() []*nodeArena {
	if b.borrowedResolved {
		return b.borrowed
	}
	b.borrowed = b.reuseState.retainBorrowed(b.arena)
	b.borrowedResolved = true
	return b.borrowed
}

func (b *resultRootBuild) isLanguage(name string) bool {
	return b.lang != nil && b.lang.Name == name
}

func resultNodesHaveError(nodes []*Node) bool {
	for _, node := range nodes {
		if node != nil && (node.IsError() || node.HasError()) {
			return true
		}
	}
	return false
}

func retagResultRoot(root *Node, sym Symbol, named bool) {
	if root == nil {
		return
	}
	root.symbol = sym
	root.setNamed(named)
}

func retagResultRootAndRefreshError(root *Node, sym Symbol, named bool) {
	retagResultRoot(root, sym, named)
	refreshResultRootError(root)
}

func refreshResultRootError(root *Node) {
	if root == nil {
		return
	}
	for i := 0; i < resultChildCount(root); i++ {
		child := resultChildAt(root, i)
		if child != nil && (child.IsError() || child.HasError()) {
			root.setHasError(true)
			return
		}
	}
	root.setHasError(false)
}

type resultRootExtraSplit struct {
	realRoot      *Node
	allExtras     []*Node
	visibleExtras []*Node
}

func splitResultRootExtras(nodes []*Node, lang *Language) resultRootExtraSplit {
	var split resultRootExtraSplit
	for _, n := range nodes {
		if n.isExtra() {
			split.allExtras = append(split.allExtras, n)
			if symbolIsVisible(lang, n.symbol) && n.endByte > n.startByte {
				split.visibleExtras = append(split.visibleExtras, n)
			}
			continue
		}
		if split.realRoot != nil {
			split.realRoot = nil
			return split
		}
		split.realRoot = n
	}
	return split
}

func (s resultRootExtraSplit) canFoldVisibleExtras() bool {
	if len(s.visibleExtras) == 0 {
		return false
	}
	for _, extra := range s.allExtras {
		if extra != nil && (extra.IsError() || extra.HasError()) {
			return false
		}
	}
	return true
}

func foldResultRootExtras(root *Node, extras []*Node, arena *nodeArena) {
	if root == nil || len(extras) == 0 {
		return
	}
	var leadingExtras []*Node
	var trailingExtras []*Node
	for _, extra := range extras {
		if extra.startByte <= root.startByte {
			leadingExtras = append(leadingExtras, extra)
		} else {
			trailingExtras = append(trailingExtras, extra)
		}
	}
	if resultMutableChildrenForMutation(root).SurroundFinalRefs(leadingExtras, trailingExtras) {
		extendResultRootRangeToExtras(root, extras)
		return
	}
	rootChildren := resultChildSliceForMutation(root)
	merged := make([]*Node, 0, len(extras)+len(rootChildren))
	leadingCount := 0
	for _, extra := range leadingExtras {
		merged = append(merged, extra)
		leadingCount++
	}
	merged = append(merged, rootChildren...)
	merged = append(merged, trailingExtras...)
	if arena != nil {
		out := arena.allocNodeSlice(len(merged))
		copy(out, merged)
		merged = out
	}
	root.children = merged

	if len(root.fieldIDs) > 0 {
		trailingCount := len(extras) - leadingCount
		padded := make([]FieldID, leadingCount+len(root.fieldIDs)+trailingCount)
		copy(padded[leadingCount:], root.fieldIDs)
		root.fieldIDs = padded
		if len(root.fieldSources) > 0 {
			paddedSources := make([]uint8, len(padded))
			copy(paddedSources[leadingCount:], root.fieldSources)
			root.fieldSources = paddedSources
		}
	}
	extendResultRootRangeToExtras(root, extras)
}

func extendResultRootRangeToExtras(root *Node, extras []*Node) {
	if root == nil {
		return
	}
	for _, extra := range extras {
		if extra == nil {
			continue
		}
		if extra.startByte < root.startByte {
			root.startByte = extra.startByte
			root.startPoint = extra.startPoint
		}
		if extra.endByte > root.endByte {
			root.endByte = extra.endByte
			root.endPoint = extra.endPoint
		}
	}
}

func (p *Parser) finalizeResultRoot(root *Node, source []byte, linkScratch *[]*Node, wireParentLinks, extendTrailing bool) {
	if root == nil {
		return
	}
	timing := p.currentMaterializationTiming()
	finalizeStart := materializationTimingStart(timing)
	defer timing.addResultFinalizeRoot(finalizeStart)
	if extendTrailing {
		start := materializationTimingStart(timing)
		extendNodeToTrailingWhitespace(root, source)
		timing.addResultExtendTrailing(start)
	}
	start := materializationTimingStart(timing)
	p.normalizeRootSourceStart(root, source)
	timing.addResultNormalizeRootStart(start)
	if p == nil || (!p.noResultCompatibilityBenchmarkOnly && !p.shouldDeferResultCompatibility(root)) {
		start = materializationTimingStart(timing)
		normalizeResultCompatibility(root, source, p)
		timing.addResultCompatibility(start)
	}
	if wireParentLinks {
		start = materializationTimingStart(timing)
		if p != nil && p.shouldDeferResultParentLinks(root) {
			root.ownerArena.deferParentLinks(root)
		} else {
			wireParentLinksWithScratch(root, linkScratch)
		}
		timing.addResultParentLink(start)
	}
}

func (p *Parser) shouldDeferResultCompatibility(root *Node) bool {
	if p == nil || p.language == nil || root == nil || p.noResultCompatibilityBenchmarkOnly || p.noTreeBenchmarkOnly {
		return false
	}
	if !parseTypeScriptLazyResultCompatibilityEnabled() {
		return false
	}
	switch p.language.Name {
	case "typescript", "tsx":
		return true
	default:
		return false
	}
}

func (p *Parser) shouldDeferResultParentLinks(root *Node) bool {
	if p == nil || p.language == nil || root == nil || root.ownerArena == nil {
		return false
	}
	if p.noResultCompatibilityBenchmarkOnly && !p.noTreeBenchmarkOnly {
		return true
	}
	if p.noTreeBenchmarkOnly {
		return false
	}
	switch p.language.Name {
	case "java", "python", "typescript", "tsx":
		return true
	default:
		return false
	}
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
