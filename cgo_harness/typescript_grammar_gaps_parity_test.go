//go:build cgo && treesitter_c_parity

package cgoharness

import (
	"strings"
	"testing"

	sitter "github.com/tree-sitter/go-tree-sitter"

	gotreesitter "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
)

func TestTypeScriptGrammarGapCOracleParity(t *testing.T) {
	languages := []struct {
		name string
		load func() *gotreesitter.Language
	}{
		{name: "typescript", load: grammars.TypescriptLanguage},
		{name: "tsx", load: grammars.TsxLanguage},
	}
	tests := []struct {
		name   string
		source string
	}{
		{
			name: "variance_annotations",
			source: "type Covariant<out T> = T;\n" +
				"type Contravariant<in T> = (value: T) => void;\n" +
				"type Invariant<in out T> = T;\n" +
				"type Combined<const out T extends object = {}, in U, in out V = U> = T;\n",
		},
		{
			name:   "two_newline_call_signatures",
			source: "type Calls = {\n  <T>(value: T): T\n  <U>(value: U): U\n}\n",
		},
		{
			name:   "three_newline_call_signatures",
			source: "type Calls = {\n  <T>(): T\n  <U>(): U\n  <V>(): V\n}\n",
		},
		{
			name:   "nested_property_calls",
			source: "type Holder = { nested: {\n  <T>(): T\n  <U>(): U\n} }\n",
		},
		{
			name:   "explicit_semicolons",
			source: "type Calls = { <T>(): T; <U>(): U; }\n",
		},
		{
			name:   "single_signature",
			source: "type Calls = { <T>(): T }\n",
		},
		{
			name:   "constructor_overloads",
			source: "type Factory = {\n  new <T>(): T\n  new <U>(): U\n}\n",
		},
		{
			name:   "index_signatures",
			source: "type Index = {\n  [key: string]: string\n  [key: number]: string\n}\n",
		},
		{
			name:   "named_methods",
			source: "type Methods = {\n  first<T>(): T\n  second<U>(): U\n}\n",
		},
		{
			name:   "ordinary_generic_call_after_newline",
			source: "foo\n<number>(1)",
		},
		{
			name:   "mixed_members",
			source: "type Mixed = {\n  value: string;\n  <T>(): T\n  create<U>(): U;\n  [key: number]: string\n}",
		},
		{
			name:   "contextual_in_property_after_default",
			source: "interface A {\n  easing: {\n    default: string\n    in: string\n  }\n}\n",
		},
		{
			name:   "repeated_contextual_in_property",
			source: "interface A {\n  easing: {\n    in: string\n    in: string\n  }\n}\n",
		},
		{
			name:   "binary_in_across_newline",
			source: "const found = key\n  in object\n",
		},
	}

	for _, language := range languages {
		t.Run(language.name, func(t *testing.T) {
			goLang := language.load()
			cLang, err := COracleLanguage(language.name)
			if err != nil {
				t.Fatal(err)
			}
			for _, test := range tests {
				t.Run(test.name, func(t *testing.T) {
					source := []byte(test.source)
					goParser := gotreesitter.NewParser(goLang)
					goTree, err := goParser.Parse(source)
					if err != nil {
						t.Fatal(err)
					}
					defer goTree.Release()
					goRoot := goTree.RootNode()
					if goRoot == nil || goRoot.HasError() || goRoot.EndByte() != uint32(len(source)) {
						t.Fatalf("Go parse is incomplete: %s", goRoot.SExpr(goLang))
					}

					cParser := sitter.NewParser()
					defer cParser.Close()
					if err := cParser.SetLanguage(cLang); err != nil {
						t.Fatal(err)
					}
					cTree := cParser.Parse(source, nil)
					if cTree == nil || cTree.RootNode() == nil {
						t.Fatal("C parse returned a nil tree")
					}
					defer cTree.Close()
					cRoot := cTree.RootNode()
					if cRoot.HasError() || cRoot.EndByte() != uint(len(source)) {
						t.Fatalf("C parse is incomplete: %s", dumpCTree(cRoot, 0))
					}

					var mismatches []string
					compareNodes(goRoot, goLang, cRoot, "root", &mismatches)
					if len(mismatches) != 0 {
						t.Fatalf("Go and C trees differ:\n%s", strings.Join(mismatches, "\n"))
					}
				})
			}
		})
	}
}
