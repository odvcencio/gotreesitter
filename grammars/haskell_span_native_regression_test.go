//go:build !grammar_subset

package grammars

import (
	"testing"

	gotreesitter "github.com/odvcencio/gotreesitter"
)

const haskellSpanRetirementSource = "module M where\nimport A\nimport B\n\nvalue = 1"

func TestHaskellSectionSpansNeedNoResultCompatibility(t *testing.T) {
	source := []byte(haskellSpanRetirementSource + "\n")
	tree, err := gotreesitter.NewParser(HaskellLanguage()).
		ParseNoResultCompatibilityBenchmarkOnly(source)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(tree.Release)

	assertHaskellSectionSpans(t, tree, source)
	runtime := tree.ParseRuntime()
	if runtime.NormalizationPassesChecked != 0 ||
		runtime.NormalizationPassesRun != 0 ||
		runtime.NormalizationNodesVisited != 0 ||
		runtime.NormalizationNodesRewritten != 0 {
		t.Fatalf(
			"normalization checked/run/visited/rewritten=%d/%d/%d/%d",
			runtime.NormalizationPassesChecked,
			runtime.NormalizationPassesRun,
			runtime.NormalizationNodesVisited,
			runtime.NormalizationNodesRewritten,
		)
	}
}

func TestHaskellSectionSpanRoutes(t *testing.T) {
	gotreesitter.ResetAdmissionCandidateCounters()
	language := HaskellLanguage()
	source := []byte(haskellSpanRetirementSource + "\n")

	productionParser := gotreesitter.NewParser(language)
	productionParser.SetAdmissionCandidateRoute(false)
	production, err := productionParser.Parse(source)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(production.Release)
	assertHaskellSectionSpans(t, production, source)

	routedBefore, fallbackBefore := gotreesitter.AdmissionCandidateCounters()
	compactParser := gotreesitter.NewParser(language)
	compactParser.SetAdmissionCandidateRoute(true)
	compactFallback, err := compactParser.Parse(source)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(compactFallback.Release)
	routedAfter, fallbackAfter := gotreesitter.AdmissionCandidateCounters()
	if routedAfter != routedBefore+1 || fallbackAfter != fallbackBefore {
		t.Fatalf(
			"compact counters routed=%d/%d fallback=%d/%d",
			routedBefore,
			routedAfter,
			fallbackBefore,
			fallbackAfter,
		)
	}
	assertHaskellSectionSpans(t, compactFallback, source)

	baseSource := []byte(haskellSpanRetirementSource)
	oldTree, err := productionParser.Parse(baseSource)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(oldTree.Release)
	endPoint := retiredDispatchPointAtByte(baseSource, len(baseSource))
	oldTree.Edit(gotreesitter.InputEdit{
		StartByte:   uint32(len(baseSource)),
		OldEndByte:  uint32(len(baseSource)),
		NewEndByte:  uint32(len(source)),
		StartPoint:  endPoint,
		OldEndPoint: endPoint,
		NewEndPoint: gotreesitter.Point{Row: endPoint.Row + 1},
	})
	incremental, profile, err := productionParser.ParseIncrementalProfiled(source, oldTree)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(incremental.Release)
	assertHaskellSectionSpans(t, incremental, source)
	if !profile.ReuseUnsupported ||
		profile.ReuseUnsupportedReason != "external_scanner_unsupported" {
		t.Fatalf("incremental reuse status = %+v", profile)
	}
}

func assertHaskellSectionSpans(
	t *testing.T,
	tree *gotreesitter.Tree,
	source []byte,
) {
	t.Helper()
	language := HaskellLanguage()
	root := tree.RootNode()
	if root == nil || root.HasError() {
		t.Fatalf("unexpected Haskell root: %v", root)
	}
	if got, want := root.EndByte(), uint32(len(source)); got != want {
		t.Fatalf("root end = %d, want %d", got, want)
	}

	imports := directNamedChildByType(root, language, "imports")
	if imports == nil {
		t.Fatalf("missing imports: %s", root.SExpr(language))
	}
	if got, want := imports.StartByte(), uint32(15); got != want {
		t.Fatalf("imports start = %d, want %d", got, want)
	}
	if got, want := imports.EndByte(), uint32(34); got != want {
		t.Fatalf("imports end = %d, want %d", got, want)
	}

	declarations := directNamedChildByType(root, language, "declarations")
	if declarations == nil {
		t.Fatalf("missing declarations: %s", root.SExpr(language))
	}
	if got, want := declarations.StartByte(), uint32(34); got != want {
		t.Fatalf("declarations start = %d, want %d", got, want)
	}
	if got, want := declarations.EndByte(), uint32(len(source)); got != want {
		t.Fatalf("declarations end = %d, want %d", got, want)
	}
}

func directNamedChildByType(
	root *gotreesitter.Node,
	language *gotreesitter.Language,
	nodeType string,
) *gotreesitter.Node {
	for index := 0; index < root.NamedChildCount(); index++ {
		child := root.NamedChild(index)
		if child != nil && child.Type(language) == nodeType {
			return child
		}
	}
	return nil
}
