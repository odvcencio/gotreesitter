package gotreesitter

import "sort"

const smallTokenDenseThreshold = 8
const cobolSmallTokenDenseThreshold = 12
const typeScriptSmallTokenDenseThreshold = 4

func buildSmallLookup(lang *Language, smallTokenLookup [][]uint16) [][]smallActionPair {
	out := make([][]smallActionPair, len(lang.SmallParseTableMap))
	table := lang.SmallParseTable
	for smallIdx, offset := range lang.SmallParseTableMap {
		pos := int(offset)
		if pos >= len(table) {
			continue
		}
		groupCount := table[pos]
		pos++
		total := 0
		countPos := pos
		denseLookupLimit := 0
		if smallIdx < len(smallTokenLookup) {
			denseLookupLimit = len(smallTokenLookup[smallIdx])
		}
		for i := uint16(0); i < groupCount; i++ {
			if countPos+1 >= len(table) {
				total = 0
				break
			}
			symbolCount := int(table[countPos+1])
			countPos += 2
			if denseLookupLimit == 0 {
				total += symbolCount
				countPos += symbolCount
				continue
			}
			for j := 0; j < symbolCount; j++ {
				if countPos >= len(table) {
					break
				}
				sym := int(table[countPos])
				if sym >= denseLookupLimit {
					total++
				}
				countPos++
			}
		}
		if total == 0 {
			continue
		}

		pairs := make([]smallActionPair, 0, total)
		for i := uint16(0); i < groupCount; i++ {
			if pos+1 >= len(table) {
				break
			}
			val := table[pos]
			symbolCount := table[pos+1]
			pos += 2
			for j := uint16(0); j < symbolCount; j++ {
				if pos >= len(table) {
					break
				}
				sym := table[pos]
				if denseLookupLimit == 0 || int(sym) >= denseLookupLimit {
					pairs = append(pairs, smallActionPair{sym: sym, val: val})
				}
				pos++
			}
		}
		sort.Slice(pairs, func(i, j int) bool { return pairs[i].sym < pairs[j].sym })
		out[smallIdx] = pairs
	}
	return out
}

func buildSmallTokenLookup(lang *Language) [][]uint16 {
	if lang == nil || lang.TokenCount == 0 || len(lang.SmallParseTableMap) == 0 || len(lang.SmallParseTable) == 0 {
		return nil
	}
	if !compactSmallTokenRows(lang) {
		threshold := smallTokenDenseThreshold
		// grammargen-compiled blobs (today only go) use dense small-token rows;
		// keyed on the general GeneratedByGrammargen flag, not the language name.
		if lang.GeneratedByGrammargen {
			threshold = 0
		} else if lang.Name == "typescript" {
			threshold = typeScriptSmallTokenDenseThreshold
		}
		return buildSmallTokenLookupFullRows(lang, threshold)
	}
	out := make([][]uint16, len(lang.SmallParseTableMap))
	table := lang.SmallParseTable
	tokenCount := int(lang.TokenCount)
	threshold := cobolSmallTokenDenseThreshold
	seen := make([]int, tokenCount)
	for smallIdx, offset := range lang.SmallParseTableMap {
		pos := int(offset)
		if pos >= len(table) {
			continue
		}
		groupCount := table[pos]
		pos++
		used := 0
		maxSym := -1
		seenStamp := smallIdx + 1
		countPos := pos
		for i := uint16(0); i < groupCount; i++ {
			if countPos+1 >= len(table) {
				break
			}
			symbolCount := table[countPos+1]
			countPos += 2
			for j := uint16(0); j < symbolCount; j++ {
				if countPos >= len(table) {
					break
				}
				sym := int(table[countPos])
				if sym >= 0 && sym < tokenCount {
					if seen[sym] != seenStamp {
						seen[sym] = seenStamp
						used++
						if sym > maxSym {
							maxSym = sym
						}
					}
				}
				countPos++
			}
		}
		if used > threshold {
			row := make([]uint16, maxSym+1)
			for i := uint16(0); i < groupCount; i++ {
				if pos+1 >= len(table) {
					break
				}
				val := table[pos]
				symbolCount := table[pos+1]
				pos += 2
				for j := uint16(0); j < symbolCount; j++ {
					if pos >= len(table) {
						break
					}
					sym := int(table[pos])
					if sym >= 0 && sym < len(row) {
						row[sym] = val
					}
					pos++
				}
			}
			out[smallIdx] = row
		}
	}
	return out
}

func compactSmallTokenRows(lang *Language) bool {
	return isCobolLanguage(lang)
}

