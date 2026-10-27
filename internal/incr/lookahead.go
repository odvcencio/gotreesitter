// Package incr implements structural incremental-reuse conditions.
package incr

import (
	"math/bits"
	"sort"
	"unsafe"
)

// Reads records examined byte ends, including failed lexer/scanner attempts.
// Entries are indexed by scan origin. Seal turns them into prefix maxima so
// a subtree can retain an upper bound in bytes beyond its own end. Including
// earlier and boundary probes is conservative: no dependency can disappear in a projection
// that hides a token or collapses a production.
type Reads struct {
	ends                    readWords
	allocated               *int64
	budget, baseline        int64
	sourceBytes             uint32
	packedBits              uint8
	flags                   uint8
	defaultProducerSchedule uint16
}

const (
	readsSealed uint8 = 1 << iota
	readsValid
	readsForestAttributes
	readsCommittedDropped
	readsDisordered
)

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
	r.ends.reset()
	r.sourceBytes = 0
	r.flags = 0
	r.defaultProducerSchedule = 0
	r.allocated = nil
	r.budget, r.baseline = 0, 0
	r.packedBits = 0
	if sourceBytes >= 0 && uint64(sourceBytes) < uint64(^uint32(0)) {
		r.flags = readsValid
		r.sourceBytes = uint32(sourceBytes)
		r.packedBits = uint8(min(31, 32-bits.Len32(r.sourceBytes)))
	}
	return r.flags&readsValid != 0
}

func (r *Reads) Recording() bool {
	return r != nil && r.flags&readsValid != 0 && r.flags&readsSealed == 0
}

// ValidForSource includes sealed histories. Sealing closes a completed
// producer's history; abstention still makes it unavailable.
func (r *Reads) ValidForSource(sourceBytes int) bool {
	return r != nil && r.flags&readsValid != 0 && r.flags&readsCommittedDropped == 0 && sourceBytes >= 0 && uint64(sourceBytes) == uint64(r.sourceBytes)
}

// CertifyDefaultProducer records the parser-owned producer context. Invalid or
// incomplete read history can never authorize the schedule.
func (r *Reads) CertifyDefaultProducer(schedule uint16) {
	if r == nil {
		return
	}
	r.defaultProducerSchedule = 0
	if schedule != 0 && r.ValidForSource(int(r.sourceBytes)) {
		r.defaultProducerSchedule = schedule
	}
}

func (r *Reads) DefaultProducerSchedule(sourceBytes int) uint16 {
	if !r.ValidForSource(sourceBytes) {
		return 0
	}
	return r.defaultProducerSchedule
}

func (r *Reads) invalidate() {
	r.flags &^= readsValid
	r.defaultProducerSchedule = 0
}

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
		r.flags |= readsForestAttributes
	}
}

func (r *Reads) CertifiedForestAttributes() bool {
	return r != nil && r.flags&(readsValid|readsCommittedDropped|readsSealed|readsForestAttributes) == readsValid|readsSealed|readsForestAttributes
}

// TrimCapacity bounds retained storage to the bytes of limit wide records.
// It never runs while any tree still owns the arena.
func (r *Reads) TrimCapacity(limit int) {
	if r != nil && r.ends.bytes() > int64(limit)*8 {
		r.ends = readWords{}
		r.defaultProducerSchedule = 0
	}
}

// Abstain prevents a new ancestor from authenticating incomplete history.
func (r *Reads) Abstain() {
	if r != nil {
		r.invalidate()
	}
}

