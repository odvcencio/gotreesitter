package parse_test

import (
	"testing"

	"github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
)

func TestOpportunisticTopLevelResyncDoesNotLiftNestedDeclarationStarts(t *testing.T) {
	for _, tc := range []struct {
		name      string
		lang      *gotreesitter.Language
		src       []byte
		forbidden []string
	}{
		{
			name:      "dart_class_inside_malformed_function",
			lang:      grammars.DartLanguage(),
			src:       []byte("main() {\n  var x = class C {}\n  print(x);\n}\n"),
			forbidden: []string{"class_definition"},
		},
		{
			name:      "java_import_inside_malformed_method",
			lang:      grammars.JavaLanguage(),
			src:       []byte("class A { void m() { int x = import java.util.List; x++; } }\n"),
			forbidden: []string{"import_declaration"},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			parser := gotreesitter.NewParser(tc.lang)
			tree, err := parser.Parse(tc.src)
			if err != nil {
				t.Fatalf("parse failed: %v", err)
			}
			root := tree.RootNode()
			if root == nil {
				t.Fatal("missing root node")
			}
			if !root.HasError() {
				t.Fatalf("malformed source parsed without error: %s", root.SExpr(tc.lang))
			}
			for _, typ := range tc.forbidden {
				if containsNodeTypeForTest(root, tc.lang, typ) {
					t.Fatalf("nested declaration-start token was lifted into %s: %s", typ, root.SExpr(tc.lang))
				}
			}
		})
	}
}

func containsNodeTypeForTest(n *gotreesitter.Node, lang *gotreesitter.Language, typ string) bool {
	if n == nil {
		return false
	}
	if n.Type(lang) == typ {
		return true
	}
	for i := 0; i < n.ChildCount(); i++ {
		if containsNodeTypeForTest(n.Child(i), lang, typ) {
			return true
		}
	}
	return false
}
