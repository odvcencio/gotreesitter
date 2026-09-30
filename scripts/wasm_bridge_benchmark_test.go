package wasmprobe

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"testing"
)

// BenchmarkWASMBridge64KiB launches one fresh Node host per sample. Its edit,
// open and no-edit metrics time the complete JavaScript bridge calls. ns/op
// includes host startup and validation; B/op and allocs/op cover this Go driver.
func BenchmarkWASMBridge64KiB(b *testing.B) {
	artifact := os.Getenv("GTS_WASM_BENCH_ARTIFACT")
	bootstrap := os.Getenv("GTS_WASM_BENCH_BOOTSTRAP")
	blob := os.Getenv("GTS_WASM_BENCH_BLOB")
	script := os.Getenv("GTS_WASM_BENCH_SCRIPT")
	if artifact == "" {
		artifact = "runtime.wasm"
	}
	if bootstrap == "" || blob == "" || script == "" {
		b.Fatal("set GTS_WASM_BENCH_BOOTSTRAP, GTS_WASM_BENCH_BLOB and GTS_WASM_BENCH_SCRIPT")
	}
	var updates, opens, noEdits, peakRSS float64
	for i := 0; i < b.N; i++ {
		cmd := exec.Command("node", script, artifact, bootstrap, blob)
		cmd.Env = append(os.Environ(), "GTS_WASM_PROGRAM_AUDIT=1", "GTS_WASM_HOST=node", "GTS_WASM_CASE=functions-64k")
		out, err := cmd.CombinedOutput()
		if err != nil {
			b.Fatalf("WASM operation failed: %v %s", err, out)
		}
		saw := false
		for _, line := range bytes.Split(out, []byte{'\n'}) {
			var sample struct {
				Kind      string  `json:"kind"`
				Update    float64 `json:"update_ms"`
				Open      float64 `json:"open_ms"`
				Unchanged float64 `json:"unchanged_ms"`
				RSS       float64 `json:"process_rss"`
			}
			if json.Unmarshal(line, &sample) == nil && sample.Kind == "document" {
				saw = true
				updates += sample.Update
				opens += sample.Open
				noEdits += sample.Unchanged
				if sample.RSS > peakRSS {
					peakRSS = sample.RSS
				}
			}
		}
		if !saw {
			b.Fatal("WASM operation had no document sample")
		}
	}
	b.ReportMetric(updates*1e6/float64(b.N), "edit-ns/op")
	b.ReportMetric(opens*1e6/float64(b.N), "open-ns/op")
	b.ReportMetric(noEdits*1e6/float64(b.N), "noedit-ns/op")
	b.ReportMetric(peakRSS, "peak-RSS-B")
}
