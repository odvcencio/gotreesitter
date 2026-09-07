//go:build gts_parsercorephase0

package gotreesitter

import (
	"fmt"
	"reflect"
	"testing"
	"unsafe"

	core "github.com/odvcencio/gotreesitter/internal/parsercorephase0"
)

type diagnosticParserCoreZeroSnapshotScanner struct{ stateless bool }

func (diagnosticParserCoreZeroSnapshotScanner) Create() any                           { return nil }
func (diagnosticParserCoreZeroSnapshotScanner) Destroy(any)                           {}
func (diagnosticParserCoreZeroSnapshotScanner) Serialize(any, []byte) int             { return 0 }
func (diagnosticParserCoreZeroSnapshotScanner) Deserialize(any, []byte)               {}
func (diagnosticParserCoreZeroSnapshotScanner) Scan(any, *ExternalLexer, []bool) bool { return false }
func (s diagnosticParserCoreZeroSnapshotScanner) ExternalScannerIsStateless() bool {
	return s.stateless
}

type diagnosticParserCoreCountingSnapshotScanner struct{ serializeCalls *int }

func (diagnosticParserCoreCountingSnapshotScanner) Create() any { return nil }
func (diagnosticParserCoreCountingSnapshotScanner) Destroy(any) {}
func (s diagnosticParserCoreCountingSnapshotScanner) Serialize(_ any, buffer []byte) int {
	(*s.serializeCalls)++
	buffer[0] = 0x7b
	return 1
}
func (diagnosticParserCoreCountingSnapshotScanner) Deserialize(any, []byte) {}
func (diagnosticParserCoreCountingSnapshotScanner) Scan(any, *ExternalLexer, []bool) bool {
	return false
}

