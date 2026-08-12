//go:build cgo && treesitter_c_parity

package cgoharness

// Outline C-oracle differential (spec.outline-api.v1, gate G-2).
//
// The cgo harness already binds the official C tree-sitter runtime and its
// query engine (parity_query_exact_test.go:14, :215). The outline projection
// (outline.go, package gotreesitter) is built entirely from tags-query
// captures: resolve the query through grammars.ResolveTagsQuery, run it, and
// feed the resulting candidates through one fixed stage C-F builder
// (collectOutlineCandidates -> filterOutlineCandidates -> buildOutlineForest
// in outline.go) regardless of which engine produced the captures. So
// running the SAME resolved tags query through BOTH query engines over the
// SAME parsed source and diffing the capture streams exactly -- pattern
// index, capture name, node type, start byte, end byte, capture text -- is a
// direct fleet probe of outline correctness: exact capture parity implies
// exact outline parity, because the builder that turns captures into an
// outline forest is shared and reads nothing else. This file never
// reconstructs the outline forest on the C side; capture-stream equality is
// the whole gate, per that argument.
//
// Gating tiers:
//
//   - CoreNine: the nine languages grammars/registry_test.go:280-291 proves
//     have a compilable tags query (go, python, javascript, typescript, tsx,
//     rust, java, c, cpp). HARD-asserted: a capture-stream divergence, a
//     query compile failure against the locked C grammar, or an
//     unexpectedly empty tags query fails the test. No language in this
//     tier is ever special-cased or skipped for a real divergence.
//
//   - FleetCensus: every OTHER registered language with a resolvable tags
//     query. LOG-ONLY -- nothing in this half of the file calls t.Error or
//     t.Fatal for a divergence. Breadth is controlled by GTS_PARITY_MODE
//     exactly like the rest of the harness (parityMode, parity_cgo_test.go
//     :192-204): a small representative set by default, the top-50
//     correctness lock set for GTS_PARITY_MODE=top50, and the full
//     206-language registry for GTS_PARITY_MODE=exhaustive. The run ends
//     with one printed census table: the ratchet baseline for a later
//     stage (spec G-3), not a gate itself.
//
// Two outcomes are explicitly NOT divergence and NOT parity (spec.outline
// -api.v1 section 6, honest limits (a) and (b)): a language with no
// resolvable tags query is "vacuous" (nothing ran), and a resolved query
// that fails to compile against one engine's grammar is
// "query_incompatible" (an inferred query is generated from the Go
// grammar's symbol table and is not guaranteed to compile against the
// pinned C grammar). Both get their own census bucket so vacuity or a
// compile gap can never masquerade as parity.
//
// Run:
//
//	go test . -tags treesitter_c_parity -run '^TestOutlineOracleDifferential$' -count=1 -v
//	GTS_PARITY_MODE=top50 go test . -tags treesitter_c_parity -run '^TestOutlineOracleDifferential$' -count=1 -v -timeout 60m
//	GTS_PARITY_MODE=exhaustive go test . -tags treesitter_c_parity -run '^TestOutlineOracleDifferential$' -count=1 -v -timeout 60m
//
// Outside the docker harness (cgo_harness/docker/run_parity_in_docker.sh),
// set GTS_PARITY_ALLOW_HOST=1 for a focused local run; see
// testmain_cgo_test.go for the host-execution gate this file inherits from
// the rest of the package.

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"

	gotreesitter "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
	sitter "github.com/tree-sitter/go-tree-sitter"
)

// outlineDifferentialCoreLanguages is the exact core-nine list
// grammars.TestCoreLanguagesHaveCompilableTagsQuery pins (registry_test.go
// :280-291). Keep this byte-for-byte identical to that test's "core" slice;
// the two lists describe the same fleet floor from two different gates.
var outlineDifferentialCoreLanguages = []string{
	"go",
	"python",
	"javascript",
	"typescript",
	"tsx",
	"rust",
	"java",
	"c",
	"cpp",
}

