package parsercorephase0

import (
	"bytes"
	"crypto/sha256"
	"errors"
	"fmt"
	"runtime"
	"testing"
	"unsafe"
)

// Alternate retained states to exercise bucketing instead of the consecutive
// checkpoint fast path. Reset batches exercise allocation and storage growth.
func BenchmarkCheckpointInterner(b *testing.B) {
	for _, size := range []int{4, 64, 1024} {
		b.Run(fmt.Sprintf("bytes=%d", size), func(b *testing.B) {
			states := make([][]byte, 256)
			for index := range states {
				states[index] = bytes.Repeat([]byte{byte(index)}, size)
			}
			b.Run("retained", func(b *testing.B) {
				interner := newCheckpointInterner(256, uint64(256*size))
				for _, state := range states {
					if _, err := interner.intern(state); err != nil {
						b.Fatal(err)
					}
				}
				b.ReportAllocs()
				b.ResetTimer()
				for index := 0; index < b.N; index++ {
					if _, err := interner.intern(states[index&255]); err != nil {
						b.Fatal(err)
					}
				}
			})
			b.Run("new", func(b *testing.B) {
				b.ReportAllocs()
				for index := 0; index < b.N; index++ {
					interner := newCheckpointInterner(256, uint64(256*size))
					for _, state := range states {
						if _, err := interner.intern(state); err != nil {
							b.Fatal(err)
						}
					}
				}
			})
		})
	}
}

func TestCheckpointInternerExactIdentityAndHashCollision(t *testing.T) {
	interner := newCheckpointInterner(8, 64)
	empty, err := interner.intern(nil)
	if err != nil || empty != 0 {
		t.Fatalf("empty checkpoint=(%d,%v), want (0,nil)", empty, err)
	}

	scratch := []byte{1, 2, 3}
	first, err := interner.intern(scratch)
	if err != nil || first == 0 {
		t.Fatalf("first checkpoint=(%d,%v)", first, err)
	}
	scratch[0] = 9
	repeated, err := interner.intern([]byte{1, 2, 3})
	if err != nil || repeated != first {
		t.Fatalf("owned exact checkpoint=(%d,%v), want %d", repeated, err, first)
	}

	forced := uint64(7)
	left, err := interner.internHash([]byte("left"), forced)
	if err != nil {
		t.Fatal(err)
	}
	right, err := interner.internHash([]byte("right"), forced)
	if err != nil {
		t.Fatal(err)
	}
	if left == right || left == 0 || right == 0 {
		t.Fatalf("hash collision collapsed exact states: left=%d right=%d", left, right)
	}
	if again, _ := interner.internHash([]byte("left"), forced); again != left {
		t.Fatalf("collision-chain lookup=%d, want %d", again, left)
	}
	for _, checkpoint := range []struct {
		id    CheckpointID
		bytes []byte
	}{{left, []byte("left")}, {right, []byte("right")}} {
		if interner.records[checkpoint.id-1].digestValid {
			t.Fatal("interning computed a receipt digest")
		}
		for repeat := 0; repeat < 2; repeat++ {
			length, digest, ok := interner.receipt(checkpoint.id)
			if !ok || length != uint32(len(checkpoint.bytes)) || digest != sha256.Sum256(checkpoint.bytes) {
				t.Fatalf("colliding checkpoint receipt=(%d,%x,%t)", length, digest, ok)
			}
		}
		if !interner.records[checkpoint.id-1].digestValid {
			t.Fatal("receipt digest was not cached")
		}
		if !interner.matches(checkpoint.id, checkpoint.bytes) {
			t.Fatal("colliding checkpoint did not match its own bytes")
		}
	}
	if interner.matches(left, []byte("right")) || interner.matches(right, []byte("left")) {
		t.Fatal("hash collision admitted different checkpoint bytes")
	}
	nonempty, err := interner.internHash([]byte{1}, 0)
	if err != nil || nonempty == 0 {
		t.Fatalf("nonempty state using empty-like digest=(%d,%v)", nonempty, err)
	}
	if stats := interner.stats(); stats.Unique != 4 || stats.SerializedBytes != 13 || stats.DigestCollisions != 1 {
		t.Fatalf("checkpoint stats=%+v", stats)
	}
}

func TestEmptyCheckpointReceiptIsConstantAndAllocationFree(t *testing.T) {
	interner := newCheckpointInterner(1, 1)
	if length, digest, ok := interner.receipt(0); !ok || length != 0 || digest != emptyCheckpointDigest || digest != sha256.Sum256(nil) {
		t.Fatalf("empty receipt=(%d,%x,%t)", length, digest, ok)
	}
	if allocs := testing.AllocsPerRun(1000, func() {
		length, digest, ok := interner.receipt(0)
		if !ok || length != 0 || digest != emptyCheckpointDigest {
			panic("empty checkpoint receipt drift")
		}
	}); allocs != 0 {
		t.Fatalf("empty checkpoint receipt allocations=%v, want 0", allocs)
	}
}

