//go:build gts_parsercorephase0

package gotreesitter

import (
	"bytes"
	"reflect"
	"strings"
	"testing"
	"unsafe"

	core "github.com/odvcencio/gotreesitter/internal/parsercorephase0"
)

func diagnosticParserCoreVersionLexerTestDFA() dfaRelexSnapshot {
	return dfaRelexSnapshot{
		lexerPos:                    17,
		lexerRow:                    3,
		lexerCol:                    9,
		lexerRangeIdx:               2,
		failTokenStartPos:           13,
		failTokenStartRow:           2,
		failTokenStartCol:           8,
		failTokenStartRangeIdx:      1,
		externalPayload:             []byte{0x11, 0x22},
		lastExternalTokenStartByte:  12,
		lastExternalTokenEndByte:    17,
		lastExternalTokenValid:      true,
		lastExternalTokenWasExtra:   true,
		externalTokenEndSameAsStart: true,
		lastTokenStartByte:          12,
		lastTokenEndByte:            17,
		lastTokenValid:              true,
		externalTokenStart:          []byte{0x31, 0x32},
		externalTokenEnd:            []byte{0x41, 0x42},
		extZeroPos:                  17,
		extZeroState:                23,
		extZeroTried:                []bool{true, false, true},
		zeroWidthPos:                17,
		zeroWidthCount:              2,
	}
}

func newDiagnosticParserCoreVersionLexerTestCore(t *testing.T) (*core.Core, core.CheckpointID, core.CheckpointID) {
	t.Helper()
	compact, err := core.New(&genericConflictTable{}, core.Limits{})
	if err != nil {
		t.Fatal(err)
	}
	before, err := compact.InternCheckpoint([]byte{
		0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08, 0x09,
	})
	if err != nil {
		t.Fatalf("intern before checkpoint: %v", err)
	}
	after, err := compact.InternCheckpoint([]byte{
		0x11, 0x12, 0x13, 0x14, 0x15, 0x16, 0x17, 0x18, 0x19,
	})
	if err != nil {
		t.Fatalf("intern after checkpoint: %v", err)
	}
	return compact, before, after
}

func newDiagnosticParserCoreVersionLexerTestSnapshot(t *testing.T) (*core.Core, *diagnosticParserCoreVersionLexerSnapshot) {
	t.Helper()
	compact, before, after := newDiagnosticParserCoreVersionLexerTestCore(t)
	dfa := diagnosticParserCoreVersionLexerTestDFA()
	dfa.externalScannerPresent = true
	language := &Language{Name: "snapshot-test", ExternalScanner: byteStateExternalScanner{}}
	var snapshot *diagnosticParserCoreVersionLexerSnapshot
	err := compact.ApplySchedulerAtomic(func(owner core.SchedulerTransactionToken) error {
		var err error
		snapshot, err = newDiagnosticParserCoreVersionLexerSnapshot(compact, language, owner, dfa, before, after)
		return err
	})
	if err != nil {
		t.Fatalf("construct owned lexer snapshot: %v", err)
	}
	return compact, snapshot
}

func TestDiagnosticParserCoreVersionLexerSnapshotCopiesAllMutableState(t *testing.T) {
	compact, before, after := newDiagnosticParserCoreVersionLexerTestCore(t)
	dfa := diagnosticParserCoreVersionLexerTestDFA()
	dfa.externalScannerPresent = true
	language := &Language{Name: "snapshot-test", ExternalScanner: byteStateExternalScanner{}}
	var snapshot *diagnosticParserCoreVersionLexerSnapshot
	err := compact.ApplySchedulerAtomic(func(owner core.SchedulerTransactionToken) error {
		var err error
		snapshot, err = newDiagnosticParserCoreVersionLexerSnapshot(compact, language, owner, dfa, before, after)
		return err
	})
	if err != nil {
		t.Fatalf("construct owned lexer snapshot: %v", err)
	}
	if snapshot == nil {
		t.Fatal("constructor returned a nil snapshot")
	}
	if snapshot.compact != compact || snapshot.coreGeneration != compact.ResetGeneration() || snapshot.coreGeneration == 0 {
		t.Fatalf("snapshot core binding=%p generation=%d, want core=%p generation=%d", snapshot.compact, snapshot.coreGeneration, compact, compact.ResetGeneration())
	}
	if snapshot.beforeCheckpoint != before || snapshot.afterCheckpoint != after {
		t.Fatalf("checkpoint IDs=%d/%d, want %d/%d", snapshot.beforeCheckpoint, snapshot.afterCheckpoint, before, after)
	}
	if snapshot.beforeCheckpointInfo != parserCoreCheckpoint(snapshot.beforeCheckpointBytes) ||
		snapshot.afterCheckpointInfo != parserCoreCheckpoint(snapshot.afterCheckpointBytes) {
		t.Fatalf("checkpoint metadata does not describe owned bytes: before=%+v after=%+v", snapshot.beforeCheckpointInfo, snapshot.afterCheckpointInfo)
	}

	dfa.externalPayload[0] = 0xff
	dfa.externalTokenStart[0] = 0xff
	dfa.externalTokenEnd[0] = 0xff
	dfa.extZeroTried[0] = false
	dfa.lexerPos = 99
	if !bytes.Equal(snapshot.dfa.externalPayload, []byte{0x11, 0x22}) ||
		!bytes.Equal(snapshot.dfa.externalTokenStart, []byte{0x31, 0x32}) ||
		!bytes.Equal(snapshot.dfa.externalTokenEnd, []byte{0x41, 0x42}) ||
		len(snapshot.dfa.extZeroTried) != 3 || !snapshot.dfa.extZeroTried[0] || snapshot.dfa.lexerPos != 17 ||
		snapshot.dfa.lexerRow != 3 || snapshot.dfa.lexerCol != 9 || snapshot.dfa.lexerRangeIdx != 2 ||
		snapshot.dfa.failTokenStartPos != 13 || snapshot.dfa.failTokenStartRow != 2 ||
		snapshot.dfa.failTokenStartCol != 8 || snapshot.dfa.failTokenStartRangeIdx != 1 ||
		snapshot.dfa.lastExternalTokenStartByte != 12 || snapshot.dfa.lastExternalTokenEndByte != 17 ||
		!snapshot.dfa.lastExternalTokenValid || !snapshot.dfa.lastExternalTokenWasExtra ||
		!snapshot.dfa.externalTokenEndSameAsStart || snapshot.dfa.lastTokenStartByte != 12 ||
		snapshot.dfa.lastTokenEndByte != 17 || !snapshot.dfa.lastTokenValid || snapshot.dfa.extZeroPos != 17 ||
		snapshot.dfa.extZeroState != 23 || snapshot.dfa.zeroWidthPos != 17 || snapshot.dfa.zeroWidthCount != 2 {
		t.Fatalf("snapshot aliases or lost source DFA state: %+v", snapshot.dfa)
	}
	if &snapshot.beforeCheckpointBytes[0] == &snapshot.afterCheckpointBytes[0] {
		t.Fatal("checkpoint copies unexpectedly alias each other")
	}
	if !bytes.Equal(snapshot.beforeCheckpointBytes, []byte{
		0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08, 0x09,
	}) || !bytes.Equal(snapshot.afterCheckpointBytes, []byte{
		0x11, 0x12, 0x13, 0x14, 0x15, 0x16, 0x17, 0x18, 0x19,
	}) {
		t.Fatalf("owned checkpoint bytes changed: before=%v after=%v", snapshot.beforeCheckpointBytes, snapshot.afterCheckpointBytes)
	}
}

