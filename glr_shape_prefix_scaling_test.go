package gotreesitter

import (
	"fmt"
	"testing"
)

// TestGSSShapePrefixWalkScaling proves the shape-prefix cache survives the
// fork/collapse-per-token pattern. Every successful boundary merge used to
// invalidate the whole cache (bumpShapePrefixEpoch) whether or not the merge
// rewrote a link 0, so the next head hash rewalked the entire spine:
// ShapePrefixWalkSteps grew with depth on every token (about 3.9M steps at
// depth 1600, versus about 12K after the fix) and the parse turned
// superlinear past depth 1600 (issue #454, third contributor). The merge
// sites now invalidate only when gssShapePrefixLink0Rewrites moved during the
// merge. Requires -tags perf, like TestGSSCanReachVisitScaling.
func TestGSSShapePrefixWalkScaling(t *testing.T) {
	if !perfCountersEnabled {
		t.Skip("requires -tags perf: ShapePrefixWalkSteps is a stub build without it")
	}
	lang := loadGLRScalingLanguage(t)
	depths := []int{200, 400, 800, 1600}
	steps := make([]uint64, len(depths))
	bumps := make([]uint64, len(depths))
	for i, d := range depths {
		ResetPerfCounters()
		parseNestedParens(t, lang, d)
		c := PerfCountersSnapshot()
		steps[i] = c.ShapePrefixWalkSteps
		bumps[i] = c.ShapePrefixEpochBumps
		t.Logf("depth=%d shapePrefixWalkSteps=%d shapePrefixEpochBumps=%d", d, steps[i], bumps[i])
	}
	assertSublinearRatio(t, "ShapePrefixWalkSteps", depths, steps, 2.5)
	assertSublinearRatio(t, "ShapePrefixEpochBumps", depths, bumps, 2.5)
}

// TestSetGSSMainLinkCountsLink0Rewrites pins the counter the merge sites use
// to decide whether a successful merge invalidated the shape-prefix cache: a
// link-0 write that changes prev or entry moves it, an identical rewrite does
// not, and the conditional bump only fires when the counter moved past the
// snapshot taken before the merge.
func TestSetGSSMainLinkCountsLink0Rewrites(t *testing.T) {
	root := &gssNode{}
	prevA := &gssNode{prev: root, depth: 1}
	prevB := &gssNode{prev: root, depth: 1}
	entryA := newStackEntryNode(1, &Node{symbol: 1})
	entryB := newStackEntryNode(2, &Node{symbol: 2})
	n := &gssNode{prev: prevA, entry: entryA, depth: 2}

	before := gssShapePrefixLink0Rewrites.Load()
	setGSSMainLink(n, 0, prevA, entryA)
	if got := gssShapePrefixLink0Rewrites.Load(); got != before {
		t.Fatalf("identical link-0 rewrite moved the counter: %d -> %d", before, got)
	}
	setGSSMainLink(n, 0, prevB, entryA)
	if got := gssShapePrefixLink0Rewrites.Load(); got != before+1 {
		t.Fatalf("prev change: counter = %d, want %d", got, before+1)
	}
	setGSSMainLink(n, 0, prevB, entryB)
	if got := gssShapePrefixLink0Rewrites.Load(); got != before+2 {
		t.Fatalf("entry change: counter = %d, want %d", got, before+2)
	}
	var scratch glrMergeScratch
	scratch.shapePrefixEpoch = 7
	scratch.bumpShapePrefixEpochIfLink0Rewritten(gssShapePrefixLink0Rewrites.Load())
	if scratch.shapePrefixEpoch != 7 {
		t.Fatalf("no rewrite since the snapshot, but the epoch moved to %d", scratch.shapePrefixEpoch)
	}
	scratch.bumpShapePrefixEpochIfLink0Rewritten(before)
	if scratch.shapePrefixEpoch != 8 {
		t.Fatalf("rewrites since the snapshot, but the epoch is %d, want 8", scratch.shapePrefixEpoch)
	}
}

// BenchmarkGLRNestedParenScalingDeep extends BenchmarkGLRNestedParenScaling
// past the depth where the shape-prefix rewalk dominated. Informational.
func BenchmarkGLRNestedParenScalingDeep(b *testing.B) {
	lang := loadGLRScalingLanguage(b)
	for _, depth := range []int{1600, 3200} {
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
