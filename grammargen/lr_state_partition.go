package grammargen

// partitionScannerStates starts with states that have a common grammar core.
// It separates incompatible actions before refining common destinations.
// Groups created by a split are visited after the existing groups.
func partitionScannerStates(tables *LRTables, grammar *NormalizedGrammar) []parserStateGroup {
	var members [][]int
	byCore := make(map[string]int)
	stateGroup := make([]int, tables.StateCount)
	for state, key := range tables.CompactCoreKeys {
		group, found := byCore[key]
		if !found {
			group = len(members)
			byCore[key] = group
			members = append(members, nil)
		}
		stateGroup[state] = group
		members[group] = append(members[group], state)
	}

	// Reduction keys use the same compact field and alias identities as
	// assembly. Conflict resolution has completed when partitioning starts.
	normalized := *grammar
	normalized.Productions = append([]Production(nil), grammar.Productions...)
	compactProductionIDs(&normalized)
	actionsDiffer := func(left, right int) bool {
		for token, leftActions := range tables.ActionTable[left] {
			rightActions, present := tables.ActionTable[right][token]
			if !present {
				// A scanner-owned token must be accepted on both sides.
				// Built-in EOF is supplied independently of the scanner.
				if token != 0 {
					return true
				}
				continue
			}
			if len(leftActions) != len(rightActions) {
				return true
			}
			for i := range leftActions {
				l, r := leftActions[i], rightActions[i]
				if l.kind != r.kind || l.isExtra != r.isExtra || l.repeat != r.repeat {
					return true
				}
				switch l.kind {
				case lrShift:
					if !l.isExtra && stateGroup[l.state] != stateGroup[r.state] {
						return true
					}
				case lrReduce:
					lp, rp := normalized.Productions[l.prodIdx], normalized.Productions[r.prodIdx]
					if lp.LHS != rp.LHS || len(lp.RHS) != len(rp.RHS) ||
						lp.DynPrec != rp.DynPrec || lp.ProductionID != rp.ProductionID || lp.IsExtra != rp.IsExtra {
						return true
					}
				}
			}
		}
		for token := range tables.ActionTable[right] {
			if _, present := tables.ActionTable[left][token]; !present && token != 0 {
				return true
			}
		}
		return false
	}
	destinationsDiffer := func(left, right int) bool {
		for token, leftActions := range tables.ActionTable[left] {
			rightActions, present := tables.ActionTable[right][token]
			if !present {
				continue
			}
			for i, l := range leftActions {
				if l.kind == lrShift && !l.isExtra && stateGroup[l.state] != stateGroup[rightActions[i].state] {
					return true
				}
			}
		}
		for symbol, leftTarget := range tables.GotoTable[left] {
			if rightTarget, present := tables.GotoTable[right][symbol]; present &&
				stateGroup[leftTarget] != stateGroup[rightTarget] {
				return true
			}
		}
		return false
	}
	split := func(incompatible func(int, int) bool) bool {
		changed := false
		for group := 0; group < len(members); group++ {
			states := members[group]
			if len(states) < 2 {
				continue
			}
			moved := make([]bool, len(states))
			var outliers []int
			for i, left := range states {
				if moved[i] {
					continue
				}
				for j := i + 1; j < len(states); j++ {
					if !moved[j] && incompatible(left, states[j]) {
						moved[j] = true
						outliers = append(outliers, states[j])
					}
				}
			}
			if len(outliers) == 0 {
				continue
			}
			kept := states[:0]
			for i, state := range states {
				if !moved[i] {
					kept = append(kept, state)
				}
			}
			members[group] = kept
			for _, state := range outliers {
				stateGroup[state] = len(members)
			}
			members = append(members, outliers)
			changed = true
		}
		return changed
	}
	split(actionsDiffer)
	for split(destinationsDiffer) {
	}

	groups := make([]parserStateGroup, len(members))
	for i, states := range members {
		first := states[0]
		groups[i] = parserStateGroup{states: states, actions: cloneStateActions(tables.ActionTable[first]), gotos: cloneStateGotos(tables.GotoTable[first])}
		for _, state := range states[1:] {
			for symbol, actions := range tables.ActionTable[state] {
				groups[i].actions[symbol] = actions
			}
			for symbol, target := range tables.GotoTable[state] {
				groups[i].gotos[symbol] = target
			}
		}
	}
	return groups
}
