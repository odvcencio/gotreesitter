package bench_test

import (
	"strconv"
	"strings"
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

// largeArrayQueryFixture returns the query (array (_) @x) and a JSON array of
// 10,000 numbers: one root node with 10,000 matches of one pattern, so the
// per-root cost of putting matches in the C cursor's order shows up.
func largeArrayQueryFixture(b *testing.B) (*gotreesitter.Query, *gotreesitter.Tree, *gotreesitter.Language) {
	b.Helper()
	entry := grammars.DetectLanguageByName("json")
	if entry == nil {
		b.Skip("JSON grammar not available")
	}
	lang := entry.Language()
	var sb strings.Builder
	sb.WriteByte('[')
	for i := 0; i < 10000; i++ {
		if i > 0 {
			sb.WriteByte(',')
		}
		sb.WriteString(strconv.Itoa(i))
	}
	sb.WriteByte(']')
	tree, err := gotreesitter.NewParser(lang).Parse([]byte(sb.String()))
	if err != nil {
		b.Fatalf("parse failed: %v", err)
	}
	q, err := gotreesitter.NewQuery(`(array (_) @x)`, lang)
	if err != nil {
		b.Fatalf("NewQuery failed: %v", err)
	}
	return q, tree, lang
}

// BenchmarkQueryExecLargeArray measures Execute on one root with 10,000
// matches of one pattern.
func BenchmarkQueryExecLargeArray(b *testing.B) {
	q, tree, _ := largeArrayQueryFixture(b)
	defer tree.Release()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if n := len(q.Execute(tree)); n != 10000 {
			b.Fatalf("matches: got %d, want 10000", n)
		}
	}
}

// BenchmarkQueryNextMatchLargeArray measures a drained QueryCursor on one
// root with 10,000 matches of one pattern.
func BenchmarkQueryNextMatchLargeArray(b *testing.B) {
	q, tree, lang := largeArrayQueryFixture(b)
	defer tree.Release()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		cursor := q.Exec(tree.RootNode(), lang, tree.Source())
		n := 0
		for {
			if _, ok := cursor.NextMatch(); !ok {
				break
			}
			n++
		}
		if n != 10000 {
			b.Fatalf("matches: got %d, want 10000", n)
		}
	}
}
