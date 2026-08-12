//go:build !grammar_subset

package grammars

import (
	"testing"

	gotreesitter "github.com/odvcencio/gotreesitter"
)

// fidlVersionedLayoutModifierFixtures cover FIDL `type X = <modifiers>
// <kind> <body>;` declarations where a declaration modifier keyword
// (`strict`, `flexible`, `resource`) is followed by a parenthesized
// `(name=value)` argument list the grammar does not accept: declaration
// modifiers are bare keywords with no argument syntax. The parser recovers
// from the unexpected `(` the same way for one, two, or three stacked
// modifiers.
var fidlVersionedLayoutModifierFixtures = []struct {
	name   string
	source string
}{
	{
		name:   "two_modifiers_with_args",
		source: "library test;\ntype Color = strict(removed=2) flexible(added=2) enum {\n    RED = 1;\n};",
	},
	{
		name:   "single_modifier_with_args",
		source: "library test;\ntype Color = strict(removed=2) enum {\n    RED = 1;\n};",
	},
	{
		name:   "three_modifiers_with_args",
		source: "library test;\ntype Color = strict(removed=2) flexible(added=2) resource(added=3) enum {\n    RED = 1;\n};",
	},
}

// TestFIDLVersionedLayoutModifiersNeedsNoResultCompatibility proves the
// native parser already produces the C-compatible recovery shape for a
// versioned-layout-modifier declaration without any post-parse
// compatibility pass: the compatibility-free route
// (ParseNoResultCompatibilityBenchmarkOnly) and the production route
// (Parse) build byte-identical trees, and production performs zero
// normalization rewrites.
func TestFIDLVersionedLayoutModifiersNeedsNoResultCompatibility(t *testing.T) {
	t.Setenv("GTS_DISPATCHER_CENSUS", "1")
	language := FidlLanguage()
	for _, fixture := range fidlVersionedLayoutModifierFixtures {
		fixture := fixture
		t.Run(fixture.name, func(t *testing.T) {
			source := []byte(fixture.source)

			raw, err := gotreesitter.NewParser(language).
				ParseNoResultCompatibilityBenchmarkOnly(source)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(raw.Release)
			assertNoNormalizationPasses(t, raw)
			if !raw.RootNode().HasError() {
				t.Fatalf("raw root has no error, want C-compatible recovery: %s", raw.RootNode().SExpr(language))
			}

			production, err := gotreesitter.NewParser(language).Parse(source)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(production.Release)
			assertNoNormalizationPasses(t, production)

			want := collapsedTokenTreeDigest(t, production, language)
			if got := collapsedTokenTreeDigest(t, raw, language); got != want {
				t.Fatalf("compatibility-free digest=%s production=%s", got, want)
			}
		})
	}
}

// TestFIDLVersionedLayoutModifiersRootShape locks the exact recovery shape
// the C oracle produces for the two-modifier witness: an extra ERROR
// wrapping the first modifier's stray argument list, the `=` token, a
// second extra ERROR wrapping the second modifier's stray argument list,
// and an inline_layout that keeps only its layout_kind and layout_body.
func TestFIDLVersionedLayoutModifiersRootShape(t *testing.T) {
	language := FidlLanguage()
	source := []byte(fidlVersionedLayoutModifierFixtures[0].source)
	tree, err := gotreesitter.NewParser(language).Parse(source)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(tree.Release)

	root := tree.RootNode()
	if !root.HasError() {
		t.Fatalf("root.HasError() = false, want C-compatible recovery error: %s", root.SExpr(language))
	}
	layout := findFIDLNodeByType(root, language, "layout_declaration")
	if layout == nil {
		t.Fatalf("layout_declaration not found in %s", root.SExpr(language))
	}
	if !layout.HasError() {
		t.Fatal("layout_declaration.HasError() = false, want true")
	}
	if got, want := layout.ChildCount(), 6; got != want {
		t.Fatalf("layout_declaration child count = %d, want %d", got, want)
	}
	if got := layout.Child(2); got == nil || got.Type(language) != "ERROR" || !got.IsExtra() || got.ChildCount() != 9 {
		t.Fatalf("layout child 2 = %#v, want extra ERROR with 9 children", got)
	}
	if got := layout.Child(3); got == nil || got.Type(language) != "=" {
		t.Fatalf("layout child 3 type = %v, want =", got)
	}
	if got := layout.Child(4); got == nil || got.Type(language) != "ERROR" || !got.IsExtra() {
		t.Fatalf("layout child 4 = %#v, want extra ERROR", got)
	}
	inline := layout.Child(5)
	if inline == nil || inline.Type(language) != "inline_layout" || inline.ChildCount() != 2 {
		t.Fatalf("layout child 5 = %#v, want inline_layout with 2 children", inline)
	}
	if got := inline.Child(0); got == nil || got.Type(language) != "layout_kind" {
		t.Fatalf("inline first child = %#v, want layout_kind", got)
	}
}

