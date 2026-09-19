package grammars_test

import (
	"bytes"
	"testing"

	gotreesitter "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
	"github.com/odvcencio/gotreesitter/internal/benchfixtures"
)

func TestCSharpIssue454RecoveredStructureNeedsNoSourceReconstruction(t *testing.T) {
	gotreesitter.ResetAdmissionCandidateCounters()
	lang := grammars.CSharpLanguage()
	source := issue454CSharpSource(137 * 1024)
	site := bytes.Index(source, []byte("x0"))
	if site < 0 {
		t.Fatal("C# edit marker is absent")
	}
	source = append(append([]byte(nil), source[:site]...), source[site+1:]...)

	rawParser := gotreesitter.NewParser(lang)
	rawParser.SetAdmissionCandidateRoute(false)
	raw, err := rawParser.ParseNoResultCompatibilityBenchmarkOnly(source)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(raw.Release)

	productionParser := gotreesitter.NewParser(lang)
	productionParser.SetAdmissionCandidateRoute(false)
	production, err := productionParser.Parse(source)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(production.Release)

	want := csharpRecoveredStructureInspection(t, raw, lang)
	got := csharpRecoveredStructureInspection(t, production, lang)
	if got.SHA256 != want.SHA256 {
		t.Fatalf("production digest = %s, raw digest = %s", got.SHA256, want.SHA256)
	}
	assertCSharpIssue454RecoveredStructure(t, production.RootNode(), lang)

	compactSource := issue454CSharpSource(16 * 1024)
	compactSite := bytes.Index(compactSource, []byte("x0"))
	if compactSite < 0 {
		t.Fatal("compact C# edit marker is absent")
	}
	compactSource = append(
		append([]byte(nil), compactSource[:compactSite]...),
		compactSource[compactSite+1:]...,
	)
	compactProductionParser := gotreesitter.NewParser(lang)
	compactProductionParser.SetAdmissionCandidateRoute(false)
	compactProduction, err := compactProductionParser.Parse(compactSource)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(compactProduction.Release)
	compactWant := csharpRecoveredStructureInspection(t, compactProduction, lang)

	routedBefore, fallbackBefore := gotreesitter.AdmissionCandidateCounters()
	compactParser := gotreesitter.NewParser(lang)
	compactParser.SetAdmissionCandidateRoute(true)
	compact, err := compactParser.Parse(compactSource)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(compact.Release)
	routedAfter, fallbackAfter := gotreesitter.AdmissionCandidateCounters()
	if routedAfter != routedBefore || fallbackAfter != fallbackBefore+1 {
		t.Fatalf(
			"compact fallback counters routed=%d/%d fallback=%d/%d: %s",
			routedBefore,
			routedAfter,
			fallbackBefore,
			fallbackAfter,
			gotreesitter.AdmissionCandidateLastFallbackReason(),
		)
	}
	compactInspection := csharpRecoveredStructureInspection(t, compact, lang)
	if compactInspection.SHA256 != compactWant.SHA256 {
		t.Fatalf(
			"compact fallback digest = %s, production digest = %s",
			compactInspection.SHA256,
			compactWant.SHA256,
		)
	}
}

func csharpRecoveredStructureInspection(
	t *testing.T,
	tree *gotreesitter.Tree,
	lang *gotreesitter.Language,
) benchfixtures.DeepTreeInspection {
	t.Helper()
	if tree == nil || tree.RootNode() == nil {
		t.Fatal("parse returned no root")
	}
	inspection, err := benchfixtures.InspectGoTree(tree.RootNode(), lang)
	if err != nil {
		t.Fatal(err)
	}
	return inspection
}

func assertCSharpIssue454RecoveredStructure(
	t *testing.T,
	root *gotreesitter.Node,
	lang *gotreesitter.Language,
) {
	t.Helper()
	if root == nil || !root.HasError() {
		t.Fatal("recovered root did not retain the localized parse error")
	}
	counts := make(map[string]int)
	var walk func(*gotreesitter.Node)
	walk = func(node *gotreesitter.Node) {
		if node == nil {
			return
		}
		counts[node.Type(lang)]++
		for i := 0; i < int(node.ChildCount()); i++ {
			walk(node.Child(i))
		}
	}
	walk(root)
	if counts["namespace_declaration"] != 1 {
		t.Fatalf("namespace declarations = %d, want 1", counts["namespace_declaration"])
	}
	if counts["class_declaration"] < 1500 {
		t.Fatalf("class declarations = %d, want at least 1500", counts["class_declaration"])
	}
	if counts["method_declaration"] != counts["class_declaration"] {
		t.Fatalf(
			"method declarations = %d, class declarations = %d",
			counts["method_declaration"],
			counts["class_declaration"],
		)
	}
	if counts["ERROR"] != 1 {
		t.Fatalf("ERROR nodes = %d, want 1", counts["ERROR"])
	}
}
