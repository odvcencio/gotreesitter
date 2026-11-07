package query_test

import (
	"testing"

	"github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
	"github.com/odvcencio/gotreesitter/internal/benchfixtures"
)

func FuzzTaggerTag(f *testing.F) {
	f.Add([]byte("package main\nfunc main() {}\n"))
	f.Add([]byte("package p\nfunc f() { g() }\n"))
	f.Add([]byte("package p\ntype Foo struct{}\nfunc (f Foo) Bar() {}\n"))
	f.Add([]byte("package p\nvar x = 1\n"))
	f.Add([]byte(""))
	f.Add([]byte("package p\nfunc f() { if ( }\n"))
	f.Add([]byte("package p\n/* unterminated"))
	f.Add([]byte("package p\n" + "((((((((((((((\n"))

	lang := grammars.GoLanguage()
	tagger, err := gotreesitter.NewTagger(lang, benchfixtures.GoTagsQuery)
	if err != nil {
		f.Fatalf("failed to create tagger: %v", err)
	}

	f.Fuzz(func(t *testing.T, src []byte) {
		if len(src) > 1<<16 {
			t.Skip()
		}
		defer func() {
			if r := recover(); r != nil {
				t.Fatalf("panic while tagging fuzz input (%d bytes): %v", len(src), r)
			}
		}()

		tags := tagger.Tag(src)
		_ = tags
	})
}
