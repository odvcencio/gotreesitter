package grammars

import (
	"reflect"
	"strings"
	"testing"

	gts "github.com/odvcencio/gotreesitter"
)

// Fixture pieces are composed so that every construct has one known location.
const (
	pyModuleHead = "import os\n" +
		"alpha = 1\n" +
		"beta, gamma = 1, 2\n" +
		"delta: int = 3\n" +
		"(eps, zeta) = 1, 2\n" +
		"[eta, theta] = 1, 2\n" +
		"*iota, kappa = 1, 2, 3\n" +
		"lam = mu = 4\n" +
		"_ = 0\n" +
		"_, nu = 5, 6\n" +
		"obj.attr = 1\n" +
		"lst[0] = 2\n" +
		"cnt += 1\n\n"

	pyInner = "    class Inner:\n" +
		"        deep = 1\n" +
		"        def im(self): pass\n"
	pyDeco2 = "    @deco\n" +
		"    class Deco2:\n" +
		"        pass\n"
	pyBase = "class Base:\n" +
		"    z = 1\n" +
		"    w: int\n" +
		"    p, q = 1, 2\n" +
		"    _ = 9\n" +
		"    def meth(self, a: int, b=2, *args, c: str = \"x\", **kw) -> str:\n" +
		"        loc = 1\n" +
		"        def inner(): pass\n" +
		"        return loc\n" +
		"    @staticmethod\n" +
		"    async def am(cls): pass\n" +
		pyInner + pyDeco2 + "\n"

	pyTop = "def top(a, /, b, *rest: int, c, d: int = 1, **opts: str) -> None:\n" +
		"    local = 1\n" +
		"    class Local:\n" +
		"        hidden = 1\n" +
		"        def lm(self): pass\n\n"
	pyKwOnly = "def kwonly(p, *, q): pass\n\n"
	pyTail   = "@dec\n" +
		"class Decorated: pass\n\n" +
		"if alpha:\n" +
		"    cond = 1\n" +
		"    def in_if(): pass\n\n"

	pyCallInner = "top(1)"
	pyCall      = "top(a, b.attr, top(1), 1, 2.5, \"s\", f\"x{y}\", -a, a + b, a and b, a < b, (a), x[0], [1], {1: 2}, (1, 2), not a, a if b else c, None, True, lambda: 1, ..., *args, key=1, fn=lambda: 1, **kw)"
	pyCallLine  = "result = " + pyCall + "\n" +
		"top()\n" +
		"obj.method(alpha)\n" +
		"print(v for v in alpha)\n"

	pyFixture = pyModuleHead + pyBase + pyTop + pyKwOnly + pyTail + pyCallLine
)

func pythonProgram(t *testing.T, kinds gts.FactKind) (*gts.FactProgram, *gts.Tree) {
	t.Helper()
	entry := DetectLanguage("main.py")
	if entry == nil {
		t.Fatal("no Python entry")
	}
	program, err := gts.NewFactProgramWithOptions(entry.Language(), kinds,
		gts.WithDeclarationRules(DeclarationRules(*entry)),
		gts.WithSignatureRules(SignatureRules(*entry)),
		gts.WithCallArgumentRules(CallArgumentRules(*entry)))
	if err != nil {
		t.Fatal(err)
	}
	return program, parsePython(t, pyFixture)
}

func parsePython(t *testing.T, source string) *gts.Tree {
	t.Helper()
	entry := DetectLanguage("main.py")
	lang := entry.Language()
	var tree *gts.Tree
	var err error
	if entry.TokenSourceFactory != nil {
		tree, err = gts.NewParser(lang).ParseWithTokenSource([]byte(source), entry.TokenSourceFactory([]byte(source), lang))
	} else {
		tree, err = gts.NewParser(lang).Parse([]byte(source))
	}
	if err != nil {
		t.Fatal(err)
	}
	if tree.RootNode().HasError() {
		t.Fatalf("parse error: %s", tree.RootNode().SExpr(lang))
	}
	return tree
}

func pyIndex(t *testing.T, haystack, needle string) int {
	t.Helper()
	i := strings.Index(haystack, needle)
	if i < 0 {
		t.Fatalf("missing %q", needle)
	}
	return i
}

type pyDecl struct {
	name, kind, nodeType, construct, shape, container, containerConstruct string
}

