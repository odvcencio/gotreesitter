//go:build cgo && treesitter_c_parity

package cgoharness

import (
	"fmt"
	"strings"
	"testing"

	sitter "github.com/tree-sitter/go-tree-sitter"

	gotreesitter "github.com/odvcencio/gotreesitter"
)

// This file is the shared A3 certification-workstream (spec.campaign.v7,
// finding tied-election-family-compact-retirement) full-corpus verification
// harness. Each certified language (perl, python, apex, ada, kotlin) has its
// own sweep test file that calls runA3CertificationSweep with that
// language's full real-corpus-or-constructed source set and its
// certification flags already landed in grammars/runtime_profiles.go.
// Kotlin ships CompactPrimaryAcceptanceDerivationCertified only --
// selectCompactAcceptanceDerivation's materiality gate
// (parsercore_phase0_driver.go, compactAcceptanceElectionIsVacuous) is what
// makes that grant safe on its own. CompactConvergedReductionSplitDropsCertified
// stays withheld: review found a compact-only divergence class on an
// annotated extension property; see the runtime_profiles.go "kotlin" entry
// comment, admission_switch_kotlin_certification_test.go, and
// kotlin_a3_certification_object_declaration_regression_test.go for the
// dedicated receipts.
//
// Method (matches the admission_switch_converged_path_test.go flag-mutation
// pattern): for every source, parse once on production (compat tail on,
// route forced off) and once on the compact candidate route (route forced
// on). A parse that declines the compact route is SAFE by construction (it
// falls back to production automatically) and is only counted and
// reason-classified. A parse that the compact route accepts is adjudicated
// against the locked C oracle directly and unconditionally: certification
// means compact==C. Matching production is not itself sufficient and is
// checked only to label the outcome -- a clean pass when compact, production,
// and C all agree, or an adjudicated exception (the one sanctioned deviation)
// when compact==C but production diverges. An accepted compact tree that does
// not match the C oracle is a genuine, unadjudicated divergence and fails the
// sweep, whether or not it happens to match production.

// a3CertificationSweepSource is one corpus file or constructed source fed
// through the sweep.
type a3CertificationSweepSource struct {
	Name   string
	Source []byte
}

// a3CertificationSweepResult is the scorecard one language's sweep produces.
type a3CertificationSweepResult struct {
	Language              string
	Files                 int
	Accepted              int
	Declined              int
	DeclineByReason       map[string]int
	AdjudicatedExceptions []string
	Divergences           []string
	TrackedDivergences    []string
	// MatchedKnownDivergences records every a3KnownDivergence entry that
	// a3MatchesKnownDivergence actually matched against a live divergence
	// during this sweep. a3ReportSweep diffs this against the caller's known
	// list to catch a repaired entry nobody removed: see the stale-entry
	// ratchet there.
	MatchedKnownDivergences map[a3KnownDivergence]bool
}

// a3KnownDivergence is one already-triaged, pre-existing production-route
// defect: the compact route only reproduces what production already
// produces (verified by parsing the witness with the compact route
// disabled), so this is not a tied-election defect and sits outside the
// materiality gate's scope. Tracked here so the sweep's own pass/fail
// signal stays honest without silently widening what counts as
// certification-clean: an entry only suppresses the exact divergence it
// names (witness, first divergent node path, and the Go/C values recorded
// there). A witness whose divergence shape has moved -- a different path,
// or the same path with different Go/C values -- is NOT a match and still
// fails the sweep as a new, unadjudicated divergence.
//
// Family tags the mechanism: "M" (materialization inherited-field
// over-projection), "S" (materialization aliased multi-token span), "D"
// (GLR derivation selection at a declared conflict). Entries burn down as
// each family's repair lands; they do not expire on their own.
type a3KnownDivergence struct {
	Witness   string
	FirstPath string
	GoValue   string
	CValue    string
	Family    string
	// Dormant exempts this entry from the stale-entry ratchet in
	// a3ReportSweep. Set it only for an entry that stays a true statement
	// about the production route but cannot match today, because the sweep
	// declines the witness before it compares trees. A dormant entry
	// reattaches on its own when the decline goes away. Every use needs a
	// written reason beside the entry that names what suppresses the match.
	// Do not set it to quiet an entry whose defect the repair fixed: that
	// entry must come out.
	Dormant bool
}

