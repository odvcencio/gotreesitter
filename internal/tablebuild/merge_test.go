package tablebuild

import (
	"errors"
	"fmt"
	"testing"

	"github.com/odvcencio/gotreesitter/internal/lexbuild"
)

// The generator has no unit tests for minimize_parse_table.rs and dedup.rs
// (v0.27.1); these tests check the ported functions one by one, then the
// merge of a small grammar end to end. Layer L4 (l4_test.go) checks the
// whole merge against the generator's output.

// splitCase runs splitStateIDGroups on states 0..n-1 with the given groups
// and returns the groups, the group of each state and the result.
func splitCase(t *testing.T, n int, groups [][]uint32, start uint32, split func(l, r int, groupOf []uint32) bool) ([][]uint32, []uint32, bool) {
	t.Helper()
	states := make([]int, n)
	groupOf := make([]uint32, n)
	for i := range states {
		states[i] = i
	}
	for g, ids := range groups {
		for _, id := range ids {
			groupOf[id] = uint32(g)
		}
	}
	var work int64
	changed := splitStateIDGroups(states, &groups, groupOf, start, func(l, r *int, groupOf []uint32) bool {
		return split(*l, *r, groupOf)
	}, &work)
	return groups, groupOf, changed
}

// TestSplitStateIDGroupsOrder: the split-off members form a new group at the
// end, in the order they were split off (by left state, then right state),
// not in state order; a member split off by one left state is compared with
// no later left state.
func TestSplitStateIDGroupsOrder(t *testing.T) {
	pairs := map[[2]int]bool{{0, 3}: true, {1, 2}: true, {1, 3}: true}
	var compared [][2]int
	groups, groupOf, changed := splitCase(t, 4, [][]uint32{{0, 1, 2, 3}}, 0, func(l, r int, _ []uint32) bool {
		compared = append(compared, [2]int{l, r})
		return pairs[[2]int{l, r}]
	})
	if !changed {
		t.Fatal("no split reported")
	}
	if got, want := fmt.Sprint(groups), "[[0 1] [3 2]]"; got != want {
		t.Fatalf("groups %s, want %s", got, want)
	}
	if got, want := fmt.Sprint(groupOf), "[0 0 1 1]"; got != want {
		t.Fatalf("group of each state %s, want %s", got, want)
	}
	// Left 0 meets 1, 2, 3 (3 splits off); left 1 meets 2 (2 splits off);
	// in the appended group, left 3 meets 2.
	if got, want := fmt.Sprint(compared), "[[0 1] [0 2] [0 3] [1 2] [3 2]]"; got != want {
		t.Fatalf("comparisons %s, want %s", got, want)
	}
}

// TestSplitStateIDGroupsSameCall: a group appended by a call is compared in
// the same call, and the group ids of split-off members change once their
// whole group has been compared, so that later groups (and not the rest of
// the same group) read them.
func TestSplitStateIDGroupsSameCall(t *testing.T) {
	var seen []string
	groups, _, changed := splitCase(t, 5, [][]uint32{{0, 1, 2}, {3, 4}}, 0, func(l, r int, groupOf []uint32) bool {
		seen = append(seen, fmt.Sprintf("%d-%d:%v", l, r, groupOf))
		switch {
		case l == 0 && r == 1, l == 0 && r == 2:
			return true // 1 and 2 leave group 0 together
		case l == 1 && r == 2:
			return true // then split again, in the appended group
		}
		return false
	})
	if !changed {
		t.Fatal("no split reported")
	}
	if got, want := fmt.Sprint(groups), "[[0] [3 4] [1] [2]]"; got != want {
		t.Fatalf("groups %s, want %s", got, want)
	}
	want := []string{
		"0-1:[0 0 0 1 1]", "0-2:[0 0 0 1 1]", // group 0, ids unchanged while it is compared
		"3-4:[0 2 2 1 1]", // group 1 reads the new group of 1 and 2
		"1-2:[0 2 2 1 1]", // the appended group 2
	}
	if fmt.Sprint(seen) != fmt.Sprint(want) {
		t.Fatalf("comparisons %v, want %v", seen, want)
	}
}

