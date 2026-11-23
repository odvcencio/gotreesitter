package grammargen

import (
	"sort"
	"strconv"
)

// minimizeParseStates first merges states with identical resolved behavior. It
// then unions compatible action rows only inside the same C-compatible core.
func minimizeParseStates(tables *LRTables, ng *NormalizedGrammar) int {
	if tables == nil || tables.StateCount < 2 {
		return 0
	}

	before := tables.StateCount
	referenceRows := yamlReferenceScannerRows(ng) && len(tables.CompactCoreKeys) == tables.StateCount
	var groups []parserStateGroup
	if referenceRows {
		groups = partitionScannerStates(tables, ng)
	} else {
		strictGroups := parserStateGroups(tables, nil)
		for {
			next := parserStateGroups(tables, strictGroups)
			if lenGroupSet(next) == lenGroupSet(strictGroups) {
				strictGroups = next
				break
			}
			strictGroups = next
		}

		groups = initialStateGroups(tables, strictGroups)
		groups = mergeCompatibleStateGroups(groups, strictGroups, ng)
		if len(groups) == tables.StateCount {
			return 0
		}
	}

	startGroup := -1
	for groupIndex, group := range groups {
		for _, state := range group.states {
			if state == 0 {
				startGroup = groupIndex
				break
			}
		}
		if startGroup >= 0 {
			break
		}
	}
	if startGroup > 0 {
		groups[0], groups[startGroup] = groups[startGroup], groups[0]
	}

	if referenceRows {
		// Elide transparent unit reductions before ordering states. External
		// scanner row numbers are assigned on first use in this order.
		applyRetainedGroups(tables, groups)
		redirectTransparentUnitReductions(tables, ng)
		pruneUnreferencedParseStates(tables)

		// The error state is assembled separately. Include its leading slot
		// in the permutation to preserve the reference's treatment of ties.
		widths := make([]int, tables.StateCount+1)
		for state := 0; state < tables.StateCount; state++ {
			actions := tables.ActionTable[state]
			width := len(tables.GotoTable[state]) + len(actions)
			for _, extra := range ng.ExtraSymbols {
				if extra < ng.TokenCount() && len(actions[extra]) == 0 {
					width++
				}
			}
			widths[state+1] = width
		}
		order := parserStateWidthOrder(widths)
		groups = make([]parserStateGroup, tables.StateCount)
		for index, id := range order[1:] {
			state := id - 1
			groups[index] = parserStateGroup{
				states:  []int{state},
				actions: tables.ActionTable[state],
				gotos:   tables.GotoTable[state],
			}
		}
	}

	applyRetainedGroups(tables, groups)
	return before - len(groups)
}

func applyRetainedGroups(tables *LRTables, groups []parserStateGroup) {
	stateGroups := make([]int, tables.StateCount)
	for groupIndex, group := range groups {
		for _, state := range group.states {
			stateGroups[state] = groupIndex
		}
	}

	actions := make(map[int]map[int][]lrAction, len(groups))
	gotos := make(map[int]map[int]int, len(groups))
	for groupIndex, group := range groups {
		actions[groupIndex] = remapStateActions(group.actions, stateGroups)
		gotos[groupIndex] = remapStateGotos(group.gotos, stateGroups)
	}
	tables.ActionTable = actions
	tables.GotoTable = gotos
	tables.StateCount = len(groups)
	tables.CompactCoreKeys = nil
}

type parserStateGroup struct {
	states         []int
	actions        map[int][]lrAction
	gotos          map[int]int
	compactCoreKey string
}

func initialStateGroups(tables *LRTables, strictGroups []int) []parserStateGroup {
	count := lenGroupSet(strictGroups)
	groups := make([]parserStateGroup, count)
	for state, groupIndex := range strictGroups {
		group := &groups[groupIndex]
		group.states = append(group.states, state)
		if len(group.states) > 1 {
			continue
		}
		group.actions = cloneStateActions(tables.ActionTable[state])
		group.gotos = cloneStateGotos(tables.GotoTable[state])
		if state < len(tables.CompactCoreKeys) {
			group.compactCoreKey = tables.CompactCoreKeys[state]
		}
	}
	for groupIndex := range groups {
		group := &groups[groupIndex]
		for _, state := range group.states[1:] {
			if state >= len(tables.CompactCoreKeys) || tables.CompactCoreKeys[state] != group.compactCoreKey {
				group.compactCoreKey = ""
				break
			}
		}
	}
	return groups
}

