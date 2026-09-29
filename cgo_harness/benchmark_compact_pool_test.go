//go:build cgo && treesitter_c_parity && treesitter_c_bench

package cgoharness

import (
	"os"
	"testing"

	gts "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
	sitter "github.com/tree-sitter/go-tree-sitter"
)

func BenchmarkCompactPoolGoFull(b *testing.B)      { benchmarkCompactPoolGo(b, true, false) }
func BenchmarkCompactPoolGoFresh(b *testing.B)     { benchmarkCompactPoolGo(b, true, true) }
func BenchmarkCompactPoolLegacyFull(b *testing.B)  { benchmarkCompactPoolGo(b, false, false) }
func BenchmarkCompactPoolLegacyFresh(b *testing.B) { benchmarkCompactPoolGo(b, false, true) }

func benchmarkCompactPoolGo(b *testing.B, compact, fresh bool) {
	benchmarkCompactPoolGoSource(b, grammars.GoLanguage(), makeGoBenchmarkSource(benchmarkFuncCount(b)), compact, fresh)
}

func benchmarkCompactPoolGoSource(b *testing.B, lang *gts.Language, source []byte, compact, fresh bool) {
	parser := gts.NewParser(lang)
	parser.SetAdmissionCandidateRoute(compact)
	warm, err := parser.Parse(source)
	if err != nil || warm == nil {
		b.Fatalf("warmup parse: %v", err)
	}
	warm.Release()
	b.ReportAllocs()
	b.SetBytes(int64(len(source)))
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if fresh {
			parser = gts.NewParser(lang)
			parser.SetAdmissionCandidateRoute(compact)
		}
		tree, err := parser.Parse(source)
		if err != nil || tree == nil {
			b.Fatalf("parse: %v", err)
		}
		if tree.RootNode().EndByte() != uint32(len(source)) {
			b.Fatal("truncated parse")
		}
		tree.Release()
	}
}

func compactPoolCliffFixture(b *testing.B) cliffFixture {
	name := os.Getenv("GTS_CLIFF_LANGUAGE")
	if name == "" {
		b.Skip("set GTS_CLIFF_LANGUAGE to one cliff fixture")
	}
	for _, fixture := range cliffFixtures {
		if fixture.Name == name {
			return fixture
		}
	}
	b.Fatalf("unknown cliff fixture %q", name)
	return cliffFixture{}
}

func benchmarkCompactPoolCliffGo(b *testing.B, compact, fresh bool) {
	fixture := compactPoolCliffFixture(b)
	entry := grammars.DetectLanguageByName(fixture.Grammar)
	if entry == nil || entry.Language() == nil {
		b.Fatal("grammar unavailable")
	}
	benchmarkCompactPoolGoSource(b, entry.Language(), loadCliffSource(b, fixture), compact, fresh)
}

func BenchmarkCompactPoolCliffGoFull(b *testing.B)      { benchmarkCompactPoolCliffGo(b, true, false) }
func BenchmarkCompactPoolCliffGoFresh(b *testing.B)     { benchmarkCompactPoolCliffGo(b, true, true) }
func BenchmarkCompactPoolCliffLegacyFull(b *testing.B)  { benchmarkCompactPoolCliffGo(b, false, false) }
func BenchmarkCompactPoolCliffLegacyFresh(b *testing.B) { benchmarkCompactPoolCliffGo(b, false, true) }

func benchmarkCompactPoolCliffC(b *testing.B, fresh bool) {
	fixture := compactPoolCliffFixture(b)
	source := loadCliffSource(b, fixture)
	lang, err := COracleLanguage(fixture.Grammar)
	if err != nil {
		b.Fatal(err)
	}
	parser := sitter.NewParser()
	if err := parser.SetLanguage(lang); err != nil {
		b.Fatal(err)
	}
	warm := parser.Parse(source, nil)
	if warm == nil {
		b.Fatal("C warmup returned no tree")
	}
	warm.Close()
	b.ReportAllocs()
	b.SetBytes(int64(len(source)))
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if fresh {
			parser.Close()
			parser = sitter.NewParser()
			if err := parser.SetLanguage(lang); err != nil {
				b.Fatal(err)
			}
		}
		tree := parser.Parse(source, nil)
		if tree == nil || tree.RootNode().EndByte() != uint(len(source)) {
			b.Fatal("C parse truncated")
		}
		tree.Close()
	}
	b.StopTimer()
	parser.Close()
}

func BenchmarkCompactPoolCliffCFull(b *testing.B)  { benchmarkCompactPoolCliffC(b, false) }
func BenchmarkCompactPoolCliffCFresh(b *testing.B) { benchmarkCompactPoolCliffC(b, true) }
