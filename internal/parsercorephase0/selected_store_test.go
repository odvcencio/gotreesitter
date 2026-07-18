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
