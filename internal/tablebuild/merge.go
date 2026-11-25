package tablebuild

import (
	"cmp"
	"slices"

	"github.com/odvcencio/gotreesitter/internal/lexbuild"
)

// This file ports the merge of compatible parse states:
// build_tables/minimize_parse_table.rs: struct SymbolKey, struct
// ConflictBits, fn Minimizer::merge_compatible_states, fn states_conflict,
// fn state_successors_differ, fn entries_conflict and fn token_conflicts (in
// the order of the Rust file), then dedup.rs: fn split_state_id_groups.
// minimize_parse_table.rs: fn Minimizer::symbol_name only names symbols for
// the generator's debug log; it is not ported.
//
// Order. The merge iterates no hash map and no hash set. It reads the entries
// of each state in IndexMap order (insertion order, kept by the entry
// slices), sorts them by symbol (each symbol has at most one entry in a
// state, so any sort gives the same order), keeps the groups as vectors and
// numbers a new group by the length of the group vector. Its result therefore
// depends only on the order of the states and of their entries, which the
// port reproduces.

// symbolKey is minimize_parse_table.rs: struct SymbolKey: a Symbol packed
// into 64 bits, the kind in the high 3 bits and the index in the low 61, so
// that comparing two keys as integers is the derived Ord of Symbol (kind,
// then index). SymbolKey::is_terminal and SymbolKey::is_non_terminal are dead
// code in the generator and are not ported.
type symbolKey uint64

// keyTagShift and keyIndexMask are minimize_parse_table.rs: const
// KEY_TAG_SHIFT and const KEY_INDEX_MASK.
const (
	keyTagShift  = 61
	keyIndexMask = uint64(1)<<keyTagShift - 1
)

// newSymbolKey is minimize_parse_table.rs: fn SymbolKey::new.
func newSymbolKey(sym Symbol) symbolKey {
	return symbolKey(uint64(sym.Kind)<<keyTagShift | uint64(sym.Index))
}

// symbol is minimize_parse_table.rs: fn SymbolKey::symbol.
func (k symbolKey) symbol() Symbol {
	var kind lexbuild.SymbolType
	switch uint64(k) >> keyTagShift {
	case 0:
		kind = lexbuild.External
	case 1:
		kind = lexbuild.End
	case 2:
		kind = lexbuild.EndOfNonTerminalExtra
	case 3:
		kind = lexbuild.Terminal
	default:
		kind = lexbuild.NonTerminal
	}
	return Symbol{Kind: kind, Index: k.index()}
}

// index is minimize_parse_table.rs: fn SymbolKey::index.
func (k symbolKey) index() uint32 { return uint32(uint64(k) & keyIndexMask) }

// conflictBits is minimize_parse_table.rs: struct ConflictBits: word-aligned
// bit sets, all indexed by terminal index, that token_conflicts reads.
// stateTerminals and conflictRows are flattened two-dimensional vectors: row
// i spans [i*rowWords, (i+1)*rowWords).
type conflictBits struct {
	rowWords int
	// stateTerminals holds, per state, which terminals have entries.
	stateTerminals []uint64
	// conflictRows holds, per token, which terminals it lexically conflicts
	// with (token_conflicts.rs: fn does_conflict).
	conflictRows []uint64
	// keywords is the keyword set.
	keywords []uint64
	// internalExternal holds the tokens that are also external tokens.
	internalExternal []uint64
}

// getConflictRow is minimize_parse_table.rs: fn ConflictBits::get_conflict_row.
func (b *conflictBits) getConflictRow(token int) []uint64 {
	base := token * b.rowWords
	return b.conflictRows[base : base+b.rowWords]
}

// getStateRow is minimize_parse_table.rs: fn ConflictBits::get_state_row.
func (b *conflictBits) getStateRow(state int) []uint64 {
	base := state * b.rowWords
	return b.stateTerminals[base : base+b.rowWords]
}

// keyedEntry is one element of the entry_maps of merge_compatible_states:
// (symbol key, action list id).
type keyedEntry struct {
	key symbolKey
	id  ActionListID
}

