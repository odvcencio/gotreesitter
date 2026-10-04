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

func TestRecoverySkippedGapInvalidPrefix(t *testing.T) {
	t.Setenv("GOT_C_RECOVERY", "0")
	for _, name := range []string{"javascript", "typescript"} {
		t.Run(name, func(t *testing.T) {
			language := *grammars.DetectLanguageByName(name).Language()
			language.AutomaticForestEnabledByDefault = false
			for _, compact := range []bool{false, true} {
				for _, source := range []string{"\x01%O", "\x00%O", "\x02%O", "\x01 \t%O"} {
					parser := gts.NewParser(&language)
					parser.SetAdmissionCandidateRoute(compact)
					tree, err := parser.Parse([]byte(source))
					if err != nil {
						t.Fatal(err)
					}
					root := tree.RootNode()
					if tree.ParseStopReason() != gts.ParseStopAccepted || root.StartByte() != 0 || root.EndByte() != uint32(len(source)) || !root.HasError() {
						t.Errorf("compact=%t source=%q stop=%v span=%d..%d error=%v tree=%s", compact, source, tree.ParseStopReason(), root.StartByte(), root.EndByte(), root.HasError(), root.SExpr(&language))
					}
					tree.Release()
				}
			}
		})
	}
}
