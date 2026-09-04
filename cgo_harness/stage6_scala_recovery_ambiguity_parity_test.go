//go:build cgo && treesitter_c_parity

package cgoharness

import (
	"testing"

	gotreesitter "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
	"github.com/odvcencio/gotreesitter/internal/benchfixtures"
	sitter "github.com/tree-sitter/go-tree-sitter"
)

func TestStage6ScalaRecoveryThroughAmbiguityParity(t *testing.T) {
	source := []byte("val f = (: Int) => x\n")
	const wantSExpr = "(compilation_unit (val_definition (identifier) (lambda_expression (bindings (ERROR) (binding (identifier))) (identifier))))"
	const wantDigest = "454a064cdf50bdcfa6eb1cdfd1faaebcda41a02995dea6befbe356bb42ed8dda"

	cLanguage, err := ParityCLanguage("scala")
	if err != nil {
		t.Fatalf("load locked Scala C parser: %v", err)
	}
	cParser := sitter.NewParser()
	t.Cleanup(cParser.Close)
	if err := cParser.SetLanguage(cLanguage); err != nil {
		t.Fatalf("set locked Scala C language: %v", err)
	}
	cTree := cParser.Parse(source, nil)
	if cTree == nil || cTree.RootNode() == nil {
		t.Fatal("locked C parser returned no root")
	}
	t.Cleanup(cTree.Close)
	cRoot := cTree.RootNode()
	if got := formatCNodeSExpr(cRoot); got != wantSExpr {
		t.Fatalf("locked C S-expression=%q, want %q", got, wantSExpr)
	}
	cDigest, err := COracleDeepDigest(cTree)
	if err != nil {
		t.Fatalf("inspect locked Scala C tree: %v", err)
	}
	if cDigest != wantDigest {
		t.Fatalf("locked Scala C digest=%s, want %s", cDigest, wantDigest)
	}

	entry := grammars.DetectLanguageByName("scala")
	if entry == nil || entry.Language() == nil {
		t.Fatal("Scala Go grammar is unavailable")
	}
	goLanguage := entry.Language()
	goParser := gotreesitter.NewParser(goLanguage)
	goParser.SetAdmissionCandidateRoute(true)
	goTree, err := goParser.Parse(source)
	if err != nil {
		t.Fatalf("compact candidate parse: %v", err)
	}
	if goTree == nil || goTree.RootNode() == nil {
		t.Fatal("compact candidate returned no root")
	}
	t.Cleanup(goTree.Release)
	goRoot := goTree.RootNode()
	if got := goRoot.SExpr(goLanguage); got != wantSExpr {
		t.Fatalf("compact Scala S-expression=%q, want %q; fallback=%q", got, wantSExpr, gotreesitter.AdmissionCandidateLastFallbackReason())
	}
	if diff := FirstDivergenceDumpV1(goRoot, goLanguage, cRoot); diff != nil {
		t.Fatalf("compact Scala tree diverges from locked C: %+v", *diff)
	}
	inspection, err := benchfixtures.InspectGoTree(goRoot, goLanguage)
	if err != nil {
		t.Fatalf("inspect compact Scala tree: %v", err)
	}
	if inspection.SHA256 != cDigest {
		t.Fatalf("compact Scala digest=%s, want locked C %s", inspection.SHA256, cDigest)
	}
}