// DiagnosticParserCoreVersionLexerRequestWitnessForTest advances a scheduler
// to the Swift ragged-token frontier and exercises its private cursor requests.
// The helper stays in an internal test file so production callers cannot arm it.
func DiagnosticParserCoreVersionLexerRequestWitnessForTest(
	lang *Language,
	source []byte,
) ([]DiagnosticParserCoreVersionLexerRequest, error) {
	if lang == nil {
		return nil, fmt.Errorf("version lexer witness requires a language")
	}
	parser := NewParser(lang)
	runner, err := newAdmissionCandidateRunner(parser)
	if err != nil {
		return nil, err
	}
	runner.options.ReceiptMode = DiagnosticParserCoreReceiptFull
	tokenSource := parser.acquireParserDFATokenSource(source)
	if tokenSource == nil {
		return nil, fmt.Errorf("acquire parser DFA token source returned nil")
	}
	defer tokenSource.Close()

	tokenSource.SetParserState(lang.InitialState)
	tokenSource.SetGLRStates(nil)
	var scannerScratch []byte
	initialBytes := tokenSource.captureExternalScannerStateInto(&scannerScratch)
	initialID, initialInfo, err := diagnosticParserCoreInternCheckpoint(runner.compact, initialBytes)
	if err != nil {
		return nil, fmt.Errorf("intern initial scanner checkpoint: %w", err)
	}
	if err := runner.compact.SetPhaseCheckpoint(initialID); err != nil {
		return nil, fmt.Errorf("set initial scanner checkpoint phase: %w", err)
	}
	seed, err := runner.compact.Seed(core.StateID(lang.InitialState), 0)
	if err != nil {
		return nil, fmt.Errorf("seed compact parser core: %w", err)
	}
	scheduler, err := newDiagnosticParserCoreGenericScheduler(
		runner.compact, tokenSource, &scannerScratch, seed, initialID, initialInfo,
		diagnosticParserCoreSeedObserver{}, runner.options,
	)
	if err != nil {
		return nil, fmt.Errorf("new generic scheduler: %w", err)
	}
	if err := scheduler.captureSharedElectionSnapshot(); err != nil {
		return nil, fmt.Errorf("capture initial lexer election: %w", err)
	}
	if err := scheduler.elect(true); err != nil {
		return nil, fmt.Errorf("initial election: %w", err)
	}
	foundWitness := false
	for step := 0; step < 32; step++ {
		stop, dispatchErr := scheduler.dispatchPass()
		if dispatchErr != nil {
			return nil, fmt.Errorf("dispatch step %d: %w", step, dispatchErr)
		}
		if stop != nil {
			return nil, fmt.Errorf("scheduler stopped before the ragged witness: %+v", stop)
		}
		if len(scheduler.headers) == 2 && scheduler.token.StartByte == 3 && scheduler.token.EndByte == 5 {
			states := make([]StateID, len(scheduler.headers))
			for index, header := range scheduler.headers {
				state, byteOffset, boundaryErr := scheduler.compact.Boundary(header.head)
				if boundaryErr != nil {
					return nil, fmt.Errorf("read witness header %d boundary: %w", index, boundaryErr)
				}
				if byteOffset != 3 {
					return nil, fmt.Errorf("witness header %d byte offset=%d, want 3", index, byteOffset)
				}
				states[index] = StateID(state)
			}
			if states[0] == StateID(47) && states[1] == StateID(524) {
				foundWitness = true
				break
			}
		}
		allClosed := true
		for _, header := range scheduler.headers {
			if !header.shifted && !header.accepted {
				allClosed = false
				break
			}
		}
		if allClosed {
			if err := scheduler.captureSharedElectionSnapshot(); err != nil {
				return nil, fmt.Errorf("capture lexer election after dispatch step %d: %w", step, err)
			}
			if err := scheduler.elect(false); err != nil {
				return nil, fmt.Errorf("next election after dispatch step %d: %w", step, err)
			}
		}
	}
	if !foundWitness {
		return nil, fmt.Errorf("did not reach two-header byte-3 witness")
	}
	capturedElection := scheduler.versionLexerBeforeElection
	scheduler.versionLexerBeforeElection--
	if err := scheduler.seedVersionLexerOwnership(); err == nil {
		return nil, fmt.Errorf("stale lexer election snapshot was accepted")
	}
	scheduler.versionLexerBeforeElection = capturedElection
	if err := scheduler.seedVersionLexerOwnership(); err != nil {
		return nil, fmt.Errorf("seed per-header lexer ownership: %w", err)
	}
	if scheduler.headers[0].versionState != scheduler.headers[1].versionState {
		return nil, fmt.Errorf("equal lexer cursors did not share one version identity")
	}

	for index := range scheduler.headers {
		priorDFA := tokenSource.snapshotRelexState()
		priorState := tokenSource.state
		priorGLRStates := append([]StateID(nil), tokenSource.glrStates...)
		if err := scheduler.requestHeaderLexerToken(index); err != nil {
			return nil, fmt.Errorf("request header lexer token %d: %w", index, err)
		}
		if got := tokenSource.snapshotRelexState(); !reflect.DeepEqual(got, priorDFA) {
			return nil, fmt.Errorf("request %d changed the shared DFA cursor: got=%+v want=%+v", index, got, priorDFA)
		}
		if tokenSource.state != priorState {
			return nil, fmt.Errorf("request %d changed the shared parser state: got=%d want=%d", index, tokenSource.state, priorState)
		}
		if !reflect.DeepEqual(tokenSource.glrStates, priorGLRStates) {
			return nil, fmt.Errorf("request %d changed the shared GLR states: got=%v want=%v", index, tokenSource.glrStates, priorGLRStates)
		}
	}

	if len(scheduler.versionLexerRequests) != 2 {
		return nil, fmt.Errorf("owned lexer requests=%d, want 2", len(scheduler.versionLexerRequests))
	}
	scheduler.electionIndex++
	if request := scheduler.versionLexerRequestForHeader(0); request != nil {
		return nil, fmt.Errorf("owned lexer request survived its election")
	}
	scheduler.electionIndex--
	if scheduler.work.PerVersionLexRequests != 2 || scheduler.work.PerVersionLexRestores != 2 ||
		scheduler.work.PerVersionLexPublications != 6 || scheduler.work.PeakLiveVersions != 2 {
		return nil, fmt.Errorf("owned lexer work counters=%+v", scheduler.work)
	}
	scheduler.publishTotals()
	if scheduler.receipt.PerVersionLexRequests != 2 || scheduler.receipt.PerVersionLexRestores != 2 ||
		scheduler.receipt.PerVersionLexPublications != 6 || scheduler.receipt.PeakLiveVersions != 2 {
		return nil, fmt.Errorf("owned lexer receipt totals=%+v", scheduler.receipt)
	}
	want := map[StateID]struct {
		symbol      Symbol
		endByte     uint32
		external    bool
		internalDFA bool
	}{
		47:  {symbol: 35, endByte: 4, internalDFA: true},
		524: {symbol: 217, endByte: 5, external: true},
	}
	for _, request := range scheduler.versionLexerRequests {
		expect, ok := want[request.state]
		if !ok {
			return nil, fmt.Errorf("unexpected owned request state=%d", request.state)
		}
		if request.token.Symbol != expect.symbol || request.token.StartByte != 3 || request.token.EndByte != expect.endByte {
			return nil, fmt.Errorf("state %d token=%+v, want symbol=%d span=3..%d", request.state, request.token, expect.symbol, expect.endByte)
		}
		if request.token.ExternalScannerToken != expect.external || request.token.lexerInternalDFALexed() != expect.internalDFA {
			return nil, fmt.Errorf("state %d token provenance external=%t internalDFA=%t, want external=%t internalDFA=%t", request.state, request.token.ExternalScannerToken, request.token.lexerInternalDFALexed(), expect.external, expect.internalDFA)
		}
		if request.before == nil || request.after == nil || request.beforeCheckpoint.Length != 9 || request.afterCheckpoint.Length != 9 {
			return nil, fmt.Errorf("state %d request lost owned scanner checkpoints: %+v", request.state, request)
		}
		if request.before.dfa.lexerPos != 3 || request.after.dfa.lexerPos != int(expect.endByte) {
			return nil, fmt.Errorf("state %d cursor positions=%d..%d, want 3..%d", request.state, request.before.dfa.lexerPos, request.after.dfa.lexerPos, expect.endByte)
		}
	}
	return scheduler.receipt.VersionLexerRequests, nil
}

