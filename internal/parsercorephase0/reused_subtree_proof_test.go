package parsercorephase0

import (
	"errors"
	"testing"
)

func TestReusedSubtreeProofVisitsScaleWithNewRecords(t *testing.T) {
	for _, reduced := range []bool{false, true} {
		for _, count := range []uint32{512, 1024} {
			c, head, reused := reusedFixture(t)
			tables := c.tables.(*fakeTable)
			tables.gotos[tableCell{state: 1, symbol: 100}] = 1
			polls := 0
			err := c.ApplySchedulerAtomic(func(owner SchedulerTransactionToken) error {
				for index := uint32(0); index < count; index++ {
					reused.Key, reused.State = index+1, 1
					reused.StartByte, reused.EndByte = index, index+1
					var err error
					head, _, err = c.PushReusedSubtreeOwnedWithPoll(owner, head, reused, func() error { polls++; return nil })
					if err != nil {
						return err
					}
					if reduced && index > 0 {
						// Build an authenticated binary list through the real reduction seam.
						tables.gotos[tableCell{state: 1, symbol: 101}] = 1
						tables.actions = map[tableCell][]Action{{state: 1, symbol: 0}: {{Type: ActionReduce, Symbol: 101, ChildCount: 2}}}
						boundary, err := c.ClassifyBoundary(head, 0)
						if err != nil {
							return err
						}
						outputs, err := c.ReduceOutputsClassifiedIntoOwned(owner, nil, boundary, 0, ForkOrder{})
						if err != nil {
							return err
						}
						if len(outputs) != 1 {
							t.Fatalf("list reduction produced %d outputs", len(outputs))
						}
						head = outputs[0].Head
					}
				}
				return nil
			})
			if err != nil {
				t.Fatalf("reduced=%t count=%d: %v", reduced, count, err)
			}
			// Each splice adds fewer than 128 validation units. Prefix rescans would add intermediate polls.
			if polls != 2*int(count) {
				t.Fatalf("reduced=%t count=%d polls=%d, want %d", reduced, count, polls, 2*count)
			}
			if uint64(c.reuseProof.nodes) >= uint64(len(c.nodes)) || c.reuseProof.subtrees == 0 {
				t.Fatal("proof cursors did not track publication")
			}
		}
	}
}

func TestReusedSubtreeProofInvalidatesPriorMutations(t *testing.T) {
	for _, mutation := range []string{"fragility", "lineage", "recovery cost", "lineage copy"} {
		t.Run(mutation, func(t *testing.T) {
			c, seed, reused := reusedFixture(t)
			c.tables.(*fakeTable).actions = map[tableCell][]Action{{state: 1, symbol: 1}: {{Type: ActionShift, State: 1}}}
			head, err := c.Shift(seed, 1, 0, Token{Symbol: 1, EndByte: 1}, ForkOrder{})
			if err != nil {
				t.Fatal(err)
			}
			err = c.ApplySchedulerAtomic(func(owner SchedulerTransactionToken) error {
				_, _, err := c.PushReusedSubtreeOwned(owner, head, reused)
				return err
			})
			if err != nil {
				t.Fatal(err)
			}
			switch mutation {
			case "fragility":
				c.markSubtreeFragile(1)
			case "lineage":
				err = c.recordNodeLineage(seed, CleanPathRankSelected, 1)
			case "recovery cost":
				err = c.publishInheritedStoredErrorCost(seed, 1)
			case "lineage copy":
				other, appendErr := c.Seed(1, 0)
				if appendErr != nil {
					t.Fatal(appendErr)
				}
				if err = c.publishInheritedStoredErrorCost(other, 1); err == nil {
					err = c.copyRecoveryDiscontinuityLineage(other.Node, seed.Node)
				}
			}
			if err != nil {
				t.Fatal(err)
			}
			if !c.reuseProof.invalid {
				t.Fatal("mutation did not invalidate a proven prefix")
			}
			reused.Key++
			err = c.ApplySchedulerAtomic(func(owner SchedulerTransactionToken) error {
				_, _, err := c.PushReusedSubtreeOwned(owner, head, reused)
				return err
			})
			if err == nil {
				t.Fatal("stale proof admitted reuse")
			}
			if err := c.Reset(); err != nil {
				t.Fatal(err)
			}
			if c.reuseProof != (reuseValidationProof{}) {
				t.Fatal("reset retained a proof")
			}
			seed, err = c.Seed(1, 0)
			if err != nil {
				t.Fatal(err)
			}
			reused.Key = 1
			err = c.ApplySchedulerAtomic(func(owner SchedulerTransactionToken) error {
				_, _, err := c.PushReusedSubtreeOwned(owner, seed, reused)
				return err
			})
			if err != nil {
				t.Fatalf("reset failed to admit a new clean generation: %v", err)
			}
		})
	}
}

