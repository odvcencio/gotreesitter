package scannercert

import (
	"strings"
	"testing"

	gts "github.com/odvcencio/gotreesitter"
)

func TestCorrectScannerHasNoMismatches(t *testing.T) {
	a := certified(counterScanner{}, "abacab", bothValid)
	if a.verdict() != VerdictNoFindings {
		t.Fatalf("verdict = %q, want no findings; checks %+v witnesses %+v", a.verdict(), a.checks, a.witnesses)
	}
	wantCalls := CallInfo{Recorded: 6, Distinct: 6, Accepted: 5, Rejected: 1, States: 4, Masks: 1, MaxStateBytes: 1}
	if a.calls != wantCalls {
		t.Fatalf("calls = %+v, want %+v", a.calls, wantCalls)
	}
	if a.checks != (Checks{Replayed: 6}) {
		t.Fatalf("checks = %+v, want six clean replays", a.checks)
	}
	if len(a.witnesses) != 0 {
		t.Fatalf("witnesses = %+v, want none", a.witnesses)
	}
}

// A scanner whose Serialize omits the counter replays from a different state
// than the parser held, so the second and third "a" report other symbols.
func TestIncompleteSerializationIsReported(t *testing.T) {
	a := certified(counterScanner{omitState: true}, "abacab", bothValid)
	if a.verdict() != VerdictFindings {
		t.Fatalf("verdict = %q, want findings", a.verdict())
	}
	want := Checks{Replayed: 6, SymbolMismatches: 2}
	if a.checks != want {
		t.Fatalf("checks = %+v, want %+v", a.checks, want)
	}
	w := a.witnesses[0]
	if w.Kind != KindSymbol || w.Origin != (Origin{Byte: 2, Row: 0, Column: 2}) || w.Context != "acab" {
		t.Fatalf("first witness = %+v, want a symbol mismatch at 0:2", w)
	}
	if w.State != "" || w.Live == nil || w.Replay == nil || w.Live.Symbol != 2 || w.Replay.Symbol != 1 {
		t.Fatalf("witness outcomes = %+v / %+v, want live symbol 2 and replay symbol 1", w.Live, w.Replay)
	}
	if w.Live.State != "" || w.Replay.State != "" {
		t.Fatalf("witness states = %q / %q, want empty serializations", w.Live.State, w.Replay.State)
	}
}

func TestTokenSpanMismatchIsReported(t *testing.T) {
	a := certified(widthScanner{omitState: true}, "aaaa", bothValid)
	want := Checks{Replayed: 4, SpanMismatches: 2}
	if a.checks != want {
		t.Fatalf("checks = %+v, want %+v", a.checks, want)
	}
	w := a.witnesses[0]
	if w.Kind != KindSpan || w.Origin.Byte != 1 || w.Live.EndByte != 3 || w.Replay.EndByte != 2 || w.Live.EndColumn != 3 {
		t.Fatalf("witness = %+v live %+v replay %+v, want a span ending at byte 3 live and 2 replayed", w, w.Live, w.Replay)
	}
}

