//go:build gts_parsercorephase0

package gotreesitter

import (
	"errors"
	"testing"

	core "github.com/odvcencio/gotreesitter/internal/parsercorephase0"
)

// ownedZeroWidthCatchUpWitnessScanner is a minimal external scanner standing
// in for perl's `_NONASSOC` marker in the owned-dispatch synthetic harness
// below: it unconditionally emits a zero-width token for its one external
// symbol when the external lex state row marks it valid, and declines
// (falling back to the internal DFA for `a`/`b`) otherwise. It carries no
// state (ExternalScannerIsStateless), so the synthetic snapshots below never
// need to populate a payload.
type ownedZeroWidthCatchUpWitnessScanner struct{}

func (ownedZeroWidthCatchUpWitnessScanner) Create() any                      { return new(int) }
func (ownedZeroWidthCatchUpWitnessScanner) Destroy(any)                      {}
func (ownedZeroWidthCatchUpWitnessScanner) Serialize(any, []byte) int        { return 0 }
func (ownedZeroWidthCatchUpWitnessScanner) Deserialize(any, []byte)          {}
func (ownedZeroWidthCatchUpWitnessScanner) ExternalScannerIsStateless() bool { return true }

func (ownedZeroWidthCatchUpWitnessScanner) Scan(_ any, lexer *ExternalLexer, valid []bool) bool {
	if len(valid) == 0 || !valid[0] {
		return false
	}
	lexer.SetResultSymbol(3) // marker
	lexer.MarkEnd()
	return true
}

// ownedZeroWidthCatchUpWitnessLanguage builds the smallest grammar that
// reproduces the perl `_NONASSOC` shape entirely inside owned-dispatch mode:
// two owned headers independently lexing "ab", one of which (state 21) must
// shift a zero-width external marker before it can shift the same `a`/`b`
// content its sibling (state 11) reads directly.
//
// Symbols: 0 end, 1 `a`, 2 `b`, 3 marker (external, zero-width).
// States:  11 sibling-start (wants `a` directly), 12 sibling-after-a (wants
// `b`, and has NO action for it -- this is the witness's stuck point);
// 21 rescued-start (wants the marker first), 22 rescued-after-marker (wants
// `a`), 23 rescued-after-a (wants `b`, and DOES have an action for it,
// unlike the sibling's state 12).
func ownedZeroWidthCatchUpWitnessLanguage() *Language {
	lang := &Language{
		Name:            "owned_zero_width_catch_up_witness",
		SymbolCount:     4,
		TokenCount:      3,
		SymbolNames:     []string{"end", "a", "b", "marker"},
		ExternalSymbols: []Symbol{3},
		ExternalScanner: ownedZeroWidthCatchUpWitnessScanner{},
		LexStates: []LexState{
			{Default: -1, EOF: -1, Transitions: []LexTransition{
				{Lo: 'a', Hi: 'a', NextState: 1},
				{Lo: 'b', Hi: 'b', NextState: 2},
			}},
			{AcceptToken: 1, Default: -1, EOF: -1},
			{AcceptToken: 2, Default: -1, EOF: -1},
		},
		LexModes: make([]LexMode, 24),
	}
	// Every state's ExternalLexState defaults to 0 (row 0: marker invalid,
	// falls to the internal DFA above) except the rescued header's own
	// starting state, which needs the marker before anything else.
	lang.ExternalLexStates = [][]bool{
		{false}, // row 0: marker invalid
		{true},  // row 1: marker valid
	}
	lang.LexModes[21] = LexMode{ExternalLexState: 1}
	return lang
}

