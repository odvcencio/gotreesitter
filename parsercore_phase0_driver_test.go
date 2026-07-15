//go:build gts_parsercorephase0

package gotreesitter_test

import (
	"crypto/sha256"
	"fmt"
	"reflect"
	"testing"

	gotreesitter "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
	"github.com/odvcencio/gotreesitter/internal/benchfixtures"
)

type fakeGoScanner struct{}

func (fakeGoScanner) Create() any                                        { return nil }
func (fakeGoScanner) Destroy(any)                                        {}
func (fakeGoScanner) Serialize(any, []byte) int                          { return 0 }
func (fakeGoScanner) Deserialize(any, []byte)                            {}
func (fakeGoScanner) Scan(any, *gotreesitter.ExternalLexer, []bool) bool { return false }

type wrappedGoScanner struct{ grammars.GoExternalScanner }

type parserCoreHeaderStep struct {
	state       gotreesitter.StateID
	byteOffset  uint32
	creationSeq uint64
	shifted     bool
}

type parserCoreRoundStep struct {
	before, after []parserCoreHeaderStep
	actions       []gotreesitter.DiagnosticParserCoreRoundAction
}

func assertParserCoreSameTokenRounds(t *testing.T, result gotreesitter.DiagnosticParserCorePrefixResult) {
	t.Helper()
	want := []parserCoreRoundStep{
		{
			before: []parserCoreHeaderStep{{state: 186, byteOffset: 579}},
			after: []parserCoreHeaderStep{
				{state: 285, byteOffset: 579},
				{state: 193, byteOffset: 580, creationSeq: 1, shifted: true},
			},
			actions: []gotreesitter.DiagnosticParserCoreRoundAction{
				{HeaderIndex: 0, State: 186, ByteOffset: 579, Ordinal: 1, Action: gotreesitter.ParseAction{Type: gotreesitter.ParseActionShift, State: 193}, BranchOrder: 1},
				{HeaderIndex: 0, State: 186, ByteOffset: 579, Ordinal: 0, Action: gotreesitter.ParseAction{Type: gotreesitter.ParseActionReduce, Symbol: 171, ChildCount: 1}},
			},
		},
		{
			before: []parserCoreHeaderStep{
				{state: 285, byteOffset: 579},
				{state: 193, byteOffset: 580, creationSeq: 1, shifted: true},
			},
			after: []parserCoreHeaderStep{
				{state: 77, byteOffset: 579},
				{state: 193, byteOffset: 580, creationSeq: 1, shifted: true},
			},
			actions: []gotreesitter.DiagnosticParserCoreRoundAction{{HeaderIndex: 0, State: 285, ByteOffset: 579, Ordinal: 0, Action: gotreesitter.ParseAction{Type: gotreesitter.ParseActionReduce, Symbol: 188, ChildCount: 2, ProductionID: 122}}},
		},
		{
			before: []parserCoreHeaderStep{
				{state: 77, byteOffset: 579},
				{state: 193, byteOffset: 580, creationSeq: 1, shifted: true},
			},
			after: []parserCoreHeaderStep{
				{state: 253, byteOffset: 579},
				{state: 193, byteOffset: 580, creationSeq: 1, shifted: true},
			},
			actions: []gotreesitter.DiagnosticParserCoreRoundAction{{HeaderIndex: 0, State: 77, ByteOffset: 579, Ordinal: 0, Action: gotreesitter.ParseAction{Type: gotreesitter.ParseActionReduce, Symbol: 171, ChildCount: 1}}},
		},
		{
			before: []parserCoreHeaderStep{
				{state: 253, byteOffset: 579},
				{state: 193, byteOffset: 580, creationSeq: 1, shifted: true},
			},
			after: []parserCoreHeaderStep{
				{state: 254, byteOffset: 579},
				{state: 193, byteOffset: 580, creationSeq: 1, shifted: true},
			},
			actions: []gotreesitter.DiagnosticParserCoreRoundAction{{HeaderIndex: 0, State: 253, ByteOffset: 579, Ordinal: 0, Action: gotreesitter.ParseAction{Type: gotreesitter.ParseActionReduce, Symbol: 119, ChildCount: 1}}},
		},
	}
	if len(result.SameTokenRounds) != len(want) {
		t.Fatalf("same-lookahead rounds=%d, want %d", len(result.SameTokenRounds), len(want))
	}
	checkpoint := result.Elections[len(result.Elections)-1].ScannerAfter.SHA256
	checkHeaders := func(roundIndex int, label string, got []gotreesitter.DiagnosticParserCoreHeaderReceipt, expected []parserCoreHeaderStep) {
		t.Helper()
		if len(got) != len(expected) {
			t.Fatalf("same-lookahead round %d %s headers=%d, want %d", roundIndex, label, len(got), len(expected))
		}
		for index, header := range got {
			wantHeader := expected[index]
			if header.State != wantHeader.state || header.ByteOffset != wantHeader.byteOffset || header.CreationSeq != wantHeader.creationSeq || header.Shifted != wantHeader.shifted || header.Accepted || header.ExactPaths != 1 || header.Checkpoint != checkpoint {
				t.Fatalf("same-lookahead round %d %s header %d=%+v, want %+v with checkpoint %x", roundIndex, label, index, header, wantHeader, checkpoint)
			}
		}
	}
	for index, round := range result.SameTokenRounds {
		if round.Index != index || !reflect.DeepEqual(round.Actions, want[index].actions) {
			t.Fatalf("same-lookahead round %d identity/actions=%+v, want index=%d actions=%+v", index, round, index, want[index].actions)
		}
		checkHeaders(index, "before", round.Before, want[index].before)
		checkHeaders(index, "after", round.After, want[index].after)
	}
}

