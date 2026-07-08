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

func buildEagerDefaultReduceActions(p *Parser) []eagerDefaultReduceAction {
	if p == nil || p.language == nil || len(p.language.ParseActions) == 0 {
		return nil
	}
	stateCount := parserRuntimeStateCount(p)
	if stateCount == 0 {
		return nil
	}
	out := make([]eagerDefaultReduceAction, stateCount)
	tokenCount := Symbol(p.language.TokenCount)
	for state := 0; state < stateCount; state++ {
		var candidate ParseAction
		found := false
		invalid := false
		p.forEachActionIndexInState(StateID(state), func(sym Symbol, idx uint16) bool {
			if sym >= tokenCount {
				return true
			}
			if int(idx) >= len(p.classifiedActions) {
				invalid = true
				return false
			}
			classified := p.classifiedActions[idx]
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

func parserRuntimeStateCount(p *Parser) int {
	if p == nil || p.language == nil {
		return 0
	}
	stateCount := int(p.language.StateCount)
	if stateCount == 0 {
		stateCount = len(p.language.ParseTable)
	}
	if smallStates := p.smallBase + len(p.language.SmallParseTableMap); smallStates > stateCount {
		stateCount = smallStates
	}
	if len(p.language.LexModes) > stateCount {
		stateCount = len(p.language.LexModes)
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