func smallDenseLookupSymbolLimit(lang *Language) int {
	if lang == nil {
		return 0
	}
	limit := int(lang.TokenCount)
	if lang.GeneratedByGrammargen && lang.SymbolCount > lang.TokenCount {
		limit = int(lang.SymbolCount)
	}
	return limit
}

func buildSmallTokenLookupFullRows(lang *Language, threshold int) [][]uint16 {
	out := make([][]uint16, len(lang.SmallParseTableMap))
	table := lang.SmallParseTable
	symbolLimit := smallDenseLookupSymbolLimit(lang)
	for smallIdx, offset := range lang.SmallParseTableMap {
		pos := int(offset)
		if pos >= len(table) {
			continue
		}
		groupCount := table[pos]
		pos++
		row := make([]uint16, symbolLimit)
		used := 0
		for i := uint16(0); i < groupCount; i++ {
			if pos+1 >= len(table) {
				break
			}
			val := table[pos]
			symbolCount := table[pos+1]
			pos += 2
			for j := uint16(0); j < symbolCount; j++ {
				if pos >= len(table) {
					break
				}
				sym := int(table[pos])
				if sym >= 0 && sym < symbolLimit {
					if row[sym] == 0 {
						used++
					}
					row[sym] = val
				}
				pos++
			}
		}
		if used > threshold {
			out[smallIdx] = row
		}
	}
	return out
}

// lookupAction looks up the parse action for the given state and symbol.
func (p *Parser) lookupAction(state StateID, sym Symbol) *ParseActionEntry {
	idx := p.lookupActionIndex(state, sym)
	if idx == 0 {
		return nil
	}
	if int(idx) < len(p.language.ParseActions) {
		return &p.language.ParseActions[idx]
	}
	return nil
}

// lookupActionIndexFunc returns a cached bound-method closure for
// lookupActionIndex. Token-source construction happens per parse; using this
// instead of `p.lookupActionIndex` avoids allocating a fresh method-value
// closure each time. The closure captures only p, and lookupActionIndex reads
// p.denseLimit/p.language at call time, so caching is safe across language
// changes.
func (p *Parser) lookupActionIndexFunc() func(state StateID, sym Symbol) uint16 {
	if p.lookupActionIndexFn == nil {
		p.lookupActionIndexFn = p.lookupActionIndex
	}
	return p.lookupActionIndexFn
}

// lookupActionIndex returns the parse action index for (state, symbol).
// Returns 0 (the error/no-action entry) if not found.
func (p *Parser) lookupActionIndex(state StateID, sym Symbol) uint16 {
	workCountRecordTableLookup()
	if int(state) < p.denseLimit {
		if int(state) >= len(p.language.ParseTable) {
			return 0
		}
		row := p.language.ParseTable[state]
		if int(sym) >= len(row) {
			return 0
		}
		return row[sym]
	}
	return p.lookupActionIndexSmall(state, sym)
}

func (p *Parser) buildExternalValidByState() [][]uint16 {
	if p == nil || p.language == nil || len(p.language.ExternalSymbols) == 0 || len(p.language.ExternalLexStates) > 0 {
		return nil
	}
	if len(p.language.ExternalSymbols) > int(^uint16(0)) {
		return nil
	}
	stateCount := int(p.language.StateCount)
	if stateCount == 0 {
		stateCount = len(p.language.ParseTable)
		if smallStates := p.smallBase + len(p.language.SmallParseTableMap); smallStates > stateCount {
			stateCount = smallStates
		}
		if len(p.language.LexModes) > stateCount {
			stateCount = len(p.language.LexModes)
		}
	}
	if stateCount <= 0 {
		return nil
	}
	rows := make([][]uint16, stateCount)
	for state := 0; state < stateCount; state++ {
		var row []uint16
		for i, sym := range p.language.ExternalSymbols {
			if p.lookupActionIndex(StateID(state), sym) == 0 {
				continue
			}
			row = append(row, uint16(i))
		}
		rows[state] = row
	}
	return rows
}

func buildExternalValidMaskByState(rows [][]uint16, externalSymbolCount int) []uint64 {
	if len(rows) == 0 || externalSymbolCount <= 0 || externalSymbolCount > 64 {
		return nil
	}
	masks := make([]uint64, len(rows))
	any := false
	for state, row := range rows {
		var mask uint64
		for _, extIdx := range row {
			i := int(extIdx)
			if i < 0 || i >= externalSymbolCount {
				continue
			}
			mask |= uint64(1) << uint(i)
		}
		masks[state] = mask
		if mask != 0 {
			any = true
		}
	}
	if !any {
		return nil
	}
	return masks
}

