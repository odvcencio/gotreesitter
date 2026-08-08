package grammars_test

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"strings"
	"testing"

	gotreesitter "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
)

func TestIssue454CSharpFreshAndIncrementalConverge(t *testing.T) {
	lang := grammars.CSharpLanguage()
	source := issue454CSharpSource(137 * 1024)
	parser := gotreesitter.NewParser(lang)

	want := issue454CSharpParseShape(t, parser, lang, source)
	if want.hasError {
		t.Fatal("base fresh parse has an error")
	}
	for i := 0; i < 2; i++ {
		got := issue454CSharpParseShape(t, parser, lang, source)
		if got != want {
			t.Fatalf("fresh parse %d shape changed: nodes=%d, want %d", i+2, got.nodes, want.nodes)
		}
	}

	site := bytes.Index(source, []byte("x0"))
	if site < 0 {
		t.Fatal("C# edit marker is absent")
	}

	tests := []struct {
		name    string
		edited  []byte
		oldEnd  int
		newEnd  int
		oldCols uint32
		newCols uint32
	}{
		{
			name:    "replace",
			edited:  append([]byte(nil), source...),
			oldEnd:  site + 1,
			newEnd:  site + 1,
			oldCols: 1,
			newCols: 1,
		},
		{
			name:    "insert",
			edited:  append(append(append([]byte(nil), source[:site]...), 'x'), source[site:]...),
			oldEnd:  site,
			newEnd:  site + 1,
			oldCols: 0,
			newCols: 1,
		},
		{
			name:    "delete",
			edited:  append(append([]byte(nil), source[:site]...), source[site+1:]...),
			oldEnd:  site + 1,
			newEnd:  site,
			oldCols: 1,
			newCols: 0,
		},
	}
	tests[0].edited[site] = 'y'

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			oldTree, err := parser.Parse(source)
			if err != nil {
				t.Fatalf("old Parse: %v", err)
			}
			defer oldTree.Release()
			point := issue454PointAt(source, site)
			oldTree.Edit(gotreesitter.InputEdit{
				StartByte:   uint32(site),
				OldEndByte:  uint32(test.oldEnd),
				NewEndByte:  uint32(test.newEnd),
				StartPoint:  point,
				OldEndPoint: gotreesitter.Point{Row: point.Row, Column: point.Column + test.oldCols},
				NewEndPoint: gotreesitter.Point{Row: point.Row, Column: point.Column + test.newCols},
			})

			incremental, profile, err := gotreesitter.NewParser(lang).ParseIncrementalProfiled(test.edited, oldTree)
			if err != nil {
				t.Fatalf("ParseIncrementalProfiled: %v", err)
			}
			defer incremental.Release()
			if !profile.ReuseUnsupported || profile.ReuseUnsupportedReason != "external_scanner_unsupported" {
				t.Fatalf("incremental fallback status = %+v", profile)
			}
			fresh, err := gotreesitter.NewParser(lang).Parse(test.edited)
			if err != nil {
				t.Fatalf("fresh edited Parse: %v", err)
			}
			defer fresh.Release()

			got := issue454CSharpTreeShape(t, lang, incremental)
			want := issue454CSharpTreeShape(t, lang, fresh)
			if got != want {
				t.Fatalf(
					"incremental nodes=%d error=%t, fresh nodes=%d error=%t",
					got.nodes,
					got.hasError,
					want.nodes,
					want.hasError,
				)
			}
			if test.name == "delete" && !got.hasError {
				t.Fatal("delete parse did not retain the localized parse error")
			}
		})
	}
}

func BenchmarkIssue454CSharpRecoveredFullParse(b *testing.B) {
	lang := grammars.CSharpLanguage()
	source := issue454CSharpSource(137 * 1024)
	site := bytes.Index(source, []byte("x0"))
	if site < 0 {
		b.Fatal("C# edit marker is absent")
	}
	source = append(append([]byte(nil), source[:site]...), source[site+1:]...)
	parser := gotreesitter.NewParser(lang)
	parser.SetAdmissionCandidateRoute(false)
	b.ReportAllocs()
	b.SetBytes(int64(len(source)))
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		tree, err := parser.Parse(source)
		if err != nil {
			b.Fatal(err)
		}
		tree.Release()
	}
}

func issue454CSharpSource(targetBytes int) []byte {
	// This internal regression shape is not the reporter's byte-identical source.
	var source strings.Builder
	source.Grow(targetBytes + 256)
	source.WriteString("namespace Bench {\n")
	for i := 0; source.Len() < targetBytes; i++ {
		fmt.Fprintf(
			&source,
			"public static class C%d { public static int F%d() { var x%d = %d; return x%d; } }\n",
			i,
			i,
			i,
			i,
			i,
		)
	}
	source.WriteString("}\n")
	return []byte(source.String())
}

func issue454CSharpParseShape(
	t *testing.T,
	parser *gotreesitter.Parser,
	lang *gotreesitter.Language,
	source []byte,
) issue454CSharpShape {
	t.Helper()
	tree, err := parser.Parse(source)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	defer tree.Release()
	return issue454CSharpTreeShape(t, lang, tree)
}

type issue454CSharpShape struct {
	digest   [32]byte
	nodes    int
	rootType string
	hasError bool
	endByte  uint32
}

func issue454CSharpTreeShape(
	t *testing.T,
	lang *gotreesitter.Language,
	tree *gotreesitter.Tree,
) issue454CSharpShape {
	t.Helper()
	if tree == nil || tree.RootNode() == nil {
		t.Fatal("parse returned no root")
	}
	root := tree.RootNode()
	if got, want := root.EndByte(), uint32(len(tree.Source())); got != want {
		t.Fatalf("root end = %d, want %d", got, want)
	}
	return issue454CSharpShape{
		digest:   sha256.Sum256([]byte(root.SExpr(lang))),
		nodes:    issue454CSharpNodeCount(root),
		rootType: root.Type(lang),
		hasError: root.HasError(),
		endByte:  root.EndByte(),
	}
}

func issue454CSharpNodeCount(node *gotreesitter.Node) int {
	if node == nil {
		return 0
	}
	count := 1
	for i := 0; i < int(node.ChildCount()); i++ {
		count += issue454CSharpNodeCount(node.Child(i))
	}
	return count
}

func issue454PointAt(source []byte, offset int) gotreesitter.Point {
	var point gotreesitter.Point
	for _, b := range source[:offset] {
		if b == '\n' {
			point.Row++
			point.Column = 0
			continue
		}
		point.Column++
	}
	return point
}
