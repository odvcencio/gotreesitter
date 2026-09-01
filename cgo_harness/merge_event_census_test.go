//go:build cgo && treesitter_c_parity && gts_merge_census

package cgoharness

import (
	"fmt"
	"strings"
	"testing"

	sitter "github.com/tree-sitter/go-tree-sitter"

	gotreesitter "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
)

// Stage M0 receipts for spec.merge-time-election.v1.
//
// Reproduce with:
//
//	cd cgo_harness
//	GOFLAGS=-buildvcs=false GTS_PARITY_ALLOW_HOST=1 \
//	  GTS_PARITY_C_REF_BUILD_CACHE=<cache> CGO_ENABLED=1 \
//	  go test -tags "cgo treesitter_c_parity gts_merge_census" \
//	  -run 'TestMergeEventCensus' -count=1 -v .
//
// The gts_merge_census tag is what compiles the production-side census into
// the library at all. Without it the library has no instrument, so every
// shipped gate runs against exactly the code main ships
// (merge_event_census_inert_test.go ratchets that).

// mergeCensusOracleForTest builds the instrumented reference runtime once per
// test binary run.
var mergeCensusOracleCache *mergeCensusCOracle

func TestMergeCensusTotalsAggregatesPhysicalHeadMergeTelemetry(t *testing.T) {
	totals := &mergeCensusTotals{}
	totals.add(mergeCensusRow{
		CompactAccepted: true,
		Go: gotreesitter.MergeEventCensusCounts{
			CompactPhysicalHeadMergeAttempts:   3,
			CompactPhysicalHeadMergeSuccesses:  2,
			CompactPhysicalHeadMergeInputLinks: 5,
		},
	})
	totals.add(mergeCensusRow{
		CompactAccepted: true,
		Go: gotreesitter.MergeEventCensusCounts{
			CompactPhysicalHeadMergeAttempts:   7,
			CompactPhysicalHeadMergeSuccesses:  4,
			CompactPhysicalHeadMergeInputLinks: 9,
		},
	})
	if totals.CompactPhysicalAttempts != 10 || totals.CompactPhysicalSuccesses != 6 ||
		totals.CompactPhysicalInputLinks != 14 {
		t.Fatalf("physical merge totals=%d/%d/%d, want 10/6/14",
			totals.CompactPhysicalAttempts, totals.CompactPhysicalSuccesses, totals.CompactPhysicalInputLinks,
		)
	}
}

func mergeCensusOracleForTest(t *testing.T) *mergeCensusCOracle {
	t.Helper()
	if mergeCensusOracleCache != nil {
		return mergeCensusOracleCache
	}
	repoRoot, err := mergeCensusRepoRoot()
	if err != nil {
		t.Fatalf("resolve repository root: %v", err)
	}
	oracle, err := mergeCensusBuildCOracle(repoRoot)
	if err != nil {
		t.Fatalf("build the instrumented reference runtime: %v", err)
	}
	t.Cleanup(func() {
		oracle.Cleanup()
		mergeCensusOracleCache = nil
	})
	t.Logf(
		"instrumented reference runtime: source=%s runtime-sha=%s patch-sha=%s driver-sha=%s",
		oracle.RuntimeDir, oracle.RuntimeSHA[:16], oracle.PatchSHA[:16], oracle.DriverSHA[:16],
	)
	mergeCensusOracleCache = oracle
	return oracle
}

