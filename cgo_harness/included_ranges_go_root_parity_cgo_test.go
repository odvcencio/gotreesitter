//go:build cgo && treesitter_c_parity

package cgoharness

// Compare injected Go ranges with the locked C tree.
// Keep the historical geometries that exposed root normalization defects.

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	gts "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
	sitter "github.com/tree-sitter/go-tree-sitter"
)

func TestIncludedRangesGoIncrementalLockedC(t *testing.T) {
	for _, mode := range []string{"leaf_edit", "prefix_insert", "range_change"} {
		for _, compact := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/compact=%t", mode, compact), func(t *testing.T) {
				source := []byte("// host\npackage first\nvar a = 1\n// gap\npackage second\nvar b = 2\n")
				start := bytes.Index(source, []byte("package first"))
				end := bytes.Index(source, []byte("// gap"))
				language := grammars.GoLanguage()
				parser := gts.NewParser(language)
				parser.SetAdmissionCandidateRoute(compact)
				oracle := sitter.NewParser()
				defer oracle.Close()
				if err := oracle.SetLanguage(loadCanonicalGoCLanguage(t)); err != nil {
					t.Fatal(err)
				}
				setRanges := func(data []byte, first, last int) {
					sp, ep := pointAtOffset(data, first), pointAtOffset(data, last)
					parser.SetIncludedRanges([]gts.Range{{StartByte: uint32(first), EndByte: uint32(last), StartPoint: sp, EndPoint: ep}})
					if err := oracle.SetIncludedRanges([]sitter.Range{{StartByte: uint(first), EndByte: uint(last), StartPoint: sitter.Point{Row: uint(sp.Row), Column: uint(sp.Column)}, EndPoint: sitter.Point{Row: uint(ep.Row), Column: uint(ep.Column)}}}); err != nil {
						t.Fatal(err)
					}
				}
				setRanges(source, start, end)
				old, err := parser.Parse(source)
				if err != nil || old == nil {
					t.Fatalf("old parse: %v", err)
				}
				defer old.Release()
				cOld := oracle.Parse(source, nil)
				if cOld == nil {
					t.Fatal("C returned no old tree")
				}
				defer cOld.Close()
				assertG18LockedCExact(t, "old included tree", old, language, cOld)
				edited := append([]byte(nil), source...)
				if mode == "range_change" {
					start, end = bytes.Index(source, []byte("package second")), len(source)
				} else {
					at, oldEnd, replacement := bytes.Index(source, []byte("1\n")), 0, "3"
					oldEnd = at + 1
					if mode == "prefix_insert" {
						at, oldEnd, replacement = 0, 0, "// added\n"
						start += len(replacement)
						end += len(replacement)
					}
					edited = append(append(append([]byte(nil), source[:at]...), replacement...), source[oldEnd:]...)
					edit := gts.InputEdit{StartByte: uint32(at), OldEndByte: uint32(oldEnd), NewEndByte: uint32(at + len(replacement)), StartPoint: pointAtOffset(source, at), OldEndPoint: pointAtOffset(source, oldEnd), NewEndPoint: pointAtOffset(edited, at+len(replacement))}
					old.Edit(edit)
					cEdit := realCorpusCInputEdit(edit)
					cOld.Edit(&cEdit)
				}
				setRanges(edited, start, end)
				next, profile, err := parser.ParseIncrementalProfiled(edited, old)
				if next != nil && next != old {
					defer next.Release()
				}
				if err != nil || next == nil {
					t.Fatalf("incremental parse: %v", err)
				}
				cNext := oracle.Parse(edited, cOld)
				if cNext == nil {
					t.Fatal("C returned no incremental tree")
				}
				defer cNext.Close()
				cFresh := oracle.Parse(edited, nil)
				if cFresh == nil {
					t.Fatal("C returned no fresh tree")
				}
				defer cFresh.Close()
				assertG18LockedCExact(t, "incremental C", next, language, cNext)
				assertG18LockedCExact(t, "fresh C", next, language, cFresh)
				if mode != "range_change" && (profile.ReuseUnsupported || profile.ReusedSubtrees == 0) {
					t.Fatalf("stable ranges lost reuse: %+v", profile)
				}
				if mode == "range_change" && (!profile.ReuseUnsupported || profile.ReuseUnsupportedReason != "old_tree_included_ranges_changed") {
					t.Fatalf("range change attribution: %+v", profile)
				}
			})
		}
	}
}

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

// includedRangesRootObservation is one measured root, pinned exactly.
type includedRangesRootObservation struct {
	Kind       string
	StartByte  uint32
	EndByte    uint32
	ChildCount int
	HasError   bool
}

type includedRangesGeometry struct {
	name  string
	spans [2][2]int
	c     includedRangesRootObservation
}

// Keep exact C root observations as well as complete tree comparisons.
var includedRangesGoGeometries = []includedRangesGeometry{
	{"interior_anchors", [2][2]int{{26, 150}, {203, 276}}, includedRangesRootObservation{"source_file", 26, 276, 7, true}},
	{"anchored_at_zero_and_eof", [2][2]int{{0, 150}, {203, 276}}, includedRangesRootObservation{"source_file", 0, 276, 7, true}},
	{"trimmed_tail", [2][2]int{{0, 150}, {203, 250}}, includedRangesRootObservation{"source_file", 0, 250, 6, true}},
	{"non_whitespace_gap", [2][2]int{{40, 150}, {203, 260}}, includedRangesRootObservation{"source_file", 40, 260, 5, true}},
}

