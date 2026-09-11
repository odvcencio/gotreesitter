package gotreesitter

// gssRecoveryCostsEqual requires one cumulative cost across every graph path.
// The bounded walk rejects cycles and large proofs without allocating storage.
func gssRecoveryCostsEqual(scratch *glrMergeScratch, preflight *gssMainPreflight, a, b *gssNode) bool {
	if scratch == nil || scratch.parser == nil || !scratch.parser.errorCostCompetitionEnabled() {
		return false
	}
	remaining := 4096
	left, leftOK := gssUniformRecoveryCost(scratch, preflight, a, 0, &remaining)
	right, rightOK := gssUniformRecoveryCost(scratch, preflight, b, 0, &remaining)
	return leftOK && rightOK && left == right
}

func gssUniformRecoveryCost(scratch *glrMergeScratch, preflight *gssMainPreflight, node *gssNode, depth int, remaining *int) (uint32, bool) {
	if node == nil {
		return 0, true
	}
	if depth >= 128 || *remaining <= 0 {
		return 0, false
	}
	*remaining--
	count := node.linkCount()
	if preflight != nil {
		count = preflight.linkCount(node)
	}
	var cost uint32
	for i := 0; i < count; i++ {
		if *remaining <= 0 {
			return 0, false
		}
		*remaining--
		var prev *gssNode
		var entry stackEntry
		if preflight != nil {
			prev, entry = preflight.linkAt(node, i)
		} else {
			prev, entry = node.link(i)
		}
		prefix, ok := gssUniformRecoveryCost(scratch, preflight, prev, depth+1, remaining)
		if !ok {
			return 0, false
		}
		var own uint32
		if stackEntryHasNode(entry) {
			materialized := stackEntryNode(entry)
			if materialized == nil {
				return 0, false
			}
			if materialized.isMissing() && len(materialized.children) == 0 {
				own = cErrCostPerMissingTree + cErrCostPerRecovery
			} else if materialized.hasError() || materialized.symbol == errorSymbol {
				if len(scratch.parser.cNodeMemoCache) == 0 {
					return 0, false
				}
				cached := scratch.parser.cNodeMemoPrimaryHit(materialized)
				if cached == nil || !cached.hasCost || cached.ver != materialized.equivVersion {
					return 0, false
				}
				own = cached.cost
			}
		}
		if own > ^uint32(0)-prefix {
			return 0, false
		}
		current := prefix + own
		if i > 0 && current != cost {
			return 0, false
		}
		cost = current
	}
	return cost, true
}

func gssRecoveryStacksCanMerge(scratch *glrMergeScratch, a, b *glrStack) bool {
	return a.top().state != 0 && b.top().state != 0 && !a.accepted && !b.accepted && !a.cPaused && !b.cPaused && a.cRec == nil && b.cRec == nil &&
		a.cRecoverMissingGroup == b.cRecoverMissingGroup && a.cRecoveryUnvalidatedMarker == b.cRecoveryUnvalidatedMarker &&
		gssRecoveryCostsEqual(scratch, nil, a.gss.head, b.gss.head)
}
