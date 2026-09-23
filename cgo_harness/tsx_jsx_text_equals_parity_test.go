//go:build cgo && treesitter_c_parity

package cgoharness

import (
	"strings"
	"testing"

	gotreesitter "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
	sitter "github.com/tree-sitter/go-tree-sitter"
)

func TestTsxJsxTextEqualsLockedC(t *testing.T) {
	cLang, err := COracleLanguage("tsx")
	if err != nil {
		t.Fatal(err)
	}
	goLang := grammars.TsxLanguage()
	for _, test := range []struct {
		name   string
		source string
	}{
		{name: "spaced", source: "const a = <p>a = b</p>;\n"},
		{name: "tight", source: "const a = <code>k=v</code>;\n"},
		{name: "after_attribute", source: "const a = <p className = \"x\">a = b</p>;\n"},
	} {
		t.Run(test.name, func(t *testing.T) {
			source := []byte(test.source)
			cParser := sitter.NewParser()
			defer cParser.Close()
			if err := cParser.SetLanguage(cLang); err != nil {
				t.Fatal(err)
			}
			cTree := cParser.Parse(source, nil)
			if cTree == nil || cTree.RootNode() == nil {
				t.Fatal("locked C parse returned no root")
			}
			defer cTree.Close()
			cRoot := cTree.RootNode()
			if cRoot.HasError() || cRoot.EndByte() != uint(len(source)) {
				t.Fatalf("locked C parse is incomplete: %s", dumpCTree(cRoot, 0))
			}

			goTree, err := gotreesitter.NewParser(goLang).Parse(source)
			if err != nil {
				t.Fatal(err)
			}
			defer goTree.Release()
			goRoot := goTree.RootNode()
			if goRoot == nil {
				t.Fatal("Go parse returned no root")
			}
			if goRoot.HasError() || goRoot.EndByte() != uint32(len(source)) {
				t.Fatalf("Go parse is incomplete: %s", goRoot.SExpr(goLang))
			}

			var mismatches []string
			compareNodes(goRoot, goLang, cRoot, "root", &mismatches)
			if len(mismatches) != 0 {
				t.Fatalf("Go and locked C trees differ:\n%s", strings.Join(mismatches, "\n"))
			}
		})
	}
}
