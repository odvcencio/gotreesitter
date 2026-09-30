package parse_test

import (
	"fmt"
	"testing"

	gts "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
	"github.com/odvcencio/gotreesitter/internal/benchfixtures"
)

func TestSharedExternalPaddingKeepsSwitchCaseVersion(t *testing.T) {
	lang := grammars.GoLanguage()
	for _, source := range []string{
		"package p\nfunc f(){switch x{case\nY:}}\n",
		"package p\nfunc f(){switch x{case\r\nY:}}\n",
		"package p\nfunc f(){switch x{case\nY, Z:\nreturn}}\n",
	} {
		for _, candidate := range []bool{false, true} {
			t.Run(fmt.Sprintf("%q/compact=%t", source, candidate), func(t *testing.T) {
				input := []byte(source)
				parser := gts.NewParser(lang)
				parser.SetAdmissionCandidateRoute(candidate)
				tree, err := parser.Parse(input)
				if err != nil {
					t.Fatal(err)
				}
				defer tree.Release()
				if tree.RootNode().HasError() || tree.ParseRuntime().StopReason != gts.ParseStopAccepted || tree.RootNode().EndByte() != uint32(len(source)) {
					t.Fatalf("switch case lost its clean version: %s, stop=%s", tree.RootNode().SExpr(lang), tree.ParseRuntime().StopReason)
				}
				if allocations := testing.AllocsPerRun(100, func() {
					next, err := parser.ParseIncremental(input, tree)
					if err != nil {
						panic(err)
					}
					next.Release()
				}); allocations != 0 {
					t.Fatalf("no-edit reparse allocated %g times", allocations)
				}
			})
		}
	}
}

func TestSharedExternalPaddingIncrementalInvariant(t *testing.T) {
	lang := grammars.GoLanguage()
	source := []byte("package p\nfunc f(){switch x{case\nY:}}\n")
	for _, candidate := range []bool{false, true} {
		t.Run(fmt.Sprintf("compact=%t", candidate), func(t *testing.T) {
			parser := gts.NewParser(lang)
			parser.SetAdmissionCandidateRoute(candidate)
			old, err := parser.Parse(source)
			if err != nil {
				t.Fatal(err)
			}
			defer func() { old.Release() }()
			for index, step := range benchfixtures.EditingSession(source) {
				old.Edit(step.Edit)
				next, err := parser.ParseIncremental(step.Source, old)
				if err != nil {
					t.Fatal(err)
				}
				fresh, err := parser.Parse(step.Source)
				if err != nil {
					next.Release()
					t.Fatal(err)
				}
				got, err := benchfixtures.InspectGoTree(next.RootNode(), lang)
				if err != nil {
					t.Fatal(err)
				}
				want, err := benchfixtures.InspectGoTree(fresh.RootNode(), lang)
				fresh.Release()
				if err != nil {
					t.Fatal(err)
				}
				if got.SHA256 != want.SHA256 {
					next.Release()
					t.Fatalf("step %d incremental=%s fresh=%s", index+1, got.SHA256, want.SHA256)
				}
				if next.RootNode().IsError() && !next.RootNode().HasError() {
					t.Fatalf("step %d ERROR root has no error flag", index+1)
				}
				if next.RootNode().EndByte() < uint32(len(step.Source)) && (next.ParseRuntime().StopReason == gts.ParseStopAccepted || next.ParseRuntime().StopReason == gts.ParseStopNone) {
					t.Fatalf("step %d unexplained root gap", index+1)
				}
				old.Release()
				old = next
			}
		})
	}
}
