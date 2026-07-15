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
	continuationSeen := false
	subsequentConflictSeen := false
	for _, stat := range profile.SnapshotTop(-1) {
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
	if result.Boundary != gotreesitter.DiagnosticParserCoreSubsequentConflictBoundary || result.Tokens != 96 || result.Dispatches != 180 || result.State != 232 || result.Lookahead.Symbol != 4 || prefixErr == nil {
		t.Fatalf("rewrite prefix identity drifted: boundary=%s tokens=%d dispatches=%d state=%d lookahead=%d fork_actions=%+v fork_boundaries=%d fork_paths=%d same_rounds=%+v last_order=%d", result.Boundary, result.Tokens, result.Dispatches, result.State, result.Lookahead.Symbol, result.ForkActions, result.ForkBoundaries, result.ForkLogicalPaths, result.SameTokenRounds, result.LastBranchOrder)
	}
	if result.Detail != "later multi-action cell reached before execution" || len(result.Elections) != int(result.Tokens) {
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
	if result.SubsequentConflict == nil {
		t.Fatal("missing subsequent-conflict receipt")
	}
	subsequent := result.SubsequentConflict
	wantSubsequentActions := []gotreesitter.ParseAction{
		{Type: gotreesitter.ParseActionReduce, Symbol: 171, ChildCount: 1},
		{Type: gotreesitter.ParseActionShift, State: 194},
	}
	if subsequent.State != 232 || subsequent.ByteOffset != 724 || subsequent.ElectionIndex != 95 || subsequent.Score != -10 || subsequent.BranchOrder != 1 || !subsequent.HasBranchOrder || subsequent.Token.Symbol != 4 || subsequent.Token.Text != "." || subsequent.Token.StartByte != 724 || subsequent.Token.EndByte != 725 ||
		subsequent.Header.State != 232 || subsequent.Header.ByteOffset != 724 || subsequent.Header.CreationSeq != 1 || subsequent.Header.Shifted || subsequent.Header.Accepted || subsequent.Header.ExactPaths != 1 || subsequent.Header.Checkpoint != result.Elections[95].ScannerAfter.SHA256 ||
		!reflect.DeepEqual(subsequent.Actions, wantSubsequentActions) {
		t.Fatalf("subsequent conflict receipt=%+v, want actions=%+v checkpoint=%x", subsequent, wantSubsequentActions, result.Elections[95].ScannerAfter.SHA256)
	}
	emptyCheckpoint := gotreesitter.DiagnosticParserCoreScannerCheckpoint{Length: 0, SHA256: sha256.Sum256(nil)}
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
	if result.LastBranchOrder != 1 || len(result.SameTokenRounds) != 4 {
		t.Fatalf("same-lookahead scheduler receipt = rounds=%d last_branch_order=%d, want 4/1", len(result.SameTokenRounds), result.LastBranchOrder)
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
	if canonicalErr == nil || canonical.Boundary != gotreesitter.DiagnosticParserCoreSubsequentConflictBoundary || canonical.Tokens != 96 || canonical.Dispatches != 180 || canonical.State != 232 || canonical.Lookahead.Symbol != 4 || canonical.ForkBoundaries != 2 || canonical.ForkLogicalPaths != 2 || canonical.LastBranchOrder != 1 || len(canonical.SameTokenRounds) != 4 || canonical.OracleCondenseResolution == nil || canonical.ContinuationElection == nil || canonical.SubsequentConflict == nil || !canonical.ExactRootDFA {
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