// parserActionTableView is the EXACT input set the action-table walk and the
// eager-default-reduce builder read from a Parser. It exists so those two can
// be driven without a Parser at all.
//
// Before this type they took a *Parser, and the per-language memo below had to
// hand-replicate five Parser fields onto a scratch Parser to call them. That
// replication was invisible to the compiler: the day either function started
// reading a sixth field, the memo would have silently produced a different
// table for every Parser of that Language. Naming the inputs turns that class
// of bug into a compile error.
type parserActionTableView struct {
	language          *Language
	denseLimit        int
	smallBase         int
	smallTokenLookup  [][]uint16
	smallLookup       [][]smallActionPair
	classifiedActions []classifiedParseAction
}

// actionTableView projects the fields a built Parser already holds.
func (p *Parser) actionTableView() parserActionTableView {
	if p == nil {
		return parserActionTableView{}
	}
	return parserActionTableView{
		language:          p.language,
		denseLimit:        p.denseLimit,
		smallBase:         p.smallBase,
		smallTokenLookup:  p.smallTokenLookup,
		smallLookup:       p.smallLookup,
		classifiedActions: p.classifiedActions,
	}
}

func (p *Parser) forEachActionIndexInState(state StateID, visit func(sym Symbol, idx uint16) bool) {
	if p == nil {
		return
	}
	p.actionTableView().forEachActionIndexInState(state, visit)
}

func (v parserActionTableView) forEachActionIndexInState(state StateID, visit func(sym Symbol, idx uint16) bool) {
	if v.language == nil || visit == nil {
		return
	}
	if int(state) < v.denseLimit {
		if int(state) >= len(v.language.ParseTable) {
			return
		}
		row := v.language.ParseTable[state]
		for sym, idx := range row {
			if idx == 0 {
				continue
			}
			if !visit(Symbol(sym), idx) {
				return
			}
		}
		return
	}

	smallIdx := int(state) - v.smallBase
	if smallIdx < 0 || smallIdx >= len(v.language.SmallParseTableMap) {
		return
	}
	hasDenseRow := smallIdx < len(v.smallTokenLookup) && len(v.smallTokenLookup[smallIdx]) > 0
	hasOverflow := smallIdx < len(v.smallLookup) && len(v.smallLookup[smallIdx]) > 0
	if hasDenseRow || hasOverflow {
		// A small state can carry a dense smallTokenLookup row and a
		// smallLookup overflow slice at the same time (dense covers the
		// low symbol ids, overflow covers the rest). buildSmallLookup only
		// admits symbols at or beyond the dense row's length into
		// overflow, so the two sets are always disjoint and both must be
		// visited to enumerate the state completely.
		if hasDenseRow {
			for sym, idx := range v.smallTokenLookup[smallIdx] {
				if idx == 0 {
					continue
				}
				if !visit(Symbol(sym), idx) {
					return
				}
			}
		}
		if hasOverflow {
			for _, pair := range v.smallLookup[smallIdx] {
				if !visit(Symbol(pair.sym), pair.val) {
					return
				}
			}
		}
		return
	}

	offset := v.language.SmallParseTableMap[smallIdx]
	table := v.language.SmallParseTable
	if int(offset) >= len(table) {
		return
	}
	groupCount := table[offset]
	pos := int(offset) + 1
	for i := uint16(0); i < groupCount; i++ {
		if pos+1 >= len(table) {
			return
		}
		sectionValue := table[pos]
		symbolCount := table[pos+1]
		pos += 2
		for j := uint16(0); j < symbolCount; j++ {
			if pos >= len(table) {
				return
			}
			if !visit(Symbol(table[pos]), sectionValue) {
				return
			}
			pos++
		}
	}
}

func (p *Parser) lookupActionIndexSmall(state StateID, sym Symbol) uint16 {
	// Small (compressed sparse) table lookup.
	smallIdx := int(state) - p.smallBase
	if smallIdx < 0 || smallIdx >= len(p.language.SmallParseTableMap) {
		return 0
	}
	if smallIdx < len(p.smallTokenLookup) {
		row := p.smallTokenLookup[smallIdx]
		if int(sym) < len(row) {
			return row[sym]
		}
	}
	if smallIdx < len(p.smallLookup) {
		pairs := p.smallLookup[smallIdx]
		if len(pairs) > 0 {
			target := uint16(sym)
			if len(pairs) <= 8 {
				for i := 0; i < len(pairs); i++ {
					if pairs[i].sym == target {
						return pairs[i].val
					}
					if pairs[i].sym > target {
						return 0
					}
				}
				return 0
			}
			lo, hi := 0, len(pairs)
			for lo < hi {
				mid := int(uint(lo+hi) >> 1)
				if pairs[mid].sym < target {
					lo = mid + 1
				} else {
					hi = mid
				}
			}
			if lo < len(pairs) && pairs[lo].sym == target {
				return pairs[lo].val
			}
			return 0
		}
	}
	offset := p.language.SmallParseTableMap[smallIdx]
	table := p.language.SmallParseTable
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
			if table[pos] == uint16(sym) {
				return sectionValue
			}
			pos++
		}
	}
	return 0
}

