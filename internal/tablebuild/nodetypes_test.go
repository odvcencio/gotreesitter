package tablebuild

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"testing"
)

// The tests in this file port the generate_node_types tests of the
// #[cfg(test)] module of node_types.rs (v0.27.1). Each Rust test builds an
// InputGrammar in a rule pool; here the same grammar is written as
// grammar.json and prepared by Prepare. The assertions are made on the node
// records of the L1 comparison (fields, children and subtypes of each node
// type). The test-side generateNodeTypes does not compute the root and extra
// flags or the order of the entries, so the Rust assertions on those are not
// ported; each case says where it drops one.

// Rule builders that write grammar.json rules, named after the Rust helpers.
func jSym(name string) string { return fmt.Sprintf(`{"type":"SYMBOL","name":%q}`, name) }
func jStr(v string) string    { return fmt.Sprintf(`{"type":"STRING","value":%q}`, v) }
func jPat(v string) string    { return fmt.Sprintf(`{"type":"PATTERN","value":%q}`, v) }
func jBlank() string          { return `{"type":"BLANK"}` }
func jSeq(m ...string) string { return `{"type":"SEQ","members":[` + strings.Join(m, ",") + `]}` }
func jChoice(m ...string) string {
	return `{"type":"CHOICE","members":[` + strings.Join(m, ",") + `]}`
}
func jField(name, content string) string {
	return fmt.Sprintf(`{"type":"FIELD","name":%q,"content":%s}`, name, content)
}
func jAlias(content, value string, named bool) string {
	return fmt.Sprintf(`{"type":"ALIAS","content":%s,"named":%t,"value":%q}`, content, named, value)
}

// jRepeat is RulePool::repeat, the one-or-more repetition (REPEAT1 in grammar.json).
func jRepeat(content string) string { return `{"type":"REPEAT1","content":` + content + `}` }

type jGrammar struct {
	rules      [][2]string // name, rule, in order
	extras     []string
	externals  []string
	supertypes []string
	inline     []string
}

func (g jGrammar) text() string {
	var rules []string
	for _, r := range g.rules {
		rules = append(rules, fmt.Sprintf("%q:%s", r[0], r[1]))
	}
	list := func(xs []string) string { return "[" + strings.Join(xs, ",") + "]" }
	names := func(xs []string) string {
		b, _ := json.Marshal(xs)
		if xs == nil {
			return "[]"
		}
		return string(b)
	}
	return `{"name":"t","rules":{` + strings.Join(rules, ",") + `},"extras":` + list(g.extras) +
		`,"externals":` + list(g.externals) + `,"supertypes":` + names(g.supertypes) + `,"inline":` + names(g.inline) + `}`
}

// nodeRecordMap prepares the grammar and returns its node records by key.
func nodeRecordMap(t *testing.T, g jGrammar) (map[string]string, int) {
	t.Helper()
	port, err := newL1Port([]byte(g.text()))
	if err != nil {
		t.Fatalf("prepare: %v", err)
	}
	nodes, err := port.generateNodeTypes()
	if err != nil {
		t.Fatalf("generate node types: %v", err)
	}
	m := map[string]string{}
	nodeRecords(nodes, func(k, v string) {
		if _, dup := m[k]; dup {
			t.Fatalf("record %q twice", k)
		}
		m[k] = v
	})
	return m, len(nodes)
}

// wantEntry checks every record of one node entry: kind is regular, leaf or
// supertype, and parts maps "children", "subtypes" and "field <name>" to values.
func wantEntry(t *testing.T, recs map[string]string, ref, kind string, parts map[string]string) {
	t.Helper()
	key := "node " + ref
	if got, ok := recs[key]; !ok || got != kind {
		t.Errorf("%s: got %q (present %t), want %q", key, got, ok, kind)
	}
	for k, v := range recs {
		if !strings.HasPrefix(k, key+" ") {
			continue
		}
		part := strings.TrimPrefix(k, key+" ")
		if want, ok := parts[part]; !ok {
			t.Errorf("%s: unexpected %s = %s", key, part, v)
		} else if v != want {
			t.Errorf("%s %s:\n got %s\nwant %s", key, part, v, want)
		}
	}
	for part := range parts {
		if _, ok := recs[key+" "+part]; !ok {
			t.Errorf("%s: no %s", key, part)
		}
	}
}

