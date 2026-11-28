//go:build cgo && treesitter_c_parity

package cgoharness

import (
	"fmt"
	"strings"
	"testing"

	gotreesitter "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
	sitter "github.com/tree-sitter/go-tree-sitter"
)

func dumpGoTree(n *gotreesitter.Node, lang *gotreesitter.Language, depth int) string {
	var b strings.Builder
	indent := strings.Repeat("  ", depth)
	b.WriteString(fmt.Sprintf("%s(%s [%d-%d] named=%v children=%d", indent, n.Type(lang), n.StartByte(), n.EndByte(), n.IsNamed(), n.ChildCount()))
	if n.ChildCount() == 0 {
		b.WriteString(")\n")
		return b.String()
	}
	b.WriteString("\n")
	for i := 0; i < n.ChildCount(); i++ {
		b.WriteString(dumpGoTree(n.Child(i), lang, depth+1))
	}
	b.WriteString(fmt.Sprintf("%s)\n", indent))
	return b.String()
}

func dumpCTree(n *sitter.Node, depth int) string {
	var b strings.Builder
	indent := strings.Repeat("  ", depth)
	b.WriteString(fmt.Sprintf("%s(%s [%d-%d] named=%v children=%d", indent, n.Kind(), n.StartByte(), n.EndByte(), n.IsNamed(), n.ChildCount()))
	if n.ChildCount() == 0 {
		b.WriteString(")\n")
		return b.String()
	}
	b.WriteString("\n")
	for i := uint(0); i < uint(n.ChildCount()); i++ {
		b.WriteString(dumpCTree(n.Child(i), depth+1))
	}
	b.WriteString(fmt.Sprintf("%s)\n", indent))
	return b.String()
}

func TestParityNoSkip(t *testing.T) {
	for _, name := range []string{"ini", "make", "julia", "d", "scss"} {
		name := name
		t.Run(name, func(t *testing.T) {
			tc := parityCase{name: name, source: grammars.ParseSmokeSample(name)}
			src := normalizedSource(tc.name, tc.source)
			t.Logf("source (%d bytes): %q", len(src), string(src))

			goTree, goLang, err := parseWithGo(tc, src, nil)
			if err != nil {
				t.Fatalf("Go parse error: %v", err)
			}
			t.Logf("Go tree:\n%s", dumpGoTree(goTree.RootNode(), goLang, 0))

			cLang, err := ParityCLanguage(name)
			if err != nil {
				t.Skipf("C parser unavailable: %v", err)
			}
			cParser := sitter.NewParser()
			defer cParser.Close()
			if err := cParser.SetLanguage(cLang); err != nil {
				t.Skipf("C SetLanguage: %v", err)
			}
			cTree := cParser.Parse(src, nil)
			if cTree == nil {
				t.Fatal("C nil tree")
			}
			defer cTree.Close()
			t.Logf("C tree:\n%s", dumpCTree(cTree.RootNode(), 0))

			var errs []string
			compareNodes(goTree.RootNode(), goLang, cTree.RootNode(), "root", &errs)
			if len(errs) > 0 {
				t.Errorf("DIVERGENCES: %s", strings.Join(errs, "; "))
			} else {
				t.Logf("PARITY OK")
			}
		})
	}
}
