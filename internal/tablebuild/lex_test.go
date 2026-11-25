package tablebuild

import (
	"errors"
	"fmt"
	"testing"

	"github.com/odvcencio/gotreesitter/internal/lexbuild"
)

// TestLexTokens: the set that build_lex_table reads from a parse state is the
// keys of its terminal entries chained with its reserved words; lexbuild
// filters it.
func TestLexTokens(t *testing.T) {
	t0, t1, t3 := lexbuild.TerminalSym(0), lexbuild.TerminalSym(1), lexbuild.TerminalSym(3)
	e0 := lexbuild.ExternalSym(0)
	state := &InternedState{TerminalEntries: []TerminalActionList{{t1, 0}, {lexbuild.EndSym(), 0}, {e0, 0}, {t0, 0}}}
	state.ReservedWords.Insert(t3)
	state.ReservedWords.Insert(t1)
	got := lexTokens(state)
	want := []Symbol{t0, t1, t3, e0, lexbuild.EndSym()}
	if fmt.Sprint(got.Symbols()) != fmt.Sprint(want) {
		t.Errorf("tokens %v, want %v", got.Symbols(), want)
	}
}

// TestPopulateExternalLexStates: row 0 is the empty set; each state gets the
// first equal set; a terminal stands for the external token that corresponds
// to it, the last one when two do.
func TestPopulateExternalLexStates(t *testing.T) {
	t0, t1 := lexbuild.TerminalSym(0), lexbuild.TerminalSym(1)
	e0, e1, e2 := lexbuild.ExternalSym(0), lexbuild.ExternalSym(1), lexbuild.ExternalSym(2)
	sg := &lexbuild.SyntaxGrammar{ExternalTokens: []lexbuild.ExternalToken{
		{Ident: "a"},
		{Ident: "b", CorrespondingInternal: t1, HasCorresponding: true},
		{Ident: "c", CorrespondingInternal: t1, HasCorresponding: true},
	}}
	entries := func(syms ...Symbol) []TerminalActionList {
		var out []TerminalActionList
		for _, s := range syms {
			out = append(out, TerminalActionList{s, 0})
		}
		return out
	}
	table := &InternedTable{States: []InternedState{
		{TerminalEntries: entries(t0)},                        // no external: row 0
		{TerminalEntries: entries(e0, t0)},                    // {a}: row 1
		{TerminalEntries: entries(t1)},                        // t1 stands for c (the last): row 2
		{TerminalEntries: entries(t0, e0)},                    // {a} again: row 1
		{TerminalEntries: entries(e2, e1, lexbuild.EndSym())}, // {b, c}: row 3
		{TerminalEntries: entries(e2)},                        // {c}: row 2
	}}
	populateExternalLexStates(table, sg)
	var ids []uint32
	for _, s := range table.States {
		ids = append(ids, s.ExternalLexStateID)
	}
	if fmt.Sprint(ids) != fmt.Sprint([]uint32{0, 1, 2, 1, 3, 2}) {
		t.Errorf("external lex state ids %v", ids)
	}
	var rows []string
	for _, set := range table.ExternalLexStates {
		rows = append(rows, fmt.Sprint(set.Symbols()))
	}
	want := []string{fmt.Sprint([]Symbol{}), fmt.Sprint([]Symbol{e0}), fmt.Sprint([]Symbol{e2}), fmt.Sprint([]Symbol{e1, e2})}
	if fmt.Sprint(rows) != fmt.Sprint(want) {
		t.Errorf("external lex states %v, want %v", rows, want)
	}
}

// reservedGrammar has a word token, a keyword, and a reserved word that is
// not a keyword ("!"), so the reserved word stays a token of its own in the
// sets of build_lex_table. A label token makes some token sets incompatible,
// so they do not all merge into one group.
const reservedGrammar = `{"name":"t","word":"identifier",
 "reserved":{"global":[{"type":"STRING","value":"!"}]},
 "rules":{
  "source":{"type":"REPEAT1","content":{"type":"SYMBOL","name":"stmt"}},
  "stmt":{"type":"CHOICE","members":[
    {"type":"SEQ","members":[{"type":"STRING","value":"let"},{"type":"SYMBOL","name":"identifier"}]},
    {"type":"SEQ","members":[{"type":"SYMBOL","name":"label"},{"type":"STRING","value":"!"}]}]},
  "label":{"type":"PATTERN","value":"[a-z]+:"},
  "identifier":{"type":"PATTERN","value":"[a-z]+"}},
 "extras":[{"type":"PATTERN","value":"\\s"}]}`

// TestBuildLexTableReservedWords: a parse state whose reserved words hold a
// token that none of its terminal entries holds gets a lex state that lexes
// that token, because build_lex_table chains the reserved words to the
// terminal entries. Without the chain, parse state 5 gets a lex state with no
// step on '!'.
func TestBuildLexTableReservedWords(t *testing.T) {
	g, err := Prepare([]byte(reservedGrammar))
	if err != nil {
		t.Fatal(err)
	}
	info, err := GetVariableInfo(g.Syntax, g.Lex, g.DefaultAliases)
	if err != nil {
		t.Fatal(err)
	}
	tables, err := BuildTables(g, info, OptMergeStates)
	if err != nil {
		t.Fatal(err)
	}
	bang := -1
	for i, v := range g.Lex.Variables {
		if v.Ident == "!" {
			bang = i
		}
	}
	if bang < 0 {
		t.Fatal("no token !")
	}
	bangSym := lexbuild.TerminalSym(bang)
	main := tables.LexTables.Main
	checked := 0
	for i := range tables.ParseTable.States {
		s := &tables.ParseTable.States[i]
		if !s.ReservedWords.Contains(bangSym) {
			continue
		}
		valid := false
		for _, e := range s.TerminalEntries {
			valid = valid || e.Symbol == bangSym
		}
		if valid {
			continue
		}
		checked++
		target, _, ok := main.Step(int(s.LexStateID), false, '!')
		if !ok {
			t.Errorf("parse state %d: lex state %d has no step on '!'", i, s.LexStateID)
			continue
		}
		if acc, ok := main.Accept(target); !ok || acc != bangSym {
			t.Errorf("parse state %d: '!' leads to lex state %d, which accepts %v (%v), want %v", i, target, acc, ok, bangSym)
		}
	}
	if checked == 0 {
		t.Fatal("no parse state holds '!' only as a reserved word")
	}
	t.Logf("%d parse states hold '!' only as a reserved word; token sets %d, groups %d, lex states %d", checked, tables.Stats.LexTokenSets, tables.Stats.LexGroups, tables.Stats.MainLexStates)
}

// TestMarkFragileTokensBudget: the fragile marking stops with a size error
// before it passes its limit.
func TestMarkFragileTokensBudget(t *testing.T) {
	g, err := Prepare([]byte(reservedGrammar))
	if err != nil {
		t.Fatal(err)
	}
	info, err := GetVariableInfo(g.Syntax, g.Lex, g.DefaultAliases)
	if err != nil {
		t.Fatal(err)
	}
	old := bFragileWork.limit
	bFragileWork.limit = 1
	defer func() { bFragileWork.limit = old }()
	_, err = BuildTables(g, info, OptMergeStates)
	var se *lexbuild.SizeError
	if !errors.As(err, &se) || se.What != bFragileWork.what {
		t.Fatalf("err %v, want the size error of %q", err, bFragileWork.what)
	}
}
