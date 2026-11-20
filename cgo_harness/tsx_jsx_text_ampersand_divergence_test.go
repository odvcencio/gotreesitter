//go:build cgo && treesitter_c_parity

package cgoharness

import (
	"strings"
	"testing"

	gotreesitter "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
	sitter "github.com/tree-sitter/go-tree-sitter"
)

// The effective locked TSX scanner includes the JSX text fix from #1242.
func TestTsxJsxTextAmpersandLockedC(t *testing.T) {
	cLang, err := COracleLanguage("tsx")
	if err != nil {
		t.Fatal(err)
	}
	goLang := grammars.TsxLanguage()

	for _, test := range []struct {
		name   string
		source string
	}{
		{name: "spaced", source: "const a = <p>Org & Team</p>;\n"},
		{name: "tight", source: "const a = <p>AT&T</p>;\n"},
		{name: "alone", source: "const a = <p>&</p>;\n"},
	} {
		t.Run(test.name, func(t *testing.T) {
			source := []byte(test.source)

			goTree, err := gotreesitter.NewParser(goLang).Parse(source)
			if err != nil {
				t.Fatal(err)
			}
			defer goTree.Release()
			goRoot := goTree.RootNode()
			if goRoot == nil {
				t.Fatal("Go parse returned no root")
			}
			if goRoot.HasError() || goRoot.EndByte() != uint32(len(source)) {
				t.Fatalf("gotreesitter regressed issue #1242: parse is incomplete: %s", goRoot.SExpr(goLang))
			}

			cParser := sitter.NewParser()
			defer cParser.Close()
			if err := cParser.SetLanguage(cLang); err != nil {
				t.Fatal(err)
			}
			cTree := cParser.Parse(source, nil)
			if cTree == nil || cTree.RootNode() == nil {
				t.Fatal("locked C parse returned no root")
			}
			defer cTree.Close()
			cRoot := cTree.RootNode()
			if cRoot.HasError() || cRoot.EndByte() != uint(len(source)) {
				t.Fatalf("patched C rejected JSX text: %s", cRoot.ToSexp())
			}
			if diff := FirstDivergenceDumpV1(goRoot, goLang, cRoot); diff != nil {
				t.Fatalf("public Go differs from patched C: %+v", diff)
			}
		})
	}
}

// A semicolon beyond the end of an earlier jsx_text token determines whether
// the following bytes belong to that token or to html_character_reference.
func TestTsxJsxTextEntityEditAndUndoLockedC(t *testing.T) {
	cLang, err := COracleLanguage("tsx")
	if err != nil {
		t.Fatal(err)
	}
	lang := grammars.TsxLanguage()
	for _, prefix := range []string{"", "word "} {
		t.Run("prefix_"+strings.TrimSpace(prefix), func(t *testing.T) {
			source := "const a = <p>" + prefix + "&amp; tail</p>;"
			at := strings.Index(source, "amp;") + len("amp")
			without := source[:at] + source[at+1:]
			goParser := gotreesitter.NewParser(lang)
			cParser := sitter.NewParser()
			defer cParser.Close()
			if err := cParser.SetLanguage(cLang); err != nil {
				t.Fatal(err)
			}
			var oldGo *gotreesitter.Tree
			var oldC *sitter.Tree
			for step, text := range []string{source, without, source} {
				if step > 0 {
					oldEnd, newEnd := at+1, at
					if step == 2 {
						oldEnd, newEnd = newEnd, oldEnd
					}
					edit := gotreesitter.InputEdit{
						StartByte: uint32(at), OldEndByte: uint32(oldEnd), NewEndByte: uint32(newEnd),
						StartPoint: gotreesitter.Point{Column: uint32(at)}, OldEndPoint: gotreesitter.Point{Column: uint32(oldEnd)}, NewEndPoint: gotreesitter.Point{Column: uint32(newEnd)},
					}
					oldGo.Edit(edit)
					oldC.Edit(gradSitterEdit(edit))
				}
				src := []byte(text)
				var nextGo *gotreesitter.Tree
				var err error
				if step == 0 {
					nextGo, err = goParser.Parse(src)
				} else {
					nextGo, err = goParser.ParseIncremental(src, oldGo)
				}
				if err != nil {
					t.Fatal(err)
				}
				defer nextGo.Release()
				nextC := cParser.Parse(src, oldC)
				if nextC == nil {
					t.Fatalf("step %d: C parse returned nil", step)
				}
				defer nextC.Close()
				freshC := receiptCTree(t, cLang, src)
				defer freshC.Close()
				freshGo, err := gotreesitter.NewParser(lang).Parse(src)
				if err != nil {
					t.Fatal(err)
				}
				defer freshGo.Release()
				want := freshC.RootNode()
				if want.HasError() || want.EndByte() != uint(len(src)) || strings.Contains(want.ToSexp(), "(html_character_reference)") != (step != 1) {
					t.Fatalf("step %d: unexpected fresh C tree: %s", step, want.ToSexp())
				}
				for _, tree := range []*gotreesitter.Tree{nextGo, freshGo} {
					if diff := FirstDivergenceDumpV1(tree.RootNode(), lang, want); diff != nil {
						t.Fatalf("step %d: fresh C parity: %+v", step, diff)
					}
				}
				if diff := FirstDivergenceDumpV1(nextGo.RootNode(), lang, nextC.RootNode()); diff != nil {
					t.Fatalf("step %d: incremental C parity: %+v", step, diff)
				}
				oldGo, oldC = nextGo, nextC
			}
		})
	}
}
