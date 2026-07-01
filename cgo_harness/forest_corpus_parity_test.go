package cgoharness

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	gts "github.com/odvcencio/gotreesitter"
	grm "github.com/odvcencio/gotreesitter/grammars"
)

// TestForestCorpusParity is the correctness gate for promoting a language onto
// the GSS-forest fast path (Parser.tryForestFastPath / builtinForestDefaults).
//
// The runtime dispatch falls back to the production parser whenever the forest
// declines (failure / error node / truncation), so it can never regress those
// cases. The ONE risk it cannot catch is a forest that produces a clean,
// complete, but STRUCTURALLY DIFFERENT tree — a silent divergence. This test
// closes that gap: for every real-corpus file it parses with the production
// parser and with the forest, and on every file the forest would dispatch
// (clean + complete) it asserts byte-identical s-expressions. Any divergence
// fails the test and blocks default-on for that language.
//
// Languages come from GTS_FOREST_LANGS (comma-separated; default "bash") so a
// candidate (swift, fortran, ...) can be vetted before it is added to the
// runtime allowlist. It also reports dispatch rate and wall speedup, which is
// the "wall" half of "full parity wall and correctness".
//
// Run heavy (real corpus) under Docker per the repo's testing discipline:
//
//	cgo_harness/docker/run_forest_corpus_parity.sh
func TestForestCorpusParity(t *testing.T) {
	// Opt-in: this is a heavy real-corpus gate that currently FAILS by design
	// while the forest is pre-parity (it reports the divergences blocking
	// default-on). Keep it out of the default `go test ./...` run; the Docker
	// runner and manual promotion checks set GTS_FOREST_CORPUS=1.
	if strings.TrimSpace(os.Getenv("GTS_FOREST_CORPUS")) == "" {
		t.Skip("set GTS_FOREST_CORPUS=1 to run the forest real-corpus parity gate")
	}
	langs := strings.Split(envOr("GTS_FOREST_LANGS", "bash"), ",")
	loaders := forestLanguageLoaders()
	repoRoot := forestRepoRoot(t)

	anyRun := false
	for _, raw := range langs {
		name := strings.TrimSpace(raw)
		if name == "" {
			continue
		}
		load, ok := loaders[name]
		if !ok {
			t.Errorf("%s: unknown language (not in grammars.AllLanguages)", name)
			continue
		}
		dir := forestCorpusDir(repoRoot, name)
		if dir == "" {
			t.Logf("%s: no corpus_real/%s directory — skipping", name, name)
			continue
		}
		files := forestCorpusFiles(t, dir)
		if len(files) == 0 {
			t.Logf("%s: corpus_real/%s empty — skipping", name, name)
			continue
		}
		anyRun = true
		lang := load()
		runForestLangParity(t, name, lang, files)
	}
	if !anyRun {
		t.Skip("no forest corpus available for requested languages")
	}
}

func runForestLangParity(t *testing.T, name string, lang *gts.Language, files []string) {
	t.Helper()
	gts.ResetPerfCounters()
	var (
		total, dispatched, fellBack, diverged int
		prodNanos, forestNanos                int64
		divergedFiles                         []string
		fallbackReasons                       = map[string]int{}
		verboseFallbacks                      = strings.TrimSpace(os.Getenv("GTS_FOREST_CORPUS_VERBOSE")) != ""
	)
	for _, f := range files {
		func(f string) {
			src, err := os.ReadFile(f)
			if err != nil {
				t.Errorf("%s: read %s: %v", name, f, err)
				return
			}
			total++

			// Production baseline (forest off).
			gts.SetGLRForestEnabled(false)
			st := time.Now()
			prodTree, _ := gts.NewParser(lang).Parse(src)
			prodNanos += time.Since(st).Nanoseconds()
			var prodRoot *gts.Node
			if prodTree != nil {
				prodRoot = prodTree.RootNode()
			}
			if prodRoot == nil {
				if prodTree != nil {
					prodTree.Release()
				}
				t.Errorf("%s: production produced no tree for %s", name, filepath.Base(f))
				return
			}
			defer prodTree.Release()
			want := rangedRepr(prodRoot, lang)

			// Forest result + the dispatch acceptance criteria (mirrors
			// tryForestFastPath: clean, no error node, reaches the last
			// non-whitespace byte).
			st = time.Now()
			forestTree, ok := gts.NewParser(lang).ParseForestExperimental(src)
			forestNanos += time.Since(st).Nanoseconds()
			if forestTree != nil {
				defer forestTree.Release()
			}
			var root *gts.Node
			if forestTree != nil {
				root = forestTree.RootNode()
			}
			if !ok || root == nil || root.HasError() || root.EndByte() < lastNonWSByte(src) {
				fellBack++
				reason := forestFallbackReason(ok, root, src)
				fallbackReasons[reason]++
				if verboseFallbacks {
					endByte := uint32(0)
					hasError := false
					if root != nil {
						endByte = root.EndByte()
						hasError = root.HasError()
					}
					t.Logf("%s: forest fallback %s reason=%s ok=%v root=%v hasError=%v end=%d lastNonWS=%d size=%d",
						name, filepath.Base(f), reason, ok, root != nil, hasError, endByte, lastNonWSByte(src), len(src))
					t.Logf("%s: production fallback peer %s hasError=%v end=%d",
						name, filepath.Base(f), prodRoot.HasError(), prodRoot.EndByte())
				}
				return
			}
			dispatched++
			if got := rangedRepr(root, lang); got != want {
				diverged++
				divergedFiles = append(divergedFiles, filepath.Base(f))
			}
		}(f)
	}

	speedup := 0.0
	if forestNanos > 0 {
		speedup = float64(prodNanos) / float64(forestNanos)
	}
	t.Logf("%-8s files=%d dispatched=%d fellback=%d diverged=%d | prod=%.1fms forest=%.1fms speedup=%.1fx",
		name, total, dispatched, fellBack, diverged,
		float64(prodNanos)/1e6, float64(forestNanos)/1e6, speedup)
	if fellBack > 0 {
		t.Logf("%-8s fallback reasons: %s", name, formatFallbackReasons(fallbackReasons))
	}
	if perf := gts.PerfCountersSnapshot(); perf.ForestReduceCalls > 0 || perf.ForestCoalesceCalls > 0 {
		t.Logf("%-8s forest perf: reduce calls=%d zero=%d linear=%d dfs=%d dfs_links=%d dfs_multilink=%d dfs_extra=%d dfs_visits=%d dfs_path_entries=%d max_path=%d max_child=%d goto_hit=%d goto_miss=%d coalesce calls=%d new=%d append=%d dedup=%d cap_drop=%d cap_replace=%d precap_drop=%d",
			name,
			perf.ForestReduceCalls,
			perf.ForestReduceZero,
			perf.ForestReduceLinearNoExtras,
			perf.ForestReduceDFS,
			perf.ForestReduceDFSLinks,
			perf.ForestReduceDFSMultiLinkSteps,
			perf.ForestReduceDFSExtraLinks,
			perf.ForestReduceDFSVisits,
			perf.ForestReduceDFSPathEntries,
			perf.ForestReduceMaxPathLen,
			perf.ForestReduceMaxChildCount,
			perf.ForestReduceGotoHits,
			perf.ForestReduceGotoMisses,
			perf.ForestCoalesceCalls,
			perf.ForestCoalesceNewNodes,
			perf.ForestCoalesceLinkAppends,
			perf.ForestCoalesceDedupHits,
			perf.ForestCoalesceCapDrops,
			perf.ForestCoalesceCapReplacements,
			perf.ForestCoalescePreCapDrops)
	}
	if diverged > 0 {
		sort.Strings(divergedFiles)
		t.Errorf("%s: %d/%d dispatched files DIVERGED from production (blocks forest default-on): %s",
			name, diverged, dispatched, strings.Join(divergedFiles, ", "))
	}
}

