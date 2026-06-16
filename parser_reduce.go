package gotreesitter

import (
	"fmt"
	"time"
)

type reduceChainSignature struct {
	state        StateID
	depth        int
	symbol       Symbol
	childCount   uint8
	productionID uint16
}

// A repeated signature has the same state, stack depth, reduced symbol, child
// count, and production ID. Applying it again cannot advance parser state; it
// only builds another wrapper around the same top-level shape.
const maxRepeatedReduceChainSignature = 1

type classifiedParseActionClass uint8

const (
	classifiedParseActionNoAction classifiedParseActionClass = iota
	classifiedParseActionSingleReduce
	classifiedParseActionSingleShift
	classifiedParseActionSingleAccept
	classifiedParseActionSingleOther
	classifiedParseActionMulti
)

type classifiedParseAction struct {
	class  classifiedParseActionClass
	action ParseAction
}

type reduceChainHint struct {
	startState     StateID
	lookahead      Symbol
	terminalStates []StateID
	terminalAction classifiedParseActionClass
	maxSteps       uint16
}

func buildReduceChainHintIndex(hints []reduceChainHint) []int {
	if len(hints) == 0 {
		return nil
	}
	maxState := StateID(0)
	for _, hint := range hints {
		if hint.startState > maxState {
			maxState = hint.startState
		}
	}
	index := make([]int, int(maxState)+1)
	for i := range index {
		index[i] = -1
	}
	for i, hint := range hints {
		slot := int(hint.startState)
		if index[slot] == -1 {
			index[slot] = i
		} else {
			index[slot] = -2
		}
	}
	return index
}

func buildClassifiedParseActions(lang *Language) []classifiedParseAction {
	if lang == nil || len(lang.ParseActions) == 0 {
		return nil
	}
	out := make([]classifiedParseAction, len(lang.ParseActions))
	for i := range lang.ParseActions {
		out[i] = classifyParseActionEntry(lang.ParseActions[i])
	}
	return out
}

func classifyParseActionEntry(entry ParseActionEntry) classifiedParseAction {
	if len(entry.Actions) == 0 {
		return classifiedParseAction{class: classifiedParseActionNoAction}
	}
	if len(entry.Actions) != 1 {
		return classifiedParseAction{class: classifiedParseActionMulti}
	}
	action := entry.Actions[0]
	switch action.Type {
	case ParseActionReduce:
		return classifiedParseAction{class: classifiedParseActionSingleReduce, action: action}
	case ParseActionShift:
		return classifiedParseAction{class: classifiedParseActionSingleShift, action: action}
	case ParseActionAccept:
		return classifiedParseAction{class: classifiedParseActionSingleAccept, action: action}
	default:
		return classifiedParseAction{class: classifiedParseActionSingleOther, action: action}
	}
}

func buildReduceChainHints(lang *Language) []reduceChainHint {
	if lang == nil || !parseReduceChainHintsEnabled() {
		return nil
	}
	if len(lang.ReduceChainHints) != 0 {
		return buildReduceChainHintsFromMetadata(lang, lang.ReduceChainHints)
	}
	return buildReduceChainHintsFromMetadata(lang, defaultReduceChainHintMetadata(lang))
}

func buildReduceChainHintsFromMetadata(lang *Language, hints []ReduceChainHint) []reduceChainHint {
	out := make([]reduceChainHint, 0, len(hints))
	for i := range hints {
		hint := hints[i]
		terminalAction, ok := reduceChainTerminalActionClass(hint.TerminalAction)
		if !ok || hint.MaxSteps == 0 || !reduceChainHintInRange(lang, hint) {
			continue
		}
		out = append(out, reduceChainHint{
			startState:     hint.StartState,
			lookahead:      hint.Lookahead,
			terminalStates: append([]StateID(nil), hint.TerminalStates...),
			terminalAction: terminalAction,
			maxSteps:       hint.MaxSteps,
		})
	}
	return out
}

func reduceChainTerminalActionClass(action ReduceChainTerminalAction) (classifiedParseActionClass, bool) {
	switch action {
	case ReduceChainTerminalNoAction:
		return classifiedParseActionNoAction, true
	case ReduceChainTerminalSingleReduce:
		return classifiedParseActionSingleReduce, true
	case ReduceChainTerminalSingleShift:
		return classifiedParseActionSingleShift, true
	case ReduceChainTerminalSingleAccept:
		return classifiedParseActionSingleAccept, true
	case ReduceChainTerminalSingleOther:
		return classifiedParseActionSingleOther, true
	case ReduceChainTerminalMulti:
		return classifiedParseActionMulti, true
	default:
		return classifiedParseActionNoAction, false
	}
}

func reduceChainHintInRange(lang *Language, hint ReduceChainHint) bool {
	if lang == nil || uint32(hint.StartState) >= lang.StateCount || uint32(hint.Lookahead) >= lang.SymbolCount || len(hint.TerminalStates) == 0 {
		return false
	}
	for _, state := range hint.TerminalStates {
		if uint32(state) >= lang.StateCount {
			return false
		}
	}
	return true
}

func defaultReduceChainHintMetadata(lang *Language) []ReduceChainHint {
	switch lang.Name {
	case "python":
		if !languageSymbolNameMatches(lang, Symbol(101), "_newline") {
			return nil
		}
		return []ReduceChainHint{{
			StartState:     StateID(1101),
			Lookahead:      Symbol(101),
			TerminalStates: []StateID{StateID(2336), StateID(2361), StateID(2098), StateID(2460)},
			TerminalAction: ReduceChainTerminalSingleShift,
			MaxSteps:       10,
		}}
	case "rust":
		if !languageSymbolNameMatches(lang, Symbol(5), ")") {
			return nil
		}
		return []ReduceChainHint{{
			StartState:     StateID(205),
			Lookahead:      Symbol(5),
			TerminalStates: []StateID{StateID(98), StateID(132), StateID(133)},
			TerminalAction: ReduceChainTerminalSingleShift,
			MaxSteps:       32,
		}}
	default:
		return nil
	}
}

func languageSymbolNameMatches(lang *Language, sym Symbol, name string) bool {
	if lang == nil {
		return false
	}
	idx := int(sym)
	return idx >= 0 && idx < len(lang.SymbolNames) && lang.SymbolNames[idx] == name
}

func reduceChainTerminalState(s *glrStack, fallback StateID) StateID {
	if s == nil || s.dead {
		return fallback
	}
	return s.top().state
}

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
	for _, entry := range lang.ParseActions {
		for _, act := range entry.Actions {
			if act.Type != ParseActionReduce {
				continue
			}
			pid := int(act.ProductionID)
			if pid < 0 || pid >= len(out) {
				continue
			}
			if !out[pid] {
				continue
			}
			if fieldMapHasEffectiveFields(lang, int(act.ChildCount), act.ProductionID) {
				continue
			}
			out[pid] = false
		}
	}
	return out
}

