package gotreesitter

import "fmt"

type reduceChainSignature struct {
	state        StateID
	depth        int
	symbol       Symbol
	childCount   uint8
	productionID uint16
}

const maxRepeatedReduceChainSignature = 32

func buildReduceAliasSequences(lang *Language) [][]Symbol {
	if lang == nil || len(lang.AliasSequences) == 0 {
		return nil
	}
	out := make([][]Symbol, len(lang.AliasSequences))
	for i, seq := range lang.AliasSequences {
		for j := range seq {
			if seq[j] != 0 {
				out[i] = seq
				break
			}
		}
	}
	return out
}

func buildAliasTargetSymbols(lang *Language) []bool {
	if lang == nil || len(lang.AliasSequences) == 0 {
		return nil
	}
	out := make([]bool, len(lang.SymbolNames))
	any := false
	for _, seq := range lang.AliasSequences {
		for _, sym := range seq {
			if sym == 0 || int(sym) >= len(out) {
				continue
			}
			out[sym] = true
			any = true
		}
	}
	if !any {
		return nil
	}
	return out
}

func buildReduceFieldPresence(lang *Language) []bool {
	if lang == nil || len(lang.FieldMapSlices) == 0 {
		return nil
	}
	out := make([]bool, len(lang.FieldMapSlices))
	for i, fm := range lang.FieldMapSlices {
		out[i] = fm[1] != 0
	}
	return out
}

func buildSingleTokenWrapperSymbols(lang *Language) []bool {
	if lang == nil || len(lang.ParseActions) == 0 || len(lang.SymbolMetadata) == 0 {
		return nil
	}

	valid := make([]bool, len(lang.SymbolMetadata))
	for i, meta := range lang.SymbolMetadata {
		valid[i] = meta.Visible && meta.Named
	}

	seenBySymbol := make([]map[uint16]uint8, len(lang.SymbolMetadata))
	any := false
	for _, entry := range lang.ParseActions {
		for _, act := range entry.Actions {
			if act.Type != ParseActionReduce {
				continue
			}
			sym := int(act.Symbol)
			if sym < 0 || sym >= len(valid) || !valid[sym] {
				continue
			}
			seen := seenBySymbol[sym]
			if seen == nil {
				seen = map[uint16]uint8{}
				seenBySymbol[sym] = seen
			}
			if prev, ok := seen[act.ProductionID]; ok && prev != act.ChildCount {
				valid[sym] = false
				continue
			}
			seen[act.ProductionID] = act.ChildCount
		}
	}

	out := make([]bool, len(lang.SymbolMetadata))
	for sym, seen := range seenBySymbol {
		if !valid[sym] || len(seen) != 1 {
			continue
		}
		for _, cc := range seen {
			if cc == 1 {
				out[sym] = true
				any = true
			}
		}
	}
	if !any {
		return nil
	}
	return out
}

func (p *Parser) applyActionWithReduceChain(s *glrStack, act ParseAction, tok Token, anyReduced *bool, nodeCount *int, arena *nodeArena, entryScratch *glrEntryScratch, gssScratch *gssScratch, tmpEntries *[]stackEntry, deferParentLinks bool, trackChildErrors *bool) bool {
	p.applyAction(s, act, tok, anyReduced, nodeCount, arena, entryScratch, gssScratch, tmpEntries, deferParentLinks, trackChildErrors)
	if act.Type != ParseActionReduce || tok.NoLookahead || s == nil || s.dead || s.accepted || s.shifted {
		return false
	}
	return p.chainSingleReduceActions(s, tok, anyReduced, nodeCount, arena, entryScratch, gssScratch, tmpEntries, deferParentLinks, trackChildErrors)
}

func (p *Parser) pushOrExtendErrorNode(s *glrStack, state StateID, tok Token, nodeCount *int, arena *nodeArena, entryScratch *glrEntryScratch, gssScratch *gssScratch, trackChildErrors *bool) {
	if s != nil {
		top := s.top().node
		if top != nil &&
			top.symbol == errorSymbol &&
			!top.isMissing &&
			len(top.children) == 0 &&
			top.parseState == state &&
			tok.StartByte >= top.endByte {
			top.endByte = tok.EndByte
			top.endPoint = tok.EndPoint
			top.hasError = true
			nodeBumpEquivVersion(top)
			if s.byteOffset < top.endByte {
				s.byteOffset = top.endByte
			}
			if trackChildErrors != nil {
				*trackChildErrors = true
			}
			return
		}
	}

	errNode := newLeafNodeInArena(arena, errorSymbol, true,
		tok.StartByte, tok.EndByte, tok.StartPoint, tok.EndPoint)
	errNode.hasError = true
	if trackChildErrors != nil {
		*trackChildErrors = true
	}
	if perfCountersEnabled {
		perfRecordErrorNode()
	}
	errNode.parseState = state
	p.pushStackNode(s, state, errNode, entryScratch, gssScratch)
	if nodeCount != nil {
		*nodeCount = *nodeCount + 1
	}
}

func reduceChainSignatureFor(state StateID, depth int, act ParseAction) reduceChainSignature {
	return reduceChainSignature{
		state:        state,
		depth:        depth,
		symbol:       act.Symbol,
		childCount:   act.ChildCount,
		productionID: act.ProductionID,
	}
}

func noteRepeatedReduceChainSignature(prev reduceChainSignature, prevCount int, next reduceChainSignature) (reduceChainSignature, int, bool) {
	if prev == next {
		prevCount++
	} else {
		prev = next
		prevCount = 1
	}
	return prev, prevCount, prevCount > maxRepeatedReduceChainSignature
}

func (p *Parser) chainSingleReduceActions(s *glrStack, tok Token, anyReduced *bool, nodeCount *int, arena *nodeArena, entryScratch *glrEntryScratch, gssScratch *gssScratch, tmpEntries *[]stackEntry, deferParentLinks bool, trackChildErrors *bool) bool {
	if s == nil || s.dead || s.accepted || s.shifted {
		return false
	}
	const maxInlineReduceChain = 256
	parseActions := p.language.ParseActions
	chainLen := 0
	var lastSig reduceChainSignature
	repeatedSigCount := 0
	for chainLen < maxInlineReduceChain {
		currentState := s.top().state
		currentDepth := s.depth()
		actionIdx := p.lookupActionIndex(currentState, tok.Symbol)
		if actionIdx == 0 || int(actionIdx) >= len(parseActions) {
			return false
		}

		actions := parseActions[actionIdx].Actions
		if len(actions) != 1 {
			if perfCountersEnabled {
				perfRecordReduceChainBreakMulti()
			}
			return false
		}

		next := actions[0]
		switch next.Type {
		case ParseActionReduce:
			var repeated bool
			lastSig, repeatedSigCount, repeated = noteRepeatedReduceChainSignature(lastSig, repeatedSigCount, reduceChainSignatureFor(currentState, currentDepth, next))
			if repeated {
				if p != nil && p.glrTrace {
					fmt.Printf("      -> REDUCE-CHAIN CYCLE state=%d depth=%d sym=%d prod=%d count=%d\n",
						currentState, currentDepth, next.Symbol, next.ProductionID, repeatedSigCount)
				}
				return true
			}
			chainLen++
			if perfCountersEnabled {
				perfRecordReduceChainStep(chainLen)
			}
			p.applyAction(s, next, tok, anyReduced, nodeCount, arena, entryScratch, gssScratch, tmpEntries, deferParentLinks, trackChildErrors)
			if s.dead || s.accepted || s.shifted {
				return false
			}
		case ParseActionShift:
			if perfCountersEnabled {
				perfRecordReduceChainBreakShift()
			}
			return false
		case ParseActionAccept:
			if perfCountersEnabled {
				perfRecordReduceChainBreakAccept()
			}
			return false
		default:
			if perfCountersEnabled {
				perfRecordReduceChainBreakMulti()
			}
			return false
		}
	}
	return false
}

