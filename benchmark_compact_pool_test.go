package gotreesitter_test

import (
	"os"
	"testing"

	gts "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
	"github.com/odvcencio/gotreesitter/internal/benchfixtures"
)

func BenchmarkGoParsePoolLargeFull(b *testing.B)  { benchmarkCompactPoolLarge(b, false) }
func BenchmarkGoParsePoolLargeFresh(b *testing.B) { benchmarkCompactPoolLarge(b, true) }

func benchmarkCompactPoolLarge(b *testing.B, fresh bool) {
	path := os.Getenv("GTS_COMPACT_POOL_SOURCE")
	if path == "" {
		b.Skip("set GTS_COMPACT_POOL_SOURCE to a generated Go fixture")
	}
	source, err := os.ReadFile(path)
	if err != nil {
		b.Fatal(err)
	}
	lang := grammars.GoLanguage()
	parser := gts.NewParser(lang)
	warm, err := parser.Parse(source)
	if err != nil {
		b.Fatal(err)
	}
	requireCompleteParse(b, warm, source, lang, "large warmup")
	warm.Release()
	b.ReportAllocs()
	b.SetBytes(int64(len(source)))
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if fresh {
			parser = gts.NewParser(lang)
		}
		tree, err := parser.Parse(source)
		if err != nil {
			b.Fatal(err)
		}
		requireCompleteParse(b, tree, source, lang, "large parse")
		tree.Release()
	}
}

// BenchmarkGoParseFreshParserDFA measures the file-at-a-time API: immutable
// language tables are warm, but each file gets a new Parser. This is distinct
// from BenchmarkGoParseFullDFA, which reuses one Parser throughout.
func BenchmarkGoParseFreshParserDFA(b *testing.B) {
	benchmarkCompactPoolFreshParser(b, makeGoBenchmarkSource(benchmarkFuncCount(b)))
}

func BenchmarkGoParseFreshParserCanonical(b *testing.B) {
	fixtures, err := benchfixtures.LoadGoFullParseFixtures()
	if err != nil {
		b.Fatal(err)
	}
	for _, fixture := range fixtures {
		b.Run(fixture.Fixture.ID, func(b *testing.B) {
			benchmarkCompactPoolFreshParser(b, fixture.Source)
		})
	}
}

func benchmarkCompactPoolFreshParser(b *testing.B, source []byte) {
	lang := grammars.GoLanguage()
	warm, err := gts.NewParser(lang).Parse(source)
	if err != nil {
		b.Fatal(err)
	}
	requireCompleteParse(b, warm, source, lang, "fresh parser warmup")
	warm.Release()
	b.ReportAllocs()
	b.SetBytes(int64(len(source)))
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		parser := gts.NewParser(lang)
		tree, err := parser.Parse(source)
		if err != nil {
			b.Fatal(err)
		}
		requireCompleteParse(b, tree, source, lang, "fresh parser")
		tree.Release()
	}
}