func TestDiagnosticParserCoreVersionLexerSnapshotPublicationCopiesPublishedState(t *testing.T) {
	compact, snapshot := newDiagnosticParserCoreVersionLexerTestSnapshot(t)
	var header diagnosticParserCoreHeader
	if err := header.publishVersionLexerSnapshot(compact, snapshot.language, snapshot); err != nil {
		t.Fatalf("publish lexer snapshot: %v", err)
	}
	published := header.versionLexerSnapshot()
	if published == nil || published == snapshot {
		t.Fatal("publication did not create a distinct snapshot")
	}

	// Mutating the constructor result must not mutate the header-owned copy.
	snapshot.dfa.externalPayload[0] = 0xff
	snapshot.dfa.externalTokenStart[0] = 0xff
	snapshot.dfa.extZeroTried[0] = false
	snapshot.dfa.failTokenStartPos = 99
	snapshot.beforeCheckpointBytes[0] = 0xff
	if published.dfa.externalPayload[0] != 0x11 || published.dfa.externalTokenStart[0] != 0x31 ||
		!published.dfa.extZeroTried[0] || published.dfa.failTokenStartPos != 13 ||
		published.beforeCheckpointBytes[0] != 0x01 {
		t.Fatal("published snapshot aliases the constructor result")
	}

	// Mutating the published copy must not mutate the constructor result either.
	published.dfa.externalTokenEnd[0] = 0xee
	published.dfa.failTokenStartRow = 88
	published.afterCheckpointBytes[0] = 0xee
	if snapshot.dfa.externalTokenEnd[0] != 0x41 || snapshot.dfa.failTokenStartRow != 2 ||
		snapshot.afterCheckpointBytes[0] != 0x11 {
		t.Fatal("constructor snapshot aliases the published copy")
	}
}

func TestDiagnosticParserCoreVersionLexerSnapshotCloneDoesNotAlias(t *testing.T) {
	_, snapshot := newDiagnosticParserCoreVersionLexerTestSnapshot(t)
	clone := snapshot.clone()
	if clone == nil || clone == snapshot {
		t.Fatal("clone did not create a distinct snapshot")
	}
	clone.dfa.externalPayload[0] = 0xff
	clone.dfa.externalTokenStart[0] = 0xff
	clone.dfa.externalTokenEnd[0] = 0xff
	clone.dfa.extZeroTried[0] = false
	clone.beforeCheckpointBytes[0] = 0xff
	clone.afterCheckpointBytes[0] = 0xff
	if snapshot.dfa.externalPayload[0] != 0x11 || snapshot.dfa.externalTokenStart[0] != 0x31 ||
		snapshot.dfa.externalTokenEnd[0] != 0x41 || !snapshot.dfa.extZeroTried[0] ||
		snapshot.beforeCheckpointBytes[0] != 0x01 || snapshot.afterCheckpointBytes[0] != 0x11 {
		t.Fatal("snapshot clone aliases mutable state")
	}
}

