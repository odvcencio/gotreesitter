//go:build cgo && treesitter_c_parity && gts_parsercorephase0 && !gts_no_parsercorephase0

package cgoharness

import (
	"testing"

	"github.com/odvcencio/gotreesitter/cgo_harness/internal/nativealloc"
	sitter "github.com/tree-sitter/go-tree-sitter"
)

// Collect allocation counts separately from timing. The oracle's allocator
// hook covers the runtime; each UTF-8 bridge callback requests one CString of
// len(chunk)+1 bytes outside that hook. go-pointer v0.0.1 also requests one
// byte for the input payload handle. The C Go grammar has no external scanner.
func TestAccountingCompleteCNativeAllocations(t *testing.T) {
	for _, tc := range loadCanonicalGoIncrementalCases(t) {
		if tc.spec.Name != "same_line_length_change" {
			continue
		}
		lang := canonicalIncrementalCLanguage(t, "go")
		sitter.SetAllocator(nativealloc.Malloc, nativealloc.Calloc, nativealloc.Realloc, nativealloc.Free)
		defer sitter.SetAllocator(nil, nil, nil, nil)
		parser := sitter.NewParser()
		defer parser.Close()
		if err := parser.SetLanguage(lang); err != nil {
			t.Fatal(err)
		}
		tree := parser.Parse(tc.source, nil)
		requireCanonicalCIncrementalTree(t, tree, tc.source, "native allocation initial")
		defer func() { tree.Close() }()
		edited := false
		bridge := nativealloc.Counts{}
		step := func() {
			target, edit := tc.edited, tc.cForward
			if edited {
				target, edit = tc.source, tc.cReverse
			}
			tree.Edit(&edit)
			// The pinned bridge saves one input payload with malloc(1).
			bridge.Bytes++
			bridge.Allocations++
			next := parser.ParseWith(func(i int, _ sitter.Point) []byte {
				chunk := target[len(target):]
				if i < len(target) {
					chunk = target[i:]
				}
				bridge.Bytes += uint64(len(chunk) + 1)
				bridge.Allocations++
				return chunk
			}, tree)
			requireCanonicalCIncrementalTree(t, next, target, "native allocation complete edit")
			tree.Close()
			tree = next
			edited = !edited
		}
		for i := 0; i < 32; i++ {
			step()
		}
		var priorCore, priorBridge nativealloc.Counts
		const operations = 128
		for block := 0; block < 2; block++ {
			nativealloc.Reset()
			bridge = nativealloc.Counts{}
			for i := 0; i < operations; i++ {
				step()
			}
			core := nativealloc.Snapshot()
			if core.Bytes == 0 || core.Allocations == 0 || bridge.Bytes == 0 || bridge.Allocations == 0 {
				t.Fatalf("missing native work: core=%+v bridge=%+v", core, bridge)
			}
			if block == 1 && (core != priorCore || bridge != priorBridge) {
				t.Fatalf("unstable warmed allocation counts: first core=%+v bridge=%+v; second core=%+v bridge=%+v", priorCore, priorBridge, core, bridge)
			}
			priorCore, priorBridge = core, bridge
			t.Logf("workload=%s operations=%d block=%d runtime_requested_B/op=%.3f runtime_allocs/op=%.3f bridge_requested_B/op=%.3f bridge_allocs/op=%.3f native_requested_B/op=%.3f native_allocs/op=%.3f", tc.spec.Name, operations, block, float64(core.Bytes)/operations, float64(core.Allocations)/operations, float64(bridge.Bytes)/operations, float64(bridge.Allocations)/operations, float64(core.Bytes+bridge.Bytes)/operations, float64(core.Allocations+bridge.Allocations)/operations)
		}
		fresh := parser.Parse(tc.source, nil)
		requireCanonicalCIncrementalTree(t, fresh, tc.source, "native allocation fresh")
		defer fresh.Close()
		got, want := canonicalCTreeDigest(t, tree, "native allocation result"), canonicalCTreeDigest(t, fresh, "native allocation fresh")
		if got != want {
			t.Fatalf("native allocation measurement changed result: got=%s want=%s", got, want)
		}
		t.Logf("complete_tree_digest=%s", got)
		return
	}
	t.Fatal("missing fixed workload")
}
