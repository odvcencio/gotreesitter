//go:build cgo && treesitter_c_bench

package gotreesitter_test

import (
	"bytes"
	"testing"

	sitter "github.com/smacker/go-tree-sitter"
	sittergo "github.com/smacker/go-tree-sitter/golang"
)

func cTreeSitterPointAtOffset(src []byte, offset int) sitter.Point {
	p := pointAtOffset(src, offset)
	return sitter.Point{Row: p.Row, Column: p.Column}
}

func newCTreeSitterParser(tb testing.TB) *sitter.Parser {
	tb.Helper()
	parser := sitter.NewParser()
	parser.SetLanguage(sittergo.GetLanguage())
	return parser
}

// BenchmarkCTreeSitterGoParseFull benchmarks the C tree-sitter Go parser as a
// baseline for the pure-Go parser benchmarks in benchmark_test.go.
func BenchmarkCTreeSitterGoParseFull(b *testing.B) {
	parser := newCTreeSitterParser(b)
	defer parser.Close()

	src := makeGoBenchmarkSource(benchmarkFuncCount(b))

	b.ReportAllocs()
	b.SetBytes(int64(len(src)))
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		tree := parser.Parse(nil, src)
		if tree == nil || tree.RootNode() == nil {
			b.Fatal("parse returned nil root")
		}
		tree.Close()
	}
}

func BenchmarkCTreeSitterGoParseIncrementalSingleByteEdit(b *testing.B) {
	parser := newCTreeSitterParser(b)
	defer parser.Close()

	src := makeGoBenchmarkSource(benchmarkFuncCount(b))

	editAt := bytes.Index(src, []byte("v := 0"))
	if editAt < 0 {
		b.Fatal("could not find edit marker")
	}
	editAt += len("v := ")
	start := cTreeSitterPointAtOffset(src, editAt)
	end := cTreeSitterPointAtOffset(src, editAt+1)

	tree := parser.Parse(nil, src)
	if tree == nil || tree.RootNode() == nil {
		b.Fatal("initial parse returned nil root")
	}
	defer tree.Close()

	edit := sitter.EditInput{
		StartIndex:  uint32(editAt),
		OldEndIndex: uint32(editAt + 1),
		NewEndIndex: uint32(editAt + 1),
		StartPoint:  start,
		OldEndPoint: end,
		NewEndPoint: end,
	}

	b.ReportAllocs()
	b.SetBytes(int64(len(src)))
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		if src[editAt] == '0' {
			src[editAt] = '1'
		} else {
			src[editAt] = '0'
		}

		tree.Edit(edit)
		newTree := parser.Parse(tree, src)
		if newTree == nil || newTree.RootNode() == nil {
			b.Fatal("incremental parse returned nil root")
		}
		tree.Close()
		tree = newTree
	}
}

func BenchmarkCTreeSitterGoParseIncrementalNoEdit(b *testing.B) {
	parser := newCTreeSitterParser(b)
	defer parser.Close()

	src := makeGoBenchmarkSource(benchmarkFuncCount(b))
	tree := parser.Parse(nil, src)
	if tree == nil || tree.RootNode() == nil {
		b.Fatal("initial parse returned nil root")
	}
	defer tree.Close()

	b.ReportAllocs()
	b.SetBytes(int64(len(src)))
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		newTree := parser.Parse(tree, src)
		if newTree == nil || newTree.RootNode() == nil {
			b.Fatal("incremental parse returned nil root")
		}
		tree.Close()
		tree = newTree
	}
}
