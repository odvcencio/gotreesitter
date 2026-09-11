//go:build gts_parsercorephase0

package gotreesitter

// C4 stage 2 runtime equivalence obligations (spec.c4-bytecode-isa.v1
// section 5, R1-R6) plus the R6 pass-mix publication.
//
// The gate discipline is differential: every canonical fixture runs twice on
// the same runner shape, once with the corridor lane off and once with it on,
// and every observable the scheduler produces must be byte-identical. That is
// the strongest available R4 fork-boundary proof for a lane that shares the
// scheduler struct: the corridor exits and re-enters hundreds to thousands of
// times per fixture, so identical branchOrder, creationSeq, dispatch, token,
// and work vectors at the end of the parse can only hold when every one of
// those handoffs preserved state exactly.

import (
	"encoding/json"
	"os"
	"testing"

	core "github.com/odvcencio/gotreesitter/internal/parsercorephase0"
)

// corridorFixtureObservation is everything one canonical fixture run exposes
// that the corridor could perturb.
type corridorFixtureObservation struct {
	Fixture        string                          `json:"fixture"`
	DeepTreeSHA256 string                          `json:"deep_tree_sha256"`
	RootEndByte    uint32                          `json:"root_end_byte"`
	RootHasError   bool                            `json:"root_has_error"`
	CoreWork       core.Work                       `json:"core_work"`
	SchedulerWork  DiagnosticParserCoreGenericWork `json:"scheduler_work"`
	Dispatches     uint64                          `json:"dispatches"`
	Tokens         uint64                          `json:"tokens"`
	BranchOrder    uint64                          `json:"branch_order"`
	NextSeq        uint64                          `json:"next_creation_seq"`
	ElectionIndex  int                             `json:"election_index"`
	RawSelected    core.RawSelectedCensus          `json:"raw_selected_internal_census"`
}

func runCorridorFixture(t *testing.T, fixtureID string) corridorFixtureObservation {
	t.Helper()
	fixture := loadDiagnosticParserCoreCanonicalFixture(t, fixtureID)
	runner, err := newParserCoreFreshFullRunner(parserCoreWarmGoScanner, parserCoreFreshFullCanonicalOptions())
	if err != nil {
		t.Fatalf("fresh full runner: %v", err)
	}
	scheduler, tokenSource, err := runner.executeSchedulerOpen(fixture.Source, runner.compact, true)
	if err != nil {
		if tokenSource != nil {
			tokenSource.Close()
		}
		t.Fatalf("execute scheduler: %v", err)
	}
	defer tokenSource.Close()
	derivations, err := runner.compact.Derivations(scheduler.acceptedHead)
	if err != nil || len(derivations) != 1 {
		t.Fatalf("accepted compact derivations=%d err=%v", len(derivations), err)
	}
	rawSelected, err := runner.compact.RawSelectedSubtreeCensus(derivations[0].Payloads)
	if err != nil {
		t.Fatalf("raw selected census: %v", err)
	}
	tree, err := runner.materialize(fixture.Source, runner.compact, scheduler.acceptedHead)
	if err != nil {
		t.Fatalf("materialize: %v", err)
	}
	defer tree.Release()
	digest := requireDiagnosticParserCoreCanonicalTreeDigest(t, tree, runner.lang)

	return corridorFixtureObservation{
		Fixture:        fixtureID,
		DeepTreeSHA256: digest,
		RootEndByte:    tree.root.endByte,
		RootHasError:   tree.root.HasError(),
		CoreWork:       runner.compact.Work(),
		SchedulerWork:  scheduler.work,
		Dispatches:     scheduler.dispatches,
		Tokens:         scheduler.tokens,
		BranchOrder:    scheduler.branchOrder,
		NextSeq:        scheduler.nextSeq,
		ElectionIndex:  scheduler.electionIndex,
		RawSelected:    rawSelected,
	}
}

