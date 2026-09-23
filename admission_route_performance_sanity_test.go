package gotreesitter_test

import (
	"fmt"
	"sort"
	"strings"
	"testing"
	"time"

	gts "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
)

// TestAdmissionRoutePerformanceSanity is gotreesitter's CI performance gate.
//
// buildbox's tamarack harness measures the compact route against a same-host
// C tree-sitter oracle -- the authoritative, ground-truth comparison -- but
// that harness needs a C toolchain and prebuilt tree-sitter grammar objects
// that an ordinary CI runner does not carry, and cgo cross-process timing
// noise makes an oracle comparison a poor fit for a gate that must stay
// stable on a shared runner. So this gate does not touch the C oracle at
// all: it compares the compact ("candidate") route against the production
// route, both pure Go, interleaved in the SAME process on a fixed corpus.
// That relative, in-process comparison is what buildbox validated as stable
// under load: alternating routes iteration by iteration cancels most of a
// shared runner's noise, because both routes see approximately the same
// instantaneous system state, and buildbox measured that stability directly
// -- a relative in-process ratio held within 5% even at host load average
// 40 (see tam/run_bis.sh's interleaved production/candidate comparison).
//
// The tolerance below is deliberately far wider than what buildbox measured
// (the compact route ran 1.1x to 2.2x slower than production on most
// languages before lever 1 defaulted it off): this gate is a backstop
// against a GROSS regression (a future change making the compact route,
// say, 10x slower, or hanging), not a precision benchmark -- that job stays
// on buildbox's tamarack harness, run by hand against a real corpus, not in
// CI.
//
// Every language here has an external scanner or otherwise exercises a
// distinct lexing path (YAML and Markdown in particular exercise lever 3's
// lazy read-frontier tracking); a language whose compact route declines or
// falls back on its corpus sample is skipped rather than failed, since a
// fallback measures production against itself and asserting a ratio there
// proves nothing about the compact route.
func TestAdmissionRoutePerformanceSanity(t *testing.T) {
	if testing.Short() {
		t.Skip("performance sanity gate skipped under -short")
	}

	const (
		warmupIters   = 3
		measuredIters = 25
		// maxRatio is deliberately generous (see the doc comment above): a
		// true regression this gate is meant to catch is many times this,
		// not a fraction of it.
		maxRatio = 6.0
	)

	for _, tc := range admissionRoutePerformanceCorpus() {
		t.Run(tc.lang, func(t *testing.T) {
			lang := tc.language
			source := []byte(tc.source)

			prod := gts.NewParser(lang)
			prod.SetAdmissionCandidateRoute(false)
			cand := gts.NewParser(lang)
			cand.SetAdmissionCandidateRoute(true)

			gts.ResetAdmissionCandidateCountersForTest()
			warm, err := cand.Parse(source)
			if err != nil {
				t.Fatalf("candidate warm parse: %v", err)
			}
			warm.Release()
			if routed, fallback := gts.AdmissionCandidateCounters(); routed == 0 || fallback != 0 {
				t.Skipf("candidate route did not cleanly route this corpus sample (routed=%d fallback=%d reason=%q); skipping, a fallback would only measure production against itself",
					routed, fallback, gts.AdmissionCandidateLastFallbackReason())
			}

			// Interleave every measured iteration (production, then
			// candidate) instead of running two separate blocks: this is
			// what cancels a shared runner's transient load, thermal
			// throttling, or GC-phase noise between the two sides, since
			// each pair sees approximately the same instantaneous system
			// state. Warm up both routes first so neither pays a one-time
			// first-parse cost the other already absorbed.
			for i := 0; i < warmupIters; i++ {
				if tree, err := prod.Parse(source); err != nil {
					t.Fatalf("production warmup: %v", err)
				} else {
					tree.Release()
				}
				if tree, err := cand.Parse(source); err != nil {
					t.Fatalf("candidate warmup: %v", err)
				} else {
					tree.Release()
				}
			}

			ratios := make([]float64, 0, measuredIters)
			for i := 0; i < measuredIters; i++ {
				prodStart := time.Now()
				prodTree, err := prod.Parse(source)
				prodElapsed := time.Since(prodStart)
				if err != nil {
					t.Fatalf("production parse: %v", err)
				}
				prodTree.Release()

				candStart := time.Now()
				candTree, err := cand.Parse(source)
				candElapsed := time.Since(candStart)
				if err != nil {
					t.Fatalf("candidate parse: %v", err)
				}
				candTree.Release()

				if prodElapsed <= 0 {
					continue
				}
				ratios = append(ratios, float64(candElapsed)/float64(prodElapsed))
			}
			if len(ratios) == 0 {
				t.Fatal("every production iteration measured zero elapsed time; corpus sample is too small to time reliably")
			}

			ratio := medianFloat64(ratios)
			t.Logf("%s: candidate/production median ratio = %.3f over %d interleaved iterations", tc.lang, ratio, len(ratios))
			if ratio > maxRatio {
				t.Fatalf("candidate/production median ratio = %.3f, want <= %.1f (gross performance regression on the compact route)", ratio, maxRatio)
			}
		})
	}
}

