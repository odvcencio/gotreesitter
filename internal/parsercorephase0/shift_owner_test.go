package parsercorephase0

import (
	"errors"
	"reflect"
	"strings"
	"testing"
)

func TestShiftOwnerMatchesSeparateOperations(t *testing.T) {
	for _, fresh := range []bool{false, true} {
		t.Run(map[bool]string{false: "transaction", true: "fresh"}[fresh], func(t *testing.T) {
			before, seedBefore, _ := newSchedulerTransactionShiftFixture(t)
			after, seedAfter, _ := newSchedulerTransactionShiftFixture(t)
			run := func(c *Core, fn func(SchedulerTransactionToken) error) error {
				if fresh {
					return c.RunFreshSchedulerSession(fn)
				}
				return c.ApplySchedulerAtomic(fn)
			}
			var oldHead, newHead Head
			if err := run(before, func(owner SchedulerTransactionToken) error {
				var err error
				oldHead, err = before.ShiftDirectWithLiveCondenseCandidatesOwned(owner, nil, seedBefore, 9, 2, false, Token{Symbol: 9, EndByte: 1}, ForkOrder{})
				if err != nil {
					return err
				}
				return before.RecordHeadOwnerOwned(owner, oldHead, 7)
			}); err != nil {
				t.Fatal(err)
			}
			if err := run(after, func(owner SchedulerTransactionToken) error {
				var published bool
				var err error
				newHead, published, err = after.ShiftDirectAndPublishLineageOwned(owner, nil, seedAfter, 9, 2, false, Token{Symbol: 9, EndByte: 1}, ForkOrder{}, ShiftHeadLineage{Owner: 7})
				if err == nil && !published {
					t.Fatal("new head did not publish its owner")
				}
				return err
			}); err != nil {
				t.Fatal(err)
			}
			if oldHead != newHead || !reflect.DeepEqual(before.nodes.snapshotForTest(), after.nodes.snapshotForTest()) || !reflect.DeepEqual(before.links.Values(), after.links.Values()) || !reflect.DeepEqual(before.subtrees.snapshotForTest(), after.subtrees.snapshotForTest()) || !reflect.DeepEqual(before.nodeLineages, after.nodeLineages) || !reflect.DeepEqual(captureSchedulerTransactionState(before), captureSchedulerTransactionState(after)) {
				t.Fatalf("fused shift changed state or work: old=%+v new=%+v oldwork=%+v newwork=%+v", oldHead, newHead, before.Work(), after.Work())
			}
			if after.nodeLineages[newHead.Node-1].owner != 7 {
				t.Fatal("owner not retained")
			}
		})
	}
}

func TestShiftOwnerRollbackAndInvalidLineage(t *testing.T) {
	c, seed, _ := newSchedulerTransactionShiftFixture(t)
	before := captureSchedulerTransactionState(c)
	sentinel := errors.New("rollback fused owner")
	err := c.ApplySchedulerAtomic(func(owner SchedulerTransactionToken) error {
		head, published, err := c.ShiftDirectAndPublishLineageOwned(owner, nil, seed, 9, 2, false, Token{Symbol: 9, EndByte: 1}, ForkOrder{}, ShiftHeadLineage{Owner: 7})
		if err != nil {
			return err
		}
		if !published || c.nodeLineages[head.Node-1].owner != 7 {
			t.Fatal("owner absent before rollback")
		}
		return sentinel
	})
	if !errors.Is(err, sentinel) || !reflect.DeepEqual(before, captureSchedulerTransactionState(c)) {
		t.Fatalf("rollback err=%v state=%+v", err, captureSchedulerTransactionState(c))
	}
	err = c.ApplySchedulerAtomic(func(owner SchedulerTransactionToken) error {
		_, published, err := c.ShiftDirectAndPublishLineageOwned(owner, nil, seed, 9, 2, false, Token{Symbol: 9, EndByte: 1}, ForkOrder{}, ShiftHeadLineage{Owner: 0})
		if published {
			t.Fatal("zero owner published")
		}
		return err
	})
	if err == nil || !strings.Contains(err.Error(), "zero scheduler lineage") || !reflect.DeepEqual(before, captureSchedulerTransactionState(c)) {
		t.Fatalf("zero owner err=%v state=%+v", err, captureSchedulerTransactionState(c))
	}
}