// a3MatchesKnownDivergence reports whether first (the first entry in a
// compactT3StructuralDivergences result) is exactly the divergence known
// records for witness, and returns that entry if so.
func a3MatchesKnownDivergence(known []a3KnownDivergence, witness string, first compactT3StructuralDivergence) (a3KnownDivergence, bool) {
	for _, k := range known {
		if k.Witness == witness && k.FirstPath == first.Path && k.GoValue == first.GoValue && k.CValue == first.CValue {
			return k, true
		}
	}
	return a3KnownDivergence{}, false
}

// a3DeclineReasonClass buckets a compact-route decline reason string into a
// coarse, stable class for the sweep's decline scorecard. Declines are safe
// regardless of class (they fall back to production automatically); the
// classification exists purely for the receipt.
func a3DeclineReasonClass(reason string) string {
	switch {
	case reason == "":
		return "unknown-empty-reason"
	case strings.Contains(reason, "converged-path reduction split"):
		return "converged-path-reduction-split"
	case strings.Contains(reason, "material-acceptance-election"):
		// Must precede the "did not accept EOF" case below: the materiality
		// gate's full decline detail (admission_census.go,
		// admissionCensusStopDecline) wraps its own boundary text with a
		// "did not accept EOF: " prefix, so that coarser, generic substring
		// always matches too. Checking this first keeps a genuine tied
		// election (parsercore_phase0_driver.go,
		// compactAcceptanceElectionIsVacuous) out of the no-eof-accept
		// catch-all bucket.
		//
		// This bucket only ever fires with GTS_ADMISSION_CENSUS=1: without
		// it, requireParserCoreFreshFullAcceptance
		// (parsercore_phase0_fresh_full_runner.go) returns the plain,
		// unclassified "did not accept EOF" error instead of the detailed
		// boundary text this case matches. testmain_cgo_test.go's TestMain
		// sets that env var process-wide for this package before any test
		// runs, so every sweep's decline classification is honest.
		return "material-acceptance-election"
	case strings.Contains(reason, "did not accept EOF"):
		return "no-eof-accept"
	case strings.Contains(reason, "not sole exact EOF") || strings.Contains(reason, "acceptance is not sole exact EOF"):
		return "not-sole-exact-eof"
	case strings.Contains(reason, "runner unavailable"):
		return "runner-unavailable"
	case strings.Contains(reason, "recovery"):
		return "recovery-entered"
	default:
		return "other:" + reason
	}
}

