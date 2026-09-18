package gotreesitter_test

import (
	"bytes"
	"fmt"
	"testing"

	gts "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
	"github.com/odvcencio/gotreesitter/internal/benchfixtures"
)

// The issue #454 deletion now resynchronizes and reuses the unchanged suffix.
// Require complete fresh-tree equality, real reuse, and the existing allocation bound.
func TestIncrementalReuseBudgetAllowsResynchronizedEdit(t *testing.T) {
	var b bytes.Buffer
	b.WriteString("#include <stdio.h>\n\n")
	for i := 0; b.Len() < 137<<10; i++ {
		fmt.Fprintf(&b, "int f%d(int a, int b) {\n    int x0 = a + b;\n    printf(\"f%d %%d\\n\", x0);\n    return x0;\n}\n\n", i, i)
	}
	source := b.Bytes()
	site := bytes.Index(source, []byte("x0"))
	if site < 0 {
		t.Fatal("fixture has no edit site")
	}
	edited := append(append([]byte{}, source[:site]...), source[site+1:]...)
	row, col := uint32(0), uint32(0)
	for _, c := range source[:site] {
		if c == '\n' {
			row++
			col = 0
		} else {
			col++
		}
	}
	edit := gts.InputEdit{
		StartByte: uint32(site), OldEndByte: uint32(site + 1), NewEndByte: uint32(site),
		StartPoint: gts.Point{Row: row, Column: col}, OldEndPoint: gts.Point{Row: row, Column: col + 1}, NewEndPoint: gts.Point{Row: row, Column: col},
	}
	lang := grammars.CLanguage()
	parser := gts.NewParser(lang)
	parser.SetAdmissionCandidateRoute(false)
	old, err := parser.Parse(source)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	defer old.Release()
	old.Edit(edit)
	incremental, profile, err := parser.ParseIncrementalProfiled(edited, old)
	if err != nil {
		t.Fatalf("incremental parse: %v", err)
	}
	if incremental != old {
		defer incremental.Release()
	}
	fresh, err := parser.Parse(edited)
	if err != nil {
		t.Fatalf("fresh parse: %v", err)
	}
	defer fresh.Release()
	for _, tree := range []*gts.Tree{incremental, fresh} {
		if tree == nil || tree.RootNode() == nil || tree.ParseStoppedEarly() || tree.RootNode().EndByte() != uint32(len(edited)) {
			t.Fatal("parse did not cover the complete edited source")
		}
	}
	got, err := benchfixtures.InspectGoTree(incremental.RootNode(), lang)
	if err != nil {
		t.Fatal(err)
	}
	want, err := benchfixtures.InspectGoTree(fresh.RootNode(), lang)
	if err != nil {
		t.Fatal(err)
	}
	if got.SHA256 != want.SHA256 {
		t.Fatal("incremental tree does not match the fresh parse")
	}
	if profile.ReuseUnsupported || profile.ReuseUnsupportedReason != "" || !profile.OldTreeReuseRoute || profile.ReusedSubtrees == 0 {
		t.Fatalf("expected suffix reuse without a full retry: profile=%+v", profile)
	}
	if profile.ReusedBytes == 0 || profile.ReusedBytes > uint64(len(edited)) {
		t.Fatalf("reused bytes = %d, source bytes = %d", profile.ReusedBytes, len(edited))
	}
	// Preserve the historical allocation ceiling even when the retry is unnecessary.
	if profile.NewNodesAllocated > 800_000 {
		t.Fatalf("incremental edit built %d nodes", profile.NewNodesAllocated)
	}
}
