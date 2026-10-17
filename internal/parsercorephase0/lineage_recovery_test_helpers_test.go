package parsercorephase0

func (c *Core) nodeLineage(id NodeID) (*nodeLineageRecord, error) {
	v, err := c.lineageView(id)
	if err != nil {
		return nil, err
	}
	r := v.record()
	return &r, nil
}
func (c *Core) testLineageAt(index int) nodeLineageRecord {
	v, err := c.lineageView(NodeID(index + 1))
	if err != nil {
		panic(err)
	}
	return v.record()
}
func (c *Core) mutateTestLineage(index int, edit func(*nodeLineageRecord)) {
	v := c.testLineageAt(index)
	edit(&v)
	if err := c.publishNodeLineage(NodeID(index+1), v); err != nil {
		panic(err)
	}
}

func (c *Core) testLineagesSnapshot() []nodeLineageRecord {
	if len(c.nodeLineages) == 0 {
		return nil
	}
	out := make([]nodeLineageRecord, len(c.nodeLineages))
	for i := range out {
		out[i] = c.testLineageAt(i)
	}
	return out
}
