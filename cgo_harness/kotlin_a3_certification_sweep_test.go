//go:build cgo && treesitter_c_parity

package cgoharness

import (
	"path/filepath"
	"testing"

	"github.com/odvcencio/gotreesitter/grammars"
)

// TestKotlinA3CompactCertificationFullCorpusSweep is the A3
// certification-workstream (spec.campaign.v7, finding
// tied-election-family-compact-retirement) full-corpus verification receipt
// for Kotlin. It runs the same shared sweep harness as the four other landed
// languages (runA3CertificationSweep) against the SHIPPED language --
// grammars/runtime_profiles.go carries CompactPrimaryAcceptanceDerivationCertified
// only for Kotlin's exact blob.
//
// selectCompactAcceptanceDerivation's materiality gate
// (parsercore_phase0_driver.go, compactAcceptanceElectionIsVacuous) is what
// makes the primary-acceptance-derivation grant safe on its own: forcing it
// alone used to regress the object_declaration witness (a C-divergent
// accept, issue #93) before the gate existed; the gate now declines that
// tied election and falls back to production instead of publishing the
// wrong one -- see admission_switch_kotlin_certification_test.go for the
// dedicated no-cgo receipt pinning that decline. The
// annotated_declaration witness is a distinct, genuine tied election, not
// merely a derivation-coverage gap: on any profile that reaches the
// acceptance-election point (for example with split-drops also forced), its
// decline detail classifies as mechanism=material-acceptance-election --
// proof an election is involved and correctly judged material, not that the
// C-correct derivation is unreachable. Under the shipped primary-accept-only
// profile it declines earlier, at the converged-path-split checkpoint,
// before ever reaching that election point -- either way it is a decline,
// which this sweep records without chasing a fix for it.
// object_declaration_no_body_members ("object S {}") is different again:
// its decline classifies as mechanism=material-acceptance-election under
// the shipped primary-accept-only profile directly, with no split-drops
// forcing needed -- this witness reaches the tied-election point on its
// own, so the materiality gate is live today, not merely latent insurance
// for a future split-drops re-grant.
//
// CompactConvergedReductionSplitDropsCertified stays withheld: review found
// a compact-only divergence class this sweep's original 3-file real corpus
// did not surface (the annotated_extension_property_getter_* witnesses
// below). Forcing split-drops alone accepts a C-divergent tree on those
// witnesses (an unadjudicated regression, not a tied election -- the
// materiality gate does not apply to the converged-path split mechanism).
// See the runtime_profiles.go "kotlin" entry comment for the full ledger.
func TestKotlinA3CompactCertificationFullCorpusSweep(t *testing.T) {
	lang := grammars.KotlinLanguage()
	if !lang.CompactPrimaryAcceptanceDerivationCertified {
		t.Fatal("kotlin did not receive the A3 primary-acceptance-derivation certification")
	}
	if lang.CompactConvergedReductionSplitDropsCertified {
		t.Fatal("kotlin unexpectedly carries the withheld converged-split-drop certification (see grammars/runtime_profiles.go kotlin entry comment)")
	}

	real := a3LoadRealCorpusDir(t, "kotlin", filepath.Join("corpus_real", "kotlin"))
	constructed := append([]a3CertificationSweepSource{{
		Name:   "smoke_sample",
		Source: []byte(grammars.ParseSmokeSample("kotlin")),
	}}, kotlinA3AdversarialSources()...)
	sources := append(append([]a3CertificationSweepSource{}, real...), constructed...)
	t.Logf("kotlin A3 full-corpus sweep source denominator: real=%d constructed=%d total=%d", len(real), len(constructed), len(sources))

	result := runA3CertificationSweep(t, "kotlin", "kotlin", lang, sources, kotlinA3KnownDivergences)
	a3ReportSweep(t, result, kotlinA3KnownDivergences)
}

