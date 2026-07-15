//go:build gts_parsercorephase0

package gotreesitter

import (
	"errors"
	"reflect"
	"testing"

	core "github.com/odvcencio/gotreesitter/internal/parsercorephase0"
)

func TestDiagnosticParserCoreGenericSingleExtraPreservesPayloadAndState(t *testing.T) {
	table := &genericConflictTable{cells: map[genericConflictCell][]core.Action{
		{state: 5, symbol: 9}: {{Type: core.ActionShift, State: 0, Extra: true}},
	}}
	compact, err := core.New(table, core.Limits{})
	if err != nil {
		t.Fatal(err)
	}
	seed, err := compact.Seed(5, 10)
	if err != nil {
		t.Fatal(err)
	}
	before, err := compact.Stats(seed)
	if err != nil {
		t.Fatal(err)
	}
	token := Token{
		Symbol: 9, StartByte: 10, EndByte: 20,
		ExternalScannerToken: true, ExternalScannerStartByte: 10,
	}
	election := DiagnosticParserCoreElection{
		Token:         token,
		ScannerBefore: DiagnosticParserCoreScannerCheckpoint{Length: 1, SHA256: [32]byte{1}},
		ScannerAfter:  DiagnosticParserCoreScannerCheckpoint{Length: 2, SHA256: [32]byte{2}},
	}
	scheduler := &diagnosticParserCoreGenericScheduler{
		compact: compact,
		headers: []diagnosticParserCoreHeader{{head: seed, creationSeq: 4, checkpoint: election.ScannerAfter.SHA256}},
		token:   token, checkpoint: election.ScannerAfter, currentElection: election, electionIndex: 12,
		options: DiagnosticParserCorePrefixOptions{MaxDispatches: 10},
		receipt: &DiagnosticParserCoreGenericScheduler{},
	}
	stop, err := scheduler.dispatchPass()
	if err != nil {
		t.Fatal(err)
	}
	if stop != nil {
		t.Fatalf("single extra declined: %+v", stop)
	}
	if len(scheduler.headers) != 1 || !scheduler.headers[0].shifted {
		t.Fatalf("extra header did not close: %+v", scheduler.headers)
	}
	state, byteOffset, err := compact.Boundary(scheduler.headers[0].head)
	if err != nil {
		t.Fatal(err)
	}
	if state != 5 || byteOffset != 20 {
		t.Fatalf("target-zero extra boundary=(%d,%d), want retained state 5 at byte 20", state, byteOffset)
	}
	after, err := compact.Stats(scheduler.headers[0].head)
	if err != nil {
		t.Fatal(err)
	}
	if after.Nodes-before.Nodes != 1 || after.Links-before.Links != 1 || after.Subtrees-before.Subtrees != 1 || after.Children != before.Children {
		t.Fatalf("extra physical delta before=%+v after=%+v", before, after)
	}
	derivations, err := compact.Derivations(scheduler.headers[0].head)
	if err != nil {
		t.Fatal(err)
	}
	if len(derivations) != 1 || len(derivations[0].Payloads) != 1 {
		t.Fatalf("extra derivations=%+v", derivations)
	}
	view, err := compact.Subtree(derivations[0].Payloads[0])
	if err != nil {
		t.Fatal(err)
	}
	if view.Symbol != 9 || view.StartByte != 10 || view.EndByte != 20 || view.ProductionID != 0 || view.DynamicPrecedence != 0 ||
		len(view.Children) != 0 || len(view.Fields) != 0 || len(view.Aliases) != 0 || !view.Extra || !view.External || !view.Terminal {
		t.Fatalf("extra payload=%+v", view)
	}
	if scheduler.work != (DiagnosticParserCoreGenericWork{
		Passes: 1, ActionLookups: 1, Dispatches: 1, ExtraShifts: 1, Canonicalizations: 1, PeakHeaders: 1,
	}) || len(scheduler.receipt.Rounds) != 1 || len(scheduler.receipt.ExternalShifts) != 1 {
		t.Fatalf("extra accounting work=%+v receipt=%+v", scheduler.work, scheduler.receipt)
	}
	round := scheduler.receipt.Rounds[0]
	if len(round.Actions) != 1 || round.Actions[0].State != 5 || round.Actions[0].ByteOffset != 10 || !round.Actions[0].Action.Extra || round.Actions[0].Action.State != 0 ||
		len(round.After) != 1 || round.After[0].State != 5 || round.After[0].ByteOffset != 20 || !round.After[0].Shifted {
		t.Fatalf("extra round=%+v", round)
	}
	external := scheduler.receipt.ExternalShifts[0]
	if external.ElectionIndex != 12 || external.RoundIndex != 0 || !reflect.DeepEqual(external.Token, token) ||
		external.ScannerBefore != election.ScannerBefore || external.ScannerAfter != election.ScannerAfter || len(external.Payloads) != 1 ||
		!external.Payloads[0].Extra || !external.Payloads[0].External || !external.Payloads[0].Terminal {
		t.Fatalf("external extra receipt=%+v", external)
	}
}

