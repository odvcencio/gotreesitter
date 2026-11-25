package tablebuild

import (
	"errors"
	"testing"

	"github.com/odvcencio/gotreesitter/internal/lexbuild"
)

func shift(state uint32) ParseAction { return ParseAction{Kind: ActionShift, State: state} }

// TestTerminalRemapSkipsUnreferencedPoolEntries is tables.rs: test
// terminal_remap_skips_unreferenced_pool_entries. remap_terminal_references
// must remap only the pool slots that some state references: slots that no
// state uses (remove_unit_reductions leaves them) can hold Shift targets
// outside the remap's domain.
func TestTerminalRemapSkipsUnreferencedPoolEntries(t *testing.T) {
	table := &InternedTable{}
	table.ActionLists.push([]ParseAction{shift(0)})
	table.ActionLists.push([]ParseAction{shift(1)})
	table.States = append(table.States, InternedState{
		TerminalEntries: []TerminalActionList{{lexbuild.EndSym(), newActionListID(0, true)}},
	})

	// Slot 1 is unreferenced, so f must never see state 1 (else this indexes
	// out of bounds).
	replacement := []uint32{7}
	table.remapTerminalReferences(func(state uint32) uint32 { return replacement[state] })

	if a := table.ActionLists.Get(newActionListID(0, false))[0]; a.Kind != ActionShift || a.State != 7 {
		t.Fatalf("slot 0: %+v, want Shift 7", a)
	}
	if a := table.ActionLists.Get(newActionListID(1, false))[0]; a.Kind != ActionShift || a.State != 1 {
		t.Fatalf("slot 1: %+v, want Shift 1", a)
	}
}

func TestActionListIDReusableBit(t *testing.T) {
	id := newActionListID(5, true)
	if id.Index() != 5 || !id.Reusable() {
		t.Fatalf("new(5, true) = %#x", uint32(id))
	}
	id.setReusable(false)
	if id.Index() != 5 || id.Reusable() {
		t.Fatalf("set_reusable(false) = %#x", uint32(id))
	}
	err := catchGeneratorPanic(func() error { newActionListID(actionListReusableBit, false); return nil })
	var p *GeneratorPanicError
	if !errors.As(err, &p) {
		t.Fatalf("an index at the reusable bit: %v", err)
	}
}

// TestInternAndCanonicalize: intern_table shares equal lists; canonicalize
// rebuilds the pool with the empty list at 0, then the lists in order of
// first use by the states, and keeps each entry's reusable flag.
func TestInternAndCanonicalize(t *testing.T) {
	a, b := []ParseAction{shift(4)}, []ParseAction{shift(2), {Kind: ActionReduce, Symbol: lexbuild.NonTerminalSym(1), ChildCount: 2}}
	raw := &ParseTable{States: []ParseState{
		{TerminalEntries: []TerminalEntry{{lexbuild.TerminalSym(0), ParseTableEntry{Actions: a, Reusable: true}}, {lexbuild.TerminalSym(1), ParseTableEntry{Actions: b, Reusable: true}}}},
		{TerminalEntries: []TerminalEntry{{lexbuild.TerminalSym(0), ParseTableEntry{Actions: b, Reusable: false}}}},
	}}
	table := internTable(raw)
	if table.ActionLists.Len() != 2 {
		t.Fatalf("intern_table made %d lists, want 2", table.ActionLists.Len())
	}
	if raw.States[0].TerminalEntries != nil {
		t.Fatal("intern_table did not move the states")
	}
	ids := []ActionListID{table.States[0].TerminalEntries[0].ID, table.States[0].TerminalEntries[1].ID, table.States[1].TerminalEntries[0].ID}
	if ids[1].Index() != ids[2].Index() || !ids[1].Reusable() || ids[2].Reusable() {
		t.Fatalf("ids %#x", ids)
	}
	// A list that no state uses any more, pushed before the used ones.
	table.ActionLists.push([]ParseAction{shift(9)})
	table.States[1].TerminalEntries[0].ID = newActionListID(0, false)
	table.ActionLists.canonicalize(table.States)
	if table.ActionLists.Len() != 3 {
		t.Fatalf("canonicalize kept %d lists, want 3 (empty, a, b)", table.ActionLists.Len())
	}
	if len(table.ActionLists.Get(newActionListID(0, false))) != 0 {
		t.Fatal("list 0 is not empty")
	}
	got := []ActionListID{table.States[0].TerminalEntries[0].ID, table.States[0].TerminalEntries[1].ID, table.States[1].TerminalEntries[0].ID}
	want := []ActionListID{newActionListID(1, true), newActionListID(2, true), newActionListID(1, false)}
	for i := range got {
		if got[i] != want[i] {
			t.Fatalf("ids %#x, want %#x", got, want)
		}
	}
}

