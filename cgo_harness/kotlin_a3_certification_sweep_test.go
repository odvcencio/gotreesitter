//go:build cgo && treesitter_c_parity

package cgoharness

import (
	"path/filepath"
	"testing"

	"github.com/odvcencio/gotreesitter/grammars"
)

// TestKotlinA3CompactCertificationForcedSweep is the R1 verification receipt
// for Kotlin: it runs the same shared sweep harness as the four landed
// languages (runA3CertificationSweep), with both A3 certificates
// (CompactConvergedReductionSplitDropsCertified and
// CompactPrimaryAcceptanceDerivationCertified) forced on locally for the
// duration of this test only. It does not touch
// grammars/runtime_profiles.go and does not certify Kotlin -- see
// admission_switch_kotlin_certification_withheld_test.go and
// kotlin_a3_certification_object_declaration_regression_test.go for why
// Kotlin's certification still does not land in this PR.
//
// Before selectCompactAcceptanceDerivation's materiality gate
// (parsercore_phase0_driver.go, compactAcceptanceElectionIsVacuous), forcing
// both certificates regressed the object_declaration witness (a C-divergent
// accept). The gate closes that: the witness's two tied derivations are not
// byte-identical, so it now declines and falls back to production instead.
// The annotated_declaration witness is a separate, pre-existing derivation-
// coverage gap (no election is involved; the C-correct derivation is not
// reachable at the accepted head at all) and is expected to remain a
// decline or a divergence -- this sweep records that outcome, it does not
// chase a fix for it.
func TestKotlinA3CompactCertificationForcedSweep(t *testing.T) {
	lang := grammars.KotlinLanguage()
	if lang.CompactConvergedReductionSplitDropsCertified || lang.CompactPrimaryAcceptanceDerivationCertified {
		t.Fatal("shared kotlin language unexpectedly carries a withheld A3 certificate")
	}
	lang.CompactConvergedReductionSplitDropsCertified = true
	lang.CompactPrimaryAcceptanceDerivationCertified = true
	defer func() {
		lang.CompactConvergedReductionSplitDropsCertified = false
		lang.CompactPrimaryAcceptanceDerivationCertified = false
	}()

	real := a3LoadRealCorpusDir(t, "kotlin", filepath.Join("corpus_real", "kotlin"))
	constructed := append([]a3CertificationSweepSource{{
		Name:   "smoke_sample",
		Source: []byte(grammars.ParseSmokeSample("kotlin")),
	}}, kotlinA3AdversarialSources()...)
	sources := append(append([]a3CertificationSweepSource{}, real...), constructed...)
	t.Logf("kotlin A3 forced sweep source denominator: real=%d constructed=%d total=%d", len(real), len(constructed), len(sources))

	result := runA3CertificationSweep(t, "kotlin", "kotlin", lang, sources, kotlinA3KnownDivergences)
	a3ReportSweep(t, result)
}

// kotlinA3KnownDivergences is an already-triaged, pre-existing
// production-route defect the tightened sweep criterion surfaced: the
// compact route only reproduces what production already produces (verified
// directly, with the compact route disabled). Family D: a declared GLR
// conflict where Go's reduceForkWindowPreference disagrees with C's
// ts_parser__select_tree on which branch to keep (assignment vs getter,
// reachable only through the detached "get() = ..." that follows an
// annotated extension-property declaration). Not a tied election, not this
// gate's scope; repair lane is tracked separately.
var kotlinA3KnownDivergences = []a3KnownDivergence{
	{
		Witness:   "large__DeprecatedInstant.kt",
		FirstPath: "/source_file/assignment[14]",
		GoValue:   "assignment", CValue: "getter", Family: "D",
	},
}

// kotlinA3AdversarialSources gathers Kotlin's known tied-election and
// derivation-coverage-gap witnesses: the platform-modifier-recovery witness
// (b4b_alternative_set_v2_kotlin_adjudication_test.go), the object-
// declaration witness at both the collapsing and non-collapsing spacing
// (admission_switch_kotlin_certification_withheld_test.go,
// kotlin_a3_certification_object_declaration_regression_test.go), the empty-
// body object-declaration witness, and the annotated-declaration derivation-
// coverage gap.
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
	}
}
