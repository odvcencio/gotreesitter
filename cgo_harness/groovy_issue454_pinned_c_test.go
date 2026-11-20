//go:build cgo && treesitter_c_parity

package cgoharness

import (
	"strings"
	"testing"

	g "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/internal/benchfixtures"
	"github.com/odvcencio/gotreesitter/internal/ccore/bind"
)

func TestGroovyIssue454LaterJuxtaposedCallMatchesPinnedC(t *testing.T) {
	src := []byte("def f(a,b) {\n def x0 = a + b\n println \"hi\"\n}\n")
	gl, cl := ccoreLanguages(t, "groovy")
	cTree := compactT3ParseC(t, cl, src)
	defer cTree.Close()

	_, freshBefore, _ := bind.RouteCounters()
	tree, err := g.NewParser(gl).Parse(src)
	if err != nil || tree == nil {
		t.Fatalf("certified default parse: %v", err)
	}
	defer tree.Release()
	_, freshAfter, _ := bind.RouteCounters()
	root := tree.RootNode()
	if freshAfter-freshBefore != 1 || tree.ParseStopReason() != g.ParseStopAccepted ||
		root == nil || root.HasError() || root.EndByte() != uint32(len(src)) {
		t.Fatalf("certified default: fresh=%d stop=%s root=%v", freshAfter-freshBefore, tree.ParseStopReason(), root)
	}
	if !strings.Contains(root.SExpr(gl), "(juxt_function_call ") {
		t.Fatal("second statement is missing its juxtaposed call")
	}
	got, err := benchfixtures.InspectGoTree(root, gl)
	if err != nil {
		t.Fatal(err)
	}
	want, err := canonicalCTreeInspection(cTree.RootNode())
	if err != nil {
		t.Fatal(err)
	}
	if got.SHA256 != want.SHA256 {
		t.Fatalf("public tree differs from pinned C: Go %s C %s", got.SHA256, want.SHA256)
	}
	t.Logf("certified default and pinned C agree on %s", got.SHA256)
}
