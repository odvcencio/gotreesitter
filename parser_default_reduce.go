package gotreesitter

import (
	"fmt"
	"os"
)

type eagerDefaultReduceAction struct {
	ok     bool
	action ParseAction
}

func parseEagerDefaultReduceEnabled() bool {
	parseEagerDefaultOnce.Do(func() {
		switch os.Getenv("GOT_EAGER_DEFAULT_REDUCE") {
		case "1", "true", "TRUE", "True":
			parseEagerDefault = true
		default:
			parseEagerDefault = false
		}
	})
	return parseEagerDefault
}

func parseEagerDefaultReduceDebugEnabled() bool {
	parseEagerDefaultDebugOnce.Do(func() {
		parseEagerDefaultDebug = os.Getenv("GOT_EAGER_DEFAULT_REDUCE_DEBUG") == "1"
	})
	return parseEagerDefaultDebug
}

func buildEagerDefaultReduceActions(v parserActionTableView) []eagerDefaultReduceAction {
	if v.language == nil || len(v.language.ParseActions) == 0 {
		return nil
	}
	stateCount := parserRuntimeStateCount(v)
	if stateCount == 0 {
		return nil
	}
	out := make([]eagerDefaultReduceAction, stateCount)
	tokenCount := Symbol(v.language.TokenCount)
	for state := 0; state < stateCount; state++ {
		var candidate ParseAction
		found := false
		invalid := false
		v.forEachActionIndexInState(StateID(state), func(sym Symbol, idx uint16) bool {
			if sym >= tokenCount {
				return true
			}
			if int(idx) >= len(v.classifiedActions) {
				invalid = true
				return false
			}
			classified := v.classifiedActions[idx]
			if classified.class == classifiedParseActionSingleShift &&
				(classified.action.Extra || classified.action.ExtraChain) {
				return true
			}
			if classified.class != classifiedParseActionSingleReduce ||
				classified.action.ChildCount == 0 {
				invalid = true
				return false
			}
			if !found {
				candidate = classified.action
				found = true
				return true
			}
			if !sameEagerDefaultReduce(candidate, classified.action) {
				invalid = true
				return false
			}
			return true
		})
		if found && !invalid {
			out[state] = eagerDefaultReduceAction{ok: true, action: candidate}
		}
	}
	return out
}

func (p *Parser) isExternalToken(sym Symbol) bool {
	if p == nil || p.language == nil {
		return false
	}
	for _, external := range p.language.ExternalSymbols {
		if external == sym {
			return true
		}
	}
	return false
}

func parserRuntimeStateCount(v parserActionTableView) int {
	if v.language == nil {
		return 0
	}
	stateCount := int(v.language.StateCount)
	if stateCount == 0 {
		stateCount = len(v.language.ParseTable)
	}
	if smallStates := v.smallBase + len(v.language.SmallParseTableMap); smallStates > stateCount {
		stateCount = smallStates
	}
	if len(v.language.LexModes) > stateCount {
		stateCount = len(v.language.LexModes)
	}
	return stateCount
}

func sameEagerDefaultReduce(a, b ParseAction) bool {
	return a.Type == ParseActionReduce &&
		b.Type == ParseActionReduce &&
		a.Symbol == b.Symbol &&
		a.ChildCount == b.ChildCount &&
		a.DynamicPrecedence == b.DynamicPrecedence &&
		a.ProductionID == b.ProductionID
}

func (p *Parser) eagerDefaultReduceAction(state StateID) (ParseAction, bool) {
	if p == nil || int(state) >= len(p.eagerDefaultReduces) {
		return ParseAction{}, false
	}
	entry := p.eagerDefaultReduces[state]
	return entry.action, entry.ok
}

