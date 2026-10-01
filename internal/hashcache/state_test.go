package hashcache

import (
	"testing"
	"unsafe"
)

func TestMetadataPreservesEntrySizeAndCheckpointDepth(t *testing.T) {
	type original struct {
		Ref  uint32
		Hash uint64
	}
	type candidate struct {
		Ref   uint32
		State Metadata
		Hash  uint64
	}
	if unsafe.Sizeof(original{}) != unsafe.Sizeof(candidate{}) {
		t.Fatal("depth metadata enlarged the cache entry")
	}
	if Encode(Complete).State() != Complete {
		t.Fatal("completed hash lost its state")
	}
	pending := State(1)
	if !LazyDepthSupported {
		if !pending.NeedsHash() || Encode(pending).State() != Complete {
			t.Fatal("architecture without padding must retain eager hashing")
		}
		return
	}
	for i := 1; i < MaxPendingDepth; i++ {
		if Encode(pending).State() != pending || pending.NeedsHash() {
			t.Fatal("shallow pending depth was lost or computed early")
		}
		pending = State(1).IncludeChild(pending)
	}
	if !pending.NeedsHash() || State(1).IncludeChild(Complete) != 1 {
		t.Fatal("deep chain did not checkpoint or completed child did not reset depth")
	}
}
