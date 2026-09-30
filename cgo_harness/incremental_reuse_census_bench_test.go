//go:build cgo && treesitter_c_parity

package cgoharness

import (
	"crypto/sha256"
	"flag"
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	gts "github.com/odvcencio/gotreesitter"
	sitter "github.com/tree-sitter/go-tree-sitter"
)

// BenchmarkIncrementalReuseCensus gates default-off overhead on the middle
// fixed edit site of each size/class. The complete three-site matrix remains
// in TestIncrementalReuseCensus. Each timed operation is a Go-C-C-Go cycle;
// go-ns/edit, c-ns/edit, and go/c expose per-edit measurements explicitly.
// The randomized benchmark wrapper supplies one explicit shuffle seed per
// process; this benchmark uses the same seed to shuffle its subcases too.
func BenchmarkIncrementalReuseCensus(b *testing.B) {
	fixtures := reuseCensusCases(b)
	root := os.Getenv("GTS_INCR_CENSUS_ROOT")
	if root == "" {
		b.Fatal("census root is required")
	}
	entry := parityEntriesByName[fixtures[0].Language]
	lang := entry.Language()
	cLang, err := COracleLanguage(entry.Name)
	if err != nil {
		b.Fatal(err)
	}
	type scenario struct {
		name           string
		source, edited []byte
		edit           gts.InputEdit
	}
	var cases []scenario
	for _, fixture := range fixtures {
		source, err := os.ReadFile(filepath.Join(root, fixture.Language, fixture.Path))
		if err != nil {
			b.Fatal(err)
		}
		if len(source) != fixture.Bytes || fmt.Sprintf("%x", sha256.Sum256(source)) != fixture.SHA256 {
			b.Fatal("fixture drift")
		}
		for _, kind := range []string{"one_byte", "100_byte", "splice"} {
			edited, edit := reuseCensusEdit(source, kind, 1, nil)
			cases = append(cases, scenario{fmt.Sprintf("%d/%s", fixture.TargetBytes, kind), source, edited, edit})
		}
	}
	seed := int64(1)
	if f := flag.Lookup("test.shuffle"); f != nil {
		if parsed, err := strconv.ParseInt(f.Value.String(), 10, 64); err == nil {
			seed = parsed
		}
	}
	rand.New(rand.NewSource(seed)).Shuffle(len(cases), func(i, j int) { cases[i], cases[j] = cases[j], cases[i] })
	for _, tc := range cases {
		b.Run(tc.name, func(b *testing.B) {
			p := gts.NewParser(lang)
			cp := sitter.NewParser()
			defer cp.Close()
			if err := cp.SetLanguage(cLang); err != nil {
				b.Fatal(err)
			}
			cedit := realCorpusCInputEdit(tc.edit)
			var goNanos, cNanos int64
			var tokens, nodes uint64
			b.ReportAllocs()
			b.SetBytes(int64(len(tc.edited)))
			b.ResetTimer()
			b.StopTimer()
			for i := 0; i < b.N; i++ {
				oldGo1, err := reuseCensusParse(p, entry, tc.source, nil)
				if err != nil {
					b.Fatal(err)
				}
				oldC1 := cp.Parse(tc.source, nil)
				oldC2 := cp.Parse(tc.source, nil)
				if oldC1 == nil || oldC2 == nil {
					b.Fatal("nil C old tree")
				}
				oldGo2, err := reuseCensusParse(p, entry, tc.source, nil)
				if err != nil {
					b.Fatal(err)
				}
				b.StartTimer()
				started := time.Now()
				oldGo1.Edit(tc.edit)
				go1, err1 := reuseCensusParse(p, entry, tc.edited, oldGo1)
				goNanos += time.Since(started).Nanoseconds()
				started = time.Now()
				oldC1.Edit(&cedit)
				c1 := cp.Parse(tc.edited, oldC1)
				cNanos += time.Since(started).Nanoseconds()
				started = time.Now()
				oldC2.Edit(&cedit)
				c2 := cp.Parse(tc.edited, oldC2)
				cNanos += time.Since(started).Nanoseconds()
				started = time.Now()
				oldGo2.Edit(tc.edit)
				go2, err2 := reuseCensusParse(p, entry, tc.edited, oldGo2)
				goNanos += time.Since(started).Nanoseconds()
				b.StopTimer()
				if err1 != nil || err2 != nil || go1 == nil || go2 == nil || c1 == nil || c2 == nil {
					b.Fatalf("parse failure: %v %v", err1, err2)
				}
				for _, tree := range []*gts.Tree{go1, go2} {
					rt := tree.ParseRuntime()
					tokens += rt.TokensConsumed
					nodes += uint64(rt.NodesAllocated)
					if rt.StopReason != gts.ParseStopAccepted || tree.RootNode().EndByte() != uint32(len(tc.edited)) {
						b.Fatalf("incomplete edit: %s", rt.StopReason)
					}
				}
				go1.Release()
				go2.Release()
				oldGo1.Release()
				oldGo2.Release()
				c1.Close()
				c2.Close()
				oldC1.Close()
				oldC2.Close()
			}
			b.ReportMetric(float64(goNanos)/float64(2*b.N), "go-ns/edit")
			b.ReportMetric(float64(cNanos)/float64(2*b.N), "c-ns/edit")
			b.ReportMetric(float64(goNanos)/float64(cNanos), "go/c")
			b.ReportMetric(float64(tokens)/float64(2*b.N), "go-tokens/edit")
			b.ReportMetric(float64(nodes)/float64(2*b.N), "go-nodes/edit")
		})
	}
}
