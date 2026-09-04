package parsercorephase0

import (
	"errors"
	"math"
	"strings"
	"sync"
	"testing"
	"unsafe"
)

func selectedStoreTestPolicy(t *testing.T, visible ...Symbol) SelectedStorePolicy {
	t.Helper()
	symbols := make([]SelectedSymbolPolicy, 16)
	for _, symbol := range visible {
		symbols[symbol] = SelectedSymbolPolicy{Visible: true, Named: true}
	}
	unary := make([]SelectedUnaryRule, len(symbols)*len(symbols))
	policy, err := NewSelectedStorePolicy(symbols, unary, visible[0])
	if err != nil {
		t.Fatal(err)
	}
	return policy
}

func TestSelectedStorePreservesRepeatedOccurrenceIdentity(t *testing.T) {
	compact, err := New(&fakeTable{}, Limits{})
	if err != nil {
		t.Fatal(err)
	}
	leaf, err := compact.appendSubtree(subtreeRecord{symbol: 1, endByte: 1, terminal: true}, nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	parent, err := compact.appendSubtree(subtreeRecord{symbol: 2, endByte: 1}, []SubtreeID{leaf, leaf}, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	store, err := compact.BuildSelectedStore([]SubtreeID{parent}, selectedStoreTestPolicy(t, 2, 1), []byte("x"), nil)
	if err != nil {
		t.Fatal(err)
	}
	root, _ := store.Record(store.Root())
	left, _ := store.Child(root, 0)
	right, _ := store.Child(root, 1)
	if left == 0 || right == 0 || left == right || store.NodeCount() != 3 {
		t.Fatalf("selected occurrences left=%d right=%d nodes=%d", left, right, store.NodeCount())
	}
}

func TestSelectedStoreElidesHiddenParentsBeforeSeal(t *testing.T) {
	compact, err := New(&fakeTable{}, Limits{})
	if err != nil {
		t.Fatal(err)
	}
	leaf, _ := compact.appendSubtree(subtreeRecord{symbol: 1, endByte: 1, terminal: true}, nil, nil, nil)
	hidden, _ := compact.appendSubtree(subtreeRecord{symbol: 3, endByte: 1}, []SubtreeID{leaf}, nil, nil)
	root, _ := compact.appendSubtree(subtreeRecord{symbol: 2, endByte: 1}, []SubtreeID{hidden}, nil, nil)
	policy := selectedStoreTestPolicy(t, 2, 1)
	store, err := compact.BuildSelectedStore([]SubtreeID{root}, policy, []byte("x"), nil)
	if err != nil {
		t.Fatal(err)
	}
	rootRecord, _ := store.Record(store.Root())
	child, _ := store.Child(rootRecord, 0)
	childRecord, _ := store.Record(child)
	if store.NodeCount() != 2 || childRecord.Symbol != 1 {
		t.Fatalf("hidden-elided nodes=%d child=%+v", store.NodeCount(), childRecord)
	}
}

func TestSelectedStorePropagatesHiddenMissingErrors(t *testing.T) {
	for _, test := range []struct {
		name           string
		nested         bool
		visibleMissing bool
	}{
		{name: "hidden"},
		{name: "nested-hidden", nested: true},
		{name: "visible", visibleMissing: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			compact, err := New(&fakeTable{}, Limits{})
			if err != nil {
				t.Fatal(err)
			}
			missing, err := compact.appendSubtree(subtreeRecord{
				symbol: 3, terminal: true, missing: true,
			}, nil, nil, nil)
			if err != nil {
				t.Fatal(err)
			}
			child := missing
			if test.nested {
				child, err = compact.appendSubtree(subtreeRecord{symbol: 4}, []SubtreeID{missing}, nil, nil)
				if err != nil {
					t.Fatal(err)
				}
			}
			root, err := compact.appendSubtree(subtreeRecord{symbol: 2}, []SubtreeID{child}, nil, nil)
			if err != nil {
				t.Fatal(err)
			}
			visible := []Symbol{2}
			if test.visibleMissing {
				visible = append(visible, 3)
			}
			store, err := compact.BuildSelectedStore([]SubtreeID{root}, selectedStoreTestPolicy(t, visible...), nil, nil)
			if err != nil {
				t.Fatal(err)
			}
			defer store.Release()
			rootRecord, ok := store.Record(store.Root())
			if !ok || !rootRecord.HasError() || rootRecord.Missing() {
				t.Fatalf("root=%+v, want an ordinary root with error content", rootRecord)
			}
			wantChildren := uint16(0)
			if test.visibleMissing {
				wantChildren = 1
			}
			if rootRecord.ChildCount != wantChildren {
				t.Fatalf("root children=%d, want %d", rootRecord.ChildCount, wantChildren)
			}
			if test.visibleMissing {
				childID, childOK := store.Child(rootRecord, 0)
				childRecord, recordOK := store.Record(childID)
				if !childOK || !recordOK || !childRecord.Missing() || !childRecord.HasError() {
					t.Fatalf("visible missing child=%+v child_ok=%t record_ok=%t", childRecord, childOK, recordOK)
				}
			}
		})
	}
}

func TestSelectedBuildScratchCountsAndReleasesErrorFlags(t *testing.T) {
	core, err := New(&fakeTable{}, Limits{})
	if err != nil {
		t.Fatal(err)
	}
	before := core.FootprintBytes()
	core.selectedBuild.resultHasError = make([]bool, 0, 17)
	want := uint64(cap(core.selectedBuild.resultHasError)) * uint64(unsafe.Sizeof(bool(false)))
	if got := core.FootprintBytes() - before; got != want {
		t.Fatalf("error-flag scratch footprint=%d, want %d", got, want)
	}

	core.selectedBuild.resultHasError = make([]bool, coreRetentionCapBytes+1)
	core.missingLeafProvenance = make([]missingLeafProvenance, 0, 1)
	core.releaseOversizedRetention()
	if core.selectedBuild.resultHasError != nil {
		t.Fatal("retention release kept oversized selected-build error flags")
	}
	if core.missingLeafProvenance != nil {
		t.Fatal("retention release kept missing-leaf provenance")
	}
}

func TestSelectedStoreRetainsCompiledAliasChildOccurrence(t *testing.T) {
	for _, folded := range []bool{false, true} {
		name := "nonfolded"
		if folded {
			name = "folded"
		}
		t.Run(name, func(t *testing.T) {
			build := func(limit uint64) (*SelectedStore, error) {
				compact, err := New(&fakeTable{}, Limits{MaxSelectedBytes: limit})
				if err != nil {
					return nil, err
				}
				leaf, err := compact.appendSubtree(subtreeRecord{
					symbol: 1, productionID: 8, dynamicPrecedence: 2, endByte: 1, terminal: true,
				}, nil, nil, nil)
				if err != nil {
					return nil, err
				}
				rawChild, childSymbol := leaf, Symbol(1)
				rootSymbol := Symbol(3)
				if folded {
					rawChild, err = compact.appendSubtree(subtreeRecord{
						symbol: 3, productionID: 9, dynamicPrecedence: 3, endByte: 1,
					}, []SubtreeID{leaf}, nil, nil)
					if err != nil {
						return nil, err
					}
					childSymbol = 3
					rootSymbol = 4
				}
				root, err := compact.appendSubtree(
					subtreeRecord{symbol: rootSymbol, productionID: 7, endByte: 1},
					[]SubtreeID{rawChild},
					[]FieldMapEntry{{FieldID: 4, ChildIndex: 0}},
					[]Symbol{2},
				)
				if err != nil {
					return nil, err
				}
				policy := selectedStoreTestPolicy(t, 2, 1)
				if folded {
					policy = selectedStoreTestPolicy(t, 2, 1, 3)
					policy.Unary[int(Symbol(3))*len(policy.Symbols)+int(Symbol(1))] = SelectedUnaryRenameLeaf
				}
				policy.SetRetainedAliasChildren([]SelectedAliasChildPair{{Alias: 2, Child: childSymbol}})
				return compact.BuildSelectedStore([]SubtreeID{root}, policy, []byte("x"), nil)
			}

			store, err := build(0)
			if err != nil {
				t.Fatal(err)
			}
			required := store.RetainedBytes()
			assertSelectedRetainedAliasShape(t, store, folded)
			store.Release()

			exact, err := build(required)
			if err != nil {
				t.Fatalf("exact retained cap %d: %v", required, err)
			}
			assertSelectedRetainedAliasShape(t, exact, folded)
			exact.Release()
			if required == 0 {
				t.Fatal("retained alias store reported zero retained bytes")
			}
			if below, err := build(required - 1); err == nil || below != nil || !strings.Contains(err.Error(), "retained-byte cap") {
				t.Fatalf("below-cap store=%v err=%v required=%d", below, err, required)
			}
		})
	}
}

func assertSelectedRetainedAliasShape(t *testing.T, store *SelectedStore, folded bool) {
	t.Helper()
	wrapper, ok := store.Record(store.Root())
	childID, childOK := store.Child(wrapper, 0)
	child, recordOK := store.Record(childID)
	wantChildSymbol := Symbol(1)
	wantPrecedence := int32(2)
	if folded {
		wantChildSymbol = 3
		wantPrecedence = 5
	}
	if !ok || !childOK || !recordOK || store.NodeCount() != 2 || store.ChildCount() != 1 ||
		wrapper.Symbol != 2 || wrapper.Field != 4 || wrapper.ChildCount != 1 || wrapper.Parent != 0 || wrapper.ChildIndex != 0 ||
		wrapper.DynamicPrecedence != wantPrecedence || child.Symbol != wantChildSymbol || child.Field != 0 ||
		child.Parent != store.Root() || child.ChildIndex != 0 || child.DynamicPrecedence != wantPrecedence {
		t.Fatalf("folded=%t wrapper=%+v child=%+v nodes=%d children=%d", folded, wrapper, child, store.NodeCount(), store.ChildCount())
	}
}

func TestSelectedStoreDoesNotCollapseExtraUnaryChild(t *testing.T) {
	compact, err := New(&fakeTable{}, Limits{})
	if err != nil {
		t.Fatal(err)
	}
	extra, _ := compact.appendSubtree(subtreeRecord{symbol: 1, endByte: 1, terminal: true, extra: true}, nil, nil, nil)
	root, _ := compact.appendSubtree(subtreeRecord{symbol: 2, endByte: 1}, []SubtreeID{extra}, nil, nil)
	policy := selectedStoreTestPolicy(t, 2, 1)
	policy.Unary[int(Symbol(2))*len(policy.Symbols)+int(Symbol(1))] = SelectedUnaryPass
	store, err := compact.BuildSelectedStore([]SubtreeID{root}, policy, []byte("x"), nil)
	if err != nil {
		t.Fatal(err)
	}
	if store.NodeCount() != 2 {
		t.Fatalf("extra unary child collapsed: nodes=%d", store.NodeCount())
	}
}

func TestSelectedStoreRejectsMalformedPolicy(t *testing.T) {
	compact, err := New(&fakeTable{}, Limits{})
	if err != nil {
		t.Fatal(err)
	}
	leaf, _ := compact.appendSubtree(subtreeRecord{symbol: 1, endByte: 1, terminal: true}, nil, nil, nil)
	valid := selectedStoreTestPolicy(t, 1)
	tests := []struct {
		name   string
		mutate func(*SelectedStorePolicy)
	}{
		{name: "unary-width", mutate: func(policy *SelectedStorePolicy) { policy.Unary = policy.Unary[:1] }},
		{name: "root", mutate: func(policy *SelectedStorePolicy) { policy.ExpectedRoot = Symbol(len(policy.Symbols)) }},
		{name: "partial-compat", mutate: func(policy *SelectedStorePolicy) { policy.Cases = make([]bool, len(policy.Symbols)) }},
		{name: "compat-width", mutate: func(policy *SelectedStorePolicy) {
			policy.SemicolonContainers = make([]bool, len(policy.Symbols)-1)
			policy.Cases = make([]bool, len(policy.Symbols))
			policy.StatementLists = make([]bool, len(policy.Symbols))
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			policy := valid
			test.mutate(&policy)
			store, err := compact.BuildSelectedStore([]SubtreeID{leaf}, policy, []byte("x"), nil)
			if err == nil || store != nil {
				t.Fatalf("malformed policy store=%v err=%v", store, err)
			}
		})
	}
}

func TestSelectedStoreEnforcesOccurrenceAndRetainedByteCaps(t *testing.T) {
	for _, test := range []struct {
		name   string
		limits Limits
		want   string
	}{
		{name: "occurrences", limits: Limits{MaxSelectedOccurrences: 2, MaxSelectedBytes: 1 << 20}, want: "occurrence cap"},
		{name: "retained-bytes", limits: Limits{MaxSelectedOccurrences: 8, MaxSelectedBytes: 1}, want: "retained-byte cap"},
	} {
		t.Run(test.name, func(t *testing.T) {
			compact, err := New(&fakeTable{}, test.limits)
			if err != nil {
				t.Fatal(err)
			}
			leaf, _ := compact.appendSubtree(subtreeRecord{symbol: 1, endByte: 1, terminal: true}, nil, nil, nil)
			root, _ := compact.appendSubtree(subtreeRecord{symbol: 2, endByte: 1}, []SubtreeID{leaf, leaf}, nil, nil)
			store, err := compact.BuildSelectedStore([]SubtreeID{root}, selectedStoreTestPolicy(t, 2, 1), []byte("x"), nil)
			if err == nil || store != nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("capped store=%v err=%v want=%q", store, err, test.want)
			}
		})
	}
}

