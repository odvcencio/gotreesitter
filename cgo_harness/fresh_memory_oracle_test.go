//go:build cgo && treesitter_c_parity && treesitter_c_perfscan

package cgoharness

import (
	"bufio"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
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
	sitter "github.com/tree-sitter/go-tree-sitter"
)

type freshMemoryNativeSample struct {
	NS           uint64 `json:"ns"`
	Allocs       uint64 `json:"allocs"`
	Bytes        uint64 `json:"requested_bytes"`
	PeakLive     uint64 `json:"peak_live_bytes"`
	BaselineLive uint64 `json:"baseline_live_bytes"`
}

func freshMemoryNativeRun(artifact, source, axis string, reps int) ([]freshMemoryNativeSample, error) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(reps+2)*30*time.Second)
	defer cancel()
	output, err := exec.CommandContext(ctx, artifact, "measure", axis, source, "1", strconv.Itoa(reps), "30000000").CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("native measurement: %w: %s", err, output)
	}
	var samples []freshMemoryNativeSample
	schema, gotAxis, status := false, false, false
	scanner := bufio.NewScanner(bytes.NewReader(output))
	for scanner.Scan() {
		key, value, ok := strings.Cut(scanner.Text(), "=")
		if !ok {
			return nil, fmt.Errorf("invalid protocol line %q", scanner.Text())
		}
		switch key {
		case "schema":
			if schema || value != staticCPerfSchema {
				return nil, fmt.Errorf("invalid schema %q", value)
			}
			schema = true
		case "axis":
			if gotAxis || value != axis {
				return nil, fmt.Errorf("invalid axis %q", value)
			}
			gotAxis = true
		case "status":
			if status || value != "ok" {
				return nil, fmt.Errorf("invalid status %q", value)
			}
			status = true
		case "sample_ns", "sample_allocs", "sample_bytes", "sample_peak_live", "sample_baseline_live":
			n, err := strconv.ParseUint(value, 10, 64)
			if err != nil {
				return nil, err
			}
			if key == "sample_ns" {
				if n == 0 {
					return nil, fmt.Errorf("zero native duration")
				}
				samples = append(samples, freshMemoryNativeSample{NS: n})
				continue
			}
			if len(samples) == 0 {
				return nil, fmt.Errorf("memory field before sample")
			}
			sample := &samples[len(samples)-1]
			switch key {
			case "sample_allocs":
				sample.Allocs = n
			case "sample_bytes":
				sample.Bytes = n
			case "sample_peak_live":
				sample.PeakLive = n
			case "sample_baseline_live":
				sample.BaselineLive = n
			}
		default:
			return nil, fmt.Errorf("unknown native field %q", key)
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	if !schema || !gotAxis || !status || len(samples) != reps {
		return nil, fmt.Errorf("incomplete native response: %s", output)
	}
	if axis == "memory" {
		for _, sample := range samples {
			if sample.Allocs == 0 || sample.Bytes == 0 || sample.PeakLive <= sample.BaselineLive {
				return nil, fmt.Errorf("missing native memory metrics: %+v", sample)
			}
		}
	}
	return samples, nil
}

// TestFreshMemoryLockedC authenticates the exact memory workloads before any
// timing. Native CPU/RSS runs use the ordinary allocator. A separate allocator
// instrumentation run measures C allocations, requested bytes, and live bytes.
// GTS_FRESH_MEMORY_OUTPUT may name an external evidence directory.
func TestFreshMemoryLockedC(t *testing.T) {
	name := os.Getenv("GTS_FRESH_MEMORY_LANG")
	if name == "" {
		t.Skip("set GTS_FRESH_MEMORY_LANG to one language")
	}
	entry := grammars.DetectLanguageByName(name)
	if entry == nil {
		t.Fatalf("unknown language %q", name)
	}
	driver, err := filepath.Abs(filepath.Join("pure_c", "memory_oracle.c"))
	if err != nil {
		t.Fatal(err)
	}
	oracle, err := buildStaticCPerfOracleWithDriver(name, driver)
	if err != nil {
		t.Fatal(err)
	}
	defer oracle.Close()
	cLanguage, err := COracleLanguage(name)
	if err != nil {
		t.Fatal(err)
	}
	cParser := sitter.NewParser()
	defer cParser.Close()
	if err := cParser.SetLanguage(cLanguage); err != nil {
		t.Fatal(err)
	}
	lang := entry.Language()
	parser := gts.NewParser(lang)
	out := os.Getenv("GTS_FRESH_MEMORY_OUTPUT")
	if out == "" {
		out = t.TempDir()
	}
	if err := os.MkdirAll(out, 0755); err != nil {
		t.Fatal(err)
	}
	// Preserve the authenticated executable for standalone /usr/bin/time runs.
	artifact, err := os.ReadFile(oracle.artifact)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(out, name+"-memory-oracle"), artifact, 0755); err != nil {
		t.Fatal(err)
	}
	for _, kib := range []int{137, 1024} {
		t.Run(fmt.Sprintf("%dKiB", kib), func(t *testing.T) {
			source, _, err := benchfixtures.GeneratedSource(name, kib*1024)
			if err != nil {
				t.Fatal(err)
			}
			path := filepath.Join(out, fmt.Sprintf("%s-%d.source", name, kib))
			if err := os.WriteFile(path, source, 0444); err != nil {
				t.Fatal(err)
			}
			cTree := cParser.Parse(source, nil)
			if cTree == nil {
				t.Fatal("locked C returned no tree")
			}
			defer cTree.Close()
			cDigest, err := COracleDeepDigest(cTree)
			if err != nil {
				t.Fatal(err)
			}
			nativeDigest, _, err := oracle.deepDigest(source, 30*time.Second)
			if err != nil {
				t.Fatal(err)
			}
			if nativeDigest != cDigest {
				t.Fatalf("native=%s binding=%s", nativeDigest, cDigest)
			}
			var rt gts.ParseRuntime
			for pass := 0; pass < 3; pass++ {
				tree, err := parser.Parse(source)
				if err != nil {
					t.Fatal(err)
				}
				inspection, err := benchfixtures.InspectGoTree(tree.RootNode(), lang)
				if err != nil {
					tree.Release()
					t.Fatal(err)
				}
				rt = tree.ParseRuntime()
				if tree.ParseStopReason() != gts.ParseStopAccepted || tree.RootNode().HasError() || tree.RootNode().EndByte() != uint32(len(source)) || inspection.SHA256 != cDigest {
					tree.Release()
					t.Fatalf("pass %d differs from C: digest=%s want=%s runtime=%s", pass, inspection.SHA256, cDigest, rt.Summary())
				}
				tree.Release()
			}
			memory, err := freshMemoryNativeRun(oracle.artifact, path, "memory", 3)
			if err != nil {
				t.Fatal(err)
			}
			cpu, err := freshMemoryNativeRun(oracle.artifact, path, "full_operation", 20)
			if err != nil {
				t.Fatal(err)
			}
			sum := sha256.Sum256(source)
			// Keep receipts portable: do not publish checkout or artifact paths.
			receipt := struct {
				Language      string                    `json:"language"`
				InputBytes    int                       `json:"input_bytes"`
				SourceSHA     string                    `json:"source_sha256"`
				TreeSHA       string                    `json:"tree_sha256"`
				RuntimeCommit string                    `json:"runtime_commit"`
				GrammarCommit string                    `json:"grammar_commit"`
				ArtifactSHA   string                    `json:"artifact_sha256"`
				DriverSHA     string                    `json:"driver_sha256"`
				GoRuntime     gts.ParseRuntime          `json:"go_runtime"`
				Memory        []freshMemoryNativeSample `json:"native_memory"`
				CPU           []freshMemoryNativeSample `json:"native_cpu"`
			}{name, len(source), hex.EncodeToString(sum[:]), cDigest, COracleRuntimeCommit, oracle.identity.Language.GrammarCommit, oracle.identity.Language.ArtifactSHA256, oracle.identity.Common.DriverSHA256, rt, memory, cpu}
			data, err := json.MarshalIndent(receipt, "", "  ")
			if err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(out, fmt.Sprintf("%s-%d.json", name, kib)), append(data, '\n'), 0644); err != nil {
				t.Fatal(err)
			}
			t.Logf("bytes=%d tree=%s runtime=%s C=%+v", len(source), cDigest, rt.Summary(), memory[0])
		})
	}
}