var outlineDifferentialCoreSet = func() map[string]bool {
	out := make(map[string]bool, len(outlineDifferentialCoreLanguages))
	for _, name := range outlineDifferentialCoreLanguages {
		out[name] = true
	}
	return out
}()

// outlineDiffStatus classifies one language's differential outcome for the
// census table.
type outlineDiffStatus string

const (
	// outlineDiffEqual: the two capture streams are identical in every
	// compared field.
	outlineDiffEqual outlineDiffStatus = "equal"
	// outlineDiffDiverged: both engines produced a tree and ran the query;
	// the capture streams differ. A real finding -- never special-cased.
	outlineDiffDiverged outlineDiffStatus = "diverged"
	// outlineDiffVacuous: the language has no resolvable tags query, so
	// neither engine ran a query. Nothing to compare; not parity.
	outlineDiffVacuous outlineDiffStatus = "vacuous"
	// outlineDiffQueryIncompatible: the resolved tags query -- the same
	// string on both sides -- failed to compile against one engine's
	// grammar. Not a capture-level divergence and not parity.
	outlineDiffQueryIncompatible outlineDiffStatus = "query_incompatible"
	// outlineDiffSkipped: the comparison could not run for a reason
	// unrelated to query or outline correctness (no C reference for this
	// language/ABI, unsupported Go parse backend, operator exclusion).
	outlineDiffSkipped outlineDiffStatus = "skipped"
)

// outlineDiffResult is one language's differential outcome.
type outlineDiffResult struct {
	language      string
	fixtureSource string // "smoke_sample" or "corpus_sources"
	fixtureBytes  int
	goMatches     int
	cMatches      int
	goCaptures    int
	cCaptures     int
	mismatches    int // count of differing (match, capture) positions
	status        outlineDiffStatus
	skipEligible  bool // true only for infra reasons the hard tier may also skip on
	detail        string
	firstDiff     string
	outlineReport string // sanity-only Outliner receipt; never gates
}

func TestOutlineOracleDifferential(t *testing.T) {
	t.Run("CoreNine", testOutlineDifferentialCoreNine)
	t.Run("FleetCensus", testOutlineDifferentialFleetCensus)
}

// testOutlineDifferentialCoreNine HARD-asserts capture-stream parity for the
// core nine languages. See the file comment for exactly what counts as a
// pass; every failure path reports language, fixture, and (for a real
// divergence) the first differing capture on both sides.
func testOutlineDifferentialCoreNine(t *testing.T) {
	for _, name := range outlineDifferentialCoreLanguages {
		name := name
		t.Run(name, func(t *testing.T) {
			scheduleParityMemoryScavenge(t)
			entry, ok := parityEntriesByName[name]
			if !ok {
				t.Fatalf("core outline language %q is not registered in grammars.AllLanguages", name)
			}
			result := runOutlineDifferentialLanguage(entry)
			logOutlineDifferentialResult(t, result)

			switch result.status {
			case outlineDiffEqual:
				return
			case outlineDiffSkipped:
				if result.skipEligible {
					t.Skipf("%s: %s", name, result.detail)
					return
				}
				t.Fatalf("core outline language %q could not be compared: %s", name, result.detail)
			case outlineDiffVacuous:
				t.Fatalf("core outline language %q resolved an empty tags query; "+
					"grammars/registry_test.go:280-291 guarantees a compilable core-nine query", name)
			case outlineDiffQueryIncompatible:
				t.Fatalf("core outline language %q: the tags query that compiles against the Go grammar "+
					"failed to compile against the locked C grammar: %s", name, result.detail)
			case outlineDiffDiverged:
				t.Fatalf("core outline language %q: C-oracle capture-stream divergence "+
					"(%d mismatched position(s); go_matches=%d/go_captures=%d c_matches=%d/c_captures=%d, fixture=%s %dB)\n"+
					"first mismatch: %s",
					name, result.mismatches, result.goMatches, result.goCaptures, result.cMatches, result.cCaptures,
					result.fixtureSource, result.fixtureBytes, result.firstDiff)
			}
		})
	}
}

