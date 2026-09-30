//go:build cgo && treesitter_c_parity

package cgoharness

import (
	"fmt"
	"reflect"
	"slices"
	"strings"
	"testing"

	gotreesitter "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
	sitter "github.com/tree-sitter/go-tree-sitter"
)

type exactQueryCase struct {
	name   string
	lang   string
	source string
	query  string
}

type exactQueryMatch struct {
	PatternIndex int
	Captures     []exactQueryCapture
}

type exactQueryCapture struct {
	Name      string
	Type      string
	Named     bool
	StartByte uint32
	EndByte   uint32
	Text      string
}

func TestParityQueryExactJavaScriptEcosystemPatterns(t *testing.T) {
	cases := []exactQueryCase{
		{
			name: "top_level_comments_are_separate_matches",
			lang: "javascript",
			source: `// one
// two
const value = 1;
// three
`,
			query: `(program (comment) @comment)`,
		},
		{
			name: "anchor_after_zero_or_more_comments",
			lang: "javascript",
			source: `const one = require("one");
const two = require(
  // keep this with the dependency
  "two"
);
const three = require(
  /* leading block */
  // leading line
  "three"
);
`,
			query: `
(call_expression
  function: (identifier) @fn
  arguments: (arguments . (comment)* . (string (string_fragment) @from))
  (#eq? @fn "require"))
`,
		},
		{
			name:   "repeated_capture_grouping",
			lang:   "javascript",
			source: `const values = [alpha, beta, gamma];`,
			query:  `(array (identifier) @item)`,
		},
		{
			name: "top_level_repetition_groups_adjacent_comments",
			lang: "javascript",
			source: `// a
// b
// c

call();

// d
`,
			query: `(comment)+ @doc`,
		},
		{
			name: "parent_repetition_uses_first_contiguous_comment_run",
			lang: "javascript",
			source: `// a
// b
// c

call();

// d
`,
			query: `(program (comment)+ @doc)`,
		},
		{
			name: "parent_star_repetition_uses_first_contiguous_comment_run",
			lang: "javascript",
			source: `// a
// b
// c

call();

// d
`,
			query: `(program (comment)* @doc)`,
		},
		{
			name:   "parent_star_repetition_without_child",
			lang:   "javascript",
			source: `call();`,
			query:  `(program (comment)* @doc)`,
		},
		{
			name:   "parent_optional_repetition_without_child",
			lang:   "javascript",
			source: `call();`,
			query:  `(program (comment)? @doc)`,
		},
		{
			name: "top_level_star_repetition_groups_adjacent_comments",
			lang: "javascript",
			source: `// a
// b
// c

call();

// d
`,
			query: `(comment)* @doc`,
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			assertExactQueryParity(t, tc)
		})
	}

}

func TestParityQueryExactKeywordStringPatterns(t *testing.T) {
	cases := []exactQueryCase{
		{
			name:   "kotlin_val_keyword_alias",
			lang:   "kotlin",
			source: grammars.ParseSmokeSample("kotlin"),
			query:  `"val" @keyword`,
		},
		{
			name:   "predicate_newline_escape",
			lang:   "go",
			source: "package p\n/*line\nbreak*/\n",
			query:  `((comment) @comment (#eq? @comment "/*line\nbreak*/"))`,
		},
		{
			name:   "predicate_quote_and_backslash_escapes",
			lang:   "go",
			source: "package p\n// q\"q a\\b\n",
			query:  `((comment) @comment (#eq? @comment "// q\"q a\\b"))`,
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			assertExactQueryParity(t, tc)
		})
	}

}