// keyedShift is one element of the shift_maps of merge_compatible_states:
// (symbol key, target state of the entry's last action).
type keyedShift struct {
	key   symbolKey
	state uint32
}

// indexedGoto is one element of the nonterminal_maps of
// merge_compatible_states: (non-terminal index, goto action).
type indexedGoto struct {
	index  uint32
	action GotoAction
}

// mergeCompatibleStates is minimize_parse_table.rs: fn
// Minimizer::merge_compatible_states.
//
// The states start in one group per parse item set core (the core id), each
// group's members in state order. One call of split_state_id_groups with
// states_conflict splits off the states whose entries cannot share a state;
// then split_state_id_groups with state_successors_differ runs until it
// splits nothing. The groups that hold state 0 (the error state) and state 1
// (the start state) move to positions 0 and 1, and each group becomes one
// state: the first member's entries, extended (IndexMap::extend: a key that
// exists keeps its place and takes the later value) with the other members'
// entries in group order. Every Shift and Goto then names the group of its
// target.
//
// Like the generator, the port finds both group positions before it swaps,
// and does not renumber the states after the swaps. The swaps change
// nothing: the search gives state 0 core id 0 and state 1 core id 1, each is
// the first member of its core's group, and split_state_id_groups only
// splits off members after the first.
func (m *minimizer) mergeCompatibleStates() {
	table := m.parseTable
	sg := m.syntaxGrammar
	states := table.States
	if len(states) == 0 {
		panicLikeGenerator("merge_compatible_states on a table without states (the maximum core id of no state)")
	}
	maxCoreID := uint32(0)
	for i := range states {
		maxCoreID = max(maxCoreID, states[i].CoreID)
	}
	coreCount := 1 + int(maxCoreID)

	// Initially group the states by their parse item set core.
	groupIDsByStateID := make([]uint32, 0, len(states))
	// Room for the maximum possible number of groups (one per state), as the
	// generator reserves it; split_state_id_groups appends the new groups.
	stateIDsByGroupID := make([][]uint32, coreCount, max(len(states), coreCount))
	for i := range states {
		coreID := states[i].CoreID
		stateIDsByGroupID[coreID] = append(stateIDsByGroupID[coreID], uint32(i))
		groupIDsByStateID = append(groupIDsByStateID, coreID)
	}
	m.stats.CoreGroups = coreCount

	// The terminal entries of each state, sorted by symbol key, for the
	// merge-join of states_conflict.
	entryMaps := make([][]keyedEntry, len(states))
	for s := range states {
		entries := make([]keyedEntry, len(states[s].TerminalEntries))
		for k, e := range states[s].TerminalEntries {
			entries[k] = keyedEntry{newSymbolKey(e.Symbol), e.ID}
		}
		slices.SortFunc(entries, func(a, b keyedEntry) int { return cmp.Compare(a.key, b.key) })
		entryMaps[s] = entries
	}

	// Word-aligned bit sets, so that token_conflicts can test a candidate
	// token against all of a state's terminals: per state, which terminal
	// indices have entries; per token, which terminal indices it lexically
	// conflicts with; the keyword set.
	nTerminals := len(m.lexicalGrammar.Variables)
	rowWords := (nTerminals + 63) / 64
	set := func(bits []uint64, index int) { bits[index/64] |= 1 << (index % 64) }

	stateTerminals := make([]uint64, len(states)*rowWords)
	for s := range states {
		row := stateTerminals[s*rowWords : (s+1)*rowWords]
		for _, e := range states[s].TerminalEntries {
			if e.Symbol.IsTerminal() {
				set(row, int(e.Symbol.Index))
			}
		}
	}

	conflictRows := make([]uint64, nTerminals*rowWords)
	for i := 0; i < nTerminals; i++ {
		row := conflictRows[i*rowWords : (i+1)*rowWords]
		for j := 0; j < nTerminals; j++ {
			if m.tokenConflictMap.DoesConflict(i, j) {
				set(row, j)
			}
		}
	}

	keywords := make([]uint64, rowWords)
	for _, symbol := range m.keywords.Symbols() {
		if symbol.IsTerminal() {
			set(keywords, int(symbol.Index))
		}
	}

	internalExternal := make([]uint64, rowWords)
	for _, external := range sg.ExternalTokens {
		if external.HasCorresponding {
			set(internalExternal, int(external.CorrespondingInternal.Index))
		}
	}

	bits := &conflictBits{
		rowWords:         rowWords,
		stateTerminals:   stateTerminals,
		conflictRows:     conflictRows,
		keywords:         keywords,
		internalExternal: internalExternal,
	}

	splitStateIDGroups(states, &stateIDsByGroupID, groupIDsByStateID, 0,
		func(left, right *InternedState, groups []uint32) bool {
			return m.statesConflict(left, right, groups, entryMaps, bits)
		}, &m.mergeWork)
	m.stats.GroupsAfterConflicts = len(stateIDsByGroupID)

	// Per state, the shift target of each terminal entry whose last action
	// is a Shift, and the goto action of each non-terminal entry, sorted by
	// symbol. The states' actions do not change while the groups split.
	shiftMaps := make([][]keyedShift, len(states))
	for s := range states {
		var shifts []keyedShift
		for _, e := range states[s].TerminalEntries {
			actions := table.ActionLists.Get(e.ID)
			if len(actions) == 0 {
				continue
			}
			if last := actions[len(actions)-1]; last.Kind == ActionShift {
				shifts = append(shifts, keyedShift{newSymbolKey(e.Symbol), last.State})
			}
		}
		slices.SortFunc(shifts, func(a, b keyedShift) int { return cmp.Compare(a.key, b.key) })
		shiftMaps[s] = shifts
	}

	// Only the symbol index is stored: all non-terminal entries share one
	// kind, so the index alone orders and compares them.
	nonterminalMaps := make([][]indexedGoto, len(states))
	for s := range states {
		entries := make([]indexedGoto, len(states[s].NonterminalEntries))
		for k, e := range states[s].NonterminalEntries {
			entries[k] = indexedGoto{e.Symbol.Index, e.Action}
		}
		slices.SortFunc(entries, func(a, b indexedGoto) int { return cmp.Compare(a.index, b.index) })
		nonterminalMaps[s] = entries
	}

	for {
		m.stats.SuccessorPasses++
		if !splitStateIDGroups(states, &stateIDsByGroupID, groupIDsByStateID, 0,
			func(left, right *InternedState, groups []uint32) bool {
				return m.stateSuccessorsDiffer(left, right, groups, shiftMaps, nonterminalMaps)
			}, &m.mergeWork) {
			break
		}
	}

	errorGroupIndex := groupPosition(stateIDsByGroupID, 0)
	startGroupIndex := groupPosition(stateIDsByGroupID, 1)
	stateIDsByGroupID[errorGroupIndex], stateIDsByGroupID[0] = stateIDsByGroupID[0], stateIDsByGroupID[errorGroupIndex]
	stateIDsByGroupID[startGroupIndex], stateIDsByGroupID[1] = stateIDsByGroupID[1], stateIDsByGroupID[startGroupIndex]

	// Create a list of new parse states: one state for each group of old
	// states. Go shape: dense position maps stand in for the IndexMaps' hash
	// lookups while a state is extended.
	terminalPositions := newEntryPositions(nTerminals, len(sg.ExternalTokens))
	nonterminalPositions := make([]int32, len(sg.Variables))
	for i := range nonterminalPositions {
		nonterminalPositions[i] = -1
	}
	newStates := make([]InternedState, 0, len(stateIDsByGroupID))
	for _, stateIDs := range stateIDsByGroupID {
		if len(stateIDs) == 0 {
			panicLikeGenerator("merge_compatible_states: an empty group")
		}
		// Initialize the new state based on the first old state in the group.
		parseState := states[stateIDs[0]]
		states[stateIDs[0]] = InternedState{}

		if len(stateIDs) > 1 {
			for k, e := range parseState.TerminalEntries {
				*terminalPositions.slot(e.Symbol) = int32(k)
			}
			for k, e := range parseState.NonterminalEntries {
				nonterminalPositions[e.Symbol.Index] = int32(k)
			}
			// Extend the new state with all of the actions from the other old
			// states in the group.
			for _, stateID := range stateIDs[1:] {
				other := states[stateID]
				states[stateID] = InternedState{}

				parseState.HasEOFGatedReduce = parseState.HasEOFGatedReduce || other.HasEOFGatedReduce
				for _, e := range other.TerminalEntries {
					if slot := terminalPositions.slot(e.Symbol); *slot >= 0 {
						parseState.TerminalEntries[*slot].ID = e.ID
					} else {
						*slot = int32(len(parseState.TerminalEntries))
						parseState.TerminalEntries = append(parseState.TerminalEntries, e)
					}
				}
				for _, e := range other.NonterminalEntries {
					if slot := &nonterminalPositions[e.Symbol.Index]; *slot >= 0 {
						parseState.NonterminalEntries[*slot].Action = e.Action
					} else {
						*slot = int32(len(parseState.NonterminalEntries))
						parseState.NonterminalEntries = append(parseState.NonterminalEntries, e)
					}
				}
				parseState.ReservedWords.InsertAll(&other.ReservedWords)
				for _, e := range parseState.TerminalEntries {
					parseState.ReservedWords.Remove(e.Symbol)
				}
			}
			for _, e := range parseState.TerminalEntries {
				*terminalPositions.slot(e.Symbol) = -1
			}
			for _, e := range parseState.NonterminalEntries {
				nonterminalPositions[e.Symbol.Index] = -1
			}
		}

		// Update the new state's outgoing references using the new grouping.
		parseState.updateNonterminalReferences(func(stateID uint32, _ *InternedState) uint32 {
			return groupIDsByStateID[stateID]
		})
		newStates = append(newStates, parseState)
	}

	table.States = newStates
	table.remapTerminalReferences(func(stateID uint32) uint32 { return groupIDsByStateID[stateID] })
	m.stats.StatesAfterMerge = len(newStates)
}

