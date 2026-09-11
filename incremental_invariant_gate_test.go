package gotreesitter_test

// TestIncrementalInvariantGate is the non-cherry-picking incremental-
// correctness gate. It is committable, default-on, and pure Go (no cgo, no
// C oracle needed): it only checks gotreesitter against itself.
//
// The fundamental tree-sitter contract under test: for the same final
// source text, ParseIncremental(edited, oldTree.Edit(e)) must be
// structurally identical to Parse(edited). This gate sweeps a curated,
// real-world corpus, exercising single-byte DELETE / INSERT / same-length
// REPLACE edits at a strided cadence across (in principle) every site in
// each file, and enforces that invariant for every site where the *fresh*
// parse of the edited text is genuinely clean: full-span (root covers the
// whole edited buffer) AND zero IsError()/IsMissing() nodes anywhere in the
// tree, checked via a recursive node walk. Node.HasError() is never used as
// a cleanliness signal -- the root-cause investigation that motivated this
// gate found it unreliable (a tree can be silently, structurally wrong while
// HasError() still reports false).
//
// Divergences are tracked, not silently skipped. Every one found is checked
// against testdata/incremental_allowlist.json, keyed by
// (language, edit-class, divergence-signature) where the signature is the
// type of the first structurally-diverging node plus the kind of divergence
// (childCount/type/span/named/missing). A divergence NOT in the allowlist
// fails the build -- this is what "de-silences" the corruption: today a
// consumer has no signal at all (HasError() is false, the tree looks fine);
// this gate turns that silence into a loud, specific CI failure. An
// allowlist entry that stops reproducing ALSO fails the build (a ratchet
// that forces the allowlist to be cleaned up once the underlying bug is
// fixed, rather than accumulating dead entries forever).
//
// This intentionally does NOT cherry-pick a single passing edit site the way
// cgo_harness/parity_cgo_test.go (breaks on the first fresh-parity-safe
// candidate, logs+skips the rest) and
// cgo_harness/benchmark_real_corpus_parity_test.go (verifyRealCorpusIncrementalCandidate
// returns on the first accepted candidate) do. Every strided site is
// evaluated and every outcome is accounted for, either as a pass, a known
// allowlisted divergence, or a hard failure.
//
// Scope note: the assertion above is restricted to sites where the *fresh*
// parse is genuinely clean. Sites where fresh already contains ERROR/MISSING
// nodes (e.g. an edit landed mid-operator and produced genuinely malformed
// source) are excluded from the hard assertion -- error-recovery shape is a
// separate, much noisier surface (a large fraction of arbitrary byte-level
// edits land there) that is not what "silent incremental corruption" refers
// to. The freshly-clean subclass is exactly the one with no error signal
// available to a consumer, which is why it is the one this gate hard-fails
// on.
import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	gts "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
)

