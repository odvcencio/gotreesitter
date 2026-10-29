package grammars

import gts "github.com/odvcencio/gotreesitter"

// TypeScript and TSX share node types and field names for every shape below,
// so one set of rows serves both grammars. Each row still resolves against the
// entry's own grammar and is dropped when any symbol or field is missing.
func init() {
	for _, grammar := range []struct {
		name           string
		angleAssertion bool
	}{
		{"typescript", true},
		// TSX has no angle-bracket assertion (<T>x), because it conflicts with
		// JSX; a row naming it would fail closed on that grammar.
		{"tsx", false},
	} {
		declarationRuleTable[grammar.name] = tsDeclarationRules()
		signatureRuleTable[grammar.name] = tsSignatureRules()
		callArgumentRuleTable[grammar.name] = tsCallArgumentRules(grammar.angleAssertion)
	}
}

// tsScopes lists the parent chains of a program-level declaration, nearest
// first: bare, exported, ambient ("declare"), and exported ambient. Anything
// else (function bodies, namespaces, blocks) is local and excluded.
var tsScopes = [][]string{
	{"program"},
	{"export_statement", "program"},
	{"ambient_declaration", "program"},
	{"ambient_declaration", "export_statement", "program"},
}

// tsOwners are the named bodies whose members receive a Container.
var tsOwners = []struct{ node, name string }{
	{"class_declaration", "type_identifier"},
	{"abstract_class_declaration", "type_identifier"},
	{"interface_declaration", "type_identifier"},
	{"enum_declaration", "identifier"},
}

// tsScopedContainers names only program-level owners, so members of local and
// namespaced types are not declarations.
func tsScopedContainers() []gts.ContainerRule {
	var rules []gts.ContainerRule
	for _, scope := range tsScopes {
		for _, o := range tsOwners {
			rules = append(rules, gts.ContainerRule{NodeType: o.node, NameField: "name", NameNodeType: o.name, TypeField: "body", Ancestors: scope})
		}
	}
	return rules
}

// tsAnyContainers names an owner wherever it appears, for signatures.
func tsAnyContainers() []gts.ContainerRule {
	var rules []gts.ContainerRule
	for _, o := range tsOwners {
		rules = append(rules, gts.ContainerRule{NodeType: o.node, NameField: "name", NameNodeType: o.name, TypeField: "body"})
	}
	return rules
}

var tsTypeNameRules = []gts.TypeNameRule{
	{NodeType: "type_identifier", Leaf: true},
	{NodeType: "nested_type_identifier", Field: "name"},
	{NodeType: "generic_type", Field: "name"},
}

func tsDeclarationRules() []gts.DeclarationRule {
	var rules []gts.DeclarationRule
	for _, scope := range tsScopes {
		for _, t := range []struct{ node, name, shape string }{
			{"class_declaration", "type_identifier", "class"},
			{"abstract_class_declaration", "type_identifier", "class"},
			{"interface_declaration", "type_identifier", "interface"},
			{"type_alias_declaration", "type_identifier", "alias"},
			{"enum_declaration", "identifier", "enum"},
		} {
			rules = append(rules, gts.DeclarationRule{NodeType: t.node, Kind: "type", NameField: "name", NameNodeType: t.name, Shape: t.shape, Ancestors: scope})
		}

	}
	rules = append(rules, javascriptBindingRows(tsScopes)...)
	owners := tsScopedContainers()
	rules = append(rules, javascriptMemberRows("public_field_definition", "name", owners)...)
	for _, name := range []string{"property_identifier", "private_property_identifier"} {
		rules = append(rules,
			gts.DeclarationRule{NodeType: "method_signature", Kind: "method", NameField: "name", NameNodeType: name, Ancestors: []string{"class_body"}, ContainerPath: owners},
			gts.DeclarationRule{NodeType: "abstract_method_signature", Kind: "method", NameField: "name", NameNodeType: name, Ancestors: []string{"class_body"}, ContainerPath: owners},
		)
	}
	rules = append(rules,
		gts.DeclarationRule{NodeType: "property_signature", Kind: "field", NameField: "name", NameNodeType: "property_identifier", Ancestors: []string{"interface_body"}, ContainerPath: owners},
		gts.DeclarationRule{NodeType: "method_signature", Kind: "method", NameField: "name", NameNodeType: "property_identifier", Ancestors: []string{"interface_body"}, ContainerPath: owners},
		// Enum members are constants. A bare member is its own name node.
		gts.DeclarationRule{NodeType: "enum_assignment", Kind: "constant", NameField: "name", NameNodeType: "property_identifier", Ancestors: []string{"enum_body"}, ContainerPath: owners},
		gts.DeclarationRule{NodeType: "property_identifier", Kind: "constant", NameSelf: true, NameNodeType: "property_identifier", Ancestors: []string{"enum_body"}, ContainerPath: owners},
	)
	// An interface's extended names are embeddings whose container is the
	// interface itself.
	var selfOwners []gts.ContainerRule
	for _, scope := range tsScopes {
		selfOwners = append(selfOwners, gts.ContainerRule{NodeType: "interface_declaration", NameField: "name", NameNodeType: "type_identifier", Ancestors: scope, Self: true})
	}
	rules = append(rules, gts.DeclarationRule{NodeType: "extends_type_clause", Kind: "embed", BaseName: true, BaseNameField: "type", TypeNames: tsTypeNameRules,
		Ancestors: []string{"interface_declaration"}, ContainerPath: selfOwners})
	return rules
}

