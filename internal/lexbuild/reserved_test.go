package lexbuild

import (
	"reflect"
	"testing"
)

// Fixtures for reserved words, written by hand from the generator's output. The two grammars were
// run through tree-sitter 0.27.1; the reference rows are given in each comment (in parser.c the
// table ts_reserved_words has one row per distinct set of a parse state, row 0 empty). The port
// carries the grammar's own sets in declaration order ("global" first, id 0) and the id of the
// innermost reserved(...) on each step.

func setNames(p *Prepared, s *TokenSet) []string {
	var out []string
	for _, sy := range s.symbols() {
		out = append(out, p.Lex.Variables[sy.Index].Ident)
	}
	return out
}

func stepsOf(p *Prepared, prod Production) (names []string, reserved []uint16) {
	for _, st := range prod.Steps {
		if st.Symbol.IsTerminal() {
			names = append(names, p.Lex.Variables[st.Symbol.Index].Ident)
		} else {
			names = append(names, "$"+p.Syntax.Variables[st.Symbol.Index].Ident)
		}
		reserved = append(reserved, st.Reserved)
	}
	return
}

// Reference: ts_reserved_words rows [1] = {if} and [2] = {if, else}; the parse state after "if"
// has reserved_word_set_id 1 (the set "kw", declared second) and the state after "else" has
// id 2 (the set "global").
func TestReservedFixtureOne(t *testing.T) {
	p, err := Prepare([]byte(`{"name":"t","word":"identifier",
 "reserved":{"global":[{"type":"STRING","value":"if"},{"type":"STRING","value":"else"}],"kw":[{"type":"STRING","value":"if"}]},
 "rules":{
  "source":{"type":"REPEAT1","content":{"type":"SYMBOL","name":"stmt"}},
  "stmt":{"type":"CHOICE","members":[
     {"type":"SEQ","members":[{"type":"STRING","value":"if"},{"type":"RESERVED","context_name":"kw","content":{"type":"SYMBOL","name":"identifier"}}]},
     {"type":"SEQ","members":[{"type":"STRING","value":"else"},{"type":"SYMBOL","name":"identifier"}]}]},
  "identifier":{"type":"PATTERN","value":"[a-z]+"}}}`))
	if err != nil {
		t.Fatal(err)
	}
	if len(p.Syntax.ReservedWordSets) != 2 {
		t.Fatalf("%d sets", len(p.Syntax.ReservedWordSets))
	}
	if got := setNames(p, &p.Syntax.ReservedWordSets[0]); !reflect.DeepEqual(got, []string{"if", "else"}) && !reflect.DeepEqual(got, []string{"else", "if"}) {
		t.Errorf("set 0 (global): %v", got)
	}
	if got := setNames(p, &p.Syntax.ReservedWordSets[1]); !reflect.DeepEqual(got, []string{"if"}) {
		t.Errorf("set 1 (kw): %v", got)
	}
	var found [][2]interface{}
	for i, v := range p.Syntax.Variables {
		if v.Ident != "stmt" {
			continue
		}
		vp := p.Syntax.VarProds[i]
		for _, prod := range p.Syntax.Productions[vp[0]:vp[1]] {
			n, r := stepsOf(p, prod)
			found = append(found, [2]interface{}{n, r})
		}
	}
	want := [][2]interface{}{
		{[]string{"if", "identifier"}, []uint16{0, 1}},
		{[]string{"else", "identifier"}, []uint16{0, 0}},
	}
	if !reflect.DeepEqual(found, want) {
		t.Errorf("stmt productions: %v, want %v", found, want)
	}
}

// Reference: rows [1] = {if} (state 3, after "let": the identifier is read under "kw") and
// [2] = {if, else} (state 5, after the first identifier: the second one is read under "global"). The
// inner reserved(...) wins inside the outer one, and a step outside both has id 0.
func TestReservedFixtureTwoNested(t *testing.T) {
	p, err := Prepare([]byte(`{"name":"t","word":"identifier",
 "reserved":{"global":[{"type":"STRING","value":"if"},{"type":"STRING","value":"else"}],"kw":[{"type":"STRING","value":"if"}]},
 "rules":{
  "source":{"type":"SEQ","members":[{"type":"STRING","value":"let"},
    {"type":"RESERVED","context_name":"kw","content":{"type":"SEQ","members":[{"type":"SYMBOL","name":"identifier"},
       {"type":"RESERVED","context_name":"global","content":{"type":"SYMBOL","name":"identifier"}}]}},
    {"type":"CHOICE","members":[{"type":"STRING","value":"if"},{"type":"STRING","value":"else"}]}]},
  "identifier":{"type":"PATTERN","value":"[a-z]+"}}}`))
	if err != nil {
		t.Fatal(err)
	}
	var found [][2]interface{}
	vp := p.Syntax.VarProds[0]
	for _, prod := range p.Syntax.Productions[vp[0]:vp[1]] {
		n, r := stepsOf(p, prod)
		found = append(found, [2]interface{}{n, r})
	}
	want := [][2]interface{}{
		{[]string{"let", "identifier", "identifier", "if"}, []uint16{0, 1, 0, 0}},
		{[]string{"let", "identifier", "identifier", "else"}, []uint16{0, 1, 0, 0}},
	}
	if !reflect.DeepEqual(found, want) {
		t.Errorf("source productions: %v, want %v", found, want)
	}
}
