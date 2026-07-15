//go:build gts_parsercorephase0

package gotreesitter

import (
	"errors"
	"reflect"
	"strings"
	"testing"

	core "github.com/odvcencio/gotreesitter/internal/parsercorephase0"
)

type diagnosticParserCoreDispatchProbeTable struct {
	base      *genericConflictTable
	scheduler *diagnosticParserCoreGenericScheduler
	reenter   bool
	panicOnce bool
}

func (table *diagnosticParserCoreDispatchProbeTable) Actions(state core.StateID, symbol core.Symbol) (core.ActionRow, error) {
	if table.panicOnce {
		table.panicOnce = false
		panic("dispatch classification panic")
	}
	if table.reenter {
		table.reenter = false
		_, err := table.scheduler.dispatchPass()
		if err != nil {
			return core.ActionRow{}, err
		}
	}
	return table.base.Actions(state, symbol)
}

func (table *diagnosticParserCoreDispatchProbeTable) Goto(state core.StateID, symbol core.Symbol) (core.StateID, error) {
	return table.base.Goto(state, symbol)
}

func (table *diagnosticParserCoreDispatchProbeTable) ProductionFields(productionID uint16, childCount int) ([]core.FieldMapEntry, error) {
	return table.base.ProductionFields(productionID, childCount)
}

func (table *diagnosticParserCoreDispatchProbeTable) ProductionAliases(productionID uint16, childCount int) ([]core.Symbol, error) {
	return table.base.ProductionAliases(productionID, childCount)
}

func newDiagnosticParserCoreDispatchProbeScheduler(t *testing.T, table core.TableView) (*diagnosticParserCoreGenericScheduler, core.Head) {
	t.Helper()
	compact, err := core.New(table, core.Limits{MaxDerivations: 8, MaxPopPaths: 8})
	if err != nil {
		t.Fatal(err)
	}
	head, err := compact.Seed(1, 0)
	if err != nil {
		t.Fatal(err)
	}
	return &diagnosticParserCoreGenericScheduler{
		compact: compact, headers: []diagnosticParserCoreHeader{{head: head, creationSeq: 1}},
		token:   Token{Symbol: 9, EndByte: 1},
		options: DiagnosticParserCorePrefixOptions{MaxDispatches: 20},
		receipt: &DiagnosticParserCoreGenericScheduler{},
	}, head
}

func assertDiagnosticParserCoreDispatchScratchClean(t *testing.T, scheduler *diagnosticParserCoreGenericScheduler) {
	t.Helper()
	scratch := &scheduler.dispatchScratch
	if scratch.busy || len(scratch.cells) != 0 || len(scratch.noActionIndices) != 0 {
		t.Fatalf("dispatch scratch retained logical state: %+v", scratch)
	}
	for index, cell := range scratch.cells[:cap(scratch.cells)] {
		if cell.actions.Len() != 0 {
			t.Fatalf("dispatch scratch cell %d retained an action row", index)
		}
	}
}

func TestDiagnosticParserCoreDispatchScratchRejectsReentryBeforeCountersAndRetries(t *testing.T) {
	base := &genericConflictTable{actions: []core.Action{{Type: core.ActionShift, State: 2}}}
	table := &diagnosticParserCoreDispatchProbeTable{base: base, reenter: true}
	scheduler, _ := newDiagnosticParserCoreDispatchProbeScheduler(t, table)
	table.scheduler = scheduler

	if _, err := scheduler.dispatchPass(); err == nil || !strings.Contains(err.Error(), "reentrant generic scheduler dispatch") {
		t.Fatalf("reentrant dispatch error=%v", err)
	}
	if scheduler.work.Passes != 1 || scheduler.work.ActionLookups != 0 || scheduler.dispatches != 0 {
		t.Fatalf("reentrant dispatch crossed inner counters: work=%+v dispatches=%d", scheduler.work, scheduler.dispatches)
	}
	assertDiagnosticParserCoreDispatchScratchClean(t, scheduler)

	stop, err := scheduler.dispatchPass()
	if err != nil || stop != nil || len(scheduler.headers) != 1 || !scheduler.headers[0].shifted {
		t.Fatalf("retry stop=%+v err=%v headers=%+v", stop, err, scheduler.headers)
	}
	assertDiagnosticParserCoreDispatchScratchClean(t, scheduler)
}

func TestDiagnosticParserCoreDispatchScratchCleansAfterPanicAndRetries(t *testing.T) {
	base := &genericConflictTable{actions: []core.Action{{Type: core.ActionShift, State: 2}}}
	table := &diagnosticParserCoreDispatchProbeTable{base: base, panicOnce: true}
	scheduler, _ := newDiagnosticParserCoreDispatchProbeScheduler(t, table)
	table.scheduler = scheduler

	func() {
		defer func() {
			if recovered := recover(); recovered != "dispatch classification panic" {
				t.Fatalf("panic=%v", recovered)
			}
		}()
		_, _ = scheduler.dispatchPass()
	}()
	assertDiagnosticParserCoreDispatchScratchClean(t, scheduler)

	stop, err := scheduler.dispatchPass()
	if err != nil || stop != nil || len(scheduler.headers) != 1 || !scheduler.headers[0].shifted {
		t.Fatalf("retry stop=%+v err=%v headers=%+v", stop, err, scheduler.headers)
	}
	assertDiagnosticParserCoreDispatchScratchClean(t, scheduler)
}

