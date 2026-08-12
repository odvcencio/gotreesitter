package gotreesitter_test

import (
	"os"
	"testing"

	"github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
)

// The included-ranges route reaches the Kotlin result-compatibility arm and no
// test covered it. normalizeKotlinRecoveredSourceFileRoot was deleted on
// 2026-08-02 as dead code. The census behind that deletion measured the fresh,
// over-64-KiB, incremental and pinned routes. It never measured
// Parser.SetIncludedRanges. The member is live on that fifth route: it
// publishes a `source_file` root where the deletion published `ERROR`.
//
// Production reaches this route through injection. injection.go calls
// childParser.SetIncludedRanges for every injected child, and any host document
// that carries two or more Kotlin regions produces exactly this shape.
//
// These tests are the route's committed floor for Kotlin. The C-oracle
// comparison lives in
// cgo_harness/included_ranges_kotlin_root_parity_cgo_test.go.
//
// Update: the parser's padding scan now clips to the included ranges, so
// this route no longer forces recovery on this fixture and the root already
// comes out as `source_file` before normalizeKotlinRecoveredSourceFileRoot
// inspects it. See TestIncludedRangesKotlinArmMemberNowInert below.
//
// Fixture provenance: testdata/included_ranges/kotlin_work_queue_test.kt is a
// byte-exact copy of kotlinx-coroutines-core/jvm/test/scheduling/WorkQueueTest.kt
// from the kotlinx.coroutines project, Apache License 2.0. The copy is byte
// exact because the pinned ranges below are byte offsets into it.

func includedRangesKotlinPointAt(src []byte, off int) gotreesitter.Point {
	var point gotreesitter.Point
	for i := 0; i < off && i < len(src); i++ {
		if src[i] == '\n' {
			point.Row++
			point.Column = 0
			continue
		}
		point.Column++
	}
	return point
}

// includedRangesKotlinFixtureSpans is the fixture's three included ranges. Two
// interior gaps carry non-whitespace Kotlin text, the way an injection child
// receives separated regions of a host document.
var includedRangesKotlinFixtureSpans = [3][2]int{{0, 795}, {883, 1987}, {2019, 3094}}

func loadIncludedRangesKotlinFixture(tb testing.TB) ([]byte, []gotreesitter.Range) {
	tb.Helper()
	src, err := os.ReadFile("testdata/included_ranges/kotlin_work_queue_test.kt")
	if err != nil {
		tb.Fatalf("read included-ranges fixture: %v", err)
	}
	ranges := make([]gotreesitter.Range, 0, len(includedRangesKotlinFixtureSpans))
	for _, span := range includedRangesKotlinFixtureSpans {
		start, end := span[0], span[1]
		if end > len(src) {
			end = len(src)
		}
		ranges = append(ranges, gotreesitter.Range{
			StartByte:  uint32(start),
			EndByte:    uint32(end),
			StartPoint: includedRangesKotlinPointAt(src, start),
			EndPoint:   includedRangesKotlinPointAt(src, end),
		})
	}
	return src, ranges
}

// TestIncludedRangesKotlinRootStaysSourceFile pins the root symbol on the
// included-ranges route. The Kotlin arm's normalizeKotlinRecoveredSourceFileRoot
// member owns this result. Removing the member turns the root into `ERROR`,
// which diverges from the locked Kotlin C oracle. That oracle publishes
// `source_file` for the same input and the same ranges.
func TestIncludedRangesKotlinRootStaysSourceFile(t *testing.T) {
	lang := grammars.KotlinLanguage()
	if lang == nil {
		t.Skip("kotlin grammar unavailable")
	}
	src, ranges := loadIncludedRangesKotlinFixture(t)

	parser := gotreesitter.NewParser(lang)
	parser.SetIncludedRanges(ranges)
	tree, err := parser.Parse(src)
	if err != nil {
		t.Fatalf("included-ranges parse: %v", err)
	}
	if tree == nil || tree.RootNode() == nil {
		t.Fatal("included-ranges parse returned no tree")
	}
	defer tree.Release()

	root := tree.RootNode()
	if got := root.Type(lang); got != "source_file" {
		t.Fatalf("included-ranges root type = %q, want %q", got, "source_file")
	}
	if root.IsError() {
		t.Fatal("included-ranges root is an ERROR node")
	}
}