func TestFactDeclarationsPythonGolden(t *testing.T) {
	program, tree := pythonProgram(t, gts.FactDeclarations)
	defer tree.Release()
	inner := "class Inner:\n        deep = 1\n        def im(self): pass"
	deco2 := "class Deco2:\n        pass"
	base := strings.TrimSuffix(pyBase, "\n\n")
	want := []pyDecl{
		{"alpha", "variable", "assignment", "alpha = 1", "", "", ""},
		{"beta", "variable", "assignment", "beta, gamma = 1, 2", "", "", ""},
		{"gamma", "variable", "assignment", "beta, gamma = 1, 2", "", "", ""},
		{"delta", "variable", "assignment", "delta: int = 3", "", "", ""},
		{"eps", "variable", "assignment", "(eps, zeta) = 1, 2", "", "", ""},
		{"zeta", "variable", "assignment", "(eps, zeta) = 1, 2", "", "", ""},
		{"eta", "variable", "assignment", "[eta, theta] = 1, 2", "", "", ""},
		{"theta", "variable", "assignment", "[eta, theta] = 1, 2", "", "", ""},
		{"iota", "variable", "assignment", "*iota, kappa = 1, 2, 3", "", "", ""},
		{"kappa", "variable", "assignment", "*iota, kappa = 1, 2, 3", "", "", ""},
		{"lam", "variable", "assignment", "lam = mu = 4", "", "", ""},
		{"mu", "variable", "assignment", "mu = 4", "", "", ""},
		{"nu", "variable", "assignment", "_, nu = 5, 6", "", "", ""},
		{"Base", "type", "class_definition", base, "class", "", ""},
		{"z", "field", "assignment", "z = 1", "", "Base", base},
		{"w", "field", "assignment", "w: int", "", "Base", base},
		{"p", "field", "assignment", "p, q = 1, 2", "", "Base", base},
		{"q", "field", "assignment", "p, q = 1, 2", "", "Base", base},
		{"meth", "method", "function_definition", "def meth(self, a: int, b=2, *args, c: str = \"x\", **kw) -> str:\n        loc = 1\n        def inner(): pass\n        return loc", "", "Base", base},
		{"am", "method", "function_definition", "async def am(cls): pass", "", "Base", base},
		{"Inner", "type", "class_definition", "class Inner:\n        deep = 1\n        def im(self): pass", "class", "Base", base},
		{"deep", "field", "assignment", "deep = 1", "", "Base.Inner", inner},
		{"im", "method", "function_definition", "def im(self): pass", "", "Base.Inner", inner},
		{"Deco2", "type", "class_definition", deco2, "class", "Base", base},
		{"Decorated", "type", "class_definition", "class Decorated: pass", "class", "", ""},
		{"result", "variable", "assignment", "result = " + pyCall, "", "", ""},
	}
	got := program.Extract(tree).Declarations
	if len(got) != len(want) {
		t.Fatalf("declarations = %d, want %d:\n%#v", len(got), len(want), got)
	}
	for i, w := range want {
		start := pyIndex(t, pyFixture, w.construct)
		if w.construct == "class Inner:\n        deep = 1\n        def im(self): pass" || w.construct == deco2 {
			// Nested constructs are unique; this guards the fixture itself.
			if strings.Count(pyFixture, w.construct) != 1 {
				t.Fatalf("ambiguous %q", w.construct)
			}
		}
		if w.container != "" {
			start = pyIndex(t, pyFixture, w.containerConstruct) + pyIndex(t, w.containerConstruct, w.construct)
		}
		name := start + pyIndex(t, w.construct, w.name)
		if w.name == "Base" || w.name == "Inner" || w.name == "Deco2" || w.name == "Decorated" {
			name = start + len("class ")
		}
		if w.name == "am" {
			name = start + len("async def ")
		}
		if strings.HasPrefix(w.construct, "def ") {
			name = start + len("def ")
		}
		wantFact := gts.DeclarationFact{Lang: "python", Kind: w.kind, Name: w.name, NodeType: w.nodeType,
			StartByte: uint32(start), EndByte: uint32(start + len(w.construct)),
			NameStartByte: uint32(name), NameEndByte: uint32(name + len(w.name)), Shape: w.shape, Container: w.container}
		if w.container != "" {
			cs := pyIndex(t, pyFixture, w.containerConstruct)
			wantFact.ContainerStartByte, wantFact.ContainerEndByte = uint32(cs), uint32(cs+len(w.containerConstruct))
		}
		if got[i] != wantFact {
			t.Errorf("declaration %d = %#v\nwant %#v", i, got[i], wantFact)
		}
	}
}

