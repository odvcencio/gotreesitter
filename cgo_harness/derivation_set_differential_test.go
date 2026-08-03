//go:build cgo && treesitter_c_parity && gts_derivation_set_census

package cgoharness

import (
	"fmt"
	"testing"

	sitter "github.com/tree-sitter/go-tree-sitter"

	gotreesitter "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
)

// Stage D0 receipts for spec.derivation-set-equivalence.v1.
//
// Reproduce both with:
//
//	cd cgo_harness
//	GOFLAGS=-buildvcs=false GTS_PARITY_ALLOW_HOST=1 \
//	  GTS_PARITY_C_REF_BUILD_CACHE=<cache> CGO_ENABLED=1 \
//	  go test -tags "cgo treesitter_c_parity gts_derivation_set_census" \
//	  -run 'TestDerivationSet' -count=1 -v .
//
// The gts_derivation_set_census tag is what compiles the compact-side census
// into the library at all. Without it the library has no instrument, so both
// tests below are absent from the build and every shipped gate runs against
// exactly the code main ships.

// TestDerivationSetDifferentialWitnessReproduction is stage D0's must-pass
// acceptance criterion: the eight witnesses R2 falsified must reproduce as
// classified set-level differences.
//
// The R2 measurement (finding.derivation-set-equivalence-prerequisite) wired
// a faithful port of C's selection ladder and produced 10 new accepts, 8 of
// them C-divergent. Ranking cannot repair those 8, because the candidate set
// itself is wrong. This test states, per witness, exactly which set-level
// difference the instrument measures. Every expectation here is a
// MEASUREMENT pinned after the fact, not a prediction: a change in any of
// them is a change in the lane's ground truth and must be re-adjudicated,
// not re-fitted.
func TestDerivationSetDifferentialWitnessReproduction(t *testing.T) {
	if !gotreesitter.DerivationSetCensusBuilt() {
		t.Fatal("derivation-set census is not compiled into the library; build with -tags gts_derivation_set_census")
	}

	type witness struct {
		Language string
		CName    string
		Lang     func() *gotreesitter.Language
		Name     string
		Source   string
		// Classes is the exact difference-class multiset the instrument must
		// report, in report order.
		Classes []string
		// Mechanisms is the exact mechanism attribution per class entry.
		Mechanisms []string
		// CompactSetSize and CVersionSetSize pin |D| and |V|.
		CompactSetSize  int
		CVersionSetSize int
		// ForceConvergedSplitDrops grants the converged-split-drop
		// certification kotlin withholds, for the duration of this one
		// measurement. It follows the established in-place force-and-restore
		// pattern (kotlin_a3_certification_object_declaration_regression_test.go).
		ForceConvergedSplitDrops bool
		// ExpectNoAccept marks a witness the compact route declines before
		// any accept, so no derivation set exists to compare.
		ExpectNoAccept bool
		// Note records why this witness sits in the lane.
		Note string
	}

	witnesses := []witness{
		{
			Language: "apex", CName: "apex", Lang: grammars.ApexLanguage,
			Name:   "class_literal_alias",
			Source: "public class C {\n  void m() {\n    Object t = RecordPage.class;\n  }\n}\n",
			Note:   "the named apex hypothesis: compact builds a class_literal alternative the reference runtime never builds",
		},
		{
			Language: "perl", CName: "perl", Lang: grammars.PerlLanguage,
			Name:   "print_filehandle_list_material_election",
			Source: "print $fh, \"a\", \"b\";\n",
			Note:   "list_expression against ambiguous_function_call_expression",
		},
		{
			Language: "perl", CName: "perl", Lang: grammars.PerlLanguage,
			Name:   "unshift_two_args",
			Source: "unshift @found, $_;\n",
			Note:   "list_expression against ambiguous_function_call_expression",
		},
		{
			Language: "perl", CName: "perl", Lang: grammars.PerlLanguage,
			Name:   "join_bare",
			Source: "join \"\\n\", \"a\", \"b\";\n",
			Note:   "list_expression against ambiguous_function_call_expression",
		},
		{
			Language: "ada", CName: "ada", Lang: grammars.AdaLanguage,
			Name:   "bare_aggregate_assignment_material_election",
			Source: "procedure P is\nbegin\n   R := (F => 1, G => 2);\nend;\n",
			Note:   "record_aggregate against named_array_aggregate",
		},
		{
			Language: "ada", CName: "ada", Lang: grammars.AdaLanguage,
			Name: "record_aggregate_named",
			Source: "package P is\n" +
				"   type R is record\n" +
				"      X : Integer;\n" +
				"      Y : Integer;\n" +
				"   end record;\n" +
				"   V : constant R := (X => 1, Y => 2);\n" +
				"end P;\n",
			Note: "record_aggregate field difference",
		},
		{
			Language: "ada", CName: "ada", Lang: grammars.AdaLanguage,
			Name: "discriminated_record",
			Source: "package P is\n" +
				"   type Buffer (Size : Positive) is record\n" +
				"      Data : String (1 .. Size);\n" +
				"   end record;\n" +
				"end P;\n",
			Note: "child-count difference; the spec and the finding both record this one as a pre-existing production defect that R1's decline was masking",
		},
		{
			Language: "kotlin", CName: "kotlin", Lang: grammars.KotlinLanguage,
			Name:           "annotated_declaration",
			Source:         "@Suppress(\"UNUSED\")\nfun f() {}\n",
			ExpectNoAccept: true,
			Note:           "source_file child count; measured: the shipped kotlin profile declines this input at the no-action boundary, so no accept-time derivation set exists",
		},
		{
			Language: "kotlin", CName: "kotlin", Lang: grammars.KotlinLanguage,
			Name:                     "annotated_declaration_forced_split_drops",
			Source:                   "@Suppress(\"UNUSED\")\nfun f() {}\n",
			ForceConvergedSplitDrops: true,
			Note:                     "same source with the converged-split-drop certification kotlin withholds, which is the configuration that reaches an accept at all",
		},
	}

	// Expectations, pinned from the first measurement. Keyed by
	// language/name.
	expected := map[string]struct {
		Classes         []string
		Mechanisms      []string
		CompactSetSize  int
		CVersionSetSize int
	}{
		"apex/class_literal_alias":                        {Classes: []string{derivationDiffExtra}, Mechanisms: []string{derivationMechanismCondense}, CompactSetSize: 2, CVersionSetSize: 1},
		"perl/print_filehandle_list_material_election":    {Classes: []string{derivationDiffExtra}, Mechanisms: []string{derivationMechanismCondense}, CompactSetSize: 2, CVersionSetSize: 1},
		"perl/unshift_two_args":                           {Classes: []string{derivationDiffExtra}, Mechanisms: []string{derivationMechanismCondense}, CompactSetSize: 2, CVersionSetSize: 1},
		"perl/join_bare":                                  {Classes: []string{derivationDiffExtra, derivationDiffDifferent}, Mechanisms: []string{derivationMechanismToken, derivationMechanismToken}, CompactSetSize: 2, CVersionSetSize: 1},
		"ada/bare_aggregate_assignment_material_election": {Classes: []string{derivationDiffExtra, derivationDiffDifferent}, Mechanisms: []string{derivationMechanismCondense, derivationMechanismCondense}, CompactSetSize: 2, CVersionSetSize: 1},
		"ada/record_aggregate_named":                      {Classes: []string{derivationDiffExtra, derivationDiffDifferent}, Mechanisms: []string{derivationMechanismCondense, derivationMechanismCondense}, CompactSetSize: 2, CVersionSetSize: 1},
		"ada/discriminated_record":                        {Classes: []string{derivationDiffExtra}, Mechanisms: []string{derivationMechanismCondense}, CompactSetSize: 2, CVersionSetSize: 1},
		// The only witness whose cardinalities agree: |D| = |V| = 2, and the
		// sets still disagree on content. It is the reason DIFFERENT is a
		// class of its own and not a synonym for EXTRA.
		"kotlin/annotated_declaration_forced_split_drops": {Classes: []string{derivationDiffDifferent}, Mechanisms: []string{derivationMechanismCondense}, CompactSetSize: 2, CVersionSetSize: 2},
	}

	languages := map[string]*gotreesitter.Language{}
	cLanguages := map[string]*sitter.Language{}
	for _, w := range witnesses {
		if _, ok := languages[w.Language]; !ok {
			languages[w.Language] = w.Lang()
		}
		if _, ok := cLanguages[w.CName]; !ok {
			cLang, err := COracleLanguage(w.CName)
			if err != nil {
				t.Fatalf("load %s C oracle: %v", w.CName, err)
			}
			cLanguages[w.CName] = cLang
		}
	}

	reproduced := 0
	for _, w := range witnesses {
		key := w.Language + "/" + w.Name
		lang := languages[w.Language]
		if w.ForceConvergedSplitDrops {
			previous := lang.CompactConvergedReductionSplitDropsCertified
			lang.CompactConvergedReductionSplitDropsCertified = true
			defer func() { lang.CompactConvergedReductionSplitDropsCertified = previous }()
		}
		report, err := runDerivationSetDifferential(w.Language, w.CName, lang, cLanguages[w.CName], w.Name, []byte(w.Source))
		if w.ForceConvergedSplitDrops {
			lang.CompactConvergedReductionSplitDropsCertified = false
		}
		if err != nil {
			t.Fatalf("%s: %v", key, err)
		}
		classes := make([]string, 0, len(report.Differences))
		mechanisms := make([]string, 0, len(report.Differences))
		for _, difference := range report.Differences {
			classes = append(classes, difference.Class)
			mechanisms = append(mechanisms, difference.Mechanism)
		}
		t.Logf(
			"WITNESS %-46s accepts=%d |D|=%d |V|=%d c-version-count-max=%d declined=%v selected-fold=%d c-published-in-D=%v c-published-fold=%d classes=%v mechanisms=%v (%s)",
			key, report.CompactAccepts, report.CompactSetSize, report.CVersionSetSize, report.CVersionCountMax,
			report.CompactDeclined, report.CompactSelectedFoldIndex, report.CPublishedInCompactSet, report.CPublishedFoldIndex,
			classes, mechanisms, w.Note,
		)
		if report.CompactDeclineReason != "" {
			t.Logf("    compact route fallback reason: %s", report.CompactDeclineReason)
		}
		for _, difference := range report.Differences {
			t.Logf("    %-9s %-15s %s", difference.Class, difference.Mechanism, difference.Detail)
		}
		if testing.Verbose() {
			for index, shape := range report.CandidateShapes {
				t.Logf("    compact candidate %d shape: %s", index, shape)
			}
			t.Logf("    reference published shape: %s", report.PublishedShape)
		}
		if w.ExpectNoAccept {
			// A measured non-reproduction, pinned as such. The witness is
			// real, but it is not a set difference at an accept on the
			// shipped profile: the compact route declines earlier, so D does
			// not exist. Stage D0 reports this rather than forcing it into
			// the taxonomy.
			if report.Skipped == "" {
				t.Errorf("%s: expected no accept-time derivation set, but the instrument compared one", key)
				continue
			}
			reproduced++
			continue
		}
		if report.Skipped != "" {
			t.Errorf("%s: no comparison ran: %s", key, report.Skipped)
			continue
		}
		if len(report.Differences) == 0 {
			t.Errorf("%s: witness did NOT reproduce -- the instrument measured no set-level difference", key)
			continue
		}
		want, ok := expected[key]
		if !ok {
			t.Errorf("%s: no pinned expectation", key)
			continue
		}
		if !equalStrings(classes, want.Classes) {
			t.Errorf("%s: difference classes %v, want %v", key, classes, want.Classes)
		}
		if !equalStrings(mechanisms, want.Mechanisms) {
			t.Errorf("%s: mechanisms %v, want %v", key, mechanisms, want.Mechanisms)
		}
		if report.CompactSetSize != want.CompactSetSize {
			t.Errorf("%s: |D|=%d, want %d", key, report.CompactSetSize, want.CompactSetSize)
		}
		if report.CVersionSetSize != want.CVersionSetSize {
			t.Errorf("%s: |V|=%d, want %d", key, report.CVersionSetSize, want.CVersionSetSize)
		}
		reproduced++
	}
	if reproduced != len(witnesses) {
		t.Fatalf("reproduced %d of %d witness measurements", reproduced, len(witnesses))
	}
	t.Logf(
		"all %d witness measurements match their pinned classification (8 falsified witnesses; kotlin's is measured twice, once on the shipped profile and once with the withheld certification forced)",
		len(witnesses),
	)
}