// TestIncludedRangesKotlinArmMemberNowInert records the current state of the
// arm's recovered-root member on this route: the parser's padding scan now
// clips to the configured included ranges, so the parse no longer forces
// recovery between the three ranges and never hands the member an ERROR
// root to repair. The member still runs on every parse; it now returns
// immediately on this fixture because the root already carries the right
// symbol before the member inspects it. If a future change reopens this
// route to recovery, the member starts rewriting the tree again and the
// assertion below catches it.
func TestIncludedRangesKotlinArmMemberNowInert(t *testing.T) {
	lang := grammars.KotlinLanguage()
	if lang == nil {
		t.Skip("kotlin grammar unavailable")
	}
	t.Setenv("GTS_DISPATCHER_CENSUS", "1")
	src, ranges := loadIncludedRangesKotlinFixture(t)

	parser := gotreesitter.NewParser(lang)
	parser.SetIncludedRanges(ranges)
	tree, err := parser.Parse(src)
	if err != nil {
		t.Fatalf("included-ranges parse: %v", err)
	}
	if tree == nil {
		t.Fatal("included-ranges parse returned no tree")
	}
	defer tree.Release()

	if got := tree.RootNode().Type(lang); got != "source_file" {
		t.Fatalf("included-ranges root type = %q, want %q", got, "source_file")
	}

	// Census receipts are recorded on the production route only. An
	// included-ranges parse always lands there today, because
	// admission_switch.go declines the compact candidate route whenever the
	// parser carries included ranges. This is a Fatal, not a Skip: if the
	// compact runner ever gains included-ranges support, this test must fail
	// loudly instead of turning into a silent skip that stops watching the
	// member.
	passesPtr := tree.ParseRuntime().NormalizationPasses
	if passesPtr == nil || len(*passesPtr) == 0 {
		t.Fatal("included-ranges parse recorded no census receipts; the route no longer pins production")
	}
	const subpass = "dispatch.kotlin.recovered-source-file-root"
	var found bool
	for _, pass := range *passesPtr {
		if pass.Name != subpass {
			continue
		}
		found = true
		if pass.NodesRewritten != 0 {
			t.Fatalf("%s rewrote %d nodes, want 0: the root should already carry the right symbol before the member runs", subpass, pass.NodesRewritten)
		}
	}
	if !found {
		t.Fatalf("%s has no census receipt on the included-ranges route", subpass)
	}
}

// TestIncludedRangesKotlinRootHasErrorPinned pins the route's one remaining
// known divergence from the C oracle now that the padding scan clips to the
// included ranges: the root's HasError flag. Every other measured field now
// matches the C oracle exactly on this fixture: span, child count, and root
// kind. See TestIncludedRangesKotlinRootParity in
// cgo_harness/included_ranges_kotlin_root_parity_cgo_test.go for the
// C-oracle receipt these pins came from.
//
// Before the padding-scan fix, this test pinned a larger defect: the parse
// stopped with no live GLR stacks and the root dropped the third included
// range entirely, ending at byte 1986 instead of 3094. That truncation is
// gone. If the HasError divergence below ever closes too, update this pin
// from a fresh C-oracle receipt.
func TestIncludedRangesKotlinRootHasErrorPinned(t *testing.T) {
	lang := grammars.KotlinLanguage()
	if lang == nil {
		t.Skip("kotlin grammar unavailable")
	}
	src, ranges := loadIncludedRangesKotlinFixture(t)

	parser := gotreesitter.NewParser(lang)
	parser.SetIncludedRanges(ranges)
	tree, err := parser.Parse(src)
	if err != nil {
		t.Fatalf("included-ranges parse: %v", err)
	}
	if tree == nil || tree.RootNode() == nil {
		t.Fatal("included-ranges parse returned no tree")
	}
	defer tree.Release()

	root := tree.RootNode()
	const (
		pinnedEnd        = uint32(3094) // matches the C oracle
		pinnedChildCount = 8            // matches the C oracle
	)
	if got := root.StartByte(); got != 0 {
		t.Errorf("included-ranges root start byte = %d, want 0 (matches the C oracle)", got)
	}
	if got := root.EndByte(); got != pinnedEnd {
		t.Errorf("included-ranges root end byte = %d, want %d (matches the C oracle)", got, pinnedEnd)
	}
	if got := root.ChildCount(); got != pinnedChildCount {
		t.Errorf("included-ranges root child count = %d, want %d (matches the C oracle)", got, pinnedChildCount)
	}
	if root.IsError() {
		t.Error("included-ranges root is an ERROR node")
	}
	if !root.HasError() {
		t.Error("included-ranges root reports no error; the pinned value is true while the C oracle reports false — refresh this pin from a fresh C-oracle receipt if this ever flips")
	}
}