func medianFloat64(xs []float64) float64 {
	ys := append([]float64(nil), xs...)
	sort.Float64s(ys)
	n := len(ys)
	if n%2 == 1 {
		return ys[n/2]
	}
	return (ys[n/2-1] + ys[n/2]) / 2
}

type admissionRoutePerformanceCase struct {
	lang     string
	language *gts.Language
	source   string
}

// admissionRoutePerformanceCorpus builds a fixed, self-contained,
// typical-file-sized (a few KiB) sample per language, covering every
// language buildbox's tamarack harness measured: Go, Python, TypeScript,
// Rust, YAML, Bash, Markdown, Lua, and CSS.
func admissionRoutePerformanceCorpus() []admissionRoutePerformanceCase {
	const repeat = 60
	return []admissionRoutePerformanceCase{
		{"go", grammars.GoLanguage(), repeatBlock(repeat, func(i int) string {
			return fmt.Sprintf("func handler%d(x int) (int, error) {\n\tif x < 0 {\n\t\treturn 0, fmt.Errorf(\"negative: %%d\", x)\n\t}\n\treturn x * %d, nil\n}\n\n", i, i)
		}, "package sample\n\nimport \"fmt\"\n\n")},
		{"python", grammars.PythonLanguage(), repeatBlock(repeat, func(i int) string {
			return fmt.Sprintf("def handler_%d(x):\n    if x < 0:\n        raise ValueError(f\"negative: {x}\")\n    return x * %d\n\n", i, i)
		}, "")},
		{"typescript", grammars.TypescriptLanguage(), repeatBlock(repeat, func(i int) string {
			return fmt.Sprintf("export function handler%d(x: number): number {\n  if (x < 0) {\n    throw new Error(`negative: ${x}`);\n  }\n  return x * %d;\n}\n\n", i, i)
		}, "")},
		{"rust", grammars.RustLanguage(), repeatBlock(repeat, func(i int) string {
			return fmt.Sprintf("fn handler_%d(x: i64) -> Result<i64, String> {\n    if x < 0 {\n        return Err(format!(\"negative: {}\", x));\n    }\n    Ok(x * %d)\n}\n\n", i, i)
		}, "")},
		{"yaml", grammars.YamlLanguage(), repeatBlock(repeat, func(i int) string {
			return fmt.Sprintf("handler_%d:\n  weight: %d\n  tags:\n    - alpha\n    - beta\n  enabled: true\n", i, i)
		}, "")},
		{"bash", grammars.BashLanguage(), repeatBlock(repeat, func(i int) string {
			return fmt.Sprintf("handler_%d() {\n  local x=\"$1\"\n  if [ \"$x\" -lt 0 ]; then\n    echo \"negative: $x\" >&2\n    return 1\n  fi\n  echo $((x * %d))\n}\n\n", i, i)
		}, "#!/usr/bin/env bash\nset -euo pipefail\n\n")},
		{"markdown", grammars.MarkdownLanguage(), repeatBlock(repeat, func(i int) string {
			return fmt.Sprintf("## Section %d\n\nThis section covers handler %d, its inputs, and its *typical* output.\n\n- input: `x`\n- output: `x * %d`\n\n", i, i, i)
		}, "# Sample Document\n\n")},
		{"lua", grammars.LuaLanguage(), repeatBlock(repeat, func(i int) string {
			return fmt.Sprintf("function handler_%d(x)\n  if x < 0 then\n    error(\"negative: \" .. x)\n  end\n  return x * %d\nend\n\n", i, i)
		}, "")},
		{"css", grammars.CssLanguage(), repeatBlock(repeat, func(i int) string {
			return fmt.Sprintf(".handler-%d {\n  display: flex;\n  margin: %dpx;\n  color: #%06x;\n}\n\n", i, i%20, (i*257)%0xFFFFFF)
		}, "")},
	}
}

// repeatBlock builds a typical-file-sized source by prefixing header and
// concatenating n distinct (not identical) blocks, so the corpus is large
// enough to time reliably without depending on an external fixture file.
func repeatBlock(n int, block func(i int) string, header string) string {
	var b strings.Builder
	b.WriteString(header)
	for i := 0; i < n; i++ {
		b.WriteString(block(i))
	}
	return b.String()
}
