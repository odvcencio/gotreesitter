package parsercorephase0

import (
	"errors"
	"math"
	"reflect"
	"testing"
)

func openRegionAncestorFixture(t *testing.T) (*Core, StackSummaryCandidate, []SubtreeID) {
	t.Helper()
	c := newAncestorRecoveryTestCore(t, &fakeTable{}, Limits{})
	seed, err := c.Seed(1, 0)
	if err != nil {
		t.Fatal(err)
	}
	ordinary := appendAncestorRecoveryPayload(t, c, 1, 0, 1, false)
	extra := appendAncestorRecoveryPayload(t, c, 2, 1, 2, true)
	head := appendAncestorRecoveryHead(t, c, seed, 2, ordinary)
	head = appendAncestorRecoveryHead(t, c, head, 3, extra)
	candidates, err := c.StackSummaryCandidates(head, 2)
	if err != nil {
		t.Fatal(err)
	}
	region := appendAncestorRecoveryPayload(t, c, 3, 3, 4, false)
	regionExtra := appendAncestorRecoveryPayload(t, c, 4, 4, 5, true)
	return c, ancestorRecoveryCandidateForState(t, candidates, 1), []SubtreeID{ordinary, extra, region, regionExtra}
}

func TestRecoverToAncestorOpenRegionScoreOverflow(t *testing.T) {
	c := newAncestorRecoveryTestCore(t, &fakeTable{}, Limits{})
	head, err := c.Seed(1, 0)
	if err != nil {
		t.Fatal(err)
	}
	for i, score := range []int64{math.MaxInt64, 1} {
		payload := appendAncestorRecoveryPayload(t, c, 1, uint32(i), uint32(i+1), false)
		head, err = c.appendPrivate(StateID(i+2), uint32(i+1), linkInput{prev: head.Node, payload: payload, scoreDelta: score})
		if err != nil {
			t.Fatal(err)
		}
	}
	candidates, err := c.StackSummaryCandidates(head, 2)
	if err != nil {
		t.Fatal(err)
	}
	candidate := ancestorRecoveryCandidateForState(t, candidates, 1)
	child := appendAncestorRecoveryPayload(t, c, 3, 2, 3, false)
	before := captureSchedulerTransactionState(c)
	err = c.ApplySchedulerAtomic(func(owner SchedulerTransactionToken) error {
		_, _ = c.RecoverToAncestorStateWithOpenRegionAndCostOwned(owner, candidate, 2, 3, []SubtreeID{child}, func(NodeID, SubtreeID) (uint32, error) {
			t.Fatal("overflow reached cost publication")
			return 0, nil
		})
		return nil
	})
	if err == nil {
		t.Fatal("score overflow committed")
	}
	if after := captureSchedulerTransactionState(c); !reflect.DeepEqual(before, after) {
		t.Fatal("score overflow changed storage")
	}
}

