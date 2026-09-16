package gotreesitter

import (
	"testing"
	"unsafe"
)

func TestCNodeMemoStandardCapacityWarmRegrowth(t *testing.T) {
	t.Run("backing", func(t *testing.T) {
		p := &Parser{cNodeMemoCache: make([]cNodeMemoCacheEntry, cNodeMemoCacheSize)}
		backing := &p.cNodeMemoCache[0]
		for operation := 0; operation < 3; operation++ {
			state := p.beginParseOperationBudget()
			if len(p.cNodeMemoCache) != cNodeMemoCacheInitialSize {
				t.Fatalf("operation %d started with %d entries", operation, len(p.cNodeMemoCache))
			}
			p.beginCNodeMemoEpoch()
			p.growCNodeMemoCacheTo(cNodeMemoCacheSize)
			p.endParseOperationBudget(state)
			if &p.cNodeMemoCache[0] != backing {
				t.Fatalf("operation %d replaced the retained standard backing array", operation)
			}
		}
	})
	t.Run("allocations", func(t *testing.T) {
		p := &Parser{cNodeMemoCache: make([]cNodeMemoCacheEntry, cNodeMemoCacheSize)}
		allocations := testing.AllocsPerRun(20, func() {
			state := p.beginParseOperationBudget()
			p.beginCNodeMemoEpoch()
			p.growCNodeMemoCacheTo(cNodeMemoCacheSize)
			p.endParseOperationBudget(state)
		})
		if allocations != 0 {
			t.Fatalf("warm standard regrowth allocated %.0f objects per operation, want zero", allocations)
		}
	})
}

func TestCNodeMemoStandardRegrowthClearsActiveAndTail(t *testing.T) {
	for _, tc := range []struct {
		name  string
		epoch uint16
		want  uint16
	}{
		{name: "current_epoch", epoch: 9, want: 10},
		{name: "epoch_wrap", epoch: ^uint16(0), want: 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p := &Parser{cNodeMemoCache: make([]cNodeMemoCacheEntry, cNodeMemoCacheSize), cNodeMemoEpoch: tc.epoch}
			stale := cNodeMemoCacheEntry{node: 1, ver: 7, cost: 11, visCount: 13, epoch: tc.want, hasCost: true, hasVis: true}
			for i := range p.cNodeMemoCache {
				p.cNodeMemoCache[i] = stale
			}
			state := p.beginParseOperationBudget()
			defer p.endParseOperationBudget(state)
			p.beginCNodeMemoEpoch()
			if p.cNodeMemoEpoch != tc.want {
				t.Fatalf("epoch = %d, want %d", p.cNodeMemoEpoch, tc.want)
			}
			p.cNodeMemoCache[0] = stale
			if p.cNodeMemoCache[:cap(p.cNodeMemoCache)][cNodeMemoCacheSize-1] != stale {
				t.Fatal("setup lost the hidden stale tail")
			}
			p.cNodeMemoThrash = cNodeMemoThrashGrowThreshold
			p.growCNodeMemoCacheTo(cNodeMemoCacheSize)
			if len(p.cNodeMemoCache) != cNodeMemoCacheSize || p.cNodeMemoThrash != 0 || p.cNodeMemoEpoch != tc.want {
				t.Fatalf("growth changed length, thrash, or epoch: %d/%d/%d", len(p.cNodeMemoCache), p.cNodeMemoThrash, p.cNodeMemoEpoch)
			}
			for i, entry := range p.cNodeMemoCache {
				if entry != (cNodeMemoCacheEntry{}) {
					t.Fatalf("slot %d retained an entry after growth: %+v", i, entry)
				}
			}
			if p.cNodeMemoPeakTier != RecoveryNodeMemoTierStandard || p.cNodeMemoOperationPeakTier != RecoveryNodeMemoTierStandard {
				t.Fatal("growth did not preserve standard-tier peak telemetry")
			}
		})
	}
}