func wantNodes(t *testing.T, recs map[string]string, refs ...string) {
	t.Helper()
	var got []string
	for k := range recs {
		if r, ok := strings.CutPrefix(k, "node "); ok && !strings.Contains(r, `/named `) && !strings.Contains(r, `/anon `) {
			got = append(got, r)
		}
	}
	sort.Strings(got)
	want := append([]string(nil), refs...)
	sort.Strings(want)
	if strings.Join(got, ", ") != strings.Join(want, ", ") {
		t.Errorf("node types:\n got %s\nwant %s", strings.Join(got, ", "), strings.Join(want, ", "))
	}
}

func fv(multiple, required bool, types ...string) string {
	return fmt.Sprintf("multiple=%t required=%t types=[%s]", multiple, required, strings.Join(types, " "))
}

func named(n string) string { return fmt.Sprintf("%q/named", n) }
func anon(n string) string  { return fmt.Sprintf("%q/anon", n) }

// test_node_types_simple (root flag not checked)
func TestNodeTypesSimple(t *testing.T) {
	recs, _ := nodeRecordMap(t, jGrammar{rules: [][2]string{
		{"v1", jSeq(jField("f1", jSym("v2")), jField("f2", jStr(";")))},
		{"v2", jStr("x")},
		// Not reachable from the start symbol, so absent from the node types.
		{"v3", jStr("y")},
	}})
	wantNodes(t, recs, named("v1"), anon(";"), named("v2"))
	wantEntry(t, recs, named("v1"), "regular", map[string]string{
		"field f1": fv(false, true, named("v2")),
		"field f2": fv(false, true, anon(";")),
	})
	wantEntry(t, recs, anon(";"), "leaf", nil)
	wantEntry(t, recs, named("v2"), "leaf", nil)
}

// test_node_types_simple_extras (root and extra flags not checked)
func TestNodeTypesSimpleExtras(t *testing.T) {
	recs, _ := nodeRecordMap(t, jGrammar{
		extras: []string{jSym("v3")},
		rules: [][2]string{
			{"v1", jSeq(jField("f1", jSym("v2")), jField("f2", jStr(";")))},
			{"v2", jStr("x")},
			// Reachable from the extras only; a literal, so it becomes a token.
			{"v3", jStr("y")},
		},
	})
	wantNodes(t, recs, named("v1"), anon(";"), named("v2"), named("v3"))
	wantEntry(t, recs, named("v1"), "regular", map[string]string{
		"field f1": fv(false, true, named("v2")),
		"field f2": fv(false, true, anon(";")),
	})
	wantEntry(t, recs, named("v3"), "leaf", nil)
}

// test_node_types_deeper_extras (root and extra flags not checked)
func TestNodeTypesDeeperExtras(t *testing.T) {
	recs, n := nodeRecordMap(t, jGrammar{
		extras: []string{jSym("v3")},
		rules: [][2]string{
			{"v1", jSeq(jField("f1", jSym("v2")), jField("f2", jStr(";")))},
			{"v2", jStr("x")},
			// Reachable from the extras only; not a literal, so it stays a rule.
			{"v3", jSeq(jStr("y"), jRepeat(jStr("z")))},
		},
	})
	if n != 6 {
		t.Errorf("node types: got %d, want 6", n)
	}
	wantEntry(t, recs, named("v1"), "regular", map[string]string{
		"field f1": fv(false, true, named("v2")),
		"field f2": fv(false, true, anon(";")),
	})
	wantEntry(t, recs, named("v3"), "regular", nil)
	wantEntry(t, recs, anon(";"), "leaf", nil)
	wantEntry(t, recs, named("v2"), "leaf", nil)
}

