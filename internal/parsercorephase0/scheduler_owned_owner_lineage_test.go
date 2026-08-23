package parsercorephase0

import (
	"errors"
	"reflect"
	"slices"
	"strings"
	"testing"
)

func TestRecordHeadOwnerAndLineageOwnedCommitsAllState(t *testing.T) {
	defer SetAlternativeSetRecordingEnabledForTest(true)()
	compact := newTinyCore(t, 4)
	head, err := compact.Seed(1, 0)
	if err != nil {
		t.Fatal(err)
	}
	set := NewAlternativeSetMember(11, 0)
	refs := g18RefSetFrom(t, compact,
		g18Ref(7, 2, 3, 1), g18Ref(7, 2, 3, 0), g18Ref(8, 1, 1, 0),
	)

	if err := compact.ApplySchedulerAtomic(func(owner SchedulerTransactionToken) error {
		return compact.RecordHeadOwnerAndLineageOwned(
			owner, head, 7, CleanPathRankSelected, 13, set, true, true, refs,
		)
	}); err != nil {
		t.Fatal(err)
	}
	record, err := compact.nodeLineage(head.Node)
	if err != nil {
		t.Fatal(err)
	}
	if record.owner != 7 || record.rank != CleanPathRankSelected || record.lineage != 13 ||
		!record.converged || !record.blended {
		t.Fatalf("committed owner and lineage=%+v", *record)
	}
	if got := alternativeSetMembersForTest(t, compact, record.set); !slices.Equal(got, []uint32{packAlternativeSetMember(11, 0)}) {
		t.Fatalf("committed set=%v", got)
	}
	gotRefs, err := compact.NodeLineageDropCohortRefs(head.Node)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(g18Members(t, compact, gotRefs), g18Members(t, compact, refs)) {
		t.Fatalf("committed refs=%v, want %v", gotRefs, refs)
	}
	if len(compact.nodeLineageJournal) != 0 || len(compact.transactions) != 0 {
		t.Fatalf("committed transaction retained rollback state: journal=%d transactions=%d", len(compact.nodeLineageJournal), len(compact.transactions))
	}
}

func TestRecordHeadOwnerAndLineageOwnedRollsBackAllState(t *testing.T) {
	defer SetAlternativeSetRecordingEnabledForTest(true)()
	compact := newTinyCore(t, 4)
	head, err := compact.Seed(1, 0)
	if err != nil {
		t.Fatal(err)
	}
	set := NewAlternativeSetMember(11, 0)
	refs := g18RefSetFrom(t, compact,
		g18Ref(7, 2, 3, 1), g18Ref(7, 2, 3, 0), g18Ref(8, 1, 1, 0),
	)
	beforeSpill := slices.Clone(compact.dropCohortRefSpill)
	sentinel := errors.New("combined owner and lineage rollback")
	err = compact.ApplySchedulerAtomic(func(owner SchedulerTransactionToken) error {
		if err := compact.RecordHeadOwnerAndLineageOwned(
			owner, head, 7, CleanPathRankSelected, 13, set, true, true, refs,
		); err != nil {
			return err
		}
		if len(compact.nodeLineageJournal) == 0 {
			t.Fatal("combined mutation did not journal state")
		}
		return sentinel
	})
	if !errors.Is(err, sentinel) {
		t.Fatalf("rollback error=%v, want %v", err, sentinel)
	}
	record, err := compact.nodeLineage(head.Node)
	if err != nil {
		t.Fatal(err)
	}
	if record.owner != 0 || record.rank != CleanPathRankNotApplicable || record.lineage != 0 ||
		record.converged || record.blended || record.set.Len() != 0 || !record.dropCohortRefs.Empty() {
		t.Fatalf("rolled-back owner and lineage=%+v", *record)
	}
	if !slices.Equal(compact.dropCohortRefSpill, beforeSpill) || len(compact.nodeLineageJournal) != 0 || len(compact.transactions) != 0 {
		t.Fatalf("rollback retained state: spill=%v/%v journal=%d transactions=%d", compact.dropCohortRefSpill, beforeSpill, len(compact.nodeLineageJournal), len(compact.transactions))
	}
}

func TestRecordHeadOwnerAndLineageOwnedRejectsInvalidOwnerLineage(t *testing.T) {
	compact := newTinyCore(t, 4)
	head, err := compact.Seed(1, 0)
	if err != nil {
		t.Fatal(err)
	}
	before := captureSchedulerTransactionState(compact)
	err = compact.ApplySchedulerAtomic(func(owner SchedulerTransactionToken) error {
		if innerErr := compact.RecordHeadOwnerAndLineageOwned(owner, head, 0, CleanPathRankSelected, 13, AlternativeSet{}, false, false); innerErr == nil {
			t.Fatal("zero owner lineage unexpectedly succeeded")
		}
		return nil
	})
	if err == nil || !strings.Contains(err.Error(), "poisoned scheduler transaction") {
		t.Fatalf("invalid owner lineage error=%v", err)
	}
	if !reflect.DeepEqual(captureSchedulerTransactionState(compact), before) {
		t.Fatalf("invalid owner lineage changed state: before=%+v after=%+v", before, captureSchedulerTransactionState(compact))
	}
}

