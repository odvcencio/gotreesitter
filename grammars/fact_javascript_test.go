package grammars

import (
	"reflect"
	"strings"
	"testing"

	gts "github.com/odvcencio/gotreesitter"
)

func TestJavaScriptFactRulesResolveAndStayLanguageScoped(t *testing.T) {
	js := LangEntry{Name: "javascript", Language: JavascriptLanguage}
	if n := len(DeclarationRules(js)); n == 0 {
		t.Fatal("no JavaScript declaration rules")
	}
	if n := len(SignatureRules(js)); n == 0 {
		t.Fatal("no JavaScript signature rules")
	}
	if n := len(CallArgumentRules(js)); n != 2 {
		t.Fatalf("JavaScript call-argument rules = %d; want call_expression and new_expression", n)
	}
	// Every JavaScript row names JavaScript-only node types; Go's grammar lacks them.
	foreign := LangEntry{Name: "javascript", Language: GoLanguage}
	if got := DeclarationRules(foreign); len(got) != 0 {
		t.Fatalf("declaration rows resolved against Go: %#v", got)
	}
	if got := SignatureRules(foreign); len(got) != 0 {
		t.Fatalf("signature rows resolved against Go: %#v", got)
	}
	if got := CallArgumentRules(foreign); len(got) != 0 {
		t.Fatalf("argument rows resolved against Go: %#v", got)
	}
}

func TestJavaScriptDeclarationRulesGateMissingGrammarData(t *testing.T) {
	entry := LangEntry{Name: "javascript", Language: JavascriptLanguage}
	var good gts.DeclarationRule
	for _, row := range DeclarationRules(entry) {
		if row.Kind == "constant" {
			good = row
			break
		}
	}
	if good.ParentChildField != "kind" || good.ParentChildNodeType != "const" {
		t.Fatalf("missing const binding row: %#v", good)
	}
	for _, part := range []string{"parent field", "parent node", "container node"} {
		t.Run(part, func(t *testing.T) {
			bad := good
			switch part {
			case "parent field":
				bad.ParentChildField = "absent"
			case "parent node":
				bad.ParentChildNodeType = "absent"
			case "container node":
				bad.ContainerPath = []gts.ContainerRule{{NodeType: "absent", NameField: "name", NameNodeType: "identifier", TypeField: "body"}}
			}
			if got := gateDeclarationRules(entry, []gts.DeclarationRule{bad, good}); len(got) != 1 || got[0].ParentChildField != "kind" || got[0].ParentChildNodeType != "const" {
				t.Fatalf("gate = %#v", got)
			}
		})
	}
}

func TestJavaScriptSignatureRulesGateMissingGrammarData(t *testing.T) {
	entry := LangEntry{Name: "javascript", Language: JavascriptLanguage}
	var good gts.SignatureRule
	for _, r := range SignatureRules(entry) {
		if r.DelegateField != "" {
			good = r
			break
		}
	}
	if len(good.DelegateNodeTypes) == 0 {
		t.Fatal("missing value-bound signature row")
	}
	for _, part := range []string{"value field", "value node", "parameter node", "parameter name node", "alt parameters"} {
		t.Run(part, func(t *testing.T) {
			bad := good
			bad.Parameters = append([]gts.ParameterRule(nil), good.Parameters...)
			switch part {
			case "value field":
				bad.DelegateField = "absent"
			case "value node":
				bad.DelegateNodeTypes = []string{"absent"}
			case "parameter node":
				bad.Parameters[0].NodeType = "absent"
			case "alt parameters":
				bad.SingleParameterField = "absent"
			case "parameter name node":
				bad.Parameters[0].NameNodeType = "absent"
			}
			if got := gateSignatureRules(entry, []gts.SignatureRule{bad, good}); len(got) != 1 {
				t.Fatalf("gate = %#v", got)
			}
		})
	}
}

