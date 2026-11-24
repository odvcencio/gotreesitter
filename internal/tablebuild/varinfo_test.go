package tablebuild

import (
	"fmt"
	"reflect"
	"testing"

	"github.com/odvcencio/gotreesitter/internal/lexbuild"
)

// The tests in this file port the get_variable_info tests of the
// #[cfg(test)] module of node_types.rs (v0.27.1). The tests of that module
// that check generate_node_types are ported in nodetypes_test.go, through the
// node records of the L1 comparison.

// buildLexicalGrammar is the build_lexical_grammar helper: ten named tokens.
func buildLexicalGrammar() *lexbuild.LexicalGrammar {
	lg := &lexbuild.LexicalGrammar{}
	for i := 0; i < 10; i++ {
		lg.Variables = append(lg.Variables, lexbuild.LexicalVariable{Ident: fmt.Sprintf("token_%d", i), Kind: lexbuild.VarNamed})
	}
	return lg
}

type varSpec struct {
	name  string
	kind  lexbuild.VariableType
	prods [][]lexbuild.ProductionStep
}

// buildSyntaxGrammar is the build_syntax_grammar helper.
func buildSyntaxGrammar(vars []varSpec, supertypes []Symbol) *lexbuild.SyntaxGrammar {
	sg := &lexbuild.SyntaxGrammar{SupertypeSymbols: supertypes}
	for _, v := range vars {
		start := len(sg.Productions)
		for _, steps := range v.prods {
			sg.Productions = append(sg.Productions, lexbuild.Production{Steps: steps})
		}
		sg.VarProds = append(sg.VarProds, [2]int{start, len(sg.Productions)})
		sg.Variables = append(sg.Variables, lexbuild.SyntaxVariable{Ident: v.name, Kind: v.kind})
	}
	return sg
}

// step is the step helper: a symbol with an optional field and the
// "no reserved words" sentinel.
func step(sym Symbol, field string) lexbuild.ProductionStep {
	st := lexbuild.ProductionStep{Symbol: sym, Reserved: 0xFFFF}
	if field != "" {
		st.Field, st.HasField = field, true
	}
	return st
}

func normal(s Symbol) ChildType { return ChildType{Symbol: s} }

func wantFields(t *testing.T, label string, got map[string]*FieldInfo, want map[string]FieldInfo) {
	t.Helper()
	g := map[string]FieldInfo{}
	for k, v := range got {
		g[k] = *v
	}
	if !reflect.DeepEqual(g, want) {
		t.Errorf("%s fields:\n got %+v\nwant %+v", label, g, want)
	}
}

// test_get_variable_info
func TestGetVariableInfo(t *testing.T) {
	lg := buildLexicalGrammar()
	sg := buildSyntaxGrammar([]varSpec{
		// Required field `field1` has only one node type.
		{"rule0", lexbuild.VarNamed, [][]lexbuild.ProductionStep{{step(term(0), ""), step(nonTerm(1), "field1")}}},
		// Hidden node.
		{"_rule1", lexbuild.VarHidden, [][]lexbuild.ProductionStep{{step(term(1), "")}}},
		// Optional field `field2` can have two possible node types.
		{"rule2", lexbuild.VarNamed, [][]lexbuild.ProductionStep{
			{step(term(0), "")},
			{step(term(0), ""), step(term(2), "field2")},
			{step(term(0), ""), step(term(3), "field2")},
		}},
	}, nil)
	info, err := GetVariableInfo(sg, lg, AliasMap{})
	if err != nil {
		t.Fatal(err)
	}
	wantFields(t, "rule0", info[0].Fields, map[string]FieldInfo{
		"field1": {ChildQuantity{Exists: true, Required: true}, []ChildType{normal(term(1))}},
	})
	wantFields(t, "rule2", info[2].Fields, map[string]FieldInfo{
		"field2": {ChildQuantity{Exists: true}, []ChildType{normal(term(2)), normal(term(3))}},
	})
}

// test_get_variable_info_with_repetitions_inside_fields
func TestGetVariableInfoWithRepetitionsInsideFields(t *testing.T) {
	lg := buildLexicalGrammar()
	sg := buildSyntaxGrammar([]varSpec{
		// A field associated with a repetition.
		{"rule0", lexbuild.VarNamed, [][]lexbuild.ProductionStep{{step(nonTerm(1), "field1")}, {}}},
		{"_rule0_repeat", lexbuild.VarHidden, [][]lexbuild.ProductionStep{
			{step(term(1), "")},
			{step(nonTerm(1), ""), step(nonTerm(1), "")},
		}},
	}, nil)
	info, err := GetVariableInfo(sg, lg, AliasMap{})
	if err != nil {
		t.Fatal(err)
	}
	wantFields(t, "rule0", info[0].Fields, map[string]FieldInfo{
		"field1": {ChildQuantity{Exists: true, Multiple: true}, []ChildType{normal(term(1))}},
	})
}

// test_get_variable_info_with_inherited_fields
func TestGetVariableInfoWithInheritedFields(t *testing.T) {
	lg := buildLexicalGrammar()
	dot := lexbuild.ProductionStep{Symbol: term(2), Alias: Alias{Set: true, Value: ".", Named: false}, Reserved: 0xFFFF}
	sg := buildSyntaxGrammar([]varSpec{
		{"rule0", lexbuild.VarNamed, [][]lexbuild.ProductionStep{
			{step(term(0), ""), step(nonTerm(1), ""), step(term(1), "")},
			{step(nonTerm(1), "")},
		}},
		// Hidden node with fields.
		{"_rule1", lexbuild.VarHidden, [][]lexbuild.ProductionStep{{dot, step(term(3), "field1")}}},
	}, nil)
	info, err := GetVariableInfo(sg, lg, AliasMap{})
	if err != nil {
		t.Fatal(err)
	}
	wantFields(t, "rule0", info[0].Fields, map[string]FieldInfo{
		"field1": {ChildQuantity{Exists: true, Required: true}, []ChildType{normal(term(3))}},
	})
	want := FieldInfo{ChildQuantity{Exists: true, Multiple: true}, []ChildType{normal(term(0)), normal(term(1))}}
	if !reflect.DeepEqual(info[0].ChildrenWithoutFields, want) {
		t.Errorf("children without fields:\n got %+v\nwant %+v", info[0].ChildrenWithoutFields, want)
	}
}

