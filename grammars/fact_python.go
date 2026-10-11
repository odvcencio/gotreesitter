package grammars

import gts "github.com/odvcencio/gotreesitter"

// Python rows. Python has no constants, so declarations never emit that kind.
// Scope is exact: bindings at module or class-body level only. Locals,
// conditional (if/try) blocks and function-nested classes produce no
// declaration facts.

// pythonPatternWrappers hold several bindings on the left of an assignment.
var pythonPatternWrappers = []string{"pattern_list", "tuple_pattern", "list_pattern", "list_splat_pattern"}

// pythonContainerRules name the class that owns a block. Nested classes
// extend the path with dots, as anonymous Go owners do. Classes inside
// function bodies resolve no owner.
var pythonContainerRules = []gts.ContainerRule{
	{NodeType: "class_definition", NameField: "name", NameNodeType: "identifier", TypeField: "body", Ancestors: []string{"module"}},
	{NodeType: "class_definition", NameField: "name", NameNodeType: "identifier", TypeField: "body", Ancestors: []string{"decorated_definition", "module"}},
	{NodeType: "class_definition", NameField: "name", NameNodeType: "identifier", TypeField: "body", ParentPath: []string{"block"}},
	{NodeType: "class_definition", NameField: "name", NameNodeType: "identifier", TypeField: "body", ParentPath: []string{"decorated_definition", "block"}},
}

func pythonDeclarationRules() []gts.DeclarationRule {
	var rules []gts.DeclarationRule
	// A chained assignment `a = b = c` nests the later targets in `right`.
	// Chains of up to three targets resolve.
	for depth := 0; depth <= 2; depth++ {
		chain := make([]string, depth)
		for i := range chain {
			chain[i] = "assignment"
		}
		rules = append(rules,
			gts.DeclarationRule{NodeType: "assignment", Kind: "variable", NameField: "left", NameNodeType: "identifier", NameWrappers: pythonPatternWrappers, Ancestors: append(append([]string(nil), chain...), "module")},
			gts.DeclarationRule{NodeType: "assignment", Kind: "field", NameField: "left", NameNodeType: "identifier", NameWrappers: pythonPatternWrappers, Ancestors: append(append([]string(nil), chain...), "block"), ContainerPath: pythonContainerRules})
	}
	for _, parent := range []string{"", "decorated_definition"} {
		var top, nested []string
		if parent != "" {
			top, nested = []string{parent, "module"}, []string{parent, "block"}
		} else {
			top, nested = []string{"module"}, []string{"block"}
		}
		rules = append(rules,
			gts.DeclarationRule{NodeType: "class_definition", Kind: "type", Shape: "class", NameField: "name", NameNodeType: "identifier", Ancestors: top},
			gts.DeclarationRule{NodeType: "class_definition", Kind: "type", Shape: "class", NameField: "name", NameNodeType: "identifier", Ancestors: nested, ContainerPath: pythonContainerRules},
			gts.DeclarationRule{NodeType: "function_definition", Kind: "method", NameField: "name", NameNodeType: "identifier", Ancestors: nested, ContainerPath: pythonContainerRules})
	}
	return rules
}

var pythonSplatNames = []string{"list_splat_pattern", "dictionary_splat_pattern"}

var pythonParameterRules = []gts.ParameterRule{
	{NodeType: "identifier", NameSelf: true},
	{NodeType: "typed_parameter", NameNodeType: "identifier", NameNodeTypes: pythonSplatNames, VariadicNodeTypes: pythonSplatNames, TypeField: "type"},
	{NodeType: "default_parameter", NameField: "name", NameNodeType: "identifier"},
	{NodeType: "typed_default_parameter", NameField: "name", NameNodeType: "identifier", TypeField: "type"},
	// A bare splat is variadic; its name keeps the stars the grammar gives it.
	{NodeType: "list_splat_pattern", NameSelf: true, Variadic: true},
	{NodeType: "dictionary_splat_pattern", NameSelf: true, Variadic: true},
}

func pythonSignatureRules() []gts.SignatureRule {
	row := func(kind string, ancestors []string, methods bool) gts.SignatureRule {
		r := gts.SignatureRule{NodeType: "function_definition", Kind: kind, NameField: "name", NameNodeType: "identifier",
			ParametersField: "parameters", ResultsField: "return_type", ListNodeTypes: []string{"parameters"},
			Parameters: pythonParameterRules, Ancestors: ancestors}
		if methods {
			r.ContainerPath, r.RequireContainer = pythonContainerRules, true
		}
		return r
	}
	return []gts.SignatureRule{
		row("method", []string{"decorated_definition", "block"}, true),
		row("method", []string{"block"}, true),
		row("function", nil, false),
	}
}

func pythonCallArgumentRules() []gts.CallArgumentRule {
	kinds := map[string][]string{
		"identifier":        {"identifier"},
		"selector":          {"attribute"},
		"call":              {"call"},
		"literal":           {"integer", "float", "string", "concatenated_string", "none", "true", "false", "ellipsis"},
		"composite_literal": {"list", "dictionary", "set", "tuple"},
		"func_literal":      {"lambda"},
		"unary":             {"unary_operator", "not_operator"},
		"binary":            {"binary_operator", "boolean_operator", "comparison_operator"},
		"index":             {"subscript"},
		"parenthesized":     {"parenthesized_expression"},
		"keyword":           {"keyword_argument"},
	}
	var rows []gts.ExpressionKindRule
	for _, kind := range []string{"identifier", "selector", "call", "literal", "composite_literal", "func_literal", "unary", "binary", "index", "parenthesized", "keyword"} {
		for _, node := range kinds[kind] {
			rows = append(rows, gts.ExpressionKindRule{NodeType: node, Kind: kind})
		}
	}
	return []gts.CallArgumentRule{{NodeType: "call", ArgumentsField: "arguments", ListNodeType: "argument_list",
		SpreadNodeType: "list_splat", SpreadNodeTypes: []string{"dictionary_splat"}, Kinds: rows}}
}