// mergeCensusBaselineConstructed pins the constructed-source portion of the
// M0 baseline. Constructed sources are compiled into this package, so every
// host reproduces these numbers exactly.
//
// Every number here is a MEASUREMENT pinned after the fact, not a prediction.
// Stage M1 must move Ratio toward 1 and must not raise
// SourcesWhereGoOverMerges above zero; a change in any other field is a
// change in the lane's ground truth and must be re-adjudicated, not
// re-fitted.
var mergeCensusBaselineConstructed = map[string]struct {
	Sources int
	// CMergeSuccesses is M_c, the reference runtime's collapsed pairs.
	CMergeSuccesses uint64
	// GoSuccesses is M_p, production's collapsed pairs.
	GoSuccesses uint64
	// RefuseNoGSSHead, RefuseScoreOrShifted, RefuseDistinctShapes and
	// LinkPayloadShallowWouldAccept are the four refusal classes stage M1
	// must answer for. The middle two are items 1 and 2 of spec section 1.2;
	// the last is item 3's cost; the first has no counterpart in the
	// reference runtime at all.
	RefuseNoGSSHead               uint64
	RefuseScoreOrShifted          uint64
	RefuseDistinctShapes          uint64
	LinkPayloadShallowWouldAccept uint64
	// SourcesWhereGoOverMerges is gate G6's stop-the-line class: production
	// collapsing MORE versions than the reference runtime.
	SourcesWhereGoOverMerges int
	// SourcesWhereCMergesAndGoDoesNot is the lane's headline class.
	SourcesWhereCMergesAndGoDoesNot int
}{
	"apex": {Sources: 25, CMergeSuccesses: 31, GoSuccesses: 1, RefuseNoGSSHead: 53, RefuseScoreOrShifted: 0, RefuseDistinctShapes: 53, LinkPayloadShallowWouldAccept: 0, SourcesWhereGoOverMerges: 0, SourcesWhereCMergesAndGoDoesNot: 5},
	"perl": {Sources: 17, CMergeSuccesses: 57, GoSuccesses: 0, RefuseNoGSSHead: 43, RefuseScoreOrShifted: 0, RefuseDistinctShapes: 0, LinkPayloadShallowWouldAccept: 0, SourcesWhereGoOverMerges: 0, SourcesWhereCMergesAndGoDoesNot: 7},
	// PR #708 elects Ada aggregate conflicts before the GLR fork. This removes
	// five no-GSS-head opportunities without changing merges or other gates.
	"ada": {Sources: 23, CMergeSuccesses: 47, GoSuccesses: 2, RefuseNoGSSHead: 30, RefuseScoreOrShifted: 92, RefuseDistinctShapes: 6, LinkPayloadShallowWouldAccept: 0, SourcesWhereGoOverMerges: 0, SourcesWhereCMergesAndGoDoesNot: 5},
	// Clean-suffix reset removes four redundant Kotlin merges. Exact C tree
	// parity remains pinned by TestKotlinRecoverySuffixSourcesMatchC.
	"kotlin": {Sources: 13, CMergeSuccesses: 54, GoSuccesses: 8, RefuseNoGSSHead: 2, RefuseScoreOrShifted: 0, RefuseDistinctShapes: 0, LinkPayloadShallowWouldAccept: 8, SourcesWhereGoOverMerges: 0, SourcesWhereCMergesAndGoDoesNot: 3},
	"python": {Sources: 26, CMergeSuccesses: 2, GoSuccesses: 0, RefuseNoGSSHead: 9, RefuseScoreOrShifted: 0, RefuseDistinctShapes: 0, LinkPayloadShallowWouldAccept: 0, SourcesWhereGoOverMerges: 0, SourcesWhereCMergesAndGoDoesNot: 2},
}

// The M0 pinned aggregate over the five A3 sweep corpora's constructed
// sources. M_p/M_c = 11/191 = 0.0576. That ratio is the lane's progress
// number alongside D0's 32 set differences: stage M1 onward drives it toward
// 1, and gate G6 forbids any source where production merges more than the
// reference runtime.
const (
	mergeCensusBaselineCMerges  uint64 = 191
	mergeCensusBaselineGoMerges uint64 = 11
	// mergeCensusBaselineSources is the constructed-source denominator, the
	// same 104 sources D0 measures.
	mergeCensusBaselineSources = 104
)

