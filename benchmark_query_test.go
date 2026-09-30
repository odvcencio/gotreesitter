package gotreesitter_test

import (
	"fmt"
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

// BenchmarkQueryQuantifiedWitness measures the complete public query operation
// on the overseer's failed-suffix witness. Parsing and compilation are setup.
func BenchmarkQueryQuantifiedWitness(b *testing.B) {
	for _, comments := range []int{16, 32, 64} {
		b.Run(fmt.Sprintf("comments=%d", comments), func(b *testing.B) {
			lang := grammars.GoLanguage()
			source := []byte("package audit\n" + strings.Repeat("// audit\n", comments) + "func F() {}\n")
			parser := gotreesitter.NewParser(lang)
			tree, err := parser.Parse(source)
			if err != nil || tree == nil || tree.RootNode().HasError() {
				b.Fatalf("parse witness: %v", err)
			}
			defer tree.Release()
			q, err := gotreesitter.NewQuery(`(source_file (comment)+ @comment . (type_declaration) @type)`, lang)
			if err != nil {
				b.Fatal(err)
			}
			operations := []struct {
				name string
				run  func() int
			}{
				{"Execute", func() int { return len(q.Execute(tree)) }},
				{"ExecuteInto", func() int { return len(q.ExecuteInto(tree, nil)) }},
				{"Cursor", func() int {
					cursor := q.Exec(tree.RootNode(), lang, source)
					count := 0
					for {
						_, ok := cursor.NextMatch()
						if !ok {
							return count
						}
						count++
					}
				}},
			}
			for _, operation := range operations {
				b.Run(operation.name, func(b *testing.B) {
					b.ReportAllocs()
					b.SetBytes(int64(len(source)))
					for i := 0; i < b.N; i++ {
						if count := operation.run(); count != 0 {
							b.Fatalf("matches=%d, want 0", count)
						}
					}
				})
			}
		})
	}
}

// BenchmarkQueryQuantifiedSuccess catches cost shifted into successful runs.
// It includes materializing and returning every captured comment.
func BenchmarkQueryQuantifiedSuccess(b *testing.B) {
	benchmarkQueryQuantifiedSuccess(b, `(source_file (comment)+ @comment)`)
}

func BenchmarkQueryRootQuantifiedSuccess(b *testing.B) {
	benchmarkQueryQuantifiedSuccess(b, `(comment)+ @comment`)
}

func benchmarkQueryQuantifiedSuccess(b *testing.B, query string) {
	for _, comments := range []int{32, 256, 4096} {
		b.Run(fmt.Sprintf("comments=%d", comments), func(b *testing.B) {
			lang := grammars.GoLanguage()
			source := []byte("package audit\n" + strings.Repeat("// audit\n", comments) + "func F() {}\n")
			parser := gotreesitter.NewParser(lang)
			tree, err := parser.Parse(source)
			if err != nil || tree == nil || tree.RootNode().HasError() {
				b.Fatalf("parse success fixture: %v", err)
			}
			defer tree.Release()
			q, err := gotreesitter.NewQuery(query, lang)
			if err != nil {
				b.Fatal(err)
			}
			b.ReportAllocs()
			b.SetBytes(int64(len(source)))
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				matches := q.ExecuteInto(tree, nil)
				if len(matches) != 1 || len(matches[0].Captures) != comments {
					b.Fatal("successful run lost captures")
				}
			}
		})
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
