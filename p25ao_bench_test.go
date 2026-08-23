package gotreesitter_test

import (
	"os"
	"testing"

	gotreesitter "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
	"github.com/odvcencio/gotreesitter/internal/benchfixtures"
)

func benchmarkP25aoParse(b *testing.B, language *gotreesitter.Language, path string) {
	source, err := os.ReadFile(path)
	if err != nil {
		b.Fatal(err)
	}
	parser := gotreesitter.NewParser(language)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		tree, err := parser.Parse(source)
		if err != nil {
			b.Fatal(err)
		}
		tree.Release()
	}
}

func BenchmarkP25aoHighHitSwift(b *testing.B) {
	benchmarkP25aoParse(b, grammars.SwiftLanguage(), "grammars/testdata/swift_corpus/stdlib_Collection.swift")
}

func BenchmarkP25aoHighHitJavaScript(b *testing.B) {
	benchmarkP25aoParse(b, grammars.JavascriptLanguage(), "testdata/parser_result/csharp/jsontextreader_excerpt.cs")
}

func BenchmarkP25aoRecoveryDeletion(b *testing.B) {
	fixtures, err := benchfixtures.LoadGoFullParseFixtures()
	if err != nil {
		b.Fatal(err)
	}
	var source []byte
	for _, fixture := range fixtures {
		if fixture.Fixture.ID == "rewrite" {
			source = fixture.Source
			break
		}
	}
	if len(source) == 0 || len(source) <= 2077 {
		b.Fatal("rewrite fixture is unavailable")
	}
	edited := append([]byte(nil), source[:2076]...)
	edited = append(edited, source[2077:]...)
	point := gotreesitter.Point{}
	for _, c := range source[:2076] {
		if c == '\n' {
			point.Row++
			point.Column = 0
		} else {
			point.Column++
		}
	}
	edit := gotreesitter.InputEdit{StartByte: 2076, OldEndByte: 2077, NewEndByte: 2076, StartPoint: point, OldEndPoint: gotreesitter.Point{Row: point.Row, Column: point.Column + 1}, NewEndPoint: point}
	parser := gotreesitter.NewParser(grammars.GoLanguage())
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		oldTree, err := parser.Parse(source)
		if err != nil {
			b.Fatal(err)
		}
		oldTree.Edit(edit)
		newTree, err := parser.ParseIncremental(edited, oldTree)
		if err != nil {
			b.Fatal(err)
		}
		newTree.Release()
	}
}
