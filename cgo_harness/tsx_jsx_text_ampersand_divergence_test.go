//go:build cgo && treesitter_c_parity

package cgoharness

import (
	"testing"

	gotreesitter "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
	sitter "github.com/tree-sitter/go-tree-sitter"
)

// TestTsxJsxTextAmpersandKnownDivergenceFromLockedC documents an accepted,
// deliberate parity divergence for gotreesitter issue #1242.
//
// A bare '&' in TSX/JSX text (for example "Org & Team") is legal JSXText:
// tsc 5.9.3 accepts it, and the grammar excludes only '{', '<', '>', '}'.
// The pinned C oracle (tree-sitter/tree-sitter-typescript at the commit in
// grammars/languages.lock) still rejects it, because upstream's
// common/scanner.h unconditionally stops scan_jsx_text at any '&' -- the
// same defect tracked upstream as tree-sitter-javascript#366, still open.
//
// gotreesitter now matches tsc instead of the buggy C oracle here: the Go
// external scanner only stops at '&' when a full html_character_reference
// follows (see grammars/runtime/tsx_scanner.go, tsxScanHTMLCharacterReference).
// This test locks that choice in both directions:
//   - the Go parse must stay clean (the actual fix);
//   - the C oracle must still error (so this test fails loudly, prompting a
//     re-review, if a future languages.lock bump ever pulls in an upstream
//     fix for tree-sitter-javascript#366 -- at that point this divergence
//     note, and the matching skip in the broad parity boards, should be
//     removed instead of carried forward).
func TestTsxJsxTextAmpersandKnownDivergenceFromLockedC(t *testing.T) {
	cLang, err := COracleLanguage("tsx")
	if err != nil {
		t.Fatal(err)
	}
	goLang := grammars.TsxLanguage()

	for _, test := range []struct {
		name   string
		source string
	}{
		{name: "spaced", source: "const a = <p>Org & Team</p>;\n"},
		{name: "tight", source: "const a = <p>AT&T</p>;\n"},
		{name: "alone", source: "const a = <p>&</p>;\n"},
	} {
		t.Run(test.name, func(t *testing.T) {
			source := []byte(test.source)

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
				t.Fatalf("gotreesitter regressed issue #1242: parse is incomplete: %s", goRoot.SExpr(goLang))
			}

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
			if !cRoot.HasError() {
				t.Fatalf("locked C oracle now accepts a bare '&' in JSX text; " +
					"upstream tree-sitter-javascript#366 looks fixed at the pinned commit -- " +
					"remove this divergence note and re-check parity boards for gotreesitter issue #1242")
			}
		})
	}
}