// kotlinA3KnownDivergences is an already-triaged, pre-existing
// production-route defect the tightened sweep criterion surfaced: the
// compact route only reproduces what production already produces (verified
// directly, with the compact route disabled). Family D: a declared GLR
// conflict where Go's reduceForkWindowPreference disagrees with C's
// ts_parser__select_tree on which branch to keep (assignment vs getter,
// reachable only through the detached "get() = ..." that follows an
// annotated extension-property declaration). Not a tied election, not this
// gate's scope; repair lane is tracked separately. This is a different
// defect from the annotated_extension_property_getter_* witnesses below:
// those are compact-only divergences (production is C-exact) gated on the
// withheld split-drops certificate, not a pre-existing production defect
// the compact route merely reproduces.
//
// Currently dormant, not dead: with CompactConvergedReductionSplitDropsCertified
// withheld, large__DeprecatedInstant.kt now declines outright (it hits its
// own, unrelated converged-path-split point elsewhere in the file) rather
// than accepting with this known divergence, so this entry matches nothing
// in the primary-accept-only sweep today. Keep it: it stays a true
// statement about production, and reattaches live the moment split-drops is
// re-granted (this exact file would go back to accepting).
var kotlinA3KnownDivergences = []a3KnownDivergence{
	{
		Witness:   "large__DeprecatedInstant.kt",
		FirstPath: "/source_file/assignment[14]",
		GoValue:   "assignment", CValue: "getter", Family: "D",
		// Dormant for the reason the paragraph above records: the withheld
		// split-drops certificate makes this witness decline, so the sweep
		// never reaches the comparison that would match this entry. The
		// entry stays a true statement about production and reattaches if
		// split-drops is re-granted. The stale-entry ratchet in
		// a3ReportSweep skips it for that reason alone.
		Dormant: true,
	},
}

// kotlinA3AdversarialSources gathers Kotlin's known tied-election and
// derivation-coverage-gap witnesses: the platform-modifier-recovery witness
// (b4b_alternative_set_v2_kotlin_adjudication_test.go, declines under the
// withheld split-drops certificate -- see the runtime_profiles.go "kotlin"
// entry comment), the object-declaration witness at both the collapsing and
// non-collapsing spacing (admission_switch_kotlin_certification_test.go,
// kotlin_a3_certification_object_declaration_regression_test.go), the
// empty-body object-declaration witness, the annotated-declaration tied
// election, and the annotated-extension-property-getter witnesses (line and
// block comment variants) that blocked the split-drops grant.
func kotlinA3AdversarialSources() []a3CertificationSweepSource {
	return []a3CertificationSweepSource{
		{Name: "platform_modifier_recovery", Source: []byte("internal actual fun f(): String = \"x\"\n")},
		{Name: "object_declaration_multiline", Source: []byte("package demo\n\nobject Singleton {\n    fun work() = Unit\n}\n")},
		{Name: "object_declaration_no_body_members", Source: []byte("object S {}\n")},
		{Name: "annotated_declaration", Source: []byte("@Suppress(\"UNUSED\")\nfun f() {}\n")},
		{Name: "try_catch", Source: []byte("fun f() {\n    try {\n        g()\n    } catch (e: Exception) {\n        h()\n    }\n}\n")},
		{Name: "class_with_constructor", Source: []byte("class Point(val x: Int, val y: Int)\n")},
		{Name: "when_expression", Source: []byte("fun f(x: Int): String = when (x) {\n    1 -> \"one\"\n    else -> \"other\"\n}\n")},
		{Name: "extension_function", Source: []byte("fun String.shout(): String = this.uppercase()\n")},
		{Name: "data_class", Source: []byte("data class User(val name: String, val age: Int)\n")},
		{Name: "lambda_trailing", Source: []byte("val xs = listOf(1, 2, 3).map { it * 2 }\n")},
		// annotated_extension_property_getter_line_comment /
		// _block_comment are the blocking witnesses review found: an
		// annotated extension property with a getter, followed by a
		// trailing comment. Production is C-exact on both. With
		// CompactConvergedReductionSplitDropsCertified forced on alone,
		// the compact route accepts a C-divergent tree: the annotation
		// tears into a bare prefix_expression and the getter becomes an
		// assignment (first divergence at /source_file, child count 4 vs
		// the C oracle's 3). CompactPrimaryAcceptanceDerivationCertified
		// alone is clean on both: the converged-path split declines and
		// falls back to production's correct tree. Pinned here so any
		// future attempt to re-grant split-drops must pass this sweep.
		{Name: "annotated_extension_property_getter_line_comment", Source: []byte("@Deprecated(\"old\")\nval Int.double: Int\n    get() = this * 2\n// trailing\n")},
		{Name: "annotated_extension_property_getter_block_comment", Source: []byte("@Deprecated(\"old\")\nval Int.double: Int\n    get() = this * 2\n/* trailing */\n")},
	}
}