func TestCheckpointInternerHashCollisionComparesAllBytes(t *testing.T) {
	interner := newCheckpointInterner(16, 1024)
	states := [][]byte{
		bytes.Repeat([]byte{1}, 64),
		bytes.Repeat([]byte{1}, 64),
		bytes.Repeat([]byte{1}, 64),
		bytes.Repeat([]byte{1}, 64),
		bytes.Repeat([]byte{1}, 63),
	}
	states[1][0] = 2
	states[2][32] = 2
	states[3][63] = 2
	for index, state := range states {
		id, err := interner.internHash(state, 0)
		if err != nil || id != CheckpointID(index+1) {
			t.Fatalf("colliding state %d=(%d,%v), want ID %d", index, id, err, index+1)
		}
	}
	for index, state := range states {
		id, err := interner.internHash(state, 0)
		if err != nil || id != CheckpointID(index+1) {
			t.Fatalf("collision-chain state %d=(%d,%v), want ID %d", index, id, err, index+1)
		}
		for other, candidate := range states {
			if got := interner.matches(id, candidate); got != (index == other) {
				t.Fatalf("colliding state %d matches state %d=%t", index, other, got)
			}
		}
	}
}

func TestCheckpointInternerCopyBytesExactAndReusesCapacity(t *testing.T) {
	interner := newCheckpointInterner(4, 64)
	want := []byte{1, 2, 3, 4}
	id, err := interner.intern(want)
	if err != nil {
		t.Fatal(err)
	}

	destination := make([]byte, 0, 8)
	backing := &destination[:cap(destination)][0]
	got, ok := interner.copyBytes(id, destination)
	if !ok || !bytes.Equal(got, want) {
		t.Fatalf("copied checkpoint=(%v,%t), want %v", got, ok, want)
	}
	if cap(got) != cap(destination) || &got[:cap(got)][0] != backing {
		t.Fatalf("copy did not reuse destination capacity: cap=%d/%d", cap(got), cap(destination))
	}

	got[0] = 9
	repeated, ok := interner.copyBytes(id, got[:0])
	if !ok || !bytes.Equal(repeated, want) {
		t.Fatalf("retained checkpoint changed through copied bytes=(%v,%t), want %v", repeated, ok, want)
	}

	emptyDestination := make([]byte, 3, 8)
	empty, ok := interner.copyBytes(0, emptyDestination)
	if !ok || len(empty) != 0 || cap(empty) != cap(emptyDestination) {
		t.Fatalf("empty checkpoint copy=(%v,%t), cap=%d, want empty with cap=%d", empty, ok, cap(empty), cap(emptyDestination))
	}

	sentinel := []byte{7, 8, 9}
	before := append([]byte(nil), sentinel...)
	if copied, ok := interner.copyBytes(CheckpointID(99), sentinel); ok || copied != nil {
		t.Fatalf("invalid checkpoint copy=(%v,%t), want (nil,false)", copied, ok)
	}
	if !bytes.Equal(sentinel, before) {
		t.Fatalf("invalid checkpoint copy mutated destination=%v, want %v", sentinel, before)
	}
}

