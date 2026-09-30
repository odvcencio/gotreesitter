//go:build cgo && treesitter_c_parity && treesitter_c_perfscan

package cgoharness

import (
	"strings"
	"testing"
	"time"

	gts "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
)

func BenchmarkJavaScriptMalformedQuoteNativeLockedC(b *testing.B) {
	benchmarkJavaScriptMalformedQuoteNativeLockedC(b, 40)
}

func BenchmarkJavaScriptMalformedQuoteNativeLargeLockedC(b *testing.B) {
	benchmarkJavaScriptMalformedQuoteNativeLockedC(b, 16000)
}

// The C slots use the existing publication oracle's native parse-only samples.
// Process setup and native tree deletion do not enter those samples. Go's
// Parse plus Release is timed, so the Go/C ratio includes Go's release cost.
// C-slot B/op and allocs/op describe the Go sidecar transport, not C's heap.
func benchmarkJavaScriptMalformedQuoteNativeLockedC(b *testing.B, functions int) {
	source := issue1335Source(functions)
	oracle, err := buildStaticCPerfOracle("javascript")
	if err != nil {
		b.Fatal(err)
	}
	defer oracle.Close()
	for _, mode := range []string{"go_first", "c_first", "c_second", "go_second"} {
		b.Run(mode, func(b *testing.B) {
			b.ReportAllocs()
			b.SetBytes(int64(len(source)))
			if strings.HasPrefix(mode, "go_") {
				parser := gts.NewParser(grammars.JavascriptLanguage())
				parser.SetAdmissionCandidateRoute(false)
				warm, err := parser.Parse(source)
				if err != nil {
					b.Fatal(err)
				}
				warm.Release()
				b.ResetTimer()
				for i := 0; i < b.N; i++ {
					tree, err := parser.Parse(source)
					if err != nil {
						b.Fatal(err)
					}
					tree.Release()
				}
				return
			}
			b.ResetTimer()
			measurement := oracle.measure(source, perfScanAxisFull, 1, b.N, 5*time.Second)
			b.StopTimer()
			if measurement.Status != perfScanStatusOK || len(measurement.Samples) != b.N {
				b.Fatalf("native C measurement: status=%s detail=%s samples=%d want=%d", measurement.Status, measurement.Detail, len(measurement.Samples), b.N)
			}
			var total int64
			for _, ns := range measurement.Samples {
				total += ns
			}
			// Report the C timer, rather than process launch and transport.
			b.ReportMetric(float64(total)/float64(b.N), "ns/op")
		})
	}
}
