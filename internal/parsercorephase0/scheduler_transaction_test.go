package parsercorephase0

import (
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"
)

type schedulerTransactionState struct {
	nodes, links, subtrees, children, fields, aliases int
	boundaries                                        map[boundaryIdentity]NodeID
	work                                              Work
}

func captureSchedulerTransactionState(c *Core) schedulerTransactionState {
	return schedulerTransactionState{
		nodes: len(c.nodes), links: len(c.links), subtrees: len(c.subtrees),
		children: len(c.children), fields: len(c.fields), aliases: len(c.aliases),
		boundaries: cloneBoundaryMap(c.boundaries), work: c.Work(),
	}
}

func requireClearedSchedulerFrame(t *testing.T, c *Core, epoch uint64) {
	t.Helper()
	if c.schedulerFrame.active || c.schedulerFrame.poisoned != nil || c.schedulerFrame.epoch != epoch ||
		!reflect.DeepEqual(c.schedulerFrame.mark, checkpoint{}) {
		t.Fatalf("scheduler frame retained completed state: %+v", c.schedulerFrame)
	}
}

func newSchedulerTransactionShiftFixture(t *testing.T) (*Core, Head, ClassifiedBoundary) {
	t.Helper()
	tables := &fakeTable{actions: map[tableCell][]Action{
		{state: 1, symbol: 9}: {{Type: ActionShift, State: 2}},
	}}
	compact, err := New(tables, Limits{})
	if err != nil {
		t.Fatal(err)
	}
	seed, err := compact.Seed(1, 0)
	if err != nil {
		t.Fatal(err)
	}
	boundary, err := compact.ClassifyBoundary(seed, 9)
	if err != nil {
		t.Fatal(err)
	}
	return compact, seed, boundary
}

func newSchedulerTransactionExtraShiftFixture(t *testing.T) (*Core, Head, ClassifiedBoundary) {
	t.Helper()
	tables := &fakeTable{actions: map[tableCell][]Action{
		{state: 1, symbol: 9}: {{Type: ActionShift, Extra: true}},
	}}
	compact, err := New(tables, Limits{})
	if err != nil {
		t.Fatal(err)
	}
	seed, err := compact.Seed(1, 0)
	if err != nil {
		t.Fatal(err)
	}
	boundary, err := compact.ClassifyBoundary(seed, 9)
	if err != nil {
		t.Fatal(err)
	}
	return compact, seed, boundary
}

func TestStandaloneCohortValidationPreservesClassifiedBoundary(t *testing.T) {
	for _, test := range []struct {
		name  string
		setup func(*testing.T) (*Core, Head, ClassifiedBoundary)
		token Token
		shift func(*Core, []ClassifiedBoundary, Token) ([]Head, error)
	}{
		{
			name: "ordinary", setup: newSchedulerTransactionShiftFixture,
			token: Token{Symbol: 9, EndByte: 1},
			shift: (*Core).ShiftOrdinaryClassifiedCohort,
		},
		{
			name: "extra", setup: newSchedulerTransactionExtraShiftFixture,
			token: Token{Symbol: 9, EndByte: 1, Extra: true},
			shift: (*Core).ShiftExtraClassifiedCohort,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			compact, _, boundary := test.setup(t)
			before := captureSchedulerTransactionState(compact)
			phase := compact.classificationPhase
			if _, err := test.shift(compact, []ClassifiedBoundary{boundary, boundary}, test.token); err == nil || !strings.Contains(err.Error(), "duplicate") {
				t.Fatalf("invalid standalone cohort error=%v", err)
			}
			if compact.classificationPhase != phase || !reflect.DeepEqual(captureSchedulerTransactionState(compact), before) || len(compact.transactions) != 0 {
				t.Fatalf("invalid standalone cohort changed state: phase=%d want=%d", compact.classificationPhase, phase)
			}
			if _, err := compact.ShiftClassified(boundary, 0, test.token, ForkOrder{}); err != nil {
				t.Fatalf("previously valid classification became stale: %v", err)
			}
		})
	}
}

