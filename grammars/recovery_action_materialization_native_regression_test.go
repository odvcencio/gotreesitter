//go:build !grammar_subset

package grammars

import (
	"testing"

	gotreesitter "github.com/odvcencio/gotreesitter"
)

type recoveryActionMaterializationCase struct {
	name        string
	language    func() *gotreesitter.Language
	source      string
	wantSExpr   string
	wantMissing string
}

var recoveryActionMaterializationCases = []recoveryActionMaterializationCase{
	{
		name:        "forth_body",
		language:    ForthLanguage,
		source:      ": foo 1 2\n",
		wantSExpr:   "(source_file (word_definition (start_definition) (word) (number (decimal_number)) (number (decimal_number)) (end_definition)))",
		wantMissing: "end_definition",
	},
	{
		name:      "forth_empty",
		language:  ForthLanguage,
		source:    ": foo\n",
		wantSExpr: "(source_file (ERROR (start_definition) (word)))",
	},
	{
		name:      "forth_terminated",
		language:  ForthLanguage,
		source:    ": foo 1 2 ;\n",
		wantSExpr: "(source_file (word_definition (start_definition) (word) (number (decimal_number)) (number (decimal_number)) (end_definition)))",
	},
	{
		name:      "luau_end",
		language:  LuauLanguage,
		source:    "end",
		wantSExpr: "(chunk (ERROR (identifier)))",
	},
	{
		name:      "luau_declaration_end",
		language:  LuauLanguage,
		source:    "local x = 1\nend\n",
		wantSExpr: "(chunk (variable_declaration (assignment_statement (variable_list (identifier)) (expression_list (number)))) (ERROR (identifier)))",
	},
	{
		name:      "luau_if_end",
		language:  LuauLanguage,
		source:    "if true then\nend\nend\n",
		wantSExpr: "(chunk (if_statement (true)) (ERROR (identifier)))",
	},
}

func TestRecoveryActionMaterializationNeedsNoResultCompatibility(t *testing.T) {
	for _, test := range recoveryActionMaterializationCases {
		test := test
		t.Run(test.name, func(t *testing.T) {
			source := []byte(test.source)
			language := test.language()

			tree, err := gotreesitter.NewParser(language).
				ParseNoResultCompatibilityBenchmarkOnly(source)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(tree.Release)

			assertNoNormalizationPasses(t, tree)
			assertRecoveryActionMaterializationTree(t, tree, language, test)
		})
	}
}

func TestRecoveryActionMaterializationRoutesStayExactOrFailClosed(t *testing.T) {
	for _, test := range recoveryActionMaterializationCases {
		test := test
		t.Run(test.name, func(t *testing.T) {
			source := []byte(test.source)
			language := test.language()

			productionParser := gotreesitter.NewParser(language)
			productionParser.SetAdmissionCandidateRoute(false)
			production, err := productionParser.Parse(source)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(production.Release)
			assertNoNormalizationPasses(t, production)
			assertRecoveryActionMaterializationTree(t, production, language, test)
			want := collapsedTokenTreeDigest(t, production, language)

			routedBefore, fallbackBefore := gotreesitter.AdmissionCandidateCounters()
			compactParser := gotreesitter.NewParser(language)
			compactParser.SetAdmissionCandidateRoute(true)
			compact, err := compactParser.Parse(source)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(compact.Release)
			routedAfter, fallbackAfter := gotreesitter.AdmissionCandidateCounters()
			direct := routedAfter == routedBefore+1 && fallbackAfter == fallbackBefore
			fallback := routedAfter == routedBefore && fallbackAfter == fallbackBefore+1
			if !direct && !fallback {
				t.Fatalf(
					"compact counters routed=%d/%d fallback=%d/%d: %s",
					routedBefore,
					routedAfter,
					fallbackBefore,
					fallbackAfter,
					gotreesitter.AdmissionCandidateLastFallbackReason(),
				)
			}
			assertNoNormalizationPasses(t, compact)
			assertRecoveryActionMaterializationDigest(t, "compact", compact, language, want)

			forestParser := gotreesitter.NewParser(language)
			forest, ok := forestParser.ParseForestExperimental(source)
			if ok && forest != nil {
				t.Cleanup(forest.Release)
				assertNoNormalizationPasses(t, forest)
				assertRecoveryActionMaterializationDigest(t, "forest", forest, language, want)
			} else {
				if forest != nil {
					forest.Release()
					t.Fatal("forest returned a tree with a decline")
				}
				_, _, reason, _ := forestParser.ForestDeclineInfo()
				if reason == "" {
					t.Fatal("forest declined without a reason")
				}
				forestFallback, fallbackErr := forestParser.Parse(source)
				if fallbackErr != nil {
					t.Fatal(fallbackErr)
				}
				t.Cleanup(forestFallback.Release)
				assertNoNormalizationPasses(t, forestFallback)
				assertRecoveryActionMaterializationDigest(
					t,
					"forest-fallback",
					forestFallback,
					language,
					want,
				)
			}

			assertRecoveryActionMaterializationIncremental(t, language, source, want)
		})
	}
}

