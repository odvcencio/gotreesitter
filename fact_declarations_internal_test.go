package gotreesitter

import (
	"slices"
	"testing"
)

func TestFactDeclarationsDisabledInstructions(t *testing.T) {
	lang := &Language{
		Name:        "unregistered",
		SymbolNames: []string{"end", "var_spec", "identifier", "source_file"},
		FieldNames:  []string{"", "name"},
	}
	rules := []DeclarationRule{{NodeType: "var_spec", Kind: "variable", NameField: "name", NameNodeType: "identifier", Ancestors: []string{"source_file"}}}
	for _, kinds := range []FactKind{0, FactDefinitions, FactAll} {
		plain, err := NewFactProgram(lang, kinds)
		if err != nil {
			t.Fatal(err)
		}
		configured, err := NewFactProgram(lang, kinds, WithDeclarationRules(rules))
		if err != nil {
			t.Fatal(err)
		}
		if !slices.Equal(plain.code, configured.code) || plain.hasOperations != configured.hasOperations {
			t.Fatal("disabled declarations changed instructions")
		}
		if configured.declarations != nil || configured.declarationRules != nil {
			t.Fatal("disabled declarations retained rule storage")
		}
	}
	enabled, err := NewFactProgram(lang, FactDeclarations, WithDeclarationRules(rules))
	if err != nil {
		t.Fatal(err)
	}
	if !enabled.hasOperations || len(enabled.declarations) != 1 {
		t.Fatal("enabled rule did not compile")
	}
	// Compilation owns the ancestor data; caller mutation cannot change it.
	rules[0].Ancestors[0] = "absent"
	if enabled.declarations[1][0].ancestors[0] != 3 {
		t.Fatal("program retained mutable caller data")
	}
	replaced, err := NewFactProgram(lang, FactDeclarations, WithDeclarationRules(rules), WithDeclarationRules(nil))
	if err != nil {
		t.Fatal(err)
	}
	if replaced.hasOperations || replaced.declarations != nil {
		t.Fatal("later option did not replace rules")
	}
}
