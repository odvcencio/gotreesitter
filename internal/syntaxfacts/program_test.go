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

func TestCompileNewSignatureFieldsFailClosed(t *testing.T) {
	g := testGrammar()
	good := SignatureRule{NodeType: "function", Kind: "function", NameField: "name", NameNodeType: "identifier", ParametersField: "parameters", ListNodeTypes: []string{"list"},
		Parameters: []ParameterRule{{NodeType: "parameter", NameField: "name", NameNodeType: "identifier", TypeField: "type"}}}
	if !ValidSignatureRule(g, good) {
		t.Fatal("base rule invalid")
	}
	for name, mutate := range map[string]func(*SignatureRule){
		"wrapper absent":        func(r *SignatureRule) { r.TypeWrapperNodeTypes = []string{"absent"} },
		"delegate without type": func(r *SignatureRule) { r.DelegateField = "name" },
		"delegate type only":    func(r *SignatureRule) { r.DelegateNodeTypes = []string{"function"} },
		"delegate field absent": func(r *SignatureRule) { r.DelegateField, r.DelegateNodeTypes = "absent", []string{"function"} },
		"delegate node absent":  func(r *SignatureRule) { r.DelegateField, r.DelegateNodeTypes = "name", []string{"absent"} },
		"single field absent":   func(r *SignatureRule) { r.SingleParameterField = "absent" },
		"parent field absent":   func(r *SignatureRule) { r.ParentChildField, r.ParentChildNodeType = "absent", "identifier" },
		"parent node absent":    func(r *SignatureRule) { r.ParentChildField, r.ParentChildNodeType = "name", "absent" },
		"variadic wrapper":      func(r *SignatureRule) { r.Parameters[0].VariadicNodeTypes = []string{"absent"} },
		"anonymous pattern":     func(r *SignatureRule) { r.Parameters[0].AnonymousPatterns = []string{"absent"} },
	} {
		t.Run(name, func(t *testing.T) {
			bad := good
			bad.Parameters = append([]ParameterRule(nil), good.Parameters...)
			mutate(&bad)
			if ValidSignatureRule(g, bad) {
				t.Fatal("invalid rule compiled")
			}
		})
	}
	ok := good
	ok.Parameters = []ParameterRule{{NodeType: "parameter", NameField: "name", NameNodeType: "identifier", TypeField: "type", TypeOptional: true, Optional: true, VariadicNodeTypes: []string{"spread"}, UnwrapVariadic: true, AnonymousPatterns: []string{"call"}}}
	ok.TypeWrapperNodeTypes, ok.DelegateField, ok.DelegateNodeTypes, ok.SingleParameterField = []string{"list"}, "name", []string{"function"}, "type"
	if !ValidSignatureRule(g, ok) {
		t.Fatal("valid extended rule failed to compile")
	}
}

func TestCompileParameterAndSpreadExtensionsFailClosed(t *testing.T) {
	g := testGrammar()
	good := SignatureRule{NodeType: "function", Kind: "function", NameField: "name", NameNodeType: "identifier", ParametersField: "parameters", ListNodeTypes: []string{"list"},
		Parameters: []ParameterRule{{NodeType: "identifier", NameSelf: true}, {NodeType: "parameter", NameNodeType: "identifier", NameNodeTypes: []string{"spread"}, VariadicNodeTypes: []string{"spread"}}}}
	if !ValidSignatureRule(g, good) {
		t.Fatal("untyped, unfielded parameter rows were rejected")
	}
	for name, mutate := range map[string]func(*SignatureRule){
		"name node with a field":    func(r *SignatureRule) { r.Parameters[0].NameField = "name" },
		"name node with a type":     func(r *SignatureRule) { r.Parameters[0].TypeField = "type" },
		"name node with extra type": func(r *SignatureRule) { r.Parameters[0].NameNodeTypes = []string{"spread"} },
		"absent name type":          func(r *SignatureRule) { r.Parameters[1].NameNodeTypes = []string{"absent"} },
		"absent variadic type":      func(r *SignatureRule) { r.Parameters[1].VariadicNodeTypes = []string{"absent"} },
	} {
		bad := good
		bad.Parameters = append([]ParameterRule(nil), good.Parameters...)
		mutate(&bad)
		if ValidSignatureRule(g, bad) {
			t.Errorf("%s: rule compiled", name)
		}
	}
	args := CallArgumentRule{NodeType: "call", ArgumentsField: "arguments", ListNodeType: "list", SpreadNodeType: "spread", SpreadNodeTypes: []string{"list"}}
	a := CompileArguments(g, declarationfacts.Reader[int]{}, []CallArgumentRule{args})
	args.SpreadNodeTypes[0] = "absent"
	if a == nil || !a.rules[5].spread[6] || !a.rules[5].spread[3] {
		t.Fatal("spread node types were not compiled and owned")
	}
	if ValidCallArgumentRule(g, args) {
		t.Fatal("unresolved spread node type compiled")
	}
}

func TestCompileUntypedParametersAndValueRows(t *testing.T) {
	g := testGrammar()
	good := SignatureRule{NodeType: "function", Kind: "function", NameField: "name", NameNodeType: "identifier", DelegateField: "type", DelegateNodeTypes: []string{"function"},
		ParametersField: "parameters", SingleParameterField: "name", ListNodeTypes: []string{"list"}, RequireContainer: true,
		Parameters: []ParameterRule{{NodeType: "identifier", NameSelf: true}, {NodeType: "parameter", NameNodeType: "identifier", Variadic: true}}}
	p := CompileSignatures(g, declarationfacts.Reader[int]{}, []SignatureRule{good})
	if p == nil {
		t.Fatal("untyped value row did not compile")
	}
	c := p.rules[1][0]
	if len(c.parameters[2]) != 1 || !c.parameters[2][0].self || c.parameters[2][0].typ != 0 || !c.parameters[4][0].variadic || c.delegate == 0 || c.single == 0 || !c.requireContainer {
		t.Fatalf("compiled row = %#v", c)
	}
	for name, mutate := range map[string]func(*SignatureRule){
		"value field without node": func(r *SignatureRule) { r.DelegateNodeTypes = nil },
		"value node without field": func(r *SignatureRule) { r.DelegateField = "" },
		"absent value node":        func(r *SignatureRule) { r.DelegateNodeTypes = []string{"absent"} },
		"absent alt field":         func(r *SignatureRule) { r.SingleParameterField = "absent" },
		"absent parent token":      func(r *SignatureRule) { r.ParentChildField, r.ParentChildNodeType = "name", "absent" },
		"self name with name field": func(r *SignatureRule) {
			r.Parameters = []ParameterRule{{NodeType: "identifier", NameSelf: true, NameField: "name", NameNodeType: "identifier"}}
		},
		"absent type field": func(r *SignatureRule) {
			r.Parameters = []ParameterRule{{NodeType: "parameter", NameNodeType: "identifier", TypeField: "absent"}}
		},
	} {
		bad := good
		mutate(&bad)
		if ValidSignatureRule(g, bad) {
			t.Errorf("%s: invalid row compiled", name)
		}
	}
	standalone := CallArgumentRule{NodeType: "call", ArgumentsField: "arguments", ListNodeType: "list", Standalone: true}
	if a := CompileArguments(g, declarationfacts.Reader[int]{}, []CallArgumentRule{standalone}); a == nil || !a.rules[5].standalone {
		t.Fatal("standalone flag lost")
	}
}