func TestSelectedStorePrecedenceOverflowFailsClosed(t *testing.T) {
	if _, err := checkedSelectedPrecedence(math.MaxInt32, 1); err == nil {
		t.Fatal("positive selected precedence overflow succeeded")
	}
	if _, err := checkedSelectedPrecedence(math.MinInt32, -1); err == nil {
		t.Fatal("negative selected precedence overflow succeeded")
	}
}

func TestSelectedStoreDeepTraversalIsIterativeAndCancellable(t *testing.T) {
	const depth = 20_000
	compact, err := New(&fakeTable{}, Limits{MaxSubtrees: depth + 1, MaxChildren: depth + 1})
	if err != nil {
		t.Fatal(err)
	}
	root, _ := compact.appendSubtree(subtreeRecord{symbol: 1, endByte: 1, terminal: true}, nil, nil, nil)
	for index := 0; index < depth; index++ {
		root, err = compact.appendSubtree(subtreeRecord{symbol: 3, endByte: 1}, []SubtreeID{root}, nil, nil)
		if err != nil {
			t.Fatal(err)
		}
	}
	policy := selectedStoreTestPolicy(t, 1)
	store, err := compact.BuildSelectedStore([]SubtreeID{root}, policy, []byte("x"), nil)
	if err != nil {
		t.Fatal(err)
	}
	if store.NodeCount() != 1 {
		t.Fatalf("deep hidden store nodes=%d", store.NodeCount())
	}
	stop := errors.New("stop")
	polls := 0
	stopped, err := compact.BuildSelectedStore([]SubtreeID{root}, policy, []byte("x"), func() error {
		polls++
		if polls == 2 {
			return stop
		}
		return nil
	})
	if !errors.Is(err, stop) || stopped != nil {
		t.Fatalf("cancelled store=%v err=%v", stopped, err)
	}
}

