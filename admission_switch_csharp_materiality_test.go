//go:build !gts_no_parsercorephase0

package gotreesitter_test

import (
	"crypto/sha256"
	"encoding/hex"
	"testing"

	gts "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
	"github.com/odvcencio/gotreesitter/internal/benchfixtures"
)

const csharpSmallVariableDeclarationsSource = `class A
{
    public void M()
    {
        foreach (int i in new[] { 1 })
        //           ^ variable
        {
            int j = i;
            //  ^ variable
        }

        var x = from a in sourceA
        //           ^ variable
        //                ^ variable
                join b in sourceB on a.FK equals b.PK
        //           ^ variable
        //                ^ variable
                group a by a.X into g
        //            ^ variable
        //                          ^ variable
                orderby g ascending
        //              ^ variable
                select new { A.A, B.B };
    }
}
`

func csharpSmallVariableDeclarationsBytes(t *testing.T) []byte {
	t.Helper()
	source := []byte(csharpSmallVariableDeclarationsSource)
	digest := sha256.Sum256(source)
	if len(source) != 642 || hex.EncodeToString(digest[:]) != "d532120abe52b3af477aa079e33a6998ef6b1a4370cff257277d319cd1912dd1" {
		t.Fatalf("embedded C# source identity is bytes=%d sha256=%x, want 642/d532120abe52b3af477aa079e33a6998ef6b1a4370cff257277d319cd1912dd1", len(source), digest)
	}
	return source
}

func TestAdmissionCandidateCSharpMaterialityOnlyAcceptance(t *testing.T) {
	source := csharpSmallVariableDeclarationsBytes(t)
	lang := grammars.CSharpLanguage()
	if lang.CompactPrimaryAcceptanceDerivationCertified {
		t.Fatal("C# unexpectedly has a primary-acceptance runtime profile")
	}

	production := gts.NewParser(lang)
	production.SetAdmissionCandidateRoute(false)
	productionTree, err := production.Parse(source)
	if err != nil {
		t.Fatalf("production parse: %v", err)
	}
	defer productionTree.Release()
	productionRoot := productionTree.RootNode()
	if productionRoot == nil {
		t.Fatal("production tree has no root")
	}
	if productionRoot.HasError() {
		t.Fatalf("production tree is not clean: %s", productionRoot.SExpr(lang))
	}
	productionDigest, err := benchfixtures.InspectGoTree(productionRoot, lang)
	if err != nil {
		t.Fatalf("inspect production tree: %v", err)
	}

	gts.ResetAdmissionCandidateCountersForTest()
	candidate := gts.NewParser(lang)
	candidate.SetAdmissionCandidateRoute(true)
	candidateTree, err := candidate.Parse(source)
	if err != nil {
		t.Fatalf("candidate parse: %v", err)
	}
	defer candidateTree.Release()
	routed, fallback := gts.AdmissionCandidateCounters()
	if routed != 1 || fallback != 0 {
		t.Fatalf("candidate counters = %d/%d, want 1/0; reason=%q", routed, fallback, gts.AdmissionCandidateLastFallbackReason())
	}
	candidateRoot := candidateTree.RootNode()
	if candidateRoot == nil {
		t.Fatal("candidate tree has no root")
	}
	if candidateRoot.HasError() {
		t.Fatalf("candidate tree is not clean: %s", candidateRoot.SExpr(lang))
	}
	candidateDigest, err := benchfixtures.InspectGoTree(candidateRoot, lang)
	if err != nil {
		t.Fatalf("inspect candidate tree: %v", err)
	}
	if candidateDigest.SHA256 != productionDigest.SHA256 {
		t.Fatalf("candidate digest %s differs from production %s", candidateDigest.SHA256, productionDigest.SHA256)
	}
}
