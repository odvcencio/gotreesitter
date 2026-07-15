//go:build gts_parsercorephase0

package gotreesitter

import (
	"errors"
	"math"
	"reflect"
	"testing"

	core "github.com/odvcencio/gotreesitter/internal/parsercorephase0"
)

func TestDiagnosticParserCorePointIndexPollsBeforeScanning(t *testing.T) {
	want := errors.New("stop")
	polls := 0
	index, err := newDiagnosticParserCorePointIndex([]byte("first\nsecond\n"), func() error {
		polls++
		return want
	})
	if !errors.Is(err, want) || polls != 1 || index.lineStarts != nil {
		t.Fatalf("stopped point index=%+v err=%v polls=%d", index, err, polls)
	}
}

type genericConflictTable struct {
	actions []core.Action
	cells   map[genericConflictCell][]core.Action
	gotos   map[genericConflictCell]core.StateID
}

func (t *genericConflictTable) Actions(state core.StateID, symbol core.Symbol) ([]core.Action, error) {
	if actions := t.cells[genericConflictCell{state: state, symbol: symbol}]; actions != nil {
		return append([]core.Action(nil), actions...), nil
	}
	if state == 1 && symbol == 9 {
		return append([]core.Action(nil), t.actions...), nil
	}
	return nil, nil
}

type genericConflictCell struct {
	state  core.StateID
	symbol core.Symbol
}

func (t *genericConflictTable) Goto(state core.StateID, symbol core.Symbol) (core.StateID, error) {
	return t.gotos[genericConflictCell{state: state, symbol: symbol}], nil
}

func (*genericConflictTable) ProductionFields(uint16, int) ([]core.FieldMapEntry, error) {
	return nil, nil
}

func (*genericConflictTable) ProductionAliases(uint16, int) ([]core.Symbol, error) {
	return nil, nil
}