func TestFactDeclarationsPythonOmissions(t *testing.T) {
	program, tree := pythonProgram(t, gts.FactDeclarations)
	defer tree.Release()
	for _, f := range program.Extract(tree).Declarations {
		switch f.Name {
		case "_", "loc", "local", "hidden", "lm", "Local", "inner", "top", "kwonly", "cond", "in_if", "attr", "cnt", "obj", "lst", "os", "self", "a", "b":
			t.Errorf("unexpected declaration %#v", f)
		}
	}
	// A module that holds only an underscore binding has no facts.
	small := parsePython(t, "_ = 1\n_, _ = 2, 3\n")
	defer small.Release()
	if got := program.Extract(small).Declarations; len(got) != 0 {
		t.Fatalf("underscore declarations = %#v", got)
	}
}

type pyParam struct {
	decl, name, typ string
	variadic        bool
}

func pyParams(t *testing.T, source, signature string, specs []pyParam, last ...bool) []gts.ParameterFact {
	t.Helper()
	var result []gts.ParameterFact
	base := pyIndex(t, source, signature)
	for _, w := range specs {
		at := pyIndex(t, signature, w.decl)
		if len(last) > 0 && last[0] {
			at = strings.LastIndex(signature, w.decl)
		}
		f := gts.ParameterFact{Name: w.name, Type: w.typ, Variadic: w.variadic,
			StartByte: uint32(base + at), EndByte: uint32(base + at + len(w.decl))}
		if w.name != "" {
			n := at + pyIndex(t, w.decl, w.name)
			f.NameStartByte, f.NameEndByte = uint32(base+n), uint32(base+n+len(w.name))
		}
		if w.typ != "" {
			ty := at + strings.LastIndex(w.decl, w.typ)
			f.TypeStartByte, f.TypeEndByte = uint32(base+ty), uint32(base+ty+len(w.typ))
		}
		result = append(result, f)
	}
	return result
}

func TestFactSignaturesPythonGolden(t *testing.T) {
	program, tree := pythonProgram(t, gts.FactSignatures)
	defer tree.Release()
	meth := "def meth(self, a: int, b=2, *args, c: str = \"x\", **kw) -> str:\n        loc = 1\n        def inner(): pass\n        return loc"
	specs := []struct {
		decl, kind, name, container string
		params, results             []pyParam
	}{
		{meth, "method", "meth", "Base", []pyParam{{"self", "self", "", false}, {"a: int", "a", "int", false}, {"b=2", "b", "", false}, {"*args", "*args", "", true}, {"c: str = \"x\"", "c", "str", false}, {"**kw", "**kw", "", true}}, []pyParam{{"str", "", "str", false}}},
		{"def inner(): pass", "function", "inner", "", nil, nil},
		{"async def am(cls): pass", "method", "am", "Base", []pyParam{{"cls", "cls", "", false}}, nil},
		{"def im(self): pass", "method", "im", "Base.Inner", []pyParam{{"self", "self", "", false}}, nil},
		{"def top(a, /, b, *rest: int, c, d: int = 1, **opts: str) -> None:\n    local = 1\n    class Local:\n        hidden = 1\n        def lm(self): pass", "function", "top", "",
			[]pyParam{{"a", "a", "", false}, {"b", "b", "", false}, {"*rest: int", "*rest", "int", true}, {"c", "c", "", false}, {"d: int = 1", "d", "int", false}, {"**opts: str", "**opts", "str", true}}, []pyParam{{"None", "", "None", false}}},
		{"def lm(self): pass", "function", "lm", "", []pyParam{{"self", "self", "", false}}, nil},
		{"def kwonly(p, *, q): pass", "function", "kwonly", "", []pyParam{{"p", "p", "", false}, {"q", "q", "", false}}, nil},
		{"def in_if(): pass", "function", "in_if", "", nil, nil},
	}
	got := program.Extract(tree).Signatures
	if len(got) != len(specs) {
		t.Fatalf("signatures = %d, want %d:\n%#v", len(got), len(specs), got)
	}
	for i, w := range specs {
		at := pyIndex(t, pyFixture, w.decl)
		nameAt := at + pyIndex(t, w.decl, w.name)
		want := gts.SignatureFact{Lang: "python", Kind: w.kind, Name: w.name, Container: w.container, NodeType: "function_definition",
			StartByte: uint32(at), EndByte: uint32(at + len(w.decl)), NameStartByte: uint32(nameAt), NameEndByte: uint32(nameAt + len(w.name)),
			Parameters: pyParams(t, pyFixture, w.decl, w.params), Results: pyParams(t, pyFixture, w.decl, w.results, true)}
		if !reflect.DeepEqual(got[i], want) {
			t.Errorf("signature %d = %#v\nwant %#v", i, got[i], want)
		}
	}
	for _, f := range got {
		if f.Name == "Base" || f.Name == "Local" || f.Name == "result" {
			t.Errorf("class or variable became a signature: %#v", f)
		}
	}
}

