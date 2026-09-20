//go:build cgo && treesitter_c_parity

package cgoharness

import (
	"fmt"
	"strings"
	"testing"
	"time"

	sitter "github.com/tree-sitter/go-tree-sitter"

	gotreesitter "github.com/odvcencio/gotreesitter"
)

func TestCSharpGrammargenCGORegressionCases(t *testing.T) {
	var grammar grammargenCGOGrammar
	found := false
	for _, g := range grammargenCGOGrammars {
		if g.name == "c_sharp" {
			grammar = g
			found = true
			break
		}
	}
	if !found {
		t.Fatal("missing c_sharp grammargen CGO config")
	}

	gram, err := importGrammargenSource(grammar)
	if err != nil {
		t.Skipf("import unavailable: %v", err)
	}
	timeout := grammar.genTimeout
	if timeout == 0 {
		timeout = 30 * time.Second
	}
	genLang, err := grammargenGenerate(gram, timeout)
	if err != nil {
		t.Fatalf("generate grammar: %v", err)
	}
	refLang := grammar.blobFunc()
	if refLang != nil && refLang.ExternalScanner != nil && len(genLang.ExternalSymbols) > 0 {
		if scanner, ok := gotreesitter.AdaptExternalScannerByExternalOrder(refLang, genLang); ok {
			genLang.ExternalScanner = scanner
		}
	}

	cLang, err := ParityCLanguage("c_sharp")
	if err != nil {
		t.Skipf("C parser unavailable: %v", err)
	}
	cParser := sitter.NewParser()
	defer cParser.Close()
	if err := cParser.SetLanguage(cLang); err != nil {
		t.Fatalf("C SetLanguage: %v", err)
	}

	genParser := gotreesitter.NewParser(genLang)
	blobParser := gotreesitter.NewParser(refLang)

	cases := []struct {
		name string
		src  string
		// knownDivergence names a Go-versus-C divergence that a separate lane
		// owns. The case then requires the divergence to stay present, so the
		// pin fails loudly when the lane closes it.
		knownDivergence string
	}{
		{name: "contextual_file_invocation", src: "file.Method(1, 2);\n"},
		{
			name: "collection_expression_trailing_comma",
			src:  "var x = [ y, ];\n",
			// C keeps collection_expression; Go keeps bracketed_argument_list
			// (element_binding_expression). Both engines merge the two
			// derivations at the same state on a dynamic-precedence tie, but C
			// seats the last conflict action as the incumbent while Go seats the
			// first. The conflict-fork ordering lane owns the fix; the host pin is
			// TestCSharpCollectionExpressionTrailingCommaKnownDivergence.
			knownDivergence: "element_binding_expression",
		},
		{name: "conditional_access_in_if", src: "if (a?.B != 1) { }\n"},
		{name: "conditional_element_access", src: "var x = dict?[\"a\"];\n"},
		{name: "dereference_vs_logical_and", src: "bool c = (a) && b;\n"},
		{name: "is_operator_vs_conditional_expression", src: "int a = 1 is Object ? 1 : 2;\n"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			src := []byte(tc.src)

			cTree := cParser.Parse(src, nil)
			if cTree == nil || cTree.RootNode() == nil {
				t.Fatal("C nil tree")
			}
			defer cTree.Close()
			cRoot := cTree.RootNode()
			if cRoot.HasError() {
				t.Fatalf("C root has error:\n%s", dumpCTree(cRoot, 0))
			}

			genTree, err := genParser.Parse(src)
			if err != nil {
				t.Fatalf("generated parse error: %v", err)
			}
			genRoot := genTree.RootNode()

			blobTree, err := blobParser.Parse(src)
			if err != nil {
				t.Fatalf("blob parse error: %v", err)
			}
			blobRoot := blobTree.RootNode()

			var genVsCErrs []string
			compareNodes(genRoot, genLang, cRoot, "root", &genVsCErrs)
			if tc.knownDivergence != "" {
				genSExpr := genRoot.SExpr(genLang)
				blobSExpr := blobRoot.SExpr(refLang)
				if len(genVsCErrs) == 0 || !strings.Contains(genSExpr, tc.knownDivergence) || genSExpr != blobSExpr {
					t.Fatalf(
						"known divergence %q did not reproduce (close the pin and TestCSharpCollectionExpressionTrailingCommaKnownDivergence together):\ngenerated:\n%s\n\nblob:\n%s\n\nc:\n%s",
						tc.knownDivergence, genSExpr, blobSExpr, dumpCTree(cRoot, 0),
					)
				}
				t.Logf("known divergence present: Go keeps %s, C keeps collection_expression (conflict-fork ordering lane)", tc.knownDivergence)
				return
			}
			if len(genVsCErrs) == 0 {
				return
			}

			var genVsBlobErrs []string
			compareGoTreesForLangs(genRoot, genLang, blobRoot, refLang, "root", &genVsBlobErrs)

			var blobVsCErrs []string
			compareNodes(blobRoot, refLang, cRoot, "root", &blobVsCErrs)

			t.Fatalf(
				"generated-vs-C divergences:\n%s\n\ngenerated-vs-blob:\n%s\n\nblob-vs-C divergences:\n%s\n\ngenerated:\n%s\n\nblob:\n%s\n\nc:\n%s",
				joinTopErrors(genVsCErrs),
				joinTopErrors(genVsBlobErrs),
				joinTopErrors(blobVsCErrs),
				genRoot.SExpr(genLang),
				blobRoot.SExpr(refLang),
				dumpCTree(cRoot, 0),
			)
		})
	}
}

func joinTopErrors(errs []string) string {
	if len(errs) == 0 {
		return "(none)"
	}
	if len(errs) > 5 {
		errs = errs[:5]
	}
	return strings.Join(errs, "\n")
}

func compareGoTreesForLangs(left *gotreesitter.Node, leftLang *gotreesitter.Language, right *gotreesitter.Node, rightLang *gotreesitter.Language, path string, errs *[]string) {
	if len(*errs) > 0 {
		return
	}
	if left == nil || right == nil {
		if left != nil || right != nil {
			*errs = append(*errs, path+": nil mismatch")
		}
		return
	}
	if got, want := left.Type(leftLang), right.Type(rightLang); got != want {
		*errs = append(*errs, path+`: Type left="`+got+`" right="`+want+`"`)
		return
	}
	if got, want := left.ChildCount(), right.ChildCount(); got != want {
		*errs = append(*errs, fmt.Sprintf("%s: ChildCount left=%d right=%d", path, got, want))
		return
	}
	for i := 0; i < left.ChildCount(); i++ {
		compareGoTreesForLangs(left.Child(i), leftLang, right.Child(i), rightLang, fmt.Sprintf("%s[%d]", path, i), errs)
		if len(*errs) > 0 {
			return
		}
	}
}
