package grammars

import (
	"strings"

	gts "github.com/odvcencio/gotreesitter"
)

// declarationRuleTable belongs to the grammars package, not the extractor.
// Exact paths omit local bindings, anonymous nested members, and embeddings.
var declarationRuleTable = map[string][]gts.DeclarationRule{
	"go": {
		{NodeType: "type_spec", Kind: "type", NameField: "name", NameNodeType: "type_identifier", TypeField: "type", TypeNodeType: "struct_type", Shape: "struct"},
		{NodeType: "type_spec", Kind: "type", NameField: "name", NameNodeType: "type_identifier", TypeField: "type", TypeNodeType: "interface_type", Shape: "interface"},
		{NodeType: "type_spec", Kind: "type", NameField: "name", NameNodeType: "type_identifier"},
		{NodeType: "type_alias", Kind: "type", NameField: "name", NameNodeType: "type_identifier", Shape: "alias"},
		{NodeType: "var_spec", Kind: "variable", NameField: "name", NameNodeType: "identifier", Ancestors: []string{"var_declaration", "source_file"}},
		{NodeType: "var_spec", Kind: "variable", NameField: "name", NameNodeType: "identifier", Ancestors: []string{"var_spec_list", "var_declaration", "source_file"}},
		{NodeType: "const_spec", Kind: "constant", NameField: "name", NameNodeType: "identifier", Ancestors: []string{"const_declaration", "source_file"}},
		{NodeType: "field_declaration", Kind: "field", NameField: "name", NameNodeType: "field_identifier", Ancestors: []string{"field_declaration_list", "struct_type", "type_spec"}, ContainerNameField: "name", ContainerNameNodeType: "type_identifier", ContainerTypeField: "type"},
		{NodeType: "method_elem", Kind: "method", NameField: "name", NameNodeType: "field_identifier", Ancestors: []string{"interface_type", "type_spec"}, ContainerNameField: "name", ContainerNameNodeType: "type_identifier", ContainerTypeField: "type"},
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
	hasSymbol := func(name string) bool { _, ok := lang.SymbolByName(name); return ok }
	hasField := func(name string) bool { _, ok := lang.FieldByName(name); return ok }
	for _, rule := range candidates {
		if !hasSymbol(rule.NodeType) || !hasSymbol(rule.NameNodeType) || !hasField(rule.NameField) {
			continue
		}
		if !everyOutlineOwnerTypeExists(rule.Ancestors, hasSymbol) {
			continue
		}
		if rule.TypeField != "" && !hasField(rule.TypeField) || rule.TypeNodeType != "" && !hasSymbol(rule.TypeNodeType) {
			continue
		}
		if rule.ContainerNameField != "" && !hasField(rule.ContainerNameField) || rule.ContainerNameNodeType != "" && !hasSymbol(rule.ContainerNameNodeType) || rule.ContainerTypeField != "" && !hasField(rule.ContainerTypeField) {
			continue
		}
		rule.Ancestors = append([]string(nil), rule.Ancestors...)
		rules = append(rules, rule)
	}
	return rules
}