// applyAction applies a single parse action to a GLR stack.
func (p *Parser) applyAction(s *glrStack, act ParseAction, tok Token, anyReduced *bool, nodeCount *int, arena *nodeArena, entryScratch *glrEntryScratch, gssScratch *gssScratch, tmpEntries *[]stackEntry, deferParentLinks bool, trackChildErrors *bool) {
	if p != nil && p.glrTrace && s != nil {
		fmt.Printf("    APPLY type=%d cur_state=%d tok=%d act_state=%d act_sym=%d act_cnt=%d extra=%v rep=%v depth=%d\n",
			act.Type, s.top().state, tok.Symbol, act.State, act.Symbol, act.ChildCount, act.Extra, act.Repetition, s.depth())
	}
	switch act.Type {
	case ParseActionShift:
		named := p.isNamedSymbol(tok.Symbol)
		leaf := newLeafNodeInArena(arena, tok.Symbol, named,
			tok.StartByte, tok.EndByte, tok.StartPoint, tok.EndPoint)
		if tok.Missing || (p != nil && p.language != nil &&
			(p.language.Name == "c" || p.language.Name == "cpp" || p.language.Name == "objc") &&
			tok.Symbol != 0 && tok.StartByte == tok.EndByte && tok.Text == "") {
			leaf.isMissing = true
			leaf.hasError = true
			if trackChildErrors != nil {
				*trackChildErrors = true
			}
		}
		leaf.isExtra = act.Extra
		if leaf.isExtra && perfCountersEnabled {
			perfRecordExtraNode()
		}
		currentState := s.top().state
		targetState := extraShiftTargetState(currentState, act)
		leaf.preGotoState = currentState
		leaf.parseState = targetState
		p.recordCurrentExternalLeafCheckpoint(leaf, tok)
		p.pushStackNode(s, targetState, leaf, entryScratch, gssScratch)
		s.shifted = true
		*nodeCount++
		if p != nil && p.glrTrace {
			fmt.Printf("      -> SHIFT new_state=%d depth=%d\n", targetState, s.depth())
		}

	case ParseActionReduce:
		entries := s.entries
		borrowed := false
		if entries == nil {
			if !s.cacheEntries && s.gss.head != nil {
				tmp := []stackEntry(nil)
				if tmpEntries != nil {
					tmp = *tmpEntries
				}
				p.applyReduceActionFromGSS(s, act, tok, anyReduced, nodeCount, arena, entryScratch, gssScratch, tmpEntries, tmp, deferParentLinks, trackChildErrors != nil && *trackChildErrors)
				return
			}
			if s.cacheEntries {
				entries = s.ensureEntries(entryScratch)
			} else {
				tmp := []stackEntry(nil)
				if tmpEntries != nil {
					tmp = *tmpEntries
				}
				entries, borrowed = s.entriesForRead(tmp)
			}
		}
		p.applyReduceAction(s, act, tok, anyReduced, nodeCount, arena, entryScratch, gssScratch, entries, deferParentLinks, trackChildErrors != nil && *trackChildErrors)
		if borrowed && tmpEntries != nil {
			*tmpEntries = entries[:0]
		}
		if p != nil && p.glrTrace && s != nil && !s.dead {
			fmt.Printf("      -> REDUCE top_state=%d depth=%d\n", s.top().state, s.depth())
		}

	case ParseActionAccept:
		s.accepted = true
		if p != nil && p.glrTrace {
			fmt.Printf("      -> ACCEPT\n")
		}

	case ParseActionRecover:
		if tok.Symbol == 0 && tok.StartByte == tok.EndByte {
			s.accepted = true
			return
		}
		recoverState := s.top().state
		if act.State != 0 {
			recoverState = act.State
		}
		p.pushOrExtendErrorNode(s, recoverState, tok, nodeCount, arena, entryScratch, gssScratch, trackChildErrors)
		if p != nil && p.glrTrace && s != nil && !s.dead {
			fmt.Printf("      -> RECOVER state=%d depth=%d\n", s.top().state, s.depth())
		}
	}
}

func extraShiftTargetState(current StateID, act ParseAction) StateID {
	if !act.Extra || act.State != 0 {
		return act.State
	}
	return current
}

func (p *Parser) pushStackNode(s *glrStack, state StateID, node *Node, entryScratch *glrEntryScratch, gssScratch *gssScratch) {
	s.push(state, node, entryScratch, gssScratch)
	if !s.recoverabilityKnown {
		return
	}
	if !s.mayRecover && p.stateCanRecover(state) {
		s.mayRecover = true
	}
}

func reduceWindowFromGSS(s *glrStack, childCount int, buf []stackEntry) ([]stackEntry, StateID, bool) {
	if s == nil || s.gss.head == nil || s.depth() == 0 {
		return nil, 0, false
	}
	if childCount == 0 {
		return buf[:0], s.top().state, true
	}

	rev := buf[:0]
	nonExtraFound := 0
	n := s.gss.head
	for n != nil {
		rev = append(rev, n.entry)
		if n.entry.node != nil && !n.entry.node.isExtra {
			nonExtraFound++
			if nonExtraFound == childCount {
				break
			}
		}
		n = n.prev
	}
	if nonExtraFound < childCount || n == nil || n.prev == nil {
		return rev[:0], 0, false
	}
	topState := n.prev.entry.state

	for i, j := 0, len(rev)-1; i < j; i, j = i+1, j-1 {
		rev[i], rev[j] = rev[j], rev[i]
	}
	return rev, topState, true
}

func (p *Parser) tryFastVisibleReduceActionFromGSS(s *glrStack, act ParseAction, tok Token, anyReduced *bool, nodeCount *int, arena *nodeArena, entryScratch *glrEntryScratch, gssScratch *gssScratch, tmpEntries *[]stackEntry, deferParentLinks bool, trackChildErrors bool) bool {
	if p == nil || s == nil || s.gss.head == nil || p.language == nil {
		return false
	}
	childCount := int(act.ChildCount)
	if childCount <= 1 || childCount > 8 {
		return false
	}
	if len(p.reduceAliasSequence(act.ProductionID)) != 0 || p.reduceProductionHasFields(act.ProductionID) {
		return false
	}
	if p.forceRawSpanAll || (int(act.Symbol) < len(p.forceRawSpanTable) && p.forceRawSpanTable[act.Symbol]) {
		return false
	}
	parentVisible := true
	if idx := int(act.Symbol); idx < len(p.language.SymbolMetadata) {
		parentVisible = p.language.SymbolMetadata[act.Symbol].Visible
	}
	if !parentVisible {
		return false
	}

	var childBuf [8]*Node
	symbolMeta := p.language.SymbolMetadata
	n := s.gss.head
	for i := childCount - 1; i >= 0; i-- {
		if n == nil {
			return false
		}
		child := n.entry.node
		if child == nil || child.isExtra {
			return false
		}
		visible := true
		if idx := int(child.symbol); idx < len(symbolMeta) {
			visible = symbolMeta[child.symbol].Visible
		}
		if !visible {
			return false
		}
		childBuf[i] = child
		n = n.prev
	}
	if n == nil {
		return false
	}
	topState := n.entry.state
	targetDepth := s.depth() - childCount
	if targetDepth < 0 {
		return false
	}

	children := arena.allocNodeSlice(childCount)
	copy(children, childBuf[:childCount])
	named := p.isNamedSymbol(act.Symbol)
	var parent *Node
	if deferParentLinks {
		parent = newParentNodeInArenaNoLinksWithFieldSources(arena, act.Symbol, named, children, nil, nil, act.ProductionID, trackChildErrors)
	} else {
		parent = newParentNodeInArenaWithFieldSources(arena, act.Symbol, named, children, nil, nil, act.ProductionID)
	}
	*nodeCount++

	gotoState := p.lookupGoto(topState, act.Symbol)
	targetState := topState
	if gotoState != 0 {
		targetState = gotoState
	}
	if tok.NoLookahead && targetState == topState {
		parent.isExtra = true
	}
	parent.preGotoState = topState
	parent.parseState = targetState
	if !s.truncate(targetDepth) {
		s.dead = true
		if tmpEntries != nil {
			*tmpEntries = (*tmpEntries)[:0]
		}
		return true
	}
	p.pushStackNode(s, targetState, parent, entryScratch, gssScratch)
	s.score += int(act.DynamicPrecedence)
	*anyReduced = true
	if tmpEntries != nil {
		*tmpEntries = (*tmpEntries)[:0]
	}
	return true
}

func (p *Parser) applyReduceActionFromGSS(s *glrStack, act ParseAction, tok Token, anyReduced *bool, nodeCount *int, arena *nodeArena, entryScratch *glrEntryScratch, gssScratch *gssScratch, tmpEntries *[]stackEntry, tmp []stackEntry, deferParentLinks bool, trackChildErrors bool) {
	if p.tryFastVisibleReduceActionFromGSS(s, act, tok, anyReduced, nodeCount, arena, entryScratch, gssScratch, tmpEntries, deferParentLinks, trackChildErrors) {
		return
	}
	childCount := int(act.ChildCount)
	windowEntries, topState, ok := reduceWindowFromGSS(s, childCount, tmp)
	if !ok {
		s.dead = true
		if tmpEntries != nil {
			*tmpEntries = windowEntries[:0]
		}
		return
	}

	actualEnd := len(windowEntries)
	reducedEnd := actualEnd
	for i := actualEnd - 1; i >= 0; i-- {
		n := windowEntries[i].node
		if n == nil || !n.isExtra {
			break
		}
		reducedEnd--
	}

	children, fieldIDs, fieldSources := p.buildReduceChildren(windowEntries, 0, reducedEnd, childCount, act.Symbol, act.ProductionID, arena)

	targetDepth := s.depth() - actualEnd
	if targetDepth < 0 || !s.truncate(targetDepth) {
		s.dead = true
		if tmpEntries != nil {
			*tmpEntries = windowEntries[:0]
		}
		return
	}

	if child := p.collapsibleUnarySelfReduction(act, tok, arena, windowEntries, 0, reducedEnd, children, fieldIDs); child != nil {
		gotoState := p.lookupGoto(topState, act.Symbol)
		targetState := topState
		if gotoState != 0 {
			targetState = gotoState
		}
		if tok.NoLookahead && targetState == topState {
			child.isExtra = true
		}
		child.productionID = act.ProductionID
		child.preGotoState = topState
		child.parseState = targetState
		nodeBumpEquivVersion(child)
		p.pushStackNode(s, targetState, child, entryScratch, gssScratch)
		for i := reducedEnd; i < actualEnd; i++ {
			extra := windowEntries[i].node
			if extra == nil {
				continue
			}
			extra.parseState = targetState
			nodeBumpEquivVersion(extra)
			p.pushStackNode(s, targetState, extra, entryScratch, gssScratch)
		}
		s.score += int(act.DynamicPrecedence)
		*anyReduced = true
		if tmpEntries != nil {
			*tmpEntries = windowEntries[:0]
		}
		return
	}

	named := p.isNamedSymbol(act.Symbol)
	var parent *Node
	if deferParentLinks {
		parent = newParentNodeInArenaNoLinksWithFieldSources(arena, act.Symbol, named, children, fieldIDs, fieldSources, act.ProductionID, trackChildErrors)
	} else {
		parent = newParentNodeInArenaWithFieldSources(arena, act.Symbol, named, children, fieldIDs, fieldSources, act.ProductionID)
	}
	shouldUseRawSpan := shouldUseRawSpanForReduction(act.Symbol, children, p.language.SymbolMetadata, p.forceRawSpanAll, p.forceRawSpanTable)
	if shouldUseRawSpan && reducedEnd > 0 {
		span := computeReduceRawSpan(windowEntries, 0, reducedEnd)
		if int(act.Symbol) < len(p.forceRawSpanTable) && p.forceRawSpanTable[act.Symbol] && actualEnd > reducedEnd {
			extendRawSpanToTrailingEntries(&span, windowEntries, reducedEnd, actualEnd)
		}
		parent.startByte = span.startByte
		parent.endByte = span.endByte
		parent.startPoint = span.startPoint
		parent.endPoint = span.endPoint
	}
	// Extend parent span to cover invisible children dropped by buildReduceChildren.
	extendParentSpanToWindow(parent, windowEntries, 0, reducedEnd, p.language.SymbolMetadata, p.language.SymbolNames)
	*nodeCount++

	gotoState := p.lookupGoto(topState, act.Symbol)
	targetState := topState
	if gotoState != 0 {
		targetState = gotoState
	}
	if tok.NoLookahead && targetState == topState {
		parent.isExtra = true
	}
	parent.preGotoState = topState
	parent.parseState = targetState
	p.pushStackNode(s, targetState, parent, entryScratch, gssScratch)
	for i := reducedEnd; i < actualEnd; i++ {
		extra := windowEntries[i].node
		if extra == nil {
			continue
		}
		extra.parseState = targetState
		nodeBumpEquivVersion(extra)
		p.pushStackNode(s, targetState, extra, entryScratch, gssScratch)
	}

	s.score += int(act.DynamicPrecedence)
	*anyReduced = true
	if tmpEntries != nil {
		*tmpEntries = windowEntries[:0]
	}
}

