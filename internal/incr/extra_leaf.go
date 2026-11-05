package incr

// ExtraLeaf authenticates a stored extra leaf that the parser may push again.
// A reduction pushes the trailing extras of its window again with the goto
// state and stores that state in each leaf, so the stored parse state is not
// the state of the leaf's shift. The state that was live before the first push
// is not rewritten. When it is the live state now, the same token and the lone
// extra shift in the table give the same push the parser made before.
func ExtraLeaf(extraLeaf, sameToken, sameStateBeforePush, loneExtraShift bool) bool {
	return extraLeaf && sameToken && sameStateBeforePush && loneExtraShift
}