func TestDiagnosticParserCoreGenericConflictArbitraryNOrdering(t *testing.T) {
	actions := []core.Action{
		{Type: core.ActionShift, State: 2},
		{Type: core.ActionShift, State: 3},
		{Type: core.ActionShift, State: 4},
	}
	compact, err := core.New(&genericConflictTable{actions: actions}, core.Limits{})
	if err != nil {
		t.Fatal(err)
	}
	prefix, err := compact.Seed(9, 0)
	if err != nil {
		t.Fatal(err)
	}
	incoming, err := compact.Seed(1, 0)
	if err != nil {
		t.Fatal(err)
	}
	suffix, err := compact.Seed(8, 0)
	if err != nil {
		t.Fatal(err)
	}
	headers := []diagnosticParserCoreHeader{
		{head: prefix, creationSeq: 2},
		{head: prefix, creationSeq: 3},
		{head: incoming, creationSeq: 4},
		{head: suffix, creationSeq: 6},
	}
	before, err := diagnosticParserCoreHeaderReceipts(compact, headers)
	if err != nil {
		t.Fatal(err)
	}
	scheduler := &diagnosticParserCoreGenericScheduler{
		compact: compact, headers: headers, token: Token{Symbol: 9, StartByte: 0, EndByte: 1, ExternalScannerToken: true, ExternalScannerStartByte: 0},
		branchOrder: 7, nextSeq: 10, options: DiagnosticParserCorePrefixOptions{MaxDispatches: 100},
		electionIndex: 44,
		currentElection: DiagnosticParserCoreElection{
			Token:         Token{Symbol: 9, StartByte: 0, EndByte: 1, ExternalScannerToken: true, ExternalScannerStartByte: 0},
			ScannerBefore: DiagnosticParserCoreScannerCheckpoint{Length: 1, SHA256: [32]byte{1}},
			ScannerAfter:  DiagnosticParserCoreScannerCheckpoint{Length: 2, SHA256: [32]byte{2}},
		},
		receipt: &DiagnosticParserCoreGenericScheduler{},
	}
	if err := scheduler.applyGenericConflict(before, diagnosticParserCoreGenericCell{
		headerIndex: 2, receipt: before[2], actions: actions,
	}); err != nil {
		t.Fatal(err)
	}
	receipts, err := diagnosticParserCoreHeaderReceipts(compact, scheduler.headers)
	if err != nil {
		t.Fatal(err)
	}
	var states []StateID
	var sequences []uint64
	for _, receipt := range receipts {
		states = append(states, receipt.State)
		sequences = append(sequences, receipt.CreationSeq)
	}
	if !reflect.DeepEqual(states, []StateID{9, 2, 8, 3, 4}) || !reflect.DeepEqual(sequences, []uint64{2, 4, 6, 10, 11}) {
		t.Fatalf("physical conflict order states=%v sequences=%v", states, sequences)
	}
	if scheduler.branchOrder != 9 || scheduler.nextSeq != 12 || scheduler.dispatches != 1 || scheduler.work != (DiagnosticParserCoreGenericWork{
		Dispatches: 1, Conflicts: 1, ConflictActions: 3, Forks: 2, ConflictHeads: 3,
		Canonicalizations: 1, PeakHeaders: 5,
	}) {
		t.Fatalf("scheduler allocation/work drift: order=%d seq=%d dispatches=%d work=%+v", scheduler.branchOrder, scheduler.nextSeq, scheduler.dispatches, scheduler.work)
	}
	conflict := scheduler.receipt.Conflicts[0]
	if len(conflict.Round.Actions) != 3 || conflict.Round.Actions[0].Ordinal != 1 || conflict.Round.Actions[0].BranchOrder != 8 ||
		conflict.Round.Actions[1].Ordinal != 2 || conflict.Round.Actions[1].BranchOrder != 9 || conflict.Round.Actions[2].Ordinal != 0 ||
		len(conflict.Prefix) != 2 || conflict.PrimaryOutput.State != 2 || len(conflict.OriginalSuffix) != 1 ||
		len(conflict.SecondaryArms) != 2 || len(conflict.SecondaryArms[0].Outputs) != 1 || len(conflict.SecondaryArms[1].Outputs) != 1 ||
		len(conflict.AdditionalPrimaryOutputs) != 0 || len(conflict.After) != 5 {
		t.Fatalf("three-action conflict receipt drifted: %+v", conflict)
	}
	if len(scheduler.receipt.ExternalShifts) != 1 {
		t.Fatalf("external conflict receipts=%+v, want one round", scheduler.receipt.ExternalShifts)
	}
	external := scheduler.receipt.ExternalShifts[0]
	if external.ElectionIndex != 44 || external.RoundIndex != 0 || !reflect.DeepEqual(external.Token, scheduler.currentElection.Token) ||
		external.ScannerBefore != scheduler.currentElection.ScannerBefore || external.ScannerAfter != scheduler.currentElection.ScannerAfter || len(external.Payloads) != 3 {
		t.Fatalf("external conflict receipt drifted: %+v", external)
	}
	for index, payload := range external.Payloads {
		if payload.ID != uint32(index+1) || payload.Symbol != 9 || payload.StartByte != 0 || payload.EndByte != 1 ||
			payload.ProductionID != 0 || payload.DynamicPrecedence != 0 || len(payload.Children) != 0 || len(payload.Fields) != 0 || len(payload.Aliases) != 0 ||
			!payload.Terminal || !payload.External || payload.Extra {
			t.Fatalf("external conflict payload %d=%+v", index, payload)
		}
	}
}

