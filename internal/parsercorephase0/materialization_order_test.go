package parsercorephase0

import (
	"strings"
	"testing"
)

func TestMaterializationOrderRejectsRepeatedOwnership(t *testing.T) {
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
	if _, err := compact.MaterializationOrder([]SubtreeID{parent}, nil); err == nil || !strings.Contains(err.Error(), "repeated public-tree ownership") {
		t.Fatalf("repeated compact child error = %v", err)
	}
}

func TestMaterializationOrderRejectsCycle(t *testing.T) {
	compact, err := New(&fakeTable{}, Limits{})
	if err != nil {
		t.Fatal(err)
	}
	leaf, err := compact.appendSubtree(subtreeRecord{symbol: 1, endByte: 1, terminal: true}, nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	parent, err := compact.appendSubtree(subtreeRecord{symbol: 2, endByte: 1}, []SubtreeID{leaf}, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	record, err := compact.subtree(parent)
	if err != nil {
		t.Fatal(err)
	}
	compact.children[record.firstChild] = parent
	if _, err := compact.MaterializationOrder([]SubtreeID{parent}, nil); err == nil || !strings.Contains(err.Error(), "cycle") {
		t.Fatalf("compact cycle error = %v", err)
	}
}

func TestMaterializationOrderValidatesRemappedMetadata(t *testing.T) {
	tables := &fakeTable{
		fields: map[uint16][]FieldMapEntry{7: {{FieldID: 3, ChildIndex: 0}}},
		aliases: map[productionKey][]Symbol{
			{productionID: 7, childCount: 1}: {9},
		},
	}
	compact, err := New(tables, Limits{})
	if err != nil {
		t.Fatal(err)
	}
	extra, err := compact.appendSubtree(subtreeRecord{symbol: 1, endByte: 1, extra: true, terminal: true}, nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	child, err := compact.appendSubtree(subtreeRecord{symbol: 2, endByte: 1, terminal: true}, nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	parent, err := compact.appendSubtree(
		subtreeRecord{symbol: 3, productionID: 7, endByte: 1},
		[]SubtreeID{extra, child},
		[]FieldMapEntry{{FieldID: 3, ChildIndex: 1}},
		[]Symbol{0, 9},
	)
	if err != nil {
		t.Fatal(err)
	}
	order, err := compact.MaterializationOrder([]SubtreeID{parent}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(order) != 3 || order[2] != parent {
		t.Fatalf("materialization order = %v", order)
	}

	record, err := compact.subtree(parent)
	if err != nil {
		t.Fatal(err)
	}
	compact.fields[record.firstField].ChildIndex = 0
	if _, err := compact.MaterializationOrder([]SubtreeID{parent}, nil); err == nil || !strings.Contains(err.Error(), "metadata does not match") {
		t.Fatalf("field metadata mismatch error = %v", err)
	}
	compact.fields[record.firstField].ChildIndex = 1
	compact.aliases[record.firstAlias+1] = 8
	if _, err := compact.MaterializationOrder([]SubtreeID{parent}, nil); err == nil || !strings.Contains(err.Error(), "metadata does not match") {
		t.Fatalf("alias metadata mismatch error = %v", err)
	}
}

func TestMaterializationOrderPollsAndPublishesNothingOnStop(t *testing.T) {
	compact, err := New(&fakeTable{}, Limits{})
	if err != nil {
		t.Fatal(err)
	}
	leaf, err := compact.appendSubtree(subtreeRecord{symbol: 1, endByte: 1, terminal: true}, nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	polls := 0
	if order, err := compact.MaterializationOrder([]SubtreeID{leaf}, func() error {
		polls++
		return errMaterializationOrderStop
	}); err != errMaterializationOrderStop || order != nil || polls != 1 {
		t.Fatalf("stopped order=%v err=%v polls=%d", order, err, polls)
	}
}

var errMaterializationOrderStop = materializationOrderTestError("stop")

type materializationOrderTestError string

func (err materializationOrderTestError) Error() string { return string(err) }
