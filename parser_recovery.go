package gotreesitter

func buildStateRecoverTable(lang *Language) []bool {
	_, table, _ := buildRecoverActionsByState(lang)
	return table
}

// buildKeywordStates precomputes whether each parser state can produce a
// keyword token, so keyword promotion can skip states where it is impossible.
func buildKeywordStates(lang *Language) []bool {
	if lang == nil || lang.KeywordCaptureToken == 0 || len(lang.KeywordLexStates) == 0 {
		return nil
	}

	symbolCount := int(lang.SymbolCount)
	if symbolCount <= 0 {
		symbolCount = len(lang.SymbolNames)
	}
	if symbolCount <= 0 {
		symbolCount = 64
	}
	keywordSymbols := make([]bool, symbolCount)
	ensureSymbolCap := func(sym Symbol) {
		idx := int(sym)
		if idx < len(keywordSymbols) {
			return
		}
		newLen := len(keywordSymbols)
		if newLen == 0 {
			newLen = 64
		}
		for idx >= newLen {
			newLen *= 2
		}
		expanded := make([]bool, newLen)
		copy(expanded, keywordSymbols)
		keywordSymbols = expanded
	}

	keywordCount := 0
	for i := range lang.KeywordLexStates {
		sym := lang.KeywordLexStates[i].AcceptToken
		if sym == 0 || sym == lang.KeywordCaptureToken {
			continue
		}
		ensureSymbolCap(sym)
		if !keywordSymbols[sym] {
			keywordSymbols[sym] = true
			keywordCount++
		}
	}
	if keywordCount == 0 {
		return nil
	}

	stateCount := int(lang.StateCount)
	if stateCount <= 0 {
		stateCount = len(lang.ParseTable)
	}
	if smallCount := int(lang.LargeStateCount) + len(lang.SmallParseTableMap); smallCount > stateCount {
		stateCount = smallCount
	}
	if stateCount <= 0 {
		return nil
	}

	hasKeyword := make([]bool, stateCount)
	anyState := false
	for state := 0; state < len(lang.ParseTable) && state < stateCount; state++ {
		row := lang.ParseTable[state]
		for sym, idx := range row {
			if idx == 0 {
				continue
			}
			if sym < len(keywordSymbols) && keywordSymbols[sym] {
				hasKeyword[state] = true
				anyState = true
				break
			}
		}
	}

	if len(lang.SmallParseTableMap) > 0 && len(lang.SmallParseTable) > 0 {
		base := int(lang.LargeStateCount)
		table := lang.SmallParseTable
		for smallIdx, offset := range lang.SmallParseTableMap {
			state := base + smallIdx
			if state < 0 || state >= stateCount || hasKeyword[state] {
				continue
			}
			pos := int(offset)
			if pos >= len(table) {
				continue
			}
			groupCount := int(table[pos])
			pos++
			for i := 0; i < groupCount; i++ {
				if pos+1 >= len(table) {
					break
				}
				sectionValue := table[pos]
				groupSymbolCount := int(table[pos+1])
				pos += 2
				end := pos + groupSymbolCount
				if end > len(table) {
					end = len(table)
				}
				if sectionValue != 0 {
					for _, sym := range table[pos:end] {
						if int(sym) < len(keywordSymbols) && keywordSymbols[sym] {
							hasKeyword[state] = true
							anyState = true
							break
						}
					}
				}
				pos = end
				if hasKeyword[state] {
					break
				}
			}
		}
	}

	if !anyState {
		return nil
	}
	return hasKeyword
}