func TestDiagnosticParserCoreGenericMultiHeadExtraSharesPayload(t *testing.T) {
	table := &genericConflictTable{cells: map[genericConflictCell][]core.Action{
		{state: 5, symbol: 9}: {{Type: core.ActionShift, Extra: true}},
		{state: 6, symbol: 9}: {{Type: core.ActionShift, Extra: true}},
	}}
	compact, err := core.New(table, core.Limits{})
	if err != nil {
		t.Fatal(err)
	}
	first, _ := compact.Seed(5, 10)
	second, _ := compact.Seed(6, 10)
	before, _ := compact.Stats(first)
	token := Token{Symbol: 9, StartByte: 10, EndByte: 20, ExternalScannerToken: true}
	election := DiagnosticParserCoreElection{
		Token:         token,
		ScannerBefore: DiagnosticParserCoreScannerCheckpoint{Length: 1, SHA256: [32]byte{1}},
		ScannerAfter:  DiagnosticParserCoreScannerCheckpoint{Length: 2, SHA256: [32]byte{2}},
	}
	scheduler := &diagnosticParserCoreGenericScheduler{
		compact: compact,
		headers: []diagnosticParserCoreHeader{
			{head: first, creationSeq: 4, checkpoint: election.ScannerAfter.SHA256},
			{head: second, creationSeq: 7, checkpoint: election.ScannerAfter.SHA256},
		},
		token: token, checkpoint: election.ScannerAfter, currentElection: election, electionIndex: 12,
		options: DiagnosticParserCorePrefixOptions{MaxDispatches: 10}, receipt: &DiagnosticParserCoreGenericScheduler{},
	}
	stop, err := scheduler.dispatchPass()
	if err != nil || stop != nil {
		t.Fatalf("multi-head extra stop=%+v err=%v", stop, err)
	}
	if len(scheduler.headers) != 2 || !scheduler.headers[0].shifted || !scheduler.headers[1].shifted ||
		scheduler.headers[0].creationSeq != 4 || scheduler.headers[1].creationSeq != 7 {
		t.Fatalf("multi-head extra order/identity drifted: %+v", scheduler.headers)
	}
	after, _ := compact.Stats(scheduler.headers[0].head)
	if after.Nodes-before.Nodes != 2 || after.Links-before.Links != 2 || after.Subtrees-before.Subtrees != 1 || after.Children != before.Children {
		t.Fatalf("multi-head extra physical delta before=%+v after=%+v", before, after)
	}
	var shared core.SubtreeID
	for index, header := range scheduler.headers {
		state, offset, err := compact.Boundary(header.head)
		if err != nil || state != core.StateID(5+index) || offset != 20 {
			t.Fatalf("multi-head extra boundary %d=(%d,%d) err=%v", index, state, offset, err)
		}
		paths, err := compact.Derivations(header.head)
		if err != nil || len(paths) != 1 || len(paths[0].Payloads) != 1 {
			t.Fatalf("multi-head extra paths %d=%+v err=%v", index, paths, err)
		}
		if shared == 0 {
			shared = paths[0].Payloads[0]
		} else if paths[0].Payloads[0] != shared {
			t.Fatalf("multi-head extra payload=%d, want shared %d", paths[0].Payloads[0], shared)
		}
	}
	if scheduler.work != (DiagnosticParserCoreGenericWork{
		Passes: 1, ActionLookups: 2, Dispatches: 2, ExtraShifts: 2, ExtraCohorts: 1, Canonicalizations: 1, PeakHeaders: 2,
	}) || len(scheduler.receipt.Rounds) != 1 || len(scheduler.receipt.Rounds[0].Actions) != 2 || len(scheduler.receipt.ExternalShifts) != 1 || len(scheduler.receipt.ExternalShifts[0].Payloads) != 1 {
		t.Fatalf("multi-head extra accounting work=%+v receipt=%+v", scheduler.work, scheduler.receipt)
	}
	payload := scheduler.receipt.ExternalShifts[0].Payloads[0]
	if payload.ID != uint32(shared) || !payload.Extra || !payload.External || !payload.Terminal {
		t.Fatalf("multi-head external extra payload=%+v shared=%d", payload, shared)
	}
}

