package grammars

import (
	gts "github.com/odvcencio/gotreesitter"
	"testing"
)

func TestDeclarationRulesGateMissingGrammarData(t *testing.T) {
	entry := LangEntry{Name: "go", Language: GoLanguage}
	good := gts.DeclarationRule{NodeType: "type_alias", Kind: "type", NameField: "name", NameNodeType: "type_identifier", Shape: "alias"}
	for _, part := range []string{"node", "name field", "name type", "ancestor", "type field", "type node", "container field", "container name type", "container type field"} {
		t.Run(part, func(t *testing.T) {
			bad := good
			switch part {
			case "node":
				bad.NodeType = "absent_node"
			case "name field":
				bad.NameField = "absent_field"
			case "name type":
				bad.NameNodeType = "absent_node"
			case "ancestor":
				bad.Ancestors = []string{"absent_node"}
			case "type field":
				bad.TypeField = "absent_field"
				bad.TypeNodeType = "struct_type"
			case "type node":
				bad.TypeField = "type"
				bad.TypeNodeType = "absent_node"
			case "container field":
				bad.ContainerNameField = "absent_field"
			case "container name type":
				bad.ContainerNameNodeType = "absent_node"
			case "container type field":
				bad.ContainerTypeField = "absent_field"
			}
			if got := gateDeclarationRules(entry, []gts.DeclarationRule{bad, good}); len(got) != 1 || got[0].NodeType != good.NodeType {
				t.Fatalf("gate = %#v; want valid row only", got)
			}
		})
	}
	if got := gateDeclarationRules(LangEntry{}, []gts.DeclarationRule{good}); len(got) != 0 {
		t.Fatalf("nil grammar gate = %#v", got)
	}
	if len(DeclarationRules(entry)) == 0 {
		t.Fatal("Go rules were gated out")
	}
	if got := DeclarationRules(LangEntry{Name: "unknown"}); len(got) != 0 {
		t.Fatalf("unknown language rules = %#v", got)
	}
}

func TestSignatureRulesGateMissingGrammarData(t *testing.T) {
	entry := LangEntry{Name: "go", Language: GoLanguage}
	rules := SignatureRules(entry)
	if len(rules) != 3 {
		t.Fatalf("Go signature rules = %#v", rules)
	}
	wrappers := rules[1].TypeNames
	if wrappers[len(wrappers)-1].NodeType != "parenthesized_type" {
		t.Fatal("missing parenthesized receiver type path")
	}
	for _, part := range []string{"node", "name", "parameters", "receiver", "parameter node", "parameter field", "base node", "base field", "receiver wrapper node", "receiver wrapper field", "ancestor"} {
		t.Run(part, func(t *testing.T) {
			bad := rules[1]
			bad.Parameters = append([]gts.ParameterRule(nil), bad.Parameters...)
			bad.TypeNames = append([]gts.TypeNameRule(nil), bad.TypeNames...)
			switch part {
			case "node":
				bad.NodeType = "absent"
			case "name":
				bad.NameField = "absent"
			case "parameters":
				bad.ParametersField = "absent"
			case "receiver":
				bad.ReceiverField = "absent"
			case "parameter node":
				bad.Parameters[0].NodeType = "absent"
			case "parameter field":
				bad.Parameters[0].TypeField = "absent"
			case "base node":
				bad.TypeNames[0].NodeType = "absent"
			case "base field":
				bad.TypeNames[0].Field = "absent"
			case "receiver wrapper node":
				bad.TypeNames[len(bad.TypeNames)-1].NodeType = "absent"
			case "receiver wrapper field":
				bad.TypeNames[len(bad.TypeNames)-1].Field = "absent"
			case "ancestor":
				bad.Ancestors = []string{"absent"}
			}
			if got := gateSignatureRules(entry, []gts.SignatureRule{bad, rules[0]}); len(got) != 1 {
				t.Fatalf("gate = %#v", got)
			}
		})
	}
	if got := SignatureRules(LangEntry{}); len(got) != 0 {
		t.Fatal(got)
	}
}

func TestCallArgumentRulesGateMissingGrammarData(t *testing.T) {
	entry := LangEntry{Name: "go", Language: GoLanguage}
	rules := CallArgumentRules(entry)
	if len(rules) != 1 {
		t.Fatalf("Go call-argument rules = %#v", rules)
	}
	for _, part := range []string{"node", "field", "list", "spread", "kind node"} {
		t.Run(part, func(t *testing.T) {
			bad := rules[0]
			bad.Kinds = append([]gts.ExpressionKindRule(nil), bad.Kinds...)
			switch part {
			case "node":
				bad.NodeType = "absent"
			case "field":
				bad.ArgumentsField = "absent"
			case "list":
				bad.ListNodeType = "absent"
			case "spread":
				bad.SpreadNodeType = "absent"
			case "kind node":
				bad.Kinds[0].NodeType = "absent"
			}
			if got := gateCallArgumentRules(entry, []gts.CallArgumentRule{bad, rules[0]}); len(got) != 1 {
				t.Fatalf("gate = %#v", got)
			}
		})
	}
	if got := CallArgumentRules(LangEntry{}); len(got) != 0 {
		t.Fatal(got)
	}
}

func TestDeclarationRulesGateMissingGrammarPaths(t *testing.T) {
	entry := LangEntry{Name: "go", Language: GoLanguage}
	var good gts.DeclarationRule
	for _, r := range DeclarationRules(entry) {
		if r.Embedded {
			good = r
			break
		}
	}
	if !good.Embedded {
		t.Fatal("missing embedding rule")
	}
	for _, part := range []string{"base field", "base type", "base path field", "container node", "container field", "parent path", "body wrapper"} {
		t.Run(part, func(t *testing.T) {
			bad := good
			bad.TypeNames = append([]gts.TypeNameRule(nil), good.TypeNames...)
			bad.ContainerPath = copyContainerRules(good.ContainerPath)
			switch part {
			case "base field":
				bad.BaseNameField = "absent"
			case "base type":
				bad.TypeNames[0].NodeType = "absent"
			case "base path field":
				bad.TypeNames[0].Field = "absent"
			case "container node":
				bad.ContainerPath[0].NodeType = "absent"
			case "container field":
				bad.ContainerPath[0].TypeField = "absent"
			case "parent path":
				bad.ContainerPath[0].ParentPath = []string{"absent"}
			case "body wrapper":
				bad.ContainerPath[0].BodyWrappers[0].NodeType = "absent"
			}
			if got := gateDeclarationRules(entry, []gts.DeclarationRule{bad, good}); len(got) != 1 {
				t.Fatalf("gate = %#v", got)
			}
		})
	}
}
