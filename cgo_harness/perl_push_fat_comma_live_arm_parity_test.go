//go:build cgo && treesitter_c_parity

package cgoharness

import (
	"strings"
	"testing"

	sitter "github.com/tree-sitter/go-tree-sitter"

	gotreesitter "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
)

// TestPerlPushFatCommaLiveArmLockedCParity records why the Perl
// result-compatibility arm stays. For push with a fat comma after the first
// argument, the engine's own tree differs from locked C, and the production
// tree (with the arm) matches it. The comma form is covered by
// TestPerlSchedulerActionLoadBearingCOracleParity. Retire the arm only after
// the raw tree matches C for these inputs.
func TestPerlPushFatCommaLiveArmLockedCParity(t *testing.T) {
	goLang := grammars.PerlLanguage()
	cLang, err := COracleLanguage("perl")
	if err != nil {
		t.Fatal(err)
	}

	for _, source := range []string{
		"push @a => 1;\n",
		"push @a => {x=>1};\n",
		"push @a => $b, $c;\n",
	} {
		source := source
		t.Run(source, func(t *testing.T) {
			scheduleParityMemoryScavenge(t)
			src := []byte(source)

			goParser := gotreesitter.NewParser(goLang)
			goParser.SetAdmissionCandidateRoute(false)
			rawTree, err := goParser.ParseNoResultCompatibilityBenchmarkOnly(src)
			if err != nil {
				t.Fatalf("raw parse: %v", err)
			}
			defer rawTree.Release()

			prodParser := gotreesitter.NewParser(goLang)
			prodParser.SetAdmissionCandidateRoute(false)
			prodTree, err := prodParser.Parse(src)
			if err != nil {
				t.Fatalf("production parse: %v", err)
			}
			defer prodTree.Release()

			cParser := sitter.NewParser()
			defer cParser.Close()
			if err := cParser.SetLanguage(cLang); err != nil {
				t.Fatal(err)
			}
			cTree := cParser.Parse(src, nil)
			if cTree == nil || cTree.RootNode() == nil {
				t.Fatal("C parse returned a nil tree")
			}
			defer cTree.Close()

			var rawVsC, prodVsC []string
			compareNodes(rawTree.RootNode(), goLang, cTree.RootNode(), "root", &rawVsC)
			compareNodes(prodTree.RootNode(), goLang, cTree.RootNode(), "root", &prodVsC)

			if len(prodVsC) != 0 {
				t.Fatalf("production tree diverges from the C oracle: %s", strings.Join(prodVsC, " | "))
			}
			if len(rawVsC) == 0 {
				t.Fatalf("raw engine tree already matches C; the Perl arm may be retirable for this input, re-run the full Perl receipts")
			}
		})
	}
}
