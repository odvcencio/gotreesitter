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

type identityTestTable struct {
	fakeTable
	identity [32]byte
	valid    bool
}

func (t *identityTestTable) TableIdentity() ([32]byte, bool) {
	if t == nil {
		return [32]byte{}, false
	}
	return t.identity, t.valid
}

func TestCoreTableIdentityCapturesAndRejectsProducerDrift(t *testing.T) {
	tables := &identityTestTable{identity: [32]byte{1}, valid: true}
	compact, err := New(tables, Limits{})
	if err != nil {
		t.Fatal(err)
	}
	if !compact.TableIdentityMatches() {
		t.Fatal("matching table identity was rejected")
	}
	tables.identity[0] = 2
	if compact.TableIdentityMatches() {
		t.Fatal("changed table identity was accepted")
	}
	legacy, err := New(&fakeTable{}, Limits{})
	if err != nil {
		t.Fatal(err)
	}
	if legacy.TableIdentityMatches() {
		t.Fatal("table without identity was accepted for replay")
	}
}

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
	if !reflect.DeepEqual(actionRowValues(got), wantConflict) {
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
	if !reflect.DeepEqual(actionRowValues(reduceActions), wantReduceCell) {
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
	borrowed, err := core.MaterializationView(paths[0].Payloads[0])
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(borrowed.Aliases, []Symbol{229}) {
		t.Fatalf("borrowed aliases = %#v, want pinned [229]", borrowed.Aliases)
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
	_, err = core.condense(key, linkInput{prev: seed.Node, payload: payload, scoreDelta: 8})
	var capacity *LiveLinkCapacityError
	if !errors.As(err, &capacity) {
		t.Fatalf("ninth distinct live link error=%v, want *LiveLinkCapacityError", err)
	}
	if want := (&LiveLinkCapacityError{State: 2, ByteOffset: 1, ObservedLinks: 9, Limit: 8}); *capacity != *want || err.Error() != "parser-core phase zero: shared (2,1) live-link cap exceeded: 9 > 8" {
		t.Fatalf("ninth distinct live link error=%+v text=%q, want=%+v", capacity, err, want)
	}
	after, err := core.Stats(head)
	if err != nil || after != before {
		t.Fatalf("ninth-link failure mutated core: before=%+v after=%+v err=%v", before, after, err)
	}
	canonical, ok := core.CanonicalBoundary(2, 1, false, 0)
	if !ok || canonical != head {
		t.Fatalf("ninth-link failure changed canonical boundary: head=%+v canonical=%+v ok=%t", head, canonical, ok)
	}
}

func TestLiveCondenseCandidatesDropsSequentialVersionHistory(t *testing.T) {
	compact := newTinyCoreWithLimits(t, Limits{MaxLinksPerBoundary: 2})
	compact.diagnostics.foldSamePredecessorShallowPayloads = false
	seed, _ := compact.Seed(1, 0)
	payload, _ := compact.appendSubtree(subtreeRecord{symbol: 1, terminal: true, endByte: 1}, nil, nil, nil)
	key := compact.boundaryKey(2, 1)
	var first, second Head
	var secondOutcome condenseOutcome
	err := compact.ApplySchedulerAtomic(func(owner SchedulerTransactionToken) error {
		var err error
		first, err = compact.condense(key, linkInput{prev: seed.Node, payload: payload, scoreDelta: 1})
		if err != nil {
			return err
		}
		first, err = compact.condense(key, linkInput{prev: seed.Node, payload: payload, scoreDelta: 2})
		if err != nil {
			return err
		}
		if err := compact.RecordReductionLineageOwned(owner, []ReductionOutput{{
			Head: first, CleanPathRank: CleanPathRankSelected, MultiplePopPaths: true,
		}}, 7); err != nil {
			return err
		}
		return compact.runLiveCondenseCandidates(nil, func() error {
			secondOutcome, err = compact.condenseWithOutcome(key, linkInput{prev: seed.Node, payload: payload, scoreDelta: 3})
			second = secondOutcome.head
			return err
		})
	})
	if err != nil {
		t.Fatal(err)
	}
	if first == second {
		t.Fatalf("sequential versions share one physical head: %+v", first)
	}
	if !secondOutcome.historicalBoundarySplit {
		t.Fatal("sequential version replacement lost historical boundary split provenance")
	}
	if !secondOutcome.historicalConvergedSplit {
		t.Fatal("historical converged version lost its split provenance")
	}
	if secondOutcome.historicalCleanPathRank != CleanPathRankSelected ||
		secondOutcome.historicalLineage != 7 {
		t.Fatalf(
			"historical lineage = (%v,%d), want (%v,7)",
			secondOutcome.historicalCleanPathRank,
			secondOutcome.historicalLineage,
			CleanPathRankSelected,
		)
	}
	record, err := compact.node(second.Node)
	if err != nil {
		t.Fatal(err)
	}
	if record.linkCount != 1 {
		t.Fatalf("second sequential version links = %d, want 1", record.linkCount)
	}
}

func TestReductionLineageRollsBackWithSchedulerTransaction(t *testing.T) {
	compact := newTinyCore(t, 4)
	head, err := compact.Seed(1, 0)
	if err != nil {
		t.Fatal(err)
	}
	outputs := []ReductionOutput{{
		Head: head, CleanPathRank: CleanPathRankUnselected, MultiplePopPaths: true,
	}}
	sentinel := errors.New("rollback lineage")
	err = compact.ApplySchedulerAtomic(func(owner SchedulerTransactionToken) error {
		if err := compact.RecordReductionLineageOwned(owner, outputs, 11); err != nil {
			return err
		}
		return sentinel
	})
	if !errors.Is(err, sentinel) {
		t.Fatalf("rollback error = %v, want %v", err, sentinel)
	}
	provenance, err := compact.nodeLineage(head.Node)
	if err != nil {
		t.Fatal(err)
	}
	// The alternative set's rollback restores count/flags/spillRef only
	// (spec.b4b-alternative-set.v1 section 3.3); an inline slot the rolled-
	// back insert wrote is deliberately left as unread stale data beyond the
	// restored count, so compare membership (Len), not raw struct equality.
	if provenance.owner != 0 || provenance.lineage != 0 || provenance.rank != CleanPathRankNotApplicable ||
		provenance.converged || provenance.set.Len() != 0 {
		t.Fatalf("rolled-back lineage = %+v, want zero", *provenance)
	}
	err = compact.ApplySchedulerAtomic(func(owner SchedulerTransactionToken) error {
		return compact.RecordReductionLineageOwned(owner, outputs, 12)
	})
	if err != nil {
		t.Fatal(err)
	}
	provenance, err = compact.nodeLineage(head.Node)
	if err != nil {
		t.Fatal(err)
	}
	if provenance.rank != CleanPathRankUnselected || provenance.lineage != 12 || !provenance.converged {
		t.Fatalf("committed lineage = %+v, want unselected lineage 12", *provenance)
	}
}

func TestHeadOwnerRollsBackWithSchedulerTransaction(t *testing.T) {
	compact := newTinyCore(t, 4)
	head, err := compact.Seed(1, 0)
	if err != nil {
		t.Fatal(err)
	}
	sentinel := errors.New("rollback owner")
	err = compact.ApplySchedulerAtomic(func(owner SchedulerTransactionToken) error {
		if err := compact.RecordHeadOwnerOwned(owner, head, 7); err != nil {
			return err
		}
		return sentinel
	})
	if !errors.Is(err, sentinel) {
		t.Fatalf("rollback error = %v, want %v", err, sentinel)
	}
	provenance, err := compact.nodeLineage(head.Node)
	if err != nil {
		t.Fatal(err)
	}
	if provenance.owner != 0 {
		t.Fatalf("rolled-back owner = %d, want zero", provenance.owner)
	}
	if err := compact.ApplySchedulerAtomic(func(owner SchedulerTransactionToken) error {
		return compact.RecordHeadOwnerOwned(owner, head, 8)
	}); err != nil {
		t.Fatal(err)
	}
	if provenance.owner != 8 {
		t.Fatalf("committed owner = %d, want 8", provenance.owner)
	}
}

func TestHistoricalBoundaryDoesNotInventConvergence(t *testing.T) {
	compact := newTinyCoreWithLimits(t, Limits{MaxLinksPerBoundary: 2})
	compact.diagnostics.foldSamePredecessorShallowPayloads = false
	seed, _ := compact.Seed(1, 0)
	payload, _ := compact.appendSubtree(subtreeRecord{symbol: 1, terminal: true, endByte: 1}, nil, nil, nil)
	key := compact.boundaryKey(2, 1)
	var outcome condenseOutcome
	err := compact.ApplySchedulerAtomic(func(_ SchedulerTransactionToken) error {
		if _, err := compact.condense(key, linkInput{prev: seed.Node, payload: payload, scoreDelta: 1}); err != nil {
			return err
		}
		return compact.runLiveCondenseCandidates(nil, func() error {
			var err error
			outcome, err = compact.condenseWithOutcome(key, linkInput{
				prev: seed.Node, payload: payload, scoreDelta: 2,
			})
			return err
		})
	})
	if err != nil {
		t.Fatal(err)
	}
	if !outcome.historicalBoundarySplit {
		t.Fatal("sequential replacement lost its historical boundary marker")
	}
	if outcome.historicalConvergedSplit {
		t.Fatal("single-path historical boundary invented convergence")
	}
	if !outcome.historicalForestDeterministic {
		t.Fatal("non-fragile historical forest was not marked deterministic")
	}
	if outcome.historicalCleanPathRank != CleanPathRankNotApplicable || outcome.historicalLineage != 0 {
		t.Fatalf("single-path historical provenance = (%v,%d), want none", outcome.historicalCleanPathRank, outcome.historicalLineage)
	}
}

func TestHistoricalBoundaryProvesDeterministicForest(t *testing.T) {
	compact := newTinyCoreWithLimits(t, Limits{MaxLinksPerBoundary: 2})
	seed, _ := compact.Seed(1, 0)
	payload, _ := compact.appendSubtree(subtreeRecord{symbol: 1, terminal: true, endByte: 1}, nil, nil, nil)
	key := compact.boundaryKey(2, 1)
	input := linkInput{prev: seed.Node, payload: payload, scoreDelta: 1}
	var outcome condenseOutcome
	err := compact.ApplySchedulerAtomic(func(_ SchedulerTransactionToken) error {
		if _, err := compact.condense(key, input); err != nil {
			return err
		}
		return compact.runLiveCondenseCandidates(nil, func() error {
			var err error
			outcome, err = compact.condenseWithOutcome(key, input)
			return err
		})
	})
	if err != nil {
		t.Fatal(err)
	}
	if !outcome.historicalBoundarySplit || !outcome.historicalForestDeterministic {
		t.Fatalf("historical replacement provenance = %+v, want deterministic split", outcome)
	}
	if outcome.historicalConvergedSplit {
		t.Fatal("exact single-path replacement invented convergence")
	}
}

func TestHistoricalBoundaryKeepsFragileForestUnproved(t *testing.T) {
	compact := newTinyCoreWithLimits(t, Limits{MaxLinksPerBoundary: 2})
	seed, _ := compact.Seed(1, 0)
	payload, _ := compact.appendSubtree(subtreeRecord{
		symbol: 1, terminal: true, endByte: 1, fragile: true,
	}, nil, nil, nil)
	key := compact.boundaryKey(2, 1)
	input := linkInput{prev: seed.Node, payload: payload}
	var outcome condenseOutcome
	err := compact.ApplySchedulerAtomic(func(_ SchedulerTransactionToken) error {
		if _, err := compact.condense(key, input); err != nil {
			return err
		}
		return compact.runLiveCondenseCandidates(nil, func() error {
			var err error
			outcome, err = compact.condenseWithOutcome(key, input)
			return err
		})
	})
	if err != nil {
		t.Fatal(err)
	}
	if !outcome.historicalBoundarySplit || outcome.historicalForestDeterministic {
		t.Fatalf("fragile historical provenance = %+v, want unproved split", outcome)
	}
}

func TestHistoricalBoundaryProvesSourceSelfOwnership(t *testing.T) {
	compact := newTinyCoreWithLimits(t, Limits{MaxLinksPerBoundary: 2})
	seed, _ := compact.Seed(1, 0)
	payload, _ := compact.appendSubtree(subtreeRecord{
		symbol: 1, terminal: true, endByte: 1, fragile: true,
	}, nil, nil, nil)
	key := compact.boundaryKey(2, 1)
	input := linkInput{prev: seed.Node, payload: payload}
	var outcome condenseOutcome
	err := compact.ApplySchedulerAtomic(func(owner SchedulerTransactionToken) error {
		retired, err := compact.condense(key, input)
		if err != nil {
			return err
		}
		if err := compact.RecordHeadOwnerOwned(owner, retired, 7); err != nil {
			return err
		}
		compact.reductionSourceOwner = 7
		defer func() { compact.reductionSourceOwner = 0 }()
		return compact.runLiveCondenseCandidates(nil, func() error {
			outcome, err = compact.condenseWithOutcome(key, input)
			return err
		})
	})
	if err != nil {
		t.Fatal(err)
	}
	if !outcome.historicalBoundarySplit || !outcome.historicalForestDeterministic {
		t.Fatalf("source-self historical provenance = %+v, want deterministic", outcome)
	}
	if outcome.historicalConvergedSplit {
		t.Fatal("source-self historical replacement invented convergence")
	}
}

func TestLiveCondenseCandidatesMergesActiveHead(t *testing.T) {
	compact := newTinyCoreWithLimits(t, Limits{MaxLinksPerBoundary: 2})
	compact.diagnostics.foldSamePredecessorShallowPayloads = false
	seed, _ := compact.Seed(1, 0)
	payload, _ := compact.appendSubtree(subtreeRecord{symbol: 1, terminal: true, endByte: 1}, nil, nil, nil)
	key := compact.boundaryKey(2, 1)
	var active, merged Head
	err := compact.ApplySchedulerAtomic(func(_ SchedulerTransactionToken) error {
		var err error
		active, err = compact.condense(key, linkInput{prev: seed.Node, payload: payload, scoreDelta: 1})
		if err != nil {
			return err
		}
		candidates := []CondenseCandidate{{Head: active}}
		return compact.runLiveCondenseCandidates(candidates, func() error {
			merged, err = compact.condense(key, linkInput{prev: seed.Node, payload: payload, scoreDelta: 2})
			return err
		})
	})
	if err != nil {
		t.Fatal(err)
	}
	record, err := compact.node(merged.Node)
	if err != nil {
		t.Fatal(err)
	}
	if record.linkCount != 2 {
		t.Fatalf("merged active candidate links = %d, want 2", record.linkCount)
	}
}

func TestLiveCondenseCandidatesPreservesSameDispatchMergeOrder(t *testing.T) {
	compact := newTinyCoreWithLimits(t, Limits{MaxLinksPerBoundary: 2})
	compact.diagnostics.foldSamePredecessorShallowPayloads = false
	seed, _ := compact.Seed(1, 0)
	payload, _ := compact.appendSubtree(subtreeRecord{symbol: 1, terminal: true, endByte: 1}, nil, nil, nil)
	key := compact.boundaryKey(2, 1)
	var head Head
	err := compact.ApplySchedulerAtomic(func(_ SchedulerTransactionToken) error {
		return compact.runLiveCondenseCandidates(nil, func() error {
			if _, err := compact.condense(key, linkInput{prev: seed.Node, payload: payload, scoreDelta: 1}); err != nil {
				return err
			}
			var err error
			head, err = compact.condense(key, linkInput{prev: seed.Node, payload: payload, scoreDelta: 2})
			return err
		})
	})
	if err != nil {
		t.Fatal(err)
	}
	record, err := compact.node(head.Node)
	if err != nil {
		t.Fatal(err)
	}
	var inline [inlineAdjacencyCapacity]linkRecord
	links, err := compact.publishedNodeLinksInto(inline[:0], *record)
	if err != nil {
		t.Fatal(err)
	}
	if len(links) != 2 || links[0].scoreDelta != 1 || links[1].scoreDelta != 2 {
		t.Fatalf("same-dispatch link order = %+v, want score deltas [1 2]", links)
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
	if canonical, ok := core.CanonicalBoundary(2, 0, true, 0); !ok || canonical != shifted {
		t.Fatalf("consumed canonical head=%+v ok=%t, want %+v", canonical, ok, shifted)
	}
	if canonical, ok := core.CanonicalBoundary(2, 0, false, 0); !ok || canonical != runnable {
		t.Fatalf("runnable canonical head=%+v ok=%t, want %+v", canonical, ok, runnable)
	}
}

func TestScannerCheckpointPreventsSameBoundaryCondensation(t *testing.T) {
	core := newTinyCore(t, 4)
	seed, _ := core.Seed(1, 0)
	payload, _ := core.appendSubtree(subtreeRecord{symbol: 1, terminal: true, endByte: 1}, nil, nil, nil)
	firstCheckpoint := mustInternCheckpoint(t, core, []byte{1})
	secondCheckpoint := mustInternCheckpoint(t, core, []byte{2})
	if err := core.SetPhaseCheckpoint(firstCheckpoint); err != nil {
		t.Fatal(err)
	}
	first, err := core.condense(core.boundaryKey(2, 1), linkInput{prev: seed.Node, payload: payload})
	if err != nil {
		t.Fatal(err)
	}
	if err := core.SetPhaseCheckpoint(secondCheckpoint); err != nil {
		t.Fatal(err)
	}
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

func TestBeginFrontierRejectsActiveTransactionWithoutMutation(t *testing.T) {
	core := newTinyCore(t, 4)
	_, _ = core.Seed(1, 0)
	beforeBoundaries := cloneBoundaryMap(core.boundaries)
	beforeFrontier, beforeCheckpoint := core.frontier, core.checkpoint
	err := core.ApplyAtomic(func() error {
		if err := core.BeginFrontier(); err == nil || !strings.Contains(err.Error(), "active transaction") {
			return fmt.Errorf("begin frontier error=%v", err)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if core.frontier != beforeFrontier || core.checkpoint != beforeCheckpoint || !reflect.DeepEqual(core.boundaries.logicalMap(), beforeBoundaries) {
		t.Fatalf("rejected frontier advance mutated core: frontier=%d/%d checkpoint=%x/%x boundaries=%v/%v",
			core.frontier, beforeFrontier, core.checkpoint, beforeCheckpoint, core.boundaries.logicalMap(), beforeBoundaries)
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
		got, _ := compact.boundaries.get(key)
		if !errors.Is(err, sentinel) || got != head.Node {
			t.Fatalf("same-key rollback err=%v boundary=%d, want %d", err, got, head.Node)
		}
		if compact.boundaries.count != 1 {
			t.Fatalf("same-key rollback entries=%d, want 1", compact.boundaries.count)
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
		if _, exists := compact.boundaries.get(key); exists {
			t.Fatal("absent boundary survived rollback")
		}
		if compact.boundaries.count != 1 {
			t.Fatalf("absent-key rollback entries=%d, want 1", compact.boundaries.count)
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
			got, _ := compact.boundaries.get(key)
			if got != 3 || len(compact.transactions) != 1 || len(compact.boundaryJournal) != 2 {
				return fmt.Errorf("inner commit state boundary=%d transactions=%d journal=%d", got, len(compact.transactions), len(compact.boundaryJournal))
			}
			return sentinel
		})
		got, _ := compact.boundaries.get(key)
		if !errors.Is(err, sentinel) || got != head.Node {
			t.Fatalf("outer rollback err=%v boundary=%d, want %d", err, got, head.Node)
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
			got, _ := compact.boundaries.get(key)
			if !errors.Is(innerErr, sentinel) || got != 2 || len(compact.transactions) != 1 || len(compact.boundaryJournal) != 1 {
				return fmt.Errorf("inner rollback err=%v boundary=%d transactions=%d journal=%d", innerErr, got, len(compact.transactions), len(compact.boundaryJournal))
			}
			return nil
		})
		got, _ := compact.boundaries.get(key)
		if err != nil || got != 2 {
			t.Fatalf("outer commit err=%v boundary=%d, want 2", err, got)
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
	wantCheckpoint := mustInternCheckpoint(t, compact, []byte{1, 2, 3})
	if err := compact.SetPhaseCheckpoint(wantCheckpoint); err != nil {
		t.Fatal(err)
	}
	beforeCheckpoint = compact.checkpoint
	sentinel := errors.New("rollback")
	err = compact.ApplyAtomic(func() error {
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
	if afterStats != beforeStats || compact.frontier != beforeFrontier || compact.checkpoint != beforeCheckpoint || !reflect.DeepEqual(compact.boundaries.logicalMap(), beforeBoundaries) {
		t.Fatalf("scalar/arena rollback stats=%+v/%+v frontier=%d/%d checkpoint=%x/%x boundaries=%v/%v",
			afterStats, beforeStats, compact.frontier, beforeFrontier, compact.checkpoint, beforeCheckpoint, compact.boundaries.logicalMap(), beforeBoundaries)
	}
	assertTransactionJournalClean(t, compact)
}

func TestBeginFrontierRetiresCanonicalLookupButPreservesGraphHistory(t *testing.T) {
	compact := newTinyCore(t, 8)
	seed, err := compact.Seed(1, 0)
	if err != nil {
		t.Fatal(err)
	}
	payload, err := compact.appendSubtree(subtreeRecord{symbol: 1, terminal: true, endByte: 1}, nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	old, err := compact.condense(compact.boundaryKey(2, 1), linkInput{prev: seed.Node, payload: payload})
	if err != nil {
		t.Fatal(err)
	}
	oldKey := compact.boundaryKey(2, 1)
	if got := compact.BoundaryIndexStats(); got.CurrentEntries != 2 || got.RetainedEntries != 2 {
		t.Fatalf("pre-advance boundary stats=%+v, want 2/2", got)
	}
	before, err := compact.Derivations(old)
	if err != nil || len(before) != 1 {
		t.Fatalf("pre-advance derivations=%+v err=%v", before, err)
	}
	if err := compact.BeginFrontier(); err != nil {
		t.Fatal(err)
	}
	if got := compact.BoundaryIndexStats(); got.CurrentEntries != 0 || got.RetainedEntries != 0 {
		t.Fatalf("retired boundary stats=%+v, want 0/0", got)
	}
	if _, exists := compact.boundaries.get(oldKey); exists || compact.boundaries.count != 0 {
		t.Fatalf("historical canonical lookup survived: exists=%t entries=%d", exists, compact.boundaries.count)
	}
	state, offset, err := compact.Boundary(old)
	if err != nil || state != 2 || offset != 1 {
		t.Fatalf("historical head invalid after frontier advance: state=%d offset=%d err=%v", state, offset, err)
	}
	after, err := compact.Derivations(old)
	if err != nil || !reflect.DeepEqual(after, before) {
		t.Fatalf("historical derivation drifted after frontier advance: got=%+v want=%+v err=%v", after, before, err)
	}
	newHead, err := compact.condense(compact.boundaryKey(2, 1), linkInput{prev: seed.Node, payload: payload})
	if err != nil {
		t.Fatal(err)
	}
	if newHead == old {
		t.Fatalf("new frontier reused historical canonical head %+v", old)
	}
	if got := compact.BoundaryIndexStats(); got.CurrentEntries != 1 || got.RetainedEntries != 1 {
		t.Fatalf("new-frontier boundary stats=%+v, want 1/1", got)
	}
}

func TestBeginFrontierWarmPathAllocations(t *testing.T) {
	compact := newTinyCore(t, 8)
	const width = 32
	cycle := func() {
		for index := 0; index < width; index++ {
			compact.writeBoundary(compact.boundaryKey(StateID(index+1), uint32(index)), NodeID(index+1))
		}
		if err := compact.BeginFrontier(); err != nil {
			t.Fatal(err)
		}
	}
	cycle()
	if got := testing.AllocsPerRun(100, cycle); got != 0 {
		t.Fatalf("warm frontier cycle allocations=%v, want 0", got)
	}
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
	if after != before || !reflect.DeepEqual(compact.boundaries.logicalMap(), beforeBoundaries) {
		t.Fatalf("nested parser operations survived rollback: stats=%+v/%+v boundaries=%v/%v", after, before, compact.boundaries.logicalMap(), beforeBoundaries)
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
	got, _ := compact.boundaries.get(key)
	if got != head.Node {
		t.Fatalf("panic rollback boundary=%d, want %d", got, head.Node)
	}
	assertTransactionJournalClean(t, compact)
}

func TestApplyAtomicNoOpDoesNotScaleWithBoundaryCount(t *testing.T) {
	compact, err := New(&fakeTable{}, Limits{MaxNodes: 1 << 16})
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
		t.Fatalf("no-op transaction allocations=%v, want 0 with %d preloaded boundaries", allocs, compact.boundaries.count)
	}
	assertTransactionJournalClean(t, compact)
}

func BenchmarkApplyAtomicNoOpPreloadedBoundaries(b *testing.B) {
	compact, err := New(&fakeTable{}, Limits{MaxNodes: 1 << 17})
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

func TestShiftOrdinaryCohortSupportsZeroWidthTerminal(t *testing.T) {
	tables := &fakeTable{actions: map[tableCell][]Action{
		{state: 1, symbol: 9}: {{Type: ActionShift, State: 3}},
		{state: 2, symbol: 9}: {{Type: ActionShift, State: 4}},
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
	}, 9, Token{Symbol: 9, StartByte: 4, EndByte: 4})
	if err != nil {
		t.Fatal(err)
	}
	if len(shifted) != 2 {
		t.Fatalf("shifted cohort length=%d, want 2", len(shifted))
	}
	for index, wantState := range []StateID{3, 4} {
		state, byteOffset, err := compact.Boundary(shifted[index])
		if err != nil {
			t.Fatal(err)
		}
		if state != wantState || byteOffset != 4 {
			t.Fatalf("shifted boundary %d=(state %d, byte %d), want (state %d, byte 4)", index, state, byteOffset, wantState)
		}
		paths, err := compact.Derivations(shifted[index])
		if err != nil {
			t.Fatal(err)
		}
		if len(paths) != 1 || len(paths[0].Payloads) != 1 {
			t.Fatalf("shifted boundary %d paths=%+v, want one terminal derivation", index, paths)
		}
		view, err := compact.Subtree(paths[0].Payloads[0])
		if err != nil {
			t.Fatal(err)
		}
		if view.Symbol != 9 || view.StartByte != 4 || view.EndByte != 4 || view.Extra || !view.Terminal {
			t.Fatalf("shifted boundary %d terminal=%+v", index, view)
		}
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
		if _, ok := compact.CanonicalBoundary(3, 5, true, 0); ok {
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

func TestShiftExtraCohortSharesOneTerminalPayloadAndRetainsZeroTargets(t *testing.T) {
	tables := &fakeTable{actions: map[tableCell][]Action{
		{state: 1, symbol: 9}: {{Type: ActionShift, Extra: true}},
		{state: 2, symbol: 9}: {{Type: ActionShift, State: 5, Extra: true}},
	}}
	compact, err := New(tables, Limits{MaxDerivations: 4, MaxPopPaths: 4})
	if err != nil {
		t.Fatal(err)
	}
	first, _ := compact.Seed(1, 4)
	second, _ := compact.Seed(2, 4)
	before, _ := compact.Stats(first)

	shifted, err := compact.ShiftExtraCohort([]ExtraCohortShiftInput{{Head: first}, {Head: second}}, 9, Token{
		Symbol: 9, StartByte: 4, EndByte: 7, Extra: true, External: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(shifted) != 2 {
		t.Fatalf("shifted cohort length=%d, want 2", len(shifted))
	}
	for index, wantState := range []StateID{1, 5} {
		state, offset, err := compact.Boundary(shifted[index])
		if err != nil || state != wantState || offset != 7 {
			t.Fatalf("shifted head %d boundary=(%d,%d) err=%v, want (%d,7)", index, state, offset, err, wantState)
		}
	}
	after, _ := compact.Stats(shifted[1])
	if after.Nodes-before.Nodes != 2 || after.Links-before.Links != 2 || after.Subtrees-before.Subtrees != 1 || after.CurrentExactPaths != 1 {
		t.Fatalf("shared extra cohort physical delta=%+v -> %+v, want N+2/L+2/S+1", before, after)
	}
	var shared SubtreeID
	for index, head := range shifted {
		paths, err := compact.Derivations(head)
		if err != nil || len(paths) != 1 || len(paths[0].Payloads) != 1 {
			t.Fatalf("shifted head %d paths=%+v err=%v", index, paths, err)
		}
		if shared == 0 {
			shared = paths[0].Payloads[0]
		} else if paths[0].Payloads[0] != shared {
			t.Fatalf("shifted head %d payload=%d, want shared %d", index, paths[0].Payloads[0], shared)
		}
	}
	view, err := compact.Subtree(shared)
	if err != nil {
		t.Fatal(err)
	}
	if view.Symbol != 9 || view.StartByte != 4 || view.EndByte != 7 || !view.Extra || !view.External || !view.Terminal || len(view.Children) != 0 || len(view.Fields) != 0 || len(view.Aliases) != 0 {
		t.Fatalf("shared extra terminal view=%+v", view)
	}
}

func TestShiftExtraCohortAllowsZeroWidthExternalTerminal(t *testing.T) {
	tables := &fakeTable{actions: map[tableCell][]Action{
		{state: 1, symbol: 9}: {{Type: ActionShift, Extra: true}},
	}}
	compact, err := New(tables, Limits{MaxDerivations: 4, MaxPopPaths: 4})
	if err != nil {
		t.Fatal(err)
	}
	seed, err := compact.Seed(1, 4)
	if err != nil {
		t.Fatal(err)
	}
	shifted, err := compact.ShiftExtraCohort([]ExtraCohortShiftInput{{Head: seed}}, 9, Token{
		Symbol: 9, StartByte: 4, EndByte: 4, Extra: true, External: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(shifted) != 1 {
		t.Fatalf("shifted cohort length=%d, want 1", len(shifted))
	}
	state, offset, err := compact.Boundary(shifted[0])
	if err != nil || state != 1 || offset != 4 {
		t.Fatalf("zero-width external boundary=(%d,%d) err=%v", state, offset, err)
	}
	paths, err := compact.Derivations(shifted[0])
	if err != nil || len(paths) != 1 || len(paths[0].Payloads) != 1 {
		t.Fatalf("zero-width external paths=%+v err=%v", paths, err)
	}
	view, err := compact.Subtree(paths[0].Payloads[0])
	if err != nil {
		t.Fatal(err)
	}
	if view.StartByte != 4 || view.EndByte != 4 || !view.Extra || !view.External || !view.Terminal {
		t.Fatalf("zero-width external payload=%+v", view)
	}
}

func TestShiftExtraCohortValidatesBeforeMutation(t *testing.T) {
	tables := &fakeTable{actions: map[tableCell][]Action{
		{state: 1, symbol: 9}: {{Type: ActionShift, Extra: true}},
		{state: 2, symbol: 9}: {{Type: ActionShift, State: 4}},
		{state: 3, symbol: 9}: {{Type: ActionShift, Extra: true, ExtraChain: true}},
		{state: 4, symbol: 9}: {{Type: ActionShift, Extra: true, Repetition: true}},
	}}
	compact, err := New(tables, Limits{MaxDerivations: 4, MaxPopPaths: 4})
	if err != nil {
		t.Fatal(err)
	}
	first, _ := compact.Seed(1, 4)
	ordinary, _ := compact.Seed(2, 4)
	extraChain, _ := compact.Seed(3, 4)
	repetition, _ := compact.Seed(4, 4)
	before, _ := compact.Stats(first)
	tests := []struct {
		name   string
		inputs []ExtraCohortShiftInput
		token  Token
	}{
		{name: "empty", token: Token{Symbol: 9, StartByte: 4, EndByte: 5, Extra: true}},
		{name: "duplicate", inputs: []ExtraCohortShiftInput{{Head: first}, {Head: first}}, token: Token{Symbol: 9, StartByte: 4, EndByte: 5, Extra: true}},
		{name: "ordinary-action", inputs: []ExtraCohortShiftInput{{Head: first}, {Head: ordinary}}, token: Token{Symbol: 9, StartByte: 4, EndByte: 5, Extra: true}},
		{name: "extra-chain", inputs: []ExtraCohortShiftInput{{Head: extraChain}}, token: Token{Symbol: 9, StartByte: 4, EndByte: 5, Extra: true}},
		{name: "repetition", inputs: []ExtraCohortShiftInput{{Head: repetition}}, token: Token{Symbol: 9, StartByte: 4, EndByte: 5, Extra: true}},
		{name: "ordinary-token", inputs: []ExtraCohortShiftInput{{Head: first}}, token: Token{Symbol: 9, StartByte: 4, EndByte: 5}},
		{name: "zero-width", inputs: []ExtraCohortShiftInput{{Head: first}}, token: Token{Symbol: 9, StartByte: 4, EndByte: 4, Extra: true}},
		{name: "symbol-mismatch", inputs: []ExtraCohortShiftInput{{Head: first}}, token: Token{Symbol: 8, StartByte: 4, EndByte: 5, Extra: true}},
		{name: "invalid-head", inputs: []ExtraCohortShiftInput{{Head: first}, {Head: Head{Node: 999}}}, token: Token{Symbol: 9, StartByte: 4, EndByte: 5, Extra: true}},
		{name: "nonzero-ordinal", inputs: []ExtraCohortShiftInput{{Head: first, ActionOrdinal: 1}}, token: Token{Symbol: 9, StartByte: 4, EndByte: 5, Extra: true}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if _, err := compact.ShiftExtraCohort(test.inputs, 9, test.token); err == nil {
				t.Fatal("invalid extra cohort shift succeeded")
			}
			after, _ := compact.Stats(first)
			if after != before {
				t.Fatalf("validation failure mutated core: before=%+v after=%+v", before, after)
			}
		})
	}
}

func TestShiftExtraCohortRollsBackCaps(t *testing.T) {
	newCohort := func(t *testing.T, limits Limits, commonTarget bool) (*Core, Head, Head) {
		t.Helper()
		firstAction := Action{Type: ActionShift, Extra: true}
		secondAction := Action{Type: ActionShift, Extra: true}
		if commonTarget {
			firstAction.State, secondAction.State = 3, 3
		}
		compact, err := New(&fakeTable{actions: map[tableCell][]Action{
			{state: 1, symbol: 9}: {firstAction},
			{state: 2, symbol: 9}: {secondAction},
		}}, limits)
		if err != nil {
			t.Fatal(err)
		}
		first, _ := compact.Seed(1, 4)
		second, _ := compact.Seed(2, 4)
		return compact, first, second
	}
	t.Run("subtree", func(t *testing.T) {
		compact, first, second := newCohort(t, Limits{MaxSubtrees: 1, MaxDerivations: 4, MaxPopPaths: 4}, false)
		if _, err := compact.appendSubtree(subtreeRecord{symbol: 8, terminal: true}, nil, nil, nil); err != nil {
			t.Fatal(err)
		}
		before, _ := compact.Stats(first)
		if _, err := compact.ShiftExtraCohort([]ExtraCohortShiftInput{{Head: first}, {Head: second}}, 9, Token{Symbol: 9, StartByte: 4, EndByte: 5, Extra: true}); err == nil {
			t.Fatal("subtree-capped extra cohort shift succeeded")
		}
		after, _ := compact.Stats(first)
		if after != before {
			t.Fatalf("subtree cap failure mutated core: before=%+v after=%+v", before, after)
		}
	})
	for _, test := range []struct {
		name         string
		limits       Limits
		commonTarget bool
	}{
		{name: "second-node", limits: Limits{MaxNodes: 3, MaxDerivations: 4, MaxPopPaths: 4}},
		{name: "second-link", limits: Limits{MaxLinks: 1, MaxDerivations: 4, MaxPopPaths: 4}},
		{name: "second-live-link", limits: Limits{MaxLinksPerBoundary: 1, MaxDerivations: 4, MaxPopPaths: 4}, commonTarget: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			compact, first, second := newCohort(t, test.limits, test.commonTarget)
			before, _ := compact.Stats(first)
			if _, err := compact.ShiftExtraCohort([]ExtraCohortShiftInput{{Head: first}, {Head: second}}, 9, Token{Symbol: 9, StartByte: 4, EndByte: 5, Extra: true}); err == nil {
				t.Fatal("capped extra cohort shift succeeded")
			}
			after, _ := compact.Stats(first)
			if after != before {
				t.Fatalf("cap failure mutated core: before=%+v after=%+v", before, after)
			}
			states := []StateID{1, 2}
			if test.commonTarget {
				states = []StateID{3}
			}
			for _, state := range states {
				if _, ok := compact.CanonicalBoundary(state, 5, true, 0); ok {
					t.Fatalf("rolled-back extra cohort boundary remains published for state %d", state)
				}
			}
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
	beforeWork := compact.Work()
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
	afterWork := compact.Work()
	if afterWork.ParentConstructionsProxy-beforeWork.ParentConstructionsProxy != 1 {
		t.Fatalf("shared reduction parent constructions=%d, want 1", afterWork.ParentConstructionsProxy-beforeWork.ParentConstructionsProxy)
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
			if _, err := compact.subtree(parent); err != nil {
				t.Fatalf("shared parent lookup err=%v", err)
			}
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

func TestNoLookaheadReductionMarksOnlyTransparentGotoExtra(t *testing.T) {
	run := func(t *testing.T, gotoState StateID, wantExtra bool) {
		t.Helper()
		tables := &fakeTable{
			actions: map[tableCell][]Action{
				{state: 1, symbol: 9}: {{Type: ActionShift, State: 2}},
				{state: 2, symbol: 0}: {{Type: ActionReduce, Symbol: 5, ChildCount: 1}},
			},
			gotos: map[tableCell]StateID{
				{state: 1, symbol: 5}: gotoState,
			},
		}
		compact, err := New(tables, Limits{MaxDerivations: 4, MaxPopPaths: 4})
		if err != nil {
			t.Fatal(err)
		}
		seed, err := compact.Seed(1, 0)
		if err != nil {
			t.Fatal(err)
		}
		shifted, err := compact.Shift(
			seed,
			9,
			0,
			Token{Symbol: 9, StartByte: 0, EndByte: 1},
			ForkOrder{},
		)
		if err != nil {
			t.Fatal(err)
		}
		compact.SetReduceNoLookaheadContext(true)
		reduced, err := compact.Reduce(shifted, 0, 0, ForkOrder{})
		compact.SetReduceNoLookaheadContext(false)
		if err != nil {
			t.Fatal(err)
		}
		if len(reduced) != 1 {
			t.Fatalf("reduction outputs=%d, want 1", len(reduced))
		}
		paths, err := compact.Derivations(reduced[0])
		if err != nil {
			t.Fatal(err)
		}
		if len(paths) != 1 || len(paths[0].Payloads) != 1 {
			t.Fatalf("reduced paths=%+v, want one payload", paths)
		}
		view, err := compact.Subtree(paths[0].Payloads[0])
		if err != nil {
			t.Fatal(err)
		}
		if view.Extra != wantExtra {
			t.Fatalf("reduced parent extra=%t, want %t", view.Extra, wantExtra)
		}
	}

	t.Run("transparent", func(t *testing.T) { run(t, 1, true) })
	t.Run("state-progress", func(t *testing.T) { run(t, 3, false) })
}

func TestReduceOutputsAggregatesFreshnessPerFinalBoundary(t *testing.T) {
	t.Run("new-dominates-later-fold", func(t *testing.T) {
		tables := &fakeTable{
			actions: map[tableCell][]Action{{state: 3, symbol: 9}: {{Type: ActionReduce, Symbol: 2, ChildCount: 1}}},
			gotos:   map[tableCell]StateID{{state: 1, symbol: 2}: 4},
		}
		compact, err := New(tables, Limits{MaxDerivations: 4, MaxPopPaths: 4})
		if err != nil {
			t.Fatal(err)
		}
		seed, _ := compact.Seed(1, 0)
		first, _ := compact.appendSubtree(subtreeRecord{symbol: 10, endByte: 1, terminal: true}, nil, nil, nil)
		second, _ := compact.appendSubtree(subtreeRecord{symbol: 11, endByte: 1, terminal: true}, nil, nil, nil)
		head, err := compact.condense(compact.boundaryKey(3, 1), linkInput{prev: seed.Node, payload: first, scoreDelta: 10})
		if err != nil {
			t.Fatal(err)
		}
		head, err = compact.condense(compact.boundaryKey(3, 1), linkInput{prev: seed.Node, payload: second, scoreDelta: 9})
		if err != nil {
			t.Fatal(err)
		}
		outputs, err := compact.ReduceOutputs(head, 9, 0, ForkOrder{})
		if err != nil || len(outputs) != 1 || outputs[0].Freshness != ReductionNew {
			t.Fatalf("aggregate outputs=%+v err=%v", outputs, err)
		}
		paths, err := compact.Derivations(outputs[0].Head)
		if err != nil || len(paths) != 1 || paths[0].Score != 10 {
			t.Fatalf("selected final canonical paths=%+v err=%v", paths, err)
		}
		stable := outputs[0]
		again, err := compact.ReduceOutputs(head, 9, 0, ForkOrder{})
		if err != nil || len(again) != 1 || again[0].Freshness != ReductionUnchanged {
			t.Fatalf("repeat outputs=%+v err=%v", again, err)
		}
		if outputs[0] != stable {
			t.Fatalf("later reduction mutated stable compatibility output: got=%+v want=%+v", outputs[0], stable)
		}
	})

	t.Run("mixed-unchanged-and-new", func(t *testing.T) {
		tables := &fakeTable{
			actions: map[tableCell][]Action{{state: 3, symbol: 9}: {{Type: ActionReduce, Symbol: 2, ChildCount: 1}}},
			gotos: map[tableCell]StateID{
				{state: 1, symbol: 2}: 4,
				{state: 2, symbol: 2}: 5,
			},
		}
		compact, err := New(tables, Limits{MaxDerivations: 4, MaxPopPaths: 4})
		if err != nil {
			t.Fatal(err)
		}
		firstSeed, _ := compact.Seed(1, 0)
		secondSeed, _ := compact.Seed(2, 0)
		firstChild, _ := compact.appendSubtree(subtreeRecord{symbol: 10, endByte: 1, terminal: true}, nil, nil, nil)
		secondChild, _ := compact.appendSubtree(subtreeRecord{symbol: 11, endByte: 1, terminal: true}, nil, nil, nil)
		head, err := compact.condense(compact.boundaryKey(3, 1), linkInput{prev: firstSeed.Node, payload: firstChild, scoreDelta: 7})
		if err != nil {
			t.Fatal(err)
		}
		head, err = compact.condense(compact.boundaryKey(3, 1), linkInput{prev: secondSeed.Node, payload: secondChild, scoreDelta: 8})
		if err != nil {
			t.Fatal(err)
		}
		incumbentParent, err := compact.appendSubtree(subtreeRecord{symbol: 2, startByte: 0, endByte: 1}, []SubtreeID{firstChild}, nil, nil)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := compact.condense(compact.boundaryKey(4, 1), linkInput{prev: firstSeed.Node, payload: incumbentParent, scoreDelta: 7}); err != nil {
			t.Fatal(err)
		}
		dst := make([]ReductionOutput, 0, 2)
		backing := &dst[:1][0]
		outputs, err := compact.ReduceOutputsInto(dst, head, 9, 0, ForkOrder{})
		if err != nil || len(outputs) != 2 {
			t.Fatalf("mixed outputs=%+v err=%v", outputs, err)
		}
		if &outputs[0] != backing {
			t.Fatal("caller-owned reduction destination was not reused")
		}
		state0, _, _ := compact.Boundary(outputs[0].Head)
		state1, _, _ := compact.Boundary(outputs[1].Head)
		if state0 != 4 || outputs[0].Freshness != ReductionUnchanged || state1 != 5 || outputs[1].Freshness != ReductionNew {
			t.Fatalf("mixed output order/freshness=%+v states=(%d,%d)", outputs, state0, state1)
		}
	})

	t.Run("spill-aggregates-a-b-c-c-with-canonical-head", func(t *testing.T) {
		tables := &fakeTable{
			actions: map[tableCell][]Action{{state: 3, symbol: 9}: {{Type: ActionReduce, Symbol: 2, ChildCount: 1}}},
			gotos: map[tableCell]StateID{
				{state: 1, symbol: 2}: 4,
				{state: 2, symbol: 2}: 5,
				{state: 6, symbol: 2}: 7,
				{state: 8, symbol: 2}: 7,
			},
		}
		compact, err := New(tables, Limits{MaxDerivations: 8, MaxPopPaths: 8})
		if err != nil {
			t.Fatal(err)
		}
		states := []StateID{1, 2, 6, 8}
		seeds := make([]Head, len(states))
		children := make([]SubtreeID, len(states))
		var head Head
		for index, state := range states {
			seeds[index], err = compact.Seed(state, 0)
			if err != nil {
				t.Fatal(err)
			}
			children[index], err = compact.appendSubtree(subtreeRecord{symbol: Symbol(10 + index), endByte: 1, terminal: true}, nil, nil, nil)
			if err != nil {
				t.Fatal(err)
			}
			head, err = compact.condense(compact.boundaryKey(3, 1), linkInput{
				prev: seeds[index].Node, payload: children[index], scoreDelta: int64(index),
			})
			if err != nil {
				t.Fatal(err)
			}
		}

		// The first C path already exists. Its repeat is unchanged, while the
		// second C path adds a distinct predecessor to the same canonical head.
		incumbentC, err := compact.appendSubtree(subtreeRecord{symbol: 2, endByte: 1}, []SubtreeID{children[2]}, nil, nil)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := compact.condense(compact.boundaryKey(7, 1), linkInput{
			prev: seeds[2].Node, payload: incumbentC, scoreDelta: 2,
		}); err != nil {
			t.Fatal(err)
		}

		outputs, err := compact.ReduceOutputsInto(make([]ReductionOutput, 0, 3), head, 9, 0, ForkOrder{})
		if err != nil || len(outputs) != 3 {
			t.Fatalf("A/B/C/C outputs=%+v err=%v", outputs, err)
		}
		wantStates := []StateID{4, 5, 7}
		wantFreshness := []ReductionFreshness{ReductionNew, ReductionNew, ReductionUpdated}
		for index := range outputs {
			state, _, err := compact.Boundary(outputs[index].Head)
			if err != nil || state != wantStates[index] || outputs[index].Freshness != wantFreshness[index] {
				t.Fatalf("A/B/C/C output %d=%+v state=%d err=%v, want state=%d freshness=%d", index, outputs[index], state, err, wantStates[index], wantFreshness[index])
			}
		}
		canonicalC, ok := compact.CanonicalBoundary(7, 1, false, 0)
		if !ok || outputs[2].Head != canonicalC {
			t.Fatalf("C output head=%+v canonical=%+v ok=%t", outputs[2].Head, canonicalC, ok)
		}
		paths, err := compact.Derivations(outputs[2].Head)
		if err != nil || len(paths) != 2 {
			t.Fatalf("C canonical derivations=%+v err=%v, want two", paths, err)
		}
		if compact.reductionScratch.spilled || len(compact.reductionScratch.boundaries) != 0 || len(compact.reductionScratch.boundaryByKey) != 0 || len(compact.reductionScratch.batchParents) != 0 {
			t.Fatalf("successful spill retained logical scratch: %+v", compact.reductionScratch)
		}
	})

	t.Run("unchanged-updated-unchanged-aggregates-updated", func(t *testing.T) {
		tables := &fakeTable{
			actions: map[tableCell][]Action{{state: 3, symbol: 9}: {{Type: ActionReduce, Symbol: 2, ChildCount: 1}}},
			gotos:   map[tableCell]StateID{{state: 1, symbol: 2}: 4},
		}
		compact, err := New(tables, Limits{MaxDerivations: 8, MaxPopPaths: 8})
		if err != nil {
			t.Fatal(err)
		}
		seed, _ := compact.Seed(1, 0)
		children := make([]SubtreeID, 3)
		for index := range children {
			children[index], err = compact.appendSubtree(subtreeRecord{symbol: Symbol(10 + index), endByte: 1, terminal: true}, nil, nil, nil)
			if err != nil {
				t.Fatal(err)
			}
		}
		var head Head
		for index, score := range []int64{10, 11, 9} {
			head, err = compact.condense(compact.boundaryKey(3, 1), linkInput{prev: seed.Node, payload: children[index], scoreDelta: score})
			if err != nil {
				t.Fatal(err)
			}
		}
		incumbent, err := compact.appendSubtree(subtreeRecord{symbol: 2, endByte: 1}, []SubtreeID{children[0]}, nil, nil)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := compact.condense(compact.boundaryKey(4, 1), linkInput{prev: seed.Node, payload: incumbent, scoreDelta: 10}); err != nil {
			t.Fatal(err)
		}
		outputs, err := compact.ReduceOutputs(head, 9, 0, ForkOrder{})
		if err != nil || len(outputs) != 1 || outputs[0].Freshness != ReductionUpdated {
			t.Fatalf("updated aggregate outputs=%+v err=%v", outputs, err)
		}
		paths, err := compact.Derivations(outputs[0].Head)
		if err != nil || len(paths) != 1 || paths[0].Score != 11 {
			t.Fatalf("updated aggregate final paths=%+v err=%v", paths, err)
		}
	})
}

func TestReductionOutputScratchSpilledAbsentKeyUsesAppendIndex(t *testing.T) {
	var scratch reductionOutputScratch
	keys := []boundaryKey{
		{frontier: 1, state: 1},
		{frontier: 1, state: 2},
		{frontier: 1, state: 3},
		{frontier: 1, state: 4},
	}
	for index, key := range keys {
		got, seen := scratch.boundary(key)
		if seen || got != index {
			t.Fatalf("absent key %d index=%d seen=%t, want index=%d unseen", index, got, seen, index)
		}
		scratch.store(got, seen, reductionBoundaryOutput{key: key, head: Head{Node: NodeID(index + 1)}})
	}
	if !scratch.spilled || len(scratch.boundaries) != len(keys) {
		t.Fatalf("spill state=%+v, want four ordered boundaries", scratch)
	}
	for index, key := range keys {
		got, seen := scratch.boundary(key)
		if !seen || got != index || scratch.boundaries[got].head.Node != NodeID(index+1) {
			t.Fatalf("stored key %d index=%d seen=%t output=%+v", index, got, seen, scratch.boundaries[got])
		}
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

func cloneBoundaryMap(source boundaryIndex) map[boundaryIdentity]NodeID {
	return source.logicalMap()
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
	if got := unsafe.Sizeof(ClassifiedBoundary{}); got > 56 {
		t.Fatalf("ClassifiedBoundary size = %d, want <= 56", got)
	}
	for name, typ := range map[string]reflect.Type{
		"node":                   reflect.TypeFor[nodeRecord](),
		"node-lineage":           reflect.TypeFor[nodeLineageRecord](),
		"link":                   reflect.TypeFor[linkRecord](),
		"subtree":                reflect.TypeFor[subtreeRecord](),
		"external-provenance":    reflect.TypeFor[externalPayloadProvenance](),
		"lexer-skipped-prefix":   reflect.TypeFor[lexerSkippedPrefixProvenance](),
		"drop-cohort-action":     reflect.TypeFor[dropCohortActionIdentity](),
		"drop-cohort-record":     reflect.TypeFor[dropCohortRecord](),
		"drop-cohort-member":     reflect.TypeFor[dropCohortMember](),
		"drop-cohort-derivation": reflect.TypeFor[dropCohortDerivationRecord](),
		"drop-cohort-mutation":   reflect.TypeFor[dropCohortMutation](),
	} {
		for i := 0; i < typ.NumField(); i++ {
			kind := typ.Field(i).Type.Kind()
			if kind == reflect.Pointer || kind == reflect.Slice || kind == reflect.Map || kind == reflect.Interface || kind == reflect.String {
				t.Fatalf("%s record field %s is GC-visible %s", name, typ.Field(i).Name, kind)
			}
		}
	}
	if got := unsafe.Sizeof(nodeRecord{}); got != 32 {
		t.Fatalf("nodeRecord size = %d, want 32", got)
	}
	// G18 adds the value-owned drop-cohort reference set to this lineage
	// record. Keep the 104-byte width explicit because every node pays it.
	if got := unsafe.Sizeof(nodeLineageRecord{}); got != 104 {
		t.Fatalf("nodeLineageRecord size = %d, want 104", got)
	}
	if got := unsafe.Sizeof(linkRecord{}); got != 32 {
		t.Fatalf("linkRecord size = %d, want 32", got)
	}
	// G18 adds one value-owned reference set beside the historical set. The
	// reduction provenance handoff adds one value-owned LinkChainRef.
	if got := unsafe.Sizeof(ReductionOutput{}); got != 112 {
		t.Fatalf("ReductionOutput size = %d, want 112", got)
	}
	if got := unsafe.Sizeof(LinkChainRef{}); got != 8 {
		t.Fatalf("LinkChainRef size = %d, want 8", got)
	}
	if got := unsafe.Sizeof(popPath{}); got != 88 {
		t.Fatalf("popPath size = %d, want 88", got)
	}
	if got := unsafe.Sizeof(subtreeRecord{}); got != 44 {
		t.Fatalf("subtreeRecord size = %d, want 44", got)
	}
	if got := unsafe.Sizeof(externalPayloadProvenance{}); got != 12 {
		t.Fatalf("externalPayloadProvenance size = %d, want 12", got)
	}
	if got := unsafe.Sizeof(lexerSkippedPrefixProvenance{}); got != 8 {
		t.Fatalf("lexerSkippedPrefixProvenance size = %d, want 8", got)
	}
	// Store the bounded prefix length in Token's existing tail padding. Every
	// compact shift copies this value, including grammars that do not use it.
	if got := unsafe.Sizeof(Token{}); got != 16 {
		t.Fatalf("Token size = %d, want 16", got)
	}
}

func TestActionRowDescriptorClassifiesImmutableRows(t *testing.T) {
	tests := []struct {
		name      string
		actions   []Action
		kind      ActionRowKind
		hasShift  bool
		hasReduce bool
	}{
		{name: "empty", kind: ActionRowEmpty},
		{name: "shift", actions: []Action{{Type: ActionShift, State: 2}}, kind: ActionRowShift, hasShift: true},
		{name: "extra", actions: []Action{{Type: ActionShift, Extra: true}}, kind: ActionRowExtraShift, hasShift: true},
		{name: "reduce", actions: []Action{{Type: ActionReduce, Symbol: 7}}, kind: ActionRowReduce, hasReduce: true},
		{name: "accept", actions: []Action{{Type: ActionAccept}}, kind: ActionRowAccept},
		{name: "shift-conflict", actions: []Action{{Type: ActionShift, State: 2}, {Type: ActionShift, State: 3}}, kind: ActionRowConflict, hasShift: true},
		{name: "mixed-conflict", actions: []Action{{Type: ActionReduce, Symbol: 7}, {Type: ActionShift, State: 2}}, kind: ActionRowConflict, hasShift: true, hasReduce: true},
		{name: "repetition", actions: []Action{{Type: ActionShift, Repetition: true}}, kind: ActionRowUnsupported},
		{name: "extra-chain", actions: []Action{{Type: ActionShift, ExtraChain: true}}, kind: ActionRowUnsupported},
		{name: "recover", actions: []Action{{Type: ActionRecover}}, kind: ActionRowUnsupported},
		{name: "multi-accept", actions: []Action{{Type: ActionAccept}, {Type: ActionReduce}}, kind: ActionRowUnsupported},
		{name: "extra-conflict", actions: []Action{{Type: ActionShift, Extra: true}, {Type: ActionReduce}}, kind: ActionRowUnsupported},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			row := NewActionRow(test.actions, false)
			descriptor := row.Descriptor()
			if descriptor.Kind() != test.kind || descriptor.HasShift() != test.hasShift || descriptor.HasReduce() != test.hasReduce {
				t.Fatalf("descriptor=(kind=%v shift=%t reduce=%t), want (%v %t %t)", descriptor.Kind(), descriptor.HasShift(), descriptor.HasReduce(), test.kind, test.hasShift, test.hasReduce)
			}
			if len(test.actions) != 0 {
				want := test.actions[0]
				test.actions[0] = Action{Type: ActionRecover}
				got := row.At(0)
				got.Type = ActionRecover
				if row.At(0) != want || row.Descriptor().Kind() != test.kind {
					t.Fatalf("source/result mutation changed immutable row: action=%+v descriptor=%v", row.At(0), row.Descriptor().Kind())
				}
			}
		})
	}
}

// TestActionRowReusableCarriesLanguageTableBitUnchanged pins that NewActionRow's
// reusable argument (Language.ParseActions[i].Reusable) round-trips through
// Reusable() unchanged. This is substrate for a later forced-reuse tranche:
// nothing reads Reusable() yet, and it does not affect Descriptor's dispatch
// shape (describeActionRow never sees it; it is derived only from actions).
func TestActionRowReusableCarriesLanguageTableBitUnchanged(t *testing.T) {
	tests := []struct {
		name     string
		reusable bool
	}{
		{name: "reusable-true", reusable: true},
		{name: "reusable-false", reusable: false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			row := NewActionRow([]Action{{Type: ActionShift, State: 2}}, test.reusable)
			if got := row.Reusable(); got != test.reusable {
				t.Fatalf("row.Reusable() = %t, want %t", got, test.reusable)
			}
		})
	}
}

// TestActionRowReusableZeroValueIsFalse pins that a zero-value ActionRow
// (returned for an empty action list, matching NewActionRow's pre-existing
// empty-row short circuit) reports Reusable() == false regardless of the
// reusable argument, since the empty row never allocates actionRowData.
func TestActionRowReusableZeroValueIsFalse(t *testing.T) {
	if got := (ActionRow{}).Reusable(); got != false {
		t.Fatalf("zero-value ActionRow.Reusable() = %t, want false", got)
	}
	if got := NewActionRow(nil, true).Reusable(); got != false {
		t.Fatalf("NewActionRow(nil, true).Reusable() = %t, want false (empty row drops the bit)", got)
	}
}

func TestClassifiedBoundaryAuthenticatesOwnerAndMonotonicPhase(t *testing.T) {
	tables := &fakeTable{actions: map[tableCell][]Action{
		{state: 1, symbol: 9}: {{Type: ActionShift, State: 2}},
	}}
	compact, err := New(tables, Limits{})
	if err != nil {
		t.Fatal(err)
	}
	head, err := compact.Seed(1, 0)
	if err != nil {
		t.Fatal(err)
	}
	classified, err := compact.ClassifyBoundary(head, 9)
	if err != nil || classified.Head() != head || classified.State() != 1 || classified.ByteOffset() != 0 || classified.Actions().Len() != 1 {
		t.Fatalf("classification=%+v err=%v", classified, err)
	}
	if got := classified.Actions().Descriptor().Kind(); got != ActionRowShift {
		t.Fatalf("classification descriptor=%v, want shift", got)
	}
	other, err := New(tables, Limits{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := other.ShiftClassified(classified, 0, Token{Symbol: 9, EndByte: 1}, ForkOrder{}); err == nil {
		t.Fatal("foreign core accepted classified boundary")
	}
	if err := compact.SetPhaseCheckpoint(mustInternCheckpoint(t, compact, []byte{1})); err != nil {
		t.Fatal(err)
	}
	if _, err := compact.ShiftClassified(classified, 0, Token{Symbol: 9, EndByte: 1}, ForkOrder{}); err == nil {
		t.Fatal("new scanner checkpoint accepted stale classified boundary")
	}
	classified, err = compact.ClassifyBoundary(head, 9)
	if err != nil {
		t.Fatal(err)
	}
	if err := compact.BeginFrontier(); err != nil {
		t.Fatal(err)
	}
	if _, err := compact.ShiftClassified(classified, 0, Token{Symbol: 9, EndByte: 1}, ForkOrder{}); err == nil {
		t.Fatal("new frontier accepted stale classified boundary")
	}
	classified, err = compact.ClassifyBoundary(head, 9)
	if err != nil {
		t.Fatal(err)
	}
	if err := compact.Reset(); err != nil {
		t.Fatal(err)
	}
	if _, err := compact.ShiftClassified(classified, 0, Token{Symbol: 9, EndByte: 1}, ForkOrder{}); err == nil {
		t.Fatal("reset core accepted stale classified boundary")
	}
}

func TestClassifiedBoundaryRollbackInvalidatesReusedNodeIDs(t *testing.T) {
	tables := &fakeTable{
		actions: map[tableCell][]Action{
			{state: 1, symbol: 9}: {{Type: ActionShift, State: 2}},
			{state: 2, symbol: 9}: {
				{Type: ActionShift, State: 4},
				{Type: ActionReduce, Symbol: 7, ChildCount: 0},
			},
			{state: 3, symbol: 9}: {{Type: ActionShift, State: 5}},
		},
		gotos: map[tableCell]StateID{{state: 2, symbol: 7}: 6},
	}
	sentinel := errors.New("rollback")

	t.Run("escaped capability rejects reused node", func(t *testing.T) {
		compact, err := New(tables, Limits{})
		if err != nil {
			t.Fatal(err)
		}
		var escaped ClassifiedBoundary
		var rolledBackNode NodeID
		err = compact.ApplyAtomic(func() error {
			head, err := compact.Seed(2, 0)
			if err != nil {
				return err
			}
			rolledBackNode = head.Node
			escaped, err = compact.ClassifyBoundary(head, 9)
			if err != nil {
				return err
			}
			return sentinel
		})
		if !errors.Is(err, sentinel) {
			t.Fatalf("failed transaction err=%v, want sentinel", err)
		}
		reused, err := compact.Seed(3, 0)
		if err != nil {
			t.Fatal(err)
		}
		if reused.Node != rolledBackNode {
			t.Fatalf("rollback NodeID=%d reused=%d, adversarial reuse was not established", rolledBackNode, reused.Node)
		}
		if _, err := compact.ShiftClassified(escaped, 0, Token{Symbol: 9, EndByte: 1}, ForkOrder{}); err == nil {
			t.Fatal("stale classified shift accepted a reused NodeID")
		}
		if _, err := compact.ReduceOutputsClassifiedInto(nil, escaped, 1, ForkOrder{}); err == nil {
			t.Fatal("stale classified reduction accepted a reused NodeID")
		}
		fresh, err := compact.ClassifyBoundary(reused, 9)
		if err != nil {
			t.Fatal(err)
		}
		shifted, err := compact.ShiftClassified(fresh, 0, Token{Symbol: 9, EndByte: 1}, ForkOrder{})
		if err != nil {
			t.Fatal(err)
		}
		state, _, err := compact.Boundary(shifted)
		if err != nil || state != 5 {
			t.Fatalf("fresh post-rollback shift state=%d err=%v, want 5", state, err)
		}
	})

	t.Run("nested rollback invalidates preexisting capability", func(t *testing.T) {
		compact, err := New(tables, Limits{})
		if err != nil {
			t.Fatal(err)
		}
		stable, err := compact.Seed(1, 0)
		if err != nil {
			t.Fatal(err)
		}
		before, err := compact.ClassifyBoundary(stable, 9)
		if err != nil {
			t.Fatal(err)
		}
		var innerEscaped ClassifiedBoundary
		err = compact.ApplyAtomic(func() error {
			innerErr := compact.ApplyAtomic(func() error {
				head, err := compact.Seed(2, 0)
				if err != nil {
					return err
				}
				innerEscaped, err = compact.ClassifyBoundary(head, 9)
				if err != nil {
					return err
				}
				return sentinel
			})
			if !errors.Is(innerErr, sentinel) {
				return fmt.Errorf("inner rollback err=%v, want sentinel", innerErr)
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := compact.ShiftClassified(before, 0, Token{Symbol: 9, EndByte: 1}, ForkOrder{}); err == nil {
			t.Fatal("capability predating nested rollback remained valid")
		}
		if _, err := compact.ShiftClassified(innerEscaped, 0, Token{Symbol: 9, EndByte: 1}, ForkOrder{}); err == nil {
			t.Fatal("capability escaping nested rollback remained valid")
		}
		fresh, err := compact.ClassifyBoundary(stable, 9)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := compact.ShiftClassified(fresh, 0, Token{Symbol: 9, EndByte: 1}, ForkOrder{}); err != nil {
			t.Fatalf("fresh classification after nested rollback failed: %v", err)
		}
	})

	t.Run("successful transaction preserves capability", func(t *testing.T) {
		compact, err := New(tables, Limits{})
		if err != nil {
			t.Fatal(err)
		}
		stable, err := compact.Seed(1, 0)
		if err != nil {
			t.Fatal(err)
		}
		classified, err := compact.ClassifyBoundary(stable, 9)
		if err != nil {
			t.Fatal(err)
		}
		if err := compact.ApplyAtomic(func() error {
			_, err := compact.Seed(3, 0)
			return err
		}); err != nil {
			t.Fatal(err)
		}
		if _, err := compact.ShiftClassified(classified, 0, Token{Symbol: 9, EndByte: 1}, ForkOrder{}); err != nil {
			t.Fatalf("successful transaction invalidated classification: %v", err)
		}
	})

	t.Run("rollback phase overflow fails closed", func(t *testing.T) {
		compact, err := New(tables, Limits{})
		if err != nil {
			t.Fatal(err)
		}
		compact.classificationPhase = math.MaxUint64
		defer func() {
			if recovered := recover(); recovered == nil {
				t.Fatal("rollback classification phase overflow did not panic")
			}
			if compact.classificationPhase != math.MaxUint64 {
				t.Fatalf("rollback classification phase wrapped to %d", compact.classificationPhase)
			}
		}()
		_ = compact.ApplyAtomic(func() error { return sentinel })
	})
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

func TestDerivationsDeepSinglePath(t *testing.T) {
	const depth = 50_000
	core := newTinyCoreWithLimits(t, Limits{
		MaxNodes: depth + 1, MaxLinks: depth, MaxSubtrees: depth,
		MaxDerivations: 2, MaxPopPaths: 2,
	})
	seed, err := core.Seed(1, 0)
	if err != nil {
		t.Fatal(err)
	}
	core.links = make([]linkRecord, depth)
	core.subtrees = make([]subtreeRecord, depth)
	core.nodes = append(core.nodes, make([]nodeRecord, depth)...)
	for index := 0; index < depth; index++ {
		linkID := LinkID(index + 1)
		core.links[index] = linkRecord{
			prev: NodeID(index + 1), payload: SubtreeID(index + 1),
		}
		core.nodes[index+1] = nodeRecord{
			firstLink: uint32(linkID), linkCount: 1, pathCount: 1,
		}
	}
	core.links[0].scoreDelta = 2
	core.links[depth/2].scoreDelta = -3
	core.links[depth-1].scoreDelta = 5
	core.links[0].flags = linkFlagHasOrder
	core.links[0].order = 7
	core.links[depth/2].flags = linkFlagHasOrder
	core.links[depth/2].order = 9

	paths, err := core.Derivations(Head{Node: seed.Node + depth})
	if err != nil {
		t.Fatal(err)
	}
	if len(paths) != 1 {
		t.Fatalf("derivation count=%d, want 1", len(paths))
	}
	path := paths[0]
	if len(path.Payloads) != depth {
		t.Fatalf("payload count=%d, want %d", len(path.Payloads), depth)
	}
	if path.Payloads[0] != 1 || path.Payloads[depth-1] != depth {
		t.Fatalf("payload endpoints=%d,%d, want 1,%d", path.Payloads[0], path.Payloads[depth-1], depth)
	}
	if path.Score != 4 {
		t.Fatalf("score=%d, want 4", path.Score)
	}
	if !path.HasBranchOrder || path.BranchOrder != 9 {
		t.Fatalf("branch order=(%t,%d), want (true,9)", path.HasBranchOrder, path.BranchOrder)
	}
}

func newTinyCore(t *testing.T, pathCap uint64) *Core {
	t.Helper()
	return newTinyCoreWithLimits(t, Limits{MaxDerivations: pathCap, MaxPopPaths: pathCap})
}

func mustInternCheckpoint(t *testing.T, compact *Core, serialized []byte) CheckpointID {
	t.Helper()
	id, err := compact.InternCheckpoint(serialized)
	if err != nil {
		t.Fatal(err)
	}
	return id
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
