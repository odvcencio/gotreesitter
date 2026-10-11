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
