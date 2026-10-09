package parsercorephase0

import (
	"bytes"
	"crypto/sha256"
	"errors"
	"runtime"
	"testing"
	"unsafe"
)

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

	const forced = uint64(7)
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
	nonempty, err := interner.internHash([]byte{1}, 0)
	if err != nil || nonempty == 0 {
		t.Fatalf("nonempty state using zero hash=(%d,%v)", nonempty, err)
	}
	if stats := interner.stats(); stats.Unique != 4 || stats.SerializedBytes != 13 || stats.DigestCollisions != 1 {
		t.Fatalf("checkpoint stats=%+v", stats)
	}
}

func TestCheckpointInternerCollisionReceiptsPreserveExactBytes(t *testing.T) {
	interner := newCheckpointInterner(3, 32)
	states := [][]byte{[]byte("left"), []byte("rite"), []byte("left\x00")}
	ids := make([]CheckpointID, len(states))
	for index, state := range states {
		id, err := interner.internHash(state, 0)
		if err != nil || id != CheckpointID(index+1) {
			t.Fatalf("colliding checkpoint %d=(%d,%v)", index, id, err)
		}
		ids[index] = id
	}
	before := interner.stats()
	for index, state := range states {
		id, err := interner.internHash(state, 0)
		if err != nil || id != ids[index] {
			t.Fatalf("collision lookup %d=(%d,%v), want %d", index, id, err, ids[index])
		}
		length, digest, ok := interner.receipt(id)
		if !ok || length != uint32(len(state)) || digest != sha256.Sum256(state) {
			t.Fatalf("collision receipt %d=(%d,%x,%t)", index, length, digest, ok)
		}
		if copied, ok := interner.copyBytes(id, nil); !ok || !bytes.Equal(copied, state) {
			t.Fatalf("collision bytes %d=(%v,%t)", index, copied, ok)
		}
		for other, different := range states {
			if interner.matches(id, different) != (index == other) {
				t.Fatalf("checkpoint %d confused with checkpoint %d", index, other)
			}
		}
	}
	if _, err := interner.internHash([]byte("more"), 0); err == nil {
		t.Fatal("colliding checkpoint bypassed the identity cap")
	}
	if got := interner.stats(); got != before {
		t.Fatalf("collision lookups or failed insertion changed stats: got=%+v want=%+v", got, before)
	}
	interner.reset()
	id, err := interner.internHash([]byte("rite"), 0)
	if err != nil || id != 1 || interner.stats().DigestCollisions != 0 {
		t.Fatalf("reset retained a collision chain: id=%d err=%v stats=%+v", id, err, interner.stats())
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
