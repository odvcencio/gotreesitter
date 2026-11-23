package grammargen

// redirectTransparentUnitReductions replaces invisible one-child reductions with
// an immediately available predecessor goto after action resolution.
func redirectTransparentUnitReductions(tables *LRTables, ng *NormalizedGrammar) {
	fieldCarriers := compactInheritedFieldCarriers(ng)
	aliased := make([]bool, len(ng.Symbols))
	for symbol := range ng.defaultAliasDisplays {
		aliased[symbol] = true
	}
	for _, production := range ng.Productions {
		for _, alias := range production.Aliases {
			aliased[production.RHS[alias.ChildIndex]] = true
		}
	}

	unit := make([]int, tables.StateCount)
	for state := 1; state < tables.StateCount; state++ {
		symbol := 0
		for _, actions := range tables.ActionTable[state] {
			for _, action := range actions {
				if action.kind == lrShift && action.isExtra {
					continue
				}
				if action.kind != lrReduce {
					symbol = -1
					break
				}
				p := ng.Productions[action.prodIdx]
				s := ng.Symbols[p.LHS]
				if len(p.RHS) != 1 || p.DynPrec != 0 || p.IsExtra ||
					len(p.Fields) != 0 || len(p.Aliases) != 0 || aliased[p.LHS] || fieldCarriers[p.RHS[0]] ||
					s.Visible || s.Supertype || s.IsExtra ||
					(symbol != 0 && symbol != p.LHS) {
					symbol = -1
					break
				}
				symbol = p.LHS
			}
			if symbol < 0 {
				break
			}
		}
		unit[state] = symbol
	}
	redirect := func(from, target int) int {
		original := target
		for hop := 0; unit[target] > 0; hop++ {
			next, ok := tables.GotoTable[from][unit[target]]
			if !ok {
				return target
			}
			if hop == len(unit) || next == target || next == original {
				return original
			}
			target = next
		}
		return target
	}
	for state := 0; state < tables.StateCount; state++ {
		actions := cloneStateActions(tables.ActionTable[state])
		gotos := cloneStateGotos(tables.GotoTable[state])
		for symbol, target := range gotos {
			next := redirect(state, target)
			if next != target {
				gotos[symbol] = next
			}
		}
		for _, list := range actions {
			for i := range list {
				a := &list[i]
				if a.kind != lrShift || a.isExtra {
					continue
				}
				next := redirect(state, a.state)
				if next != a.state {
					a.state = next
				}
			}
		}
		tables.ActionTable[state] = actions
		tables.GotoTable[state] = gotos
	}
}

func pruneUnreferencedParseStates(tables *LRTables) {
	referenced := make([]bool, tables.StateCount)
	referenced[0] = true
	for state := 0; state < tables.StateCount; state++ {
		for _, list := range tables.ActionTable[state] {
			for _, action := range list {
				if action.kind == lrShift && !action.isExtra {
					referenced[action.state] = true
				}
			}
		}
		for _, target := range tables.GotoTable[state] {
			referenced[target] = true
		}
	}
	var groups []parserStateGroup
	for state, keep := range referenced {
		if keep {
			groups = append(groups, parserStateGroup{
				states: []int{state}, actions: tables.ActionTable[state], gotos: tables.GotoTable[state],
			})
		}
	}
	applyRetainedGroups(tables, groups)
}