func TestParityQueryPointRangeEmptyNodeBoundaries(t *testing.T) {
	const (
		langName = "typescript"
		source   = "const { value: [dirPath, { dirName, options, fileNames }] } = result;\nswitch (x) { case: }\n"
		queryStr = `(identifier) @candidate
(switch_case) @clause`
	)
	src := []byte(source)
	goTree, goLang, err := parseWithGo(parityCase{name: langName, source: source}, src, nil)
	if err != nil {
		t.Fatalf("Go parse error: %v", err)
	}
	defer releaseGoTree(goTree)
	cLang, err := ParityCLanguage(langName)
	if err != nil {
		t.Fatalf("load C parser: %v", err)
	}
	cParser := sitter.NewParser()
	defer cParser.Close()
	if err := cParser.SetLanguage(cLang); err != nil {
		t.Fatalf("C SetLanguage: %v", err)
	}
	cTree := cParser.Parse(src, nil)
	if cTree == nil {
		t.Fatal("C parser returned nil tree")
	}
	defer cTree.Close()

	goQuery, err := gotreesitter.NewQuery(queryStr, goLang)
	if err != nil {
		t.Fatalf("Go NewQuery: %v", err)
	}
	cQuery, queryErr := sitter.NewQuery(cLang, queryStr)
	if queryErr != nil {
		t.Fatalf("C NewQuery: %v", queryErr)
	}
	defer cQuery.Close()

	var missing *gotreesitter.Node
	gotreesitter.Walk(goTree.RootNode(), func(node *gotreesitter.Node, _ int) gotreesitter.WalkAction {
		if missing == nil && node.IsMissing() {
			missing = node
		}
		return gotreesitter.WalkContinue
	})
	if missing == nil || !missing.IsMissing() {
		t.Fatalf("Go missing identifier = %+v; tree=%s", missing, goTree.RootNode().SExpr(goLang))
	}
	boundary := missing.StartPoint()
	before := pointBefore(boundary)
	after := gotreesitter.Point{Row: boundary.Row + 1}

	cases := []struct {
		name       string
		start, end gotreesitter.Point
		want       []string
	}{
		{name: "zero_width_at_start", start: boundary, end: after, want: []string{"clause"}},
		{name: "zero_width_at_end", start: before, end: boundary, want: []string{"clause"}},
		{name: "strictly_outside", start: after, end: gotreesitter.Point{Row: after.Row + 1}},
		{name: "reversed_is_ignored", start: after, end: boundary, want: []string{"candidate", "candidate", "candidate", "candidate", "clause"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			goNames := collectGoPointRangeCaptureNames(goQuery, goTree, goLang, src, tc.start, tc.end)
			cNames := collectCPointRangeCaptureNames(cQuery, cTree, src, tc.start, tc.end)
			slices.Sort(goNames)
			slices.Sort(cNames)
			want := append([]string(nil), tc.want...)
			slices.Sort(want)
			if !reflect.DeepEqual(goNames, cNames) || !reflect.DeepEqual(goNames, want) {
				t.Fatalf("captures Go=%v C=%v want=%v", goNames, cNames, want)
			}
		})
	}

	t.Run("zero_end_sentinels_are_unbounded", func(t *testing.T) {
		goPointNames := collectGoPointRangeCaptureNames(goQuery, goTree, goLang, src, boundary, gotreesitter.Point{})
		cPointNames := collectCPointRangeCaptureNames(cQuery, cTree, src, boundary, gotreesitter.Point{})
		slices.Sort(goPointNames)
		slices.Sort(cPointNames)
		if len(goPointNames) == 0 || !reflect.DeepEqual(goPointNames, cPointNames) {
			t.Fatalf("zero point end captures Go=%v C=%v, want equal non-empty unbounded suffix", goPointNames, cPointNames)
		}

		startByte := missing.StartByte()
		goByteNames := collectGoByteRangeCaptureNames(goQuery, goTree, goLang, src, startByte, 0)
		cByteNames := collectCByteRangeCaptureNames(cQuery, cTree, src, startByte, 0)
		slices.Sort(goByteNames)
		slices.Sort(cByteNames)
		if len(goByteNames) == 0 || !reflect.DeepEqual(goByteNames, cByteNames) {
			t.Fatalf("zero byte end captures Go=%v C=%v, want equal non-empty unbounded suffix", goByteNames, cByteNames)
		}
	})
}

func pointBefore(p gotreesitter.Point) gotreesitter.Point {
	if p.Column > 0 {
		p.Column--
	}
	return p
}

func collectGoPointRangeCaptureNames(q *gotreesitter.Query, tree *gotreesitter.Tree, lang *gotreesitter.Language, source []byte, start, end gotreesitter.Point) []string {
	cursor := q.Exec(tree.RootNode(), lang, source)
	cursor.SetPointRange(start, end)
	var names []string
	for {
		capture, ok := cursor.NextCapture()
		if !ok {
			return names
		}
		names = append(names, capture.Name)
	}
}

