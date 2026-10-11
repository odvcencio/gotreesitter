package grammars

import (
	"strings"

	gts "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/internal/declarationfacts"
	"github.com/odvcencio/gotreesitter/internal/syntaxfacts"
)

// declarationRuleTable belongs to the grammars package, not the extractor.
// Exact ownership paths omit local bindings and type-set terms.
var declarationRuleTable = map[string][]gts.DeclarationRule{
	"go": {
		{NodeType: "type_spec", Kind: "type", NameField: "name", NameNodeType: "type_identifier", TypeField: "type", TypeNodeType: "struct_type", Shape: "struct"},
		{NodeType: "type_spec", Kind: "type", NameField: "name", NameNodeType: "type_identifier", TypeField: "type", TypeNodeType: "interface_type", Shape: "interface"},
		{NodeType: "type_spec", Kind: "type", NameField: "name", NameNodeType: "type_identifier"},
		{NodeType: "type_alias", Kind: "type", NameField: "name", NameNodeType: "type_identifier", Shape: "alias"},
		{NodeType: "var_spec", Kind: "variable", NameField: "name", NameNodeType: "identifier", Ancestors: []string{"var_declaration", "source_file"}},
		{NodeType: "var_spec", Kind: "variable", NameField: "name", NameNodeType: "identifier", Ancestors: []string{"var_spec_list", "var_declaration", "source_file"}},
		{NodeType: "const_spec", Kind: "constant", NameField: "name", NameNodeType: "identifier", Ancestors: []string{"const_declaration", "source_file"}},
		{NodeType: "field_declaration", Kind: "field", NameField: "name", NameNodeType: "field_identifier", Ancestors: []string{"field_declaration_list", "struct_type"}, ContainerPath: goContainerRules},
		{NodeType: "field_declaration", Kind: "field", NameField: "name", BaseName: true, BaseNameField: "type", Embedded: true, TypeNames: goTypeNameRules, Ancestors: []string{"field_declaration_list", "struct_type"}, ContainerPath: goContainerRules},
		{NodeType: "type_elem", Kind: "embed", BaseName: true, TypeNames: goTypeNameRules[:3], ExcludeNames: []string{"comparable", "any", "bool", "byte", "rune", "string", "int", "int8", "int16", "int32", "int64", "uint", "uint8", "uint16", "uint32", "uint64", "uintptr", "float32", "float64", "complex64", "complex128"}, Ancestors: []string{"interface_type"}, ContainerPath: goContainerRules},
		{NodeType: "method_elem", Kind: "method", NameField: "name", NameNodeType: "field_identifier", Ancestors: []string{"interface_type"}, ContainerPath: goContainerRules},
	},
}

// DeclarationRules returns rules resolved against entry's own grammar symbols
// and fields. Unsupported languages and unresolved rows return no rules.
// Compose with NewFactProgramWithOptions(lang, FactDefinitions|FactDeclarations,
// WithDeclarationRules(grammars.DeclarationRules(entry))).
func DeclarationRules(entry LangEntry) []gts.DeclarationRule {
	return gateDeclarationRules(entry, declarationRuleTable[strings.TrimSpace(entry.Name)])
}

func gateDeclarationRules(entry LangEntry, candidates []gts.DeclarationRule) []gts.DeclarationRule {
	if len(candidates) == 0 || entry.Language == nil {
		return nil
	}
	lang := entry.Language()
	if lang == nil {
		return nil
	}
	var rules []gts.DeclarationRule
	g := factGrammar(lang)
	for _, rule := range candidates {
		if !declarationfacts.ValidRule(g, rule) {
			continue
		}
		rule.Ancestors = append([]string(nil), rule.Ancestors...)
		rule.TypeNames = append([]gts.TypeNameRule(nil), rule.TypeNames...)
		rule.ExcludeNames = append([]string(nil), rule.ExcludeNames...)
		rule.ContainerPath = copyContainerRules(rule.ContainerPath)
		rules = append(rules, rule)
	}
	return rules
}

var goTypeNameRules = []gts.TypeNameRule{
	{NodeType: "type_identifier", Leaf: true},
	{NodeType: "qualified_type", Field: "name"},
	{NodeType: "generic_type", Field: "type"},
	{NodeType: "pointer_type"},
}

