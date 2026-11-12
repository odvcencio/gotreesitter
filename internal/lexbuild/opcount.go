package lexbuild

import "sync/atomic"

// Operation counters. Each counts the work of one function that an earlier
// version did in quadratic time: the elements it moved, copied or scanned. The
// counters are compiled in always. A function adds to its counter once, after
// its loops (addIntRange and stepAt once for each call), so the cost is one
// atomic addition for each call. Tests reset a counter, run a construction and
// compare the count with a bound that a linear version meets and a quadratic
// version misses by a wide factor. Nothing in the package reads a counter.
type opCounter struct{ n atomic.Int64 }

func (c *opCounter) add(n int) { c.n.Add(int64(n)) }

var ops struct {
	flagBytes   opCounter // filterPatternFlags: bytes read and bytes written
	rangeSteps  opCounter // addIntRange: loop passes and ranges moved by an insertion
	mergeSteps  opCounter // removeIntersectionSteps: loop passes and ranges copied
	addSteps    opCounter // CharSet.add: ranges of the tail and of the added set that the merge reads
	unionRanges opCounter // unionOf: ranges collected, plus n log n for the sort
	concatRunes opCounter // hirConcatOf: runes copied into a joined literal, and nodes copied into the flat list
	classSteps  opCounter // classDifference: ranges of the right operand passed or compared
	groupRunes  opCounter // parseGroup: runes inspected to classify a group
	errBytes    opCounter // pattern errors: bytes of the pattern quoted into a message
	transEvals  opCounter // stepAt: transitions evaluated
	condProbes  opCounter // rangesContain: probes of the binary search
}

// resetOps sets every counter to zero (for tests).
func resetOps() {
	for _, c := range []*opCounter{&ops.flagBytes, &ops.rangeSteps, &ops.mergeSteps, &ops.addSteps, &ops.unionRanges, &ops.concatRunes,
		&ops.classSteps, &ops.groupRunes, &ops.errBytes, &ops.transEvals, &ops.condProbes} {
		c.n.Store(0)
	}
}