func assertProductionParserCoreCells(t *testing.T, profile *gotreesitter.AmbiguityProfile) {
	t.Helper()
	type cellKey struct {
		state     gotreesitter.StateID
		lookahead gotreesitter.Symbol
	}
	want := map[gotreesitter.StateID][]gotreesitter.ParseAction{
		186: {
			{Type: gotreesitter.ParseActionReduce, Symbol: 171, ChildCount: 1},
			{Type: gotreesitter.ParseActionShift, State: 193},
		},
		285: {{Type: gotreesitter.ParseActionReduce, Symbol: 188, ChildCount: 2, ProductionID: 122}},
		77:  {{Type: gotreesitter.ParseActionReduce, Symbol: 171, ChildCount: 1}},
		253: {{Type: gotreesitter.ParseActionReduce, Symbol: 119, ChildCount: 1}},
		254: nil,
	}
	seen := make(map[gotreesitter.StateID]bool, len(want))
	orderedWant := map[cellKey][]gotreesitter.ParseAction{
		{164, 86}: {{Type: gotreesitter.ParseActionShift, State: 410}},
		{194, 86}: {{Type: gotreesitter.ParseActionShift, State: 444}},
		{410, 10}: {{Type: gotreesitter.ParseActionReduce, Symbol: 177, ChildCount: 3, ProductionID: 107}},
		{79, 10}:  {{Type: gotreesitter.ParseActionReduce, Symbol: 171, ChildCount: 1}},
		{18, 10}:  {{Type: gotreesitter.ParseActionReduce, Symbol: 119, ChildCount: 1}},
		{65, 10}:  {{Type: gotreesitter.ParseActionShift, State: 248}},
		{444, 10}: {{Type: gotreesitter.ParseActionReduce, Symbol: 190, ChildCount: 3, ProductionID: 123}},
		{22, 10}:  nil,
	}
	orderedSeen := make(map[cellKey]bool, len(orderedWant))
	continuationSeen := false
	subsequentConflictSeen := false
	productionCells := append(profile.SnapshotTop(-1), profile.SnapshotTopReduceChains(-1)...)
	for _, stat := range productionCells {
		key := cellKey{stat.State, stat.Lookahead}
		if actions, ok := orderedWant[key]; ok && reflect.DeepEqual(stat.Actions, actions) && (stat.Hits > 0 || stat.ReduceChainHits > 0) {
			orderedSeen[key] = true
		}
		if stat.State == 193 && stat.Lookahead == 86 && reflect.DeepEqual(stat.Actions, []gotreesitter.ParseAction{{Type: gotreesitter.ParseActionShift, State: 186}}) && stat.Hits > 0 {
			continuationSeen = true
		}
		if stat.State == 232 && stat.Lookahead == 4 && reflect.DeepEqual(stat.Actions, []gotreesitter.ParseAction{
			{Type: gotreesitter.ParseActionReduce, Symbol: 171, ChildCount: 1},
			{Type: gotreesitter.ParseActionShift, State: 194},
		}) && stat.Hits > 0 {
			subsequentConflictSeen = true
		}
		actions, ok := want[stat.State]
		if !ok || stat.Lookahead != 20 || !reflect.DeepEqual(stat.Actions, actions) || stat.Hits == 0 {
			continue
		}
		seen[stat.State] = true
	}
	for state := range want {
		if !seen[state] {
			t.Fatalf("production scheduler did not authenticate state %d/lookahead 20 actions %+v", state, want[state])
		}
	}
	if !continuationSeen {
		t.Fatal("production scheduler did not authenticate continuation state 193/lookahead 86 shift to state 186")
	}
	if !subsequentConflictSeen {
		t.Fatal("production scheduler did not authenticate subsequent state 232/lookahead 4 conflict")
	}
	for key, actions := range orderedWant {
		if !orderedSeen[key] {
			t.Fatalf("production scheduler did not authenticate ordered-cohort state %d/lookahead %d actions %+v", key.state, key.lookahead, actions)
		}
	}
}

