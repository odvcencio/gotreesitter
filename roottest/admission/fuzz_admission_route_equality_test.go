//go:build !gts_no_parsercorephase0

package admission_test

import (
	"fmt"
	"github.com/odvcencio/gotreesitter/internal/frozengo"
	"sort"
	"testing"

	gts "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
	"github.com/odvcencio/gotreesitter/internal/benchfixtures"
	"github.com/odvcencio/gotreesitter/internal/ccore/bind"
)

// routeEqualityFuzzLanguages is the curated language set this fuzzer drives:
// the compact-certified heavyweights (go, javascript, html, swift, python,
// bash) plus three more compact-eligible languages (erlang, haskell, rust)
// that also pass the compact admission smoke ratchet
// (admissionScorecardRequiredCompactPasses, admission_scorecard_test.go).
// The fuzz language-selector byte indexes into this slice (modulo its
// length), so its order is part of the corpus contract: changing it changes
// which language a committed seed exercises.
var routeEqualityFuzzLanguages = []string{
	"go",
	"javascript",
	"html",
	"swift",
	"python",
	"bash",
	"erlang",
	"haskell",
	"rust",
}

// admissionRouteEqualityLanguage bundles one curated language with a
// compact-forced and a production-forced Parser, both built once and reused
// for the whole fuzz run (matching FuzzGoParseDoesNotPanic's convention in
// roottest/parse/fuzz_parser_test.go).
type admissionRouteEqualityLanguage struct {
	name       string
	lang       *gts.Language
	compact    *gts.Parser
	production *gts.Parser
}

// routeEqualityFuzzMaxInputBytes bounds every fuzz input. This is a latency
// safety cap, unrelated to the compact admission switch's former 64 KiB
// source-length eligibility floor (tranche B9 retired that floor entirely;
// this cap would exist unchanged even if it never had): the
// language-selector byte mutates independently of the content byte, so the
// mutator can and does pair a large seed (e.g. a canonical Go fixture) with
// an unrelated curated language. Measured directly (production-forced
// parses of real Go source, mis-parsed as each curated language): swift and
// erlang stay under 50ms through 16,000 bytes but climb past 3s by 32,000
// and past 7s by 65,000 -- a shape consistent with GLR multi-stack growth on
// ambiguous, wrong-grammar token streams, not a hang. Below this cap every
// curated language stayed under 60ms on the same probe. A go-test-fuzz
// worker that runs a multi-second input risks Go's own hang detector
// killing it, which reports as a spurious failure unrelated to route
// equality. 4096 is a full order of magnitude below the measured slow
// region.
//
// This cap is not raised as part of retiring the admission wall: doing so
// would need a fresh mis-parsed-mismatched-grammar latency measurement at
// the new ceiling for every curated language, which is out of this
// tranche's scope. Generative (fuzzed, size-varying) route-equality
// coverage for large inputs specifically is deferred; the tranche's own
// large-file witness table (see its PR) is the non-generative, targeted
// evidence for large-input route equality instead.
const routeEqualityFuzzMaxInputBytes = 4096

