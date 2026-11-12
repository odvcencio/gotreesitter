package grammars

import (
	"os"
	"path/filepath"
	"testing"

	ts "github.com/odvcencio/gotreesitter"
)

func TestCppUserTypeTemplateArgumentInParameterListParsesWithoutError(t *testing.T) {
	src := []byte("#include <vector>\nstruct Rule {};\nstatic inline void f(std::vector<Rule> *v) {}\n")
	parser := ts.NewParser(CppLanguage())
	tree, err := parser.Parse(src)
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	root := tree.RootNode()
	if root == nil {
		t.Fatal("missing root node")
	}
	if root.HasError() {
		t.Fatalf("unexpected parse errors: %s runtime=%s", root.SExpr(CppLanguage()), tree.ParseRuntime().Summary())
	}
}

// A template argument list closed right before an outer close angle, as in
// `A<B<T> >` or `A<B<T>>`, must close the outer template too. The expected
// trees are the C runtime's for the same grammar revision.
func TestCppNestedTemplateCloseAngleMatchesC(t *testing.T) {
	const tail = " (function_definition (primitive_type) (function_declarator (identifier) (parameter_list)) (compound_statement)))"
	const nested = "(template_type (type_identifier) (template_argument_list (type_descriptor (template_type (type_identifier) (template_argument_list (type_descriptor (type_identifier)))))))"
	for _, tc := range []struct{ src, want string }{
		{
			src:  "using X = A<B<T> >;",
			want: "(translation_unit (alias_declaration (type_identifier) (type_descriptor " + nested + "))" + tail,
		},
		{
			src:  "A<B<T>> x;",
			want: "(translation_unit (declaration " + nested + " (identifier))" + tail,
		},
		{
			src: "template <typename T>\nstruct S {\n    using X = typename A<B<T>>::type;\n    using Y =\n        A<B<T> >::type;\n    void m() {}\n};",
			want: "(translation_unit (template_declaration (template_parameter_list (type_parameter_declaration (type_identifier))) (struct_specifier (type_identifier) (field_declaration_list" +
				" (alias_declaration (type_identifier) (type_descriptor (dependent_type (qualified_identifier " + nested + " (type_identifier)))))" +
				" (alias_declaration (type_identifier) (type_descriptor (qualified_identifier " + nested + " (type_identifier))))" +
				" (function_definition (primitive_type) (function_declarator (field_identifier) (parameter_list)) (compound_statement)))))" + tail,
		},
	} {
		src := []byte(tc.src + "\nvoid after() {}\n")
		tree, err := ts.NewParser(CppLanguage()).Parse(src)
		if err != nil {
			t.Fatalf("parse %q: %v", tc.src, err)
		}
		root := tree.RootNode()
		if root.EndByte() != uint32(len(src)) || root.HasError() {
			t.Errorf("%q: root ends at %d of %d, HasError=%v, runtime=%s", tc.src, root.EndByte(), len(src), root.HasError(), tree.ParseRuntime().Summary())
		}
		if got := root.SExpr(CppLanguage()); got != tc.want {
			t.Errorf("%q:\n got  %s\n want %s", tc.src, got, tc.want)
		}
	}
}

// The per-stack own-lex-mode reading must stay limited to elections that
// mixed up same-named symbols. Earthfile tells many tokens apart by lex state
// alone; re-reading them per stack sends the committed sample, which C parses
// cleanly, down an error path.
func TestOwnLexModeReadingKeepsEarthfileSampleClean(t *testing.T) {
	src, err := os.ReadFile(filepath.Join("..", "internal", "benchfixtures", "testdata", "real", "earthfile"))
	if err != nil {
		t.Fatalf("read earthfile sample: %v", err)
	}
	entry := DetectLanguageByName("earthfile")
	if entry == nil {
		t.Fatal("earthfile grammar not registered")
	}
	parser := ts.NewParser(entry.Language())
	parser.SetTimeoutMicros(10_000_000)
	tree, err := parser.Parse(src)
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	if root := tree.RootNode(); root.HasError() || root.EndByte() != uint32(len(src)) {
		t.Fatalf("earthfile sample: has_error=%v end=%d/%d runtime=%s", root.HasError(), root.EndByte(), len(src), tree.ParseRuntime().Summary())
	}
}
