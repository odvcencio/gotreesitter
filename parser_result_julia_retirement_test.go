package gotreesitter_test

// Retirement receipt for the `dispatch.julia` dispatcher arm (Decision 0007,
// strict locked-C parity; campaign/julia-decision.md). The arm and its five
// sub-repairs were deleted after the locked C oracle matched the raw Go parse
// on the ACTIVE witness while production diverged. This file pins:
//
//   - the ACTIVE firing witness (`inline-recovered-return-range`, 38 bytes):
//     the raw-Go normalized dump digest must stay equal to the locked-C oracle
//     digest recorded in campaign/julia-decision.md,
//     sha256:9e2de3670281c5a0013d55b570e6c175c7d7eb2d5142bca99276974ea61453e2;
//   - the 24-witness corpus from the pre-deletion firing census (recorded in
//     campaign/julia-trailing-comma-defect.md): production output must equal
//     raw output on every witness now that no julia compat producer remains;
//   - the trailing-comma defect witness (`x, = 1\n`): its raw dump digest is
//     pinned so the known raw-vs-C divergence documented in
//     campaign/julia-trailing-comma-defect.md stays observable.
//
// Any future reintroduction of a julia rewrite — or a grammar revision that
// moves these shapes — fails loudly here and reopens the filed defect.

import (
	"crypto/sha256"
	"fmt"
	"os"
	"strings"
	"testing"

	gotreesitter "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
)

const (
	juliaRetiredActiveWitnessDigest = "sha256:9e2de3670281c5a0013d55b570e6c175c7d7eb2d5142bca99276974ea61453e2"
	juliaRetiredDefectWitnessDigest = "sha256:f6780ebcc18b9638afb915c133bc5d627bff6abdbe0570d8e6c783035aa47d03"
)

type juliaRetirementWitness struct {
	name   string
	source []byte
}

func juliaRetirementCorpus(t *testing.T) []juliaRetirementWitness {
	t.Helper()
	witnesses := []juliaRetirementWitness{
		{name: "inline-clean-program", source: []byte("module M\nexport f\nf(x) = x + 1\nend\n")},
		{name: "inline-recovered-return-range", source: []byte("function f()\n    return 1:(2 + 3)\nend\n")},
		{name: "inline-bracket-comprehension", source: []byte("[x for x in xs]\n")},
		{name: "inline-subscript-single-row", source: []byte("a = [1]\nb = a[1]\n")},
		{name: "inline-macro-juxtaposition", source: []byte("@nbits 1 2\n")},
		{name: "inline-trailing-comma-tuple", source: []byte("x, = 1\n")},
		{name: "inline-broken", source: []byte("function f(\n  return 1:(2 + 3)\nend\n")},
		{name: "inline-assignment-tuple-multi", source: []byte("x, y = 1, 2\n")},
		{name: "inline-subscript-binary-index", source: []byte("a = [1, 2]\nb = a[1 + 1]\n")},
		{name: "inline-macro-string-arg", source: []byte("@show \"s\"\n")},
		{name: "inline-bare-return-range", source: []byte("function f()\n    return 1:2\nend\n")},
		{name: "inline-clean-for-loop", source: []byte("for i in 1:3\n    println(i)\nend\n")},
		{name: "inline-clean-struct", source: []byte("struct P\n    x::Int\nend\n")},
		{name: "inline-unbalanced-paren", source: []byte("f(x\n")},
		{name: "inline-string-interpolation", source: []byte("s = \"a$(b)c\"\n")},
		{name: "inline-ternary", source: []byte("y = x > 0 ? 1 : -1\n")},
		{name: "inline-chained-comparison", source: []byte("ok = 1 < x < 10\n")},
		{name: "inline-while-break", source: []byte("while true\n    break\nend\n")},
		{name: "inline-local-func", source: []byte("g(x) = x^2 + 1\n")},
		{name: "inline-comment-only", source: []byte("# just a comment\n")},
		{name: "inline-try-catch", source: []byte("try\n    error(\"x\")\ncatch e\n    show(e)\nend\n")},
		{name: "inline-nested-module-error", source: []byte("module M\nusing X.\nend\n")},
		{name: "inline-trailing-comma-newline-rhs", source: []byte("x,\n= 1\n")},
		{name: "inline-recovery-missing-end", source: []byte("function f()\n    return 1\n")},
	}
	src, err := os.ReadFile("testdata/compact_selected_lineage/julia_utils.jl")
	if err != nil {
		t.Fatalf("read corpus fixture: %v", err)
	}
	witnesses[7] = juliaRetirementWitness{name: "testdata/compact_selected_lineage/julia_utils.jl", source: src}
	if len(witnesses) != 24 {
		t.Fatalf("corpus size = %d, want 24", len(witnesses))
	}
	return witnesses
}