// FuzzAdmissionRouteEquality is the campaign v7 tranche B2 generative gate.
// When the certified core serves a parse (read from bind.RouteCounters), the
// v1 invariants are the oracle: no panic, the root covers the input or the
// stop reason explains the gap, an ERROR root has HasError, and an
// incremental reparse after one deterministic edit equals a fresh parse. The
// compact-versus-legacy comparison below remains for the languages the core
// does not serve (go, swift, haskell).
// An accepted compact tree must equal production unless a byte-exact witness
// carries a pinned C deep digest. Certified recovery has a separate C gate.
// The shared locked-C manifest supplies every byte-exact exception and its
// required cgo proof.
// When the compact route declines, Parse serves production within the same
// call. The input remains useful for panic and hang coverage.
//
// The fuzz entry point is the shipped Parser.Parse route, reached through
// the same public per-Parser admission switch every caller can use
// (SetAdmissionCandidateRoute, admission_switch.go) -- not the C0
// attribution harness's diagnostic lane (cgo_harness/attribution), which
// bypasses Parse and the admission switch entirely and never ships.
//
// FIXED FINDING (root-start pull-back leading-byte drop): live fuzzing used
// to reproduce a false-clean divergence in under 4 seconds from a cold
// cache, independent of which curated language it landed on. Confirmed
// witnesses: html "&0", "&;", "&#", ">0", "&000"; erlang "\x010", "\x100";
// haskell "\"\n" -- all the same shape: one non-trivia byte at byte 0
// (document start) that the accepted derivation's own root reduce never
// represented (HasError()==false, no node anywhere in the derivation covers
// the byte) while production correctly reported HasError()==true.
// diagnosticParserCoreReduceChildrenTilingGap (the B1 fix) never saw this
// gap because it is exempt at the derivation's own root symbol
// (isDerivationRootReduce, parsercore_phase0_driver.go); the shared post-
// materialization normalizeRootSourceStart (parser_result_root_build.go)
// then pulled the public root span back over the missing byte on the
// assumption of a legitimately elided leading extra, so
// finalizeDiagnosticParserCoreAcceptedRootSpan's own start check saw an
// already-laundered, tautologically-correct span. Fixed by a second,
// root-specific decline in materializeDiagnosticParserCoreAcceptedSelection
// (parsercore_phase0_driver.go) that reads the derivation's raw,
// pre-normalization root span start -- the one value the pull-back
// overwrites -- and declines fail-closed whenever it starts after the
// source's first non-trivia byte. See
// TestCompactRouteRootLeadingGapDeclines
// (admission_route_equality_root_leading_gap_test.go) for the pinned
// per-witness regression and TestCompactRouteLeadingCommentStillServes for
// the fail-open control (a legitimate leading comment still routes through
// the compact candidate).
//
// The bounded CI mutation step remains advisory. Its shared fuzz cache and
// 4 KiB cap do not provide a stable 45-second workload. A prior run also
// reported an uncommitted Haskell structural residual. Promote the step only
// after clean-cache, per-language soaks resolve both risks. The committed seed
// corpus remains required.
func FuzzAdmissionRouteEquality(f *testing.F) {
	// Several curated languages (html, javascript, swift, ...) are not
	// otherwise loaded by the always-on suite; purge the process-wide
	// embedded cache afterward so this fuzzer does not inflate heap for
	// later suite tests (matches admission_scorecard_test.go and
	// admission_route_equality_leaf_tiling_test.go).
	f.Cleanup(func() { grammars.PurgeEmbeddedLanguageCache() })

	languages := make([]admissionRouteEqualityLanguage, len(routeEqualityFuzzLanguages))
	for i, name := range routeEqualityFuzzLanguages {
		entry := grammars.DetectLanguageByName(name)
		if entry == nil {
			f.Fatalf("curated route-equality language %q is not registered", name)
		}
		lang := entry.Language()
		if name == "go" {
			// The shipped go blob is served by the certified core. This fuzzer
			// compares the compact route with the older engine, so it uses the
			// frozen artifact that the compact scheduler was certified against.
			lang = frozengo.Language()
		}
		if lang == nil {
			f.Fatalf("curated route-equality language %q resolved a nil Language", name)
		}
		if support := grammars.EvaluateParseSupport(*entry, lang); support.Backend != grammars.ParseBackendDFA {
			f.Fatalf("curated route-equality language %q is not DFA-routable (backend=%s reason=%s); Parser.Parse cannot serve it",
				name, support.Backend, support.Reason)
		}

		compact := gts.NewParser(lang)
		compact.SetAdmissionCandidateRoute(true)
		production := gts.NewParser(lang)
		production.SetAdmissionCandidateRoute(false)
		languages[i] = admissionRouteEqualityLanguage{name: name, lang: lang, compact: compact, production: production}
	}

	seedAdmissionRouteEqualityCorpus(f, languages)

	f.Fuzz(func(t *testing.T, src []byte, langSel byte) {
		if len(src) >= routeEqualityFuzzMaxInputBytes {
			t.Skip("input at or above the fuzz latency safety cap")
		}
		lp := languages[int(langSel)%len(languages)]
		exerciseAdmissionRouteEquality(t, lp, src, false)
	})
}

