//go:build !grammar_subset

package grammars

import (
	"testing"

	gotreesitter "github.com/odvcencio/gotreesitter"
)

// Issue #1581: the F# grammar pinned before ionide/tree-sitter-fsharp b576ecf
// had no optional("rec") in named_module, so "module rec A.B" fell into error
// recovery, and its import_decl had no optional("type"), so "open type X"
// took "type" as the opened name.

func parseFSharpRegression(t *testing.T, source string) (*gotreesitter.Tree, *gotreesitter.Language) {
	t.Helper()
	language := FsharpLanguage()
	tree, err := gotreesitter.NewParser(language).Parse([]byte(source))
	if err != nil {
		t.Fatalf("parse %q: %v", source, err)
	}
	t.Cleanup(tree.Release)
	root := tree.RootNode()
	if root == nil {
		t.Fatalf("parse %q: no root, stop %s", source, tree.ParseStopReason())
	}
	if root.HasError() || tree.ParseStopReason() != gotreesitter.ParseStopAccepted {
		t.Fatalf("parse %q: has error %v, stop %s: %s", source, root.HasError(), tree.ParseStopReason(), root.SExpr(language))
	}
	return tree, language
}

func TestFSharpModuleRecParsesAsNamedModule(t *testing.T) {
	tests := []struct {
		name, source, module string
	}{
		{name: "dotted", source: "module rec Acme.Demo\nlet x = 1\n", module: "Acme.Demo"},
		{name: "access-modifier", source: "module internal rec Acme.Demo\nlet x = 1\n", module: "Acme.Demo"},
		{name: "attribute", source: "[<AutoOpen>]\nmodule rec Acme.Demo\nlet x = 1\n", module: "Acme.Demo"},
		{name: "single-segment", source: "module rec Demo\nlet x = 1\n", module: "Demo"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tree, language := parseFSharpRegression(t, tt.source)
			source := []byte(tt.source)
			root := tree.RootNode()
			module := findRecoveryActionMaterializationNode(root, language, "named_module")
			if module == nil {
				t.Fatalf("no named_module: %s", root.SExpr(language))
			}
			name := module.ChildByFieldName("name", language)
			if name == nil || name.Type(language) != "long_identifier" || name.Text(source) != tt.module {
				t.Fatalf("named_module name = %v, want long_identifier %q: %s", name, tt.module, root.SExpr(language))
			}
			rec := false
			for i := 0; i < module.ChildCount(); i++ {
				child := module.Child(i)
				if !child.IsNamed() && child.Type(language) == "rec" {
					rec = true
				}
			}
			if !rec {
				t.Fatalf("named_module has no rec keyword: %s", root.SExpr(language))
			}
			if findRecoveryActionMaterializationNode(module, language, "function_or_value_defn") == nil {
				t.Fatalf("named_module does not hold the let binding: %s", root.SExpr(language))
			}
		})
	}
}

func TestFSharpOpenTypeImportsLongIdentifier(t *testing.T) {
	const text = "open type System.Math\n"
	tree, language := parseFSharpRegression(t, text)
	source := []byte(text)
	root := tree.RootNode()
	imports := 0
	var decl *gotreesitter.Node
	gotreesitter.Walk(root, func(node *gotreesitter.Node, depth int) gotreesitter.WalkAction {
		if node.Type(language) == "import_decl" {
			imports++
			decl = node
		}
		return gotreesitter.WalkContinue
	})
	if imports != 1 {
		t.Fatalf("import_decl count = %d, want 1: %s", imports, root.SExpr(language))
	}
	if decl.Text(source) != "open type System.Math" {
		t.Fatalf("import_decl text = %q: %s", decl.Text(source), root.SExpr(language))
	}
	if decl.NamedChildCount() != 1 {
		t.Fatalf("import_decl named children = %d, want 1: %s", decl.NamedChildCount(), root.SExpr(language))
	}
	name := decl.NamedChild(0)
	if name.Type(language) != "long_identifier" || name.Text(source) != "System.Math" {
		t.Fatalf("import_decl name = %s %q, want long_identifier \"System.Math\": %s",
			name.Type(language), name.Text(source), root.SExpr(language))
	}
}
