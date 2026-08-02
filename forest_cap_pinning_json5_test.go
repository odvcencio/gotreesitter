package gotreesitter_test

import (
	"bytes"
	"fmt"
	"testing"

	gotreesitter "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
)

// TestForestCapTiePinningJSON5FlatArrayTreeParity is the shipped-table
// regression guard for span-maximal pinning (glr_forest.go,
// forestCapReplacementIndex / forestCapTiePinsCandidate).
//
// json5 is a shipped, default-on forest language whose flat-array grammar
// shape fires the hidden-symbol cap-tie arm constantly -- unlike the
// javascript repeated-top-level-item repro this feature's evidence base
// used, which never ties on the shipped table. A 7-item array already
// fires it (`[0,1,2,3,4,5,6]`, 15 bytes), and a large flat array fires it
// tens of thousands of times. This test asserts the pin never changes the
// observable tree (a full node-by-node fingerprint against a
// forced-production parse of the identical source) across a range of N,
// and it records CandidatesPinned so a future table resync that moves the
// count is visible in this test's own log output rather than silently.
func TestForestCapTiePinningJSON5FlatArrayTreeParity(t *testing.T) {
	build := func(n int) []byte {
		var b bytes.Buffer
		b.WriteByte('[')
		for i := 0; i < n; i++ {
			if i > 0 {
				b.WriteByte(',')
			}
			fmt.Fprintf(&b, "%d", i)
		}
		b.WriteByte(']')
		return b.Bytes()
	}
	lang := grammars.Json5Language()
	totalHidden, totalPinned, totalNodes := 0, 0, 0
	for _, n := range []int{1, 5, 7, 8, 10, 20, 50, 100, 200, 400} {
		src := build(n)

		forestParser := gotreesitter.NewParser(lang)
		forestTree, err := forestParser.Parse(src)
		if err != nil || forestTree == nil {
			t.Fatalf("N=%d: forest-routed parse failed: %v", n, err)
		}
		if !forestTree.UsedForestFastPath() {
			t.Fatalf("N=%d: expected forest routing on the shipped json5 table", n)
		}
		stats := forestParser.ForestCapTieStats()
		totalHidden += stats.HiddenTieDecisions
		totalPinned += stats.CandidatesPinned

		gotreesitter.SetGLRForestEnabled(false)
		prodParser := gotreesitter.NewParser(lang)
		prodTree, err := prodParser.Parse(src)
		gotreesitter.SetGLRForestEnabled(true)
		if err != nil || prodTree == nil {
			t.Fatalf("N=%d: production parse failed: %v", n, err)
		}

		totalNodes += forestCapPinningJSON5FingerprintDiff(t, n, forestTree.RootNode(), prodTree.RootNode())
		forestTree.Release()
		prodTree.Release()
	}
	t.Logf("json5 flat-array N sweep: nodes_compared=%d HiddenTieDecisions=%d CandidatesPinned=%d", totalNodes, totalHidden, totalPinned)
	if totalPinned == 0 {
		t.Fatal("span-maximal pinning never fired on the json5 flat-array sweep; the shipped-table regression this test exists to catch would be invisible")
	}
}

// TestForestCapTiePinningJSON5LargeArrayTreeParity is the same guard at the
// scale the reviewer measured (~108KB, ~20000 items): a single large parse
// with tens of thousands of cap-tie decisions in one pass.
func TestForestCapTiePinningJSON5LargeArrayTreeParity(t *testing.T) {
	var b bytes.Buffer
	b.WriteByte('[')
	for i := 0; i < 20000; i++ {
		if i > 0 {
			b.WriteByte(',')
		}
		fmt.Fprintf(&b, "%d", i)
	}
	b.WriteByte(']')
	src := b.Bytes()

	lang := grammars.Json5Language()
	forestParser := gotreesitter.NewParser(lang)
	forestTree, err := forestParser.Parse(src)
	if err != nil || forestTree == nil {
		t.Fatalf("forest-routed parse failed: %v", err)
	}
	if !forestTree.UsedForestFastPath() {
		t.Fatal("expected forest routing on the shipped json5 table for the large array")
	}
	stats := forestParser.ForestCapTieStats()

	gotreesitter.SetGLRForestEnabled(false)
	prodParser := gotreesitter.NewParser(lang)
	prodTree, err := prodParser.Parse(src)
	gotreesitter.SetGLRForestEnabled(true)
	if err != nil || prodTree == nil {
		t.Fatalf("production parse failed: %v", err)
	}

	nodes := forestCapPinningJSON5FingerprintDiff(t, len(src), forestTree.RootNode(), prodTree.RootNode())
	t.Logf("json5 large array (%d bytes): nodes_compared=%d HiddenTieDecisions=%d CandidatesPinned=%d", len(src), nodes, stats.HiddenTieDecisions, stats.CandidatesPinned)
	if stats.CandidatesPinned == 0 {
		t.Fatal("span-maximal pinning never fired on the large json5 array; the scale this test exists to cover would be invisible")
	}
	forestTree.Release()
	prodTree.Release()
}

func forestCapPinningJSON5FingerprintDiff(t *testing.T, n int, a, b *gotreesitter.Node) int {
	t.Helper()
	if (a == nil) != (b == nil) {
		t.Fatalf("N=%d: nil mismatch: forest=%v production=%v", n, a == nil, b == nil)
	}
	if a == nil {
		return 0
	}
	if a.Symbol() != b.Symbol() || a.StartByte() != b.StartByte() || a.EndByte() != b.EndByte() ||
		a.IsNamed() != b.IsNamed() || a.IsExtra() != b.IsExtra() || a.IsMissing() != b.IsMissing() ||
		a.HasError() != b.HasError() || a.ChildCount() != b.ChildCount() {
		t.Fatalf("N=%d: node fingerprint diverged: forest={sym=%d start=%d end=%d named=%v extra=%v missing=%v err=%v children=%d} production={sym=%d start=%d end=%d named=%v extra=%v missing=%v err=%v children=%d}",
			n, a.Symbol(), a.StartByte(), a.EndByte(), a.IsNamed(), a.IsExtra(), a.IsMissing(), a.HasError(), a.ChildCount(),
			b.Symbol(), b.StartByte(), b.EndByte(), b.IsNamed(), b.IsExtra(), b.IsMissing(), b.HasError(), b.ChildCount())
	}
	count := 1
	for i := 0; i < a.ChildCount(); i++ {
		count += forestCapPinningJSON5FingerprintDiff(t, n, a.Child(i), b.Child(i))
	}
	return count
}
