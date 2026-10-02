//go:build cgo && treesitter_c_parity && treesitter_c_perfscan

package cgoharness

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/odvcencio/gotreesitter/internal/benchfixtures"
	sitter "github.com/tree-sitter/go-tree-sitter"
)

const freshGeneratedStaticCAxis = "full_operation"
const freshGeneratedStaticCTimingRegion = "ts_parser_parse_string_plus_validation_and_tree_delete;process,source,parser_setup_excluded"

func TestStaticCPerfOracleCompleteOperation(t *testing.T) {
	oracle, err := buildStaticCPerfOracle("go")
	if err != nil {
		t.Fatal(err)
	}
	defer oracle.Close()
	source := []byte("package main\nfunc main() {}\n")
	for _, axis := range []string{perfScanAxisFull, freshGeneratedStaticCAxis} {
		measured := oracle.measure(source, axis, 1, 3, 10*time.Second)
		if measured.Status != perfScanStatusOK || len(measured.Samples) != 3 {
			t.Fatalf("%s: %+v", axis, measured)
		}
		for _, sample := range measured.Samples {
			if sample <= 0 {
				t.Fatalf("%s: nonpositive sample %d", axis, sample)
			}
		}
	}
}

// BenchmarkFreshGeneratedStaticC uses the authenticated, statically linked C
// oracle. The native clock includes validation and tree deletion; it excludes
// process launch, source loading, and parser setup. Go allocation metrics here
// describe the measurement transport, not allocations inside the C runtime.
func BenchmarkFreshGeneratedStaticC(b *testing.B) {
	for _, name := range []string{"go", "javascript", "typescript", "python", "rust", "java", "c", "cpp"} {
		b.Run(name, func(b *testing.B) {
			oracle, err := buildStaticCPerfOracle(name)
			if err != nil {
				b.Fatal(err)
			}
			defer oracle.Close()
			// Go recalibrates each leaf benchmark. Authenticate each immutable
			// fixture once per process rather than repeating its deep C walk
			// for every calibration; none of this setup enters the native clock.
			sources := make(map[int][]byte)
			for _, size := range benchfixtures.FreshSizesForSeed(flag.Lookup("test.shuffle").Value.String()) {
				b.Run(fmt.Sprintf("%dKiB", size>>10), func(b *testing.B) {
					source := sources[size]
					if source == nil {
						var err error
						source, _, err = benchfixtures.GeneratedSource(name, size)
						if err != nil {
							b.Fatal(err)
						}
						digest, sourceSHA, err := oracle.deepDigest(source, 10*time.Second)
						if err != nil {
							b.Fatal(err)
						}
						language, err := ParityCLanguage(name)
						if err != nil {
							b.Fatal(err)
						}
						parser := sitter.NewParser()
						defer parser.Close()
						if err := parser.SetLanguage(language); err != nil {
							b.Fatal(err)
						}
						tree := parser.Parse(source, nil)
						if tree == nil {
							b.Fatal("C parity transport returned no tree")
						}
						parityDigest := canonicalCTreeDigest(b, tree, name)
						tree.Close()
						if digest != parityDigest {
							b.Fatalf("static digest=%s parity digest=%s", digest, parityDigest)
						}
						if dir := os.Getenv("GTS_FRESH_STATIC_C_IDENTITY_DIR"); dir != "" {
							identity := oracle.identity
							identity.Common.TimingRegion = freshGeneratedStaticCTimingRegion
							data, err := json.MarshalIndent(struct {
								Identity  perfScanOracleIdentity `json:"identity"`
								SourceSHA string                 `json:"source_sha256"`
								DeepSHA   string                 `json:"deep_sha256"`
							}{identity, sourceSHA, digest}, "", "  ")
							if err != nil {
								b.Fatal(err)
							}
							if err := os.MkdirAll(dir, 0755); err != nil {
								b.Fatal(err)
							}
							if err := os.WriteFile(filepath.Join(dir, fmt.Sprintf("%s-%d.json", name, size>>10)), append(data, '\n'), 0644); err != nil {
								b.Fatal(err)
							}
						}
						sources[size] = source
					}
					b.SetBytes(int64(len(source)))
					b.ReportAllocs()
					b.ResetTimer()
					measured := oracle.measure(source, freshGeneratedStaticCAxis, 1, b.N, 10*time.Second)
					b.StopTimer()
					if measured.Status != perfScanStatusOK || len(measured.Samples) != b.N {
						b.Fatalf("static C operation: %+v", measured)
					}
					var total int64
					for _, ns := range measured.Samples {
						total += ns
					}
					b.ReportMetric(float64(total)/float64(len(measured.Samples)), "ns/op")
					b.ReportMetric(float64(len(source))*float64(len(measured.Samples))*1000/float64(total), "MB/s")
				})
			}
		})
	}
}
