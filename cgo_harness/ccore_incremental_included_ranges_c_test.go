//go:build cgo && treesitter_c_parity

package cgoharness

import (
	"slices"
	"strings"
	"testing"

	"github.com/odvcencio/gotreesitter/grammars"
	"github.com/odvcencio/gotreesitter/internal/ccore"
	"github.com/odvcencio/gotreesitter/internal/ccore/bind"
	sitter "github.com/tree-sitter/go-tree-sitter"
)

func TestCcoreIncrementalIncludedRangesJSONMatchesPinnedC(t *testing.T) {
	cfg, err := bind.Config(grammars.JsonLanguage())
	if err != nil {
		t.Fatal(err)
	}
	cLanguage, err := COracleLanguage("json")
	if err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		name, original, updated string
		oldSpans, newSpans      [][2]int
		edit                    *sitter.InputEdit
		mustReject              string
		mustReuse               bool
	}{
		{
			name: "altered number lookahead", original: "1x 2",
			oldSpans: [][2]int{{0, 1}, {2, 4}}, newSpans: [][2]int{{0, 4}},
			mustReject: "cant_reuse_node_contains_different_included_range tree:number",
		},
		{
			name: "remove suffix after a reusable array member", original: "[1, 2, 3]",
			oldSpans: [][2]int{{0, 9}}, newSpans: [][2]int{{0, 5}, {8, 9}},
			mustReuse: true,
		},
		{
			name: "enable full input after a ranged parse", original: "[1, 2]",
			oldSpans: [][2]int{{0, 2}, {5, 6}},
		},
		{
			name: "range formerly implicit", original: "[1, 2]",
			newSpans: [][2]int{{0, 2}, {5, 6}},
		},
		{
			name: "edit advances both range points", original: "x[12,y2]z", updated: "x[\n12,y2]z",
			oldSpans: [][2]int{{1, 5}, {6, 8}}, newSpans: [][2]int{{1, 6}, {7, 9}},
			edit: &sitter.InputEdit{
				StartByte: 2, OldEndByte: 2, NewEndByte: 3,
				StartPosition: sitter.Point{Column: 2}, OldEndPosition: sitter.Point{Column: 2},
				NewEndPosition: sitter.Point{Row: 1},
			},
			mustReuse: true,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			core, err := ccore.NewParser(cfg)
			if err != nil {
				t.Fatal(err)
			}
			core.SetStopAfter(1024, false)
			core.SetShortcut(true)
			cp := sitter.NewParser()
			defer cp.Close()
			if err := cp.SetLanguage(cLanguage); err != nil {
				t.Fatal(err)
			}

			source := []byte(test.original)
			oldRanges, cOldRanges := coreAndCIncludedRanges(source, test.oldSpans)
			old, stop := core.ParseIncluded(source, oldRanges)
			if old == nil || stop != "" {
				t.Fatalf("old ccore parse: %q", stop)
			}
			defer old.Release()
			if err := cp.SetIncludedRanges(cOldRanges); err != nil {
				t.Fatal(err)
			}
			ct := cp.Parse(source, nil)
			if ct == nil {
				t.Fatal("pinned C returned no old tree")
			}
			defer ct.Close()
			assertIncludedTreeMatchesC(t, core, old, ct)
			if len(oldRanges) != 0 {
				assertIncludedRangesEqual(t, old.IncludedRanges(), ct.IncludedRanges())
			}

			if test.edit != nil {
				edited := old.Copy()
				defer edited.Release()
				copyC := ct.Clone()
				defer copyC.Close()
				edit := test.edit
				edited.Edit(ccore.Edit{
					StartByte: uint32(edit.StartByte), OldEndByte: uint32(edit.OldEndByte), NewEndByte: uint32(edit.NewEndByte),
					StartRow: uint32(edit.StartPosition.Row), StartCol: uint32(edit.StartPosition.Column),
					OldEndRow: uint32(edit.OldEndPosition.Row), OldEndCol: uint32(edit.OldEndPosition.Column),
					NewEndRow: uint32(edit.NewEndPosition.Row), NewEndCol: uint32(edit.NewEndPosition.Column),
				})
				copyC.Edit(edit)
				assertIncludedRangesEqual(t, edited.IncludedRanges(), copyC.IncludedRanges())
				if !slices.Equal(oldRanges, old.IncludedRanges()) {
					t.Fatal("editing a copy changed the original ranges")
				}
				old, ct, source = edited, copyC, []byte(test.updated)
			}

			newRanges, cNewRanges := coreAndCIncludedRanges(source, test.newSpans)
			if err := cp.SetIncludedRanges(cNewRanges); err != nil {
				t.Fatal(err)
			}
			var goTrace, cTrace []string
			core.SetLogger(func(line string) { goTrace = append(goTrace, line) })
			cp.SetLogger(func(kind sitter.LogType, line string) {
				if kind == sitter.LogTypeParse {
					cTrace = append(cTrace, line)
				}
			})
			newC := cp.Parse(source, ct)
			if newC == nil {
				t.Fatal("pinned C returned no edited tree")
			}
			defer newC.Close()
			next, stop := core.ParseWithIncluded(source, old, newRanges)
			if next == nil || stop != "" {
				t.Fatalf("incremental ccore parse: %q", stop)
			}
			defer next.Release()
			assertIncludedTreeMatchesC(t, core, next, newC)
			goReuse, _, _ := core.ReuseStats()
			if core.ShortcutTaken() {
				t.Fatal("ranged parse unexpectedly used the contiguous shortcut")
			}
			if test.mustReuse && goReuse == 0 {
				t.Fatalf("expected nonzero reuse, trace=%q", goTrace)
			}
			if test.mustReject != "" {
				for _, trace := range [][]string{goTrace, cTrace} {
					if !strings.Contains(strings.Join(trace, "\n"), test.mustReject) {
						t.Fatalf("expected lookahead rejection %q, trace=%q", test.mustReject, trace)
					}
				}
			}

			changes, ok := ccore.ChangedRanges(old, next, cfg.Tables, old.IncludedRanges(), next.IncludedRanges())
			if !ok {
				t.Fatal("ccore changed ranges declined")
			}
			assertIncludedRangesEqual(t, changes, ct.ChangedRanges(newC))

			core.SetLogger(nil)
			cp.SetLogger(nil)
			fresh, stop := core.ParseIncluded(source, newRanges)
			if fresh == nil || stop != "" {
				t.Fatalf("fresh ccore parse: %q", stop)
			}
			defer fresh.Release()
			freshC := cp.Parse(source, nil)
			if freshC == nil {
				t.Fatal("pinned C returned no fresh tree")
			}
			defer freshC.Close()
			assertIncludedTreeMatchesC(t, core, fresh, freshC)
			if core.Digest(next) != core.Digest(fresh) {
				t.Fatal("incremental included-range tree differs from fresh")
			}
		})
	}
}

