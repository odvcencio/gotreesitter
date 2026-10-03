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
	forestAttributes bool
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
	r.forestAttributes = false
	r.allocated = nil
	r.budget, r.baseline = 0, 0
	if r.valid {
		r.sourceBytes = uint32(sourceBytes)
	}
	return r.valid
}

func (r *Reads) Recording() bool { return r != nil && r.valid && !r.sealed }

func (r *Reads) SourceBytes() uint32 {
	if r == nil {
		return 0
	}
	return r.sourceBytes
}

// CertifyForestAttributes requires a producer that preserves
// native leaf states, keyword flags, and reduction fragility. Unsupported pop paths or an
// unsupported scan invalidates the complete receipt through Abstain.
func (r *Reads) CertifyForestAttributes() {
	if r.Recording() {
		r.forestAttributes = true
	}
}

func (r *Reads) CertifiedForestAttributes() bool {
	return r != nil && r.valid && r.sealed && r.forestAttributes
}

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
	return r.lookahead(end, true)
}

// LeafLookahead excludes probes starting at the token's end. Those probes
// select the next token and may decide a parent reduction, but cannot change
// the leaf that has already been shifted. Earlier failed probes still count.
func (r *Reads) LeafLookahead(end uint32) (uint32, bool) {
	return r.lookahead(end, false)
}

func (r *Reads) lookahead(end uint32, includeBoundary bool) (uint32, bool) {
	if r == nil || !r.valid || !r.sealed || end == 0 || end > r.sourceBytes {
		return 0, false
	}
	i, _ := slices.BinarySearchFunc(r.ends, end, func(entry read, end uint32) int {
		if entry.start < end || (includeBoundary && entry.start == end) {
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

// LookaheadCursor scans a sealed history in node-allocation order. Increasing
// ends advance once through the history; a backwards end uses the same binary
// search as an individual lookup. The caller must create a new cursor after Reset.
type LookaheadCursor struct {
	reads           *Reads
	includeBoundary bool
	index           int
	end             uint32
}

func (r *Reads) Cursor(includeBoundary bool) LookaheadCursor {
	return LookaheadCursor{reads: r, includeBoundary: includeBoundary}
}

func (c *LookaheadCursor) Lookahead(end uint32) (uint32, bool) {
	r := c.reads
	if r == nil || !r.valid || !r.sealed || end == 0 || end > r.sourceBytes {
		return 0, false
	}
	if end < c.end {
		c.index, _ = slices.BinarySearchFunc(r.ends, end, func(entry read, end uint32) int {
			if entry.start < end || (c.includeBoundary && entry.start == end) {
				return -1
			}
			return 1
		})
	} else {
		for c.index < len(r.ends) {
			start := r.ends[c.index].start
			if start > end || (start == end && !c.includeBoundary) {
				break
			}
			c.index++
		}
	}
	c.end = end
	if c.index == 0 || r.ends[c.index-1].end < end {
		return 0, false
	}
	return r.ends[c.index-1].end - end, true
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

// FreshLeaf authenticates a terminal already lexed for this dispatch. A single
// shift leaves no skipped reduction or conflict arm to reconstruct.
func FreshLeaf(singleShift, sameToken, sameExtra, clean, nonempty bool) bool {
	return singleShift && sameToken && sameExtra && clean && nonempty
}

// RecoveryShape rejects ERROR roots and overlapping recovery regions. Reusing
// an ordinary derivation cannot certify hidden children inside those regions.
func RecoveryShape[N comparable](root N, clean, isError func(N) bool, childCount func(N) int, childAt func(N, int) N) bool {
	if clean(root) {
		return true
	}
	if isError(root) {
		return false
	}
	type entry struct {
		node       N
		underError bool
	}
	stack := []entry{{node: root}}
	for len(stack) > 0 {
		e := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		if clean(e.node) {
			continue
		}
		err := isError(e.node)
		if err && e.underError {
			return false
		}
		for i := 0; i < childCount(e.node); i++ {
			child := childAt(e.node, i)
			if !clean(child) {
				stack = append(stack, entry{child, e.underError || err})
			}
		}
	}
	return true
}

// RecoveryEditChangesTerminal distinguishes an actual lexical change from a
// same-terminal edit that merely perturbs recovery. The old terminal's span
// already includes the edit. An insertion can produce a separate terminal
// even when its symbol equals the surviving terminal's symbol. The caller must
// independently certify lexer reads, scanner state, and parser policy.
func RecoveryEditChangesTerminal(oldStart, newStart, newEnd, editStart, editOldEnd, editNewEnd uint32, sameSymbol bool) bool {
	return !sameSymbol || (editStart == editOldEnd && editNewEnd > editStart &&
		newStart == editStart && newEnd == editNewEnd && oldStart >= editNewEnd)
}
