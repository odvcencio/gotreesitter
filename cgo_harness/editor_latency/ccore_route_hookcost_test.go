//go:build cgo && treesitter_c_parity

package editorlatency

import (
	"fmt"
	"os"
	"strconv"
	"testing"
	"time"

	gts "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/internal/ccore/bind"
	"github.com/odvcencio/gotreesitter/internal/ccore/bind/bindtest"
)

// TestCcoreRouteHookCost times the routed parse in one of two arms, one arm per
// process (the hard ceiling is read once from the environment):
//
//	CCORE_HOOK_ARM=free     GOT_PARSE_MEMORY_HARD_CEILING_MB=0: the soft budget is switched
//	                        off in the test and the ceiling by the variable, so no progress
//	                        hook is installed (asserted before timing)
//	CCORE_HOOK_ARM=default  the committed default: the default budget and the default hard
//	                        ceiling (2048 MiB) are in force and the hook is installed (asserted)
//
// A launcher runs the two arms alternately and compares the medians. Output is
// ns per fresh parse and per edit:
//
//	CCORE_TIMING=1 CCORE_HOOK_ARM=default GTS_EDIT_LANGUAGE=go-1mib go test -run '^TestCcoreRouteHookCost$'
func TestCcoreRouteHookCost(t *testing.T) {
	if os.Getenv("CCORE_TIMING") == "" {
		t.Skip("set CCORE_TIMING")
	}
	arm := os.Getenv("CCORE_HOOK_ARM")
	if arm != "free" && arm != "default" {
		t.Skip("set CCORE_HOOK_ARM=free or default")
	}
	runs := 9
	if v, err := strconv.Atoi(os.Getenv("CCORE_RUNS")); err == nil && v > 0 {
		runs = v
	}
	budget := 300 * time.Millisecond
	var d driver
	var cells []workload
	name := os.Getenv("GTS_EDIT_LANGUAGE")
	if name == "go-1mib" {
		d, cells = ccoreTimingGo1MiB()
	} else {
		d, _ = loadDriver(t)
		cells = d.workloads()
	}
	restore, err := bindtest.CertifyForTests(1|2|4|8, []string{"ImmediateTokens", "ZeroWidthTokens", "LayoutFallbackLexState", "AfterWhitespaceLexState"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer restore()
	softOff := arm == "free"
	soft, ceiling, hooked, err := bind.RouteLimits(d.lang, len(d.source), softOff)
	if err != nil {
		t.Fatal(err)
	}
	if arm == "free" && (soft > 0 || ceiling > 0 || hooked) {
		t.Fatalf("the hook-free arm has a limit: soft %d, ceiling %d, hook installed %t (run with GOT_PARSE_MEMORY_HARD_CEILING_MB=0)", soft, ceiling, hooked)
	}
	if arm == "default" && (soft <= 0 || ceiling <= 0 || !hooked) {
		t.Fatalf("the default arm lacks a limit: soft %d, ceiling %d, hook installed %t", soft, ceiling, hooked)
	}
	_, svc0, _ := bind.RouteCounters() // the core must serve the timed requests: a fallback must not be timed as core work
	fmt.Printf("CCHOOKARM arm=%s lang=%s soft=%d ceiling=%d hook=%t\n", arm, name, soft, ceiling, hooked)
	mk := func() *gts.Parser {
		p := gts.NewParser(d.lang)
		p.SetAdmissionCandidateRoute(true)
		if softOff {
			p.SetMemoryBudgetBytes(-1)
		}
		return p
	}
	var fresh []float64
	for r := 0; r < runs+1; r++ {
		p := mk()
		n := 0
		start := time.Now()
		for time.Since(start) < budget || n < 2 {
			tr, err := p.Parse(d.source)
			if err != nil {
				t.Fatal(err)
			}
			tr.Release()
			n++
		}
		if r > 0 {
			fresh = append(fresh, float64(time.Since(start).Nanoseconds())/float64(n))
		}
	}
	fmt.Printf("CCHOOK arm=%s lang=%s cell=fresh bytes=%d ns=%.0f runs=%d\n", arm, name, len(d.source), median(fresh), runs)
	for _, w := range cells {
		var ns []float64
		for r := 0; r < runs+1; r++ {
			p := mk()
			tree, err := p.Parse(w.initial)
			if err != nil {
				t.Fatal(err)
			}
			var total time.Duration
			edits := 0
			start := time.Now()
			for time.Since(start) < budget || edits < 2*len(w.steps) {
				for _, st := range w.steps {
					t0 := time.Now()
					tree.Edit(st.edit)
					next, err := p.ParseIncremental(st.source, tree)
					if err != nil {
						t.Fatal(err)
					}
					total += time.Since(t0)
					if next != tree {
						tree.Release()
					}
					tree = next
					edits++
				}
				if w.name == "typing" {
					tree.Release()
					tree, _ = p.Parse(w.initial)
				}
			}
			tree.Release()
			if r > 0 {
				ns = append(ns, float64(total.Nanoseconds())/float64(edits))
			}
		}
		fmt.Printf("CCHOOK arm=%s lang=%s cell=%s bytes=%d ns=%.0f runs=%d\n", arm, name, w.name, len(d.source), median(ns), runs)
	}
	if decl, svc1, _ := bind.RouteCounters(); svc1 <= svc0 || len(decl) != 0 {
		t.Fatalf("the core served %d requests during the timing, declines %v", svc1-svc0, decl)
	}
}
