//go:build gts_parsercorephase0

package gotreesitter

import (
	"errors"
	"math"
	"reflect"
	"testing"

	core "github.com/odvcencio/gotreesitter/internal/parsercorephase0"
)

func newGenericFreshnessSource(t *testing.T, table *genericConflictTable) (*core.Core, core.Head) {
	t.Helper()
	compact, err := core.New(table, core.Limits{MaxDerivations: 8, MaxPopPaths: 8})
	if err != nil {
		t.Fatal(err)
	}
	seed, err := compact.Seed(1, 0)
	if err != nil {
		t.Fatal(err)
	}
	head, err := compact.Shift(seed, 8, 0, core.Token{Symbol: 8, EndByte: 1}, core.ForkOrder{})
	if err != nil {
		t.Fatal(err)
	}
	return compact, head
}

func TestDiagnosticParserCoreGenericReductionPauseIsFinite(t *testing.T) {
	table := &genericConflictTable{
		cells: map[genericConflictCell][]core.Action{
			{state: 1, symbol: 8}: {{Type: core.ActionShift, State: 3}},
			{state: 3, symbol: 9}: {{Type: core.ActionReduce, Symbol: 2, ChildCount: 1}},
			{state: 4, symbol: 9}: {{Type: core.ActionShift, State: 10}},
		},
		gotos: map[genericConflictCell]core.StateID{{state: 1, symbol: 2}: 4},
	}
	compact, source := newGenericFreshnessSource(t, table)
	outputs, err := compact.ReduceOutputs(source, 9, 0, core.ForkOrder{})
	if err != nil || len(outputs) != 1 || outputs[0].Freshness != core.ReductionNew {
		t.Fatalf("prepopulate outputs=%+v err=%v", outputs, err)
	}
	scheduler := &diagnosticParserCoreGenericScheduler{
		compact: compact,
		headers: []diagnosticParserCoreHeader{{head: source, creationSeq: 3}, {head: outputs[0].Head, creationSeq: 7}},
		token:   Token{Symbol: 9, StartByte: 1, EndByte: 2},
		tokenSource: &dfaTokenSource{language: &Language{
			SymbolCount: 10,
			SymbolMetadata: []SymbolMetadata{
				8: {Visible: true, Named: true},
			},
		}},
		options: DiagnosticParserCorePrefixOptions{
			MaxDispatches:         20,
			materializationSource: []byte("a?"),
		},
		receipt: &DiagnosticParserCoreGenericScheduler{},
	}
	before, err := diagnosticParserCoreHeaderReceipts(compact, scheduler.headers)
	if err != nil {
		t.Fatal(err)
	}
	cell := mustDiagnosticParserCoreGenericCell(t, compact, 0, scheduler.headers[0], 9)
	if err := scheduler.applyGenericReduction(before, cell); err != nil {
		t.Fatal(err)
	}
	if !scheduler.headers[0].paused || scheduler.epochProgress || scheduler.work.ReductionPauses != 1 {
		t.Fatalf("unchanged reduction did not pause: headers=%+v work=%+v progress=%t", scheduler.headers, scheduler.work, scheduler.epochProgress)
	}
	if baseline, set := scheduler.headers[0].recoveryNodeBaseline(); !set || baseline != 1 {
		t.Fatalf("reduction pause baseline=%d/%t, want one visible shifted node", baseline, set)
	}
	if stop, err := scheduler.dispatchPass(); err != nil || stop != nil {
		t.Fatalf("sibling shift pass stop=%+v err=%v", stop, err)
	}
	if stop, err := scheduler.dispatchPass(); err != nil || stop != nil {
		t.Fatalf("paused drop pass stop=%+v err=%v", stop, err)
	}
	if len(scheduler.headers) != 1 || !scheduler.headers[0].shifted || scheduler.headers[0].creationSeq != 7 || len(scheduler.receipt.NoActionDrops) != 1 {
		t.Fatalf("finite paused/drop result headers=%+v drops=%+v", scheduler.headers, scheduler.receipt.NoActionDrops)
	}

	sole := &diagnosticParserCoreGenericScheduler{
		compact: compact, headers: []diagnosticParserCoreHeader{{head: source, paused: true}},
		token:   Token{Symbol: 9, StartByte: 1, EndByte: 2},
		options: DiagnosticParserCorePrefixOptions{MaxDispatches: 20}, receipt: &DiagnosticParserCoreGenericScheduler{},
	}
	stop, err := sole.dispatchPass()
	if err != nil || stop == nil || stop.boundary != DiagnosticParserCoreNoAction ||
		stop.detail != "generic scheduler has only paused heads for the elected token" || !sole.headers[0].paused {
		t.Fatalf("all-paused stop=%+v err=%v headers=%+v", stop, err, sole.headers)
	}
}

