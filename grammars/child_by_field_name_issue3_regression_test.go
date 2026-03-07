package grammars

import (
	"testing"

	"github.com/odvcencio/gotreesitter"
)

func findFirstNamedByType(root *gotreesitter.Node, lang *gotreesitter.Language, typ string) *gotreesitter.Node {
	if root == nil {
		return nil
	}
	queue := []*gotreesitter.Node{root}
	for len(queue) > 0 {
		n := queue[0]
		queue = queue[1:]
		if n.IsNamed() && n.Type(lang) == typ {
			return n
		}
		for i := 0; i < n.ChildCount(); i++ {
			if child := n.Child(i); child != nil {
				queue = append(queue, child)
			}
		}
	}
	return nil
}

func TestChildByFieldNameRegressionPythonFunctionDefinition(t *testing.T) {
	entry := findEntryByName(t, "python")
	src := []byte("def f(a, b=1):\n    return a + b\n")
	tree, lang := parseSampleForEntry(t, entry, src)

	fn := findFirstNamedByType(tree.RootNode(), lang, "function_definition")
	if fn == nil {
		t.Fatal("expected function_definition node in Python sample")
	}

	for _, fieldName := range []string{"name", "parameters", "body"} {
		node := fn.ChildByFieldName(fieldName, lang)
		if node == nil || !node.IsNamed() {
			t.Fatalf("function_definition %s field should resolve to a named node", fieldName)
		}
	}
}

func TestChildByFieldNameRegressionTSXAttributeValue(t *testing.T) {
	entry := findEntryByName(t, "tsx")
	src := []byte("const el = <A.B x={foo?.bar} y=\"z\" />;\n")
	tree, lang := parseSampleForEntry(t, entry, src)

	attr := findFirstNamedByType(tree.RootNode(), lang, "jsx_attribute")
	if attr == nil {
		t.Fatal("expected jsx_attribute node in TSX sample")
	}

	value := attr.ChildByFieldName("value", lang)
	if value == nil {
		t.Fatal("jsx_attribute value field should resolve to a node")
	}
	if value.Type(lang) == "=" {
		t.Fatal("jsx_attribute value field resolved to '=' token instead of value node")
	}
}

func BenchmarkIssue3ChildByFieldName(b *testing.B) {
	pythonEntry := findEntryByName(b, "python")
	pythonTree, pythonLang := parseSampleForEntry(b, pythonEntry, []byte("def f(a, b=1):\n    return a + b\n"))
	pythonFn := findFirstNamedByType(pythonTree.RootNode(), pythonLang, "function_definition")
	if pythonFn == nil {
		b.Fatal("missing Python function_definition node")
	}

	tsxEntry := findEntryByName(b, "tsx")
	tsxTree, tsxLang := parseSampleForEntry(b, tsxEntry, []byte("const el = <A.B x={foo?.bar} y=\"z\" />;\n"))
	tsxAttr := findFirstNamedByType(tsxTree.RootNode(), tsxLang, "jsx_attribute")
	if tsxAttr == nil {
		b.Fatal("missing TSX jsx_attribute node")
	}

	b.Run("python_function_definition_body", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			_ = pythonFn.ChildByFieldName("body", pythonLang)
		}
	})

	b.Run("tsx_jsx_attribute_value", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			_ = tsxAttr.ChildByFieldName("value", tsxLang)
		}
	})
}