func mergeCompatibleStateGroups(groups []parserStateGroup, strictGroups []int, ng *NormalizedGrammar) []parserStateGroup {
	if len(groups) < 2 || ng == nil {
		return groups
	}
	matchers := buildTerminalStartMatchers(ng.Terminals)
	byCore := make(map[string][]int)
	for groupIndex, group := range groups {
		if group.compactCoreKey != "" {
			byCore[group.compactCoreKey] = append(byCore[group.compactCoreKey], groupIndex)
		}
	}

	merged := make([]parserStateGroup, 0, len(groups))
	consumed := make([]bool, len(groups))
	for groupIndex, group := range groups {
		if consumed[groupIndex] {
			continue
		}
		if group.compactCoreKey == "" {
			merged = append(merged, group)
			consumed[groupIndex] = true
			continue
		}
		for _, candidateIndex := range byCore[group.compactCoreKey] {
			if consumed[candidateIndex] {
				continue
			}
			candidate := groups[candidateIndex]
			placed := false
			for target := range merged {
				if merged[target].compactCoreKey != candidate.compactCoreKey ||
					!stateGroupsCompatible(&merged[target], &candidate, strictGroups, ng, matchers) {
					continue
				}
				mergeStateGroups(&merged[target], candidate)
				placed = true
				break
			}
			if !placed {
				merged = append(merged, candidate)
			}
			consumed[candidateIndex] = true
		}
	}
	return merged
}

func stateGroupsCompatible(left, right *parserStateGroup, strictGroups []int, ng *NormalizedGrammar, matchers map[int]terminalStartMatcher) bool {
	if len(left.gotos) != len(right.gotos) {
		return false
	}
	for symbol, target := range left.gotos {
		otherTarget, ok := right.gotos[symbol]
		if !ok || strictGroups[target] != strictGroups[otherTarget] {
			return false
		}
	}

	// A subset merge can introduce external lookaheads even when only one
	// side has new terminals. Preserve the scanner context in both directions.
	if ng.externalLexerOnly {
		for _, external := range ng.ExternalSymbols {
			_, validLeft := left.actions[external]
			_, validRight := right.actions[external]
			if validLeft != validRight {
				return false
			}
		}
	}
	for symbol, actions := range left.actions {
		if other, ok := right.actions[symbol]; ok && !sameActionLists(actions, other, strictGroups) {
			return false
		}
	}
	for leftSymbol := range left.actions {
		if _, common := right.actions[leftSymbol]; common {
			continue
		}
		for rightSymbol := range right.actions {
			if _, common := left.actions[rightSymbol]; common {
				continue
			}
			if !disjointTerminals(leftSymbol, rightSymbol, ng, matchers) {
				return false
			}
		}
	}
	return true
}

func disjointTerminals(left, right int, ng *NormalizedGrammar, matchers map[int]terminalStartMatcher) bool {
	if left <= 0 || right <= 0 || left >= ng.TokenCount() || right >= ng.TokenCount() {
		return false
	}
	for _, external := range ng.ExternalSymbols {
		if left == external || right == external {
			return false
		}
	}
	if left == ng.WordSymbolID || right == ng.WordSymbolID {
		return false
	}
	for _, keyword := range ng.KeywordSymbols {
		if left == keyword || right == keyword {
			return false
		}
	}
	leftMatcher, leftOK := matchers[left]
	rightMatcher, rightOK := matchers[right]
	return leftOK && rightOK && !terminalStartMatchersOverlap(leftMatcher, rightMatcher)
}

func sameActionLists(left, right []lrAction, groups []int) bool {
	if len(left) != len(right) {
		return false
	}
	leftKeys := make([]string, 0, len(left))
	rightKeys := make([]string, 0, len(right))
	for _, action := range left {
		leftKeys = append(leftKeys, parserActionSignature(action, groups))
	}
	for _, action := range right {
		rightKeys = append(rightKeys, parserActionSignature(action, groups))
	}
	sort.Strings(leftKeys)
	sort.Strings(rightKeys)
	for i := range leftKeys {
		if leftKeys[i] != rightKeys[i] {
			return false
		}
	}
	return true
}

func mergeStateGroups(target *parserStateGroup, source parserStateGroup) {
	target.states = append(target.states, source.states...)
	for symbol, actions := range source.actions {
		if _, exists := target.actions[symbol]; !exists {
			target.actions[symbol] = append([]lrAction(nil), actions...)
		}
	}
}

func parserStateGroups(tables *LRTables, previous []int) []int {
	groups := make([]int, tables.StateCount)
	bySignature := make(map[string]int, tables.StateCount)
	for state := 0; state < tables.StateCount; state++ {
		signature := parserStateSignature(tables, state, previous)
		group, ok := bySignature[signature]
		if !ok {
			group = len(bySignature)
			bySignature[signature] = group
		}
		groups[state] = group
	}
	return groups
}

