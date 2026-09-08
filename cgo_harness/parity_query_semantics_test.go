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
	// source replaces the smoke sample when set, for cases that need an
	// ERROR node or a particular shape.
	source string
	// known names a divergence that another board owns. The case still runs
	// and reports the divergence as a skip; it fails when the engines agree,
	// so the entry is removed once the board closes it.
	known string
}

var parityQuerySemanticsCases = []parityQuerySemanticsCase{
	// Anchors before and after anonymous child patterns (C ignores anonymous
	// siblings when it enforces an anchor, except after an unnamed wildcard).
	{lang: "squirrel", query: `(local_declaration (identifier) @v . "=")`},
	{lang: "squirrel", query: `(local_declaration (identifier) @v "=")`},
	{lang: "squirrel", query: `(local_declaration (identifier) @v . (integer) @i)`},
	{lang: "squirrel", query: `(local_declaration "local" . (identifier) @v)`},
	{lang: "squirrel", query: `(local_declaration (integer) @i . ";")`},
	{lang: "squirrel", query: `(local_declaration (_) @a . (_) @b)`},
	{lang: "go", query: `(source_file (package_clause) @p . (import_declaration) @i)`},
	{lang: "go", query: `(package_clause "package" . (package_identifier) @n)`},
	{lang: "go", query: `(import_declaration "import" . (import_spec) @s)`},
	{lang: "go", query: `(function_declaration (identifier) @n . (parameter_list) @p)`},
	{lang: "go", query: `(source_file (_) @first . (_) @second)`},
	{lang: "go", query: `(parameter_list "(" . ")" @close)`},
	{lang: "javascript", query: `(lexical_declaration "const" . (variable_declarator) @d)`},
	{lang: "javascript", query: `(variable_declarator (identifier) @n . "=" . (_) @v)`},
	{lang: "python", query: `(function_definition "def" . (identifier) @n)`},
	{lang: "python", query: `(parameters "(" . (identifier) @first)`},
	{lang: "python", query: `(parameters (identifier) @last . ")")`},
	// Supertype patterns: C compiles a supertype name to a wildcard step
	// that checks the node's hidden supertype ancestors, keys a
	// wildcard-rooted pattern on its child without checking the root, and
	// accepts `super/sub` only for a real subtype.
	{lang: "hare", query: `(type) @t`},
	{lang: "hare", query: `(function_declaration (type) @t)`},
	{lang: "hare", query: `(expression) @e`},
	{lang: "hare", query: `(declaration) @d`},
	{lang: "hare", query: `(declaration (_) @c)`},
	{lang: "hare", query: `(type (_) @c)`},
	{lang: "hare", query: `(type/builtin_type) @t`},
	{lang: "hare", query: `[(type) (expression)] @x`},
	{lang: "luau", query: `(type) @t`},
	{lang: "luau", query: `(type (identifier) @t)`},
	{lang: "luau", query: `(type (builtin_type) @b)`},
	{lang: "luau", query: `(type (number) @n)`},
	{lang: "luau", query: `(expression) @e`},
	{lang: "luau", query: `(expression (identifier) @i)`},
	{lang: "luau", query: `(statement) @s`},
	{lang: "luau", query: `(statement (variable_declaration) @v)`},
	{lang: "luau", query: `(type/builtin_type) @t`},
	{lang: "javascript", query: `(expression) @e`},
	{lang: "javascript", query: `(primary_expression) @p`},
	{lang: "javascript", query: `(statement) @s`},
	{lang: "javascript", query: `(declaration) @d`},
	{lang: "javascript", query: `(pattern) @p`},
	{lang: "javascript", query: `(expression (identifier) @i)`},
	{lang: "javascript", query: `(variable_declarator value: (expression) @v)`},
	{lang: "javascript", query: `(call_expression function: (expression) @f)`},
	{lang: "javascript", query: `(call_expression function: (primary_expression) @f)`},
	{lang: "javascript", query: `(expression/identifier) @i`},
	{lang: "javascript", query: `(primary_expression/identifier) @i`},
	{lang: "python", query: `(expression) @e`},
	{lang: "python", query: `(primary_expression) @p`},
	{lang: "python", query: `(pattern) @p`},
	{lang: "python", query: `(parameter) @p`},
	{lang: "python", query: `(call function: (primary_expression) @f)`},
	{lang: "python", query: `(expression (identifier) @i)`},
	{lang: "python", query: `(expression/identifier) @i`},
	{lang: "go", query: `(expression) @e`},
	{lang: "go", query: `(statement) @s`},
	{lang: "go", query: `(type) @t`},
	{lang: "go", query: `(simple_statement) @s`},
	{lang: "go", query: `(call_expression function: (expression) @f)`},
	{lang: "go", query: `(expression (identifier) @i)`},
	{lang: "go", query: `(type/type_identifier) @t`},
	{lang: "rust", query: `(expression) @e`},
	{lang: "rust", query: `(type) @t`},
	{lang: "rust", query: `(pattern) @p`},
	{lang: "rust", query: `(declaration_statement) @d`},
	{lang: "rust", query: `(let_declaration type: (type) @t)`},
	// Hidden rule names that carry no supertype flag are not query node
	// types; a supertype keeps its grammar name.
	{lang: "go", query: `(_expression) @e`},
	{lang: "go", query: `(_statement) @s`},
	{lang: "go", query: `(_simple_statement) @s`},
	{lang: "go", query: `(_type) @t`},
	{lang: "go", query: `(_expression (identifier) @i)`},
	{lang: "go", query: `(_simple_type/type_identifier) @t`,
		known: "grammargen supertype map: an aliased subtype (identifier aliased to type_identifier) is recorded as its target symbol, so the subtype check rejects the alias name C accepts"},
	{lang: "rust", query: `(_expression) @e`},
	{lang: "rust", query: `(_type) @t`},
	{lang: "rust", query: `(_pattern) @p`},
	{lang: "rust", query: `(_literal) @l`},
	{lang: "rust", query: `(let_declaration type: (_type) @t)`},
	// Wildcards never match ERROR nodes, and a skipped wildcard root still
	// needs a parent that is not an ERROR node. The sources are ones where
	// both engines build the same recovered tree.
	{lang: "python", query: `(_) @n`, source: "x = \n"},
	{lang: "python", query: `_ @n`, source: "x = \n"},
	{lang: "python", query: `(_ (identifier) @i)`, source: "x = \n"},
	{lang: "python", query: `(_ (_) @c)`, source: "x = \n"},
	{lang: "python", query: `(module (_) @c)`, source: "x = \n"},
	{lang: "python", query: `(ERROR) @e`, source: "x = \n"},
	{lang: "python", query: `(ERROR (_) @c)`, source: "x = \n"},
	{lang: "python", query: `(ERROR (identifier) @i)`, source: "x = \n"},
	{lang: "python", query: `[(_) (ERROR)] @x`, source: "x = \n"},
	{lang: "python", query: `[(ERROR) (identifier)] @x`, source: "x = \n"},
	{lang: "python", query: `(_ [(identifier) (integer)] @x)`, source: "x = (1 +\n"},
	{lang: "python", query: `(_ (identifier) @i . (integer) @n)`, source: "x = (1 +\n"},
	{lang: "python", query: `(ERROR (identifier) @i . (integer) @n)`, source: "x = (1 +\n"},
	{lang: "python", query: `(_ (integer) @n .)`, source: "x = (1 +\n"},
	{lang: "python", query: `(expression) @e`, source: "x = (1 +\n"},
	{lang: "python", query: `(primary_expression) @p`, source: "x = (1 +\n",
		known: "recovery: the compact error region absorbs terminals, so a subtree C reduced before the error (integer under primary_expression) keeps no hidden wrapper"},
	{lang: "python", query: `(function_definition parameters: (parameters (MISSING ")") @m))`, source: "def f(:\n    return 1\n"},
	{lang: "python", query: `(MISSING) @m`, source: "def f(:\n    return 1\n"},
	{lang: "python", query: `(_) @n`, source: "def f(:\n    return 1\n"},
	{lang: "javascript", query: `(_ !name (identifier) @i)`},
	{lang: "javascript", query: `(_ name: (identifier) @i)`},
	{lang: "javascript", query: `(_ . (identifier) @i)`},
	{lang: "javascript", query: `(_ (identifier) @i .)`},
	{lang: "javascript", query: `(expression . (identifier) @i)`},
	{lang: "javascript", query: `((_) @p (identifier) @i)`},
	{lang: "javascript", query: `(_ "(" @open)`},
	{lang: "javascript", query: `(_ (_ (identifier) @i))`},
	{lang: "python", query: `(_ (identifier) @i)`, source: "def f(:\n    return 1\n"},
}

