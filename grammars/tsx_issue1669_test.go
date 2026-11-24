package grammars

import (
	"os"
	"testing"

	gotreesitter "github.com/odvcencio/gotreesitter"
)

// Issue 1669: a valid TSX file with a typed arrow function after generic
// calls came back with an ERROR node on the legacy route. The default route
// must parse it cleanly and cover the whole input.
func TestTSXIssue1669ContextDependentArrow(t *testing.T) {
	source, err := os.ReadFile("testdata/tsx_issue1669.tsx")
	if err != nil {
		t.Fatal(err)
	}
	lang := TsxLanguage()
	tree, err := gotreesitter.NewParser(lang).ParseStrict(source)
	if err != nil {
		t.Fatal(err)
	}
	defer tree.Release()
	root := tree.RootNode()
	if root.HasErrorOrMissing() || tree.ParseStoppedEarly() || int(root.EndByte()) != len(source) {
		t.Fatalf("incomplete parse: %s", root.SExpr(lang))
	}
}

// Issue 1669: String.raw templates with invalid escape sequences. The pinned
// upstream grammars reject them, and pinned C reports an error too. See
// tree-sitter-javascript issue 337. This test pins that behaviour. If it
// fails after a grammar bump, upstream now accepts these inputs: flip the
// assertion to require a clean parse.
func TestStringRawInvalidEscapeUpstreamIssue337(t *testing.T) {
	bt := "`"
	sources := map[string]string{
		"hex":  "const a = String.raw" + bt + `\x` + bt + ";",
		"uni":  "const b = String.raw" + bt + `\u945` + bt + ";",
		"path": "const c = String.raw" + bt + `C:\Users\a b\.t3\x` + bt + ";",
	}
	langs := map[string]func() *gotreesitter.Language{
		"typescript": TypescriptLanguage,
		"tsx":        TsxLanguage,
	}
	for ln, load := range langs {
		for name, src := range sources {
			t.Run(ln+"/"+name, func(t *testing.T) {
				tree, err := gotreesitter.NewParser(load()).ParseStrict([]byte(src))
				if err != nil {
					t.Fatal(err)
				}
				defer tree.Release()
				if !tree.RootNode().HasErrorOrMissing() {
					t.Fatalf("upstream now accepts this input; update the test (tree-sitter-javascript issue 337)")
				}
			})
		}
	}
}