func TestJavaScriptCallArgumentRulesGateMissingGrammarData(t *testing.T) {
	entry := LangEntry{Name: "javascript", Language: JavascriptLanguage}
	rules := CallArgumentRules(entry)
	if len(rules) != 2 || rules[0].NodeType != "call_expression" || rules[1].NodeType != "new_expression" || !rules[1].Standalone || rules[0].Standalone {
		t.Fatalf("rules = %#v", rules)
	}
	for _, part := range []string{"node", "list", "spread", "kind node"} {
		t.Run(part, func(t *testing.T) {
			bad := rules[0]
			bad.Kinds = append([]gts.ExpressionKindRule(nil), bad.Kinds...)
			switch part {
			case "node":
				bad.NodeType = "absent"
			case "list":
				bad.ListNodeType = "absent"
			case "spread":
				bad.SpreadNodeType = "absent"
			case "kind node":
				bad.Kinds[0].NodeType = "absent"
			}
			if got := gateCallArgumentRules(entry, []gts.CallArgumentRule{bad, rules[0]}); len(got) != 1 {
				t.Fatalf("gate = %#v", got)
			}
		})
	}
}

func parseJavaScriptFacts(t *testing.T, source string) *gts.Tree {
	t.Helper()
	entry := DetectLanguage("main.js")
	lang := entry.Language()
	parser := gts.NewParser(lang)
	var tree *gts.Tree
	var err error
	if entry.TokenSourceFactory != nil {
		tree, err = parser.ParseWithTokenSource([]byte(source), entry.TokenSourceFactory([]byte(source), lang))
	} else {
		tree, err = parser.Parse([]byte(source))
	}
	if err != nil {
		t.Fatal(err)
	}
	if tree == nil || tree.RootNode() == nil {
		t.Fatal("JavaScript fixture returned nil tree/root")
	}
	if tree.RootNode().HasError() {
		t.Fatalf("JavaScript fixture has errors: %s", tree.RootNode().SExpr(lang))
	}
	return tree
}

const javaScriptFactsSource = `const a = 1, b = 2;
let l1, l2 = 3;
var v = 4;
export const c = () => 1, d = function (x, y = 2, ...rest) {};
export let e1 = 5;
const { dx, dy: dz } = obj;
const [p, q] = arr;
function top(a1, { b1 }, [c1], d1 = 1, ...r1) { const local = 1; function inner() {} class Inner { im() {} } }
export function exported() {}
function* gen(n) {}
class Base extends Object {
  static s = 1;
  f = 2;
  #p = 3;
  m(a2) {}
  static sm() {}
  get gg() { return 1; }
  set gg(v2) {}
  static get sg() { return 2; }
  constructor(cx) { super(); }
  [computed]() {}
  static {}
}
export default class Named {}
const arrow1 = x => x;
const Cls = class {};
if (cond) { let blockLet = 1; }
`

func richJavaScriptProgram(t *testing.T, kinds gts.FactKind) *gts.FactProgram {
	t.Helper()
	entry := DetectLanguage("main.js")
	p, err := gts.NewFactProgramWithOptions(entry.Language(), kinds,
		gts.WithDeclarationRules(DeclarationRules(*entry)),
		gts.WithSignatureRules(SignatureRules(*entry)),
		gts.WithCallArgumentRules(CallArgumentRules(*entry)))
	if err != nil {
		t.Fatal(err)
	}
	return p
}

// jsSpan locates text inside the unique context in source.
func jsSpan(t *testing.T, source, context, text string) (uint32, uint32) {
	t.Helper()
	base := strings.Index(source, context)
	if base < 0 || strings.Index(source[base+1:], context) >= 0 {
		t.Fatalf("context %q is missing or not unique", context)
	}
	at := strings.Index(context, text)
	if at < 0 {
		t.Fatalf("missing %q in %q", text, context)
	}
	return uint32(base + at), uint32(base + at + len(text))
}

// jsNameOffset finds a declared name after any leading modifier keywords.
func jsNameOffset(construct, name string) uint32 {
	if at := strings.Index(construct, " "+name); at >= 0 {
		return uint32(at + 1)
	}
	return uint32(strings.Index(construct, name))
}

