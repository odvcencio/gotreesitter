package parsercorephase0

import (
	"errors"
	"strings"
	"testing"
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
}

func (t *selectedStorePolicyTable) SelectedStorePolicy() (SelectedStorePolicy, error) {
	return t.policy, nil
}

func TestSelectedStoreAuthenticatedPolicyCapability(t *testing.T) {
	policy := selectedStoreTestPolicy(t, 1)
	tables := &selectedStorePolicyTable{policy: policy}
	compact, err := New(tables, Limits{})
	if err != nil {
		t.Fatal(err)
	}
	leaf, _ := compact.appendSubtree(subtreeRecord{symbol: 1, endByte: 1, terminal: true}, nil, nil, nil)
	store, err := compact.BuildAuthenticatedSelectedStore([]SubtreeID{leaf}, []byte("x"), nil)
	if err != nil || store == nil || store.NodeCount() != 1 {
		t.Fatalf("authenticated store=%v err=%v", store, err)
	}
	if err := compact.Reset(); err != nil {
		t.Fatal(err)
	}
	if stale, err := compact.BuildAuthenticatedSelectedStore([]SubtreeID{leaf}, []byte("x"), nil); err == nil || stale != nil {
		t.Fatalf("stale accepted roots store=%v err=%v", stale, err)
	}
	withoutPolicy, _ := New(&fakeTable{}, Limits{})
	if missing, err := withoutPolicy.BuildAuthenticatedSelectedStore([]SubtreeID{1}, []byte("x"), nil); err == nil || missing != nil {
		t.Fatalf("missing policy store=%v err=%v", missing, err)
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