// buildRecoverActionsByState precomputes recover actions keyed by parser state
// and symbol to avoid repeated action-table scans in recover lookups.
func buildRecoverActionsByState(lang *Language) ([][]recoverSymbolAction, []bool, []bool) {
	if lang == nil {
		return nil, nil, nil
	}
	if len(lang.ParseActions) == 0 {
		return nil, nil, nil
	}

	recoverActions := make([]ParseAction, len(lang.ParseActions))
	hasRecoverAction := make([]bool, len(lang.ParseActions))
	anyRecoverAction := false
	for i := range lang.ParseActions {
		act, ok := recoverAction(&lang.ParseActions[i])
		if !ok {
			continue
		}
		recoverActions[i] = act
		hasRecoverAction[i] = true
		anyRecoverAction = true
	}
	if !anyRecoverAction {
		return nil, nil, nil
	}

	stateCount := int(lang.StateCount)
	if stateCount <= 0 {
		stateCount = len(lang.ParseTable)
	}
	if smallCount := int(lang.LargeStateCount) + len(lang.SmallParseTableMap); smallCount > stateCount {
		stateCount = smallCount
	}
	if stateCount <= 0 {
		return nil, nil, nil
	}
	symbolCount := int(lang.SymbolCount)
	if symbolCount <= 0 {
		symbolCount = len(lang.SymbolNames)
	}
	if symbolCount <= 0 {
		symbolCount = 64
	}

	recoverByState := make([][]recoverSymbolAction, stateCount)
	hasRecoverSymbol := make([]bool, symbolCount)
	ensureSymbolCap := func(sym uint16) {
		idx := int(sym)
		if idx < len(hasRecoverSymbol) {
			return
		}
		newLen := len(hasRecoverSymbol)
		if newLen == 0 {
			newLen = 64
		}
		for idx >= newLen {
			newLen *= 2
		}
		expanded := make([]bool, newLen)
		copy(expanded, hasRecoverSymbol)
		hasRecoverSymbol = expanded
	}
	for state := 0; state < len(lang.ParseTable) && state < stateCount; state++ {
		row := lang.ParseTable[state]
		for sym, idx := range row {
			if int(idx) < len(hasRecoverAction) && hasRecoverAction[idx] {
				ensureSymbolCap(uint16(sym))
				hasRecoverSymbol[sym] = true
				recoverByState[state] = append(recoverByState[state], recoverSymbolAction{
					sym:    uint16(sym),
					action: recoverActions[idx],
				})
			}
		}
	}

	if len(lang.SmallParseTableMap) > 0 && len(lang.SmallParseTable) > 0 {
		base := int(lang.LargeStateCount)
		table := lang.SmallParseTable
		for smallIdx, offset := range lang.SmallParseTableMap {
			state := base + smallIdx
			if state < 0 || state >= stateCount {
				continue
			}
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
				symbolCount := int(table[pos+1])
				pos += 2
				hasRecover := int(sectionValue) < len(hasRecoverAction) && hasRecoverAction[sectionValue]
				for j := 0; j < symbolCount; j++ {
					if pos >= len(table) {
						break
					}
					if hasRecover {
						ensureSymbolCap(table[pos])
						hasRecoverSymbol[table[pos]] = true
						recoverByState[state] = append(recoverByState[state], recoverSymbolAction{
							sym:    table[pos],
							action: recoverActions[sectionValue],
						})
					}
					pos++
				}
			}
		}
	}

	hasRecoverState := make([]bool, stateCount)
	anyState := false
	for i := range recoverByState {
		if len(recoverByState[i]) > 0 {
			hasRecoverState[i] = true
			anyState = true
		}
	}
	if !anyState {
		return nil, nil, nil
	}
	anySymbol := false
	for i := range hasRecoverSymbol {
		if hasRecoverSymbol[i] {
			anySymbol = true
			break
		}
	}
	if !anySymbol {
		hasRecoverSymbol = nil
	}
	return recoverByState, hasRecoverState, hasRecoverSymbol
}

func (p *Parser) stateCanRecover(state StateID) bool {
	if len(p.hasRecoverState) == 0 {
		return true
	}
	idx := int(state)
	return idx >= 0 && idx < len(p.hasRecoverState) && p.hasRecoverState[idx]
}

func (p *Parser) symbolCanRecover(sym Symbol) bool {
	if len(p.hasRecoverSymbol) == 0 {
		return true
	}
	idx := int(sym)
	return idx >= 0 && idx < len(p.hasRecoverSymbol) && p.hasRecoverSymbol[idx]
}

func (p *Parser) recoverActionForState(state StateID, sym Symbol) (ParseAction, bool) {
	if len(p.recoverByState) == 0 {
		return ParseAction{}, false
	}
	idx := int(state)
	if idx < 0 || idx >= len(p.recoverByState) {
		return ParseAction{}, false
	}
	entries := p.recoverByState[idx]
	if len(entries) == 0 {
		return ParseAction{}, false
	}
	target := uint16(sym)
	for i := range entries {
		if entries[i].sym == target {
			return entries[i].action, true
		}
	}
	return ParseAction{}, false
}
