package bench_test

import (
	"os"
	"testing"

	gotreesitter "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
)

func loadCMakeCorpusFile(tb testing.TB, name string) []byte {
	tb.Helper()
	src, err := os.ReadFile(name)
	if err != nil {
		tb.Fatalf("read %s: %v", name, err)
	}
	return src
}

func BenchmarkCMakeForestParse(b *testing.B) {
	src := loadCMakeCorpusFile(b, "../../cgo_harness/corpus_real/cmake/medium__CMakeLists.txt")
	lang := grammars.CmakeLanguage()
	parser := gotreesitter.NewParser(lang)
	gotreesitter.SetGLRForestEnabled(true)
	b.ReportAllocs()
	b.SetBytes(int64(len(src)))

	for i := 0; i < b.N; i++ {
		tree, err := parser.Parse(src)
		if err != nil {
			b.Fatalf("Parse: %v", err)
		}
		if tree.RootNode() == nil || tree.RootNode().HasError() {
			b.Fatalf("bad CMake forest parse")
		}
		tree.Release()
	}
}
