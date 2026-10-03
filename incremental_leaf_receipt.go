package gotreesitter

import "github.com/odvcencio/gotreesitter/internal/incr"

// An alias changes the public symbol, but C's first-leaf reuse check still
// consults the underlying lexer symbol. Keep that rare terminal metadata in
// the arena rather than expanding every node's header.
func legacyReuseFirstLeafSymbol(n *Node) Symbol {
	if n == nil {
		return 0
	}
	if a := n.ownerArena; a != nil {
		a.compactReuseDependencyMu.RLock()
		symbol, ok := a.legacyReuseRawSymbols[n]
		a.compactReuseDependencyMu.RUnlock()
		if ok {
			return symbol
		}
	}
	return n.symbol
}

func setLegacyReuseRawSymbol(n *Node, symbol Symbol) {
	if n == nil || n.ownerArena == nil {
		return
	}
	a := n.ownerArena
	a.compactReuseDependencyMu.Lock()
	defer a.compactReuseDependencyMu.Unlock()
	if _, present := a.legacyReuseRawSymbols[n]; present {
		a.legacyReuseRawSymbols[n] = symbol
		return
	}
	cost := int64(96)
	if a.legacyReuseRawSymbols == nil {
		cost += 256
	}
	if !a.canAllocateLegacyReuseDependency(cost) {
		return
	}
	if a.legacyReuseRawSymbols == nil {
		a.legacyReuseRawSymbols = make(map[*Node]Symbol)
	}
	a.legacyReuseRawSymbols[n] = symbol
	a.allocatedBytes += cost
}

func recordLegacyReuseAliasSymbol(n *Node, lang *Language) {
	recordLegacyReuseAliasSymbolFrom(n, n, lang)
}

func recordLegacyReuseAliasSymbolFrom(dst, src *Node, lang *Language) {
	if src == nil || lang == nil || nodeChildCountNoMaterialize(src) != 0 {
		return
	}
	symbol := legacyReuseFirstLeafSymbol(src)
	if uint32(symbol) < lang.TokenCount {
		setLegacyReuseRawSymbol(dst, symbol)
	}
}

func copyLegacyReuseLeafReceipt(dst, src *Node) {
	if dst == nil || src == nil {
		return
	}
	if word := legacyReuseWord(src, false); word != nil {
		flags := *word & (legacyReuseLeafKnown | legacyReuseKeyword)
		if flags != 0 {
			if target := legacyReuseWord(dst, true); target != nil {
				*target |= flags
			}
		}
	}
	if symbol := legacyReuseFirstLeafSymbol(src); symbol != src.symbol {
		setLegacyReuseRawSymbol(dst, symbol)
	}
}

// Nested ERROR regions can expose hidden derivations and alias decisions that
// a clean subtree receipt does not authenticate. Follow only error-bearing
// paths; ordinary clean suffixes do not require a second tree walk.
func incrementalRecoveryShapeCertified(root *Node) bool {
	return incr.RecoveryShape(root, func(n *Node) bool { return n == nil || !n.HasError() },
		func(n *Node) bool { return n.IsError() },
		func(n *Node) int { return n.ChildCount() },
		func(n *Node, i int) *Node { return n.Child(i) })
}
