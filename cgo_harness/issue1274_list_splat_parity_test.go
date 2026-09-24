//go:build cgo && treesitter_c_parity

package cgoharness

import (
	"testing"

	gotreesitter "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
	sitter "github.com/tree-sitter/go-tree-sitter"
)

func TestIssue1274ListSplatParity(t *testing.T) {
	language := grammars.PythonLanguage()
	cLanguage, err := ParityCLanguage("python")
	if err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		name   string
		source string
		wantC  string
	}{
		{"attribute_argument", "g(*a.b)\n", "(module (call function: (identifier) arguments: (argument_list (list_splat (attribute object: (identifier) attribute: (identifier))))))"},
		{"subscript_argument", "g(*a[1:])\n", "(module (call function: (identifier) arguments: (argument_list (list_splat (subscript value: (identifier) subscript: (slice (integer)))))))"},
		{"call_argument", "g(*a())\n", "(module (call function: (identifier) arguments: (argument_list (list_splat (call function: (identifier) arguments: (argument_list))))))"},
		{"attribute_pattern", "x, *a.b = y\n", "(module (assignment left: (pattern_list (identifier) (list_splat_pattern (attribute object: (identifier) attribute: (identifier)))) right: (identifier)))"},
		{"condition_list", "if [*self.filter_vertical, *self.filter_horizontal]:\n    pass\n", "(module (if_statement condition: (list (list_splat (attribute object: (identifier) attribute: (identifier))) (list_splat (attribute object: (identifier) attribute: (identifier)))) consequence: (block (pass_statement))))"},
		{"condition_three_splats", "if x:\n    names = [\n        *query.extra_select,\n        *query.values_select,\n        *query.annotation_select,\n    ]\n", "(module (if_statement condition: (identifier) consequence: (block (assignment left: (identifier) right: (list (list_splat (attribute object: (identifier) attribute: (identifier))) (list_splat (attribute object: (identifier) attribute: (identifier))) (list_splat (attribute object: (identifier) attribute: (identifier))))))))"},
		{"class_method_two_splats", "class C:\n    @classmethod\n    def f(cls):\n        cls.fields = [\n            *AllFieldsModel._meta.fields,\n            *AllFieldsModel._meta.private_fields,\n        ]\n", "(module (class_definition name: (identifier) body: (block (decorated_definition (decorator (identifier)) definition: (function_definition name: (identifier) parameters: (parameters (identifier)) body: (block (assignment left: (attribute object: (identifier) attribute: (identifier)) right: (list (list_splat (attribute object: (attribute object: (identifier) attribute: (identifier)) attribute: (identifier))) (list_splat (attribute object: (attribute object: (identifier) attribute: (identifier)) attribute: (identifier)))))))))))"},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			source := []byte(test.source)
			cParser := sitter.NewParser()
			defer cParser.Close()
			if err := cParser.SetLanguage(cLanguage); err != nil {
				t.Fatal(err)
			}
			cTree := cParser.Parse(source, nil)
			if cTree == nil || cTree.RootNode() == nil {
				t.Fatal("C parser returned no tree")
			}
			defer cTree.Close()
			if got := cTree.RootNode().ToSexp(); got != test.wantC {
				t.Fatalf("C tree = %s, want %s", got, test.wantC)
			}
			for _, route := range []struct {
				name    string
				compact bool
			}{{"production", false}, {"compact", true}} {
				t.Run(route.name, func(t *testing.T) {
					parser := gotreesitter.NewParser(language)
					parser.SetAdmissionCandidateRoute(route.compact)
					tree, err := parser.Parse(source)
					if err != nil {
						t.Fatal(err)
					}
					defer tree.Release()
					if diff := FirstDivergenceDumpV1(tree.RootNode(), language, cTree.RootNode()); diff != nil {
						t.Fatalf("Go tree = %s; C divergence: %+v", tree.RootNode().SExpr(language), diff)
					}
				})
			}
		})
	}
}
