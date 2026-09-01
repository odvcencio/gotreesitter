//go:build !gts_no_parsercorephase0

package gotreesitter

import (
	"testing"
	"unsafe"

	core "github.com/odvcencio/gotreesitter/internal/parsercorephase0"
)

// selectCompetingRecoveryLineage is the acceptance half of C's version
// competition: when more than one head accepts, price the competitors and
// publish the cheaper tree instead of declining. These tests drive it through
// a hand-built scheduler to isolate the acceptance policy from the live fork.

type lineageSelectionTable struct{}

func (lineageSelectionTable) Actions(core.StateID, core.Symbol) (core.ActionRow, error) {
	return core.ActionRow{}, nil
}
func (lineageSelectionTable) Goto(core.StateID, core.Symbol) (core.StateID, error) { return 0, nil }
func (lineageSelectionTable) ProductionFields(uint16, int) ([]core.FieldMapEntry, error) {
	return nil, nil
}
func (lineageSelectionTable) ProductionAliases(uint16, int) ([]core.Symbol, error) { return nil, nil }

// newLineageSelectionScheduler builds a scheduler carrying two competing
// accepted heads: one that inserted a MISSING leaf, one that absorbed a span
// into an ERROR region. The spans reproduce the measured php witness, where
// absorbing nine bytes with one visible child costs 609 against the
// insertion's flat 610.
func newLineageSelectionScheduler(t *testing.T, armed bool) *diagnosticParserCoreGenericScheduler {
	t.Helper()
	compact, err := core.New(lineageSelectionTable{}, core.Limits{MaxDerivations: 8, MaxPopPaths: 8})
	if err != nil {
		t.Fatal(err)
	}
	seed, err := compact.Seed(core.StateID(1), 0)
	if err != nil {
		t.Fatalf("Seed: %v", err)
	}
	missingHead, err := compact.ShiftMissingLeaf(seed, core.StateID(2), core.Symbol(3), 16)
	if err != nil {
		t.Fatalf("ShiftMissingLeaf: %v", err)
	}
	absorbed, err := compact.ErrorRegionLeaf(core.Symbol(4), 6, 15, false)
	if err != nil {
		t.Fatalf("ErrorRegionLeaf: %v", err)
	}
	absorbHead, err := compact.ErrorRegionResume(seed, core.StateID(3), 6, 15, []core.SubtreeID{absorbed})
	if err != nil {
		t.Fatalf("ErrorRegionResume: %v", err)
	}

	lang := &Language{SymbolMetadata: make([]SymbolMetadata, 16)}
	for i := range lang.SymbolMetadata {
		lang.SymbolMetadata[i] = SymbolMetadata{Visible: true, Named: true}
	}
	scheduler := &diagnosticParserCoreGenericScheduler{compact: compact}
	scheduler.tokenSource = &dfaTokenSource{language: lang}
	scheduler.options.materializationSource = []byte("<?php namespace ; ?>")
	scheduler.options.allowCompactRecoveryLineageSelection = armed
	scheduler.headers = make([]diagnosticParserCoreHeader, 2)
	scheduler.headers[0].head = missingHead
	scheduler.headers[1].head = absorbHead
	for index := range scheduler.headers {
		scheduler.headers[index].markRecoveryLineage()
	}
	return scheduler
}

// TestSelectCompetingRecoveryLineagePublishesTheCheaperTree is the claim: on
// the php witness C keeps the ERROR tree, and so must this.
func TestSelectCompetingRecoveryLineagePublishesTheCheaperTree(t *testing.T) {
	scheduler := newLineageSelectionScheduler(t, true)
	winner, resolved, err := scheduler.selectCompetingRecoveryLineage()
	if err != nil {
		t.Fatalf("select: %v", err)
	}
	if !resolved {
		t.Fatal("two priceable recovery lineages were not resolved")
	}
	if winner != 1 {
		t.Fatalf("winner=%d, want 1: the 609 absorb beats the 610 insertion", winner)
	}
}

// TestSelectCompetingRecoveryLineageDeclinesUnarmed proves the capability
// keeps uncertified parses on the conservative path.
func TestSelectCompetingRecoveryLineageDeclinesUnarmed(t *testing.T) {
	scheduler := newLineageSelectionScheduler(t, false)
	_, resolved, err := scheduler.selectCompetingRecoveryLineage()
	if err != nil {
		t.Fatalf("select: %v", err)
	}
	if resolved {
		t.Fatal("an unarmed scheduler resolved a competing frontier")
	}
}

