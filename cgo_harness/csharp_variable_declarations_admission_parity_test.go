//go:build cgo && treesitter_c_parity

package cgoharness

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"testing"

	gotreesitter "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
	sitter "github.com/tree-sitter/go-tree-sitter"
)

const csharpVariableDeclarationsSourceSHA256 = "d532120abe52b3af477aa079e33a6998ef6b1a4370cff257277d319cd1912dd1"

func TestCSharpVariableDeclarationsAdmissionLockedCParity(t *testing.T) {
	source, err := os.ReadFile("../testdata/admission_direct/recursive_insert/c_sharp.cs")
	if err != nil {
		t.Fatalf("read C# variableDeclarations source: %v", err)
	}
	if len(source) != 642 {
		t.Fatalf("source bytes=%d, want 642", len(source))
	}
	sum := sha256.Sum256(source)
	if got := hex.EncodeToString(sum[:]); got != csharpVariableDeclarationsSourceSHA256 {
		t.Fatalf("source SHA-256=%s, want %s", got, csharpVariableDeclarationsSourceSHA256)
	}

	language := grammars.CSharpLanguage()
	if language == nil {
		t.Fatal("C# Go language is nil")
	}
	cLanguage, err := COracleLanguage("c_sharp")
	if err != nil {
		t.Fatalf("load locked C# language: %v", err)
	}
	cParser := sitter.NewParser()
	t.Cleanup(cParser.Close)
	if err := cParser.SetLanguage(cLanguage); err != nil {
		t.Fatalf("set locked C# language: %v", err)
	}
	cTree := cParser.Parse(source, nil)
	if cTree == nil || cTree.RootNode() == nil {
		t.Fatal("locked C# parser returned no tree")
	}
	t.Cleanup(cTree.Close)
	requireCSharpVariableDeclarationsCRoot(t, cTree.RootNode(), len(source))

	productionParser := gotreesitter.NewParser(language)
	productionParser.SetAdmissionCandidateRoute(false)
	productionTree, err := productionParser.Parse(source)
	if err != nil {
		t.Fatalf("production parse: %v", err)
	}
	t.Cleanup(func() { productionTree.Release() })
	requireCSharpVariableDeclarationsRoot(t, "production", productionTree.RootNode(), len(source))
	assertLockedCTreeExact(t, "C# variableDeclarations production", productionTree, language, cTree)

	routedBefore, fallbackBefore := gotreesitter.AdmissionCandidateCounters()
	candidateParser := gotreesitter.NewParser(language)
	candidateParser.SetAdmissionCandidateRoute(true)
	candidateTree, err := candidateParser.Parse(source)
	if err != nil {
		t.Fatalf("compact candidate parse: %v", err)
	}
	t.Cleanup(func() { candidateTree.Release() })
	routedAfter, fallbackAfter := gotreesitter.AdmissionCandidateCounters()
	routed := routedAfter - routedBefore
	fallback := fallbackAfter - fallbackBefore
	if routed != 1 || fallback != 0 {
		t.Fatalf(
			"compact candidate counters routed=%d fallback=%d reason=%q, want 1/0",
			routed,
			fallback,
			gotreesitter.AdmissionCandidateLastFallbackReason(),
		)
	}
	requireCSharpVariableDeclarationsRoot(t, "compact candidate", candidateTree.RootNode(), len(source))
	assertLockedCTreeExact(t, "C# variableDeclarations compact candidate", candidateTree, language, cTree)
}

func requireCSharpVariableDeclarationsRoot(t *testing.T, label string, root *gotreesitter.Node, sourceLen int) {
	t.Helper()
	if root == nil {
		t.Fatalf("%s root is nil", label)
	}
	if root.HasError() {
		t.Fatalf("%s root has an error", label)
	}
	if root.StartByte() != 0 || root.EndByte() != uint32(sourceLen) {
		t.Fatalf("%s root span=%d..%d, want 0..%d", label, root.StartByte(), root.EndByte(), sourceLen)
	}
}

func requireCSharpVariableDeclarationsCRoot(t *testing.T, root *sitter.Node, sourceLen int) {
	t.Helper()
	if root == nil {
		t.Fatal("locked C root is nil")
	}
	if root.HasError() {
		t.Fatal("locked C root has an error")
	}
	if root.StartByte() != 0 || root.EndByte() != uint(sourceLen) {
		t.Fatalf("locked C root span=%d..%d, want 0..%d", root.StartByte(), root.EndByte(), sourceLen)
	}
}