// TestSplitStateIDGroupsStart: groups before the start group are not
// compared, and a call that splits nothing reports false.
func TestSplitStateIDGroupsStart(t *testing.T) {
	groups, _, changed := splitCase(t, 4, [][]uint32{{0, 1}, {2, 3}}, 1, func(l, r int, _ []uint32) bool {
		if l == 0 {
			t.Errorf("group 0 compared")
		}
		return false
	})
	if changed {
		t.Fatal("a split reported")
	}
	if got, want := fmt.Sprint(groups), "[[0 1] [2 3]]"; got != want {
		t.Fatalf("groups %s, want %s", got, want)
	}
}

func TestSymbolKeyOrder(t *testing.T) {
	syms := []Symbol{
		lexbuild.ExternalSym(7), lexbuild.EndSym(), lexbuild.EndOfNonTerminalExtraSym(),
		lexbuild.TerminalSym(0), lexbuild.TerminalSym(300), lexbuild.NonTerminalSym(2),
	}
	for i, a := range syms {
		if got := newSymbolKey(a).symbol(); got != a {
			t.Errorf("key of %v decodes to %v", a, got)
		}
		for j, b := range syms {
			if (newSymbolKey(a) < newSymbolKey(b)) != lexbuild.SymbolLess(a, b) {
				t.Errorf("key order of %v and %v differs from Symbol order", syms[i], syms[j])
			}
		}
	}
}

// conflictCase is a minimizer and the conflict bits of two terminals' rows,
// for token_conflicts: terminal 0 conflicts with terminal 1, terminal 3 is
// the word token and terminal 4 a keyword (they conflict both ways), and
// terminal 2 is also an external token.
func conflictCase() (*minimizer, *conflictBits) {
	sg := &lexbuild.SyntaxGrammar{WordToken: lexbuild.TerminalSym(3), HasWord: true}
	m := &minimizer{syntaxGrammar: sg}
	m.keywords.Insert(lexbuild.TerminalSym(4))
	bits := &conflictBits{rowWords: 1, keywords: []uint64{1 << 4}, internalExternal: []uint64{1 << 2}}
	bits.conflictRows = []uint64{1 << 1, 0, 0, 1 << 4, 1 << 3}
	return m, bits
}

// TestTokenConflicts: each rule of token_conflicts, in its order.
func TestTokenConflicts(t *testing.T) {
	m, bits := conflictCase()
	state := func(id uint32, terminals uint64) *InternedState {
		bits.stateTerminals = append(bits.stateTerminals[:id], terminals)
		return &InternedState{ID: id}
	}
	withTerminal1 := state(0, 1<<1)
	cases := []struct {
		name  string
		right *InternedState
		token Symbol
		want  bool
	}{
		{"end of non-terminal extra", withTerminal1, lexbuild.EndOfNonTerminalExtraSym(), true},
		{"external token", withTerminal1, lexbuild.ExternalSym(0), true},
		{"conflicting terminal", withTerminal1, lexbuild.TerminalSym(0), true},
		{"terminal without conflicts", withTerminal1, lexbuild.TerminalSym(1), false},
		{"internal and external terminal", withTerminal1, lexbuild.TerminalSym(2), true},
		// The end token reads the conflict row of terminal 0.
		{"end token reads row 0", withTerminal1, lexbuild.EndSym(), true},
	}
	for _, c := range cases {
		if got := m.tokenConflicts(9, c.right.ID, c.right, bits, c.token); got != c.want {
			t.Errorf("%s: %v, want %v", c.name, got, c.want)
		}
	}

	// A reserved word of the right state never conflicts, even when it is an
	// internal and external token.
	reserved := state(0, 1<<1)
	reserved.ReservedWords.Insert(lexbuild.TerminalSym(0))
	reserved.ReservedWords.Insert(lexbuild.TerminalSym(2))
	for _, tok := range []Symbol{lexbuild.TerminalSym(0), lexbuild.TerminalSym(2)} {
		if m.tokenConflicts(9, 0, reserved, bits, tok) {
			t.Errorf("reserved word %v conflicts", tok)
		}
	}

	// The word token and a keyword do not conflict with each other, though
	// their rows say they do; without a word token they would.
	withKeyword, withWord := state(0, 1<<4), state(1, 1<<3)
	if m.tokenConflicts(9, 0, withKeyword, bits, lexbuild.TerminalSym(3)) {
		t.Error("the word token conflicts with a keyword")
	}
	if m.tokenConflicts(9, 1, withWord, bits, lexbuild.TerminalSym(4)) {
		t.Error("a keyword conflicts with the word token")
	}
	m.syntaxGrammar.HasWord = false
	if !m.tokenConflicts(9, 1, withWord, bits, lexbuild.TerminalSym(4)) {
		t.Error("without a word token, terminal 4 does not conflict with terminal 3")
	}
}