func TestParityQuerySemantics(t *testing.T) {
	for _, tc := range parityQuerySemanticsCases {
		tc := tc
		t.Run(tc.lang+"/"+strings.ReplaceAll(tc.query, " ", "_"), func(t *testing.T) {
			src := []byte(grammars.ParseSmokeSample(tc.lang))
			if tc.source != "" {
				src = []byte(tc.source)
			}
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
			goQuery, err := gotreesitter.NewQuery(tc.query, goLang)
			if qErr != nil {
				// A query the C compiler rejects must be rejected by Go too.
				if err == nil {
					if tc.known != "" {
						t.Skipf("known divergence (%s)\n C rejects the query but Go compiles it\n C: %v", tc.known, qErr)
					}
					t.Fatalf("C rejects the query but Go compiles it\n C: %v", qErr)
				}
				if tc.known != "" {
					t.Fatalf("both compilers reject the query; remove the known divergence note %q", tc.known)
				}
				t.Logf("both compilers reject the query\n C: %v\nGo: %v", qErr, err)
				return
			}
			defer cQuery.Close()
			if err != nil {
				if tc.known != "" {
					t.Skipf("known divergence (%s)\n Go rejects the query but C compiles it\nGo: %v", tc.known, err)
				}
				t.Fatalf("Go query: %v", err)
			}
			want := parityQueryCapturesC(cQuery, cTree, src)
			got := parityQueryCapturesGo(goQuery, goTree, goLang, src)
			if strings.Join(want, "\n") != strings.Join(got, "\n") {
				if tc.known != "" {
					t.Skipf("known divergence (%s)\n C: %v\nGo: %v", tc.known, want, got)
				}
				t.Fatalf("query captures diverge\n C: %v\nGo: %v\nGo tree: %s", want, got, goTree.RootNode().SExpr(goLang))
			}
			if tc.known != "" {
				t.Fatalf("captures agree; remove the known divergence note %q", tc.known)
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
