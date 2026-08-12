//go:build !grammar_subset

package grammars

import (
	"testing"

	gotreesitter "github.com/odvcencio/gotreesitter"
)

// elixirExtrasAndPairsCase pins one Elixir tree shape that a retired
// post-parse compatibility pass used to repair by hand:
//
//   - "comment": the external scanner's hidden `_newline_before_comment`
//     synchronization token must never surface as a tree node, whether the
//     preceding comment sits at the source root or inside a call/do-block.
//   - "pairs": consecutive `key: value` map entries must group under one
//     `keywords` node, and a map's `|` update syntax or bare `=>` entries
//     must wrap under a `binary_operator` node, exactly as C tree-sitter
//     builds them.
//
// Every case here already holds with zero post-parse rewrites: native
// scheduling and reduction publish the C-equivalent shape directly.
type elixirExtrasAndPairsCase struct {
	name      string
	source    string
	wantSExpr string
	assert    func(*testing.T, *gotreesitter.Node, *gotreesitter.Language)
}

func elixirExtrasAndPairsCases() []elixirExtrasAndPairsCase {
	return []elixirExtrasAndPairsCase{
		{
			name:      "root_comments_separated_by_newline",
			source:    "# one\n# two\n\n[]",
			wantSExpr: "(source (comment) (comment) (list))",
			assert:    assertElixirRootCommentsAreBareExtras,
		},
		{
			name:      "do_block_comment_before_keyword",
			source:    "fun x\n# comment\ndo\n  x\nend",
			wantSExpr: "(source (call (identifier) (arguments (identifier)) (comment) (do_block (identifier))))",
		},
		{
			name:      "map_consecutive_keyword_pairs",
			source:    "%{a: 1, b: 2}",
			wantSExpr: "(source (map (map_content (keywords (pair (keyword) (integer)) (pair (keyword) (integer))))))",
			assert:    assertElixirMapKeywordsGrouped,
		},
		{
			name:      "map_keyword_pairs_split_by_comment",
			source:    "%{\n  a: 1,\n  # comment\n  b: 2\n}",
			wantSExpr: "(source (map (map_content (keywords (pair (keyword) (integer)) (comment) (pair (keyword) (integer))))))",
		},
		{
			name:      "map_update_syntax_wraps_binary_operator",
			source:    `%{map | name: "Silly"}`,
			wantSExpr: `(source (map (map_content (binary_operator (identifier) (keywords (pair (keyword) (string (quoted_content))))))))`,
			assert:    assertElixirMapUpdateBinaryOperatorWrapped,
		},
		{
			name:      "map_arrow_entries_wrap_binary_operator",
			source:    "%{1 => 2, 3 => 4}",
			wantSExpr: "(source (map (map_content (binary_operator (integer) (integer)) (binary_operator (integer) (integer)))))",
		},
		{
			name:      "map_mixed_arrow_and_keyword_entries",
			source:    `%{"a" => 1, b: 2, c: 3}`,
			wantSExpr: `(source (map (map_content (binary_operator (string (quoted_content)) (integer)) (keywords (pair (keyword) (integer)) (pair (keyword) (integer))))))`,
		},
	}
}

func TestElixirExtrasAndPairsNeedNoResultCompatibility(t *testing.T) {
	language := ElixirLanguage()
	for _, test := range elixirExtrasAndPairsCases() {
		test := test
		t.Run(test.name, func(t *testing.T) {
			source := []byte(test.source + "\n")
			tree, err := gotreesitter.NewParser(language).
				ParseNoResultCompatibilityBenchmarkOnly(source)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(tree.Release)
			assertElixirExtrasAndPairsShape(t, tree.RootNode(), language, test)
			assertNoNormalizationPasses(t, tree)
		})
	}
}

func TestElixirExtrasAndPairsRetiredDispatchArmRoutes(t *testing.T) {
	language := ElixirLanguage()
	for _, test := range elixirExtrasAndPairsCases() {
		test := test
		t.Run(test.name, func(t *testing.T) {
			for _, receipt := range retiredDispatchRouteReceipts(t, language, []byte(test.source)) {
				t.Run(receipt.name, func(t *testing.T) {
					assertElixirExtrasAndPairsShape(t, receipt.tree.RootNode(), language, test)
				})
			}
		})
	}
}

func assertElixirExtrasAndPairsShape(
	t *testing.T,
	root *gotreesitter.Node,
	language *gotreesitter.Language,
	test elixirExtrasAndPairsCase,
) {
	t.Helper()
	if got := root.SExpr(language); got != test.wantSExpr {
		t.Fatalf("%s tree = %s, want %s", test.name, got, test.wantSExpr)
	}
	if test.assert != nil {
		test.assert(t, root, language)
	}
}

func assertElixirRootCommentsAreBareExtras(
	t *testing.T,
	root *gotreesitter.Node,
	language *gotreesitter.Language,
) {
	t.Helper()
	if root.ChildCount() != 3 {
		t.Fatalf("root child count = %d, want 3: %s", root.ChildCount(), root.SExpr(language))
	}
	for index := 0; index < 2; index++ {
		comment := root.Child(index)
		if comment == nil || comment.Type(language) != "comment" || !comment.IsExtra() {
			t.Fatalf("root child %d = %v, want an extra comment", index, comment)
		}
	}
	list := root.Child(2)
	if list == nil || list.Type(language) != "list" || list.IsExtra() {
		t.Fatalf("root child 2 = %v, want a non-extra list", list)
	}
}

func assertElixirMapKeywordsGrouped(
	t *testing.T,
	root *gotreesitter.Node,
	language *gotreesitter.Language,
) {
	t.Helper()
	content := findFirstNamedDescendantWhere(
		root,
		language,
		"map_content",
		func(*gotreesitter.Node) bool { return true },
	)
	if content == nil || content.ChildCount() != 1 {
		t.Fatalf("missing single-child map_content: %s", root.SExpr(language))
	}
	if got := content.FieldNameForChild(0, language); got != "" {
		t.Fatalf("map_content child field = %q, want empty", got)
	}
	keywords := content.Child(0)
	if keywords == nil || keywords.Type(language) != "keywords" || !keywords.IsNamed() {
		t.Fatalf("map_content child = %v, want a named keywords node", keywords)
	}
}

func assertElixirMapUpdateBinaryOperatorWrapped(
	t *testing.T,
	root *gotreesitter.Node,
	language *gotreesitter.Language,
) {
	t.Helper()
	binary := findFirstNamedDescendantWhere(
		root,
		language,
		"binary_operator",
		func(*gotreesitter.Node) bool { return true },
	)
	if binary == nil || binary.ChildCount() != 3 {
		t.Fatalf("missing three-child map update binary_operator: %s", root.SExpr(language))
	}
	for index, want := range []string{"left", "operator", "right"} {
		if got := binary.FieldNameForChild(index, language); got != want {
			t.Fatalf("binary_operator child %d field = %q, want %q", index, got, want)
		}
	}
	keywords := binary.Child(2)
	if keywords == nil || keywords.Type(language) != "keywords" {
		t.Fatalf("binary_operator right child = %v, want keywords", keywords)
	}
}