// runA3CertificationSweep parses every source in sources on both routes for
// lang (which must already carry the certification flags under test -- the
// caller lands them in grammars/runtime_profiles.go, or forces them
// temporarily for a withheld-language probe) and classifies every outcome.
// cLangName resolves the locked C oracle used to adjudicate every compact
// accept that diverges from production. known is this language's enumerated
// list of already-triaged, pre-existing production-route divergences
// (a3KnownDivergence); pass nil for a language with none. It never mutates
// lang.
func runA3CertificationSweep(t *testing.T, language, cLangName string, lang *gotreesitter.Language, sources []a3CertificationSweepSource, known []a3KnownDivergence) a3CertificationSweepResult {
	t.Helper()
	result := a3CertificationSweepResult{
		Language:                language,
		DeclineByReason:         map[string]int{},
		MatchedKnownDivergences: map[a3KnownDivergence]bool{},
	}

	cLang, err := COracleLanguage(cLangName)
	if err != nil {
		t.Fatalf("load %s C oracle: %v", cLangName, err)
	}

	for _, src := range sources {
		result.Files++

		production := gotreesitter.NewParser(lang)
		production.SetAdmissionCandidateRoute(false)
		productionTree, err := production.Parse(src.Source)
		if err != nil {
			t.Fatalf("%s: production parse: %v", src.Name, err)
		}

		routedBefore, fallbackBefore := gotreesitter.AdmissionCandidateCounters()
		candidate := gotreesitter.NewParser(lang)
		candidate.SetAdmissionCandidateRoute(true)
		candidateTree, err := candidate.Parse(src.Source)
		if err != nil {
			t.Fatalf("%s: candidate parse: %v", src.Name, err)
		}
		routedAfter, fallbackAfter := gotreesitter.AdmissionCandidateCounters()

		switch {
		case routedAfter == routedBefore+1 && fallbackAfter == fallbackBefore:
			// Accepted via the compact route. Certification means compact == the
			// locked C oracle tree, checked directly and unconditionally: matching
			// production alone is not sufficient, because production can itself
			// carry an undetected divergence from C that an accepted compact tree
			// would then inherit and pass silently. Every accepted source is
			// therefore adjudicated against the C oracle here, not only the ones
			// that already disagree with production. Production agreement is used
			// only to label the outcome (a clean pass, or an adjudicated exception
			// where compact is right and production is wrong); it never
			// substitutes for the C comparison.
			result.Accepted++
			prodDivergences := a3GoTreeDivergences(candidateTree.RootNode(), lang, productionTree.RootNode())
			cTree := a3ParseCOracle(t, cLang, src.Source)
			cDivergences := compactT3StructuralDivergences(candidateTree.RootNode(), lang, cTree.RootNode())
			cTree.Close()
			switch {
			case len(cDivergences) == 0:
				// compact == C. A clean pass when production also agrees; an
				// adjudicated exception (the existing sanctioned deviation) when
				// production does not.
				if len(prodDivergences) != 0 {
					result.AdjudicatedExceptions = append(result.AdjudicatedExceptions, src.Name)
					t.Logf(
						"%s: ADJUDICATED EXCEPTION -- compact diverges from production at %d point(s) but matches the C oracle exactly; first production divergence: %s",
						src.Name, len(prodDivergences), prodDivergences[0],
					)
				}
			case len(prodDivergences) == 0:
				// compact == production, but neither matches C: reproducing
				// production is not a sanctioned deviation on its own. If this
				// exact divergence is already triaged and tracked as a
				// pre-existing production-route defect (known), it is safe by
				// the same construction as a decline -- the compact route
				// contributes nothing here, it only inherits what production
				// already produces -- so it is counted separately and does not
				// fail the sweep. Any other divergence, or this witness's
				// divergence at a different node or with different Go/C
				// values, is not a match and still fails.
				if k, ok := a3MatchesKnownDivergence(known, src.Name, cDivergences[0]); ok {
					result.MatchedKnownDivergences[k] = true
					detail := fmt.Sprintf(
						"%s: TRACKED DIVERGENCE (family %s, pre-existing production-route defect) -- compact matches production but both diverge from the C oracle at %d point(s), first: %s",
						src.Name, k.Family, len(cDivergences), compactT3FormatDivergence(cDivergences[0]),
					)
					result.TrackedDivergences = append(result.TrackedDivergences, detail)
					t.Log(detail)
					break
				}
				detail := fmt.Sprintf(
					"%s: UNADJUDICATED DIVERGENCE -- compact matches production but both diverge from the C oracle at %d point(s), first: %s",
					src.Name, len(cDivergences), compactT3FormatDivergence(cDivergences[0]),
				)
				result.Divergences = append(result.Divergences, detail)
				t.Log(detail)
			default:
				if k, ok := a3MatchesKnownDivergence(known, src.Name, cDivergences[0]); ok {
					result.MatchedKnownDivergences[k] = true
					detail := fmt.Sprintf(
						"%s: TRACKED DIVERGENCE (family %s, pre-existing production-route defect) -- compact matches neither production (%d pt(s), first: %s) nor the C oracle (%d pt(s), first: %s)",
						src.Name, k.Family, len(prodDivergences), prodDivergences[0], len(cDivergences), compactT3FormatDivergence(cDivergences[0]),
					)
					result.TrackedDivergences = append(result.TrackedDivergences, detail)
					t.Log(detail)
					break
				}
				detail := fmt.Sprintf(
					"%s: UNADJUDICATED DIVERGENCE -- compact matches neither production (%d pt(s), first: %s) nor the C oracle (%d pt(s), first: %s)",
					src.Name, len(prodDivergences), prodDivergences[0], len(cDivergences), compactT3FormatDivergence(cDivergences[0]),
				)
				result.Divergences = append(result.Divergences, detail)
				t.Log(detail)
			}
		case fallbackAfter == fallbackBefore+1 && routedAfter == routedBefore:
			// Declined: safe by construction (production served the parse).
			result.Declined++
			class := a3DeclineReasonClass(gotreesitter.AdmissionCandidateLastFallbackReason())
			result.DeclineByReason[class]++
		default:
			t.Fatalf(
				"%s: ambiguous admission counters before=(%d,%d) after=(%d,%d) -- expected exactly one of routed/fallback to advance by one",
				src.Name, routedBefore, fallbackBefore, routedAfter, fallbackAfter,
			)
		}

		productionTree.Release()
		candidateTree.Release()
	}

	return result
}