type reduceRange struct {
	start      int
	reducedEnd int
	actualEnd  int
	topState   StateID
}

type reduceRawSpan struct {
	startByte  uint32
	endByte    uint32
	startPoint Point
	endPoint   Point
}

func computeReduceRange(entries []stackEntry, childCount int) (reduceRange, bool) {
	start := len(entries)
	nonExtraFound := 0
	for nonExtraFound < childCount && start > 1 {
		start--
		if entries[start].node != nil && !entries[start].node.isExtra {
			nonExtraFound++
		}
	}
	if nonExtraFound < childCount {
		return reduceRange{}, false
	}

	actualEnd := len(entries)
	reducedEnd := actualEnd
	for i := actualEnd - 1; i >= start; i-- {
		n := entries[i].node
		if n == nil || !n.isExtra {
			break
		}
		reducedEnd--
	}
	return reduceRange{
		start:      start,
		reducedEnd: reducedEnd,
		actualEnd:  actualEnd,
		topState:   entries[start-1].state,
	}, true
}

func computeReduceRawSpan(entries []stackEntry, start, end int) reduceRawSpan {
	span := reduceRawSpan{}
	if end <= start {
		return span
	}

	foundStart := false
	for i := start; i < end; i++ {
		n := entries[i].node
		if n != nil && !n.isExtra {
			span.startByte = n.startByte
			span.startPoint = n.startPoint
			foundStart = true
			break
		}
	}

	foundEnd := false
	for i := end - 1; i >= start; i-- {
		n := entries[i].node
		if n != nil && !n.isExtra {
			span.endByte = n.endByte
			span.endPoint = n.endPoint
			foundEnd = true
			break
		}
	}

	firstRaw := entries[start].node
	lastRaw := entries[end-1].node
	if !foundStart && firstRaw != nil {
		span.startByte = firstRaw.startByte
		span.startPoint = firstRaw.startPoint
	}
	if !foundEnd && lastRaw != nil {
		span.endByte = lastRaw.endByte
		span.endPoint = lastRaw.endPoint
	}
	return span
}

func extendRawSpanToTrailingEntries(span *reduceRawSpan, entries []stackEntry, start, end int) {
	if span == nil || end <= start {
		return
	}
	for i := end - 1; i >= start; i-- {
		n := entries[i].node
		if n == nil {
			continue
		}
		if n.endByte > span.endByte {
			span.endByte = n.endByte
			span.endPoint = n.endPoint
		}
		return
	}
}

func shouldUseRawSpanForReduction(sym Symbol, children []*Node, symbolMeta []SymbolMetadata, forceRawSpanAll bool, forceRawSpanTable []bool) bool {
	if len(children) == 0 {
		return true
	}
	if forceRawSpanAll {
		return true
	}
	if int(sym) < len(forceRawSpanTable) && forceRawSpanTable[sym] {
		return true
	}
	if int(sym) < len(symbolMeta) && !symbolMeta[sym].Visible {
		return true
	}
	return false
}

// extendParentSpanToWindow widens the parent node's [startByte, endByte] to
// recover span from entries that buildReduceChildren drops. Two categories:
//
//  1. Leading extras: extend startByte backward (extras before first structural child).
//  2. Invisible non-extra leaf children: these are structural children whose symbol
//     is not visible AND that have no children to inline. buildReduceChildren skips
//     them entirely (the "if len(kids) == 0 { continue }" path), losing their span.
//     In C tree-sitter, ts_subtree_set_children includes ALL children in the parent
//     span, so we must recover these dropped spans to match.
//
// Trailing extras (separated into [reducedEnd, actualEnd)) are NOT scanned because
// they become siblings of the parent, not children.
func extendParentSpanToWindow(parent *Node, entries []stackEntry, start, reducedEnd int, symbolMeta []SymbolMetadata, symbolNames []string) {
	// Leading extras: extend startByte backward until the first structural child.
	for i := start; i < reducedEnd; i++ {
		n := entries[i].node
		if n == nil {
			continue
		}
		if !n.isExtra {
			break
		}
		if n.startByte < parent.startByte {
			parent.startByte = n.startByte
			parent.startPoint = n.startPoint
		}
	}
	// Invisible non-extra children: extend parent span for entries that
	// buildReduceChildren drops or inlines away.
	//
	// Scan from the end toward the beginning so backward extension can chain
	// across adjacent hidden leaves. A forward-only pass misses prefixes like
	// markdown plain-text runs because the earlier hidden tokens become
	// contiguous only after a later sibling has already pulled startByte back.
	// The same reverse scan is still safe for endByte growth because the
	// contiguity checks below prevent phantom gaps from inflating the span.
	for i := reducedEnd - 1; i >= start; i-- {
		n := entries[i].node
		if n == nil || n.isExtra {
			continue
		}
		visible := true
		if idx := int(n.symbol); idx < len(symbolMeta) {
			visible = symbolMeta[n.symbol].Visible
		}
		if visible {
			continue // visible children are already represented in parent's children
		}
		if isNonSpanExtendingInvisibleSymbol(n.symbol, symbolNames) {
			continue
		}
		// Invisible entries (with or without children) may have span that
		// extends beyond their inlined children due to nested invisible leaf
		// extensions. Apply contiguity check below.
		if n.endByte >= parent.startByte && n.startByte < parent.startByte {
			parent.startByte = n.startByte
			parent.startPoint = n.startPoint
		}
		if n.startByte <= parent.endByte && n.endByte > parent.endByte {
			parent.endByte = n.endByte
			parent.endPoint = n.endPoint
		}
		if n.startByte == n.endByte && n.startByte > parent.endByte &&
			isSpanExtendingInvisibleSymbol(n.symbol, symbolNames) {
			parent.endByte = n.endByte
			parent.endPoint = n.endPoint
		}
	}
	// Follow with a forward pass for endByte growth so contiguous hidden tails
	// can chain (for example interpolated multiline string middle -> string end).
	for i := start; i < reducedEnd; i++ {
		n := entries[i].node
		if n == nil || n.isExtra {
			continue
		}
		visible := true
		if idx := int(n.symbol); idx < len(symbolMeta) {
			visible = symbolMeta[n.symbol].Visible
		}
		if visible {
			continue
		}
		if isNonSpanExtendingInvisibleSymbol(n.symbol, symbolNames) {
			continue
		}
		if n.startByte <= parent.endByte && n.endByte > parent.endByte {
			parent.endByte = n.endByte
			parent.endPoint = n.endPoint
		}
		if n.startByte == n.endByte && n.startByte > parent.endByte &&
			isSpanExtendingInvisibleSymbol(n.symbol, symbolNames) {
			parent.endByte = n.endByte
			parent.endPoint = n.endPoint
		}
	}
}

func isSpanExtendingInvisibleSymbol(sym Symbol, symbolNames []string) bool {
	idx := int(sym)
	if idx < 0 || idx >= len(symbolNames) {
		return false
	}
	switch symbolNames[idx] {
	case "_implicit_end_tag":
		return true
	case "_outdent":
		return true
	case "_single_line_string_end":
		return true
	case "_multiline_string_end":
		return true
	case "_interpolated_string_middle":
		return true
	case "_interpolated_multiline_string_middle":
		return true
	default:
		return false
	}
}

func isNonSpanExtendingInvisibleSymbol(sym Symbol, symbolNames []string) bool {
	idx := int(sym)
	if idx < 0 || idx >= len(symbolNames) {
		return false
	}
	switch symbolNames[idx] {
	case "_line_ending_or_eof":
		return true
	default:
		return false
	}
}

const (
	fieldSourceNone uint8 = iota
	fieldSourceDirect
	fieldSourceInherited
)

func fieldSourceAt(fieldSources []uint8, i int) uint8 {
	if i < 0 || i >= len(fieldSources) {
		return fieldSourceNone
	}
	return fieldSources[i]
}

