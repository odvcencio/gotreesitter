//go:build cgo && treesitter_c_parity

package cgoharness

import (
	"bytes"
	"fmt"
	"strings"
	"testing"

	"github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
	sitter "github.com/tree-sitter/go-tree-sitter"
)

func TestCInitializerRepetitionMatchesLockedC(t *testing.T) {
	cl, err := COracleLanguage("c")
	if err != nil {
		t.Fatal(err)
	}
	cp := sitter.NewParser()
	defer cp.Close()
	if err := cp.SetLanguage(cl); err != nil {
		t.Fatal(err)
	}
	lang := grammars.CLanguage()
	for _, rows := range []int{1, 2, 4, 8, 16, 40, 80} {
		for _, ws := range []bool{false, true} {
			t.Run(fmt.Sprintf("rows%d_ws%t", rows, ws), func(t *testing.T) {
				var b strings.Builder
				b.WriteString("static const element_t table[] = {\n")
				for i := 0; i < rows; i++ {
					if ws {
						b.WriteString("\t{{ 0x1,0x2, 0x3, 0x4, }}, \n")
					} else {
						b.WriteString("  {{ 0x1, 0x2, 0x3, 0x4, }},\n")
					}
				}
				b.WriteString("};\n")
				src := []byte(b.String())
				tree, e := gotreesitter.NewParser(lang).Parse(src)
				if e != nil {
					t.Fatal(e)
				}
				defer tree.Release()
				c := cp.Parse(src, nil)
				if c == nil {
					t.Fatal("nil C tree")
				}
				defer c.Close()
				assertLockedCTreeExact(t, fmt.Sprintf("rows%d", rows), tree, lang, c)
			})
		}
	}
}

func TestCInitializerRepetitionEditsMatchFreshAndLockedC(t *testing.T) {
	lang := grammars.CLanguage()
	cl, err := COracleLanguage("c")
	if err != nil {
		t.Fatal(err)
	}
	cp := sitter.NewParser()
	defer cp.Close()
	if err := cp.SetLanguage(cl); err != nil {
		t.Fatal(err)
	}
	source := []byte("static const element_t table[] = {\n" + strings.Repeat("  {{ 0x1, 0x2, 0x3, 0x4, }},\n", 40) + "};\n")
	for _, tc := range []struct{ name, needle, replacement string }{{"numeric", "0x3", "0x9"}, {"trailing-comma", ", }", " }"}} {
		t.Run(tc.name, func(t *testing.T) {
			parser := gotreesitter.NewParser(lang)
			parser.SetAdmissionCandidateRoute(false)
			old, err := parser.Parse(source)
			if err != nil {
				t.Fatal(err)
			}
			defer old.Release()
			start := bytes.Index(source, []byte(tc.needle))
			if start < 0 {
				t.Fatal("edit not found")
			}
			end := start + len(tc.needle)
			newEnd := start + len(tc.replacement)
			edited := append([]byte(nil), source[:start]...)
			edited = append(edited, tc.replacement...)
			edited = append(edited, source[end:]...)
			old.Edit(gotreesitter.InputEdit{StartByte: uint32(start), OldEndByte: uint32(end), NewEndByte: uint32(newEnd), StartPoint: pointAtOffset(source, start), OldEndPoint: pointAtOffset(source, end), NewEndPoint: pointAtOffset(edited, newEnd)})
			incremental, err := parser.ParseIncremental(edited, old)
			if err != nil {
				t.Fatal(err)
			}
			defer incremental.Release()
			freshParser := gotreesitter.NewParser(lang)
			freshParser.SetAdmissionCandidateRoute(false)
			fresh, err := freshParser.Parse(edited)
			if err != nil {
				t.Fatal(err)
			}
			defer fresh.Release()
			c := cp.Parse(edited, nil)
			if c == nil {
				t.Fatal("nil C tree")
			}
			defer c.Close()
			assertLockedCTreeExact(t, tc.name+"-incremental", incremental, lang, c)
			assertLockedCTreeExact(t, tc.name+"-fresh", fresh, lang, c)
			type pair struct{ incremental, fresh *gotreesitter.Node }
			pending := []pair{{incremental.RootNode(), fresh.RootNode()}}
			for len(pending) > 0 {
				last := len(pending) - 1
				p := pending[last]
				pending = pending[:last]
				if p.incremental.ParseState() != p.fresh.ParseState() || p.incremental.PreGotoState() != p.fresh.PreGotoState() {
					t.Fatalf("state mismatch at %s[%d:%d]: parse %d/%d pre-goto %d/%d", p.incremental.Type(lang), p.incremental.StartByte(), p.incremental.EndByte(), p.incremental.ParseState(), p.fresh.ParseState(), p.incremental.PreGotoState(), p.fresh.PreGotoState())
				}
				if p.incremental.ChildCount() != p.fresh.ChildCount() {
					t.Fatal("child count mismatch")
				}
				for i := 0; i < p.incremental.ChildCount(); i++ {
					pending = append(pending, pair{p.incremental.Child(i), p.fresh.Child(i)})
				}
			}
			var noEditErr error
			allocations := testing.AllocsPerRun(5, func() {
				same, err := parser.ParseIncremental(edited, incremental)
				if err != nil {
					noEditErr = err
					return
				}
				if same == nil {
					noEditErr = fmt.Errorf("nil no-edit tree")
					return
				}
				same.Release()
			})
			if noEditErr != nil || allocations != 0 {
				t.Fatalf("no-edit allocations=%g err=%v", allocations, noEditErr)
			}
		})
	}
}