// A rejected scan that changes the serialized state breaks the contract only
// for a scanner that promises to preserve it. By default the token source
// restores the saved state itself, and a scanner that declares retention keeps
// the change on purpose. The count stays in the report either way.
func TestRejectedScanThatChangesStateIsAFindingOnlyWhenTheScannerPromisesNone(t *testing.T) {
	dirty := counterScanner{dirtyOnReject: true}
	for _, tc := range []struct {
		name     string
		scanner  gts.ExternalScanner
		contract string
		finding  bool
	}{
		{"default contract", dirty, ContractRestored, false},
		{"declares retention", retainingCounter{dirty}, ContractRetained, false},
		{"declares both, retention wins", bothCounter{dirty}, ContractRetained, false},
		{"declares preservation", preservingCounter{dirty}, ContractPreserved, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := failureContractOf(tc.scanner); got != tc.contract {
				t.Fatalf("contract = %q, want %q", got, tc.contract)
			}
			a := certified(tc.scanner, "abacab", bothValid)
			w := a.witnesses[0]
			if w.Origin.Byte != 3 || w.State != "02" || w.Live.State != "03" || w.Live.Accepted {
				t.Fatalf("witness = %+v live %+v, want the rejected scan at byte 3 moving state 02 to 03", w, w.Live)
			}
			if tc.finding {
				want := Checks{Replayed: 6, FailedScanStateChanges: 1}
				if a.checks != want || a.info != (Informational{}) || a.verdict() != VerdictFindings || w.Kind != KindFailedScanState {
					t.Fatalf("checks %+v info %+v verdict %q kind %q, want %+v and a finding", a.checks, a.info, a.verdict(), w.Kind, want)
				}
				return
			}
			if a.checks != (Checks{Replayed: 6}) || a.info != (Informational{FailedScanStateAllowed: 1}) ||
				a.verdict() != VerdictNoFindings || w.Kind != KindFailedScanStateAllowed {
				t.Fatalf("checks %+v info %+v verdict %q kind %q, want the change counted as informational", a.checks, a.info, a.verdict(), w.Kind)
			}
		})
	}
}

func TestPreservingScannerThatKeepsItsPromiseHasNoFailedScanFinding(t *testing.T) {
	a := certified(preservingCounter{counterScanner{}}, "abacab", bothValid)
	if a.verdict() != VerdictNoFindings || a.checks != (Checks{Replayed: 6}) || a.info != (Informational{}) {
		t.Fatalf("verdict %q checks %+v info %+v, want a clean report", a.verdict(), a.checks, a.info)
	}
}

func TestRestoreThatChangesBytesIsReported(t *testing.T) {
	a := certified(counterScanner{clampAbove: 2}, "aaaaa", bothValid)
	want := Checks{Replayed: 5, SymbolMismatches: 2, StateMismatches: 2, RoundTripMismatches: 2}
	if a.checks != want {
		t.Fatalf("checks = %+v, want %+v", a.checks, want)
	}
	var round *Witness
	for i := range a.witnesses {
		if a.witnesses[i].Kind == KindRoundTrip {
			round = &a.witnesses[i]
			break
		}
	}
	if round == nil || round.Origin.Byte != 3 || round.State != "03" || !strings.Contains(round.Detail, "returned 02") {
		t.Fatalf("round-trip witness = %+v, want byte 3 restoring 03 as 02", round)
	}
}

func TestUnstableSerializeIsReported(t *testing.T) {
	a := certified(counterScanner{flipOnSave: true}, "abacab", bothValid)
	if a.checks.SerializeUnstable != a.calls.Distinct || a.calls.Distinct == 0 {
		t.Fatalf("unstable = %d of %d calls, want every call", a.checks.SerializeUnstable, a.calls.Distinct)
	}
	if a.verdict() != VerdictFindings {
		t.Fatalf("verdict = %q, want findings", a.verdict())
	}
}

func TestOversizeSerializeIsReported(t *testing.T) {
	a := certified(counterScanner{oversize: true}, "ab", bothValid)
	if a.checks.SerializeOverflow != a.calls.Distinct || a.calls.Distinct == 0 {
		t.Fatalf("overflow = %d of %d calls, want every call", a.checks.SerializeOverflow, a.calls.Distinct)
	}
	if a.calls.MaxStateBytes != stateBufferSize {
		t.Fatalf("max state bytes = %d, want the %d-byte buffer", a.calls.MaxStateBytes, stateBufferSize)
	}
}

func TestReplayPanicIsReportedNotRaised(t *testing.T) {
	a := certified(counterScanner{panicOn: 2}, "aaaa", bothValid)
	if a.checks.ReplayPanics != 1 || a.verdict() != VerdictFindings {
		t.Fatalf("checks = %+v verdict %q, want one replay panic and findings", a.checks, a.verdict())
	}
	w := a.witnesses[0]
	if w.Kind != KindReplayPanic || w.State != "02" || !strings.Contains(w.Detail, "cannot restore this state") {
		t.Fatalf("witness = %+v, want a panic restoring state 02", w)
	}
}