// ownedZeroWidthCatchUpWitnessTable is the genericConflictTable
// (parsercore_phase0_generic_conflict_internal_test.go) encoding the witness
// grammar's action table directly: no real grammar compile is needed since
// this scheduler's owned-dispatch path reads only (state, symbol) -> actions
// through this interface.
func ownedZeroWidthCatchUpWitnessTable() *genericConflictTable {
	return &genericConflictTable{cells: map[genericConflictCell][]core.Action{
		{state: 11, symbol: 1}: {{Type: core.ActionShift, State: 12}},
		// {state: 12, symbol: 2} is deliberately absent: the sibling has no
		// action for `b` -- the witness's stuck point.
		{state: 21, symbol: 3}: {{Type: core.ActionShift, State: 22}},
		{state: 22, symbol: 1}: {{Type: core.ActionShift, State: 23}},
		{state: 23, symbol: 2}: {{Type: core.ActionShift, State: 24}},
	}}
}

// ownedZeroWidthCatchUpWitnessTableRescuedStuckAfterMarker is
// ownedZeroWidthCatchUpWitnessTable with {state 22, symbol 1} removed: the
// rescued header now has no action immediately after its own zero-width
// marker shift, so its own reopened reclassification is itself a no-action
// candidate at byte 0 -- the same byte position its marker shift left it at,
// since a zero-width shift never advances the byte cursor.
func ownedZeroWidthCatchUpWitnessTableRescuedStuckAfterMarker() *genericConflictTable {
	return &genericConflictTable{cells: map[genericConflictCell][]core.Action{
		{state: 11, symbol: 1}: {{Type: core.ActionShift, State: 12}},
		{state: 21, symbol: 3}: {{Type: core.ActionShift, State: 22}},
		// {state: 22, symbol: 1} is deliberately absent, unlike the default
		// witness table: this is the shape under test.
	}}
}

// newOwnedZeroWidthCatchUpStartSnapshot builds the byte-zero starting
// snapshot both witness headers seed from. Unlike
// newDiagnosticParserCoreOwnedLexerSnapshot's own callers
// (parsercore_phase0_per_version_lexer_lifecycle_internal_test.go), this
// harness's language declares a real (though stateless) ExternalScanner, so
// the dfaRelexSnapshot must set externalScannerPresent explicitly --
// newDiagnosticParserCoreVersionLexerSnapshot rejects a snapshot whose
// presence flag disagrees with the language's own scanner contract.
func newOwnedZeroWidthCatchUpStartSnapshot(t *testing.T, compact *core.Core, language *Language) *diagnosticParserCoreVersionLexerSnapshot {
	t.Helper()
	var snapshot *diagnosticParserCoreVersionLexerSnapshot
	err := compact.ApplySchedulerAtomic(func(owner core.SchedulerTransactionToken) error {
		var snapshotErr error
		snapshot, snapshotErr = newDiagnosticParserCoreVersionLexerSnapshot(
			compact, language, owner,
			dfaRelexSnapshot{lexerPos: 0, externalScannerPresent: true},
			0, 0,
		)
		return snapshotErr
	})
	if err != nil {
		t.Fatalf("construct owned lexer start snapshot: %v", err)
	}
	return snapshot
}

// newOwnedZeroWidthCatchUpWitnessScheduler wires the witness grammar and
// table up to a two-header owned-dispatch scheduler, seeded at state 11
// (sibling) and state 21 (rescued), both at byte 0 of "ab". Each header
// starts with no pending request (lexerRequest: 0), so classifyVersionLexerCell's
// own requestHeaderLexerToken issues its first request through the real
// dfaTokenSource -- this harness drives genuine per-header lexing, not
// pre-scripted tokens.
func newOwnedZeroWidthCatchUpWitnessScheduler(t *testing.T) *diagnosticParserCoreGenericScheduler {
	t.Helper()
	return newOwnedZeroWidthCatchUpWitnessSchedulerWithTable(t, ownedZeroWidthCatchUpWitnessTable())
}