// groupPosition is Iterator::position(|g| g.contains(&state)).unwrap() over
// the groups.
func groupPosition(stateIDsByGroupID [][]uint32, state uint32) int {
	for i, g := range stateIDsByGroupID {
		if slices.Contains(g, state) {
			return i
		}
	}
	panicLikeGenerator("merge_compatible_states: no group holds state %d", state)
	return 0
}

// statesConflict is minimize_parse_table.rs: fn Minimizer::states_conflict.
// It walks the two states' terminal entries in symbol order: a token that
// both states have must have compatible entries (entries_conflict); a token
// that only one state has must not conflict with the other state's tokens
// (token_conflicts). Like the generator, it finds a state's entries by the
// state's id, which equals its index before the states merge.
func (m *minimizer) statesConflict(state1, state2 *InternedState, groupIDsByStateID []uint32, entryMaps [][]keyedEntry, bits *conflictBits) bool {
	entries1 := entryMaps[state1.ID]
	entries2 := entryMaps[state2.ID]
	len1, len2 := len(entries1), len(entries2)
	i, j := 0, 0
	for i < len1 || j < len2 {
		var ord int
		switch {
		case i < len1 && j < len2:
			ord = cmp.Compare(entries1[i].key, entries2[j].key)
		case i < len1:
			ord = -1
		default:
			ord = 1
		}
		switch ord {
		case 0:
			e1, e2 := entries1[i], entries2[j]
			token := e1.key.symbol()
			if m.entriesConflict(state1.ID, state2.ID, token, e1.id, e2.id, groupIDsByStateID) {
				return true
			}
			i++
			j++
		case -1:
			token := entries1[i].key.symbol()
			if m.tokenConflicts(state1.ID, state2.ID, state2, bits, token) {
				return true
			}
			i++
		default:
			token := entries2[j].key.symbol()
			if m.tokenConflicts(state1.ID, state2.ID, state1, bits, token) {
				return true
			}
			j++
		}
	}
	return false
}