func TestDiagnosticParserCoreGenericExtraRejectsMixedFrontier(t *testing.T) {
	table := &genericConflictTable{cells: map[genericConflictCell][]core.Action{
		{state: 5, symbol: 9}: {{Type: core.ActionShift, State: 0, Extra: true}},
		{state: 6, symbol: 9}: {{Type: core.ActionShift, State: 7}},
	}}
	for _, test := range []struct {
		name          string
		secondState   core.StateID
		secondShifted bool
	}{
		{name: "mixed-runnable-frontier", secondState: 6},
		{name: "no-action-sibling", secondState: 7},
		{name: "already-shifted-sibling", secondState: 6, secondShifted: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			compact, err := core.New(table, core.Limits{})
			if err != nil {
				t.Fatal(err)
			}
			first, _ := compact.Seed(5, 10)
			second, _ := compact.Seed(test.secondState, 10)
			headers := []diagnosticParserCoreHeader{{head: first}, {head: second, shifted: test.secondShifted}}
			beforeFirst, _ := compact.Stats(first)
			beforeSecond, _ := compact.Stats(second)
			scheduler := &diagnosticParserCoreGenericScheduler{
				compact: compact, headers: headers, token: Token{Symbol: 9, StartByte: 10, EndByte: 20},
				options: DiagnosticParserCorePrefixOptions{MaxDispatches: 10}, receipt: &DiagnosticParserCoreGenericScheduler{},
			}
			stop, err := scheduler.dispatchPass()
			if err != nil {
				t.Fatal(err)
			}
			if stop == nil || stop.boundary != DiagnosticParserCoreExtra || stop.detail != "generic scheduler requires a homogeneous all-runnable extra cohort" {
				t.Fatalf("multi-head extra stop=%+v", stop)
			}
			afterFirst, _ := compact.Stats(first)
			afterSecond, _ := compact.Stats(second)
			if afterFirst != beforeFirst || afterSecond != beforeSecond || scheduler.dispatches != 0 || scheduler.work.Dispatches != 0 || scheduler.work.ExtraShifts != 0 || len(scheduler.receipt.Rounds) != 0 {
				t.Fatalf("multi-head preflight mutated state: before=(%+v,%+v) after=(%+v,%+v) scheduler=%+v", beforeFirst, beforeSecond, afterFirst, afterSecond, scheduler)
			}
		})
	}
}

