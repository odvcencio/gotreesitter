//go:build cgo && treesitter_c_parity

package cgoharness

import (
	"testing"

	gotreesitter "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
)

// TestEditorconfigIntegerRangeExternalSymbolCOracle pins the editorconfig
// scanner's integer-range-start fix against the real C tree-sitter oracle.
// The scanner used to hardcode gotreesitter.Symbol(32) for this token, but
// the shipped editorconfig.bin blob's ExternalSymbols is [23 24]; symbol 32
// in that blob names brace_expansion, an unrelated grammar rule.
func TestEditorconfigIntegerRangeExternalSymbolCOracle(t *testing.T) {
	source := []byte("[{1..3}]\n")

	cLanguage, err := ParityCLanguage("editorconfig")
	if err != nil {
		t.Skipf("C editorconfig oracle unavailable: %v", err)
	}
	cTree := compactT3ParseC(t, cLanguage, source)
	defer cTree.Close()
	cRoot := cTree.RootNode()
	if cRoot == nil {
		t.Fatal("editorconfig C oracle returned no root")
	}
	if cRoot.HasError() {
		t.Fatalf("editorconfig C oracle root has error:\n%s", dumpCTree(cRoot, 0))
	}

	goLanguage := grammars.EditorconfigLanguage()
	goParser := gotreesitter.NewParser(goLanguage)
	goTree, err := goParser.Parse(source)
	if err != nil {
		t.Fatalf("editorconfig Go parse: %v", err)
	}
	defer goTree.Release()
	goRoot := goTree.RootNode()
	if goRoot == nil {
		t.Fatal("editorconfig Go parse returned no root")
	}
	if goRoot.HasError() {
		t.Fatalf("editorconfig Go root has error:\n%s", dumpGoTree(goRoot, goLanguage, 0))
	}

	var errs []string
	compareNodes(goRoot, goLanguage, cRoot, "root", &errs)
	if len(errs) != 0 {
		t.Fatalf(
			"editorconfig Go/C integer-range tree diverged:\n%s\n\nGo: %s\n%s\nC: %s\n%s",
			joinTopErrors(errs),
			goRoot.SExpr(goLanguage),
			dumpGoTree(goRoot, goLanguage, 0),
			cRoot.ToSexp(),
			dumpCTree(cRoot, 0),
		)
	}

	wantSExpr := "(editorconfig (section (header (glob (integer_range (integer) (integer))))))"
	if got := goRoot.SExpr(goLanguage); got != wantSExpr {
		t.Fatalf("editorconfig Go S-expression = %s, want %s", got, wantSExpr)
	}
}

// TestEditorconfigEndOfFileExternalSymbolCOracle pins the editorconfig
// scanner's end-of-file token against the real C tree-sitter oracle for a
// header and a bare property, both at end of input with no trailing
// newline.
func TestEditorconfigEndOfFileExternalSymbolCOracle(t *testing.T) {
	cLanguage, err := ParityCLanguage("editorconfig")
	if err != nil {
		t.Skipf("C editorconfig oracle unavailable: %v", err)
	}
	goLanguage := grammars.EditorconfigLanguage()

	cases := []struct {
		name   string
		source string
	}{
		{name: "header-no-trailing-newline", source: "[foo]"},
		{name: "property-no-trailing-newline", source: "key=1"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			source := []byte(tc.source)

			cTree := compactT3ParseC(t, cLanguage, source)
			defer cTree.Close()
			cRoot := cTree.RootNode()
			if cRoot == nil {
				t.Fatal("editorconfig C oracle returned no root")
			}
			if cRoot.HasError() {
				t.Fatalf("editorconfig C oracle root has error:\n%s", dumpCTree(cRoot, 0))
			}

			goParser := gotreesitter.NewParser(goLanguage)
			goTree, err := goParser.Parse(source)
			if err != nil {
				t.Fatalf("editorconfig Go parse: %v", err)
			}
			defer goTree.Release()
			goRoot := goTree.RootNode()
			if goRoot == nil {
				t.Fatal("editorconfig Go parse returned no root")
			}
			if goRoot.HasError() {
				t.Fatalf("editorconfig Go root has error:\n%s", dumpGoTree(goRoot, goLanguage, 0))
			}

			var errs []string
			compareNodes(goRoot, goLanguage, cRoot, "root", &errs)
			if len(errs) != 0 {
				t.Fatalf(
					"editorconfig Go/C end-of-file tree diverged for %q:\n%s\n\nGo: %s\n%s\nC: %s\n%s",
					tc.source,
					joinTopErrors(errs),
					goRoot.SExpr(goLanguage),
					dumpGoTree(goRoot, goLanguage, 0),
					cRoot.ToSexp(),
					dumpCTree(cRoot, 0),
				)
			}
		})
	}
}
