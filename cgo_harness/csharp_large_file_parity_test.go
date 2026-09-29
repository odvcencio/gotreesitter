//go:build cgo && treesitter_c_parity

package cgoharness

import (
	"fmt"
	"testing"

	gotreesitter "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
	"github.com/odvcencio/gotreesitter/internal/benchfixtures"
)

// Issue 1362: clean repetition must not keep obsolete versions until the
// memory budget stops a valid compilation unit.
func TestCSharpLargeFileCondensationParity(t *testing.T) {
	cLanguage, err := COracleLanguage("c_sharp")
	if err != nil {
		t.Fatal(err)
	}
	for _, kb := range []int{2, 64, 128, 256, 512} {
		t.Run(fmt.Sprintf("%dKiB", kb), func(t *testing.T) {
			source, _, err := benchfixtures.GeneratedSource("c_sharp", kb*1024)
			if err != nil {
				t.Fatal(err)
			}
			cTree, cDigest := issue972ParseCSharp(t, cLanguage, source)
			tree, language, elapsed := issue972ParseGoCSharp(t, source)
			runtime := tree.ParseRuntime()
			if tree.RootNode().HasError() || runtime.StopReason != gotreesitter.ParseStopAccepted || tree.RootNode().EndByte() != uint32(len(source)) {
				t.Fatalf("valid compilation unit did not finish: %s", runtime.Summary())
			}
			if runtime.MaxStacksSeen > 6 {
				t.Fatalf("clean repetition retained %d versions, want at most 6: %s", runtime.MaxStacksSeen, runtime.Summary())
			}
			if digest := issue972GoDigest(t, tree, language); digest != cDigest {
				t.Fatalf("tree differs from locked C: Go=%s C=%s first=%+v", digest, cDigest, FirstDivergenceDumpV1(tree.RootNode(), language, cTree.RootNode()))
			}
			t.Logf("bytes=%d elapsed=%s %s", len(source), elapsed, runtime.Summary())
		})
	}
}

func BenchmarkCSharpLargeFileFull(b *testing.B) {
	for _, kb := range []int{64, 128, 256, 512} {
		b.Run(fmt.Sprintf("%dKiB", kb), func(b *testing.B) {
			source, _, err := benchfixtures.GeneratedSource("c_sharp", kb*1024)
			if err != nil {
				b.Fatal(err)
			}
			parser := gotreesitter.NewParser(grammars.CSharpLanguage())
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
		})
	}
}