// testOutlineDifferentialFleetCensus runs the SAME differential, LOG-ONLY,
// over every registered language outside the core nine that has a
// resolvable tags query. Breadth follows GTS_PARITY_MODE exactly like the
// rest of the harness: smoke by default, top50, or exhaustive for the full
// registry. Nothing in this subtest calls t.Error or t.Fatal for a
// divergence; the printed census table is the ratchet baseline for a later
// stage, not a gate.
func testOutlineDifferentialFleetCensus(t *testing.T) {
	scheduleParityMemoryScavenge(t)

	entries := grammars.AllLanguages()
	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		if outlineDifferentialCoreSet[entry.Name] {
			continue
		}
		if !outlineDifferentialFleetIncluded(entry.Name) {
			continue
		}
		names = append(names, entry.Name)
	}
	sort.Strings(names)

	if len(names) == 0 {
		t.Skip("no non-core languages selected for the outline fleet census at this GTS_PARITY_MODE")
	}

	rows := make([]outlineDiffResult, 0, len(names))
	for _, name := range names {
		name := name
		entry, ok := parityEntriesByName[name]
		if !ok {
			continue
		}
		t.Run(name, func(t *testing.T) {
			result := runOutlineDifferentialLanguage(entry)
			rows = append(rows, result)
			logOutlineDifferentialResult(t, result)
			// LOG-ONLY by design: no t.Error, no t.Fatal, for any status
			// including "diverged". See the file comment.
		})
	}

	t.Log(formatOutlineDifferentialCensusTable(rows))
}

// outlineDifferentialFleetIncluded controls fleet-census breadth. It mirrors
// parityIncludeStructuralLanguage (parity_cgo_test.go:231-242): smoke mode
// runs the harness's small representative set, top50 mode runs the top-50
// correctness lock set, and exhaustive mode runs the full registry -- the
// true fleet differential the spec describes. An operator exclusion
// (GTS_PARITY_SKIP_LANGS) still gets a census row so the skip stays visible
// instead of silently shrinking the table.
func outlineDifferentialFleetIncluded(name string) bool {
	if parityLanguageExcluded(name) {
		return true
	}
	if parityRunExhaustive() {
		return true
	}
	if parityMode() == "top50" {
		return top50ParityLanguageSet[name]
	}
	return smokeParityLanguages[name]
}