// Receiver types permit parentheses; embedded struct fields do not.
var goReceiverTypeNameRules = append(
	append([]gts.TypeNameRule(nil), goTypeNameRules...),
	gts.TypeNameRule{NodeType: "parenthesized_type"},
)

var goBodyWrappers = []gts.TypeNameRule{
	{NodeType: "pointer_type"}, {NodeType: "parenthesized_type"},
	{NodeType: "array_type", Field: "element"}, {NodeType: "slice_type", Field: "element"},
	{NodeType: "map_type", Field: "value"}, {NodeType: "channel_type", Field: "value"},
}

var goContainerRules = []gts.ContainerRule{
	{NodeType: "type_spec", NameField: "name", NameNodeType: "type_identifier", TypeField: "type", BodyWrappers: goBodyWrappers},
	{NodeType: "type_alias", NameField: "name", NameNodeType: "type_identifier", TypeField: "type", BodyWrappers: goBodyWrappers},
	{NodeType: "field_declaration", NameField: "name", NameNodeType: "field_identifier", TypeField: "type", BodyWrappers: goBodyWrappers, ParentPath: []string{"field_declaration_list", "struct_type"}},
	{NodeType: "var_spec", NameField: "name", NameNodeType: "identifier", TypeField: "type", BodyWrappers: goBodyWrappers, Ancestors: []string{"var_declaration", "source_file"}},
	{NodeType: "var_spec", NameField: "name", NameNodeType: "identifier", TypeField: "type", BodyWrappers: goBodyWrappers, Ancestors: []string{"var_spec_list", "var_declaration", "source_file"}},
}

func factGrammar(lang *gts.Language) declarationfacts.Grammar {
	return declarationfacts.Grammar{Names: lang.SymbolNames,
		Symbol: func(name string) (uint16, bool) { s, ok := lang.SymbolByName(name); return uint16(s), ok },
		Field:  func(name string) (uint16, bool) { f, ok := lang.FieldByName(name); return uint16(f), ok }}
}

func copyContainerRules(rows []gts.ContainerRule) []gts.ContainerRule {
	rows = append([]gts.ContainerRule(nil), rows...)
	for i := range rows {
		rows[i].BodyWrappers = append([]gts.TypeNameRule(nil), rows[i].BodyWrappers...)
		rows[i].Ancestors = append([]string(nil), rows[i].Ancestors...)
		rows[i].ParentPath = append([]string(nil), rows[i].ParentPath...)
	}
	return rows
}

var goParameterRules = []gts.ParameterRule{
	{NodeType: "parameter_declaration", NameField: "name", NameNodeType: "identifier", TypeField: "type"},
	{NodeType: "variadic_parameter_declaration", NameField: "name", NameNodeType: "identifier", TypeField: "type", Variadic: true},
	{NodeType: "type_parameter_declaration", NameField: "name", NameNodeType: "identifier", TypeField: "type"},
}

var signatureRuleTable = map[string][]gts.SignatureRule{
	"go": {
		{NodeType: "function_declaration", Kind: "function", NameField: "name", NameNodeType: "identifier", TypeParametersField: "type_parameters", ParametersField: "parameters", ResultsField: "result", ListNodeTypes: []string{"parameter_list", "type_parameter_list"}, Parameters: goParameterRules},
		{NodeType: "method_declaration", Kind: "method", NameField: "name", NameNodeType: "field_identifier", ReceiverField: "receiver", ParametersField: "parameters", ResultsField: "result", PointerNodeType: "pointer_type", ListNodeTypes: []string{"parameter_list"}, Parameters: goParameterRules, TypeNames: goReceiverTypeNameRules},
		{NodeType: "method_elem", Kind: "method", NameField: "name", NameNodeType: "field_identifier", ParametersField: "parameters", ResultsField: "result", ListNodeTypes: []string{"parameter_list"}, Parameters: goParameterRules, Ancestors: []string{"interface_type"}, ContainerPath: goContainerRules},
	},
}

