package gotreesitter

// RepairNoLookaheadLexModes marks parser states as no-lookahead when they only
// need EOF-triggered reductions plus external/trivia handling. Tree-sitter's C
// runtime uses these states to reduce before lexing the next real token.
func RepairNoLookaheadLexModes(lang *Language) {
	if lang == nil || len(lang.LexModes) == 0 || len(lang.ParseActions) == 0 {
		return
	}

	externalSet := make(map[Symbol]struct{}, len(lang.ExternalSymbols))
	for _, sym := range lang.ExternalSymbols {
		externalSet[sym] = struct{}{}
	}

	for state := 0; state < len(lang.LexModes) && state < int(lang.StateCount); state++ {
		if lang.LexModes[state].LexState == noLookaheadLexState {
			continue
		}

		eofIdx := lookupRepairActionIndex(lang, StateID(state), 0)
		if eofIdx == 0 || int(eofIdx) >= len(lang.ParseActions) {
			continue
		}
		eofEntry := lang.ParseActions[eofIdx]
		if len(eofEntry.Actions) == 0 {
			continue
		}

		allReduce := true
		for _, act := range eofEntry.Actions {
			if act.Type != ParseActionReduce {
				allReduce = false
				break
			}
		}
		if !allReduce {
			continue
		}

		needsRealLookahead := false
		for sym := Symbol(1); uint32(sym) < lang.TokenCount; sym++ {
			if _, ok := externalSet[sym]; ok {
				continue
			}
			idx := lookupRepairActionIndex(lang, StateID(state), sym)
			if idx == 0 || int(idx) >= len(lang.ParseActions) {
				continue
			}
			if repairEntryIsTriviaOnly(lang.ParseActions[idx]) {
				continue
			}
			needsRealLookahead = true
			break
		}

		if !needsRealLookahead {
			lang.LexModes[state].LexState = noLookaheadLexState
		}
	}
}

func repairEntryIsTriviaOnly(entry ParseActionEntry) bool {
	if len(entry.Actions) == 0 {
		return false
	}
	for _, act := range entry.Actions {
		if !act.Extra && !act.ExtraChain {
			return false
		}
	}
	return true
}

func lookupRepairActionIndex(lang *Language, state StateID, sym Symbol) uint16 {
	if lang == nil {
		return 0
	}

	denseLimit := int(lang.LargeStateCount)
	if denseLimit == 0 {
		denseLimit = len(lang.ParseTable)
	}
	if int(state) < denseLimit {
		if int(state) >= len(lang.ParseTable) {
			return 0
		}
		row := lang.ParseTable[state]
		if int(sym) >= len(row) {
			return 0
		}
		return row[sym]
	}

	smallIdx := int(state) - int(lang.LargeStateCount)
	if smallIdx < 0 || smallIdx >= len(lang.SmallParseTableMap) {
		return 0
	}
	table := lang.SmallParseTable
	offset := lang.SmallParseTableMap[smallIdx]
	if int(offset) >= len(table) {
		return 0
	}
	groupCount := table[offset]
	pos := int(offset) + 1
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
			if Symbol(table[pos]) == sym {
				return sectionValue
			}
			pos++
		}
	}
	return 0
}