// runOutlineDifferentialLanguage runs the differential for one language. It
// never touches *testing.T: callers decide whether a non-equal status fails
// the build (CoreNine) or is only logged (FleetCensus).
func runOutlineDifferentialLanguage(entry grammars.LangEntry) outlineDiffResult {
	result := outlineDiffResult{language: entry.Name}

	if parityLanguageExcluded(entry.Name) {
		result.status = outlineDiffSkipped
		result.skipEligible = true
		result.detail = "excluded via GTS_PARITY_SKIP_LANGS"
		return result
	}

	tagsQuery := grammars.ResolveTagsQuery(entry)
	if strings.TrimSpace(tagsQuery) == "" {
		result.status = outlineDiffVacuous
		result.detail = "no resolvable tags query"
		return result
	}

	support, ok := paritySupportForName(entry.Name)
	if !ok || support.Backend == grammars.ParseBackendUnsupported {
		reason := "no parse support report"
		if ok {
			reason = support.Reason
		}
		result.status = outlineDiffSkipped
		result.detail = "go parse unsupported: " + reason
		return result
	}

	source, provenance := outlineDifferentialFixture(entry)
	src := []byte(source)
	result.fixtureSource = provenance
	result.fixtureBytes = len(src)

	goTree, goLang, err := parseWithGo(parityCase{name: entry.Name, source: source}, src, nil)
	if err != nil {
		result.status = outlineDiffSkipped
		result.detail = "go parse error: " + err.Error()
		return result
	}
	defer releaseGoTree(goTree)

	// Sanity aside, non-gating: run the Go core Outliner over the SAME tree
	// and record its receipt. This runs whenever the Go parse succeeds,
	// independent of whether the C-oracle comparison below can run at all.
	result.outlineReport = outlineDifferentialSanityReport(goLang, tagsQuery, goTree)

	cLang, err := ParityCLanguage(entry.Name)
	if err != nil {
		if reason := parityReferenceSkipReason(err); reason != "" {
			result.status = outlineDiffSkipped
			result.skipEligible = true
			result.detail = "no C reference parser: " + reason
			return result
		}
		result.status = outlineDiffSkipped
		result.detail = "load C parser: " + err.Error()
		return result
	}

	cParser := sitter.NewParser()
	defer cParser.Close()
	if err := cParser.SetLanguage(cLang); err != nil {
		if reason := parityReferenceSkipReason(err); reason != "" {
			result.status = outlineDiffSkipped
			result.skipEligible = true
			result.detail = "C SetLanguage: " + reason
			return result
		}
		result.status = outlineDiffSkipped
		result.detail = "C SetLanguage: " + err.Error()
		return result
	}
	cTree := cParser.Parse(src, nil)
	if cTree == nil || cTree.RootNode() == nil {
		result.status = outlineDiffSkipped
		result.detail = "C parser returned no tree"
		return result
	}
	defer cTree.Close()

	goQuery, err := gotreesitter.NewQuery(tagsQuery, goLang)
	if err != nil {
		result.status = outlineDiffQueryIncompatible
		result.detail = "Go NewQuery: " + err.Error()
		return result
	}

	// sitter.NewQuery returns a concrete *QueryError, not the error
	// interface (query.go:199 in the go-tree-sitter module). Give it its
	// own variable: reusing "err" here would let := re-declare only cQuery
	// while boxing a (possibly nil) *QueryError into the pre-existing
	// error-typed "err" variable, which is never a nil interface even when
	// the underlying pointer is nil -- a classic Go footgun that would make
	// every successful C compile look like a query_incompatible failure.
	cQuery, cQueryErr := sitter.NewQuery(cLang, tagsQuery)
	if cQueryErr != nil {
		result.status = outlineDiffQueryIncompatible
		result.detail = "C NewQuery: " + cQueryErr.Error()
		return result
	}
	defer cQuery.Close()

	goMatches := outlineDiffGoQueryMatches(goLang, goTree, goQuery, src)
	cMatches, err := outlineDiffCQueryMatchesWithPredicates(cQuery, cTree, src, tagsQuery)
	if err != nil {
		result.status = outlineDiffSkipped
		result.detail = "C predicate evaluation: " + err.Error()
		return result
	}

	result.goMatches = len(goMatches)
	result.cMatches = len(cMatches)
	result.goCaptures = outlineDiffCaptureTotal(goMatches)
	result.cCaptures = outlineDiffCaptureTotal(cMatches)

	// outlineDiffCanonicalOrder (see its doc comment): neither engine
	// guarantees a relative order between two DIFFERENT patterns that both
	// start matching at the SAME tree node, and outline.go's builder does
	// not depend on it either. Canonicalizing both streams the same way
	// before comparing can only turn a same-multiset, different-order
	// "diverged" into "equal"; it can never hide a match or a capture that
	// actually differs, since nothing is merged or dropped, only reordered.
	outlineDiffCanonicalOrder(goMatches)
	outlineDiffCanonicalOrder(cMatches)

	if reflect.DeepEqual(goMatches, cMatches) {
		result.status = outlineDiffEqual
		return result
	}

	result.status = outlineDiffDiverged
	result.mismatches = outlineDiffMismatchCount(goMatches, cMatches)
	result.firstDiff = outlineDiffFirstMismatch(goMatches, cMatches)
	return result
}

