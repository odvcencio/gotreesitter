package parsercorephase0

import (
	"errors"
	"fmt"
	"math"
	"reflect"
	"slices"
	"strings"
	"testing"
	"unsafe"
)

func TestPinnedGoConflictAndReductionMetadata(t *testing.T) {
	wantConflict := []Action{
		{Type: ActionReduce, Symbol: 171, ChildCount: 1},
		{Type: ActionShift, State: 194},
	}
	wantReduce := Action{Type: ActionReduce, Symbol: 121, ChildCount: 1, DynamicPrecedence: -1, ProductionID: 44}
	wantReduceCell := []Action{wantReduce, Action{Type: ActionReduce, Symbol: 171, ChildCount: 1}}
	tables := &fakeTable{
		actions: map[tableCell][]Action{
			{state: 20, symbol: 4}: wantConflict,
			{state: 20, symbol: 6}: wantReduceCell,
		},
		gotos: map[tableCell]StateID{
			{state: 1, symbol: 121}: 101,
			{state: 1, symbol: 171}: 2,
		},
		aliases: map[productionKey][]Symbol{{productionID: 44, childCount: 1}: {229}},
	}
	core, err := New(tables, Limits{MaxDerivations: 8, MaxPopPaths: 8})
	if err != nil {
		t.Fatal(err)
	}

	got, err := core.Actions(20, 4)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, wantConflict) {
		t.Fatalf("Go cell (20,4) = %+v, want pinned conflict %+v", got, wantConflict)
	}
	conflictSeed, _ := core.Seed(20, 0)
	conflictHead, err := core.Shift(conflictSeed, 4, 1, Token{Symbol: 4, EndByte: 1}, ForkOrder{Present: true, Value: 1})
	if err != nil {
		t.Fatal(err)
	}
	conflictPaths, err := core.Derivations(conflictHead)
	if err != nil {
		t.Fatal(err)
	}
	if len(conflictPaths) != 1 || !conflictPaths[0].HasBranchOrder || conflictPaths[0].BranchOrder != 1 {
		t.Fatalf("authentic conflict shift order = %#v, want current order 1", conflictPaths)
	}

	reduceActions, err := core.Actions(20, 6)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(reduceActions, wantReduceCell) {
		t.Fatalf("Go reduction cell (20,6)/ParseActions[107] drifted: %+v", reduceActions)
	}
	if gotoState, err := tables.Goto(1, 121); err != nil || gotoState != 101 {
		t.Fatalf("lookupGoto(1,121) = %d, %v, want 101", gotoState, err)
	}

	head, err := core.Seed(1, 0)
	if err != nil {
		t.Fatal(err)
	}
	var wantOrder uint64
	var wantScore int64
	for i := 0; i < int(wantReduce.ChildCount); i++ {
		target := StateID(1)
		if i == int(wantReduce.ChildCount)-1 {
			target = 20
		}
		order := uint64(10 + i)
		delta := int64(i + 1)
		head, err = core.appendDiagnosticPayload(head, target, Token{
			Symbol: Symbol(1), StartByte: uint32(i), EndByte: uint32(i + 1),
		}, pathMeta{ScoreDelta: delta, BranchOrder: ForkOrder{Present: true, Value: order}})
		if err != nil {
			t.Fatal(err)
		}
		wantScore += delta
		wantOrder = order
	}

	frontier, err := core.Reduce(head, 6, 0, ForkOrder{})
	if err != nil {
		t.Fatalf("Reduce(real Go action): %v", err)
	}
	if len(frontier) != 1 {
		t.Fatalf("reduction frontier has %d boundaries, want 1", len(frontier))
	}
	head = frontier[0]
	paths, err := core.Derivations(head)
	if err != nil {
		t.Fatal(err)
	}
	if len(paths) != 1 || len(paths[0].Payloads) != 1 {
		t.Fatalf("reduced derivations = %#v, want one collapsed payload", paths)
	}
	wantScore += int64(wantReduce.DynamicPrecedence)
	if paths[0].Score != wantScore || !paths[0].HasBranchOrder || paths[0].BranchOrder != wantOrder {
		t.Fatalf("path meta = score %d order %v, want score %d order %v", paths[0].Score, paths[0].BranchOrder, wantScore, wantOrder)
	}

	view, err := core.Subtree(paths[0].Payloads[0])
	if err != nil {
		t.Fatal(err)
	}
	if view.Symbol != wantReduce.Symbol || view.ProductionID != wantReduce.ProductionID || view.DynamicPrecedence != wantReduce.DynamicPrecedence {
		t.Fatalf("reduction identity = (%d,%d,%d), want (%d,%d,%d)", view.Symbol, view.ProductionID, view.DynamicPrecedence, wantReduce.Symbol, wantReduce.ProductionID, wantReduce.DynamicPrecedence)
	}
	if len(view.Fields) != 0 {
		t.Fatalf("fields = %#v, want pinned empty production fields", view.Fields)
	}
	if !slices.Equal(view.Aliases, []Symbol{229}) {
		t.Fatalf("aliases = %#v, want pinned [229]", view.Aliases)
	}

	// ParseActions[106].Actions[0] is the pinned real Go production-zero
	// reduction from the conflict cell above. Production zero has no alias
	// row; generated metadata represents that ordinary case as nil rather
	// than a child-count-sized zero slice.
	base := StateID(1)
	noAliasCore, err := New(tables, Limits{MaxDerivations: 8, MaxPopPaths: 8})
	if err != nil {
		t.Fatal(err)
	}
	noAliasHead, err := noAliasCore.Seed(base, 0)
	if err != nil {
		t.Fatal(err)
	}
	noAliasHead, err = noAliasCore.appendDiagnosticPayload(noAliasHead, 20, Token{Symbol: 1, EndByte: 1}, pathMeta{})
	if err != nil {
		t.Fatal(err)
	}
	noAliasFrontier, err := noAliasCore.Reduce(noAliasHead, 4, 0, ForkOrder{Present: true, Value: 1})
	if err != nil {
		t.Fatalf("Reduce(real Go no-alias action): %v", err)
	}
	if len(noAliasFrontier) != 1 {
		t.Fatalf("no-alias reduction frontier has %d boundaries, want 1", len(noAliasFrontier))
	}
	noAliasPaths, err := noAliasCore.Derivations(noAliasFrontier[0])
	if err != nil {
		t.Fatal(err)
	}
	if len(noAliasPaths) != 1 || len(noAliasPaths[0].Payloads) != 1 {
		t.Fatalf("no-alias derivations = %#v, want one collapsed payload", noAliasPaths)
	}
	noAliasView, err := noAliasCore.Subtree(noAliasPaths[0].Payloads[0])
	if err != nil {
		t.Fatal(err)
	}
	if len(noAliasView.Aliases) != 0 {
		t.Fatalf("ordinary Go production aliases = %v, want none", noAliasView.Aliases)
	}
}