func coreAndCIncludedRanges(source []byte, spans [][2]int) ([]ccore.Range, []sitter.Range) {
	var ranges []ccore.Range
	var cRanges []sitter.Range
	for _, span := range spans {
		sr, sc := includedRangesPointAt(source, span[0])
		er, ec := includedRangesPointAt(source, span[1])
		ranges = append(ranges, ccore.Range{
			StartByte: uint32(span[0]), EndByte: uint32(span[1]),
			StartRow: uint32(sr), StartCol: uint32(sc), EndRow: uint32(er), EndCol: uint32(ec),
		})
		cRanges = append(cRanges, sitter.Range{
			StartByte: uint(span[0]), EndByte: uint(span[1]),
			StartPoint: sitter.Point{Row: sr, Column: sc}, EndPoint: sitter.Point{Row: er, Column: ec},
		})
	}
	return ranges, cRanges
}

func assertIncludedRangesEqual(t *testing.T, got []ccore.Range, want []sitter.Range) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("range count %d vs %d, ccore=%+v C=%+v", len(got), len(want), got, want)
	}
	for i, r := range want {
		expected := ccore.Range{StartByte: uint32(r.StartByte), EndByte: uint32(r.EndByte),
			StartRow: uint32(r.StartPoint.Row), StartCol: uint32(r.StartPoint.Column),
			EndRow: uint32(r.EndPoint.Row), EndCol: uint32(r.EndPoint.Column)}
		if got[i] != expected {
			t.Fatalf("range %d ccore=%+v C=%+v", i, got[i], expected)
		}
	}
}
