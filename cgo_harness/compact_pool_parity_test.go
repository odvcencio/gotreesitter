//go:build cgo && treesitter_c_parity

package cgoharness

import (
	"os"
	"testing"

	sitter "github.com/tree-sitter/go-tree-sitter"
)

// TestCompactPoolFreshParsersMatchLockedC runs one grammar per process. Each
// fresh Parser must reproduce the locked C tree after another caller changed
// the pooled core's graph. Earlier returned trees remain live throughout.
func TestCompactPoolFreshParsersMatchLockedC(t *testing.T) {
	name := os.Getenv("GTS_COMPACT_POOL_LANGUAGE")
	if name == "" {
		t.Skip("set GTS_COMPACT_POOL_LANGUAGE to one grammar")
	}
	tc, ok := parityCaseByName(name)
	if !ok {
		t.Fatalf("unknown grammar %q", name)
	}
	candidate := true
	tc.candidateRoute = &candidate
	source := normalizedSource(name, tc.source)
	cLanguage, err := COracleLanguage(name)
	if err != nil {
		t.Fatal(err)
	}
	cParser := sitter.NewParser()
	defer cParser.Close()
	if err := cParser.SetLanguage(cLanguage); err != nil {
		t.Fatal(err)
	}
	cTree := cParser.Parse(source, nil)
	if cTree == nil {
		t.Fatal("locked C returned no tree")
	}
	defer cTree.Close()
	first, lang, err := parseWithGo(tc, source, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer first.Release()
	for attempt := 0; attempt < 8; attempt++ {
		// Empty input exercises a different graph and scanner EOF state. Its
		// recovery shape is outside this clean smoke sample's parity assertion.
		empty, _, _ := parseWithGo(tc, nil, nil)
		if empty != nil {
			empty.Release()
		}
		tree, _, err := parseWithGo(tc, source, nil)
		if err != nil {
			t.Fatal(err)
		}
		if diff := FirstDivergenceDumpV1(tree.RootNode(), lang, cTree.RootNode()); diff != nil {
			tree.Release()
			t.Fatalf("fresh parser %d differs from locked C: %+v", attempt, diff)
		}
		if tree.RootNode().HasError() != cTree.RootNode().HasError() {
			tree.Release()
			t.Fatalf("fresh parser %d changed HasError", attempt)
		}
		tree.Release()
		if diff := FirstDivergenceDumpV1(first.RootNode(), lang, cTree.RootNode()); diff != nil {
			t.Fatalf("pool reuse changed the earlier live tree: %+v", diff)
		}
	}
}
