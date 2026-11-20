//go:build cgo && treesitter_c_parity

package cgoharness

import (
	"os"
	"testing"

	g "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/internal/benchfixtures"
	"github.com/odvcencio/gotreesitter/internal/ccore/bind"
)

func TestGraduatedTSXContextualArrowMatchesPinnedC(t *testing.T) {
	// The declarations preceding the typed arrow and the JSX following it
	// reproduce issue #1669. The arrow alone parses cleanly in both engines.
	src, err := os.ReadFile("testdata/issue_1669_contextual_arrow.tsx")
	if err != nil {
		t.Fatal(err)
	}
	gl, cl := ccoreLanguages(t, "tsx")
	ct := compactT3ParseC(t, cl, src)
	defer ct.Close()
	_, before, _ := bind.RouteCounters()
	tree, err := g.NewParser(gl).ParseStrict(src)
	if err != nil {
		t.Fatal(err)
	}
	if tree == nil {
		t.Fatal("missing TSX tree")
	}
	defer tree.Release()
	_, after, _ := bind.RouteCounters()
	root := tree.RootNode()
	if after-before != 1 || tree.ParseStopReason() != g.ParseStopAccepted || root == nil ||
		root.HasError() || root.EndByte() != uint32(len(src)) || ct.RootNode().HasError() {
		t.Fatalf("contextual arrow: core fresh=%d, stop=%s, Go root=%v, C error=%t",
			after-before, tree.ParseStopReason(), root != nil, ct.RootNode().HasError())
	}
	got, err := benchfixtures.InspectGoTree(root, gl)
	if err != nil {
		t.Fatal(err)
	}
	want, err := canonicalCTreeInspection(ct.RootNode())
	if err != nil {
		t.Fatal(err)
	}
	if got.SHA256 != want.SHA256 {
		t.Fatalf("TSX tree differs from pinned C: Go %s, C %s", got.SHA256, want.SHA256)
	}
}