func TestOwnedCohortValidationPoisonsOwner(t *testing.T) {
	for _, test := range []struct {
		name  string
		setup func(*testing.T) (*Core, Head, ClassifiedBoundary)
		token Token
		shift func(*Core, SchedulerTransactionToken, []ClassifiedBoundary, Token) ([]Head, error)
	}{
		{
			name: "ordinary", setup: newSchedulerTransactionShiftFixture,
			token: Token{Symbol: 9, EndByte: 1},
			shift: (*Core).ShiftOrdinaryClassifiedCohortOwned,
		},
		{
			name: "extra", setup: newSchedulerTransactionExtraShiftFixture,
			token: Token{Symbol: 9, EndByte: 1, Extra: true},
			shift: (*Core).ShiftExtraClassifiedCohortOwned,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			compact, _, boundary := test.setup(t)
			before := captureSchedulerTransactionState(compact)
			err := compact.ApplySchedulerAtomic(func(owner SchedulerTransactionToken) error {
				if _, innerErr := test.shift(compact, owner, []ClassifiedBoundary{boundary, boundary}, test.token); innerErr == nil || !strings.Contains(innerErr.Error(), "duplicate") {
					t.Fatalf("invalid owned cohort error=%v", innerErr)
				}
				return nil // Deliberately ignore the validation failure.
			})
			if err == nil || !strings.Contains(err.Error(), "poisoned scheduler transaction") || !reflect.DeepEqual(captureSchedulerTransactionState(compact), before) {
				t.Fatalf("owned validation did not poison and roll back: %v", err)
			}
		})
	}
}

func TestSchedulerTransactionCommitAndRollback(t *testing.T) {
	t.Run("commit", func(t *testing.T) {
		compact, _, boundary := newSchedulerTransactionShiftFixture(t)
		err := compact.ApplySchedulerAtomic(func(owner SchedulerTransactionToken) error {
			_, err := compact.ShiftClassifiedOwned(owner, boundary, 0, Token{Symbol: 9, EndByte: 1}, ForkOrder{})
			return err
		})
		if err != nil || compact.Work().Shifts != 1 || len(compact.transactions) != 0 {
			t.Fatalf("scheduler commit err=%v work=%+v frame=%+v transactions=%v", err, compact.Work(), compact.schedulerFrame, compact.transactions)
		}
		requireClearedSchedulerFrame(t, compact, 1)
	})

	t.Run("returned-error", func(t *testing.T) {
		compact, _, boundary := newSchedulerTransactionShiftFixture(t)
		before := captureSchedulerTransactionState(compact)
		sentinel := errors.New("scheduler rollback")
		err := compact.ApplySchedulerAtomic(func(owner SchedulerTransactionToken) error {
			if _, err := compact.ShiftClassifiedOwned(owner, boundary, 0, Token{Symbol: 9, EndByte: 1}, ForkOrder{}); err != nil {
				return err
			}
			return sentinel
		})
		after := captureSchedulerTransactionState(compact)
		if !errors.Is(err, sentinel) || !reflect.DeepEqual(after, before) || len(compact.transactions) != 0 {
			t.Fatalf("scheduler rollback err=%v before=%+v after=%+v frame=%+v transactions=%v", err, before, after, compact.schedulerFrame, compact.transactions)
		}
		requireClearedSchedulerFrame(t, compact, 1)
	})
}

func TestFreshSchedulerSessionCommitAndResetContract(t *testing.T) {
	compact, _, boundary := newSchedulerTransactionShiftFixture(t)
	var retained SchedulerTransactionToken
	err := compact.RunFreshSchedulerSession(func(owner SchedulerTransactionToken) error {
		retained = owner
		if len(compact.transactions) != 0 {
			t.Fatalf("fresh session opened rollback transaction: %v", compact.transactions)
		}
		_, err := compact.ShiftClassifiedOwned(owner, boundary, 0, Token{Symbol: 9, EndByte: 1}, ForkOrder{})
		return err
	})
	if err != nil || compact.Work().Shifts != 1 || len(compact.transactions) != 0 {
		t.Fatalf("fresh session commit err=%v work=%+v transactions=%v", err, compact.Work(), compact.transactions)
	}
	requireClearedSchedulerFrame(t, compact, retained.epoch)
	if err := compact.Reset(); err != nil {
		t.Fatalf("Reset after fresh session: %v", err)
	}
	requireClearedSchedulerFrame(t, compact, retained.epoch)
}

