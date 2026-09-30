//go:build cgo && treesitter_c_parity

package cgoharness

import (
	"crypto/sha256"
	"fmt"
	"testing"

	gts "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/internal/benchfixtures"
	sitter "github.com/tree-sitter/go-tree-sitter"
)

func TestIncrementalCReuseSizes(t *testing.T) {
	for _, name := range []string{"go", "java"} {
		t.Run(name, func(t *testing.T) {
			for _, size := range []int{32, 1024} {
				t.Run(fmt.Sprintf("%dKiB", size), func(t *testing.T) {
					source, edited, edit, lang := cReuseFixtureAtSize(t, name, size*1024)
					p := gts.NewParser(lang)
					p.SetAdmissionCandidateRoute(false)
					old, err := p.Parse(source)
					if err != nil {
						t.Fatal(err)
					}
					defer old.Release()
					if old.ParseStopReason() != gts.ParseStopAccepted || old.RootNode().HasError() || old.RootNode().EndByte() != uint32(len(source)) {
						t.Fatalf("initial parse stop=%s error=%t end=%d bytes=%d", old.ParseStopReason(), old.RootNode().HasError(), old.RootNode().EndByte(), len(source))
					}
					cReuseBeginWorkCount()
					old.Edit(edit)
					next, profile, err := p.ParseIncrementalProfiled(edited, old)
					if err != nil {
						t.Fatal(err)
					}
					cReuseEndWorkCount(t)
					defer next.Release()
					fresh, err := p.Parse(edited)
					if err != nil {
						t.Fatal(err)
					}
					defer fresh.Release()
					cl, err := COracleLanguage(name)
					if err != nil {
						t.Fatal(err)
					}
					cp := sitter.NewParser()
					defer cp.Close()
					if err := cp.SetLanguage(cl); err != nil {
						t.Fatal(err)
					}
					ct := cp.Parse(edited, nil)
					defer ct.Close()
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
						t.Fatalf("incremental=%s fresh=%s C=%s", got.SHA256, want.SHA256, oracle)
					}
					if next.ParseStopReason() != gts.ParseStopAccepted || next.RootNode().HasError() || next.RootNode().EndByte() != uint32(len(edited)) {
						t.Fatal("incremental parse lost clean completion")
					}
					allocs := testing.AllocsPerRun(3, func() {
						same, err := p.ParseIncremental(edited, next)
						if err != nil {
							t.Fatal(err)
						}
						same.Release()
					})
					if allocs != 0 {
						t.Fatalf("no-edit allocations=%g", allocs)
					}
					t.Logf("bytes=%d source=%x edited=%x digest=%s tokens=%d nodes=%d reused_subtrees=%d reused_bytes=%d", len(source), sha256.Sum256(source), sha256.Sum256(edited), got.SHA256, profile.TokensConsumed, profile.NewNodesAllocated, profile.ReusedSubtrees, profile.ReusedBytes)
				})
			}
		})
	}
}