func countEligibleNamedFieldTargets(children []*Node, fieldIDs []FieldID, start, end int) int {
	count := 0
	for i := start; i < end; i++ {
		if children[i] == nil || children[i].isExtra || children[i].isMissing || !children[i].isNamed || fieldIDs[i] != 0 {
			continue
		}
		count++
	}
	return count
}

func countEligibleFieldTargets(children []*Node, fieldIDs []FieldID, start, end int) int {
	count := 0
	for i := start; i < end; i++ {
		if children[i] == nil || children[i].isExtra || children[i].isMissing || fieldIDs[i] != 0 {
			continue
		}
		count++
	}
	return count
}

func fieldIDAppearsLater(fieldIDs []FieldID, start int, fid FieldID) bool {
	if fid == 0 || start < 0 {
		return false
	}
	for i := start; i < len(fieldIDs); i++ {
		if fieldIDs[i] == fid {
			return true
		}
	}
	return false
}

func flattenedSpanHasFieldID(fieldIDs []FieldID, start, end int, fid FieldID) bool {
	if fid == 0 || fieldIDs == nil || start >= end {
		return false
	}
	for i := start; i < end; i++ {
		if fieldIDs[i] == fid {
			return true
		}
	}
	return false
}

func flattenedSpanHasAnyDirectField(children []*Node, fieldIDs []FieldID, fieldSources []uint8, start, end int) bool {
	for i := start; i < end; i++ {
		if i < len(fieldIDs) && fieldIDs[i] != 0 && fieldSourceAt(fieldSources, i) == fieldSourceDirect {
			return true
		}
		if i < len(children) && nodeHasAnyDirectField(children[i]) {
			return true
		}
	}
	return false
}

func flattenedSpanSingleDescendantFieldTarget(children []*Node, start, end int, fid FieldID) (int, bool) {
	if fid == 0 {
		return 0, false
	}
	target := -1
	for i := start; i < end; i++ {
		child := children[i]
		if child == nil || child.isExtra || !nodeHasDirectFieldID(child, fid) {
			continue
		}
		if target >= 0 {
			return 0, false
		}
		target = i
	}
	return target, target >= 0
}

type reduceBuildScratch struct {
	nodes         []*Node
	fieldIDs      []FieldID
	fieldSources  []uint8
	trackFields   bool
	repeatStamp   []uint32
	repeatCount   []uint16
	repeatSource  []uint8
	repeatTouched []FieldID
	repeatEpoch   uint32
}

func (s *reduceBuildScratch) reset() {
	if s == nil {
		return
	}
	if len(s.nodes) > 0 {
		clear(s.nodes)
		s.nodes = s.nodes[:0]
	}
	s.fieldIDs = s.fieldIDs[:0]
	s.fieldSources = s.fieldSources[:0]
	s.trackFields = false
	s.repeatTouched = s.repeatTouched[:0]
}

func (s *reduceBuildScratch) appendNode(n *Node) {
	if s == nil {
		return
	}
	s.nodes = append(s.nodes, n)
	if s.trackFields {
		s.fieldIDs = append(s.fieldIDs, 0)
		s.fieldSources = append(s.fieldSources, fieldSourceNone)
	}
}

func (s *reduceBuildScratch) ensureFieldStorage() {
	if s == nil || s.trackFields {
		return
	}
	n := len(s.nodes)
	if cap(s.fieldIDs) < n {
		s.fieldIDs = make([]FieldID, n)
		s.fieldSources = make([]uint8, n)
	} else {
		s.fieldIDs = s.fieldIDs[:n]
		clear(s.fieldIDs)
		s.fieldSources = s.fieldSources[:n]
		clear(s.fieldSources)
	}
	s.trackFields = true
}

func (s *reduceBuildScratch) nextRepeatEpoch() uint32 {
	if s == nil {
		return 0
	}
	s.repeatEpoch++
	if s.repeatEpoch == 0 {
		clear(s.repeatStamp)
		s.repeatEpoch = 1
	}
	return s.repeatEpoch
}

func (s *reduceBuildScratch) ensureRepeatFieldCapacity(fid FieldID) {
	if s == nil {
		return
	}
	need := int(fid) + 1
	if need <= len(s.repeatStamp) {
		return
	}
	grow := cap(s.repeatStamp)
	if grow < need {
		grow = need
	}
	if grow < 32 {
		grow = 32
	}
	for grow < need {
		grow *= 2
	}

	stamp := make([]uint32, need, grow)
	copy(stamp, s.repeatStamp)
	s.repeatStamp = stamp

	count := make([]uint16, need, grow)
	copy(count, s.repeatCount)
	s.repeatCount = count

	source := make([]uint8, need, grow)
	copy(source, s.repeatSource)
	s.repeatSource = source
}

func (s *reduceBuildScratch) recordRepeatedField(epoch uint32, fid FieldID, source uint8) {
	if s == nil || fid == 0 || epoch == 0 {
		return
	}
	s.ensureRepeatFieldCapacity(fid)
	idx := int(fid)
	if s.repeatStamp[idx] != epoch {
		s.repeatStamp[idx] = epoch
		s.repeatCount[idx] = 1
		s.repeatSource[idx] = source
		s.repeatTouched = append(s.repeatTouched, fid)
		return
	}
	s.repeatCount[idx]++
	s.repeatSource[idx] = source
}

func appendFlattenedHiddenChildrenToScratch(scratch *reduceBuildScratch, n *Node, symbolMeta []SymbolMetadata) {
	if scratch == nil || n == nil {
		return
	}
	visible := true
	if idx := int(n.symbol); idx < len(symbolMeta) {
		visible = symbolMeta[n.symbol].Visible
	}
	if visible {
		scratch.appendNode(n)
		return
	}
	for _, child := range n.children {
		appendFlattenedHiddenChildrenToScratch(scratch, child, symbolMeta)
	}
}

func appendFlattenedHiddenChildrenWithFieldScratch(scratch *reduceBuildScratch, n *Node, symbolMeta []SymbolMetadata) {
	if scratch == nil || n == nil {
		return
	}
	visible := true
	if idx := int(n.symbol); idx < len(symbolMeta) {
		visible = symbolMeta[n.symbol].Visible
	}
	if visible {
		scratch.appendNode(n)
		return
	}

	nodeStart := len(scratch.nodes)
	repeatEpoch := scratch.nextRepeatEpoch()
	touchedStart := len(scratch.repeatTouched)
	for i, child := range n.children {
		spanStart := len(scratch.nodes)
		appendFlattenedHiddenChildrenWithFieldScratch(scratch, child, symbolMeta)
		spanEnd := len(scratch.nodes)
		if i >= len(n.fieldIDs) || n.fieldIDs[i] == 0 || spanStart >= spanEnd {
			continue
		}
		scratch.ensureFieldStorage()
		source := fieldSourceAt(n.fieldSources, i)
		if source == fieldSourceNone {
			source = fieldSourceDirect
		}
		applyFieldToFlattenedSpan(scratch.nodes, scratch.fieldIDs, scratch.fieldSources, spanStart, spanEnd, n.fieldIDs[i], source, false)
		if source == fieldSourceDirect {
			scratch.recordRepeatedField(repeatEpoch, n.fieldIDs[i], source)
		}
	}
	if scratch.trackFields {
		for _, fid := range scratch.repeatTouched[touchedStart:] {
			idx := int(fid)
			if idx < 0 || idx >= len(scratch.repeatCount) || scratch.repeatCount[idx] < 2 {
				continue
			}
			applyFieldToFlattenedSpan(scratch.nodes, scratch.fieldIDs, scratch.fieldSources, nodeStart, len(scratch.nodes), fid, scratch.repeatSource[idx], false)
		}
		scratch.repeatTouched = scratch.repeatTouched[:touchedStart]
		normalizeMixedSourceFieldSpan(scratch.fieldIDs, scratch.fieldSources, nodeStart, len(scratch.nodes))
	}
}

func materializeReduceChildrenFromScratch(scratch *reduceBuildScratch, arena *nodeArena) ([]*Node, []FieldID, []uint8) {
	if scratch == nil || len(scratch.nodes) == 0 {
		return nil, nil, nil
	}
	children := arena.allocNodeSlice(len(scratch.nodes))
	copy(children, scratch.nodes)
	if !scratch.trackFields {
		return children, nil, nil
	}
	fieldIDs := arena.allocFieldIDSlice(len(scratch.fieldIDs))
	copy(fieldIDs, scratch.fieldIDs)
	fieldSources := arena.allocFieldSourceSlice(len(scratch.fieldSources))
	copy(fieldSources, scratch.fieldSources)
	return children, fieldIDs, fieldSources
}