func TestReusedSubtreeProofRollbackAndOrderedKeys(t *testing.T) {
	c, head, reused := reusedFixture(t)
	stop := errors.New("stop")
	err := c.ApplySchedulerAtomic(func(owner SchedulerTransactionToken) error {
		_, _, err := c.PushReusedSubtreeOwned(owner, head, reused)
		if err != nil {
			return err
		}
		return stop
	})
	if !errors.Is(err, stop) || c.reuseProof != (reuseValidationProof{}) {
		t.Fatalf("rollback kept unpublished proof: %+v %v", c.reuseProof, err)
	}
	err = c.ApplySchedulerAtomic(func(owner SchedulerTransactionToken) error {
		_, _, err := c.PushReusedSubtreeOwned(owner, head, reused)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	before := c.reuseProof
	for _, key := range []uint32{0, 1} {
		reused.Key = key
		err = c.ApplySchedulerAtomic(func(owner SchedulerTransactionToken) error {
			_, _, err := c.PushReusedSubtreeOwned(owner, head, reused)
			return err
		})
		if err == nil || c.reuseProof != before || len(c.reusedSubtrees) != 1 {
			t.Fatal("duplicate key committed")
		}
	}
	err = c.ApplySchedulerAtomic(func(owner SchedulerTransactionToken) error {
		if err := c.RecordHeadLineageOwned(owner, head, CleanPathRankSelected, 1, AlternativeSet{}, false, false); err != nil {
			return err
		}
		return stop
	})
	if !errors.Is(err, stop) || !c.reuseProof.invalid || c.nodeLineages[head.Node-1].converged {
		t.Fatal("rollback resurrected a proof or retained lineage mutation")
	}
}

func TestReusedSubtreeProofPollRollsBackPartialValidation(t *testing.T) {
	c, head, reused := reusedFixture(t)
	c.tables.(*fakeTable).actions = map[tableCell][]Action{{state: 1, symbol: 1}: {{Type: ActionShift, State: 1}}}
	for index := uint32(0); index < 300; index++ {
		var err error
		head, err = c.Shift(head, 1, 0, Token{Symbol: 1, StartByte: index, EndByte: index + 1}, ForkOrder{})
		if err != nil {
			t.Fatal(err)
		}
	}
	reused.StartByte, reused.EndByte = 300, 310
	stop := errors.New("cancel validation")
	polls := 0
	err := c.ApplySchedulerAtomic(func(owner SchedulerTransactionToken) error {
		_, _, err := c.PushReusedSubtreeOwnedWithPoll(owner, head, reused, func() error {
			polls++
			if polls == 2 {
				return stop
			}
			return nil
		})
		return err
	})
	if !errors.Is(err, stop) || polls != 2 || c.reuseProof != (reuseValidationProof{}) || len(c.reusedSubtrees) != 0 {
		t.Fatalf("partial validation was published: %+v %v", c.reuseProof, err)
	}
}
