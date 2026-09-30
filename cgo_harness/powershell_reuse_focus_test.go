//go:build cgo && treesitter_c_parity

package cgoharness

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"

	gts "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
	"github.com/odvcencio/gotreesitter/internal/benchfixtures"
	sitter "github.com/tree-sitter/go-tree-sitter"
)

func powerShellFocusTypingSource(tb testing.TB) []byte {
	tb.Helper()
	root := os.Getenv("GTS_FOCUS_CORPUS_ROOT")
	if root == "" {
		var source bytes.Buffer
		for function := 0; function < 12; function++ {
			fmt.Fprintf(&source, "function value%d {\n", function)
			source.Write(bytes.Repeat([]byte("Write-Output 0\n"), 40))
			source.WriteString("}\n")
		}
		return source.Bytes()
	}
	source, err := os.ReadFile(filepath.Join(root, "powershell", "test", "SSHRemoting", "SSHRemoting.Basic.Tests.ps1"))
	if err != nil {
		tb.Fatal(err)
	}
	const digest = "bfa44284783f38e1508efd6a1141b9948fdb4fd9c8367b3d26278e15e0d7384f"
	if len(source) != 15220 || fmt.Sprintf("%x", sha256.Sum256(source)) != digest {
		tb.Fatal("typing fixture digest changed")
	}
	return source
}

func powerShellFocusTypingSession(source []byte) [][]byte {
	const text = "x"
	steps := make([][]byte, 200)
	for i := range steps {
		next := bytes.Clone(source)
		next = append(next, text[i%len(text)])
		steps[i] = next
		source = next
	}
	return steps
}

func TestPowerShellFocusTyping(t *testing.T) {
	source := powerShellFocusTypingSource(t)
	lang := grammars.PowershellLanguage()
	cl, err := COracleLanguage("powershell")
	if err != nil {
		t.Fatal(err)
	}
	cp := sitter.NewParser()
	defer cp.Close()
	if err := cp.SetLanguage(cl); err != nil {
		t.Fatal(err)
	}
	for _, route := range []bool{false, true} {
		t.Run(fmt.Sprintf("compact=%t", route), func(t *testing.T) {
			p := gts.NewParser(lang)
			p.SetAdmissionCandidateRoute(route)
			old, err := p.Parse(source)
			if err != nil {
				t.Fatal(err)
			}
			defer func() { old.Release() }()
			p.SetAdmissionCandidateRoute(true)
			freshParser := gts.NewParser(lang)
			freshParser.SetAdmissionCandidateRoute(true)
			t.Logf("INITIAL max_stacks=%d tokens=%d nodes=%d root_children=%d", old.ParseRuntime().MaxStacksSeen, old.ParseRuntime().TokensConsumed, old.ParseRuntime().NodesAllocated, old.RootNode().ChildCount())
			current := source
			var tokens, nodes, reusedBytes, reusedSubtrees uint64
			for step, edited := range powerShellFocusTypingSession(source) {
				at := len(current)
				edit := canonicalGoInputEdit(current, edited, at, at, at+1)
				old.Edit(edit)
				next, profile, err := p.ParseIncrementalProfiled(edited, old)
				if err != nil {
					t.Fatal(err)
				}
				fresh, err := freshParser.Parse(edited)
				if err != nil {
					t.Fatal(err)
				}
				ct := cp.Parse(edited, nil)
				got, err := benchfixtures.InspectGoTree(next.RootNode(), lang)
				if err != nil {
					t.Fatal(err)
				}
				want, err := benchfixtures.InspectGoTree(fresh.RootNode(), lang)
				if err != nil {
					t.Fatal(err)
				}
				cDigest, err := COracleDeepDigest(ct)
				if err != nil {
					t.Fatal(err)
				}
				if got.SHA256 != want.SHA256 || got.SHA256 != cDigest {
					t.Fatalf("step=%d incremental=%s fresh=%s C=%s fallback=%s compact=%s", step, got.SHA256, want.SHA256, cDigest, profile.ReuseUnsupportedReason, next.ParseRuntime().CompactIncrementalFallbackReason)
				}
				root := next.RootNode()
				if root.EndByte() != uint32(len(edited)) || (root.IsError() && !root.HasError()) {
					t.Fatal("coverage/error invariant")
				}
				if len(source) >= 4096 && step > 0 && profile.ReusedBytes == 0 {
					t.Fatalf("step=%d lost the authenticated prefix: %s", step, profile.ReuseUnsupportedReason)
				}
				tokens += profile.TokensConsumed
				nodes += profile.NewNodesAllocated
				reusedBytes += profile.ReusedBytes
				reusedSubtrees += profile.ReusedSubtrees
				if step < 4 {
					t.Logf("STEP %d tokens=%d nodes=%d reused_bytes=%d fallback=%s compact=%s", step, profile.TokensConsumed, profile.NewNodesAllocated, profile.ReusedBytes, profile.ReuseUnsupportedReason, next.ParseRuntime().CompactIncrementalFallbackReason)
				}
				fresh.Release()
				ct.Close()
				old.Release()
				old = next
				current = edited
			}
			if len(source) >= 4096 && reusedBytes == 0 {
				t.Fatal("authenticated function prefixes were never reused")
			}
			t.Logf("SESSION steps=200 tokens=%d nodes=%d reused_bytes=%d reused_subtrees=%d", tokens, nodes, reusedBytes, reusedSubtrees)
			if n := testing.AllocsPerRun(5, func() {
				same, err := p.ParseIncremental(current, old)
				if err != nil {
					t.Fatal(err)
				}
				same.Release()
			}); n != 0 {
				t.Fatalf("no-edit allocations=%g", n)
			}
		})
	}
}