func TestCoreCheckpointByteCopyMatchesReceiptAndOwnerContract(t *testing.T) {
	compact, err := New(&fakeTable{}, Limits{MaxCheckpoints: 4, MaxCheckpointBytes: 16})
	if err != nil {
		t.Fatal(err)
	}
	want := []byte{4, 5, 6}
	id := mustInternCheckpoint(t, compact, want)
	destination := make([]byte, 0, 8)

	length, digest, receiptOK := compact.CheckpointReceipt(id)
	if !receiptOK || length != uint32(len(want)) || digest != sha256.Sum256(want) {
		t.Fatalf("checkpoint receipt=(%d,%x,%t)", length, digest, receiptOK)
	}
	got, copyOK := compact.CopyCheckpointBytes(id, destination)
	if !copyOK || !bytes.Equal(got, want) {
		t.Fatalf("checkpoint copy=(%v,%t), want %v", got, copyOK, want)
	}

	if err := compact.ApplyAtomic(func() error {
		inside, ok := compact.CopyCheckpointBytes(id, got[:0])
		if !ok || !bytes.Equal(inside, want) {
			t.Fatalf("transaction checkpoint copy=(%v,%t), want %v", inside, ok, want)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}

	if copied, ok := compact.CopyCheckpointBytesOwned(SchedulerTransactionToken{}, id, destination); ok || copied != nil {
		t.Fatalf("unowned checkpoint copy=(%v,%t), want (nil,false)", copied, ok)
	}
	var owner SchedulerTransactionToken
	if err := compact.ApplySchedulerAtomic(func(token SchedulerTransactionToken) error {
		owner = token
		inside, ok := compact.CopyCheckpointBytesOwned(token, id, destination)
		if !ok || !bytes.Equal(inside, want) {
			t.Fatalf("owned checkpoint copy=(%v,%t), want %v", inside, ok, want)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if copied, ok := compact.CopyCheckpointBytesOwned(owner, id, destination); ok || copied != nil {
		t.Fatalf("stale-owner checkpoint copy=(%v,%t), want (nil,false)", copied, ok)
	}

	if copied, ok := compact.CopyCheckpointBytes(CheckpointID(99), destination); ok || copied != nil {
		t.Fatalf("invalid core checkpoint copy=(%v,%t), want (nil,false)", copied, ok)
	}
}

func BenchmarkCheckpointReceiptEmpty(b *testing.B) {
	interner := newCheckpointInterner(1, 1)
	for i := 0; i < b.N; i++ {
		_, _, _ = interner.receipt(0)
	}
}

func TestCheckpointInternerBoundsAreAtomic(t *testing.T) {
	countBound := newCheckpointInterner(1, 16)
	if _, err := countBound.intern([]byte{1}); err != nil {
		t.Fatal(err)
	}
	before := countBound.stats()
	if _, err := countBound.intern([]byte{2}); err == nil {
		t.Fatal("checkpoint identity cap was not enforced")
	}
	if got := countBound.stats(); got != before {
		t.Fatalf("identity-cap failure mutated stats: got=%+v want=%+v", got, before)
	}

	byteBound := newCheckpointInterner(4, 2)
	if _, err := byteBound.intern([]byte{1}); err != nil {
		t.Fatal(err)
	}
	before = byteBound.stats()
	if _, err := byteBound.intern([]byte{2, 3}); err == nil {
		t.Fatal("checkpoint byte cap was not enforced")
	}
	if got := byteBound.stats(); got != before {
		t.Fatalf("byte-cap failure mutated stats: got=%+v want=%+v", got, before)
	}
}

func TestCheckpointInternerResetAndTransactionContract(t *testing.T) {
	compact, err := New(&fakeTable{}, Limits{MaxCheckpoints: 4, MaxCheckpointBytes: 16})
	if err != nil {
		t.Fatal(err)
	}
	id := mustInternCheckpoint(t, compact, []byte{1, 2, 3})
	if err := compact.SetPhaseCheckpoint(id); err != nil {
		t.Fatal(err)
	}
	recordCap, byteCap := cap(compact.checkpoints.records), cap(compact.checkpoints.bytes)
	sentinel := errors.New("rollback")
	err = compact.ApplyAtomic(func() error {
		if _, err := compact.InternCheckpoint([]byte{4}); err == nil {
			return errors.New("transaction admitted checkpoint interning")
		}
		if err := compact.SetPhaseCheckpoint(id); err == nil {
			return errors.New("transaction admitted checkpoint selection")
		}
		return sentinel
	})
	if !errors.Is(err, sentinel) || compact.checkpoint != id || compact.CheckpointInternerStats().Unique != 1 {
		t.Fatalf("transaction contract drift: err=%v checkpoint=%d stats=%+v", err, compact.checkpoint, compact.CheckpointInternerStats())
	}
	if err := compact.Reset(); err != nil {
		t.Fatal(err)
	}
	if compact.checkpoint != 0 || compact.CheckpointInternerStats() != (CheckpointInternerStats{}) {
		t.Fatalf("reset retained checkpoint state: id=%d stats=%+v", compact.checkpoint, compact.CheckpointInternerStats())
	}
	if cap(compact.checkpoints.records) != recordCap || cap(compact.checkpoints.bytes) != byteCap {
		t.Fatalf("reset changed interner capacity: records=%d/%d bytes=%d/%d", cap(compact.checkpoints.records), recordCap, cap(compact.checkpoints.bytes), byteCap)
	}
	if err := compact.SetPhaseCheckpoint(id); err == nil {
		t.Fatal("reset-stale checkpoint identity was accepted")
	}
	if next := mustInternCheckpoint(t, compact, []byte{9}); next != 1 {
		t.Fatalf("reset checkpoint identity=%d, want reused ID 1", next)
	}
	if length, digest, ok := compact.CheckpointReceipt(1); !ok || length != 1 || digest != sha256.Sum256([]byte{9}) {
		t.Fatalf("reset retained a stale receipt=(%d,%x,%t)", length, digest, ok)
	}
}

func TestCheckpointCompactLayoutsAMD64(t *testing.T) {
	if runtime.GOARCH != "amd64" {
		t.Skip("amd64 layout receipt")
	}
	checks := []struct {
		name string
		got  uintptr
		want uintptr
	}{
		{"CheckpointID", unsafe.Sizeof(CheckpointID(0)), 4},
		{"boundaryKey", unsafe.Sizeof(boundaryKey{}), 24},
		{"boundaryIdentity", unsafe.Sizeof(boundaryIdentity{}), 16},
		{"boundarySlot", unsafe.Sizeof(boundarySlot{}), 24},
	}
	for _, check := range checks {
		if check.got != check.want {
			t.Errorf("%s size=%d, want %d", check.name, check.got, check.want)
		}
	}
}