// test_node_types_with_supertypes (root flag not checked)
func TestNodeTypesWithSupertypes(t *testing.T) {
	recs, _ := nodeRecordMap(t, jGrammar{
		supertypes: []string{"_v2"},
		rules: [][2]string{
			{"v1", jField("f1", jSym("_v2"))},
			{"_v2", jChoice(jSym("v3"), jSym("v4"), jStr("*"))},
			{"v3", jStr("x")},
			{"v4", jStr("y")},
		},
	})
	wantEntry(t, recs, named("_v2"), "supertype", map[string]string{
		"subtypes": "[" + anon("*") + " " + named("v3") + " " + named("v4") + "]",
	})
	wantEntry(t, recs, named("v1"), "regular", map[string]string{
		"field f1": fv(false, true, named("_v2")),
	})
}

// test_node_types_with_aliased_supertype
func TestNodeTypesWithAliasedSupertype(t *testing.T) {
	recs, _ := nodeRecordMap(t, jGrammar{
		supertypes: []string{"_expression"},
		rules: [][2]string{
			{"document", jAlias(jSym("_expression"), "expression_target", true)},
			{"_expression", jSym("identifier")},
			{"identifier", jPat("[a-z]+")},
		},
	})
	key := "node " + named("expression_target")
	if recs[key] != "regular" {
		t.Fatalf("the aliased supertype has no regular entry: %q", recs[key])
	}
	if !strings.HasSuffix(recs[key+" children"], "types=["+named("identifier")+"]") {
		t.Errorf("children: %q", recs[key+" children"])
	}
}

// test_node_types_distinguish_named_and_anonymous_aliases
func TestNodeTypesDistinguishNamedAndAnonymousAliases(t *testing.T) {
	recs, _ := nodeRecordMap(t, jGrammar{rules: [][2]string{
		{"document", jChoice(jAlias(jSym("_node"), "same", true), jAlias(jSym("_node"), "same", false))},
		{"_node", jSeq(jSym("value"), jStr("!"))},
		{"value", jPat("[a-z]+")},
	}})
	for _, ref := range []string{anon("same"), named("same")} {
		if _, ok := recs["node "+ref]; !ok {
			t.Errorf("no entry %s", ref)
		}
	}
}

// test_node_types_merge_anonymous_node_and_token_aliases
func TestNodeTypesMergeAnonymousNodeAndTokenAliases(t *testing.T) {
	recs, _ := nodeRecordMap(t, jGrammar{rules: [][2]string{
		{"document", jChoice(jAlias(jSym("_node"), "same", false), jAlias(jStr("!"), "same", false))},
		{"_node", jSeq(jSym("value"), jSym("value"), jStr("?"))},
		{"value", jPat("[a-z]+")},
	}})
	if _, ok := recs["node "+named("same")]; ok {
		t.Errorf("a named entry for same")
	}
	if got, want := recs["node "+anon("same")+" children"], fv(true, false, named("value")); got != want {
		t.Errorf("children of same:\n got %s\nwant %s", got, want)
	}
}

// test_node_types_distinguish_extra_metadata_by_namedness (extra flags not checked)
func TestNodeTypesDistinguishExtraMetadataByNamedness(t *testing.T) {
	recs, _ := nodeRecordMap(t, jGrammar{
		extras: []string{jSym("_extra")},
		rules: [][2]string{
			{"document", jChoice(jAlias(jSym("_node"), "same", false), jAlias(jSym("_extra"), "same", true))},
			{"_node", jSeq(jSym("value"), jStr("?"))},
			{"_extra", jSeq(jStr("#"), jSym("extra_value"))},
			{"value", jPat("[a-z]+")},
			{"extra_value", jPat("[A-Z]+")},
		},
	})
	for _, ref := range []string{anon("same"), named("same")} {
		if _, ok := recs["node "+ref]; !ok {
			t.Errorf("no entry %s", ref)
		}
	}
}

// test_node_types_distinguish_supertype_dependencies_by_namedness
func TestNodeTypesDistinguishSupertypeDependenciesByNamedness(t *testing.T) {
	recs, _ := nodeRecordMap(t, jGrammar{
		supertypes: []string{"_super"},
		rules: [][2]string{
			{"document", jSym("_super")},
			{"_super", jChoice(jAlias(jSym("item"), "_super", false), jSym("other"))},
			{"item", jStr("x")},
			{"other", jStr("y")},
		},
	})
	for _, ref := range []string{anon("_super"), named("_super")} {
		if _, ok := recs["node "+ref]; !ok {
			t.Errorf("no entry %s", ref)
		}
	}
}