func measureIncludedRangesRoots(
	t *testing.T,
	src []byte,
	cLang *sitter.Language,
	goLang *gts.Language,
	spans [2][2]int,
) (includedRangesRootObservation, includedRangesRootObservation, uint64) {
	t.Helper()
	cRanges := make([]sitter.Range, 0, len(spans))
	goRanges := make([]gts.Range, 0, len(spans))
	for _, span := range spans {
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
	routedBefore, fallbackBefore := gts.AdmissionCandidateCounters()
	goTree, err := goParser.Parse(src)
	if err != nil {
		t.Fatalf("go parse: %v", err)
	}
	if goTree == nil || goTree.RootNode() == nil {
		t.Fatal("go parse returned no tree")
	}
	defer goTree.Release()
	goRoot := goTree.RootNode()
	if diff := g18LockedCExactError(goTree, goLang, cTree); diff != nil {
		routedAfter, fallbackAfter := gts.AdmissionCandidateCounters()
		t.Logf("route=%d/%d reason=%q", routedAfter-routedBefore, fallbackAfter-fallbackBefore, gts.AdmissionCandidateLastFallbackReason())
		rt := goTree.ParseRuntime()
		t.Logf("stop=%s recovered=%t dropped=%t checked=%t", rt.StopReason, rt.CRecoveryEnteredErrorState, rt.CRecoveryDroppedErrorForClean, rt.CRecoverySwallowedErrorFallbackAttempted)
		for i := 0; i < goRoot.ChildCount(); i++ {
			child := goRoot.Child(i)
			t.Logf("Go child %d: %s [%d,%d]", i, child.SExpr(goLang), child.StartByte(), child.EndByte())
		}
		for i := uint(0); i < cRoot.ChildCount(); i++ {
			child := cRoot.Child(i)
			t.Logf("C child %d: %s [%d,%d]", i, child.ToSexp(), child.StartByte(), child.EndByte())
		}
	}
	assertG18LockedCExact(t, "included Go ranges", goTree, goLang, cTree)

	var armRewrites uint64
	if passes := goTree.ParseRuntime().NormalizationPasses; passes != nil {
		for _, pass := range *passes {
			if pass.Name == "dispatch.go.source-file-root" {
				armRewrites = pass.NodesRewritten
			}
		}
	}

	return includedRangesRootObservation{
			Kind:       cRoot.Kind(),
			StartByte:  uint32(cRoot.StartByte()),
			EndByte:    uint32(cRoot.EndByte()),
			ChildCount: int(cRoot.ChildCount()),
			HasError:   cRoot.HasError(),
		}, includedRangesRootObservation{
			Kind:       goRoot.Type(goLang),
			StartByte:  goRoot.StartByte(),
			EndByte:    goRoot.EndByte(),
			ChildCount: goRoot.ChildCount(),
			HasError:   goRoot.HasError(),
		}, armRewrites
}

func loadIncludedRangesGoFixture(t *testing.T) []byte {
	t.Helper()
	path := filepath.Join("..", "testdata", "included_ranges", "go_two_fences.go")
	src, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read included-ranges fixture: %v", err)
	}
	return src
}

// TestIncludedRangesGoRootParity checks complete trees and pinned C root observations.
func TestIncludedRangesGoRootParity(t *testing.T) {
	cLang := loadCanonicalGoCLanguage(t)
	goLang := grammars.GoLanguage()
	if goLang == nil {
		t.Skip("go grammar unavailable")
	}
	src := loadIncludedRangesGoFixture(t)

	for _, geometry := range includedRangesGoGeometries {
		geometry := geometry
		t.Run(geometry.name, func(t *testing.T) {
			t.Setenv("GTS_DISPATCHER_CENSUS", "1")
			gotC, gotGo, armRewrites := measureIncludedRangesRoots(t, src, cLang, goLang, geometry.spans)

			t.Logf("C   %+v", gotC)
			t.Logf("GTS %+v", gotGo)
			t.Logf("dispatch.go.source-file-root rewrote %d nodes", armRewrites)

			if gotC != geometry.c {
				t.Errorf("C oracle root moved: got %+v, pinned %+v", gotC, geometry.c)
			}
			if gotGo != gotC {
				t.Errorf("root differs from C: got %+v, C %+v", gotGo, gotC)
			}
			if armRewrites != 0 {
				t.Errorf("root normalization rewrote %d nodes, want zero", armRewrites)
			}
		})
	}
}

// TestIncludedRangesGoArmGuardsRootSymbol requires root parity without normalization repairs.
func TestIncludedRangesGoArmGuardsRootSymbol(t *testing.T) {
	cLang := loadCanonicalGoCLanguage(t)
	goLang := grammars.GoLanguage()
	if goLang == nil {
		t.Skip("go grammar unavailable")
	}
	src := loadIncludedRangesGoFixture(t)

	for _, geometry := range includedRangesGoGeometries {
		geometry := geometry
		t.Run(geometry.name, func(t *testing.T) {
			t.Setenv("GTS_DISPATCHER_CENSUS", "1")
			gotC, gotGo, armRewrites := measureIncludedRangesRoots(t, src, cLang, goLang, geometry.spans)
			if armRewrites != 0 {
				t.Fatalf("dispatch.go.source-file-root rewrote %d nodes; want 0 now that the padding scan clips to included ranges", armRewrites)
			}
			if gotGo.Kind != gotC.Kind {
				t.Fatalf("root symbol diverged from C: gotreesitter=%q C=%q", gotGo.Kind, gotC.Kind)
			}
			if gotGo.Kind == "ERROR" {
				t.Fatalf("root is an ERROR node while C publishes %q", gotC.Kind)
			}
		})
	}
}
