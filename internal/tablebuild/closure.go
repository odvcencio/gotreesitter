package tablebuild

import "github.com/odvcencio/gotreesitter/internal/lexbuild"

// This file ports build_tables/item_set_builder.rs: the FIRST and LAST sets,
// the precomputed closure additions of each non-terminal, and the closure of
// an item set. Display code is not ported.

// transitiveClosureAddition is item_set_builder.rs: struct
// TransitiveClosureAddition.
type transitiveClosureAddition struct {
	item ParseItem
	info additionInfo
}

// eq is the derived PartialEq of TransitiveClosureAddition (the item by
// ParseItem's equality, the info field by field).
func (a *transitiveClosureAddition) eq(o *transitiveClosureAddition) bool {
	return a.item.eq(&o.item) && a.info == o.info
}

// additionInfo is item_set_builder.rs: struct AdditionInfo.
type additionInfo struct {
	lookaheads           lookaheadSetID
	reservedLookaheads   uint32
	propagatesLookaheads bool
	containsWord         bool
}

// followSetInfo is item_set_builder.rs: struct FollowSetInfo.
type followSetInfo struct {
	lookaheads           lexbuild.TokenSet
	reservedLookaheads   uint32
	propagatesLookaheads bool
}

// closureOrder chooses the order in which ParseItemSetBuilder.new turns the
// follow-set map of each non-terminal into closure additions. The generator
// iterates a hash map there; the order cannot reach a result (see
// newParseItemSetBuilder), and a test builds with both orders.
type closureOrder uint8

const (
	ascendingVariables closureOrder = iota
	descendingVariables
)

// ParseItemSetBuilder is item_set_builder.rs: struct ParseItemSetBuilder.
// The FIRST and LAST sets and the reserved word set ids of first steps come
// from internal/lexbuild, which ports the same part of fn
// ParseItemSetBuilder::new.
type ParseItemSetBuilder struct {
	sg      *lexbuild.SyntaxGrammar
	sets    *lexbuild.FirstLastSets
	inlines *InlinedProductionMap
	keyMap  *ItemKeyMap
	// lookaheads interns every lookahead set of the build.
	lookaheads *lookaheadSetPool
	// firstSetIDs is first_set_ids: each FIRST set interned, by symbol.
	firstSetIDs                map[Symbol]lookaheadSetID
	transitiveClosureAdditions [][]transitiveClosureAddition
	work                       int64 // entries inserted by closures (bClosureWork)
}

// findOrPush is item_set_builder.rs: fn find_or_push.
func findOrPush(vector []transitiveClosureAddition, value transitiveClosureAddition) []transitiveClosureAddition {
	for i := range vector {
		if vector[i].eq(&value) {
			return vector
		}
	}
	return append(vector, value)
}

type followFrame struct {
	symIx                int
	lookaheads           *lexbuild.TokenSet
	reservedWordSetID    uint32
	propagatesLookaheads bool
}