var tsParameterRules = []gts.ParameterRule{
	{NodeType: "identifier", NameSelf: true},
	{NodeType: "required_parameter", NameField: "pattern", NameNodeType: "identifier", TypeField: "type", TypeOptional: true, VariadicNodeTypes: []string{"rest_pattern"}, UnwrapVariadic: true, AnonymousPatterns: []string{"this", "object_pattern", "array_pattern"}},
	{NodeType: "optional_parameter", NameField: "pattern", NameNodeType: "identifier", TypeField: "type", TypeOptional: true, Optional: true, VariadicNodeTypes: []string{"rest_pattern"}, UnwrapVariadic: true, AnonymousPatterns: []string{"this", "object_pattern", "array_pattern"}},
	{NodeType: "type_parameter", NameField: "name", NameNodeType: "type_identifier", TypeField: "constraint", TypeOptional: true},
}

func tsSignature(node, kind, nameType string) gts.SignatureRule {
	row := javascriptSignature(node, kind, nameType, tsParameterRules)
	row.TypeParametersField, row.ResultsField = "type_parameters", "return_type"
	row.ListNodeTypes = []string{"formal_parameters", "type_parameters"}
	row.TypeWrapperNodeTypes = []string{"type_annotation", "constraint"}
	return row
}

func tsSignatureRules() []gts.SignatureRule {
	var rules []gts.SignatureRule
	// Named functions, generators and overload or ambient signatures.
	for _, node := range []string{"function_declaration", "generator_function_declaration", "function_signature"} {
		rules = append(rules, tsSignature(node, "function", "identifier"))
	}
	// Methods of classes, named wherever the class appears.
	owners := tsAnyContainers()
	for _, name := range []string{"property_identifier", "private_property_identifier"} {
		for _, node := range []string{"method_definition", "method_signature", "abstract_method_signature"} {
			row := tsSignature(node, "method", name)
			row.Ancestors, row.ContainerPath = []string{"class_body"}, owners
			rules = append(rules, row)
		}
	}
	row := tsSignature("method_signature", "method", "property_identifier")
	row.Ancestors, row.ContainerPath = []string{"interface_body"}, owners
	rules = append(rules, row)
	// Function values bound by a program-level const. The fact spans the
	// declarator and reads its signature from the bound value.
	rules = append(rules, javascriptBoundSignatures(tsScopes, tsSignature("variable_declarator", "function", "identifier"))...)
	return rules
}

func tsCallArgumentRules(angleAssertion bool) []gts.CallArgumentRule {
	kinds := append([]gts.ExpressionKindRule(nil), javascriptExpressionKinds...)
	kinds = append(kinds,
		gts.ExpressionKindRule{NodeType: "this", Kind: "identifier"},
		gts.ExpressionKindRule{NodeType: "new_expression", Kind: "call"},
		gts.ExpressionKindRule{NodeType: "update_expression", Kind: "unary"},
		gts.ExpressionKindRule{NodeType: "as_expression", Kind: "type_assertion"},
		gts.ExpressionKindRule{NodeType: "satisfies_expression", Kind: "type_assertion"})
	if angleAssertion {
		kinds = append(kinds, gts.ExpressionKindRule{NodeType: "type_assertion", Kind: "type_assertion"})
	}
	return javascriptCallArgumentRows(kinds)
}
