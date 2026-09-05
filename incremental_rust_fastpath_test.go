package gotreesitter_test

import (
	"bytes"
	"testing"

	gts "github.com/odvcencio/gotreesitter"
	grm "github.com/odvcencio/gotreesitter/grammars"
)

func TestRustLineCommentTextEditUsesInvariantReuse(t *testing.T) {
	lang := grm.RustLanguage()
	oldSource := []byte("// Copyright 2012-2014\nfn main() {}\n")
	offset := bytes.Index(oldSource, []byte("2012"))
	if offset < 0 {
		t.Fatal("fixture missing edit marker")
	}
	offset += len("201")
	newSource := append([]byte(nil), oldSource...)
	newSource[offset] = '3'

	parser := gts.NewParser(lang)
	fresh, err := parser.Parse(newSource)
	if err != nil {
		t.Fatalf("fresh parse: %v", err)
	}
	defer fresh.Release()
	oldTree, err := parser.Parse(oldSource)
	if err != nil {
		t.Fatalf("old parse: %v", err)
	}
	defer oldTree.Release()

	edit := gts.InputEdit{
		StartByte:   uint32(offset),
		OldEndByte:  uint32(offset + 1),
		NewEndByte:  uint32(offset + 1),
		StartPoint:  pointForOffset(oldSource, offset),
		OldEndPoint: pointForOffset(oldSource, offset+1),
		NewEndPoint: pointForOffset(newSource, offset+1),
	}
	oldTree.Edit(edit)
	result, err := parser.ParseWith(newSource, gts.WithOldTree(oldTree), gts.WithProfiling())
	if err != nil {
		t.Fatalf("incremental parse: %v", err)
	}
	defer result.Tree.Release()
	if got, want := result.Tree.RootNode().SExpr(lang), fresh.RootNode().SExpr(lang); got != want {
		t.Fatalf("incremental tree mismatch\n got: %s\nwant: %s", got, want)
	}
	if !result.ProfileAvailable {
		t.Fatal("profile unavailable")
	}
	if result.Profile.ReuseUnsupported {
		t.Fatalf("reuse unsupported: %s", result.Profile.ReuseUnsupportedReason)
	}
	requireReleaseSameWidthReparse(t, result.Profile)
	requireIncrementalDeepTreeMatchesFresh(t, result.Tree, fresh, lang)
	if result.Profile.ReusedSubtrees == 0 {
		t.Fatal("ordinary Rust reuse lost unchanged subtrees")
	}
}