// newParseItemSetBuilder is item_set_builder.rs: fn ParseItemSetBuilder::new.
//
// Two loops of the generator iterate hash maps:
//
//  1. It interns the FIRST sets by iterating first_sets. Interning order only
//     numbers the lookahead sets, and lookahead set ids are compared only
//     for equality (see lookaheadSetPool). The port interns in symbol order.
//
//  2. It turns follow_set_info_by_non_terminal into the closure additions of
//     non-terminal i by iterating the map: the order of the additions is hash
//     order across variables, and production order within one variable. The
//     closure inserts the additions into a sorted item set, unions their
//     lookaheads and takes the largest reserved set id; union and maximum do
//     not depend on order, and the sorted set does not either, except for
//     which item an entry keeps when two equal items with different
//     productions are inserted (the first one). Two items can be equal only
//     if they have the same variable; the additions of one variable come in
//     production order in every list, whatever the map order. So the closure
//     is the same for every order of the map. The port iterates by variable
//     index (ascending, or descending for the test that checks this).
func newParseItemSetBuilder(sg *lexbuild.SyntaxGrammar, lg *lexbuild.LexicalGrammar, inlines *InlinedProductionMap, keyMap *ItemKeyMap, order closureOrder) (*ParseItemSetBuilder, error) {
	sets, err := lexbuild.NewFirstLastSets(sg, lg)
	if err != nil {
		return nil, err
	}
	result := &ParseItemSetBuilder{
		sg:                         sg,
		sets:                       sets,
		inlines:                    inlines,
		keyMap:                     keyMap,
		lookaheads:                 newLookaheadSetPool(),
		firstSetIDs:                map[Symbol]lookaheadSetID{},
		transitiveClosureAdditions: make([][]transitiveClosureAddition, len(sg.Variables)),
	}

	// Intern each FIRST set so closure propagation can union by id.
	for i := range lg.Variables {
		s := lexbuild.TerminalSym(i)
		result.firstSetIDs[s] = result.lookaheads.internRef(sets.First(s))
	}
	for i := range sg.ExternalTokens {
		s := lexbuild.ExternalSym(i)
		result.firstSetIDs[s] = result.lookaheads.internRef(sets.First(s))
	}
	for i := range sg.Variables {
		s := lexbuild.NonTerminalSym(i)
		result.firstSetIDs[s] = result.lookaheads.internRef(sets.First(s))
	}

	// For each non-terminal i, find the non-terminals that can appear at the
	// beginning of i and the tokens that can follow them, then store all of
	// those non-terminals' productions, with their lookahead info, as the
	// additions of i.
	var emptyLookaheadSet lexbuild.TokenSet
	var eofLookaheadSet lexbuild.TokenSet
	eofLookaheadSet.Insert(lexbuild.EndSym())
	var stack []followFrame
	follow := make([]*followSetInfo, len(sg.Variables))
	var touched []int
	for i := range sg.Variables {
		stack = append(stack[:0], followFrame{i, &emptyLookaheadSet, 0, true})
		for _, t := range touched {
			follow[t] = nil
		}
		touched = touched[:0]
		for len(stack) > 0 {
			fr := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			didAdd := false
			info := follow[fr.symIx]
			if info == nil {
				info = &followSetInfo{}
				follow[fr.symIx] = info
				touched = append(touched, fr.symIx)
			}
			if info.lookaheads.InsertAll(fr.lookaheads) {
				didAdd = true
			}
			if fr.reservedWordSetID > info.reservedLookaheads {
				info.reservedLookaheads = fr.reservedWordSetID
				didAdd = true
			}
			if fr.propagatesLookaheads && !info.propagatesLookaheads {
				didAdd = true
			}
			info.propagatesLookaheads = info.propagatesLookaheads || fr.propagatesLookaheads
			if !didAdd {
				continue
			}

			vp := sg.VarProds[fr.symIx]
			for prodID := vp[0]; prodID < vp[1]; prodID++ {
				p := production(sg, uint32(prodID))
				symbol, ok := p.firstSymbol()
				if !ok || !symbol.IsNonTerminal() {
					continue
				}
				switch {
				case len(p.steps) > 1:
					next := p.steps[1].Symbol
					stack = append(stack, followFrame{int(symbol.Index), sets.First(next), uint32(sets.ReservedFirst(next)), false})
				case p.requiresEOF:
					stack = append(stack, followFrame{int(symbol.Index), &eofLookaheadSet, 0, false})
				default:
					stack = append(stack, followFrame{int(symbol.Index), fr.lookaheads, fr.reservedWordSetID, fr.propagatesLookaheads})
				}
			}
		}

		present := make([]int, 0, len(touched))
		if order == descendingVariables {
			for v := len(sg.Variables) - 1; v >= 0; v-- {
				if follow[v] != nil {
					present = append(present, v)
				}
			}
		} else {
			for v := range sg.Variables {
				if follow[v] != nil {
					present = append(present, v)
				}
			}
		}
		for _, variableIndex := range present {
			fsi := follow[variableIndex]
			nonTerminal := lexbuild.NonTerminalSym(variableIndex)
			if containsSymbol(sg.VariablesToInline, nonTerminal) {
				continue
			}
			info := additionInfo{
				lookaheads:           result.lookaheads.internRef(&fsi.lookaheads),
				reservedLookaheads:   fsi.reservedLookaheads,
				propagatesLookaheads: fsi.propagatesLookaheads,
				containsWord:         sg.HasWord && fsi.lookaheads.Contains(sg.WordToken),
			}
			eofInfo := func() additionInfo {
				return additionInfo{lookaheads: result.lookaheads.internRef(&eofLookaheadSet)}
			}
			additions := result.transitiveClosureAdditions[i]
			vp := sg.VarProds[variableIndex]
			for prodID := uint32(vp[0]); prodID < uint32(vp[1]); prodID++ {
				item := ParseItem{VariableIndex: uint32(variableIndex), ProdID: prodID, keys: keyMap.keysFor(prodID)}
				if ids, ok := inlines.InlinedProdIDs(item.ProdID, item.StepIndex); ok {
					for _, id := range ids {
						itemInfo := info
						if sg.Productions[id].RequiresEOF {
							itemInfo = eofInfo()
						}
						additions = findOrPush(additions, transitiveClosureAddition{item.substituteProduction(id, keyMap.keysFor(id)), itemInfo})
					}
				} else {
					itemInfo := info
					if sg.Productions[prodID].RequiresEOF {
						itemInfo = eofInfo()
					}
					additions = findOrPush(additions, transitiveClosureAddition{item, itemInfo})
				}
			}
			result.transitiveClosureAdditions[i] = additions
		}
	}
	return result, nil
}

