package grammars

import gts "github.com/odvcencio/gotreesitter"

// JavaScript fact rows. Only program-level and exported bindings, classes and
// their members are described; locals inside function bodies match no row
// because every row names its exact ancestor chain.

// javascriptTopLevel lists the parent chains (nearest first) of a program-level
// statement: bare, or wrapped by an export statement.
var javascriptTopLevel = [][]string{{"program"}, {"export_statement", "program"}}

func javascriptChain(head string, tail []string) []string {
	return append([]string{head}, tail...)
}

var javascriptClassContainers = []gts.ContainerRule{
	{NodeType: "class_declaration", NameField: "name", NameNodeType: "identifier", TypeField: "body", Ancestors: []string{"program"}},
	{NodeType: "class_declaration", NameField: "name", NameNodeType: "identifier", TypeField: "body", Ancestors: []string{"export_statement", "program"}},
}

var javascriptMemberNames = []string{"property_identifier", "private_property_identifier"}

func javascriptBindingRows(scopes [][]string) []gts.DeclarationRule {
	var rows []gts.DeclarationRule
	for _, top := range scopes {
		for _, b := range []struct{ node, kind, token string }{{"lexical_declaration", "constant", "const"}, {"lexical_declaration", "variable", "let"}, {"variable_declaration", "variable", ""}} {
			row := gts.DeclarationRule{NodeType: "variable_declarator", Kind: b.kind, NameField: "name", NameNodeType: "identifier", Ancestors: javascriptChain(b.node, top)}
			if b.token != "" {
				row.ParentChildField, row.ParentChildNodeType = "kind", b.token
			}
			rows = append(rows, row)
		}
	}
	return rows
}

func javascriptMemberRows(fieldNode, fieldName string, owners []gts.ContainerRule) []gts.DeclarationRule {
	var rows []gts.DeclarationRule
	for _, name := range javascriptMemberNames {
		rows = append(rows,
			gts.DeclarationRule{NodeType: fieldNode, Kind: "field", NameField: fieldName, NameNodeType: name, Ancestors: []string{"class_body"}, ContainerPath: owners},
			gts.DeclarationRule{NodeType: "method_definition", Kind: "method", NameField: "name", NameNodeType: name, Ancestors: []string{"class_body"}, ContainerPath: owners})
	}
	return rows
}

func javascriptDeclarationRows() []gts.DeclarationRule {
	rows := javascriptBindingRows(javascriptTopLevel)
	for _, top := range javascriptTopLevel {
		rows = append(rows, gts.DeclarationRule{NodeType: "class_declaration", Kind: "type", Shape: "class", NameField: "name", NameNodeType: "identifier", Ancestors: top})
	}
	return append(rows, javascriptMemberRows("field_definition", "property", javascriptClassContainers)...)
}

// javascriptParameterRules keep the position of destructured parameters by
// naming them with their pattern text. Default values are not recorded.
var javascriptParameterRules = []gts.ParameterRule{
	{NodeType: "identifier", NameSelf: true},
	{NodeType: "object_pattern", NameSelf: true},
	{NodeType: "array_pattern", NameSelf: true},
	{NodeType: "assignment_pattern", NameField: "left", NameNodeType: "identifier"},
	{NodeType: "assignment_pattern", NameField: "left", NameNodeType: "object_pattern"},
	{NodeType: "assignment_pattern", NameField: "left", NameNodeType: "array_pattern"},
	{NodeType: "rest_pattern", NameNodeType: "identifier", Variadic: true},
	{NodeType: "rest_pattern", NameNodeType: "object_pattern", Variadic: true},
	{NodeType: "rest_pattern", NameNodeType: "array_pattern", Variadic: true},
}

func javascriptSignature(node, kind, nameType string, params []gts.ParameterRule) gts.SignatureRule {
	return gts.SignatureRule{NodeType: node, Kind: kind, NameField: "name", NameNodeType: nameType, ParametersField: "parameters", ListNodeTypes: []string{"formal_parameters"}, Parameters: params}
}

func javascriptBoundSignatures(scopes [][]string, row gts.SignatureRule) []gts.SignatureRule {
	var rows []gts.SignatureRule
	for _, top := range scopes {
		r := row
		r.Ancestors = javascriptChain("lexical_declaration", top)
		r.ParentChildField, r.ParentChildNodeType = "kind", "const"
		r.DelegateField, r.DelegateNodeTypes = "value", []string{"arrow_function", "function_expression", "generator_function"}
		r.SingleParameterField = "parameter"
		rows = append(rows, r)
	}
	return rows
}

func javascriptSignatureRows() []gts.SignatureRule {
	var rows []gts.SignatureRule
	for _, top := range javascriptTopLevel {
		for _, node := range []string{"function_declaration", "generator_function_declaration"} {
			row := javascriptSignature(node, "function", "identifier", javascriptParameterRules)
			row.Ancestors = top
			rows = append(rows, row)
		}
	}
	rows = append(rows, javascriptBoundSignatures(javascriptTopLevel, javascriptSignature("variable_declarator", "function", "identifier", javascriptParameterRules))...)
	for _, name := range javascriptMemberNames {
		row := javascriptSignature("method_definition", "method", name, javascriptParameterRules)
		row.Ancestors, row.ContainerPath, row.RequireContainer = []string{"class_body"}, javascriptClassContainers, true
		rows = append(rows, row)
	}
	return rows
}

var javascriptExpressionKinds = []gts.ExpressionKindRule{
	{NodeType: "identifier", Kind: "identifier"},
	{NodeType: "member_expression", Kind: "selector"},
	{NodeType: "call_expression", Kind: "call"},
	{NodeType: "object", Kind: "composite_literal"}, {NodeType: "array", Kind: "composite_literal"},
	{NodeType: "arrow_function", Kind: "func_literal"}, {NodeType: "function_expression", Kind: "func_literal"}, {NodeType: "generator_function", Kind: "func_literal"},
	{NodeType: "number", Kind: "literal"}, {NodeType: "string", Kind: "literal"}, {NodeType: "template_string", Kind: "literal"}, {NodeType: "regex", Kind: "literal"},
	{NodeType: "true", Kind: "literal"}, {NodeType: "false", Kind: "literal"}, {NodeType: "null", Kind: "literal"}, {NodeType: "undefined", Kind: "literal"},
	{NodeType: "unary_expression", Kind: "unary"}, {NodeType: "binary_expression", Kind: "binary"},
	{NodeType: "subscript_expression", Kind: "index"}, {NodeType: "parenthesized_expression", Kind: "parenthesized"},
}

// New-expression arguments are standalone: JavaScript's CallRef extraction
// does not report constructor calls, so those arguments have no CallRef to join.
func javascriptCallArgumentRows(kinds []gts.ExpressionKindRule) []gts.CallArgumentRule {
	return []gts.CallArgumentRule{
		{NodeType: "call_expression", ArgumentsField: "arguments", ListNodeType: "arguments", SpreadNodeType: "spread_element", Kinds: kinds},
		{NodeType: "new_expression", ArgumentsField: "arguments", ListNodeType: "arguments", SpreadNodeType: "spread_element", Kinds: kinds, Standalone: true},
	}
}
