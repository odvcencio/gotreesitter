//go:build cgo && treesitter_c_parity

package cgoharness

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	gts "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
	sitter "github.com/tree-sitter/go-tree-sitter"
)

func TestYAMLResultCompatibilityErrorsLockedC(t *testing.T) {
	cl, err := ParityCLanguage("yaml")
	if err != nil {
		t.Fatal(err)
	}
	identity, err := COracleIdentity("yaml")
	if err != nil {
		t.Fatal(err)
	}
	if identity.GrammarCommit != "a1c4812a73ec5e089de8e441fdea3a921e8d5079" {
		t.Fatalf("YAML fixtures need verification against new grammar %s", identity.GrammarCommit)
	}
	cp := sitter.NewParser()
	defer cp.Close()
	if err := cp.SetLanguage(cl); err != nil {
		t.Fatal(err)
	}
	gl := grammars.YamlLanguage()
	for _, name := range []string{"missing-colon", "indentation", "unclosed-flow"} {
		t.Run(name, func(t *testing.T) {
			base := filepath.Join("..", "testdata", "yaml_issue1400", name)
			source, err := os.ReadFile(base + ".yaml")
			if err != nil {
				t.Fatal(err)
			}
			want, err := os.ReadFile(base + ".tree")
			if err != nil {
				t.Fatal(err)
			}
			ct := cp.Parse(source, nil)
			defer ct.Close()
			var cSnapshot strings.Builder
			var visitC func(*sitter.Node, int, string)
			visitC = func(node *sitter.Node, depth int, field string) {
				sp, ep := node.StartPosition(), node.EndPosition()
				fmt.Fprintf(&cSnapshot, "%s%s named=%t extra=%t error=%t missing=%t has_error=%t bytes=%d:%d points=%d:%d-%d:%d field=%q\n", strings.Repeat("  ", depth), node.Kind(), node.IsNamed(), node.IsExtra(), node.IsError(), node.IsMissing(), node.HasError(), node.StartByte(), node.EndByte(), sp.Row, sp.Column, ep.Row, ep.Column, field)
				for i := uint(0); i < node.ChildCount(); i++ {
					visitC(node.Child(i), depth+1, node.FieldNameForChild(uint32(i)))
				}
			}
			visitC(ct.RootNode(), 0, "")
			if cSnapshot.String() != string(want) {
				t.Fatalf("fixture differs from locked C:\ngot:\n%s\nwant:\n%s", cSnapshot.String(), want)
			}
			gt, err := gts.NewParser(gl).Parse(source)
			if err != nil {
				t.Fatal(err)
			}
			defer gt.Release()
			var goSnapshot strings.Builder
			var visitGo func(*gts.Node, int, string)
			visitGo = func(node *gts.Node, depth int, field string) {
				sp, ep := node.StartPoint(), node.EndPoint()
				fmt.Fprintf(&goSnapshot, "%s%s named=%t extra=%t error=%t missing=%t has_error=%t bytes=%d:%d points=%d:%d-%d:%d field=%q\n", strings.Repeat("  ", depth), node.Type(gl), node.IsNamed(), node.IsExtra(), node.IsError(), node.IsMissing(), node.HasError(), node.StartByte(), node.EndByte(), sp.Row, sp.Column, ep.Row, ep.Column, field)
				for i := 0; i < node.ChildCount(); i++ {
					visitGo(node.Child(i), depth+1, node.FieldNameForChild(i, gl))
				}
			}
			visitGo(gt.RootNode(), 0, "")
			if goSnapshot.String() != cSnapshot.String() {
				t.Fatalf("Go tree differs from locked C:\nGo:\n%s\nC:\n%s", goSnapshot.String(), cSnapshot.String())
			}
		})
	}
}
