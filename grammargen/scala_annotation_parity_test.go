package grammargen

import (
	"testing"

	"github.com/odvcencio/gotreesitter"
)

func TestScalaConstructorOwnershipMatchesLockedC(t *testing.T) {
	generated, reference := loadImportedParityLanguages(t, "scala")
	tests := []struct {
		name     string
		source   string
		expected string
	}{
		{
			name:     "smallest_annotation",
			source:   "class A@a()\n",
			expected: "(compilation_unit (class_definition (identifier) (annotation (type_identifier) (arguments))))",
		},
		{
			name:   "annotation_with_argument",
			source: "class A@a(x)\n",
		},
		{
			name:   "repeated_annotations",
			source: "@a() @b() class A\n",
		},
		{
			name:   "constructor_type_control",
			source: "val c: C(42) = value\n",
		},
		{
			name:   "class_template_constructor_control",
			source: "class A { val c: C(42) = value }\n",
		},
		{
			name:   "annotated_class_template_constructor_control",
			source: "class A@a() { val c: C(42) = value }\n",
		},
		{
			name:     "two_parents_with_template",
			source:   "class F extends A with B {}\n",
			expected: "(compilation_unit (class_definition (identifier) (extends_clause (type_identifier) (type_identifier)) (template_body)))",
		},
		{
			name:     "two_parents_without_template",
			source:   "class F extends A with B\n",
			expected: "(compilation_unit (class_definition (identifier) (extends_clause (type_identifier) (type_identifier))))",
		},
		{
			name:     "one_parent_control",
			source:   "class F extends A {}\n",
			expected: "(compilation_unit (class_definition (identifier) (extends_clause (type_identifier)) (template_body)))",
		},
		{
			name:     "qualified_parents",
			source:   "class F extends a.A with b.B {}\n",
			expected: "(compilation_unit (class_definition (identifier) (extends_clause (stable_type_identifier (identifier) (type_identifier)) (stable_type_identifier (identifier) (type_identifier))) (template_body)))",
		},
		{
			name:   "generic_parent",
			source: "class A extends B[C] {}\n",
		},
		{
			name:   "qualified_generic_parent",
			source: "class A extends B.C[D, E] {}\n",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			data := []byte(test.source)
			generatedParser := gotreesitter.NewParser(generated)
			generatedParser.SetAdmissionCandidateRoute(false)
			generatedTree, err := generatedParser.Parse(data)
			if err != nil {
				t.Fatalf("generated parse: %v", err)
			}
			defer generatedTree.Release()
			referenceParser := gotreesitter.NewParser(reference)
			referenceParser.SetAdmissionCandidateRoute(false)
			referenceTree, err := referenceParser.Parse(data)
			if err != nil {
				t.Fatalf("locked C parse: %v", err)
			}
			defer referenceTree.Release()

			generatedRoot := generatedTree.RootNode()
			referenceRoot := referenceTree.RootNode()
			if generatedRoot.HasError() || referenceRoot.HasError() {
				t.Fatalf("unexpected error: generated=%t locked_c=%t", generatedRoot.HasError(), referenceRoot.HasError())
			}
			if test.expected != "" {
				if got := generatedRoot.SExpr(generated); got != test.expected {
					t.Fatalf("generated tree = %s, want %s", got, test.expected)
				}
				if got := referenceRoot.SExpr(reference); got != test.expected {
					t.Fatalf("locked C tree = %s, want %s", got, test.expected)
				}
			}
			if divs := compareTreesDeep(generatedRoot, generated, referenceRoot, reference, "root", 10); len(divs) != 0 {
				t.Fatalf("deep mismatch: %v", divs)
			}
		})
	}
}