func TestDiagnosticParserCoreVersionLexerSnapshotInterleavedErrorRunRestoration(t *testing.T) {
	states := []LexState{
		{
			Default: -1,
			EOF:     -1,
			Transitions: []LexTransition{
				{Lo: 'a', Hi: 'a', NextState: 2},
			},
		},
		{
			Default: -1,
			EOF:     -1,
			Transitions: []LexTransition{
				{Lo: 'a', Hi: 'a', NextState: 2},
			},
		},
		{AcceptToken: 1, Default: -1, EOF: -1},
	}
	d := &dfaTokenSource{lexer: &Lexer{
		states:                 states,
		source:                 []byte("xax"),
		failTokenStartPos:      77,
		failTokenStartRow:      8,
		failTokenStartCol:      9,
		failTokenStartRangeIdx: 6,
	}, language: &Language{LexStates: states}}
	d.lexer.errorRunLexState = 1
	d.lexer.hasErrorRunLexState = true

	first := d.snapshotRelexState()
	if tok := d.lexer.NextWithErrorRuns(0); tok.Symbol != errorSymbol || tok.StartByte != 0 || tok.EndByte != 1 {
		t.Fatalf("first error run = %+v, want [0,1)", tok)
	}
	second := d.snapshotRelexState()
	if tok := d.lexer.NextWithErrorRuns(0); tok.Symbol != 1 || tok.StartByte != 1 || tok.EndByte != 2 {
		t.Fatalf("delimiter token = %+v, want [1,2)", tok)
	}
	if tok := d.lexer.NextWithErrorRuns(0); tok.Symbol != errorSymbol || tok.StartByte != 2 || tok.EndByte != 3 {
		t.Fatalf("second error run = %+v, want [2,3)", tok)
	}
	if d.lexer.failTokenStartPos != 2 || d.lexer.failTokenStartCol != 2 {
		t.Fatalf("second failure origin = %d/%d, want 2/2", d.lexer.failTokenStartPos, d.lexer.failTokenStartCol)
	}

	first.restore(d)
	if d.lexer.pos != first.lexerPos || d.lexer.row != first.lexerRow || d.lexer.col != first.lexerCol ||
		d.lexer.includedRangeIdx != first.lexerRangeIdx || d.lexer.failTokenStartPos != 77 ||
		d.lexer.failTokenStartRow != 8 || d.lexer.failTokenStartCol != 9 || d.lexer.failTokenStartRangeIdx != 6 {
		t.Fatalf("first interleaved restore lost lexer failure state: %+v", *d.lexer)
	}
	if tok := d.lexer.NextWithErrorRuns(0); tok.Symbol != errorSymbol || tok.StartByte != 0 || tok.EndByte != 1 {
		t.Fatalf("replayed first error run = %+v, want [0,1)", tok)
	}
	second.restore(d)
	if d.lexer.pos != second.lexerPos || d.lexer.row != second.lexerRow || d.lexer.col != second.lexerCol ||
		d.lexer.includedRangeIdx != second.lexerRangeIdx || d.lexer.failTokenStartPos != second.failTokenStartPos ||
		d.lexer.failTokenStartRow != second.failTokenStartRow || d.lexer.failTokenStartCol != second.failTokenStartCol ||
		d.lexer.failTokenStartRangeIdx != second.failTokenStartRangeIdx {
		t.Fatalf("second interleaved restore lost lexer failure state: %+v", *d.lexer)
	}
}

func TestDiagnosticParserCoreVersionLexerSnapshotAcceptsEmptyCheckpointIDs(t *testing.T) {
	compact, err := core.New(&genericConflictTable{}, core.Limits{})
	if err != nil {
		t.Fatal(err)
	}
	dfa := diagnosticParserCoreVersionLexerTestDFA()
	dfa.externalPayload = nil
	language := &Language{Name: "snapshot-empty-test"}
	var snapshot *diagnosticParserCoreVersionLexerSnapshot
	err = compact.ApplySchedulerAtomic(func(owner core.SchedulerTransactionToken) error {
		var err error
		snapshot, err = newDiagnosticParserCoreVersionLexerSnapshot(compact, language, owner, dfa, 0, 0)
		return err
	})
	if err != nil {
		t.Fatalf("construct empty-checkpoint snapshot: %v", err)
	}
	if snapshot.beforeCheckpoint != 0 || snapshot.afterCheckpoint != 0 || snapshot.beforeCheckpointBytes != nil || snapshot.afterCheckpointBytes != nil ||
		snapshot.beforeCheckpointInfo != parserCoreEmptyCheckpoint || snapshot.afterCheckpointInfo != parserCoreEmptyCheckpoint {
		t.Fatalf("empty checkpoint state=%+v, want zero IDs and empty receipts", snapshot)
	}
	if err := snapshot.validate(); err != nil {
		t.Fatalf("validate empty-checkpoint snapshot: %v", err)
	}
}