// FuzzJavaScriptAdmissionRouteEquality keeps live recovery mutations on
// JavaScript. The core serves JavaScript, so the v1 invariants apply (see
// FuzzAdmissionRouteEquality). The C differential owns recovered-tree
// equality.
func FuzzJavaScriptAdmissionRouteEquality(f *testing.F) {
	f.Cleanup(func() { grammars.PurgeEmbeddedLanguageCache() })
	entry := grammars.DetectLanguageByName("javascript")
	if entry == nil {
		f.Fatal("JavaScript is not registered")
	}
	lang := entry.Language()
	if lang == nil {
		f.Fatal("JavaScript resolved a nil Language")
	}
	if support := grammars.EvaluateParseSupport(*entry, lang); support.Backend != grammars.ParseBackendDFA {
		f.Fatalf("JavaScript is not DFA-routable: backend=%s reason=%s", support.Backend, support.Reason)
	}
	lp := admissionRouteEqualityLanguage{
		name:       "javascript",
		lang:       lang,
		compact:    gts.NewParser(lang),
		production: gts.NewParser(lang),
	}
	lp.compact.SetAdmissionCandidateRoute(true)
	lp.production.SetAdmissionCandidateRoute(false)

	f.Add([]byte(grammars.ParseSmokeSample("javascript")))
	witnesses := loadRouteEqualityWitnesses(f)
	ids := make([]string, 0, len(witnesses))
	for id, witness := range witnesses {
		if witness.Language == "javascript" {
			ids = append(ids, id)
		}
	}
	sort.Strings(ids)
	for _, id := range ids {
		f.Add([]byte(witnesses[id].SourceUTF8))
	}
	f.Add([]byte("function A(){A000000} # 0"))
	f.Add([]byte("a; # ; b"))
	f.Add([]byte("function f(, b) { return a + b; }"))
	f.Add([]byte("const x = a ? b :;c;"))
	// Nightly input of 27 Nov 2026 (#1800): the core and the legacy engine
	// disagree on the recovered tree; the core follows pinned C.
	f.Add([]byte("function A!0{A"))

	f.Fuzz(func(t *testing.T, src []byte) {
		if len(src) >= routeEqualityFuzzMaxInputBytes {
			t.Skip("input at or above the fuzz latency safety cap")
		}
		exerciseAdmissionRouteEquality(t, lp, src, true)
	})
}

func exerciseAdmissionRouteEquality(t *testing.T, lp admissionRouteEqualityLanguage, src []byte, recoveryOracleOwned bool) {
	t.Helper()
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("panic during route-equality fuzz (lang=%s bytes=%d): %v\ninput=%s",
				lp.name, len(src), r, previewRouteEqualityInput(src))
		}
	}()

	gts.ResetAdmissionCandidateCounters()
	_, coreBefore, _ := bind.RouteCounters()
	compactTree, err := lp.compact.Parse(src)
	if err != nil {
		t.Fatalf("lang=%s compact-lane Parse: %v (input=%s)", lp.name, err, previewRouteEqualityInput(src))
	}
	if compactTree == nil {
		t.Fatalf("lang=%s compact-lane Parse returned a nil tree with no error (input=%s)", lp.name, previewRouteEqualityInput(src))
	}
	defer compactTree.Release()

	// A certified grammar is served by the certified core, which follows
	// pinned C on malformed input; the legacy engine, the other side of the
	// comparison below, does not. For those parses the v1 invariants are the
	// oracle, not legacy equality.
	if _, coreAfter, _ := bind.RouteCounters(); coreAfter > coreBefore {
		assertCoreServedInvariants(t, lp, src, compactTree)
		return
	}

	routed, _ := gts.AdmissionCandidateCounters()
	if routed != 1 {
		// Compact declined -- engine-side, or never attempted. Parse
		// already served production internally within this same call
		// (the B1 fail-closed guarantee), so compactTree already IS the
		// production tree. Vacuous for route equality; still exercised
		// for panic and hang coverage.
		return
	}

	productionTree, err := lp.production.Parse(src)
	if err != nil {
		t.Fatalf("lang=%s production-lane Parse: %v (input=%s)", lp.name, err, previewRouteEqualityInput(src))
	}
	if productionTree == nil {
		t.Fatalf("lang=%s production-lane Parse returned a nil tree with no error (input=%s)", lp.name, previewRouteEqualityInput(src))
	}
	defer productionTree.Release()
	if recoveryOracleOwned && lp.lang.CompactStrategy2ErrorRegionCertified &&
		routeEqualityTreeCarriesNativeRecoveryNode(compactTree.RootNode()) {
		if diff := routeEqualityFirstDivergence(compactTree.RootNode(), productionTree.RootNode(), lp.lang, "root"); diff != "" {
			t.Logf("lang=%s bytes=%d recovery tree is C-oracle-owned: %s (input=%s)",
				lp.name, len(src), diff, previewRouteEqualityInput(src))
		}
		return
	}

	assertAdmissionRouteEquality(t, lp, src, compactTree, productionTree)
}

