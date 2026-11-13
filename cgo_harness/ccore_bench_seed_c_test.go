//go:build cgo && treesitter_c_parity

package cgoharness

// Seeded workload order, paired Go-C-C-Go cycles and per-iteration latency for
// the core's benchmarks.
//
// scripts/run_randomized_benchmarks.sh sets GTS_BENCHMARK_SEED once per process
// and runs a baseline and a head with the same seed. Go's own shuffle flag only
// reorders top-level benchmarks, so the child workloads of BenchmarkCcore and
// BenchmarkCcoreBigEdit run through internal/benchorder, which orders them by
// that seed: the order differs between seeds and is the same on every revision
// for one seed. Without the variable the seed is 0.

import (
	"os"
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/odvcencio/gotreesitter/internal/benchorder"
	"github.com/odvcencio/gotreesitter/internal/ccore/bind/bindtest"
	sitter "github.com/tree-sitter/go-tree-sitter"
)

func ccoreBenchSeed(tb testing.TB) int64 {
	s := os.Getenv(benchorder.SeedEnv)
	if s == "" {
		return 0
	}
	seed, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		tb.Fatalf("%s must contain an integer: %v", benchorder.SeedEnv, err)
	}
	return seed
}

// ccoreReleaseLast releases the tree of the last parse on revisions whose core
// has a Release (the store); on the others the call does nothing.
func ccoreReleaseLast(sp *bindtest.Parser) {
	if r, ok := any(sp).(interface{ ReleaseLast() }); ok {
		r.ReleaseLast()
	}
}

func ccoreRelease(sp *bindtest.Parser, t bindtest.Tree) {
	if r, ok := any(sp).(interface{ Release(bindtest.Tree) }); ok {
		r.Release(t)
	}
}

// ccorePairedFresh measures fresh parses of src in Go-C-C-Go cycles: every
// iteration parses with the core, twice with C, and with the core again, each
// parse timed alone. It reports the mean time per parse of each side and their
// ratio.
func ccorePairedFresh(b *testing.B, sp *bindtest.Parser, cp *sitter.Parser, src []byte) {
	goParse := func() int64 {
		t0 := time.Now()
		sp.Parse(src, nil)
		d := time.Since(t0).Nanoseconds()
		ccoreReleaseLast(sp)
		return d
	}
	cParse := func() int64 {
		t0 := time.Now()
		t := cp.Parse(src, nil)
		d := time.Since(t0).Nanoseconds()
		t.Close()
		return d
	}
	var goNs, cNs int64
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		goNs += goParse()
		cNs += cParse()
		cNs += cParse()
		goNs += goParse()
	}
	b.ReportMetric(float64(goNs)/float64(2*b.N), "go-ns/op")
	b.ReportMetric(float64(cNs)/float64(2*b.N), "c-ns/op")
	b.ReportMetric(float64(goNs)/float64(cNs), "go-over-c")
}

func ccoreP99(v []int64) float64 {
	if len(v) == 0 {
		return 0
	}
	s := append([]int64(nil), v...)
	sort.Slice(s, func(i, j int) bool { return s[i] < s[j] })
	return float64(s[min(len(s)-1, (len(s)*99+99)/100-1)])
}

// BenchmarkCcoreBigEditCycle times one-byte edit plus reparse in the 1 MiB Go
// file in Go-C-C-Go cycles and reports the per-iteration latency distribution:
// the mean and the 99th percentile of the edit and reparse, and of the same with
// the release of the old trees counted (the core's release; C frees its trees
// inside Close, which is timed with it). Workloads run in seeded order.
func BenchmarkCcoreBigEditCycle(b *testing.B) {
	gl, cl := ccoreLanguages(b, "go")
	src, names, srcs, edits := ccoreBigEdits(b)
	var cases []benchorder.Case
	for i := range edits {
		i := i
		cases = append(cases, benchorder.Case{Label: strings.ReplaceAll(names[i], "/", "_"), Run: func(b *testing.B) {
			core := ccoreNew(gl)
			core.Parse(src, nil)
			base := core.Tree()
			cp := sitter.NewParser()
			defer cp.Close()
			if err := cp.SetLanguage(cl); err != nil {
				b.Fatal(err)
			}
			cbase := cp.Parse(src, nil)
			defer cbase.Close()
			goCycle := func() (edit, rel int64) {
				t0 := time.Now()
				edited := core.Edit(base, edits[i])
				nt, _ := core.ParseIncremental(srcs[i], edited, nil)
				edit = time.Since(t0).Nanoseconds()
				r0 := time.Now()
				ccoreRelease(core, nt)
				ccoreRelease(core, edited)
				rel = time.Since(r0).Nanoseconds()
				return
			}
			cCycle := func() (edit, rel int64) {
				ce := cbase.Clone()
				t0 := time.Now()
				ce.Edit(ccoreSitterEdit(&edits[i]))
				nt := cp.Parse(srcs[i], ce)
				edit = time.Since(t0).Nanoseconds()
				r0 := time.Now()
				nt.Close()
				ce.Close()
				rel = time.Since(r0).Nanoseconds()
				return
			}
			var goEdit, goRel, cEdit, cRel []int64
			add := func(e, r *[]int64, f func() (int64, int64)) {
				a, c := f()
				*e = append(*e, a)
				*r = append(*r, a+c)
			}
			b.ResetTimer()
			for n := 0; n < b.N; n++ {
				add(&goEdit, &goRel, goCycle)
				add(&cEdit, &cRel, cCycle)
				add(&cEdit, &cRel, cCycle)
				add(&goEdit, &goRel, goCycle)
			}
			b.StopTimer()
			mean := func(v []int64) float64 {
				var s int64
				for _, x := range v {
					s += x
				}
				return float64(s) / float64(len(v))
			}
			b.ReportMetric(mean(goEdit), "go-ns/op")
			b.ReportMetric(ccoreP99(goEdit), "go-p99-ns")
			b.ReportMetric(mean(goRel), "go-with-release-ns/op")
			b.ReportMetric(ccoreP99(goRel), "go-with-release-p99-ns")
			b.ReportMetric(mean(cRel), "c-ns/op")
			b.ReportMetric(ccoreP99(cRel), "c-p99-ns")
			b.ReportMetric(mean(goEdit)/mean(cRel), "go-over-c")
		}})
	}
	benchorder.Run(b, cases, ccoreBenchSeed(b))
}