// outlineDiffGoQueryMatches runs a compiled Go query over a Go tree and
// reduces it to the same neutral shape assertExactQueryParity compares
// (collectGoExactQueryMatches, parity_query_exact_test.go:504-536), minus
// the *testing.T dependency: this differential decides per-tier how a
// compile or run failure is reported, so the collector itself never fails a
// test.
func outlineDiffGoQueryMatches(lang *gotreesitter.Language, tree *gotreesitter.Tree, q *gotreesitter.Query, source []byte) []exactQueryMatch {
	cursor := q.Exec(tree.RootNode(), lang, source)
	var matches []exactQueryMatch
	for {
		m, ok := cursor.NextMatch()
		if !ok {
			break
		}
		snap := exactQueryMatch{PatternIndex: m.PatternIndex}
		for _, c := range m.Captures {
			if c.Node == nil {
				continue
			}
			snap.Captures = append(snap.Captures, exactQueryCapture{
				Name:      c.Name,
				Type:      c.Node.Type(lang),
				Named:     c.Node.IsNamed(),
				StartByte: c.Node.StartByte(),
				EndByte:   c.Node.EndByte(),
				Text:      c.Text(source),
			})
		}
		matches = append(matches, snap)
	}
	return matches
}

// The C-side counterpart of outlineDiffGoQueryMatches is
// outlineDiffCQueryMatchesWithPredicates (outline_differential_predicates_
// test.go), not a plain cursor.Matches(...).Next() loop like
// collectCExactQueryMatches (parity_query_exact_test.go:538-579): a
// resolved tags query's predicates can sit outside their pattern's own
// parens (grammars/tags_query_infer.go's elixir/clojure/commonlisp/scheme
// overrides all do), which gotreesitter's own parser tolerates but the C
// library compiles into a separate, always-true phantom pattern; see that
// file's comment for the full mechanism.

func outlineDiffCaptureTotal(matches []exactQueryMatch) int {
	total := 0
	for _, m := range matches {
		total += len(m.Captures)
	}
	return total
}

// outlineDiffCanonicalOrder sorts a capture-stream snapshot in place into a
// comparison-stable order: ascending by the match's own earliest capture
// StartByte (its tree position), then ascending PatternIndex as a tie-break.
//
// Both engines walk a parsed tree in the same depth-first, byte-ascending
// order (this harness's own structural parity suite establishes the two
// trees are shaped identically fleet-wide; parity_cgo_test.go:59), so a
// genuine structural difference already shows up as a position or content
// mismatch under this key. What this key additionally absorbs is
// narrower: when two DIFFERENT patterns both start matching at the exact
// same tree node -- elixir's unconditional
// "(call (identifier) @name) @reference.call" and its predicate-gated
// "@definition.module"/"@definition.function" siblings all root at the same
// "call" node -- gotreesitter's own matcher always tries same-rooted
// patterns in ascending declaration order (query.go's buildRootPatternIndex
// walks q.patterns 0..N-1 and appends to each symbol's candidate list in
// that order, unconditionally), which is exactly this key's tie-break, but
// the C library's compiled NFA does not make the same guarantee (verified:
// even with every predicate correctly nested in its own pattern's parens,
// tree-sitter's own query engine still emits elixir's reference.call match
// before the definition.module match at their shared root node, the
// reverse of gotreesitter's order).
//
// outline.go's builder cannot see this reordering: collectOutlineCandidates
// discards every "@reference.*"-only match before anything else inspects it
// (no "@definition.*" capture, defCount == 0), and every OTHER order
// dependency in that pipeline (filterOutlineCandidates' "keep the first
// emitted" duplicate tie-break, buildOutlineForest's own explicit
// start-byte/end-byte sort) only chooses among candidates already proven
// indistinguishable in every field the returned OutlineSymbol carries. So
// this reordering changes no real output.
//
// The important property for THIS differential is narrower still and does
// not depend on that outline-specific argument: sorting is a pure
// reordering applied identically to both streams. It cannot merge or drop a
// match or a capture, so it can only turn a same-multiset, different-order
// "diverged" into "equal" -- it can never hide a match or capture that
// genuinely differs between engines, and outlineDiffFirstMismatch still
// reports the first differing element in the (now-shared) canonical order.
func outlineDiffCanonicalOrder(matches []exactQueryMatch) {
	sort.SliceStable(matches, func(i, j int) bool {
		pi, pj := outlineDiffMatchPosition(matches[i]), outlineDiffMatchPosition(matches[j])
		if pi != pj {
			return pi < pj
		}
		return matches[i].PatternIndex < matches[j].PatternIndex
	})
}