func TestDiagnosticParserCoreGenericConflictMultiOutputSequencing(t *testing.T) {
	actions := []core.Action{
		{Type: core.ActionReduce, Symbol: 7, ChildCount: 1},
		{Type: core.ActionShift, State: 11},
		{Type: core.ActionReduce, Symbol: 8, ChildCount: 1},
	}
	table := &genericConflictTable{
		cells: map[genericConflictCell][]core.Action{
			{state: 1, symbol: 9}:  {{Type: core.ActionShift, State: 3}},
			{state: 2, symbol: 9}:  {{Type: core.ActionShift, State: 3}},
			{state: 3, symbol: 10}: actions,
		},
		gotos: map[genericConflictCell]core.StateID{
			{state: 1, symbol: 7}: 4,
			{state: 2, symbol: 7}: 5,
			{state: 1, symbol: 8}: 6,
			{state: 2, symbol: 8}: 7,
		},
	}
	compact, err := core.New(table, core.Limits{MaxDerivations: 8, MaxPopPaths: 8})
	if err != nil {
		t.Fatal(err)
	}
	first, err := compact.Seed(1, 0)
	if err != nil {
		t.Fatal(err)
	}
	second, err := compact.Seed(2, 0)
	if err != nil {
		t.Fatal(err)
	}
	firstShifted, err := compact.Shift(first, 9, 0, core.Token{Symbol: 9, EndByte: 1}, core.ForkOrder{Present: true, Value: 1})
	if err != nil {
		t.Fatal(err)
	}
	secondShifted, err := compact.Shift(second, 9, 0, core.Token{Symbol: 9, EndByte: 1}, core.ForkOrder{Present: true, Value: 2})
	if err != nil {
		t.Fatal(err)
	}
	shiftedStats, err := compact.Stats(secondShifted)
	if err != nil {
		t.Fatal(err)
	}
	if firstShifted == secondShifted || shiftedStats.CurrentExactPaths != 2 {
		t.Fatalf("second immutable head did not retain both converged paths: first=%v second=%v stats=%+v", firstShifted, secondShifted, shiftedStats)
	}
	if err := compact.BeginFrontier(); err != nil {
		t.Fatal(err)
	}
	beforeConflict, err := compact.Stats(secondShifted)
	if err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		name    string
		nextSeq uint64
	}{
		{name: "partial-secondary-sequence-overflow", nextSeq: math.MaxUint64 - 2},
		{name: "partial-primary-sequence-overflow", nextSeq: math.MaxUint64 - 3},
	} {
		t.Run(test.name, func(t *testing.T) {
			if _, err := executeDiagnosticParserCoreConflictDetailed(
				compact, diagnosticParserCoreHeader{head: secondShifted, creationSeq: 4}, 0,
				Token{Symbol: 10, StartByte: 1, EndByte: 2}, actions, 7, test.nextSeq,
			); err == nil {
				t.Fatal("overflowing multi-output conflict unexpectedly succeeded")
			}
			afterConflict, err := compact.Stats(secondShifted)
			if err != nil {
				t.Fatal(err)
			}
			if afterConflict != beforeConflict {
				t.Fatalf("multi-output overflow leaked graph mutation: before=%+v after=%+v", beforeConflict, afterConflict)
			}
		})
	}
	prefix, err := compact.Seed(9, 1)
	if err != nil {
		t.Fatal(err)
	}
	suffix, err := compact.Seed(8, 1)
	if err != nil {
		t.Fatal(err)
	}
	headers := []diagnosticParserCoreHeader{
		{head: prefix, creationSeq: 2},
		{head: secondShifted, creationSeq: 4},
		{head: suffix, creationSeq: 6},
	}
	before, err := diagnosticParserCoreHeaderReceipts(compact, headers)
	if err != nil {
		t.Fatal(err)
	}
	scheduler := &diagnosticParserCoreGenericScheduler{
		compact: compact, headers: headers, token: Token{Symbol: 10, StartByte: 1, EndByte: 2},
		branchOrder: 7, nextSeq: 10, options: DiagnosticParserCorePrefixOptions{MaxDispatches: 100},
		receipt: &DiagnosticParserCoreGenericScheduler{},
	}
	if err := scheduler.applyGenericConflict(before, diagnosticParserCoreGenericCell{
		headerIndex: 1, receipt: before[1], actions: actions,
	}); err != nil {
		t.Fatal(err)
	}
	receipts, err := diagnosticParserCoreHeaderReceipts(compact, scheduler.headers)
	if err != nil {
		t.Fatal(err)
	}
	var states []StateID
	var sequences []uint64
	for _, receipt := range receipts {
		states = append(states, receipt.State)
		sequences = append(sequences, receipt.CreationSeq)
	}
	if !reflect.DeepEqual(states, []StateID{9, 4, 8, 11, 6, 7, 5}) ||
		!reflect.DeepEqual(sequences, []uint64{2, 4, 6, 10, 11, 12, 13}) {
		t.Fatalf("multi-output order states=%v sequences=%v", states, sequences)
	}
	conflict := scheduler.receipt.Conflicts[0]
	if scheduler.branchOrder != 9 || scheduler.nextSeq != 14 || conflict.PrimaryOutput.State != 4 || len(conflict.AdditionalPrimaryOutputs) != 1 || conflict.AdditionalPrimaryOutputs[0].State != 5 ||
		len(conflict.SecondaryArms) != 2 || len(conflict.SecondaryArms[0].Outputs) != 1 || len(conflict.SecondaryArms[1].Outputs) != 2 ||
		len(conflict.Round.Actions) != 3 || conflict.Round.Actions[0].Ordinal != 1 || conflict.Round.Actions[1].Ordinal != 2 || conflict.Round.Actions[2].Ordinal != 0 {
		t.Fatalf("multi-output conflict allocation drifted: order=%d seq=%d receipt=%+v", scheduler.branchOrder, scheduler.nextSeq, conflict)
	}
}

