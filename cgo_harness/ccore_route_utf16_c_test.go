//go:build cgo && treesitter_c_parity

package cgoharness

// The UTF-16 entry points served by internal/ccore, against pinned C on the
// UTF-8 bytes of the same text. The route is on and every bound language is
// certified (bind.CertifyForTests).
//
//	CCORE_ROUTE_UTF16=1 go test -run '^TestCcoreRouteUTF16$'

import (
	"fmt"
	"os"
	"testing"
	"unicode/utf16"

	g "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
	"github.com/odvcencio/gotreesitter/internal/ccore/bind"
	sitter "github.com/tree-sitter/go-tree-sitter"
)

func ccoreU16Bytes(units []uint16, bigEndian bool) []byte {
	out := make([]byte, 0, 2*len(units))
	for _, u := range units {
		if bigEndian {
			out = append(out, byte(u>>8), byte(u))
		} else {
			out = append(out, byte(u), byte(u>>8))
		}
	}
	return out
}

func TestCcoreRouteUTF16(t *testing.T) {
	if os.Getenv("CCORE_ROUTE_UTF16") == "" {
		t.Skip("set CCORE_ROUTE_UTF16")
	}
	defer ccoreRouteCertify(t)()
	inputs := map[string][]string{
		"json":       {"", "{\"k\": \"héllo 日本 😀\", \"v\": [1, 2]}\n"},
		"python":     {"", "x = \"héllo 日本 😀\"\n# コメント\ndef f(a):\n    return a\n"},
		"typescript": {"", "const s = \"héllo 日本 😀\"; // コメント\nfunction f(a: number) { return a; }\n"},
	}
	total, equal := 0, 0
	for name, list := range inputs {
		entry := grammars.DetectLanguageByName(name)
		lang := entry.Language()
		cLang, err := ParityCLanguage(name)
		if err != nil {
			t.Fatal(err)
		}
		cp := sitter.NewParser()
		if err := cp.SetLanguage(cLang); err != nil {
			t.Fatal(err)
		}
		r4, _ := ccoreRouteSource(t, name)
		for i, extra := range list {
			src := r4
			label := "r4"
			if extra != "" {
				src, label = []byte(extra), "non-ascii"
			}
			units := utf16.Encode([]rune(string(src)))
			ct := cp.Parse(src, nil)
			// byte offset -> code unit offset, computed from the text alone
			unitAt := func(b uint32) uint32 { return uint32(len(utf16.Encode([]rune(string(src[:b]))))) }
			for _, kind := range []string{"le", "be", "units"} {
				p := g.NewParser(lang)
				p.SetAdmissionCandidateRoute(true)
				_, f0, _ := bind.RouteCounters()
				var tree *g.Tree
				switch kind {
				case "units":
					tree, err = p.ParseUTF16(units)
				case "le":
					tree, err = p.ParseUTF16Bytes(ccoreU16Bytes(units, false), g.UTF16LittleEndian)
				case "be":
					tree, err = p.ParseUTF16Bytes(ccoreU16Bytes(units, true), g.UTF16BigEndian)
				}
				if err != nil || tree == nil {
					t.Fatalf("%s %s %s: %v", name, label, kind, err)
				}
				_, f1, _ := bind.RouteCounters()
				divs := compactT3StructuralDivergences(tree.RootNode(), lang, ct.RootNode())
				// every node's UTF-16 range
				badRange := 0
				var walk func(n *g.Node)
				walk = func(n *g.Node) {
					r, ok := tree.UTF16RangeForNode(n)
					if !ok || r.StartCodeUnit != unitAt(n.StartByte()) || r.EndCodeUnit != unitAt(n.EndByte()) {
						badRange++
					}
					for j := 0; j < n.ChildCount(); j++ {
						walk(n.Child(j))
					}
				}
				walk(tree.RootNode())
				total++
				ok := f1 > f0 && len(divs) == 0 && badRange == 0
				if ok {
					equal++
				}
				fmt.Printf("CCUTF16 lang=%s input=%s entry=%s served=%t divergences=%d bad_utf16_ranges=%d\n", name, label, kind, f1 > f0, len(divs), badRange)
				ccoreExpect(t, ok, "%s %s %s: served=%t divergences=%d bad ranges=%d", name, label, kind, f1 > f0, len(divs), badRange)
				// a UTF-16 edit and an incremental reparse, against C's incremental parse
				if kind == "units" && len(units) > 8 {
					at := uint32(len(units) / 2)
					for at > 0 && at < uint32(len(units)) && utf16.IsSurrogate(rune(units[at])) {
						at--
					}
					nu := append(append(append([]uint16(nil), units[:at]...), 'é'), units[at:]...)
					ok2 := tree.EditUTF16(g.UTF16Edit{StartCodeUnit: at, OldEndCodeUnit: at, NewEndCodeUnit: at + 1}, nu)
					if !ok2 {
						t.Fatalf("%s %s: EditUTF16 refused", name, label)
					}
					edits := tree.Edits()
					e := edits[len(edits)-1]
					nsrc := []byte(string(utf16.Decode(nu)))
					ct.Edit(&sitter.InputEdit{StartByte: uint(e.StartByte), OldEndByte: uint(e.OldEndByte), NewEndByte: uint(e.NewEndByte),
						StartPosition:  sitter.Point{Row: uint(e.StartPoint.Row), Column: uint(e.StartPoint.Column)},
						OldEndPosition: sitter.Point{Row: uint(e.OldEndPoint.Row), Column: uint(e.OldEndPoint.Column)},
						NewEndPosition: sitter.Point{Row: uint(e.NewEndPoint.Row), Column: uint(e.NewEndPoint.Column)}})
					cn := cp.Parse(nsrc, ct)
					_, i0, _ := bind.RouteCounters()
					inc, err := p.ParseIncrementalUTF16(nu, tree)
					if err != nil || inc == nil {
						t.Fatalf("%s %s: incremental: %v", name, label, err)
					}
					_, i1, _ := bind.RouteCounters()
					d2 := compactT3StructuralDivergences(inc.RootNode(), lang, cn.RootNode())
					total++
					if i1 > i0 && len(d2) == 0 {
						equal++
					}
					fmt.Printf("CCUTF16 lang=%s input=%s entry=incremental served_incr=%t divergences=%d\n", name, label, i1 > i0, len(d2))
					ccoreExpect(t, i1 > i0 && len(d2) == 0, "%s %s incremental: served=%t divergences=%d", name, label, i1 > i0, len(d2))
					inc.Release()
					cn.Close()
				}
				tree.Release()
			}
			ct.Close()
			_ = i
		}
		cp.Close()
	}
	fmt.Printf("CCUTF16TOTAL checks=%d equal_and_served=%d\n", total, equal)
}