func TestFactCallArgumentsPythonGoldenAndJoin(t *testing.T) {
	program, tree := pythonProgram(t, gts.FactCalls|gts.FactCallArguments)
	defer tree.Release()
	facts := program.Extract(tree)
	type arg struct{ text, kind, node string }
	outer := []arg{
		{"a", "identifier", "identifier"}, {"b.attr", "selector", "attribute"}, {"top(1)", "call", "call"},
		{"1", "literal", "integer"}, {"2.5", "literal", "float"}, {"\"s\"", "literal", "string"}, {"f\"x{y}\"", "literal", "string"},
		{"-a", "unary", "unary_operator"}, {"a + b", "binary", "binary_operator"}, {"a and b", "binary", "boolean_operator"},
		{"a < b", "binary", "comparison_operator"}, {"(a)", "parenthesized", "parenthesized_expression"}, {"x[0]", "index", "subscript"},
		{"[1]", "composite_literal", "list"}, {"{1: 2}", "composite_literal", "dictionary"}, {"(1, 2)", "composite_literal", "tuple"},
		{"not a", "unary", "not_operator"}, {"a if b else c", "other", "conditional_expression"}, {"None", "literal", "none"},
		{"True", "literal", "true"}, {"lambda: 1", "func_literal", "lambda"}, {"...", "literal", "ellipsis"},
		{"*args", "identifier", "list_splat"}, {"key=1", "keyword", "keyword_argument"}, {"fn=lambda: 1", "keyword", "keyword_argument"},
		{"**kw", "identifier", "dictionary_splat"},
	}
	callAt := pyIndex(t, pyFixture, pyCall)
	var want []gts.CallArgumentFact
	pos := callAt + len("top(")
	for i, a := range outer {
		if !strings.HasPrefix(pyFixture[pos:], a.text) {
			t.Fatalf("argument %d %q not at %d", i, a.text, pos)
		}
		want = append(want, gts.CallArgumentFact{CallStartByte: uint32(callAt), CallEndByte: uint32(callAt + len(pyCall)), Index: i, Kind: a.kind, NodeType: a.node,
			StartByte: uint32(pos), EndByte: uint32(pos + len(a.text)), Spread: a.node == "list_splat" || a.node == "dictionary_splat"})
		pos += len(a.text) + len(", ")
	}
	innerAt := callAt + pyIndex(t, pyCall, pyCallInner)
	want = append(want, gts.CallArgumentFact{CallStartByte: uint32(innerAt), CallEndByte: uint32(innerAt + len(pyCallInner)), Kind: "literal", NodeType: "integer",
		StartByte: uint32(innerAt + len("top(")), EndByte: uint32(innerAt + len("top(1"))})
	objAt := pyIndex(t, pyFixture, "obj.method(alpha)")
	want = append(want, gts.CallArgumentFact{CallStartByte: uint32(objAt), CallEndByte: uint32(objAt + len("obj.method(alpha)")), Kind: "identifier", NodeType: "identifier",
		StartByte: uint32(objAt + len("obj.method(")), EndByte: uint32(objAt + len("obj.method(alpha"))})
	if !reflect.DeepEqual(facts.CallArguments, want) {
		for i := range facts.CallArguments {
			if i >= len(want) || facts.CallArguments[i] != want[i] {
				t.Errorf("argument %d = %#v", i, facts.CallArguments[i])
				if i < len(want) {
					t.Errorf("   want %#v", want[i])
				}
			}
		}
		t.Fatalf("call arguments = %d, want %d", len(facts.CallArguments), len(want))
	}
	// Every argument call range joins exactly one CallRef; zero-argument and
	// generator-argument calls have refs but no argument entries.
	joined := map[[2]uint32]int{}
	for _, a := range facts.CallArguments {
		joined[[2]uint32{a.CallStartByte, a.CallEndByte}]++
	}
	refs := map[[2]uint32]bool{}
	for _, c := range facts.Calls {
		refs[[2]uint32{c.StartByte, c.EndByte}] = true
	}
	for key := range joined {
		if !refs[key] {
			t.Errorf("argument call range %v has no CallRef", key)
		}
	}
	for _, text := range []string{"top()", "print(v for v in alpha)"} {
		at := pyIndex(t, pyFixture, text)
		key := [2]uint32{uint32(at), uint32(at + len(text))}
		if !refs[key] || joined[key] != 0 {
			t.Errorf("%q: ref=%v arguments=%d", text, refs[key], joined[key])
		}
	}
	// Argument-only selection still joins the same calls.
	only, tree2 := pythonProgram(t, gts.FactCallArguments)
	defer tree2.Release()
	if got := only.Extract(tree2); !reflect.DeepEqual(got.CallArguments, want) || len(got.Calls) != 0 {
		t.Errorf("argument-only selection differs: %d arguments, %d calls", len(got.CallArguments), len(got.Calls))
	}
}