func collectCPointRangeCaptureNames(q *sitter.Query, tree *sitter.Tree, source []byte, start, end gotreesitter.Point) []string {
	cursor := sitter.NewQueryCursor()
	defer cursor.Close()
	cursor.SetPointRange(
		sitter.NewPoint(uint(start.Row), uint(start.Column)),
		sitter.NewPoint(uint(end.Row), uint(end.Column)),
	)
	namesByID := q.CaptureNames()
	matches := cursor.Captures(q, tree.RootNode(), source)
	var names []string
	for {
		match, index := matches.Next()
		if match == nil {
			return names
		}
		capture := match.Captures[index]
		if int(capture.Index) < len(namesByID) {
			names = append(names, namesByID[capture.Index])
		}
	}
}

func collectGoByteRangeCaptureNames(q *gotreesitter.Query, tree *gotreesitter.Tree, lang *gotreesitter.Language, source []byte, start, end uint32) []string {
	cursor := q.Exec(tree.RootNode(), lang, source)
	cursor.SetByteRange(start, end)
	var names []string
	for {
		capture, ok := cursor.NextCapture()
		if !ok {
			return names
		}
		names = append(names, capture.Name)
	}
}

func collectCByteRangeCaptureNames(q *sitter.Query, tree *sitter.Tree, source []byte, start, end uint32) []string {
	cursor := sitter.NewQueryCursor()
	defer cursor.Close()
	cursor.SetByteRange(uint(start), uint(end))
	namesByID := q.CaptureNames()
	matches := cursor.Captures(q, tree.RootNode(), source)
	var names []string
	for {
		match, index := matches.Next()
		if match == nil {
			return names
		}
		capture := match.Captures[index]
		if int(capture.Index) < len(namesByID) {
			names = append(names, namesByID[capture.Index])
		}
	}
}

func TestParityQueryExactMissing(t *testing.T) {
	cases := []exactQueryCase{
		{
			name:   "missing_semicolon",
			lang:   "c",
			source: "int a\n",
			query:  `(MISSING ";") @missing`,
		},
		{
			name:   "missing_or_identifier_alternation",
			lang:   "c",
			source: "int a;\nint b\n",
			query:  `[(MISSING) (identifier)] @match`,
		},
		{
			name:   "qualified_missing_or_identifier_alternation",
			lang:   "c",
			source: "int a;\nint b\n",
			query:  `[(MISSING ";") (identifier)] @match`,
		},
		{
			name:   "qualified_missing_named_alternation",
			lang:   "typescript",
			source: "const { value: [dirPath, { dirName, options, fileNames }] } = result;\nswitch (x) { case: }\n",
			query:  `[(MISSING identifier) (switch_case)] @match`,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assertExactQueryParity(t, tc)
		})
	}
}

func TestParityDescendantByteRangeBoundaries(t *testing.T) {
	for _, tc := range []struct {
		name       string
		lang       string
		source     string
		start, end uint32
	}{
		{name: "token_end", lang: "go", source: "package main\n\nimport \"fmt\"\n", start: 7, end: 7},
		{name: "line_end", lang: "go", source: "package main\n\nimport \"fmt\"\n", start: 12, end: 12},
		{name: "out_of_bounds", lang: "go", source: "package main\n\nimport \"fmt\"\n", start: 1000, end: 1005},
		{name: "reversed", lang: "go", source: "package main\n\nimport \"fmt\"\n", start: 5, end: 4},
		{name: "zero_width_missing", lang: "c", source: "int a\n", start: 5, end: 5},
	} {
		t.Run(tc.name, func(t *testing.T) {
			src := []byte(tc.source)
			goTree, goLang, err := parseWithGo(parityCase{name: tc.lang, source: tc.source}, src, nil)
			if err != nil {
				t.Fatal(err)
			}
			defer releaseGoTree(goTree)
			cLang, err := ParityCLanguage(tc.lang)
			if err != nil {
				t.Fatal(err)
			}
			cParser := sitter.NewParser()
			defer cParser.Close()
			if err := cParser.SetLanguage(cLang); err != nil {
				t.Fatal(err)
			}
			cTree := cParser.Parse(src, nil)
			defer cTree.Close()

			goNode := goTree.RootNode().DescendantForByteRange(tc.start, tc.end)
			cNode := cTree.RootNode().DescendantForByteRange(uint(tc.start), uint(tc.end))
			if (goNode == nil) != (cNode == nil) {
				t.Fatalf("nil mismatch: Go=%v C=%v", goNode == nil, cNode == nil)
			}
			if goNode == nil {
				return
			}
			if goNode.Type(goLang) != cNode.Kind() || goNode.StartByte() != uint32(cNode.StartByte()) || goNode.EndByte() != uint32(cNode.EndByte()) || goNode.IsMissing() != cNode.IsMissing() {
				t.Fatalf("descendant mismatch: Go=%s[%d,%d] missing=%v C=%s[%d,%d] missing=%v", goNode.Type(goLang), goNode.StartByte(), goNode.EndByte(), goNode.IsMissing(), cNode.Kind(), cNode.StartByte(), cNode.EndByte(), cNode.IsMissing())
			}
		})
	}
}

