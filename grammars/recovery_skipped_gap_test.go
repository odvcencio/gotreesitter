package grammars_test

import (
	"testing"

	gts "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
)

func TestRecoverySkippedGapBeforeConflict(t *testing.T) {
	lang := grammars.JavascriptLanguage()
	for _, source := range []string{";;%t", "a();%b();"} {
		t.Run(source, func(t *testing.T) {
			parser := gts.NewParser(lang)
			parser.SetAdmissionCandidateRoute(false)
			tree, err := parser.Parse([]byte(source))
			if err != nil {
				t.Fatal(err)
			}
			defer tree.Release()
			root := tree.RootNode()
			if tree.ParseStopReason() != gts.ParseStopAccepted || root.EndByte() != uint32(len(source)) || !root.HasError() {
				t.Fatalf("stop=%v span=%d..%d error=%v tree=%s", tree.ParseStopReason(), root.StartByte(), root.EndByte(), root.HasError(), root.SExpr(lang))
			}
		})
	}
}