// stateSuccessorsDiffer is minimize_parse_table.rs: fn
// Minimizer::state_successors_differ. Two states differ when, for a token
// that both have, the Shifts that end their entries go to different groups,
// or, for a non-terminal that both have, the gotos go to different groups
// or only one of them is ShiftExtra. Tokens and non-terminals that only one
// state has do not count.
func (m *minimizer) stateSuccessorsDiffer(state1, state2 *InternedState, groupIDsByStateID []uint32, shiftMaps [][]keyedShift, nonterminalMaps [][]indexedGoto) bool {
	shifts1 := shiftMaps[state1.ID]
	shifts2 := shiftMaps[state2.ID]
	i, j := 0, 0
	for i < len(shifts1) && j < len(shifts2) {
		k1, s1 := shifts1[i].key, shifts1[i].state
		k2, s2 := shifts2[j].key, shifts2[j].state
		switch cmp.Compare(k1, k2) {
		case -1:
			i++
		case 1:
			j++
		default:
			if groupIDsByStateID[s1] != groupIDsByStateID[s2] {
				return true
			}
			i++
			j++
		}
	}

	nonterms1 := nonterminalMaps[state1.ID]
	nonterms2 := nonterminalMaps[state2.ID]
	i, j = 0, 0
	for i < len(nonterms1) && j < len(nonterms2) {
		idx1, s1 := nonterms1[i].index, nonterms1[i].action
		idx2, s2 := nonterms2[j].index, nonterms2[j].action
		switch cmp.Compare(idx1, idx2) {
		case -1:
			i++
		case 1:
			j++
		default:
			switch {
			case s1.ShiftExtra && s2.ShiftExtra:
			case !s1.ShiftExtra && !s2.ShiftExtra:
				if groupIDsByStateID[s1.State] != groupIDsByStateID[s2.State] {
					return true
				}
			default:
				return true
			}
			i++
			j++
		}
	}

	return false
}