func TestFactDeclarationsJavaScriptGolden(t *testing.T) {
	source := javaScriptFactsSource
	tree := parseJavaScriptFacts(t, source)
	defer tree.Release()
	class := "class Base extends Object {\n  static s = 1;\n  f = 2;\n  #p = 3;\n  m(a2) {}\n  static sm() {}\n  get gg() { return 1; }\n  set gg(v2) {}\n  static get sg() { return 2; }\n  constructor(cx) { super(); }\n  [computed]() {}\n  static {}\n}"
	specs := []struct {
		name, kind, node, construct, in, shape, container string
	}{
		{"a", "constant", "variable_declarator", "a = 1", "const a = 1, b = 2;", "", ""},
		{"b", "constant", "variable_declarator", "b = 2", "const a = 1, b = 2;", "", ""},
		{"l1", "variable", "variable_declarator", "l1", "let l1, l2 = 3;", "", ""},
		{"l2", "variable", "variable_declarator", "l2 = 3", "let l1, l2 = 3;", "", ""},
		{"v", "variable", "variable_declarator", "v = 4", "var v = 4;", "", ""},
		{"c", "constant", "variable_declarator", "c = () => 1", "export const c = () => 1, d", "", ""},
		{"d", "constant", "variable_declarator", "d = function (x, y = 2, ...rest) {}", "export const c = () => 1, d = function (x, y = 2, ...rest) {};", "", ""},
		{"e1", "variable", "variable_declarator", "e1 = 5", "export let e1 = 5;", "", ""},
		{"Base", "type", "class_declaration", class, class, "class", ""},
		{"s", "field", "field_definition", "static s = 1", class, "", "Base"},
		{"f", "field", "field_definition", "f = 2", class, "", "Base"},
		{"#p", "field", "field_definition", "#p = 3", class, "", "Base"},
		{"m", "method", "method_definition", "m(a2) {}", class, "", "Base"},
		{"sm", "method", "method_definition", "static sm() {}", class, "", "Base"},
		{"gg", "method", "method_definition", "get gg() { return 1; }", class, "", "Base"},
		{"gg", "method", "method_definition", "set gg(v2) {}", class, "", "Base"},
		{"sg", "method", "method_definition", "static get sg() { return 2; }", class, "", "Base"},
		{"constructor", "method", "method_definition", "constructor(cx) { super(); }", class, "", "Base"},
		{"Named", "type", "class_declaration", "class Named {}", "export default class Named {}", "class", ""},
		{"arrow1", "constant", "variable_declarator", "arrow1 = x => x", "const arrow1 = x => x;", "", ""},
		{"Cls", "constant", "variable_declarator", "Cls = class {}", "const Cls = class {};", "", ""},
	}
	var want []gts.DeclarationFact
	for _, w := range specs {
		start, end := jsSpan(t, source, w.in, w.construct)
		off := jsNameOffset(w.construct, w.name)
		ns, ne := start+off, start+off+uint32(len(w.name))
		f := gts.DeclarationFact{Lang: "javascript", Kind: w.kind, Name: w.name, NodeType: w.node, StartByte: start, EndByte: end, NameStartByte: ns, NameEndByte: ne, Shape: w.shape, Container: w.container}
		if w.container != "" {
			f.ContainerStartByte, f.ContainerEndByte = jsSpan(t, source, class, class)
		}
		want = append(want, f)
	}
	got := richJavaScriptProgram(t, gts.FactDeclarations).Extract(tree).Declarations
	if !reflect.DeepEqual(got, want) {
		for i := range got {
			if i >= len(want) || got[i] != want[i] {
				t.Errorf("declaration %d = %#v", i, got[i])
				if i < len(want) {
					t.Errorf("      want %#v", want[i])
				}
			}
		}
		t.Fatalf("got %d declarations; want %d", len(got), len(want))
	}
	// Destructuring and locals never produce facts.
	for _, f := range got {
		switch f.Name {
		case "dx", "dz", "p", "q", "local", "inner", "Inner", "im", "blockLet", "computed":
			t.Errorf("unexpected fact %#v", f)
		}
	}
}