func (p *Parser) applyEagerDefaultReduces(source []byte, phase string, stacks []glrStack, nodeCount *int, arena *nodeArena, entryScratch *glrEntryScratch, gssScratch *gssScratch, tmpEntries *[]stackEntry, deferParentLinks bool, trackChildErrors *bool) bool {
	if p == nil || len(stacks) == 0 || len(p.eagerDefaultReduces) == 0 {
		return false
	}
	anyReduced := false
	debug := parseEagerDefaultReduceDebugEnabled()
	type seenKey struct {
		stack int
		sig   reduceChainSignature
	}
	seen := make(map[seenKey]int)
	for step := 0; step < maxConsecutivePrimaryReduces; step++ {
		progressed := false
		for i := range stacks {
			s := &stacks[i]
			if s.dead || s.accepted || s.shifted || s.cPaused || s.depth() == 0 {
				continue
			}
			state := s.top().state
			act, ok := p.eagerDefaultReduceAction(state)
			if !ok {
				if debug {
					fmt.Printf("  stack[%d] EAGER-DEFAULT-SKIP phase=%s state=%d\n", i, phase, state)
				}
				continue
			}
			sig := reduceChainSignature{
				state:        state,
				depth:        s.depth(),
				symbol:       act.Symbol,
				childCount:   act.ChildCount,
				productionID: act.ProductionID,
			}
			key := seenKey{stack: i, sig: sig}
			seen[key]++
			if seen[key] > maxRepeatedReduceChainSignature+1 {
				if p.glrTrace || debug {
					fmt.Printf("      -> EAGER-DEFAULT-REDUCE CYCLE state=%d depth=%d sym=%d prod=%d\n",
						state, sig.depth, act.Symbol, act.ProductionID)
				}
				continue
			}
			if p.glrTrace || debug {
				fmt.Printf("  stack[%d] EAGER-DEFAULT-REDUCE phase=%s state=%d sym=%d cnt=%d prod=%d\n",
					i, phase, state, act.Symbol, act.ChildCount, act.ProductionID)
			}
			reduceTok := eagerDefaultReduceToken(s)
			reduced := false
			p.applyAction(source, s, act, reduceTok, &reduced, nodeCount, arena, entryScratch, gssScratch, tmpEntries, deferParentLinks, trackChildErrors)
			if reduced {
				anyReduced = true
				progressed = true
			}
		}
		if !progressed {
			return anyReduced
		}
	}
	return anyReduced
}

type externalDefaultReduceSeenKey struct {
	stack int
	sig   reduceChainSignature
}

func (p *Parser) canApplyExternalNoActionDefaultReduce(tok Token, stacks []glrStack) bool {
	if p == nil || p.language == nil || len(stacks) == 0 || len(p.eagerDefaultReduces) == 0 || tok.NoLookahead || !p.isExternalToken(tok.Symbol) {
		return false
	}
	if p.anyLiveStackShifted(stacks) {
		return false
	}
	eligible := 0
	for i := range stacks {
		s := &stacks[i]
		if !externalDefaultReduceStackEligible(s) {
			continue
		}
		eligible++
		state := s.top().state
		if p.lookupActionIndex(state, tok.Symbol) != 0 {
			return false
		}
		if _, ok := p.eagerDefaultReduceAction(state); !ok {
			return false
		}
	}
	return eligible != 0
}

func (p *Parser) externalNoActionDefaultReducesStable(tok Token, stacks []glrStack) bool {
	if p == nil || p.language == nil || len(p.eagerDefaultReduces) == 0 || tok.NoLookahead || !p.isExternalToken(tok.Symbol) {
		return true
	}
	for i := range stacks {
		s := &stacks[i]
		if !externalDefaultReduceStackEligible(s) {
			continue
		}
		state := s.top().state
		if p.lookupActionIndex(state, tok.Symbol) == 0 {
			if _, ok := p.eagerDefaultReduceAction(state); ok {
				return false
			}
		}
	}
	return true
}

func (p *Parser) applyExternalNoActionDefaultReduceStep(source []byte, tok Token, stacks []glrStack, nodeCount *int, arena *nodeArena, entryScratch *glrEntryScratch, gssScratch *gssScratch, tmpEntries *[]stackEntry, deferParentLinks bool, trackChildErrors *bool, seen map[externalDefaultReduceSeenKey]int) bool {
	if !p.canApplyExternalNoActionDefaultReduce(tok, stacks) {
		return false
	}
	anyReduced := false
	debug := parseEagerDefaultReduceDebugEnabled()
	for i := range stacks {
		s := &stacks[i]
		if !externalDefaultReduceStackEligible(s) {
			continue
		}
		state := s.top().state
		act, ok := p.eagerDefaultReduceAction(state)
		if !ok {
			continue
		}
		sig := reduceChainSignature{
			state:        state,
			depth:        s.depth(),
			symbol:       act.Symbol,
			childCount:   act.ChildCount,
			productionID: act.ProductionID,
		}
		if seen != nil {
			key := externalDefaultReduceSeenKey{stack: i, sig: sig}
			seen[key]++
			if seen[key] > maxRepeatedReduceChainSignature+1 {
				if p.glrTrace || debug {
					fmt.Printf("      -> EXTERNAL-DEFAULT-REDUCE CYCLE state=%d depth=%d sym=%d prod=%d\n",
						state, sig.depth, act.Symbol, act.ProductionID)
				}
				continue
			}
		}
		if p.glrTrace || debug {
			fmt.Printf("  stack[%d] EXTERNAL-DEFAULT-REDUCE state=%d lookahead=%d sym=%d cnt=%d prod=%d\n",
				i, state, tok.Symbol, act.Symbol, act.ChildCount, act.ProductionID)
		}
		reduceTok := eagerDefaultReduceToken(s)
		reduced := false
		p.applyAction(source, s, act, reduceTok, &reduced, nodeCount, arena, entryScratch, gssScratch, tmpEntries, deferParentLinks, trackChildErrors)
		if reduced {
			anyReduced = true
		}
	}
	return anyReduced
}