func (p *Parser) buildReduceChildrenAllVisible(entries []stackEntry, start, end, childCount int, aliasSeq []Symbol, rawFieldIDs []FieldID, rawInherited []bool, symbolMeta []SymbolMetadata, arena *nodeArena) ([]*Node, []FieldID, []uint8, bool) {
	visibleCount := 0
	structuralChildIndex := 0
	for i := start; i < end; i++ {
		n := entries[i].node
		if n == nil {
			continue
		}
		effectiveSymbol := n.symbol
		if !n.isExtra {
			if structuralChildIndex < len(aliasSeq) {
				if alias := aliasSeq[structuralChildIndex]; alias != 0 {
					effectiveSymbol = alias
				}
			}
			structuralChildIndex++
		}
		visible := true
		if idx := int(effectiveSymbol); idx < len(symbolMeta) {
			visible = symbolMeta[effectiveSymbol].Visible
		}
		if !visible {
			return nil, nil, nil, false
		}
		visibleCount++
	}
	if visibleCount == 0 {
		return nil, nil, nil, true
	}

	children := arena.allocNodeSlice(visibleCount)
	var fieldIDs []FieldID
	var fieldSources []uint8
	if rawFieldIDs != nil {
		fieldIDs = arena.allocFieldIDSlice(visibleCount)
		fieldSources = arena.allocFieldSourceSlice(visibleCount)
	}

	out := 0
	structuralChildIndex = 0
	for i := start; i < end; i++ {
		n := entries[i].node
		if n == nil {
			continue
		}
		var fid FieldID
		inherited := false
		if !n.isExtra {
			if structuralChildIndex < len(rawFieldIDs) {
				fid = rawFieldIDs[structuralChildIndex]
				if structuralChildIndex < len(rawInherited) {
					inherited = rawInherited[structuralChildIndex]
				}
			}
			if structuralChildIndex < len(aliasSeq) {
				if alias := aliasSeq[structuralChildIndex]; alias != 0 {
					n = aliasedNodeInArena(arena, p.language, n, alias)
				}
			}
			structuralChildIndex++
		}
		children[out] = n
		if fieldIDs != nil && !inherited && !p.shouldSuppressVisibleDirectField(n, fid) {
			fieldIDs[out] = fid
			if fid != 0 {
				fieldSources[out] = fieldSourceDirect
			}
		}
		out++
	}
	if fieldIDs != nil {
		p.suppressReducedChildFields(children, fieldIDs, fieldSources)
	}
	return children, fieldIDs, fieldSources, true
}

func (p *Parser) buildReduceChildren(entries []stackEntry, start, end, childCount int, parentSymbol Symbol, productionID uint16, arena *nodeArena) ([]*Node, []FieldID, []uint8) {
	lang := p.language
	symbolMeta := lang.SymbolMetadata

	aliasSeq := p.reduceAliasSequence(productionID)
	parentVisible := true
	if idx := int(parentSymbol); idx < len(symbolMeta) {
		parentVisible = symbolMeta[parentSymbol].Visible
	}
	preserveHiddenFields := false
	if parentVisible {
		for i := start; i < end; i++ {
			n := entries[i].node
			if n == nil {
				continue
			}
			visible := true
			if idx := int(n.symbol); idx < len(symbolMeta) {
				visible = symbolMeta[n.symbol].Visible
			}
			if !visible && hiddenTreeHasFieldIDs(n) {
				preserveHiddenFields = true
				break
			}
		}
	}
	if len(aliasSeq) == 0 && !p.reduceProductionHasFields(productionID) && !preserveHiddenFields {
		return p.buildReduceChildrenNoAliasNoFieldsStreaming(entries, start, end, parentSymbol, symbolMeta, arena)
	}

	rawFieldIDs, rawInherited := p.buildFieldIDs(childCount, productionID, arena)
	if children, fieldIDs, fieldSources, ok := p.buildReduceChildrenAllVisible(entries, start, end, childCount, aliasSeq, rawFieldIDs, rawInherited, symbolMeta, arena); ok {
		return children, fieldIDs, fieldSources
	}

	var scratch *reduceBuildScratch
	if p != nil && p.reduceScratch != nil {
		scratch = p.reduceScratch
	} else {
		scratch = &reduceBuildScratch{}
	}
	scratch.reset()
	if rawFieldIDs != nil {
		scratch.ensureFieldStorage()
	}

	structuralChildIndex := 0
	for i := start; i < end; i++ {
		n := entries[i].node
		if n == nil {
			continue
		}
		var fid FieldID
		inherited := false
		if !n.isExtra {
			if structuralChildIndex < len(rawFieldIDs) {
				fid = rawFieldIDs[structuralChildIndex]
				if structuralChildIndex < len(rawInherited) {
					inherited = rawInherited[structuralChildIndex]
				}
			}
			if structuralChildIndex < len(aliasSeq) {
				if alias := aliasSeq[structuralChildIndex]; alias != 0 {
					n = aliasedNodeInArena(arena, lang, n, alias)
				}
			}
			structuralChildIndex++
		}
		visible := true
		if idx := int(n.symbol); idx < len(symbolMeta) {
			visible = symbolMeta[n.symbol].Visible
		}
		if visible {
			out := len(scratch.nodes)
			scratch.appendNode(n)
			if scratch.trackFields {
				if !inherited && !p.shouldSuppressVisibleDirectField(n, fid) {
					scratch.fieldIDs[out] = fid
					if fid != 0 {
						scratch.fieldSources[out] = fieldSourceDirect
					}
				}
			}
			continue
		}

		kids := n.children
		if len(kids) == 0 {
			continue
		}
		spanStart := len(scratch.nodes)
		if hiddenTreeHasFieldIDs(n) {
			appendFlattenedHiddenChildrenWithFieldScratch(scratch, n, symbolMeta)
		} else {
			appendFlattenedHiddenChildrenToScratch(scratch, n, symbolMeta)
		}
		if scratch.trackFields {
			fieldEnd := len(scratch.fieldIDs)
			// Apply the parent's inherited field assignment to the
			// flattened child span, but only if inlining did not
			// already surface that same field on one of the copied
			// children.
			if fid != 0 {
				source := fieldSourceDirect
				if inherited {
					source = fieldSourceInherited
				}
				if inherited && !flattenedSpanHasFieldID(scratch.fieldIDs, spanStart, fieldEnd, fid) {
					if target, ok := flattenedSpanSingleDescendantFieldTarget(scratch.nodes, spanStart, fieldEnd, fid); ok {
						scratch.fieldIDs[target] = fid
						scratch.fieldSources[target] = fieldSourceInherited
						normalizeMixedSourceFieldSpan(scratch.fieldIDs, scratch.fieldSources, spanStart, fieldEnd)
						continue
					}
				}
				if inherited && fieldEnd-spanStart == 1 && !flattenedSpanHasFieldID(scratch.fieldIDs, spanStart, fieldEnd, fid) {
					child := scratch.nodes[spanStart]
					if child == nil {
						continue
					}
					if nodeHasDirectFieldID(child, fid) || len(child.children) == 0 {
						continue
					}
				}
				if inherited && n.isNamed && !flattenedSpanHasFieldID(scratch.fieldIDs, spanStart, fieldEnd, fid) && countEligibleNamedFieldTargets(scratch.nodes, scratch.fieldIDs, spanStart, fieldEnd) > 1 {
					continue
				}
				if inherited && !flattenedSpanHasFieldID(scratch.fieldIDs, spanStart, fieldEnd, fid) && flattenedSpanHasAnyDirectField(scratch.nodes, scratch.fieldIDs, scratch.fieldSources, spanStart, fieldEnd) {
					if fieldEnd-spanStart != 1 {
						continue
					}
					child := scratch.nodes[spanStart]
					if child == nil || !nodeHasDirectFieldID(child, fid) {
						continue
					}
				}
				if !inherited || !fieldIDAppearsLater(rawFieldIDs, structuralChildIndex, fid) {
					applyFieldToFlattenedSpan(scratch.nodes, scratch.fieldIDs, scratch.fieldSources, spanStart, fieldEnd, fid, source, true)
					normalizeMixedSourceFieldSpan(scratch.fieldIDs, scratch.fieldSources, spanStart, fieldEnd)
				}
			}
		} else if fid != 0 {
			scratch.ensureFieldStorage()
			fieldEnd := len(scratch.fieldIDs)
			source := fieldSourceDirect
			if inherited {
				source = fieldSourceInherited
			}
			if inherited && !flattenedSpanHasFieldID(scratch.fieldIDs, spanStart, fieldEnd, fid) {
				if target, ok := flattenedSpanSingleDescendantFieldTarget(scratch.nodes, spanStart, fieldEnd, fid); ok {
					scratch.fieldIDs[target] = fid
					scratch.fieldSources[target] = fieldSourceInherited
					normalizeMixedSourceFieldSpan(scratch.fieldIDs, scratch.fieldSources, spanStart, fieldEnd)
					continue
				}
			}
			if inherited && fieldEnd-spanStart == 1 && !flattenedSpanHasFieldID(scratch.fieldIDs, spanStart, fieldEnd, fid) {
				child := scratch.nodes[spanStart]
				if child == nil {
					continue
				}
				if nodeHasDirectFieldID(child, fid) || len(child.children) == 0 {
					continue
				}
			}
			if inherited && n.isNamed && !flattenedSpanHasFieldID(scratch.fieldIDs, spanStart, fieldEnd, fid) && countEligibleNamedFieldTargets(scratch.nodes, scratch.fieldIDs, spanStart, fieldEnd) > 1 {
				continue
			}
			if inherited && !flattenedSpanHasFieldID(scratch.fieldIDs, spanStart, fieldEnd, fid) && flattenedSpanHasAnyDirectField(scratch.nodes, scratch.fieldIDs, scratch.fieldSources, spanStart, fieldEnd) {
				if fieldEnd-spanStart != 1 {
					continue
				}
				child := scratch.nodes[spanStart]
				if child == nil || !nodeHasDirectFieldID(child, fid) {
					continue
				}
			}
			if !inherited || !fieldIDAppearsLater(rawFieldIDs, structuralChildIndex, fid) {
				applyFieldToFlattenedSpan(scratch.nodes, scratch.fieldIDs, scratch.fieldSources, spanStart, fieldEnd, fid, source, true)
				normalizeMixedSourceFieldSpan(scratch.fieldIDs, scratch.fieldSources, spanStart, fieldEnd)
			}
		}
	}
	if scratch.trackFields {
		p.suppressReducedChildFields(scratch.nodes, scratch.fieldIDs, scratch.fieldSources)
	}
	return materializeReduceChildrenFromScratch(scratch, arena)
}