// outlineDiffMatchPosition is a match's tree position for
// outlineDiffCanonicalOrder: the smallest StartByte among its own captures.
// A match with no captures at all (never produced by any resolved tags
// query in this repository -- every pattern binds at least one capture)
// sorts last within its PatternIndex group.
func outlineDiffMatchPosition(m exactQueryMatch) uint32 {
	pos := ^uint32(0)
	for _, c := range m.Captures {
		if c.StartByte < pos {
			pos = c.StartByte
		}
	}
	return pos
}

// outlineDiffMismatchCount counts capture-stream positions where the two
// sequences disagree: a missing/extra match, a pattern-index disagreement,
// or a missing/extra/differing capture within an otherwise-aligned match.
// It is a magnitude indicator for the census table, not a formal edit
// distance.
func outlineDiffMismatchCount(goMatches, cMatches []exactQueryMatch) int {
	n := len(goMatches)
	if len(cMatches) > n {
		n = len(cMatches)
	}
	count := 0
	for i := 0; i < n; i++ {
		if i >= len(goMatches) || i >= len(cMatches) {
			count++
			continue
		}
		g, c := goMatches[i], cMatches[i]
		if g.PatternIndex != c.PatternIndex {
			count++
		}
		capN := len(g.Captures)
		if len(c.Captures) > capN {
			capN = len(c.Captures)
		}
		for j := 0; j < capN; j++ {
			if j >= len(g.Captures) || j >= len(c.Captures) {
				count++
				continue
			}
			if g.Captures[j] != c.Captures[j] {
				count++
			}
		}
	}
	return count
}

// outlineDiffFirstMismatch returns a precise, human-readable description of
// the first position where the two capture streams disagree, with both
// sides' values. Empty when the streams are identical (never called in that
// case, since it is only invoked after reflect.DeepEqual reports false).
func outlineDiffFirstMismatch(goMatches, cMatches []exactQueryMatch) string {
	n := len(goMatches)
	if len(cMatches) > n {
		n = len(cMatches)
	}
	for i := 0; i < n; i++ {
		if i >= len(goMatches) {
			return fmt.Sprintf("match[%d]: absent on Go side; C has pattern=%d captures=%d",
				i, cMatches[i].PatternIndex, len(cMatches[i].Captures))
		}
		if i >= len(cMatches) {
			return fmt.Sprintf("match[%d]: absent on C side; Go has pattern=%d captures=%d",
				i, goMatches[i].PatternIndex, len(goMatches[i].Captures))
		}
		g, c := goMatches[i], cMatches[i]
		if g.PatternIndex != c.PatternIndex {
			return fmt.Sprintf("match[%d]: pattern index Go=%d C=%d", i, g.PatternIndex, c.PatternIndex)
		}
		capN := len(g.Captures)
		if len(c.Captures) > capN {
			capN = len(c.Captures)
		}
		for j := 0; j < capN; j++ {
			if j >= len(g.Captures) {
				return fmt.Sprintf("match[%d] capture[%d]: absent on Go side; C=%s", i, j, formatOutlineDiffCapture(c.Captures[j]))
			}
			if j >= len(c.Captures) {
				return fmt.Sprintf("match[%d] capture[%d]: absent on C side; Go=%s", i, j, formatOutlineDiffCapture(g.Captures[j]))
			}
			if gc, cc := g.Captures[j], c.Captures[j]; gc != cc {
				return fmt.Sprintf("match[%d] capture[%d]: Go=%s C=%s", i, j, formatOutlineDiffCapture(gc), formatOutlineDiffCapture(cc))
			}
		}
	}
	return ""
}

