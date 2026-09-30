//go:build linux && cgo && treesitter_c_parity

package cgoharness

import (
	"bytes"
	"fmt"
	"testing"

	gts "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
	"github.com/odvcencio/gotreesitter/internal/benchfixtures"
	sitter "github.com/tree-sitter/go-tree-sitter"
)

// q6PythonIssue454Source reproduces the inline generator in issue #454,
// which PR #1290 measured. The general issue454bench generator uses x0 in
// every function and produces a different source and work count.
func q6PythonIssue454Source() []byte {
	var source bytes.Buffer
	for i := 0; source.Len() < 137*1024; i++ {
		fmt.Fprintf(&source, "def f%d(a, b):\n    x%d = a + b\n    print(\"f%d\", x%d)\n    return x%d\n\n", i, i, i, i, i)
	}
	return source.Bytes()
}

func TestQ6PythonStackCapLockedCReceipt(t *testing.T) {
	t.Setenv("GOT_GLR_MAX_STACKS", "")
	source := q6PythonIssue454Source()
	if len(source) != 140363 {
		t.Fatalf("original generator produced %d bytes", len(source))
	}
	language := grammars.PythonLanguage()
	cLanguage, err := COracleLanguage("python")
	if err != nil {
		t.Fatal(err)
	}
	cParser := sitter.NewParser()
	defer cParser.Close()
	if err := cParser.SetLanguage(cLanguage); err != nil {
		t.Fatal(err)
	}
	cTree := cParser.Parse(source, nil)
	if cTree == nil || cTree.RootNode() == nil {
		t.Fatal("C returned no tree")
	}
	defer cTree.Close()
	cDigest, err := canonicalCTreeInspection(cTree.RootNode())
	if err != nil {
		t.Fatal(err)
	}
	var visibleNodes uint64
	for _, count := range cDigest.NodeKinds {
		visibleNodes += count
	}
	if visibleNodes != 59105 || cTree.RootNode().HasError() {
		t.Fatalf("C nodes=%d error=%t", visibleNodes, cTree.RootNode().HasError())
	}
	for _, route := range []struct {
		candidate bool
		workNodes int
	}{{false, 371146}, {true, 123749}} {
		parser := gts.NewParser(language)
		parser.SetAdmissionCandidateRoute(route.candidate)
		tree, err := parser.Parse(source)
		if err != nil {
			t.Fatal(err)
		}
		defer tree.Release()
		goDigest, err := benchfixtures.InspectGoTree(tree.RootNode(), language)
		if err != nil {
			t.Fatal(err)
		}
		if diff := FirstDivergenceDumpV1(tree.RootNode(), language, cTree.RootNode()); diff != nil || goDigest.SHA256 != cDigest.SHA256 {
			t.Fatalf("candidate=%t Go=%s C=%s diff=%+v", route.candidate, goDigest.SHA256, cDigest.SHA256, diff)
		}
		if runtime := tree.ParseRuntime(); runtime.StopReason != gts.ParseStopAccepted || runtime.NodesAllocated != route.workNodes {
			t.Fatalf("candidate=%t stop=%s nodes=%d, want %d", route.candidate, runtime.StopReason, runtime.NodesAllocated, route.workNodes)
		}
		t.Logf("candidate=%t bytes=%d visible_nodes=%d work_nodes=%d C=%s", route.candidate, len(source), visibleNodes, route.workNodes, cDigest.SHA256)
	}
}
