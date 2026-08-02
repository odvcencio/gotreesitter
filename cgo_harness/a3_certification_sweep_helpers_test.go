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
// harness. Each certified language (perl, python, apex, ada; kotlin is
// withheld, see admission_switch_kotlin_certification_withheld_test.go and
// kotlin_a3_certification_object_declaration_regression_test.go) has its own
// sweep test file that calls runA3CertificationSweep with that language's
// full real-corpus-or-constructed source set and its certification flags
// already landed in grammars/runtime_profiles.go.
//
// Method (matches the admission_switch_converged_path_test.go flag-mutation
// pattern): for every source, parse once on production (compat tail on,
// route forced off) and once on the compact candidate route (route forced
// on). A parse that declines the compact route is SAFE by construction (it
// falls back to production automatically) and is only counted and
// reason-classified. A parse that the compact route accepts must either
// byte-match the production tree, or -- where the two differ -- match the
// locked C oracle exactly (the adjudicated-exception rule: compact==C is
// correct even when production diverges). Any accepted compact tree that
// matches neither production nor the C oracle is a genuine, unadjudicated
// divergence and fails the sweep.

// a3CertificationSweepSource is one corpus file or constructed source fed
// through the sweep.
type a3CertificationSweepSource struct {
	Name   string
	Source []byte
}

// a3CertificationSweepResult is the scorecard one language's sweep produces.
type a3CertificationSweepResult struct {
	Language               string
	Files                  int
	Accepted               int
	Declined               int
	DeclineByReason        map[string]int
	AdjudicatedExceptions  []string
	Divergences            []string
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
// accept that diverges from production. It never mutates lang.
func runA3CertificationSweep(t *testing.T, language, cLangName string, lang *gotreesitter.Language, sources []a3CertificationSweepSource) a3CertificationSweepResult {
	t.Helper()
	result := a3CertificationSweepResult{Language: language, DeclineByReason: map[string]int{}}

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
			// Accepted via the compact route.
			result.Accepted++
			prodDivergences := a3GoTreeDivergences(candidateTree.RootNode(), lang, productionTree.RootNode())
			if len(prodDivergences) == 0 {
				break
			}
			// Compact disagrees with production: adjudicate against the locked
			// C oracle. compact==C is the adjudicated exception (production is
			// the divergent side); compact!=C is a genuine, unadjudicated
			// divergence that blocks certification.
			cTree := a3ParseCOracle(t, cLang, src.Source)
			cDivergences := compactT3StructuralDivergences(candidateTree.RootNode(), lang, cTree.RootNode())
			cTree.Close()
			if len(cDivergences) == 0 {
				result.AdjudicatedExceptions = append(result.AdjudicatedExceptions, src.Name)
				t.Logf(
					"%s: ADJUDICATED EXCEPTION -- compact diverges from production at %d point(s) but matches the C oracle exactly; first production divergence: %s",
					src.Name, len(prodDivergences), prodDivergences[0],
				)
			} else {
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
// divergence was found. Declines and adjudicated exceptions never fail the
// sweep.
func a3ReportSweep(t *testing.T, result a3CertificationSweepResult) {
	t.Helper()
	t.Logf(
		"%s A3 sweep: files=%d accepted=%d declined=%d adjudicated-exceptions=%d divergences=%d",
		result.Language, result.Files, result.Accepted, result.Declined,
		len(result.AdjudicatedExceptions), len(result.Divergences),
	)
	for class, count := range result.DeclineByReason {
		t.Logf("%s decline reason class %q: %d", result.Language, class, count)
	}
	for _, name := range result.AdjudicatedExceptions {
		t.Logf("%s adjudicated exception: %s", result.Language, name)
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