func TestFactSignaturesJavaScriptGolden(t *testing.T) {
	source := javaScriptFactsSource
	tree := parseJavaScriptFacts(t, source)
	defer tree.Release()
	class := "class Base extends Object {\n  static s = 1;\n  f = 2;\n  #p = 3;\n  m(a2) {}\n  static sm() {}\n  get gg() { return 1; }\n  set gg(v2) {}\n  static get sg() { return 2; }\n  constructor(cx) { super(); }\n  [computed]() {}\n  static {}\n}"
	type param struct {
		decl, name string
		variadic   bool
	}
	dFn := "d = function (x, y = 2, ...rest) {}"
	specs := []struct {
		kind, name, node, construct, in, container string
		params                                     []param
	}{
		{"function", "c", "variable_declarator", "c = () => 1", "export const c = () => 1, d", "", nil},
		{"function", "d", "variable_declarator", dFn, dFn, "", []param{{"x", "x", false}, {"y = 2", "y", false}, {"...rest", "rest", true}}},
		{"function", "top", "function_declaration", "function top(a1, { b1 }, [c1], d1 = 1, ...r1) { const local = 1; function inner() {} class Inner { im() {} } }", "function top(a1, { b1 }, [c1], d1 = 1, ...r1) { const local = 1; function inner() {} class Inner { im() {} } }", "",
			[]param{{"a1", "a1", false}, {"{ b1 }", "{ b1 }", false}, {"[c1]", "[c1]", false}, {"d1 = 1", "d1", false}, {"...r1", "r1", true}}},
		{"function", "exported", "function_declaration", "function exported() {}", "export function exported() {}", "", nil},
		{"function", "gen", "generator_function_declaration", "function* gen(n) {}", "function* gen(n) {}", "", []param{{"n", "n", false}}},
		{"method", "m", "method_definition", "m(a2) {}", class, "Base", []param{{"a2", "a2", false}}},
		{"method", "sm", "method_definition", "static sm() {}", class, "Base", nil},
		{"method", "gg", "method_definition", "get gg() { return 1; }", class, "Base", nil},
		{"method", "gg", "method_definition", "set gg(v2) {}", class, "Base", []param{{"v2", "v2", false}}},
		{"method", "sg", "method_definition", "static get sg() { return 2; }", class, "Base", nil},
		{"method", "constructor", "method_definition", "constructor(cx) { super(); }", class, "Base", []param{{"cx", "cx", false}}},
		{"function", "arrow1", "variable_declarator", "arrow1 = x => x", "const arrow1 = x => x;", "", []param{{"x", "x", false}}},
	}
	var want []gts.SignatureFact
	for _, w := range specs {
		start, end := jsSpan(t, source, w.in, w.construct)
		off := jsNameOffset(w.construct, w.name)
		f := gts.SignatureFact{Lang: "javascript", Kind: w.kind, Name: w.name, Container: w.container, NodeType: w.node, StartByte: start, EndByte: end, NameStartByte: start + off, NameEndByte: start + off + uint32(len(w.name))}
		for _, p := range w.params {
			open := strings.Index(w.construct, "(")
			if open < 0 {
				open = strings.Index(w.construct, "=")
			}
			base := start + uint32(open+strings.Index(w.construct[open:], p.decl))
			pf := gts.ParameterFact{Name: p.name, Variadic: p.variadic, StartByte: base, EndByte: base + uint32(len(p.decl))}
			nameAt := base + uint32(strings.LastIndex(p.decl, p.name))
			if p.name == "{ b1 }" || p.name == "[c1]" {
				nameAt = base
			}
			pf.NameStartByte, pf.NameEndByte = nameAt, nameAt+uint32(len(p.name))
			f.Parameters = append(f.Parameters, pf)
		}
		want = append(want, f)
	}
	got := richJavaScriptProgram(t, gts.FactSignatures).Extract(tree).Signatures
	if !reflect.DeepEqual(got, want) {
		for i := range got {
			if i >= len(want) || !reflect.DeepEqual(got[i], want[i]) {
				t.Errorf("signature %d = %#v", i, got[i])
				if i < len(want) {
					t.Errorf("     want %#v", want[i])
				}
			}
		}
		t.Fatalf("got %d signatures; want %d", len(got), len(want))
	}
}

