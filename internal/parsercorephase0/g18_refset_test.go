package parsercorephase0

import (
	"errors"
	"reflect"
	"runtime"
	"slices"
	"testing"
	"unsafe"
)

func g18Ref(owner, epoch, sequence uint64, branch uint16) DropCohortRef {
	return DropCohortRef{Owner: owner, Epoch: epoch, Sequence: sequence, Branch: branch}
}

func g18RefSetFrom(t *testing.T, core *Core, refs ...DropCohortRef) DropCohortRefSet {
	t.Helper()
	var set DropCohortRefSet
	for _, ref := range refs {
		if !core.AddDropCohortRef(&set, ref) && set.Len() == 0 {
			t.Fatalf("add drop-cohort ref %v failed", ref)
		}
	}
	return set
}

func g18Members(t *testing.T, core *Core, set DropCohortRefSet) []DropCohortRef {
	t.Helper()
	members := make([]DropCohortRef, set.Len())
	for index := range members {
		var ok bool
		members[index], ok = core.DropCohortRefAt(set, index)
		if !ok {
			t.Fatalf("reference %d is invalid in %+v", index, set)
		}
	}
	return members
}

func TestG18RefSetNestedAndSequential(t *testing.T) {
	core := newTinyCore(t, 8)
	var set DropCohortRefSet
	refs := []DropCohortRef{
		g18Ref(9, 1, 2, 1),
		g18Ref(9, 1, 1, 1),
		g18Ref(9, 1, 3, 0),
		g18Ref(8, 2, 1, 0),
	}
	for _, ref := range refs {
		if !core.AddDropCohortRef(&set, ref) {
			t.Fatalf("add %v failed", ref)
		}
	}
	members := g18Members(t, core, set)
	want := slices.Clone(refs)
	slices.SortFunc(want, func(left, right DropCohortRef) int {
		if dropCohortRefLess(left, right) {
			return -1
		}
		if dropCohortRefLess(right, left) {
			return 1
		}
		return 0
	})
	if !slices.Equal(members, want) {
		t.Fatalf("ordered refs=%v, want %v", members, want)
	}
	if !set.Spilled() || set.Len() != len(want) {
		t.Fatalf("nested/sequential set=%+v, want spilled count %d", set, len(want))
	}
}

func TestG18RefSetOrderDedupeAndMultiBranchValidity(t *testing.T) {
	core := newTinyCore(t, 8)
	first := g18Ref(3, 4, 5, 1)
	set := g18RefSetFrom(t, core, first, first, g18Ref(3, 4, 5, 0))
	members := g18Members(t, core, set)
	if set.Blended() || set.Len() != 2 || len(members) != 2 {
		t.Fatalf("multi-branch set=%+v members=%v", set, members)
	}
	if !dropCohortRefLess(members[0], members[1]) {
		t.Fatalf("multi-branch order=%v", members)
	}
	if core.AddDropCohortRef(&set, first) {
		t.Fatal("exact duplicate insertion reported a change")
	}
	if set.Len() != 2 {
		t.Fatalf("exact duplicate changed count: %+v", set)
	}
}

func TestG18RefSetInlineToSpillAndOverflow(t *testing.T) {
	core := newTinyCore(t, 8)
	var set DropCohortRefSet
	for index := 0; index < dropCohortRefHardCap+1; index++ {
		ref := g18Ref(1, 1, uint64(index+1), 0)
		if !core.AddDropCohortRef(&set, ref) && index < dropCohortRefHardCap {
			t.Fatalf("add %d failed before hard cap", index)
		}
	}
	if !set.Spilled() || !set.Overflowed() || set.Len() != dropCohortRefHardCap {
		t.Fatalf("overflow set=%+v, want spilled count %d and overflow", set, dropCohortRefHardCap)
	}
	if got := len(core.dropCohortRefSpill); got != dropCohortRefHardCap {
		t.Fatalf("sequential spill length=%d, want %d", got, dropCohortRefHardCap)
	}
	before := set
	if core.AddDropCohortRef(&set, g18Ref(1, 1, 1000, 0)) {
		t.Fatal("overflowed set accepted a new reference")
	}
	if !reflect.DeepEqual(set, before) {
		t.Fatalf("overflowed set changed: before=%+v after=%+v", before, set)
	}
}