func TestSelectedStoreDeclinesUnsupportedFieldProfile(t *testing.T) {
	compact, err := New(&fakeTable{}, Limits{})
	if err != nil {
		t.Fatal(err)
	}
	leaf, _ := compact.appendSubtree(subtreeRecord{symbol: 1, endByte: 1, terminal: true}, nil, nil, nil)
	parent, _ := compact.appendSubtree(subtreeRecord{symbol: 2, endByte: 1}, []SubtreeID{leaf}, []FieldMapEntry{{FieldID: 1, ChildIndex: 0, Inherited: true}}, nil)
	store, err := compact.BuildSelectedStore([]SubtreeID{parent}, selectedStoreTestPolicy(t, 2, 1), []byte("x"), nil)
	if err == nil || store != nil || !strings.Contains(err.Error(), "outside admitted") {
		t.Fatalf("unsupported field store=%v err=%v", store, err)
	}
}

type selectedStorePolicyTable struct {
	fakeTable
	policy SelectedStorePolicy
	calls  int
}

func (t *selectedStorePolicyTable) SelectedStorePolicy() (SelectedStorePolicy, error) {
	t.calls++
	return t.policy, nil
}

func TestSelectedStoreAuthenticatedPolicyCapability(t *testing.T) {
	policy := selectedStoreTestPolicy(t, 1)
	tables := &selectedStorePolicyTable{policy: policy}
	compact, err := New(tables, Limits{})
	if err != nil {
		t.Fatal(err)
	}
	if tables.calls != 0 {
		t.Fatalf("selected policy was built eagerly: calls=%d", tables.calls)
	}
	leaf, _ := compact.appendSubtree(subtreeRecord{symbol: 1, endByte: 1, terminal: true}, nil, nil, nil)
	store, err := compact.BuildAuthenticatedSelectedStore([]SubtreeID{leaf}, []byte("x"), nil)
	if err != nil || store == nil || store.NodeCount() != 1 {
		t.Fatalf("authenticated store=%v err=%v", store, err)
	}
	if tables.calls != 1 {
		t.Fatalf("selected policy lazy calls=%d want=1", tables.calls)
	}
	if err := compact.Reset(); err != nil {
		t.Fatal(err)
	}
	if record, ok := store.Record(store.Root()); !ok || record.Symbol != 1 || record.StartByte != 0 || record.EndByte != 1 {
		t.Fatalf("sealed store changed after core reset: record=%+v ok=%t", record, ok)
	}
	if stale, err := compact.BuildAuthenticatedSelectedStore([]SubtreeID{leaf}, []byte("x"), nil); err == nil || stale != nil {
		t.Fatalf("stale accepted roots store=%v err=%v", stale, err)
	}
	retained := store.RetainedBytes()
	store.Release()
	store.Release()
	if retained == 0 || store.Root() != 0 || store.NodeCount() != 0 || store.ChildCount() != 0 || store.RetainedBytes() != 0 {
		t.Fatalf("released store remained readable: retained=%d root=%d nodes=%d children=%d bytes=%d",
			retained, store.Root(), store.NodeCount(), store.ChildCount(), store.RetainedBytes())
	}
	if _, ok := store.Record(1); ok {
		t.Fatal("released store record remained readable")
	}
	if _, ok := store.Cursor().Record(); ok {
		t.Fatal("released store cursor remained readable")
	}
	freshLeaf, _ := compact.appendSubtree(subtreeRecord{symbol: 1, endByte: 1, terminal: true}, nil, nil, nil)
	freshStore, err := compact.BuildAuthenticatedSelectedStore([]SubtreeID{freshLeaf}, []byte("x"), nil)
	if err != nil || freshStore == nil || freshStore.NodeCount() != 1 {
		t.Fatalf("store backing was not reusable after release: store=%v err=%v", freshStore, err)
	}
	if tables.calls != 1 {
		t.Fatalf("selected policy was rebuilt: calls=%d", tables.calls)
	}
	freshStore.Release()
	withoutPolicy, _ := New(&fakeTable{}, Limits{})
	if missing, err := withoutPolicy.BuildAuthenticatedSelectedStore([]SubtreeID{1}, []byte("x"), nil); err == nil || missing != nil {
		t.Fatalf("missing policy store=%v err=%v", missing, err)
	}
}

