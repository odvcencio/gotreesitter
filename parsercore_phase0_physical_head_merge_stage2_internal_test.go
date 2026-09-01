//go:build gts_parsercorephase0

package gotreesitter

import (
	"testing"

	core "github.com/odvcencio/gotreesitter/internal/parsercorephase0"
)

func newStage2RecoveryHeadPair(t *testing.T) (*core.Core, core.Head, core.Head, core.SubtreeID, core.SubtreeID) {
	t.Helper()
	compact, err := core.New(&genericConflictTable{}, core.Limits{MaxDerivations: 8, MaxPopPaths: 8})
	if err != nil {
		t.Fatal(err)
	}
	leftSeed, err := compact.Seed(1, 0)
	if err != nil {
		t.Fatal(err)
	}
	leftChild, err := compact.ErrorRegionLeaf(3, 0, 1, false)
	if err != nil {
		t.Fatal(err)
	}
	left, err := compact.ErrorRegionResume(leftSeed, 2, 0, 1, []core.SubtreeID{leftChild})
	if err != nil {
		t.Fatal(err)
	}
	if err := compact.BeginFrontier(); err != nil {
		t.Fatal(err)
	}
	rightSeed, err := compact.Seed(1, 0)
	if err != nil {
		t.Fatal(err)
	}
	rightChild, err := compact.ErrorRegionLeaf(4, 0, 1, false)
	if err != nil {
		t.Fatal(err)
	}
	right, err := compact.ErrorRegionResume(rightSeed, 2, 0, 1, []core.SubtreeID{rightChild})
	if err != nil {
		t.Fatal(err)
	}
	if left == right {
		t.Fatal("frontier generations reused one physical recovery head")
	}
	return compact, left, right, leftChild, rightChild
}

func TestStage2PhysicalHeadMergePreservesDistinctErrorRegions(t *testing.T) {
	compact, left, right, leftChild, rightChild := newStage2RecoveryHeadPair(t)
	leftState := &diagnosticParserCoreVersionState{
		s3Region: &diagnosticParserCoreS3Region{
			state: 2, startByte: 0, endByte: 1, children: []core.SubtreeID{leftChild},
		},
	}
	rightState := &diagnosticParserCoreVersionState{
		s3Region: &diagnosticParserCoreS3Region{
			state: 2, startByte: 0, endByte: 1, children: []core.SubtreeID{rightChild},
		},
	}
	headers := []diagnosticParserCoreHeader{
		{head: left, shifted: true, versionState: leftState, recoveryFlags: diagnosticParserCoreRecoveryCompetitorFlag},
		{head: right, shifted: true, versionState: rightState, recoveryFlags: diagnosticParserCoreRecoveryCompetitorFlag},
	}
	var scratch diagnosticParserCoreCanonicalScratch
	out, err := scratch.canonicalizeRecovery(compact, headers)
	if err != nil {
		t.Fatal(err)
	}
	if len(out) != 2 || out[0].head == out[1].head {
		t.Fatalf("distinct error regions were physically merged: %+v", out)
	}
	if out[0].recoveryRegion() == nil || out[1].recoveryRegion() == nil ||
		out[0].recoveryRegion().children[0] != leftChild || out[1].recoveryRegion().children[0] != rightChild {
		t.Fatalf("error-region children changed during canonicalization: %+v", out)
	}
}

func TestStage2PhysicalHeadMergePreservesRecoveryLineage(t *testing.T) {
	compact, left, right, _, _ := newStage2RecoveryHeadPair(t)
	headers := []diagnosticParserCoreHeader{
		{head: left, shifted: true, recoveryFlags: diagnosticParserCoreRecoveryCompetitorFlag},
		{head: right, shifted: true, recoveryFlags: diagnosticParserCoreRecoveryCompetitorFlag},
	}
	var scratch diagnosticParserCoreCanonicalScratch
	out, err := scratch.canonicalizeRecovery(compact, headers)
	if err != nil {
		t.Fatal(err)
	}
	if len(out) != 2 || out[0].head == out[1].head {
		t.Fatalf("recovery lineage heads were physically merged: %+v", out)
	}
	for index, header := range out {
		if !header.isRecoveryLineage() {
			t.Fatalf("header %d lost recovery lineage marker: %+v", index, out)
		}
	}
}

