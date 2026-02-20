package gotreesitter_test

import (
	"bytes"
	"fmt"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/odvcencio/mane/gotreesitter"
	"github.com/odvcencio/mane/grammars"
)

// Run C baseline comparisons with:
//   go test ./gotreesitter -tags treesitter_c_bench -bench 'Benchmark(Go|CTreeSitter)Parse' -benchmem

func makeGoBenchmarkSource(funcCount int) []byte {
	var sb strings.Builder
	sb.Grow(funcCount * 48)
	sb.WriteString("package main\n\n")
	for i := 0; i < funcCount; i++ {
		fmt.Fprintf(&sb, "func f%d() int { v := %d; return v }\n", i, i)
	}
	return []byte(sb.String())
}

func pointAtOffset(src []byte, offset int) gotreesitter.Point {
	var row uint32
	var col uint32
	for i := 0; i < offset && i < len(src); {
		r, size := utf8.DecodeRune(src[i:])
		if r == '\n' {
			row++
			col = 0
		} else {
			col++
		}
		i += size
	}
	return gotreesitter.Point{Row: row, Column: col}
}

func benchmarkFuncCount(b *testing.B) int {
	if testing.Short() {
		return 100
	}
	return 500
}

func BenchmarkGoParseFull(b *testing.B) {
	lang := grammars.GoLanguage()
	parser := gotreesitter.NewParser(lang)
	src := makeGoBenchmarkSource(benchmarkFuncCount(b))

	b.ReportAllocs()
	b.SetBytes(int64(len(src)))
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		ts := grammars.NewGoTokenSource(src, lang)
		tree := parser.ParseWithTokenSource(src, ts)
		if tree.RootNode() == nil {
			b.Fatal("parse returned nil root")
		}
	}
}

func BenchmarkGoParseIncrementalSingleByteEdit(b *testing.B) {
	lang := grammars.GoLanguage()
	parser := gotreesitter.NewParser(lang)
	src := makeGoBenchmarkSource(benchmarkFuncCount(b))

	editAt := bytes.Index(src, []byte("v := 0"))
	if editAt < 0 {
		b.Fatal("could not find edit marker")
	}
	editAt += len("v := ")
	start := pointAtOffset(src, editAt)
	end := pointAtOffset(src, editAt+1)

	tree := parser.ParseWithTokenSource(src, grammars.NewGoTokenSource(src, lang))
	if tree.RootNode() == nil {
		b.Fatal("initial parse returned nil root")
	}

	edit := gotreesitter.InputEdit{
		StartByte:   uint32(editAt),
		OldEndByte:  uint32(editAt + 1),
		NewEndByte:  uint32(editAt + 1),
		StartPoint:  start,
		OldEndPoint: end,
		NewEndPoint: end,
	}

	b.ReportAllocs()
	b.SetBytes(int64(len(src)))
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		// Toggle one ASCII digit in place so byte/point ranges stay stable.
		if src[editAt] == '0' {
			src[editAt] = '1'
		} else {
			src[editAt] = '0'
		}

		tree.Edit(edit)
		ts := grammars.NewGoTokenSource(src, lang)
		tree = parser.ParseIncrementalWithTokenSource(src, tree, ts)
		if tree.RootNode() == nil {
			b.Fatal("incremental parse returned nil root")
		}
	}
}

func BenchmarkGoParseIncrementalNoEdit(b *testing.B) {
	lang := grammars.GoLanguage()
	parser := gotreesitter.NewParser(lang)
	src := makeGoBenchmarkSource(benchmarkFuncCount(b))

	tree := parser.ParseWithTokenSource(src, grammars.NewGoTokenSource(src, lang))
	if tree.RootNode() == nil {
		b.Fatal("initial parse returned nil root")
	}

	b.ReportAllocs()
	b.SetBytes(int64(len(src)))
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		ts := grammars.NewGoTokenSource(src, lang)
		tree = parser.ParseIncrementalWithTokenSource(src, tree, ts)
		if tree.RootNode() == nil {
			b.Fatal("incremental parse returned nil root")
		}
	}
}
