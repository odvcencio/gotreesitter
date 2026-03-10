package grammargen

import "math/bits"

// bitset is a compact set of non-negative integers backed by a []uint64.
// It supports O(1) add/contains/remove and O(n/64) union/intersect/equal.
type bitset struct {
	words []uint64
}

func newBitset(capacity int) bitset {
	n := (capacity + 63) / 64
	if n == 0 {
		n = 1
	}
	return bitset{words: make([]uint64, n)}
}

func (b *bitset) grow(idx int) {
	need := idx/64 + 1
	if need <= len(b.words) {
		return
	}
	old := b.words
	b.words = make([]uint64, need)
	copy(b.words, old)
}

func (b *bitset) add(idx int) {
	w := idx / 64
	if w >= len(b.words) {
		b.grow(idx)
	}
	b.words[w] |= 1 << uint(idx%64)
}

func (b *bitset) contains(idx int) bool {
	w := idx / 64
	if w >= len(b.words) {
		return false
	}
	return b.words[w]&(1<<uint(idx%64)) != 0
}

func (b *bitset) empty() bool {
	for _, w := range b.words {
		if w != 0 {
			return false
		}
	}
	return true
}

// unionWith sets b = b | other. Returns true if b changed.
func (b *bitset) unionWith(other *bitset) bool {
	if len(other.words) > len(b.words) {
		old := b.words
		b.words = make([]uint64, len(other.words))
		copy(b.words, old)
	}
	changed := false
	for i, w := range other.words {
		before := b.words[i]
		b.words[i] |= w
		if b.words[i] != before {
			changed = true
		}
	}
	return changed
}

func (b *bitset) equal(other *bitset) bool {
	maxLen := len(b.words)
	if len(other.words) > maxLen {
		maxLen = len(other.words)
	}
	for i := 0; i < maxLen; i++ {
		var bw, ow uint64
		if i < len(b.words) {
			bw = b.words[i]
		}
		if i < len(other.words) {
			ow = other.words[i]
		}
		if bw != ow {
			return false
		}
	}
	return true
}

func (b *bitset) clone() bitset {
	c := bitset{words: make([]uint64, len(b.words))}
	copy(c.words, b.words)
	return c
}

// count returns the number of set bits (population count).
func (b *bitset) count() int {
	n := 0
	for _, w := range b.words {
		n += bits.OnesCount64(w)
	}
	return n
}

// forEach calls fn for each set bit index.
func (b *bitset) forEach(fn func(int)) {
	for wi, w := range b.words {
		base := wi * 64
		for w != 0 {
			tz := bits.TrailingZeros64(w)
			fn(base + tz)
			w &= w - 1 // clear lowest set bit
		}
	}
}

// hash computes a 64-bit hash of the bitset contents.
func (b *bitset) hash() uint64 {
	h := uint64(0xcbf29ce484222325) // FNV offset basis
	for _, w := range b.words {
		h ^= w
		h *= 0x100000001b3 // FNV prime
	}
	return h
}
