//go:build gts_parsercorephase0

package gotreesitter

import (
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

func TestDiagnosticParserCoreGenericExtraRequiresSoleHeader(t *testing.T) {
	table := &genericConflictTable{cells: map[genericConflictCell][]core.Action{
		{state: 5, symbol: 9}: {{Type: core.ActionShift, State: 0, Extra: true}},
		{state: 6, symbol: 9}: {{Type: core.ActionShift, State: 7}},
	}}
	for _, test := range []struct {
		name          string
		secondShifted bool
	}{
		{name: "mixed-runnable-frontier"},
		{name: "already-shifted-sibling", secondShifted: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			compact, err := core.New(table, core.Limits{})
			if err != nil {
				t.Fatal(err)
			}
			first, _ := compact.Seed(5, 10)
			second, _ := compact.Seed(6, 10)
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
			if stop == nil || stop.boundary != DiagnosticParserCoreExtra || stop.detail != "generic scheduler extra shift requires one sole runnable head" {
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
