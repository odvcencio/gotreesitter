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
	"github.com/odvcencio/gotreesitter/internal/benchfixtures"
	sitter "github.com/tree-sitter/go-tree-sitter"
)

func TestC26CMaskingShiftWitness(t *testing.T) {
	tests := []struct {
		name string
		src  []byte
	}{
		{
			name: "minimal",
			src:  []byte("let x = 1&<<7\n"),
		},
		{
			name: "corpus",
			src: func() []byte {
				path := filepath.Join("..", "grammars", "testdata", "swift_corpus", "stdlib_ASCII.swift")
				src, err := os.ReadFile(path)
				if err != nil {
					panic(err)
				}
				return src
			}(),
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			source := test.src
			sourceDigest := sha256.Sum256(source)
			goTree, goLang, err := parseWithGo(parityCase{name: "swift", source: string(source)}, source, nil)
			if err != nil {
				t.Fatalf("parse Swift with Go: %v", err)
			}
			defer releaseGoTree(goTree)

			cLang, err := ParityCLanguage("swift")
			if err != nil {
				t.Fatalf("load locked Swift C parser: %v", err)
			}
			cParser := sitter.NewParser()
			defer cParser.Close()
			if err := cParser.SetLanguage(cLang); err != nil {
				t.Fatalf("set locked Swift C parser language: %v", err)
			}
			cTree := cParser.Parse(source, nil)
			if cTree == nil || cTree.RootNode() == nil {
				t.Fatal("locked C parser returned no tree")
			}
			defer cTree.Close()

			goRoot := goTree.RootNode()
			cRoot := cTree.RootNode()
			goInspection, err := benchfixtures.InspectGoTree(goRoot, goLang)
			if err != nil {
				t.Fatalf("inspect Go tree: %v", err)
			}
			cDigest, err := COracleDeepDigest(cTree)
			if err != nil {
				t.Fatalf("inspect C tree: %v", err)
			}
			diff := FirstDivergenceDumpV1(goRoot, goLang, cRoot)
			t.Logf("C26C_MASK name=%s bytes=%d source_sha256=%x go_sha256=%s c_sha256=%s go_root=%d:%d go_children=%d go_error=%v c_root=%d:%d c_children=%d c_error=%v first_diff=%+v target=%d", test.name, len(source), sourceDigest, goInspection.SHA256, cDigest, goRoot.StartByte(), goRoot.EndByte(), goRoot.ChildCount(), goRoot.HasError(), cRoot.StartByte(), cRoot.EndByte(), int(cRoot.ChildCount()), cRoot.HasError(), diff, strings.Index(string(source), "1&<<7"))
			c26cLogGoRootChildren(t, "go", goRoot, goLang, source)
			c26cLogCRootChildren(t, "c", cRoot, source)
		})
	}
}

func c26cLogGoRootChildren(t *testing.T, label string, root *gotreesitter.Node, lang *gotreesitter.Language, source []byte) {
	t.Helper()
	limit := root.ChildCount()
	if limit > 12 {
		limit = 12
	}
	for i := 0; i < limit; i++ {
		child := root.Child(i)
		t.Logf("%s.root.child[%d] type=%q span=%d:%d named=%v extra=%v children=%d text=%s", label, i, child.Type(lang), child.StartByte(), child.EndByte(), child.IsNamed(), child.IsExtra(), child.ChildCount(), c26cText(source, child.StartByte(), child.EndByte()))
	}
}

func c26cLogCRootChildren(t *testing.T, label string, root *sitter.Node, source []byte) {
	t.Helper()
	limit := int(root.ChildCount())
	if limit > 12 {
		limit = 12
	}
	for i := 0; i < limit; i++ {
		child := root.Child(uint(i))
		t.Logf("%s.root.child[%d] kind=%q span=%d:%d named=%v extra=%v children=%d text=%q", label, i, child.Kind(), child.StartByte(), child.EndByte(), child.IsNamed(), child.IsExtra(), int(child.ChildCount()), c26cText(source, uint32(child.StartByte()), uint32(child.EndByte())))
	}
}

func c26cText(source []byte, start, end uint32) string {
	if start > end || end > uint32(len(source)) {
		return ""
	}
	text := string(source[start:end])
	if len(text) > 100 {
		text = text[:100]
	}
	return fmt.Sprintf("%q", text)
}