// CommitDroppedInput declines a selected token or EOF that omitted bytes.
// Discarded lexer probes still contribute read bounds without changing this
// decision; a later lex mode can legitimately recover their input.
func (r *Reads) CommitDroppedInput(dropped bool) {
	if r != nil && dropped {
		// Preserve ordinary read metadata; it still serves the legacy frontier.
		// The completed history cannot certify a result that omitted input.
		r.flags |= readsCommittedDropped
		r.defaultProducerSchedule = 0
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
	return ReadsHeaderBytes() + r.ends.bytes()
}

// Records use one word when the source origin and probe width fit together.
// Wider probes promote the same history to two words per record. The origin
// consumes exactly the bits needed by this source; the rest encode its width.
// Both forms retain the exact prefix bounds, including failed and EOF probes.
func (r *Reads) wordCount() int {
	if r.packedBits == 0 {
		return 2
	}
	return 1
}
func (r *Reads) count() int {
	if r.packedBits == 0 {
		return r.ends.len() / 2
	}
	return r.ends.len()
}
func (r *Reads) at(i int) read {
	if r.packedBits == 0 {
		return read{r.ends.at(2 * i), r.ends.at(2*i + 1)}
	}
	word := r.ends.at(i)
	start := word >> r.packedBits
	return read{start, start + (word & ((uint32(1) << r.packedBits) - 1))}
}
func (r *Reads) put(i int, start, end uint32) {
	if r.packedBits == 0 {
		r.ends.put(2*i, start)
		r.ends.put(2*i+1, end)
		return
	}
	r.ends.put(i, start<<r.packedBits|(end-start))
}

// wideReadSort orders pairs without another allocation or slice cast.
type wideReadSort Reads

func (r *wideReadSort) Len() int           { return r.ends.len() / 2 }
func (r *wideReadSort) Less(i, j int) bool { return r.ends.at(2*i) < r.ends.at(2*j) }
func (r *wideReadSort) Swap(i, j int) {
	a, b := r.ends.at(2*i), r.ends.at(2*j)
	r.ends.put(2*i, b)
	r.ends.put(2*j, a)
	a, b = r.ends.at(2*i+1), r.ends.at(2*j+1)
	r.ends.put(2*i+1, b)
	r.ends.put(2*j+1, a)
}
func (r *Reads) charge(cost int64) bool {
	if r.allocated != nil {
		used := max(int64(0), *r.allocated-r.baseline)
		if r.budget > 0 && (used >= r.budget || cost > r.budget-used) {
			return false
		}
		*r.allocated += cost
	}
	return true
}
func (r *Reads) promoteWide() bool {
	n := r.count()
	if r.ends.capacity() > int(^uint(0)>>1)/2 || !r.ends.reserve(2*r.ends.capacity(), r.charge) {
		r.invalidate()
		return false
	}
	r.ends.resize(2 * n)
	for i := n - 1; i >= 0; i-- {
		e := r.at(i)
		r.ends.put(2*i, e.start)
		r.ends.put(2*i+1, e.end)
	}
	r.packedBits = 0
	return true
}

func (r *Reads) Record(start int, end uint32) {
	if r == nil || r.flags&readsValid == 0 {
		return
	}
	if r.flags&readsSealed != 0 || start < 0 || uint64(start) > uint64(r.sourceBytes) || uint64(start) > uint64(end) {
		r.invalidate()
		return
	}
	origin := uint32(start)
	if r.packedBits != 0 && end-origin >= uint32(1)<<r.packedBits {
		if !r.promoteWide() {
			return
		}
	}
	if n := r.count(); n != 0 {
		last := r.at(n - 1)
		if last.start == origin {
			r.put(n-1, origin, max(last.end, end))
			return
		}
		// Contained later probes cannot increase any prefix maximum.
		if origin > last.start && end <= last.end {
			return
		}
		if origin < last.start {
			for i := n - 2; i >= max(0, n-8); i-- {
				entry := r.at(i)
				if entry.start == origin {
					r.put(i, origin, max(entry.end, end))
					return
				}
			}
		}
	}
	if n := r.count(); n != 0 && origin < r.at(n-1).start {
		r.flags |= readsDisordered
	}
	words := r.wordCount()
	want := r.ends.len() + words
	if want > r.ends.capacity() {
		capacity := max(128*words, r.ends.capacity()*2)
		if r.ends.capacity() >= readWordPageSize {
			capacity = want
		}
		if !r.ends.reserve(capacity, r.charge) {
			r.invalidate()
			return
		}
	}
	index := r.count()
	r.ends.resize(want)
	r.put(index, origin, end)
}
func (r *Reads) Seal() {
	if r == nil || r.flags&readsSealed != 0 {
		return
	}
	if r.flags&readsDisordered != 0 {
		if r.packedBits == 0 {
			sort.Sort((*wideReadSort)(r))
		} else {
			sort.Sort((*packedReadWordSort)(&r.ends))
		}
	}
	var end uint32
	count := 0
	for i, n := 0, r.count(); i < n; i++ {
		entry := r.at(i)
		end = max(end, entry.end)
		if count != 0 && r.at(count-1).start == entry.start {
			r.put(count-1, entry.start, end)
		} else {
			r.put(count, entry.start, end)
			count++
		}
	}
	r.ends.resize(count * r.wordCount())
	r.flags |= readsSealed
	r.allocated = nil
}
func (r *Reads) Lookahead(end uint32) (uint32, bool) { return r.lookahead(end, true) }

// LeafLookahead excludes the next token's boundary probe, while retaining
// failed probes from earlier origins.
func (r *Reads) LeafLookahead(end uint32) (uint32, bool) { return r.lookahead(end, false) }
func (r *Reads) search(end uint32, includeBoundary bool) int {
	low, high := 0, r.count()
	for low < high {
		mid := int(uint(low+high) >> 1)
		start := r.at(mid).start
		if start < end || (includeBoundary && start == end) {
			low = mid + 1
		} else {
			high = mid
		}
	}
	return low
}
func (r *Reads) lookahead(end uint32, includeBoundary bool) (uint32, bool) {
	if r == nil || r.flags&readsValid == 0 || r.flags&readsSealed == 0 || end == 0 || end > r.sourceBytes {
		return 0, false
	}
	i := r.search(end, includeBoundary)
	if i == 0 {
		return 0, false
	}
	frontier := r.at(i - 1).end
	if frontier < end {
		return 0, false
	}
	return frontier - end, true
}

// LookaheadCursor scans sealed history in node-allocation order. A backwards
// end uses the same binary search as an individual lookup. Reset needs a new cursor.
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
	if r == nil || r.flags&readsValid == 0 || r.flags&readsSealed == 0 || end == 0 || end > r.sourceBytes {
		return 0, false
	}
	if end < c.end {
		c.index = r.search(end, c.includeBoundary)
	} else {
		for c.index < r.count() {
			start := r.at(c.index).start
			if start > end || (start == end && !c.includeBoundary) {
				break
			}
			c.index++
		}
	}
	c.end = end
	if c.index == 0 {
		return 0, false
	}
	frontier := r.at(c.index - 1).end
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

// ReadsHeaderBytes charges the full header while preserving the conservative floor.
func ReadsHeaderBytes() int64 { return max(64, int64(unsafe.Sizeof(Reads{}))) }
