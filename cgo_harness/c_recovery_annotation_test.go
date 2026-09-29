//go:build cgo && treesitter_c_parity

package cgoharness

import (
	"testing"

	gts "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
	sitter "github.com/tree-sitter/go-tree-sitter"
)

// A completed function must not make a missing-token alternative appear to
// have made progress since the parameter error. C resets that count at pause.
func TestCRecoveryAnnotation(t *testing.T) {
	source := []byte("int ok(void) { return 0; }\nint f(int argc UNUSED, const char **argv UNUSED) { return 1; }\n")
	lang := grammars.CLanguage()
	cl, err := ParityCLanguage("c")
	if err != nil {
		t.Fatal(err)
	}
	cp := sitter.NewParser()
	defer cp.Close()
	if err := cp.SetLanguage(cl); err != nil {
		t.Fatal(err)
	}
	ct := cp.Parse(source, nil)
	if ct == nil {
		t.Fatal("C parser returned no tree")
	}
	defer ct.Close()
	if !ct.RootNode().HasError() {
		t.Fatal("fixture must exercise recovery")
	}
	p := gts.NewParser(lang)
	tree, err := p.ParseWithTokenSource(source, grammars.NewCTokenSourceOrEOF(source, lang))
	if err != nil {
		t.Fatal(err)
	}
	if tree == nil {
		t.Fatal("Go parser returned no tree")
	}
	defer tree.Release()
	if !tree.RootNode().HasError() {
		t.Fatal("Go tree lost the recovery error")
	}
	if diff := firstLockedCTreeFlagDivergence(tree.RootNode(), lang, ct.RootNode(), "/"); diff != nil {
		t.Fatal(diff)
	}
	if diff := FirstDivergenceDumpV1(tree.RootNode(), lang, ct.RootNode()); diff != nil {
		t.Fatalf("%+v", diff)
	}
}
