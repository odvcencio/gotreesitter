package gotreesitter_test

import (
	"testing"

	gts "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
	"github.com/odvcencio/gotreesitter/internal/benchfixtures"
)

// Check clean roots and route accounting. The shared C family tests check tree semantics.
func TestAdmissionGenericConflictFamilyRouteIntegrity(t *testing.T) {
	lang := grammars.GoLanguage()
	if lang == nil {
		t.Skip("Go language unavailable")
	}
	family := benchfixtures.GoGenericConflictFamily()
	var routed, declined int
	for _, fixture := range family {
		t.Run(fixture.Name, func(t *testing.T) {
			src := fixture.Source()
			prod := gts.NewParser(lang)
			prod.SetAdmissionCandidateRoute(false)
			prodTree, err := prod.Parse(src)
			if err != nil {
				t.Fatalf("production parse: %v", err)
			}
			defer prodTree.Release()
			if prodTree.RootNode() == nil {
				t.Fatal("production returned no root")
			}
			if prodTree.RootNode().HasError() {
				t.Fatalf("production reference has an error node for %q", fixture.Expression)
			}
			gts.ResetAdmissionCandidateCountersForTest()
			cand := gts.NewParser(lang)
			cand.SetAdmissionCandidateRoute(true)
			candTree, err := cand.Parse(src)
			if err != nil {
				t.Fatalf("candidate parse: %v", err)
			}
			defer candTree.Release()
			if candTree.RootNode() == nil || candTree.RootNode().HasError() {
				t.Fatalf("candidate returned no clean root for %q", fixture.Expression)
			}
			candRouted, candFallback := gts.AdmissionCandidateCounters()
			switch {
			case candRouted == 1 && candFallback == 0:
				routed++
			case candRouted == 0 && candFallback == 1:
				declined++
			default:
				t.Fatalf("ambiguous routing counters for %q: routed=%d fallback=%d", fixture.Expression, candRouted, candFallback)
			}
			t.Logf("%q routed=%d fallback=%d", fixture.Expression, candRouted, candFallback)
		})
	}
	t.Logf("generic family: %d routed, %d declined, %d fixtures", routed, declined, len(family))
}

// Measure the repeated generic call that exercises merge overflow ordering.
func BenchmarkGoParseGenericExpressionDFA(b *testing.B) {
	var source []byte
	for _, fixture := range benchfixtures.GoGenericConflictFamily() {
		if fixture.Name == "in_expression" {
			source = fixture.Source()
			break
		}
	}
	if source == nil {
		b.Fatal("generic expression fixture is missing")
	}
	lang := grammars.GoLanguage()
	parser := gts.NewParser(lang)
	parser.SetAdmissionCandidateRoute(false)
	tree, err := parser.Parse(source)
	if err != nil {
		releaseBenchmarkTree(tree)
		b.Fatal(err)
	}
	requireCompleteParse(b, tree, source, lang, "generic expression warmup")
	tree.Release()
	b.ReportAllocs()
	b.SetBytes(int64(len(source)))
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		tree, err := parser.Parse(source)
		if err != nil {
			releaseBenchmarkTree(tree)
			b.Fatal(err)
		}
		requireCompleteParse(b, tree, source, lang, "generic expression")
		tree.Release()
	}
}