func TestDiagnosticParserCoreVersionLexerSnapshotSurvivesPhaseChangesAndRejectsReset(t *testing.T) {
	compact, snapshot := newDiagnosticParserCoreVersionLexerTestSnapshot(t)
	lang := snapshot.language
	payload := lang.ExternalScanner.Create()
	ts := &dfaTokenSource{
		lexer:              &Lexer{source: []byte("abc"), pos: 4, row: 5, col: 6},
		language:           lang,
		hasExternalScanner: true,
		externalPayload:    payload,
	}
	before := *ts.lexer
	resetGeneration := compact.ResetGeneration()
	if err := compact.BeginFrontier(); err != nil {
		t.Fatalf("advance core frontier: %v", err)
	}
	if compact.ResetGeneration() != resetGeneration {
		t.Fatalf("frontier advanced reset generation to %d, want %d", compact.ResetGeneration(), resetGeneration)
	}
	if err := compact.SetPhaseCheckpoint(snapshot.afterCheckpoint); err != nil {
		t.Fatalf("advance checkpoint phase: %v", err)
	}
	if compact.ResetGeneration() != resetGeneration {
		t.Fatalf("checkpoint advanced reset generation to %d, want %d", compact.ResetGeneration(), resetGeneration)
	}
	if err := snapshot.restore(compact, ts); err != nil {
		t.Fatalf("restore rejected a snapshot across phase changes: %v", err)
	}
	if ts.lexer.pos != snapshot.dfa.lexerPos || ts.lexer.row != snapshot.dfa.lexerRow || ts.lexer.col != snapshot.dfa.lexerCol ||
		ts.lexer.includedRangeIdx != snapshot.dfa.lexerRangeIdx || ts.lexer.failTokenStartPos != snapshot.dfa.failTokenStartPos ||
		ts.lexer.failTokenStartRow != snapshot.dfa.failTokenStartRow || ts.lexer.failTokenStartCol != snapshot.dfa.failTokenStartCol ||
		ts.lexer.failTokenStartRangeIdx != snapshot.dfa.failTokenStartRangeIdx {
		t.Fatalf("phase-change restore lost lexer state: got=%+v want=%+v", *ts.lexer, snapshot.dfa)
	}
	if err := compact.Reset(); err != nil {
		t.Fatalf("reset compact core: %v", err)
	}
	if compact.ResetGeneration() == resetGeneration {
		t.Fatalf("reset did not advance reset generation: %d", resetGeneration)
	}
	before = *ts.lexer
	if err := snapshot.restore(compact, ts); err == nil {
		t.Fatal("restore accepted a snapshot from an older reset generation")
	}
	if ts.lexer.pos != before.pos || ts.lexer.row != before.row || ts.lexer.col != before.col ||
		ts.lexer.includedRangeIdx != before.includedRangeIdx || ts.lexer.failTokenStartPos != before.failTokenStartPos ||
		ts.lexer.failTokenStartRow != before.failTokenStartRow || ts.lexer.failTokenStartCol != before.failTokenStartCol ||
		ts.lexer.failTokenStartRangeIdx != before.failTokenStartRangeIdx {
		t.Fatalf("reset-mismatch restore mutated lexer: got=%+v want=%+v", *ts.lexer, before)
	}
	var header diagnosticParserCoreHeader
	if err := header.publishVersionLexerSnapshot(compact, lang, snapshot); err == nil {
		t.Fatal("publication accepted a snapshot from an older reset generation")
	}
}

func TestDiagnosticParserCoreVersionLexerSnapshotPublicationPreservesRecoveryRegion(t *testing.T) {
	compact, snapshot := newDiagnosticParserCoreVersionLexerTestSnapshot(t)
	firstRegion := &diagnosticParserCoreS3Region{state: 7, startByte: 2, endByte: 5, children: []core.SubtreeID{11, 12}}
	secondRegion := &diagnosticParserCoreS3Region{state: 9, startByte: 5, endByte: 8, children: []core.SubtreeID{21, 22}}
	var header diagnosticParserCoreHeader
	header.openRecoveryRegion(firstRegion)
	if err := header.publishVersionLexerSnapshot(compact, snapshot.language, snapshot); err != nil {
		t.Fatalf("publish lexer snapshot: %v", err)
	}
	publishedState := header.versionState
	publishedSnapshot := header.versionLexerSnapshot()
	if publishedState == nil || publishedSnapshot == nil || publishedSnapshot == snapshot {
		t.Fatal("snapshot publication did not create an immutable wrapper copy")
	}
	publishedFirstRegion := header.recoveryRegion()
	if publishedFirstRegion == firstRegion || publishedFirstRegion.state != 7 || publishedFirstRegion.startByte != 2 ||
		publishedFirstRegion.endByte != 5 || len(publishedFirstRegion.children) != 2 ||
		publishedFirstRegion.children[0] != 11 || publishedFirstRegion.children[1] != 12 {
		t.Fatalf("published recovery region=%+v, want an owned copy", publishedFirstRegion)
	}
	firstRegion.state = 70
	firstRegion.startByte = 20
	firstRegion.children[0] = 110
	if publishedFirstRegion.state != 7 || publishedFirstRegion.startByte != 2 || publishedFirstRegion.children[0] != 11 {
		t.Fatal("published recovery region aliases the caller's region or children")
	}

	header.setRecoveryRegion(secondRegion)
	publishedSecondRegion := header.recoveryRegion()
	if publishedSecondRegion == secondRegion || publishedSecondRegion.state != 9 || publishedSecondRegion.startByte != 5 ||
		publishedSecondRegion.endByte != 8 || len(publishedSecondRegion.children) != 2 ||
		publishedSecondRegion.children[0] != 21 || publishedSecondRegion.children[1] != 22 ||
		header.versionLexerSnapshot() != publishedSnapshot {
		t.Fatal("region update did not preserve the published lexer snapshot")
	}
	secondRegion.endByte = 80
	secondRegion.children[1] = 220
	if publishedSecondRegion.endByte != 8 || publishedSecondRegion.children[1] != 22 {
		t.Fatal("updated recovery region aliases the caller's region or children")
	}
	copyOfHeader := header
	header.closeRecoveryRegion()
	if header.recoveryRegion() != nil || header.versionLexerSnapshot() != publishedSnapshot {
		t.Fatal("region closure dropped the lexer snapshot")
	}
	if copyOfHeader.recoveryRegion() != publishedSecondRegion || copyOfHeader.versionLexerSnapshot() != publishedSnapshot {
		t.Fatal("header copy changed after region closure")
	}
	header.clearVersionLexerSnapshot()
	if header.versionState != nil {
		t.Fatal("clearing the only version state left a wrapper")
	}
}