func TestFreshSchedulerSessionFailClosed(t *testing.T) {
	t.Run("ignored-owned-error-poisons", func(t *testing.T) {
		compact, _, boundary := newSchedulerTransactionShiftFixture(t)
		err := compact.RunFreshSchedulerSession(func(owner SchedulerTransactionToken) error {
			if _, innerErr := compact.ShiftClassifiedOwned(owner, boundary, 1, Token{Symbol: 9, EndByte: 1}, ForkOrder{}); innerErr == nil {
				t.Fatal("invalid owned shift unexpectedly succeeded")
			}
			return nil
		})
		if err == nil || !strings.Contains(err.Error(), "poisoned fresh scheduler session") {
			t.Fatalf("ignored owned error did not poison fresh session: %v", err)
		}
		requireClearedSchedulerFrame(t, compact, 1)
		if state := captureSchedulerTransactionState(compact); state.nodes != 0 || state.links != 0 || state.subtrees != 0 || state.work != (Work{}) {
			t.Fatalf("failed fresh session retained published state: %+v", state)
		}
	})

	t.Run("nested-session-poisons", func(t *testing.T) {
		compact, _, _ := newSchedulerTransactionShiftFixture(t)
		err := compact.RunFreshSchedulerSession(func(SchedulerTransactionToken) error {
			if nestedErr := compact.RunFreshSchedulerSession(func(SchedulerTransactionToken) error { return nil }); nestedErr == nil {
				t.Fatal("nested fresh session unexpectedly succeeded")
			}
			return nil
		})
		if err == nil || !strings.Contains(err.Error(), "poisoned fresh scheduler session") {
			t.Fatalf("ignored nested error did not poison fresh session: %v", err)
		}
		requireClearedSchedulerFrame(t, compact, 1)
		if state := captureSchedulerTransactionState(compact); state.nodes != 0 || state.links != 0 || state.subtrees != 0 || state.work != (Work{}) {
			t.Fatalf("nested fresh session retained published state: %+v", state)
		}
	})

	t.Run("nested-nil-session-poisons", func(t *testing.T) {
		compact, _, _ := newSchedulerTransactionShiftFixture(t)
		err := compact.RunFreshSchedulerSession(func(SchedulerTransactionToken) error {
			if nestedErr := compact.RunFreshSchedulerSession(nil); nestedErr == nil {
				t.Fatal("nested nil fresh session unexpectedly succeeded")
			}
			return nil
		})
		if err == nil || !strings.Contains(err.Error(), "poisoned fresh scheduler session") {
			t.Fatalf("ignored nested nil error did not poison fresh session: %v", err)
		}
		requireClearedSchedulerFrame(t, compact, 1)
		if state := captureSchedulerTransactionState(compact); state.nodes != 0 || state.links != 0 || state.subtrees != 0 || state.work != (Work{}) {
			t.Fatalf("nested nil fresh session retained published state: %+v", state)
		}
	})

	t.Run("panic-clears-capability", func(t *testing.T) {
		compact, _, boundary := newSchedulerTransactionShiftFixture(t)
		func() {
			defer func() {
				if recovered := recover(); recovered != "fresh panic" {
					t.Fatalf("recovered=%v", recovered)
				}
			}()
			_ = compact.RunFreshSchedulerSession(func(owner SchedulerTransactionToken) error {
				if _, err := compact.ShiftClassifiedOwned(owner, boundary, 0, Token{Symbol: 9, EndByte: 1}, ForkOrder{}); err != nil {
					return err
				}
				panic("fresh panic")
			})
		}()
		requireClearedSchedulerFrame(t, compact, 1)
		if state := captureSchedulerTransactionState(compact); state.nodes != 0 || state.links != 0 || state.subtrees != 0 || state.work != (Work{}) {
			t.Fatalf("panicked fresh session retained published state: %+v", state)
		}
	})
}

