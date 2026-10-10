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
