package gotreesitter_test

import (
	"crypto/sha256"
	"fmt"
	"testing"

	"github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
	"github.com/odvcencio/gotreesitter/internal/benchfixtures"
)

// BenchmarkGoParseFreshParserPerFileDFA measures the admission Parse path when a
// caller creates a fresh Parser for every file (a common per-request pattern).
// It guards the per-*Language table cache: the candidate route must not rebuild
// the compact action tables on each fresh Parser. Set GTS_ADMISSION_CANDIDATE=0
// to measure the production route for the same interleaved comparison.
func BenchmarkGoParseFreshParserPerFileDFA(b *testing.B) {
	lang := grammars.GoLanguage()
	src := makeGoBenchmarkSource(benchmarkFuncCount(b))

	// Warm the per-language table cache once so the steady-state per-file cost,
	// not the one-time first-build, is what the loop measures.
	warm := gotreesitter.NewParser(lang)
	if tree, err := warm.Parse(src); err != nil {
		b.Fatalf("warm parse error: %v", err)
	} else {
		tree.Release()
	}

	b.ReportAllocs()
	b.SetBytes(int64(len(src)))
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		parser := gotreesitter.NewParser(lang)
		tree, err := parser.Parse(src)
		if err != nil {
			b.Fatalf("parse error: %v", err)
		}
		requireCompleteParse(b, tree, src, lang, "fresh parser full dfa")
		tree.Release()
	}
}

// BenchmarkGoParseCompactFreshOneMiB measures the large-file allocation path
// with a new parser for each file. The shared language tables are warm; the
// record arrays are not. Its witness and work match the large locked-C gate.
func BenchmarkGoParseCompactFreshOneMiB(b *testing.B) {
	source, _, err := benchfixtures.GeneratedSource("go", 1<<20)
	if err != nil {
		b.Fatal(err)
	}
	if len(source) != 1048591 || fmt.Sprintf("%x", sha256.Sum256(source)) != "76c70b62127085d3c791ba98a24cba9f22bd343e40afb43522bbb5041cede524" {
		b.Fatal("large Go witness changed")
	}
	lang := grammars.GoLanguage()
	warm := gotreesitter.NewParser(lang)
	warm.SetAdmissionCandidateRoute(true)
	tree, err := warm.Parse([]byte("package p\n"))
	if err != nil || tree == nil {
		b.Fatalf("warm tables: %v", err)
	}
	tree.Release()
	gotreesitter.ResetAdmissionCandidateCountersForTest()
	b.ReportAllocs()
	b.SetBytes(int64(len(source)))
	b.ResetTimer()
	for range b.N {
		p := gotreesitter.NewParser(lang)
		p.SetAdmissionCandidateRoute(true)
		tree, err := p.Parse(source)
		if err != nil || tree == nil {
			b.Fatalf("large Go parse: %v", err)
		}
		rt := tree.ParseRuntime()
		if tree.RootNode().HasError() || tree.RootNode().EndByte() != uint32(len(source)) || rt.NodesAllocated != 593457 || rt.TokensConsumed != 425742 {
			b.Fatalf("large Go work or completion changed: %s", rt.Summary())
		}
		tree.Release()
	}
	b.StopTimer()
	routed, fallback := gotreesitter.AdmissionCandidateCounters()
	if routed != uint64(b.N) || fallback != 0 {
		b.Fatalf("large Go route compact=%d fallback=%d", routed, fallback)
	}
}
