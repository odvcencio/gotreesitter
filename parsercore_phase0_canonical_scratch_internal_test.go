//go:build gts_parsercorephase0

package gotreesitter

import (
	"reflect"
	"testing"

	core "github.com/odvcencio/gotreesitter/internal/parsercorephase0"
)

func newDiagnosticParserCoreCanonicalTestCore(t *testing.T) (*core.Core, core.Head, core.Head) {
	t.Helper()
	compact, err := core.New(&genericConflictTable{}, core.Limits{})
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
	return compact, first, second
}

func TestDiagnosticParserCoreCanonicalScratchPreservesLaterWinnerOrder(t *testing.T) {
	compact, first, second := newDiagnosticParserCoreCanonicalTestCore(t)
	var scratch diagnosticParserCoreCanonicalScratch
	out, err := scratch.canonicalize(compact, []diagnosticParserCoreHeader{
		{head: first, creationSeq: 3, freshness: core.ReductionNew},
		{head: second, creationSeq: 5},
		{head: first, creationSeq: 7},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(out) != 2 || out[0].head != second || out[0].creationSeq != 5 || out[1].head != first || out[1].creationSeq != 7 {
		t.Fatalf("later-winner order drifted: %+v", out)
	}
}

func TestDiagnosticParserCoreCanonicalScratchFreshRunnableDominance(t *testing.T) {
	compact, head, _ := newDiagnosticParserCoreCanonicalTestCore(t)
	var scratch diagnosticParserCoreCanonicalScratch
	out, err := scratch.canonicalize(compact, []diagnosticParserCoreHeader{
		{head: head, creationSeq: 3, paused: true, freshness: core.ReductionNew},
		{head: head, creationSeq: 5, paused: true},
		{head: head, creationSeq: 7, freshness: core.ReductionUpdated},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(out) != 1 || out[0].creationSeq != 5 || out[0].paused || out[0].freshness != 0 {
		t.Fatalf("fresh/runnable dominance drifted: %+v", out)
	}
}

func TestDiagnosticParserCoreCanonicalScratchCollapsesCanonicalHeads(t *testing.T) {
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
	if err != nil || len(lowOutputs) != 1 {
		t.Fatalf("low reduction outputs=%+v err=%v", lowOutputs, err)
	}
	stale := lowOutputs[0].Head
	highOutputs, err := compact.ReduceOutputs(high, 9, 0, core.ForkOrder{})
	if err != nil || len(highOutputs) != 1 || highOutputs[0].Head == stale {
		t.Fatalf("high reduction outputs=%+v stale=%+v err=%v", highOutputs, stale, err)
	}
	canonical := highOutputs[0].Head
	var scratch diagnosticParserCoreCanonicalScratch
	out, err := scratch.canonicalize(compact, []diagnosticParserCoreHeader{
		{head: stale, creationSeq: 3},
		{head: canonical, creationSeq: 5},
	})
	if err != nil || len(out) != 1 || out[0].head != canonical || out[0].creationSeq != 3 {
		t.Fatalf("canonical collapse output=%+v canonical=%+v err=%v", out, canonical, err)
	}
}

func TestDiagnosticParserCoreCanonicalScratchDoesNotAliasPublishedOutput(t *testing.T) {
	compact, firstHead, secondHead := newDiagnosticParserCoreCanonicalTestCore(t)
	var scratch diagnosticParserCoreCanonicalScratch
	first, err := scratch.canonicalize(compact, []diagnosticParserCoreHeader{
		{head: firstHead, creationSeq: 3},
		{head: secondHead, creationSeq: 5},
	})
	if err != nil {
		t.Fatal(err)
	}
	firstSnapshot := append([]diagnosticParserCoreHeader(nil), first...)
	// Simulate an enclosing scheduler rollback restoring the current headers
	// without restoring the scratch selector. The scratch must still discover
	// that its nominal target aliases the input prefix and use the other buffer.
	scratch.nextBuffer = 0
	second, err := scratch.canonicalize(compact, first[:1])
	if err != nil {
		t.Fatal(err)
	}
	if &first[0] == &second[0] {
		t.Fatal("successive canonical outputs share backing storage")
	}
	second[0].creationSeq = 11
	if !reflect.DeepEqual(first, firstSnapshot) {
		t.Fatalf("next output mutated prior output: got=%+v want=%+v", first, firstSnapshot)
	}
	secondSnapshot := append([]diagnosticParserCoreHeader(nil), second...)
	nextBeforeFailure := scratch.nextBuffer
	if _, err := scratch.canonicalize(compact, []diagnosticParserCoreHeader{
		{head: firstHead, creationSeq: 13},
		{head: core.Head{Node: 1 << 30}, creationSeq: 17},
	}); err == nil {
		t.Fatal("invalid head unexpectedly canonicalized")
	}
	if scratch.nextBuffer != nextBeforeFailure || !reflect.DeepEqual(second, secondSnapshot) {
		t.Fatalf("failed run published scratch: next=%d want=%d current=%+v want=%+v", scratch.nextBuffer, nextBeforeFailure, second, secondSnapshot)
	}
}

func TestDiagnosticParserCoreCanonicalScratchSteadyStateDoesNotAllocate(t *testing.T) {
	compact, first, second := newDiagnosticParserCoreCanonicalTestCore(t)
	input := []diagnosticParserCoreHeader{{head: first, creationSeq: 3}, {head: second, creationSeq: 5}}
	var scratch diagnosticParserCoreCanonicalScratch
	for range 4 {
		if _, err := scratch.canonicalize(compact, input); err != nil {
			t.Fatal(err)
		}
	}
	var runErr error
	if allocs := testing.AllocsPerRun(1000, func() {
		_, runErr = scratch.canonicalize(compact, input)
	}); allocs != 0 || runErr != nil {
		t.Fatalf("steady canonicalization allocs=%v err=%v", allocs, runErr)
	}
}
