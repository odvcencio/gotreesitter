//go:build cgo && treesitter_c_parity

package cgoharness

// Reparses of grammars whose scanner has no incremental-reuse capability in the
// present engine, from clean and from error trees: the route serves them, and
// the result equals C's fresh parse of the same bytes.
//
//	CCORE_CENSUS=1 go test -run '^TestCcoreRouteScannerAndErrorReparses$'

import (
	"fmt"
	"os"
	"testing"

	g "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
	"github.com/odvcencio/gotreesitter/internal/ccore/bind"
)

func ccoreInsertAt(src []byte, at int, ins string) ([]byte, g.InputEdit) {
	pt := func(b []byte, n int) g.Point {
		var p g.Point
		for _, c := range b[:n] {
			if c == '\n' {
				p.Row++
				p.Column = 0
			} else {
				p.Column++
			}
		}
		return p
	}
	out := append(append(append([]byte(nil), src[:at]...), ins...), src[at:]...)
	return out, g.InputEdit{StartByte: uint32(at), OldEndByte: uint32(at), NewEndByte: uint32(at + len(ins)),
		StartPoint: pt(src, at), OldEndPoint: pt(src, at), NewEndPoint: pt(out, at+len(ins))}
}

func TestCcoreRouteScannerAndErrorReparses(t *testing.T) {
	if os.Getenv("CCORE_CENSUS") == "" {
		t.Skip("set CCORE_CENSUS")
	}
	defer ccoreRouteCertify(t)()
	total, servedAll, equalC := 0, 0, 0
	for _, name := range []string{"bash", "kotlin", "lua", "php", "ruby", "yaml", "scala", "cpp", "html"} {
		entry := grammars.DetectLanguageByName(name)
		if entry == nil {
			t.Fatalf("no language %q", name)
		}
		lang := entry.Language()
		cLang, err := ParityCLanguage(name)
		if err != nil {
			t.Fatal(err)
		}
		src, _ := ccoreRouteSource(t, name)
		p := g.NewParser(lang)
		p.SetAdmissionCandidateRoute(true)
		tree, err := p.Parse(src)
		if err != nil || tree == nil {
			t.Fatalf("%s: %v", name, err)
		}
		_, f0, i0 := bind.RouteCounters()
		steps, eq := 0, 0
		cur := src
		for _, ins := range []string{")", " "} { // the first edit usually makes an error; the second reparses from it
			at := len(cur) / 2
			for at < len(cur) && cur[at] != '\n' {
				at++
			}
			next, edit := ccoreInsertAt(cur, at, ins)
			tree.Edit(edit)
			nt, err := p.ParseIncremental(next, tree)
			if err != nil || nt == nil {
				t.Fatalf("%s: %v", name, err)
			}
			steps++
			cTree := compactT3ParseC(t, cLang, next)
			if len(compactT3StructuralDivergences(nt.RootNode(), lang, cTree.RootNode())) == 0 {
				eq++
			}
			cTree.Close()
			if nt != tree {
				tree.Release()
			}
			tree, cur = nt, next
		}
		tree.Release()
		_, f1, i1 := bind.RouteCounters()
		fmt.Printf("CCCENSUS lang=%s reparses=%d served_by_core=%d equal_to_c=%d\n", name, steps, (f1-f0)+(i1-i0), eq)
		total += steps
		servedAll += int((f1 - f0) + (i1 - i0))
		equalC += eq
	}
	fmt.Printf("CCCENSUSTOTAL reparses=%d served_by_core=%d equal_to_c=%d\n", total, servedAll, equalC)
	ccoreExpect(t, total > 0 && servedAll == total, "%d of %d reparses served by the core", servedAll, total)
	ccoreExpect(t, equalC == total, "%d of %d reparses equal C", equalC, total)
}
