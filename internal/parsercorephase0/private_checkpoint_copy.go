package parsercorephase0

import "bytes"

// PrivateCheckpointCopy contains authenticated metadata and caller-owned
// immutable bytes. Bytes never refers to the checkpoint interner's storage.
type PrivateCheckpointCopy struct {
	Bytes  []byte
	Length uint32
	SHA256 [32]byte
}

// PrivateCheckpointCopyOwned authenticates one checkpoint under the active
// scheduler owner. It reuses a caller-owned immutable copy only after exact
// comparison with independently retained interner bytes; otherwise it copies
// those bytes into a new private allocation.
func (c *Core) PrivateCheckpointCopyOwned(owner SchedulerTransactionToken, id CheckpointID, private []byte) (PrivateCheckpointCopy, bool) {
	if c == nil || c.validateSchedulerTransaction(owner) != nil {
		return PrivateCheckpointCopy{}, false
	}
	if id == 0 {
		return PrivateCheckpointCopy{SHA256: emptyCheckpointDigest}, true
	}
	record, ok := c.checkpoints.record(id)
	if !ok {
		return PrivateCheckpointCopy{}, false
	}
	start := uint64(record.offset)
	end := start + uint64(record.length)
	if end < start || end > uint64(len(c.checkpoints.bytes)) {
		return PrivateCheckpointCopy{}, false
	}
	authoritative := c.checkpoints.bytes[start:end]
	if !bytes.Equal(private, authoritative) {
		private = append([]byte(nil), authoritative...)
	}
	digest, exact := c.checkpoints.recordDigest(id, record)
	if !exact {
		return PrivateCheckpointCopy{}, false
	}
	return PrivateCheckpointCopy{Bytes: private, Length: record.length, SHA256: digest}, true
}