// TestParserCoreCorridorRuntimeEquivalence is R1 (work-count board
// byte-identical), R2 (deep-tree digests byte-identical), and R4 (fork
// boundary handoff) on every canonical fixture.
func TestParserCoreCorridorRuntimeEquivalence(t *testing.T) {
	for _, admission := range diagnosticParserCoreCanonicalAdmissions {
		admission := admission
		t.Run(admission.id, func(t *testing.T) {
			restoreOff := SetParserCoreCorridorEnabledForTest(false)
			generic := runCorridorFixture(t, admission.id)
			restoreOff()

			restoreOn := SetParserCoreCorridorEnabledForTest(true)
			corridor := runCorridorFixture(t, admission.id)
			restoreOn()

			// The corridor must actually have run, or the gate proves nothing.
			if corridor.SchedulerWork.CorridorPasses == 0 {
				t.Fatalf("corridor lane executed no passes on %s", admission.id)
			}
			if generic.SchedulerWork.CorridorPasses != 0 {
				t.Fatalf("corridor counter is non-zero with the lane off: %d", generic.SchedulerWork.CorridorPasses)
			}

			// R1: the full scheduler work vector, minus the corridor's own
			// coverage counter, must be identical.
			genericWork := generic.SchedulerWork
			corridorWork := corridor.SchedulerWork
			corridorWork.CorridorPasses = 0
			if genericWork != corridorWork {
				t.Fatalf("scheduler work diverged\n generic=%+v\ncorridor=%+v", genericWork, corridorWork)
			}
			if generic.CoreWork != corridor.CoreWork {
				t.Fatalf("compact core work diverged\n generic=%+v\ncorridor=%+v", generic.CoreWork, corridor.CoreWork)
			}

			// R2: byte-identical deep-tree digest and identical root shape.
			if generic.DeepTreeSHA256 != corridor.DeepTreeSHA256 {
				t.Fatalf("deep tree digest diverged: generic=%s corridor=%s", generic.DeepTreeSHA256, corridor.DeepTreeSHA256)
			}
			if generic.DeepTreeSHA256 != admission.deepTreeSHA256 {
				t.Fatalf("deep tree digest=%s want pinned=%s", generic.DeepTreeSHA256, admission.deepTreeSHA256)
			}
			if generic.RootEndByte != corridor.RootEndByte || generic.RootHasError != corridor.RootHasError {
				t.Fatalf("root shape diverged: generic=%+v corridor=%+v", generic, corridor)
			}
			if generic.RawSelected != corridor.RawSelected {
				t.Fatalf("raw selected census diverged: generic=%+v corridor=%+v", generic.RawSelected, corridor.RawSelected)
			}

			// R4: branchOrder and creationSeq order the fork lane's arms, and
			// dispatch/token identity is what the cap protocol authenticates.
			if generic.BranchOrder != corridor.BranchOrder || generic.NextSeq != corridor.NextSeq ||
				generic.Dispatches != corridor.Dispatches || generic.Tokens != corridor.Tokens ||
				generic.ElectionIndex != corridor.ElectionIndex {
				t.Fatalf("fork-boundary identity diverged\n generic=%+v\ncorridor=%+v", generic, corridor)
			}

			// R1 partition invariant: single-header passes are a strict subset
			// of all passes, and corridor passes a strict subset of those.
			if corridorWork.SingleHeaderPasses > corridorWork.Passes {
				t.Fatalf("SingleHeaderPasses=%d exceeds Passes=%d", corridorWork.SingleHeaderPasses, corridorWork.Passes)
			}
			if corridor.SchedulerWork.CorridorPasses > corridorWork.SingleHeaderPasses {
				t.Fatalf("CorridorPasses=%d exceeds SingleHeaderPasses=%d", corridor.SchedulerWork.CorridorPasses, corridorWork.SingleHeaderPasses)
			}
		})
	}
}

// TestParserCoreCorridorReduceShiftIncrementalReceipt checks the first
// superinstruction through the reuse-consuming public path. The initial
// candidate tree must remain byte-exact after a changed-source reparse.
func TestParserCoreCorridorReduceShiftIncrementalReceipt(t *testing.T) {
	if !corridorReduceShiftEnabled() {
		t.Skip("set GTS_C4_REDUCE_SHIFT=1 to run the REDUCE_SHIFT receipt")
	}
	runParserCoreCorridorSuperinstructionIncrementalReceipt(t, "REDUCE_SHIFT")
}

// TestParserCoreCorridorReduceChainIncrementalReceipt checks the guarded
// unary two-step chain through the reuse-consuming public path.
func TestParserCoreCorridorReduceChainIncrementalReceipt(t *testing.T) {
	if !corridorReduceChainEnabled() {
		t.Skip("set GTS_C4_REDUCE_CHAIN=1 to run the REDUCE_CHAIN receipt")
	}
	runParserCoreCorridorSuperinstructionIncrementalReceipt(t, "REDUCE_CHAIN")
}

