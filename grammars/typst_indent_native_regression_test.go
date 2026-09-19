//go:build !grammar_subset

package grammars

import (
	"testing"

	gotreesitter "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/internal/benchfixtures"
)

// typstNestedListSource comes from typst/list/list-nested in the locked
// tree-sitter-typst corpus.
const typstNestedListSource = `- First level.

  - Second level.
    There are multiple paragraphs.

    - Third level.

    Still the same bullet point.

  - Still level 2.

- At the top.

`

const typstNestedListSExpr = `(source_file (item (text) (parbreak) (item (text) (text) (parbreak) (item (text)) (parbreak) (text)) (parbreak) (item (text))) (parbreak) (item (text)) (parbreak))`

const typstNestedListDigest = "d6a6dc404934db691499e7c80f719996ad1a94313fd9a7163bfaf7caac177632"

func TestTypstNestedListNeedsNoResultCompatibility(t *testing.T) {
	source := []byte(typstNestedListSource)
	language := TypstLanguage()
	tree, err := gotreesitter.NewParser(language).
		ParseNoResultCompatibilityBenchmarkOnly(source)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(tree.Release)

	assertTypstNestedListTree(t, "compatibility-free", tree, language, source)
}

func TestTypstNestedListNativeRoutes(t *testing.T) {
	gotreesitter.ResetAdmissionCandidateCounters()
	source := []byte(typstNestedListSource)
	language := TypstLanguage()

	productionParser := gotreesitter.NewParser(language)
	productionParser.SetAdmissionCandidateRoute(false)
	production, err := productionParser.Parse(source)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(production.Release)
	assertTypstNestedListTree(t, "production", production, language, source)

	routedBefore, fallbackBefore := gotreesitter.AdmissionCandidateCounters()
	compactParser := gotreesitter.NewParser(language)
	compactParser.SetAdmissionCandidateRoute(true)
	compact, err := compactParser.Parse(source)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(compact.Release)
	routedAfter, fallbackAfter := gotreesitter.AdmissionCandidateCounters()
	if routedAfter != routedBefore+1 || fallbackAfter != fallbackBefore {
		t.Fatalf(
			"compact route counters routed=%d/%d fallback=%d/%d: %s",
			routedBefore,
			routedAfter,
			fallbackBefore,
			fallbackAfter,
			gotreesitter.AdmissionCandidateLastFallbackReason(),
		)
	}
	assertTypstNestedListTree(t, "compact", compact, language, source)

	forestParser := gotreesitter.NewParser(language)
	forest, ok := forestParser.ParseForestExperimental(source)
	if !ok || forest == nil {
		offset, symbol, reason, _ := forestParser.ForestDeclineInfo()
		t.Fatalf("forest declined at %d symbol=%d reason=%s", offset, symbol, reason)
	}
	t.Cleanup(forest.Release)
	if !forest.ParseRuntime().ForestFastPath {
		t.Fatal("forest parse did not report the forest route")
	}
	assertTypstNestedListTree(t, "forest", forest, language, source)

	baseSource := append([]byte(nil), source...)
	baseSource[2] = 'f'
	incrementalParser := gotreesitter.NewParser(language)
	incrementalParser.SetAdmissionCandidateRoute(false)
	oldTree, err := incrementalParser.Parse(baseSource)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(oldTree.Release)
	oldTree.Edit(gotreesitter.InputEdit{
		StartByte:   2,
		OldEndByte:  3,
		NewEndByte:  3,
		StartPoint:  gotreesitter.Point{Column: 2},
		OldEndPoint: gotreesitter.Point{Column: 3},
		NewEndPoint: gotreesitter.Point{Column: 3},
	})
	incremental, profile, err := incrementalParser.ParseIncrementalProfiled(source, oldTree)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(incremental.Release)
	if !profile.ReuseUnsupported ||
		profile.ReuseUnsupportedReason != "external_scanner_unsupported" {
		t.Fatalf("incremental reuse status = %+v", profile)
	}
	assertTypstNestedListTree(t, "incremental", incremental, language, source)
}

func assertTypstNestedListTree(
	t *testing.T,
	route string,
	tree *gotreesitter.Tree,
	language *gotreesitter.Language,
	source []byte,
) {
	t.Helper()
	root := tree.RootNode()
	if root == nil || root.HasError() {
		t.Fatalf("%s root = %v, want a clean tree", route, root)
	}
	if got := root.SExpr(language); got != typstNestedListSExpr {
		t.Fatalf("%s tree = %s, want %s", route, got, typstNestedListSExpr)
	}
	if got, want := root.StartByte(), uint32(0); got != want {
		t.Fatalf("%s root start = %d, want %d", route, got, want)
	}
	if got, want := root.EndByte(), uint32(len(source)); got != want {
		t.Fatalf("%s root end = %d, want %d", route, got, want)
	}
	inspection, err := benchfixtures.InspectGoTree(root, language)
	if err != nil {
		t.Fatal(err)
	}
	if got := inspection.SHA256; got != typstNestedListDigest {
		t.Fatalf("%s digest = %s, want %s", route, got, typstNestedListDigest)
	}
}
