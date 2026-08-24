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
//     campaign/julia-trailing-comma-defect.md §4 and shipped as testdata under
//     campaign/fixtures/julia/): production output must equal raw output on
//     every witness now that no julia compat producer remains;
//   - the trailing-comma defect witness (`x, = 1\n`): its raw dump digest is
//     pinned so the known raw-vs-C divergence documented in
//     campaign/julia-trailing-comma-defect.md stays observable;
//   - a post-deletion firing census over that same fixture corpus asserting
//     no `dispatch.julia` normalization pass is ever recorded again.
//
// Any future reintroduction of a julia rewrite — or a grammar revision that
// moves these shapes — fails loudly here and reopens the filed defect.

import (
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	gotreesitter "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
)

const (
	juliaRetiredActiveWitnessDigest = "sha256:9e2de3670281c5a0013d55b570e6c175c7d7eb2d5142bca99276974ea61453e2"
	juliaRetiredDefectWitnessDigest = "sha256:f6780ebcc18b9638afb915c133bc5d627bff6abdbe0570d8e6c783035aa47d03"
)

// juliaFixtureDir is the committed regression corpus: the ACTIVE firing
// witness plus the pre-deletion firing-census sweep witnesses, shipped as
// testdata under campaign/fixtures/julia/ so the retirement receipts stay
// re-runnable against real files.
const juliaFixtureDir = "campaign/fixtures/julia"

// juliaCensusWitnessFiles lists the canonical 24-witness pre-deletion firing
// census corpus in census order (campaign/fixtures/julia/FIRING-CENSUS.md,
// campaign/julia-trailing-comma-defect.md §4). The ACTIVE firing witness is
// inline-recovered-return-range.
var juliaCensusWitnessFiles = []string{
	"inline-clean-program",
	"inline-recovered-return-range",
	"inline-bracket-comprehension",
	"inline-subscript-single-row",
	"inline-macro-juxtaposition",
	"inline-trailing-comma-tuple",
	"inline-broken",
	"julia_utils",
	"inline-subscript-binary-index",
	"inline-macro-string-arg",
	"inline-bare-return-range",
	"inline-clean-for-loop",
	"inline-clean-struct",
	"inline-unbalanced-paren",
	"inline-string-interpolation",
	"inline-ternary",
	"inline-chained-comparison",
	"inline-while-break",
	"inline-local-func",
	"inline-comment-only",
	"inline-try-catch",
	"inline-nested-module-error",
	"inline-trailing-comma-newline-rhs",
	"inline-recovery-missing-end",
}

type juliaRetirementWitness struct {
	name   string
	source []byte
}

func juliaReadFixture(t *testing.T, base string) []byte {
	t.Helper()
	src, err := os.ReadFile(filepath.Join(juliaFixtureDir, base+".jl"))
	if err != nil {
		t.Fatalf("read corpus fixture %s: %v", base, err)
	}
	return src
}

