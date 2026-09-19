//go:build !gts_no_parsercorephase0

package gotreesitter

import "testing"

// TestMarkStoppedEarlyTreeHasErrorAppliesOnCompactFinalizeTail exercises the
// compact candidate route's own finalize tail (admission_switch_candidate.go)
// directly, since the compact engine's own strict acceptance gate declines
// (falls back to production) rather than ever returning a genuinely
// truncated tree in practice. This proves the wiring in
// finalizeCompactReturnedTreeForParse without depending on provoking a
// truncation inside the separate compact engine's own work counters.
func TestMarkStoppedEarlyTreeHasErrorAppliesOnCompactFinalizeTail(t *testing.T) {
	source := []byte("1+1")
	root := NewLeafNode(1, true, 0, 1, Point{}, Point{Column: 1})
	tree := NewTree(root, source, buildArithmeticLanguage())
	tree.parseRuntime.StopReason = ParseStopNodeLimit
	tree.parseRuntime.RootEndByte = root.EndByte()
	tree.parseRuntime.ExpectedEOFByte = uint32(len(source))
	tree.parseRuntime.Truncated = true

	parser := NewParser(buildArithmeticLanguage())
	parser.finalizeCompactReturnedTreeForParse(tree, source)

	if !root.HasError() {
		t.Fatalf("HasError() = false after finalizeCompactReturnedTreeForParse, want true; runtime=%s", tree.ParseRuntime().Summary())
	}
}