func TestDiagnosticParserCoreHeaderPathsMarksBoundedObservation(t *testing.T) {
	table := &genericConflictTable{cells: make(map[genericConflictCell][]core.Action)}
	for state := core.StateID(1); state <= 64; state++ {
		target := core.StateID(100 + (state-1)/8)
		table.cells[genericConflictCell{state: state, symbol: 1}] = []core.Action{{Type: core.ActionShift, State: target}}
		table.cells[genericConflictCell{target, 2}] = []core.Action{{Type: core.ActionShift, State: 200}}
	}
	table.cells[genericConflictCell{state: 200, symbol: 3}] = []core.Action{{Type: core.ActionShift, State: 300}}
	table.cells[genericConflictCell{state: 1000, symbol: 3}] = []core.Action{{Type: core.ActionShift, State: 300}}

	compact, err := core.New(table, core.Limits{MaxDerivations: 64})
	if err != nil {
		t.Fatal(err)
	}
	groups := make([]core.Head, 8)
	for state := core.StateID(1); state <= 64; state++ {
		seed, seedErr := compact.Seed(state, 0)
		if seedErr != nil {
			t.Fatal(seedErr)
		}
		group, shiftErr := compact.Shift(seed, 1, 0, core.Token{Symbol: 1, EndByte: 1}, core.ForkOrder{})
		if shiftErr != nil {
			t.Fatal(shiftErr)
		}
		groups[(state-1)/8] = group
	}
	spare, err := compact.Seed(1000, 1)
	if err != nil {
		t.Fatal(err)
	}
	if err := compact.BeginFrontier(); err != nil {
		t.Fatal(err)
	}
	var sixtyFour core.Head
	for _, group := range groups {
		sixtyFour, err = compact.Shift(group, 2, 0, core.Token{Symbol: 2, StartByte: 1, EndByte: 2}, core.ForkOrder{})
		if err != nil {
			t.Fatal(err)
		}
	}
	if err := compact.BeginFrontier(); err != nil {
		t.Fatal(err)
	}
	head, err := compact.Shift(sixtyFour, 3, 0, core.Token{Symbol: 3, StartByte: 2, EndByte: 3}, core.ForkOrder{})
	if err != nil {
		t.Fatal(err)
	}
	head, err = compact.Shift(spare, 3, 0, core.Token{Symbol: 3, StartByte: 2, EndByte: 3}, core.ForkOrder{})
	if err != nil {
		t.Fatal(err)
	}
	receipt, err := diagnosticParserCoreHeaderPaths(compact, diagnosticParserCoreHeader{head: head})
	if err != nil {
		t.Fatal(err)
	}
	if receipt.Header.ExactPaths != 65 || !receipt.DerivationsTruncated || len(receipt.Derivations) != 0 {
		t.Fatalf("bounded path receipt=%+v, want 65 paths with explicitly truncated derivations", receipt)
	}
}

