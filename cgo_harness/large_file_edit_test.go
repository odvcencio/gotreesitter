//go:build cgo && treesitter_c_parity

package cgoharness

import (
	"crypto/sha256"
	"testing"
	"time"

	gts "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/internal/benchfixtures"
	sitter "github.com/tree-sitter/go-tree-sitter"
)

// Run each language in its own process, including RSS and profile runs.
func BenchmarkLargeFileEdit(b *testing.B) {
	for _, name := range []string{"c_sharp", "go", "java", "typescript", "python"} {
		b.Run(name, func(b *testing.B) {
			source, edited, edit, lang := cReuseFixtureAtSize(b, name, 1024*1024)
			// Paired Go-C-C-Go cycles keep the complete operation identical.
			for _, implementation := range []string{"GoFirst", "CFirst", "CSecond", "GoSecond"} {
				b.Run(implementation, func(b *testing.B) {
					b.ReportAllocs()
					b.SetBytes(int64(len(source)))
					if implementation == "GoFirst" || implementation == "GoSecond" {
						p := gts.NewParser(lang)
						p.SetAdmissionCandidateRoute(false)
						tree, err := p.Parse(source)
						if err != nil {
							b.Fatal(err)
						}
						defer func() { tree.Release() }()
						b.ResetTimer()
						for i := 0; i < b.N; i++ {
							to := edited
							if i%2 != 0 {
								to = source
							}
							tree.Edit(edit)
							next, err := p.ParseIncremental(to, tree)
							if err != nil {
								b.Fatal(err)
							}
							tree.Release()
							tree = next
						}
						b.StopTimer()
						if tree.ParseStopReason() != gts.ParseStopAccepted || tree.RootNode().HasError() || tree.RootNode().EndByte() != uint32(len(source)) {
							b.Fatal("Go edit lost clean completion")
						}
					} else {
						cl, err := COracleLanguage(name)
						if err != nil {
							b.Fatal(err)
						}
						p := sitter.NewParser()
						defer p.Close()
						if err := p.SetLanguage(cl); err != nil {
							b.Fatal(err)
						}
						tree := largeFileEditCInitial(p, source)
						if tree == nil {
							b.Fatal("C initial parse failed")
						}
						defer func() { tree.Close() }()
						ce := realCorpusCInputEdit(edit)
						b.ResetTimer()
						for i := 0; i < b.N; i++ {
							to := edited
							if i%2 != 0 {
								to = source
							}
							tree.Edit(&ce)
							next := p.Parse(to, tree)
							if next == nil {
								b.Fatal("C incremental parse failed")
							}
							tree.Close()
							tree = next
						}
						b.StopTimer()
						if tree.RootNode().HasError() || tree.RootNode().EndByte() != uint(len(source)) {
							b.Fatal("C edit lost clean completion")
						}
					}
				})
			}
		})
	}
}

