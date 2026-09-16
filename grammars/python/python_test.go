package python

import (
	"os/exec"
	"strings"
	"testing"

	gotreesitter "github.com/odvcencio/gotreesitter"
)

func TestPackageDoesNotDependOnAggregateGrammars(t *testing.T) {
	out, err := exec.Command("go", "list", "-deps", ".").Output()
	if err != nil {
		t.Fatal(err)
	}
	for _, dependency := range strings.Fields(string(out)) {
		if dependency == "github.com/odvcencio/gotreesitter/grammars" {
			t.Fatal("standalone Python package imports the aggregate grammar registry")
		}
	}
}

func TestLanguageLoadsWithoutAggregateRegistry(t *testing.T) {
	lang := Language()
	if lang == nil || lang.Name != "python" {
		t.Fatalf("Language() = %v, want named Python language", lang)
	}
	if Language() != lang {
		t.Fatal("Language() did not return the cached language")
	}
	if lang.ExternalScanner == nil || len(lang.ExternalLexStates) == 0 {
		t.Fatal("standalone Python language lacks external scanner support")
	}
	if lang.ExternalScannerFullParseRetryPolicy != gotreesitter.ExternalScannerFullParseRetrySkipRepeat {
		t.Fatal("standalone Python language lacks its certified retry policy")
	}
	if !lang.CompactConvergedReductionSplitDropsCertified ||
		!lang.CompactPrimaryAcceptanceDerivationCertified ||
		!lang.CompactAcceptanceStructuralElectionCertified ||
		!lang.CompactMixedGSSMergeCertified {
		t.Fatal("standalone Python language lacks its certified compact profile")
	}
}

func TestLanguageParsesScannerBackedSource(t *testing.T) {
	source := []byte("def f():\n    values = [1, 2]\n    return f\"{*values,}\"\n")
	parser := gotreesitter.NewParser(Language())
	tree, err := parser.ParseStrict(source)
	if err != nil {
		t.Fatal(err)
	}
	defer tree.Release()
	root := tree.RootNode()
	if root == nil || root.HasError() || root.EndByte() != uint32(len(source)) {
		t.Fatalf("standalone Python root = %+v", root)
	}
}
