package gotreesitter_test

import (
	"os"
	"testing"

	"github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
)

// The included-ranges route reaches the Go result-compatibility arm and no
// other test covered it. Worker maple-h proposed deleting
// normalizeGoSourceFileRoot after a census over the fresh, over-64-KiB, and
// incremental routes recorded zero rewrites on 22,613 parses. The member is
// live on this fourth route: it publishes a `source_file` root where the
// deletion published `ERROR`.
//
// Production reaches this route through injection. injection.go calls
// childParser.SetIncludedRanges for every injected child, and
// grammars/markdown_injection_register.go maps the `go` and `golang` fence
// languages to the Go grammar, so a Markdown document with two or more Go
// fences produces exactly this shape.
//
// These tests are the route's committed floor. The C-oracle comparison lives
// in cgo_harness/included_ranges_go_root_parity_cgo_test.go.
//
// Update: the parser's padding scan now clips to the included ranges, so
// this route no longer forces recovery on this fixture and the root already
// comes out as `source_file` before normalizeGoSourceFileRoot inspects it.
// See TestIncludedRangesGoArmMemberNowInert below.

func includedRangesGoPointAt(src []byte, off int) gotreesitter.Point {
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

// includedRangesGoFixtureSpans is the fixture's two included ranges. Both
// start at a positive offset, which is the shape an injection child actually
// receives: a Markdown fence cannot begin at byte 0, because the opening
// fence line always precedes the content.
//
// The anchored geometry {{0, 150}, {203, 276}} is deliberately NOT used here.
// It is the one geometry where the root span and child count reach C parity,
// and pinning it would read as a parity claim the route does not support. See
// cgo_harness/included_ranges_go_root_parity_cgo_test.go, which measures both
// geometries and pins each observation separately.
var includedRangesGoFixtureSpans = [2][2]int{{26, 150}, {203, 276}}

func loadIncludedRangesGoFixture(tb testing.TB) ([]byte, []gotreesitter.Range) {
	tb.Helper()
	src, err := os.ReadFile("testdata/included_ranges/go_two_fences.go")
	if err != nil {
		tb.Fatalf("read included-ranges fixture: %v", err)
	}
	ranges := make([]gotreesitter.Range, 0, len(includedRangesGoFixtureSpans))
	for _, span := range includedRangesGoFixtureSpans {
		start, end := span[0], span[1]
		if end > len(src) {
			end = len(src)
		}
		ranges = append(ranges, gotreesitter.Range{
			StartByte:  uint32(start),
			EndByte:    uint32(end),
			StartPoint: includedRangesGoPointAt(src, start),
			EndPoint:   includedRangesGoPointAt(src, end),
		})
	}
	return src, ranges
}

// TestIncludedRangesGoRootStaysSourceFile pins the root symbol on the
// included-ranges route. The Go arm's normalizeGoSourceFileRoot member owns
// this result today. Deleting the member turns the root into `ERROR`, which
// diverges from the C oracle.
func TestIncludedRangesGoRootStaysSourceFile(t *testing.T) {
	lang := grammars.GoLanguage()
	if lang == nil {
		t.Skip("go grammar unavailable")
	}
	src, ranges := loadIncludedRangesGoFixture(t)

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

// TestIncludedRangesGoArmMemberNowInert records the current state of the
// arm's root member on this route: the parser's padding scan now clips to
// the configured included ranges, so the parse no longer forces recovery
// between the two ranges and never hands the member an ERROR root to
// repair. The member still runs on every parse; it now returns immediately
// on this fixture because the root already carries the right symbol before
// the member inspects it. If a future change reopens this route to
// recovery, the member starts rewriting the tree again and the assertion
// below catches it.
func TestIncludedRangesGoArmMemberNowInert(t *testing.T) {
	lang := grammars.GoLanguage()
	if lang == nil {
		t.Skip("go grammar unavailable")
	}
	t.Setenv("GTS_DISPATCHER_CENSUS", "1")
	src, ranges := loadIncludedRangesGoFixture(t)

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
	var found bool
	for _, pass := range *passesPtr {
		if pass.Name != "dispatch.go.source-file-root" {
			continue
		}
		found = true
		if pass.NodesRewritten != 0 {
			t.Fatalf("dispatch.go.source-file-root rewrote %d nodes, want 0: the root should already carry the right symbol before the member runs", pass.NodesRewritten)
		}
	}
	if !found {
		t.Fatal("dispatch.go.source-file-root has no census receipt on the included-ranges route")
	}
}