// incrGateCorpusEntry describes one curated corpus file swept by the gate.
//
// stride is a hand-calibrated, FIXED byte stride (not derived from a
// wall-clock calibration at test-run time -- that would make the exact set
// of probed sites depend on machine speed, which would make the allowlist
// non-deterministic across environments). Strides were chosen by measuring
// real per-site incremental-parse cost on each file so the full sweep
// (3 edit classes x every curated file) finishes in roughly 1-3 minutes.
// python_setup.py is cheap enough to sweep exhaustively (stride 1: every
// single byte position, zero cherry-picking in the most literal sense).
var incrGateCorpus = []incrGateCorpusEntry{
	{language: "python", path: "python_setup.py", stride: 1},
	{language: "python", path: "python_large_indent.py", stride: 900},
	{language: "javascript", path: "javascript_grammar.js", stride: 200},
	{language: "json", path: "json_contract.json", stride: 15},
	{language: "go", path: "go_print.go", stride: 600},
	{language: "rust", path: "rust_ast.rs", stride: 600},
	// css_stylesheet.css: added for campaign O(edit) workstream W1
	// (spec.campaign.oedit) -- the differential oracle gate the W1 PR
	// description cites, run against the language the mechanism's
	// pre-existing top-level sibling behavior was validated on (see
	// forestFastPathDirtyPrefixScannerSensitive, incremental.go) and the
	// one the campaign's evidence base describes as already having
	// working top-level sibling reuse.
	{language: "css", path: "css_stylesheet.css", stride: 300},
	// c_repeated_functions.c: there was previously no C entry in this
	// corpus at all, which is how issue #454's C incremental-delete defect
	// (see TestIssue454CIncrementalDeleteMatchesFresh,
	// grammars/c_issue454_regression_test.go, for the exact 1-node repro
	// pinned as a named regression) escaped: no gate exercised a
	// large-enough C fixture to trip ParseStopMemoryBudget. This entry adds
	// the same harness's general-purpose sweep coverage for C (broad
	// structural-divergence protection at every "genuinely clean" edit
	// site, matching every other language above) using the same
	// repeated-function generator shape, scaled down to 24 KiB.
	//
	// It deliberately does NOT re-exercise issue #454's exact failure mode:
	// that defect's edit is a "transient error" by construction (deleting
	// one byte of an "x0" identifier leaves the invalid declarator "0", so
	// the FRESH parse of the edited text always carries a local ERROR/MISSING
	// node near the edit). This gate's freshClean scope deliberately excludes
	// exactly that class of site (see the file-level doc comment above) --
	// confirmed interactively: sweeping this fixture with the fix reverted
	// still reports zero divergences, because the one site that reaches
	// ParseStopMemoryBudget (a delete landing inside an "xN" declarator, the
	// same shape as the reported defect) is excluded from freshClean before
	// the comparison ever runs. TestIssue454CIncrementalDeleteMatchesFresh
	// has no such filter and is the actual regression gate for that defect.
	//
	// stride=3999 was chosen after an interactive sweep of this exact
	// fixture found delete/insert edits at several byte offsets (not
	// monotonic in position -- keyed to which token the edit lands in) drove
	// NewNodesAllocated past 700K (correct once resolved, ~0.6-1.5s wall
	// time at this 24 KiB scale) before the same ParseStopMemoryBudget
	// fallback in production landed on one of them (byte 4000) at every
	// checked stack (both ParseIncrementalWithTokenSource, this gate's own
	// route for "c", and the plain DFA path). At the full ~137 KiB scale
	// used by the named regression test, the same pattern was observed
	// taking upward of a minute at some offsets before the fallback resolves
	// it correctly -- too slow and non-uniform for a general sweep's CI
	// budget, hence the smaller fixture here. 7 sites span top (byte 1)
	// through near-bottom (byte 23995) of the 24616-byte fixture per the
	// task's "top/middle/bottom" ask, and this sweep completes in well
	// under a minute even when every site is reuse-hostile.
	{language: "c", path: "c_repeated_functions.c", stride: 3999},
}

type incrGateCorpusEntry struct {
	language string
	path     string // relative to testdata/incremental_gate
	stride   int
}

// incrGateAllowlistEntry is one committed, tracked, KNOWN divergence.
type incrGateAllowlistEntry struct {
	Language   string `json:"language"`
	EditClass  string `json:"editClass"`
	Signature  string `json:"signature"`
	FreshClean bool   `json:"freshGenuinelyClean"`
	Note       string `json:"note"`
}

type incrGateAllowlist struct {
	Version     int                      `json:"version"`
	Description string                   `json:"description"`
	Entries     []incrGateAllowlistEntry `json:"entries"`
}

type incrGateAllowlistKey struct {
	language, editClass, signature string
}

func loadIncrGateAllowlist(t *testing.T) *incrGateAllowlist {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("testdata", "incremental_allowlist.json"))
	if err != nil {
		t.Fatalf("read testdata/incremental_allowlist.json: %v", err)
	}
	var al incrGateAllowlist
	if err := json.Unmarshal(raw, &al); err != nil {
		t.Fatalf("parse testdata/incremental_allowlist.json: %v", err)
	}
	seen := map[incrGateAllowlistKey]bool{}
	for _, e := range al.Entries {
		if e.Language == "" || e.EditClass == "" || e.Signature == "" {
			t.Fatalf("allowlist entry missing language/editClass/signature: %+v", e)
		}
		key := incrGateAllowlistKey{e.Language, e.EditClass, e.Signature}
		if seen[key] {
			t.Fatalf("duplicate allowlist entry for %s | %s | %s", e.Language, e.EditClass, e.Signature)
		}
		seen[key] = true
	}
	return &al
}