// TestMergeEventCensusBaseline publishes the M0 baseline: how many merges the
// reference runtime performs, how many production performs, and which gate
// refuses the rest, for the five A3 sweep corpora and the real corpus.
//
// The five corpora are D0's own corpora, source for source
// (derivationSetCensusLanguages), so the two censuses share a denominator.
func TestMergeEventCensusBaseline(t *testing.T) {
	if !gotreesitter.MergeEventCensusBuilt() {
		t.Fatal("the merge-event census is not compiled into the library; build with -tags gts_merge_census")
	}
	oracle := mergeCensusOracleForTest(t)

	grand := &mergeCensusTotals{}
	grandConstructed := &mergeCensusTotals{}
	var rows []string

	for _, entry := range derivationSetCensusLanguages() {
		lang := entry.Language()
		constructed := entry.Constructed()
		real := derivationSetLoadRealCorpus(t, entry.RealDir)

		constructedTotals := &mergeCensusTotals{}
		realTotals := &mergeCensusTotals{}

		for _, group := range []struct {
			Sources []a3CertificationSweepSource
			Totals  *mergeCensusTotals
			Grand   []*mergeCensusTotals
		}{
			{constructed, constructedTotals, []*mergeCensusTotals{grandConstructed, grand}},
			{real, realTotals, []*mergeCensusTotals{grand}},
		} {
			if len(group.Sources) == 0 {
				continue
			}
			cRows, err := mergeCensusRunC(oracle, entry.Name, group.Sources)
			if err != nil {
				t.Fatalf("%s: %v", entry.Name, err)
			}
			for index, source := range group.Sources {
				row := runMergeEventCensusRow(entry.Name, lang, source.Name, source.Source, cRows[index])
				group.Totals.add(row)
				for _, target := range group.Grand {
					target.add(row)
				}
				mergeCensusLogRow(t, row)
			}
		}

		rows = append(rows, mergeCensusFormatTotals(entry.Name+" constructed", constructedTotals))
		rows = append(rows, mergeCensusFormatTotals(entry.Name+" real       ", realTotals))

		want, pinned := mergeCensusBaselineConstructed[entry.Name]
		if !pinned {
			t.Errorf("%s: no pinned constructed baseline", entry.Name)
			continue
		}
		if constructedTotals.Sources != want.Sources {
			t.Errorf("%s: constructed source count %d, want %d", entry.Name, constructedTotals.Sources, want.Sources)
		}
		if constructedTotals.CMergeSuccesses != want.CMergeSuccesses {
			t.Errorf("%s: reference-runtime merges %d, want %d", entry.Name, constructedTotals.CMergeSuccesses, want.CMergeSuccesses)
		}
		if constructedTotals.GoSuccesses != want.GoSuccesses {
			t.Errorf("%s: production merges %d, want %d", entry.Name, constructedTotals.GoSuccesses, want.GoSuccesses)
		}
		if constructedTotals.RefuseNoGSSHead != want.RefuseNoGSSHead {
			t.Errorf("%s: no-packed-head refusals %d, want %d", entry.Name, constructedTotals.RefuseNoGSSHead, want.RefuseNoGSSHead)
		}
		if constructedTotals.SourcesWhereCMergesAndGoDoesNot != want.SourcesWhereCMergesAndGoDoesNot {
			t.Errorf("%s: sources where the reference runtime merges and production does not %d, want %d", entry.Name, constructedTotals.SourcesWhereCMergesAndGoDoesNot, want.SourcesWhereCMergesAndGoDoesNot)
		}
		if constructedTotals.RefuseScoreOrShifted != want.RefuseScoreOrShifted {
			t.Errorf("%s: score-or-shifted refusals %d, want %d", entry.Name, constructedTotals.RefuseScoreOrShifted, want.RefuseScoreOrShifted)
		}
		if constructedTotals.RefuseDistinctShapes != want.RefuseDistinctShapes {
			t.Errorf("%s: distinct-shape refusals %d, want %d", entry.Name, constructedTotals.RefuseDistinctShapes, want.RefuseDistinctShapes)
		}
		if constructedTotals.LinkPayloadShallowWouldAccept != want.LinkPayloadShallowWouldAccept {
			t.Errorf("%s: shallow-would-accept comparisons %d, want %d", entry.Name, constructedTotals.LinkPayloadShallowWouldAccept, want.LinkPayloadShallowWouldAccept)
		}
		if constructedTotals.SourcesWhereGoOverMerges != want.SourcesWhereGoOverMerges {
			t.Errorf("%s: sources where production over-merges %d, want %d", entry.Name, constructedTotals.SourcesWhereGoOverMerges, want.SourcesWhereGoOverMerges)
		}
	}

	t.Log("M0 BASELINE CENSUS -- merge events per language")
	for _, row := range rows {
		t.Log(row)
	}
	t.Log(mergeCensusFormatTotals("TOTAL constructed", grandConstructed))
	t.Log(mergeCensusFormatTotals("TOTAL all corpora", grand))
	t.Logf("MERGE RATIO M_p/M_c (constructed)=%s (all corpora)=%s", grandConstructed.ratioText(), grand.ratioText())
	t.Logf("REFUSAL BREAKDOWN (constructed): %s", grandConstructed.refusalLine())
	t.Logf("TIER-2 LINK PAYLOAD (constructed): %s", grandConstructed.linkPayloadLine())

	// The pinned aggregate. A fall in M_c or a rise in M_p is the point of
	// stages M1 onward, but the pin moves with a receipt, never silently.
	if grandConstructed.Sources != mergeCensusBaselineSources {
		t.Errorf("constructed source count %d, pinned at %d", grandConstructed.Sources, mergeCensusBaselineSources)
	}
	if grandConstructed.CMergeSuccesses != mergeCensusBaselineCMerges {
		t.Errorf(
			"reference-runtime merge total M_c is %d, pinned at %d -- M_c is a property of the reference runtime and must not move at all unless the pinned grammars or the pinned runtime moved",
			grandConstructed.CMergeSuccesses, mergeCensusBaselineCMerges,
		)
	}
	if grandConstructed.GoSuccesses != mergeCensusBaselineGoMerges {
		t.Errorf(
			"production merge total M_p is %d, pinned at %d -- a RISE is the point of stages M1 onward, but the pin must move with the receipt, never silently",
			grandConstructed.GoSuccesses, mergeCensusBaselineGoMerges,
		)
	}

	// Gate G6 in advance: a source where production collapses MORE versions
	// than the reference runtime is a stop-the-line over-merge from stage M1
	// on. M0 pins the baseline at zero so the gate has a floor.
	if grand.SourcesWhereGoOverMerges != 0 {
		t.Errorf(
			"OVER-MERGE: %d sources where production merges more than the reference runtime; the M0 baseline is zero and gate G6 is stop-the-line",
			grand.SourcesWhereGoOverMerges,
		)
	}
	if grand.CompactPhysicalAttempts != 0 || grand.CompactPhysicalSuccesses != 0 ||
		grand.CompactPhysicalInputLinks != 0 {
		t.Errorf(
			"accepted corpus physical merges changed: attempts=%d successes=%d input-links=%d, want all zero",
			grand.CompactPhysicalAttempts, grand.CompactPhysicalSuccesses, grand.CompactPhysicalInputLinks,
		)
	}
}

