package bench_test

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"math/rand"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	gts "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
	"github.com/odvcencio/gotreesitter/internal/benchfixtures"
)

// BenchmarkFreshMemory measures complete fresh parses with a warm parser and
// released trees. Run one language per process using GTS_FRESH_MEMORY_LANG.
func BenchmarkFreshMemory(b *testing.B) {
	name := os.Getenv("GTS_FRESH_MEMORY_LANG")
	if name == "" {
		b.Fatal("set GTS_FRESH_MEMORY_LANG to one language")
	}
	entry := grammars.DetectLanguageByName(name)
	if entry == nil {
		b.Fatalf("unknown language %q", name)
	}
	lang := entry.Language()
	sizes := []int{137, 1024}
	if seed, err := strconv.ParseInt(flag.Lookup("test.shuffle").Value.String(), 10, 64); err == nil {
		rand.New(rand.NewSource(seed)).Shuffle(len(sizes), func(i, j int) { sizes[i], sizes[j] = sizes[j], sizes[i] })
	}
	for _, kib := range sizes {
		b.Run(fmt.Sprintf("%s/%dKiB", name, kib), func(b *testing.B) {
			source, _, err := benchfixtures.GeneratedSource(name, kib*1024)
			if err != nil {
				b.Fatal(err)
			}
			benchmarkFreshMemoryGo(b, source, lang)
		})
	}
}

func benchmarkFreshMemoryGo(b *testing.B, source []byte, lang *gts.Language) {
	parser := gts.NewParser(lang)
	parse := func() *gts.Tree {
		tree, err := parser.Parse(source)
		if err != nil {
			b.Fatal(err)
		}
		if tree == nil || tree.RootNode() == nil {
			b.Fatal("parse returned no root")
		}
		rt := tree.ParseRuntime()
		if tree.RootNode().HasError() || tree.RootNode().EndByte() != uint32(len(source)) || tree.ParseStoppedEarly() || rt.Truncated {
			b.Fatalf("incomplete fresh parse: %s", rt.Summary())
		}
		return tree
	}
	warm := parse()
	rt := warm.ParseRuntime()
	warm.Release()
	b.ReportAllocs()
	b.SetBytes(int64(len(source)))
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		parse().Release()
	}
	b.StopTimer()
	b.ReportMetric(float64(rt.TokensConsumed), "tokens/op")
	b.ReportMetric(float64(rt.NodesAllocated), "nodes/op")
	b.ReportMetric(float64(rt.MaxStacksSeen), "stacks/op")
	b.ReportMetric(float64(rt.ArenaBytesAllocated)/float64(len(source)), "arena-B/input-B")
}

// BenchmarkFreshMemoryGoC pairs complete Go and native C operations in
// Go-C-C-Go cycles. C B/op and allocs/op describe subprocess transport;
// TestFreshMemoryLockedC supplies the native allocator measurements.
func BenchmarkFreshMemoryGoC(b *testing.B) {
	name, dir := os.Getenv("GTS_FRESH_MEMORY_LANG"), os.Getenv("GTS_FRESH_MEMORY_NATIVE_DIR")
	if name == "" || dir == "" {
		b.Fatal("set GTS_FRESH_MEMORY_LANG and GTS_FRESH_MEMORY_NATIVE_DIR")
	}
	entry := grammars.DetectLanguageByName(name)
	if entry == nil {
		b.Fatalf("unknown language %q", name)
	}
	artifact := filepath.Join(dir, name+"-memory-oracle")
	binary, err := os.ReadFile(artifact)
	if err != nil {
		b.Fatal(err)
	}
	artifactSum := sha256.Sum256(binary)
	sizes := []int{137, 1024}
	if seed, err := strconv.ParseInt(flag.Lookup("test.shuffle").Value.String(), 10, 64); err == nil {
		rand.New(rand.NewSource(seed)).Shuffle(len(sizes), func(i, j int) { sizes[i], sizes[j] = sizes[j], sizes[i] })
	}
	for _, kib := range sizes {
		b.Run(fmt.Sprintf("%s/%dKiB", name, kib), func(b *testing.B) {
			source, _, err := benchfixtures.GeneratedSource(name, kib*1024)
			if err != nil {
				b.Fatal(err)
			}
			receiptData, err := os.ReadFile(filepath.Join(dir, fmt.Sprintf("%s-%d.json", name, kib)))
			if err != nil {
				b.Fatal(err)
			}
			var receipt struct {
				SourceSHA   string `json:"source_sha256"`
				ArtifactSHA string `json:"artifact_sha256"`
			}
			if err := json.Unmarshal(receiptData, &receipt); err != nil {
				b.Fatal(err)
			}
			sourceSum := sha256.Sum256(source)
			if receipt.SourceSHA != hex.EncodeToString(sourceSum[:]) || receipt.ArtifactSHA != hex.EncodeToString(artifactSum[:]) {
				b.Fatal("native artifact or source differs from locked-C preflight")
			}
			path := filepath.Join(b.TempDir(), "source")
			if err := os.WriteFile(path, source, 0444); err != nil {
				b.Fatal(err)
			}
			b.Run("GoA", func(b *testing.B) { benchmarkFreshMemoryGo(b, source, entry.Language()) })
			b.Run("CA", func(b *testing.B) { benchmarkFreshMemoryNativeC(b, artifact, path, len(source)) })
			b.Run("CB", func(b *testing.B) { benchmarkFreshMemoryNativeC(b, artifact, path, len(source)) })
			b.Run("GoB", func(b *testing.B) { benchmarkFreshMemoryGo(b, source, entry.Language()) })
		})
	}
}

func benchmarkFreshMemoryNativeC(b *testing.B, artifact, path string, sourceBytes int) {
	b.ReportAllocs()
	b.SetBytes(int64(sourceBytes))
	b.ResetTimer()
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(b.N+2)*30*time.Second)
	defer cancel()
	output, err := exec.CommandContext(ctx, artifact, "measure", "full_operation", path, "1", strconv.Itoa(b.N), "30000000").CombinedOutput()
	b.StopTimer()
	if err != nil {
		b.Fatalf("native parse: %v: %s", err, output)
	}
	var total uint64
	count := 0
	schema, axis, status := false, false, false
	for _, line := range strings.Split(strings.TrimSpace(string(output)), "\n") {
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			b.Fatalf("invalid native protocol %q", line)
		}
		switch key {
		case "schema":
			if schema || value != "gts-static-c-perf/v2" {
				b.Fatal("invalid native schema")
			}
			schema = true
		case "axis":
			if axis || value != "full_operation" {
				b.Fatal("invalid native axis")
			}
			axis = true
		case "status":
			if status || value != "ok" {
				b.Fatalf("native status %q", value)
			}
			status = true
		case "sample_ns":
			n, err := strconv.ParseUint(value, 10, 64)
			if err != nil || n == 0 {
				b.Fatalf("invalid native sample %q", value)
			}
			total += n
			count++
		default:
			b.Fatalf("unexpected native field %q", key)
		}
	}
	if !schema || !axis || !status || count != b.N {
		b.Fatalf("incomplete native measurement: %s", output)
	}
	b.ReportMetric(float64(total)/float64(count), "ns/op")
	b.ReportMetric(float64(sourceBytes)*float64(count)*1000/float64(total), "MB/s")
}
