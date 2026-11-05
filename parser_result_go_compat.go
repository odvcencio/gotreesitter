package gotreesitter

import (
	"bytes"
	"math"
)

const goSemicolonContainerSymbolCount = 11

type goCompatibilitySymbols struct {
	semicolon         Symbol
	semicolonSentinel Symbol
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
	siblingBoundary  bool
	trailingBoundary bool
}

// compatRuntimeMemoryBudgetStopReason forces a real runtime memory-budget
// check and, when it trips, latches p.compatMemoryBudgetTripped. Every
// compat call site that needs memory-budget awareness — the Go compat
// walk-internal poller's memoryBudgetParser hook and goCompatMemoryBudgetStopReason's
// per-stage boundary check (this file / parser_result_go.go), and their JS/TS
// fused-walk counterparts (javaScriptTypeScriptCompatMemoryBudgetStopReason
// and the fused walk's own poller.memoryBudgetParser hook, parser_result_javascript_typescript.go)
// — goes through this shared helper instead of calling
// runtimeMemoryBudgetStopReasonNow directly, so a trip detected anywhere in
// either compat pipeline is reliably surfaced later by
// resultMaterializationStopReason (see compatMemoryBudgetTripped's doc
// comment on the Parser struct for why a plain re-check is not enough).
func (p *Parser) compatRuntimeMemoryBudgetStopReason() ParseStopReason {
	if p == nil {
		return ParseStopNone
	}
	if reason := p.runtimeMemoryBudgetStopReasonNow(); reason == ParseStopMemoryBudget {
		p.compatMemoryBudgetTripped = true
		return reason
	}
	return ParseStopNone
}

func normalizeGoCompatibilityWithParser(root *Node, source []byte, lang *Language, p *Parser) ParseStopReason {
	return normalizeGoCompatibilityInRangesWithParser(root, source, lang, nil, p)
}

func normalizeGoCompatibilityInRangesWithParser(root *Node, source []byte, lang *Language, incrementalRanges []Range, p *Parser) ParseStopReason {
	var frames *[]goCompatSubtreeFrame
	var stopCheck parseStopCheck
	if p != nil {
		frames = p.goCompatFrames
		stopCheck = p.activeParseStopCheck()
	}
	return normalizeGoCompatibilityInRangesWithStopAndScratch(root, source, lang, incrementalRanges, stopCheck, frames, p)
}

// normalizeGoCompatibilityInRangesWithStopAndScratch drives the Go compat
// walk. memBudgetParser, when non-nil, is wired into the poller so the walk
// itself polls the runtime memory budget (see parseStopPoller.pollNow and
// walkGoCompatSubtree) instead of only ever reacting to timeout/cancellation:
// a C-recovery-widened Go tree can make the walk's own per-node work
// (semicolon-container filtering, sibling-boundary rewrites) balloon heap
// growth independent of whatever the parse loop itself already enforced (see
// the 2026-07-12 gocompat-walk-containment-gap finding).
// resultMaterializationShouldStop (not parseStopReasonIsActive) gates every
// bail-out below so a ParseStopMemoryBudget the poller reports is honored the
// same way Timeout/Cancelled already are.
func normalizeGoCompatibilityInRangesWithStopAndScratch(root *Node, source []byte, lang *Language, incrementalRanges []Range, stopCheck parseStopCheck, frames *[]goCompatSubtreeFrame, memBudgetParser *Parser) ParseStopReason {
	if root == nil || lang == nil || lang.Name != "go" || len(source) == 0 {
		return ParseStopNone
	}
	poller := parseStopPoller{check: stopCheck, memoryBudgetParser: memBudgetParser}
	if reason := poller.pollNow(); resultMaterializationShouldStop(reason) {
		return reason
	}
	flags := goCompatibilitySourceFlagsFor(source)
	syms, ok := goCompatibilitySymbolsForLanguage(lang)
	if !ok {
		return poller.pollNow()
	}
	if reason := normalizeGoCompatibilitySubtreeWithStopAndScratch(root, source, syms, flags, incrementalRanges, &poller, frames); resultMaterializationShouldStop(reason) {
		return reason
	}
	return poller.pollNow()
}

