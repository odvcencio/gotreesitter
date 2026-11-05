package parsercorephase0

import "bytes"

// CheckpointCopyMatch distinguishes an unavailable or changed receipt from
// a private copy whose bytes no longer match its authenticated checkpoint.
type CheckpointCopyMatch uint8

const (
	CheckpointCopyUnknown CheckpointCopyMatch = iota
	CheckpointCopyChanged
	CheckpointCopyExact
)

// MatchCheckpointPrivateCopy validates metadata and exact private bytes with
// one retained-record lookup. The digest pointer is read only during this call;
// the method neither retains it nor exposes checkpoint storage. Generation
// and destination authentication remain the caller's responsibility.
func (c *Core) MatchCheckpointPrivateCopy(id CheckpointID, private []byte, digest *[32]byte) CheckpointCopyMatch {
	if c == nil || digest == nil {
		return CheckpointCopyUnknown
	}
	if id == 0 {
		if len(private) != 0 || *digest != emptyCheckpointDigest {
			return CheckpointCopyUnknown
		}
		return CheckpointCopyExact
	}
	if uint64(id) > uint64(len(c.checkpoints.records)) {
		return CheckpointCopyUnknown
	}
	record := &c.checkpoints.records[id-1]
	if uint64(len(private)) != uint64(record.length) {
		return CheckpointCopyUnknown
	}
	start := uint64(record.offset)
	end := start + uint64(record.length)
	if end < start || end > uint64(len(c.checkpoints.bytes)) {
		return CheckpointCopyChanged
	}
	want, exact := c.checkpoints.recordDigest(id, *record)
	if !exact || *digest != want {
		return CheckpointCopyUnknown
	}
	if !bytes.Equal(c.checkpoints.bytes[start:end], private) {
		return CheckpointCopyChanged
	}
	return CheckpointCopyExact
}