func TestCNodeMemoStandardRegrowthDuringChildRecursion(t *testing.T) {
	for _, partial := range []string{"empty", "cost_only", "visibility_only"} {
		t.Run(partial, func(t *testing.T) {
			lang := &Language{SymbolMetadata: []SymbolMetadata{{}, {Visible: true}}}
			p := &Parser{language: lang, cNodeMemoCache: make([]cNodeMemoCacheEntry, cNodeMemoCacheSize)}
			state := p.beginParseOperationBudget()
			defer p.endParseOperationBudget(state)
			p.beginCNodeMemoEpoch()
			parent, missing, other := collidingCNodeMemoNodes(t, len(p.cNodeMemoCache)>>1)
			parent.symbol, missing.symbol, other.symbol = 1, 1, 1
			missing.setMissing(true)
			parent.children = []*Node{missing, other}
			wantCost := cNodeErrorCostLang(lang, parent)
			wantVisible := cNodeVisibleSubtreeCountUncachedLang(lang, parent)
			if wantCost == 0 || wantVisible == 0 {
				t.Fatal("fixture must exercise nonzero cost and visibility")
			}
			slot := p.cNodeMemoSlot(parent)
			slot.ver = parent.equivVersion
			if partial == "cost_only" {
				slot.cost, slot.hasCost = wantCost, true
			}
			if partial == "visibility_only" {
				slot.visCount, slot.hasVis = uint32(wantVisible), true
			}
			p.cNodeMemoThrash = cNodeMemoThrashGrowThreshold - 1
			cost, visible := p.cNodeErrorCostAndVisibleSubtreeCount(parent)
			if len(p.cNodeMemoCache) != cNodeMemoCacheSize {
				t.Fatal("the child collision did not grow the cache")
			}
			if cost != wantCost || visible != wantVisible {
				t.Fatalf("recursive growth changed aggregates: %d/%d, want %d/%d", cost, visible, wantCost, wantVisible)
			}
			cost, visible = p.cNodeErrorCostAndVisibleSubtreeCount(parent)
			if cost != wantCost || visible != wantVisible {
				t.Fatal("the next aggregate lookup consumed a stale slot after growth")
			}
		})
	}
}

func TestCNodeMemoStandardRegrowthThresholdAndSlot(t *testing.T) {
	p := &Parser{cNodeMemoCache: make([]cNodeMemoCacheEntry, cNodeMemoCacheSize)}
	state := p.beginParseOperationBudget()
	defer p.endParseOperationBudget(state)
	p.beginCNodeMemoEpoch()
	a, b, c := collidingCNodeMemoNodes(t, len(p.cNodeMemoCache)>>1)
	p.cNodeMemoSlot(a)
	p.cNodeMemoSlot(b)
	if p.cNodeMemoThrash != 1 || len(p.cNodeMemoCache) != cNodeMemoCacheInitialSize {
		t.Fatal("first collision changed the standard growth threshold")
	}
	p.cNodeMemoThrash = cNodeMemoThrashGrowThreshold - 2
	p.cNodeMemoSlot(c)
	if p.cNodeMemoThrash != cNodeMemoThrashGrowThreshold-1 || len(p.cNodeMemoCache) != cNodeMemoCacheInitialSize {
		t.Fatal("memo grew before the threshold collision")
	}
	slot := p.cNodeMemoSlot(a)
	if len(p.cNodeMemoCache) != cNodeMemoCacheSize || p.cNodeMemoThrash != 0 {
		t.Fatal("memo did not grow at the threshold collision")
	}
	wantIndex := cNodeMemoCacheIndex(uintptr(unsafe.Pointer(a)), len(p.cNodeMemoCache)>>1)
	want := cNodeMemoCacheEntry{node: uintptr(unsafe.Pointer(a)), epoch: p.cNodeMemoEpoch}
	if slot != &p.cNodeMemoCache[wantIndex] || *slot != want {
		t.Fatalf("growth returned a stale or incorrectly addressed slot: %+v", *slot)
	}
	for i, entry := range p.cNodeMemoCache {
		if i != wantIndex && entry != (cNodeMemoCacheEntry{}) {
			t.Fatalf("growth retained the old memo entry at %d", i)
		}
	}
	if p.cNodeMemoCollisionCount() != 3 {
		t.Fatalf("collision telemetry = %d, want 3", p.cNodeMemoCollisionCount())
	}
}

