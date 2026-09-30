package gotreesitter_test

import (
	"fmt"
	"strings"
	"testing"

	gts "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
	"github.com/odvcencio/gotreesitter/internal/benchfixtures"
)

func TestJavaScriptMalformedQuotePreservesDeclaration(t *testing.T) {
	language := grammars.JavascriptLanguage()
	for _, fixture := range []struct {
		name   string
		clean  []byte
		digest string
	}{
		{"minimal", []byte("var x = a;\n"), "b41240f9a8b1fc46e76c20afc94a42327c1a89529c4bf610dd0446129da785f9"},
		{"issue_1335", issue454JS(), "f5b06003a7595bde54d6949e26a8c45ef7908cff1ab318e16edd676db959abe0"},
	} {
		t.Run(fixture.name, func(t *testing.T) {
			at := strings.Index(string(fixture.clean), "x")
			broken := append(append(append([]byte{}, fixture.clean[:at]...), '"'), fixture.clean[at:]...)
			for _, compact := range []bool{false, true} {
				t.Run(fmt.Sprintf("compact=%t", compact), func(t *testing.T) {
					parser := gts.NewParser(language)
					parser.SetAdmissionCandidateRoute(compact)
					fresh, err := parser.Parse(broken)
					if err != nil {
						t.Fatal(err)
					}
					defer fresh.Release()
					root := fresh.RootNode()
					if root.Type(language) != "program" || !root.HasError() || root.StartByte() != 0 || root.EndByte() != uint32(len(broken)) {
						t.Fatalf("malformed quote lost the program: %s", root.SExpr(language))
					}
					inspection, err := benchfixtures.InspectGoTree(root, language)
					if err != nil {
						t.Fatal(err)
					}
					if inspection.SHA256 != fixture.digest {
						t.Fatalf("fresh locked-C digest: got %s, want %s", inspection.SHA256, fixture.digest)
					}
					allocations := testing.AllocsPerRun(100, func() {
						same, err := parser.ParseIncremental(broken, fresh)
						if err != nil {
							panic(err)
						}
						same.Release()
					})
					if allocations != 0 {
						t.Fatalf("unchanged malformed input allocated %.2f times", allocations)
					}
					old, err := parser.Parse(fixture.clean)
					if err != nil {
						t.Fatal(err)
					}
					middle := issue454Step(t, parser, language, old, fixture.clean, broken)
					old.Release()
					last := issue454Step(t, parser, language, middle, broken, fixture.clean)
					middle.Release()
					defer last.Release()
					if last.RootNode().HasError() {
						t.Fatal("quote deletion failed to repair the declaration")
					}
				})
			}
		})
	}
}