func TestDiagnosticParserCoreVersionLexerSnapshotRollbackRestoresImmutablePointer(t *testing.T) {
	compact, snapshot := newDiagnosticParserCoreVersionLexerTestSnapshot(t)
	first := &diagnosticParserCoreS3Region{state: 1}
	second := &diagnosticParserCoreS3Region{state: 2}
	headers := []diagnosticParserCoreHeader{{}}
	headers[0].openRecoveryRegion(first)
	if err := headers[0].publishVersionLexerSnapshot(compact, snapshot.language, snapshot); err != nil {
		t.Fatalf("publish lexer snapshot: %v", err)
	}
	originalState := headers[0].versionState
	originalSnapshot := headers[0].versionLexerSnapshot()
	var scratch diagnosticParserCoreHeaderRollbackScratch
	if err := scratch.begin(headers); err != nil {
		t.Fatalf("begin rollback: %v", err)
	}
	ownedFirst := headers[0].recoveryRegion()
	if ownedFirst == nil || ownedFirst == first || ownedFirst.state != first.state {
		t.Fatalf("published recovery region=%+v, want an owned copy of %+v", ownedFirst, first)
	}
	headers[0].setRecoveryRegion(second)
	headers[0].clearVersionLexerSnapshot()
	scratch.finish(&headers, true)
	restoredRegion := headers[0].recoveryRegion()
	if headers[0].versionState != originalState || headers[0].versionLexerSnapshot() != originalSnapshot || restoredRegion != ownedFirst || restoredRegion.state != first.state {
		t.Fatal("rollback did not restore the complete immutable version state")
	}
}

func TestDiagnosticParserCoreVersionLexerSnapshotRestoreRestoresScannerAndGuards(t *testing.T) {
	compact, snapshot := newDiagnosticParserCoreVersionLexerTestSnapshot(t)
	lang := snapshot.language
	payload := lang.ExternalScanner.Create()
	ts := &dfaTokenSource{
		lexer:              &Lexer{source: []byte("abc")},
		language:           lang,
		hasExternalScanner: true,
		externalPayload:    payload,
	}
	if err := snapshot.restore(compact, ts); err != nil {
		t.Fatalf("restore snapshot: %v", err)
	}
	if got := *payload.(*byte); got != 0x11 {
		t.Fatalf("scanner payload=%d, want 17", got)
	}
	if ts.lexer.pos != 17 || ts.lexer.row != 3 || ts.lexer.col != 9 || ts.lexer.includedRangeIdx != 2 {
		t.Fatalf("lexer cursor=%d/%d/%d range=%d, want 17/3/9/2", ts.lexer.pos, ts.lexer.row, ts.lexer.col, ts.lexer.includedRangeIdx)
	}
	if !bytes.Equal(ts.externalTokenStart, []byte{0x31, 0x32}) || !bytes.Equal(ts.externalTokenEnd, []byte{0x41, 0x42}) ||
		ts.extZeroPos != 17 || ts.extZeroState != 23 || len(ts.extZeroTried) != 3 || !ts.extZeroTried[0] ||
		ts.zeroWidthPos != 17 || ts.zeroWidthCount != 2 {
		t.Fatalf("scanner bookkeeping was not restored: %+v", ts)
	}
}

func TestDiagnosticParserCoreVersionLexerSnapshotRequiresOwnedCheckpointAccess(t *testing.T) {
	compact, before, after := newDiagnosticParserCoreVersionLexerTestCore(t)
	dfa := diagnosticParserCoreVersionLexerTestDFA()
	dfa.externalScannerPresent = true
	language := &Language{Name: "snapshot-owner-test", ExternalScanner: byteStateExternalScanner{}}
	if _, err := newDiagnosticParserCoreVersionLexerSnapshot(compact, language, core.SchedulerTransactionToken{}, dfa, before, after); err == nil {
		t.Fatal("snapshot constructor accepted an unauthenticated owner")
	}
	var stale core.SchedulerTransactionToken
	if err := compact.ApplySchedulerAtomic(func(owner core.SchedulerTransactionToken) error {
		stale = owner
		return nil
	}); err != nil {
		t.Fatalf("finish owner session: %v", err)
	}
	if _, err := newDiagnosticParserCoreVersionLexerSnapshot(compact, language, stale, dfa, before, after); err == nil {
		t.Fatal("snapshot constructor accepted a stale owner")
	}
	if err := compact.ApplySchedulerAtomic(func(owner core.SchedulerTransactionToken) error {
		if _, err := newDiagnosticParserCoreVersionLexerSnapshot(compact, language, owner, dfa, core.CheckpointID(99), after); err == nil {
			t.Fatal("snapshot constructor accepted a missing checkpoint")
		}
		return nil
	}); err != nil {
		t.Fatalf("finish missing-checkpoint owner session: %v", err)
	}
}

func TestDiagnosticParserCoreVersionLexerSnapshotFootprintDeduplicatesHeaderCopies(t *testing.T) {
	compact, snapshot := newDiagnosticParserCoreVersionLexerTestSnapshot(t)
	region := &diagnosticParserCoreS3Region{children: make([]core.SubtreeID, 1, 3)}
	state := &diagnosticParserCoreVersionState{s3Region: region, relexSnapshot: snapshot}
	baseScheduler := diagnosticParserCoreGenericScheduler{compact: compact}
	base := diagnosticParserCoreSchedulerFootprintBytes(&baseScheduler)
	headers := make([]diagnosticParserCoreHeader, 2, 2)
	headers[0].versionState = state
	headers[1] = headers[0]
	scheduler := diagnosticParserCoreGenericScheduler{compact: compact, headers: headers}
	delta := diagnosticParserCoreSchedulerFootprintBytes(&scheduler) - base
	want := uint64(2)*uint64(unsafe.Sizeof(diagnosticParserCoreHeader{})) +
		diagnosticParserCoreVersionStateFootprintBytes(state) +
		uint64(cap(scheduler.footprintRefs))*uint64(unsafe.Sizeof(diagnosticParserCoreFootprintRef{}))
	if delta != want {
		t.Fatalf("shared version-state footprint delta=%d, want %d", delta, want)
	}
}