func TestShiftOwnerPreservesExistingOwnerConflict(t *testing.T) {
	c, seed, _ := newSchedulerTransactionShiftFixture(t)
	var head Head
	err := c.RunFreshSchedulerSession(func(owner SchedulerTransactionToken) error {
		var published bool
		var err error
		head, published, err = c.ShiftDirectAndPublishLineageOwned(owner, nil, seed, 9, 2, false, Token{Symbol: 9, EndByte: 1}, ForkOrder{}, ShiftHeadLineage{Owner: 7})
		if err != nil {
			return err
		}
		if !published {
			t.Fatal("first head not fresh")
		}
		again, bound, err := c.ShiftDirectAndPublishLineageOwned(owner, []CondenseCandidate{{Head: head}}, seed, 9, 2, false, Token{Symbol: 9, EndByte: 1}, ForkOrder{}, ShiftHeadLineage{Owner: 11})
		if err != nil {
			return err
		}
		if again != head || bound || c.nodeLineages[head.Node-1].owner != 7 {
			t.Fatalf("existing owner rewritten: head=%+v got=%+v published=%t owner=%d", head, again, bound, c.nodeLineages[head.Node-1].owner)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	err = c.ApplySchedulerAtomic(func(owner SchedulerTransactionToken) error { return c.RecordHeadOwnerOwned(owner, head, 11) })
	if err == nil || !strings.Contains(err.Error(), "multiple scheduler owners") || c.nodeLineages[head.Node-1].owner != 7 {
		t.Fatalf("owner conflict=%v owner=%d", err, c.nodeLineages[head.Node-1].owner)
	}
}

func TestShiftOwnerRejectsForeignActiveToken(t *testing.T) {
	c, seed, _ := newSchedulerTransactionShiftFixture(t)
	control, controlSeed, _ := newSchedulerTransactionShiftFixture(t)
	other, _, _ := newSchedulerTransactionShiftFixture(t)
	otherBefore := captureSchedulerTransactionState(other)
	err := other.RunFreshSchedulerSession(func(foreign SchedulerTransactionToken) error {
		for _, test := range []struct {
			core  *Core
			seed  Head
			fused bool
		}{{control, controlSeed, false}, {c, seed, true}} {
			got := test.core.RunFreshSchedulerSession(func(owner SchedulerTransactionToken) error {
				var err error
				if test.fused {
					var published bool
					_, published, err = test.core.ShiftDirectAndPublishLineageOwned(foreign, nil, test.seed, 9, 2, false, Token{Symbol: 9, EndByte: 1}, ForkOrder{}, ShiftHeadLineage{Owner: 7})
					if published {
						t.Fatal("foreign token published an owner")
					}
				} else {
					_, err = test.core.ShiftDirectWithLiveCondenseCandidatesOwned(foreign, nil, test.seed, 9, 2, false, Token{Symbol: 9, EndByte: 1}, ForkOrder{})
				}
				if err == nil {
					t.Fatal("foreign token accepted")
				}
				return nil
			})
			if got == nil || !strings.Contains(got.Error(), "poisoned fresh scheduler session") || !strings.Contains(got.Error(), "different core") {
				t.Fatalf("foreign token did not poison receiver: %v", got)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	// A poisoned fresh session intentionally clears its receiver. Compare that
	// complete native state with the existing API, and preserve the other core.
	if !reflect.DeepEqual(captureSchedulerTransactionState(c), captureSchedulerTransactionState(control)) || !reflect.DeepEqual(c.nodes.snapshotForTest(), control.nodes.snapshotForTest()) || !reflect.DeepEqual(c.nodeLineages, control.nodeLineages) {
		t.Fatal("foreign-token cleanup differs from existing shift")
	}
	if !reflect.DeepEqual(otherBefore, captureSchedulerTransactionState(other)) {
		t.Fatal("foreign token mutated its owning core")
	}
}

func TestShiftOwnerScalarLineageMatchesSeparatePublication(t *testing.T) {
	cases := []ShiftHeadLineage{
		{Owner: 7, PublishLineage: true, Rank: CleanPathRankSelected, Lineage: 42},
		{Owner: 7, PublishLineage: true, Rank: CleanPathRankUnselected, Lineage: 42},
		{Owner: 7, PublishLineage: true, Rank: CleanPathRankNotApplicable, Lineage: 42},
		{Owner: 7, PublishLineage: true, Rank: CleanPathRankSelected, Lineage: 0},
		{Owner: 7, PublishLineage: true, Rank: CleanPathRankSelection(255), Lineage: 42},
		{Owner: 7, PublishLineage: true, Rank: CleanPathRankSelected, Lineage: 42, Set: AlternativeSet{inline: [alternativeSetInlineCapacity]uint32{1 << 16}, count: 1}},
		{Owner: 7, PublishLineage: true, Rank: CleanPathRankUnselected, Lineage: 42, PreviousHead: Head{Node: 2}},
	}
	for i, pub := range cases {
		for _, fresh := range []bool{false, true} {
			before, seedBefore, _ := newSchedulerTransactionShiftFixture(t)
			after, seedAfter, _ := newSchedulerTransactionShiftFixture(t)
			run := func(c *Core, fn func(SchedulerTransactionToken) error) error {
				if fresh {
					return c.RunFreshSchedulerSession(fn)
				}
				return c.ApplySchedulerAtomic(fn)
			}
			var oldHead, newHead Head
			oldErr := run(before, func(owner SchedulerTransactionToken) error {
				var err error
				oldHead, err = before.ShiftDirectWithLiveCondenseCandidatesOwned(owner, nil, seedBefore, 9, 2, false, Token{Symbol: 9, EndByte: 1}, ForkOrder{})
				if err != nil {
					return err
				}
				if err = before.RecordHeadOwnerOwned(owner, oldHead, pub.Owner); err != nil {
					return err
				}
				return before.RecordHeadLineageOwned(owner, oldHead, pub.Rank, pub.Lineage, pub.Set, pub.Blended, pub.SetDirty || oldHead != pub.PreviousHead, DropCohortRefSet{})
			})
			newErr := run(after, func(owner SchedulerTransactionToken) error {
				var published bool
				var err error
				newHead, published, err = after.ShiftDirectAndPublishLineageOwned(owner, nil, seedAfter, 9, 2, false, Token{Symbol: 9, EndByte: 1}, ForkOrder{}, pub)
				if err == nil && !published {
					t.Fatalf("case=%d fresh=%t no publication", i, fresh)
				}
				return err
			})
			if (oldErr == nil) != (newErr == nil) || oldHead != newHead || !reflect.DeepEqual(before.nodes.snapshotForTest(), after.nodes.snapshotForTest()) || !reflect.DeepEqual(before.links.Values(), after.links.Values()) || !reflect.DeepEqual(before.nodeLineages, after.nodeLineages) || !reflect.DeepEqual(before.nodeLineageRecovery, after.nodeLineageRecovery) || !reflect.DeepEqual(before.alternativeSpillArena, after.alternativeSpillArena) || !reflect.DeepEqual(captureSchedulerTransactionState(before), captureSchedulerTransactionState(after)) {
				t.Fatalf("case=%d fresh=%t olderr=%v newerr=%v oldwork=%+v newwork=%+v", i, fresh, oldErr, newErr, before.Work(), after.Work())
			}
		}
	}
}