func equalStrings(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}

// derivationSetBaselineConstructed pins the constructed-source portion of the
// D0 baseline census. Constructed sources are compiled into this package, so
// every host reproduces these numbers exactly. Stages D1 and D2 must drive
// the difference counts down; a rise is a regression.
var derivationSetBaselineConstructed = map[string]struct {
	Sources     int
	Compared    int
	Extra       int
	Missing     int
	Different   int
	Order       int
	Differences int
}{
	"apex":   {Sources: 25, Compared: 22, Extra: 1, Missing: 0, Different: 0, Order: 0, Differences: 1},
	"perl":   {Sources: 17, Compared: 17, Extra: 7, Missing: 1, Different: 4, Order: 0, Differences: 12},
	"ada":    {Sources: 23, Compared: 23, Extra: 6, Missing: 0, Different: 9, Order: 0, Differences: 15},
	"kotlin": {Sources: 13, Compared: 7, Extra: 0, Missing: 0, Different: 0, Order: 0, Differences: 0},
	"python": {Sources: 26, Compared: 26, Extra: 2, Missing: 0, Different: 2, Order: 0, Differences: 4},
}

// derivationSetBaselineConstructedTotal is the D0 baseline: the total
// set-difference count over the five A3 sweep corpora's constructed sources.
// Stages D1 and D2 must drive this number down.
const derivationSetBaselineConstructedTotal = 32