// TestEntriesConflict: entries are compatible when they share a pool list,
// or when their lists are equal with Shifts compared by target group and
// repetition flag.
func TestEntriesConflict(t *testing.T) {
	reduce := ParseAction{Kind: ActionReduce, Symbol: lexbuild.NonTerminalSym(0), ChildCount: 1}
	m := &minimizer{parseTable: &InternedTable{}}
	pool := &m.parseTable.ActionLists
	list := func(actions ...ParseAction) ActionListID { return newActionListID(pool.push(actions), true) }
	groupOf := []uint32{0, 1, 1}
	shiftRep := ParseAction{Kind: ActionShift, State: 2, IsRepetition: true}
	cases := []struct {
		name   string
		a, b   ActionListID
		expect bool
	}{
		{"one list, reusable flags differ", newActionListID(0, true), newActionListID(0, false), false},
		{"shifts to one group", list(shift(1)), list(shift(2)), false},
		{"shifts to two groups", list(shift(0)), list(shift(1)), true},
		{"repetition flags differ", list(shift(1)), list(shiftRep), true},
		{"lengths differ", list(shift(1)), list(shift(1), reduce), true},
		{"equal reduces after shifts", list(shift(1), reduce), list(shift(2), reduce), false},
		{"shift against reduce", list(shift(1)), list(reduce), true},
	}
	for _, c := range cases {
		if got := m.entriesConflict(0, 1, lexbuild.TerminalSym(0), c.a, c.b, groupOf); got != c.expect {
			t.Errorf("%s: %v, want %v", c.name, got, c.expect)
		}
	}
}