// test_get_variable_info_with_supertypes
func TestGetVariableInfoWithSupertypes(t *testing.T) {
	lg := buildLexicalGrammar()
	sg := buildSyntaxGrammar([]varSpec{
		{"rule0", lexbuild.VarNamed, [][]lexbuild.ProductionStep{{step(term(0), ""), step(nonTerm(1), "field1"), step(term(1), "")}}},
		{"_rule1", lexbuild.VarHidden, [][]lexbuild.ProductionStep{{step(term(2), "")}, {step(term(3), "")}}},
	}, []Symbol{nonTerm(1)}) // _rule1 is a supertype
	info, err := GetVariableInfo(sg, lg, AliasMap{})
	if err != nil {
		t.Fatal(err)
	}
	wantFields(t, "rule0", info[0].Fields, map[string]FieldInfo{
		"field1": {ChildQuantity{Exists: true, Required: true}, []ChildType{normal(nonTerm(1))}},
	})
}

// test_supertype_and_inline_conflict: a rule that is both a supertype and
// inlined loses its supertype (intern_symbols), so the supertype map has no
// entry for it, and the field that holds it lists the inlined rule's
// children. The Rust test asserts on the node types; this one asserts on the
// variable info and the supertype map that the node types are built from.
func TestSupertypeAndInlineConflict(t *testing.T) {
	grammar := `{"name":"g","rules":{
		"v1":{"type":"FIELD","name":"f1","content":{"type":"SYMBOL","name":"_v2"}},
		"_v2":{"type":"CHOICE","members":[{"type":"SYMBOL","name":"v3"},{"type":"SYMBOL","name":"v4"}]},
		"v3":{"type":"STRING","value":"x"},
		"v4":{"type":"STRING","value":"y"}},
		"supertypes":["_v2"],"inline":["_v2"]}`
	g, err := Prepare([]byte(grammar))
	if err != nil {
		t.Fatal(err)
	}
	info, err := GetVariableInfo(g.Syntax, g.Lex, g.DefaultAliases)
	if err != nil {
		t.Fatal(err)
	}
	if m := GetSupertypeSymbolMap(g.Syntax, g.DefaultAliases, info); len(m) != 0 {
		t.Fatalf("supertype map: %+v", m)
	}
	f := info[0].Fields["f1"]
	if f == nil {
		t.Fatal("v1 has no field f1")
	}
	var names []string
	for _, ct := range f.Types {
		if ct.Aliased || !ct.Symbol.IsTerminal() {
			t.Fatalf("field f1 holds %+v", ct)
		}
		names = append(names, g.Lex.Variables[ct.Symbol.Index].Ident)
	}
	if !reflect.DeepEqual(names, []string{"v3", "v4"}) {
		t.Fatalf("field f1 types: %v", names)
	}
}

// TestSupertypeAliasCollision: validate_supertype_aliases refuses a named alias
// with the name of a supertype.
func TestSupertypeAliasCollision(t *testing.T) {
	grammar := `{"name":"g","rules":{
		"a":{"type":"SEQ","members":[{"type":"SYMBOL","name":"_s"},{"type":"ALIAS","content":{"type":"STRING","value":"z"},"named":true,"value":"_s"}]},
		"_s":{"type":"CHOICE","members":[{"type":"SYMBOL","name":"b"},{"type":"SYMBOL","name":"c"}]},
		"b":{"type":"STRING","value":"x"},
		"c":{"type":"STRING","value":"y"}},
		"supertypes":["_s"]}`
	g, err := Prepare([]byte(grammar))
	if err != nil {
		t.Fatal(err)
	}
	_, err = GetVariableInfo(g.Syntax, g.Lex, g.DefaultAliases)
	if err == nil || err.Error() != "Named alias `_s` conflicts with a supertype of the same name." {
		t.Fatalf("got %v", err)
	}
}

// TestInvalidSupertype: validate_supertype_structure refuses a supertype with
// a multi-step production, and names the hidden child that causes it.
func TestInvalidSupertype(t *testing.T) {
	grammar := `{"name":"g","rules":{
		"a":{"type":"SYMBOL","name":"_s"},
		"_s":{"type":"CHOICE","members":[{"type":"SYMBOL","name":"b"},{"type":"SYMBOL","name":"_h"}]},
		"_h":{"type":"SEQ","members":[{"type":"SYMBOL","name":"b"},{"type":"SYMBOL","name":"b"}]},
		"b":{"type":"STRING","value":"x"}},
		"supertypes":["_s"]}`
	g, err := Prepare([]byte(grammar))
	if err != nil {
		t.Fatal(err)
	}
	_, err = GetVariableInfo(g.Syntax, g.Lex, g.DefaultAliases)
	want := "Supertypes must have a single visible child, but `_s` can have multiple. The hidden child `_h` can expand into multiple nodes. Consider making `_h` visible."
	if err == nil || err.Error() != want {
		t.Fatalf("got %v", err)
	}
}
