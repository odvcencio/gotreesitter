package gotreesitter_test

import (
	"testing"

	"github.com/odvcencio/mane/gotreesitter"
	"github.com/odvcencio/mane/grammars"
)

func fullReplaceEdit(oldSrc, newSrc []byte) gotreesitter.InputEdit {
	oldEnd := pointAtOffset(oldSrc, len(oldSrc))
	newEnd := pointAtOffset(newSrc, len(newSrc))
	return gotreesitter.InputEdit{
		StartByte:   0,
		OldEndByte:  uint32(len(oldSrc)),
		NewEndByte:  uint32(len(newSrc)),
		StartPoint:  gotreesitter.Point{},
		OldEndPoint: oldEnd,
		NewEndPoint: newEnd,
	}
}

func FuzzGoParseDoesNotPanic(f *testing.F) {
	f.Add([]byte("package main\nfunc main() {}\n"))
	f.Add([]byte("package p\nvar x = \"hello\"\n"))
	f.Add([]byte("package p\nfunc f() { if ( }\n"))
	f.Add([]byte("package p\n/* unterminated"))
	f.Add([]byte("package p\nfunc f(){"))
	f.Add([]byte("package p\n" + "((((((((((((((\n"))

	lang := grammars.GoLanguage()
	parser := gotreesitter.NewParser(lang)

	f.Fuzz(func(t *testing.T, src []byte) {
		if len(src) > 1<<16 {
			t.Skip()
		}
		defer func() {
			if r := recover(); r != nil {
				t.Fatalf("panic while parsing fuzz input (%d bytes): %v", len(src), r)
			}
		}()

		tree := parser.ParseWithTokenSource(src, mustGoTokenSource(t, src, lang))
		if tree == nil {
			t.Fatal("parse returned nil tree")
		}
	})
}

func FuzzGoParseIncrementalDoesNotPanic(f *testing.F) {
	f.Add([]byte("package main\nfunc main() {}\n"), []byte("package main\nfunc main(){println(1)}\n"))
	f.Add([]byte("package p\nvar x = 1\n"), []byte("package p\nvar x = \"\n"))
	f.Add([]byte("package p\nfunc f() {\n\treturn\n}\n"), []byte("package p\nfunc f() {\n\treturn 1\n}\n"))

	lang := grammars.GoLanguage()
	parser := gotreesitter.NewParser(lang)

	f.Fuzz(func(t *testing.T, oldSrc, newSrc []byte) {
		if len(oldSrc) > 1<<15 || len(newSrc) > 1<<15 {
			t.Skip()
		}
		defer func() {
			if r := recover(); r != nil {
				t.Fatalf("panic while incremental parsing fuzz input old=%d new=%d: %v", len(oldSrc), len(newSrc), r)
			}
		}()

		oldTree := parser.ParseWithTokenSource(oldSrc, mustGoTokenSource(t, oldSrc, lang))
		if oldTree == nil {
			t.Fatal("initial parse returned nil tree")
		}

		oldTree.Edit(fullReplaceEdit(oldSrc, newSrc))
		newTree := parser.ParseIncrementalWithTokenSource(newSrc, oldTree, mustGoTokenSource(t, newSrc, lang))
		if newTree == nil {
			t.Fatal("incremental parse returned nil tree")
		}

		if root := newTree.RootNode(); root != nil {
			_ = root.ChildCount()
		}
	})
}