func mergeCensusFormatTotals(label string, t *mergeCensusTotals) string {
	return fmt.Sprintf(
		"%-22s sources=%3d M_c=%6d M_p=%6d ratio=%-8s c-attempts=%7d go-attempts=%7d over-merge-sources=%d c-merges-go-does-not=%d | refusals: %s | tier2: %s | compact: accepted=%d union-attempts=%d union-appends=%d physical-attempts=%d physical-successes=%d physical-input-links=%d",
		label, t.Sources, t.CMergeSuccesses, t.GoSuccesses, t.ratioText(),
		t.CMergeAttempts, t.GoAttempts, t.SourcesWhereGoOverMerges, t.SourcesWhereCMergesAndGoDoesNot,
		t.refusalLine(), t.linkPayloadLine(),
		t.CompactAccepted, t.CompactUnionAttempt, t.CompactUnionAppend,
		t.CompactPhysicalAttempts, t.CompactPhysicalSuccesses, t.CompactPhysicalInputLinks,
	)
}

func mergeCensusLogRow(t *testing.T, row mergeCensusRow) {
	t.Helper()
	if !testing.Verbose() {
		return
	}
	ratio := "undefined"
	if value, ok := row.MergeRatio(); ok {
		ratio = fmt.Sprintf("%.4f", value)
	}
	t.Logf(
		"%s/%s: M_c=%d M_p=%d ratio=%s c-status=%s c-link-union(att=%d dup=%d prec=%d rec=%d app=%d rej=%d) go-refusals(score=%d shapes=%d clean=%d state=%d cost=%d) tier2-shallow-would-accept=%d compact-physical(att=%d ok=%d links=%d)",
		row.Language, row.Name, row.C.MergeSuccesses, row.Go.Successes, ratio, row.C.Status,
		row.C.LinkUnionAttempts, row.C.LinkUnionDuplicate, row.C.LinkUnionPrecedence,
		row.C.LinkUnionRecursive, row.C.LinkUnionAppended, row.C.LinkUnionRejected,
		row.Go.RefuseScoreOrShifted, row.Go.RefuseDistinctShapes, row.Go.RefuseCleanZero,
		row.Go.RefuseStateOrOffset, row.Go.RefuseErrorCost,
		row.Go.LinkPayloadShallowWouldAccept,
		row.Go.CompactPhysicalHeadMergeAttempts,
		row.Go.CompactPhysicalHeadMergeSuccesses,
		row.Go.CompactPhysicalHeadMergeInputLinks,
	)
}