func TestDiagnosticParserCoreGenericExtraPostExecutionFailureRollsBack(t *testing.T) {
	table := &genericConflictTable{cells: map[genericConflictCell][]core.Action{
		{state: 5, symbol: 9}: {{Type: core.ActionShift, Extra: true}},
		{state: 6, symbol: 9}: {{Type: core.ActionShift, Extra: true}},
	}}
	compact, err := core.New(table, core.Limits{})
	if err != nil {
		t.Fatal(err)
	}
	first, _ := compact.Seed(5, 10)
	second, _ := compact.Seed(6, 10)
	scheduler := &diagnosticParserCoreGenericScheduler{
		compact: compact, headers: []diagnosticParserCoreHeader{{head: first, creationSeq: 4}, {head: second, creationSeq: 7}},
		token: Token{Symbol: 9, StartByte: 10, EndByte: 20}, options: DiagnosticParserCorePrefixOptions{MaxDispatches: 10},
		receipt: &DiagnosticParserCoreGenericScheduler{}, extraPostExecutionFault: func() error { return errors.New("post-execution fault") },
	}
	beforeStats, _ := compact.Stats(first)
	beforeHeaders := append([]diagnosticParserCoreHeader(nil), scheduler.headers...)
	before, _ := diagnosticParserCoreHeaderReceipts(compact, scheduler.headers)
	cells := []diagnosticParserCoreGenericCell{
		{headerIndex: 0, receipt: before[0], actions: table.cells[genericConflictCell{state: 5, symbol: 9}]},
		{headerIndex: 1, receipt: before[1], actions: table.cells[genericConflictCell{state: 6, symbol: 9}]},
	}
	err = scheduler.applyGenericExtraShifts(before, cells)
	if err == nil {
		t.Fatal("post-execution fault unexpectedly succeeded")
	}
	afterStats, _ := compact.Stats(first)
	if beforeStats != afterStats || !reflect.DeepEqual(scheduler.headers, beforeHeaders) || scheduler.dispatches != 0 || scheduler.work != (DiagnosticParserCoreGenericWork{}) || len(scheduler.receipt.Rounds) != 0 || len(scheduler.receipt.ExternalShifts) != 0 {
		t.Fatalf("extra post-execution rollback leaked before=%+v after=%+v scheduler=%+v", beforeStats, afterStats, scheduler)
	}
	for _, state := range []core.StateID{5, 6} {
		if _, ok := compact.CanonicalBoundary(state, 20, true, [32]byte{}); ok {
			t.Fatalf("extra post-execution rollback left state %d", state)
		}
	}
}

func TestDiagnosticParserCoreGenericExtraConflictFailsPreflight(t *testing.T) {
	table := &genericConflictTable{cells: map[genericConflictCell][]core.Action{
		{state: 5, symbol: 9}: {
			{Type: core.ActionShift, State: 7},
			{Type: core.ActionShift, State: 0, Extra: true},
		},
	}}
	compact, err := core.New(table, core.Limits{})
	if err != nil {
		t.Fatal(err)
	}
	seed, err := compact.Seed(5, 10)
	if err != nil {
		t.Fatal(err)
	}
	before, _ := compact.Stats(seed)
	scheduler := &diagnosticParserCoreGenericScheduler{
		compact: compact, headers: []diagnosticParserCoreHeader{{head: seed}},
		token:   Token{Symbol: 9, StartByte: 10, EndByte: 20},
		options: DiagnosticParserCorePrefixOptions{MaxDispatches: 10}, receipt: &DiagnosticParserCoreGenericScheduler{},
	}
	stop, err := scheduler.dispatchPass()
	if err != nil {
		t.Fatal(err)
	}
	if stop == nil || stop.boundary != DiagnosticParserCoreExtra || stop.detail != "generic scheduler extra action is not one sole shift" {
		t.Fatalf("extra conflict stop=%+v", stop)
	}
	after, _ := compact.Stats(seed)
	if after != before || scheduler.dispatches != 0 || scheduler.work.Dispatches != 0 || scheduler.work.ExtraShifts != 0 || len(scheduler.receipt.Rounds) != 0 {
		t.Fatalf("extra conflict preflight mutated state: before=%+v after=%+v scheduler=%+v", before, after, scheduler)
	}
}
