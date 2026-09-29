//go:build cgo && treesitter_c_parity && treesitter_c_bench && treesitter_c_perfscan

package cgoharness

import (
	"testing"
	"time"
)

func BenchmarkCompactPoolLockedCFull(b *testing.B) {
	benchmarkCompactPoolLockedC(b, "go", makeGoBenchmarkSource(benchmarkFuncCount(b)))
}

func BenchmarkCompactPoolCliffLockedCFull(b *testing.B) {
	fixture := compactPoolCliffFixture(b)
	benchmarkCompactPoolLockedC(b, fixture.Grammar, loadCliffSource(b, fixture))
}

// C-ns/op comes from the authenticated perf_scan executable's parse-only
// timing region. The ordinary ns/op and allocation columns include process
// transport and must not be used as native C performance measurements.
func benchmarkCompactPoolLockedC(b *testing.B, language string, source []byte) {
	oracle, err := buildStaticCPerfOracle(language)
	if err != nil {
		b.Fatal(err)
	}
	defer oracle.Close()
	b.ResetTimer()
	measurement := oracle.measure(source, "full", 2, b.N, 5*time.Second)
	if measurement.Status != perfScanStatusOK || len(measurement.Samples) != b.N {
		b.Fatalf("locked C measurement: %+v", measurement)
	}
	var total int64
	for _, sample := range measurement.Samples {
		total += sample
	}
	b.ReportMetric(float64(total)/float64(b.N), "C-ns/op")
}
