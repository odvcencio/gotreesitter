//go:build cgo && treesitter_c_parity

package cgoharness

import (
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	gotreesitter "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/internal/benchfixtures"
	sitter "github.com/tree-sitter/go-tree-sitter"
)

// TestWolframDispatchRetirementLockedCParity compares all three parser-produced
// Wolfram A0 witnesses with the locked C parser on raw and production routes.
func TestWolframDispatchRetirementLockedCParity(t *testing.T) {
	entry, ok := parityEntriesByName["wolfram"]
	if !ok {
		t.Fatal("missing Wolfram grammar entry")
	}
	language := entry.Language()
	cLanguage, err := COracleLanguage("wolfram")
	if err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name   string
		file   string
		sha256 string
	}{
		{
			name:   "evaluation-utilities",
			file:   "large__EvaluationUtilities.wl",
			sha256: "e03c8588214ce3a0a5ba48d1f1335276c1826356052c33df1f3184a6d6303a53",
		},
		{
			name:   "output-handling-utilities",
			file:   "medium__OutputHandlingUtilities.wl",
			sha256: "45a6287c3c8ad5f4f37298d4915d1bfb29e6e91ee0eccde1c842efb7c90e3dec",
		},
		{
			name:   "paclet-info",
			file:   "small__PacletInfo.m",
			sha256: "55be9b6143e5dd68ddb433bb9c95c0388a505b65c452fb6036e064d537e3f602",
		},
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			source, err := os.ReadFile(filepath.Join(
				"..", "testdata", "dispatcher_census_a0", "wolfram", test.file,
			))
			if err != nil {
				t.Fatal(err)
			}
			if got := fmt.Sprintf("%x", sha256.Sum256(source)); got != test.sha256 {
				t.Fatalf("source SHA-256 = %s, want %s", got, test.sha256)
			}

			rawParser := gotreesitter.NewParser(language)
			rawParser.SetAdmissionCandidateRoute(false)
			rawTree, err := rawParser.ParseNoResultCompatibilityBenchmarkOnly(source)
			if err != nil {
				t.Fatalf("raw parse: %v", err)
			}
			t.Cleanup(rawTree.Release)
			rawRuntime := rawTree.ParseRuntime()
			if rawRuntime.NormalizationNodesRewritten != 0 {
				t.Fatalf("raw normalization rewrote %d nodes", rawRuntime.NormalizationNodesRewritten)
			}

			productionParser := gotreesitter.NewParser(language)
			productionParser.SetAdmissionCandidateRoute(false)
			productionTree, err := productionParser.Parse(source)
			if err != nil {
				t.Fatalf("production parse: %v", err)
			}
			t.Cleanup(productionTree.Release)
			productionRuntime := productionTree.ParseRuntime()
			if productionRuntime.NormalizationNodesRewritten != 0 {
				t.Fatalf("production normalization rewrote %d nodes", productionRuntime.NormalizationNodesRewritten)
			}

			cParser := sitter.NewParser()
			t.Cleanup(cParser.Close)
			if err := cParser.SetLanguage(cLanguage); err != nil {
				t.Fatal(err)
			}
			cTree := cParser.Parse(source, nil)
			if cTree == nil || cTree.RootNode() == nil {
				t.Fatal("C oracle returned a nil tree")
			}
			t.Cleanup(cTree.Close)

			cDigest, err := COracleDeepDigest(cTree)
			if err != nil {
				t.Fatalf("inspect C deep tree: %v", err)
			}
			rawDigest := assertWolframLockedCTreeExact(t, "raw", rawTree, language, cTree, cDigest)
			productionDigest := assertWolframLockedCTreeExact(t, "production", productionTree, language, cTree, cDigest)
			t.Logf(
				"witness=%s bytes=%d source_sha256=%s raw_digest=%s production_digest=%s raw_rewrites=%d production_rewrites=%d",
				test.file,
				len(source),
				test.sha256,
				rawDigest,
				productionDigest,
				rawRuntime.NormalizationNodesRewritten,
				productionRuntime.NormalizationNodesRewritten,
			)
		})
	}
}

func assertWolframLockedCTreeExact(
	t *testing.T,
	label string,
	goTree *gotreesitter.Tree,
	goLang *gotreesitter.Language,
	cTree *sitter.Tree,
	wantDigest string,
) string {
	t.Helper()
	goRoot := goTree.RootNode()
	cRoot := cTree.RootNode()
	if diff := FirstDivergenceDumpV1(goRoot, goLang, cRoot); diff != nil {
		t.Fatalf("%s tree diverges from the locked C oracle: %+v", label, diff)
	}
	if diff := firstLockedCTreeFlagDivergence(goRoot, goLang, cRoot, "/"+goRoot.Type(goLang)); diff != nil {
		t.Fatalf("%s tree has a missing or error flag divergence: %v", label, diff)
	}
	inspection, err := benchfixtures.InspectGoTree(goRoot, goLang)
	if err != nil {
		t.Fatalf("inspect %s Go deep tree: %v", label, err)
	}
	if inspection.SHA256 != wantDigest {
		t.Fatalf("%s deep digest Go=%s C=%s", label, inspection.SHA256, wantDigest)
	}
	t.Logf("%s route matches locked C exactly: symbols, fields, spans, points, extras, missing/error flags, deep digest=%s", label, inspection.SHA256)
	return inspection.SHA256
}