func TestDiagnosticParserCoreGenericConflictArenaCapsRollback(t *testing.T) {
	actions := []core.Action{
		{Type: core.ActionShift, State: 2},
		{Type: core.ActionShift, State: 3},
		{Type: core.ActionShift, State: 4},
	}
	for _, test := range []struct {
		name        string
		maxSubtrees uint32
	}{
		{name: "partial-secondary", maxSubtrees: 1},
		{name: "partial-primary", maxSubtrees: 2},
	} {
		t.Run(test.name, func(t *testing.T) {
			compact, err := core.New(&genericConflictTable{actions: actions}, core.Limits{MaxSubtrees: test.maxSubtrees})
			if err != nil {
				t.Fatal(err)
			}
			incoming, err := compact.Seed(1, 0)
			if err != nil {
				t.Fatal(err)
			}
			before, err := compact.Stats(incoming)
			if err != nil {
				t.Fatal(err)
			}
			header := diagnosticParserCoreHeader{head: incoming, creationSeq: 4}
			receipt, err := diagnosticParserCoreHeaderReceipt(compact, header)
			if err != nil {
				t.Fatal(err)
			}
			scheduler := &diagnosticParserCoreGenericScheduler{
				compact: compact, headers: []diagnosticParserCoreHeader{header},
				token: Token{Symbol: 9, StartByte: 0, EndByte: 1}, branchOrder: 7, nextSeq: 10,
				options: DiagnosticParserCorePrefixOptions{MaxDispatches: 100}, receipt: &DiagnosticParserCoreGenericScheduler{},
			}
			if err := compact.ApplyAtomic(func() error {
				return scheduler.applyGenericConflict([]DiagnosticParserCoreHeaderReceipt{receipt}, diagnosticParserCoreGenericCell{
					headerIndex: 0, receipt: receipt, actions: actions,
				})
			}); err == nil {
				t.Fatal("capped conflict unexpectedly succeeded")
			}
			after, err := compact.Stats(incoming)
			if err != nil {
				t.Fatal(err)
			}
			if after != before {
				t.Fatalf("capped conflict leaked graph mutation: before=%+v after=%+v", before, after)
			}
			if scheduler.dispatches != 0 || scheduler.branchOrder != 7 || scheduler.nextSeq != 10 || scheduler.work != (DiagnosticParserCoreGenericWork{}) ||
				len(scheduler.headers) != 1 || scheduler.headers[0] != header || !reflect.DeepEqual(scheduler.receipt, &DiagnosticParserCoreGenericScheduler{}) {
				t.Fatalf("capped conflict leaked scheduler allocation/receipt state: %+v receipt=%+v", scheduler, scheduler.receipt)
			}
		})
	}
}

func TestDiagnosticParserCoreGenericConflictPreflight(t *testing.T) {
	actions := []core.Action{
		{Type: core.ActionShift, State: 2},
		{Type: core.ActionShift, State: 3},
		{Type: core.ActionShift, State: 4},
	}
	compact, err := core.New(&genericConflictTable{actions: actions}, core.Limits{})
	if err != nil {
		t.Fatal(err)
	}
	incoming, err := compact.Seed(1, 0)
	if err != nil {
		t.Fatal(err)
	}
	before, _ := compact.Stats(incoming)
	for _, test := range []struct {
		name        string
		branchOrder uint64
		nextSeq     uint64
	}{
		{name: "branch-order", branchOrder: math.MaxUint64 - 1, nextSeq: 1},
		{name: "creation-sequence", branchOrder: 1, nextSeq: math.MaxUint64 - 1},
	} {
		t.Run(test.name, func(t *testing.T) {
			if _, err := executeDiagnosticParserCoreConflictDetailed(
				compact, diagnosticParserCoreHeader{head: incoming}, 0,
				Token{Symbol: 9, StartByte: 0, EndByte: 1}, actions, test.branchOrder, test.nextSeq,
			); err == nil {
				t.Fatal("overflowing conflict unexpectedly succeeded")
			}
			after, _ := compact.Stats(incoming)
			if after != before {
				t.Fatalf("preflight failure mutated graph: before=%+v after=%+v", before, after)
			}
		})
	}
}

func TestDiagnosticParserCoreGenericConflictRejectsUnsupportedArmBeforeMutation(t *testing.T) {
	actions := []core.Action{
		{Type: core.ActionShift, State: 2},
		{Type: core.ActionShift, State: 3, Extra: true},
	}
	compact, err := core.New(&genericConflictTable{actions: actions}, core.Limits{})
	if err != nil {
		t.Fatal(err)
	}
	incoming, err := compact.Seed(1, 0)
	if err != nil {
		t.Fatal(err)
	}
	before, _ := compact.Stats(incoming)
	scheduler := &diagnosticParserCoreGenericScheduler{
		compact: compact, headers: []diagnosticParserCoreHeader{{head: incoming}},
		token: Token{Symbol: 9, StartByte: 0, EndByte: 1}, options: DiagnosticParserCorePrefixOptions{MaxDispatches: 100},
		receipt: &DiagnosticParserCoreGenericScheduler{},
	}
	stop, err := scheduler.dispatchPass()
	if err != nil {
		t.Fatal(err)
	}
	if stop == nil || stop.boundary != DiagnosticParserCoreExtra || scheduler.dispatches != 0 || scheduler.work.ActionLookups != 1 {
		t.Fatalf("unsupported-arm preflight stop=%+v dispatches=%d work=%+v", stop, scheduler.dispatches, scheduler.work)
	}
	after, _ := compact.Stats(incoming)
	if after != before {
		t.Fatalf("unsupported-arm preflight mutated graph: before=%+v after=%+v", before, after)
	}
}

