package parse_test

import (
	"fmt"
	"strings"
	"testing"

	gts "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
	"github.com/odvcencio/gotreesitter/internal/ccore/certified"
	"github.com/odvcencio/gotreesitter/internal/ccore/certified/certifiedtest"
)

// markdownPinnedCCase is a Markdown input with the tree pinned C gives for it
// (tree-sitter-markdown a0a00f81 for the block grammar, f969cd3a for the inline
// grammar). Inline sources are the text of the block tree's inline node, which
// a consumer parses on its own with the inline grammar.
//
// defaultDiffers marks an input on which the default engine departs from
// pinned C while its grammar has no certificate. That engine reads one
// lookahead for every live GLR stack, so a stack whose own lex mode needs a
// narrower token at the same byte (`\` where the shared token is the
// two-byte escape, `&` where it is `&ouml;`) or the scanner's closing
// emphasis delimiter dies; C lexes each version in its own mode and keeps it.
// The core lexes per version and gives pinned C's tree. The block grammar has
// a certificate, so its inputs run on the core by default and carry no flag.
type markdownPinnedCCase struct {
	name           string
	grammar        string
	source         string
	want           string
	defaultDiffers bool
}

var markdownPinnedCCases = []markdownPinnedCCase{
	{
		name:    "block_link_escaped_bracket",
		grammar: "markdown",
		source:  "[link \\[bar](/uri)\n",
		want:    `(document [0,19] (section [0,19] (paragraph [0,19] (inline [0,18] ("[" [0,1]) ("\\" [6,7]) ("[" [7,8]) ("]" [11,12]) ("(" [12,13]) ("/" [13,14]) (")" [17,18])))))`,
	},
	{
		name:    "block_link_escaped_bracket_after_break",
		grammar: "markdown",
		source:  "***\n\n[link \\[bar](/uri)\n",
		want:    `(document [0,24] (section [0,24] (thematic_break [0,4]) (paragraph [5,24] (inline [5,23] ("[" [5,6]) ("\\" [11,12]) ("[" [12,13]) ("]" [16,17]) ("(" [17,18]) ("/" [18,19]) (")" [22,23])))))`,
	},
	{
		name:    "block_reference_link_escaped_bracket",
		grammar: "markdown",
		source:  "[link \\[bar][ref]\n\n[ref]: /uri\n",
		want:    `(document [0,31] (section [0,31] (paragraph [0,18] (inline [0,17] ("[" [0,1]) ("\\" [6,7]) ("[" [7,8]) ("]" [11,12]) ("[" [12,13]) ("]" [16,17]))) (link_reference_definition [19,31] (link_label [19,24] ("[" [19,20]) ("]" [23,24])) (":" [24,25]) (link_destination [26,30] ("/" [26,27])))))`,
	},
	{
		name:    "inline_link_escaped_bracket",
		grammar: "markdown_inline",
		source:  "[link \\[bar](/uri)",
		want:    `(inline [0,18] (inline_link [0,18] ("[" [0,1]) (link_text [1,11] (backslash_escape [6,8])) ("]" [11,12]) ("(" [12,13]) (link_destination [13,17] ("/" [13,14])) (")" [17,18])))`,
	},
	{
		name:           "inline_attr_entity",
		grammar:        "markdown_inline",
		source:         "foo <a href=\"&ouml;\">",
		want:           `(inline [0,21] (html_tag [4,21] ("<" [4,5]) ("=" [11,12]) ("\"" [12,13]) ("&" [13,14]) (";" [18,19]) ("\"" [19,20]) (">" [20,21])))`,
		defaultDiffers: true,
	},
	{
		name:           "inline_attr_backslash",
		grammar:        "markdown_inline",
		source:         "foo <a href=\"\\*\">",
		want:           `(inline [0,17] (html_tag [4,17] ("<" [4,5]) ("=" [11,12]) ("\"" [12,13]) ("\\" [13,14]) ("*" [14,15]) ("\"" [15,16]) (">" [16,17])))`,
		defaultDiffers: true,
	},
	{
		name:           "inline_attr_numeric_reference",
		grammar:        "markdown_inline",
		source:         "<a href=\"&#x6A;avascript:alert(1)\">x</a>",
		want:           `(inline [0,40] (html_tag [0,35] ("<" [0,1]) ("=" [7,8]) ("\"" [8,9]) ("&" [9,10]) ("#" [10,11]) (";" [14,15]) (":" [24,25]) ("(" [30,31]) (")" [32,33]) ("\"" [33,34]) (">" [34,35])) (html_tag [36,40] ("<" [36,37]) ("/" [37,38]) (">" [39,40])))`,
		defaultDiffers: true,
	},
	{
		name:           "inline_emphasis_over_bad_comment",
		grammar:        "markdown_inline",
		source:         "*a <!--> <a href=\" b*",
		want:           `(inline [0,21] (emphasis [0,21] (emphasis_delimiter [0,1]) ("<!--" [3,7]) (">" [7,8]) ("<" [9,10]) ("=" [16,17]) ("\"" [17,18]) (emphasis_delimiter [20,21])))`,
		defaultDiffers: true,
	},
}

