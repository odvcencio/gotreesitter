package main

import (
	"fmt"
	"strings"
	"testing"
	"unicode/utf16"

	"github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
)

func parseTestTree(t *testing.T, source string) (*gotreesitter.Tree, *gotreesitter.Language) {
	t.Helper()
	entry := grammars.DetectLanguageByName("go")
	if entry == nil {
		t.Skip("Go is not included in this grammar subset")
	}
	lang := entry.Language()
	parser := gotreesitter.NewParser(lang)
	tree, err := parser.ParseUTF16(utf16.Encode([]rune(source)))
	if err != nil {
		t.Fatalf("parse test source: %v", err)
	}
	t.Cleanup(tree.Release)
	return tree, lang
}

func countTreeNodes(node *gotreesitter.Node) int {
	if node == nil {
		return 0
	}
	count := 1
	for i := 0; i < node.ChildCount(); i++ {
		count += countTreeNodes(node.Child(i))
	}
	return count
}

func countJSONNodes(node *jsonNode) int {
	if node == nil {
		return 0
	}
	count := 1
	for _, child := range node.Children {
		count += countJSONNodes(child)
	}
	return count
}

func TestBuildJSONTreeMarksOnlyOmittedNodesAsTruncated(t *testing.T) {
	tree, lang := parseTestTree(t, "package p\n\nfunc f() { println(1) }\n")
	root := tree.RootNode()
	nodeCount := countTreeNodes(root)
	if nodeCount < 2 {
		t.Fatalf("test tree has only %d node(s)", nodeCount)
	}

	exact, truncated := buildJSONTree(tree, lang, root, nodeCount)
	if truncated || exact.Truncated {
		t.Fatal("tree that exactly fits the node limit was marked truncated")
	}
	if got := countJSONNodes(exact); got != nodeCount {
		t.Fatalf("exact tree contains %d nodes, want %d", got, nodeCount)
	}

	over, truncated := buildJSONTree(tree, lang, root, nodeCount-1)
	if !truncated || !over.Truncated {
		t.Fatal("tree with an omitted node was not marked truncated")
	}
	if got := countJSONNodes(over); got != nodeCount-1 {
		t.Fatalf("truncated tree contains %d nodes, want %d", got, nodeCount-1)
	}
}

func TestBuildJSONParseResultHandlesEmptyTree(t *testing.T) {
	tree, lang := parseTestTree(t, "package p\n")
	result, err := buildJSONParseResult(tree, lang, nil, maxTreeNodes)
	if err != nil {
		t.Fatalf("build empty parse result: %v", err)
	}
	if result.SExpr != "" || result.HasError || result.Tree != "null" {
		t.Fatalf("empty parse result = %+v, want empty S-expression, no error, and null tree", result)
	}
}

func TestRuntimeBoundaryNilInputsStaySafe(t *testing.T) {
	loaded := newRuntimeLanguage("json", nil)
	if loaded.language != nil || loaded.tokenSourceFactory != nil {
		t.Fatalf("nil runtime language = %+v, want no language or token-source factory", loaded)
	}

	if start16, end16 := spans16(nil, 7, 19); start16 != 7 || end16 != 19 {
		t.Fatalf("nil-tree UTF-16 fallback = (%d, %d), want (7, 19)", start16, end16)
	}

	if matches, truncated := executeQueryJSON(nil, nil, nil, maxQueryMatches); len(matches) != 0 || truncated {
		t.Fatalf("nil-input query = (%d matches, truncated=%v), want no matches and no truncation", len(matches), truncated)
	}
}

func TestExecuteQueryJSONHandlesEmptyRoot(t *testing.T) {
	entry := grammars.DetectLanguageByName("go")
	if entry == nil {
		t.Skip("Go is not included in this grammar subset")
	}
	lang := entry.Language()
	query, err := gotreesitter.NewQuery(`(identifier) @id`, lang)
	if err != nil {
		t.Fatalf("compile query: %v", err)
	}
	tree := gotreesitter.NewTree(nil, nil, lang)
	t.Cleanup(tree.Release)

	matches, truncated := executeQueryJSON(query, tree, lang, maxQueryMatches)
	if len(matches) != 0 || truncated {
		t.Fatalf("empty-root query = (%d matches, truncated=%v), want no matches and no truncation", len(matches), truncated)
	}
}

func goIdentifiersSource(variableCount int) string {
	var source strings.Builder
	source.WriteString("package p\n\nvar (\n")
	for i := 0; i < variableCount; i++ {
		fmt.Fprintf(&source, "v%03d int\n", i)
	}
	source.WriteString(")\n")
	return source.String()
}

