//go:build cgo && treesitter_c_parity

package cgoharness

import (
	"bytes"
	"fmt"
	"strings"
	"testing"

	gts "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
	sitter "github.com/tree-sitter/go-tree-sitter"
)

func TestRecoveredVersionProgressLockedC(t *testing.T) {
	for _, name := range []string{"typescript", "tsx"} {
		t.Run(name, func(t *testing.T) {
			lang := grammars.TypescriptLanguage()
			if name == "tsx" {
				lang = grammars.TsxLanguage()
			}
			cl, err := ParityCLanguage(name)
			if err != nil {
				t.Fatal(err)
			}
			cp := sitter.NewParser()
			defer cp.Close()
			if err := cp.SetLanguage(cl); err != nil {
				t.Fatal(err)
			}
			var source strings.Builder
			count := 0
			for source.Len() < 20*1024 {
				fmt.Fprintf(&source, "export function f%d(a: number): number {\n  const v%d = a + %d;\n  return v%d;\n}\n\n", count, count, count, count)
				count++
			}
			src := []byte(source.String())
			for _, index := range []int{0, count / 2} {
				t.Run(fmt.Sprint(index), func(t *testing.T) {
					marker := []byte(fmt.Sprintf("const v%d =", index))
					start := bytes.Index(src, marker)
					if start < 0 {
						t.Fatal("edit marker is missing")
					}
					start += len(marker) - 1
					edited := append([]byte(nil), src...)
					edited[start] = 'z'
					edit := gts.InputEdit{StartByte: uint32(start), OldEndByte: uint32(start + 1), NewEndByte: uint32(start + 1), StartPoint: pointAtOffset(src, start), OldEndPoint: pointAtOffset(src, start+1), NewEndPoint: pointAtOffset(edited, start+1)}
					p := gts.NewParser(lang)
					p.SetAdmissionCandidateRoute(false)
					old, err := p.Parse(src)
					if err != nil {
						t.Fatal(err)
					}
					defer old.Release()
					old.Edit(edit)
					next, err := p.ParseIncremental(edited, old)
					if err != nil {
						t.Fatal(err)
					}
					defer next.Release()
					freshParser := gts.NewParser(lang)
					freshParser.SetAdmissionCandidateRoute(false)
					fresh, err := freshParser.Parse(edited)
					if err != nil {
						t.Fatal(err)
					}
					defer fresh.Release()
					oracle := cp.Parse(edited, nil)
					if oracle == nil {
						t.Fatal("C tree is nil")
					}
					defer oracle.Close()
					assertG18LockedCExact(t, "incremental recovered progress", next, lang, oracle)
					assertG18LockedCExact(t, "fresh recovered progress", fresh, lang, oracle)
				})
			}
		})
	}
}