func assertRecoveryActionMaterializationIncremental(
	t *testing.T,
	language *gotreesitter.Language,
	source []byte,
	want string,
) {
	t.Helper()
	if len(source) == 0 {
		t.Fatal("incremental fixture is empty")
	}

	oldSource := source[:len(source)-1]
	parser := gotreesitter.NewParser(language)
	parser.SetAdmissionCandidateRoute(false)
	oldTree, err := parser.Parse(oldSource)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(oldTree.Release)
	startPoint := retiredDispatchPointAtByte(oldSource, len(oldSource))
	oldTree.Edit(gotreesitter.InputEdit{
		StartByte:   uint32(len(oldSource)),
		OldEndByte:  uint32(len(oldSource)),
		NewEndByte:  uint32(len(source)),
		StartPoint:  startPoint,
		OldEndPoint: startPoint,
		NewEndPoint: retiredDispatchPointAtByte(source, len(source)),
	})
	incremental, _, err := parser.ParseIncrementalProfiled(source, oldTree)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(incremental.Release)
	assertNoNormalizationPasses(t, incremental)
	assertRecoveryActionMaterializationDigest(
		t,
		"incremental",
		incremental,
		language,
		want,
	)
}

func assertRecoveryActionMaterializationTree(
	t *testing.T,
	tree *gotreesitter.Tree,
	language *gotreesitter.Language,
	test recoveryActionMaterializationCase,
) {
	t.Helper()
	root := tree.RootNode()
	if root == nil {
		t.Fatal("root is nil")
	}
	if got := root.SExpr(language); got != test.wantSExpr {
		t.Fatalf("tree = %s, want %s", got, test.wantSExpr)
	}
	if test.wantMissing == "" {
		return
	}
	missing := findRecoveryActionMaterializationNode(root, language, test.wantMissing)
	if missing == nil || !missing.IsMissing() || missing.StartByte() != missing.EndByte() {
		t.Fatalf(
			"%s missing node = %v, want zero-width missing node",
			test.wantMissing,
			missing,
		)
	}
}

func findRecoveryActionMaterializationNode(
	node *gotreesitter.Node,
	language *gotreesitter.Language,
	name string,
) *gotreesitter.Node {
	if node == nil {
		return nil
	}
	if node.Type(language) == name {
		return node
	}
	for index := 0; index < node.ChildCount(); index++ {
		if found := findRecoveryActionMaterializationNode(node.Child(index), language, name); found != nil {
			return found
		}
	}
	return nil
}

func assertRecoveryActionMaterializationDigest(
	t *testing.T,
	route string,
	tree *gotreesitter.Tree,
	language *gotreesitter.Language,
	want string,
) {
	t.Helper()
	if got := collapsedTokenTreeDigest(t, tree, language); got != want {
		t.Fatalf("%s digest = %s, want production %s", route, got, want)
	}
}