func TestPythonRulesGateMissingGrammarData(t *testing.T) {
	entry := LangEntry{Name: "python", Language: PythonLanguage}
	decls, sigs, args := DeclarationRules(entry), SignatureRules(entry), CallArgumentRules(entry)
	if len(decls) == 0 || len(sigs) == 0 || len(args) == 0 {
		t.Fatalf("Python rows were gated out: %d %d %d", len(decls), len(sigs), len(args))
	}
	for _, d := range decls {
		if d.Kind == "constant" {
			t.Fatalf("Python has no constants: %#v", d)
		}
	}
	// Every Python row set also fails closed on a grammar lacking its names.
	goEntry := LangEntry{Name: "go", Language: GoLanguage}
	for name, got := range map[string]int{
		"declarations": len(gateDeclarationRules(goEntry, declarationRuleTable["python"])),
		"signatures":   len(gateSignatureRules(goEntry, signatureRuleTable["python"])),
		"arguments":    len(gateCallArgumentRules(goEntry, callArgumentRuleTable["python"])),
	} {
		if got != 0 {
			t.Errorf("Go grammar kept %d Python %s rows", got, name)
		}
	}
	// Rows naming a missing node type or field drop individually.
	badDecl := decls[0]
	badDecl.NameWrappers = []string{"absent_wrapper"}
	if got := gateDeclarationRules(entry, []gts.DeclarationRule{badDecl, decls[0]}); len(got) != 1 {
		t.Errorf("declaration gate with missing wrapper kept %d rows", len(got))
	}
	badSig := sigs[0]
	badSig.Parameters = append([]gts.ParameterRule(nil), badSig.Parameters...)
	badSig.Parameters[0].NameNodeTypes = []string{"absent_name"}
	if got := gateSignatureRules(entry, []gts.SignatureRule{badSig, sigs[0]}); len(got) != 1 {
		t.Errorf("signature gate with missing name type kept %d rows", len(got))
	}
	badSig = sigs[0]
	badSig.Parameters = append([]gts.ParameterRule(nil), badSig.Parameters...)
	badSig.Parameters[0].VariadicNodeTypes = []string{"absent_name"}
	if got := gateSignatureRules(entry, []gts.SignatureRule{badSig, sigs[0]}); len(got) != 1 {
		t.Errorf("signature gate with missing variadic type kept %d rows", len(got))
	}
	badArgs := args[0]
	badArgs.SpreadNodeTypes = []string{"absent_spread"}
	if got := gateCallArgumentRules(entry, []gts.CallArgumentRule{badArgs, args[0]}); len(got) != 1 {
		t.Errorf("argument gate with missing spread kept %d rows", len(got))
	}
}
