//go:build !grammar_subset

package grammars

import (
	"testing"

	gotreesitter "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/internal/benchfixtures"
)

type collapsedTokenRetirementCase struct {
	name           string
	language       func() *gotreesitter.Language
	source         []byte
	compactDecline bool
	assert         func(*testing.T, *gotreesitter.Tree, *gotreesitter.Language, []byte)
}

func TestCollapsedTokenProducerNeedsNoResultCompatibility(t *testing.T) {
	for _, test := range collapsedTokenRetirementCases() {
		test := test
		t.Run(test.name, func(t *testing.T) {
			language := test.language()
			tree, err := gotreesitter.NewParser(language).
				ParseNoResultCompatibilityBenchmarkOnly(test.source)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(tree.Release)
			test.assert(t, tree, language, test.source)
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
		})
	}
}

func TestCollapsedTokenRetirementRoutes(t *testing.T) {
	for _, test := range collapsedTokenRetirementCases() {
		test := test
		t.Run(test.name, func(t *testing.T) {
			language := test.language()
			source := append(append([]byte(nil), test.source...), '\n')

			productionParser := gotreesitter.NewParser(language)
			productionParser.SetAdmissionCandidateRoute(false)
			production, err := productionParser.Parse(source)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(production.Release)
			test.assert(t, production, language, source)

			routedBefore, fallbackBefore := gotreesitter.AdmissionCandidateCounters()
			compactParser := gotreesitter.NewParser(language)
			compactParser.SetAdmissionCandidateRoute(true)
			compact, err := compactParser.Parse(source)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(compact.Release)
			test.assert(t, compact, language, source)
			routedAfter, fallbackAfter := gotreesitter.AdmissionCandidateCounters()
			if test.compactDecline {
				if routedAfter != routedBefore || fallbackAfter != fallbackBefore+1 {
					t.Fatalf(
						"compact decline counters routed=%d/%d fallback=%d/%d",
						routedBefore,
						routedAfter,
						fallbackBefore,
						fallbackAfter,
					)
				}
			} else if routedAfter != routedBefore+1 || fallbackAfter != fallbackBefore {
				t.Fatalf(
					"compact route counters routed=%d/%d fallback=%d/%d",
					routedBefore,
					routedAfter,
					fallbackBefore,
					fallbackAfter,
				)
			}

			forestParser := gotreesitter.NewParser(language)
			forest, ok := forestParser.ParseForestExperimental(source)
			if !ok || forest == nil {
				offset, symbol, reason, _ := forestParser.ForestDeclineInfo()
				t.Fatalf(
					"forest declined at %d symbol=%d reason=%s",
					offset,
					symbol,
					reason,
				)
			}
			t.Cleanup(forest.Release)
			test.assert(t, forest, language, source)
			if !forest.ParseRuntime().ForestFastPath {
				t.Fatal("forest parse did not report the forest route")
			}

			incrementalParser := gotreesitter.NewParser(language)
			incrementalParser.SetAdmissionCandidateRoute(false)
			oldTree, err := incrementalParser.Parse(test.source)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(oldTree.Release)
			endPoint := retiredDispatchPointAtByte(test.source, len(test.source))
			oldTree.Edit(gotreesitter.InputEdit{
				StartByte:   uint32(len(test.source)),
				OldEndByte:  uint32(len(test.source)),
				NewEndByte:  uint32(len(source)),
				StartPoint:  endPoint,
				OldEndPoint: endPoint,
				NewEndPoint: gotreesitter.Point{Row: endPoint.Row + 1},
			})
			incremental, profile, err := incrementalParser.ParseIncrementalProfiled(source, oldTree)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(incremental.Release)
			test.assert(t, incremental, language, source)
			if profile.ReuseUnsupported {
				if profile.ReuseUnsupportedReason == "" {
					t.Fatal("incremental reuse declined without a reason")
				}
			} else if !profile.OldTreeReuseRoute ||
				profile.ReusedSubtrees == 0 ||
				profile.ReusedBytes == 0 {
				t.Fatalf("incremental route did not reuse the old tree: %+v", profile)
			}

			want := collapsedTokenTreeDigest(t, production, language)
			for route, tree := range map[string]*gotreesitter.Tree{
				"compact":     compact,
				"forest":      forest,
				"incremental": incremental,
			} {
				if got := collapsedTokenTreeDigest(t, tree, language); got != want {
					t.Fatalf("%s digest = %s, want production %s", route, got, want)
				}
			}
		})
	}
}