func TestFrontierEpochPreventsStaleBoundaryResurrection(t *testing.T) {
	core := newTinyCore(t, 4)
	seed, err := core.Seed(1, 0)
	if err != nil {
		t.Fatal(err)
	}
	payload, err := core.appendSubtree(subtreeRecord{symbol: 1, terminal: true, endByte: 1}, nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	oldKey := core.boundaryKey(2, 1)
	oldHead, err := core.condense(oldKey, linkInput{prev: seed.Node, payload: payload, scoreDelta: 1})
	if err != nil {
		t.Fatal(err)
	}
	if err := core.BeginFrontier(); err != nil {
		t.Fatal(err)
	}
	newHead, err := core.condense(core.boundaryKey(2, 1), linkInput{prev: seed.Node, payload: payload, scoreDelta: 2})
	if err != nil {
		t.Fatal(err)
	}
	stats, err := core.Stats(newHead)
	if err != nil {
		t.Fatal(err)
	}
	if stats.CurrentExactPaths != 1 {
		t.Fatalf("new epoch inherited %d exact paths, want 1", stats.CurrentExactPaths)
	}
	paths, err := core.Derivations(newHead)
	if err != nil {
		t.Fatal(err)
	}
	if len(paths) != 1 || paths[0].Score != 2 {
		t.Fatalf("new epoch derivations = %#v, want only score-2 path", paths)
	}
	oldPaths, err := core.Derivations(oldHead)
	if err != nil {
		t.Fatal(err)
	}
	if len(oldPaths) != 1 || oldPaths[0].Score != 1 {
		t.Fatalf("persistent old head derivations = %#v, want score-1 path", oldPaths)
	}
	if _, err := core.condense(oldKey, linkInput{prev: seed.Node, payload: payload, scoreDelta: 3}); err == nil || !strings.Contains(err.Error(), "frontier mismatch") {
		t.Fatalf("stale boundary insertion error = %v, want frontier mismatch", err)
	}
}

func TestHeaderConvergenceRetainsFirstZeroChildPayload(t *testing.T) {
	core := newTinyCore(t, 6)
	seed, err := core.Seed(1, 0)
	if err != nil {
		t.Fatal(err)
	}
	payload, err := core.appendSubtree(subtreeRecord{symbol: 1, terminal: true, endByte: 1}, nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	key := core.boundaryKey(2, 1)
	head, err := core.condense(key, linkInput{prev: seed.Node, payload: payload, scoreDelta: 1, order: ForkOrder{Present: true, Value: 7}})
	if err != nil {
		t.Fatal(err)
	}
	head, err = core.condense(key, linkInput{prev: seed.Node, payload: payload, scoreDelta: 2, order: ForkOrder{Present: true, Value: 7}})
	if err != nil {
		t.Fatal(err)
	}
	head, err = core.condense(key, linkInput{prev: seed.Node, payload: payload, scoreDelta: 1, order: ForkOrder{Present: true, Value: 8}})
	if err != nil {
		t.Fatal(err)
	}

	paths, err := core.Derivations(head)
	if err != nil {
		t.Fatal(err)
	}
	want := []Derivation{{Payloads: []SubtreeID{payload}, Score: 1, BranchOrder: 7, HasBranchOrder: true}}
	if !reflect.DeepEqual(paths, want) {
		t.Fatalf("derivations = %#v, want first effective-zero link %#v", paths, want)
	}
	stats, err := core.Stats(head)
	if err != nil {
		t.Fatal(err)
	}
	if stats.Links != 1 || stats.CurrentExactPaths != 1 {
		t.Fatalf("selection stats = %+v, want one retained effective-zero path", stats)
	}
}

func TestSharedBoundaryPathMultiplicityDoesNotGateExecution(t *testing.T) {
	build := func(t *testing.T, limits Limits) (*Core, Head) {
		t.Helper()
		core := newTinyCoreWithLimits(t, limits)
		core.diagnostics.foldSamePredecessorShallowPayloads = false
		seed, _ := core.Seed(1, 0)
		payload, _ := core.appendSubtree(subtreeRecord{symbol: 1, terminal: true, endByte: 1}, nil, nil, nil)
		buildFourPathHead := func(state StateID, scoreBase int64) Head {
			key := core.boundaryKey(state, 1)
			var head Head
			for index := int64(0); index < 4; index++ {
				var err error
				head, err = core.condense(key, linkInput{prev: seed.Node, payload: payload, scoreDelta: scoreBase + index})
				if err != nil {
					t.Fatal(err)
				}
			}
			return head
		}
		left := buildFourPathHead(2, 10)
		right := buildFourPathHead(3, 20)
		key := core.boundaryKey(4, 1)
		head, err := core.condense(key, linkInput{prev: left.Node, payload: payload, scoreDelta: 30})
		if err != nil {
			t.Fatal(err)
		}
		head, err = core.condense(key, linkInput{prev: right.Node, payload: payload, scoreDelta: 40})
		if err != nil {
			t.Fatal(err)
		}
		return core, head
	}

	compact, head := build(t, Limits{})
	stats, err := compact.Stats(head)
	if err != nil || stats.CurrentExactPaths != 8 {
		t.Fatalf("default packed multiplicity stats=%+v err=%v, want 8 paths", stats, err)
	}
	paths, err := compact.Derivations(head)
	if err != nil || len(paths) != 8 {
		t.Fatalf("default packed derivations=%d err=%v, want 8", len(paths), err)
	}

	diagnostic, diagnosticHead := build(t, Limits{MaxDerivations: 6})
	diagnosticStats, err := diagnostic.Stats(diagnosticHead)
	if err != nil || diagnosticStats.CurrentExactPaths != 8 {
		t.Fatalf("diagnostic-capped execution stats=%+v err=%v, want retained 8 paths", diagnosticStats, err)
	}
	if _, err := diagnostic.Derivations(diagnosticHead); err == nil || !strings.Contains(err.Error(), "derivation enumeration cap") {
		t.Fatalf("diagnostic derivation cap error=%v", err)
	}
}

func TestLiveLinkCapIsLocalAndTransactional(t *testing.T) {
	core := newTinyCoreWithLimits(t, Limits{MaxLinksPerBoundary: 1})
	core.diagnostics.foldSamePredecessorShallowPayloads = false
	seed, _ := core.Seed(1, 0)
	head, err := core.appendDiagnosticPayload(seed, 2, Token{Symbol: 1, EndByte: 1}, pathMeta{ScoreDelta: 1})
	if err != nil {
		t.Fatal(err)
	}
	before, err := core.Stats(head)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := core.appendDiagnosticPayload(seed, 2, Token{Symbol: 1, EndByte: 1}, pathMeta{ScoreDelta: 2, BranchOrder: ForkOrder{Present: true, Value: 9}}); err == nil || !strings.Contains(err.Error(), "live-link cap") {
		t.Fatalf("second distinct live link error=%v", err)
	}
	after, err := core.Stats(head)
	if err != nil {
		t.Fatal(err)
	}
	if after != before {
		t.Fatalf("declined append mutated storage: before=%+v after=%+v", before, after)
	}
}

func TestDefaultLiveLinkCapRejectsNinthDistinctLink(t *testing.T) {
	core := newTinyCoreWithLimits(t, Limits{})
	core.diagnostics.foldSamePredecessorShallowPayloads = false
	seed, _ := core.Seed(1, 0)
	payload, _ := core.appendSubtree(subtreeRecord{symbol: 1, terminal: true, endByte: 1}, nil, nil, nil)
	key := core.boundaryKey(2, 1)
	var head Head
	for index := int64(0); index < 8; index++ {
		var err error
		head, err = core.condense(key, linkInput{prev: seed.Node, payload: payload, scoreDelta: index})
		if err != nil {
			t.Fatalf("live link %d: %v", index+1, err)
		}
	}
	before, err := core.Stats(head)
	if err != nil || before.CurrentExactPaths != 8 {
		t.Fatalf("eight-link stats=%+v err=%v", before, err)
	}
	duplicate, err := core.condense(key, linkInput{prev: seed.Node, payload: payload, scoreDelta: 7})
	if err != nil || duplicate != head {
		t.Fatalf("exact duplicate consumed a live-link slot: duplicate=%+v head=%+v err=%v", duplicate, head, err)
	}
	if _, err := core.condense(key, linkInput{prev: seed.Node, payload: payload, scoreDelta: 8}); err == nil || !strings.Contains(err.Error(), "live-link cap exceeded: 9 > 8") {
		t.Fatalf("ninth distinct live link error=%v", err)
	}
	after, err := core.Stats(head)
	if err != nil || after != before {
		t.Fatalf("ninth-link failure mutated core: before=%+v after=%+v err=%v", before, after, err)
	}
	canonical, ok := core.CanonicalBoundary(2, 1, false, [32]byte{})
	if !ok || canonical != head {
		t.Fatalf("ninth-link failure changed canonical boundary: head=%+v canonical=%+v ok=%t", head, canonical, ok)
	}
}

func TestExtraShiftWithZeroTargetRetainsCurrentState(t *testing.T) {
	tables := &fakeTable{
		actions: map[tableCell][]Action{
			{state: 1, symbol: 1}: {{Type: ActionShift, Extra: true}},
		},
	}
	core, err := New(tables, Limits{MaxDerivations: 2, MaxPopPaths: 2})
	if err != nil {
		t.Fatal(err)
	}
	seed, _ := core.Seed(1, 0)
	head, err := core.Shift(seed, 1, 0, Token{Symbol: 1, EndByte: 1, Extra: true}, ForkOrder{})
	if err != nil {
		t.Fatal(err)
	}
	n, _ := core.node(head.Node)
	if n.state != 1 {
		t.Fatalf("extra shift target state = %d, want current state 1", n.state)
	}
	before, _ := core.Stats(head)
	if _, err := core.Shift(head, 1, 0, Token{Symbol: 1, EndByte: 2, Extra: false}, ForkOrder{}); err == nil || !strings.Contains(err.Error(), "disagrees") {
		t.Fatalf("extra mismatch error = %v", err)
	}
	after, _ := core.Stats(head)
	if after != before {
		t.Fatalf("extra mismatch mutated core: before=%+v after=%+v", before, after)
	}
}

func TestConsumedPhasePreventsZeroWidthShiftReductionCondensation(t *testing.T) {
	tables := &fakeTable{actions: map[tableCell][]Action{
		{state: 1, symbol: 1}: {{Type: ActionShift, State: 2}},
	}}
	core, err := New(tables, Limits{MaxDerivations: 4, MaxPopPaths: 4})
	if err != nil {
		t.Fatal(err)
	}
	seed, _ := core.Seed(1, 0)
	shifted, err := core.Shift(seed, 1, 0, Token{Symbol: 1}, ForkOrder{})
	if err != nil {
		t.Fatal(err)
	}
	payload, err := core.appendSubtree(subtreeRecord{symbol: 2, terminal: true}, nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	runnable, err := core.condense(core.boundaryKey(2, 0), linkInput{prev: seed.Node, payload: payload})
	if err != nil {
		t.Fatal(err)
	}
	if shifted.Node == runnable.Node {
		t.Fatalf("consumed and runnable zero-width heads condensed to %d", shifted.Node)
	}
	if canonical, ok := core.CanonicalBoundary(2, 0, true, [32]byte{}); !ok || canonical != shifted {
		t.Fatalf("consumed canonical head=%+v ok=%t, want %+v", canonical, ok, shifted)
	}
	if canonical, ok := core.CanonicalBoundary(2, 0, false, [32]byte{}); !ok || canonical != runnable {
		t.Fatalf("runnable canonical head=%+v ok=%t, want %+v", canonical, ok, runnable)
	}
}

func TestScannerCheckpointPreventsSameBoundaryCondensation(t *testing.T) {
	core := newTinyCore(t, 4)
	seed, _ := core.Seed(1, 0)
	payload, _ := core.appendSubtree(subtreeRecord{symbol: 1, terminal: true, endByte: 1}, nil, nil, nil)
	firstCheckpoint := [32]byte{1}
	secondCheckpoint := [32]byte{2}
	core.SetPhaseCheckpoint(firstCheckpoint)
	first, err := core.condense(core.boundaryKey(2, 1), linkInput{prev: seed.Node, payload: payload})
	if err != nil {
		t.Fatal(err)
	}
	core.SetPhaseCheckpoint(secondCheckpoint)
	second, err := core.condense(core.boundaryKey(2, 1), linkInput{prev: seed.Node, payload: payload})
	if err != nil {
		t.Fatal(err)
	}
	if first == second {
		t.Fatalf("distinct scanner checkpoints condensed to head %+v", first)
	}
	if canonical, ok := core.CanonicalBoundary(2, 1, false, firstCheckpoint); !ok || canonical != first {
		t.Fatalf("first checkpoint canonical head=%+v ok=%t, want %+v", canonical, ok, first)
	}
	if canonical, ok := core.CanonicalBoundary(2, 1, false, secondCheckpoint); !ok || canonical != second {
		t.Fatalf("second checkpoint canonical head=%+v ok=%t, want %+v", canonical, ok, second)
	}
}

func TestApplyAtomicRollsBackEarlierConflictArm(t *testing.T) {
	core := newTinyCore(t, 4)
	seed, _ := core.Seed(1, 0)
	payload, _ := core.appendSubtree(subtreeRecord{symbol: 1, terminal: true, endByte: 1}, nil, nil, nil)
	before, _ := core.Stats(seed)
	beforeFrontier, beforeCheckpoint := core.frontier, core.checkpoint
	err := core.ApplyAtomic(func() error {
		if err := core.BeginFrontier(); err != nil {
			return err
		}
		core.SetPhaseCheckpoint([32]byte{1})
		if _, err := core.condense(core.boundaryKey(2, 1), linkInput{prev: seed.Node, payload: payload}); err != nil {
			return err
		}
		return errors.New("later primary arm declined")
	})
	if err == nil || !strings.Contains(err.Error(), "primary") {
		t.Fatalf("atomic conflict error=%v", err)
	}
	after, _ := core.Stats(seed)
	if after != before {
		t.Fatalf("atomic conflict rollback mutated core: before=%+v after=%+v", before, after)
	}
	if core.frontier != beforeFrontier || core.checkpoint != beforeCheckpoint {
		t.Fatalf("atomic phase rollback=(%d,%x), want (%d,%x)", core.frontier, core.checkpoint, beforeFrontier, beforeCheckpoint)
	}
	if _, ok := core.CanonicalBoundary(2, 1, false, [32]byte{}); ok {
		t.Fatal("rolled-back conflict boundary remains published")
	}
}

func TestBoundaryMutationJournalRollback(t *testing.T) {
	newCore := func(t *testing.T) (*Core, Head) {
		t.Helper()
		compact, err := New(&fakeTable{}, Limits{})
		if err != nil {
			t.Fatal(err)
		}
		head, err := compact.Seed(1, 0)
		if err != nil {
			t.Fatal(err)
		}
		return compact, head
	}
	sentinel := errors.New("rollback")

	t.Run("repeated-same-key", func(t *testing.T) {
		compact, head := newCore(t)
		key := compact.boundaryKey(1, 0)
		err := compact.ApplyAtomic(func() error {
			compact.writeBoundary(key, 41)
			compact.writeBoundary(key, 42)
			if len(compact.boundaryJournal) != 2 {
				return fmt.Errorf("journal entries=%d, want 2", len(compact.boundaryJournal))
			}
			return sentinel
		})
		if !errors.Is(err, sentinel) || compact.boundaries[key] != head.Node {
			t.Fatalf("same-key rollback err=%v boundary=%d, want %d", err, compact.boundaries[key], head.Node)
		}
		assertTransactionJournalClean(t, compact)
	})

	t.Run("absent-key-deletion", func(t *testing.T) {
		compact, _ := newCore(t)
		key := compact.boundaryKey(9, 9)
		err := compact.ApplyAtomic(func() error {
			compact.writeBoundary(key, 9)
			return sentinel
		})
		if !errors.Is(err, sentinel) {
			t.Fatalf("absent-key rollback err=%v", err)
		}
		if _, exists := compact.boundaries[key]; exists {
			t.Fatal("absent boundary survived rollback")
		}
		assertTransactionJournalClean(t, compact)
	})

	t.Run("nested-success-outer-rollback", func(t *testing.T) {
		compact, head := newCore(t)
		key := compact.boundaryKey(1, 0)
		err := compact.ApplyAtomic(func() error {
			compact.writeBoundary(key, 2)
			if err := compact.ApplyAtomic(func() error {
				compact.writeBoundary(key, 3)
				return nil
			}); err != nil {
				return err
			}
			if compact.boundaries[key] != 3 || len(compact.transactions) != 1 || len(compact.boundaryJournal) != 2 {
				return fmt.Errorf("inner commit state boundary=%d transactions=%d journal=%d", compact.boundaries[key], len(compact.transactions), len(compact.boundaryJournal))
			}
			return sentinel
		})
		if !errors.Is(err, sentinel) || compact.boundaries[key] != head.Node {
			t.Fatalf("outer rollback err=%v boundary=%d, want %d", err, compact.boundaries[key], head.Node)
		}
		assertTransactionJournalClean(t, compact)
	})

	t.Run("inner-failure-outer-commit", func(t *testing.T) {
		compact, _ := newCore(t)
		key := compact.boundaryKey(1, 0)
		err := compact.ApplyAtomic(func() error {
			compact.writeBoundary(key, 2)
			innerErr := compact.ApplyAtomic(func() error {
				compact.writeBoundary(key, 3)
				return sentinel
			})
			if !errors.Is(innerErr, sentinel) || compact.boundaries[key] != 2 || len(compact.transactions) != 1 || len(compact.boundaryJournal) != 1 {
				return fmt.Errorf("inner rollback err=%v boundary=%d transactions=%d journal=%d", innerErr, compact.boundaries[key], len(compact.transactions), len(compact.boundaryJournal))
			}
			return nil
		})
		if err != nil || compact.boundaries[key] != 2 {
			t.Fatalf("outer commit err=%v boundary=%d, want 2", err, compact.boundaries[key])
		}
		assertTransactionJournalClean(t, compact)
	})
}

func TestMutationJournalRestoresScalarsAndArenas(t *testing.T) {
	compact, err := New(&fakeTable{}, Limits{})
	if err != nil {
		t.Fatal(err)
	}
	head, err := compact.Seed(1, 0)
	if err != nil {
		t.Fatal(err)
	}
	beforeStats, _ := compact.Stats(head)
	beforeBoundaries := cloneBoundaryMap(compact.boundaries)
	beforeFrontier, beforeCheckpoint := compact.frontier, compact.checkpoint
	wantCheckpoint := [32]byte{1, 2, 3}
	sentinel := errors.New("rollback")
	err = compact.ApplyAtomic(func() error {
		if err := compact.BeginFrontier(); err != nil {
			return err
		}
		compact.SetPhaseCheckpoint(wantCheckpoint)
		payload, err := compact.appendSubtree(subtreeRecord{symbol: 7, terminal: true}, nil, nil, nil)
		if err != nil {
			return err
		}
		if _, err := compact.appendSubtree(subtreeRecord{symbol: 8}, []SubtreeID{payload}, nil, nil); err != nil {
			return err
		}
		if _, err := compact.condense(compact.boundaryKey(2, 1), linkInput{prev: head.Node, payload: payload}); err != nil {
			return err
		}
		return sentinel
	})
	if !errors.Is(err, sentinel) {
		t.Fatalf("scalar/arena rollback err=%v", err)
	}
	afterStats, _ := compact.Stats(head)
	if afterStats != beforeStats || compact.frontier != beforeFrontier || compact.checkpoint != beforeCheckpoint || !reflect.DeepEqual(compact.boundaries, beforeBoundaries) {
		t.Fatalf("scalar/arena rollback stats=%+v/%+v frontier=%d/%d checkpoint=%x/%x boundaries=%v/%v",
			afterStats, beforeStats, compact.frontier, beforeFrontier, compact.checkpoint, beforeCheckpoint, compact.boundaries, beforeBoundaries)
	}
	assertTransactionJournalClean(t, compact)
}

func TestOuterRollbackUndoesSuccessfulNestedParserOperations(t *testing.T) {
	tables := &fakeTable{
		actions: map[tableCell][]Action{
			{state: 1, symbol: 9}:  {{Type: ActionShift, State: 3}},
			{state: 2, symbol: 9}:  {{Type: ActionShift, State: 3}},
			{state: 3, symbol: 10}: {{Type: ActionReduce, Symbol: 4, ChildCount: 1}},
		},
		gotos: map[tableCell]StateID{{state: 1, symbol: 4}: 5},
	}
	compact, err := New(tables, Limits{MaxDerivations: 8, MaxPopPaths: 8})
	if err != nil {
		t.Fatal(err)
	}
	first, _ := compact.Seed(1, 0)
	second, _ := compact.Seed(2, 0)
	before, _ := compact.Stats(first)
	beforeBoundaries := cloneBoundaryMap(compact.boundaries)
	sentinel := errors.New("outer rollback")
	err = compact.ApplyAtomic(func() error {
		shifted, err := compact.Shift(first, 9, 0, Token{Symbol: 9, EndByte: 1}, ForkOrder{})
		if err != nil {
			return err
		}
		if _, err := compact.Reduce(shifted, 10, 0, ForkOrder{}); err != nil {
			return err
		}
		if _, err := compact.ShiftOrdinaryCohort(
			[]OrdinaryCohortShiftInput{{Head: first}, {Head: second}},
			9,
			Token{Symbol: 9, EndByte: 1},
		); err != nil {
			return err
		}
		if len(compact.transactions) != 1 || len(compact.boundaryJournal) == 0 {
			return fmt.Errorf("nested commits did not remain in outer journal: transactions=%d journal=%d", len(compact.transactions), len(compact.boundaryJournal))
		}
		return sentinel
	})
	if !errors.Is(err, sentinel) {
		t.Fatalf("nested parser operation rollback err=%v", err)
	}
	after, _ := compact.Stats(first)
	if after != before || !reflect.DeepEqual(compact.boundaries, beforeBoundaries) {
		t.Fatalf("nested parser operations survived rollback: stats=%+v/%+v boundaries=%v/%v", after, before, compact.boundaries, beforeBoundaries)
	}
	assertTransactionJournalClean(t, compact)
}

func TestBoundaryJournalDoesNotGrowOutsideTransaction(t *testing.T) {
	compact, err := New(&fakeTable{}, Limits{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := compact.Seed(1, 0); err != nil {
		t.Fatal(err)
	}
	compact.writeBoundary(compact.boundaryKey(2, 0), 1)
	assertTransactionJournalClean(t, compact)
}

func TestTransactionCheckpointRejectsNonLIFOUse(t *testing.T) {
	compact, err := New(&fakeTable{}, Limits{})
	if err != nil {
		t.Fatal(err)
	}
	outer := compact.mark()
	_ = compact.mark()
	defer func() {
		if recovered := recover(); recovered == nil {
			t.Fatal("out-of-order transaction commit did not panic")
		}
	}()
	compact.commit(outer)
}

func TestApplyAtomicPanicRollsBackAndRepanics(t *testing.T) {
	compact, err := New(&fakeTable{}, Limits{})
	if err != nil {
		t.Fatal(err)
	}
	head, err := compact.Seed(1, 0)
	if err != nil {
		t.Fatal(err)
	}
	key := compact.boundaryKey(1, 0)
	func() {
		defer func() {
			if recovered := recover(); recovered != "boom" {
				t.Fatalf("recovered=%v, want boom", recovered)
			}
		}()
		_ = compact.ApplyAtomic(func() error {
			compact.writeBoundary(key, 99)
			panic("boom")
		})
	}()
	if compact.boundaries[key] != head.Node {
		t.Fatalf("panic rollback boundary=%d, want %d", compact.boundaries[key], head.Node)
	}
	assertTransactionJournalClean(t, compact)
}

func TestApplyAtomicNoOpDoesNotScaleWithBoundaryCount(t *testing.T) {
	compact, err := New(&fakeTable{}, Limits{})
	if err != nil {
		t.Fatal(err)
	}
	for index := 0; index < 1<<15; index++ {
		compact.writeBoundary(boundaryKey{frontier: 1, state: StateID(index + 1), byteOffset: uint32(index)}, NodeID(index+1))
	}
	noop := func() error { return nil }
	if err := compact.ApplyAtomic(noop); err != nil {
		t.Fatal(err)
	}
	allocs := testing.AllocsPerRun(100, func() {
		if err := compact.ApplyAtomic(noop); err != nil {
			panic(err)
		}
	})
	if allocs != 0 {
		t.Fatalf("no-op transaction allocations=%v, want 0 with %d preloaded boundaries", allocs, len(compact.boundaries))
	}
	assertTransactionJournalClean(t, compact)
}

func BenchmarkApplyAtomicNoOpPreloadedBoundaries(b *testing.B) {
	compact, err := New(&fakeTable{}, Limits{})
	if err != nil {
		b.Fatal(err)
	}
	for index := 0; index < 1<<16; index++ {
		compact.writeBoundary(boundaryKey{frontier: 1, state: StateID(index + 1), byteOffset: uint32(index)}, NodeID(index+1))
	}
	noop := func() error { return nil }
	if err := compact.ApplyAtomic(noop); err != nil {
		b.Fatal(err)
	}
	b.ReportAllocs()
	b.ResetTimer()
	for index := 0; index < b.N; index++ {
		if err := compact.ApplyAtomic(noop); err != nil {
			b.Fatal(err)
		}
	}
}

func assertTransactionJournalClean(t *testing.T, compact *Core) {
	t.Helper()
	if len(compact.transactions) != 0 || len(compact.boundaryJournal) != 0 {
		t.Fatalf("transaction state not clean: transactions=%v journal=%v", compact.transactions, compact.boundaryJournal)
	}
}

func TestShiftOrdinaryCohortSharesOneTerminalPayload(t *testing.T) {
	tables := &fakeTable{actions: map[tableCell][]Action{
		{state: 1, symbol: 9}: {{Type: ActionShift, State: 3}},
		{state: 2, symbol: 9}: {{Type: ActionShift, State: 3}},
	}}
	compact, err := New(tables, Limits{MaxDerivations: 4, MaxPopPaths: 4})
	if err != nil {
		t.Fatal(err)
	}
	first, _ := compact.Seed(1, 4)
	second, _ := compact.Seed(2, 4)
	before, _ := compact.Stats(first)

	shifted, err := compact.ShiftOrdinaryCohort([]OrdinaryCohortShiftInput{
		{Head: first, ActionOrdinal: 0},
		{Head: second, ActionOrdinal: 0},
	}, 9, Token{Symbol: 9, StartByte: 4, EndByte: 5})
	if err != nil {
		t.Fatal(err)
	}
	if len(shifted) != 2 {
		t.Fatalf("shifted cohort length=%d, want 2", len(shifted))
	}
	after, _ := compact.Stats(shifted[1])
	if after.Nodes-before.Nodes != 2 || after.Links-before.Links != 2 || after.Subtrees-before.Subtrees != 1 || after.CurrentExactPaths != 2 {
		t.Fatalf("shared cohort physical delta=%+v -> %+v, want N+2/L+2/S+1 and two paths", before, after)
	}
	var shared SubtreeID
	for index, head := range shifted {
		paths, err := compact.Derivations(head)
		if err != nil {
			t.Fatal(err)
		}
		if len(paths) != index+1 {
			t.Fatalf("shifted head %d paths=%d, want %d", index, len(paths), index+1)
		}
		for _, path := range paths {
			if len(path.Payloads) != 1 {
				t.Fatalf("shifted head %d path=%+v, want one terminal", index, path)
			}
			if shared == 0 {
				shared = path.Payloads[0]
			} else if path.Payloads[0] != shared {
				t.Fatalf("shifted head %d payload=%d, want shared %d", index, path.Payloads[0], shared)
			}
		}
	}
	view, err := compact.Subtree(shared)
	if err != nil {
		t.Fatal(err)
	}
	if view.Symbol != 9 || view.StartByte != 4 || view.EndByte != 5 || view.Extra || !view.Terminal || len(view.Children) != 0 || len(view.Fields) != 0 || len(view.Aliases) != 0 {
		t.Fatalf("shared terminal view=%+v", view)
	}
}

func TestExternalTerminalProvenanceIsPartOfSubtreeIdentity(t *testing.T) {
	tables := &fakeTable{actions: map[tableCell][]Action{
		{state: 1, symbol: 9}: {{Type: ActionShift, State: 3}},
	}}
	compact, err := New(tables, Limits{MaxDerivations: 4, MaxPopPaths: 4})
	if err != nil {
		t.Fatal(err)
	}
	seed, err := compact.Seed(1, 4)
	if err != nil {
		t.Fatal(err)
	}
	ordinary, err := compact.Shift(seed, 9, 0, Token{Symbol: 9, StartByte: 4, EndByte: 5}, ForkOrder{})
	if err != nil {
		t.Fatal(err)
	}
	external, err := compact.Shift(seed, 9, 0, Token{Symbol: 9, StartByte: 4, EndByte: 5, External: true}, ForkOrder{})
	if err != nil {
		t.Fatal(err)
	}
	ordinaryPaths, err := compact.Derivations(ordinary)
	if err != nil {
		t.Fatal(err)
	}
	externalPaths, err := compact.Derivations(external)
	if err != nil {
		t.Fatal(err)
	}
	if len(ordinaryPaths) != 1 || len(externalPaths) != 2 {
		t.Fatalf("ordinary/external path identity ordinary=%+v external=%+v", ordinaryPaths, externalPaths)
	}
	var ordinarySeen, externalSeen bool
	for _, path := range externalPaths {
		if len(path.Payloads) != 1 {
			t.Fatalf("terminal derivation=%+v", path)
		}
		view, err := compact.Subtree(path.Payloads[0])
		if err != nil {
			t.Fatal(err)
		}
		if view.Symbol != 9 || view.StartByte != 4 || view.EndByte != 5 || view.Extra || !view.Terminal {
			t.Fatalf("terminal provenance view=%+v", view)
		}
		if view.External {
			externalSeen = true
		} else {
			ordinarySeen = true
		}
	}
	if !ordinarySeen || !externalSeen {
		t.Fatalf("ordinary/external provenance collapsed: ordinary=%t external=%t", ordinarySeen, externalSeen)
	}
}

func TestExternalTerminalCohortSharesProvenance(t *testing.T) {
	tables := &fakeTable{actions: map[tableCell][]Action{
		{state: 1, symbol: 9}: {{Type: ActionShift, State: 3}},
		{state: 2, symbol: 9}: {{Type: ActionShift, State: 3}},
	}}
	compact, err := New(tables, Limits{MaxDerivations: 4, MaxPopPaths: 4})
	if err != nil {
		t.Fatal(err)
	}
	first, _ := compact.Seed(1, 4)
	second, _ := compact.Seed(2, 4)
	shifted, err := compact.ShiftOrdinaryCohort([]OrdinaryCohortShiftInput{
		{Head: first, ActionOrdinal: 0},
		{Head: second, ActionOrdinal: 0},
	}, 9, Token{Symbol: 9, StartByte: 4, EndByte: 5, External: true})
	if err != nil {
		t.Fatal(err)
	}
	var shared SubtreeID
	for _, head := range shifted {
		paths, err := compact.Derivations(head)
		if err != nil {
			t.Fatal(err)
		}
		for _, path := range paths {
			if len(path.Payloads) != 1 {
				t.Fatalf("external cohort path=%+v", path)
			}
			if shared == 0 {
				shared = path.Payloads[0]
			} else if path.Payloads[0] != shared {
				t.Fatalf("external cohort payload=%d, want shared %d", path.Payloads[0], shared)
			}
		}
	}
	view, err := compact.Subtree(shared)
	if err != nil {
		t.Fatal(err)
	}
	if !view.External || !view.Terminal || view.Extra {
		t.Fatalf("external cohort provenance=%+v", view)
	}
}

func TestExternalShiftCapRollsBackProvenance(t *testing.T) {
	tables := &fakeTable{actions: map[tableCell][]Action{
		{state: 1, symbol: 9}: {{Type: ActionShift, State: 3}},
	}}
	compact, err := New(tables, Limits{MaxNodes: 1})
	if err != nil {
		t.Fatal(err)
	}
	seed, err := compact.Seed(1, 4)
	if err != nil {
		t.Fatal(err)
	}
	before, _ := compact.Stats(seed)
	if _, err := compact.Shift(seed, 9, 0, Token{Symbol: 9, StartByte: 4, EndByte: 5, External: true}, ForkOrder{}); err == nil {
		t.Fatal("capped external shift unexpectedly succeeded")
	}
	after, _ := compact.Stats(seed)
	if after != before {
		t.Fatalf("capped external shift leaked provenance payload: before=%+v after=%+v", before, after)
	}
}

func TestShiftOrdinaryCohortValidatesBeforeMutation(t *testing.T) {
	tables := &fakeTable{actions: map[tableCell][]Action{
		{state: 1, symbol: 9}: {{Type: ActionShift, State: 3}},
		{state: 2, symbol: 9}: {{Type: ActionReduce, Symbol: 4}},
		{state: 3, symbol: 9}: {{Type: ActionShift, State: 4, ExtraChain: true}},
		{state: 4, symbol: 9}: {{Type: ActionShift, State: 5, Repetition: true}},
	}}
	compact, err := New(tables, Limits{MaxDerivations: 4, MaxPopPaths: 4})
	if err != nil {
		t.Fatal(err)
	}
	first, _ := compact.Seed(1, 4)
	second, _ := compact.Seed(2, 4)
	extraChain, _ := compact.Seed(3, 4)
	decoratedShift, _ := compact.Seed(4, 4)
	before, _ := compact.Stats(first)
	firstPaths, _ := compact.Derivations(first)
	secondPaths, _ := compact.Derivations(second)
	tests := []struct {
		name   string
		inputs []OrdinaryCohortShiftInput
		token  Token
	}{
		{name: "empty", token: Token{Symbol: 9, StartByte: 4, EndByte: 5}},
		{name: "duplicate-head", inputs: []OrdinaryCohortShiftInput{{Head: first}, {Head: first}}, token: Token{Symbol: 9, StartByte: 4, EndByte: 5}},
		{name: "incompatible-second-action", inputs: []OrdinaryCohortShiftInput{{Head: first}, {Head: second}}, token: Token{Symbol: 9, StartByte: 4, EndByte: 5}},
		{name: "extra-chain-action", inputs: []OrdinaryCohortShiftInput{{Head: first}, {Head: extraChain}}, token: Token{Symbol: 9, StartByte: 4, EndByte: 5}},
		{name: "decorated-shift-action", inputs: []OrdinaryCohortShiftInput{{Head: first}, {Head: decoratedShift}}, token: Token{Symbol: 9, StartByte: 4, EndByte: 5}},
		{name: "extra-token", inputs: []OrdinaryCohortShiftInput{{Head: first}}, token: Token{Symbol: 9, StartByte: 4, EndByte: 5, Extra: true}},
		{name: "zero-width-token", inputs: []OrdinaryCohortShiftInput{{Head: first}}, token: Token{Symbol: 9, StartByte: 4, EndByte: 4}},
		{name: "symbol-mismatch", inputs: []OrdinaryCohortShiftInput{{Head: first}}, token: Token{Symbol: 8, StartByte: 4, EndByte: 5}},
		{name: "invalid-head", inputs: []OrdinaryCohortShiftInput{{Head: first}, {Head: Head{Node: 999}}}, token: Token{Symbol: 9, StartByte: 4, EndByte: 5}},
		{name: "nonzero-ordinal", inputs: []OrdinaryCohortShiftInput{{Head: first, ActionOrdinal: 1}}, token: Token{Symbol: 9, StartByte: 4, EndByte: 5}},
		{name: "negative-ordinal", inputs: []OrdinaryCohortShiftInput{{Head: first, ActionOrdinal: -1}}, token: Token{Symbol: 9, StartByte: 4, EndByte: 5}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if _, err := compact.ShiftOrdinaryCohort(test.inputs, 9, test.token); err == nil {
				t.Fatal("invalid ordinary cohort shift succeeded")
			}
			after, _ := compact.Stats(first)
			gotFirst, _ := compact.Derivations(first)
			gotSecond, _ := compact.Derivations(second)
			if after != before || !reflect.DeepEqual(gotFirst, firstPaths) || !reflect.DeepEqual(gotSecond, secondPaths) {
				t.Fatalf("validation failure mutated core: before=%+v after=%+v first=%+v second=%+v", before, after, gotFirst, gotSecond)
			}
		})
	}
}

func TestShiftOrdinaryCohortRollsBackCaps(t *testing.T) {
	newCohort := func(t *testing.T, limits Limits) (*Core, Head, Head) {
		t.Helper()
		tables := &fakeTable{actions: map[tableCell][]Action{
			{state: 1, symbol: 9}: {{Type: ActionShift, State: 3}},
			{state: 2, symbol: 9}: {{Type: ActionShift, State: 3}},
		}}
		compact, err := New(tables, limits)
		if err != nil {
			t.Fatal(err)
		}
		first, err := compact.Seed(1, 4)
		if err != nil {
			t.Fatal(err)
		}
		second, err := compact.Seed(2, 4)
		if err != nil {
			t.Fatal(err)
		}
		return compact, first, second
	}
	assertRollback := func(t *testing.T, compact *Core, first, second Head) {
		t.Helper()
		before, _ := compact.Stats(first)
		firstPaths, _ := compact.Derivations(first)
		secondPaths, _ := compact.Derivations(second)
		if _, err := compact.ShiftOrdinaryCohort([]OrdinaryCohortShiftInput{{Head: first}, {Head: second}}, 9, Token{Symbol: 9, StartByte: 4, EndByte: 5}); err == nil {
			t.Fatal("capped ordinary cohort shift succeeded")
		}
		after, _ := compact.Stats(first)
		gotFirst, _ := compact.Derivations(first)
		gotSecond, _ := compact.Derivations(second)
		if after != before || !reflect.DeepEqual(gotFirst, firstPaths) || !reflect.DeepEqual(gotSecond, secondPaths) {
			t.Fatalf("cap failure mutated core: before=%+v after=%+v first=%+v second=%+v", before, after, gotFirst, gotSecond)
		}
		if _, ok := compact.CanonicalBoundary(3, 5, true, [32]byte{}); ok {
			t.Fatal("rolled-back cohort boundary remains published")
		}
	}

	t.Run("subtree", func(t *testing.T) {
		compact, first, second := newCohort(t, Limits{MaxSubtrees: 1, MaxDerivations: 4, MaxPopPaths: 4})
		if _, err := compact.appendSubtree(subtreeRecord{symbol: 8, terminal: true}, nil, nil, nil); err != nil {
			t.Fatal(err)
		}
		assertRollback(t, compact, first, second)
	})
	for _, test := range []struct {
		name   string
		limits Limits
	}{
		{name: "second-node", limits: Limits{MaxNodes: 3, MaxDerivations: 4, MaxPopPaths: 4}},
		{name: "second-link", limits: Limits{MaxLinks: 1, MaxDerivations: 4, MaxPopPaths: 4}},
		{name: "second-live-link", limits: Limits{MaxLinksPerBoundary: 1, MaxDerivations: 4, MaxPopPaths: 4}},
	} {
		t.Run(test.name, func(t *testing.T) {
			compact, first, second := newCohort(t, test.limits)
			assertRollback(t, compact, first, second)
		})
	}
}

func TestZeroChildReductionPushesParentAboveExistingExtra(t *testing.T) {
	tables := &fakeTable{
		actions: map[tableCell][]Action{{state: 3, symbol: 1}: {{Type: ActionReduce, Symbol: 2}}},
		gotos:   map[tableCell]StateID{{state: 3, symbol: 2}: 4},
	}
	core, err := New(tables, Limits{MaxDerivations: 4, MaxPopPaths: 4})
	if err != nil {
		t.Fatal(err)
	}
	head, _ := core.Seed(1, 0)
	head, err = core.appendDiagnosticPayload(head, 3, Token{Symbol: 10, EndByte: 1, Extra: true}, pathMeta{})
	if err != nil {
		t.Fatal(err)
	}
	frontier, err := core.Reduce(head, 1, 0, ForkOrder{})
	if err != nil || len(frontier) != 1 {
		t.Fatalf("zero-child reduce frontier=%v err=%v", frontier, err)
	}
	state, offset, err := core.Boundary(frontier[0])
	if err != nil || state != 4 || offset != 1 {
		t.Fatalf("zero-child boundary=(%d,%d) err=%v, want (4,1)", state, offset, err)
	}
	paths, err := core.Derivations(frontier[0])
	if err != nil || len(paths) != 1 || len(paths[0].Payloads) != 2 {
		t.Fatalf("zero-child derivations=%#v err=%v", paths, err)
	}
	parent, err := core.Subtree(paths[0].Payloads[1])
	if err != nil || parent.Symbol != 2 || parent.StartByte != 1 || parent.EndByte != 1 || len(parent.Children) != 0 {
		t.Fatalf("zero-child parent=%+v err=%v", parent, err)
	}
}

func TestReductionRepushesConsecutiveTrailingExtrasInOrder(t *testing.T) {
	core, err := New(diagnosticReduceTable(1, 3), Limits{MaxDerivations: 4, MaxPopPaths: 4})
	if err != nil {
		t.Fatal(err)
	}
	head, _ := core.Seed(1, 0)
	head, err = core.appendDiagnosticPayload(head, 3, Token{Symbol: 10, EndByte: 1}, pathMeta{ScoreDelta: 1, BranchOrder: ForkOrder{Present: true, Value: 7}})
	if err != nil {
		t.Fatal(err)
	}
	head, err = core.appendDiagnosticPayload(head, 3, Token{Symbol: 11, StartByte: 1, EndByte: 2, Extra: true}, pathMeta{ScoreDelta: 2, BranchOrder: ForkOrder{Present: true, Value: 8}})
	if err != nil {
		t.Fatal(err)
	}
	head, err = core.appendDiagnosticPayload(head, 3, Token{Symbol: 12, StartByte: 2, EndByte: 3, Extra: true}, pathMeta{ScoreDelta: 4, BranchOrder: ForkOrder{Present: true, Value: 9}})
	if err != nil {
		t.Fatal(err)
	}
	beforePaths, err := core.Derivations(head)
	if err != nil || len(beforePaths) != 1 || len(beforePaths[0].Payloads) != 3 {
		t.Fatalf("pre-reduction trailing-extra derivations=%#v err=%v", beforePaths, err)
	}
	wantExtraPayloads := append([]SubtreeID(nil), beforePaths[0].Payloads[1:]...)
	frontier, err := core.Reduce(head, 1, 0, ForkOrder{})
	if err != nil || len(frontier) != 1 {
		t.Fatalf("trailing-extra reduce frontier=%v err=%v", frontier, err)
	}
	state, offset, err := core.Boundary(frontier[0])
	if err != nil || state != 4 || offset != 3 {
		t.Fatalf("trailing-extra boundary=(%d,%d) err=%v, want (4,3)", state, offset, err)
	}
	paths, err := core.Derivations(frontier[0])
	if err != nil || len(paths) != 1 || len(paths[0].Payloads) != 3 {
		t.Fatalf("trailing-extra derivations=%#v err=%v", paths, err)
	}
	if paths[0].Score != 10 || !paths[0].HasBranchOrder || paths[0].BranchOrder != 9 {
		t.Fatalf("trailing-extra path metadata=%+v, want score=10 order=9", paths[0])
	}
	if !slices.Equal(paths[0].Payloads[1:], wantExtraPayloads) {
		t.Fatalf("re-pushed payload IDs=%v, want exact reuse of %v", paths[0].Payloads[1:], wantExtraPayloads)
	}
	parent, err := core.Subtree(paths[0].Payloads[0])
	if err != nil || parent.Symbol != 2 || parent.StartByte != 0 || parent.EndByte != 1 || len(parent.Children) != 1 {
		t.Fatalf("trailing-extra parent=%+v err=%v", parent, err)
	}
	for index, want := range []SubtreeView{
		{Symbol: 11, StartByte: 1, EndByte: 2, Extra: true, Terminal: true},
		{Symbol: 12, StartByte: 2, EndByte: 3, Extra: true, Terminal: true},
	} {
		got, err := core.Subtree(paths[0].Payloads[index+1])
		if err != nil || !reflect.DeepEqual(got, want) {
			t.Fatalf("re-pushed extra %d=%+v err=%v, want %+v", index, got, err, want)
		}
	}
}

func TestReduceSharesCompleteIdentityParentWithinBatch(t *testing.T) {
	compact, head := newSharedReductionFixture(t, true)
	before, _ := compact.Stats(head)
	frontier, err := compact.Reduce(head, 9, 0, ForkOrder{})
	if err != nil {
		t.Fatal(err)
	}
	if len(frontier) != 2 {
		t.Fatalf("shared reduction frontier=%d, want 2", len(frontier))
	}
	after, _ := compact.Stats(frontier[0])
	if after.Nodes-before.Nodes != 2 || after.Links-before.Links != 2 || after.Subtrees-before.Subtrees != 1 || after.Children-before.Children != 1 {
		t.Fatalf("shared reduction delta=%+v -> %+v, want N+2/L+2/S+1/children+1", before, after)
	}
	var parent SubtreeID
	for index, wantState := range []StateID{4, 5} {
		state, _, err := compact.Boundary(frontier[index])
		if err != nil || state != wantState {
			t.Fatalf("frontier %d state=%d err=%v, want %d", index, state, err, wantState)
		}
		paths, err := compact.Derivations(frontier[index])
		if err != nil || len(paths) != 1 || len(paths[0].Payloads) != 1 {
			t.Fatalf("frontier %d paths=%+v err=%v", index, paths, err)
		}
		if parent == 0 {
			parent = paths[0].Payloads[0]
		} else if paths[0].Payloads[0] != parent {
			t.Fatalf("frontier %d parent=%d, want shared %d", index, paths[0].Payloads[0], parent)
		}
	}
	second, err := compact.Reduce(head, 9, 0, ForkOrder{})
	if err != nil || len(second) != 2 {
		t.Fatalf("second reduction frontier=%v err=%v", second, err)
	}
	secondStats, _ := compact.Stats(second[0])
	if secondStats.Subtrees-after.Subtrees != 1 || secondStats.Children-after.Children != 1 {
		t.Fatalf("second Reduce reused across calls: first=%+v second=%+v", after, secondStats)
	}
	paths, _ := compact.Derivations(second[0])
	foundNew := false
	for _, path := range paths {
		foundNew = foundNew || path.Payloads[0] != parent
	}
	if len(paths) != 2 || !foundNew {
		t.Fatalf("second Reduce parent identity=%+v, want a new batch-local payload", paths)
	}
}

func TestReduceBatchSharingPreservesProspectiveLinkMultiplicity(t *testing.T) {
	for _, test := range []struct {
		name              string
		collidingMetadata bool
		wantSubtrees      uint32
	}{
		{name: "exact-link-collision-falls-back", collidingMetadata: true, wantSubtrees: 2},
		{name: "different-score-order-shares", collidingMetadata: false, wantSubtrees: 1},
	} {
		t.Run(test.name, func(t *testing.T) {
			compact, head := newReductionLinkCollisionFixture(t, test.collidingMetadata)
			before, _ := compact.Stats(head)
			frontier, err := compact.Reduce(head, 9, 0, ForkOrder{})
			if err != nil || len(frontier) != 1 {
				t.Fatalf("collision reduction frontier=%v err=%v", frontier, err)
			}
			after, _ := compact.Stats(frontier[0])
			paths, err := compact.Derivations(frontier[0])
			if err != nil || len(paths) != 2 || after.CurrentExactPaths != 2 {
				t.Fatalf("collision reduction paths=%+v stats=%+v err=%v", paths, after, err)
			}
			if after.Subtrees-before.Subtrees != test.wantSubtrees {
				t.Fatalf("collision=%t subtree delta=%d, want %d", test.collidingMetadata, after.Subtrees-before.Subtrees, test.wantSubtrees)
			}
			if test.collidingMetadata && paths[0].Payloads[0] == paths[1].Payloads[0] {
				t.Fatalf("colliding links reused payload %d and lost physical distinctness", paths[0].Payloads[0])
			}
			if !test.collidingMetadata && paths[0].Payloads[0] != paths[1].Payloads[0] {
				t.Fatalf("distinct score/order links failed to share parent: %+v", paths)
			}
		})
	}
}

func TestReduceDoesNotShareDifferentChildIdentity(t *testing.T) {
	compact, head := newSharedReductionFixture(t, false)
	before, _ := compact.Stats(head)
	frontier, err := compact.Reduce(head, 9, 0, ForkOrder{})
	if err != nil || len(frontier) != 2 {
		t.Fatalf("different-child reduction frontier=%v err=%v", frontier, err)
	}
	after, _ := compact.Stats(frontier[0])
	if after.Subtrees-before.Subtrees != 2 || after.Children-before.Children != 2 {
		t.Fatalf("different child IDs shared a parent: before=%+v after=%+v", before, after)
	}
}

func TestReductionParentIdentityIncludesScalarsFlagsAndOrderedSides(t *testing.T) {
	base := subtreeRecord{symbol: 2, productionID: 3, dynamicPrecedence: -4, startByte: 5, endByte: 6, extra: true, terminal: true}
	children := []SubtreeID{7, 8}
	fields := []FieldMapEntry{{FieldID: 9, ChildIndex: 1, Inherited: true}, {FieldID: 10}}
	aliases := []Symbol{10, 0, 11}
	if !reductionParentIdentityEqual(base, children, fields, aliases, base, slices.Clone(children), slices.Clone(fields), slices.Clone(aliases)) {
		t.Fatal("complete identical parent identity did not match")
	}
	recordCases := []subtreeRecord{
		{symbol: 12, productionID: 3, dynamicPrecedence: -4, startByte: 5, endByte: 6, extra: true, terminal: true},
		{symbol: 2, productionID: 12, dynamicPrecedence: -4, startByte: 5, endByte: 6, extra: true, terminal: true},
		{symbol: 2, productionID: 3, dynamicPrecedence: 12, startByte: 5, endByte: 6, extra: true, terminal: true},
		{symbol: 2, productionID: 3, dynamicPrecedence: -4, startByte: 12, endByte: 6, extra: true, terminal: true},
		{symbol: 2, productionID: 3, dynamicPrecedence: -4, startByte: 5, endByte: 12, extra: true, terminal: true},
		{symbol: 2, productionID: 3, dynamicPrecedence: -4, startByte: 5, endByte: 6, terminal: true},
		{symbol: 2, productionID: 3, dynamicPrecedence: -4, startByte: 5, endByte: 6, extra: true},
	}
	for index, candidate := range recordCases {
		if reductionParentIdentityEqual(base, children, fields, aliases, candidate, children, fields, aliases) {
			t.Fatalf("scalar/flag case %d matched", index)
		}
	}
	for index, candidate := range [][]SubtreeID{{7, 9}, {8, 7}, {7}} {
		if reductionParentIdentityEqual(base, children, fields, aliases, base, candidate, fields, aliases) {
			t.Fatalf("child identity case %d matched", index)
		}
	}
	fieldCases := [][]FieldMapEntry{
		{{FieldID: 11, ChildIndex: 1, Inherited: true}, {FieldID: 10}},
		{{FieldID: 9, ChildIndex: 0, Inherited: true}, {FieldID: 10}},
		{{FieldID: 9, ChildIndex: 1}, {FieldID: 10}},
		{{FieldID: 10}, {FieldID: 9, ChildIndex: 1, Inherited: true}},
		{{FieldID: 9, ChildIndex: 1, Inherited: true}},
	}
	for index, candidate := range fieldCases {
		if reductionParentIdentityEqual(base, children, fields, aliases, base, children, candidate, aliases) {
			t.Fatalf("field identity case %d matched", index)
		}
	}
	for index, candidate := range [][]Symbol{{10, 1, 11}, {11, 0, 10}, {10, 0}, {10, 11}} {
		if reductionParentIdentityEqual(base, children, fields, aliases, base, children, fields, candidate) {
			t.Fatalf("alias identity case %d matched", index)
		}
	}
}

func TestReduceBatchParentSharingRollsBackCaps(t *testing.T) {
	for _, name := range []string{"subtree", "children", "second-node", "second-link"} {
		t.Run(name, func(t *testing.T) {
			compact, head := newSharedReductionFixture(t, true)
			before, _ := compact.Stats(head)
			beforeBoundaries := cloneBoundaryMap(compact.boundaries)
			beforePaths, _ := compact.Derivations(head)
			switch name {
			case "subtree":
				compact.limits.MaxSubtrees = before.Subtrees
			case "children":
				compact.limits.MaxChildren = before.Children
			case "second-node":
				compact.limits.MaxNodes = before.Nodes + 1
			case "second-link":
				compact.limits.MaxLinks = before.Links + 1
			}
			if _, err := compact.Reduce(head, 9, 0, ForkOrder{}); err == nil || !strings.Contains(err.Error(), "cap") {
				t.Fatalf("%s cap error=%v", name, err)
			}
			after, _ := compact.Stats(head)
			afterBoundaries := cloneBoundaryMap(compact.boundaries)
			afterPaths, _ := compact.Derivations(head)
			if after != before || !reflect.DeepEqual(afterBoundaries, beforeBoundaries) || !reflect.DeepEqual(afterPaths, beforePaths) {
				t.Fatalf("%s cap mutated shared reduction: before=%+v after=%+v paths=%+v", name, before, after, afterPaths)
			}
		})
	}
}

func cloneBoundaryMap(source map[boundaryKey]NodeID) map[boundaryKey]NodeID {
	clone := make(map[boundaryKey]NodeID, len(source))
	for key, id := range source {
		clone[key] = id
	}
	return clone
}

func newSharedReductionFixture(t *testing.T, sameChild bool) (*Core, Head) {
	t.Helper()
	tables := &fakeTable{
		actions: map[tableCell][]Action{{state: 3, symbol: 9}: {{Type: ActionReduce, Symbol: 2, ChildCount: 1, ProductionID: 6}}},
		gotos: map[tableCell]StateID{
			{state: 1, symbol: 2}: 4,
			{state: 2, symbol: 2}: 5,
		},
		fields:  map[uint16][]FieldMapEntry{6: {{FieldID: 7, ChildIndex: 0}}},
		aliases: map[productionKey][]Symbol{{productionID: 6, childCount: 1}: {8}},
	}
	compact, err := New(tables, Limits{MaxDerivations: 4, MaxPopPaths: 4})
	if err != nil {
		t.Fatal(err)
	}
	compact.diagnostics.foldSamePredecessorShallowPayloads = false
	first, _ := compact.Seed(1, 0)
	second, _ := compact.Seed(2, 0)
	firstChild, err := compact.appendSubtree(subtreeRecord{symbol: 10, startByte: 0, endByte: 1, terminal: true}, nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	secondChild := firstChild
	if !sameChild {
		secondChild, err = compact.appendSubtree(subtreeRecord{symbol: 10, startByte: 0, endByte: 1, terminal: true}, nil, nil, nil)
		if err != nil {
			t.Fatal(err)
		}
	}
	head, err := compact.condense(compact.boundaryKey(3, 1), linkInput{prev: first.Node, payload: firstChild})
	if err != nil {
		t.Fatal(err)
	}
	head, err = compact.condense(compact.boundaryKey(3, 1), linkInput{prev: second.Node, payload: secondChild})
	if err != nil {
		t.Fatal(err)
	}
	return compact, head
}

func newReductionLinkCollisionFixture(t *testing.T, collidingMetadata bool) (*Core, Head) {
	t.Helper()
	tables := &fakeTable{
		actions: map[tableCell][]Action{{state: 3, symbol: 9}: {{Type: ActionReduce, Symbol: 2, ChildCount: 2}}},
		gotos:   map[tableCell]StateID{{state: 1, symbol: 2}: 4},
	}
	compact, err := New(tables, Limits{MaxDerivations: 4, MaxPopPaths: 4})
	if err != nil {
		t.Fatal(err)
	}
	compact.diagnostics.foldSamePredecessorShallowPayloads = false
	seed, _ := compact.Seed(1, 0)
	firstChild, _ := compact.appendSubtree(subtreeRecord{symbol: 10, endByte: 1, terminal: true}, nil, nil, nil)
	secondChild, _ := compact.appendSubtree(subtreeRecord{symbol: 11, startByte: 1, endByte: 2, terminal: true}, nil, nil, nil)
	firstMid, err := compact.appendPrivate(2, 1, linkInput{
		prev: seed.Node, payload: firstChild, scoreDelta: 1, order: ForkOrder{Present: true, Value: 7},
	})
	if err != nil {
		t.Fatal(err)
	}
	secondMid, err := compact.appendPrivate(2, 1, linkInput{
		prev: seed.Node, payload: firstChild, scoreDelta: 2, order: ForkOrder{Present: true, Value: 8},
	})
	if err != nil {
		t.Fatal(err)
	}
	secondScore := int64(3)
	secondOrder := ForkOrder{Present: true, Value: 9}
	if collidingMetadata {
		secondScore = 0
		secondOrder = ForkOrder{Present: true, Value: 7}
	}
	head, err := compact.condense(compact.boundaryKey(3, 2), linkInput{
		prev: firstMid.Node, payload: secondChild, scoreDelta: 1, order: ForkOrder{Present: true, Value: 7},
	})
	if err != nil {
		t.Fatal(err)
	}
	head, err = compact.condense(compact.boundaryKey(3, 2), linkInput{
		prev: secondMid.Node, payload: secondChild, scoreDelta: secondScore, order: secondOrder,
	})
	if err != nil {
		t.Fatal(err)
	}
	return compact, head
}

func TestConvergedReductionPathsCondenseOnlyAfterTrailingExtraRepush(t *testing.T) {
	tables := &fakeTable{
		actions: map[tableCell][]Action{{state: 3, symbol: 1}: {{Type: ActionReduce, Symbol: 2, ChildCount: 2}}},
		gotos:   map[tableCell]StateID{{state: 1, symbol: 2}: 4},
	}
	core, err := New(tables, Limits{MaxDerivations: 2, MaxPopPaths: 2})
	if err != nil {
		t.Fatal(err)
	}
	core.diagnostics.foldSamePredecessorShallowPayloads = false
	seed, _ := core.Seed(1, 0)
	head, err := core.appendDiagnosticPayload(seed, 2, Token{Symbol: 10, EndByte: 1}, pathMeta{ScoreDelta: 1, BranchOrder: ForkOrder{Present: true, Value: 7}})
	if err != nil {
		t.Fatal(err)
	}
	head, err = core.appendDiagnosticPayload(seed, 2, Token{Symbol: 10, EndByte: 1}, pathMeta{ScoreDelta: 2, BranchOrder: ForkOrder{Present: true, Value: 8}})
	if err != nil {
		t.Fatal(err)
	}
	head, err = core.appendDiagnosticPayload(head, 2, Token{Symbol: 11, StartByte: 1, EndByte: 2, Extra: true}, pathMeta{ScoreDelta: 3, BranchOrder: ForkOrder{Present: true, Value: 9}})
	if err != nil {
		t.Fatal(err)
	}
	head, err = core.appendDiagnosticPayload(head, 3, Token{Symbol: 12, StartByte: 2, EndByte: 3}, pathMeta{ScoreDelta: 4, BranchOrder: ForkOrder{Present: true, Value: 10}})
	if err != nil {
		t.Fatal(err)
	}
	head, err = core.appendDiagnosticPayload(head, 3, Token{Symbol: 13, StartByte: 3, EndByte: 4, Extra: true}, pathMeta{ScoreDelta: 5, BranchOrder: ForkOrder{Present: true, Value: 11}})
	if err != nil {
		t.Fatal(err)
	}
	before, _ := core.Stats(head)
	frontier, err := core.Reduce(head, 1, 0, ForkOrder{})
	if err != nil || len(frontier) != 1 {
		t.Fatalf("converged trailing-extra frontier=%v err=%v", frontier, err)
	}
	stats, err := core.Stats(frontier[0])
	if err != nil || stats.CurrentExactPaths != 2 {
		t.Fatalf("converged trailing-extra stats=%+v err=%v, want exactly 2 paths", stats, err)
	}
	if stats.Subtrees-before.Subtrees != 2 {
		t.Fatalf("trailing-extra reduction shared batch parents: before=%+v after=%+v", before, stats)
	}
	paths, err := core.Derivations(frontier[0])
	if err != nil || len(paths) != 2 {
		t.Fatalf("converged trailing-extra derivations=%#v err=%v", paths, err)
	}
	if paths[0].Score != 13 || paths[1].Score != 14 || paths[0].BranchOrder != 11 || paths[1].BranchOrder != 11 {
		t.Fatalf("converged interstitial/trailing-extra metadata=%#v, want scores 13/14 and order 11", paths)
	}

	rollback, err := New(tables, Limits{MaxDerivations: 2, MaxPopPaths: 2})
	if err != nil {
		t.Fatal(err)
	}
	rollback.diagnostics.foldSamePredecessorShallowPayloads = false
	rollbackSeed, _ := rollback.Seed(1, 0)
	rollbackHead, _ := rollback.appendDiagnosticPayload(rollbackSeed, 2, Token{Symbol: 10, EndByte: 1}, pathMeta{ScoreDelta: 1})
	rollbackHead, _ = rollback.appendDiagnosticPayload(rollbackSeed, 2, Token{Symbol: 10, EndByte: 1}, pathMeta{ScoreDelta: 2})
	rollbackHead, _ = rollback.appendDiagnosticPayload(rollbackHead, 2, Token{Symbol: 11, StartByte: 1, EndByte: 2, Extra: true}, pathMeta{ScoreDelta: 3})
	rollbackHead, _ = rollback.appendDiagnosticPayload(rollbackHead, 3, Token{Symbol: 12, StartByte: 2, EndByte: 3}, pathMeta{ScoreDelta: 4})
	rollbackHead, _ = rollback.appendDiagnosticPayload(rollbackHead, 3, Token{Symbol: 13, StartByte: 3, EndByte: 4, Extra: true}, pathMeta{ScoreDelta: 5})
	rollbackBefore, _ := rollback.Stats(rollbackHead)
	rollback.limits.MaxPopPaths = 1
	if _, err := rollback.Reduce(rollbackHead, 1, 0, ForkOrder{}); err == nil || !strings.Contains(err.Error(), "cap") {
		t.Fatalf("converged trailing-extra cap error=%v", err)
	}
	rollbackAfter, _ := rollback.Stats(rollbackHead)
	if rollbackAfter != rollbackBefore {
		t.Fatalf("failed trailing-extra reduction mutated core: before=%+v after=%+v", rollbackBefore, rollbackAfter)
	}
	if before.CurrentExactPaths != 2 {
		t.Fatalf("fixture input paths=%d, want 2", before.CurrentExactPaths)
	}
}

func TestReductionOwnsInterstitialExtrasAndRemapsStructuralMetadata(t *testing.T) {
	tables := &fakeTable{
		actions: map[tableCell][]Action{{state: 3, symbol: 1}: {{
			Type: ActionReduce, Symbol: 20, ChildCount: 3, DynamicPrecedence: 2, ProductionID: 5,
		}}},
		gotos: map[tableCell]StateID{{state: 1, symbol: 20}: 4},
		fields: map[uint16][]FieldMapEntry{5: {
			{FieldID: 1, ChildIndex: 0},
			{FieldID: 2, ChildIndex: 1, Inherited: true},
			{FieldID: 3, ChildIndex: 2},
		}},
		aliases: map[productionKey][]Symbol{{productionID: 5, childCount: 3}: {101, 102, 103}},
	}
	core, err := New(tables, Limits{MaxDerivations: 4, MaxPopPaths: 4})
	if err != nil {
		t.Fatal(err)
	}
	head, _ := core.Seed(1, 0)
	inputs := []struct {
		state StateID
		token Token
		score int64
	}{
		{state: 1, token: Token{Symbol: 30, EndByte: 1, Extra: true}, score: 10},
		{state: 2, token: Token{Symbol: 31, StartByte: 5, EndByte: 6}, score: 1},
		{state: 2, token: Token{Symbol: 32, StartByte: 6, EndByte: 7, Extra: true}, score: 2},
		{state: 2, token: Token{Symbol: 33, StartByte: 7, EndByte: 8, Extra: true}, score: 3},
		{state: 2, token: Token{Symbol: 34, StartByte: 8, EndByte: 9}, score: 4},
		{state: 3, token: Token{Symbol: 35, StartByte: 9, EndByte: 10}, score: 5},
		{state: 3, token: Token{Symbol: 36, StartByte: 10, EndByte: 11, Extra: true}, score: 6},
		{state: 3, token: Token{Symbol: 37, StartByte: 11, EndByte: 12, Extra: true}, score: 7},
	}
	for index, input := range inputs {
		head, err = core.appendDiagnosticPayload(head, input.state, input.token, pathMeta{
			ScoreDelta: input.score, BranchOrder: ForkOrder{Present: true, Value: uint64(index + 1)},
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	beforePaths, err := core.Derivations(head)
	if err != nil || len(beforePaths) != 1 || len(beforePaths[0].Payloads) != len(inputs) {
		t.Fatalf("interstitial fixture derivations=%#v err=%v", beforePaths, err)
	}
	inputPayloads := append([]SubtreeID(nil), beforePaths[0].Payloads...)
	frontier, err := core.Reduce(head, 1, 0, ForkOrder{})
	if err != nil || len(frontier) != 1 {
		t.Fatalf("interstitial reduction frontier=%v err=%v", frontier, err)
	}
	state, offset, err := core.Boundary(frontier[0])
	if err != nil || state != 4 || offset != 12 {
		t.Fatalf("interstitial boundary=(%d,%d) err=%v, want (4,12)", state, offset, err)
	}
	paths, err := core.Derivations(frontier[0])
	if err != nil || len(paths) != 1 || len(paths[0].Payloads) != 4 {
		t.Fatalf("interstitial result derivations=%#v err=%v", paths, err)
	}
	if paths[0].Score != 40 || !paths[0].HasBranchOrder || paths[0].BranchOrder != 8 {
		t.Fatalf("interstitial result metadata=%+v, want score=40 order=8", paths[0])
	}
	if paths[0].Payloads[0] != inputPayloads[0] || !slices.Equal(paths[0].Payloads[2:], inputPayloads[6:]) {
		t.Fatalf("leading/trailing extra payload reuse=%v, input=%v", paths[0].Payloads, inputPayloads)
	}
	parent, err := core.Subtree(paths[0].Payloads[1])
	if err != nil {
		t.Fatal(err)
	}
	if parent.Symbol != 20 || parent.StartByte != 5 || parent.EndByte != 10 || !slices.Equal(parent.Children, inputPayloads[1:6]) {
		t.Fatalf("interstitial parent identity=%+v, input payloads=%v", parent, inputPayloads)
	}
	wantFields := []FieldMapEntry{
		{FieldID: 1, ChildIndex: 0},
		{FieldID: 2, ChildIndex: 3, Inherited: true},
		{FieldID: 3, ChildIndex: 4},
	}
	if !reflect.DeepEqual(parent.Fields, wantFields) {
		t.Fatalf("remapped fields=%+v, want %+v", parent.Fields, wantFields)
	}
	if !slices.Equal(parent.Aliases, []Symbol{101, 0, 0, 102, 103}) {
		t.Fatalf("remapped aliases=%v, want [101 0 0 102 103]", parent.Aliases)
	}
	for _, field := range parent.Fields {
		child, err := core.Subtree(parent.Children[field.ChildIndex])
		if err != nil || child.Extra {
			t.Fatalf("direct field %+v attached to extra child %+v err=%v", field, child, err)
		}
	}
}

func TestReductionMetadataRemapFailuresRollback(t *testing.T) {
	tests := []struct {
		name    string
		fields  []FieldMapEntry
		aliases []Symbol
	}{
		{name: "field structural index", fields: []FieldMapEntry{{FieldID: 1, ChildIndex: 2}}},
		{name: "alias structural count", aliases: []Symbol{10, 11}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			tables := &fakeTable{
				actions: map[tableCell][]Action{{state: 3, symbol: 1}: {{
					Type: ActionReduce, Symbol: 2, ChildCount: 1, ProductionID: 5,
				}}},
				gotos:   map[tableCell]StateID{{state: 1, symbol: 2}: 4},
				fields:  map[uint16][]FieldMapEntry{5: test.fields},
				aliases: map[productionKey][]Symbol{{productionID: 5, childCount: 1}: test.aliases},
			}
			core, err := New(tables, Limits{MaxDerivations: 2, MaxPopPaths: 2})
			if err != nil {
				t.Fatal(err)
			}
			head, _ := core.Seed(1, 0)
			head, _ = core.appendDiagnosticPayload(head, 3, Token{Symbol: 10, EndByte: 1}, pathMeta{})
			before, _ := core.Stats(head)
			if _, err := core.Reduce(head, 1, 0, ForkOrder{}); err == nil {
				t.Fatal("invalid production metadata was admitted")
			}
			after, _ := core.Stats(head)
			if after != before {
				t.Fatalf("metadata remap failure mutated core: before=%+v after=%+v", before, after)
			}
		})
	}
}

func TestReductionFieldRemapPastUint8RollsBack(t *testing.T) {
	tables := &fakeTable{
		actions: map[tableCell][]Action{{state: 3, symbol: 1}: {{
			Type: ActionReduce, Symbol: 2, ChildCount: 2, ProductionID: 5,
		}}},
		gotos:  map[tableCell]StateID{{state: 1, symbol: 2}: 4},
		fields: map[uint16][]FieldMapEntry{5: {{FieldID: 1, ChildIndex: 1}}},
	}
	core, err := New(tables, Limits{
		MaxNodes: 1024, MaxLinks: 1024, MaxSubtrees: 1024,
		MaxDerivations: 2, MaxPopPaths: 2,
	})
	if err != nil {
		t.Fatal(err)
	}
	head, _ := core.Seed(1, 0)
	head, _ = core.appendDiagnosticPayload(head, 2, Token{Symbol: 10, EndByte: 1}, pathMeta{})
	for index := 0; index < 256; index++ {
		start := uint32(index + 1)
		head, err = core.appendDiagnosticPayload(head, 2, Token{Symbol: 11, StartByte: start, EndByte: start + 1, Extra: true}, pathMeta{})
		if err != nil {
			t.Fatal(err)
		}
	}
	head, _ = core.appendDiagnosticPayload(head, 3, Token{Symbol: 12, StartByte: 257, EndByte: 258}, pathMeta{})
	before, _ := core.Stats(head)
	if _, err := core.Reduce(head, 1, 0, ForkOrder{}); err == nil || !strings.Contains(err.Error(), "uint8") {
		t.Fatalf("wide field remap error=%v", err)
	}
	after, _ := core.Stats(head)
	if after != before {
		t.Fatalf("wide field remap failure mutated core: before=%+v after=%+v", before, after)
	}
}

func TestReduceScoreOverflowRollsBack(t *testing.T) {
	core, err := New(diagnosticReduceTable(1, 1), Limits{MaxDerivations: 4, MaxPopPaths: 4})
	if err != nil {
		t.Fatal(err)
	}
	head, _ := core.Seed(1, 0)
	head, err = core.appendDiagnosticPayload(head, 3, Token{Symbol: 1, EndByte: 1}, pathMeta{ScoreDelta: math.MaxInt64})
	if err != nil {
		t.Fatal(err)
	}
	before, _ := core.Stats(head)
	if _, err := core.Reduce(head, 1, 0, ForkOrder{}); err == nil || !strings.Contains(err.Error(), "score overflow") {
		t.Fatalf("Reduce overflow error = %v", err)
	}
	after, _ := core.Stats(head)
	if after != before {
		t.Fatalf("Reduce overflow did not roll back: before=%+v after=%+v", before, after)
	}
}

func TestReductionReturnsEveryDistinctGotoBoundary(t *testing.T) {
	tables := &fakeTable{
		actions: map[tableCell][]Action{
			{state: 3, symbol: 1}: {{Type: ActionReduce, Symbol: 2, ChildCount: 1}},
		},
		gotos: map[tableCell]StateID{
			{state: 1, symbol: 2}: 4,
			{state: 2, symbol: 2}: 5,
		},
	}
	core, err := New(tables, Limits{MaxDerivations: 4, MaxPopPaths: 4})
	if err != nil {
		t.Fatal(err)
	}
	left, _ := core.Seed(1, 0)
	right, _ := core.Seed(2, 0)
	left, err = core.appendDiagnosticPayload(left, 3, Token{Symbol: 1, EndByte: 1}, pathMeta{BranchOrder: ForkOrder{Present: true, Value: 1}})
	if err != nil {
		t.Fatal(err)
	}
	merged, err := core.appendDiagnosticPayload(right, 3, Token{Symbol: 1, EndByte: 1}, pathMeta{BranchOrder: ForkOrder{Present: true, Value: 2}})
	if err != nil {
		t.Fatal(err)
	}
	_ = left // merged is the immutable replacement containing both paths.
	frontier, err := core.Reduce(merged, 1, 0, ForkOrder{})
	if err != nil {
		t.Fatal(err)
	}
	if len(frontier) != 2 {
		t.Fatalf("reduction frontier has %d boundaries, want 2", len(frontier))
	}
	states := make([]StateID, 0, 2)
	for _, head := range frontier {
		n, _ := core.node(head.Node)
		states = append(states, n.state)
	}
	slices.Sort(states)
	if !slices.Equal(states, []StateID{4, 5}) {
		t.Fatalf("reduction frontier states = %v, want [4 5]", states)
	}
}

func TestCompactArenaRecordsRemainPointerFree(t *testing.T) {
	for name, typ := range map[string]reflect.Type{
		"node":    reflect.TypeFor[nodeRecord](),
		"link":    reflect.TypeFor[linkRecord](),
		"subtree": reflect.TypeFor[subtreeRecord](),
	} {
		for i := 0; i < typ.NumField(); i++ {
			kind := typ.Field(i).Type.Kind()
			if kind == reflect.Pointer || kind == reflect.Slice || kind == reflect.Map || kind == reflect.Interface || kind == reflect.String {
				t.Fatalf("%s record field %s is GC-visible %s", name, typ.Field(i).Name, kind)
			}
		}
	}
	if got := unsafe.Sizeof(nodeRecord{}); got > 32 {
		t.Fatalf("nodeRecord size = %d, want <= 32", got)
	}
	if got := unsafe.Sizeof(linkRecord{}); got > 32 {
		t.Fatalf("linkRecord size = %d, want <= 32", got)
	}
	if got := unsafe.Sizeof(subtreeRecord{}); got > 64 {
		t.Fatalf("subtreeRecord size = %d, want <= 64", got)
	}
}

func TestCycleAndOverflowDeclineFailClosed(t *testing.T) {
	t.Run("cycle", func(t *testing.T) {
		core := newTinyCore(t, 2)
		seed, _ := core.Seed(1, 0)
		payload, _ := core.appendSubtree(subtreeRecord{symbol: 1, terminal: true}, nil, nil, nil)
		head, err := core.condense(core.boundaryKey(2, 0), linkInput{prev: seed.Node, payload: payload})
		if err != nil {
			t.Fatal(err)
		}
		n, _ := core.node(head.Node)
		core.links[n.firstLink-1].prev = head.Node // adversarial arena corruption
		if _, err := core.Derivations(head); err == nil || !strings.Contains(err.Error(), "cycle") {
			t.Fatalf("Derivations cycle error = %v", err)
		}
	})

	t.Run("path_count", func(t *testing.T) {
		core := newTinyCoreWithLimits(t, Limits{MaxDerivations: math.MaxUint64, MaxPopPaths: math.MaxUint64})
		seedA, _ := core.Seed(1, 0)
		seedB, _ := core.Seed(2, 0)
		payload, _ := core.appendSubtree(subtreeRecord{symbol: 1, terminal: true}, nil, nil, nil)
		key := core.boundaryKey(3, 0)
		if _, err := core.condense(key, linkInput{prev: seedA.Node, payload: payload}); err != nil {
			t.Fatal(err)
		}
		b, _ := core.node(seedB.Node)
		b.pathCount = math.MaxUint64
		head, err := core.condense(key, linkInput{prev: seedB.Node, payload: payload})
		if err != nil {
			t.Fatalf("path-count overflow changed execution: %v", err)
		}
		stats, err := core.Stats(head)
		if err != nil || stats.CurrentExactPaths != math.MaxUint64 {
			t.Fatalf("saturated path telemetry=%+v err=%v", stats, err)
		}
	})

	t.Run("score", func(t *testing.T) {
		core := newTinyCore(t, 2)
		seed, _ := core.Seed(1, 0)
		payload, _ := core.appendSubtree(subtreeRecord{symbol: 1, terminal: true}, nil, nil, nil)
		head, err := core.condense(core.boundaryKey(2, 0), linkInput{prev: seed.Node, payload: payload, scoreDelta: math.MaxInt64})
		if err != nil {
			t.Fatal(err)
		}
		head, err = core.condense(core.boundaryKey(3, 0), linkInput{prev: head.Node, payload: payload, scoreDelta: 1})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := core.Derivations(head); err == nil || !strings.Contains(err.Error(), "score overflow") {
			t.Fatalf("score overflow error = %v", err)
		}
	})

	t.Run("max_branch_order_is_scalar_not_arithmetic", func(t *testing.T) {
		core := newTinyCore(t, 2)
		seed, _ := core.Seed(1, 0)
		payload, _ := core.appendSubtree(subtreeRecord{symbol: 1, terminal: true}, nil, nil, nil)
		head, err := core.condense(core.boundaryKey(2, 0), linkInput{prev: seed.Node, payload: payload, order: ForkOrder{Present: true, Value: math.MaxUint64}})
		if err != nil {
			t.Fatal(err)
		}
		paths, err := core.Derivations(head)
		if err != nil || len(paths) != 1 || paths[0].BranchOrder != math.MaxUint64 {
			t.Fatalf("max scalar branch order = %#v, %v", paths, err)
		}
	})
}

func newTinyCore(t *testing.T, pathCap uint64) *Core {
	t.Helper()
	return newTinyCoreWithLimits(t, Limits{MaxDerivations: pathCap, MaxPopPaths: pathCap})
}

func newTinyCoreWithLimits(t *testing.T, limits Limits) *Core {
	t.Helper()
	core, err := New(&fakeTable{}, limits)
	if err != nil {
		t.Fatal(err)
	}
	return core
}

func diagnosticReduceTable(childCount uint8, dynamicPrecedence int16) *fakeTable {
	return &fakeTable{
		actions: map[tableCell][]Action{
			{state: 3, symbol: 1}: {{
				Type: ActionReduce, Symbol: 2, ChildCount: childCount, DynamicPrecedence: dynamicPrecedence,
			}},
		},
		gotos: map[tableCell]StateID{{state: 1, symbol: 2}: 4},
	}
}
