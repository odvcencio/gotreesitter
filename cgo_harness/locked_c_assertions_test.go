//go:build cgo && treesitter_c_parity

package cgoharness

import (
	"fmt"
	"testing"

	gotreesitter "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/internal/benchfixtures"
	sitter "github.com/tree-sitter/go-tree-sitter"
)

func assertG18LockedCExact(
	t *testing.T,
	label string,
	tree *gotreesitter.Tree,
	language *gotreesitter.Language,
	cTree *sitter.Tree,
) {
	t.Helper()
	if err := g18LockedCExactError(tree, language, cTree); err != nil {
		t.Fatalf("%s: %v", label, err)
	}
	goInspection, err := benchfixtures.InspectGoTree(tree.RootNode(), language)
	if err != nil {
		t.Fatalf("%s inspect Go tree: %v", label, err)
	}
	t.Logf("%s locked-C deep digest=%s", label, goInspection.SHA256)
}

func g18LockedCExactError(tree *gotreesitter.Tree, language *gotreesitter.Language, cTree *sitter.Tree) error {
	goRoot := tree.RootNode()
	cRoot := cTree.RootNode()
	if diff := FirstDivergenceDumpV1(goRoot, language, cRoot); diff != nil {
		return fmt.Errorf("node or field divergence: %+v", diff)
	}
	if err := g18FirstLockedCFlagDifference(goRoot, language, cRoot, "/"+goRoot.Type(language)); err != nil {
		return fmt.Errorf("flag divergence: %w", err)
	}
	goInspection, err := benchfixtures.InspectGoTree(goRoot, language)
	if err != nil {
		return fmt.Errorf("inspect Go tree: %w", err)
	}
	cDigest, err := COracleDeepDigest(cTree)
	if err != nil {
		return fmt.Errorf("inspect C tree: %w", err)
	}
	if goInspection.SHA256 != cDigest {
		return fmt.Errorf("deep digest Go=%s C=%s", goInspection.SHA256, cDigest)
	}
	return nil
}

func g18FirstLockedCFlagDifference(
	goNode *gotreesitter.Node,
	language *gotreesitter.Language,
	cNode *sitter.Node,
	path string,
) error {
	if goNode == nil || cNode == nil {
		return fmt.Errorf("%s: nil mismatch Go=%v C=%v", path, goNode == nil, cNode == nil)
	}
	if goNode.IsMissing() != cNode.IsMissing() {
		return fmt.Errorf("%s: missing Go=%v C=%v", path, goNode.IsMissing(), cNode.IsMissing())
	}
	if goNode.IsError() != cNode.IsError() {
		return fmt.Errorf("%s: error Go=%v C=%v", path, goNode.IsError(), cNode.IsError())
	}
	for index := 0; index < goNode.ChildCount(); index++ {
		goChild := goNode.Child(index)
		cChild := cNode.Child(uint(index))
		childPath := fmt.Sprintf("%s/%s[%d]", path, goChild.Type(language), index)
		if err := g18FirstLockedCFlagDifference(goChild, language, cChild, childPath); err != nil {
			return err
		}
	}
	return nil
}