func largeStateGotoKey(state StateID, sym Symbol) uint64 {
	return uint64(state)<<32 | uint64(sym)
}

func (l *Language) lookupLargeStateGoto(state StateID, sym Symbol) StateID {
	if l == nil || len(l.LargeStateGotos) == 0 {
		return 0
	}
	return l.LargeStateGotos[largeStateGotoKey(state, sym)]
}

// lookupGoto returns the GOTO target state for a nonterminal symbol.
func (p *Parser) lookupGoto(state StateID, sym Symbol) StateID {
	if p == nil || p.language == nil {
		return 0
	}
	if p.language.TokenCount > 0 && uint32(sym) >= p.language.TokenCount {
		if target := p.language.lookupLargeStateGoto(state, sym); target != 0 {
			return target
		}
	}

	raw := p.lookupActionIndex(state, sym)
	if raw == 0 {
		return 0
	}

	// ts2go-generated grammars encode nonterminal GOTO values directly as
	// parser state IDs. Hand-built grammars encode parse-action indices.
	// ts2go always sets InitialState=1 (tree-sitter convention); hand-built
	// grammars default to InitialState=0.
	if p.language.TokenCount > 0 &&
		uint32(sym) >= p.language.TokenCount &&
		p.language.StateCount > 0 &&
		p.language.InitialState > 0 {
		return StateID(raw)
	}

	// Hand-built grammar or terminal symbol: look up in parse actions.
	if int(raw) < len(p.language.ParseActions) {
		entry := &p.language.ParseActions[raw]
		if len(entry.Actions) > 0 && entry.Actions[0].Type == ParseActionShift {
			return entry.Actions[0].State
		}
	}
	return 0
}

// languageDenseLimit returns the dense parse-table cutoff for this Language.
//
// Do not derive this cutoff again anywhere else. Several call sites need the
// same answer, and two copies can drift. A drifted cutoff changes the
// eager-default-reduce table for every Parser of the Language.
func languageDenseLimit(l *Language) int {
	if l.LargeStateCount > 0 {
		return int(l.LargeStateCount)
	}
	return len(l.ParseTable)
}

// parserDerivedTables holds the per-language derived parser tables that are
// pure functions of the decoded grammar tables. NewParser previously rebuilt
// them on every call; they are now built once per *Language and shared,
// read-only, by every Parser of that Language.
type parserDerivedTables struct {
	smallTokenLookup             [][]uint16
	smallLookup                  [][]smallActionPair
	externalValidByState         [][]uint16
	externalValidMaskByState     []uint64
	classifiedActions            []classifiedParseAction
	eagerDefaultReduces          []eagerDefaultReduceAction
	reduceChainHints             []reduceChainHint
	reduceChainHintByState       []int
	reduceAliasSeq               [][]Symbol
	aliasTargetSymbol            []bool
	keepSameNamedAnonChildSymbol []bool
	sharedAnonymousTokenSymbol   []bool
	reduceHasFields              []bool
	reduceFieldPlans             []reduceFieldPlan
	recoverByState               [][]recoverSymbolAction
	hasRecoverState              []bool
	hasRecoverSymbol             []bool
	hasKeywordState              []bool
}