func TestStatelessScannerNeedsNoSerializedState(t *testing.T) {
	a := certified(echoScanner{}, "aaba", []bool{true})
	if a.verdict() != VerdictNoFindings || a.calls.States != 1 || a.calls.EmptyStates != 4 || a.calls.MaxStateBytes != 0 {
		t.Fatalf("verdict %q calls %+v, want no findings over one empty state", a.verdict(), a.calls)
	}
}

func TestRepeatedCallsAreReplayedOnce(t *testing.T) {
	rec := newRecorder(echoScanner{})
	rec.begin(0)
	payload := rec.Create()
	for i := 0; i < 3; i++ {
		rec.Scan(payload, newLexer([]byte("ab"), 0, 0, 0), []bool{true})
	}
	rec.Scan(payload, newLexer([]byte("ab"), 1, 0, 1), []bool{true})
	a := rec.analyze(echoScanner{}, symbolNames{})
	if a.calls.Recorded != 4 || a.calls.Distinct != 2 || a.checks.Replayed != 2 {
		t.Fatalf("calls %+v checks %+v, want 4 recorded, 2 distinct, 2 replayed", a.calls, a.checks)
	}
}

func TestUnusedScannerHasNoEvidence(t *testing.T) {
	rec := newRecorder(echoScanner{})
	a := rec.analyze(echoScanner{}, symbolNames{})
	if a.verdict() != VerdictNoEvidence || a.calls != (CallInfo{}) {
		t.Fatalf("verdict %q calls %+v, want no evidence", a.verdict(), a.calls)
	}
}

func TestWitnessesAreCappedPerKindButEveryMismatchIsCounted(t *testing.T) {
	a := certified(counterScanner{omitState: true}, strings.Repeat("a", 16), bothValid)
	if a.checks.SymbolMismatches != 10 {
		t.Fatalf("symbol mismatches = %d, want 10", a.checks.SymbolMismatches)
	}
	if len(a.witnesses) != MaxWitnessesPerKind {
		t.Fatalf("witnesses = %d, want the cap of %d", len(a.witnesses), MaxWitnessesPerKind)
	}
	for i := 1; i < len(a.witnesses); i++ {
		if a.witnesses[i].Origin.Byte <= a.witnesses[i-1].Origin.Byte {
			t.Fatalf("witnesses are not in first-seen order: %+v", a.witnesses)
		}
	}
}

func TestWitnessNamesTheCorpusSource(t *testing.T) {
	inner := counterScanner{omitState: true}
	rec := newRecorder(inner)
	rec.begin(0)
	scanEvery(rec, "ab", bothValid)
	rec.begin(3)
	scanEvery(rec, "aab", bothValid)
	a := rec.analyze(inner, symbolNames{})
	if len(a.witnesses) != 1 || a.witnesses[0].Source != 3 || a.witnesses[0].Origin.Byte != 1 {
		t.Fatalf("witnesses = %+v, want one mismatch at byte 1 of source 3", a.witnesses)
	}
}

// The recorder must replay each call against the bytes the scanner saw, not
// against whichever buffer the parser reuses later.
func TestRecorderKeepsItsOwnCopyOfEachSource(t *testing.T) {
	inner := counterScanner{omitState: true}
	rec := newRecorder(inner)
	rec.begin(0)
	buf := []byte("aab")
	payload := rec.Create()
	rec.Scan(payload, newLexer(buf, 0, 0, 0), bothValid)
	rec.Scan(payload, newLexer(buf, 1, 0, 1), bothValid)
	copy(buf, "zzz")
	a := rec.analyze(inner, symbolNames{})
	if a.checks.SymbolMismatches != 1 || a.witnesses[0].Context != "ab" {
		t.Fatalf("checks %+v witnesses %+v, want the mismatch replayed on the original bytes", a.checks, a.witnesses)
	}
}