func TestDiagnosticParserCoreVersionLexerSnapshotResetClearsRetainedHeaderBuffers(t *testing.T) {
	_, snapshot := newDiagnosticParserCoreVersionLexerTestSnapshot(t)
	state := &diagnosticParserCoreVersionState{relexSnapshot: snapshot}
	active := make([]diagnosticParserCoreHeader, 1, 2)
	active[0].versionState = state
	rollback := make([]diagnosticParserCoreHeader, 1, 2)
	rollback[0].versionState = state
	canonical := make([]diagnosticParserCoreHeader, 1, 2)
	canonical[0].versionState = state
	conflictOutputs := make([]diagnosticParserCoreHeader, 1, 2)
	conflictOutputs[0].versionState = state
	conflictAssembly := make([]diagnosticParserCoreHeader, 1, 2)
	conflictAssembly[0].versionState = state
	reductions := make([]diagnosticParserCoreHeader, 1, 2)
	reductions[0].versionState = state
	scheduler := diagnosticParserCoreGenericScheduler{
		headers: active,
		canonicalScratch: diagnosticParserCoreCanonicalScratch{
			headerBuffers: [2][]diagnosticParserCoreHeader{canonical, canonical},
		},
		conflictScratch: diagnosticParserCoreConflictScratch{
			outputs:        conflictOutputs,
			headerAssembly: conflictAssembly,
		},
		reductionReplacements: reductions,
	}
	scheduler.headerRollbackScratch.headers = rollback
	scheduler.headerRollbackScratch.inline[0].versionState = state
	scheduler.canonicalScratch.inlineHeaders[0][0].versionState = state
	before := diagnosticParserCoreSchedulerFootprintBytes(&scheduler)
	if err := resetDiagnosticParserCoreGenericScheduler(&scheduler); err != nil {
		t.Fatalf("reset scheduler: %v", err)
	}
	for name, headers := range map[string][]diagnosticParserCoreHeader{
		"active": active, "rollback": rollback, "canonical": canonical,
		"conflict outputs": conflictOutputs, "conflict assembly": conflictAssembly,
		"reductions": reductions,
	} {
		for index, header := range headers[:cap(headers)] {
			if header.versionState != nil {
				t.Fatalf("%s header slot %d retained version state", name, index)
			}
		}
	}
	if scheduler.headerRollbackScratch.inline[0].versionState != nil || scheduler.canonicalScratch.inlineHeaders[0][0].versionState != nil {
		t.Fatal("inline header scratch retained version state after reset")
	}
	if after := diagnosticParserCoreSchedulerFootprintBytes(&scheduler); after >= before {
		t.Fatalf("reset footprint=%d, want below pre-reset %d", after, before)
	}
}

func TestDiagnosticParserCoreVersionLexerSnapshotRetirementClearsLoser(t *testing.T) {
	compact, err := core.New(&genericConflictTable{
		actions: []core.Action{{Type: core.ActionShift, State: 2}},
	}, core.Limits{})
	if err != nil {
		t.Fatal(err)
	}
	survivor, err := compact.Seed(1, 0)
	if err != nil {
		t.Fatal(err)
	}
	survivor, err = compact.Shift(survivor, 9, 0, core.Token{
		Symbol: 9, StartByte: 0, EndByte: 1,
	}, core.ForkOrder{})
	if err != nil {
		t.Fatalf("shift survivor: %v", err)
	}
	loser, err := compact.Seed(2, 0)
	if err != nil {
		t.Fatal(err)
	}
	_, snapshot := newDiagnosticParserCoreVersionLexerTestSnapshot(t)
	state := &diagnosticParserCoreVersionState{relexSnapshot: snapshot}
	headers := make([]diagnosticParserCoreHeader, 2, 2)
	headers[0] = diagnosticParserCoreHeader{head: survivor, checkpoint: 0, creationSeq: 1, shifted: true, versionState: state}
	headers[1] = diagnosticParserCoreHeader{head: loser, checkpoint: 0, creationSeq: 2, versionState: state}
	headers[0].markRecoveryLineage()
	headers[1].markRecoveryLineage()
	survivorState := headers[0].versionState
	scheduler := diagnosticParserCoreGenericScheduler{
		compact:           compact,
		headers:           headers,
		checkpointID:      0,
		recoveryIsolation: true,
		epochProgress:     true,
		token:             Token{Symbol: 9, StartByte: 0, EndByte: 1},
		options: DiagnosticParserCorePrefixOptions{
			allowCompactRecoveryLineageSelection:          true,
			allowCompactRecoveryTrailingLineageRetirement: true,
		},
	}
	retired, err := scheduler.retireTrailingRecoveryNoActionLineage([]int{1})
	if err != nil || !retired {
		t.Fatalf("retirement=%t err=%v, want true/nil", retired, err)
	}
	if len(scheduler.headers) != 1 || scheduler.headers[0].versionState == survivorState ||
		scheduler.headers[0].versionLexerSnapshot() != snapshot || scheduler.headers[0].isRecoveryLineage() {
		t.Fatalf("survivor state did not clear lineage while retaining lexer ownership: headers=%+v", scheduler.headers)
	}
	if baseline, set := scheduler.headers[0].recoveryNodeBaseline(); set || baseline != 0 {
		t.Fatalf("retired survivor baseline=%d/%t, want cleared 0/false", baseline, set)
	}
	if headers[1].versionState != nil {
		t.Fatal("retired loser retained its immutable version state")
	}
}

