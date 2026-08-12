//go:build !grammar_subset

package grammars

import (
	"testing"

	gotreesitter "github.com/odvcencio/gotreesitter"
)

// enforceConstParamCase pins one Enforce `formal_parameter` shape that a
// retired post-parse compatibility pass used to repair by hand: a `const`
// modifier followed by the `int` primitive type must parse as
// `(formal_parameter_modifier) (type_int) (identifier) ...`, never as a
// misread `(type_identifier (identifier)) (identifier) ...` pair that
// swallows the parameter's own name. Every case here already holds with
// zero post-parse rewrites: native scheduling and reduction publish the
// C-equivalent shape directly, including the default-value clause the
// retired pass had to resynthesize by re-lexing raw source bytes.
type enforceConstParamCase struct {
	name      string
	source    string
	wantSExpr string
	assert    func(*testing.T, *gotreesitter.Node, *gotreesitter.Language)
}

func enforceConstParamCases() []enforceConstParamCase {
	return []enforceConstParamCase{
		{
			name:      "const_int_default_value",
			source:    "class A { void m(const int x = 69) {} }",
			wantSExpr: "(compilation_unit (decl_class (identifier) (class_body (decl_method (type_void) (identifier) (formal_parameters (formal_parameter (formal_parameter_modifier) (type_int) (identifier) (literal_int))) (block)))))",
			assert:    assertEnforceConstIntFormalParameterFields,
		},
		{
			name:      "const_int_no_default",
			source:    "class A { void m(const int x) {} }",
			wantSExpr: "(compilation_unit (decl_class (identifier) (class_body (decl_method (type_void) (identifier) (formal_parameters (formal_parameter (formal_parameter_modifier) (type_int) (identifier))) (block)))))",
		},
		{
			name:      "const_int_default_among_other_modifiers",
			source:    "override void foo(const int x = 69, inout int y, out int y) {}",
			wantSExpr: "(compilation_unit (decl_method (method_modifier) (type_void) (identifier) (formal_parameters (formal_parameter (formal_parameter_modifier) (type_int) (identifier) (literal_int)) (formal_parameter (formal_parameter_modifier) (type_int) (identifier)) (formal_parameter (formal_parameter_modifier) (type_int) (identifier))) (block)))",
		},
		{
			name:      "const_int_default_expression",
			source:    "void foo(const int x = 1 + 2) {}",
			wantSExpr: "(compilation_unit (decl_method (type_void) (identifier) (formal_parameters (formal_parameter (formal_parameter_modifier) (type_int) (identifier) (expression_binary (literal_int) (literal_int)))) (block)))",
		},
		{
			name:      "const_int_forward_declaration",
			source:    "void foo(const int x = 69);",
			wantSExpr: "(compilation_unit (decl_method (type_void) (identifier) (formal_parameters (formal_parameter (formal_parameter_modifier) (type_int) (identifier) (literal_int)))))",
		},
	}
}

func TestEnforceConstParamNeedsNoResultCompatibility(t *testing.T) {
	language := EnforceLanguage()
	for _, test := range enforceConstParamCases() {
		test := test
		t.Run(test.name, func(t *testing.T) {
			source := []byte(test.source + "\n")
			tree, err := gotreesitter.NewParser(language).
				ParseNoResultCompatibilityBenchmarkOnly(source)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(tree.Release)
			assertEnforceConstParamShape(t, tree.RootNode(), language, test)
			assertNoNormalizationPasses(t, tree)
		})
	}
}

func TestEnforceConstParamRetiredDispatchArmRoutes(t *testing.T) {
	language := EnforceLanguage()
	for _, test := range enforceConstParamCases() {
		test := test
		t.Run(test.name, func(t *testing.T) {
			for _, receipt := range retiredDispatchRouteReceipts(t, language, []byte(test.source)) {
				t.Run(receipt.name, func(t *testing.T) {
					assertEnforceConstParamShape(t, receipt.tree.RootNode(), language, test)
				})
			}
		})
	}
}

func assertEnforceConstParamShape(
	t *testing.T,
	root *gotreesitter.Node,
	language *gotreesitter.Language,
	test enforceConstParamCase,
) {
	t.Helper()
	if got := root.SExpr(language); got != test.wantSExpr {
		t.Fatalf("%s tree = %s, want %s", test.name, got, test.wantSExpr)
	}
	if test.assert != nil {
		test.assert(t, root, language)
	}
}

func assertEnforceConstIntFormalParameterFields(
	t *testing.T,
	root *gotreesitter.Node,
	language *gotreesitter.Language,
) {
	t.Helper()
	param := findFirstNamedDescendantWhere(
		root,
		language,
		"formal_parameter",
		func(*gotreesitter.Node) bool { return true },
	)
	if param == nil || param.ChildCount() != 5 {
		t.Fatalf("missing five-child const-int formal_parameter: %s", root.SExpr(language))
	}
	wantTypes := []string{"formal_parameter_modifier", "type_int", "identifier", "=", "literal_int"}
	wantFields := []string{"", "type", "name", "default", "default"}
	for index, wantType := range wantTypes {
		child := param.Child(index)
		if child == nil || child.Type(language) != wantType {
			t.Fatalf("formal_parameter child %d = %v, want %s", index, child, wantType)
		}
		if got := param.FieldNameForChild(index, language); got != wantFields[index] {
			t.Fatalf("formal_parameter child %d field = %q, want %q", index, got, wantFields[index])
		}
	}
	name := param.Child(2)
	if name.ChildCount() != 0 {
		t.Fatalf("formal_parameter name = %v, want a bare identifier leaf", name)
	}
}
