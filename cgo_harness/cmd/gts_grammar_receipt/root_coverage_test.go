//go:build cgo && treesitter_c_parity

package main

import (
	"testing"

	gotreesitter "github.com/odvcencio/gotreesitter"
	cgo_harness "github.com/odvcencio/gotreesitter/cgo_harness"
	"github.com/odvcencio/gotreesitter/grammars"
	sitter "github.com/tree-sitter/go-tree-sitter"
)

// Locked C excludes leading padding from the root span, so on "\na" its root
// starts at byte 1. These six grammars failed the whole-input invariant only
// because it required StartByte() == 0 while Go already matched C's span.
func TestRootCoversLikeCLeadingPadding(t *testing.T) {
	t.Chdir("../..") // The locked C loader resolves paths from the harness root.
	for _, name := range []string{"bicep", "elsa", "verilog", "vhdl", "dhall", "pug"} {
		t.Run(name, func(t *testing.T) {
			entry := grammars.DetectLanguageByName(name)
			if entry == nil || entry.Language() == nil {
				t.Fatalf("grammar %s unavailable", name)
			}
			lang := entry.Language()
			cLang, err := cgo_harness.COracleLanguage(name)
			if err != nil {
				t.Fatal(err)
			}
			cParser := sitter.NewParser()
			defer cParser.Close()
			if err := cParser.SetLanguage(cLang); err != nil {
				t.Fatal(err)
			}
			parse := func(src string) (*gotreesitter.Tree, *sitter.Tree) {
				goTree, err := gotreesitter.NewParser(lang).Parse([]byte(src))
				if err != nil {
					t.Fatal(err)
				}
				cTree := cParser.Parse([]byte(src), nil)
				if cTree == nil || cTree.RootNode() == nil {
					t.Fatal("locked C parse returned no tree")
				}
				return goTree, cTree
			}

			source := "\na"
			goTree, cTree := parse(source)
			defer goTree.Release()
			defer cTree.Close()
			root, cRoot := goTree.RootNode(), cTree.RootNode()
			if uint(root.StartByte()) != cRoot.StartByte() || uint(root.EndByte()) != cRoot.EndByte() {
				t.Fatalf("Go root %d..%d, C root %d..%d", root.StartByte(), root.EndByte(), cRoot.StartByte(), cRoot.EndByte())
			}
			if !rootCoversLikeC(root, len(source), cRoot) {
				t.Fatalf("root %d..%d equals C's span but was rejected", root.StartByte(), root.EndByte())
			}

			// Without a C root, or when C's root starts at byte 0, the strict
			// rule applies: a root that starts after byte 0 does not cover.
			if root.StartByte() > 0 {
				if rootCoversLikeC(root, len(source), nil) {
					t.Fatal("nonzero-start root accepted without a C root")
				}
				zeroGo, zeroStart := parse("a")
				defer zeroGo.Release()
				defer zeroStart.Close()
				if zeroStart.RootNode().StartByte() == 0 && rootCoversLikeC(root, len(source), zeroStart.RootNode()) {
					t.Fatal("nonzero-start root accepted against a C root that starts at byte 0")
				}
			}
		})
	}
}