// newOwnedZeroWidthCatchUpWitnessSchedulerWithTable is
// newOwnedZeroWidthCatchUpWitnessScheduler parameterized over the compact
// conflict table, so a caller can remove one action cell (for example
// {state 22, symbol 1}) to reach a header shape the default table never
// produces -- a rescued header that is itself stuck immediately after its
// own zero-width marker shift -- without duplicating the rest of the
// harness's grammar, lexer, and snapshot wiring.
func newOwnedZeroWidthCatchUpWitnessSchedulerWithTable(t *testing.T, table *genericConflictTable) *diagnosticParserCoreGenericScheduler {
	t.Helper()
	lang := ownedZeroWidthCatchUpWitnessLanguage()
	compact, err := core.New(table, core.Limits{})
	if err != nil {
		t.Fatalf("construct compact core: %v", err)
	}
	siblingHead, err := compact.Seed(11, 0)
	if err != nil {
		t.Fatalf("seed sibling head: %v", err)
	}
	rescuedHead, err := compact.Seed(21, 0)
	if err != nil {
		t.Fatalf("seed rescued head: %v", err)
	}
	source := []byte("ab")
	// hasAnyActionForSymbol (parser_dfa_token_source.go) uses this lookup to
	// decide whether a zero-width token is usable at all, independent of the
	// compact core's own grammar table (genericConflictTable above); it must
	// mirror this witness's own action cells or Next() silently skips the
	// marker as dead weight.
	lookupActionIndex := func(state StateID, sym Symbol) uint16 {
		switch {
		case state == 11 && sym == 1, state == 21 && sym == 3,
			state == 22 && sym == 1, state == 23 && sym == 2:
			return 1
		default:
			return 0
		}
	}
	tokenSource := newDFATokenSourceDirect(NewLexer(lang.LexStates, source), lang, lookupActionIndex, nil, nil, nil)
	t.Cleanup(tokenSource.Close)

	startSnapshot := newOwnedZeroWidthCatchUpStartSnapshot(t, compact, lang)
	scannerScratch := make([]byte, 0, 64)

	scheduler := &diagnosticParserCoreGenericScheduler{
		compact:     compact,
		tokenSource: tokenSource,
		headers: []diagnosticParserCoreHeader{
			{creationSeq: 0, head: siblingHead, versionState: &diagnosticParserCoreVersionState{relexSnapshot: startSnapshot}},
			{creationSeq: 1, head: rescuedHead, versionState: &diagnosticParserCoreVersionState{relexSnapshot: startSnapshot}},
		},
		electionIndex:               7,
		versionLexerOwnershipActive: true,
		scannerScratch:              &scannerScratch,
		options: DiagnosticParserCorePrefixOptions{
			ReceiptMode:   DiagnosticParserCoreReceiptSummary,
			MaxDispatches: 64,
		},
		receipt: &DiagnosticParserCoreGenericScheduler{},
	}
	return scheduler
}

// errOwnedDispatchLoopExhausted marks a runOwnedDispatchUntilStuckOrDone call
// that never reached a terminal state (stop, decline, or single-survivor
// success) within its own round budget. A caller that let this collapse
// into "nil, nil" could mistake a genuine non-termination bug (for example,
// zeroWidthReopened never being consumed) for a passing test.
var errOwnedDispatchLoopExhausted = errors.New("owned dispatch harness: round budget exhausted without reaching a terminal state")

// runOwnedDispatchUntilStuckOrDone drives dispatchPass repeatedly, mirroring
// run()'s own loop (parsercore_phase0_driver.go): between dispatchPass calls
// it replicates run()'s own allClosed check (including the zeroWidthReopened
// exception ownedZeroWidthCatchUp's own doc comment describes) and, when
// every live header has shifted, calls beginNextVersionLexerElection to open
// the next owned round exactly as run() would -- dispatchPass alone has no
// way to advance a fully closed frontier (that is
// beginNextVersionLexerElection's own job), so a harness that only calls
// dispatchPass would misreport "no runnable head" as a decline instead of a
// normal round boundary.
func runOwnedDispatchUntilStuckOrDone(t *testing.T, scheduler *diagnosticParserCoreGenericScheduler) (*diagnosticParserCoreGenericUnsupported, error) {
	t.Helper()
	for round := 0; round < 32; round++ {
		allClosed := true
		for index := range scheduler.headers {
			header := &scheduler.headers[index]
			if (!header.shifted || header.isZeroWidthReopened()) && !header.accepted {
				allClosed = false
				break
			}
		}
		if allClosed {
			// A sole surviving head closing its round is this harness's own
			// success shape: run()'s own single-head handling
			// (rejoinSharedLexerFromOwnedHeader + elect) needs a full shared
			// dispatch setup this synthetic table-only harness does not
			// build, and this test only needs to observe that the drop
			// happened and left the rescued header where it should be.
			if len(scheduler.headers) < 2 {
				return nil, nil
			}
			if err := scheduler.beginNextVersionLexerElection(); err != nil {
				return nil, err
			}
			continue
		}
		stop, err := scheduler.dispatchPass()
		if err != nil || stop != nil {
			return stop, err
		}
	}
	return nil, errOwnedDispatchLoopExhausted
}

