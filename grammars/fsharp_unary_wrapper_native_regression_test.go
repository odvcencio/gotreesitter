//go:build !grammar_subset

package grammars

import (
	"testing"

	gotreesitter "github.com/odvcencio/gotreesitter"
)

var fsharpUnaryWrapperFixture = []byte(
	"open Namespace.Module\n" +
		"let simple = Alpha\n",
)

func TestFSharpUnaryWrapperProducerNeedsNoResultCompatibility(t *testing.T) {
	t.Setenv("GTS_DISPATCHER_CENSUS", "1")
	language := FsharpLanguage()
	tree, err := gotreesitter.NewParser(language).
		ParseNoResultCompatibilityBenchmarkOnly(fsharpUnaryWrapperFixture)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(tree.Release)
	assertFSharpUnaryWrapperTree(t, "compatibility-free", tree, language, fsharpUnaryWrapperFixture)
	assertNoNormalizationPasses(t, tree)

	production, err := gotreesitter.NewParser(language).Parse(fsharpUnaryWrapperFixture)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(production.Release)
	assertFSharpUnaryWrapperTree(t, "production", production, language, fsharpUnaryWrapperFixture)
	if runtime := production.ParseRuntime(); runtime.NormalizationNodesRewritten != 0 {
		t.Fatalf("production normalization rewrote %d nodes", runtime.NormalizationNodesRewritten)
	}
	if got, want := collapsedTokenTreeDigest(t, tree, language), collapsedTokenTreeDigest(t, production, language); got != want {
		t.Fatalf("compatibility-free digest = %s, want production %s", got, want)
	}
}

func TestFSharpUnaryWrapperRoutesStayExactOrFailClosed(t *testing.T) {
	language := FsharpLanguage()
	source := append([]byte(nil), fsharpUnaryWrapperFixture...)

	productionParser := gotreesitter.NewParser(language)
	productionParser.SetAdmissionCandidateRoute(false)
	production, err := productionParser.Parse(source)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(production.Release)
	assertFSharpUnaryWrapperTree(t, "production", production, language, source)
	want := collapsedTokenTreeDigest(t, production, language)

	routedBefore, fallbackBefore := gotreesitter.AdmissionCandidateCounters()
	compactParser := gotreesitter.NewParser(language)
	compactParser.SetAdmissionCandidateRoute(true)
	compact, err := compactParser.Parse(source)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(compact.Release)
	assertFSharpUnaryWrapperTree(t, "compact", compact, language, source)
	if got := collapsedTokenTreeDigest(t, compact, language); got != want {
		t.Fatalf("compact digest = %s, want production %s", got, want)
	}
	routedAfter, fallbackAfter := gotreesitter.AdmissionCandidateCounters()
	if routedAfter == routedBefore && fallbackAfter != fallbackBefore+1 {
		t.Fatalf(
			"compact route did not route or fail closed: routed=%d/%d fallback=%d/%d",
			routedBefore,
			routedAfter,
			fallbackBefore,
			fallbackAfter,
		)
	}

	forestParser := gotreesitter.NewParser(language)
	forest, ok := forestParser.ParseForestExperimental(source)
	if ok && forest != nil {
		t.Cleanup(forest.Release)
		assertFSharpUnaryWrapperTree(t, "forest", forest, language, source)
		if got := collapsedTokenTreeDigest(t, forest, language); got != want {
			t.Fatalf("forest digest = %s, want production %s", got, want)
		}
		if !forest.ParseRuntime().ForestFastPath {
			t.Fatal("forest parse did not report the forest route")
		}
	} else {
		_, _, reason, _ := forestParser.ForestDeclineInfo()
		if reason == "" {
			t.Fatal("forest route failed closed without a reason")
		}
	}

	incrementalParser := gotreesitter.NewParser(language)
	incrementalParser.SetAdmissionCandidateRoute(false)
	oldSource := source[:len(source)-1]
	oldTree, err := incrementalParser.Parse(oldSource)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(oldTree.Release)
	endPoint := retiredDispatchPointAtByte(oldSource, len(oldSource))
	oldTree.Edit(gotreesitter.InputEdit{
		StartByte:   uint32(len(oldSource)),
		OldEndByte:  uint32(len(oldSource)),
		NewEndByte:  uint32(len(source)),
		StartPoint:  endPoint,
		OldEndPoint: endPoint,
		NewEndPoint: gotreesitter.Point{Row: endPoint.Row + 1},
	})
	incremental, _, err := incrementalParser.ParseIncrementalProfiled(source, oldTree)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(incremental.Release)
	assertFSharpUnaryWrapperTree(t, "incremental", incremental, language, source)
	if got := collapsedTokenTreeDigest(t, incremental, language); got != want {
		t.Fatalf("incremental digest = %s, want production %s", got, want)
	}

	repeated, err := gotreesitter.NewParser(language).Parse(source)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(repeated.Release)
	if got := collapsedTokenTreeDigest(t, repeated, language); got != want {
		t.Fatalf("repeated digest = %s, want production %s", got, want)
	}
}

func assertFSharpUnaryWrapperTree(
	t *testing.T,
	route string,
	tree *gotreesitter.Tree,
	language *gotreesitter.Language,
	source []byte,
) {
	t.Helper()
	root := tree.RootNode()
	if root == nil || root.HasError() || tree.ParseStopReason() != gotreesitter.ParseStopAccepted {
		t.Fatalf("%s root = %v stop=%s", route, root, tree.ParseStopReason())
	}
	wants := map[string][]string{
		"simple": {"identifier"},
		"Alpha":  {"long_identifier"},
	}
	seen := make(map[string]int, len(wants))
	gotreesitter.Walk(root, func(node *gotreesitter.Node, depth int) gotreesitter.WalkAction {
		if depth > 0 && node.Parent() == nil {
			t.Fatalf("%s node %q at depth %d has no parent", route, node.Type(language), depth)
		}
		if node.Type(language) != "long_identifier_or_op" {
			return gotreesitter.WalkContinue
		}
		text := string(node.Text(source))
		wantChildren, ok := wants[text]
		if !ok {
			return gotreesitter.WalkContinue
		}
		seen[text]++
		if node.ChildCount() != len(wantChildren) {
			t.Fatalf("%s %q children = %d, want %d: %s", route, text, node.ChildCount(), len(wantChildren), root.SExpr(language))
		}
		for index, wantType := range wantChildren {
			if got := node.Child(index).Type(language); got != wantType {
				t.Fatalf("%s %q child %d = %q, want %q", route, text, index, got, wantType)
			}
		}
		return gotreesitter.WalkContinue
	})
	for text := range wants {
		if seen[text] != 1 {
			t.Fatalf("%s %q occurrences = %d, want 1: %s", route, text, seen[text], root.SExpr(language))
		}
	}
	dotted := findRecoveryActionMaterializationNode(root, language, "long_identifier")
	if dotted == nil || string(dotted.Text(source)) != "Namespace.Module" || dotted.ChildCount() != 3 {
		t.Fatalf("%s dotted wrapper = %v, want Namespace.Module with three children", route, dotted)
	}
}
