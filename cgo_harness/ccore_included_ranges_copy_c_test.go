//go:build cgo && treesitter_c_parity && gts_ccore_test_certify

package cgoharness

import (
	"fmt"
	"testing"

	g "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
	"github.com/odvcencio/gotreesitter/internal/ccore/bind"
	"github.com/odvcencio/gotreesitter/internal/ccore/bind/bindtest"
	"github.com/odvcencio/gotreesitter/internal/ccore/certified"
)

func TestCcoreCopyRetainsRangeAdmissionBoundary(t *testing.T) {
	lang := grammars.JsonLanguage()
	source := []byte("[1]\n[2]\n")
	for _, views := range []bool{false, true} {
		t.Run(fmt.Sprint("views=", views), func(t *testing.T) {
			prior := bind.ViewsOn(views)
			defer bind.ViewsOn(prior)
			p := g.NewParser(lang)
			p.SetAdmissionCandidateRoute(true)
			p.SetIncludedRanges([]g.Range{{EndByte: 3, EndPoint: g.Point{Column: 3}}})
			restore, err := bindtest.CertifyForTestsProviders(
				uint8(certified.Fresh|certified.Recover|certified.Incr|certified.Ranges), nil, nil, nil,
			)
			if err != nil {
				t.Fatal(err)
			}
			_, fresh, _ := bind.RouteCounters()
			old, err := p.Parse(source)
			restore()
			if err != nil || old == nil {
				t.Fatalf("range parse: %v", err)
			}
			defer old.Release()
			if _, served, _ := bind.RouteCounters(); served-fresh != 1 || old.RootNode().EndByte() != 3 {
				t.Fatal("range parse did not use ccore or approached the wrong EOF")
			}
			copy := old.Copy()
			defer copy.Release()
			p.SetIncludedRanges(nil)
			want, err := p.Parse(source)
			if err != nil || want == nil {
				t.Fatalf("whole-input reference: %v", err)
			}
			defer want.Release()
			if want.RootNode().EndByte() <= old.RootNode().EndByte() {
				t.Fatal("whole-input reference did not read the second value")
			}
			for _, c := range []struct {
				name string
				tree *g.Tree
			}{
				{"original", old},
				{"copy", copy},
			} {
				t.Run(c.name, func(t *testing.T) {
					_, f0, i0 := bind.RouteCounters()
					next, err := p.ParseIncremental(source, c.tree)
					if err != nil || next == nil {
						t.Fatalf("whole-input parse: %v", err)
					}
					defer next.Release()
					_, f1, i1 := bind.RouteCounters()
					if f1-f0 != 1 || i1-i0 != 0 {
						t.Errorf("range-to-whole parse without a range grant: fresh=%d incremental=%d, want 1/0", f1-f0, i1-i0)
					}
					if next.RootNode().EndByte() != want.RootNode().EndByte() ||
						next.RootNode().SExpr(lang) != want.RootNode().SExpr(lang) {
						t.Error("whole-input tree differs from a fresh parse")
					}
				})
			}
		})
	}
}
