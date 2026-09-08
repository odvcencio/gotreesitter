//go:build !gts_no_parsercorephase0

package gotreesitter

import (
	"errors"
	"reflect"
	"testing"

	core "github.com/odvcencio/gotreesitter/internal/parsercorephase0"
)

func TestRecoveryMarkerPreservesHistoriesWithoutAbsorbing(t *testing.T) {
	s := newRecoveryLineageForkScheduler(t, true)
	other, err := s.compact.Seed(3, 1)
	if err != nil {
		t.Fatal(err)
	}
	frontier := []diagnosticParserCoreHeader{s.headers[0], {head: other}}
	before := append([]diagnosticParserCoreHeader(nil), frontier...)
	var marker diagnosticParserCoreHeader
	staged := diagnosticParserCoreS5Work{}
	err = s.compact.ApplySchedulerAtomic(func(owner core.SchedulerTransactionToken) error {
		var err error
		marker, err = s.s5MergeRecoveryMarkerOwned(owner, frontier, &staged)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	state, position, err := s.compact.Boundary(marker.head)
	if err != nil || state != 0 || position != 1 {
		t.Fatalf("marker boundary=%d/%d err=%v", state, position, err)
	}
	if !reflect.DeepEqual(frontier, before) || marker.recoveryRegion() != nil || marker.shifted || s.s3RegionOpened {
		t.Fatal("marker construction changed its inputs or absorbed a token")
	}
	candidates, err := s.compact.StackSummaryCandidates(marker.head, 1)
	if err != nil {
		t.Fatal(err)
	}
	var states []core.StateID
	for _, candidate := range candidates {
		states = append(states, candidate.State())
		if candidate.Depth() != 1 || candidate.ByteOffset() != 1 {
			t.Fatalf("marker history depth=%d byte=%d", candidate.Depth(), candidate.ByteOffset())
		}
	}
	if !reflect.DeepEqual(states, []core.StateID{1, 3}) || staged.recoveryDiscontinuityMerges != 1 {
		t.Fatalf("marker histories=%v merges=%d", states, staged.recoveryDiscontinuityMerges)
	}
}

func TestRecoveryAdvanceEOFRollsBackAfterFork(t *testing.T) {
	for _, panicFault := range []bool{false, true} {
		t.Run(map[bool]string{false: "error", true: "panic"}[panicFault], func(t *testing.T) {
			s := newRecoveryLineageForkScheduler(t, true)
			s.options.MaxDispatches = 10
			var absorber diagnosticParserCoreHeader
			err := s.compact.ApplySchedulerAtomic(func(owner core.SchedulerTransactionToken) error {
				var err error
				absorber, err = s.s5AppendAndMergeAbsorberOwned(owner, s.headers, 0, 0, &diagnosticParserCoreS5Work{})
				return err
			})
			if err != nil {
				t.Fatal(err)
			}
			s.headers[0] = absorber
			s.receipt = &DiagnosticParserCoreGenericScheduler{Tokens: 7}
			s.verifierHeaderPtr, s.verifierBound = &s.headers[0], 1
			candidates, err := s.compact.StackSummaryCandidates(absorber.head, 1)
			if err != nil || len(candidates) != 1 {
				t.Fatalf("summary=%v err=%v", candidates, err)
			}
			before, err := s.compact.Stats(absorber.head)
			if err != nil {
				t.Fatal(err)
			}
			beforeWork, schedulerWork, receipt := s.compact.Work(), s.work, s.receipt
			region := absorber.recoveryRegion()
			children := append([]core.SubtreeID(nil), region.children...)
			failure := errors.New("EOF publication fault")
			calls := 0
			cost := func(core.NodeID, core.SubtreeID) (uint32, error) {
				calls++
				if calls == 1 {
					return 0, nil
				}
				if len(s.headers) != 2 {
					t.Fatalf("fault preceded recovery fork: headers=%d", len(s.headers))
				}
				if panicFault {
					panic(failure)
				}
				return 0, failure
			}
			var caught any
			func() {
				defer func() { caught = recover() }()
				err = s.advanceRecoveryEOF(0, absorber, candidates[0], true, cost)
			}()
			if calls < 2 || (panicFault && caught != failure) || (!panicFault && !errors.Is(err, failure)) {
				t.Fatalf("fault calls=%d panic=%v err=%v", calls, caught, err)
			}
			requireS5ForkRollback(t, s, absorber, before, beforeWork, receipt)
			if s.work != schedulerWork || s.headers[0].recoveryRegion() != region || !reflect.DeepEqual(region.children, children) {
				t.Fatal("rollback changed recovery work or region ownership")
			}
		})
	}
}
