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

// +case:id=async_generic_arrow,desc=`Async generic arrows and neighboring expression forms`,expect=`Fresh and incremental trees match locked C through both routes`
func TestTypeScriptAsyncGenericArrowLockedC(t *testing.T) {
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
				`const invoke = async <T>(value: T): Promise<T> => value;`,
				`const invoke = async <T>(value: T) => value;`,
				`const invoke = async <T,>(value: T): Promise<T> => value;`,
				`const invoke = async <T extends object>(value: T): Promise<T> => value;`,
				`const invoke = async <T, U>(value: T, other: U): Promise<T> => value;`,
				`const invoke = async /* generic */ <T>(value: T): Promise<T> => { return value; };`,
				`const invoke = async <T>(value: T): Promise<T> => value; function after(x: number) { return x; }`,
				`const invoke = wrap<number>(async <T>(value: T): Promise<T> => value);`,
				`const invoke = async <A>(a: A) => async <B>(b: B) => async <C>(c: C) => [a, b, c];`,
				`class Handler { invoke = async <T>(value: T): Promise<T> => value; }`,
				`const first = async <T>(value: T) => value; const second = async <U>(other: U) => other;`,
				`const result = async < limit;`,
				`const result = async<T>(value);`,
				`const invoke = async (value: number) => value;`,
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
						assertLockedCTreeExact(t, "fresh async expression", tree, lang, cTree)
						if tree.RootNode().HasErrorOrMissing() || tree.ParseStoppedEarly() || tree.RootNode().EndByte() != uint32(len(input)) {
							t.Fatal("incomplete fresh tree")
						}

						// Edit the contextual keyword's suffix, where generic and comparison
						// interpretations diverge, then compare both Go paths with fresh C.
						offset := strings.Index(source, "async") + len("async")
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
						assertLockedCTreeExact(t, "edited fresh async expression", fresh, lang, cEdited)
						assertLockedCTreeExact(t, "edited incremental async expression", incremental, lang, cEdited)
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