// acquireParserDerivedTables builds the derived parser tables exactly once per
// Language, even under concurrent first use, and returns the shared instance.
//
// The read set, traced through every builder and the helpers they call:
//
//	AliasSequences, ExternalLexStates, ExternalSymbols, FieldMapEntries,
//	FieldMapSlices,
//	GeneratedByGrammargen, LargeStateCount, LexModes, Name, ParseActions,
//	KeywordCaptureToken, KeywordLexStates, ParseTable, ReduceChainHints,
//	SmallParseTable, SmallParseTableMap, StateCount, SymbolCount,
//	SymbolMetadata, SymbolNames, TokenCount
//
// Every write to one of those fields runs before the Language reaches a
// caller: inside LoadLanguage, inside decodeLanguageBlobData's repair passes,
// in grammargen assembly, or in ts2go generation. The embedded loader also
// performs its scanner, lex-state, and runtime-profile attaching inside the
// cache entry's own sync.Once, before it returns the Language.
//
// ExternalLexStates controls whether the optional external-valid fallback
// tables are built. NewParser checks its current value before installing those
// tables, so attaching scanner states after the first build remains correct.
// Removing states after a first build that skipped the fallback only uses the
// exact action-table lookup path.
//
// The supported POST-load mutations write none of the other fields. They write
// ExternalScanner and the runtime-profile flags.
//
// TWO STALENESS HAZARDS THAT THIS MEMO INTRODUCES. Before it, NewParser
// rebuilt these tables on every call, so a later call observed any change.
// Now the first call freezes them.
//
//  1. Attach before the first NewParser. AttachLanguageSupport assigns
//     Language.Name when it is empty, and buildSmallTokenLookup branches on
//     that name. Build a parser first and the tables keep the pre-attach name.
//     The embedded loader already attaches inside its own sync.Once.
//  2. Do not poke table cells in place. Writing into ParseTable,
//     SmallParseTable, or a ParseActionEntry's Actions leaves this memo
//     serving tables built from the old contents. Swap the whole slice, or
//     build a fresh Language. The recovery-gate memo documents the same
//     hazard for its own inputs (cRecoveryGateCacheKey).
func (l *Language) acquireParserDerivedTables() *parserDerivedTables {
	if l == nil {
		return &parserDerivedTables{}
	}
	l.parserDerivedOnce.Do(func() {
		l.parserDerived = buildParserDerivedTables(l)
	})
	if derived := l.parserDerived; derived != nil {
		return derived
	}
	// sync.Once records completion even when its function PANICS, so a builder
	// that panicked on first use would otherwise leave parserDerived nil and
	// make every later NewParser nil-dereference far from the original cause.
	// Rebuild unmemoized instead: the caller still gets correct tables, and the
	// only cost is that a Language whose first build panicked stops being
	// cached.
	return buildParserDerivedTables(l)
}

// buildParserDerivedTables computes the derived tables from a Language alone.
// It creates a temporary lookup Parser only while building external-valid
// rows. The eager-default-reduce builder consumes an explicit
// parserActionTableView, so its inputs have no field-drift hazard.
//
// INVARIANT: nothing reachable from here may call acquireParserDerivedTables.
// It runs inside that function's sync.Once, and Once.Do is not re-entrant, so
// a re-entering call blocks on the in-progress build ON THE SAME GOROUTINE and
// never returns. The failure is a silent hang with no panic and no stack, so
// keep this function to the builders and the view above.
func buildParserDerivedTables(l *Language) *parserDerivedTables {
	t := &parserDerivedTables{}
	if len(l.SmallParseTableMap) > 0 && len(l.SmallParseTable) > 0 {
		t.smallTokenLookup = buildSmallTokenLookup(l)
		t.smallLookup = buildSmallLookup(l, t.smallTokenLookup)
	}
	if len(l.ExternalLexStates) == 0 {
		lookupParser := &Parser{
			language:         l,
			denseLimit:       languageDenseLimit(l),
			smallBase:        int(l.LargeStateCount),
			smallTokenLookup: t.smallTokenLookup,
			smallLookup:      t.smallLookup,
		}
		t.externalValidByState = lookupParser.buildExternalValidByState()
		t.externalValidMaskByState = buildExternalValidMaskByState(t.externalValidByState, len(l.ExternalSymbols))
	}
	t.classifiedActions = buildClassifiedParseActions(l)
	t.keepSameNamedAnonChildSymbol = buildKeepSameNamedAnonChildSymbols(l)
	t.sharedAnonymousTokenSymbol = buildSharedAnonymousTokenSymbols(l)
	t.eagerDefaultReduces = buildEagerDefaultReduceActions(parserActionTableView{
		language:          l,
		denseLimit:        languageDenseLimit(l),
		smallBase:         int(l.LargeStateCount),
		smallTokenLookup:  t.smallTokenLookup,
		smallLookup:       t.smallLookup,
		classifiedActions: t.classifiedActions,
	})
	t.reduceChainHints = buildReduceChainHints(l)
	t.reduceChainHintByState = buildReduceChainHintIndex(t.reduceChainHints)
	t.reduceAliasSeq = buildReduceAliasSequences(l)
	t.aliasTargetSymbol = buildAliasTargetSymbols(l)
	t.reduceHasFields = buildReduceFieldPresence(l)
	t.reduceFieldPlans = buildReduceFieldPlans(l)
	t.recoverByState, t.hasRecoverState, t.hasRecoverSymbol = buildRecoverActionsByState(l)
	t.hasKeywordState = buildKeywordStates(l)
	return t
}
