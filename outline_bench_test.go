package gotreesitter_test

import (
	"strings"
	"testing"

	gts "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
	"github.com/odvcencio/gotreesitter/internal/benchfixtures"
)

// outlineBenchWorkload holds one already-parsed tree, one compiled outliner,
// and one compiled tagger over the identical language and tags query. Both
// benchmarks below reuse it, so they measure projection only: the parse and
// both query compilations happen once, outside the timed loop.
type outlineBenchWorkload struct {
	id       string
	tree     *gts.Tree
	outliner *gts.Outliner
	tagger   *gts.Tagger
}

func newOutlineBenchWorkloads(tb testing.TB) []outlineBenchWorkload {
	tb.Helper()

	entry := grammars.DetectLanguageByName("go")
	if entry == nil {
		tb.Fatal("the go grammar is not registered")
	}
	lang := entry.Language()
	query := grammars.ResolveTagsQuery(*entry)
	if query == "" {
		tb.Fatal("the go tags query resolved empty")
	}

	fixtures, err := benchfixtures.LoadGoFullParseFixtures()
	if err != nil {
		tb.Fatalf("load go fixtures: %v", err)
	}

	workloads := make([]outlineBenchWorkload, 0, len(fixtures))
	for _, fixture := range fixtures {
		parser := gts.NewParser(lang)
		tree, err := parser.Parse(fixture.Source)
		if err != nil {
			tb.Fatalf("%s: parse: %v", fixture.Fixture.ID, err)
		}
		tb.Cleanup(tree.Release)

		outliner, err := gts.NewOutliner(lang, query)
		if err != nil {
			tb.Fatalf("%s: build outliner: %v", fixture.Fixture.ID, err)
		}
		tagger, err := gts.NewTagger(lang, query)
		if err != nil {
			tb.Fatalf("%s: build tagger: %v", fixture.Fixture.ID, err)
		}
		workloads = append(workloads, outlineBenchWorkload{
			id:       fixture.Fixture.ID,
			tree:     tree,
			outliner: outliner,
			tagger:   tagger,
		})
	}
	return workloads
}

// BenchmarkOutlineTree measures the outline projection over already-parsed Go
// corpus trees. Pair it with BenchmarkOutlineTaggerBaseline, which runs the
// identical query over the identical trees, to read the ratio the
// specification budgets at 1.5 times.
//
//	go test . -run '^$' -bench 'BenchmarkOutline' -benchtime 200x -count 5
func BenchmarkOutlineTree(b *testing.B) {
	workloads := newOutlineBenchWorkloads(b)
	for _, workload := range workloads {
		workload := workload
		b.Run(workload.id, func(b *testing.B) {
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				symbols, report := workload.outliner.OutlineTree(workload.tree)
				if report.Symbols == 0 || len(symbols) == 0 {
					b.Fatalf("%s: outline produced nothing", workload.id)
				}
			}
		})
	}
}

// BenchmarkOutlineTaggerBaseline is the comparison arm: the same tags query,
// the same trees, through Tagger.TagTree.
func BenchmarkOutlineTaggerBaseline(b *testing.B) {
	workloads := newOutlineBenchWorkloads(b)
	for _, workload := range workloads {
		workload := workload
		b.Run(workload.id, func(b *testing.B) {
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				tags := workload.tagger.TagTree(workload.tree)
				if len(tags) == 0 {
					b.Fatalf("%s: tagger produced nothing", workload.id)
				}
			}
		})
	}
}

// TestOutlineTreeCostStaysNearTagger is the continuous-integration-visible
// half of the performance gate. Wall time on a shared runner is too noisy to
// assert, so this test asserts the deterministic cost driver instead: the
// outline and the tagger execute the same query over the same tree, so the
// outline must not visit more matches than the tagger produces tags, and its
// allocation count must stay inside a fixed multiple of the tagger's.
//
// Allocation counts are deterministic for a fixed tree and query, so this
// assertion does not flake on a contended host. The wall-clock ratio is
// measured by the two benchmarks above and reported with the change.
func TestOutlineTreeCostStaysNearTagger(t *testing.T) {
	// The allocation budget. Observed ratios are 1.005 to 1.018, because the
	// outline allocates only the candidate slice, the sorted copy, the
	// accepted, parent, and children slices, and one slice per parent that
	// has children. A budget of 1.5 leaves room for that and still fails on
	// a regression that adds a per-node allocation; a looser budget would
	// let a threefold regression through unnoticed.
	const allocationBudgetMultiple = 1.5

	workloads := newOutlineBenchWorkloads(t)
	if len(workloads) == 0 {
		t.Fatal("no benchmark workloads")
	}

	for _, workload := range workloads {
		workload := workload
		t.Run(workload.id, func(t *testing.T) {
			symbols, report := workload.outliner.OutlineTree(workload.tree)
			tags := workload.tagger.TagTree(workload.tree)
			if len(symbols) == 0 || report.Symbols == 0 {
				t.Fatalf("%s: outline produced nothing", workload.id)
			}
			if len(tags) == 0 {
				t.Fatalf("%s: tagger produced nothing", workload.id)
			}

			// Every outline candidate comes from a DEFINITION tag.
			// Comparing against every tag would be loose by an order of
			// magnitude, because the Go tags query is mostly call
			// references, so count the definition tags exactly.
			definitionTags := 0
			for _, tag := range tags {
				if strings.HasPrefix(tag.Kind, "definition.") {
					definitionTags++
				}
			}
			if definitionTags == 0 {
				t.Fatalf("%s: the tagger produced no definition tag, so the bound is vacuous", workload.id)
			}
			if report.Candidates() > definitionTags {
				t.Errorf("%s: outline saw %d candidates from %d definition tags; it must not do more query work than the tagger",
					workload.id, report.Candidates(), definitionTags)
			}

			outlineAllocs := testing.AllocsPerRun(3, func() {
				workload.outliner.OutlineTree(workload.tree)
			})
			taggerAllocs := testing.AllocsPerRun(3, func() {
				workload.tagger.TagTree(workload.tree)
			})
			if taggerAllocs <= 0 {
				t.Fatalf("%s: tagger baseline allocated nothing, so the budget is meaningless", workload.id)
			}
			budget := taggerAllocs * allocationBudgetMultiple
			if outlineAllocs > budget {
				t.Errorf("%s: outline allocated %.0f times per call, budget %.0f (%.1f times the tagger's %.0f)",
					workload.id, outlineAllocs, budget, allocationBudgetMultiple, taggerAllocs)
			}
			t.Logf("%s: outline %.0f allocs, tagger %.0f allocs, ratio %.2f; outline symbols=%d tags=%d",
				workload.id, outlineAllocs, taggerAllocs, outlineAllocs/taggerAllocs, report.Symbols, len(tags))
		})
	}
}
