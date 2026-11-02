//go:build cgo && treesitter_c_parity

package cgoharness

import (
	"encoding/json"
	"os"
	"testing"

	gotreesitter "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
)

func TestMakeFunctionVariablesCOracle(t *testing.T) {
	data, err := os.ReadFile("../grammars/testdata/make_function_variables.json")
	if err != nil {
		t.Fatal(err)
	}
	var cases []struct {
		Name, Source string
		Error        bool
	}
	if err := json.Unmarshal(data, &cases); err != nil {
		t.Fatal(err)
	}
	cLanguage, err := ParityCLanguage("make")
	if err != nil {
		t.Fatalf("C make oracle unavailable: %v", err)
	}
	goLanguage := grammars.MakeLanguage()
	for _, tc := range cases {
		t.Run(tc.Name, func(t *testing.T) {
			source := []byte(tc.Source)
			cTree := compactT3ParseC(t, cLanguage, source)
			defer cTree.Close()
			goTree, err := gotreesitter.NewParser(goLanguage).Parse(source)
			if err != nil {
				t.Fatal(err)
			}
			defer goTree.Release()
			cRoot, goRoot := cTree.RootNode(), goTree.RootNode()
			if cRoot == nil || goRoot == nil {
				t.Fatal("missing root")
			}
			if cRoot.HasError() != tc.Error || goRoot.HasError() != tc.Error {
				t.Fatalf("expected HasError=%v; C: %s; Go: %s", tc.Error, cRoot.ToSexp(), goRoot.SExpr(goLanguage))
			}
			if cRoot.EndByte() != uint(len(source)) || goRoot.EndByte() != uint32(len(source)) {
				t.Fatal("root does not cover source")
			}
			var errs []string
			compareNodes(goRoot, goLanguage, cRoot, "root", &errs)
			if len(errs) != 0 {
				t.Fatalf("Go/C trees diverged: %s", joinTopErrors(errs))
			}
		})
	}
}
