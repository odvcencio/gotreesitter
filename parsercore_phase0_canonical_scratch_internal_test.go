//go:build gts_parsercorephase0

package gotreesitter

import (
	"errors"
	"fmt"
	"reflect"
	"runtime"
	"testing"
	"unsafe"

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

// TestDiagnosticParserCoreCanonicalScratchSingleHeaderRemapsAndPublishesFresh
// exercises the compact scheduler's single-header shortcut. It keeps the
// canonical boundary remap, clears reduction freshness, and preserves the
// double-buffer publication rule.
func TestDiagnosticParserCoreCanonicalScratchSingleHeaderRemapsAndPublishesFresh(t *testing.T) {
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
	seed, err := compact.Seed(1, 0)
	if err != nil {
		t.Fatal(err)
	}
	low, err := compact.Shift(seed, 7, 0, core.Token{Symbol: 7, EndByte: 1}, core.ForkOrder{})
	if err != nil {
		t.Fatal(err)
	}
	high, err := compact.Shift(seed, 8, 0, core.Token{Symbol: 8, EndByte: 1}, core.ForkOrder{})
	if err != nil {
		t.Fatal(err)
	}
	lowOutputs, err := compact.ReduceOutputs(low, 9, 0, core.ForkOrder{})
	if err != nil || len(lowOutputs) != 1 {
		t.Fatalf("low reduction outputs=%+v err=%v", lowOutputs, err)
	}
	highOutputs, err := compact.ReduceOutputs(high, 9, 0, core.ForkOrder{})
	if err != nil || len(highOutputs) != 1 || highOutputs[0].Head == lowOutputs[0].Head {
		t.Fatalf("high reduction outputs=%+v low=%+v err=%v", highOutputs, lowOutputs, err)
	}
	stale := lowOutputs[0].Head
	canonical := highOutputs[0].Head
	staleState, staleByte, err := compact.Boundary(stale)
	if err != nil {
		t.Fatal(err)
	}
	canonicalState, canonicalByte, err := compact.Boundary(canonical)
	if err != nil {
		t.Fatal(err)
	}
	if staleState != canonicalState || staleByte != canonicalByte {
		t.Fatalf("fixture does not share a canonical boundary: stale=%d/%d canonical=%d/%d", staleState, staleByte, canonicalState, canonicalByte)
	}

	input := []diagnosticParserCoreHeader{{
		head: stale, creationSeq: 3, freshness: core.ReductionNew,
	}}
	var scratch diagnosticParserCoreCanonicalScratch
	first, err := scratch.canonicalize(compact, input)
	if err != nil {
		t.Fatal(err)
	}
	if len(first) != 1 || first[0].head != canonical || first[0].freshness != 0 {
		t.Fatalf("single-header remap/freshness=%+v canonical=%+v", first, canonical)
	}
	if gotState, gotByte, boundaryErr := compact.Boundary(first[0].head); boundaryErr != nil || gotState != staleState || gotByte != staleByte {
		t.Fatalf("remapped boundary=%d/%d err=%v, want=%d/%d", gotState, gotByte, boundaryErr, staleState, staleByte)
	}
	if &first[0] == &input[0] {
		t.Fatal("single-header canonicalization aliased caller input")
	}
	firstSnapshot := append([]diagnosticParserCoreHeader(nil), first...)
	second, err := scratch.canonicalize(compact, first)
	if err != nil {
		t.Fatal(err)
	}
	if len(second) != 1 || &second[0] == &first[0] {
		t.Fatal("single-header canonicalization did not alternate published buffers")
	}
	second[0].creationSeq = 11
	if !reflect.DeepEqual(first, firstSnapshot) {
		t.Fatalf("next single-header output mutated prior publication: got=%+v want=%+v", first, firstSnapshot)
	}
}

// TestDiagnosticParserCoreCanonicalScratchSingleHeaderFailureDoesNotPublish
// keeps an invalid head from advancing the publication buffer or mutating the
// last valid single-header result.
func TestDiagnosticParserCoreCanonicalScratchSingleHeaderFailureDoesNotPublish(t *testing.T) {
	compact, head, _ := newDiagnosticParserCoreCanonicalTestCore(t)
	var scratch diagnosticParserCoreCanonicalScratch
	published, err := scratch.canonicalize(compact, []diagnosticParserCoreHeader{{head: head, freshness: core.ReductionNew}})
	if err != nil {
		t.Fatal(err)
	}
	publishedSnapshot := append([]diagnosticParserCoreHeader(nil), published...)
	nextBefore := scratch.nextBuffer
	_, err = scratch.canonicalize(compact, []diagnosticParserCoreHeader{{
		head: core.Head{Node: 1 << 30}, freshness: core.ReductionNew,
	}})
	if err == nil {
		t.Fatal("invalid single header unexpectedly canonicalized")
	}
	if scratch.nextBuffer != nextBefore || !reflect.DeepEqual(published, publishedSnapshot) {
		t.Fatalf("failed single-header run published scratch: next=%d want=%d current=%+v want=%+v", scratch.nextBuffer, nextBefore, published, publishedSnapshot)
	}
}

// TestDiagnosticParserCoreElectionSummaryMatchesFullReceipt proves that the
// summary election path produces the same closed-frontier result as the full
// receipt path. The modes retain different diagnostic detail only.
func TestDiagnosticParserCoreElectionSummaryMatchesFullReceipt(t *testing.T) {
	fixture := loadDiagnosticParserCoreCanonicalFixture(t, "rewrite")
	target := uint32(919)
	run := func(mode DiagnosticParserCoreReceiptMode) DiagnosticParserCorePrefixResult {
		result, err := DiagnosticParseParserCorePrefix(
			parserCoreWarmGoScanner,
			fixture.Source,
			DiagnosticParserCorePrefixOptions{
				ReceiptMode: mode, GenericStopAtClosedByte: &target,
				MaxTokens: 300000, MaxDispatches: 600000,
				Limits: diagnosticParserCoreCanonicalLimits(),
			},
		)
		if err != nil {
			t.Fatalf("%v receipt route declined: result=%+v err=%v", mode, result, err)
		}
		return result
	}
	full := run(DiagnosticParserCoreReceiptFull)
	summary := run(DiagnosticParserCoreReceiptSummary)
	if full.GenericScheduler == nil || summary.GenericScheduler == nil || full.GenericScheduler.Completion == nil || summary.GenericScheduler.Completion == nil {
		t.Fatalf("missing completion: full=%+v summary=%+v", full.GenericScheduler, summary.GenericScheduler)
	}
	if full.Boundary != summary.Boundary || full.Detail != summary.Detail || full.Tokens != summary.Tokens ||
		full.Dispatches != summary.Dispatches || full.State != summary.State || full.Lookahead != summary.Lookahead ||
		full.LastBranchOrder != summary.LastBranchOrder || full.Completed != summary.Completed || full.Materialized != summary.Materialized {
		t.Fatalf("summary/full result drift: full=%+v summary=%+v", full, summary)
	}
	fullCompletion := full.GenericScheduler.Completion
	summaryCompletion := summary.GenericScheduler.Completion
	if fullCompletion.TargetByte != summaryCompletion.TargetByte || fullCompletion.ElectionIndex != summaryCompletion.ElectionIndex ||
		fullCompletion.LastToken != summaryCompletion.LastToken || fullCompletion.State != summaryCompletion.State ||
		fullCompletion.Stats != summaryCompletion.Stats || fullCompletion.Work != summaryCompletion.Work {
		t.Fatalf("summary/full completion drift: full=%+v summary=%+v", fullCompletion, summaryCompletion)
	}
	if len(full.GenericScheduler.Elections) == 0 || len(summary.GenericScheduler.Elections) != 0 ||
		len(full.Elections) == 0 || len(summary.Elections) != 0 || len(fullCompletion.Headers) == 0 || len(summaryCompletion.Headers) != 0 {
		t.Fatalf("receipt retention drift: full=%+v summary=%+v", full.GenericScheduler, summary.GenericScheduler)
	}
}

// TestDiagnosticParserCoreElectionPreservesInvalidCheckpointDeclines proves
// the direct summary state read preserves malformed-checkpoint errors and
// known-checkpoint identity declines in both receipt modes.
func TestDiagnosticParserCoreElectionPreservesInvalidCheckpointDeclines(t *testing.T) {
	compact, head, _ := newDiagnosticParserCoreCanonicalTestCore(t)
	foreign := mustDiagnosticCheckpointID(t, compact, []byte{1})
	for _, mode := range []DiagnosticParserCoreReceiptMode{
		DiagnosticParserCoreReceiptFull,
		DiagnosticParserCoreReceiptSummary,
	} {
		for _, test := range []struct {
			name          string
			checkpoint    core.CheckpointID
			wantMalformed bool
		}{
			{name: "known-mismatch", checkpoint: foreign},
			{name: "unknown", checkpoint: core.CheckpointID(99), wantMalformed: true},
		} {
			t.Run(test.name, func(t *testing.T) {
				scheduler := &diagnosticParserCoreGenericScheduler{
					compact: compact,
					headers: []diagnosticParserCoreHeader{{
						head: head, shifted: true, checkpoint: test.checkpoint,
					}},
					checkpointID: 0,
					options:      DiagnosticParserCorePrefixOptions{ReceiptMode: mode, MaxTokens: 1},
				}
				err := scheduler.elect(false)
				var decline *diagnosticParserCoreDecline
				if test.wantMalformed {
					if err == nil || errors.As(err, &decline) || err.Error() != "parser-core phase zero: header references unknown checkpoint identity" {
						t.Fatalf("invalid checkpoint err=%v decline=%+v", err, decline)
					}
				} else if !errors.As(err, &decline) || decline.boundary != DiagnosticParserCoreIdentity ||
					decline.detail != "generic scheduler election frontier is not closed and checkpoint-continuous" {
					t.Fatalf("mismatched checkpoint err=%v decline=%+v", err, decline)
				}
				if scheduler.tokens != 0 || scheduler.work.Elections != 0 || len(scheduler.electStates) != 0 || scheduler.headers[0].checkpoint != test.checkpoint {
					t.Fatalf("checkpoint rejection mutated scheduler: %+v", scheduler)
				}
			})
		}
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

// TestDiagnosticParserCoreCanonicalScratchSteadyStateSingleHeaderDoesNotAllocate
// pins the single-header fast path in canonicalize (the len(normalized) == 1
// branch), which skips the key-slice sizing and key-struct build that the
// two-header case above still exercises. Both cases must independently stay
// at zero steady-state allocations.
func TestDiagnosticParserCoreCanonicalScratchSteadyStateSingleHeaderDoesNotAllocate(t *testing.T) {
	compact, first, _ := newDiagnosticParserCoreCanonicalTestCore(t)
	input := []diagnosticParserCoreHeader{{head: first, creationSeq: 3}}
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
		t.Fatalf("steady single-header canonicalization allocs=%v err=%v", allocs, runErr)
	}
}

func TestDiagnosticParserCoreCanonicalScratchMappedSpillPreservesSemantics(t *testing.T) {
	compact, err := core.New(&genericConflictTable{}, core.Limits{})
	if err != nil {
		t.Fatal(err)
	}
	heads := make([]core.Head, 9)
	for index := range heads {
		heads[index], err = compact.Seed(core.StateID(index+1), 0)
		if err != nil {
			t.Fatal(err)
		}
	}
	checkpoint := mustDiagnosticCheckpointID(t, compact, []byte{7})
	headers := make([]diagnosticParserCoreHeader, 0, 10)
	headers = append(headers, diagnosticParserCoreHeader{
		head: heads[0], checkpoint: checkpoint, creationSeq: 1, paused: true, freshness: core.ReductionNew,
	})
	for index := 1; index < len(heads); index++ {
		headers = append(headers, diagnosticParserCoreHeader{
			head: heads[index], checkpoint: checkpoint, creationSeq: uint64(index + 1),
		})
	}
	headers = append(headers, diagnosticParserCoreHeader{
		head: heads[0], checkpoint: checkpoint, creationSeq: 99,
	})
	var scratch diagnosticParserCoreCanonicalScratch
	out, err := scratch.canonicalize(compact, headers)
	if err != nil {
		t.Fatal(err)
	}
	if len(out) != len(heads) || scratch.groups == nil {
		t.Fatalf("mapped spill output=%+v groups=%v", out, scratch.groups)
	}
	for index := 0; index < len(out)-1; index++ {
		if out[index].head != heads[index+1] || out[index].creationSeq != uint64(index+2) {
			t.Fatalf("mapped spill winner %d=%+v", index, out[index])
		}
	}
	last := out[len(out)-1]
	if last.head != heads[0] || last.creationSeq != 99 || last.paused || last.freshness != 0 || last.checkpoint != checkpoint {
		t.Fatalf("mapped spill duplicate winner=%+v", last)
	}
}

func TestDiagnosticParserCoreCanonicalScratchUsesSemanticVersionStateIdentity(t *testing.T) {
	for _, count := range []int{2, 9} {
		for _, changed := range []bool{false, true} {
			name := fmt.Sprintf("count-%d", count)
			if changed {
				name += "-semantic-change"
			}
			t.Run(name, func(t *testing.T) {
				compact, head, _ := newDiagnosticParserCoreCanonicalTestCore(t)
				snapshot := &diagnosticParserCoreVersionLexerSnapshot{
					compact: compact, coreGeneration: compact.ResetGeneration(),
					language: &Language{Name: "canonical-test"},
				}
				leftRequest := diagnosticParserCoreVersionLexerRequest{
					electionIndex: 1, headerCreationSeq: 2, state: 3,
					token:  Token{Symbol: 7, StartByte: 4, EndByte: 5, isKeyword: true},
					before: snapshot, after: snapshot.clone(),
					beforeCheckpoint: snapshot.beforeCheckpointInfo,
					afterCheckpoint:  snapshot.afterCheckpointInfo,
					beforeID:         snapshot.beforeCheckpoint, afterID: snapshot.afterCheckpoint,
					raggedSpan: true, valid: true,
				}
				rightRequest := leftRequest
				rightRequest.electionIndex = 19
				rightRequest.headerCreationSeq = 23
				rightRequest.token.isKeyword = false
				rightRequest.before = snapshot.clone()
				rightRequest.after = snapshot.clone()
				scheduler := &diagnosticParserCoreGenericScheduler{
					compact: compact,
					versionLexerRequests: []diagnosticParserCoreVersionLexerRequest{
						leftRequest, rightRequest,
					},
				}
				leftState := &diagnosticParserCoreVersionState{
					relexSnapshot: snapshot, lexerRequest: 1,
				}
				rightState := &diagnosticParserCoreVersionState{
					relexSnapshot: snapshot.clone(), lexerRequest: 2,
				}
				if changed {
					rightState.recoveryGroup = 1
				}
				scratch := diagnosticParserCoreCanonicalScratch{
					versionStateEqual: scheduler.versionLexerStateEqual,
				}
				headers := make([]diagnosticParserCoreHeader, count)
				for index := range headers {
					state := leftState
					if index != 0 {
						state = rightState
					}
					headers[index] = diagnosticParserCoreHeader{
						head: head, creationSeq: uint64(index + 1),
						freshness: func() core.ReductionFreshness {
							if index == 0 {
								return 0
							}
							return core.ReductionNew
						}(),
						versionState: state,
					}
				}
				out, err := scratch.canonicalize(compact, headers)
				if err != nil {
					t.Fatal(err)
				}
				want := 1
				if changed {
					want = 2
				}
				if len(out) != want {
					t.Fatalf("semantic keys produced %d headers, want %d: %+v", len(out), want, out)
				}
				if !changed && out[0].versionState != leftState {
					t.Fatalf("canonical winner did not retain the first header version state: got=%p want=%p", out[0].versionState, leftState)
				}
				if count > diagnosticParserCoreLinearCanonicalLimit && scratch.groups == nil {
					t.Fatal("mapped canonicalization did not allocate its group map")
				}
			})
		}
	}
}

func mustDiagnosticCheckpointID(t testing.TB, compact *core.Core, serialized []byte) core.CheckpointID {
	t.Helper()
	id, err := compact.InternCheckpoint(serialized)
	if err != nil {
		t.Fatal(err)
	}
	return id
}

func TestDiagnosticParserCoreCheckpointCompactLayoutsAMD64(t *testing.T) {
	if runtime.GOARCH != "amd64" {
		t.Skip("amd64 layout receipt")
	}
	// G18 adds two value-owned DropCohortRefSet fields. Each set is 72 bytes.
	// The exact scheduler header size is 224 bytes. Canonicalization and
	// rollback copy this header.
	if got := unsafe.Sizeof(diagnosticParserCoreHeader{}); got != 224 {
		t.Fatalf("scheduler header size=%d, want 224", got)
	}
	if got := unsafe.Sizeof(diagnosticParserCorePhaseHead{}); got != 24 {
		t.Fatalf("canonical phase key size=%d, want 24", got)
	}
}
