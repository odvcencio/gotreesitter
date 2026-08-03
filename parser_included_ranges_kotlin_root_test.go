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

// TestIncludedRangesKotlinArmMemberIsLive records that the arm's recovered-root
// member actually rewrites the tree on this route. It is the positive control
// that makes the test above a real gate instead of an accident: if the member
// ever becomes inert here, this fails and the retirement question reopens with
// evidence.
func TestIncludedRangesKotlinArmMemberIsLive(t *testing.T) {
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

	// Census receipts are recorded on the production route only. An
	// included-ranges parse always lands there today, because
	// admission_switch.go declines the compact candidate route whenever the
	// parser carries included ranges. This is a Fatal, not a Skip: if the
	// compact runner ever gains included-ranges support, this positive control
	// must fail loudly instead of turning into a silent skip that stops
	// guarding the member.
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
		if pass.NodesRewritten == 0 {
			t.Fatalf("%s recorded no rewrite on the included-ranges route", subpass)
		}
		t.Logf("%s rewrote %d nodes", subpass, pass.NodesRewritten)
	}
	if !found {
		t.Fatalf("%s has no census receipt on the included-ranges route", subpass)
	}
}

// TestIncludedRangesKotlinRootCoveragePinned pins the route's known coverage
// defect so a change becomes loud instead of silent.
//
// The parse stops with no_stacks_alive and the root ends at byte 1986, so the
// third included range is dropped. The C oracle publishes `source_file [0,3094)`
// with no error for the same input. The cause is separate from the member this
// file gates: bytesAreParserPadding scans raw bytes between the stack offset and
// the next token start with no range clipping, so a non-whitespace gap between
// ranges kills every GLR stack. The truncation predates the member deletion and
// predates its restoration; it is measured identical on both.
//
// When the clipping fix lands, this test fails. That is the intended signal.
// Update the pins then, from a fresh C-oracle receipt.
func TestIncludedRangesKotlinRootCoveragePinned(t *testing.T) {
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
		pinnedStart = uint32(0)
		pinnedEnd   = uint32(1986)
		oracleEnd   = uint32(3094)
	)
	if got := root.StartByte(); got != pinnedStart {
		t.Errorf("included-ranges root start byte = %d, pinned %d", got, pinnedStart)
	}
	if got := root.EndByte(); got != pinnedEnd {
		t.Errorf("included-ranges root end byte = %d, pinned %d (C oracle publishes %d)",
			got, pinnedEnd, oracleEnd)
	}
	if !root.HasError() {
		t.Error("included-ranges root reports no error; the pinned value is true while the C oracle reports false")
	}
}
