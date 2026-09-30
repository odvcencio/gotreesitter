package gotreesitter_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// BenchmarkLockedCTreeNavigationComplete feeds the same generated sources to
// the standalone locked-runtime driver. Reported allocation metrics are native
// requested bytes/calls, including realloc requests, rather than cgo wrapper
// allocations. Driver startup and source loading are outside its timed region.
func BenchmarkLockedCTreeNavigationComplete(b *testing.B) {
	driver := os.Getenv("GTS_TREE_NAV_C_DRIVER")
	if driver == "" {
		b.Skip("set GTS_TREE_NAV_C_DRIVER and GTS_TREE_NAV_C_GO/C_SHARP to locked artifacts")
	}
	for _, name := range []string{"go", "c_sharp"} {
		b.Run(name, func(b *testing.B) {
			_, source, _ := treeViewWitness(b, name)
			path := filepath.Join(b.TempDir(), "source.txt")
			if err := os.WriteFile(path, source, 0600); err != nil {
				b.Fatal(err)
			}
			grammar := os.Getenv("GTS_TREE_NAV_C_" + strings.ToUpper(name))
			if grammar == "" {
				b.Fatalf("missing locked grammar artifact for %s", name)
			}
			for _, mode := range []string{"Full", "Edit"} {
				b.Run(mode, func(b *testing.B) {
					b.ResetTimer()
					var fields []string
					for i := 0; i < b.N; i++ {
						output, err := exec.Command(driver, grammar, name, "tree_sitter_"+name, path, mode, "750").CombinedOutput()
						if err != nil {
							b.Fatalf("C complete operation: %v\n%s", err, output)
						}
						fields = strings.Fields(string(output))
					}
					b.StopTimer()
					if len(fields) != 10 || fields[0] != "BenchmarkLockedCNavigation/"+name+"/"+mode {
						b.Fatalf("invalid C benchmark row: %v", fields)
					}
					for i := 2; i < len(fields); i += 2 {
						value, err := strconv.ParseFloat(fields[i], 64)
						if err != nil || value < 0 {
							b.Fatalf("invalid C metric: %v", fields)
						}
						b.ReportMetric(value, fields[i+1])
					}
				})
			}
		})
	}
}
