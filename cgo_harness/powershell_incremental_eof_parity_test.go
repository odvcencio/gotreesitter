//go:build cgo && treesitter_c_parity

package cgoharness

import (
	"fmt"
	"strings"
	"testing"

	gotreesitter "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
	sitter "github.com/tree-sitter/go-tree-sitter"
)

// EOF appends must discard reductions justified by the old EOF lookahead,
// even when the preceding edit session returned a legacy tree.
func TestPowerShellIncrementalEOFCommandMatchesCReference(t *testing.T) {
	cLang, err := ParityCLanguage("powershell")
	if err != nil {
		t.Fatal(err)
	}
	cParser := sitter.NewParser()
	defer cParser.Close()
	if err := cParser.SetLanguage(cLang); err != nil {
		t.Fatal(err)
	}
	lang := grammars.PowershellLanguage()
	for _, text := range []string{"a\n", "Describe \"event\" { It \"works\" { Remove-Event -SourceIdentifier Timer } }\n"} {
		for _, compactOld := range []bool{false, true} {
			for _, profiled := range []bool{false, true} {
				t.Run(fmt.Sprintf("%s/compactOld=%t/profiled=%t", text, compactOld, profiled), func(t *testing.T) {
					before, after := []byte(text), []byte(text+"x")
					parser := gotreesitter.NewParser(lang)
					parser.SetAdmissionCandidateRoute(compactOld)
					old, err := parser.Parse(before)
					if err != nil {
						t.Fatal(err)
					}
					defer old.Release()
					point := pointAtOffset(before, len(before))
					old.Edit(gotreesitter.InputEdit{
						StartByte: uint32(len(before)), OldEndByte: uint32(len(before)), NewEndByte: uint32(len(after)),
						StartPoint: point, OldEndPoint: point, NewEndPoint: pointAtOffset(after, len(after)),
					})
					parser.SetAdmissionCandidateRoute(true)
					var inc *gotreesitter.Tree
					if profiled {
						inc, _, err = parser.ParseIncrementalProfiled(after, old)
					} else {
						inc, err = parser.ParseIncremental(after, old)
					}
					if err != nil {
						t.Fatal(err)
					}
					defer inc.Release()
					cTree := cParser.Parse(after, nil)
					if cTree == nil {
						t.Fatal("locked C parse returned no tree")
					}
					defer cTree.Close()
					if cTree.RootNode().HasError() || cTree.RootNode().EndByte() != uint(len(after)) {
						t.Fatal("locked C rejected or truncated the EOF command")
					}
					var diffs []string
					compareNodes(inc.RootNode(), lang, cTree.RootNode(), "root", &diffs)
					if len(diffs) != 0 {
						t.Fatalf("incremental Go differs from locked C:\n%s", strings.Join(diffs, "\n"))
					}
				})
			}
		}
	}
}
