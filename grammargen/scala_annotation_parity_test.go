package grammargen

import (
	"testing"

	"github.com/odvcencio/gotreesitter"
)

func TestScalaAnnotationArgumentsMatchLockedC(t *testing.T) {
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
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			data := []byte(test.source)
			generatedTree, err := gotreesitter.NewParser(generated).Parse(data)
			if err != nil {
				t.Fatalf("generated parse: %v", err)
			}
			defer generatedTree.Release()
			referenceTree, err := gotreesitter.NewParser(reference).Parse(data)
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

func TestScalaAnnotationArgumentsMatchLockedCWithLRSplitting(t *testing.T) {
	var grammarSpec importParityGrammar
	for _, candidate := range importParityGrammars {
		if candidate.name == "scala" {
			grammarSpec = candidate
			break
		}
	}
	if grammarSpec.name == "" {
		t.Fatal("scala import parity grammar not found")
	}
	gram, err := importParityGrammarSource(grammarSpec)
	if err != nil {
		t.Skipf("scala grammar not available: %v", err)
	}
	gram.EnableLRSplitting = true
	generated, err := generateWithTimeout(gram, 4*grammarSpec.genTimeout)
	if err != nil {
		t.Fatalf("generate scala language with LR splitting: %v", err)
	}
	reference := grammarSpec.blobFunc()
	adaptExternalScanner(reference, generated)

	assertGeneratedAndReferenceDeepParity(t, generated, reference, "class A@a()\n")
}
