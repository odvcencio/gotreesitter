//go:build gts_incr_census

package gotreesitter_test

import (
	"testing"

	gts "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
	"github.com/odvcencio/gotreesitter/internal/benchfixtures"
)

func TestIncrementalCensusTransparency(t *testing.T) {
	for _, name := range []string{"go", "javascript", "typescript", "python", "rust", "java", "c_sharp", "powershell"} {
		t.Run(name, func(t *testing.T) {
			var entry grammars.LangEntry
			for _, v := range grammars.AllLanguages() {
				if v.Name == name {
					entry = v
					break
				}
			}
			lang := entry.Language()
			src := []byte(grammars.ParseSmokeSample(name))
			parse := func(p *gts.Parser, source []byte, old *gts.Tree) (*gts.Tree, error) {
				if entry.TokenSourceFactory != nil {
					factory := func(s []byte) (gts.TokenSource, error) { return entry.TokenSourceFactory(s, lang), nil }
					if old != nil {
						return p.ParseIncrementalWithTokenSourceFactory(source, old, factory)
					}
					return p.ParseWithTokenSourceFactory(source, factory)
				}
				if old != nil {
					return p.ParseIncremental(source, old)
				}
				return p.Parse(source)
			}
			p := gts.NewParser(lang)
			old, err := parse(p, src, nil)
			if err != nil {
				t.Fatal(err)
			}
			defer old.Release()
			// The core API must return unchanged trees without entering an engine.
			// Factories may themselves allocate before their separate entry point sees
			// the tree; the real-corpus receipt records that pre-existing cost.
			allocs := testing.AllocsPerRun(20, func() {
				same, err := p.ParseIncremental(src, old)
				if err != nil {
					t.Fatal(err)
				}
				same.Release()
			})
			if allocs != 0 {
				t.Fatalf("no-edit allocated %.0f times", allocs)
			}
			edited := append(append([]byte(nil), src...), byte(' '))
			pt := old.RootNode().EndPoint()
			edit := gts.InputEdit{StartByte: uint32(len(src)), OldEndByte: uint32(len(src)), NewEndByte: uint32(len(src) + 1), StartPoint: pt, OldEndPoint: pt, NewEndPoint: gts.Point{Row: pt.Row, Column: pt.Column + 1}}
			old.Edit(edit)
			plain, err := parse(p, edited, old)
			if err != nil {
				t.Fatal(err)
			}
			defer plain.Release()
			plainDigest, err := benchfixtures.InspectGoTree(plain.RootNode(), lang)
			if err != nil {
				t.Fatal(err)
			}
			tracedOld, err := parse(p, src, nil)
			if err != nil {
				t.Fatal(err)
			}
			defer tracedOld.Release()
			traced, report, err := gts.DiagnosticObserveIncrementalReuse(tracedOld, func() (*gts.Tree, error) { tracedOld.Edit(edit); return parse(p, edited, tracedOld) })
			if err != nil {
				t.Fatal(err)
			}
			defer traced.Release()
			tracedDigest, err := benchfixtures.InspectGoTree(traced.RootNode(), lang)
			if err != nil {
				t.Fatal(err)
			}
			if plainDigest.SHA256 != tracedDigest.SHA256 {
				t.Fatal("observation changed tree")
			}
			a, b := plain.ParseRuntime(), traced.ParseRuntime()
			if a.TokensConsumed != b.TokensConsumed || a.NodesAllocated != b.NodesAllocated || a.Iterations != b.Iterations {
				t.Fatalf("observation changed work counters: %+v / %+v", a, b)
			}
			if report.OldNodes != report.ReusedNodes+report.LostNodes {
				t.Fatal("old node partition incomplete")
			}
			var nanos int64
			var lost uint64
			for _, row := range report.Rows {
				nanos += row.Nanos
				lost += row.LostNodes
			}
			if nanos+report.ObserveNanos != report.EditNanos || lost != report.LostNodes {
				t.Fatalf("invalid attribution: %+v", report)
			}
		})
	}
}
