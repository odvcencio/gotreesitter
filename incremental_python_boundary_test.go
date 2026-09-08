package gotreesitter_test

import (
	"fmt"
	"os"
	"testing"

	gts "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
)

func TestPythonIncrementalElseKeywordEdits(t *testing.T) {
	source, err := os.ReadFile("testdata/incremental_gate/python_setup.py")
	if err != nil {
		t.Fatal(err)
	}
	lang := grammars.PythonLanguage()
	for _, compact := range []bool{false, true} {
		for _, tc := range []struct {
			pos   int
			class string
		}{{1241, "delete"}, {1241, "replace"}} {
			t.Run(fmt.Sprintf("compact_%t/%d_%s", compact, tc.pos, tc.class), func(t *testing.T) {
				p := gts.NewParser(lang)
				p.SetAdmissionCandidateRoute(compact)
				old, err := p.Parse(source)
				if err != nil {
					t.Fatal(err)
				}
				defer old.Release()
				edited, edit := incrGateBuildEdit(source, tc.pos, tc.class)
				old.Edit(edit)
				incr, err := p.ParseIncremental(edited, old)
				if err != nil {
					t.Fatal(err)
				}
				defer incr.Release()
				freshParser := gts.NewParser(lang)
				freshParser.SetAdmissionCandidateRoute(compact)
				fresh, err := freshParser.Parse(edited)
				if err != nil {
					t.Fatal(err)
				}
				defer fresh.Release()
				if !fresh.RootNode().HasError() || !incr.RootNode().HasError() {
					t.Fatal("malformed else keyword lost its error signal")
				}
				if d := incrGateFirstDivergence(lang, fresh.RootNode(), incr.RootNode(), nil); d != nil {
					t.Fatalf("divergence=%+v", d)
				}
			})
		}
	}
}