func TestDiagnosticParserCoreVersionLexerElectionCaptureReusesScratch(t *testing.T) {
	scanner := byteStateExternalScanner{}
	payload := scanner.Create()
	*payload.(*byte) = 0x5a
	tokenSource := &dfaTokenSource{
		lexer: &Lexer{
			source:                 []byte("abc"),
			pos:                    2,
			row:                    3,
			col:                    4,
			includedRangeIdx:       1,
			failTokenStartPos:      1,
			failTokenStartRow:      2,
			failTokenStartCol:      3,
			failTokenStartRangeIdx: 1,
		},
		language:                    &Language{Name: "version-lexer-scratch-test", ExternalScanner: scanner},
		hasExternalScanner:          true,
		externalPayload:             payload,
		lastExternalTokenStartByte:  5,
		lastExternalTokenEndByte:    8,
		lastExternalTokenValid:      true,
		lastExternalTokenWasExtra:   true,
		externalTokenEndSameAsStart: true,
		lastTokenStartByte:          4,
		lastTokenEndByte:            8,
		lastTokenValid:              true,
		externalTokenStart:          []byte{0x11, 0x12},
		externalTokenEnd:            []byte{0x21, 0x22},
		extZeroPos:                  7,
		extZeroState:                23,
		extZeroTried:                []bool{true, false, true},
		zeroWidthPos:                8,
		zeroWidthCount:              2,
	}
	scheduler := diagnosticParserCoreGenericScheduler{
		tokenSource:   tokenSource,
		electionIndex: 4,
		checkpointID:  7,
	}
	if err := scheduler.captureSharedElectionSnapshot(); err != nil {
		t.Fatal(err)
	}
	payloadPointer := unsafe.Pointer(&scheduler.versionLexerBeforeScratch.externalPayload[:cap(scheduler.versionLexerBeforeScratch.externalPayload)][0])
	startPointer := unsafe.Pointer(&scheduler.versionLexerBeforeScratch.externalTokenStart[:cap(scheduler.versionLexerBeforeScratch.externalTokenStart)][0])
	endPointer := unsafe.Pointer(&scheduler.versionLexerBeforeScratch.externalTokenEnd[:cap(scheduler.versionLexerBeforeScratch.externalTokenEnd)][0])
	triedPointer := unsafe.Pointer(&scheduler.versionLexerBeforeScratch.extZeroTried[:cap(scheduler.versionLexerBeforeScratch.extZeroTried)][0])
	var captureErr error
	if allocs := testing.AllocsPerRun(100, func() {
		captureErr = scheduler.captureSharedElectionSnapshot()
	}); allocs != 0 {
		t.Fatalf("warm election snapshot allocations=%v, want 0", allocs)
	}
	if captureErr != nil {
		t.Fatal(captureErr)
	}
	if payloadPointer != unsafe.Pointer(&scheduler.versionLexerBeforeScratch.externalPayload[:cap(scheduler.versionLexerBeforeScratch.externalPayload)][0]) ||
		startPointer != unsafe.Pointer(&scheduler.versionLexerBeforeScratch.externalTokenStart[:cap(scheduler.versionLexerBeforeScratch.externalTokenStart)][0]) ||
		endPointer != unsafe.Pointer(&scheduler.versionLexerBeforeScratch.externalTokenEnd[:cap(scheduler.versionLexerBeforeScratch.externalTokenEnd)][0]) ||
		triedPointer != unsafe.Pointer(&scheduler.versionLexerBeforeScratch.extZeroTried[:cap(scheduler.versionLexerBeforeScratch.extZeroTried)][0]) {
		t.Fatal("warm election capture replaced reusable backing storage")
	}
	got := scheduler.versionLexerBefore
	if got.lexerPos != 2 || got.lexerRow != 3 || got.lexerCol != 4 || got.lexerRangeIdx != 1 ||
		got.failTokenStartPos != 1 || got.failTokenStartRow != 2 || got.failTokenStartCol != 3 || got.failTokenStartRangeIdx != 1 ||
		!reflect.DeepEqual(got.externalPayload, []byte{0x5a}) || !reflect.DeepEqual(got.externalTokenStart, []byte{0x11, 0x12}) ||
		!reflect.DeepEqual(got.externalTokenEnd, []byte{0x21, 0x22}) || !reflect.DeepEqual(got.extZeroTried, []bool{true, false, true}) {
		t.Fatalf("reused election snapshot lost state: %+v", got)
	}
	tokenSource.externalTokenStart[0] = 0xff
	tokenSource.externalTokenEnd[0] = 0xee
	tokenSource.extZeroTried[0] = false
	*payload.(*byte) = 0xdd
	tokenSource.lexer.pos = 99
	tokenSource.lexer.row = 98
	tokenSource.lexer.col = 97
	tokenSource.lexer.includedRangeIdx = 96
	tokenSource.lexer.failTokenStartPos = 95
	tokenSource.lexer.failTokenStartRow = 94
	tokenSource.lexer.failTokenStartCol = 93
	tokenSource.lexer.failTokenStartRangeIdx = 92
	tokenSource.lastExternalTokenStartByte = 91
	tokenSource.lastExternalTokenEndByte = 90
	tokenSource.lastExternalTokenValid = false
	tokenSource.lastExternalTokenWasExtra = false
	tokenSource.externalTokenEndSameAsStart = false
	tokenSource.lastTokenStartByte = 89
	tokenSource.lastTokenEndByte = 88
	tokenSource.lastTokenValid = false
	tokenSource.extZeroPos = 87
	tokenSource.extZeroState = 86
	tokenSource.zeroWidthPos = 85
	tokenSource.zeroWidthCount = 84
	if !reflect.DeepEqual(got.externalPayload, []byte{0x5a}) || !reflect.DeepEqual(got.externalTokenStart, []byte{0x11, 0x12}) ||
		!reflect.DeepEqual(got.externalTokenEnd, []byte{0x21, 0x22}) || !reflect.DeepEqual(got.extZeroTried, []bool{true, false, true}) {
		t.Fatalf("live token source mutated the captured election state: %+v", got)
	}
	got.restore(tokenSource)
	if restored := tokenSource.snapshotRelexState(); !reflect.DeepEqual(restored, got) {
		t.Fatalf("reused election snapshot restore=%+v, want %+v", restored, got)
	}
}

