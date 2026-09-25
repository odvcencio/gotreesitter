//go:build cgo && treesitter_c_parity

package cgoharness

import (
	"bytes"
	"fmt"
	"testing"

	sitter "github.com/tree-sitter/go-tree-sitter"

	gts "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
	"github.com/odvcencio/gotreesitter/internal/benchfixtures"
)

func TestScalaIssue454AliasLockedCParity(t *testing.T) {
	var source bytes.Buffer
	source.WriteString("package demo\n\n")
	for i := 0; source.Len() < 137<<10; i++ {
		fmt.Fprintf(&source, "object O%d {\n  def f%d(a: Int, b: Int): Int = {\n    val x%d = a + b\n    x%d\n  }\n}\n\n", i, i, i, i)
	}
	valid := source.Bytes()
	broken := []byte("object First { def f(a: Int): Int = { val x = a +\n x } }\nobject Second { def g = ( }\n")
	lang := grammars.ScalaLanguage()
	cLang, err := ParityCLanguage("scala")
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name string
		src  []byte
	}{
		{name: "report", src: valid},
		{name: "mixed_error", src: broken},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cParser := sitter.NewParser()
			defer cParser.Close()
			if err := cParser.SetLanguage(cLang); err != nil {
				t.Fatal(err)
			}
			cTree := cParser.Parse(tc.src, nil)
			if cTree == nil || cTree.RootNode() == nil {
				t.Fatal("C parser returned no root")
			}
			defer cTree.Close()
			cDigest, err := COracleDeepDigest(cTree)
			if err != nil {
				t.Fatal(err)
			}
			for _, route := range []struct {
				name      string
				candidate bool
			}{
				{name: "production"},
				{name: "compact", candidate: true},
			} {
				t.Run(route.name, func(t *testing.T) {
					parser := gts.NewParser(lang)
					parser.SetAdmissionCandidateRoute(route.candidate)
					goTree, err := parser.Parse(tc.src)
					if err != nil {
						t.Fatal(err)
					}
					defer goTree.Release()
					inspection, err := benchfixtures.InspectGoTree(goTree.RootNode(), lang)
					if err != nil {
						t.Fatal(err)
					}
					if inspection.SHA256 != cDigest {
						t.Fatalf("Go digest %s differs from locked C digest %s", inspection.SHA256, cDigest)
					}
				})
			}
		})
	}
	t.Run("insert_allocation", func(t *testing.T) {
		site := bytes.Index(valid, []byte("x0"))
		if site < 0 {
			t.Fatal("report fixture has no edit site")
		}
		edited := append(append(append([]byte{}, valid[:site]...), valid[site]), valid[site:]...)
		row := uint32(bytes.Count(valid[:site], []byte{'\n'}))
		col := uint32(site - bytes.LastIndexByte(valid[:site], '\n') - 1)
		point := gts.Point{Row: row, Column: col}
		edit := gts.InputEdit{
			StartByte: uint32(site), OldEndByte: uint32(site), NewEndByte: uint32(site + 1),
			StartPoint: point, OldEndPoint: point, NewEndPoint: gts.Point{Row: row, Column: col + 1},
		}
		cParser := sitter.NewParser()
		defer cParser.Close()
		if err := cParser.SetLanguage(cLang); err != nil {
			t.Fatal(err)
		}
		cTree := cParser.Parse(edited, nil)
		if cTree == nil || cTree.RootNode() == nil {
			t.Fatal("C parser returned no root")
		}
		defer cTree.Close()
		cDigest, err := COracleDeepDigest(cTree)
		if err != nil {
			t.Fatal(err)
		}
		for _, route := range []struct {
			name      string
			candidate bool
		}{{name: "production"}, {name: "compact", candidate: true}} {
			t.Run(route.name, func(t *testing.T) {
				parser := gts.NewParser(lang)
				parser.SetAdmissionCandidateRoute(route.candidate)
				old, err := parser.Parse(valid)
				if err != nil {
					t.Fatal(err)
				}
				defer old.Release()
				old.Edit(edit)
				inc, profile, err := parser.ParseIncrementalProfiled(edited, old)
				if err != nil {
					t.Fatal(err)
				}
				defer inc.Release()
				if profile.NewNodesAllocated > 150_000 {
					t.Fatalf("report edit allocated %d nodes", profile.NewNodesAllocated)
				}
				inspection, err := benchfixtures.InspectGoTree(inc.RootNode(), lang)
				if err != nil {
					t.Fatal(err)
				}
				if inspection.SHA256 != cDigest {
					t.Fatalf("incremental digest %s differs from locked C digest %s", inspection.SHA256, cDigest)
				}
			})
		}
	})
}