func TestDiagnosticParserCoreRewritePrefixUsesExactProductionElection(t *testing.T) {
	fixtures, err := benchfixtures.LoadGoFullParseFixtures()
	if err != nil {
		t.Fatal(err)
	}
	var rewrite benchfixtures.LoadedFixture
	for _, fixture := range fixtures {
		if fixture.Fixture.ID == "rewrite" {
			rewrite = fixture
			break
		}
	}
	if len(rewrite.Source) == 0 {
		t.Fatal("frozen rewrite fixture not found")
	}
	scanner := grammars.GoExternalScanner{}
	result, prefixErr := gotreesitter.DiagnosticParseParserCorePrefix(scanner, rewrite.Source, gotreesitter.DiagnosticParserCorePrefixOptions{})
	if result.Grammar != "go" || !result.ExactRootDFA || result.Materialized {
		t.Fatalf("route identity = grammar %q exact=%t materialized=%t err=%v", result.Grammar, result.ExactRootDFA, result.Materialized, prefixErr)
	}
	if result.SourceSHA256 == [32]byte{} || result.GrammarBlobSHA256 == [32]byte{} || result.Tokens == 0 || len(result.Elections) == 0 {
		t.Fatalf("prefix did not reach an authenticated election: %+v", result)
	}
	if got := fmt.Sprintf("%x", result.GrammarBlobSHA256); got != "9cf914d26d962d1a62e7954f8b20b302337a44cb7d4a07218eec482c45a57a08" {
		t.Fatalf("embedded grammar receipt = %s", got)
	}
	if prefixErr != nil && result.Boundary == "" {
		t.Fatalf("untyped prefix error: %v", prefixErr)
	}
	if result.Boundary != gotreesitter.DiagnosticParserCoreCohortCondensed || result.Tokens != 98 || result.Dispatches != 189 || result.State != 248 || result.Lookahead.Symbol != 10 || prefixErr == nil {
		t.Fatalf("rewrite prefix identity drifted: boundary=%s tokens=%d dispatches=%d state=%d lookahead=%d fork_actions=%+v fork_boundaries=%d fork_paths=%d same_rounds=%+v last_order=%d", result.Boundary, result.Tokens, result.Dispatches, result.State, result.Lookahead.Symbol, result.ForkActions, result.ForkBoundaries, result.ForkLogicalPaths, result.SameTokenRounds, result.LastBranchOrder)
	}
	if result.Detail != "ordered cohort applied authenticated state22 cost condense" || len(result.Elections) != int(result.Tokens) {
		t.Fatalf("continued prefix detail/elections = %q/%d, want one election per %d tokens", result.Detail, len(result.Elections), result.Tokens)
	}
	if result.OracleCondenseResolution == nil || result.ContinuationElection == nil {
		t.Fatalf("missing oracle-condense continuation receipts: resolution=%+v election=%+v", result.OracleCondenseResolution, result.ContinuationElection)
	}
	lastEpochCheckpoint := result.Elections[67].ScannerAfter
	resolution := result.OracleCondenseResolution
	if !resolution.OraclePinned || !resolution.PausedDropped || resolution.PausedResumed || resolution.PausedEffectiveCost != 100 || resolution.PreservedEffectiveCost != 0 || resolution.Lookahead.Symbol != 20 || resolution.Lookahead.StartByte != 579 || resolution.Lookahead.EndByte != 580 ||
		resolution.Paused.State != 254 || resolution.Paused.ByteOffset != 579 || resolution.Paused.CreationSeq != 0 || resolution.Paused.Shifted || resolution.Paused.ExactPaths != 1 ||
		resolution.Preserved.State != 193 || resolution.Preserved.ByteOffset != 580 || resolution.Preserved.CreationSeq != 1 || !resolution.Preserved.Shifted || resolution.Preserved.ExactPaths != 1 ||
		resolution.PrecedingScannerAfter != lastEpochCheckpoint || resolution.Paused.Checkpoint != lastEpochCheckpoint.SHA256 || resolution.Preserved.Checkpoint != lastEpochCheckpoint.SHA256 {
		t.Fatalf("no-action resolution receipt drifted: %+v prior_checkpoint=%x", resolution, lastEpochCheckpoint)
	}
	continuation := result.ContinuationElection
	if continuation.State != 193 || continuation.ByteOffset != 580 || continuation.ElectionIndex != 68 || !continuation.CheckpointContinuous || continuation.Token.Symbol != 86 || continuation.Token.StartByte != 580 || continuation.Token.EndByte != 586 ||
		continuation.ExpectedBefore != lastEpochCheckpoint || continuation.ExpectedBefore.Length != 0 || continuation.ActualBefore != continuation.ExpectedBefore ||
		continuation.SchedulerHeader.State != 193 || continuation.SchedulerHeader.ByteOffset != 580 || continuation.SchedulerHeader.CreationSeq != 1 || continuation.SchedulerHeader.Shifted || continuation.SchedulerHeader.Accepted || continuation.SchedulerHeader.ExactPaths != 1 || continuation.SchedulerHeader.Checkpoint != result.Elections[68].ScannerAfter.SHA256 ||
		!reflect.DeepEqual(continuation.Token, result.Elections[68].Token) {
		t.Fatalf("continuation election receipt drifted: %+v election=%+v", continuation, result.Elections[68])
	}
	if continuation.HandoffBoundary != gotreesitter.DiagnosticParserCoreSingleStateContinuation {
		t.Fatalf("continuation handoff boundary=%s", continuation.HandoffBoundary)
	}
	if result.LaterForkExecution == nil || result.SubsequentConflict != nil {
		t.Fatalf("later-fork/third-conflict receipts=%+v/%+v", result.LaterForkExecution, result.SubsequentConflict)
	}
	later := result.LaterForkExecution
	checkpoint := result.Elections[95].ScannerAfter.SHA256
	wantLaterActions := []gotreesitter.DiagnosticParserCoreRoundAction{
		{HeaderIndex: 0, State: 232, ByteOffset: 724, Ordinal: 1, Action: gotreesitter.ParseAction{Type: gotreesitter.ParseActionShift, State: 194}, BranchOrder: 2},
		{HeaderIndex: 0, State: 232, ByteOffset: 724, Ordinal: 0, Action: gotreesitter.ParseAction{Type: gotreesitter.ParseActionReduce, Symbol: 171, ChildCount: 1}},
	}
	if later.ElectionIndex != 95 || later.Token.Symbol != 4 || later.Token.Text != "." || later.Token.StartByte != 724 || later.Token.EndByte != 725 ||
		later.BranchOrderBefore != 1 || later.BranchOrderAfter != 2 || later.NextCreationSeqBefore != 2 || later.NextCreationSeqAfter != 3 || later.ExactPaths != 2 || later.Round.Index != 0 ||
		len(later.Round.Before) != 1 || later.Round.Before[0].CreationSeq != 1 || later.Round.Before[0].State != 232 || later.Round.Before[0].ByteOffset != 724 || later.Round.Before[0].Shifted || later.Round.Before[0].ExactPaths != 1 || later.Round.Before[0].Checkpoint != checkpoint ||
		!reflect.DeepEqual(later.Round.Actions, wantLaterActions) || len(later.Round.After) != 2 || len(later.Boundaries) != 2 {
		t.Fatalf("later fork execution=%+v, want actions=%+v checkpoint=%x", later, wantLaterActions, checkpoint)
	}
	wantLaterHeaders := []parserCoreHeaderStep{
		{state: 18, byteOffset: 724, creationSeq: 1},
		{state: 194, byteOffset: 725, creationSeq: 2, shifted: true},
	}
	wantLaterOrders := []uint64{1, 2}
	for index, boundary := range later.Boundaries {
		wantHeader := wantLaterHeaders[index]
		if boundary.Header.State != wantHeader.state || boundary.Header.ByteOffset != wantHeader.byteOffset || boundary.Header.CreationSeq != wantHeader.creationSeq || boundary.Header.Shifted != wantHeader.shifted || boundary.Header.Accepted || boundary.Header.ExactPaths != 1 || boundary.Header.Checkpoint != checkpoint ||
			boundary.Score != -10 || boundary.BranchOrder != wantLaterOrders[index] || !boundary.HasBranchOrder || !reflect.DeepEqual(boundary.Header, later.Round.After[index]) {
			t.Fatalf("later fork boundary %d=%+v, want header=%+v score=-10 order=%d checkpoint=%x", index, boundary, wantHeader, wantLaterOrders[index], checkpoint)
		}
	}
	if later.ClosedExactPaths != 2 || len(later.ClosedBoundaries) != 2 {
		t.Fatalf("later fork closure=%+v exact_paths=%d", later.ClosedBoundaries, later.ClosedExactPaths)
	}
	wantClosedHeaders := []parserCoreHeaderStep{
		{state: 164, byteOffset: 725, creationSeq: 1, shifted: true},
		{state: 194, byteOffset: 725, creationSeq: 2, shifted: true},
	}
	for index, boundary := range later.ClosedBoundaries {
		wantHeader := wantClosedHeaders[index]
		if boundary.Header.State != wantHeader.state || boundary.Header.ByteOffset != wantHeader.byteOffset || boundary.Header.CreationSeq != wantHeader.creationSeq || boundary.Header.Shifted != wantHeader.shifted || boundary.Header.Accepted || boundary.Header.ExactPaths != 1 || boundary.Header.Checkpoint != checkpoint ||
			boundary.Score != -10 || boundary.BranchOrder != wantLaterOrders[index] || !boundary.HasBranchOrder {
			t.Fatalf("later fork closed boundary %d=%+v, want header=%+v score=-10 order=%d checkpoint=%x", index, boundary, wantHeader, wantLaterOrders[index], checkpoint)
		}
	}
	if len(result.OrderedElections) != 2 {
		t.Fatalf("ordered elections=%+v, want two", result.OrderedElections)
	}
	emptyCheckpoint := gotreesitter.DiagnosticParserCoreScannerCheckpoint{Length: 0, SHA256: sha256.Sum256(nil)}
	wantOrderedStates := [][]gotreesitter.StateID{{164, 194}, {410, 444}}
	wantOrderedTokens := []struct {
		symbol     gotreesitter.Symbol
		text       string
		start, end uint32
	}{{86, "edits", 725, 730}, {10, "=", 731, 732}}
	for electionIndex, election := range result.OrderedElections {
		wantToken := wantOrderedTokens[electionIndex]
		if election.Index != electionIndex || !reflect.DeepEqual(election.States, wantOrderedStates[electionIndex]) ||
			election.ExpectedBefore != emptyCheckpoint || election.ActualBefore != emptyCheckpoint || !election.CheckpointContinuous ||
			election.Token.Symbol != wantToken.symbol || election.Token.Text != wantToken.text || election.Token.StartByte != wantToken.start || election.Token.EndByte != wantToken.end ||
			election.Token.Missing || election.Token.NoLookahead || election.Token.ExternalScannerToken ||
			election.BranchOrder != 2 || election.NextCreationSeq != 3 || len(election.Before) != 2 || len(election.Reset) != 2 || len(election.Boundaries) != 2 {
			t.Fatalf("ordered election %d drifted: %+v", electionIndex, election)
		}
		for headerIndex := range election.Before {
			beforeHeader, resetHeader, boundary := election.Before[headerIndex], election.Reset[headerIndex], election.Boundaries[headerIndex]
			if beforeHeader.State != wantOrderedStates[electionIndex][headerIndex] || beforeHeader.CreationSeq != uint64(headerIndex+1) || !beforeHeader.Shifted || beforeHeader.Accepted || beforeHeader.ExactPaths != 1 || beforeHeader.Checkpoint != emptyCheckpoint.SHA256 ||
				resetHeader.State != beforeHeader.State || resetHeader.ByteOffset != beforeHeader.ByteOffset || resetHeader.CreationSeq != beforeHeader.CreationSeq || resetHeader.Shifted || resetHeader.Accepted || resetHeader.ExactPaths != 1 || resetHeader.Checkpoint != emptyCheckpoint.SHA256 ||
				!reflect.DeepEqual(boundary.Header, beforeHeader) || boundary.Score != -10 || boundary.BranchOrder != uint64(headerIndex+1) || !boundary.HasBranchOrder {
				t.Fatalf("ordered election %d header %d drifted: before=%+v reset=%+v boundary=%+v", electionIndex, headerIndex, beforeHeader, resetHeader, boundary)
			}
		}
	}
	wantCohortActions := [][]gotreesitter.DiagnosticParserCoreRoundAction{
		{
			{HeaderIndex: 0, State: 164, ByteOffset: 725, Action: gotreesitter.ParseAction{Type: gotreesitter.ParseActionShift, State: 410}},
			{HeaderIndex: 1, State: 194, ByteOffset: 725, Action: gotreesitter.ParseAction{Type: gotreesitter.ParseActionShift, State: 444}},
		},
		{
			{HeaderIndex: 0, State: 410, ByteOffset: 730, Action: gotreesitter.ParseAction{Type: gotreesitter.ParseActionReduce, Symbol: 177, ChildCount: 3, ProductionID: 107}},
			{HeaderIndex: 0, State: 79, ByteOffset: 730, Action: gotreesitter.ParseAction{Type: gotreesitter.ParseActionReduce, Symbol: 171, ChildCount: 1}},
			{HeaderIndex: 0, State: 18, ByteOffset: 730, Action: gotreesitter.ParseAction{Type: gotreesitter.ParseActionReduce, Symbol: 119, ChildCount: 1}},
			{HeaderIndex: 1, State: 444, ByteOffset: 730, Action: gotreesitter.ParseAction{Type: gotreesitter.ParseActionReduce, Symbol: 190, ChildCount: 3, ProductionID: 123}},
		},
		{{HeaderIndex: 0, State: 65, ByteOffset: 730, Action: gotreesitter.ParseAction{Type: gotreesitter.ParseActionShift, State: 248}}},
	}
	if len(result.CohortRounds) != len(wantCohortActions) {
		t.Fatalf("cohort rounds=%+v, want three production passes", result.CohortRounds)
	}
	for roundIndex, round := range result.CohortRounds {
		if round.Index != roundIndex || !reflect.DeepEqual(round.Actions, wantCohortActions[roundIndex]) {
			t.Fatalf("cohort round %d=%+v, want actions=%+v", roundIndex, round, wantCohortActions[roundIndex])
		}
	}
	condense := result.CohortCondense
	if condense == nil || !condense.Executed || !condense.OraclePinned || !condense.PausedDropped || condense.PausedResumed || condense.ElectionIndex != 97 ||
		condense.PausedOpenRecoveryCost != 500 || condense.PausedSkippedTreeCost != 100 || condense.PausedEffectiveCost != 600 || condense.PreservedEffectiveCost != 0 ||
		condense.Token.Symbol != 10 || condense.Token.Text != "=" || condense.Token.StartByte != 731 || condense.Token.EndByte != 732 ||
		condense.Paused.State != 22 || condense.Paused.ByteOffset != 730 || condense.Paused.CreationSeq != 2 || condense.Paused.Shifted ||
		condense.Preserved.State != 248 || condense.Preserved.ByteOffset != 732 || condense.Preserved.CreationSeq != 1 || !condense.Preserved.Shifted ||
		len(condense.Before) != 2 || len(condense.After) != 1 || len(condense.BeforeBoundaries) != 2 || len(condense.AfterBoundaries) != 1 ||
		condense.BeforeBoundaries[0].Score != -10 || condense.BeforeBoundaries[0].BranchOrder != 1 || !condense.BeforeBoundaries[0].HasBranchOrder ||
		condense.BeforeBoundaries[1].Score != -10 || condense.BeforeBoundaries[1].BranchOrder != 2 || !condense.BeforeBoundaries[1].HasBranchOrder ||
		condense.AfterBoundaries[0].Score != -10 || condense.AfterBoundaries[0].BranchOrder != 1 || !condense.AfterBoundaries[0].HasBranchOrder ||
		!reflect.DeepEqual(condense.After[0], condense.Preserved) {
		t.Fatalf("state22 cohort condense drifted: %+v", condense)
	}
	for _, want := range []struct {
		index      int
		start, end uint32
	}{{index: 72, start: 595, end: 596}, {index: 74, start: 597, end: 598}} {
		election := result.Elections[want.index]
		if election.Token.Symbol != 94 || election.Token.StartByte != want.start || election.Token.EndByte != want.end || !election.Token.ExternalScannerToken ||
			election.ScannerBefore != emptyCheckpoint || election.ScannerAfter != emptyCheckpoint || election.CurrentCheckpointValid ||
			election.CurrentCheckpointStart != emptyCheckpoint || election.CurrentCheckpointEnd != emptyCheckpoint || election.CurrentCheckpointBytes != [2]uint32{} {
			t.Fatalf("external-semicolon election %d=%+v, want symbol 94 span %d..%d with empty serialized state and no current checkpoint", want.index, election, want.start, want.end)
		}
	}
	wantForkActions := []gotreesitter.DiagnosticParserCoreForkAction{
		{Ordinal: 1, Action: gotreesitter.ParseAction{Type: gotreesitter.ParseActionShift, State: 193}, BranchOrder: 1},
		{Ordinal: 0, Action: gotreesitter.ParseAction{Type: gotreesitter.ParseActionReduce, Symbol: 171, ChildCount: 1}},
	}
	wantForkBoundaries := []gotreesitter.DiagnosticParserCoreForkBoundary{
		{State: 193, ByteOffset: 580, ExactPaths: 1},
		{State: 285, ByteOffset: 579, ExactPaths: 1},
	}
	if !reflect.DeepEqual(result.ForkActions, wantForkActions) || !reflect.DeepEqual(result.ForkBoundaryReceipts, wantForkBoundaries) || result.ForkBoundaries != 2 || result.ForkLogicalPaths != 2 {
		t.Fatalf("first fork receipt: actions=%+v boundary_receipts=%+v boundaries=%d paths=%d, want actions=%+v boundary_receipts=%+v boundaries=2 paths=2", result.ForkActions, result.ForkBoundaryReceipts, result.ForkBoundaries, result.ForkLogicalPaths, wantForkActions, wantForkBoundaries)
	}
	if result.LastBranchOrder != 2 || len(result.SameTokenRounds) != 4 {
		t.Fatalf("same-lookahead scheduler receipt = rounds=%d last_branch_order=%d, want 4/2", len(result.SameTokenRounds), result.LastBranchOrder)
	}
	assertParserCoreSameTokenRounds(t, result)
	wantExtraSpans := [][2]uint32{{49, 116}, {117, 183}, {184, 266}, {267, 310}, {457, 517}, {599, 664}}
	wantExtraStates := []gotreesitter.StateID{542, 542, 542, 542, 349, 343}
	if len(result.ExtraShifts) != len(wantExtraSpans) {
		t.Fatalf("plain extra shifts=%+v, want %d", result.ExtraShifts, len(wantExtraSpans))
	}
	for index, extra := range result.ExtraShifts {
		wantAction := gotreesitter.ParseAction{Type: gotreesitter.ParseActionShift, Extra: true}
		if extra.State != wantExtraStates[index] || extra.EffectiveState != wantExtraStates[index] || extra.Token.Symbol != 92 ||
			extra.Token.StartByte != wantExtraSpans[index][0] || extra.Token.EndByte != wantExtraSpans[index][1] ||
			extra.Token.NoLookahead || extra.Token.ExternalScannerToken || !reflect.DeepEqual(extra.Action, wantAction) {
			t.Fatalf("plain extra %d=%+v, want state-preserving DFA comment span=%v action=%+v", index, extra, wantExtraSpans[index], wantAction)
		}
		var election *gotreesitter.DiagnosticParserCoreElection
		for electionIndex := range result.Elections {
			candidate := &result.Elections[electionIndex]
			if candidate.Token.StartByte == extra.Token.StartByte && candidate.Token.EndByte == extra.Token.EndByte {
				election = candidate
				break
			}
		}
		if election == nil || !reflect.DeepEqual(election.Token, extra.Token) || election.ScannerBefore.Length != 0 || election.ScannerAfter.Length != 0 || election.CurrentCheckpointValid {
			t.Fatalf("plain extra election %d=%+v, extra=%+v", index, election, extra)
		}
	}
	wantTrailingReduction := gotreesitter.ParseAction{Type: gotreesitter.ParseActionReduce, Symbol: 197, ChildCount: 4}
	var foundTrailing bool
	for _, reduction := range result.ReductionAttempts {
		if reduction.State == 542 && reduction.Lookahead.Symbol == 16 {
			foundTrailing = reduction.Applied && reflect.DeepEqual(reduction.Action, wantTrailingReduction)
			break
		}
	}
	if !foundTrailing {
		t.Fatalf("state 542/lookahead 16 trailing-extra reduction missing: reductions=%+v", result.ReductionAttempts)
	}
	wantInterstitialReduction := gotreesitter.ParseAction{Type: gotreesitter.ParseActionReduce, Symbol: 197, ChildCount: 3}
	var foundInterstitial bool
	for _, attempt := range result.ReductionAttempts {
		if !attempt.Applied {
			t.Fatalf("pre-fork reduction was not applied: %+v", attempt)
		}
		if attempt.State == 349 && attempt.Lookahead.Symbol == 16 && reflect.DeepEqual(attempt.Action, wantInterstitialReduction) {
			foundInterstitial = true
		}
	}
	if !foundInterstitial {
		t.Fatalf("applied state 349/lookahead 16 interstitial-extra reduction missing")
	}

	// The comparison parse is evidence only. It does not feed tokens, actions,
	// or stack state into the compact scheduler.
	profile := gotreesitter.NewAmbiguityProfile()
	parser := gotreesitter.NewParser(grammars.GoLanguage())
	parser.SetAmbiguityProfile(profile)
	type tokenIdentity struct {
		symbol     gotreesitter.Symbol
		start, end uint32
	}
	var productionTokens []tokenIdentity
	parser.SetLogger(func(kind gotreesitter.ParserLogType, message string) {
		if kind != gotreesitter.ParserLogLex {
			return
		}
		var symbol uint16
		var start, end uint32
		if _, scanErr := fmt.Sscanf(message, "token sym=%d start=%d end=%d", &symbol, &start, &end); scanErr == nil {
			productionTokens = append(productionTokens, tokenIdentity{symbol: gotreesitter.Symbol(symbol), start: start, end: end})
		}
	})
	tree, err := parser.Parse(rewrite.Source)
	if err != nil {
		t.Fatal(err)
	}
	defer tree.Release()
	if tree.ParseStoppedEarly() || tree.RootNode() == nil || tree.RootNode().EndByte() != uint32(len(rewrite.Source)) {
		t.Fatalf("production comparison did not accept frozen rewrite: runtime=%+v", tree.ParseRuntime())
	}
	assertProductionParserCoreCells(t, profile)
	if len(productionTokens) < len(result.Elections) {
		t.Fatalf("production token trace has %d tokens, prefix has %d", len(productionTokens), len(result.Elections))
	}
	for index, election := range result.Elections {
		want := productionTokens[index]
		if election.Token.Symbol != want.symbol || election.Token.StartByte != want.start || election.Token.EndByte != want.end {
			t.Fatalf("election %d token=(%d,%d,%d) production=(%d,%d,%d)", index, election.Token.Symbol, election.Token.StartByte, election.Token.EndByte, want.symbol, want.start, want.end)
		}
	}
	if prefixErr == nil || result.Boundary == "" {
		t.Fatalf("prefix stopped without a typed boundary: result=%+v err=%v", result, prefixErr)
	}
	t.Logf("parser-core rewrite prefix boundary=%s detail=%q tokens=%d dispatches=%d state=%d lookahead=%d fork_boundaries=%+v", result.Boundary, result.Detail, result.Tokens, result.Dispatches, result.State, result.Lookahead.Symbol, result.ForkBoundaryReceipts)
}

