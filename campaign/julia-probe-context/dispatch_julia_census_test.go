// Probe census for dispatcher arm `dispatch.julia` (R2, docs/root-normalization-retirement.md).
//
// Cheapest-probe design: run the production parser over a small set of Julia
// witnesses (repo-shipped .jl file plus inline malformed sources that target
// each registered local repair in parser_result_julia.go) under
// GTS_DISPATCHER_CENSUS=1 and read the `dispatch.julia` pass receipt from
// tree.ParseRuntime().NormalizationPasses. Also prints raw
// (ParseNoResultCompatibilityBenchmarkOnly) vs production root shape per
// witness so "the arm ran but changed nothing" and "the arm rewrote" are both
// observable.
package juliaprobe

import (
	"os"
	"testing"

	gotreesitter "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
)

func dispatchJuliaReceipt(t *testing.T, tree *gotreesitter.Tree) (checked, run, visited, rewritten uint64) {
	t.Helper()
	rt := tree.ParseRuntime()
	if rt.NormalizationPasses == nil {
		return 0, 0, 0, 0
	}
	for _, pass := range *rt.NormalizationPasses {
		if pass.Name == "dispatch.julia" {
			return pass.Checked, pass.Run, pass.NodesVisited, pass.NodesRewritten
		}
	}
	return 0, 0, 0, 0
}

func TestDispatchJuliaCensusProbe(t *testing.T) {
	t.Setenv("GTS_DISPATCHER_CENSUS", "1")

	type witness struct {
		name   string
		source []byte
	}
	witnesses := []witness{
		// Clean program: arm should be inert.
		{name: "inline-clean-program", source: []byte("module M\nexport f\nf(x) = x + 1\nend\n")},
		// normalizeJuliaRecoveredReturnRange: `return a:b(c+d)` recovered range.
		{name: "inline-recovered-return-range", source: []byte("function f()\n    return 1:(2 + 3)\nend\n")},
		// normalizeJuliaBracketForComprehensions: `[expr for it in coll]` parsed as matrix.
		{name: "inline-bracket-comprehension", source: []byte("[x for x in xs]\n")},
		// normalizeJuliaSubscriptSingleRowMatrix: `a[i]` parsed as single-row matrix.
		{name: "inline-subscript-single-row", source: []byte("a = [1]\nb = a[1]\n")},
		// normalizeJuliaMacroArgumentJuxtaposition: macro args `1 2` / `1 "s"`.
		{name: "inline-macro-juxtaposition", source: []byte("@nbits 1 2\n")},
		// normalizeJuliaTrailingCommaAssignmentTuple: `x, = 1`.
		{name: "inline-trailing-comma-tuple", source: []byte("x, = 1\n")},
		// Error-bearing recovered source.
		{name: "inline-broken", source: []byte("function f(\n  return 1:(2 + 3)\nend\n") },
	}
	for _, p := range []string{"../../testdata/compact_selected_lineage/julia_utils.jl"} {
		src, err := os.ReadFile(p)
		if err != nil {
			t.Fatalf("read %s: %v", p, err)
		}
		witnesses = append(witnesses, witness{name: p, source: src})
	}

	lang := grammars.JuliaLanguage()
	for _, w := range witnesses {
		w := w
		t.Run(w.name, func(t *testing.T) {
			parser := gotreesitter.NewParser(lang)
			tree, err := parser.Parse(w.source)
			if err != nil {
				t.Fatalf("production parse failed: %v", err)
			}
			t.Cleanup(tree.Release)

			raw, err := gotreesitter.NewParser(lang).ParseNoResultCompatibilityBenchmarkOnly(w.source)
			if err != nil {
				t.Fatalf("raw parse failed: %v", err)
			}
			t.Cleanup(raw.Release)

			checked, run, visited, rewritten := dispatchJuliaReceipt(t, tree)
			root := tree.RootNode()
			rawRoot := raw.RootNode()
			t.Logf("witness=%s bytes=%d", w.name, len(w.source))
			t.Logf("raw_root=%s hasError=%v children=%d", rawRoot.Type(lang), rawRoot.HasError(), rawRoot.ChildCount())
			t.Logf("prod_root=%s hasError=%v children=%d", root.Type(lang), root.HasError(), root.ChildCount())
			t.Logf("dispatch.julia receipt: checked=%d run=%d visited=%d rewritten=%d", checked, run, visited, rewritten)
			if checked == 0 && run == 0 {
				t.Logf("NOTE: no dispatch.julia pass record on this witness (parse likely took a route that did not record the arm; receipt-only probe, not a gate)")
			}
			if rewritten > 0 {
				t.Logf("ACTIVE: arm rewrote %d node(s)", rewritten)
			} else {
				t.Logf("INERT-SO-FAR: arm rewrote nothing on this witness")
			}
		})
	}
}
