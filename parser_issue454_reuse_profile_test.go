package gotreesitter_test

import (
	"bytes"
	"fmt"
	"strings"
	"testing"

	gotreesitter "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
)

func issue454GoDeleteFixture(t testing.TB) ([]byte, []byte, gotreesitter.InputEdit) {
	t.Helper()
	var sourceBuilder strings.Builder
	sourceBuilder.WriteString("package main\n\nimport \"fmt\"\n\n")
	for i := 0; sourceBuilder.Len() < 2<<10; i++ {
		fmt.Fprintf(&sourceBuilder, "func f%d(a int, b int) int {\n\tx := a + b\n\tfmt.Println(\"f%d\", x)\n\treturn x\n}\n\n", i, i)
	}
	source := []byte(sourceBuilder.String())
	editAt := bytes.Index(source, []byte("x := a + b"))
	if editAt < 0 {
		t.Fatal("edit marker is absent")
	}
	edited := append(append([]byte{}, source[:editAt]...), source[editAt+1:]...)
	start := pointAtOffset(source, editAt)
	edit := gotreesitter.InputEdit{
		StartByte:   uint32(editAt),
		OldEndByte:  uint32(editAt + 1),
		NewEndByte:  uint32(editAt),
		StartPoint:  start,
		OldEndPoint: gotreesitter.Point{Row: start.Row, Column: start.Column + 1},
		NewEndPoint: start,
	}
	return source, edited, edit
}

func TestIssue454RetryReportsSelectedReuseCoverage(t *testing.T) {
	source, edited, edit := issue454GoDeleteFixture(t)
	parser := gotreesitter.NewParser(grammars.GoLanguage())
	oldTree, err := parser.Parse(source)
	if err != nil {
		t.Fatal(err)
	}
	defer oldTree.Release()
	oldTree.Edit(edit)
	incremental, profile, err := parser.ParseIncrementalProfiled(edited, oldTree)
	if err != nil {
		t.Fatal(err)
	}
	defer incremental.Release()
	fresh, err := parser.Parse(edited)
	if err != nil {
		t.Fatal(err)
	}
	defer fresh.Release()

	if incremental.RootNode().SExpr(grammars.GoLanguage()) != fresh.RootNode().SExpr(grammars.GoLanguage()) {
		t.Fatal("incremental parse differs from fresh parse")
	}
	if profile.AcceptedErrorRetryAttempts != 1 {
		t.Fatalf("accepted-error retries = %d, want 1", profile.AcceptedErrorRetryAttempts)
	}
	if profile.ReusedBytes == 0 || profile.ReusedBytes > uint64(len(edited)) {
		t.Fatalf("selected reuse coverage = %d bytes for a %d-byte source", profile.ReusedBytes, len(edited))
	}
}

// BenchmarkIssue454GoProfiledDelete includes tree copying, editing, parsing, and release.
func BenchmarkIssue454GoProfiledDelete(b *testing.B) {
	source, edited, edit := issue454GoDeleteFixture(b)
	parser := gotreesitter.NewParser(grammars.GoLanguage())
	original, err := parser.Parse(source)
	if err != nil {
		b.Fatal(err)
	}
	defer original.Release()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		old := original.Copy()
		old.Edit(edit)
		next, profile, err := parser.ParseIncrementalProfiled(edited, old)
		if err != nil || next == nil || next.ParseStoppedEarly() {
			b.Fatalf("incremental parse failed: %v", err)
		}
		if profile.AcceptedErrorRetryAttempts != 1 {
			b.Fatalf("retry attempts = %d, want 1", profile.AcceptedErrorRetryAttempts)
		}
		if next != old {
			next.Release()
		}
		old.Release()
	}
}