// SignatureRules returns grammar-gated rows for declared Go functions, receiver
// methods and interface methods. Function literals and function types are omitted.
func SignatureRules(entry LangEntry) []gts.SignatureRule {
	return gateSignatureRules(entry, signatureRuleTable[strings.TrimSpace(entry.Name)])
}

func gateSignatureRules(entry LangEntry, candidates []gts.SignatureRule) []gts.SignatureRule {
	if entry.Language == nil || len(candidates) == 0 {
		return nil
	}
	lang := entry.Language()
	if lang == nil {
		return nil
	}
	var rows []gts.SignatureRule
	for _, row := range candidates {
		if !syntaxfacts.ValidSignatureRule(factGrammar(lang), row) {
			continue
		}
		row.Ancestors = append([]string(nil), row.Ancestors...)
		row.ListNodeTypes = append([]string(nil), row.ListNodeTypes...)
		row.Parameters = append([]gts.ParameterRule(nil), row.Parameters...)
		row.TypeNames = append([]gts.TypeNameRule(nil), row.TypeNames...)
		row.ContainerPath = copyContainerRules(row.ContainerPath)
		rows = append(rows, row)
	}
	return rows
}

var callArgumentRuleTable = map[string][]gts.CallArgumentRule{
	"go": {{NodeType: "call_expression", ArgumentsField: "arguments", ListNodeType: "argument_list", SpreadNodeType: "variadic_argument", Kinds: []gts.ExpressionKindRule{
		{NodeType: "identifier", Kind: "identifier"},
		{NodeType: "selector_expression", Kind: "selector"},
		{NodeType: "call_expression", Kind: "call"},
		{NodeType: "composite_literal", Kind: "composite_literal"},
		{NodeType: "func_literal", Kind: "func_literal"},
		{NodeType: "int_literal", Kind: "literal"}, {NodeType: "float_literal", Kind: "literal"}, {NodeType: "imaginary_literal", Kind: "literal"},
		{NodeType: "interpreted_string_literal", Kind: "literal"}, {NodeType: "raw_string_literal", Kind: "literal"}, {NodeType: "rune_literal", Kind: "literal"},
		{NodeType: "true", Kind: "literal"}, {NodeType: "false", Kind: "literal"}, {NodeType: "nil", Kind: "literal"},
		{NodeType: "unary_expression", Kind: "unary"}, {NodeType: "binary_expression", Kind: "binary"},
		{NodeType: "index_expression", Kind: "index"}, {NodeType: "slice_expression", Kind: "slice"},
		{NodeType: "type_assertion_expression", Kind: "type_assertion"}, {NodeType: "parenthesized_expression", Kind: "parenthesized"},
		{NodeType: "type_identifier", Kind: "type"}, {NodeType: "qualified_type", Kind: "type"}, {NodeType: "generic_type", Kind: "type"},
		{NodeType: "pointer_type", Kind: "type"}, {NodeType: "array_type", Kind: "type"}, {NodeType: "slice_type", Kind: "type"},
		{NodeType: "map_type", Kind: "type"}, {NodeType: "channel_type", Kind: "type"}, {NodeType: "function_type", Kind: "type"},
		{NodeType: "struct_type", Kind: "type"}, {NodeType: "interface_type", Kind: "type"}, {NodeType: "parenthesized_type", Kind: "type"},
	}}},
}

// CallArgumentRules returns grammar-gated direct call-argument rows. Expression
// kinds are grammar data; unresolved rows are dropped rather than guessed.
func CallArgumentRules(entry LangEntry) []gts.CallArgumentRule {
	return gateCallArgumentRules(entry, callArgumentRuleTable[strings.TrimSpace(entry.Name)])
}

func gateCallArgumentRules(entry LangEntry, candidates []gts.CallArgumentRule) []gts.CallArgumentRule {
	if entry.Language == nil || len(candidates) == 0 {
		return nil
	}
	lang := entry.Language()
	if lang == nil {
		return nil
	}
	var rows []gts.CallArgumentRule
	for _, row := range candidates {
		if !syntaxfacts.ValidCallArgumentRule(factGrammar(lang), row) {
			continue
		}
		row.Kinds = append([]gts.ExpressionKindRule(nil), row.Kinds...)
		rows = append(rows, row)
	}
	return rows
}