// test_node_types_with_alias_matching_canonical_supertype
func TestNodeTypesWithAliasMatchingCanonicalSupertype(t *testing.T) {
	g := jGrammar{
		supertypes: []string{"_super"},
		rules: [][2]string{
			{"document", jChoice(jAlias(jSym("_node"), "_super", true), jSym("_super"))},
			{"_super", jChoice(jSym("one"), jSym("two"))},
			{"_node", jSeq(jSym("value"), jStr("?"))},
			{"one", jStr("1")},
			{"two", jStr("2")},
			{"value", jPat("[a-z]+")},
		},
	}
	p, err := Prepare([]byte(g.text()))
	if err != nil {
		t.Fatal(err)
	}
	_, err = GetVariableInfo(p.Syntax, p.Lex, p.DefaultAliases)
	if e, ok := err.(*VariableInfoError); !ok || e.SupertypeAlias != "_super" {
		t.Fatalf("got %v, want SupertypeAliasCollision(_super)", err)
	}
}

// test_node_types_supertype_with_only_hidden_child
func TestNodeTypesSupertypeWithOnlyHiddenChild(t *testing.T) {
	nodeRecordMap(t, jGrammar{
		supertypes: []string{"_type_a", "_type_b"},
		externals:  []string{jSym("_hidden_ext")},
		rules: [][2]string{
			{"v1", jSeq(jSym("_type_a"), jSym("_type_b"))},
			// Supertype A: a choice of named subtypes.
			{"_type_a", jChoice(jSym("v2"), jSym("v3"))},
			{"v2", jStr("x")},
			{"v3", jStr("y")},
			// Supertype B: a hidden external token, no subtypes.
			{"_type_b", jSym("_hidden_ext")},
		},
	})
}

// test_node_types_for_children_without_fields (root flag not checked)
func TestNodeTypesForChildrenWithoutFields(t *testing.T) {
	recs, _ := nodeRecordMap(t, jGrammar{rules: [][2]string{
		{"v1", jSeq(jSym("v2"), jField("f1", jSym("v3")), jSym("v4"))},
		{"v2", jSeq(jStr("{"), jChoice(jSym("v3"), jBlank()), jStr("}"))},
		{"v3", jStr("x")},
		{"v4", jStr("y")},
	}})
	wantEntry(t, recs, named("v1"), "regular", map[string]string{
		"children": fv(true, true, named("v2"), named("v4")),
		"field f1": fv(false, true, named("v3")),
	})
	wantEntry(t, recs, named("v2"), "regular", map[string]string{
		"children": fv(false, false, named("v3")),
	})
}

// test_node_types_with_inlined_rules (root flag not checked)
func TestNodeTypesWithInlinedRules(t *testing.T) {
	recs, _ := nodeRecordMap(t, jGrammar{
		inline: []string{"v2"},
		rules: [][2]string{
			{"v1", jSeq(jSym("v2"), jSym("v3"))},
			// v2 is inlined, so it has no entry.
			{"v2", jAlias(jStr("a"), "x", true)},
			{"v3", jStr("b")},
		},
	})
	wantEntry(t, recs, named("v1"), "regular", map[string]string{
		"children": fv(true, true, named("v3"), named("x")),
	})
	if _, ok := recs["node "+named("v2")]; ok {
		t.Errorf("an entry for the inlined v2")
	}
}

// test_node_types_for_aliased_nodes
func TestNodeTypesForAliasedNodes(t *testing.T) {
	recs, _ := nodeRecordMap(t, jGrammar{rules: [][2]string{
		{"thing", jChoice(jSym("type"), jSym("expression"))},
		{"type", jChoice(jAlias(jSym("identifier"), "type_identifier", true), jStr("void"))},
		{"expression", jChoice(jSym("identifier"), jAlias(jSym("foo_identifier"), "identifier", true))},
		{"identifier", jPat(`\w+`)},
		{"foo_identifier", jPat(`[\w-]+`)},
	}})
	for k := range recs {
		if strings.Contains(k, `"foo_identifier"`) {
			t.Errorf("an entry for foo_identifier: %s", k)
		}
	}
	wantEntry(t, recs, named("identifier"), "leaf", nil)
	wantEntry(t, recs, named("type_identifier"), "leaf", nil)
}