func forestFallbackReason(ok bool, root *gts.Node, src []byte) string {
	switch {
	case !ok:
		return "parse_failed"
	case root == nil:
		return "nil_root"
	case root.HasError():
		return "has_error"
	case root.EndByte() < lastNonWSByte(src):
		return "truncated"
	default:
		return "unknown"
	}
}

func formatFallbackReasons(reasons map[string]int) string {
	if len(reasons) == 0 {
		return "none"
	}
	keys := make([]string, 0, len(reasons))
	for k := range reasons {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, k := range keys {
		parts = append(parts, fmt.Sprintf("%s=%d", k, reasons[k]))
	}
	return strings.Join(parts, ",")
}

func forestLanguageLoaders() map[string]func() *gts.Language {
	out := map[string]func() *gts.Language{}
	for _, e := range grm.AllLanguages() {
		out[e.Name] = e.Language
	}
	return out
}

func forestRepoRoot(t *testing.T) string {
	t.Helper()
	if v := strings.TrimSpace(os.Getenv("GTS_REPO_ROOT")); v != "" {
		return v
	}
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	// Tests run from cgo_harness/; corpus_real lives there or at repo root.
	for _, cand := range []string{wd, filepath.Dir(wd)} {
		if _, err := os.Stat(filepath.Join(cand, "cgo_harness", "corpus_real")); err == nil {
			return cand
		}
		if _, err := os.Stat(filepath.Join(cand, "corpus_real")); err == nil {
			return cand
		}
	}
	return wd
}

func forestCorpusDir(repoRoot, lang string) string {
	for _, cand := range []string{
		filepath.Join(repoRoot, "cgo_harness", "corpus_real", lang),
		filepath.Join(repoRoot, "corpus_real", lang),
	} {
		if info, err := os.Stat(cand); err == nil && info.IsDir() {
			return cand
		}
	}
	return ""
}

func forestCorpusFiles(t *testing.T, dir string) []string {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read corpus dir %s: %v", dir, err)
	}
	var files []string
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		files = append(files, filepath.Join(dir, e.Name()))
	}
	sort.Strings(files)
	return files
}

// rangedRepr serializes the visible (named) tree WITH each node's byte span, so
// the parity check catches byte-range divergences (e.g. the root not spanning
// trailing whitespace) that a plain s-expression comparison misses.
func rangedRepr(n *gts.Node, lang *gts.Language) string {
	var sb strings.Builder
	var walk func(*gts.Node)
	walk = func(nd *gts.Node) {
		fmt.Fprintf(&sb, "(%s[%d:%d]", nd.Type(lang), nd.StartByte(), nd.EndByte())
		for i := 0; i < nd.NamedChildCount(); i++ {
			sb.WriteByte(' ')
			walk(nd.NamedChild(i))
		}
		sb.WriteByte(')')
	}
	walk(n)
	return sb.String()
}

func lastNonWSByte(src []byte) uint32 {
	end := len(src)
	for end > 0 {
		switch src[end-1] {
		case ' ', '\t', '\r', '\n':
			end--
			continue
		}
		break
	}
	return uint32(end)
}

func envOr(key, def string) string {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		return v
	}
	return def
}