// TestOwnedDispatchZeroWidthCatchUpAdmitsRaggedNoActionDrop is the
// synthetic-harness witness for this task: without ownedZeroWidthCatchUp
// (relexZeroWidthExternalTokenForState's port target), the rescued header's
// zero-width marker shift permanently costs it one owned request relative to
// its sibling, so by the time the sibling gets stuck at state 12 with no
// action for `b`, the rescued header's own last real shift (`a`, byte 0..1)
// starts one byte behind the sibling's stuck token (`b`, byte 1..2) and
// versionLexerNoActionDropEligible's same-start-byte proof declines. With
// ownedZeroWidthCatchUp active, the rescued header re-requests immediately
// after its zero-width marker shift instead of waiting for the next full
// barrier round, so both headers reach real content at the same pace and the
// drop succeeds once the sibling is provably stuck.
func TestOwnedDispatchZeroWidthCatchUpAdmitsRaggedNoActionDrop(t *testing.T) {
	scheduler := newOwnedZeroWidthCatchUpWitnessScheduler(t)
	stop, err := runOwnedDispatchUntilStuckOrDone(t, scheduler)
	if err != nil {
		t.Fatalf("owned dispatch run: %v", err)
	}
	if stop != nil {
		t.Fatalf("owned dispatch declined: %+v", stop)
	}
	// The sibling (no action for `b`) must have been dropped, leaving only
	// the rescued header, now past its own `b` shift (state 24).
	if len(scheduler.headers) != 1 {
		t.Fatalf("headers after drop = %d, want 1 (only the rescued header should survive)", len(scheduler.headers))
	}
	state, byteOffset, err := scheduler.compact.Boundary(scheduler.headers[0].head)
	if err != nil {
		t.Fatalf("boundary of surviving head: %v", err)
	}
	if state != 24 || byteOffset != 2 {
		t.Fatalf("surviving head state=%d byteOffset=%d, want state=24 byteOffset=2 (rescued header past its own `b` shift)", state, byteOffset)
	}
}

// TestOwnedDispatchZeroWidthCatchUpDeclinesWithoutBudget proves the catch-up
// mechanism is what makes the drop possible, not some other latent behavior:
// with the rescued header's own per-election budget exhausted up front, the
// exact same harness reproduces today's decline (generic scheduler has no
// table action for the elected token), matching this task's own
// investigation of the perl `_NONASSOC` witness before this port.
//
// The budget is per-header, per-election (ownedZeroWidthCatchUp's own doc
// comment), kept in scheduler.zeroWidthCatchUp keyed by creationSeq rather
// than on the header itself (the header has no spare bytes; see
// diagnosticParserCoreHeader's own layout comment): pre-exhausting it means
// setting BOTH budget=0 AND election to the scheduler's own current election
// (electionIndex+1), matching the sentinel ownedZeroWidthCatchUp itself
// writes on a real reset. Leaving election at its zero value would instead
// look like "never reset for this election", and ownedZeroWidthCatchUp would
// lazily refill the budget on its very first call, defeating this test's own
// setup.
func TestOwnedDispatchZeroWidthCatchUpDeclinesWithoutBudget(t *testing.T) {
	scheduler := newOwnedZeroWidthCatchUpWitnessScheduler(t)
	rescued := &scheduler.headers[1]
	scheduler.zeroWidthCatchUp = map[uint64]diagnosticParserCoreZeroWidthCatchUpState{
		rescued.creationSeq: {budget: 0, election: uint32(scheduler.electionIndex + 1)},
	}
	stop, err := runOwnedDispatchUntilStuckOrDone(t, scheduler)
	if err != nil {
		t.Fatalf("owned dispatch run: %v", err)
	}
	if stop == nil {
		t.Fatal("owned dispatch accepted the ragged frontier with the catch-up budget exhausted, want the pre-fix decline")
	}
	if stop.boundary != DiagnosticParserCoreRecovery || stop.detail != diagnosticParserCoreNoTableActionDetail {
		t.Fatalf("decline = %+v, want boundary=%s detail=%q", stop, DiagnosticParserCoreRecovery, diagnosticParserCoreNoTableActionDetail)
	}
}