// settleEagerDefaultReduceChainForReuse applies the pending eager-default
// (unconditional) reduce chain on a SINGLE live stack BEFORE the incremental
// reuse check, so tryReuseSubtree observes the same top-of-stack state the old
// tree recorded as a candidate's PreGotoState. This closes campaign O(edit)
// workstream W1b's settling gap (spec.campaign.oedit): the W1 primary loop and
// its conservative fallback both compare against the settled goto state, but
// tryReuseSubtree runs before the dispatch loop's reduce chain executes, so on
// Go's top-level declaration list the live top-of-stack state was one (or a
// short chain) of pending unconditional reduces "behind" the recorded state,
// and every non-leaf sibling candidate was rejected (ReuseRejectRootNonLeaf
// Changed).
//
// Soundness (decision-0008, no unsound shortcuts). Every reduce this performs
// is the top state's eager-default reduce -- a reduce the parse table fires for
// EVERY lookahead that has an action in that state (buildEagerDefaultReduce
// Actions proves the whole state reduces to the same production regardless of
// lookahead). It is therefore lookahead-INDEPENDENT. Each step is applied with
// the REAL lookahead token via the same applyReduceActionDispatch the dispatch
// loop's reduce chain uses (chainSingleReduceActions), so the resulting stack
// is byte-identical to what dispatch would build next -- this only moves that
// unconditional prefix earlier. It NEVER settles a lookahead-dependent reduce:
// it stops the instant the top state is not eager-default, which is exactly
// where a lookahead-specific decision would begin. If reuse then falls through,
// the dispatch loop resumes from the settled state and completes the identical
// chain, so the final tree is unchanged whether reuse succeeds or not.
//
// The loop also stops when: the current lookahead has no eager-default reduce
// action in the top state (an extra-token shift or a genuine no-action /
// error-recovery cell -- dispatch would NOT reduce next, so neither may this);
// a reduce forks the stack via a GSS multi-link (forked=true, so the caller
// drains the pending forks and skips reuse for this pass, since reuse requires
// a single stack); or the stack dies / accepts / shifts. A step cap plus a
// per-step reduce-chain signature guard mirror chainSingleReduceActions so a
// pathological unit-reduce loop cannot spin here -- if it caps out, the
// dispatch loop's certified reduce-chain cycle recovery takes over.
func (p *Parser) settleEagerDefaultReduceChainForReuse(source []byte, s *glrStack, tok Token, nodeCount *int, arena *nodeArena, entryScratch *glrEntryScratch, gssScratch *gssScratch, tmpEntries *[]stackEntry, deferParentLinks bool, trackChildErrors *bool, forked *bool) bool {
	if incrCensusEnabled {
		censusPhase := incrCensusEnter("selection/eager_settle")
		defer incrCensusLeave(censusPhase)
	}
	if p == nil || s == nil || len(p.eagerDefaultReduces) == 0 || tok.NoLookahead {
		return false
	}
	if s.dead || s.accepted || s.shifted || s.cPaused || s.depth() == 0 {
		return false
	}
	anyReduced := false
	var lastSig reduceChainSignature
	repeatedSigCount := 0
	for step := 0; step < maxConsecutivePrimaryReduces; step++ {
		state := s.top().state
		act, ok := p.eagerDefaultReduceAction(state)
		if !ok {
			return anyReduced
		}
		// Settle only when the dispatch loop would itself reduce for THIS
		// lookahead now: the current token must resolve to exactly this
		// eager-default reduce in the action table. If tok instead takes an
		// extra-token shift, or has no action at all (error recovery), the
		// dispatch loop does NOT reduce next, so neither may we -- defer.
		entry := p.lookupAction(state, tok.Symbol)
		if entry == nil || len(entry.Actions) != 1 ||
			entry.Actions[0].Type != ParseActionReduce ||
			!sameEagerDefaultReduce(entry.Actions[0], act) {
			return anyReduced
		}
		var repeated bool
		repeatedSigCount, repeated = noteRepeatedReduceChainAction(&lastSig, repeatedSigCount, state, s.depth(), act)
		if repeated {
			// Pathological unit-reduce loop: leave it for the dispatch loop's
			// certified reduce-chain cycle recovery.
			return anyReduced
		}
		p.applyReduceActionDispatch(source, s, act, tok, &anyReduced, nodeCount, arena, entryScratch, gssScratch, tmpEntries, deferParentLinks, trackChildErrors)
		if len(p.pendingForkStacks) > 0 {
			if forked != nil {
				*forked = true
			}
			return anyReduced
		}
		if s.dead || s.accepted || s.shifted {
			return anyReduced
		}
	}
	return anyReduced
}