// entriesConflict is minimize_parse_table.rs: fn Minimizer::entries_conflict.
// Two entries for one token are compatible when they name the same pool
// list, or when their lists have equal length and equal actions, where two
// Shifts are equal when their targets are in the same group and their
// repetition flags agree. The reusable flags are not compared.
func (m *minimizer) entriesConflict(stateID1, stateID2 uint32, token Symbol, id1, id2 ActionListID, groupIDsByStateID []uint32) bool {
	// To be compatible, entries need to have the same actions.
	if id1.Index() == id2.Index() {
		return false
	}
	actions1 := m.parseTable.ActionLists.Get(id1)
	actions2 := m.parseTable.ActionLists.Get(id2)
	if len(actions1) != len(actions2) {
		return true
	}

	for k := range actions1 {
		action1, action2 := actions1[k], actions2[k]
		// Two shift actions are equivalent if their destinations are in the
		// same group.
		if action1.Kind == ActionShift && action2.Kind == ActionShift {
			group1 := groupIDsByStateID[action1.State]
			group2 := groupIDsByStateID[action2.State]
			if group1 == group2 && action1.IsRepetition == action2.IsRepetition {
				continue
			}
			return true
		} else if action1 != action2 {
			return true
		}
	}

	return false
}

// tokenConflicts is minimize_parse_table.rs: fn Minimizer::token_conflicts:
// whether adding newToken to rightState could change what the lexer does
// there. The end of a non-terminal extra and external tokens always
// conflict; a token that is a reserved word of rightState never does; a
// token that is also an external token always does. Otherwise the token
// conflicts when one of rightState's terminals is in its conflict row,
// except that the word token and a keyword do not conflict with each other.
//
// As in the generator, every token that reaches the conflict row is read by
// its index alone: the end-of-input token (index 0) reads the row of
// terminal 0.
func (m *minimizer) tokenConflicts(leftID, rightID uint32, rightState *InternedState, bits *conflictBits, newToken Symbol) bool {
	if newToken == lexbuild.EndOfNonTerminalExtraSym() {
		return true
	}

	// Do not add external tokens; they could conflict lexically with any of
	// the state's existing lookahead tokens.
	if newToken.IsExternal() {
		return true
	}

	if rightState.ReservedWords.Contains(newToken) {
		return false
	}

	// Do not add tokens which are both internal and external. Their validity
	// could influence the behavior of the external scanner. internalExternal
	// is indexed by terminal index only, so gate on the kind.
	if newToken.IsTerminal() && bits.internalExternal[newToken.Index/64]&(1<<(newToken.Index%64)) != 0 {
		return true
	}

	sg := m.syntaxGrammar
	newTokenIsWord := sg.HasWord && sg.WordToken == newToken
	newTokenIsKeyword := sg.HasWord && m.keywords.Contains(newToken)
	// Do not add a token if it conflicts with an existing token. Test the
	// candidate's conflict row against the state's terminal bits, masking
	// out the word and keyword exemptions.
	row := bits.getConflictRow(int(newToken.Index))
	rightTerminalBits := bits.getStateRow(int(rightState.ID))
	for w, rowWord := range row {
		candidates := rightTerminalBits[w] & rowWord
		if newTokenIsKeyword && sg.WordToken.IsTerminal() && int(sg.WordToken.Index)/64 == w {
			candidates &^= 1 << (sg.WordToken.Index % 64)
		}
		if newTokenIsWord {
			candidates &^= bits.keywords[w]
		}
		if candidates != 0 {
			return true
		}
	}

	return false
}

