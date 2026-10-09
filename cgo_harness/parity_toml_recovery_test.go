//go:build cgo && treesitter_c_parity

package cgoharness

import (
	"fmt"
	"testing"

	gts "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
	sitter "github.com/tree-sitter/go-tree-sitter"
)

// Issue #1340: an unfinished value must stay in its table, with the same
// missing value, error flags and scanner-padding extent as the locked C tree.
func TestTOMLUnfinishedPairWithScannerPadding(t *testing.T) {
	cl, err := COracleLanguage("toml")
	if err != nil {
		t.Fatal(err)
	}
	lang := grammars.TomlLanguage()
	for _, prefix := range []struct{ name, text string }{
		{"root", ""},
		{"table", "[session]\nvalue = 0\n"},
		{"nested_table", "[session.child]\nvalue = 0\n"},
		{"array_table", "[[sessions]]\nvalue = 0\n"},
	} {
		for _, padding := range []string{" ", "  ", "\t", " \t "} {
			for _, newline := range []bool{false, true} {
				for _, candidate := range []bool{false, true} {
					t.Run(fmt.Sprintf("%s/padding=%q/newline=%t/compact=%t", prefix.name, padding, newline, candidate), func(t *testing.T) {
						broken := []byte(prefix.text + "half =" + padding)
						at := len(broken)
						clean := append(append([]byte(nil), broken...), '1')
						if newline {
							broken = append(broken, '\n')
							clean = append(clean, '\n')
						}
						p := gts.NewParser(lang)
						p.SetAdmissionCandidateRoute(candidate)
						cp := sitter.NewParser()
						defer cp.Close()
						if err := cp.SetLanguage(cl); err != nil {
							t.Fatal(err)
						}
						old, err := p.Parse(clean)
						if err != nil {
							t.Fatal(err)
						}
						cold := cp.Parse(clean, nil)
						if cold == nil {
							old.Release()
							t.Fatal("C returned no clean tree")
						}
						edit := gts.InputEdit{StartByte: uint32(at), OldEndByte: uint32(at + 1), NewEndByte: uint32(at), StartPoint: pointAtOffset(clean, at), OldEndPoint: pointAtOffset(clean, at+1), NewEndPoint: pointAtOffset(broken, at)}
						old.Edit(edit)
						ce := realCorpusCInputEdit(edit)
						cold.Edit(&ce)
						changed, err := p.ParseIncremental(broken, old)
						old.Release()
						if err != nil {
							cold.Close()
							t.Fatal(err)
						}
						defer changed.Release()
						cchanged := cp.Parse(broken, cold)
						cold.Close()
						if cchanged == nil {
							t.Fatal("C returned no edited tree")
						}
						defer cchanged.Close()
						fresh, err := p.Parse(broken)
						if err != nil {
							t.Fatal(err)
						}
						defer fresh.Release()
						cfresh := cp.Parse(broken, nil)
						if cfresh == nil {
							t.Fatal("C returned no fresh tree")
						}
						defer cfresh.Close()
						for _, gt := range []*gts.Tree{fresh, changed} {
							for _, ct := range []*sitter.Tree{cfresh, cchanged} {
								assertLockedCTreeExactWithErrors(t, "unfinished pair", gt, lang, ct)
							}
							if !gt.RootNode().HasError() {
								t.Fatal("missing value lost its error flag")
							}
						}
						repair := gts.InputEdit{StartByte: uint32(at), OldEndByte: uint32(at), NewEndByte: uint32(at + 1), StartPoint: pointAtOffset(broken, at), OldEndPoint: pointAtOffset(broken, at), NewEndPoint: pointAtOffset(clean, at+1)}
						changed.Edit(repair)
						repaired, err := p.ParseIncremental(clean, changed)
						if err != nil {
							t.Fatal(err)
						}
						defer repaired.Release()
						cclean := cp.Parse(clean, nil)
						if cclean == nil {
							t.Fatal("C returned no repaired tree")
						}
						defer cclean.Close()
						assertLockedCTreeExact(t, "repaired pair", repaired, lang, cclean)
					})
				}
			}
		}
	}
}
