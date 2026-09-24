//go:build cgo && treesitter_c_parity

package cgoharness

import (
	"bytes"
	"fmt"
	"testing"

	gotreesitter "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
	sitter "github.com/tree-sitter/go-tree-sitter"
)

func issue454ValidCSharpSource(n int) []byte {
	var b bytes.Buffer
	b.WriteString("using System;\n\n")
	for i := 0; b.Len() < n; i++ {
		fmt.Fprintf(&b, "class C%d {\n\tpublic int F%d(int a, int b) {\n\t\tvar x%d = a + b;\n\t\treturn x%d;\n\t}\n}\n\n", i, i, i, i)
	}
	return b.Bytes()
}

func TestIssue454ValidCSharpLockedCParity(t *testing.T) {
	goLang := grammars.CSharpLanguage()
	cLang, err := COracleLanguage("c_sharp")
	if err != nil {
		t.Fatal(err)
	}
	cParser := sitter.NewParser()
	defer cParser.Close()
	if err := cParser.SetLanguage(cLang); err != nil {
		t.Fatal(err)
	}
	for _, sizeKB := range []int{2, 8, 32, 137} {
		t.Run(fmt.Sprintf("%dKB", sizeKB), func(t *testing.T) {
			source := issue454ValidCSharpSource(sizeKB << 10)
			cTree := cParser.Parse(source, nil)
			if cTree == nil {
				t.Fatal("C oracle returned no tree")
			}
			defer cTree.Close()
			if cTree.RootNode().HasError() || cTree.RootNode().EndByte() != uint(len(source)) {
				t.Fatal("C oracle did not complete a clean parse")
			}
			for _, compact := range []bool{false, true} {
				parser := gotreesitter.NewParser(goLang)
				parser.SetAdmissionCandidateRoute(compact)
				tree, err := parser.Parse(source)
				if err != nil {
					t.Fatal(err)
				}
				assertLockedCTreeExact(t, fmt.Sprintf("C# %dKB compact=%t", sizeKB, compact), tree, goLang, cTree)
				tree.Release()
			}
		})
	}
}
