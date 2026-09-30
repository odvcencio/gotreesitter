package gotreesitter

import (
	"bytes"
	"unsafe"

	"github.com/odvcencio/gotreesitter/internal/incr"
)

const (
	legacyReuseKeyword   = uint32(1) << 31
	legacyReuseLeafKnown = uint32(1) << 30
	legacyReuseCountMask = legacyReuseLeafKnown - 1
)

// The legacy facade keeps dependency words parallel to its pinned Node layout.
// The scanner and lexer populate Reads during parsing; edits consume immutable
// per-subtree byte counts, without rerunning lexical decisions.
func legacyReuseReadsEligible(d *dfaTokenSource, source []byte) bool {
	if !legacyReuseReadHistoryEligible(d, source) {
		return false
	}
	if d.hasExternalSymbols || d.hasExternalScanner || d.language.ExternalScanner != nil {
		scanner, ok := d.language.ExternalScanner.(StatelessExternalScanner)
		if !d.hasExternalScanner || !ok || !scanner.ExternalScannerIsStateless() {
			return false
		}
	}
	return true
}

func legacyReuseReadHistoryEligible(d *dfaTokenSource, source []byte) bool {
	if d == nil || !d.tokenInvariantInternalPrimitivesSupported() || d.lexer == nil || len(d.lexer.includedRanges) != 0 {
		return false
	}
	// Contextual close-angle probes currently have only aggregate read history.
	// Until they supply individual frontiers, keep their established verifier.
	return !supportsCompactCloseAngleSplit(d.language.Name) || !bytes.Contains(source, []byte(">>"))
}

func (a *nodeArena) beginLegacyReuseReads(d *dfaTokenSource, source []byte) {
	if !legacyReuseReadHistoryEligible(d, source) {
		return
	}
	if a.legacyReuseReads == nil {
		const cost = 64
		if !a.canAllocateLegacyReuseDependency(cost) {
			return
		}
		a.legacyReuseReads = incr.NewReads(len(source))
		if a.legacyReuseReads == nil {
			return
		}
		a.allocatedBytes += a.legacyReuseReads.Bytes()
	} else if !a.legacyReuseReads.Reset(len(source)) {
		return
	}
	a.legacyReuseReads.BindBudget(a.budgetBytes, a.budgetBaselineBytes, &a.allocatedBytes)
	d.lexer.reuseReads = a.legacyReuseReads
}

func (a *nodeArena) canAllocateLegacyReuseDependency(cost int64) bool {
	used := max(int64(0), a.allocatedBytes-a.budgetBaselineBytes)
	return cost >= 0 && (a.budgetBytes <= 0 || (used < a.budgetBytes && cost <= a.budgetBytes-used))
}

func (a *nodeArena) legacyReuseDependencyBytesAllocated() int64 {
	if a == nil {
		return 0
	}
	total := a.legacyReuseReads.Bytes() + int64(cap(a.nodeReuseLookahead))*4
	for i := range a.nodeSlabs {
		total += int64(cap(a.nodeSlabs[i].reuseLookahead)) * 4
	}
	return total
}

func (a *nodeArena) resetLegacyReuseDependencies() {
	if a.legacyReuseReads != nil {
		a.legacyReuseReads.Reset(-1)
		a.legacyReuseReads.TrimCapacity(maxRetainedChildSliceCapacityForClass(a.class))
	}
	a.legacyReuseDependenciesReady = false
	a.legacyReuseSourceChanged = false
	clear(a.nodeReuseLookahead)
	for i := range a.nodeSlabs {
		clear(a.nodeSlabs[i].reuseLookahead)
	}
}

func (a *nodeArena) prepareLegacyReuseDependencies() {
	if a == nil || a.legacyReuseReads == nil || a.legacyReuseDependenciesReady {
		return
	}
	a.legacyReuseReads.Seal()
	a.legacyReuseDependenciesReady = true
	fill := func(nodes []Node, used int, words *[]uint32) {
		if cap(*words) >= len(nodes) {
			*words = (*words)[:len(nodes)]
		} else {
			cost := int64(len(nodes)-cap(*words)) * 4
			if !a.canAllocateLegacyReuseDependency(cost) {
				return
			}
			*words = make([]uint32, len(nodes))
			a.allocatedBytes += cost
		}
		for i := 0; i < min(used, len(nodes)); i++ {
			if count, ok := a.legacyReuseReads.Lookahead(nodes[i].endByte); ok {
				if encoded := incr.Encode(count); encoded != 0 && encoded <= legacyReuseCountMask {
					(*words)[i] = (*words)[i]&^legacyReuseCountMask | encoded
				}
			}
		}
	}
	fill(a.nodes, a.used, &a.nodeReuseLookahead)
	for i := range a.nodeSlabs {
		slab := &a.nodeSlabs[i]
		fill(slab.data, slab.used, &slab.reuseLookahead)
	}
}

func legacyReuseLookahead(n *Node) (uint32, bool) {
	if n == nil || n.ownerArena == nil || n.ownerArena.legacyReuseSourceChanged {
		return 0, false
	}
	if word := legacyReuseWord(n, false); word != nil {
		return incr.Decode(*word & legacyReuseCountMask)
	}
	return 0, false
}

