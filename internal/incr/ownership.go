package incr

import "sync/atomic"

// Ownership records whether a published arena has only local descendant
// edges. A mixed arena stays mixed until its final owner releases it.
// Parent hints are not descendant edges and do not affect this receipt.
type Ownership struct {
	state atomic.Uint32
}

const (
	ownershipLocal = uint32(1)
	ownershipMixed = uint32(2)
	ownershipFresh = uint32(3)
)

// BeginFresh records a producer that builds its nodes without an old tree.
// It cannot recertify an arena that already published borrowed edges.
func (o *Ownership) BeginFresh() { o.state.CompareAndSwap(0, ownershipFresh) }

// Publish seals the fresh producer only when its complete ownership list has
// no foreign arenas. A later borrowed publication revokes that certificate.
func (o *Ownership) Publish(borrowed bool) {
	if borrowed {
		o.state.Store(ownershipMixed)
	} else {
		o.state.CompareAndSwap(ownershipFresh, ownershipLocal)
	}
}

func (o *Ownership) Local() bool { return o.state.Load() == ownershipLocal }

// Reset runs only after the arena's last owner releases it.
func (o *Ownership) Reset() { o.state.Store(0) }

// VisitOwners walks exactly the reachable ownership edges. visit records each
// node's owner and may prune descendants only when that owner's local receipt
// is complete. Unknown and mixed owners still use the full child walk.
// Popped pointers are cleared before scratch is retained.
func VisitOwners[N comparable](root N, scratch *[]N, visit func(N) bool, children func(N, []N) []N) {
	var zero N
	stack := append((*scratch)[:0], root)
	for len(stack) != 0 {
		last := len(stack) - 1
		node := stack[last]
		stack[last] = zero
		stack = stack[:last]
		if node != zero && !visit(node) {
			stack = children(node, stack)
		}
	}
	*scratch = stack
}