func lenGroupSet(groups []int) int {
	max := -1
	for _, group := range groups {
		if group > max {
			max = group
		}
	}
	return max + 1
}

func parserStateSignature(tables *LRTables, state int, groups []int) string {
	buf := make([]byte, 0, 256)
	appendInt := func(value int) {
		buf = strconv.AppendInt(buf, int64(value), 10)
		buf = append(buf, ';')
	}
	for _, symbol := range sortedActionSymbols(tables.ActionTable[state]) {
		appendInt(symbol)
		actionSignatures := make([]string, 0, len(tables.ActionTable[state][symbol]))
		for _, action := range tables.ActionTable[state][symbol] {
			actionSignatures = append(actionSignatures, parserActionSignature(action, groups))
		}
		sort.Strings(actionSignatures)
		for _, action := range actionSignatures {
			appendInt(len(action))
			buf = append(buf, action...)
		}
	}
	buf = append(buf, '|')
	for _, symbol := range sortedGotoSymbols(tables.GotoTable[state]) {
		appendInt(symbol)
		if groups == nil {
			appendInt(0)
		} else {
			appendInt(groups[tables.GotoTable[state][symbol]])
		}
	}
	return string(buf)
}

func parserActionSignature(action lrAction, groups []int) string {
	buf := make([]byte, 0, 128)
	appendInt := func(value int) {
		buf = strconv.AppendInt(buf, int64(value), 10)
		buf = append(buf, ';')
	}
	appendBool := func(value bool) {
		if value {
			appendInt(1)
		} else {
			appendInt(0)
		}
	}
	appendInts := func(values []int) {
		values = append([]int(nil), values...)
		sort.Ints(values)
		appendInt(len(values))
		for _, value := range values {
			appendInt(value)
		}
	}
	appendContributors := func(values []lrShiftContributor) {
		parts := make([]string, 0, len(values))
		for _, value := range values {
			parts = append(parts, strconv.Itoa(value.lhsSym)+":"+strconv.Itoa(value.prec)+":"+strconv.FormatBool(value.hasPrec)+":"+strconv.Itoa(int(value.assoc)))
		}
		sort.Strings(parts)
		appendInt(len(parts))
		for _, part := range parts {
			appendInt(len(part))
			buf = append(buf, part...)
		}
	}

	appendInt(int(action.kind))
	if action.kind == lrShift && groups != nil {
		appendInt(groups[action.state])
	} else {
		appendInt(0)
	}
	appendInt(action.prodIdx)
	appendInt(action.prec)
	appendBool(action.hasPrec)
	appendInt(int(action.assoc))
	appendInt(action.lhsSym)
	appendInts(action.lhsSyms)
	appendContributors(action.shiftContributors)
	appendContributors(action.conflictContributors)
	appendBool(action.isExtra)
	appendBool(action.repeat)
	appendInt(action.repeatLHS)
	appendInts(action.repeatLHSSyms)
	return string(buf)
}

func cloneStateActions(actions map[int][]lrAction) map[int][]lrAction {
	result := make(map[int][]lrAction, len(actions))
	for symbol, list := range actions {
		result[symbol] = append([]lrAction(nil), list...)
	}
	return result
}

func cloneStateGotos(gotos map[int]int) map[int]int {
	result := make(map[int]int, len(gotos))
	for symbol, state := range gotos {
		result[symbol] = state
	}
	return result
}

func sortedActionSymbols(actions map[int][]lrAction) []int {
	symbols := make([]int, 0, len(actions))
	for symbol := range actions {
		symbols = append(symbols, symbol)
	}
	sort.Ints(symbols)
	return symbols
}

func sortedGotoSymbols(gotos map[int]int) []int {
	symbols := make([]int, 0, len(gotos))
	for symbol := range gotos {
		symbols = append(symbols, symbol)
	}
	sort.Ints(symbols)
	return symbols
}

func remapStateActions(actions map[int][]lrAction, groups []int) map[int][]lrAction {
	result := cloneStateActions(actions)
	for _, list := range result {
		for i := range list {
			if list[i].kind == lrShift {
				list[i].state = groups[list[i].state]
			}
		}
	}
	return result
}

func remapStateGotos(gotos map[int]int, groups []int) map[int]int {
	result := make(map[int]int, len(gotos))
	for symbol, state := range gotos {
		result[symbol] = groups[state]
	}
	return result
}
