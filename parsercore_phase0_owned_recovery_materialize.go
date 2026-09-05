//go:build !gts_no_parsercorephase0

package gotreesitter

import "errors"

// buildCompactOwnedRecoveryAcceptedTree applies C's acceptance splice to one
// grammar root and its stack extras. Recovery turns authenticate the stack.
// Reject other topologies instead of manufacturing a grammar root.
func buildCompactOwnedRecoveryAcceptedTree(
	parser *Parser,
	nodes []*Node,
	source []byte,
	arena *nodeArena,
	linkScratch *[]*Node,
) (*Tree, error) {
	if parser == nil || parser.language == nil || arena == nil || len(nodes) == 0 {
		return nil, errors.New("parser-core phase zero: native recovery acceptance has incomplete input")
	}
	rootIndex := -1
	var previousEnd uint32
	for index, node := range nodes {
		if node == nil || node.ownerArena != arena || node.startByte > node.endByte ||
			(index > 0 && node.startByte < previousEnd) {
			return nil, errors.New("parser-core phase zero: native recovery acceptance has invalid payload ownership or order")
		}
		previousEnd = node.endByte
		if !node.IsExtra() {
			if rootIndex >= 0 {
				return nil, errors.New("parser-core phase zero: native recovery acceptance requires one grammar root")
			}
			rootIndex = index
		}
	}
	if rootIndex < 0 || !parser.hasRootSymbol || nodes[rootIndex].Symbol() != parser.rootSymbol {
		return nil, errors.New("parser-core phase zero: native recovery acceptance has no exact grammar root")
	}
	root := nodes[rootIndex]
	// Materialization has already projected hidden grammar children. Preserve
	// their field metadata while surrounding them with accepted stack extras.
	var leading, trailing []*Node
	for index, node := range nodes {
		if index == rootIndex {
			continue
		}
		if node.Symbol() != errorSymbol && !symbolIsVisible(parser.language, node.Symbol()) {
			if node.ChildCount() != 0 {
				return nil, errors.New("parser-core phase zero: native recovery acceptance cannot project a hidden stack extra")
			}
			continue
		}
		if index < rootIndex {
			leading = append(leading, node)
		} else {
			trailing = append(trailing, node)
		}
	}
	spliceCompactAcceptedRootExtras(root, leading, trailing, arena)
	extendResultRootRangeToExtras(root, nodes)
	if resultNodesHaveError(nodes) {
		root.setHasError(true)
	}
	builder := newResultRootBuild(parser, source, arena, nil, nil, linkScratch)
	return builder.finishNativeAcceptedTree(root, builder.shouldWireParentLinks), nil
}

// Preserve stack order even when a zero-width extra shares the root's start.
func spliceCompactAcceptedRootExtras(root *Node, leading, trailing []*Node, arena *nodeArena) {
	if len(leading)+len(trailing) == 0 || resultMutableChildrenForMutation(root).SurroundFinalRefs(leading, trailing) {
		return
	}
	children := resultChildSliceForMutation(root)
	merged := arena.allocNodeSlice(len(leading) + len(children) + len(trailing))
	copy(merged, leading)
	copy(merged[len(leading):], children)
	copy(merged[len(leading)+len(children):], trailing)
	root.children = merged
	fields, sources := root.fieldIDs(), root.fieldSources()
	if len(fields) != 0 {
		padded := arena.allocFieldIDSlice(len(merged))
		copy(padded[len(leading):], fields)
		var paddedSources []uint8
		if len(sources) != 0 {
			paddedSources = make([]uint8, len(merged))
			copy(paddedSources[len(leading):], sources)
		}
		root.setFieldMetadata(padded, paddedSources)
	}
}
