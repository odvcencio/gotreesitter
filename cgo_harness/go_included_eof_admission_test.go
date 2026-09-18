//go:build cgo && treesitter_c_parity && gts_parsercorephase0 && !gts_no_parsercorephase0

package cgoharness

import (
	"os"
	"testing"

	gts "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
	sitter "github.com/tree-sitter/go-tree-sitter"
)

func TestGoCompactIncludedRangeEOFRecoveryLockedC(t *testing.T) {
	lang := grammars.GoLanguage()
	p := gts.NewParser(lang)
	p.SetAdmissionCandidateRoute(true)
	cl, err := ParityCLanguage("go")
	if err != nil {
		t.Fatal(err)
	}
	cp := sitter.NewParser()
	defer cp.Close()
	if err := cp.SetLanguage(cl); err != nil {
		t.Fatal(err)
	}
	recovery, err := os.ReadFile("../testdata/included_ranges/go_two_fences.go")
	if err != nil {
		t.Fatal(err)
	}
	parent := t
	var retained []struct {
		name   string
		tree   *gts.Tree
		oracle *sitter.Tree
	}
	for _, tc := range []struct {
		name, source string
		spans        [][2]int
	}{
		{"recovery", string(recovery), [][2]int{{0, 150}, {203, 250}}},
		{"included_eof", "package p\nfunc f(){x=y!", [][2]int{{0, 22}}},
		{"physical_eof", "package p\nfunc f(){x=y", nil},
		{"reset", "package q\n", nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			source := []byte(tc.source)
			var gr []gts.Range
			var cr []sitter.Range
			for _, span := range tc.spans {
				a, b := pointAtOffset(source, span[0]), pointAtOffset(source, span[1])
				gr = append(gr, gts.Range{StartByte: uint32(span[0]), EndByte: uint32(span[1]), StartPoint: a, EndPoint: b})
				cr = append(cr, sitter.Range{StartByte: uint(span[0]), EndByte: uint(span[1]), StartPoint: sitter.Point{Row: uint(a.Row), Column: uint(a.Column)}, EndPoint: sitter.Point{Row: uint(b.Row), Column: uint(b.Column)}})
			}
			p.SetIncludedRanges(gr)
			if err := cp.SetIncludedRanges(cr); err != nil {
				t.Fatal(err)
			}
			routedBefore, fallbackBefore := gts.AdmissionCandidateCounters()
			tree, err := p.Parse(source)
			if err != nil {
				t.Fatal(err)
			}
			parent.Cleanup(tree.Release)
			oracle := cp.Parse(source, nil)
			if oracle == nil {
				t.Fatal("C tree is nil")
			}
			parent.Cleanup(oracle.Close)
			retained = append(retained, struct {
				name   string
				tree   *gts.Tree
				oracle *sitter.Tree
			}{tc.name, tree, oracle})
			assertG18LockedCExact(t, tc.name, tree, lang, oracle)
			routed, fallback := gts.AdmissionCandidateCounters()
			if routed != routedBefore+1 || fallback != fallbackBefore {
				t.Fatalf("included ranges fell back: %s", gts.AdmissionCandidateLastFallbackReason())
			}
		})
	}
	for _, result := range retained {
		assertG18LockedCExact(t, "retained "+result.name, result.tree, lang, result.oracle)
	}
}
