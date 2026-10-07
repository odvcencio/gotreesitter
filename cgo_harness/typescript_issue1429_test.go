//go:build cgo && treesitter_c_parity

package cgoharness

import (
	"os"
	"testing"

	gotreesitter "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
	sitter "github.com/tree-sitter/go-tree-sitter"
)

func TestTypeScriptIssue1429CParity(t *testing.T) {
	fixture, err := os.ReadFile("../grammars/testdata/typescript_issue1429.ts")
	if err != nil {
		t.Fatal(err)
	}

	cases := []struct{ name, source string }{
		{"computed_getter", `class Value { get [Symbol.toStringTag](): string { return "Value"; } }`},
		{"generic_import_type", `type Value<T> = import("./module.js").Value<T>;`},
		{"unique_identifier", `const unique = [1, 2]; for (let i = 1; i < unique.length; i++) {}`},
		{"symbol_tuple_label", `const labels: [spoken: string, symbol: string][] = [];`},
		{"computed_getter_object_type", `class Value { get [key](): { value: string } { return { value: "Value" }; } }`},
		{"computed_getter_function_type", `class Value { get [key](): () => string { return () => "Value"; } }`},
		{"nested_generic_import_type", `type Value<T> = import("./module.js").Types.Value<T>;`},
		{"optional_symbol_tuple_label", `type Labels = [symbol?: string];`},
		{"unlabeled_optional_symbol", `type Labels = [symbol?];`},
		{"unlabeled_get", `type get = string; type Value = [get];`},
		{"unlabeled_optional_get", `type get = string; type Value = [get?];`},
		{"unlabeled_async", `type async = number; type Value = [async, string];`},
		{"unlabeled_optional_async", `type async = number; type Value = [async?];`},
		{"get_tuple_label", `type Value = [get: string];`},
		{"optional_get_tuple_label", `type Value = [get?: string];`},
		{"async_tuple_label", `type Value = [async: number];`},
		{"optional_async_tuple_label", `type Value = [async?: number];`},
		{"unlabeled_set", `type set = string; type Value = [set];`},
		{"unlabeled_optional_set", `type set = string; type Value = [set?];`},
		{"unlabeled_type", `type type = string; type Value = [type];`},
		{"unlabeled_optional_type", `type type = string; type Value = [type?];`},
		{"type_tuple_label", `type Value = [type: string];`},
		{"optional_type_tuple_label", `type Value = [type?: string];`},
		{"predefined_tuple_labels", `type Value = [any: string, number: string, boolean: string, string: string, symbol: string, void: string, unknown: string, never: string, object: string];`},
		{"optional_predefined_tuple_labels", `type Value = [any?: string, number?: string, boolean?: string, string?: string, symbol?: string, void?: string, unknown?: string, never?: string, object?: string];`},
		{"unlabeled_predefined_types", `type Value = [any, number, boolean, string, symbol, void, unknown, never, object];`},
		{"unlabeled_optional_predefined_types", `type Value = [any?, number?, boolean?, string?, symbol?, void?, unknown?, never?, object?];`},
		{"unique_symbol_type", `declare const tag: unique symbol;`},
		{"ternary_get_call", `const value = flag ? get[key]() : fallback;`},
		{"ternary_set_call", `const value = flag ? set[key]() : fallback;`},
		{"collection_fixture", string(fixture)},
	}

	for _, name := range []string{"typescript", "tsx"} {
		t.Run(name, func(t *testing.T) {
			goLang := grammars.DetectLanguageByName(name).Language()
			cLang, err := COracleLanguage(name)
			if err != nil {
				t.Fatal(err)
			}

			for _, c := range cases {
				t.Run(c.name, func(t *testing.T) {
					source := []byte(c.source)
					cParser := sitter.NewParser()
					defer cParser.Close()
					if err := cParser.SetLanguage(cLang); err != nil {
						t.Fatal(err)
					}

					cTree := cParser.Parse(source, nil)
					if cTree == nil {
						t.Fatal("nil C tree")
					}

					defer cTree.Close()
					cRoot := cTree.RootNode()
					goParser := gotreesitter.NewParser(goLang)
					goTree, err := goParser.ParseStrict(source)
					if err != nil {
						t.Fatal(err)
					}

					defer goTree.Release()
					goRoot := goTree.RootNode()
					if cRoot.HasError() || goRoot.HasErrorOrMissing() || goTree.ParseStoppedEarly() || goRoot.EndByte() != uint32(len(source)) || cRoot.EndByte() != uint(len(source)) {
						t.Fatalf("incomplete parse: C error=%v Go=%s", cRoot.HasError(), goRoot.SExpr(goLang))
					}

					assertLockedCTreeExact(t, "fresh", goTree, goLang, cTree)

					edited := append(append([]byte(nil), source...), []byte("\n// edited\n")...)
					goTree.Edit(gotreesitter.InputEdit{
						StartByte: uint32(len(source)), OldEndByte: uint32(len(source)), NewEndByte: uint32(len(edited)),
						StartPoint: pointAtOffset(source, len(source)), OldEndPoint: pointAtOffset(source, len(source)), NewEndPoint: pointAtOffset(edited, len(edited)),
					})

					incremental, err := goParser.ParseIncremental(edited, goTree)
					if err != nil {
						t.Fatal(err)
					}

					defer incremental.Release()
					fresh, err := gotreesitter.NewParser(goLang).ParseStrict(edited)
					if err != nil {
						t.Fatal(err)
					}

					defer fresh.Release()
					cEdited := cParser.Parse(edited, nil)
					if cEdited == nil {
						t.Fatal("nil edited C tree")
					}

					defer cEdited.Close()
					assertLockedCTreeExact(t, "edited fresh", fresh, goLang, cEdited)
					assertLockedCTreeExact(t, "edited incremental", incremental, goLang, cEdited)
				})
			}
		})
	}
}

func TestJavaScriptIssue1429CParity(t *testing.T) {
	goLang := grammars.JavascriptLanguage()
	cLang, err := COracleLanguage("javascript")
	if err != nil {
		t.Fatal(err)
	}

	for _, source := range []string{
		`class Value { get [key]() { return "Value"; } }`,
		`const value = flag ? get[key]() : fallback;`,
		`const value = flag ? set[key]() : fallback;`,
	} {
		t.Run(source, func(t *testing.T) {
			cParser := sitter.NewParser()
			defer cParser.Close()
			if err := cParser.SetLanguage(cLang); err != nil {
				t.Fatal(err)
			}

			cTree := cParser.Parse([]byte(source), nil)
			if cTree == nil {
				t.Fatal("nil C tree")
			}

			defer cTree.Close()
			goTree, err := gotreesitter.NewParser(goLang).ParseStrict([]byte(source))
			if err != nil {
				t.Fatal(err)
			}

			defer goTree.Release()
			assertLockedCTreeExact(t, "indexed get/set", goTree, goLang, cTree)
		})
	}
}
