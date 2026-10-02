//go:build cgo && treesitter_c_parity

package cgoharness

import (
	"testing"

	ts "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
	sitter "github.com/tree-sitter/go-tree-sitter"
)

func TestPerlDeclaredShiftEditsMatchFreshAndLockedC(t *testing.T) {
	lang := grammars.DetectLanguageByName("perl").Language()
	cl, err := COracleLanguage("perl")
	if err != nil {
		t.Fatal(err)
	}
	cp := sitter.NewParser()
	defer cp.Close()
	if err := cp.SetLanguage(cl); err != nil {
		t.Fatal(err)
	}
	for _, source := range []string{"{l$s, }", "{(p())if@;}", "{a(())if@_}"} {
		t.Run(source, func(t *testing.T) {
			for _, candidate := range []bool{false, true} {
				p := ts.NewParser(lang)
				p.SetAdmissionCandidateRoute(candidate)
				before := []byte(source)
				old, err := p.Parse(before)
				if err != nil {
					t.Fatal(err)
				}
				defer old.Release()
				pos := uint32(3)
				if source == "{(p())if@;}" {
					pos = 2
				} else if source == "{a(())if@_}" {
					pos = 1
				}
				after := append([]byte(nil), before...)
				after[pos] = 'q'
				old.Edit(ts.InputEdit{StartByte: pos, OldEndByte: pos + 1, NewEndByte: pos + 1, StartPoint: ts.Point{Column: pos}, OldEndPoint: ts.Point{Column: pos + 1}, NewEndPoint: ts.Point{Column: pos + 1}})
				inc, err := p.ParseIncremental(after, old)
				if err != nil {
					t.Fatal(err)
				}
				defer inc.Release()
				freshParser := ts.NewParser(lang)
				freshParser.SetAdmissionCandidateRoute(candidate)
				fresh, err := freshParser.Parse(after)
				if err != nil {
					t.Fatal(err)
				}
				defer fresh.Release()
				ct := cp.Parse(after, nil)
				if ct == nil {
					t.Fatal("nil C tree")
				}
				defer ct.Close()
				if ct.RootNode().HasError() {
					t.Fatal("edited control has a C error")
				}
				if diff := FirstDivergenceDumpV1(fresh.RootNode(), lang, ct.RootNode()); diff != nil {
					t.Fatalf("candidate=%t fresh vs C: %+v", candidate, diff)
				}
				if diff := FirstDivergenceDumpV1(inc.RootNode(), lang, ct.RootNode()); diff != nil {
					t.Fatalf("candidate=%t incremental vs fresh/C: %+v", candidate, diff)
				}
				allocs := testing.AllocsPerRun(10, func() {
					same, err := p.ParseIncremental(after, inc)
					if err != nil {
						panic(err)
					}
					same.Release()
				})
				if allocs != 0 {
					t.Fatalf("candidate=%t no-edit allocations=%g", candidate, allocs)
				}
			}
		})
	}
}
