package gotreesitter

import (
	"fmt"
	"os"
	"strings"
	"testing"
)

// Nested-parenthesis scaling fixture. arbiter's grammar (same author,
// Apache-2.0; grammar.bin copied read-only into testdata/glr_scaling) forks
// the GLR parser's stack while resolving paren_expr and collapses it back on
// most tokens, which is the exact fork/collapse-per-token pattern that made
// tryDemoteSingleLinearGSS (parser.go) and gssNodeCanReach (glr.go)
// superlinear before this fix:
//
//   - tryDemoteSingleLinearGSS materialized the whole GSS chain back to a
//     flat array on every single-stack transition, and the very next fork
//     rebuilt an all-new GSS chain from that array (glrStack.ensureGSS) --
//     both O(depth), recurring every token.
//   - gssNodeCanReach ran an unbounded DFS toward the GSS root on every
//     merge-candidate check, without pruning on the depth already stored on
//     gssNode.
//
// See TestGSSDemotionHysteresisScaling and TestGSSCanReachVisitScaling.
const glrScalingGrammarPath = "testdata/glr_scaling/arbiter_grammar.bin"

func loadGLRScalingLanguage(tb testing.TB) *Language {
	tb.Helper()
	blob, err := os.ReadFile(glrScalingGrammarPath)
	if err != nil {
		tb.Fatalf("read %s: %v", glrScalingGrammarPath, err)
	}
	lang, err := LoadLanguage(blob)
	if err != nil {
		tb.Fatalf("LoadLanguage(%s): %v", glrScalingGrammarPath, err)
	}
	return lang
}

// nestedParenSource builds a valid arbiter rule whose "when" condition is
// wrapped in `depth` levels of parenthesized grouping: "rule R { when {
// (((...x > 1...))) } then A {} }". arbiter's paren_expr = "(" expr ")" lets
// this nest to any depth, and the grammar's ambiguity in disambiguating a
// grouping paren from the surrounding expression forks and re-collapses the
// GLR stack close to once per '(' and ')'.
func nestedParenSource(depth int) []byte {
	var b strings.Builder
	b.WriteString("rule R { when { ")
	for i := 0; i < depth; i++ {
		b.WriteByte('(')
	}
	b.WriteString("x > 1")
	for i := 0; i < depth; i++ {
		b.WriteByte(')')
	}
	b.WriteString(" } then A {} }")
	return []byte(b.String())
}

func parseNestedParens(tb testing.TB, lang *Language, depth int) *Tree {
	tb.Helper()
	parser := NewParser(lang)
	tree, err := parser.Parse(nestedParenSource(depth))
	if err != nil {
		tb.Fatalf("Parse(depth=%d): %v", depth, err)
	}
	if tree.RootNode().HasError() {
		tb.Fatalf("Parse(depth=%d) produced ERROR nodes; fixture no longer parses cleanly", depth)
	}
	return tree
}

// assertSublinearRatio fails if any consecutive depth-doubling in values
// grew by more than maxRatio. It compares (value+1) so a fix that drives a
// counter to exactly 0 at every depth (observed for both GSSNodesDemoted and
// GSSCanReachVisits after this fix) still yields a well-defined, passing
// ratio instead of a divide-by-zero.
func assertSublinearRatio(t *testing.T, label string, depths []int, values []uint64, maxRatio float64) {
	t.Helper()
	for i := 1; i < len(values); i++ {
		prev, cur := values[i-1], values[i]
		ratio := float64(cur+1) / float64(prev+1)
		t.Logf("%s: depth %d->%d: %d -> %d (ratio %.2f)", label, depths[i-1], depths[i], prev, cur, ratio)
		if ratio > maxRatio {
			t.Errorf("%s: depth %d->%d ratio = %.2f, want <= %.2f (prev=%d cur=%d)",
				label, depths[i-1], depths[i], ratio, maxRatio, prev, cur)
		}
	}
}

// TestGSSDemotionHysteresisScaling proves tryDemoteSingleLinearGSS no longer
// thrashes on an input that forks and collapses the GLR stack close to once
// per token. Before the hysteresis fix, GSSNodesDemoted grew close to
// quadratically with depth: a demotion fired on nearly every single-stack
// token, and each one re-walked (materialized) the whole accumulated GSS
// chain. This fails on the pre-fix tree (ratios there are close to 4x per
// depth doubling, i.e. O(depth^2)).
func TestGSSDemotionHysteresisScaling(t *testing.T) {
	lang := loadGLRScalingLanguage(t)
	depths := []int{100, 200, 400, 800}
	demoted := make([]uint64, len(depths))
	demotions := make([]uint64, len(depths))
	for i, d := range depths {
		rt := parseNestedParens(t, lang, d).ParseRuntime()
		demoted[i] = rt.GSSNodesDemoted
		demotions[i] = rt.GSSDemotions
		t.Logf("depth=%d demotions=%d nodesDemoted=%d", d, rt.GSSDemotions, rt.GSSNodesDemoted)
	}
	assertSublinearRatio(t, "GSSNodesDemoted", depths, demoted, 2.5)
	assertSublinearRatio(t, "GSSDemotions", depths, demotions, 2.5)
}

// TestGSSCanReachVisitScaling proves gssNodeCanReach no longer walks toward
// the GSS root on every merge-candidate check. It requires -tags perf, which
// wires GSSCanReachVisits to a real counter (perf_metrics_perf.go); without
// that tag the counter is a compiled-out no-op (perf_metrics_stub.go), so the
// test skips rather than asserting on a value that can never move.
func TestGSSCanReachVisitScaling(t *testing.T) {
	if !perfCountersEnabled {
		t.Skip("requires -tags perf: GSSCanReachVisits is a stub build without it")
	}
	lang := loadGLRScalingLanguage(t)
	depths := []int{100, 200, 400, 800}
	visits := make([]uint64, len(depths))
	for i, d := range depths {
		ResetPerfCounters()
		parseNestedParens(t, lang, d)
		visits[i] = PerfCountersSnapshot().GSSCanReachVisits
		t.Logf("depth=%d gssNodeCanReach visits=%d", d, visits[i])
	}
	assertSublinearRatio(t, "GSSCanReachVisits", depths, visits, 2.5)
}

// BenchmarkGLRNestedParenScaling reports wall-clock parse time at each
// depth for the before/after comparison in the fix's report. It is
// informational (no pass/fail threshold): wall time on a shared/busy host is
// noisy, which is exactly why TestGSSDemotionHysteresisScaling and
// TestGSSCanReachVisitScaling above assert on deterministic operation counts
// instead. Run with: go test -run NONE -bench BenchmarkGLRNestedParenScaling
// -tags perf -benchtime 5x
func BenchmarkGLRNestedParenScaling(b *testing.B) {
	lang := loadGLRScalingLanguage(b)
	for _, depth := range []int{100, 200, 400, 800} {
		src := nestedParenSource(depth)
		b.Run(fmt.Sprintf("depth=%d", depth), func(b *testing.B) {
			for i := 0; i < b.N; i++ {
				parser := NewParser(lang)
				tree, err := parser.Parse(src)
				if err != nil {
					b.Fatal(err)
				}
				if tree.RootNode().HasError() {
					b.Fatal("parse produced ERROR nodes")
				}
			}
		})
	}
}
