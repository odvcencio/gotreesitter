//go:build cgo && treesitter_c_parity

package cgoharness

import (
	"os"
	"testing"

	gts "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
	sitter "github.com/tree-sitter/go-tree-sitter"
)

// BenchmarkSharedRecoveryFile measures the complete public parse operation,
// including retries, result publication, and tree release. Supply one grammar
// and fixture with GTS_SHARED_RECOVERY_LANGUAGE and GTS_SHARED_RECOVERY_FILE.
// Correctness is checked separately so the same benchmark can measure a
// baseline whose recovery tree differs from C. Go/C are paired Go-C-C-Go.
func BenchmarkSharedRecoveryFile(b *testing.B) {
	name, path := os.Getenv("GTS_SHARED_RECOVERY_LANGUAGE"), os.Getenv("GTS_SHARED_RECOVERY_FILE")
	if name == "" || path == "" {
		b.Skip("set GTS_SHARED_RECOVERY_LANGUAGE and GTS_SHARED_RECOVERY_FILE")
	}
	source, err := os.ReadFile(path)
	if err != nil {
		b.Fatal(err)
	}
	entry := grammars.DetectLanguageByName(name)
	if entry == nil || entry.Language == nil {
		b.Fatalf("grammar %q unavailable", name)
	}
	language := entry.Language()
	cLanguage, err := ParityCLanguage(name)
	if err != nil {
		b.Fatal(err)
	}
	for _, runtime := range []string{"Go", "C", "C", "Go"} {
		b.Run(runtime, func(b *testing.B) {
			b.ReportAllocs()
			b.SetBytes(int64(len(source)))
			if runtime == "C" {
				parser := sitter.NewParser()
				defer parser.Close()
				if err := parser.SetLanguage(cLanguage); err != nil {
					b.Fatal(err)
				}
				b.ResetTimer()
				for i := 0; i < b.N; i++ {
					tree := parser.Parse(source, nil)
					if tree == nil {
						b.Fatal("C returned no tree")
					}
					if tree.RootNode().EndByte() < uint(len(source)) {
						tree.Close()
						b.Fatal("C root does not cover input")
					}
					tree.Close()
				}
				return
			}
			parser := gts.NewParser(language)
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				var tree *gts.Tree
				var err error
				if entry.TokenSourceFactory == nil {
					tree, err = parser.Parse(source)
				} else {
					tree, err = parser.ParseWithTokenSourceFactory(source, func(input []byte) (gts.TokenSource, error) {
						return entry.TokenSourceFactory(input, language), nil
					})
				}
				if err != nil || tree == nil {
					b.Fatalf("Go parse: %v", err)
				}
				if tree.ParseStopReason() != gts.ParseStopAccepted || tree.RootNode().EndByte() < uint32(len(source)) {
					stop, end := tree.ParseStopReason(), tree.RootNode().EndByte()
					tree.Release()
					b.Fatalf("Go incomplete operation: stop=%s end=%d bytes=%d", stop, end, len(source))
				}
				tree.Release()
			}
		})
	}
}
