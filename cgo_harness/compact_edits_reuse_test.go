//go:build cgo && treesitter_c_parity && gts_engine_ceiling

package cgoharness

import (
	"fmt"
	"testing"

	gts "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
	"github.com/odvcencio/gotreesitter/internal/benchfixtures"
	sitter "github.com/tree-sitter/go-tree-sitter"
)

// Run one language per process. Every direction compares full public trees,
// including fields, flags and points, rather than only their S-expressions.
func TestCompactEditsReuse(t *testing.T) {
	name := ceilingLanguage(t)
	lang := grammars.DetectLanguageByName(name).Language()
	cl, err := COracleLanguage(name)
	if err != nil {
		t.Fatal(err)
	}
	cp := sitter.NewParser()
	defer cp.Close()
	if err := cp.SetLanguage(cl); err != nil {
		t.Fatal(err)
	}
	for _, input := range ceilingInputs(t) {
		if input.mode == "fresh" {
			continue
		}
		t.Run(input.size+"/"+input.mode, func(t *testing.T) {
			p := gts.NewParser(lang)
			p.SetAdmissionCandidateRoute(true)
			gts.ResetAdmissionCandidateCounters()
			tree, err := p.Parse(input.source[0])
			ceilingGoTree(t, tree, input.source[0], err)
			defer func() { tree.Release() }()
			served, declined := gts.AdmissionCandidateCounters()
			if served != 1 || declined != 0 {
				t.Fatalf("initial compact declined: %s", gts.AdmissionCandidateLastFallbackReason())
			}
			for step := 0; step < 4; step++ {
				direction := step % 2
				source := input.source[1-direction]
				cReuseBeginWorkCount()
				tree.Edit(input.edit[direction])
				next, profile, err := p.ParseIncrementalProfiled(source, tree)
				cReuseEndWorkCount(t)
				ceilingGoTree(t, next, source, err)
				tree.Release()
				tree = next
				fresh, err := p.Parse(source)
				ceilingGoTree(t, fresh, source, err)
				ct := cp.Parse(source, nil)
				if ct == nil {
					t.Fatal("C returned no tree")
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
				fresh.Release()
				ct.Close()
				if got.SHA256 != want.SHA256 || got.SHA256 != oracle {
					t.Fatalf("step=%d incremental=%s fresh=%s C=%s", step, got.SHA256, want.SHA256, oracle)
				}
				rt := next.ParseRuntime()
				if !rt.CompactIncrementalReuseRoute || profile.ReusedSubtrees == 0 || profile.ReusedBytes == 0 {
					t.Fatalf("step=%d compact declined: %s profile=%+v", step, rt.CompactIncrementalFallbackReason, profile)
				}
				t.Logf("COUNTERS %s/%s/%s step=%d tokens=%d nodes=%d reused=%d/%d digest=%s", name, input.size, input.mode, step, profile.TokensConsumed, profile.NewNodesAllocated, profile.ReusedSubtrees, profile.ReusedBytes, got.SHA256)
			}
			allocations := testing.AllocsPerRun(3, func() {
				next, err := p.ParseIncremental(input.source[0], tree)
				if err != nil {
					panic(err)
				}
				next.Release()
			})
			if allocations != 0 {
				t.Fatal(fmt.Sprintf("no-edit allocations=%g", allocations))
			}
		})
	}
}
