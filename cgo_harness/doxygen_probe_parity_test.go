//go:build cgo && treesitter_c_parity

package cgoharness

import (
	"crypto/sha256"
	"os"
	"path/filepath"
	"testing"

	gotreesitter "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
	"github.com/odvcencio/gotreesitter/internal/benchfixtures"
	sitter "github.com/tree-sitter/go-tree-sitter"
)

func TestDoxygenRegisteredWitnessesLockedCDeepParity(t *testing.T) {
	t.Setenv("GTS_DISPATCHER_CENSUS", "1")
	entry, ok := parityEntriesByName["doxygen"]
	if !ok {
		t.Fatal("missing Doxygen grammar entry")
	}
	goLang := entry.Language()
	cLang, err := COracleLanguage("doxygen")
	if err != nil {
		t.Fatal(err)
	}
	witnesses := []struct {
		name, path, source string
	}{
		{name: "a0_CMakeLists", path: "../testdata/dispatcher_census_a0/doxygen/medium__CMakeLists.txt"},
		{name: "a0_metrics", path: "../testdata/dispatcher_census_a0/doxygen/medium__metrics.py"},
		{name: "a0_example_cfg", path: "../testdata/dispatcher_census_a0/doxygen/small__example.cfg"},
		{name: "registered_smoke", source: grammars.ParseSmokeSample("doxygen")},
		{name: "historical_childless_error", source: "/** Adds all words in \\a s to document \\a doc with weight \\a wfd */"},
		{name: "historical_recovered_document", source: "/**\n * @param {int} value\n * @brief Example\n */"},
	}
	for _, witness := range witnesses {
		witness := witness
		t.Run(witness.name, func(t *testing.T) {
			source := []byte(witness.source)
			if witness.path != "" {
				var err error
				source, err = os.ReadFile(filepath.Join("..", "testdata", "dispatcher_census_a0", "doxygen", filepath.Base(witness.path)))
				if err != nil {
					t.Fatal(err)
				}
			}
			t.Logf("source_sha256=%x bytes=%d", sha256.Sum256(source), len(source))

			rawParser := gotreesitter.NewParser(goLang)
			rawParser.SetAdmissionCandidateRoute(false)
			raw, err := rawParser.ParseNoResultCompatibilityBenchmarkOnly(source)
			if err != nil {
				t.Fatalf("raw parse: %v", err)
			}
			t.Cleanup(raw.Release)
			productionParser := gotreesitter.NewParser(goLang)
			productionParser.SetAdmissionCandidateRoute(false)
			production, err := productionParser.Parse(source)
			if err != nil {
				t.Fatalf("production parse: %v", err)
			}
			t.Cleanup(production.Release)

			cParser := sitter.NewParser()
			t.Cleanup(cParser.Close)
			if err := cParser.SetLanguage(cLang); err != nil {
				t.Fatal(err)
			}
			cTree := cParser.Parse(source, nil)
			if cTree == nil || cTree.RootNode() == nil {
				t.Fatal("C oracle returned a nil tree")
			}
			t.Cleanup(cTree.Close)
			cDigest, err := COracleDeepDigest(cTree)
			if err != nil {
				t.Fatalf("C digest: %v", err)
			}
			rawInspection, err := benchfixtures.InspectGoTree(raw.RootNode(), goLang)
			if err != nil {
				t.Fatalf("raw digest: %v", err)
			}
			productionInspection, err := benchfixtures.InspectGoTree(production.RootNode(), goLang)
			if err != nil {
				t.Fatalf("production digest: %v", err)
			}
			t.Logf("raw_digest=%s production_digest=%s c_digest=%s raw_rewrites=%d production_rewrites=%d raw_root=%s raw_error=%t production_root=%s production_error=%t", rawInspection.SHA256, productionInspection.SHA256, cDigest, raw.ParseRuntime().NormalizationNodesRewritten, production.ParseRuntime().NormalizationNodesRewritten, raw.RootNode().Type(goLang), raw.RootNode().HasError(), production.RootNode().Type(goLang), production.RootNode().HasError())
			if diff := FirstDivergenceDumpV1(production.RootNode(), goLang, cTree.RootNode()); diff != nil {
				t.Fatalf("production diverges from locked C: %+v", diff)
			}
			if diff := firstLockedCTreeFlagDivergence(production.RootNode(), goLang, cTree.RootNode(), "/"+production.RootNode().Type(goLang)); diff != nil {
				t.Fatalf("production flags diverge from locked C: %v", diff)
			}
			if productionInspection.SHA256 != cDigest {
				t.Fatalf("production deep digest=%s C=%s", productionInspection.SHA256, cDigest)
			}
			if rawInspection.SHA256 != productionInspection.SHA256 {
				t.Errorf("raw and production deep digests differ: raw=%s production=%s", rawInspection.SHA256, productionInspection.SHA256)
			}
		})
	}
}