// a3ReportSweep logs the scorecard and fails the test if any unadjudicated
// divergence was found, or if any entry in known went stale (see the
// stale-entry ratchet below). Declines, adjudicated exceptions, and tracked
// (already-triaged, pre-existing production-route) divergences never fail
// the sweep on their own. known must be the exact list runA3CertificationSweep
// produced result from; pass nil for a language with no known-divergence
// list.
func a3ReportSweep(t *testing.T, result a3CertificationSweepResult, known []a3KnownDivergence) {
	t.Helper()
	t.Logf(
		"%s A3 sweep: files=%d accepted=%d declined=%d adjudicated-exceptions=%d tracked-divergences=%d divergences=%d",
		result.Language, result.Files, result.Accepted, result.Declined,
		len(result.AdjudicatedExceptions), len(result.TrackedDivergences), len(result.Divergences),
	)
	for class, count := range result.DeclineByReason {
		t.Logf("%s decline reason class %q: %d", result.Language, class, count)
	}
	for _, name := range result.AdjudicatedExceptions {
		t.Logf("%s adjudicated exception: %s", result.Language, name)
	}
	for _, d := range result.TrackedDivergences {
		t.Logf("%s tracked divergence: %s", result.Language, d)
	}

	// Stale-entry ratchet: a known-divergence entry suppresses one exact,
	// currently-live divergence. After the repair for that divergence lands,
	// the sweep no longer reaches the entry. runA3CertificationSweep records
	// a match only when a3MatchesKnownDivergence fires. Such an entry then
	// certifies nothing, but it still reads as a live defect to a person who
	// skims the list. Fail loud instead. PR #638 removed eight repaired
	// entries by hand; this ratchet makes the next burn-down mandatory.
	//
	// An entry marked Dormant is exempt. A dormant entry describes a
	// production defect that is still real, but the sweep declines its
	// witness before it compares trees, so no match can happen today. See
	// the Dormant field's own comment for the rules.
	var stale []string
	for _, k := range known {
		if k.Dormant {
			continue
		}
		if !result.MatchedKnownDivergences[k] {
			stale = append(stale, fmt.Sprintf("%s (family %s, path %s)", k.Witness, k.Family, k.FirstPath))
		}
	}
	if len(stale) != 0 {
		for _, s := range stale {
			t.Errorf("%s A3 sweep: stale allowlist entry -- remove it: %s", result.Language, s)
		}
		t.Fatalf("%s A3 sweep found %d stale known-divergence entry(s) that matched nothing this run; the repair landed, burn the entry down", result.Language, len(stale))
	}

	if len(result.Divergences) != 0 {
		for _, d := range result.Divergences {
			t.Error(d)
		}
		t.Fatalf("%s A3 sweep found %d unadjudicated divergence(s); certification withheld", result.Language, len(result.Divergences))
	}
}

