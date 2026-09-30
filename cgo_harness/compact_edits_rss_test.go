//go:build linux && cgo && treesitter_c_parity && gts_engine_ceiling

package cgoharness

import (
	"encoding/json"
	"os"
	"syscall"
	"testing"

	gts "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
)

// Run each language/engine/mode in its own process under /usr/bin/time -v.
// Keep the default GC policy: the allocation-counting probe disables GC and
// therefore cannot supply an RSS receipt. Include declined compact attempts
// and their fallback in the whole-operation footprint.
func TestCompactEditsRSS(t *testing.T) {
	engine, mode := os.Getenv("GTS_CEILING_RSS_ENGINE"), os.Getenv("GTS_CEILING_RSS_MODE")
	if engine != "C" && engine != "legacy" && engine != "compact" {
		t.Fatal("set GTS_CEILING_RSS_ENGINE to C, legacy or compact")
	}
	var input ceilingInput
	found := false
	for _, candidate := range ceilingInputs(t) {
		if candidate.size == "1m" && candidate.mode == mode {
			input, found = candidate, true
			break
		}
	}
	if !found {
		t.Fatal("set GTS_CEILING_RSS_MODE to a 1 MiB fixture mode")
	}
	served, declined := uint64(0), uint64(0)
	reason := ""
	if engine == "C" {
		language, err := cOracleRawLanguage(input.language)
		if err != nil {
			t.Fatal(err)
		}
		p := newCeilingCParser(language, input.source[0], input.source[1], input.edit[0], input.edit[1], mode != "fresh")
		if p == nil {
			t.Fatal("C failed to initialize a complete tree")
		}
		if p.batch(4, false).failed {
			p.close()
			t.Fatal("C parse failed")
		}
		p.close()
	} else {
		p := gts.NewParser(grammars.DetectLanguageByName(input.language).Language())
		p.SetAdmissionCandidateRoute(engine == "compact")
		gts.ResetAdmissionCandidateCounters()
		tree, err := p.Parse(input.source[0])
		ceilingGoTree(t, tree, input.source[0], err)
		if mode == "fresh" {
			tree.Release()
			tree = nil
		}
		for step := 0; step < 4; step++ {
			var next *gts.Tree
			var source []byte
			if mode == "fresh" {
				source = input.source[0]
				next, err = p.Parse(source)
			} else {
				direction := step % 2
				source = input.source[1-direction]
				tree.Edit(input.edit[direction])
				next, err = p.ParseIncremental(source, tree)
				tree.Release()
			}
			ceilingGoTree(t, next, source, err)
			if next.RootNode().HasError() {
				t.Fatal("clean RSS fixture has an error")
			}
			if mode == "fresh" {
				next.Release()
			} else {
				tree = next
			}
		}
		if tree != nil {
			tree.Release()
		}
		served, declined = gts.AdmissionCandidateCounters()
		reason = gts.AdmissionCandidateLastFallbackReason()
	}
	var usage syscall.Rusage
	if err := syscall.Getrusage(syscall.RUSAGE_SELF, &usage); err != nil {
		t.Fatal(err)
	}
	rss := usage.Maxrss * 1024
	receipt, err := json.Marshal(map[string]any{
		"language": input.language, "engine": engine, "mode": mode,
		"source_bytes": len(input.source[0]), "max_rss_bytes": rss,
		"rss_per_source_byte": float64(rss) / float64(len(input.source[0])),
		"compact_served":      served, "compact_declined": declined, "decline_reason": reason,
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("RSS %s", receipt)
	if rss > int64(len(input.source[0]))*400 {
		t.Fatalf("RSS exceeds 400 bytes per source byte: %d/%d", rss, len(input.source[0]))
	}
}
