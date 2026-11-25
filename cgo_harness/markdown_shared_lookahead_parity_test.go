//go:build cgo && treesitter_c_parity

package cgoharness

import (
	"fmt"
	"strings"
	"testing"

	gotreesitter "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
	"github.com/odvcencio/gotreesitter/internal/ccore/bind/bindtest"
	"github.com/odvcencio/gotreesitter/internal/ccore/certified"
	sitter "github.com/tree-sitter/go-tree-sitter"
)

// markdownSharedLookaheadCases are inputs where a Markdown consumer saw the
// default Go route depart from pinned C: a link whose text holds an escaped
// bracket (block grammar), inline HTML whose attribute value holds an entity
// or a backslash, and emphasis around a malformed comment and an unclosed tag
// (inline grammar). The
// inline sources are the text of the inline node that the block grammar
// reports, parsed on their own, as the consumer parses them.
//
// defaultDiffers marks a known difference of the default engine while the
// grammar has no certificate: it reads one lookahead for every live GLR stack,
// so a stack that needs a different token at the same byte dies, where C lexes
// each version in its own mode. The core matches pinned C on all of them. The
// block grammar has a certificate, so its inputs run on the core by default.
var markdownSharedLookaheadCases = []struct {
	name, grammar, source string
	defaultDiffers        bool
}{
	{"block_link_escaped_bracket", "markdown", "[link \\[bar](/uri)\n", false},
	{"block_link_escaped_bracket_after_break", "markdown", "***\n\n[link \\[bar](/uri)\n", false},
	{"block_reference_link_escaped_bracket", "markdown", "[link \\[bar][ref]\n\n[ref]: /uri\n", false},
	{"inline_link_escaped_bracket", "markdown_inline", "[link \\[bar](/uri)", false},
	{"inline_attr_entity", "markdown_inline", "foo <a href=\"&ouml;\">", true},
	{"inline_attr_backslash", "markdown_inline", "foo <a href=\"\\*\">", true},
	{"inline_attr_numeric_reference", "markdown_inline", "<a href=\"&#x6A;avascript:alert(1)\">x</a>", true},
	{"inline_emphasis_over_bad_comment", "markdown_inline", "*a <!--> <a href=\" b*", true},
}

func markdownSharedLookaheadLanguage(grammar string) *gotreesitter.Language {
	if grammar == "markdown_inline" {
		return grammars.MarkdownInlineLanguage()
	}
	lang := grammars.MarkdownLanguage()
	grammars.AdaptScannerForLanguage("markdown", lang)
	return lang
}

func markdownSharedLookaheadCertified(lang *gotreesitter.Language) bool {
	sha, ok := lang.GrammarBlobSHA256()
	if !ok {
		return false
	}
	_, certifiedBlob := certified.Blobs[sha]
	return certifiedBlob
}

func markdownSharedLookaheadCTree(n *sitter.Node) string {
	var b strings.Builder
	var walk func(n *sitter.Node)
	walk = func(n *sitter.Node) {
		kind := n.Kind()
		if !n.IsNamed() {
			kind = fmt.Sprintf("%q", kind)
		}
		if n.IsMissing() {
			kind = "MISSING " + kind
		}
		fmt.Fprintf(&b, "(%s [%d,%d]", kind, n.StartByte(), n.EndByte())
		for i := uint(0); i < n.ChildCount(); i++ {
			b.WriteByte(' ')
			walk(n.Child(i))
		}
		b.WriteByte(')')
	}
	walk(n)
	return b.String()
}

// TestMarkdownSharedLookaheadInputsMatchLockedC compares the default Go route with
// pinned C for each input: symbols, fields, spans, extras, missing and error
// flags, and the deep digest. A known difference is logged while its grammar
// has no certificate, and compared once it has one.
func TestMarkdownSharedLookaheadInputsMatchLockedC(t *testing.T) {
	markdownSharedLookaheadCompare(t, false)
}

// TestMarkdownSharedLookaheadInputsCoreMatchesLockedC makes the same comparison with
// both Markdown grammars certified for the core for the test run, as a
// certificate would serve them. It needs -tags gts_ccore_test_certify.
func TestMarkdownSharedLookaheadInputsCoreMatchesLockedC(t *testing.T) {
	restore, err := bindtest.CertifyForTests(1|2|4|8, []string{"ImmediateTokens", "ZeroWidthTokens", "LayoutFallbackLexState", "AfterWhitespaceLexState"}, nil)
	if err != nil {
		t.Skip(err)
	}
	defer restore()
	markdownSharedLookaheadCompare(t, true)
}

func markdownSharedLookaheadCompare(t *testing.T, core bool) {
	route := "default route"
	if core {
		route = "core route"
	}
	for _, tc := range markdownSharedLookaheadCases {
		t.Run(tc.name, func(t *testing.T) {
			cLang, err := COracleLanguage(tc.grammar)
			if err != nil {
				t.Fatal(err)
			}
			cParser := sitter.NewParser()
			defer cParser.Close()
			if err := cParser.SetLanguage(cLang); err != nil {
				t.Fatal(err)
			}
			source := []byte(tc.source)
			cTree := cParser.Parse(source, nil)
			if cTree == nil || cTree.RootNode() == nil {
				t.Fatal("pinned C returned no tree")
			}
			defer cTree.Close()
			t.Logf("pinned C %s %q: %s", tc.grammar, tc.source, cTree.RootNode().ToSexp())
			t.Logf("pinned C full: %s", markdownSharedLookaheadCTree(cTree.RootNode()))
			cDigest, err := COracleDeepDigest(cTree)
			if err != nil {
				t.Fatal(err)
			}

			lang := markdownSharedLookaheadLanguage(tc.grammar)
			parser := gotreesitter.NewParser(lang)
			if core {
				parser.SetAdmissionCandidateRoute(true)
			}
			tree, err := parser.Parse(source)
			if err != nil {
				t.Fatal(err)
			}
			defer tree.Release()
			if !core && tc.defaultDiffers && !markdownSharedLookaheadCertified(lang) {
				if diff := FirstDivergenceDumpV1(tree.RootNode(), lang, cTree.RootNode()); diff != nil {
					t.Logf("known default-route difference until %s is certified: %+v", tc.grammar, diff)
					return
				}
			}
			assertJsdocLockedCTreeExact(t, tc.grammar+" "+route, tree, lang, cTree, cDigest)
		})
	}
}
