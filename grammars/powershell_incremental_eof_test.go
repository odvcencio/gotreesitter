//go:build !grammar_subset || grammar_subset_powershell

package grammars_test

import (
	"fmt"
	"testing"

	gotreesitter "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
)

// Appending a command can invalidate every compact reuse candidate. The
// old EOF boundary must be reparsed instead of falling back to a
// different engine that treats the command as an error.
func TestPowerShellIncrementalEOFCommand(t *testing.T) {
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
					old.Edit(spliceTestEdit(before, after))
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
					if inc != old {
						defer inc.Release()
					}
					fresh, err := parser.Parse(after)
					if err != nil {
						t.Fatal(err)
					}
					defer fresh.Release()
					if fresh.RootNode().HasError() {
						t.Fatal("fresh parse rejected the appended command")
					}
					if diff := spliceTreeDiff(inc.RootNode(), fresh.RootNode(), lang, ""); diff != "" {
						t.Fatalf("%s\ninc: %s\nfresh: %s", diff, inc.RootNode().SExpr(lang), fresh.RootNode().SExpr(lang))
					}
					var noEditErr error
					allocs := testing.AllocsPerRun(5, func() {
						next, parseErr := parser.ParseIncremental(after, inc)
						if parseErr != nil {
							noEditErr = parseErr
							return
						}
						next.Release()
					})
					if noEditErr != nil {
						t.Fatalf("no-edit reparse: %v", noEditErr)
					}
					if allocs != 0 {
						t.Fatalf("no-edit reparse allocations=%g, want 0", allocs)
					}
				})
			}
		}
	}
}