func juliaRetirementCorpus(t *testing.T) []juliaRetirementWitness {
	t.Helper()
	witnesses := make([]juliaRetirementWitness, 0, len(juliaCensusWitnessFiles))
	for _, base := range juliaCensusWitnessFiles {
		name := base
		if base == "julia_utils" {
			name = "julia_utils.jl"
		}
		witnesses = append(witnesses, juliaRetirementWitness{name: name, source: juliaReadFixture(t, base)})
	}
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

func juliaParseRaw(t *testing.T, lang *gotreesitter.Language, source []byte) *gotreesitter.Tree {
	t.Helper()
	raw, err := gotreesitter.NewParser(lang).ParseNoResultCompatibilityBenchmarkOnly(source)
	if err != nil {
		t.Fatalf("raw parse failed: %v", err)
	}
	return raw
}

func TestJuliaRetirementRawMatchesLockedCOracleOnFiringWitness(t *testing.T) {
	lang := grammars.JuliaLanguage()
	source := []byte("function f()\n    return 1:(2 + 3)\nend\n")
	raw := juliaParseRaw(t, lang, source)
	defer raw.Release()
	if got := juliaDumpDigest(t, raw.RootNode(), lang, source); got != juliaRetiredActiveWitnessDigest {
		t.Fatalf("firing witness raw digest = %s, want locked-C digest %s; the grammar revision moved the recovered return-range shape and campaign/julia-decision.md must be re-decided", got, juliaRetiredActiveWitnessDigest)
	}
}

func TestJuliaRetirementTrailingCommaDefectWitnessPinned(t *testing.T) {
	lang := grammars.JuliaLanguage()
	source := []byte("x, = 1\n")
	raw := juliaParseRaw(t, lang, source)
	defer raw.Release()
	root := raw.RootNode()
	if got := root.HasError(); !got {
		t.Fatal("trailing-comma defect witness lost its error flag; campaign/julia-trailing-comma-defect.md reopen condition 1 fires")
	}
	if got := juliaDumpDigest(t, root, lang, source); got != juliaRetiredDefectWitnessDigest {
		t.Fatalf("trailing-comma defect witness raw digest = %s, want pinned %s; see campaign/julia-trailing-comma-defect.md reopen conditions", got, juliaRetiredDefectWitnessDigest)
	}
}

func juliaAssertProductionEqualsRaw(t *testing.T, lang *gotreesitter.Language, w juliaRetirementWitness) {
	t.Helper()
	prod, err := gotreesitter.NewParser(lang).Parse(w.source)
	if err != nil {
		t.Fatalf("production parse failed: %v", err)
	}
	defer prod.Release()
	raw := juliaParseRaw(t, lang, w.source)
	defer raw.Release()
	prodRoot, rawRoot := prod.RootNode(), raw.RootNode()
	if prodRoot.HasError() != rawRoot.HasError() {
		t.Fatalf("root error flag diverges: production=%v raw=%v", prodRoot.HasError(), rawRoot.HasError())
	}
	if got, want := juliaDumpDigest(t, prodRoot, lang, w.source), juliaDumpDigest(t, rawRoot, lang, w.source); got != want {
		t.Fatalf("production tree differs from raw tree on retired-arm witness:\n production=%s\n raw      =%s", got, want)
	}
}

func TestJuliaRetirementProductionEqualsRawOverCorpus(t *testing.T) {
	lang := grammars.JuliaLanguage()
	for _, w := range juliaRetirementCorpus(t) {
		w := w
		t.Run(w.name, func(t *testing.T) {
			juliaAssertProductionEqualsRaw(t, lang, w)
		})
	}
}

// TestJuliaRetirementFiringCensusPostDeletion runs a firing census over the
// committed fixture corpus AFTER the deletion (the pre-deletion census over
// these same witnesses is recorded in campaign/julia-trailing-comma-defect.md
// §4 and campaign/fixtures/julia/FIRING-CENSUS.md): with the producer deleted,
// no parse may record a `dispatch.julia` normalization pass again.
func TestJuliaRetirementFiringCensusPostDeletion(t *testing.T) {
	t.Setenv("GTS_DISPATCHER_CENSUS", "1")
	lang := grammars.JuliaLanguage()
	for _, w := range juliaRetirementCorpus(t) {
		w := w
		t.Run(w.name, func(t *testing.T) {
			tree, err := gotreesitter.NewParser(lang).Parse(w.source)
			if err != nil {
				t.Fatalf("production parse failed: %v", err)
			}
			defer tree.Release()
			rt := tree.ParseRuntime()
			if rt.NormalizationPasses != nil {
				for _, pass := range *rt.NormalizationPasses {
					if pass.Name == "dispatch.julia" {
						t.Fatalf("post-retirement firing: dispatch.julia pass recorded on witness %s (checked=%d run=%d visited=%d rewritten=%d); a julia producer was reintroduced without a new receipt",
							w.name, pass.Checked, pass.Run, pass.NodesVisited, pass.NodesRewritten)
					}
				}
			}
			t.Logf("witness=%s bytes=%d status=NO-PASS-RECORD prod_root=%s hasError=%v",
				w.name, len(w.source), tree.RootNode().Type(lang), tree.RootNode().HasError())
		})
	}
}

// TestJuliaRetirementFixtureCorpusIsComplete pins the shipped corpus itself:
// campaign/fixtures/julia must contain exactly the 24 census witnesses plus
// the one extra recovery variant, and every shipped witness must satisfy
// production==raw parity.
func TestJuliaRetirementFixtureCorpusIsComplete(t *testing.T) {
	entries, err := os.ReadDir(juliaFixtureDir)
	if err != nil {
		t.Fatalf("read fixture dir: %v", err)
	}
	want := map[string]bool{"inline-assignment-tuple-multi": true}
	for _, base := range juliaCensusWitnessFiles {
		want[base] = true
	}
	got := map[string]bool{}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".jl") {
			continue
		}
		base := strings.TrimSuffix(e.Name(), ".jl")
		if !want[base] {
			t.Errorf("unexpected fixture %s/%s not covered by the census corpus", juliaFixtureDir, e.Name())
			continue
		}
		got[base] = true
	}
	for base := range want {
		if !got[base] {
			t.Errorf("missing fixture %s/%s.jl", juliaFixtureDir, base)
		}
	}
	if len(got) != len(want) {
		t.Fatalf("fixture corpus size = %d, want %d", len(got), len(want))
	}
	lang := grammars.JuliaLanguage()
	for _, base := range juliaCensusWitnessFiles {
		name := base
		if base == "julia_utils" {
			name = "julia_utils.jl"
		}
		juliaAssertProductionEqualsRaw(t, lang, juliaRetirementWitness{name: name, source: juliaReadFixture(t, base)})
	}
	juliaAssertProductionEqualsRaw(t, lang, juliaRetirementWitness{name: "inline-assignment-tuple-multi", source: juliaReadFixture(t, "inline-assignment-tuple-multi")})
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
