//go:build cgo && treesitter_c_parity

package cgoharness

import (
	"testing"

	gotreesitter "github.com/odvcencio/gotreesitter"
	sitter "github.com/tree-sitter/go-tree-sitter"
)

// TestBashStrayCloseParenInWordMatchesLockedCOnCompactRoute pins the nightly
// fuzz finding "0)0A" (FuzzAdmissionRouteEquality, bash). Locked C keeps the
// stray ")" as an extra ERROR inside the command_name concatenation. The
// compact route builds exactly that tree. The production route closes
// command_name before the ")" and makes the trailing word a sibling, so it
// still diverges from C.
//
// The production half of this test pins today's open divergence. When the
// production recovery ranking is fixed, that subtest fails with a message
// that says to require exact equality for both routes.
func TestBashStrayCloseParenInWordMatchesLockedCOnCompactRoute(t *testing.T) {
	cLang, err := COracleLanguage("bash")
	if err != nil {
		t.Fatal(err)
	}
	cParser := sitter.NewParser()
	t.Cleanup(cParser.Close)
	if err := cParser.SetLanguage(cLang); err != nil {
		t.Fatal(err)
	}

	for _, source := range []string{"0)0A", "a)b"} {
		cTree := cParser.Parse([]byte(source), nil)
		if cTree == nil || cTree.RootNode() == nil {
			t.Fatal("locked C parser returned no tree")
		}
		t.Cleanup(cTree.Close)

		t.Run(source+"/compact", func(t *testing.T) {
			tree, lang, err := parseWithGo(parityCase{
				name: "bash", source: source, candidateRoute: boolPtr(true),
			}, []byte(source), nil)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { releaseGoTree(tree) })
			assertLockedCTreeExactWithErrors(t, "Bash stray close paren", tree, lang, cTree)
		})

		t.Run(source+"/production/open-divergence", func(t *testing.T) {
			tree, lang, err := parseWithGo(parityCase{
				name: "bash", source: source, candidateRoute: boolPtr(false),
			}, []byte(source), nil)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { releaseGoTree(tree) })
			var goTree *gotreesitter.Tree = tree
			if FirstDivergenceDumpV1(goTree.RootNode(), lang, cTree.RootNode()) == nil {
				t.Fatalf("production route now matches locked C for %q: require exact equality for both routes and delete this pin", source)
			}
		})
	}
}

func boolPtr(b bool) *bool { return &b }
