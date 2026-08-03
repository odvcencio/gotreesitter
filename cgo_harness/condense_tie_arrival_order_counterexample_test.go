//go:build cgo && treesitter_c_parity && gts_derivation_set_census

package cgoharness

import (
	"testing"

	sitter "github.com/tree-sitter/go-tree-sitter"

	gotreesitter "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
)

// Stage D1 receipt for spec.derivation-set-equivalence.v1 section 2.
//
// Section 2 proposes to repair the condense-class differences by collapsing a
// same-predecessor precedence tie under a PROVEN ARRIVAL ORDER: "the unstamped
// primary precedes stamped links; stamped links order by ascending stamp". The
// measurement below falsifies that rule. Two witnesses reach the identical tie
// shape -- an unstamped incumbent link against an incoming link stamped with
// the parse's first fork -- and the reference runtime's published tree sits on
// OPPOSITE sides of that pair. No total order on fork provenance can serve
// both, so no arrival-order collapse repairs this class.
//
// Reproduce with:
//
//	cd cgo_harness
//	GOFLAGS=-buildvcs=false GTS_PARITY_ALLOW_HOST=1 \
//	  GTS_PARITY_C_REF_BUILD_CACHE=<cache> CGO_ENABLED=1 \
//	  go test -tags "cgo treesitter_c_parity gts_derivation_set_census" \
//	  -run 'TestCondenseTieArrivalOrder' -count=1 -v .
//
// The measurement carries no shipped behaviour: the compact core keeps the
// deferral (internal/parsercorephase0/core.go, condenseWithOutcomeAtomic),
// exactly as it did before this receipt landed.

// TestCondenseTieArrivalOrderCounterexample pins the two witnesses that
// falsify the arrival-order collapse.
//
// Both witnesses reach the same precedence tie in Core.condense: one boundary,
// one predecessor, two structurally different payloads over one span, equal
// dynamic precedence. Both ties carry the same fork provenance -- the
// incumbent link is unstamped and the incoming link carries fork stamp 1.
//
// The instrument then asks which compact candidate equals the reference
// runtime's published tree, and reports its position in the compact fold order
// (CPublishedFoldIndex). The two witnesses answer differently:
//
//   - apex class_literal_alias: the reference runtime's tree is the candidate
//     the FOLD ORDER LISTS FIRST. A first-wins collapse is right here.
//   - objc protocol_argument: the reference runtime's tree is the candidate
//     the FOLD ORDER LISTS SECOND. A first-wins collapse is wrong here.
//
// A collapse rule reads only the two links' fork provenance, which is equal
// across the two witnesses, so it must return the same side for both. One of
// the two is therefore always wrong. That is the falsification.
//
// The measured consequence, for the record: wiring "unstamped first" produced
// 14 unadjudicated divergences across the ada, kotlin and perl A3 sweeps, and
// wiring the opposite polarity produced ada trees matching neither production
// nor the reference runtime. Restricting the collapse to the only shape whose
// order IS provable -- two links that both carry fork stamps, which is one arm
// pair of one dispatch -- moved the census by zero, because no witness in the
// five sweep corpora has that shape.
func TestCondenseTieArrivalOrderCounterexample(t *testing.T) {
	if !gotreesitter.DerivationSetCensusBuilt() {
		t.Fatal("derivation-set census is not compiled into the library; build with -tags gts_derivation_set_census")
	}

	type witness struct {
		Language string
		CName    string
		Lang     func() *gotreesitter.Language
		Name     string
		Source   string
		// PublishedFoldIndex is the compact fold-order position of the
		// candidate whose tree equals the reference runtime's published tree.
		PublishedFoldIndex int
		// CompactSetSize and CVersionSetSize pin |D| and |V|.
		CompactSetSize  int
		CVersionSetSize int
		Note            string
	}

	witnesses := []witness{
		{
			Language: "apex", CName: "apex", Lang: grammars.ApexLanguage,
			Name:               "class_literal_alias",
			Source:             "public class C {\n  void m() {\n    Object t = RecordPage.class;\n  }\n}\n",
			PublishedFoldIndex: 0,
			CompactSetSize:     2,
			CVersionSetSize:    1,
			Note:               "the reference runtime's tree is the candidate the fold order lists FIRST",
		},
		{
			Language: "objc", CName: "objc", Lang: grammars.ObjcLanguage,
			Name:               "protocol_argument",
			Source:             "@interface CallbackClient : NSObject <ClientProtocol>\n@end\n",
			PublishedFoldIndex: 1,
			CompactSetSize:     2,
			CVersionSetSize:    1,
			Note:               "the reference runtime's tree is the candidate the fold order lists SECOND",
		},
	}

	seen := map[int]string{}
	for _, w := range witnesses {
		lang := w.Lang()
		if lang == nil {
			t.Fatalf("%s: language unavailable", w.Language)
		}
		cLang, err := COracleLanguage(w.CName)
		if err != nil {
			t.Fatalf("load %s C oracle: %v", w.CName, err)
		}
		report, err := runDerivationSetDifferential(w.Language, w.CName, lang, cLang, w.Name, []byte(w.Source))
		if err != nil {
			t.Fatalf("%s/%s: %v", w.Language, w.Name, err)
		}
		key := w.Language + "/" + w.Name
		t.Logf(
			"COUNTEREXAMPLE %-28s |D|=%d |V|=%d c-published-in-D=%v c-published-fold-index=%d declined=%v (%s)",
			key, report.CompactSetSize, report.CVersionSetSize,
			report.CPublishedInCompactSet, report.CPublishedFoldIndex, report.CompactDeclined, w.Note,
		)
		for index, shape := range report.CandidateShapes {
			t.Logf("    compact candidate %d: %s", index, shape)
		}
		t.Logf("    reference published : %s", report.PublishedShape)

		if report.Skipped != "" {
			t.Fatalf("%s: no comparison ran: %s", key, report.Skipped)
		}
		if !report.CPublishedInCompactSet {
			t.Fatalf("%s: the reference runtime's tree is not a compact candidate at all; this witness no longer states the counterexample", key)
		}
		if report.CompactSetSize != w.CompactSetSize {
			t.Errorf("%s: |D|=%d, want %d", key, report.CompactSetSize, w.CompactSetSize)
		}
		if report.CVersionSetSize != w.CVersionSetSize {
			t.Errorf("%s: |V|=%d, want %d", key, report.CVersionSetSize, w.CVersionSetSize)
		}
		if report.CPublishedFoldIndex != w.PublishedFoldIndex {
			t.Errorf("%s: reference tree sits at compact fold index %d, want %d", key, report.CPublishedFoldIndex, w.PublishedFoldIndex)
		}
		if other, clash := seen[report.CPublishedFoldIndex]; clash {
			t.Errorf(
				"%s and %s now agree on the winning fold index (%d); the counterexample no longer holds and section 2's arrival-order collapse must be re-measured",
				other, key, report.CPublishedFoldIndex,
			)
		}
		seen[report.CPublishedFoldIndex] = key
	}

	if len(seen) < 2 {
		t.Fatal("the two witnesses must disagree on the winning fold index; that disagreement IS the falsification")
	}
	t.Log("FALSIFIED: one tie shape, one fork provenance, two opposite reference-runtime survivors -- no arrival-order collapse repairs the condense-class differences")
}

