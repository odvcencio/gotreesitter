package queryexec

type capturePrefix[N comparable] struct {
	Previous uint64
	Name     string
	Node     N
}

// Sequences interns capture prefixes with exact keys, avoiding lossy hashes
// and quadratic comparisons between matches with a common capture prefix.
type Sequences[N comparable] struct {
	prefixes map[capturePrefix[N]]uint64
	finished map[uint64]bool
}

func (s *Sequences[N]) Extend(previous uint64, name string, node N) uint64 {
	if s.prefixes == nil {
		s.prefixes = make(map[capturePrefix[N]]uint64)
	}
	key := capturePrefix[N]{previous, name, node}
	if id, ok := s.prefixes[key]; ok {
		return id
	}
	id := uint64(len(s.prefixes)) + 1
	s.prefixes[key] = id
	return id
}

func (s *Sequences[N]) Seen(id uint64) bool {
	if s.finished == nil {
		s.finished = make(map[uint64]bool)
	}
	if s.finished[id] {
		return true
	}
	s.finished[id] = true
	return false
}
