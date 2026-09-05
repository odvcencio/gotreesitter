//go:build !gts_no_parsercorephase0

package gotreesitter

import (
	"sort"
	"unsafe"

	core "github.com/odvcencio/gotreesitter/internal/parsercorephase0"
)

func compactIncrementalMaterializationDecline(detail string) error {
	return &diagnosticParserCoreDecline{boundary: DiagnosticParserCoreAccept, detail: "compact incremental materialization: " + detail}
}

// materializeBorrowed authenticates a public subtree without changing its metadata.
func (session *compactIncrementalReuseSession) materializeBorrowed(
	parser *Parser,
	id core.SubtreeID,
	view core.MaterializationSubtreeView,
	states *compactReplayStates,
	points *diagnosticParserCorePointIndex,
) (*Node, error) {
	if parser == nil || parser.language == nil || points == nil ||
		session == nil || session.oldTree == nil || session.oldTree.root == nil || session.oldTree.language != parser.language ||
		view.ReusedKey == 0 || uint64(view.ReusedKey) > uint64(len(session.nodes)) {
		return nil, compactIncrementalMaterializationDecline("borrowed subtree has no live ownership session")
	}
	node := session.nodes[view.ReusedKey-1]
	if node == nil || node.ownerArena == nil || node.dirty() || node.hasError() || node.isMissing() || node.isFragile() ||
		node.isExtra() || !compactNodeMayBeReused(node) || nodeChildCountNoMaterialize(node) == 0 ||
		!parser.isVisibleSymbol(node.symbol) || uint32(node.symbol) < parser.language.TokenCount {
		return nil, compactIncrementalMaterializationDecline("borrowed subtree is not a clean visible nonterminal")
	}
	owned := node.ownerArena == session.oldTree.arena
	for _, arena := range session.oldTree.borrowedArena {
		owned = owned || node.ownerArena == arena
	}
	if !owned {
		return nil, compactIncrementalMaterializationDecline("borrowed subtree arena does not belong to the old tree")
	}
	if view.Terminal || view.Extra || view.External || view.Missing || view.Fragile ||
		len(view.Children) != 0 || len(view.Aliases) != 0 ||
		Symbol(view.Symbol) != node.symbol || view.StartByte != node.startByte || view.EndByte != node.endByte ||
		int32(view.DynamicPrecedence) != node.dynamicPrecedence ||
		node.startPoint != points.point(view.StartByte) || node.endPoint != points.point(view.EndByte) ||
		StateID(view.ReusedPreGotoState) != node.preGotoState || StateID(view.ReusedState) != node.parseState {
		return nil, compactIncrementalMaterializationDecline("borrowed subtree descriptor does not match the public node")
	}
	pre, state, preKnown, stateKnown := states.get(id)
	if !preKnown || !stateKnown || pre != node.preGotoState || state != node.parseState {
		return nil, compactIncrementalMaterializationDecline("borrowed subtree states do not match the accepted derivation")
	}
	return node, nil
}

// appendBorrowed records an authenticated subtree span without classifying it as a terminal.
func (scratch *diagnosticParserCoreAcceptedLeafCoverageScratch) appendBorrowed(id core.SubtreeID, node *Node, sourceLen uint32) error {
	if scratch == nil {
		return nil
	}
	if node == nil || node.startByte >= node.endByte || node.endByte > sourceLen {
		return compactIncrementalMaterializationDecline("borrowed coverage span is invalid")
	}
	if count := len(scratch.spans); count > 0 && node.startByte < scratch.spans[count-1].endByte {
		return compactIncrementalMaterializationDecline("borrowed coverage overlaps an accepted span")
	}
	scratch.spans = append(scratch.spans, diagnosticParserCoreAcceptedLeafSpan{
		id: id, startByte: node.startByte, endByte: node.endByte, borrowed: node,
	})
	return nil
}

func (scratch *diagnosticParserCoreAcceptedLeafCoverageScratch) hasBorrowedSubtree(node *Node) bool {
	if scratch == nil || node == nil {
		return false
	}
	index := sort.Search(len(scratch.spans), func(index int) bool {
		return scratch.spans[index].startByte >= node.startByte
	})
	for index < len(scratch.spans) && scratch.spans[index].startByte == node.startByte {
		span := scratch.spans[index]
		if span.borrowed == node && span.endByte == node.endByte {
			return true
		}
		index++
	}
	return false
}

// visitCompactBorrowedProjection follows hidden wrappers to the visible children that a reduction can change.
func visitCompactBorrowedProjection(parser *Parser, node *Node, arena *nodeArena, visit func(*Node) error) error {
	if node == nil {
		return nil
	}
	if node.ownerArena != arena {
		return visit(node)
	}
	if parser.isVisibleSymbol(node.symbol) {
		return nil
	}
	for _, child := range node.children {
		if err := visitCompactBorrowedProjection(parser, child, arena, visit); err != nil {
			return err
		}
	}
	return nil
}

func validateCompactBorrowedReduceInputs(parser *Parser, entries []stackEntry, productionID uint16, arena *nodeArena) error {
	aliases := parser.reduceAliasSequence(productionID)
	structuralChild := 0
	for _, entry := range entries {
		node := stackEntryNode(entry)
		if node == nil || node.isExtra() {
			continue
		}
		alias := Symbol(0)
		if structuralChild < len(aliases) {
			alias = aliases[structuralChild]
		}
		structuralChild++
		if alias == 0 {
			continue
		}
		if err := visitCompactBorrowedProjection(parser, node, arena, func(*Node) error {
			return compactIncrementalMaterializationDecline("an alias would change a borrowed subtree")
		}); err != nil {
			return err
		}
	}
	return nil
}

