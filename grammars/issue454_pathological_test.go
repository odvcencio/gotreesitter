package grammars

import (
	"bytes"
	"fmt"
	"testing"

	gotreesitter "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/internal/benchfixtures"
)

func TestIssue454BrokenMakeCompletes(t *testing.T) {
	var b bytes.Buffer
	for i := 0; b.Len() < 3584; i++ {
		fmt.Fprintf(&b, "VAR%d = value%d\ntarget%d: dep%d\n\t@echo target%d\n\n", i, i, i, i, i)
	}
	source := b.Bytes()
	at := bytes.Index(source, []byte("target0:")) + len("target")
	source = append(append(append([]byte(nil), source[:at]...), '('), source[at:]...)
	for _, compact := range []bool{false, true} {
		parser := gotreesitter.NewParser(MakeLanguage())
		parser.SetAdmissionCandidateRoute(compact)
		parser.SetTimeoutMicros(2_000_000)
		tree, err := parser.Parse(source)
		if err != nil {
			t.Fatal(err)
		}
		if tree.ParseStoppedEarly() || tree.RootNode() == nil || !tree.RootNode().HasError() {
			t.Fatalf("compact=%v: incomplete broken Make parse: %s", compact, tree.ParseRuntime().Summary())
		}
		tree.Release()
	}
}

func TestIssue454HTTPCommentRunCompletes(t *testing.T) {
	for _, size := range []int{2, 4, 8, 16, 32} {
		var b bytes.Buffer
		for i := 0; b.Len() < size<<10; i++ {
			fmt.Fprintf(&b, "# note %d\n", i)
		}
		source := b.Bytes()
		for _, compact := range []bool{false, true} {
			parser := gotreesitter.NewParser(HttpLanguage())
			parser.SetAdmissionCandidateRoute(compact)
			parser.SetTimeoutMicros(2_000_000)
			tree, err := parser.Parse(source)
			if err != nil {
				t.Fatal(err)
			}
			root := tree.RootNode()
			if tree.ParseStoppedEarly() || root == nil || root.HasError() || root.EndByte() != uint32(len(source)) {
				t.Fatalf("size=%d compact=%v: incomplete HTTP parse: %s", size, compact, tree.ParseRuntime().Summary())
			}
			lines := bytes.Count(source, []byte{'\n'})
			sections := (lines + 7) / 8
			if root.ChildCount() != sections || tree.ParseRuntime().NodesAllocated != lines+sections+1 {
				t.Fatalf("size=%d compact=%v: sections=%d nodes=%d, want %d and %d", size, compact,
					root.ChildCount(), tree.ParseRuntime().NodesAllocated, sections, lines+sections+1)
			}
			tree.Release()
		}
	}
	parser := gotreesitter.NewParser(HttpLanguage())
	parser.SetMemoryBudgetBytes(-1)
	source := bytes.Repeat([]byte("# note x\n"), 4096)
	tree, err := parser.Parse(source)
	if err != nil {
		t.Fatal(err)
	}
	if tree.ParseStoppedEarly() || tree.RootNode().HasError() || tree.ParseRuntime().MemoryBudgetBytes != 0 {
		t.Fatalf("disabled budget changed the comment parse: %s", tree.ParseRuntime().Summary())
	}
	tree.Release()
}

func TestIssue454DartPoorReuseFallsBack(t *testing.T) {
	var b bytes.Buffer
	b.WriteString("class C {\n")
	for i := 0; b.Len() < 137<<10; i++ {
		fmt.Fprintf(&b, "  int f%d(int a, int b) {\n    var x%d = a + b;\n    return x%d;\n  }\n", i, i, i)
	}
	b.WriteString("}\n")
	source := b.Bytes()
	at := bytes.Index(source, []byte("x0"))
	edited := append(append(append([]byte(nil), source[:at]...), 'x'), source[at:]...)
	row := uint32(bytes.Count(source[:at], []byte{'\n'}))
	column := uint32(at - bytes.LastIndexByte(source[:at], '\n') - 1)
	point := gotreesitter.Point{Row: row, Column: column}
	edit := gotreesitter.InputEdit{
		StartByte: uint32(at), OldEndByte: uint32(at), NewEndByte: uint32(at + 1),
		StartPoint: point, OldEndPoint: point,
		NewEndPoint: gotreesitter.Point{Row: row, Column: column + 1},
	}
	for _, compact := range []bool{false, true} {
		parser := gotreesitter.NewParser(DartLanguage())
		parser.SetAdmissionCandidateRoute(compact)
		old, err := parser.Parse(source)
		if err != nil {
			t.Fatal(err)
		}
		old.Edit(edit)
		incremental, profile, err := parser.ParseIncrementalProfiled(edited, old)
		if err != nil {
			t.Fatal(err)
		}
		fresh, err := parser.Parse(edited)
		if err != nil {
			t.Fatal(err)
		}
		if !profile.ReuseUnsupported || profile.ReuseUnsupportedReason != "incremental_parse_reuse_budget_full_retry" {
			t.Fatalf("compact=%v: poor reuse did not fall back: %+v", compact, profile)
		}
		incDigest, err := benchfixtures.InspectGoTree(incremental.RootNode(), DartLanguage())
		if err != nil {
			t.Fatal(err)
		}
		freshDigest, err := benchfixtures.InspectGoTree(fresh.RootNode(), DartLanguage())
		if err != nil {
			t.Fatal(err)
		}
		if incDigest.SHA256 != freshDigest.SHA256 {
			t.Fatalf("compact=%v: incremental tree differs from fresh", compact)
		}
		plainOld, err := parser.Parse(source)
		if err != nil {
			t.Fatal(err)
		}
		plainOld.Edit(edit)
		plain, err := parser.ParseIncremental(edited, plainOld)
		if err != nil {
			t.Fatal(err)
		}
		plainDigest, err := benchfixtures.InspectGoTree(plain.RootNode(), DartLanguage())
		if err != nil {
			t.Fatal(err)
		}
		if plainDigest.SHA256 != freshDigest.SHA256 {
			t.Fatalf("compact=%v: plain incremental tree differs from fresh", compact)
		}
		old.Release()
		incremental.Release()
		fresh.Release()
		plainOld.Release()
		plain.Release()
	}
}