func (p *Parser) buildReduceChildrenNoAliasNoFieldsStreaming(entries []stackEntry, start, end int, parentSymbol Symbol, symbolMeta []SymbolMetadata, arena *nodeArena) ([]*Node, []FieldID, []uint8) {
	visibleCount := 0
	allVisible := true
	for i := start; i < end; i++ {
		n := entries[i].node
		if n == nil {
			continue
		}
		visible := true
		if idx := int(n.symbol); idx < len(symbolMeta) {
			visible = symbolMeta[n.symbol].Visible
		}
		if !visible {
			allVisible = false
			break
		}
		visibleCount++
	}
	if allVisible {
		if visibleCount == 0 {
			return nil, nil, nil
		}
		children := arena.allocNodeSlice(visibleCount)
		out := 0
		for i := start; i < end; i++ {
			n := entries[i].node
			if n == nil {
				continue
			}
			children[out] = n
			out++
		}
		return children, nil, nil
	}

	var scratch *reduceBuildScratch
	if p != nil && p.reduceScratch != nil {
		scratch = p.reduceScratch
	} else {
		scratch = &reduceBuildScratch{}
	}
	scratch.reset()

	parentVisible := true
	if idx := int(parentSymbol); idx < len(symbolMeta) {
		parentVisible = symbolMeta[parentSymbol].Visible
	}
	for i := start; i < end; i++ {
		n := entries[i].node
		if n == nil {
			continue
		}
		visible := true
		if idx := int(n.symbol); idx < len(symbolMeta) {
			visible = symbolMeta[n.symbol].Visible
		}
		if visible {
			scratch.appendNode(n)
			continue
		}
		if parentVisible {
			appendFlattenedHiddenChildrenToScratch(scratch, n, symbolMeta)
			continue
		}
		if len(n.children) == 0 {
			continue
		}
		scratch.appendNode(n)
	}
	children, _, _ := materializeReduceChildrenFromScratch(scratch, arena)
	return children, nil, nil
}

func (p *Parser) shouldSuppressVisibleDirectField(n *Node, fid FieldID) bool {
	if p == nil || p.language == nil || n == nil || fid == 0 {
		return false
	}
	if p.language.Name != "dart" {
		return false
	}
	if int(fid) >= len(p.language.FieldNames) || p.language.FieldNames[fid] != "name" {
		return false
	}
	switch n.Type(p.language) {
	case "constructor_param", "super_formal_parameter":
		return true
	default:
		return false
	}
}

func (p *Parser) suppressReducedChildFields(children []*Node, fieldIDs []FieldID, fieldSources []uint8) {
	if p == nil || len(children) == 0 || len(fieldIDs) == 0 {
		return
	}
	limit := len(children)
	if len(fieldIDs) < limit {
		limit = len(fieldIDs)
	}
	for i := 0; i < limit; i++ {
		if !p.shouldSuppressVisibleDirectField(children[i], fieldIDs[i]) {
			continue
		}
		fieldIDs[i] = 0
		if fieldSources != nil && i < len(fieldSources) {
			fieldSources[i] = fieldSourceNone
		}
	}
}

func countFlattenedHiddenChildren(n *Node, symbolMeta []SymbolMetadata) int {
	if n == nil {
		return 0
	}
	visible := true
	if idx := int(n.symbol); idx < len(symbolMeta) {
		visible = symbolMeta[n.symbol].Visible
	}
	if visible {
		return 1
	}
	count := 0
	for _, child := range n.children {
		count += countFlattenedHiddenChildren(child, symbolMeta)
	}
	return count
}

func appendFlattenedHiddenChildren(dst []*Node, out int, n *Node, symbolMeta []SymbolMetadata) int {
	return appendFlattenedHiddenChildrenWithFields(dst, nil, nil, out, n, symbolMeta)
}

func appendFlattenedHiddenChildrenWithFields(dst []*Node, fieldDst []FieldID, fieldSrcDst []uint8, out int, n *Node, symbolMeta []SymbolMetadata) int {
	if n == nil {
		return out
	}
	visible := true
	if idx := int(n.symbol); idx < len(symbolMeta) {
		visible = symbolMeta[n.symbol].Visible
	}
	if visible {
		dst[out] = n
		return out + 1
	}
	nodeStart := out
	type hiddenFieldSpan struct {
		count  int
		source uint8
	}
	var repeated map[FieldID]hiddenFieldSpan
	for i, child := range n.children {
		spanStart := out
		out = appendFlattenedHiddenChildrenWithFields(dst, fieldDst, fieldSrcDst, out, child, symbolMeta)
		if fieldDst != nil && i < len(n.fieldIDs) && n.fieldIDs[i] != 0 {
			source := fieldSourceAt(n.fieldSources, i)
			if source == fieldSourceNone {
				source = fieldSourceDirect
			}
			applyFieldToFlattenedSpan(dst, fieldDst, fieldSrcDst, spanStart, out, n.fieldIDs[i], source, false)
			if source == fieldSourceDirect && spanStart < out {
				if repeated == nil {
					repeated = make(map[FieldID]hiddenFieldSpan)
				}
				span := repeated[n.fieldIDs[i]]
				span.count++
				span.source = source
				repeated[n.fieldIDs[i]] = span
			}
		}
	}
	for fid, span := range repeated {
		if span.count < 2 {
			continue
		}
		applyFieldToFlattenedSpan(dst, fieldDst, fieldSrcDst, nodeStart, out, fid, span.source, false)
	}
	normalizeMixedSourceFieldSpan(fieldDst, fieldSrcDst, nodeStart, out)
	return out
}

func normalizeMixedSourceFieldSpan(fieldIDs []FieldID, fieldSources []uint8, start, end int) {
	if fieldIDs == nil || fieldSources == nil || start >= end {
		return
	}
	type mixedSourceSpan struct {
		firstDirect  int
		lastDirect   int
		hasDirect    bool
		hasInherited bool
	}
	type mixedSourceEntry struct {
		fid  FieldID
		span mixedSourceSpan
	}
	var small [8]mixedSourceEntry
	spans := small[:0]
	for i := start; i < end; i++ {
		fid := fieldIDs[i]
		if fid == 0 {
			continue
		}
		source := fieldSourceAt(fieldSources, i)
		if source != fieldSourceDirect && source != fieldSourceInherited {
			continue
		}
		idx := -1
		for j := range spans {
			if spans[j].fid == fid {
				idx = j
				break
			}
		}
		if idx < 0 {
			spans = append(spans, mixedSourceEntry{
				fid: fid,
				span: mixedSourceSpan{
					firstDirect: -1,
					lastDirect:  -1,
				},
			})
			idx = len(spans) - 1
		}
		span := &spans[idx].span
		switch source {
		case fieldSourceDirect:
			if !span.hasDirect {
				span.firstDirect = i
			}
			span.lastDirect = i
			span.hasDirect = true
		case fieldSourceInherited:
			span.hasInherited = true
		}
	}
	for _, entry := range spans {
		fid := entry.fid
		span := entry.span
		if !span.hasDirect || !span.hasInherited {
			continue
		}
		for i := start; i < end; i++ {
			if fieldIDs[i] != fid || fieldSourceAt(fieldSources, i) != fieldSourceInherited {
				continue
			}
			if i < span.firstDirect || i > span.lastDirect {
				fieldIDs[i] = 0
				fieldSources[i] = fieldSourceNone
			}
		}
	}
}

func applyFieldToFlattenedSpan(children []*Node, fieldIDs []FieldID, fieldSources []uint8, start, end int, fid FieldID, source uint8, preferNamed bool) {
	if fid == 0 || fieldIDs == nil || start >= end {
		return
	}
	inherited := source == fieldSourceInherited
	conflictCount, multipleKinds := flattenedSpanConflictSummary(children, fieldIDs, start, end, fid)
	override := !multipleKinds && conflictCount >= 2
	if override {
		for j := start; j < end; j++ {
			if children[j] == nil || children[j].isExtra || children[j].isMissing {
				continue
			}
			if inherited && fieldIDs[j] != 0 && fieldIDs[j] != fid && fieldSourceAt(fieldSources, j) == fieldSourceDirect {
				continue
			}
			fieldIDs[j] = fid
			if fieldSources != nil {
				fieldSources[j] = source
			}
		}
		return
	}
	if !multipleKinds && conflictCount == 1 && preferNamed {
		for j := start; j < end; j++ {
			if children[j] == nil || children[j].isExtra || children[j].isMissing || !children[j].isNamed {
				continue
			}
			if inherited && fieldIDs[j] != 0 && fieldIDs[j] != fid && fieldSourceAt(fieldSources, j) == fieldSourceDirect {
				continue
			}
			fieldIDs[j] = fid
			if fieldSources != nil {
				fieldSources[j] = source
			}
			return
		}
	}
	alreadyAssigned := false
	for j := start; j < end; j++ {
		if fieldIDs[j] == fid {
			alreadyAssigned = true
			break
		}
	}
	if source == fieldSourceDirect && alreadyAssigned {
		first := -1
		for j := start; j < end; j++ {
			if fieldIDs[j] != fid {
				continue
			}
			if first < 0 {
				first = j
			}
		}
	}
	if inherited && !preferNamed && !alreadyAssigned {
		if countEligibleNamedFieldTargets(children, fieldIDs, start, end) > 1 {
			return
		}
	}
	namedTargets := 0
	totalTargets := 0
	allowAnonymousSingleDirectTarget := false
	if source == fieldSourceDirect && !alreadyAssigned {
		namedTargets = countEligibleNamedFieldTargets(children, fieldIDs, start, end)
		totalTargets = countEligibleFieldTargets(children, fieldIDs, start, end)
		allowAnonymousSingleDirectTarget = namedTargets == 0 && totalTargets == 1
	}
	for j := start; !alreadyAssigned && j < end; j++ {
		if fieldIDs[j] != 0 || children[j] == nil || children[j].isExtra || children[j].isMissing {
			continue
		}
		if preferNamed && !allowAnonymousSingleDirectTarget && !children[j].isNamed {
			continue
		}
		if inherited && nodeHasDirectFieldID(children[j], fid) && end-start != 1 {
			continue
		}
		if source == fieldSourceDirect {
			if namedTargets == 0 && totalTargets == 1 {
				for k := start; k < end; k++ {
					if children[k] == nil || children[k].isExtra || children[k].isMissing || fieldIDs[k] != 0 {
						continue
					}
					fieldIDs[k] = fid
					if fieldSources != nil {
						fieldSources[k] = source
					}
					break
				}
				break
			}
			if namedTargets > 1 {
				for k := start; k < end; k++ {
					if children[k] == nil || children[k].isExtra || children[k].isMissing || !children[k].isNamed || fieldIDs[k] != 0 {
						continue
					}
					fieldIDs[k] = fid
					if fieldSources != nil {
						fieldSources[k] = source
					}
				}
				break
			}
			if namedTargets == 1 && totalTargets > 1 {
				for k := start; k < end; k++ {
					if children[k] == nil || children[k].isExtra || children[k].isMissing || fieldIDs[k] != 0 {
						continue
					}
					fieldIDs[k] = fid
					if fieldSources != nil {
						fieldSources[k] = source
					}
				}
				break
			}
			if namedTargets == 1 {
				for k := start; k < end; k++ {
					if children[k] == nil || children[k].isExtra || children[k].isMissing || !children[k].isNamed || fieldIDs[k] != 0 {
						continue
					}
					fieldIDs[k] = fid
					if fieldSources != nil {
						fieldSources[k] = source
					}
				}
				break
			}
		}
		fieldIDs[j] = fid
		if fieldSources != nil {
			fieldSources[j] = source
		}
		break
	}
}

