//go:build cgo && treesitter_c_parity

package cgoharness

import (
	"crypto/sha256"
	"fmt"
	"testing"

	gotreesitter "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/internal/benchfixtures"
	sitter "github.com/tree-sitter/go-tree-sitter"
)

const (
	jsdocRetirementTrigger       = "/**\n * @param {string} name\n * @returns {number}\n */\n"
	jsdocRetirementControl       = "/**\n * @param {string} name\n */\n"
	jsdocRetirementTriggerSHA256 = "8a1683a43035994f3abf03f2f9556b96514a745018c5373ff77d3127fb27d201"
	jsdocRetirementControlSHA256 = "0f4dbe6ca5d62b8c033c09ac26689c787a66298540c46b3af7a9760a7240b5ce"
)

func TestJsdocDispatchRetirementLockedCParity(t *testing.T) {
	t.Setenv("GTS_DISPATCHER_CENSUS", "1")
	entry, ok := parityEntriesByName["jsdoc"]
	if !ok {
		t.Fatal("missing JSDoc grammar entry")
	}
	language := entry.Language()
	cLanguage, err := COracleLanguage("jsdoc")
	if err != nil {
		t.Fatal(err)
	}

	for _, test := range []struct {
		name   string
		source []byte
		sha256 string
	}{
		{name: "multi_tag_trigger", source: []byte(jsdocRetirementTrigger), sha256: jsdocRetirementTriggerSHA256},
		{name: "single_tag_control", source: []byte(jsdocRetirementControl), sha256: jsdocRetirementControlSHA256},
	} {
		t.Run(test.name, func(t *testing.T) {
			if got := fmt.Sprintf("%x", sha256.Sum256(test.source)); got != test.sha256 {
				t.Fatalf("source SHA-256 = %s, want %s", got, test.sha256)
			}

			rawParser := gotreesitter.NewParser(language)
			rawParser.SetAdmissionCandidateRoute(false)
			raw, err := rawParser.ParseNoResultCompatibilityBenchmarkOnly(test.source)
			if err != nil {
				t.Fatalf("raw parse: %v", err)
			}
			t.Cleanup(raw.Release)

			productionParser := gotreesitter.NewParser(language)
			productionParser.SetAdmissionCandidateRoute(false)
			production, err := productionParser.Parse(test.source)
			if err != nil {
				t.Fatalf("production parse: %v", err)
			}
			t.Cleanup(production.Release)

			cParser := sitter.NewParser()
			t.Cleanup(cParser.Close)
			if err := cParser.SetLanguage(cLanguage); err != nil {
				t.Fatal(err)
			}
			cTree := cParser.Parse(test.source, nil)
			if cTree == nil || cTree.RootNode() == nil {
				t.Fatal("C oracle returned a nil tree")
			}
			t.Cleanup(cTree.Close)

			cDigest, err := COracleDeepDigest(cTree)
			if err != nil {
				t.Fatalf("inspect C deep tree: %v", err)
			}
			rawRuntime := raw.ParseRuntime()
			productionRuntime := production.ParseRuntime()
			t.Logf("witness=%s bytes=%d source_sha256=%s raw_rewrites=%d production_rewrites=%d c_digest=%s", test.name, len(test.source), test.sha256, rawRuntime.NormalizationNodesRewritten, productionRuntime.NormalizationNodesRewritten, cDigest)

			assertJsdocLockedCTreeExact(t, "raw", raw, language, cTree, cDigest)
			assertJsdocLockedCTreeExact(t, "production", production, language, cTree, cDigest)
		})
	}
}

func assertJsdocLockedCTreeExact(t *testing.T, label string, goTree *gotreesitter.Tree, goLang *gotreesitter.Language, cTree *sitter.Tree, wantDigest string) {
	t.Helper()
	goRoot := goTree.RootNode()
	cRoot := cTree.RootNode()
	if diff := FirstDivergenceDumpV1(goRoot, goLang, cRoot); diff != nil {
		t.Errorf("%s tree diverges from the locked C oracle: %+v", label, diff)
		return
	}
	if diff := firstLockedCTreeFlagDivergence(goRoot, goLang, cRoot, "/"+goRoot.Type(goLang)); diff != nil {
		t.Errorf("%s tree has a missing or error flag divergence: %v", label, diff)
		return
	}
	inspection, err := benchfixtures.InspectGoTree(goRoot, goLang)
	if err != nil {
		t.Errorf("inspect %s Go deep tree: %v", label, err)
		return
	}
	if inspection.SHA256 != wantDigest {
		t.Errorf("%s deep digest Go=%s C=%s", label, inspection.SHA256, wantDigest)
		return
	}
	t.Logf("%s route matches locked C exactly: symbols, fields, spans, points, extras, missing/error flags, deep digest=%s", label, inspection.SHA256)
}
