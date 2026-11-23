//go:build cgo && treesitter_c_parity

package cgoharness

import (
	"os"
	"slices"
	"testing"

	g "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/internal/ccore/bind"
	sitter "github.com/tree-sitter/go-tree-sitter"
)

func TestPublicCcoreChangedRangesAgainstC(t *testing.T) {
	langName := os.Getenv("CCORE_LANG")
	var oldSource, newSource string
	switch langName {
	case "diff":
		const prefix = "diff --git a/main.go b/main.go\n--- a/main.go\n+++ b/main.go\n"
		const first = "@@ -1 +1 @@\n-before\n+after\n"
		const second = "@@ -8 +8 @@\n-previous\n+next\n"
		const inserted = "@@ -4 +4 @@\n-small\n+slightly longer\n"
		oldSource, newSource = prefix+first+second, prefix+first+inserted+second
	case "json":
		oldSource, newSource = "[0,\n  2,\n  3]\n", "[0,\n  {\"x\": 1},\n  2,\n  3]\n"
	case "css":
		oldSource, newSource = "a { color: red; }\n", "a { color: blue; }\n"
	default:
		t.Skip("set CCORE_LANG to diff, json, or css")
	}
	lang, ref := ccoreLanguages(t, langName)
	oracle := sitter.NewParser()
	defer oracle.Close()
	if err := oracle.SetLanguage(ref); err != nil {
		t.Fatal(err)
	}
	p := g.NewParser(lang)
	oldBytes, newBytes := []byte(oldSource), []byte(newSource)
	_, freshBefore, _ := bind.RouteCounters()
	old, err := p.Parse(oldBytes)
	if err != nil || old == nil {
		t.Fatalf("old parse: %v", err)
	}
	defer old.Release()
	if _, freshAfter, _ := bind.RouteCounters(); freshAfter-freshBefore != 1 {
		t.Fatal("original parse did not use ccore")
	}
	oldC := oracle.Parse(oldBytes, nil)
	if oldC == nil {
		t.Fatal("old C parse returned nil")
	}
	defer oldC.Close()

	// Hold a view below the root to exercise edits after a caller has read nodes.
	if old.RootNode().Child(0) == nil {
		t.Fatal("missing first child")
	}
	e := ccoreDiffEdit(oldBytes, newBytes)
	old.Edit(g.InputEdit{
		StartByte: e.StartByte, OldEndByte: e.OldEndByte, NewEndByte: e.NewEndByte,
		StartPoint: g.Point{Row: e.StartRow, Column: e.StartCol},
		OldEndPoint: g.Point{Row: e.OldEndRow, Column: e.OldEndCol},
		NewEndPoint: g.Point{Row: e.NewEndRow, Column: e.NewEndCol},
	})
	oldC.Edit(ccoreSitterEdit(e))

	for _, mode := range []string{"incremental", "fresh"} {
		t.Run(mode, func(t *testing.T) {
			var next *g.Tree
			var nextC *sitter.Tree
			var err error
			_, f0, i0 := bind.RouteCounters()
			if mode == "incremental" {
				next, err = p.ParseIncremental(newBytes, old)
				nextC = oracle.Parse(newBytes, oldC)
			} else {
				next, err = p.Parse(newBytes)
				nextC = oracle.Parse(newBytes, nil)
			}
			if err != nil || next == nil || nextC == nil {
				t.Fatalf("new parse: Go error %v, C tree %p", err, nextC)
			}
			defer next.Release()
			defer nextC.Close()
			if _, f1, i1 := bind.RouteCounters(); f1-f0+i1-i0 != 1 {
				t.Fatal("new parse did not use ccore")
			}
			var want []g.Range
			for _, r := range oldC.ChangedRanges(nextC) {
				want = append(want, g.Range{
					StartByte: uint32(r.StartByte), EndByte: uint32(r.EndByte),
					StartPoint: g.Point{Row: uint32(r.StartPoint.Row), Column: uint32(r.StartPoint.Column)},
					EndPoint: g.Point{Row: uint32(r.EndPoint.Row), Column: uint32(r.EndPoint.Column)},
				})
			}
			if langName == "css" && (len(want) != 0 || len(old.ChangedRanges()) == 0) {
				t.Fatalf("expected a text edit without a syntax change, got %v", want)
			}
			if langName != "css" && len(want) == 0 {
				t.Fatal("expected a structural change")
			}
			if got := g.DiffChangedRanges(old, next); !slices.Equal(got, want) {
				t.Errorf("Go changed ranges %v; pinned C %v", got, want)
			}
		})
	}
}