func TestLargeFileEditInvariant(t *testing.T) {
	for _, name := range []string{"c_sharp", "go", "java", "typescript", "python"} {
		t.Run(name, func(t *testing.T) {
			source, edited, edit, lang := cReuseFixtureAtSize(t, name, 1024*1024)
			p := gts.NewParser(lang)
			p.SetAdmissionCandidateRoute(false)
			old, err := p.Parse(source)
			if err != nil {
				t.Fatal(err)
			}
			defer func() { old.Release() }()
			cl, err := COracleLanguage(name)
			if err != nil {
				t.Fatal(err)
			}
			cp := sitter.NewParser()
			defer cp.Close()
			if err := cp.SetLanguage(cl); err != nil {
				t.Fatal(err)
			}
			ct := cp.Parse(source, nil)
			defer func() { ct.Close() }()
			for step := 0; step < 4; step++ {
				to := edited
				if step%2 != 0 {
					to = source
				}
				cReuseBeginWorkCount()
				start := time.Now()
				old.Edit(edit)
				editNS := time.Since(start).Nanoseconds()
				next, profile, err := p.ParseIncrementalProfiled(to, old)
				if err != nil {
					t.Fatal(err)
				}
				cReuseEndWorkCount(t)
				ce := realCorpusCInputEdit(edit)
				ct.Edit(&ce)
				cn := cp.Parse(to, ct)
				if cn == nil {
					t.Fatal("C incremental parse failed")
				}
				ct.Close()
				ct = cn
				fresh, err := p.Parse(to)
				if err != nil {
					t.Fatal(err)
				}
				got, err := benchfixtures.InspectGoTree(next.RootNode(), lang)
				if err != nil {
					t.Fatal(err)
				}
				want, err := benchfixtures.InspectGoTree(fresh.RootNode(), lang)
				if err != nil {
					t.Fatal(err)
				}
				oracle, err := COracleDeepDigest(ct)
				if err != nil {
					t.Fatal(err)
				}
				if got.SHA256 != want.SHA256 || got.SHA256 != oracle {
					t.Fatalf("step=%d incremental=%s fresh=%s C=%s", step, got.SHA256, want.SHA256, oracle)
				}
				if name == "typescript" {
					// Certified reuse keeps the same locked-C digest while reducing
					// the 1 MiB frontier from 918299 fresh nodes to 28477 nodes.
					// Keep the discarded-work guard for uncertified fresh results.
					freshNodes := uint64(fresh.ParseRuntime().NodesAllocated)
					if profile.ReusedSubtrees == 0 && profile.NewNodesAllocated != freshNodes {
						t.Fatalf("large uncertified frontier built discarded nodes: edit=%d fresh=%d", profile.NewNodesAllocated, freshNodes)
					}
					if profile.ReusedSubtrees != 0 && profile.NewNodesAllocated >= freshNodes {
						t.Fatalf("certified frontier rebuilt the full tree: edit=%d fresh=%d reused=%d", profile.NewNodesAllocated, freshNodes, profile.ReusedSubtrees)
					}
				}
				root := next.RootNode()
				if next.ParseStopReason() != gts.ParseStopAccepted || root.HasError() || root.EndByte() != uint32(len(to)) {
					t.Fatalf("step=%d stop=%s error=%t end=%d bytes=%d", step, next.ParseStopReason(), root.HasError(), root.EndByte(), len(to))
				}
				t.Logf("LARGE_EDIT language=%s bytes=%d source=%x step=%d digest=%s tokens=%d nodes=%d reused_subtrees=%d reused_bytes=%d edit_ns=%d reuse_ns=%d reparse_ns=%d arena_bytes=%d arena_baseline=%d fallback=%s", name, len(to), sha256.Sum256(to), step, got.SHA256, profile.TokensConsumed, profile.NewNodesAllocated, profile.ReusedSubtrees, profile.ReusedBytes, editNS, profile.ReuseCursorNanos, profile.ReparseNanos, profile.ArenaBytesAllocated, profile.ArenaBaselineBytes, profile.ReuseUnsupportedReason)
				fresh.Release()
				old.Release()
				old = next
			}
			allocs := testing.AllocsPerRun(3, func() {
				same, err := p.ParseIncremental(source, old)
				if err != nil {
					t.Fatal(err)
				}
				same.Release()
			})
			if allocs != 0 {
				t.Fatalf("no-edit allocations=%g", allocs)
			}
		})
	}
}

func TestLargeFileEditStopControls(t *testing.T) {
	source, edited, edit, lang := cReuseFixtureAtSize(t, "typescript", 1024*1024)
	initial := gts.NewParser(lang)
	initial.SetAdmissionCandidateRoute(false)
	old, err := initial.Parse(source)
	if err != nil {
		t.Fatal(err)
	}
	defer old.Release()
	old.Edit(edit)
	var cancelled uint32 = 1
	for _, test := range []struct {
		name      string
		configure func(*gts.Parser)
		stop      gts.ParseStopReason
	}{
		{"node_limit", func(p *gts.Parser) { p.SetParseWorkLimits(gts.ParseWorkLimits{NodeLimit: 100}) }, gts.ParseStopNodeLimit},
		{"iteration_limit", func(p *gts.Parser) { p.SetParseWorkLimits(gts.ParseWorkLimits{IterationLimit: 100}) }, gts.ParseStopIterationLimit},
		{"cancelled", func(p *gts.Parser) { p.SetCancellationFlag(&cancelled) }, gts.ParseStopCancelled},
	} {
		t.Run(test.name, func(t *testing.T) {
			p := gts.NewParser(lang)
			p.SetAdmissionCandidateRoute(false)
			test.configure(p)
			next, err := p.ParseIncremental(edited, old)
			if err != nil {
				t.Fatal(err)
			}
			defer next.Release()
			fresh, err := p.Parse(edited)
			if err != nil {
				t.Fatal(err)
			}
			defer fresh.Release()
			t.Logf("STOP_CONTROL name=%s incremental_stop=%s fresh_stop=%s incremental_iterations=%d fresh_iterations=%d incremental_nodes=%d fresh_nodes=%d", test.name, next.ParseStopReason(), fresh.ParseStopReason(), next.ParseRuntime().Iterations, fresh.ParseRuntime().Iterations, next.ParseRuntime().NodesAllocated, fresh.ParseRuntime().NodesAllocated)
			got, err := benchfixtures.InspectGoTree(next.RootNode(), lang)
			if err != nil {
				t.Fatal(err)
			}
			want, err := benchfixtures.InspectGoTree(fresh.RootNode(), lang)
			if err != nil {
				t.Fatal(err)
			}
			if got.SHA256 != want.SHA256 || next.ParseStopReason() != test.stop || fresh.ParseStopReason() != test.stop {
				t.Fatalf("incremental=%s (%s) fresh=%s (%s), want stop=%s", got.SHA256, next.ParseStopReason(), want.SHA256, fresh.ParseStopReason(), test.stop)
			}
			if next.RootNode().IsError() && !next.RootNode().HasError() {
				t.Fatal("ERROR root lost HasError")
			}
		})
	}
}
