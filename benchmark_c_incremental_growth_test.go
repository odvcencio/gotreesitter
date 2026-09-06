package gotreesitter_test

import (
	"bytes"
	"fmt"
	"testing"

	"github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
)

func BenchmarkCIncrementalRecoveryGrowth(b *testing.B) {
	lang := grammars.CLanguage()
	for _, count := range []int{16, 256} {
		b.Run(fmt.Sprintf("functions%d", count), func(b *testing.B) {
			var source []byte
			for i := 0; i < count; i++ {
				source = fmt.Appendf(source, "int f%d(void) { int x%d = %d; return x%d; }\n", i, i, i, i)
			}
			site := bytes.Index(source, []byte("x0"))
			edited := append(append([]byte{}, source[:site]...), source[site+1:]...)
			oldParser := gotreesitter.NewParser(lang)
			oldParser.SetAdmissionCandidateRoute(false)
			parser := gotreesitter.NewParser(lang)
			b.ReportAllocs()
			b.SetBytes(int64(len(edited)))
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				b.StopTimer()
				old, err := oldParser.Parse(source)
				if err != nil {
					b.Fatal(err)
				}
				old.Edit(gotreesitter.InputEdit{StartByte: uint32(site), OldEndByte: uint32(site + 1), NewEndByte: uint32(site), StartPoint: gotreesitter.Point{Column: uint32(site)}, OldEndPoint: gotreesitter.Point{Column: uint32(site + 1)}, NewEndPoint: gotreesitter.Point{Column: uint32(site)}})
				b.StartTimer()
				tree, err := parser.ParseIncremental(edited, old)
				if err != nil {
					b.Fatal(err)
				}
				if tree.ParseStoppedEarly() || tree.RootNode().EndByte() != uint32(len(edited)) {
					b.Fatal("incomplete incremental tree")
				}
				tree.Release()
				b.StopTimer()
				old.Release()
				b.StartTimer()
			}
		})
	}
}
