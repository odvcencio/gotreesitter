//go:build cgo && treesitter_c_parity && gts_ccore_test_certify

package cgoharness

import (
	"testing"

	g "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
	"github.com/odvcencio/gotreesitter/internal/ccore/bind"
	"github.com/odvcencio/gotreesitter/internal/ccore/bind/bindtest"
	"github.com/odvcencio/gotreesitter/internal/ccore/certified"
	sitter "github.com/tree-sitter/go-tree-sitter"
)

func TestCcoreIncludedRangesPublicRoutes(t *testing.T) {
	lang := grammars.JsonLanguage()
	src := []byte("xx[1, 2, 3]yy")
	goRanges := func(spans [][2]int) []g.Range {
		ranges := make([]g.Range, len(spans))
		for i, span := range spans {
			ranges[i] = g.Range{StartByte: uint32(span[0]), EndByte: uint32(span[1]),
				StartPoint: g.Point{Column: uint32(span[0])}, EndPoint: g.Point{Column: uint32(span[1])}}
		}
		return ranges
	}
	before := [][2]int{{2, 11}}
	after := [][2]int{{2, 7}, {10, 11}}
	parser := g.NewParser(lang)
	parser.SetAdmissionCandidateRoute(true)

	parser.SetIncludedRanges(goRanges(before))
	_, fresh0, incr0 := bind.RouteCounters()
	ungranted, err := parser.Parse(src)
	if err != nil || ungranted == nil {
		t.Fatalf("ungranted parse: %v", err)
	}
	ungranted.Release()
	_, fresh1, incr1 := bind.RouteCounters()
	if fresh1 != fresh0 || incr1 != incr0 {
		t.Fatal("the core served ranges without a range certificate")
	}

	restore, err := bindtest.CertifyForTests(certified.Fresh|certified.Recover|certified.Incr|certified.Ranges, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer restore()
	cp := sitter.NewParser()
	defer cp.Close()
	cl, err := COracleLanguage("json")
	if err != nil {
		t.Fatal(err)
	}
	if err := cp.SetLanguage(cl); err != nil {
		t.Fatal(err)
	}
	setC := func(spans [][2]int) {
		t.Helper()
		_, ranges := coreAndCIncludedRanges(src, spans)
		if err := cp.SetIncludedRanges(ranges); err != nil {
			t.Fatal(err)
		}
	}
	setC(before)
	cOld := cp.Parse(src, nil)
	if cOld == nil {
		t.Fatal("pinned C returned no initial tree")
	}
	defer cOld.Close()
	old, err := parser.Parse(src)
	if err != nil || old == nil {
		t.Fatalf("certified fresh parse: %v", err)
	}
	defer old.Release()
	assertLockedCTreeExact(t, "ranged fresh", old, lang, cOld)
	if rt := old.ParseRuntime(); rt.ExpectedEOFByte != 11 || rt.RootEndByte != 11 {
		t.Fatalf("fresh range extent: %+v", rt)
	}

	setC(after)
	cNext := cp.Parse(src, cOld)
	if cNext == nil {
		t.Fatal("pinned C returned no incremental tree")
	}
	defer cNext.Close()
	parser.SetIncludedRanges(goRanges(after))
	next, err := parser.ParseIncremental(src, old)
	if err != nil || next == nil {
		t.Fatalf("certified incremental parse: %v", err)
	}
	defer next.Release()
	assertLockedCTreeExact(t, "range-only reparse", next, lang, cNext)
	_, fresh2, incr2 := bind.RouteCounters()
	if fresh2-fresh1 != 1 || incr2-incr1 != 1 {
		t.Fatalf("ccore served fresh=%d incremental=%d, want 1 each", fresh2-fresh1, incr2-incr1)
	}
	if rt := next.ParseRuntime(); !rt.IncrementalOldTreeReuseRoute || rt.ExpectedEOFByte != 11 {
		t.Fatalf("incremental range attribution: %+v", rt)
	}
}