func flattenedSpanConflictSummary(children []*Node, fieldIDs []FieldID, start, end int, fid FieldID) (int, bool) {
	var conflict FieldID
	conflictCount := 0
	for j := start; j < end; j++ {
		if children[j] == nil || fieldIDs[j] == 0 || fieldIDs[j] == fid {
			continue
		}
		if nodeHasDirectFieldID(children[j], fieldIDs[j]) {
			continue
		}
		if conflict == 0 {
			conflict = fieldIDs[j]
			conflictCount = 1
			continue
		}
		if fieldIDs[j] != conflict {
			return conflictCount, true
		}
		conflictCount++
	}
	return conflictCount, false
}
func nodeHasDirectFieldID(n *Node, fid FieldID) bool {
	if n == nil || fid == 0 {
		return false
	}
	for i := range n.fieldIDs {
		if n.fieldIDs[i] == fid {
			return true
		}
	}
	return false
}

func nodeHasAnyDirectField(n *Node) bool {
	if n == nil {
		return false
	}
	for i := range n.fieldIDs {
		if n.fieldIDs[i] != 0 && fieldSourceAt(n.fieldSources, i) == fieldSourceDirect {
			return true
		}
	}
	for _, child := range n.children {
		if nodeHasAnyDirectField(child) {
			return true
		}
	}
	return false
}

func (p *Parser) applyReduceAction(s *glrStack, act ParseAction, tok Token, anyReduced *bool, nodeCount *int, arena *nodeArena, entryScratch *glrEntryScratch, gssScratch *gssScratch, entries []stackEntry, deferParentLinks bool, trackChildErrors bool) {
	childCount := int(act.ChildCount)
	window, ok := computeReduceRange(entries, childCount)
	if !ok {
		// Not enough stack entries — kill this stack version.
		s.dead = true
		return
	}

	children, fieldIDs, fieldSources := p.buildReduceChildren(entries, window.start, window.reducedEnd, childCount, act.Symbol, act.ProductionID, arena)

	trailingStart := window.reducedEnd
	trailingEnd := window.actualEnd

	// Pop all reduced entries in one step after collection.
	if !s.truncate(window.start) {
		s.dead = true
		return
	}

	if child := p.collapsibleUnarySelfReduction(act, tok, arena, entries, window.start, window.reducedEnd, children, fieldIDs); child != nil {
		gotoState := p.lookupGoto(window.topState, act.Symbol)
		targetState := window.topState
		if gotoState != 0 {
			targetState = gotoState
		}
		if tok.NoLookahead && targetState == window.topState {
			child.isExtra = true
		}
		child.productionID = act.ProductionID
		child.preGotoState = window.topState
		child.parseState = targetState
		nodeBumpEquivVersion(child)
		p.pushStackNode(s, targetState, child, entryScratch, gssScratch)
		for i := trailingStart; i < trailingEnd; i++ {
			extra := entries[i].node
			if extra == nil {
				continue
			}
			extra.parseState = targetState
			nodeBumpEquivVersion(extra)
			p.pushStackNode(s, targetState, extra, entryScratch, gssScratch)
		}
		s.score += int(act.DynamicPrecedence)
		*anyReduced = true
		return
	}

	named := p.isNamedSymbol(act.Symbol)
	var parent *Node
	if deferParentLinks {
		parent = newParentNodeInArenaNoLinksWithFieldSources(arena, act.Symbol, named, children, fieldIDs, fieldSources, act.ProductionID, trackChildErrors)
	} else {
		parent = newParentNodeInArenaWithFieldSources(arena, act.Symbol, named, children, fieldIDs, fieldSources, act.ProductionID)
	}
	shouldUseRawSpan := shouldUseRawSpanForReduction(act.Symbol, children, p.language.SymbolMetadata, p.forceRawSpanAll, p.forceRawSpanTable)
	if shouldUseRawSpan && window.reducedEnd > window.start {
		span := computeReduceRawSpan(entries, window.start, window.reducedEnd)
		if int(act.Symbol) < len(p.forceRawSpanTable) && p.forceRawSpanTable[act.Symbol] && window.actualEnd > window.reducedEnd {
			extendRawSpanToTrailingEntries(&span, entries, window.reducedEnd, window.actualEnd)
		}
		parent.startByte = span.startByte
		parent.endByte = span.endByte
		parent.startPoint = span.startPoint
		parent.endPoint = span.endPoint
	}
	// Extend parent span to cover invisible children dropped by buildReduceChildren.
	extendParentSpanToWindow(parent, entries, window.start, window.reducedEnd, p.language.SymbolMetadata, p.language.SymbolNames)
	*nodeCount++

	gotoState := p.lookupGoto(window.topState, act.Symbol)
	targetState := window.topState
	if gotoState != 0 {
		targetState = gotoState
	}
	if tok.NoLookahead && targetState == window.topState {
		parent.isExtra = true
	}
	parent.preGotoState = window.topState
	parent.parseState = targetState
	p.pushStackNode(s, targetState, parent, entryScratch, gssScratch)
	for i := trailingStart; i < trailingEnd; i++ {
		extra := entries[i].node
		if extra == nil {
			continue
		}
		extra.parseState = targetState
		nodeBumpEquivVersion(extra)
		p.pushStackNode(s, targetState, extra, entryScratch, gssScratch)
	}

	s.score += int(act.DynamicPrecedence)
	*anyReduced = true
}

func (p *Parser) collapsibleUnarySelfReduction(act ParseAction, tok Token, arena *nodeArena, entries []stackEntry, start, reducedEnd int, children []*Node, fieldIDs []FieldID) *Node {
	if p == nil || arena == nil || tok.NoLookahead {
		return nil
	}
	if reducedEnd-start != 1 || len(children) != 1 || len(fieldIDs) != 0 {
		return nil
	}
	child := children[0]
	if child == nil || child.ownerArena != arena || child.parent != nil {
		return nil
	}
	if start < 0 || start >= len(entries) || entries[start].node != child {
		return nil
	}
	if p.reduceProductionHasFields(act.ProductionID) || len(p.reduceAliasSequence(act.ProductionID)) != 0 {
		return nil
	}
	if child.symbol != act.Symbol {
		if child.ChildCount() != 0 || !p.canCollapseNamedLeafWrapper(act.Symbol, child.symbol) {
			return nil
		}
		if !p.isSingleTokenWrapperSymbol(act.Symbol) && !p.sameSymbolName(act.Symbol, child.symbol) {
			return nil
		}
		return aliasedNodeInArena(arena, p.language, child, act.Symbol)
	}
	return child
}

func (p *Parser) canCollapseNamedLeafWrapper(parentSym, childSym Symbol) bool {
	if p == nil || p.language == nil {
		return false
	}
	if parentSym == childSym {
		return true
	}
	meta := p.language.SymbolMetadata
	if int(parentSym) >= len(meta) || int(childSym) >= len(meta) {
		return false
	}
	parent := meta[parentSym]
	child := meta[childSym]
	if !parent.Visible || !parent.Named {
		return false
	}
	if !child.Visible || child.Named {
		return false
	}
	return true
}

func (p *Parser) isSingleTokenWrapperSymbol(sym Symbol) bool {
	if p == nil || len(p.singleTokenWrapperSymbol) == 0 {
		return false
	}
	if int(sym) < 0 || int(sym) >= len(p.singleTokenWrapperSymbol) {
		return false
	}
	return p.singleTokenWrapperSymbol[sym]
}

func (p *Parser) sameSymbolName(a, b Symbol) bool {
	if p == nil || p.language == nil {
		return false
	}
	meta := p.language.SymbolMetadata
	if int(a) < len(meta) && int(b) < len(meta) {
		an := meta[a].Name
		bn := meta[b].Name
		if an != "" && bn != "" {
			return an == bn
		}
	}
	names := p.language.SymbolNames
	if int(a) >= len(names) || int(b) >= len(names) {
		return false
	}
	return names[a] == names[b]
}

