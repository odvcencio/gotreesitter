//go:build cgo && treesitter_c_parity && gts_parsercorephase0 && !gts_no_parsercorephase0

package cgoharness

import (
	"testing"

	gts "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
	sitter "github.com/tree-sitter/go-tree-sitter"
)

func TestTypeScriptCleanSiblingAdoptionLockedC(t *testing.T) {
	source := []byte("foo<A00>(2);\n")
	language := grammars.TypescriptLanguage()
	parser := gts.NewParser(language)
	parser.SetAdmissionCandidateRoute(true)
	beforeRouted, beforeFallback := gts.AdmissionCandidateCounters()
	tree, err := parser.Parse(source)
	if err != nil || tree == nil {
		t.Fatalf("Go parse: %v", err)
	}
	defer tree.Release()
	routed, fallback := gts.AdmissionCandidateCounters()
	if routed-beforeRouted != 1 || fallback-beforeFallback != 0 {
		t.Fatalf("compact route=%d fallback=%d reason=%q", routed-beforeRouted, fallback-beforeFallback, gts.AdmissionCandidateLastFallbackReason())
	}
	cLanguage, err := ParityCLanguage("typescript")
	if err != nil {
		t.Fatal(err)
	}
	oracleParser := sitter.NewParser()
	defer oracleParser.Close()
	if err := oracleParser.SetLanguage(cLanguage); err != nil {
		t.Fatal(err)
	}
	oracle := oracleParser.Parse(source, nil)
	if oracle == nil {
		t.Fatal("C parse returned no tree")
	}
	defer oracle.Close()
	assertLockedCTreeExact(t, "clean sibling adoption", tree, language, oracle)
}