func priorErrorAncestorFixture(t *testing.T) (*Core, StackSummaryCandidate, SubtreeID, NodeID, []SubtreeID) {
	t.Helper()
	c := newAncestorRecoveryTestCore(t, &fakeTable{}, Limits{})
	seed, err := c.Seed(1, 0)
	if err != nil {
		t.Fatal(err)
	}
	priorChild := appendAncestorRecoveryPayload(t, c, 1, 0, 1, false)
	prior, err := c.appendSubtree(subtreeRecord{symbol: ErrorRegionSymbol, startByte: 0, endByte: 1, extra: true}, []SubtreeID{priorChild}, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	target := appendAncestorRecoveryHead(t, c, seed, 2, prior)
	c.nodeLineages[target.Node-1].storedErrorCost = 601
	ordinary := appendAncestorRecoveryPayload(t, c, 2, 1, 2, false)
	head := appendAncestorRecoveryHead(t, c, target, 3, ordinary)
	candidates, err := c.StackSummaryCandidates(head, 1)
	if err != nil {
		t.Fatal(err)
	}
	candidate := ancestorRecoveryCandidateForState(t, candidates, 2)
	region := appendAncestorRecoveryPayload(t, c, 3, 2, 3, false)
	return c, candidate, region, target.Node, []SubtreeID{priorChild, ordinary, region}
}

func TestRecoverToAncestorOpenRegionPopsPriorError(t *testing.T) {
	c, candidate, region, target, children := priorErrorAncestorFixture(t)
	node, err := c.node(target)
	if err != nil {
		t.Fatal(err)
	}
	c.links[node.firstLink-1].scoreDelta = 7
	var recovered Head
	err = c.ApplySchedulerAtomic(func(owner SchedulerTransactionToken) error {
		var err error
		recovered, err = c.RecoverToAncestorStateWithOpenRegionAndCostOwned(owner, candidate, 2, 3, []SubtreeID{region}, func(prev NodeID, id SubtreeID) (uint32, error) {
			prefix, err := c.RecoveryStoredErrorCost(Head{Node: prev})
			if err != nil {
				return 0, err
			}
			view, err := c.Subtree(id)
			if err != nil {
				return 0, err
			}
			if prev != 1 || prefix != 0 || view.StartByte != 0 || !reflect.DeepEqual(view.Children, children) {
				t.Errorf("prior ERROR retained: prev=%d prefixCost=%d region=%+v", prev, prefix, view)
			}
			return prefix + 803, nil
		})
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	paths, err := c.Derivations(recovered)
	if err != nil || len(paths) != 1 || paths[0].Score != 7 || len(paths[0].Payloads) != 1 {
		t.Fatalf("prior ERROR score or topology changed: paths=%+v err=%v", paths, err)
	}
	if cost, err := c.RecoveryStoredErrorCost(recovered); err != nil || cost != 803 {
		t.Fatalf("replacement ERROR cost=%d err=%v, want 803", cost, err)
	}
}

func TestRecoverToAncestorOpenRegionPriorErrorRollback(t *testing.T) {
	for _, name := range []string{"ambiguous", "childless", "bad-span", "cost", "outer", "old-api"} {
		t.Run(name, func(t *testing.T) {
			c, candidate, region, target, all := priorErrorAncestorFixture(t)
			node, _ := c.node(target)
			links, err := c.publishedNodeLinksInto(nil, *node)
			if err != nil {
				t.Fatal(err)
			}
			priorID := links[0].payload
			cost := ReductionOutputCostFunc(func(NodeID, SubtreeID) (uint32, error) { return 803, nil })
			sentinel := errors.New("prior ERROR rollback")
			switch name {
			case "ambiguous":
				alternate := appendAncestorRecoveryPayload(t, c, 9, 0, 1, true)
				target, err = c.appendAdjacencyNode(2, 1, []linkRecord{links[0], {prev: links[0].prev, payload: alternate}})
				if err != nil {
					t.Fatal(err)
				}
				head := appendAncestorRecoveryHead(t, c, Head{Node: target}, 3, all[1])
				candidates, err := c.StackSummaryCandidates(head, 1)
				if err != nil {
					t.Fatal(err)
				}
				candidate = ancestorRecoveryCandidateForState(t, candidates, 2)
			case "childless":
				c.subtrees[priorID-1].childCount = 0
			case "bad-span":
				c.subtrees[priorID-1].endByte = 2
			case "cost":
				cost = func(NodeID, SubtreeID) (uint32, error) { return 0, sentinel }
			}
			before := captureSchedulerTransactionState(c)
			err = c.ApplySchedulerAtomic(func(owner SchedulerTransactionToken) error {
				if name == "old-api" {
					_, err := c.RecoverToAncestorStateWithCostOwned(owner, candidate, func(prev NodeID, _ SubtreeID) (uint32, error) {
						if prev != target {
							t.Fatal("old API popped the prior ERROR")
						}
						return 803, nil
					})
					return err
				}
				_, _ = c.RecoverToAncestorStateWithOpenRegionAndCostOwned(owner, candidate, 2, 3, []SubtreeID{region}, cost)
				if name == "outer" {
					return sentinel
				}
				return nil
			})
			if name == "old-api" {
				if err != nil {
					t.Fatal(err)
				}
				return
			}
			if err == nil {
				t.Fatal("invalid prior ERROR committed")
			}
			if after := captureSchedulerTransactionState(c); !reflect.DeepEqual(before, after) {
				t.Fatal("prior ERROR failure changed storage")
			}
			assertTransactionJournalClean(t, c)
		})
	}
}

func TestRecoverToAncestorOpenRegionFoldsExtrasAndCost(t *testing.T) {
	c, candidate, children := openRegionAncestorFixture(t)
	calls := 0
	err := c.ApplySchedulerAtomic(func(owner SchedulerTransactionToken) error {
		out, err := c.RecoverToAncestorStateWithOpenRegionAndCostOwned(owner, candidate, 2, 6, children[2:], func(prev NodeID, id SubtreeID) (uint32, error) {
			calls++
			r, err := c.subtree(id)
			if err != nil {
				return 0, err
			}
			if r.symbol != ErrorRegionSymbol || !r.extra || r.startByte != 0 || r.endByte != 6 {
				t.Fatalf("wrong region: %+v", r)
			}
			got := c.children[r.firstChild : r.firstChild+r.childCount]
			if !reflect.DeepEqual(got, children) {
				t.Fatalf("children=%v want=%v", got, children)
			}
			return 123, nil
		})
		if err != nil {
			return err
		}
		if got, err := c.RecoveryStoredErrorCost(out); err != nil || got != 123 {
			t.Fatalf("cost=%d err=%v", got, err)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if calls != 1 {
		t.Fatalf("cost calls=%d, want one final region", calls)
	}
}

func TestRecoverToAncestorOpenRegionFailurePoisonsAndRollsBack(t *testing.T) {
	for _, name := range []string{"empty", "span", "invalid-id", "overlap", "foreign", "stale", "cost", "panic", "outer", "limit", "nil-cost"} {
		t.Run(name, func(t *testing.T) {
			c, candidate, children := openRegionAncestorFixture(t)
			region := children[2:]
			start, end := uint32(2), uint32(6)
			cost := ReductionOutputCostFunc(func(NodeID, SubtreeID) (uint32, error) { return 1, nil })
			sentinel := errors.New("reject region")
			switch name {
			case "empty":
				region = nil
			case "span":
				end = 1
			case "invalid-id":
				region = []SubtreeID{SubtreeID(len(c.subtrees) + 1)}
			case "overlap":
				region = []SubtreeID{children[3], children[2]}
			case "foreign":
				candidate.owner = &Core{}
			case "stale":
				candidate.generation++
			case "cost":
				cost = func(NodeID, SubtreeID) (uint32, error) { return 0, sentinel }
			case "panic":
				cost = func(NodeID, SubtreeID) (uint32, error) { panic(sentinel) }
			case "limit":
				c.limits.MaxChildren = 3
			case "nil-cost":
				cost = nil
			}
			before := captureSchedulerTransactionState(c)
			var gotErr error
			func() {
				defer func() {
					if v := recover(); v != nil && name != "panic" {
						panic(v)
					}
				}()
				gotErr = c.ApplySchedulerAtomic(func(owner SchedulerTransactionToken) error {
					_, _ = c.RecoverToAncestorStateWithOpenRegionAndCostOwned(owner, candidate, start, end, region, cost)
					if name == "outer" {
						return sentinel
					}
					return nil
				})
			}()
			if name != "panic" && gotErr == nil {
				t.Fatal("ignored failure committed")
			}
			if after := captureSchedulerTransactionState(c); !reflect.DeepEqual(before, after) {
				t.Fatalf("failed region changed storage: before=%+v after=%+v", before, after)
			}
			assertTransactionJournalClean(t, c)
		})
	}
}