func (al *incrGateAllowlist) index() map[incrGateAllowlistKey]*incrGateAllowlistEntry {
	m := make(map[incrGateAllowlistKey]*incrGateAllowlistEntry, len(al.Entries))
	for i := range al.Entries {
		e := &al.Entries[i]
		m[incrGateAllowlistKey{e.Language, e.EditClass, e.Signature}] = e
	}
	return m
}

// Keep each language in a separate top-level test. The gate is deliberately
// exhaustive, and a single serial meta-test exceeds the race-test timeout on
// hosted runners even though every language case is independently bounded.
// Language-level ownership also preserves the allowlist ratchet: all corpus
// files that can reproduce an entry for a language run in the same lane.
func TestIncrementalInvariantGatePython(t *testing.T) {
	testIncrementalInvariantGateLanguage(t, "python")
}

func TestIncrementalInvariantGateJavaScript(t *testing.T) {
	testIncrementalInvariantGateLanguage(t, "javascript")
}

func TestIncrementalInvariantGateJSON(t *testing.T) {
	testIncrementalInvariantGateLanguage(t, "json")
}

func TestIncrementalInvariantGateGo(t *testing.T) {
	testIncrementalInvariantGateLanguage(t, "go")
}

func TestIncrementalInvariantGateRust(t *testing.T) {
	testIncrementalInvariantGateLanguage(t, "rust")
}

func TestIncrementalInvariantGateCSS(t *testing.T) {
	testIncrementalInvariantGateLanguage(t, "css")
}

// TestIncrementalInvariantGateC adds C to this file's general sweep, using
// the same harness every other language gate uses. There was previously no C
// entry in incrGateCorpus at all -- part of how issue #454's C
// incremental-delete defect (a reuse-hostile delete tripped
// ParseStopMemoryBudget and published a 1-node ERROR tree against a fresh
// parse of tens of thousands of nodes) escaped: C shipped "certified" (a
// reuse path opened) with no gate exercising a large-enough fixture to trip
// the failure. This sweep's freshClean scope structurally excludes that
// defect's own edit shape (see the c_repeated_functions.c corpus entry's
// comment for why); TestIssue454CIncrementalDeleteMatchesFresh
// (grammars/c_issue454_regression_test.go) is the pinned named regression
// for that specific defect. This gate is the general-purpose complement:
// broad structural-divergence protection for C at every genuinely clean
// edit site, matching the coverage every other language here already has.
func TestIncrementalInvariantGateC(t *testing.T) {
	testIncrementalInvariantGateLanguage(t, "c")
}

func testIncrementalInvariantGateLanguage(t *testing.T, language string) {
	allowlist := loadIncrGateAllowlist(t)
	index := allowlist.index()

	matched := make(map[incrGateAllowlistKey]bool)
	var matchedMu sync.Mutex
	// swept records which languages actually ran, so the ratchet below only
	// enforces entries for languages this invocation swept. Without it, a
	// per-language subset run (e.g. `go test -run TestIncrementalInvariantGate/python`,
	// the natural way to debug one language) would ratchet-fail on entries for
	// languages it never ran -- a spurious "STALE ALLOWLIST ENTRY" that would
	// mislead a maintainer into deleting a live known-bug record.
	swept := make(map[string]bool)
	var sweptMu sync.Mutex

	for _, entry := range incrGateCorpus {
		if entry.language != language {
			continue
		}
		entry := entry
		t.Run(entry.path, func(t *testing.T) {
			sweptMu.Lock()
			swept[entry.language] = true
			sweptMu.Unlock()
			stats := runIncrGateSweep(t, entry, index, matched, &matchedMu)
			t.Logf("sites=%d fullSpan=%d freshClean=%d divergent(freshClean-scoped)=%d newUnlisted=%d",
				stats.totalSites, stats.fullSpanSites, stats.cleanSites, stats.divergentSites, len(stats.unlisted))
			for _, u := range stats.unlisted {
				t.Errorf("%s", u)
			}
		})
	}
	if !swept[language] {
		t.Fatalf("incremental invariant gate has no corpus entries for language %q", language)
	}

	// Ratchet: every allowlist entry must have reproduced at least once in
	// this sweep. A stale entry (bug fixed, or corpus/algorithm changed so it
	// no longer manifests) must be removed, not left to rot -- otherwise the
	// allowlist silently grows and stops meaning "currently reproducing known
	// bug".
	for _, e := range allowlist.Entries {
		sweptMu.Lock()
		ranLang := swept[e.Language]
		sweptMu.Unlock()
		if !ranLang {
			// This language was not swept in this invocation (e.g. a filtered
			// subset run), so we have no evidence about its entries -- skip,
			// don't ratchet-fail.
			continue
		}
		key := incrGateAllowlistKey{e.Language, e.EditClass, e.Signature}
		matchedMu.Lock()
		ok := matched[key]
		matchedMu.Unlock()
		if !ok {
			t.Errorf("STALE ALLOWLIST ENTRY (ratchet failure): %s | %s | %s did not reproduce anywhere in this sweep. "+
				"Before removing it from testdata/incremental_allowlist.json, confirm WHY it stopped reproducing: "+
				"(a) the incremental bug was genuinely FIXED (remove it), or (b) the fresh-clean observation window "+
				"CLOSED -- a grammar/parser change made the fresh parse at those sites contain ERROR/MISSING so they "+
				"fell out of this gate's scope while the bug may persist (do NOT remove; investigate). "+
				"The gate cannot distinguish these; a blind removal can silently retire a still-live bug. (recorded note: %q)",
				e.Language, e.EditClass, e.Signature, e.Note)
		}
	}
}

