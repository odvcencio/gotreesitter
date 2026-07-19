//go:build gts_parsercorephase0

package gotreesitter

import (
	"errors"
	"fmt"
	"reflect"
	"testing"

	core "github.com/odvcencio/gotreesitter/internal/parsercorephase0"
)

var diagnosticParserCoreGoHighlightQueryForTest func() (string, error)

// SetDiagnosticParserCoreGoHighlightQueryForTest installs the real registry
// query from the external test package without introducing a grammars -> root
// import cycle in the tagged internal admission suite.
func SetDiagnosticParserCoreGoHighlightQueryForTest(loader func() (string, error)) {
	diagnosticParserCoreGoHighlightQueryForTest = loader
}

func loadDiagnosticParserCoreGoHighlightQueryForTest() (string, error) {
	if diagnosticParserCoreGoHighlightQueryForTest == nil {
		return "", fmt.Errorf("diagnostic parser-core Go highlight query loader is not installed")
	}
	return diagnosticParserCoreGoHighlightQueryForTest()
}

type selectedQueryParityCapture struct {
	PatternIndex int
	CaptureIndex int
	Name         string
	Symbol       Symbol
	Named        bool
	StartByte    uint32
	EndByte      uint32
	Text         string
	TextOverride string
}

func TestDiagnosticParserCoreSelectedStoreGoHighlightParity(t *testing.T) {
	querySource, err := loadDiagnosticParserCoreGoHighlightQueryForTest()
	if err != nil {
		t.Fatal(err)
	}
	for _, row := range diagnosticParserCoreCanonicalAdmissions {
		row := row
		t.Run(row.id, func(t *testing.T) {
			fixture := loadDiagnosticParserCoreCanonicalFixture(t, row.id)
			runner, err := newParserCoreFreshFullRunner(parserCoreWarmGoScanner, parserCoreFreshFullCanonicalOptions())
			if err != nil {
				t.Fatal(err)
			}
			query, err := NewQuery(querySource, runner.lang)
			if err != nil {
				t.Fatalf("compile Go highlight query: %v", err)
			}
			store, err := runner.parseSelectedStore(fixture.Source)
			if err != nil {
				t.Fatalf("selected parse: %v", err)
			}
			defer store.Release()
			tree, err := runner.parse(fixture.Source)
			if err != nil {
				t.Fatalf("materialized parse: %v", err)
			}
			defer tree.Release()

			var selectedBuffer selectedStoreQueryBuffer
			selectedMatches, err := executeSelectedStoreQuery(query, store, runner.lang, fixture.Source, &selectedBuffer)
			if err != nil {
				t.Fatalf("selected query: %v", err)
			}
			publicMatches := query.Execute(tree)
			want := normalizePublicQueryMatches(publicMatches, fixture.Source)
			got := normalizeSelectedQueryMatches(selectedMatches, store, fixture.Source)
			if !reflect.DeepEqual(got, want) {
				t.Fatalf("ordered query output differs: selected=%d public=%d\nfirst difference: %s", len(got), len(want), firstSelectedQueryParityDifference(got, want))
			}

			selectedRanges, err := highlightSelectedStore(query, store, runner.lang, fixture.Source, &selectedBuffer)
			if err != nil {
				t.Fatalf("selected highlight: %v", err)
			}
			publicRanges := highlightRangesFromPublicMatches(publicMatches)
			if !reflect.DeepEqual(selectedRanges, publicRanges) {
				t.Fatalf("resolved highlights differ: selected=%d public=%d", len(selectedRanges), len(publicRanges))
			}
		})
	}
}

func TestDiagnosticParserCoreSelectedStoreQueryDirectiveParity(t *testing.T) {
	fixture := loadDiagnosticParserCoreCanonicalFixture(t, "rewrite")
	runner, err := newParserCoreFreshFullRunner(parserCoreWarmGoScanner, parserCoreFreshFullCanonicalOptions())
	if err != nil {
		t.Fatal(err)
	}
	query, err := NewQuery(`
((identifier) @name
 (#strip! @name "^."))
((selector_expression
   operand: (_) @operand
   "." @dot
   field: (field_identifier) @field)
 (#select-adjacent! @operand @dot))
`, runner.lang)
	if err != nil {
		t.Fatalf("compile directive query: %v", err)
	}
	store, err := runner.parseSelectedStore(fixture.Source)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Release()
	tree, err := runner.parse(fixture.Source)
	if err != nil {
		t.Fatal(err)
	}
	defer tree.Release()

	selectedMatches, err := executeSelectedStoreQuery(query, store, runner.lang, fixture.Source, nil)
	if err != nil {
		t.Fatal(err)
	}
	want := normalizePublicQueryMatches(query.Execute(tree), fixture.Source)
	got := normalizeSelectedQueryMatches(selectedMatches, store, fixture.Source)
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("directive output differs: selected=%d public=%d\nfirst difference: %s", len(got), len(want), firstSelectedQueryParityDifference(got, want))
	}
	stripped := 0
	adjacent := 0
	for _, capture := range got {
		if capture.Name == "name" && capture.TextOverride != "" {
			stripped++
		}
		if capture.Name == "operand" {
			adjacent++
		}
	}
	if stripped == 0 || adjacent == 0 {
		t.Fatalf("directive proof was not exercised: stripped=%d adjacent=%d", stripped, adjacent)
	}
}

