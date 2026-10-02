package incr

// SubtreeErrorRank takes the maximum error rank over a read-only subtree.
// inspect returns the node's rank and its dense children. An adapter can
// return an already computed rank with no children for other representations.
// Rank 2 ends the walk, just as it ends parser result selection's error scan.
func SubtreeErrorRank[N comparable](root N, inspect func(N) (int, []N)) int {
	var zero N
	if root == zero {
		return 0
	}
	rank, children := inspect(root)
	if rank == 2 {
		return rank
	}
	for _, child := range children {
		if childRank := SubtreeErrorRank(child, inspect); childRank > rank {
			rank = childRank
		}
		if rank == 2 {
			break
		}
	}
	return rank
}