func TestStage2PhysicalHeadMergePreservesDistinctLexerSnapshots(t *testing.T) {
	compact, left, right, _, _ := newStage2RecoveryHeadPair(t)
	leftSnapshot := &diagnosticParserCoreVersionLexerSnapshot{}
	rightSnapshot := &diagnosticParserCoreVersionLexerSnapshot{}
	headers := []diagnosticParserCoreHeader{
		{head: left, shifted: true, versionState: &diagnosticParserCoreVersionState{relexSnapshot: leftSnapshot}},
		{head: right, shifted: true, versionState: &diagnosticParserCoreVersionState{relexSnapshot: rightSnapshot}},
	}
	before := compact.Work()
	var scratch diagnosticParserCoreCanonicalScratch
	out, err := scratch.canonicalize(compact, headers)
	if err != nil {
		t.Fatal(err)
	}
	if len(out) != 2 {
		t.Fatalf("distinct lexer snapshots were merged: %+v", out)
	}
	seenLeft, seenRight := false, false
	for _, header := range out {
		seenLeft = seenLeft || header.versionLexerSnapshot() == leftSnapshot
		seenRight = seenRight || header.versionLexerSnapshot() == rightSnapshot
	}
	if !seenLeft || !seenRight {
		t.Fatalf("lexer snapshot identity was lost: %+v", out)
	}
	if after := compact.Work(); after != before {
		t.Fatalf("lexer snapshot guard entered physical merge path: before=%+v after=%+v", before, after)
	}
}