func TestDiagnosticParserCoreSelectedStoreQueryMissingDeclines(t *testing.T) {
	runner, err := newParserCoreFreshFullRunner(parserCoreWarmGoScanner, parserCoreFreshFullCanonicalOptions())
	if err != nil {
		t.Fatal(err)
	}
	query, err := NewQuery(`(MISSING) @missing`, runner.lang)
	if err != nil {
		t.Fatal(err)
	}
	fixture := loadDiagnosticParserCoreCanonicalFixture(t, "rewrite")
	store, err := runner.parseSelectedStore(fixture.Source)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Release()
	if _, err := executeSelectedStoreQuery(query, store, runner.lang, fixture.Source, nil); !errors.Is(err, errSelectedStoreQueryMissingUnsupported) {
		t.Fatalf("missing query error=%v want=%v", err, errSelectedStoreQueryMissingUnsupported)
	}
}

func TestDiagnosticParserCoreSelectedStoreQueryTraversalAllocations(t *testing.T) {
	runner, err := newParserCoreFreshFullRunner(parserCoreWarmGoScanner, parserCoreFreshFullCanonicalOptions())
	if err != nil {
		t.Fatal(err)
	}
	query, err := NewQuery(`(identifier) @name`, runner.lang)
	if err != nil {
		t.Fatal(err)
	}
	query.DisablePattern(0)
	fixture := loadDiagnosticParserCoreCanonicalFixture(t, "query_compile")
	store, err := runner.parseSelectedStore(fixture.Source)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Release()
	var buffer selectedStoreQueryBuffer
	if _, err := executeSelectedStoreQuery(query, store, runner.lang, fixture.Source, &buffer); err != nil {
		t.Fatal(err)
	}
	allocations := testing.AllocsPerRun(20, func() {
		matches, err := executeSelectedStoreQuery(query, store, runner.lang, fixture.Source, &buffer)
		if err != nil || len(matches) != 0 {
			panic("selected traversal allocation probe changed result")
		}
	})
	if allocations != 0 {
		t.Fatalf("allocation-free selected traversal: got %.2f allocs/run", allocations)
	}
}