func assertExactQueryParity(t *testing.T, tc exactQueryCase) {
	t.Helper()

	src := []byte(tc.source)
	goTree, goLang, err := parseWithGo(parityCase{name: tc.lang, source: tc.source}, src, nil)
	if err != nil {
		t.Fatalf("Go parse error: %v", err)
	}
	defer releaseGoTree(goTree)

	cLang, err := ParityCLanguage(tc.lang)
	if err != nil {
		if skipReason := parityReferenceSkipReason(err); skipReason != "" {
			t.Skipf("skip C reference parser: %s", skipReason)
		}
		t.Fatalf("load C parser: %v", err)
	}

	cParser := sitter.NewParser()
	defer cParser.Close()
	if err := cParser.SetLanguage(cLang); err != nil {
		if skipReason := parityReferenceSkipReason(err); skipReason != "" {
			t.Skipf("skip C reference parser SetLanguage: %s", skipReason)
		}
		t.Fatalf("C SetLanguage: %v", err)
	}
	cTree := cParser.Parse(src, nil)
	if cTree == nil || cTree.RootNode() == nil {
		t.Fatal("C parser returned nil tree")
	}
	defer cTree.Close()

	var structuralErrs []string
	compareNodes(goTree.RootNode(), goLang, cTree.RootNode(), "root", &structuralErrs)

	goMatches := collectGoExactQueryMatches(t, goLang, goTree, tc.query, src)
	cMatches := collectCExactQueryMatches(t, cLang, cTree, tc.query, src)

	queryMatches := reflect.DeepEqual(goMatches, cMatches)
	if len(structuralErrs) == 0 && queryMatches {
		return
	}

	var structuralReport string
	if len(structuralErrs) > 0 {
		structuralReport = fmt.Sprintf("\nStructural divergence:\n%s\nGo root: %s\nC root:  %s\nGo children:\n%s\nC children:\n%s",
			firstLines(structuralErrs, 12),
			goTree.RootNode().SExpr(goLang),
			cTree.RootNode().ToSexp(),
			formatGoRootChildren(goTree.RootNode(), goLang, src),
			formatCRootChildren(cTree.RootNode(), src))
	}

	var queryReport string
	if !queryMatches {
		queryReport = fmt.Sprintf("\nQuery divergence:\nGo:\n%s\nC:\n%s",
			formatExactQueryMatches(goMatches),
			formatExactQueryMatches(cMatches))
	}

	t.Fatalf("exact query parity mismatch for %s/%s\nQuery:\n%s%s%s",
		tc.lang, tc.name,
		strings.TrimSpace(tc.query),
		structuralReport,
		queryReport)
}