func goCompatibilitySourceFlagsFor(source []byte) goCompatibilitySourceFlags {
	return goCompatibilitySourceFlags{
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

// goNormalizerPopBudget bounds the fast-path DFS pop count. A well-formed go
// tree has far fewer than this many nodes (the node count is a small multiple of
// the token count, itself bounded by the source length), so the budget is never
// reached on legitimate input; it only trips on a cyclic/DAG transient tree,
// where it caps wasted work at O(len(source)) before the deduped retry.
//
// The arithmetic runs in int64 and clamps to math.MaxInt before narrowing back
// to int: on a 32-bit platform (int is 32 bits) a >33MB source would otherwise
// overflow 64*(sourceLen+1) past math.MaxInt32 and wrap negative, making every
// pops > budget check trivially true (or false, depending on the wrapped
// sign) instead of acting as an actual budget. Clamping preserves the intended
// "practically unbounded for legitimate input" semantics on both platforms;
// on 64-bit the clamp is unreachable for any realistic source length, so the
// constant's behavior there is unchanged.
func goNormalizerPopBudget(sourceLen int) int {
	budget := 64*(int64(sourceLen)+1) + 1<<16
	if budget > int64(math.MaxInt) {
		return math.MaxInt
	}
	return int(budget)
}

func goCompatibilitySymbolsForLanguage(lang *Language) (goCompatibilitySymbols, bool) {
	var syms goCompatibilitySymbols
	syms.semicolon, _ = symbolByName(lang, ";")
	syms.semicolonSentinel, _ = symbolByName(lang, "\x00")
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

type goCompatSubtreeFrame struct {
	node *Node
	exit bool
}

func normalizeGoCompatibilitySubtreeWithStopAndScratch(n *Node, source []byte, syms goCompatibilitySymbols, flags goCompatibilitySourceFlags, incrementalRanges []Range, poller *parseStopPoller, frames *[]goCompatSubtreeFrame) ParseStopReason {
	if n == nil {
		return ParseStopNone
	}
	// Fast path (no dedup) with a pop budget, then a deduped retry if the
	// budget trips: a well-formed tree never re-pushes a node, so the fast
	// path terminates in O(nodes) and is byte-for-byte the original walk. The
	// budget guards against a recovery-mode transient tree that is a
	// DAG/cycle (the same *Node reachable via many parents or a back-edge),
	// which would otherwise re-push shared nodes along every path. Every
	// per-node mutation here (semicolon-drop, sibling-boundary and
	// trailing-extra span adjustment) is idempotent and keyed on node
	// identity, so descending each distinct node once matches the
	// well-formed fast path byte-for-byte.
	var stack []goCompatSubtreeFrame
	if frames != nil {
		stack = (*frames)[:0]
	}
	reason, completed, stack := walkGoCompatSubtree(n, source, syms, flags, incrementalRanges, poller, nil, stack)
	if !completed {
		reason, _, stack = walkGoCompatSubtree(n, source, syms, flags, incrementalRanges, poller, make(map[*Node]struct{}), stack[:0])
	}
	if frames != nil {
		*frames = stack[:0]
	}
	return reason
}

func walkGoCompatSubtree(root *Node, source []byte, syms goCompatibilitySymbols, flags goCompatibilitySourceFlags, incrementalRanges []Range, poller *parseStopPoller, seen map[*Node]struct{}, stack []goCompatSubtreeFrame) (ParseStopReason, bool, []goCompatSubtreeFrame) {
	budget := goNormalizerPopBudget(len(source))
	pops := 0
	stack = append(stack[:0], goCompatSubtreeFrame{node: root})
	if seen != nil {
		seen[root] = struct{}{}
	}
	// Exit frames re-enqueue an already-seen node deliberately and are never
	// deduped; only child entry frames consult seen.
	pushChild := func(c *Node) {
		if c == nil {
			return
		}
		if seen != nil {
			if _, ok := seen[c]; ok {
				return
			}
			seen[c] = struct{}{}
		}
		stack = append(stack, goCompatSubtreeFrame{node: c})
	}
	for len(stack) > 0 {
		if reason := poller.poll(); resultMaterializationShouldStop(reason) {
			return reason, true, stack
		}
		top := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		n := top.node
		if n == nil {
			continue
		}
		if top.exit {
			if flags.trailingBoundary {
				normalizeGoStatementListTrailingExtras(n, source, syms)
			}
			continue
		}
		if seen == nil {
			pops++
			if pops > budget {
				return ParseStopNone, false, stack
			}
		}
		if !goNodeOverlapsAnyRange(n, incrementalRanges) {
			continue
		}
		childCount := resultChildCount(n)
		if childCount > 0 {
			normalizeGoSemicolonContainer(n, source, syms)
			if flags.siblingBoundary {
				normalizeGoAdjacentSiblingBoundaries(n, source, syms)
			}
		}
		stack = append(stack, goCompatSubtreeFrame{node: n, exit: true})
		if n.ownerArena == nil || n.childIndex > finalChildSidecarIndexBase {
			for i := len(n.children) - 1; i >= 0; i-- {
				pushChild(n.children[i])
			}
			continue
		}
		view := resultMutableChildrenForMutation(n)
		if view.hasFinalChildRefs() {
			for i := view.Len() - 1; i >= 0; i-- {
				entry, ok := view.Entry(i)
				if !ok || stackEntryNodeChildCount(entry) == 0 {
					continue
				}
				pushChild(resultChildAt(n, i))
			}
		} else {
			for i := childCount - 1; i >= 0; i-- {
				pushChild(resultChildAt(n, i))
			}
		}
	}
	return poller.pollNow(), true, stack
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
	if (syms.semicolon == 0 && syms.semicolonSentinel == 0) || !syms.isSemicolonContainer(n.symbol) {
		return
	}
	view := resultMutableChildrenForMutation(n)
	if view.hasFinalChildRefs() {
		normalizeGoSemicolonFinalRefs(view, source, syms)
		return
	}
	if !goHasDroppableSemicolonChild(n, source, syms) {
		return
	}
	// Capture the container's span before dropping. The DFA lexer's auto-semi
	// alternatives (`\n`/`\x00`) are zero-width matches, so dropping them here
	// never loses real source coverage. A token-source-driven lexer (e.g.
	// GoTokenSource) can instead emit a non-zero-width auto-semicolon token
	// that consumes the actual newline byte; populateParentNode (invoked by
	// replaceNodeChildrenUnfielded below) recomputes the container's span
	// from the surviving children only, which would silently shrink past
	// those consumed bytes and leave a gap uncovered by any node. Re-extend
	// back to the pre-drop span (a no-op when nothing was actually
	// consumed) to keep the dropped separator's bytes attributed to the
	// container, matching the C reference's hidden-separator span.
	origEnd := n.endByte
	children := resultChildSliceForMutation(n)
	kept := make([]*Node, 0, len(children))
	droppedMissing := false
	for _, child := range children {
		if goIsDroppableSemicolonNode(child, source, syms) {
			droppedMissing = droppedMissing || child.isMissing()
			continue
		}
		kept = append(kept, child)
	}
	if droppedMissing {
		goRetainProjectedMissingSemicolonShape(view)
	}
	replaceNodeChildrenUnfielded(n, cloneNodeSliceIfArena(n.ownerArena, kept))
	// Projection hides an automatic separator, not its genuine MISSING error.
	if droppedMissing {
		n.setHasError(true)
	}
	if n.endByte < origEnd {
		extendNodeEndTo(n, origEnd, source)
	}
}

func normalizeGoSemicolonFinalRefs(view resultMutableChildView, source []byte, syms goCompatibilitySymbols) {
	if !goHasDroppableSemicolonFinalRef(view, source, syms) {
		return
	}
	droppedMissing := false
	filtered := view.FilterFinalRefs(func(_ int, entry stackEntry) bool {
		drop := goIsDroppableSemicolonEntry(entry, source, syms)
		missing := drop && stackEntryNodeIsMissing(entry)
		if missing && !droppedMissing {
			goRetainProjectedMissingSemicolonShape(view)
		}
		droppedMissing = droppedMissing || missing
		return !drop
	})
	if filtered && droppedMissing {
		view.parent.setHasError(true)
	}
}

// goRetainProjectedMissingSemicolonShape preserves the selected pre-filter
// children only when a real MISSING separator is about to disappear and the
// single-stack parse elided its raw shape. Reconciliation can then prove the
// retained error using its existing raw-MISSING walk. Existing history is never
// replaced, and ordinary/stale/repaired visible leaves cannot create proof.
func goRetainProjectedMissingSemicolonShape(view resultMutableChildView) {
	n := view.parent
	if n == nil || n.ownerArena == nil || n.rawShape != 0 {
		return
	}
	count := 0
	for i := 0; i < view.Len(); i++ {
		if entry, ok := view.Entry(i); ok && stackEntryHasNode(entry) {
			count++
		}
	}
	if count == 0 || count > rawShapeMaxExactChildCount {
		return
	}
	arena := n.ownerArena
	ref, shape := arena.allocRawShape()
	if shape == nil {
		return
	}
	shape.symbol = n.symbol
	shape.productionID = n.productionID
	shape.childRange = arena.allocRawShapeChildren(count)
	children := arena.rawShapeChildren(shape)
	out := 0
	for i := 0; i < view.Len(); i++ {
		entry, ok := view.Entry(i)
		if !ok || !stackEntryHasNode(entry) {
			continue
		}
		children[out] = newRawShapeChild(entry)
		out++
	}
	// Hashes are optional cache entries; rawShapeHash computes this one lazily
	// if a later consumer asks for it. Avoid arming a hash cache for proof only.
	n.rawShape = ref
}

func goHasDroppableSemicolonFinalRef(view resultMutableChildView, source []byte, syms goCompatibilitySymbols) bool {
	for i := 0; i < view.Len(); i++ {
		entry, ok := view.Entry(i)
		if ok && goIsDroppableSemicolonEntry(entry, source, syms) {
			return true
		}
	}
	return false
}

func goHasDroppableSemicolonChild(n *Node, source []byte, syms goCompatibilitySymbols) bool {
	if n != nil && (n.ownerArena == nil || n.childIndex > finalChildSidecarIndexBase) {
		for _, child := range n.children {
			if goIsDroppableSemicolonNode(child, source, syms) {
				return true
			}
		}
		return false
	}
	for i := 0; i < resultChildCount(n); i++ {
		if goIsDroppableSemicolonNode(resultChildAt(n, i), source, syms) {
			return true
		}
	}
	return false
}

func goIsDroppableSemicolonNode(n *Node, source []byte, syms goCompatibilitySymbols) bool {
	return n != nil && goIsDroppableSemicolonSymbol(n.symbol, syms) && goShouldDropSemicolonSpan(n.startByte, n.endByte, source)
}

func goIsDroppableSemicolonEntry(entry stackEntry, source []byte, syms goCompatibilitySymbols) bool {
	return stackEntryHasNode(entry) &&
		goIsDroppableSemicolonSymbol(stackEntryNodeSymbol(entry), syms) &&
		goShouldDropSemicolonSpan(stackEntryNodeStartByte(entry), stackEntryNodeEndByte(entry), source)
}

func goIsDroppableSemicolonSymbol(sym Symbol, syms goCompatibilitySymbols) bool {
	return (syms.semicolon != 0 && sym == syms.semicolon) ||
		(syms.semicolonSentinel != 0 && sym == syms.semicolonSentinel)
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
