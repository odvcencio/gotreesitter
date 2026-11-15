//go:build cgo && treesitter_c_parity

package editorlatency

// Time through the public API of the ccore route against the legacy default
// (directional, one language per process). The arms alternate within a run:
//
//	CCORE_TIMING=1 GTS_EDIT_LANGUAGE=<lang|go-1mib> GTS_EDIT_MANIFEST=... GTS_EDIT_FIXTURES=... \
//	  go test -run '^TestCcoreRouteTiming$'
//
// Each cell is timed as ns per edit (Tree.Edit plus Parser.ParseIncremental,
// the old tree released), and as ns per fresh parse. For the routed arm the
// time is split into the core parse, the conversion to the public tree, the
// core's tree edit, and the position update of the public nodes (the rest of
// Tree.Edit).

import (
	"fmt"
	"os"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"

	gts "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
	"github.com/odvcencio/gotreesitter/internal/ccore/bind"
	"github.com/odvcencio/gotreesitter/internal/ccore/bind/bindtest"
	"github.com/odvcencio/gotreesitter/internal/scanmode"
)

// Register unchecked scanners for comparison in this test binary.
func init() {
	for _, entry := range grammars.AllLanguages() {
		scanner := entry.Language().ExternalScanner
		if scanner == nil {
			continue
		}
		if _, ok := scanmode.Lookup(scanner); ok {
			continue
		}
		payload := scanner.Create()
		scanner.Destroy(payload)
		scanmode.RegisterScanner(scanner, payload, false, func(v any) (any, []byte, bool) {
			return v, []byte(reflect.TypeOf(v).String()), true
		})
	}
}

func ccoreTimingGo1MiB() (driver, []workload) {
	var b strings.Builder
	b.WriteString("package main\n\n")
	for i := 0; i < 22000; i++ {
		fmt.Fprintf(&b, "func f%d() int { v := %d; return v }\n", i, i)
	}
	src := []byte(b.String())
	e := grammars.DetectLanguageByName("go")
	d := driver{source: src, entry: *e, lang: e.Language()}
	at := len(src) / 2
	for src[at] < '0' || src[at] > '9' {
		at++
	}
	old := src[at]
	nw := byte('0' + (old-'0'+1)%10)
	one := editStep(src, at, at+1, []byte{nw})
	undo := editStep(one.source, at, at+1, []byte{old})
	return d, []workload{{name: "one_byte", initial: src, steps: []step{one, undo}}}
}

func median(v []float64) float64 {
	s := append([]float64(nil), v...)
	sort.Float64s(s)
	return s[len(s)/2]
}

func TestCcoreRouteTiming(t *testing.T) {
	if os.Getenv("CCORE_TIMING") == "" {
		t.Skip("set CCORE_TIMING")
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
	_, svc0, _ := bind.RouteCounters() // the core must serve the timed requests: a fallback must not be timed as core work
	lang := d.lang
	mk := func(routed bool) *gts.Parser {
		p := gts.NewParser(lang)
		p.SetAdmissionCandidateRoute(routed)
		return p
	}
	// fresh parse
	freshNs := map[string][]float64{}
	var freshSplit [2]float64
	for r := 0; r < runs+1; r++ {
		for _, routed := range []bool{false, true} {
			p := mk(routed)
			bind.RouteNanos(true)
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
			per := float64(time.Since(start).Nanoseconds()) / float64(n)
			if r > 0 { // run 0 is a warm-up
				k := "legacy"
				if routed {
					k = "routed"
					pa, co, _, _ := bind.RouteNanos(true)
					freshSplit = [2]float64{float64(pa) / float64(n), float64(co) / float64(n)}
				}
				freshNs[k] = append(freshNs[k], per)
			}
		}
	}
	fmt.Printf("CCTIME lang=%s cell=fresh bytes=%d legacy_ns=%.0f routed_ns=%.0f routed_parse_ns=%.0f routed_convert_ns=%.0f runs=%d\n",
		name, len(d.source), median(freshNs["legacy"]), median(freshNs["routed"]), freshSplit[0], freshSplit[1], runs)
	for _, w := range cells {
		ns := map[string][]float64{}
		var split [5]float64
		for r := 0; r < runs+1; r++ {
			for _, routed := range []bool{false, true} {
				p := mk(routed)
				tree, err := p.Parse(w.initial)
				if err != nil {
					t.Fatal(err)
				}
				bind.RouteNanos(true)
				var total, editTotal time.Duration
				edits := 0
				start := time.Now()
				for time.Since(start) < budget || edits < 2*len(w.steps) {
					for _, st := range w.steps {
						t0 := time.Now()
						tree.Edit(st.edit)
						t1 := time.Now()
						next, err := p.ParseIncremental(st.source, tree)
						if err != nil {
							t.Fatal(err)
						}
						t2 := time.Now()
						editTotal += t1.Sub(t0)
						total += t2.Sub(t0)
						if next != tree {
							tree.Release()
						}
						tree = next
						edits++
					}
					// return to the initial text: the steps of a cell end back on it, except typing
					if w.name == "typing" {
						tree.Release()
						tree, _ = p.Parse(w.initial)
					}
				}
				tree.Release()
				if r == 0 {
					continue
				}
				k := "legacy"
				if routed {
					k = "routed"
					pa, co, ce, ck := bind.RouteNanos(true)
					e := float64(edits)
					split = [5]float64{float64(pa) / e, float64(co) / e, float64(ce) / e, (float64(editTotal.Nanoseconds()) - float64(ce)) / e, float64(ck) / e}
				}
				ns[k] = append(ns[k], float64(total.Nanoseconds())/float64(edits))
			}
		}
		fmt.Printf("CCTIME lang=%s cell=%s bytes=%d legacy_ns=%.0f routed_ns=%.0f routed_parse_ns=%.0f routed_convert_ns=%.0f routed_core_edit_ns=%.0f routed_position_update_ns=%.0f routed_edit_check_ns=%.0f runs=%d\n",
			name, w.name, len(d.source), median(ns["legacy"]), median(ns["routed"]), split[0], split[1], split[2], split[3], split[4], runs)
	}
	if decl, svc1, _ := bind.RouteCounters(); svc1 <= svc0 || len(decl) != 0 {
		t.Fatalf("the core served %d requests during the timing, declines %v", svc1-svc0, decl)
	}
}