func TestSelectedStoreAuthenticatedPolicyCancellationPrecedesLazyBuild(t *testing.T) {
	policy := selectedStoreTestPolicy(t, 1)
	tables := &selectedStorePolicyTable{policy: policy}
	compact, err := New(tables, Limits{})
	if err != nil {
		t.Fatal(err)
	}
	stop := errors.New("stop before policy")
	store, err := compact.BuildAuthenticatedSelectedStore([]SubtreeID{1}, []byte("x"), func() error { return stop })
	if !errors.Is(err, stop) || store != nil {
		t.Fatalf("cancelled authenticated store=%v err=%v", store, err)
	}
	if tables.calls != 0 {
		t.Fatalf("cancelled authenticated policy calls=%d want=0", tables.calls)
	}
}

func TestSelectedStoreRootMergePreflightsCapAndLaterBuildRemainsBounded(t *testing.T) {
	recordBytes := uint64(unsafe.Sizeof(SelectedNodeRecord{}))
	childBytes := uint64(unsafe.Sizeof(SelectedNodeID(0)))
	limit := 3*recordBytes + childBytes
	compact, err := New(&fakeTable{}, Limits{MaxSelectedOccurrences: 8, MaxSelectedBytes: limit})
	if err != nil {
		t.Fatal(err)
	}
	child, _ := compact.appendSubtree(subtreeRecord{symbol: 1, endByte: 1, terminal: true}, nil, nil, nil)
	root, _ := compact.appendSubtree(subtreeRecord{symbol: 2, endByte: 1}, []SubtreeID{child}, nil, nil)
	extra, _ := compact.appendSubtree(subtreeRecord{symbol: 3, endByte: 1, terminal: true, extra: true}, nil, nil, nil)
	policy := selectedStoreTestPolicy(t, 2, 1, 3)
	if store, err := compact.BuildSelectedStore([]SubtreeID{extra, root}, policy, []byte("x"), nil); err == nil || store != nil || !strings.Contains(err.Error(), "retained-byte cap") {
		t.Fatalf("over-cap merged roots store=%v err=%v", store, err)
	}
	store, err := compact.BuildSelectedStore([]SubtreeID{root}, policy, []byte("x"), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Release()
	if got := store.RetainedBytes(); got > limit {
		t.Fatalf("later selected store retained=%d cap=%d", got, limit)
	}
}

func TestSelectedStoreRootMergeRelocatesLaterChildWindows(t *testing.T) {
	compact, err := New(&fakeTable{}, Limits{})
	if err != nil {
		t.Fatal(err)
	}
	rootChild, _ := compact.appendSubtree(subtreeRecord{symbol: 1, endByte: 1, terminal: true}, nil, nil, nil)
	root, _ := compact.appendSubtree(subtreeRecord{symbol: 2, endByte: 1}, []SubtreeID{rootChild}, nil, nil)
	extraChild, _ := compact.appendSubtree(subtreeRecord{symbol: 4, endByte: 1, terminal: true}, nil, nil, nil)
	extra, _ := compact.appendSubtree(subtreeRecord{symbol: 3, endByte: 1, extra: true}, []SubtreeID{extraChild}, nil, nil)
	store, err := compact.BuildSelectedStore([]SubtreeID{extra, root}, selectedStoreTestPolicy(t, 2, 1, 3, 4), []byte("x"), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Release()
	rootRecord, _ := store.Record(store.Root())
	extraID, ok := store.Child(rootRecord, 0)
	if !ok {
		t.Fatal("merged extra root is missing")
	}
	extraRecord, _ := store.Record(extraID)
	extraChildID, ok := store.Child(extraRecord, 0)
	extraChildRecord, _ := store.Record(extraChildID)
	if !ok || extraRecord.Symbol != 3 || extraRecord.ChildCount != 1 || extraChildRecord.Symbol != 4 {
		t.Fatalf("relocated extra window extra=%+v child=%+v ok=%t", extraRecord, extraChildRecord, ok)
	}
}

func TestSelectedStoreRejectsNonExtraSiblingRootAndRecoversLifecycle(t *testing.T) {
	compact, err := New(&fakeTable{}, Limits{})
	if err != nil {
		t.Fatal(err)
	}
	root, _ := compact.appendSubtree(subtreeRecord{symbol: 2, endByte: 1, terminal: true}, nil, nil, nil)
	other, _ := compact.appendSubtree(subtreeRecord{symbol: 3, endByte: 1, terminal: true}, nil, nil, nil)
	policy := selectedStoreTestPolicy(t, 2, 3)
	if store, err := compact.BuildSelectedStore([]SubtreeID{other, root}, policy, []byte("x"), nil); err == nil || store != nil || !strings.Contains(err.Error(), "non-extra sibling root") {
		t.Fatalf("non-extra sibling store=%v err=%v", store, err)
	}
	if got := selectedStoreRetainedBytes(cap(compact.selectedPool.records), cap(compact.selectedPool.children)); got > compact.limits.MaxSelectedBytes {
		t.Fatalf("failed sibling lifecycle pooled=%d cap=%d", got, compact.limits.MaxSelectedBytes)
	}
	store, err := compact.BuildSelectedStore([]SubtreeID{root}, policy, []byte("x"), nil)
	if err != nil {
		t.Fatal(err)
	}
	store.Release()
}

func TestSelectedStorePoolKeepsAtomicCappedBackingPair(t *testing.T) {
	recordBytes := uint64(unsafe.Sizeof(SelectedNodeRecord{}))
	childBytes := uint64(unsafe.Sizeof(SelectedNodeID(0)))
	type capacities struct{ records, children int }
	largeRecords := capacities{records: 20, children: 1}
	largeChildren := capacities{records: 1, children: 100}
	limit := max(
		uint64(largeRecords.records)*recordBytes+uint64(largeRecords.children)*childBytes,
		uint64(largeChildren.records)*recordBytes+uint64(largeChildren.children)*childBytes,
	)
	for _, order := range []struct {
		name        string
		first, last capacities
	}{
		{name: "records-then-children", first: largeRecords, last: largeChildren},
		{name: "children-then-records", first: largeChildren, last: largeRecords},
	} {
		t.Run(order.name, func(t *testing.T) {
			compact, err := New(&fakeTable{}, Limits{MaxSelectedBytes: limit})
			if err != nil {
				t.Fatal(err)
			}
			makeStore := func(c capacities) *SelectedStore {
				return &SelectedStore{owner: compact, records: make([]SelectedNodeRecord, 0, c.records), children: make([]SelectedNodeID, 0, c.children)}
			}
			first, last := makeStore(order.first), makeStore(order.last)
			first.Release()
			last.Release()
			pooled := compact.selectedPool
			if got := selectedStoreRetainedBytes(cap(pooled.records), cap(pooled.children)); got > limit {
				t.Fatalf("pooled pair retained=%d cap=%d", got, limit)
			}
			got := capacities{records: cap(pooled.records), children: cap(pooled.children)}
			if got != largeRecords && got != largeChildren {
				t.Fatalf("pooled pair synthesized capacities=%+v", got)
			}
		})
	}
}

func TestSelectedStoreSiblingReleasesAreSynchronized(t *testing.T) {
	compact, err := New(&fakeTable{}, Limits{})
	if err != nil {
		t.Fatal(err)
	}
	left := &SelectedStore{owner: compact, records: make([]SelectedNodeRecord, 0, 8), children: make([]SelectedNodeID, 0, 8)}
	right := &SelectedStore{owner: compact, records: make([]SelectedNodeRecord, 0, 4), children: make([]SelectedNodeID, 0, 4)}
	var wait sync.WaitGroup
	wait.Add(2)
	go func() { defer wait.Done(); left.Release() }()
	go func() { defer wait.Done(); right.Release() }()
	wait.Wait()
	compact.selectedPoolMu.Lock()
	retained := selectedStoreRetainedBytes(cap(compact.selectedPool.records), cap(compact.selectedPool.children))
	compact.selectedPoolMu.Unlock()
	if retained > compact.limits.MaxSelectedBytes {
		t.Fatalf("concurrent releases retained=%d cap=%d", retained, compact.limits.MaxSelectedBytes)
	}
}

func TestSelectedStoreRealSnapshotsSurviveResetAndReleaseOrders(t *testing.T) {
	for _, mode := range []string{"first-then-second", "second-then-first", "concurrent"} {
		t.Run(mode, func(t *testing.T) {
			compact, err := New(&fakeTable{}, Limits{})
			if err != nil {
				t.Fatal(err)
			}
			policy := selectedStoreTestPolicy(t, 2, 1)
			build := func(start, end uint32) *SelectedStore {
				leaf, appendErr := compact.appendSubtree(subtreeRecord{symbol: 1, startByte: start, endByte: end, terminal: true}, nil, nil, nil)
				if appendErr != nil {
					t.Fatal(appendErr)
				}
				root, appendErr := compact.appendSubtree(subtreeRecord{symbol: 2, endByte: end}, []SubtreeID{leaf}, nil, nil)
				if appendErr != nil {
					t.Fatal(appendErr)
				}
				store, buildErr := compact.BuildSelectedStore([]SubtreeID{root}, policy, []byte("xx"), nil)
				if buildErr != nil {
					t.Fatal(buildErr)
				}
				return store
			}
			first := build(0, 1)
			if err := compact.Reset(); err != nil {
				t.Fatal(err)
			}
			second := build(1, 2)
			validate := func(name string, store *SelectedStore, start, end uint32) {
				t.Helper()
				root, ok := store.Record(store.Root())
				if !ok || root.Symbol != 2 || root.ChildCount != 1 {
					t.Fatalf("%s root=%+v ok=%t", name, root, ok)
				}
				childID, ok := store.Child(root, 0)
				child, childOK := store.Record(childID)
				if !ok || !childOK || child.Symbol != 1 || child.StartByte != start || child.EndByte != end || child.Parent != store.Root() {
					t.Fatalf("%s child=%+v id=%d ok=%t/%t", name, child, childID, ok, childOK)
				}
			}
			validate("first", first, 0, 1)
			validate("second", second, 1, 2)
			switch mode {
			case "first-then-second":
				first.Release()
				second.Release()
			case "second-then-first":
				second.Release()
				first.Release()
			case "concurrent":
				var wait sync.WaitGroup
				wait.Add(2)
				go func() { defer wait.Done(); first.Release() }()
				go func() { defer wait.Done(); second.Release() }()
				wait.Wait()
			}
			if first.Root() != 0 || second.Root() != 0 || first.RetainedBytes() != 0 || second.RetainedBytes() != 0 {
				t.Fatalf("released snapshots remained readable: first=%d/%d second=%d/%d", first.Root(), first.RetainedBytes(), second.Root(), second.RetainedBytes())
			}
			compact.selectedPoolMu.Lock()
			retained := selectedStoreRetainedBytes(cap(compact.selectedPool.records), cap(compact.selectedPool.children))
			compact.selectedPoolMu.Unlock()
			if retained > compact.limits.MaxSelectedBytes {
				t.Fatalf("pooled real snapshot backing=%d cap=%d", retained, compact.limits.MaxSelectedBytes)
			}
		})
	}
}

func TestSelectedStoreRootGrowthCopyPollsCancellation(t *testing.T) {
	const childCount = 2048
	compact, err := New(&fakeTable{}, Limits{})
	if err != nil {
		t.Fatal(err)
	}
	records := make([]SelectedNodeRecord, childCount*2+1)
	children := make([]SelectedNodeID, childCount)
	for index := 0; index < childCount; index++ {
		children[index] = SelectedNodeID(index + 1)
		records[childCount+index] = SelectedNodeRecord{Symbol: 3, flags: selectedNodeFlagExtra}
	}
	realID := SelectedNodeID(len(records))
	records[realID-1] = SelectedNodeRecord{Symbol: 2, ChildCount: childCount}
	store := &SelectedStore{owner: compact, records: records, children: children}
	defer store.Release()
	roots := make([]SelectedNodeID, 0, childCount+1)
	for index := 0; index < childCount; index++ {
		roots = append(roots, SelectedNodeID(childCount+index+1))
	}
	roots = append(roots, realID)
	stop := errors.New("stop during selected root growth")
	polls := 0
	err = store.finishRoots(roots, selectedStoreTestPolicy(t, 2, 3), math.MaxUint64, func() error {
		polls++
		if polls == 2 {
			return stop
		}
		return nil
	})
	if !errors.Is(err, stop) || polls != 2 {
		t.Fatalf("root growth err=%v polls=%d", err, polls)
	}
	if store.Root() != 0 || len(store.children) != childCount || cap(store.children) != childCount {
		t.Fatalf("cancelled root growth published state: root=%d len=%d cap=%d", store.Root(), len(store.children), cap(store.children))
	}
}

func TestSelectedRootMergeCountsPreflightArithmetic(t *testing.T) {
	tests := []struct {
		name                          string
		children, roots, realChildren int
		wantMerged, wantFinal         int
		wantErr                       bool
	}{
		{name: "ordinary", children: 10, roots: 3, realChildren: 2, wantMerged: 4, wantFinal: 12},
		{name: "merged-overflow", children: 0, roots: math.MaxUint16 + 2, wantErr: true},
		{name: "int-addition-overflow", children: int(^uint(0) >> 1), roots: 2, wantErr: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			merged, final, err := selectedRootMergeCounts(test.children, test.roots, uint16(test.realChildren))
			if (err != nil) != test.wantErr {
				t.Fatalf("selectedRootMergeCounts() error=%v wantErr=%t", err, test.wantErr)
			}
			if !test.wantErr && (merged != test.wantMerged || final != test.wantFinal) {
				t.Fatalf("selectedRootMergeCounts()=(%d,%d) want=(%d,%d)", merged, final, test.wantMerged, test.wantFinal)
			}
		})
	}
}

func TestSelectedCursorWalksSealedStore(t *testing.T) {
	compact, _ := New(&fakeTable{}, Limits{})
	left, _ := compact.appendSubtree(subtreeRecord{symbol: 1, endByte: 1, terminal: true}, nil, nil, nil)
	right, _ := compact.appendSubtree(subtreeRecord{symbol: 1, startByte: 1, endByte: 2, terminal: true}, nil, nil, nil)
	root, _ := compact.appendSubtree(subtreeRecord{symbol: 2, endByte: 2}, []SubtreeID{left, right}, nil, nil)
	store, err := compact.BuildSelectedStore([]SubtreeID{root}, selectedStoreTestPolicy(t, 2, 1), []byte("xx"), nil)
	if err != nil {
		t.Fatal(err)
	}
	cursor := store.Cursor()
	row, ok := cursor.Record()
	if !ok || row.Symbol != 2 || row.ChildCount != 2 {
		t.Fatalf("root cursor=%+v ok=%t", row, ok)
	}
	child, ok := cursor.Child(1)
	if !ok {
		t.Fatal("cursor child unavailable")
	}
	parent, ok := child.Parent()
	if !ok || parent.ID() != cursor.ID() {
		t.Fatalf("cursor parent=%d ok=%t", parent.ID(), ok)
	}
	first, _ := cursor.Child(0)
	next, ok := first.NextSibling()
	if !ok || next.ID() != child.ID() {
		t.Fatalf("cursor next sibling=%d ok=%t want=%d", next.ID(), ok, child.ID())
	}
}
