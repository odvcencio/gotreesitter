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
		name           string
		source         []byte
		path           string
		sha256         string
		goDigest       string
		cDigest        string
		wantDivergence *normalizationKnownDivergence
	}{
		{
			name:   "parser-produced-pe-reference-trigger",
			source: []byte("<!ELEMENT colspec %ho; EMPTY >"),
			sha256: "f6903445e1a330ae0fd42b19c43538ea30b0da6261c1fe5ae452fc713597f0c7",
			// Commit 8e9a083f5 (fix(parser): Fix PHP issue #454 recovery
			// leaf error clearing) generalized ordinary-leaf error
			// clearing during C-oracle recovery. As a side effect it
			// closed this witness's Name-subtree divergence: raw and
			// production Go trees now match the C oracle exactly, so the
			// known-divergence ratchet is retired per its own escape
			// hatch ("now matches locked C; remove the ratchet").
		},
		{
			name:   "historical-medium-calstblx",
			path:   filepath.Join("..", "testdata", "dispatcher_census_a0", "dtd", "medium__calstblx.dtd"),
			sha256: "54c96c2aa55e2a95b4d0f9ac30df90cfdd717fa1c52f6d3547f1cbd3c8ad4b85",
			// See the parser-produced-pe-reference-trigger comment above:
			// commit 8e9a083f5 also closed this witness's recovered
			// closing-token divergence. Go now matches the C oracle
			// exactly on both routes.
		},
		{
			name:   "historical-large-dbits",
			path:   filepath.Join("..", "testdata", "dispatcher_census_a0", "dtd", "large__dbits.dtd"),
			sha256: "923e8f6ea911bd940ea95d957028fa3155a11b54a756bbe291d3710e110172d9",
		},
		{
			name:   "historical-large-docbook",
			path:   filepath.Join("..", "testdata", "dispatcher_census_a0", "dtd", "large__docbook.dtd"),
			sha256: "4f54c108abea1e4ae8e13e98d79bc0534d442012ed7ab40fcb4052dc843f65dd",
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
			if got := fmt.Sprintf("%x", sha256.Sum256(source)); got != witness.sha256 {
				t.Fatalf("source SHA-256 = %s, want %s", got, witness.sha256)
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

			if witness.wantDivergence != nil {
				assertDTDKnownDeepDigests(t, witness.name, rawTree, productionTree, language, cTree, witness.goDigest, witness.cDigest)
			}

			rawDiff := FirstDivergenceDumpV1(rawTree.RootNode(), language, cTree.RootNode())
			if !normalizationAssertKnownDivergence(t, witness.name, rawDiff, witness.wantDivergence) {
				return
			}
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

func assertDTDKnownDeepDigests(
	t *testing.T,
	witness string,
	rawTree *gotreesitter.Tree,
	productionTree *gotreesitter.Tree,
	language *gotreesitter.Language,
	cTree *sitter.Tree,
	wantGoDigest string,
	wantCDigest string,
) {
	t.Helper()
	rawInspection, err := benchfixtures.InspectGoTree(rawTree.RootNode(), language)
	if err != nil {
		t.Fatalf("inspect raw %s tree: %v", witness, err)
	}
	productionInspection, err := benchfixtures.InspectGoTree(productionTree.RootNode(), language)
	if err != nil {
		t.Fatalf("inspect production %s tree: %v", witness, err)
	}
	cDigest, err := COracleDeepDigest(cTree)
	if err != nil {
		t.Fatalf("inspect C %s tree: %v", witness, err)
	}
	if rawInspection.SHA256 != wantGoDigest || productionInspection.SHA256 != wantGoDigest || cDigest != wantCDigest {
		t.Fatalf(
			"pinned %s tree digest changed: raw Go=%s production Go=%s C=%s; want Go=%s C=%s",
			witness,
			rawInspection.SHA256,
			productionInspection.SHA256,
			cDigest,
			wantGoDigest,
			wantCDigest,
		)
	}
	t.Logf("pinned %s tree digests: raw Go=%s production Go=%s C=%s", witness, rawInspection.SHA256, productionInspection.SHA256, cDigest)
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
