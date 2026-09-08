package parsercorephase0

// PushedFrontier reports the payload the last push placed on an exact
// single-link frontier, and the state that payload was pushed from. The eager
// materializer calls it after each scheduler push. A frontier with more than
// one link, or an unset link, reports ok=false so the caller declines quietly
// and leaves the subtree to the postorder pass.
func (c *Core) PushedFrontier(head Head) (payload SubtreeID, pre StateID, ok bool) {
	if c == nil || head.Node == 0 || uint64(head.Node) > uint64(len(c.nodes)) {
		return 0, 0, false
	}
	n := &c.nodes[head.Node-1]
	if n.linkCount != 1 || n.firstLink == 0 || uint64(n.firstLink) > uint64(len(c.links)) {
		return 0, 0, false
	}
	link := &c.links[n.firstLink-1]
	if link.next != 0 || link.prev == 0 || uint64(link.prev) > uint64(len(c.nodes)) ||
		link.payload == 0 || uint64(link.payload) > uint64(len(c.subtrees)) {
		return 0, 0, false
	}
	return link.payload, c.nodes[link.prev-1].state, true
}

// FillMaterializationView validates one committed subtree record and fills
// its materialization view and its replay view in place. The postorder pass
// performs the same validation before it visits a record, so a subtree built
// eagerly passes the same checks as one built after acceptance.
func (c *Core) FillMaterializationView(id SubtreeID, view *MaterializationSubtreeView, replay *MaterializationReplayView) error {
	record, err := c.subtree(id)
	if err != nil {
		return err
	}
	if err := c.validateMaterializationMetadata(id, record); err != nil {
		return err
	}
	c.fillMaterializationSubtreeView(id, record, view)
	*replay = c.materializationReplayViewForRecord(id, record)
	return nil
}

// SubtreeChildren returns the child ids of one committed subtree record.
func (c *Core) SubtreeChildren(id SubtreeID) ([]SubtreeID, error) {
	record, err := c.subtree(id)
	if err != nil {
		return nil, err
	}
	return c.children[record.firstChild : record.firstChild+record.childCount], nil
}

// fillMaterializationSubtreeView writes the full materialization view of one
// record into view. MaterializationView, FillMaterializationView, and the
// postorder pass share it.
func (c *Core) fillMaterializationSubtreeView(id SubtreeID, record *subtreeRecord, view *MaterializationSubtreeView) {
	*view = MaterializationSubtreeView{
		Symbol:            record.symbol,
		ProductionID:      record.productionID,
		DynamicPrecedence: int32(record.dynamicPrecedence),
		StartByte:         record.startByte,
		EndByte:           record.endByte,
		Children:          c.children[record.firstChild : record.firstChild+record.childCount],
		Aliases:           c.aliases[record.firstAlias : record.firstAlias+record.aliasCount],
		Extra:             record.extra,
		External:          record.external,
		Terminal:          record.terminal,
		Fragile:           record.fragile,
		Missing:           record.missing,
	}
	// appendAuthenticatedTerminal records scanner provenance only for an
	// external terminal, or for every terminal once the language capability
	// is on. Any other terminal has no entry, so skip the search.
	if record.terminal && (record.external || c.terminalScannerCheckpointProvenance) {
		if provenance, ok := c.externalPayloadScannerProvenance(id); ok {
			view.ExternalScannerCheckpointStart = provenance.start
			view.ExternalScannerCheckpointEnd = provenance.end
			view.ExternalScannerCheckpointExact = true
		}
	}
	if record.missing {
		view.MissingDependency, view.MissingDependencyExact = c.missingLeafDependency(id)
	}
	if len(c.lexerSkippedPrefixes) != 0 {
		view.LexerSkippedPrefixStart, view.LexerSkippedPrefix = c.lexerSkippedPrefix(id)
	}
	c.applyReusedMaterializationView(id, view)
}
