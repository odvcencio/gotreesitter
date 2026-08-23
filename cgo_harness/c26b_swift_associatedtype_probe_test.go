//go:build cgo && treesitter_c_parity

package cgoharness

import (
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	gotreesitter "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
	"github.com/odvcencio/gotreesitter/internal/benchfixtures"
	sitter "github.com/tree-sitter/go-tree-sitter"
)

func TestC26BAssociatedtypeConformanceWitness(t *testing.T) {
	cases := []struct {
		name string
		src  []byte
	}{
		{
			name: "minimal",
			src:  []byte("protocol P {\n  associatedtype Stride: SignedNumeric, Comparable\n}\n"),
		},
		{
			name: "corpus",
			src: func() []byte {
				src, err := os.ReadFile(filepath.Join("..", "grammars", "testdata", "swift_corpus", "stdlib_Stride.swift"))
				if err != nil {
					t.Fatalf("read Swift corpus witness: %v", err)
				}
				return src
			}(),
		},
	}

	goLang := grammars.SwiftLanguage()
	cLang, err := ParityCLanguage("swift")
	if err != nil {
		t.Fatalf("load locked Swift C parser: %v", err)
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			goParser := gotreesitter.NewParser(goLang)
			goTree, err := goParser.Parse(test.src)
			if err != nil {
				t.Fatalf("parse Swift with Go: %v", err)
			}
			defer goTree.Release()

			cParser := sitter.NewParser()
			defer cParser.Close()
			if err := cParser.SetLanguage(cLang); err != nil {
				t.Fatalf("set locked Swift language: %v", err)
			}
			cTree := cParser.Parse(test.src, nil)
			if cTree == nil || cTree.RootNode() == nil {
				t.Fatal("locked C parser returned no tree")
			}
			defer cTree.Close()

			goInspection, err := benchfixtures.InspectGoTree(goTree.RootNode(), goLang)
			if err != nil {
				t.Fatalf("inspect Go tree: %v", err)
			}
			cDigest, err := COracleDeepDigest(cTree)
			if err != nil {
				t.Fatalf("inspect locked C tree: %v", err)
			}
			fmt.Printf("C26B_ASSOC name=%s bytes=%d source_sha256=%x go_sha256=%s c_sha256=%s go_root=%d:%d go_error=%t c_root=%d:%d c_error=%t go_error_node=%s c_error_node=%s first_diff=%+v\n",
				test.name,
				len(test.src),
				sha256.Sum256(test.src),
				goInspection.SHA256,
				cDigest,
				goTree.RootNode().StartByte(),
				goTree.RootNode().EndByte(),
				goTree.RootNode().HasError(),
				cTree.RootNode().StartByte(),
				cTree.RootNode().EndByte(),
				cTree.RootNode().HasError(),
				c26bFirstGoError(goTree.RootNode(), goLang, "/source_file"),
				c26bFirstCError(cTree.RootNode(), "/source_file"),
				FirstDivergenceDumpV1(goTree.RootNode(), goLang, cTree.RootNode()),
			)
			if target := strings.Index(string(test.src), "associatedtype Stride: SignedNumeric, Comparable"); target >= 0 {
				fmt.Printf("C26B_TARGET name=%s target=%d go_context=%s c_context=%s\n",
					test.name,
					target,
					strings.Join(c26bGoErrorContext(goTree.RootNode(), goLang, uint32(target), "/source_file"), " -> "),
					strings.Join(c26bCErrorContext(cTree.RootNode(), uint32(target), "/source_file"), " -> "),
				)
			}
			if FirstDivergenceDumpV1(goTree.RootNode(), goLang, cTree.RootNode()) == nil {
				t.Fatal("associatedtype witness unexpectedly matches locked C")
			}
		})
	}
}

func c26bGoErrorContext(node *gotreesitter.Node, lang *gotreesitter.Language, target uint32, path string) []string {
	if node == nil || target < node.StartByte() || target > node.EndByte() {
		return nil
	}
	current := []string(nil)
	if node.Type(lang) == "ERROR" {
		current = append(current, fmt.Sprintf("%s[%d:%d] children=%d", path, node.StartByte(), node.EndByte(), node.ChildCount()))
	}
	for i := 0; i < node.ChildCount(); i++ {
		child := node.Child(i)
		if child == nil {
			continue
		}
		if nested := c26bGoErrorContext(child, lang, target, fmt.Sprintf("%s/%s[%d]", path, child.Type(lang), i)); nested != nil {
			current = append(current, nested...)
		}
	}
	return current
}

func c26bCErrorContext(node *sitter.Node, target uint32, path string) []string {
	if node == nil || target < uint32(node.StartByte()) || target > uint32(node.EndByte()) {
		return nil
	}
	current := []string(nil)
	if node.Kind() == "ERROR" {
		current = append(current, fmt.Sprintf("%s[%d:%d] children=%d", path, node.StartByte(), node.EndByte(), node.ChildCount()))
	}
	for i := 0; i < int(node.ChildCount()); i++ {
		child := node.Child(uint(i))
		if child == nil {
			continue
		}
		if nested := c26bCErrorContext(child, target, fmt.Sprintf("%s/%s[%d]", path, child.Kind(), i)); nested != nil {
			current = append(current, nested...)
		}
	}
	return current
}

func c26bFirstGoError(node *gotreesitter.Node, lang *gotreesitter.Language, path string) string {
	if node == nil {
		return "<nil>"
	}
	if node.Type(lang) == "ERROR" {
		return fmt.Sprintf("%s[%d:%d] children=%d", path, node.StartByte(), node.EndByte(), node.ChildCount())
	}
	for i := 0; i < node.ChildCount(); i++ {
		child := node.Child(i)
		if child == nil {
			continue
		}
		if got := c26bFirstGoError(child, lang, fmt.Sprintf("%s/%s[%d]", path, child.Type(lang), i)); got != "<nil>" {
			return got
		}
	}
	return "<nil>"
}

func c26bFirstCError(node *sitter.Node, path string) string {
	if node == nil {
		return "<nil>"
	}
	if node.Kind() == "ERROR" {
		return fmt.Sprintf("%s[%d:%d] children=%d", path, node.StartByte(), node.EndByte(), node.ChildCount())
	}
	for i := 0; i < int(node.ChildCount()); i++ {
		child := node.Child(uint(i))
		if child == nil {
			continue
		}
		if got := c26bFirstCError(child, fmt.Sprintf("%s/%s[%d]", path, child.Kind(), i)); got != "<nil>" {
			return got
		}
	}
	return "<nil>"
}
