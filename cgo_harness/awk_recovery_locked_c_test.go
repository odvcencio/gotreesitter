//go:build cgo && treesitter_c_parity

package cgoharness

import (
	"testing"

	gotreesitter "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
	"github.com/odvcencio/gotreesitter/internal/benchfixtures"
)

func TestAWKRecoveredProductionMatchesLockedC(t *testing.T) {
	source := awkRecoveredFixture(t)
	language := grammars.AwkLanguage()
	cLanguage, err := COracleLanguage("awk")
	if err != nil {
		t.Fatal(err)
	}
	cTree := awkCTree(t, cLanguage, source)
	defer cTree.Close()
	cDigest, err := COracleDeepDigest(cTree)
	if err != nil {
		t.Fatal(err)
	}
	parser := gotreesitter.NewParser(language)
	parser.SetAdmissionCandidateRoute(false)
	tree, err := parser.Parse(source)
	if err != nil {
		t.Fatal(err)
	}
	defer tree.Release()
	inspection, err := benchfixtures.InspectGoTree(tree.RootNode(), language)
	if err != nil {
		t.Fatal(err)
	}
	if inspection.SHA256 != cDigest {
		t.Fatalf("Go digest = %s, locked C = %s", inspection.SHA256, cDigest)
	}
	if divergence := FirstDivergenceDumpV1(tree.RootNode(), language, cTree.RootNode()); divergence != nil {
		t.Fatalf("locked-C tree divergence: %+v", divergence)
	}
}