// TestStateSuccessorsDiffer: only tokens and non-terminals that both states
// have count; ShiftExtra against a Goto differs.
func TestStateSuccessorsDiffer(t *testing.T) {
	m := &minimizer{}
	groupOf := []uint32{0, 1, 2, 2, 3}
	a, b := &InternedState{ID: 0}, &InternedState{ID: 1}
	k := func(i int) symbolKey { return newSymbolKey(lexbuild.TerminalSym(i)) }
	cases := []struct {
		name           string
		shifts1        []keyedShift
		shifts2        []keyedShift
		gotos1, gotos2 []indexedGoto
		want           bool
	}{
		{"same groups", []keyedShift{{k(0), 2}}, []keyedShift{{k(0), 3}}, nil, nil, false},
		{"different groups", []keyedShift{{k(0), 2}}, []keyedShift{{k(0), 4}}, nil, nil, true},
		{"tokens of one state only", []keyedShift{{k(0), 2}}, []keyedShift{{k(1), 4}}, nil, nil, false},
		{"gotos to one group", nil, nil, []indexedGoto{{5, GotoAction{State: 2}}}, []indexedGoto{{5, GotoAction{State: 3}}}, false},
		{"gotos to two groups", nil, nil, []indexedGoto{{5, GotoAction{State: 2}}}, []indexedGoto{{5, GotoAction{State: 4}}}, true},
		{"ShiftExtra on both", nil, nil, []indexedGoto{{5, GotoAction{ShiftExtra: true}}}, []indexedGoto{{5, GotoAction{ShiftExtra: true}}}, false},
		{"ShiftExtra against a goto", nil, nil, []indexedGoto{{5, GotoAction{ShiftExtra: true}}}, []indexedGoto{{5, GotoAction{State: 2}}}, true},
		{"non-terminals of one state only", nil, nil, []indexedGoto{{5, GotoAction{State: 2}}}, []indexedGoto{{6, GotoAction{State: 4}}}, false},
	}
	for _, c := range cases {
		got := m.stateSuccessorsDiffer(a, b, groupOf, [][]keyedShift{c.shifts1, c.shifts2}, [][]indexedGoto{c.gotos1, c.gotos2})
		if got != c.want {
			t.Errorf("%s: %v, want %v", c.name, got, c.want)
		}
	}
}

// buildMergedText prepares a grammar.json text and builds its tables with
// and without the merge.
func buildMergedText(t *testing.T, text string) (*PreparedGrammar, *Tables, *Tables) {
	t.Helper()
	g, err := Prepare([]byte(text))
	if err != nil {
		t.Fatalf("prepare: %v", err)
	}
	info, err := GetVariableInfo(g.Syntax, g.Lex, g.DefaultAliases)
	if err != nil {
		t.Fatalf("variable info: %v", err)
	}
	merged, err := BuildTables(g, info, OptMergeStates)
	if err != nil {
		t.Fatalf("merged tables: %v", err)
	}
	unmerged, err := BuildTables(g, info, 0)
	if err != nil {
		t.Fatalf("unmerged tables: %v", err)
	}
	return g, merged, unmerged
}

// reduceOn returns the variable that a state reduces on a token, or "".
func reduceOn(g *PreparedGrammar, table *InternedTable, state *InternedState, token string) string {
	for _, e := range state.TerminalEntries {
		if !e.Symbol.IsTerminal() || g.Lex.Variables[e.Symbol.Index].Ident != token {
			continue
		}
		for _, a := range table.ActionLists.Get(e.ID) {
			if a.Kind == ActionReduce {
				return g.Syntax.Variables[a.Symbol.Index].Ident
			}
		}
	}
	return ""
}

