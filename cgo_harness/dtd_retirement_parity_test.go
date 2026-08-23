//go:build cgo && treesitter_c_parity

package cgoharness

import (
	"crypto/sha256"
	"os"
	"path/filepath"
	"testing"

	gotreesitter "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/internal/benchfixtures"
	sitter "github.com/tree-sitter/go-tree-sitter"
)

func TestDTDDispatchRetirementLockedCParity(t *testing.T) {
	t.Setenv("GTS_DISPATCHER_CENSUS", "1")

	entry, ok := parityEntriesByName["dtd"]
	if !ok {
		t.Fatal("missing DTD grammar entry")
	}
	language := entry.Language()
	cLanguage, err := COracleLanguage("dtd")
	if err != nil {
		t.Fatal(err)
	}

	witnesses := []struct {
		name   string
		source []byte
		path   string
	}{
		{
			name:   "parser-produced-pe-reference-trigger",
			source: []byte("<!ELEMENT colspec %ho; EMPTY >"),
		},
		{
			name: "historical-medium-calstblx",
			path: filepath.Join("..", "testdata", "dispatcher_census_a0", "dtd", "medium__calstblx.dtd"),
		},
		{
			name: "historical-large-dbits",
			path: filepath.Join("..", "testdata", "dispatcher_census_a0", "dtd", "large__dbits.dtd"),
		},
		{
			name: "historical-large-docbook",
			path: filepath.Join("..", "testdata", "dispatcher_census_a0", "dtd", "large__docbook.dtd"),
		},
	}

	for _, witness := range witnesses {
		witness := witness
		t.Run(witness.name, func(t *testing.T) {
			source := witness.source
			if witness.path != "" {
				var err error
				source, err = os.ReadFile(witness.path)
				if err != nil {
					t.Fatal(err)
				}
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

			rawParser := gotreesitter.NewParser(language)
			rawParser.SetAdmissionCandidateRoute(false)
			rawTree, err := rawParser.ParseNoResultCompatibilityBenchmarkOnly(source)
			if err != nil {
				t.Fatalf("raw parse: %v", err)
			}
			t.Cleanup(rawTree.Release)

			productionParser := gotreesitter.NewParser(language)
			productionParser.SetAdmissionCandidateRoute(false)
			productionTree, err := productionParser.Parse(source)
			if err != nil {
				t.Fatalf("production parse: %v", err)
			}
			t.Cleanup(productionTree.Release)

			rawDigest := assertDTDLockedCTreeExact(t, "raw", rawTree, language, cTree)
			productionDigest := assertDTDLockedCTreeExact(t, "production", productionTree, language, cTree)
			if rawDigest != productionDigest {
				t.Fatalf("raw and production deep digests differ: raw=%s production=%s", rawDigest, productionDigest)
			}
			if rawTree.ParseRuntime().NormalizationNodesRewritten != 0 {
				t.Fatalf("raw route rewrote %d nodes", rawTree.ParseRuntime().NormalizationNodesRewritten)
			}
			if productionTree.ParseRuntime().NormalizationNodesRewritten != 0 {
				t.Fatalf("production route rewrote %d nodes", productionTree.ParseRuntime().NormalizationNodesRewritten)
			}
			t.Logf(
				"source_sha256=%x bytes=%d raw_digest=%s production_digest=%s rewrites=0",
				sha256.Sum256(source),
				len(source),
				rawDigest,
				productionDigest,
			)
		})
	}
}

func assertDTDLockedCTreeExact(
	t *testing.T,
	label string,
	goTree *gotreesitter.Tree,
	goLang *gotreesitter.Language,
	cTree *sitter.Tree,
) string {
	t.Helper()
	goRoot := goTree.RootNode()
	cRoot := cTree.RootNode()
	if diff := FirstDivergenceDumpV1(goRoot, goLang, cRoot); diff != nil {
		t.Fatalf("%s node or field divergence: %+v", label, diff)
	}
	if diff := firstLockedCTreeFlagDivergence(goRoot, goLang, cRoot, "/"+goRoot.Type(goLang)); diff != nil {
		t.Fatal(diff)
	}
	goInspection, err := benchfixtures.InspectGoTree(goRoot, goLang)
	if err != nil {
		t.Fatalf("inspect Go deep tree: %v", err)
	}
	cDigest, err := COracleDeepDigest(cTree)
	if err != nil {
		t.Fatalf("inspect C deep tree: %v", err)
	}
	if goInspection.SHA256 != cDigest {
		t.Fatalf("%s deep digest Go=%s C=%s", label, goInspection.SHA256, cDigest)
	}
	t.Logf("%s exact symbols, fields, spans, points, extras, missing/error flags, and deep digest=%s", label, cDigest)
	return goInspection.SHA256
}
