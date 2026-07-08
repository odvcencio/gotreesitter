package gotreesitter

import (
	"bytes"
	"fmt"
	"strings"
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

type reduceFieldPlan struct {
	childCount uint8
	fieldIDs   []FieldID
	inherited  []bool
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

func buildReduceFieldPlans(lang *Language) []reduceFieldPlan {
	if lang == nil || len(lang.FieldMapSlices) == 0 || len(lang.FieldMapEntries) == 0 {
		return nil
	}
	out := make([]reduceFieldPlan, len(lang.FieldMapSlices))
	seen := make([]bool, len(out))
	any := false
	for _, entry := range lang.ParseActions {
		for _, act := range entry.Actions {
			if act.Type != ParseActionReduce {
				continue
			}
			pid := int(act.ProductionID)
			if pid < 0 || pid >= len(out) || seen[pid] {
				continue
			}
			fieldIDs, inherited := buildFieldPlanForProduction(lang, int(act.ChildCount), act.ProductionID)
			if fieldIDs == nil {
				continue
			}
			out[pid] = reduceFieldPlan{
				childCount: act.ChildCount,
				fieldIDs:   fieldIDs,
				inherited:  inherited,
			}
			seen[pid] = true
			any = true
		}
	}
	if !any {
		return nil
	}
	return out
}

func fieldMapHasEffectiveFields(lang *Language, childCount int, productionID uint16) bool {
	fieldIDs, _ := buildFieldPlanForProduction(lang, childCount, productionID)
	return fieldIDSliceHasAny(fieldIDs)
}

func buildFieldPlanForProduction(lang *Language, childCount int, productionID uint16) ([]FieldID, []bool) {
	if lang == nil || childCount <= 0 || len(lang.FieldMapEntries) == 0 {
		return nil, nil
	}
	pid := int(productionID)
	if pid < 0 || pid >= len(lang.FieldMapSlices) {
		return nil, nil
	}
	fm := lang.FieldMapSlices[pid]
	count := int(fm[1])
	if count == 0 {
		return nil, nil
	}
	fieldIDs := make([]FieldID, childCount)
	inherited := make([]bool, childCount)
	conflictedInherited := make([]bool, childCount)
	start := int(fm[0])
	assigned := false
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
		assigned = true
	}
	if !assigned {
		return nil, nil
	}
	return fieldIDs, inherited
}

// buildKeepSameNamedAnonChildSymbols computes, for each visible named rule
// symbol, whether a unary reduction over a same-named visible anonymous token
// child must KEEP that child (C tree-sitter childCount==1) rather than collapse
// it to a childless named leaf (childCount==0).
//
// The distinguisher (verified against the C oracle): a `rule: $ => 'literal'`
// rule whose literal appears in ONLY that rule is inlined by tree-sitter into a
// single named token (childCount==0, COLLAPSE — e.g. go's `nil`/`true`/`false`/
// `iota`). When the literal is shared across 2+ productions, tree-sitter extracts
// it as a distinct anonymous token that the named rule then wraps as a visible
// child (childCount==1, KEEP — e.g. ruby `nil`, solidity `true`/`false`, css
// `to`, typst `return`).
//
// The runtime proxy for "the anonymous token is a genuinely extracted/shared
// token" is the number of distinct parse states it shifts INTO: a literal used
// only by its own wrapper rule shifts into exactly one state (which immediately
// reduces the wrapper), while a shared/extracted token shifts into 2+ distinct
// states. So: KEEP iff the same-named anon token shifts into >= 2 distinct
// target states.
//
// Returns a per-symbol slice indexed by the named parent symbol, or nil when no
// such keep-cases exist (so synthetic Languages without a parse table are
// unaffected).
func buildKeepSameNamedAnonChildSymbols(lang *Language) []bool {
	if lang == nil || len(lang.SymbolMetadata) == 0 {
		return nil
	}
	if len(lang.ParseTable) == 0 && len(lang.SmallParseTableMap) == 0 {
		return nil
	}

	meta := lang.SymbolMetadata
	tokenCount := int(lang.TokenCount)

	// For each name, find its visible named symbol and visible anonymous token.
	type pair struct{ named, anon int }
	byName := map[string]*pair{}
	for sym := 0; sym < len(meta); sym++ {
		m := meta[sym]
		if !m.Visible {
			continue
		}
		nm := m.Name
		if nm == "" && sym < len(lang.SymbolNames) {
			nm = lang.SymbolNames[sym]
		}
		if nm == "" {
			continue
		}
		p := byName[nm]
		if p == nil {
			p = &pair{named: -1, anon: -1}
			byName[nm] = p
		}
		if m.Named {
			p.named = sym
		} else {
			p.anon = sym
		}
	}

	// Collect anon-token candidates (those with a same-named named twin) and
	// count distinct shift-target states per anon token.
	anonShiftTargets := map[int]map[int]struct{}{}
	for _, p := range byName {
		if p.named < 0 || p.anon < 0 {
			continue
		}
		anonShiftTargets[p.anon] = map[int]struct{}{}
	}
	if len(anonShiftTargets) == 0 {
		return nil
	}

	denseLimit := int(lang.LargeStateCount)
	if denseLimit == 0 {
		denseLimit = len(lang.ParseTable)
	}
	smallBase := int(lang.LargeStateCount)
	forEachStateAction(lang, denseLimit, smallBase, func(state, sym int, act ParseAction) {
		if act.Type != ParseActionShift {
			return
		}
		if set, ok := anonShiftTargets[sym]; ok && sym < tokenCount {
			set[int(act.State)] = struct{}{}
		}
	})

	out := make([]bool, len(meta))
	any := false
	for _, p := range byName {
		if p.named < 0 || p.anon < 0 {
			continue
		}
		if len(anonShiftTargets[p.anon]) >= 2 {
			out[p.named] = true
			any = true
		}
	}
	if !any {
		return nil
	}
	return out
}

func buildSharedAnonymousTokenSymbols(lang *Language) []bool {
	if lang == nil || len(lang.SymbolMetadata) == 0 || (len(lang.ParseTable) == 0 && len(lang.SmallParseTableMap) == 0) {
		return nil
	}
	tokenCount := int(lang.TokenCount)
	shiftTargets := map[int]map[int]struct{}{}
	for sym, meta := range lang.SymbolMetadata {
		if sym >= tokenCount || !meta.Visible || meta.Named {
			continue
		}
		shiftTargets[sym] = map[int]struct{}{}
	}
	if len(shiftTargets) == 0 {
		return nil
	}

	denseLimit := int(lang.LargeStateCount)
	if denseLimit == 0 {
		denseLimit = len(lang.ParseTable)
	}
	smallBase := int(lang.LargeStateCount)
	forEachStateAction(lang, denseLimit, smallBase, func(state, sym int, act ParseAction) {
		if act.Type != ParseActionShift || sym >= tokenCount {
			return
		}
		if set, ok := shiftTargets[sym]; ok {
			set[int(act.State)] = struct{}{}
		}
	})

	out := make([]bool, len(lang.SymbolMetadata))
	any := false
	for sym, targets := range shiftTargets {
		if len(targets) >= 2 {
			out[sym] = true
			any = true
		}
	}
	if !any {
		return nil
	}
	return out
}

// forEachStateAction iterates every (state, symbol) -> ParseAction across both
// the dense and small (sparse) parse tables, invoking fn for each action of the
// resolved action entry. Action index 0 is the no-action/error entry.
func forEachStateAction(lang *Language, denseLimit, smallBase int, fn func(state, sym int, act ParseAction)) {
	resolve := func(state, sym int, idx uint16) {
		if idx == 0 || int(idx) >= len(lang.ParseActions) {
			return
		}
		for _, act := range lang.ParseActions[idx].Actions {
			fn(state, sym, act)
		}
	}

	for state := 0; state < denseLimit && state < len(lang.ParseTable); state++ {
		row := lang.ParseTable[state]
		for sym, idx := range row {
			if idx == 0 {
				continue
			}
			resolve(state, sym, idx)
		}
	}

	table := lang.SmallParseTable
	for smallIdx, offset := range lang.SmallParseTableMap {
		state := smallBase + smallIdx
		pos := int(offset)
		if pos >= len(table) {
			continue
		}
		groupCount := table[pos]
		pos++
		for i := uint16(0); i < groupCount; i++ {
			if pos+1 >= len(table) {
				break
			}
			sectionValue := table[pos]
			symbolCount := table[pos+1]
			pos += 2
			for j := uint16(0); j < symbolCount; j++ {
				if pos >= len(table) {
					break
				}
				sym := int(table[pos])
				resolve(state, sym, sectionValue)
				pos++
			}
		}
	}
}

func (p *Parser) applyActionWithReduceChain(source []byte, s *glrStack, act ParseAction, tok Token, anyReduced *bool, nodeCount *int, arena *nodeArena, entryScratch *glrEntryScratch, gssScratch *gssScratch, tmpEntries *[]stackEntry, deferParentLinks bool, trackChildErrors *bool) bool {
	if act.Type != ParseActionReduce {
		p.applyAction(source, s, act, tok, anyReduced, nodeCount, arena, entryScratch, gssScratch, tmpEntries, deferParentLinks, trackChildErrors)
		return false
	}
	p.applyReduceActionDispatch(source, s, act, tok, anyReduced, nodeCount, arena, entryScratch, gssScratch, tmpEntries, deferParentLinks, trackChildErrors)
	if tok.NoLookahead || s == nil || s.dead || s.accepted || s.shifted {
		return false
	}
	return p.chainSingleReduceActions(source, s, tok, anyReduced, nodeCount, arena, entryScratch, gssScratch, tmpEntries, deferParentLinks, trackChildErrors)
}

type conflictReduceFrontierSeed struct {
	action      ParseAction
	beforeState StateID
	beforeByte  uint32
	beforeDepth int
	afterState  StateID
	afterByte   uint32
	afterDepth  int
}

func (p *Parser) completeConflictReduceFrontier(source []byte, s *glrStack, tok Token, seed conflictReduceFrontierSeed, liveStacks, maxLiveStacks int, allocBranchOrder func() uint64, anyReduced *bool, nodeCount *int, arena *nodeArena, entryScratch *glrEntryScratch, gssScratch *gssScratch, tmpEntries *[]stackEntry, deferParentLinks bool, trackChildErrors *bool) {
	if p == nil || p.language == nil || tok.NoLookahead || s == nil || s.dead || s.accepted || s.shifted || s.cPaused {
		return
	}
	// Mirror of the C runtime's version-window discipline
	// (MAX_VERSION_COUNT + MAX_VERSION_COUNT_OVERFLOW): C never creates a new
	// stack version once the version pool is saturated — the current version
	// simply keeps reducing without forking. This port's boundary merge/cull
	// enforces the same window (maxStacks + fullParseGLRStackOverflow) only
	// BETWEEN iterations, so a single token's conflict-reduce frontier walk
	// could admit one transient shifted fork per pending spine frame with no
	// population bound at all (observed: 6 survivors x 258-step walks = 1548
	// transient stacks and O(n^2) node allocation on union-list-shaped
	// sources). Once the live population (dispatch stacks plus already-queued
	// pending forks) has reached the same cull-engagement threshold the
	// boundary discipline uses, stop CREATING new frontier forks; the walk
	// itself continues reducing in place, and the stacks such forks would
	// have duplicated are exactly the ones the next boundary cull retains.
	// maxLiveStacks <= 0 disables the gate (incremental and c_sharp arenas,
	// where glrStackCullTrigger returns maxStacks with no overflow window and
	// culling historically engages immediately).
	frontierForkAdmissible := func() bool {
		if maxLiveStacks <= 0 {
			return true
		}
		return liveStacks+len(p.pendingFrontierForkStacks)+len(p.pendingForkStacks) < maxLiveStacks
	}
	if seed.action.Type != ParseActionReduce || seed.beforeDepth == 0 || seed.afterDepth == 0 {
		return
	}
	if seed.afterState != s.top().state || seed.afterByte != s.byteOffset || seed.afterDepth != s.depth() {
		return
	}
	const maxConflictFrontierActions = 256
	type frontierReduceKey struct {
		state             StateID
		byteOffset        uint32
		depth             int
		symbol            Symbol
		childCount        uint8
		productionID      uint16
		dynamicPrecedence int16
		actionState       StateID
	}
	seenReduces := make(map[frontierReduceKey]struct{}, 4)
	terminalForkedReduces := make(map[frontierReduceKey]struct{}, 4)
	seedKey := frontierReduceKey{
		state:             seed.beforeState,
		byteOffset:        seed.beforeByte,
		depth:             seed.beforeDepth,
		symbol:            seed.action.Symbol,
		childCount:        seed.action.ChildCount,
		productionID:      seed.action.ProductionID,
		dynamicPrecedence: seed.action.DynamicPrecedence,
		actionState:       seed.action.State,
	}
	makeReduceKey := func(act ParseAction) frontierReduceKey {
		return frontierReduceKey{
			state:             s.top().state,
			byteOffset:        s.byteOffset,
			depth:             s.depth(),
			symbol:            act.Symbol,
			childCount:        act.ChildCount,
			productionID:      act.ProductionID,
			dynamicPrecedence: act.DynamicPrecedence,
			actionState:       act.State,
		}
	}
	seenReduces[seedKey] = struct{}{}
	completeMultiActionFrontier := func(actions []ParseAction, step int) bool {
		if len(actions) != 2 {
			return false
		}
		var terminal ParseAction
		terminalSet := false
		var reduce ParseAction
		reduceSet := false
		for _, act := range actions {
			switch act.Type {
			case ParseActionReduce:
				if reduceSet {
					return false
				}
				reduce = act
				reduceSet = true
			case ParseActionShift, ParseActionRecover, ParseActionAccept:
				if terminalSet {
					return false
				}
				terminal = act
				terminalSet = true
			default:
				return false
			}
		}
		if !reduceSet || !terminalSet {
			return false
		}
		currentReduceKey := makeReduceKey(reduce)
		appendTerminalFork := func(fork glrStack) {
			if allocBranchOrder != nil {
				fork.branchOrder = allocBranchOrder()
			}
			p.pendingFrontierForkStacks = append(p.pendingFrontierForkStacks, fork)
			terminalForkedReduces[currentReduceKey] = struct{}{}
		}
		terminalAppended := false
		_, terminalAlreadyForked := terminalForkedReduces[currentReduceKey]
		if !terminalAlreadyForked && frontierForkAdmissible() {
			switch terminal.Type {
			case ParseActionShift:
				fork := s.cloneWithScratch(gssScratch)
				if p.guardRealShiftGap(source, &fork, tok) {
					p.noteStopActionDiagnostic("conflict-frontier-fork-shift", &fork, tok, terminal, len(actions), true, step, 0, false)
					p.applyShiftAction(&fork, terminal, tok, nodeCount, arena, entryScratch, gssScratch, trackChildErrors)
					p.noteStopActionResult(&fork)
					appendTerminalFork(fork)
					terminalAppended = true
				}
			case ParseActionAccept:
				fork := s.cloneWithScratch(gssScratch)
				p.noteStopActionDiagnostic("conflict-frontier-fork-accept", &fork, tok, terminal, len(actions), true, step, 0, false)
				p.applyAcceptAction(&fork)
				p.noteStopActionResult(&fork)
				appendTerminalFork(fork)
				terminalAppended = true
			case ParseActionRecover:
				fork := s.cloneWithScratch(gssScratch)
				if p.guardRealTokenAttachmentGap(source, &fork, tok, "conflict-frontier-fork-recover") {
					p.noteStopActionDiagnostic("conflict-frontier-fork-recover", &fork, tok, terminal, len(actions), true, step, 0, false)
					p.applyRecoverAction(&fork, terminal, tok, nodeCount, arena, entryScratch, gssScratch, trackChildErrors)
					fork.shifted = true
					p.noteStopActionResult(&fork)
					appendTerminalFork(fork)
					terminalAppended = true
				}
			default:
				return false
			}
		}
		if _, ok := seenReduces[currentReduceKey]; ok {
			if terminalAppended || terminalAlreadyForked {
				s.dead = true
				return true
			}
			return false
		}
		seenReduces[currentReduceKey] = struct{}{}
		p.noteStopActionDiagnostic("conflict-frontier-fork-reduce", s, tok, reduce, len(actions), true, step, 0, false)
		p.applyReduceActionDispatch(source, s, reduce, tok, anyReduced, nodeCount, arena, entryScratch, gssScratch, tmpEntries, deferParentLinks, trackChildErrors)
		p.noteStopActionResult(s)
		return true
	}
	for step := 1; step <= maxConflictFrontierActions; step++ {
		if s.dead || s.accepted || s.shifted || s.cPaused || s.depth() == 0 {
			return
		}
		actionIdx := p.lookupActionIndex(s.top().state, tok.Symbol)
		if actionIdx == 0 || int(actionIdx) >= len(p.language.ParseActions) {
			return
		}
		actions := p.language.ParseActions[actionIdx].Actions
		if len(actions) != 1 {
			if completeMultiActionFrontier(actions, step) {
				continue
			}
			return
		}
		act := actions[0]
		switch act.Type {
		case ParseActionReduce:
			key := makeReduceKey(act)
			if _, ok := seenReduces[key]; ok {
				return
			}
			seenReduces[key] = struct{}{}
			p.noteStopActionDiagnostic("conflict-frontier-reduce", s, tok, act, 1, true, step, 0, false)
			p.applyReduceActionDispatch(source, s, act, tok, anyReduced, nodeCount, arena, entryScratch, gssScratch, tmpEntries, deferParentLinks, trackChildErrors)
			p.noteStopActionResult(s)
		case ParseActionShift:
			if !p.guardRealShiftGap(source, s, tok) {
				return
			}
			p.noteStopActionDiagnostic("conflict-frontier-shift", s, tok, act, 1, true, step, 0, false)
			p.applyShiftAction(s, act, tok, nodeCount, arena, entryScratch, gssScratch, trackChildErrors)
			p.noteStopActionResult(s)
			return
		case ParseActionAccept:
			p.noteStopActionDiagnostic("conflict-frontier-accept", s, tok, act, 1, true, step, 0, false)
			p.applyAcceptAction(s)
			p.noteStopActionResult(s)
			return
		case ParseActionRecover:
			if !p.guardRealTokenAttachmentGap(source, s, tok, "conflict-frontier-recover") {
				return
			}
			p.noteStopActionDiagnostic("conflict-frontier-recover", s, tok, act, 1, true, step, 0, false)
			p.applyRecoverAction(s, act, tok, nodeCount, arena, entryScratch, gssScratch, trackChildErrors)
			s.shifted = true
			p.noteStopActionResult(s)
			return
		default:
			return
		}
	}
}

func (p *Parser) pushOrExtendErrorNode(s *glrStack, state StateID, tok Token, nodeCount *int, arena *nodeArena, entryScratch *glrEntryScratch, gssScratch *gssScratch, trackChildErrors *bool) {
	if p != nil {
		// An ERROR node is entering a stack: costs can be nonzero from here on
		// (sticky per-parse gate, see crecoveryCostCompetitionRelevant).
		p.crecoveryCostCompetitionRelevant = true
	}
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
	pushState := p.schemeErrorRecoveryState(state)
	errNode.parseState = pushState
	p.pushStackNode(s, pushState, errNode, entryScratch, gssScratch)
	if nodeCount != nil {
		*nodeCount = *nodeCount + 1
	}
}

// pushLexErrorRunLeaf absorbs an unlexable-run lookahead (errorSymbol token
// from NextWithErrorRuns) into the stack, mirroring C's skipped-error lexing:
// the run becomes an ERROR leaf that is EXTRA — present in the tree but
// transparent to production arity — and parsing resumes in the same state.
func (p *Parser) pushLexErrorRunLeaf(s *glrStack, state StateID, tok Token, nodeCount *int, arena *nodeArena, entryScratch *glrEntryScratch, gssScratch *gssScratch, trackChildErrors *bool) {
	if p != nil {
		// See pushOrExtendErrorNode: error content makes costs relevant.
		p.crecoveryCostCompetitionRelevant = true
	}
	leaf := newLeafNodeInArena(arena, errorSymbol, true,
		tok.StartByte, tok.EndByte, tok.StartPoint, tok.EndPoint)
	leaf.setHasError(true)

	if s != nil {
		top := stackEntryNode(s.top())
		if top != nil &&
			top.symbol == errorSymbol &&
			top.isExtra() &&
			!top.isMissing() &&
			len(top.children) > 0 &&
			top.parseState == state &&
			tok.StartByte >= top.endByte {
			// Incremental open-region cost: an unlexable-run extend is the
			// same append-only mutation as a recovery absorb, so keep the
			// (node, equivVersion) memo warm instead of forcing an
			// O(len(children)) rewalk per skipped token (see
			// cErrRegionPostAbsorb, parser_recover_c.go).
			pre := p.cErrRegionPreAbsorb(top)
			top.children = append(top.children, leaf)
			top.endByte = tok.EndByte
			top.endPoint = tok.EndPoint
			top.setHasError(true)
			nodeBumpEquivVersion(top)
			p.cErrRegionPostAbsorb(pre, leaf)
			if debugRecoveryCycleChecks {
				debugRecoveryCheckNodeAcyclic(p, arena, "lex-error-run-extend", top)
			}
			if s.byteOffset < top.endByte {
				s.byteOffset = top.endByte
			}
			if trackChildErrors != nil {
				*trackChildErrors = true
			}
			if nodeCount != nil {
				*nodeCount = *nodeCount + 1
			}
			return
		}
	}

	// C wraps the skipped-error token in an ERROR internal node (the error
	// repeat), so the tree shape is ERROR -> ERROR-leaf.
	wrapper := newParentNodeInArena(arena, errorSymbol, true, []*Node{leaf}, nil, 0)
	wrapper.setHasError(true)
	wrapper.setExtra(true)
	if trackChildErrors != nil {
		*trackChildErrors = true
	}
	if perfCountersEnabled {
		perfRecordErrorNode()
	}
	wrapper.parseState = state
	p.pushStackNode(s, state, wrapper, entryScratch, gssScratch)
	if nodeCount != nil {
		*nodeCount = *nodeCount + 2
	}
}

// schemeErrorRecoveryState returns the state an error node should be pushed in
// for tree-sitter-scheme. When a datum fails inside a list that has not yet
// shifted a datum (e.g. immediately after "("), the error must be recovered as
// a `_datum` so the list keeps its opening delimiter, matching tree-sitter C's
// `(list "(" (ERROR) ...)`. We follow the grammar's own GOTO on `_datum`; if it
// leaves the state unchanged (the list is already in its datum-repeat state) or
// no GOTO exists, the state is returned unchanged so non-list contexts and the
// common mid-list case are untouched.
func (p *Parser) schemeErrorRecoveryState(state StateID) StateID {
	if p == nil || !p.isScheme || !p.schemeHasDatumSymbol {
		return state
	}
	gotoState := p.lookupGoto(state, p.schemeDatumSymbol)
	if gotoState == 0 || gotoState == state {
		return state
	}
	return gotoState
}

// nearestActionRecoveryLanguage reports whether in-context recovery
// (tryNearestActionStateRecovery) is enabled for the active grammar. This is
// the C ts_parser__recover candidate rule — pop to the nearest stack state
// that has an action on the current lookahead, wrap only the popped
// fragments into an ERROR, and resume in context — without C's multi-version
// cost competition. It is enabled per-grammar only after real-corpus
// verification against the C oracle, because without cost competition a
// stray delimiter (e.g. '}') can resync into a deep wrong context in
// grammars with reusable delimiters.
func (p *Parser) nearestActionRecoveryLanguage() bool {
	if p == nil || p.language == nil {
		return false
	}
	switch p.language.Name {
	case "jq":
		// jq's real corpus (src/builtin.jq) uses control expressions
		// (`if … end`) directly as object-pair values, which the grammar
		// rejects. C pops the failed value fragments to the state where `}`
		// can act, wraps them in a nested ERROR inside the pair, and the
		// surrounding function_definition completes cleanly. Verified
		// byte-faithful on the jq corpus.
		return true
	}
	return false
}

// nearestActionRecoveryMaxPop bounds how many stack entries the in-context
// recovery may pop. C's cost competition naturally prefers shallow pops; this
// bound is the cheap stand-in that keeps a stray delimiter from unwinding a
// deep, otherwise-healthy spine.
const nearestActionRecoveryMaxPop = 8

// tryNearestActionStateRecovery mirrors C ts_parser__recover's candidate
// rule for the no-action fall-through: walk the stack from the top looking
// for the nearest state (within nearestActionRecoveryMaxPop) that has an
// action on the current lookahead, pop the entries above it, wrap them into
// a single extra (arity-transparent) ERROR node pushed at that state, and
// retry the lookahead there. Returns true when it recovered.
func (p *Parser) tryNearestActionStateRecovery(s *glrStack, tok Token, nodeCount *int, arena *nodeArena, entryScratch *glrEntryScratch, gssScratch *gssScratch, trackChildErrors *bool) bool {
	if p == nil || s == nil || arena == nil || tok.Symbol == 0 || tok.Symbol == errorSymbol {
		return false
	}
	if p.noTreeBenchmarkOnly || !p.nearestActionRecoveryLanguage() {
		return false
	}
	s.ensureGSS(gssScratch)
	depth := s.depth()
	if depth < 2 {
		return false
	}
	entries := make([]stackEntry, 0, depth)
	for n := s.gss.head; n != nil; n = n.prev {
		entries = append(entries, n.entry)
	}
	// entries[0] is the top. The top state itself has no action (that is why
	// we are here); scan below it, bounded.
	recoverIdx := -1
	maxIdx := nearestActionRecoveryMaxPop
	if maxIdx >= len(entries) {
		maxIdx = len(entries) - 1
	}
	for i := 1; i <= maxIdx; i++ {
		if p.lookupActionIndex(entries[i].state, tok.Symbol) != 0 {
			recoverIdx = i
			break
		}
	}
	if recoverIdx < 0 {
		return false
	}
	// Materialize the popped fragments (entries above the recovery state) in
	// stack order (base-most first); they become the ERROR node's children.
	popped := entries[:recoverIdx]
	errChildren := make([]*Node, 0, len(popped))
	for i := len(popped) - 1; i >= 0; i-- {
		node, _ := materializeStackEntryPayloadEntryWithParser(p, arena, popped[i], materializeForRecovery, materializeForRecovery)
		if node == nil {
			return false
		}
		errChildren = append(errChildren, node)
	}
	recoverState := entries[recoverIdx].state
	if !s.truncate(depth - recoverIdx) {
		return false
	}
	// See newRecoveryParentNodeInArena: popped fragments can be transient
	// reduce parents whose .parent field is the result-time materializer's
	// map; eager link wiring here would link this ERROR under itself.
	errNode := p.newRecoveryParentNodeInArena(arena, errorSymbol, true, errChildren, 0)
	errNode.setHasError(true)
	errNode.setExtra(true)
	nodeBumpEquivVersion(errNode)
	if perfCountersEnabled {
		perfRecordErrorNode()
	}
	if trackChildErrors != nil {
		*trackChildErrors = true
	}
	errNode.preGotoState = recoverState
	errNode.parseState = recoverState
	p.pushStackNode(s, recoverState, errNode, entryScratch, gssScratch)
	if nodeCount != nil {
		*nodeCount = *nodeCount + 1
	}
	if p.glrTrace {
		fmt.Printf("      -> NEAREST-RECOVER tok=%d state=%d popped=%d depth=%d\n",
			tok.Symbol, recoverState, len(errChildren), s.depth())
	}
	return true
}

// Resync recovery status codes returned by tryResyncErrorRecovery.
const (
	resyncNone    = 0 // not handled; caller falls back to pushOrExtendErrorNode
	resyncRetry   = 1 // resynced; caller retries the action at the same token
	resyncAdvance = 2 // resynced and consumed the token; caller reads the next token
)

// resyncTopLevelLanguage reports whether the top-level panic-mode resync
// (tryResyncErrorRecovery) is enabled for the active grammar. It is scoped to
// grammars whose start symbol is a flat top-level repeat for which
// initial-state resync is validated to localize errors without regressing
// other recoveries: plain C (translation_unit) and jq (program's
// REPEAT1(function_definition)). Broadening this (e.g. to cpp/objc or other
// flat-top-level grammars) is safe only after verifying that grammar's
// language-specific recovery tests against the resync behavior; cpp in
// particular has recovery tests that lock in the existing leaf-path output, so
// it is intentionally excluded here.
func (p *Parser) resyncTopLevelLanguage() bool {
	if p == nil || p.language == nil {
		return false
	}
	switch p.language.Name {
	case "c":
		return true
	case "jq":
		// jq's start symbol `program` is a flat REPEAT1(function_definition)
		// (optionally followed by a trailing expression) — the same flat
		// top-level shape as C's translation_unit, for which initial-state
		// resync is well-behaved. Real jq sources (e.g. jq's own
		// src/builtin.jq) contain constructs the grammar rejects: an
		// `if … end` / `reduce …` / `try …` control expression used directly
		// as an object pair value, which the grammar admits only as a
		// `primary_expression` (i.e. via a postfix `?` forming an
		// optional_expression). C tree-sitter localizes those into a single
		// nested ERROR (inserting a missing `?`) while keeping the
		// surrounding program/function_definition structure intact. Without
		// resync Go collapses the whole file into a flat ERROR root; resyncing
		// to the program's initial state preserves the completed
		// function_definition siblings and contains the damage to one ERROR
		// subtree, matching C's `program` root shape.
		return true
	}
	return false
}

func (p *Parser) opportunisticTopLevelResyncAllowed(tok Token) bool {
	if p == nil || p.language == nil || tok.NoLookahead || tok.Symbol == 0 || tok.Symbol == errorSymbol {
		return false
	}
	return p.lookupActionIndex(p.language.InitialState, tok.Symbol) != 0
}

func (p *Parser) retryStructuralTopLevelResyncAdvanceAllowed(tok Token) bool {
	return p != nil &&
		p.language != nil &&
		p.retryStructuralTopLevelResync &&
		!tok.NoLookahead &&
		tok.Symbol != 0 &&
		tok.Symbol != errorSymbol &&
		tok.StartByte < tok.EndByte
}

// tryResyncErrorRecovery implements the panic-mode resync that mirrors C
// tree-sitter's ts_parser__recover for the no-action fall-through. When the
// current top state has no action for the lookahead, the parser is stuck: the
// previous behavior (pushOrExtendErrorNode at the same dead-end state) appended
// a flat ERROR leaf and kept reading at that non-progressing state, so every
// following token shredded into another top-level fragment.
//
// Instead, we pop DOWN to the grammar's top-level (initial) state, wrap the
// failed region into a SINGLE localized ERROR node while PRESERVING any
// already-completed valid top-level siblings, and resume there. The lookahead
// then has an action at (or just after) that state, so subsequent valid
// top-level constructs parse with proper nesting under the real root and the
// damage is contained to one ERROR subtree.
//
// Returns:
//   - resyncNone: no top-level frame can act on the lookahead; caller falls
//     back to pushOrExtendErrorNode (accumulate token, re-test next token).
//   - resyncRetry: resynced; caller retries the action at the same token.
//   - resyncAdvance: resynced and folded the current token into the ERROR;
//     caller advances to the next token.
func (p *Parser) tryResyncErrorRecovery(source []byte, s *glrStack, tok Token, nodeCount *int, arena *nodeArena, entryScratch *glrEntryScratch, gssScratch *gssScratch, trackChildErrors *bool) int {
	return p.tryResyncErrorRecoveryMode(source, s, tok, nodeCount, arena, entryScratch, gssScratch, trackChildErrors, false)
}

func (p *Parser) tryOpportunisticTopLevelResyncRecovery(source []byte, s *glrStack, tok Token, nodeCount *int, arena *nodeArena, entryScratch *glrEntryScratch, gssScratch *gssScratch, trackChildErrors *bool) int {
	return p.tryResyncErrorRecoveryMode(source, s, tok, nodeCount, arena, entryScratch, gssScratch, trackChildErrors, true)
}

func (p *Parser) tryResyncErrorRecoveryMode(source []byte, s *glrStack, tok Token, nodeCount *int, arena *nodeArena, entryScratch *glrEntryScratch, gssScratch *gssScratch, trackChildErrors *bool, opportunisticRetryOnly bool) int {
	if p == nil || s == nil || arena == nil || tok.Symbol == 0 {
		return resyncNone
	}
	// Only meaningful for full-tree parsing; no-tree benchmark stacks carry
	// noTreeNode payloads that this path does not reconstruct.
	if p.noTreeBenchmarkOnly {
		return resyncNone
	}
	// Scope: always-on top-level resync stays language-scoped to grammars
	// validated for it. The opportunistic path is narrower: ordinary passes may
	// only replay at a legal top-level token, while selected full-parse retries
	// may also advance through a concrete failed suffix token.
	languageScoped := p.resyncTopLevelLanguage()
	opportunisticRetryAllowed := false
	opportunisticAdvanceAllowed := false
	if !languageScoped && opportunisticRetryOnly {
		opportunisticRetryAllowed = p.opportunisticTopLevelResyncAllowed(tok)
		opportunisticAdvanceAllowed = p.retryStructuralTopLevelResyncAdvanceAllowed(tok)
	}
	if !languageScoped && (!opportunisticRetryOnly || (!opportunisticRetryAllowed && !opportunisticAdvanceAllowed)) {
		return resyncNone
	}

	// Materialize the stack entries top-to-bottom so we can both probe states
	// and collect the popped subtrees. The stack may be in either the dense
	// entries form or the GSS form.
	s.ensureGSS(gssScratch)
	depth := s.depth()
	if depth < 2 {
		return resyncNone
	}
	entries := make([]stackEntry, 0, depth)
	for n := s.gss.head; n != nil; n = n.prev {
		entries = append(entries, n.entry)
	}

	// entries[0] is the top; entries[len-1] is the base. We resync to a genuine
	// top-level recovery context: an ancestor whose state is the grammar's
	// initial state (the start-symbol repeat, e.g. translation_unit / program /
	// source_file). Resyncing there wraps the failed region into ONE localized
	// ERROR and resumes parsing the next valid top-level construct with proper
	// nesting under the real root.
	//
	// We deliberately do NOT resync to an arbitrary mid-stack state that merely
	// happens to accept the lookahead (e.g. a stray '}' or ',' that a deep
	// initializer/block production can shift): those resync into the wrong
	// context and keep the parse fragmented. Scanning from the base upward
	// selects the deepest top-level frame, containing the most damage in a
	// single ERROR subtree.
	initialState := p.language.InitialState
	recoverIdx := -1
	for i := len(entries) - 1; i >= 1; i-- {
		if entries[i].state == initialState {
			recoverIdx = i
			break
		}
	}
	if recoverIdx < 0 {
		return resyncNone
	}
	recoverState := entries[recoverIdx].state

	// Materialize the popped subtrees (entries above the recovered top-level
	// frame), in stack order (base-most popped first). These are zero or more
	// ALREADY-COMPLETED valid top-level siblings (e.g. a function_definition that
	// reduced before the failing construct began) followed by the partial/failed
	// construct that triggered the no-action.
	popped := entries[:recoverIdx] // top-first: top .. recoverIdx-1
	poppedNodes := make([]*Node, 0, len(popped))
	for i := len(popped) - 1; i >= 0; i-- { // stack order: base-most first
		node, _ := materializeStackEntryPayloadEntryWithParser(p, arena, popped[i], materializeForRecovery, materializeForRecovery)
		if node == nil {
			// A non-Node payload (no-tree leaf) slipped in; abort and let the
			// conservative fallback handle this token.
			return resyncNone
		}
		poppedNodes = append(poppedNodes, node)
	}
	if !languageScoped && opportunisticRetryOnly && opportunisticRetryAllowed && p.tryReplayTopLevelRecovery(source, s, tok, recoverState, poppedNodes, entryScratch, gssScratch) {
		if p.glrTrace {
			fmt.Printf("      -> LEGAL-REPLAY-RESYNC tok=%d recover_state=%d popped=%d depth=%d\n",
				tok.Symbol, recoverState, len(poppedNodes), s.depth())
		}
		return resyncRetry
	}
	// Split the popped span: keep the leading run of COMPLETED valid top-level
	// items as preserved siblings, and wrap the trailing failed construct into
	// ONE localized ERROR. A completed top-level item is a node that (a) the
	// running top-level state has a GOTO for, and (b) lands in a state
	// from which the parse could legally end (i.e. that state can act on the
	// EOF/end symbol). Condition (b) is what distinguishes a fully reduced
	// top-level item (after which translation_unit may accept) from a partial
	// construct's internal entries (which also have GOTOs but cannot end the
	// file). Without (b) the loop would greedily follow the failed construct's
	// own GOTO chain and land right back at the stuck state.
	endSym := Symbol(0)
	preservedEnd := 0
	gotoState := recoverState
	for preservedEnd < len(poppedNodes) {
		n := poppedNodes[preservedEnd]
		next := p.lookupGoto(gotoState, n.symbol)
		if next == 0 {
			break
		}
		if p.lookupActionIndex(next, endSym) == 0 {
			break
		}
		gotoState = next
		preservedEnd++
	}
	if preservedEnd >= len(poppedNodes) {
		// Everything popped was valid (no failed suffix to wrap). This should
		// not happen on the no-action path; bail rather than push an empty ERROR.
		return resyncNone
	}

	// Decide how to handle the current lookahead. If the post-sibling top-level
	// state can act on it, we resync and RETRY the action there (the common
	// case: the lookahead starts the next valid construct). If it cannot (e.g.
	// the lookahead is the failed construct's own terminator like ';' or '}'),
	// fold the current token into the ERROR as well and ADVANCE; the next
	// construct-starting token will then shift cleanly at the preserved
	// top-level frame. Either way the prior valid siblings are preserved.
	status := resyncRetry
	if p.lookupActionIndex(gotoState, tok.Symbol) == 0 {
		status = resyncAdvance
	}
	if !languageScoped && opportunisticRetryOnly {
		switch status {
		case resyncRetry:
			if !opportunisticRetryAllowed {
				return resyncNone
			}
		case resyncAdvance:
			if !opportunisticAdvanceAllowed {
				return resyncNone
			}
		default:
			return resyncNone
		}
	}
	if status == resyncRetry {
		if recovered, target, ok := p.rebuildAliasPrefixedRecoveredSuffix(source, gotoState, poppedNodes[preservedEnd:], tok.Symbol, arena); ok {
			keepDepth := len(entries) - recoverIdx
			if !s.truncate(keepDepth) {
				return resyncNone
			}
			pushState := recoverState
			for i := 0; i < preservedEnd; i++ {
				n := poppedNodes[i]
				next := p.lookupGoto(pushState, n.symbol)
				if next == 0 {
					return resyncNone
				}
				n.preGotoState = pushState
				n.parseState = next
				nodeBumpEquivVersion(n)
				pushState = next
				p.pushStackNode(s, next, n, entryScratch, gssScratch)
			}
			recovered.preGotoState = pushState
			recovered.parseState = target
			nodeBumpEquivVersion(recovered)
			p.pushStackNode(s, target, recovered, entryScratch, gssScratch)
			if nodeCount != nil {
				*nodeCount = *nodeCount + 1
			}
			if p.glrTrace {
				fmt.Printf("      -> RESYNC-RECOVERED-SUFFIX tok=%d state=%d target=%d depth=%d\n",
					tok.Symbol, pushState, target, s.depth())
			}
			return resyncRetry
		}
	}

	errChildren := make([]*Node, 0, len(poppedNodes)-preservedEnd+1)
	errChildren = append(errChildren, poppedNodes[preservedEnd:]...)
	if status == resyncAdvance {
		if !p.guardRealTokenAttachmentGap(source, s, tok, "resync") {
			return resyncNone
		}
		// Fold the unparseable current token into the ERROR span.
		tokLeaf := newLeafNodeInArena(arena, tok.Symbol, p.isNamedSymbol(tok.Symbol),
			tok.StartByte, tok.EndByte, tok.StartPoint, tok.EndPoint)
		tokLeaf.setHasError(true)
		tokLeaf.setExternalScannerToken(tok.ExternalScannerToken)
		errChildren = append(errChildren, tokLeaf)
	}
	// See newRecoveryParentNodeInArena: popped fragments can be transient
	// reduce parents whose .parent field is the result-time materializer's
	// map; eager link wiring here would link this ERROR under itself.
	errNode := p.newRecoveryParentNodeInArena(arena, errorSymbol, true, errChildren, 0)
	errNode.setHasError(true)
	nodeBumpEquivVersion(errNode)
	if perfCountersEnabled {
		perfRecordErrorNode()
	}
	if trackChildErrors != nil {
		*trackChildErrors = true
	}

	// Pop everything ABOVE the recovered state. entries is top-first, so the
	// recovered state at top-first index recoverIdx sits at 1-based stack depth
	// len(entries)-recoverIdx; truncating to that depth keeps the recovered
	// state and everything below it and discards the popped span above it.
	keepDepth := len(entries) - recoverIdx
	if !s.truncate(keepDepth) {
		return resyncNone
	}
	// Re-push the preserved valid top-level siblings, advancing the state via
	// GOTO exactly as the original parse did.
	pushState := recoverState
	for i := 0; i < preservedEnd; i++ {
		n := poppedNodes[i]
		next := p.lookupGoto(pushState, n.symbol)
		if next == 0 {
			// Defensive: should not happen given the preservation loop above.
			return resyncNone
		}
		n.preGotoState = pushState
		n.parseState = next
		nodeBumpEquivVersion(n)
		pushState = next
		p.pushStackNode(s, next, n, entryScratch, gssScratch)
	}
	// Push the ERROR node, keeping pushState as the top state so the lookahead
	// has an action on the next dispatch (resyncRetry) or so the next token
	// shifts cleanly (resyncAdvance).
	errNode.preGotoState = pushState
	errNode.parseState = pushState
	p.pushStackNode(s, pushState, errNode, entryScratch, gssScratch)
	if nodeCount != nil {
		*nodeCount = *nodeCount + 1
	}
	if p.glrTrace {
		fmt.Printf("      -> RESYNC tok=%d status=%d recover_state=%d preserved=%d wrapped=%d depth=%d\n",
			tok.Symbol, status, recoverState, preservedEnd, len(errChildren), s.depth())
	}
	return status
}

func (p *Parser) rebuildAliasPrefixedRecoveredSuffix(source []byte, state StateID, suffix []*Node, lookahead Symbol, arena *nodeArena) (*Node, StateID, bool) {
	if p == nil || p.language == nil || len(source) == 0 || len(suffix) != 2 {
		return nil, 0, false
	}
	prefix := suffix[0]
	recovered := suffix[1]
	if prefix == nil || recovered == nil || !recovered.hasError() || recovered.symbol == errorSymbol {
		return nil, 0, false
	}
	if nodeChildCountNoMaterialize(prefix) != 0 || nodeChildCountNoMaterialize(recovered) == 0 {
		return nil, 0, false
	}
	target := p.lookupGoto(state, recovered.symbol)
	if target == 0 || p.lookupActionIndex(target, lookahead) == 0 {
		return nil, 0, false
	}
	alt, ok := p.sameLexemeActiveAliasSymbol(source, state, prefix)
	if !ok {
		return nil, 0, false
	}

	childCount := nodeChildCountNoMaterialize(recovered)
	children := make([]*Node, 0, childCount+1)
	aliasLeaf := cloneNodeInArena(arena, prefix)
	aliasLeaf.symbol = alt
	aliasLeaf.setNamed(p.isNamedSymbol(alt))
	aliasLeaf.preGotoState = state
	if shiftTarget, ok := p.shiftTargetForStateSymbol(state, alt); ok {
		aliasLeaf.parseState = shiftTarget
	}
	nodeBumpEquivVersion(aliasLeaf)
	children = append(children, aliasLeaf)
	for i := 0; i < childCount; i++ {
		child := nodeChildAtForReason(recovered, i, materializeForRecovery)
		if child == nil {
			return nil, 0, false
		}
		cloned := cloneNodeInArena(arena, child)
		if i == 0 {
			p.restoreAnonymousTokenForRecoveredTail(source, cloned)
		}
		p.materializeAnonymousChildrenForRecoveredError(source, cloned, arena)
		children = append(children, cloned)
	}

	rebuilt := newParentNodeInArena(arena, recovered.symbol, p.isNamedSymbol(recovered.symbol), children, nil, recovered.productionID)
	rebuilt.startByte = prefix.startByte
	rebuilt.startPoint = prefix.startPoint
	rebuilt.endByte = recovered.endByte
	rebuilt.endPoint = recovered.endPoint
	rebuilt.setHasError(true)
	rebuilt.dynamicPrecedence = recovered.dynamicPrecedence
	rebuilt.rawShape = recovered.rawShape
	return rebuilt, target, true
}

func (p *Parser) materializeAnonymousChildrenForRecoveredError(source []byte, n *Node, arena *nodeArena) {
	if p == nil || p.language == nil || n == nil || n.symbol != errorSymbol || nodeChildCountNoMaterialize(n) != 0 {
		return
	}
	if n.startByte >= n.endByte || int(n.endByte) > len(source) {
		return
	}
	children := make([]*Node, 0, 4)
	for pos := int(n.startByte); pos < int(n.endByte); pos++ {
		text := bytesToStringNoCopy(source[pos : pos+1])
		var tokSym Symbol
		for _, sym := range p.language.TokenSymbolsByName(text) {
			idx := int(sym)
			if idx < 0 || idx >= len(p.language.SymbolMetadata) {
				continue
			}
			meta := p.language.SymbolMetadata[idx]
			if meta.Visible && !meta.Named {
				tokSym = sym
				break
			}
		}
		if tokSym == 0 {
			continue
		}
		startPoint := advancePointByBytes(n.startPoint, source[int(n.startByte):pos])
		endPoint := advancePointByBytes(startPoint, source[pos:pos+1])
		children = append(children, newLeafNodeInArena(arena, tokSym, false, uint32(pos), uint32(pos+1), startPoint, endPoint))
	}
	if len(children) == 0 {
		return
	}
	n.children = children
	n.setNamed(true)
	n.setHasError(true)
	nodeBumpEquivVersion(n)
}

func (p *Parser) restoreAnonymousTokenForRecoveredTail(source []byte, n *Node) {
	if p == nil || p.language == nil || n == nil || nodeChildCountNoMaterialize(n) != 0 {
		return
	}
	idx := int(n.symbol)
	if idx < 0 || idx >= len(p.language.SymbolMetadata) || n.startByte >= n.endByte || int(n.endByte) > len(source) {
		return
	}
	meta := p.language.SymbolMetadata[idx]
	if !meta.Visible || !meta.Named {
		return
	}
	text := bytesToStringNoCopy(source[n.startByte:n.endByte])
	for _, sym := range p.language.TokenSymbolsByName(text) {
		symIdx := int(sym)
		if symIdx < 0 || symIdx >= len(p.language.SymbolMetadata) {
			continue
		}
		symMeta := p.language.SymbolMetadata[symIdx]
		if symMeta.Visible && !symMeta.Named {
			n.symbol = sym
			n.setNamed(false)
			nodeBumpEquivVersion(n)
			return
		}
	}
}

func (p *Parser) sameLexemeActiveAliasSymbol(source []byte, state StateID, n *Node) (Symbol, bool) {
	if p == nil || p.language == nil || n == nil || n.startByte >= n.endByte || int(n.endByte) > len(source) {
		return 0, false
	}
	prefixName := p.symbolDisplayName(n.symbol)
	preferredBase := recoveredAliasBaseName(prefixName)
	var best Symbol
	bestScore := -1
	p.forEachActionIndexInState(state, func(sym Symbol, idx uint16) bool {
		if sym == 0 || sym == n.symbol || uint32(sym) >= p.language.TokenCount {
			return true
		}
		if !p.sameAliasTokenShape(n.symbol, sym) || p.sameSymbolName(n.symbol, sym) || !parseActionIndexHasShift(p.language, idx) {
			return true
		}
		score := 0
		name := p.symbolDisplayName(sym)
		if preferredBase != "" && name == preferredBase {
			score += 4
		}
		if name != "" && prefixName != "" && strings.HasPrefix(prefixName, name+"_") {
			score += 2
		}
		if best == 0 || score > bestScore {
			best = sym
			bestScore = score
		}
		return true
	})
	if best != 0 && bestScore > 0 {
		return best, true
	}
	return 0, false
}

func recoveredAliasBaseName(name string) string {
	for _, suffix := range []string{"_start", "_end", "_begin", "_open", "_close"} {
		if strings.HasSuffix(name, suffix) && len(name) > len(suffix) {
			return strings.TrimSuffix(name, suffix)
		}
	}
	return ""
}

func (p *Parser) symbolDisplayName(sym Symbol) string {
	if p == nil || p.language == nil {
		return ""
	}
	idx := int(sym)
	if idx < 0 || idx >= len(p.language.SymbolMetadata) {
		return ""
	}
	if name := p.language.SymbolMetadata[idx].Name; name != "" {
		return name
	}
	if idx < len(p.language.SymbolNames) {
		return p.language.SymbolNames[idx]
	}
	return ""
}

func parseActionIndexHasShift(lang *Language, idx uint16) bool {
	if lang == nil || idx == 0 || int(idx) >= len(lang.ParseActions) {
		return false
	}
	for _, act := range lang.ParseActions[idx].Actions {
		if act.Type == ParseActionShift {
			return true
		}
	}
	return false
}

func (p *Parser) sameAliasTokenShape(a, b Symbol) bool {
	if p == nil || p.language == nil {
		return false
	}
	ai := int(a)
	bi := int(b)
	if ai < 0 || bi < 0 || ai >= len(p.language.SymbolMetadata) || bi >= len(p.language.SymbolMetadata) {
		return false
	}
	am := p.language.SymbolMetadata[ai]
	bm := p.language.SymbolMetadata[bi]
	return am.Visible && bm.Visible && am.Named == bm.Named && uint32(ai) < p.language.TokenCount && uint32(bi) < p.language.TokenCount
}

func (p *Parser) tryReplayTopLevelRecovery(source []byte, s *glrStack, tok Token, recoverState StateID, poppedNodes []*Node, entryScratch *glrEntryScratch, gssScratch *gssScratch) bool {
	if p == nil || s == nil || p.language == nil || len(poppedNodes) == 0 {
		return false
	}
	for _, n := range poppedNodes {
		if n == nil || n.hasError() || n.isMissing() {
			return false
		}
	}
	split, targets, ok := p.findTopLevelReplaySplit(source, recoverState, poppedNodes, tok.Symbol)
	if !ok {
		return false
	}
	state := recoverState
	depth := s.depth()
	if !s.truncate(depth - len(poppedNodes)) {
		return false
	}
	for i := 0; i < split; i++ {
		n := poppedNodes[i]
		n.setExtra(true)
		n.preGotoState = state
		n.parseState = state
		nodeBumpEquivVersion(n)
		p.pushStackNode(s, state, n, entryScratch, gssScratch)
	}
	for i, n := range poppedNodes[split:] {
		next := targets[i]
		n.preGotoState = state
		n.parseState = next
		nodeBumpEquivVersion(n)
		state = next
		p.pushStackNode(s, state, n, entryScratch, gssScratch)
	}
	return true
}

func (p *Parser) findTopLevelReplaySplit(source []byte, recoverState StateID, poppedNodes []*Node, lookahead Symbol) (int, []StateID, bool) {
	// The fully degenerate split (split == len(poppedNodes), i.e. a
	// zero-length suffix) leaves EVERY popped node unchased: none of them
	// are validated by walking the automaton, they are simply re-attached
	// as unvalidated "extra" top-level siblings (see tryReplayTopLevelRecovery).
	// That trivially succeeds whenever the lookahead can merely START a new
	// top-level construct (already checked by the caller), regardless of
	// whether the popped fragments ever combined into anything grammatically
	// complete — which silently discards a real parse failure (e.g. a class
	// body missing its terminating ';') instead of wrapping it in an ERROR.
	//
	// Every OTHER split (< len(poppedNodes)) requires its suffix to walk a
	// real GOTO/SHIFT chain through the automaton and land in a state that
	// accepts the lookahead — that is itself a meaningful validity check, so
	// it stays unrestricted; it is what correctly rescues legitimate GLR
	// ambiguity-resolution dead ends (e.g. constructor-name-vs-declaration
	// forks that transiently merge into a state that can't see the next
	// token, such as `extern std::string foo();`) without ever touching an
	// ERROR node.
	//
	// We only gate the fully degenerate case, and only on AFFIRMATIVE
	// evidence of truncation: the popped span's source text contains a
	// brace ('{' or '}'). This closes the brace-carrying subclass of
	// truncation only — a class/struct/function body that closed cleanly
	// but whose enclosing declaration never reached a legal end (e.g. a
	// missing ';' after `class C { ... }`). It is NOT a general truncation
	// detector: brace-free top-level truncations (a bodyless
	// `class C : public D` missing its ';', a truncated `using X = int`,
	// an un-terminated template forward declaration, etc.) carry no brace
	// in the popped span, so the degenerate split is still accepted and
	// those cases still false-clean — identical, not worse than, base
	// behavior before this change; not fixed by it either. Empirically
	// (LLVM + tree-sitter-cpp corpus walk), legitimate GLR-rescue popped
	// spans (e.g. a constructor-name-vs-declaration fork that transiently
	// dead-ends, `extern std::string foo();`) are brace-free, so this
	// narrower check leaves that rescue path intact.
	//
	// The scan is a raw byte search, not token-aware: a '{'/'}' inside a
	// string or comment literal within the popped span would also count as
	// "evidence" and could in principle cause an over-eager rejection of a
	// degenerate split that was otherwise legal. In practice this has not
	// been observed to cause a false rejection on valid code (~20 adversarial
	// brace-in-string/comment probes against the corpus produced zero
	// flips): whenever the popped span is genuinely valid, a smaller,
	// non-degenerate split with a real GOTO/SHIFT-validated suffix chain
	// almost always exists and is tried first (the loop below tries
	// split=0..len(poppedNodes) in order), so the degenerate case is only
	// ever reached — and only ever needs gating — when it is the sole
	// candidate. A pathological grammar/input combination where the ONLY
	// viable split is the degenerate one AND the popped span's only brace
	// is inside a string/comment remains a theoretical false-rejection
	// gap; it has not been demonstrated in practice.
	degenerate := len(poppedNodes)
	rejectDegenerate := poppedSpanHasBraceEvidence(source, poppedNodes)
	for split := 0; split <= len(poppedNodes); split++ {
		if split == degenerate && rejectDegenerate {
			continue
		}
		state := recoverState
		targets := make([]StateID, 0, len(poppedNodes)-split)
		ok := true
		for i := split; i < len(poppedNodes); i++ {
			next, replayOK := p.replayTopLevelNodeTarget(state, poppedNodes[i])
			if !replayOK {
				ok = false
				break
			}
			targets = append(targets, next)
			state = next
		}
		if ok && p.lookupActionIndex(state, lookahead) != 0 {
			return split, targets, true
		}
	}
	return 0, nil, false
}

// poppedSpanHasBraceEvidence reports whether any popped node's source span
// contains a '{' or '}' byte. See findTopLevelReplaySplit for why this is
// used as affirmative evidence that a degenerate (fully unvalidated) replay
// would silently swallow a genuine truncation rather than rescue a spurious
// GLR dead end.
func poppedSpanHasBraceEvidence(source []byte, poppedNodes []*Node) bool {
	for _, n := range poppedNodes {
		if n == nil {
			continue
		}
		s, e := n.startByte, n.endByte
		if e > uint32(len(source)) || s >= e {
			continue
		}
		if bytes.IndexByte(source[s:e], '{') >= 0 || bytes.IndexByte(source[s:e], '}') >= 0 {
			return true
		}
	}
	return false
}

func (p *Parser) replayTopLevelNodeTarget(state StateID, n *Node) (StateID, bool) {
	if p == nil || n == nil {
		return 0, false
	}
	if len(n.children) == 0 {
		if target, ok := p.shiftTargetForStateSymbol(state, n.symbol); ok {
			return target, true
		}
	}
	if target := p.lookupGoto(state, n.symbol); target != 0 {
		return target, true
	}
	return 0, false
}

func (p *Parser) shiftTargetForStateSymbol(state StateID, sym Symbol) (StateID, bool) {
	if p == nil || p.language == nil {
		return 0, false
	}
	idx := p.lookupActionIndex(state, sym)
	if idx == 0 || int(idx) >= len(p.language.ParseActions) {
		return 0, false
	}
	for _, act := range p.language.ParseActions[idx].Actions {
		if act.Type == ParseActionShift {
			return extraShiftTargetState(state, act), true
		}
	}
	return 0, false
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

func (p *Parser) recoverReduceChainCycle(source []byte, s *glrStack, state StateID, tok Token, nodeCount *int, arena *nodeArena, entryScratch *glrEntryScratch, gssScratch *gssScratch, trackChildErrors *bool) bool {
	if tok.NoLookahead || s == nil || s.dead || s.accepted {
		return false
	}
	if tok.Symbol == 0 {
		if p.errorCostCompetitionEnabled() && tok.StartByte == tok.EndByte {
			s.cPaused = true
		}
		return false
	}
	if !p.guardRealTokenAttachmentGap(source, s, tok, "reduce-chain-cycle") {
		return false
	}
	p.pushOrExtendErrorNode(s, state, tok, nodeCount, arena, entryScratch, gssScratch, trackChildErrors)
	return true
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

func (p *Parser) chainSingleReduceActions(source []byte, s *glrStack, tok Token, anyReduced *bool, nodeCount *int, arena *nodeArena, entryScratch *glrEntryScratch, gssScratch *gssScratch, tmpEntries *[]stackEntry, deferParentLinks bool, trackChildErrors *bool) bool {
	if s == nil || s.dead || s.accepted || s.shifted {
		return false
	}
	if p.ambiguityProfile != nil {
		return p.chainSingleReduceActionsProfiled(source, s, tok, anyReduced, nodeCount, arena, entryScratch, gssScratch, tmpEntries, deferParentLinks, trackChildErrors)
	}
	if len(p.classifiedActions) == len(p.language.ParseActions) {
		return p.chainSingleReduceActionsClassified(source, s, tok, anyReduced, nodeCount, arena, entryScratch, gssScratch, tmpEntries, deferParentLinks, trackChildErrors)
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
				p.noteStopActionDiagnostic("reduce-chain-cycle", s, tok, next, 1, true, chainLen+1, repeatedSigCount, true)
				return p.recoverReduceChainCycle(source, s, currentState, tok, nodeCount, arena, entryScratch, gssScratch, trackChildErrors)
			}
			chainLen++
			if perfCountersEnabled {
				perfRecordReduceChainStep(chainLen)
			}
			p.noteStopActionDiagnostic("reduce-chain", s, tok, next, 1, true, chainLen, repeatedSigCount, false)
			p.applyReduceActionDispatch(source, s, next, tok, anyReduced, nodeCount, arena, entryScratch, gssScratch, tmpEntries, deferParentLinks, trackChildErrors)
			p.noteStopActionResult(s)
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

func (p *Parser) chainSingleReduceActionsClassified(source []byte, s *glrStack, tok Token, anyReduced *bool, nodeCount *int, arena *nodeArena, entryScratch *glrEntryScratch, gssScratch *gssScratch, tmpEntries *[]stackEntry, deferParentLinks bool, trackChildErrors *bool) bool {
	if len(p.reduceChainHints) != 0 {
		if hint, ok := p.reduceChainHintFor(s.top().state, tok.Symbol); ok {
			return p.chainSingleReduceActionsClassifiedHinted(source, s, tok, hint, anyReduced, nodeCount, arena, entryScratch, gssScratch, tmpEntries, deferParentLinks, trackChildErrors)
		}
	}
	return p.chainSingleReduceActionsClassifiedDefault(source, s, tok, anyReduced, nodeCount, arena, entryScratch, gssScratch, tmpEntries, deferParentLinks, trackChildErrors)
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

func (p *Parser) chainSingleReduceActionsClassifiedHinted(source []byte, s *glrStack, tok Token, hint reduceChainHint, anyReduced *bool, nodeCount *int, arena *nodeArena, entryScratch *glrEntryScratch, gssScratch *gssScratch, tmpEntries *[]stackEntry, deferParentLinks bool, trackChildErrors *bool) bool {
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
		p.noteStopActionDiagnostic("reduce-chain-hint", s, tok, classified.action, 1, true, steps, 0, false)
		p.applyReduceActionDispatch(source, s, classified.action, tok, anyReduced, nodeCount, arena, entryScratch, gssScratch, tmpEntries, deferParentLinks, trackChildErrors)
		p.noteStopActionResult(s)
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
	return p.chainSingleReduceActionsClassifiedDefault(source, s, tok, anyReduced, nodeCount, arena, entryScratch, gssScratch, tmpEntries, deferParentLinks, trackChildErrors)
}

func (p *Parser) chainSingleReduceActionsClassifiedDefault(source []byte, s *glrStack, tok Token, anyReduced *bool, nodeCount *int, arena *nodeArena, entryScratch *glrEntryScratch, gssScratch *gssScratch, tmpEntries *[]stackEntry, deferParentLinks bool, trackChildErrors *bool) bool {
	if p.noResultCompatibilityBenchmarkOnly {
		return p.chainSingleReduceActionsClassifiedBenchmarkOnly(source, s, tok, anyReduced, nodeCount, arena, entryScratch, gssScratch, tmpEntries, deferParentLinks, trackChildErrors)
	}
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
				p.noteStopActionDiagnostic("reduce-chain-cycle", s, tok, next, 1, true, chainLen+1, repeatedSigCount, true)
				return p.recoverReduceChainCycle(source, s, currentState, tok, nodeCount, arena, entryScratch, gssScratch, trackChildErrors)
			}
			chainLen++
			if perfCountersEnabled {
				perfRecordReduceChainStep(chainLen)
			}
			p.noteStopActionDiagnostic("reduce-chain", s, tok, next, 1, true, chainLen, repeatedSigCount, false)
			p.applyReduceActionDispatch(source, s, next, tok, anyReduced, nodeCount, arena, entryScratch, gssScratch, tmpEntries, deferParentLinks, trackChildErrors)
			p.noteStopActionResult(s)
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

func (p *Parser) chainSingleReduceActionsClassifiedBenchmarkOnly(source []byte, s *glrStack, tok Token, anyReduced *bool, nodeCount *int, arena *nodeArena, entryScratch *glrEntryScratch, gssScratch *gssScratch, tmpEntries *[]stackEntry, deferParentLinks bool, trackChildErrors *bool) bool {
	const maxInlineReduceChain = 256
	actions := p.classifiedActions
	for chainLen := 0; chainLen < maxInlineReduceChain; {
		actionIdx := p.lookupActionIndex(s.top().state, tok.Symbol)
		if actionIdx == 0 || int(actionIdx) >= len(actions) {
			return false
		}

		classified := &actions[actionIdx]
		switch classified.class {
		case classifiedParseActionSingleReduce:
			chainLen++
			if perfCountersEnabled {
				perfRecordReduceChainStep(chainLen)
			}
			p.applyReduceActionDispatch(source, s, classified.action, tok, anyReduced, nodeCount, arena, entryScratch, gssScratch, tmpEntries, deferParentLinks, trackChildErrors)
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

func (p *Parser) chainSingleReduceActionsProfiled(source []byte, s *glrStack, tok Token, anyReduced *bool, nodeCount *int, arena *nodeArena, entryScratch *glrEntryScratch, gssScratch *gssScratch, tmpEntries *[]stackEntry, deferParentLinks bool, trackChildErrors *bool) bool {
	if len(p.classifiedActions) == len(p.language.ParseActions) {
		return p.chainSingleReduceActionsClassifiedProfiled(source, s, tok, anyReduced, nodeCount, arena, entryScratch, gssScratch, tmpEntries, deferParentLinks, trackChildErrors)
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
				p.noteStopActionDiagnostic("reduce-chain-cycle", s, tok, next, 1, true, chainLen+1, repeatedSigCount, true)
				return p.recoverReduceChainCycle(source, s, currentState, tok, nodeCount, arena, entryScratch, gssScratch, trackChildErrors)
			}
			chainLen++
			if perfCountersEnabled {
				perfRecordReduceChainStep(chainLen)
			}
			reduceStart := time.Now()
			p.noteStopActionDiagnostic("reduce-chain-profiled", s, tok, next, 1, true, chainLen, repeatedSigCount, false)
			p.applyReduceActionDispatch(source, s, next, tok, anyReduced, nodeCount, arena, entryScratch, gssScratch, tmpEntries, deferParentLinks, trackChildErrors)
			p.noteStopActionResult(s)
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

func (p *Parser) chainSingleReduceActionsClassifiedProfiled(source []byte, s *glrStack, tok Token, anyReduced *bool, nodeCount *int, arena *nodeArena, entryScratch *glrEntryScratch, gssScratch *gssScratch, tmpEntries *[]stackEntry, deferParentLinks bool, trackChildErrors *bool) bool {
	if len(p.reduceChainHints) != 0 {
		if hint, ok := p.reduceChainHintFor(s.top().state, tok.Symbol); ok {
			return p.chainSingleReduceActionsClassifiedHintedProfiled(source, s, tok, hint, anyReduced, nodeCount, arena, entryScratch, gssScratch, tmpEntries, deferParentLinks, trackChildErrors)
		}
	}
	return p.chainSingleReduceActionsClassifiedProfiledDefault(source, s, tok, anyReduced, nodeCount, arena, entryScratch, gssScratch, tmpEntries, deferParentLinks, trackChildErrors)
}

func (p *Parser) chainSingleReduceActionsClassifiedHintedProfiled(source []byte, s *glrStack, tok Token, hint reduceChainHint, anyReduced *bool, nodeCount *int, arena *nodeArena, entryScratch *glrEntryScratch, gssScratch *gssScratch, tmpEntries *[]stackEntry, deferParentLinks bool, trackChildErrors *bool) bool {
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
		p.noteStopActionDiagnostic("reduce-chain-hint-profiled", s, tok, next, 1, true, chainLen, 0, false)
		p.applyReduceActionDispatch(source, s, next, tok, anyReduced, nodeCount, arena, entryScratch, gssScratch, tmpEntries, deferParentLinks, trackChildErrors)
		p.noteStopActionResult(s)
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
	return p.chainSingleReduceActionsClassifiedProfiledDefault(source, s, tok, anyReduced, nodeCount, arena, entryScratch, gssScratch, tmpEntries, deferParentLinks, trackChildErrors)
}

func (p *Parser) chainSingleReduceActionsClassifiedProfiledDefault(source []byte, s *glrStack, tok Token, anyReduced *bool, nodeCount *int, arena *nodeArena, entryScratch *glrEntryScratch, gssScratch *gssScratch, tmpEntries *[]stackEntry, deferParentLinks bool, trackChildErrors *bool) bool {
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
				p.noteStopActionDiagnostic("reduce-chain-cycle", s, tok, next, 1, true, chainLen+1, repeatedSigCount, true)
				return p.recoverReduceChainCycle(source, s, currentState, tok, nodeCount, arena, entryScratch, gssScratch, trackChildErrors)
			}
			chainLen++
			if perfCountersEnabled {
				perfRecordReduceChainStep(chainLen)
			}
			reduceStart := time.Now()
			p.noteStopActionDiagnostic("reduce-chain-profiled", s, tok, next, 1, true, chainLen, repeatedSigCount, false)
			p.applyReduceActionDispatch(source, s, next, tok, anyReduced, nodeCount, arena, entryScratch, gssScratch, tmpEntries, deferParentLinks, trackChildErrors)
			p.noteStopActionResult(s)
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
func (p *Parser) applyAction(source []byte, s *glrStack, act ParseAction, tok Token, anyReduced *bool, nodeCount *int, arena *nodeArena, entryScratch *glrEntryScratch, gssScratch *gssScratch, tmpEntries *[]stackEntry, deferParentLinks bool, trackChildErrors *bool) {
	if p != nil && p.glrTrace && s != nil {
		fmt.Printf("    APPLY type=%d cur_state=%d tok=%d act_state=%d act_sym=%d act_cnt=%d extra=%v rep=%v depth=%d\n",
			act.Type, s.top().state, tok.Symbol, act.State, act.Symbol, act.ChildCount, act.Extra, act.Repetition, s.depth())
	}
	switch act.Type {
	case ParseActionShift:
		p.applyShiftAction(s, act, tok, nodeCount, arena, entryScratch, gssScratch, trackChildErrors)

	case ParseActionReduce:
		p.applyReduceActionDispatch(source, s, act, tok, anyReduced, nodeCount, arena, entryScratch, gssScratch, tmpEntries, deferParentLinks, trackChildErrors)

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
			leaf.setExternalScannerToken(tok.ExternalScannerToken)
			leaf.preGotoState = currentState
			leaf.parseState = targetState
			p.pushStackCompactCheckpointLeaf(s, targetState, leaf, entryScratch, gssScratch)
		} else {
			leaf := newNoTreeLeafNodeInArena(arena, tok.Symbol, named,
				tok.StartByte, tok.EndByte, tok.StartPoint, tok.EndPoint)
			leaf.setExtra(extra)
			leaf.setExternalScannerToken(tok.ExternalScannerToken)
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
		leaf.setExternalScannerToken(tok.ExternalScannerToken)
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
		if tok.ExternalScannerToken {
			flags |= nodeFlagExternalScannerToken
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
		leaf.setExternalScannerToken(tok.ExternalScannerToken)
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

func (p *Parser) applyReduceActionDispatch(source []byte, s *glrStack, act ParseAction, tok Token, anyReduced *bool, nodeCount *int, arena *nodeArena, entryScratch *glrEntryScratch, gssScratch *gssScratch, tmpEntries *[]stackEntry, deferParentLinks bool, trackChildErrors *bool) {
	entries := s.entries
	borrowed := false
	if entries == nil {
		if !s.cacheEntries && s.gss.head != nil {
			tmp := []stackEntry(nil)
			if tmpEntries != nil {
				tmp = *tmpEntries
			}
			if p != nil && p.reduceScratch != nil && p.reduceScratch.transientParents != nil {
				p.applyReduceActionFromGSSTransientParents(source, s, act, tok, anyReduced, nodeCount, arena, entryScratch, gssScratch, tmpEntries, tmp, deferParentLinks, trackChildErrors != nil && *trackChildErrors)
			} else {
				p.applyReduceActionFromGSS(source, s, act, tok, anyReduced, nodeCount, arena, entryScratch, gssScratch, tmpEntries, tmp, deferParentLinks, trackChildErrors != nil && *trackChildErrors)
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
		p.applyReduceActionTransientParents(source, s, act, tok, anyReduced, nodeCount, arena, entryScratch, gssScratch, entries, deferParentLinks, trackChildErrors != nil && *trackChildErrors)
	} else {
		p.applyReduceAction(source, s, act, tok, anyReduced, nodeCount, arena, entryScratch, gssScratch, entries, deferParentLinks, trackChildErrors != nil && *trackChildErrors)
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

type reduceFork struct {
	window   []stackEntry
	topState StateID
	popTo    *gssNode
}

func reduceForksSameChildSelectionGroup(a, b reduceFork) bool {
	return a.popTo == b.popTo && a.topState == b.topState
}

func (p *Parser) selectReduceForkChildren(arena *nodeArena, act ParseAction, forks []reduceFork) []reduceFork {
	if len(forks) < 2 {
		return forks
	}
	out := forks[:0]
	for _, fork := range forks {
		keep := true
		insertAt := -1
		for i := 0; i < len(out); {
			if !reduceForksSameChildSelectionGroup(out[i], fork) {
				i++
				continue
			}
			switch p.reduceForkWindowPreference(arena, act, fork, out[i]) {
			case -1:
				if insertAt < 0 {
					insertAt = i
				}
				out = append(out[:i], out[i+1:]...)
			case 1:
				keep = false
				i = len(out)
			default:
				i++
			}
		}
		if keep {
			out = insertReduceFork(out, insertAt, fork)
		}
	}
	return out
}

func insertReduceFork(forks []reduceFork, index int, fork reduceFork) []reduceFork {
	if index < 0 || index >= len(forks) {
		return append(forks, fork)
	}
	forks = append(forks, reduceFork{})
	copy(forks[index+1:], forks[index:])
	forks[index] = fork
	return forks
}

func (p *Parser) reduceForkWindowPreference(arena *nodeArena, act ParseAction, a, b reduceFork) int {
	aEnd := reducedEndBeforeTrailingExtras(a.window)
	bEnd := reducedEndBeforeTrailingExtras(b.window)
	aParent := p.reduceForkTemporaryParent(arena, act, a.window[:aEnd])
	bParent := p.reduceForkTemporaryParent(arena, act, b.window[:bEnd])
	if aParent != nil && bParent != nil {
		aEntry := newStackEntryNode(aParent.parseState, aParent)
		bEntry := newStackEntryNode(bParent.parseState, bParent)
		if ac, bc := p.rawStackEntryErrorCost(arena, aEntry), p.rawStackEntryErrorCost(arena, bEntry); ac != bc {
			if ac < bc {
				return -1
			}
			return 1
		}
		if ad, bd := aParent.dynamicPrecedence, bParent.dynamicPrecedence; ad != bd {
			if ad > bd {
				return -1
			}
			return 1
		}
		if ac := p.rawStackEntryErrorCost(arena, aEntry); ac > 0 {
			return -1
		}
		if cmp := p.compareRawStackEntries(arena, aEntry, bEntry); cmp != 0 {
			if cmp < 0 {
				return -1
			}
			return 1
		}
		return 1
	}
	ac := p.reduceForkTemporaryParentErrorCost(arena, act.Symbol, a.window[:aEnd])
	bc := p.reduceForkTemporaryParentErrorCost(arena, act.Symbol, b.window[:bEnd])
	if ac != bc {
		if ac < bc {
			return -1
		}
		return 1
	}
	if ad, bd := reduceWindowDynamicPrecedence(a.window, 0, aEnd, act), reduceWindowDynamicPrecedence(b.window, 0, bEnd, act); ad != bd {
		if ad > bd {
			return -1
		}
		return 1
	}
	if ac > 0 {
		return -1
	}
	if cmp := p.compareRawReduceWindows(arena, a.window[:aEnd], b.window[:bEnd]); cmp != 0 {
		if cmp < 0 {
			return -1
		}
		return 1
	}
	return 1
}

func (p *Parser) reduceForkTemporaryParent(arena *nodeArena, act ParseAction, entries []stackEntry) *Node {
	if p == nil || p.language == nil {
		return nil
	}
	childCount := int(act.ChildCount)
	children, fieldIDs, fieldSources, _ := p.buildReduceChildrenWithPath(entries, 0, len(entries), childCount, act.Symbol, act.ProductionID, arena)
	parent := newParentNodeInArenaNoLinksWithFieldSources(arena, act.Symbol, p.isNamedSymbol(act.Symbol), children, fieldIDs, fieldSources, act.ProductionID, false)
	if parent == nil {
		return nil
	}
	parent.rawShape = p.captureRawShape(arena, act.Symbol, act.ProductionID, entries, 0, len(entries))
	setReduceNodeDynamicPrecedence(parent, entries, 0, len(entries), act)
	return parent
}

func (p *Parser) reduceForkTemporaryParentErrorCost(arena *nodeArena, parentSymbol Symbol, entries []stackEntry) uint32 {
	var cost uint32
	for i := range entries {
		cost += p.rawStackEntryErrorCost(arena, entries[i])
	}
	if parentSymbol == errorSymbol {
		var startByte, endByte uint32
		var startPoint, endPoint Point
		for i, found := 0, false; i < len(entries); i++ {
			if !stackEntryHasNode(entries[i]) {
				continue
			}
			if !found {
				startByte = stackEntryNodeStartByte(entries[i])
				startPoint = stackEntryNodeStartPoint(entries[i])
				found = true
			}
			endByte = stackEntryNodeEndByte(entries[i])
			endPoint = stackEntryNodeEndPoint(entries[i])
			if stackEntryNodeIsExtra(entries[i]) {
				continue
			}
			if stackEntryNodeSymbol(entries[i]) == errorSymbol && stackEntryNodeChildCount(entries[i]) == 0 {
				continue
			}
			if cSymbolVisibleLang(p.language, stackEntryNodeSymbol(entries[i])) {
				cost += cErrCostPerSkippedTree
			} else if count := p.rawStackEntryVisibleChildCount(arena, entries[i]); count > 0 {
				cost += cErrCostPerSkippedTree * uint32(count)
			}
		}
		bytes := uint32(0)
		rows := uint32(0)
		if endByte > startByte {
			bytes = endByte - startByte
		}
		if endPoint.Row > startPoint.Row {
			rows = endPoint.Row - startPoint.Row
		}
		cost += cErrCostPerRecovery + cErrCostPerSkippedChar*bytes + cErrCostPerSkippedLine*rows
	}
	return cost
}

func (p *Parser) rawStackEntryErrorCost(arena *nodeArena, entry stackEntry) uint32 {
	if !stackEntryHasNode(entry) {
		return 0
	}
	if stackEntryNodeIsMissing(entry) && stackEntryNodeChildCount(entry) == 0 {
		return cErrCostPerMissingTree + cErrCostPerRecovery
	}
	var cost uint32
	childCount := stackEntryNodeChildCount(entry)
	for i := 0; i < childCount; i++ {
		child, ok := rawStackEntryChildAt(arena, entry, i)
		if !ok {
			continue
		}
		cost += p.rawStackEntryErrorCost(arena, child)
	}
	if stackEntryNodeSymbol(entry) == errorSymbol {
		for i := 0; i < childCount; i++ {
			child, ok := rawStackEntryChildAt(arena, entry, i)
			if !ok || stackEntryNodeIsExtra(child) {
				continue
			}
			if stackEntryNodeSymbol(child) == errorSymbol && stackEntryNodeChildCount(child) == 0 {
				continue
			}
			if cSymbolVisibleLang(p.language, stackEntryNodeSymbol(child)) {
				cost += cErrCostPerSkippedTree
			} else if count := p.rawStackEntryVisibleChildCount(arena, child); count > 0 {
				cost += cErrCostPerSkippedTree * uint32(count)
			}
		}
		bytes := uint32(0)
		rows := uint32(0)
		if endByte, startByte := stackEntryNodeEndByte(entry), stackEntryNodeStartByte(entry); endByte > startByte {
			bytes = endByte - startByte
		}
		if endPoint, startPoint := stackEntryNodeEndPoint(entry), stackEntryNodeStartPoint(entry); endPoint.Row > startPoint.Row {
			rows = endPoint.Row - startPoint.Row
		}
		cost += cErrCostPerRecovery + cErrCostPerSkippedChar*bytes + cErrCostPerSkippedLine*rows
	}
	return cost
}

func (p *Parser) rawStackEntryVisibleChildCount(arena *nodeArena, entry stackEntry) int {
	if !stackEntryHasNode(entry) {
		return 0
	}
	if cSymbolVisibleLang(p.language, stackEntryNodeSymbol(entry)) {
		return 1
	}
	count := 0
	for i, n := 0, stackEntryNodeChildCount(entry); i < n; i++ {
		child, ok := rawStackEntryChildAt(arena, entry, i)
		if !ok {
			continue
		}
		count += p.rawStackEntryVisibleChildCount(arena, child)
	}
	return count
}

func (p *Parser) compareRawReduceWindows(arena *nodeArena, a, b []stackEntry) int {
	if len(a) != len(b) {
		if len(a) < len(b) {
			return -1
		}
		return 1
	}
	for i := range a {
		if cmp := p.compareRawStackEntries(arena, a[i], b[i]); cmp != 0 {
			return cmp
		}
	}
	return 0
}

func stackEntryDynamicPrecedence(entry stackEntry) int32 {
	if n := stackEntryNode(entry); n != nil {
		return n.dynamicPrecedence
	}
	if n := stackEntryNoTreeNode(entry); n != nil {
		return n.dynamicPrecedence
	}
	if n := stackEntryCompactFullLeaf(entry); n != nil {
		return n.dynamicPrecedence
	}
	if n := stackEntryPendingParent(entry); n != nil {
		return n.dynamicPrecedence
	}
	return 0
}

func addStackEntryDynamicPrecedence(entry *stackEntry, delta int16) {
	if entry == nil || delta == 0 {
		return
	}
	if n := stackEntryNode(*entry); n != nil {
		n.dynamicPrecedence += int32(delta)
		nodeBumpEquivVersion(n)
		return
	}
	if n := stackEntryNoTreeNode(*entry); n != nil {
		n.dynamicPrecedence += int32(delta)
		return
	}
	if n := stackEntryCompactFullLeaf(*entry); n != nil {
		n.dynamicPrecedence += int32(delta)
		return
	}
	if n := stackEntryPendingParent(*entry); n != nil {
		n.dynamicPrecedence += int32(delta)
	}
}

func reduceWindowDynamicPrecedence(entries []stackEntry, start, end int, act ParseAction) int32 {
	if start < 0 {
		start = 0
	}
	if end > len(entries) {
		end = len(entries)
	}
	var dyn int32
	for i := start; i < end; i++ {
		dyn += stackEntryDynamicPrecedence(entries[i])
	}
	return dyn + int32(act.DynamicPrecedence)
}

func setReduceNodeDynamicPrecedence(n *Node, entries []stackEntry, start, end int, act ParseAction) {
	if n != nil {
		n.dynamicPrecedence = reduceWindowDynamicPrecedence(entries, start, end, act)
	}
}

func (p *Parser) compareRawStackEntries(arena *nodeArena, a, b stackEntry) int {
	return p.compareRawStackEntriesRec(arena, a, b, 0)
}

func (p *Parser) compareRawStackEntriesRec(arena *nodeArena, a, b stackEntry, depth int) int {
	if depth > maxTreeWalkDepth {
		return 0
	}
	if stackEntryHasNode(a) != stackEntryHasNode(b) {
		if !stackEntryHasNode(a) {
			return -1
		}
		return 1
	}
	if !stackEntryHasNode(a) {
		return 0
	}
	aShape, aHasShape := rawShapeForStackEntry(arena, a)
	bShape, bHasShape := rawShapeForStackEntry(arena, b)
	if aHasShape != bHasShape {
		// A one-sided sidecar means the exact raw comparison lacks data. Fall
		// back to materialized/pending views below instead of inventing order.
		aHasShape = false
		bHasShape = false
	}
	as, bs := stackEntryNodeSymbol(a), stackEntryNodeSymbol(b)
	ac, bc := stackEntryNodeChildCount(a), stackEntryNodeChildCount(b)
	if aHasShape && bHasShape {
		as, bs = aShape.symbol, bShape.symbol
		ac, bc = int(aShape.childCount), int(bShape.childCount)
	}
	if as != bs {
		if as < bs {
			return -1
		}
		return 1
	}
	if p.symbolIsGeneratedRepeatAux(as) &&
		stackEntryNodeStartByte(a) == stackEntryNodeStartByte(b) &&
		stackEntryNodeEndByte(a) != stackEntryNodeEndByte(b) {
		if stackEntryNodeEndByte(a) < stackEntryNodeEndByte(b) {
			return -1
		}
		return 1
	}
	if ac != bc {
		if ac < bc {
			return -1
		}
		return 1
	}
	for i := 0; i < ac; i++ {
		achild, aok := rawStackEntryChildAt(arena, a, i)
		bchild, bok := rawStackEntryChildAt(arena, b, i)
		if aok != bok {
			if !aok {
				return -1
			}
			return 1
		}
		if !aok {
			continue
		}
		if rawStackEntryChildPairHashEqual(arena, achild, bchild) {
			// Bottom-up fingerprint match (see rawShape.contentHash /
			// rawShapeComputeContentHash): this child's subtree is treated as
			// interchangeable with its counterpart under the same
			// negligible-collision hash tradeoff documented on
			// rawStackEntryChildPairHashEqual, so the recursive comparison
			// below is skipped rather than re-walked. This is what keeps this
			// comparison (used both for raw-shape tie-break ordering and, via
			// the forest link-cap eviction path, for scoring every new
			// alternative against capped residents) bounded on shapes with a
			// long shared prefix — e.g. C# designer-style blocks of hundreds
			// of near-identical `this.fieldN = ...;` statements, where each
			// reduce compares the whole accumulated statement list against a
			// candidate that differs only in its newest, still-being-parsed
			// statement.
			continue
		}
		cmp := p.compareRawStackEntriesRec(arena, achild, bchild, depth+1)
		if cmp != 0 {
			return cmp
		}
	}
	return 0
}

// rawStackEntryChildPairHashEqual reports whether a and b are TREATED AS
// equal (cmp==0) under compareRawStackEntriesRec, in O(1) and without walking
// their subtrees. This is not a proof of equality — it is a probabilistic
// shortcut: it requires BOTH sides to carry a captured raw shape with
// matching symbol, childCount and contentHash (see
// rawShapeComputeContentHash), AND matching start/end spans on the entries
// themselves, and then accepts that combination as sufficient. contentHash is
// a 64-bit FNV-1a fingerprint, so a match here carries the same
// negligible-collision (~2^-64) risk of a false positive that the package's
// existing GSS merge-key hashing already accepts for equivalence decisions
// (see rawShapeComputeContentHash's doc comment, and gssHashSeed/
// gssHashPrime in glr_gss.go) — it is deliberately NOT re-verified by the
// exact recursive walk. Should a collision ever occur, its only effect is on
// this comparison's caller (raw-shape tie-break ordering and forest link-cap
// eviction scoring): two subtrees that are not actually identical could be
// treated as interchangeable for ambiguity/ordering purposes. It can never
// affect memory safety — this result only feeds comparison/ordering
// decisions, never indexing, allocation, or bounds. The span check exists
// because compareRawStackEntriesRec's "generated repeat aux" tie-break reads
// the live entry's own span rather than shape data; requiring equal spans
// here falsifies that tie-break's precondition
// (stackEntryNodeEndByte(a) != stackEntryNodeEndByte(b)), so skipping is safe
// whether or not the symbol is a generated-repeat-aux nonterminal.
func rawStackEntryChildPairHashEqual(arena *nodeArena, a, b stackEntry) bool {
	if arena == nil {
		return false
	}
	aShape, aHasShape := rawShapeForStackEntry(arena, a)
	if !aHasShape {
		return false
	}
	bShape, bHasShape := rawShapeForStackEntry(arena, b)
	if !bHasShape {
		return false
	}
	if aShape.symbol != bShape.symbol || aShape.childCount != bShape.childCount || aShape.contentHash != bShape.contentHash {
		return false
	}
	return stackEntryNodeStartByte(a) == stackEntryNodeStartByte(b) && stackEntryNodeEndByte(a) == stackEntryNodeEndByte(b)
}

func (p *Parser) symbolIsGeneratedRepeatAux(sym Symbol) bool {
	if p == nil || p.language == nil {
		return false
	}
	idx := int(sym)
	if idx < 0 || idx >= len(p.language.SymbolMetadata) {
		return false
	}
	return p.language.SymbolMetadata[idx].GeneratedRepeatAux
}

func rawStackEntryChildAt(arena *nodeArena, entry stackEntry, i int) (stackEntry, bool) {
	if shape, ok := rawShapeForStackEntry(arena, entry); ok {
		children := arena.rawShapeChildren(shape)
		if i < 0 || i >= len(children) {
			return stackEntry{}, false
		}
		child := children[i]
		if child.shapeRef != 0 {
			if n := stackEntryNode(child.entry); n != nil {
				n.rawShape = child.shapeRef
			}
		}
		return child.entry, stackEntryHasNode(child.entry)
	}
	if node := stackEntryNode(entry); node != nil {
		return nodeChildEntryAtNoMaterialize(node, i)
	}
	if parent := stackEntryPendingParent(entry); parent != nil {
		child := parent.childEntry(arena, i)
		return child, stackEntryHasNode(child)
	}
	return stackEntry{}, false
}

func rawShapeForStackEntry(arena *nodeArena, entry stackEntry) (*rawShape, bool) {
	ref := stackEntryRawShapeRef(entry)
	if ref == 0 {
		return nil, false
	}
	return arena.rawShapeForRef(ref)
}

func reduceWindowsFromGSS(s *glrStack, childCount int, maxForks int) []reduceFork {
	if s == nil || s.gss.head == nil || childCount <= 0 || maxForks <= 0 {
		return nil
	}

	var forks []reduceFork
	var revBuf [64]stackEntry
	revPath := revBuf[:0]

	var dfs func(n *gssNode, remaining int)
	dfs = func(n *gssNode, remaining int) {
		if n == nil || len(forks) >= maxForks {
			return
		}
		for i, count := 0, n.linkCount(); i < count; i++ {
			if len(forks) >= maxForks {
				return
			}
			prev, entry := n.link(i)
			mark := len(revPath)
			revPath = append(revPath, entry)

			nextRemaining := remaining
			if stackEntryHasNode(entry) && !stackEntryNodeIsExtra(entry) {
				nextRemaining--
			}
			if nextRemaining == 0 {
				if prev != nil {
					pathLen := len(revPath)
					window := make([]stackEntry, pathLen)
					for j := 0; j < pathLen; j++ {
						window[j] = revPath[pathLen-1-j]
					}
					forks = append(forks, reduceFork{
						window:   window,
						topState: prev.entry.state,
						popTo:    prev,
					})
				}
				revPath = revPath[:mark]
				continue
			}

			dfs(prev, nextRemaining)
			revPath = revPath[:mark]
		}
	}

	dfs(s.gss.head, childCount)
	return forks
}

const selectedReduceGSSWorkBudgetMultiplier = 10

func selectedReduceGSSWorkBudget(childCount int, maxGroups int) int {
	if childCount <= 0 || maxGroups <= 0 {
		return 0
	}
	budget := maxGroups * maxGroups * childCount * selectedReduceGSSWorkBudgetMultiplier
	if budget < maxGroups {
		return maxGroups
	}
	return budget
}

func (p *Parser) selectedReduceWindowsFromGSS(arena *nodeArena, act ParseAction, s *glrStack, childCount int, maxGroups int) []reduceFork {
	forks, _, _ := p.selectedReduceWindowsFromGSSWithBudget(arena, act, s, childCount, maxGroups, selectedReduceGSSWorkBudget(childCount, maxGroups))
	return forks
}

func (p *Parser) selectedReduceWindowsFromGSSWithBudget(arena *nodeArena, act ParseAction, s *glrStack, childCount int, maxGroups int, workBudget int) ([]reduceFork, int, bool) {
	if s == nil || s.gss.head == nil || childCount <= 0 || maxGroups <= 0 {
		return nil, 0, false
	}
	if workBudget <= 0 {
		return nil, 0, true
	}

	var forks []reduceFork
	var revBuf [64]stackEntry
	revPath := revBuf[:0]
	work := 0
	capped := false

	addFork := func(fork reduceFork) {
		keep := true
		insertAt := -1
		for i := 0; i < len(forks); {
			if !reduceForksSameChildSelectionGroup(forks[i], fork) {
				i++
				continue
			}
			switch p.reduceForkWindowPreference(arena, act, fork, forks[i]) {
			case -1:
				if insertAt < 0 {
					insertAt = i
				}
				forks = append(forks[:i], forks[i+1:]...)
			case 1:
				keep = false
				i = len(forks)
			default:
				i++
			}
		}
		if !keep {
			return
		}
		if insertAt >= 0 || len(forks) < maxGroups {
			forks = insertReduceFork(forks, insertAt, fork)
		}
	}

	var dfs func(n *gssNode, remaining int)
	dfs = func(n *gssNode, remaining int) {
		if n == nil || capped {
			return
		}
		for i, count := 0, n.linkCount(); i < count; i++ {
			if work >= workBudget {
				capped = true
				return
			}
			work++
			prev, entry := n.link(i)
			mark := len(revPath)
			revPath = append(revPath, entry)

			nextRemaining := remaining
			if stackEntryHasNode(entry) && !stackEntryNodeIsExtra(entry) {
				nextRemaining--
			}
			if nextRemaining == 0 {
				if prev != nil {
					pathLen := len(revPath)
					window := make([]stackEntry, pathLen)
					for j := 0; j < pathLen; j++ {
						window[j] = revPath[pathLen-1-j]
					}
					addFork(reduceFork{
						window:   window,
						topState: prev.entry.state,
						popTo:    prev,
					})
				}
				revPath = revPath[:mark]
				continue
			}

			dfs(prev, nextRemaining)
			if capped {
				revPath = revPath[:mark]
				return
			}
			revPath = revPath[:mark]
		}
	}

	dfs(s.gss.head, childCount)
	return forks, work, capped
}

func markReduceApplied(s *glrStack, act ParseAction, anyReduced *bool) {
	s.score += int(act.DynamicPrecedence)
	*anyReduced = true
}

func tryMergePostReduceFork(p *Parser, target, fork *glrStack) bool {
	if target == nil || fork == nil || target.accepted || fork.accepted {
		return false
	}
	if target.entries != nil || fork.entries != nil || target.gss.head == nil || fork.gss.head == nil {
		return false
	}
	if target.top().state != fork.top().state || target.byteOffset != fork.byteOffset {
		return false
	}
	return tryGSSMainMergeForParser(p, target, fork)
}

func (p *Parser) postReduceForkMergeHasFinalizationRisk(s *glrStack, tok Token) bool {
	if p == nil || p.language == nil || s == nil || s.dead || s.depth() == 0 {
		return true
	}
	if tok.NoLookahead {
		return true
	}
	actionIdx := p.lookupActionIndex(s.top().state, 0)
	if tok.Symbol != 0 {
		return true
	}
	if actionIdx == 0 {
		return true
	}
	if int(actionIdx) >= len(p.language.ParseActions) {
		return true
	}
	actions := p.language.ParseActions[actionIdx].Actions
	if len(actions) != 1 {
		return true
	}
	act := actions[0]
	if act.Type == ParseActionReduce {
		return act.ChildCount == 0
	}
	return act.Type != ParseActionAccept
}

func (p *Parser) tryFastVisibleReduceActionFromGSS(s *glrStack, act ParseAction, tok Token, anyReduced *bool, nodeCount *int, arena *nodeArena, entryScratch *glrEntryScratch, gssScratch *gssScratch, tmpEntries *[]stackEntry, deferParentLinks bool, trackChildErrors bool) bool {
	if p == nil || s == nil || s.gss.head == nil || p.language == nil {
		return false
	}
	childCount := int(act.ChildCount)
	if !gssSpanIsLinear(s.gss.head, childCount) {
		return false
	}
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
	var rawEntries [16]stackEntry
	rawWindow := rawEntries[:0]
	if childCount > cap(rawEntries) {
		rawWindow = make([]stackEntry, 0, childCount)
	}
	for i := 0; i < childCount; i++ {
		rawWindow = append(rawWindow, newStackEntryNode(0, children[i]))
	}
	rawShape := p.captureRawShape(arena, act.Symbol, act.ProductionID, rawWindow, 0, len(rawWindow))
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
	parent.rawShape = rawShape
	setReduceNodeDynamicPrecedence(parent, rawWindow, 0, len(rawWindow), act)
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

func (p *Parser) tryFastUnaryCollapseFromGSS(s *glrStack, act ParseAction, tok Token, anyReduced *bool, arena *nodeArena, entryScratch *glrEntryScratch, gssScratch *gssScratch, tmpEntries *[]stackEntry) bool {
	if p == nil || s == nil || s.gss.head == nil || p.language == nil || arena == nil {
		return false
	}
	if p.reduceTiming != nil || arena.breakdownEnabled || tok.NoLookahead || act.ChildCount != 1 {
		return false
	}
	if p.reduceProductionHasEffectiveFields(1, act.ProductionID, arena) || len(p.reduceAliasSequence(act.ProductionID)) != 0 {
		return false
	}

	head := s.gss.head
	child := stackEntryNode(head.entry)
	if child == nil || child.isExtra() || child.ownerArena != arena || child.parent != nil || !p.isVisibleSymbol(child.symbol) {
		return false
	}
	if head.prev == nil {
		return false
	}
	collapsed, _ := p.collapseUnaryChildForReductionWithRule(act, arena, child)
	if collapsed == nil {
		return false
	}

	targetDepth := s.depth() - 1
	if targetDepth < 0 {
		return false
	}
	topState := head.prev.entry.state
	gotoState := p.lookupGoto(topState, act.Symbol)
	targetState := topState
	if gotoState != 0 {
		targetState = gotoState
	}
	collapsed.productionID = act.ProductionID
	collapsed.preGotoState = topState
	collapsed.parseState = targetState
	nodeBumpEquivVersion(collapsed)
	if !s.truncateBeforePush(targetDepth) {
		s.dead = true
		if tmpEntries != nil {
			*tmpEntries = (*tmpEntries)[:0]
		}
		return true
	}
	p.pushStackNode(s, targetState, collapsed, entryScratch, gssScratch)
	markReduceApplied(s, act, anyReduced)
	if tmpEntries != nil {
		*tmpEntries = (*tmpEntries)[:0]
	}
	return true
}

func (p *Parser) applyNoTreeReduceActionFromGSS(s *glrStack, act ParseAction, tok Token, anyReduced *bool, nodeCount *int, arena *nodeArena, entryScratch *glrEntryScratch, gssScratch *gssScratch, tmpEntries *[]stackEntry, tmp []stackEntry, trackChildErrors bool) {
	if glrFaithfulCapOneMerge && s.gss.head != nil && !gssSpanIsLinear(s.gss.head, int(act.ChildCount)) {
		s.dead = true
		releaseReduceWindowEntries(tmpEntries, nil)
		return
	}
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

func (p *Parser) applyReduceActionFromGSS(source []byte, s *glrStack, act ParseAction, tok Token, anyReduced *bool, nodeCount *int, arena *nodeArena, entryScratch *glrEntryScratch, gssScratch *gssScratch, tmpEntries *[]stackEntry, tmp []stackEntry, deferParentLinks bool, trackChildErrors bool) {
	if p != nil && p.noTreeBenchmarkOnly {
		p.applyNoTreeReduceActionFromGSS(s, act, tok, anyReduced, nodeCount, arena, entryScratch, gssScratch, tmpEntries, tmp, trackChildErrors)
		return
	}
	if s.gss.head != nil && !gssSpanIsLinear(s.gss.head, int(act.ChildCount)) {
		p.applyReduceActionForked(source, s, act, tok, anyReduced, nodeCount, arena, entryScratch, gssScratch, tmpEntries, tmp, deferParentLinks, trackChildErrors)
		return
	}
	if p.tryFastUnaryCollapseFromGSS(s, act, tok, anyReduced, arena, entryScratch, gssScratch, tmpEntries) {
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
			p.pushCollapsedUnaryReduceEntry(s, act, tok, child, arena, entryScratch, gssScratch, windowEntries, window.reducedEnd, window.actualEnd, window.topState)
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
		p.pushCollapsedUnaryReduceNode(s, act, tok, child, arena, entryScratch, gssScratch, windowEntries, window.reducedEnd, window.actualEnd, window.topState)
		markReduceApplied(s, act, anyReduced)
		releaseReduceWindowEntries(tmpEntries, windowEntries)
		return
	}

	childStart := time.Time{}
	if timing != nil {
		childStart = time.Now()
	}
	rawShape := p.captureRawShape(arena, act.Symbol, act.ProductionID, windowEntries, window.start, window.reducedEnd)
	children, fieldIDs, fieldSources, childPath := p.buildReduceChildrenWithPath(windowEntries, window.start, window.reducedEnd, childCount, act.Symbol, act.ProductionID, arena)
	if timing != nil {
		timing.reduceChildBuildNanos += time.Since(childStart).Nanoseconds()
	}

	if !truncateStackForReduce(s, targetDepth) {
		releaseReduceWindowEntries(tmpEntries, windowEntries)
		return
	}

	if child := p.collapsibleUnarySelfReduction(act, tok, arena, windowEntries, window.start, window.reducedEnd, children, fieldIDs); child != nil {
		p.pushCollapsedUnaryReduceNode(s, act, tok, child, arena, entryScratch, gssScratch, windowEntries, window.reducedEnd, window.actualEnd, window.topState)
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
	parent.rawShape = rawShape
	setReduceNodeDynamicPrecedence(parent, windowEntries, window.start, window.reducedEnd, act)
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
		extendParentSpanToWindow(parent, windowEntries, window.start, window.reducedEnd, p.language.SymbolMetadata, p.spanExtendingInvisibleSymbols, p.nonSpanExtendingInvisibleSymbols, source)
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

func (p *Parser) applyReduceActionForked(source []byte, s *glrStack, act ParseAction, tok Token, anyReduced *bool, nodeCount *int, arena *nodeArena, entryScratch *glrEntryScratch, gssScratch *gssScratch, tmpEntries *[]stackEntry, _ []stackEntry, deferParentLinks bool, trackChildErrors bool) {
	forks := p.selectedReduceWindowsFromGSS(arena, act, s, int(act.ChildCount), maxStacksPerMergeKey)
	if perfCountersEnabled {
		perfRecordReduceForkCall(len(forks))
	}
	if len(forks) == 0 {
		s.dead = true
		releaseReduceWindowEntries(tmpEntries, nil)
		return
	}

	named := p.isNamedSymbol(act.Symbol)
	applyForkToStack := func(target *glrStack, fork reduceFork) {
		window := fork.window
		reducedEnd := reducedEndBeforeTrailingExtras(window)
		actualEnd := len(window)
		childCount := int(act.ChildCount)

		rawShape := p.captureRawShape(arena, act.Symbol, act.ProductionID, window, 0, reducedEnd)
		children, fieldIDs, fieldSources, childPath := p.buildReduceChildrenWithPath(window, 0, reducedEnd, childCount, act.Symbol, act.ProductionID, arena)

		target.gss.head = fork.popTo
		if target.entries != nil {
			target.entries = nil
		}

		if child := p.collapsibleUnarySelfReduction(act, tok, arena, window, 0, reducedEnd, children, fieldIDs); child != nil {
			p.pushCollapsedUnaryReduceNode(target, act, tok, child, arena, entryScratch, gssScratch, window, reducedEnd, actualEnd, fork.topState)
			target.byteOffset = target.gss.byteOffset()
			return
		}

		var parent *Node
		if deferParentLinks {
			parent = newParentNodeInArenaNoLinksWithFieldSources(arena, act.Symbol, named, children, fieldIDs, fieldSources, act.ProductionID, trackChildErrors)
		} else {
			parent = newParentNodeInArenaWithFieldSources(arena, act.Symbol, named, children, fieldIDs, fieldSources, act.ProductionID)
		}
		parent.rawShape = rawShape
		setReduceNodeDynamicPrecedence(parent, window, 0, reducedEnd, act)
		p.recordReductionParentConstructed(arena, parent, act.Symbol, len(children), fieldIDs, fieldSources, childPath)

		shouldUseRawSpan := shouldUseRawSpanForReduction(act.Symbol, children, p.language.SymbolMetadata, p.forceRawSpanAll, p.forceRawSpanTable)
		if shouldUseRawSpan && reducedEnd > 0 {
			span := computeReduceRawSpan(window, 0, reducedEnd)
			if int(act.Symbol) < len(p.forceRawSpanTable) && p.forceRawSpanTable[act.Symbol] && actualEnd > reducedEnd {
				extendRawSpanToTrailingEntries(&span, window, reducedEnd, actualEnd)
			}
			parent.startByte = span.startByte
			parent.endByte = span.endByte
			parent.startPoint = span.startPoint
			parent.endPoint = span.endPoint
		}
		if reduceChildPathMayDropSpan(childPath) {
			extendParentSpanToWindow(parent, window, 0, reducedEnd, p.language.SymbolMetadata, p.spanExtendingInvisibleSymbols, p.nonSpanExtendingInvisibleSymbols, source)
		}
		*nodeCount++

		gotoState := p.lookupGoto(fork.topState, act.Symbol)
		targetState := fork.topState
		if gotoState != 0 {
			targetState = gotoState
		}
		if tok.NoLookahead && targetState == fork.topState {
			parent.setExtra(true)
		}
		parent.preGotoState = fork.topState
		parent.parseState = targetState
		p.pushStackNode(target, targetState, parent, entryScratch, gssScratch)
		for i := reducedEnd; i < actualEnd; i++ {
			extra := stackEntryNode(window[i])
			if extra == nil {
				continue
			}
			extra.parseState = targetState
			nodeBumpEquivVersion(extra)
			p.pushStackNode(target, targetState, extra, entryScratch, gssScratch)
		}
		target.byteOffset = target.gss.byteOffset()
	}

	base := s.cloneWithScratch(gssScratch)
	applyForkToStack(s, forks[0])
	markReduceApplied(s, act, anyReduced)

	for i := 1; i < len(forks); i++ {
		clone := base.cloneWithScratch(gssScratch)
		applyForkToStack(&clone, forks[i])
		clone.score = base.score + int(act.DynamicPrecedence)
		if !p.disablePostReduceForkMerge {
			if !p.postReduceForkMergeHasFinalizationRisk(&clone, tok) {
				if perfCountersEnabled {
					perfRecordPostReduceMergeAttempt()
				}
				if tryMergePostReduceFork(p, s, &clone) {
					if perfCountersEnabled {
						perfRecordPostReduceMergePrimarySuccess()
					}
					continue
				}
				merged := false
				for j := range p.pendingForkStacks {
					if perfCountersEnabled {
						perfRecordPostReduceMergeAttempt()
					}
					if tryMergePostReduceFork(p, &p.pendingForkStacks[j], &clone) {
						if perfCountersEnabled {
							perfRecordPostReduceMergePendingSuccess()
						}
						merged = true
						break
					}
				}
				if merged {
					continue
				}
			} else if perfCountersEnabled {
				perfRecordPostReduceMergeFinalizationRiskSkip()
			}
		} else if perfCountersEnabled {
			perfRecordPostReduceMergeDisabledSkip()
		}
		p.pendingForkStacks = append(p.pendingForkStacks, clone)
		if perfCountersEnabled {
			perfRecordPendingForkStackAppend(len(p.pendingForkStacks))
		}
	}
	releaseReduceWindowEntries(tmpEntries, nil)
}

func (p *Parser) tryFastVisibleReduceActionFromGSSTransientParents(s *glrStack, act ParseAction, tok Token, anyReduced *bool, nodeCount *int, arena *nodeArena, entryScratch *glrEntryScratch, gssScratch *gssScratch, tmpEntries *[]stackEntry, deferParentLinks bool, trackChildErrors bool) bool {
	if p == nil || s == nil || s.gss.head == nil || p.language == nil {
		return false
	}
	childCount := int(act.ChildCount)
	if !gssSpanIsLinear(s.gss.head, childCount) {
		return false
	}
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

func (p *Parser) applyReduceActionFromGSSTransientParents(source []byte, s *glrStack, act ParseAction, tok Token, anyReduced *bool, nodeCount *int, arena *nodeArena, entryScratch *glrEntryScratch, gssScratch *gssScratch, tmpEntries *[]stackEntry, tmp []stackEntry, deferParentLinks bool, trackChildErrors bool) {
	if s.gss.head != nil && !gssSpanIsLinear(s.gss.head, int(act.ChildCount)) {
		p.applyReduceActionForked(source, s, act, tok, anyReduced, nodeCount, arena, entryScratch, gssScratch, tmpEntries, tmp, deferParentLinks, trackChildErrors)
		return
	}
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
			p.pushCollapsedUnaryReduceEntry(s, act, tok, child, arena, entryScratch, gssScratch, windowEntries, reducedEnd, actualEnd, topState)
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
		p.pushCollapsedUnaryReduceNode(s, act, tok, child, arena, entryScratch, gssScratch, windowEntries, reducedEnd, actualEnd, topState)
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
	rawShape := p.captureRawShape(arena, act.Symbol, act.ProductionID, windowEntries, 0, reducedEnd)
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
		p.pushCollapsedUnaryReduceNode(s, act, tok, child, arena, entryScratch, gssScratch, windowEntries, reducedEnd, actualEnd, topState)
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
	parent.rawShape = rawShape
	setReduceNodeDynamicPrecedence(parent, windowEntries, 0, reducedEnd, act)
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
		extendParentSpanToWindow(parent, windowEntries, 0, reducedEnd, p.language.SymbolMetadata, p.spanExtendingInvisibleSymbols, p.nonSpanExtendingInvisibleSymbols, source)
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
	if stackEntryStructuralForPending(entry, symbolMeta, nil) {
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
	switch pendingPlainHiddenVisibleDescendantCount(entry, arena, symbolMeta, nil) {
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
		count, childHasPayload, childHasError, ok := pendingNoFieldChildCount(entries[i], arena, parentVisible, symbolMeta, nil)
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
		if _, ok := pendingReduceWindowSpanWithExtras(entries, start, reducedEnd); !ok {
			arena.recordPendingParentRejected(pendingParentRejectSpan)
			return false
		}
	}
	var startByte, endByte uint32
	var startPoint, endPoint Point
	if childCount > 0 {
		var first, last stackEntry
		if firstEntry, lastEntry, ok := pendingNoFieldChildEndpoints(entries, start, reducedEnd, arena, parentVisible, symbolMeta, nil); ok {
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
	} else if span, ok := pendingReduceWindowSpanWithExtras(entries, start, reducedEnd); ok {
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
	parent.rawShape = p.captureRawShape(arena, act.Symbol, act.ProductionID, entries, start, reducedEnd)
	parent.dynamicPrecedence = reduceWindowDynamicPrecedence(entries, start, reducedEnd, act)
	out := 0
	flattenedParents := 0
	flattenedChildRefs := 0
	parentChildren := parent.childRefs(arena)
	for i := start; i < reducedEnd; i++ {
		next, parents, refs := fillPendingNoFieldChildren(parentChildren, out, entries[i], arena, parentVisible, symbolMeta, nil)
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
	retargeted := false
	for i := reducedEnd; i < trailingEnd; i++ {
		extra, ok := retargetStackEntryPayload(entries[i], targetState)
		if !ok {
			continue
		}
		retargeted = true
		p.pushStackEntry(s, extra, entryScratch, gssScratch)
	}
	if retargeted {
		// retargetStackEntryPayload rewrote a spine node's parseState in place,
		// which feeds gssEntryHash and thus every cached root->head shape prefix
		// rolling through that node; invalidate the prefix cache (see glr.go
		// glrMaterializingShapeHash doc). p.mergeScratch is nil-safe.
		p.mergeScratch.bumpShapePrefixEpoch()
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

	window, ok := scanPendingDirectFieldParentWindow(entries, start, reducedEnd, arena, symbolMeta, nil)
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
	} else if span, ok := pendingReduceWindowSpanWithExtras(entries, start, reducedEnd); ok {
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
	parent.rawShape = p.captureRawShape(arena, act.Symbol, act.ProductionID, entries, start, reducedEnd)
	parent.dynamicPrecedence = reduceWindowDynamicPrecedence(entries, start, reducedEnd, act)
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
	retargeted := false
	for i := reducedEnd; i < trailingEnd; i++ {
		extra, ok := retargetStackEntryPayload(entries[i], targetState)
		if !ok {
			continue
		}
		retargeted = true
		p.pushStackEntry(s, extra, entryScratch, gssScratch)
	}
	if retargeted {
		// retargetStackEntryPayload rewrote a spine node's parseState in place,
		// which feeds gssEntryHash and thus every cached root->head shape prefix
		// rolling through that node; invalidate the prefix cache (see glr.go
		// glrMaterializingShapeHash doc). p.mergeScratch is nil-safe.
		p.mergeScratch.bumpShapePrefixEpoch()
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

func scanPendingDirectFieldParentWindow(entries []stackEntry, start, reducedEnd int, arena *nodeArena, symbolMeta []SymbolMetadata, preservedHidden []bool) (pendingDirectFieldParentWindow, bool) {
	var window pendingDirectFieldParentWindow
	for i := start; i < reducedEnd; i++ {
		entry := entries[i]
		if !stackEntryHasNode(entry) {
			continue
		}
		if stackEntryNodeIsMissing(entry) {
			return pendingDirectFieldParentWindow{}, false
		}
		if !stackEntryStructuralForPending(entry, symbolMeta, preservedHidden) {
			if stackEntryNodeHasError(entry) || stackEntryTreeHasFieldIDs(entry, arena) || pendingPlainHiddenVisibleDescendantCount(entry, arena, symbolMeta, preservedHidden) != 0 {
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
		if !stackEntryStructuralForPending(entry, symbolMeta, nil) {
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
				switch pendingPlainHiddenVisibleDescendantCount(entry, arena, symbolMeta, nil) {
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
	return symbolStructuralForHiddenFlattening(sym, symbolMeta, nil)
}

func stackEntryVisibleForPending(entry stackEntry, symbolMeta []SymbolMetadata) bool {
	return symbolVisibleForPending(stackEntryNodeSymbol(entry), symbolMeta)
}

func stackEntryStructuralForPending(entry stackEntry, symbolMeta []SymbolMetadata, preservedHidden []bool) bool {
	return symbolStructuralForHiddenFlattening(stackEntryNodeSymbol(entry), symbolMeta, preservedHidden)
}

func pendingPlainHiddenVisibleDescendantCount(entry stackEntry, arena *nodeArena, symbolMeta []SymbolMetadata, preservedHidden []bool) int {
	if !stackEntryHasNode(entry) || stackEntryNodeIsMissing(entry) {
		return 0
	}
	if stackEntryStructuralForPending(entry, symbolMeta, preservedHidden) {
		return 1
	}
	if parent := stackEntryPendingParent(entry); parent != nil {
		count := 0
		for i := 0; i < parent.childEntryCount(); i++ {
			child := parent.childEntry(arena, i)
			count += pendingPlainHiddenVisibleDescendantCount(child, arena, symbolMeta, preservedHidden)
		}
		return count
	}
	if node := stackEntryNode(entry); node != nil && !hiddenTreeHasFieldIDs(node) {
		count := 0
		for _, child := range node.children {
			count += pendingPlainHiddenVisibleDescendantCount(newStackEntryNode(child.parseState, child), arena, symbolMeta, preservedHidden)
		}
		return count
	}
	return 0
}

func pendingNoFieldChildCount(entry stackEntry, arena *nodeArena, parentVisible bool, symbolMeta []SymbolMetadata, preservedHidden []bool) (count int, hasPayload bool, hasError bool, ok bool) {
	if !stackEntryHasNode(entry) {
		return 0, false, false, true
	}
	if stackEntryNodeIsMissing(entry) {
		return 0, false, false, false
	}
	hasPayload = stackEntryCompactFullLeaf(entry) != nil || stackEntryPendingParent(entry) != nil
	hasError = stackEntryNodeHasError(entry)
	if stackEntryStructuralForPending(entry, symbolMeta, preservedHidden) {
		return 1, hasPayload, hasError, true
	}
	if parentVisible {
		if stackEntryTreeHasFieldIDs(entry, arena) {
			return 0, false, false, false
		}
		if parent := stackEntryPendingParent(entry); parent != nil {
			for i := 0; i < parent.childEntryCount(); i++ {
				child := parent.childEntry(arena, i)
				childCount, childPayload, childHasError, childOK := pendingNoFieldChildCount(child, arena, true, symbolMeta, preservedHidden)
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
				childCount, childPayload, childHasError, childOK := pendingNoFieldChildCount(childEntry, arena, true, symbolMeta, preservedHidden)
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

func pendingNoFieldChildEndpoints(entries []stackEntry, start, end int, arena *nodeArena, parentVisible bool, symbolMeta []SymbolMetadata, preservedHidden []bool) (first, last stackEntry, ok bool) {
	for i := start; i < end; i++ {
		next, found := pendingNoFieldFirstChild(entries[i], arena, parentVisible, symbolMeta, preservedHidden)
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
		next, found := pendingNoFieldLastChild(entries[i], arena, parentVisible, symbolMeta, preservedHidden)
		if !found {
			continue
		}
		last = next
		return first, last, true
	}
	return stackEntry{}, stackEntry{}, false
}

func pendingNoFieldFirstChild(entry stackEntry, arena *nodeArena, parentVisible bool, symbolMeta []SymbolMetadata, preservedHidden []bool) (stackEntry, bool) {
	if !stackEntryHasNode(entry) || stackEntryNodeIsMissing(entry) {
		return stackEntry{}, false
	}
	if stackEntryStructuralForPending(entry, symbolMeta, preservedHidden) {
		return entry, true
	}
	if parentVisible {
		if parent := stackEntryPendingParent(entry); parent != nil {
			for i := 0; i < parent.childEntryCount(); i++ {
				child := parent.childEntry(arena, i)
				if next, ok := pendingNoFieldFirstChild(child, arena, true, symbolMeta, preservedHidden); ok {
					return next, true
				}
			}
			return stackEntry{}, false
		}
		if node := stackEntryNode(entry); node != nil {
			for _, child := range node.children {
				if next, ok := pendingNoFieldFirstChild(newStackEntryNode(child.parseState, child), arena, true, symbolMeta, preservedHidden); ok {
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

func pendingNoFieldLastChild(entry stackEntry, arena *nodeArena, parentVisible bool, symbolMeta []SymbolMetadata, preservedHidden []bool) (stackEntry, bool) {
	if !stackEntryHasNode(entry) || stackEntryNodeIsMissing(entry) {
		return stackEntry{}, false
	}
	if stackEntryStructuralForPending(entry, symbolMeta, preservedHidden) {
		return entry, true
	}
	if parentVisible {
		if parent := stackEntryPendingParent(entry); parent != nil {
			for i := parent.childEntryCount() - 1; i >= 0; i-- {
				child := parent.childEntry(arena, i)
				if next, ok := pendingNoFieldLastChild(child, arena, true, symbolMeta, preservedHidden); ok {
					return next, true
				}
			}
			return stackEntry{}, false
		}
		if node := stackEntryNode(entry); node != nil {
			for i := len(node.children) - 1; i >= 0; i-- {
				child := node.children[i]
				if next, ok := pendingNoFieldLastChild(newStackEntryNode(child.parseState, child), arena, true, symbolMeta, preservedHidden); ok {
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

func fillPendingNoFieldChildren(dst []pendingChildEntry, out int, entry stackEntry, arena *nodeArena, parentVisible bool, symbolMeta []SymbolMetadata, preservedHidden []bool) (next int, flattenedParents int, flattenedChildRefs int) {
	if !stackEntryHasNode(entry) || stackEntryNodeIsMissing(entry) {
		return out, 0, 0
	}
	if stackEntryStructuralForPending(entry, symbolMeta, preservedHidden) {
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
				out, parents, refs = fillPendingNoFieldChildren(dst, out, child, arena, true, symbolMeta, preservedHidden)
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
				out, parents, refs = fillPendingNoFieldChildren(dst, out, newStackEntryNode(child.parseState, child), arena, true, symbolMeta, preservedHidden)
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

func pendingReduceWindowSpanWithExtras(entries []stackEntry, start, end int) (reduceRawSpan, bool) {
	span := reduceRawSpan{}
	if end <= start {
		return span, false
	}
	foundStart := false
	for i := start; i < end; i++ {
		entry := entries[i]
		if !stackEntryHasNode(entry) || stackEntryNodeIsMissing(entry) {
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
		if !stackEntryHasNode(entry) || stackEntryNodeIsMissing(entry) {
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
// recover span from entries that buildReduceChildren drops. Leading extras are
// intentionally ignored: they are trivia before the first visible child and
// must not pull a visible parent's start earlier. Two categories are recovered:
//
//  1. Leading invisible structural children: hidden children before the first
//     visible child can seed the parent extent.
//  2. Invisible non-extra leaf children: these are structural children whose symbol
//     is not visible AND that have no children to inline. buildReduceChildren skips
//     them entirely (the "if len(kids) == 0 { continue }" path), losing their span.
//     In C tree-sitter, ts_subtree_set_children includes ALL children in the parent
//     span, so we must recover these dropped spans to match.
//
// Trailing extras (separated into [reducedEnd, actualEnd)) are NOT scanned because
// they become siblings of the parent, not children.
func extendParentSpanToWindow(parent *Node, entries []stackEntry, start, reducedEnd int, symbolMeta []SymbolMetadata, spanExtendingInvisibleSymbols, nonSpanExtendingInvisibleSymbols []bool, source []byte) {
	// Leading invisible structural children: extend startByte back to the
	// earliest one. C's ts_subtree_set_children seeds the parent extent from its
	// FIRST child unconditionally, including hidden openers that buildReduceChildren
	// drops. The general invisible-recovery scan below requires byte-contiguity
	// with the current parent.startByte, but a hidden prefix can be separated from
	// the first visible child by ordinary parser padding (for example Dart's
	// hidden `import` keyword before the URI). Scanning only the genuine prefix
	// (stop at the first visible child) keeps this exactly to C's "first child"
	// rule: when the first child is visible, the loop breaks immediately and the
	// span is untouched.
	for i := start; i < reducedEnd; i++ {
		n := stackEntryNode(entries[i])
		if n == nil {
			continue
		}
		if n.isExtra() {
			continue
		}
		visible := true
		if int(n.symbol) < len(symbolMeta) {
			visible = symbolMeta[n.symbol].Visible
		}
		if visible {
			break // first visible child seeds the extent; nothing earlier to add
		}
		if symbolMarked(nonSpanExtendingInvisibleSymbols, n.symbol) {
			continue
		}
		if n.startByte < parent.startByte {
			parent.startByte = n.startByte
			parent.startPoint = n.startPoint
		}
	}
	// Invisible non-extra children: extend parent span for entries that
	// buildReduceChildren drops or inlines away. Symbols explicitly marked as
	// span-extending may bridge a scanner-owned gap before their token; Scala
	// interpolated string tails are represented this way.
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
		spanExtending := symbolMarked(spanExtendingInvisibleSymbols, n.symbol)
		if (n.startByte <= parent.endByte || spanExtending || spanBridgeIsParserPadding(source, parent.endByte, n.startByte)) && n.endByte > parent.endByte {
			parent.endByte = n.endByte
			parent.endPoint = n.endPoint
		}
		if n.startByte == n.endByte && n.startByte > parent.endByte &&
			spanExtending {
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
		spanExtending := symbolMarked(spanExtendingInvisibleSymbols, n.symbol)
		if (n.startByte <= parent.endByte || spanExtending || spanBridgeIsParserPadding(source, parent.endByte, n.startByte)) && n.endByte > parent.endByte {
			parent.endByte = n.endByte
			parent.endPoint = n.endPoint
		}
		if n.startByte == n.endByte && n.startByte > parent.endByte &&
			spanExtending {
			parent.endByte = n.endByte
			parent.endPoint = n.endPoint
		}
	}
}

func spanBridgeIsParserPadding(source []byte, from, to uint32) bool {
	if len(source) == 0 || from >= to {
		return false
	}
	return bytesAreParserPadding(source, from, to)
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

func symbolStructuralForHiddenFlattening(sym Symbol, symbolMeta []SymbolMetadata, preservedHidden []bool) bool {
	if symbolMarked(preservedHidden, sym) {
		return true
	}
	if idx := int(sym); idx >= 0 && idx < len(symbolMeta) {
		return symbolMeta[sym].Visible
	}
	return true
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

func appendFlattenedHiddenChildrenToScratch(scratch *reduceBuildScratch, n *Node, symbolMeta []SymbolMetadata, preservedHidden []bool) {
	if scratch == nil || n == nil {
		return
	}
	if symbolStructuralForHiddenFlattening(n.symbol, symbolMeta, preservedHidden) {
		scratch.appendNode(n)
		return
	}
	paddingStartByte := n.startByte
	paddingStartPoint := n.startPoint
	paddingSource := n
	for _, child := range n.children {
		before := len(scratch.nodes)
		appendFlattenedHiddenChildrenToScratch(scratch, child, symbolMeta, preservedHidden)
		paddingStartByte, paddingStartPoint = absorbFlattenedHiddenPaddingScratch(scratch, before, paddingStartByte, paddingStartPoint, paddingSource, nil, symbolMeta)
		paddingSource = child
	}
}

func appendFlattenedHiddenChildrenWithFieldScratch(scratch *reduceBuildScratch, n *Node, symbolMeta []SymbolMetadata, preservedHidden []bool) {
	if scratch == nil || n == nil {
		return
	}
	if symbolStructuralForHiddenFlattening(n.symbol, symbolMeta, preservedHidden) {
		scratch.appendNode(n)
		return
	}

	nodeStart := len(scratch.nodes)
	repeatEpoch := scratch.nextRepeatEpoch()
	touchedStart := len(scratch.repeatTouched)
	paddingStartByte := n.startByte
	paddingStartPoint := n.startPoint
	paddingSource := n
	for i, child := range n.children {
		spanStart := len(scratch.nodes)
		appendFlattenedHiddenChildrenWithFieldScratch(scratch, child, symbolMeta, preservedHidden)
		spanEnd := len(scratch.nodes)
		paddingStartByte, paddingStartPoint = absorbFlattenedHiddenPaddingScratch(scratch, spanStart, paddingStartByte, paddingStartPoint, paddingSource, nil, symbolMeta)
		paddingSource = child
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

func (p *Parser) buildReduceChildrenAllVisible(entries []stackEntry, start, end, childCount int, aliasSeq []Symbol, rawFieldIDs []FieldID, rawInherited []bool, parentVisible bool, symbolMeta []SymbolMetadata, arena *nodeArena) ([]*Node, []FieldID, []uint8, bool) {
	visibleCount := 0
	structuralChildIndex := 0
	for i := start; i < end; i++ {
		n := stackEntryNode(entries[i])
		if n == nil {
			continue
		}
		// C parity (ts_parser__reduce): popped subtrees — including extra
		// ERROR carriers produced by recover_to_state — become children
		// as-is. Reduces never dissolve an ERROR node; doing so drops the
		// error cost and HasError bit C keeps (php `static function a()`:
		// C's `program` keeps (ERROR ...) as a direct child).
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
	parentVisible := cSymbolVisibleLang(lang, parentSymbol)

	aliasSeq := p.reduceAliasSequence(productionID)
	productionHasFields := p.reduceProductionHasEffectiveFields(childCount, productionID, arena)
	if len(aliasSeq) == 0 && !productionHasFields {
		if children, fieldIDs, fieldSources, path, ok := p.buildReduceChildrenNoAliasNoFieldsPlanned(entries, start, end, parentSymbol, symbolMeta, arena); ok {
			return children, fieldIDs, fieldSources, path
		}
	}

	rawFieldIDs, rawInherited := p.buildFieldIDs(childCount, productionID, arena)
	if children, fieldIDs, fieldSources, ok := p.buildReduceChildrenAllVisible(entries, start, end, childCount, aliasSeq, rawFieldIDs, rawInherited, parentVisible, symbolMeta, arena); ok {
		return children, fieldIDs, fieldSources, reduceChildPathForLen(len(children), reduceChildPathAllVisible)
	}

	scratch := p.newReduceBuildScratch(rawFieldIDs)
	p.appendReduceChildrenToScratch(scratch, entries, start, end, aliasSeq, rawFieldIDs, rawInherited, parentVisible, symbolMeta, arena, lang)
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

	var pendingPaddingStart uint32
	var pendingPaddingPoint Point
	var pendingPaddingSource *Node
	havePendingPadding := false
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
			if havePendingPadding && flattenedHiddenSiblingPaddingTarget(n, pendingPaddingSource, symbolMeta) && pendingPaddingStart < n.startByte {
				n = cloneNodeInArena(arena, n)
				n.startByte = pendingPaddingStart
				n.startPoint = pendingPaddingPoint
			}
			scratch.appendNode(n)
			havePendingPadding = false
			continue
		}
		if parentVisible {
			before := len(scratch.nodes)
			appendFlattenedHiddenChildrenToScratch(scratch, n, symbolMeta, nil)
			after := len(scratch.nodes)
			pendingPaddingStart, pendingPaddingPoint, havePendingPadding = flattenedHiddenEntryPadding(n, scratch.nodes, before, after)
			pendingPaddingSource = n
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

func (p *Parser) appendReduceChildrenToScratch(scratch *reduceBuildScratch, entries []stackEntry, start, end int, aliasSeq []Symbol, rawFieldIDs []FieldID, rawInherited []bool, parentVisible bool, symbolMeta []SymbolMetadata, arena *nodeArena, lang *Language) {
	structuralChildIndex := 0
	var pendingPaddingStart uint32
	var pendingPaddingPoint Point
	var pendingPaddingSource *Node
	havePendingPadding := false
	for i := start; i < end; i++ {
		item, ok := reduceChildBuildItemForEntry(entries[i], structuralChildIndex, aliasSeq, rawFieldIDs, rawInherited, arena, lang)
		if !ok {
			continue
		}
		structuralChildIndex = item.nextStructuralIndex
		spanStart, spanEnd := p.appendReduceChildItemToScratch(scratch, item, rawFieldIDs, structuralChildIndex, parentVisible, symbolMeta, havePendingPadding, pendingPaddingStart, pendingPaddingPoint, pendingPaddingSource, arena)
		if !symbolVisibleForPending(item.node.symbol, symbolMeta) {
			pendingPaddingStart, pendingPaddingPoint, havePendingPadding = flattenedHiddenEntryPadding(item.node, scratch.nodes, spanStart, spanEnd)
			pendingPaddingSource = item.node
			continue
		}
		havePendingPadding = false
	}
}

func (p *Parser) appendReduceChildItemToScratch(scratch *reduceBuildScratch, item reduceChildBuildItem, rawFieldIDs []FieldID, nextStructuralChildIndex int, parentVisible bool, symbolMeta []SymbolMetadata, havePadding bool, paddingStartByte uint32, paddingStartPoint Point, paddingSource *Node, arena *nodeArena) (int, int) {
	n := item.node
	if symbolVisibleForPending(n.symbol, symbolMeta) {
		if havePadding && flattenedHiddenSiblingPaddingTarget(n, paddingSource, symbolMeta) && paddingStartByte < n.startByte {
			n = cloneNodeInArena(arena, n)
			n.startByte = paddingStartByte
			n.startPoint = paddingStartPoint
		}
		start := len(scratch.nodes)
		p.appendVisibleReduceChildToScratch(scratch, n, item.fieldID, item.inherited)
		return start, len(scratch.nodes)
	}
	if len(n.children) == 0 {
		return len(scratch.nodes), len(scratch.nodes)
	}

	spanStart := len(scratch.nodes)
	if hiddenTreeHasFieldIDs(n) {
		appendFlattenedHiddenChildrenWithFieldScratch(scratch, n, symbolMeta, nil)
	} else {
		appendFlattenedHiddenChildrenToScratch(scratch, n, symbolMeta, nil)
	}
	if item.fieldID == 0 {
		return spanStart, len(scratch.nodes)
	}
	if !scratch.trackFields {
		scratch.ensureFieldStorage()
	}
	fieldEnd := len(scratch.fieldIDs)
	applyParentFieldToFlattenedHiddenSpan(scratch, n, spanStart, fieldEnd, item.fieldID, item.inherited, rawFieldIDs, nextStructuralChildIndex)
	return spanStart, len(scratch.nodes)
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

func countFlattenedHiddenChildren(n *Node, symbolMeta []SymbolMetadata, preservedHidden []bool) int {
	if n == nil {
		return 0
	}
	if symbolStructuralForHiddenFlattening(n.symbol, symbolMeta, preservedHidden) {
		return 1
	}
	count := 0
	for _, child := range n.children {
		count += countFlattenedHiddenChildren(child, symbolMeta, preservedHidden)
	}
	return count
}

func appendFlattenedHiddenChildren(dst []*Node, out int, n *Node, symbolMeta []SymbolMetadata, preservedHidden []bool) int {
	return appendFlattenedHiddenChildrenWithFields(dst, nil, nil, out, n, symbolMeta, preservedHidden)
}

func appendFlattenedHiddenChildrenWithFields(dst []*Node, fieldDst []FieldID, fieldSrcDst []uint8, out int, n *Node, symbolMeta []SymbolMetadata, preservedHidden []bool) int {
	if n == nil {
		return out
	}
	if symbolStructuralForHiddenFlattening(n.symbol, symbolMeta, preservedHidden) {
		dst[out] = n
		return out + 1
	}
	nodeStart := out
	paddingStartByte := n.startByte
	paddingStartPoint := n.startPoint
	type hiddenFieldSpan struct {
		count  int
		source uint8
	}
	var repeated map[FieldID]hiddenFieldSpan
	for i, child := range n.children {
		spanStart := out
		out = appendFlattenedHiddenChildrenWithFields(dst, fieldDst, fieldSrcDst, out, child, symbolMeta, preservedHidden)
		paddingStartByte, paddingStartPoint = absorbFlattenedHiddenPaddingNodes(dst, spanStart, out, paddingStartByte, paddingStartPoint, child, nil, symbolMeta)
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

func absorbFlattenedHiddenPaddingScratch(scratch *reduceBuildScratch, start int, paddingStartByte uint32, paddingStartPoint Point, source *Node, arena *nodeArena, symbolMeta []SymbolMetadata) (uint32, Point) {
	if scratch == nil {
		return paddingStartByte, paddingStartPoint
	}
	return absorbFlattenedHiddenPaddingNodes(scratch.nodes, start, len(scratch.nodes), paddingStartByte, paddingStartPoint, source, arena, symbolMeta)
}

func absorbFlattenedHiddenPaddingNodes(nodes []*Node, start, end int, paddingStartByte uint32, paddingStartPoint Point, source *Node, arena *nodeArena, symbolMeta []SymbolMetadata) (uint32, Point) {
	if start >= end {
		if source != nil && source.startByte <= paddingStartByte && source.endByte > paddingStartByte {
			return source.endByte, source.endPoint
		}
		return paddingStartByte, paddingStartPoint
	}
	first := nodes[start]
	if first == nil {
		return paddingStartByte, paddingStartPoint
	}
	if paddingStartByte < first.startByte && flattenedHiddenPaddingTarget(first, source, symbolMeta) {
		if arena == nil {
			arena = first.ownerArena
		}
		cloned := cloneNodeInArena(arena, first)
		if cloned != nil {
			cloned.startByte = paddingStartByte
			cloned.startPoint = paddingStartPoint
			nodes[start] = cloned
		}
	}
	last := nodes[end-1]
	if last == nil {
		return paddingStartByte, paddingStartPoint
	}
	return last.endByte, last.endPoint
}

func flattenedHiddenEntryPadding(source *Node, nodes []*Node, start, end int) (uint32, Point, bool) {
	if source == nil {
		return 0, Point{}, false
	}
	if start < end {
		last := nodes[end-1]
		if last != nil && source.endByte > last.endByte {
			return last.endByte, last.endPoint, true
		}
		return source.endByte, source.endPoint, false
	}
	if source.endByte > source.startByte {
		return source.startByte, source.startPoint, true
	}
	return 0, Point{}, false
}

func flattenedHiddenPaddingTarget(n, source *Node, symbolMeta []SymbolMetadata) bool {
	if n == nil || n.isExtra() || n.isMissing() || n.hasError() {
		return false
	}
	if n.isExternalScannerToken() {
		return false
	}
	if idx := int(n.symbol); idx >= 0 && idx < len(symbolMeta) {
		meta := symbolMeta[n.symbol]
		if !meta.Visible || meta.Named {
			return false
		}
		return len(n.children) > 0
	}
	return !n.isNamed() && len(n.children) > 0
}

func flattenedHiddenSiblingPaddingTarget(n, source *Node, symbolMeta []SymbolMetadata) bool {
	return flattenedHiddenPaddingTarget(n, source, symbolMeta)
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

func (p *Parser) applyReduceAction(source []byte, s *glrStack, act ParseAction, tok Token, anyReduced *bool, nodeCount *int, arena *nodeArena, entryScratch *glrEntryScratch, gssScratch *gssScratch, entries []stackEntry, deferParentLinks bool, trackChildErrors bool) {
	var timing *parseMaterializationTiming
	if p != nil {
		timing = p.reduceTiming
	}
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
		window, ok = computeReduceRangeForFullPayloads(entries, childCount, p != nil && p.usePendingFullParents())
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
			p.pushCollapsedUnaryReduceEntry(s, act, tok, child, arena, entryScratch, gssScratch, entries, window.reducedEnd, window.actualEnd, window.topState)
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
		p.pushCollapsedUnaryReduceNode(s, act, tok, child, arena, entryScratch, gssScratch, entries, window.reducedEnd, window.actualEnd, window.topState)
		s.score += int(act.DynamicPrecedence)
		*anyReduced = true
		return
	}

	childStart := time.Time{}
	if timing != nil {
		childStart = time.Now()
	}
	rawShape := p.captureRawShape(arena, act.Symbol, act.ProductionID, entries, window.start, window.reducedEnd)
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
		p.pushCollapsedUnaryReduceNode(s, act, tok, child, arena, entryScratch, gssScratch, entries, trailingStart, trailingEnd, window.topState)
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
	parent.rawShape = rawShape
	setReduceNodeDynamicPrecedence(parent, entries, window.start, window.reducedEnd, act)
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
		extendParentSpanToWindow(parent, entries, window.start, window.reducedEnd, p.language.SymbolMetadata, p.spanExtendingInvisibleSymbols, p.nonSpanExtendingInvisibleSymbols, source)
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

func (p *Parser) applyReduceActionTransientParents(source []byte, s *glrStack, act ParseAction, tok Token, anyReduced *bool, nodeCount *int, arena *nodeArena, entryScratch *glrEntryScratch, gssScratch *gssScratch, entries []stackEntry, deferParentLinks bool, trackChildErrors bool) {
	var timing *parseMaterializationTiming
	if p != nil {
		timing = p.reduceTiming
	}
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
		window, ok = computeReduceRangeForFullPayloads(entries, childCount, p != nil && p.usePendingFullParents())
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
			p.pushCollapsedUnaryReduceEntry(s, act, tok, child, arena, entryScratch, gssScratch, entries, window.reducedEnd, window.actualEnd, window.topState)
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
		p.pushCollapsedUnaryReduceNode(s, act, tok, child, arena, entryScratch, gssScratch, entries, window.reducedEnd, window.actualEnd, window.topState)
		s.score += int(act.DynamicPrecedence)
		*anyReduced = true
		return
	}

	childStart := time.Time{}
	if timing != nil {
		childStart = time.Now()
	}
	rawShape := p.captureRawShape(arena, act.Symbol, act.ProductionID, entries, window.start, window.reducedEnd)
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
		p.pushCollapsedUnaryReduceNode(s, act, tok, child, arena, entryScratch, gssScratch, entries, trailingStart, trailingEnd, window.topState)
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
	parent.rawShape = rawShape
	setReduceNodeDynamicPrecedence(parent, entries, window.start, window.reducedEnd, act)
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
		extendParentSpanToWindow(parent, entries, window.start, window.reducedEnd, p.language.SymbolMetadata, p.spanExtendingInvisibleSymbols, p.nonSpanExtendingInvisibleSymbols, source)
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

	parent := newNoTreeReduceNodeInArenaWithRawShape(arena, act, p.isNamedSymbol(act.Symbol), entries, start, reducedEnd, tok, trackChildErrors, !p.noResultCompatibilityBenchmarkOnly)
	if tok.NoLookahead && targetState == topState {
		parent.setExtra(true)
	}
	parent.preGotoState = topState
	parent.parseState = targetState
	p.pushStackNoTreeNode(s, targetState, parent, entryScratch, gssScratch)
	retargeted := false
	for i := trailingStart; i < trailingEnd; i++ {
		extra, ok := retargetStackEntryPayload(entries[i], targetState)
		if !ok {
			continue
		}
		retargeted = true
		p.pushStackEntry(s, extra, entryScratch, gssScratch)
	}
	if retargeted {
		// retargetStackEntryPayload rewrote a spine node's parseState in place,
		// which feeds gssEntryHash and thus every cached root->head shape prefix
		// rolling through that node; invalidate the prefix cache (see glr.go
		// glrMaterializingShapeHash doc). p.mergeScratch is nil-safe.
		p.mergeScratch.bumpShapePrefixEpoch()
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

func newNoTreeReduceNodeInArena(arena *nodeArena, act ParseAction, named bool, entries []stackEntry, start, reducedEnd int, tok Token, trackChildErrors bool) *noTreeNode {
	return newNoTreeReduceNodeInArenaWithRawShape(arena, act, named, entries, start, reducedEnd, tok, trackChildErrors, true)
}

func newNoTreeReduceNodeInArenaWithRawShape(arena *nodeArena, act ParseAction, named bool, entries []stackEntry, start, reducedEnd int, tok Token, trackChildErrors bool, captureRawShape bool) *noTreeNode {
	var n *noTreeNode
	if arena == nil {
		n = &noTreeNode{}
	} else {
		n = arena.allocNoTreeNode()
		arena.noTreeReduceNodesConstructed++
	}
	n.symbol = act.Symbol
	n.startByte = tok.StartByte
	n.endByte = tok.StartByte
	n.parseState = 0
	n.preGotoState = 0
	n.productionID = act.ProductionID
	if captureRawShape {
		n.rawShape = captureRawShapeInArena(arena, act, entries, start, reducedEnd)
	} else {
		n.rawShape = 0
	}
	if captureRawShape {
		n.dynamicPrecedence = reduceWindowDynamicPrecedence(entries, start, reducedEnd, act)
	} else {
		n.dynamicPrecedence = int32(act.DynamicPrecedence)
	}
	n.flags = noTreeNodeInitialFlags(named)
	if !captureRawShape {
		return n
	}
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

func captureRawShapeInArena(arena *nodeArena, act ParseAction, entries []stackEntry, start, end int) rawShapeRef {
	if arena == nil {
		return 0
	}
	var p Parser
	return p.captureRawShape(arena, act.Symbol, act.ProductionID, entries, start, end)
}

func captureCollapsedUnaryRawShape(arena *nodeArena, act ParseAction, entries []stackEntry, reducedEnd int) rawShapeRef {
	return captureRawShapeInArena(arena, act, entries, reducedEnd-1, reducedEnd)
}

func (p *Parser) pushCollapsedUnaryReduceNode(s *glrStack, act ParseAction, tok Token, child *Node, arena *nodeArena, entryScratch *glrEntryScratch, gssScratch *gssScratch, entries []stackEntry, trailingStart, trailingEnd int, topState StateID) {
	rawShape := captureCollapsedUnaryRawShape(arena, act, entries, trailingStart)
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
	child.rawShape = rawShape
	child.dynamicPrecedence += int32(act.DynamicPrecedence)
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

func (p *Parser) pushCollapsedUnaryReduceEntry(s *glrStack, act ParseAction, tok Token, child stackEntry, arena *nodeArena, entryScratch *glrEntryScratch, gssScratch *gssScratch, entries []stackEntry, trailingStart, trailingEnd int, topState StateID) {
	rawShape := captureCollapsedUnaryRawShape(arena, act, entries, trailingStart)
	gotoState := p.lookupGoto(topState, act.Symbol)
	targetState := topState
	if gotoState != 0 {
		targetState = gotoState
	}
	setCollapsedUnaryEntryMetadata(&child, act, tok.NoLookahead && targetState == topState, topState, targetState)
	setStackEntryRawShapeRef(&child, rawShape)
	addStackEntryDynamicPrecedence(&child, act.DynamicPrecedence)
	p.pushStackEntry(s, child, entryScratch, gssScratch)
	retargeted := false
	for i := trailingStart; i < trailingEnd; i++ {
		extra, ok := retargetStackEntryPayload(entries[i], targetState)
		if !ok {
			continue
		}
		retargeted = true
		p.pushStackEntry(s, extra, entryScratch, gssScratch)
	}
	if retargeted {
		// retargetStackEntryPayload rewrote a spine node's parseState in place,
		// which feeds gssEntryHash and thus every cached root->head shape prefix
		// rolling through that node; invalidate the prefix cache (see glr.go
		// glrMaterializingShapeHash doc). p.mergeScratch is nil-safe.
		p.mergeScratch.bumpShapePrefixEpoch()
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
		if !p.isVisibleSymbol(child.symbol) && !p.canCollapseHiddenChoicePassthroughSymbol(act.Symbol) {
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
		if !p.isVisibleSymbol(parent.symbol) && !p.canCollapseHiddenChoicePassthroughSymbol(act.Symbol) {
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
		if p.shouldKeepVisibleAnonymousTokenChild(act.Symbol, childSym) {
			return collapseUnaryRuleNone
		}
		return collapseUnaryRuleNamedLeafAlias
	}
	if p.isSharedVisibleAnonymousToken(childSym) {
		return collapseUnaryRuleNone
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
	if symbolMarked(p.aliasPreservedWrapperSymbols, parentSym) {
		return false
	}
	return invisibleUnaryWrapperCollapsible(meta[parentSym]) || p.canCollapseHiddenChoicePassthroughSymbol(parentSym)
}

func (p *Parser) canCollapseHiddenChoicePassthroughSymbol(parentSym Symbol) bool {
	if p == nil || p.language == nil {
		return false
	}
	if symbolMarked(p.aliasPreservedWrapperSymbols, parentSym) {
		return false
	}
	idx := int(parentSym)
	return idx >= 0 &&
		idx < len(p.language.HiddenChoicePassthroughSymbols) &&
		p.language.HiddenChoicePassthroughSymbols[idx]
}

// buildAliasPreservedWrapperSymbols marks invisible aux wrapper symbols that
// appear in the grammar's C-side ts_non_terminal_alias_map: such wrappers can
// be ALIASED to a visible symbol by an enclosing production, so the wrapper
// node must survive unary reduces. Collapsing it (renaming-through to its lone
// child) makes the later alias relabel the child instead of nesting it, e.g.
// lua's `'\027'` produced (string_content) instead of C's
// (string_content (escape_sequence)).
//
// The compiled blobs do not carry ts_non_terminal_alias_map, so the affected
// symbols are listed per language here. Extend this table whenever a grammar's
// alias map names an aux wrapper that an enclosing rule aliases.
func buildAliasPreservedWrapperSymbols(lang *Language) []bool {
	if lang == nil {
		return nil
	}
	var names []string
	switch lang.Name {
	case "lua":
		// ts_non_terminal_alias_map (tree-sitter-lua @ 10fe0054):
		// aux_sym__doublequote_string_content and
		// aux_sym__singlequote_string_content are aliased to string_content
		// by the _quote_string productions.
		names = []string{"_doublequote_string_content", "_singlequote_string_content"}
	default:
		return nil
	}
	var out []bool
	for i, name := range lang.SymbolNames {
		for _, want := range names {
			if name != want {
				continue
			}
			if out == nil {
				out = make([]bool, len(lang.SymbolNames))
			}
			out[i] = true
		}
	}
	return out
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
	if !p.isVisibleSymbol(child.symbol) && !p.canCollapseHiddenChoicePassthroughSymbol(act.Symbol) {
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
		if p.shouldKeepVisibleAnonymousTokenChild(act.Symbol, child.symbol) {
			return nil, collapseUnaryRuleNone
		}
		return aliasedNodeInArena(arena, p.language, child, act.Symbol), collapseUnaryRuleNamedLeafAlias
	}
	if p.isSharedVisibleAnonymousToken(child.symbol) {
		return nil, collapseUnaryRuleNone
	}
	return child, collapseUnaryRuleSameSymbol
}

func (p *Parser) shouldKeepVisibleAnonymousTokenChild(parentSym, childSym Symbol) bool {
	if p == nil || p.language == nil {
		return true
	}
	meta := p.language.SymbolMetadata
	if int(parentSym) < 0 || int(parentSym) >= len(meta) ||
		int(childSym) < 0 || int(childSym) >= len(meta) {
		return true
	}
	parent := meta[parentSym]
	child := meta[childSym]
	if !parent.Visible || !parent.Named || !child.Visible || child.Named {
		return true
	}
	if p.wrapsSameNamedAnonymousToken(parentSym, childSym) || p.isSharedAnonymousToken(childSym) {
		return true
	}
	// Only the inlined-token artifact collapses to a childless named leaf: a
	// named rule whose body is a single literal that tree-sitter lexes directly as
	// the rule symbol (e.g. go `nil`/`true`/`false`/`iota`). In the loaded
	// Language ts2go splits that into a same-named named rule plus a same-named
	// visible anonymous token. A DIFFERENT-named visible anonymous child (e.g.
	// optional_chain `?.`, starlark continue_statement `continue`, empty `;`,
	// moduleExpr `module`) is a genuine production member that C keeps.
	return !p.sameSymbolName(parentSym, childSym)
}

func (p *Parser) canCollapseInvisibleUnaryWrapper(parentSym Symbol, child *Node) bool {
	if p == nil || p.language == nil || child == nil || child.isExtra() || child.isMissing() || child.IsError() {
		return false
	}
	meta := p.language.SymbolMetadata
	if int(parentSym) >= len(meta) {
		return false
	}
	if symbolMarked(p.aliasPreservedWrapperSymbols, parentSym) {
		return false
	}
	return invisibleUnaryWrapperCollapsible(meta[parentSym]) || p.canCollapseHiddenChoicePassthroughSymbol(parentSym)
}

// invisibleUnaryWrapperCollapsible reports whether an invisible unary wrapper
// symbol may be collapsed (renamed-through to its lone child) during reduction.
// Anonymous (invisible AND unnamed) wrappers are collapsible by default. Named
// hidden wrappers need generated HiddenChoicePassthroughSymbols metadata before
// they can collapse, because alias-referenced hidden named wrappers must survive
// for flattenedVisibleAliasTarget/materializeHiddenNodeForAlias to build C's
// single-child alias nesting.
func invisibleUnaryWrapperCollapsible(meta SymbolMetadata) bool {
	if meta.Visible {
		return false
	}
	return !meta.Named
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

// wrapsSameNamedAnonymousToken reports whether parentSym is a visible named rule
// whose single unary child is a distinct anonymous token that shares the parent's
// name AND whose child token C tree-sitter keeps as a visible child (so the node
// retains ChildCount()==1 instead of collapsing to a childless named leaf).
//
// e.g. ruby `nil`, solidity `true`/`false`, css `to`, typst `return`, and the
// LLVM `dso_local`/`unnamed_addr`/… rules all KEEP their same-named anonymous
// token child. By contrast go's `nil`/`true`/`false`/`iota` collapse it away:
// their `$ => 'literal'` rule is inlined by tree-sitter into a single named
// token. The two shapes are byte-identical in the loaded Language's symbol and
// lexer metadata, so the keep-vs-collapse decision is derived from parse-table
// behavior in buildKeepSameNamedAnonChildSymbols (the anon token shifts into 2+
// distinct states only when it is a genuinely extracted/shared token).
func (p *Parser) wrapsSameNamedAnonymousToken(parentSym, childSym Symbol) bool {
	if p == nil || p.language == nil || parentSym == childSym {
		return false
	}
	meta := p.language.SymbolMetadata
	if int(parentSym) < 0 || int(parentSym) >= len(meta) ||
		int(childSym) < 0 || int(childSym) >= len(meta) {
		return false
	}
	parent := meta[parentSym]
	child := meta[childSym]
	// Parent must be a visible named rule; child must be a visible anonymous token.
	if !parent.Visible || !parent.Named || !child.Visible || child.Named {
		return false
	}
	if !p.sameSymbolName(parentSym, childSym) {
		return false
	}
	// General signal: the same-named anon token is a genuinely extracted/shared
	// token (shifts into 2+ distinct parse states), so C keeps it as a child.
	if int(parentSym) < len(p.keepSameNamedAnonChildSymbol) && p.keepSameNamedAnonChildSymbol[parentSym] {
		return true
	}
	// Per-language fallback: a few rules keep their same-named anon token child
	// even though it shifts into only one state — grammar-level multi-site sharing
	// is collapsed onto a single parse state by LR state merging, defeating the
	// >=2-distinct-shift-targets proxy above. These are byte-identical to go's
	// collapse cases in the loaded Language, so they can only be separated by a
	// per-language opt-in verified against the C oracle.
	//   - llvm: unnamed_addr, thread_local, dso_local, ...
	//   - dart: `base` class modifier (its 3 grammar call sites merge to one state)
	if p.language.Name == "llvm" {
		return true
	}
	return p.language.Name == "dart" &&
		int(parentSym) < len(p.language.SymbolNames) &&
		p.language.SymbolNames[parentSym] == "base"
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

func (p *Parser) isSharedAnonymousToken(sym Symbol) bool {
	if p == nil || int(sym) < 0 || int(sym) >= len(p.sharedAnonymousTokenSymbol) {
		return false
	}
	return p.sharedAnonymousTokenSymbol[sym]
}

func (p *Parser) isSharedVisibleAnonymousToken(sym Symbol) bool {
	if !p.isSharedAnonymousToken(sym) || p == nil || p.language == nil {
		return false
	}
	idx := int(sym)
	if idx < 0 || idx >= len(p.language.SymbolMetadata) {
		return false
	}
	meta := p.language.SymbolMetadata[idx]
	return meta.Visible && !meta.Named
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
	if n == nil || alias == 0 {
		return n
	}
	if n.symbol == alias {
		return n
	}
	if shouldAliasMaterializeInvisibleLeafToAnonymousAlias(n, alias, lang) {
		return materializeAnonymousLeafAliasWrapper(arena, lang, n, alias)
	}

	if lang != nil {
		if idx := int(n.symbol); idx < len(lang.SymbolMetadata) && !lang.SymbolMetadata[n.symbol].Visible {
			if child := flattenedVisibleAliasTarget(n, lang.SymbolMetadata, nil, alias, int(lang.TokenCount)); child != nil {
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

func shouldAliasMaterializeInvisibleLeafToAnonymousAlias(n *Node, alias Symbol, lang *Language) bool {
	if n == nil || lang == nil || nodeChildCountNoMaterialize(n) != 0 {
		return false
	}
	childIdx := int(n.symbol)
	aliasIdx := int(alias)
	if childIdx < 0 || childIdx >= len(lang.SymbolMetadata) || aliasIdx < 0 || aliasIdx >= len(lang.SymbolMetadata) {
		return false
	}
	childMeta := lang.SymbolMetadata[childIdx]
	aliasMeta := lang.SymbolMetadata[aliasIdx]
	if childMeta.Visible || !aliasMeta.Visible || aliasMeta.Named {
		return false
	}
	return lang.TokenCount > 0 && uint32(childIdx) < lang.TokenCount && uint32(aliasIdx) >= lang.TokenCount
}

func materializeAnonymousLeafAliasWrapper(arena *nodeArena, lang *Language, n *Node, alias Symbol) *Node {
	named := false
	if lang != nil {
		if aliasIdx := int(alias); aliasIdx >= 0 && aliasIdx < len(lang.SymbolMetadata) {
			named = lang.SymbolMetadata[aliasIdx].Named
		}
	}
	child := cloneNodeInArena(arena, n)
	child.symbol = alias
	child.setNamed(named)
	wrapper := newParentNodeInArena(arena, alias, named, []*Node{child}, nil, n.productionID)
	wrapper.startByte = n.startByte
	wrapper.endByte = n.endByte
	wrapper.startPoint = n.startPoint
	wrapper.endPoint = n.endPoint
	wrapper.parseState = n.parseState
	wrapper.preGotoState = n.preGotoState
	wrapper.rawShape = n.rawShape
	wrapper.dynamicPrecedence = n.dynamicPrecedence
	return wrapper
}

func flattenedVisibleAliasTarget(n *Node, symbolMeta []SymbolMetadata, preservedHidden []bool, alias Symbol, tokenCount int) *Node {
	if n == nil || hiddenTreeHasFieldIDs(n) {
		return nil
	}
	if countFlattenedHiddenChildren(n, symbolMeta, preservedHidden) != 1 {
		return nil
	}
	root := n
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
			// A NAMED visible leaf is a real grammar node (e.g. lua's
			// escape_sequence inside the hidden repeat aliased to
			// string_content). Upstream tree-sitter keeps the wrapper and
			// nests the leaf under the alias: (string_content
			// (escape_sequence)). Only anonymous token-shaped leaves are
			// relabeled in place.
			if idx := int(n.symbol); idx < len(symbolMeta) && symbolMeta[n.symbol].Named {
				return nil
			}
			if shouldAliasPreserveAnonymousLeafWrapper(n.symbol, alias, symbolMeta, tokenCount) {
				return nil
			}
			// Rename-through is also only sound when the leaf covers the whole
			// hidden wrapper. When the wrapper carries extra HIDDEN tokens
			// alongside the lone visible leaf, C tree-sitter keeps the
			// wrapper's span. Relabeling the leaf would shrink the node to
			// the leaf's span, so route to materializeHiddenNodeForAlias
			// instead.
			if n.startByte != root.startByte || n.endByte != root.endByte {
				return nil
			}
			return n
		}
		var next *Node
		for _, child := range n.children {
			if countFlattenedHiddenChildren(child, symbolMeta, preservedHidden) == 0 {
				continue
			}
			next = child
			break
		}
		n = next
	}
	return nil
}

func shouldAliasPreserveAnonymousLeafWrapper(child, alias Symbol, symbolMeta []SymbolMetadata, tokenCount int) bool {
	childIdx := int(child)
	aliasIdx := int(alias)
	if childIdx < 0 || childIdx >= len(symbolMeta) || aliasIdx < 0 || aliasIdx >= len(symbolMeta) {
		return false
	}
	childMeta := symbolMeta[childIdx]
	aliasMeta := symbolMeta[aliasIdx]
	if !childMeta.Visible || childMeta.Named || !aliasMeta.Visible {
		return false
	}
	if tokenCount > 0 && aliasIdx >= tokenCount && aliasMeta.Named {
		return true
	}
	return false
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
	normalizedCount := countFlattenedHiddenChildren(n, symbolMeta, nil)
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
	out := appendFlattenedHiddenChildrenWithFields(children, fieldIDs, fieldSources, 0, n, symbolMeta, nil)
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
	// Memoized: field-ID presence is an immutable property once a subtree is
	// materialized. See nodeFlagFieldIDCacheComputed. Fresh arena nodes start
	// with flags=0 (cache uncomputed); only read/written during reduce on
	// already-built immutable child subtrees, so the cache is never stale.
	if n.flags&nodeFlagFieldIDCacheComputed != 0 {
		return n.flags&nodeFlagFieldIDCacheHasFieldIDs != 0
	}
	result := false
	for _, fid := range n.fieldIDs {
		if fid != 0 {
			result = true
			break
		}
	}
	if !result {
		for _, child := range n.children {
			if hiddenTreeHasFieldIDs(child) {
				result = true
				break
			}
		}
	}
	n.flags |= nodeFlagFieldIDCacheComputed
	if result {
		n.flags |= nodeFlagFieldIDCacheHasFieldIDs
	}
	return result
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
	if pid < len(p.reduceFieldPlans) {
		plan := p.reduceFieldPlans[pid]
		if plan.fieldIDs != nil && int(plan.childCount) == childCount {
			return plan.fieldIDs, plan.inherited
		}
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