func TestCNodeMemoStandardRegrowthPreservesTemporaryOwnership(t *testing.T) {
	p := &Parser{cNodeMemoCache: make([]cNodeMemoCacheEntry, cNodeMemoCacheSize)}
	outer := p.beginParseOperationBudget()
	p.beginCNodeMemoEpoch()
	p.growCNodeMemoCacheTo(cNodeMemoCacheSize)
	standard := &p.cNodeMemoCache[0]
	inner := p.beginParseOperationBudget()
	p.growCNodeMemoCacheTo(cNodeMemoRecoveryCacheSize)
	if &p.cNodeMemoCache[0] == standard || len(p.cNodeMemoCache) != cNodeMemoRecoveryCacheSize {
		t.Fatal("temporary growth reused or replaced the standard tier incorrectly")
	}
	retained := p.forestDeclineMemo.cNodeMemoRetainedCache
	if len(retained) != cNodeMemoCacheSize || &retained[0] != standard {
		t.Fatal("temporary growth did not preserve the standard slab")
	}
	activeBytes := cNodeMemoCacheBytesForEntries(cap(p.cNodeMemoCache))
	retainedBytes := cNodeMemoCacheBytesForEntries(cap(retained))
	if activeBytes > 3<<20 || retainedBytes > 384<<10 || activeBytes+retainedBytes > (3<<20)+(384<<10) {
		t.Fatalf("temporary storage exceeds its bounds: active=%d retained=%d", activeBytes, retainedBytes)
	}
	p.endParseOperationBudget(inner)
	if len(p.cNodeMemoCache) != cNodeMemoRecoveryCacheSize || p.cNodeMemoOperationDepth != 1 {
		t.Fatal("nested cleanup released the temporary tier too soon")
	}
	p.endParseOperationBudget(outer)
	if len(p.cNodeMemoCache) != cNodeMemoCacheSize || &p.cNodeMemoCache[0] != standard || p.forestDeclineMemo.cNodeMemoRetainedCache != nil {
		t.Fatal("outer cleanup did not restore the standard slab and release temporary ownership")
	}
	peak, peakBytes, collisions := p.DebugCNodeMemoOperationStats()
	if peak != cNodeMemoRecoveryCacheSize || peakBytes != cNodeMemoCacheBytesForEntries(cNodeMemoRecoveryCacheSize) || collisions != 0 {
		t.Fatalf("temporary telemetry changed: %d/%d/%d", peak, peakBytes, collisions)
	}
	next := p.beginParseOperationBudget()
	defer p.endParseOperationBudget(next)
	if len(p.cNodeMemoCache) != cNodeMemoCacheInitialSize || p.cNodeMemoOperationPeakTier != RecoveryNodeMemoTierNone {
		t.Fatal("next operation did not reset its logical size and peak telemetry")
	}
}

func TestCNodeMemoStandardRegrowthDropsOversizedCapacity(t *testing.T) {
	p := &Parser{cNodeMemoCache: make([]cNodeMemoCacheEntry, cNodeMemoRecoveryCacheSize)[:cNodeMemoCacheInitialSize]}
	oversized := &p.cNodeMemoCache[0]
	p.beginCNodeMemoEpoch()
	p.cNodeMemoCache[0] = cNodeMemoCacheEntry{node: 1, epoch: p.cNodeMemoEpoch, hasCost: true, cost: 17}
	p.growCNodeMemoCacheTo(cNodeMemoCacheSize)
	if len(p.cNodeMemoCache) != cNodeMemoCacheSize || cap(p.cNodeMemoCache) != cNodeMemoCacheSize {
		t.Fatalf("standard growth retained oversized capacity: len=%d cap=%d", len(p.cNodeMemoCache), cap(p.cNodeMemoCache))
	}
	if &p.cNodeMemoCache[0] == oversized {
		t.Fatal("standard growth reused the temporary-sized backing array")
	}
	if p.forestDeclineMemo != nil && p.forestDeclineMemo.cNodeMemoRetainedCache != nil {
		t.Fatal("standard growth created a retained oversized alias")
	}
	for i, entry := range p.cNodeMemoCache {
		if entry != (cNodeMemoCacheEntry{}) {
			t.Fatalf("replacement standard slot %d was not empty", i)
		}
	}
}
