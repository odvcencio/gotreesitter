package main

import (
	"fmt"
	"reflect"
	"strings"
	"testing"
	"unicode/utf16"

	"github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
	"github.com/odvcencio/gotreesitter/internal/ccore/bind"
	"github.com/odvcencio/gotreesitter/internal/ccore/certified"
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

// TestExecuteQueryJSONReportsOffsetAdjustedRange proves executeQueryJSON
// reads the capture range accessors, so a query with a #offset! directive
// reports the adjusted range in Start/End, not the captured node's own
// unmodified range.
func TestExecuteQueryJSONReportsOffsetAdjustedRange(t *testing.T) {
	source := "package p\n\nfunc main() {}\n"
	tree, lang := parseTestTree(t, source)

	query, err := gotreesitter.NewQuery(`
((function_declaration name: (identifier) @id)
  (#offset! @id 0 1 0 -1))
`, lang)
	if err != nil {
		t.Fatalf("compile query: %v", err)
	}

	matches, truncated := executeQueryJSON(query, tree, lang, maxQueryMatches)
	if truncated {
		t.Fatal("query unexpectedly reported truncation")
	}
	if len(matches) != 1 || len(matches[0].Captures) != 1 {
		t.Fatalf("matches: got %+v, want exactly one match with one capture", matches)
	}
	capture := matches[0].Captures[0]

	// The identifier node itself still spans "main" (4 bytes); #offset!
	// shrinks the reported range by one byte at each end, to "ai".
	nameStart := uint32(strings.Index(source, "main"))
	wantStart, wantEnd := nameStart+1, nameStart+uint32(len("main"))-1
	if capture.Start != wantStart || capture.End != wantEnd {
		t.Fatalf("Start/End: got [%d,%d), want [%d,%d)", capture.Start, capture.End, wantStart, wantEnd)
	}
	if got, want := capture.Text, "ai"; got != want {
		t.Fatalf("Text: got %q, want %q", got, want)
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
	entry := grammars.DetectLanguageByName("c")
	if entry == nil {
		t.Skip("C is not included in this grammar subset")
	}
	lang := entry.Language()
	loaded := newRuntimeLanguage("c", lang)
	if loaded.tokenSourceFactory == nil {
		t.Fatal("C runtime language did not retain its registered token-source factory")
	}

	factory := loaded.tokenSourceFactory
	calls := 0
	loaded.tokenSourceFactory = func(source []byte) gotreesitter.TokenSource {
		calls++
		return factory(source)
	}

	tree, err := loaded.parseUTF16(`char *s = "gotreesitter";`)
	if err != nil {
		t.Fatalf("parse through registered token source: %v", err)
	}
	tree.Release()
	if calls != 1 {
		t.Fatalf("parse token-source factory calls = %d, want 1", calls)
	}

	highlighter, err := loaded.newHighlighter(`(string_literal) @string`)
	if err != nil {
		t.Fatalf("create highlighter through registered token source: %v", err)
	}
	if ranges := highlighter.Highlight([]byte(`char *s = "gotreesitter";`)); len(ranges) == 0 {
		t.Fatal("highlighter returned no string captures")
	}
	if calls != 2 {
		t.Fatalf("parse and highlight token-source factory calls = %d, want 2", calls)
	}

	uncertified := newRuntimeLanguage("out-of-tree-c", lang)
	if uncertified.tokenSourceFactory != nil {
		t.Fatal("unknown language name inherited a registry token-source factory")
	}
}

// The bridge parses a registry language with its registered token-source
// factory, fresh and then incrementally over UTF-16, and the incremental tree
// must equal a fresh parse of the edited source on whichever route serves it.
//
// On the default route the language's certificate decides. When it lists the
// table-v1 scan provider (C's does), the certified core serves both parses: it
// lexes with the grammar's own tables, so the factory is called once per parse
// and its source is never read. The core runs no fresh-result check of the
// existing engine; its incremental result equals a fresh parse by
// certification, and it reparses fresh any incremental result that has an
// error. Without the grant the existing engine serves both parses.
//
// On the existing engine (the core route switched off), a span-changing edit
// needs a fresh-result check, and that check must build its source with the
// registered factory from the edited source: the factory is called a third
// time.
func TestRuntimeLanguageIncrementalUTF16UsesRegisteredTokenSourceFactory(t *testing.T) {
	entry := grammars.DetectLanguageByName("c")
	if entry == nil {
		t.Skip("C is not included in this grammar subset")
	}
	sum, _ := entry.Language().GrammarBlobSHA256()
	cert, certifiedBlob := certified.Blobs[sum]
	granted := certifiedBlob && cert.HasProvider("table-v1")
	for _, tc := range []struct {
		name     string
		routeOff bool
	}{{"default_route", false}, {"existing_engine", true}} {
		t.Run(tc.name, func(t *testing.T) {
			core := granted && !tc.routeOff
			loaded := newRuntimeLanguage("c", entry.Language())
			if loaded.tokenSourceFactory == nil {
				t.Fatal("C runtime language did not retain its registered token-source factory")
			}
			factory := loaded.tokenSourceFactory
			var factorySources []string
			loaded.tokenSourceFactory = func(source []byte) gotreesitter.TokenSource {
				factorySources = append(factorySources, string(source))
				return factory(source)
			}
			newParser := func() *gotreesitter.Parser {
				p := gotreesitter.NewParser(loaded.language)
				if tc.routeOff {
					p.SetAdmissionCandidateRoute(false)
				}
				return p
			}
			parser := newParser()
			oldSource := units(`char *s = "gotreesitter";`)
			newSource := units(`char *s = "treesitter";`)
			served0, declined0 := bridgeCoreCounts()
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
			served1, _ := bridgeCoreCounts()
			newTree, err := loaded.parseUTF16Units(parser, newSource, oldTree)
			if err != nil {
				t.Fatalf("incremental parse: %v", err)
			}
			if newTree == nil {
				t.Fatal("incremental parse returned no tree")
			}
			defer newTree.Release()
			served2, declined2 := bridgeCoreCounts()
			if newTree.RootNode() == nil || newTree.RootNode().HasError() {
				t.Fatalf("incremental parse returned invalid tree: %v", newTree.RootNode())
			}
			wantSources := []string{`char *s = "gotreesitter";`, `char *s = "treesitter";`}
			if !core {
				wantSources = append(wantSources, `char *s = "treesitter";`) // the fresh-result check
			}
			if !reflect.DeepEqual(factorySources, wantSources) {
				t.Fatalf("factory sources (core=%t) = %q, want %q", core, factorySources, wantSources)
			}
			switch {
			case core && (served1-served0 != 1 || served2-served1 != 1 || declined2 != declined0):
				t.Fatalf("core: served full %d and incremental %d, declined %d; want 1, 1 and 0", served1-served0, served2-served1, declined2-declined0)
			case !core && served2 != served0:
				t.Fatalf("existing engine: the core served %d requests, want 0", served2-served0)
			}
			fresh, err := loaded.parseUTF16Units(newParser(), newSource, nil)
			if err != nil {
				t.Fatalf("fresh parse: %v", err)
			}
			defer fresh.Release()
			gotJSON, gotTruncated := buildJSONTree(newTree, loaded.language, newTree.RootNode(), maxTreeNodes)
			wantJSON, wantTruncated := buildJSONTree(fresh, loaded.language, fresh.RootNode(), maxTreeNodes)
			if gotTruncated || wantTruncated || !reflect.DeepEqual(gotJSON, wantJSON) {
				t.Fatalf("incremental UTF-16 tree differs from fresh: got=%+v want=%+v", gotJSON, wantJSON)
			}
			t.Logf("core=%t factory calls=%d; incremental UTF-16 tree equals fresh", core, len(factorySources))
		})
	}
}

// bridgeCoreCounts is the number of requests the certified core served, fresh
// and incremental, and the number it declined, since the process started.
func bridgeCoreCounts() (served, declined int64) {
	d, f, i := bind.RouteCounters()
	for _, v := range d {
		declined += v
	}
	return f + i, declined
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
