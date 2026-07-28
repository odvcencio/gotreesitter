//go:build !grammar_subset

package grammars

import (
	"bytes"
	"testing"

	gotreesitter "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/internal/benchfixtures"
)

var forthDefinitionCases = []struct {
	name               string
	source             string
	wantError          bool
	wantWordDefinition bool
	wantMissingEnd     bool
}{
	{
		name:               "body",
		source:             ": foo 1 2\n",
		wantError:          true,
		wantWordDefinition: true,
		wantMissingEnd:     true,
	},
	{name: "empty", source: ": foo\n", wantError: true},
	{
		name:               "terminated",
		source:             ": foo 1 2 ;\n",
		wantWordDefinition: true,
	},
}

func TestForthDefinitionsNeedNoResultCompatibility(t *testing.T) {
	language := ForthLanguage()
	for _, test := range forthDefinitionCases {
		t.Run(test.name, func(t *testing.T) {
			source := []byte(test.source)
			tree, err := gotreesitter.NewParser(language).
				ParseNoResultCompatibilityBenchmarkOnly(source)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(tree.Release)

			assertNoNormalizationPasses(t, tree)
			assertForthDefinitionTree(
				t,
				"compatibility-free",
				tree,
				language,
				test.wantError,
				test.wantWordDefinition,
				test.wantMissingEnd,
			)
		})
	}
}

func TestForthUnterminatedDefinitionNativeRoutes(t *testing.T) {
	language := ForthLanguage()
	source := []byte(": foo 1 2\n")

	productionParser := gotreesitter.NewParser(language)
	productionParser.SetAdmissionCandidateRoute(false)
	production, err := productionParser.Parse(source)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(production.Release)
	assertNoNormalizationPasses(t, production)
	assertForthDefinitionTree(t, "production", production, language, true, true, true)
	wantDigest := forthTreeDigest(t, production, language)

	routedBefore, fallbackBefore := gotreesitter.AdmissionCandidateCounters()
	compactParser := gotreesitter.NewParser(language)
	compactParser.SetAdmissionCandidateRoute(true)
	compact, err := compactParser.Parse(source)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(compact.Release)
	routedAfter, fallbackAfter := gotreesitter.AdmissionCandidateCounters()
	if routedAfter != routedBefore || fallbackAfter != fallbackBefore+1 {
		t.Fatalf(
			"compact route counters routed=%d/%d fallback=%d/%d: %s",
			routedBefore,
			routedAfter,
			fallbackBefore,
			fallbackAfter,
			gotreesitter.AdmissionCandidateLastFallbackReason(),
		)
	}
	assertNoNormalizationPasses(t, compact)
	assertForthDefinitionTree(t, "compact", compact, language, true, true, true)
	assertForthDigest(t, "compact", compact, language, wantDigest)

	forestParser := gotreesitter.NewParser(language)
	forest, ok := forestParser.ParseForestExperimental(source)
	if ok || forest != nil {
		if forest != nil {
			forest.Release()
		}
		t.Fatal("forest route returned a tree for an unterminated definition")
	}
	offset, symbol, reason, _ := forestParser.ForestDeclineInfo()
	if offset != uint32(len(source)) || symbol != 0 || reason != "eof_no_root" {
		t.Fatalf(
			"forest decline = offset %d symbol %d reason %q, want %d/0/eof_no_root",
			offset,
			symbol,
			reason,
			len(source),
		)
	}
	forestFallback, err := forestParser.Parse(source)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(forestFallback.Release)
	assertNoNormalizationPasses(t, forestFallback)
	assertForthDefinitionTree(t, "forest-fallback", forestFallback, language, true, true, true)
	assertForthDigest(t, "forest-fallback", forestFallback, language, wantDigest)

	assertForthIncrementalDeleteReceipt(t, language)
}

func assertForthIncrementalDeleteReceipt(
	t *testing.T,
	language *gotreesitter.Language,
) {
	t.Helper()

	oldSource := []byte(": foo 1 2 ;\n")
	semicolon := bytes.IndexByte(oldSource, ';')
	if semicolon < 0 {
		t.Fatal("missing semicolon in the incremental fixture")
	}
	source := append([]byte(nil), oldSource[:semicolon]...)
	source = append(source, oldSource[semicolon+1:]...)

	parser := gotreesitter.NewParser(language)
	parser.SetAdmissionCandidateRoute(false)
	oldTree, err := parser.Parse(oldSource)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(oldTree.Release)
	oldTree.Edit(gotreesitter.InputEdit{
		StartByte:   uint32(semicolon),
		OldEndByte:  uint32(semicolon + 1),
		NewEndByte:  uint32(semicolon),
		StartPoint:  gotreesitter.Point{Column: uint32(semicolon)},
		OldEndPoint: gotreesitter.Point{Column: uint32(semicolon + 1)},
		NewEndPoint: gotreesitter.Point{Column: uint32(semicolon)},
	})
	incremental, _, err := parser.ParseIncrementalProfiled(source, oldTree)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(incremental.Release)
	assertNoNormalizationPasses(t, incremental)
	assertForthDefinitionTree(t, "incremental-delete", incremental, language, true, true, true)

	fresh, err := gotreesitter.NewParser(language).
		ParseNoResultCompatibilityBenchmarkOnly(source)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(fresh.Release)
	assertNoNormalizationPasses(t, fresh)
	assertForthDefinitionTree(t, "fresh-delete", fresh, language, true, true, true)
	assertForthDigest(
		t,
		"incremental-delete",
		incremental,
		language,
		forthTreeDigest(t, fresh, language),
	)
}

func assertForthDefinitionTree(
	t *testing.T,
	route string,
	tree *gotreesitter.Tree,
	language *gotreesitter.Language,
	wantError bool,
	wantWordDefinition bool,
	wantMissingEnd bool,
) {
	t.Helper()

	root := tree.RootNode()
	if root == nil {
		t.Fatalf("%s root is nil", route)
	}
	if got := root.HasError(); got != wantError {
		t.Fatalf("%s root HasError = %t, want %t: %s", route, got, wantError, root.SExpr(language))
	}
	definition := findForthNode(root, language, "word_definition")
	if !wantWordDefinition {
		if definition != nil {
			t.Fatalf("%s has unexpected word_definition: %s", route, root.SExpr(language))
		}
		if findForthNode(root, language, "end_definition") != nil {
			t.Fatalf("%s has unexpected end_definition: %s", route, root.SExpr(language))
		}
		return
	}
	if definition == nil {
		t.Fatalf("%s missing word_definition: %s", route, root.SExpr(language))
	}
	end := findForthNode(definition, language, "end_definition")
	if end == nil {
		t.Fatalf("%s missing end_definition: %s", route, root.SExpr(language))
	}
	if got := end.IsMissing(); got != wantMissingEnd {
		t.Fatalf("%s end_definition IsMissing = %t, want %t", route, got, wantMissingEnd)
	}
	if wantMissingEnd && end.StartByte() != end.EndByte() {
		t.Fatalf(
			"%s missing end_definition span = %d-%d, want zero width",
			route,
			end.StartByte(),
			end.EndByte(),
		)
	}
}

func findForthNode(
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
		if found := findForthNode(node.Child(index), language, name); found != nil {
			return found
		}
	}
	return nil
}

func forthTreeDigest(
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

func assertForthDigest(
	t *testing.T,
	route string,
	tree *gotreesitter.Tree,
	language *gotreesitter.Language,
	want string,
) {
	t.Helper()
	if got := forthTreeDigest(t, tree, language); got != want {
		t.Fatalf("%s digest = %s, want %s", route, got, want)
	}
}