func markdownPinnedCLanguage(grammar string) *gts.Language {
	if grammar == "markdown_inline" {
		return grammars.MarkdownInlineLanguage()
	}
	lang := grammars.MarkdownLanguage()
	grammars.AdaptScannerForLanguage("markdown", lang)
	return lang
}

// markdownPinnedCTree prints every node (named, anonymous, missing) with its
// byte span, in the form the pinned-C comparison in cgo_harness logs.
func markdownPinnedCTree(n *gts.Node, lang *gts.Language) string {
	var b strings.Builder
	var walk func(n *gts.Node)
	walk = func(n *gts.Node) {
		kind := n.Type(lang)
		if !n.IsNamed() {
			kind = fmt.Sprintf("%q", kind)
		}
		if n.IsMissing() {
			kind = "MISSING " + kind
		}
		fmt.Fprintf(&b, "(%s [%d,%d]", kind, n.StartByte(), n.EndByte())
		for i := 0; i < n.ChildCount(); i++ {
			b.WriteByte(' ')
			walk(n.Child(i))
		}
		b.WriteByte(')')
	}
	walk(n)
	return b.String()
}

func parseMarkdownPinnedCCase(t *testing.T, tc markdownPinnedCCase, core bool) (string, bool) {
	t.Helper()
	lang := markdownPinnedCLanguage(tc.grammar)
	parser := gts.NewParser(lang)
	if core {
		parser.SetAdmissionCandidateRoute(true)
	}
	tree, err := parser.Parse([]byte(tc.source))
	if err != nil {
		t.Fatal(err)
	}
	defer tree.Release()
	root := tree.RootNode()
	return markdownPinnedCTree(root, lang), root.HasError()
}

// TestMarkdownPinnedCTreesOnCore parses each input with both Markdown grammars
// certified for the core for the test run, as a certificate would serve them,
// and requires pinned C's tree.
func TestMarkdownPinnedCTreesOnCore(t *testing.T) {
	defer certifiedtest.Install(certified.Fresh|certified.Recover|certified.Scan|certified.Incr, []string{"ImmediateTokens", "ZeroWidthTokens", "LayoutFallbackLexState", "AfterWhitespaceLexState"}, nil)()
	for _, tc := range markdownPinnedCCases {
		t.Run(tc.name, func(t *testing.T) {
			got, hasError := parseMarkdownPinnedCCase(t, tc, true)
			if got != tc.want || hasError {
				t.Fatalf("core tree for %q:\n got  %s (HasError=%v)\n want %s", tc.source, got, hasError, tc.want)
			}
		})
	}
}

// TestMarkdownPinnedCTreesOnDefaultRoute requires pinned C's tree on the
// default route for every input that is not a known difference, and for every
// input once its grammar's blob is certified (the default route is then the
// core). A known difference that starts to match pinned C fails the test, so
// the list only shrinks.
func TestMarkdownPinnedCTreesOnDefaultRoute(t *testing.T) {
	for _, tc := range markdownPinnedCCases {
		t.Run(tc.name, func(t *testing.T) {
			lang := markdownPinnedCLanguage(tc.grammar)
			sha, ok := lang.GrammarBlobSHA256()
			_, certifiedBlob := certified.Blobs[sha]
			got, hasError := parseMarkdownPinnedCCase(t, tc, false)
			matches := got == tc.want && !hasError
			switch {
			case matches && tc.defaultDiffers && ok && !certifiedBlob:
				t.Fatalf("default route now gives pinned C's tree for %q; clear defaultDiffers", tc.source)
			case !matches && (!tc.defaultDiffers || certifiedBlob):
				t.Fatalf("default tree for %q:\n got  %s (HasError=%v)\n want %s", tc.source, got, hasError, tc.want)
			case !matches:
				t.Logf("known difference until %s is certified:\n got  %s\n want %s", tc.grammar, got, tc.want)
			}
		})
	}
}
