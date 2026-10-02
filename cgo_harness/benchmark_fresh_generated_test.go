//go:build cgo && treesitter_c_parity

package cgoharness

import (
	"flag"
	"fmt"
	"testing"

	gotreesitter "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
	"github.com/odvcencio/gotreesitter/internal/benchfixtures"
	sitter "github.com/tree-sitter/go-tree-sitter"
)

// TestFreshGeneratedLockedC checks the same whole-operation workloads that
// BenchmarkFreshGenerated profiles in the root module, against the locked C
// runtime and grammar. Select one language per Docker invocation with -run.
func TestFreshGeneratedLockedC(t *testing.T) {
	testFreshGeneratedLockedC(t, true)
}

func TestFreshGeneratedDefaultLockedC(t *testing.T) {
	testFreshGeneratedLockedC(t, false)
}

func testFreshGeneratedLockedC(t *testing.T, forceLegacy bool) {
	for _, name := range []string{"go", "javascript", "typescript", "python", "rust", "java", "c", "cpp"} {
		t.Run(name, func(t *testing.T) {
			for _, size := range []int{32 << 10, 137 << 10, 1 << 20} {
				t.Run(fmt.Sprintf("%dKiB", size>>10), func(t *testing.T) {
					source, _, err := benchfixtures.GeneratedSource(name, size)
					if err != nil {
						t.Fatal(err)
					}
					lang := grammars.DetectLanguageByName(name).Language()
					parser := gotreesitter.NewParser(lang)
					if forceLegacy {
						parser.SetAdmissionCandidateRoute(false)
					}
					tree, err := parser.Parse(source)
					if err != nil {
						t.Fatal(err)
					}
					defer tree.Release()
					cLang, err := ParityCLanguage(name)
					if err != nil {
						t.Fatal(err)
					}
					cParser := sitter.NewParser()
					defer cParser.Close()
					if err := cParser.SetLanguage(cLang); err != nil {
						t.Fatal(err)
					}
					cTree := cParser.Parse(source, nil)
					if cTree == nil {
						t.Fatal("C parse returned no tree")
					}
					defer cTree.Close()
					if tree.ParseStopReason() != gotreesitter.ParseStopAccepted || tree.RootNode().EndByte() != uint32(len(source)) || tree.RootNode().HasError() != cTree.RootNode().HasError() {
						t.Fatalf("incomplete or differing error state: %s", tree.ParseRuntime().Summary())
					}
					if diff := FirstDivergenceDumpV1(tree.RootNode(), lang, cTree.RootNode()); diff != nil {
						t.Fatalf("fresh tree differs from locked C: %+v", *diff)
					}
					inspection, err := benchfixtures.InspectGoTree(tree.RootNode(), lang)
					if err != nil {
						t.Fatal(err)
					}
					digest := canonicalCTreeDigest(t, cTree, name)
					if inspection.SHA256 != digest {
						t.Fatalf("deep digest=%s, locked C=%s", inspection.SHA256, digest)
					}
					t.Logf("bytes=%d digest=%s runtime=%s", len(source), digest, tree.ParseRuntime().Summary())
					// A fresh parser's first allocation and retained arenas on later
					// operations must produce the same locked-C tree.
					for pass := 0; pass < 2; pass++ {
						warm, err := parser.Parse(source)
						if err != nil {
							t.Fatal(err)
						}
						inspection, err := benchfixtures.InspectGoTree(warm.RootNode(), lang)
						if err != nil {
							warm.Release()
							t.Fatal(err)
						}
						if warm.ParseStopReason() != gotreesitter.ParseStopAccepted || warm.RootNode().HasError() != cTree.RootNode().HasError() || inspection.SHA256 != digest {
							warm.Release()
							t.Fatalf("warm pass %d differs from locked C: digest=%s, want %s", pass+1, inspection.SHA256, digest)
						}
						warm.Release()
					}
				})
			}
		})
	}
}

// BenchmarkFreshGeneratedC measures fresh parsing plus tree release using the
// locked C runtime on exactly the generated sources used by the Go benchmark.
// This callback transport is diagnostic; use BenchmarkFreshGeneratedStaticC
// with treesitter_c_perfscan for publication native-C ratios.
func BenchmarkFreshGeneratedC(b *testing.B) {
	for _, name := range []string{"go", "javascript", "typescript", "python", "rust", "java", "c", "cpp"} {
		b.Run(name, func(b *testing.B) {
			for _, size := range benchfixtures.FreshSizesForSeed(flag.Lookup("test.shuffle").Value.String()) {
				b.Run(fmt.Sprintf("%dKiB", size>>10), func(b *testing.B) {
					source, _, err := benchfixtures.GeneratedSource(name, size)
					if err != nil {
						b.Fatal(err)
					}
					lang, err := ParityCLanguage(name)
					if err != nil {
						b.Fatal(err)
					}
					parser := sitter.NewParser()
					defer parser.Close()
					if err := parser.SetLanguage(lang); err != nil {
						b.Fatal(err)
					}
					warm := parser.Parse(source, nil)
					if warm == nil || warm.RootNode().HasError() || warm.RootNode().EndByte() != uint(len(source)) {
						b.Fatal("incomplete C warm parse")
					}
					warm.Close()
					b.SetBytes(int64(len(source)))
					b.ReportAllocs()
					b.ResetTimer()
					for i := 0; i < b.N; i++ {
						tree := parser.Parse(source, nil)
						if tree == nil {
							b.Fatal("C parse returned no tree")
						}
						tree.Close()
					}
				})
			}
		})
	}
}