func TestDiagnosticParserCoreVersionLexerElectionReusesCheckpointSerialization(t *testing.T) {
	serializeCalls := 0
	scanner := diagnosticParserCoreCountingSnapshotScanner{serializeCalls: &serializeCalls}
	tokenSource := &dfaTokenSource{
		lexer:              &Lexer{source: []byte("abc"), pos: 2},
		language:           &Language{Name: "version-lexer-serialization-test", ExternalScanner: scanner},
		hasExternalScanner: true,
	}
	scheduler := diagnosticParserCoreGenericScheduler{
		tokenSource:   tokenSource,
		electionIndex: 4,
		checkpointID:  7,
	}
	var checkpointScratch []byte
	payload := tokenSource.captureExternalScannerStateInto(&checkpointScratch)
	if serializeCalls != 1 {
		t.Fatalf("checkpoint Serialize calls=%d, want 1", serializeCalls)
	}
	if err := scheduler.captureSharedElectionSnapshotFromExternalPayload(payload); err != nil {
		t.Fatal(err)
	}
	if serializeCalls != 1 {
		t.Fatalf("election snapshot Serialize calls=%d, want no second call", serializeCalls)
	}
	if !reflect.DeepEqual(scheduler.versionLexerBefore.externalPayload, []byte{0x7b}) {
		t.Fatalf("election payload=%v, want [123]", scheduler.versionLexerBefore.externalPayload)
	}
	checkpointScratch[0] = 0xff
	if !reflect.DeepEqual(scheduler.versionLexerBefore.externalPayload, []byte{0x7b}) {
		t.Fatalf("checkpoint scratch mutated election payload=%v", scheduler.versionLexerBefore.externalPayload)
	}
}

