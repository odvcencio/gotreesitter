//go:build cgo && treesitter_c_parity

package cgoharness

import (
	"testing"

	gotreesitter "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
	sitter "github.com/tree-sitter/go-tree-sitter"
)

// TestBladeCustomContainerMismatchedEndTagCOracle checks that reductions keep hidden MISSING end tags.
func TestBladeCustomContainerMismatchedEndTagCOracle(t *testing.T) {
	cLanguage, err := COracleLanguage("blade")
	if err != nil {
		t.Fatalf("COracleLanguage(blade): %v", err)
	}
	goLang := grammars.BladeLanguage()

	for _, input := range []string{"<x-alert>hi</span>", "<div>hi</span>"} {
		t.Run(inputSubtestName(input), func(t *testing.T) {
			src := []byte(input)

			cParser := sitter.NewParser()
			defer cParser.Close()
			if err := cParser.SetLanguage(cLanguage); err != nil {
				t.Fatalf("SetLanguage: %v", err)
			}
			cTree := cParser.Parse(src, nil)
			defer cTree.Close()
			cRoot := cTree.RootNode()
			if !cRoot.HasError() {
				t.Fatalf("blade C oracle root has no error for %q:\n%s", input, dumpCTree(cRoot, 0))
			}

			goParser := gotreesitter.NewParser(goLang)
			goTree, err := goParser.Parse(src)
			if err != nil {
				t.Fatalf("Go Parse: %v", err)
			}
			defer goTree.Release()
			goRoot := goTree.RootNode()

			if !goRoot.HasError() {
				t.Fatalf("blade Go root has no error for %q (the MISSING _implicit_end_tag leaf vanished):\n%s", input, dumpGoTree(goRoot, goLang, 0))
			}

			// SExpr omits the MISSING prefix for named nodes, so compare structure.
			cShape := cOracleShape(cRoot)
			goShapeV := goShape(goRoot, goLang)
			if cShape.kind != goShapeV.kind || cShape.children != goShapeV.children ||
				cShape.hasError != goShapeV.hasError || cShape.start != goShapeV.start || cShape.end != goShapeV.end {
				t.Fatalf("blade %q: C and Go disagree on shape:\n  C:  %+v\n  Go: %+v", input, cShape, goShapeV)
			}

			implicitEnd := findBladeGoNodeByType(goRoot, goLang, "_implicit_end_tag")
			if implicitEnd == nil {
				t.Fatalf("blade %q: no _implicit_end_tag node found; expected a MISSING one", input)
			}
			if !implicitEnd.IsMissing() {
				t.Fatalf("blade %q: _implicit_end_tag node is present but not marked missing", input)
			}
		})
	}
}