// TestOwnedDispatchZeroWidthCatchUpBudgetIsLazilyPerHeaderPerElection is the
// direct unit witness for ownedZeroWidthCatchUp's own lazy reset (finding:
// "assert the initializer sets the budget"): a header's first catch-up call
// under a given election fills its budget to
// maxOwnedZeroWidthCatchUpsPerElection and immediately spends one; repeated
// calls under the SAME election keep spending from that same header-owned
// counter, independent of any other header; and advancing electionIndex
// resets the budget again on the next call, exactly once, lazily.
func TestOwnedDispatchZeroWidthCatchUpBudgetIsLazilyPerHeaderPerElection(t *testing.T) {
	scheduler := &diagnosticParserCoreGenericScheduler{
		electionIndex: 3,
		headers: []diagnosticParserCoreHeader{
			{creationSeq: 0},
			{creationSeq: 1},
		},
	}
	scheduler.ownedZeroWidthCatchUp(0)
	if got := scheduler.zeroWidthCatchUp[0].budget; got != maxOwnedZeroWidthCatchUpsPerElection-1 {
		t.Fatalf("header 0 budget after first catch-up = %d, want %d (lazily filled, then spent one)", got, maxOwnedZeroWidthCatchUpsPerElection-1)
	}
	if !scheduler.headers[0].isZeroWidthReopened() {
		t.Fatal("header 0 was not reopened by its first catch-up")
	}
	if got := scheduler.zeroWidthCatchUp[1].budget; got != 0 || scheduler.headers[1].isZeroWidthReopened() {
		t.Fatalf("header 1 budget=%d reopened=%t, want untouched by header 0's own catch-up", got, scheduler.headers[1].isZeroWidthReopened())
	}
	scheduler.headers[0].clearZeroWidthReopened() // classifyVersionLexerCell's own one-shot consumption
	for i := 1; i < maxOwnedZeroWidthCatchUpsPerElection; i++ {
		scheduler.ownedZeroWidthCatchUp(0)
	}
	if got := scheduler.zeroWidthCatchUp[0].budget; got != 0 {
		t.Fatalf("header 0 budget after exhausting this election = %d, want 0", got)
	}
	scheduler.headers[0].clearZeroWidthReopened()
	scheduler.ownedZeroWidthCatchUp(0)
	if scheduler.headers[0].isZeroWidthReopened() {
		t.Fatal("header 0 was reopened after its own per-election budget was exhausted")
	}
	scheduler.electionIndex++
	scheduler.ownedZeroWidthCatchUp(0)
	if got := scheduler.zeroWidthCatchUp[0].budget; got != maxOwnedZeroWidthCatchUpsPerElection-1 {
		t.Fatalf("header 0 budget after the next election = %d, want %d (lazily reset)", got, maxOwnedZeroWidthCatchUpsPerElection-1)
	}
	if !scheduler.headers[0].isZeroWidthReopened() {
		t.Fatal("header 0 was not reopened after its budget reset for the next election")
	}
}

