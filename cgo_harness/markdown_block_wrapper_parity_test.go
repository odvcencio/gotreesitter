//go:build cgo && treesitter_c_parity

package cgoharness

import (
	"fmt"
	"testing"

	gotreesitter "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
	sitter "github.com/tree-sitter/go-tree-sitter"
)

func TestMarkdownBlockWrappersMatchLockedC(t *testing.T) {
	lang := grammars.MarkdownLanguage()
	cLang, err := COracleLanguage("markdown")
	if err != nil {
		t.Fatal(err)
	}
	p := sitter.NewParser()
	defer p.Close()
	if err := p.SetLanguage(cLang); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct{ name, source string }{
		{"thematic_break", "***\n"},
		{"fenced_code", "```go\npackage main\n```\n"},
		{"block_quote", "> quote\n"},
		{"list", "- one\n- two\n"},
		{"indented_code", "    code\n"},
		{"html_block", "<div>\ntext\n</div>\n"},
		{"link_reference", "[a]: /url\n"},
		{"link_reference_title", "[a]: /url \"t\"\n"},
		{"consecutive_references", "[a]: /u1\n[b]: /u2\n"},
		{"reference_after_paragraph", "p\n\n[a]: /url\n"},
		{"reference_title_next_line", "[a]: /url\n\"title\"\n"},
	} {
		source := tc.source
		cTree := p.Parse([]byte(source), nil)
		if cTree == nil {
			t.Fatal("C returned no tree")
		}
		for _, candidate := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/candidate=%t", tc.name, candidate), func(t *testing.T) {
				gp := gotreesitter.NewParser(lang)
				gp.SetAdmissionCandidateRoute(candidate)
				tree, err := gp.Parse([]byte(source))
				if err != nil {
					t.Fatal(err)
				}
				defer tree.Release()
				if diff := FirstDivergenceDumpV1(tree.RootNode(), lang, cTree.RootNode()); diff != nil {
					t.Errorf("source=%q divergence=%+v", source, diff)
				}
			})
		}
		cTree.Close()
	}
}
