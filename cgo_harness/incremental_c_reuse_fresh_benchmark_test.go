//go:build cgo && treesitter_c_parity

package cgoharness

import (
	"flag"
	"fmt"
	"math/rand"
	"strconv"
	"testing"

	gts "github.com/odvcencio/gotreesitter"
	sitter "github.com/tree-sitter/go-tree-sitter"
)

// Fresh parsing uses the same admitted fixtures as the edit measurements.
func BenchmarkCReuseFreshLanguages(b *testing.B) {
	seed, _ := strconv.ParseInt(flag.Lookup("test.shuffle").Value.String(), 10, 64)
	for _, name := range cReuseLanguages {
		b.Run(name, func(b *testing.B) {
			sizes := []int{32, 137, 1024}
			rand.New(rand.NewSource(seed)).Shuffle(len(sizes), func(i, j int) { sizes[i], sizes[j] = sizes[j], sizes[i] })
			for _, size := range sizes {
				b.Run(fmt.Sprintf("%dKiB", size), func(b *testing.B) {
					f := cReuseLanguageFixture(b, name, size*1024, "byte1")
					for _, engine := range []string{"Go1", "C1", "C2", "Go2"} {
						b.Run(engine, func(b *testing.B) {
							b.ReportAllocs()
							b.SetBytes(int64(len(f.source)))
							if engine[0] == 'G' {
								p := gts.NewParser(f.lang)
								p.SetAdmissionCandidateRoute(false)
								b.ResetTimer()
								for i := 0; i < b.N; i++ {
									tree, err := p.Parse(f.source)
									if err != nil {
										b.Fatal(err)
									}
									tree.Release()
								}
							} else {
								cl, err := COracleLanguage(name)
								if err != nil {
									b.Fatal(err)
								}
								p := sitter.NewParser()
								defer p.Close()
								if err := p.SetLanguage(cl); err != nil {
									b.Fatal(err)
								}
								b.ResetTimer()
								for i := 0; i < b.N; i++ {
									tree := p.Parse(f.source, nil)
									if tree == nil {
										b.Fatal("C fresh parse failed")
									}
									tree.Close()
								}
							}
						})
					}
				})
			}
		})
	}
}
