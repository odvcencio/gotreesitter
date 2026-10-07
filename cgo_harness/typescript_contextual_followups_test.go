//go:build cgo && treesitter_c_parity

package cgoharness

import (
	"strings"
	"testing"

	gotreesitter "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
	sitter "github.com/tree-sitter/go-tree-sitter"
)

func TestTypeScriptContextualFollowupsCParity(t *testing.T) {
	for _, name := range []string{"typescript", "tsx"} {
		t.Run(name, func(t *testing.T) {
			goLang := grammars.DetectLanguageByName(name).Language()
			cLang, err := COracleLanguage(name)
			if err != nil {
				t.Fatal(err)
			}

			for _, source := range []string{
				`class A { get [/"/.source]() { return 1; } }`,
				`class A { get [x / y]() { return 1; } }`,
				`class A { get [x++ / "/".length]() { return 1; } }`,
				`class A { get [/* lead */ /"/.source]() { return 1; } }`,
				`class A { get [obj.in / "/".length]() { return 1; } }`,
				"class A { get [(() => { x\nreturn /\"/.source; })()]() { return 1; } }",
				"class A { get [`outer${`inner]${x}`}value`]() { return 1; } }",
				`class A { static get [x]() { return 1; } }`,
				`class A { static set [x](value: number) {} }`,
				`class A { public static get [x](): number { return 1; } }`,
				`class A { public [x](): number { return 1; } }`,
				`class A { static get[x]() { return 1; } }`,
				`class A { get ["]"](): number { return 1; } }`,
				`class A { get ['['](): number { return 1; } }`,
				`class A { get ["\\\"]"](): number { return 1; } }`,
				"class A { get [`]`](): number { return 1; } }",
				`class A { get [/* ] */ x](): number { return 1; } }`,
				"class A { get [// ]\nx](): number { return 1; } }",
				`class A { get [keys["["]](): number { return 1; } }`,
				`class A { set [x](value: string = ")") {} }`,
				`class A { set [x](value = /"/) {} }`,
				`class A { set [x](value = x++ / "/".length) {} }`,
				`const value = flag ? get["]"]() : fallback;`,
				`const value = flag ? set["["]() : fallback;`,
				`type A = [keyof: string];`,
				`type A = [keyof?: string];`,
				`type Shape = { value: string }; type A = [keyof Shape];`,
				`type Shape = { value: string }; type A = [(keyof Shape)?];`,
				`type get = string; type A = [get, get?];`,
				`type async = string; type A = [async, async?];`,
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
					goParser := gotreesitter.NewParser(goLang)
					goTree, err := goParser.ParseStrict([]byte(source))
					if err != nil {
						t.Fatal(err)
					}

					defer goTree.Release()
					if goTree.RootNode().HasErrorOrMissing() || cTree.RootNode().HasError() || goTree.ParseStoppedEarly() || goTree.RootNode().EndByte() != uint32(len(source)) || cTree.RootNode().EndByte() != uint(len(source)) {
						t.Fatalf("incomplete fresh parse: Go=%s C=%s", goTree.RootNode().SExpr(goLang), cTree.RootNode().ToSexp())
					}

					assertLockedCTreeExact(t, "fresh", goTree, goLang, cTree)

					// Insert inside the computed name or tuple, so reuse crosses
					// the same token and field boundaries as the original parse.
					offset := strings.IndexByte(source, '[') + 1
					edited := []byte(source[:offset] + " " + source[offset:])
					goTree.Edit(gotreesitter.InputEdit{
						StartByte: uint32(offset), OldEndByte: uint32(offset), NewEndByte: uint32(offset + 1),
						StartPoint: pointAtOffset([]byte(source), offset), OldEndPoint: pointAtOffset([]byte(source), offset), NewEndPoint: pointAtOffset(edited, offset+1),
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
					if fresh.RootNode().HasErrorOrMissing() || incremental.RootNode().HasErrorOrMissing() || cEdited.RootNode().HasError() || fresh.ParseStoppedEarly() || incremental.ParseStoppedEarly() || fresh.RootNode().EndByte() != uint32(len(edited)) || incremental.RootNode().EndByte() != uint32(len(edited)) || cEdited.RootNode().EndByte() != uint(len(edited)) {
						t.Fatal("incomplete edited parse")
					}

					assertLockedCTreeExact(t, "edited fresh", fresh, goLang, cEdited)
					assertLockedCTreeExact(t, "edited incremental", incremental, goLang, cEdited)
				})
			}
		})
	}
}

func TestJavaScriptContextualSuffixCParity(t *testing.T) {
	goLang := grammars.JavascriptLanguage()
	cLang, err := COracleLanguage("javascript")
	if err != nil {
		t.Fatal(err)
	}

	for _, source := range []string{
		`class A { set [x](value = /"/) {} }`,
		`class A { set [x](value = x++ / "/".length) {} }`,
		`class A { get [/"/.source]() { return 1; } }`,
		`class A { get [x / y]() { return 1; } }`,
		`class A { get [x++ / "/".length]() { return 1; } }`,
		`class A { get [/* lead */ /"/.source]() { return 1; } }`,
		`class A { get [obj.in / "/".length]() { return 1; } }`,
		"class A { get [(() => { x\nreturn /\"/.source; })()]() { return 1; } }",
		"class A { get [`outer${`inner]${x}`}value`]() { return 1; } }",
		`class A { static get [x]() { return 1; } }`,
		`class A { get ["]"]() { return 1; } }`,
		`class A { get ['[']() { return 1; } }`,
		`class A { get [/* ] */ x]() { return 1; } }`,
		`const value = flag ? get["]"]() : fallback;`,
		`const value = flag ? set["["]() : fallback;`,
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
			assertLockedCTreeExact(t, "fresh", goTree, goLang, cTree)
		})
	}
}
