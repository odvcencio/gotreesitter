//go:build gts_parsercorephase0

package gotreesitter

import (
	"bytes"
	core "github.com/odvcencio/gotreesitter/internal/parsercorephase0"
	"os"
	"reflect"
	"testing"
)

func TestSingleVersionDirectOutputGo(t *testing.T) {
	for _, row := range diagnosticParserCoreCanonicalAdmissions {
		t.Run(row.id, func(t *testing.T) {
			fixture := loadDiagnosticParserCoreCanonicalFixture(t, row.id)
			r, err := newParserCoreFreshFullRunner(parserCoreWarmGoScanner, parserCoreFreshFullCanonicalOptions())
			if err != nil {
				t.Fatal(err)
			}
			checkSingleVersionDirectOutput(t, r, fixture.Source, true)
		})
	}
}
func TestSingleVersionDirectOutputYAML(t *testing.T) {
	sample, err := os.ReadFile("internal/benchfixtures/testdata/real/yaml")
	if err != nil {
		t.Fatal(err)
	}
	document := append(append([]byte("---\n"), sample...), '\n')
	for _, owned := range []bool{false, true} {
		for name, source := range map[string][]byte{"r4": sample, "eof8k": bytes.Repeat(document, 4)} {
			t.Run(name+map[bool]string{false: "_ordinary", true: "_owned"}[owned], func(t *testing.T) {
				restore := core.SetExternalLexerOwnershipProbeForTest(owned)
				defer restore()
				r, err := newAdmissionCandidateRunner(NewParser(yamlSelectedProjectionLanguageForTest))
				if err != nil {
					t.Fatal(err)
				}
				checkSingleVersionDirectOutput(t, r, source, false)
			})
		}
	}
}
func checkSingleVersionDirectOutput(t *testing.T, r *parserCoreFreshFullRunner, source []byte, query bool) {
	t.Helper()
	native := nativeSelectedOutputRunner{runner: r}
	store, err := native.parse(source)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Release()
	work := r.compact.Work()
	tree, err := r.parse(source)
	if err != nil {
		t.Fatal(err)
	}
	defer tree.Release()
	requireDiagnosticParserCoreSelectedStoreMatchesTree(t, store, tree.RootNode(), r.lang)
	if r.compact.Work() != work {
		t.Fatal("compact output changed graph work")
	}
	if query {
		text, err := loadDiagnosticParserCoreGoHighlightQueryForTest()
		if err != nil {
			t.Fatal(err)
		}
		q, err := NewQuery(text, r.lang)
		if err != nil {
			t.Fatal(err)
		}
		matches, err := executeSelectedStoreQuery(q, store, r.lang, source, nil)
		if err != nil {
			t.Fatal(err)
		}
		want := normalizePublicQueryMatches(q.Execute(tree), source)
		got := normalizeSelectedQueryMatches(matches, store, source)
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("ordered captures differ: %s", firstSelectedQueryParityDifference(got, want))
		}
	}
	t.Logf("bytes=%d nodes=%d retained=%d work=%+v", len(source), store.NodeCount(), store.RetainedBytes(), work)
}