type incrGateStats struct {
	totalSites, fullSpanSites, cleanSites, divergentSites int
	unlisted                                              []string
}

func runIncrGateSweep(
	t *testing.T,
	entry incrGateCorpusEntry,
	index map[incrGateAllowlistKey]*incrGateAllowlistEntry,
	matched map[incrGateAllowlistKey]bool,
	matchedMu *sync.Mutex,
) incrGateStats {
	t.Helper()

	e := grammars.DetectLanguageByName(entry.language)
	if e == nil {
		t.Fatalf("unknown language %q", entry.language)
	}
	lang := e.Language()
	support := grammars.EvaluateParseSupport(*e, lang)
	if support.Backend == grammars.ParseBackendUnsupported || support.Backend == grammars.ParseBackendDFAPartial {
		t.Fatalf("language %q has no usable full-parse backend for the gate: %s", entry.language, support.Reason)
	}

	src, err := os.ReadFile(filepath.Join("testdata", "incremental_gate", entry.path))
	if err != nil {
		t.Fatalf("read corpus file: %v", err)
	}
	if len(src) < 8 {
		t.Fatalf("corpus file too small to sweep: %s", entry.path)
	}

	freshP := gts.NewParser(lang)
	incrP := gts.NewParser(lang)

	parseFresh := func(p *gts.Parser, s []byte) *gts.Tree {
		if support.Backend == grammars.ParseBackendTokenSource {
			tr, _ := p.ParseWithTokenSource(s, e.TokenSourceFactory(s, lang))
			return tr
		}
		tr, _ := p.Parse(s)
		return tr
	}

	baseline := parseFresh(freshP, src)
	if baseline != nil {
		defer baseline.Release()
	}
	if baseline == nil || baseline.RootNode() == nil {
		t.Fatalf("baseline fresh parse of %s failed", entry.path)
	}

	stride := entry.stride
	if stride < 1 {
		stride = 1
	}

	var stats incrGateStats
	editClasses := []string{"delete", "insert", "replace"}

	for i := 1; i < len(src)-1; i += stride {
		for _, cls := range editClasses {
			edited, edit := incrGateBuildEdit(src, i, cls)

			fresh := parseFresh(freshP, edited)
			stats.totalSites++
			if fresh == nil || fresh.RootNode() == nil || int(fresh.RootNode().EndByte()) != len(edited) {
				if fresh != nil {
					fresh.Release()
				}
				continue // fresh parse itself did not cover the whole edited buffer; out of scope
			}
			stats.fullSpanSites++

			fe, fm := incrGateTreeStats(fresh.RootNode())
			if fe != 0 || fm != 0 {
				fresh.Release()
				continue // fresh is not genuinely clean; error-recovery-path divergence is out of this gate's scope
			}
			stats.cleanSites++

			oldCopy := baseline.Copy()
			oldCopy.Edit(edit)
			var incr *gts.Tree
			if support.Backend == grammars.ParseBackendTokenSource {
				incr, _ = incrP.ParseIncrementalWithTokenSource(edited, oldCopy, e.TokenSourceFactory(edited, lang))
			} else {
				incr, _ = incrP.ParseIncremental(edited, oldCopy)
			}
			if incr == nil || incr.RootNode() == nil {
				if incr != nil {
					incr.Release()
				}
				oldCopy.Release()
				fresh.Release()
				stats.divergentSites++
				stats.unlisted = append(stats.unlisted, fmt.Sprintf(
					"SILENT-CORRUPTION(fresh-clean) unlisted: %s pos=%d class=%s: ParseIncremental returned nil/no-root while fresh parse of the identical edited text was clean and full-span",
					entry.path, i, cls))
				continue
			}

			d := incrGateFirstDivergence(lang, fresh.RootNode(), incr.RootNode(), nil)
			incr.Release()
			oldCopy.Release()
			fresh.Release()
			if d == nil {
				continue // genuinely identical -- the invariant holds here
			}
			stats.divergentSites++

			sig := d.signature()
			key := incrGateAllowlistKey{entry.language, cls, sig}
			if _, ok := index[key]; ok {
				matchedMu.Lock()
				matched[key] = true
				matchedMu.Unlock()
				continue // known, tracked divergence
			}
			stats.unlisted = append(stats.unlisted, fmt.Sprintf(
				"SILENT-CORRUPTION(fresh-clean) unlisted divergence: %s | %s | %s "+
					"(file=%s pos=%d divergingPath=%s detail=%s) -- fresh parse of the edited text is clean "+
					"(zero IsError()/IsMissing(), full span) but ParseIncremental diverges structurally from it. "+
					"This is either a NEW regression, or a real known-bug site that needs a documented entry in "+
					"testdata/incremental_allowlist.json.",
				entry.language, cls, sig, entry.path, i, d.path, d.detail))
		}
	}
	return stats
}

