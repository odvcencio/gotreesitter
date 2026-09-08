package gotreesitter_test

import (
	"bytes"
	"fmt"
	"testing"

	gts "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
)

// TestIncrementalReuseBudgetDeclinesReuseHostileEdit is the issue #454 C
// single-byte delete: the edit turns `x0` into `0` and old-tree reuse
// resynchronizes nowhere, so the incremental attempt used to build 3.2
// million nodes before the memory budget stopped it. The reuse budget stops
// the attempt after a bounded number of nodes, and the parser runs one plain
// full parse, which the returned tree must match exactly.
func TestIncrementalReuseBudgetDeclinesReuseHostileEdit(t *testing.T) {
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
	old.Edit(edit)
	incremental, profile, err := parser.ParseIncrementalProfiled(edited, old)
	if err != nil {
		t.Fatalf("incremental parse: %v", err)
	}
	fresh, err := parser.Parse(edited)
	if err != nil {
		t.Fatalf("fresh parse: %v", err)
	}
	if got, want := incremental.RootNode().SExpr(lang), fresh.RootNode().SExpr(lang); got != want {
		t.Fatal("incremental tree does not match the fresh parse")
	}
	if profile.ReuseUnsupportedReason != "incremental_parse_reuse_budget_full_retry" {
		t.Fatalf("reuse unsupported reason = %q, want the reuse budget full retry; profile=%+v", profile.ReuseUnsupportedReason, profile)
	}
	// The bound is four times the fresh-parse arena estimate for this source
	// plus the plain full parse itself; the old behavior built 3.2 million.
	if profile.NewNodesAllocated > 800_000 {
		t.Fatalf("reuse-hostile edit built %d nodes before the full retry", profile.NewNodesAllocated)
	}
}