func TestDiagnosticParserCoreGenericReductionSequenceOverflowRollsBack(t *testing.T) {
	table := &genericConflictTable{
		cells: map[genericConflictCell][]core.Action{
			{state: 1, symbol: 8}: {{Type: core.ActionShift, State: 3}},
			{state: 2, symbol: 8}: {{Type: core.ActionShift, State: 3}},
			{state: 3, symbol: 9}: {{Type: core.ActionReduce, Symbol: 4, ChildCount: 1}},
		},
		gotos: map[genericConflictCell]core.StateID{
			{state: 1, symbol: 4}: 5,
			{state: 2, symbol: 4}: 6,
		},
	}
	compact, err := core.New(table, core.Limits{MaxDerivations: 8, MaxPopPaths: 8})
	if err != nil {
		t.Fatal(err)
	}
	first, _ := compact.Seed(1, 0)
	second, _ := compact.Seed(2, 0)
	if _, err = compact.Shift(first, 8, 0, core.Token{Symbol: 8, EndByte: 1}, core.ForkOrder{}); err != nil {
		t.Fatal(err)
	}
	head, err := compact.Shift(second, 8, 0, core.Token{Symbol: 8, EndByte: 1}, core.ForkOrder{})
	if err != nil {
		t.Fatal(err)
	}
	scheduler := &diagnosticParserCoreGenericScheduler{
		compact: compact, headers: []diagnosticParserCoreHeader{{head: head, creationSeq: 3}},
		token: Token{Symbol: 9, StartByte: 1, EndByte: 2}, nextSeq: math.MaxUint64,
		options: DiagnosticParserCorePrefixOptions{MaxDispatches: 20}, receipt: &DiagnosticParserCoreGenericScheduler{},
	}
	beforeStats, _ := compact.Stats(head)
	beforeHeaders := append([]diagnosticParserCoreHeader(nil), scheduler.headers...)
	before, _ := diagnosticParserCoreHeaderReceipts(compact, scheduler.headers)
	cell := mustDiagnosticParserCoreGenericCell(t, compact, 0, scheduler.headers[0], 9)
	err = scheduler.applyGenericReduction(before, cell)
	if err == nil {
		t.Fatal("overflowing reduction unexpectedly succeeded")
	}
	afterStats, _ := compact.Stats(head)
	if beforeStats != afterStats || !reflect.DeepEqual(scheduler.headers, beforeHeaders) || scheduler.nextSeq != math.MaxUint64 || scheduler.dispatches != 0 || scheduler.work != (DiagnosticParserCoreGenericWork{}) || len(scheduler.receipt.Rounds) != 0 {
		t.Fatalf("overflow rollback leaked: before=%+v after=%+v scheduler=%+v", beforeStats, afterStats, scheduler)
	}
	for _, state := range []core.StateID{5, 6} {
		if _, ok := compact.CanonicalBoundary(state, 1, false, 0); ok {
			t.Fatalf("overflow left canonical state %d", state)
		}
	}
}