func TestStage2PhysicalHeadMergeCanonicalizationUnionsLineage(t *testing.T) {
	table := &genericConflictTable{cells: map[genericConflictCell][]core.Action{
		{state: 1, symbol: 3}: {{Type: core.ActionShift, State: 2}},
		{state: 1, symbol: 4}: {{Type: core.ActionShift, State: 2}},
	}}
	compact, err := core.New(table, core.Limits{MaxDerivations: 8, MaxPopPaths: 8})
	if err != nil {
		t.Fatal(err)
	}
	seed, err := compact.Seed(1, 0)
	if err != nil {
		t.Fatal(err)
	}
	left, err := compact.Shift(seed, 3, 0, core.Token{Symbol: 3, EndByte: 1}, core.ForkOrder{})
	if err != nil {
		t.Fatal(err)
	}
	if err := compact.BeginFrontier(); err != nil {
		t.Fatal(err)
	}
	seed, err = compact.Seed(1, 0)
	if err != nil {
		t.Fatal(err)
	}
	right, err := compact.Shift(seed, 4, 0, core.Token{Symbol: 4, EndByte: 1}, core.ForkOrder{})
	if err != nil {
		t.Fatal(err)
	}
	source, err := compact.Seed(9, 0)
	if err != nil {
		t.Fatal(err)
	}
	leftSet := core.NewAlternativeSetMember(11, 1)
	rightSet := core.NewAlternativeSetMember(12, 2)
	scheduler := diagnosticParserCoreGenericScheduler{compact: compact, headers: []diagnosticParserCoreHeader{
		{head: source, creationSeq: 1},
		{head: left, creationSeq: 3, altSet: leftSet, cleanPathRank: core.CleanPathRankSelected, cleanPathLineage: 11},
		{head: right, creationSeq: 5, altSet: rightSet, cleanPathRank: core.CleanPathRankUnselected, cleanPathLineage: 12},
	}}
	err = compact.ApplySchedulerAtomic(func(owner core.SchedulerTransactionToken) error {
		if err := scheduler.reindexCondenseCandidatesOwned(owner, 0); err != nil {
			return err
		}
		if err := scheduler.canonicalizeOwned(owner); err != nil {
			return err
		}
		return scheduler.persistHeaderLineageOwned(owner)
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(scheduler.headers) != 2 {
		t.Fatalf("physical heads did not canonicalize: %+v", scheduler.headers)
	}
	merged := scheduler.headers[1]
	members, ok := compact.AlternativeSetMembers(merged.altSet)
	if !ok || len(members) != 2 ||
		core.AlternativeSetMemberEvent(members[0]) != 11 ||
		core.AlternativeSetMemberEvent(members[1]) != 12 {
		t.Fatalf("physical-head lineage union=%v/%t", members, ok)
	}
	if merged.cleanPathRank != core.CleanPathRankUnknown || merged.cleanPathLineage != 0 {
		t.Fatalf("physical-head scalar lineage did not fail closed: %+v", merged)
	}
	work := compact.Work()
	if work.PhysicalHeadMergeAttempts != 1 || work.PhysicalHeadMergeSuccesses != 1 || work.PhysicalHeadMergeInputLinks != 1 {
		t.Fatalf("physical-head scheduler telemetry=%+v", work)
	}
}

func TestStage2PhysicalHeadMergeRunsThroughGenericReductionDispatch(t *testing.T) {
	table := &genericConflictTable{
		cells: map[genericConflictCell][]core.Action{
			{state: 1, symbol: 3}: {{Type: core.ActionShift, State: 2}},
			{state: 1, symbol: 4}: {{Type: core.ActionShift, State: 2}},
			{state: 1, symbol: 5}: {{Type: core.ActionShift, State: 3}},
			{state: 3, symbol: 9}: {{Type: core.ActionReduce, Symbol: 6, ChildCount: 1}},
		},
		gotos: map[genericConflictCell]core.StateID{{state: 1, symbol: 6}: 7},
	}
	compact, err := core.New(table, core.Limits{MaxDerivations: 8, MaxPopPaths: 8})
	if err != nil {
		t.Fatal(err)
	}
	leftSeed, err := compact.Seed(1, 0)
	if err != nil {
		t.Fatal(err)
	}
	left, err := compact.Shift(leftSeed, 3, 0, core.Token{Symbol: 3, EndByte: 1}, core.ForkOrder{})
	if err != nil {
		t.Fatal(err)
	}
	if err := compact.BeginFrontier(); err != nil {
		t.Fatal(err)
	}
	rightSeed, err := compact.Seed(1, 0)
	if err != nil {
		t.Fatal(err)
	}
	right, err := compact.Shift(rightSeed, 4, 0, core.Token{Symbol: 4, EndByte: 1}, core.ForkOrder{})
	if err != nil {
		t.Fatal(err)
	}
	sourceSeed, err := compact.Seed(1, 0)
	if err != nil {
		t.Fatal(err)
	}
	source, err := compact.Shift(sourceSeed, 5, 0, core.Token{Symbol: 5, EndByte: 1}, core.ForkOrder{})
	if err != nil {
		t.Fatal(err)
	}
	if left == right {
		t.Fatal("fixture did not create two physical sibling heads")
	}

	scheduler := &diagnosticParserCoreGenericScheduler{
		compact: compact,
		headers: []diagnosticParserCoreHeader{
			{head: source, creationSeq: 1},
			{head: left, creationSeq: 2},
			{head: right, creationSeq: 3},
		},
		token: Token{Symbol: 9, StartByte: 1, EndByte: 2},
		options: DiagnosticParserCorePrefixOptions{
			MaxDispatches: 20,
			ReceiptMode:   DiagnosticParserCoreReceiptSummary,
		},
		receipt: &DiagnosticParserCoreGenericScheduler{},
	}
	before, err := diagnosticParserCoreHeaderReceipts(compact, scheduler.headers)
	if err != nil {
		t.Fatal(err)
	}
	cell := mustDiagnosticParserCoreGenericCell(t, compact, 0, scheduler.headers[0], 9)
	workBefore := compact.Work()
	if err := scheduler.applyGenericReduction(before, cell); err != nil {
		t.Fatal(err)
	}
	workAfter := compact.Work()
	if workAfter.PhysicalHeadMergeAttempts-workBefore.PhysicalHeadMergeAttempts != 1 ||
		workAfter.PhysicalHeadMergeSuccesses-workBefore.PhysicalHeadMergeSuccesses != 1 ||
		workAfter.PhysicalHeadMergeInputLinks-workBefore.PhysicalHeadMergeInputLinks != 1 {
		t.Fatalf("generic reduction physical-merge telemetry before=%+v after=%+v", workBefore, workAfter)
	}
	canonical, ok := compact.CanonicalBoundary(2, 1, false, 0)
	if !ok {
		t.Fatal("generic reduction did not publish the merged sibling boundary")
	}
	derivations, err := compact.Derivations(canonical)
	if err != nil {
		t.Fatal(err)
	}
	if len(derivations) != 2 || len(derivations[0].Payloads) != 1 || len(derivations[1].Payloads) != 1 {
		t.Fatalf("generic reduction merged derivations=%+v", derivations)
	}
	first, err := compact.Subtree(derivations[0].Payloads[0])
	if err != nil {
		t.Fatal(err)
	}
	second, err := compact.Subtree(derivations[1].Payloads[0])
	if err != nil {
		t.Fatal(err)
	}
	if first.Symbol != 3 || second.Symbol != 4 {
		t.Fatalf("generic reduction changed stable sibling order: first=%+v second=%+v", first, second)
	}
}

func TestStage2PhysicalHeadMergeCandidatesPreserveDistinctLexerVersionsForClassification(t *testing.T) {
	compact, first, second := newDiagnosticParserCoreCanonicalTestCore(t)
	sharedSnapshot := &diagnosticParserCoreVersionLexerSnapshot{}
	sharedState := &diagnosticParserCoreVersionState{relexSnapshot: sharedSnapshot}
	distinctState := &diagnosticParserCoreVersionState{relexSnapshot: &diagnosticParserCoreVersionLexerSnapshot{}}
	scheduler := diagnosticParserCoreGenericScheduler{compact: compact, headers: []diagnosticParserCoreHeader{
		{head: first, versionState: sharedState},
		{head: second, versionState: sharedState},
		{head: second, versionState: distinctState},
		{head: second, versionState: sharedState, recoveryFlags: diagnosticParserCoreRecoveryCompetitorFlag},
		{head: second, versionState: sharedState, paused: true},
		{head: second, versionState: sharedState, accepted: true},
	}}
	candidates := scheduler.collectCondenseCandidates(0)
	if len(candidates) != 2 || candidates[0].Head != second || candidates[1].Head != second {
		t.Fatalf("classification candidates=%+v, want both clean lexer versions", candidates)
	}
}

func TestStage2PhysicalHeadMergeRejectsClearedRecoveryCostedLineage(t *testing.T) {
	compact, first, second := newDiagnosticParserCoreCanonicalTestCore(t)
	recovered := diagnosticParserCoreHeader{head: second}
	recovered.markRecoveryLineage()
	recovered.openRecoveryRegion(&diagnosticParserCoreS3Region{state: 2, startByte: 0, endByte: 1})
	recovered.closeRecoveryRegion()
	recovered.clearRecoveryLineage()
	recovered.clearVersionLexerSnapshot()
	if recovered.isRecoveryLineage() || !recovered.isRecoveryCosted() || recovered.versionState != nil {
		t.Fatalf("cleared recovery header lost its permanent cost marker: %+v", recovered)
	}

	scheduler := diagnosticParserCoreGenericScheduler{compact: compact, headers: []diagnosticParserCoreHeader{
		{head: first}, recovered,
	}}
	if candidates := scheduler.collectCondenseCandidates(0); len(candidates) != 0 {
		t.Fatalf("recovery-costed candidate entered clean physical merge: %+v", candidates)
	}
	scheduler.headers[0], scheduler.headers[1] = scheduler.headers[1], scheduler.headers[0]
	if candidates := scheduler.collectCondenseCandidates(0); len(candidates) != 0 {
		t.Fatalf("recovery-costed source entered clean physical merge: %+v", candidates)
	}
	if work := compact.Work(); work.PhysicalHeadMergeAttempts != 0 || work.PhysicalHeadMergeSuccesses != 0 {
		t.Fatalf("recovery-cost guard entered physical merge: %+v", work)
	}
}

func TestStage2PhysicalHeadMergeFailsClosedAcrossDistinctLexerOwnership(t *testing.T) {
	table := &genericConflictTable{cells: map[genericConflictCell][]core.Action{
		{state: 1, symbol: 3}: {{Type: core.ActionShift, State: 2}},
		{state: 1, symbol: 4}: {{Type: core.ActionShift, State: 2}},
	}}
	compact, err := core.New(table, core.Limits{MaxDerivations: 8, MaxPopPaths: 8})
	if err != nil {
		t.Fatal(err)
	}
	seed, err := compact.Seed(1, 0)
	if err != nil {
		t.Fatal(err)
	}
	left, err := compact.Shift(seed, 3, 0, core.Token{Symbol: 3, EndByte: 1}, core.ForkOrder{})
	if err != nil {
		t.Fatal(err)
	}
	if err := compact.BeginFrontier(); err != nil {
		t.Fatal(err)
	}
	seed, err = compact.Seed(1, 0)
	if err != nil {
		t.Fatal(err)
	}
	right, err := compact.Shift(seed, 4, 0, core.Token{Symbol: 4, EndByte: 1}, core.ForkOrder{})
	if err != nil {
		t.Fatal(err)
	}
	source, err := compact.Seed(9, 0)
	if err != nil {
		t.Fatal(err)
	}
	scheduler := diagnosticParserCoreGenericScheduler{compact: compact, headers: []diagnosticParserCoreHeader{
		{head: source},
		{head: left, versionState: &diagnosticParserCoreVersionState{relexSnapshot: &diagnosticParserCoreVersionLexerSnapshot{}}},
		{head: right, versionState: &diagnosticParserCoreVersionState{relexSnapshot: &diagnosticParserCoreVersionLexerSnapshot{}}},
	}}
	before := compact.Work()
	err = compact.ApplySchedulerAtomic(func(owner core.SchedulerTransactionToken) error {
		return scheduler.reindexCondenseCandidatesOwned(owner, 0)
	})
	if err == nil || err.Error() != "parser-core phase zero: physical heads have different lexer ownership" {
		t.Fatalf("distinct lexer ownership error=%v", err)
	}
	if compact.Work() != before {
		t.Fatalf("distinct lexer ownership entered physical merge: before=%+v after=%+v", before, compact.Work())
	}
}
