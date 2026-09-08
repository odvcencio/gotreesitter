//go:build cgo && treesitter_c_parity && gts_parsercorephase0 && !gts_no_parsercorephase0

package cgoharness

import (
	"fmt"
	"os"
	"testing"

	gts "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
	sitter "github.com/tree-sitter/go-tree-sitter"
)

func TestPythonIncrementalBoundaryLockedC(t *testing.T) {
	source, err := os.ReadFile("../testdata/incremental_gate/python_setup.py")
	if err != nil {
		t.Fatal(err)
	}
	lang := grammars.PythonLanguage()
	cl, err := ParityCLanguage("python")
	if err != nil {
		t.Fatal(err)
	}
	cp := sitter.NewParser()
	defer cp.Close()
	if err := cp.SetLanguage(cl); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		pos    int
		delete bool
	}{{593, false}, {594, false}, {595, false}, {596, false}, {1241, true}, {1241, false}} {
		t.Run(fmt.Sprintf("%d_delete_%t", tc.pos, tc.delete), func(t *testing.T) {
			edited := append([]byte(nil), source...)
			end := tc.pos + 1
			if tc.delete {
				edited = append(edited[:tc.pos], edited[tc.pos+1:]...)
				end = tc.pos
			} else {
				edited[tc.pos] = 'z'
			}
			edit := gts.InputEdit{StartByte: uint32(tc.pos), OldEndByte: uint32(tc.pos + 1), NewEndByte: uint32(end), StartPoint: pointAtOffset(source, tc.pos), OldEndPoint: pointAtOffset(source, tc.pos+1), NewEndPoint: pointAtOffset(edited, end)}
			oracle := cp.Parse(edited, nil)
			if oracle == nil {
				t.Fatal("nil C tree")
			}
			defer oracle.Close()
			t.Logf("C HasError=%t", oracle.RootNode().HasError())
			for _, incremental := range []bool{false, true} {
				t.Run(fmt.Sprintf("incremental_%t", incremental), func(t *testing.T) {
					p := gts.NewParser(lang)
					p.SetAdmissionCandidateRoute(false)
					var tree *gts.Tree
					var err error
					if incremental {
						old, e := p.Parse(source)
						if e != nil {
							t.Fatal(e)
						}
						defer old.Release()
						old.Edit(edit)
						tree, err = p.ParseIncremental(edited, old)
					} else {
						tree, err = p.Parse(edited)
					}
					if err != nil {
						t.Fatal(err)
					}
					defer tree.Release()
					assertG18LockedCExact(t, "python boundary", tree, lang, oracle)
				})
			}
		})
	}
}

func TestPythonAdjacentIdentifiersLockedC(t *testing.T) {
	lang := grammars.PythonLanguage()
	cl, err := ParityCLanguage("python")
	if err != nil {
		t.Fatal(err)
	}
	cp := sitter.NewParser()
	defer cp.Close()
	if err := cp.SetLanguage(cl); err != nil {
		t.Fatal(err)
	}
	for _, source := range []string{"z return x\n", "z x\n"} {
		t.Run(source, func(t *testing.T) {
			p := gts.NewParser(lang)
			p.SetAdmissionCandidateRoute(false)
			tree, err := p.Parse([]byte(source))
			if err != nil {
				t.Fatal(err)
			}
			defer tree.Release()
			oracle := cp.Parse([]byte(source), nil)
			if oracle == nil {
				t.Fatal("nil C tree")
			}
			defer oracle.Close()
			assertG18LockedCExact(t, "adjacent identifiers", tree, lang, oracle)
		})
	}
}