func TestCompleteAcceptancePricesRecoveryFrontierBeforeCollapse(t *testing.T) {
	scheduler := newLineageSelectionScheduler(t, true)
	scheduler.recoveryIsolation = true
	scheduler.options.ReceiptMode = DiagnosticParserCoreReceiptSummary
	scheduler.receipt = &scheduler.receiptBacking
	for index := range scheduler.headers {
		scheduler.headers[index].accepted = true
	}
	scheduler.work.Accepts = uint64(len(scheduler.headers))
	eof := uint32(len(scheduler.options.materializationSource))
	scheduler.token = Token{StartByte: eof, EndByte: eof}
	want := scheduler.headers[1].head

	if err := scheduler.completeAcceptance(); err != nil {
		t.Fatalf("completeAcceptance: %v", err)
	}
	if scheduler.acceptedHead != want || len(scheduler.headers) != 1 {
		t.Fatalf("accepted head=%v headers=%d, want absorb head %v", scheduler.acceptedHead, len(scheduler.headers), want)
	}
	acceptance := scheduler.receipt.Acceptance
	if acceptance == nil || acceptance.Accepts != 2 || acceptance.Work.Accepts != 2 ||
		acceptance.Work.RecoveryLineageSelections != 1 {
		t.Fatalf("acceptance=%+v, want two accepts and one recovery selection", acceptance)
	}
}

// TestSelectCompetingRecoveryLineageDeclinesNonRecoveryHead proves the route
// refuses ordinary grammar ambiguity. Error cost answers "which recovery is
// cheaper", which is not the question a plain ambiguous frontier is asking, so
// pricing one would silently substitute a different decision procedure.
func TestSelectCompetingRecoveryLineageDeclinesNonRecoveryHead(t *testing.T) {
	scheduler := newLineageSelectionScheduler(t, true)
	scheduler.headers[1].clearRecoveryLineage()
	_, resolved, err := scheduler.selectCompetingRecoveryLineage()
	if err != nil {
		t.Fatalf("select: %v", err)
	}
	if resolved {
		t.Fatal("a frontier containing a non-recovery head was resolved by error cost")
	}
}

// TestSelectCompetingRecoveryLineageChargesOpenSegments proves the header's
// open-recovery count reaches pricing. Charging the missing lineage nothing
// and the absorbing lineage one paused segment inverts the winner, because the
// margin between them is a single point.
func TestSelectCompetingRecoveryLineageChargesOpenSegments(t *testing.T) {
	scheduler := newLineageSelectionScheduler(t, true)
	// A paused head carries one open-recovery segment, derived rather than
	// stored, so pricing charges it RecoveryCostPerRecovery.
	scheduler.headers[1].paused = true

	winner, resolved, err := scheduler.selectCompetingRecoveryLineage()
	if err != nil {
		t.Fatalf("select: %v", err)
	}
	if !resolved {
		t.Fatal("frontier was not resolved")
	}
	if winner != 0 {
		t.Fatalf("winner=%d, want 0: charging the absorb one paused segment (+500) puts it above the insertion", winner)
	}
}

// TestSelectCompetingRecoveryLineageDeclinesWithoutSource proves pricing
// refuses to run without the source text. Rows are read from it, and a missing
// source would silently price every ERROR region at zero rows.
func TestSelectCompetingRecoveryLineageDeclinesWithoutSource(t *testing.T) {
	scheduler := newLineageSelectionScheduler(t, true)
	scheduler.options.materializationSource = nil
	_, resolved, err := scheduler.selectCompetingRecoveryLineage()
	if err != nil {
		t.Fatalf("select: %v", err)
	}
	if resolved {
		t.Fatal("pricing ran without a source text")
	}
}

// TestSelectCompetingRecoveryLineageDeclinesSingleHead proves the helper only
// speaks to genuinely competing frontiers; a sole head takes the ordinary
// path.
func TestSelectCompetingRecoveryLineageDeclinesSingleHead(t *testing.T) {
	scheduler := newLineageSelectionScheduler(t, true)
	scheduler.headers = scheduler.headers[:1]
	_, resolved, err := scheduler.selectCompetingRecoveryLineage()
	if err != nil {
		t.Fatalf("select: %v", err)
	}
	if resolved {
		t.Fatal("a single-head frontier was routed through competition")
	}
}

// TestRecoveryLineageMarkerDoesNotGrowTheHeader pins one byte of recovery
// flags before versionState. A second field grows the header to 232 bytes.
func TestRecoveryLineageMarkerDoesNotGrowTheHeader(t *testing.T) {
	if got := unsafe.Sizeof(diagnosticParserCoreHeader{}); got != 224 {
		t.Fatalf("scheduler header is %d bytes, want 224", got)
	}
	var header diagnosticParserCoreHeader
	if header.isRecoveryLineage() {
		t.Fatal("a zero header reported itself as a recovery lineage")
	}
	header.markRecoveryLineage()
	if !header.isRecoveryLineage() || !header.isRecoveryCosted() {
		t.Fatal("marking did not take")
	}
	header.clearRecoveryLineage()
	if header.isRecoveryLineage() || !header.isRecoveryCosted() {
		t.Fatal("lineage clearing changed permanent cost provenance")
	}
}