func TestDiagnosticParserCoreRecoveryReductionForkPreservesMarkers(t *testing.T) {
	table := &genericConflictTable{
		cells: map[genericConflictCell][]core.Action{
			{state: 1, symbol: 8}: {{Type: core.ActionShift, State: 3}},
			{state: 2, symbol: 8}: {{Type: core.ActionShift, State: 3}},
			{state: 3, symbol: 9}: {{Type: core.ActionReduce, Symbol: 4, ChildCount: 1}},
		},
		gotos: map[genericConflictCell]core.StateID{
			{state: 1, symbol: 4}: 5,
			{state: 2, symbol: 4}: 6,
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
	if _, err = compact.Shift(first, 8, 0, core.Token{Symbol: 8, EndByte: 1}, core.ForkOrder{}); err != nil {
		t.Fatal(err)
	}
	head, err := compact.Shift(second, 8, 0, core.Token{Symbol: 8, EndByte: 1}, core.ForkOrder{})
	if err != nil {
		t.Fatal(err)
	}
	scheduler := &diagnosticParserCoreGenericScheduler{
		compact:              compact,
		headers:              []diagnosticParserCoreHeader{{head: head, creationSeq: 3}},
		token:                Token{Symbol: 9, StartByte: 1, EndByte: 2},
		nextSeq:              10,
		nextCleanPathLineage: 1,
		options: DiagnosticParserCorePrefixOptions{
			MaxDispatches:                        20,
			allowCompactRecoveryLineageSelection: true,
		},
		receipt: &DiagnosticParserCoreGenericScheduler{},
	}
	scheduler.headers[0].markRecoveryLineage()
	scheduler.recoveryIsolation = true
	before, err := diagnosticParserCoreHeaderReceipts(compact, scheduler.headers)
	if err != nil {
		t.Fatal(err)
	}
	cell := mustDiagnosticParserCoreGenericCell(t, compact, 0, scheduler.headers[0], 9)
	if err := scheduler.applyGenericReduction(before, cell); err != nil {
		t.Fatal(err)
	}
	if len(scheduler.headers) != 2 {
		t.Fatalf("reduction produced %d heads, want 2", len(scheduler.headers))
	}
	for index := range scheduler.headers {
		if !scheduler.headers[index].isRecoveryLineage() || !scheduler.headers[index].isRecoveryCosted() {
			t.Fatalf("reduction output %d lost recovery competition provenance", index)
		}
	}
	if !scheduler.recoveryIsolation {
		t.Fatal("ordinary reduction disabled recovery competition")
	}
	if !scheduler.competingRecoveryFrontier() {
		t.Fatal("ordinary reduction stopped the marked outputs from competing")
	}
	if scheduler.work.RecoveryAmbiguityForks != 1 {
		t.Fatalf("recovery ambiguity telemetry=%+v, want one reduction fork", scheduler.work)
	}
}

func TestDiagnosticParserCoreCanonicalizeRunnableDominatesPaused(t *testing.T) {
	compact, err := core.New(&genericConflictTable{}, core.Limits{})
	if err != nil {
		t.Fatal(err)
	}
	head, _ := compact.Seed(1, 0)
	headers, err := canonicalizeDiagnosticParserCoreHeaders(compact, []diagnosticParserCoreHeader{
		{head: head, creationSeq: 3, paused: true},
		{head: head, creationSeq: 7},
	})
	if err != nil || len(headers) != 1 || headers[0].paused || headers[0].creationSeq != 7 {
		t.Fatalf("canonical runnable dominance headers=%+v err=%v", headers, err)
	}
}

func TestDiagnosticParserCoreUpdatedReductionAdoptsActiveSibling(t *testing.T) {
	table := &genericConflictTable{
		cells: map[genericConflictCell][]core.Action{
			{state: 1, symbol: 7}: {{Type: core.ActionShift, State: 5}},
			{state: 1, symbol: 8}: {{Type: core.ActionShift, State: 3}},
			{state: 5, symbol: 9}: {{Type: core.ActionReduce, Symbol: 2, ChildCount: 1, DynamicPrecedence: 1}},
			{state: 3, symbol: 9}: {{Type: core.ActionReduce, Symbol: 2, ChildCount: 1, DynamicPrecedence: 2}},
		},
		gotos: map[genericConflictCell]core.StateID{{state: 1, symbol: 2}: 4},
	}
	compact, err := core.New(table, core.Limits{MaxDerivations: 8, MaxPopPaths: 8})
	if err != nil {
		t.Fatal(err)
	}
	seed, _ := compact.Seed(1, 0)
	low, _ := compact.Shift(seed, 7, 0, core.Token{Symbol: 7, EndByte: 1}, core.ForkOrder{})
	high, _ := compact.Shift(seed, 8, 0, core.Token{Symbol: 8, EndByte: 1}, core.ForkOrder{})
	lowOutputs, err := compact.ReduceOutputs(low, 9, 0, core.ForkOrder{})
	if err != nil || len(lowOutputs) != 1 || lowOutputs[0].Freshness != core.ReductionNew {
		t.Fatalf("low output=%+v err=%v", lowOutputs, err)
	}
	scheduler := &diagnosticParserCoreGenericScheduler{
		compact: compact,
		headers: []diagnosticParserCoreHeader{{head: high, creationSeq: 3}, {head: lowOutputs[0].Head, creationSeq: 11}},
		token:   Token{Symbol: 9, StartByte: 1, EndByte: 2}, nextSeq: math.MaxUint64,
		options: DiagnosticParserCorePrefixOptions{MaxDispatches: 20}, receipt: &DiagnosticParserCoreGenericScheduler{},
	}
	before, _ := diagnosticParserCoreHeaderReceipts(compact, scheduler.headers)
	cell := mustDiagnosticParserCoreGenericCell(t, compact, 0, scheduler.headers[0], 9)
	if err := scheduler.applyGenericReduction(before, cell); err != nil {
		t.Fatal(err)
	}
	if len(scheduler.headers) != 2 || !scheduler.headers[0].paused || scheduler.headers[1].paused || scheduler.headers[1].creationSeq != 11 || scheduler.nextSeq != math.MaxUint64 {
		t.Fatalf("updated sibling adoption headers=%+v nextSeq=%d", scheduler.headers, scheduler.nextSeq)
	}
	canonical, ok := compact.CanonicalBoundary(4, 1, false, 0)
	if !ok || scheduler.headers[1].head != canonical {
		t.Fatalf("active sibling head=%+v canonical=%+v ok=%t", scheduler.headers[1].head, canonical, ok)
	}
}

func TestDiagnosticParserCoreConflictUpdatedOutputPreservesSuffixIdentity(t *testing.T) {
	actions := []core.Action{
		{Type: core.ActionReduce, Symbol: 2, ChildCount: 1, DynamicPrecedence: 2},
		{Type: core.ActionShift, State: 6},
	}
	table := &genericConflictTable{
		cells: map[genericConflictCell][]core.Action{
			{state: 1, symbol: 7}: {{Type: core.ActionShift, State: 5}},
			{state: 1, symbol: 8}: {{Type: core.ActionShift, State: 3}},
			{state: 5, symbol: 9}: {{Type: core.ActionReduce, Symbol: 2, ChildCount: 1, DynamicPrecedence: 1}},
			{state: 3, symbol: 9}: actions,
		},
		gotos: map[genericConflictCell]core.StateID{{state: 1, symbol: 2}: 4},
	}
	compact, err := core.New(table, core.Limits{MaxDerivations: 8, MaxPopPaths: 8})
	if err != nil {
		t.Fatal(err)
	}
	seed, _ := compact.Seed(1, 0)
	low, _ := compact.Shift(seed, 7, 0, core.Token{Symbol: 7, EndByte: 1}, core.ForkOrder{})
	high, _ := compact.Shift(seed, 8, 0, core.Token{Symbol: 8, EndByte: 1}, core.ForkOrder{})
	lowOutputs, err := compact.ReduceOutputs(low, 9, 0, core.ForkOrder{})
	if err != nil || len(lowOutputs) != 1 {
		t.Fatalf("low output=%+v err=%v", lowOutputs, err)
	}
	scheduler := &diagnosticParserCoreGenericScheduler{
		compact: compact,
		headers: []diagnosticParserCoreHeader{{head: high, creationSeq: 3}, {head: lowOutputs[0].Head, creationSeq: 11}},
		token:   Token{Symbol: 9, StartByte: 1, EndByte: 2}, branchOrder: 7, nextSeq: 20,
		options: DiagnosticParserCorePrefixOptions{MaxDispatches: 20}, receipt: &DiagnosticParserCoreGenericScheduler{},
	}
	before, _ := diagnosticParserCoreHeaderReceipts(compact, scheduler.headers)
	cell := mustDiagnosticParserCoreGenericCell(t, compact, 0, scheduler.headers[0], 9)
	if err := scheduler.applyGenericConflict(before, cell); err != nil {
		t.Fatal(err)
	}
	receipts, _ := diagnosticParserCoreHeaderReceipts(compact, scheduler.headers)
	if len(receipts) != 2 || receipts[0].State != 4 || receipts[0].CreationSeq != 11 || receipts[0].Paused ||
		receipts[1].State != 6 || receipts[1].CreationSeq != 20 || !receipts[1].Shifted || scheduler.nextSeq != 21 {
		t.Fatalf("conflict suffix identity/order receipts=%+v nextSeq=%d", receipts, scheduler.nextSeq)
	}
}

func TestDiagnosticParserCoreConflictSecondaryUpdateAdoptsActiveSibling(t *testing.T) {
	for _, activeBefore := range []bool{true, false} {
		name := "active-after"
		if activeBefore {
			name = "active-before"
		}
		t.Run(name, func(t *testing.T) {
			actions := []core.Action{
				{Type: core.ActionShift, State: 6},
				{Type: core.ActionReduce, Symbol: 2, ChildCount: 1, DynamicPrecedence: 2},
			}
			table := &genericConflictTable{
				cells: map[genericConflictCell][]core.Action{
					{state: 1, symbol: 7}: {{Type: core.ActionShift, State: 5}},
					{state: 1, symbol: 8}: {{Type: core.ActionShift, State: 3}},
					{state: 5, symbol: 9}: {{Type: core.ActionReduce, Symbol: 2, ChildCount: 1, DynamicPrecedence: 1}},
					{state: 3, symbol: 9}: actions,
				},
				gotos: map[genericConflictCell]core.StateID{{state: 1, symbol: 2}: 4},
			}
			compact, err := core.New(table, core.Limits{MaxDerivations: 8, MaxPopPaths: 8})
			if err != nil {
				t.Fatal(err)
			}
			seed, _ := compact.Seed(1, 0)
			low, _ := compact.Shift(seed, 7, 0, core.Token{Symbol: 7, EndByte: 1}, core.ForkOrder{})
			high, _ := compact.Shift(seed, 8, 0, core.Token{Symbol: 8, EndByte: 1}, core.ForkOrder{})
			lowOutputs, err := compact.ReduceOutputs(low, 9, 0, core.ForkOrder{})
			if err != nil || len(lowOutputs) != 1 {
				t.Fatalf("low output=%+v err=%v", lowOutputs, err)
			}
			active := diagnosticParserCoreHeader{head: lowOutputs[0].Head, creationSeq: 11}
			source := diagnosticParserCoreHeader{head: high, creationSeq: 3}
			headers := []diagnosticParserCoreHeader{source, active}
			if activeBefore {
				headers = []diagnosticParserCoreHeader{active, source}
			}
			sourceIndex := 0
			if activeBefore {
				sourceIndex = 1
			}
			scheduler := &diagnosticParserCoreGenericScheduler{
				compact: compact, headers: headers,
				token: Token{Symbol: 9, StartByte: 1, EndByte: 2}, branchOrder: 7, nextSeq: math.MaxUint64,
				options: DiagnosticParserCorePrefixOptions{MaxDispatches: 20}, receipt: &DiagnosticParserCoreGenericScheduler{},
			}
			before, _ := diagnosticParserCoreHeaderReceipts(compact, scheduler.headers)
			cell := mustDiagnosticParserCoreGenericCell(t, compact, sourceIndex, scheduler.headers[sourceIndex], 9)
			if err := scheduler.applyGenericConflict(before, cell); err != nil {
				t.Fatal(err)
			}
			receipts, _ := diagnosticParserCoreHeaderReceipts(compact, scheduler.headers)
			wantStates := []StateID{6, 4}
			wantSeq := []uint64{3, 11}
			if activeBefore {
				wantStates, wantSeq = []StateID{4, 6}, []uint64{11, 3}
			}
			gotStates := []StateID{receipts[0].State, receipts[1].State}
			gotSeq := []uint64{receipts[0].CreationSeq, receipts[1].CreationSeq}
			if !reflect.DeepEqual(gotStates, wantStates) || !reflect.DeepEqual(gotSeq, wantSeq) || scheduler.nextSeq != math.MaxUint64 || scheduler.branchOrder != 8 {
				t.Fatalf("secondary adoption states=%v seq=%v order=%d next=%d", gotStates, gotSeq, scheduler.branchOrder, scheduler.nextSeq)
			}
			arm := scheduler.receipt.Conflicts[0].SecondaryArms[0]
			if !arm.Adopted || arm.Paused || len(arm.Outputs) != 0 || arm.BranchOrder != 8 {
				t.Fatalf("secondary adoption receipt=%+v", arm)
			}
			for _, header := range scheduler.headers {
				if header.freshness != 0 {
					t.Fatalf("successful conflict retained freshness: %+v", scheduler.headers)
				}
			}
		})
	}
}

func TestDiagnosticParserCoreConflictAllUnchangedPauses(t *testing.T) {
	actions := []core.Action{
		{Type: core.ActionReduce, Symbol: 2, ChildCount: 1},
		{Type: core.ActionReduce, Symbol: 2, ChildCount: 1},
	}
	table := &genericConflictTable{
		cells: map[genericConflictCell][]core.Action{
			{state: 1, symbol: 8}: {{Type: core.ActionShift, State: 3}},
			{state: 3, symbol: 9}: actions,
		},
		gotos: map[genericConflictCell]core.StateID{{state: 1, symbol: 2}: 4},
	}
	compact, source := newGenericFreshnessSource(t, table)
	if _, err := compact.ReduceOutputs(source, 9, 0, core.ForkOrder{}); err != nil {
		t.Fatal(err)
	}
	scheduler := &diagnosticParserCoreGenericScheduler{
		compact: compact, headers: []diagnosticParserCoreHeader{{head: source, creationSeq: 4}},
		token: Token{Symbol: 9, StartByte: 1, EndByte: 2}, branchOrder: 7, nextSeq: math.MaxUint64,
		options: DiagnosticParserCorePrefixOptions{MaxDispatches: 20}, receipt: &DiagnosticParserCoreGenericScheduler{},
	}
	before, _ := diagnosticParserCoreHeaderReceipts(compact, scheduler.headers)
	cell := mustDiagnosticParserCoreGenericCell(t, compact, 0, scheduler.headers[0], 9)
	if err := scheduler.applyGenericConflict(before, cell); err != nil {
		t.Fatal(err)
	}
	if len(scheduler.headers) != 1 || !scheduler.headers[0].paused || scheduler.headers[0].freshness != 0 || scheduler.branchOrder != 8 || scheduler.nextSeq != math.MaxUint64 || scheduler.epochProgress {
		t.Fatalf("all-unchanged conflict scheduler=%+v", scheduler)
	}
	conflict := scheduler.receipt.Conflicts[0]
	if !conflict.PrimaryPaused || conflict.PrimaryAdopted || len(conflict.SecondaryArms) != 1 || !conflict.SecondaryArms[0].Paused || conflict.SecondaryArms[0].Adopted {
		t.Fatalf("all-unchanged conflict receipt=%+v", conflict)
	}
	stop, err := scheduler.dispatchPass()
	if err != nil || stop == nil || stop.boundary != DiagnosticParserCoreNoAction ||
		stop.detail != "generic scheduler has only paused heads for the elected token" {
		t.Fatalf("all-unchanged conflict stop=%+v err=%v", stop, err)
	}
}

func TestDiagnosticParserCoreConflictPostExecutionFailureRollsBack(t *testing.T) {
	actions := []core.Action{{Type: core.ActionShift, State: 2}, {Type: core.ActionShift, State: 3}}
	compact, err := core.New(&genericConflictTable{actions: actions}, core.Limits{})
	if err != nil {
		t.Fatal(err)
	}
	source, _ := compact.Seed(1, 0)
	scheduler := &diagnosticParserCoreGenericScheduler{
		compact: compact, headers: []diagnosticParserCoreHeader{{head: source, creationSeq: 4}},
		token: Token{Symbol: 9, EndByte: 1}, branchOrder: 7, nextSeq: 10, nextCleanPathLineage: 11,
		options: DiagnosticParserCorePrefixOptions{MaxDispatches: 20}, receipt: &DiagnosticParserCoreGenericScheduler{},
		conflictPostExecutionFault: func() error { return errors.New("post-execution fault") },
	}
	beforeStats, _ := compact.Stats(source)
	beforeHeaders := append([]diagnosticParserCoreHeader(nil), scheduler.headers...)
	before, _ := diagnosticParserCoreHeaderReceipts(compact, scheduler.headers)
	cell := mustDiagnosticParserCoreGenericCell(t, compact, 0, scheduler.headers[0], 9)
	err = scheduler.applyGenericConflict(before, cell)
	if err == nil {
		t.Fatal("post-execution fault unexpectedly succeeded")
	}
	afterStats, _ := compact.Stats(source)
	if beforeStats != afterStats || !reflect.DeepEqual(scheduler.headers, beforeHeaders) || scheduler.branchOrder != 7 || scheduler.nextSeq != 10 || scheduler.nextCleanPathLineage != 11 || scheduler.dispatches != 0 || scheduler.work != (DiagnosticParserCoreGenericWork{}) || !reflect.DeepEqual(scheduler.receipt, &DiagnosticParserCoreGenericScheduler{}) {
		t.Fatalf("post-execution rollback leaked: before=%+v after=%+v scheduler=%+v", beforeStats, afterStats, scheduler)
	}
	for _, state := range []core.StateID{2, 3} {
		if _, ok := compact.CanonicalBoundary(state, 1, true, 0); ok {
			t.Fatalf("post-execution rollback left shifted state %d", state)
		}
	}
}

func TestDiagnosticParserCoreSummaryConflictFailureRollsBack(t *testing.T) {
	actions := []core.Action{{Type: core.ActionShift, State: 2}, {Type: core.ActionShift, State: 3}}
	compact, err := core.New(&genericConflictTable{actions: actions}, core.Limits{})
	if err != nil {
		t.Fatal(err)
	}
	source, err := compact.Seed(1, 0)
	if err != nil {
		t.Fatal(err)
	}
	scheduler := &diagnosticParserCoreGenericScheduler{
		compact: compact, headers: []diagnosticParserCoreHeader{{head: source, creationSeq: 4}},
		token: Token{Symbol: 9, EndByte: 1}, branchOrder: 7, nextSeq: 10,
		options:                    DiagnosticParserCorePrefixOptions{ReceiptMode: DiagnosticParserCoreReceiptSummary, MaxDispatches: 20},
		receipt:                    &DiagnosticParserCoreGenericScheduler{ReceiptMode: DiagnosticParserCoreReceiptSummary},
		conflictPostExecutionFault: func() error { return errors.New("summary post-execution fault") },
	}
	beforeStats, err := compact.Stats(source)
	if err != nil {
		t.Fatal(err)
	}
	beforeCoreWork := compact.Work()
	beforeHeaders := append([]diagnosticParserCoreHeader(nil), scheduler.headers...)
	before, err := scheduler.headerReceipts(scheduler.headers)
	if err != nil {
		t.Fatal(err)
	}
	cell := mustDiagnosticParserCoreGenericCell(t, compact, 0, scheduler.headers[0], 9)
	err = scheduler.applyGenericConflict(before, cell)
	if err == nil {
		t.Fatal("summary post-execution fault unexpectedly succeeded")
	}
	afterStats, statsErr := compact.Stats(source)
	if statsErr != nil {
		t.Fatal(statsErr)
	}
	wantReceipt := &DiagnosticParserCoreGenericScheduler{ReceiptMode: DiagnosticParserCoreReceiptSummary}
	if beforeStats != afterStats || compact.Work() != beforeCoreWork || !reflect.DeepEqual(scheduler.headers, beforeHeaders) || scheduler.branchOrder != 7 || scheduler.nextSeq != 10 || scheduler.dispatches != 0 || scheduler.work != (DiagnosticParserCoreGenericWork{}) || !reflect.DeepEqual(scheduler.receipt, wantReceipt) {
		t.Fatalf("summary rollback leaked: before=%+v after=%+v scheduler=%+v", beforeStats, afterStats, scheduler)
	}
	for _, state := range []core.StateID{2, 3} {
		if _, ok := compact.CanonicalBoundary(state, 1, true, 0); ok {
			t.Fatalf("summary rollback left shifted state %d", state)
		}
	}
}

func TestDiagnosticParserCoreConflictFiltersUnchangedArm(t *testing.T) {
	actions := []core.Action{
		{Type: core.ActionShift, State: 6},
		{Type: core.ActionReduce, Symbol: 2, ChildCount: 1},
	}
	table := &genericConflictTable{
		cells: map[genericConflictCell][]core.Action{
			{state: 1, symbol: 8}: {{Type: core.ActionShift, State: 3}},
			{state: 3, symbol: 9}: actions,
		},
		gotos: map[genericConflictCell]core.StateID{{state: 1, symbol: 2}: 4},
	}
	compact, source := newGenericFreshnessSource(t, table)
	if outputs, err := compact.ReduceOutputs(source, 9, 1, core.ForkOrder{Present: true, Value: 7}); err != nil || len(outputs) != 1 {
		t.Fatalf("prepopulate conflict reduction outputs=%+v err=%v", outputs, err)
	}
	scheduler := &diagnosticParserCoreGenericScheduler{
		compact: compact, headers: []diagnosticParserCoreHeader{{head: source, creationSeq: 4}},
		token: Token{Symbol: 9, StartByte: 1, EndByte: 2}, branchOrder: 7, nextSeq: math.MaxUint64,
		options: DiagnosticParserCorePrefixOptions{MaxDispatches: 20}, receipt: &DiagnosticParserCoreGenericScheduler{},
	}
	before, _ := diagnosticParserCoreHeaderReceipts(compact, scheduler.headers)
	cell := mustDiagnosticParserCoreGenericCell(t, compact, 0, scheduler.headers[0], 9)
	if err := scheduler.applyGenericConflict(before, cell); err != nil {
		t.Fatal(err)
	}
	receipts, _ := diagnosticParserCoreHeaderReceipts(compact, scheduler.headers)
	if len(receipts) != 1 || receipts[0].State != 6 || receipts[0].CreationSeq != 4 || !receipts[0].Shifted ||
		scheduler.branchOrder != 8 || scheduler.nextSeq != math.MaxUint64 {
		t.Fatalf("filtered conflict headers=%+v order=%d seq=%d", receipts, scheduler.branchOrder, scheduler.nextSeq)
	}
	conflict := scheduler.receipt.Conflicts[0]
	if conflict.PrimaryPaused || len(conflict.SecondaryArms) != 1 || !conflict.SecondaryArms[0].Paused || conflict.SecondaryArms[0].BranchOrder != 8 || len(conflict.SecondaryArms[0].Outputs) != 0 ||
		len(conflict.Round.Actions) != 2 || conflict.Round.Actions[0].Ordinal != 1 || conflict.Round.Actions[0].BranchOrder != 8 || conflict.Round.Actions[1].Ordinal != 0 {
		t.Fatalf("filtered conflict receipt=%+v", conflict)
	}

	beforeGraph, _ := compact.Stats(scheduler.headers[0].head)
	rollback := &diagnosticParserCoreGenericScheduler{
		compact: compact, headers: []diagnosticParserCoreHeader{{head: source, creationSeq: 4}},
		token: Token{Symbol: 9, StartByte: 1, EndByte: 2}, branchOrder: 7, nextSeq: 10,
		options: DiagnosticParserCorePrefixOptions{MaxDispatches: 0}, receipt: &DiagnosticParserCoreGenericScheduler{},
	}
	rollbackBefore, _ := diagnosticParserCoreHeaderReceipts(compact, rollback.headers)
	if err := compact.ApplyAtomic(func() error {
		cell := mustDiagnosticParserCoreGenericCell(t, compact, 0, rollback.headers[0], 9)
		return rollback.applyGenericConflict(rollbackBefore, cell)
	}); err == nil {
		t.Fatal("capped filtered conflict unexpectedly succeeded")
	}
	afterGraph, _ := compact.Stats(scheduler.headers[0].head)
	if beforeGraph != afterGraph || rollback.branchOrder != 7 || rollback.nextSeq != 10 || !reflect.DeepEqual(rollback.receipt, &DiagnosticParserCoreGenericScheduler{}) {
		t.Fatalf("filtered conflict rollback leaked: before=%+v after=%+v scheduler=%+v", beforeGraph, afterGraph, rollback)
	}
}