type compactBorrowedProjectionCount struct {
	input, output uint64
}

type compactBorrowedProjectionFrame struct {
	node  *Node
	count compactBorrowedProjectionCount
}

type compactBorrowedProjectionScratch struct {
	roots        map[*Node]compactBorrowedProjectionCount
	borrowed     map[*Node]compactBorrowedProjectionCount
	walk         []compactBorrowedProjectionFrame
	rootsPeak    uint64
	borrowedPeak uint64
}

func (scratch *compactBorrowedProjectionScratch) reset() {
	clear(scratch.roots)
	clear(scratch.borrowed)
	clear(scratch.walk[:cap(scratch.walk)])
	scratch.walk = scratch.walk[:0]
}

func (scratch *compactBorrowedProjectionScratch) footprintBytes() uint64 {
	if scratch == nil {
		return 0
	}
	// Count map storage conservatively, including growth storage and bucket overhead.
	bytes := 96*(scratch.rootsPeak+scratch.borrowedPeak) + uint64(cap(scratch.walk))*uint64(unsafe.Sizeof(compactBorrowedProjectionFrame{}))
	if scratch.roots != nil {
		bytes += 256
	}
	if scratch.borrowed != nil {
		bytes += 256
	}
	return bytes
}

func validateCompactBorrowedReduceProjection(parser *Parser, entries []stackEntry, children []*Node, arena *nodeArena) error {
	var scratch compactBorrowedProjectionScratch
	return validateCompactBorrowedReduceProjectionWithScratch(parser, entries, children, arena, &scratch, nil)
}

// validateCompactBorrowedReduceProjectionWithScratch compares borrowed identities once on each side.
// Equal hidden roots preserve an already validated projection and need no descendant walk.
func validateCompactBorrowedReduceProjectionWithScratch(
	parser *Parser,
	entries []stackEntry,
	children []*Node,
	arena *nodeArena,
	scratch *compactBorrowedProjectionScratch,
	poll func() error,
) error {
	defer scratch.reset()
	steps := uint32(0)
	check := func() error {
		steps++
		if poll != nil && steps&255 == 0 {
			return poll()
		}
		return nil
	}
	if poll != nil {
		if err := poll(); err != nil {
			return err
		}
	}
	width := max(4, len(entries)+len(children))
	if scratch.rootsPeak > 64 && scratch.rootsPeak > uint64(width)*4 {
		scratch.roots, scratch.rootsPeak = nil, 0
	}
	if scratch.borrowedPeak > 64 && scratch.borrowedPeak > uint64(width)*4 {
		scratch.borrowed, scratch.borrowedPeak = nil, 0
	}
	if scratch.roots == nil {
		scratch.roots = make(map[*Node]compactBorrowedProjectionCount, width)
		scratch.rootsPeak = uint64(width)
	}
	record := func(node *Node, input bool) {
		if node == nil || (node.ownerArena == arena && parser.isVisibleSymbol(node.symbol)) {
			return
		}
		count := scratch.roots[node]
		if input {
			count.input++
		} else {
			count.output++
		}
		scratch.roots[node] = count
		scratch.rootsPeak = max(scratch.rootsPeak, uint64(len(scratch.roots)))
	}
	for _, entry := range entries {
		if err := check(); err != nil {
			return err
		}
		record(stackEntryNode(entry), true)
	}
	for _, child := range children {
		if err := check(); err != nil {
			return err
		}
		record(child, false)
	}
	for node, count := range scratch.roots {
		if err := check(); err != nil {
			return err
		}
		if count.input == 1 && count.output == 1 {
			continue
		}
		scratch.walk = append(scratch.walk, compactBorrowedProjectionFrame{node: node, count: count})
	}
	for len(scratch.walk) != 0 {
		if err := check(); err != nil {
			return err
		}
		last := len(scratch.walk) - 1
		frame := scratch.walk[last]
		scratch.walk[last] = compactBorrowedProjectionFrame{}
		scratch.walk = scratch.walk[:last]
		node := frame.node
		if node == nil {
			continue
		}
		if node.ownerArena != arena {
			if scratch.borrowed == nil {
				scratch.borrowed = make(map[*Node]compactBorrowedProjectionCount)
			}
			count := scratch.borrowed[node]
			count.input += frame.count.input
			count.output += frame.count.output
			scratch.borrowed[node] = count
			scratch.borrowedPeak = max(scratch.borrowedPeak, uint64(len(scratch.borrowed)))
			continue
		}
		if parser.isVisibleSymbol(node.symbol) {
			continue
		}
		for _, child := range node.children {
			if err := check(); err != nil {
				return err
			}
			scratch.walk = append(scratch.walk, compactBorrowedProjectionFrame{node: child, count: frame.count})
		}
	}
	for _, count := range scratch.borrowed {
		if err := check(); err != nil {
			return err
		}
		if count.input != 1 || count.output != 1 {
			return compactIncrementalMaterializationDecline("a reduction changed a borrowed subtree projection")
		}
	}
	if poll != nil {
		return poll()
	}
	return nil
}
