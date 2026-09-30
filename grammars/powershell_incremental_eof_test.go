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
					routedBefore, declinedBefore := gotreesitter.AdmissionCandidateCounters()
					var inc *gotreesitter.Tree
					if profiled {
						var profile gotreesitter.IncrementalParseProfile
						inc, profile, err = parser.ParseIncrementalProfiled(after, old)
						if err == nil && (!profile.ReuseUnsupported || profile.ReuseUnsupportedReason != "eof_append_fresh" ||
							profile.OldTreeReuseRoute || profile.ReusedBytes != 0 || profile.ReusedSubtrees != 0) {
							t.Fatalf("EOF append profile: %+v", profile)
						}
					} else {
						inc, err = parser.ParseIncremental(after, old)
					}
					if err != nil {
						t.Fatal(err)
					}
					if inc != old {
						defer inc.Release()
					}
					routedAfter, declinedAfter := gotreesitter.AdmissionCandidateCounters()
					if routedAfter+declinedAfter != routedBefore+declinedBefore+1 {
						t.Fatalf("EOF append full-route events: %d -> %d, want one fresh attempt",
							routedBefore+declinedBefore, routedAfter+declinedAfter)
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
					for _, noEditProfiled := range []bool{false, true} {
						var noEditErr error
						var changedTree bool
						allocs := testing.AllocsPerRun(5, func() {
							var next *gotreesitter.Tree
							var parseErr error
							if noEditProfiled {
								next, _, parseErr = parser.ParseIncrementalProfiled(after, inc)
							} else {
								next, parseErr = parser.ParseIncremental(after, inc)
							}
							if parseErr != nil {
								noEditErr = parseErr
								return
							}
							changedTree = changedTree || next != inc
							next.Release()
						})
						if noEditErr != nil {
							t.Fatalf("no-edit reparse: %v", noEditErr)
						}
						if allocs != 0 {
							t.Fatalf("no-edit reparse profiled=%t allocations=%g, want 0", noEditProfiled, allocs)
						}
						if changedTree {
							t.Fatal("no-edit reparse did not retain the result tree")
						}
					}
				})
			}
		}
	}
}
