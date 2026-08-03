//go:build cgo && treesitter_c_parity

package cgoharness

// C-oracle coverage for the included-ranges route, Kotlin arm.
//
// normalizeKotlinRecoveredSourceFileRoot was deleted on 2026-08-02 as dead code
// and is restored. The census behind the deletion measured the fresh,
// over-64-KiB, incremental and pinned routes and never measured
// Parser.SetIncludedRanges. On that route the member fires and moves the
// published root toward this oracle: without it the root is `ERROR`, with it
// the root is `source_file`, and the oracle publishes `source_file`.
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
// Kotlin C oracle on the included-ranges route, and pins the route's known
// span and error divergences so a change surfaces instead of passing silently.
//
// The root kind is a hard parity assertion: it is the value the restored arm
// member owns. The root span and the root error flag still diverge, because the
// route drops the third included range through an unrelated defect
// (bytesAreParserPadding scans raw bytes between ranges with no clipping). Those
// two are pinned, not asserted equal. When the clipping fix lands this test
// fails and the pins move to the oracle values.
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

	// The regression guard. The restored arm member owns this result. Delete the
	// member and gotreesitter publishes `ERROR` here.
	if got, want := goRoot.Type(goLang), cRoot.Kind(); got != want {
		t.Fatalf("included-ranges root kind: gotreesitter=%q C=%q", got, want)
	}
	if goRoot.IsError() {
		t.Fatal("included-ranges root is an ERROR node while C publishes a named root")
	}
	if got, want := goRoot.StartByte(), uint32(cRoot.StartByte()); got != want {
		t.Errorf("included-ranges root start byte: gotreesitter=%d C=%d", got, want)
	}

	// Pinned pre-existing divergences, both owned by the unclipped padding scan
	// and not by the restored member. Neither value is asserted as correct; the
	// pins make any change visible.
	const (
		pinnedGoEnd     = uint32(1986)
		pinnedCEnd      = uint32(3094)
		pinnedCChildren = 8
	)
	if goRoot.EndByte() != pinnedGoEnd || uint32(cRoot.EndByte()) != pinnedCEnd {
		t.Errorf("included-ranges root end byte moved: gotreesitter=%d C=%d, pinned gotreesitter=%d C=%d",
			goRoot.EndByte(), cRoot.EndByte(), pinnedGoEnd, pinnedCEnd)
	}
	if cRoot.HasError() != false || goRoot.HasError() != true {
		t.Errorf("included-ranges root HasError moved: C=%v gotreesitter=%v, pinned C=false gotreesitter=true",
			cRoot.HasError(), goRoot.HasError())
	}
	if got := int(cRoot.ChildCount()); got != pinnedCChildren {
		t.Errorf("C oracle root child count = %d, pinned %d", got, pinnedCChildren)
	}
}
