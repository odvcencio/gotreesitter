package grammars

import (
	"context"
	"strings"
	"testing"

	gts "github.com/odvcencio/gotreesitter"
)

// +case:id=typescript_modifier_method_names,desc=`Modifiers before generator and private methods`,expect=`AST and declaration/signature facts preserve the actual name and its byte range`
func TestTypeScriptModifierMethodNames(t *testing.T) {
	for _, language := range []string{"typescript", "tsx"} {
		t.Run(language, func(t *testing.T) {
			entry := DetectLanguageByName(language)
			lang := entry.Language()
			program, err := gts.NewFactProgramWithOptions(lang, gts.FactDeclarations|gts.FactSignatures,
				gts.WithDeclarationRules(DeclarationRules(*entry)),
				gts.WithSignatureRules(SignatureRules(*entry)))
			if err != nil {
				t.Fatal(err)
			}
			for _, tc := range []struct{ member, name string }{
				{`private async *stream(value: string): AsyncGenerator<string> { yield value; }`, "stream"},
				{`protected async *generic /* type docs */ <T>(value: T): AsyncGenerator<T> { yield value; }`, "generic"},
				{`public static async *events() { yield 1; }`, "events"},
				{`private async*compact() { yield 1; }`, "compact"},
				{`static async #pending() {}`, "#pending"},
				{`public async /* name docs */ commented() {}`, "commented"},
				{`public async ordinary<T>(value: T): Promise<T> { return value; }`, "ordinary"},
				{`public async () {}`, "async"},
				{`public value /* c */ (): void {}`, "value"},
				{`public async /* c */ () {}`, "async"},
				{`public get size () { return 1; }`, "size"},
				{`public set size (value: number) {}`, "size"},
			} {
				t.Run(tc.member, func(t *testing.T) {
					source := []byte("class Box { " + tc.member + " }")
					tree, err := entry.ParseStrict(context.Background(), source)
					if tree != nil {
						defer tree.Release()
					}
					if err != nil {
						t.Fatal(err)
					}
					root := tree.RootNode()
					if root.HasErrorOrMissing() || root.EndByte() != uint32(len(source)) {
						t.Fatalf("incomplete tree: %s", root.SExpr(lang))
					}
					start := uint32(strings.Index(string(source), tc.name))
					end := start + uint32(len(tc.name))
					checkName := func(kind, name string, lo, hi uint32) {
						t.Helper()
						if name != tc.name || lo != start || hi != end {
							t.Errorf("%s name = %q [%d,%d), want %q [%d,%d)", kind, name, lo, hi, tc.name, start, end)
						}
					}
					methods := 0
					gts.Walk(root, func(node *gts.Node, _ int) gts.WalkAction {
						if node.Type(lang) == "method_definition" {
							methods++
							name := node.ChildByFieldName("name", lang)
							if tc.name == "ordinary" {
								params := node.ChildByFieldName("type_parameters", lang)
								if params == nil || string(source[params.StartByte():params.EndByte()]) != "<T>" {
									t.Error("generic method is missing its type_parameters field")
								}
							}
							if name == nil {
								t.Error("method has no name field")
							} else {
								checkName("AST", string(source[name.StartByte():name.EndByte()]), name.StartByte(), name.EndByte())
							}
						}
						return gts.WalkContinue
					})
					if methods != 1 {
						t.Errorf("method nodes = %d, want 1", methods)
					}
					facts := program.Extract(tree)
					declarations := 0
					for _, fact := range facts.Declarations {
						if fact.Kind == "method" {
							declarations++
							checkName("declaration", fact.Name, fact.NameStartByte, fact.NameEndByte)
						}
					}
					if declarations != 1 {
						t.Errorf("method declarations = %d, want 1", declarations)
					}
					if len(facts.Signatures) != 1 {
						t.Errorf("signatures = %d, want 1", len(facts.Signatures))
					}
					for _, fact := range facts.Signatures {
						checkName("signature", fact.Name, fact.NameStartByte, fact.NameEndByte)
					}
				})
			}
		})
	}
}

// TestTypeScriptModifierFieldNameWithTrailingComment protects a field whose
// name is followed by a line comment before its type annotation.
func TestTypeScriptModifierFieldNameWithTrailingComment(t *testing.T) {
	for _, name := range []string{"typescript", "tsx"} {
		t.Run(name, func(t *testing.T) {
			entry := DetectLanguageByName(name)
			lang := entry.Language()
			source := []byte("class A { private x // c\n: number }")
			tree, err := entry.ParseStrict(context.Background(), source)
			if err != nil {
				t.Fatal(err)
			}
			defer tree.Release()
			if tree.RootNode().HasErrorOrMissing() {
				t.Fatal("unexpected parse error")
			}
			fields := 0
			gts.Walk(tree.RootNode(), func(node *gts.Node, _ int) gts.WalkAction {
				if node.Type(lang) != "public_field_definition" {
					return gts.WalkContinue
				}
				fields++
				field := node.ChildByFieldName("name", lang)
				if field == nil || string(source[field.StartByte():field.EndByte()]) != "x" {
					t.Error("field name is not x")
				}
				return gts.WalkContinue
			})
			if fields != 1 {
				t.Fatalf("fields = %d, want 1", fields)
			}
		})
	}
}
