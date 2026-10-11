package syntaxfacts

import (
	"github.com/odvcencio/gotreesitter/internal/declarationfacts"
	"testing"
)

func testGrammar() declarationfacts.Grammar {
	names := []string{"end", "function", "identifier", "list", "parameter", "call", "spread"}
	fields := map[string]uint16{"name": 1, "parameters": 2, "type": 3, "arguments": 4}
	return declarationfacts.Grammar{Names: names,
		Symbol: func(name string) (uint16, bool) {
			for i, n := range names {
				if n == name {
					return uint16(i), true
				}
			}
			return 0, false
		},
		Field: func(name string) (uint16, bool) { f, ok := fields[name]; return f, ok }}
}

func TestCompileOwnsRowsAndFailsClosed(t *testing.T) {
	g := testGrammar()
	sig := SignatureRule{NodeType: "function", Kind: "function", NameField: "name", NameNodeType: "identifier", ParametersField: "parameters", ListNodeTypes: []string{"list"}, Parameters: []ParameterRule{{NodeType: "parameter", NameField: "name", NameNodeType: "identifier", TypeField: "type"}}}
	args := CallArgumentRule{NodeType: "call", ArgumentsField: "arguments", ListNodeType: "list", SpreadNodeType: "spread", Kinds: []ExpressionKindRule{{NodeType: "identifier", Kind: "identifier"}}}
	p := CompileSignatures(g, declarationfacts.Reader[int]{}, []SignatureRule{sig})
	a := CompileArguments(g, declarationfacts.Reader[int]{}, []CallArgumentRule{args})
	sig.Parameters[0].TypeField = "absent"
	sig.ListNodeTypes[0] = "absent"
	args.Kinds[0].Kind = "changed"
	if p == nil || len(p.rules[1][0].parameters) != 1 || !p.rules[1][0].lists[3] || a.rules[5].kinds[2] != "identifier" {
		t.Fatal("rules were not owned")
	}
	if CompileSignatures(g, declarationfacts.Reader[int]{}, []SignatureRule{sig}) != nil {
		t.Fatal("unresolved signature compiled")
	}
	args.ArgumentsField = "absent"
	if CompileArguments(g, declarationfacts.Reader[int]{}, []CallArgumentRule{args}) != nil {
		t.Fatal("unresolved arguments compiled")
	}
	if CompileSignatures(g, declarationfacts.Reader[int]{}, nil) != nil || CompileArguments(g, declarationfacts.Reader[int]{}, nil) != nil {
		t.Fatal("empty rules compiled")
	}
	var signatures []SignatureFact
	var arguments []CallArgumentFact
	p.Extract(0, nil, &signatures)
	a.Extract(0, nil, nil, &arguments)
	(*Signatures[int])(nil).Extract(1, nil, &signatures)
	(*Arguments[int])(nil).Extract(1, nil, nil, &arguments)
	p.Extract(1, nil, nil)
	a.Extract(1, nil, nil, nil)
	if len(signatures) != 0 || len(arguments) != 0 {
		t.Fatal("nil extraction changed facts")
	}
}
