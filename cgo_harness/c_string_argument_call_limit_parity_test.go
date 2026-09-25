//go:build cgo && treesitter_c_parity

package cgoharness

import (
	"fmt"
	"strings"
	"testing"

	gotreesitter "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
	sitter "github.com/tree-sitter/go-tree-sitter"
)

func TestCStringArgumentCallCountMatchesLockedC(t *testing.T) {
	cases := []struct {
		count     int
		functions bool
	}{
		{count: 4090, functions: true},
		{count: 4091, functions: true},
		{count: 4100, functions: true},
		{count: 4090},
		{count: 4091},
		{count: 4092},
		{count: 8192},
		{count: 65536},
	}
	cLang, err := ParityCLanguage("c")
	if err != nil {
		t.Fatal(err)
	}
	cParser := sitter.NewParser()
	defer cParser.Close()
	if err := cParser.SetLanguage(cLang); err != nil {
		t.Fatal(err)
	}
	for _, tc := range cases {
		shape := "calls"
		if tc.functions {
			shape = "functions"
		}
		t.Run(fmt.Sprintf("%d/%s", tc.count, shape), func(t *testing.T) {
			source := cStringArgumentOracleSource(tc.count, tc.functions)
			cTree := cParser.Parse(source, nil)
			if cTree == nil || cTree.RootNode() == nil {
				t.Fatal("locked C parser returned no tree")
			}
			cRoot := cTree.RootNode()
			if cRoot.HasError() || cRoot.StartByte() != 0 || cRoot.EndByte() != uint(len(source)) {
				t.Fatalf("locked C tree: error=%t bytes=%d..%d, source=%d", cRoot.HasError(), cRoot.StartByte(), cRoot.EndByte(), len(source))
			}
			goLang := grammars.CLanguage()
			goParser := gotreesitter.NewParser(goLang)
			goParser.SetAdmissionCandidateRoute(false)
			goTree, err := goParser.ParseWithTokenSource(source, grammars.NewCTokenSourceOrEOF(source, goLang))
			if err != nil {
				t.Fatalf("TokenSource parse: %v", err)
			}
			if diff := FirstDivergenceDumpV1(goTree.RootNode(), goLang, cRoot); diff != nil {
				t.Fatalf("TokenSource differs from locked C: %+v", diff)
			}
			goTree.Release()
			cTree.Close()
			runParityCase(t, parityCase{name: "c"}, "string-argument-call-count", source)
		})
	}
}

func cStringArgumentOracleSource(count int, functions bool) []byte {
	var source strings.Builder
	if functions {
		source.Grow(count * 90)
		source.WriteString("#include <stdio.h>\n\n")
		for i := range count {
			fmt.Fprintf(&source, "int f%d(int a, int b) {\n\tint x%d = a + b;\n\tprintf(\"%%d\\n\", x%d);\n\treturn x%d;\n}\n\n", i, i, i, i)
		}
	} else {
		source.Grow(count * 16)
		source.WriteString("void f(void) {\n")
		for i := range count {
			fmt.Fprintf(&source, "g(\"x\", %d);\n", i)
		}
		source.WriteString("}\n")
	}
	return []byte(source.String())
}