// TestOwnedDispatchZeroWidthCatchUpPreservesCanonicalBoundaryIdentity is the
// regression test for the review finding that fails when ownedZeroWidthCatchUp
// clears header.shifted directly: header.shifted is also the shifted
// component of compact.CanonicalBoundary's own boundary identity
// (canonicalizeWithMutation, parsercore_phase0_driver.go), so clearing it
// early lets the very next canonicalization probe the rescued header's
// (state, byteOffset) pair under the UNSHIFTED key. If a foreign node already
// occupies that exact (state, byteOffset, unshifted) boundary -- standing in
// for a sibling's own reduction/goto landing at the same place -- that
// canonicalization remaps the rescued header's own head onto the foreign
// node instead of leaving it on the one its own marker shift actually
// produced.
//
// This test drives compact.Shift directly to build the exact head the
// rescued header's own marker shift would have produced -- registered at
// (state 22, byte 0, shifted=true) -- then calls ownedZeroWidthCatchUp
// itself (the function under test, unmodified) on a two-header scheduler, so
// a real bug in ownedZeroWidthCatchUp (not a hand-simulated one) is what
// this test would catch. compact.Seed registers its foreign node at the
// same (state, byte) pair but with shifted=false, deliberately colliding
// only under the buggy unshifted key. Both calls use the compact core's own
// current checkpoint (0, since nothing here drives the scanner), so no
// explicit checkpoint alignment is needed.
func TestOwnedDispatchZeroWidthCatchUpPreservesCanonicalBoundaryIdentity(t *testing.T) {
	table := ownedZeroWidthCatchUpWitnessTable()
	compact, err := core.New(table, core.Limits{})
	if err != nil {
		t.Fatalf("construct compact core: %v", err)
	}
	rescuedStart, err := compact.Seed(21, 0)
	if err != nil {
		t.Fatalf("seed rescued start: %v", err)
	}
	rescuedHeadAfterMarker, err := compact.Shift(rescuedStart, 3, 0, core.Token{
		Symbol: 3, StartByte: 0, EndByte: 0, External: true,
	}, core.ForkOrder{})
	if err != nil {
		t.Fatalf("shift rescued header past its own marker: %v", err)
	}
	// A second, unrelated live head: ownedZeroWidthCatchUp's own multi-head
	// gate (len(s.headers) < 2) declines outright with only one header, so
	// this sibling stands in for whatever else is live in the frontier when
	// the marker shift commits. Its own identity plays no further role.
	siblingHead, err := compact.Seed(99, 5)
	if err != nil {
		t.Fatalf("seed sibling head: %v", err)
	}

	scheduler := &diagnosticParserCoreGenericScheduler{
		compact: compact,
		headers: []diagnosticParserCoreHeader{
			{creationSeq: 0, head: siblingHead, shifted: true},
			{creationSeq: 1, head: rescuedHeadAfterMarker, shifted: true},
		},
		electionIndex: 0,
	}
	const rescuedCreationSeq = 1
	scheduler.ownedZeroWidthCatchUp(rescuedCreationSeq)
	rescued := &scheduler.headers[1]
	if rescued.creationSeq != rescuedCreationSeq {
		t.Fatalf("headers[1].creationSeq = %d, want %d (test setup precondition)", rescued.creationSeq, rescuedCreationSeq)
	}
	if !rescued.isZeroWidthReopened() {
		t.Fatal("ownedZeroWidthCatchUp did not reopen the rescued header")
	}
	if !rescued.shifted {
		t.Fatal("ownedZeroWidthCatchUp cleared header.shifted; it must leave the committed shift's own value untouched (see its own doc comment)")
	}

	// Seed the foreign, standalone node only now: Seed's own boundary key
	// (internal/parsercorephase0/core.go) folds in the compact core's
	// current frontier, so seeding it before the marker shift above would
	// register it at a stale frontier instead of the one canonicalization
	// below actually probes.
	foreignHead, err := compact.Seed(22, 0)
	if err != nil {
		t.Fatalf("seed foreign boundary: %v", err)
	}
	if foreignHead == rescuedHeadAfterMarker {
		t.Fatal("foreign (unshifted) and rescued (shifted) boundaries collided before canonicalization ran; test setup is not isolating shifted from unshifted identity")
	}

	err = compact.ApplySchedulerAtomic(func(owner core.SchedulerTransactionToken) error {
		return scheduler.canonicalizeOwned(owner)
	})
	if err != nil {
		t.Fatalf("canonicalize: %v", err)
	}
	if len(scheduler.headers) != 2 {
		t.Fatalf("headers after canonicalize = %d, want 2 (no merge should occur)", len(scheduler.headers))
	}
	var rescuedAfterCanon *diagnosticParserCoreHeader
	for index := range scheduler.headers {
		if scheduler.headers[index].creationSeq == rescuedCreationSeq {
			rescuedAfterCanon = &scheduler.headers[index]
		}
	}
	if rescuedAfterCanon == nil {
		t.Fatal("rescued header (creationSeq 1) not found after canonicalize")
	}
	if rescuedAfterCanon.head == foreignHead {
		t.Fatal("canonicalize remapped the rescued header onto the foreign (state 22, byte 0, unshifted) boundary instead of keeping its own shifted identity")
	}
	if rescuedAfterCanon.head != rescuedHeadAfterMarker {
		t.Fatalf("rescued header head = %v, want %v (its own marker-shift head, unchanged)", rescuedAfterCanon.head, rescuedHeadAfterMarker)
	}
}