// TestRecoveryOpenSegmentsDerivesFromLiveState proves the open-recovery term
// tracks the header fields the ordinary reduce and pause paths mutate, rather
// than a stored copy those paths would leave stale. A stale count mis-prices a
// lineage by 500, ten times the margin this arbitration turns on.
func TestRecoveryOpenSegmentsDerivesFromLiveState(t *testing.T) {
	var header diagnosticParserCoreHeader
	if got := header.recoveryOpenSegments(); got != 0 {
		t.Fatalf("a clean header reported %d open segments", got)
	}

	// C: `s.cPaused || (s.cRec != nil && s.cRec.openErr == nil)`.
	header.paused = true
	if got := header.recoveryOpenSegments(); got != 1 {
		t.Fatalf("a paused header reported %d open segments, want 1", got)
	}
	header.paused = false

	header.openRecoveryRegion(&diagnosticParserCoreS3Region{})
	if got := header.recoveryOpenSegments(); got != 1 {
		t.Fatalf("an opened-but-empty error region reported %d open segments, want 1", got)
	}
	// Once the region holds absorbed content, its cost lives in the published
	// subtrees and the open term must stop applying.
	header.setRecoveryRegion(&diagnosticParserCoreS3Region{children: []core.SubtreeID{1}})
	if got := header.recoveryOpenSegments(); got != 0 {
		t.Fatalf("a region with absorbed content reported %d open segments, want 0", got)
	}
}

// TestCollapseToRecoveryWinnerSeatsTheWinnerAndClearsLosers covers the one
// line that actually changes acceptance behaviour. A transposed index or a
// wrong-slot write here would publish the loser's tree, and no other test in
// this package would notice.
func TestCollapseToRecoveryWinnerSeatsTheWinnerAndClearsLosers(t *testing.T) {
	scheduler := newLineageSelectionScheduler(t, true)
	scheduler.headers = append(scheduler.headers, diagnosticParserCoreHeader{})
	scheduler.headers[2].markRecoveryLineage()
	scheduler.headers[2].openRecoveryRegion(&diagnosticParserCoreS3Region{})
	scheduler.recoveryIsolation = true
	retainedRegion := &diagnosticParserCoreS3Region{}
	for target := range scheduler.canonicalScratch.headerBuffers {
		buffer := make([]diagnosticParserCoreHeader, 2, 4)
		buffer[1].markRecoveryLineage()
		buffer[1].openRecoveryRegion(retainedRegion)
		scheduler.canonicalScratch.headerBuffers[target] = buffer
	}
	want := scheduler.headers[1]

	scheduler.collapseToRecoveryWinner(1)

	if len(scheduler.headers) != 1 {
		t.Fatalf("frontier has %d headers after collapse, want 1", len(scheduler.headers))
	}
	if scheduler.headers[0].head != want.head {
		t.Fatal("collapse seated a head other than the winner at index 0")
	}
	if scheduler.work.RecoveryLineageSelections != 1 {
		t.Fatalf("collapse recorded %d selections, want 1", scheduler.work.RecoveryLineageSelections)
	}
	if scheduler.recoveryIsolation || scheduler.headers[0].isRecoveryLineage() {
		t.Fatal("collapse retained recovery competition state on the winner")
	}
	// The losers must not stay live in the backing array: each retains an open
	// error region for the scheduler's lifetime otherwise.
	tail := scheduler.headers[:cap(scheduler.headers)][1:3]
	for index := range tail {
		if tail[index].recoveryRegion() != nil || tail[index].isRecoveryLineage() {
			t.Fatalf("dropped header %d is still live in the backing array", index+1)
		}
	}
	for target, buffer := range scheduler.canonicalScratch.headerBuffers {
		for index, header := range buffer[:cap(buffer)] {
			if header.recoveryRegion() == retainedRegion || header.isRecoveryLineage() {
				t.Fatalf("canonical buffer %d retained loser %d", target, index)
			}
		}
	}
}

func TestCollapseToRecoveryWinnerRecordsOnlyTheOriginalS5AbsorbLineage(t *testing.T) {
	newScheduler := func() *diagnosticParserCoreGenericScheduler {
		return &diagnosticParserCoreGenericScheduler{
			s5MissingInsertions: 1,
			headers: []diagnosticParserCoreHeader{
				{creationSeq: 4},
				{creationSeq: 5},
			},
		}
	}
	absorb := newScheduler()
	absorb.collapseToRecoveryWinner(0)
	if !absorb.selectedRecoveryAbsorbLineage {
		t.Fatal("the earlier original S5 lineage was not recorded as the absorb winner")
	}

	missing := newScheduler()
	missing.collapseToRecoveryWinner(1)
	if missing.selectedRecoveryAbsorbLineage {
		t.Fatal("the later missing lineage was recorded as the absorb winner")
	}
}
