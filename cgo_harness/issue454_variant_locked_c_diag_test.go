//go:build cgo && treesitter_c_parity

package cgoharness

import (
	"crypto/sha256"
	"fmt"
	"os"
	"testing"

	sitter "github.com/tree-sitter/go-tree-sitter"

	gotreesitter "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
	"github.com/odvcencio/gotreesitter/internal/benchfixtures"
)

func TestIssue454VariantLockedCDiag(t *testing.T) {
	cases := []struct {
		name   string
		lang   string
		load   func() *gotreesitter.Language
		path   string
		source string
	}{
		{name: "cobol_a0_small", lang: "cobol", load: grammars.CobolLanguage, path: "../testdata/dispatcher_census_a0/cobol/small__CVERSNP1.cpy"},
		{name: "cobol_a0_medium", lang: "cobol", load: grammars.CobolLanguage, path: "../testdata/dispatcher_census_a0/cobol/medium__DBANK02P.cbl"},
		{name: "cobol_a0_large", lang: "cobol", load: grammars.CobolLanguage, path: "../testdata/dispatcher_census_a0/cobol/large__MBANK30.cpy"},
		{name: "wgsl_a0_small", lang: "wgsl", load: grammars.WgslLanguage, path: "../testdata/dispatcher_census_a0/wgsl/small__fragmentTextureQuad.wgsl"},
		{name: "wgsl_a0_medium_normal", lang: "wgsl", load: grammars.WgslLanguage, path: "../testdata/dispatcher_census_a0/wgsl/medium__normalMap.wgsl"},
		{name: "wgsl_a0_medium_radiosity", lang: "wgsl", load: grammars.WgslLanguage, path: "../testdata/dispatcher_census_a0/wgsl/medium__radiosity.wgsl"},
		{name: "cooklang_punctuation", lang: "cooklang", load: grammars.CooklangLanguage, source: "Add @salt{1%tsp}.\n"},
		{name: "cooklang_recovered", lang: "cooklang", load: grammars.CooklangLanguage, source: "---\nservings: 4\nemoji: 🥟\ntags: warm, fried, starter\n---\n\nServe hot.\n"},
		{name: "cooklang_no_newline", lang: "cooklang", load: grammars.CooklangLanguage, source: "Add @salt{1%tsp}."},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			var source []byte
			if tc.path != "" {
				var err error
				source, err = os.ReadFile(tc.path)
				if err != nil {
					t.Fatalf("read source: %v", err)
				}
			} else {
				source = []byte(tc.source)
			}
			cLang, err := ParityCLanguage(tc.lang)
			if err != nil {
				t.Fatalf("load locked C language: %v", err)
			}
			cParser := sitter.NewParser()
			defer cParser.Close()
			if err := cParser.SetLanguage(cLang); err != nil {
				t.Fatalf("set C language: %v", err)
			}
			goLang := tc.load()
			goParser := gotreesitter.NewParser(goLang)
			goParser.SetAdmissionCandidateRoute(false)
			goTree, err := goParser.ParseNoResultCompatibilityBenchmarkOnly(source)
			if err != nil {
				t.Fatalf("parse Go tree: %v", err)
			}
			defer goTree.Release()
			cTree := cParser.Parse(source, nil)
			if cTree == nil || cTree.RootNode() == nil {
				t.Fatal("C parse returned no tree")
			}
			defer cTree.Close()
			goRoot := goTree.RootNode()
			cRoot := cTree.RootNode()
			var diffs []compactT3StructuralDivergence
			compactT3WalkStructuralDivergences(goRoot, goLang, cRoot, "root", &diffs)
			goDigest := sha256.Sum256([]byte(goRoot.SExpr(goLang)))
			cDigest := sha256.Sum256([]byte(dumpCTree(cRoot, 0)))
			goInspection, err := benchfixtures.InspectGoTree(goRoot, goLang)
			if err != nil {
				t.Fatalf("inspect Go tree: %v", err)
			}
			cDeepDigest, err := COracleDeepDigest(cTree)
			if err != nil {
				t.Fatalf("inspect C tree: %v", err)
			}
			t.Logf("raw_error=%v c_error=%v raw_sexpr_sha256=%x c_tree_sha256=%x go_deep_digest=%s c_deep_digest=%s structural_diffs=%d", goRoot.HasError(), cRoot.HasError(), goDigest, cDigest, goInspection.SHA256, cDeepDigest, len(diffs))
			for i, diff := range diffs {
				if i == 8 {
					break
				}
				t.Logf("diff[%d]=%s", i, fmt.Sprint(diff))
			}
		})
	}
}