// juliaNormalizedDump mirrors the locked-C three-way dump format used by
// cgo_harness TestJuliaDecisionThreeWayDump: lines carry a "G " marker that is
// normalized away by replacing "G " with "X" (dropping the space), so the
// digests below are directly comparable with campaign/julia-decision.md.
func juliaNormalizedDump(n *gotreesitter.Node, lang *gotreesitter.Language, src []byte, depth int, b *strings.Builder) {
	txt := ""
	s, e := n.StartByte(), n.EndByte()
	if int(e) <= len(src) {
		raw := string(src[s:e])
		if len(raw) > 60 {
			raw = raw[:60]
		}
		txt = fmt.Sprintf("%q", raw)
	}
	fmt.Fprintf(b, "G %s%s [%d:%d] named=%v missing=%v extra=%v cc=%d %s\n",
		strings.Repeat("  ", depth), n.Type(lang), s, e, n.IsNamed(), n.IsMissing(), n.IsExtra(), n.ChildCount(), txt)
	for i := 0; i < n.ChildCount(); i++ {
		juliaNormalizedDump(n.Child(i), lang, src, depth+1, b)
	}
}

func juliaDumpDigest(t *testing.T, root *gotreesitter.Node, lang *gotreesitter.Language, src []byte) string {
	t.Helper()
	var b strings.Builder
	juliaNormalizedDump(root, lang, src, 0, &b)
	return fmt.Sprintf("sha256:%x", sha256.Sum256([]byte(strings.ReplaceAll(b.String(), "G ", "X"))))
}

func TestJuliaRetirementRawMatchesLockedCOracleOnFiringWitness(t *testing.T) {
	lang := grammars.JuliaLanguage()
	source := []byte("function f()\n    return 1:(2 + 3)\nend\n")
	raw, err := gotreesitter.NewParser(lang).ParseNoResultCompatibilityBenchmarkOnly(source)
	if err != nil {
		t.Fatalf("raw parse failed: %v", err)
	}
	defer raw.Release()
	if got := juliaDumpDigest(t, raw.RootNode(), lang, source); got != juliaRetiredActiveWitnessDigest {
		t.Fatalf("firing witness raw digest = %s, want locked-C digest %s; the grammar revision moved the recovered return-range shape and campaign/julia-decision.md must be re-decided", got, juliaRetiredActiveWitnessDigest)
	}
}

func TestJuliaRetirementTrailingCommaDefectWitnessPinned(t *testing.T) {
	lang := grammars.JuliaLanguage()
	source := []byte("x, = 1\n")
	raw, err := gotreesitter.NewParser(lang).ParseNoResultCompatibilityBenchmarkOnly(source)
	if err != nil {
		t.Fatalf("raw parse failed: %v", err)
	}
	defer raw.Release()
	root := raw.RootNode()
	if got := root.HasError(); !got {
		t.Fatal("trailing-comma defect witness lost its error flag; campaign/julia-trailing-comma-defect.md reopen condition 1 fires")
	}
	if got := juliaDumpDigest(t, root, lang, source); got != juliaRetiredDefectWitnessDigest {
		t.Fatalf("trailing-comma defect witness raw digest = %s, want pinned %s; see campaign/julia-trailing-comma-defect.md reopen conditions", got, juliaRetiredDefectWitnessDigest)
	}
}

func TestJuliaRetirementProductionEqualsRawOverCorpus(t *testing.T) {
	lang := grammars.JuliaLanguage()
	for _, w := range juliaRetirementCorpus(t) {
		w := w
		t.Run(w.name, func(t *testing.T) {
			prod, err := gotreesitter.NewParser(lang).Parse(w.source)
			if err != nil {
				t.Fatalf("production parse failed: %v", err)
			}
			defer prod.Release()
			raw, err := gotreesitter.NewParser(lang).ParseNoResultCompatibilityBenchmarkOnly(w.source)
			if err != nil {
				t.Fatalf("raw parse failed: %v", err)
			}
			defer raw.Release()
			prodRoot, rawRoot := prod.RootNode(), raw.RootNode()
			if prodRoot.HasError() != rawRoot.HasError() {
				t.Fatalf("root error flag diverges: production=%v raw=%v", prodRoot.HasError(), rawRoot.HasError())
			}
			if got, want := juliaDumpDigest(t, prodRoot, lang, w.source), juliaDumpDigest(t, rawRoot, lang, w.source); got != want {
				t.Fatalf("production tree differs from raw tree on retired-arm witness:\n production=%s\n raw      =%s", got, want)
			}
		})
	}
}

// findNodeByText was previously defined in parser_result_julia_test.go (deleted
// with the retired arm's behavior tests) and is still used by the kotlin real
// corpus tests.
func findNodeByText(root *gotreesitter.Node, lang *gotreesitter.Language, source []byte, typ, text string) *gotreesitter.Node {
	if root == nil {
		return nil
	}
	if root.Type(lang) == typ && root.Text(source) == text {
		return root
	}
	for i := 0; i < root.ChildCount(); i++ {
		if found := findNodeByText(root.Child(i), lang, source, typ, text); found != nil {
			return found
		}
	}
	return nil
}