// TestOwnedDispatchZeroWidthCatchUpDropsReopenedHeaderStuckAfterMarker is the
// regression test for the review finding that versionLexerNoActionDropEligible
// and diagnosticParserCoreGenericNoActionDropEligible read header.shifted
// directly: a reopened header that commits its own zero-width marker shift
// and then finds no action for its very next token is, from the
// same-start-byte proof's own point of view, no different from a header
// that never shifted at all this round -- its marker shift never moved its
// byte cursor. Reading raw shifted there instead makes such a header
// undroppable purely because ownedZeroWidthCatchUp deliberately left
// shifted=true across the reopen (see effectivelyShifted's own doc
// comment): the whole drop would decline even though the rescued header's
// own pending request legitimately has no action, at the same byte position
// (0) its surviving sibling's last real shift started from.
//
// This uses ownedZeroWidthCatchUpWitnessTableRescuedStuckAfterMarker, which
// removes {state 22, symbol 1} from the default witness table: the rescued
// header now has no action immediately after its own marker shift, so it is
// the one that must be dropped here, leaving the sibling as the sole
// survivor.
func TestOwnedDispatchZeroWidthCatchUpDropsReopenedHeaderStuckAfterMarker(t *testing.T) {
	scheduler := newOwnedZeroWidthCatchUpWitnessSchedulerWithTable(t, ownedZeroWidthCatchUpWitnessTableRescuedStuckAfterMarker())
	stop, err := runOwnedDispatchUntilStuckOrDone(t, scheduler)
	if err != nil {
		t.Fatalf("owned dispatch run: %v", err)
	}
	if stop != nil {
		t.Fatalf("owned dispatch declined: %+v", stop)
	}
	if len(scheduler.headers) != 1 {
		t.Fatalf("headers after drop = %d, want 1 (only the sibling should survive)", len(scheduler.headers))
	}
	if scheduler.headers[0].creationSeq != 0 {
		t.Fatalf("surviving header creationSeq = %d, want 0 (the sibling; the rescued header had no action and should have been dropped)", scheduler.headers[0].creationSeq)
	}
	state, byteOffset, err := scheduler.compact.Boundary(scheduler.headers[0].head)
	if err != nil {
		t.Fatalf("boundary of surviving head: %v", err)
	}
	if state != 12 || byteOffset != 1 {
		t.Fatalf("surviving head state=%d byteOffset=%d, want state=12 byteOffset=1 (sibling past its own `a` shift)", state, byteOffset)
	}
}
