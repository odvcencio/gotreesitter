package parsercorephase0

// CanonicalHeadMask proves that a frontier has distinct physical heads when
// each head sets a different bit. Distinct heads imply distinct complete
// canonical keys. A collision keeps the ordinary exact grouping path.
// This local proof never authenticates a boundary or a lexer owner.
type CanonicalHeadMask uint64

// Add records one nonzero head. True proves that its low six bits differ
// from every recorded head; false leaves the mask unchanged. Callers stop
// relying on the proof after a false result.
func (s *CanonicalHeadMask) Add(head Head) bool {
	if head.Node == 0 {
		return false
	}
	bit := uint64(1) << (head.Node & 63)
	if uint64(*s)&bit != 0 {
		return false
	}
	*s |= CanonicalHeadMask(bit)
	return true
}
