package parsercorephase0

// NativeProjectionReceiptGeometry authenticates a native occurrence's raw
// identity without rebuilding its materialization sidecars. sameOccurrence
// means the caller checked both the original public-node address and symbol.
// Visible aliases retain their conservative unknown-receipt boundary.
func (c *Core) NativeProjectionReceiptGeometry(id SubtreeID, symbol Symbol, leaf, sameOccurrence bool) (SubtreeGeometry, bool, error) {
	r, err := c.subtree(id)
	if err != nil {
		return SubtreeGeometry{}, false, err
	}
	g := SubtreeGeometry{StartByte: r.startByte, EndByte: r.endByte, Symbol: r.symbol, Terminal: r.terminal, Missing: r.missing}
	if r.symbol == symbol {
		return g, true, nil
	}
	if c.selectedPolicy == nil {
		return g, false, nil
	}
	meta, ok := c.selectedPolicy.symbol(r.symbol)
	if !ok || meta.Visible {
		return g, false, nil
	}
	if leaf {
		return g, sameOccurrence, nil
	}
	return g, !r.terminal && r.childCount == 1 && r.aliasCount == 0, nil
}