func formatOutlineDiffCapture(c exactQueryCapture) string {
	return fmt.Sprintf("{name:%s type:%s named:%v start:%d end:%d text:%q}",
		c.Name, c.Type, c.Named, c.StartByte, c.EndByte, c.Text)
}

// outlineDifferentialMaxFixtureBytes caps the opportunistic real-corpus
// fixture so the differential's per-file cost stays modest even at
// exhaustive breadth.
const outlineDifferentialMaxFixtureBytes = 48 * 1024

// outlineDifferentialFixture picks one fixture for a language, following the
// harness's existing corpus-then-smoke-sample convention (compare
// realCorpusBenchmarkRoot, benchmark_real_corpus_parity_test.go:337-357):
// prefer a small file from a real corpus_sources/<name> checkout when one is
// present, and fall back to the language's dedicated smoke sample
// (grammars.ParseSmokeSample) -- the same fixture parityCases already uses
// for every registered language (parity_cgo_test.go:75-84). corpus_sources
// is a set of git submodules that are not always checked out; the fallback
// keeps this differential runnable with zero external state.
func outlineDifferentialFixture(entry grammars.LangEntry) (source, provenance string) {
	if src, ok := outlineDifferentialCorpusFixture(entry); ok {
		return src, "corpus_sources"
	}
	return grammars.ParseSmokeSample(entry.Name), "smoke_sample"
}

func outlineDifferentialCorpusRoot() string {
	for _, candidate := range []string{
		"corpus_sources",
		filepath.Join("cgo_harness", "corpus_sources"),
		filepath.Join("..", "cgo_harness", "corpus_sources"),
	} {
		if st, err := os.Stat(candidate); err == nil && st.IsDir() {
			return candidate
		}
	}
	return ""
}

// outlineDifferentialCorpusFixture returns the first modestly-sized file
// under corpus_sources/<name> matching the language's registered extensions,
// in lexical walk order (deterministic). It is a best-effort lookup: any
// absence (no corpus root, no per-language subdir, no matching file) simply
// returns false so the caller falls back to the smoke sample.
func outlineDifferentialCorpusFixture(entry grammars.LangEntry) (string, bool) {
	root := outlineDifferentialCorpusRoot()
	if root == "" {
		return "", false
	}
	dir := filepath.Join(root, entry.Name)
	if st, err := os.Stat(dir); err != nil || !st.IsDir() {
		return "", false
	}

	exts := make(map[string]bool, len(entry.Extensions))
	for _, ext := range entry.Extensions {
		exts[strings.ToLower(ext)] = true
	}

	var chosen string
	_ = filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		if len(exts) > 0 && !exts[strings.ToLower(filepath.Ext(path))] {
			return nil
		}
		info, statErr := d.Info()
		if statErr != nil || info.Size() == 0 || info.Size() > outlineDifferentialMaxFixtureBytes {
			return nil
		}
		chosen = path
		return filepath.SkipAll
	})
	if chosen == "" {
		return "", false
	}
	data, err := os.ReadFile(chosen)
	if err != nil || len(data) == 0 {
		return "", false
	}
	return string(data), true
}