// TestCondenseTieArrivalOrderVersionSetIsSingleton records WHY the arrival
// order cannot decide these ties: the reference runtime never collapses them,
// because it never holds two versions there at all.
//
// stack_node_add_link's equivalence branch (stack.c:200-247) only runs when two
// links already connect one pair of stack nodes. At every witness below the
// reference runtime folds exactly ONE root into finished_tree (|V| = 1), so
// that branch never executes. The compact core's second derivation therefore
// has no counterpart to arrive before or after; it is a candidate the reference
// runtime never built. Ranking a real candidate against one that does not exist
// is not an ordering problem, which is why an order rule cannot repair it.
func TestCondenseTieArrivalOrderVersionSetIsSingleton(t *testing.T) {
	if !gotreesitter.DerivationSetCensusBuilt() {
		t.Fatal("derivation-set census is not compiled into the library; build with -tags gts_derivation_set_census")
	}

	type witness struct {
		Language string
		CName    string
		Lang     func() *gotreesitter.Language
		Name     string
		Source   string
	}
	witnesses := []witness{
		{"apex", "apex", grammars.ApexLanguage, "class_literal_alias",
			"public class C {\n  void m() {\n    Object t = RecordPage.class;\n  }\n}\n"},
		{"objc", "objc", grammars.ObjcLanguage, "protocol_argument",
			"@interface CallbackClient : NSObject <ClientProtocol>\n@end\n"},
		{"ada", "ada", grammars.AdaLanguage, "bare_aggregate_assignment_material_election",
			"procedure P is\nbegin\n   R := (F => 1, G => 2);\nend;\n"},
		{"perl", "perl", grammars.PerlLanguage, "unshift_two_args",
			"unshift @found, $_;\n"},
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

	for _, w := range witnesses {
		report, err := runDerivationSetDifferential(w.Language, w.CName, languages[w.Language], cLanguages[w.CName], w.Name, []byte(w.Source))
		if err != nil {
			t.Fatalf("%s/%s: %v", w.Language, w.Name, err)
		}
		key := w.Language + "/" + w.Name
		t.Logf("SINGLETON-V %-52s |D|=%d |V|=%d c-version-count-max=%d", key, report.CompactSetSize, report.CVersionSetSize, report.CVersionCountMax)
		if report.Skipped != "" {
			t.Fatalf("%s: no comparison ran: %s", key, report.Skipped)
		}
		if report.CVersionSetSize != 1 {
			t.Errorf("%s: |V|=%d, want 1 -- the reference runtime is expected to fold exactly one root here", key, report.CVersionSetSize)
		}
		if report.CompactSetSize <= report.CVersionSetSize {
			t.Errorf("%s: |D|=%d is not larger than |V|=%d; this witness no longer shows a surplus compact candidate", key, report.CompactSetSize, report.CVersionSetSize)
		}
	}
	t.Log("the reference runtime folds one root at every witness, so stack_node_add_link's equivalence branch never runs there")
}
