//go:build cgo && treesitter_c_parity

package cgoharness

// C-oracle coverage for the included-ranges route. No test covered
// SetIncludedRanges for any language before this one, which is why a proposed
// deletion of the Go arm's normalizeGoSourceFileRoot member passed a full green
// suite while it turned this route's root from `source_file` into `ERROR`.
//
// Production reaches the route through injection: injection.go calls
// childParser.SetIncludedRanges for every injected child, and
// grammars/markdown_injection_register.go maps the `go` and `golang` fence
// languages to the Go grammar.
//
// Run: GTS_PARITY_ALLOW_HOST=1 go test ./cgo_harness -tags treesitter_c_parity \
//        -run TestIncludedRangesGoRootParity -v

import (
	"os"
	"path/filepath"
	"testing"

	gts "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
	sitter "github.com/tree-sitter/go-tree-sitter"
)

// includedRangesGoSpans matches the spans pinned by the non-cgo gate in
// parser_included_ranges_go_root_test.go. Both cut mid-content, the way an
// injection child receives its fences.
var includedRangesGoSpans = [2][2]int{{0, 150}, {203, 276}}

func includedRangesPointAt(src []byte, off int) (uint, uint) {
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

// TestIncludedRangesGoRootParity pins the root symbol, the root span, and the
// root child count against the locked C oracle on the included-ranges route.
//
// The root error flag still diverges on this route: C reports HasError while
// gotreesitter reports a clean root. That divergence predates this test. The
// test pins the current value instead of asserting parity, so a change in
// either direction surfaces instead of passing silently.
func TestIncludedRangesGoRootParity(t *testing.T) {
	cLang := loadCanonicalGoCLanguage(t)
	goLang := grammars.GoLanguage()
	if goLang == nil {
		t.Skip("go grammar unavailable")
	}

	path := filepath.Join("..", "testdata", "included_ranges", "go_two_fences.go")
	src, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read included-ranges fixture: %v", err)
	}

	cRanges := make([]sitter.Range, 0, len(includedRangesGoSpans))
	goRanges := make([]gts.Range, 0, len(includedRangesGoSpans))
	for _, span := range includedRangesGoSpans {
		start, end := span[0], span[1]
		if end > len(src) {
			end = len(src)
		}
		sr, sc := includedRangesPointAt(src, start)
		er, ec := includedRangesPointAt(src, end)
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

	// The regression guard. The Go arm's normalizeGoSourceFileRoot member owns
	// this result. Deleting the member publishes `ERROR` here and diverges.
	if got, want := goRoot.Type(goLang), cRoot.Kind(); got != want {
		t.Fatalf("included-ranges root kind: gotreesitter=%q C=%q", got, want)
	}
	if goRoot.IsError() {
		t.Fatal("included-ranges root is an ERROR node while C publishes a named root")
	}
	if got, want := goRoot.StartByte(), uint32(cRoot.StartByte()); got != want {
		t.Errorf("included-ranges root start byte: gotreesitter=%d C=%d", got, want)
	}
	if got, want := goRoot.EndByte(), uint32(cRoot.EndByte()); got != want {
		t.Errorf("included-ranges root end byte: gotreesitter=%d C=%d", got, want)
	}
	if got, want := goRoot.ChildCount(), int(cRoot.ChildCount()); got != want {
		t.Errorf("included-ranges root child count: gotreesitter=%d C=%d", got, want)
	}

	// Pinned pre-existing divergence. C reports HasError on this route and
	// gotreesitter does not. Neither value is asserted as correct here; the
	// pin makes any change visible.
	if cRoot.HasError() != true || goRoot.HasError() != false {
		t.Errorf("included-ranges root HasError moved: C=%v gotreesitter=%v, pinned C=true gotreesitter=false",
			cRoot.HasError(), goRoot.HasError())
	}
}