func TestDiagnosticParserCoreDispatchScratchDoesNotAliasFullReceipts(t *testing.T) {
	table := &genericConflictTable{cells: map[genericConflictCell][]core.Action{
		{state: 1, symbol: 9}:  {{Type: core.ActionShift, State: 2}},
		{state: 2, symbol: 10}: {{Type: core.ActionShift, State: 3}},
	}}
	scheduler, _ := newDiagnosticParserCoreDispatchProbeScheduler(t, table)

	if stop, err := scheduler.dispatchPass(); err != nil || stop != nil {
		t.Fatalf("first dispatch stop=%+v err=%v", stop, err)
	}
	if len(scheduler.receipt.Rounds) != 1 {
		t.Fatalf("first dispatch rounds=%+v", scheduler.receipt.Rounds)
	}
	firstBefore := append([]DiagnosticParserCoreHeaderReceipt(nil), scheduler.receipt.Rounds[0].Before...)
	firstActions := append([]DiagnosticParserCoreRoundAction(nil), scheduler.receipt.Rounds[0].Actions...)
	firstAfter := append([]DiagnosticParserCoreHeaderReceipt(nil), scheduler.receipt.Rounds[0].After...)
	assertDiagnosticParserCoreDispatchScratchClean(t, scheduler)

	scheduler.headers[0].shifted = false
	scheduler.token = Token{Symbol: 10, StartByte: 1, EndByte: 2}
	if stop, err := scheduler.dispatchPass(); err != nil || stop != nil {
		t.Fatalf("second dispatch stop=%+v err=%v", stop, err)
	}
	if len(scheduler.receipt.Rounds) != 2 || !reflect.DeepEqual(scheduler.receipt.Rounds[0].Before, firstBefore) || !reflect.DeepEqual(scheduler.receipt.Rounds[0].Actions, firstActions) || !reflect.DeepEqual(scheduler.receipt.Rounds[0].After, firstAfter) {
		t.Fatalf("second dispatch mutated first full receipt: %+v", scheduler.receipt.Rounds)
	}
	assertDiagnosticParserCoreDispatchScratchClean(t, scheduler)
}

func TestDiagnosticParserCoreDispatchScratchCleansNoActionReturn(t *testing.T) {
	scheduler, _ := newDiagnosticParserCoreDispatchProbeScheduler(t, &genericConflictTable{})
	stop, err := scheduler.dispatchPass()
	if err != nil || stop == nil || stop.boundary != DiagnosticParserCoreNoAction || stop.headerIndex != 0 {
		t.Fatalf("no-action stop=%+v err=%v", stop, err)
	}
	if cap(scheduler.dispatchScratch.noActionIndices) == 0 {
		t.Fatal("no-action classification capacity was not retained")
	}
	assertDiagnosticParserCoreDispatchScratchClean(t, scheduler)
}

func TestDiagnosticParserCoreDispatchScratchCleansAfterConflictRollback(t *testing.T) {
	actions := []core.Action{{Type: core.ActionShift, State: 2}, {Type: core.ActionShift, State: 3}}
	table := &genericConflictTable{actions: actions}
	scheduler, head := newDiagnosticParserCoreDispatchProbeScheduler(t, table)
	scheduler.branchOrder = 7
	scheduler.nextSeq = 10
	scheduler.conflictPostExecutionFault = func() error { return errors.New("post-execution fault") }
	beforeStats, err := scheduler.compact.Stats(head)
	if err != nil {
		t.Fatal(err)
	}
	beforeHeaders := append([]diagnosticParserCoreHeader(nil), scheduler.headers...)

	if _, err := scheduler.dispatchPass(); err == nil || !strings.Contains(err.Error(), "post-execution fault") {
		t.Fatalf("conflict fault error=%v", err)
	}
	afterStats, err := scheduler.compact.Stats(head)
	if err != nil {
		t.Fatal(err)
	}
	if beforeStats != afterStats || !reflect.DeepEqual(scheduler.headers, beforeHeaders) || scheduler.branchOrder != 7 || scheduler.nextSeq != 10 || scheduler.dispatches != 0 || len(scheduler.receipt.Rounds) != 0 || len(scheduler.receipt.Conflicts) != 0 {
		t.Fatalf("conflict rollback leaked: before=%+v after=%+v scheduler=%+v", beforeStats, afterStats, scheduler)
	}
	assertDiagnosticParserCoreDispatchScratchClean(t, scheduler)

	scheduler.conflictPostExecutionFault = nil
	stop, err := scheduler.dispatchPass()
	if err != nil || stop != nil || len(scheduler.headers) != 2 {
		t.Fatalf("conflict retry stop=%+v err=%v headers=%+v", stop, err, scheduler.headers)
	}
	assertDiagnosticParserCoreDispatchScratchClean(t, scheduler)
}

func TestDiagnosticParserCoreDispatchScratchSteadyStateDoesNotAllocate(t *testing.T) {
	row := core.NewActionRow([]core.Action{{Type: core.ActionShift, State: 2}})
	scratch := diagnosticParserCoreDispatchScratch{
		cells: make([]diagnosticParserCoreGenericCell, 0, 2), noActionIndices: make([]int, 0, 2),
	}
	var runErr error
	if allocs := testing.AllocsPerRun(1000, func() {
		runErr = scratch.begin()
		scratch.cells = append(scratch.cells, diagnosticParserCoreGenericCell{headerIndex: 1, actions: row})
		scratch.noActionIndices = append(scratch.noActionIndices, 0)
		scratch.finish()
	}); allocs != 0 || runErr != nil {
		t.Fatalf("steady dispatch scratch allocs=%v err=%v", allocs, runErr)
	}
	for index, cell := range scratch.cells[:cap(scratch.cells)] {
		if cell.actions.Len() != 0 {
			t.Fatalf("finished cell %d retained an action row", index)
		}
	}
}