// test_node_types_with_multiple_valued_fields (root flag not checked)
func TestNodeTypesWithMultipleValuedFields(t *testing.T) {
	recs, _ := nodeRecordMap(t, jGrammar{rules: [][2]string{
		{"a", jSeq(jChoice(jBlank(), jRepeat(jField("f1", jSym("b")))), jRepeat(jSym("c")))},
		{"b", jStr("b")},
		{"c", jStr("c")},
	}})
	wantEntry(t, recs, named("a"), "regular", map[string]string{
		"children": fv(true, true, named("c")),
		"field f1": fv(true, false, named("b")),
	})
}

// test_node_types_with_fields_on_hidden_tokens (root flag not checked)
func TestNodeTypesWithFieldsOnHiddenTokens(t *testing.T) {
	recs, _ := nodeRecordMap(t, jGrammar{rules: [][2]string{
		{"script", jSeq(jField("a", jPat("hi")), jField("b", jPat("bye")))},
	}})
	wantNodes(t, recs, named("script"))
	wantEntry(t, recs, named("script"), "regular", nil)
}

// test_node_types_with_multiple_rules_same_alias_name (root flag and entry order not checked)
func TestNodeTypesWithMultipleRulesSameAliasName(t *testing.T) {
	recs, _ := nodeRecordMap(t, jGrammar{rules: [][2]string{
		{"script", jChoice(jSym("a"), jAlias(jSym("b"), "a", true))},
		{"a", jSeq(jField("f1", jStr("1")), jField("f2", jStr("2")))},
		{"b", jSeq(jField("f2", jStr("22")), jField("f2", jStr("222")), jField("f3", jStr("3")))},
	}})
	wantNodes(t, recs, named("a"), named("script"), anon("1"), anon("2"), anon("22"), anon("222"), anon("3"))
	// A combination of the types of a and b.
	wantEntry(t, recs, named("a"), "regular", map[string]string{
		"field f1": fv(false, false, anon("1")),
		"field f2": fv(true, true, anon("2"), anon("22"), anon("222")),
		"field f3": fv(false, false, anon("3")),
	})
	// Only one node.
	wantEntry(t, recs, named("script"), "regular", map[string]string{
		"children": fv(false, true, named("a")),
	})
}

// test_node_types_with_tokens_aliased_to_match_rules (entry order not checked)
func TestNodeTypesWithTokensAliasedToMatchRules(t *testing.T) {
	recs, _ := nodeRecordMap(t, jGrammar{rules: [][2]string{
		{"a", jSeq(jSym("b"), jSym("c"))},
		// Ordinarily, b nodes have two named c children.
		{"b", jSeq(jSym("c"), jStr("B"), jSym("c"))},
		// "D" is aliased as a b, which makes a b node with no children.
		{"c", jChoice(jStr("C"), jAlias(jStr("D"), "b", true))},
	}})
	wantNodes(t, recs, named("a"), named("b"), named("c"), anon("B"), anon("C"))
	wantEntry(t, recs, named("b"), "regular", map[string]string{
		"children": fv(true, false, named("c")),
	})
}

// test_supertype_and_inline_conflict, on the node types: the inlined
// supertype has no entry, and the field lists the inlined rule's children.
func TestNodeTypesSupertypeAndInlineConflict(t *testing.T) {
	recs, _ := nodeRecordMap(t, jGrammar{
		supertypes: []string{"_v2"},
		inline:     []string{"_v2"},
		rules: [][2]string{
			{"v1", jField("f1", jSym("_v2"))},
			{"_v2", jChoice(jSym("v3"), jSym("v4"))},
			{"v3", jStr("x")},
			{"v4", jStr("y")},
		},
	})
	for k := range recs {
		if strings.Contains(k, `"_v2"`) {
			t.Errorf("an entry for the inlined supertype: %s", k)
		}
	}
	if got := recs["node "+named("v1")+" field f1"]; !strings.HasSuffix(got, "types=["+named("v3")+" "+named("v4")+"]") {
		t.Errorf("field f1: %s", got)
	}
}
