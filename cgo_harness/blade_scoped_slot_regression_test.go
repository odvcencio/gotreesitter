//go:build cgo && treesitter_c_parity

package cgoharness

import (
	"testing"

	gotreesitter "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
)

// TestBladeScopedSlotBareCloseCOracle pins the fix ported from upstream
// tree-sitter-blade commit b5291d1b (PR #133, "support scoped slots"),
// part of the 42b3c5a06bc2 -> b5291d1ba207 grammar bump, against the real C
// tree-sitter oracle.
//
// Before the fix, tree-sitter-blade's src/tag.h classified "<x-slot:name>"
// as a plain CUSTOM tag, so a bare closing "</x-slot>" (customName
// "X-SLOT") never matched the open tag's customName ("X-SLOT:NAME") and the
// parser recorded an erroneous_end_tag. The b5291d1b tag.h added a
// dedicated X_SLOT tag type whose tag_eq lets a bare "</x-slot>" close any
// open "<x-slot:name>" tag. The construct is wrapped in <div>, not a Blade
// component tag such as <x-alert>, to isolate this fix from an unrelated,
// pre-existing GLR gap on the Go runtime with mismatched end tags directly
// inside a custom-tag-typed container.
func TestBladeScopedSlotBareCloseCOracle(t *testing.T) {
	source := []byte("<div><x-slot:title>content</x-slot></div>")

	cLanguage, err := ParityCLanguage("blade")
	if err != nil {
		t.Skipf("C blade oracle unavailable: %v", err)
	}
	cTree := compactT3ParseC(t, cLanguage, source)
	defer cTree.Close()
	cRoot := cTree.RootNode()
	if cRoot == nil {
		t.Fatal("blade C oracle returned no root")
	}
	if cRoot.HasError() {
		t.Fatalf("blade C oracle root has error:\n%s", dumpCTree(cRoot, 0))
	}

	goLanguage := grammars.BladeLanguage()
	goParser := gotreesitter.NewParser(goLanguage)
	goTree, err := goParser.Parse(source)
	if err != nil {
		t.Fatalf("blade Go parse: %v", err)
	}
	defer goTree.Release()
	goRoot := goTree.RootNode()
	if goRoot == nil {
		t.Fatal("blade Go parse returned no root")
	}
	if goRoot.HasError() {
		t.Fatalf("blade Go root has error:\n%s", dumpGoTree(goRoot, goLanguage, 0))
	}

	var errs []string
	compareNodes(goRoot, goLanguage, cRoot, "root", &errs)
	if len(errs) != 0 {
		t.Fatalf(
			"blade Go/C scoped-slot tree diverged:\n%s\n\nGo: %s\n%s\nC: %s\n%s",
			joinTopErrors(errs),
			goRoot.SExpr(goLanguage),
			dumpGoTree(goRoot, goLanguage, 0),
			cRoot.ToSexp(),
			dumpCTree(cRoot, 0),
		)
	}

	if bad := findBladeGoNodeByType(goRoot, goLanguage, "erroneous_end_tag"); bad != nil {
		t.Fatalf("blade Go bare </x-slot> did not close <x-slot:title>: %s", goRoot.SExpr(goLanguage))
	}
}

func findBladeGoNodeByType(node *gotreesitter.Node, lang *gotreesitter.Language, name string) *gotreesitter.Node {
	if node == nil {
		return nil
	}
	if node.Type(lang) == name {
		return node
	}
	for i := 0; i < node.ChildCount(); i++ {
		if found := findBladeGoNodeByType(node.Child(i), lang, name); found != nil {
			return found
		}
	}
	return nil
}
