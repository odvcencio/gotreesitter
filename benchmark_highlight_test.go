package gotreesitter_test

import (
	"bytes"
	"fmt"
	"strings"
	"testing"

	"github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
)

// BenchmarkHighlight measures highlighting a 500-function Go file from scratch.
func BenchmarkHighlight(b *testing.B) {
	entry := grammars.DetectLanguage("main.go")
	if entry == nil {
		b.Skip("Go grammar not available")
	}

	lang := entry.Language()
	src := makeGoBenchmarkSource(benchmarkFuncCount(b))

	var opts []gotreesitter.HighlighterOption
	if entry.TokenSourceFactory != nil {
		factory := entry.TokenSourceFactory
		opts = append(opts, gotreesitter.WithTokenSourceFactory(func(s []byte) gotreesitter.TokenSource {
			return factory(s, lang)
		}))
	}

	hl, err := gotreesitter.NewHighlighter(lang, entry.HighlightQuery, opts...)
	if err != nil {
		b.Fatalf("NewHighlighter failed: %v", err)
	}

	b.ReportAllocs()
	b.SetBytes(int64(len(src)))
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		ranges := hl.Highlight(src)
		if len(ranges) == 0 {
			b.Fatal("highlight returned no ranges")
		}
	}
}

// makeDiffBenchmarkSource returns a unified diff of hunkCount one-hunk files.
func makeDiffBenchmarkSource(hunkCount int) []byte {
	var sb strings.Builder
	sb.Grow(hunkCount * 160)
	for i := 0; i < hunkCount; i++ {
		fmt.Fprintf(&sb, "diff --git a/f%[1]d.go b/f%[1]d.go\n", i)
		sb.WriteString("index 83db48f..bf2a3c1 100644\n")
		fmt.Fprintf(&sb, "--- a/f%[1]d.go\n+++ b/f%[1]d.go\n", i)
		fmt.Fprintf(&sb, "@@ -1,3 +1,4 @@ func f%d()\n", i)
		sb.WriteString(" context\n-removed line\n+added line\n+another added line\n context\n")
	}
	return []byte(sb.String())
}

// BenchmarkHighlightDiff measures highlighting a unified diff from scratch. The
// diff query sets (#set! priority 95) on its punctuation, so each call resolves
// two priorities and lays one over the other. BenchmarkHighlight does not reach
// that path, because the Go query sets no priority.
func BenchmarkHighlightDiff(b *testing.B) {
	entry := grammars.DetectLanguageByName("diff")
	if entry == nil {
		b.Skip("diff grammar not available")
	}

	lang := entry.Language()
	src := makeDiffBenchmarkSource(benchmarkFuncCount(b))

	var opts []gotreesitter.HighlighterOption
	if entry.TokenSourceFactory != nil {
		factory := entry.TokenSourceFactory
		opts = append(opts, gotreesitter.WithTokenSourceFactory(func(s []byte) gotreesitter.TokenSource {
			return factory(s, lang)
		}))
	}

	hl, err := gotreesitter.NewHighlighter(lang, entry.HighlightQuery, opts...)
	if err != nil {
		b.Fatalf("NewHighlighter failed: %v", err)
	}

	b.ReportAllocs()
	b.SetBytes(int64(len(src)))
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		ranges := hl.Highlight(src)
		if len(ranges) == 0 {
			b.Fatal("highlight returned no ranges")
		}
	}
}

// BenchmarkHighlightIncremental measures re-highlighting after a single-byte edit.
func BenchmarkHighlightIncremental(b *testing.B) {
	entry := grammars.DetectLanguage("main.go")
	if entry == nil {
		b.Skip("Go grammar not available")
	}

	lang := entry.Language()
	src := makeGoBenchmarkSource(benchmarkFuncCount(b))

	// Locate the edit site: "v := 0" -> toggle the '0'.
	editAt := bytes.Index(src, []byte("v := 0"))
	if editAt < 0 {
		b.Fatal("could not find edit marker")
	}
	editAt += len("v := ")
	start := pointAtOffset(src, editAt)
	end := pointAtOffset(src, editAt+1)

	// Build the initial tree via a parser so we can hand it to HighlightIncremental.
	parser := gotreesitter.NewParser(lang)
	ts := mustGoTokenSource(b, src, lang)
	tree, err := parser.ParseWithTokenSource(src, ts)
	if err != nil {
		b.Fatalf("initial parse failed: %v", err)
	}
	if tree.RootNode() == nil {
		b.Fatal("initial parse returned nil root")
	}

	var opts []gotreesitter.HighlighterOption
	if entry.TokenSourceFactory != nil {
		factory := entry.TokenSourceFactory
		opts = append(opts, gotreesitter.WithTokenSourceFactory(func(s []byte) gotreesitter.TokenSource {
			return factory(s, lang)
		}))
	}

	hl, err := gotreesitter.NewHighlighter(lang, entry.HighlightQuery, opts...)
	if err != nil {
		b.Fatalf("NewHighlighter failed: %v", err)
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
		// Toggle one ASCII digit in place.
		if src[editAt] == '0' {
			src[editAt] = '1'
		} else {
			src[editAt] = '0'
		}

		tree.Edit(edit)
		ranges, newTree := hl.HighlightIncremental(src, tree)
		if len(ranges) == 0 {
			b.Fatal("incremental highlight returned no ranges")
		}
		if newTree != tree {
			tree.Release()
		}
		tree = newTree
	}
	tree.Release()
}
