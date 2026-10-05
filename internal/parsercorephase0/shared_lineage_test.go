package parsercorephase0

import (
	"errors"
	"testing"
)

func newSharedLineageCore(t *testing.T) *Core {
	t.Helper()
	compact := reserveTestCore(t, reserveTestLimits())
	if err := compact.EnableSharedLineageRecords(); err != nil {
		t.Fatal(err)
	}
	return compact
}

func TestSharedLineagesKeepIndependentOwnersAndMutableHistory(t *testing.T) {
	compact := newSharedLineageCore(t)
	left, _ := compact.Seed(1, 0)
	right, _ := compact.Seed(2, 0)
	for index, head := range []Head{left, right} {
		if err := compact.recordHeadOwner(head, uint32(index+1)); err != nil {
			t.Fatal(err)
		}
		if err := compact.recordNodeLineage(head, CleanPathRankSelected, 7); err != nil {
			t.Fatal(err)
		}
	}
	if len(compact.nodeLineages) != 1 {
		t.Fatalf("identical history uses %d records", len(compact.nodeLineages))
	}
	before, _ := compact.nodeLineageValue(right.Node)
	if err := compact.recordNodeLineage(left, CleanPathRankUnknown, 0); err != nil {
		t.Fatal(err)
	}
	leftRecord, _ := compact.nodeLineageValue(left.Node)
	rightRecord, _ := compact.nodeLineageValue(right.Node)
	if leftRecord.owner != 1 || leftRecord.rank != CleanPathRankUnknown || rightRecord != before || rightRecord.owner != 2 {
		t.Fatalf("history mutation leaked across owners: left=%+v right=%+v before=%+v", leftRecord, rightRecord, before)
	}
}

func TestSharedLineageRollbackRestoresReferencesAndReusesNodeIDs(t *testing.T) {
	compact := newSharedLineageCore(t)
	head, _ := compact.Seed(1, 0)
	if err := compact.recordHeadOwner(head, 1); err != nil {
		t.Fatal(err)
	}
	if err := compact.recordNodeLineage(head, CleanPathRankSelected, 7); err != nil {
		t.Fatal(err)
	}
	before, _ := compact.nodeLineageValue(head.Node)
	beforeRecords := len(compact.nodeLineages)
	abort := errors.New("rollback")
	var discarded Head
	err := compact.ApplySchedulerAtomic(func(owner SchedulerTransactionToken) error {
		if err := compact.recordNodeLineage(head, CleanPathRankUnknown, 0); err != nil {
			return err
		}
		var err error
		discarded, err = compact.Seed(2, 0)
		if err != nil {
			return err
		}
		if err := compact.recordHeadOwner(discarded, 2); err != nil {
			return err
		}
		return abort
	})
	after, errRead := compact.nodeLineageValue(head.Node)
	if !errors.Is(err, abort) || errRead != nil || after != before || len(compact.nodeLineages) != beforeRecords {
		t.Fatalf("rollback err=%v lineage=%+v/%+v records=%d/%d read=%v", err, after, before, len(compact.nodeLineages), beforeRecords, errRead)
	}
	if _, err := compact.nodeLineageValue(discarded.Node); err == nil {
		t.Fatal("rolled-back node still resolves")
	}
	replacement, err := compact.Seed(2, 0)
	if err != nil || replacement != discarded {
		t.Fatalf("replacement=%+v err=%v", replacement, err)
	}
	if err := compact.recordNodeLineage(replacement, CleanPathRankUnknown, 0); err != nil {
		t.Fatal(err)
	}
	value, err := compact.nodeLineageValue(replacement.Node)
	if err != nil || value.owner != 0 || value.rank != CleanPathRankUnknown {
		t.Fatalf("reused node=%+v err=%v", value, err)
	}
}

func TestSharedLineageMetadataIsBoundedAndResetInvalidatesHistory(t *testing.T) {
	compact := newSharedLineageCore(t)
	compact.limits.MaxMetadata = 1
	head, _ := compact.Seed(1, 0)
	if err := compact.recordNodeLineage(head, CleanPathRankSelected, 7); err != nil {
		t.Fatal(err)
	}
	before, _ := compact.nodeLineageValue(head.Node)
	if err := compact.recordNodeLineage(head, CleanPathRankUnknown, 0); err == nil {
		t.Fatal("metadata cap did not stop a new history")
	}
	after, _ := compact.nodeLineageValue(head.Node)
	if after != before {
		t.Fatal("metadata cap changed published history")
	}
	if err := compact.Reset(); err != nil {
		t.Fatal(err)
	}
	if _, err := compact.nodeLineageValue(head.Node); err == nil {
		t.Fatal("reset retained a live lineage")
	}
	if err := compact.ResetReleasingRetention(); err != nil {
		t.Fatal(err)
	}
	if compact.nodeOwners != nil || compact.nodeLineageRefs != nil || compact.nodeLineageIntern != nil {
		t.Fatal("release retained shared metadata storage")
	}
}

func TestSharedLineageStorageDoesNotGrowWithIdenticalHistory(t *testing.T) {
	compact := newSharedLineageCore(t)
	for index := 0; index < 1000; index++ {
		head, err := compact.Seed(StateID(index+1), 0)
		if err != nil {
			t.Fatal(err)
		}
		if err := compact.recordHeadOwner(head, uint32(index+1)); err != nil {
			t.Fatal(err)
		}
		if err := compact.recordNodeLineage(head, CleanPathRankSelected, 7); err != nil {
			t.Fatal(err)
		}
	}
	if len(compact.nodeLineages) != 1 || compact.sharedLineageEntries() != 1 {
		t.Fatal("identical histories did not share one immutable record")
	}
	if cap(compact.nodeOwners)+cap(compact.nodeLineageRefs) >= 4*len(compact.nodes) {
		t.Fatal("node references retained excessive capacity")
	}
}
