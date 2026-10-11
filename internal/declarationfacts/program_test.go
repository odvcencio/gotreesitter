package declarationfacts

import "testing"

func testGrammar() Grammar {
	names := []string{"end", "var_spec", "identifier", "source_file"}
	symbols := map[string]uint16{"var_spec": 1, "identifier": 2, "source_file": 3}
	return Grammar{
		Names:  names,
		Symbol: func(name string) (uint16, bool) { symbol, ok := symbols[name]; return symbol, ok },
		Field:  func(name string) (uint16, bool) { return 1, name == "name" },
	}
}

func TestCompileDropsUnresolvedRules(t *testing.T) {
	good := Rule{NodeType: "var_spec", Kind: "variable", NameField: "name", NameNodeType: "identifier"}
	for _, part := range []string{"node", "kind", "name field", "name type", "ancestor", "type field", "type node", "container"} {
		t.Run(part, func(t *testing.T) {
			bad := good
			switch part {
			case "node":
				bad.NodeType = "absent"
			case "kind":
				bad.Kind = ""
			case "name field":
				bad.NameField = "absent"
			case "name type":
				bad.NameNodeType = "absent"
			case "ancestor":
				bad.Ancestors = []string{"absent"}
			case "type field":
				bad.TypeField = "absent"
			case "type node":
				bad.TypeField = "name"
				bad.TypeNodeType = "absent"
			case "container":
				bad.ContainerNameField = "name"
			}
			if got := Compile(testGrammar(), Reader[int]{}, []Rule{bad}); got != nil {
				t.Fatal("unresolved rule compiled")
			}
			if got := Compile(testGrammar(), Reader[int]{}, []Rule{bad, good}); got == nil || len(got.rules[1]) != 1 {
				t.Fatal("valid rule lost or invalid rule retained")
			}
		})
	}
	if got := Compile(testGrammar(), Reader[int]{}, nil); got != nil {
		t.Fatal("empty rules compiled")
	}
}

func TestCompileCopiesAncestorsAndExtractsSiblingNames(t *testing.T) {
	type node struct {
		symbol     uint16
		parent     int
		children   []int
		fields     []uint16
		start, end uint32
	}
	const source = "first, second string"
	nodes := []node{
		{}, {symbol: 3, children: []int{2}, fields: []uint16{0}, end: uint32(len(source))},
		{symbol: 1, parent: 1, children: []int{3, 4, 5}, fields: []uint16{1, 1, 0}, end: uint32(len(source))},
		{symbol: 2, parent: 2, start: 0, end: 5},
		{symbol: 2, parent: 2, start: 7, end: 13},
		// An identifier outside the direct name field must never become a fact.
		{symbol: 2, parent: 2, start: 14, end: 20},
	}
	grammar := testGrammar()
	reader := Reader[int]{
		Language:   "unregistered",
		Symbol:     func(n int) uint16 { return nodes[n].symbol },
		Parent:     func(n int) int { return nodes[n].parent },
		ChildCount: func(n int) int { return len(nodes[n].children) },
		Child:      func(n, i int) int { return nodes[n].children[i] },
		Field:      func(n, i int) uint16 { return nodes[n].fields[i] },
		Missing:    func(int) bool { return false },
		Text:       func(n int, source []byte) string { return string(source[nodes[n].start:nodes[n].end]) },
		NodeType:   func(n int) string { return grammar.Names[nodes[n].symbol] },
		StartByte:  func(n int) uint32 { return nodes[n].start },
		EndByte:    func(n int) uint32 { return nodes[n].end },
	}
	rules := []Rule{{NodeType: "var_spec", Kind: "variable", NameField: "name", NameNodeType: "identifier", Ancestors: []string{"source_file"}}}
	program := Compile(grammar, reader, rules)
	rules[0].Ancestors[0] = "absent"
	var facts []Fact
	program.Extract(1, []byte(source), &facts)
	if len(facts) != 2 {
		t.Fatalf("facts = %#v", facts)
	}
	for i, want := range []Fact{
		{Lang: "unregistered", Kind: "variable", Name: "first", NodeType: "var_spec", EndByte: 20, NameEndByte: 5},
		{Lang: "unregistered", Kind: "variable", Name: "second", NodeType: "var_spec", EndByte: 20, NameStartByte: 7, NameEndByte: 13},
	} {
		if facts[i] != want {
			t.Errorf("fact %d = %#v, want %#v", i, facts[i], want)
		}
	}
	program.Extract(0, []byte(source), &facts)
	(*Program[int])(nil).Extract(1, []byte(source), &facts)
	program.Extract(1, []byte(source), nil)
	if len(facts) != 2 {
		t.Fatal("nil extraction changed facts")
	}
}