// TestMergeKeepsLR1Split: in the grammar of the classic example that LALR(1)
// cannot parse, the two states after "a c" and "b c" share a core but reduce
// different rules on the same token. entries_conflict keeps them apart, so
// the merged table still has one state that reduces x on "d" and y on "e",
// and one that does the reverse. Expressions in parentheses, nested at two
// depths, give states that do merge.
func TestMergeKeepsLR1Split(t *testing.T) {
	g := tGrammar{rules: [][2]string{
		{"start", jChoice(
			jSeq(jStr("a"), jSym("x"), jStr("d")),
			jSeq(jStr("b"), jSym("y"), jStr("d")),
			jSeq(jStr("a"), jSym("y"), jStr("e")),
			jSeq(jStr("b"), jSym("x"), jStr("e")),
			jSeq(jStr("f"), jSym("expr")),
		)},
		{"x", jStr("c")},
		{"y", jStr("c")},
		{"expr", jChoice(jStr("n"), jSeq(jStr("("), jSym("expr"), jStr(")")))},
	}}
	pg, merged, unmerged := buildMergedText(t, g.text())
	s := merged.Stats
	if s.StatesAfterMerge >= s.RawStates || s.StatesAfterMinimizing >= unmerged.Stats.StatesAfterMinimizing {
		t.Fatalf("states: raw %d, after merge %d, final %d (unmerged final %d)", s.RawStates, s.StatesAfterMerge, s.StatesAfterMinimizing, unmerged.Stats.StatesAfterMinimizing)
	}
	if s.GroupsAfterConflicts <= s.CoreGroups {
		t.Errorf("states_conflict split no group: %d core groups, %d after", s.CoreGroups, s.GroupsAfterConflicts)
	}
	table := merged.ParseTable
	found := map[string]bool{}
	for i := range table.States {
		d, e := reduceOn(pg, table, &table.States[i], "d"), reduceOn(pg, table, &table.States[i], "e")
		if d != "" || e != "" {
			found[d+"/"+e] = true
		}
	}
	if !found["x/y"] || !found["y/x"] || len(found) != 2 {
		t.Fatalf("states that reduce on d/e: %v, want x/y and y/x", found)
	}
	t.Logf("states: raw %d, core groups %d, after states_conflict %d, successor calls %d, after merge %d, final %d (unmerged %d)",
		s.RawStates, s.CoreGroups, s.GroupsAfterConflicts, s.SuccessorPasses, s.StatesAfterMerge, s.StatesAfterMinimizing, unmerged.Stats.StatesAfterMinimizing)
}

// TestMergeExtendsEntries: the merged state keeps the first member's entries
// in place, takes the later member's value for a key that both have
// (IndexMap::extend), appends the keys it lacks, joins the reserved words
// less the merged terminals, ORs the end-of-input-gated flag, and every
// Shift and Goto names a group.
func TestMergeExtendsEntries(t *testing.T) {
	t0, t1, t2 := lexbuild.TerminalSym(0), lexbuild.TerminalSym(1), lexbuild.TerminalSym(2)
	n0, n1 := lexbuild.NonTerminalSym(0), lexbuild.NonTerminalSym(1)
	table := &InternedTable{}
	pool := &table.ActionLists
	toState := func(s uint32) ActionListID { return newActionListID(pool.push([]ParseAction{shift(s)}), true) }
	// States 2 and 3 share core 2; state 3's entry for t0 is another pool list
	// with an equal action, and it is not reusable.
	shared := toState(1)
	other := newActionListID(pool.push([]ParseAction{shift(1)}), false)
	table.States = []InternedState{
		{ID: 0, CoreID: 0},
		{ID: 1, CoreID: 1, NonterminalEntries: []NonterminalEntry{{n0, GotoAction{State: 3}}}},
		{ID: 2, CoreID: 2, TerminalEntries: []TerminalActionList{{t0, shared}},
			NonterminalEntries: []NonterminalEntry{{n0, GotoAction{State: 2}}}},
		{ID: 3, CoreID: 2, HasEOFGatedReduce: true, TerminalEntries: []TerminalActionList{{t1, toState(3)}, {t0, other}},
			NonterminalEntries: []NonterminalEntry{{n1, GotoAction{ShiftExtra: true}}, {n0, GotoAction{State: 3}}}},
	}
	table.States[2].ReservedWords.Insert(t1)
	table.States[3].ReservedWords.Insert(t2)
	sg := &lexbuild.SyntaxGrammar{Variables: make([]lexbuild.SyntaxVariable, 2)}
	lg := &lexbuild.LexicalGrammar{Variables: make([]lexbuild.LexicalVariable, 3)}
	m := &minimizer{parseTable: table, syntaxGrammar: sg, lexicalGrammar: lg, stats: &TableStats{}}
	// No terminal conflicts: the token conflict map is not read when there
	// are no conflicts to test, but conflict rows are built for every
	// terminal pair, so give the minimizer an empty analysis through a
	// grammar with three tokens.
	m.tokenConflictMap = analysisWithTokens(t, 3)
	err := catchGeneratorPanic(func() error { m.mergeCompatibleStates(); return nil })
	if err != nil {
		t.Fatal(err)
	}
	if len(table.States) != 3 {
		t.Fatalf("%d states, want 3", len(table.States))
	}
	s := &table.States[2]
	if s.ID != 2 || s.CoreID != 2 || !s.HasEOFGatedReduce {
		t.Errorf("merged state: id %d, core %d, end-of-input-gated %v", s.ID, s.CoreID, s.HasEOFGatedReduce)
	}
	if got, want := fmt.Sprint(s.TerminalEntries), fmt.Sprint([]TerminalActionList{{t0, other}, {t1, newActionListID(2, true)}}); got != want {
		t.Errorf("terminal entries %s, want %s", got, want)
	}
	if got, want := fmt.Sprint(s.NonterminalEntries), fmt.Sprint([]NonterminalEntry{{n0, GotoAction{State: 2}}, {n1, GotoAction{ShiftExtra: true}}}); got != want {
		t.Errorf("non-terminal entries %s, want %s", got, want)
	}
	if got := fmt.Sprint(s.ReservedWords.Symbols()); got != fmt.Sprint([]Symbol{t2}) {
		t.Errorf("reserved words %s, want [t2]", got)
	}
	if g := table.States[1].NonterminalEntries[0].Action; g.State != 2 {
		t.Errorf("state 1 goes to %d, want group 2", g.State)
	}
	for _, e := range s.TerminalEntries {
		if a := table.ActionLists.Get(e.ID)[0]; a.State > 2 {
			t.Errorf("entry %v shifts to %d, not a group", e.Symbol, a.State)
		}
	}
}