// transitiveClosure is item_set_builder.rs: fn
// ParseItemSetBuilder::transitive_closure.
func (b *ParseItemSetBuilder) transitiveClosure(itemSet *ParseItemSet) ParseItemSet {
	var result ParseItemSet
	for i := range itemSet.Entries {
		entry := &itemSet.Entries[i]
		if ids, ok := b.inlines.InlinedProdIDs(entry.Item.ProdID, entry.Item.StepIndex); ok {
			for _, id := range ids {
				b.addItem(&result, &ParseItemSetEntry{
					Item:                     entry.Item.substituteProduction(id, b.keyMap.keysFor(id)),
					lookaheads:               entry.lookaheads,
					FollowingReservedWordSet: entry.FollowingReservedWordSet,
				})
			}
		} else {
			b.addItem(&result, entry)
		}
	}
	return result
}

// firstSet is item_set_builder.rs: fn ParseItemSetBuilder::first_set.
func (b *ParseItemSetBuilder) firstSet(s Symbol) *lexbuild.TokenSet { return b.sets.First(s) }

// lastSet is item_set_builder.rs: fn ParseItemSetBuilder::last_set.
func (b *ParseItemSetBuilder) lastSet(s Symbol) *lexbuild.TokenSet { return b.sets.Last(s) }

// addItem is item_set_builder.rs: fn ParseItemSetBuilder::add_item.
func (b *ParseItemSetBuilder) addItem(set *ParseItemSet, entry *ParseItemSetEntry) {
	sg := b.sg
	if step, ok := entry.Item.step(sg); ok && step.Symbol.IsNonTerminal() {
		next := entry.Item.successor()
		var followingTokens lookaheadSetID
		var followingReserved uint32
		// Determine which tokens can follow this non-terminal.
		if nextStep, ok := next.step(sg); ok {
			followingTokens = b.firstSetIDs[nextStep.Symbol]
			followingReserved = uint32(b.sets.ReservedFirst(nextStep.Symbol))
		} else {
			followingTokens = entry.lookaheads
			followingReserved = entry.FollowingReservedWordSet
		}

		// Use the pre-computed additions to expand the non-terminal.
		additions := b.transitiveClosureAdditions[step.Symbol.Index]
		b.work += int64(len(additions))
		bClosureWork.checkWork(b.work)
		for k := range additions {
			addition := &additions[k]
			e := set.insert(addition.item)
			e.lookaheads = b.lookaheads.union(e.lookaheads, addition.info.lookaheads)
			if addition.info.containsWord {
				e.FollowingReservedWordSet = max(e.FollowingReservedWordSet, addition.info.reservedLookaheads)
			}
			if addition.info.propagatesLookaheads {
				e.lookaheads = b.lookaheads.union(e.lookaheads, followingTokens)
				if sg.HasWord && b.lookaheads.get(followingTokens).Contains(sg.WordToken) {
					e.FollowingReservedWordSet = max(e.FollowingReservedWordSet, followingReserved)
				}
			}
		}
	}

	e := set.insert(entry.Item)
	e.lookaheads = b.lookaheads.union(e.lookaheads, entry.lookaheads)
	e.FollowingReservedWordSet = max(e.FollowingReservedWordSet, entry.FollowingReservedWordSet)
}