// settleDeterministicReduceChainForReuse advances a single live stack to a
// candidate's recorded pre-goto state using either the unique REDUCE action
// selected by the real current lookahead or the same certified conflict choice
// used by normal dispatch. Unlike the eager-default settle above, these
// reductions may depend on the lookahead; they are still exact scheduler replay
// because no new choice is made here. This is the ownership bridge for hidden
// repetition nodes erased from the visible tree: block reuse may push one
// visible sibling, then the ordinary parser must reduce its hidden list wrapper
// before the next visible sibling owns the same frontier as it did in the
// original parse.
//
// The function fails closed on unresolved conflicts, shifts,
// recovery/no-action cells, forks, cycles, or a target it cannot reach through
// reductions alone.
func (p *Parser) settleDeterministicReduceChainForReuse(source []byte, s *glrStack, tok Token, target StateID, maxStacksSeen int, reuse *reuseCursor, nodeCount *int, arena *nodeArena, entryScratch *glrEntryScratch, gssScratch *gssScratch, tmpEntries *[]stackEntry, deferParentLinks bool, trackChildErrors *bool, forked *bool) (bool, bool) {
	if incrCensusEnabled {
		censusPhase := incrCensusEnter("selection/ownership_settle")
		defer incrCensusLeave(censusPhase)
	}
	if p == nil || s == nil || tok.NoLookahead || s.dead || s.accepted || s.shifted || s.cPaused || s.depth() == 0 {
		return false, false
	}
	if s.top().state == target {
		return false, true
	}
	anyReduced := false
	var lastSig reduceChainSignature
	repeatedSigCount := 0
	for step := 0; step < maxConsecutivePrimaryReduces; step++ {
		state := s.top().state
		entry := p.lookupAction(state, tok.Symbol)
		if entry == nil || len(entry.Actions) == 0 {
			return anyReduced, false
		}
		var act ParseAction
		reduceFromConflict := false
		switch len(entry.Actions) {
		case 1:
			act = entry.Actions[0]
		default:
			chosen, ok := p.deterministicConflictChoiceForDispatch(source, s, tok, state, entry.Actions, maxStacksSeen, reuse)
			if !ok || chosen.Type != ParseActionReduce {
				return anyReduced, false
			}
			act = chosen
			reduceFromConflict = true
		}
		if act.Type != ParseActionReduce {
			return anyReduced, false
		}
		var repeated bool
		repeatedSigCount, repeated = noteRepeatedReduceChainAction(&lastSig, repeatedSigCount, state, s.depth(), act)
		if repeated {
			return anyReduced, false
		}
		stepReduced := false
		previousReduceActionConflict := p.reduceActionConflict
		// Normal dispatch scopes this flag to one action. Preserve the
		// conflict-selected reduction's fragility without leaking it into
		// subsequent unique reductions or the following reuse step.
		p.reduceActionConflict = reduceFromConflict
		p.applyReduceActionDispatch(source, s, act, tok, &stepReduced, nodeCount, arena, entryScratch, gssScratch, tmpEntries, deferParentLinks, trackChildErrors)
		p.reduceActionConflict = previousReduceActionConflict
		if stepReduced {
			anyReduced = true
		}
		if len(p.pendingForkStacks) > 0 {
			if forked != nil {
				*forked = true
			}
			return anyReduced, false
		}
		if s.dead || s.accepted || s.shifted || !stepReduced {
			return anyReduced, false
		}
		if s.top().state == target {
			return anyReduced, true
		}
	}
	return anyReduced, false
}

func (p *Parser) anyLiveStackShifted(stacks []glrStack) bool {
	for i := range stacks {
		if !stacks[i].dead && stacks[i].shifted {
			return true
		}
	}
	return false
}

func externalDefaultReduceStackEligible(s *glrStack) bool {
	return s != nil && !s.dead && !s.accepted && !s.shifted && !s.cPaused && s.depth() > 0
}

func eagerDefaultReduceToken(s *glrStack) Token {
	if s == nil || s.depth() == 0 {
		return Token{}
	}
	top := s.top()
	endByte := s.byteOffset
	endPoint := Point{}
	if stackEntryHasNode(top) {
		endByte = stackEntryNodeEndByte(top)
		endPoint = stackEntryNodeEndPoint(top)
	}
	return Token{
		Symbol:     errorSymbol,
		StartByte:  endByte,
		EndByte:    endByte,
		StartPoint: endPoint,
		EndPoint:   endPoint,
	}
}
