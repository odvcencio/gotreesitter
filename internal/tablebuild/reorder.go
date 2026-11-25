package tablebuild

// This file ports build_tables/minimize_parse_table.rs: fn
// Minimizer::remove_unused_states and fn
// Minimizer::reorder_states_by_descending_size. Neither changes a state's ID
// field: like the generator, the port keeps the id that the search gave the
// state.

// removeUnusedStates is minimize_parse_table.rs: fn
// Minimizer::remove_unused_states. It removes every state, other than the
// error state (0) and the start state (1), that no Shift and no Goto of any
// state reaches, and renumbers the rest in order. One pass: a state that only
// removed states reach is kept.
func (m *minimizer) removeUnusedStates() {
	table := m.parseTable
	if len(table.States) < 2 {
		panicLikeGenerator("remove_unused_states on %d states", len(table.States))
	}
	stateUsageMap := make([]bool, len(table.States))
	stateUsageMap[0] = true
	stateUsageMap[1] = true
	for i := range table.States {
		table.States[i].referencedStates(&table.ActionLists, func(referencedState uint32) {
			stateUsageMap[referencedState] = true
		})
	}

	removedPredecessorCount := 0
	stateReplacementMap := make([]uint32, len(table.States))
	for stateID := range table.States {
		stateReplacementMap[stateID] = uint32(stateID - removedPredecessorCount)
		if !stateUsageMap[stateID] {
			removedPredecessorCount++
		}
	}

	// The generator removes each unused state from the vector in place; the
	// port compacts the kept states in one pass, in the same order.
	kept := table.States[:0]
	for originalStateID := range table.States {
		if !stateUsageMap[originalStateID] {
			continue
		}
		state := table.States[originalStateID]
		state.updateNonterminalReferences(func(otherStateID uint32, _ *InternedState) uint32 {
			return stateReplacementMap[otherStateID]
		})
		kept = append(kept, state)
	}
	for i := len(kept); i < len(table.States); i++ {
		table.States[i] = InternedState{}
	}
	table.States = kept
	table.remapTerminalReferences(func(stateID uint32) uint32 { return stateReplacementMap[stateID] })
}

// reorderStatesByDescendingSize is minimize_parse_table.rs: fn
// Minimizer::reorder_states_by_descending_size. States 0 and 1 stay first;
// the others are ordered by their entry count, largest first, with
// slice::sort_unstable_by_key. States with equal entry counts take the order
// that the standard library's unstable sort gives them; width_order.go ports
// that sort.
func (m *minimizer) reorderStatesByDescendingSize() {
	table := m.parseTable
	// Get a mapping of old state index -> new state index.
	oldIDsByNewID := make([]int, len(table.States))
	for i := range oldIDsByNewID {
		oldIDsByNewID[i] = i
	}
	sortUnstableByKey(oldIDsByNewID, func(i int) int64 {
		// Don't change states 0 (the error state) or 1 (the start state).
		if i <= 1 {
			return int64(i) - 1_000_000
		}
		// Reorder all the other states by descending symbol count.
		state := &table.States[i]
		return -int64(len(state.TerminalEntries) + len(state.NonterminalEntries))
	})

	// Get the inverse mapping.
	newIDsByOldID := make([]uint32, len(oldIDsByNewID))
	for id, oldID := range oldIDsByNewID {
		newIDsByOldID[oldID] = uint32(id)
	}

	// Reorder the parse states and update their references to reflect the
	// new ordering.
	states := make([]InternedState, len(oldIDsByNewID))
	for id, oldID := range oldIDsByNewID {
		state := table.States[oldID]
		table.States[oldID] = InternedState{}
		state.updateNonterminalReferences(func(other uint32, _ *InternedState) uint32 { return newIDsByOldID[other] })
		states[id] = state
	}
	table.States = states
	table.remapTerminalReferences(func(id uint32) uint32 { return newIDsByOldID[id] })
}
