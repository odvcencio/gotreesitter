//go:build !grammar_subset

package grammars

import (
	"testing"

	gotreesitter "github.com/odvcencio/gotreesitter"
)

// hlslSubscriptAssignmentFixtures cover `Name[index] = value;`, a construct
// the grammar's declared conflict lets read two ways: a subscript
// assignment expression, or a C++-style structured-binding declaration
// (`Name` as the type, `[index]` as the binding list). The grammar assigns
// structured_binding_declarator a negative dynamic precedence, so a clean,
// unambiguous election always prefers the subscript assignment.
var hlslSubscriptAssignmentFixtures = []struct {
	name   string
	source string
}{
	{
		name:   "single_index_in_function",
		source: "void f() {\n    Foo[bar] = value;\n}",
	},
	{
		name:   "top_level",
		source: "Foo[bar] = value;",
	},
	{
		name:   "two_indices",
		source: "void f() {\n    Foo[bar, baz] = value;\n}",
	},
	{
		name:   "numeric_index",
		source: "void f() {\n    Foo[42] = value;\n}",
	},
	{
		name:   "call_value",
		source: "void f() {\n    Foo[bar] = compute(baz);\n}",
	},
}

// TestHLSLSubscriptAssignmentNeedsNoResultCompatibility proves the native
// parser already elects the C-compatible subscript-assignment shape for
// `Name[index] = value;` without any post-parse compatibility pass: the
// compatibility-free route (ParseNoResultCompatibilityBenchmarkOnly) and the
// production route (Parse) build byte-identical trees, and production
// performs zero normalization rewrites.
func TestHLSLSubscriptAssignmentNeedsNoResultCompatibility(t *testing.T) {
	t.Setenv("GTS_DISPATCHER_CENSUS", "1")
	language := HlslLanguage()
	for _, fixture := range hlslSubscriptAssignmentFixtures {
		fixture := fixture
		t.Run(fixture.name, func(t *testing.T) {
			source := []byte(fixture.source)

			raw, err := gotreesitter.NewParser(language).
				ParseNoResultCompatibilityBenchmarkOnly(source)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(raw.Release)
			assertNoNormalizationPasses(t, raw)
			assertHLSLSubscriptAssignmentShape(t, raw, language)

			production, err := gotreesitter.NewParser(language).Parse(source)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(production.Release)
			assertNoNormalizationPasses(t, production)
			assertHLSLSubscriptAssignmentShape(t, production, language)

			want := collapsedTokenTreeDigest(t, production, language)
			if got := collapsedTokenTreeDigest(t, raw, language); got != want {
				t.Fatalf("compatibility-free digest=%s production=%s", got, want)
			}
		})
	}
}

// assertHLSLSubscriptAssignmentShape locks the exact election the C oracle
// makes: an error-free assignment_expression wrapping a subscript_expression,
// never a structured-binding declaration.
func assertHLSLSubscriptAssignmentShape(t *testing.T, tree *gotreesitter.Tree, language *gotreesitter.Language) {
	t.Helper()
	root := tree.RootNode()
	if root == nil || root.HasError() {
		t.Fatalf("root = %v, want error-free tree", root)
	}
	assign := findHLSLNodeByType(root, language, "assignment_expression")
	if assign == nil {
		t.Fatalf("assignment_expression not found in %s", root.SExpr(language))
	}
	if got, want := assign.ChildCount(), 3; got != want {
		t.Fatalf("assignment_expression child count = %d, want %d", got, want)
	}
	subscript := assign.Child(0)
	if subscript == nil || subscript.Type(language) != "subscript_expression" {
		t.Fatalf("assignment left child = %#v, want subscript_expression", subscript)
	}
	if binding := findHLSLNodeByType(root, language, "structured_binding_declarator"); binding != nil {
		t.Fatalf("found structured_binding_declarator in %s, want none", root.SExpr(language))
	}
}

func findHLSLNodeByType(root *gotreesitter.Node, lang *gotreesitter.Language, typ string) *gotreesitter.Node {
	if root == nil {
		return nil
	}
	if root.Type(lang) == typ {
		return root
	}
	for i := 0; i < root.ChildCount(); i++ {
		if found := findHLSLNodeByType(root.Child(i), lang, typ); found != nil {
			return found
		}
	}
	return nil
}

// TestHLSLSubscriptAssignmentRetiredDispatchArmRoutes proves production,
// compact, forest, and incremental routes all build the same tree for the
// subscript-assignment witnesses with no result compatibility pass involved.
func TestHLSLSubscriptAssignmentRetiredDispatchArmRoutes(t *testing.T) {
	t.Setenv("GTS_DISPATCHER_CENSUS", "1")
	language := HlslLanguage()
	for _, fixture := range hlslSubscriptAssignmentFixtures {
		fixture := fixture
		t.Run(fixture.name, func(t *testing.T) {
			baseSource := []byte(fixture.source)
			for _, receipt := range retiredDispatchRouteReceiptsAllowCompactFallback(t, language, baseSource) {
				assertNoNormalizationPasses(t, receipt.tree)
				assertHLSLSubscriptAssignmentShape(t, receipt.tree, language)
			}
		})
	}
}