func TestG18RefSetRollbackRestoresNodeAndSpillExactly(t *testing.T) {
	core := newTinyCore(t, 8)
	head, err := core.Seed(1, 0)
	if err != nil {
		t.Fatal(err)
	}
	beforeSpill := slices.Clone(core.dropCohortRefSpill)
	sentinel := errors.New("g18 reference rollback")
	err = core.ApplySchedulerAtomic(func(owner SchedulerTransactionToken) error {
		refs := g18RefSetFrom(t, core,
			g18Ref(1, 1, 1, 0), g18Ref(1, 1, 2, 0), g18Ref(1, 1, 3, 0),
		)
		if err := core.RecordHeadLineageRefsOwned(owner, head, refs); err != nil {
			return err
		}
		return sentinel
	})
	if !errors.Is(err, sentinel) {
		t.Fatalf("rollback error=%v, want %v", err, sentinel)
	}
	got, err := core.NodeLineageDropCohortRefs(head.Node)
	if err != nil {
		t.Fatal(err)
	}
	if !got.Empty() || !slices.Equal(core.dropCohortRefSpill, beforeSpill) {
		t.Fatalf("rollback refs=%+v spill=%v, want empty and %v", got, core.dropCohortRefSpill, beforeSpill)
	}
}

func TestG18RefSetPreflightLimitsAreAtomic(t *testing.T) {
	core := newTinyCoreWithLimits(t, Limits{MaxDropCohortRefs: 2, MaxDropCohortRefBytes: 2 * uint64(unsafe.Sizeof(DropCohortRef{}))})
	var set DropCohortRefSet
	if !core.AddDropCohortRef(&set, g18Ref(1, 1, 1, 0)) || !core.AddDropCohortRef(&set, g18Ref(1, 1, 2, 0)) {
		t.Fatal("inline preflight setup failed")
	}
	beforeSet := set
	beforeSpill := slices.Clone(core.dropCohortRefSpill)
	if core.AddDropCohortRef(&set, g18Ref(1, 1, 3, 0)) {
		t.Fatal("spill preflight accepted an over-limit reference")
	}
	if !reflect.DeepEqual(set, beforeSet) || !slices.Equal(core.dropCohortRefSpill, beforeSpill) {
		t.Fatalf("failed preflight changed set=%+v spill=%v", set, core.dropCohortRefSpill)
	}
}

func TestG18RefSetAccountingResetAndRetentionRelease(t *testing.T) {
	core := newTinyCore(t, 8)
	beforeStorage := core.StorageBytes()
	var set DropCohortRefSet
	for index := 0; index < 3; index++ {
		if !core.AddDropCohortRef(&set, g18Ref(1, 1, uint64(index+1), 0)) {
			t.Fatal("spill setup failed")
		}
	}
	wantDelta := uint64(len(core.dropCohortRefSpill)) * uint64(unsafe.Sizeof(DropCohortRef{}))
	if got := core.StorageBytes() - beforeStorage; got != wantDelta {
		t.Fatalf("StorageBytes delta=%d, want %d", got, wantDelta)
	}
	if got := core.FootprintBytes(); got < wantDelta {
		t.Fatalf("FootprintBytes=%d, want at least %d", got, wantDelta)
	}
	retained := cap(core.dropCohortRefSpill)
	if err := core.Reset(); err != nil {
		t.Fatal(err)
	}
	if len(core.dropCohortRefSpill) != 0 || cap(core.dropCohortRefSpill) != retained {
		t.Fatalf("Reset spill len/cap=%d/%d, want 0/%d", len(core.dropCohortRefSpill), cap(core.dropCohortRefSpill), retained)
	}
	core.dropCohortRefSpill = make([]DropCohortRef, coreRetentionCapBytes/uint64(unsafe.Sizeof(DropCohortRef{}))+1)
	if err := core.ResetReleasingRetention(); err != nil {
		t.Fatal(err)
	}
	if core.dropCohortRefSpill != nil {
		t.Fatal("retention release kept an oversized drop-cohort spill")
	}
}