// assertCoreServedInvariants checks the v1 invariants (docs/v1-design.md,
// invariant gate) on a parse that the certified core served:
//
//  1. the root covers the input, or the stop reason explains the gap;
//  2. an ERROR root implies HasError();
//  3. an incremental reparse after one deterministic edit equals a fresh
//     parse of the edited source (D8).
//
// The caller has already established that Parse returned without a panic.
func assertCoreServedInvariants(t *testing.T, lp admissionRouteEqualityLanguage, src []byte, tree *gts.Tree) {
	t.Helper()
	root := tree.RootNode()
	if root == nil {
		if len(src) != 0 {
			t.Fatalf("lang=%s bytes=%d core returned a nil root for non-empty input (input=%s)", lp.name, len(src), previewRouteEqualityInput(src))
		}
		return
	}
	if root.IsError() && !root.HasError() {
		t.Fatalf("lang=%s bytes=%d ERROR root without HasError (input=%s)", lp.name, len(src), previewRouteEqualityInput(src))
	}
	if int(root.EndByte()) != len(src) && !coreStopExplainsGap(tree) {
		t.Fatalf("lang=%s bytes=%d root ends at byte %d with stop reason %s, which does not explain the gap (input=%s)",
			lp.name, len(src), root.EndByte(), tree.ParseStopReason(), previewRouteEqualityInput(src))
	}

	edited, edit := routeEqualityDeterministicEdit(src)
	tree.Edit(edit)
	incremental, err := lp.compact.ParseIncremental(edited, tree)
	if err != nil || incremental == nil {
		t.Fatalf("lang=%s incremental reparse: tree=%v err=%v (input=%s)", lp.name, incremental != nil, err, previewRouteEqualityInput(src))
	}
	defer incremental.Release()
	fresh, err := lp.compact.Parse(edited)
	if err != nil || fresh == nil {
		t.Fatalf("lang=%s fresh parse of edited source: tree=%v err=%v (input=%s)", lp.name, fresh != nil, err, previewRouteEqualityInput(src))
	}
	defer fresh.Release()
	ir, fr := incremental.RootNode(), fresh.RootNode()
	if (ir == nil) != (fr == nil) {
		t.Fatalf("lang=%s incremental root nilness differs from fresh after edit %+v (input=%s)", lp.name, edit, previewRouteEqualityInput(src))
	}
	if ir == nil {
		return
	}
	if ir.HasError() != fr.HasError() {
		t.Fatalf("lang=%s HasError incremental=%t fresh=%t after edit %+v (input=%s)",
			lp.name, ir.HasError(), fr.HasError(), edit, previewRouteEqualityInput(src))
	}
	if diff := routeEqualityFirstDivergence(ir, fr, lp.lang, "root"); diff != "" {
		t.Fatalf("lang=%s incremental differs from fresh after edit %+v: %s (input=%s)", lp.name, edit, diff, previewRouteEqualityInput(src))
	}
}