func TestDiagnosticParserCoreVersionLexerElectionScratchPreservesZeroPayload(t *testing.T) {
	for _, stateless := range []bool{false, true} {
		t.Run(fmt.Sprintf("stateless_%t", stateless), func(t *testing.T) {
			scanner := diagnosticParserCoreZeroSnapshotScanner{stateless: stateless}
			tokenSource := &dfaTokenSource{
				lexer:              &Lexer{source: []byte("x")},
				language:           &Language{Name: "zero-snapshot-test", ExternalScanner: scanner},
				hasExternalScanner: true,
			}
			want := tokenSource.snapshotRelexState()
			var scratch dfaRelexSnapshotScratch
			got := tokenSource.snapshotRelexStateWithScratch(&scratch)
			if !reflect.DeepEqual(got, want) || got.externalPayload != nil {
				t.Fatalf("zero-payload scratch snapshot=%+v, want %+v", got, want)
			}
			if len(scratch.externalPayload) != 0 || cap(scratch.externalPayload) != externalScannerSerializationBufferSize {
				t.Fatalf("zero-payload scratch len/cap=%d/%d, want 0/%d", len(scratch.externalPayload), cap(scratch.externalPayload), externalScannerSerializationBufferSize)
			}
			var next dfaRelexSnapshot
			if allocs := testing.AllocsPerRun(100, func() {
				next = tokenSource.snapshotRelexStateWithScratch(&scratch)
			}); allocs != 0 {
				t.Fatalf("warm zero-payload snapshot allocations=%v, want 0", allocs)
			}
			if !reflect.DeepEqual(next, want) || next.externalPayload != nil {
				t.Fatalf("warm zero-payload scratch snapshot=%+v, want %+v", next, want)
			}
		})
	}
}

func TestDiagnosticParserCoreVersionLexerSidecarFootprintAndReset(t *testing.T) {
	compact, snapshot := newDiagnosticParserCoreVersionLexerTestSnapshot(t)
	requests := make([]diagnosticParserCoreVersionLexerRequest, 1, 3)
	requests[0] = diagnosticParserCoreVersionLexerRequest{before: snapshot, after: snapshot, valid: true}
	requests[:cap(requests)][2] = diagnosticParserCoreVersionLexerRequest{before: snapshot, after: snapshot, valid: true}
	scheduler := diagnosticParserCoreGenericScheduler{
		compact:                      compact,
		versionLexerBefore:           cloneDiagnosticParserCoreDFARelexSnapshot(snapshot.dfa),
		versionLexerBeforeValid:      true,
		versionLexerBeforeElection:   7,
		versionLexerBeforeCheckpoint: snapshot.beforeCheckpoint,
		versionLexerRequests:         requests,
	}
	base := diagnosticParserCoreSchedulerFootprintBytes(&diagnosticParserCoreGenericScheduler{compact: compact})
	delta := diagnosticParserCoreSchedulerFootprintBytes(&scheduler) - base
	want := uint64(cap(requests))*uint64(unsafe.Sizeof(diagnosticParserCoreVersionLexerRequest{})) +
		diagnosticParserCoreVersionLexerSnapshotFootprintBytes(snapshot) +
		diagnosticParserCoreDFARelexSnapshotRetainedBytes(scheduler.versionLexerBefore) +
		uint64(cap(scheduler.footprintRefs))*uint64(unsafe.Sizeof(diagnosticParserCoreFootprintRef{}))
	if delta != want {
		t.Fatalf("version lexer sidecar footprint=%d, want %d", delta, want)
	}
	if err := resetDiagnosticParserCoreGenericScheduler(&scheduler); err != nil {
		t.Fatalf("reset scheduler: %v", err)
	}
	for index, request := range requests[:cap(requests)] {
		if request.before != nil || request.after != nil || request.valid {
			t.Fatalf("request backing slot %d retained a snapshot: %+v", index, request)
		}
	}
	if scheduler.versionLexerBeforeValid || scheduler.versionLexerBeforeElection != 0 || scheduler.versionLexerBeforeCheckpoint != 0 ||
		scheduler.versionLexerBefore.externalPayload != nil ||
		scheduler.versionLexerBefore.externalTokenStart != nil || scheduler.versionLexerBefore.externalTokenEnd != nil ||
		scheduler.versionLexerBefore.extZeroTried != nil {
		t.Fatalf("reset retained the shared election snapshot: %+v", scheduler.versionLexerBefore)
	}
}

