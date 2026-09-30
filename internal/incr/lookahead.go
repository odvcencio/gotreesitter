// Package incr implements structural incremental-reuse conditions.
package incr

import "slices"

// Reads records examined byte ends, including failed lexer/scanner attempts.
// Entries are indexed by scan origin. Seal turns them into prefix maxima so
// a subtree can retain an upper bound in bytes beyond its own end. Including
// earlier and boundary probes is conservative: no dependency can disappear in a projection
// that hides a token or collapses a production.
type Reads struct {
	ends             []read
	sourceBytes      uint32
	sealed           bool
	valid            bool
	allocated        *int64
	budget, baseline int64
}

type read struct{ start, end uint32 }

func NewReads(sourceBytes int) *Reads {
	if sourceBytes < 0 || uint64(sourceBytes) >= uint64(^uint32(0)) {
		return nil
	}
	r := &Reads{}
	r.Reset(sourceBytes)
	return r
}

// Reset starts an independent parse after the owning arena is released.
// The integer-only backing array can be retained under the arena's cap.
func (r *Reads) Reset(sourceBytes int) bool {
	r.ends = r.ends[:0]
	r.sourceBytes = 0
	r.sealed = false
	r.valid = sourceBytes >= 0 && uint64(sourceBytes) < uint64(^uint32(0))
	r.allocated = nil
	r.budget, r.baseline = 0, 0
	if r.valid {
		r.sourceBytes = uint32(sourceBytes)
	}
	return r.valid
}

func (r *Reads) Recording() bool { return r != nil && r.valid && !r.sealed }

// TrimCapacity bounds memory retained between parses; it never runs while
// any tree still owns the arena.
func (r *Reads) TrimCapacity(limit int) {
	if r != nil && cap(r.ends) > limit {
		r.ends = nil
	}
}

// Abstain prevents a new ancestor from authenticating incomplete history.
func (r *Reads) Abstain() {
	if r != nil {
		r.valid = false
	}
}

// BindBudget charges each backing-array growth before it allocates. Failure
// makes the history unavailable; the caller retains its conservative route.
func (r *Reads) BindBudget(limit, baseline int64, allocated *int64) {
	r.budget, r.baseline, r.allocated = limit, baseline, allocated
}

func (r *Reads) Bytes() int64 {
	if r == nil {
		return 0
	}
	return 64 + int64(cap(r.ends))*8
}

func (r *Reads) Record(start int, end uint32) {
	if r == nil {
		return
	}
	if !r.valid {
		return
	}
	if r.sealed || start < 0 || uint64(start) > uint64(r.sourceBytes) || uint64(start) > uint64(end) {
		r.valid = false
		return
	}
	if n := len(r.ends); n != 0 && r.ends[n-1].start == uint32(start) {
		r.ends[n-1].end = max(r.ends[n-1].end, end)
		return
	}
	if len(r.ends) == cap(r.ends) {
		capacity := max(128, cap(r.ends)*2)
		cost := int64(capacity-cap(r.ends)) * 8
		if r.allocated != nil {
			used := max(int64(0), *r.allocated-r.baseline)
			if r.budget > 0 && (used >= r.budget || cost > r.budget-used) {
				r.valid = false
				return
			}
			*r.allocated += cost
		}
		next := make([]read, len(r.ends), capacity)
		copy(next, r.ends)
		r.ends = next
	}
	r.ends = append(r.ends, read{uint32(start), end})
}

func (r *Reads) Seal() {
	if r == nil || r.sealed {
		return
	}
	slices.SortFunc(r.ends, func(a, b read) int {
		if a.start < b.start {
			return -1
		}
		if a.start > b.start {
			return 1
		}
		return 0
	})
	var end uint32
	count := 0
	for _, entry := range r.ends {
		end = max(end, entry.end)
		if count != 0 && r.ends[count-1].start == entry.start {
			r.ends[count-1].end = end
		} else {
			r.ends[count] = read{entry.start, end}
			count++
		}
	}
	r.ends = r.ends[:count]
	r.sealed = true
	r.allocated = nil
}

func (r *Reads) Lookahead(end uint32) (uint32, bool) {
	if r == nil || !r.valid || !r.sealed || end == 0 || end > r.sourceBytes {
		return 0, false
	}
	i, _ := slices.BinarySearchFunc(r.ends, end, func(entry read, end uint32) int {
		if entry.start <= end {
			return -1
		}
		return 1
	})
	if i == 0 {
		return 0, false
	}
	frontier := r.ends[i-1].end
	if frontier < end {
		return 0, false
	}
	return frontier - end, true
}

// Encode distinguishes an authenticated zero lookahead from unknown metadata.
func Encode(bytes uint32) uint32 {
	if bytes == ^uint32(0) {
		return 0
	}
	return bytes + 1
}

func Decode(encoded uint32) (uint32, bool) { return encoded - 1, encoded != 0 }

// FirstLeaf implements ts_parser__can_reuse_first_leaf. The adapter supplies
// the complete Go lex-mode equality, including its whitespace DFA mode.
func FirstLeaf(noLookahead, hasActions, sameLexMode, keywordCapture, keyword, sameParseState, emptyNonEOF, externalMode, reusable bool) bool {
	if noLookahead {
		return false
	}
	if hasActions && sameLexMode && (!keywordCapture || (!keyword && sameParseState)) {
		return true
	}
	return !emptyNonEOF && !externalMode && reusable
}