func collapsedTokenRetirementCases() []collapsedTokenRetirementCase {
	return []collapsedTokenRetirementCase{
		{
			name:     "hcl",
			language: HclLanguage,
			source:   []byte("a {\n b = true\n c = [false]\n d = {}\n}"),
			assert:   assertHCLCollapsedTokens,
		},
		{
			name:     "cpon",
			language: CponLanguage,
			source:   []byte(`[null,true,false]`),
			assert:   assertCPONCollapsedTokens,
		},
		{
			name:     "c_sharp",
			language: CSharpLanguage,
			source:   []byte("public class C { public bool F = false; void M() { var x = global::System.String.Empty; } }"),
			assert:   assertCSharpCollapsedTokens,
		},
		{
			name:     "powershell",
			language: PowershellLanguage,
			source:   []byte("$a = 1\n$a += 2"),
			assert:   assertPowerShellCollapsedTokens,
		},
	}
}

func assertHCLCollapsedTokens(
	t *testing.T,
	tree *gotreesitter.Tree,
	language *gotreesitter.Language,
	source []byte,
) {
	t.Helper()
	wants := map[string]map[string]int{
		"block_start":  {"{": 1},
		"block_end":    {"}": 1},
		"bool_lit":     {"true": 1, "false": 1},
		"tuple_start":  {"[": 1},
		"tuple_end":    {"]": 1},
		"object_start": {"{": 1},
		"object_end":   {"}": 1},
	}
	assertCollapsedTokenChildren(t, tree, language, source, wants)
}

func assertCPONCollapsedTokens(
	t *testing.T,
	tree *gotreesitter.Tree,
	language *gotreesitter.Language,
	source []byte,
) {
	t.Helper()
	root := collapsedTokenCleanRoot(t, tree, language)
	nodes := collapsedChildRegressionNodes(root, language, "null")
	if len(nodes) != 1 {
		t.Fatalf("null count = %d, want 1: %s", len(nodes), root.SExpr(language))
	}
	for _, node := range nodes {
		if node.ChildCount() != 0 || string(node.Text(source)) != "null" {
			t.Fatalf("null shape = children:%d text:%q", node.ChildCount(), node.Text(source))
		}
	}
}

func assertCSharpCollapsedTokens(
	t *testing.T,
	tree *gotreesitter.Tree,
	language *gotreesitter.Language,
	source []byte,
) {
	t.Helper()
	wants := map[string]map[string]int{
		"modifier":        {"public": 2},
		"boolean_literal": {"false": 1},
		"identifier":      {"global": 1},
	}
	assertCollapsedTokenChildren(t, tree, language, source, wants)
}

func assertPowerShellCollapsedTokens(
	t *testing.T,
	tree *gotreesitter.Tree,
	language *gotreesitter.Language,
	source []byte,
) {
	t.Helper()
	wants := map[string]map[string]int{
		"assignement_operator": {"=": 1, "+=": 1},
	}
	assertCollapsedTokenChildren(t, tree, language, source, wants)
}

func assertCollapsedTokenChildren(
	t *testing.T,
	tree *gotreesitter.Tree,
	language *gotreesitter.Language,
	source []byte,
	wants map[string]map[string]int,
) {
	t.Helper()
	root := collapsedTokenCleanRoot(t, tree, language)
	for parentType, children := range wants {
		seen := make(map[string]int, len(children))
		for _, parent := range collapsedChildRegressionNodes(root, language, parentType) {
			text := string(parent.Text(source))
			if _, ok := children[text]; !ok {
				continue
			}
			if parent.ChildCount() != 1 {
				t.Fatalf("%s %q children = %d, want 1", parentType, text, parent.ChildCount())
			}
			child := parent.Child(0)
			if child == nil || child.IsNamed() || child.Type(language) != text ||
				child.StartByte() != parent.StartByte() ||
				child.EndByte() != parent.EndByte() {
				t.Fatalf("%s %q child = %v", parentType, text, child)
			}
			seen[text]++
		}
		for childType, count := range children {
			if seen[childType] != count {
				t.Fatalf(
					"%s/%s count = %d, want %d: %s",
					parentType,
					childType,
					seen[childType],
					count,
					root.SExpr(language),
				)
			}
		}
	}
}

func collapsedTokenCleanRoot(
	t *testing.T,
	tree *gotreesitter.Tree,
	language *gotreesitter.Language,
) *gotreesitter.Node {
	t.Helper()
	root := tree.RootNode()
	if root == nil || root.HasError() {
		t.Fatalf("unexpected root: %v", root)
	}
	if tree.ParseStopReason() != gotreesitter.ParseStopAccepted {
		t.Fatalf("parse stop = %s", tree.ParseStopReason())
	}
	return root
}

func collapsedTokenTreeDigest(
	t *testing.T,
	tree *gotreesitter.Tree,
	language *gotreesitter.Language,
) string {
	t.Helper()
	inspection, err := benchfixtures.InspectGoTree(tree.RootNode(), language)
	if err != nil {
		t.Fatal(err)
	}
	return inspection.SHA256
}