func fieldMapHasEffectiveFields(lang *Language, childCount int, productionID uint16) bool {
	if lang == nil || childCount <= 0 || len(lang.FieldMapEntries) == 0 {
		return false
	}
	pid := int(productionID)
	if pid < 0 || pid >= len(lang.FieldMapSlices) {
		return false
	}
	fm := lang.FieldMapSlices[pid]
	count := int(fm[1])
	if count == 0 {
		return false
	}
	fieldIDs := make([]FieldID, childCount)
	inherited := make([]bool, childCount)
	conflictedInherited := make([]bool, childCount)
	start := int(fm[0])
	for i := 0; i < count; i++ {
		entryIdx := start + i
		if entryIdx >= len(lang.FieldMapEntries) {
			break
		}
		entry := lang.FieldMapEntries[entryIdx]
		if int(entry.ChildIndex) >= childCount {
			continue
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
	}
	return fieldIDSliceHasAny(fieldIDs)
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
	if act.Type != ParseActionReduce {
		p.applyAction(s, act, tok, anyReduced, nodeCount, arena, entryScratch, gssScratch, tmpEntries, deferParentLinks, trackChildErrors)
		return false
	}
	p.applyReduceActionDispatch(s, act, tok, anyReduced, nodeCount, arena, entryScratch, gssScratch, tmpEntries, deferParentLinks, trackChildErrors)
	if tok.NoLookahead || s == nil || s.dead || s.accepted || s.shifted {
		return false
	}
	return p.chainSingleReduceActions(s, tok, anyReduced, nodeCount, arena, entryScratch, gssScratch, tmpEntries, deferParentLinks, trackChildErrors)
}

func (p *Parser) pushOrExtendErrorNode(s *glrStack, state StateID, tok Token, nodeCount *int, arena *nodeArena, entryScratch *glrEntryScratch, gssScratch *gssScratch, trackChildErrors *bool) {
	if s != nil {
		top := stackEntryNode(s.top())
		if top != nil &&
			top.symbol == errorSymbol &&
			!top.isMissing() &&
			len(top.children) == 0 &&
			top.parseState == state &&
			tok.StartByte >= top.endByte {
			top.endByte = tok.EndByte
			top.endPoint = tok.EndPoint
			top.setHasError(true)
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
	errNode.setHasError(true)
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

func noteRepeatedReduceChainSignature(prev reduceChainSignature, prevCount int, next reduceChainSignature) (reduceChainSignature, int, bool) {
	if prev == next {
		prevCount++
	} else {
		prev = next
		prevCount = 1
	}
	return prev, prevCount, prevCount > maxRepeatedReduceChainSignature
}

func noteRepeatedReduceChainAction(prev *reduceChainSignature, prevCount int, state StateID, depth int, act ParseAction) (int, bool) {
	if prev.state == state &&
		prev.depth == depth &&
		prev.symbol == act.Symbol &&
		prev.childCount == act.ChildCount &&
		prev.productionID == act.ProductionID {
		prevCount++
	} else {
		prev.state = state
		prev.depth = depth
		prev.symbol = act.Symbol
		prev.childCount = act.ChildCount
		prev.productionID = act.ProductionID
		prevCount = 1
	}
	return prevCount, prevCount > maxRepeatedReduceChainSignature
}

func (p *Parser) useCompactNoTreeShiftLeaf() bool {
	return p != nil && p.noTreeBenchmarkOnly && p.compactNoTreeShiftLeaves
}

func (p *Parser) useCompactFullShiftLeaf() bool {
	return p != nil && !p.noTreeBenchmarkOnly && p.compactFullShiftLeaves
}

func (p *Parser) usePendingFullParents() bool {
	return p != nil && !p.noTreeBenchmarkOnly && p.pendingFullParents
}

func (p *Parser) canCompactFullShiftLeaf(act ParseAction, tok Token) bool {
	return p.useCompactFullShiftLeaf() &&
		!act.Extra &&
		!tok.NoLookahead &&
		!p.shiftTokenIsMissingError(tok)
}

func (p *Parser) shiftTokenIsMissingError(tok Token) bool {
	if tok.Missing {
		return true
	}
	return p != nil && p.language != nil &&
		(p.language.Name == "c" || p.language.Name == "cpp" || p.language.Name == "objc") &&
		tok.Symbol != 0 && tok.StartByte == tok.EndByte && tok.Text == ""
}

func (p *Parser) chainSingleReduceActions(s *glrStack, tok Token, anyReduced *bool, nodeCount *int, arena *nodeArena, entryScratch *glrEntryScratch, gssScratch *gssScratch, tmpEntries *[]stackEntry, deferParentLinks bool, trackChildErrors *bool) bool {
	if s == nil || s.dead || s.accepted || s.shifted {
		return false
	}
	if p.ambiguityProfile != nil {
		return p.chainSingleReduceActionsProfiled(s, tok, anyReduced, nodeCount, arena, entryScratch, gssScratch, tmpEntries, deferParentLinks, trackChildErrors)
	}
	if len(p.classifiedActions) == len(p.language.ParseActions) {
		return p.chainSingleReduceActionsClassified(s, tok, anyReduced, nodeCount, arena, entryScratch, gssScratch, tmpEntries, deferParentLinks, trackChildErrors)
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
			repeatedSigCount, repeated = noteRepeatedReduceChainAction(&lastSig, repeatedSigCount, currentState, currentDepth, next)
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
			p.applyReduceActionDispatch(s, next, tok, anyReduced, nodeCount, arena, entryScratch, gssScratch, tmpEntries, deferParentLinks, trackChildErrors)
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

func (p *Parser) chainSingleReduceActionsClassified(s *glrStack, tok Token, anyReduced *bool, nodeCount *int, arena *nodeArena, entryScratch *glrEntryScratch, gssScratch *gssScratch, tmpEntries *[]stackEntry, deferParentLinks bool, trackChildErrors *bool) bool {
	if len(p.reduceChainHints) != 0 {
		if hint, ok := p.reduceChainHintFor(s.top().state, tok.Symbol); ok {
			return p.chainSingleReduceActionsClassifiedHinted(s, tok, hint, anyReduced, nodeCount, arena, entryScratch, gssScratch, tmpEntries, deferParentLinks, trackChildErrors)
		}
	}
	return p.chainSingleReduceActionsClassifiedDefault(s, tok, anyReduced, nodeCount, arena, entryScratch, gssScratch, tmpEntries, deferParentLinks, trackChildErrors)
}

func (p *Parser) reduceChainHintFor(state StateID, lookahead Symbol) (reduceChainHint, bool) {
	if int(state) >= len(p.reduceChainHintByState) {
		return reduceChainHint{}, false
	}
	hintIndex := p.reduceChainHintByState[int(state)]
	if hintIndex == -2 {
		for _, hint := range p.reduceChainHints {
			if hint.startState == state && hint.lookahead == lookahead {
				return hint, true
			}
		}
		return reduceChainHint{}, false
	}
	if hintIndex < 0 || hintIndex >= len(p.reduceChainHints) {
		return reduceChainHint{}, false
	}
	hint := p.reduceChainHints[hintIndex]
	if hint.lookahead != lookahead {
		return reduceChainHint{}, false
	}
	return hint, true
}

func reduceChainHintTerminalMatches(hint reduceChainHint, state StateID, class classifiedParseActionClass) bool {
	if class != hint.terminalAction {
		return false
	}
	return reduceChainHintTerminalStateAllowed(hint, state)
}

func reduceChainHintTerminalStateAllowed(hint reduceChainHint, state StateID) bool {
	for _, terminalState := range hint.terminalStates {
		if state == terminalState {
			return true
		}
	}
	return false
}

func (p *Parser) chainSingleReduceActionsClassifiedHinted(s *glrStack, tok Token, hint reduceChainHint, anyReduced *bool, nodeCount *int, arena *nodeArena, entryScratch *glrEntryScratch, gssScratch *gssScratch, tmpEntries *[]stackEntry, deferParentLinks bool, trackChildErrors *bool) bool {
	if perfCountersEnabled {
		perfRecordReduceChainHintCandidate()
		perfRecordReduceChainHintTaken()
	}
	actions := p.classifiedActions
	steps := 0
	for steps < int(hint.maxSteps) {
		currentState := s.top().state
		actionIdx := p.lookupActionIndex(currentState, tok.Symbol)
		if actionIdx == 0 || int(actionIdx) >= len(actions) {
			if reduceChainHintTerminalMatches(hint, currentState, classifiedParseActionNoAction) {
				if perfCountersEnabled {
					perfRecordReduceChainHintTerminalOK()
				}
				return false
			}
			if perfCountersEnabled {
				perfRecordReduceChainHintTerminalMismatch()
			}
			return false
		}
		classified := &actions[actionIdx]
		if classified.class != classifiedParseActionSingleReduce {
			if reduceChainHintTerminalMatches(hint, currentState, classified.class) {
				if perfCountersEnabled {
					perfRecordReduceChainHintTerminalOK()
				}
			} else if reduceChainHintTerminalStateAllowed(hint, currentState) {
				if perfCountersEnabled {
					perfRecordReduceChainHintUnexpected()
				}
			} else if perfCountersEnabled {
				perfRecordReduceChainHintTerminalMismatch()
			}
			switch classified.class {
			case classifiedParseActionSingleShift:
				if perfCountersEnabled {
					perfRecordReduceChainBreakShift()
				}
			case classifiedParseActionSingleAccept:
				if perfCountersEnabled {
					perfRecordReduceChainBreakAccept()
				}
			default:
				if perfCountersEnabled {
					perfRecordReduceChainBreakMulti()
				}
			}
			return false
		}

		steps++
		if perfCountersEnabled {
			perfRecordReduceChainStep(steps)
			perfRecordReduceChainHintSteps(1)
		}
		p.applyReduceActionDispatch(s, classified.action, tok, anyReduced, nodeCount, arena, entryScratch, gssScratch, tmpEntries, deferParentLinks, trackChildErrors)
		if s.dead || s.accepted || s.shifted {
			if perfCountersEnabled {
				perfRecordReduceChainHintDead()
			}
			return false
		}
	}
	if perfCountersEnabled {
		perfRecordReduceChainHintLimit()
	}
	return p.chainSingleReduceActionsClassifiedDefault(s, tok, anyReduced, nodeCount, arena, entryScratch, gssScratch, tmpEntries, deferParentLinks, trackChildErrors)
}

func (p *Parser) chainSingleReduceActionsClassifiedDefault(s *glrStack, tok Token, anyReduced *bool, nodeCount *int, arena *nodeArena, entryScratch *glrEntryScratch, gssScratch *gssScratch, tmpEntries *[]stackEntry, deferParentLinks bool, trackChildErrors *bool) bool {
	const maxInlineReduceChain = 256
	actions := p.classifiedActions
	chainLen := 0
	var lastSig reduceChainSignature
	repeatedSigCount := 0
	for chainLen < maxInlineReduceChain {
		currentState := s.top().state
		currentDepth := s.depth()
		actionIdx := p.lookupActionIndex(currentState, tok.Symbol)
		if actionIdx == 0 || int(actionIdx) >= len(actions) {
			return false
		}

		classified := &actions[actionIdx]
		switch classified.class {
		case classifiedParseActionSingleReduce:
			next := classified.action
			var repeated bool
			repeatedSigCount, repeated = noteRepeatedReduceChainAction(&lastSig, repeatedSigCount, currentState, currentDepth, next)
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
			p.applyReduceActionDispatch(s, next, tok, anyReduced, nodeCount, arena, entryScratch, gssScratch, tmpEntries, deferParentLinks, trackChildErrors)
			if s.dead || s.accepted || s.shifted {
				return false
			}
		case classifiedParseActionSingleShift:
			if perfCountersEnabled {
				perfRecordReduceChainBreakShift()
			}
			return false
		case classifiedParseActionSingleAccept:
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

func (p *Parser) chainSingleReduceActionsProfiled(s *glrStack, tok Token, anyReduced *bool, nodeCount *int, arena *nodeArena, entryScratch *glrEntryScratch, gssScratch *gssScratch, tmpEntries *[]stackEntry, deferParentLinks bool, trackChildErrors *bool) bool {
	if len(p.classifiedActions) == len(p.language.ParseActions) {
		return p.chainSingleReduceActionsClassifiedProfiled(s, tok, anyReduced, nodeCount, arena, entryScratch, gssScratch, tmpEntries, deferParentLinks, trackChildErrors)
	}
	const maxInlineReduceChain = 256
	parseActions := p.language.ParseActions
	chainLen := 0
	classHits := 0
	chainStartState := s.top().state
	chainStart := time.Now()
	var lastSig reduceChainSignature
	repeatedSigCount := 0
	for chainLen < maxInlineReduceChain {
		currentState := s.top().state
		currentDepth := s.depth()
		actionIdx := p.lookupActionIndex(currentState, tok.Symbol)
		if actionIdx == 0 || int(actionIdx) >= len(parseActions) {
			p.ambiguityProfile.recordReduceChainRun(chainStartState, tok.Symbol, currentState, classifiedParseActionNoAction, chainLen, chainLen, classHits, time.Since(chainStart).Nanoseconds(), reduceChainStopNoAction)
			return false
		}
		classHits++

		actions := parseActions[actionIdx].Actions
		if len(actions) != 1 {
			if perfCountersEnabled {
				perfRecordReduceChainBreakMulti()
			}
			p.ambiguityProfile.recordReduceChainRun(chainStartState, tok.Symbol, currentState, classifiedParseActionMulti, chainLen, chainLen, classHits, time.Since(chainStart).Nanoseconds(), reduceChainStopMulti)
			return false
		}

		next := actions[0]
		switch next.Type {
		case ParseActionReduce:
			var repeated bool
			repeatedSigCount, repeated = noteRepeatedReduceChainAction(&lastSig, repeatedSigCount, currentState, currentDepth, next)
			if repeated {
				if p != nil && p.glrTrace {
					fmt.Printf("      -> REDUCE-CHAIN CYCLE state=%d depth=%d sym=%d prod=%d count=%d\n",
						currentState, currentDepth, next.Symbol, next.ProductionID, repeatedSigCount)
				}
				p.ambiguityProfile.recordReduceChainRun(chainStartState, tok.Symbol, currentState, classifiedParseActionSingleReduce, chainLen, chainLen, classHits, time.Since(chainStart).Nanoseconds(), reduceChainStopCycle)
				return true
			}
			chainLen++
			if perfCountersEnabled {
				perfRecordReduceChainStep(chainLen)
			}
			reduceStart := time.Now()
			p.applyReduceActionDispatch(s, next, tok, anyReduced, nodeCount, arena, entryScratch, gssScratch, tmpEntries, deferParentLinks, trackChildErrors)
			p.ambiguityProfile.recordReduceChainStep(currentState, tok.Symbol, next, chainLen, time.Since(reduceStart).Nanoseconds())
			if s.dead || s.accepted || s.shifted {
				switch {
				case s.accepted:
					p.ambiguityProfile.recordReduceChainRun(chainStartState, tok.Symbol, reduceChainTerminalState(s, currentState), classifiedParseActionSingleReduce, chainLen, chainLen, classHits, time.Since(chainStart).Nanoseconds(), reduceChainStopAccept)
				case s.shifted:
					p.ambiguityProfile.recordReduceChainRun(chainStartState, tok.Symbol, reduceChainTerminalState(s, currentState), classifiedParseActionSingleReduce, chainLen, chainLen, classHits, time.Since(chainStart).Nanoseconds(), reduceChainStopShift)
				default:
					p.ambiguityProfile.recordReduceChainRun(chainStartState, tok.Symbol, reduceChainTerminalState(s, currentState), classifiedParseActionSingleReduce, chainLen, chainLen, classHits, time.Since(chainStart).Nanoseconds(), reduceChainStopDead)
				}
				return false
			}
		case ParseActionShift:
			if perfCountersEnabled {
				perfRecordReduceChainBreakShift()
			}
			p.ambiguityProfile.recordReduceChainRun(chainStartState, tok.Symbol, currentState, classifiedParseActionSingleShift, chainLen, chainLen, classHits, time.Since(chainStart).Nanoseconds(), reduceChainStopShift)
			return false
		case ParseActionAccept:
			if perfCountersEnabled {
				perfRecordReduceChainBreakAccept()
			}
			p.ambiguityProfile.recordReduceChainRun(chainStartState, tok.Symbol, currentState, classifiedParseActionSingleAccept, chainLen, chainLen, classHits, time.Since(chainStart).Nanoseconds(), reduceChainStopAccept)
			return false
		default:
			if perfCountersEnabled {
				perfRecordReduceChainBreakMulti()
			}
			p.ambiguityProfile.recordReduceChainRun(chainStartState, tok.Symbol, currentState, classifiedParseActionSingleOther, chainLen, chainLen, classHits, time.Since(chainStart).Nanoseconds(), reduceChainStopMulti)
			return false
		}
	}
	p.ambiguityProfile.recordReduceChainRun(chainStartState, tok.Symbol, reduceChainTerminalState(s, chainStartState), classifiedParseActionSingleReduce, chainLen, chainLen, classHits, time.Since(chainStart).Nanoseconds(), reduceChainStopLimit)
	return false
}

func (p *Parser) chainSingleReduceActionsClassifiedProfiled(s *glrStack, tok Token, anyReduced *bool, nodeCount *int, arena *nodeArena, entryScratch *glrEntryScratch, gssScratch *gssScratch, tmpEntries *[]stackEntry, deferParentLinks bool, trackChildErrors *bool) bool {
	if len(p.reduceChainHints) != 0 {
		if hint, ok := p.reduceChainHintFor(s.top().state, tok.Symbol); ok {
			return p.chainSingleReduceActionsClassifiedHintedProfiled(s, tok, hint, anyReduced, nodeCount, arena, entryScratch, gssScratch, tmpEntries, deferParentLinks, trackChildErrors)
		}
	}
	return p.chainSingleReduceActionsClassifiedProfiledDefault(s, tok, anyReduced, nodeCount, arena, entryScratch, gssScratch, tmpEntries, deferParentLinks, trackChildErrors)
}

func (p *Parser) chainSingleReduceActionsClassifiedHintedProfiled(s *glrStack, tok Token, hint reduceChainHint, anyReduced *bool, nodeCount *int, arena *nodeArena, entryScratch *glrEntryScratch, gssScratch *gssScratch, tmpEntries *[]stackEntry, deferParentLinks bool, trackChildErrors *bool) bool {
	if perfCountersEnabled {
		perfRecordReduceChainHintCandidate()
		perfRecordReduceChainHintTaken()
	}
	actions := p.classifiedActions
	chainLen := 0
	classHits := 0
	chainStartState := s.top().state
	chainStart := time.Now()
	for chainLen < int(hint.maxSteps) {
		currentState := s.top().state
		actionIdx := p.lookupActionIndex(currentState, tok.Symbol)
		if actionIdx == 0 || int(actionIdx) >= len(actions) {
			if reduceChainHintTerminalMatches(hint, currentState, classifiedParseActionNoAction) {
				if perfCountersEnabled {
					perfRecordReduceChainHintTerminalOK()
				}
			} else if perfCountersEnabled {
				perfRecordReduceChainHintTerminalMismatch()
			}
			p.ambiguityProfile.recordReduceChainRun(chainStartState, tok.Symbol, currentState, classifiedParseActionNoAction, chainLen, chainLen, classHits, time.Since(chainStart).Nanoseconds(), reduceChainStopNoAction)
			return false
		}
		classHits++

		classified := &actions[actionIdx]
		if classified.class != classifiedParseActionSingleReduce {
			if reduceChainHintTerminalMatches(hint, currentState, classified.class) {
				if perfCountersEnabled {
					perfRecordReduceChainHintTerminalOK()
				}
			} else if reduceChainHintTerminalStateAllowed(hint, currentState) {
				if perfCountersEnabled {
					perfRecordReduceChainHintUnexpected()
				}
			} else if perfCountersEnabled {
				perfRecordReduceChainHintTerminalMismatch()
			}
			stop := reduceChainStopMulti
			switch classified.class {
			case classifiedParseActionSingleShift:
				if perfCountersEnabled {
					perfRecordReduceChainBreakShift()
				}
				stop = reduceChainStopShift
			case classifiedParseActionSingleAccept:
				if perfCountersEnabled {
					perfRecordReduceChainBreakAccept()
				}
				stop = reduceChainStopAccept
			default:
				if perfCountersEnabled {
					perfRecordReduceChainBreakMulti()
				}
			}
			p.ambiguityProfile.recordReduceChainRun(chainStartState, tok.Symbol, currentState, classified.class, chainLen, chainLen, classHits, time.Since(chainStart).Nanoseconds(), stop)
			return false
		}

		next := classified.action
		chainLen++
		if perfCountersEnabled {
			perfRecordReduceChainStep(chainLen)
			perfRecordReduceChainHintSteps(1)
		}
		reduceStart := time.Now()
		p.applyReduceActionDispatch(s, next, tok, anyReduced, nodeCount, arena, entryScratch, gssScratch, tmpEntries, deferParentLinks, trackChildErrors)
		p.ambiguityProfile.recordReduceChainStep(currentState, tok.Symbol, next, chainLen, time.Since(reduceStart).Nanoseconds())
		if s.dead || s.accepted || s.shifted {
			if perfCountersEnabled {
				perfRecordReduceChainHintDead()
			}
			switch {
			case s.accepted:
				p.ambiguityProfile.recordReduceChainRun(chainStartState, tok.Symbol, reduceChainTerminalState(s, currentState), classifiedParseActionSingleReduce, chainLen, chainLen, classHits, time.Since(chainStart).Nanoseconds(), reduceChainStopAccept)
			case s.shifted:
				p.ambiguityProfile.recordReduceChainRun(chainStartState, tok.Symbol, reduceChainTerminalState(s, currentState), classifiedParseActionSingleReduce, chainLen, chainLen, classHits, time.Since(chainStart).Nanoseconds(), reduceChainStopShift)
			default:
				p.ambiguityProfile.recordReduceChainRun(chainStartState, tok.Symbol, reduceChainTerminalState(s, currentState), classifiedParseActionSingleReduce, chainLen, chainLen, classHits, time.Since(chainStart).Nanoseconds(), reduceChainStopDead)
			}
			return false
		}
	}
	if perfCountersEnabled {
		perfRecordReduceChainHintLimit()
	}
	return p.chainSingleReduceActionsClassifiedProfiledDefault(s, tok, anyReduced, nodeCount, arena, entryScratch, gssScratch, tmpEntries, deferParentLinks, trackChildErrors)
}

func (p *Parser) chainSingleReduceActionsClassifiedProfiledDefault(s *glrStack, tok Token, anyReduced *bool, nodeCount *int, arena *nodeArena, entryScratch *glrEntryScratch, gssScratch *gssScratch, tmpEntries *[]stackEntry, deferParentLinks bool, trackChildErrors *bool) bool {
	const maxInlineReduceChain = 256
	actions := p.classifiedActions
	chainLen := 0
	classHits := 0
	chainStartState := s.top().state
	chainStart := time.Now()
	var lastSig reduceChainSignature
	repeatedSigCount := 0
	for chainLen < maxInlineReduceChain {
		currentState := s.top().state
		currentDepth := s.depth()
		actionIdx := p.lookupActionIndex(currentState, tok.Symbol)
		if actionIdx == 0 || int(actionIdx) >= len(actions) {
			p.ambiguityProfile.recordReduceChainRun(chainStartState, tok.Symbol, currentState, classifiedParseActionNoAction, chainLen, chainLen, classHits, time.Since(chainStart).Nanoseconds(), reduceChainStopNoAction)
			return false
		}
		classHits++

		classified := &actions[actionIdx]
		switch classified.class {
		case classifiedParseActionSingleReduce:
			next := classified.action
			var repeated bool
			repeatedSigCount, repeated = noteRepeatedReduceChainAction(&lastSig, repeatedSigCount, currentState, currentDepth, next)
			if repeated {
				if p != nil && p.glrTrace {
					fmt.Printf("      -> REDUCE-CHAIN CYCLE state=%d depth=%d sym=%d prod=%d count=%d\n",
						currentState, currentDepth, next.Symbol, next.ProductionID, repeatedSigCount)
				}
				p.ambiguityProfile.recordReduceChainRun(chainStartState, tok.Symbol, currentState, classifiedParseActionSingleReduce, chainLen, chainLen, classHits, time.Since(chainStart).Nanoseconds(), reduceChainStopCycle)
				return true
			}
			chainLen++
			if perfCountersEnabled {
				perfRecordReduceChainStep(chainLen)
			}
			reduceStart := time.Now()
			p.applyReduceActionDispatch(s, next, tok, anyReduced, nodeCount, arena, entryScratch, gssScratch, tmpEntries, deferParentLinks, trackChildErrors)
			p.ambiguityProfile.recordReduceChainStep(currentState, tok.Symbol, next, chainLen, time.Since(reduceStart).Nanoseconds())
			if s.dead || s.accepted || s.shifted {
				switch {
				case s.accepted:
					p.ambiguityProfile.recordReduceChainRun(chainStartState, tok.Symbol, reduceChainTerminalState(s, currentState), classifiedParseActionSingleReduce, chainLen, chainLen, classHits, time.Since(chainStart).Nanoseconds(), reduceChainStopAccept)
				case s.shifted:
					p.ambiguityProfile.recordReduceChainRun(chainStartState, tok.Symbol, reduceChainTerminalState(s, currentState), classifiedParseActionSingleReduce, chainLen, chainLen, classHits, time.Since(chainStart).Nanoseconds(), reduceChainStopShift)
				default:
					p.ambiguityProfile.recordReduceChainRun(chainStartState, tok.Symbol, reduceChainTerminalState(s, currentState), classifiedParseActionSingleReduce, chainLen, chainLen, classHits, time.Since(chainStart).Nanoseconds(), reduceChainStopDead)
				}
				return false
			}
		case classifiedParseActionSingleShift:
			if perfCountersEnabled {
				perfRecordReduceChainBreakShift()
			}
			p.ambiguityProfile.recordReduceChainRun(chainStartState, tok.Symbol, currentState, classified.class, chainLen, chainLen, classHits, time.Since(chainStart).Nanoseconds(), reduceChainStopShift)
			return false
		case classifiedParseActionSingleAccept:
			if perfCountersEnabled {
				perfRecordReduceChainBreakAccept()
			}
			p.ambiguityProfile.recordReduceChainRun(chainStartState, tok.Symbol, currentState, classified.class, chainLen, chainLen, classHits, time.Since(chainStart).Nanoseconds(), reduceChainStopAccept)
			return false
		default:
			if perfCountersEnabled {
				perfRecordReduceChainBreakMulti()
			}
			p.ambiguityProfile.recordReduceChainRun(chainStartState, tok.Symbol, currentState, classified.class, chainLen, chainLen, classHits, time.Since(chainStart).Nanoseconds(), reduceChainStopMulti)
			return false
		}
	}
	p.ambiguityProfile.recordReduceChainRun(chainStartState, tok.Symbol, reduceChainTerminalState(s, chainStartState), classifiedParseActionSingleReduce, chainLen, chainLen, classHits, time.Since(chainStart).Nanoseconds(), reduceChainStopLimit)
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
		p.applyShiftAction(s, act, tok, nodeCount, arena, entryScratch, gssScratch, trackChildErrors)

	case ParseActionReduce:
		p.applyReduceActionDispatch(s, act, tok, anyReduced, nodeCount, arena, entryScratch, gssScratch, tmpEntries, deferParentLinks, trackChildErrors)

	case ParseActionAccept:
		p.applyAcceptAction(s)

	case ParseActionRecover:
		p.applyRecoverAction(s, act, tok, nodeCount, arena, entryScratch, gssScratch, trackChildErrors)
	}
}

func (p *Parser) applyShiftAction(s *glrStack, act ParseAction, tok Token, nodeCount *int, arena *nodeArena, entryScratch *glrEntryScratch, gssScratch *gssScratch, trackChildErrors *bool) {
	named := p.isNamedSymbol(tok.Symbol)
	currentState := s.top().state
	targetState := extraShiftTargetState(currentState, act)
	if p.useCompactNoTreeShiftLeaf() && !p.shiftTokenIsMissingError(tok) {
		extra := act.Extra
		if cp, ok := p.currentExternalNoTreeLeafCheckpointRef(arena, tok); ok {
			leaf := newCompactCheckpointLeafInArena(arena, tok.Symbol, named, tok.StartByte, tok.EndByte, cp)
			leaf.setExtra(extra)
			leaf.preGotoState = currentState
			leaf.parseState = targetState
			p.pushStackCompactCheckpointLeaf(s, targetState, leaf, entryScratch, gssScratch)
		} else {
			leaf := newNoTreeLeafNodeInArena(arena, tok.Symbol, named,
				tok.StartByte, tok.EndByte, tok.StartPoint, tok.EndPoint)
			leaf.setExtra(extra)
			leaf.preGotoState = currentState
			leaf.parseState = targetState
			p.pushStackNoTreeNode(s, targetState, leaf, entryScratch, gssScratch)
		}
		if extra && perfCountersEnabled {
			perfRecordExtraNode()
		}
	} else if p.canCompactFullShiftLeaf(act, tok) {
		leaf := newCompactFullLeafInArena(arena, tok.Symbol, named,
			tok.StartByte, tok.EndByte, tok.StartPoint, tok.EndPoint)
		if cp, ok := p.currentExternalCompactFullLeafCheckpointRef(arena, tok); ok {
			leaf.checkpoint = cp
			leaf.hasCheckpoint = true
		}
		leaf.preGotoState = currentState
		leaf.parseState = targetState
		p.pushStackCompactFullLeaf(s, targetState, leaf, entryScratch, gssScratch)
	} else {
		// Phase 3: pre-allocation interning. Compute the candidate key
		// from primitives (token + act + state) so we can skip the arena
		// allocation entirely on hit. The previous post-allocation
		// variant paid hash+lookup overhead per shift without saving
		// the allocation, which net-regressed wall time on JS.
		isMissing := p.shiftTokenIsMissingError(tok)
		var flags nodeFlags
		if named {
			flags |= nodeFlagNamed
		}
		if act.Extra {
			flags |= nodeFlagExtra
		}
		if isMissing {
			flags |= nodeFlagMissing | nodeFlagHasError
		}
		substituteActive := internLeavesSubstituteEnabled || (p != nil && p.leafInternByLang)
		if substituteActive {
			key := internKey{
				symbol:       uint32(tok.Symbol),
				flags:        uint8(flags),
				startByte:    tok.StartByte,
				endByte:      tok.EndByte,
				parseState:   targetState,
				preGotoState: currentState,
			}
			if canonical := lookupCanonicalLeafKey(arena, key); canonical != nil {
				if internLeavesObserveEnabled {
					arena.internShiftLeafObserved++
				}
				if isMissing && trackChildErrors != nil {
					*trackChildErrors = true
				}
				if act.Extra && perfCountersEnabled {
					perfRecordExtraNode()
				}
				// External-scanner checkpoint: the canonical leaf was
				// the first one to hit this exact (sym, span, state)
				// tuple, so its checkpoint snapshot is by construction
				// the right one to apply here too. Skip the re-record.
				p.pushStackNode(s, targetState, canonical, entryScratch, gssScratch)
				s.shifted = true
				*nodeCount++
				if p != nil && p.glrTrace {
					fmt.Printf("      -> SHIFT[intern-hit] new_state=%d depth=%d\n", targetState, s.depth())
				}
				return
			}
			// Miss: fall through to the regular allocation path; store
			// the resulting leaf below before pushing.
		}
		leaf := newLeafNodeInArena(arena, tok.Symbol, named,
			tok.StartByte, tok.EndByte, tok.StartPoint, tok.EndPoint)
		if isMissing {
			leaf.setMissing(true)
			leaf.setHasError(true)
			if trackChildErrors != nil {
				*trackChildErrors = true
			}
		}
		leaf.setExtra(act.Extra)
		if leaf.isExtra() && perfCountersEnabled {
			perfRecordExtraNode()
		}
		leaf.preGotoState = currentState
		leaf.parseState = targetState
		p.recordCurrentExternalLeafCheckpoint(leaf, tok)
		if internLeavesObserveEnabled {
			arena.internShiftLeafObserved++
			if !internLeavesSubstituteEnabled {
				observeLeafInternFull(arena, leaf)
			}
		}
		if substituteActive {
			storeCanonicalLeaf(arena, leaf)
		}
		p.pushStackNode(s, targetState, leaf, entryScratch, gssScratch)
	}
	s.shifted = true
	*nodeCount++
	if p != nil && p.glrTrace {
		fmt.Printf("      -> SHIFT new_state=%d depth=%d\n", targetState, s.depth())
	}
}

func (p *Parser) applyReduceActionDispatch(s *glrStack, act ParseAction, tok Token, anyReduced *bool, nodeCount *int, arena *nodeArena, entryScratch *glrEntryScratch, gssScratch *gssScratch, tmpEntries *[]stackEntry, deferParentLinks bool, trackChildErrors *bool) {
	entries := s.entries
	borrowed := false
	if entries == nil {
		if !s.cacheEntries && s.gss.head != nil {
			tmp := []stackEntry(nil)
			if tmpEntries != nil {
				tmp = *tmpEntries
			}
			if p != nil && p.reduceScratch != nil && p.reduceScratch.transientParents != nil {
				p.applyReduceActionFromGSSTransientParents(s, act, tok, anyReduced, nodeCount, arena, entryScratch, gssScratch, tmpEntries, tmp, deferParentLinks, trackChildErrors != nil && *trackChildErrors)
			} else {
				p.applyReduceActionFromGSS(s, act, tok, anyReduced, nodeCount, arena, entryScratch, gssScratch, tmpEntries, tmp, deferParentLinks, trackChildErrors != nil && *trackChildErrors)
			}
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
	if p != nil && p.reduceScratch != nil && p.reduceScratch.transientParents != nil {
		p.applyReduceActionTransientParents(s, act, tok, anyReduced, nodeCount, arena, entryScratch, gssScratch, entries, deferParentLinks, trackChildErrors != nil && *trackChildErrors)
	} else {
		p.applyReduceAction(s, act, tok, anyReduced, nodeCount, arena, entryScratch, gssScratch, entries, deferParentLinks, trackChildErrors != nil && *trackChildErrors)
	}
	if borrowed && tmpEntries != nil {
		*tmpEntries = entries[:0]
	}
	if p != nil && p.glrTrace && s != nil && !s.dead {
		fmt.Printf("      -> REDUCE top_state=%d depth=%d\n", s.top().state, s.depth())
	}
}

func (p *Parser) applyAcceptAction(s *glrStack) {
	s.accepted = true
	if p != nil && p.glrTrace {
		fmt.Printf("      -> ACCEPT\n")
	}
}

func (p *Parser) applyRecoverAction(s *glrStack, act ParseAction, tok Token, nodeCount *int, arena *nodeArena, entryScratch *glrEntryScratch, gssScratch *gssScratch, trackChildErrors *bool) {
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

func (p *Parser) pushStackEntry(s *glrStack, entry stackEntry, entryScratch *glrEntryScratch, gssScratch *gssScratch) {
	s.pushEntry(entry, entryScratch, gssScratch)
	if !s.recoverabilityKnown {
		return
	}
	if !s.mayRecover && p.stateCanRecover(entry.state) {
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
		if stackEntryHasNode(n.entry) && !stackEntryNodeIsExtra(n.entry) {
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

func reduceWindowRangeFromGSS(s *glrStack, childCount int, buf []stackEntry) ([]stackEntry, reduceRange, bool) {
	entries, topState, ok := reduceWindowFromGSS(s, childCount, buf)
	if !ok {
		return entries, reduceRange{}, false
	}
	return entries, reduceRange{
		start:      0,
		reducedEnd: reducedEndBeforeTrailingExtras(entries),
		actualEnd:  len(entries),
		topState:   topState,
	}, true
}

func reducedEndBeforeTrailingExtras(entries []stackEntry) int {
	reducedEnd := len(entries)
	for i := len(entries) - 1; i >= 0; i-- {
		if !stackEntryHasNode(entries[i]) || !stackEntryNodeIsExtra(entries[i]) {
			break
		}
		reducedEnd--
	}
	return reducedEnd
}

func releaseReduceWindowEntries(tmpEntries *[]stackEntry, entries []stackEntry) {
	if tmpEntries != nil {
		*tmpEntries = entries[:0]
	}
}

func truncateStackForReduce(s *glrStack, targetDepth int) bool {
	if targetDepth < 0 || !s.truncateBeforePush(targetDepth) {
		s.dead = true
		return false
	}
	return true
}

func markReduceApplied(s *glrStack, act ParseAction, anyReduced *bool) {
	s.score += int(act.DynamicPrecedence)
	*anyReduced = true
}

func (p *Parser) tryFastVisibleReduceActionFromGSS(s *glrStack, act ParseAction, tok Token, anyReduced *bool, nodeCount *int, arena *nodeArena, entryScratch *glrEntryScratch, gssScratch *gssScratch, tmpEntries *[]stackEntry, deferParentLinks bool, trackChildErrors bool) bool {
	if p == nil || s == nil || s.gss.head == nil || p.language == nil {
		return false
	}
	childCount := int(act.ChildCount)
	if childCount <= 1 || childCount > 8 {
		return false
	}
	if len(p.reduceAliasSequence(act.ProductionID)) != 0 || p.reduceProductionHasEffectiveFields(childCount, act.ProductionID, arena) {
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

	timing := p.reduceTiming
	childStart := time.Time{}
	if timing != nil {
		childStart = time.Now()
	}
	var childBuf [8]*Node
	symbolMeta := p.language.SymbolMetadata
	n := s.gss.head
	for i := childCount - 1; i >= 0; i-- {
		if n == nil {
			return false
		}
		child := stackEntryNode(n.entry)
		if child == nil || child.isExtra() {
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

	children := arena.allocNodeSliceNoClear(childCount)
	arena.recordReduceChildSliceFastGSS(childCount)
	if perfCountersEnabled {
		perfRecordReduceChildrenFastGSS(childCount)
	}
	copy(children, childBuf[:childCount])
	if timing != nil {
		timing.reduceChildBuildNanos += time.Since(childStart).Nanoseconds()
	}
	named := p.isNamedSymbol(act.Symbol)
	var parent *Node
	parentStart := time.Time{}
	if timing != nil {
		parentStart = time.Now()
	}
	if deferParentLinks {
		parent = newParentNodeInArenaNoLinksWithFieldSources(arena, act.Symbol, named, children, nil, nil, act.ProductionID, trackChildErrors)
	} else {
		parent = newParentNodeInArenaWithFieldSources(arena, act.Symbol, named, children, nil, nil, act.ProductionID)
	}
	p.recordReductionParentConstructed(arena, parent, act.Symbol, len(children), nil, nil, reduceChildPathFastGSS)
	if timing != nil {
		timing.reduceParentBuildNanos += time.Since(parentStart).Nanoseconds()
	}
	*nodeCount++

	pushStart := time.Time{}
	if timing != nil {
		pushStart = time.Now()
	}
	gotoState := p.lookupGoto(topState, act.Symbol)
	targetState := topState
	if gotoState != 0 {
		targetState = gotoState
	}
	if tok.NoLookahead && targetState == topState {
		parent.setExtra(true)
	}
	parent.preGotoState = topState
	parent.parseState = targetState
	if !s.truncateBeforePush(targetDepth) {
		s.dead = true
		if tmpEntries != nil {
			*tmpEntries = (*tmpEntries)[:0]
		}
		return true
	}
	p.pushStackNode(s, targetState, parent, entryScratch, gssScratch)
	if timing != nil {
		timing.reduceStackPushNanos += time.Since(pushStart).Nanoseconds()
	}
	s.score += int(act.DynamicPrecedence)
	*anyReduced = true
	if tmpEntries != nil {
		*tmpEntries = (*tmpEntries)[:0]
	}
	return true
}

func (p *Parser) applyNoTreeReduceActionFromGSS(s *glrStack, act ParseAction, tok Token, anyReduced *bool, nodeCount *int, arena *nodeArena, entryScratch *glrEntryScratch, gssScratch *gssScratch, tmpEntries *[]stackEntry, tmp []stackEntry, trackChildErrors bool) {
	timing := p.reduceTiming
	rangeStart := time.Time{}
	if timing != nil {
		rangeStart = time.Now()
	}
	windowEntries, window, ok := reduceWindowRangeFromGSS(s, int(act.ChildCount), tmp)
	if timing != nil {
		timing.reduceRangeNanos += time.Since(rangeStart).Nanoseconds()
	}
	if !ok {
		s.dead = true
		releaseReduceWindowEntries(tmpEntries, windowEntries)
		return
	}

	noTreeStart := time.Time{}
	if timing != nil {
		noTreeStart = time.Now()
	}
	targetDepth := s.depth() - window.actualEnd
	if !truncateStackForReduce(s, targetDepth) {
		if timing != nil {
			timing.reduceNoTreeBuildNanos += time.Since(noTreeStart).Nanoseconds()
		}
		releaseReduceWindowEntries(tmpEntries, windowEntries)
		return
	}
	p.pushNoTreeReduceNode(s, act, tok, arena, entryScratch, gssScratch, windowEntries, window.start, window.reducedEnd, window.reducedEnd, window.actualEnd, window.topState, nodeCount, trackChildErrors)
	if timing != nil {
		timing.reduceNoTreeBuildNanos += time.Since(noTreeStart).Nanoseconds()
	}
	markReduceApplied(s, act, anyReduced)
	releaseReduceWindowEntries(tmpEntries, windowEntries)
}

func (p *Parser) applyReduceActionFromGSS(s *glrStack, act ParseAction, tok Token, anyReduced *bool, nodeCount *int, arena *nodeArena, entryScratch *glrEntryScratch, gssScratch *gssScratch, tmpEntries *[]stackEntry, tmp []stackEntry, deferParentLinks bool, trackChildErrors bool) {
	if p != nil && p.noTreeBenchmarkOnly {
		p.applyNoTreeReduceActionFromGSS(s, act, tok, anyReduced, nodeCount, arena, entryScratch, gssScratch, tmpEntries, tmp, trackChildErrors)
		return
	}
	if p.tryFastVisibleReduceActionFromGSS(s, act, tok, anyReduced, nodeCount, arena, entryScratch, gssScratch, tmpEntries, deferParentLinks, trackChildErrors) {
		return
	}
	timing := p.reduceTiming
	childCount := int(act.ChildCount)
	rangeStart := time.Time{}
	if timing != nil {
		rangeStart = time.Now()
	}
	windowEntries, window, ok := reduceWindowRangeFromGSS(s, childCount, tmp)
	if timing != nil {
		timing.reduceRangeNanos += time.Since(rangeStart).Nanoseconds()
	}
	if !ok {
		s.dead = true
		releaseReduceWindowEntries(tmpEntries, windowEntries)
		return
	}

	targetDepth := s.depth() - window.actualEnd
	if pendingFullParents := p.usePendingFullParents(); pendingFullParents {
		pendingStart := time.Time{}
		if timing != nil {
			pendingStart = time.Now()
		}
		if child, ok := p.collapsibleRawUnarySelfReductionEntry(act, tok, arena, windowEntries, window.start, window.reducedEnd); ok {
			if timing != nil {
				timing.reducePendingParentNanos += time.Since(pendingStart).Nanoseconds()
			}
			if !truncateStackForReduce(s, targetDepth) {
				releaseReduceWindowEntries(tmpEntries, windowEntries)
				return
			}
			p.pushCollapsedUnaryReduceEntry(s, act, tok, child, entryScratch, gssScratch, windowEntries, window.reducedEnd, window.actualEnd, window.topState)
			markReduceApplied(s, act, anyReduced)
			releaseReduceWindowEntries(tmpEntries, windowEntries)
			return
		}
		if p.tryPushPendingNoFieldParent(s, act, tok, anyReduced, nodeCount, arena, entryScratch, gssScratch, windowEntries, window.start, window.reducedEnd, window.actualEnd, window.topState, targetDepth) {
			if timing != nil {
				timing.reducePendingParentNanos += time.Since(pendingStart).Nanoseconds()
			}
			releaseReduceWindowEntries(tmpEntries, windowEntries)
			return
		}
		materializePendingPayloadEntries(p, windowEntries, window.start, window.actualEnd, arena)
		if timing != nil {
			timing.reducePendingParentNanos += time.Since(pendingStart).Nanoseconds()
		}
	}

	if child := p.collapsibleRawUnarySelfReduction(act, tok, arena, windowEntries, window.start, window.reducedEnd); child != nil {
		if !truncateStackForReduce(s, targetDepth) {
			releaseReduceWindowEntries(tmpEntries, windowEntries)
			return
		}
		p.pushCollapsedUnaryReduceNode(s, act, tok, child, entryScratch, gssScratch, windowEntries, window.reducedEnd, window.actualEnd, window.topState)
		markReduceApplied(s, act, anyReduced)
		releaseReduceWindowEntries(tmpEntries, windowEntries)
		return
	}

	childStart := time.Time{}
	if timing != nil {
		childStart = time.Now()
	}
	children, fieldIDs, fieldSources, childPath := p.buildReduceChildrenWithPath(windowEntries, window.start, window.reducedEnd, childCount, act.Symbol, act.ProductionID, arena)
	if timing != nil {
		timing.reduceChildBuildNanos += time.Since(childStart).Nanoseconds()
	}

	if !truncateStackForReduce(s, targetDepth) {
		releaseReduceWindowEntries(tmpEntries, windowEntries)
		return
	}

	if child := p.collapsibleUnarySelfReduction(act, tok, arena, windowEntries, window.start, window.reducedEnd, children, fieldIDs); child != nil {
		p.pushCollapsedUnaryReduceNode(s, act, tok, child, entryScratch, gssScratch, windowEntries, window.reducedEnd, window.actualEnd, window.topState)
		markReduceApplied(s, act, anyReduced)
		releaseReduceWindowEntries(tmpEntries, windowEntries)
		return
	}

	named := p.isNamedSymbol(act.Symbol)
	var parent *Node
	parentStart := time.Time{}
	if timing != nil {
		parentStart = time.Now()
	}
	if deferParentLinks {
		parent = newParentNodeInArenaNoLinksWithFieldSources(arena, act.Symbol, named, children, fieldIDs, fieldSources, act.ProductionID, trackChildErrors)
	} else {
		parent = newParentNodeInArenaWithFieldSources(arena, act.Symbol, named, children, fieldIDs, fieldSources, act.ProductionID)
	}
	p.recordReductionParentConstructed(arena, parent, act.Symbol, len(children), fieldIDs, fieldSources, childPath)
	if timing != nil {
		timing.reduceParentBuildNanos += time.Since(parentStart).Nanoseconds()
	}
	spanStart := time.Time{}
	if timing != nil {
		spanStart = time.Now()
	}
	shouldUseRawSpan := shouldUseRawSpanForReduction(act.Symbol, children, p.language.SymbolMetadata, p.forceRawSpanAll, p.forceRawSpanTable)
	if shouldUseRawSpan && window.reducedEnd > window.start {
		span := computeReduceRawSpan(windowEntries, window.start, window.reducedEnd)
		if int(act.Symbol) < len(p.forceRawSpanTable) && p.forceRawSpanTable[act.Symbol] && window.actualEnd > window.reducedEnd {
			extendRawSpanToTrailingEntries(&span, windowEntries, window.reducedEnd, window.actualEnd)
		}
		parent.startByte = span.startByte
		parent.endByte = span.endByte
		parent.startPoint = span.startPoint
		parent.endPoint = span.endPoint
	}
	// Extend parent span to cover invisible children dropped by buildReduceChildren.
	if reduceChildPathMayDropSpan(childPath) {
		extendParentSpanToWindow(parent, windowEntries, window.start, window.reducedEnd, p.language.SymbolMetadata, p.spanExtendingInvisibleSymbols, p.nonSpanExtendingInvisibleSymbols)
	}
	if timing != nil {
		timing.reduceSpanNanos += time.Since(spanStart).Nanoseconds()
	}
	*nodeCount++

	pushStart := time.Time{}
	if timing != nil {
		pushStart = time.Now()
	}
	gotoState := p.lookupGoto(window.topState, act.Symbol)
	targetState := window.topState
	if gotoState != 0 {
		targetState = gotoState
	}
	if tok.NoLookahead && targetState == window.topState {
		parent.setExtra(true)
	}
	parent.preGotoState = window.topState
	parent.parseState = targetState
	p.pushStackNode(s, targetState, parent, entryScratch, gssScratch)
	for i := window.reducedEnd; i < window.actualEnd; i++ {
		extra := stackEntryNode(windowEntries[i])
		if extra == nil {
			continue
		}
		extra.parseState = targetState
		nodeBumpEquivVersion(extra)
		p.pushStackNode(s, targetState, extra, entryScratch, gssScratch)
	}
	if timing != nil {
		timing.reduceStackPushNanos += time.Since(pushStart).Nanoseconds()
	}

	markReduceApplied(s, act, anyReduced)
	releaseReduceWindowEntries(tmpEntries, windowEntries)
}

func (p *Parser) tryFastVisibleReduceActionFromGSSTransientParents(s *glrStack, act ParseAction, tok Token, anyReduced *bool, nodeCount *int, arena *nodeArena, entryScratch *glrEntryScratch, gssScratch *gssScratch, tmpEntries *[]stackEntry, deferParentLinks bool, trackChildErrors bool) bool {
	if p == nil || s == nil || s.gss.head == nil || p.language == nil {
		return false
	}
	childCount := int(act.ChildCount)
	if childCount <= 1 || childCount > 8 {
		return false
	}
	if len(p.reduceAliasSequence(act.ProductionID)) != 0 || p.reduceProductionHasEffectiveFields(childCount, act.ProductionID, arena) {
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

	timing := p.reduceTiming
	childStart := time.Time{}
	if timing != nil {
		childStart = time.Now()
	}
	var childBuf [8]*Node
	symbolMeta := p.language.SymbolMetadata
	n := s.gss.head
	for i := childCount - 1; i >= 0; i-- {
		if n == nil {
			return false
		}
		child := stackEntryNode(n.entry)
		if child == nil || child.isExtra() {
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

	children := arena.allocNodeSliceNoClear(childCount)
	arena.recordReduceChildSliceFastGSS(childCount)
	if perfCountersEnabled {
		perfRecordReduceChildrenFastGSS(childCount)
	}
	copy(children, childBuf[:childCount])
	if timing != nil {
		timing.reduceChildBuildNanos += time.Since(childStart).Nanoseconds()
	}
	named := p.isNamedSymbol(act.Symbol)
	parentStart := time.Time{}
	if timing != nil {
		parentStart = time.Now()
	}
	parent := p.newReduceParentNode(arena, act.Symbol, named, children, nil, nil, act.ProductionID, deferParentLinks, trackChildErrors)
	p.recordReductionParentConstructed(arena, parent, act.Symbol, len(children), nil, nil, reduceChildPathFastGSS)
	if timing != nil {
		timing.reduceParentBuildNanos += time.Since(parentStart).Nanoseconds()
	}
	*nodeCount++

	pushStart := time.Time{}
	if timing != nil {
		pushStart = time.Now()
	}
	gotoState := p.lookupGoto(topState, act.Symbol)
	targetState := topState
	if gotoState != 0 {
		targetState = gotoState
	}
	if tok.NoLookahead && targetState == topState {
		parent.setExtra(true)
	}
	parent.preGotoState = topState
	parent.parseState = targetState
	if !s.truncateBeforePush(targetDepth) {
		s.dead = true
		if tmpEntries != nil {
			*tmpEntries = (*tmpEntries)[:0]
		}
		return true
	}
	p.pushStackNode(s, targetState, parent, entryScratch, gssScratch)
	if timing != nil {
		timing.reduceStackPushNanos += time.Since(pushStart).Nanoseconds()
	}
	s.score += int(act.DynamicPrecedence)
	*anyReduced = true
	if tmpEntries != nil {
		*tmpEntries = (*tmpEntries)[:0]
	}
	return true
}

func (p *Parser) applyReduceActionFromGSSTransientParents(s *glrStack, act ParseAction, tok Token, anyReduced *bool, nodeCount *int, arena *nodeArena, entryScratch *glrEntryScratch, gssScratch *gssScratch, tmpEntries *[]stackEntry, tmp []stackEntry, deferParentLinks bool, trackChildErrors bool) {
	if p.tryFastVisibleReduceActionFromGSSTransientParents(s, act, tok, anyReduced, nodeCount, arena, entryScratch, gssScratch, tmpEntries, deferParentLinks, trackChildErrors) {
		return
	}
	timing := p.reduceTiming
	childCount := int(act.ChildCount)
	rangeStart := time.Time{}
	if timing != nil {
		rangeStart = time.Now()
	}
	windowEntries, topState, ok := reduceWindowFromGSS(s, childCount, tmp)
	if timing != nil {
		timing.reduceRangeNanos += time.Since(rangeStart).Nanoseconds()
	}
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
		if !stackEntryHasNode(windowEntries[i]) || !stackEntryNodeIsExtra(windowEntries[i]) {
			break
		}
		reducedEnd--
	}
	if p.usePendingFullParents() {
		pendingStart := time.Time{}
		if timing != nil {
			pendingStart = time.Now()
		}
		if child, ok := p.collapsibleRawUnarySelfReductionEntry(act, tok, arena, windowEntries, 0, reducedEnd); ok {
			if timing != nil {
				timing.reducePendingParentNanos += time.Since(pendingStart).Nanoseconds()
			}
			targetDepth := s.depth() - actualEnd
			if targetDepth < 0 || !s.truncateBeforePush(targetDepth) {
				s.dead = true
				if tmpEntries != nil {
					*tmpEntries = windowEntries[:0]
				}
				return
			}
			p.pushCollapsedUnaryReduceEntry(s, act, tok, child, entryScratch, gssScratch, windowEntries, reducedEnd, actualEnd, topState)
			s.score += int(act.DynamicPrecedence)
			*anyReduced = true
			if tmpEntries != nil {
				*tmpEntries = windowEntries[:0]
			}
			return
		}
	}
	if p.usePendingFullParents() {
		pendingStart := time.Time{}
		if timing != nil {
			pendingStart = time.Now()
		}
		if p.tryPushPendingNoFieldParent(s, act, tok, anyReduced, nodeCount, arena, entryScratch, gssScratch, windowEntries, 0, reducedEnd, actualEnd, topState, s.depth()-actualEnd) {
			if timing != nil {
				timing.reducePendingParentNanos += time.Since(pendingStart).Nanoseconds()
			}
			if tmpEntries != nil {
				*tmpEntries = windowEntries[:0]
			}
			return
		}
		materializePendingPayloadEntries(p, windowEntries, 0, actualEnd, arena)
		if timing != nil {
			timing.reducePendingParentNanos += time.Since(pendingStart).Nanoseconds()
		}
	}

	if child := p.collapsibleRawUnarySelfReduction(act, tok, arena, windowEntries, 0, reducedEnd); child != nil {
		targetDepth := s.depth() - actualEnd
		if targetDepth < 0 || !s.truncateBeforePush(targetDepth) {
			s.dead = true
			if tmpEntries != nil {
				*tmpEntries = windowEntries[:0]
			}
			return
		}
		p.pushCollapsedUnaryReduceNode(s, act, tok, child, entryScratch, gssScratch, windowEntries, reducedEnd, actualEnd, topState)
		s.score += int(act.DynamicPrecedence)
		*anyReduced = true
		if tmpEntries != nil {
			*tmpEntries = windowEntries[:0]
		}
		return
	}

	childStart := time.Time{}
	if timing != nil {
		childStart = time.Now()
	}
	children, fieldIDs, fieldSources, childPath := p.buildReduceChildrenWithPath(windowEntries, 0, reducedEnd, childCount, act.Symbol, act.ProductionID, arena)
	if timing != nil {
		timing.reduceChildBuildNanos += time.Since(childStart).Nanoseconds()
	}

	targetDepth := s.depth() - actualEnd
	if targetDepth < 0 || !s.truncateBeforePush(targetDepth) {
		s.dead = true
		if tmpEntries != nil {
			*tmpEntries = windowEntries[:0]
		}
		return
	}

	if child := p.collapsibleUnarySelfReduction(act, tok, arena, windowEntries, 0, reducedEnd, children, fieldIDs); child != nil {
		p.pushCollapsedUnaryReduceNode(s, act, tok, child, entryScratch, gssScratch, windowEntries, reducedEnd, actualEnd, topState)
		s.score += int(act.DynamicPrecedence)
		*anyReduced = true
		if tmpEntries != nil {
			*tmpEntries = windowEntries[:0]
		}
		return
	}

	named := p.isNamedSymbol(act.Symbol)
	parentStart := time.Time{}
	if timing != nil {
		parentStart = time.Now()
	}
	parent := p.newReduceParentNode(arena, act.Symbol, named, children, fieldIDs, fieldSources, act.ProductionID, deferParentLinks, trackChildErrors)
	p.recordReductionParentConstructed(arena, parent, act.Symbol, len(children), fieldIDs, fieldSources, childPath)
	if timing != nil {
		timing.reduceParentBuildNanos += time.Since(parentStart).Nanoseconds()
	}
	spanStart := time.Time{}
	if timing != nil {
		spanStart = time.Now()
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
	if reduceChildPathMayDropSpan(childPath) {
		extendParentSpanToWindow(parent, windowEntries, 0, reducedEnd, p.language.SymbolMetadata, p.spanExtendingInvisibleSymbols, p.nonSpanExtendingInvisibleSymbols)
	}
	if timing != nil {
		timing.reduceSpanNanos += time.Since(spanStart).Nanoseconds()
	}
	*nodeCount++

	pushStart := time.Time{}
	if timing != nil {
		pushStart = time.Now()
	}
	gotoState := p.lookupGoto(topState, act.Symbol)
	targetState := topState
	if gotoState != 0 {
		targetState = gotoState
	}
	if tok.NoLookahead && targetState == topState {
		parent.setExtra(true)
	}
	parent.preGotoState = topState
	parent.parseState = targetState
	p.pushStackNode(s, targetState, parent, entryScratch, gssScratch)
	for i := reducedEnd; i < actualEnd; i++ {
		extra := stackEntryNode(windowEntries[i])
		if extra == nil {
			continue
		}
		extra.parseState = targetState
		nodeBumpEquivVersion(extra)
		p.pushStackNode(s, targetState, extra, entryScratch, gssScratch)
	}
	if timing != nil {
		timing.reduceStackPushNanos += time.Since(pushStart).Nanoseconds()
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
		if n := stackEntryNode(entries[start]); n != nil && !n.isExtra() {
			nonExtraFound++
		}
	}
	if nonExtraFound < childCount {
		return reduceRange{}, false
	}

	actualEnd := len(entries)
	reducedEnd := actualEnd
	for i := actualEnd - 1; i >= start; i-- {
		n := stackEntryNode(entries[i])
		if n == nil || !n.isExtra() {
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

func computeReduceRangePayload(entries []stackEntry, childCount int) (reduceRange, bool) {
	start := len(entries)
	nonExtraFound := 0
	for nonExtraFound < childCount && start > 1 {
		start--
		if stackEntryHasNode(entries[start]) && !stackEntryNodeIsExtra(entries[start]) {
			nonExtraFound++
		}
	}
	if nonExtraFound < childCount {
		return reduceRange{}, false
	}

	actualEnd := len(entries)
	reducedEnd := actualEnd
	for i := actualEnd - 1; i >= start; i-- {
		if !stackEntryHasNode(entries[i]) || !stackEntryNodeIsExtra(entries[i]) {
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

func computeReduceRangeForFullPayloads(entries []stackEntry, childCount int, payloads bool) (reduceRange, bool) {
	if payloads {
		return computeReduceRangePayload(entries, childCount)
	}
	return computeReduceRange(entries, childCount)
}

func materializePendingPayloadEntries(p *Parser, entries []stackEntry, start, end int, arena *nodeArena) {
	if end > len(entries) {
		end = len(entries)
	}
	rejectReason := pendingParentRejectUnknown
	rejectShape := pendingParentFieldRejectUnknown
	recordFieldRejectDetails := false
	if arena != nil {
		rejectReason = arena.pendingParentLastRejectReason
		recordFieldRejectDetails = arena.breakdownEnabled && rejectReason == pendingParentRejectFields
		if recordFieldRejectDetails {
			rejectShape = arena.pendingParentLastFieldRejectShape
		}
	}
	prevRejectReason := pendingParentRejectUnknown
	prevRejectShape := pendingParentFieldRejectUnknown
	prevPayloadShape := pendingParentFieldRejectPayloadUnknown
	if arena != nil {
		prevRejectReason = arena.pendingParentActiveRejectReason
		prevRejectShape = arena.pendingParentActiveFieldRejectShape
		prevPayloadShape = arena.pendingParentActiveFieldPayloadShape
		arena.pendingParentActiveRejectReason = rejectReason
		arena.pendingParentActiveFieldRejectShape = rejectShape
		defer func() {
			arena.pendingParentActiveRejectReason = prevRejectReason
			arena.pendingParentActiveFieldRejectShape = prevRejectShape
			arena.pendingParentActiveFieldPayloadShape = prevPayloadShape
		}()
	}
	for i := start; i < end; i++ {
		if stackEntryCompactFullLeaf(entries[i]) == nil && stackEntryPendingParent(entries[i]) == nil {
			continue
		}
		if recordFieldRejectDetails {
			arena.pendingParentActiveFieldPayloadShape = p.pendingParentFieldRejectPayloadShape(entries[i], arena)
		}
		materializeStackEntryPayloadWithParser(p, arena, &entries[i], compactFullLeafMaterializeForParentReduce, pendingParentMaterializeForParentReduce)
	}
}

func (p *Parser) pendingParentFieldRejectPayloadShape(entry stackEntry, arena *nodeArena) pendingParentFieldRejectPayloadShape {
	if p == nil || p.language == nil || !stackEntryHasNode(entry) {
		return pendingParentFieldRejectPayloadUnknown
	}
	symbolMeta := p.language.SymbolMetadata
	if stackEntryVisibleForPending(entry, symbolMeta) {
		if parent := stackEntryPendingParent(entry); parent != nil {
			shape := classifyPendingParentVisiblePayloadShape(parent, arena)
			switch {
			case shape.containsCompactLeaf:
				return pendingParentFieldRejectPayloadVisibleCompactLeaf
			case shape.containsNestedPending:
				return pendingParentFieldRejectPayloadVisibleNestedPayload
			case shape.containsFieldedDesc:
				return pendingParentFieldRejectPayloadVisibleFieldedDescendant
			default:
				return pendingParentFieldRejectPayloadVisibleFinalLike
			}
		}
		return pendingParentFieldRejectPayloadVisible
	}
	if n := stackEntryNode(entry); hiddenTreeHasFieldIDs(n) {
		return pendingParentFieldRejectPayloadHiddenWithFields
	}
	switch pendingPlainHiddenVisibleDescendantCount(entry, arena, symbolMeta) {
	case 0:
		return pendingParentFieldRejectPayloadHiddenEmpty
	case 1:
		return pendingParentFieldRejectPayloadHiddenOne
	default:
		return pendingParentFieldRejectPayloadHiddenMany
	}
}

type pendingParentVisiblePayloadShape struct {
	containsCompactLeaf   bool
	containsNestedPending bool
	containsFieldedDesc   bool
}

func classifyPendingParentVisiblePayloadShape(parent *pendingParent, arena *nodeArena) pendingParentVisiblePayloadShape {
	var shape pendingParentVisiblePayloadShape
	collectPendingParentVisiblePayloadShape(parent, arena, &shape)
	return shape
}

func collectPendingParentVisiblePayloadShape(parent *pendingParent, arena *nodeArena, shape *pendingParentVisiblePayloadShape) {
	if parent == nil || shape == nil {
		return
	}
	for i := 0; i < parent.childEntryCount(); i++ {
		child := parent.childEntry(arena, i)
		if stackEntryCompactFullLeaf(child) != nil {
			shape.containsCompactLeaf = true
			continue
		}
		if nested := stackEntryPendingParent(child); nested != nil {
			shape.containsNestedPending = true
			collectPendingParentVisiblePayloadShape(nested, arena, shape)
			continue
		}
		if node := stackEntryNode(child); node != nil && nodeTreeHasFieldIDs(node) {
			shape.containsFieldedDesc = true
		}
	}
}

func nodeTreeHasFieldIDs(n *Node) bool {
	if n == nil {
		return false
	}
	if len(n.fieldIDs) != 0 {
		return true
	}
	for _, child := range n.children {
		if nodeTreeHasFieldIDs(child) {
			return true
		}
	}
	return false
}

func (p *Parser) tryPushPendingNoFieldParent(s *glrStack, act ParseAction, tok Token, anyReduced *bool, nodeCount *int, arena *nodeArena, entryScratch *glrEntryScratch, gssScratch *gssScratch, entries []stackEntry, start, reducedEnd, trailingEnd int, topState StateID, truncateDepth int) bool {
	if p == nil || !p.usePendingFullParents() || p.language == nil || s == nil {
		return false
	}
	if arena != nil {
		arena.pendingParentCandidates++
	}
	if act.ChildCount == 0 {
		arena.recordPendingParentRejected(pendingParentRejectEmpty)
		return false
	}
	if act.ChildCount > 32 {
		arena.recordPendingParentRejected(pendingParentRejectChildLimit)
		return false
	}
	if len(p.reduceAliasSequence(act.ProductionID)) != 0 {
		arena.recordPendingParentRejected(pendingParentRejectAlias)
		return false
	}
	if p.forceRawSpanAll || (int(act.Symbol) < len(p.forceRawSpanTable) && p.forceRawSpanTable[act.Symbol]) {
		arena.recordPendingParentRejected(pendingParentRejectRawSpan)
		return false
	}
	if p.reduceProductionHasEffectiveFields(int(act.ChildCount), act.ProductionID, arena) {
		rawFieldIDs, rawInherited := p.buildFieldIDs(int(act.ChildCount), act.ProductionID, arena)
		if p.tryPushPendingDirectFieldParent(s, act, tok, anyReduced, nodeCount, arena, entryScratch, gssScratch, entries, start, reducedEnd, trailingEnd, topState, truncateDepth, rawFieldIDs, rawInherited) {
			return true
		}
		if arena != nil && arena.breakdownEnabled {
			p.recordPendingFieldRejectShape(arena, act, entries, start, reducedEnd)
		}
		arena.recordPendingParentRejected(pendingParentRejectFields)
		return false
	}
	symbolMeta := p.language.SymbolMetadata
	parentVisible := symbolVisibleForPending(act.Symbol, symbolMeta)
	childCount := 0
	hasPayload := false
	hasError := false
	for i := start; i < reducedEnd; i++ {
		count, childHasPayload, childHasError, ok := pendingNoFieldChildCount(entries[i], arena, parentVisible, symbolMeta)
		if !ok {
			arena.recordPendingParentRejected(pendingParentRejectChild)
			return false
		}
		childCount += count
		hasPayload = hasPayload || childHasPayload
		hasError = hasError || childHasError
	}
	if childCount == 0 {
		if !hasPayload {
			arena.recordPendingParentRejected(pendingParentRejectEmpty)
			return false
		}
		if _, ok := pendingReduceWindowSpan(entries, start, reducedEnd); !ok {
			arena.recordPendingParentRejected(pendingParentRejectSpan)
			return false
		}
	}
	var startByte, endByte uint32
	var startPoint, endPoint Point
	if childCount > 0 {
		var first, last stackEntry
		if firstEntry, lastEntry, ok := pendingNoFieldChildEndpoints(entries, start, reducedEnd, arena, parentVisible, symbolMeta); ok {
			first = firstEntry
			last = lastEntry
		} else {
			arena.recordPendingParentRejected(pendingParentRejectSpan)
			return false
		}
		startByte = stackEntryNodeStartByte(first)
		endByte = stackEntryNodeEndByte(last)
		startPoint = stackEntryNodeStartPoint(first)
		endPoint = stackEntryNodeEndPoint(last)
	}
	if span, ok := pendingReduceWindowSpan(entries, start, reducedEnd); ok {
		startByte = span.startByte
		endByte = span.endByte
		startPoint = span.startPoint
		endPoint = span.endPoint
	}
	parent := newPendingParentShellInArena(
		arena,
		act.Symbol,
		p.isNamedSymbol(act.Symbol),
		act.ProductionID,
		childCount,
		startByte,
		endByte,
		startPoint,
		endPoint,
		hasError,
	)
	out := 0
	flattenedParents := 0
	flattenedChildRefs := 0
	parentChildren := parent.childRefs(arena)
	for i := start; i < reducedEnd; i++ {
		next, parents, refs := fillPendingNoFieldChildren(parentChildren, out, entries[i], arena, parentVisible, symbolMeta)
		out = next
		flattenedParents += parents
		flattenedChildRefs += refs
	}
	if out != childCount {
		arena.recordPendingParentRejected(pendingParentRejectFill)
		return false
	}
	if arena != nil {
		arena.pendingParentsFlattened += uint64(flattenedParents)
		arena.pendingChildRefsFlattened += uint64(flattenedChildRefs)
	}
	gotoState := p.lookupGoto(topState, act.Symbol)
	targetState := topState
	if gotoState != 0 {
		targetState = gotoState
	}
	if tok.NoLookahead && targetState == topState {
		parent.setExtra(true)
	}
	parent.preGotoState = topState
	parent.parseState = targetState
	if !s.truncateBeforePush(truncateDepth) {
		s.dead = true
		return true
	}
	p.pushStackPendingParent(s, targetState, parent, entryScratch, gssScratch)
	for i := reducedEnd; i < trailingEnd; i++ {
		extra, ok := retargetStackEntryPayload(entries[i], targetState)
		if !ok {
			continue
		}
		p.pushStackEntry(s, extra, entryScratch, gssScratch)
	}
	if nodeCount != nil {
		*nodeCount = *nodeCount + 1
	}
	s.score += int(act.DynamicPrecedence)
	if anyReduced != nil {
		*anyReduced = true
	}
	return true
}

func (p *Parser) tryPushPendingDirectFieldParent(s *glrStack, act ParseAction, tok Token, anyReduced *bool, nodeCount *int, arena *nodeArena, entryScratch *glrEntryScratch, gssScratch *gssScratch, entries []stackEntry, start, reducedEnd, trailingEnd int, topState StateID, truncateDepth int, rawFieldIDs []FieldID, rawInherited []bool) bool {
	if !p.pendingDirectFieldParentEligible(s, arena, rawFieldIDs, rawInherited) {
		return false
	}
	symbolMeta := p.language.SymbolMetadata
	if !symbolVisibleForPending(act.Symbol, symbolMeta) {
		return false
	}

	window, ok := scanPendingDirectFieldParentWindow(entries, start, reducedEnd, arena, symbolMeta)
	if !ok {
		return false
	}
	useDenseFieldEntries := window.skippedHiddenChild && !p.pendingDirectFieldParentFieldsRecomputable(act.ProductionID, window.childCount, entries, start, reducedEnd, rawFieldIDs, rawInherited, symbolMeta)
	startByte := stackEntryNodeStartByte(window.first)
	endByte := stackEntryNodeEndByte(window.last)
	startPoint := stackEntryNodeStartPoint(window.first)
	endPoint := stackEntryNodeEndPoint(window.last)
	if span, ok := pendingReduceWindowSpan(entries, start, reducedEnd); ok {
		startByte = span.startByte
		endByte = span.endByte
		startPoint = span.startPoint
		endPoint = span.endPoint
	}
	parent := newPendingParentShellWithEntrySlotsInArena(
		arena,
		act.Symbol,
		p.isNamedSymbol(act.Symbol),
		act.ProductionID,
		window.childCount,
		pendingDirectFieldParentEntrySlots(window.childCount, useDenseFieldEntries),
		startByte,
		endByte,
		startPoint,
		endPoint,
		window.hasError,
	)
	if useDenseFieldEntries {
		parent.setHasFieldEntries(true)
	} else {
		parent.setHasDirectFieldEntries(true)
	}
	out := 0
	structuralChildIndex := 0
	for i := start; i < reducedEnd; i++ {
		entry := entries[i]
		if !stackEntryHasNode(entry) {
			continue
		}
		var fid FieldID
		if !stackEntryNodeIsExtra(entry) {
			if structuralChildIndex < len(rawFieldIDs) {
				fid = rawFieldIDs[structuralChildIndex]
			}
			structuralChildIndex++
		}
		if !stackEntryVisibleForPending(entry, symbolMeta) {
			continue
		}
		parent.setChildEntry(arena, out, entry)
		if useDenseFieldEntries && fid != 0 {
			parent.setChildFieldEntry(arena, out, fid, fieldSourceDirect)
		}
		out++
	}
	if out != window.childCount {
		arena.recordPendingParentRejected(pendingParentRejectFill)
		return false
	}
	gotoState := p.lookupGoto(topState, act.Symbol)
	targetState := topState
	if gotoState != 0 {
		targetState = gotoState
	}
	if tok.NoLookahead && targetState == topState {
		parent.setExtra(true)
	}
	parent.preGotoState = topState
	parent.parseState = targetState
	if !s.truncateBeforePush(truncateDepth) {
		s.dead = true
		return true
	}
	p.pushStackPendingParent(s, targetState, parent, entryScratch, gssScratch)
	for i := reducedEnd; i < trailingEnd; i++ {
		extra, ok := retargetStackEntryPayload(entries[i], targetState)
		if !ok {
			continue
		}
		p.pushStackEntry(s, extra, entryScratch, gssScratch)
	}
	if nodeCount != nil {
		*nodeCount = *nodeCount + 1
	}
	s.score += int(act.DynamicPrecedence)
	if anyReduced != nil {
		*anyReduced = true
	}
	return true
}

func (p *Parser) pendingDirectFieldParentEligible(s *glrStack, arena *nodeArena, rawFieldIDs []FieldID, rawInherited []bool) bool {
	if p == nil || p.language == nil || arena == nil || s == nil || !p.noResultCompatibilityBenchmarkOnly || len(rawFieldIDs) == 0 || !fieldIDSliceHasAny(rawFieldIDs) {
		return false
	}
	// Dart has a grammar-specific direct-field suppression rule that needs
	// materialized child type checks; keep that path on the existing reducer.
	if p.language.Name == "dart" {
		return false
	}
	for _, inherited := range rawInherited {
		if inherited {
			return false
		}
	}
	return true
}

type pendingDirectFieldParentWindow struct {
	childCount         int
	hasError           bool
	skippedHiddenChild bool
	first              stackEntry
	last               stackEntry
}

func scanPendingDirectFieldParentWindow(entries []stackEntry, start, reducedEnd int, arena *nodeArena, symbolMeta []SymbolMetadata) (pendingDirectFieldParentWindow, bool) {
	var window pendingDirectFieldParentWindow
	for i := start; i < reducedEnd; i++ {
		entry := entries[i]
		if !stackEntryHasNode(entry) {
			continue
		}
		if stackEntryNodeIsMissing(entry) {
			return pendingDirectFieldParentWindow{}, false
		}
		if !stackEntryVisibleForPending(entry, symbolMeta) {
			if stackEntryNodeHasError(entry) || stackEntryTreeHasFieldIDs(entry, arena) || pendingPlainHiddenVisibleDescendantCount(entry, arena, symbolMeta) != 0 {
				return pendingDirectFieldParentWindow{}, false
			}
			window.skippedHiddenChild = true
			continue
		}
		if window.childCount == 0 {
			window.first = entry
		}
		window.last = entry
		window.childCount++
		window.hasError = window.hasError || stackEntryNodeHasError(entry)
	}
	return window, window.childCount != 0
}

func pendingDirectFieldParentEntrySlots(childCount int, useDenseFieldEntries bool) int {
	if useDenseFieldEntries {
		return childCount * 2
	}
	return childCount
}

func (p *Parser) pendingDirectFieldParentFieldsRecomputable(productionID uint16, visibleChildCount int, entries []stackEntry, start, reducedEnd int, rawFieldIDs []FieldID, rawInherited []bool, symbolMeta []SymbolMetadata) bool {
	visibleFieldIDs, visibleInherited := p.fixedFieldIDsForProduction(visibleChildCount, productionID)
	structuralChildIndex := 0
	visibleStructuralChildIndex := 0
	for i := start; i < reducedEnd; i++ {
		entry := entries[i]
		if !stackEntryHasNode(entry) {
			continue
		}
		var fid FieldID
		inherited := false
		if !stackEntryNodeIsExtra(entry) {
			if structuralChildIndex < len(rawFieldIDs) {
				fid = rawFieldIDs[structuralChildIndex]
			}
			if structuralChildIndex < len(rawInherited) {
				inherited = rawInherited[structuralChildIndex]
			}
			structuralChildIndex++
		}
		if !stackEntryVisibleForPending(entry, symbolMeta) {
			continue
		}
		if stackEntryNodeIsExtra(entry) {
			continue
		}
		if visibleStructuralChildIndex >= visibleChildCount ||
			inherited ||
			visibleInherited[visibleStructuralChildIndex] ||
			fid != visibleFieldIDs[visibleStructuralChildIndex] {
			return false
		}
		visibleStructuralChildIndex++
	}
	return visibleStructuralChildIndex == visibleChildCount
}

func (p *Parser) fixedFieldIDsForProduction(childCount int, productionID uint16) ([32]FieldID, [32]bool) {
	var fieldIDs [32]FieldID
	var inherited [32]bool
	if p == nil || p.language == nil || childCount <= 0 || childCount > len(fieldIDs) || len(p.language.FieldMapEntries) == 0 {
		return fieldIDs, inherited
	}
	pid := int(productionID)
	if pid < 0 || pid >= len(p.language.FieldMapSlices) {
		return fieldIDs, inherited
	}
	fm := p.language.FieldMapSlices[pid]
	count := int(fm[1])
	if count == 0 {
		return fieldIDs, inherited
	}
	var conflictedInherited [32]bool
	start := int(fm[0])
	for i := 0; i < count; i++ {
		entryIdx := start + i
		if entryIdx >= len(p.language.FieldMapEntries) {
			break
		}
		entry := p.language.FieldMapEntries[entryIdx]
		idx := int(entry.ChildIndex)
		if idx < 0 || idx >= childCount {
			continue
		}
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
	}
	return fieldIDs, inherited
}

func (p *Parser) populatePendingDirectFieldEntries(parent *pendingParent, children []*Node, fieldIDs []FieldID, fieldSources []uint8, arena *nodeArena) {
	if p == nil || parent == nil || len(children) == 0 || len(fieldIDs) == 0 {
		return
	}
	structuralChildCount := 0
	for _, child := range children {
		if child != nil && !child.isExtra() {
			structuralChildCount++
		}
	}
	rawFieldIDs, rawInherited := p.buildFieldIDs(structuralChildCount, parent.productionID, arena)
	if len(rawFieldIDs) == 0 {
		return
	}
	structuralChildIndex := 0
	for i, child := range children {
		if child == nil || child.isExtra() {
			continue
		}
		var fid FieldID
		inherited := false
		if structuralChildIndex < len(rawFieldIDs) {
			fid = rawFieldIDs[structuralChildIndex]
			if structuralChildIndex < len(rawInherited) {
				inherited = rawInherited[structuralChildIndex]
			}
		}
		structuralChildIndex++
		if inherited || fid == 0 || p.shouldSuppressVisibleDirectField(child, fid) {
			continue
		}
		fieldIDs[i] = fid
		if i < len(fieldSources) {
			fieldSources[i] = fieldSourceDirect
		}
	}
}

func stackEntryTreeHasFieldIDs(entry stackEntry, arena *nodeArena) bool {
	if n := stackEntryNode(entry); n != nil {
		return hiddenTreeHasFieldIDs(n)
	}
	if parent := stackEntryPendingParent(entry); parent != nil {
		if parent.hasFieldEntries() {
			return true
		}
		for i := 0; i < parent.childEntryCount(); i++ {
			if stackEntryTreeHasFieldIDs(parent.childEntry(arena, i), arena) {
				return true
			}
		}
	}
	return false
}

func (p *Parser) recordPendingFieldRejectShape(arena *nodeArena, act ParseAction, entries []stackEntry, start, reducedEnd int) {
	if p == nil || p.language == nil || arena == nil {
		return
	}
	symbolMeta := p.language.SymbolMetadata
	if !symbolVisibleForPending(act.Symbol, symbolMeta) {
		arena.recordPendingParentFieldRejected(pendingParentFieldRejectParentHidden)
		return
	}
	rawFieldIDs, rawInherited := p.buildFieldIDs(int(act.ChildCount), act.ProductionID, arena)
	if len(rawFieldIDs) == 0 {
		arena.recordPendingParentFieldRejected(pendingParentFieldRejectNoIDs)
		return
	}
	for _, inherited := range rawInherited {
		if inherited {
			arena.recordPendingParentFieldRejected(pendingParentFieldRejectInherited)
			return
		}
	}
	for i := start; i < reducedEnd; i++ {
		entry := entries[i]
		if !stackEntryHasNode(entry) {
			continue
		}
		if stackEntryNodeIsMissing(entry) {
			arena.recordPendingParentFieldRejected(pendingParentFieldRejectChild)
			return
		}
		if !stackEntryVisibleForPending(entry, symbolMeta) {
			shape := pendingParentFieldRejectHiddenChildPlain
			if n := stackEntryNode(entry); hiddenTreeHasFieldIDs(n) {
				shape = pendingParentFieldRejectHiddenChildWithFields
			} else {
				switch pendingPlainHiddenVisibleDescendantCount(entry, arena, symbolMeta) {
				case 0:
					shape = pendingParentFieldRejectHiddenChildPlainEmpty
				case 1:
					shape = pendingParentFieldRejectHiddenChildPlainOne
				default:
					shape = pendingParentFieldRejectHiddenChildPlainMany
				}
			}
			arena.recordPendingParentFieldRejected(shape)
			return
		}
	}
	arena.recordPendingParentFieldRejected(pendingParentFieldRejectAllVisibleDirect)
}

func symbolVisibleForPending(sym Symbol, symbolMeta []SymbolMetadata) bool {
	if idx := int(sym); idx >= 0 && idx < len(symbolMeta) {
		return symbolMeta[sym].Visible
	}
	return true
}

func stackEntryVisibleForPending(entry stackEntry, symbolMeta []SymbolMetadata) bool {
	return symbolVisibleForPending(stackEntryNodeSymbol(entry), symbolMeta)
}

func pendingPlainHiddenVisibleDescendantCount(entry stackEntry, arena *nodeArena, symbolMeta []SymbolMetadata) int {
	if !stackEntryHasNode(entry) || stackEntryNodeIsMissing(entry) {
		return 0
	}
	if stackEntryVisibleForPending(entry, symbolMeta) {
		return 1
	}
	if parent := stackEntryPendingParent(entry); parent != nil {
		count := 0
		for i := 0; i < parent.childEntryCount(); i++ {
			child := parent.childEntry(arena, i)
			count += pendingPlainHiddenVisibleDescendantCount(child, arena, symbolMeta)
		}
		return count
	}
	if node := stackEntryNode(entry); node != nil && !hiddenTreeHasFieldIDs(node) {
		count := 0
		for _, child := range node.children {
			count += pendingPlainHiddenVisibleDescendantCount(newStackEntryNode(child.parseState, child), arena, symbolMeta)
		}
		return count
	}
	return 0
}

func pendingNoFieldChildCount(entry stackEntry, arena *nodeArena, parentVisible bool, symbolMeta []SymbolMetadata) (count int, hasPayload bool, hasError bool, ok bool) {
	if !stackEntryHasNode(entry) {
		return 0, false, false, true
	}
	if stackEntryNodeIsMissing(entry) {
		return 0, false, false, false
	}
	hasPayload = stackEntryCompactFullLeaf(entry) != nil || stackEntryPendingParent(entry) != nil
	hasError = stackEntryNodeHasError(entry)
	if stackEntryVisibleForPending(entry, symbolMeta) {
		return 1, hasPayload, hasError, true
	}
	if parentVisible {
		if stackEntryTreeHasFieldIDs(entry, arena) {
			return 0, false, false, false
		}
		if parent := stackEntryPendingParent(entry); parent != nil {
			for i := 0; i < parent.childEntryCount(); i++ {
				child := parent.childEntry(arena, i)
				childCount, childPayload, childHasError, childOK := pendingNoFieldChildCount(child, arena, true, symbolMeta)
				if !childOK {
					return 0, false, false, false
				}
				count += childCount
				hasPayload = hasPayload || childPayload
				hasError = hasError || childHasError
			}
			return count, hasPayload, hasError, true
		}
		if node := stackEntryNode(entry); node != nil {
			for _, child := range node.children {
				childEntry := newStackEntryNode(child.parseState, child)
				childCount, childPayload, childHasError, childOK := pendingNoFieldChildCount(childEntry, arena, true, symbolMeta)
				if !childOK {
					return 0, false, false, false
				}
				count += childCount
				hasPayload = hasPayload || childPayload
				hasError = hasError || childHasError
			}
		}
		return count, hasPayload, hasError, true
	}
	if stackEntryNodeChildCount(entry) == 0 {
		return 0, hasPayload, hasError, true
	}
	return 1, hasPayload, hasError, true
}

func pendingNoFieldChildEndpoints(entries []stackEntry, start, end int, arena *nodeArena, parentVisible bool, symbolMeta []SymbolMetadata) (first, last stackEntry, ok bool) {
	for i := start; i < end; i++ {
		next, found := pendingNoFieldFirstChild(entries[i], arena, parentVisible, symbolMeta)
		if !found {
			continue
		}
		first = next
		ok = true
		break
	}
	if !ok {
		return stackEntry{}, stackEntry{}, false
	}
	for i := end - 1; i >= start; i-- {
		next, found := pendingNoFieldLastChild(entries[i], arena, parentVisible, symbolMeta)
		if !found {
			continue
		}
		last = next
		return first, last, true
	}
	return stackEntry{}, stackEntry{}, false
}

func pendingNoFieldFirstChild(entry stackEntry, arena *nodeArena, parentVisible bool, symbolMeta []SymbolMetadata) (stackEntry, bool) {
	if !stackEntryHasNode(entry) || stackEntryNodeIsMissing(entry) {
		return stackEntry{}, false
	}
	if stackEntryVisibleForPending(entry, symbolMeta) {
		return entry, true
	}
	if parentVisible {
		if parent := stackEntryPendingParent(entry); parent != nil {
			for i := 0; i < parent.childEntryCount(); i++ {
				child := parent.childEntry(arena, i)
				if next, ok := pendingNoFieldFirstChild(child, arena, true, symbolMeta); ok {
					return next, true
				}
			}
			return stackEntry{}, false
		}
		if node := stackEntryNode(entry); node != nil {
			for _, child := range node.children {
				if next, ok := pendingNoFieldFirstChild(newStackEntryNode(child.parseState, child), arena, true, symbolMeta); ok {
					return next, true
				}
			}
		}
		return stackEntry{}, false
	}
	if stackEntryNodeChildCount(entry) == 0 {
		return stackEntry{}, false
	}
	return entry, true
}

func pendingNoFieldLastChild(entry stackEntry, arena *nodeArena, parentVisible bool, symbolMeta []SymbolMetadata) (stackEntry, bool) {
	if !stackEntryHasNode(entry) || stackEntryNodeIsMissing(entry) {
		return stackEntry{}, false
	}
	if stackEntryVisibleForPending(entry, symbolMeta) {
		return entry, true
	}
	if parentVisible {
		if parent := stackEntryPendingParent(entry); parent != nil {
			for i := parent.childEntryCount() - 1; i >= 0; i-- {
				child := parent.childEntry(arena, i)
				if next, ok := pendingNoFieldLastChild(child, arena, true, symbolMeta); ok {
					return next, true
				}
			}
			return stackEntry{}, false
		}
		if node := stackEntryNode(entry); node != nil {
			for i := len(node.children) - 1; i >= 0; i-- {
				child := node.children[i]
				if next, ok := pendingNoFieldLastChild(newStackEntryNode(child.parseState, child), arena, true, symbolMeta); ok {
					return next, true
				}
			}
		}
		return stackEntry{}, false
	}
	if stackEntryNodeChildCount(entry) == 0 {
		return stackEntry{}, false
	}
	return entry, true
}

func fillPendingNoFieldChildren(dst []pendingChildEntry, out int, entry stackEntry, arena *nodeArena, parentVisible bool, symbolMeta []SymbolMetadata) (next int, flattenedParents int, flattenedChildRefs int) {
	if !stackEntryHasNode(entry) || stackEntryNodeIsMissing(entry) {
		return out, 0, 0
	}
	if stackEntryVisibleForPending(entry, symbolMeta) {
		if out < len(dst) {
			dst[out] = newPendingChildEntry(entry)
			out++
		}
		return out, 0, 0
	}
	if parentVisible {
		if parent := stackEntryPendingParent(entry); parent != nil {
			before := out
			children := parent.childRefs(arena)
			for _, childRef := range children {
				child := childRef.stackEntry()
				var parents, refs int
				out, parents, refs = fillPendingNoFieldChildren(dst, out, child, arena, true, symbolMeta)
				flattenedParents += parents
				flattenedChildRefs += refs
			}
			if out > before {
				flattenedParents++
				flattenedChildRefs += len(children)
			}
			return out, flattenedParents, flattenedChildRefs
		}
		if node := stackEntryNode(entry); node != nil {
			before := out
			children := node.children
			for _, child := range children {
				var parents, refs int
				out, parents, refs = fillPendingNoFieldChildren(dst, out, newStackEntryNode(child.parseState, child), arena, true, symbolMeta)
				flattenedParents += parents
				flattenedChildRefs += refs
			}
			if out > before {
				flattenedChildRefs += len(children)
			}
		}
		return out, flattenedParents, flattenedChildRefs
	}
	if stackEntryNodeChildCount(entry) == 0 {
		return out, 0, 0
	}
	if out < len(dst) {
		dst[out] = newPendingChildEntry(entry)
		out++
	}
	return out, 0, 0
}

func pendingReduceWindowSpan(entries []stackEntry, start, end int) (reduceRawSpan, bool) {
	span := reduceRawSpan{}
	if end <= start {
		return span, false
	}
	foundStart := false
	for i := start; i < end; i++ {
		entry := entries[i]
		if !stackEntryHasNode(entry) || stackEntryNodeIsExtra(entry) {
			continue
		}
		span.startByte = stackEntryNodeStartByte(entry)
		span.startPoint = stackEntryNodeStartPoint(entry)
		foundStart = true
		break
	}
	if !foundStart {
		return span, false
	}
	for i := end - 1; i >= start; i-- {
		entry := entries[i]
		if !stackEntryHasNode(entry) || stackEntryNodeIsExtra(entry) {
			continue
		}
		span.endByte = stackEntryNodeEndByte(entry)
		span.endPoint = stackEntryNodeEndPoint(entry)
		return span, true
	}
	return span, true
}

func computeReduceRawSpan(entries []stackEntry, start, end int) reduceRawSpan {
	span := reduceRawSpan{}
	if end <= start {
		return span
	}

	foundStart := false
	for i := start; i < end; i++ {
		n := stackEntryNode(entries[i])
		if n != nil && !n.isExtra() {
			span.startByte = n.startByte
			span.startPoint = n.startPoint
			foundStart = true
			break
		}
	}

	foundEnd := false
	for i := end - 1; i >= start; i-- {
		n := stackEntryNode(entries[i])
		if n != nil && !n.isExtra() {
			span.endByte = n.endByte
			span.endPoint = n.endPoint
			foundEnd = true
			break
		}
	}

	firstRaw := stackEntryNode(entries[start])
	lastRaw := stackEntryNode(entries[end-1])
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
		n := stackEntryNode(entries[i])
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
func extendParentSpanToWindow(parent *Node, entries []stackEntry, start, reducedEnd int, symbolMeta []SymbolMetadata, spanExtendingInvisibleSymbols, nonSpanExtendingInvisibleSymbols []bool) {
	// Leading extras: extend startByte backward until the first structural child.
	for i := start; i < reducedEnd; i++ {
		n := stackEntryNode(entries[i])
		if n == nil {
			continue
		}
		if !n.isExtra() {
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
		n := stackEntryNode(entries[i])
		if n == nil || n.isExtra() {
			continue
		}
		visible := true
		if idx := int(n.symbol); idx < len(symbolMeta) {
			visible = symbolMeta[n.symbol].Visible
		}
		if visible {
			continue // visible children are already represented in parent's children
		}
		if symbolMarked(nonSpanExtendingInvisibleSymbols, n.symbol) {
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
			symbolMarked(spanExtendingInvisibleSymbols, n.symbol) {
			parent.endByte = n.endByte
			parent.endPoint = n.endPoint
		}
	}
	// Follow with a forward pass for endByte growth so contiguous hidden tails
	// can chain (for example interpolated multiline string middle -> string end).
	for i := start; i < reducedEnd; i++ {
		n := stackEntryNode(entries[i])
		if n == nil || n.isExtra() {
			continue
		}
		visible := true
		if idx := int(n.symbol); idx < len(symbolMeta) {
			visible = symbolMeta[n.symbol].Visible
		}
		if visible {
			continue
		}
		if symbolMarked(nonSpanExtendingInvisibleSymbols, n.symbol) {
			continue
		}
		if n.startByte <= parent.endByte && n.endByte > parent.endByte {
			parent.endByte = n.endByte
			parent.endPoint = n.endPoint
		}
		if n.startByte == n.endByte && n.startByte > parent.endByte &&
			symbolMarked(spanExtendingInvisibleSymbols, n.symbol) {
			parent.endByte = n.endByte
			parent.endPoint = n.endPoint
		}
	}
}

func buildInvisibleSpanSymbolTables(symbolNames []string) ([]bool, []bool) {
	var spanExtending []bool
	var nonSpanExtending []bool
	for i, name := range symbolNames {
		switch name {
		case "_implicit_end_tag",
			"_outdent",
			"_single_line_string_end",
			"_multiline_string_end",
			"_interpolated_string_middle",
			"_interpolated_multiline_string_middle":
			if spanExtending == nil {
				spanExtending = make([]bool, len(symbolNames))
			}
			spanExtending[i] = true
		case "_line_ending_or_eof":
			if nonSpanExtending == nil {
				nonSpanExtending = make([]bool, len(symbolNames))
			}
			nonSpanExtending[i] = true
		}
	}
	return spanExtending, nonSpanExtending
}

func symbolMarked(table []bool, sym Symbol) bool {
	idx := int(sym)
	return idx < len(table) && table[idx]
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
		if children[i] == nil || children[i].isExtra() || children[i].isMissing() || !children[i].isNamed() || fieldIDs[i] != 0 {
			continue
		}
		count++
	}
	return count
}

func countEligibleFieldTargets(children []*Node, fieldIDs []FieldID, start, end int) int {
	count := 0
	for i := start; i < end; i++ {
		if children[i] == nil || children[i].isExtra() || children[i].isMissing() || fieldIDs[i] != 0 {
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
		if child == nil || child.isExtra() || !nodeHasDirectFieldID(child, fid) {
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
	nodes             []*Node
	fieldIDs          []FieldID
	fieldSources      []uint8
	trackFields       bool
	repeatStamp       []uint32
	repeatCount       []uint16
	repeatSource      []uint8
	repeatTouched     []FieldID
	repeatEpoch       uint32
	transientParents  *transientParentScratch
	transientChildren *transientChildScratch
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
	children := arena.allocNodeSliceNoClear(len(scratch.nodes))
	if perfCountersEnabled {
		perfRecordReduceChildrenScratch(len(scratch.nodes))
	}
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

func (p *Parser) materializeNoFieldReduceChildrenFromScratch(scratch *reduceBuildScratch, arena *nodeArena) []*Node {
	if scratch == nil || len(scratch.nodes) == 0 {
		return nil
	}
	children := p.allocNoFieldReduceChildren(arena, len(scratch.nodes))
	if perfCountersEnabled {
		perfRecordReduceChildrenScratch(len(scratch.nodes))
	}
	copy(children, scratch.nodes)
	return children
}

func (p *Parser) allocNoFieldReduceChildren(arena *nodeArena, n int) []*Node {
	if n <= 0 {
		return nil
	}
	if p.shouldUseTransientReduceScratchNoAlias() {
		return p.transientChildren.alloc(n)
	}
	return arena.allocNodeSliceNoClear(n)
}

func (p *Parser) allocAllVisibleReduceChildren(arena *nodeArena, n int, aliasSeq []Symbol, rawFieldIDs []FieldID, rawInherited []bool) []*Node {
	if p != nil &&
		p.transientReduceChildren &&
		p.transientChildren != nil &&
		len(aliasSeq) == 0 &&
		len(rawFieldIDs) == 0 &&
		len(rawInherited) == 0 {
		return p.transientChildren.alloc(n)
	}
	return arena.allocNodeSliceNoClear(n)
}

func (p *Parser) buildReduceChildrenAllVisible(entries []stackEntry, start, end, childCount int, aliasSeq []Symbol, rawFieldIDs []FieldID, rawInherited []bool, symbolMeta []SymbolMetadata, arena *nodeArena) ([]*Node, []FieldID, []uint8, bool) {
	visibleCount := 0
	structuralChildIndex := 0
	for i := start; i < end; i++ {
		n := stackEntryNode(entries[i])
		if n == nil {
			continue
		}
		effectiveSymbol := n.symbol
		if !n.isExtra() {
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

	children := p.allocAllVisibleReduceChildren(arena, visibleCount, aliasSeq, rawFieldIDs, rawInherited)
	arena.recordReduceChildSliceAllVisible(visibleCount)
	if perfCountersEnabled {
		perfRecordReduceChildrenAllVisible(visibleCount)
	}
	var fieldIDs []FieldID
	var fieldSources []uint8
	if rawFieldIDs != nil {
		fieldIDs = arena.allocFieldIDSlice(visibleCount)
		fieldSources = arena.allocFieldSourceSlice(visibleCount)
	}

	out := 0
	structuralChildIndex = 0
	for i := start; i < end; i++ {
		n := stackEntryNode(entries[i])
		if n == nil {
			continue
		}
		var fid FieldID
		inherited := false
		if !n.isExtra() {
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
	children, fieldIDs, fieldSources, _ := p.buildReduceChildrenWithPath(entries, start, end, childCount, parentSymbol, productionID, arena)
	return children, fieldIDs, fieldSources
}

func (p *Parser) buildReduceChildrenWithPath(entries []stackEntry, start, end, childCount int, parentSymbol Symbol, productionID uint16, arena *nodeArena) ([]*Node, []FieldID, []uint8, reduceChildPath) {
	lang := p.language
	symbolMeta := lang.SymbolMetadata

	aliasSeq := p.reduceAliasSequence(productionID)
	productionHasFields := p.reduceProductionHasEffectiveFields(childCount, productionID, arena)
	if len(aliasSeq) == 0 && !productionHasFields {
		if children, fieldIDs, fieldSources, path, ok := p.buildReduceChildrenNoAliasNoFieldsPlanned(entries, start, end, parentSymbol, symbolMeta, arena); ok {
			return children, fieldIDs, fieldSources, path
		}
	}

	rawFieldIDs, rawInherited := p.buildFieldIDs(childCount, productionID, arena)
	if children, fieldIDs, fieldSources, ok := p.buildReduceChildrenAllVisible(entries, start, end, childCount, aliasSeq, rawFieldIDs, rawInherited, symbolMeta, arena); ok {
		return children, fieldIDs, fieldSources, reduceChildPathForLen(len(children), reduceChildPathAllVisible)
	}

	scratch := p.newReduceBuildScratch(rawFieldIDs)
	p.appendReduceChildrenToScratch(scratch, entries, start, end, aliasSeq, rawFieldIDs, rawInherited, symbolMeta, arena, lang)
	if scratch.trackFields {
		p.suppressReducedChildFields(scratch.nodes, scratch.fieldIDs, scratch.fieldSources)
	}
	if perfCountersEnabled {
		perfRecordReduceScratchGeneral(len(scratch.nodes))
	}
	arena.recordReduceChildSliceScratchGeneral(len(scratch.nodes))
	children, fieldIDs, fieldSources := materializeReduceChildrenFromScratch(scratch, arena)
	return children, fieldIDs, fieldSources, reduceChildPathForLen(len(children), reduceChildPathScratchGeneral)
}

func reduceChildPathForLen(n int, nonEmptyPath reduceChildPath) reduceChildPath {
	if n == 0 {
		return reduceChildPathNone
	}
	return nonEmptyPath
}

func reduceChildPathMayDropSpan(path reduceChildPath) bool {
	switch path {
	case reduceChildPathAllVisible, reduceChildPathNoAlias, reduceChildPathFastGSS:
		return false
	default:
		return true
	}
}

func (p *Parser) buildReduceChildrenNoAliasNoFieldsPlanned(entries []stackEntry, start, end int, parentSymbol Symbol, symbolMeta []SymbolMetadata, arena *nodeArena) ([]*Node, []FieldID, []uint8, reduceChildPath, bool) {
	visibleCount := 0
	allVisible := true
	preserveHiddenFields := false
	parentVisible := symbolVisibleForPending(parentSymbol, symbolMeta)
	for i := start; i < end; i++ {
		n := stackEntryNode(entries[i])
		if n == nil {
			continue
		}
		visible := true
		if idx := int(n.symbol); idx < len(symbolMeta) {
			visible = symbolMeta[n.symbol].Visible
		}
		if visible {
			visibleCount++
			continue
		}
		allVisible = false
		if parentVisible && hiddenTreeHasFieldIDs(n) {
			preserveHiddenFields = true
		}
	}
	if allVisible {
		if visibleCount == 0 {
			return nil, nil, nil, reduceChildPathNone, true
		}
		children := p.allocAllVisibleReduceChildren(arena, visibleCount, nil, nil, nil)
		arena.recordReduceChildSliceAllVisible(visibleCount)
		if perfCountersEnabled {
			perfRecordReduceChildrenAllVisible(visibleCount)
		}
		out := 0
		for i := start; i < end; i++ {
			n := stackEntryNode(entries[i])
			if n == nil {
				continue
			}
			children[out] = n
			out++
		}
		return children, nil, nil, reduceChildPathAllVisible, true
	}
	if preserveHiddenFields {
		return nil, nil, nil, reduceChildPathNone, false
	}

	var scratch *reduceBuildScratch
	if p != nil && p.reduceScratch != nil {
		scratch = p.reduceScratch
	} else {
		scratch = &reduceBuildScratch{}
	}
	scratch.reset()

	for i := start; i < end; i++ {
		n := stackEntryNode(entries[i])
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
	if perfCountersEnabled {
		perfRecordReduceScratchNoAlias(len(scratch.nodes))
	}
	arena.recordReduceChildSliceScratchNoAlias(len(scratch.nodes))
	children := p.materializeNoFieldReduceChildrenFromScratch(scratch, arena)
	path := reduceChildPathNone
	if len(children) > 0 {
		path = reduceChildPathScratchNoAlias
	}
	return children, nil, nil, path, true
}

func (p *Parser) newReduceBuildScratch(rawFieldIDs []FieldID) *reduceBuildScratch {
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
	return scratch
}

type reduceChildBuildItem struct {
	node                *Node
	fieldID             FieldID
	inherited           bool
	nextStructuralIndex int
}

func reduceChildBuildItemForEntry(entry stackEntry, structuralChildIndex int, aliasSeq []Symbol, rawFieldIDs []FieldID, rawInherited []bool, arena *nodeArena, lang *Language) (reduceChildBuildItem, bool) {
	n := stackEntryNode(entry)
	if n == nil {
		return reduceChildBuildItem{}, false
	}
	item := reduceChildBuildItem{node: n, nextStructuralIndex: structuralChildIndex}
	if n.isExtra() {
		return item, true
	}
	if structuralChildIndex < len(rawFieldIDs) {
		item.fieldID = rawFieldIDs[structuralChildIndex]
		if structuralChildIndex < len(rawInherited) {
			item.inherited = rawInherited[structuralChildIndex]
		}
	}
	if structuralChildIndex < len(aliasSeq) {
		if alias := aliasSeq[structuralChildIndex]; alias != 0 {
			item.node = aliasedNodeInArena(arena, lang, n, alias)
		}
	}
	item.nextStructuralIndex = structuralChildIndex + 1
	return item, true
}

func (p *Parser) appendReduceChildrenToScratch(scratch *reduceBuildScratch, entries []stackEntry, start, end int, aliasSeq []Symbol, rawFieldIDs []FieldID, rawInherited []bool, symbolMeta []SymbolMetadata, arena *nodeArena, lang *Language) {
	structuralChildIndex := 0
	for i := start; i < end; i++ {
		item, ok := reduceChildBuildItemForEntry(entries[i], structuralChildIndex, aliasSeq, rawFieldIDs, rawInherited, arena, lang)
		if !ok {
			continue
		}
		structuralChildIndex = item.nextStructuralIndex
		p.appendReduceChildItemToScratch(scratch, item, rawFieldIDs, structuralChildIndex, symbolMeta)
	}
}

func (p *Parser) appendReduceChildItemToScratch(scratch *reduceBuildScratch, item reduceChildBuildItem, rawFieldIDs []FieldID, nextStructuralChildIndex int, symbolMeta []SymbolMetadata) {
	n := item.node
	if symbolVisibleForPending(n.symbol, symbolMeta) {
		p.appendVisibleReduceChildToScratch(scratch, n, item.fieldID, item.inherited)
		return
	}
	if len(n.children) == 0 {
		return
	}

	spanStart := len(scratch.nodes)
	if hiddenTreeHasFieldIDs(n) {
		appendFlattenedHiddenChildrenWithFieldScratch(scratch, n, symbolMeta)
	} else {
		appendFlattenedHiddenChildrenToScratch(scratch, n, symbolMeta)
	}
	if item.fieldID == 0 {
		return
	}
	if !scratch.trackFields {
		scratch.ensureFieldStorage()
	}
	fieldEnd := len(scratch.fieldIDs)
	applyParentFieldToFlattenedHiddenSpan(scratch, n, spanStart, fieldEnd, item.fieldID, item.inherited, rawFieldIDs, nextStructuralChildIndex)
}

func (p *Parser) appendVisibleReduceChildToScratch(scratch *reduceBuildScratch, n *Node, fid FieldID, inherited bool) {
	out := len(scratch.nodes)
	scratch.appendNode(n)
	if !scratch.trackFields || inherited || p.shouldSuppressVisibleDirectField(n, fid) {
		return
	}
	scratch.fieldIDs[out] = fid
	if fid != 0 {
		scratch.fieldSources[out] = fieldSourceDirect
	}
}

func applyParentFieldToFlattenedHiddenSpan(scratch *reduceBuildScratch, hiddenParent *Node, spanStart, fieldEnd int, fid FieldID, inherited bool, rawFieldIDs []FieldID, nextStructuralChildIndex int) {
	source := fieldSourceForInheritance(inherited)
	hasField := flattenedSpanHasFieldID(scratch.fieldIDs, spanStart, fieldEnd, fid)
	if inherited && !hasField {
		if assignSingleDescendantInheritedField(scratch, spanStart, fieldEnd, fid) {
			normalizeMixedSourceFieldSpan(scratch.fieldIDs, scratch.fieldSources, spanStart, fieldEnd)
			return
		}
		if shouldSkipInheritedParentFieldForFlattenedSpan(scratch, hiddenParent, spanStart, fieldEnd, fid) {
			return
		}
	}
	if inherited && fieldIDAppearsLater(rawFieldIDs, nextStructuralChildIndex, fid) {
		return
	}
	applyFieldToFlattenedSpan(scratch.nodes, scratch.fieldIDs, scratch.fieldSources, spanStart, fieldEnd, fid, source, true)
	normalizeMixedSourceFieldSpan(scratch.fieldIDs, scratch.fieldSources, spanStart, fieldEnd)
}

func fieldSourceForInheritance(inherited bool) uint8 {
	if inherited {
		return fieldSourceInherited
	}
	return fieldSourceDirect
}

func assignSingleDescendantInheritedField(scratch *reduceBuildScratch, spanStart, fieldEnd int, fid FieldID) bool {
	target, ok := flattenedSpanSingleDescendantFieldTarget(scratch.nodes, spanStart, fieldEnd, fid)
	if !ok {
		return false
	}
	scratch.fieldIDs[target] = fid
	scratch.fieldSources[target] = fieldSourceInherited
	return true
}

func shouldSkipInheritedParentFieldForFlattenedSpan(scratch *reduceBuildScratch, hiddenParent *Node, spanStart, fieldEnd int, fid FieldID) bool {
	if fieldEnd-spanStart == 1 {
		child := scratch.nodes[spanStart]
		if child == nil || nodeHasDirectFieldID(child, fid) || len(child.children) == 0 {
			return true
		}
	}
	if hiddenParent.isNamed() && countEligibleNamedFieldTargets(scratch.nodes, scratch.fieldIDs, spanStart, fieldEnd) > 1 {
		return true
	}
	if !flattenedSpanHasAnyDirectField(scratch.nodes, scratch.fieldIDs, scratch.fieldSources, spanStart, fieldEnd) {
		return false
	}
	if fieldEnd-spanStart != 1 {
		return true
	}
	child := scratch.nodes[spanStart]
	return child == nil || !nodeHasDirectFieldID(child, fid)
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
	if p.language == nil || p.language.Name != "dart" {
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
	if !multipleKinds && conflictCount >= 2 {
		assignFieldToFlattenedSpanTargets(children, fieldIDs, fieldSources, start, end, fid, source, inherited, false, false)
		return
	}
	if !multipleKinds && conflictCount == 1 && preferNamed {
		if assignFieldToFlattenedSpanTargets(children, fieldIDs, fieldSources, start, end, fid, source, inherited, true, true) {
			return
		}
	}
	if flattenedSpanHasFieldID(fieldIDs, start, end, fid) {
		return
	}
	if inherited && !preferNamed {
		if countEligibleNamedFieldTargets(children, fieldIDs, start, end) > 1 {
			return
		}
	}
	if source == fieldSourceDirect {
		applyDirectFieldToUnassignedFlattenedSpan(children, fieldIDs, fieldSources, start, end, fid, source, preferNamed)
		return
	}
	assignFirstInheritedFieldToFlattenedSpan(children, fieldIDs, fieldSources, start, end, fid, source, preferNamed, inherited)
}

func assignFieldToFlattenedSpanTargets(children []*Node, fieldIDs []FieldID, fieldSources []uint8, start, end int, fid FieldID, source uint8, inherited, requireNamed, firstOnly bool) bool {
	assigned := false
	for j := start; j < end; j++ {
		if !flattenedFieldTargetEligible(children[j], requireNamed) {
			continue
		}
		if inherited && fieldIDs[j] != 0 && fieldIDs[j] != fid && fieldSourceAt(fieldSources, j) == fieldSourceDirect {
			continue
		}
		assignFlattenedField(fieldIDs, fieldSources, j, fid, source)
		assigned = true
		if firstOnly {
			return true
		}
	}
	return assigned
}

func applyDirectFieldToUnassignedFlattenedSpan(children []*Node, fieldIDs []FieldID, fieldSources []uint8, start, end int, fid FieldID, source uint8, preferNamed bool) {
	namedTargets := 0
	totalTargets := 0
	allowAnonymousSingleDirectTarget := false
	namedTargets = countEligibleNamedFieldTargets(children, fieldIDs, start, end)
	totalTargets = countEligibleFieldTargets(children, fieldIDs, start, end)
	allowAnonymousSingleDirectTarget = namedTargets == 0 && totalTargets == 1
	switch {
	case allowAnonymousSingleDirectTarget:
		assignFirstUnassignedFlattenedField(children, fieldIDs, fieldSources, start, end, fid, source, false)
	case namedTargets > 1:
		assignAllUnassignedFlattenedFields(children, fieldIDs, fieldSources, start, end, fid, source, true)
	case namedTargets == 1 && totalTargets > 1:
		assignAllUnassignedFlattenedFields(children, fieldIDs, fieldSources, start, end, fid, source, false)
	case namedTargets == 1:
		assignAllUnassignedFlattenedFields(children, fieldIDs, fieldSources, start, end, fid, source, true)
	default:
		assignFirstUnassignedFlattenedField(children, fieldIDs, fieldSources, start, end, fid, source, preferNamed)
	}
}

func assignFirstInheritedFieldToFlattenedSpan(children []*Node, fieldIDs []FieldID, fieldSources []uint8, start, end int, fid FieldID, source uint8, preferNamed, inherited bool) {
	for j := start; j < end; j++ {
		if fieldIDs[j] != 0 || children[j] == nil || children[j].isExtra() || children[j].isMissing() {
			continue
		}
		if preferNamed && !children[j].isNamed() {
			continue
		}
		if inherited && nodeHasDirectFieldID(children[j], fid) && end-start != 1 {
			continue
		}
		assignFlattenedField(fieldIDs, fieldSources, j, fid, source)
		break
	}
}

func assignFirstUnassignedFlattenedField(children []*Node, fieldIDs []FieldID, fieldSources []uint8, start, end int, fid FieldID, source uint8, requireNamed bool) {
	for k := start; k < end; k++ {
		if fieldIDs[k] != 0 || !flattenedFieldTargetEligible(children[k], requireNamed) {
			continue
		}
		assignFlattenedField(fieldIDs, fieldSources, k, fid, source)
		break
	}
}

func assignAllUnassignedFlattenedFields(children []*Node, fieldIDs []FieldID, fieldSources []uint8, start, end int, fid FieldID, source uint8, requireNamed bool) {
	for k := start; k < end; k++ {
		if fieldIDs[k] != 0 || !flattenedFieldTargetEligible(children[k], requireNamed) {
			continue
		}
		assignFlattenedField(fieldIDs, fieldSources, k, fid, source)
	}
}

func flattenedFieldTargetEligible(child *Node, requireNamed bool) bool {
	return child != nil && !child.isExtra() && !child.isMissing() && (!requireNamed || child.isNamed())
}

func assignFlattenedField(fieldIDs []FieldID, fieldSources []uint8, idx int, fid FieldID, source uint8) {
	fieldIDs[idx] = fid
	if fieldSources != nil {
		fieldSources[idx] = source
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

func (p *Parser) recordReductionParentConstructed(arena *nodeArena, parent *Node, sym Symbol, childCount int, fieldIDs []FieldID, fieldSources []uint8, childPath reduceChildPath) {
	if p == nil || p.language == nil || arena == nil {
		return
	}
	if arena.breakdownEnabled {
		visible := true
		if idx := int(sym); idx >= 0 && idx < len(p.language.SymbolMetadata) {
			visible = p.language.SymbolMetadata[sym].Visible
		}
		arena.recordReductionParentConstructed(visible, childCount, fieldIDs, fieldSources)
	}
	if arena.audit != nil {
		arena.audit.recordReduceParentChildPath(parent, childPath, childCount)
	}
}

func (p *Parser) newReduceParentNode(arena *nodeArena, sym Symbol, named bool, children []*Node, fieldIDs []FieldID, fieldSources []uint8, productionID uint16, deferParentLinks bool, trackChildErrors bool) *Node {
	var transientParents *transientParentScratch
	var transientChildren *transientChildScratch
	if p != nil && p.reduceScratch != nil {
		transientParents = p.reduceScratch.transientParents
		transientChildren = p.reduceScratch.transientChildren
	}
	if deferParentLinks &&
		transientChildren != nil &&
		transientParents != nil &&
		len(fieldIDs) == 0 &&
		len(fieldSources) == 0 &&
		transientChildren.owns(children) {
		return transientParents.allocParent(arena, sym, named, children, productionID, trackChildErrors)
	}
	if deferParentLinks {
		return newParentNodeInArenaNoLinksWithFieldSources(arena, sym, named, children, fieldIDs, fieldSources, productionID, trackChildErrors)
	}
	return newParentNodeInArenaWithFieldSources(arena, sym, named, children, fieldIDs, fieldSources, productionID)
}

type collapseUnaryRule uint8

const (
	collapseUnaryRuleNone collapseUnaryRule = iota
	collapseUnaryRuleSameSymbol
	collapseUnaryRuleInvisibleWrapper
	collapseUnaryRuleNamedLeafAlias
)

func recordCollapseRule(arena *nodeArena, rule collapseUnaryRule) {
	switch rule {
	case collapseUnaryRuleSameSymbol:
		arena.collapseRuleSameSymbol++
	case collapseUnaryRuleInvisibleWrapper:
		arena.collapseRuleInvisibleWrapper++
	case collapseUnaryRuleNamedLeafAlias:
		arena.collapseRuleNamedLeafAlias++
	}
}

func (p *Parser) applyReduceAction(s *glrStack, act ParseAction, tok Token, anyReduced *bool, nodeCount *int, arena *nodeArena, entryScratch *glrEntryScratch, gssScratch *gssScratch, entries []stackEntry, deferParentLinks bool, trackChildErrors bool) {
	timing := p.reduceTiming
	childCount := int(act.ChildCount)
	var (
		window reduceRange
		ok     bool
	)
	rangeStart := time.Time{}
	if timing != nil {
		rangeStart = time.Now()
	}
	if p != nil && p.noTreeBenchmarkOnly {
		window, ok = computeReduceRangePayload(entries, childCount)
	} else {
		window, ok = computeReduceRangeForFullPayloads(entries, childCount, p.usePendingFullParents())
	}
	if timing != nil {
		timing.reduceRangeNanos += time.Since(rangeStart).Nanoseconds()
	}
	if !ok {
		// Not enough stack entries — kill this stack version.
		s.dead = true
		return
	}

	if p != nil && p.noTreeBenchmarkOnly {
		noTreeStart := time.Time{}
		if timing != nil {
			noTreeStart = time.Now()
		}
		if !s.truncateBeforePush(window.start) {
			if timing != nil {
				timing.reduceNoTreeBuildNanos += time.Since(noTreeStart).Nanoseconds()
			}
			s.dead = true
			return
		}
		p.pushNoTreeReduceNode(s, act, tok, arena, entryScratch, gssScratch, entries, window.start, window.reducedEnd, window.reducedEnd, window.actualEnd, window.topState, nodeCount, trackChildErrors)
		if timing != nil {
			timing.reduceNoTreeBuildNanos += time.Since(noTreeStart).Nanoseconds()
		}
		s.score += int(act.DynamicPrecedence)
		*anyReduced = true
		return
	}
	if p.usePendingFullParents() {
		if child, ok := p.collapsibleRawUnarySelfReductionEntry(act, tok, arena, entries, window.start, window.reducedEnd); ok {
			if !s.truncateBeforePush(window.start) {
				s.dead = true
				return
			}
			p.pushCollapsedUnaryReduceEntry(s, act, tok, child, entryScratch, gssScratch, entries, window.reducedEnd, window.actualEnd, window.topState)
			s.score += int(act.DynamicPrecedence)
			*anyReduced = true
			return
		}
	}
	if p.usePendingFullParents() {
		pendingStart := time.Time{}
		if timing != nil {
			pendingStart = time.Now()
		}
		if p.tryPushPendingNoFieldParent(s, act, tok, anyReduced, nodeCount, arena, entryScratch, gssScratch, entries, window.start, window.reducedEnd, window.actualEnd, window.topState, window.start) {
			if timing != nil {
				timing.reducePendingParentNanos += time.Since(pendingStart).Nanoseconds()
			}
			return
		}
		materializePendingPayloadEntries(p, entries, window.start, window.actualEnd, arena)
		if timing != nil {
			timing.reducePendingParentNanos += time.Since(pendingStart).Nanoseconds()
		}
	}

	if child := p.collapsibleRawUnarySelfReduction(act, tok, arena, entries, window.start, window.reducedEnd); child != nil {
		if !s.truncateBeforePush(window.start) {
			s.dead = true
			return
		}
		p.pushCollapsedUnaryReduceNode(s, act, tok, child, entryScratch, gssScratch, entries, window.reducedEnd, window.actualEnd, window.topState)
		s.score += int(act.DynamicPrecedence)
		*anyReduced = true
		return
	}

	childStart := time.Time{}
	if timing != nil {
		childStart = time.Now()
	}
	children, fieldIDs, fieldSources, childPath := p.buildReduceChildrenWithPath(entries, window.start, window.reducedEnd, childCount, act.Symbol, act.ProductionID, arena)
	if timing != nil {
		timing.reduceChildBuildNanos += time.Since(childStart).Nanoseconds()
	}

	trailingStart := window.reducedEnd
	trailingEnd := window.actualEnd

	// Pop all reduced entries in one step after collection.
	if !s.truncateBeforePush(window.start) {
		s.dead = true
		return
	}

	if child := p.collapsibleUnarySelfReduction(act, tok, arena, entries, window.start, window.reducedEnd, children, fieldIDs); child != nil {
		p.pushCollapsedUnaryReduceNode(s, act, tok, child, entryScratch, gssScratch, entries, trailingStart, trailingEnd, window.topState)
		s.score += int(act.DynamicPrecedence)
		*anyReduced = true
		return
	}

	named := p.isNamedSymbol(act.Symbol)
	var parent *Node
	parentStart := time.Time{}
	if timing != nil {
		parentStart = time.Now()
	}
	if deferParentLinks {
		parent = newParentNodeInArenaNoLinksWithFieldSources(arena, act.Symbol, named, children, fieldIDs, fieldSources, act.ProductionID, trackChildErrors)
	} else {
		parent = newParentNodeInArenaWithFieldSources(arena, act.Symbol, named, children, fieldIDs, fieldSources, act.ProductionID)
	}
	p.recordReductionParentConstructed(arena, parent, act.Symbol, len(children), fieldIDs, fieldSources, childPath)
	if timing != nil {
		timing.reduceParentBuildNanos += time.Since(parentStart).Nanoseconds()
	}
	spanStart := time.Time{}
	if timing != nil {
		spanStart = time.Now()
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
	if reduceChildPathMayDropSpan(childPath) {
		extendParentSpanToWindow(parent, entries, window.start, window.reducedEnd, p.language.SymbolMetadata, p.spanExtendingInvisibleSymbols, p.nonSpanExtendingInvisibleSymbols)
	}
	if timing != nil {
		timing.reduceSpanNanos += time.Since(spanStart).Nanoseconds()
	}
	*nodeCount++

	pushStart := time.Time{}
	if timing != nil {
		pushStart = time.Now()
	}
	gotoState := p.lookupGoto(window.topState, act.Symbol)
	targetState := window.topState
	if gotoState != 0 {
		targetState = gotoState
	}
	if tok.NoLookahead && targetState == window.topState {
		parent.setExtra(true)
	}
	parent.preGotoState = window.topState
	parent.parseState = targetState
	p.pushStackNode(s, targetState, parent, entryScratch, gssScratch)
	for i := trailingStart; i < trailingEnd; i++ {
		extra := stackEntryNode(entries[i])
		if extra == nil {
			continue
		}
		extra.parseState = targetState
		nodeBumpEquivVersion(extra)
		p.pushStackNode(s, targetState, extra, entryScratch, gssScratch)
	}
	if timing != nil {
		timing.reduceStackPushNanos += time.Since(pushStart).Nanoseconds()
	}

	s.score += int(act.DynamicPrecedence)
	*anyReduced = true
}

func (p *Parser) applyReduceActionTransientParents(s *glrStack, act ParseAction, tok Token, anyReduced *bool, nodeCount *int, arena *nodeArena, entryScratch *glrEntryScratch, gssScratch *gssScratch, entries []stackEntry, deferParentLinks bool, trackChildErrors bool) {
	timing := p.reduceTiming
	childCount := int(act.ChildCount)
	var (
		window reduceRange
		ok     bool
	)
	rangeStart := time.Time{}
	if timing != nil {
		rangeStart = time.Now()
	}
	if p != nil && p.noTreeBenchmarkOnly {
		window, ok = computeReduceRangePayload(entries, childCount)
	} else {
		window, ok = computeReduceRangeForFullPayloads(entries, childCount, p.usePendingFullParents())
	}
	if timing != nil {
		timing.reduceRangeNanos += time.Since(rangeStart).Nanoseconds()
	}
	if !ok {
		s.dead = true
		return
	}

	if p != nil && p.noTreeBenchmarkOnly {
		noTreeStart := time.Time{}
		if timing != nil {
			noTreeStart = time.Now()
		}
		if !s.truncateBeforePush(window.start) {
			if timing != nil {
				timing.reduceNoTreeBuildNanos += time.Since(noTreeStart).Nanoseconds()
			}
			s.dead = true
			return
		}
		p.pushNoTreeReduceNode(s, act, tok, arena, entryScratch, gssScratch, entries, window.start, window.reducedEnd, window.reducedEnd, window.actualEnd, window.topState, nodeCount, trackChildErrors)
		if timing != nil {
			timing.reduceNoTreeBuildNanos += time.Since(noTreeStart).Nanoseconds()
		}
		s.score += int(act.DynamicPrecedence)
		*anyReduced = true
		return
	}
	if p.usePendingFullParents() {
		if child, ok := p.collapsibleRawUnarySelfReductionEntry(act, tok, arena, entries, window.start, window.reducedEnd); ok {
			if !s.truncateBeforePush(window.start) {
				s.dead = true
				return
			}
			p.pushCollapsedUnaryReduceEntry(s, act, tok, child, entryScratch, gssScratch, entries, window.reducedEnd, window.actualEnd, window.topState)
			s.score += int(act.DynamicPrecedence)
			*anyReduced = true
			return
		}
	}
	if p.usePendingFullParents() {
		pendingStart := time.Time{}
		if timing != nil {
			pendingStart = time.Now()
		}
		if p.tryPushPendingNoFieldParent(s, act, tok, anyReduced, nodeCount, arena, entryScratch, gssScratch, entries, window.start, window.reducedEnd, window.actualEnd, window.topState, window.start) {
			if timing != nil {
				timing.reducePendingParentNanos += time.Since(pendingStart).Nanoseconds()
			}
			return
		}
		materializePendingPayloadEntries(p, entries, window.start, window.actualEnd, arena)
		if timing != nil {
			timing.reducePendingParentNanos += time.Since(pendingStart).Nanoseconds()
		}
	}

	if child := p.collapsibleRawUnarySelfReduction(act, tok, arena, entries, window.start, window.reducedEnd); child != nil {
		if !s.truncateBeforePush(window.start) {
			s.dead = true
			return
		}
		p.pushCollapsedUnaryReduceNode(s, act, tok, child, entryScratch, gssScratch, entries, window.reducedEnd, window.actualEnd, window.topState)
		s.score += int(act.DynamicPrecedence)
		*anyReduced = true
		return
	}

	childStart := time.Time{}
	if timing != nil {
		childStart = time.Now()
	}
	children, fieldIDs, fieldSources, childPath := p.buildReduceChildrenWithPath(entries, window.start, window.reducedEnd, childCount, act.Symbol, act.ProductionID, arena)
	if timing != nil {
		timing.reduceChildBuildNanos += time.Since(childStart).Nanoseconds()
	}

	trailingStart := window.reducedEnd
	trailingEnd := window.actualEnd

	if !s.truncateBeforePush(window.start) {
		s.dead = true
		return
	}

	if child := p.collapsibleUnarySelfReduction(act, tok, arena, entries, window.start, window.reducedEnd, children, fieldIDs); child != nil {
		p.pushCollapsedUnaryReduceNode(s, act, tok, child, entryScratch, gssScratch, entries, trailingStart, trailingEnd, window.topState)
		s.score += int(act.DynamicPrecedence)
		*anyReduced = true
		return
	}

	named := p.isNamedSymbol(act.Symbol)
	parentStart := time.Time{}
	if timing != nil {
		parentStart = time.Now()
	}
	parent := p.newReduceParentNode(arena, act.Symbol, named, children, fieldIDs, fieldSources, act.ProductionID, deferParentLinks, trackChildErrors)
	p.recordReductionParentConstructed(arena, parent, act.Symbol, len(children), fieldIDs, fieldSources, childPath)
	if timing != nil {
		timing.reduceParentBuildNanos += time.Since(parentStart).Nanoseconds()
	}
	spanStart := time.Time{}
	if timing != nil {
		spanStart = time.Now()
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
	if reduceChildPathMayDropSpan(childPath) {
		extendParentSpanToWindow(parent, entries, window.start, window.reducedEnd, p.language.SymbolMetadata, p.spanExtendingInvisibleSymbols, p.nonSpanExtendingInvisibleSymbols)
	}
	if timing != nil {
		timing.reduceSpanNanos += time.Since(spanStart).Nanoseconds()
	}
	*nodeCount++

	pushStart := time.Time{}
	if timing != nil {
		pushStart = time.Now()
	}
	gotoState := p.lookupGoto(window.topState, act.Symbol)
	targetState := window.topState
	if gotoState != 0 {
		targetState = gotoState
	}
	if tok.NoLookahead && targetState == window.topState {
		parent.setExtra(true)
	}
	parent.preGotoState = window.topState
	parent.parseState = targetState
	p.pushStackNode(s, targetState, parent, entryScratch, gssScratch)
	for i := trailingStart; i < trailingEnd; i++ {
		extra := stackEntryNode(entries[i])
		if extra == nil {
			continue
		}
		extra.parseState = targetState
		nodeBumpEquivVersion(extra)
		p.pushStackNode(s, targetState, extra, entryScratch, gssScratch)
	}
	if timing != nil {
		timing.reduceStackPushNanos += time.Since(pushStart).Nanoseconds()
	}

	s.score += int(act.DynamicPrecedence)
	*anyReduced = true
}

func (p *Parser) pushNoTreeReduceNode(s *glrStack, act ParseAction, tok Token, arena *nodeArena, entryScratch *glrEntryScratch, gssScratch *gssScratch, entries []stackEntry, start, reducedEnd, trailingStart, trailingEnd int, topState StateID, nodeCount *int, trackChildErrors bool) {
	gotoState := p.lookupGoto(topState, act.Symbol)
	targetState := topState
	if gotoState != 0 {
		targetState = gotoState
	}

	parent := newNoTreeReduceNodeInArena(arena, act.Symbol, p.isNamedSymbol(act.Symbol), act.ProductionID, entries, start, reducedEnd, tok, trackChildErrors)
	if tok.NoLookahead && targetState == topState {
		parent.setExtra(true)
	}
	parent.preGotoState = topState
	parent.parseState = targetState
	p.pushStackNoTreeNode(s, targetState, parent, entryScratch, gssScratch)
	for i := trailingStart; i < trailingEnd; i++ {
		extra, ok := retargetStackEntryPayload(entries[i], targetState)
		if !ok {
			continue
		}
		p.pushStackEntry(s, extra, entryScratch, gssScratch)
	}
	if nodeCount != nil {
		*nodeCount = *nodeCount + 1
	}
}

func (p *Parser) pushStackNoTreeNode(s *glrStack, state StateID, node *noTreeNode, entryScratch *glrEntryScratch, gssScratch *gssScratch) {
	entry := newStackEntryNoTreeNode(state, node)
	s.pushEntry(entry, entryScratch, gssScratch)
	if !s.recoverabilityKnown {
		return
	}
	if !s.mayRecover && p.stateCanRecover(state) {
		s.mayRecover = true
	}
}

func (p *Parser) pushStackCompactCheckpointLeaf(s *glrStack, state StateID, leaf *compactCheckpointLeaf, entryScratch *glrEntryScratch, gssScratch *gssScratch) {
	entry := newStackEntryCompactCheckpointLeaf(state, leaf)
	p.pushStackEntry(s, entry, entryScratch, gssScratch)
}

func (p *Parser) pushStackCompactFullLeaf(s *glrStack, state StateID, leaf *compactFullLeaf, entryScratch *glrEntryScratch, gssScratch *gssScratch) {
	entry := newStackEntryCompactFullLeaf(state, leaf)
	p.pushStackEntry(s, entry, entryScratch, gssScratch)
}

func (p *Parser) pushStackPendingParent(s *glrStack, state StateID, parent *pendingParent, entryScratch *glrEntryScratch, gssScratch *gssScratch) {
	entry := newStackEntryPendingParent(state, parent)
	p.pushStackEntry(s, entry, entryScratch, gssScratch)
}

func newNoTreeReduceNodeInArena(arena *nodeArena, sym Symbol, named bool, productionID uint16, entries []stackEntry, start, reducedEnd int, tok Token, trackChildErrors bool) *noTreeNode {
	var n *noTreeNode
	if arena == nil {
		n = &noTreeNode{}
	} else {
		n = arena.allocNoTreeNode()
		arena.noTreeReduceNodesConstructed++
	}
	n.symbol = sym
	n.startByte = tok.StartByte
	n.endByte = tok.StartByte
	n.parseState = 0
	n.preGotoState = 0
	n.productionID = productionID
	n.flags = noTreeNodeInitialFlags(named)
	if reducedEnd > start {
		firstRaw := entries[start]
		lastRaw := entries[reducedEnd-1]
		var firstNonExtra stackEntry
		var lastNonExtra stackEntry
		for i := start; i < reducedEnd; i++ {
			child := entries[i]
			if !stackEntryHasNode(child) {
				continue
			}
			if !stackEntryNodeIsExtra(child) {
				if !stackEntryHasNode(firstNonExtra) {
					firstNonExtra = child
				}
				lastNonExtra = child
			}
			if trackChildErrors && stackEntryNodeHasError(child) {
				n.setHasError(true)
			}
		}
		if stackEntryHasNode(firstNonExtra) {
			n.startByte = stackEntryNodeStartByte(firstNonExtra)
		} else if stackEntryHasNode(firstRaw) {
			n.startByte = stackEntryNodeStartByte(firstRaw)
		}
		if stackEntryHasNode(lastNonExtra) {
			n.endByte = stackEntryNodeEndByte(lastNonExtra)
		} else if stackEntryHasNode(lastRaw) {
			n.endByte = stackEntryNodeEndByte(lastRaw)
		}
	}
	return n
}

func (p *Parser) pushCollapsedUnaryReduceNode(s *glrStack, act ParseAction, tok Token, child *Node, entryScratch *glrEntryScratch, gssScratch *gssScratch, entries []stackEntry, trailingStart, trailingEnd int, topState StateID) {
	gotoState := p.lookupGoto(topState, act.Symbol)
	targetState := topState
	if gotoState != 0 {
		targetState = gotoState
	}
	if tok.NoLookahead && targetState == topState {
		child.setExtra(true)
	}
	child.productionID = act.ProductionID
	child.preGotoState = topState
	child.parseState = targetState
	nodeBumpEquivVersion(child)
	p.pushStackNode(s, targetState, child, entryScratch, gssScratch)
	for i := trailingStart; i < trailingEnd; i++ {
		extra := stackEntryNode(entries[i])
		if extra == nil {
			continue
		}
		extra.parseState = targetState
		nodeBumpEquivVersion(extra)
		p.pushStackNode(s, targetState, extra, entryScratch, gssScratch)
	}
}

func (p *Parser) pushCollapsedUnaryReduceEntry(s *glrStack, act ParseAction, tok Token, child stackEntry, entryScratch *glrEntryScratch, gssScratch *gssScratch, entries []stackEntry, trailingStart, trailingEnd int, topState StateID) {
	gotoState := p.lookupGoto(topState, act.Symbol)
	targetState := topState
	if gotoState != 0 {
		targetState = gotoState
	}
	setCollapsedUnaryEntryMetadata(&child, act, tok.NoLookahead && targetState == topState, topState, targetState)
	p.pushStackEntry(s, child, entryScratch, gssScratch)
	for i := trailingStart; i < trailingEnd; i++ {
		extra, ok := retargetStackEntryPayload(entries[i], targetState)
		if !ok {
			continue
		}
		p.pushStackEntry(s, extra, entryScratch, gssScratch)
	}
}

func setCollapsedUnaryEntryMetadata(entry *stackEntry, act ParseAction, extra bool, preGotoState, parseState StateID) {
	if entry == nil {
		return
	}
	if n := stackEntryNode(*entry); n != nil {
		if extra {
			n.setExtra(true)
		}
		n.productionID = act.ProductionID
		n.preGotoState = preGotoState
		n.parseState = parseState
		nodeBumpEquivVersion(n)
		entry.state = parseState
		return
	}
	if n := stackEntryCompactFullLeaf(*entry); n != nil {
		if extra {
			n.setExtra(true)
		}
		n.productionID = act.ProductionID
		n.preGotoState = preGotoState
		n.parseState = parseState
		entry.state = parseState
		return
	}
	if n := stackEntryPendingParent(*entry); n != nil {
		if extra {
			n.setExtra(true)
		}
		n.productionID = act.ProductionID
		n.preGotoState = preGotoState
		n.parseState = parseState
		entry.state = parseState
	}
}

func (p *Parser) collapsibleRawUnarySelfReductionEntry(act ParseAction, tok Token, arena *nodeArena, entries []stackEntry, start, reducedEnd int) (stackEntry, bool) {
	if p == nil || arena == nil {
		return stackEntry{}, false
	}
	diag := arena.breakdownEnabled
	if diag {
		arena.collapseRawUnaryAttempts++
	}
	if tok.NoLookahead {
		if diag {
			arena.collapseRawUnaryMissShape++
		}
		return stackEntry{}, false
	}
	if reducedEnd-start != 1 || start < 0 || reducedEnd > len(entries) {
		if diag {
			arena.collapseRawUnaryMissShape++
		}
		return stackEntry{}, false
	}
	if p.reduceProductionHasEffectiveFields(int(act.ChildCount), act.ProductionID, arena) || len(p.reduceAliasSequence(act.ProductionID)) != 0 {
		if diag {
			arena.collapseRawUnaryMissGrammar++
		}
		return stackEntry{}, false
	}

	entry := entries[start]
	if child := stackEntryNode(entry); child != nil {
		if child.ownerArena != arena || child.parent != nil {
			if diag {
				arena.collapseRawUnaryMissChild++
			}
			return stackEntry{}, false
		}
		if !p.isVisibleSymbol(child.symbol) {
			if diag {
				arena.collapseRawUnaryMissChild++
			}
			return stackEntry{}, false
		}
		collapsed, rule := p.collapseUnaryChildForReductionWithRule(act, arena, child)
		if collapsed == nil {
			if diag {
				arena.collapseRawUnaryMissRule++
			}
			return stackEntry{}, false
		}
		if diag {
			arena.collapseRawUnarySuccesses++
			recordCollapseRule(arena, rule)
		}
		return newStackEntryNode(entry.state, collapsed), true
	}

	if parent := stackEntryPendingParent(entry); parent != nil {
		if !p.isVisibleSymbol(parent.symbol) {
			if diag {
				arena.collapseRawUnaryMissChild++
			}
			return stackEntry{}, false
		}
		rule := p.collapseUnaryPendingParentRule(act, parent)
		if rule == collapseUnaryRuleNone {
			if diag {
				arena.collapseRawUnaryMissRule++
			}
			return stackEntry{}, false
		}
		if diag {
			arena.collapseRawUnarySuccesses++
			recordCollapseRule(arena, rule)
		}
		return entry, true
	}

	leaf := stackEntryCompactFullLeaf(entry)
	if leaf == nil {
		if diag {
			arena.collapseRawUnaryMissChild++
		}
		return stackEntry{}, false
	}
	if !p.isVisibleSymbol(leaf.symbol) || leaf.isExtra() || leaf.isMissing() || leaf.hasError() {
		if diag {
			arena.collapseRawUnaryMissChild++
		}
		return stackEntry{}, false
	}
	rule := p.collapseUnaryLeafRule(act, leaf.symbol)
	if rule == collapseUnaryRuleNone {
		if diag {
			arena.collapseRawUnaryMissRule++
		}
		return stackEntry{}, false
	}
	if rule == collapseUnaryRuleNamedLeafAlias {
		cloned := newCompactFullLeafInArena(arena, leaf.symbol, leaf.isNamed(), leaf.startByte, leaf.endByte, leaf.startPoint, leaf.endPoint)
		*cloned = *leaf
		leaf = cloned
		entry = newStackEntryCompactFullLeaf(entry.state, leaf)
	}
	if rule == collapseUnaryRuleNamedLeafAlias {
		leaf.symbol = act.Symbol
		leaf.setNamed(p.isNamedSymbol(act.Symbol))
	}
	if diag {
		arena.collapseRawUnarySuccesses++
		recordCollapseRule(arena, rule)
	}
	return entry, true
}

func (p *Parser) collapseUnaryPendingParentRule(act ParseAction, parent *pendingParent) collapseUnaryRule {
	if parent == nil || parent.isExtra() || parent.isMissing() || parent.hasError() {
		return collapseUnaryRuleNone
	}
	if parent.symbol != act.Symbol {
		if p.canCollapseInvisibleUnaryWrapperSymbol(act.Symbol) {
			return collapseUnaryRuleInvisibleWrapper
		}
		return collapseUnaryRuleNone
	}
	return collapseUnaryRuleSameSymbol
}

func (p *Parser) collapseUnaryLeafRule(act ParseAction, childSym Symbol) collapseUnaryRule {
	if childSym != act.Symbol {
		if p.canCollapseInvisibleUnaryWrapperSymbol(act.Symbol) {
			return collapseUnaryRuleInvisibleWrapper
		}
		if !p.canCollapseNamedLeafWrapper(act.Symbol, childSym) {
			return collapseUnaryRuleNone
		}
		if p.shouldPreserveVisibleUnaryTokenWrapper(act.Symbol) {
			return collapseUnaryRuleNone
		}
		if !p.isSingleTokenWrapperSymbol(act.Symbol) && !p.sameSymbolName(act.Symbol, childSym) {
			return collapseUnaryRuleNone
		}
		return collapseUnaryRuleNamedLeafAlias
	}
	return collapseUnaryRuleSameSymbol
}

func (p *Parser) canCollapseInvisibleUnaryWrapperSymbol(parentSym Symbol) bool {
	if p == nil || p.language == nil {
		return false
	}
	meta := p.language.SymbolMetadata
	if int(parentSym) >= len(meta) {
		return false
	}
	return !meta[parentSym].Visible
}

func (p *Parser) collapsibleRawUnarySelfReduction(act ParseAction, tok Token, arena *nodeArena, entries []stackEntry, start, reducedEnd int) *Node {
	if p == nil || arena == nil {
		return nil
	}
	diag := arena.breakdownEnabled
	if diag {
		arena.collapseRawUnaryAttempts++
	}
	if tok.NoLookahead {
		if diag {
			arena.collapseRawUnaryMissShape++
		}
		return nil
	}
	if reducedEnd-start != 1 || start < 0 || reducedEnd > len(entries) {
		if diag {
			arena.collapseRawUnaryMissShape++
		}
		return nil
	}
	if p.reduceProductionHasEffectiveFields(int(act.ChildCount), act.ProductionID, arena) || len(p.reduceAliasSequence(act.ProductionID)) != 0 {
		if diag {
			arena.collapseRawUnaryMissGrammar++
		}
		return nil
	}
	child := stackEntryNode(entries[start])
	if child == nil || child.ownerArena != arena || child.parent != nil {
		if diag {
			arena.collapseRawUnaryMissChild++
		}
		return nil
	}
	if !p.isVisibleSymbol(child.symbol) {
		if diag {
			arena.collapseRawUnaryMissChild++
		}
		return nil
	}
	collapsed, rule := p.collapseUnaryChildForReductionWithRule(act, arena, child)
	if collapsed == nil {
		if diag {
			arena.collapseRawUnaryMissRule++
		}
		return nil
	}
	if diag {
		arena.collapseRawUnarySuccesses++
		recordCollapseRule(arena, rule)
	}
	return collapsed
}

func (p *Parser) collapsibleUnarySelfReduction(act ParseAction, tok Token, arena *nodeArena, entries []stackEntry, start, reducedEnd int, children []*Node, fieldIDs []FieldID) *Node {
	if p == nil || arena == nil {
		return nil
	}
	diag := arena.breakdownEnabled
	if diag {
		arena.collapseUnaryAttempts++
	}
	if tok.NoLookahead {
		if diag {
			arena.collapseUnaryMissShape++
		}
		return nil
	}
	if reducedEnd-start != 1 || len(children) != 1 {
		if diag {
			arena.collapseUnaryMissShape++
		}
		return nil
	}
	if fieldIDSliceHasAny(fieldIDs) {
		if diag {
			arena.collapseUnaryMissFielded++
		}
		return nil
	}
	child := children[0]
	if child == nil || child.ownerArena != arena || child.parent != nil {
		if diag {
			arena.collapseUnaryMissChild++
		}
		return nil
	}
	if start < 0 || start >= len(entries) || stackEntryNode(entries[start]) != child {
		if diag {
			arena.collapseUnaryMissChild++
		}
		return nil
	}
	if p.reduceProductionHasEffectiveFields(int(act.ChildCount), act.ProductionID, arena) || len(p.reduceAliasSequence(act.ProductionID)) != 0 {
		if diag {
			arena.collapseUnaryMissGrammar++
		}
		return nil
	}
	collapsed, rule := p.collapseUnaryChildForReductionWithRule(act, arena, child)
	if collapsed == nil {
		if diag {
			arena.collapseUnaryMissRule++
		}
		return nil
	}
	if diag {
		arena.collapseUnarySuccesses++
		recordCollapseRule(arena, rule)
	}
	return collapsed
}

func (p *Parser) collapseUnaryChildForReductionWithRule(act ParseAction, arena *nodeArena, child *Node) (*Node, collapseUnaryRule) {
	if child.symbol != act.Symbol {
		if p.canCollapseInvisibleUnaryWrapper(act.Symbol, child) {
			return child, collapseUnaryRuleInvisibleWrapper
		}
		if child.ChildCount() != 0 || !p.canCollapseNamedLeafWrapper(act.Symbol, child.symbol) {
			return nil, collapseUnaryRuleNone
		}
		if p.shouldPreserveVisibleUnaryTokenWrapper(act.Symbol) {
			return nil, collapseUnaryRuleNone
		}
		if !p.isSingleTokenWrapperSymbol(act.Symbol) && !p.sameSymbolName(act.Symbol, child.symbol) {
			return nil, collapseUnaryRuleNone
		}
		return aliasedNodeInArena(arena, p.language, child, act.Symbol), collapseUnaryRuleNamedLeafAlias
	}
	return child, collapseUnaryRuleSameSymbol
}

func (p *Parser) canCollapseInvisibleUnaryWrapper(parentSym Symbol, child *Node) bool {
	if p == nil || p.language == nil || child == nil || child.isExtra() || child.isMissing() || child.IsError() {
		return false
	}
	meta := p.language.SymbolMetadata
	if int(parentSym) >= len(meta) {
		return false
	}
	return !meta[parentSym].Visible
}

func (p *Parser) shouldPreserveVisibleUnaryTokenWrapper(parentSym Symbol) bool {
	if p == nil || p.language == nil || p.language.Name != "java" {
		return false
	}
	if int(parentSym) < 0 || int(parentSym) >= len(p.language.SymbolNames) {
		return false
	}
	switch p.language.SymbolNames[parentSym] {
	case "integral_type", "floating_point_type":
		return true
	default:
		return false
	}
}

func (p *Parser) isVisibleSymbol(sym Symbol) bool {
	if p == nil || p.language == nil {
		return true
	}
	meta := p.language.SymbolMetadata
	if idx := int(sym); idx >= 0 && idx < len(meta) {
		return meta[sym].Visible
	}
	return true
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

func (p *Parser) reduceProductionHasEffectiveFields(_ int, productionID uint16, _ *nodeArena) bool {
	return p.reduceProductionHasFields(productionID)
}

func fieldIDSliceHasAny(fieldIDs []FieldID) bool {
	for _, fid := range fieldIDs {
		if fid != 0 {
			return true
		}
	}
	return false
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
			cloned.setNamed(lang.SymbolMetadata[alias].Named)
		}
		return cloned
	}

	cloned := arena.allocNode()
	*cloned = *n
	cloned.symbol = alias
	if lang != nil && int(alias) < len(lang.SymbolMetadata) {
		cloned.setNamed(lang.SymbolMetadata[alias].Named)
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
			// Rename-through is only sound when the lone visible descendant is a
			// LEAF (a token-shaped wrapper the alias relabels in place, e.g.
			// alias($._hidden_token, $.name) -> (name)). When the descendant is an
			// internal node with its own children (a real visible wrapper such as a
			// single list_item produced by repeat1(alias($._x, $.y)) reduced once),
			// upstream tree-sitter NESTS it under the new alias rather than
			// collapsing the layer. Returning nil routes the caller to
			// materializeHiddenNodeForAlias, which builds the proper wrapper.
			if len(n.children) != 0 {
				return nil
			}
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
		if nodeHasFinalChildRefs(n) {
			childCount := nodeChildCountNoMaterialize(n)
			if childCount > 0 {
				cloned.children = make([]*Node, childCount)
				for i := 0; i < childCount; i++ {
					cloned.children[i] = nodeChildAtForReason(n, i, materializeForNormalization)
				}
			}
			cloned.childIndex = -1
		}
		return cloned
	}
	cloned := arena.allocNode()
	*cloned = *n
	cloned.ownerArena = arena
	if nodeHasFinalChildRefs(n) {
		childCount := nodeChildCountNoMaterialize(n)
		if childCount > 0 {
			children := arena.allocNodeSlice(childCount)
			for i := 0; i < childCount; i++ {
				children[i] = nodeChildAtForReason(n, i, materializeForNormalization)
			}
			cloned.children = children
		}
		cloned.childIndex = -1
	}
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

// buildFieldIDs creates the temporary field ID slice for a reduce action.
func (p *Parser) buildFieldIDs(childCount int, productionID uint16, _ *nodeArena) ([]FieldID, []bool) {
	if childCount <= 0 || len(p.language.FieldMapEntries) == 0 {
		return nil, nil
	}

	pid := int(productionID)
	if pid >= len(p.language.FieldMapSlices) {
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
				fieldIDs = p.fieldIDScratchFor(childCount)
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

func (p *Parser) fieldIDScratchFor(childCount int) []FieldID {
	if childCount <= 0 {
		return nil
	}
	if p == nil {
		return make([]FieldID, childCount)
	}
	if cap(p.fieldIDScratch) < childCount {
		p.fieldIDScratch = make([]FieldID, childCount)
	} else {
		p.fieldIDScratch = p.fieldIDScratch[:childCount]
		clear(p.fieldIDScratch)
	}
	return p.fieldIDScratch
}