func TestDiagnosticParserCoreGenericAcceptRequiresSoleFrontierBeforeMutation(t *testing.T) {
	accept := []core.Action{{Type: core.ActionAccept}}
	for _, test := range []struct {
		name          string
		cells         map[genericConflictCell][]core.Action
		headerStates  []core.StateID
		configureHead func(*diagnosticParserCoreHeader)
	}{
		{
			name:         "accept-and-no-action",
			cells:        map[genericConflictCell][]core.Action{{state: 1, symbol: 0}: accept},
			headerStates: []core.StateID{1, 2},
		},
		{
			name:          "accept-and-shifted-sibling",
			cells:         map[genericConflictCell][]core.Action{{state: 1, symbol: 0}: accept},
			headerStates:  []core.StateID{1, 2},
			configureHead: func(header *diagnosticParserCoreHeader) { header.shifted = true },
		},
		{
			name: "accept-and-reduction-sibling",
			cells: map[genericConflictCell][]core.Action{
				{state: 1, symbol: 0}: accept,
				{state: 2, symbol: 0}: {{Type: core.ActionReduce, Symbol: 3}},
			},
			headerStates: []core.StateID{1, 2},
		},
		{
			name: "multiple-accepts",
			cells: map[genericConflictCell][]core.Action{
				{state: 1, symbol: 0}: accept,
				{state: 2, symbol: 0}: accept,
			},
			headerStates: []core.StateID{1, 2},
		},
		{
			name: "accept-in-fork",
			cells: map[genericConflictCell][]core.Action{
				{state: 1, symbol: 0}: {{Type: core.ActionAccept}, {Type: core.ActionReduce, Symbol: 3}},
			},
			headerStates: []core.StateID{1},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			compact, err := core.New(&genericConflictTable{cells: test.cells}, core.Limits{})
			if err != nil {
				t.Fatal(err)
			}
			headers := make([]diagnosticParserCoreHeader, len(test.headerStates))
			for index, state := range test.headerStates {
				head, seedErr := compact.Seed(state, 0)
				if seedErr != nil {
					t.Fatal(seedErr)
				}
				headers[index] = diagnosticParserCoreHeader{head: head, creationSeq: uint64(index)}
			}
			if test.configureHead != nil {
				test.configureHead(&headers[len(headers)-1])
			}
			beforeHeaders := append([]diagnosticParserCoreHeader(nil), headers...)
			beforeStats, err := compact.Stats(headers[0].head)
			if err != nil {
				t.Fatal(err)
			}
			scheduler := &diagnosticParserCoreGenericScheduler{
				compact: compact, headers: headers,
				token:   Token{Symbol: 0, StartByte: 0, EndByte: 0},
				options: DiagnosticParserCorePrefixOptions{MaxDispatches: 100},
				receipt: &DiagnosticParserCoreGenericScheduler{},
			}
			stop, err := scheduler.dispatchPass()
			if err != nil {
				t.Fatal(err)
			}
			if stop == nil {
				t.Fatal("mixed accept frontier unexpectedly dispatched")
			}
			afterStats, err := compact.Stats(headers[0].head)
			if err != nil {
				t.Fatal(err)
			}
			if afterStats != beforeStats || !reflect.DeepEqual(scheduler.headers, beforeHeaders) || scheduler.dispatches != 0 || scheduler.work.Accepts != 0 || len(scheduler.receipt.Rounds) != 0 || scheduler.acceptedHead.Node != 0 {
				t.Fatalf("mixed accept mutated state: stop=%+v beforeStats=%+v afterStats=%+v scheduler=%+v", stop, beforeStats, afterStats, scheduler)
			}
		})
	}
}