// buildTablesText prepares a grammar.json text and builds its unmerged tables.
func buildTablesText(t *testing.T, text string) (*PreparedGrammar, *Tables) {
	t.Helper()
	g, err := Prepare([]byte(text))
	if err != nil {
		t.Fatalf("prepare: %v", err)
	}
	info, err := GetVariableInfo(g.Syntax, g.Lex, g.DefaultAliases)
	if err != nil {
		t.Fatalf("variable info: %v", err)
	}
	tables, err := BuildTables(g, info, 0)
	if err != nil {
		t.Fatalf("tables: %v", err)
	}
	return g, tables
}

// reduces reports whether any action of the table reduces the named variable.
func reduces(g *PreparedGrammar, table *InternedTable, name string) bool {
	for i := range table.States {
		for _, e := range table.States[i].TerminalEntries {
			for _, a := range table.ActionLists.Get(e.ID) {
				if a.Kind == ActionReduce && g.Syntax.Variables[a.Symbol.Index].Ident == name {
					return true
				}
			}
		}
	}
	return false
}

// TestRemoveUnitReductions: a hidden rule with single-symbol productions is
// reduced in no state of the final table (each Shift and Goto into its
// unit-reduction states goes straight on), and remove_unused_states drops
// those states. A visible rule of the same shape keeps its reductions, and so
// do a hidden rule that some step aliases and a hidden rule with a default
// alias. Each rule follows its own keyword, so that no state reduces two of
// them.
func TestRemoveUnitReductions(t *testing.T) {
	choice := jChoice(jSym("number"), jSym("word"))
	g := tGrammar{rules: [][2]string{
		{"program", jRepeat0(jChoice(
			jSeq(jStr("a"), jSym("_value"), jStr(";")),
			jSeq(jStr("b"), jSym("item"), jStr(",")),
			jSeq(jStr("c"), jAlias(jSym("_partly_aliased"), "v", true), jStr(".")),
			jSeq(jStr("d"), jSym("_partly_aliased"), jStr("!")),
			jSeq(jStr("e"), jAlias(jSym("_always_aliased"), "w", true), jStr("?")),
		))},
		{"_value", choice},
		{"item", choice},
		{"_partly_aliased", choice},
		{"_always_aliased", choice},
		{"number", jPat(`\d+`)},
		{"word", jPat(`[a-z]+`)},
	}}
	pg, tables := buildTablesText(t, g.text())
	s := tables.Stats
	if s.UnitReductionStates == 0 {
		t.Fatal("no unit-reduction state found")
	}
	if s.AfterUnusedStates >= s.RawStates || s.StatesAfterMinimizing != s.AfterUnusedStates {
		t.Fatalf("states: raw %d, after remove_unused_states %d, final %d", s.RawStates, s.AfterUnusedStates, s.StatesAfterMinimizing)
	}
	if reduces(pg, tables.ParseTable, "_value") {
		t.Error("_value is still reduced")
	}
	for _, name := range []string{"item", "_partly_aliased", "_always_aliased"} {
		if !reduces(pg, tables.ParseTable, name) {
			t.Errorf("%s lost its reductions", name)
		}
	}
	t.Logf("states: raw %d, unit-reduction states %d, after remove_unused_states %d", s.RawStates, s.UnitReductionStates, s.AfterUnusedStates)
}

// TestRemoveUnusedStatesRenumbers: the states that remain keep their order,
// and every Shift and Goto names its state's new number.
func TestRemoveUnusedStatesRenumbers(t *testing.T) {
	pool := ActionListPool{}
	pool.push([]ParseAction{shift(3)}) // 0: used by state 0
	pool.push([]ParseAction{shift(3)}) // 1: used only by state 2, which no state reaches
	table := &InternedTable{ActionLists: pool, States: []InternedState{
		{ID: 0, TerminalEntries: []TerminalActionList{{lexbuild.TerminalSym(0), newActionListID(0, true)}}},
		{ID: 1, NonterminalEntries: []NonterminalEntry{{lexbuild.NonTerminalSym(0), GotoAction{State: 4}}}},
		{ID: 2, TerminalEntries: []TerminalActionList{{lexbuild.TerminalSym(0), newActionListID(1, true)}}},
		{ID: 3},
		{ID: 4, NonterminalEntries: []NonterminalEntry{{lexbuild.NonTerminalSym(0), GotoAction{ShiftExtra: true}}}},
	}}
	m := &minimizer{parseTable: table}
	m.removeUnusedStates()
	if len(table.States) != 4 {
		t.Fatalf("%d states, want 4", len(table.States))
	}
	for i, want := range []uint32{0, 1, 3, 4} {
		if table.States[i].ID != want {
			t.Fatalf("state %d has search id %d, want %d", i, table.States[i].ID, want)
		}
	}
	if a := table.ActionLists.Get(table.States[0].TerminalEntries[0].ID)[0]; a.State != 2 {
		t.Errorf("state 0 shifts to %d, want 2", a.State)
	}
	if g := table.States[1].NonterminalEntries[0].Action; g.State != 3 {
		t.Errorf("state 1 goes to %d, want 3", g.State)
	}
	// The unused list keeps its target.
	if a := table.ActionLists.Get(newActionListID(1, false))[0]; a.State != 3 {
		t.Errorf("the unused list shifts to %d, want 3 (unchanged)", a.State)
	}
}