func TestDiagnosticParserCoreVersionLexerElectionScratchFootprintAndReset(t *testing.T) {
	payload := make([]byte, 1, externalScannerSerializationBufferSize)
	payload[0] = 0x11
	start := make([]byte, 2, externalScannerSerializationBufferSize)
	copy(start, []byte{0x21, 0x22})
	end := make([]byte, 2, externalScannerSerializationBufferSize)
	copy(end, []byte{0x31, 0x32})
	tried := make([]bool, 3, 6)
	copy(tried, []bool{true, false, true})
	scratch := dfaRelexSnapshotScratch{
		externalPayload: payload, externalTokenStart: start,
		externalTokenEnd: end, extZeroTried: tried,
	}
	scheduler := diagnosticParserCoreGenericScheduler{
		versionLexerBefore: dfaRelexSnapshot{
			externalPayload: payload, externalTokenStart: start,
			externalTokenEnd: end, extZeroTried: tried,
		},
		versionLexerBeforeScratch: scratch,
		versionLexerBeforeValid:   true,
	}
	base := diagnosticParserCoreSchedulerFootprintBytes(&diagnosticParserCoreGenericScheduler{})
	delta := diagnosticParserCoreSchedulerFootprintBytes(&scheduler) - base
	want := uint64(cap(payload) + cap(start) + cap(end) + cap(tried))
	if delta != want {
		t.Fatalf("aliased election snapshot/scratch footprint=%d, want exact %d", delta, want)
	}
	if err := resetDiagnosticParserCoreGenericScheduler(&scheduler); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(scheduler.versionLexerBefore, dfaRelexSnapshot{}) || scheduler.versionLexerBeforeValid {
		t.Fatalf("reset retained an active election snapshot: %+v", scheduler.versionLexerBefore)
	}
	retained := scheduler.versionLexerBeforeScratch
	if len(retained.externalPayload) != 0 || cap(retained.externalPayload) != externalScannerSerializationBufferSize ||
		len(retained.externalTokenStart) != 0 || cap(retained.externalTokenStart) != cap(start) ||
		len(retained.externalTokenEnd) != 0 || cap(retained.externalTokenEnd) != cap(end) ||
		len(retained.extZeroTried) != 0 || cap(retained.extZeroTried) != cap(tried) {
		t.Fatalf("reset election scratch capacities=%+v", retained)
	}
	for index, value := range retained.externalPayload[:cap(retained.externalPayload)] {
		if value != 0 {
			t.Fatalf("reset payload scratch byte %d=%d, want 0", index, value)
		}
	}
	for name, values := range map[string][]byte{
		"start": retained.externalTokenStart[:cap(retained.externalTokenStart)],
		"end":   retained.externalTokenEnd[:cap(retained.externalTokenEnd)],
	} {
		for index, value := range values {
			if value != 0 {
				t.Fatalf("reset %s scratch byte %d=%d, want 0", name, index, value)
			}
		}
	}
	for index, value := range retained.extZeroTried[:cap(retained.extZeroTried)] {
		if value {
			t.Fatalf("reset zero-width scratch bit %d=true, want false", index)
		}
	}
	scanner := byteStateExternalScanner{}
	scannerPayload := scanner.Create()
	*scannerPayload.(*byte) = 0x41
	scheduler.tokenSource = &dfaTokenSource{
		lexer:              &Lexer{source: []byte("abc")},
		language:           &Language{Name: "version-lexer-reset-scratch-test", ExternalScanner: scanner},
		hasExternalScanner: true,
		externalPayload:    scannerPayload,
		externalTokenStart: []byte{0x51, 0x52},
		externalTokenEnd:   []byte{0x61, 0x62},
		extZeroTried:       []bool{true, false, true},
	}
	var captureErr error
	if allocs := testing.AllocsPerRun(100, func() {
		captureErr = scheduler.captureSharedElectionSnapshot()
	}); allocs != 0 {
		t.Fatalf("post-reset warm election snapshot allocations=%v, want 0", allocs)
	}
	if captureErr != nil {
		t.Fatal(captureErr)
	}
}
