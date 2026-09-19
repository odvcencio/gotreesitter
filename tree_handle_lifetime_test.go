package gotreesitter_test

import (
	"testing"

	gts "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
)

const treeHandleLifetimeSource = "package main\n\nfunc main() {\n\tx := 1\n\t_ = x\n}\n"

// TestTreeReleaseTwiceKeepsLaterParseValid checks that a second Release on a
// released tree does nothing. The second call must not free the tree of a
// later parse.
func TestTreeReleaseTwiceKeepsLaterParseValid(t *testing.T) {
	lang := grammars.GoLanguage()
	parser := gts.NewParser(lang)
	src := []byte(treeHandleLifetimeSource)

	first, err := parser.Parse(src)
	if err != nil {
		t.Fatalf("first parse: %v", err)
	}
	want := first.RootNode().SExpr(lang)
	first.Release()

	second, err := parser.Parse(src)
	if err != nil {
		t.Fatalf("second parse: %v", err)
	}
	defer second.Release()
	if second == first {
		t.Fatal("second parse reused the released *Tree value")
	}
	first.Release()

	root := second.RootNode()
	if root == nil {
		t.Fatal("stale Release freed the later parse result")
	}
	if got := root.SExpr(lang); got != want {
		t.Fatalf("later parse changed after stale Release:\ngot  %s\nwant %s", got, want)
	}
}

// TestParseIncrementalUnchangedResultSurvivesOldTreeRelease checks the C idiom.
// The caller releases the old tree right after an incremental parse. An
// unchanged parse returns the old tree itself, so the result must hold its
// own handle.
func TestParseIncrementalUnchangedResultSurvivesOldTreeRelease(t *testing.T) {
	lang := grammars.GoLanguage()
	src := []byte(treeHandleLifetimeSource)
	for _, tc := range []struct {
		name  string
		parse func(p *gts.Parser, old *gts.Tree) (*gts.Tree, error)
	}{
		{"ParseIncremental", func(p *gts.Parser, old *gts.Tree) (*gts.Tree, error) {
			return p.ParseIncremental(src, old)
		}},
		{"ParseIncrementalProfiled", func(p *gts.Parser, old *gts.Tree) (*gts.Tree, error) {
			tree, _, err := p.ParseIncrementalProfiled(src, old)
			return tree, err
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			parser := gts.NewParser(lang)
			old, err := parser.Parse(src)
			if err != nil {
				t.Fatalf("initial parse: %v", err)
			}
			want := old.RootNode().SExpr(lang)

			result, err := tc.parse(parser, old)
			if err != nil {
				t.Fatalf("incremental parse: %v", err)
			}
			old.Release()
			root := result.RootNode()
			if root == nil {
				t.Fatal("releasing the old tree freed the unchanged incremental result")
			}
			if got := root.SExpr(lang); got != want {
				t.Fatalf("unchanged result changed:\ngot  %s\nwant %s", got, want)
			}
			result.Release()
			if result.RootNode() != nil {
				t.Fatal("tree stayed live after its last handle was released")
			}
		})
	}
}

// TestParseIncrementalUnchangedResultSupportsIdentityCheck checks the older
// Go idiom. The caller releases the old tree only when the result is a
// different tree. The result must stay valid.
func TestParseIncrementalUnchangedResultSupportsIdentityCheck(t *testing.T) {
	lang := grammars.GoLanguage()
	parser := gts.NewParser(lang)
	src := []byte(treeHandleLifetimeSource)
	tree, err := parser.Parse(src)
	if err != nil {
		t.Fatalf("initial parse: %v", err)
	}
	for i := 0; i < 3; i++ {
		old := tree
		tree, err = parser.ParseIncremental(src, tree)
		if err != nil {
			t.Fatalf("incremental parse %d: %v", i, err)
		}
		if old != tree {
			old.Release()
		}
		if tree.RootNode() == nil {
			t.Fatalf("incremental parse %d returned a released tree", i)
		}
	}
	tree.Release()
}