// TestDerivationSetDifferentialBaselineCensus publishes the first baseline
// census: how many set-level differences each of the five A3 sweep corpora
// carries, by class and by mechanism. That total is the number stages D1 and
// D2 must drive down.
//
// The constructed portion is pinned exactly (derivationSetBaselineConstructed).
// The real-corpus portion is reported with its file count, because
// cgo_harness/corpus_real is a generated, gitignored fixture that is not
// present on every host; a host without it still reproduces the pinned
// numbers.
func TestDerivationSetDifferentialBaselineCensus(t *testing.T) {
	if !gotreesitter.DerivationSetCensusBuilt() {
		t.Fatal("derivation-set census is not compiled into the library; build with -tags gts_derivation_set_census")
	}

	grand := newDerivationSetCensusTotals()
	grandConstructed := newDerivationSetCensusTotals()
	var rows []string

	for _, entry := range derivationSetCensusLanguages() {
		lang := entry.Language()
		cLang, err := COracleLanguage(entry.CName)
		if err != nil {
			t.Fatalf("load %s C oracle: %v", entry.CName, err)
		}
		constructed := entry.Constructed()
		real := derivationSetLoadRealCorpus(t, entry.RealDir)

		constructedTotals := newDerivationSetCensusTotals()
		realTotals := newDerivationSetCensusTotals()
		for _, source := range constructed {
			report, err := runDerivationSetDifferential(entry.Name, entry.CName, lang, cLang, source.Name, source.Source)
			if err != nil {
				t.Fatalf("%s: %v", entry.Name, err)
			}
			constructedTotals.add(report)
			grandConstructed.add(report)
			grand.add(report)
			derivationSetLogDifferences(t, report)
		}
		for _, source := range real {
			report, err := runDerivationSetDifferential(entry.Name, entry.CName, lang, cLang, source.Name, source.Source)
			if err != nil {
				t.Fatalf("%s: %v", entry.Name, err)
			}
			realTotals.add(report)
			grand.add(report)
			derivationSetLogDifferences(t, report)
		}

		rows = append(rows, fmt.Sprintf(
			"%-7s constructed: sources=%3d compared=%3d declined=%3d %s | mechanisms: %s",
			entry.Name, constructedTotals.Sources, constructedTotals.Compared, constructedTotals.Declined,
			constructedTotals.classLine(), constructedTotals.mechanismLine(),
		))
		rows = append(rows, fmt.Sprintf(
			"%-7s real       : sources=%3d compared=%3d declined=%3d %s | mechanisms: %s",
			entry.Name, realTotals.Sources, realTotals.Compared, realTotals.Declined,
			realTotals.classLine(), realTotals.mechanismLine(),
		))

		want, pinned := derivationSetBaselineConstructed[entry.Name]
		if !pinned {
			t.Errorf("%s: no pinned constructed baseline", entry.Name)
			continue
		}
		if constructedTotals.Sources != want.Sources {
			t.Errorf("%s: constructed source count %d, want %d", entry.Name, constructedTotals.Sources, want.Sources)
		}
		if constructedTotals.Compared != want.Compared {
			t.Errorf("%s: constructed compared count %d, want %d", entry.Name, constructedTotals.Compared, want.Compared)
		}
		if constructedTotals.ByClass[derivationDiffExtra] != want.Extra {
			t.Errorf("%s: constructed EXTRA %d, want %d", entry.Name, constructedTotals.ByClass[derivationDiffExtra], want.Extra)
		}
		if constructedTotals.ByClass[derivationDiffMissing] != want.Missing {
			t.Errorf("%s: constructed MISSING %d, want %d", entry.Name, constructedTotals.ByClass[derivationDiffMissing], want.Missing)
		}
		if constructedTotals.ByClass[derivationDiffDifferent] != want.Different {
			t.Errorf("%s: constructed DIFFERENT %d, want %d", entry.Name, constructedTotals.ByClass[derivationDiffDifferent], want.Different)
		}
		if constructedTotals.ByClass[derivationDiffOrder] != want.Order {
			t.Errorf("%s: constructed ORDER %d, want %d", entry.Name, constructedTotals.ByClass[derivationDiffOrder], want.Order)
		}
		if constructedTotals.Differences != want.Differences {
			t.Errorf("%s: constructed total set differences %d, want %d", entry.Name, constructedTotals.Differences, want.Differences)
		}
	}

	t.Log("D0 BASELINE CENSUS -- derivation-set differences per language")
	for _, row := range rows {
		t.Log(row)
	}
	t.Logf(
		"TOTAL constructed: sources=%d compared=%d skipped-no-accept=%d skipped-other=%d declined=%d %s | mechanisms: %s | order-undetermined=%d",
		grandConstructed.Sources, grandConstructed.Compared, grandConstructed.SkippedNoAccept, grandConstructed.SkippedOther,
		grandConstructed.Declined, grandConstructed.classLine(), grandConstructed.mechanismLine(), grandConstructed.OrderUndetermined,
	)
	t.Logf(
		"TOTAL all corpora: sources=%d compared=%d skipped-no-accept=%d skipped-other=%d declined=%d %s | mechanisms: %s | order-undetermined=%d",
		grand.Sources, grand.Compared, grand.SkippedNoAccept, grand.SkippedOther,
		grand.Declined, grand.classLine(), grand.mechanismLine(), grand.OrderUndetermined,
	)
	t.Logf("TOTAL SET-DIFFERENCE COUNT (constructed)=%d (all corpora)=%d", grandConstructed.Differences, grand.Differences)
	if grandConstructed.Differences != derivationSetBaselineConstructedTotal {
		t.Errorf(
			"D0 baseline total set-difference count is %d, pinned at %d -- a fall is the point of stages D1 and D2, but the pin must move with the receipt, never silently",
			grandConstructed.Differences, derivationSetBaselineConstructedTotal,
		)
	}
}

func derivationSetLogDifferences(t *testing.T, report derivationSetReport) {
	t.Helper()
	for _, difference := range report.Differences {
		t.Logf("%s/%s: %s %s -- %s", report.Language, report.Name, difference.Class, difference.Mechanism, difference.Detail)
	}
}
