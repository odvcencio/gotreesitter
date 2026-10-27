//go:build cgo && treesitter_c_parity

package cgoharness

import (
	"strings"
	"testing"

	gotreesitter "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
	sitter "github.com/tree-sitter/go-tree-sitter"
)

// +case:id=generic_tagged_template,desc=`Generic tagged templates and neighboring comparison forms`,expect=`Fresh and incremental trees match locked C`
func TestTypeScriptGenericTaggedTemplateCParity(t *testing.T) {
	for _, name := range []string{"typescript", "tsx"} {
		t.Run(name, func(t *testing.T) {
			goLang := grammars.DetectLanguageByName(name).Language()
			cLang, err := COracleLanguage(name)
			if err != nil {
				t.Fatal(err)
			}

			for _, source := range []string{
				"const value = tag<string>`text`;",
				"const value = tag<string[]>`text ${item}`;",
				"const value = tag<Array<string[]>>`text`;",
				"const value = sql.query<Row, Params>`select 1`;",
				"const value = { result: tag<string[]>`text` };",
				"const value = tag<string[]>`\n// template text\n/* template text */\n`;\n// actual comment",
				"const value = tag<string[]>`${flag ? `/*` : /{/.test(text)}`;\n// actual comment",
				"const value = tag<string[]>`${\n// interpolation comment\nitem\n}`;",
				"const value = tag`text`;",
				"const value = f<T>(x);",
				"const value = a < b || c > `text`;",
				"const value = a < b;",
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

					// Insert inside the type arguments or template, where the
					// generic call and comparison readings diverge.
					offset := strings.IndexAny(source, "<`") + 1
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
