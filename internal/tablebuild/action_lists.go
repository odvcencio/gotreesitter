package tablebuild

import (
	"encoding/binary"

	"github.com/odvcencio/gotreesitter/internal/lexbuild"
)

// This file ports the action list pool of tables.rs: struct ActionListId,
// struct ActionListPool (push, get, len, intern, intern_table,
// canonicalize), the methods of ParseState<ActionListId> (referenced_states,
// update_nonterminal_references) and fn
// ParseTable<ActionListId>::remap_terminal_references. tables.rs: enum
// ActionList is a []ParseAction here: its three variants only choose how the
// generator stores a list, and two lists are equal when their actions are.

// ActionListID is tables.rs: struct ActionListId: the index of an action list
// in an ActionListPool, with the high bit set when the entry is reusable.
type ActionListID uint32

// actionListReusableBit is tables.rs: const ActionListId::REUSABLE_BIT.
const actionListReusableBit = 1 << 31

// newActionListID is tables.rs: fn ActionListId::new.
func newActionListID(index uint32, reusable bool) ActionListID {
	if index >= actionListReusableBit {
		panicLikeGenerator("action list index %d reaches the reusable bit", index)
	}
	if reusable {
		return ActionListID(index | actionListReusableBit)
	}
	return ActionListID(index)
}

// Index is tables.rs: fn ActionListId::index.
func (id ActionListID) Index() int { return int(uint32(id) &^ actionListReusableBit) }

// Reusable is tables.rs: fn ActionListId::reusable.
func (id ActionListID) Reusable() bool { return uint32(id)&actionListReusableBit != 0 }

// setReusable is tables.rs: fn ActionListId::set_reusable.
func (id *ActionListID) setReusable(reusable bool) {
	*id = newActionListID(uint32(id.Index()), reusable)
}

// actionListRange is tables.rs: struct ActionListRange.
type actionListRange struct {
	offset, len uint32
}

// ActionListPool is tables.rs: struct ActionListPool: the terminal action
// lists of a parse table, each stored as a range of one action slice.
type ActionListPool struct {
	actions      []ParseAction
	ranges       []actionListRange
	remapScratch []bool
}

// push is tables.rs: fn ActionListPool::push.
func (p *ActionListPool) push(list []ParseAction) uint32 {
	index := uint32(len(p.ranges))
	offset := uint32(len(p.actions))
	p.actions = append(p.actions, list...)
	p.ranges = append(p.ranges, actionListRange{offset: offset, len: uint32(len(list))})
	return index
}

// Get is tables.rs: fn ActionListPool::get.
func (p *ActionListPool) Get(id ActionListID) []ParseAction {
	r := p.ranges[id.Index()]
	return p.actions[r.offset : r.offset+r.len : r.offset+r.len]
}

// Len is tables.rs: fn ActionListPool::len.
func (p *ActionListPool) Len() int { return len(p.ranges) }

// actionListDedup is the FxHashMap<ActionList, u32> that intern reads and
// fills. It is only read by key, so its hash order reaches no result.
type actionListDedup map[string]uint32

// actionListKey encodes an action list so that two lists have equal keys
// exactly when their actions are equal (the derived PartialEq of ActionList).
func actionListKey(list []ParseAction) string {
	const size = 1 + 4 + 1 + 1 + 4 + 2 + 4 + 2
	b := make([]byte, 0, size*len(list))
	for _, a := range list {
		var rep byte
		if a.IsRepetition {
			rep = 1
		}
		b = append(b, byte(a.Kind))
		b = binary.LittleEndian.AppendUint32(b, a.State)
		b = append(b, rep, byte(a.Symbol.Kind))
		b = binary.LittleEndian.AppendUint32(b, a.Symbol.Index)
		b = binary.LittleEndian.AppendUint16(b, a.ChildCount)
		b = binary.LittleEndian.AppendUint32(b, uint32(a.DynamicPrecedence))
		b = binary.LittleEndian.AppendUint16(b, a.ProductionID)
	}
	return string(b)
}

// intern is tables.rs: fn ActionListPool::intern: the index of an equal list
// that this dedup map has seen, or the index of the list pushed now.
func (p *ActionListPool) intern(dedup actionListDedup, list []ParseAction) uint32 {
	key := actionListKey(list)
	if index, ok := dedup[key]; ok {
		return index
	}
	index := p.push(list)
	dedup[key] = index
	return index
}

// TerminalActionList is one entry of InternedState.TerminalEntries.
type TerminalActionList struct {
	Symbol Symbol
	ID     ActionListID
}

// InternedState is tables.rs: struct ParseState<ActionListId>, the parse
// state of a table whose terminal entries are interned. As in ParseState, the
// entry slices keep the insertion order of the generator's IndexMaps.
type InternedState struct {
	ID                 uint32
	TerminalEntries    []TerminalActionList
	NonterminalEntries []NonterminalEntry
	ReservedWords      lexbuild.TokenSet
	LexStateID         uint32
	ExternalLexStateID uint32
	CoreID             uint32
	HasEOFGatedReduce  bool
}

// InternedTable is tables.rs: struct ParseTable<ActionListId>, the parse
// table after ActionListPool::intern_table.
type InternedTable struct {
	States                     []InternedState
	ActionLists                ActionListPool
	Symbols                    []Symbol
	ProductionInfos            []ProductionInfo
	MaxAliasedProductionLength int
	ExternalLexStates          []lexbuild.TokenSet
}

