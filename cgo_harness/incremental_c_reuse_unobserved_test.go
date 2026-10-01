//go:build cgo && treesitter_c_parity

package cgoharness

import (
	"fmt"
	"testing"

	gts "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/internal/benchfixtures"
	sitter "github.com/tree-sitter/go-tree-sitter"
)

// Inspecting children also wires deferred parent links. Check every session
// prefix independently so those reads cannot repair the next reuse attempt.
func TestIncrementalCReuseUnobservedSession(t *testing.T) {
	for _, name := range cReuseLanguages {
		t.Run(name, func(t *testing.T) {
			f := cReuseLanguageFixture(t, name, 137*1024, "byte1")
			cl, err := COracleLanguage(name)
			if err != nil {
				t.Fatal(err)
			}
			cp := sitter.NewParser()
			defer cp.Close()
			if err := cp.SetLanguage(cl); err != nil {
				t.Fatal(err)
			}
			for length := 1; length <= 12; length++ {
				t.Run(fmt.Sprintf("prefix%d", length), func(t *testing.T) {
					p := gts.NewParser(f.lang)
					p.SetAdmissionCandidateRoute(false)
					old, err := p.Parse(f.source)
					if err != nil {
						t.Fatal(err)
					}
					defer func() { old.Release() }()
					to := f.source
					for step := 0; step < length; step++ {
						to = f.edited
						edit := f.forward
						if step%2 != 0 {
							to, edit = f.source, f.reverse
						}
						old.Edit(edit)
						next, err := p.ParseIncremental(to, old)
						if err != nil {
							t.Fatal(err)
						}
						old.Release()
						old = next
					}
					runtime := old.ParseRuntime()
					t.Logf("prefix=%d tokens=%d nodes=%d stop=%s", length, runtime.TokensConsumed, runtime.NodesAllocated, runtime.StopReason)
					fresh, err := p.Parse(to)
					if err != nil {
						t.Fatal(err)
					}
					defer fresh.Release()
					ct := cp.Parse(to, nil)
					if ct == nil {
						t.Fatal("missing C tree")
					}
					defer ct.Close()
					got, err := benchfixtures.InspectGoTree(old.RootNode(), f.lang)
					if err != nil {
						t.Fatal(err)
					}
					want, err := benchfixtures.InspectGoTree(fresh.RootNode(), f.lang)
					if err != nil {
						t.Fatal(err)
					}
					oracle, err := COracleDeepDigest(ct)
					if err != nil {
						t.Fatal(err)
					}
					if got.SHA256 != want.SHA256 || got.SHA256 != oracle || old.RootNode().HasError() || old.RootNode().EndByte() != uint32(len(to)) || old.ParseStopReason() != gts.ParseStopAccepted {
						t.Fatalf("incremental=%s fresh=%s C=%s error=%t", got.SHA256, want.SHA256, oracle, old.RootNode().HasError())
					}
					if allocs := testing.AllocsPerRun(3, func() {
						next, err := p.ParseIncremental(to, old)
						if err != nil {
							t.Fatal(err)
						}
						next.Release()
					}); allocs != 0 {
						t.Fatalf("no-edit allocations=%g", allocs)
					}
				})
			}
		})
	}
}