// splitStateIDGroups is dedup.rs: fn split_state_id_groups. For each group
// from startGroupID on (groups that this call appends included), it compares
// each member that is not yet split off (the left state) with each later
// member that is not split off (the right state); a right state for which
// shouldSplit holds is split off. The split-off members of a group, in the
// order they were split off, become a new group at the end, and their group
// ids change only once the whole group has been compared, so the comparisons
// of later groups read them. It reports whether any group split.
//
// Go shape: work counts the comparisons and the states and groups visited
// for the budget of the merge (bMergeWork), as lexbuild's port of the same
// function does for the lex states.
func splitStateIDGroups[S any](states []S, stateIDsByGroupID *[][]uint32, groupIDsByStateID []uint32, startGroupID uint32, shouldSplit func(left, right *S, groupIDsByStateID []uint32) bool, work *int64) bool {
	result := false
	*work += int64(len(states) + len(*stateIDsByGroupID))
	bMergeWork.checkWork(*work)

	// A flat bit set instead of a search of the split list.
	isSplit := make([]bool, len(states))

	groupID := startGroupID
	for int(groupID) < len(*stateIDsByGroupID) {
		stateIDs := (*stateIDsByGroupID)[groupID]
		var splitStateIDs []uint32

		for i := 0; i < len(stateIDs); i++ {
			leftStateID := stateIDs[i]
			if isSplit[leftStateID] {
				continue
			}
			*work += int64(len(stateIDs) - i - 1)
			bMergeWork.checkWork(*work)

			leftState := &states[leftStateID]

			// Identify all of the other states in the group that are
			// incompatible with this state.
			for j := i + 1; j < len(stateIDs); j++ {
				rightStateID := stateIDs[j]
				if isSplit[rightStateID] {
					continue
				}
				if shouldSplit(leftState, &states[rightStateID], groupIDsByStateID) {
					splitStateIDs = append(splitStateIDs, rightStateID)
					isSplit[rightStateID] = true
				}
			}
		}

		// If any states were removed from the group, add them all as a new
		// group.
		if len(splitStateIDs) > 0 {
			result = true
			kept := stateIDs[:0]
			for _, id := range stateIDs {
				if !isSplit[id] {
					kept = append(kept, id)
				}
			}
			(*stateIDsByGroupID)[groupID] = kept

			newGroupID := uint32(len(*stateIDsByGroupID))
			for _, id := range splitStateIDs {
				groupIDsByStateID[id] = newGroupID
				isSplit[id] = false
			}

			*stateIDsByGroupID = append(*stateIDsByGroupID, splitStateIDs)
		}

		groupID++
	}

	return result
}