func BenchmarkPowerShellFocusTyping(b *testing.B) {
	benchmarkPowerShellFocusTyping(b, false)
}

func BenchmarkPowerShellFocusTypingLatency(b *testing.B) {
	benchmarkPowerShellFocusTyping(b, true)
}

func benchmarkPowerShellFocusTyping(b *testing.B, collectLatency bool) {
	source := powerShellFocusTypingSource(b)
	steps := powerShellFocusTypingSession(source)
	edits := make([]gts.InputEdit, len(steps))
	previous := source
	for i, edited := range steps {
		at := len(previous)
		edits[i] = canonicalGoInputEdit(previous, edited, at, at, at+1)
		previous = edited
	}
	lang := grammars.PowershellLanguage()
	engines := []string{"Go", "C", "C", "Go"}
	for _, index := range scannerFocusBenchOrder(b.Name(), len(engines)) {
		engine := engines[index]
		b.Run(fmt.Sprintf("%s%d", engine, index), func(b *testing.B) {
			b.ReportAllocs()
			b.SetBytes(int64(200))
			b.StopTimer()
			var latencies []int64
			if collectLatency {
				latencies = make([]int64, 0, b.N*len(steps))
			}
			if engine == "Go" {
				p := gts.NewParser(lang)
				p.SetAdmissionCandidateRoute(true)
				for i := 0; i < b.N; i++ {
					b.StopTimer()
					old, err := p.Parse(source)
					if err != nil {
						b.Fatal(err)
					}
					b.StartTimer()
					for step, edited := range steps {
						var started time.Time
						if collectLatency {
							started = time.Now()
						}
						edit := edits[step]
						old.Edit(edit)
						next, err := p.ParseIncremental(edited, old)
						if err != nil {
							b.Fatal(err)
						}
						old.Release()
						old = next
						if collectLatency {
							latencies = append(latencies, time.Since(started).Nanoseconds())
						}
					}
					b.StopTimer()
					old.Release()
					b.StartTimer()
				}
			} else {
				cl, err := COracleLanguage("powershell")
				if err != nil {
					b.Fatal(err)
				}
				p := sitter.NewParser()
				defer p.Close()
				if err := p.SetLanguage(cl); err != nil {
					b.Fatal(err)
				}
				for i := 0; i < b.N; i++ {
					b.StopTimer()
					old := p.Parse(source, nil)
					b.StartTimer()
					for step, edited := range steps {
						var started time.Time
						if collectLatency {
							started = time.Now()
						}
						edit := scannerFocusCEdit(edits[step])
						old.Edit(&edit)
						next := p.Parse(edited, old)
						if next == nil {
							b.Fatal("C parse returned nil")
						}
						old.Close()
						old = next
						if collectLatency {
							latencies = append(latencies, time.Since(started).Nanoseconds())
						}
					}
					b.StopTimer()
					old.Close()
					b.StartTimer()
				}
			}
			b.StopTimer()
			if collectLatency {
				slices.Sort(latencies)
				index := (len(latencies)*99+99)/100 - 1
				b.ReportMetric(float64(latencies[index]), "p99-ns/key")
			}
		})
	}
}

func BenchmarkPowerShellFocusFull(b *testing.B) {
	source := powerShellFocusTypingSource(b)
	p := gts.NewParser(grammars.PowershellLanguage())
	p.SetAdmissionCandidateRoute(true)
	warm, err := p.Parse(source)
	if err != nil {
		b.Fatal(err)
	}
	warm.Release()
	b.ReportAllocs()
	b.SetBytes(int64(len(source)))
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		tree, err := p.Parse(source)
		if err != nil {
			b.Fatal(err)
		}
		tree.Release()
	}
}
