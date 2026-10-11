package parsercorephase0

// SubtreeGeometry is the original token or reduction's byte extent. Reuse
// publication needs this identity without copying materialization sidecars or
// reconstructing scanner and replay metadata.
type SubtreeGeometry struct {
	StartByte uint32
	EndByte   uint32
	Symbol    Symbol
	Terminal  bool
	Missing   bool
}

// SubtreeGeometry authenticates an arena id with the same ownership checks as
// MaterializationView. In particular, opaque reused payloads must still carry
// a live receipt before their geometry can authenticate another projection.
func (c *Core) SubtreeGeometry(id SubtreeID) (SubtreeGeometry, error) {
	record, err := c.subtree(id)
	if err != nil {
		return SubtreeGeometry{}, err
	}
	return SubtreeGeometry{
		StartByte: record.startByte, EndByte: record.endByte, Symbol: record.symbol,
		Terminal: record.terminal, Missing: record.missing,
	}, nil
}
