//go:build gts_parsercorephase0 && !gts_no_parsercorephase0

package gotreesitter_test

import (
	"testing"

	gts "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
)

func TestTokenInvariantPythonNumericControls(t *testing.T) {
	for _, tc := range []struct {
		name, before, after string
		offset              uint32
		wantReuse           bool
	}{
		{"integer", "x = 1\n", "x = 2\n", 4, true},
		{"exponent", "x = 1e1\n", "x = 1e0\n", 6, true},
		{"binary_token_boundary", "x = 0b1\n", "x = 0b2\n", 6, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			for _, compact := range []bool{false, true} {
				name := "legacy"
				if compact {
					name = "compact"
				}
				t.Run(name, func(t *testing.T) {
					lang := grammars.PythonLanguage()
					parser := gts.NewParser(lang)
					parser.SetAdmissionCandidateRoute(compact)
					before, _ := gts.AdmissionCandidateCounters()
					old, err := parser.Parse([]byte(tc.before))
					if err != nil {
						t.Fatal(err)
					}
					defer old.Release()
					after, _ := gts.AdmissionCandidateCounters()
					if old.RootNode().HasError() || (after > before) != compact {
						t.Fatal("initial numeric fixture did not use the requested clean route")
					}
					old.Edit(gts.InputEdit{StartByte: tc.offset, OldEndByte: tc.offset + 1, NewEndByte: tc.offset + 1,
						StartPoint: gts.Point{Column: tc.offset}, OldEndPoint: gts.Point{Column: tc.offset + 1}, NewEndPoint: gts.Point{Column: tc.offset + 1}})
					next, profile, err := parser.ParseIncrementalProfiled([]byte(tc.after), old)
					if err != nil {
						t.Fatal(err)
					}
					if next != old {
						defer next.Release()
					}
					freshParser := gts.NewParser(lang)
					freshParser.SetAdmissionCandidateRoute(false)
					fresh, err := freshParser.Parse([]byte(tc.after))
					if err != nil {
						t.Fatal(err)
					}
					defer fresh.Release()
					if tc.wantReuse && fresh.RootNode().HasError() {
						t.Fatal("fresh numeric control contains an error")
					}
					if !tc.wantReuse && fresh.RootNode().SExpr(lang) == old.RootNode().SExpr(lang) {
						t.Fatalf("negative fixture did not change syntax: old=%s fresh=%s", old.RootNode().SExpr(lang), fresh.RootNode().SExpr(lang))
					}
					requireIncrementalDeepTreeMatchesFresh(t, next, fresh, lang)
					if (tc.wantReuse || compact) && profile.TokenInvariantDependencyChecks == 0 {
						t.Fatal("numeric edit did not compare internal lexer dependencies")
					}
					if gotReuse := next.RootNode() == old.RootNode(); gotReuse != tc.wantReuse {
						t.Fatalf("shared root=%t, want %t; profile=%+v", gotReuse, tc.wantReuse, profile)
					}
					if tc.wantReuse && (profile.ReparseNanos != 0 || profile.NewNodesAllocated != 0) {
						t.Fatalf("valid numeric edit reparsed: %+v", profile)
					}
					// A legacy producer may recover before returning a clean tree.
					// Unknown history must also rebuild this changed token boundary.
					if !tc.wantReuse && (profile.ReparseNanos == 0 || profile.NewNodesAllocated == 0) {
						t.Fatalf("changed numeric boundary did not rebuild: %+v", profile)
					}
				})
			}
		})
	}
}
