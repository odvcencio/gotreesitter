//go:build cgo && treesitter_c_parity

package cgoharness

import (
	"strings"
	"testing"

	gotreesitter "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
	sitter "github.com/tree-sitter/go-tree-sitter"
)

func TestComputedSetterLookaheadCParity(t *testing.T) {
	cases := []struct {
		name, source string
		jsx, typed   bool
	}{
		{"jsx_closing_tag", `class A { set [k](v = <a>x</a>) {} }`, true, false},
		{"jsx_apostrophe", `const o = { set [k](v = <p>don't</p>) {} };`, true, false},
		{"jsx_fragment", `class A { set [k](v = <>x</>) {} }`, true, false},
		{"jsx_expression", `const el = <div>{ ({ set [k](v = <b>y</b>) {} }) }</div>;`, true, false},
		{"xor_regex_quote", `class A { set [k](v = a ^ /'/.source.length) {} }`, false, false},
		{"xor_regex_bracket", `class A { get [a ^ /]/.source.length]() { return 1; } }`, false, false},
		{"xor_regex_paren", `class A { set [k](v = a ^ /)/.source.length) {} }`, false, false},
		{"private_in", `class A { #in = 1; set [k](v = this.#in / 2) {} }`, false, false},
		{"private_typeof", `class A { #typeof = 1; set [k](v = this.#typeof / 2) {} }`, false, false},
		{"private_new", `class A { #new = 1; set [k](v = this.#new / "]".length) {} }`, false, false},
		{"unicode_in", `class A { set [k](v = éin / 2) {} }`, false, false},
		{"unicode_new", `class A { set [k](v = ünew / 2) {} }`, false, false},
		{"unicode_cjk", `class A { set [k](v = 変in / 2) {} }`, false, false},
		{"typed_xor_default", `class A { set [k]({ a = b ^ /'/.source.length }: { a?: number }) {} }`, false, true},
		{"typed_private_default", `class A { #in = 1; set [k]({ a = this.#in / 2 }: { a?: number }) {} }`, false, true},
		{"typed_jsx_default", `class A { set [k]({ a = <a>x</a> }: { a?: any }) {} }`, true, true},
	}
	for _, name := range []string{"javascript", "typescript", "tsx"} {
		t.Run(name, func(t *testing.T) {
			goLang := grammars.DetectLanguageByName(name).Language()
			cLang, err := COracleLanguage(name)
			if err != nil {
				t.Fatal(err)
			}
			for _, c := range cases {
				if c.jsx && name == "typescript" || c.typed && name == "javascript" {
					continue
				}
				t.Run(c.name, func(t *testing.T) {
					cParser := sitter.NewParser()
					defer cParser.Close()
					if err := cParser.SetLanguage(cLang); err != nil {
						t.Fatal(err)
					}
					source := []byte(c.source)
					cTree := cParser.Parse(source, nil)
					if cTree == nil {
						t.Fatal("nil C tree")
					}
					defer cTree.Close()
					parser := gotreesitter.NewParser(goLang)
					tree, err := parser.ParseStrict(source)
					if err != nil {
						t.Fatal(err)
					}
					defer tree.Release()
					assertComputedSetterCParity(t, "fresh", tree, goLang, cTree, len(source))

					offset := strings.IndexByte(c.source, '[') + 1
					edited := []byte(c.source[:offset] + " " + c.source[offset:])
					tree.Edit(gotreesitter.InputEdit{
						StartByte: uint32(offset), OldEndByte: uint32(offset), NewEndByte: uint32(offset + 1),
						StartPoint: pointAtOffset(source, offset), OldEndPoint: pointAtOffset(source, offset), NewEndPoint: pointAtOffset(edited, offset+1),
					})
					incremental, err := parser.ParseIncremental(edited, tree)
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
					assertComputedSetterCParity(t, "edited fresh", fresh, goLang, cEdited, len(edited))
					assertComputedSetterCParity(t, "edited incremental", incremental, goLang, cEdited, len(edited))
				})
			}
		})
	}
}

func assertComputedSetterCParity(t *testing.T, phase string, tree *gotreesitter.Tree, lang *gotreesitter.Language, cTree *sitter.Tree, length int) {
	t.Helper()
	root := tree.RootNode()
	if root == nil {
		t.Fatal("nil Go root")
	}
	if root.HasErrorOrMissing() || tree.ParseStoppedEarly() || root.StartByte() != 0 || root.EndByte() != uint32(length) || cTree.RootNode().HasError() || cTree.RootNode().EndByte() != uint(length) {
		t.Fatalf("incomplete %s parse: Go=%s C=%s", phase, root.SExpr(lang), cTree.RootNode().ToSexp())
	}
	assertLockedCTreeExact(t, phase, tree, lang, cTree)
}
