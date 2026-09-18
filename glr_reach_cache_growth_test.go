package gotreesitter

import (
	"runtime"
	"testing"
	"unsafe"
)

func reachGrowthPreflight() *gssMainPreflight {
	return &gssMainPreflight{scratch: &glrMergeScratch{}, reachEpoch: 1, reachCacheGeneration: 1, virtualLink: make(map[*gssNode][]gssMainLink)}
}

func TestGSSReachCacheStartsSmallAndOverwriteDoesNotGrow(t *testing.T) {
	p := reachGrowthPreflight()
	a, b := &gssNode{}, &gssNode{}
	p.cacheReach(a, b, true)
	if len(p.reachCache) != 64 {
		t.Fatalf("initial entries=%d, want 64", len(p.reachCache))
	}
	wantBytes := int64(cap(p.reachCache)) * int64(unsafe.Sizeof(gssReachCacheEntry{}))
	if wantBytes > 2048 || p.scratch.preflightReachCacheBytes != wantBytes {
		t.Fatalf("initial bytes=%d recorded=%d", wantBytes, p.scratch.preflightReachCacheBytes)
	}
	for i := 0; i < 10000; i++ {
		p.cacheReach(a, b, true)
	}
	if len(p.reachCache) != 64 {
		t.Fatalf("repeated overwrite grew cache to %d", len(p.reachCache))
	}
	if value, ok := p.cachedReach(a, b); !ok || !value {
		t.Fatal("overwrite lost cached result")
	}
}

func TestGSSReachCacheCollisionGrowthIsBounded(t *testing.T) {
	p := reachGrowthPreflight()
	nodes := make([]gssNode, 2048)
	for i := 1; i < len(nodes); i++ {
		for j := 0; j < 32; j++ {
			p.cacheReach(&nodes[i], &nodes[j], i > j)
			if len(p.reachCache) > maxGSSPreflightReachCacheEntries {
				t.Fatal("cache exceeded its entry limit")
			}
			if got, ok := p.cachedReach(&nodes[i], &nodes[j]); !ok || got != (i > j) {
				t.Fatal("insertion lost its result during collision or growth")
			}
		}
	}
	if len(p.reachCache) <= 64 {
		t.Fatal("distinct colliding inserts did not grow the cache")
	}
	if got, want := p.scratch.preflightReachCacheBytes, int64(cap(p.reachCache))*int64(unsafe.Sizeof(gssReachCacheEntry{})); got != want {
		t.Fatalf("growth bytes=%d, want %d", got, want)
	}
	runtime.KeepAlive(nodes)
}

func TestGSSReachCacheEpochAndGenerationInvalidation(t *testing.T) {
	p := reachGrowthPreflight()
	a, b, c := &gssNode{}, &gssNode{}, &gssNode{}
	p.cacheReach(a, b, true)
	p.cacheReach(a, c, false)
	p.bumpReachEpoch()
	if value, ok := p.cachedReach(a, b); !ok || !value {
		t.Fatal("positive proof did not survive monotonic link addition")
	}
	if _, ok := p.cachedReach(a, c); ok {
		t.Fatal("negative proof survived link epoch change")
	}
	p.reachEpoch = ^uint32(0)
	p.bumpReachEpoch()
	if _, ok := p.cachedReach(a, b); ok {
		t.Fatal("proof survived epoch wrap")
	}
	for _, wrap := range []bool{false, true} {
		p.cacheReach(a, b, true)
		if wrap {
			p.reachCacheGeneration = ^uint32(0)
		}
		p.resetReachCacheGeneration()
		p.cacheReach(b, c, false)
		if _, ok := p.cachedReach(a, b); ok {
			t.Fatalf("old proof survived generation reset, wrap=%v", wrap)
		}
	}
}

func TestGSSReachCacheGrowthPreservesReachabilityOracle(t *testing.T) {
	p := reachGrowthPreflight()
	nodes := make([]gssNode, 96)
	for i := range nodes {
		nodes[i].depth = uint32(i + 1)
		if i > 0 {
			nodes[i].prev = &nodes[i-1]
		}
	}
	for pass := 0; pass < 2; pass++ {
		for i := range nodes {
			for j := range nodes {
				if got := p.canReach(&nodes[i], &nodes[j]); got != (i >= j) {
					t.Fatalf("pass=%d from=%d to=%d reach=%v", pass, i, j, got)
				}
			}
		}
	}
	if len(p.reachCache) <= 64 {
		t.Fatal("oracle fixture did not exercise growth")
	}
	isolated := &gssNode{}
	if p.canReach(&nodes[95], isolated) {
		t.Fatal("isolated target was reachable")
	}
	p.addVirtualLink(&nodes[0], isolated, stackEntry{})
	if !p.canReach(&nodes[95], isolated) {
		t.Fatal("cached negative concealed a virtual path")
	}
	if p.canReach(isolated, &nodes[95]) {
		t.Fatal("virtual edge created reverse reachability")
	}
	runtime.KeepAlive(nodes)
}