func TestDiagnosticParserCoreUsesEmbeddedGrammarAndExactScanner(t *testing.T) {
	fixtures, err := benchfixtures.LoadGoFullParseFixtures()
	if err != nil {
		t.Fatal(err)
	}
	var source []byte
	for _, fixture := range fixtures {
		if fixture.Fixture.ID == "rewrite" {
			source = fixture.Source
			break
		}
	}
	if len(source) == 0 {
		t.Fatal("rewrite fixture missing")
	}
	canonical, canonicalErr := gotreesitter.DiagnosticParseParserCorePrefix(grammars.GoExternalScanner{}, source, gotreesitter.DiagnosticParserCorePrefixOptions{})
	if canonicalErr == nil || canonical.Boundary != gotreesitter.DiagnosticParserCoreCohortCondensed || canonical.Tokens != 98 || canonical.Dispatches != 189 || canonical.State != 248 || canonical.Lookahead.Symbol != 10 || canonical.ForkBoundaries != 2 || canonical.ForkLogicalPaths != 2 || canonical.LastBranchOrder != 2 || len(canonical.SameTokenRounds) != 4 || canonical.OracleCondenseResolution == nil || canonical.ContinuationElection == nil || canonical.LaterForkExecution == nil || len(canonical.OrderedElections) != 2 || canonical.CohortCondense == nil || canonical.SubsequentConflict != nil || !canonical.ExactRootDFA {
		t.Fatalf("exact scanner did not reach authenticated boundary: result=%+v err=%v", canonical, canonicalErr)
	}
	wrongScanners := []struct {
		name    string
		scanner gotreesitter.ExternalScanner
	}{
		{name: "nil"},
		{name: "different", scanner: grammars.JavaScriptExternalScanner{}},
		{name: "pointer", scanner: &grammars.GoExternalScanner{}},
		{name: "wrapper", scanner: wrappedGoScanner{}},
		{name: "fake", scanner: fakeGoScanner{}},
	}
	for _, test := range wrongScanners {
		t.Run(test.name, func(t *testing.T) {
			result, err := gotreesitter.DiagnosticParseParserCorePrefix(test.scanner, source, gotreesitter.DiagnosticParserCorePrefixOptions{})
			if err == nil || result.ExactRootDFA || result.Boundary != gotreesitter.DiagnosticParserCoreIdentity {
				t.Fatalf("wrong scanner admitted: result=%+v err=%v", result, err)
			}
		})
	}
}

