package grammars_test

import (
	"testing"

	gotreesitter "github.com/odvcencio/gotreesitter"
)

// Checking an error-bearing subtree before a later edit must visit each
// ancestor once. Rechecking its descendants at both node and entry boundaries
// makes this small chain take exponentially many visits.
func TestTreeEditDeepErrorDependency(t *testing.T) {
	language := &gotreesitter.Language{
		Name:        "edit_dependency_probe",
		SymbolNames: []string{"end", "chain"},
	}
	root := gotreesitter.NewLeafNode(gotreesitter.Symbol(65535), true, 0, 1,
		gotreesitter.Point{}, gotreesitter.Point{Column: 1})
	for depth := 0; depth < 40; depth++ {
		root = gotreesitter.NewParentNode(1, true, []*gotreesitter.Node{root}, nil, 0)
	}
	if !root.HasError() {
		t.Fatal("fixture must carry an error through every ancestor")
	}
	tree := gotreesitter.NewTree(root, []byte("! ab"), language)
	defer tree.Release()
	tree.Edit(gotreesitter.InputEdit{
		StartByte: 2, OldEndByte: 2, NewEndByte: 3,
		StartPoint:  gotreesitter.Point{Column: 2},
		OldEndPoint: gotreesitter.Point{Column: 2},
		NewEndPoint: gotreesitter.Point{Column: 3},
	})
	if root.StartByte() != 0 || root.EndByte() != 1 || root.HasChanges() {
		t.Fatal("an edit after the subtree changed its span or invalidated it")
	}
}