func TestFreshSchedulerSessionRejectsRetainedToken(t *testing.T) {
	compact, _, _ := newSchedulerTransactionShiftFixture(t)
	var stale SchedulerTransactionToken
	if err := compact.RunFreshSchedulerSession(func(owner SchedulerTransactionToken) error {
		stale = owner
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if err := compact.Reset(); err != nil {
		t.Fatal(err)
	}
	seed, err := compact.Seed(1, 0)
	if err != nil {
		t.Fatal(err)
	}
	boundary, err := compact.ClassifyBoundary(seed, 9)
	if err != nil {
		t.Fatal(err)
	}
	before := captureSchedulerTransactionState(compact)
	err = compact.ApplySchedulerAtomic(func(SchedulerTransactionToken) error {
		if _, innerErr := compact.ShiftClassifiedOwned(stale, boundary, 0, Token{Symbol: 9, EndByte: 1}, ForkOrder{}); innerErr == nil || !strings.Contains(innerErr.Error(), "stale") {
			t.Fatalf("retained fresh token error=%v", innerErr)
		}
		return nil
	})
	if err == nil || !strings.Contains(err.Error(), "poisoned scheduler transaction") || !reflect.DeepEqual(captureSchedulerTransactionState(compact), before) {
		t.Fatalf("retained fresh token did not fail closed: %v", err)
	}
}

func TestSchedulerSpeculationRollsBackFreshAndOrdinarySessions(t *testing.T) {
	t.Run("ordinary", func(t *testing.T) {
		compact, _, boundary := newSchedulerTransactionShiftFixture(t)
		before := captureSchedulerTransactionState(compact)
		err := compact.ApplySchedulerAtomic(func(outer SchedulerTransactionToken) error {
			if err := compact.ApplySchedulerSpeculation(outer, func(inner SchedulerTransactionToken) (bool, error) {
				if _, err := compact.ShiftClassifiedOwned(inner, boundary, 0, Token{Symbol: 9, EndByte: 1}, ForkOrder{}); err != nil {
					return false, err
				}
				return false, nil
			}); err != nil {
				return err
			}
			if got := captureSchedulerTransactionState(compact); !reflect.DeepEqual(got, before) {
				t.Fatalf("ordinary declined speculation changed state: got=%+v want=%+v", got, before)
			}
			if _, err := compact.ShiftClassified(boundary, 0, Token{Symbol: 9, EndByte: 1}, ForkOrder{}); err != nil {
				return err
			}
			return nil
		})
		if err != nil {
			t.Fatalf("ordinary speculation err=%v", err)
		}
		if compact.Work().Shifts != 1 {
			t.Fatalf("ordinary outer shift after decline work=%+v", compact.Work())
		}
	})

	t.Run("fresh", func(t *testing.T) {
		compact, _, boundary := newSchedulerTransactionShiftFixture(t)
		err := compact.RunFreshSchedulerSession(func(outer SchedulerTransactionToken) error {
			if err := compact.ApplySchedulerSpeculation(outer, func(inner SchedulerTransactionToken) (bool, error) {
				if _, err := compact.ShiftClassifiedOwned(inner, boundary, 0, Token{Symbol: 9, EndByte: 1}, ForkOrder{}); err != nil {
					return false, err
				}
				return false, nil
			}); err != nil {
				return err
			}
			_, err := compact.ShiftClassified(boundary, 0, Token{Symbol: 9, EndByte: 1}, ForkOrder{})
			return err
		})
		if err != nil {
			t.Fatalf("fresh speculation err=%v", err)
		}
		if compact.Work().Shifts != 1 {
			t.Fatalf("fresh outer shift after decline work=%+v", compact.Work())
		}
	})
}

func TestSchedulerSpeculationAuthenticatesOrdinaryAncestorBelowAtomicMark(t *testing.T) {
	for _, commit := range []bool{false, true} {
		t.Run(fmt.Sprintf("commit_%t", commit), func(t *testing.T) {
			compact, _, boundary := newSchedulerTransactionShiftFixture(t)
			err := compact.ApplySchedulerAtomic(func(outer SchedulerTransactionToken) error {
				return compact.ApplyAtomic(func() error {
					return compact.ApplySchedulerSpeculation(outer, func(inner SchedulerTransactionToken) (bool, error) {
						if _, err := compact.ShiftClassifiedOwned(inner, boundary, 0, Token{Symbol: 9, EndByte: 1}, ForkOrder{}); err != nil {
							return false, err
						}
						return commit, nil
					})
				})
			})
			if err != nil {
				t.Fatalf("speculation below internal transaction failed: %v", err)
			}
			wantShifts := uint64(0)
			if commit {
				wantShifts = 1
			}
			if compact.Work().Shifts != wantShifts {
				t.Fatalf("shifts=%d, want %d", compact.Work().Shifts, wantShifts)
			}
		})
	}
}

func TestNestedSchedulerSpeculationPreservesOuterClassification(t *testing.T) {
	compact, _, boundary := newSchedulerTransactionShiftFixture(t)
	err := compact.ApplySchedulerAtomic(func(outer SchedulerTransactionToken) error {
		return compact.ApplySchedulerSpeculation(outer, func(first SchedulerTransactionToken) (bool, error) {
			if err := compact.ApplySchedulerSpeculation(first, func(second SchedulerTransactionToken) (bool, error) {
				if _, err := compact.ShiftClassifiedOwned(second, boundary, 0, Token{Symbol: 9, EndByte: 1}, ForkOrder{}); err != nil {
					return false, err
				}
				return false, nil
			}); err != nil {
				return false, err
			}
			if _, err := compact.ShiftClassifiedOwned(first, boundary, 0, Token{Symbol: 9, EndByte: 1}, ForkOrder{}); err != nil {
				return false, err
			}
			return false, nil
		})
	})
	if err != nil {
		t.Fatalf("nested speculation err=%v", err)
	}
}

func TestSchedulerSpeculationInvalidatesChildClassificationOnly(t *testing.T) {
	for _, fresh := range []bool{false, true} {
		t.Run(fmt.Sprintf("fresh_%t", fresh), func(t *testing.T) {
			compact, _, outerBoundary := newSchedulerTransactionShiftFixture(t)
			var childBoundary ClassifiedBoundary
			run := func(outer SchedulerTransactionToken) error {
				if err := compact.ApplySchedulerSpeculation(outer, func(inner SchedulerTransactionToken) (bool, error) {
					childHead, err := compact.ShiftClassifiedOwned(inner, outerBoundary, 0, Token{Symbol: 9, EndByte: 1}, ForkOrder{})
					if err != nil {
						return false, err
					}
					childBoundary, err = compact.ClassifyBoundary(childHead, 9)
					return false, err
				}); err != nil {
					return err
				}
				if err := compact.validateClassification(childBoundary); err == nil || !strings.Contains(err.Error(), "transaction is stale") {
					return fmt.Errorf("child classification after decline: %v", err)
				}
				_, err := compact.ShiftClassifiedOwned(outer, outerBoundary, 0, Token{Symbol: 9, EndByte: 1}, ForkOrder{})
				return err
			}
			var err error
			if fresh {
				err = compact.RunFreshSchedulerSession(run)
			} else {
				err = compact.ApplySchedulerAtomic(run)
			}
			if err != nil {
				t.Fatalf("speculation classification isolation: %v", err)
			}
		})
	}

	compact, _, outerBoundary := newSchedulerTransactionShiftFixture(t)
	var childBoundary ClassifiedBoundary
	err := compact.ApplySchedulerAtomic(func(outer SchedulerTransactionToken) error {
		if err := compact.ApplySchedulerSpeculation(outer, func(inner SchedulerTransactionToken) (bool, error) {
			childHead, err := compact.ShiftClassifiedOwned(inner, outerBoundary, 0, Token{Symbol: 9, EndByte: 1}, ForkOrder{})
			if err != nil {
				return false, err
			}
			childBoundary, err = compact.ClassifyBoundary(childHead, 9)
			return true, err
		}); err != nil {
			return err
		}
		if err := compact.validateClassification(childBoundary); err == nil || !strings.Contains(err.Error(), "transaction is stale") {
			return fmt.Errorf("child classification after commit: %v", err)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("committed speculation classification isolation: %v", err)
	}
}

func TestSchedulerSpeculationIgnoredErrorPoisonsOuter(t *testing.T) {
	compact, _, boundary := newSchedulerTransactionShiftFixture(t)
	before := captureSchedulerTransactionState(compact)
	err := compact.ApplySchedulerAtomic(func(outer SchedulerTransactionToken) error {
		return compact.ApplySchedulerSpeculation(outer, func(inner SchedulerTransactionToken) (bool, error) {
			_, _ = compact.ShiftClassifiedOwned(inner, boundary, 1, Token{Symbol: 9, EndByte: 1}, ForkOrder{})
			return false, nil
		})
	})
	if err == nil {
		t.Fatalf("ignored speculative error did not poison outer: %v", err)
	}
	if got := captureSchedulerTransactionState(compact); !reflect.DeepEqual(got, before) {
		t.Fatalf("poisoned speculation changed state: got=%+v want=%+v", got, before)
	}
}

func TestSchedulerSpeculationRejectsOuterTokenDuringChild(t *testing.T) {
	compact, _, boundary := newSchedulerTransactionShiftFixture(t)
	err := compact.ApplySchedulerAtomic(func(outer SchedulerTransactionToken) error {
		return compact.ApplySchedulerSpeculation(outer, func(SchedulerTransactionToken) (bool, error) {
			_, err := compact.ShiftClassifiedOwned(outer, boundary, 0, Token{Symbol: 9, EndByte: 1}, ForkOrder{})
			if err == nil || !strings.Contains(err.Error(), "stale") {
				return false, errors.New("outer token was accepted during child speculation")
			}
			return false, nil
		})
	})
	if err == nil {
		t.Fatalf("outer-token misuse did not poison session: %v", err)
	}
}

func TestSchedulerTransactionInsideStandaloneOwner(t *testing.T) {
	t.Run("outer-rollback", func(t *testing.T) {
		compact, _, boundary := newSchedulerTransactionShiftFixture(t)
		before := captureSchedulerTransactionState(compact)
		sentinel := errors.New("outer rollback")
		err := compact.ApplyAtomic(func() error {
			if err := compact.ApplySchedulerAtomic(func(owner SchedulerTransactionToken) error {
				_, err := compact.ShiftClassifiedOwned(owner, boundary, 0, Token{Symbol: 9, EndByte: 1}, ForkOrder{})
				return err
			}); err != nil {
				return err
			}
			return sentinel
		})
		if !errors.Is(err, sentinel) || !reflect.DeepEqual(captureSchedulerTransactionState(compact), before) || len(compact.transactions) != 0 {
			t.Fatalf("standalone outer rollback err=%v", err)
		}
		requireClearedSchedulerFrame(t, compact, 1)
	})

	t.Run("outer-panic", func(t *testing.T) {
		compact, _, boundary := newSchedulerTransactionShiftFixture(t)
		before := captureSchedulerTransactionState(compact)
		func() {
			defer func() {
				if recovered := recover(); recovered != "outer panic" {
					t.Fatalf("recovered=%v", recovered)
				}
			}()
			_ = compact.ApplyAtomic(func() error {
				if err := compact.ApplySchedulerAtomic(func(owner SchedulerTransactionToken) error {
					_, err := compact.ShiftClassifiedOwned(owner, boundary, 0, Token{Symbol: 9, EndByte: 1}, ForkOrder{})
					return err
				}); err != nil {
					return err
				}
				panic("outer panic")
			})
		}()
		if !reflect.DeepEqual(captureSchedulerTransactionState(compact), before) || len(compact.transactions) != 0 {
			t.Fatal("standalone outer panic did not restore pre-owner state")
		}
		requireClearedSchedulerFrame(t, compact, 1)
	})
}

func TestSchedulerTransactionResetPreservesEpochAndRejectsRetainedToken(t *testing.T) {
	compact, _, _ := newSchedulerTransactionShiftFixture(t)
	var stale SchedulerTransactionToken
	if err := compact.ApplySchedulerAtomic(func(owner SchedulerTransactionToken) error {
		stale = owner
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	requireClearedSchedulerFrame(t, compact, stale.epoch)
	if err := compact.Reset(); err != nil {
		t.Fatal(err)
	}
	requireClearedSchedulerFrame(t, compact, stale.epoch)
	seed, err := compact.Seed(1, 0)
	if err != nil {
		t.Fatal(err)
	}
	boundary, err := compact.ClassifyBoundary(seed, 9)
	if err != nil {
		t.Fatal(err)
	}
	before := captureSchedulerTransactionState(compact)
	err = compact.ApplySchedulerAtomic(func(current SchedulerTransactionToken) error {
		if current.epoch == stale.epoch {
			t.Fatalf("Reset reused scheduler epoch %d", current.epoch)
		}
		if _, innerErr := compact.ShiftClassifiedOwned(stale, boundary, 0, Token{Symbol: 9, EndByte: 1}, ForkOrder{}); innerErr == nil || !strings.Contains(innerErr.Error(), "stale") {
			t.Fatalf("retained token error=%v", innerErr)
		}
		return nil // The stale-token error must poison the current owner.
	})
	if err == nil || !strings.Contains(err.Error(), "poisoned scheduler transaction") || !reflect.DeepEqual(captureSchedulerTransactionState(compact), before) {
		t.Fatalf("retained token after Reset err=%v", err)
	}
	requireClearedSchedulerFrame(t, compact, stale.epoch+1)
}

func TestSchedulerTransactionBoundaryIndexRehash(t *testing.T) {
	fill := func(t *testing.T, compact *Core, start, end int) {
		t.Helper()
		for index := start; index < end; index++ {
			if _, err := compact.Seed(StateID(index+1), uint32(index)); err != nil {
				t.Fatal(err)
			}
		}
	}
	for _, test := range []struct {
		name     string
		rollback bool
	}{
		{name: "commit"},
		{name: "rollback", rollback: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			compact, err := New(&fakeTable{}, Limits{MaxNodes: 64})
			if err != nil {
				t.Fatal(err)
			}
			fill(t, compact, 0, 11)
			if len(compact.boundaries.slots) != 16 {
				t.Fatalf("pre-owner slots=%d want=16", len(compact.boundaries.slots))
			}
			oldSlots := compact.boundaries.slots
			before := captureSchedulerTransactionState(compact)
			sentinel := errors.New("rehash rollback")
			err = compact.ApplySchedulerAtomic(func(owner SchedulerTransactionToken) error {
				if innerErr := compact.RunSchedulerOwned(owner, func() error {
					_, appendErr := compact.Seed(12, 11)
					return appendErr
				}); innerErr != nil {
					return innerErr
				}
				if len(compact.boundaries.slots) != 32 || &compact.boundaries.slots[0] == &oldSlots[0] {
					t.Fatalf("scheduler rehash slots=%d", len(compact.boundaries.slots))
				}
				if test.rollback {
					return sentinel
				}
				return nil
			})
			if test.rollback {
				if !errors.Is(err, sentinel) || !reflect.DeepEqual(captureSchedulerTransactionState(compact), before) ||
					len(compact.boundaries.slots) != 16 || &compact.boundaries.slots[0] != &oldSlots[0] {
					t.Fatalf("rehash rollback err=%v slots=%d", err, len(compact.boundaries.slots))
				}
			} else if err != nil || len(compact.boundaries.slots) != 32 || compact.boundaries.count != 12 {
				t.Fatalf("rehash commit err=%v slots=%d count=%d", err, len(compact.boundaries.slots), compact.boundaries.count)
			}
			requireClearedSchedulerFrame(t, compact, 1)
			if compact.schedulerFrame.mark.boundaryIndex.slots != nil {
				t.Fatal("completed scheduler frame retained old boundary-index snapshot")
			}
		})
	}
}

func TestSchedulerTransactionIgnoredInnerErrorPoisonsOwner(t *testing.T) {
	compact, _, boundary := newSchedulerTransactionShiftFixture(t)
	before := captureSchedulerTransactionState(compact)
	err := compact.ApplySchedulerAtomic(func(owner SchedulerTransactionToken) error {
		if _, innerErr := compact.ShiftClassifiedOwned(owner, boundary, 0, Token{Symbol: 9, EndByte: 1}, ForkOrder{}); innerErr != nil {
			return innerErr
		}
		_, innerErr := compact.ShiftClassifiedOwned(owner, boundary, 7, Token{Symbol: 9, EndByte: 2}, ForkOrder{})
		if innerErr == nil {
			t.Fatal("invalid owned shift did not fail")
		}
		return nil // Deliberately ignore the inner failure.
	})
	after := captureSchedulerTransactionState(compact)
	if err == nil || !strings.Contains(err.Error(), "poisoned scheduler transaction") || !reflect.DeepEqual(after, before) {
		t.Fatalf("ignored error err=%v before=%+v after=%+v", err, before, after)
	}
}

func TestSchedulerTransactionIgnoredNestedOwnerErrorPoisonsOwner(t *testing.T) {
	compact, _, boundary := newSchedulerTransactionShiftFixture(t)
	before := captureSchedulerTransactionState(compact)
	err := compact.ApplySchedulerAtomic(func(owner SchedulerTransactionToken) error {
		if _, innerErr := compact.ShiftClassifiedOwned(owner, boundary, 0, Token{Symbol: 9, EndByte: 1}, ForkOrder{}); innerErr != nil {
			return innerErr
		}
		if nestedErr := compact.ApplySchedulerAtomic(func(SchedulerTransactionToken) error { return nil }); nestedErr == nil {
			t.Fatal("nested scheduler owner did not fail")
		}
		return nil
	})
	if err == nil || !strings.Contains(err.Error(), "poisoned scheduler transaction") || !reflect.DeepEqual(captureSchedulerTransactionState(compact), before) {
		t.Fatalf("ignored nested owner error err=%v", err)
	}
}

func TestSchedulerTransactionRecoveredOwnedPanicStillPoisonsOwner(t *testing.T) {
	compact, _, boundary := newSchedulerTransactionShiftFixture(t)
	before := captureSchedulerTransactionState(compact)
	err := compact.ApplySchedulerAtomic(func(owner SchedulerTransactionToken) error {
		func() {
			defer func() {
				if recover() == nil {
					t.Fatal("owned panic did not repanic")
				}
			}()
			_ = compact.RunSchedulerOwned(owner, func() error {
				if _, innerErr := compact.ShiftClassifiedOwned(owner, boundary, 0, Token{Symbol: 9, EndByte: 1}, ForkOrder{}); innerErr != nil {
					return innerErr
				}
				panic("recovered owned panic")
			})
		}()
		return nil
	})
	if err == nil || !strings.Contains(err.Error(), "poisoned scheduler transaction") || !reflect.DeepEqual(captureSchedulerTransactionState(compact), before) {
		t.Fatalf("recovered owned panic err=%v", err)
	}
}

func TestSchedulerTransactionTokenValidationPoisonsCurrentCore(t *testing.T) {
	t.Run("wrong-core", func(t *testing.T) {
		compact, _, _ := newSchedulerTransactionShiftFixture(t)
		other, _, otherBoundary := newSchedulerTransactionShiftFixture(t)
		before := captureSchedulerTransactionState(compact)
		otherBefore := captureSchedulerTransactionState(other)
		err := compact.ApplySchedulerAtomic(func(owner SchedulerTransactionToken) error {
			otherErr := other.ApplySchedulerAtomic(func(current SchedulerTransactionToken) error {
				if _, innerErr := other.ShiftClassifiedOwned(current, otherBoundary, 0, Token{Symbol: 9, EndByte: 1}, ForkOrder{}); innerErr != nil {
					return innerErr
				}
				_, innerErr := other.ShiftClassifiedOwned(owner, otherBoundary, 0, Token{Symbol: 9, EndByte: 1}, ForkOrder{})
				if innerErr == nil || !strings.Contains(innerErr.Error(), "different core") {
					t.Fatalf("wrong-core validation error=%v", innerErr)
				}
				if compact.schedulerFrame.poisoned != nil {
					t.Fatalf("wrong-core call mutated token owner: %v", compact.schedulerFrame.poisoned)
				}
				return nil
			})
			if otherErr == nil || !strings.Contains(otherErr.Error(), "poisoned scheduler transaction") {
				t.Fatalf("called core was not poisoned: %v", otherErr)
			}
			return nil
		})
		if err != nil || !reflect.DeepEqual(captureSchedulerTransactionState(compact), before) {
			t.Fatalf("token owner changed: err=%v before=%+v after=%+v", err, before, captureSchedulerTransactionState(compact))
		}
		if !reflect.DeepEqual(captureSchedulerTransactionState(other), otherBefore) {
			t.Fatalf("called core did not roll back: before=%+v after=%+v", otherBefore, captureSchedulerTransactionState(other))
		}
	})

	t.Run("not-top-of-stack", func(t *testing.T) {
		compact, _, boundary := newSchedulerTransactionShiftFixture(t)
		before := captureSchedulerTransactionState(compact)
		err := compact.ApplySchedulerAtomic(func(owner SchedulerTransactionToken) error {
			return compact.ApplyAtomic(func() error {
				_, innerErr := compact.ShiftClassifiedOwned(owner, boundary, 0, Token{Symbol: 9, EndByte: 1}, ForkOrder{})
				if innerErr == nil || !strings.Contains(innerErr.Error(), "top-of-stack") {
					t.Fatalf("top-of-stack validation error=%v", innerErr)
				}
				return nil
			})
		})
		if err == nil || !strings.Contains(err.Error(), "poisoned scheduler transaction") || !reflect.DeepEqual(captureSchedulerTransactionState(compact), before) {
			t.Fatalf("non-top owner was not poisoned: err=%v", err)
		}
	})

	t.Run("stale", func(t *testing.T) {
		compact, _, boundary := newSchedulerTransactionShiftFixture(t)
		var stale SchedulerTransactionToken
		if err := compact.ApplySchedulerAtomic(func(owner SchedulerTransactionToken) error {
			stale = owner
			return nil
		}); err != nil {
			t.Fatal(err)
		}
		if _, err := compact.ShiftClassifiedOwned(stale, boundary, 0, Token{Symbol: 9, EndByte: 1}, ForkOrder{}); err == nil || !strings.Contains(err.Error(), "stale") {
			t.Fatalf("stale token error=%v", err)
		}
	})
}

func TestSchedulerTransactionWrongCoreConcurrentIsolation(t *testing.T) {
	first, _, _ := newSchedulerTransactionShiftFixture(t)
	second, _, _ := newSchedulerTransactionShiftFixture(t)
	firstBefore := captureSchedulerTransactionState(first)
	secondBefore := captureSchedulerTransactionState(second)
	firstToken := make(chan SchedulerTransactionToken, 1)
	secondToken := make(chan SchedulerTransactionToken, 1)
	results := make(chan error, 2)

	go func() {
		results <- first.ApplySchedulerAtomic(func(current SchedulerTransactionToken) error {
			firstToken <- current
			foreign := <-secondToken
			if _, err := first.ShiftClassifiedOwned(foreign, ClassifiedBoundary{}, 0, Token{}, ForkOrder{}); err == nil || !strings.Contains(err.Error(), "different core") {
				return errors.New("first core accepted foreign scheduler token")
			}
			return nil
		})
	}()
	go func() {
		results <- second.ApplySchedulerAtomic(func(current SchedulerTransactionToken) error {
			secondToken <- current
			foreign := <-firstToken
			if _, err := second.ShiftClassifiedOwned(foreign, ClassifiedBoundary{}, 0, Token{}, ForkOrder{}); err == nil || !strings.Contains(err.Error(), "different core") {
				return errors.New("second core accepted foreign scheduler token")
			}
			return nil
		})
	}()

	for range 2 {
		if err := <-results; err == nil || !strings.Contains(err.Error(), "poisoned scheduler transaction") {
			t.Fatalf("called current core was not poisoned: %v", err)
		}
	}
	if !reflect.DeepEqual(captureSchedulerTransactionState(first), firstBefore) ||
		!reflect.DeepEqual(captureSchedulerTransactionState(second), secondBefore) {
		t.Fatalf("concurrent wrong-core misuse did not isolate rollback")
	}
}

func TestSchedulerTransactionPanicRollsBackAndRepanics(t *testing.T) {
	compact, _, boundary := newSchedulerTransactionShiftFixture(t)
	before := captureSchedulerTransactionState(compact)
	func() {
		defer func() {
			if recovered := recover(); recovered != "scheduler panic" {
				t.Fatalf("recovered=%v", recovered)
			}
		}()
		_ = compact.ApplySchedulerAtomic(func(owner SchedulerTransactionToken) error {
			if _, err := compact.ShiftClassifiedOwned(owner, boundary, 0, Token{Symbol: 9, EndByte: 1}, ForkOrder{}); err != nil {
				return err
			}
			panic("scheduler panic")
		})
	}()
	if after := captureSchedulerTransactionState(compact); !reflect.DeepEqual(after, before) || compact.schedulerFrame.active || len(compact.transactions) != 0 {
		t.Fatalf("panic rollback before=%+v after=%+v frame=%+v transactions=%v", before, after, compact.schedulerFrame, compact.transactions)
	}
}