func TestDiagnosticParserCoreOrderedCohortRollsBackPartialPass(t *testing.T) {
	fixtures, err := benchfixtures.LoadGoFullParseFixtures()
	if err != nil {
		t.Fatal(err)
	}
	var source []byte
	for _, fixture := range fixtures {
		if fixture.Fixture.ID == "rewrite" {
			source = fixture.Source
			break
		}
	}
	if len(source) == 0 {
		t.Fatal("rewrite fixture missing")
	}
	result, routeErr := gotreesitter.DiagnosticParseParserCorePrefix(
		grammars.GoExternalScanner{}, source,
		gotreesitter.DiagnosticParserCorePrefixOptions{MaxDispatches: 182},
	)
	if routeErr == nil || result.Boundary != gotreesitter.DiagnosticParserCoreCap {
		t.Fatalf("partial ordered pass did not fail closed: result=%+v err=%v", result, routeErr)
	}
	if result.Tokens != 96 || result.Dispatches != 181 || len(result.Elections) != 96 ||
		len(result.OrderedElections) != 0 || len(result.CohortRounds) != 0 || result.CohortCondense != nil ||
		result.LaterForkExecution == nil || len(result.LaterForkExecution.ClosedBoundaries) != 2 {
		t.Fatalf("failed ordered pass leaked staged receipts: tokens=%d dispatches=%d elections=%d ordered=%+v rounds=%+v condense=%+v later=%+v",
			result.Tokens, result.Dispatches, len(result.Elections), result.OrderedElections, result.CohortRounds, result.CohortCondense, result.LaterForkExecution)
	}
}
