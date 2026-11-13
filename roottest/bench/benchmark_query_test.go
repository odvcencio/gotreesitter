package bench_test

import (
	"testing"

	"github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
)

// BenchmarkQueryExec measures query compilation + execution on a 500-function Go file.
func BenchmarkQueryExec(b *testing.B) {
	entry := grammars.DetectLanguage("main.go")
	if entry == nil {
		b.Skip("Go grammar not available")
	}

	lang := entry.Language()
	src := makeGoBenchmarkSource(benchmarkFuncCount(b))
	ts := mustGoTokenSource(b, src, lang)

	parser := gotreesitter.NewParser(lang)
	tree, err := parser.ParseWithTokenSource(src, ts)
	if err != nil {
		b.Fatalf("parse failed: %v", err)
	}
	if tree.RootNode() == nil {
		b.Fatal("parse returned nil root")
	}
	defer tree.Release()

	highlightQuery := entry.HighlightQuery

	b.ReportAllocs()
	b.SetBytes(int64(len(src)))
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		q, err := gotreesitter.NewQuery(highlightQuery, lang)
		if err != nil {
			b.Fatalf("NewQuery failed: %v", err)
		}
		matches := q.Execute(tree)
		if len(matches) == 0 {
			b.Fatal("query returned no matches")
		}
	}
}

// BenchmarkQueryExecCompiled measures execution of a pre-compiled query,
// amortizing the compilation cost.
func BenchmarkQueryExecCompiled(b *testing.B) {
	entry := grammars.DetectLanguage("main.go")
	if entry == nil {
		b.Skip("Go grammar not available")
	}

	lang := entry.Language()
	src := makeGoBenchmarkSource(benchmarkFuncCount(b))
	ts := mustGoTokenSource(b, src, lang)

	parser := gotreesitter.NewParser(lang)
	tree, err := parser.ParseWithTokenSource(src, ts)
	if err != nil {
		b.Fatalf("parse failed: %v", err)
	}
	if tree.RootNode() == nil {
		b.Fatal("parse returned nil root")
	}
	defer tree.Release()

	q, err := gotreesitter.NewQuery(entry.HighlightQuery, lang)
	if err != nil {
		b.Fatalf("NewQuery failed: %v", err)
	}

	b.ReportAllocs()
	b.SetBytes(int64(len(src)))
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		matches := q.Execute(tree)
		if len(matches) == 0 {
			b.Fatal("query returned no matches")
		}
	}
}