func recoverAction(entry *ParseActionEntry) (ParseAction, bool) {
	if entry == nil {
		return ParseAction{}, false
	}
	for _, act := range entry.Actions {
		if act.Type == ParseActionRecover {
			return act, true
		}
	}
	return ParseAction{}, false
}

func (p *Parser) findRecoverActionOnStack(s *glrStack, sym Symbol, timing *incrementalParseTiming) (int, ParseAction, bool) {
	if s == nil {
		return 0, ParseAction{}, false
	}
	if s.recoverabilityKnown && !s.mayRecover {
		return 0, ParseAction{}, false
	}
	if timing != nil {
		timing.recoverSearches++
	}
	if !p.symbolCanRecover(sym) {
		if timing != nil {
			timing.recoverSymbolSkips++
		}
		return 0, ParseAction{}, false
	}

	if len(s.entries) > 0 {
		entries := s.entries
		for depth := len(entries) - 1; depth >= 0; depth-- {
			state := entries[depth].state
			if timing != nil {
				timing.recoverStateChecks++
			}
			if !p.stateCanRecover(state) {
				if timing != nil {
					timing.recoverStateSkips++
				}
				continue
			}
			if timing != nil {
				timing.recoverLookups++
			}
			if act, ok := p.recoverActionForState(state, sym); ok {
				if timing != nil {
					timing.recoverHits++
				}
				return depth, act, true
			}
		}
		return 0, ParseAction{}, false
	}

	if s.gss.head == nil {
		return 0, ParseAction{}, false
	}

	depth := s.gss.len() - 1
	for n := s.gss.head; n != nil; n = n.prev {
		state := n.entry.state
		if timing != nil {
			timing.recoverStateChecks++
		}
		if !p.stateCanRecover(state) {
			if timing != nil {
				timing.recoverStateSkips++
			}
			depth--
			continue
		}
		if timing != nil {
			timing.recoverLookups++
		}
		if act, ok := p.recoverActionForState(state, sym); ok {
			if timing != nil {
				timing.recoverHits++
			}
			return depth, act, true
		}
		depth--
	}
	return 0, ParseAction{}, false
}

func (p *Parser) reduceAliasSequence(productionID uint16) []Symbol {
	if p == nil {
		return nil
	}
	pid := int(productionID)
	if pid < 0 || pid >= len(p.reduceAliasSeq) {
		return nil
	}
	return p.reduceAliasSeq[pid]
}

func (p *Parser) reduceProductionHasFields(productionID uint16) bool {
	if p == nil {
		return false
	}
	pid := int(productionID)
	if pid < 0 || pid >= len(p.reduceHasFields) {
		return false
	}
	return p.reduceHasFields[pid]
}

func aliasedNodeInArena(arena *nodeArena, lang *Language, n *Node, alias Symbol) *Node {
	if n == nil || alias == 0 || n.symbol == alias {
		return n
	}

	if lang != nil {
		if idx := int(n.symbol); idx < len(lang.SymbolMetadata) && !lang.SymbolMetadata[n.symbol].Visible {
			if child := flattenedVisibleAliasTarget(n, lang.SymbolMetadata); child != nil {
				n = child
			} else {
				n = materializeHiddenNodeForAlias(arena, lang, n)
			}
		}
	}

	if arena == nil {
		cloned := &Node{}
		*cloned = *n
		cloned.symbol = alias
		if lang != nil && int(alias) < len(lang.SymbolMetadata) {
			cloned.isNamed = lang.SymbolMetadata[alias].Named
		}
		return cloned
	}

	cloned := arena.allocNode()
	*cloned = *n
	cloned.symbol = alias
	if lang != nil && int(alias) < len(lang.SymbolMetadata) {
		cloned.isNamed = lang.SymbolMetadata[alias].Named
	}
	cloned.ownerArena = arena
	return cloned
}

func flattenedVisibleAliasTarget(n *Node, symbolMeta []SymbolMetadata) *Node {
	if n == nil || hiddenTreeHasFieldIDs(n) {
		return nil
	}
	if countFlattenedHiddenChildren(n, symbolMeta) != 1 {
		return nil
	}
	for n != nil {
		visible := true
		if idx := int(n.symbol); idx < len(symbolMeta) {
			visible = symbolMeta[n.symbol].Visible
		}
		if visible {
			return n
		}
		var next *Node
		for _, child := range n.children {
			if countFlattenedHiddenChildren(child, symbolMeta) == 0 {
				continue
			}
			next = child
			break
		}
		n = next
	}
	return nil
}

func cloneNodeInArena(arena *nodeArena, n *Node) *Node {
	if n == nil {
		return nil
	}
	if arena == nil {
		cloned := &Node{}
		*cloned = *n
		return cloned
	}
	cloned := arena.allocNode()
	*cloned = *n
	cloned.ownerArena = arena
	return cloned
}

func materializeHiddenNodeForAlias(arena *nodeArena, lang *Language, n *Node) *Node {
	if n == nil || lang == nil {
		return n
	}
	symbolMeta := lang.SymbolMetadata
	normalizedCount := countFlattenedHiddenChildren(n, symbolMeta)
	if normalizedCount == 0 {
		cloned := cloneNodeInArena(arena, n)
		cloned.children = nil
		cloned.fieldIDs = nil
		cloned.fieldSources = nil
		return cloned
	}

	cloned := cloneNodeInArena(arena, n)
	children := arena.allocNodeSlice(normalizedCount)
	var fieldIDs []FieldID
	var fieldSources []uint8
	if hiddenTreeHasFieldIDs(n) {
		fieldIDs = arena.allocFieldIDSlice(normalizedCount)
		fieldSources = arena.allocFieldSourceSlice(normalizedCount)
	}
	out := appendFlattenedHiddenChildrenWithFields(children, fieldIDs, fieldSources, 0, n, symbolMeta)
	cloned.children = children[:out]
	if len(fieldIDs) > 0 {
		fieldIDs = fieldIDs[:out]
		fieldSources = fieldSources[:out]
		hasField := false
		for _, fid := range fieldIDs {
			if fid != 0 {
				hasField = true
				break
			}
		}
		if hasField {
			cloned.fieldIDs = fieldIDs
			cloned.fieldSources = fieldSources
		} else {
			cloned.fieldIDs = nil
			cloned.fieldSources = nil
		}
	} else {
		cloned.fieldIDs = nil
		cloned.fieldSources = nil
	}
	return cloned
}

func hiddenTreeHasFieldIDs(n *Node) bool {
	if n == nil {
		return false
	}
	for _, fid := range n.fieldIDs {
		if fid != 0 {
			return true
		}
	}
	for _, child := range n.children {
		if hiddenTreeHasFieldIDs(child) {
			return true
		}
	}
	return false
}

func (p *Parser) fieldFlagScratch(childCount int) ([]bool, []bool) {
	if p == nil || childCount <= 0 {
		return nil, nil
	}
	if cap(p.fieldInheritedScratch) < childCount {
		p.fieldInheritedScratch = make([]bool, childCount)
	} else {
		p.fieldInheritedScratch = p.fieldInheritedScratch[:childCount]
		clear(p.fieldInheritedScratch)
	}
	if cap(p.fieldConflictedScratch) < childCount {
		p.fieldConflictedScratch = make([]bool, childCount)
	} else {
		p.fieldConflictedScratch = p.fieldConflictedScratch[:childCount]
		clear(p.fieldConflictedScratch)
	}
	return p.fieldInheritedScratch, p.fieldConflictedScratch
}

// buildFieldIDs creates the field ID slice for a reduce action.
func (p *Parser) buildFieldIDs(childCount int, productionID uint16, arena *nodeArena) ([]FieldID, []bool) {
	if childCount <= 0 || len(p.language.FieldMapEntries) == 0 {
		return nil, nil
	}

	pid := int(productionID)
	if pid >= len(p.language.FieldMapSlices) {
		return nil, nil
	}
	if pid >= len(p.reduceHasFields) || !p.reduceHasFields[pid] {
		return nil, nil
	}

	fm := p.language.FieldMapSlices[pid]
	count := int(fm[1])
	if count == 0 {
		return nil, nil
	}

	var fieldIDs []FieldID
	inherited, conflictedInherited := p.fieldFlagScratch(childCount)
	start := int(fm[0])
	assigned := false
	for i := 0; i < count; i++ {
		entryIdx := start + i
		if entryIdx >= len(p.language.FieldMapEntries) {
			break
		}
		entry := p.language.FieldMapEntries[entryIdx]
		if int(entry.ChildIndex) < childCount {
			if fieldIDs == nil {
				fieldIDs = arena.allocFieldIDSlice(childCount)
			}
			idx := entry.ChildIndex
			switch {
			case conflictedInherited[idx]:
				if !entry.Inherited {
					fieldIDs[idx] = entry.FieldID
					inherited[idx] = false
					conflictedInherited[idx] = false
				}
			case fieldIDs[idx] == 0:
				fieldIDs[idx] = entry.FieldID
				inherited[idx] = entry.Inherited
			case !entry.Inherited && inherited[idx]:
				fieldIDs[idx] = entry.FieldID
				inherited[idx] = false
			case entry.Inherited && inherited[idx] && fieldIDs[idx] != entry.FieldID:
				fieldIDs[idx] = 0
				inherited[idx] = false
				conflictedInherited[idx] = true
			case entry.Inherited == inherited[idx]:
				fieldIDs[idx] = entry.FieldID
				inherited[idx] = entry.Inherited
			}
			assigned = true
		}
	}

	if !assigned {
		return nil, nil
	}
	return fieldIDs, inherited
}