func TestCompileNewRuleFieldsFailClosed(t *testing.T) {
	good := Rule{NodeType: "var_spec", Kind: "variable", NameField: "name", NameNodeType: "identifier"}
	for name, mutate := range map[string]func(*Rule){
		"parent field absent":       func(r *Rule) { r.ParentChildField, r.ParentChildNodeType = "absent", "identifier" },
		"parent node absent":        func(r *Rule) { r.ParentChildField, r.ParentChildNodeType = "name", "absent" },
		"parent field only":         func(r *Rule) { r.ParentChildField = "name" },
		"self with other node type": func(r *Rule) { r.NameSelf, r.NameField, r.NameNodeType = true, "", "identifier" },
		"self with name field":      func(r *Rule) { r.NameSelf, r.NameNodeType = true, "var_spec" },
	} {
		t.Run(name, func(t *testing.T) {
			bad := good
			mutate(&bad)
			if got := Compile(testGrammar(), Reader[int]{}, []Rule{bad}); got != nil {
				t.Fatal("invalid rule compiled")
			}
		})
	}
	ok := Rule{NodeType: "var_spec", Kind: "variable", NameSelf: true, NameNodeType: "var_spec", ParentChildField: "name", ParentChildNodeType: "identifier"}
	if got := Compile(testGrammar(), Reader[int]{}, []Rule{ok}); got == nil {
		t.Fatal("valid self rule with a parent requirement failed to compile")
	}
	if !ValidRule(testGrammar(), ok) {
		t.Fatal("ValidRule disagrees with Compile")
	}
}

func TestResolveSymbolsReturnsEveryNamedSymbol(t *testing.T) {
	g := Grammar{
		Names:  []string{"end", "block", "identifier", "block"},
		Symbol: func(name string) (uint16, bool) { return 1, name == "block" },
		Field:  func(string) (uint16, bool) { return 0, false },
	}
	set, ok := ResolveSymbols(g, "block")
	if !ok || len(set) != 2 || !HasSymbol(set, 1) || !HasSymbol(set, 3) {
		t.Fatalf("ResolveSymbols = %v, %v", set, ok)
	}
	if _, ok := ResolveSymbols(g, "absent"); ok {
		t.Fatal("absent name resolved")
	}
	if !HasSymbol(set, 3) || HasSymbol(set, 2) {
		t.Fatal("HasSymbol disagrees with the set")
	}
}

func TestCompileDropsUnresolvedNameWrappers(t *testing.T) {
	rule := Rule{NodeType: "var_spec", Kind: "variable", NameField: "name", NameNodeType: "identifier", NameWrappers: []string{"source_file"}}
	program := Compile(testGrammar(), Reader[int]{}, []Rule{rule})
	rule.NameWrappers[0] = "absent"
	if program == nil || !program.rules[1][0].wrappers[3] {
		t.Fatal("wrapper rows were not compiled and owned")
	}
	if Compile(testGrammar(), Reader[int]{}, []Rule{rule}) != nil {
		t.Fatal("rule with unresolved wrapper compiled")
	}
}

func TestShapeStaysTypesOnly(t *testing.T) {
	for _, kind := range []string{"type", "field", "method"} {
		row := Rule{NodeType: "var_spec", Kind: kind, NameField: "name", NameNodeType: "identifier", Shape: "class"}
		compiled, ok := compileRule(testGrammar(), row)
		if !ok {
			t.Fatal("valid rule rejected")
		}
		want := ""
		if kind == "type" {
			want = "class"
		}
		if compiled.shape != want {
			t.Fatalf("%s shape = %q; want %q", kind, compiled.shape, want)
		}
	}
}
