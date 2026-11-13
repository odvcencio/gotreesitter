package ccore_test

import (
	"testing"

	"github.com/odvcencio/gotreesitter/internal/ccore"
)

// TestReduceLinearCheck replays the goldens with the cross-check of the linear
// reduction switched on (it panics on a difference from the general walk), compares
// each replay with the golden (trace, digest, stop, records) and requires that the
// linear path ran.
func TestReduceLinearCheck(t *testing.T) {
	fast := 0
	for _, g := range loadGoldens(t) {
		p, err := ccore.NewParser(ccore.Config{Tables: g.Tables})
		if err != nil {
			t.Fatal(err)
		}
		p.SetReduceCheck(true)
		var trace []string
		p.SetLogger(func(s string) { trace = append(trace, s) })
		tr, stop := p.Parse(g.Src)
		p.SetLogger(nil)
		if stop != g.Stop {
			t.Errorf("%s: stop %q differs from the golden", g.Name, stop)
		}
		if g.Stop == "" && tr == nil { // a finished parse must return its tree: the comparisons below must not be skipped
			t.Errorf("%s: no tree for a golden that finishes", g.Name)
		}
		if tr != nil {
			if p.Digest(tr) != g.Digest {
				t.Errorf("%s: digest differs from the golden", g.Name)
			}
			if hash, lines := p.Records(tr, nil); hash != g.RecordHash || lines != g.RecordCount {
				t.Errorf("%s: records differ from the golden", g.Name)
			}
			tr.Release()
		}
		if len(trace) != len(g.Trace) {
			t.Errorf("%s: %d trace lines, want %d", g.Name, len(trace), len(g.Trace))
		} else {
			for i := range trace {
				if trace[i] != g.Trace[i] {
					t.Errorf("%s: trace line %d is %q, want %q", g.Name, i+1, trace[i], g.Trace[i])
					break
				}
			}
		}
		fast += p.FastReductions()
	}
	if fast == 0 {
		t.Fatal("no reduction took the linear path: the check compared nothing")
	}
}