func collectGoExactQueryMatches(t *testing.T, lang *gotreesitter.Language, tree *gotreesitter.Tree, queryStr string, source []byte) []exactQueryMatch {
	t.Helper()

	q, err := gotreesitter.NewQuery(queryStr, lang)
	if err != nil {
		t.Fatalf("Go NewQuery error: %v", err)
	}

	cursor := q.Exec(tree.RootNode(), lang, source)
	var matches []exactQueryMatch
	for {
		m, ok := cursor.NextMatch()
		if !ok {
			break
		}
		snap := exactQueryMatch{PatternIndex: m.PatternIndex}
		for _, c := range m.Captures {
			if c.Node == nil {
				continue
			}
			snap.Captures = append(snap.Captures, exactQueryCapture{
				Name:      c.Name,
				Type:      c.Node.Type(lang),
				Named:     c.Node.IsNamed(),
				StartByte: c.Node.StartByte(),
				EndByte:   c.Node.EndByte(),
				Text:      c.Text(source),
			})
		}
		matches = append(matches, snap)
	}
	return matches
}

func collectCExactQueryMatches(t *testing.T, lang *sitter.Language, tree *sitter.Tree, queryStr string, source []byte) []exactQueryMatch {
	t.Helper()

	query, err := sitter.NewQuery(lang, queryStr)
	if err != nil {
		t.Fatalf("C NewQuery error: %v", err)
	}
	defer query.Close()

	cursor := sitter.NewQueryCursor()
	defer cursor.Close()

	names := query.CaptureNames()
	iter := cursor.Matches(query, tree.RootNode(), source)

	var matches []exactQueryMatch
	for {
		m := iter.Next()
		if m == nil {
			break
		}
		snap := exactQueryMatch{PatternIndex: int(m.PatternIndex)}
		for _, c := range m.Captures {
			name := ""
			if int(c.Index) < len(names) {
				name = names[c.Index]
			}
			start := uint32(c.Node.StartByte())
			end := uint32(c.Node.EndByte())
			snap.Captures = append(snap.Captures, exactQueryCapture{
				Name:      name,
				Type:      c.Node.Kind(),
				Named:     c.Node.IsNamed(),
				StartByte: start,
				EndByte:   end,
				Text:      string(source[start:end]),
			})
		}
		matches = append(matches, snap)
	}
	return matches
}

func formatExactQueryMatches(matches []exactQueryMatch) string {
	if len(matches) == 0 {
		return "  <no matches>"
	}
	var b strings.Builder
	for i, m := range matches {
		fmt.Fprintf(&b, "  match[%d] pattern=%d captures=%d\n", i, m.PatternIndex, len(m.Captures))
		for j, c := range m.Captures {
			fmt.Fprintf(&b, "    capture[%d] @%s %s named=%v [%d-%d] %q\n",
				j, c.Name, c.Type, c.Named, c.StartByte, c.EndByte, c.Text)
		}
	}
	return strings.TrimRight(b.String(), "\n")
}

func firstLines(lines []string, limit int) string {
	if len(lines) <= limit {
		return strings.Join(lines, "\n")
	}
	return strings.Join(lines[:limit], "\n") + fmt.Sprintf("\n... and %d more", len(lines)-limit)
}

func formatGoRootChildren(root *gotreesitter.Node, lang *gotreesitter.Language, source []byte) string {
	if root == nil {
		return "  <nil>"
	}
	var b strings.Builder
	for i := 0; i < root.ChildCount(); i++ {
		child := root.Child(i)
		if child == nil {
			fmt.Fprintf(&b, "  [%d] <nil>\n", i)
			continue
		}
		fmt.Fprintf(&b, "  [%d] %s named=%v [%d-%d] %q\n",
			i, child.Type(lang), child.IsNamed(), child.StartByte(), child.EndByte(), child.Text(source))
	}
	return strings.TrimRight(b.String(), "\n")
}

func formatCRootChildren(root *sitter.Node, source []byte) string {
	if root == nil {
		return "  <nil>"
	}
	var b strings.Builder
	for i := uint(0); i < uint(root.ChildCount()); i++ {
		child := root.Child(i)
		if child == nil {
			fmt.Fprintf(&b, "  [%d] <nil>\n", i)
			continue
		}
		start := uint32(child.StartByte())
		end := uint32(child.EndByte())
		fmt.Fprintf(&b, "  [%d] %s named=%v [%d-%d] %q\n",
			i, child.Kind(), child.IsNamed(), start, end, string(source[start:end]))
	}
	return strings.TrimRight(b.String(), "\n")
}
