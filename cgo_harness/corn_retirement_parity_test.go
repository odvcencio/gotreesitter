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

const cornQuotedKeysSource = "{\n    'foo.bar' = 42\n    'green.eggs'.and.ham = \"hello world\"\n    'with spaces' = true\n    'escaped\\'quote' = false\n    'escaped=equals' = -3\n}"

// TestCornNormalizationCensusLockedCExact compares all A0 Corn witnesses and
// the existing quoted-keys trigger against the pinned C grammar.
func TestCornNormalizationCensusLockedCExact(t *testing.T) {
	entry, ok := parityEntriesByName["corn"]
	if !ok {
		t.Fatal("missing Corn grammar entry")
	}
	language := entry.Language()
	cLanguage, err := COracleLanguage("corn")
	if err != nil {
		t.Fatal(err)
	}
	identity, err := COracleIdentity("corn")
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("C oracle contract=%s binding=%s runtime=%s grammar_commit=%s artifact_sha256=%s", identity.Contract, identity.BindingVersion, identity.RuntimeVersion, identity.GrammarCommit, identity.GrammarArtifactSHA256)

	tests := []struct {
		name   string
		file   string
		sha256 string
		source []byte
	}{
		{
			name:   "a0-compact",
			file:   "small__compact.corn",
			sha256: "e9793277f21b19593024cf7f670934333deefb54eae122caaa1f66cf41c7606a",
		},
		{
			name:   "a0-complex",
			file:   "small__complex.corn",
			sha256: "98aaba0d478418a7855fa538cb9cd52ab81b2d2e581402d212bc3951a6a5db02",
		},
		{
			name:   "a0-readme-example",
			file:   "small__readme_example.corn",
			sha256: "7d412d6e3c5e396818885601df14ad092b8cbe4aac690a45d7a4c29e0410da94",
		},
		{
			name:   "quoted-keys-trigger",
			sha256: "1e541539a857d075420cd73d64381b4f452d98a44884356a09b3e3ca7faf0b68",
			source: []byte(cornQuotedKeysSource),
		},
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			source := test.source
			if source == nil {
				var err error
				source, err = os.ReadFile(filepath.Join(
					"..", "testdata", "dispatcher_census_a0", "corn", test.file,
				))
				if err != nil {
					t.Fatal(err)
				}
			}
			if got := fmt.Sprintf("%x", sha256.Sum256(source)); got != test.sha256 {
				t.Fatalf("source SHA-256 = %s, want %s", got, test.sha256)
			}

			cParser := sitter.NewParser()
			t.Cleanup(cParser.Close)
			if err := cParser.SetLanguage(cLanguage); err != nil {
				t.Fatal(err)
			}
			cTree := cParser.Parse(source, nil)
			if cTree == nil || cTree.RootNode() == nil {
				t.Fatal("locked C parser returned a nil tree")
			}
			t.Cleanup(cTree.Close)
			cDigest, err := COracleDeepDigest(cTree)
			if err != nil {
				t.Fatalf("inspect locked C deep tree: %v", err)
			}

			rawParser := gotreesitter.NewParser(language)
			rawParser.SetAdmissionCandidateRoute(false)
			rawTree, err := rawParser.ParseNoResultCompatibilityBenchmarkOnly(source)
			if err != nil {
				t.Fatalf("raw parse: %v", err)
			}
			t.Cleanup(rawTree.Release)
			rawRuntime := rawTree.ParseRuntime()
			rawDigest := assertCornLockedCTreeExact(t, "raw", rawTree, language, cTree, cDigest)

			productionParser := gotreesitter.NewParser(language)
			productionParser.SetAdmissionCandidateRoute(false)
			productionTree, err := productionParser.Parse(source)
			if err != nil {
				t.Fatalf("production parse: %v", err)
			}
			t.Cleanup(productionTree.Release)
			productionRuntime := productionTree.ParseRuntime()
			productionDigest := assertCornLockedCTreeExact(t, "production", productionTree, language, cTree, cDigest)

			t.Logf("witness=%s bytes=%d source_sha256=%s c_digest=%s raw_digest=%s production_digest=%s raw_rewrites=%d production_rewrites=%d", test.name, len(source), test.sha256, cDigest, rawDigest, productionDigest, rawRuntime.NormalizationNodesRewritten, productionRuntime.NormalizationNodesRewritten)
		})
	}
}

func assertCornLockedCTreeExact(
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
