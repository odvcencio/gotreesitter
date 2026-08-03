package parsercorephase0

import (
	"math"
	"math/bits"
)

// Record-arena reserve densities, expressed as records per 1024 source bytes
// so the estimate stays exact integer arithmetic (no float, no rounding
// drift, same reserve on every run for the same source length).
//
// The numbers come from a census of the four canonical Go admission fixtures
// (5 KB, 20 KB, 41 KB, 236 KB), which span the measured density range of
// real Go source:
//
//	fixture        nodes/B  links/B  subtrees/B  children/B
//	rewrite         0.5878   0.5876      0.5129      0.5409
//	query_compile   0.7333   0.7333      0.6489      0.6820
//	language        0.3655   0.3654      0.3153      0.3472
//	grammargen_lr   0.6356   0.6356      0.5562      0.5979
//
// Each constant sits just above that family's densest fixture, so a source
// of ordinary density needs no growth at all and a denser-than-canonical
// source still starts from a base that removes almost every copy. nodes and
// nodeLineages share one count because appendNodeAt appends to both on every
// node, one for one.
//
// These are a capacity estimate and nothing else. They never bound what the
// parse may publish: Limits alone does that, and every append still checks
// it. A reserve that is too small only costs the growth it used to cost; a
// reserve that is too large only costs capacity the parse would have reached
// anyway.
const (
	reserveNodesPerKiB    = 768 // 0.7500 records per source byte
	reserveLinksPerKiB    = 768 // 0.7500
	reserveSubtreesPerKiB = 680 // 0.6641
	reserveChildrenPerKiB = 730 // 0.7129
)

// ReserveRecordArenas reserves record-arena capacity for one parse over a
// source of sourceLen bytes, so the five arenas that carry the compact
// core's record mass -- nodes, nodeLineages, links, subtrees, children --
// start at their expected size instead of growing there from zero.
//
// Why this exists: Go grows a large slice by about 1.25x, so an arena that
// ends at S bytes allocates roughly 4.5-5.3x S cumulatively and copies about
// 4x S through growslice. On a first parse (a fresh core, empty arenas --
// the one-shot pattern every caller that parses a file once and exits uses)
// those five growth series are the dominant allocation cost of the whole
// parse. Reserving the capacity up front replaces the series with one
// allocation per arena.
//
// maxBytes caps the total reserve. When the source-proportional estimate
// exceeds it, every arena is scaled down by the same rational factor, so the
// reserve keeps its measured shape and the sum lands at or under maxBytes.
// The caller sets maxBytes from its own memory budget, because
// FootprintBytes gauges capacity: a reserve raises that gauge immediately,
// at construction, before the parse publishes a single record.
//
// ReserveRecordArenas is a pure capacity operation. It changes no length, no
// record, no identifier, and no Work counter, so it cannot change a parse
// result. It does nothing at all unless every arena it touches is empty
// (call it directly after New or Reset and before the seed), and it only
// ever grows: an arena that already holds enough retained capacity from an
// earlier parse on the same core keeps it.
func (c *Core) ReserveRecordArenas(sourceLen int, maxBytes uint64) {
	if c == nil || sourceLen <= 0 {
		return
	}
	if len(c.nodes) != 0 || len(c.nodeLineages) != 0 || len(c.links) != 0 ||
		len(c.subtrees) != 0 || len(c.children) != 0 {
		return
	}
	nodes := reserveRecordCount(sourceLen, reserveNodesPerKiB, c.limits.MaxNodes)
	links := reserveRecordCount(sourceLen, reserveLinksPerKiB, c.limits.MaxLinks)
	subtrees := reserveRecordCount(sourceLen, reserveSubtreesPerKiB, c.limits.MaxSubtrees)
	children := reserveRecordCount(sourceLen, reserveChildrenPerKiB, c.limits.MaxChildren)
	total := reserveTotalBytes(nodes, links, subtrees, children)
	if total == 0 {
		return
	}
	if maxBytes > 0 && total > maxBytes {
		nodes = scaleReserveCount(nodes, maxBytes, total)
		links = scaleReserveCount(links, maxBytes, total)
		subtrees = scaleReserveCount(subtrees, maxBytes, total)
		children = scaleReserveCount(children, maxBytes, total)
	}
	c.nodes = reserveArena(c.nodes, nodes)
	c.nodeLineages = reserveArena(c.nodeLineages, nodes)
	c.links = reserveArena(c.links, links)
	c.subtrees = reserveArena(c.subtrees, subtrees)
	c.children = reserveArena(c.children, children)
}