func runParserCoreCorridorSuperinstructionIncrementalReceipt(t *testing.T, label string) {
	t.Helper()
	restoreCorridor := SetParserCoreCorridorEnabledForTest(true)
	defer restoreCorridor()
	requireCandidateRouteBudgetMB(t, 256)
	fixture := loadDiagnosticParserCoreCanonicalFixture(t, "rewrite")
	candidate := newAdmissionCandidateGoParser(t)
	candidate.SetAdmissionCandidateRoute(true)
	production := newAdmissionCandidateGoParser(t)
	production.SetAdmissionCandidateRoute(false)

	candidateTree, err := candidate.Parse(fixture.Source)
	if err != nil {
		t.Fatalf("candidate fresh parse: %v", err)
	}
	productionTree, err := production.Parse(fixture.Source)
	if err != nil {
		candidateTree.Release()
		t.Fatalf("production fresh parse: %v", err)
	}

	edited := append(append([]byte(nil), fixture.Source...), ' ')
	eof := admissionTestPointAtByte(fixture.Source, len(fixture.Source))
	edit := InputEdit{
		StartByte: uint32(len(fixture.Source)), OldEndByte: uint32(len(fixture.Source)),
		NewEndByte: uint32(len(edited)), StartPoint: eof, OldEndPoint: eof,
		NewEndPoint: Point{Row: eof.Row, Column: eof.Column + 1},
	}
	candidateTree.Edit(edit)
	candidateIncremental, err := candidate.ParseIncremental(edited, candidateTree)
	if err != nil {
		candidateTree.Release()
		productionTree.Release()
		t.Fatalf("candidate incremental parse: %v", err)
	}
	productionTree.Edit(edit)
	productionIncremental, err := production.ParseIncremental(edited, productionTree)
	if err != nil {
		if candidateIncremental != candidateTree {
			candidateIncremental.Release()
		}
		candidateTree.Release()
		productionTree.Release()
		t.Fatalf("production incremental parse: %v", err)
	}

	candidateDigest := requireDiagnosticParserCoreCanonicalTreeDigest(t, candidateIncremental, candidate.language)
	productionDigest := requireDiagnosticParserCoreCanonicalTreeDigest(t, productionIncremental, production.language)
	if candidateDigest != productionDigest {
		t.Fatalf("incremental deep-tree digest diverged: candidate=%s production=%s", candidateDigest, productionDigest)
	}
	candidateRoot, productionRoot := candidateIncremental.RootNode(), productionIncremental.RootNode()
	if candidateRoot.EndByte() != productionRoot.EndByte() || candidateRoot.HasError() != productionRoot.HasError() {
		t.Fatalf("incremental root shape diverged: candidate=%d..%d error=%v production=%d..%d error=%v",
			candidateRoot.StartByte(), candidateRoot.EndByte(), candidateRoot.HasError(),
			productionRoot.StartByte(), productionRoot.EndByte(), productionRoot.HasError())
	}
	candidateRuntime, productionRuntime := candidateIncremental.ParseRuntime(), productionIncremental.ParseRuntime()
	if candidateRuntime.StopReason != productionRuntime.StopReason ||
		candidateRuntime.SourceLen != productionRuntime.SourceLen ||
		candidateRuntime.ExpectedEOFByte != productionRuntime.ExpectedEOFByte ||
		candidateRuntime.RootEndByte != productionRuntime.RootEndByte ||
		candidateRuntime.Truncated != productionRuntime.Truncated ||
		candidateRuntime.TokenSourceEOFEarly != productionRuntime.TokenSourceEOFEarly ||
		candidateRuntime.LastTokenWasEOF != productionRuntime.LastTokenWasEOF {
		t.Fatalf("incremental runtime receipt diverged: candidate=%s production=%s", candidateRuntime.Summary(), productionRuntime.Summary())
	}
	if candidateIncremental.compactMaterialized || productionIncremental.compactMaterialized {
		t.Fatalf("incremental result leaked compact materialization: candidate=%v production=%v", candidateIncremental.compactMaterialized, productionIncremental.compactMaterialized)
	}
	t.Logf("%s incremental receipt digest=%s runtime=%s", label, candidateDigest, candidateRuntime.Summary())

	if candidateIncremental != candidateTree {
		candidateIncremental.Release()
	}
	candidateTree.Release()
	if productionIncremental != productionTree {
		productionIncremental.Release()
	}
	productionTree.Release()
}