// mergeCensusFooIntWitness is one source in the Foo[int](a) discriminator
// measurement.
type mergeCensusFooIntWitness struct {
	Name   string
	Source string
	// Frame marks the bracket-free control whose counters are subtracted from
	// every other row.
	Frame bool
	Note  string
}

// TestMergeEventCensusFooIntWitness takes the discriminator receipt section 3
// of spec.merge-time-election.v1 demands before any compact collapse lands.
//
// The compact core defers a shallow-class precedence tie because a first-wins
// collapse under unproven order published the wrong tree for Go `Foo[int](a)`:
// call_expression(index_expression) against
// type_conversion_expression(generic_type) (core.go:1989-2002). What the
// reference runtime does with that pair was UNMEASURED. Exactly three
// behaviours are consistent with everything known, and the work-count patch's
// GTSLinkUnionOutcome counters discriminate them:
//
//  1. The reference runtime never builds the pair. The witness adds no
//     link-union traffic. Then the deferral is not a workaround for a missing
//     merge and the surplus arm is upstream.
//  2. The reference runtime packs both links and elects at the pop.
//     predecessor_link_union_alternate_appended rises. Then stage M3's pop
//     election replaces the deferral for this class with no order proof.
//  3. The reference runtime collapses at the same-node-pair branch.
//     predecessor_link_union_precedence_replaced rises (precedence decides)
//     or only predecessor_link_union_duplicate_noop rises (incumbency
//     decides).
//
// The counters are parse-global, so the measurement needs a control. The
// control is `_ = g(a)`: the same statement frame with no bracket, therefore
// no generic-instantiation conflict at all. `_ = a[b](c)` is NOT a control:
// it carries the SAME conflict, because `a[b]` also reads as a generic
// instantiation. The measurement below shows the two produce identical
// counters, which is itself part of the receipt.
func TestMergeEventCensusFooIntWitness(t *testing.T) {
	if !gotreesitter.MergeEventCensusBuilt() {
		t.Fatal("the merge-event census is not compiled into the library; build with -tags gts_merge_census")
	}
	oracle := mergeCensusOracleForTest(t)

	witnesses := []mergeCensusFooIntWitness{
		{
			Name: "frame_only_g_call", Source: "package p\n\nfunc f() {\n\t_ = g(a)\n}\n", Frame: true,
			Note: "control: the same statement frame with no bracket, so no generic-instantiation conflict",
		},
		{
			Name: "foo_int_a", Source: "package p\n\nfunc f() {\n\t_ = Foo[int](a)\n}\n",
			Note: "the witness: call_expression(index_expression) against type_conversion_expression(generic_type)",
		},
		{
			Name: "index_call_a_b_c", Source: "package p\n\nfunc f() {\n\t_ = a[b](c)\n}\n",
			Note: "the same conflict written with plain identifiers, kept to show it is not a control",
		},
		{
			Name: "bare_instantiation", Source: "package p\n\nfunc f() {\n\t_ = Foo[int]\n}\n",
			Note: "the instantiation without the call, which isolates the bracket half of the conflict",
		},
	}

	sources := make([]a3CertificationSweepSource, len(witnesses))
	for index, witness := range witnesses {
		sources[index] = a3CertificationSweepSource{Name: witness.Name, Source: []byte(witness.Source)}
	}
	cRows, err := mergeCensusRunC(oracle, "go", sources)
	if err != nil {
		t.Fatalf("measure the Foo[int](a) witness on the reference runtime: %v", err)
	}

	var frame mergeCensusCRow
	for index, witness := range witnesses {
		if witness.Frame {
			frame = cRows[index]
		}
		if cRows[index].Status != "ok" {
			t.Fatalf("%s: reference runtime status %q", witness.Name, cRows[index].Status)
		}
	}

	lang := grammars.GoLanguage()
	cLang, err := COracleLanguage("go")
	if err != nil {
		t.Fatalf("load go C oracle: %v", err)
	}

	verdicts := make([]string, len(witnesses))
	for index, witness := range witnesses {
		row := runMergeEventCensusRow("go", lang, witness.Name, []byte(witness.Source), cRows[index])
		t.Logf("WITNESS %-20s -- %s", witness.Name, witness.Note)
		t.Logf("    merges              : M_c=%d M_p=%d", row.C.MergeSuccesses, row.Go.Successes)
		t.Logf(
			"    reference link union: attempts=%d duplicate_noop=%d precedence_replaced=%d recursive_changed=%d alternate_appended=%d rejected=%d",
			row.C.LinkUnionAttempts, row.C.LinkUnionDuplicate, row.C.LinkUnionPrecedence,
			row.C.LinkUnionRecursive, row.C.LinkUnionAppended, row.C.LinkUnionRejected,
		)
		if !witness.Frame {
			t.Logf(
				"    delta over control  : attempts=%+d duplicate_noop=%+d precedence_replaced=%+d recursive_changed=%+d alternate_appended=%+d",
				int64(row.C.LinkUnionAttempts)-int64(frame.LinkUnionAttempts),
				int64(row.C.LinkUnionDuplicate)-int64(frame.LinkUnionDuplicate),
				int64(row.C.LinkUnionPrecedence)-int64(frame.LinkUnionPrecedence),
				int64(row.C.LinkUnionRecursive)-int64(frame.LinkUnionRecursive),
				int64(row.C.LinkUnionAppended)-int64(frame.LinkUnionAppended),
			)
		}
		t.Logf(
			"    production refusals : score-or-shifted=%d distinct-shapes=%d clean-zero=%d state-or-offset=%d error-cost=%d",
			row.Go.RefuseScoreOrShifted, row.Go.RefuseDistinctShapes, row.Go.RefuseCleanZero,
			row.Go.RefuseStateOrOffset, row.Go.RefuseErrorCost,
		)
		t.Logf(
			"    production tier 2   : tests=%d deep-refuse=%d shallow-would-accept=%d",
			row.Go.LinkPayloadTests, row.Go.LinkPayloadDeepRefusals, row.Go.LinkPayloadShallowWouldAccept,
		)
		t.Logf("    reference published : %s", mergeCensusPublishedShape(cLang, []byte(witness.Source)))
		if witness.Frame {
			verdicts[index] = "control row; no verdict"
		} else {
			verdicts[index] = mergeCensusFooIntVerdict(row.C, frame)
		}
		t.Logf("    VERDICT: %s", verdicts[index])
	}

	t.Log("FOO[INT](A) RECEIPT -- spec.merge-time-election.v1 section 3")
	for index, witness := range witnesses {
		t.Logf("  %-20s %s", witness.Name, verdicts[index])
	}

	// The receipt must be decidable, not merely reported. A verdict that stays
	// ambiguous is a real outcome and the test says so; it does not choose.
	for index, witness := range witnesses {
		if witness.Frame {
			continue
		}
		if strings.HasPrefix(verdicts[index], "AMBIGUOUS") {
			t.Logf("%s: the discriminator is ambiguous at this witness; stage M3 must not assume an outcome for it", witness.Name)
		}
	}
}

