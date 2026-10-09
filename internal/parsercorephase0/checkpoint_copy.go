package parsercorephase0

// CheckpointCopyScratch owns temporary copies used when transferring scanner
// checkpoints into a result tree. Consumers must copy the returned bytes before
// the next CopyPair call. Neither buffer aliases the core's checkpoint storage.
type CheckpointCopyScratch struct {
	start []byte
	end   []byte
}

// CopyPair copies a complete, nonempty checkpoint pair. Equal checkpoint IDs
// share one temporary copy. IDs are resolved on every call, so reset or rollback
// of the core cannot make a stale ID-to-bytes cache look valid.
func (s *CheckpointCopyScratch) CopyPair(c *Core, startID, endID CheckpointID) (start, end []byte, ok bool) {
	if s == nil || c == nil || startID == 0 || endID == 0 {
		return nil, nil, false
	}
	start, ok = c.CopyCheckpointBytes(startID, s.start)
	if !ok {
		return nil, nil, false
	}
	s.start = start
	if startID == endID {
		return start, start, true
	}
	end, ok = c.CopyCheckpointBytes(endID, s.end)
	if !ok {
		return nil, nil, false
	}
	s.end = end
	return start, end, true
}

// RetainedBytes reports backing storage for the materialization memory budget.
func (s *CheckpointCopyScratch) RetainedBytes() uint64 {
	if s == nil {
		return 0
	}
	return uint64(cap(s.start)) + uint64(cap(s.end))
}

// Reset retains at most two 4 KiB buffers between parses. Larger checkpoints
// remain supported, but their temporary copies do not stay on the parser.
func (s *CheckpointCopyScratch) Reset() {
	if s == nil {
		return
	}
	for _, buffer := range []*[]byte{&s.start, &s.end} {
		if cap(*buffer) > 4096 {
			*buffer = nil
		} else {
			*buffer = (*buffer)[:0]
		}
	}
}
