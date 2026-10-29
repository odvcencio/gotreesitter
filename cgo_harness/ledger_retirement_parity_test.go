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

// TestLedgerDispatchRetirementLockedCParity compares the two Ledger
// normalization triggers and the A0 corpus witness with the locked C parser.
func TestLedgerDispatchRetirementLockedCParity(t *testing.T) {
	entry, ok := parityEntriesByName["ledger"]
	if !ok {
		t.Fatal("missing Ledger grammar entry")
	}
	language := entry.Language()
	cLanguage, err := COracleLanguage("ledger")
	if err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name   string
		load   func() ([]byte, error)
		sha256 string
	}{
		{
			name: "recovered-date-suffix",
			load: func() ([]byte, error) {
				return []byte("\n15.03.2006 Exxon\n    Expenses:Auto:Gas          10,00 EUR\n    Liabilities:MasterCard    -10,00 EUR\n"), nil
			},
			sha256: "c9363c584f9bdb4cd3aa56a3fa6df98acc3a180c40e82ba3724d00efbc238210",
		},
		{
			name: "year-directive",
			load: func() ([]byte, error) {
				return []byte(`
--input-date-format %d.%m

Y2010
03.01 * Foo
    A                 10.00 EUR
    B

05.02 * Bar
    A                 20.00 EUR
    B

test reg A
10-Jan-03 Foo                   A                         10.00 EUR    10.00 EUR
10-Feb-05 Bar                   A                         20.00 EUR    30.00 EUR
end test

`), nil
			},
			sha256: "531a9ddc9e5fe851095ed38b5dd896dc0de4a984f56ef0269c92254e5e84a02e",
		},
		{
			name: "a0-non-profit-test-data",
			load: func() ([]byte, error) {
				return os.ReadFile(filepath.Join(
					"..", "testdata", "dispatcher_census_a0", "ledger",
					"small__non-profit-test-data.ledger",
				))
			},
			sha256: "346d9eb6257c54d9fe53aabf9d3d32f8b8d5ebfece9f9142ebcfdedfa88cbbd2",
		},
	}

	for _, test := range tests {
		test := test
		if !t.Run(test.name, func(t *testing.T) {
			source, err := test.load()
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

			productionParser := gotreesitter.NewParser(language)
			productionParser.SetAdmissionCandidateRoute(false)
			productionTree, err := productionParser.Parse(source)
			if err != nil {
				t.Fatalf("production parse: %v", err)
			}
			t.Cleanup(productionTree.Release)
			productionRuntime := productionTree.ParseRuntime()

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
			t.Logf(
				"witness=%s bytes=%d source_sha256=%s raw_rewrites=%d production_rewrites=%d c_digest=%s",
				test.name,
				len(source),
				test.sha256,
				rawRuntime.NormalizationNodesRewritten,
				productionRuntime.NormalizationNodesRewritten,
				cDigest,
			)

			rawDigest := assertLockedCTreeDigestExact(t, "raw", rawTree, language, cTree, cDigest)
			productionDigest := assertLockedCTreeDigestExact(t, "production", productionTree, language, cTree, cDigest)
			t.Logf(
				"witness=%s raw_digest=%s production_digest=%s raw_rewrites=%d production_rewrites=%d",
				test.name,
				rawDigest,
				productionDigest,
				rawRuntime.NormalizationNodesRewritten,
				productionRuntime.NormalizationNodesRewritten,
			)
		}) {
			t.FailNow()
		}
	}
}

// assertLockedCTreeDigestExact checks that goTree matches the locked C tree
// exactly (symbols, fields, spans, points, extras, missing/error flags) and that
// its deep digest equals wantDigest. It returns the Go deep digest.
func assertLockedCTreeDigestExact(
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
	t.Logf(
		"%s route matches locked C exactly: symbols, fields, spans, points, extras, missing/error flags, deep digest=%s",
		label,
		inspection.SHA256,
	)
	return inspection.SHA256
}
