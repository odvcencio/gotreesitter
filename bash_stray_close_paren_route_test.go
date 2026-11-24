//go:build !gts_no_parsercorephase0

package gotreesitter_test

import (
	"testing"

	gts "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
)

// TestBashStrayCloseParenRoutesStillDiffer pins the open finding behind the
// nightly fuzz failure in FuzzAdmissionRouteEquality (input "0)0A", bash).
// Locked C and the compact route keep the stray ")" as an extra ERROR inside
// one command_name concatenation. The production route closes command_name
// first and makes the trailing word a sibling. The compact tree is the
// correct one; see TestBashStrayCloseParenInWordMatchesLockedCOnCompactRoute
// in cgo_harness. This test fails once the production route is fixed, so the
// pin is removed and the fuzz seed can be committed.
func TestBashStrayCloseParenRoutesStillDiffer(t *testing.T) {
	entry := grammars.DetectLanguageByName("bash")
	if entry == nil {
		t.Fatal("bash is not registered")
	}
	lang := entry.Language()
	const (
		wantCompact    = "(program (command (command_name (concatenation (word) (ERROR) (word)))))"
		wantProduction = "(program (command (command_name (word)) (ERROR) (word)))"
	)
	for _, tc := range []struct {
		route bool
		want  string
	}{{true, wantCompact}, {false, wantProduction}} {
		p := gts.NewParser(lang)
		p.SetAdmissionCandidateRoute(tc.route)
		tree, err := p.Parse([]byte("a)b"))
		if err != nil {
			t.Fatal(err)
		}
		got := tree.RootNode().SExpr(lang)
		tree.Release()
		if got != tc.want {
			t.Errorf("compact=%v: got %s, want %s", tc.route, got, tc.want)
		}
	}
}