func TestSymbolNames(t *testing.T) {
	lang := &gts.Language{
		SymbolNames:     []string{"end", "a", "b", "x", "y", "z"},
		ExternalSymbols: []gts.Symbol{3, 4, 5},
	}
	names := symbolNames{lang: lang}
	if got := strings.Join(names.valid([]bool{true, false, true}), ","); got != "0:x,2:z" {
		t.Fatalf("valid = %q, want 0:x,2:z", got)
	}
	if got := names.symbol(5); got != "z" {
		t.Fatalf("symbol(5) = %q, want z", got)
	}
	if got := names.symbol(99); got != "" {
		t.Fatalf("symbol(99) = %q, want empty", got)
	}
	if got := strings.Join((symbolNames{}).valid([]bool{false, true, true}), ","); got != "1,2" {
		t.Fatalf("valid without a language = %q, want 1,2", got)
	}
}

// The parser reads these interfaces during a full parse. The recorder must
// answer as the real scanner does, and as the parser assumes for a scanner
// that lacks them, or the recorded parse would differ from the real one.
func TestRecorderMirrorsTheOptionalInterfaces(t *testing.T) {
	declared := capableScanner{
		reuse: true, checkpointed: true, stateless: true, preserves: true, retains: true,
		errorRetry: false, packedUpTo: 10,
	}
	got := newRecorder(declared).install()
	if !got.(gts.IncrementalReuseExternalScanner).SupportsIncrementalReuse() ||
		!got.(gts.CheckpointedExternalScanner).UsesExternalScannerCheckpoints() ||
		!got.(gts.StatelessExternalScanner).ExternalScannerIsStateless() ||
		!got.(gts.FailurePreservingExternalScanner).PreservesStateOnScanFailure() ||
		!got.(gts.FailureStateRetainingExternalScanner).RetainsStateOnScanFailure() {
		t.Fatal("the recorder drops a capability the scanner declares")
	}
	retry := got.(interface{ SupportsErrorModeExternalRetry() bool })
	packed := got.(interface{ SupportsPackedGSSVersionOrder(int) bool })
	if retry.SupportsErrorModeExternalRetry() || !packed.SupportsPackedGSSVersionOrder(10) || packed.SupportsPackedGSSVersionOrder(11) {
		t.Fatal("the recorder changes the scanner's error-retry or packed-GSS answer")
	}
	if _, ok := got.(gts.ExternalScannerCheckpointIdentityProvider); ok {
		t.Fatal("the recorder gives a scanner without a checkpoint identity one")
	}

	plain := newRecorder(counterScanner{}).install()
	if plain.(gts.IncrementalReuseExternalScanner).SupportsIncrementalReuse() ||
		plain.(gts.CheckpointedExternalScanner).UsesExternalScannerCheckpoints() ||
		plain.(gts.StatelessExternalScanner).ExternalScannerIsStateless() ||
		plain.(gts.FailurePreservingExternalScanner).PreservesStateOnScanFailure() ||
		plain.(gts.FailureStateRetainingExternalScanner).RetainsStateOnScanFailure() {
		t.Fatal("the recorder invents a capability for a scanner that has none")
	}
	if !plain.(interface{ SupportsErrorModeExternalRetry() bool }).SupportsErrorModeExternalRetry() ||
		!plain.(interface{ SupportsPackedGSSVersionOrder(int) bool }).SupportsPackedGSSVersionOrder(1<<20) {
		t.Fatal("the recorder must read true where the parser reads an absent interface as true")
	}
}

func TestRecorderKeepsACheckpointIdentity(t *testing.T) {
	identity := identityScanner{capableScanner{checkpointed: true}}
	got, ok := newRecorder(identity).install().(gts.ExternalScannerCheckpointIdentityProvider)
	if !ok || !got.UsesExternalScannerCheckpoints() {
		t.Fatal("the recorder hides the checkpoint identity")
	}
	if id, ok := got.CheckpointIdentity(); !ok || string(id.Scanner) != "scanner" || string(id.Grammar) != "grammar" {
		t.Fatalf("identity = %+v %v, want the scanner's own", id, ok)
	}
}
