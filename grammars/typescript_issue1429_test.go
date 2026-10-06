package grammars

import (
	"os"
	"testing"

	gotreesitter "github.com/odvcencio/gotreesitter"
)

func TestTypeScriptIssue1429(t *testing.T) {
	fixture, err := os.ReadFile("testdata/typescript_issue1429.ts")
	if err != nil {
		t.Fatal(err)
	}

	languages := []struct {
		name string
		load func() *gotreesitter.Language
	}{
		{"typescript", TypescriptLanguage},
		{"tsx", TsxLanguage},
	}

	cases := []struct {
		name, source, nodeType string
	}{
		{"computed_getter", `class Value { get [Symbol.toStringTag](): string { return "Value"; } }`, "method_definition"},
		{"generic_import_type", `type Value<T> = import("./module.js").Value<T>;`, "import_type"},
		{"unique_identifier", `const unique = [1, 2]; for (let i = 1; i < unique.length; i++) {}`, "binary_expression"},
		{"symbol_tuple_label", `const labels: [spoken: string, symbol: string][] = [];`, "tuple_type"},
		{"computed_getter_object_type", `class Value { get [key](): { value: string } { return { value: "Value" }; } }`, "method_definition"},
		{"computed_getter_function_type", `class Value { get [key](): () => string { return () => "Value"; } }`, "method_definition"},
		{"nested_generic_import_type", `type Value<T> = import("./module.js").Types.Value<T>;`, "import_type"},
		{"optional_symbol_tuple_label", `type Labels = [symbol?: string];`, "tuple_type"},
		{"unlabeled_optional_symbol", `type Labels = [symbol?];`, "tuple_type"},
		{"unique_symbol_type", `declare const tag: unique symbol;`, "type_annotation"},
		{"ternary_get_call", `const value = flag ? get[key]() : fallback;`, "ternary_expression"},
		{"ternary_set_call", `const value = flag ? set[key]() : fallback;`, "ternary_expression"},
		{"collection_fixture", string(fixture), "import_type"},
	}

	for _, language := range languages {
		t.Run(language.name, func(t *testing.T) {
			lang := language.load()
			for _, c := range cases {
				t.Run(c.name, func(t *testing.T) {
					parser := gotreesitter.NewParser(lang)
					tree, err := parser.ParseStrict([]byte(c.source))
					if err != nil {
						t.Fatal(err)
					}

					defer tree.Release()
					root := tree.RootNode()
					if root == nil {
						t.Fatal("nil root")
					}

					if root.HasErrorOrMissing() || tree.ParseStoppedEarly() || root.EndByte() != uint32(len(c.source)) {
						t.Fatalf("invalid tree: %s", root.SExpr(lang))
					}

					if countTypeScriptNodes(root, lang, c.nodeType) != 1 {
						t.Fatalf("expected one %s: %s", c.nodeType, root.SExpr(lang))
					}
				})
			}
		})
	}
}

func TestTypeScriptIssue1429RejectsMalformedSyntax(t *testing.T) {
	for _, load := range []func() *gotreesitter.Language{TypescriptLanguage, TsxLanguage} {
		lang := load()
		for _, source := range []string{
			`class Value { get [key](): string { return "Value"; }`,
			`type Value = import("./module.js").Value<;`,
			`type Value = [symbol: ];`,
		} {
			t.Run(lang.Name+"/"+source, func(t *testing.T) {
				tree, err := gotreesitter.NewParser(lang).ParseStrict([]byte(source))
				if tree != nil {
					defer tree.Release()
				}

				if err == nil && !tree.RootNode().HasErrorOrMissing() {
					t.Fatal("malformed syntax parsed without errors")
				}
			})
		}
	}
}