func TestRecordHeadOwnerAndLineageOwnedRejectsOwnerConflict(t *testing.T) {
	compact := newTinyCore(t, 4)
	head, err := compact.Seed(1, 0)
	if err != nil {
		t.Fatal(err)
	}
	if err := compact.ApplySchedulerAtomic(func(owner SchedulerTransactionToken) error {
		return compact.RecordHeadOwnerOwned(owner, head, 7)
	}); err != nil {
		t.Fatal(err)
	}
	err = compact.ApplySchedulerAtomic(func(owner SchedulerTransactionToken) error {
		if innerErr := compact.RecordHeadOwnerAndLineageOwned(owner, head, 8, CleanPathRankSelected, 13, NewAlternativeSetMember(11, 0), true, true); innerErr == nil {
			t.Fatal("conflicting owner unexpectedly succeeded")
		}
		return nil
	})
	if err == nil || !strings.Contains(err.Error(), "poisoned scheduler transaction") {
		t.Fatalf("owner conflict error=%v", err)
	}
	record, err := compact.nodeLineage(head.Node)
	if err != nil {
		t.Fatal(err)
	}
	if record.owner != 7 || record.lineage != 0 || record.set.Len() != 0 {
		t.Fatalf("owner conflict changed state=%+v", *record)
	}
}

func TestRecordHeadOwnerAndLineageOwnedRejectsStaleToken(t *testing.T) {
	compact := newTinyCore(t, 4)
	head, err := compact.Seed(1, 0)
	if err != nil {
		t.Fatal(err)
	}
	var stale SchedulerTransactionToken
	if err := compact.ApplySchedulerAtomic(func(owner SchedulerTransactionToken) error {
		stale = owner
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	err = compact.ApplySchedulerAtomic(func(_ SchedulerTransactionToken) error {
		if innerErr := compact.RecordHeadOwnerAndLineageOwned(stale, head, 7, CleanPathRankSelected, 13, AlternativeSet{}, false, false); innerErr == nil {
			t.Fatal("stale token unexpectedly succeeded")
		}
		return nil
	})
	if err == nil || !strings.Contains(err.Error(), "poisoned scheduler transaction") {
		t.Fatalf("stale token error=%v", err)
	}
	record, err := compact.nodeLineage(head.Node)
	if err != nil {
		t.Fatal(err)
	}
	if record.owner != 0 || record.lineage != 0 || record.set.Len() != 0 {
		t.Fatalf("stale token changed state=%+v", *record)
	}
}

func TestRecordHeadOwnerAndLineageOwnedRollsBackReferenceFailure(t *testing.T) {
	compact := newTinyCore(t, 4)
	head, err := compact.Seed(1, 0)
	if err != nil {
		t.Fatal(err)
	}
	refs := g18RefSetFrom(t, compact,
		g18Ref(7, 2, 3, 1), g18Ref(7, 2, 3, 0), g18Ref(8, 1, 1, 0),
	)
	beforeSpill := slices.Clone(compact.dropCohortRefSpill)
	compact.limits.MaxDropCohortRefs = 1
	err = compact.ApplySchedulerAtomic(func(owner SchedulerTransactionToken) error {
		if innerErr := compact.RecordHeadOwnerAndLineageOwned(owner, head, 7, CleanPathRankSelected, 13, NewAlternativeSetMember(11, 0), true, true, refs); innerErr == nil {
			t.Fatal("reference limit failure unexpectedly succeeded")
		}
		return nil
	})
	if err == nil || !strings.Contains(err.Error(), "poisoned scheduler transaction") {
		t.Fatalf("reference failure error=%v", err)
	}
	record, err := compact.nodeLineage(head.Node)
	if err != nil {
		t.Fatal(err)
	}
	if record.owner != 0 || record.lineage != 0 || record.set.Len() != 0 || !record.dropCohortRefs.Empty() {
		t.Fatalf("reference failure retained partial mutation=%+v", *record)
	}
	if !slices.Equal(compact.dropCohortRefSpill, beforeSpill) || len(compact.nodeLineageJournal) != 0 {
		t.Fatalf("reference failure retained spill/journal: spill=%v/%v journal=%d", compact.dropCohortRefSpill, beforeSpill, len(compact.nodeLineageJournal))
	}
}

func TestRecordHeadOwnerOwnedRemainsOwnerOnly(t *testing.T) {
	compact := newTinyCore(t, 4)
	head, err := compact.Seed(1, 0)
	if err != nil {
		t.Fatal(err)
	}
	if err := compact.ApplySchedulerAtomic(func(owner SchedulerTransactionToken) error {
		return compact.RecordHeadOwnerOwned(owner, head, 7)
	}); err != nil {
		t.Fatal(err)
	}
	record, err := compact.nodeLineage(head.Node)
	if err != nil {
		t.Fatal(err)
	}
	if record.owner != 7 || record.lineage != 0 || record.rank != CleanPathRankNotApplicable ||
		record.converged || record.blended || record.set.Len() != 0 || !record.dropCohortRefs.Empty() {
		t.Fatalf("owner-only operation changed lineage state=%+v", *record)
	}
}

func TestRecordHeadOwnerAndLineageOwnedHonorsSetDirtyFalse(t *testing.T) {
	defer SetAlternativeSetRecordingEnabledForTest(true)()
	compact := newTinyCore(t, 4)
	head, err := compact.Seed(1, 0)
	if err != nil {
		t.Fatal(err)
	}
	set := NewAlternativeSetMember(11, 0)
	if err := compact.ApplySchedulerAtomic(func(owner SchedulerTransactionToken) error {
		return compact.RecordHeadOwnerAndLineageOwned(
			owner, head, 7, CleanPathRankUnselected, 13, set, false, false,
		)
	}); err != nil {
		t.Fatal(err)
	}
	record, err := compact.nodeLineage(head.Node)
	if err != nil {
		t.Fatal(err)
	}
	if record.owner != 7 || record.rank != CleanPathRankUnselected || record.lineage != 13 || !record.converged {
		t.Fatalf("setDirty=false scalar state=%+v", *record)
	}
	if record.set.Len() != 0 {
		t.Fatalf("setDirty=false recorded set=%+v, want empty", record.set)
	}
}
