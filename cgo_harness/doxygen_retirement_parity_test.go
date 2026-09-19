//go:build cgo && treesitter_c_parity

package cgoharness

import (
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	gotreesitter "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
	"github.com/odvcencio/gotreesitter/internal/benchfixtures"
	sitter "github.com/tree-sitter/go-tree-sitter"
)

// TestDoxygenDispatchRetirementLockedCParity keeps the three A0 divergence
// classes exact. The registered smoke witness remains an exact C control.
func TestDoxygenDispatchRetirementLockedCParity(t *testing.T) {
	t.Setenv("GTS_DISPATCHER_CENSUS", "1")

	entry, ok := parityEntriesByName["doxygen"]
	if !ok {
		t.Fatal("missing Doxygen grammar entry")
	}
	goLanguage := entry.Language()
	cLanguage, err := COracleLanguage("doxygen")
	if err != nil {
		t.Fatal(err)
	}

	witnesses := []struct {
		name           string
		path           string
		source         string
		sourceSHA256   string
		goDigest       string
		cDigest        string
		wantDivergence *normalizationKnownDivergence
	}{
		{
			name:         "a0_CMakeLists",
			path:         filepath.Join("..", "testdata", "dispatcher_census_a0", "doxygen", "medium__CMakeLists.txt"),
			sourceSHA256: "66408d6539b27d7c49b1e51777605c38c91b6d924267db5109ee00e2a1cfcf41",
			goDigest:     "a206903ee351591886014cb963d527769fc710d513af7b84c6dba9d9cc77cd2b",
			cDigest:      "d6f623d2b87344001e98de5528b44e38b102e564491871a9ffb64c1b73d193c5",
			wantDivergence: &normalizationKnownDivergence{
				Path:     "/document",
				Category: "type",
				GoValue:  "document",
				CValue:   "ERROR",
				Reason:   "the C oracle keeps the whole A0 source under an ERROR root",
			},
		},
		{
			name:         "a0_metrics",
			path:         filepath.Join("..", "testdata", "dispatcher_census_a0", "doxygen", "medium__metrics.py"),
			sourceSHA256: "31622a6c075ffa6f78a16af6e379f517213d42ff67729bbd0d10551c5fca9702",
			goDigest:     "5adbacb1ec949237a802a56a5c95c3c7a1ce17fe9c8db5423b63f083da62d5d1",
			cDigest:      "6660931c2bf1bf1e0f909a1cac1e4cd8446853ae4466781c943e28fbcc61e860",
			wantDivergence: &normalizationKnownDivergence{
				Path:     "/ERROR",
				Category: "shape",
				GoValue:  "children=0",
				CValue:   "children=279",
				Reason:   "the C oracle retains the recovered ERROR children that Go currently drops",
			},
		},
		{
			name:         "a0_example_cfg",
			path:         filepath.Join("..", "testdata", "dispatcher_census_a0", "doxygen", "small__example.cfg"),
			sourceSHA256: "86998161914382f8152e4984db091e7bf486799c1091fc6c57db4e704eee4a3b",
			goDigest:     "4e961b6abe28b703bc0e1e2033afc70fbb1d913b522687d4d270fd780b2ed6c4",
			cDigest:      "f1938d5c7bc544856a5df6c204af75af10a5395bd1f89f560c74caef5acf191f",
			wantDivergence: &normalizationKnownDivergence{
				Path:     "/document",
				Category: "type",
				GoValue:  "document",
				CValue:   "ERROR",
				Reason:   "the C oracle keeps the whole A0 source under an ERROR root",
			},
		},
		{
			name:         "registered_smoke",
			source:       grammars.ParseSmokeSample("doxygen"),
			sourceSHA256: "e2d564b999c40b0a53450771ffa82adf7880375449e8628fefd118aae21056d7",
			goDigest:     "1ae089a98760be594f06d0820951e01714097e99621cc2cd4428ce09ba867083",
			cDigest:      "1ae089a98760be594f06d0820951e01714097e99621cc2cd4428ce09ba867083",
		},
	}

	for _, witness := range witnesses {
		witness := witness
		t.Run(witness.name, func(t *testing.T) {
			source := []byte(witness.source)
			if witness.path != "" {
				var err error
				source, err = os.ReadFile(witness.path)
				if err != nil {
					t.Fatal(err)
				}
			}
			if got := fmt.Sprintf("%x", sha256.Sum256(source)); got != witness.sourceSHA256 {
				t.Fatalf("source SHA-256 = %s, want %s", got, witness.sourceSHA256)
			}

			rawParser := gotreesitter.NewParser(goLanguage)
			rawParser.SetAdmissionCandidateRoute(false)
			rawTree, err := rawParser.ParseNoResultCompatibilityBenchmarkOnly(source)
			if err != nil {
				t.Fatalf("raw parse: %v", err)
			}
			t.Cleanup(rawTree.Release)

			productionParser := gotreesitter.NewParser(goLanguage)
			productionParser.SetAdmissionCandidateRoute(false)
			productionTree, err := productionParser.Parse(source)
			if err != nil {
				t.Fatalf("production parse: %v", err)
			}
			t.Cleanup(productionTree.Release)

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

			rawInspection, err := benchfixtures.InspectGoTree(rawTree.RootNode(), goLanguage)
			if err != nil {
				t.Fatalf("inspect raw Go tree: %v", err)
			}
			productionInspection, err := benchfixtures.InspectGoTree(productionTree.RootNode(), goLanguage)
			if err != nil {
				t.Fatalf("inspect production Go tree: %v", err)
			}
			cDigest, err := COracleDeepDigest(cTree)
			if err != nil {
				t.Fatalf("inspect C tree: %v", err)
			}
			if rawInspection.SHA256 != witness.goDigest || productionInspection.SHA256 != witness.goDigest {
				t.Fatalf("Go digest changed: raw=%s production=%s want=%s", rawInspection.SHA256, productionInspection.SHA256, witness.goDigest)
			}
			if cDigest != witness.cDigest {
				t.Fatalf("C digest changed: got=%s want=%s", cDigest, witness.cDigest)
			}
			if rawTree.ParseRuntime().NormalizationNodesRewritten != 0 {
				t.Fatalf("raw route rewrote %d nodes", rawTree.ParseRuntime().NormalizationNodesRewritten)
			}
			if productionTree.ParseRuntime().NormalizationNodesRewritten != 0 {
				t.Fatalf("production route rewrote %d nodes", productionTree.ParseRuntime().NormalizationNodesRewritten)
			}
			t.Logf(
				"source_sha256=%s raw_digest=%s production_digest=%s c_digest=%s raw_rewrites=%d production_rewrites=%d",
				witness.sourceSHA256,
				rawInspection.SHA256,
				productionInspection.SHA256,
				cDigest,
				rawTree.ParseRuntime().NormalizationNodesRewritten,
				productionTree.ParseRuntime().NormalizationNodesRewritten,
			)

			diff := FirstDivergenceDumpV1(productionTree.RootNode(), goLanguage, cTree.RootNode())
			if !normalizationAssertKnownDivergence(t, witness.name, diff, witness.wantDivergence) {
				return
			}
			if diff := firstLockedCTreeFlagDivergence(productionTree.RootNode(), goLanguage, cTree.RootNode(), "/"+productionTree.RootNode().Type(goLanguage)); diff != nil {
				t.Fatalf("production flags diverge from locked C: %v", diff)
			}
			if productionInspection.SHA256 != cDigest {
				t.Fatalf("production deep digest=%s, want C=%s", productionInspection.SHA256, cDigest)
			}
			if rawInspection.SHA256 != productionInspection.SHA256 {
				t.Fatalf("raw and production deep digests differ: raw=%s production=%s", rawInspection.SHA256, productionInspection.SHA256)
			}
		})
	}
}