// mergeCensusPublishedShape renders the reference runtime's published tree in
// one line, so the receipt records WHICH reading the reference runtime chose
// beside the counters that show HOW it chose.
func mergeCensusPublishedShape(cLang *sitter.Language, source []byte) string {
	parser := sitter.NewParser()
	defer parser.Close()
	if err := parser.SetLanguage(cLang); err != nil {
		return "<set-language error: " + err.Error() + ">"
	}
	tree := parser.Parse(source, nil)
	if tree == nil {
		return "<no tree>"
	}
	defer tree.Close()
	root := tree.RootNode()
	if root == nil {
		return "<no root>"
	}
	return root.ToSexp()
}

// mergeCensusFooIntVerdict names which of the three consistent reference
// behaviours the counters show, or states plainly that the reading is
// ambiguous. Every count is a delta over the bracket-free control, so the
// verdict describes the witness rather than the statement frame.
func mergeCensusFooIntVerdict(row, frame mergeCensusCRow) string {
	appended := int64(row.LinkUnionAppended) - int64(frame.LinkUnionAppended)
	duplicate := int64(row.LinkUnionDuplicate) - int64(frame.LinkUnionDuplicate)
	precedence := int64(row.LinkUnionPrecedence) - int64(frame.LinkUnionPrecedence)
	recursive := int64(row.LinkUnionRecursive) - int64(frame.LinkUnionRecursive)

	suffix := ""
	if duplicate > 0 && appended > 0 {
		suffix = fmt.Sprintf(
			" The same region also adds %d duplicate no-ops, so the reference runtime collapses some pairs by incumbency while packing others; the counters are parse-global and cannot say which node each one landed on.",
			duplicate,
		)
	}

	switch {
	case appended <= 0 && duplicate <= 0 && precedence <= 0 && recursive <= 0:
		return "BEHAVIOUR 1 (pair never built): the witness adds no link-union traffic over the bracket-free control, so the reference runtime never held two links to collapse; the compact deferral is not a workaround for a missing merge and the surplus arm is upstream"
	case precedence > 0 && appended <= 0:
		return fmt.Sprintf(
			"BEHAVIOUR 3 (collapsed by precedence): the witness adds %d precedence replacements and no alternate append, so strict dynamic precedence decides; compact must verify its scoreDelta accounting against the reference sum before porting the comparison",
			precedence,
		)
	case appended > 0 && precedence == 0:
		return fmt.Sprintf(
			"BEHAVIOUR 2 (packed, elected at the pop): the witness adds %d alternate link appends and ZERO precedence replacements, so the reference runtime packs both links and the first spanning pop elects; stage M3's pop election replaces the deferral for this class with no order proof needed.%s",
			appended, suffix,
		)
	case duplicate > 0 && appended <= 0 && precedence <= 0:
		return fmt.Sprintf(
			"BEHAVIOUR 3 (collapsed by incumbency): the witness adds %d duplicate no-ops and neither an append nor a precedence replacement, so the incumbent link survives the tie; the discriminator is version arrival order and compact must reproduce it structurally or keep the deferral",
			duplicate,
		)
	default:
		return fmt.Sprintf(
			"AMBIGUOUS: the witness adds appends=%d duplicate=%d precedence=%d recursive=%d, which matches more than one of the three consistent behaviours; reported as ambiguous rather than chosen",
			appended, duplicate, precedence, recursive,
		)
	}
}
