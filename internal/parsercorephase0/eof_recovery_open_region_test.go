package parsercorephase0

import (
	"errors"
	"reflect"
	"testing"
)

func TestRecoverEOFAcceptOpenRegionFlattensAndPricesOnce(t *testing.T) {
	fixture := newRecoverEOFAcceptFixture(t)
	c := fixture.core
	child := appendAncestorRecoveryPayload(t, c, 13, 3, 4, false)
	var head Head
	var root SubtreeID
	calls := 0
	err := c.ApplySchedulerAtomic(func(owner SchedulerTransactionToken) error {
		var err error
		head, root, err = c.RecoverEOFAcceptWithOpenRegionAndCostOwned(owner, fixture.head, 2, 4, []SubtreeID{child}, func(prev NodeID, id SubtreeID) (uint32, error) {
			calls++
			prefix, err := c.RecoveryStoredErrorCost(Head{Node: prev})
			if err != nil || prefix != 0 {
				t.Fatalf("replacement inherited cost %d: %v", prefix, err)
			}
			view, err := c.Subtree(id)
			if err != nil {
				return 0, err
			}
			want := append(append([]SubtreeID(nil), fixture.payload...), child)
			if view.Symbol != ErrorRegionSymbol || view.Extra || view.StartByte != 0 || view.EndByte != 4 || !reflect.DeepEqual(view.Children, want) {
				t.Fatalf("wrong flattened EOF root: %+v", view)
			}
			return 834, nil
		})
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	if calls != 1 || !c.IsRecoverEOFAcceptRoot(root) {
		t.Fatalf("cost calls=%d root=%d", calls, root)
	}
	if cost, err := c.RecoveryStoredErrorCost(head); err != nil || cost != 834 {
		t.Fatalf("cost=%d error=%v", cost, err)
	}
	paths, err := c.Derivations(head)
	if err != nil || len(paths) != 1 || !reflect.DeepEqual(paths[0].Payloads, []SubtreeID{root}) {
		t.Fatalf("replacement paths=%+v error=%v", paths, err)
	}
}

func TestRecoverEOFAcceptOpenRegionPreservesPrecedence(t *testing.T) {
	fixture := newRecoverEOFAcceptFixture(t)
	c := fixture.core
	leaf := appendAncestorRecoveryPayload(t, c, 13, 2, 3, false)
	head, err := c.appendPrivate(10, 3, linkInput{prev: fixture.head.Node, payload: leaf, scoreDelta: 9})
	if err != nil {
		t.Fatal(err)
	}
	child := appendAncestorRecoveryPayload(t, c, 14, 3, 4, false)
	err = c.ApplySchedulerAtomic(func(owner SchedulerTransactionToken) error {
		var err error
		head, _, err = c.RecoverEOFAcceptWithOpenRegionAndCostOwned(owner, head, 3, 4, []SubtreeID{child}, func(NodeID, SubtreeID) (uint32, error) { return 1, nil })
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	paths, err := c.Derivations(head)
	if err != nil || len(paths) != 1 || paths[0].Score != 9 {
		t.Fatalf("lost dynamic precedence: paths=%+v error=%v", paths, err)
	}
}

func TestRecoverEOFAcceptOpenRegionFailuresRollBack(t *testing.T) {
	for _, name := range []string{"empty", "span", "unknown-child", "overlap", "cost", "outer", "panic", "limit", "missing", "extra", "unknown-scanner"} {
		t.Run(name, func(t *testing.T) {
			fixture := newRecoverEOFAcceptFixture(t)
			c := fixture.core
			child := appendAncestorRecoveryPayload(t, c, 13, 2, 3, false)
			children := []SubtreeID{child}
			start, end := uint32(2), uint32(3)
			cost := ReductionOutputCostFunc(func(NodeID, SubtreeID) (uint32, error) { return 1, nil })
			sentinel := errors.New("stop EOF recovery")
			switch name {
			case "empty":
				children = nil
			case "span":
				end = 1
			case "unknown-child":
				children[0] = SubtreeID(len(c.subtrees) + 1)
			case "overlap":
				start = 0
				children[0] = fixture.payload[0]
			case "cost":
				cost = func(NodeID, SubtreeID) (uint32, error) { return 0, sentinel }
			case "panic":
				cost = func(NodeID, SubtreeID) (uint32, error) { panic(sentinel) }
			case "limit":
				c.limits.MaxChildren = 1
			case "missing":
				c.subtrees[child-1].missing = true
			case "extra":
				c.subtrees[child-1].extra = true
			case "unknown-scanner":
				c.externalPayloadsQuiescent = false
				c.subtrees[child-1].external = true
				c.subtrees[child-1].externalProvenanceState = subtreeExternalProvenanceInexactHasExternal
			}
			before := captureSchedulerTransactionState(c)
			var gotErr error
			func() {
				defer func() {
					if value := recover(); value != nil && name != "panic" {
						panic(value)
					}
				}()
				gotErr = c.ApplySchedulerAtomic(func(owner SchedulerTransactionToken) error {
					_, _, _ = c.RecoverEOFAcceptWithOpenRegionAndCostOwned(owner, fixture.head, start, end, children, cost)
					if name == "outer" {
						return sentinel
					}
					return nil
				})
			}()
			if name != "panic" && gotErr == nil {
				t.Fatal("ignored EOF failure committed")
			}
			if after := captureSchedulerTransactionState(c); !reflect.DeepEqual(before, after) {
				t.Fatal("failed EOF recovery changed storage")
			}
			assertTransactionJournalClean(t, c)
		})
	}
}
