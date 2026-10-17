package parsercorephase0

import (
	"errors"
	"fmt"
	"math"
	"strings"
	"testing"
)

func scannerElectionBindingState(c *Core) [6]uint64 {
	exact := uint64(0)
	if c.externalTokenScannerExact {
		exact = 1
	}
	return [6]uint64{c.frontier, c.classificationPhase, uint64(c.checkpoint),
		uint64(c.externalTokenScannerStart), uint64(c.externalTokenScannerEnd), exact}
}

func TestBindScannerElectionMatchesReceiptThenBinding(t *testing.T) {
	for _, start := range []CheckpointID{0, 1, 2, 3, math.MaxUint32} {
		for _, end := range []CheckpointID{0, 1, 2, 3, math.MaxUint32} {
			t.Run(fmt.Sprintf("%d/%d", start, end), func(t *testing.T) {
				old := newTinyCoreWithLimits(t, Limits{})
				got := newTinyCoreWithLimits(t, Limits{})
				for _, c := range []*Core{old, got} {
					mustInternCheckpoint(t, c, []byte{1})
					mustInternCheckpoint(t, c, []byte{2})
					if err := c.SetPhaseExternalTokenScannerCheckpoints(1, 1); err != nil {
						t.Fatal(err)
					}
				}
				legacyBind := func(c *Core) error {
					if _, _, ok := c.CheckpointReceipt(start); !ok {
						return ErrUnknownScannerCheckpoint
					}
					if _, _, ok := c.CheckpointReceipt(end); !ok {
						return ErrUnknownScannerCheckpoint
					}
					return c.SetPhaseExternalTokenScannerCheckpoints(start, end)
				}
				wantErr, gotErr := legacyBind(old), got.BindScannerElection(start, end)
				if fmt.Sprint(gotErr) != fmt.Sprint(wantErr) ||
					errors.Is(gotErr, ErrUnknownScannerCheckpoint) != errors.Is(wantErr, ErrUnknownScannerCheckpoint) {
					t.Fatalf("error=%v, want %v", gotErr, wantErr)
				}
				if scannerElectionBindingState(got) != scannerElectionBindingState(old) || got.Work() != old.Work() {
					t.Fatalf("state=%v, want %v", scannerElectionBindingState(got), scannerElectionBindingState(old))
				}
			})
		}
	}
}

func TestBindScannerElectionRejectsAtomically(t *testing.T) {
	c := newTinyCoreWithLimits(t, Limits{})
	id := mustInternCheckpoint(t, c, []byte{1})
	if err := c.BindScannerElection(0, id); err != nil {
		t.Fatal(err)
	}
	if err := c.ApplySchedulerAtomic(func(SchedulerTransactionToken) error {
		before := scannerElectionBindingState(c)
		if err := c.BindScannerElection(id, id); err == nil || !strings.Contains(err.Error(), "active transaction") {
			t.Fatalf("transaction error=%v", err)
		}
		if err := c.BindScannerElection(id+1, id); !errors.Is(err, ErrUnknownScannerCheckpoint) {
			t.Fatalf("invalid-ID error precedence=%v", err)
		}
		if scannerElectionBindingState(c) != before {
			t.Fatal("rejection changed scanner election")
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	c.classificationPhase = math.MaxUint64
	before := scannerElectionBindingState(c)
	if err := c.BindScannerElection(id, id); err == nil || !strings.Contains(err.Error(), "overflow") {
		t.Fatalf("overflow error=%v", err)
	}
	if scannerElectionBindingState(c) != before {
		t.Fatal("overflow changed scanner election")
	}
	if err := c.BindScannerElection(0, id); err != nil {
		t.Fatalf("unchanged election at maximum phase: %v", err)
	}
	c.classificationPhase = 1
	if err := c.Reset(); err != nil {
		t.Fatal(err)
	}
	if err := c.BindScannerElection(id, 0); !errors.Is(err, ErrUnknownScannerCheckpoint) {
		t.Fatalf("pre-reset ID error=%v", err)
	}
	if err := c.BindScannerElection(0, 0); err != nil {
		t.Fatalf("empty scanner election: %v", err)
	}
	var missing *Core
	if err := missing.BindScannerElection(0, 0); !errors.Is(err, ErrUnknownScannerCheckpoint) {
		t.Fatalf("nil-core error=%v", err)
	}
}

func TestBindScannerElectionStartChangeInvalidatesBoundary(t *testing.T) {
	c := newTinyCoreWithLimits(t, Limits{})
	first := mustInternCheckpoint(t, c, []byte{1})
	second := mustInternCheckpoint(t, c, []byte{2})
	end := mustInternCheckpoint(t, c, []byte{3})
	if err := c.BindScannerElection(first, end); err != nil {
		t.Fatal(err)
	}
	head, err := c.Seed(1, 0)
	if err != nil {
		t.Fatal(err)
	}
	boundary, err := c.ClassifyBoundary(head, 9)
	if err != nil {
		t.Fatal(err)
	}
	generation := c.AuthenticationGeneration()
	if err := c.BindScannerElection(second, end); err != nil {
		t.Fatal(err)
	}
	if c.AuthenticationGeneration() <= generation {
		t.Fatal("changed start did not advance authentication")
	}
	if err := c.validateClassification(boundary); err == nil || !strings.Contains(err.Error(), "stale") {
		t.Fatalf("old boundary error=%v", err)
	}
}