func TestDiagnosticParserCoreVersionLexerSnapshotRejectsCrossCoreDestination(t *testing.T) {
	compact, snapshot := newDiagnosticParserCoreVersionLexerTestSnapshot(t)
	other, err := core.New(&genericConflictTable{}, core.Limits{})
	if err != nil {
		t.Fatal(err)
	}
	target := &dfaTokenSource{
		lexer:              &Lexer{source: []byte("abc"), pos: 4, row: 5, col: 6},
		language:           snapshot.language,
		hasExternalScanner: true,
		externalPayload:    snapshot.language.ExternalScanner.Create(),
	}
	before := *target.lexer
	if err := snapshot.restore(other, target); err == nil {
		t.Fatal("restore accepted a snapshot for a different Core")
	}
	if !reflect.DeepEqual(*target.lexer, before) {
		t.Fatalf("cross-Core restore mutated the target lexer: got=%+v want=%+v", *target.lexer, before)
	}
	var header diagnosticParserCoreHeader
	if err := header.publishVersionLexerSnapshot(other, snapshot.language, snapshot); err == nil {
		t.Fatal("publication accepted a snapshot for a different Core")
	}
	if header.versionState != nil {
		t.Fatal("failed cross-Core publication installed version state")
	}
	if compact == other {
		t.Fatal("test Cores unexpectedly alias")
	}
}

func TestDiagnosticParserCoreVersionLexerSnapshotRejectsCrossLanguageDestination(t *testing.T) {
	compact, snapshot := newDiagnosticParserCoreVersionLexerTestSnapshot(t)
	otherLanguage := &Language{Name: "other-snapshot-test", ExternalScanner: byteStateExternalScanner{}}
	target := &dfaTokenSource{
		lexer:              &Lexer{source: []byte("abc"), pos: 4, row: 5, col: 6},
		language:           otherLanguage,
		hasExternalScanner: true,
		externalPayload:    otherLanguage.ExternalScanner.Create(),
	}
	before := *target.lexer
	if err := snapshot.restore(compact, target); err == nil {
		t.Fatal("restore accepted a snapshot for a different Language")
	}
	if !reflect.DeepEqual(*target.lexer, before) {
		t.Fatalf("cross-Language restore mutated the target lexer: got=%+v want=%+v", *target.lexer, before)
	}
	var header diagnosticParserCoreHeader
	if err := header.publishVersionLexerSnapshot(compact, otherLanguage, snapshot); err == nil {
		t.Fatal("publication accepted a snapshot for a different Language")
	}
	if header.versionState != nil {
		t.Fatal("failed cross-Language publication installed version state")
	}
}

func TestDiagnosticParserCoreVersionLexerSnapshotRejectsScannerReplacement(t *testing.T) {
	compact, snapshot := newDiagnosticParserCoreVersionLexerTestSnapshot(t)
	language := snapshot.language
	originalScanner := language.ExternalScanner
	language.ExternalScanner = checkpointByteExternalScanner{}
	defer func() { language.ExternalScanner = originalScanner }()
	target := &dfaTokenSource{
		lexer:              &Lexer{source: []byte("abc"), pos: 4, row: 5, col: 6},
		language:           language,
		hasExternalScanner: true,
		externalPayload:    language.ExternalScanner.Create(),
	}
	before := *target.lexer
	if err := snapshot.restore(compact, target); err == nil {
		t.Fatal("restore accepted a replacement scanner contract")
	}
	if !reflect.DeepEqual(*target.lexer, before) {
		t.Fatalf("scanner-mismatch restore mutated the target lexer: got=%+v want=%+v", *target.lexer, before)
	}
	var header diagnosticParserCoreHeader
	if err := header.publishVersionLexerSnapshot(compact, language, snapshot); err == nil {
		t.Fatal("publication accepted a replacement scanner contract")
	}
}

