//go:build cgo && treesitter_c_parity

package cgoharness

// C-oracle coverage for the included-ranges route, Kotlin arm.
//
// normalizeKotlinRecoveredSourceFileRoot was deleted on 2026-08-02 as dead code
// and is restored. The census behind the deletion measured the fresh,
// over-64-KiB, incremental and pinned routes and never measured
// Parser.SetIncludedRanges. On that route the member used to fire and move the
// published root toward this oracle: without it the root was `ERROR`, with it
// the root was `source_file`, and the oracle publishes `source_file`.
//
// Update: the parser's padding scan now clips to the included ranges, so this
// route no longer forces recovery on this fixture. The root already comes out
// as `source_file` before the member inspects it, and the member no longer
// fires here. The root span and child count now match the oracle too; only
// the root's HasError flag still diverges. See the doc comment on
// TestIncludedRangesKotlinRootParity below for the current pins.
//
// Production reaches the route through injection: injection.go calls
// childParser.SetIncludedRanges for every injected child.
//
// Run: GTS_PARITY_ALLOW_HOST=1 CGO_ENABLED=1 \
//        go test . -tags "cgo treesitter_c_parity" \
//        -run TestIncludedRangesKotlinRootParity -v

import (
	"os"
	"path/filepath"
	"testing"

	gts "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
	sitter "github.com/tree-sitter/go-tree-sitter"
)

// includedRangesKotlinSpans matches the spans pinned by the non-cgo gate in
// parser_included_ranges_kotlin_root_test.go. Two interior gaps carry
// non-whitespace Kotlin text.
var includedRangesKotlinSpans = [3][2]int{{0, 795}, {883, 1987}, {2019, 3094}}

func includedRangesKotlinPointAt(src []byte, off int) (uint, uint) {
	var row, col uint
	for i := 0; i < off && i < len(src); i++ {
		if src[i] == '\n' {
			row++
			col = 0
			continue
		}
		col++
	}
	return row, col
}

// TestIncludedRangesKotlinRootParity pins the root symbol against the locked
// Kotlin C oracle on the included-ranges route, and pins the route's one
// remaining known divergence so a change surfaces instead of passing
// silently.
//
// The root kind, span, and child count are hard parity assertions now that
// the padding scan clips to the included ranges: the parser no longer drops
// the third included range, and the restored arm member is not needed to
// reach the right root symbol on this fixture. The root's HasError flag
// still diverges from the oracle and is pinned, not asserted equal. If a
// future change closes that divergence too, this test fails and the pin
// moves to the oracle value.
func TestIncludedRangesKotlinRootParity(t *testing.T) {
	cLang, err := COracleLanguage("kotlin")
	if err != nil {
		t.Fatalf("load kotlin C oracle: %v", err)
	}
	goLang := grammars.KotlinLanguage()
	if goLang == nil {
		t.Skip("kotlin grammar unavailable")
	}

	path := filepath.Join("..", "testdata", "included_ranges", "kotlin_work_queue_test.kt")
	src, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read included-ranges fixture: %v", err)
	}

	cRanges := make([]sitter.Range, 0, len(includedRangesKotlinSpans))
	goRanges := make([]gts.Range, 0, len(includedRangesKotlinSpans))
	for _, span := range includedRangesKotlinSpans {
		start, end := span[0], span[1]
		if end > len(src) {
			end = len(src)
		}
		sr, sc := includedRangesKotlinPointAt(src, start)
		er, ec := includedRangesKotlinPointAt(src, end)
		cRanges = append(cRanges, sitter.Range{
			StartByte:  uint(start),
			EndByte:    uint(end),
			StartPoint: sitter.Point{Row: sr, Column: sc},
			EndPoint:   sitter.Point{Row: er, Column: ec},
		})
		goRanges = append(goRanges, gts.Range{
			StartByte:  uint32(start),
			EndByte:    uint32(end),
			StartPoint: gts.Point{Row: uint32(sr), Column: uint32(sc)},
			EndPoint:   gts.Point{Row: uint32(er), Column: uint32(ec)},
		})
	}

	cParser := sitter.NewParser()
	defer cParser.Close()
	if err := cParser.SetLanguage(cLang); err != nil {
		t.Fatalf("set C language: %v", err)
	}
	if err := cParser.SetIncludedRanges(cRanges); err != nil {
		t.Fatalf("C SetIncludedRanges: %v", err)
	}
	cTree := cParser.Parse(src, nil)
	if cTree == nil {
		t.Fatal("C parse returned nil")
	}
	defer cTree.Close()
	cRoot := cTree.RootNode()

	goParser := gts.NewParser(goLang)
	goParser.SetIncludedRanges(goRanges)
	goTree, err := goParser.Parse(src)
	if err != nil {
		t.Fatalf("go parse: %v", err)
	}
	if goTree == nil || goTree.RootNode() == nil {
		t.Fatal("go parse returned no tree")
	}
	defer goTree.Release()
	goRoot := goTree.RootNode()

	t.Logf("C   root kind=%s isError=%v hasError=%v span=[%d,%d) children=%d",
		cRoot.Kind(), cRoot.IsError(), cRoot.HasError(),
		cRoot.StartByte(), cRoot.EndByte(), cRoot.ChildCount())
	t.Logf("GTS root kind=%s isError=%v hasError=%v span=[%d,%d) children=%d",
		goRoot.Type(goLang), goRoot.IsError(), goRoot.HasError(),
		goRoot.StartByte(), goRoot.EndByte(), goRoot.ChildCount())

	// The regression guard. The padding scan clips to the included ranges now,
	// so the route no longer forces recovery here and the restored arm member
	// is not needed to reach this root symbol.
	if got, want := goRoot.Type(goLang), cRoot.Kind(); got != want {
		t.Fatalf("included-ranges root kind: gotreesitter=%q C=%q", got, want)
	}
	if goRoot.IsError() {
		t.Fatal("included-ranges root is an ERROR node while C publishes a named root")
	}
	if got, want := goRoot.StartByte(), uint32(cRoot.StartByte()); got != want {
		t.Errorf("included-ranges root start byte: gotreesitter=%d C=%d", got, want)
	}

	// Span and child count now match the C oracle: the padding-scan fix
	// stopped the route from dropping the third included range.
	const (
		pinnedEnd      = uint32(3094)
		pinnedChildren = 8
	)
	if got := goRoot.EndByte(); got != pinnedEnd {
		t.Errorf("included-ranges root end byte: gotreesitter=%d, want %d (matches C)", got, pinnedEnd)
	}
	if got := uint32(cRoot.EndByte()); got != pinnedEnd {
		t.Errorf("C oracle root end byte moved: got %d, pinned %d", got, pinnedEnd)
	}
	if got := int(cRoot.ChildCount()); got != pinnedChildren {
		t.Errorf("C oracle root child count = %d, pinned %d", got, pinnedChildren)
	}
	if got := goRoot.ChildCount(); got != pinnedChildren {
		t.Errorf("included-ranges root child count: gotreesitter=%d, want %d (matches C)", got, pinnedChildren)
	}

	// The one field that still diverges from the oracle. Not asserted as
	// correct; the pin makes a future change visible instead of silent.
	if cRoot.HasError() != false || goRoot.HasError() != true {
		t.Errorf("included-ranges root HasError moved: C=%v gotreesitter=%v, pinned C=false gotreesitter=true",
			cRoot.HasError(), goRoot.HasError())
	}
}