func findFIDLNodeByType(root *gotreesitter.Node, lang *gotreesitter.Language, typ string) *gotreesitter.Node {
	if root == nil {
		return nil
	}
	if root.Type(lang) == typ {
		return root
	}
	for i := 0; i < root.ChildCount(); i++ {
		if found := findFIDLNodeByType(root.Child(i), lang, typ); found != nil {
			return found
		}
	}
	return nil
}

// TestFIDLVersionedLayoutModifiersRoutesStayExactOrFailClosed proves
// production, compact, forest, and incremental routes all build the same
// tree for the versioned-layout-modifier witnesses with no result
// compatibility pass involved. The forest route may legitimately decline
// this recovery-heavy shape; when it does, its own Parse fallback must still
// match production exactly.
func TestFIDLVersionedLayoutModifiersRoutesStayExactOrFailClosed(t *testing.T) {
	t.Setenv("GTS_DISPATCHER_CENSUS", "1")
	language := FidlLanguage()
	for _, fixture := range fidlVersionedLayoutModifierFixtures {
		fixture := fixture
		t.Run(fixture.name, func(t *testing.T) {
			source := []byte(fixture.source)

			productionParser := gotreesitter.NewParser(language)
			productionParser.SetAdmissionCandidateRoute(false)
			production, err := productionParser.Parse(source)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(production.Release)
			assertNoNormalizationPasses(t, production)
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
					routedBefore, routedAfter, fallbackBefore, fallbackAfter,
					gotreesitter.AdmissionCandidateLastFallbackReason(),
				)
			}
			assertNoNormalizationPasses(t, compact)
			if got := collapsedTokenTreeDigest(t, compact, language); got != want {
				t.Fatalf("compact digest=%s production=%s", got, want)
			}

			forestParser := gotreesitter.NewParser(language)
			forest, ok := forestParser.ParseForestExperimental(source)
			if ok && forest != nil {
				t.Cleanup(forest.Release)
				assertNoNormalizationPasses(t, forest)
				if got := collapsedTokenTreeDigest(t, forest, language); got != want {
					t.Fatalf("forest digest=%s production=%s", got, want)
				}
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
				if got := collapsedTokenTreeDigest(t, forestFallback, language); got != want {
					t.Fatalf("forest-fallback digest=%s production=%s", got, want)
				}
			}

			if len(source) == 0 {
				t.Fatal("fixture is empty")
			}
			oldSource := source[:len(source)-1]
			incrementalParser := gotreesitter.NewParser(language)
			incrementalParser.SetAdmissionCandidateRoute(false)
			oldTree, err := incrementalParser.Parse(oldSource)
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
			incremental, _, err := incrementalParser.ParseIncrementalProfiled(source, oldTree)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(incremental.Release)
			assertNoNormalizationPasses(t, incremental)
			if got := collapsedTokenTreeDigest(t, incremental, language); got != want {
				t.Fatalf("incremental digest=%s production=%s", got, want)
			}
		})
	}
}