func TestExecuteQueryJSONCapsWorkAndReportsTruncation(t *testing.T) {
	tests := []struct {
		name          string
		variableCount int
		wantTruncated bool
	}{
		{name: "exactly at cap", variableCount: maxQueryMatches, wantTruncated: false},
		{name: "one over cap", variableCount: maxQueryMatches + 1, wantTruncated: true},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			tree, lang := parseTestTree(t, goIdentifiersSource(test.variableCount))
			query, err := gotreesitter.NewQuery(`(identifier) @id`, lang)
			if err != nil {
				t.Fatalf("compile query: %v", err)
			}

			matches, truncated := executeQueryJSON(query, tree, lang, maxQueryMatches)
			if len(matches) != maxQueryMatches {
				t.Fatalf("got %d matches, want %d", len(matches), maxQueryMatches)
			}
			if truncated != test.wantTruncated {
				t.Fatalf("truncated = %v, want %v", truncated, test.wantTruncated)
			}
		})
	}
}

func TestRuntimeLanguageUsesRegisteredTokenSourceFactory(t *testing.T) {
	entry := grammars.DetectLanguageByName("json")
	if entry == nil {
		t.Skip("JSON is not included in this grammar subset")
	}
	lang := entry.Language()
	loaded := newRuntimeLanguage("json", lang)
	if loaded.tokenSourceFactory == nil {
		t.Fatal("JSON runtime language did not retain its registered token-source factory")
	}

	factory := loaded.tokenSourceFactory
	calls := 0
	loaded.tokenSourceFactory = func(source []byte) gotreesitter.TokenSource {
		calls++
		return factory(source)
	}

	tree, err := loaded.parseUTF16(`{"name":"gotreesitter"}`)
	if err != nil {
		t.Fatalf("parse through registered token source: %v", err)
	}
	tree.Release()
	if calls != 1 {
		t.Fatalf("parse token-source factory calls = %d, want 1", calls)
	}

	highlighter, err := loaded.newHighlighter(`(string) @string`)
	if err != nil {
		t.Fatalf("create highlighter through registered token source: %v", err)
	}
	if ranges := highlighter.Highlight([]byte(`{"name":"gotreesitter"}`)); len(ranges) == 0 {
		t.Fatal("highlighter returned no string captures")
	}
	if calls != 2 {
		t.Fatalf("parse and highlight token-source factory calls = %d, want 2", calls)
	}

	uncertified := newRuntimeLanguage("out-of-tree-json", lang)
	if uncertified.tokenSourceFactory != nil {
		t.Fatal("unknown language name inherited a registry token-source factory")
	}
}

func TestRuntimeLanguageIncrementalUTF16UsesRegisteredTokenSourceFactory(t *testing.T) {
	entry := grammars.DetectLanguageByName("json")
	if entry == nil {
		t.Skip("JSON is not included in this grammar subset")
	}
	loaded := newRuntimeLanguage("json", entry.Language())
	if loaded.tokenSourceFactory == nil {
		t.Fatal("JSON runtime language did not retain its registered token-source factory")
	}

	factory := loaded.tokenSourceFactory
	calls := 0
	loaded.tokenSourceFactory = func(source []byte) gotreesitter.TokenSource {
		calls++
		return factory(source)
	}
	parser := gotreesitter.NewParser(loaded.language)
	oldSource := units(`{"name":"gotreesitter"}`)
	newSource := units(`{"name":"treesitter"}`)
	oldTree, err := loaded.parseUTF16Units(parser, oldSource, nil)
	if err != nil {
		t.Fatalf("parse old source: %v", err)
	}
	if oldTree == nil {
		t.Fatal("parse old source returned no tree")
	}
	defer oldTree.Release()

	edit, changed := utf16EditBetween(oldSource, newSource)
	if !changed || !oldTree.EditUTF16(edit, newSource) {
		t.Fatalf("apply UTF-16 edit: changed=%v edit=%+v", changed, edit)
	}
	newTree, err := loaded.parseUTF16Units(parser, newSource, oldTree)
	if err != nil {
		t.Fatalf("incremental parse: %v", err)
	}
	if newTree == nil {
		t.Fatal("incremental parse returned no tree")
	}
	defer newTree.Release()
	if newTree.RootNode() == nil || newTree.RootNode().HasError() {
		t.Fatalf("incremental parse returned invalid tree: %v", newTree.RootNode())
	}
	if calls != 2 {
		t.Fatalf("full and incremental token-source factory calls = %d, want 2", calls)
	}
}

func TestRuntimeLanguageKeepsGoOnDFA(t *testing.T) {
	entry := grammars.DetectLanguageByName("go")
	if entry == nil {
		t.Skip("Go is not included in this grammar subset")
	}
	if loaded := newRuntimeLanguage("go", entry.Language()); loaded.tokenSourceFactory != nil {
		t.Fatal("Go runtime language revived the intentionally disabled token-source factory")
	}
}