// coreStopExplainsGap reports whether the parse recorded a reason for ending
// before the end of the input.
func coreStopExplainsGap(tree *gts.Tree) bool {
	if tree.ParseStoppedEarly() || tree.ParseRuntime().Truncated {
		return true
	}
	switch tree.ParseStopReason() {
	case gts.ParseStopNone, gts.ParseStopAccepted:
		return false
	}
	return true
}

// routeEqualityDeterministicEdit inserts one byte, '0', at a position that
// depends only on the input, and returns the edited source with its edit.
func routeEqualityDeterministicEdit(src []byte) ([]byte, gts.InputEdit) {
	sum := 0
	for _, b := range src {
		sum += int(b)
	}
	pos := sum % (len(src) + 1)
	pointAt := func(b []byte, n int) gts.Point {
		var p gts.Point
		for _, c := range b[:n] {
			if c == '\n' {
				p.Row++
				p.Column = 0
			} else {
				p.Column++
			}
		}
		return p
	}
	start := pointAt(src, pos)
	edited := make([]byte, 0, len(src)+1)
	edited = append(edited, src[:pos]...)
	edited = append(edited, '0')
	edited = append(edited, src[pos:]...)
	return edited, gts.InputEdit{
		StartByte:   uint32(pos),
		OldEndByte:  uint32(pos),
		NewEndByte:  uint32(pos + 1),
		StartPoint:  start,
		OldEndPoint: start,
		NewEndPoint: pointAt(edited, pos+1),
	}
}

