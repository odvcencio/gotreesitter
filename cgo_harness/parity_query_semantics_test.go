//go:build cgo && treesitter_c_parity

package cgoharness

import (
	"fmt"
	"sort"
	"strings"
	"testing"

	gotreesitter "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
	sitter "github.com/tree-sitter/go-tree-sitter"
)

// parityQuerySemanticsCase is one query run against one smoke sample on
// both engines. The captures must agree exactly, including which nodes an
// anchor admits and which supertype patterns match.
type parityQuerySemanticsCase struct {
	lang  string
	query string
}

var parityQuerySemanticsCases = []parityQuerySemanticsCase{
	// Anchors before and after anonymous child patterns (C ignores anonymous
	// siblings when it enforces an anchor, except after an unnamed wildcard).
	{"squirrel", `(local_declaration (identifier) @v . "=")`},
	{"squirrel", `(local_declaration (identifier) @v "=")`},
	{"squirrel", `(local_declaration (identifier) @v . (integer) @i)`},
	{"squirrel", `(local_declaration "local" . (identifier) @v)`},
	{"squirrel", `(local_declaration (integer) @i . ";")`},
	{"squirrel", `(local_declaration (_) @a . (_) @b)`},
	{"go", `(source_file (package_clause) @p . (import_declaration) @i)`},
	{"go", `(package_clause "package" . (package_identifier) @n)`},
	{"go", `(import_declaration "import" . (import_spec) @s)`},
	{"go", `(function_declaration (identifier) @n . (parameter_list) @p)`},
	{"go", `(source_file (_) @first . (_) @second)`},
	{"go", `(parameter_list "(" . ")" @close)`},
	{"javascript", `(lexical_declaration "const" . (variable_declarator) @d)`},
	{"javascript", `(variable_declarator (identifier) @n . "=" . (_) @v)`},
	{"python", `(function_definition "def" . (identifier) @n)`},
	{"python", `(parameters "(" . (identifier) @first)`},
	{"python", `(parameters (identifier) @last . ")")`},
}

func TestParityQuerySemantics(t *testing.T) {
	for _, tc := range parityQuerySemanticsCases {
		tc := tc
		t.Run(tc.lang+"/"+strings.ReplaceAll(tc.query, " ", "_"), func(t *testing.T) {
			src := []byte(grammars.ParseSmokeSample(tc.lang))
			goTree, goLang, err := parseWithGo(parityCase{name: tc.lang, source: string(src)}, src, nil)
			if err != nil {
				t.Fatalf("Go parse: %v", err)
			}
			defer releaseGoTree(goTree)
			cLang, err := ParityCLanguage(tc.lang)
			if err != nil {
				if reason := parityReferenceSkipReason(err); reason != "" {
					t.Skipf("skip C reference: %s", reason)
				}
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
			if diff := FirstDivergenceDumpV1(goTree.RootNode(), goLang, cTree.RootNode()); diff != nil {
				t.Skipf("trees diverge before the query runs: %+v", diff)
			}
			cQuery, qErr := sitter.NewQuery(cLang, tc.query)
			if qErr != nil {
				t.Fatalf("C query: %v", qErr)
			}
			defer cQuery.Close()
			goQuery, err := gotreesitter.NewQuery(tc.query, goLang)
			if err != nil {
				t.Fatalf("Go query: %v", err)
			}
			want := parityQueryCapturesC(cQuery, cTree, src)
			got := parityQueryCapturesGo(goQuery, goTree, goLang, src)
			if strings.Join(want, "\n") != strings.Join(got, "\n") {
				t.Fatalf("query captures diverge\n C: %v\nGo: %v\nGo tree: %s", want, got, goTree.RootNode().SExpr(goLang))
			}
			t.Logf("%d captures agree", len(want))
		})
	}
}

func parityQueryCapturesC(q *sitter.Query, tree *sitter.Tree, src []byte) []string {
	cursor := sitter.NewQueryCursor()
	defer cursor.Close()
	names := q.CaptureNames()
	var out []string
	matches := cursor.Matches(q, tree.RootNode(), src)
	for {
		m := matches.Next()
		if m == nil {
			break
		}
		for _, c := range m.Captures {
			out = append(out, fmt.Sprintf("p%d %s [%d-%d]", m.PatternIndex, names[c.Index], c.Node.StartByte(), c.Node.EndByte()))
		}
	}
	sort.Strings(out)
	return out
}

func parityQueryCapturesGo(q *gotreesitter.Query, tree *gotreesitter.Tree, lang *gotreesitter.Language, src []byte) []string {
	var out []string
	for _, m := range q.Execute(tree) {
		for _, c := range m.Captures {
			out = append(out, fmt.Sprintf("p%d %s [%d-%d]", m.PatternIndex, c.Name, c.Node.StartByte(), c.Node.EndByte()))
		}
	}
	sort.Strings(out)
	return out
}
