package grammars_test

import (
	"compress/gzip"
	"io"
	"os"
	"testing"

	gts "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
)

func BenchmarkGoRetryBudgetCliff(b *testing.B) {
	archive, err := os.Open("../internal/benchfixtures/testdata/cliffs/go_parser.go.gz")
	if err != nil {
		b.Fatal(err)
	}
	defer archive.Close()
	reader, err := gzip.NewReader(archive)
	if err != nil {
		b.Fatal(err)
	}
	defer reader.Close()
	source, err := io.ReadAll(reader)
	if err != nil {
		b.Fatal(err)
	}
	parser := gts.NewParser(grammars.GoLanguage())
	b.SetBytes(int64(len(source)))
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