// TestParserCoreCorridorPassMixReceipt is obligation R6: stage 2 publishes the
// pass-mix counter per canonical fixture. Setting GTS_C4_PASSMIX_RESULT writes
// the board row; without it the test still asserts the counter is populated
// and internally consistent, so the row can never silently go missing.
func TestParserCoreCorridorPassMixReceipt(t *testing.T) {
	type passMixRow struct {
		Fixture            string  `json:"fixture"`
		Passes             uint64  `json:"passes"`
		SingleHeaderPasses uint64  `json:"single_header_passes"`
		CorridorPasses     uint64  `json:"corridor_passes"`
		SingleHeaderShare  float64 `json:"single_header_share"`
		CorridorShare      float64 `json:"corridor_share"`
		CorridorOfSingle   float64 `json:"corridor_share_of_single_header"`
	}
	type passMixBoard struct {
		Schema string       `json:"schema"`
		Spec   string       `json:"spec"`
		Rows   []passMixRow `json:"rows"`
	}

	board := passMixBoard{Schema: "gts-c4-pass-mix/v1", Spec: "spec.c4-bytecode-isa.v1"}
	restore := SetParserCoreCorridorEnabledForTest(true)
	defer restore()
	for _, admission := range diagnosticParserCoreCanonicalAdmissions {
		observed := runCorridorFixture(t, admission.id)
		work := observed.SchedulerWork
		if work.Passes == 0 || work.SingleHeaderPasses == 0 {
			t.Fatalf("%s: pass-mix counters are empty: %+v", admission.id, work)
		}
		row := passMixRow{
			Fixture:            admission.id,
			Passes:             work.Passes,
			SingleHeaderPasses: work.SingleHeaderPasses,
			CorridorPasses:     work.CorridorPasses,
			SingleHeaderShare:  float64(work.SingleHeaderPasses) / float64(work.Passes),
			CorridorShare:      float64(work.CorridorPasses) / float64(work.Passes),
		}
		if work.SingleHeaderPasses != 0 {
			row.CorridorOfSingle = float64(work.CorridorPasses) / float64(work.SingleHeaderPasses)
		}
		board.Rows = append(board.Rows, row)
		t.Logf("R6 %s: passes=%d single_header=%d (%.1f%%) corridor=%d (%.1f%% of passes, %.1f%% of single-header)",
			admission.id, row.Passes, row.SingleHeaderPasses, 100*row.SingleHeaderShare,
			row.CorridorPasses, 100*row.CorridorShare, 100*row.CorridorOfSingle)
	}
	if path := os.Getenv("GTS_C4_PASSMIX_RESULT"); path != "" {
		data, err := json.MarshalIndent(board, "", "  ")
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, append(data, '\n'), 0o600); err != nil {
			t.Fatal(err)
		}
	}
}

// TestParserCoreCorridorDefaultOn verifies the default route executes corridor passes.
func TestParserCoreCorridorDefaultOn(t *testing.T) {
	if os.Getenv("GTS_C4_CORRIDOR") != "" {
		t.Skip("GTS_C4_CORRIDOR is set in this environment")
	}
	if !parserCoreCorridorEnabled() {
		t.Fatal("corridor lane is off by default")
	}
	observed := runCorridorFixture(t, diagnosticParserCoreCanonicalAdmissions[0].id)
	if observed.SchedulerWork.CorridorPasses == 0 {
		t.Fatal("corridor executed no passes with the default gate")
	}
}

// BenchmarkDiagnosticParserCoreCorridorSchedulerOnly is the stage-2
// measurement lane (spec section 8, stage 2 item 2f). It is the existing
// warm scheduler-only lane with the corridor gate forced on, so an
// order-balanced A/B against
// BenchmarkDiagnosticParserCoreWarmSchedulerOnlyQueryCompile reports the
// corridor's scheduler-family delta on the same fixture and the same
// lifecycle. It is a local proxy only; a gate-grade number needs the
// dedicated quiet host and the sealed-epoch protocol.
func BenchmarkDiagnosticParserCoreCorridorSchedulerOnly(b *testing.B) {
	restore := SetParserCoreCorridorEnabledForTest(true)
	defer restore()
	prepared, err := parserCoreWarmPrepare()
	if err != nil {
		b.Fatal(err)
	}
	scheduler, err := prepared.executeScheduler(prepared.compact, true)
	if err != nil {
		b.Fatal(err)
	}
	if scheduler.work.CorridorPasses == 0 {
		b.Fatal("corridor lane executed no passes in the measurement lane")
	}
	tree, err := prepared.materialize(prepared.compact, scheduler.acceptedHead)
	if err != nil {
		b.Fatal(err)
	}
	parserCoreWarmAdmitAgainstProduction(b, prepared, tree)
	tree.Release()
	b.ReportAllocs()
	b.SetBytes(int64(len(prepared.source)))
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := prepared.executeScheduler(prepared.compact, true); err != nil {
			b.Fatal(err)
		}
	}
}