// incrGateBuildEdit constructs the edited buffer and the corresponding
// InputEdit for one of the three edit classes swept by the gate:
//   - delete:  remove the byte at i
//   - insert:  duplicate the byte at i (a single-byte, non-noop insertion)
//   - replace: substitute the byte at i with a different fixed byte
//     (same-length edit)
func incrGateBuildEdit(src []byte, i int, class string) ([]byte, gts.InputEdit) {
	switch class {
	case "delete":
		edited := make([]byte, 0, len(src)-1)
		edited = append(edited, src[:i]...)
		edited = append(edited, src[i+1:]...)
		p := incrGatePointAt(src, i)
		return edited, gts.InputEdit{
			StartByte: uint32(i), OldEndByte: uint32(i + 1), NewEndByte: uint32(i),
			StartPoint: p, OldEndPoint: incrGatePointAt(src, i+1), NewEndPoint: p,
		}
	case "insert":
		edited := make([]byte, 0, len(src)+1)
		edited = append(edited, src[:i]...)
		edited = append(edited, src[i])
		edited = append(edited, src[i:]...)
		p := incrGatePointAt(src, i)
		return edited, gts.InputEdit{
			StartByte: uint32(i), OldEndByte: uint32(i), NewEndByte: uint32(i + 1),
			StartPoint: p, OldEndPoint: p, NewEndPoint: incrGatePointAt(edited, i+1),
		}
	case "replace":
		repByte := byte('z')
		if src[i] == 'z' {
			repByte = 'y'
		}
		edited := append([]byte{}, src...)
		edited[i] = repByte
		return edited, gts.InputEdit{
			StartByte: uint32(i), OldEndByte: uint32(i + 1), NewEndByte: uint32(i + 1),
			StartPoint: incrGatePointAt(src, i), OldEndPoint: incrGatePointAt(src, i+1), NewEndPoint: incrGatePointAt(edited, i+1),
		}
	default:
		panic("incrGateBuildEdit: unknown edit class " + class)
	}
}

func incrGatePointAt(src []byte, off int) gts.Point {
	row, col := 0, 0
	for idx := 0; idx < off && idx < len(src); idx++ {
		if src[idx] == '\n' {
			row++
			col = 0
		} else {
			col++
		}
	}
	return gts.Point{Row: uint32(row), Column: uint32(col)}
}