// internTable is tables.rs: fn ActionListPool::intern_table. It moves the
// table: the states of t are emptied.
func internTable(t *ParseTable) *InternedTable {
	out := &InternedTable{
		States:                     make([]InternedState, len(t.States)),
		Symbols:                    t.Symbols,
		ProductionInfos:            t.ProductionInfos,
		MaxAliasedProductionLength: t.MaxAliasedProductionLength,
		ExternalLexStates:          t.ExternalLexStates,
	}
	ids := actionListDedup{}
	for i := range t.States {
		state := &t.States[i]
		entries := make([]TerminalActionList, len(state.TerminalEntries))
		for k, e := range state.TerminalEntries {
			index := out.ActionLists.intern(ids, e.Entry.Actions)
			entries[k] = TerminalActionList{Symbol: e.Symbol, ID: newActionListID(index, e.Entry.Reusable)}
		}
		out.States[i] = InternedState{
			ID:                 state.ID,
			TerminalEntries:    entries,
			NonterminalEntries: state.NonterminalEntries,
			ReservedWords:      state.ReservedWords,
			LexStateID:         state.LexStateID,
			ExternalLexStateID: state.ExternalLexStateID,
			CoreID:             state.CoreID,
			HasEOFGatedReduce:  state.HasEOFGatedReduce,
		}
		*state = ParseState{}
	}
	return out
}

// canonicalize is tables.rs: fn ActionListPool::canonicalize. It rebuilds
// the pool so that it holds each list once: index 0 is the empty list, then
// the lists in the order the states (and, within a state, the entries) first
// use them. Lists that the states no longer use are dropped.
func (p *ActionListPool) canonicalize(states []InternedState) {
	oldActions, oldRanges := p.actions, p.ranges
	p.actions, p.ranges = nil, nil
	ids := actionListDedup{}
	// render's "no action" default
	p.intern(ids, nil)
	oldToNew := make([]uint32, len(oldRanges))
	for i := range oldToNew {
		oldToNew[i] = ^uint32(0)
	}
	for si := range states {
		for k := range states[si].TerminalEntries {
			id := &states[si].TerminalEntries[k].ID
			oldIndex := id.Index()
			newIndex := oldToNew[oldIndex]
			// INVARIANT: ParseTableEntry::index can never return u32::MAX
			if newIndex == ^uint32(0) {
				r := oldRanges[oldIndex]
				newIndex = p.intern(ids, oldActions[r.offset:r.offset+r.len])
				oldToNew[oldIndex] = newIndex
			}
			*id = newActionListID(newIndex, id.Reusable())
		}
	}
}

// isEndOfNonTerminalExtra is tables.rs: fn ParseState::is_end_of_non_terminal_extra.
func (s *InternedState) isEndOfNonTerminalExtra() bool {
	for _, e := range s.TerminalEntries {
		if e.Symbol == lexbuild.EndOfNonTerminalExtraSym() {
			return true
		}
	}
	return false
}

// nonterminal is IndexMap::get on the non-terminal entries.
func (s *InternedState) nonterminal(sym Symbol) (GotoAction, bool) {
	for _, e := range s.NonterminalEntries {
		if e.Symbol == sym {
			return e.Action, true
		}
	}
	return GotoAction{}, false
}

// insertNonterminal is IndexMap::insert on the non-terminal entries: replace
// the value in place, or append.
func (s *InternedState) insertNonterminal(sym Symbol, action GotoAction) {
	for i := range s.NonterminalEntries {
		if s.NonterminalEntries[i].Symbol == sym {
			s.NonterminalEntries[i].Action = action
			return
		}
	}
	s.NonterminalEntries = append(s.NonterminalEntries, NonterminalEntry{sym, action})
}

// referencedStates is tables.rs: fn ParseState::referenced_states: it calls
// f with the target of every Shift of the terminal entries, in entry and
// action order, then with the target of every Goto.
func (s *InternedState) referencedStates(pool *ActionListPool, f func(uint32)) {
	for _, e := range s.TerminalEntries {
		for _, a := range pool.Get(e.ID) {
			if a.Kind == ActionShift {
				f(a.State)
			}
		}
	}
	for _, e := range s.NonterminalEntries {
		if !e.Action.ShiftExtra {
			f(e.Action.State)
		}
	}
}

// updateNonterminalReferences is tables.rs: fn
// ParseState::update_nonterminal_references. It calls f with the target of
// each Goto and the state as it was before this call, then stores the
// targets that f changed.
func (s *InternedState) updateNonterminalReferences(f func(other uint32, s *InternedState) uint32) {
	type update struct {
		symbol Symbol
		state  uint32
	}
	var updates []update
	for _, e := range s.NonterminalEntries {
		if e.Action.ShiftExtra {
			continue
		}
		if result := f(e.Action.State, s); result != e.Action.State {
			updates = append(updates, update{e.Symbol, result})
		}
	}
	for _, u := range updates {
		s.insertNonterminal(u.symbol, GotoAction{State: u.state})
	}
}

// remapTerminalReferences is tables.rs: fn
// ParseTable<ActionListId>::remap_terminal_references. It rewrites the
// Shift targets of the pool's lists that some state uses, each list once;
// a list that no state uses keeps its targets.
func (t *InternedTable) remapTerminalReferences(f func(uint32) uint32) {
	p := &t.ActionLists
	if cap(p.remapScratch) < p.Len() {
		p.remapScratch = make([]bool, p.Len())
	}
	p.remapScratch = p.remapScratch[:p.Len()]
	clear(p.remapScratch)
	for si := range t.States {
		for _, e := range t.States[si].TerminalEntries {
			p.remapScratch[e.ID.Index()] = true
		}
	}
	for index, r := range p.ranges {
		if !p.remapScratch[index] {
			continue
		}
		for k := r.offset; k < r.offset+r.len; k++ {
			if a := &p.actions[k]; a.Kind == ActionShift {
				a.State = f(a.State)
			}
		}
	}
}