// analysisWithTokens returns the lexical analysis of a grammar with n string
// tokens that do not conflict with one another.
func analysisWithTokens(t *testing.T, n int) *lexbuild.Analysis {
	t.Helper()
	var members []string
	for i := 0; i < n; i++ {
		members = append(members, jStr(fmt.Sprintf("k%c", 'a'+i)))
	}
	g := tGrammar{rules: [][2]string{{"start", jRepeat0(jChoice(members...))}}}
	raw, err := buildText(t, g.text())
	if err != nil {
		t.Fatal(err)
	}
	return raw.Analysis
}

// TestMergeErrors: a table without states, or without a start state, stops
// with the generator's panic.
func TestMergeErrors(t *testing.T) {
	for _, states := range [][]InternedState{nil, {{ID: 0}}} {
		m := &minimizer{
			parseTable:       &InternedTable{States: states},
			syntaxGrammar:    &lexbuild.SyntaxGrammar{},
			lexicalGrammar:   &lexbuild.LexicalGrammar{},
			tokenConflictMap: analysisWithTokens(t, 1),
			stats:            &TableStats{},
		}
		err := catchGeneratorPanic(func() error { m.mergeCompatibleStates(); return nil })
		var p *GeneratorPanicError
		if !errors.As(err, &p) {
			t.Errorf("%d states: %v, want a generator panic", len(states), err)
		}
	}
}

// TestMergeBudget lowers the merge budget: BuildTables with OptMergeStates
// stops with a *lexbuild.SizeError, and the unmerged build does not count.
func TestMergeBudget(t *testing.T) {
	old := bMergeWork.limit
	bMergeWork.limit = 10
	defer func() { bMergeWork.limit = old }()
	g, err := Prepare(readTestdataGrammar(t, "json"))
	if err != nil {
		t.Fatal(err)
	}
	info, err := GetVariableInfo(g.Syntax, g.Lex, g.DefaultAliases)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := BuildTables(g, info, 0); err != nil {
		t.Fatalf("unmerged build: %v", err)
	}
	_, err = BuildTables(g, info, OptMergeStates)
	var se *lexbuild.SizeError
	if !errors.As(err, &se) || se.What != bMergeWork.what {
		t.Fatalf("want a size error for %q, got %v", bMergeWork.what, err)
	}
}