func TestG18RefSetReductionOutputAndNodePersistence(t *testing.T) {
	core := newTinyCore(t, 8)
	head, err := core.Seed(1, 0)
	if err != nil {
		t.Fatal(err)
	}
	refs := g18RefSetFrom(t, core, g18Ref(1, 2, 3, 0), g18Ref(1, 2, 3, 1), g18Ref(2, 1, 1, 0))
	if err := core.ApplySchedulerAtomic(func(owner SchedulerTransactionToken) error {
		return core.RecordReductionLineageOwned(owner, []ReductionOutput{{
			Head: head, DropCohortRefs: refs, CleanPathRank: CleanPathRankSelected, MultiplePopPaths: true,
		}}, 1)
	}); err != nil {
		t.Fatal(err)
	}
	got, err := core.NodeLineageDropCohortRefs(head.Node)
	if err != nil {
		t.Fatal(err)
	}
	members := g18Members(t, core, got)
	if len(members) != refs.Len() || got.Blended() {
		t.Fatalf("persisted refs=%+v members=%v", got, members)
	}
}

func TestG18RefSetRecordSizes(t *testing.T) {
	if got := unsafe.Sizeof(DropCohortRef{}); got != 32 {
		t.Fatalf("DropCohortRef size=%d, want 32", got)
	}
	if got := unsafe.Sizeof(DropCohortRefSet{}); got != 72 {
		t.Fatalf("DropCohortRefSet size=%d, want 72", got)
	}
	if got := unsafe.Sizeof(nodeLineageRecord{}); got != 36 {
		t.Fatalf("nodeLineageRecord size=%d, want 36", got)
	}
	if got := unsafe.Sizeof(nodeLineageMutation{}); got != 28 {
		t.Fatalf("nodeLineageMutation size=%d, want 28", got)
	}
	if got := unsafe.Sizeof(ReductionOutput{}); got != 112 {
		t.Fatalf("ReductionOutput size=%d, want 112", got)
	}
	if got := unsafe.Sizeof(LinkChainRef{}); got != 8 {
		t.Fatalf("LinkChainRef size=%d, want 8", got)
	}
	if got := unsafe.Sizeof(CondenseCandidate{}); got != 88 {
		t.Fatalf("CondenseCandidate size=%d, want 88", got)
	}
}

func TestG18RefSetInlineEnumerationAndUnionAllocations(t *testing.T) {
	core := newTinyCore(t, 8)
	left := g18RefSetFrom(t, core, g18Ref(1, 1, 1, 0))
	right := g18RefSetFrom(t, core, g18Ref(2, 1, 1, 0))
	var sink DropCohortRef
	enumerationAllocs := testing.AllocsPerRun(1000, func() {
		for index := 0; index < left.Len(); index++ {
			var ok bool
			sink, ok = core.DropCohortRefAt(left, index)
			if !ok {
				t.Fatal("inline reference enumeration failed")
			}
		}
		runtime.KeepAlive(sink)
	})
	if enumerationAllocs != 0 {
		t.Fatalf("inline enumeration allocations=%v, want zero", enumerationAllocs)
	}
	unionAllocs := testing.AllocsPerRun(1000, func() {
		dst := left
		if !core.UnionDropCohortRefs(&dst, right) {
			t.Fatal("inline union did not change the destination")
		}
		runtime.KeepAlive(dst)
	})
	if unionAllocs != 0 {
		t.Fatalf("inline union allocations=%v, want zero", unionAllocs)
	}
}
