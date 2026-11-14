//go:build cgo && treesitter_c_parity

package cgoharness

import (
	"fmt"
	"strings"
	"testing"

	gotreesitter "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
	sitter "github.com/tree-sitter/go-tree-sitter"
)

// +case:id=typescript_modifier_method_parity,desc=`Modifier-prefixed methods retain grammar-owned names`,expect=`Fresh and incremental trees match locked C through both routes`
func TestTypeScriptModifierMethodNamesLockedC(t *testing.T) {
	for _, name := range []string{"typescript", "tsx"} {
		t.Run(name, func(t *testing.T) {
			lang := grammars.DetectLanguageByName(name).Language()
			cLang, err := COracleLanguage(name)
			if err != nil {
				t.Fatal(err)
			}
			cParser := sitter.NewParser()
			defer cParser.Close()
			if err := cParser.SetLanguage(cLang); err != nil {
				t.Fatal(err)
			}
			for _, source := range []string{
				`class Box { private async *stream(value: string): AsyncGenerator<string> { yield value; } }`,
				`class Box { protected async *generic /* type docs */ <T>(value: T): AsyncGenerator<T> { yield value; } }`,
				`class Box { public static async *events() { yield 1; } }`,
				`class Box { private async*compact() { yield 1; } }`,
				`class Box { static async #pending() {} }`,
				`class Box { public async /* name docs */ commented() {} }`,
				`class Box { public async ordinary<T>(value: T): Promise<T> { return value; } }`,
				`class Box { public async () {} }`,
				`class Box { public value /* c */ (): void {} }`,
				"class Box { private x // c\n: number }",
				`class Box { public async /* c */ () {} }`,
				`class Box { public get size () { return 1; } }`,
				`class Box { public set size (value: number) {} }`,
			} {
				for _, candidate := range []bool{false, true} {
					t.Run(fmt.Sprintf("%s/candidate=%t", source, candidate), func(t *testing.T) {
						parser := gotreesitter.NewParser(lang)
						parser.SetAdmissionCandidateRoute(candidate)
						input := []byte(source)
						tree, err := parser.ParseStrict(input)
						if err != nil {
							t.Fatal(err)
						}
						defer tree.Release()
						cTree := cParser.Parse(input, nil)
						if cTree == nil {
							t.Fatal("nil C tree")
						}
						defer cTree.Close()
						assertLockedCTreeExact(t, "fresh method", tree, lang, cTree)
						if tree.RootNode().HasErrorOrMissing() || tree.ParseStoppedEarly() || tree.RootNode().EndByte() != uint32(len(input)) {
							t.Fatal("incomplete fresh tree")
						}

						// Shift the member prefix and its fields, then compare incremental
						// and fresh trees with the pinned C runtime.
						offset := strings.Index(source, "{") + 1
						edited := []byte(source[:offset] + " " + source[offset:])
						tree.Edit(gotreesitter.InputEdit{
							StartByte: uint32(offset), OldEndByte: uint32(offset), NewEndByte: uint32(offset + 1),
							StartPoint: pointAtOffset(input, offset), OldEndPoint: pointAtOffset(input, offset), NewEndPoint: pointAtOffset(edited, offset+1),
						})
						incremental, err := parser.ParseIncremental(edited, tree)
						if err != nil {
							t.Fatal(err)
						}
						defer incremental.Release()
						fresh, err := parser.ParseStrict(edited)
						if err != nil {
							t.Fatal(err)
						}
						defer fresh.Release()
						cEdited := cParser.Parse(edited, nil)
						if cEdited == nil {
							t.Fatal("nil edited C tree")
						}
						defer cEdited.Close()
						assertLockedCTreeExact(t, "edited fresh method", fresh, lang, cEdited)
						assertLockedCTreeExact(t, "edited incremental method", incremental, lang, cEdited)
						if allocations := testing.AllocsPerRun(10, func() {
							next, err := parser.ParseIncremental(edited, incremental)
							if err != nil {
								panic(err)
							}
							next.Release()
						}); allocations != 0 {
							t.Fatalf("no-edit reparse allocated %.2f times", allocations)
						}
					})
				}
			}
		})
	}
}