// incrGateTreeStats walks the tree counting IsError()/IsMissing() nodes.
// Deliberately does NOT use Node.HasError(): the root-cause investigation
// behind this gate found HasError() unreliable as a cleanliness signal.
func incrGateTreeStats(n *gts.Node) (errN, missN int) {
	if n == nil {
		return
	}
	if n.IsError() {
		errN++
	}
	if n.IsMissing() {
		missN++
	}
	for i := 0; i < n.ChildCount(); i++ {
		e, m := incrGateTreeStats(n.Child(i))
		errN += e
		missN += m
	}
	return
}

type incrGateDivergence struct {
	kind, nodeType, path, detail string
}

// incrGateFirstDivergence walks fresh and incr in lockstep and returns the
// first structural difference found (type, span, isNamed, isMissing,
// childCount -- in that order, matching cgo_harness/parity_cgo_test.go's
// compareGoNodes field order), or nil if the two subtrees are identical.
func incrGateFirstDivergence(lang *gts.Language, fresh, incr *gts.Node, ancestorPath []string) *incrGateDivergence {
	if fresh == nil || incr == nil {
		if fresh != incr {
			// Preserve the non-nil side's type so the signature is specific
			// (e.g. "block:nilmismatch", not the content-free "<nil>:nilmismatch")
			// -- keeps the allowlist key narrow and triage informative.
			nodeType := "<nil>"
			if fresh != nil {
				nodeType = fresh.Type(lang)
			} else if incr != nil {
				nodeType = incr.Type(lang)
			}
			return &incrGateDivergence{kind: "nil", nodeType: nodeType, path: strings.Join(ancestorPath, ">")}
		}
		return nil
	}
	ft, it := fresh.Type(lang), incr.Type(lang)
	if ft != it {
		return &incrGateDivergence{kind: "type", nodeType: ft, path: strings.Join(ancestorPath, ">"), detail: fmt.Sprintf("%s!=%s", ft, it)}
	}
	curPath := append(append([]string{}, ancestorPath...), ft)
	if fresh.StartByte() != incr.StartByte() || fresh.EndByte() != incr.EndByte() {
		return &incrGateDivergence{kind: "span", nodeType: ft, path: strings.Join(curPath, ">"),
			detail: fmt.Sprintf("[%d,%d)!=[%d,%d)", fresh.StartByte(), fresh.EndByte(), incr.StartByte(), incr.EndByte())}
	}
	if fresh.IsNamed() != incr.IsNamed() {
		return &incrGateDivergence{kind: "named", nodeType: ft, path: strings.Join(curPath, ">")}
	}
	if fresh.IsMissing() != incr.IsMissing() {
		return &incrGateDivergence{kind: "missing", nodeType: ft, path: strings.Join(curPath, ">")}
	}
	fc, ic := fresh.ChildCount(), incr.ChildCount()
	if fc != ic {
		return &incrGateDivergence{kind: "childCount", nodeType: ft, path: strings.Join(curPath, ">"), detail: fmt.Sprintf("%+d", ic-fc)}
	}
	for i := 0; i < fc; i++ {
		if d := incrGateFirstDivergence(lang, fresh.Child(i), incr.Child(i), curPath); d != nil {
			return d
		}
	}
	return nil
}

func (d *incrGateDivergence) signature() string {
	switch d.kind {
	case "childCount":
		// EXACT signed delta, never bucketed. Bucketing large deltas into
		// "(large-)"/"(large+)" was removed deliberately: a coarse bucket lets
		// a live allowlist entry blanket every |delta|>5 divergence in its
		// (language, editClass) cell, so a brand-new catastrophic corruption
		// (e.g. delta -500) would be silently absorbed by an entry recorded
		// for a delta -16 bug. With exact deltas, a worse magnitude is a NEW,
		// unlisted signature and the gate hard-fails on it.
		return fmt.Sprintf("%s:childCount%s", d.nodeType, d.detail)
	case "type":
		return fmt.Sprintf("%s:type(%s)", d.nodeType, d.detail)
	case "span":
		return fmt.Sprintf("%s:span", d.nodeType)
	case "named":
		return fmt.Sprintf("%s:named", d.nodeType)
	case "missing":
		return fmt.Sprintf("%s:missing", d.nodeType)
	case "nil":
		return fmt.Sprintf("%s:nilmismatch", d.nodeType)
	default:
		return fmt.Sprintf("%s:%s", d.nodeType, d.kind)
	}
}