func TestFactCallArgumentsJavaScriptGoldenAndJoin(t *testing.T) {
	const source = "foo(a, b.c, g(), { k: 1 }, [1], () => 1, function () {}, 1, \"s\", -a, a + b, a[0], (a), ...rest, `t${x}`, /re/, null, true, new X(1), c ? 1 : 2);\nnew Foo(a, ...b);\nnew Bar;\nobj.method?.(z);\nempty();\n"
	tree := parseJavaScriptFacts(t, source)
	defer tree.Release()
	facts := richJavaScriptProgram(t, gts.FactCalls|gts.FactCallArguments).Extract(tree)
	type arg = struct {
		text, kind, node string
		spread           bool
	}
	specs := []struct {
		call string
		args []arg
	}{
		{"foo(a, b.c, g(), { k: 1 }, [1], () => 1, function () {}, 1, \"s\", -a, a + b, a[0], (a), ...rest, `t${x}`, /re/, null, true, new X(1), c ? 1 : 2)", []arg{
			{"a", "identifier", "identifier", false}, {"b.c", "selector", "member_expression", false}, {"g()", "call", "call_expression", false},
			{"{ k: 1 }", "composite_literal", "object", false}, {"[1]", "composite_literal", "array", false},
			{"() => 1", "func_literal", "arrow_function", false}, {"function () {}", "func_literal", "function_expression", false},
			{"1", "literal", "number", false}, {"\"s\"", "literal", "string", false}, {"-a", "unary", "unary_expression", false},
			{"a + b", "binary", "binary_expression", false}, {"a[0]", "index", "subscript_expression", false}, {"(a)", "parenthesized", "parenthesized_expression", false},
			{"...rest", "identifier", "spread_element", true}, {"`t${x}`", "literal", "template_string", false}, {"/re/", "literal", "regex", false},
			{"null", "literal", "null", false}, {"true", "literal", "true", false}, {"new X(1)", "other", "new_expression", false}, {"c ? 1 : 2", "other", "ternary_expression", false},
		}},
		{"new X(1)", []arg{{"1", "literal", "number", false}}},
		{"new Foo(a, ...b)", []arg{{"a", "identifier", "identifier", false}, {"...b", "identifier", "spread_element", true}}},
		{"obj.method?.(z)", []arg{{"z", "identifier", "identifier", false}}},
	}
	var want []gts.CallArgumentFact
	for _, w := range specs {
		callAt := strings.Index(source, w.call)
		next := strings.Index(w.call, "(") + 1
		if strings.HasPrefix(w.call, "obj.method") {
			next = strings.Index(w.call, "?.(") + 3
		}
		for i, a := range w.args {
			at := strings.Index(w.call[next:], a.text) + next
			next = at + len(a.text)
			want = append(want, gts.CallArgumentFact{CallStartByte: uint32(callAt), CallEndByte: uint32(callAt + len(w.call)), Index: i, Kind: a.kind, NodeType: a.node, StartByte: uint32(callAt + at), EndByte: uint32(callAt + at + len(a.text)), Spread: a.spread})
		}
	}
	if !reflect.DeepEqual(facts.CallArguments, want) {
		for i := range facts.CallArguments {
			if i >= len(want) || facts.CallArguments[i] != want[i] {
				t.Errorf("argument %d = %#v", i, facts.CallArguments[i])
				if i < len(want) {
					t.Errorf("      want %#v", want[i])
				}
			}
		}
		t.Fatalf("got %d arguments; want %d", len(facts.CallArguments), len(want))
	}
	// call_expression arguments join exactly one CallRef; new_expression has none.
	for _, a := range facts.CallArguments {
		matches := 0
		for _, c := range facts.Calls {
			if c.StartByte == a.CallStartByte && c.EndByte == a.CallEndByte {
				matches++
			}
		}
		isNew := strings.HasPrefix(source[a.CallStartByte:a.CallEndByte], "new ")
		if (isNew && matches != 0) || (!isNew && matches != 1) {
			t.Fatalf("argument joins %d calls: %#v", matches, a)
		}
	}
	only := richJavaScriptProgram(t, gts.FactCallArguments).Extract(tree)
	if !reflect.DeepEqual(only.CallArguments, facts.CallArguments) || len(only.Calls) != 0 {
		t.Fatal("argument-only selection changed join")
	}
}
