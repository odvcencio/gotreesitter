//go:build cgo && treesitter_c_parity

package cgoharness

import (
	"fmt"
	"os"
	"testing"

	gotreesitter "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
	"github.com/odvcencio/gotreesitter/internal/benchfixtures"
	sitter "github.com/tree-sitter/go-tree-sitter"
)

func TestSwiftLedgerDirectivesMatchLockedC(t *testing.T) {
	source, err := os.ReadFile("../internal/benchfixtures/testdata/real/swift")
	if err != nil {
		t.Fatal(err)
	}
	cLang, err := COracleLanguage("swift")
	if err != nil {
		t.Fatal(err)
	}
	cParser := sitter.NewParser()
	defer cParser.Close()
	if err := cParser.SetLanguage(cLang); err != nil {
		t.Fatal(err)
	}
	lang := grammars.SwiftLanguage()
	parser := gotreesitter.NewParser(lang)
	parser.SetAdmissionCandidateRoute(false)
	full, err := parser.Parse(source)
	if err != nil {
		t.Fatal(err)
	}
	defer full.Release()
	step := benchfixtures.EditingSession(source)[0]
	for _, phase := range []struct {
		name   string
		source []byte
		parse  func() (*gotreesitter.Tree, error)
	}{
		{"full", source, func() (*gotreesitter.Tree, error) { return full, nil }},
		{"edit", step.Source, func() (*gotreesitter.Tree, error) {
			full.Edit(step.Edit)
			return parser.ParseIncremental(step.Source, full)
		}},
	} {
		t.Run(phase.name, func(t *testing.T) {
			cTree := cParser.Parse(phase.source, nil)
			if cTree == nil || cTree.RootNode() == nil {
				t.Fatal("locked C parser returned no tree")
			}
			defer cTree.Close()
			tree, err := phase.parse()
			if err != nil {
				t.Fatal(err)
			}
			if tree != full {
				defer tree.Release()
			}
			t.Logf("Go_has_error=%t C_has_error=%t", tree.RootNode().HasError(), cTree.RootNode().HasError())
			assertLockedCTreeExact(t, "Swift ledger directives", tree, lang, cTree)
		})
	}
}

func TestSwiftFreshDirectivesAndCommentBoundariesMatchLockedC(t *testing.T) {
	cLang, err := COracleLanguage("swift")
	if err != nil {
		t.Fatal(err)
	}
	cParser := sitter.NewParser()
	t.Cleanup(cParser.Close)
	if err := cParser.SetLanguage(cLang); err != nil {
		t.Fatal(err)
	}
	for _, source := range []string{
		"(>){}\n//\n{}",
		"x\n#else\npublic protocolo{staticfuncn()throws->E}",
		"#if D\npublic protocol M{}\n#else\npublic protocol M{}\n#endif\n",
		"struct S {\n#if D\nvar x: Int\n#else\nvar x: Int\n#endif\n}\n",
		"let x = 1\n// hi\nlet y = 2\n",
	} {
		cTree := cParser.Parse([]byte(source), nil)
		if cTree == nil || cTree.RootNode() == nil {
			t.Fatal("locked C parser returned no tree")
		}
		t.Cleanup(cTree.Close)
		for _, candidate := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/compact=%t", source, candidate), func(t *testing.T) {
				tree, lang, err := parseWithGo(parityCase{name: "swift", source: source, candidateRoute: &candidate}, []byte(source), nil)
				if err != nil {
					t.Fatal(err)
				}
				t.Cleanup(func() { releaseGoTree(tree) })
				assertLockedCTreeExact(t, "Swift directives and comments", tree, lang, cTree)
			})
		}
	}
}