// outlineDifferentialSanityReport runs the Go core Outliner over the Go tree
// and formats its OutlineReport. Diagnostic only -- see the file comment --
// and never influences outlineDiffResult.status.
func outlineDifferentialSanityReport(lang *gotreesitter.Language, tagsQuery string, tree *gotreesitter.Tree) string {
	outliner, err := gotreesitter.NewOutliner(lang, tagsQuery)
	if err != nil {
		return fmt.Sprintf("NewOutliner error: %v", err)
	}
	symbols, report := outliner.OutlineTree(tree)
	return fmt.Sprintf(
		"symbols=%d omitted=%d(no_name=%d dup=%d name_conflict=%d conflict=%d overlap=%d bad_name_range=%d multi_def=%d) "+
			"owner_rule_misses=%d decline=%q truncated=%v tree_has_error=%v",
		len(symbols), report.Omitted(), report.OmittedNoName, report.OmittedDuplicate, report.OmittedNameConflict,
		report.OmittedConflict, report.OmittedOverlap, report.OmittedInvalidNameRange, report.OmittedMultipleDefinitions,
		report.OwnerRuleMisses, report.DeclineReason, report.Truncated, report.TreeHasError,
	)
}

func logOutlineDifferentialResult(t *testing.T, r outlineDiffResult) {
	t.Helper()
	t.Logf("outline differential %-14s status=%-18s fixture=%s(%dB) go_matches=%d c_matches=%d go_captures=%d c_captures=%d mismatches=%d",
		r.language, r.status, r.fixtureSource, r.fixtureBytes, r.goMatches, r.cMatches, r.goCaptures, r.cCaptures, r.mismatches)
	if r.detail != "" {
		t.Logf("  detail: %s", r.detail)
	}
	if r.firstDiff != "" {
		t.Logf("  first mismatch: %s", r.firstDiff)
	}
	if r.outlineReport != "" {
		t.Logf("  outline report (sanity, non-gating): %s", r.outlineReport)
	}
}

// formatOutlineDifferentialCensusTable renders the fleet census's one
// summary table: the ratchet baseline for a later stage (spec.outline-api.v1
// gate G-3). Log-only: printing this table is the entire contract here, it
// never drives a pass/fail decision.
func formatOutlineDifferentialCensusTable(rows []outlineDiffResult) string {
	counts := make(map[outlineDiffStatus]int, 8)
	for _, r := range rows {
		counts[r.status]++
	}

	var b strings.Builder
	fmt.Fprintf(&b, "\nOutline C-oracle differential fleet census (log-only, mode=%s, languages=%d)\n", parityMode(), len(rows))
	fmt.Fprintf(&b, "%-16s %-18s %9s %9s %9s %10s  %s\n", "language", "status", "go_match", "c_match", "mismatch", "fixture_B", "detail")
	for _, r := range rows {
		fmt.Fprintf(&b, "%-16s %-18s %9d %9d %9d %10d  %s\n",
			r.language, r.status, r.goMatches, r.cMatches, r.mismatches, r.fixtureBytes, outlineDiffTableDetail(r.detail, 64))
	}
	fmt.Fprintf(&b, "\nsummary: equal=%d diverged=%d vacuous=%d query_incompatible=%d skipped=%d total=%d\n",
		counts[outlineDiffEqual], counts[outlineDiffDiverged], counts[outlineDiffVacuous],
		counts[outlineDiffQueryIncompatible], counts[outlineDiffSkipped], len(rows))
	return b.String()
}

func outlineDiffTruncate(s string, limit int) string {
	if len(s) <= limit {
		return s
	}
	if limit <= 3 {
		return s[:limit]
	}
	return s[:limit-3] + "..."
}

// outlineDiffTableDetail flattens a (possibly multi-line) detail string --
// for example a *QueryError message, which spells its own "Impossible
// pattern:\n<snippet>\n<caret>" body -- to one space-joined line before
// truncating it, so a census row never spills across the fixed-width table.
// The full, unflattened detail still reaches the per-language log line (see
// logOutlineDifferentialResult); only the table row is flattened.
func outlineDiffTableDetail(detail string, limit int) string {
	flat := strings.Join(strings.Fields(detail), " ")
	return outlineDiffTruncate(flat, limit)
}