func TestDiagnosticParserCoreVersionLexerSnapshotBindsCheckpointIdentity(t *testing.T) {
	compact, before, after := newDiagnosticParserCoreVersionLexerTestCore(t)
	scanner := newC26lCheckpointScanner()
	language := &Language{Name: "checkpoint-identity-snapshot-test", ExternalScanner: scanner}
	dfa := diagnosticParserCoreVersionLexerTestDFA()
	dfa.externalScannerPresent = true
	dfa.externalPayload = []byte{1, 2, 3}
	var snapshot *diagnosticParserCoreVersionLexerSnapshot
	if err := compact.ApplySchedulerAtomic(func(owner core.SchedulerTransactionToken) error {
		var err error
		snapshot, err = newDiagnosticParserCoreVersionLexerSnapshot(compact, language, owner, dfa, before, after)
		return err
	}); err != nil {
		t.Fatalf("construct identity-bound snapshot: %v", err)
	}
	if snapshot == nil {
		t.Fatal("constructed checkpoint identity snapshot is nil")
	}
	if !snapshot.checkpointIdentityValid || snapshot.checkpointIdentity == ([32]byte{}) {
		t.Fatalf("snapshot identity=%x/%t, want a non-zero authenticated identity", snapshot.checkpointIdentity, snapshot.checkpointIdentityValid)
	}
	if !snapshot.checkpointIdentityRequired {
		t.Fatal("snapshot did not record that its scanner requires checkpoint identity")
	}
	clone := snapshot.clone()
	if clone == nil || clone.checkpointIdentity != snapshot.checkpointIdentity ||
		!clone.checkpointIdentityRequired || !clone.checkpointIdentityValid {
		t.Fatal("snapshot clone lost checkpoint identity")
	}

	target := &dfaTokenSource{
		lexer:              &Lexer{source: []byte("abc"), pos: 4, row: 5, col: 6},
		language:           language,
		hasExternalScanner: true,
		externalPayload:    scanner.Create(),
	}
	beforeTarget := target.snapshotRelexState()
	scanner.grammarID = []byte("checkpoint-identity-drift")
	if err := snapshot.restore(compact, target); err == nil {
		t.Fatal("restore accepted a changed checkpoint identity")
	}
	if got := target.snapshotRelexState(); !got.equal(beforeTarget) {
		t.Fatalf("identity-mismatch restore mutated target state: got=%+v want=%+v", got, beforeTarget)
	}
	var header diagnosticParserCoreHeader
	if err := header.publishVersionLexerSnapshot(compact, language, snapshot); err == nil {
		t.Fatal("publication accepted a changed checkpoint identity")
	}
	if header.versionState != nil {
		t.Fatal("failed identity-drift publication installed version state")
	}

	scanner.grammarID = []byte("grammar-c26l")
	if err := snapshot.restore(compact, target); err != nil {
		t.Fatalf("restore rejected the original checkpoint identity: %v", err)
	}

	scheduler := &diagnosticParserCoreGenericScheduler{
		compact:     compact,
		tokenSource: &dfaTokenSource{language: language},
		headers: []diagnosticParserCoreHeader{{
			versionState: &diagnosticParserCoreVersionState{relexSnapshot: snapshot},
		}},
	}
	if got := scheduler.equivalentVersionLexerSnapshot(snapshot.dfa, before, after); got != snapshot {
		t.Fatalf("equivalent snapshot lookup = %p, want %p", got, snapshot)
	}
	scanner.grammarID = []byte("checkpoint-identity-drift-again")
	if got := scheduler.equivalentVersionLexerSnapshot(snapshot.dfa, before, after); got != nil {
		t.Fatalf("equivalent snapshot lookup reused identity-drifted snapshot %p", got)
	}
}

func TestDiagnosticParserCoreVersionLexerSnapshotRejectsIncompleteCheckpointIdentity(t *testing.T) {
	compact, before, after := newDiagnosticParserCoreVersionLexerTestCore(t)
	scanner := newC26lCheckpointScanner()
	scanner.identityOK = false
	language := &Language{Name: "incomplete-checkpoint-identity-snapshot-test", ExternalScanner: scanner}
	dfa := diagnosticParserCoreVersionLexerTestDFA()
	dfa.externalScannerPresent = true
	dfa.externalPayload = []byte{1, 2, 3}
	err := compact.ApplySchedulerAtomic(func(owner core.SchedulerTransactionToken) error {
		_, err := newDiagnosticParserCoreVersionLexerSnapshot(compact, language, owner, dfa, before, after)
		return err
	})
	if err == nil || !strings.Contains(err.Error(), "requires a complete checkpoint identity") {
		t.Fatalf("incomplete checkpoint identity error=%v, want the identity rejection", err)
	}
}

func TestDiagnosticParserCoreVersionLexerSnapshotRejectsUnrepresentableScannerState(t *testing.T) {
	compact, before, after := newDiagnosticParserCoreVersionLexerTestCore(t)
	dfa := diagnosticParserCoreVersionLexerTestDFA()
	dfa.externalPayload = nil
	dfa.externalScannerPresent = true
	language := &Language{Name: "unrepresentable-snapshot-test", ExternalScanner: byteStateExternalScanner{}}
	err := compact.ApplySchedulerAtomic(func(owner core.SchedulerTransactionToken) error {
		_, err := newDiagnosticParserCoreVersionLexerSnapshot(compact, language, owner, dfa, before, after)
		return err
	})
	if err == nil {
		t.Fatal("constructor accepted an unrepresentable external scanner state")
	}
}

func TestDiagnosticParserCoreVersionLexerSnapshotAcceptsSameContractIDZero(t *testing.T) {
	compact, err := core.New(&genericConflictTable{}, core.Limits{})
	if err != nil {
		t.Fatal(err)
	}
	language := &Language{Name: "empty-contract-snapshot-test"}
	dfa := diagnosticParserCoreVersionLexerTestDFA()
	dfa.externalPayload = nil
	var snapshot *diagnosticParserCoreVersionLexerSnapshot
	err = compact.ApplySchedulerAtomic(func(owner core.SchedulerTransactionToken) error {
		var err error
		snapshot, err = newDiagnosticParserCoreVersionLexerSnapshot(compact, language, owner, dfa, 0, 0)
		return err
	})
	if err != nil {
		t.Fatalf("construct ID-zero snapshot: %v", err)
	}
	target := &dfaTokenSource{lexer: &Lexer{source: []byte("abc")}, language: language}
	if err := snapshot.restore(compact, target); err != nil {
		t.Fatalf("same-contract ID-zero restore failed: %v", err)
	}
	var header diagnosticParserCoreHeader
	if err := header.publishVersionLexerSnapshot(compact, language, snapshot); err != nil {
		t.Fatalf("same-contract ID-zero publication failed: %v", err)
	}
}