// ReserveRecordArenaBytes reports the total record-arena capacity, in bytes,
// that ReserveRecordArenas would take ON AN EMPTY CORE at sourceLen and
// maxBytes. It allocates nothing. Callers use it to check a reserve against
// a memory budget before they take it, and tests use it to pin the reserve's
// arithmetic.
//
// The "empty core" qualifier is the whole contract. ReserveRecordArenas does
// nothing at all when any arena already holds a record, and it keeps any
// arena whose capacity is already large enough, so on a warm core this
// function is an upper bound on the real cost rather than the real cost. It
// deliberately reports the estimate itself, not the difference from the
// core's current state, because a budget check wants to know what a fresh
// parse of this source costs.
func (c *Core) ReserveRecordArenaBytes(sourceLen int, maxBytes uint64) uint64 {
	if c == nil || sourceLen <= 0 {
		return 0
	}
	nodes := reserveRecordCount(sourceLen, reserveNodesPerKiB, c.limits.MaxNodes)
	links := reserveRecordCount(sourceLen, reserveLinksPerKiB, c.limits.MaxLinks)
	subtrees := reserveRecordCount(sourceLen, reserveSubtreesPerKiB, c.limits.MaxSubtrees)
	children := reserveRecordCount(sourceLen, reserveChildrenPerKiB, c.limits.MaxChildren)
	total := reserveTotalBytes(nodes, links, subtrees, children)
	if maxBytes > 0 && total > maxBytes {
		return reserveTotalBytes(
			scaleReserveCount(nodes, maxBytes, total),
			scaleReserveCount(links, maxBytes, total),
			scaleReserveCount(subtrees, maxBytes, total),
			scaleReserveCount(children, maxBytes, total),
		)
	}
	return total
}

// reserveTotalBytes is the byte cost of one reserve. nodes counts twice
// because nodes and nodeLineages are reserved to the same count.
func reserveTotalBytes(nodes, links, subtrees, children int) uint64 {
	return uint64(nodes)*coreNodeRecordBytes +
		uint64(nodes)*coreNodeLineageRecordBytes +
		uint64(links)*coreLinkRecordBytes +
		uint64(subtrees)*coreSubtreeRecordBytes +
		uint64(children)*coreChildRecordBytes
}

// reserveRecordCount converts a source length to a record count at the given
// density, then clamps it to this family's configured Limits ceiling. The
// parse can never publish more than the limit, so reserving past it would be
// pure waste.
func reserveRecordCount(sourceLen int, perKiB uint64, limit uint32) int {
	if sourceLen <= 0 || perKiB == 0 || limit == 0 {
		return 0
	}
	count := (uint64(sourceLen)*perKiB + 1023) / 1024
	if count > uint64(limit) {
		count = uint64(limit)
	}
	if count > math.MaxInt32 {
		count = math.MaxInt32
	}
	return int(count)
}

// scaleReserveCount shrinks one arena's count by allowed/total, the same
// factor every arena in the reserve gets.
//
// The product is taken at 128 bits. count is clamped to MaxInt32 and allowed
// is a byte ceiling, so a 64-bit product cannot overflow for any ceiling this
// package ships; computing it wide anyway keeps the result exact for any
// ceiling a future caller passes.
//
// bits.Div64 cannot panic here. It panics only when the quotient would not
// fit in 64 bits, and the guard above establishes allowed < total, so the
// quotient is strictly below count, which is itself at most MaxInt32.
func scaleReserveCount(count int, allowed, total uint64) int {
	if count <= 0 || total == 0 || allowed >= total {
		return count
	}
	hi, lo := bits.Mul64(uint64(count), allowed)
	scaled, _ := bits.Div64(hi, lo, total)
	return int(scaled)
}

// reserveArena returns an empty slice with at least want capacity, keeping
// the caller's existing backing array whenever it is already large enough.
func reserveArena[T any](arena []T, want int) []T {
	if want <= 0 || cap(arena) >= want {
		return arena
	}
	return make([]T, 0, want)
}

// RetentionCapBytesForTest exposes coreRetentionCapBytes so a caller in
// another package can prove its own reserve ceiling stays strictly below the
// size at which this core drops retained capacity.
func RetentionCapBytesForTest() uint64 { return coreRetentionCapBytes }
