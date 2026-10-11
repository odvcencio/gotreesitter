package grammars

import (
	"testing"

	gts "github.com/odvcencio/gotreesitter"
)

func TestJavaScriptFactRulesResolveAndStayLanguageScoped(t *testing.T) {
	js := LangEntry{Name: "javascript", Language: JavascriptLanguage}
	if n := len(DeclarationRules(js)); n == 0 {
		t.Fatal("no JavaScript declaration rules")
	}
	if n := len(SignatureRules(js)); n == 0 {
		t.Fatal("no JavaScript signature rules")
	}
	if n := len(CallArgumentRules(js)); n != 2 {
		t.Fatalf("JavaScript call-argument rules = %d; want call_expression and new_expression", n)
	}
	// Every JavaScript row names JavaScript-only node types; Go's grammar lacks them.
	foreign := LangEntry{Name: "javascript", Language: GoLanguage}
	if got := DeclarationRules(foreign); len(got) != 0 {
		t.Fatalf("declaration rows resolved against Go: %#v", got)
	}
	if got := SignatureRules(foreign); len(got) != 0 {
		t.Fatalf("signature rows resolved against Go: %#v", got)
	}
	if got := CallArgumentRules(foreign); len(got) != 0 {
		t.Fatalf("argument rows resolved against Go: %#v", got)
	}
}

func TestJavaScriptDeclarationRulesGateMissingGrammarData(t *testing.T) {
	entry := LangEntry{Name: "javascript", Language: JavascriptLanguage}
	var good gts.DeclarationRule
	for _, row := range DeclarationRules(entry) {
		if row.Kind == "constant" {
			good = row
			break
		}
	}
	if good.ParentChildField != "kind" || good.ParentChildNodeType != "const" {
		t.Fatalf("missing const binding row: %#v", good)
	}
	for _, part := range []string{"parent field", "parent node", "container node"} {
		t.Run(part, func(t *testing.T) {
			bad := good
			switch part {
			case "parent field":
				bad.ParentChildField = "absent"
			case "parent node":
				bad.ParentChildNodeType = "absent"
			case "container node":
				bad.ContainerPath = []gts.ContainerRule{{NodeType: "absent", NameField: "name", NameNodeType: "identifier", TypeField: "body"}}
			}
			if got := gateDeclarationRules(entry, []gts.DeclarationRule{bad, good}); len(got) != 1 || got[0].ParentChildField != "kind" || got[0].ParentChildNodeType != "const" {
				t.Fatalf("gate = %#v", got)
			}
		})
	}
}

func TestJavaScriptSignatureRulesGateMissingGrammarData(t *testing.T) {
	entry := LangEntry{Name: "javascript", Language: JavascriptLanguage}
	var good gts.SignatureRule
	for _, r := range SignatureRules(entry) {
		if r.DelegateField != "" {
			good = r
			break
		}
	}
	if len(good.DelegateNodeTypes) == 0 {
		t.Fatal("missing value-bound signature row")
	}
	for _, part := range []string{"value field", "value node", "parameter node", "parameter name node", "alt parameters"} {
		t.Run(part, func(t *testing.T) {
			bad := good
			bad.Parameters = append([]gts.ParameterRule(nil), good.Parameters...)
			switch part {
			case "value field":
				bad.DelegateField = "absent"
			case "value node":
				bad.DelegateNodeTypes = []string{"absent"}
			case "parameter node":
				bad.Parameters[0].NodeType = "absent"
			case "alt parameters":
				bad.SingleParameterField = "absent"
			case "parameter name node":
				bad.Parameters[0].NameNodeType = "absent"
			}
			if got := gateSignatureRules(entry, []gts.SignatureRule{bad, good}); len(got) != 1 {
				t.Fatalf("gate = %#v", got)
			}
		})
	}
}

func TestJavaScriptCallArgumentRulesGateMissingGrammarData(t *testing.T) {
	entry := LangEntry{Name: "javascript", Language: JavascriptLanguage}
	rules := CallArgumentRules(entry)
	if len(rules) != 2 || rules[0].NodeType != "call_expression" || rules[1].NodeType != "new_expression" || !rules[1].Standalone || rules[0].Standalone {
		t.Fatalf("rules = %#v", rules)
	}
	for _, part := range []string{"node", "list", "spread", "kind node"} {
		t.Run(part, func(t *testing.T) {
			bad := rules[0]
			bad.Kinds = append([]gts.ExpressionKindRule(nil), bad.Kinds...)
			switch part {
			case "node":
				bad.NodeType = "absent"
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
}