// seedAdmissionRouteEqualityCorpus commits the B2 seed corpus: the 20-witness
// B0/B1 adjudication manifest (minus the two documented open swift
// residuals), the four canonical Go docs/benchmarks.md fixtures (trimmed below
// routeEqualityFuzzMaxInputBytes, the fuzzer's own latency cap, when
// needed), one smoke snippet per curated language, and an empty-source edge
// case. languages supplies the language-selector byte for each language
// name.
func seedAdmissionRouteEqualityCorpus(f *testing.F, languages []admissionRouteEqualityLanguage) {
	f.Helper()

	langIndex := make(map[string]byte, len(languages))
	for i, lp := range languages {
		langIndex[lp.name] = byte(i)
	}

	// Empty source: Parse's documented nil-root edge case.
	f.Add([]byte{}, langIndex["go"])

	// One smoke snippet per curated language (grammars.ParseSmokeSamples,
	// the same manifest the compact admission scorecard drives).
	for _, name := range routeEqualityFuzzLanguages {
		f.Add([]byte(grammars.ParseSmokeSample(name)), langIndex[name])
	}

	// The four canonical Go full-parse fixtures (docs/benchmarks.md / internal/
	// benchfixtures), heads trimmed to routeEqualityFuzzMaxInputBytes: only
	// rewrite.go (5,116 bytes) already fits; query_compile.go (20,168),
	// language.go (41,387), and grammargen_lr.go (235,626) all need a
	// trimmed head. An untrimmed seed would immediately skip on every run
	// (the fuzz body's own size cap), so trimming here is what makes these
	// fixtures actually exercise the corpus.
	fixtures, err := benchfixtures.LoadGoFullParseFixtures()
	if err != nil {
		f.Fatalf("load canonical Go fixtures: %v", err)
	}
	for _, lf := range fixtures {
		src := lf.Source
		if len(src) >= routeEqualityFuzzMaxInputBytes {
			src = append([]byte(nil), src[:routeEqualityFuzzMaxInputBytes-1]...)
		}
		f.Add(src, langIndex["go"])
	}

	// The 20-witness B0/B1 adjudication manifest (cgo_harness/testdata/
	// compact_t3_oracle_witnesses_v2.json): 10 HTML, 8 JavaScript, 2 Swift.
	// These are the false-clean witnesses the B1 leaf-tiling gate now
	// declines (admission_route_equality_leaf_tiling_test.go). html_min_a is
	// the literal B1 reference witness "<a></a^>"
	// (TestCompactRouteHTMLErroneousEndTagByteGapDeclines); it is seeded
	// here as that witness, not duplicated.
	//
	// swift_log_1 and swift_log_2 are deliberately EXCLUDED. Both are a
	// documented, still-open compact-accepts/production-errors divergence
	// that the B1 tiling gate does not, and must not, reject: compact's
	// accepted derivation fully tiles the source for both; the two engines
	// instead disagree on shift/reduce resolution for the ambiguous
	// trailing ">>" in "x >> y>>" -- a different mechanism from the byte-
	// coverage gap this tranche's gate closes. See
	// TestCompactRouteSwiftShiftComparisonGapIsNotATilingDefect
	// (admission_route_equality_swift_residual_test.go), which pins the
	// still-open state as its own dedicated, documented residual. Seeding
	// them here would fail this fuzzer's equality assertion on every run
	// for an already-known, already-tracked finding, not a new one.
	// Promote them into this corpus once that residual closes.
	witnesses := loadRouteEqualityWitnesses(f)
	ids := make([]string, 0, len(witnesses))
	for id := range witnesses {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	excludedWitnesses := map[string]bool{"swift_log_1": true, "swift_log_2": true}
	for _, id := range ids {
		if excludedWitnesses[id] {
			continue
		}
		w := witnesses[id]
		idx, ok := langIndex[w.Language]
		if !ok {
			f.Fatalf("witness %q language %q is outside the curated route-equality language set", id, w.Language)
		}
		f.Add([]byte(w.SourceUTF8), idx)
	}

	// Class-e pin (campaign v7 class-e closure, spore.2026-08-02.alder-e.js-
	// false-clean and spore.2026-08-02.hornbeam-e.byte-continuity): a lexer-
	// skipped byte run the compact scheduler silently shifted across, tolerated
	// by the now-retired bytesAreSingleByteDecorationTrivia predicate
	// (parsercore_phase0_driver.go). Each seed is one isolated, space-padded
	// stray byte between two real tokens -- the exact shape the mechanical
	// sweep that found 189 occurrences (javascript 75, haskell 65, html 42,
	// bash 7) injected. See TestCompactRouteLexerSkippedByteGapDeclines
	// (admission_route_equality_byte_continuity_gap_test.go) for the pinned
	// direct regression on the first witness.
	f.Add([]byte("function A(){A000000} # 0"), langIndex["javascript"])
	f.Add([]byte("a; # ; b"), langIndex["javascript"])
	f.Add([]byte("<html> & <body>Hello</body></html>"), langIndex["html"])
	f.Add([]byte("e \\ cho hi"), langIndex["bash"])

	// Issue #983 pin: the shared token source splits Swift's ">>" into two
	// ">" tokens. Production's contextualActionIndex defers the narrow ">"
	// shift whenever the header's own lex mode reads the wider ">>" with a
	// real action; the compact dispatch loop lacked that deferral and
	// elected a clean derivation production declines
	// (deferContextualCloseAngleAction, parser_dfa_token_source.go).
	f.Add([]byte(" 0%0>>"), langIndex["swift"])
	f.Add([]byte("0>>"), langIndex["swift"])

	// Issue #984 pins both byte-exact Erlang forms after route graduation.
	f.Add([]byte("000\"0A!A \"A\"=0:A0!)A\"0%0000"), langIndex["erlang"])
	f.Add([]byte("000\"0A!A \"A\"=0:A0!)A\"0%0000\n"), langIndex["erlang"])

	// Nightly inputs of 27 Nov 2026 (#1801, #1800). The certified core serves
	// these grammars and follows pinned C on malformed input, so the legacy
	// engine's tree is not the oracle; the v1 invariants are.
	f.Add([]byte("0A"), langIndex["python"])
	f.Add([]byte("function A!0{A"), langIndex["javascript"])
	f.Add([]byte("0)0A"), langIndex["bash"])
}

// routeEqualityTreeCarriesNativeRecoveryNode reports whether root contains a
// node that only native compact recovery can publish. S3 publishes an extra
// ERROR container. S5 publishes a missing terminal.
//
// Such a tree follows the C oracle instead of production's divergent recovery
// shape. Route equality does not apply to that tree.
func routeEqualityTreeCarriesNativeRecoveryNode(n *gts.Node) bool {
	if n == nil {
		return false
	}
	if n.IsMissing() || (n.IsError() && n.IsExtra()) {
		return true
	}
	for i := 0; i < n.ChildCount(); i++ {
		if routeEqualityTreeCarriesNativeRecoveryNode(n.Child(i)) {
			return true
		}
	}
	return false
}

// assertAdmissionRouteEquality enforces the B2 gate: when the compact route
// accepted, its tree must equal production's on every dimension the gate
// names -- HasError, deep structure (type, span, field, flags, named-ness),
// and full leaf byte coverage.
//
// Certified native recovery has one deliberate exception, keyed on tree
// shape instead of an enumerated source allowlist. When compact publishes an
// ERROR container or a missing terminal, it follows the C oracle. Production
// can differ, so only HasError is required there. The deep checks are logged.
// A byte-exact allowlist covers only the committed witnesses. Live fuzzing
// finds new inputs for the same certified mechanism. The shape check keeps
// the full equality gate for ordinary compact parses.
func assertAdmissionRouteEquality(t *testing.T, lp admissionRouteEqualityLanguage, src []byte, compactTree, productionTree *gts.Tree) {
	t.Helper()

	compactRoot := compactTree.RootNode()
	productionRoot := productionTree.RootNode()
	if (compactRoot == nil) != (productionRoot == nil) {
		t.Fatalf("lang=%s bytes=%d root nilness diverges: compact=%t production=%t (input=%s)",
			lp.name, len(src), compactRoot == nil, productionRoot == nil, previewRouteEqualityInput(src))
	}
	if compactRoot == nil {
		return
	}

	if compactRoot.HasError() != productionRoot.HasError() {
		t.Fatalf("lang=%s bytes=%d HasError diverges: compact=%t production=%t (input=%s)",
			lp.name, len(src), compactRoot.HasError(), productionRoot.HasError(), previewRouteEqualityInput(src))
	}

	if lp.lang.CompactStrategy2ErrorRegionCertified && routeEqualityTreeCarriesNativeRecoveryNode(compactRoot) {
		if diff := routeEqualityFirstDivergence(compactRoot, productionRoot, lp.lang, "root"); diff != "" {
			t.Logf("lang=%s bytes=%d native recovery node present: compact follows the C oracle instead of production: %s (input=%s)",
				lp.name, len(src), diff, previewRouteEqualityInput(src))
		}
		return
	}

	if diff := routeEqualityFirstDivergence(compactRoot, productionRoot, lp.lang, "root"); diff != "" {
		t.Fatalf("lang=%s bytes=%d structural divergence: %s (input=%s)", lp.name, len(src), diff, previewRouteEqualityInput(src))
	}

	compactCoverage := routeEqualityLeafRanges(compactRoot)
	productionCoverage := routeEqualityLeafRanges(productionRoot)
	if diff := routeEqualityCoverageDivergence(compactCoverage, productionCoverage); diff != "" {
		t.Fatalf("lang=%s bytes=%d byte-coverage divergence: %s (input=%s)", lp.name, len(src), diff, previewRouteEqualityInput(src))
	}

	// Belt-and-suspenders: an independent whole-tree digest (type, field,
	// span, flags, child count, in preorder) must also agree.
	compactDigest, err := benchfixtures.InspectGoTree(compactRoot, lp.lang)
	if err != nil {
		t.Fatalf("lang=%s bytes=%d compact deep-tree digest: %v", lp.name, len(src), err)
	}
	productionDigest, err := benchfixtures.InspectGoTree(productionRoot, lp.lang)
	if err != nil {
		t.Fatalf("lang=%s bytes=%d production deep-tree digest: %v", lp.name, len(src), err)
	}
	if compactDigest.SHA256 != productionDigest.SHA256 {
		t.Fatalf("lang=%s bytes=%d deep-tree digest diverges without a node mismatch found by the walk above: compact=%s production=%s (input=%s)",
			lp.name, len(src), compactDigest.SHA256, productionDigest.SHA256, previewRouteEqualityInput(src))
	}
}

// routeEqualityFirstDivergence walks compact and production together in
// preorder and returns a description of the first mismatch in type, byte
// span, point span, field name, or the named/extra/missing/error/has-error
// flags -- or "" when every node agrees.
func routeEqualityFirstDivergence(compact, production *gts.Node, lang *gts.Language, path string) string {
	if compact == nil || production == nil {
		if compact == nil && production == nil {
			return ""
		}
		return fmt.Sprintf("%s nil compact=%t production=%t", path, compact == nil, production == nil)
	}
	if compact.Type(lang) != production.Type(lang) ||
		compact.StartByte() != production.StartByte() ||
		compact.EndByte() != production.EndByte() ||
		compact.StartPoint() != production.StartPoint() ||
		compact.EndPoint() != production.EndPoint() ||
		compact.IsNamed() != production.IsNamed() ||
		compact.IsExtra() != production.IsExtra() ||
		compact.IsMissing() != production.IsMissing() ||
		compact.IsError() != production.IsError() ||
		compact.ChildCount() != production.ChildCount() {
		return fmt.Sprintf(
			"%s compact=%s[%d:%d] named=%t extra=%t missing=%t error=%t children=%d production=%s[%d:%d] named=%t extra=%t missing=%t error=%t children=%d",
			path,
			compact.Type(lang), compact.StartByte(), compact.EndByte(),
			compact.IsNamed(), compact.IsExtra(), compact.IsMissing(), compact.IsError(), compact.ChildCount(),
			production.Type(lang), production.StartByte(), production.EndByte(),
			production.IsNamed(), production.IsExtra(), production.IsMissing(), production.IsError(), production.ChildCount(),
		)
	}
	if compact.HasError() != production.HasError() {
		return fmt.Sprintf("%s has_error compact=%t production=%t", path, compact.HasError(), production.HasError())
	}
	for i := 0; i < compact.ChildCount(); i++ {
		compactField := compact.FieldNameForChild(i, lang)
		productionField := production.FieldNameForChild(i, lang)
		if compactField != productionField {
			return fmt.Sprintf("%s/%d field compact=%q production=%q", path, i, compactField, productionField)
		}
		childPath := fmt.Sprintf("%s/%s[%d]", path, compact.Child(i).Type(lang), i)
		if diff := routeEqualityFirstDivergence(compact.Child(i), production.Child(i), lang, childPath); diff != "" {
			return diff
		}
	}
	return ""
}

// routeEqualityLeafRanges returns every leaf's [start, end) byte range in
// preorder -- the B2 gate's explicit full-byte-coverage dimension,
// independent of the general structural walk above.
func routeEqualityLeafRanges(root *gts.Node) [][2]uint32 {
	if root == nil {
		return nil
	}
	var ranges [][2]uint32
	var walk func(n *gts.Node)
	walk = func(n *gts.Node) {
		if n.ChildCount() == 0 {
			ranges = append(ranges, [2]uint32{n.StartByte(), n.EndByte()})
			return
		}
		for i := 0; i < n.ChildCount(); i++ {
			walk(n.Child(i))
		}
	}
	walk(root)
	return ranges
}

// routeEqualityCoverageDivergence compares two leaf byte-range lists and
// describes the first mismatch, or "" when they agree exactly.
func routeEqualityCoverageDivergence(compact, production [][2]uint32) string {
	if len(compact) != len(production) {
		return fmt.Sprintf("leaf count compact=%d production=%d", len(compact), len(production))
	}
	for i := range compact {
		if compact[i] != production[i] {
			return fmt.Sprintf("leaf[%d] byte range compact=%d..%d production=%d..%d", i, compact[i][0], compact[i][1], production[i][0], production[i][1])
		}
	}
	return ""
}

// previewRouteEqualityInput renders a fuzz input as a bounded, safely
// quoted string for failure messages: arbitrary fuzz bytes may not be valid
// UTF-8 or printable.
func previewRouteEqualityInput(src []byte) string {
	const maxPreviewBytes = 200
	if len(src) > maxPreviewBytes {
		src = src[:maxPreviewBytes]
	}
	return fmt.Sprintf("%q", src)
}