// A whole-tree lexical shortcut proves today's token tuples, but does not
// rebuild each subtree's new scan frontier. Never carry the old bounds into
// a later structural reuse proof. Shared arenas abstain until their reset.
func (t *Tree) abstainLegacyReuseDependencies() {
	if t == nil {
		return
	}
	if t.arena != nil {
		t.arena.legacyReuseSourceChanged = true
	}
	for _, a := range t.borrowedArena {
		if a != nil {
			a.legacyReuseSourceChanged = true
		}
	}
}

func legacyReuseWord(n *Node, write bool) *uint32 {
	if n == nil || n.ownerArena == nil {
		return nil
	}
	a := n.ownerArena
	const size = unsafe.Sizeof(Node{})
	target := uintptr(unsafe.Pointer(n))
	get := func(nodes []Node, words *[]uint32) (*uint32, bool) {
		if len(nodes) == 0 {
			return nil, false
		}
		base := uintptr(unsafe.Pointer(&nodes[0]))
		if target < base || target >= base+uintptr(len(nodes))*size {
			return nil, false
		}
		i := int((target - base) / size)
		if write && i >= len(*words) {
			if cap(*words) >= len(nodes) {
				*words = (*words)[:len(nodes)]
			} else {
				cost := int64(len(nodes)-cap(*words)) * 4
				if !a.canAllocateLegacyReuseDependency(cost) {
					return nil, true
				}
				next := make([]uint32, len(nodes))
				copy(next, *words)
				*words = next
				a.allocatedBytes += cost
			}
		}
		if i >= len(*words) {
			return nil, true
		}
		return &(*words)[i], true
	}
	if word, found := get(a.nodes, &a.nodeReuseLookahead); found {
		return word
	}
	for i := range a.nodeSlabs {
		s := &a.nodeSlabs[i]
		if word, found := get(s.data, &s.reuseLookahead); found {
			return word
		}
	}
	return nil
}

func noteLegacyReuseLeaf(n *Node, tok Token) {
	if n == nil || n.ownerArena == nil || !n.ownerArena.legacyReuseReads.Recording() {
		return
	}
	if word := legacyReuseWord(n, true); word != nil {
		*word |= legacyReuseLeafKnown
		if tok.isKeyword() {
			*word |= legacyReuseKeyword
		}
	}
}

func (t *Tree) prepareLegacyReuseDependencies() {
	if t == nil {
		return
	}
	t.arena.prepareLegacyReuseDependencies()
	for _, a := range t.borrowedArena {
		if a != t.arena {
			a.prepareLegacyReuseDependencies()
		}
	}
}

// C edits invalidate lookahead dependencies even when a node's visible text
// ends before the edit. Preserve that text's coordinates and propagate changes
// down to every child whose recorded scan touched the edit.
func editLegacyLookaheadOnly(n *Node, edit InputEdit) bool {
	if n == nil || n.isMissing() || n.hasError() || n.endByte >= edit.StartByte {
		return false
	}
	count, ok := legacyReuseLookahead(n)
	if !ok || uint64(n.endByte)+uint64(count) < uint64(edit.StartByte) {
		return false
	}
	if inputEditIsNoop(edit) && uint64(n.endByte)+uint64(count) == uint64(edit.StartByte) {
		return false
	}
	n.setDirty(true)
	if perfCountersEnabled {
		perfRecordNodeEditMarked()
	}
	for i := 0; i < nodeChildCountNoMaterialize(n); i++ {
		child := nodeChildAtForReason(n, i, materializeForEdit)
		editLegacyLookaheadOnly(child, edit)
	}
	return true
}

func (p *Parser) legacyCanReuseFirstLeaf(state StateID, n *Node) bool {
	leaf := leftmostLeaf(n)
	if leaf == nil || int(state) >= len(p.language.LexModes) || int(leaf.preGotoState) >= len(p.language.LexModes) {
		return false
	}
	current, original := p.language.LexModes[state], p.language.LexModes[leaf.preGotoState]
	keyword := false // C nonterminal subtrees do not carry the keyword bit.
	if nodeChildCountNoMaterialize(n) == 0 {
		word := legacyReuseWord(n, false)
		keyword = word == nil || *word&legacyReuseLeafKnown == 0 || *word&legacyReuseKeyword != 0
	}
	entry := p.lookupAction(state, leaf.symbol)
	hasActions, reusable := entry != nil && len(entry.Actions) != 0, entry != nil && entry.Reusable
	return incr.FirstLeaf(current.LexStateIndex() == noLookaheadLexState, hasActions,
		current == original, leaf.symbol == p.language.KeywordCaptureToken,
		keyword, n.preGotoState == state,
		n.startByte == n.endByte && leaf.symbol != 0, current.ExternalLexState != 0, reusable)
}

// The legacy tree omits C's leading-padding ownership. Its token source has
// already scanned the boundary, so authenticate that omitted boundary with the
// fresh token before applying the C table and lex-mode conditions.
func legacyReuseMatchesLookahead(n *Node, lookahead Token) bool {
	leaf := leftmostLeaf(n)
	return leaf != nil && leaf.symbol == lookahead.Symbol && leaf.startByte == lookahead.StartByte && leaf.endByte == lookahead.EndByte
}
