package tablebuild

import (
	"errors"

	"github.com/odvcencio/gotreesitter/internal/lexbuild"
)

// This file ports build_tables/minimize_parse_table.rs: fn
// minimize_parse_table and fn Minimizer::remove_unit_reductions. reorder.go
// ports remove_unused_states and reorder_states_by_descending_size. The merge
// of compatible states (merge_compatible_states and the functions it calls)
// is not ported yet.

// OptLevel is generate.rs: struct OptLevel, a set of bit flags. The
// generator's default is OptMergeStates; its --disable-optimizations flag
// gives the empty set.
type OptLevel uint32

// OptMergeStates is OptLevel::MergeStates.
const OptMergeStates OptLevel = 1 << 0

// ErrMergeNotPorted is returned for OptMergeStates: merge_compatible_states
// is not ported yet, and a faithful build does not fall back to another merge.
var ErrMergeNotPorted = errors.New("tablebuild: merge_compatible_states is not ported yet; build with OptLevel 0 (the generator's --disable-optimizations)")

// minimizer is minimize_parse_table.rs: struct Minimizer. The token conflict
// map and the keywords are read only by the merge.
type minimizer struct {
	parseTable    *InternedTable
	syntaxGrammar *lexbuild.SyntaxGrammar
	simpleAliases AliasMap

	unitReductionStates int // the states that removeUnitReductions skips over
}

// minimizeParseTable is minimize_parse_table.rs: fn minimize_parse_table. It
// fills the counts of stats as the passes run.
func minimizeParseTable(parseTable *InternedTable, sg *lexbuild.SyntaxGrammar, simpleAliases AliasMap, optimizations OptLevel, stats *TableStats) error {
	m := &minimizer{parseTable: parseTable, syntaxGrammar: sg, simpleAliases: simpleAliases}
	if optimizations&OptMergeStates != 0 {
		return ErrMergeNotPorted
	}
	if err := m.removeUnitReductions(); err != nil {
		return err
	}
	stats.UnitReductionStates = m.unitReductionStates
	stats.AfterUnitReductions = parseTable.ActionLists.Len()
	m.removeUnusedStates()
	stats.AfterUnusedStates = len(parseTable.States)
	m.reorderStatesByDescendingSize()
	return nil
}

// removeUnitReductions is minimize_parse_table.rs: fn
// Minimizer::remove_unit_reductions. A unit-reduction state is one without an
// end-of-input-gated reduce whose only actions (besides ShiftExtra) reduce one
// symbol over one child with production id 0, where the symbol is not a named
// rule, a supertype, an extra, a symbol with a default alias or a symbol that
// some step aliases. Every Shift and Goto into such a state is redirected to
// the state that the referring state's goto on the reduced symbol reaches.
//
// The generator collects the aliased symbols and the unit-reduction states in
// hash maps that are only read by key, so their hash order reaches no result.
// It visits the states in index order and repeats each state until a pass
// changes nothing; where a pass would see the state unchanged and still ask
// for another pass, the generator never returns, and the port returns a
// *GeneratorPanicError instead.
func (m *minimizer) removeUnitReductions() error {
	sg := m.syntaxGrammar
	table := m.parseTable

	aliasedSymbols := map[Symbol]bool{}
	for i := range sg.Variables {
		vp := sg.VarProds[i]
		for prodID := vp[0]; prodID < vp[1]; prodID++ {
			for _, step := range sg.Productions[prodID].Steps {
				if step.Alias.Set {
					aliasedSymbols[step.Symbol] = true
				}
			}
		}
	}

	unitReductionSymbolsByState := map[uint32]Symbol{}
	for i := range table.States {
		state := &table.States[i]
		if state.HasEOFGatedReduce {
			continue
		}
		onlyUnitReductions := true
		var unitReductionSymbol Symbol
		hasUnitReductionSymbol := false
	entries:
		for _, e := range state.TerminalEntries {
			for _, action := range table.ActionLists.Get(e.ID) {
				switch {
				case action.Kind == ActionShiftExtra:
					continue
				case action.Kind == ActionReduce && action.ChildCount == 1 && action.ProductionID == 0 &&
					!m.isSimpleAliased(action.Symbol) &&
					!containsSymbol(sg.SupertypeSymbols, action.Symbol) &&
					!containsSymbol(sg.ExtraSymbols, action.Symbol) &&
					!aliasedSymbols[action.Symbol] &&
					sg.Variables[action.Symbol.Index].Kind != lexbuild.VarNamed &&
					(!hasUnitReductionSymbol || unitReductionSymbol == action.Symbol):
					unitReductionSymbol, hasUnitReductionSymbol = action.Symbol, true
					continue
				}
				onlyUnitReductions = false
				break entries
			}
		}
		if hasUnitReductionSymbol && onlyUnitReductions {
			unitReductionSymbolsByState[uint32(i)] = unitReductionSymbol
		}
	}

	m.unitReductionStates = len(unitReductionSymbolsByState)
	if len(unitReductionSymbolsByState) == 0 {
		return nil
	}

	actionListIDs := actionListDedup{}
	var actions []ParseAction
	for stateIndex := range table.States {
		passes := 0
		done := false
		for !done {
			done = true
			changed := false
			passes++
			if passes > len(table.States)+1 {
				return &GeneratorPanicError{What: "remove_unit_reductions repeats a state without end (the generator never returns)"}
			}
			state := &table.States[stateIndex]

			state.updateNonterminalReferences(func(otherStateID uint32, s *InternedState) uint32 {
				symbol, ok := unitReductionSymbolsByState[otherStateID]
				if !ok {
					return otherStateID
				}
				done = false
				if g, ok := s.nonterminal(symbol); ok && !g.ShiftExtra {
					if g.State != otherStateID {
						changed = true
					}
					return g.State
				}
				return otherStateID
			})

			for i := range state.TerminalEntries {
				oldID := state.TerminalEntries[i].ID
				actions = append(actions[:0], table.ActionLists.Get(oldID)...)
				entryChanged := false
				for k := range actions {
					// A Shift onto a unit-reduction state (one whose only action
					// reduces a single symbol) can skip it. Shift then reduce then
					// goto is equivalent to shifting straight to the goto target
					// for that symbol in this case.
					target := &actions[k]
					if target.Kind != ActionShift {
						continue
					}
					symbol, ok := unitReductionSymbolsByState[target.State]
					if !ok {
						continue
					}
					g, ok := state.nonterminal(symbol)
					if !ok || g.ShiftExtra || g.State == target.State {
						continue
					}
					target.State = g.State
					entryChanged = true
					done = false
				}
				if entryChanged {
					changed = true
					index := table.ActionLists.intern(actionListIDs, actions)
					state.TerminalEntries[i].ID = newActionListID(index, oldID.Reusable())
				}
			}

			if !done && !changed {
				return &GeneratorPanicError{What: "remove_unit_reductions asks for another pass over an unchanged state (the generator never returns)"}
			}
		}
	}
	return nil
}

// isSimpleAliased is simple_aliases.contains_key.
func (m *minimizer) isSimpleAliased(s Symbol) bool {
	_, ok := m.simpleAliases[s]
	return ok
}
