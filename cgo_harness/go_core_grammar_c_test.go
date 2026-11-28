//go:build cgo && treesitter_c_parity

package cgoharness

import (
	"strings"
	"testing"

	"github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammargen"
	sitter "github.com/tree-sitter/go-tree-sitter"
)

func TestGeneratedGoCoreSpansMatchPinnedC(t *testing.T) {
	_, blob, err := grammargen.GenerateLanguageAndBlob(grammargen.GoCoreGrammar())
	if err != nil {
		t.Fatal(err)
	}
	lang, err := gotreesitter.LoadLanguage(blob)
	if err != nil {
		t.Fatal(err)
	}
	if len(lang.ExternalSymbols) != 0 {
		t.Fatalf("expected internal line terminators, got external symbols %v", lang.ExternalSymbols)
	}
	oracle, err := ParityCLanguage("go")
	if err != nil {
		t.Fatal(err)
	}
	cp := sitter.NewParser()
	defer cp.Close()
	if err := cp.SetLanguage(oracle); err != nil {
		t.Fatal(err)
	}

	const newlines = "package p\nfunc f(x int) int {\n switch x {\n case 1:\n  return 1\n\n case 2:\n  return 2\n }\n return 0\n}\n"
	for _, test := range []struct {
		name   string
		before string
		after  string
	}{
		{"switch with adjacent newlines", strings.Replace(newlines, "return 1\n\n", "return 1\n", 1), newlines},
		{"switch with whitespace between newlines", newlines, strings.Replace(newlines, "return 1\n\n", "return 1\n \n", 1)},
		{"six grouped fields", "package p\ntype T struct { a,b,c,d,e int }\n", "package p\ntype T struct { a,b,c,d,e,f int }\n"},
	} {
		t.Run(test.name, func(t *testing.T) {
			core := ccoreNew(lang)
			before, after := []byte(test.before), []byte(test.after)
			if stop := core.Parse(before, nil); stop != "" {
				t.Fatalf("initial ccore parse stopped: %s", stop)
			}
			initial := core.Tree()
			defer core.Release(initial)
			cInitial := cp.Parse(before, nil)
			if cInitial == nil {
				t.Fatal("pinned C returned no initial tree")
			}
			defer cInitial.Close()
			if want := ccoreCDigestChanges(cInitial, false); cInitial.RootNode().HasError() || core.DigestOf(initial, false) != want {
				t.Fatalf("initial trees differ or pinned C reported an error")
			}

			edit := ccoreDiffEdit(before, after)
			edited := core.Edit(initial, *edit)
			defer core.Release(edited)
			next, stop := core.ParseIncremental(after, edited, nil)
			defer core.Release(next)
			if stop != "" {
				t.Fatalf("incremental ccore parse stopped: %s", stop)
			}
			fresh := ccoreNew(lang)
			defer fresh.ReleaseLast()
			if stop := fresh.Parse(after, nil); stop != "" {
				t.Fatalf("fresh ccore parse stopped: %s", stop)
			}

			cInitial.Edit(ccoreSitterEdit(edit))
			cFresh, cNext := cp.Parse(after, nil), cp.Parse(after, cInitial)
			if cFresh == nil || cNext == nil {
				if cFresh != nil {
					cFresh.Close()
				}
				if cNext != nil {
					cNext.Close()
				}
				t.Fatal("pinned C returned no edited tree")
			}
			defer cFresh.Close()
			defer cNext.Close()
			want := ccoreCDigestChanges(cFresh, false)
			if cFresh.RootNode().HasError() || ccoreCDigestChanges(cNext, false) != want {
				t.Fatal("pinned C fresh and incremental trees differ or contain an error")
			}
			if got := fresh.Digest(); got != want {
				t.Errorf("fresh ccore digest %s, want pinned C %s", got, want)
			}
			if got := core.DigestOf(next, false); got != want {
				t.Errorf("incremental ccore digest %s, want pinned C %s", got, want)
			}
		})
	}
}