func a3ParseCOracle(t *testing.T, cLang *sitter.Language, source []byte) *sitter.Tree {
	t.Helper()
	parser := sitter.NewParser()
	defer parser.Close()
	if err := parser.SetLanguage(cLang); err != nil {
		t.Fatalf("set C oracle language: %v", err)
	}
	tree := parser.Parse(source, nil)
	if tree == nil || tree.RootNode() == nil {
		t.Fatal("C oracle parse returned no root")
	}
	return tree
}

// a3GoTreeDivergences walks two Go trees sharing the same Language in
// lockstep and returns every structural divergence found (type, named,
// missing, HasError, byte span, child count, and incoming field name). A nil
// result means the two trees are structurally identical on every compared
// axis.
func a3GoTreeDivergences(a *gotreesitter.Node, lang *gotreesitter.Language, b *gotreesitter.Node) []string {
	var out []string
	a3WalkGoTreeDivergences(a, lang, b, "/root", &out)
	return out
}

func a3WalkGoTreeDivergences(a *gotreesitter.Node, lang *gotreesitter.Language, b *gotreesitter.Node, path string, out *[]string) {
	if a == nil || b == nil {
		if a != nil || b != nil {
			*out = append(*out, fmt.Sprintf("%s: nil mismatch (a-nil=%v b-nil=%v)", path, a == nil, b == nil))
		}
		return
	}

	aType, bType := a.Type(lang), b.Type(lang)
	if aType != bType {
		*out = append(*out, fmt.Sprintf("%s: type %q != %q", path, aType, bType))
	}
	if a.IsNamed() != b.IsNamed() {
		*out = append(*out, fmt.Sprintf("%s: named %v != %v", path, a.IsNamed(), b.IsNamed()))
	}
	if a.IsMissing() != b.IsMissing() {
		*out = append(*out, fmt.Sprintf("%s: missing %v != %v", path, a.IsMissing(), b.IsMissing()))
	}
	if a.HasError() != b.HasError() {
		*out = append(*out, fmt.Sprintf("%s: hasError %v != %v", path, a.HasError(), b.HasError()))
	}
	if a.StartByte() != b.StartByte() || a.EndByte() != b.EndByte() {
		*out = append(*out, fmt.Sprintf("%s: span [%d-%d] != [%d-%d]", path, a.StartByte(), a.EndByte(), b.StartByte(), b.EndByte()))
	}

	aCount, bCount := a.ChildCount(), b.ChildCount()
	if aCount != bCount {
		*out = append(*out, fmt.Sprintf("%s: childCount %d != %d", path, aCount, bCount))
		return
	}

	for i := 0; i < aCount; i++ {
		aChild, bChild := a.Child(i), b.Child(i)
		if af, bf := a.FieldNameForChild(i, lang), b.FieldNameForChild(i, lang); af != bf {
			*out = append(*out, fmt.Sprintf("%s[%d]: field %q != %q", path, i, af, bf))
		}
		childType := "?"
		if aChild != nil {
			childType = aChild.Type(lang)
		} else if bChild != nil {
			childType = bChild.Type(lang)
		}
		a3WalkGoTreeDivergences(aChild, lang, bChild, fmt.Sprintf("%s/%s[%d]", path, childType, i), out)
	}
}