func BenchmarkDiagnosticParserCoreSelectedStoreGoHighlight(b *testing.B) {
	querySource, err := loadDiagnosticParserCoreGoHighlightQueryForTest()
	if err != nil {
		b.Fatal(err)
	}
	for _, row := range diagnosticParserCoreCanonicalAdmissions {
		row := row
		b.Run(row.id, func(b *testing.B) {
			fixture := loadDiagnosticParserCoreCanonicalFixture(b, row.id)
			runner, err := newParserCoreFreshFullRunner(parserCoreWarmGoScanner, parserCoreFreshFullCanonicalOptions())
			if err != nil {
				b.Fatal(err)
			}
			query, err := NewQuery(querySource, runner.lang)
			if err != nil {
				b.Fatal(err)
			}
			store, err := runner.parseSelectedStore(fixture.Source)
			if err != nil {
				b.Fatal(err)
			}
			tree, err := runner.parse(fixture.Source)
			if err != nil {
				store.Release()
				b.Fatal(err)
			}
			var selectedBuffer selectedStoreQueryBuffer
			selectedRanges, err := highlightSelectedStore(query, store, runner.lang, fixture.Source, &selectedBuffer)
			if err != nil || len(selectedRanges) == 0 {
				tree.Release()
				store.Release()
				b.Fatalf("selected preflight: ranges=%d err=%v", len(selectedRanges), err)
			}
			publicMatches := query.Execute(tree)
			if len(publicMatches) == 0 {
				tree.Release()
				store.Release()
				b.Fatal("public query preflight returned no matches")
			}

			b.Run("selected_query", func(b *testing.B) {
				b.ReportAllocs()
				b.SetBytes(int64(len(fixture.Source)))
				for index := 0; index < b.N; index++ {
					matches, err := executeSelectedStoreQuery(query, store, runner.lang, fixture.Source, &selectedBuffer)
					if err != nil || len(matches) == 0 {
						b.Fatalf("matches=%d err=%v", len(matches), err)
					}
				}
			})
			b.Run("legacy_query", func(b *testing.B) {
				b.ReportAllocs()
				b.SetBytes(int64(len(fixture.Source)))
				for index := 0; index < b.N; index++ {
					if matches := query.Execute(tree); len(matches) == 0 {
						b.Fatal("public query returned no matches")
					}
				}
			})
			b.Run("selected_parse_highlight", func(b *testing.B) {
				b.ReportAllocs()
				b.SetBytes(int64(len(fixture.Source)))
				for index := 0; index < b.N; index++ {
					candidate, err := runner.parseSelectedStore(fixture.Source)
					if err != nil {
						b.Fatal(err)
					}
					ranges, err := highlightSelectedStore(query, candidate, runner.lang, fixture.Source, &selectedBuffer)
					candidate.Release()
					if err != nil || len(ranges) == 0 {
						b.Fatalf("ranges=%d err=%v", len(ranges), err)
					}
				}
			})
			b.Run("legacy_parse_highlight", func(b *testing.B) {
				b.ReportAllocs()
				b.SetBytes(int64(len(fixture.Source)))
				for index := 0; index < b.N; index++ {
					candidate, err := runner.parse(fixture.Source)
					if err != nil {
						b.Fatal(err)
					}
					ranges := highlightRangesFromPublicMatches(query.Execute(candidate))
					candidate.Release()
					if len(ranges) == 0 {
						b.Fatal("public parse+highlight returned no ranges")
					}
				}
			})
			tree.Release()
			store.Release()
		})
	}
}

func normalizePublicQueryMatches(matches []QueryMatch, source []byte) []selectedQueryParityCapture {
	var normalized []selectedQueryParityCapture
	for _, match := range matches {
		for captureIndex, capture := range match.Captures {
			if capture.Node == nil {
				continue
			}
			normalized = append(normalized, selectedQueryParityCapture{
				PatternIndex: match.PatternIndex,
				CaptureIndex: captureIndex,
				Name:         capture.Name,
				Symbol:       capture.Node.Symbol(),
				Named:        capture.Node.IsNamed(),
				StartByte:    capture.Node.StartByte(),
				EndByte:      capture.Node.EndByte(),
				Text:         capture.Node.Text(source),
				TextOverride: capture.TextOverride,
			})
		}
	}
	return normalized
}

func normalizeSelectedQueryMatches(matches []queryReaderMatch[queryValueCapture[core.SelectedNodeID]], store *core.SelectedStore, source []byte) []selectedQueryParityCapture {
	reader := selectedStoreQueryReader{store: store}
	var normalized []selectedQueryParityCapture
	for _, match := range matches {
		for captureIndex, capture := range match.Captures {
			normalized = append(normalized, selectedQueryParityCapture{
				PatternIndex: match.PatternIndex,
				CaptureIndex: captureIndex,
				Name:         capture.Name,
				Symbol:       reader.Symbol(capture.Node),
				Named:        reader.IsNamed(capture.Node),
				StartByte:    reader.StartByte(capture.Node),
				EndByte:      reader.EndByte(capture.Node),
				Text:         reader.Text(capture.Node, source),
				TextOverride: capture.TextOverride,
			})
		}
	}
	return normalized
}

func firstSelectedQueryParityDifference(got, want []selectedQueryParityCapture) string {
	limit := len(got)
	if len(want) < limit {
		limit = len(want)
	}
	for index := 0; index < limit; index++ {
		if got[index] != want[index] {
			return fmt.Sprintf("index=%d selected=%+v public=%+v", index, got[index], want[index])
		}
	}
	return fmt.Sprintf("common prefix=%d selected=%d public=%d", limit, len(got), len(want))
}

func highlightRangesFromPublicMatches(matches []QueryMatch) []HighlightRange {
	var ranges []HighlightRange
	for _, match := range matches {
		for _, capture := range match.Captures {
			if capture.Node == nil || capture.Node.StartByte() == capture.Node.EndByte() {
				continue
			}
			ranges = append(ranges, HighlightRange{
				StartByte:    capture.Node.StartByte(),
				EndByte:      capture.Node.EndByte(),
				Capture:      capture.Name,
				PatternIndex: match.PatternIndex,
			})
		}
	}
	return resolveOverlaps(ranges)
}
