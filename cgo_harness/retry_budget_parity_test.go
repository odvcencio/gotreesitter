//go:build cgo && treesitter_c_parity

package cgoharness

import (
	"os"
	"testing"

	gts "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
	"github.com/odvcencio/gotreesitter/internal/benchfixtures"
	sitter "github.com/tree-sitter/go-tree-sitter"
)

func TestRetryBudgetPreservesRequiredGoMergeLockedC(t *testing.T) {
	source, err := os.ReadFile("../testdata/work_count/retry_go_query_kotlin_regression.go")
	if err != nil {
		t.Fatal(err)
	}
	language, err := COracleLanguage("go")
	if err != nil {
		t.Fatal(err)
	}
	parser := sitter.NewParser()
	defer parser.Close()
	if err := parser.SetLanguage(language); err != nil {
		t.Fatal(err)
	}
	cTree := parser.Parse(source, nil)
	if cTree == nil {
		t.Fatal("fresh C returned no tree")
	}
	defer cTree.Close()
	cDigest, err := COracleDeepDigest(cTree)
	if err != nil {
		t.Fatal(err)
	}
	for _, budget := range []bool{false, true} {
		lang := *grammars.GoLanguage()
		lang.FullParseRetryWorkBudgetEnabled = budget
		tree, err := gts.NewParser(&lang).Parse(source)
		if err != nil {
			t.Fatal(err)
		}
		inspection, err := benchfixtures.InspectGoTree(tree.RootNode(), &lang)
		root := tree.RootNode()
		if err != nil {
			tree.Release()
			t.Fatal(err)
		}
		if inspection.SHA256 != cDigest || uint(root.StartByte()) != cTree.RootNode().StartByte() || uint(root.EndByte()) != cTree.RootNode().EndByte() || root.HasError() != cTree.RootNode().HasError() {
			t.Errorf("budget=%t Go digest=%s, fresh locked C=%s", budget, inspection.SHA256, cDigest)
		}
		tree.Release()
	}
}
